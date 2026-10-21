package style

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// colorValue resolves a color token inside an arbitrary value (a gradient
// stop, a shadow color): a theme color name, or a CSS color.
func (c *pctx) colorValue(tok string) (colorSpec, bool, error) {
	if spec, ok := themeColor(c.th, tok); ok {
		if spec.token != "" {
			// An arbitrary value is resolved once, when it is parsed; a
			// token depends on the element's color scheme.
			return colorSpec{}, true, fmt.Errorf("the theme token %q cannot be used inside an arbitrary value; use a color utility (from-%s, shadow-%s) or a literal color", tok, tok, tok)
		}
		return spec, true, nil
	}
	if !looksLikeColor(tok) {
		return colorSpec{}, false, nil
	}
	spec, err := parseColor(tok)
	if err != nil {
		return colorSpec{}, true, err
	}
	return spec, true, nil
}

var backgroundPositions = map[string][2]float32{
	"center": {50, 50}, "top": {50, 0}, "bottom": {50, 100}, "left": {0, 50}, "right": {100, 50},
	"top-left": {0, 0}, "left-top": {0, 0}, "top-right": {100, 0}, "right-top": {100, 0},
	"bottom-left": {0, 100}, "left-bottom": {0, 100}, "bottom-right": {100, 100}, "right-bottom": {100, 100},
}

func registerColors(r *registry) {
	// Background keywords: image none, size, position and repeat.
	bgKeys := map[string]Op{
		"none":    op(func(e *env) { e.sc.bgImage = bgImage{kind: 3} }),
		"cover":   op(func(e *env) { e.sc.bgSize = BackgroundSize{Kind: SizeCover} }),
		"contain": op(func(e *env) { e.sc.bgSize = BackgroundSize{Kind: SizeContain} }),
		"auto":    op(func(e *env) { e.sc.bgSize = BackgroundSize{Kind: SizeAutoImage} }),
		"repeat":  bgRepeatOp(RepeatBoth), "no-repeat": bgRepeatOp(RepeatNone),
		"repeat-x": bgRepeatOp(RepeatX), "repeat-y": bgRepeatOp(RepeatY),
		"repeat-round": bgRepeatOp(RepeatRound), "repeat-space": bgRepeatOp(RepeatSpace),
	}
	for _, k := range sortedMapKeys(backgroundPositions) {
		p := backgroundPositions[k]
		bgKeys[k] = op(func(e *env) {
			e.sc.bgPosX, e.sc.bgPosY, e.sc.bgPosSet = PctLP(p[0]), PctLP(p[1]), true
		})
	}
	r.enum("bg", false, bgKeys)
	for _, k := range []string{"fixed", "local", "scroll"} {
		r.unsupported["bg-"+k] = "background-attachment is not supported; use position: fixed or sticky on a layer instead"
	}

	// bg-[...]: image, gradient function, size, position or color.
	r.add("bg", false, bgArbitrary, nil)
	r.colorUtil("bg", slotBg, nil)

	r.add("bg-position", false, func(c *pctx, a *arg) (Op, error) {
		if !a.arb || a.hasMod {
			if p, ok := backgroundPositions[a.value]; ok && !a.hasMod {
				return op(func(e *env) { e.sc.bgPosX, e.sc.bgPosY, e.sc.bgPosSet = PctLP(p[0]), PctLP(p[1]), true }), nil
			}
			return Op{}, errNoMatch
		}
		return bgPositionOp(a.inner)
	}, func(*Theme) []string { return sortedMapKeys(backgroundPositions) })
	r.add("bg-size", false, func(c *pctx, a *arg) (Op, error) {
		if !a.arb || a.hasMod {
			return Op{}, errNoMatch
		}
		return bgSizeOp(a.inner)
	}, nil)

	// Clip and origin.
	boxes := map[string]BackgroundBox{"border": BackgroundBorderBox, "padding": BackgroundPaddingBox, "content": BackgroundContentBox}
	clip, origin := map[string]Op{}, map[string]Op{}
	for _, k := range sortedMapKeys(boxes) {
		b := boxes[k]
		clip[k] = op(func(e *env) { e.s.BackgroundClip = b })
		origin[k] = op(func(e *env) { e.sc.bgOrigin, e.sc.bgOriginOK = b, true })
	}
	clip["text"] = op(func(e *env) { e.s.BackgroundClip = BackgroundText })
	r.enum("bg-clip", false, clip)
	r.enum("bg-origin", false, origin)

	// Gradients.
	for _, g := range []struct {
		root string
		kind GradientKind
	}{{"bg-linear", GradientLinear}, {"bg-radial", GradientRadial}, {"bg-conic", GradientConic}} {
		g := g
		r.add(g.root, true, func(c *pctx, a *arg) (Op, error) { return gradientUtility(c, a, g.kind) },
			func(*Theme) []string { return gradientValues(g.kind) })
	}
	for _, d := range []string{"t", "tr", "r", "br", "b", "bl", "l", "tl"} {
		r.alias("bg-gradient-to-"+d, "bg-linear-to-"+d)
	}

	// Gradient stops: from-*, via-* and to-* take a color or a position.
	for _, s := range []struct {
		root string
		slot colorSlot
		pos  func(*scratch) *StopPos
	}{
		{"from", slotFrom, func(sc *scratch) *StopPos { return &sc.fromPos }},
		{"via", slotVia, func(sc *scratch) *StopPos { return &sc.viaPos }},
		{"to", slotTo, func(sc *scratch) *StopPos { return &sc.toPos }},
	} {
		s := s
		r.add(s.root, false, func(c *pctx, a *arg) (Op, error) {
			// A position: from-10%, from-[30px].
			if !a.hasMod {
				var raw string
				if a.arb {
					if a.hint == "" || a.hint == "length" || a.hint == "percentage" {
						raw = a.inner
					}
				} else if strings.HasSuffix(a.value, "%") {
					raw = a.value
				}
				if raw != "" && !looksLikeColor(raw) {
					e, err := parseLengthExpr(raw, true)
					if err == nil {
						lp, err := trackLP(e, c.th.RootFontSize)
						if err != nil {
							return Op{}, err
						}
						return op(func(e *env) { *s.pos(e.sc) = StopPos{Set: true, LP: lp} }), nil
					}
					if a.hint != "" {
						return Op{}, err
					}
				}
			}
			spec, err := colorArg(c, a)
			if err != nil {
				return Op{}, err
			}
			return op(func(e *env) { e.sc.colors[s.slot] = spec }), nil
		}, func(th *Theme) []string {
			return append(colorKeys(th), "0%", "10%", "25%", "50%", "75%", "90%", "100%")
		})
	}
}

func bgRepeatOp(rp BackgroundRepeat) Op { return op(func(e *env) { e.sc.bgRepeat = rp }) }

func gradientValues(kind GradientKind) []string {
	switch kind {
	case GradientLinear:
		return []string{"to-t", "to-tr", "to-r", "to-br", "to-b", "to-bl", "to-l", "to-tl", "0", "45", "90", "135", "180", "225", "270", "315"}
	case GradientConic:
		return []string{"", "0", "45", "90", "135", "180", "225", "270", "315"}
	}
	return []string{""}
}

var cornerDirs = map[string]struct {
	angle  float32
	corner Corner
}{
	"to-t": {0, CornerNone}, "to-r": {90, CornerNone}, "to-b": {180, CornerNone}, "to-l": {270, CornerNone},
	"to-tr": {0, CornerTopRight}, "to-br": {0, CornerBottomRight}, "to-bl": {0, CornerBottomLeft}, "to-tl": {0, CornerTopLeft},
}

// gradientUtility handles bg-linear-*, bg-radial-* and bg-conic-*.
func gradientUtility(c *pctx, a *arg, kind GradientKind) (Op, error) {
	g := newGradient(kind)
	if a.hasMod {
		switch a.mod {
		case "srgb":
			g.Interp = gfx.InterpSRGB
		case "oklab":
			g.Interp = gfx.InterpOklab
		case "oklch", "hsl", "longer", "shorter", "increasing", "decreasing":
			return Op{}, unsupportedErr("only the srgb and oklab interpolation modifiers are supported (/srgb, /oklab)")
		default:
			return Op{}, fmt.Errorf("unknown interpolation modifier /%s; use /srgb or /oklab", a.mod)
		}
	}
	value := a.value
	switch {
	case a.arb:
		gr, err := parseGradientList(c, kind, a.inner)
		if err != nil {
			return Op{}, err
		}
		gr.Interp = g.Interp
		g = gr
	case kind == GradientLinear:
		if d, ok := cornerDirs[value]; ok {
			g.Angle, g.Corner = d.angle, d.corner
		} else if n, ok := parseFloat(value); ok && n >= 0 {
			g.Angle = n
			if a.neg {
				g.Angle = -n
			}
		} else {
			return Op{}, errNoMatch
		}
	case kind == GradientConic:
		if value != "" {
			n, ok := parseFloat(value)
			if !ok || n < 0 {
				return Op{}, errNoMatch
			}
			g.Angle = n
			if a.neg {
				g.Angle = -n
			}
		}
	case kind == GradientRadial:
		if value != "" {
			return Op{}, errNoMatch
		}
	}
	if a.neg && a.arb {
		return Op{}, errNoMatch
	}
	return op(func(e *env) { e.sc.bgImage = bgImage{kind: 1, gradient: g} }), nil
}

var gradientFuncs = map[string]GradientKind{
	"linear-gradient(": GradientLinear, "radial-gradient(": GradientRadial, "conic-gradient(": GradientConic,
}

// bgArbitrary handles bg-[...] values that are not colors: url(), gradient
// functions, sizes and positions.
func bgArbitrary(c *pctx, a *arg) (Op, error) {
	if !a.arb || a.hasMod {
		return Op{}, errNoMatch
	}
	in := strings.TrimSpace(a.inner)
	if in == "" {
		return Op{}, fmt.Errorf("the value is empty; use e.g. bg-[url(/a.png)], bg-[#fff] or bg-[linear-gradient(...)]")
	}
	switch a.hint {
	case "color":
		return Op{}, errNoMatch
	case "url", "image":
		return bgImageOp(c, in)
	case "length", "size", "bg-size", "percentage":
		return bgSizeOp(in)
	case "position":
		return bgPositionOp(in)
	case "":
	default:
		return Op{}, errNoMatch
	}
	if items := splitTop(in, ','); len(items) > 1 && bgImageItem(items[0]) {
		// Several layers, topmost first: bg-[linear-gradient(...),url(a.jpg)].
		list := make([]bgImage, len(items))
		for i, it := range items {
			img, err := parseBgImage(c, strings.TrimSpace(it))
			if err != nil {
				return Op{}, err
			}
			list[i] = img
		}
		return op(func(e *env) { e.sc.bgImage = bgImage{kind: 5, list: list} }), nil
	}
	if bgImageItem(in) {
		img, err := parseBgImage(c, in)
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) { e.sc.bgImage = img }), nil
	}
	if strings.Contains(in, "gradient(") {
		return Op{}, unsupportedErr("only linear-gradient(), radial-gradient() and conic-gradient() are supported as backgrounds (repeating gradients are not)")
	}
	if looksLikeColor(in) {
		return Op{}, errNoMatch
	}
	if looksLikeLength(strings.Fields(in)[0]) || in == "auto" || in == "cover" || in == "contain" {
		return bgSizeOp(in)
	}
	return Op{}, errNoMatch
}

func bgImageOp(c *pctx, in string) (Op, error) {
	if !strings.HasPrefix(in, "url(") || !strings.HasSuffix(in, ")") {
		return Op{}, fmt.Errorf("%q is not an image; use bg-[url(/path/to/image.png)]", in)
	}
	img, err := parseBgImage(c, in)
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) { e.sc.bgImage = img }), nil
}

// bgImageItem reports whether s starts like a background image: url() or
// a gradient function.
func bgImageItem(s string) bool {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "url(") {
		return true
	}
	for prefix := range gradientFuncs {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// parseBgImage parses one background image: url(...) or a gradient
// function.
func parseBgImage(c *pctx, in string) (bgImage, error) {
	if strings.HasPrefix(in, "url(") {
		if !strings.HasSuffix(in, ")") {
			return bgImage{}, fmt.Errorf("%q is not an image; use bg-[url(/path/to/image.png)]", in)
		}
		u := strings.TrimSpace(in[4 : len(in)-1])
		u = strings.Trim(u, `"'`)
		if u == "" {
			return bgImage{}, fmt.Errorf("url() is empty")
		}
		return bgImage{kind: 2, url: u}, nil
	}
	for _, prefix := range sortedMapKeys(gradientFuncs) {
		if strings.HasPrefix(in, prefix) && strings.HasSuffix(in, ")") {
			g, err := parseGradientList(c, gradientFuncs[prefix], in[len(prefix):len(in)-1])
			if err != nil {
				return bgImage{}, err
			}
			return bgImage{kind: 1, gradient: g}, nil
		}
	}
	return bgImage{}, fmt.Errorf("%q is not a background image; use url(...), linear-gradient(...), radial-gradient(...) or conic-gradient(...)", in)
}

func bgSizeOp(in string) (Op, error) {
	f := strings.Fields(in)
	switch {
	case len(f) == 1 && f[0] == "cover":
		return op(func(e *env) { e.sc.bgSize = BackgroundSize{Kind: SizeCover} }), nil
	case len(f) == 1 && f[0] == "contain":
		return op(func(e *env) { e.sc.bgSize = BackgroundSize{Kind: SizeContain} }), nil
	case len(f) == 1 && f[0] == "auto":
		return op(func(e *env) { e.sc.bgSize = BackgroundSize{} }), nil
	case len(f) == 0 || len(f) > 2:
		return Op{}, fmt.Errorf("invalid background size %q; use cover, contain or one or two lengths", in)
	}
	dims := make([]dimSpec, len(f))
	for i, w := range f {
		if w == "auto" {
			dims[i] = dimSpec{kind: DimAuto}
			continue
		}
		e, err := parseLengthExpr(w, true)
		if err != nil {
			return Op{}, fmt.Errorf("invalid background size %q", in)
		}
		dims[i] = dimSpec{kind: DimLength, expr: e}
	}
	if len(dims) == 1 {
		dims = append(dims, dimSpec{kind: DimAuto})
	}
	return op(func(e *env) {
		e.sc.bgSize = BackgroundSize{Kind: SizeLength, W: dims[0].dim(e), H: dims[1].dim(e)}
	}), nil
}

func bgPositionOp(in string) (Op, error) {
	x, y, err := parsePositionPair(strings.Fields(in))
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) { e.sc.bgPosX, e.sc.bgPosY, e.sc.bgPosSet = x, y, true }), nil
}
