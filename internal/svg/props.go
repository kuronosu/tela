package svg

import (
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
)

// Properties: presentation attributes and the style attribute.
//
// Each element's declarations are parsed once (decls). A declaration in
// the style attribute wins over the presentation attribute of the same
// property; an invalid value is dropped at parse time, so it never hides
// a valid one (CSS). "inherit" takes the parent's computed value. The
// cascade (cascade) then computes an element's style from its parent's.

// prop identifies a property.
type prop uint8

const (
	// Inherited properties.
	pFill prop = iota
	pFillOpacity
	pFillRule
	pStroke
	pStrokeWidth
	pStrokeOpacity
	pStrokeLinecap
	pStrokeLinejoin
	pStrokeMiterlimit
	pStrokeDasharray
	pStrokeDashoffset
	pColor
	pVisibility
	pClipRule
	pFontSize
	// Not inherited.
	pOpacity
	pDisplay
	pClipPath
	pMask
	pStopColor
	pStopOpacity
	pOverflow
	pMaskType
	numProps
)

var propNames = map[string]prop{
	"fill": pFill, "fill-opacity": pFillOpacity, "fill-rule": pFillRule,
	"stroke": pStroke, "stroke-width": pStrokeWidth, "stroke-opacity": pStrokeOpacity,
	"stroke-linecap": pStrokeLinecap, "stroke-linejoin": pStrokeLinejoin,
	"stroke-miterlimit": pStrokeMiterlimit, "stroke-dasharray": pStrokeDasharray,
	"stroke-dashoffset": pStrokeDashoffset, "color": pColor, "visibility": pVisibility,
	"clip-rule": pClipRule, "font-size": pFontSize, "opacity": pOpacity, "display": pDisplay,
	"clip-path": pClipPath, "mask": pMask, "stop-color": pStopColor, "stop-opacity": pStopOpacity,
	"overflow": pOverflow, "mask-type": pMaskType,
}

// paintKind is the kind of an SVG <paint>.
type paintKind uint8

const (
	paintNone paintKind = iota
	paintColor
	paintCurrent
	paintURL
)

// paintDecl is a declared <paint>: none, a color, currentColor, or a
// reference with an optional fallback (none, a color or currentColor).
type paintDecl struct {
	kind     paintKind
	color    gfx.Color
	id       string    // paintURL: the referenced id ("" for an external URL)
	fallback paintKind // paintURL: paintNone means no fallback unless hasFB
	fbColor  gfx.Color
	hasFB    bool
}

func parsePaint(s string) (paintDecl, bool) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "none") {
		return paintDecl{kind: paintNone}, true
	}
	if id, rest, ok := scanFuncIRI(s); ok {
		p := paintDecl{kind: paintURL, id: id}
		if rest == "" {
			return p, true
		}
		p.hasFB = true
		if strings.EqualFold(rest, "none") {
			p.fallback = paintNone
			return p, true
		}
		c, ok := parseColor(rest)
		if !ok {
			return paintDecl{}, false
		}
		p.fallback, p.fbColor = paintColor, c.c
		if c.current {
			p.fallback = paintCurrent
		}
		return p, true
	}
	c, ok := parseColor(s)
	if !ok {
		return paintDecl{}, false
	}
	if c.current {
		return paintDecl{kind: paintCurrent}, true
	}
	return paintDecl{kind: paintColor, color: c.c}, true
}

// decls are an element's parsed declarations.
type decls struct {
	set, inherit uint32 // bit p: property p is declared / declared as inherit

	fill, stroke                                        paintDecl
	fillOpacity, strokeOpacity, opacity, stopOpacity    float32
	fillRule, clipRule                                  gfx.FillRule
	strokeWidth, dashOffset, fontSize                   length
	linecap                                             gfx.LineCap
	linejoin                                            gfx.LineJoin
	miterlimit                                          float32
	dashes                                              []length // nil: none
	color, stopColor                                    colorVal
	hidden, displayNone, overflowVisible, maskTypeAlpha bool
	clipPath, mask                                      string // "" with the bit set: none
}

func (d *decls) has(p prop) bool { return d.set&(1<<p) != 0 }

func (d *decls) inherits(p prop) bool { return d.inherit&(1<<p) != 0 }

// declsOf returns the parsed declarations of e, parsing them on first use:
// its presentation attributes, the normal declarations of the style
// sheet rules that match it (css.go) and of its style attribute, then the
// important ones of the rules and of the style attribute.
func declsOf(e *element) *decls {
	if e.decl != nil {
		return e.decl
	}
	d := &decls{}
	for _, a := range e.attrs {
		if p, ok := propNames[a.name]; ok {
			d.declare(p, a.value)
		}
	}
	st, hasStyle := e.attr("style")
	for _, important := range [2]bool{false, true} {
		for _, sd := range e.sheet {
			if sd.important == important {
				d.declare(propNames[sd.name], sd.value)
			}
		}
		if hasStyle {
			forEachDeclaration(st, func(name, value string, imp bool) {
				if p, ok := propNames[strings.ToLower(name)]; ok && imp == important {
					d.declare(p, value)
				}
			})
		}
	}
	e.decl = d
	return d
}

// forEachDeclaration splits a declaration block (a style attribute) into
// declarations, dropping comments; important reports a !important.
func forEachDeclaration(s string, fn func(name, value string, important bool)) {
	s = stripComments(s)
	for _, part := range strings.Split(s, ";") {
		name, value, ok := strings.Cut(part, ":")
		if !ok {
			continue
		}
		name, value = strings.TrimSpace(name), strings.TrimSpace(value)
		important := false
		if i := strings.Index(strings.ToLower(value), "!important"); i >= 0 {
			value, important = strings.TrimSpace(value[:i]), true
		}
		if name != "" && value != "" {
			fn(name, value, important)
		}
	}
}

// declare records a declaration of p if value is valid for it.
func (d *decls) declare(p prop, value string) {
	value = strings.TrimSpace(value)
	if value == "inherit" {
		d.set |= 1 << p
		d.inherit |= 1 << p
		return
	}
	// The parsers below store into d as they go; an invalid value restores
	// what an earlier declaration (the presentation attribute) set.
	saved := *d
	ok := true
	switch p {
	case pFill, pStroke:
		var v paintDecl
		if v, ok = parsePaint(value); ok {
			if p == pFill {
				d.fill = v
			} else {
				d.stroke = v
			}
		}
	case pFillOpacity:
		d.fillOpacity, ok = parseAlpha(value)
	case pStrokeOpacity:
		d.strokeOpacity, ok = parseAlpha(value)
	case pOpacity:
		d.opacity, ok = parseAlpha(value)
	case pStopOpacity:
		d.stopOpacity, ok = parseAlpha(value)
	case pFillRule, pClipRule:
		var r gfx.FillRule
		switch value {
		case "nonzero":
			r = gfx.NonZero
		case "evenodd":
			r = gfx.EvenOdd
		default:
			ok = false
		}
		if p == pFillRule {
			d.fillRule = r
		} else {
			d.clipRule = r
		}
	case pStrokeWidth:
		d.strokeWidth, ok = parseLength(value)
		ok = ok && d.strokeWidth.v >= 0
	case pStrokeDashoffset:
		d.dashOffset, ok = parseLength(value)
	case pFontSize:
		d.fontSize, ok = parseLength(value)
		ok = ok && d.fontSize.v >= 0
	case pStrokeLinecap:
		switch value {
		case "butt":
			d.linecap = gfx.CapButt
		case "round":
			d.linecap = gfx.CapRound
		case "square":
			d.linecap = gfx.CapSquare
		default:
			ok = false
		}
	case pStrokeLinejoin:
		// Like Chromium, miter-clip and arcs (SVG 2) are not supported
		// values: the declaration is dropped and the inherited join used.
		switch value {
		case "miter":
			d.linejoin = gfx.JoinMiter
		case "round":
			d.linejoin = gfx.JoinRound
		case "bevel":
			d.linejoin = gfx.JoinBevel
		default:
			ok = false
		}
	case pStrokeMiterlimit:
		var v float64
		v, ok = parseNumber(value)
		ok = ok && v >= 1
		d.miterlimit = float32(v)
	case pStrokeDasharray:
		d.dashes, ok = parseDashArray(value)
	case pColor:
		d.color, ok = parseColor(value)
	case pStopColor:
		d.stopColor, ok = parseColor(value)
	case pVisibility:
		switch value {
		case "visible":
			d.hidden = false
		case "hidden", "collapse":
			d.hidden = true
		default:
			ok = false
		}
	case pDisplay:
		d.displayNone = value == "none"
	case pOverflow:
		switch value {
		case "visible", "auto":
			d.overflowVisible = true
		case "hidden", "scroll", "clip":
			d.overflowVisible = false
		default:
			ok = false
		}
	case pMaskType:
		switch value {
		case "luminance":
			d.maskTypeAlpha = false
		case "alpha":
			d.maskTypeAlpha = true
		default:
			ok = false
		}
	case pClipPath, pMask:
		id := "" // none
		if value != "none" {
			var rest string
			id, rest, ok = scanFuncIRI(value)
			ok = ok && rest == ""
			if ok && id == "" {
				id = "\x00" // an external reference: valid, matches no element
			}
		}
		if p == pClipPath {
			d.clipPath = id
		} else {
			d.mask = id
		}
	}
	if !ok {
		*d = saved
		return
	}
	d.set |= 1 << p
	d.inherit &^= 1 << p
}

// parseDashArray parses stroke-dasharray: none, or lengths separated by
// commas and/or white space. A negative length invalidates the list.
// The returned slice is nil for none.
func parseDashArray(s string) ([]length, bool) {
	if s == "none" {
		return nil, true
	}
	var out []length
	i := skipSpace(s, 0)
	for i < len(s) {
		l, j, ok := scanLength(s, i)
		if !ok || l.v < 0 || len(out) >= 1024 {
			return nil, false
		}
		out = append(out, l)
		k := skipSpace(s, j)
		if k < len(s) && s[k] == ',' {
			k = skipSpace(s, k+1)
			if k == len(s) {
				return nil, false
			}
		}
		i = k
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// Root dependencies: bits of the inherited properties whose value at an
// element comes from the root element (the root's own declaration or the
// initial value), which Options.Root overrides.
const (
	fromRootFill uint8 = 1 << iota
	fromRootStroke
	fromRootStrokeWidth
	fromRootColor
)

// style is the computed style of an element.
type style struct {
	// Inherited.
	fill, stroke               paintDecl
	fillOpacity, strokeOpacity float32
	fillRule, clipRule         gfx.FillRule
	strokeWidth                length // em resolved to px; % kept
	dashOffset                 length
	linecap                    gfx.LineCap
	linejoin                   gfx.LineJoin
	miterlimit                 float32
	dashes                     []length
	color                      gfx.Color // valid without fromRootColor
	hidden                     bool
	fontSize                   float32
	rootFontSize               float32
	fromRoot                   uint8

	// Not inherited.
	opacity                      float32
	displayNone, overflowVisible bool
	maskTypeAlpha                bool
	clipPath, mask               string
	hasClipPath, hasMask         bool
	stopColor                    colorVal
	stopOpacity                  float32
}

// initialStyle is the style the root inherits from: the initial values.
func initialStyle() style {
	return style{
		fill:          paintDecl{kind: paintColor, color: gfx.Black},
		fillOpacity:   1,
		strokeOpacity: 1,
		strokeWidth:   length{1, uNone},
		miterlimit:    gfx.DefaultMiterLimit,
		color:         gfx.Black,
		fontSize:      defaultFontSize,
		rootFontSize:  defaultFontSize,
		fromRoot:      fromRootFill | fromRootStroke | fromRootStrokeWidth | fromRootColor,
		opacity:       1,
		stopColor:     colorVal{c: gfx.Black},
		stopOpacity:   1,
	}
}

// cascade computes the style of an element with declarations d whose
// parent has the computed style parent. isRoot keeps the root-dependency
// bits even for the root's own declarations (Options.Root overrides
// them).
func cascade(d *decls, parent *style, isRoot bool) style {
	s := *parent
	// Non-inherited properties start from their initial values.
	s.opacity, s.displayNone, s.overflowVisible, s.maskTypeAlpha = 1, false, false, false
	s.clipPath, s.mask, s.hasClipPath, s.hasMask = "", "", false, false
	s.stopColor, s.stopOpacity = colorVal{c: gfx.Black}, 1

	own := func(p prop) bool { return d.has(p) && !d.inherits(p) }
	fromRoot := func(p prop, bit uint8) {
		if own(p) && !isRoot {
			s.fromRoot &^= bit
		}
	}
	if own(pFontSize) {
		switch d.fontSize.u {
		case uPercent:
			s.fontSize = float32(float64(d.fontSize.v) * float64(parent.fontSize) / 100)
		default:
			s.fontSize = d.fontSize.resolve(axisX, viewport{}, parent.fonts())
		}
	}
	if isRoot {
		s.rootFontSize = s.fontSize
	}
	if own(pFill) {
		s.fill = d.fill
	}
	fromRoot(pFill, fromRootFill)
	if own(pStroke) {
		s.stroke = d.stroke
	}
	fromRoot(pStroke, fromRootStroke)
	if own(pStrokeWidth) {
		s.strokeWidth = absLength(d.strokeWidth, s.fonts())
	}
	fromRoot(pStrokeWidth, fromRootStrokeWidth)
	if own(pColor) && !d.color.current { // color: currentColor is inherit
		s.color = d.color.c
		if !isRoot {
			s.fromRoot &^= fromRootColor
		}
	}
	if own(pFillOpacity) {
		s.fillOpacity = d.fillOpacity
	}
	if own(pStrokeOpacity) {
		s.strokeOpacity = d.strokeOpacity
	}
	if own(pFillRule) {
		s.fillRule = d.fillRule
	}
	if own(pClipRule) {
		s.clipRule = d.clipRule
	}
	if own(pStrokeDashoffset) {
		s.dashOffset = absLength(d.dashOffset, s.fonts())
	}
	if own(pStrokeLinecap) {
		s.linecap = d.linecap
	}
	if own(pStrokeLinejoin) {
		s.linejoin = d.linejoin
	}
	if own(pStrokeMiterlimit) {
		s.miterlimit = d.miterlimit
	}
	if own(pStrokeDasharray) {
		s.dashes = d.dashes
		if len(s.dashes) > 0 {
			ds := make([]length, len(s.dashes))
			for i, l := range s.dashes {
				ds[i] = absLength(l, s.fonts())
			}
			s.dashes = ds
		}
	}
	if own(pVisibility) {
		s.hidden = d.hidden
	}
	// Non-inherited: own value, or the parent's for inherit.
	pick := func(p prop) (mine, inh bool) { return own(p), d.has(p) && d.inherits(p) }
	if m, i := pick(pOpacity); m {
		s.opacity = d.opacity
	} else if i {
		s.opacity = parent.opacity
	}
	if m, i := pick(pDisplay); m {
		s.displayNone = d.displayNone
	} else if i {
		s.displayNone = parent.displayNone
	}
	if m, i := pick(pOverflow); m {
		s.overflowVisible = d.overflowVisible
	} else if i {
		s.overflowVisible = parent.overflowVisible
	}
	if m, i := pick(pMaskType); m {
		s.maskTypeAlpha = d.maskTypeAlpha
	} else if i {
		s.maskTypeAlpha = parent.maskTypeAlpha
	}
	if m, i := pick(pClipPath); m {
		s.clipPath, s.hasClipPath = d.clipPath, d.clipPath != ""
	} else if i {
		s.clipPath, s.hasClipPath = parent.clipPath, parent.hasClipPath
	}
	if m, i := pick(pMask); m {
		s.mask, s.hasMask = d.mask, d.mask != ""
	} else if i {
		s.mask, s.hasMask = parent.mask, parent.hasMask
	}
	if m, i := pick(pStopColor); m {
		s.stopColor = d.stopColor
	} else if i {
		s.stopColor = parent.stopColor
	}
	if m, i := pick(pStopOpacity); m {
		s.stopOpacity = d.stopOpacity
	} else if i {
		s.stopOpacity = parent.stopOpacity
	}
	return s
}

// absLength resolves font-relative units, which compute to px at the
// declaring element (CSS); percentages stay relative.
func absLength(l length, f fonts) length {
	switch l.u {
	case uPercent, uNone:
		return l
	}
	return length{l.resolve(axisX, viewport{}, f), uNone}
}

func (s *style) fonts() fonts { return fonts{s.fontSize, s.rootFontSize} }
