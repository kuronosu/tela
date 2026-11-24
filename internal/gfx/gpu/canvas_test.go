package gpu

import (
	"image"
	"math"
	"math/rand"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The tests of the Porter-Duff operators and of image patterns (ADR-319)
// without a device: the passes and the instances of their frames, what they
// hide and read, and the textures of the patterns' images. The pixels are
// compared on a device (scenes4_test.go, canvas_device_test.go).

// TestOperatorsMatchShader checks the operators' numbers, the set of those
// that change the backdrop where the source is transparent, and the bits of
// a pattern in quad.frag against Go's.
func TestOperatorsMatchShader(t *testing.T) {
	src, err := os.ReadFile("shaders/quad.frag")
	if err != nil {
		t.Fatal(err)
	}
	number := func(name string) string {
		m := regexp.MustCompile(`const uint ` + name + ` = (.*);`).FindStringSubmatch(string(src))
		if m == nil {
			t.Fatalf("quad.frag has no constant %s", name)
		}
		return m[1]
	}
	names := map[gfx.BlendMode]string{gfx.SrcIn: "BLEND_SRC_IN", gfx.SrcOut: "BLEND_SRC_OUT", gfx.SrcAtop: "BLEND_SRC_ATOP",
		gfx.DstOver: "BLEND_DST_OVER", gfx.DstIn: "BLEND_DST_IN", gfx.DstOut: "BLEND_DST_OUT", gfx.DstAtop: "BLEND_DST_ATOP",
		gfx.Copy: "BLEND_COPY", gfx.Xor: "BLEND_XOR"}
	// The sets of the shader, as the shader writes them.
	sets := map[string][]gfx.BlendMode{}
	for _, m := range operators {
		if got, want := number(names[m]), itoa(int(m))+"u"; got != want {
			t.Errorf("%s is %s in quad.frag, want %s", names[m], got, want)
		}
		if m.Unbounded() {
			sets["UNBOUNDED"] = append(sets["UNBOUNDED"], m)
		}
		// The factors, from the definition of the operators.
		fa := map[gfx.BlendMode]string{gfx.SrcIn: "ALPHA", gfx.SrcOut: "REST", gfx.SrcAtop: "ALPHA", gfx.DstOver: "REST", gfx.DstAtop: "REST", gfx.Copy: "ONE", gfx.Xor: "REST"}[m]
		fb := map[gfx.BlendMode]string{gfx.SrcAtop: "REST", gfx.DstOver: "ONE", gfx.DstIn: "ALPHA", gfx.DstOut: "REST", gfx.DstAtop: "ALPHA", gfx.Xor: "REST"}[m]
		if fa != "" {
			sets["FA_"+fa] = append(sets["FA_"+fa], m)
		}
		if fb != "" {
			sets["FB_"+fb] = append(sets["FB_"+fb], m)
		}
	}
	for set, modes := range sets {
		var want []string
		for _, m := range modes {
			want = append(want, "1u << "+names[m])
		}
		if got := number(set); got != strings.Join(want, " | ") {
			t.Errorf("%s is %s in quad.frag, want %s", set, got, strings.Join(want, " | "))
		}
	}
	for name, v := range map[string]int{"PATTERN": FlagPattern, "REPEAT_X": FlagRepeatX, "REPEAT_Y": FlagRepeatY} {
		if got, want := number(name), itoa(v)+"u"; got != want {
			t.Errorf("%s is %s in quad.frag, want %s", name, got, want)
		}
	}
	if gfx.SrcIn <= gfx.PlusLighter {
		t.Errorf("the shader takes the modes after %s for operators", gfx.PlusLighter)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append(b, byte('0'+n%10))
	}
	slices.Reverse(b)
	return string(b)
}

// boundsOf returns the quad of instance i.
func boundsOf(f *Frame, i int) image.Rectangle {
	b := floats(f, i, SlotBounds)
	return image.Rect(int(b[0]), int(b[1]), int(b[2]), int(b[3]))
}

// TestEncodeOperators checks the frame of a layer with each operator: its
// content, a copy of what is under it and its composite, over the whole clip
// for an unbounded one, and in a region over the clip inside the region.
func TestEncodeOperators(t *testing.T) {
	size := image.Pt(120, 100)
	clip := image.Rect(10, 20, 90, 80)
	for _, m := range operators {
		dl := gfxtest.Build(func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 60, 60), sky)
			b.PushClipRect(gfx.RectXYWH(10, 20, 80, 60))
			b.PushLayer(0.6, m)
			b.FillRect(gfx.RectXYWH(40, 40, 10, 10), rose)
			b.PopLayer()
			b.PopClip()
		})
		for _, region := range []image.Rectangle{{}, image.Rect(0, 0, 45, 50), image.Rect(60, 60, 70, 70), image.Rect(0, 0, 8, 8)} {
			var e Encoder
			f := e.Encode(dl, size, Options{Clear: &gfx.White, Region: region})
			checkFrame(t, m.String(), f)
			if len(f.Unsupported) != 0 {
				t.Fatalf("%s: unsupported %q", m, f.Unsupported)
			}
			// The pixels the layer can change, with the margin of a pixel of a
			// layer's bounds, inside the clip.
			want := clip
			if !m.Unbounded() {
				want = image.Rect(39, 39, 51, 51)
			}
			if !region.Empty() {
				want = want.Intersect(region)
			}
			var got image.Rectangle
			for pi := range f.Passes {
				for _, b := range f.Passes[pi].Batches {
					if b.Pipeline != PipelineBlend {
						continue
					}
					if f.Passes[pi].Target != 0 || b.Sources[0] == 0 || b.Sources[2] == 0 {
						t.Errorf("%s, region %v: composited in the target %d from %v", m, region, f.Passes[pi].Target, b.Sources)
					}
					for i := b.First; i < b.First+b.Count; i++ {
						got = got.Union(boundsOf(f, i).Intersect(b.Scissor))
						if h := head(f, i)[1]; h&255 != 153 || h>>8&255 != uint32(m) {
							t.Errorf("%s: head %#x", m, h)
						}
					}
				}
			}
			if got != want {
				t.Errorf("%s, region %v: composited over %v, want %v (passes %q)", m, region, got, want, passesOf(f))
			}
		}
	}
}

// TestEncodeEmptyOperators checks the operators over a layer that shows
// nothing: an unbounded one clears its clip, with an instance that replaces
// the pixels when the clip falls on their boundaries, else with a copy of
// what is under it and nothing to read as its source. The others do nothing.
func TestEncodeEmptyOperators(t *testing.T) {
	size := image.Pt(120, 100)
	empty := []struct {
		name string
		draw func(b *gfx.Builder, m gfx.BlendMode)
	}{
		{"opacity 0", func(b *gfx.Builder, m gfx.BlendMode) {
			b.PushLayer(0, m, gfx.Blur{StdDev: 4})
			b.FillRect(gfx.RectXYWH(40, 40, 10, 10), rose)
			b.PushLayer(1, gfx.Xor)
			b.FillRect(gfx.RectXYWH(40, 40, 10, 10), rose)
			b.PopLayer()
			b.PopLayer()
		}},
		{"an opacity that rounds to 0", func(b *gfx.Builder, m gfx.BlendMode) {
			b.PushLayer(0.001, m, gfx.Blur{StdDev: 4})
			b.FillRect(gfx.RectXYWH(40, 40, 10, 10), rose)
			b.PopLayer()
		}},
		{"no content", func(b *gfx.Builder, m gfx.BlendMode) { b.PushLayer(1, m); b.PopLayer() }},
		{"an empty mask", func(b *gfx.Builder, m gfx.BlendMode) {
			b.PushMaskLayer(gfx.Mask{}, 1, m)
			b.FillRect(gfx.RectXYWH(40, 40, 10, 10), rose)
			b.PopLayer()
		}},
	}
	for _, c := range empty {
		for _, m := range operators {
			frame := func(e *Encoder, clip gfx.RRect, region image.Rectangle) *Frame {
				f := e.Encode(gfxtest.Build(func(b *gfx.Builder) {
					b.FillRect(gfx.RectXYWH(0, 0, 60, 60), sky)
					b.PushClipRRect(clip)
					c.draw(b, m)
					b.PopClip()
					b.FillRect(gfx.RectXYWH(0, 0, 20, 20), rose)
				}), size, Options{Clear: &gfx.White, Region: region})
				checkFrame(t, c.name, f)
				if len(f.Unsupported) != 0 {
					t.Fatalf("%s, %s: unsupported %q", c.name, m, f.Unsupported)
				}
				return f
			}
			var e Encoder
			aligned := gfx.RRect{Rect: gfx.RectXYWH(10, 20, 80, 60)}
			f := frame(&e, aligned, image.Rectangle{})
			if !m.Unbounded() {
				if instances(f) != 2 || len(f.Passes) != 1 {
					t.Errorf("%s, %s: %d instances, passes %q", c.name, m, instances(f), passesOf(f))
				}
				continue
			}
			// One pass, no target, no copy.
			if got := passesOf(f); !slices.Equal(got, []string{"dest+clear opaque opaque opaque"}) || len(f.Targets) != 0 {
				t.Errorf("%s, %s over an aligned clip: passes %q, %d targets", c.name, m, got, len(f.Targets))
			}
			if instances(f) != 3 || boundsOf(f, 1) != image.Rect(10, 20, 90, 80) || head(f, 1) != [4]uint32{FlagFull, 0, 0, 0} {
				t.Errorf("%s, %s over an aligned clip: %d instances, %v, head %v", c.name, m, instances(f), boundsOf(f, 1), head(f, 1))
			}
			if f = frame(&e, aligned, image.Rect(0, 0, 30, 30)); instances(f) != 3 || f.Passes[0].Batches[1].Scissor != image.Rect(10, 20, 30, 30) {
				t.Errorf("%s, %s in a region: %+v", c.name, m, f.Passes)
			}
			if f = frame(&e, aligned, image.Rect(0, 0, 9, 9)); instances(f) != 2 {
				t.Errorf("%s, %s in a region outside its clip: %d instances", c.name, m, instances(f))
			}
			// A clip with edges inside pixels.
			f = frame(&e, gfx.NewRRect(gfx.RectXYWH(10, 20, 80, 60), 20), image.Rectangle{})
			copied := SourceDest
			var comp *Batch
			for pi := range f.Passes {
				for bi := range f.Passes[pi].Batches {
					if b := &f.Passes[pi].Batches[bi]; b.Pipeline == PipelineBlend {
						comp = b
					}
				}
			}
			if comp == nil || comp.Sources[0] != 0 || comp.Sources[1] != 0 || comp.Sources[2] == 0 || len(f.Passes) != 3 || len(f.Targets) != 1 {
				t.Fatalf("%s, %s over a rounded clip: passes %q, %+v", c.name, m, passesOf(f), comp)
			}
			if f.Passes[1].Batches[0].Sources[0] != copied || f.Passes[1].Target != int(comp.Sources[2]) {
				t.Errorf("%s, %s: the pass before the composite is not the copy: %+v", c.name, m, f.Passes[1])
			}
			if h := head(f, comp.First); h[1]&255 != 0 || h[1]>>8&255 == 0 || boundsOf(f, comp.First).Union(boundsOf(f, comp.First+comp.Count-1)) != image.Rect(10, 20, 90, 80) {
				t.Errorf("%s, %s: the composite's head is %v, its quad %v", c.name, m, h, boundsOf(f, comp.First))
			}
			for _, tg := range e.res.targets {
				if tg.busy {
					t.Errorf("%s, %s: the target %d is still in use after the frame", c.name, m, tg.id)
				}
			}
		}
	}
	// Larger than the device's textures: named, not drawn.
	e := Encoder{Limits: Limits{MaxTextureSize: 64}}
	f := e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 100, 100), 20))
		b.PushLayer(0, gfx.Copy)
		b.PopLayer()
		b.PopClip()
	}), size, Options{})
	if want := []string{"PushLayer (a layer larger than the device's textures)"}; !slices.Equal(f.Unsupported, want) {
		t.Errorf("a clip larger than the textures: unsupported %q, want %q", f.Unsupported, want)
	}
}

// TestCullOperators checks that an operator keeps what it reads, over all of
// its clip, and that it is hidden like any layer where a cover after it lies
// over all it changes.
func TestCullOperators(t *testing.T) {
	size := image.Pt(200, 200)
	whole := gfx.RectXYWH(0, 0, 200, 200)
	for _, c := range []struct {
		name   string
		draw   func(b *gfx.Builder)
		hidden int
	}{
		{"an unbounded operator whose source is elsewhere", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.PushLayer(1, gfx.DstIn)
			b.FillRect(gfx.RectXYWH(150, 150, 10, 10), rose)
			b.PopLayer()
			b.FillRect(whole, slate)
		}, 0},
		{"one that is not unbounded", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.PushLayer(1, gfx.DstOut)
			b.FillRect(gfx.RectXYWH(150, 150, 10, 10), rose)
			b.PopLayer()
			b.FillRect(whole, slate)
		}, 1},
		{"one clipped beside the fill", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.PushClipRect(gfx.RectXYWH(130, 0, 70, 200))
			b.PushLayer(1, gfx.Copy)
			b.PopLayer()
			b.PopClip()
			b.FillRect(whole, slate)
		}, 1},
		{"a cover before an operator", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.FillRect(whole, slate) // hides it, and is read
			b.PushLayer(0, gfx.Copy)
			b.PopLayer()
		}, 1},
		{"an opaque pattern", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.FillRect(whole, photoPattern(gfx.Affine{0.3, 0.1, -0.2, 0.4, 5, 6}))
		}, 1},
		{"an opaque pattern in a rounded rectangle", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.FillRRect(gfx.NewRRect(whole, 15), photoPattern(gfx.Identity))
		}, 1},
		{"a pattern with translucent pixels", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.FillRect(whole, tilePattern(gfx.RepeatBoth, gfx.SampleLinear, gfx.Identity))
		}, 0},
		{"a pattern that repeats along one axis", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.FillRect(whole, gfx.ImagePattern{Image: photo, Repeat: gfx.RepeatY, Transform: gfx.Scale(4, 4)})
		}, 0},
		{"a pattern that does not repeat", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.FillRect(whole, gfx.ImagePattern{Image: photo, Repeat: gfx.RepeatNone, Transform: gfx.Scale(4, 4)})
		}, 0},
		{"a pattern whose mapping is out of range", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			b.FillRect(whole, gfx.ImagePattern{Image: photo, Sampling: gfx.SampleNearest, Transform: gfx.Scale(1e-38, 1)})
		}, 0},
		{"a pattern as a path", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(20, 20, 100, 100), rose)
			var p gfx.Path
			p.AddRRect(gfx.NewRRect(whole, 10))
			b.FillPath(p, gfx.NonZero, photoPattern(gfx.Identity))
		}, 0},
	} {
		var e Encoder
		f := e.Encode(gfxtest.Build(c.draw), size, Options{Clear: &gfx.White})
		checkFrame(t, c.name, f)
		if e.culled != c.hidden || len(f.Unsupported) != 0 {
			t.Errorf("%s: %d commands hidden, want %d (passes %q, unsupported %q)", c.name, e.culled, c.hidden, passesOf(f), f.Unsupported)
		}
	}
	// A cover after the operator hides what both lie under, as it hides the
	// composite: the pixels it covers are not cleared.
	var e Encoder
	f := e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.PushLayer(0, gfx.Copy)
		b.PopLayer()
		b.FillRect(gfx.RectXYWH(0, 0, 200, 150), slate)
	}), size, Options{Clear: &gfx.White})
	if instances(f) != 2 || boundsOf(f, 0) != image.Rect(0, 150, 200, 200) || boundsOf(f, 1) != image.Rect(0, 0, 200, 150) {
		t.Errorf("a cover over a cleared frame: %d instances, %v and %v", instances(f), boundsOf(f, 0), boundsOf(f, 1))
	}
}

// TestOpaqueAt checks what a frame drawn over an opaque color knows of its
// pixels under a glass pane: an operator before the pane that reaches it may
// have left them translucent, so its cached backdrop is blended and covers
// nothing.
func TestOpaqueAt(t *testing.T) {
	size := image.Pt(200, 200)
	pane := gfx.RRect{Rect: gfx.RectXYWH(20, 20, 100, 100)}
	op := func(m gfx.BlendMode, r gfx.Rect) func(b *gfx.Builder) {
		return func(b *gfx.Builder) {
			b.PushClipRect(r)
			b.PushLayer(1, m)
			b.FillRect(r, gfx.Black.WithAlpha(0.5))
			b.PopLayer()
			b.PopClip()
		}
	}
	under, beside := gfx.RectXYWH(60, 60, 20, 20), gfx.RectXYWH(150, 10, 40, 150)
	type test struct {
		name          string
		before, after func(b *gfx.Builder)
		want          bool
	}
	tests := []test{
		{"nothing", nil, nil, true},
		{"copy beside it", op(gfx.Copy, beside), nil, true},
		{"copy after it", nil, op(gfx.Copy, under), true},
		{"a clear under it", func(b *gfx.Builder) { b.PushClipRect(under); b.PushLayer(0, gfx.SrcIn); b.PopLayer(); b.PopClip() }, nil, false},
		{"destination-in without a clip, its source beside it", func(b *gfx.Builder) {
			b.PushLayer(1, gfx.DstIn)
			b.FillRect(beside, gfx.Black)
			b.PopLayer()
		}, nil, false},
		{"one inside a group", func(b *gfx.Builder) {
			b.PushLayer(0.5, gfx.SrcOver)
			op(gfx.Copy, under)(b)
			b.PopLayer()
		}, nil, true},
	}
	for _, m := range append([]gfx.BlendMode{gfx.Multiply, gfx.PlusLighter}, operators...) {
		want := m == gfx.Multiply || m == gfx.PlusLighter || m == gfx.SrcAtop || m == gfx.DstOver
		tests = append(tests, test{m.String() + " under it", op(m, under), nil, want})
		// Whether a mode keeps opaque pixels opaque, by its formula.
		img := image.NewRGBA(image.Rect(0, 0, 1, 1))
		raster.Render(img, gfxtest.Build(func(b *gfx.Builder) { op(m, gfx.RectXYWH(0, 0, 1, 1))(b) }), raster.Options{Clear: &gfx.White})
		if opaque := img.Pix[3] == 255; opaque == clearsOpaque(m) {
			t.Errorf("%s: an opaque pixel under half of black gets the alpha %d, clearsOpaque says %v", m, img.Pix[3], clearsOpaque(m))
		}
	}
	for _, c := range tests {
		dl := gfxtest.Build(func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 200, 200), slate)
			if c.before != nil {
				c.before(b)
			}
			b.BackdropFilter(pane, gfx.Blur{StdDev: 4})
			if c.after != nil {
				c.after(b)
			}
		})
		var e Encoder
		var bc BackdropCache
		e.Encode(dl, size, Options{Clear: &gfx.White, Backdrops: &bc})
		if e.glass.Stored != 1 || len(bc.ents) != 1 {
			t.Fatalf("%s: %+v", c.name, e.glass)
		}
		if bc.ents[0].opaque != c.want {
			t.Errorf("%s: the cached backdrop is opaque: %v, want %v", c.name, bc.ents[0].opaque, c.want)
		}
		// Painted from the entry: without blending only when it is opaque.
		bc.Retain(dl, 1, []int{0}, nil)
		f := e.Encode(dl, size, Options{Clear: &gfx.White, Backdrops: &bc})
		blended := true
		for pi := range f.Passes {
			for _, b := range f.Passes[pi].Batches {
				if b.Pipeline == PipelineOpaque && head(f, b.First)[0]&FlagLayer != 0 && f.Passes[pi].Target == 0 {
					blended = false
				}
			}
		}
		if e.glass.Hits != 1 || blended == c.want {
			t.Errorf("%s: from the cache, %+v, blended %v, want %v", c.name, e.glass, blended, !c.want)
		}
	}
}

// patternList is a list of one rectangle filled with each pattern.
func patternList(ps ...gfx.ImagePattern) *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) {
		for i, p := range ps {
			b.FillRect(gfx.RectXYWH(float32(10+60*i), 10, 50, 50), p)
		}
	})
}

// TestEncodePattern checks the instances of patterns: for each shape the
// image they read, the flags, and coordinates that the shader computes as the
// rasterizer does.
func TestEncodePattern(t *testing.T) {
	size := image.Pt(400, 300)
	tile := gfxtest.PatternTile
	for rep := gfx.RepeatBoth; rep <= gfx.RepeatNone; rep++ {
		for _, s := range []gfx.Sampling{gfx.SampleLinear, gfx.SampleNearest} {
			want := uint32(FlagPattern | PaintImageLinear<<FlagPaintShift)
			if s == gfx.SampleNearest {
				want = FlagPattern | PaintImageNearest<<FlagPaintShift
			}
			if rep.X() {
				want |= FlagRepeatX
			}
			if rep.Y() {
				want |= FlagRepeatY
			}
			p := tilePattern(rep, s, gfx.Scale(2, 3))
			for _, c := range []struct {
				name string
				draw func(b *gfx.Builder)
			}{
				{"FillRect", func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(10.5, 10, 50, 50), p) }},
				{"FillRRect", func(b *gfx.Builder) { b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10, 10, 200, 200), 8), p) }},
				{"FillPath", func(b *gfx.Builder) { b.FillPath(gfxtest.StarPath(150, 150, 120), gfx.NonZero, p) }},
				{"StrokePath", func(b *gfx.Builder) { b.StrokePath(gfxtest.StarPath(100, 100, 40), gfx.NewStrokeStyle(3), p) }},
				{"DrawGlyphRun", func(b *gfx.Builder) { text(b, "SoA", 20, 10.2, 50, p) }},
				{"large glyphs", func(b *gfx.Builder) { text(b, "So", 250, 10, 250, p) }},
			} {
				var e Encoder
				f := e.Encode(gfxtest.Build(c.draw), size, Options{})
				checkFrame(t, c.name, f)
				if len(f.Unsupported) != 0 || len(f.Images) != 1 || f.Images[0].ID != tile.ID() || instances(f) == 0 {
					t.Fatalf("%s: unsupported %q, %d images, %d instances", c.name, f.Unsupported, len(f.Images), instances(f))
				}
				for i := range instances(f) {
					if got := head(f, i)[0] & (7<<FlagPaintShift | 0xffff<<FlagSegmentsShift); got != want {
						t.Errorf("%s, %s, %s: paint flags %#x, want %#x", c.name, rep, s, got, want)
					}
				}
				for _, b := range batches(f) {
					if b.Image != tile.ID() || b.Pipeline != PipelineQuad {
						t.Errorf("%s: a batch of the image %d by the pipeline %d", c.name, b.Image, b.Pipeline)
					}
				}
			}
		}
	}
	// What is drawn after a pattern reads nothing, and color glyphs of a run
	// painted with one do not.
	var e Encoder
	f := e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.PushClipRect(gfx.RectXYWH(0, 0, 100, 100)) // batches of their own
		text(b, "S", 20, 10, 50, photoPattern(gfx.Identity))
		b.PopClip()
		b.PushClipRect(gfx.RectXYWH(100, 0, 100, 100))
		text(b, "*", 20, 110, 50, photoPattern(gfx.Identity))
		b.PopClip()
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10, 210, 50, 50), 8), slate)
	}), size, Options{})
	bs := batches(f)
	if len(bs) != 3 || bs[0].Image != photo.ID() || bs[1].Image != 0 || bs[2].Image != 0 {
		t.Errorf("a glyph, a color glyph and a fill: batches %+v", bs)
	}
}

// TestPatternCoordinates computes the image coordinates of patterns as the
// shader does, in float32, and checks them against the rasterizer's: never
// below 0 along an axis that repeats, and within a ten millionth of a pixel,
// times the size of the largest of the coordinates over the quad.
func TestPatternCoordinates(t *testing.T) {
	rng := rand.New(rand.NewSource(319))
	between := func(lo, hi float64) float32 { return float32(lo + rng.Float64()*(hi-lo)) }
	worst := 0.0
	for range 4000 {
		m := gfx.Affine{between(-3, 3), between(-3, 3), between(-3, 3), between(-3, 3), between(-2000, 2000), between(-2000, 2000)}
		p := gfx.ImagePattern{Image: gfxtest.PatternTile, Repeat: gfx.PatternRepeat(rng.Intn(4)), Sampling: gfx.Sampling(rng.Intn(2)),
			Transform: gfx.Affine{between(-2, 2), between(-2, 2), between(-2, 2), between(-2, 2), between(-500, 500), between(-500, 500)}}
		if rng.Intn(2) == 0 { // scaled and translated
			m[1], m[2], p.Transform[1], p.Transform[2] = 0, 0, 0, 0
		}
		mp, ok := raster.MapPattern(p, m)
		pe, ok2 := patternPaint(p, &m)
		if !ok || !ok2 {
			continue
		}
		x0, y0 := rng.Intn(3000), rng.Intn(2000)
		quad := image.Rect(x0, y0, x0+1+rng.Intn(1500), y0+1+rng.Intn(1500))
		var in instance
		pe.put(&in, quad)
		f := func(slot, k int) float32 { return math.Float32frombits(in[slot][k]) }
		for range 8 {
			x, y := rng.Intn(quad.Dx()), rng.Intn(quad.Dy())
			relX, relY := float32(x)+0.5, float32(y)+0.5
			X, Y := float64(quad.Min.X+x)+0.5, float64(quad.Min.Y+y)+0.5
			for axis := range 2 {
				// The shader's sum, and the rest.
				uv := float32(float32(float32(f(SlotPaint+axis, 0)*relX)+float32(f(SlotPaint+axis, 1)*relY)) + f(SlotPaint+axis, 2))
				rest := float32(float32(float32(f(SlotPaint+axis, 3)*relX)+float32(f(SlotPaint+2, 2*axis)*relY)) + f(SlotPaint+2, 2*axis+1))
				if exact := float64(f(SlotPaint+axis, 0))*float64(relX) + float64(f(SlotPaint+axis, 1))*float64(relY) + float64(f(SlotPaint+axis, 2)); float64(uv) != exact {
					t.Fatalf("the sum is rounded: %v, exactly %v (%v under %v, quad %v)", uv, exact, p, m, quad)
				}
				whole := float32(math.Floor(float64(uv)))
				got := float64(whole) + float64(float32(uv-whole)+rest)
				want, n, repeats := mp.UA*X+mp.UB*Y+mp.UC, mp.Image.Width(), p.Repeat.X()
				ext := math.Abs(mp.UA)*float64(quad.Dx()) + math.Abs(mp.UB)*float64(quad.Dy())
				if axis == 1 {
					want, n, repeats = mp.VA*X+mp.VB*Y+mp.VC, mp.Image.Height(), p.Repeat.Y()
					ext = math.Abs(mp.VA)*float64(quad.Dx()) + math.Abs(mp.VB)*float64(quad.Dy())
				}
				half := -0.5
				if p.Sampling == gfx.SampleNearest {
					half = 1.0 / (1 << 20)
				}
				want += half
				diff := got - want
				if repeats {
					if got < -0.5-1e-6 {
						t.Fatalf("a coordinate of %v along an axis that repeats", got)
					}
					diff -= math.Round(diff/float64(n)) * float64(n)
				} else {
					ext = max(ext, math.Abs(want))
				}
				worst = max(worst, math.Abs(diff))
				// What is left of the largest coordinate, 24 bits below it and
				// 12 below that.
				if lim := 2e-7 + ext*1e-9; math.Abs(diff) > lim {
					t.Fatalf("a coordinate is %v, the rasterizer's %v (off by %v, limit %v): %v under %v, quad %v", got, want, diff, lim, p, m, quad)
				}
			}
		}
	}
	t.Logf("the largest difference: %v of a pixel", worst)
}

// TestPatternImages checks the textures of the patterns' images: the mip
// level the rasterizer samples, uploaded once, kept while a list on screen
// has the pattern, drawn or not, within the budget, and replaced by the next
// frame's for a stream.
func TestPatternImages(t *testing.T) {
	size := image.Pt(400, 300)
	// The level.
	var e Encoder
	for n, s := range []float32{1, 0.5, 0.25} {
		f := e.Encode(patternList(photoPattern(gfx.Scale(s, s))), size, Options{})
		if len(f.Images) != 1 || f.Images[0].ID != photo.Level(n).ID() || f.Images[0].Width != 96>>n || f.Images[0].Height != 64>>n {
			t.Fatalf("a pattern at %v: %+v, want the level %d", s, f.Images, n)
		}
		if got := imagesOf(f); !slices.Equal(got, []uint64{photo.Level(n).ID()}) {
			t.Errorf("a pattern at %v reads %v", s, got)
		}
	}
	bytes := int64(0)
	for n := range 3 {
		bytes += int64(len(photo.Level(n).Pix()))
	}
	if m := e.Memory().Images; m != bytes {
		t.Errorf("three levels: %d bytes, want %d", m, bytes)
	}

	// Kept while the list has it: hidden, or outside the regions drawn, in
	// every command that paints, in a layer. Then it goes.
	page := func(with bool, label float32) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) {
			if with {
				p := photoPattern(gfx.Scale(0.3, 0.3)) // level 1
				b.FillRect(gfx.RectXYWH(30, 30, 80, 80), p)
				text(b, "So", 20, 300, 100, tilePattern(gfx.RepeatX, gfx.SampleLinear, gfx.Identity))
				b.PushLayer(0.5, gfx.SrcOver)
				b.StrokePath(gfxtest.StarPath(300, 220, 40), gfx.NewStrokeStyle(3), gfx.ImagePattern{Image: gfxtest.PatternBig, Transform: gfx.Scale(0.5, 0.5)})
				b.PopLayer()
			}
			b.FillRect(gfx.RectXYWH(20, 20, 200, 200), slate) // hides the first
			b.FillRect(gfx.RectXYWH(40+label, 40, 30, 10), rose)
		})
	}
	e = Encoder{}
	if f := e.Encode(page(true, 0), size, Options{}); len(f.Images) != 2 {
		t.Fatalf("the first frame uploads %d images, want the two that show", len(f.Images))
	}
	e.EndFrame()
	// Uncovered for a frame: the third.
	region := image.Rect(25, 25, 120, 120)
	e.Encode(gfxtest.Build(func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(30, 30, 80, 80), photoPattern(gfx.Scale(0.3, 0.3))) }), size, Options{Region: region})
	e.EndFrame()
	held := e.Memory().Images
	for i := range 3 * idleFrames {
		f := e.Encode(page(true, float32(i%7)), size, Options{Region: region})
		e.EndFrame()
		if len(f.Images) != 0 || len(f.Releases) != 0 || instances(f) > 2 {
			t.Fatalf("frame %d: %d images uploaded, %d released, %d instances", i, len(f.Images), len(f.Releases), instances(f))
		}
	}
	// So it is for a rest, which knows the list and not how it is drawn.
	if f := e.Rest(page(true, 0)); len(f.Releases) != 0 || e.Memory().Images != held {
		t.Errorf("a rest with the patterns on screen releases %v", f.Releases)
	}
	if f := e.Encode(page(true, 0), size, Options{}); len(f.Images) != 0 {
		t.Errorf("after a rest the page uploads %d images", len(f.Images))
	}
	released := 0
	for i := range 3 * idleFrames {
		released += len(e.Encode(page(false, float32(i%7)), size, Options{Region: region}).Releases)
		e.EndFrame()
	}
	if released != 3 || e.Memory().Images != 0 || len(e.res.levels) != 0 {
		t.Errorf("the patterns left the page: %d images released, %d bytes held, levels %v", released, e.Memory().Images, e.res.levels)
	}
	// In a mask, which a rest looks into.
	masked := gfxtest.Build(func(b *gfx.Builder) {
		b.PushMaskLayer(maskOf(gfx.MaskAlpha, func(mb *gfx.Builder) { mb.FillRect(gfx.RectXYWH(0, 0, 50, 50), photoPattern(gfx.Scale(0.2, 0.2))) }), 1, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(0, 0, 50, 50), rose)
		b.PopLayer()
	})
	e = Encoder{}
	e.Encode(masked, size, Options{})
	e.EndFrame()
	if f := e.Rest(masked); len(f.Releases) != 0 {
		t.Errorf("a rest releases the image of a mask's pattern: %v", f.Releases)
	}
	if f := e.Rest(patternList()); len(f.Releases) != 1 {
		t.Errorf("a rest without the pattern: releases %v", f.Releases)
	}

	// A level a DrawImage drew first is kept for the pattern too.
	lvl := photo.Level(2)
	e = Encoder{}
	e.Encode(drawImages(lvl), size, Options{})
	e.Encode(patternList(photoPattern(gfx.Scale(0.25, 0.25))), size, Options{})
	e.EndFrame()
	if f := e.Rest(patternList(photoPattern(gfx.Scale(0.25, 0.25)))); len(f.Releases) != 0 {
		t.Errorf("a rest releases a level that a DrawImage drew first: %v", f.Releases)
	}

	// The budget: an image that no frame draws goes first.
	a, c := imageOfSize(40, 40), imageOfSize(40, 40)
	e = Encoder{Limits: Limits{ImageBytes: 4 * 40 * 40}}
	pa, pc := gfx.ImagePattern{Image: a, Transform: gfx.Identity}, gfx.ImagePattern{Image: c, Transform: gfx.Identity}
	if f := e.Encode(patternList(pa, pc), size, Options{}); len(f.Images) != 2 || len(f.Releases) != 0 {
		t.Errorf("a frame over the budget: %d images, releases %v", len(f.Images), f.Releases)
	}
	e = Encoder{Limits: Limits{ImageBytes: 4 * 40 * 40}}
	e.Encode(patternList(pa), size, Options{})
	if f := e.Encode(patternList(pc), size, Options{}); !slices.Equal(f.Releases, []uint64{a.ID()}) {
		t.Errorf("over the budget: releases %v, want %d", f.Releases, a.ID())
	}

	// A stream: one texture, the level drawn's, frame after frame.
	e = Encoder{}
	for _, s := range []float32{1, 0.5} {
		var prev uint64
		for i := range 2 * idleFrames {
			img := gfx.NewStreamImage(16, 16, make([]byte, 4*16*16), false, 7)
			f := e.Encode(patternList(gfx.ImagePattern{Image: img, Transform: gfx.Scale(s, s)}), size, Options{})
			e.EndFrame()
			if len(f.Images) != 1 || (i > 0 && !slices.Equal(f.Releases, []uint64{prev})) {
				t.Fatalf("frame %d of a stream at %v: %d images, releases %v, want %d", i, s, len(f.Images), f.Releases, prev)
			}
			if got, want := e.Memory().Images, int64(4*16*s*16*s); got != want {
				t.Fatalf("frame %d of a stream at %v: %d bytes, want %d", i, s, got, want)
			}
			prev = f.Images[0].ID
		}
	}

	// Larger than the device's textures: named. A level that is not, is
	// drawn.
	big := imageOfSize(80, 40)
	e = Encoder{Limits: Limits{MaxTextureSize: 64}}
	f := e.Encode(patternList(gfx.ImagePattern{Image: big, Transform: gfx.Identity}), size, Options{})
	if want := []string{"image pattern (an image larger than the device's textures)"}; !slices.Equal(f.Unsupported, want) || len(f.Images) != 0 || instances(f) != 0 {
		t.Errorf("an image larger than the textures: unsupported %q, %d images", f.Unsupported, len(f.Images))
	}
	if f = e.Encode(patternList(gfx.ImagePattern{Image: big, Transform: gfx.Scale(0.5, 0.5)}), size, Options{}); len(f.Unsupported) != 0 || len(f.Images) != 1 {
		t.Errorf("its mip level: unsupported %q, %d images", f.Unsupported, len(f.Images))
	}
	// Outside what is drawn it does not matter.
	if f = e.Encode(patternList(gfx.ImagePattern{Image: big, Transform: gfx.Identity}), size, Options{Region: image.Rect(100, 100, 200, 200)}); len(f.Unsupported) != 0 {
		t.Errorf("outside the region: unsupported %q", f.Unsupported)
	}
}

func imageOfSize(w, h int) *gfx.Image { return gfx.NewImage(image.NewRGBA(image.Rect(0, 0, w, h))) }

// TestPatternExternal checks patterns of memory the device imported: it is
// sampled where it lies at its size, with no pixels read, and a mip level is
// computed from pixels that are read once per frame, in one texture.
func TestPatternExternal(t *testing.T) {
	dev := &nullDevice{}
	size := image.Pt(400, 300)
	for _, copied := range []bool{false, true} {
		e := &Encoder{dev: dev}
		m, mem := fakeImport(dev, 16, 16, copied)
		img := gfx.NewExternalImage(16, 16, true, m.Frame())
		f := e.Encode(patternList(gfx.ImagePattern{Image: img, Transform: gfx.Scale(2, 2)}), size, Options{})
		checkFrame(t, "an external pattern", f)
		if len(f.Unsupported) != 0 || mem.reads != 0 {
			t.Fatalf("copied %v: unsupported %q, pixels read %d times", copied, f.Unsupported, mem.reads)
		}
		if got, want := imagesOf(f), map[bool]uint64{false: m.id, true: img.ID()}[copied]; !slices.Equal(got, []uint64{want}) {
			t.Errorf("copied %v: the batches read %v, want %d", copied, got, want)
		}
		if (len(f.Imports) == 1) == copied || (len(f.Images) == 1 && f.Images[0].From == m) != copied {
			t.Errorf("copied %v: %d imports, images %+v", copied, len(f.Imports), f.Images)
		}
		// Minified: each frame of the buffer is an image of its own.
		var prev uint64
		for i := range 3 {
			frame := gfx.NewExternalImage(16, 16, true, m.Frame())
			f := e.Encode(patternList(gfx.ImagePattern{Image: frame, Transform: gfx.Scale(0.5, 0.5)}), size, Options{})
			if len(f.Images) != 1 || f.Images[0].Width != 8 || f.Images[0].From != nil || len(f.Images[0].Pix) != 4*8*8 || mem.reads != i+1 {
				t.Fatalf("copied %v, frame %d minified: images %+v, pixels read %d times", copied, i, f.Images, mem.reads)
			}
			if i > 0 && !slices.Contains(f.Releases, prev) {
				t.Errorf("copied %v, frame %d minified: releases %v, want %d", copied, i, f.Releases, prev)
			}
			prev = f.Images[0].ID
		}
		// Closed memory is an image without pixels, as for DrawImage.
		m.Close()
		f = e.Encode(patternList(gfx.ImagePattern{Image: gfx.NewExternalImage(16, 16, true, m.Frame()), Transform: gfx.Identity}), size, Options{})
		if len(f.Unsupported) != 0 || len(f.Imports) != 0 || len(f.Images) != 1 || !bytesAllZero(f.Images[0].Pix) {
			t.Errorf("copied %v: closed memory: %d imports, images %d", copied, len(f.Imports), len(f.Images))
		}
	}
}

// TestPatternExtremes encodes patterns under transforms at the limits of
// float32: every frame is well formed, and what is drawn is finite.
func TestPatternExtremes(t *testing.T) {
	size := image.Pt(400, 300)
	for _, m := range []gfx.Affine{
		gfx.Scale(1e-30, 1e-30), gfx.Scale(1e30, 1e30), gfx.Scale(1e-18, 1e18), gfx.Translate(1e30, -1e30),
		{1e-20, 1, 1, 1e-20, 1e10, 3}, gfx.Scale(1e-38, 1e-38), {3e38, 0, 0, 3e38, 0, 0}, gfx.Translate(0.5, 1<<30), gfx.Scale(1e-38, 1),
	} {
		for rep := gfx.RepeatBoth; rep <= gfx.RepeatNone; rep++ {
			for _, s := range []gfx.Sampling{gfx.SampleLinear, gfx.SampleNearest} {
				p := tilePattern(rep, s, m)
				var e Encoder
				f := e.Encode(gfxtest.Build(func(b *gfx.Builder) {
					b.FillRect(gfx.RectXYWH(0, 0, 400, 300), p)
					b.PushTransform(m)
					shapesOf(b, photoPattern(gfx.Identity))
					b.PopTransform()
					shapesOf(b, p)
				}), size, Options{Scale: 1.5})
				checkFrame(t, "a pattern at the limits", f)
				if len(f.Unsupported) != 0 {
					t.Errorf("%v: unsupported %q", m, f.Unsupported)
				}
			}
		}
	}
}

// TestSteadyCanvas checks that a frame of operators and patterns drawn again
// costs the Encoder no allocation and uploads nothing.
func TestSteadyCanvas(t *testing.T) {
	for _, s := range canvasScenes {
		dl := s.displayList()
		var e Encoder
		opts := Options{Scale: 1, Clear: &gfx.White}
		for range 3 {
			e.Encode(dl, image.Pt(s.W, s.H), opts)
			e.EndFrame()
		}
		var f *Frame
		allocs := testing.AllocsPerRun(10, func() {
			f = e.Encode(dl, image.Pt(s.W, s.H), opts)
			e.EndFrame()
		})
		if allocs != 0 || uploads(f) != 0 || len(f.Images) != 0 {
			t.Errorf("%s: %.0f allocations, %d uploads, %d images", s.Name, allocs, uploads(f), len(f.Images))
		}
	}
}
