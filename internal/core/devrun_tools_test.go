//go:build teladev

package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
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
