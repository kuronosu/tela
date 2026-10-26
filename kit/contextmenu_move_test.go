package kit_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/kit"
)

var touchAndPen = []platform.PointerKind{platform.PointerTouch, platform.PointerPen}

func kindName(k platform.PointerKind) string {
	if k == platform.PointerTouch {
		return "touch"
	}
	return "pen"
}

// press puts a touch contact or a pen tip down at a point (a pen hovers
// there first).
func (a *menuRaw) press(kind platform.PointerKind, x, y float32) {
	a.t.Helper()
	a.event(platform.PointerEvent{Action: platform.PointerMove, Kind: kind, ID: 1, Pos: platform.Point{X: x, Y: y}})
	a.event(platform.PointerEvent{Action: platform.PointerDown, Kind: kind, ID: 1,
		Pos: platform.Point{X: x, Y: y}, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()})
}

func (a *menuRaw) release(kind platform.PointerKind, x, y float32) {
	a.t.Helper()
	a.event(platform.PointerEvent{Action: platform.PointerUp, Kind: kind, ID: 1,
		Pos: platform.Point{X: x, Y: y}, Button: platform.ButtonPrimary})
}

// hold moves the clock forward by d, stopping at the frames and at the
// gesture deadlines (the long press) on the way.
func (a *menuRaw) hold(d time.Duration) {
	a.t.Helper()
	target := a.b.Now() + d
	a.b.Wake()
	for {
		a.b.Drain(a.host)
		now := a.b.Now()
		if now >= target {
			break
		}
		next := target
		if t, ok := a.b.NextFrameTime(); ok && t > now && t < next {
			next = t
		}
		if t, ok := a.tree.NextDeadline(); ok && t > now && t < next {
			next = t
		}
		a.b.Advance(next - now)
		a.b.Wake()
	}
	a.settle()
}

// countingPage is contextPage with Children that count their builds.
func countingPage(builds *int) func() tela.Node {
	m := kit.ContextMenu[string]{
		AriaLabel: "Actions",
		Class:     "absolute top-5 left-5 h-40 w-60 rounded-md border border-dashed",
		Items: []kit.MenuItem[string]{
			{Key: "copy", Label: "Copy"}, {Key: "paste", Label: "Paste"}, {Key: "delete", Label: "Delete"},
		},
		Children: tela.C(func() tela.Node {
			*builds++
			return h.Button(h.Class("m-4 h-8 w-20"), h.Text("Inner"))
		}),
	}
	return func() tela.Node {
		return h.Div(h.Class("relative h-full w-full"), m,
			h.Button(h.Class("absolute top-5 left-[320px] h-8 w-20"), h.Text("Outside")))
	}
}

// A touch or a pen that stays down elsewhere on the area while the menu
// is open moves the open menu there and leaves it open after the release
// (Radix), without building the ContextMenu or its panel again (the
// point is a deferred read of the float pass); a quick tap elsewhere on
// the area still closes it.
func TestContextMenuLongPressWhileOpenMoves(t *testing.T) {
	for _, kind := range touchAndPen {
		t.Run(kindName(kind), func(t *testing.T) {
			children := 0
			app := mountMenuRaw(t, countingPage(&children), 480, 320)
			app.rightClick(100, 80)
			if app.find("menu", "Actions") == nil {
				t.Fatalf("no menu:\n%s", app.snapshot())
			}
			// On the area (20..260 x 20..180), below its button and left of
			// the menu.
			app.press(kind, 40, 150)
			builds := app.tree.Stats().Builds
			app.hold(600 * time.Millisecond)
			if d := app.tree.Stats().Builds - builds; d != 0 {
				t.Errorf("moving the open menu built %d components, want 0", d)
			}
			app.release(kind, 40, 150)
			if n := countMenus(app.tree.Semantics(), "Actions"); n != 1 {
				t.Fatalf("%d menus after the release of the long press:\n%s", n, app.snapshot())
			}
			n := app.find("menu", "Actions")
			if n.Bounds.X0 != 40 || n.Bounds.Y0 != 150 {
				t.Errorf("menu at (%v, %v), want (40, 150)", n.Bounds.X0, n.Bounds.Y0)
			}
			// A quick tap elsewhere on the area, right of its button and
			// above the menu, closes it.
			app.press(kind, 240, 40)
			app.release(kind, 240, 40)
			if strings.Contains(app.snapshot(), "menu") {
				t.Errorf("a tap outside left the menu open:\n%s", app.snapshot())
			}
		})
	}
}

// A right click elsewhere on the area closes the menu and opens a new one
// without building the area's children again (the ContextMenu builds, and
// its Children are the same node).
func TestContextMenuReopenKeepsChildren(t *testing.T) {
	children := 0
	app := mountMenuRaw(t, countingPage(&children), 480, 320)
	app.rightClick(100, 80)
	before := children
	app.rightClick(40, 140)
	if n := countMenus(app.tree.Semantics(), "Actions"); n != 1 {
		t.Fatalf("%d menus:\n%s", n, app.snapshot())
	}
	if n := app.find("menu", "Actions"); n.Bounds.X0 != 40 || n.Bounds.Y0 != 140 {
		t.Errorf("menu at (%v, %v), want (40, 140)", n.Bounds.X0, n.Bounds.Y0)
	}
	if children != before {
		t.Errorf("the area's children built %d times", children-before)
	}
}
