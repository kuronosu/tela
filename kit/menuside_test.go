package kit_test

import (
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/kit"
)

// The side a menu or popover is on decides how it enters: one above its
// trigger grows from its bottom edge and slides up, one below it from its
// top edge and slides down. The Placement the build reads comes from the
// float pass of the same frame, which runs when the frame is painted, so the
// frame builds again before it is shown (ADR-164): the first frame is
// already styled for the side the element ends on, with no jump a frame or
// two later.

// findRole returns the first node of the semantics tree with the role.
func (a *menuRaw) findRole(role string) *semantics.Node {
	var walk func(n *semantics.Node) *semantics.Node
	walk = func(n *semantics.Node) *semantics.Node {
		if n.Role == role {
			return n
		}
		for _, c := range n.Children {
			if f := walk(c); f != nil {
				return f
			}
		}
		return nil
	}
	return walk(a.tree.Semantics())
}

// pressFrames presses and releases button at (x, y) without settling and
// returns the bounds of the first node with the role in each of the next n
// frames (the zero rectangle while there is none), and where it ends.
func (a *menuRaw) pressFrames(button platform.PointerButton, x, y float32, role string, n int) (frames []gfx.Rect, final gfx.Rect) {
	a.t.Helper()
	p := platform.Point{X: x, Y: y}
	a.move(x, y)
	a.event(platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerMouse, ID: 1, Pos: p, Button: button, Buttons: button.Mask()})
	a.win.Inject(platform.PointerEvent{Action: platform.PointerUp, Kind: platform.PointerMouse, ID: 1, Pos: p, Button: button})
	for range n {
		a.b.Drain(a.host)
		var r gfx.Rect
		if m := a.findRole(role); m != nil {
			r = m.Bounds
		}
		frames = append(frames, r)
		a.b.Advance(a.b.FrameInterval())
		a.b.Wake()
	}
	a.advance(400 * time.Millisecond)
	m := a.findRole(role)
	if m == nil {
		a.t.Fatalf("no %s is open:\n%s", role, a.snapshot())
	}
	return frames, m.Bounds
}

// expectEntersUpwards checks the frames of an element that ends above its
// trigger: from the first frame it is seen in, it is never above its final
// place (the slide of the other side) and only moves up toward it.
func expectEntersUpwards(t *testing.T, frames []gfx.Rect, final gfx.Rect) {
	t.Helper()
	var seen []gfx.Rect
	for _, f := range frames {
		if f != (gfx.Rect{}) {
			seen = append(seen, f)
		}
	}
	if len(seen) < 3 {
		t.Fatalf("the element was seen in %d of the first %d frames: %v", len(seen), len(frames), frames)
	}
	const eps = 0.01
	for i, f := range seen {
		if f.Y1 < final.Y1-eps || f.Y0 < final.Y0-eps {
			t.Errorf("frame %d: the element is at y %v..%v, above its final %v..%v: it entered from the wrong side", i, f.Y0, f.Y1, final.Y0, final.Y1)
		}
		if i > 0 && (f.Y1 > seen[i-1].Y1+eps || f.Y0 > seen[i-1].Y0+eps) {
			t.Errorf("frame %d: the element jumped from y %v..%v to %v..%v", i, seen[i-1].Y0, seen[i-1].Y1, f.Y0, f.Y1)
		}
	}
}

func TestContextMenuFlippedAboveIsStyledForItsSideInTheFirstFrame(t *testing.T) {
	app := mountMenuRaw(t, roomPage, 600, 400)
	frames, final := app.pressFrames(platform.ButtonSecondary, 100, 350, "menu", 8)
	if final.Y1 > 350.5 {
		t.Fatalf("the menu ends at %v, want it above the pointer", final)
	}
	expectEntersUpwards(t, frames, final)
}

func TestDropdownMenuFlippedAboveIsStyledForItsSideInTheFirstFrame(t *testing.T) {
	page := func() tela.Node {
		return h.Div(h.Class("relative h-full w-full"), h.Div(h.Class("absolute bottom-4 left-20"), kit.DropdownMenu[string]{
			AriaLabel: "More",
			Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
				return h.Button(p, h.Class("h-8 w-24 border"), h.Text("Open"))
			}),
			Items: roomItems(),
		}))
	}
	app := mountMenuRaw(t, page, 400, 400)
	frames, final := app.pressFrames(platform.ButtonPrimary, 120, 370, "menu", 8)
	if final.Y1 > 370 {
		t.Fatalf("the menu ends at %v, want it above its trigger", final)
	}
	expectEntersUpwards(t, frames, final)
}

func TestPopoverFlippedAboveIsStyledForItsSideInTheFirstFrame(t *testing.T) {
	page := func() tela.Node {
		return h.Div(h.Class("relative h-full w-full"), h.Div(h.Class("absolute bottom-4 left-20"), kit.Popover{
			AriaLabel: "More",
			Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
				return h.Button(p, h.Class("h-8 w-24 border"), h.Text("Open"))
			}),
			Children: tela.C(func() tela.Node { return h.Div(h.Class("h-40 w-40"), h.Text("Content")) }),
		}))
	}
	app := mountMenuRaw(t, page, 400, 300)
	frames, final := app.pressFrames(platform.ButtonPrimary, 120, 270, "dialog", 8)
	if final.Y1 > 270 {
		t.Fatalf("the popover ends at %v, want it above its trigger", final)
	}
	expectEntersUpwards(t, frames, final)
}
