package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

// The attached server (mcp_attach.go) against a fake tools socket: a
// window with a counter button and a text field, each injected input
// changing it, and a "reload" that numbers the elements again.

// fakeApp is the fake connection: attachConn plus the app behind it.
type fakeApp struct {
	mu     sync.Mutex
	calls  []string
	pid    int
	build  int
	wins   []toolsWindow
	count  int
	field  string
	sel    bool
	focus  bool
	frame  int
	frames []core.FrameProfile
	logs   []toolsLogEntry
	// base moves the node and element IDs: a reload changes it.
	base uint64
	// disabled marks the button [disabled].
	disabled bool
	// noNodes answers semantics without node IDs, as an older Tela would.
	noNodes bool
	// cover is a dialog over the window, which hit tests find first.
	cover *gfx.Rect
	// scrolls is how many reveal calls still answer that a scroll is
	// under way; hidden makes every element clipped whole; noReveal
	// answers reveal and hit as an older Tela does.
	scrolls  int
	hidden   bool
	noReveal bool
	// shape, when set, is the one point of the button that reaches it (a
	// rounded corner cuts the rest), which reveal reports as its action
	// point unless noPoint.
	shape   *gfx.Point
	noPoint bool
	// animating draws a frame before every Profiles answer, as an app
	// whose animation never ends has drawn one between any two polls.
	animating bool
	// cwd is tela dev's working directory, which info reports; with it
	// the component's sites are absolute paths under it. dir and preview
	// are its package's directory and its --preview.
	cwd, dir, preview string
	// closed is Done's channel.
	closed chan struct{}
}

var (
	fakeButton = gfx.Rect{X0: 10, Y0: 50, X1: 110, Y1: 90}
	fakeField  = gfx.Rect{X0: 10, Y0: 170, X1: 210, Y1: 210}
	// fakeText is the "Count: N" text, laid out in a paragraph with no box
	// of its own.
	fakeText = gfx.Rect{X0: 10, Y0: 110, X1: 110, Y1: 130}
)

func newFakeApp() *fakeApp {
	return &fakeApp{pid: 100, build: 1, closed: make(chan struct{}),
		wins: []toolsWindow{{ID: 1, Title: "devapp", Width: 480, Height: 400, Scale: 2, Focused: true, Visible: true, Active: true}}}
}

func (f *fakeApp) note(format string, args ...any) {
	f.calls = append(f.calls, fmt.Sprintf(format, args...))
}

// calledWith returns the calls so far, one per line.
func (f *fakeApp) called() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, "\n")
}

func (f *fakeApp) drew() {
	f.frame++
	f.frames = append(f.frames, core.FrameProfile{Frame: f.frame, Duration: time.Millisecond, Built: 1, Styled: 2, LaidOut: 3, Damage: 400})
}

func (f *fakeApp) reload() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.build++
	f.base += 100
	f.frame, f.frames = 0, nil
}

func (f *fakeApp) Info(context.Context) (toolsInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return toolsInfo{Pkg: "./devapp", Dir: f.dir, Cwd: f.cwd, Preview: f.preview, PID: f.pid, Build: f.build, Protocol: 1}, nil
}

func (f *fakeApp) Windows(context.Context) ([]toolsWindow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]toolsWindow(nil), f.wins...), nil
}

func (f *fakeApp) Semantics(_ context.Context, win uint32) (toolsSemantics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	btn := `- button "Increment"`
	if f.disabled {
		btn += " [disabled]"
	}
	sem := toolsSemantics{
		YAML:     btn + "\n- text: " + fmt.Sprintf("Count: %d", f.count) + "\n- textbox \"name\": " + f.field,
		Nodes:    []uint64{f.base + 1, 0, f.base + 2},
		Elements: []core.ElementID{core.ElementID(f.base + 11), core.ElementID(f.base + 14), core.ElementID(f.base + 12)},
		Srcs:     []string{"main.go:10", "", "main.go:12"},
	}
	if f.cover != nil {
		sem.YAML += "\n- dialog \"Confirm\""
		sem.Nodes = append(sem.Nodes, f.base+3)
		sem.Elements = append(sem.Elements, core.ElementID(f.base+13))
		sem.Srcs = append(sem.Srcs, "main.go:20")
	}
	if f.noNodes {
		sem.Nodes, sem.Elements, sem.Srcs = nil, nil, nil
	}
	return sem, nil
}

func (f *fakeApp) Hit(_ context.Context, win uint32, x, y float32, target core.ElementID) (toolsHit, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.noReveal {
		return toolsHit{}, &toolsCallError{rpcMethodNotFound, "the app does not answer this DevTools request"}
	}
	in := func(r gfx.Rect) bool { return x >= r.X0 && x < r.X1 && y >= r.Y0 && y < r.Y1 }
	var h toolsHit
	switch {
	case f.cover != nil && in(*f.cover):
		h = toolsHit{ID: core.ElementID(f.base + 13), Node: f.base + 3}
	case in(fakeButton) && f.shape != nil && (x != f.shape.X || y != f.shape.Y):
		h = toolsHit{ID: core.ElementID(f.base + 9)} // the rounded box around it
	case in(fakeButton):
		h = toolsHit{ID: core.ElementID(f.base + 11), Node: f.base + 1}
	case in(fakeText):
		h = toolsHit{ID: core.ElementID(f.base + 14)}
	case in(fakeField):
		h = toolsHit{ID: core.ElementID(f.base + 12), Node: f.base + 2}
	}
	h.Within = h.ID != 0 && h.ID == target
	return h, nil
}

func (f *fakeApp) Reveal(_ context.Context, win uint32, id core.ElementID) (toolsReveal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("reveal %d %d", win, id)
	if f.noReveal {
		return toolsReveal{}, &toolsCallError{rpcMethodNotFound, "the app does not answer this DevTools request"}
	}
	var r gfx.Rect
	var pt *gfx.Point
	switch uint64(id) - f.base {
	case 11:
		r = fakeButton
		if !f.noPoint {
			pt = f.shape
		}
	case 12:
		r = fakeField
	case 14:
		r = fakeText
	default:
		return toolsReveal{}, errors.New("no such element")
	}
	if f.scrolls > 0 {
		f.scrolls--
		f.drew()
		return toolsReveal{Visible: &r, Moved: true}, nil
	}
	if f.hidden {
		return toolsReveal{}, nil
	}
	return toolsReveal{Visible: &r, Point: pt}, nil
}

func (f *fakeApp) Element(_ context.Context, win uint32, id core.ElementID) (core.ElementInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch uint64(id) - f.base {
	case 11:
		r := fakeButton
		return core.ElementInfo{ID: id, Kind: "button", Bounds: &r}, nil
	case 12:
		r := fakeField
		return core.ElementInfo{ID: id, Kind: "input", Bounds: &r}, nil
	case 14:
		return core.ElementInfo{ID: id, Kind: "text", Text: fmt.Sprintf("Count: %d", f.count)}, nil
	}
	return core.ElementInfo{}, errors.New("no such element")
}

func (f *fakeApp) Elements(_ context.Context, win uint32, detail bool) ([]core.ElementInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	counter := core.ElementInfo{ID: core.ElementID(f.base + 10), Kind: "main.Counter", Component: true, Builds: 3,
		Props:  []core.PropInfo{{Name: "Step", Value: "1"}},
		Hooks:  []core.HookInfo{{Index: 0, Kind: "UseState", Value: fmt.Sprint(f.count)}},
		Recent: []core.BuildRecord{{Frame: 2, Reasons: []core.BuildReason{{Cause: core.CauseState, Hook: 0, HookKind: "UseState"}}}}}
	if f.cwd != "" {
		counter.Site = core.SourceSite{File: f.cwd + "/ui/counter.go", Line: 7}
		counter.Hooks[0].Site = core.SourceSite{File: f.cwd + "/ui/counter.go", Line: 8}
	}
	r := fakeButton
	return []core.ElementInfo{counter, {ID: core.ElementID(f.base + 11), Parent: counter.ID, Kind: "button", Bounds: &r}}, nil
}

func (f *fakeApp) Inspect(_ context.Context, win uint32, id core.ElementID) (core.InspectNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("inspect %d %d", win, id)
	return core.InspectNode{Kind: "button", Element: id, Src: "main.go:10"}, nil
}

func (f *fakeApp) Profiles(_ context.Context, win uint32, since int) ([]core.FrameProfile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.animating {
		f.drew()
	}
	var out []core.FrameProfile
	for _, p := range f.frames {
		if p.Frame > since {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeApp) Logs(_ context.Context, since uint64) ([]toolsLogEntry, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []toolsLogEntry
	for _, e := range f.logs {
		if e.Seq > since {
			out = append(out, e)
		}
	}
	return out, uint64(len(f.logs)), nil
}

func (f *fakeApp) Screenshot(_ context.Context, win uint32) (image.Image, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("screenshot %d", win)
	img := image.NewRGBA(image.Rect(0, 0, 960, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 960; x++ {
			img.Set(x, y, color.RGBA{uint8(x / 4), uint8(y / 4), 90, 255})
		}
	}
	return img, nil
}

// press handles a pointer down and up at x, y.
func (f *fakeApp) press(x, y float32, count int) {
	in := func(r gfx.Rect) bool { return x >= r.X0 && x < r.X1 && y >= r.Y0 && y < r.Y1 }
	f.focus = in(fakeField)
	if in(fakeButton) && !f.disabled {
		f.count += count
		f.drew()
	}
	if f.focus {
		f.drew()
	}
}

func (f *fakeApp) Pointer(_ context.Context, win uint32, action string, x, y float32, button string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("pointer %d %s %g,%g %s", win, action, x, y, button)
	if action == "click" {
		f.press(x, y, 1)
	}
	return nil
}

func (f *fakeApp) Click(ctx context.Context, win uint32, x, y float32) error {
	return f.Clicks(ctx, win, x, y, "", 1)
}

func (f *fakeApp) Clicks(_ context.Context, win uint32, x, y float32, button string, count int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("click %d %g,%g %q x%d", win, x, y, button, count)
	if button != "right" {
		f.press(x, y, count)
	}
	return nil
}

func (f *fakeApp) Scroll(_ context.Context, win uint32, x, y, dx, dy float32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("scroll %d %g,%g %g,%g", win, x, y, dx, dy)
	return nil
}

func (f *fakeApp) Key(_ context.Context, win uint32, combo, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("key %d %s", win, combo)
	switch combo {
	case "ControlOrMeta+a":
		f.sel = f.focus
	case "Backspace":
		if f.sel {
			f.field, f.sel = "", false
			f.drew()
		}
	}
	return nil
}

func (f *fakeApp) Text(_ context.Context, win uint32, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.note("text %d %q", win, text)
	if f.focus {
		if f.sel {
			f.field, f.sel = "", false
		}
		f.field += text
		f.drew()
	}
	return nil
}

func (f *fakeApp) Done() <-chan struct{} { return f.closed }
func (f *fakeApp) Close() error          { return nil }

var _ attachConn = (*fakeApp)(nil)
var _ attachConn = (*toolsClient)(nil)

// newTestAttach returns a session on app, with a settle fast enough for a
// test.
func newTestAttach(app *fakeApp) *attachSession {
	s := newAttachSession("", func(string, ...any) {})
	s.dial = func(string) (attachConn, error) { return app, nil }
	s.quiet, s.first, s.max, s.poll = 5*time.Millisecond, 20*time.Millisecond, 300*time.Millisecond, time.Millisecond
	return s
}

// tool calls a tool and returns its text and whether it failed.
func (s *attachSession) tool(t *testing.T, name, args string) (string, mcpResult) {
	t.Helper()
	res, ok := s.call(context.Background(), name, json.RawMessage(args))
	if !ok {
		t.Fatalf("no tool %s", name)
	}
	var texts []string
	for _, c := range res.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n"), res
}

func (s *attachSession) mustTool(t *testing.T, name, args string) string {
	t.Helper()
	text, res := s.tool(t, name, args)
	if res.IsError {
		t.Fatalf("%s %s failed: %s", name, args, text)
	}
	return text
}

func wantIn(t *testing.T, what, text string, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(text, sub) {
			t.Errorf("%s lacks %q:\n%s", what, sub, text)
		}
	}
}

func TestAttachWindowsAndSnapshot(t *testing.T) {
	app := newFakeApp()
	app.wins = append(app.wins, toolsWindow{ID: 2, Title: "Settings", Width: 300, Height: 200, Scale: 1, Visible: true, Active: true})
	s := newTestAttach(app)
	text := s.mustTool(t, "windows", `{}`)
	wantIn(t, "windows", text, `window 1 "devapp": 480x400 logical pixels, scale 2, focused, the default`, `window 2 "Settings": 300x200 logical pixels, scale 1`)

	text = s.mustTool(t, "snapshot", `{}`)
	wantIn(t, "snapshot", text, "Window 1", "scale 2", `- button "Increment" [ref=e1] [src=main.go:10]`, "- text: Count: 0", `- textbox "name" [ref=e2] [src=main.go:12]`)
	// Another window has its own refs, numbered on.
	text = s.mustTool(t, "snapshot", `{"window": 2}`)
	wantIn(t, "snapshot of window 2", text, `button "Increment" [ref=e3]`, `textbox "name" [ref=e4]`)
	if text := s.mustTool(t, "snapshot", `{}`); !strings.Contains(text, `[ref=e1]`) {
		t.Errorf("window 1 lost its refs:\n%s", text)
	}
	if text, res := s.tool(t, "snapshot", `{"window": 7}`); !res.IsError || !strings.Contains(text, "no window 7; the app shows 1, 2") {
		t.Errorf("an unknown window: %v %s", res.IsError, text)
	}
	if text, res := s.tool(t, "snapshot", `{"windo": 2}`); !res.IsError || !strings.Contains(text, `unknown argument "windo"`) {
		t.Errorf("a misspelled argument: %v %s", res.IsError, text)
	}
}

func TestAttachClickFillAndSettle(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)

	text := s.mustTool(t, "click", `{"ref": "e1"}`)
	wantIn(t, "click", text, `Clicked button "Increment" [ref=e1].`, "- text: Count: 1", `[ref=e1] [src=main.go:10]`,
		"### Frames\n1 frame(s), numbered 1 to 1 (caused by this call)")
	if got := app.called(); !strings.Contains(got, `click 1 60,70 "" x1`) {
		t.Errorf("the click was injected as:\n%s", got)
	}
	text = s.mustTool(t, "click", `{"role": "button", "name": "Increment", "double": true}`)
	wantIn(t, "double click", text, "Double-clicked", "- text: Count: 3")
	if got := app.called(); !strings.Contains(got, `click 1 60,70 "" x2`) {
		t.Errorf("the double click was injected as:\n%s", got)
	}
	text = s.mustTool(t, "click", `{"x": 60, "y": 70}`)
	wantIn(t, "click at a point", text, "Clicked at (60, 70).", "Count: 4")
	if text, res := s.tool(t, "click", `{"x": 600, "y": 70}`); !res.IsError || !strings.Contains(text, "outside the window") {
		t.Errorf("a point outside: %v %s", res.IsError, text)
	}

	text = s.mustTool(t, "fill", `{"ref": "e2", "text": "ana", "submit": true}`)
	wantIn(t, "fill", text, `Filled textbox "name" [ref=e2] with "ana"`, `- textbox "name" [ref=e2] [src=main.go:12]: ana`)
	got := app.called()
	i := strings.Index(got, "click 1 110,190")
	j := strings.Index(got, "key 1 ControlOrMeta+a")
	k := strings.Index(got, `text 1 "ana"`)
	l := strings.Index(got, "key 1 Enter")
	if i < 0 || !(i < j && j < k && k < l) {
		t.Errorf("fill must click, select all, type, press Enter in that order:\n%s", got)
	}
	s.mustTool(t, "fill", `{"ref": "e2", "text": ""}`)
	if app.field != "" {
		t.Errorf("an empty fill left %q", app.field)
	}

	// The frames the action caused are what perf reports.
	text = s.mustTool(t, "perf", `{"action": true}`)
	wantIn(t, "perf", text, "frame(s)", "the frames the last acting call caused", "built 1, styled 2, laid out 3")

	text = s.mustTool(t, "hover", `{"ref": "e1"}`)
	wantIn(t, "hover", text, "The pointer is over button")
	wantIn(t, "hover call", app.called(), "pointer 1 move 60,70")

	text = s.mustTool(t, "scroll", `{"dy": 120}`)
	wantIn(t, "scroll", text, "Scrolled by (0, 120) over the window (at its center).")
	wantIn(t, "scroll call", app.called(), "scroll 1 240,200 0,120")
	s.mustTool(t, "scroll", `{"ref": "e1", "dx": 5}`)
	wantIn(t, "scroll over an element", app.called(), "scroll 1 60,70 5,0")

	text = s.mustTool(t, "drag", `{"from": {"ref": "e1"}, "dx": 40}`)
	wantIn(t, "drag", text, "Dragged button", "by (40, 0).")
	wantIn(t, "drag calls", app.called(), "pointer 1 down 60,70", "pointer 1 move 100,70", "pointer 1 up 100,70")

	text = s.mustTool(t, "press", `{"key": "Tab"}`)
	wantIn(t, "press", text, "Pressed Tab.")
	text = s.mustTool(t, "press", `{"ref": "e2", "key": "Enter"}`)
	wantIn(t, "press on a field", text, "to focus it and pressed Enter")
	if text, res := s.tool(t, "press", `{"ref": "e1", "key": "Enter"}`); !res.IsError || !strings.Contains(text, "without activating it") {
		t.Errorf("press that would activate a button: %v %s", res.IsError, text)
	}
}

// An element is scrolled into view before the action, and a point of it
// that another element covers is not acted on: another point of its
// visible part is, or the action is refused with what covers it.
func TestAttachActionability(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)

	// A scroll under way: reveal is asked again once its frames are drawn.
	app.scrolls = 1
	s.mustTool(t, "click", `{"ref": "e1"}`)
	if got := app.called(); strings.Count(got, "reveal 1 11") != 2 || !strings.Contains(got, `click 1 60,70 "" x1`) {
		t.Errorf("a click after a scroll into view:\n%s", got)
	}

	// A dialog over the button's center: the click goes to the center of
	// its top-left quarter.
	app.cover = &gfx.Rect{X0: 50, Y0: 0, X1: 300, Y1: 400}
	s.mustTool(t, "snapshot", `{}`)
	s.mustTool(t, "click", `{"ref": "e1"}`)
	if got := app.called(); !strings.Contains(got, `click 1 35,60 "" x1`) {
		t.Errorf("the click on the button's visible part:\n%s", got)
	}
	// The dialog over all of it: refused, naming the dialog.
	app.cover = &gfx.Rect{X0: 0, Y0: 0, X1: 300, Y1: 400}
	before := app.count
	text, res := s.tool(t, "click", `{"ref": "e1"}`)
	if !res.IsError || app.count != before {
		t.Fatalf("a click on a covered button: error %v, count %d (was %d): %s", res.IsError, app.count, before, text)
	}
	wantIn(t, "the covered click", text, `button "Increment" [ref=e1] is covered by dialog "Confirm" [ref=e3] at (60, 70)`)
	if text, res := s.tool(t, "fill", `{"ref": "e2", "text": "x"}`); !res.IsError || !strings.Contains(text, "is covered by dialog") {
		t.Errorf("a fill of a covered field: %v %s", res.IsError, text)
	}
	app.cover = nil

	// Clipped whole.
	app.hidden = true
	if text, res := s.tool(t, "hover", `{"ref": "e1"}`); !res.IsError || !strings.Contains(text, "is not visible even scrolled into view") {
		t.Errorf("a hover of a clipped button: %v %s", res.IsError, text)
	}
	app.hidden = false

	// An app of an older Tela: the center of the bounds, unchecked.
	app.noReveal = true
	s.mustTool(t, "click", `{"ref": "e1"}`)
	if got := app.called(); strings.Count(got, `click 1 60,70 "" x1`) != 2 {
		t.Errorf("the click on an older Tela:\n%s", got)
	}
}

// An animation draws between every two polls of the settle. (A goroutine
// that drew every 2 ms fell behind the 5 ms quiet window under load, and
// the call settled early: 2 of 40 runs beside the dashboard's tests.)
func TestAttachSettleReportsAnAppThatKeepsDrawing(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)
	app.mu.Lock()
	app.animating = true
	app.mu.Unlock()
	start := time.Now()
	text := s.mustTool(t, "hover", `{"x": 5, "y": 5}`)
	if d := time.Since(start); d < s.max {
		t.Errorf("the call returned after %v, before the wait of %v", d, s.max)
	}
	wantIn(t, "hover", text, "still drawing frames")
}

func TestAttachRefs(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)

	if text, res := s.tool(t, "click", `{"ref": "e9"}`); !res.IsError || !strings.Contains(text, `unknown ref "e9"`) {
		t.Errorf("an unknown ref: %v %s", res.IsError, text)
	}
	if text, res := s.tool(t, "click", `{"role": "link"}`); !res.IsError || !strings.Contains(text, "no element matches link") {
		t.Errorf("no match: %v %s", res.IsError, text)
	}
	if text, res := s.tool(t, "click", `{"name": "x"}`); !res.IsError || !strings.Contains(text, "name needs a role") {
		t.Errorf("a name alone: %v %s", res.IsError, text)
	}
	if text, res := s.tool(t, "click", `{"testId": "go"}`); !res.IsError || !strings.Contains(text, "testId is not available while attached") {
		t.Errorf("a test id: %v %s", res.IsError, text)
	}
	if text, res := s.tool(t, "click", `{"text": "n"}`); !res.IsError || !strings.Contains(text, "matches 3 elements") {
		t.Errorf("an ambiguous text: %v %s", res.IsError, text)
	}
	s.mustTool(t, "click", `{"text": "increm", "nth": 0}`)
	app.disabled = true
	s.mustTool(t, "snapshot", `{}`)
	if text, res := s.tool(t, "click", `{"ref": "e1"}`); !res.IsError || !strings.Contains(text, "it is disabled") {
		t.Errorf("a disabled button: %v %s", res.IsError, text)
	}
	if text, res := s.tool(t, "hover", `{"ref": "e1"}`); res.IsError {
		t.Errorf("a disabled button can be hovered: %s", text)
	}

	// A ref given out whose element is not in the window now is gone.
	s.win(1).refs.issued["e77"] = true
	if text, res := s.tool(t, "click", `{"ref": "e77"}`); !res.IsError || !strings.Contains(text, "ref e77 is gone") {
		t.Errorf("a ref that is gone: %v %s", res.IsError, text)
	}
}

func TestAttachReload(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	text := s.mustTool(t, "snapshot", `{}`)
	wantIn(t, "snapshot", text, `button "Increment" [ref=e1]`, `textbox "name" [ref=e2]`)
	s.mustTool(t, "click", `{"ref": "e1"}`)

	// tela dev builds the app again: its elements are numbered anew.
	app.reload()
	text = s.mustTool(t, "click", `{"ref": "e1"}`)
	wantIn(t, "click after the reload", text, "tela dev built the app again (build 2)", `button "Increment" [ref=e1]`, `textbox "name" [ref=e2]`)
	got := app.called()
	if strings.Count(got, `click 1 60,70 "" x1`) != 2 {
		t.Errorf("the ref did not find the button again:\n%s", got)
	}
	// The element IDs of the new build are the ones inspected.
	s.mustTool(t, "inspect", `{"ref": "e1"}`)
	wantIn(t, "inspect call", app.called(), "inspect 1 111")
}

func TestAttachNoNodeIDs(t *testing.T) {
	app := newFakeApp()
	app.noNodes = true
	s := newTestAttach(app)
	text := s.mustTool(t, "snapshot", `{}`)
	if strings.Contains(text, "[ref=") || !strings.Contains(text, `- button "Increment"`) {
		t.Errorf("a snapshot without node IDs:\n%s", text)
	}
	if text, res := s.tool(t, "click", `{"role": "button"}`); !res.IsError || !strings.Contains(text, "no element matches") {
		t.Errorf("a click without node IDs: %v %s", res.IsError, text)
	}
}

func TestAttachScreenshot(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)
	decode := func(res mcpResult) image.Image {
		t.Helper()
		for _, c := range res.Content {
			if c.Type == "image" {
				b, err := base64.StdEncoding.DecodeString(c.Data)
				if err != nil {
					t.Fatal(err)
				}
				img, err := png.Decode(bytes.NewReader(b))
				if err != nil {
					t.Fatal(err)
				}
				return img
			}
		}
		t.Fatal("no image in the result")
		return nil
	}
	text, res := s.tool(t, "screenshot", `{}`)
	wantIn(t, "screenshot", text, "960x800 real pixels", "480x400 logical at scale 2")
	if b := decode(res).Bounds(); b.Dx() != 960 || b.Dy() != 800 {
		t.Errorf("screenshot is %v", b)
	}
	text, res = s.tool(t, "screenshot", `{"ref": "e1"}`)
	// The button is 100x40 logical with an 8 px margin: 116x56 logical, 232x112 real.
	wantIn(t, "element screenshot", text, `button "Increment"`, "100x40 logical pixels", "232x112 real pixels")
	if b := decode(res).Bounds(); b.Dx() != 232 || b.Dy() != 112 {
		t.Errorf("element screenshot is %v", b)
	}
}

func TestAttachInspectStateLogsWait(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)
	text := s.mustTool(t, "inspect", `{"ref": "e1"}`)
	wantIn(t, "inspect", text, `Layout tree of button "Increment"`, `"src": "main.go:10"`)
	text = s.mustTool(t, "state", `{"ref": "e1"}`)
	wantIn(t, "state of a ref", text, "### main.Counter", "props: Step=1", "hook #1 UseState = 0", "built 3 time(s)", "frame 2: UseState #1")
	text = s.mustTool(t, "state", `{"role": "button", "name": "Increment"}`)
	wantIn(t, "state by role and name", text, "### main.Counter", "props: Step=1")
	text = s.mustTool(t, "state", `{}`)
	wantIn(t, "state", text, "1 components with props or hooks", "### main.Counter")

	// What was logged before the client came is not news; the dev
	// warnings after it are in the next result.
	app.mu.Lock()
	app.logs = []toolsLogEntry{{Seq: 1, Source: "stderr", Level: 0, Msg: "old"}}
	app.mu.Unlock()
	s = newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)
	app.mu.Lock()
	app.logs = append(app.logs,
		toolsLogEntry{Seq: 2, Source: "app", Level: 4, Msg: "unknown class \"flx\""},
		toolsLogEntry{Seq: 3, Source: "stderr", Level: 8, Msg: "boom"},
		toolsLogEntry{Seq: 4, Source: "stderr", Level: 0, Msg: "hello"})
	app.mu.Unlock()
	text = s.mustTool(t, "snapshot", `{}`)
	wantIn(t, "snapshot", text, "### Dev warnings", `unknown class "flx"`, "stderr: boom")
	if strings.Contains(text, "old") || strings.Contains(text, "hello") {
		t.Errorf("a result carries logs it should not:\n%s", text)
	}
	text = s.mustTool(t, "logs", `{"all": true}`)
	wantIn(t, "logs", text, "[warn] dev: unknown class", "[error] stderr: boom", "[info] stderr: hello", "[info] stderr: old")
	text = s.mustTool(t, "logs", `{}`)
	wantIn(t, "logs again", text, "No new log entries")

	text = s.mustTool(t, "wait", `{"ms": 5}`)
	wantIn(t, "wait", text, "waited 5 ms of real time")
	app.mu.Lock()
	app.count = 5
	app.mu.Unlock()
	text = s.mustTool(t, "wait", `{"text": "Count: 5"}`)
	wantIn(t, "wait for a text", text, `"Count: 5" is shown`)
	go func() {
		time.Sleep(30 * time.Millisecond)
		app.mu.Lock()
		app.count = 6
		app.mu.Unlock()
	}()
	text = s.mustTool(t, "wait", `{"textGone": "Count: 5"}`)
	wantIn(t, "wait for a text to go", text, `"Count: 5" is gone`, "Count: 6")
	if text, res := s.tool(t, "wait", `{}`); !res.IsError || !strings.Contains(text, "wait needs") {
		t.Errorf("an empty wait: %v %s", res.IsError, text)
	}
}

func TestAttachUnavailableTools(t *testing.T) {
	s := newTestAttach(newFakeApp())
	for name, want := range map[string]string{
		"resize": "does not resize it",
		"open":   "one running app",
		"prefer": "system's own preferences",
	} {
		text, res := s.tool(t, name, `{}`)
		if !res.IsError || !strings.Contains(text, name+" is not available while attached to tela dev") || !strings.Contains(text, want) {
			t.Errorf("%s: %v %s", name, res.IsError, text)
		}
	}
	// Every tool takes a window, and the list is the headless one's.
	var attached, headless []string
	for _, tl := range s.list() {
		attached = append(attached, tl.Name)
		if tl.Name != "windows" && !strings.Contains(string(tl.InputSchema), `"window"`) {
			t.Errorf("%s takes no window", tl.Name)
		}
	}
	for _, d := range mcpToolDefs(nil) {
		if d.tool.Name == "check" || d.tool.Name == "verify" {
			continue // they judge the package's source in a process of their own, not a window
		}
		headless = append(headless, d.tool.Name)
	}
	if strings.Join(attached, " ") != "windows "+strings.Join(headless, " ") {
		t.Errorf("tools %v, want windows then the headless %v", attached, headless)
	}
}

func TestAttachNoSession(t *testing.T) {
	s := newAttachSession("", func(string, ...any) {})
	s.dial = func(string) (attachConn, error) {
		return nil, errors.New("no tela dev session is running; start one with tela dev [pkg]")
	}
	text, res := s.tool(t, "snapshot", `{}`)
	if !res.IsError || !strings.Contains(text, "no tela dev session is running; start one with tela dev") {
		t.Errorf("without a session: %v %s", res.IsError, text)
	}
}

func TestAttachReconnects(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)
	close(app.closed) // the connection ended
	dials := 0
	app2 := newFakeApp()
	app2.pid = 200
	s.dial = func(string) (attachConn, error) { dials++; return app2, nil }
	text := s.mustTool(t, "snapshot", `{}`)
	wantIn(t, "snapshot", text, "The connection to tela dev ended", "another tela dev session", `[ref=e`)
	if dials != 1 {
		t.Errorf("%d dials", dials)
	}
}

func TestParseAttachArgs(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want mcpOptions
		err  string
	}{
		{[]string{"--attach"}, mcpOptions{Attach: true}, ""},
		{[]string{"--attach", "/run/x/tools.sock"}, mcpOptions{Attach: true, Addr: "/run/x/tools.sock"}, ""},
		{[]string{"-v", "--attach=/run/x/tools.sock"}, mcpOptions{Attach: true, Addr: "/run/x/tools.sock", Verbose: true}, ""},
		{[]string{"--attach=false", "./app"}, mcpOptions{Pkg: "./app"}, ""},
		{[]string{"--attach", "--size", "400x300"}, mcpOptions{}, "--size only applies to the headless server"},
		{[]string{"--attach", "a", "b"}, mcpOptions{}, "at most one <addr>"},
		{[]string{"--attach=a", "b"}, mcpOptions{}, "given twice"},
	} {
		got, err := parseMCPArgs(tc.args)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("%v: error %v, want %q", tc.args, err, tc.err)
			}
			continue
		}
		if err != nil || got.Attach != tc.want.Attach || got.Addr != tc.want.Addr || got.Verbose != tc.want.Verbose || (!tc.want.Attach && got.Pkg != tc.want.Pkg) {
			t.Errorf("%v: %+v, %v; want %+v", tc.args, got, err, tc.want)
		}
	}
}

// The gaps of a target (ADR-299's actionability): a point of the button
// that only the app can find (a rounded corner cuts the rest of its
// rectangle), a text of a paragraph with no box of its own, a drag whose
// end is scrolled into view only once the button is down at the start,
// and state's sites relative to the app's working directory.
func TestAttachTargets(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)

	// The center and the quarters of the button miss it: the app's point
	// is clicked.
	app.shape = &gfx.Point{X: 20, Y: 85}
	if text, res := s.tool(t, "click", `{"ref": "e1"}`); res.IsError || !strings.Contains(app.called(), `click 1 20,85 "" x1`) {
		t.Errorf("the click on the app's point of the button: %s\n%s", text, app.called())
	}
	app.noPoint = true // an app of an older Tela: no point, the guesses miss
	if text, res := s.tool(t, "click", `{"ref": "e1"}`); !res.IsError || !strings.Contains(text, "is covered by") {
		t.Errorf("a click without the app's point: %v %s", res.IsError, text)
	}
	app.shape, app.noPoint = nil, false

	// A text with no box of its own, found by its text.
	text, _ := s.tool(t, "click", `{"text": "Count: 1"}`)
	wantIn(t, "click on a text", text, `Clicked the text "Count: 1".`)
	wantIn(t, "click on a text", app.called(), "reveal 1 14", `click 1 60,120 "" x1`)
	if text, res := s.tool(t, "hover", `{"text": "Count: 1", "exact": true}`); res.IsError {
		t.Errorf("hover on a text: %s", text)
	}
	if text, res := s.tool(t, "click", `{"role": "text"}`); !res.IsError || !strings.Contains(text, "no element matches text") {
		t.Errorf("a text line is not an element of role text: %v %s", res.IsError, text)
	}

	// A drag to an element: the button goes down on the start before the
	// end is scrolled into view.
	app.mu.Lock()
	app.calls = nil
	app.mu.Unlock()
	text = s.mustTool(t, "drag", `{"from": {"ref": "e1"}, "to": {"ref": "e2"}}`)
	wantIn(t, "drag to an element", text, `Dragged button "Increment" [ref=e1] to textbox "name" [ref=e2].`)
	got := app.called()
	down, reveal, up := strings.Index(got, "pointer 1 down 60,70"), strings.Index(got, "reveal 1 12"), strings.Index(got, "pointer 1 up 110,190")
	if down < 0 || reveal < down || up < reveal {
		t.Errorf("the drag must press on the start, then reveal the end, then release on it:\n%s", got)
	}

	// state: the sites as the app's working directory shows them.
	app.cwd = "/work/devapp"
	text = s.mustTool(t, "state", `{"ref": "e1"}`)
	wantIn(t, "state", text, "### main.Counter made at ui/counter.go:7", "hook #1 UseState at ui/counter.go:8")
}

// film over the real window takes the frames it presents after an action,
// or as it is; test writes the session's acting calls as a telatest test
// of tela dev's package, which mounts its screen headlessly.
func TestAttachFilmAndTest(t *testing.T) {
	app := newFakeApp()
	s := newTestAttach(app)
	s.mustTool(t, "snapshot", `{}`)

	text, res := s.tool(t, "film", `{"span": 100, "every": 50, "action": {"click": "e1"}}`)
	if res.IsError {
		t.Fatalf("film of a click: %s", text)
	}
	wantIn(t, "film of a click", text, "Filmed 3 frames over", "(after click)", "### The action", `Clicked button "Increment" [ref=e1].`)
	if images := len(res.Content) - 1; images != 1 || app.count != 1 {
		t.Errorf("film of a click: %d images, count %d", images, app.count)
	}
	text = s.mustTool(t, "film", `{"span": 60, "every": 30}`)
	wantIn(t, "film of the window", text, "Filmed 3 frames over", "(the window as it is)")
	if text, res := s.tool(t, "film", `{"span": 60, "apng": "m.png"}`); !res.IsError || !strings.Contains(text, "module of tela dev's package is not known") {
		t.Errorf("an animated PNG without the package's directory: %v %s", res.IsError, text)
	}
	if text, res := s.tool(t, "test", `{"name": "flow"}`); !res.IsError || !strings.Contains(text, "does not say where its package is") {
		t.Errorf("test without the package's directory: %v %s", res.IsError, text)
	}

	skipShort(t)
	dir := mcpAppModule(t)
	screen := `package mcpapp

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

func init() { Previews["Fake"] = tela.Preview{Root: tela.C(fakeScreen)} }

// fakeScreen is the screen of the fake app of the attach tests.
func fakeScreen() tela.Node {
	n, setN := tela.UseState(0)
	name, setName := tela.UseState("")
	return Div(Class("flex flex-col gap-2 p-4"),
		Button(OnClick(func() { setN.Set(n + 1) }), Text("Increment")),
		Textf("Count: %d", n),
		Input(AriaLabel("name"), Value(name), OnChange(func(e tela.ChangeEvent) { setName.Set(e.Value) })))
}
`
	if err := os.WriteFile(filepath.Join(dir, "app", "fake.go"), []byte(screen), 0o644); err != nil {
		t.Fatal(err)
	}
	app.mu.Lock()
	app.dir, app.preview = filepath.Join(dir, "app"), "Fake"
	app.mu.Unlock()
	s.mustTool(t, "fill", `{"ref": "e2", "text": "ana"}`)
	text = s.mustTool(t, "test", `{"name": "attached flow"}`)
	wantIn(t, "test", text, "TestAttachedFlow passes")
	b, err := os.ReadFile(filepath.Join(dir, "app", "attached_flow_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	wantIn(t, "the test file", string(b),
		`app := telatest.Mount(t, Previews["Fake"], telatest.Size(480, 400), telatest.Scale(2))`,
		`app.Click(telatest.ByRole("button", "Increment"))`, `app.ExpectText("Count: 1")`,
		`app.Advance(100 * time.Millisecond)`,
		`app.Fill(telatest.ByRole("textbox", "name"), "ana")`, `app.ExpectValue(telatest.ByRole("textbox", "name"), "ana")`)

	// A reload starts the recording over.
	app.reload()
	if text, res := s.tool(t, "test", `{"name": "attached flow"}`); !res.IsError || !strings.Contains(text, "nothing to write") {
		t.Errorf("test after a reload: %v %s", res.IsError, text)
	}
	// An action on another window: a test of the screen cannot follow.
	app.mu.Lock()
	app.wins = append(app.wins, toolsWindow{ID: 2, Title: "Settings", Width: 300, Height: 200, Scale: 1, Visible: true, Active: true})
	app.mu.Unlock()
	s.mustTool(t, "hover", `{"x": 5, "y": 5, "window": 2}`)
	s.mustTool(t, "hover", `{"x": 5, "y": 5}`)
	if text, res := s.tool(t, "test", `{"name": "attached flow"}`); !res.IsError || !strings.Contains(text, "another window") {
		t.Errorf("test after an action on another window: %v %s", res.IsError, text)
	}
}
