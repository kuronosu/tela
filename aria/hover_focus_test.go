package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

func hoverPage(ev *events, disabled bool) func() tela.Node {
	return func() tela.Node {
		hover := aria.UseHover(aria.HoverOptions{
			Disabled:      disabled,
			OnHoverStart:  func(e aria.HoverEvent) { ev.add("start:" + string(e.PointerType)) },
			OnHoverEnd:    func(e aria.HoverEvent) { ev.add("end:" + string(e.PointerType)) },
			OnHoverChange: func(h bool) { ev.add(map[bool]string{true: "change:on", false: "change:off"}[h]) },
		})
		return Div(hover.Props, TestID("box"), Class("h-10 w-[100px]"), If(hover.Hovered, Class("w-[140px]")), Text("Card"))
	}
}

func TestHoverMouse(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, hoverPage(&ev, false), telatest.Size(300, 100))
	app.Hover(telatest.ByTestID("box"))
	if got := ev.take(); got != "start:mouse change:on" {
		t.Errorf("enter: %q", got)
	}
	if w := boxWidth(app); w != 140 {
		t.Errorf("hovered width %g", w)
	}
	app.MouseMove(250, 50)
	if got := ev.take(); got != "end:mouse change:off" {
		t.Errorf("leave: %q", got)
	}
	if w := boxWidth(app); w != 100 {
		t.Errorf("width after leaving %g", w)
	}
}

// A pen hovers; a touch does not, though the press of a touch enters the
// element for the duration of the contact.
func TestHoverIgnoresTouchButNotPen(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(hoverPage(&ev, false)), 300, 100)
	app.tap(platform.PointerTouch, 20, 20)
	if got := ev.take(); got != "" {
		t.Errorf("touch: %q", got)
	}
	app.pointer(platform.PointerPen, platform.PointerMove, 20, 20, false)
	if got := ev.take(); got != "start:pen change:on" {
		t.Errorf("pen: %q", got)
	}
	app.pointer(platform.PointerPen, platform.PointerMove, 250, 50, false)
	if got := ev.take(); got != "end:pen change:off" {
		t.Errorf("pen leaves: %q", got)
	}
}

func TestHoverDisabled(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, hoverPage(&ev, true), telatest.Size(300, 100))
	app.Hover(telatest.ByTestID("box"))
	if got := ev.take(); got != "" {
		t.Errorf("disabled: %q", got)
	}
	if w := boxWidth(app); w != 100 {
		t.Errorf("disabled width %g", w)
	}
}

// The focus ring shows after the keyboard and not after a click.
func TestFocusRingVisibleOnlyAfterKeyboard(t *testing.T) {
	page := func() tela.Node {
		ring := aria.UseFocusRing(aria.FocusRingOptions{})
		return Div(Class("flex gap-2"),
			Button(ring.Props, TestID("a"), Class("h-10 w-[100px]"),
				If(ring.Focused, Class("h-[50px]")), If(ring.FocusVisible, Class("w-[140px]")), Text("A")),
			Button(TestID("b"), Class("h-10 w-[100px]"), Text("B")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(400, 100))
	size := func() (float32, float32) {
		b := app.Bounds(telatest.ByTestID("a"))
		return b.Width, b.Height
	}
	if w, h := size(); w != 100 || h != 40 {
		t.Fatalf("idle %g x %g", w, h)
	}
	app.Click(telatest.ByTestID("a"))
	if w, h := size(); w != 100 || h != 50 {
		t.Errorf("after a click: %g x %g, want focused (50 high) and not visible (100 wide)", w, h)
	}
	app.Press("x")
	if w, h := size(); w != 140 || h != 50 {
		t.Errorf("after a key: %g x %g, want visible", w, h)
	}
	app.Click(telatest.ByTestID("a"))
	if w, _ := size(); w != 100 {
		t.Errorf("a click after the key: still visible (%g)", w)
	}
	app.Click(telatest.ByTestID("b"))
	if w, h := size(); w != 100 || h != 40 {
		t.Errorf("blurred: %g x %g", w, h)
	}
	app.Press("Tab") // from B nothing follows: wraps to A
	if w, h := size(); w != 140 || h != 50 {
		t.Errorf("Tab: %g x %g, want visible", w, h)
	}
}

func TestFocusRingWithin(t *testing.T) {
	page := func() tela.Node {
		ring := aria.UseFocusRing(aria.FocusRingOptions{Within: true})
		return Div(Class("flex flex-col"),
			Div(ring.Props, TestID("group"), Class("h-20 w-[100px]"),
				If(ring.Focused, Class("w-[120px]")), If(ring.FocusVisible, Class("w-[140px]")),
				Button(TestID("a"), Text("A")), Button(TestID("b"), Text("B"))),
			Button(TestID("c"), Text("C")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 200))
	width := func() float32 { return app.Bounds(telatest.ByTestID("group")).Width }
	app.Click(telatest.ByTestID("a"))
	if w := width(); w != 120 {
		t.Errorf("click inside: %g, want focused-within and not visible", w)
	}
	app.Press("Tab") // A to B, inside the group
	if w := width(); w != 140 {
		t.Errorf("Tab inside: %g, want visible", w)
	}
	app.Press("Tab") // B to C, out of the group
	if w := width(); w != 100 {
		t.Errorf("Tab out: %g", w)
	}
}
