package platform

import (
	"image"
	"slices"
)

// Transparent windows (WindowOptions.Transparent, ADR-249). See
// "Transparent windows" in the package documentation.

// InputAlpha is the alpha, of 255, from which a pixel of a Transparent
// window's presented frame takes pointer input: 64, a quarter. Pointer
// input over the other pixels passes to what is below the window. The
// shadows of Tailwind's shadow-* utilities stay below it (the visible
// part of shadow-2xl is at most about 15 % opaque), so they do not catch
// the clicks meant for the desktop, while content does, translucent
// surfaces included (bg-white/30).
const InputAlpha = 64

// InputMask is the part of a Transparent window that takes pointer input:
// the pixels of its presented frames whose alpha is at least InputAlpha,
// in physical pixels (framebuffer coordinates). A backend keeps one per
// transparent window and updates it in Present from the very pixels and
// damage it presents, so it always matches what is on screen; the cost is
// proportional to the damage, as only the damaged part of each damaged
// row is scanned again. The zero value is an empty mask of size zero.
// It is not safe for concurrent use.
type InputMask struct {
	size image.Point
	rows [][]span // per row, sorted, disjoint, non-adjacent runs
	tmp  []span
}

// span is a run [x0, x1) of pixels of one row that take input.
type span struct{ x0, x1 int32 }

// Size returns the size of the frames the mask was last updated with.
func (m *InputMask) Size() image.Point { return m.size }

// Update updates the mask with the rectangles rects of fb, the damage of
// a Present already clipped to fb.Rect (ClipDamage). A frame of a new
// size starts from an empty mask, so its first Present must be full. It
// reports whether the mask changed.
func (m *InputMask) Update(fb *image.RGBA, rects []image.Rectangle) bool {
	size := fb.Rect.Size()
	changed := false
	if size != m.size {
		m.size = size
		m.rows = make([][]span, size.Y)
		changed = true
	}
	bounds := image.Rectangle{Max: size}
	for _, r := range rects {
		r = r.Intersect(bounds)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			if m.updateRow(fb, y, int32(r.Min.X), int32(r.Max.X)) {
				changed = true
			}
		}
	}
	return changed
}

// updateRow scans the pixels [x0, x1) of row y again and reports whether
// the row's runs changed.
func (m *InputMask) updateRow(fb *image.RGBA, y int, x0, x1 int32) bool {
	old := m.rows[y]
	out := m.tmp[:0]
	add := func(s span) {
		if s.x1 <= s.x0 {
			return
		}
		if n := len(out); n > 0 && out[n-1].x1 >= s.x0 {
			out[n-1].x1 = max(out[n-1].x1, s.x1)
			return
		}
		out = append(out, s)
	}
	for i := 0; i < len(old) && old[i].x0 < x0; i++ {
		add(span{old[i].x0, min(old[i].x1, x0)})
	}
	pix := fb.Pix[fb.PixOffset(0, y):]
	for x := x0; x < x1; {
		if pix[4*x+3] < InputAlpha {
			x++
			continue
		}
		s := x
		for x < x1 && pix[4*x+3] >= InputAlpha {
			x++
		}
		add(span{s, x})
	}
	for _, s := range old {
		if s.x1 > x1 {
			add(span{max(s.x0, x1), s.x1})
		}
	}
	m.tmp = out
	if sameSpans(old, out) {
		return false
	}
	m.rows[y] = append(old[:0:0], out...)
	return true
}

func sameSpans(a, b []span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Contains reports whether the pixel p (physical, framebuffer
// coordinates) takes input. Points outside the frame do not.
func (m *InputMask) Contains(p image.Point) bool {
	if p.X < 0 || p.Y < 0 || p.X >= m.size.X || p.Y >= m.size.Y {
		return false
	}
	row := m.rows[p.Y]
	x := int32(p.X)
	lo, hi := 0, len(row)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case x < row[mid].x0:
			hi = mid
		case x >= row[mid].x1:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// Empty reports whether no pixel takes input.
func (m *InputMask) Empty() bool {
	for _, r := range m.rows {
		if len(r) > 0 {
			return false
		}
	}
	return true
}

// Rects returns the mask as at most limit rectangles (limit > 0), in
// bands of rows with the same runs, sorted by y then x (X11's YX-banded
// order): exact when that fits, otherwise a cover of the mask that may
// take input at some pixels of the bounding boxes of its rows, never
// fewer. Window systems take input regions as rectangles (wl_region, the
// X11 SHAPE input region); a frame of text over transparent pixels has
// many runs.
func (m *InputMask) Rects(limit int) []image.Rectangle { return rowRects(m.rows, limit) }

// RectsWith returns the union of the mask and the rectangles extra (in
// the same physical pixels, cut to the mask's size) as Rects does: YX
// banded, at most limit rectangles. A transparent window's input with its
// resize handles is one (HandleRects, ADR-261); the mask is unchanged.
func (m *InputMask) RectsWith(extra []image.Rectangle, limit int) []image.Rectangle {
	if len(extra) == 0 {
		return m.Rects(limit)
	}
	rows := slices.Clone(m.rows)
	bounds := image.Rectangle{Max: m.size}
	for _, r := range extra {
		r = r.Intersect(bounds)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			rows[y] = addSpan(rows[y], span{int32(r.Min.X), int32(r.Max.X)})
		}
	}
	return rowRects(rows, limit)
}

// addSpan returns the runs of row with s added, in a new slice (row may
// be the mask's).
func addSpan(row []span, s span) []span {
	out := make([]span, 0, len(row)+1)
	i := 0
	for ; i < len(row) && row[i].x1 < s.x0; i++ {
		out = append(out, row[i])
	}
	for ; i < len(row) && row[i].x0 <= s.x1; i++ {
		s.x0, s.x1 = min(s.x0, row[i].x0), max(s.x1, row[i].x1)
	}
	out = append(out, s)
	return append(out, row[i:]...)
}

// rowRects returns rows of runs as at most limit banded rectangles (see
// Rects).
func rowRects(rows [][]span, limit int) []image.Rectangle {
	if limit < 1 {
		limit = 1
	}
	if out := bands(rows, 1, false, limit); out != nil {
		return out
	}
	// Each row's runs merged into their bounding run, then rows merged in
	// groups of growing height until the bands fit.
	for g := 1; ; g *= 2 {
		if out := bands(rows, g, true, limit); out != nil || g >= len(rows) {
			if out == nil {
				out = bands(rows, len(rows), true, limit)
			}
			return out
		}
	}
}

// bands returns the bands of rows (in groups of g rows, each group taking
// the union of its rows' bounding runs when bound is set), or nil when
// they would be more than limit rectangles.
func bands(rows [][]span, g int, bound bool, limit int) []image.Rectangle {
	out := []image.Rectangle{}
	var cur []span
	start := 0
	flush := func(end int) bool {
		for _, s := range cur {
			if len(out) == limit {
				return false
			}
			out = append(out, image.Rect(int(s.x0), start, int(s.x1), end))
		}
		return true
	}
	var buf [1]span
	for y := 0; y < len(rows); y += g {
		row := rows[y]
		if bound || g > 1 {
			lo, hi := int32(-1), int32(-1)
			for k := y; k < min(y+g, len(rows)); k++ {
				if r := rows[k]; len(r) > 0 {
					if lo < 0 || r[0].x0 < lo {
						lo = r[0].x0
					}
					hi = max(hi, r[len(r)-1].x1)
				}
			}
			row = nil
			if lo >= 0 {
				buf[0] = span{lo, hi}
				row = buf[:]
			}
		}
		if y > 0 && sameSpans(row, cur) {
			continue
		}
		if !flush(y) {
			return nil
		}
		cur = append(cur[:0:0], row...)
		start = y
	}
	if !flush(len(rows)) {
		return nil
	}
	return out
}
