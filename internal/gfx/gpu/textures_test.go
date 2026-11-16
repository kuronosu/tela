//go:build telagpu && (linux || windows)

package gpu

import (
	"errors"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The tests of the textures on a device: atlases that fill up, frames that
// follow each other, renderers that share a device, and what is left when
// things are closed.

// within checks a render against the rasterizer's under the general rule.
func within(t *testing.T, what string, gpu, cpu *image.RGBA) {
	t.Helper()
	if m := compare(gpu, cpu); m.Max > tolHard || m.Over2 > tolShare {
		t.Errorf("%s: largest difference %d at %v, %.3f%% of the pixels over 2 levels", what, m.Max, m.Worst, 100*m.Over2)
	}
}

// TestAtlasLimits renders the texture scenes with atlases far too small
// for them: pictures are cut into many tiles, frames take several passes,
// and the pixels are those of a renderer with room.
func TestAtlasLimits(t *testing.T) {
	limits := Limits{CoveragePage: 128, CoveragePages: 2, ColorPage: 64, ColorPages: 1}
	eachDevice(t, func(t *testing.T, dev Device) {
		label := deviceLabel(dev.Name())
		// One Renderer for every scene: each starts in the atlases the one
		// before left full.
		r := Renderer{}
		r.enc.Limits = limits
		passes := 0
		for i := range textureScenes {
			s := &textureScenes[i]
			dl := s.displayList()
			const scale = 2
			w, h := sceneSize(s, scale)
			target, err := dev.NewTarget(w, h)
			if err != nil {
				t.Fatal(err)
			}
			for round := 0; round < 2; round++ {
				res, err := r.Render(target, dl, Options{Scale: scale, Clear: &s.clear})
				if err != nil {
					t.Fatal(err)
				}
				if res.Unsupported != nil {
					t.Fatalf("%s: not drawn: %v", s.Name, res.Unsupported)
				}
				passes = max(passes, res.Passes)
				gpu := readAll(t, target)
				// Against the same device with room: the same pixels, but
				// for the last bit of a gradient computed from the corner of
				// another tile.
				roomy, _ := renderGPU(t, dev, w, h, dl, Options{Scale: scale, Clear: &s.clear})
				dump(t, fmt.Sprintf("limits_%s_%d_%s", s.Name, round, label), gpu, roomy)
				if m := compare(gpu, roomy); m.Max > 1 {
					t.Errorf("%s, round %d (%d passes): differs from a render with room: largest difference %d at %v, %d pixels",
						s.Name, round, res.Passes, m.Max, m.Worst, m.Over0)
				}
			}
			target.Close()
		}
		if passes < 3 {
			t.Errorf("the frames took at most %d passes: the atlases were not too small", passes)
		}
	})
}

// TestFrames renders frames that follow each other with one Renderer:
// text that scrolls, changes and comes back, with EndFrame between them.
// Every frame is compared with the rasterizer's, and the frames that draw
// what was drawn before upload nothing.
func TestFrames(t *testing.T) {
	const w, h = 480, 320
	lines := []string{"SolA il oSi Aolis", "lAoS ioAl SiAo loS", "oil Sail Also Solo", "AoS*lio%S il@oA", "iSo lAS olA Sio lAi"}
	page := func(scroll, first int, size float32) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) {
			b.FillRect(gfx.RectXYWH(0, 0, 400, 300), gfx.White)
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(10.5, 10.5, 300, 190), 16))
			b.PushTransform(gfx.Translate(0, float32(-scroll)))
			for i := 0; i < 14; i++ {
				text(b, lines[(first+i)%len(lines)], size, 18.3, 30+float32(i)*size*1.3, slate)
			}
			b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(200, 40, 96, 64), gfx.SampleLinear)
			b.StrokePath(gfxtest.StarPath(250, 150, 30), gfx.NewStrokeStyle(3), rose)
			b.PopTransform()
			b.PopClip()
		})
	}
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		cache := raster.NewGlyphCache(8 << 20)
		for _, limits := range []Limits{{}, {CoveragePage: 256, CoveragePages: 2, ColorPage: 128, ColorPages: 1}} {
			r := Renderer{}
			r.enc.Limits, r.enc.GlyphCache = limits, cache
			frame := func(what string, dl *gfx.DisplayList, scale float32) Result {
				t.Helper()
				res, err := r.Render(target, dl, Options{Scale: scale, Clear: &gfx.White})
				if err != nil {
					t.Fatal(err)
				}
				if res.Unsupported != nil {
					t.Fatalf("%s: not drawn: %v", what, res.Unsupported)
				}
				r.EndFrame()
				cpu := image.NewRGBA(image.Rect(0, 0, w, h))
				raster.Render(cpu, dl, raster.Options{Scale: scale, Clear: &gfx.White, GlyphCache: cache})
				within(t, what, readAll(t, target), cpu)
				return res
			}
			if res := frame("the first frame", page(0, 0, 14), 1); res.Uploads == 0 {
				t.Error("the first frame uploads nothing")
			}
			if res := frame("the same frame", page(0, 0, 14), 1); res.Uploads != 0 || res.UploadBytes != 0 {
				t.Errorf("the same frame again uploads %d pictures, %d bytes", res.Uploads, res.UploadBytes)
			}
			// Scrolled by whole pixels: the same pictures at other places.
			if limits == (Limits{}) {
				for _, scroll := range []int{1, 7, 40} {
					if res := frame(fmt.Sprintf("scrolled by %d", scroll), page(scroll, 0, 14), 1); res.Uploads != 0 {
						t.Errorf("scrolled by %d pixels: %d uploads", scroll, res.Uploads)
					}
				}
			}
			// Many frames of other text at other sizes and scales: the atlas
			// grows, fills (with the small limits), evicts; and the first
			// frame comes back.
			uploaded := 0
			for i := 0; i < 40; i++ {
				scale := []float32{1, 1.5, 1.25}[i%3]
				res := frame(fmt.Sprintf("frame %d", i), page(i*13, i, 9+float32(i%17)*1.5), scale)
				uploaded += res.UploadBytes
			}
			if uploaded == 0 {
				t.Error("40 frames of new text uploaded nothing")
			}
			frame("the first frame again", page(0, 0, 14), 1)
			if res := frame("and again", page(0, 0, 14), 1); res.Uploads != 0 {
				t.Errorf("the first frame, drawn twice at the end: %d uploads", res.Uploads)
			}
			// At rest: everything is released; the next frame draws again.
			r.Trim()
			if res := frame("after Trim", page(0, 0, 14), 1); res.Uploads == 0 {
				t.Error("after Trim a frame uploads nothing")
			}
		}
	})
}

// TestResync checks that a device follows its renderers through what
// breaks the order of their frames: two renderers on one device, a frame
// encoded and never drawn, a device trimmed behind a renderer's back.
func TestResync(t *testing.T) {
	const w, h = 300, 200
	scenes := []*gfx.DisplayList{
		gfxtest.Build(func(b *gfx.Builder) {
			text(b, "SolA*il", 30, 10.3, 50, slate)
			b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(150, 20, 96, 64), gfx.SampleLinear)
			b.FillPath(gfxtest.StarPath(60, 130, 50), gfx.EvenOdd, indigo)
		}),
		gfxtest.Build(func(b *gfx.Builder) {
			b.PushClipPath(gfx.CirclePath(gfx.Pt(150, 100), 90), gfx.NonZero)
			text(b, "oAS%lio", 44, 20.6, 110, rose)
			b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(180, 100, 90, 90), gfx.SampleNearest)
			b.PopClip()
		}),
	}
	var cpus [2]*image.RGBA
	for i, dl := range scenes {
		cpus[i] = renderCPU(w, h, dl, 1, gfx.White)
	}
	eachDevice(t, func(t *testing.T, dev Device) {
		var targets [2]Target
		for i := range targets {
			var err error
			if targets[i], err = dev.NewTarget(w, h); err != nil {
				t.Fatal(err)
			}
			defer targets[i].Close()
		}
		var rs [2]Renderer
		draw := func(what string, r *Renderer, ti, si int) Result {
			t.Helper()
			res, err := r.Render(targets[ti], scenes[si], Options{Clear: &gfx.White})
			if err != nil {
				t.Fatalf("%s: %v", what, err)
			}
			within(t, what, readAll(t, targets[ti]), cpus[si])
			return res
		}
		// Two renderers taking turns on the device, on both targets.
		for i := 0; i < 6; i++ {
			draw(fmt.Sprintf("turn %d", i), &rs[i%2], i%2, (i/2)%2)
		}
		// One renderer for both targets, as intended: nothing is uploaded
		// twice.
		r := &rs[0]
		draw("both scenes", r, 0, 0)
		draw("both scenes", r, 1, 1)
		if a, b := draw("again", r, 1, 0), draw("again", r, 0, 1); a.Uploads != 0 || b.Uploads != 0 {
			t.Errorf("one renderer on two targets uploads again: %d and %d pictures", a.Uploads, b.Uploads)
		}
		// A frame that uploads is encoded and not drawn: the device refuses
		// the next one, and the renderer starts over.
		r.enc.Trim()
		f := r.enc.Encode(scenes[0], image.Pt(w, h), Options{})
		if f.Res.Next == f.Res.Base {
			t.Fatal("the lost frame uploads nothing")
		}
		f = r.enc.Encode(scenes[1], image.Pt(w, h), Options{Clear: &gfx.White})
		if err := targets[0].Draw(f); !errors.Is(err, ErrResourcesLost) {
			t.Errorf("a frame after a lost one: %v, want ErrResourcesLost", err)
		}
		draw("after a lost frame", r, 0, 1)
		draw("after a lost frame", r, 1, 0)
		// The device is trimmed; then the renderer too.
		dev.Trim()
		if res := draw("after the device was trimmed", r, 0, 0); res.Uploads == 0 {
			t.Error("after the device was trimmed a frame uploads nothing")
		}
		draw("after the device was trimmed", r, 1, 1)
		r.Trim()
		r.Trim()
		draw("after Trim", r, 1, 0)
		draw("after Trim", r, 0, 1)
	})
}

// TestTextureLifetime leaves textures of every kind on a device when it is
// closed, and closes things in the wrong order: the validation layer
// reports any object left alive (TELA_VK_VALIDATE=1).
func TestTextureLifetime(t *testing.T) {
	dl := gfxtest.Build(func(b *gfx.Builder) {
		text(b, "SolA*%il", 30, 10.3, 50, slate)
		for i := 0; i < 5; i++ {
			img := photo.Level(i)
			b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(10+float32(i)*50, 70, 40, 30), gfx.SampleLinear)
		}
		b.PushClipPath(gfx.CirclePath(gfx.Pt(150, 150), 40), gfx.NonZero)
		b.FillPath(gfxtest.StarPath(150, 150, 50), gfx.NonZero, indigo)
		b.PopClip()
	})
	// More images than one descriptor pool serves.
	many := gfxtest.Build(func(b *gfx.Builder) {
		for i := 0; i < imageSetsPerPool+10; i++ {
			px := image.NewRGBA(image.Rect(0, 0, 2, 2))
			for k := range px.Pix {
				px.Pix[k] = uint8(255 - i)
			}
			b.DrawImage(gfx.NewImage(px), gfx.Rect{}, gfx.RectXYWH(float32(i%20)*12, float32(i/20)*12, 10, 10), gfx.SampleNearest)
		}
	})
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(300, 200)
		if err != nil {
			t.Fatal(err)
		}
		var r Renderer
		for i := 0; i < 2; i++ {
			if res, err := r.Render(target, dl, Options{Clear: &gfx.White}); err != nil || res.Unsupported != nil {
				t.Fatal(err, res.Unsupported)
			}
			within(t, "the scene", readAll(t, target), renderCPU(300, 200, dl, 1, gfx.White))
			res, err := r.Render(target, many, Options{Clear: &gfx.White})
			if err != nil {
				t.Fatal(err)
			}
			if want := imageSetsPerPool + 10; i == 0 && res.Uploads != want {
				t.Errorf("%d images created, want %d", res.Uploads, want)
			}
			within(t, "many images", readAll(t, target), renderCPU(300, 200, many, 1, gfx.White))
			// Frames end until the images are given up, and the device
			// destroys them with a frame that draws none.
			for k := 0; k <= idleFrames+1; k++ {
				r.EndFrame()
			}
			if _, err := r.Render(target, nil, Options{Clear: &gfx.White}); err != nil {
				t.Fatal(err)
			}
		}
		// Atlases, sprites, images and their pools are alive now.
		if _, err := r.Render(target, dl, Options{Clear: &gfx.White}); err != nil {
			t.Fatal(err)
		}
		if _, err := r.Render(target, many, Options{Clear: &gfx.White}); err != nil {
			t.Fatal(err)
		}
		// A second device-side trim, a target closed before the device,
		// and the device closed with everything on it; then the renderer
		// trimmed after its device is gone.
		target.Close()
		other, err := dev.NewTarget(64, 64)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Render(other, dl, Options{Clear: &gfx.White}); err != nil {
			t.Fatal(err)
		}
		dev.Close()
		r.Trim()
		dev.Trim()
		if _, err := r.Render(other, dl, Options{}); err == nil {
			t.Error("a target of a closed device was drawn into")
		}
	})
}

// layeredScene is a frame with every kind of offscreen target: a layer, a
// mask, a blend mode's copy of the backdrop, the steps of filters at full
// and at reduced resolution, a backdrop filter.
func layeredScene(shift float32) *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) {
		busy(b, gfx.RectXYWH(0, 0, 300, 200))
		b.PushLayer(0.8, gfx.Multiply, gfx.Blur{StdDev: 2}, gfx.Saturate(1.5))
		b.FillRRect(gfx.NewRRect(gfx.RectXYWH(20+shift, 20, 110, 70), 12), amber)
		text(b, "SoAl", 30, 30+shift, 70, indigo)
		b.PopLayer()
		mask := maskOf(gfx.MaskAlpha, func(m *gfx.Builder) { text(m, "iSo", 60, 150, 80, gfx.Black) })
		b.PushMaskLayer(mask, 1, gfx.SrcOver, gfx.DropShadow{Offset: gfx.Pt(2, 3), StdDev: 9, Color: gfx.Black.WithAlpha(0.6)})
		b.FillRect(gfx.RectXYWH(140, 20, 150, 80), gfx.LinearGradient{Start: gfx.Pt(140, 20), End: gfx.Pt(290, 100), Stops: stops(rose, sky)})
		b.PopLayer()
		glass(b, gfx.NewRRect(gfx.RectXYWH(40+shift, 100, 200, 80), 16), 10, gfx.White.WithAlpha(0.2))
		text(b, "olAS iolS", 22, 60+shift, 150, slate)
	})
}

// TestTargetLifetime follows the offscreen targets on the device through
// frames, EndFrame, Trim and Close: the device holds the targets the
// frames list, no more, and none is left when it is closed (which the
// validation layer checks, TELA_VK_VALIDATE=1).
func TestTargetLifetime(t *testing.T) {
	const w, h = 300, 200
	plain := gfxtest.Build(func(b *gfx.Builder) { busy(b, gfx.RectXYWH(0, 0, 300, 200)) })
	eachDevice(t, func(t *testing.T, dev Device) {
		vd := dev.(*vkDevice)
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		var r Renderer
		frame := func(what string, dl *gfx.DisplayList, opts Options) Result {
			t.Helper()
			res, err := r.Render(target, dl, opts)
			if err != nil {
				t.Fatalf("%s: %v", what, err)
			}
			if res.Unsupported != nil {
				t.Fatalf("%s: not drawn: %v", what, res.Unsupported)
			}
			if len(vd.offscreen) != len(r.enc.res.targets) {
				t.Errorf("%s: the device holds %d offscreen targets, the renderer's pool %d", what, len(vd.offscreen), len(r.enc.res.targets))
			}
			return res
		}
		dl := layeredScene(0)
		frame("the first frame", dl, Options{Clear: &gfx.White})
		within(t, "the first frame", readAll(t, target), renderCPU(w, h, dl, 1, gfx.White))
		n := len(vd.offscreen)
		if n < 4 {
			t.Fatalf("a frame of layers and filters left %d offscreen targets", n)
		}
		// The same frame, and the same moved a little, draw with the same
		// targets and upload nothing.
		for i := 0; i < 5; i++ {
			res := frame("a frame again", layeredScene(float32(i%2)), Options{Clear: &gfx.White})
			r.EndFrame()
			if len(vd.offscreen) != n || (i > 1 && res.Uploads != 0) {
				t.Errorf("frame %d: %d offscreen targets (the first frame left %d), %d uploads", i, len(vd.offscreen), n, res.Uploads)
			}
		}
		// A region inside the glass: one more target, for the grown region.
		frame("a region", dl, Options{Clear: &gfx.White, Region: image.Rect(100, 130, 140, 150)})
		within(t, "a region", readAll(t, target), renderCPU(w, h, dl, 1, gfx.White))
		// Frames without layers: the targets go once they were idle long
		// enough, and the device destroys them with the next frame.
		for i := 0; i <= idleFrames+1; i++ {
			r.EndFrame()
		}
		frame("a frame without layers", plain, Options{Clear: &gfx.White})
		if len(vd.offscreen) != 0 {
			t.Errorf("%d offscreen targets after %d frames without layers", len(vd.offscreen), idleFrames)
		}
		// Layers again, then at rest.
		frame("layers again", dl, Options{Clear: &gfx.White})
		within(t, "layers again", readAll(t, target), renderCPU(w, h, dl, 1, gfx.White))
		r.Trim()
		if len(vd.offscreen) != 0 {
			t.Errorf("%d offscreen targets after Trim", len(vd.offscreen))
		}
		// A frame lost between two frames of layers: the device starts over
		// with the renderer.
		frame("after Trim", dl, Options{Clear: &gfx.White})
		r.enc.Encode(layeredScene(40), image.Pt(w, h), Options{})
		frame("after a lost frame", dl, Options{Clear: &gfx.White})
		within(t, "after a lost frame", readAll(t, target), renderCPU(w, h, dl, 1, gfx.White))
		// The device is closed with its offscreen targets alive, and the
		// target after it.
		if len(vd.offscreen) == 0 {
			t.Error("no offscreen target is alive when the device is closed")
		}
		dev.Close()
		target.Close()
	})
}

// TestKeepsPixelsLayers checks that layers, blend modes and backdrop
// filters composite over what the target holds from earlier renders, as
// the rasterizer does over its image: a render without Clear, whole and
// of a region that its backdrop filters read beyond.
func TestKeepsPixelsLayers(t *testing.T) {
	const w, h = 300, 200
	first := gfxtest.Build(func(b *gfx.Builder) { busy(b, gfx.RectXYWH(0, 0, 300, 200)) })
	second := layeredScene(0)
	third := gfxtest.Build(func(b *gfx.Builder) {
		// Nothing opaque under the filters: they read the earlier frames.
		b.BackdropFilter(gfx.NewRRect(gfx.RectXYWH(30, 30, 240, 140), 20), gfx.Blur{StdDev: 6}, gfx.Contrast(1.3))
		b.PushLayer(0.7, gfx.Difference)
		b.FillPath(gfx.CirclePath(gfx.Pt(150, 100), 60), gfx.NonZero, sky)
		b.PopLayer()
		b.BackdropFilterOpacity(gfx.NewRRect(gfx.RectXYWH(100, 60, 150, 100), 30), 0.6, gfx.Blur{StdDev: 14})
	})
	eachDevice(t, func(t *testing.T, dev Device) {
		target, err := dev.NewTarget(w, h)
		if err != nil {
			t.Fatal(err)
		}
		defer target.Close()
		var r Renderer
		cpu := image.NewRGBA(image.Rect(0, 0, w, h))
		for i, c := range []struct {
			dl     *gfx.DisplayList
			region image.Rectangle
		}{{first, image.Rectangle{}}, {second, image.Rectangle{}}, {third, image.Rect(120, 70, 200, 120)}, {third, image.Rect(0, 0, 150, 200)}, {third, image.Rectangle{}}} {
			if _, err := r.Render(target, c.dl, Options{Region: c.region}); err != nil {
				t.Fatal(err)
			}
			raster.Render(cpu, c.dl, raster.Options{Region: c.region})
			gpu := readAll(t, target)
			m := compare(gpu, cpu)
			dump(t, fmt.Sprintf("keeps_layers_%d_%s", i, deviceLabel(dev.Name())), gpu, cpu)
			t.Logf("after render %d: largest difference %d, %.3f%% of the pixels over 1 level, %.3f%% over 2", i+1, m.Max, 100*m.Over1, 100*m.Over2)
			// The differences of a level of each render add up, and the
			// filters of the next one read them.
			if m.Max > tolHard || m.Over2 > 4*tolShare {
				t.Errorf("after render %d: largest difference %d at %v, %.3f%% of the pixels over 2 levels", i+1, m.Max, m.Worst, 100*m.Over2)
			}
		}
	})
}

// glassPage is a page of two glass panes over an opaque background with a
// horizontal stripe at stripe under them, labels painted after the
// filters, and, when card is not empty, an opaque card painted over
// everything.
func glassPage(stripe, label float32, card gfx.Rect) *gfx.DisplayList {
	return gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 400, 300), slate)
		b.FillRect(gfx.RectXYWH(0, stripe, 400, 12), rose)
		b.FillRect(gfx.RectXYWH(130, 0, 14, 300), amber)
		for i, x := range []float32{20, 210} {
			box := gfx.NewRRect(gfx.RectXYWH(x, 20, 170, 260), 16)
			b.BackdropFilter(box, gfx.Blur{StdDev: 8})
			b.FillRRect(box, gfx.White.WithAlpha(0.3))
			b.FillRect(gfx.RectXYWH(x+20+label*float32(i+1), 60, 40, 12), emerald)
		}
		if !card.Empty() {
			b.FillRect(card, indigo)
		}
	})
}

// TestBackdropCachePixels renders sequences of frames into one target
// with a BackdropCache, each frame as the regions of its damage, and
// checks after every frame that the target holds the pixels of a full
// render of its list without a cache, bit for bit: filters drawn from
// their entries, entries patched by the renders of a frame whose damage
// changed their input, entries made by a full frame or by a filter whose
// input rested, and entries under covers.
func TestBackdropCachePixels(t *testing.T) {
	type frame struct {
		what        string
		dl          *gfx.DisplayList
		prev, patch []int
		regions     []gfx.Rect // logical; none: a full frame
	}
	same, changed := []int{0, 1}, []int{-1, -1}
	// The output of the filters within their reach (3 sigma, rounded up)
	// of a change of rows y0 to y1 under both panes.
	rows := func(y0, y1 float32) gfx.Rect { return gfx.Rect{X0: 0, Y0: y0 - 30, X1: 400, Y1: y1 + 30} }
	card, band := gfx.RectXYWH(0, 10, 400, 280), gfx.RectXYWH(0, 100, 400, 100)
	sequences := []struct {
		name   string
		frames []frame
		want   BackdropStats // what the renders of the sequence did
	}{
		{"hits and patches", []frame{
			{"full", glassPage(200, 0, gfx.Rect{}), nil, nil, nil},
			{"a label moves", glassPage(200, 4, gfx.Rect{}), same, nil, []gfx.Rect{gfx.RectXYWH(40, 60, 70, 12), gfx.RectXYWH(230, 60, 80, 12)}},
			{"the stripe moves", glassPage(210, 4, gfx.Rect{}), changed, same, []gfx.Rect{rows(200, 222)}},
			{"a label again", glassPage(210, 8, gfx.Rect{}), same, nil, []gfx.Rect{gfx.RectXYWH(40, 60, 70, 12), gfx.RectXYWH(230, 60, 90, 12)}},
			{"the stripe in two regions", glassPage(240, 8, gfx.Rect{}), changed, same, []gfx.Rect{rows(210, 222), rows(240, 252)}},
			{"a full frame", glassPage(240, 8, gfx.Rect{}), same, nil, nil},
		}, BackdropStats{Hits: 6, Patched: 6, Stored: 2}},
		{"a patch under a cover", []frame{
			// The filters are hidden by the card in the render of the
			// stripe, whose region they grow: the card covers the panes.
			{"full", glassPage(140, 0, gfx.Rect{}), nil, nil, nil},
			{"a card comes", glassPage(140, 0, card), same, nil, []gfx.Rect{card}},
			{"the stripe moves under the card", glassPage(150, 0, card), changed, same, []gfx.Rect{rows(140, 162)}},
			{"the card goes", glassPage(150, 0, gfx.Rect{}), same, nil, []gfx.Rect{card}},
		}, BackdropStats{Stored: 4}}, // the entries left behind are made again
		{"a patch under a cover, in part", []frame{
			// The render of the stripe shows the filters around the card.
			{"full, with a card", glassPage(140, 0, band), nil, nil, nil},
			{"the stripe moves under the card", glassPage(150, 0, band), changed, same, []gfx.Rect{rows(140, 162)}},
			{"the card goes", glassPage(150, 0, gfx.Rect{}), same, nil, []gfx.Rect{band}},
		}, BackdropStats{Hits: 2, Patched: 2, Stored: 2}},
		{"entries made after a rest", []frame{
			{"full, another list", glassPage(60, 0, gfx.Rect{}), nil, nil, nil},
			{"a list the cache was not moved to", glassPage(60, 3, gfx.Rect{}), nil, nil, []gfx.Rect{gfx.RectXYWH(40, 60, 70, 12), gfx.RectXYWH(230, 60, 80, 12)}},
			{"the input rested", glassPage(60, 6, gfx.Rect{}), same, nil, []gfx.Rect{gfx.RectXYWH(40, 60, 70, 12), gfx.RectXYWH(230, 60, 90, 12)}},
			{"drawn from the entry made", glassPage(60, 9, gfx.Rect{}), same, nil, []gfx.Rect{gfx.RectXYWH(40, 60, 80, 12), gfx.RectXYWH(230, 60, 90, 12)}},
		}, BackdropStats{Hits: 2, Stored: 4, Computed: 2}},
	}
	white := gfx.White
	eachDevice(t, func(t *testing.T, dev Device) {
		for _, scale := range []float32{1, 2, 0} {
			// Scale 0: scale 1 with another Renderer drawing on the device
			// between the frames, which makes it start over: what the cache
			// held is gone, and the Renderer finds it out.
			intrude := scale == 0
			if intrude {
				scale = 1
			}
			w, h := int(400*scale), int(300*scale)
			for _, sq := range sequences {
				target, err := dev.NewTarget(w, h)
				if err != nil {
					t.Fatal(err)
				}
				ref, err := dev.NewTarget(w, h)
				if err != nil {
					t.Fatal(err)
				}
				var r Renderer
				var bc BackdropCache
				var glass BackdropStats
				for fi, fr := range sq.frames {
					if fr.prev != nil || fr.regions == nil {
						bc.Retain(fr.dl, scale, fr.prev, fr.patch)
					}
					regions := []image.Rectangle{{}}
					if fr.regions != nil {
						regions = regions[:0]
						for _, lr := range fr.regions {
							regions = append(regions, image.Rect(int(lr.X0*scale), int(lr.Y0*scale), int(math.Ceil(float64(lr.X1*scale))), int(math.Ceil(float64(lr.Y1*scale)))).Intersect(image.Rect(0, 0, w, h)))
						}
					}
					for _, region := range regions {
						res, err := r.Render(target, fr.dl, Options{Scale: scale, Region: region, Clear: &white, Backdrops: &bc})
						if err != nil {
							t.Fatal(err)
						}
						glass.Hits += res.Backdrops.Hits
						glass.Patched += res.Backdrops.Patched
						glass.Stored += res.Backdrops.Stored
						glass.Computed += res.Backdrops.Computed
						if testing.Verbose() {
							t.Logf("x%g, %s, %s, %v: %+v, %d hidden", scale, sq.name, fr.what, region, res.Backdrops, res.Culled)
						}
					}
					r.EndFrame()
					got := readAll(t, target)
					// By the same Renderer, which a device follows (another one
					// would make it start over), into a target of its own.
					if _, err := r.Render(ref, fr.dl, Options{Scale: scale, Clear: &white}); err != nil {
						t.Fatal(err)
					}
					want := readAll(t, ref)
					if intrude {
						renderGPU(t, dev, w, h, fr.dl, Options{Scale: scale, Clear: &white})
					}
					if m := compare(got, want); m.Max != 0 {
						dump(t, fmt.Sprintf("bdcache_%s_x%g_%d_%s", strings.ReplaceAll(sq.name, " ", "_"), scale, fi, deviceLabel(dev.Name())), got, want)
						t.Errorf("x%g, %s, %s: differs from a full render without a cache: largest difference %d at %v, %d pixels (cache %+v)",
							scale, sq.name, fr.what, m.Max, m.Worst, m.Over0, glass)
					}
				}
				if glass != sq.want && !intrude {
					t.Errorf("x%g, %s: the renders did %+v with the cache, want %+v", scale, sq.name, glass, sq.want)
				}
				bc.Reset()
				r.Trim()
				target.Close()
				ref.Close()
			}
		}
	})
}
