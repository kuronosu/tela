//go:build darwin

package cocoa

import (
	"time"

	"github.com/kuronosu/tela/internal/darwin"
)

// fsel holds the selectors of the refresh rate (both macOS 12).
var fsel = struct{ minimumRefreshInterval, maximumFramesPerSecond darwin.SEL }{
	minimumRefreshInterval: darwin.Sel("minimumRefreshInterval"),
	maximumFramesPerSecond: darwin.Sel("maximumFramesPerSecond"),
}

// refresh is the loop's Refresh hook: the refresh interval of the window's
// screen, the one that shows most of it (-[NSWindow screen]; nil, and so
// unknown, off every screen). It is asked at each frame, so no
// notification has to be followed for a window that moves or a screen
// whose mode changes.
func (w *Window) refresh() time.Duration {
	screen := darwin.Send(w.ns, lsel.screen)
	if screen == 0 {
		return 0
	}
	return refreshFrom(darwin.SendFloat(screen, fsel.minimumRefreshInterval), darwin.SendInt(screen, fsel.maximumFramesPerSecond))
}
