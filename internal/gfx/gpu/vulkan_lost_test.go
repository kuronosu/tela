//go:build telagpu && linux

package gpu

import (
	"errors"
	"image"
	"math/rand"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/vk"
)

// TestLostInFlight loses a device in every call that submits work, by a wait
// that times out at once, so that the submission is in flight: whatever the
// caller then gives up (its textures, its targets, its exported images, its
// imports) is there until Close, which waits. The validation layer reports
// an object destroyed in use (TELA_VK_VALIDATE=1).
func TestLostInFlight(t *testing.T) {
	// A scene with what a frame makes: ramps, coverage in the atlas, a layer
	// and a backdrop filter in targets, images, imports.
	scene := func(n int, imgs ...*gfx.Image) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) {
			for i := range n {
				g := gfx.LinearGradient{Start: gfx.Pt(0, 0), End: gfx.Pt(200, 0), Stops: []gfx.GradientStop{{Offset: 0, Color: sky}, {Offset: 1, Color: rose.WithAlpha(float32(i+1) / 50)}}}
				b.FillRect(gfx.RectXYWH(0, float32(3*i), 200, 3), g)
			}
			b.FillPath(gfx.CirclePath(gfx.Pt(100, 60), float32(20+n)), gfx.NonZero, rose)
			b.PushLayer(0.5, gfx.SrcOver)
			for i, img := range imgs {
				b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(float32(10+20*i), 10, 90, 70), gfx.SampleLinear)
			}
			b.PopLayer()
			glass(b, gfx.NewRRect(gfx.RectXYWH(20, 20, 120, 60), 12), 6, gfx.White.WithAlpha(0.2))
		})
	}
	type state struct {
		app, win   *vkDevice
		target     Target
		r          Renderer
		own, other *Imported // sampled; copied
		exp        Export
		frame      func(n int) *gfx.DisplayList
		// images and offscreen are what the device holds before the call that
		// loses it.
		images, offscreen int
	}
	for _, op := range []struct {
		name string
		do   func(t *testing.T, s *state) error
	}{
		{"Draw", func(t *testing.T, s *state) error {
			_, err := s.r.Render(s.target, s.frame(2), Options{Clear: &gfx.White})
			return err
		}},
		// The first submission of a frame, that makes a larger texture of
		// ramps.
		{"Ramps", func(t *testing.T, s *state) error {
			_, err := s.r.Render(s.target, s.frame(40), Options{Clear: &gfx.White})
			return err
		}},
		{"Profiled", func(t *testing.T, s *state) error {
			s.win.SetProfiling(true)
			_, err := s.r.Render(s.target, s.frame(2), Options{Clear: &gfx.White})
			return err
		}},
		// The first submission of a Renderer, that makes an atlas.
		{"Atlas", func(t *testing.T, s *state) error {
			var r Renderer
			s.win.waitErr = nil
			s.r.Trim()
			s.win.waitErr = func() error { return vk.Timeout }
			s.images, s.offscreen = 0, 0
			_, err := r.Render(s.target, s.frame(2), Options{Clear: &gfx.White})
			return err
		}},
		{"NewTarget", func(t *testing.T, s *state) error {
			_, err := s.win.NewTarget(64, 64)
			return err
		}},
		{"ReadPixels", func(t *testing.T, s *state) error {
			return s.target.ReadPixels(image.NewRGBA(image.Rect(0, 0, 200, 120)), image.Rect(0, 0, 200, 120))
		}},
		{"Shift", func(t *testing.T, s *state) error { return s.target.Shift(image.Rect(0, 0, 100, 100), 3, 5) }},
		{"CopyFrom", func(t *testing.T, s *state) error { return s.exp.CopyFrom(s.target, image.Rect(0, 0, 200, 120)) }},
		{"Trim", func(t *testing.T, s *state) error {
			// The textures go before its first submission.
			s.r.Trim()
			s.images, s.offscreen = 0, 0
			return s.win.usable()
		}},
		{"Pixels", func(t *testing.T, s *state) error {
			if s.own.Pixels() != nil {
				t.Error("a read whose wait timed out gave pixels")
			}
			return s.win.usable()
		}},
		{"Pixels copied", func(t *testing.T, s *state) error {
			if s.other.Pixels() != nil {
				t.Error("a read whose wait timed out gave pixels")
			}
			return s.win.usable()
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			eachPair(t, onGPU, func(t *testing.T, app, win *vkDevice) {
				s := &state{app: app, win: win}
				var err error
				if s.target, err = win.NewTarget(200, 120); err != nil {
					t.Fatal(err)
				}
				formats, _ := win.ImportFormats()
				b := newAppBuffer(t, app, 90, 70, formats[0], false)
				b.draw(appPicture(90, 70, 1))
				if s.own, err = win.Import(b.desc()); err != nil {
					t.Fatal(err)
				}
				// Rows no device samples where they lie.
				odd := newAppBuffer(t, app, 64, 80, DmabufFormat{Fourcc: FourccABGR8888}, false)
				odd.draw(appPicture(64, 80, 2))
				desc := odd.desc()
				desc.Width, desc.Height, desc.Planes[0].Offset = 32, 60, desc.Planes[0].Stride+4
				if s.other, err = win.Import(desc); err != nil {
					t.Fatal(err)
				}
				if s.exp, err = win.NewExport(200, 120, win.ExportFormats()); err != nil {
					t.Fatal(err)
				}
				s.frame = func(n int) *gfx.DisplayList {
					pic := appPicture(80, 60, n)
					return scene(n, gfx.NewExternalImage(90, 70, false, s.own.Frame()), gfx.NewExternalImage(32, 60, false, s.other.Frame()),
						gfx.NewStreamImage(80, 60, pic.Pix, false, 9), gfx.NewImage(pic))
				}
				if _, err := s.r.Render(s.target, s.frame(1), Options{Clear: &gfx.White}); err != nil {
					t.Fatal(err)
				}
				if err := s.exp.CopyFrom(s.target, image.Rect(0, 0, 200, 120)); err != nil {
					t.Fatal(err)
				}
				waits := 0
				win.waitErr = func() error {
					waits++
					return vk.Timeout
				}
				s.images, s.offscreen = len(win.images), len(win.offscreen)
				targets, exports := len(win.targets), len(win.exports)
				if s.images < 2 || s.offscreen == 0 || targets != 1 || exports != 1 {
					t.Fatalf("the device holds %d images, %d offscreen targets, %d targets and %d exports: the test needs some", s.images, s.offscreen, targets, exports)
				}
				if err := op.do(t, s); !errors.Is(err, ErrDeviceLost) || !win.Lost() {
					t.Fatalf("%v (lost %v), want a lost device", err, win.Lost())
				}
				before := waits
				// What a caller that fell back does, before it closes the
				// device.
				s.r.EndFrame()
				s.r.Rest()
				s.r.Trim()
				s.target.Close()
				s.exp.Close()
				s.own.Close()
				s.other.Close()
				if len(win.images) < s.images || len(win.offscreen) < s.offscreen || len(win.targets) < targets || len(win.exports) < exports {
					t.Errorf("a lost device gave up what its work may use: %d of %d images, %d of %d offscreen targets, %d of %d targets, %d of %d exports are left",
						len(win.images), s.images, len(win.offscreen), s.offscreen, len(win.targets), targets, len(win.exports), exports)
				}
				if len(win.held) < 2 {
					t.Errorf("%d objects held for Close, want the two imports at least", len(win.held))
				}
				if err := s.target.Shift(image.Rect(0, 0, 10, 10), 1, 1); !errors.Is(err, ErrDeviceLost) {
					t.Errorf("a call on the target closed: %v", err)
				}
				if waits != before && op.name != "Atlas" { // which sets a wait of its own
					t.Errorf("%d submissions after the loss", waits-before)
				}
				win.Close()
				if len(win.held) != 0 || s.own.Pixels() != nil {
					t.Errorf("after Close: %d objects held", len(win.held))
				}
				s.target.Close()
				s.exp.Close()
			})
		})
	}
}

// TestImportGoroutines imports, closes and reads buffers from several
// goroutines while the device draws frames with other imports: the calls
// keep apart (go test -race), no frame fails and its pixels are right, and
// nothing is left.
func TestImportGoroutines(t *testing.T) {
	eachPair(t, onGPU, func(t *testing.T, app, win *vkDevice) {
		const w, h = 64, 48
		formats, _ := win.ImportFormats()
		b := newAppBuffer(t, app, w, h, formats[0], false)
		pic := appPicture(w, h, 1)
		b.draw(pic)
		odd := newAppBuffer(t, app, w, h+1, DmabufFormat{Fourcc: FourccABGR8888}, false)
		odd.draw(appPicture(w, h+1, 2))
		oddDesc := odd.desc()
		oddDesc.Width, oddDesc.Height, oddDesc.Planes[0].Offset = w/2, h, 4
		descs := []DmabufDescriptor{b.desc(), oddDesc}
		shown, err := win.Import(b.desc())
		if err != nil {
			t.Fatal(err)
		}
		target, err := win.NewTarget(160, 120)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		stop := make(chan struct{})
		for g := range 4 {
			rng := rand.New(rand.NewSource(int64(g)))
			wg.Go(func() {
				var held []*Imported
				for {
					select {
					case <-stop:
						for _, m := range held {
							m.Close()
						}
						return
					default:
					}
					m, err := win.Import(descs[rng.Intn(2)])
					if err != nil {
						t.Error(err)
						return
					}
					if own, foreign := win.ImportFormats(); len(own) == 0 || len(foreign) == 0 || win.Lost() || win.DeviceID().Name == "" {
						t.Error("the device does not say what it imports")
					}
					if rng.Intn(4) == 0 && m.Pixels() == nil {
						t.Error("an open buffer has no pixels")
					}
					if held = append(held, m); len(held) > 3 {
						i := rng.Intn(len(held))
						held[i].Close()
						held[i].Close()
						if held[i].Pixels() != nil {
							t.Error("a closed buffer has pixels")
						}
						held = append(held[:i], held[i+1:]...)
					}
				}
			})
		}
		dl := func(img *gfx.Image) *gfx.DisplayList {
			return gfxtest.Build(func(b *gfx.Builder) {
				b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(20, 20, w, h), gfx.SampleLinear)
				b.PushLayer(0.6, gfx.SrcOver)
				b.DrawImage(img, gfx.Rect{}, gfx.RectXYWH(60, 50, w, h), gfx.SampleLinear)
				b.PopLayer()
			})
		}
		want := renderCPU(160, 120, dl(gfx.NewImage(asImported(pic, formats[0]))), 1, gfx.White)
		var r Renderer
		for n := range 150 {
			// The textures of images come and go with those of the imports.
			list := dl(gfx.NewExternalImage(w, h, false, shown.Frame()))
			if n%3 == 0 {
				small := appPicture(20, 20, n)
				if _, err := r.Render(target, dl(gfx.NewStreamImage(20, 20, small.Pix, false, 4)), Options{Clear: &gfx.White}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := r.Render(target, list, Options{Clear: &gfx.White}); err != nil {
				t.Fatal(err)
			}
			r.EndFrame()
			switch {
			case n%50 == 49:
				r.Trim()
			case n%20 == 19:
				r.Rest()
			}
		}
		within(t, "drawn while others import", readAll(t, target), want)
		close(stop)
		wg.Wait()
		if len(win.imports) != 1 {
			t.Errorf("%d imports left", len(win.imports))
		}
		// The device closes under the imports.
		for range 3 {
			wg.Go(func() {
				for {
					m, err := win.Import(b.desc())
					if err != nil {
						return
					}
					m.Pixels()
					m.Close()
				}
			})
		}
		shown.Close()
		target.Close()
		win.Close()
		wg.Wait()
	})
}
