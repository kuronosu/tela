package style

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/text"
)

// fontSizeOp builds an op that sets the font size and, in the second pass,
// the line height modifier of text-sm/6. size resolves against the parent's
// font size for em and %.
func fontSizeOp(size func(e *env) float32, sizeLeading float32, hasLeading bool, post func(*env)) Op {
	return Op{font: true, apply: func(e *env) {
		e.s.Text.FontSize = max(size(e), 0)
		if hasLeading {
			e.sc.sizeLeading, e.sc.sizeLeadingSet = sizeLeading, true
		}
	}, post: post}
}

// lineHeightArg parses a line-height value: a theme leading key, "none", a
// spacing number, or an arbitrary number or length.
func lineHeightArg(c *pctx, raw string, arb bool, inner string) (func(*env) text.LineHeight, bool, error) {
	if arb {
		if n, ok := parseFloat(inner); ok {
			return func(*env) text.LineHeight { return text.LineHeightMultiple(n) }, true, nil
		}
		ex, err := parseLengthExpr(inner, false)
		if err != nil {
			if err == errNotLength {
				return nil, true, fmt.Errorf("%q is not a line height; use a number ([1.2]) or a length ([22px])", inner)
			}
			return nil, true, err
		}
		return func(e *env) text.LineHeight { return text.LineHeightPixels(e.lp(&ex).Px) }, true, nil
	}
	if raw == "none" {
		return func(*env) text.LineHeight { return text.LineHeightMultiple(1) }, true, nil
	}
	if v, ok := c.th.Leading[raw]; ok {
		return func(*env) text.LineHeight { return text.LineHeightMultiple(v) }, true, nil
	}
	if ex, ok := spacingExpr(c.th, raw); ok {
		return func(e *env) text.LineHeight { return text.LineHeightPixels(e.lp(&ex).Px) }, true, nil
	}
	return nil, false, nil
}

func registerTypography(r *registry) {
	// text-*: alignment, wrapping and overflow keywords.
	aligns := map[string]text.TextAlign{"left": text.AlignLeft, "center": text.AlignCenter,
		"right": text.AlignRight, "justify": text.AlignJustify, "start": text.AlignStart, "end": text.AlignEnd}
	keys := map[string]Op{}
	for _, k := range sortedMapKeys(aligns) {
		v := aligns[k]
		keys[k] = op(func(e *env) { e.s.Text.TextAlign = v })
	}
	setWrap := func(wrap bool) Op {
		return op(func(e *env) {
			ws := &e.s.Text.WhiteSpace
			switch {
			case wrap && *ws == text.WhiteSpaceNowrap:
				*ws = text.WhiteSpaceNormal
			case wrap && *ws == text.WhiteSpacePre:
				*ws = text.WhiteSpacePreWrap
			case !wrap && *ws == text.WhiteSpaceNormal, !wrap && *ws == text.WhiteSpacePreLine:
				*ws = text.WhiteSpaceNowrap
			case !wrap && *ws == text.WhiteSpacePreWrap:
				*ws = text.WhiteSpacePre
			}
		})
	}
	keys["wrap"], keys["balance"], keys["pretty"] = setWrap(true), setWrap(true), setWrap(true)
	keys["nowrap"] = setWrap(false)
	keys["ellipsis"] = op(func(e *env) { e.s.TextOverflow = text.TextOverflowEllipsis })
	keys["clip"] = op(func(e *env) { e.s.TextOverflow = text.TextOverflowClip })
	r.enum("text", false, keys)

	// text-<size>[/<line height>] and text-[<length>].
	r.add("text", false, func(c *pctx, a *arg) (Op, error) {
		var size func(*env) float32
		var sizeLeading float32
		hasLeading := false
		switch {
		case a.arb:
			if a.hint != "" && a.hint != "length" && a.hint != "percentage" && a.hint != "absolute-size" && a.hint != "relative-size" {
				return Op{}, errNoMatch
			}
			if a.hint == "" && looksLikeColor(a.inner) {
				return Op{}, errNoMatch
			}
			ex, err := parseLengthExpr(a.inner, true)
			if err != nil {
				if err == errNotLength {
					if a.hint == "" {
						return Op{}, errNoMatch
					}
					return Op{}, fmt.Errorf("%q is not a font size; use e.g. text-[14px] or text-[1.25rem]", a.inner)
				}
				return Op{}, err
			}
			size = func(e *env) float32 {
				lp := e.lp(&ex)
				return lp.Px + float32(e.parentFontSize*lp.Pct)/100
			}
		default:
			ts, ok := c.th.Text[a.value]
			if !ok {
				return Op{}, errNoMatch
			}
			ex := exprFromLength(ts.Size)
			size = func(e *env) float32 {
				// em in a theme size is relative to the parent's size.
				return e.lp(&ex).Px
			}
			sizeLeading, hasLeading = ts.LineHeight, true
		}
		var post func(*env)
		if a.hasMod {
			var lh func(*env) text.LineHeight
			if strings.HasPrefix(a.mod, "[") && strings.HasSuffix(a.mod, "]") {
				f, ok, err := lineHeightArg(c, "", true, arbitrarySpaces(a.mod[1:len(a.mod)-1]))
				if !ok || err != nil {
					return Op{}, err
				}
				lh = f
			} else {
				f, ok, err := lineHeightArg(c, a.mod, false, "")
				if err != nil {
					return Op{}, err
				}
				if !ok {
					return Op{}, fmt.Errorf("unknown line height /%s; use a spacing number (text-sm/6), a leading name (text-sm/tight) or [22px]", a.mod)
				}
				lh = f
			}
			post = func(e *env) { e.sc.leading, e.sc.leadingSet = lh(e), true }
		}
		return fontSizeOp(size, sizeLeading, hasLeading, post), nil
	}, func(th *Theme) []string { return th.Keys(NamespaceText) })
	r.colorUtil("text", slotText, nil)

	// font-*: weight, family.
	r.add("font", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		if a.arb {
			if a.hint == "" || a.hint == "number" {
				if n, ok := parseFloat(a.inner); ok {
					return fontWeightOp(n)
				}
			}
			if a.hint != "" && a.hint != "family-name" && a.hint != "generic-name" {
				return Op{}, errNoMatch
			}
			fam := parseFamilyList(a.inner)
			if len(fam) == 0 {
				return Op{}, fmt.Errorf("%q is not a font family list; use e.g. font-['Open_Sans',sans-serif]", a.inner)
			}
			return op(func(e *env) { e.s.Text.FontFamily = fam }), nil
		}
		if w, ok := c.th.FontWeights[a.value]; ok {
			return fontWeightOp(w)
		}
		if fam, ok := c.th.Fonts[a.value]; ok {
			return op(func(e *env) { e.s.Text.FontFamily = fam }), nil
		}
		return Op{}, errNoMatch
	}, func(th *Theme) []string {
		return append(th.Keys(NamespaceFontWeight), th.Keys(NamespaceFont)...)
	})
	r.static("italic", op(func(e *env) { e.s.Text.FontStyle = text.StyleItalic }))
	r.static("not-italic", op(func(e *env) { e.s.Text.FontStyle = text.StyleNormal }))
	stretch := map[string]float32{"ultra-condensed": 50, "extra-condensed": 62.5, "condensed": 75,
		"semi-condensed": 87.5, "normal": 100, "semi-expanded": 112.5, "expanded": 125,
		"extra-expanded": 150, "ultra-expanded": 200}
	r.add("font-stretch", false, func(c *pctx, a *arg) (Op, error) {
		var v float32
		switch {
		case a.hasMod:
			return Op{}, errNoMatch
		case a.arb:
			s := strings.TrimSuffix(a.inner, "%")
			n, ok := parseFloat(s)
			if !ok {
				return Op{}, fmt.Errorf("%q is not a percentage; use e.g. font-stretch-[75%%]", a.inner)
			}
			v = n
		default:
			if s, ok := stretch[a.value]; ok {
				v = s
			} else if n, ok := parseFloat(strings.TrimSuffix(a.value, "%")); ok && strings.HasSuffix(a.value, "%") {
				v = n
			} else {
				return Op{}, errNoMatch
			}
		}
		return op(func(e *env) { e.s.Text.FontStretch = v }), nil
	}, func(*Theme) []string { return sortedMapKeys(stretch) })

	// tracking-*, leading-*.
	r.add("tracking", true, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		var ex lenExpr
		if a.arb {
			e, err := lenArg(c, a, false, false)
			if err != nil {
				return Op{}, err
			}
			ex = e
		} else if l, ok := c.th.Tracking[a.value]; ok {
			ex = exprFromLength(l)
		} else {
			return Op{}, errNoMatch
		}
		if a.neg {
			ex = ex.scaled(-1)
		}
		return op(func(e *env) { e.s.Text.LetterSpacing = e.lp(&ex).Px }), nil
	}, func(th *Theme) []string { return th.Keys(NamespaceTracking) })
	r.add("leading", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		f, ok, err := lineHeightArg(c, a.value, a.arb, a.inner)
		if err != nil {
			return Op{}, err
		}
		if !ok {
			return Op{}, errNoMatch
		}
		return op(func(e *env) { e.sc.leading, e.sc.leadingSet = f(e), true }), nil
	}, func(th *Theme) []string {
		return append(append(th.Keys(NamespaceLeading), "none"), "3", "4", "5", "6", "7", "8", "9", "10")
	})

	// Transform, decoration and misc.
	r.static("uppercase", op(func(e *env) { e.s.Text.TextTransform = text.TransformUppercase }))
	r.static("lowercase", op(func(e *env) { e.s.Text.TextTransform = text.TransformLowercase }))
	r.static("capitalize", op(func(e *env) { e.s.Text.TextTransform = text.TransformCapitalize }))
	r.static("normal-case", op(func(e *env) { e.s.Text.TextTransform = text.TransformNone }))

	lines := map[string]text.Decoration{"underline": text.DecorationUnderline,
		"overline": text.DecorationOverline, "line-through": text.DecorationLineThrough}
	for _, k := range sortedMapKeys(lines) {
		v := lines[k]
		r.static(k, op(func(e *env) { e.s.Text.Decoration.Lines, e.sc.decoLinesSet = v, true }))
	}
	r.static("no-underline", op(func(e *env) { e.s.Text.Decoration.Lines, e.sc.decoLinesSet = 0, false }))
	decoStyles := map[string]DecorationStyle{"solid": DecorationSolid, "double": DecorationDouble,
		"dotted": DecorationDotted, "dashed": DecorationDashed, "wavy": DecorationWavy}
	deco := map[string]Op{
		"auto":      op(func(e *env) { e.s.Text.Decoration.Thickness = 0 }),
		"from-font": op(func(e *env) { e.s.Text.Decoration.Thickness = 0 }),
	}
	for _, k := range sortedMapKeys(decoStyles) {
		v := decoStyles[k]
		deco[k] = op(func(e *env) { e.s.Text.Decoration.Style = v })
	}
	r.enum("decoration", false, deco)
	r.add("decoration", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		if a.arb {
			if a.hint == "color" || (a.hint == "" && looksLikeColor(a.inner)) {
				return Op{}, errNoMatch
			}
			ex, err := lenArg(c, a, false, false)
			if err != nil {
				return Op{}, err
			}
			return op(func(e *env) { e.s.Text.Decoration.Thickness = e.lp(&ex).Px }), nil
		}
		n, ok := parseFloat(a.value)
		if !ok || n < 0 || a.value[0] == '+' {
			return Op{}, errNoMatch
		}
		return op(func(e *env) { e.s.Text.Decoration.Thickness = n }), nil
	}, func(*Theme) []string { return []string{"0", "1", "2", "4", "8"} })
	r.colorUtil("decoration", slotDecoration, nil)
	r.add("underline-offset", true, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		if a.value == "auto" && !a.arb {
			return op(func(e *env) { e.s.Text.Decoration.Offset, e.s.Text.Decoration.OffsetSet = 0, false }), nil
		}
		var ex lenExpr
		if a.arb {
			e, err := lenArg(c, a, false, false)
			if err != nil {
				return Op{}, err
			}
			ex = e
		} else if n, ok := parseFloat(a.value); ok && n >= 0 && a.value[0] != '+' {
			ex = exprPx(n)
		} else {
			return Op{}, errNoMatch
		}
		if a.neg {
			ex = ex.scaled(-1)
		}
		return op(func(e *env) {
			e.s.Text.Decoration.Offset, e.s.Text.Decoration.OffsetSet = e.lp(&ex).Px, true
		}), nil
	}, func(*Theme) []string { return []string{"auto", "0", "1", "2", "4", "8"} })

	ws := map[string]text.WhiteSpace{"normal": text.WhiteSpaceNormal, "nowrap": text.WhiteSpaceNowrap,
		"pre": text.WhiteSpacePre, "pre-line": text.WhiteSpacePreLine, "pre-wrap": text.WhiteSpacePreWrap,
		"break-spaces": text.WhiteSpaceBreakSpaces}
	wsOps := map[string]Op{}
	for _, k := range sortedMapKeys(ws) {
		v := ws[k]
		wsOps[k] = op(func(e *env) { e.s.Text.WhiteSpace = v })
	}
	r.enum("whitespace", false, wsOps)

	r.static("break-normal", op(func(e *env) {
		e.s.Text.OverflowWrap, e.s.Text.WordBreak = text.OverflowWrapNormal, text.WordBreakNormal
	}))
	r.static("break-words", op(func(e *env) { e.s.Text.OverflowWrap = text.OverflowWrapBreakWord }))
	r.static("break-all", op(func(e *env) { e.s.Text.WordBreak = text.WordBreakBreakAll }))
	r.static("break-keep", op(func(e *env) { e.s.Text.WordBreak = text.WordBreakKeepAll }))
	r.enum("wrap", false, map[string]Op{
		"normal":     op(func(e *env) { e.s.Text.OverflowWrap = text.OverflowWrapNormal }),
		"break-word": op(func(e *env) { e.s.Text.OverflowWrap = text.OverflowWrapBreakWord }),
		"anywhere":   op(func(e *env) { e.s.Text.OverflowWrap = text.OverflowWrapAnywhere }),
	})
	r.static("truncate", op(func(e *env) {
		e.s.OverflowX, e.s.OverflowY = OverflowHidden, OverflowHidden
		e.s.TextOverflow = text.TextOverflowEllipsis
		e.s.Text.WhiteSpace = text.WhiteSpaceNowrap
	}))
	r.alias("overflow-ellipsis", "text-ellipsis")
	r.alias("overflow-clip-text", "text-clip")

	// vertical-align of atomic inline-level boxes (inline-block,
	// inline-flex, inline images and buttons inside a paragraph).
	r.enum("align", false, map[string]Op{
		"baseline":    op(func(e *env) { e.s.VerticalAlign = text.BoxBaseline }),
		"top":         op(func(e *env) { e.s.VerticalAlign = text.BoxTop }),
		"middle":      op(func(e *env) { e.s.VerticalAlign = text.BoxMiddle }),
		"bottom":      op(func(e *env) { e.s.VerticalAlign = text.BoxBottom }),
		"text-top":    op(func(e *env) { e.s.VerticalAlign = text.BoxTextTop }),
		"text-bottom": op(func(e *env) { e.s.VerticalAlign = text.BoxTextBottom }),
	})

	// line-clamp-N (clips like Tailwind's overflow: hidden).
	r.add("line-clamp", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		if a.value == "none" && !a.arb {
			return op(func(e *env) {
				e.s.LineClamp = 0
				e.s.OverflowX, e.s.OverflowY = OverflowVisible, OverflowVisible
			}), nil
		}
		var n int
		var ok bool
		if a.arb {
			n, ok = parseUint(a.inner)
		} else {
			n, ok = parseUint(a.value)
		}
		if !ok || n < 1 {
			return Op{}, errNoMatch
		}
		return op(func(e *env) {
			e.s.LineClamp = n
			e.s.OverflowX, e.s.OverflowY = OverflowHidden, OverflowHidden
		}), nil
	}, func(*Theme) []string {
		out := []string{"none"}
		for i := 1; i <= 6; i++ {
			out = append(out, strconv.Itoa(i))
		}
		return out
	})

	// font-variant-numeric.
	const (
		figure   = text.NumericLining | text.NumericOldstyle
		spacing  = text.NumericProportional | text.NumericTabular
		fraction = text.NumericDiagonalFractions | text.NumericStackedFractions
	)
	numeric := func(name string, group, bit text.NumericVariant) {
		r.static(name, op(func(e *env) { e.s.Text.Numeric = e.s.Text.Numeric&^group | bit }))
	}
	r.static("normal-nums", op(func(e *env) { e.s.Text.Numeric = 0 }))
	numeric("ordinal", text.NumericOrdinal, text.NumericOrdinal)
	numeric("slashed-zero", text.NumericSlashedZero, text.NumericSlashedZero)
	numeric("lining-nums", figure, text.NumericLining)
	numeric("oldstyle-nums", figure, text.NumericOldstyle)
	numeric("proportional-nums", spacing, text.NumericProportional)
	numeric("tabular-nums", spacing, text.NumericTabular)
	numeric("diagonal-fractions", fraction, text.NumericDiagonalFractions)
	numeric("stacked-fractions", fraction, text.NumericStackedFractions)

	r.static("antialiased", Op{})
	r.static("subpixel-antialiased", Op{})
}

func fontWeightOp(w float32) (Op, error) {
	if w < 1 || w > 1000 {
		return Op{}, fmt.Errorf("a font weight must be from 1 to 1000, got %g", w)
	}
	return op(func(e *env) { e.s.Text.FontWeight = w }), nil
}

// parseFamilyList parses "'Open Sans', sans-serif" into family names.
func parseFamilyList(s string) []string {
	var out []string
	for _, f := range splitTop(s, ',') {
		f = strings.TrimSpace(f)
		f = strings.Trim(f, `"'`)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}
