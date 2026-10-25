package layout

import "math"

// This file holds the small value types the algorithms work with: an
// optional float (Taffy's Option<f32>), sizes and rects of floats, optional
// floats and available space, and their main/cross axis accessors.
//
// Determinism (ADR-027): every product that feeds a sum is rounded with an
// explicit float32 conversion, so the compiler cannot fuse it into a
// multiply-add; results are then bit-identical on every architecture.

var inf = float32(math.Inf(1))

// fmax returns the larger of a and b, ignoring a NaN operand (Rust's
// f32::max, which Taffy's code relies on).
func fmax(a, b float32) float32 {
	if a > b || b != b {
		return a
	}
	return b
}

// fmin returns the smaller of a and b, ignoring a NaN operand.
func fmin(a, b float32) float32 {
	if a < b || b != b {
		return a
	}
	return b
}

// round rounds half away from zero (Rust's f32::round).
func round(v float32) float32 { return float32(math.Round(float64(v))) }

// isNormal reports whether v is neither zero, subnormal, infinite nor NaN
// (Rust's f32::is_normal).
func isNormal(v float32) bool {
	a := math.Abs(float64(v))
	return a >= 0x1p-126 && a <= math.MaxFloat32
}

// opt is an optional float32.
type opt struct {
	v  float32
	ok bool
}

var none opt

func some(v float32) opt { return opt{v, true} }

// or returns o if set, else b.
func (o opt) or(b opt) opt {
	if o.ok {
		return o
	}
	return b
}

// unwrapOr returns o's value if set, else d.
func (o opt) unwrapOr(d float32) float32 {
	if o.ok {
		return o.v
	}
	return d
}

// or0 returns o's value or 0.
func (o opt) or0() float32 { return o.unwrapOr(0) }

// The maybe* methods follow Taffy's MaybeMath for Option<f32> with an
// Option<f32> operand: an unset operand leaves o unchanged, an unset o
// stays unset.

func (o opt) maybeMin(b opt) opt {
	if o.ok && b.ok {
		return some(fmin(o.v, b.v))
	}
	return o
}

func (o opt) maybeMax(b opt) opt {
	if o.ok && b.ok {
		return some(fmax(o.v, b.v))
	}
	return o
}

func (o opt) maybeClamp(min, max opt) opt {
	if !o.ok {
		return o
	}
	return some(fclampOpt(o.v, min, max))
}

// The *F methods apply a plain float operand to a set o.

func (o opt) maxF(b float32) opt {
	if o.ok {
		o.v = fmax(o.v, b)
	}
	return o
}

func (o opt) addF(b float32) opt {
	if o.ok {
		o.v += b
	}
	return o
}

func (o opt) subF(b float32) opt {
	if o.ok {
		o.v -= b
	}
	return o
}

// fclampOpt clamps v by optional bounds (Taffy's f32::maybe_clamp).
func fclampOpt(v float32, min, max opt) float32 {
	if max.ok {
		v = fmin(v, max.v)
	}
	if min.ok {
		v = fmax(v, min.v)
	}
	return v
}

// fmaxOpt returns max(v, b) if b is set.
func fmaxOpt(v float32, b opt) float32 {
	if b.ok {
		return fmax(v, b.v)
	}
	return v
}

// fminOpt returns min(v, b) if b is set.
func fminOpt(v float32, b opt) float32 {
	if b.ok {
		return fmin(v, b.v)
	}
	return v
}

// fsubOpt returns v - b if b is set.
func fsubOpt(v float32, b opt) float32 {
	if b.ok {
		return v - b.v
	}
	return v
}

// axis is an absolute axis.
type axis uint8

const (
	horizontal axis = iota
	vertical
)

func (d FlexDirection) mainAxis() axis {
	if d.isRow() {
		return horizontal
	}
	return vertical
}

func (d FlexDirection) crossAxis() axis {
	if d.isRow() {
		return vertical
	}
	return horizontal
}

// point is a 2D point or vector.
type point struct{ x, y float32 }

func (p point) main(dir FlexDirection) float32 {
	if dir.isRow() {
		return p.x
	}
	return p.y
}

func (p point) cross(dir FlexDirection) float32 {
	if dir.isRow() {
		return p.y
	}
	return p.x
}

// size is a width and a height.
type size struct{ w, h float32 }

func (s size) get(a axis) float32 {
	if a == horizontal {
		return s.w
	}
	return s.h
}

func (s size) main(dir FlexDirection) float32 {
	if dir.isRow() {
		return s.w
	}
	return s.h
}

func (s size) cross(dir FlexDirection) float32 {
	if dir.isRow() {
		return s.h
	}
	return s.w
}

func (s *size) setMain(dir FlexDirection, v float32) {
	if dir.isRow() {
		s.w = v
	} else {
		s.h = v
	}
}

func (s *size) setCross(dir FlexDirection, v float32) {
	if dir.isRow() {
		s.h = v
	} else {
		s.w = v
	}
}

func (s size) add(b size) size { return size{s.w + b.w, s.h + b.h} }
func (s size) sub(b size) size { return size{s.w - b.w, s.h - b.h} }
func (s size) opts() optSize   { return optSize{some(s.w), some(s.h)} }

// maybeClamp clamps each axis by optional bounds.
func (s size) maybeClamp(min, max optSize) size {
	return size{fclampOpt(s.w, min.w, max.w), fclampOpt(s.h, min.h, max.h)}
}

// maybeMax raises each axis to an optional floor.
func (s size) maybeMax(b optSize) size { return size{fmaxOpt(s.w, b.w), fmaxOpt(s.h, b.h)} }

// optSize is a pair of optional floats.
type optSize struct{ w, h opt }

func (s optSize) main(dir FlexDirection) opt {
	if dir.isRow() {
		return s.w
	}
	return s.h
}

func (s optSize) cross(dir FlexDirection) opt {
	if dir.isRow() {
		return s.h
	}
	return s.w
}

func (s *optSize) setMain(dir FlexDirection, v opt) {
	if dir.isRow() {
		s.w = v
	} else {
		s.h = v
	}
}

func (s *optSize) setCross(dir FlexDirection, v opt) {
	if dir.isRow() {
		s.h = v
	} else {
		s.w = v
	}
}

func (s optSize) or(b optSize) optSize     { return optSize{s.w.or(b.w), s.h.or(b.h)} }
func (s optSize) maybeSubF(b size) optSize { return optSize{s.w.subF(b.w), s.h.subF(b.h)} }
func (s optSize) maybeMaxF(b size) optSize { return optSize{s.w.maxF(b.w), s.h.maxF(b.h)} }
func (s optSize) unwrapOr(b size) size     { return size{s.w.unwrapOr(b.w), s.h.unwrapOr(b.h)} }
func (s optSize) maybeClamp(min, max optSize) optSize {
	return optSize{s.w.maybeClamp(min.w, max.w), s.h.maybeClamp(min.h, max.h)}
}

// bothSet reports whether both axes are set.
func (s optSize) bothSet() bool { return s.w.ok && s.h.ok }

// applyAspectRatio fills an unset axis from the other through ratio
// (width / height), if exactly one axis is set.
func (s optSize) applyAspectRatio(ratio opt) optSize {
	if !ratio.ok {
		return s
	}
	switch {
	case s.w.ok && !s.h.ok:
		s.h = some(s.w.v / ratio.v)
	case s.h.ok && !s.w.ok:
		s.w = some(float32(s.h.v * ratio.v))
	}
	return s
}

// rect holds four edge values (insets, padding, border, margin) or the
// edges of a rectangle (scrollable overflow).
type rect struct{ l, r, t, b float32 }

func (r rect) add(o rect) rect { return rect{r.l + o.l, r.r + o.r, r.t + o.t, r.b + o.b} }

// sum returns the sums of the horizontal and vertical edges.
func (r rect) sum() size { return size{r.l + r.r, r.t + r.b} }

func (r rect) hsum() float32 { return r.l + r.r }
func (r rect) vsum() float32 { return r.t + r.b }

func (r rect) mainStart(dir FlexDirection) float32 {
	if dir.isRow() {
		return r.l
	}
	return r.t
}

func (r rect) mainEnd(dir FlexDirection) float32 {
	if dir.isRow() {
		return r.r
	}
	return r.b
}

func (r rect) crossStart(dir FlexDirection) float32 {
	if dir.isRow() {
		return r.t
	}
	return r.l
}

func (r rect) crossEnd(dir FlexDirection) float32 {
	if dir.isRow() {
		return r.b
	}
	return r.r
}

func (r rect) mainSum(dir FlexDirection) float32 {
	if dir.isRow() {
		return r.l + r.r
	}
	return r.t + r.b
}

func (r rect) crossSum(dir FlexDirection) float32 {
	if dir.isRow() {
		return r.t + r.b
	}
	return r.l + r.r
}

// union returns the smallest rectangle containing r and o (Taffy's
// Rect::union). The zero rect is not a neutral element in general, but
// scrollable overflow rects always contain the origin, so it is one here.
func (r rect) union(o rect) rect {
	return rect{fmin(r.l, o.l), fmax(r.r, o.r), fmin(r.t, o.t), fmax(r.b, o.b)}
}

// optRect holds four optional edge values.
type optRect struct{ l, r, t, b opt }

func (r optRect) or0() rect { return rect{r.l.or0(), r.r.or0(), r.t.or0(), r.b.or0()} }

func (r optRect) mainStart(dir FlexDirection) opt {
	if dir.isRow() {
		return r.l
	}
	return r.t
}

func (r optRect) mainEnd(dir FlexDirection) opt {
	if dir.isRow() {
		return r.r
	}
	return r.b
}

func (r optRect) crossStart(dir FlexDirection) opt {
	if dir.isRow() {
		return r.t
	}
	return r.l
}

func (r optRect) crossEnd(dir FlexDirection) opt {
	if dir.isRow() {
		return r.b
	}
	return r.r
}

// boolRect holds four flags, one per edge.
type boolRect struct{ l, r, t, b bool }

func (r boolRect) mainStart(dir FlexDirection) bool {
	if dir.isRow() {
		return r.l
	}
	return r.t
}

func (r boolRect) mainEnd(dir FlexDirection) bool {
	if dir.isRow() {
		return r.r
	}
	return r.b
}

func (r boolRect) crossStart(dir FlexDirection) bool {
	if dir.isRow() {
		return r.t
	}
	return r.l
}

func (r boolRect) crossEnd(dir FlexDirection) bool {
	if dir.isRow() {
		return r.b
	}
	return r.r
}

// SpaceKind is the kind of an AvailableSpace.
type SpaceKind uint8

const (
	// Definite space of Value logical pixels.
	Definite SpaceKind = iota
	// MinContentSpace: size the content as small as it can be without
	// overflowing (every soft wrap opportunity taken).
	MinContentSpace
	// MaxContentSpace: size the content as large as it wants to be (no
	// soft wrapping).
	MaxContentSpace
)

// AvailableSpace is the space available to a node in one axis: a definite
// amount, or a min-content or max-content sizing constraint.
type AvailableSpace struct {
	Value float32
	Kind  SpaceKind
}

// DefiniteSpace returns definite available space of v logical pixels.
func DefiniteSpace(v float32) AvailableSpace { return AvailableSpace{v, Definite} }

// Sizing constraints.
var (
	MinContentConstraint = AvailableSpace{Kind: MinContentSpace}
	MaxContentConstraint = AvailableSpace{Kind: MaxContentSpace}
)

func (a AvailableSpace) isDefinite() bool { return a.Kind == Definite }

// opt returns the definite value, if any.
func (a AvailableSpace) opt() opt {
	if a.Kind == Definite {
		return some(a.Value)
	}
	return none
}

// maybeSet returns definite space v if set, else a.
func (a AvailableSpace) maybeSet(v opt) AvailableSpace {
	if v.ok {
		return DefiniteSpace(v.v)
	}
	return a
}

// subF subtracts v from definite space.
func (a AvailableSpace) subF(v float32) AvailableSpace {
	if a.Kind == Definite {
		a.Value -= v
	}
	return a
}

// maybeClamp clamps definite space by optional bounds; min-content and
// max-content constraints are left unchanged (Taffy's
// AvailableSpace::maybe_clamp).
func (a AvailableSpace) maybeClamp(min, max opt) AvailableSpace {
	if a.Kind == Definite {
		a.Value = fclampOpt(a.Value, min, max)
	}
	return a
}

// maxF raises definite space to at least v.
func (a AvailableSpace) maxF(v float32) AvailableSpace {
	if a.Kind == Definite {
		a.Value = fmax(a.Value, v)
	}
	return a
}

// availSize is available space in both axes.
type availSize struct{ w, h AvailableSpace }

func (s availSize) main(dir FlexDirection) AvailableSpace {
	if dir.isRow() {
		return s.w
	}
	return s.h
}

func (s availSize) cross(dir FlexDirection) AvailableSpace {
	if dir.isRow() {
		return s.h
	}
	return s.w
}

func (s availSize) withMain(dir FlexDirection, v AvailableSpace) availSize {
	if dir.isRow() {
		s.w = v
	} else {
		s.h = v
	}
	return s
}

func (s availSize) withCross(dir FlexDirection, v AvailableSpace) availSize {
	if dir.isRow() {
		s.h = v
	} else {
		s.w = v
	}
	return s
}

func (s availSize) opts() optSize { return optSize{s.w.opt(), s.h.opt()} }

// sizeFromCross returns an optSize with only the cross axis set.
func sizeFromCross(dir FlexDirection, v opt) optSize {
	var s optSize
	s.setCross(dir, v)
	return s
}

// sumGaps returns the total size of the gaps between n items.
func sumGaps(gap float32, n int) float32 {
	if n <= 1 {
		return 0
	}
	return float32(gap * float32(n-1))
}
