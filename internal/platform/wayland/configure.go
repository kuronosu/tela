//go:build linux

package wayland

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
	"github.com/kuronosu/tela/internal/platform/wayland/wire"
)

// toplevelStates is the decoded state array of xdg_toplevel.configure.
type toplevelStates struct {
	maximized, fullscreen, resizing, activated, tiled, suspended bool
}

func decodeStates(raw []byte) toplevelStates {
	var s toplevelStates
	for _, v := range wire.Uint32s(raw) {
		switch protocols.XdgToplevelState(v) {
		case protocols.XdgToplevelStateMaximized:
			s.maximized = true
		case protocols.XdgToplevelStateFullscreen:
			s.fullscreen = true
		case protocols.XdgToplevelStateResizing:
			s.resizing = true
		case protocols.XdgToplevelStateActivated:
			s.activated = true
		case protocols.XdgToplevelStateTiledLeft, protocols.XdgToplevelStateTiledRight,
			protocols.XdgToplevelStateTiledTop, protocols.XdgToplevelStateTiledBottom:
			s.tiled = true
		case protocols.XdgToplevelStateSuspended:
			s.suspended = true
		}
	}
	return s
}

// constrained reports whether the compositor dictates the size: maximized
// and fullscreen sizes must be obeyed (xdg_toplevel raises
// invalid_surface_state for a larger maximized buffer) and tiled windows
// are expected to fill their tile. Otherwise the size is a suggestion.
func (s toplevelStates) constrained() bool { return s.maximized || s.fullscreen || s.tiled }

// sizeChoice is the size bookkeeping of a toplevel, in logical pixels.
type sizeChoice struct {
	current image.Point // size in use
	// stateless is the size the window had (or asked for) the last time
	// it was neither maximized, fullscreen nor tiled. A configure of 0x0
	// after leaving those states restores it (winit's "stateless size";
	// without it an unmaximized window would keep its maximized size).
	stateless image.Point
	min       image.Point // MinSize, rounded up
	fixed     bool        // FixedSize: the window picks its size, not the user
}

// choose returns the size to use for a toplevel configure of w×h with the
// given states, and updates the bookkeeping.
//
//   - Constrained (maximized, fullscreen, tiled): a non-zero size is used
//     as is, even below MinSize, because the compositor imposes it (the
//     platform contract lets the OS impose sizes; the app copes).
//   - Floating: the size is a suggestion. A FixedSize window keeps its
//     size; otherwise the suggestion is used, raised to MinSize.
//   - A zero width or height means "client decides" for that axis: the
//     current size, or the stateless size when floating.
func (c *sizeChoice) choose(w, h int32, st toplevelStates) image.Point {
	base := c.current
	if !st.constrained() {
		base = c.stateless
	}
	size := base
	if w > 0 {
		size.X = int(w)
	}
	if h > 0 {
		size.Y = int(h)
	}
	if !st.constrained() {
		if c.fixed {
			size = base
		}
		size = raise(size, c.min)
		c.stateless = size
	}
	if size.X < 1 {
		size.X = 1
	}
	if size.Y < 1 {
		size.Y = 1
	}
	c.current = size
	return size
}

func raise(p, min image.Point) image.Point {
	return image.Point{X: max(p.X, min.X), Y: max(p.Y, min.Y)}
}

// logicalPoint rounds a platform.Size to whole logical pixels, the unit of
// xdg_toplevel sizes, keeping at least 1x1 for non-empty sizes.
func logicalPoint(s platform.Size) image.Point {
	r := func(v float32) int {
		if !(v > 0) {
			return 0
		}
		return max(1, int(math.Round(float64(v))))
	}
	return image.Point{X: r(s.W), Y: r(s.H)}
}

// ceilPoint rounds a platform.Size up, for minimum sizes.
func ceilPoint(s platform.Size) image.Point {
	return image.Point{X: int(math.Ceil(float64(s.W))), Y: int(math.Ceil(float64(s.H)))}
}

func sizeOf(p image.Point) platform.Size { return platform.Size{W: float32(p.X), H: float32(p.Y)} }
