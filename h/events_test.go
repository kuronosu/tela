package h_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// app is a headless window driven by injected pointer events.
type app struct {
	t *testing.T
	b *headless.Backend
	h *core.Host
	w *headless.Window
}

func open(t *testing.T, root func() tela.Node) *app {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}})
	h, err := core.NewHost(b, core.Options{Root: tela.C(root)})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(h)
	return &app{t: t, b: b, h: h, w: h.Window().(*headless.Window)}
}

func (a *app) send(es ...platform.PointerEvent) {
	for _, e := range es {
		a.w.Inject(e)
	}
	a.b.Drain(a.h)
}

func (a *app) frame() {
	a.b.Advance(a.b.FrameInterval())
	a.b.Drain(a.h)
}

func pe(act platform.PointerAction, x, y float32, b platform.PointerButton, held platform.Buttons) platform.PointerEvent {
	return platform.PointerEvent{Action: act, Pos: platform.Point{X: x, Y: y}, Button: b, Buttons: held}
}

var primary = platform.ButtonPrimary.Mask()

func (a *app) heading() string {
	in := a.h.Tree().Inspect()
	var find func(n *core.InspectNode) string
	find = func(n *core.InspectNode) string {
		if n.Kind == "h1" {
			return n.Text
		}
		for i := range n.Children {
			if s := find(&n.Children[i]); s != "" {
				return s
			}
		}
		return ""
	}
	return find(&in)
}

func (a *app) buttonCenter() (float32, float32) {
	in := a.h.Tree().Inspect()
	var find func(n *core.InspectNode) *core.InspectNode
	find = func(n *core.InspectNode) *core.InspectNode {
		if n.Kind == "button" {
			return n
		}
		for i := range n.Children {
			if f := find(&n.Children[i]); f != nil {
				return f
			}
		}
		return nil
	}
	b := find(&in)
	if b == nil {
		a.t.Fatal("no button")
	}
	c := b.Rect.Center()
	return c.X, c.Y
}

// TestCounterInteractive drives the Counter of ARCHITECTURE section 1:
// hovering and pressing restyle the button, a click increments.
func TestCounterInteractive(t *testing.T) {
	a := open(t, Counter)
	x, y := a.buttonCenter()
	before := a.w.Frame()
	a.send(pe(platform.PointerMove, x, y, 0, 0))
	a.frame()
	hovered := a.w.Frame()
	if px := hovered.RGBAAt(int(x)-40, int(y)); px == before.RGBAAt(int(x)-40, int(y)) {
		t.Errorf("hover:bg-white/25 did not change the button's pixels (%v)", px)
	}
	if a.w.Cursor() != platform.CursorPointer {
		t.Errorf("cursor %v over the button", a.w.Cursor())
	}
	a.send(pe(platform.PointerDown, x, y, platform.ButtonPrimary, primary))
	a.frame()
	if pressed := a.w.Frame(); bytes.Equal(pressed.Pix, hovered.Pix) {
		t.Error("active:scale-95 did not change the picture")
	}
	a.send(pe(platform.PointerUp, x, y, platform.ButtonPrimary, 0))
	a.frame()
	if got := a.heading(); got != "Count: 1" {
		t.Errorf("after a click: %q", got)
	}
}

func TestEveryHandlerProp(t *testing.T) {
	var log []string
	add := func(s string) { log = append(log, s) }
	mouse := func(name string) func(tela.MouseEvent) { return func(tela.MouseEvent) { add(name) } }
	pointer := func(name string) func(tela.PointerEvent) { return func(tela.PointerEvent) { add(name) } }
	drag := func(name string) func(tela.DragEvent) { return func(tela.DragEvent) { add(name) } }
	disabled := false
	root := func() tela.Node {
		return Div(Class("flex gap-4 p-4"),
			Div(Class("size-20"),
				OnClick(mouse("Click")), OnDoubleClick(mouse("DoubleClick")), OnContextMenu(mouse("ContextMenu")),
				OnMouseDown(mouse("MouseDown")), OnMouseUp(mouse("MouseUp")), OnMouseMove(mouse("MouseMove")),
				OnMouseEnter(mouse("MouseEnter")), OnMouseLeave(mouse("MouseLeave")),
				OnPointerDown(pointer("PointerDown")), OnPointerMove(pointer("PointerMove")),
				OnPointerUp(pointer("PointerUp")), OnPointerCancel(pointer("PointerCancel")),
				OnPointerEnter(pointer("PointerEnter")), OnPointerLeave(pointer("PointerLeave")),
				Disabled(disabled),
			),
			Div(Class("size-20"),
				OnDragStart(drag("DragStart")), OnDrag(drag("Drag")), OnDragEnd(drag("DragEnd")),
				OnLongPress(pointer("LongPress")),
			),
		)
	}
	a := open(t, root)
	step := func(want string, es ...platform.PointerEvent) {
		t.Helper()
		log = nil
		a.send(es...)
		if got := strings.Join(log, " "); got != want {
			t.Errorf("got  %q\nwant %q", got, want)
		}
	}
	step("PointerEnter MouseEnter PointerMove MouseMove", pe(platform.PointerMove, 40, 40, 0, 0))
	step("PointerDown MouseDown PointerUp MouseUp Click",
		pe(platform.PointerDown, 40, 40, platform.ButtonPrimary, primary), pe(platform.PointerUp, 40, 40, platform.ButtonPrimary, 0))
	step("PointerDown MouseDown PointerUp MouseUp Click DoubleClick",
		pe(platform.PointerDown, 40, 40, platform.ButtonPrimary, primary), pe(platform.PointerUp, 40, 40, platform.ButtonPrimary, 0))
	a.b.Advance(time.Second)
	step("PointerDown MouseDown PointerUp MouseUp ContextMenu",
		pe(platform.PointerDown, 40, 40, platform.ButtonSecondary, platform.ButtonSecondary.Mask()),
		pe(platform.PointerUp, 40, 40, platform.ButtonSecondary, 0))
	step("PointerDown MouseDown PointerCancel",
		pe(platform.PointerDown, 40, 40, platform.ButtonPrimary, primary), pe(platform.PointerCancel, 40, 40, 0, 0))
	step("PointerLeave MouseLeave", pe(platform.PointerMove, 140, 40, 0, 0))
	step("DragStart Drag DragEnd",
		pe(platform.PointerDown, 140, 40, platform.ButtonPrimary, primary),
		pe(platform.PointerMove, 150, 40, 0, primary), pe(platform.PointerMove, 160, 40, 0, primary),
		pe(platform.PointerUp, 160, 40, platform.ButtonPrimary, 0))
	step("", pe(platform.PointerDown, 140, 40, platform.ButtonPrimary, primary))
	a.b.Advance(500 * time.Millisecond)
	a.b.Wake()
	step("LongPress")
	step("", pe(platform.PointerUp, 140, 40, platform.ButtonPrimary, 0))

	// Disabled: the element's handlers do not run.
	disabled = true
	a.h.Tree().Mount(tela.C(root))
	a.frame()
	a.b.Advance(time.Second)
	step("", pe(platform.PointerMove, 40, 40, 0, 0),
		pe(platform.PointerDown, 40, 40, platform.ButtonPrimary, primary), pe(platform.PointerUp, 40, 40, platform.ButtonPrimary, 0))
}

func TestHandlerPropsPanicOnNil(t *testing.T) {
	for name, f := range map[string]func(){
		"OnDoubleClick":   func() { OnDoubleClick(nil) },
		"OnContextMenu":   func() { OnContextMenu(nil) },
		"OnMouseMove":     func() { OnMouseMove(nil) },
		"OnPointerDown":   func() { OnPointerDown(nil) },
		"OnPointerMove":   func() { OnPointerMove(nil) },
		"OnPointerUp":     func() { OnPointerUp(nil) },
		"OnPointerCancel": func() { OnPointerCancel(nil) },
		"OnPointerEnter":  func() { OnPointerEnter(nil) },
		"OnPointerLeave":  func() { OnPointerLeave(nil) },
		"OnLongPress":     func() { OnLongPress(nil) },
		"OnDragStart":     func() { OnDragStart(nil) },
		"OnDrag":          func() { OnDrag(nil) },
		"OnDragEnd":       func() { OnDragEnd(nil) },
		"OnWheel":         func() { OnWheel(nil) },
		"OnScroll":        func() { OnScroll(nil) },
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(r.(string), name+"(nil)") {
					t.Errorf("%s(nil): panic %v", name, r)
				}
			}()
			f()
		}()
	}
}
