package main

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/devtools"
	"github.com/kuronosu/tela/internal/core"
)

// TestDevtoolsEndToEnd attaches the DevTools' model, over the real client
// and adapter, to a real tela dev of testdata/devapp: the tree, the
// details with why a component rendered, a recording, the pick tool and a
// reload that keeps the selection by path.
func TestDevtoolsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	e := startE2E(t, func(c *supervisorConfig) { c.tools, c.toolsDir = true, dir })
	e.expect(0, "devapp: counter", "devapp: counter n=0 pkg=0")
	e.until("the first frame", func() bool { return e.window().Frame() != nil })
	s, err := findDevSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	// One client feeds the model; another drives the app's input.
	mc, err := dialTools(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	m := devtools.NewModel(newDevtoolsSource(mc))
	defer m.Close()
	ic, err := dialTools(s.Addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ic.Close()
	ctx := context.Background()
	waitBuild(t, ic, 1)

	m.Start()
	until := func(what string, cond func(v devtools.View) bool) devtools.View {
		t.Helper()
		deadline := time.Now().Add(devWait)
		for {
			v := m.View()
			if cond(v) {
				return v
			}
			if time.Now().After(deadline) {
				t.Fatalf("timeout waiting for %s; view: conn %d, err %q, %d elements, selected %d", what, v.Conn, v.Err, len(v.Els), v.Selected)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	find := func(v devtools.View, ok func(core.ElementInfo) bool) core.ElementInfo {
		t.Helper()
		i := slices.IndexFunc(v.Els, ok)
		if i < 0 {
			t.Fatalf("no such element among %d", len(v.Els))
		}
		return v.Els[i]
	}
	v := until("the tree", func(v devtools.View) bool {
		return v.Conn == devtools.Connected && len(v.Els) > 0 && len(v.Windows) == 1
	})
	if v.Info.Pkg == "" || v.Windows[0].Title != "devapp" {
		t.Errorf("info %+v, windows %+v", v.Info, v.Windows)
	}
	counter := find(v, func(el core.ElementInfo) bool { return el.Kind == "main.Counter" && el.Component })
	label := find(v, func(el core.ElementInfo) bool { return el.Kind == "text" && el.Text == "Increment" })
	button := v.Els[v.Index[label.Parent]]

	// Selecting a component fetches its details and layout.
	m.Select(counter.ID, true)
	v = until("the counter's details", func(v devtools.View) bool { return v.Detail != nil && v.Detail.ID == counter.ID })
	if len(v.Detail.Hooks) == 0 || v.Detail.Hooks[0].Kind != "UseState" || v.Detail.Hooks[0].Site.IsZero() {
		t.Errorf("counter details: %+v", v.Detail)
	}
	// A component has no box of its own: the layout is the button's.
	m.Select(button.ID, true)
	v = until("the button's layout", func(v devtools.View) bool { return v.Box != nil && v.Box.Element == button.ID })
	if v.Box.Rect.Height() != 40 || v.Box.Rect.Width() != 128 {
		t.Errorf("the button's box is %v, devapp says 128x40", v.Box.Rect)
	}
	m.Select(counter.ID, true)
	until("the counter's details again", func(v devtools.View) bool { return v.Detail != nil && v.Detail.ID == counter.ID })

	// A click in the app rebuilds the counter: the model learns why, live.
	if err := ic.Click(ctx, v.Window, (button.Bounds.X0+button.Bounds.X1)/2, (button.Bounds.Y0+button.Bounds.Y1)/2); err != nil {
		t.Fatal(err)
	}
	e.expect(0, "devapp: counter", "devapp: counter n=1 pkg=1")
	v = until("the counter's new build", func(v devtools.View) bool {
		d := v.Detail
		return d != nil && len(d.Recent) > 0 && d.Recent[len(d.Recent)-1].Reasons[0].Cause == core.CauseState
	})
	if r := v.Detail.Recent[len(v.Detail.Recent)-1].Reasons[0]; r.HookKind != "UseState" {
		t.Errorf("the reason of the last build: %+v", r)
	}

	// A recording sees the frames of the next click.
	m.SetRecording(true)
	if err := ic.Click(ctx, v.Window, (button.Bounds.X0+button.Bounds.X1)/2, (button.Bounds.Y0+button.Bounds.Y1)/2); err != nil {
		t.Fatal(err)
	}
	v = until("a recorded frame that built the counter", func(v devtools.View) bool {
		return slices.ContainsFunc(v.Frames, func(f *core.FrameProfile) bool { return slices.Contains(f.BuiltIDs, counter.ID) })
	})
	m.SelectFrame(v.Frames[slices.IndexFunc(v.Frames, func(f *core.FrameProfile) bool { return slices.Contains(f.BuiltIDs, counter.ID) })].Frame)
	until("the elements of the frame", func(v devtools.View) bool {
		return slices.ContainsFunc(v.FrameEls, func(el core.ElementInfo) bool { return el.ID == counter.ID })
	})
	m.SetRecording(false)

	// The pick tool: a click in the app selects what is under it.
	m.TogglePick()
	until("pick mode", func(v devtools.View) bool { return v.Picking })
	waitPicking := time.Now().Add(devWait)
	for { // the call to the session is asynchronous: click once it is on
		if time.Now().After(waitPicking) {
			t.Fatal("pick mode never reached the app")
		}
		if err := ic.Click(ctx, v.Window, 60, 148); err != nil {
			t.Fatal(err)
		}
		v = m.View()
		if !v.Picking {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	v = until("the picked element", func(v devtools.View) bool {
		return v.Detail != nil && v.Detail.Kind == "button" && v.Selected != counter.ID
	})
	if v.Selected == 0 {
		t.Fatalf("nothing selected after the pick")
	}

	// A reload: the selection moves to the same path in the new build.
	m.Select(counter.ID, true)
	until("the counter again", func(v devtools.View) bool { return v.Detail != nil && v.Detail.ID == counter.ID })
	_, mark := e.edit(func(s string) string { return strings.Replace(s, `Text("Version A")`, `Text("Version B")`, 1) })
	e.expect(mark, "devapp: counter", "devapp: counter n=2 pkg=2")
	v = until("the reloaded tree", func(v devtools.View) bool {
		return v.Build >= 2 && slices.ContainsFunc(v.Els, func(el core.ElementInfo) bool { return el.Kind == "text" && el.Text == "Version B" }) && v.Detail != nil
	})
	if i, ok := v.Index[v.Selected]; !ok || v.Els[i].Kind != "main.Counter" {
		t.Errorf("after the reload the selection is %d (%v), want main.Counter", v.Selected, ok)
	}
}
