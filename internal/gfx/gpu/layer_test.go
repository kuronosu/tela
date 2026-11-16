package gpu

import (
	"fmt"
	"image"
	"math"
	"math/rand"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The tests of layers, filters and backdrop filters without a device: the
// passes a frame is made of, their targets and what they read.

// passOf describes a pass in a word: the target, "+clear" when it clears,
// and the pipelines of its batches.
func passOf(p *Pass) string {
	s := "dest"
	if p.Target != 0 {
		s = "t" + strconv.Itoa(p.Target)
	}
	if p.Clear != nil {
		s += "+clear"
	}
	for _, b := range p.Batches {
		s += [...]string{" quad", " blend", " filter", " plain", " opaque"}[b.Pipeline]
	}
	return s
}

func passesOf(f *Frame) []string {
	var out []string
	for i := range f.Passes {
		out = append(out, passOf(&f.Passes[i]))
	}
	return out
}

func targetOf(t *testing.T, f *Frame, id int) TargetInfo {
	t.Helper()
	for _, info := range f.Targets {
		if info.ID == id {
			return info
		}
	}
	t.Fatalf("the frame lists no target %d", id)
	return TargetInfo{}
}

func TestLayoutMatchesFilterShader(t *testing.T) {
	src, err := os.ReadFile("shaders/filter.frag")
	if err != nil {
		t.Fatal(err)
	}
	frag := string(src)
	m := regexp.MustCompile(`(?s)struct Instance \{(.*?)\n\};`).FindStringSubmatch(frag)
	if m == nil {
		t.Fatal("no struct Instance in filter.frag")
	}
	want := map[string]int{"bounds": SlotBounds, "head": FilterSlotHead, "src": FilterSlotSrc, "srcRect": FilterSlotSrcRect, "axis": FilterSlotAxis,
		"aux": FilterSlotAux, "auxRect": FilterSlotAuxRect, "color": FilterSlotColor, "mirror": FilterSlotMirror, "post": FilterSlotPost}
	slot := 0
	for _, f := range regexp.MustCompile(`(?m)^\s*[ui]?vec4 (\w+)(?:\[(\d+)\])?;`).FindAllStringSubmatch(m[1], -1) {
		if w, ok := want[f[1]]; ok {
			if w != slot {
				t.Errorf("Instance.%s is at slot %d in filter.frag, %d in frame.go", f[1], slot, w)
			}
			delete(want, f[1])
		}
		n := 1
		if f[2] != "" {
			n, _ = strconv.Atoi(f[2])
		}
		slot += n
	}
	if slot != InstanceSlots {
		t.Errorf("struct Instance has %d slots in filter.frag, InstanceSlots is %d", slot, InstanceSlots)
	}
	for name := range want {
		t.Errorf("filter.frag has no Instance.%s", name)
	}
	consts := map[string]uint32{"STEP_TAPS": FilterTaps, "STEP_UP": FilterUp, "STEP_COPY": FilterCopy,
		"FLAG_ROUND": FilterFlagRound, "FLAG_DROP": FilterFlagDrop, "FLAG_MIRROR": FilterFlagMirror, "FLAG_MIRROR_AUX": FilterFlagMirrorAux}
	check := func(file, text string, consts map[string]uint32) {
		for name, v := range consts {
			m := regexp.MustCompile(`const uint ` + name + ` = (\d+)u;`).FindStringSubmatch(text)
			if m == nil {
				t.Errorf("%s has no constant %s", file, name)
			} else if got, _ := strconv.Atoi(m[1]); uint32(got) != v {
				t.Errorf("%s is %d in %s, %d in Go", name, got, file, v)
			}
		}
	}
	check("filter.frag", frag, consts)
	// The blend modes and the layer flag of the quad shader.
	quad, err := os.ReadFile("shaders/quad.frag")
	if err != nil {
		t.Fatal(err)
	}
	check("quad.frag", string(quad), map[string]uint32{
		"LAYER": FlagLayer, "BLEND_MULTIPLY": uint32(gfx.Multiply), "BLEND_SCREEN": uint32(gfx.Screen), "BLEND_OVERLAY": uint32(gfx.Overlay),
		"BLEND_DARKEN": uint32(gfx.Darken), "BLEND_LIGHTEN": uint32(gfx.Lighten), "BLEND_COLOR_DODGE": uint32(gfx.ColorDodge),
		"BLEND_COLOR_BURN": uint32(gfx.ColorBurn), "BLEND_HARD_LIGHT": uint32(gfx.HardLight), "BLEND_SOFT_LIGHT": uint32(gfx.SoftLight),
		"BLEND_DIFFERENCE": uint32(gfx.Difference), "BLEND_EXCLUSION": uint32(gfx.Exclusion), "BLEND_HUE": uint32(gfx.Hue),
		"BLEND_SATURATION": uint32(gfx.Saturation), "BLEND_COLOR": uint32(gfx.ColorBlend), "BLEND_LUMINOSITY": uint32(gfx.Luminosity),
		"BLEND_PLUS_LIGHTER": uint32(gfx.PlusLighter),
	})
	if LayerMaskAlpha != 1 || LayerMaskLuminance != 2 || gfx.SrcOver != 0 {
		t.Error("the quad shader takes a mask kind of 1 for alpha and 2 for luminance, and a blend mode of 0 for source-over")
	}
}

// TestEncodeLayer checks the passes of a layer: the pass below ends, the
// content is a pass into a cleared target without the clip around the
// layer, and the pass below goes on, without a clear, with one instance
// that composites the target through that clip.
func TestEncodeLayer(t *testing.T) {
	var star gfx.Path
	star.MoveTo(gfx.Pt(20, 20))
	star.LineTo(gfx.Pt(90, 40))
	star.LineTo(gfx.Pt(40, 90))
	star.Close()
	for _, c := range []struct {
		name string
		clip func(b *gfx.Builder)
		// what the composite carries
		clipKind uint32
		mask     bool
	}{
		{"no clip", nil, 0, false},
		{"a rounded clip", func(b *gfx.Builder) { b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(10.5, 10.5, 70, 70), 12)) }, ClipDevice, false},
		{"a path clip", func(b *gfx.Builder) { b.PushClipPath(star, gfx.NonZero) }, 0, true},
	} {
		f := encode(100, 100, Options{Clear: &gfx.White}, func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 100, 100), slate)
			if c.clip != nil {
				c.clip(b)
			}
			b.PushLayer(0.5, gfx.SrcOver)
			b.FillRect(gfx.RectXYWH(20, 30, 40, 30), rose)
			b.FillRect(gfx.RectXYWH(30, 40, 40, 30), sky)
			b.PopLayer()
			if c.clip != nil {
				b.PopClip()
			}
			b.FillRect(gfx.RectXYWH(0, 90, 100, 10), amber)
		})
		checkFrame(t, c.name, f)
		if len(f.Targets) != 1 || f.Targets[0].Format != FormatRGBA8 {
			t.Fatalf("%s: targets %+v", c.name, f.Targets)
		}
		id := f.Targets[0].ID
		// (The composite and what follows it have other scissors: two
		// batches of the pass that goes on.)
		want := []string{"dest+clear quad", fmt.Sprintf("t%d+clear quad", id), "dest quad quad"}
		if got := passesOf(f); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("%s: passes %q, want %q", c.name, got, want)
		}
		layer, last := &f.Passes[1], &f.Passes[2]
		rect := image.Rect(19, 29, 71, 71) // the content's bounds, with the rasterizer's margin
		if c.clip != nil {
			rect = rect.Intersect(layer.Clear.Rect) // cut by the clip's bounds
		}
		if layer.Origin != layer.Clear.Rect.Min || !layer.Clear.Rect.In(image.Rect(19, 19, 91, 91)) || layer.Clear.Color != [4]uint8{} || layer.Clear.Rect != rect {
			t.Errorf("%s: the layer's pass: origin %v, clear %+v, want the rectangle %v", c.name, layer.Origin, layer.Clear, rect)
		}
		// The content: two instances, without any clip, in the layer's
		// pixels.
		lb := layer.Batches[0]
		if lb.Count != 2 || lb.Scissor != rect || lb.Sources != [3]Source{} {
			t.Errorf("%s: the content's batch %+v", c.name, lb)
		}
		for i := lb.First; i < lb.First+lb.Count; i++ {
			if fl := head(f, i)[0]; fl>>FlagClipShift&3 != 0 || fl&FlagMask != 0 || fl&FlagLayer != 0 {
				t.Errorf("%s: a content instance has the flags %#x", c.name, fl)
			}
		}
		// The composite: the first instance of the pass that goes on, a
		// batch of its own that reads the layer.
		cb := last.Batches[0]
		if cb.Count != 1 || cb.Sources != [3]Source{Source(id)} || cb.Pipeline != PipelineQuad || cb.Scissor != rect {
			t.Fatalf("%s: the composite's batch %+v", c.name, cb)
		}
		h := head(f, cb.First)
		if h[0]&FlagLayer == 0 || h[0]>>FlagClipShift&3 != c.clipKind || (h[0]&FlagMask != 0) != c.mask {
			t.Errorf("%s: the composite has the flags %#x, want the clip %d, a mask %v", c.name, h[0], c.clipKind, c.mask)
		}
		if h[1] != 128 { // opacity 0.5, source-over, no mask
			t.Errorf("%s: the composite's layer word is %#x, want the opacity 128 alone", c.name, h[1])
		}
		if tex := slotInts(f, cb.First, SlotTex); image.Pt(int(-tex[0]), int(-tex[1])) != layer.Origin {
			t.Errorf("%s: the composite reads from %v, the layer starts at %v", c.name, tex, layer.Origin)
		}
		// What follows the layer is drawn by the same pass.
		if n := len(last.Batches); last.Batches[n-1].Sources != [3]Source{} || last.Clear != nil {
			t.Errorf("%s: the pass that goes on: %+v", c.name, last)
		}
	}
}

// TestEncodeLayerKinds checks what the other kinds of layers add: a blend
// mode copies the backdrop first and composites without the device's
// blending; a mask is a target drawn before the content; nested layers
// nest their passes; siblings share their targets.
func TestEncodeLayerKinds(t *testing.T) {
	content := func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(20, 20, 40, 40), rose) }
	// A blend mode.
	f := encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 100, 100), slate)
		b.PushLayer(1, gfx.Multiply)
		content(b)
		b.PopLayer()
	})
	checkFrame(t, "blend", f)
	if got := passesOf(f); len(got) != 4 || got[0] != "dest quad" || got[2][len(got[2])-6:] != "filter" || got[3] != "dest blend" {
		t.Fatalf("blend: passes %q", got)
	}
	layer, back, last := f.Passes[1].Target, f.Passes[2].Target, &f.Passes[3]
	if layer == back || f.Passes[2].Clear != nil || f.Passes[2].Batches[0].Sources != [3]Source{SourceDest} {
		t.Errorf("blend: the copy of the backdrop: %+v (the layer is the target %d)", f.Passes[2], layer)
	}
	cb := last.Batches[0]
	if cb.Sources != [3]Source{Source(layer), 0, Source(back)} || head(f, cb.First)[1] != 255|uint32(gfx.Multiply)<<8 {
		t.Errorf("blend: the composite %+v, layer word %#x", cb, head(f, cb.First)[1])
	}
	if off := floats(f, cb.First, SlotPaint+1); image.Pt(int(-off[2]), int(-off[3])) != f.Passes[2].Origin {
		t.Errorf("blend: the composite reads the backdrop from %v, its copy starts at %v", off, f.Passes[2].Origin)
	}

	// A mask: its target is drawn first, whole, then the content.
	mask := gfx.Mask{Mode: gfx.MaskLuminance, List: gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 200, 200), gfx.White)
		b.FillRect(gfx.RectXYWH(30, 30, 10, 10), gfx.Black)
	})}
	f = encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushMaskLayer(mask, 0.5, gfx.SrcOver)
		content(b)
		b.PopLayer()
	})
	checkFrame(t, "mask", f)
	if got := passesOf(f); len(got) != 3 || got[2] != "dest quad" || f.Passes[0].Clear == nil || f.Passes[1].Clear == nil {
		t.Fatalf("mask: passes %q", got)
	}
	if f.Passes[0].Batches[0].Count != 2 || f.Passes[1].Batches[0].Count != 1 {
		t.Errorf("mask: the mask's pass has %d instances and the content's %d, want 2 and 1", f.Passes[0].Batches[0].Count, f.Passes[1].Batches[0].Count)
	}
	cb = f.Passes[2].Batches[0]
	if cb.Sources != [3]Source{Source(f.Passes[1].Target), Source(f.Passes[0].Target)} || head(f, cb.First)[1] != 128|LayerMaskLuminance<<16 {
		t.Errorf("mask: the composite %+v, layer word %#x", cb, head(f, cb.First)[1])
	}
	// The mask covers the pixels composited, whatever its list paints.
	if f.Passes[0].Clear.Rect != f.Passes[1].Clear.Rect {
		t.Errorf("mask: it holds %v, the layer %v", f.Passes[0].Clear.Rect, f.Passes[1].Clear.Rect)
	}

	// Three layers one in another, then two siblings: the passes nest, and
	// the frame needs the targets of one chain only.
	var e Encoder
	f = e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.PushLayer(0.9, gfx.SrcOver)
		content(b)
		b.PushLayer(0.8, gfx.SrcOver)
		content(b)
		b.PushLayer(0.7, gfx.SrcOver)
		content(b)
		content(b)
		b.PopLayer()
		content(b)
		b.PopLayer()
		b.PopLayer()
		for i := 0; i < 5; i++ {
			b.PushLayer(0.5, gfx.SrcOver)
			content(b)
			content(b) // (a layer of one rectangle alone needs no target)
			b.PopLayer()
		}
	}), image.Pt(100, 100), Options{})
	checkFrame(t, "nested", f)
	if len(f.Targets) != 3 {
		t.Errorf("nested: %d targets, want the 3 of the deepest chain", len(f.Targets))
	}
	a, b, c := f.Passes[0].Target, f.Passes[1].Target, f.Passes[2].Target
	want := []int{a, b, c, b, a, 0}
	for i, w := range want {
		if f.Passes[i].Target != w || (f.Passes[i].Clear != nil) != (i < 3) {
			t.Fatalf("nested: pass %d is %q, want the target %d", i, passOf(&f.Passes[i]), w)
		}
	}
	if a == b || b == c || a == c || a == 0 || len(f.Passes) != 6+2*5 {
		// (Each sibling ends the destination's pass and starts it again.)
		t.Errorf("nested: targets %d, %d, %d in %d passes: %q", a, b, c, len(f.Passes), passesOf(f))
	}
	// The same frame again finds its targets.
	ids := fmt.Sprint(f.Targets)
	if f = e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.PushLayer(0.9, gfx.SrcOver)
		content(b)
		b.PopLayer()
	}), image.Pt(100, 100), Options{}); fmt.Sprint(f.Targets) != ids {
		t.Errorf("a second frame lists the targets %v, the first %v", f.Targets, ids)
	}

	// A layer of which nothing shows draws nothing: no opacity, nothing in
	// the region, an empty clip.
	for name, draw := range map[string]func(b *gfx.Builder){
		"outside the region": func(b *gfx.Builder) {
			b.PushLayer(0.5, gfx.SrcOver, gfx.Blur{StdDev: 2})
			b.FillRect(gfx.RectXYWH(70, 70, 20, 20), rose)
			b.PopLayer()
		},
		"an empty clip": func(b *gfx.Builder) {
			b.PushClipRect(gfx.RectXYWH(5, 5, 0, 10))
			b.PushLayer(0.5, gfx.Multiply)
			content(b)
			b.PopLayer()
			b.PopClip()
		},
	} {
		if f := encode(100, 100, Options{Region: image.Rect(0, 0, 40, 40)}, draw); len(f.Passes) != 0 || len(f.Targets) != 0 {
			t.Errorf("%s: passes %q, %d targets", name, passesOf(f), len(f.Targets))
		}
	}
}

// TestEncodeFade checks that a layer that only fades one rectangle on
// pixel boundaries is drawn without a target, as the rectangle with the
// layer's opacity on its coverage, and that every other layer keeps its
// target.
func TestEncodeFade(t *testing.T) {
	rect := gfx.RectXYWH(10, 20, 60, 40)
	one := func(op float32, blend gfx.BlendMode, r gfx.Rect, p gfx.Paint, fs ...gfx.Filter) func(b *gfx.Builder) {
		return func(b *gfx.Builder) {
			b.PushLayer(op, blend, fs...)
			b.FillRect(r, p)
			b.PopLayer()
		}
	}
	f := encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 100, 100), slate.WithAlpha(0.5))
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(15.5, 25.5, 80, 70), 12))
		one(0.4, gfx.SrcOver, rect, rose)(b) // over the clip's corner
		b.PopClip()
	})
	checkFrame(t, "fade", f)
	if len(f.Passes) != 1 || len(f.Targets) != 0 || instances(f) != 2 {
		t.Fatalf("a layer of one rectangle: passes %q, %d targets, %d instances", passesOf(f), len(f.Targets), instances(f))
	}
	h := head(f, 1)
	if h[0]&FlagOpacity == 0 || h[0]&FlagLayer != 0 || h[1] != 102 || h[0]>>FlagClipShift&3 != ClipDevice {
		t.Errorf("the faded rectangle: flags %#x, opacity %d, want FlagOpacity, the clip around the layer and 102", h[0], h[1])
	}
	if got, want := f.Passes[0].Batches[1].Scissor, image.Rect(15, 25, 71, 61); got != want {
		t.Errorf("the faded rectangle's scissor: %v, want the clip inside the layer's bounds, %v", got, want)
	}
	if h0 := head(f, 0); h0[0]&FlagOpacity != 0 {
		t.Errorf("a fill outside the layer is faded (flags %#x)", h0[0])
	}
	// Everything else is a layer in a target of its own.
	for _, c := range []struct {
		name string
		draw func(b *gfx.Builder)
	}{
		{"an edge between pixels", one(0.4, gfx.SrcOver, gfx.RectXYWH(10.5, 20, 60, 40), rose)},
		{"a gradient", one(0.4, gfx.SrcOver, rect, gfx.LinearGradient{End: gfx.Pt(100, 0), Stops: stops(rose, sky)})},
		{"a blend mode", one(0.4, gfx.Multiply, rect, rose)},
		{"a filter", one(0.4, gfx.SrcOver, rect, rose, gfx.Blur{StdDev: 2})},
		{"a rotation", func(b *gfx.Builder) {
			b.PushTransform(gfx.RotateAbout(0.2, gfx.Pt(40, 40)))
			one(0.4, gfx.SrcOver, rect, rose)(b)
			b.PopTransform()
		}},
		{"a rounded rectangle", func(b *gfx.Builder) {
			b.PushLayer(0.4, gfx.SrcOver)
			b.FillRRect(gfx.NewRRect(rect, 6), rose)
			b.PopLayer()
		}},
		{"two rectangles", func(b *gfx.Builder) {
			b.PushLayer(0.4, gfx.SrcOver)
			b.FillRect(rect, rose)
			b.FillRect(rect, sky)
			b.PopLayer()
		}},
		{"a rectangle in a clip", func(b *gfx.Builder) {
			b.PushLayer(0.4, gfx.SrcOver)
			b.PushClipRect(gfx.RectXYWH(20, 20, 20, 20))
			b.FillRect(rect, rose)
			b.PopClip()
			b.PopLayer()
		}},
		{"a mask", func(b *gfx.Builder) {
			b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(b *gfx.Builder) { b.FillRect(rect, gfx.Black) }), 0.4, gfx.SrcOver)
			b.FillRect(rect, rose)
			b.PopLayer()
		}},
	} {
		f := encode(100, 100, Options{}, c.draw)
		checkFrame(t, c.name, f)
		faded := false
		for i := range instances(f) {
			for pi := range f.Passes {
				for _, bt := range f.Passes[pi].Batches {
					if bt.Pipeline != PipelineFilter && i >= bt.First && i < bt.First+bt.Count && head(f, i)[0]&(FlagOpacity|FlagLayer) == FlagOpacity {
						faded = true
					}
				}
			}
		}
		if len(f.Targets) == 0 || faded {
			t.Errorf("%s: %d targets, faded without a layer: %v (passes %q)", c.name, len(f.Targets), faded, passesOf(f))
		}
	}
	// An opaque layer is no layer; one of opacity 0 draws nothing.
	if f := encode(100, 100, Options{}, one(1, gfx.SrcOver, rect, rose)); len(f.Targets) == 0 {
		// (The rasterizer's layer at opacity 1 is still a layer here: one
		// rectangle in a target, composited whole.)
		t.Log("a layer at opacity 1 is drawn without a target")
	}
	if f := encode(100, 100, Options{}, one(0, gfx.SrcOver, rect, rose)); instances(f) != 0 {
		t.Errorf("a layer of opacity 0 draws %d instances", instances(f))
	}
}

// filterSteps returns the filter instances of a frame in the order they
// are drawn, with the pass of each.
func filterSteps(f *Frame) (steps []int, passes []*Pass) {
	for pi := range f.Passes {
		for _, b := range f.Passes[pi].Batches {
			if b.Pipeline == PipelineFilter {
				steps = append(steps, b.First)
				passes = append(passes, &f.Passes[pi])
			}
		}
	}
	return steps, passes
}

// TestEncodeFilters checks the steps of the filters of a layer: their
// order, the formats of their targets, what each reads, and that the color
// matrices ride on the step before them.
func TestEncodeFilters(t *testing.T) {
	layer := func(fs ...gfx.Filter) *Frame {
		f := encode(300, 200, Options{}, func(b *gfx.Builder) {
			b.PushLayer(1, gfx.SrcOver, fs...)
			b.FillRect(gfx.RectXYWH(100, 60, 80, 60), rose)
			b.PopLayer()
		})
		checkFrame(t, fmt.Sprint(fs), f)
		return f
	}
	type step struct {
		kind   uint32
		flags  uint32
		format TargetFormat
		post   uint32
	}
	for _, c := range []struct {
		name string
		fs   []gfx.Filter
		want []step
	}{
		// Small: the kernel along x into half floats, then along y,
		// rounded.
		{"blur 3", []gfx.Filter{gfx.Blur{StdDev: 3}}, []step{{FilterTaps, 0, FormatRGBA16F, 0}, {FilterTaps, FilterFlagRound, FormatRGBA8, 0}}},
		// Large: blocks along x, along y, the kernel twice, and back.
		{"blur 20", []gfx.Filter{gfx.Blur{StdDev: 20}}, []step{{FilterTaps, 0, FormatRGBA16F, 0}, {FilterTaps, 0, FormatRGBA16F, 0},
			{FilterTaps, 0, FormatRGBA16F, 0}, {FilterTaps, 0, FormatRGBA16F, 0}, {FilterUp, FilterFlagRound, FormatRGBA8, 0}}},
		{"matrix", []gfx.Filter{gfx.Grayscale(1)}, []step{{FilterCopy, FilterFlagRound, FormatRGBA8, 1}}},
		{"blur, two matrices", []gfx.Filter{gfx.Blur{StdDev: 3}, gfx.Grayscale(1), gfx.Invert(1)},
			[]step{{FilterTaps, 0, FormatRGBA16F, 0}, {FilterTaps, FilterFlagRound, FormatRGBA8, 2}}},
		{"matrix, blur", []gfx.Filter{gfx.Grayscale(1), gfx.Blur{StdDev: 3}},
			[]step{{FilterCopy, FilterFlagRound, FormatRGBA8, 1}, {FilterTaps, 0, FormatRGBA16F, 0}, {FilterTaps, FilterFlagRound, FormatRGBA8, 0}}},
		{"drop shadow", []gfx.Filter{gfx.DropShadow{Offset: gfx.Pt(2, 3), StdDev: 2, Color: gfx.Black}},
			[]step{{FilterTaps, 0, FormatRGBA16F, 0}, {FilterTaps, FilterFlagRound | FilterFlagDrop, FormatRGBA8, 0}}},
		{"a sharp drop shadow, then a matrix", []gfx.Filter{gfx.DropShadow{Color: gfx.Black}, gfx.Sepia(1)},
			[]step{{FilterCopy, FilterFlagRound | FilterFlagDrop, FormatRGBA8, 1}}},
	} {
		f := layer(c.fs...)
		steps, passes := filterSteps(f)
		if len(steps) != len(c.want) {
			t.Errorf("%s: %d steps, want %d: %q", c.name, len(steps), len(c.want), passesOf(f))
			continue
		}
		prev := f.Passes[0].Target // the layer
		for i, s := range steps {
			h := head(f, s)
			w := c.want[i]
			format := targetOf(t, f, passes[i].Target).Format
			if h[0] != w.kind || h[1] != w.flags || format != w.format || slotInts(f, s, FilterSlotPost)[1] != int32(w.post) {
				t.Errorf("%s, step %d: kind %d, flags %#x, format %d, %d matrices; want %+v", c.name, i, h[0], h[1], format, slotInts(f, s, FilterSlotPost)[1], w)
			}
			// Each step reads the one before it, and a drop shadow's last
			// step the layer too.
			b := passes[i].Batches[0]
			wantSrc := [3]Source{Source(prev)}
			if w.flags&FilterFlagDrop != 0 {
				wantSrc[1] = Source(f.Passes[0].Target)
			}
			if b.Sources != wantSrc {
				t.Errorf("%s, step %d reads %v, want %v", c.name, i, b.Sources, wantSrc)
			}
			if w.kind == FilterTaps && h[3] == 0 {
				t.Errorf("%s, step %d has no taps", c.name, i)
			}
			prev = passes[i].Target
		}
		// The composite reads the last step.
		last := f.Passes[len(f.Passes)-1]
		if last.Target != 0 || last.Batches[0].Sources[0] != Source(prev) {
			t.Errorf("%s: the composite reads %v, the last step is the target %d", c.name, last.Batches[0].Sources, prev)
		}
	}
	// The layer of a filter holds its reach beyond the pixels composited,
	// and each step what the next reads.
	f := layer(gfx.Blur{StdDev: 3})
	steps, passes := filterSteps(f)
	surf := f.Passes[0].Clear.Rect
	comp := f.Passes[len(f.Passes)-1].Batches[0].Scissor
	reach := comp.Min.Sub(surf.Min)
	if reach.X != reach.Y || reach.X < 6 || reach.X > 9 || surf.Max.Sub(comp.Max) != reach {
		t.Errorf("blur 3: the layer holds %v for %v", surf, comp)
	}
	hr, vr := passes[0].Batches[0].Scissor, passes[1].Batches[0].Scissor
	if vr != comp || hr.Min.X != comp.Min.X || hr.Max.X != comp.Max.X || hr.Min.Y != surf.Min.Y || hr.Max.Y != surf.Max.Y {
		t.Errorf("blur 3: the steps draw %v then %v, for %v of a layer %v", hr, vr, comp, surf)
	}
	if r := slotInts(f, steps[0], FilterSlotSrcRect); image.Rect(int(r[0]), int(r[1]), int(r[2]), int(r[3])) != surf {
		t.Errorf("blur 3: the first step's source holds %v, the layer %v", r, surf)
	}
	// A blur that changes nothing is no step.
	if f := layer(gfx.Blur{StdDev: 0.001}); len(f.Passes) != 2 || len(f.Targets) != 1 {
		t.Errorf("a blur too small to show: passes %q", passesOf(f))
	}
}

// TestEncodeBackdrop checks a backdrop filter: the pass of the surface
// ends, the filters read the surface, mirrored at the shape's bounds, and
// the surface's pass goes on with the paint. A region that the filters
// read beyond is rendered apart, in a target the destination is copied
// from.
func TestEncodeBackdrop(t *testing.T) {
	shape := gfx.NewRRect(gfx.RectXYWH(40, 30, 100, 80), 12)
	draw := func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 200, 150), slate)
		b.BackdropFilterOpacity(shape, 0.5, gfx.Blur{StdDev: 3}, gfx.Saturate(2))
		b.FillRect(gfx.RectXYWH(50, 40, 20, 20), rose)
	}
	f := encode(200, 150, Options{Clear: &gfx.White}, draw)
	checkFrame(t, "backdrop", f)
	if got := passesOf(f); len(got) != 4 || got[0] != "dest+clear quad" || got[3] != "dest quad quad" {
		t.Fatalf("backdrop: passes %q", got)
	}
	steps, passes := filterSteps(f)
	if len(steps) != 2 || passes[0].Batches[0].Sources != [3]Source{SourceDest} {
		t.Fatalf("backdrop: %d steps, the first reads %v", len(steps), passes[0].Batches[0].Sources)
	}
	h := head(f, steps[0])
	if h[1]&FilterFlagMirror == 0 || slotInts(f, steps[0], FilterSlotMirror) != [4]int32{40, 30, 100, 80} {
		t.Errorf("backdrop: the first step has the flags %#x and mirrors at %v", h[1], slotInts(f, steps[0], FilterSlotMirror))
	}
	if r := slotInts(f, steps[0], FilterSlotSrcRect); r[0] >= 40 || r[1] >= 30 || r[2] <= 140 || r[3] <= 110 {
		t.Errorf("backdrop: the backdrop holds %v: not beyond the shape", r)
	}
	if head(f, steps[1])[1]&FilterFlagMirror != 0 || slotInts(f, steps[1], FilterSlotPost)[1] != 1 {
		t.Errorf("backdrop: the second step: flags %#x, %d matrices", head(f, steps[1])[1], slotInts(f, steps[1], FilterSlotPost)[1])
	}
	paint := f.Passes[3].Batches[0]
	if paint.Count != 1 || paint.Sources != [3]Source{Source(passes[1].Target)} || paint.Scissor != image.Rect(40, 30, 140, 110) {
		t.Errorf("backdrop: the paint %+v", paint)
	}
	if lw := head(f, paint.First)[1]; lw != 128|1<<24 {
		t.Errorf("backdrop: the paint's layer word is %#x, want the opacity 128 scaling the colors", lw)
	}
	// Without a filter that shows, the backdrop is copied once.
	f = encode(200, 150, Options{}, func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 200, 150), slate)
		b.BackdropFilter(shape, gfx.Blur{StdDev: 0.001})
	})
	if steps, _ := filterSteps(f); len(steps) != 1 || head(f, steps[0])[0] != FilterCopy {
		t.Errorf("a backdrop filter that changes nothing: passes %q", passesOf(f))
	}

	// A region inside the shape: the filter reads beyond it, so the grown
	// region is rendered in a target of its own, cleared like the region,
	// and the region alone is copied to the destination.
	region := image.Rect(80, 60, 100, 80)
	f = encode(200, 150, Options{Clear: &gfx.White, Region: region}, draw)
	checkFrame(t, "backdrop region", f)
	first, last := &f.Passes[0], &f.Passes[len(f.Passes)-1]
	grown := raster.BackdropRegion(gfxtest.Build(draw), 1, region, image.Rect(0, 0, 200, 150))
	if grown == region || first.Target == 0 || first.Clear == nil || first.Clear.Rect != grown || first.Origin != grown.Min {
		t.Fatalf("backdrop region: the first pass %q clears %+v at %v; the grown region is %v", passOf(first), first.Clear, first.Origin, grown)
	}
	for i := range f.Passes[:len(f.Passes)-1] {
		if f.Passes[i].Target == 0 {
			t.Errorf("backdrop region: pass %d draws into the destination before the copy", i)
		}
	}
	cb := last.Batches[0]
	if last.Target != 0 || len(last.Batches) != 1 || cb.Pipeline != PipelineFilter || cb.Scissor != region || cb.Sources != [3]Source{Source(first.Target)} || head(f, cb.First)[0] != FilterCopy {
		t.Errorf("backdrop region: the last pass %+v", last)
	}
	// The filter computes the region, not all of the grown one.
	_, passes = filterSteps(f)
	if got := passes[1].Batches[0].Scissor; got != region {
		t.Errorf("backdrop region: the filter's last step draws %v, want the region %v", got, region)
	}
	// Without a clear, the target starts from the destination's pixels.
	f = encode(200, 150, Options{Region: region}, draw)
	checkFrame(t, "backdrop region, no clear", f)
	if b := f.Passes[0].Batches[0]; f.Passes[0].Clear != nil || b.Pipeline != PipelineFilter || b.Sources != [3]Source{SourceDest} || b.Scissor != grown {
		t.Errorf("backdrop region, no clear: the first pass %+v", f.Passes[0])
	}
	// A region the filter does not reach grows nothing.
	f = encode(200, 150, Options{Region: image.Rect(150, 120, 200, 150)}, draw)
	if got := passesOf(f); len(got) != 1 || got[0] != "dest quad" {
		t.Errorf("a region away from the filter: passes %q", got)
	}
}

// TestEncodeLayerFlush checks that passes go on in their target after an
// atlas is emptied in the middle of a layer: the layer's target is cleared
// once, by its first pass, and every instance reads its pictures as they
// are when it is drawn.
func TestEncodeLayerFlush(t *testing.T) {
	dl := gfxtest.Build(func(b *gfx.Builder) {
		text(b, "SoAl", 30, 4, 30, slate)
		b.PushLayer(0.5, gfx.SrcOver)
		for i := 0; i < 6; i++ {
			text(b, "SoAli", 22+2*float32(i), 4, 60+26*float32(i), indigo)
		}
		b.PopLayer()
		text(b, "ioS", 30, 100, 30, rose)
	})
	size := image.Pt(200, 220)
	// The same instances with room for everything.
	var roomy Encoder
	want := (&atlasSim{t: t}).draw(roomy.Encode(dl, size, Options{}))
	e := Encoder{Limits: Limits{CoveragePage: 64, CoveragePages: 1}}
	f := e.Encode(dl, size, Options{})
	checkFrame(t, "flush", f)
	if len(f.Unsupported) != 0 {
		t.Fatalf("not drawn: %v", f.Unsupported)
	}
	clears, inLayer := 0, 0
	var layer int
	for i := range f.Passes {
		p := &f.Passes[i]
		if p.Target == 0 {
			if p.Clear != nil {
				t.Errorf("pass %d clears the destination", i)
			}
			continue
		}
		if layer == 0 {
			layer = p.Target
		}
		if p.Target != layer {
			t.Fatalf("pass %d draws into the target %d, the layer is %d", i, p.Target, layer)
		}
		inLayer++
		if p.Clear != nil {
			clears++
			if inLayer != 1 {
				t.Errorf("pass %d clears the layer again", i)
			}
		}
	}
	if inLayer < 2 || clears != 1 {
		t.Fatalf("%d passes into the layer, %d of them clear it; want several, the first clearing: %q", inLayer, clears, passesOf(f))
	}
	// Every instance reads the pixels it reads with room.
	got := (&atlasSim{t: t}).draw(f)
	if len(got) != len(want) {
		t.Fatalf("%d instances, %d with room", len(got), len(want))
	}
	for i := range got {
		if fmt.Sprint(got[i]) != fmt.Sprint(want[i]) {
			t.Fatalf("instance %d reads other pixels after a flush", i)
		}
	}
}

// TestTargetPool checks the pool of offscreen targets: sizes in steps,
// reuse within a frame and across frames, what idle targets and a budget
// give up.
func TestTargetPool(t *testing.T) {
	for _, c := range [][3]int{{1, 4096, 64}, {64, 4096, 64}, {65, 4096, 72}, {100, 4096, 104}, {1000, 4096, 1024}, {2160, 4096, 2304}, {3840, 4096, 3840},
		{4000, 4096, 4096}, {4090, 4096, 4096}, {3, 4, 4}, {50, 40, 50}} {
		if got := bucketDim(c[0], c[1]); got != c[2] || got < c[0] {
			t.Errorf("bucketDim(%d, %d) = %d, want %d", c[0], c[1], got, c[2])
		}
	}
	layer := func(w, h float32, fs ...gfx.Filter) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) {
			b.PushLayer(0.5, gfx.SrcOver, fs...)
			b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10, 10, w, h), 4), rose) // (a rectangle alone needs no target)
			b.PopLayer()
		})
	}
	size := image.Pt(800, 600)
	var e Encoder
	f := e.Encode(layer(300, 200), size, Options{})
	if len(f.Targets) != 1 || f.Targets[0].Width < 302 || f.Targets[0].Width > 340 || f.Targets[0].Height < 202 || f.Targets[0].Height > 230 {
		t.Fatalf("a layer of 302x202: targets %+v", f.Targets)
	}
	first := f.Targets[0]
	// A layer a little smaller, frame after frame, keeps the target.
	for i := 0; i < 3*idleFrames; i++ {
		f = e.Encode(layer(300-float32(i%8), 200), size, Options{})
		e.EndFrame()
		if len(f.Targets) != 1 || f.Targets[0] != first {
			t.Fatalf("frame %d: targets %+v, want %+v", i, f.Targets, first)
		}
	}
	// Another size is another target; the first one goes when no frame
	// used it for a while, and comes back under a new ID.
	f = e.Encode(layer(600, 400), size, Options{})
	if len(f.Targets) != 2 {
		t.Fatalf("a second size: targets %+v", f.Targets)
	}
	for i := 0; i <= idleFrames; i++ {
		e.EndFrame()
		f = e.Encode(layer(600, 400), size, Options{})
	}
	e.EndFrame()
	if len(f.Targets) != 1 || f.Targets[0].ID == first.ID {
		t.Fatalf("after %d frames without the first layer: targets %+v", idleFrames, f.Targets)
	}
	f = e.Encode(layer(300, 200), size, Options{})
	if len(f.Targets) != 2 || f.Targets[1].ID == first.ID || f.Targets[1].Width != first.Width {
		t.Errorf("the first layer again: targets %+v", f.Targets)
	}
	// Frames without layers give everything up, and Trim at once.
	for i := 0; i <= idleFrames+1; i++ {
		e.Encode(layer(0, 0), size, Options{})
		e.EndFrame()
	}
	if f = e.Encode(layer(0, 0), size, Options{}); len(f.Targets) != 0 || e.res.targetBytes != 0 {
		t.Errorf("after %d frames without layers: targets %+v, %d bytes", idleFrames, f.Targets, e.res.targetBytes)
	}
	e.Encode(layer(300, 200, gfx.Blur{StdDev: 12}), size, Options{})
	if len(e.res.targets) < 3 {
		t.Fatalf("a blurred layer: %d targets", len(e.res.targets))
	}
	e.Trim()
	if f = e.Encode(layer(0, 0), size, Options{}); len(f.Targets) != 0 || f.Res.Base != 0 {
		t.Errorf("after Trim: targets %+v, resources %+v", f.Targets, f.Res)
	}

	// A budget: targets of other frames go as soon as it is exceeded, the
	// oldest first; a frame that needs more keeps what it draws with.
	e = Encoder{Limits: Limits{TargetBytes: 1 << 20}}
	var held int64
	for i := 0; i < 12; i++ {
		f = e.Encode(layer(float32(100+60*i), 300), size, Options{})
		e.EndFrame()
		held = 0
		for _, info := range f.Targets {
			held += int64(4 * info.Width * info.Height)
		}
		if held > 1<<20+int64(4*f.Targets[len(f.Targets)-1].Width*f.Targets[len(f.Targets)-1].Height) {
			t.Fatalf("frame %d holds %d bytes of targets with a budget of 1 MiB: %+v", i, held, f.Targets)
		}
	}
	if len(f.Targets) > 2 {
		t.Errorf("after 12 layers of growing sizes: %d targets", len(f.Targets))
	}
	f = e.Encode(layer(700, 500, gfx.Blur{StdDev: 2}, gfx.Blur{StdDev: 3}), size, Options{})
	checkFrame(t, "over the budget", f)
	if len(f.Unsupported) != 0 || len(f.Passes) < 5 {
		t.Errorf("a frame over the budget: unsupported %v, passes %q", f.Unsupported, passesOf(f))
	}
}

// blurLine applies a blurAxis to a line of device pixels, in float64, as
// the passes of the filter pipeline do: the pixels outside the line are
// transparent.
func blurLine(f *Frame, a *blurAxis, in []float64) []float64 {
	at := func(i int) float64 {
		if i < 0 || i >= len(in) {
			return 0
		}
		return in[i]
	}
	tap := func(r tapRange, k int) (int, float64) {
		return int(f.Taps[2*(r.first+k)]), float64(f.Taps[2*(r.first+k)+1])
	}
	out := make([]float64, len(in))
	if a.ident {
		copy(out, in)
		return out
	}
	if a.g == 1 {
		for i := range out {
			for k := 0; k < a.taps.n; k++ {
				t, w := tap(a.taps, k)
				out[i] += w * at(i+t)
			}
		}
		return out
	}
	g := a.g
	blocks, lows := map[int]float64{}, map[int]float64{}
	block := func(b int) float64 {
		v, ok := blocks[b]
		if !ok {
			for k := 0; k < a.pre.n; k++ {
				t, w := tap(a.pre, k)
				v += w * at(b*g+t)
			}
			blocks[b] = v
		}
		return v
	}
	low := func(b int) float64 {
		v, ok := lows[b]
		if !ok {
			for k := 0; k < a.taps.n; k++ {
				t, w := tap(a.taps, k)
				v += w * block(b+t)
			}
			lows[b] = v
		}
		return v
	}
	for x := range out {
		t := 2*(x-a.sh) + 1 - g
		i := floorDiv(t, 2*g)
		n := float64(t - 2*g*i)
		out[x] = ((float64(2*g)-n)*low(i) + n*low(i+1)) / float64(2*g)
	}
	return out
}

// TestBlurKernels checks the kernels of the blurs against the
// rasterizer's, without a device: every kernel stays within the reach the
// display list promises, and what the passes compute along a line (hard
// edges, single pixels, stripes of one to four pixels, noise) is the
// rasterizer's blur within a level and a half, whatever the size and the
// place of the line.
func TestBlurKernels(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	const n = 2600
	in := make([]float64, n)
	for i := range in {
		switch seg := i / 200; {
		case seg%4 == 0: // edges
			if i%200 > 100 {
				in[i] = 255
			}
		case seg%4 == 1: // stripes of 1 to 4 pixels
			if w := 1 + seg/4; (i/w)%2 == 0 {
				in[i] = 255
			}
		case seg%4 == 2: // single pixels and noise
			if i%37 == 0 {
				in[i] = 255
			} else if i%200 > 120 {
				in[i] = float64(rng.Intn(256))
			}
		default: // a period near the blocks' size
			if i%5 < 2 {
				in[i] = 255
			}
		}
	}
	var e Encoder
	e.Encode(nil, image.Pt(1, 1), Options{})
	worstAll, worstFull := 0.0, 0.0
	for _, s := range []float64{0.2, 0.5, 1, 1.9, 2, 2.7, 3.5, 5, 5.9, 6, 6.4, 7, 7.9, 8, 9.3, 11.9, 12, 14, 15.9, 16, 19, 20.9, 21, 24, 31.9, 32, 40, 47.9, 48, 64, 90, 128, 200, 256} {
		for _, off := range []float64{0, 3.3, -7.5, 60} {
			a := e.kernel(s, off)
			k := raster.NewBlurKernel(s, off)
			if lim := int(math.Floor(k.Limit)); a.left > lim || a.right > lim {
				t.Errorf("sigma %g, offset %g: the kernel reads %d before and %d after a pixel, beyond the reach %d", s, off, a.left, a.right, lim)
			}
			if a.ident != k.Identity() {
				t.Errorf("sigma %g, offset %g: identity %v, the rasterizer's %v", s, off, a.ident, k.Identity())
			}
			// Without an offset every deviation from 2*lowSigma on is
			// reduced. (With one, the block size whose kernel would be cut
			// too much to fit the reach is not used.)
			if off == 0 && k.Variance > 0 && (a.g > 1) != (s >= 2*lowSigma) {
				t.Errorf("sigma %g: blocks of %d", s, a.g)
			}
			if a.g > 1 && (s/float64(a.g) < lowSigma || a.g > maxBlock) {
				t.Errorf("sigma %g: blocks of %d leave a deviation of %g blocks", s, a.g, s/float64(a.g))
			}
			for _, r := range []tapRange{a.taps, a.pre} {
				var sum float64
				for i := 0; i < r.n; i++ {
					sum += float64(e.frame.Taps[2*(r.first+i)+1])
				}
				if r.n > 0 && math.Abs(sum-1) > 1e-5 {
					t.Errorf("sigma %g, offset %g: weights add up to %v", s, off, sum)
				}
			}
			for _, ox := range []int{0, 3, 13} { // the line's place on the blocks' grid
				line := in[ox:]
				got := blurLine(&e.frame, &a, line)
				want := applyRasterKernel(&k, line)
				worst, at := 0.0, 0
				sup := max(k.Left, k.Right, a.left, a.right)
				for i := sup; i < len(line)-sup; i++ {
					if d := math.Abs(got[i] - want[i]); d > worst {
						worst, at = d, i
					}
				}
				worstAll = max(worstAll, worst)
				// Where the rasterizer blurs at full resolution, its
				// kernel is three boxes exactly, and the passes here are
				// within 0.6 level of it; where it reduces too, its own
				// blocks differ from that by as much again.
				limit := 1.5
				switch {
				case k.Down == 1 && a.g == 1:
					limit = 1e-3 // the same kernel (in float32)
				case k.Down == 1:
					limit = 0.6
					worstFull = max(worstFull, worst)
				}
				if worst > limit {
					t.Errorf("sigma %g, offset %g, line at %d: %.2f levels from the rasterizer's blur at pixel %d (blocks of %d, the rasterizer's %d)", s, off, ox, worst, at, a.g, k.Down)
				}
			}
		}
	}
	t.Logf("largest difference from the rasterizer's kernels: %.2f levels; %.2f where it blurs at full resolution", worstAll, worstFull)
}

// applyRasterKernel computes what a raster.BlurKernel says the rasterizer
// computes along a line that starts at device pixel 0 (raster's
// TestBlurKernel checks it against the blur itself).
func applyRasterKernel(k *raster.BlurKernel, in []float64) []float64 {
	at := func(i int) float64 {
		if i < 0 || i >= len(in) {
			return 0
		}
		return in[i]
	}
	out := make([]float64, len(in))
	if k.Down <= 1 {
		for i := range out {
			for t, w := range k.W {
				out[i] += w * at(i+k.Lo+t)
			}
		}
		return out
	}
	f := k.Down
	block := func(b int) float64 {
		var s float64
		for x := b * f; x < b*f+f; x++ {
			s += at(x)
		}
		return s / float64(f)
	}
	low := map[int]float64{}
	lowAt := func(b int) float64 {
		v, ok := low[b]
		if !ok {
			for t, w := range k.W {
				v += w * block(b+k.Lo+t)
			}
			low[b] = v
		}
		return v
	}
	for x := range out {
		t := 2*(x-k.Shift) + 1 - f
		i := floorDiv(t, 2*f)
		n := float64(t - 2*f*i)
		out[x] = ((float64(2*f)-n)*lowAt(i) + n*lowAt(i+1)) / float64(2*f)
	}
	return out
}
