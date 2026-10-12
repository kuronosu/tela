// Command shapes shows a fixed scene of rounded cards, gradients, shadows,
// blur and strokes in a native window. It is the phase-1 showcase: the same
// scene is rendered by `tela snapshot ./examples/shapes` without a window.
//
// The app draws only when the backend sends a FrameEvent, which happens after
// every ConfigureEvent (first show, resize, scale change). It never requests
// frames, so it uses no CPU while idle.
package main

import (
	"fmt"
	"image"
	"io"
	"os"
	"runtime"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/auto"
)

func init() {
	// The UI thread must be the main OS thread (required by Cocoa later).
	runtime.LockOSThread()
}

func main() {
	if err := run(os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "shapes:", err)
		os.Exit(1)
	}
}

// run opens the window and runs the event loop on the calling goroutine.
func run(log io.Writer) error {
	backend, err := auto.New()
	if err != nil {
		return err
	}
	win, err := backend.NewWindow(platform.WindowOptions{
		Title: "Tela — shapes",
		AppID: "dev.tela.shapes",
		Size:  platform.Size{W: 960, H: 640},
	})
	if err != nil {
		return err
	}
	return backend.Run(newApp(backend, win, log))
}

// app is the event handler of the example. It owns the framebuffer and
// caches the display list of the last size.
type app struct {
	backend platform.Backend
	win     platform.Window
	log     io.Writer

	fb *image.RGBA // reused between frames; reallocated when the size changes

	dl         *gfx.DisplayList // Scene(dlW, dlH)
	dlW, dlH   float32
	renderer   raster.Renderer
	frameCount int
}

func newApp(backend platform.Backend, win platform.Window, log io.Writer) *app {
	return &app{backend: backend, win: win, log: log}
}

// HandleEvent implements platform.Handler.
func (a *app) HandleEvent(w platform.Window, e platform.Event) {
	if w != a.win {
		return
	}
	switch e := e.(type) {
	case platform.ConfigureEvent:
		// A FrameEvent follows every ConfigureEvent, so there is nothing to
		// request here.
		p := e.Metrics.Physical()
		fmt.Fprintf(a.log, "shapes: %s configure %v @%g → %dx%d px\n",
			a.backend.Name(), e.Metrics.Size, e.Metrics.Scale, p.X, p.Y)
	case platform.FrameEvent:
		a.draw()
	case platform.CloseRequestedEvent:
		a.win.Close()
		a.backend.Quit()
	}
}

// draw renders the scene at the window's current metrics and presents it.
func (a *app) draw() {
	m := a.win.Metrics()
	phys := m.Physical()
	if phys.X <= 0 || phys.Y <= 0 {
		return
	}
	if want := image.Rect(0, 0, phys.X, phys.Y); a.fb == nil || a.fb.Rect != want {
		a.fb = image.NewRGBA(want)
	}
	if a.dl == nil || a.dlW != m.Size.W || a.dlH != m.Size.H {
		a.dl, a.dlW, a.dlH = Scene(m.Size.W, m.Size.H), m.Size.W, m.Size.H
	}
	raster.Clear(a.fb, image.Rectangle{}, gfx.White)
	a.renderer.Render(a.fb, a.dl, raster.Options{Scale: float32(m.Scale)})
	// Present on every FrameEvent: a compositor that sent a configure
	// (tiled, maximized) expects a buffer of the new size in answer.
	if err := a.win.Present(a.fb, nil); err != nil {
		fmt.Fprintln(a.log, "shapes: present:", err)
	}
	a.frameCount++
}
