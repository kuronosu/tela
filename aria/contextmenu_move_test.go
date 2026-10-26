package aria_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
)

var touchAndPen = []platform.PointerKind{platform.PointerTouch, platform.PointerPen}

func kindName(k platform.PointerKind) string {
	if k == platform.PointerTouch {
		return "touch"
	}
	return "pen"
}

// A touch or a pen that stays down elsewhere on the area while the menu
// is open moves the open menu there, and its release does not close it
// (Radix): the pointerdown was outside the menu, but the long press takes
// the menu out of that press. A quick tap elsewhere on the area still
// closes it.
func TestContextMenuLongPressWhileOpenMoves(t *testing.T) {
	for _, kind := range touchAndPen {
		t.Run(kindName(kind), func(t *testing.T) {
			var ev events
			app := mountRaw(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300)
			app.rightClick(90, 60)
			if !hasMenu(app) {
				t.Fatal("no menu")
			}
			// On the area, below its button and left of the menu.
			app.down(kind, 30, 130)
			app.advance(600 * time.Millisecond)
			if x, y, _, _ := menuBounds(t, app); x != 30 || y != 130 {
				t.Errorf("after the long press the menu is at (%v, %v), want (30, 130)", x, y)
			}
			app.up(kind, 30, 130)
			if n := countNodes(app.tree.Semantics(), "menu", "Actions"); n != 1 {
				t.Fatalf("%d menus after the release of the long press:\n%s", n, app.tree.AriaSnapshot())
			}
			if x, y, _, _ := menuBounds(t, app); x != 30 || y != 130 {
				t.Errorf("after the release the menu is at (%v, %v), want (30, 130)", x, y)
			}
			if f := focusedNode(app.tree.Semantics()); f == nil || f.Role != "menu" && f.Role != "menuitem" {
				t.Errorf("the moved menu lost the focus:\n%s", app.tree.AriaSnapshot())
			}
			// A quick tap elsewhere on the area (right of its button, above
			// the menu) closes it, as any press outside does.
			app.tap(kind, 200, 30)
			if hasMenu(app) {
				t.Errorf("a tap outside left the menu open:\n%s", app.tree.AriaSnapshot())
			}
			if got := ev.take(); got != "" {
				t.Errorf("events %q", got)
			}
		})
	}
}

// The same with the menu opened by a long press: a second long press
// moves it and leaves it open.
func TestContextMenuSecondLongPressMoves(t *testing.T) {
	for _, kind := range touchAndPen {
		t.Run(kindName(kind), func(t *testing.T) {
			var ev events
			app := mountRaw(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300)
			app.down(kind, 130, 40) // on the area, right of its button
			app.advance(600 * time.Millisecond)
			app.up(kind, 130, 40)
			if x, y, _, _ := menuBounds(t, app); x != 130 || y != 40 {
				t.Fatalf("menu at (%v, %v), want (130, 40)", x, y)
			}
			app.down(kind, 30, 120)
			app.advance(600 * time.Millisecond)
			app.up(kind, 30, 120)
			if n := countNodes(app.tree.Semantics(), "menu", "Actions"); n != 1 {
				t.Fatalf("%d menus after the second long press:\n%s", n, app.tree.AriaSnapshot())
			}
			if x, y, _, _ := menuBounds(t, app); x != 30 || y != 120 {
				t.Errorf("menu at (%v, %v), want (30, 120)", x, y)
			}
			if got := ev.take(); got != "" {
				t.Errorf("events %q", got)
			}
		})
	}
}

// A long press in another area while a menu is open closes that menu at
// the release and opens the other area's: only the long-pressed area's
// own menu is kept out of the press.
func TestContextMenuLongPressInAnotherArea(t *testing.T) {
	for _, kind := range touchAndPen {
		t.Run(kindName(kind), func(t *testing.T) {
			app := mountRaw(t, tela.C(twoAreas), 500, 300)
			app.rightClick(50, 50)
			if findNode(app.tree.Semantics(), "menu", "Left") == nil {
				t.Fatalf("no left menu:\n%s", app.tree.AriaSnapshot())
			}
			app.down(kind, 250, 100)
			app.advance(600 * time.Millisecond)
			app.up(kind, 250, 100)
			s := app.tree.Semantics()
			if findNode(s, "menu", "Left") != nil || findNode(s, "menu", "Right") == nil {
				t.Fatalf("after a long press in the other area:\n%s", app.tree.AriaSnapshot())
			}
			n := findNode(s, "menu", "Right")
			if n.Bounds.X0 != 250 || n.Bounds.Y0 != 100 {
				t.Errorf("right menu at (%v, %v), want (250, 100)", n.Bounds.X0, n.Bounds.Y0)
			}
		})
	}
}

// recordingPage is ctxPage that keeps the result of every build.
func recordingPage(rec *[]aria.ContextMenuTriggerResult, ev *events) func() tela.Node {
	return func() tela.Node {
		cm := aria.UseContextMenuTrigger(aria.ContextMenuTriggerOptions{})
		*rec = append(*rec, cm)
		return Div(Class("relative h-full"),
			Div(cm.TriggerProps, Class("absolute left-5 top-5 h-[120px] w-[200px]"),
				Button(OnClick(func() { ev.add("inner") }), Class("mt-4 ml-4 h-8 w-20"), Text("Inner"))),
			Portal(If(cm.Open, ctxMenu{cm, ev})))
	}
}

// Each build's Anchor belongs to the opening it saw: it follows a move of
// the open menu and keeps its point when the menu opens again elsewhere,
// so a menu exiting in a Presence stays where it was (the anchor of
// PopoverOptions is this Anchor).
func TestContextMenuAnchorPerOpening(t *testing.T) {
	var rec []aria.ContextMenuTriggerResult
	var ev events
	app := mountRaw(t, tela.C(recordingPage(&rec, &ev)), 400, 300)
	app.rightClick(90, 60)
	first := rec[len(rec)-1]
	if !first.Open {
		t.Fatal("no menu")
	}
	if a := first.Anchor(); a.X != 90 || a.Y != 60 {
		t.Fatalf("Anchor %+v, want (90, 60)", a)
	}
	// Moved by a long press: the opening's Anchor follows, with no build
	// of the component that holds the area.
	builds := len(rec)
	app.down(platform.PointerPen, 200, 30)
	app.advance(600 * time.Millisecond)
	app.up(platform.PointerPen, 200, 30)
	if a := first.Anchor(); a.X != 200 || a.Y != 30 {
		t.Errorf("after a move Anchor is %+v, want (200, 30)", a)
	}
	if len(rec) != builds {
		t.Errorf("moving the open menu built the area's component %d times", len(rec)-builds)
	}
	// Opened again elsewhere: the first opening's Anchor keeps its point,
	// the new one has the new point.
	app.rightClick(40, 130)
	second := rec[len(rec)-1]
	if a := second.Anchor(); a.X != 40 || a.Y != 130 {
		t.Errorf("the second opening's Anchor is %+v, want (40, 130)", a)
	}
	if a := first.Anchor(); a.X != 200 || a.Y != 30 {
		t.Errorf("the first opening's Anchor moved to %+v, want it kept at (200, 30)", a)
	}
	if x, y, _, _ := menuBounds(t, app); x != 40 || y != 130 {
		t.Errorf("menu at (%v, %v), want (40, 130)", x, y)
	}
}
