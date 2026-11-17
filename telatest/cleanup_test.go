package telatest

import (
	"runtime"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// The testing package keeps the cleanups of a parallel test until the
// whole run ends: the cleanup Mount registers must not keep the App, and
// with it the window's framebuffers, alive once it has run.
func TestMountCleanupForgetsTheApp(t *testing.T) {
	tb := &fatalTB{TB: t}
	defer runtime.KeepAlive(tb) // testing keeps the test, and its cleanups, alive
	collected := make(chan struct{})
	func() {
		a := MountFunc(tb, func() tela.Node { return h.Div(h.Class("size-20 bg-sky-500")) }, Size(200, 120))
		runtime.AddCleanup(a, func(ch chan struct{}) { close(ch) }, collected)
	}()
	for i := len(tb.cleanups) - 1; i >= 0; i-- {
		tb.cleanups[i]() // as testing runs them; the slice still holds them after
	}
	for range 50 {
		runtime.GC()
		select {
		case <-collected:
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("the App is still reachable after its cleanups ran")
}
