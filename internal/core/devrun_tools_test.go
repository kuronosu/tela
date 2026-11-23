//go:build teladev

package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform/remote"
)

// A flush drops the entries of the trees whose window closed: devTools
// keeps the last frame and log entry pushed per open tree only.
func TestDevToolsFlushDropsClosedTrees(t *testing.T) {
	saved := devTools
	t.Cleanup(func() { devTools = saved })
	open := newTestTree(t, 100, 100, div(Text("open")), nil)
	closed := newTestTree(t, 100, 100, div(Text("closed")), nil)
	h := &Host{views: []*winView{{tree: open}}}
	devTools.h, devTools.rb, devTools.on = h, nil, true
	devTools.frames = map[*Tree]int{open: 1, closed: 2}
	devTools.logs = map[*Tree]uint64{open: 1, closed: 2}
	devToolsFlush(h)
	if _, ok := devTools.frames[closed]; ok || devTools.frames[open] != 1 {
		t.Errorf("frames %v, want the open tree's only", devTools.frames)
	}
	if _, ok := devTools.logs[closed]; ok || devTools.logs[open] != 1 {
		t.Errorf("logs %v, want the open tree's only", devTools.logs)
	}
}

// An entry logged on another goroutine while the app is idle arms the
// push from the UI thread: a post reaches the main tree, without a frame.
func TestDevToolsLoggedWhileIdleArms(t *testing.T) {
	saved := devTools
	t.Cleanup(func() { devTools = saved })
	tr := newTestTree(t, 100, 100, div(Text("idle")), nil)
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	h := &Host{tree: tr, views: []*winView{{tree: tr}}}
	woke := make(chan struct{}, 8)
	tr.SetWake(func() { woke <- struct{}{} })
	devTools.h, devTools.on, devTools.armed = h, true, false
	// A push was just made: the flush the post arms waits devloop.ToolsBatch
	// instead of landing in the same ApplyUpdates (with a zero last push its
	// timer fires at once, and on a slow machine the flush ran before the
	// test disarmed it, on maps this test does not set up).
	devTools.last = time.Now()
	devTools.rb = new(remote.Backend) // only compared with nil before the flush
	logged := devToolsLogged(h)
	insLogAdded.Store(&logged)
	t.Cleanup(func() { insLogAdded.Store(nil) })

	go tr.logf("tela: logged while idle")
	select {
	case <-woke:
	case <-time.After(5 * time.Second):
		t.Fatal("the UI thread was not woken by an entry logged while idle")
	}
	tr.ApplyUpdates() // the post: it arms the flush
	if !devTools.armed {
		t.Error("the push of the entry is not armed")
	}
	devTools.armed = false // the flush would write to the connection
}

// HasElement tells a mounted element from an unknown one, and is false
// while the instrumentation is off.
func TestTreeHasElement(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}})
	tr.SetViewport(gfx.Size{W: 200, H: 100})
	tr.Mount(div(Key("a"), Text("a")))
	tr.Frame()
	if tr.HasElement(1) {
		t.Error("HasElement with the instrumentation off")
	}
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.Frame()
	id := idOfKey(t, tr, "a")
	if !tr.HasElement(id) || tr.HasElement(id+1000) {
		t.Errorf("HasElement(%d) %v, HasElement(%d) %v", id, tr.HasElement(id), id+1000, tr.HasElement(id+1000))
	}
}
