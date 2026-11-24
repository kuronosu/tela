package main

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/testutil/idle"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 960, 760

// mount opens the screen in the window of main. Frames are drawn on a
// goroutine, so the waits are generous for slow machines.
func mount(t *testing.T, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = root
	opts = append([]telatest.Option{telatest.Size(windowW, windowH), telatest.Timeout(30 * time.Second)}, opts...)
	return telatest.MountApp(t, app, opts...)
}

// TestGolden is Preview, at scale 1 and 2: the scene four seconds in, drawn
// in software at the size it is shown at.
func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, Preview(), telatest.Scale(scale)).Golden(fmt.Sprintf("texture_x%g.png", scale))
		})
	}
}

// TestSizes mounts the window at other sizes, down to a narrow one: dev mode
// is on, so any dev message fails the test.
func TestSizes(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {420, 700}, {1600, 1000}} {
		mount(t, Preview(), telatest.Size(s[0], s[1]))
	}
}

func TestAria(t *testing.T) {
	a := mount(t, Preview())
	a.AriaGolden("texture.aria.yml")
	a.ExpectAccessible()
}

// TestIdleBudget: a paused scene costs nothing, though its loop is there.
func TestIdleBudget(t *testing.T) {
	a := mount(t, Demo{Paused: true, At: 4 * time.Second})
	a.ExpectText("766×430 at scale 1") // reported after the frame was drawn
	idle.Zero(t, "texture, paused", idle.Watch(a))
	idle.Zero(t, "texture, a still", idle.Watch(mount(t, Preview())))
}

var (
	play  = telatest.ByRole("button", "Play")
	pause = telatest.ByRole("button", "Pause")
)

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
	still := a.PixelAt(480, 250)

	a.Click(play)
	a.WaitFor("the planet to turn again", func() bool { return a.PixelAt(480, 250) != still })
}

// TestResize: the paused scene follows the size it is shown at.
func TestResize(t *testing.T) {
	a := mount(t, Demo{Paused: true, At: 4 * time.Second})
	a.ExpectText("766×430 at scale 1")
	a.Resize(640, 700)
	a.ExpectText("590×332 at scale 1")
	idle.Zero(t, "texture, resized", idle.Watch(a))
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
	a := mount(t, Preview())
	a.Focus(telatest.ByRole("slider", "Speed"))
	a.Press("ArrowRight")
	a.ExpectText("1.25×")
}

// TestClock: the scene's clock follows the frames, at its speed, and skips
// what it was not shown.
func TestClock(t *testing.T) {
	c := newControls(time.Second)
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

// TestSceneSizes draws the scene at sizes down to a pixel.
func TestSceneSizes(t *testing.T) {
	for _, s := range []image.Point{{1, 1}, {1, 40}, {300, 2}, {97, 61}} {
		u := sceneAt(time.Minute, s.X, s.Y)
		img := image.NewRGBA(image.Rect(0, 0, s.X, s.Y))
		drawScene(img, &u)
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 255 {
				t.Fatalf("%v: pixel %d is not opaque", s, i/4)
			}
		}
	}
}

func BenchmarkSoftware(b *testing.B) {
	const w, h = 1536, 864
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; b.Loop(); i++ {
		u := sceneAt(time.Duration(i)*time.Second/60, w, h)
		drawScene(img, &u)
	}
}
