package platform

import "image"

// MinVisibleWidth and MinVisibleHeight are how much of a window must lie
// in one work area for [FitOnScreens] to leave it where it is (or all of
// the window, when it is smaller), in the backend's desktop units
// (physical pixels on Win32, X11 and headless; points on Cocoa). They are
// enough to grab a title bar and drag the window back: Chromium keeps 30
// by 30 (window_sizer.cc, kMinVisibleWidth and kMinVisibleHeight), which
// is a tight target at a scale of 2; these are a little more generous.
const (
	MinVisibleWidth  = 100
	MinVisibleHeight = 50
)

// FitOnScreens returns where a window whose rectangle is win (the visible
// part of its frame where the backend knows it, else its content area, in
// the backend's desktop coordinates) can open on the monitors present,
// whose work areas (the monitor minus task bars, docks and panels) are
// works, the primary first. Backends apply it to WindowOptions.Position
// at creation (X11 at the first map, once the window manager told the
// frame): a position saved in an earlier run may be on a monitor that is
// gone (a laptop undocked, a resolution changed).
//
// win is returned as it is when it can still be grabbed: its top edge is
// inside a work area vertically (the title bar is not above the screen or
// under a menu bar) and at least [MinVisibleWidth] by [MinVisibleHeight]
// of it (or its whole width or height, when smaller) lies in that work
// area. Otherwise the window moves into one work area, keeping its size:
// the one it overlaps most, else the nearest (ties go to the first, the
// primary). It moves as little as fits there; a window wider or taller
// than the area goes to the area's left or top edge. With no work areas
// (none known), or only empty ones, win is returned as it is.
//
// The same rule is what Chromium (display::AdjustBoundsToBeVisibleOnDisplay
// and WindowSizer), electron-window-state and Tauri's window-state plugin
// apply to a restored window.
func FitOnScreens(win image.Rectangle, works []image.Rectangle) image.Rectangle {
	win = win.Canon()
	best, bestOverlap, bestDist := -1, 0.0, 0.0
	for i, a := range works {
		if a.Empty() {
			continue
		}
		if grabbable(win, a) {
			return win
		}
		overlap, dist := 0.0, rectDistance2(win, a)
		if in := win.Intersect(a); !in.Empty() {
			overlap = float64(in.Dx()) * float64(in.Dy())
		}
		switch {
		case best < 0,
			overlap > bestOverlap,
			overlap == bestOverlap && overlap == 0 && dist < bestDist:
			best, bestOverlap, bestDist = i, overlap, dist
		}
	}
	if best < 0 {
		return win
	}
	a := works[best]
	return win.Add(image.Pt(fitAxis(win.Min.X, win.Dx(), a.Min.X, a.Max.X), fitAxis(win.Min.Y, win.Dy(), a.Min.Y, a.Max.Y)).Sub(win.Min))
}

// grabbable reports whether win's top edge is inside a vertically and
// enough of win lies in a (see FitOnScreens). An empty window counts as
// one pixel at its top-left.
func grabbable(win, a image.Rectangle) bool {
	if win.Min.Y < a.Min.Y || win.Min.Y >= a.Max.Y {
		return false
	}
	w, h := max(win.Dx(), 1), max(win.Dy(), 1)
	in := image.Rectangle{Min: win.Min, Max: win.Min.Add(image.Pt(w, h))}.Intersect(a)
	return in.Dx() >= min(w, MinVisibleWidth) && in.Dy() >= min(h, MinVisibleHeight)
}

// fitAxis returns where a span of length n starting at p goes inside
// [lo, hi): unchanged when it fits, else moved as little as possible, and
// at lo when it is longer than the area.
func fitAxis(p, n, lo, hi int) int {
	if p+n > hi {
		p = hi - n
	}
	return max(p, lo)
}

// rectDistance2 returns the squared distance between two rectangles: zero
// when they overlap or touch. It is a float64 because the square of a
// distance across 32-bit coordinates overflows an int.
func rectDistance2(r, s image.Rectangle) float64 {
	dx := float64(max(s.Min.X-r.Max.X, r.Min.X-s.Max.X, 0))
	dy := float64(max(s.Min.Y-r.Max.Y, r.Min.Y-s.Max.Y, 0))
	return dx*dx + dy*dy
}
