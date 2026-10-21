// Port of @floating-ui/core (computePosition, computeCoordsFromPlacement,
// detectOverflow) and of the placement helpers of @floating-ui/utils.
//
// Copyright (c) 2021-present Floating UI contributors. MIT License; the
// full notice is in THIRD_PARTY.md.
//
// The port keeps Floating UI's structure and names (middleware that return
// coordinates, data and a reset; a state with the middleware data of the
// current run) with these differences: it is synchronous (no platform
// object: the engine of package core supplies the rectangles in a
// FloatInput), it works in float32 window coordinates with the "fixed"
// strategy, and a middleware cannot measure the floating element again
// (Floating UI's reset with rects): the size middleware records the limits
// and the engine lays the element out again and calls the position
// function again with the new size.

package aria

import (
	"slices"
	"strings"
)

// rect is Floating UI's Rect: a position and a size, in window pixels.
type rect struct{ x, y, width, height float32 }

func (r rect) right() float32  { return r.x + r.width }
func (r rect) bottom() float32 { return r.y + r.height }

// pos returns the coordinate of r along axis a (Floating UI's rect[axis]).
func (r rect) pos(a axis) float32 {
	if a == axisY {
		return r.y
	}
	return r.x
}

// length returns the size of r along axis a (rect[getAxisLength(axis)]).
func (r rect) length(a axis) float32 {
	if a == axisY {
		return r.height
	}
	return r.width
}

// coords is a position (Floating UI's Coords).
type coords struct{ x, y float32 }

func (c coords) get(a axis) float32 {
	if a == axisY {
		return c.y
	}
	return c.x
}

func (c *coords) set(a axis, v float32) {
	if a == axisY {
		c.y = v
	} else {
		c.x = v
	}
}

// elementRects are the rectangles of the reference and the floating element.
type elementRects struct{ reference, floating rect }

// sideOffsets are overflows (or padding) per side (Floating UI's
// SideObject).
type sideOffsets struct{ top, right, bottom, left float32 }

// side returns the offset of the named side ("top", "right", "bottom" or
// "left").
func (s sideOffsets) side(name string) float32 {
	switch name {
	case "top":
		return s.top
	case "right":
		return s.right
	case "bottom":
		return s.bottom
	}
	return s.left
}

type axis uint8

const (
	axisX axis = iota
	axisY
)

func oppositeAxis(a axis) axis {
	if a == axisX {
		return axisY
	}
	return axisX
}

// ---------------------------------------------------------------------
// Placements (@floating-ui/utils).

// allPlacements are the 12 placements, in Floating UI's order.
var allPlacements = []string{
	"top", "top-start", "top-end",
	"right", "right-start", "right-end",
	"bottom", "bottom-start", "bottom-end",
	"left", "left-start", "left-end",
}

func validPlacement(p string) bool { return slices.Contains(allPlacements, p) }

func getSide(placement string) string {
	side, _, _ := strings.Cut(placement, "-")
	return side
}

// getAlignment returns "start", "end" or "" (centered).
func getAlignment(placement string) string {
	_, a, _ := strings.Cut(placement, "-")
	return a
}

func getSideAxis(placement string) axis {
	if placement[0] == 't' || placement[0] == 'b' {
		return axisY
	}
	return axisX
}

func getAlignmentAxis(placement string) axis { return oppositeAxis(getSideAxis(placement)) }

var oppositeSide = map[string]string{"left": "right", "right": "left", "bottom": "top", "top": "bottom"}

func getOppositePlacement(placement string) string {
	side := getSide(placement)
	return oppositeSide[side] + placement[len(side):]
}

func getOppositeAlignmentPlacement(placement string) string {
	if strings.Contains(placement, "start") {
		return strings.Replace(placement, "start", "end", 1)
	}
	return strings.Replace(placement, "end", "start", 1)
}

func getExpandedPlacements(placement string) []string {
	opposite := getOppositePlacement(placement)
	return []string{getOppositeAlignmentPlacement(placement), opposite, getOppositeAlignmentPlacement(opposite)}
}

// getAlignmentSides returns the side the alignment starts from and the
// opposite one.
func getAlignmentSides(placement string, r elementRects, rtl bool) [2]string {
	alignment := getAlignment(placement)
	ax := getAlignmentAxis(placement)
	var main string
	if ax == axisX {
		startSide := "start"
		if rtl {
			startSide = "end"
		}
		if alignment == startSide {
			main = "right"
		} else {
			main = "left"
		}
	} else if alignment == "start" {
		main = "bottom"
	} else {
		main = "top"
	}
	if r.reference.length(ax) > r.floating.length(ax) {
		main = getOppositePlacement(main)
	}
	return [2]string{main, getOppositePlacement(main)}
}

func getSideList(side string, isStart, rtl bool) []string {
	switch side {
	case "top", "bottom":
		if rtl {
			if isStart {
				return []string{"right", "left"}
			}
			return []string{"left", "right"}
		}
		if isStart {
			return []string{"left", "right"}
		}
		return []string{"right", "left"}
	case "left", "right":
		if isStart {
			return []string{"top", "bottom"}
		}
		return []string{"bottom", "top"}
	}
	return nil
}

// getOppositeAxisPlacements lists the placements on the other axis, for
// flip's fallbackAxisSideDirection (direction is "start" or "end").
func getOppositeAxisPlacements(placement string, flipAlignment bool, direction string, rtl bool) []string {
	alignment := getAlignment(placement)
	list := getSideList(getSide(placement), direction == "start", rtl)
	if alignment != "" {
		for i, s := range list {
			list[i] = s + "-" + alignment
		}
		if flipAlignment {
			n := len(list)
			for i := 0; i < n; i++ {
				list = append(list, getOppositeAlignmentPlacement(list[i]))
			}
		}
	}
	return list
}

// ---------------------------------------------------------------------
// computeCoordsFromPlacement.

func computeCoordsFromPlacement(r elementRects, placement string, rtl bool) coords {
	sideAxis := getSideAxis(placement)
	alignmentAxis := getAlignmentAxis(placement)
	side := getSide(placement)
	isVertical := sideAxis == axisY
	ref, fl := r.reference, r.floating
	commonX := ref.x + ref.width/2 - fl.width/2
	commonY := ref.y + ref.height/2 - fl.height/2
	commonAlign := ref.length(alignmentAxis)/2 - fl.length(alignmentAxis)/2
	var c coords
	switch side {
	case "top":
		c = coords{commonX, ref.y - fl.height}
	case "bottom":
		c = coords{commonX, ref.y + ref.height}
	case "right":
		c = coords{ref.x + ref.width, commonY}
	case "left":
		c = coords{ref.x - fl.width, commonY}
	default:
		c = coords{ref.x, ref.y}
	}
	sign := float32(1)
	if rtl && isVertical {
		sign = -1
	}
	switch getAlignment(placement) {
	case "start":
		c.set(alignmentAxis, c.get(alignmentAxis)-commonAlign*sign)
	case "end":
		c.set(alignmentAxis, c.get(alignmentAxis)+commonAlign*sign)
	}
	return c
}

// ---------------------------------------------------------------------
// The platform (what Floating UI asks of it) and detectOverflow.

// environment is what the position function knows about the world, from
// the engine's FloatInput.
type environment struct {
	// reference is the anchor's border box; floating the size of the
	// floating element (rects.floating, at the origin).
	reference rect
	floating  rect
	// clip is the clipping rectangle of the floating element, referenceClip
	// the one of the reference (Floating UI's clippingAncestors within the
	// viewport, for each of them).
	clip, referenceClip rect
	// rtl is Floating UI's platform.isRTL.
	rtl bool
	// arrow is the size of the arrow element; hasArrow reports one.
	arrow    rect
	hasArrow bool
}

// overflowOptions are the options of detectOverflow that Tela supports:
// the boundary is always the clipping rectangle of an element and the root
// boundary the window.
type overflowOptions struct {
	// padding keeps the element that far from the boundary.
	padding sideOffsets
	// reference selects elementContext "reference" (the reference rectangle
	// against the boundary of its own clipping ancestors); altBoundary the
	// boundary of the other element.
	reference   bool
	altBoundary bool
}

// detectOverflow returns how much the element (the floating element at
// state.x, state.y, or the reference) overflows its clipping boundary on
// each side: positive is overflowing by that many pixels, negative how many
// are left (Floating UI's detectOverflow).
func detectOverflow(s *state, o overflowOptions) sideOffsets {
	// element is the element whose clipping ancestors give the boundary:
	// elements[altBoundary ? altContext : elementContext].
	boundaryOfReference := o.reference
	if o.altBoundary {
		boundaryOfReference = !boundaryOfReference
	}
	clip := s.env.clip
	if boundaryOfReference {
		clip = s.env.referenceClip
	}
	el := s.rects.reference
	if !o.reference {
		el = rect{s.x, s.y, s.rects.floating.width, s.rects.floating.height}
	}
	return sideOffsets{
		top:    clip.y - el.y + o.padding.top,
		bottom: el.bottom() - clip.bottom() + o.padding.bottom,
		left:   clip.x - el.x + o.padding.left,
		right:  el.right() - clip.right() + o.padding.right,
	}
}

func uniformPadding(p float32) sideOffsets { return sideOffsets{p, p, p, p} }

// ---------------------------------------------------------------------
// computePosition.

// maxResetCount is the number of resets that can occur before bailing to
// avoid infinite reset loops.
const maxResetCount = 50

// middlewareData is Floating UI's middlewareData: what each middleware
// stored while the position was computed (merged field by field across
// resets, like `{...middlewareData[name], ...data}`).
type middlewareData struct {
	arrow         arrowData
	autoPlacement placementData
	flip          placementData
	hide          hideData
	offset        offsetData
	shift         shiftData
}

// placementData is the data of flip and autoPlacement: the index of the
// placement being tried and the overflows of the ones tried.
type placementData struct {
	index     int
	overflows []placementOverflow
}

// placementOverflow is the overflow of one placement checked by flip or
// autoPlacement.
type placementOverflow struct {
	placement string
	overflows []float32
}

// state is Floating UI's MiddlewareState.
type state struct {
	x, y             float32
	initialPlacement string
	placement        string
	rects            elementRects
	data             *middlewareData
	env              *environment
}

// resetKind says how a middleware asks computePosition to start again.
type resetKind uint8

const (
	noReset resetKind = iota
	// resetAll runs every middleware again with the current coordinates
	// (`reset: true`).
	resetAll
	// resetPlacement does so with another placement, whose coordinates are
	// computed again (`reset: {placement}`).
	resetPlacement
)

// result is what a middleware returns (Floating UI's MiddlewareReturn):
// new coordinates, set or not, and a reset.
type result struct {
	setX, setY bool
	x, y       float32
	reset      resetKind
	placement  string
}

func (r *result) setCoord(a axis, v float32) {
	if a == axisY {
		r.setY, r.y = true, v
	} else {
		r.setX, r.x = true, v
	}
}

// middleware is one entry of the middleware array.
type middleware struct {
	name string
	fn   func(s *state) result
}

// position is the outcome of computePosition.
type position struct {
	x, y      float32
	placement string
	data      middlewareData
}

// computePosition computes the coordinates that place the floating element
// next to the reference (Floating UI's computePosition with the fixed
// strategy), running the middleware in order.
func computePosition(env *environment, placement string, mws []middleware) position {
	rects := elementRects{reference: env.reference, floating: env.floating}
	c := computeCoordsFromPlacement(rects, placement, env.rtl)
	stateful := placement
	resetCount := 0
	var data middlewareData
	for i := 0; i < len(mws); i++ {
		s := state{
			x: c.x, y: c.y,
			initialPlacement: placement,
			placement:        stateful,
			rects:            rects,
			data:             &data,
			env:              env,
		}
		r := mws[i].fn(&s)
		if r.setX {
			c.x = r.x
		}
		if r.setY {
			c.y = r.y
		}
		if r.reset != noReset && resetCount < maxResetCount {
			resetCount++
			if r.reset == resetPlacement {
				if r.placement != "" {
					stateful = r.placement
				}
				c = computeCoordsFromPlacement(rects, stateful, env.rtl)
			}
			i = -1
		}
	}
	return position{x: c.x, y: c.y, placement: stateful, data: data}
}

// sizeLimits are the limits the apply function of one size middleware asked
// for in its last run (the last call per dimension wins).
type sizeLimits struct {
	maxWidth, maxHeight       float32
	hasMaxWidth, hasMaxHeight bool
}

func clamp(start, value, end float32) float32 { return max(start, min(value, end)) }
