package aria

import (
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// Floating is what [UseFloating] returns: the props of the floating
// element and of its arrow, and what the position computed.
//
// Everything the position decides is known in the frame that shows the
// element: Props and ArrowProps move it and the arrow while the frame is
// painted, so no frame ever shows them in the wrong place. A component
// builds again, to style itself by Placement, in the frame after.
type Floating struct {
	// Props go on the floating element: they make it fixed to the window at
	// its top-left corner (a later class such as "absolute" in the element's
	// Class wins) and place it next to its anchor. Put the element in a
	// Portal so that no ancestor clips it.
	Props tela.Prop
	// ArrowProps go on the arrow element (the one whose ElementRef was given
	// to [Arrow]): they make it absolute at the top-left corner of its
	// parent, the floating element, and move it so that it points at the
	// anchor, half of it outside the floating element's border box on the
	// side that faces the anchor. Without the [Arrow] middleware they do
	// nothing but make it absolute.
	ArrowProps tela.Prop
	// Placement is the placement the floating element is in, after flip or
	// autoPlacement chose it: one of Floating UI's 12 placements. It is the
	// placement given to [Placement] (default "bottom") until the first
	// position is computed, and follows the engine by one frame: use it to
	// style, and read ArrowX and the like where the same frame matters.
	Placement string
	// Side and Align split Placement: Side is "top", "right", "bottom" or
	// "left", the side of the anchor the element is on; Align is "start",
	// "end" or "" (centered).
	Side, Align string
	// ArrowX and ArrowY are Floating UI's arrow data: the offset of the
	// arrow along the floating element's edge that faces the anchor, from the
	// element's left edge (ArrowX, for the top and bottom sides) or top edge
	// (ArrowY, for left and right); the other one is 0. They are valid in the
	// frame where they change: read them in a paint-phase closure such as
	// tela.TranslateFn. Zero without the [Arrow] middleware.
	ArrowX, ArrowY *tela.Value[float32]
	// ReferenceHidden reports that the anchor is scrolled out of view (the
	// [Hide] middleware, strategy "referenceHidden"); Escaped that the
	// floating element is out of the area where the anchor can be seen
	// (strategy "escaped"). They are false without [Hide], and an anchor that
	// is not displayed counts as hidden. Read them in a paint-phase closure
	// to follow the same frame:
	//
	//	Style(tela.OpacityFn(tela.MapValue(fl.ReferenceHidden, func(hidden bool) float32 {
	//	    if hidden { return 0 }
	//	    return 1
	//	})))
	ReferenceHidden, Escaped *tela.Value[bool]
}

// arrowPlace is where the arrow goes: its translation from the top-left
// corner of the floating element.
type arrowPlace struct{ x, y float32 }

// floatingPublish holds the Values that the position function publishes.
type floatingPublish struct {
	placement       *tela.Value[string]
	arrowX, arrowY  *tela.Value[float32]
	arrowPlace      *tela.Value[arrowPlace]
	referenceHidden *tela.Value[bool]
	escaped         *tela.Value[bool]
}

// UseFloating positions a floating element (a popover, tooltip, menu or
// select list) next to its anchor: Floating UI's computePosition with its
// middleware, run by Tela's frame pipeline (ADR-099). It is a hook: call it
// at the top level of a component.
//
//	func Popover(open bool) tela.Node {
//	    anchor := tela.UseElementRef()
//	    arrow := tela.UseElementRef()
//	    fl := aria.UseFloating(anchor, aria.Placement("bottom-start"), aria.Offset(8),
//	        aria.Flip(), aria.Shift(8), aria.Arrow(arrow))
//	    return Fragment(
//	        Button(Ref(anchor), Text("Open")),
//	        If(open, Portal(
//	            Div(fl.Props, Class("w-64 rounded-md border bg-popover p-4 shadow-md"),
//	                Text("Content"),
//	                Div(Ref(arrow), fl.ArrowProps, Class("size-2 rotate-45 border bg-popover"))))))
//	}
//
// The anchor is the element that carries the ref (h.Ref), or, when anchor
// is the zero tela.ElementRef, the virtual rectangle that [AnchorRect]
// returns, such as the pointer of a context menu. The options configure the
// placement and the middleware, which run in the order given, as in
// Floating UI's middleware array, and the order matters: offset before
// flip and shift, arrow last.
//
// The element is placed in the paint phase of the frame that shows it, on
// the anchor as painted: it follows its anchor when an ancestor scrolls or
// a transform moves it, without a frame of its own, and nothing runs while
// nothing changes. The options are read again on every build of the
// component.
func UseFloating(anchor tela.ElementRef, opts ...FloatingOption) Floating {
	cfg := newFloatingConfig(opts)
	if anchor == (tela.ElementRef{}) && cfg.anchorRect == nil && core.DevNow() {
		panic("aria: UseFloating(tela.ElementRef{}) has no anchor: pass the ref attached to the anchor element with h.Ref (from tela.UseElementRef), or give a virtual anchor with aria.AnchorRect(func() tela.Rect { ... })")
	}
	pub := floatingPublish{
		placement:       tela.UseValue(cfg.placement),
		arrowX:          tela.UseValue(float32(0)),
		arrowY:          tela.UseValue(float32(0)),
		arrowPlace:      tela.UseValue(arrowPlace{}),
		referenceHidden: tela.UseValue(false),
		escaped:         tela.UseValue(false),
	}
	placement := pub.placement.Get()
	if !validPlacement(placement) {
		placement = cfg.placement
	}
	engine := core.Floating(core.FloatOptions{
		Anchor:     anchor,
		AnchorRect: cfg.anchorRect,
		Arrow:      cfg.arrow,
		Place:      func(in core.FloatInput) core.FloatResult { return cfg.place(in, &pub) },
	})
	arrowProps := core.MergeProps(
		h.Class("absolute top-0 left-0"),
		h.Style(tela.TranslateFn(func() (x, y float32) {
			p := pub.arrowPlace.Get()
			return p.x, p.y
		})),
	)
	return Floating{
		Props:           core.MergeProps(h.Class("fixed top-0 left-0"), engine),
		ArrowProps:      arrowProps,
		Placement:       placement,
		Side:            getSide(placement),
		Align:           getAlignment(placement),
		ArrowX:          pub.arrowX,
		ArrowY:          pub.arrowY,
		ReferenceHidden: pub.referenceHidden,
		Escaped:         pub.escaped,
	}
}

// ---------------------------------------------------------------------
// Options.

// floatingConfig is the configuration the options build.
type floatingConfig struct {
	placement  string
	anchorRect func() tela.Rect
	arrow      tela.ElementRef
	mws        []middleware
	sizeLims   []*sizeLimits
	usesHide   bool
}

func newFloatingConfig(opts []FloatingOption) *floatingConfig {
	c := &floatingConfig{placement: "bottom"}
	for _, o := range opts {
		if o.apply != nil {
			o.apply(c)
		}
	}
	return c
}

// FloatingOption configures [UseFloating]: the placement, a virtual anchor
// or a middleware. Build them with [Placement], [AnchorRect], [Offset],
// [Flip], [Shift], [Size], [Arrow], [Hide] and [AutoPlacement].
type FloatingOption struct {
	apply func(*floatingConfig)
}

// bad reports a mistake in an option: it panics in dev mode, with the fix,
// and returns false otherwise so that the caller ignores the option.
func bad(format string, args ...any) bool {
	if core.DevNow() {
		panic("aria: " + fmt.Sprintf(format, args...))
	}
	return false
}

// checkPlacement validates a placement given to function fn.
func checkPlacement(fn, p string) bool {
	if validPlacement(p) {
		return true
	}
	hint := ""
	if s := closest(p, allPlacements); s != "" {
		hint = fmt.Sprintf(" Did you mean %q?", s)
	}
	return bad("%s: unknown placement %q.%s The placements are %s", fn, p, hint, strings.Join(allPlacements, ", "))
}

// closest returns the member of list nearest to s by edit distance, or ""
// when none is near.
func closest(s string, list []string) string {
	best, bestD := "", max(2, len(s)/3)+1
	for _, c := range list {
		if d := editDistance(strings.ToLower(s), c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// one returns the only options struct of a variadic parameter.
func one[T any](fn string, opts []T) (o T) {
	switch len(opts) {
	case 0:
	case 1:
		o = opts[0]
	default:
		bad("%s: takes at most one options struct, got %d", fn, len(opts))
		o = opts[0]
	}
	return o
}

// Placement sets the placement the floating element goes in, one of
// Floating UI's 12: "top", "top-start", "top-end", "right", "right-start",
// "right-end", "bottom" (the default), "bottom-start", "bottom-end",
// "left", "left-start" and "left-end". The side is where the element goes
// relative to the anchor; "start" and "end" align it with that edge of the
// anchor, mirrored in a right-to-left element (h.Dir("rtl")), as in
// Floating UI. An unknown placement panics in dev mode, with the closest
// valid one, and is ignored otherwise.
func Placement(placement string) FloatingOption {
	ok := checkPlacement(fmt.Sprintf("Placement(%q)", placement), placement)
	return FloatingOption{func(c *floatingConfig) {
		if ok {
			c.placement = placement
		}
	}}
}

// AnchorRect makes the anchor a virtual rectangle (Floating UI's virtual
// elements), for a context menu at the pointer or a popover at a text
// selection. rect returns it in window coordinates (for the pointer, a
// Rect of zero size at its position). It runs in the paint phase of the
// frames that place the element; a tela.Value it reads with Get places the
// element again when the value changes. It is used when the anchor given to
// [UseFloating] is the zero tela.ElementRef:
//
//	pos := tela.UseValue(tela.Rect{})
//	fl := aria.UseFloating(tela.ElementRef{}, aria.Placement("bottom-start"), aria.Flip(),
//	    aria.AnchorRect(pos.Get))
func AnchorRect(rect func() tela.Rect) FloatingOption {
	if rect == nil {
		bad("AnchorRect(nil): pass the func that returns the virtual anchor's Rect in window coordinates, such as func() tela.Rect { return tela.Rect{X: x, Y: y} }")
		return FloatingOption{}
	}
	return FloatingOption{func(c *floatingConfig) { c.anchorRect = rect }}
}

// OffsetOptions are the options of [Offset] beyond the distance.
type OffsetOptions struct {
	// CrossAxis moves the element along the anchor's edge, so that it
	// skids away from the alignment (Floating UI's crossAxis). Positive
	// moves it right for the top and bottom sides, down for the left and
	// right ones (mirrored in a right-to-left element for the top and
	// bottom sides).
	CrossAxis float32
	// AlignmentAxis replaces CrossAxis for aligned placements ("-start",
	// "-end") when HasAlignmentAxis is set: positive moves the element
	// toward the inside of the anchor's edge from the start and, negated,
	// from the end (Floating UI's alignmentAxis, which is a number or null).
	AlignmentAxis    float32
	HasAlignmentAxis bool
}

// Offset moves the element mainAxis pixels away from the anchor, along the
// side it is on (Floating UI's offset): the gap between a popover and its
// button. Give it before [Flip] and [Shift] so that they see the final
// distance. The options skid it along the anchor's edge.
func Offset(mainAxis float32, opts ...OffsetOptions) FloatingOption {
	o := one("Offset", opts)
	mw := offsetMiddleware(offsetParams{
		mainAxis: mainAxis, crossAxis: o.CrossAxis,
		alignmentAxis: o.AlignmentAxis, hasAlignmentAxis: o.HasAlignmentAxis,
	})
	return FloatingOption{func(c *floatingConfig) { c.mws = append(c.mws, mw) }}
}

// FlipOptions are the options of [Flip]; the zero value is Floating UI's
// defaults.
type FlipOptions struct {
	// Padding is the space to keep between the element and the edges of the
	// area it must stay in (the window, or the visible part of the scroll
	// containers around it when it is not in a Portal).
	Padding float32
	// NoMainAxis stops flip from checking the overflow on the side of the
	// placement (Floating UI's mainAxis: false); NoCrossAxis on the
	// alignment axis (crossAxis: false).
	NoMainAxis, NoCrossAxis bool
	// CrossAxisAlignment is Floating UI's crossAxis: 'alignment': the
	// overflow on the alignment axis only counts while the next placement
	// is on the same axis as the initial one, so the element changes side
	// only when every placement on its side overflows.
	CrossAxisAlignment bool
	// FallbackPlacements are the placements to try, in order, after the
	// initial one. Nil tries the opposite one (and, for an aligned
	// placement, its other alignments first).
	FallbackPlacements []string
	// FallbackStrategy is what to do when no placement fits: "bestFit" (the
	// default) takes the one that overflows least, "initialPlacement" goes
	// back to the initial one.
	FallbackStrategy string
	// FallbackAxisSideDirection adds the placements on the other axis to
	// the fallbacks when FallbackPlacements is nil: "none" (the default),
	// "start" or "end", the side tried first.
	FallbackAxisSideDirection string
	// NoFlipAlignment stops an aligned placement from trying its other
	// alignment (Floating UI's flipAlignment: false).
	NoFlipAlignment bool
}

// Flip flips the element to the opposite side when its placement would
// overflow the area it must stay in (Floating UI's flip): a popover below
// a button near the bottom of the window goes above it. It checks the
// placements in order and keeps the first that fits. Give it after
// [Offset] and before [Shift].
func Flip(opts ...FlipOptions) FloatingOption {
	o := one("Flip", opts)
	p := flipParams{
		padding:  o.Padding,
		mainAxis: !o.NoMainAxis, crossAxis: !o.NoCrossAxis,
		crossAxisAlignment:        o.CrossAxisAlignment && !o.NoCrossAxis,
		fallbackStrategy:          "bestFit",
		fallbackAxisSideDirection: "none",
		flipAlignment:             !o.NoFlipAlignment,
	}
	if o.FallbackPlacements != nil {
		p.hasFallbackPlacements = true
		for _, fp := range o.FallbackPlacements {
			if checkPlacement("Flip: FallbackPlacements", fp) {
				p.fallbackPlacements = append(p.fallbackPlacements, fp)
			}
		}
	}
	switch o.FallbackStrategy {
	case "":
	case "bestFit", "initialPlacement":
		p.fallbackStrategy = o.FallbackStrategy
	default:
		bad("Flip: FallbackStrategy %q is unknown: use \"bestFit\" (the default) or \"initialPlacement\"", o.FallbackStrategy)
	}
	switch o.FallbackAxisSideDirection {
	case "":
	case "none", "start", "end":
		p.fallbackAxisSideDirection = o.FallbackAxisSideDirection
	default:
		bad("Flip: FallbackAxisSideDirection %q is unknown: use \"none\" (the default), \"start\" or \"end\"", o.FallbackAxisSideDirection)
	}
	mw := flipMiddleware(p)
	return FloatingOption{func(c *floatingConfig) { c.mws = append(c.mws, mw) }}
}

// ShiftLimiter limits how far [Shift] moves the element. Build one with
// [LimitShift]; the zero value limits nothing.
type ShiftLimiter struct {
	fn func(s *state) coords
}

// LimitShiftOptions are the options of [LimitShift]; the zero value is
// Floating UI's defaults.
type LimitShiftOptions struct {
	// Offset is how far the element may leave the anchor's span along the
	// main axis of the shift: positive keeps it that many pixels attached
	// (Floating UI's offset, as a number or its mainAxis).
	Offset float32
	// CrossAxisOffset is the same along the other axis (offset.crossAxis).
	CrossAxisOffset float32
	// NoMainAxis and NoCrossAxis turn the limit off on an axis (mainAxis:
	// false, crossAxis: false).
	NoMainAxis, NoCrossAxis bool
}

// LimitShift returns the limiter that keeps the element attached to the
// anchor while it shifts: the element stops following the window's edge
// when it would no longer touch the anchor (Floating UI's limitShift).
//
//	aria.Shift(8, aria.ShiftOptions{Limiter: aria.LimitShift()})
func LimitShift(opts ...LimitShiftOptions) ShiftLimiter {
	o := one("LimitShift", opts)
	return ShiftLimiter{fn: limitShiftMiddleware(o.Offset, o.CrossAxisOffset, !o.NoMainAxis, !o.NoCrossAxis)}
}

// ShiftOptions are the options of [Shift]; the zero value is Floating UI's
// defaults.
type ShiftOptions struct {
	// NoMainAxis stops the shift along the side's axis (the one that keeps
	// the element in view: x for the top and bottom sides; Floating UI's
	// mainAxis: false).
	NoMainAxis bool
	// CrossAxis also shifts it along the other axis, which would move it
	// into the anchor (crossAxis: true).
	CrossAxis bool
	// Limiter limits the shift; see [LimitShift].
	Limiter ShiftLimiter
}

// Shift keeps the element in view by sliding it along the anchor's edge
// when it overflows the window (Floating UI's shift), padding pixels from
// the edges. It does not change the side: give it after [Flip].
func Shift(padding float32, opts ...ShiftOptions) FloatingOption {
	o := one("Shift", opts)
	mw := shiftMiddleware(shiftParams{
		padding: padding, mainAxis: !o.NoMainAxis, crossAxis: o.CrossAxis, limiter: o.Limiter,
	})
	return FloatingOption{func(c *floatingConfig) { c.mws = append(c.mws, mw) }}
}

// Rects are the rectangles of the anchor and the floating element, in
// window coordinates.
type Rects struct {
	// Reference is the anchor's border box.
	Reference tela.Rect
	// Floating is the floating element's border box at the position the
	// middleware before Size left it at.
	Floating tela.Rect
}

// Available is what [Size] gives its apply function: the room the floating
// element has and the means to limit it.
type Available struct {
	// Width and Height are the largest size the floating element's border
	// box can take without leaving the area it must stay in, on the side of
	// the anchor it is on (Floating UI's availableWidth and availableHeight).
	// Negative when there is no room at all.
	Width, Height float32
	// Rects are the rectangles of the anchor and the floating element.
	Rects Rects
	// Placement is the placement the size is computed for.
	Placement string

	sink *sizeLimits
}

// MaxWidth limits the floating element's border-box width to w: the smaller
// of w and the element's own max-width wins, and the element is laid out
// again within the frame. Not calling it removes the limit.
func (a Available) MaxWidth(w float32) {
	if a.sink != nil {
		a.sink.maxWidth, a.sink.hasMaxWidth = w, true
	}
}

// MaxHeight limits the floating element's border-box height to h, like
// [Available.MaxWidth]. An element with overflow-y-auto then scrolls its
// content.
func (a Available) MaxHeight(h float32) {
	if a.sink != nil {
		a.sink.maxHeight, a.sink.hasMaxHeight = h, true
	}
}

// SizeOptions are the options of [Size].
type SizeOptions struct {
	// Padding is the space to keep between the element and the edges of
	// the area it must stay in.
	Padding float32
}

// Size gives apply the room the floating element has (Floating UI's size),
// so that a long list scrolls inside the window instead of leaving it.
// apply calls the methods of its argument to limit the element; the limits
// are applied in the same frame, with no flash of the unlimited element:
//
//	aria.Size(func(a aria.Available) {
//	    a.MaxHeight(a.Height)
//	})
//
// Put the element's scrolling content in an element with a bounded height
// (a flex column with min-h-0 and overflow-y-auto), or give the floating
// element itself overflow-y-auto. apply runs in the paint phase: it must be
// quick and must not set state. A limit that apply does not ask for in a
// run is removed.
func Size(apply func(Available), opts ...SizeOptions) FloatingOption {
	o := one("Size", opts)
	if apply == nil {
		bad("Size(nil): pass the func that limits the element, such as func(a aria.Available) { a.MaxHeight(a.Height) }")
		return FloatingOption{}
	}
	return FloatingOption{func(c *floatingConfig) {
		// The limits belong to the configuration: they are read after the
		// position was computed.
		lim := new(sizeLimits)
		c.sizeLims = append(c.sizeLims, lim)
		c.mws = append(c.mws, sizeMiddleware(o.Padding, apply, lim))
	}}
}

// ArrowOptions are the options of [Arrow].
type ArrowOptions struct {
	// Padding keeps the arrow that many pixels from the corners of the
	// floating element (Floating UI's padding).
	Padding float32
}

// Arrow computes where the arrow element points at the anchor (Floating
// UI's arrow). ref is the ref of the arrow element, attached with h.Ref
// next to the Floating's ArrowProps. Give it last, after Flip and Shift.
func Arrow(ref tela.ElementRef, opts ...ArrowOptions) FloatingOption {
	o := one("Arrow", opts)
	if ref == (tela.ElementRef{}) {
		bad("Arrow(tela.ElementRef{}): pass the ref of the arrow element, from tela.UseElementRef, and attach it to the arrow with h.Ref")
		return FloatingOption{}
	}
	mw := arrowMiddleware(uniformPadding(o.Padding))
	return FloatingOption{func(c *floatingConfig) {
		c.arrow = ref
		c.mws = append(c.mws, mw)
	}}
}

// HideOptions are the options of [Hide].
type HideOptions struct {
	// Strategy is "referenceHidden" (the default: the anchor is out of view)
	// or "escaped" (the floating element is out of the area where the anchor
	// can be seen). Give Hide twice to track both.
	Strategy string
	// Padding is the space to keep around the area.
	Padding float32
}

// Hide tracks whether the anchor is scrolled out of view (Floating UI's
// hide), published as [Floating].ReferenceHidden and [Floating].Escaped.
func Hide(opts ...HideOptions) FloatingOption {
	o := one("Hide", opts)
	strategy := o.Strategy
	switch strategy {
	case "":
		strategy = "referenceHidden"
	case "referenceHidden", "escaped":
	default:
		bad("Hide: Strategy %q is unknown: use \"referenceHidden\" (the default) or \"escaped\"", strategy)
		return FloatingOption{}
	}
	mw := hideMiddleware(strategy, o.Padding)
	return FloatingOption{func(c *floatingConfig) {
		c.usesHide = true
		c.mws = append(c.mws, mw)
	}}
}

// AutoPlacementOptions are the options of [AutoPlacement]; the zero value is
// Floating UI's defaults.
type AutoPlacementOptions struct {
	// Padding is the space to keep between the element and the edges of the
	// area it must stay in.
	Padding float32
	// CrossAxis also counts the overflow on the alignment axis when it
	// chooses among aligned placements.
	CrossAxis bool
	// Alignment restricts the choice to placements with that alignment
	// ("start" or "end").
	Alignment string
	// AllowedPlacements are the placements it chooses among; nil is all.
	AllowedPlacements []string
	// NoAutoAlignment stops it from choosing the other alignment when
	// Alignment is set.
	NoAutoAlignment bool
}

// AutoPlacement chooses the placement with the most room, without a
// preferred one (Floating UI's autoPlacement). It is an alternative to
// [Flip]: do not give both.
func AutoPlacement(opts ...AutoPlacementOptions) FloatingOption {
	o := one("AutoPlacement", opts)
	p := autoPlacementParams{padding: o.Padding, crossAxis: o.CrossAxis, autoAlignment: !o.NoAutoAlignment}
	switch o.Alignment {
	case "", "start", "end":
		p.alignment = o.Alignment
	default:
		bad("AutoPlacement: Alignment %q is unknown: use \"start\" or \"end\"", o.Alignment)
	}
	if o.AllowedPlacements != nil {
		p.hasAllowed = true
		for _, ap := range o.AllowedPlacements {
			if checkPlacement("AutoPlacement: AllowedPlacements", ap) {
				p.allowedPlacements = append(p.allowedPlacements, ap)
			}
		}
	}
	mw := autoPlacementMiddleware(p)
	return FloatingOption{func(c *floatingConfig) { c.mws = append(c.mws, mw) }}
}

// ---------------------------------------------------------------------
// The position function.

func rectOf(r tela.Rect) rect { return rect{r.X, r.Y, r.Width, r.Height} }

// place is the Place function of the engine: Floating UI's computePosition
// on the rectangles of the frame, publishing what components render from.
func (c *floatingConfig) place(in core.FloatInput, pub *floatingPublish) core.FloatResult {
	if !in.Anchored {
		// The anchor is not displayed: leave the element where it is
		// (Floating UI does not compute without a reference).
		if c.usesHide {
			core.PublishValue(pub.referenceHidden, true)
		}
		return core.FloatResult{X: float32(math.NaN()), Y: float32(math.NaN())}
	}
	env := environment{
		reference:     rectOf(in.Anchor),
		floating:      rect{width: in.Floating.Width, height: in.Floating.Height},
		clip:          rectOf(in.Clip),
		referenceClip: rectOf(in.AnchorClip),
		rtl:           in.RTL,
		arrow:         rect{width: in.Arrow.Width, height: in.Arrow.Height},
		hasArrow:      in.Arrow.Width > 0 || in.Arrow.Height > 0,
	}
	pos := computePosition(&env, c.placement, c.mws)
	res := core.FloatResult{X: pos.x, Y: pos.y}
	for _, lim := range c.sizeLims {
		if lim.hasMaxWidth {
			res.MaxWidth, res.HasMaxWidth = lim.maxWidth, true
		}
		if lim.hasMaxHeight {
			res.MaxHeight, res.HasMaxHeight = lim.maxHeight, true
		}
	}
	core.PublishValue(pub.placement, pos.placement)
	if c.hasArrow() && env.hasArrow {
		var ax, ay float32 // Floating UI's arrow data, for the placement's side
		var at arrowPlace
		switch getSide(pos.placement) {
		case "bottom":
			ax, at = pos.data.arrow.x, arrowPlace{pos.data.arrow.x, -env.arrow.height / 2}
		case "top":
			ax, at = pos.data.arrow.x, arrowPlace{pos.data.arrow.x, env.floating.height - env.arrow.height/2}
		case "right":
			ay, at = pos.data.arrow.y, arrowPlace{-env.arrow.width / 2, pos.data.arrow.y}
		default: // left
			ay, at = pos.data.arrow.y, arrowPlace{env.floating.width - env.arrow.width/2, pos.data.arrow.y}
		}
		core.PublishValue(pub.arrowX, ax)
		core.PublishValue(pub.arrowY, ay)
		core.PublishValue(pub.arrowPlace, at)
	}
	if c.usesHide {
		core.PublishValue(pub.referenceHidden, pos.data.hide.referenceHidden)
		core.PublishValue(pub.escaped, pos.data.hide.escaped)
	}
	return res
}

func (c *floatingConfig) hasArrow() bool { return c.arrow != (tela.ElementRef{}) }
