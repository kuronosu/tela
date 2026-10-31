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
// relative reports that size depends on the parent's font size.
func fontSizeOp(size func(e *env) float32, relative bool, sizeLeading float32, hasLeading bool, post func(*env)) Op {
	return Op{font: true, apply: func(e *env) {
		e.s.Text.FontSize = max(size(e), 0)
		e.s.FontSizeSet = !relative
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
	registerTextSizes(r)
	registerFontFace(r)
	registerTextSpacing(r)
	registerTextTransform(r)
	registerTextDecoration(r)
	registerTextWrapping(r)
	registerTextBlocks(r)
	registerNumericVariants(r)

	r.static("antialiased", Op{})
	r.static("subpixel-antialiased", Op{})
}

// registerTextSizes registers text-*: alignment, wrapping and overflow
// keywords, sizes and colors.
func registerTextSizes(r *registry) {
	aligns := map[string]text.TextAlign{"left": text.AlignLeft, "center": text.AlignCenter,
		"right": text.AlignRight, "justify": text.AlignJustify, "start": text.AlignStart, "end": text.AlignEnd}
	keys := map[string]Op{}
	for _, k := range sortedMapKeys(aligns) {
		v := aligns[k]
		keys[k] = op(func(e *env) { e.s.Text.TextAlign = v })
	}
	keys["wrap"], keys["nowrap"] = textWrapOp(true, text.TextWrapAuto), textWrapOp(false, text.TextWrapAuto)
	keys["balance"], keys["pretty"] = textWrapOp(true, text.TextWrapBalance), textWrapOp(true, text.TextWrapPretty)
	keys["ellipsis"] = op(func(e *env) { e.s.TextOverflow = text.TextOverflowEllipsis })
	keys["clip"] = op(func(e *env) { e.s.TextOverflow = text.TextOverflowClip })
	r.enum("text", false, keys)

	// text-<size>[/<line height>] and text-[<length>].
	r.add("text", false, textSizeUtility, func(th *Theme) []string { return th.Keys(NamespaceText) })
	r.colorUtil("text", slotText, nil)
}

// textWrapOp builds the op of a text-wrap class. text-wrap is the shorthand
// of text-wrap-mode (folded into white-space here) and text-wrap-style,
// which each of these classes sets: text-wrap and text-nowrap reset it to
// auto.
func textWrapOp(wrap bool, style text.TextWrapStyle) Op {
	return op(func(e *env) {
		e.s.Text.WrapStyle = style
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

// fontSizeSpec is the font size part of a text-<size> class: how to resolve
// the size, whether it depends on the parent's font size, and the line
// height that a theme size brings along.
type fontSizeSpec struct {
	size        func(*env) float32
	relative    bool
	sizeLeading float32
	hasLeading  bool
}

// textSizeUtility parses text-<size>[/<line height>] and text-[<length>].
func textSizeUtility(c *pctx, a *arg) (Op, error) {
	var spec fontSizeSpec
	var err error
	if a.arb {
		spec, err = arbitraryFontSize(a)
	} else {
		spec, err = themeFontSize(c, a)
	}
	if err != nil {
		return Op{}, err
	}
	var post func(*env)
	if a.hasMod {
		lh, err := sizeLineHeightMod(c, a.mod)
		if err != nil {
			return Op{}, err
		}
		post = func(e *env) { e.sc.leading, e.sc.leadingSet = lh(e), true }
	}
	return fontSizeOp(spec.size, spec.relative, spec.sizeLeading, spec.hasLeading, post), nil
}

// arbitraryFontSize parses the text-[<length>] form.
func arbitraryFontSize(a *arg) (fontSizeSpec, error) {
	if a.hint != "" && a.hint != "length" && a.hint != "percentage" && a.hint != "absolute-size" && a.hint != "relative-size" {
		return fontSizeSpec{}, errNoMatch
	}
	if a.hint == "" && looksLikeColor(a.inner) {
		return fontSizeSpec{}, errNoMatch
	}
	ex, err := parseLengthExpr(a.inner, true)
	if err != nil {
		if err == errNotLength {
			if a.hint == "" {
				return fontSizeSpec{}, errNoMatch
			}
			return fontSizeSpec{}, fmt.Errorf("%q is not a font size; use e.g. text-[14px] or text-[1.25rem]", a.inner)
		}
		return fontSizeSpec{}, err
	}
	size := func(e *env) float32 {
		lp := e.lp(&ex)
		return lp.Px + float32(e.parentFontSize*lp.Pct)/100
	}
	return fontSizeSpec{size: size, relative: ex.fontRelative()}, nil
}

// themeFontSize parses the text-<size> form of a theme text size.
func themeFontSize(c *pctx, a *arg) (fontSizeSpec, error) {
	ts, ok := c.th.Text[a.value]
	if !ok {
		return fontSizeSpec{}, errNoMatch
	}
	ex := exprFromLength(ts.Size)
	size := func(e *env) float32 {
		// em in a theme size is relative to the parent's size.
		return e.lp(&ex).Px
	}
	return fontSizeSpec{size: size, relative: ex.fontRelative(), sizeLeading: ts.LineHeight, hasLeading: true}, nil
}

// sizeLineHeightMod parses the /<line height> modifier of text-<size>.
func sizeLineHeightMod(c *pctx, mod string) (func(*env) text.LineHeight, error) {
	if strings.HasPrefix(mod, "[") && strings.HasSuffix(mod, "]") {
		// An arbitrary line height always resolves (ok is true) or errors.
		f, _, err := lineHeightArg(c, "", true, arbitrarySpaces(mod[1:len(mod)-1]))
		return f, err
	}
	f, ok, err := lineHeightArg(c, mod, false, "")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("unknown line height /%s; use a spacing number (text-sm/6), a leading name (text-sm/tight) or [22px]", mod)
	}
	return f, nil
}

// registerFontFace registers font-*: weight, family, style and stretch.
func registerFontFace(r *registry) {
	r.add("font", false, fontUtility, func(th *Theme) []string {
		return append(th.Keys(NamespaceFontWeight), th.Keys(NamespaceFont)...)
	})
	r.static("italic", op(func(e *env) { e.s.Text.FontStyle = text.StyleItalic }))
	r.static("not-italic", op(func(e *env) { e.s.Text.FontStyle = text.StyleNormal }))
	stretch := map[string]float32{"ultra-condensed": 50, "extra-condensed": 62.5, "condensed": 75,
		"semi-condensed": 87.5, "normal": 100, "semi-expanded": 112.5, "expanded": 125,
		"extra-expanded": 150, "ultra-expanded": 200}
	r.add("font-stretch", false, fontStretchUtility(stretch), func(*Theme) []string { return sortedMapKeys(stretch) })
}

// fontUtility parses font-<weight>, font-<family> and their arbitrary forms.
func fontUtility(c *pctx, a *arg) (Op, error) {
	if a.hasMod {
		return Op{}, errNoMatch
	}
	if a.arb {
		return arbitraryFont(a)
	}
	if w, ok := c.th.FontWeights[a.value]; ok {
		return fontWeightOp(w)
	}
	if fam, ok := c.th.Fonts[a.value]; ok {
		return op(func(e *env) { e.s.Text.FontFamily = fam }), nil
	}
	return Op{}, errNoMatch
}

// arbitraryFont parses font-[<weight>] and font-[<family list>].
func arbitraryFont(a *arg) (Op, error) {
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

// fontStretchUtility returns the parser of font-stretch-*, whose keywords
// map to percentages.
func fontStretchUtility(stretch map[string]float32) handler {
	return func(c *pctx, a *arg) (Op, error) {
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
	}
}

// registerTextSpacing registers tracking-* and leading-*.
func registerTextSpacing(r *registry) {
	r.add("tracking", true, trackingUtility, func(th *Theme) []string { return th.Keys(NamespaceTracking) })
	r.add("leading", false, leadingUtility, func(th *Theme) []string {
		return append(append(th.Keys(NamespaceLeading), "none"), "3", "4", "5", "6", "7", "8", "9", "10")
	})
}

// trackingUtility parses tracking-<name> and tracking-[<length>].
func trackingUtility(c *pctx, a *arg) (Op, error) {
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
}

// leadingUtility parses leading-<name>, leading-<n> and leading-[<value>].
func leadingUtility(c *pctx, a *arg) (Op, error) {
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
}

// registerTextTransform registers uppercase, lowercase, capitalize and
// normal-case.
func registerTextTransform(r *registry) {
	r.static("uppercase", op(func(e *env) { e.s.Text.TextTransform = text.TransformUppercase }))
	r.static("lowercase", op(func(e *env) { e.s.Text.TextTransform = text.TransformLowercase }))
	r.static("capitalize", op(func(e *env) { e.s.Text.TextTransform = text.TransformCapitalize }))
	r.static("normal-case", op(func(e *env) { e.s.Text.TextTransform = text.TransformNone }))
}

// registerTextDecoration registers underline and its relatives, decoration-*
// and underline-offset-*.
func registerTextDecoration(r *registry) {
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
	r.add("decoration", false, decorationThicknessUtility, func(*Theme) []string { return []string{"0", "1", "2", "4", "8"} })
	r.colorUtil("decoration", slotDecoration, nil)
	r.add("underline-offset", true, underlineOffsetUtility, func(*Theme) []string { return []string{"auto", "0", "1", "2", "4", "8"} })
}

// decorationThicknessUtility parses decoration-<n> and decoration-[<length>].
func decorationThicknessUtility(c *pctx, a *arg) (Op, error) {
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
}

// underlineOffsetUtility parses underline-offset-<n|auto> and
// underline-offset-[<length>].
func underlineOffsetUtility(c *pctx, a *arg) (Op, error) {
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
}

// registerTextWrapping registers white-space, word breaking, overflow wrap
// and truncate.
func registerTextWrapping(r *registry) {
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
}

// registerTextBlocks registers align-* and line-clamp-*.
func registerTextBlocks(r *registry) {
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

	r.add("line-clamp", false, lineClampUtility, func(*Theme) []string {
		out := []string{"none"}
		for i := 1; i <= 6; i++ {
			out = append(out, strconv.Itoa(i))
		}
		return out
	})
}

// lineClampUtility parses line-clamp-N (clips like Tailwind's
// overflow: hidden).
func lineClampUtility(c *pctx, a *arg) (Op, error) {
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
}

// registerNumericVariants registers the font-variant-numeric classes.
func registerNumericVariants(r *registry) {
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
