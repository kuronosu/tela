//go:build windows

package win32

import (
	"os"
	"strings"
)

// Experimental switches for diagnosing why the Windows 11 snap layouts
// flyout of a frameless window may not appear while the window is in its
// normal state (it does while maximized). They are read once from
// TELA_WIN32_SNAP (a comma-separated list) and change nothing by default;
// they go away once the cause is known.
//
//   - noextend: no DwmExtendFrameIntoClientArea (no DWM frame margin).
//   - ncstrip: keep a one-pixel non-client strip at the bottom of the
//     window in its normal state, so that it has a non-client area as a
//     maximized one does (its offscreen resize ring); DWM draws the strip
//     (a frame margin of one pixel).
var snapVariant = os.Getenv("TELA_WIN32_SNAP")

func snapHas(v string) bool {
	for _, s := range strings.Split(snapVariant, ",") {
		if strings.TrimSpace(s) == v {
			return true
		}
	}
	return false
}
