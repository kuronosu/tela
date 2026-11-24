package main

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/testutil/idle"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1180, 660

// mount opens the screen in the window of main. Frames are drawn on a
// goroutine, so the waits are generous for slow machines.
func mount(t *testing.T, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = root
	opts = append([]telatest.Option{telatest.Size(windowW, windowH), telatest.Timeout(30 * time.Second)}, opts...)
	return telatest.MountApp(t, app, opts...)
}

// still is a preview.
func still(s Scene) tela.Component { return Previews[scenes[s].name].Root }

// TestGolden is the previews, at scale 1 and 2: the scenes four seconds in,
// drawn in software at the size they are shown at.
func TestGolden(t *testing.T) {
	for s := range scenes {
		for _, scale := range []float64{1, 2} {
			name := fmt.Sprintf("%s_x%g", strings.ToLower(scenes[s].name), scale)
			t.Run(name, func(t *testing.T) {
				mount(t, still(Scene(s)), telatest.Scale(scale)).Golden("texture_" + name + ".png")
			})
		}
	}
}

// TestGoldenSizes is the scene that fills its side of the window, in a short
// window too, the small one, and a frame of a quarter of the size, stretched, in
// the smallest window.
func TestGoldenSizes(t *testing.T) {
	for _, c := range []struct {
		name string
		root Demo
		w, h float32
	}{
		{"full", Demo{Still: true, Size: Full, At: 4 * time.Second}, windowW, windowH},
		{"short", Demo{Still: true, Scene: Satin, Size: Full, At: 4 * time.Second}, 1400, 600},
		{"small", Demo{Still: true, Scene: Mountains, Size: Small, At: 4 * time.Second}, windowW, windowH},
		{"narrow", Demo{Still: true, Scene: Satin, Percent: 25, At: 4 * time.Second}, 760, 600},
	} {
		t.Run(c.name, func(t *testing.T) {
			mount(t, c.root, telatest.Size(c.w, c.h)).Golden("texture_" + c.name + "_x1.png")
		})
	}
}

// TestSizes mounts the window at other sizes, below the smallest one of the
// app too, which a tiling window manager can give it: dev mode is on, so any
// dev message fails the test. The controls are on the right of the scene, whole
// and in the window, and nothing scrolls.
func TestSizes(t *testing.T) {
	last := telatest.ByRole("radio", "Software")
	for _, s := range [][2]float32{{windowW, windowH}, {760, 600}, {1600, 1000}, {1600, 420}, {720, 360}} {
		for _, size := range []CardSize{Small, Medium, Full} {
			app := App
			app.Root, app.MinWidth, app.MinHeight = Demo{Still: true, Size: size}, 0, 0
			a := telatest.MountApp(t, app, telatest.Size(s[0], s[1]), telatest.Timeout(30*time.Second))
			what := fmt.Sprintf("%v, %s", s, size)
			r := a.Bounds(scene(Planet))
			if d := r.Width/r.Height - 16.0/9; d < -0.01 || d > 0.01 {
				t.Errorf("%s: the scene is %v", what, r)
			}
			if r.X < 0 || r.Y < 0 || r.Y+r.Height > s[1] {
				t.Errorf("%s: the scene is cut off: %v", what, r)
			}
			play := a.Bounds(telatest.ByRole("button", "Play"))
			a.ScrollAt(s[0]/2, s[1]/2, 0, 2000)
			a.ScrollAt(s[0]-100, s[1]/2, 0, 2000)
			if a.Bounds(telatest.ByRole("button", "Play")) != play || a.Bounds(scene(Planet)) != r {
				t.Errorf("%s: the window scrolls", what)
			}
			for _, l := range []telatest.Locator{last, telatest.ByRole("button", "Play"), telatest.ByTestID("resolution"), telatest.ByRole("radio", "Satin")} {
				b := a.Bounds(l)
				if b.X < r.X+r.Width || b.X+b.Width > s[0] || b.Y < 0 || b.Y+b.Height > s[1] {
					t.Errorf("%s: %v is at %v, not on the right of the scene (%v)", what, l, b, r)
				}
			}
			a.Click(last)
			a.Expect(last, telatest.Checked(true))
		}
	}
}

func TestAria(t *testing.T) {
	a := mount(t, still(Planet))
	a.AriaGolden("texture.aria.yml")
	a.ExpectAccessible()
}

// TestIdleBudget: a paused scene costs nothing, though its loop is there.
func TestIdleBudget(t *testing.T) {
	for s := range scenes {
		a := mount(t, Demo{Scene: Scene(s), Paused: true, At: 4 * time.Second})
		a.ExpectText("766×430 at scale 1") // reported after the frame was drawn
		idle.Zero(t, "texture, paused", idle.Watch(a))
		idle.Zero(t, "texture, a still", idle.Watch(mount(t, still(Scene(s)))))
	}
}

var (
	play  = telatest.ByRole("button", "Play")
	pause = telatest.ByRole("button", "Pause")
)

// scene is the texture that shows s.
func scene(s Scene) telatest.Locator { return telatest.ByRole("img", scenes[s].about) }

func choose(a *telatest.App, radio string) { a.Click(telatest.ByRole("radio", radio)) }

func resolve(a *telatest.App, percent int) {
	a.Click(telatest.ByTestID("resolution"))
	a.Click(telatest.ByRole("option", fmt.Sprintf("%d%%", percent)))
}

// TestPlayAndPause: the scene plays, and rests when it is paused.
func TestPlayAndPause(t *testing.T) {
	a := mount(t, Demo{})
	a.ExpectCount(pause, 1)
	a.ExpectText("the CPU, as pixels. Tela draws with the CPU (build with -tags telagpu)")
	first := a.PixelAt(480, 250)
	a.WaitFor("the planet to turn", func() bool { return a.PixelAt(480, 250) != first })

	a.Click(pause)
	a.ExpectText("Paused")
	idle.Zero(t, "texture, paused", idle.Watch(a))
	rest := a.PixelAt(480, 250)

	a.Click(play)
	a.WaitFor("the planet to turn again", func() bool { return a.PixelAt(480, 250) != rest })
}

// TestScenes: another scene is drawn by the same loop, playing and paused;
// paused, one frame of it.
func TestScenes(t *testing.T) {
	a := mount(t, Demo{})
	for _, s := range []Scene{Mountains, Satin, Planet, Satin} {
		before := a.PixelAt(700, 120)
		choose(a, scenes[s].name)
		a.ExpectCount(scene(s), 1)
		a.WaitFor(scenes[s].name, func() bool { return a.PixelAt(700, 120) != before })
	}

	a = mount(t, Demo{Paused: true, At: 4 * time.Second})
	a.ExpectText("766×430 at scale 1")
	tex := scene(Planet)
	for _, s := range []Scene{Mountains, Satin, Planet} {
		oneFrame(t, a, tex, scenes[s].name, func() { choose(a, scenes[s].name) })
		tex = scene(s)
		at, shot := a.Bounds(tex), a.Screenshot()
		u := sceneAt(s, 4*time.Second, 766, 430)
		want := image.NewRGBA(image.Rect(0, 0, 766, 430))
		drawScene(want, &u)
		for _, p := range []image.Point{{383, 215}, {600, 60}, {100, 400}, {700, 300}} {
			if got, want := shot.RGBAAt(int(at.X+0.5)+p.X, int(at.Y+0.5)+p.Y), want.RGBAAt(p.X, p.Y); got != want {
				t.Errorf("%s, %v: %v, want %v", scenes[s].name, p, got, want)
			}
		}
	}
}

// oneFrame checks that what act does to a paused scene draws it once, at rest
// after that, and that Tela holds that frame only.
func oneFrame(t *testing.T, a *telatest.App, tex telatest.Locator, what string, act func()) {
	t.Helper()
	x := a.Texture(tex)
	before := x.Stats().Submitted
	act()
	a.WaitFor(what, func() bool { return x.Stats().Shown > before })
	idle.Zero(t, "texture, paused, "+what, idle.Watch(a))
	if n := x.Stats().Submitted - before; n != 1 {
		t.Errorf("%s: %d frames, want 1", what, n)
	}
	a.ExpectReleases(1)
}

// TestResolution: the frames are of a share of the size they are shown at,
// and the card stays as it is; paused, a change draws one frame.
func TestResolution(t *testing.T) {
	a := mount(t, Demo{Paused: true, At: 4 * time.Second}, telatest.Scale(2))
	a.ExpectText("1532×862 at scale 2")
	card := a.Bounds(scene(Planet))
	for _, c := range []struct {
		percent int
		text    string
	}{
		{25, "383×216 of 1532×862 at scale 2"},
		{50, "766×431 of 1532×862 at scale 2"},
		{200, "3064×1724 of 1532×862 at scale 2"},
		{75, "1149×647 of 1532×862 at scale 2"},
	} {
		oneFrame(t, a, scene(Planet), c.text, func() { resolve(a, c.percent) })
		a.ExpectText(c.text)
		if got := a.Bounds(scene(Planet)); got != card {
			t.Errorf("%d%%: the scene is at %v, was at %v", c.percent, got, card)
		}
	}
	// Combined with a resize, which keeps the share.
	a.Resize(900, 800)
	a.ExpectText("747×420 of 996×560 at scale 2")
	oneFrame(t, a, scene(Planet), "the whole size", func() { resolve(a, 100) })
	a.ExpectText("996×560 at scale 2")

	a.Click(play)
	a.ExpectText("shown")
	resolve(a, 25)
	a.ExpectText("249×140 of 996×560 at scale 2, ")
	resolve(a, 200)
	a.ExpectText("1992×1120 of 996×560 at scale 2, ")
	a.Click(pause)
	a.ExpectText("Paused")
	a.ExpectReleases(1)
}

// TestCardSize: the card's size changes the size the scene is shown at, and
// its frames follow, at the same resolution.
func TestCardSize(t *testing.T) {
	a := mount(t, Demo{Paused: true, At: 4 * time.Second})
	a.ExpectText("766×430 at scale 1")
	oneFrame(t, a, scene(Planet), "full", func() { choose(a, "Full") })
	a.ExpectText("778×438 at scale 1")
	oneFrame(t, a, scene(Planet), "half", func() { resolve(a, 50) })
	a.ExpectText("389×219 of 778×438 at scale 1")
	oneFrame(t, a, scene(Planet), "small", func() { choose(a, "Small") })
	a.ExpectText("223×125 of 446×250 at scale 1")
	oneFrame(t, a, scene(Planet), "medium", func() { choose(a, "Medium") })
	a.ExpectText("383×215 of 766×430 at scale 1")

	a.Click(play)
	a.ExpectText("383×215 of 766×430 at scale 1, ")
	choose(a, "Full")
	a.ExpectText("389×219 of 778×438 at scale 1, ")
	resolve(a, 200)
	a.ExpectText("1556×876 of 778×438 at scale 1, ")
	a.Resize(1400, 600) // short: the scene is as tall as the window
	a.ExpectText("1996×1124 of 998×562 at scale 1, ")
	choose(a, "Small")
	a.ExpectText("892×500 of 446×250 at scale 1, ")
	a.Click(pause)
	a.ExpectText("Paused")
	a.ExpectReleases(1)
}

// TestResize: the paused scene follows the size it is shown at.
func TestResize(t *testing.T) {
	a := mount(t, Demo{Paused: true, At: 4 * time.Second})
	a.ExpectText("766×430 at scale 1")
	a.Resize(800, 700)
	a.ExpectText("398×224 at scale 1")
	idle.Zero(t, "texture, resized", idle.Watch(a))
}

// TestStillFollows: a still is drawn again for another scene and another
// resolution.
func TestStillFollows(t *testing.T) {
	a := mount(t, still(Planet))
	before := a.PixelAt(700, 120)
	choose(a, "Mountains")
	if a.PixelAt(700, 120) == before {
		t.Error("the scene did not change")
	}
	resolve(a, 25)
	a.ExpectText("192×108 of 766×430 at scale 1")
	idle.Zero(t, "texture, a still", idle.Watch(a))
}

// TestModes: a choice of who draws, here while paused, draws the scene once
// more and says what does.
func TestModes(t *testing.T) {
	a := mount(t, Demo{Paused: true, At: 4 * time.Second})
	a.ExpectText("build with -tags telagpu")
	a.Click(telatest.ByRole("radio", "Software"))
	a.ExpectNoText("build with -tags telagpu")
}

// TestUnknownGPU: where the GPU that draws Tela's windows cannot be drawn
// with, the scene is drawn in software and the window says why; so it is
// when Tela's GPU is gone.
func TestUnknownGPU(t *testing.T) {
	fake := telatest.NewFakeTextureDevice(tela.TextureDevice{GPU: true, Name: "Fake GPU", RenderNode: "/dev/dri/renderD999", UUID: [16]byte{1: 99}})
	a := mount(t, Demo{}, telatest.WithTextureDevice(fake))
	a.ExpectText("Tela draws on Fake GPU (/dev/dri/renderD999)")
	a.ExpectText("the CPU, as pixels. Drawn in software: ")
	fake.SetDevice(tela.TextureDevice{})
	a.ExpectText("Tela draws on the CPU")
	a.ExpectText("the CPU, as pixels. Tela draws with the CPU (build with -tags telagpu)")
}

// TestSpeed moves the slider.
func TestSpeed(t *testing.T) {
	a := mount(t, still(Planet))
	a.Focus(telatest.ByRole("slider", "Speed"))
	a.Press("ArrowRight")
	a.ExpectText("1.25×")
}

// TestClock: the scene's clock follows the frames, at its speed, and skips
// what it was not shown.
func TestClock(t *testing.T) {
	c := newControls(time.Second, picture{})
	t0 := time.Now()
	step := func(ms int, want time.Duration, moves bool) {
		t.Helper()
		if got := c.advance(t0.Add(time.Duration(ms) * time.Millisecond)); got != moves {
			t.Errorf("at %d ms: moving %v", ms, got)
		}
		if c.now() != want {
			t.Errorf("at %d ms: the clock is at %v, want %v", ms, c.now(), want)
		}
	}
	step(0, time.Second, true)
	step(20, 1020*time.Millisecond, true)
	c.set(false, 2)
	step(40, 1060*time.Millisecond, true)
	c.set(true, 2)
	step(60, 1060*time.Millisecond, false)
	c.set(false, 1)
	step(1060, 1160*time.Millisecond, true) // a second out of sight
	step(1050, 1160*time.Millisecond, true) // never back
}

// TestFrameSize: a share of the size it is shown at, at least 16 pixels, at
// most what the device takes.
func TestFrameSize(t *testing.T) {
	for _, c := range []struct {
		w, h, percent, limit int
		want                 image.Point
	}{
		{1532, 862, 100, frameLimit, image.Pt(1532, 862)},
		{1532, 862, 50, frameLimit, image.Pt(766, 431)},
		{1532, 862, 200, frameLimit, image.Pt(3064, 1724)},
		{1532, 862, 200, 2048, image.Pt(2048, 1152)},
		{1532, 862, 100, 1024, image.Pt(1024, 576)},
		{80, 45, 25, frameLimit, image.Pt(28, 16)},
		{10, 10, 100, frameLimit, image.Pt(16, 16)},
		{9000, 2, 100, 4096, image.Pt(4096, 16)},
	} {
		w, h := frameSize(tela.TextureSize{Width: c.w, Height: c.h, Scale: 1}, c.percent, c.limit)
		if got := image.Pt(w, h); got != c.want {
			t.Errorf("%d%% of %dx%d, at most %d: %v, want %v", c.percent, c.w, c.h, c.limit, got, c.want)
		}
	}
}

// TestSceneSizes draws the scenes at sizes down to a pixel, and does it again
// with one processor: the same pixels.
func TestSceneSizes(t *testing.T) {
	for s := range scenes {
		for _, size := range []image.Point{{1, 1}, {1, 40}, {300, 2}, {97, 61}} {
			u := sceneAt(Scene(s), time.Minute, size.X, size.Y)
			img := image.NewRGBA(image.Rect(0, 0, size.X, size.Y))
			drawScene(img, &u)
			for i := 3; i < len(img.Pix); i += 4 {
				if img.Pix[i] != 255 {
					t.Fatalf("%s, %v: pixel %d is not opaque", scenes[s].name, size, i/4)
				}
			}
			one := newShader(&u)()
			row := make([]byte, 4*size.X)
			for y := size.Y - 1; y >= 0; y-- { // in any order
				drawRow(one, row, &u, y)
				if string(row) != string(img.Pix[y*img.Stride:][:len(row)]) {
					t.Fatalf("%s, %v: row %d is not the same when it is drawn in another order", scenes[s].name, size, y)
				}
			}
		}
	}
}

func BenchmarkSoftware(b *testing.B) {
	const w, h = 1532, 862
	for s := range scenes {
		b.Run(scenes[s].name, func(b *testing.B) {
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			for i := 0; b.Loop(); i++ {
				u := sceneAt(Scene(s), time.Duration(i)*time.Second/60, w, h)
				drawScene(img, &u)
			}
		})
	}
}
