package style

import (
	"errors"
	"fmt"
	"strings"
)

// This file holds the value resolvers that utilities share: spacing
// numbers, fractions, dimensions, colors and plain numbers.

// spacingExpr resolves a spacing-scale value: "px" or a non-negative
// multiple of the theme's spacing unit ("4", "0.5", "13").
func spacingExpr(th *Theme, v string) (lenExpr, bool) {
	if v == "px" {
		return exprPx(1), true
	}
	n, ok := parseFloat(v)
	if !ok || n < 0 || v[0] == '+' || v[0] == '-' {
		return lenExpr{}, false
	}
	return exprFromLength(th.Spacing).scaled(n), true
}

// fractionExpr resolves "1/2" to 50%.
func fractionExpr(raw string) (lenExpr, bool) {
	i := strings.IndexByte(raw, '/')
	if i < 0 {
		return lenExpr{}, false
	}
	a, ok1 := parseFloat(raw[:i])
	b, ok2 := parseFloat(raw[i+1:])
	if !ok1 || !ok2 || b == 0 || a < 0 {
		return lenExpr{}, false
	}
	return exprPct(float32(100*a) / b), true
}

// dimSpec is a parsed dimension that still has to be evaluated.
type dimSpec struct {
	kind DimKind
	expr lenExpr
}

func (d dimSpec) dim(e *env) Dim {
	if d.kind != DimLength {
		return Dim{Kind: d.kind}
	}
	return DimLP(e.lp(&d.expr), d.expr.hasPercent())
}

// dimOpts says which values a dimension utility accepts.
type dimOpts uint16

const (
	dimAuto      dimOpts = 1 << iota // auto
	dimNone                          // none, as auto (max-w-none)
	dimFull                          // full: 100%
	dimFrac                          // 1/2
	dimSpacing                       // 4, 0.5, px
	dimIntrinsic                     // min, max, fit
	dimContainer                     // md, 3xl, prose, screen-md
	dimScreen                        // screen, svw, dvh...
	dimNeg                           // a leading '-' is allowed
	dimZeroOK                        // a bare 0 length is a spacing value
)

// axisWidth and axisHeight tell the screen keywords which viewport side to use.
type axis uint8

const (
	axisX axis = iota
	axisY
)

// parseDim parses the value of a size, inset, margin or basis utility.
func parseDim(c *pctx, a *arg, o dimOpts, ax axis) (dimSpec, error) {
	th := c.th
	neg := func(d dimSpec) (dimSpec, error) {
		if a.neg {
			if d.kind != DimLength {
				return d, fmt.Errorf("a negative sign needs a length")
			}
			d.expr = d.expr.scaled(-1)
		}
		return d, nil
	}
	if a.arb {
		if a.hasMod {
			return dimSpec{}, errNoMatch
		}
		if a.hint != "" && a.hint != "length" && a.hint != "percentage" {
			return dimSpec{}, errNoMatch
		}
		switch a.inner {
		case "auto":
			return dimSpec{kind: DimAuto}, nil
		case "min-content":
			return dimSpec{kind: DimMinContent}, nil
		case "max-content":
			return dimSpec{kind: DimMaxContent}, nil
		case "fit-content":
			return dimSpec{kind: DimFitContent}, nil
		}
		if a.hint == "" && looksLikeColor(a.inner) {
			return dimSpec{}, errNoMatch
		}
		e, err := parseLengthExpr(a.inner, true)
		if err != nil {
			if err == errNotLength {
				return dimSpec{}, fmt.Errorf("%q is not a length; use a number with a unit such as [320px], [20rem], [50%%] or a calc() expression", a.inner)
			}
			return dimSpec{}, err
		}
		return neg(dimSpec{kind: DimLength, expr: e})
	}
	if a.hasMod && o&dimFrac == 0 {
		return dimSpec{}, errNoMatch
	}
	v := a.value
	if a.hasMod {
		v = a.raw
	}
	switch v {
	case "auto":
		if o&dimAuto != 0 && !a.neg {
			return dimSpec{kind: DimAuto}, nil
		}
		return dimSpec{}, errNoMatch
	case "none":
		if o&dimNone != 0 && !a.neg {
			return dimSpec{kind: DimAuto}, nil
		}
		return dimSpec{}, errNoMatch
	case "full":
		if o&dimFull != 0 {
			return neg(dimSpec{kind: DimLength, expr: exprPct(100)})
		}
		return dimSpec{}, errNoMatch
	case "min", "max", "fit":
		if o&dimIntrinsic == 0 || a.neg {
			return dimSpec{}, errNoMatch
		}
		k := map[string]DimKind{"min": DimMinContent, "max": DimMaxContent, "fit": DimFitContent}[v]
		return dimSpec{kind: k}, nil
	case "screen", "svw", "lvw", "dvw", "svh", "lvh", "dvh":
		if o&dimScreen == 0 {
			return dimSpec{}, errNoMatch
		}
		switch {
		case v == "screen" && ax == axisX, v[len(v)-1] == 'w':
			return neg(dimSpec{kind: DimLength, expr: exprVw(100)})
		}
		return neg(dimSpec{kind: DimLength, expr: exprVh(100)})
	case "prose":
		if o&dimContainer != 0 {
			return dimSpec{kind: DimLength, expr: exprCh(65)}, nil
		}
		return dimSpec{}, errNoMatch
	}
	if o&dimFrac != 0 && strings.Contains(v, "/") {
		if e, ok := fractionExpr(v); ok {
			return neg(dimSpec{kind: DimLength, expr: e})
		}
		return dimSpec{}, errNoMatch
	}
	if a.hasMod {
		return dimSpec{}, errNoMatch
	}
	if o&dimSpacing != 0 {
		if e, ok := spacingExpr(th, v); ok {
			return neg(dimSpec{kind: DimLength, expr: e})
		}
	}
	if o&dimContainer != 0 {
		if l, ok := th.Containers[v]; ok {
			return neg(dimSpec{kind: DimLength, expr: exprFromLength(l)})
		}
		if bp, ok := strings.CutPrefix(v, "screen-"); ok {
			if l, ok := th.Breakpoints[bp]; ok {
				return neg(dimSpec{kind: DimLength, expr: exprFromLength(l)})
			}
		}
	}
	return dimSpec{}, errNoMatch
}

// dimValues lists the value suffixes of a dimension utility for
// suggestions.
func dimValues(o dimOpts, ax axis) func(*Theme) []string {
	return func(th *Theme) []string {
		var out []string
		if o&dimAuto != 0 {
			out = append(out, "auto")
		}
		if o&dimNone != 0 {
			out = append(out, "none")
		}
		if o&dimFull != 0 {
			out = append(out, "full")
		}
		if o&dimFrac != 0 {
			out = append(out, "1/2", "1/3", "2/3", "1/4", "3/4", "1/5", "2/5", "3/5", "4/5", "1/6", "5/6", "1/12", "11/12")
		}
		if o&dimSpacing != 0 {
			out = append(out, SpacingSuggestions...)
		}
		if o&dimIntrinsic != 0 {
			out = append(out, "min", "max", "fit")
		}
		if o&dimScreen != 0 {
			out = append(out, "screen", "svh", "lvh", "dvh", "svw", "lvw", "dvw")
		}
		if o&dimContainer != 0 {
			out = append(out, th.Keys(NamespaceContainer)...)
			out = append(out, "prose")
			for _, bp := range th.Keys(NamespaceBreakpoint) {
				out = append(out, "screen-"+bp)
			}
		}
		return out
	}
}

// dimUtil registers a utility that sets Dim fields.
func (r *registry) dimUtil(root string, o dimOpts, ax axis, set func(*env, Dim)) {
	r.add(root, o&dimNeg != 0, func(c *pctx, a *arg) (Op, error) {
		if a.raw == "" {
			return Op{}, errNoMatch
		}
		d, err := parseDim(c, a, o, ax)
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) { set(e, d.dim(e)) }), nil
	}, dimValues(o, ax))
}

// numberArg parses a plain number value: "3", "[3]", "0.5".
func numberArg(a *arg) (float32, bool) {
	if a.hasMod {
		return 0, false
	}
	if a.arb {
		if a.hint != "" && a.hint != "number" && a.hint != "integer" {
			return 0, false
		}
		return parseFloat(a.inner)
	}
	return parseFloat(a.value)
}

// colorArg resolves the color of a color utility: a theme color or an
// arbitrary color, with an opacity modifier. It returns errNoMatch when
// the value is not a color.
func colorArg(c *pctx, a *arg) (colorSpec, error) {
	var spec colorSpec
	switch {
	case a.arb:
		if a.hint != "" && a.hint != "color" {
			return spec, errNoMatch
		}
		if a.hint == "" && !looksLikeColor(a.inner) {
			return spec, errNoMatch
		}
		s, err := parseColor(a.inner)
		if err != nil {
			if errors.Is(err, errNotColor) {
				return spec, fmt.Errorf("%q is not a color; use #rrggbb, rgb(), hsl(), oklch() or oklab()", a.inner)
			}
			return spec, err
		}
		spec = s
	default:
		s, ok := themeColor(c.th, a.value)
		if !ok {
			return spec, errNoMatch
		}
		spec = s
	}
	if a.hasMod {
		alpha, err := parseAlphaModifier(a.mod)
		if err != nil {
			return spec, err
		}
		spec = spec.withAlpha(alpha)
	}
	return spec, nil
}

// parseAlphaModifier parses the opacity modifier of a color: "50" (50%),
// "2.5" or "[0.15]".
func parseAlphaModifier(m string) (float32, error) {
	if strings.HasPrefix(m, "[") && strings.HasSuffix(m, "]") {
		inner := m[1 : len(m)-1]
		if strings.HasSuffix(inner, "%") {
			v, ok := parseFloat(inner[:len(inner)-1])
			if ok && v >= 0 && v <= 100 {
				return v / 100, nil
			}
		} else if v, ok := parseFloat(inner); ok && v >= 0 && v <= 1 {
			return v, nil
		}
		return 0, fmt.Errorf("the opacity modifier %s must be a number from 0 to 1 or a percentage", m)
	}
	if strings.HasPrefix(m, "(") {
		return 0, unsupportedErr("CSS variables are not supported in opacity modifiers")
	}
	v, ok := parseFloat(m)
	if !ok || v < 0 || v > 100 {
		return 0, fmt.Errorf("the opacity modifier /%s must be a number from 0 to 100 (for example /50) or an arbitrary value (/[0.15])", m)
	}
	return v / 100, nil
}

// colorKeys returns the theme color keys plus transparent and current, for
// the vocabulary.
func colorKeys(th *Theme) []string {
	keys := th.Keys(NamespaceColor)
	keys = append(keys, th.TokenNames()...)
	return append(keys, "transparent", "current")
}

// colorUtil registers a utility that sets a color slot.
func (r *registry) colorUtil(root string, slot colorSlot, extra func(*env)) {
	r.add(root, false, func(c *pctx, a *arg) (Op, error) {
		if a.raw == "" {
			return Op{}, errNoMatch
		}
		spec, err := colorArg(c, a)
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) {
			e.sc.colors[slot] = spec
			if extra != nil {
				extra(e)
			}
		}), nil
	}, colorKeys)
}

// lenArg parses an arbitrary length value, or a spacing value when
// spacing is true, for utilities that take a length (border-[3px],
// gap-4). pct says whether a percentage is allowed.
func lenArg(c *pctx, a *arg, spacing, pct bool) (lenExpr, error) {
	if a.hasMod {
		return lenExpr{}, errNoMatch
	}
	if a.arb {
		if a.hint != "" && a.hint != "length" && a.hint != "percentage" && a.hint != "line-width" {
			return lenExpr{}, errNoMatch
		}
		if a.hint == "" && looksLikeColor(a.inner) {
			return lenExpr{}, errNoMatch
		}
		e, err := parseLengthExpr(a.inner, pct)
		if err != nil {
			if err == errNotLength {
				if a.hint == "" {
					return lenExpr{}, errNoMatch
				}
				return lenExpr{}, fmt.Errorf("%q is not a length; use a number with a unit such as [12px] or [1.5rem]", a.inner)
			}
			return lenExpr{}, err
		}
		return e, nil
	}
	if spacing {
		if e, ok := spacingExpr(c.th, a.value); ok {
			return e, nil
		}
	}
	return lenExpr{}, errNoMatch
}
