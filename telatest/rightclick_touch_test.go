package telatest_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// Right() clicks with the secondary button: OnContextMenu runs once it is
// released, OnClick does not, and the pointer events carry the right button.
func TestRightClickReachesOnContextMenu(t *testing.T) {
	var rec recorder
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Button(Class("px-4 py-2"),
			OnPointerDown(func(e tela.PointerEvent) {
				rec.add("down:%v:%v:%v", e.Kind, e.Button, e.Buttons.Has(tela.MouseButtonRight))
			}),
			OnPointerUp(func(e tela.PointerEvent) { rec.add("up:%v:%v", e.Button, e.Buttons) }),
			OnContextMenu(func(e tela.MouseEvent) { rec.add("contextmenu:%v", e.Button) }),
			OnClick(func(tela.MouseEvent) { rec.add("click") }),
			Text("Target"))
	}), telatest.Size(200, 100))
	btn := telatest.ByRole("button", "Target")
	app.Click(btn, telatest.Right())
	if got, want := rec.take(), "down:mouse:right:true,up:right:0,contextmenu:right"; got != want {
		t.Errorf("a right click sent %q, want %q", got, want)
	}
	// A primary click after it is the button's own again.
	app.Click(btn)
	if got := rec.take(); got != "down:mouse:left:false,up:left:0,click" {
		t.Errorf("a click after a right click sent %q", got)
	}
}

// The actionability checks of a primary click apply, and the failure says
// right-click.
func TestRightClickWaitsForActionability(t *testing.T) {
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(func() tela.Node {
			return Button(Disabled(true), Text("Locked"))
		}), telatest.Size(200, 100), telatest.Timeout(50*time.Millisecond))
		app.Click(telatest.ByRole("button", "Locked"), telatest.Right())
	}, `cannot right-click getByRole("button", name="Locked")`, "is disabled")
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(func() tela.Node {
			return Button(Disabled(true), Text("Locked"))
		}), telatest.Size(200, 100), telatest.Timeout(50*time.Millisecond))
		app.Tap(telatest.ByRole("button", "Locked"))
	}, `cannot tap getByRole("button", name="Locked")`, "is disabled")
	mustFail(t, func(tb testing.TB) {
		app := telatest.Mount(tb, tela.C(func() tela.Node {
			return Button(Disabled(true), Text("Locked"))
		}), telatest.Size(200, 100), telatest.Timeout(50*time.Millisecond))
		app.LongPress(telatest.ByRole("button", "Locked"))
	}, `cannot long-press getByRole("button", name="Locked")`, "is disabled")
}

// Right() holds the secondary button for Delay, and ClickAt takes both.
func TestRightClickWithDelayAndAt(t *testing.T) {
	var (
		app        *telatest.App
		downAt, at time.Duration
		x, y       float32
		clicks     int
	)
	app = telatest.Mount(t, tela.C(func() tela.Node {
		return Div(Class("absolute top-5 left-5 h-40 w-60"),
			OnPointerDown(func() { downAt = app.Now() }),
			OnContextMenu(func(e tela.MouseEvent) { at, x, y = app.Now(), e.X, e.Y }),
			OnClick(func() { clicks++ }))
	}), telatest.Size(300, 220))
	app.ClickAt(120, 80, telatest.Right(), telatest.Delay(600*time.Millisecond))
	if x != 120 || y != 80 {
		t.Errorf("OnContextMenu at (%v, %v), want (120, 80)", x, y)
	}
	if held := at - downAt; held < 600*time.Millisecond {
		t.Errorf("the secondary button was held for %v before the release, want at least the Delay of 600ms", held)
	}
	if clicks != 0 {
		t.Errorf("a right click clicked %d times", clicks)
	}
	// A point outside the window still fails, with a message that teaches.
	mustFail(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(func() tela.Node { return Div() }), telatest.Size(100, 100)).
			ClickAt(500, 20, telatest.Right())
	}, "ClickAt(500, 20)", "outside the window")
}

// Tap is a touch tap: OnClick runs, from a touch pointer.
func TestTapIsATouchClick(t *testing.T) {
	var rec recorder
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Button(Class("px-4 py-2"),
			OnPointerDown(func(e tela.PointerEvent) { rec.add("down:%v:%v", e.Kind, e.IsPrimary) }),
			OnPointerUp(func(e tela.PointerEvent) { rec.add("up:%v", e.Kind) }),
			OnClick(func(e tela.MouseEvent) { rec.add("click%d", e.ClickCount) }),
			OnContextMenu(func() { rec.add("contextmenu") }),
			Text("Go"))
	}), telatest.Size(200, 100))
	btn := telatest.ByRole("button", "Go")
	app.Tap(btn)
	got := rec.take()
	if want := "down:touch:true,up:touch,click1"; got != want {
		t.Errorf("a tap sent %q, want %q", got, want)
	}
	// Each tap is a first click, even right after another.
	app.Tap(btn)
	app.Tap(btn)
	if got := rec.take(); got != "down:touch:true,up:touch,click1,down:touch:true,up:touch,click1" {
		t.Errorf("two taps sent %q", got)
	}
}

// LongPress holds a finger past the threshold: OnLongPress runs while it is
// down, and the release neither clicks nor opens a context menu of a mouse.
func TestLongPressFiresOnLongPress(t *testing.T) {
	var (
		rec recorder
		app *telatest.App
	)
	app = telatest.Mount(t, tela.C(func() tela.Node {
		return Button(Class("px-4 py-2"),
			OnLongPress(func(e tela.PointerEvent) { rec.add("long:%v", e.Kind) }),
			OnPointerUp(func(e tela.PointerEvent) { rec.add("up") }),
			OnClick(func() { rec.add("click") }),
			Text("Hold"))
	}), telatest.Size(200, 100))
	before := app.Now()
	app.LongPress(telatest.ByRole("button", "Hold"))
	if got := rec.take(); got != "long:touch,up" {
		t.Errorf("a long press sent %q, want the long press before the release and no click", got)
	}
	if held := app.Now() - before; held < 500*time.Millisecond {
		t.Errorf("the finger was held for %v, want past the 500ms threshold", held)
	}
}

// contextArea is a kit.ContextMenu over (20, 20) to (260, 180) with a button
// inside it.
func contextArea(clicks *int) func() tela.Node {
	return func() tela.Node {
		return Div(Class("relative h-full w-full"), kit.ContextMenu[string]{
			AriaLabel: "Actions",
			Class:     "absolute top-5 left-5 h-40 w-60 rounded-md border border-dashed",
			Items:     []kit.MenuItem[string]{{Key: "copy", Label: "Copy"}, {Key: "paste", Label: "Paste"}},
			Children:  Button(Class("m-4 h-8 w-20"), OnClick(func() { *clicks++ }), Text("Inner")),
		})
	}
}

// Right-clicking and long-pressing the area of a kit.ContextMenu open it, a
// tap and a mouse long press do not.
func TestContextMenuOpensFromRightClickAndLongPress(t *testing.T) {
	clicks := 0
	app := telatest.MountFunc(t, contextArea(&clicks), telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	menu := telatest.ByRole("menu", "Actions")
	inner := telatest.ByRole("button", "Inner")

	app.Tap(inner)
	app.ExpectCount(menu, 0)
	if clicks != 1 {
		t.Fatalf("a tap on the button clicked %d times, want 1", clicks)
	}
	app.Click(inner, telatest.Delay(800*time.Millisecond))
	app.ExpectCount(menu, 0)
	if clicks != 2 {
		t.Fatalf("a slow mouse click on the button clicked %d times in all, want 2", clicks)
	}

	app.LongPress(inner)
	app.ExpectCount(menu, 1)
	if clicks != 2 {
		t.Errorf("a long press clicked the button")
	}
	// The menu opened under the finger.
	b, m := app.Bounds(inner), app.Bounds(menu)
	if cx, cy := b.X+b.Width/2, b.Y+b.Height/2; m.X < cx-1 || m.X > cx+1 || m.Y < cy-1 || m.Y > cy+1 {
		t.Errorf("the menu is at (%v, %v), want the touch point (%v, %v)", m.X, m.Y, cx, cy)
	}
	app.Press("Escape")
	app.ExpectCount(menu, 0)

	app.ClickAt(100, 80, telatest.Right())
	app.ExpectCount(menu, 1)
	if m := app.Bounds(menu); m.X != 100 || m.Y != 80 {
		t.Errorf("the menu is at (%v, %v), want the pointer (100, 80)", m.X, m.Y)
	}
	// Right-clicking an element inside the area opens it too.
	app.Press("Escape")
	app.Click(inner, telatest.Right())
	app.ExpectCount(menu, 1)
	if clicks != 2 {
		t.Errorf("a right click clicked the button")
	}
}

// The actions settle like the others: under NoSettle they handle the work
// due and leave running animations where they are.
func TestRightClickTapAndLongPressUnderNoSettle(t *testing.T) {
	var rec recorder
	app := telatest.Mount(t, tela.C(func() tela.Node {
		return Button(Class("px-4 py-2"),
			OnContextMenu(func() { rec.add("contextmenu") }),
			OnClick(func() { rec.add("click") }),
			OnLongPress(func() { rec.add("long") }),
			Text("Go"))
	}), telatest.Size(200, 100), telatest.NoSettle())
	btn := telatest.ByRole("button", "Go")
	app.Click(btn, telatest.Right())
	app.Tap(btn)
	app.LongPress(btn)
	if got := rec.take(); got != "contextmenu,click,long" {
		t.Errorf("sent %q, want contextmenu,click,long", got)
	}
}
