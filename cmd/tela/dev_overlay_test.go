package main

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/remote"
	"github.com/kuronosu/tela/internal/testutil/golden"
	"github.com/kuronosu/tela/telatest"
)

// The picture of the error overlay over a fixed fake frame (an app's
// screen), at scale 1 and 2.

const compilerOutput = `# example.com/dashboard
./main.go:41:17: undefined: totalRevenue
./main.go:58:3: declared and not used: label
./panel.go:12:24: cannot use "12" (untyped string constant) as int value in argument to h.Gap
./panel.go:30:2: too many errors
`

const panicText = `panic: runtime error: index out of range [3] with length 3

goroutine 1 [running]:
main.(*Table).row(...)
	/home/dev/dashboard/table.go:87
main.Table.func1({0x6a1f00, 0xc0000a2000})
	/home/dev/dashboard/table.go:64 +0x1d4
github.com/kuronosu/tela/internal/core.(*Tree).buildComponent(0xc0001a4000, 0xc0002b6000)
	/go/pkg/mod/github.com/kuronosu/tela@v0.9.0/internal/core/build.go:312 +0x8a
github.com/kuronosu/tela/internal/core.(*Tree).Frame(0xc0001a4000)
	/go/pkg/mod/github.com/kuronosu/tela@v0.9.0/internal/core/tree.go:472 +0x11c
main.main()
	/home/dev/dashboard/main.go:23 +0x5d
runtime.main()
	/usr/local/go/src/runtime/proc.go:283 +0x28b
`

// fakeFrame is a screen of an app, in physical pixels.
func fakeFrame(t testing.TB, w, hh float32, scale float64) *image.RGBA {
	t.Helper()
	card := func(title, value string) tela.Node {
		return h.Div(h.Class("flex-1 rounded-xl bg-white p-4 shadow-sm border border-slate-200"),
			h.Div(h.Class("text-sm text-slate-500"), h.Text(title)),
			h.Div(h.Class("text-3xl font-semibold text-slate-900"), h.Text(value)))
	}
	root := h.Div(h.Class("flex flex-col gap-4 p-6 bg-slate-100 w-full h-full"),
		h.Div(h.Class("text-2xl font-bold text-slate-800"), h.Text("Dashboard")),
		h.Div(h.Class("flex gap-4"),
			card("Revenue", "$48,210"), card("Orders", "1,284"), card("Visitors", "9,402")),
		h.Div(h.Class("flex-1 rounded-xl bg-linear-to-br from-indigo-500 to-fuchsia-600")),
		h.Div(h.Class("flex gap-3"),
			h.Button(h.Class("px-4 py-2 rounded-lg bg-indigo-600 text-white"), h.Text("Refresh")),
			h.Button(h.Class("px-4 py-2 rounded-lg bg-white text-slate-700 border border-slate-200"), h.Text("Export"))),
	)
	img, err := telatest.Render(root, telatest.Size(w, hh), telatest.Scale(scale))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// drawOverlay renders the model and composites it over frame, the way the
// host does.
func drawOverlay(t testing.TB, p problem, frame *image.RGBA, size platform.Size, scale float64) (*image.RGBA, ovGeom) {
	t.Helper()
	m := newOverlayModel(p)
	pic, g, err := m.render(size, scale)
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewRGBA(frame.Rect)
	copy(out.Pix, frame.Pix)
	compositeOver(out, pic, image.Rect(
		int(g.paint.X0*float32(scale)), int(g.paint.Y0*float32(scale)),
		int(g.paint.X1*float32(scale)+0.999), int(g.paint.Y1*float32(scale)+0.999)))
	return out, g
}

func TestOverlayGoldens(t *testing.T) {
	size := platform.Size{W: 800, H: 520}
	cases := []struct {
		name string
		p    problem
	}{
		{"build_docked", problem{kind: problemBuild, summary: "build failed", text: compilerOutput, live: true}},
		{"build_first", problem{kind: problemBuild, summary: "build failed", text: compilerOutput}},
		{"panic", problem{kind: problemPanic, summary: "the app panicked: runtime error: index out of range [3] with length 3; save a change or press r to start it again", text: panicText}},
		{"exit", problem{kind: problemExit, summary: "the app exited with status 3; save a change or press r to start it again"}},
		{"start", problem{kind: problemStart, summary: "the new build showed no frame in 15s; the old build keeps running", live: true}},
	}
	for _, scale := range []float64{1, 2} {
		frame := fakeFrame(t, size.W, size.H, scale)
		for _, c := range cases {
			t.Run(fmt.Sprintf("%s_x%g", c.name, scale), func(t *testing.T) {
				img, _ := drawOverlay(t, c.p, frame, size, scale)
				golden.Image(t, fmt.Sprintf("overlay/%s_x%g", c.name, scale), img)
			})
		}
	}
}

func TestOverlayGeometry(t *testing.T) {
	size := platform.Size{W: 800, H: 520}
	long := problem{kind: problemBuild, text: strings.Repeat("./main.go:1:1: x\n", 100), live: true}
	g := newOverlayModel(long).geometry(size)
	if !g.docked {
		t.Fatal("a build error with a live app is docked")
	}
	if want := float32(208); g.panel.Height() != 220 || g.panel.Y1 != 520 || g.panel.Y0 != 520-220 {
		t.Errorf("docked panel %+v, want the bottom 220 (40%% of 520 is %v, below the minimum)", g.panel, want)
	}
	if g.hidden == 0 || g.visible+g.hidden != 100 {
		t.Errorf("visible %d hidden %d, want them to add up to 100 lines", g.visible, g.hidden)
	}
	if !(g.close.X0 > g.panel.X1-60 && g.close.Y0 >= g.panel.Y0 && g.close.Y1 <= g.panel.Y0+60) {
		t.Errorf("close button %+v is not at the top right of %+v", g.close, g.panel)
	}
	tall := platform.Size{W: 1000, H: 1000}
	if g := newOverlayModel(long).geometry(tall); g.panel.Height() != 400 {
		t.Errorf("docked height %v in a window 1000 tall, want 400", g.panel.Height())
	}

	few := problem{kind: problemExit, summary: "the app exited"}
	g = newOverlayModel(few).geometry(size)
	if g.docked || g.paint.Width() != size.W || g.paint.Height() != size.H {
		t.Errorf("an exit covers the whole window: %+v", g)
	}
	if g.panel.Height() >= size.H-2*ovMargin {
		t.Errorf("a panel with no text should not fill the window: %+v", g.panel)
	}
	if g.panel.Width() > ovMaxWidth {
		t.Errorf("panel %v wide, want at most %d", g.panel.Width(), ovMaxWidth)
	}
}

func TestOverlayText(t *testing.T) {
	lines := compilerLines(compilerOutput)
	if len(lines) != 5 {
		t.Fatalf("%d lines, want 5", len(lines))
	}
	if lines[0].segs[0].class != clsDim {
		t.Errorf("the package header is %q, want dim", lines[0].segs[0].class)
	}
	if s := lines[1].segs; len(s) != 2 || s[0].text != "./main.go:41:17" || s[0].class != clsLoc || s[1].text != ": undefined: totalRevenue" {
		t.Errorf("a compiler line: %+v", s)
	}

	pl := panicLines(panicText)
	class := func(sub string) string {
		for _, l := range pl {
			for _, s := range l.segs {
				if strings.Contains(s.text, sub) {
					return s.class
				}
			}
		}
		return "missing"
	}
	for sub, want := range map[string]string{
		"panic: runtime error":            clsStrong,
		"goroutine 1":                     clsDim,
		"main.Table.func1":                clsPlain,
		"/home/dev/dashboard/table.go:64": clsLoc,
		"internal/core.(*Tree).Frame":     clsFaint,
		"tree.go:472":                     clsFaint,
		"runtime.main":                    clsFaint,
	} {
		if got := class(sub); got != want {
			t.Errorf("%q has class %q, want %q", sub, got, want)
		}
	}
	for _, l := range splitLines("a\n\tb\r\n") {
		if strings.ContainsAny(l, "\t\r") {
			t.Errorf("line %q keeps a tab or a carriage return", l)
		}
	}
}

func TestCompositeOver(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(dst.Pix, []uint8{200, 100, 0, 255, 200, 100, 0, 255})
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(src.Pix, []uint8{0, 0, 0, 0, 0, 0, 0, 128}) // transparent, half black
	compositeOver(dst, src, dst.Rect)
	if got := dst.Pix[:4]; got[0] != 200 || got[1] != 100 || got[3] != 255 {
		t.Errorf("a transparent pixel changed the frame: %v", got)
	}
	if got := dst.Pix[4:]; got[0] < 98 || got[0] > 102 || got[1] < 48 || got[1] > 52 || got[3] != 255 {
		t.Errorf("half black over (200,100,0): %v", got)
	}
}

// overlayWindow is a platform.Window that only knows its metrics; the
// embedded nil interface panics on anything else, which keeps the routing
// honest about what it touches.
type overlayWindow struct {
	platform.Window
	m platform.Metrics
}

func (w overlayWindow) Metrics() platform.Metrics { return w.m }

// TestOverlayRouting drives overlayView.handleInput on a docked panel: which
// events the overlay takes, which go on to the app, and the state of the
// pointer sequence it tracks (ADR-117).
func TestOverlayRouting(t *testing.T) {
	size := platform.Size{W: 800, H: 520}
	win := overlayWindow{m: platform.Metrics{Size: size, Scale: 1}}
	p := problem{kind: problemBuild, summary: "build failed", text: compilerOutput, live: true}
	g := newOverlayModel(p).geometry(size)
	if !g.docked {
		t.Fatalf("geometry %+v: want a docked panel", g)
	}
	mid := func(r gfx.Rect) platform.Point { return platform.Point{X: (r.X0 + r.X1) / 2, Y: (r.Y0 + r.Y1) / 2} }
	closeBtn := mid(g.close)
	body := platform.Point{X: g.panel.X0 + 12, Y: g.panel.Y1 - 6} // in the panel, off the button
	if g.close.Contains(gfx.Point(body)) {
		t.Fatalf("body point %v is on the close button %+v", body, g.close)
	}
	above := platform.Point{X: size.W / 2, Y: g.panel.Y0 / 2} // the app's part of the window
	held := platform.ButtonPrimary.Mask()
	other := platform.ButtonSecondary.Mask()
	down := func(pos platform.Point) platform.Event {
		return platform.PointerEvent{Action: platform.PointerDown, Button: platform.ButtonPrimary, Buttons: held, Pos: pos}
	}
	up := func(pos platform.Point, buttons platform.Buttons) platform.Event {
		return platform.PointerEvent{Action: platform.PointerUp, Button: platform.ButtonPrimary, Buttons: buttons, Pos: pos}
	}
	move := func(pos platform.Point, buttons platform.Buttons) platform.Event {
		return platform.PointerEvent{Action: platform.PointerMove, Buttons: buttons, Pos: pos}
	}
	cancel := platform.PointerEvent{Action: platform.PointerCancel, Pos: body}
	key := func(k platform.Key, a platform.KeyAction) platform.Event {
		return platform.KeyEvent{Key: k, Action: a}
	}
	scroll := func(pos platform.Point) platform.Event { return platform.ScrollEvent{Pos: pos} }

	// step is one event and what the overlay does with it: whether it took
	// the event, whether a press is captured afterwards and whether the
	// panel is gone afterwards.
	type step struct {
		name      string
		ev        platform.Event
		handled   bool
		captured  bool
		dismissed bool
	}
	cases := []struct {
		name  string
		steps []step
	}{
		{"app press and drag", []step{
			{"press above", down(above), false, false, false},
			{"drag into the panel", move(body, held), false, false, false},
			{"release inside the panel", up(body, 0), false, false, false},
		}},
		{"hover", []step{
			{"move over the panel", move(body, 0), true, false, false},
			{"move over the app", move(above, 0), false, false, false},
		}},
		{"panel press and drag out", []step{
			{"press on the panel", down(body), true, true, false},
			{"drag out to the app", move(above, held), true, true, false},
			{"release over the app", up(above, 0), true, false, false},
			{"the next move is the app's", move(above, 0), false, false, false},
		}},
		{"close button", []step{
			{"press on the button", down(closeBtn), true, true, false},
			{"release on the button", up(closeBtn, 0), true, false, true},
			{"later events go to the app", down(body), false, false, true},
		}},
		{"press on x, release off x", []step{
			{"press on the button", down(closeBtn), true, true, false},
			{"release on the panel body", up(body, 0), true, false, false},
		}},
		{"press on x, release outside the panel", []step{
			{"press on the button", down(closeBtn), true, true, false},
			{"release over the app", up(above, 0), true, false, false},
		}},
		{"press off x, release on x", []step{
			{"press on the body", down(body), true, true, false},
			{"release on the button", up(closeBtn, 0), true, false, false},
		}},
		{"release with a button still held", []step{
			{"press on the panel", down(body), true, true, false},
			{"release of one of two buttons", up(body, other), true, true, false},
			{"drag stays on the panel", move(above, other), true, true, false},
			{"release of the last button", up(above, 0), true, false, false},
		}},
		{"release on x with another button held", []step{
			{"press on the button", down(closeBtn), true, true, false},
			{"release of one of two buttons", up(closeBtn, other), true, false, true},
		}},
		{"release without a press", []step{
			{"release over the panel", up(body, 0), false, false, false},
		}},
		{"cancel of a panel press", []step{
			{"press on the panel", down(body), true, true, false},
			{"cancel", cancel, true, false, false},
			{"the next move is the app's", move(above, 0), false, false, false},
		}},
		{"cancel of an app press", []step{
			{"press above", down(above), false, false, false},
			{"cancel", cancel, false, false, false},
		}},
		{"escape", []step{
			{"another key", key(platform.KeyEnter, platform.KeyPress), false, false, false},
			{"escape release", key(platform.KeyEscape, platform.KeyRelease), false, false, false},
			{"escape press", key(platform.KeyEscape, platform.KeyPress), true, false, true},
			{"escape again", key(platform.KeyEscape, platform.KeyPress), false, false, true},
		}},
		{"escape during a press", []step{
			{"press on the panel", down(body), true, true, false},
			{"escape press", key(platform.KeyEscape, platform.KeyPress), true, false, true},
		}},
		{"scroll", []step{
			{"over the panel", scroll(body), true, false, false},
			{"over the app", scroll(above), false, false, false},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := newOverlayView(nil)
			v.attach(&remote.Host{})
			v.model = newOverlayModel(p)
			for _, s := range c.steps {
				if got := v.handleInput(win, s.ev); got != s.handled {
					t.Fatalf("%s: handled = %v, want %v", s.name, got, s.handled)
				}
				if got := v.press.captured; got != s.captured {
					t.Fatalf("%s: captured = %v, want %v", s.name, got, s.captured)
				}
				if got := !v.active(); got != s.dismissed {
					t.Fatalf("%s: dismissed = %v, want %v", s.name, got, s.dismissed)
				}
			}
		})
	}

	t.Run("no problem", func(t *testing.T) {
		v := newOverlayView(nil)
		v.attach(&remote.Host{})
		for _, ev := range []platform.Event{down(body), scroll(body), key(platform.KeyEscape, platform.KeyPress)} {
			if v.handleInput(win, ev) {
				t.Errorf("%T taken with no problem on view", ev)
			}
		}
	})
}
