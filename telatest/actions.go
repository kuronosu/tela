package telatest

import (
	"errors"
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gesture"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/semantics"
)

// ---------------------------------------------------------------------
// Resolving locators.

// one waits for l to match exactly one element and returns its node. It
// fails the test at once when l matches several (strictness), and at the
// timeout when it matches none.
func (a *App) one(l Locator) *semantics.Node {
	a.tb.Helper()
	return a.oneWith(l, a.waitSettled)
}

// oneQuery is one for queries, which under [NoSettle] handle only the work
// due.
func (a *App) oneQuery(l Locator) *semantics.Node {
	a.tb.Helper()
	return a.oneWith(l, a.waitFor)
}

// oneWith is one with the wait that settles the app.
func (a *App) oneWith(l Locator, wait func(check func() (bool, string))) *semantics.Node {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	var n *semantics.Node
	wait(func() (bool, string) {
		a.tb.Helper()
		nodes := a.find(l)
		switch len(nodes) {
		case 0:
			return false, a.missMessage(l)
		case 1:
			n = nodes[0]
			return true, ""
		}
		a.fatalf("%s", a.strictMessage(l, nodes))
		return false, ""
	})
	return n
}

// target waits for l to match exactly one element that can receive the
// pointer, like Playwright's actionability checks: it is visible (laid
// out, not hidden), enabled when the action needs it, scrolled into view,
// and the element at its center is itself or one of its descendants (no
// overlay covers it). It returns the point to act on, the element's
// center in window coordinates.
func (a *App) target(l Locator, verb string, needEnabled bool) (*semantics.Node, platform.Point) {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	var (
		found *semantics.Node
		at    platform.Point
	)
	a.waitSettled(func() (bool, string) {
		a.tb.Helper()
		nodes := a.find(l)
		switch len(nodes) {
		case 0:
			return false, a.missMessage(l)
		case 1:
		default:
			a.fatalf("%s", a.strictMessage(l, nodes))
		}
		n, p, why := a.actionable(nodes[0], verb, needEnabled)
		if why != "" {
			return false, fmt.Sprintf("cannot %s %s: %s", verb, l, why)
		}
		found, at = n, p
		return true, ""
	})
	return found, at
}

// actionable checks n and returns the node (refreshed after a scroll), the
// point to act on and, when the element cannot be acted on now, the reason.
func (a *App) actionable(n *semantics.Node, verb string, needEnabled bool) (*semantics.Node, platform.Point, string) {
	a.tb.Helper()
	if n.Bounds.Empty() {
		return n, platform.Point{}, describeNode(n) + " has an empty bounding box: it is not laid out or has no size"
	}
	if needEnabled && n.Has(semantics.StateDisabled) {
		return n, platform.Point{}, describeNode(n) + " is disabled"
	}
	vp := a.tree.Viewport()
	view := gfx.Rect{X1: vp.W, Y1: vp.H}
	if !view.Contains(n.Bounds.Center()) {
		// Scroll it into view and look again.
		if err := a.tree.ScrollNodeIntoView(n.ID); err != nil && !errors.Is(err, core.ErrNodeGone) {
			return n, platform.Point{}, describeNode(n) + " cannot be scrolled into view: " + err.Error()
		}
		a.settle()
		fresh := a.tree.Semantics().Find(n.ID)
		if fresh == nil {
			return n, platform.Point{}, describeNode(n) + " is gone"
		}
		n = fresh
		if n.Bounds.Empty() || !view.Contains(n.Bounds.Center()) {
			return n, platform.Point{}, fmt.Sprintf("%s is outside the window at %v even after scrolling it into view", describeNode(n), n.Bounds)
		}
	}
	c := n.Bounds.Center()
	if hit := a.tree.HitNode(c.X, c.Y); hit != 0 && hit != n.ID && n.Find(hit) == nil && !a.coveredInline(n, hit) {
		by := "another element"
		if h := a.tree.Semantics().Find(hit); h != nil {
			by = a.describeMatch(h)
		}
		return n, platform.Point{}, fmt.Sprintf("%s receives the pointer at its center (%.0f, %.0f) instead of %s: "+
			"it covers the element, or the element has pointer-events-none", by, c.X, c.Y, describeNode(n))
	}
	return n, platform.Point{X: c.X, Y: c.Y}, ""
}

// coveredInline reports whether the hit node is an ancestor of the inline
// element n: a hit on text lands on the element that lays it out.
func (a *App) coveredInline(n *semantics.Node, hit semantics.NodeID) bool {
	if !n.Inline && n.Kind != semantics.KindText {
		return false
	}
	h := a.tree.Semantics().Find(hit)
	return h != nil && h.Find(n.ID) != nil
}

// Bounds returns the border box of the element l locates, in logical
// pixels from the window's top-left corner, after transforms and scroll
// offsets: the same rectangle as tela.ElementRef.Bounds. For an inline
// element it bounds its text.
//
//	r := app.Bounds(telatest.ByRole("button", "Save"))
//	if r.Width < 80 { t.Errorf("the button is %gpx wide", r.Width) }
func (a *App) Bounds(l Locator) tela.Rect {
	a.tb.Helper()
	n := a.oneQuery(l)
	r := n.Bounds
	return tela.Rect{X: r.X0, Y: r.Y0, Width: r.Width(), Height: r.Y1 - r.Y0}
}

// Count returns how many elements l matches now, after settling the app
// (under [NoSettle], handling only the work due: running animations stay
// where they are). It does not wait.
func (a *App) Count(l Locator) int {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	a.settleQuery()
	return len(a.find(l))
}

// ---------------------------------------------------------------------
// Pointer.

// pointer injects a pointer event and settles the app.
func (a *App) pointer(act platform.PointerAction, p platform.Point, button platform.PointerButton, buttons platform.Buttons) {
	a.tb.Helper()
	a.win.Inject(platform.PointerEvent{Action: act, Pos: p, Button: button, Buttons: buttons})
	a.settleAfter()
}

// moveTo moves the pointer, unless it is already there.
func (a *App) moveTo(p platform.Point) {
	a.tb.Helper()
	if a.hasPos && a.pos == p {
		return
	}
	a.pos, a.hasPos = p, true
	a.pointer(platform.PointerMove, p, platform.ButtonNone, a.held)
}

func (a *App) down() { a.tb.Helper(); a.press(platform.ButtonPrimary) }
func (a *App) up()   { a.tb.Helper(); a.release(platform.ButtonPrimary) }

// press presses button of the mouse at its position.
func (a *App) press(button platform.PointerButton) {
	a.tb.Helper()
	if a.held != 0 {
		if button == platform.ButtonPrimary && a.held.Has(button) {
			a.fatalf("the primary button is already pressed; release it with MouseUp first")
		}
		a.fatalf("cannot press the %s button: another button is already pressed; release it with MouseUp first", buttonName(button))
	}
	a.held = button.Mask()
	a.pointer(platform.PointerDown, a.pos, button, a.held)
}

// release releases button of the mouse at its position.
func (a *App) release(button platform.PointerButton) {
	a.tb.Helper()
	if !a.held.Has(button) {
		a.fatalf("the %s button is not pressed; press it with MouseDown first", buttonName(button))
	}
	a.held &^= button.Mask()
	a.pointer(platform.PointerUp, a.pos, button, a.held)
}

func buttonName(b platform.PointerButton) string {
	if b == platform.ButtonSecondary {
		return "secondary"
	}
	return "primary"
}

// touchID is the id of the contact [App.Tap] and [App.LongPress] use.
const touchID platform.PointerID = 1

// touchDown puts a finger down at p: a touch pointer with its primary
// contact, which no hover precedes.
func (a *App) touchDown(p platform.Point) {
	a.tb.Helper()
	a.win.Inject(platform.PointerEvent{Action: platform.PointerDown, Kind: platform.PointerTouch, ID: touchID,
		Pos: p, Button: platform.ButtonPrimary, Buttons: platform.ButtonPrimary.Mask()})
	a.settleAfter()
}

// touchUp lifts the finger at p; a touch pointer leaves the window with it.
func (a *App) touchUp(p platform.Point) {
	a.tb.Helper()
	a.win.Inject(platform.PointerEvent{Action: platform.PointerUp, Kind: platform.PointerTouch, ID: touchID,
		Pos: p, Button: platform.ButtonPrimary})
	a.win.Inject(platform.PointerEvent{Action: platform.PointerLeave, Kind: platform.PointerTouch, ID: touchID, Pos: p})
	a.settleAfter()
}

// ClickOption configures [App.Click] and [App.ClickAt].
type ClickOption func(*clickConfig)

type clickConfig struct {
	delay  time.Duration
	button platform.PointerButton
}

// Delay holds the button down for d of app time between the press and the
// release, like Playwright's click delay. It is how a test makes a long
// press of the mouse (the arena recognizes one after 500 ms): the handler
// runs while the button is held and the release then ends the press without
// a click. For a touch long press, which opens a context menu, use
// [App.LongPress].
func Delay(d time.Duration) ClickOption { return func(c *clickConfig) { c.delay = d } }

// Right makes [App.Click] and [App.ClickAt] press and release the secondary
// (right) mouse button, like Playwright's click({button: 'right'}): real
// pointer events with the secondary button, so the element's OnContextMenu
// runs once the button is released (a kit.ContextMenu opens) and its OnClick
// does not. The element must be as actionable as for a primary click:
// visible, enabled, in view and not covered. It combines with [Delay].
//
//	app.Click(telatest.ByTestID("area"), telatest.Right())
//	app.ClickAt(120, 80, telatest.Right())
func Right() ClickOption { return func(c *clickConfig) { c.button = platform.ButtonSecondary } }

func clickConfigOf(opts []ClickOption) clickConfig {
	cfg := clickConfig{button: platform.ButtonPrimary}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// Click moves the pointer to the center of the element l locates, presses
// the primary button and releases it: real pointer events (hover, :active,
// focus on press, the gesture arena and the click), not a call to the
// handler. Each Click is a first click (ClickCount 1) even right after
// another, as in Playwright; use [App.DoubleClick] for two. It waits for the element to be visible, enabled, in view and not
// covered by another element, and fails the test when l matches none or
// several. With [Right] it presses the secondary button instead (the
// element's OnContextMenu runs, not its OnClick); [App.Tap] and
// [App.LongPress] are the touch counterparts.
func (a *App) Click(l Locator, opts ...ClickOption) {
	a.tb.Helper()
	cfg := clickConfigOf(opts)
	verb := "click"
	if cfg.button == platform.ButtonSecondary {
		verb = "right-click"
	}
	_, p := a.target(l, verb, true)
	a.clickPoint(p, cfg)
}

// clickPoint presses and releases the configured button at p.
func (a *App) clickPoint(p platform.Point, cfg clickConfig) {
	a.tb.Helper()
	a.moveTo(p)
	a.tree.BreakClickSequence()
	a.press(cfg.button)
	if cfg.delay > 0 {
		a.advance(cfg.delay)
		a.settleAfter()
	}
	a.release(cfg.button)
}

// Tap taps the element l locates with a finger: it puts a touch pointer
// down at the element's center and lifts it, with real touch pointer events
// (no hover; :active, the gesture arena and the click as for a finger), like
// Playwright's tap. The element's OnClick runs, with a touch pointer in its
// event. It waits for the element to be visible, enabled, in view and not
// covered, and fails the test when l matches none or several.
//
//	app.Tap(telatest.ByRole("button", "Save"))
func (a *App) Tap(l Locator) {
	a.tb.Helper()
	_, p := a.target(l, "tap", true)
	a.tree.BreakClickSequence()
	a.touchDown(p)
	a.touchUp(p)
}

// longPressHold is how long [App.LongPress] keeps the finger down: the
// engine's long-press threshold and a margin past it.
const longPressHold = gesture.LongPressTimeout + 100*time.Millisecond

// LongPress presses the element l locates with a finger at its center,
// holds it past the long-press threshold (500 ms of app time: the clock
// moves, so the timers run) and lifts it. The element's OnLongPress runs
// while the finger is down and the press then ends without a click. A touch
// long press also opens the menu of a kit.ContextMenu or
// aria.UseContextMenuTrigger (a long press of the mouse does not; see
// [App.Click] with [Delay]). It waits for the element to be visible,
// enabled, in view and not covered.
//
//	app.LongPress(telatest.ByTestID("photo"))
//	app.ExpectCount(telatest.ByRole("menu", "Actions"), 1)
func (a *App) LongPress(l Locator) {
	a.tb.Helper()
	_, p := a.target(l, "long-press", true)
	a.tree.BreakClickSequence()
	a.touchDown(p)
	a.advance(longPressHold)
	a.settleAfter()
	a.touchUp(p)
}

// ClickAt clicks at the point (x, y) of the window, in logical pixels from
// its top-left corner, with the same real pointer events as [App.Click]
// (hover, :active, focus on press, the gesture arena and the click) but
// without looking for an element: whatever is under the point receives
// them, like a user clicking there. It is for what a locator cannot name
// (a canvas, a chart, a drop zone found with [App.Bounds]) and fails the
// test when the point is outside the window. [Right] clicks with the
// secondary button, to open a context menu at a point.
//
//	r := app.Bounds(telatest.ByTestID("chart"))
//	app.ClickAt(r.X+r.Width/2, r.Y+10)
//	app.ClickAt(r.X+20, r.Y+20, telatest.Right())
func (a *App) ClickAt(x, y float32, opts ...ClickOption) {
	a.tb.Helper()
	a.clickPoint(a.windowPoint("ClickAt", x, y), clickConfigOf(opts))
}

// windowPoint checks that (x, y) is inside the window and returns it as a
// pointer position.
func (a *App) windowPoint(what string, x, y float32) platform.Point {
	a.tb.Helper()
	a.mustBeOpen()
	vp := a.tree.Viewport()
	if !(x >= 0 && y >= 0 && x < vp.W && y < vp.H) {
		a.fatalf("%s(%g, %g): the point is outside the window, which is %gx%g logical pixels; "+
			"give coordinates from its top-left corner (App.Bounds returns them for an element)", what, x, y, vp.W, vp.H)
	}
	return platform.Point{X: x, Y: y}
}

// DoubleClick clicks the element twice in a row, so the second click has a
// click count of 2 and OnDoubleClick runs. See [App.Click].
func (a *App) DoubleClick(l Locator) {
	a.tb.Helper()
	_, p := a.target(l, "double-click", true)
	a.moveTo(p)
	a.tree.BreakClickSequence()
	a.down()
	a.up()
	a.down()
	a.up()
}

// Hover moves the pointer to the center of the element l locates, so its
// hover: and group-hover: states apply. It waits for the element to be
// visible, in view and not covered.
func (a *App) Hover(l Locator) {
	a.tb.Helper()
	_, p := a.target(l, "hover", false)
	a.moveTo(p)
}

// Focus gives the keyboard focus to the element l locates, like calling
// focus() on it: OnBlur and OnFocus run. It fails the test when the element
// cannot take the focus (give it a Button, TabIndex or an interactive role
// element).
func (a *App) Focus(l Locator) {
	a.tb.Helper()
	n := a.one(l)
	if err := a.tree.FocusNode(n.ID); err != nil {
		if errors.Is(err, core.ErrNotFocusable) {
			a.fatalf("cannot focus %s: it is not focusable; a Button or an element with h.TabIndex is", describeNode(n))
		}
		a.fatalf("cannot focus %s: %v", describeNode(n), err)
	}
	a.settleAfter()
}

// Scroll moves the pointer to the center of the element l locates and
// scrolls the wheel there by (dx, dy) logical pixels, positive to the right
// and down, as a touchpad does: the nearest scroll container under the
// pointer that can move scrolls at once, with no animation. Use Press("PageDown")
// or Press("End") for the animated key scrolling.
func (a *App) Scroll(l Locator, dx, dy float32) {
	a.tb.Helper()
	_, p := a.target(l, "scroll", false)
	a.moveTo(p)
	a.win.Inject(platform.ScrollEvent{Pos: p, Delta: platform.Point{X: dx, Y: dy}})
	a.settleAfter()
}

// MouseMove moves the pointer to (x, y), in logical pixels from the window's
// top-left corner.
func (a *App) MouseMove(x, y float32) {
	a.tb.Helper()
	a.mustBeOpen()
	a.moveTo(platform.Point{X: x, Y: y})
}

// MouseDown presses the primary button at the pointer's position (move it
// first with MouseMove, Hover or Click).
func (a *App) MouseDown() {
	a.tb.Helper()
	a.mustBeOpen()
	if !a.hasPos {
		a.fatalf("MouseDown: the pointer is outside the window; move it first with MouseMove, Hover or Click")
	}
	a.down()
}

// MouseUp releases the primary button.
func (a *App) MouseUp() {
	a.tb.Helper()
	a.mustBeOpen()
	a.up()
}

// DragOption configures [App.Drag], [App.DragBy] and [App.DragFromTo].
type DragOption func(*dragConfig)

type dragConfig struct{ steps int }

// Steps sets how many pointer moves the drag is made of (default 12): the
// pointer moves in equal steps between the start and the end, and each move
// runs a frame, as a window would.
func Steps(n int) DragOption { return func(c *dragConfig) { c.steps = n } }

func dragSteps(opts []DragOption) int {
	cfg := dragConfig{steps: 12}
	for _, o := range opts {
		o(&cfg)
	}
	return max(cfg.steps, 1)
}

// Drag presses the primary button at the center of from, moves the pointer
// in steps to the center of to, and releases it there: a real drag through
// the gesture arena (OnDragStart, OnDrag, OnDragEnd), the pointer capture
// and the hover states of the elements on the way. The center of to is read
// once the drag has started.
func (a *App) Drag(from, to Locator, opts ...DragOption) {
	a.tb.Helper()
	steps := dragSteps(opts)
	_, p := a.target(from, "drag", true)
	a.moveTo(p)
	a.down()
	_, end := a.target(to, "drag to", false)
	a.dragPath(p, end, steps)
	a.up()
}

// DragBy drags the element l locates by (dx, dy) logical pixels from its
// center. See [App.Drag].
func (a *App) DragBy(l Locator, dx, dy float32, opts ...DragOption) {
	a.tb.Helper()
	steps := dragSteps(opts)
	_, p := a.target(l, "drag", true)
	a.moveTo(p)
	a.down()
	a.dragPath(p, platform.Point{X: p.X + dx, Y: p.Y + dy}, steps)
	a.up()
}

// DragFromTo presses the primary button at the point from, moves the
// pointer in steps to the point to and releases it there, in logical pixels
// from the window's top-left corner: the drag of [App.Drag] without
// locators, for sources and targets a locator cannot name (a canvas, a
// slider track at a given value). It fails the test when a point is
// outside the window.
//
//	r := app.Bounds(telatest.ByTestID("track"))
//	app.DragFromTo(tela.Point{X: r.X + 4, Y: r.Y + 8}, tela.Point{X: r.X + r.Width/2, Y: r.Y + 8})
func (a *App) DragFromTo(from, to tela.Point, opts ...DragOption) {
	a.tb.Helper()
	steps := dragSteps(opts)
	p := a.windowPoint("DragFromTo", from.X, from.Y)
	end := a.windowPoint("DragFromTo", to.X, to.Y)
	a.moveTo(p)
	a.down()
	a.dragPath(p, end, steps)
	a.up()
}

// dragPath moves the pointer from from to to in equal steps, the last one
// exactly at to.
func (a *App) dragPath(from, to platform.Point, steps int) {
	a.tb.Helper()
	for i := 1; i <= steps; i++ {
		f := float32(i) / float32(steps)
		p := platform.Point{X: from.X + (to.X-from.X)*f, Y: from.Y + (to.Y-from.Y)*f}
		if i == steps {
			p = to
		}
		a.moveTo(p)
	}
}
