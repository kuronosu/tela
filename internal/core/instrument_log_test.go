package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// An entry logged on another goroutine while the tree is idle (no frame
// comes) is told to insLogAdded on that goroutine, so a tela dev child
// pushes it without waiting for a frame (devrun_tools.go).
func TestInstrumentLogAddedWhileIdle(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Warn: func(string) {}, Log: func(string) {}})
	tr.SetViewport(gfx.Size{W: 200, H: 100})
	tr.Mount(div(Text("idle")))
	tr.Frame()
	tr.SetInstrument(true)
	t.Cleanup(func() { tr.SetInstrument(false) })
	tr.Frame()

	got := make(chan *Tree, 4)
	f := func(t *Tree) {
		if t == tr {
			got <- t
		}
	}
	insLogAdded.Store(&f)
	t.Cleanup(func() { insLogAdded.Store(nil) })

	go tr.logf("tela: an image failed to load")
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("an entry logged while idle was not told")
	}
	if es := tr.Logs(0); len(es) != 1 || es[0].Msg != "tela: an image failed to load" {
		t.Errorf("log %+v", es)
	}
	go tr.warnf("tela: a warning between frames")
	select {
	case <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("a warning logged while idle was not told")
	}
}
