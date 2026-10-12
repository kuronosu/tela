// Copyright (c) 2018 Visly Inc. and Taffy contributors. MIT License.
// Ported from Taffy v0.14.0 (src/compute/common/alignment.rs,
// src/compute/common/scrollable_overflow.rs and
// src/compute/common/sizing_keyword.rs); see THIRD_PARTY.md.

package layout

// alignmentFallback resolves the fallbacks of an align-content or
// justify-content value (CSS Box Alignment 3, and csswg-drafts#10154): with
// one item or no free space the distributed values fall back to a position
// and become safe; safe alignment falls back to start when the items
// overflow. It returns a keyword without the safe modifier. Callers resolve
// AlignNormal first.
func alignmentFallback(free float32, n int, a Align) Align {
	k, safe := a.keyword(), a.isSafe()
	if n <= 1 || free <= 0 {
		switch k {
		case AlignStretch, AlignSpaceBetween:
			k, safe = AlignFlexStart, true
		case AlignSpaceAround, AlignSpaceEvenly:
			k, safe = AlignCenter, true
		}
	}
	if free <= 0 && safe {
		k = AlignStart
	}
	return k
}

// alignmentOffset returns the offset of an item (or flex line) for
// align-content and justify-content: the leading space before the first
// item, or the gap plus distributed space before any other.
func alignmentOffset(free float32, n int, gap float32, k Align, reversed, first bool) float32 {
	if first {
		switch k {
		case AlignFlexStart:
			if reversed {
				return free
			}
		case AlignEnd:
			return free
		case AlignFlexEnd:
			if !reversed {
				return free
			}
		case AlignCenter:
			return free / 2
		case AlignSpaceAround:
			if free >= 0 {
				return free / float32(n) / 2
			}
			return free / 2
		case AlignSpaceEvenly:
			if free >= 0 {
				return free / float32(n+1)
			}
			return free / 2
		}
		return 0
	}
	free = fmax(free, 0)
	switch k {
	case AlignSpaceBetween:
		return gap + free/float32(n-1)
	case AlignSpaceAround:
		return gap + free/float32(n)
	case AlignSpaceEvenly:
		return gap + free/float32(n+1)
	}
	return gap
}

// overflowContribution returns the rectangle a child contributes to its
// parent's scrollable overflow. loc is the child's border-box position from
// the parent's scroll origin (mirrored in RTL). The child's own overflow
// escapes only in the axes where it is visible and the child is not a
// scroll container; a parent scroll container excludes children wholly in
// its unreachable region (before the scroll origin).
func overflowContribution(loc point, sz size, childOverflow rect, st *Style, parentScrolls bool) rect {
	scrolls := st.isScrollContainer()
	px := !scrolls && st.OverflowX == OverflowVisible
	py := !scrolls && st.OverflowY == OverflowVisible
	endW, endH := sz.w, sz.h
	if px {
		endW = fmax(sz.w, childOverflow.r)
	}
	if py {
		endH = fmax(sz.h, childOverflow.b)
	}
	if endW <= 0 || endH <= 0 {
		return rect{}
	}
	var startX, startY float32
	if px {
		startX = fmin(0, childOverflow.l)
	}
	if py {
		startY = fmin(0, childOverflow.t)
	}
	c := rect{l: loc.x + startX, r: loc.x + endW, t: loc.y + startY, b: loc.y + endH}
	if parentScrolls && (c.r <= 0 || c.b <= 0) {
		return rect{}
	}
	return c
}

// keywordKind is how a sizing keyword resolves.
type keywordKind uint8

const (
	keywordNone    keywordKind = iota // not a keyword, or unresolvable: auto
	keywordMeasure                    // measure the item under the given space
	keywordExact                      // a size
)

// resolveSizingKeyword resolves a size or flex-basis value that is one of
// min-content, max-content, fit-content, fit-content(...) or stretch.
// stretch is the size the item takes when stretched (available space
// minus margins); basis is the percentage basis of fit-content(%).
func resolveSizingKeyword(d Dim, stretch, basis opt) (keywordKind, AvailableSpace, float32) {
	switch d.Unit {
	case UnitMinContent:
		return keywordMeasure, MinContentConstraint, 0
	case UnitMaxContent:
		return keywordMeasure, MaxContentConstraint, 0
	case UnitFitContentPx:
		return keywordMeasure, DefiniteSpace(d.Value), 0
	case UnitFitContentPercent:
		if basis.ok {
			return keywordMeasure, DefiniteSpace(float32(basis.v * d.Value)), 0
		}
	case UnitFitContent:
		if stretch.ok {
			return keywordMeasure, DefiniteSpace(stretch.v), 0
		}
	case UnitStretch:
		if stretch.ok {
			return keywordExact, AvailableSpace{}, stretch.v
		}
	}
	return keywordNone, AvailableSpace{}, 0
}

// resolveAbsoluteSizingKeywords fills the known dimensions of an absolutely
// positioned item whose size styles are sizing keywords. area is its
// containing block; the stretch size subtracts the insets and margins.
func (e *Engine) resolveAbsoluteSizingKeywords(p, c Node, known *optSize, sizeStyle dimSize, area size, inset, margin optRect, sizing sizingMode) {
	stretch := size{
		w: fmax(area.w-inset.l.or0()-inset.r.or0()-margin.l.or0()-margin.r.or0(), 0),
		h: fmax(area.h-inset.t.or0()-inset.b.or0()-margin.t.or0()-margin.b.or0(), 0),
	}
	kw, kh := keywordNone, keywordNone
	var aw, ah AvailableSpace
	var ew, eh float32
	if !known.w.ok {
		kw, aw, ew = resolveSizingKeyword(sizeStyle.w, some(stretch.w), some(area.w))
	}
	if !known.h.ok {
		kh, ah, eh = resolveSizingKeyword(sizeStyle.h, some(stretch.h), some(area.h))
	}
	// Both axes measured: one measurement.
	if kw == keywordMeasure && kh == keywordMeasure {
		*known = e.measureChildSizeBoth(p, c, optSize{}, area.opts(), availSize{aw, ah}, sizing).opts()
		return
	}
	switch kw {
	case keywordExact:
		known.w = some(ew)
	case keywordMeasure:
		known.w = some(e.measureChildSize(p, c, *known, area.opts(), availSize{aw, DefiniteSpace(stretch.h)}, sizing, horizontal))
	}
	switch kh {
	case keywordExact:
		known.h = some(eh)
	case keywordMeasure:
		w := DefiniteSpace(stretch.w)
		if known.w.ok {
			w = DefiniteSpace(known.w.v)
		}
		known.h = some(e.measureChildSize(p, c, *known, area.opts(), availSize{w, ah}, sizing, vertical))
	}
}
