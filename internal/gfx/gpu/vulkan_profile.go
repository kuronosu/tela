//go:build telagpu && (linux || windows)

package gpu

import (
	"encoding/binary"
	"image"
	"math"
	"slices"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/vk"
)

// Where the time of a frame goes on the device (Profiler): a tool of the
// renderer's development, off unless asked for.
//
// A frame is drawn as always, but each pass is a submission of its own,
// timed with its wait. Before a pass, the instances of each class it holds
// (classify) are drawn alone, with the pass's own pipelines, scissors and
// textures, into a surface apart of the pass's format, a few times, and
// the fastest is the class's time: the device's work for those pixels,
// without the others'. An empty render pass submitted the same way gives
// what a submission costs, which is taken away. The sum of the classes of
// a pass is close to the pass's own time on a device that shades fragments
// where they are drawn (both of the owner's do); what is left is the cost
// of the pass itself.

// profileRepeat is the number of times a class is drawn: the fastest
// counts.
const profileRepeat = 3

type profiler struct {
	rows map[string]*ProfileRow
	// scratch are the surfaces the classes are drawn into, by format.
	scratch [2]*surface
	runs    []profRun
	classes []string
}

// profRun is a run of consecutive instances of one class in a batch.
type profRun struct {
	batch        int // in the pass
	first, count int
	class        string
}

func (d *vkDevice) SetProfiling(on bool) {
	switch {
	case on && d.prof == nil:
		d.prof = &profiler{rows: map[string]*ProfileRow{}}
	case !on:
		d.dropProfile()
	}
}

func (d *vkDevice) TakeProfile() []ProfileRow {
	if d.prof == nil {
		return nil
	}
	out := make([]ProfileRow, 0, len(d.prof.rows))
	for _, r := range d.prof.rows {
		out = append(out, *r)
	}
	slices.SortFunc(out, func(a, b ProfileRow) int { return int(b.Time - a.Time) })
	clear(d.prof.rows)
	return out
}

// dropProfile ends the measurement and destroys its surfaces. Nothing is
// in flight.
func (d *vkDevice) dropProfile() {
	if d.prof == nil {
		return
	}
	for _, s := range d.prof.scratch {
		if s != nil && d.dev != nil {
			d.destroySurface(s)
		}
	}
	d.prof = nil
}

func (p *profiler) add(class string, runs int, pixels, texels int64, t time.Duration) {
	r := p.rows[class]
	if r == nil {
		r = &ProfileRow{Class: class}
		p.rows[class] = r
	}
	r.Runs += runs
	r.Pixels += pixels
	r.Texels += texels
	r.Time += t
}

// classify names the class of an instance of a pipeline, and returns the
// taps a filter step reads per pixel.
func classify(pipeline Pipeline, in []byte) (class string, taps int) {
	u := func(slot, i int) uint32 { return binary.LittleEndian.Uint32(in[16*slot+4*i:]) }
	f := func(slot, i int) float32 { return math.Float32frombits(u(slot, i)) }
	if pipeline == PipelineFilter {
		step, flags, n := u(FilterSlotHead, 0), u(FilterSlotHead, 1), int(u(FilterSlotHead, 3))
		switch step {
		case FilterTaps:
			class, taps = "filter taps", n
			if u(FilterSlotSrc, 2) > 1 || u(FilterSlotSrc, 3) > 1 {
				class = "filter taps (to blocks)"
			}
		case FilterUp:
			class, taps = "filter up", 4
		default:
			class, taps = "filter copy", 1
		}
		if flags&FilterFlagRound == 0 {
			class += " 16F"
		}
		if flags&FilterFlagMirror != 0 {
			class += " +mirror"
		}
		if flags&FilterFlagDrop != 0 {
			class += " +drop"
		}
		if u(FilterSlotPost, 1) > 0 {
			class += " +matrix"
		}
		return class, taps
	}
	flags := u(SlotHead, 0)
	switch {
	case flags&FlagLayer != 0:
		class = "layer composite"
		if u(SlotHead, 1)>>24 != 0 {
			class = "backdrop paint"
		}
		if pipeline == PipelineBlend {
			class += " (blend mode)"
		}
	case flags&FlagKindMask == FlagCoverAtlas:
		class = "atlas coverage"
		if w, h := f(SlotBounds, 2)-f(SlotBounds, 0), f(SlotBounds, 3)-f(SlotBounds, 1); w*h > 128*128 {
			class = "atlas coverage (large)"
		}
	case flags&FlagKindMask != 0:
		class = "shadow outer"
		if flags&FlagKindMask == FlagInsetShadow {
			class = "shadow inset"
		}
		if (flags>>FlagBlurShift)&3 != BlurAnalytic {
			class += " (hard)"
		}
	default:
		class = "fill rect"
		for i := range 4 {
			if f(SlotShape+1, i) > 0 {
				class = "fill rrect"
			}
		}
		if flags&FlagShapeLocal != 0 {
			class += " (local)"
		}
	}
	if flags&FlagLayer == 0 {
		switch (flags >> FlagPaintShift) & 7 {
		case PaintSolid:
		case PaintImageLinear, PaintImageNearest:
			if flags&FlagPattern != 0 {
				class += ", pattern"
			} else {
				class += ", image"
			}
		case PaintSprite:
			class += ", sprite"
		default:
			class += ", gradient"
		}
	}
	if (flags>>FlagClipShift)&3 != 0 {
		class += " +clip"
	}
	if flags&FlagMask != 0 {
		class += " +mask"
	}
	switch pipeline {
	case PipelinePlain:
		class += " (plain)"
	case PipelineOpaque:
		class += " (opaque)"
	}
	return class, 0
}

// profScratch returns the surface the classes of a pass into s are drawn
// into: one of s's format, at least as large.
func (d *vkDevice) profScratch(s *surface) (*surface, error) {
	p := d.prof
	if sc := p.scratch[s.format]; sc != nil {
		if sc.w >= s.w && sc.h >= s.h {
			return sc, nil
		}
		d.destroySurface(sc)
		p.scratch[s.format] = nil
	}
	sc, err := d.newSurface(s.w, s.h, s.format)
	if err != nil {
		return nil, err
	}
	p.scratch[s.format] = sc
	return sc, nil
}

// timed runs record as a submission and returns how long it took.
func (d *vkDevice) timed(record func(cb vk.CommandBuffer)) (time.Duration, error) {
	t0 := time.Now()
	err := d.run(record)
	return time.Since(t0), err
}

// drawProfiled is the recording of draw, measured: see the top of the
// file.
func (d *vkDevice) drawProfiled(t *vkTarget, f *Frame, prelude func(cb vk.CommandBuffer), dataOff int) error {
	p := d.prof
	dev := d.dev
	if err := d.run(prelude); err != nil {
		return err
	}
	// What a submission of a render pass that draws nothing costs.
	sc, err := d.profScratch(t.surf)
	if err != nil {
		return err
	}
	empty := time.Duration(math.MaxInt64)
	for range profileRepeat {
		dt, err := d.timed(func(cb vk.CommandBuffer) {
			d.use(cb, sc, vk.ImageLayoutColorAttachmentOptimal)
			dev.CmdBeginRenderPass(cb, &vk.RenderPassBeginInfo{
				SType:       vk.StructureTypeRenderPassBeginInfo,
				RenderPass:  d.renderPass[sc.format],
				Framebuffer: sc.framebuffer,
				RenderArea:  rect2D(image.Rect(0, 0, sc.w, sc.h)),
			}, vk.SubpassContentsInline)
			dev.CmdEndRenderPass(cb)
		})
		if err != nil {
			return err
		}
		empty = min(empty, dt)
	}
	next := 0 // in d.plan
	for pi := range f.Passes {
		pass := &f.Passes[pi]
		plan := d.plan[next : next+len(pass.Batches)]
		next += len(pass.Batches)
		s := d.surfaceOf(t, pass.Target)
		// The runs of the pass, by class.
		p.runs, p.classes = p.runs[:0], p.classes[:0]
		for bi := range pass.Batches {
			b := &pass.Batches[bi]
			if plan[bi].skip {
				continue
			}
			sci := b.Scissor
			for k := b.First; k < b.First+b.Count; k++ {
				in := f.Instances[k*InstanceSize : (k+1)*InstanceSize]
				class, taps := classify(b.Pipeline, in)
				bounds := image.Rect(
					int(math.Float32frombits(binary.LittleEndian.Uint32(in[0:]))), int(math.Float32frombits(binary.LittleEndian.Uint32(in[4:]))),
					int(math.Float32frombits(binary.LittleEndian.Uint32(in[8:]))), int(math.Float32frombits(binary.LittleEndian.Uint32(in[12:])))).Intersect(sci)
				px := int64(bounds.Dx()) * int64(bounds.Dy())
				p.add(class, 0, px, px*int64(taps), 0)
				if n := len(p.runs); n > 0 && p.runs[n-1].batch == bi && p.runs[n-1].class == class {
					p.runs[n-1].count++
				} else {
					p.runs = append(p.runs, profRun{batch: bi, first: k, count: 1, class: class})
				}
				if !slices.Contains(p.classes, class) {
					p.classes = append(p.classes, class)
				}
			}
		}
		if len(p.classes) > 0 {
			if sc, err = d.profScratch(s); err != nil {
				return err
			}
		}
		for _, class := range p.classes {
			best := time.Duration(math.MaxInt64)
			n := 0
			for range profileRepeat {
				n = 0
				dt, err := d.timed(func(cb vk.CommandBuffer) {
					for bi := range pass.Batches {
						for _, src := range pass.Batches[bi].Sources {
							switch {
							case src == SourceDest:
								d.use(cb, t.surf, vk.ImageLayoutShaderReadOnlyOptimal)
							case src > 0:
								d.use(cb, d.offscreen[int(src)], vk.ImageLayoutShaderReadOnlyOptimal)
							}
						}
					}
					d.use(cb, sc, vk.ImageLayoutColorAttachmentOptimal)
					dev.CmdBeginRenderPass(cb, &vk.RenderPassBeginInfo{
						SType:       vk.StructureTypeRenderPassBeginInfo,
						RenderPass:  d.renderPass[sc.format],
						Framebuffer: sc.framebuffer,
						RenderArea:  rect2D(image.Rect(0, 0, sc.w, sc.h)),
					}, vk.SubpassContentsInline)
					// The pass's own mapping of pixels: the scratch is at least
					// as large as its target.
					constants := &[4]float32{2 / float32(s.w), 2 / float32(s.h), float32(pass.Origin.X), float32(pass.Origin.Y)}
					dev.CmdSetViewport(cb, 0, []vk.Viewport{{Width: float32(s.w), Height: float32(s.h), MinDepth: 0, MaxDepth: 1}})
					dev.CmdBindDescriptorSets(cb, vk.PipelineBindPointGraphics, d.pipelineLayout, 0, []vk.DescriptorSet{d.set}, nil)
					dev.CmdPushConstants(cb, d.pipelineLayout, vk.ShaderStageVertex|vk.ShaderStageFragment, 0, 16, unsafe.Pointer(constants))
					var pipeline vk.Pipeline
					var set vk.DescriptorSet
					for _, r := range p.runs {
						if r.class != class {
							continue
						}
						pb := &plan[r.batch]
						if pb.pipeline != pipeline {
							pipeline = pb.pipeline
							dev.CmdBindPipeline(cb, vk.PipelineBindPointGraphics, pipeline)
						}
						if pb.set != set {
							set = pb.set
							dev.CmdBindDescriptorSets(cb, vk.PipelineBindPointGraphics, d.pipelineLayout, 1, []vk.DescriptorSet{set}, nil)
						}
						dev.CmdSetScissor(cb, 0, []vk.Rect2D{pb.scissor})
						dev.CmdDraw(cb, 4, uint32(r.count), 0, uint32(r.first))
						n++
					}
					dev.CmdEndRenderPass(cb)
				})
				if err != nil {
					return err
				}
				best = min(best, dt)
			}
			p.add(class, n, 0, 0, max(best-empty, 0))
		}
		// The pass itself.
		dt, err := d.timed(func(cb vk.CommandBuffer) { d.recordPass(cb, t, pass, plan, dataOff) })
		if err != nil {
			return err
		}
		p.add("(passes)", 1, 0, 0, max(dt-empty, 0))
		p.add("(submit)", 1, 0, 0, empty)
	}
	// The destination rests ready to be read back.
	return d.run(func(cb vk.CommandBuffer) {
		d.giveImports(cb, f)
		d.use(cb, t.surf, vk.ImageLayoutTransferSrcOptimal)
	})
}
