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
	maximized, fullscreen, resizing, activated, suspended bool
	// tiled holds the edges the window is tiled against (tiled_left,
	// tiled_right, tiled_top, tiled_bottom), as the bits of the matching
	// resize edges: GTK neither resizes from nor shadows a tiled edge.
	tiled protocols.XdgToplevelResizeEdge
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
		case protocols.XdgToplevelStateTiledLeft:
			s.tiled |= protocols.XdgToplevelResizeEdgeLeft
		case protocols.XdgToplevelStateTiledRight:
			s.tiled |= protocols.XdgToplevelResizeEdgeRight
		case protocols.XdgToplevelStateTiledTop:
			s.tiled |= protocols.XdgToplevelResizeEdgeTop
		case protocols.XdgToplevelStateTiledBottom:
			s.tiled |= protocols.XdgToplevelResizeEdgeBottom
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
func (s toplevelStates) constrained() bool { return s.maximized || s.fullscreen || s.tiled != 0 }

// sizeChoice is the size bookkeeping of a toplevel, in logical pixels.
type sizeChoice struct {
	current image.Point // size in use
	// stateless is the size the window had (or asked for) the last time
	// it was neither maximized, fullscreen nor tiled. A configure of 0x0
	// after leaving those states restores it (winit's "stateless size";
	// without it an unmaximized window would keep its maximized size).
	stateless image.Point
	min       image.Point // MinSize (or SetMinSize), rounded up
	max       image.Point // MaxSize (or SetMaxSize), rounded down; 0 = no limit
	fixed     bool        // FixedSize: the window picks its size, not the user
}

// limit applies the size limits to p: clamped to max, then raised to min,
// per axis (the minimum wins, like platform.ClampSize).
func (c *sizeChoice) limit(p image.Point) image.Point {
	if c.max.X > 0 && p.X > c.max.X {
		p.X = c.max.X
	}
	if c.max.Y > 0 && p.Y > c.max.Y {
		p.Y = c.max.Y
	}
	return raise(p, c.min)
}

// hints returns the xdg_toplevel min and max sizes to send: the limits,
// or the stateless size for both with FixedSize. A maximum below the
// minimum is raised to it, since xdg_toplevel rejects a minimum above the
// maximum (invalid_size) and the minimum wins anyway.
func (c *sizeChoice) hints() (min, max image.Point) {
	min, max = c.min, c.max
	if c.fixed {
		min, max = c.stateless, c.stateless
	}
	if max.X > 0 && max.X < min.X {
		max.X = min.X
	}
	if max.Y > 0 && max.Y < min.Y {
		max.Y = min.Y
	}
	return min, max
}

// choose returns the size to use for a toplevel configure of w×h with the
// given states, and updates the bookkeeping.
//
//   - Constrained (maximized, fullscreen, tiled): a non-zero size is used
//     as is, even below MinSize, because the compositor imposes it (the
//     platform contract lets the OS impose sizes; the app copes).
//   - Floating: the size is a suggestion. A FixedSize window keeps its
//     size; otherwise the suggestion is used, within the size limits.
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
		size = c.limit(size)
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

// floorPoint rounds a platform.Size down, for maximum sizes, keeping at
// least 1 for a non-zero component (zero is "no limit").
func floorPoint(s platform.Size) image.Point {
	f := func(v float32) int {
		if !(v > 0) {
			return 0
		}
		return max(1, int(math.Floor(float64(v))))
	}
	return image.Point{X: f(s.W), Y: f(s.H)}
}

func sizeOf(p image.Point) platform.Size { return platform.Size{W: float32(p.X), H: float32(p.Y)} }
