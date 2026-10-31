package style

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text"
)

type cornerMask uint8

const (
	cTL cornerMask = 1 << iota
	cTR
	cBR
	cBL
	// The logical corners, named block side then inline side:
	// start-start, start-end, end-end and end-start (env.corners
	// resolves them with the element's direction).
	cSS
	cSE
	cEE
	cES
)

// corners resolves the logical corners of m against the element's
// direction: in a right-to-left element the inline start is the right.
func (e *env) corners(m cornerMask) cornerMask {
	if m&(cSS|cSE|cEE|cES) == 0 {
		return m
	}
	rtl := e.s.Text.Direction == text.DirectionRTL
	out := m &^ (cSS | cSE | cEE | cES)
	pick := func(bit, ltr, rtlCorner cornerMask) {
		if m&bit != 0 {
			if rtl {
				out |= rtlCorner
			} else {
				out |= ltr
			}
		}
	}
	pick(cSS, cTL, cTR)
	pick(cSE, cTR, cTL)
	pick(cEE, cBR, cBL)
	pick(cES, cBL, cBR)
	return out
}

func setCorners(c *Corners[LP], m cornerMask, l LP) {
	if m&cTL != 0 {
		c.TopLeft = l
	}
	if m&cTR != 0 {
		c.TopRight = l
	}
	if m&cBR != 0 {
		c.BottomRight = l
	}
	if m&cBL != 0 {
		c.BottomLeft = l
	}
}

// borderSides returns pointers to the four borders in CSS order.
func borderSides(s *ComputedStyle) [4]*Border {
	return [4]*Border{&s.Border.Top, &s.Border.Right, &s.Border.Bottom, &s.Border.Left}
}

var borderStyles = map[string]BorderStyle{
	"solid": BorderSolid, "dashed": BorderDashed, "dotted": BorderDotted,
	"double": BorderDouble, "hidden": BorderHidden, "none": BorderNone,
}

// radiusFull stands for Tailwind's calc(infinity * 1px).
const radiusFull = 9999

func registerBorders(r *registry) {
	registerBorderSides(r)
	registerBorderRadius(r)
	registerOutline(r)
	registerRing(r)
}

// registerBorderSides registers the border width, color and style classes.
func registerBorderSides(r *registry) {
	for _, p := range sideRoots("border", "-") {
		r.add(p.root, false, borderWidthUtility(p.root, p.mask), func(th *Theme) []string {
			return []string{"", "0", "2", "4", "8"}
		})
		r.add(p.root, false, borderColorUtility(p.mask), colorKeys)
	}
	// Style keywords exist for the whole border only (vocabulary).
	r.add("border", false, func(*pctx, *arg) (Op, error) { return Op{}, errNoMatch }, func(*Theme) []string {
		return []string{"solid", "dashed", "dotted", "double", "hidden", "none"}
	})
}

// setBorderWidth sets the width of the borders that mask selects.
func setBorderWidth(e *env, mask sideMask, w float32) {
	m := e.sides(mask)
	for i, side := range []sideMask{sTop, sRight, sBottom, sLeft} {
		if m&side != 0 {
			borderSides(e.s)[i].Width = w
		}
	}
}

// borderWidthUtility returns the parser of the width (and, for the whole
// border, the style) classes of the sides that mask selects.
func borderWidthUtility(root string, mask sideMask) handler {
	return func(c *pctx, a *arg) (Op, error) {
		if a.raw == "" {
			return op(func(e *env) { setBorderWidth(e, mask, 1) }), nil
		}
		if a.hasMod {
			return Op{}, errNoMatch
		}
		if root == "border" {
			if st, ok := borderStyles[a.value]; ok && !a.arb {
				return op(func(e *env) {
					for _, b := range borderSides(e.s) {
						b.Style = st
					}
				}), nil
			}
		}
		if !a.arb {
			if n, ok := parseFloat(a.value); ok && n >= 0 && a.value[0] != '+' {
				return op(func(e *env) { setBorderWidth(e, mask, n) }), nil
			}
			return Op{}, errNoMatch
		}
		ex, ok, err := arbitraryLineWidth(a)
		if !ok {
			if err != nil {
				return Op{}, err
			}
			return Op{}, errNoMatch
		}
		return op(func(e *env) { setBorderWidth(e, mask, e.lp(&ex).Px) }), nil
	}
}

// arbitraryLineWidth parses the bracketed width of border-[...]. ok is
// false when the value is not a width (a color, or another type hint);
// err is set only when a type hint demanded a length.
func arbitraryLineWidth(a *arg) (ex lenExpr, ok bool, err error) {
	if a.hint != "" && a.hint != "length" && a.hint != "line-width" {
		return lenExpr{}, false, nil
	}
	if looksLikeColor(a.inner) {
		return lenExpr{}, false, nil
	}
	ex, err = parseLengthExpr(a.inner, false)
	if err == nil {
		return ex, true, nil
	}
	if a.hint != "" {
		return lenExpr{}, false, err
	}
	return lenExpr{}, false, nil
}

// borderColorUtility returns the parser of the color classes of the sides
// that mask selects.
func borderColorUtility(mask sideMask) handler {
	return func(c *pctx, a *arg) (Op, error) {
		if a.raw == "" {
			return Op{}, errNoMatch
		}
		spec, err := colorArg(c, a)
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) {
			m := e.sides(mask)
			for i, side := range []sideMask{sTop, sRight, sBottom, sLeft} {
				if m&side != 0 {
					e.sc.colors[slotBorderTop+colorSlot(i)] = spec
				}
			}
		}), nil
	}
}

// registerBorderRadius registers the rounded-* classes.
func registerBorderRadius(r *registry) {
	corners := []struct {
		root string
		mask cornerMask
	}{
		{"rounded", cTL | cTR | cBR | cBL},
		{"rounded-t", cTL | cTR}, {"rounded-r", cTR | cBR}, {"rounded-b", cBR | cBL}, {"rounded-l", cTL | cBL},
		{"rounded-s", cSS | cES}, {"rounded-e", cSE | cEE},
		{"rounded-tl", cTL}, {"rounded-tr", cTR}, {"rounded-br", cBR}, {"rounded-bl", cBL},
		{"rounded-ss", cSS}, {"rounded-se", cSE}, {"rounded-ee", cEE}, {"rounded-es", cES},
	}
	for _, cn := range corners {
		r.add(cn.root, false, borderRadiusUtility(cn.mask), func(th *Theme) []string {
			return append(th.Keys(NamespaceRadius), "none", "full")
		})
	}
}

// borderRadiusUtility returns the parser of the rounded class of the
// corners that mask selects.
func borderRadiusUtility(mask cornerMask) handler {
	return func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		var ex lenExpr
		var err error
		if a.arb {
			ex, err = arbitraryRadius(a)
		} else {
			ex, err = themeRadius(c, a)
		}
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) { setCorners(&e.s.Radius, e.corners(mask), e.lp(&ex)) }), nil
	}
}

// arbitraryRadius parses the rounded-[<length>] form.
func arbitraryRadius(a *arg) (lenExpr, error) {
	if a.hint != "" && a.hint != "length" && a.hint != "percentage" {
		return lenExpr{}, errNoMatch
	}
	ex, err := parseLengthExpr(a.inner, true)
	if err != nil {
		if err == errNotLength {
			return lenExpr{}, fmt.Errorf("%q is not a length; use e.g. rounded-[12px] or rounded-[50%%]", a.inner)
		}
		return lenExpr{}, err
	}
	return ex, nil
}

// themeRadius parses the rounded and rounded-<name> forms, where none and
// full are fixed keywords.
func themeRadius(c *pctx, a *arg) (lenExpr, error) {
	switch {
	case a.raw == "":
		l, ok := c.th.Radius["DEFAULT"]
		if !ok {
			return lenExpr{}, errNoMatch
		}
		return exprFromLength(l), nil
	case a.value == "none":
		return lenExpr{}, nil
	case a.value == "full":
		return exprPx(radiusFull), nil
	}
	l, ok := c.th.Radius[a.value]
	if !ok || a.value == "DEFAULT" {
		return lenExpr{}, errNoMatch
	}
	return exprFromLength(l), nil
}

func registerOutline(r *registry) {
	r.add("outline", false, func(c *pctx, a *arg) (Op, error) {
		switch {
		case a.raw == "":
			return op(func(e *env) { e.s.Outline.Width = 1; e.sc.outlineUsed = true }), nil
		case a.hasMod:
			return Op{}, errNoMatch
		case a.value == "none" && !a.arb:
			return op(func(e *env) { e.sc.outlineStyle, e.sc.outlineStyleSet = BorderNone, true }), nil
		case a.value == "hidden" && !a.arb:
			return op(func(e *env) {
				e.s.Outline.Width, e.s.Outline.Offset = 2, 2
				e.sc.outlineStyle, e.sc.outlineStyleSet = BorderSolid, true
				e.sc.colors[slotOutline] = solidSpec(gfx.Transparent)
			}), nil
		}
		if !a.arb {
			if st, ok := borderStyles[a.value]; ok {
				return op(func(e *env) { e.sc.outlineStyle, e.sc.outlineStyleSet = st, true }), nil
			}
			if n, ok := parseFloat(a.value); ok && n >= 0 && a.value[0] != '+' {
				return op(func(e *env) { e.s.Outline.Width = n; e.sc.outlineUsed = true }), nil
			}
		} else if a.hint == "" || a.hint == "length" || a.hint == "line-width" {
			if !looksLikeColor(a.inner) {
				ex, err := parseLengthExpr(a.inner, false)
				if err == nil {
					return op(func(e *env) { e.s.Outline.Width = e.lp(&ex).Px; e.sc.outlineUsed = true }), nil
				}
				if a.hint != "" {
					return Op{}, err
				}
			}
		}
		return Op{}, errNoMatch
	}, func(*Theme) []string {
		return []string{"", "0", "1", "2", "4", "8", "none", "hidden", "solid", "dashed", "dotted", "double"}
	})
	r.colorUtil("outline", slotOutline, nil)

	r.add("outline-offset", true, func(c *pctx, a *arg) (Op, error) {
		var px float32
		var ex *lenExpr
		if a.arb {
			e, err := lenArg(c, a, false, false)
			if err != nil {
				return Op{}, err
			}
			ex = &e
		} else if n, ok := numberArg(a); ok && n >= 0 {
			px = n
		} else {
			return Op{}, errNoMatch
		}
		neg := a.neg
		return op(func(e *env) {
			v := px
			if ex != nil {
				v = e.lp(ex).Px
			}
			if neg {
				v = -v
			}
			e.s.Outline.Offset = v
		}), nil
	}, func(*Theme) []string { return []string{"0", "1", "2", "4", "8"} })
}

func registerRing(r *registry) {
	width := func(root string, set func(*env, float32)) {
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			var px float32
			var ex *lenExpr
			switch {
			case a.raw == "":
				px = 1
			case a.hasMod:
				return Op{}, errNoMatch
			case a.arb:
				e, err := lenArg(c, a, false, false)
				if err != nil {
					return Op{}, err
				}
				ex = &e
			default:
				n, ok := parseFloat(a.value)
				if !ok || n < 0 || a.value[0] == '+' {
					return Op{}, errNoMatch
				}
				px = n
			}
			return op(func(e *env) {
				v := px
				if ex != nil {
					v = e.lp(ex).Px
				}
				e.sc.shadowUsed = true
				set(e, v)
			}), nil
		}, func(*Theme) []string { return []string{"", "0", "1", "2", "4", "8"} })
	}
	width("ring", func(e *env, w float32) { e.sc.ringW = w })
	width("inset-ring", func(e *env, w float32) { e.sc.insetRingW = w })
	r.colorUtil("ring", slotRing, nil)
	r.colorUtil("inset-ring", slotInsetRing, nil)
	r.static("ring-inset", op(func(e *env) { e.sc.ringInset = true }))

	// ring-offset-N sets the width only; it paints when a ring or shadow does.
	r.add("ring-offset", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod || a.raw == "" {
			return Op{}, errNoMatch
		}
		var px float32
		var ex *lenExpr
		if a.arb {
			if a.hint == "color" || (a.hint == "" && looksLikeColor(a.inner)) {
				return Op{}, errNoMatch
			}
			e, err := lenArg(c, a, false, false)
			if err != nil {
				return Op{}, err
			}
			ex = &e
		} else {
			n, ok := parseFloat(a.value)
			if !ok || n < 0 || a.value[0] == '+' {
				return Op{}, errNoMatch
			}
			px = n
		}
		return op(func(e *env) {
			v := px
			if ex != nil {
				v = e.lp(ex).Px
			}
			e.sc.ringOffW = v
		}), nil
	}, func(*Theme) []string { return []string{"0", "1", "2", "4", "8"} })
	r.colorUtil("ring-offset", slotRingOffset, nil)
}

// ---------------------------------------------------------------------
// Shadows.

// currentShadowColor marks a shadow layer whose color is currentColor; it
// is replaced when the style is finalized. It is compared exactly: colors
// are extended sRGB, so a negative component alone does not mark it.
var currentShadowColor = gfx.Color{R: -1, G: -1, B: -1, A: 1}

// parseShadowList parses an arbitrary CSS shadow list:
// "0 10px 30px rgba(0,0,0,.3), inset 0 1px 0 #fff".
func parseShadowList(c *pctx, s string, allowSpread bool) ([]Shadow, error) {
	var out []Shadow
	for _, layer := range splitTop(s, ',') {
		var sh Shadow
		sh.Color = currentShadowColor
		var nums []float32
		for _, tok := range fieldsTop(layer) {
			if tok == "inset" {
				sh.Inset = true
				continue
			}
			if !looksLikeColor(tok) {
				if e, err := parseLengthExpr(tok, false); err == nil {
					// Shadow lengths are fixed: rem is the root size and em
					// the root size too (the element is not known here).
					ev := env{fontSize: c.th.RootFontSize, theme: c.th}
					nums = append(nums, e.eval(&ev).Px)
					continue
				} else if err != errNotLength {
					return nil, err
				}
			}
			spec, ok, err := c.colorValue(tok)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("invalid shadow value %q in %q", tok, layer)
			}
			if spec.current {
				sh.Color = currentShadowColor
			} else {
				sh.Color = spec.c
			}
		}
		if len(nums) < 2 || len(nums) > 4 {
			return nil, fmt.Errorf("a shadow needs 2 to 4 lengths (x y blur spread), got %q", strings.TrimSpace(layer))
		}
		sh.OffsetX, sh.OffsetY = nums[0], nums[1]
		if len(nums) > 2 {
			sh.Blur = nums[2]
		}
		if len(nums) > 3 {
			if !allowSpread {
				return nil, fmt.Errorf("spread is not allowed here, got %q", strings.TrimSpace(layer))
			}
			sh.Spread = nums[3]
		}
		if sh.Blur < 0 {
			return nil, fmt.Errorf("the blur of a shadow cannot be negative, got %q", strings.TrimSpace(layer))
		}
		out = append(out, sh)
	}
	return out, nil
}

// resolveCurrentShadows replaces the currentColor marker.
func resolveCurrentShadows(layers []Shadow, cur gfx.Color) []Shadow {
	has := false
	for i := range layers {
		if layers[i].Color == currentShadowColor {
			has = true
			break
		}
	}
	if !has {
		return layers
	}
	out := make([]Shadow, len(layers))
	copy(out, layers)
	for i := range out {
		if out[i].Color == currentShadowColor {
			out[i].Color = cur
		}
	}
	return out
}
