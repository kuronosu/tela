// Package exampletest is the test harness shared by the examples: it mounts
// a screen in a headless window, injects pointer events the way a backend
// would, finds boxes by their text, and checks goldens at scale 1 and 2.
//
// It stands in for the public telatest package (phase 4, T10). When that
// lands, the examples' tests should move to it.
package exampletest

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// App is a screen running in a headless window.
type App struct {
	T       *testing.T
	Backend *headless.Backend
	Host    *core.Host
	Window  *headless.Window
	logs    []string
	x, y    float32
}

// Open mounts root (built as a component) in a w x h logical window at
// scale 1, in dev mode: class errors and other warnings are collected and
// reported by [App.CheckLogs].
func Open(t *testing.T, root func() tela.Node, w, h float32) *App {
	t.Helper()
	a := &App{T: t}
	a.Backend = headless.New(headless.Options{Size: platform.Size{W: w, H: h}})
	host, err := core.NewHost(a.Backend, core.Options{
		Config: core.Config{Dev: true, Log: func(m string) { a.logs = append(a.logs, m) }},
		Root:   core.Component(root),
	})
	if err != nil {
		t.Fatal(err)
	}
	a.Host = host
	a.Window = host.Window().(*headless.Window)
	// Like Run, run the functions given to tela.Post, and unmount at the
	// end so effects clean up (stopping goroutines).
	stop := host.Tree().ReceivePosts()
	t.Cleanup(func() {
		stop()
		host.Tree().Unmount()
	})
	a.Backend.Drain(host)
	return a
}

// Tree returns the window's tree.
func (a *App) Tree() *core.Tree { return a.Host.Tree() }

// CheckLogs fails the test for every dev message (unknown class, missing
// key...) logged so far.
func (a *App) CheckLogs() {
	a.T.Helper()
	for _, m := range a.logs {
		a.T.Errorf("dev message: %s", m)
	}
	a.logs = nil
}

// Frame advances the clock by one frame interval and handles what the
// window and the tree have queued.
func (a *App) Frame() {
	a.Backend.Advance(a.Backend.FrameInterval())
	a.Backend.Drain(a.Host)
}

// Advance moves the clock forward by d and handles the resulting events.
func (a *App) Advance(d time.Duration) {
	a.Backend.Advance(d)
	a.Backend.Drain(a.Host)
}

func (a *App) send(e platform.PointerEvent) {
	a.Window.Inject(e)
	a.Backend.Drain(a.Host)
	a.Frame()
}

var primary = platform.ButtonPrimary.Mask()

// Move moves the pointer to (x, y).
func (a *App) Move(x, y float32) {
	a.x, a.y = x, y
	a.send(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: x, Y: y}})
}

// Down presses the primary button at the current position.
func (a *App) Down() {
	a.send(platform.PointerEvent{Action: platform.PointerDown, Pos: platform.Point{X: a.x, Y: a.y},
		Button: platform.ButtonPrimary, Buttons: primary})
}

// Up releases the primary button at the current position.
func (a *App) Up() {
	a.send(platform.PointerEvent{Action: platform.PointerUp, Pos: platform.Point{X: a.x, Y: a.y},
		Button: platform.ButtonPrimary})
}

// Click moves to (x, y), presses and releases.
func (a *App) Click(x, y float32) {
	a.Move(x, y)
	a.Down()
	a.Up()
}

// Drag presses at from, moves to to in steps with the button held, and
// releases. A frame is handled after every event, as in a window.
func (a *App) Drag(from, to gfx.Point, steps int) {
	a.Move(from.X, from.Y)
	a.Down()
	for i := 1; i <= steps; i++ {
		f := float32(i) / float32(steps)
		a.x, a.y = from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f
		a.send(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: a.x, Y: a.y}, Buttons: primary})
	}
	a.Up()
}

// Find returns the first box, in tree order, for which match is true.
func (a *App) Find(match func(core.InspectNode) bool) (core.InspectNode, bool) {
	return find(a.Tree().Inspect(), match)
}

func find(n core.InspectNode, match func(core.InspectNode) bool) (core.InspectNode, bool) {
	if match(n) {
		return n, true
	}
	for _, c := range n.Children {
		if f, ok := find(c, match); ok {
			return f, true
		}
	}
	return core.InspectNode{}, false
}

// All returns every box for which match is true, in tree order.
func (a *App) All(match func(core.InspectNode) bool) []core.InspectNode {
	var out []core.InspectNode
	var walk func(n core.InspectNode)
	walk = func(n core.InspectNode) {
		if match(n) {
			out = append(out, n)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(a.Tree().Inspect())
	return out
}

// Text returns the box of the paragraph whose text is exactly s; the test
// fails when there is none.
func (a *App) Text(s string) core.InspectNode {
	a.T.Helper()
	n, ok := a.Find(func(n core.InspectNode) bool { return n.Text == s })
	if !ok {
		a.T.Fatalf("no text %q on screen", s)
	}
	return n
}

// HasText reports whether a paragraph has exactly the text s.
func (a *App) HasText(s string) bool {
	_, ok := a.Find(func(n core.InspectNode) bool { return n.Text == s })
	return ok
}

// Button returns the box of the button that contains the text label; the
// test fails when there is none.
func (a *App) Button(label string) core.InspectNode {
	a.T.Helper()
	n, ok := a.Find(func(n core.InspectNode) bool {
		if n.Kind != "button" {
			return false
		}
		_, has := find(n, func(c core.InspectNode) bool { return c.Text == label })
		return has
	})
	if !ok {
		a.T.Fatalf("no button %q on screen", label)
	}
	return n
}

// ClickButton clicks the center of the button that contains the text label.
func (a *App) ClickButton(label string) {
	a.T.Helper()
	c := a.Button(label).Rect.Center()
	a.Click(c.X, c.Y)
}

// Contains reports whether the box or one of its descendants is a
// paragraph with exactly the text s.
func Contains(n core.InspectNode, s string) bool {
	_, ok := find(n, func(c core.InspectNode) bool { return c.Text == s })
	return ok
}

// Classes returns the class names of a box.
func Classes(n core.InspectNode) []string { return strings.Fields(n.Style.Classes) }

// HasClass reports whether the box has the class name.
func HasClass(n core.InspectNode, name string) bool {
	for _, c := range Classes(n) {
		if c == name {
			return true
		}
	}
	return false
}

// ClickText clicks the center of the paragraph whose text is exactly s.
func (a *App) ClickText(s string) {
	a.T.Helper()
	c := a.Text(s).Rect.Center()
	a.Click(c.X, c.Y)
}

// WithClass returns the boxes whose classes include every one of the given
// class names.
func (a *App) WithClass(names ...string) []core.InspectNode {
	return a.All(func(n core.InspectNode) bool {
		have := strings.Fields(n.Style.Classes)
	next:
		for _, w := range names {
			for _, h := range have {
				if h == w {
					continue next
				}
			}
			return false
		}
		return len(have) > 0
	})
}

// Image returns the window's content.
func (a *App) Image() *image.RGBA { return a.Window.Frame() }

// Golden renders root headlessly at the given size and at scales 1 and 2
// and compares the images with testdata/<name>_x1.png and <name>_x2.png,
// bit by bit (TELA_UPDATE=1 writes them). It also fails on dev messages.
func Golden(t *testing.T, name string, root func() tela.Node, w, h float32) {
	t.Helper()
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var logs []string
			tree := core.NewTree(core.Config{Dev: true, Log: func(m string) { logs = append(logs, m) }})
			tree.SetViewport(gfx.Size{W: w, H: h})
			tree.SetScale(scale)
			tree.Mount(core.Component(root))
			dl, _ := tree.Frame()
			golden.Image(t, fmt.Sprintf("%s_x%g", name, scale), tree.Rasterize(dl, scale))
			for _, m := range logs {
				t.Errorf("dev message: %s", m)
			}
		})
	}
}

// AriaGolden mounts root headlessly at the given size and compares its
// aria snapshot (what `tela inspect --aria` prints, with its final
// newline) with testdata/<name>.aria.yml (TELA_UPDATE=1 writes it). It
// also fails on dev messages.
func AriaGolden(t *testing.T, name string, root func() tela.Node, w, h float32) {
	t.Helper()
	var logs []string
	tree := core.NewTree(core.Config{Dev: true, Log: func(m string) { logs = append(logs, m) }})
	tree.SetViewport(gfx.Size{W: w, H: h})
	tree.Mount(core.Component(root))
	tree.Frame()
	golden.Text(t, name+".aria.yml", tree.AriaSnapshot()+"\n")
	for _, m := range logs {
		t.Errorf("dev message: %s", m)
	}
}
