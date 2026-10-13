package win32

import (
	"image"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// baseDPI is the DPI of a scale of 1 (USER_DEFAULT_SCREEN_DPI).
const baseDPI = 96

// maxWindowSide bounds a window's physical client side. Win32 coordinates in
// messages are 16-bit signed, and no display is wider.
const maxWindowSide = 32767

// scaleFromDPI returns the display scale of a DPI: dpi / 96. A zero DPI (a
// failed query) is a scale of 1.
func scaleFromDPI(dpi uint32) float64 {
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / baseDPI
}

// clampPhysical keeps a physical client size inside what a window can have:
// at least 1x1 (CreateWindowExW would accept 0, but the platform contract
// has no empty window besides a minimized one) and at most maxWindowSide.
func clampPhysical(p image.Point) image.Point {
	p.X = min(max(p.X, 1), maxWindowSide)
	p.Y = min(max(p.Y, 1), maxWindowSide)
	return p
}

// metricsFor returns the window metrics for a physical client size at dpi:
// the logical size is physical / scale (platform.LogicalSize), which the
// sizing rule maps back to the same physical size.
func metricsFor(client image.Point, dpi uint32) platform.Metrics {
	scale := scaleFromDPI(dpi)
	return platform.Metrics{Size: platform.LogicalSize(client, scale), Scale: scale}
}

// logicalPoint converts client coordinates in physical pixels to logical
// pixels.
func logicalPoint(x, y int, scale float64) platform.Point {
	return platform.Point{X: float32(float64(x) / scale), Y: float32(float64(y) / scale)}
}

// refreshInterval returns the frame interval of a display refresh rate in
// hertz, as reported by DEVMODE.dmDisplayFrequency. Zero and one mean "the
// hardware default" in that field, and absurd values come from broken
// drivers; all of them give the 60 Hz default.
func refreshInterval(hz uint32) time.Duration {
	if hz < 24 || hz > 1000 {
		hz = 60
	}
	return time.Second / time.Duration(hz)
}
