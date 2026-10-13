// Command text is the phase-2 showcase: mixed-script paragraphs (Latin,
// Arabic, CJK, emoji), a code block, truncation and decorations, laid out on
// a page of cards in a native window. The same page is rendered by
// `tela snapshot ./examples/text` without a window.
//
// The paragraphs are shaped once; resizing the window only breaks their
// lines again. Clicking and dragging selects text (double click: a word,
// triple click: a paragraph; a selection never leaves its paragraph), and
// the wheel scrolls a page taller than the window.
//
// The app draws only when the backend sends a FrameEvent, and asks for one
// only when the selection or the scroll offset changes: no timers, no caret
// blinking, no CPU while idle.
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
	"github.com/kuronosu/tela/internal/text"
)

func init() {
	// The UI thread must be the main OS thread (required by Cocoa later).
	runtime.LockOSThread()
}

// fonts is the collection the scene is shaped with: the embedded fonts only
// (Inter, JetBrains Mono, Noto Sans Arabic, Noto Color Emoji), so snapshots
// look the same on every machine. CJK is not embedded (ADR-031):
//
//   - the window enables the operating system's fonts (see run), which fill
//     in the CJK text;
//   - the tests register a small CJK subset font (main_test.go), and since
//     `tela snapshot` compiles the package's tests too, snapshots get it as
//     well. Without either, the CJK text draws as missing-glyph boxes.
var fonts = text.NewDefaultCollection()

// Scene returns the showcase for a window of w x h logical pixels. It is
// the phase-1 contract of `tela snapshot`, which the CLI still accepts: a
// package with a Scene(w, h float32) *gfx.DisplayList can be snapshotted
// without a tela.Node tree. This example keeps it on purpose, since it
// exercises the internal text layer (internal/text) directly instead of
// going through the elements of package h; the examples built on the public
// API define Preview() or Previews.
//
// It shapes every paragraph and places the cards for width w. The window
// does not call it: it keeps one page and only re-breaks it (see app).
func Scene(w, h float32) *gfx.DisplayList {
	return sceneWith(fonts, w, h, selection{})
}

// sceneWith builds the display list of a new page shaped with c, with the
// given selection drawn (the zero selection draws none).
func sceneWith(c *text.Collection, w, h float32, sel selection) *gfx.DisplayList {
	pg := newPage(c)
	pg.arrange(w)
	b := gfx.NewBuilder()
	pg.paint(b, w, h, 0, sel)
	return b.Finish()
}

func main() {
	if err := run(os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "text:", err)
		os.Exit(1)
	}
}

// run opens the window and runs the event loop on the calling goroutine.
func run(log io.Writer) error {
	// Real windows get the machine's fonts for what the embedded ones
	// cannot draw (CJK, Hebrew, Thai...). A failure only costs those scripts.
	if err := fonts.EnableSystemFonts(text.SystemFontOptions{}); err != nil {
		fmt.Fprintln(log, "text: system fonts disabled:", err)
	}
	backend, err := auto.New()
	if err != nil {
		return err
	}
	win, err := backend.NewWindow(platform.WindowOptions{
		Title: "Tela — text",
		AppID: "dev.tela.text",
		Size:  platform.Size{W: 960, H: 720},
	})
	if err != nil {
		return err
	}
	return backend.Run(newApp(backend, win, fonts, log))
}

// app is the event handler of the example. It owns the framebuffer, the page
// (shaped once) and the interaction state.
type app struct {
	backend platform.Backend
	win     platform.Window
	log     io.Writer

	fb       *image.RGBA // reused between frames; reallocated when the size changes
	renderer raster.Renderer

	pg     *page
	it     *interaction
	cursor platform.Cursor // the shape last set on the window

	frameCount int
}

func newApp(backend platform.Backend, win platform.Window, c *text.Collection, log io.Writer) *app {
	pg := newPage(c)
	return &app{backend: backend, win: win, log: log, pg: pg, it: newInteraction(pg)}
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
		fmt.Fprintf(a.log, "text: %s configure %v @%g → %dx%d px\n",
			a.backend.Name(), e.Metrics.Size, e.Metrics.Scale, p.X, p.Y)
	case platform.FrameEvent:
		a.draw()
	case platform.PointerEvent:
		redraw := a.it.pointer(e)
		if c := a.it.cursor(); c != a.cursor {
			a.cursor = c
			a.win.SetCursor(c)
		}
		if redraw {
			a.win.RequestFrame()
		}
	case platform.ScrollEvent:
		if a.it.wheel(e) {
			a.win.RequestFrame()
		}
	case platform.CloseRequestedEvent:
		a.win.Close()
		a.backend.Quit()
	}
}

// draw lays the page out for the window's current size (a no-op unless the
// width changed), renders it and presents it.
func (a *app) draw() {
	m := a.win.Metrics()
	phys := m.Physical()
	if phys.X <= 0 || phys.Y <= 0 {
		return
	}
	if want := image.Rect(0, 0, phys.X, phys.Y); a.fb == nil || a.fb.Rect != want {
		a.fb = image.NewRGBA(want)
	}
	a.pg.arrange(m.Size.W)
	a.it.setView(m.Size.H)

	b := gfx.NewBuilder()
	a.pg.paint(b, m.Size.W, m.Size.H, a.it.scroll, a.it.sel)
	raster.Clear(a.fb, image.Rectangle{}, gfx.White)
	a.renderer.Render(a.fb, b.Finish(), raster.Options{Scale: float32(m.Scale)})
	// Present on every FrameEvent: a compositor that sent a configure
	// (tiled, maximized) expects a buffer of the new size in answer.
	if err := a.win.Present(a.fb, nil); err != nil {
		fmt.Fprintln(a.log, "text: present:", err)
	}
	a.frameCount++
}
