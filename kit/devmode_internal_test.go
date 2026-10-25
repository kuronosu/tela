package kit

import (
	"testing"

	"github.com/kuronosu/tela/internal/core"
)

// devMode is asked on the way of every build of a table and a few other
// components: it must be a lookup, not a panic provoked and recovered.
func TestDevModeCostsNoPanic(t *testing.T) {
	if got, want := devMode(), core.DevNow(); got != want {
		t.Errorf("devMode() = %v, core.DevNow() = %v", got, want)
	}
	if n := testing.AllocsPerRun(100, func() { devMode() }); n != 0 {
		t.Errorf("devMode allocates %v times per call, want 0 (it provokes a panic)", n)
	}
}
