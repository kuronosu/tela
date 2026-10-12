package style

import "fmt"

// LP is a length-percentage: Px logical pixels plus Pct percent of a
// reference length that only layout knows (the containing block's width or
// height, the box's own size for translate and border-radius, the gradient
// line for gradient stops). It covers CSS lengths, percentages and
// calc(a + b) of one of each.
type LP struct {
	Px  float32
	Pct float32 // percent: 50 means 50%
}

// PxLP returns a length-percentage of v logical pixels.
func PxLP(v float32) LP { return LP{Px: v} }

// PctLP returns a length-percentage of v percent.
func PctLP(v float32) LP { return LP{Pct: v} }

// Resolve returns the length in logical pixels given the reference length
// basis that the percentage refers to.
func (l LP) Resolve(basis float32) float32 {
	p := float32(basis * l.Pct) // explicit rounding: fusion-safe (ADR-027)
	return l.Px + p/100
}

// HasPercent reports whether the value depends on a reference length.
func (l LP) HasPercent() bool { return l.Pct != 0 }

// String formats the value as CSS: "12px", "50%" or "calc(50% + 12px)".
func (l LP) String() string {
	switch {
	case l.Pct == 0:
		return fmt.Sprintf("%gpx", l.Px)
	case l.Px == 0:
		return fmt.Sprintf("%g%%", l.Pct)
	}
	return fmt.Sprintf("calc(%g%% + %gpx)", l.Pct, l.Px)
}

// DimKind selects the meaning of a Dim.
type DimKind uint8

const (
	// DimAuto is the CSS keyword auto (for max-width and max-height it
	// stands for none). It is the zero value.
	DimAuto DimKind = iota
	// DimLength is the length-percentage in Dim.LP.
	DimLength
	// DimMinContent is the keyword min-content.
	DimMinContent
	// DimMaxContent is the keyword max-content.
	DimMaxContent
	// DimFitContent is the keyword fit-content.
	DimFitContent
)

// Dim is a size, margin, inset or flex-basis: auto, an intrinsic-size
// keyword or a length-percentage. Absolute units (rem, em, vw, vh, ch) are
// resolved to pixels when the style is computed; what is left for layout is
// pixels and percentages.
type Dim struct {
	Kind DimKind
	LP
}

// Common dimension values.
var (
	Auto       = Dim{}
	MinContent = Dim{Kind: DimMinContent}
	MaxContent = Dim{Kind: DimMaxContent}
	FitContent = Dim{Kind: DimFitContent}
)

// DimPx returns a dimension of v logical pixels.
func DimPx(v float32) Dim { return Dim{Kind: DimLength, LP: LP{Px: v}} }

// DimPct returns a dimension of v percent of the reference length.
func DimPct(v float32) Dim { return Dim{Kind: DimLength, LP: LP{Pct: v}} }

// DimOf returns a dimension of the length-percentage lp.
func DimOf(lp LP) Dim { return Dim{Kind: DimLength, LP: lp} }

// IsAuto reports whether d is auto.
func (d Dim) IsAuto() bool { return d.Kind == DimAuto }

// IsLength reports whether d is a length-percentage.
func (d Dim) IsLength() bool { return d.Kind == DimLength }

// Resolve returns the size in pixels and true for a length-percentage with
// the given percentage basis, and 0, false for auto and the keywords.
func (d Dim) Resolve(basis float32) (float32, bool) {
	if d.Kind != DimLength {
		return 0, false
	}
	return d.LP.Resolve(basis), true
}

// String formats the dimension as CSS.
func (d Dim) String() string {
	switch d.Kind {
	case DimLength:
		return d.LP.String()
	case DimMinContent:
		return "min-content"
	case DimMaxContent:
		return "max-content"
	case DimFitContent:
		return "fit-content"
	}
	return "auto"
}

// Sides holds one value per edge of a box, in CSS order.
type Sides[T any] struct{ Top, Right, Bottom, Left T }

// All returns Sides with v on every edge.
func AllSides[T any](v T) Sides[T] { return Sides[T]{v, v, v, v} }

// Corners holds one value per corner of a box, in CSS order.
type Corners[T any] struct{ TopLeft, TopRight, BottomRight, BottomLeft T }

// ChPerEm approximates the "0" advance, the CSS ch unit, as a fraction of
// the font size (ARCHITECTURE section 10 lists ch as approximate). Inter's
// zero is about 0.62 em wide.
const ChPerEm = 0.613

// lenExpr is a length expression as written in a class or an arbitrary
// value: a linear combination of units that is evaluated when the style is
// computed, when the root font size, the element's font size and the
// viewport are known. min(), max() and clamp() of expressions without a
// percentage are kept as a function node.
type lenExpr struct {
	px, rem, em, vw, vh, ch, pct float32
	fn                           *lenFn
}

type fnKind uint8

const (
	fnMin fnKind = iota
	fnMax
	fnClamp
	fnSum   // args[0] + args[1]
	fnScale // args[0] * args[1].px
)

type lenFn struct {
	kind fnKind
	args []lenExpr
}

func exprPx(v float32) lenExpr  { return lenExpr{px: v} }
func exprRem(v float32) lenExpr { return lenExpr{rem: v} }
func exprPct(v float32) lenExpr { return lenExpr{pct: v} }
func exprVw(v float32) lenExpr  { return lenExpr{vw: v} }
func exprVh(v float32) lenExpr  { return lenExpr{vh: v} }
func exprCh(v float32) lenExpr  { return lenExpr{ch: v} }
func exprEm(v float32) lenExpr  { return lenExpr{em: v} }
func exprZero() lenExpr         { return lenExpr{} }
func (l lenExpr) isZero() bool  { return l == lenExpr{} }
func (l lenExpr) hasPct() bool  { return l.pct != 0 || l.fn != nil && l.fn.hasPct() }
func (f *lenFn) hasPct() bool {
	for i := range f.args {
		if f.args[i].hasPct() {
			return true
		}
	}
	return false
}

func exprFromLength(l Length) lenExpr {
	switch l.Unit {
	case UnitRem:
		return exprRem(l.Value)
	case UnitEm:
		return exprEm(l.Value)
	}
	return exprPx(l.Value)
}

// scaled returns l multiplied by k.
func (l lenExpr) scaled(k float32) lenExpr {
	if l.fn != nil {
		// min(), max() and clamp() do not distribute over a scale, so
		// wrap the node and scale its value when it is evaluated.
		return lenExpr{fn: &lenFn{kind: fnScale, args: []lenExpr{l, exprPx(k)}}}
	}
	return lenExpr{
		px: float32(l.px * k), rem: float32(l.rem * k), em: float32(l.em * k),
		vw: float32(l.vw * k), vh: float32(l.vh * k), ch: float32(l.ch * k),
		pct: float32(l.pct * k),
	}
}

// add returns l + m. Function nodes are wrapped in a sum node.
func (l lenExpr) add(m lenExpr) lenExpr {
	if l.fn != nil || m.fn != nil {
		return lenExpr{fn: &lenFn{kind: fnSum, args: []lenExpr{l, m}}}
	}
	return lenExpr{
		px: l.px + m.px, rem: l.rem + m.rem, em: l.em + m.em,
		vw: l.vw + m.vw, vh: l.vh + m.vh, ch: l.ch + m.ch, pct: l.pct + m.pct,
	}
}

// eval evaluates the expression with the unit context of e.
func (l *lenExpr) eval(e *env) LP {
	if l.fn != nil {
		f := l.fn
		switch f.kind {
		case fnSum:
			a, b := f.args[0].eval(e), f.args[1].eval(e)
			return LP{a.Px + b.Px, a.Pct + b.Pct}
		case fnScale:
			a, k := f.args[0].eval(e), f.args[1].px
			return LP{float32(a.Px * k), float32(a.Pct * k)}
		case fnMin:
			v := f.args[0].eval(e).Px
			for i := 1; i < len(f.args); i++ {
				v = min(v, f.args[i].eval(e).Px)
			}
			return LP{Px: v}
		case fnMax:
			v := f.args[0].eval(e).Px
			for i := 1; i < len(f.args); i++ {
				v = max(v, f.args[i].eval(e).Px)
			}
			return LP{Px: v}
		case fnClamp:
			lo, mid, hi := f.args[0].eval(e).Px, f.args[1].eval(e).Px, f.args[2].eval(e).Px
			return LP{Px: max(lo, min(mid, hi))}
		}
	}
	px := l.px
	if l.rem != 0 {
		px += float32(l.rem * e.rootFont())
	}
	if l.em != 0 {
		px += float32(l.em * e.fontSize)
	}
	if l.ch != 0 {
		px += float32(float32(l.ch*e.fontSize) * ChPerEm)
	}
	if l.vw != 0 {
		px += float32(l.vw*e.viewport.W) / 100
	}
	if l.vh != 0 {
		px += float32(l.vh*e.viewport.H) / 100
	}
	return LP{Px: px, Pct: l.pct}
}

// px evaluates an expression that must not contain a percentage.
func (l *lenExpr) pxValue(e *env) float32 { return l.eval(e).Px }
