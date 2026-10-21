package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

// Touch and pen press like the mouse, and a pointer that moves off the
// element ends the press: the hook measures the element, since a touch
// keeps its press target.
func TestPressTouchAndPen(t *testing.T) {
	for kind, name := range map[platform.PointerKind]string{platform.PointerTouch: "touch", platform.PointerPen: "pen"} {
		t.Run(name, func(t *testing.T) {
			var ev events
			app := mountRaw(t, tela.C(pressPage(&ev, nil)), 300, 100)
			app.tap(kind, 20, 20)
			if got := ev.take(); got != "start:"+name+" change:on up:"+name+" end:"+name+" change:off press:"+name {
				t.Errorf("tap: %q", got)
			}
			// Down, off the element, release: the press ended when the
			// pointer left.
			app.down(kind, 20, 20)
			ev.take()
			app.move(kind, 200, 20)
			if got := ev.take(); got != "end:"+name+" change:off" {
				t.Errorf("leave: %q", got)
			}
			app.up(kind, 200, 20)
			if got := ev.take(); got != "" {
				t.Errorf("release outside: %q", got)
			}
			// A cancelled pointer ends the press without a press.
			app.down(kind, 20, 20)
			ev.take()
			app.cancel(kind, 20, 20)
			if got := ev.take(); got != "end:"+name+" change:off" {
				t.Errorf("cancel: %q", got)
			}
		})
	}
}

// A touch that becomes a scroll cancels the press of the element it went
// down on.
func TestPressTouchScrollCancels(t *testing.T) {
	var ev events
	page := func() tela.Node {
		press := aria.UsePress(aria.PressOptions{
			OnPressStart: func(aria.PressEvent) { ev.add("start") },
			OnPressEnd:   func(aria.PressEvent) { ev.add("end") },
			OnPress:      func(aria.PressEvent) { ev.add("press") },
		})
		return Div(Class("h-[200px] w-[200px] overflow-y-auto"),
			Div(press.Props, Class("h-[60px]"), Text("row")),
			Div(Class("h-[1000px]")))
	}
	app := mountRaw(t, tela.C(page), 300, 300)
	app.tap(platform.PointerTouch, 50, 30)
	if got := ev.take(); got != "start end press" {
		t.Errorf("tap: %q", got)
	}
	app.down(platform.PointerTouch, 50, 50)
	for y := float32(50); y > 0; y -= 6 {
		app.move(platform.PointerTouch, 50, y)
	}
	app.up(platform.PointerTouch, 50, 1)
	if got := ev.take(); got != "start end" {
		t.Errorf("pan: %q", got)
	}
}

// A disabled element ignores every input.
func TestPressDisabled(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, pressPage(&ev, func(o *aria.PressOptions) { o.Disabled = true }), telatest.Size(300, 100))
	app.Click(telatest.ByTestID("box"))
	app.Focus(telatest.ByTestID("box"))
	app.Press("Enter")
	app.Press("Space")
	got := ev.take()
	if got != "parent:pointerdown parent:pointerup parent:keydown parent:keyup parent:keydown parent:keyup" {
		t.Errorf("disabled: %q", got)
	}
}

// A press in progress ends when the element becomes disabled.
func TestPressBecomesDisabledWhilePressed(t *testing.T) {
	var ev events
	var setOff tela.Setter[bool]
	page := func() tela.Node {
		off, set := tela.UseState(false)
		setOff = set
		return usePressPage(&ev, func(o *aria.PressOptions) { o.Disabled = off })
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.MouseMove(20, 20)
	app.MouseDown()
	ev.take()
	setOff.Set(true)
	app.Advance(20 * time.Millisecond)
	if got := ev.take(); got != "end:mouse change:off" {
		t.Errorf("disabled while pressed: %q", got)
	}
}

// A click that no pointer made is a virtual press: the semantics click of
// an agent, and the click a Label sends to the control it labels.
func TestPressVirtual(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(pressPage(&ev, nil)), 300, 100)
	app.clickNode("button", "Go")
	if got := ev.take(); got != "start:virtual change:on up:virtual end:virtual change:off press:virtual" {
		t.Errorf("virtual: %q", got)
	}
	// A real click is not also a virtual one.
	app.tap(platform.PointerMouse, 20, 20)
	if got := ev.take(); got != "start:mouse change:on up:mouse end:mouse change:off press:mouse" {
		t.Errorf("click: %q", got)
	}
	// And the click of that press left nothing behind for the next one.
	app.clickNode("button", "Go")
	if got := ev.take(); got != "start:virtual change:on up:virtual end:virtual change:off press:virtual" {
		t.Errorf("virtual after a click: %q", got)
	}
}

func TestPressPreventFocusOnPress(t *testing.T) {
	page := func() tela.Node {
		press := aria.UsePress(aria.PressOptions{PreventFocusOnPress: true})
		return Div(
			Button(TestID("other"), Text("Other")),
			Button(press.Props, TestID("box"), Text("Box")))
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 100))
	app.Focus(telatest.ByTestID("other"))
	app.Click(telatest.ByTestID("box"))
	app.ExpectFocused(telatest.ByTestID("other"))
}
