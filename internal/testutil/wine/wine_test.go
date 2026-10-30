package wine

import (
	"runtime"
	"testing"
)

func TestDetected(t *testing.T) {
	if runtime.GOOS != "windows" && Detected() {
		t.Errorf("Detected() = true on %s", runtime.GOOS)
	}
}

// TestSkipRunsWithoutWine checks that Skip does nothing where Wine is not
// detected, so a test that calls it still runs on a real system.
func TestSkipRunsWithoutWine(t *testing.T) {
	if Detected() {
		t.Skip("under Wine")
	}
	Skip(t, "none")
	if t.Skipped() {
		t.Error("Skip skipped a test outside Wine")
	}
}
