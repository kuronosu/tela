package h_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// settle delivers what is due and a few refreshes, so frames requested
// meanwhile run.
func settle(b *headless.Backend, h *core.Host) {
	b.Drain(h)
	for range 4 {
		b.Advance(b.FrameInterval())
		b.Drain(h)
	}
}

// A tela.Window opens a frameless modal child window with a custom title
// bar; UseWindow in its content is that window's handle, and its close
// control runs OnClose, which unmounts it (ADR-180).
func TestWindowComponent(t *testing.T) {
	var child tela.WindowHandle
	settings := func() tela.Node {
		child = tela.UseWindow()
		return Div(Class("flex h-10 items-center"), WindowDrag(),
			Span(Class("grow"), Text("Settings")),
			Button(Class("size-10"), WindowControl(tela.WindowClose), AriaLabel("Close"), Text("x")),
		)
	}
	root := func() tela.Node {
		open, setOpen := tela.UseState(true)
		return Div(Text("main"), If(open, tela.Component(tela.Window{
			Title: "Settings", Width: 300, Height: 200,
			Frameless: true, Modal: true, RootClass: "bg-white",
			OnClose: func() { setOpen.Set(false) },
			Root:    tela.C(settings),
		})))
	}
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}})
	h, err := core.NewHost(b, core.Options{Root: tela.C(root)})
	if err != nil {
		t.Fatal(err)
	}
	settle(b, h)
	ws := b.Windows()
	if len(ws) != 2 {
		t.Fatalf("%d windows, want the main window and Settings", len(ws))
	}
	w := ws[1]
	o := w.Options()
	if w.Title() != "Settings" || !o.Frameless || !o.Modal || o.Parent != ws[0] || o.Size != (platform.Size{W: 300, H: 200}) {
		t.Errorf("Settings window: title %q, options %+v", w.Title(), o)
	}
	if child != h.Handle(w) || !child.State().ClientSideDecorations {
		t.Errorf("UseWindow in the window's content: %v, state %+v", child == h.Handle(w), child.State())
	}
	var close platform.HitRegion
	kinds := ""
	for _, r := range w.HitRegions() {
		kinds += r.Kind.String() + " "
		if r.Kind == platform.HitClose {
			close = r
		}
	}
	if kinds != "caption close " {
		t.Fatalf("hit regions %v, want a caption and a close button", w.HitRegions())
	}
	cx, cy := close.Rect.X+close.Rect.W/2, close.Rect.Y+close.Rect.H/2
	w.Inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: cx, Y: cy}})
	w.Inject(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: cx, Y: cy}, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()})
	w.Inject(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: cx, Y: cy}, Button: platform.ButtonPrimary})
	settle(b, h)
	if !w.Closed() || len(b.Windows()) != 1 {
		t.Errorf("after the close control: closed %v, %d windows", w.Closed(), len(b.Windows()))
	}
}

// h.WindowControl rejects values that are not a control.
func TestWindowControlChecks(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("WindowControl(0) did not panic")
		}
	}()
	WindowControl(0)
}
