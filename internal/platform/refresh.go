package platform

import (
	"math"
	"time"
)

// Refresh rates outside this range are taken as unknown: what drivers,
// virtual displays and broken modes report, not displays.
const (
	minRefreshRate = 1     // Hz
	maxRefreshRate = 10000 // Hz
)

// RefreshInterval returns the refresh interval of a display that refreshes
// hz times a second, as [FrameEvent].Refresh reports it: rounded to the
// nanosecond, or 0 (unknown) for a rate no display has (zero, negative,
// NaN, below 1 Hz or above 10 kHz).
func RefreshInterval(hz float64) time.Duration {
	if !(hz >= minRefreshRate && hz <= maxRefreshRate) {
		return 0
	}
	return time.Duration(math.Round(float64(time.Second) / hz))
}

// CheckFrameEvent returns what is wrong with e, or "": Time and Refresh are
// not negative, and a known Refresh is an interval RefreshInterval returns.
func CheckFrameEvent(e FrameEvent) string {
	switch {
	case e.Time < 0:
		return "negative Time " + e.Time.String()
	case e.Refresh != 0 && RefreshInterval(float64(time.Second)/float64(e.Refresh)) == 0:
		return "implausible Refresh " + e.Refresh.String()
	}
	return ""
}
