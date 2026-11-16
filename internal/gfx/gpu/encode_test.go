package gpu

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"math"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// encode encodes what draw records for a target of w x h pixels.
// encode encodes what draw draws, with the quad pipeline only: the tests
// of the structure of frames look at its batches (TestEncodePlain looks at
// the plain ones).
func encode(w, h int, opts Options, draw func(b *gfx.Builder)) *Frame {
	e := Encoder{noPlain: true}
	return e.Encode(gfxtest.Build(draw), image.Pt(w, h), opts)
}

func instances(f *Frame) int { return len(f.Instances) / InstanceSize }

// floats returns slot s of instance i as float32 values.
func floats(f *Frame, i, s int) [4]float32 {
	var out [4]float32
	for k := range out {
		out[k] = math.Float32frombits(binary.LittleEndian.Uint32(f.Instances[i*InstanceSize+16*s+4*k:]))
	}
	return out
}

// slotInts returns slot s of instance i as int32 values.
func slotInts(f *Frame, i, s int) [4]int32 {
	var out [4]int32
	for k := range out {
		out[k] = int32(binary.LittleEndian.Uint32(f.Instances[i*InstanceSize+16*s+4*k:]))
	}
	return out
}

// head returns the four uint32 of SlotHead of instance i.
func head(f *Frame, i int) [4]uint32 {
	var out [4]uint32
	for k := range out {
		out[k] = binary.LittleEndian.Uint32(f.Instances[i*InstanceSize+16*SlotHead+4*k:])
	}
	return out
}

func batches(f *Frame) []Batch {
	if len(f.Passes) == 0 {
		return nil
	}
	return f.Passes[0].Batches
}

// TestLayoutMatchesShaders checks the instance layout and the flag bits
// of frame.go against the GLSL sources that read them.
func TestLayoutMatchesShaders(t *testing.T) {
	src, err := os.ReadFile("shaders/quad.frag")
	if err != nil {
		t.Fatal(err)
	}
	frag := string(src)
	members := func(name string) []string {
		m := regexp.MustCompile(`(?s)struct ` + name + ` \{(.*?)\n\};`).FindStringSubmatch(frag)
		if m == nil {
			t.Fatalf("no struct %s in quad.frag", name)
		}
		var out []string
		for _, f := range regexp.MustCompile(`(?m)^\s*([ui]?vec4|Shape) (\w+);`).FindAllStringSubmatch(m[1], -1) {
			out = append(out, f[1]+" "+f[2])
		}
		return out
	}
	if got := len(members("Shape")); got != ShapeSlots {
		t.Errorf("struct Shape has %d vec4 members, ShapeSlots is %d", got, ShapeSlots)
	}
	want := map[string]int{"bounds": SlotBounds, "head": SlotHead, "paint0": SlotPaint, "shape": SlotShape, "clip": SlotClip, "blur": SlotBlur, "sigma": SlotSigma, "origin": SlotOrigin, "tex": SlotTex}
	slot := 0
	for _, m := range members("Instance") {
		typ, name, _ := strings.Cut(m, " ")
		if w, ok := want[name]; ok {
			if w != slot {
				t.Errorf("Instance.%s is at slot %d in quad.frag, %d in frame.go", name, slot, w)
			}
			delete(want, name)
		}
		if typ == "Shape" {
			slot += ShapeSlots
		} else {
			slot++
		}
	}
	if slot != InstanceSlots {
		t.Errorf("struct Instance has %d slots in quad.frag, InstanceSlots is %d", slot, InstanceSlots)
	}
	for name := range want {
		t.Errorf("quad.frag has no Instance.%s", name)
	}
	for name, v := range map[string]uint32{
		"KIND_MASK": FlagKindMask, "KIND_OUTER": FlagOuterShadow, "KIND_INSET": FlagInsetShadow, "SHAPE_LOCAL": FlagShapeLocal,
		"CLIP_SHIFT": FlagClipShift, "PAINT_SHIFT": FlagPaintShift, "SPREAD_SHIFT": FlagSpreadShift, "BLUR_SHIFT": FlagBlurShift,
		"SEGMENTS_SHIFT": FlagSegmentsShift, "KIND_ATLAS": FlagCoverAtlas, "CLIP_MASK": FlagMask,
		"PAINT_IMAGE": PaintImageLinear, "PAINT_NEAREST": PaintImageNearest, "PAINT_SPRITE": PaintSprite,
	} {
		m := regexp.MustCompile(`const uint ` + name + ` = (\d+)u;`).FindStringSubmatch(frag)
		if m == nil {
			t.Errorf("quad.frag has no constant %s", name)
		} else if got, _ := strconv.Atoi(m[1]); uint32(got) != v {
			t.Errorf("%s is %d in quad.frag, %d in frame.go", name, got, v)
		}
	}
	vert, err := os.ReadFile("shaders/quad.vert")
	if err != nil {
		t.Fatal(err)
	}
	if want := "vec4 rest[" + strconv.Itoa(InstanceSlots-1) + "];"; !strings.Contains(string(vert), want) {
		t.Errorf("quad.vert does not declare an instance of %d slots (%q)", InstanceSlots, want)
	}
	if RampWidth != raster.RampSize {
		t.Errorf("RampWidth is %d, raster.RampSize %d", RampWidth, raster.RampSize)
	}
}

func TestEncodeRect(t *testing.T) {
	f := encode(100, 80, Options{Scale: 2}, func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(10.25, 5, 20, 10.3), gfx.RGB8(255, 0, 0).WithAlpha(0.5))
	})
	if f.Size != image.Pt(100, 80) || len(f.Passes) != 1 || f.Passes[0].Clear != nil || len(f.Unsupported) != 0 {
		t.Fatalf("frame: %+v", f)
	}
	if got, want := batches(f), []Batch{{Pipeline: PipelineQuad, Scissor: image.Rect(0, 0, 100, 80), First: 0, Count: 1}}; !reflect.DeepEqual(got, want) {
		t.Errorf("batches %+v, want %+v", got, want)
	}
	// Device rectangle 20.5, 10, 60.5, 30.6, snapped to 1/256: 30.6 becomes
	// 7834/256. The quad is the pixels it touches; the shape is relative to
	// the quad's corner.
	if got, want := floats(f, 0, SlotBounds), [4]float32{20, 10, 61, 31}; got != want {
		t.Errorf("bounds %v, want %v", got, want)
	}
	if got, want := floats(f, 0, SlotShape), [4]float32{0.5, 0, 40.5, 7834.0/256 - 10}; got != want {
		t.Errorf("shape %v, want %v", got, want)
	}
	h := head(f, 0)
	if h[0] != 0 {
		t.Errorf("flags %#x, want 0 (a fill of a device shape with a solid paint, no clip)", h[0])
	}
	if want := packRGBA8([4]uint8{128, 0, 0, 128}); h[2] != want {
		t.Errorf("color %#x, want %#x", h[2], want)
	}
}

func TestEncodeRadiiFollowTheTransform(t *testing.T) {
	rr := gfx.RRectCorners(gfx.RectXYWH(0, 0, 40, 20), gfx.Pt(1, 2), gfx.Pt(3, 4), gfx.Pt(5, 6), gfx.Pt(7, 8))
	for _, c := range []struct {
		name       string
		m          gfx.Affine
		radX, radY [4]float32 // device corners: TL, TR, BR, BL
	}{
		{"identity", gfx.Identity, [4]float32{1, 3, 5, 7}, [4]float32{2, 4, 6, 8}},
		{"scale", gfx.Scale(2, 3), [4]float32{2, 6, 10, 14}, [4]float32{6, 12, 18, 24}},
		{"flip x", gfx.Translate(50, 0).Mul(gfx.Scale(-1, 1)), [4]float32{3, 1, 7, 5}, [4]float32{4, 2, 8, 6}},
		{"flip y", gfx.Translate(0, 50).Mul(gfx.Scale(1, -1)), [4]float32{7, 5, 3, 1}, [4]float32{8, 6, 4, 2}},
		// x' = 30 - y, y' = x: a quarter turn clockwise. The local top-left
		// corner lands top-right, and x and y radii swap.
		{"quarter turn", gfx.Affine{0, 1, -1, 0, 30, 0}, [4]float32{8, 2, 4, 6}, [4]float32{7, 1, 3, 5}},
	} {
		f := encode(100, 100, Options{}, func(b *gfx.Builder) {
			b.PushTransform(c.m)
			b.FillRRect(rr, gfx.Black)
			b.PopTransform()
		})
		if instances(f) != 1 {
			t.Fatalf("%s: %d instances", c.name, instances(f))
		}
		if head(f, 0)[0]&FlagShapeLocal != 0 {
			t.Errorf("%s: a local shape under an axis-aligned transform", c.name)
		}
		if x, y := floats(f, 0, SlotShape+1), floats(f, 0, SlotShape+2); x != c.radX || y != c.radY {
			t.Errorf("%s: radii x %v y %v, want x %v y %v", c.name, x, y, c.radX, c.radY)
		}
	}
	// Under a rotation the shape stays local, with the inverse transform.
	f := encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushTransform(gfx.Translate(50, 50).Mul(gfx.Rotate(0.5)))
		b.FillRRect(rr, gfx.Black)
		b.PopTransform()
	})
	if instances(f) != 1 || head(f, 0)[0]&FlagShapeLocal == 0 {
		t.Fatalf("a rotated shape: %d instances, flags %#x", instances(f), head(f, 0)[0])
	}
	if got, want := floats(f, 0, SlotShape), [4]float32{0, 0, 40, 20}; got != want {
		t.Errorf("local rect %v, want %v", got, want)
	}
	inv, o := floats(f, 0, SlotShape+3), floats(f, 0, SlotBounds)
	sin, cos := math.Sincos(0.5)
	// The local point of the pixel center (60, 70): relative to the quad's
	// corner it is (60 - o.x, 70 - o.y).
	t0 := floats(f, 0, SlotShape+4)
	x, y := float64(60-o[0]), float64(70-o[1])
	lx := float64(inv[0])*x + float64(inv[2])*y + float64(t0[0])
	ly := float64(inv[1])*x + float64(inv[3])*y + float64(t0[1])
	wx, wy := cos*10+sin*20, -sin*10+cos*20
	if math.Abs(lx-wx) > 1e-4 || math.Abs(ly-wy) > 1e-4 {
		t.Errorf("the pixel center (60, 70) maps to (%g, %g), want (%g, %g)", lx, ly, wx, wy)
	}
}

func TestEncodeClips(t *testing.T) {
	// (Translucent: an opaque fill would hide the fills before it, cull.go.)
	fill := func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(0, 0, 100, 100), gfx.Black.WithAlpha(0.5)) }
	clipOf := func(f *Frame, i int) uint32 { return head(f, i)[0] >> FlagClipShift & 3 }

	// On pixel boundaries: a scissor, no clip in the instance.
	f := encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushClipRect(gfx.RectXYWH(10, 20, 30, 40))
		fill(b)
		b.PopClip()
		fill(b)
	})
	want := []Batch{
		{Pipeline: PipelineQuad, Scissor: image.Rect(10, 20, 40, 60), First: 0, Count: 1},
		{Pipeline: PipelineQuad, Scissor: image.Rect(0, 0, 100, 100), First: 1, Count: 1},
	}
	if got := batches(f); !reflect.DeepEqual(got, want) {
		t.Errorf("an aligned clip: batches %+v, want %+v", got, want)
	}
	if clipOf(f, 0) != 0 {
		t.Errorf("an aligned clip is in the instance (flags %#x)", head(f, 0)[0])
	}

	// Between pixels: the scissor is the pixels touched, and the instance
	// carries the clip, relative to its origin (the quad is the whole
	// target here).
	f = encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushClipRect(gfx.RectXYWH(10.5, 20, 30, 40.25))
		fill(b)
		b.PopClip()
	})
	if got := batches(f); len(got) != 1 || got[0].Scissor != image.Rect(10, 20, 41, 61) {
		t.Errorf("a clip between pixels: batches %+v", got)
	}
	if clipOf(f, 0) != ClipDevice {
		t.Errorf("a clip between pixels: flags %#x", head(f, 0)[0])
	}
	if got, want := floats(f, 0, SlotClip), [4]float32{10.5, 20, 40.5, 60.25}; got != want {
		t.Errorf("clip rect %v, want %v", got, want)
	}

	// Under a rotation: a local clip.
	f = encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushTransform(gfx.RotateAbout(0.3, gfx.Pt(50, 50)))
		b.PushClipRect(gfx.RectXYWH(30, 30, 40, 40))
		fill(b)
		b.PopClip()
		b.PopTransform()
	})
	if instances(f) != 1 || clipOf(f, 0) != ClipLocal || len(f.Unsupported) != 0 {
		t.Errorf("a rotated clip: %d instances, flags %#x, unsupported %v", instances(f), head(f, 0)[0], f.Unsupported)
	}

	// Nested: a clip between pixels well inside another one replaces it
	// (the outer one covers every pixel left), and an aligned clip inside
	// keeps it.
	f = encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushClipRect(gfx.RectXYWH(10.5, 10.5, 80, 80))
		b.PushClipRect(gfx.RectXYWH(20.25, 20.25, 30, 30))
		fill(b)
		b.PopClip()
		b.PushClipRect(gfx.RectXYWH(5, 60, 50, 20))
		fill(b)
		b.PopClip()
		b.PopClip()
	})
	if len(f.Unsupported) != 0 || instances(f) != 2 {
		t.Fatalf("nested clips: %d instances, unsupported %v", instances(f), f.Unsupported)
	}
	if got, want := floats(f, 0, SlotClip), [4]float32{20.25, 20.25, 50.25, 50.25}; got != want || clipOf(f, 0) != ClipDevice {
		t.Errorf("the inner clip: %v (flags %#x), want %v", got, head(f, 0)[0], want)
	}
	if got, want := floats(f, 1, SlotClip), [4]float32{10.5, 10.5, 90.5, 90.5}; got != want || clipOf(f, 1) != ClipDevice {
		t.Errorf("the outer clip under an aligned one: %v (flags %#x), want %v", got, head(f, 1)[0], want)
	}
	if got, want := batches(f)[1].Scissor, image.Rect(10, 60, 55, 80); got != want {
		t.Errorf("the scissor of the aligned clip inside: %v, want %v", got, want)
	}

	// Two clips between pixels that share edge pixels: the instances inside
	// carry the first, and read the second from a clip mask in the atlas.
	f = encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushClipRect(gfx.RectXYWH(10.5, 10.5, 80, 80))
		b.PushClipRect(gfx.RectXYWH(10.25, 20.25, 30, 30))
		fill(b)
		b.PopClip()
		fill(b)
		b.PopClip()
	})
	// The fill inside both is drawn in parts: the one where the second
	// clip covers entirely reads no mask, the pieces along its edges read
	// their tile of it; all carry the first clip, and so does the fill
	// inside the first only, the last instance.
	n := instances(f)
	if len(f.Unsupported) != 0 || n < 3 {
		t.Fatalf("clips sharing edge pixels: %d instances, unsupported %v", n, f.Unsupported)
	}
	masked, plain := 0, 0
	for i := 0; i < n-1; i++ {
		h := head(f, i)[0]
		if clipOf(f, i) != ClipDevice {
			t.Errorf("part %d of the instance inside both: flags %#x, want a clip", i, h)
		}
		if got, want := floats(f, i, SlotClip), [4]float32{10.5, 10.5, 90.5, 90.5}; got != want {
			t.Errorf("the clip of part %d of the instance inside both: %v, want %v", i, got, want)
		}
		if h&FlagMask != 0 {
			masked++
		} else {
			plain++
		}
	}
	if masked == 0 || plain != 1 {
		t.Errorf("the instance inside both: %d parts read the mask and %d do not, want some and one", masked, plain)
	}
	if h := head(f, n-1)[0]; clipOf(f, n-1) != ClipDevice || h&FlagMask != 0 {
		t.Errorf("the instance inside the first only: flags %#x, want a clip and no mask", h)
	}
	// The mask is the second clip over the pixels both leave, with the
	// rasterizer's coverage, 3/4 of the first column and row, read by the
	// parts where they are in the atlas.
	sim := &atlasSim{t: t}
	mask := map[image.Point]uint8{}
	sc := f.Passes[0].Batches[0].Scissor
	for i, rd := range sim.draw(f)[:n-1] {
		vis := rd.quad.Intersect(sc)
		k := 0
		for y := vis.Min.Y; y < vis.Max.Y; y++ {
			for x := vis.Min.X; x < vis.Max.X; x++ {
				mask[image.Pt(x, y)] = 255
				if head(f, i)[0]&FlagMask != 0 {
					mask[image.Pt(x, y)] = rd.mask[k]
				}
				k++
			}
		}
	}
	at := func(x, y int) uint8 { return mask[image.Pt(10+x, 20+y)] }
	if len(mask) != 31*31 || at(0, 0) != 143 || at(1, 0) != 191 || at(0, 1) != 191 || at(1, 1) != 255 || at(30, 0) != 48 || at(30, 30) != 16 {
		t.Errorf("the mask's pixels (%d of them): corner %d, top %d, left %d, inside %d, top right %d, bottom right %d",
			len(mask), at(0, 0), at(1, 0), at(0, 1), at(1, 1), at(30, 0), at(30, 30))
	}

	// A clip that leaves nothing.
	f = encode(100, 100, Options{}, func(b *gfx.Builder) {
		b.PushClipRect(gfx.RectXYWH(200, 200, 10, 10))
		fill(b)
		b.PopClip()
		b.PushClipRect(gfx.RectXYWH(10, 10, 0.001, 50))
		fill(b)
		b.PopClip()
	})
	if instances(f) != 0 || len(f.Passes) != 0 {
		t.Errorf("clips that leave nothing: %d instances, %d passes", instances(f), len(f.Passes))
	}
}

func TestEncodeUnsupported(t *testing.T) {
	var star gfx.Path
	star.MoveTo(gfx.Pt(10, 10))
	star.LineTo(gfx.Pt(50, 20))
	star.LineTo(gfx.Pt(20, 50))
	star.Close()
	// Every command is drawn.
	everything := gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 1, 1), gfx.Black)
		b.FillPath(star, gfx.EvenOdd, gfx.Black)
		b.StrokePath(star, gfx.NewStrokeStyle(2), gfx.Black)
		b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(0, 0, 20, 20), gfx.SampleNearest)
		b.DrawGlyphRun(gfxtest.FontA.Run("So*", 20, 10, 40, gfx.Black))
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(0, 0, 50, 50), 8))
		b.PushClipPath(star, gfx.NonZero)
		b.FillRect(gfx.RectXYWH(20, 20, 10, 10), gfx.Black)
		b.PopClip()
		b.PopClip()
		b.PushLayer(0.5, gfx.Multiply, gfx.Blur{StdDev: 3})
		b.FillPath(star, gfx.NonZero, gfx.Black)
		b.PopLayer()
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(0, 0, 50, 50), 8), gfx.Blur{StdDev: 2})
		b.Marker(7)
	})
	var e Encoder
	if f := e.Encode(everything, image.Pt(100, 100), Options{}); len(f.Unsupported) != 0 || instances(f) < 10 {
		t.Errorf("a list of every command: unsupported %v, %d instances", f.Unsupported, instances(f))
	}
	// What is larger than the device's textures is reported, not drawn: an
	// image, a layer, the backdrop of a backdrop filter.
	e = Encoder{Limits: Limits{MaxTextureSize: 4}}
	f := e.Encode(everything, image.Pt(100, 100), Options{})
	want := []string{
		"DrawImage (an image larger than the device's textures)",
		"PushLayer (a layer larger than the device's textures)",
		"BackdropFilter (a backdrop larger than the device's textures)",
	}
	if !reflect.DeepEqual(f.Unsupported, want) || len(f.Images) != 0 || len(f.Targets) != 0 {
		t.Errorf("too large: unsupported %v, %d images, %d targets", f.Unsupported, len(f.Images), len(f.Targets))
	}
	for i := range f.Passes {
		if f.Passes[i].Target != 0 {
			t.Errorf("too large: a pass into the target %d", f.Passes[i].Target)
		}
	}
}

func TestEncodeRegion(t *testing.T) {
	dl := gpuScenes[0].displayList() // gpu_rects
	var e Encoder
	size := image.Pt(200, 120)
	full := e.Encode(dl, size, Options{})
	all := append([]byte(nil), full.Instances...)
	region := image.Rect(30, 20, 90, 70)
	f := e.Encode(dl, size, Options{Region: region})
	n := instances(f)
	if n == 0 || n >= len(all)/InstanceSize {
		t.Fatalf("%d instances in the region, %d in the frame", n, len(all)/InstanceSize)
	}
	for _, b := range batches(f) {
		if !b.Scissor.In(region) || b.Scissor.Empty() {
			t.Errorf("scissor %v outside the region %v", b.Scissor, region)
		}
	}
	// Every instance of the region is an instance of the full frame, byte
	// for byte and in the same order: the pixels cannot differ.
	subsequence(t, "gpu_rects", f.Instances, all)
	// A region without area is the whole target; one outside draws
	// nothing.
	if f := e.Encode(dl, size, Options{Region: image.Rect(5, 5, 5, 50)}); !bytes.Equal(f.Instances, all) {
		t.Error("a region without area is not the whole target")
	}
	if f := e.Encode(dl, size, Options{Region: image.Rect(300, 0, 400, 100), Clear: &gfx.White}); len(f.Passes) != 0 {
		t.Errorf("a region outside the target: %d passes", len(f.Passes))
	}

	// The same for what reads textures: the instances of a region are
	// those of the full frame, with the same places in the atlases (the
	// region finds the pictures the full frame made), in tiles small enough
	// that pictures and clip masks are cut into several; and a region
	// encoded first, in a new Encoder, makes the pictures the full frame
	// then finds.
	limits := Limits{CoveragePage: 256, CoveragePages: 64, ColorPage: 128, ColorPages: 64}
	for i := range textureScenes {
		s := &textureScenes[i]
		dl := s.displayList()
		size := image.Pt(2*s.W, 2*s.H)
		e := Encoder{Limits: limits}
		all := append([]byte(nil), e.Encode(dl, size, Options{Scale: 2}).Instances...)
		first := Encoder{Limits: limits}
		for _, region := range []image.Rectangle{image.Rect(100, 60, 420, 300), image.Rect(0, 0, 130, 130), image.Rect(500, 300, 501, 600), image.Rect(256, 128, 512, 384)} {
			f := e.Encode(dl, size, Options{Scale: 2, Region: region})
			if uploads(f) != 0 {
				t.Errorf("%s, region %v: %d uploads after a full frame", s.Name, region, uploads(f))
			}
			subsequence(t, s.Name, f.Instances, all)
			f = first.Encode(dl, size, Options{Scale: 2, Region: region})
			for _, b := range batches(f) {
				if !b.Scissor.In(region) {
					t.Errorf("%s: scissor %v outside the region %v", s.Name, b.Scissor, region)
				}
			}
		}
		f := first.Encode(dl, size, Options{Scale: 2})
		if instances(f) != len(all)/InstanceSize {
			t.Errorf("%s: a full frame after regions has %d instances, alone %d", s.Name, instances(f), len(all)/InstanceSize)
		}
		// The pictures are at other places, but they are the same pictures.
		a, b := (&atlasSim{t: t}), (&atlasSim{t: t})
		e.Trim()
		if !reflect.DeepEqual(readsOf(a, &first, dl, size), readsOf(b, &e, dl, size)) {
			t.Errorf("%s: after regions, the instances of a full frame read other pixels", s.Name)
		}
	}
}

// readsOf starts e over, encodes dl with it and returns what the frame's
// instances read from the atlases.
func readsOf(sim *atlasSim, e *Encoder, dl *gfx.DisplayList, size image.Point) []instanceRead {
	e.Trim()
	return sim.draw(e.Encode(dl, size, Options{Scale: 2}))
}

// subsequence checks that the instances of part are instances of all, in
// the same order. The first ramp row of an instance is left out: the rows
// of a frame's ramps are in the order the frame meets its gradients, and
// a region meets fewer (the rows hold the same colors).
func subsequence(t *testing.T, what string, part, all []byte) {
	t.Helper()
	const row = 16*SlotHead + 4
	same := func(a, b []byte) bool {
		return bytes.Equal(a[:row], b[:row]) && bytes.Equal(a[row+4:], b[row+4:])
	}
	rest := all
	for i := 0; i < len(part)/InstanceSize; i++ {
		in := part[i*InstanceSize : (i+1)*InstanceSize]
		found := false
		for len(rest) > 0 {
			same := same(rest[:InstanceSize], in)
			rest = rest[InstanceSize:]
			if same {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s: instance %d of the region is not in the full frame, or out of order", what, i)
		}
	}
}

func TestEncodeClearAndEmpty(t *testing.T) {
	var e Encoder
	size := image.Pt(40, 30)
	half := gfx.Color{R: 1, G: 0.5, B: 0, A: 0.5}
	for _, dl := range []*gfx.DisplayList{nil, gfx.NewBuilder().Finish()} {
		if f := e.Encode(dl, size, Options{}); len(f.Passes) != 0 || instances(f) != 0 {
			t.Errorf("an empty list: %+v", f)
		}
		f := e.Encode(dl, size, Options{Clear: &half, Region: image.Rect(-5, 10, 20, 100)})
		if len(f.Passes) != 1 || len(f.Passes[0].Batches) != 0 || f.Passes[0].Clear == nil {
			t.Fatalf("a clear: %+v", f)
		}
		if c := *f.Passes[0].Clear; c.Rect != image.Rect(0, 10, 20, 30) || c.Color != [4]uint8{128, 64, 0, 128} {
			t.Errorf("clear %+v", c)
		}
	}
	for _, s := range []float32{-1, float32(math.NaN()), float32(math.Inf(1))} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("scale %v did not panic", s)
				}
			}()
			e.Encode(nil, size, Options{Scale: s})
		}()
	}
}

func TestEncodeBatches(t *testing.T) {
	f := encode(100, 100, Options{}, func(b *gfx.Builder) {
		for i := 0; i < 5; i++ {
			b.FillRect(gfx.RectXYWH(float32(i)*10, 0, 8, 8), gfx.Black)
		}
		b.PushClipRect(gfx.RectXYWH(0, 50, 50, 50))
		b.FillRect(gfx.RectXYWH(0, 0, 100, 100), gfx.Black)
		b.FillRect(gfx.RectXYWH(0, 0, 100, 100), gfx.White)
		b.PopClip()
		b.FillRect(gfx.RectXYWH(0, 0, 1, 1), gfx.Black)
		// Nothing of these reaches the frame: transparent, outside, empty.
		b.FillRect(gfx.RectXYWH(0, 0, 10, 10), gfx.Transparent)
		b.FillRect(gfx.RectXYWH(500, 0, 10, 10), gfx.Black)
		b.FillRect(gfx.RectXYWH(10, 10, 0.001, 10), gfx.Black) // snapped to nothing
	})
	want := []Batch{
		{Pipeline: PipelineQuad, Scissor: image.Rect(0, 0, 100, 100), First: 0, Count: 5},
		{Pipeline: PipelineQuad, Scissor: image.Rect(0, 50, 50, 100), First: 5, Count: 2},
		{Pipeline: PipelineQuad, Scissor: image.Rect(0, 0, 100, 100), First: 7, Count: 1},
	}
	if got := batches(f); !reflect.DeepEqual(got, want) {
		t.Errorf("batches %+v, want %+v", got, want)
	}
	// Inside the clip the quad is still the shape's pixels in the target,
	// not the scissor: instances do not depend on the scissor.
	if got, want := floats(f, 5, SlotBounds), [4]float32{0, 0, 100, 100}; got != want {
		t.Errorf("bounds under a scissor %v, want %v", got, want)
	}
}

func TestEncodeGradients(t *testing.T) {
	st := stops(rose, indigo, amber)
	hard := []gfx.GradientStop{stop(0, rose), stop(0, sky), stop(0.5, sky), stop(0.5, slate), stop(1, slate), stop(1, amber)}
	var e Encoder
	f := e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(10, 0, 50, 10), gfx.LinearGradient{Start: gfx.Pt(10, 0), End: gfx.Pt(60, 0), Stops: st, Spread: gfx.SpreadReflect})
		b.FillRect(gfx.RectXYWH(0, 10, 50, 10), gfx.RadialGradient{Center: gfx.Pt(25, 15), Radius: gfx.Pt(20, 5), Stops: st})
		b.FillRect(gfx.RectXYWH(0, 20, 50, 10), gfx.ConicGradient{Center: gfx.Pt(25, 25), StartAngle: math.Pi, Stops: hard})
		b.FillRect(gfx.RectXYWH(0, 30, 50, 10), gfx.FocalGradient{Focal: gfx.Pt(20, 35), Center: gfx.Pt(25, 35), Radius: 20, Stops: st, Interp: gfx.InterpSRGB})
		// Gradients that paint one color are solid paints.
		b.FillRect(gfx.RectXYWH(0, 40, 50, 10), gfx.LinearGradient{Start: gfx.Pt(5, 5), End: gfx.Pt(5, 5), Stops: st})
		b.FillRect(gfx.RectXYWH(0, 50, 50, 10), gfx.RadialGradient{Center: gfx.Pt(5, 5), Stops: st})
	}), image.Pt(200, 200), Options{Scale: 2})
	if instances(f) != 6 {
		for i := 0; i < instances(f); i++ {
			t.Logf("instance %d: flags %#x bounds %v", i, head(f, i)[0], floats(f, i, SlotBounds))
		}
		t.Fatalf("%d instances", instances(f))
	}
	kind := func(i int) uint32 { return head(f, i)[0] >> FlagPaintShift & 7 }
	rows := func(i int) (first, n int) { return int(head(f, i)[1]), int(head(f, i)[0] >> FlagSegmentsShift) }
	for i, want := range []uint32{PaintLinear, PaintRadial, PaintConic, PaintFocal, PaintSolid, PaintSolid} {
		if kind(i) != want {
			t.Errorf("instance %d: paint kind %d, want %d", i, kind(i), want)
		}
	}
	if got := head(f, 0)[0] >> FlagSpreadShift & 3; got != uint32(gfx.SpreadReflect) {
		t.Errorf("spread %d", got)
	}
	if want := packRGBA8(premul8(amber)); head(f, 4)[2] != want || head(f, 5)[2] != want {
		t.Errorf("the degenerate gradients are %#x and %#x, want the last stop %#x", head(f, 4)[2], head(f, 5)[2], want)
	}
	// The same stops and color space share a ramp; another color space or
	// other stops get their own rows. The hard stops at 0 and 1 make no
	// segment: 2 rows.
	r0, n0 := rows(0)
	r1, n1 := rows(1)
	r2, n2 := rows(2)
	r3, n3 := rows(3)
	if r0 != 0 || n0 != 2 || r1 != 0 || n1 != 2 || r2 != 2 || n2 != 2 || r3 != 4 || n3 != 2 {
		t.Errorf("ramp rows (first, count): %d,%d %d,%d %d,%d %d,%d", r0, n0, r1, n1, r2, n2, r3, n3)
	}
	if len(f.Ramps) != 6*4*RampWidth || len(f.RampSegments) != 6*RampSegmentSize {
		t.Fatalf("%d ramp bytes, %d segment values", len(f.Ramps), len(f.RampSegments))
	}
	// The rows are the rasterizer's, and the pad colors the first and last
	// stops (not the ends of the rows, with hard stops at 0 and 1).
	rp := raster.GradientRamp(hard, gfx.InterpOklab)
	for k, seg := range rp.Segments {
		row := f.Ramps[(2+k)*4*RampWidth:][:4*RampWidth]
		for i, c := range seg.Colors {
			if [4]uint8(row[4*i:]) != c {
				t.Fatalf("ramp row %d, entry %d: %v, want %v", 2+k, i, row[4*i:4*i+4], c)
			}
		}
		if got := f.RampSegments[(2+k)*RampSegmentSize:][:2]; got[0] != float32(seg.Offset) || got[1] != float32(seg.Scale) {
			t.Errorf("segment %d: %v, want offset %g and factor %g", k, got, seg.Offset, seg.Scale)
		}
	}
	if h := head(f, 2); h[2] != packRGBA8(premul8(rose)) || h[3] != packRGBA8(premul8(amber)) {
		t.Errorf("pad colors %#x %#x", h[2], h[3])
	}
	if p := floats(f, 2, SlotPaint+3); p[1] != 0 || p[2] != 1 {
		t.Errorf("stop range %v..%v, want 0..1", p[1], p[2])
	}
	if p := floats(f, 2, SlotPaint); p[3] != 0.5 {
		t.Errorf("conic start %v turns, want 0.5", p[3])
	}
	// The linear gradient: t = (x - 10)/50 in logical pixels; at scale 2,
	// for a pixel center X relative to the quad (at device x 20):
	// t = X/100.
	if p := floats(f, 0, SlotPaint); math.Abs(float64(p[0])-0.01) > 1e-9 || p[1] != 0 || math.Abs(float64(p[2])) > 1e-9 {
		t.Errorf("linear coefficients %v, want 0.01, 0, 0", p)
	}

	// The next frame bakes nothing again and starts its rows at 0.
	f = e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 50, 10), gfx.ConicGradient{Center: gfx.Pt(25, 25), Stops: hard})
	}), image.Pt(100, 100), Options{})
	if r, n := int(head(f, 0)[1]), int(head(f, 0)[0]>>FlagSegmentsShift); r != 0 || n != 2 || len(f.Ramps) != 2*4*RampWidth {
		t.Errorf("the second frame: rows %d,%d and %d ramp bytes", r, n, len(f.Ramps))
	}
	if len(e.ramps) != 3 {
		t.Errorf("the encoder keeps %d ramps, want 3", len(e.ramps))
	}
}

func TestEncodeShadows(t *testing.T) {
	box := gfx.NewRRect(gfx.RectXYWH(20, 20, 40, 30), 6)
	mode := func(f *Frame, i int) uint32 { return head(f, i)[0] >> FlagBlurShift & 3 }
	kind := func(f *Frame, i int) uint32 { return head(f, i)[0] & FlagKindMask }
	one := func(m gfx.Affine, s gfx.DrawShadow) *Frame {
		s.RRect, s.Color = box, gfx.Black
		return encode(200, 200, Options{}, func(b *gfx.Builder) {
			b.PushTransform(m)
			b.DrawShadow(s)
			b.PopTransform()
		})
	}
	f := one(gfx.Identity, gfx.DrawShadow{Offset: gfx.Pt(2, 4), Blur: 10, Spread: 3})
	if instances(f) != 1 || kind(f, 0) != FlagOuterShadow || mode(f, 0) != BlurAnalytic {
		t.Fatalf("an outer shadow: %d instances, flags %#x", instances(f), head(f, 0)[0])
	}
	// The quad is the shape grown by three standard deviations, as the
	// rasterizer paints it: the box moved by the offset and grown by the
	// spread is 19..65 x 21..57, and 3 sigma is 15.
	if got, want := floats(f, 0, SlotBounds), [4]float32{4, 6, 80, 72}; got != want {
		t.Errorf("bounds %v, want %v", got, want)
	}
	if got, want := floats(f, 0, SlotBlur), [4]float32{15, 15, 61, 51}; got != want {
		t.Errorf("the blurred shape %v, want %v", got, want)
	}
	if got, want := floats(f, 0, SlotBlur+1), [4]float32{9, 9, 9, 9}; got != want {
		t.Errorf("its radii %v, want %v (the radius grown by the spread)", got, want)
	}
	if got, want := floats(f, 0, SlotSigma), [4]float32{5, 5, 1, 1}; got != want {
		t.Errorf("sigma %v, want %v", got, want)
	}
	if got, want := floats(f, 0, SlotShape), [4]float32{16, 14, 56, 44}; got != want {
		t.Errorf("the box %v, want %v", got, want)
	}

	// At scale and under a rotation: the frame of the Gaussian is local.
	f = one(gfx.RotateAbout(0.4, gfx.Pt(40, 35)).Mul(gfx.Scale(2, 2)), gfx.DrawShadow{Blur: 10})
	if got, want := floats(f, 0, SlotSigma), [4]float32{5, 5, 0.5, 0.5}; mode(f, 0) != BlurAnalytic || got != want {
		t.Errorf("a rotated shadow: mode %d, sigma %v, want %v", mode(f, 0), got, want)
	}
	if got, want := floats(f, 0, SlotBlur), [4]float32{20, 20, 60, 50}; got != want {
		t.Errorf("a rotated shadow: shape %v, want the local %v", got, want)
	}

	// No blur: the shape's coverage, device or local.
	if f = one(gfx.Identity, gfx.DrawShadow{Offset: gfx.Pt(3, 3)}); mode(f, 0) != BlurHardDevice {
		t.Errorf("a hard shadow: mode %d", mode(f, 0))
	}
	if f = one(gfx.Rotate(0.2), gfx.DrawShadow{Offset: gfx.Pt(3, 3)}); mode(f, 0) != BlurHardLocal {
		t.Errorf("a hard rotated shadow: mode %d", mode(f, 0))
	}

	// Inset: the quad is the box; an inner shape shrunk to nothing shadows
	// the whole box, and an outer shape shrunk to nothing paints nothing.
	f = one(gfx.Identity, gfx.DrawShadow{Blur: 6, Spread: 2, Inset: true})
	if kind(f, 0) != FlagInsetShadow || mode(f, 0) != BlurAnalytic || floats(f, 0, SlotBounds) != [4]float32{20, 20, 60, 50} {
		t.Errorf("an inset shadow: flags %#x, bounds %v", head(f, 0)[0], floats(f, 0, SlotBounds))
	}
	if f = one(gfx.Identity, gfx.DrawShadow{Blur: 6, Spread: 40, Inset: true}); instances(f) != 1 || mode(f, 0) != BlurEmpty {
		t.Errorf("an inset shadow without inner shape: %d instances, mode %d", instances(f), mode(f, 0))
	}
	if f = one(gfx.Identity, gfx.DrawShadow{Blur: 6, Spread: -40}); instances(f) != 0 {
		t.Errorf("an outer shadow of nothing: %d instances", instances(f))
	}
}

// TestEncodeScenes encodes every scene at several scales and checks that
// no instance carries a value that is not finite, that batches cover the
// instances in order, and that an Encoder gives the same frame when
// reused.
// checkFrame checks that a frame is well formed: every instance drawn by
// exactly one batch, scissors and clears inside their targets, pipelines
// that suit them, and a pass graph a device can execute, in which every
// target is listed, none is read by a pass that draws into it, and none is
// read before a pass of the frame has drawn into it.
func checkFrame(t *testing.T, what string, f *Frame) {
	t.Helper()
	checkFrameKept(t, what, f, nil)
}

// checkFrameKept is checkFrame for a frame that may read the targets kept,
// which hold pixels of earlier frames (a BackdropCache's), before it draws
// into them.
func checkFrameKept(t *testing.T, what string, f *Frame, kept []int) {
	t.Helper()
	targets := map[int]TargetInfo{}
	for _, info := range f.Targets {
		if _, dup := targets[info.ID]; dup || info.ID <= 0 || info.Width < 1 || info.Height < 1 || info.Format > FormatRGBA16F {
			t.Fatalf("%s: target %+v (listed twice: %v)", what, info, dup)
		}
		targets[info.ID] = info
	}
	drawn := map[int]bool{}
	for _, id := range kept {
		drawn[id] = true
	}
	seen := make([]bool, instances(f))
	for pi := range f.Passes {
		p := &f.Passes[pi]
		rect, format := image.Rectangle{Max: f.Size}, FormatRGBA8
		if p.Target != 0 {
			info, ok := targets[p.Target]
			if !ok {
				t.Fatalf("%s: pass %d draws into the target %d, which is not listed", what, pi, p.Target)
			}
			rect, format = image.Rectangle{Min: p.Origin, Max: p.Origin.Add(image.Pt(info.Width, info.Height))}, info.Format
		} else if p.Origin != (image.Point{}) {
			t.Fatalf("%s: pass %d draws into the destination with the origin %v", what, pi, p.Origin)
		}
		if p.Clear != nil && p.Clear.Rect.Intersect(rect).Empty() {
			t.Fatalf("%s: pass %d clears %v of a target that holds %v", what, pi, p.Clear.Rect, rect)
		}
		if p.Clear == nil && len(p.Batches) == 0 && len(p.Uploads) == 0 {
			t.Fatalf("%s: pass %d does nothing", what, pi)
		}
		for _, b := range p.Batches {
			if b.Count <= 0 || b.First < 0 || b.First+b.Count > len(seen) || b.Scissor.Empty() || !b.Scissor.In(rect) {
				t.Fatalf("%s: pass %d into %v: batch %+v", what, pi, rect, b)
			}
			if (b.Pipeline != PipelineFilter && format != FormatRGBA8) || b.Pipeline > PipelineOpaque {
				t.Fatalf("%s: pass %d: pipeline %d in a target of the format %d", what, pi, b.Pipeline, format)
			}
			if b.Image != 0 && b.Sources[0] != 0 {
				t.Fatalf("%s: pass %d: a batch with an image and a first source: %+v", what, pi, b)
			}
			for _, src := range b.Sources {
				switch {
				case src == 0 || (src == SourceDest && p.Target != 0):
				case src == SourceDest || int(src) == p.Target:
					t.Fatalf("%s: pass %d into the target %d reads it", what, pi, p.Target)
				case src < 0 || targets[int(src)].ID == 0:
					t.Fatalf("%s: pass %d reads the target %d, which is not listed", what, pi, src)
				case !drawn[int(src)]:
					t.Fatalf("%s: pass %d reads the target %d before any pass drew into it", what, pi, src)
				}
			}
			for i := b.First; i < b.First+b.Count; i++ {
				if seen[i] {
					t.Fatalf("%s: instance %d is drawn twice", what, i)
				}
				seen[i] = true
				bd := floats(f, i, SlotBounds)
				if !(bd[0] < bd[2] && bd[1] < bd[3]) {
					t.Fatalf("%s: instance %d: bounds %v", what, i, bd)
				}
				if b.Pipeline == PipelineFilter {
					continue
				}
				for slot := 0; slot < InstanceSlots; slot++ {
					if slot == SlotHead || slot == SlotTex {
						continue
					}
					for _, v := range floats(f, i, slot) {
						if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
							t.Fatalf("%s: instance %d, slot %d: %v", what, i, slot, floats(f, i, slot))
						}
					}
				}
				if p.Target == 0 && (bd[0] < 0 || bd[1] < 0 || int(bd[2]) > f.Size.X || int(bd[3]) > f.Size.Y) {
					t.Fatalf("%s: instance %d: bounds %v outside the target %v", what, i, bd, f.Size)
				}
			}
		}
		drawn[p.Target] = true
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("%s: instance %d is in no batch", what, i)
		}
	}
	if len(f.Ramps)%(4*RampWidth) != 0 || len(f.Ramps)/(4*RampWidth) != len(f.RampSegments)/RampSegmentSize {
		t.Fatalf("%s: %d ramp bytes, %d segment values", what, len(f.Ramps), len(f.RampSegments))
	}
	if len(f.Taps)%2 != 0 {
		t.Fatalf("%s: %d tap values", what, len(f.Taps))
	}
}

func TestEncodeScenes(t *testing.T) {
	var e Encoder
	for _, s := range allScenes() {
		dl := s.displayList()
		for _, scale := range []float32{1, 1.5, 2, 0.37} {
			size := image.Pt(int(float32(s.W)*scale)+1, int(float32(s.H)*scale)+1)
			what := fmt.Sprintf("%s x%g", s.Name, scale)
			f := e.Encode(dl, size, Options{Scale: scale, Clear: &s.clear})
			if len(f.Unsupported) != 0 {
				t.Fatalf("%s: not drawn: %v", what, f.Unsupported)
			}
			checkFrame(t, what, f)
			first := append([]byte(nil), f.Instances...)
			passes := len(f.Passes)
			if f = e.Encode(dl, size, Options{Scale: scale, Clear: &s.clear}); !bytes.Equal(f.Instances, first) || len(f.Passes) != passes {
				t.Fatalf("%s: a second encode gives other instances", what)
			}
			// Regions, backdrop filters included: well-formed frames too.
			for _, region := range []image.Rectangle{image.Rect(10, 10, 90, 70), image.Rect(size.X/2, size.Y/3, size.X, size.Y)} {
				checkFrame(t, fmt.Sprintf("%s, region %v", what, region), e.Encode(dl, size, Options{Scale: scale, Region: region}))
			}
			for _, tg := range e.res.targets {
				if tg.busy {
					t.Fatalf("%s: the target %d is still in use after the frame", what, tg.id)
				}
			}
		}
		e.EndFrame()
	}
}

// TestOpenWithoutBackend checks what Open says in a build without a
// device backend.
func TestOpenWithoutBackend(t *testing.T) {
	dev, err := Open(Config{})
	if err == nil {
		dev.Close()
		return // a build with a backend, on a machine with a device
	}
	if hasBackend {
		t.Logf("no device: %v", err)
		return
	}
	if !errors.Is(err, ErrUnavailable) || dev != nil {
		t.Errorf("Open: %v, %v; want ErrUnavailable", dev, err)
	}
}

// TestEncodePlain checks which instances the plain pipelines take: the
// insides of solid fills (PipelineOpaque for an opaque color), of layers
// composited source-over without a mask, and the copies between targets
// (PipelineOpaque, marked as copies); not edges, gradients, images, clips
// or blend modes.
func TestEncodePlain(t *testing.T) {
	var e Encoder
	pipelines := func(f *Frame) map[Pipeline]int {
		n := map[Pipeline]int{}
		for _, p := range f.Passes {
			for _, b := range p.Batches {
				n[b.Pipeline] += b.Count
			}
		}
		return n
	}
	size := image.Pt(300, 200)
	f := e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(10.5, 10, 200, 150), 12), gfx.White.WithAlpha(0.5))
	}), size, Options{})
	checkFrame(t, "translucent", f)
	if n := pipelines(f); n[PipelinePlain] != 1 || n[PipelineOpaque] != 0 || n[PipelineQuad] == 0 {
		t.Errorf("a translucent rounded fill: %v, want its inside plain and its edges quads", n)
	}
	f = e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 300, 200), slate)
	}), size, Options{})
	if n := pipelines(f); n[PipelineOpaque] != 1 || len(n) != 1 {
		t.Errorf("an opaque fill on pixel boundaries: %v, want one opaque instance", n)
	}
	for _, c := range []struct {
		name string
		draw func(b *gfx.Builder)
	}{
		{"a gradient", func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 300, 200), gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(300, 0), Stops: stops(rose, sky)})
		}},
		{"an image", func(b *gfx.Builder) {
			b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(0, 0, 300, 200), gfx.SampleLinear)
		}},
		{"a fill under a rounded clip", func(b *gfx.Builder) {
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(20, 20, 200, 150), 20))
			b.PushClipPath(gfx.CirclePath(gfx.Pt(100, 100), 90), gfx.NonZero)
			b.FillRect(gfx.RectXYWH(0, 0, 300, 200), slate.WithAlpha(0.5))
			b.PopClip()
			b.PopClip()
		}},
		{"a layer with a blend mode", func(b *gfx.Builder) {
			b.PushLayer(1, gfx.Multiply)
			b.FillRect(gfx.RectXYWH(20.5, 20, 200, 150), rose.WithAlpha(0.5))
			b.PopLayer()
		}},
	} {
		f := e.Encode(gfxtest.Build(c.draw), size, Options{})
		checkFrame(t, c.name, f)
		// (The layer's content is drawn plain inside, and the backdrop its
		// blend mode reads is a copy: its composite is not.)
		if n := pipelines(&Frame{Passes: f.Passes[len(f.Passes)-1:]}); n[PipelinePlain] != 0 || n[PipelineOpaque] != 0 {
			t.Errorf("%s: %v, want no plain instance", c.name, n)
		}
	}
	// A layer that fades two shapes is composited plain inside.
	f = e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.PushLayer(0.5, gfx.SrcOver)
		b.FillRect(gfx.RectXYWH(20.5, 20, 200, 150), rose)
		b.FillRect(gfx.RectXYWH(40.5, 40, 200, 150), sky.WithAlpha(0.5))
		b.PopLayer()
	}), size, Options{})
	checkFrame(t, "layer", f)
	last := f.Passes[len(f.Passes)-1]
	if n := pipelines(&Frame{Passes: []Pass{last}}); n[PipelinePlain] != 1 {
		t.Errorf("a layer's composite: %v, want its inside plain", n)
	}
	// A region grown for a backdrop filter is copied back with the opaque
	// pipeline, as a copy.
	f = e.Encode(gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 300, 200), slate)
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(40, 30, 200, 120), 12), gfx.Blur{StdDev: 4})
	}), size, Options{Region: image.Rect(100, 80, 140, 100), Clear: &gfx.White})
	checkFrame(t, "grown", f)
	last = f.Passes[len(f.Passes)-1]
	cb := last.Batches[len(last.Batches)-1]
	if last.Target != 0 || cb.Pipeline != PipelineOpaque || head(f, cb.First)[1]&(1<<25) == 0 || cb.Scissor != image.Rect(100, 80, 140, 100) {
		t.Errorf("the copy of a grown region: %+v (head %v)", cb, head(f, cb.First))
	}
}
