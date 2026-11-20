package oauth

import (
	"testing"
	"time"
)

// SetNow replaces the clock of the expiry arithmetic for the test.
func SetNow(t *testing.T, f func() time.Time) {
	t.Helper()
	old := now
	now = f
	t.Cleanup(func() { now = old })
}

// SetSlowDownStep replaces the 5 s a slow_down adds to the polling
// interval, for the test.
func SetSlowDownStep(t *testing.T, d time.Duration) {
	t.Helper()
	old := slowDownStep
	slowDownStep = d
	t.Cleanup(func() { slowDownStep = old })
}
