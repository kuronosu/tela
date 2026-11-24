package cocoa

import (
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// refreshFrom returns the refresh interval (FrameEvent.Refresh) of a screen
// from NSScreen's minimumRefreshInterval, in seconds, or when that is not
// one a display has, from its maximumFramesPerSecond, which is in whole
// frames (60 for 59.94 Hz). On a screen with a variable rate both are of
// its highest one. 0 is unknown.
func refreshFrom(minInterval float64, maxFPS int) time.Duration {
	if d := platform.RefreshInterval(1 / minInterval); d != 0 {
		return d
	}
	return platform.RefreshInterval(float64(maxFPS))
}
