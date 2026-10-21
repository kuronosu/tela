package aria_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

// pressPage is a pressable 100x40 box at the top left of a window, inside
// a parent that records the pointer, click and key events that reach it
// ("parent:pointerdown"...). Pressed makes the box 140 px wide.
func pressPage(ev *events, tweak func(*aria.PressOptions)) func() tela.Node {
	return func() tela.Node { return usePressPage(ev, tweak) }
}

// usePressPage is the body of pressPage, a custom hook so that a test can
// call it from its own component.
func usePressPage(ev *events, tweak func(*aria.PressOptions)) tela.Node {
	{
		o := aria.PressOptions{
			OnPressStart:  func(e aria.PressEvent) { ev.add("start:" + string(e.PointerType)) },
			OnPressUp:     func(e aria.PressEvent) { ev.add("up:" + string(e.PointerType)) },
			OnPressEnd:    func(e aria.PressEvent) { ev.add("end:" + string(e.PointerType)) },
			OnPressChange: func(p bool) { ev.add("change:" + map[bool]string{true: "on", false: "off"}[p]) },
			OnPress:       func(e aria.PressEvent) { ev.add("press:" + string(e.PointerType)) },
		}
		if tweak != nil {
			tweak(&o)
		}
		press := aria.UsePress(o)
		return Div(Class("h-full"),
			OnPointerDown(func() { ev.add("parent:pointerdown") }),
			OnPointerUp(func() { ev.add("parent:pointerup") }),
			OnClick(func() { ev.add("parent:click") }),
			OnKeyDown(func() { ev.add("parent:keydown") }),
			OnKeyUp(func() { ev.add("parent:keyup") }),
			Div(press.Props, TestID("box"), Role("button"), TabIndex(0),
				Class("h-10 w-[100px] bg-white"), If(press.Pressed, Class("w-[140px]")), Text("Go")))
	}
}

func boxWidth(app *telatest.App) float32 { return app.Bounds(telatest.ByTestID("box")).Width }

// A mouse click is a press: start, up, end, press, in that order, once, and
// the box is Pressed only while the button is down.
func TestPressMouse(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, pressPage(&ev, nil), telatest.Size(300, 100))
	app.MouseMove(20, 20)
	app.MouseDown()
	if got := ev.take(); got != "start:mouse change:on" {
		t.Errorf("down: %q", got)
	}
	if w := boxWidth(app); w != 140 {
		t.Errorf("pressed width %g, want 140", w)
	}
	app.MouseUp()
	if got := ev.take(); got != "up:mouse end:mouse change:off press:mouse" {
		t.Errorf("up: %q", got)
	}
	if w := boxWidth(app); w != 100 {
		t.Errorf("released width %g, want 100", w)
	}
	app.Click(telatest.ByTestID("box"))
	if got := ev.take(); got != "start:mouse change:on up:mouse end:mouse change:off press:mouse" {
		t.Errorf("click: %q", got)
	}
}

// Leaving the element while pressed ends the press (end, no press) and
// coming back while held starts it again; a release over the element
// presses.
func TestPressMouseLeavesAndReturns(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, pressPage(&ev, nil), telatest.Size(300, 100))
	app.MouseMove(20, 20)
	app.MouseDown()
	ev.take()
	app.MouseMove(200, 20) // out
	if got := ev.take(); got != "end:mouse change:off" {
		t.Errorf("leave: %q", got)
	}
	if w := boxWidth(app); w != 100 {
		t.Errorf("width while outside %g", w)
	}
	app.MouseMove(30, 20) // back
	if got := ev.take(); got != "start:mouse change:on" {
		t.Errorf("return: %q", got)
	}
	app.MouseUp()
	if got := ev.take(); got != "up:mouse end:mouse change:off press:mouse" {
		t.Errorf("release: %q", got)
	}
	// Released outside: no press, and nothing else after the end.
	app.MouseMove(20, 20)
	app.MouseDown()
	app.MouseMove(250, 50)
	ev.take()
	app.MouseUp()
	if got := ev.take(); got != "" {
		t.Errorf("release outside: %q", got)
	}
}

func TestPressShouldCancelOnPointerExit(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, pressPage(&ev, func(o *aria.PressOptions) { o.ShouldCancelOnPointerExit = true }), telatest.Size(300, 100))
	app.MouseMove(20, 20)
	app.MouseDown()
	ev.take()
	app.MouseMove(200, 20)
	if got := ev.take(); got != "end:mouse change:off" {
		t.Errorf("leave: %q", got)
	}
	app.MouseMove(30, 20)
	app.MouseUp()
	// The release is a plain pointer event for the parent: no press.
	if got := ev.take(); got != "parent:pointerup" {
		t.Errorf("a cancelled press came back: %q", got)
	}
	// The next press works.
	app.Click(telatest.ByTestID("box"))
	if got := ev.take(); got != "start:mouse change:on up:mouse end:mouse change:off press:mouse" {
		t.Errorf("click after the cancel: %q", got)
	}
}

// Pressing stops the propagation of the pointer events and of the click;
// ContinuePropagation lets them through.
func TestPressStopsPropagationByDefault(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, pressPage(&ev, func(o *aria.PressOptions) {
		o.OnPressStart, o.OnPressUp, o.OnPressEnd, o.OnPressChange, o.OnPress = nil, nil, nil, nil, nil
	}), telatest.Size(300, 100))
	app.Click(telatest.ByTestID("box"))
	if got := ev.take(); got != "" {
		t.Errorf("the parent saw %q", got)
	}

	app = telatest.MountFunc(t, pressPage(&ev, func(o *aria.PressOptions) {
		o.OnPressStart = func(e aria.PressEvent) { e.ContinuePropagation() }
		o.OnPressEnd = func(e aria.PressEvent) { e.ContinuePropagation() }
		o.OnPressUp, o.OnPressChange, o.OnPress = nil, nil, nil
	}), telatest.Size(300, 100))
	app.Click(telatest.ByTestID("box"))
	got := ev.take()
	// The pointer events bubble; the click is still stopped (React Aria).
	if got != "parent:pointerdown parent:pointerup" {
		t.Errorf("continued: the parent saw %q", got)
	}
}

func TestPressKeyboard(t *testing.T) {
	var ev events
	app := telatest.MountFunc(t, pressPage(&ev, nil), telatest.Size(300, 100))
	app.Focus(telatest.ByTestID("box"))
	ev.take()
	app.Press("Enter")
	if got := ev.take(); got != "start:keyboard change:on up:keyboard end:keyboard change:off press:keyboard" {
		t.Errorf("Enter: %q", got)
	}
	app.Press("Space")
	if got := ev.take(); got != "start:keyboard change:on up:keyboard end:keyboard change:off press:keyboard" {
		t.Errorf("Space: %q", got)
	}
	// Other keys are not presses.
	app.Press("a")
	if got := ev.take(); got != "parent:keydown parent:keyup" {
		t.Errorf("a: %q", got)
	}
}

// Enter presses on the key down and ignores repeats; Space starts on the
// key down (Pressed), ignores repeats and completes on the key up; a key
// down while another press is going does nothing.
func TestPressKeyboardTiming(t *testing.T) {
	var ev events
	app := mountRaw(t, tela.C(pressPage(&ev, nil)), 300, 100)
	app.key(platform.KeyPress, platform.KeyTab, platform.CodeTab, false)
	app.key(platform.KeyRelease, platform.KeyTab, platform.CodeTab, false)
	ev.take()

	app.key(platform.KeyPress, platform.KeyEnter, platform.CodeEnter, false)
	if got := ev.take(); got != "start:keyboard change:on up:keyboard end:keyboard change:off press:keyboard" {
		t.Errorf("Enter down: %q", got)
	}
	app.key(platform.KeyPress, platform.KeyEnter, platform.CodeEnter, true)
	app.key(platform.KeyPress, platform.KeyEnter, platform.CodeEnter, true)
	app.key(platform.KeyRelease, platform.KeyEnter, platform.CodeEnter, false)
	if got := ev.take(); got != "" {
		t.Errorf("Enter repeats and release: %q", got)
	}

	app.key(platform.KeyPress, keySpace, platform.CodeSpace, false)
	if got := ev.take(); got != "start:keyboard change:on" {
		t.Errorf("Space down: %q", got)
	}
	app.key(platform.KeyPress, keySpace, platform.CodeSpace, true)
	app.key(platform.KeyPress, platform.KeyEnter, platform.CodeEnter, false) // while Space is held
	if got := ev.take(); got != "" {
		t.Errorf("Space repeat: %q", got)
	}
	app.key(platform.KeyRelease, keySpace, platform.CodeSpace, false)
	if got := ev.take(); got != "up:keyboard end:keyboard change:off press:keyboard" {
		t.Errorf("Space up: %q", got)
	}

	// The focus leaving in the middle of a Space press cancels it.
	app.key(platform.KeyPress, keySpace, platform.CodeSpace, false)
	ev.take()
	app.tap(platform.PointerMouse, 250, 50) // the press elsewhere blurs the box
	if got := ev.take(); got != "parent:pointerdown end:keyboard change:off parent:pointerup parent:click" {
		t.Errorf("blur during a Space press: %q", got)
	}
	app.key(platform.KeyRelease, keySpace, platform.CodeSpace, false)
	if got := ev.take(); got != "parent:keyup" { // nothing is focused: the key goes to the outermost element
		t.Errorf("Space up after the blur: %q", got)
	}
}
