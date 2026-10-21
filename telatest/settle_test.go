package telatest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// TestSettleRunsSlowFiniteAnimations: whether an app goes quiet is decided
// in app time, so a finite animation whose frames take longer in real time
// than the timeout (a starved machine; here every frame sleeps) still runs
// to its end before Mount returns, and is not reported as endless. Before,
// a real-time limit gave up after the timeout and the app was captured
// mid-animation, depending on the machine's speed.
func TestSettleRunsSlowFiniteAnimations(t *testing.T) {
	var v *tela.Value[float32]
	frames := 0
	f := mustPass(t, func(tb testing.TB) {
		telatest.Mount(tb, tela.C(func() tela.Node {
			v = tela.UseTween(float32(1), tela.Initial(float32(0)), tela.Duration(300*time.Millisecond))
			return Div(Class("size-10"), Style(tela.TranslateFn(func() (float32, float32) {
				frames++
				time.Sleep(40 * time.Millisecond) // a slow frame
				return v.Get() * 10, 0
			})))
		}), telatest.Size(100, 100), telatest.Timeout(400*time.Millisecond))
		// 300 ms of app time is 18 frames at 60 Hz, over 700 ms of real
		// time here: longer than the 400 ms timeout.
		if got := v.Get(); got != 1 {
			tb.Errorf("after Mount the tween is at %v after %d frames, want 1 (it ran to its end)", got, frames)
		}
	})
	for _, l := range f.logs {
		if strings.Contains(l, "still requesting frames") || strings.Contains(l, "without a frame to run") {
			t.Errorf("a finite animation was reported as endless: %s", l)
		}
	}
}
