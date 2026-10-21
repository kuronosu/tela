// Port of the middleware of @floating-ui/core: offset, flip, shift (with
// limitShift), size, arrow, hide and autoPlacement.
//
// Copyright (c) 2021-present Floating UI contributors. MIT License; the
// full notice is in THIRD_PARTY.md.

package aria

import (
	"math"
	"sort"

	"github.com/kuronosu/tela"
)

// ---------------------------------------------------------------------
// offset

// offsetData is middlewareData.offset: the translation the middleware
// applied and the placement it applied it to.
type offsetData struct {
	set       bool
	x, y      float32
	placement string
}

// offsetParams are the values of Floating UI's offset: the main axis (the
// distance), the cross axis (the skidding) and the alignment axis, which
// replaces the cross axis for aligned placements (negated for "end").
type offsetParams struct {
	mainAxis, crossAxis float32
	alignmentAxis       float32
	hasAlignmentAxis    bool
}

func isOriginSide(side string) bool { return side == "left" || side == "top" }

// convertValueToCoords is Floating UI's convertValueToCoords.
func convertValueToCoords(s *state, p offsetParams) coords {
	side := getSide(s.placement)
	alignment := getAlignment(s.placement)
	isVertical := getSideAxis(s.placement) == axisY
	mainAxisMulti := float32(1)
	if isOriginSide(side) {
		mainAxisMulti = -1
	}
	crossAxisMulti := float32(1)
	if s.env.rtl && isVertical {
		crossAxisMulti = -1
	}
	mainAxis, crossAxis := p.mainAxis, p.crossAxis
	if alignment != "" && p.hasAlignmentAxis {
		if alignment == "end" {
			crossAxis = p.alignmentAxis * -1
		} else {
			crossAxis = p.alignmentAxis
		}
	}
	if isVertical {
		return coords{x: crossAxis * crossAxisMulti, y: mainAxis * mainAxisMulti}
	}
	return coords{x: mainAxis * mainAxisMulti, y: crossAxis * crossAxisMulti}
}

// offsetMiddleware modifies the placement by translating the floating
// element along the main and cross axes.
func offsetMiddleware(p offsetParams) middleware {
	return middleware{name: "offset", fn: func(s *state) result {
		diff := convertValueToCoords(s, p)
		// If the placement is the same and the arrow caused an alignment
		// offset then we don't need to change the positioning coordinates.
		if o := s.data.offset; o.set && s.placement == o.placement && s.data.arrow.hasAlignmentOffset() {
			return result{}
		}
		s.data.offset = offsetData{set: true, x: diff.x, y: diff.y, placement: s.placement}
		return result{setX: true, setY: true, x: s.x + diff.x, y: s.y + diff.y}
	}}
}

// ---------------------------------------------------------------------
// shift

// shiftData is middlewareData.shift: how far the middleware moved the
// element and which axes it checked.
type shiftData struct {
	set      bool
	x, y     float32
	enabledX bool
	enabledY bool
}

// shiftParams are the options of Floating UI's shift (besides the overflow
// ones).
type shiftParams struct {
	padding             float32
	mainAxis, crossAxis bool
	limiter             ShiftLimiter
}

// shiftMiddleware keeps the floating element in view by shifting it along
// the side of its placement.
func shiftMiddleware(p shiftParams) middleware {
	return middleware{name: "shift", fn: func(s *state) result {
		c := coords{s.x, s.y}
		overflow := detectOverflow(s, overflowOptions{padding: uniformPadding(p.padding)})
		crossAxis := getSideAxis(getSide(s.placement))
		mainAxis := oppositeAxis(crossAxis)
		mainCoord := c.get(mainAxis)
		crossCoord := c.get(crossAxis)
		if p.mainAxis {
			minSide, maxSide := "left", "right"
			if mainAxis == axisY {
				minSide, maxSide = "top", "bottom"
			}
			lo := mainCoord + overflow.side(minSide)
			hi := mainCoord - overflow.side(maxSide)
			mainCoord = clamp(lo, mainCoord, hi)
		}
		if p.crossAxis {
			minSide, maxSide := "left", "right"
			if crossAxis == axisY {
				minSide, maxSide = "top", "bottom"
			}
			lo := crossCoord + overflow.side(minSide)
			hi := crossCoord - overflow.side(maxSide)
			crossCoord = clamp(lo, crossCoord, hi)
		}
		limited := coords{}
		limited.set(mainAxis, mainCoord)
		limited.set(crossAxis, crossCoord)
		if p.limiter.fn != nil {
			ls := *s
			ls.x, ls.y = limited.x, limited.y
			limited = p.limiter.fn(&ls)
		}
		d := shiftData{set: true, x: limited.x - c.x, y: limited.y - c.y}
		if mainAxis == axisX {
			d.enabledX, d.enabledY = p.mainAxis, p.crossAxis
		} else {
			d.enabledY, d.enabledX = p.mainAxis, p.crossAxis
		}
		s.data.shift = d
		return result{setX: true, setY: true, x: limited.x, y: limited.y}
	}}
}

// limitShiftMiddleware is Floating UI's limitShift limiter.
func limitShiftMiddleware(offset, crossOffset float32, mainAxisOn, crossAxisOn bool) func(s *state) coords {
	return func(s *state) coords {
		c := coords{s.x, s.y}
		crossAxis := getSideAxis(s.placement)
		mainAxis := oppositeAxis(crossAxis)
		mainCoord := c.get(mainAxis)
		crossCoord := c.get(crossAxis)
		if mainAxisOn {
			limitMin := s.rects.reference.pos(mainAxis) - s.rects.floating.length(mainAxis) + offset
			limitMax := s.rects.reference.pos(mainAxis) + s.rects.reference.length(mainAxis) - offset
			if mainCoord < limitMin {
				mainCoord = limitMin
			} else if mainCoord > limitMax {
				mainCoord = limitMax
			}
		}
		if crossAxisOn {
			origin := isOriginSide(getSide(s.placement))
			var offData float32 // middlewareData.offset?.[crossAxis] || 0
			if s.data.offset.set {
				offData = coords{s.data.offset.x, s.data.offset.y}.get(crossAxis)
			}
			limitMin := s.rects.reference.pos(crossAxis) - s.rects.floating.length(crossAxis)
			limitMax := s.rects.reference.pos(crossAxis) + s.rects.reference.length(crossAxis)
			if origin {
				limitMin += offData
				limitMax -= crossOffset
			} else {
				limitMin += crossOffset
				limitMax += offData
			}
			if crossCoord < limitMin {
				crossCoord = limitMin
			} else if crossCoord > limitMax {
				crossCoord = limitMax
			}
		}
		var out coords
		out.set(mainAxis, mainCoord)
		out.set(crossAxis, crossCoord)
		return out
	}
}

// ---------------------------------------------------------------------
// flip

// flipParams are the options of Floating UI's flip.
type flipParams struct {
	padding                   float32
	mainAxis, crossAxis       bool
	crossAxisAlignment        bool // crossAxis: 'alignment'
	fallbackPlacements        []string
	hasFallbackPlacements     bool
	fallbackStrategy          string // "bestFit" or "initialPlacement"
	fallbackAxisSideDirection string // "none", "start" or "end"
	flipAlignment             bool
}

// flipMiddleware flips the placement to keep the floating element in view
// when the preferred placement overflows the clipping rectangle.
func flipMiddleware(p flipParams) middleware {
	return middleware{name: "flip", fn: func(s *state) result {
		// If a reset by the arrow was caused due to an alignment offset
		// being added, flip() has already done its work.
		if s.data.arrow.hasAlignmentOffset() {
			return result{}
		}
		side := getSide(s.placement)
		initialSideAxis := getSideAxis(s.initialPlacement)
		isBasePlacement := getSide(s.initialPlacement) == s.initialPlacement
		rtl := s.env.rtl
		var fallback []string
		if p.hasFallbackPlacements {
			fallback = append(fallback, p.fallbackPlacements...)
		} else if isBasePlacement || !p.flipAlignment {
			fallback = []string{getOppositePlacement(s.initialPlacement)}
		} else {
			fallback = getExpandedPlacements(s.initialPlacement)
		}
		hasFallbackAxisSideDirection := p.fallbackAxisSideDirection != "none"
		if !p.hasFallbackPlacements && hasFallbackAxisSideDirection {
			fallback = append(fallback, getOppositeAxisPlacements(s.initialPlacement, p.flipAlignment, p.fallbackAxisSideDirection, rtl)...)
		}
		placements := append([]string{s.initialPlacement}, fallback...)
		overflow := detectOverflow(s, overflowOptions{padding: uniformPadding(p.padding)})
		var overflows []float32
		overflowsData := append([]placementOverflow(nil), s.data.flip.overflows...)
		if p.mainAxis {
			overflows = append(overflows, overflow.side(side))
		}
		if p.crossAxis {
			sides := getAlignmentSides(s.placement, s.rects, rtl)
			overflows = append(overflows, overflow.side(sides[0]), overflow.side(sides[1]))
		}
		overflowsData = append(overflowsData, placementOverflow{s.placement, overflows})

		// One or more sides is overflowing.
		if !allAtMost(overflows, 0) {
			nextIndex := s.data.flip.index + 1
			if nextIndex < len(placements) {
				nextPlacement := placements[nextIndex]
				ignoreCrossAxisOverflow := p.crossAxisAlignment && p.crossAxis && initialSideAxis != getSideAxis(nextPlacement)
				ok := !ignoreCrossAxisOverflow
				if !ok {
					// We leave the current main axis only if every
					// placement on that axis overflows the main axis.
					ok = true
					for _, d := range overflowsData {
						if getSideAxis(d.placement) == initialSideAxis && !(overflowAt(d.overflows, 0) > 0) {
							ok = false
							break
						}
					}
				}
				if ok {
					// Try next placement and re-run the lifecycle.
					s.data.flip = placementData{index: nextIndex, overflows: overflowsData}
					return result{reset: resetPlacement, placement: nextPlacement}
				}
			}

			// First, find the candidates that fit on the main axis side of
			// overflow, then find the placement that fits the best on the
			// main cross axis side.
			resetTo := ""
			var cands []placementOverflow
			for _, d := range overflowsData {
				if overflowAt(d.overflows, 0) <= 0 {
					cands = append(cands, d)
				}
			}
			sort.SliceStable(cands, func(i, j int) bool {
				return overflowAt(cands[i].overflows, 1) < overflowAt(cands[j].overflows, 1)
			})
			if len(cands) > 0 {
				resetTo = cands[0].placement
			}

			// Otherwise fallback.
			if resetTo == "" {
				switch p.fallbackStrategy {
				case "bestFit":
					type scored struct {
						placement string
						sum       float32
					}
					var list []scored
					for _, d := range overflowsData {
						if hasFallbackAxisSideDirection {
							cur := getSideAxis(d.placement)
							// Create a bias to the y side axis due to
							// horizontal reading directions favoring
							// greater width.
							if cur != initialSideAxis && cur != axisY {
								continue
							}
						}
						var sum float32
						for _, o := range d.overflows {
							if o > 0 {
								sum += o
							}
						}
						list = append(list, scored{d.placement, sum})
					}
					sort.SliceStable(list, func(i, j int) bool { return list[i].sum < list[j].sum })
					if len(list) > 0 {
						resetTo = list[0].placement
					}
				case "initialPlacement":
					resetTo = s.initialPlacement
				}
			}
			if s.placement != resetTo {
				return result{reset: resetPlacement, placement: resetTo}
			}
		}
		// Floating UI stores the data of every run; the index and the
		// overflows only change when a placement is tried (above).
		return result{}
	}}
}

// overflowAt returns overflows[i], or NaN like a missing element in
// JavaScript (every comparison with it is false).
func overflowAt(overflows []float32, i int) float32 {
	if i < len(overflows) {
		return overflows[i]
	}
	return float32(math.NaN())
}

func allAtMost(v []float32, limit float32) bool {
	for _, x := range v {
		if !(x <= limit) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------
// autoPlacement

// autoPlacementParams are the options of Floating UI's autoPlacement.
type autoPlacementParams struct {
	padding           float32
	crossAxis         bool
	alignment         string // "", "start" or "end"
	allowedPlacements []string
	hasAllowed        bool
	autoAlignment     bool
}

func getPlacementList(alignment string, autoAlignment bool, allowed []string) []string {
	var sorted []string
	if alignment != "" {
		for _, p := range allowed {
			if getAlignment(p) == alignment {
				sorted = append(sorted, p)
			}
		}
		for _, p := range allowed {
			if getAlignment(p) != alignment {
				sorted = append(sorted, p)
			}
		}
	} else {
		for _, p := range allowed {
			if getSide(p) == p {
				sorted = append(sorted, p)
			}
		}
	}
	var out []string
	for _, p := range sorted {
		if alignment != "" {
			if getAlignment(p) == alignment || (autoAlignment && getOppositeAlignmentPlacement(p) != p) {
				out = append(out, p)
			}
			continue
		}
		out = append(out, p)
	}
	return out
}

// autoPlacementMiddleware chooses the placement with the most space
// available, without a preferred one. Alternative to flip.
func autoPlacementMiddleware(p autoPlacementParams) middleware {
	return middleware{name: "autoPlacement", fn: func(s *state) result {
		var placements []string
		if p.alignment != "" || !p.hasAllowed {
			allowed := p.allowedPlacements
			if !p.hasAllowed {
				allowed = allPlacements
			}
			placements = getPlacementList(p.alignment, p.autoAlignment, allowed)
		} else {
			placements = p.allowedPlacements
		}
		overflow := detectOverflow(s, overflowOptions{padding: uniformPadding(p.padding)})
		currentIndex := s.data.autoPlacement.index
		if currentIndex >= len(placements) {
			return result{}
		}
		currentPlacement := placements[currentIndex]
		alignmentSides := getAlignmentSides(currentPlacement, s.rects, s.env.rtl)

		// Make computeCoords start from the right place.
		if s.placement != currentPlacement {
			return result{reset: resetPlacement, placement: placements[0]}
		}
		currentOverflows := []float32{
			overflow.side(getSide(currentPlacement)),
			overflow.side(alignmentSides[0]),
			overflow.side(alignmentSides[1]),
		}
		all := append(append([]placementOverflow(nil), s.data.autoPlacement.overflows...), placementOverflow{currentPlacement, currentOverflows})

		// There are more placements to check.
		if currentIndex+1 < len(placements) {
			s.data.autoPlacement = placementData{index: currentIndex + 1, overflows: all}
			return result{reset: resetPlacement, placement: placements[currentIndex+1]}
		}
		type scored struct {
			placement string
			score     float32
			overflows []float32
		}
		list := make([]scored, len(all))
		for i, d := range all {
			var score float32
			if getAlignment(d.placement) != "" && p.crossAxis {
				// Check along the main axis and main cross axis side.
				score = d.overflows[0] + d.overflows[1]
			} else {
				// Check only the main axis.
				score = d.overflows[0]
			}
			list[i] = scored{d.placement, score, d.overflows}
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].score < list[j].score })
		resetTo := list[0].placement
		for _, d := range list {
			n := 3
			if getAlignment(d.placement) != "" {
				// Aligned placements should not check their opposite cross
				// axis side.
				n = 2
			}
			if allAtMost(d.overflows[:n], 0) {
				resetTo = d.placement
				break
			}
		}
		if resetTo != s.placement {
			s.data.autoPlacement = placementData{index: currentIndex + 1, overflows: all}
			return result{reset: resetPlacement, placement: resetTo}
		}
		return result{}
	}}
}

// ---------------------------------------------------------------------
// arrow

// arrowData is middlewareData.arrow: the offset of the arrow along its
// axis (x or y), how far it is from being centered on the reference, and
// the alignment offset the middleware added to the floating element.
type arrowData struct {
	ran             bool
	hasX, hasY      bool
	x, y            float32
	centerOffset    float32
	alignmentOffset float32
}

// hasAlignmentOffset reports a truthy middlewareData.arrow.alignmentOffset.
func (a arrowData) hasAlignmentOffset() bool {
	return a.alignmentOffset != 0 && !math.IsNaN(float64(a.alignmentOffset))
}

// arrowMiddleware provides data to position an inner element of the
// floating element so that it appears centered to the reference element.
func arrowMiddleware(padding sideOffsets) middleware {
	return middleware{name: "arrow", fn: func(s *state) result {
		if !s.env.hasArrow {
			s.data.arrow.ran = true
			return result{}
		}
		c := coords{s.x, s.y}
		ax := getAlignmentAxis(s.placement)
		minProp, maxProp := "left", "right"
		if ax == axisY {
			minProp, maxProp = "top", "bottom"
		}
		arrowLen := s.env.arrow.length(ax)
		endDiff := s.rects.reference.length(ax) + s.rects.reference.pos(ax) - c.get(ax) - s.rects.floating.length(ax)
		startDiff := c.get(ax) - s.rects.reference.pos(ax)
		// The arrow's offset parent is the floating element: its size is
		// the border box (Floating UI reads the client size).
		clientSize := s.rects.floating.length(ax)
		centerToReference := endDiff/2 - startDiff/2

		// If the padding is large enough that it causes the arrow to no
		// longer be centered, modify the padding so that it is centered.
		largestPossiblePadding := clientSize/2 - arrowLen/2 - 1
		minPadding := min(padding.side(minProp), largestPossiblePadding)
		maxPadding := min(padding.side(maxProp), largestPossiblePadding)

		// Make sure the arrow doesn't overflow the floating element if the
		// center point is outside the floating element's bounds.
		lo := minPadding
		hi := clientSize - arrowLen - maxPadding
		center := clientSize/2 - arrowLen/2 + centerToReference
		offset := clamp(lo, center, hi)

		// If the reference is small enough that the arrow's padding causes
		// it to point to nothing for an aligned placement, adjust the offset
		// of the floating element itself. To ensure shift() continues to
		// take action, a single reset is performed when this is true.
		shouldAddOffset := !s.data.arrow.ran && getAlignment(s.placement) != "" && center != offset
		if shouldAddOffset {
			pad := maxPadding
			if center < lo {
				pad = minPadding
			}
			shouldAddOffset = s.rects.reference.length(ax)/2-pad-arrowLen/2 < 0
		}
		var alignmentOffset float32
		if shouldAddOffset {
			if center < lo {
				alignmentOffset = center - lo
			} else {
				alignmentOffset = center - hi
			}
		}
		d := &s.data.arrow
		d.ran = true
		if ax == axisY {
			d.hasY, d.y = true, offset
		} else {
			d.hasX, d.x = true, offset
		}
		d.centerOffset = center - offset - alignmentOffset
		if shouldAddOffset {
			d.alignmentOffset = alignmentOffset
		}
		var r result
		r.setCoord(ax, c.get(ax)+alignmentOffset)
		if shouldAddOffset {
			r.reset = resetAll
		}
		return r
	}}
}

// ---------------------------------------------------------------------
// hide

// hideData is middlewareData.hide.
type hideData struct {
	referenceHidden, escaped bool
	referenceHiddenOffsets   sideOffsets
	escapedOffsets           sideOffsets
	hasReference, hasEscaped bool
}

func getSideOffsets(overflow sideOffsets, r rect) sideOffsets {
	return sideOffsets{
		top:    overflow.top - r.height,
		right:  overflow.right - r.width,
		bottom: overflow.bottom - r.height,
		left:   overflow.left - r.width,
	}
}

func isAnySideFullyClipped(o sideOffsets) bool {
	return o.top >= 0 || o.right >= 0 || o.bottom >= 0 || o.left >= 0
}

// hideMiddleware provides data to hide the floating element in applicable
// situations, such as when its reference is scrolled out of view
// ("referenceHidden") or it is not in the same clipping context as its
// reference ("escaped").
func hideMiddleware(strategy string, padding float32) middleware {
	return middleware{name: "hide", fn: func(s *state) result {
		switch strategy {
		case "referenceHidden":
			overflow := detectOverflow(s, overflowOptions{padding: uniformPadding(padding), reference: true})
			offsets := getSideOffsets(overflow, s.rects.reference)
			s.data.hide.hasReference = true
			s.data.hide.referenceHiddenOffsets = offsets
			s.data.hide.referenceHidden = isAnySideFullyClipped(offsets)
		case "escaped":
			overflow := detectOverflow(s, overflowOptions{padding: uniformPadding(padding), altBoundary: true})
			offsets := getSideOffsets(overflow, s.rects.floating)
			s.data.hide.hasEscaped = true
			s.data.hide.escapedOffsets = offsets
			s.data.hide.escaped = isAnySideFullyClipped(offsets)
		}
		return result{}
	}}
}

// ---------------------------------------------------------------------
// size

// sizeMiddleware provides the space available to the floating element
// (Floating UI's size) and calls apply with it. The limits that apply asks
// for go to lim, cleared at every run; they reach the engine in the result.
func sizeMiddleware(padding float32, apply func(Available), lim *sizeLimits) middleware {
	return middleware{name: "size", fn: func(s *state) result {
		*lim = sizeLimits{}
		overflow := detectOverflow(s, overflowOptions{padding: uniformPadding(padding)})
		side := getSide(s.placement)
		alignment := getAlignment(s.placement)
		isYAxis := getSideAxis(s.placement) == axisY
		width, height := s.rects.floating.width, s.rects.floating.height
		var heightSide, widthSide string
		if side == "top" || side == "bottom" {
			heightSide = side
			rtlAlign := "end"
			if s.env.rtl {
				rtlAlign = "start"
			}
			if alignment == rtlAlign {
				widthSide = "left"
			} else {
				widthSide = "right"
			}
		} else {
			widthSide = side
			if alignment == "end" {
				heightSide = "top"
			} else {
				heightSide = "bottom"
			}
		}
		maximumClippingHeight := height - overflow.top - overflow.bottom
		maximumClippingWidth := width - overflow.left - overflow.right
		overflowAvailableHeight := min(height-overflow.side(heightSide), maximumClippingHeight)
		overflowAvailableWidth := min(width-overflow.side(widthSide), maximumClippingWidth)
		sh := s.data.shift
		noShift := !sh.set
		availableHeight := overflowAvailableHeight
		availableWidth := overflowAvailableWidth
		if sh.set && sh.enabledX {
			availableWidth = maximumClippingWidth
		}
		if sh.set && sh.enabledY {
			availableHeight = maximumClippingHeight
		}
		if noShift && alignment == "" {
			xMin := max(overflow.left, 0)
			xMax := max(overflow.right, 0)
			yMin := max(overflow.top, 0)
			yMax := max(overflow.bottom, 0)
			if isYAxis {
				w := max(overflow.left, overflow.right)
				if xMin != 0 || xMax != 0 {
					w = xMin + xMax
				}
				availableWidth = width - 2*w
			} else {
				h := max(overflow.top, overflow.bottom)
				if yMin != 0 || yMax != 0 {
					h = yMin + yMax
				}
				availableHeight = height - 2*h
			}
		}
		if apply != nil {
			apply(Available{
				Width: availableWidth, Height: availableHeight,
				Rects: Rects{
					Reference: tela.Rect{X: s.rects.reference.x, Y: s.rects.reference.y, Width: s.rects.reference.width, Height: s.rects.reference.height},
					Floating:  tela.Rect{X: s.x, Y: s.y, Width: width, Height: height},
				},
				Placement: s.placement,
				sink:      lim,
			})
		}
		return result{}
	}}
}
