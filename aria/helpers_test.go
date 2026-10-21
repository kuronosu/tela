package aria_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/semantics"
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
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: h}})
	host, err := core.NewHost(b, core.Options{
		Config: core.Config{Dev: true, IDPrefix: "tt-"},
		Root:   core.NodeOf(root),
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &rawApp{t: t, b: b, host: host, win: host.Window().(*headless.Window), tree: host.Tree()}
	a.win.SetFocused(true)
	a.settle()
	return a
}

// settle runs the frames the app asks for.
func (a *rawApp) settle() {
	a.t.Helper()
	for range 50 {
		a.b.Drain(a.host)
		if !a.b.Pending() && !a.tree.Dirty() && !a.host.NeedsFrame() {
			return
		}
		a.b.Advance(a.b.FrameInterval())
	}
}

func (a *rawApp) pointer(kind platform.PointerKind, act platform.PointerAction, x, y float32, held bool) {
	a.t.Helper()
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
	var find func(n *semantics.Node) *semantics.Node
	find = func(n *semantics.Node) *semantics.Node {
		if n.Role == role && n.Name == name {
			return n
		}
		for _, c := range n.Children {
			if f := find(c); f != nil {
				return f
			}
		}
		return nil
	}
	n := find(a.tree.Semantics())
	if n == nil {
		a.t.Fatalf("no %s %q in the semantics tree:\n%s", role, name, a.tree.AriaSnapshot())
	}
	if err := a.tree.ClickNode(n.ID); err != nil {
		a.t.Fatal(err)
	}
	a.settle()
}
