package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/kit"
)

// telatest has no touch or pen pointer yet, so the touch tests of the
// picker inject platform events into a headless window, as aria's tests do.
type touchApp struct {
	t    *testing.T
	b    *headless.Backend
	host *core.Host
	win  *headless.Window
	tree *core.Tree
}

func mountTouch(t *testing.T, root tela.Component, w, h float32) *touchApp {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: h}})
	host, err := core.NewHost(b, core.Options{Config: core.Config{Dev: true, IDPrefix: "tt-"}, Root: core.NodeOf(root)})
	if err != nil {
		t.Fatal(err)
	}
	a := &touchApp{t: t, b: b, host: host, win: host.Window().(*headless.Window), tree: host.Tree()}
	a.win.SetFocused(true)
	a.settle()
	return a
}

func (a *touchApp) settle() {
	for range 50 {
		a.b.Drain(a.host)
		if !a.b.Pending() && !a.tree.Dirty() && !a.host.NeedsFrame() {
			return
		}
		a.b.Advance(a.b.FrameInterval())
	}
}

func (a *touchApp) inject(kind platform.PointerKind, act platform.PointerAction, x, y float32, held bool) {
	e := platform.PointerEvent{Action: act, Kind: kind, ID: 1, Pos: platform.Point{X: x, Y: y}}
	switch act {
	case platform.PointerDown:
		e.Button, e.Buttons = platform.ButtonPrimary, platform.ButtonPrimary.Mask()
	case platform.PointerUp:
		e.Button = platform.ButtonPrimary
	}
	if held && act != platform.PointerDown {
		e.Buttons = platform.ButtonPrimary.Mask()
	}
	a.win.Inject(e)
	a.settle()
}

// pointer is an event of a pointer whose button is held (but for a press
// and a release).
func (a *touchApp) pointer(kind platform.PointerKind, act platform.PointerAction, x, y float32) {
	a.inject(kind, act, x, y, act != platform.PointerUp)
}

func (a *touchApp) down(kind platform.PointerKind, x, y float32) {
	a.inject(kind, platform.PointerMove, x, y, false)
	a.inject(kind, platform.PointerDown, x, y, true)
}

func (a *touchApp) find(n *semantics.Node, role, name string) *semantics.Node {
	if n.Role == role && n.Name == name {
		return n
	}
	for _, c := range n.Children {
		if f := a.find(c, role, name); f != nil {
			return f
		}
	}
	return nil
}

// node returns the semantics node of a role and name.
func (a *touchApp) node(role, name string) *semantics.Node {
	n := a.find(a.tree.Semantics(), role, name)
	if n == nil {
		a.t.Fatalf("no %s %q on screen", role, name)
	}
	return n
}

func (a *touchApp) value(role, name string) string { return a.node(role, name).Value }

// point is the point at fractions fx, fy of the box of a node.
func (a *touchApp) point(role, name string, fx, fy float32) (float32, float32) {
	b := a.node(role, name).Bounds
	return b.X0 + (b.X1-b.X0)*fx, b.Y0 + (b.Y1-b.Y0)*fy
}

func TestColorPickerTouchAndPen(t *testing.T) {
	for _, kind := range []platform.PointerKind{platform.PointerTouch, platform.PointerPen} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			var committed colorLog
			picker := kit.ColorPicker{DefaultValue: new(hsb(30, 80, 90)), Alpha: true, OnValueCommit: committed.add}
			a := mountTouch(t, tela.C(colorPage(picker)), 320, 400)

			// The area: a press places the thumb, the drag follows the finger,
			// also off the area.
			x, y := a.point("group", "Color area", 0.25, 0.5)
			a.down(kind, x, y)
			if got := a.value("slider", "Color area"); got != "Saturation 25%, Brightness 50%" {
				t.Errorf("after the touch: %q", got)
			}
			x, y = a.point("group", "Color area", 0.5, 0.25)
			a.pointer(kind, platform.PointerMove, x, y)
			a.pointer(kind, platform.PointerMove, x+400, y-100)
			a.pointer(kind, platform.PointerUp, x+400, y-100)
			if got := a.value("slider", "Color area"); got != "Saturation 100%, Brightness 100%" {
				t.Errorf("after the drag: %q", got)
			}
			if len(committed.got) != 1 {
				t.Errorf("%d commits after the area's drag, want 1", len(committed.got))
			}

			// The hue and the alpha sliders.
			x, y = a.point("group", "Hue", 0.5, 0.5)
			a.down(kind, x, y)
			x, y = a.point("group", "Hue", 0.75, 0.5)
			a.pointer(kind, platform.PointerMove, x, y)
			a.pointer(kind, platform.PointerUp, x, y)
			if got := a.value("slider", "Hue"); got != "Hue 270°" {
				t.Errorf("hue after the drag: %q", got)
			}
			x, y = a.point("group", "Alpha", 0.4, 0.5)
			a.down(kind, x, y)
			a.pointer(kind, platform.PointerUp, x, y)
			if got := a.value("slider", "Alpha"); got != "Alpha 40%" {
				t.Errorf("alpha after the tap: %q", got)
			}
			if len(committed.got) != 3 {
				t.Errorf("%d commits after the three gestures, want 3", len(committed.got))
			}
			// A cancel ends the drag and keeps the color.
			x, y = a.point("group", "Alpha", 0.6, 0.5)
			a.down(kind, x, y)
			a.pointer(kind, platform.PointerCancel, x, y)
			a.pointer(kind, platform.PointerMove, x+40, y)
			if got := a.value("slider", "Alpha"); got != "Alpha 60%" {
				t.Errorf("alpha after the cancel: %q", got)
			}
		})
	}
}
