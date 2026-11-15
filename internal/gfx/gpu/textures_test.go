//go:build telagpu && (linux || windows)

package gpu

import (
	"errors"
	"fmt"
	"image"
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
	if m := compare(gpu, cpu); m.max > tolHard || m.over2 > tolShare {
		t.Errorf("%s: largest difference %d at %v, %.3f%% of the pixels over 2 levels", what, m.max, m.worst, 100*m.over2)
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
				if m := compare(gpu, roomy); m.max > 1 {
					t.Errorf("%s, round %d (%d passes): differs from a render with room: largest difference %d at %v, %d pixels",
						s.Name, round, res.Passes, m.max, m.worst, m.over0)
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
