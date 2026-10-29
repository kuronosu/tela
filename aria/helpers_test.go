package aria_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
)

// events records what the callbacks of a test saw, in order.
type events struct{ log []string }

func (e *events) add(s string) { e.log = append(e.log, s) }

// take returns the events as one string and clears them.
func (e *events) take() string {
	s := strings.Join(e.log, " ")
	e.log = e.log[:0]
	return s
}

// rawApp mounts a component in a headless window and injects platform
// events directly, for what telatest has no action for yet: touch and pen
// pointers, key repeats and the semantics click of an agent.
type rawApp struct {
	t    *testing.T
	b    *headless.Backend
	host *core.Host
	win  *headless.Window
	tree *core.Tree
}

func mountRaw(t *testing.T, root tela.Component, w, h float32) *rawApp {
	t.Helper()
	b, host, win := rawwin.Mount(t, root, w, h, 0, core.Config{})
	return &rawApp{t: t, b: b, host: host, win: win, tree: host.Tree()}
}

// settle runs the frames the app asks for.
func (a *rawApp) settle() {
	a.t.Helper()
	rawwin.Settle(a.b, a.host)
}

// advance moves the clock forward by d, stopping at the frames and the
// gesture deadlines (a long press) on the way.
func (a *rawApp) advance(d time.Duration) {
	a.t.Helper()
	rawwin.Hold(a.b, a.host, d)
}

func (a *rawApp) pointer(kind platform.PointerKind, act platform.PointerAction, x, y float32, held bool) {
	a.t.Helper()
	rawwin.Pointer(a.win, kind, act, x, y, platform.ButtonPrimary, held)
	a.settle()
}

func (a *rawApp) down(kind platform.PointerKind, x, y float32) {
	a.t.Helper()
	a.pointer(kind, platform.PointerMove, x, y, false)
	a.pointer(kind, platform.PointerDown, x, y, true)
}
func (a *rawApp) move(kind platform.PointerKind, x, y float32) {
	a.t.Helper()
	a.pointer(kind, platform.PointerMove, x, y, true)
}
func (a *rawApp) up(kind platform.PointerKind, x, y float32) {
	a.t.Helper()
	a.pointer(kind, platform.PointerUp, x, y, false)
}
func (a *rawApp) cancel(kind platform.PointerKind, x, y float32) {
	a.t.Helper()
	a.pointer(kind, platform.PointerCancel, x, y, false)
}

// tap is a press and release at a point.
func (a *rawApp) tap(kind platform.PointerKind, x, y float32) {
	a.t.Helper()
	a.down(kind, x, y)
	a.up(kind, x, y)
}

// rightClick presses and releases the secondary button of a mouse, which
// makes the contextmenu event.
func (a *rawApp) rightClick(x, y float32) {
	a.t.Helper()
	a.pointer(platform.PointerMouse, platform.PointerMove, x, y, false)
	rawwin.Pointer(a.win, platform.PointerMouse, platform.PointerDown, x, y, platform.ButtonSecondary, true)
	a.settle()
	rawwin.Pointer(a.win, platform.PointerMouse, platform.PointerUp, x, y, platform.ButtonSecondary, false)
	a.settle()
}

func (a *rawApp) key(act platform.KeyAction, key platform.Key, code platform.KeyCode, repeat bool) {
	a.t.Helper()
	e := platform.KeyEvent{Action: act, Key: key, Code: code, Location: code.Location(), Repeat: repeat}
	if act == platform.KeyPress && !key.IsNamed() {
		e.Text = string(key)
	}
	a.win.Inject(e)
	a.settle()
}

const keySpace = platform.Key(" ")

// clickNode is the semantics click of an agent (no pointer, no key).
func (a *rawApp) clickNode(role, name string) {
	a.t.Helper()
	n := rawwin.FindNode(a.tree.Semantics(), role, name)
	if n == nil {
		a.t.Fatalf("no %s %q in the semantics tree:\n%s", role, name, a.tree.AriaSnapshot())
	}
	if err := a.tree.ClickNode(n.ID); err != nil {
		a.t.Fatal(err)
	}
	a.settle()
}
