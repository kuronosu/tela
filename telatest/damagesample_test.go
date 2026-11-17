package telatest

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// fatalTB records the first Fatalf instead of failing the test, and stops
// the goroutine that called it, like the real one.
type fatalTB struct {
	testing.TB
	mu       sync.Mutex
	fatal    string
	cleanups []func()
}

func (f *fatalTB) Helper()           {}
func (f *fatalTB) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }
func (f *fatalTB) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.fatal = fmt.Sprintf(format, args...)
	f.mu.Unlock()
	runtime.Goexit()
}

// The sampled damage check (internal/testutil/damagesample, -short)
// compares the last frame when the app closes, so what a frame it skipped
// left wrong in the window is still found.
func TestSampledDamageChecksTheLastFrame(t *testing.T) {
	tb := &fatalTB{TB: t}
	a := MountFunc(tb, func() tela.Node {
		n, setN := tela.UseState(0)
		return h.Button(h.Class("m-4 p-2 bg-sky-500"), h.OnClick(func() { setN.Set(n + 1) }), h.Textf("Count %d", n))
	}, Size(200, 120), CheckDamage())
	a.damageEvery, a.sh.damage = 1000, a // no frame of the test is compared
	a.Click(ByRole("button", "Count 0"))
	if !a.damageOwed {
		t.Fatal("the frame of the click was compared")
	}
	// A frame whose damage was wrong: the window holds other pixels than
	// a full rendering, and the tree has nothing left to draw.
	fb := a.win.Frame()
	fb.Pix[0] ^= 0xff
	if err := a.win.Present(fb, nil); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Close()
	}()
	<-done
	if !strings.Contains(tb.fatal, "damage check: pixel (0, 0)") {
		t.Fatalf("closing the app reported %q, want the pixel the last frame got wrong", tb.fatal)
	}
	for _, f := range tb.cleanups {
		f()
	}
}
