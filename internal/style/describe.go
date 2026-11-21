package style

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// This file describes a class in CSS terms, for editors (tela lsp: hover and
// the detail of a completion): what `p-4` or `bg-red-500` set, after the
// theme is resolved. It is table-driven, from the class name and the theme,
// and does not run the class's op: ops are closures over a ComputedStyle,
// which has no CSS names. The tables cover the common utilities; a valid
// class they do not know is described by its utility name only
// (Description.Utility), never guessed.

// Declaration is one CSS declaration, such as padding: 1rem.
type Declaration struct {
	Property, Value string
}

// String returns "property: value".
func (d Declaration) String() string { return d.Property + ": " + d.Value }

// Description is what a class does, in CSS terms (Parser.Describe).
type Description struct {
	// Class is the class as written.
	Class string
	// Variants say, in words, when the class applies: "on hover", "in dark
	// mode", "when the window is at least 768px wide (md)". Empty when
	// always.
	Variants []string
	// Declarations are the CSS declarations of the class after theme
	// resolution; empty when the tables do not know the utility (then
	// Utility names it) or for an alias (see Expansion).
	Declarations []Declaration
	// Utility is the utility the class is, without variants and sign, such
	// as "p" or "bg": set when Declarations is empty.
	Utility string
	// Expansion is the class string an alias expands to, and Parts the
	// description of each of its classes.
	Expansion string
	Parts     []Description
}

// descOnce builds the describer's tables on first use, so that apps,
// which never describe a class, do not link them (ADR-301).
var descOnce sync.Once

func descLoad() {
	descOnce.Do(func() {
		descInitStatic()
		descInitBorderSides()
		descInitRadiusSides()
		descInitGradientDir()
		descInitVariantWords()
		descInitRoots()
		descInitMore()
	})
}

// Describe describes class, a single class with its variants (hover:p-4).
// The error is the compile error of the class (an UnknownClassError and
// the like) when it is not valid; the description then has the class and
// its variants only.
func (p *Parser) Describe(class string) (Description, error) {
	descLoad()
	return p.describe(class, nil)
}

func (p *Parser) describe(class string, stack []string) (Description, error) {
	d := Description{Class: class}
	if err := p.Compile(class).Err; err != nil {
		return d, err
	}
	parts := splitTop(class, ':')
	util := parts[len(parts)-1]
	for _, v := range parts[:len(parts)-1] {
		d.Variants = append(d.Variants, describeVariant(p.th, v))
	}
	if exp, ok := p.aliases[util]; ok && !slices.Contains(stack, util) {
		d.Expansion = exp
		stack = append(stack, util)
		for _, tok := range strings.Fields(exp) {
			part, err := p.describe(tok, stack)
			if err != nil {
				continue
			}
			d.Parts = append(d.Parts, part)
		}
		return d, nil
	}
	c := &descCtx{th: p.th}
	if strings.HasPrefix(util, "-") {
		c.neg, util = true, util[1:]
	}
	d.Declarations = c.declarations(util)
	if len(d.Declarations) == 0 {
		d.Utility = utilityRoot(util)
	}
	return d, nil
}

// utilityRoot returns the root of the utility util: the longest prefix up
// to a dash that the registry knows as a root.
func utilityRoot(util string) string {
	if len(reg.byRoot[util]) > 0 {
		return util
	}
	for i := len(util) - 1; i > 0; i-- {
		if util[i] == '-' && len(reg.byRoot[util[:i]]) > 0 {
			return util[:i]
		}
	}
	if i := strings.IndexByte(util, '-'); i > 0 {
		return util[:i]
	}
	return util
}

// descCtx resolves the values of one class.
type descCtx struct {
	th  *Theme
	neg bool
}

// descFn describes root-v: v is what follows the root and its dash ("" for
// the bare root), with the modifier ("/50") still on it. It returns nil
// when v is not one of the root's values.
type descFn func(c *descCtx, v string) []Declaration

func (c *descCtx) declarations(util string) []Declaration {
	if !c.neg {
		if ds, ok := descStatic[util]; ok {
			return ds
		}
	}
	if fn, ok := descRoots[util]; ok {
		if ds := fn(c, ""); ds != nil {
			return ds
		}
	}
	depth := 0
	var dashes []int
	for i := 0; i < len(util); i++ {
		switch util[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case '-':
			if depth == 0 {
				dashes = append(dashes, i)
			}
		}
	}
	for i := len(dashes) - 1; i >= 0; i-- {
		d := dashes[i]
		if fn, ok := descRoots[util[:d]]; ok {
			if ds := fn(c, util[d+1:]); ds != nil {
				return ds
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Values.

// num formats a number the way CSS writes it.
func num(f float32) string { return strconv.FormatFloat(float64(f), 'f', -1, 32) }

// sig4 formats a ratio with four significant digits.
func sig4(f float64) string { return strconv.FormatFloat(f, 'g', 4, 64) }

// numF formats a computed number without float32 noise.
func numF(f float64) string {
	return strconv.FormatFloat(float64(float32(f)), 'f', -1, 32)
}

// arbitrary returns the content of an arbitrary value, [..] or (--var),
// with the underscores of Tailwind's syntax turned into spaces.
func arbitrary(v string) (string, bool) {
	switch {
	case strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]"):
		return strings.ReplaceAll(v[1:len(v)-1], "_", " "), true
	case strings.HasPrefix(v, "(") && strings.HasSuffix(v, ")"):
		return "var(" + v[1:len(v)-1] + ")", true
	}
	return "", false
}

// negate gives a length its sign.
func (c *descCtx) negate(s string) string {
	if !c.neg {
		return s
	}
	switch {
	case s == "0" || s == "auto":
		return s
	case strings.HasPrefix(s, "-"):
		return s[1:]
	case s != "" && (s[0] == '.' || (s[0] >= '0' && s[0] <= '9')):
		return "-" + s
	}
	return "calc(" + s + " * -1)"
}

// spacing resolves a value of the spacing scale: a number of the theme's
// spacing unit, px, a fraction or an arbitrary length.
func (c *descCtx) spacing(v string) (string, bool) {
	if a, ok := arbitrary(v); ok {
		return c.negate(a), true
	}
	if v == "px" {
		return c.negate("1px"), true
	}
	if n, err := strconv.ParseFloat(v, 64); err == nil && n >= 0 {
		if n == 0 {
			return "0", true
		}
		return c.negate(numF(n*float64(c.th.Spacing.Value)) + c.th.Spacing.Unit.String()), true
	}
	if f, ok := fraction(v); ok {
		return c.negate(f), true
	}
	return "", false
}

// fraction turns "1/2" into "50%".
func fraction(v string) (string, bool) {
	a, b, ok := strings.Cut(v, "/")
	if !ok {
		return "", false
	}
	x, err1 := strconv.ParseFloat(a, 64)
	y, err2 := strconv.ParseFloat(b, 64)
	if err1 != nil || err2 != nil || y == 0 {
		return "", false
	}
	return numF(x/y*100) + "%", true
}

// size resolves a width, height or inset value; vh is true for the
// properties along the vertical axis.
func (c *descCtx) size(v string, vertical, containers bool) (string, bool) {
	switch v {
	case "auto":
		return "auto", true
	case "full":
		return c.negate("100%"), true
	case "screen":
		if vertical {
			return "100vh", true
		}
		return "100vw", true
	case "min":
		return "min-content", true
	case "max":
		return "max-content", true
	case "fit":
		return "fit-content", true
	case "none":
		return "none", true
	case "svw", "lvw", "dvw", "svh", "lvh", "dvh":
		return "100" + v, true
	case "lh":
		return "1lh", true
	}
	if containers {
		if l, ok := c.th.Containers[v]; ok {
			return l.String(), true
		}
		if v == "prose" {
			return "65ch", true
		}
		if bp, ok := strings.CutPrefix(v, "screen-"); ok {
			if l, ok := c.th.Breakpoints[bp]; ok {
				return c.negate(l.String()), true
			}
		}
	}
	return c.spacing(v)
}

// splitAlpha splits a color value "red-500/50" into the color and the
// opacity as written ("50", "[.3]"; empty when there is none).
func splitAlpha(v string) (color, alpha string) {
	if i := lastTopSlash(v); i > 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

// alphaValue turns a color modifier into a number in [0, 1].
func alphaValue(a string) (float32, bool) {
	if in, ok := arbitrary(a); ok {
		a = in
		if strings.HasSuffix(a, "%") {
			n, err := strconv.ParseFloat(strings.TrimSuffix(a, "%"), 64)
			return float32(n / 100), err == nil
		}
		n, err := strconv.ParseFloat(a, 64)
		return float32(n), err == nil
	}
	n, err := strconv.ParseFloat(a, 64)
	return float32(n / 100), err == nil
}

// color resolves a color value: a palette color, a semantic token, current,
// transparent or an arbitrary color, with an optional opacity modifier. A
// palette color is its hex value; a token has one per color scheme.
func (c *descCtx) color(v string) (string, bool) {
	name, alpha := splitAlpha(v)
	var a float32 = 1
	if alpha != "" {
		var ok bool
		if a, ok = alphaValue(alpha); !ok {
			return "", false
		}
	}
	if in, ok := arbitrary(name); ok {
		in = strings.TrimPrefix(in, "color:")
		spec, err := parseColor(in)
		if err != nil {
			return "", false
		}
		if spec.set && !spec.current && spec.token == "" {
			return spec.c.MulAlpha(a).String(), true
		}
		return in, true
	}
	switch name {
	case "transparent":
		return "transparent", true
	case "current":
		if a < 1 {
			return fmt.Sprintf("color-mix(in oklab, currentColor %s%%, transparent)", numF(float64(a)*100)), true
		}
		return "currentColor", true
	case "inherit":
		return "inherit", true
	}
	if col, ok := c.th.Colors[name]; ok {
		return col.MulAlpha(a).String(), true
	}
	if t, ok := c.th.Tokens[name]; ok {
		return fmt.Sprintf("%s in light, %s in dark (the %s color token)", t.Light.MulAlpha(a), t.Dark.MulAlpha(a), name), true
	}
	return "", false
}

// length resolves a length value of a theme namespace or an arbitrary one.
func lengthOf(m map[string]Length, v string) (string, bool) {
	if v == "" {
		v = "DEFAULT"
	}
	if l, ok := m[v]; ok {
		return l.String(), true
	}
	if a, ok := arbitrary(v); ok {
		return a, true
	}
	return "", false
}

// shadowCSS formats the layers of a box, drop or text shadow.
func shadowCSS(layers []Shadow) string {
	parts := make([]string, len(layers))
	for i, s := range layers {
		f := []string{px0(s.OffsetX), px0(s.OffsetY), px0(s.Blur)}
		if s.Spread != 0 {
			f = append(f, px0(s.Spread))
		}
		f = append(f, s.Color.String())
		if s.Inset {
			f = append([]string{"inset"}, f...)
		}
		parts[i] = strings.Join(f, " ")
	}
	return strings.Join(parts, ", ")
}

func px0(v float32) string {
	if v == 0 {
		return "0"
	}
	return num(v) + "px"
}

func (b CubicBezier) String() string {
	return fmt.Sprintf("cubic-bezier(%s, %s, %s, %s)", num(b.X1), num(b.Y1), num(b.X2), num(b.Y2))
}

// ---------------------------------------------------------------------
// The tables.

func ds(pairs ...string) []Declaration {
	out := make([]Declaration, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Declaration{pairs[i], pairs[i+1]})
	}
	return out
}

// props sets the same value on every property.
func props(properties []string, value string) []Declaration {
	out := make([]Declaration, len(properties))
	for i, p := range properties {
		out[i] = Declaration{p, value}
	}
	return out
}

// descStatic are the classes without a value.
var descStatic map[string][]Declaration

func descInitStatic() {
	descStatic = map[string][]Declaration{
		"block": ds("display", "block"), "inline-block": ds("display", "inline-block"),
		"inline": ds("display", "inline"), "flex": ds("display", "flex"),
		"inline-flex": ds("display", "inline-flex"), "grid": ds("display", "grid"),
		"inline-grid": ds("display", "inline-grid"), "contents": ds("display", "contents"),
		"flow-root": ds("display", "flow-root"), "hidden": ds("display", "none"),
		"static": ds("position", "static"), "fixed": ds("position", "fixed"),
		"absolute": ds("position", "absolute"), "relative": ds("position", "relative"),
		"sticky":  ds("position", "sticky"),
		"visible": ds("visibility", "visible"), "invisible": ds("visibility", "hidden"),
		"collapse": ds("visibility", "collapse"),
		"isolate":  ds("isolation", "isolate"), "isolation-auto": ds("isolation", "auto"),
		"truncate": ds("overflow", "hidden", "text-overflow", "ellipsis", "white-space", "nowrap"),
		"italic":   ds("font-style", "italic"), "not-italic": ds("font-style", "normal"),
		"uppercase": ds("text-transform", "uppercase"), "lowercase": ds("text-transform", "lowercase"),
		"capitalize": ds("text-transform", "capitalize"), "normal-case": ds("text-transform", "none"),
		"underline": ds("text-decoration-line", "underline"), "overline": ds("text-decoration-line", "overline"),
		"line-through": ds("text-decoration-line", "line-through"), "no-underline": ds("text-decoration-line", "none"),
		"antialiased":          ds("-webkit-font-smoothing", "antialiased"),
		"subpixel-antialiased": ds("-webkit-font-smoothing", "auto"),
		"box-border":           ds("box-sizing", "border-box"),
		"sr-only": ds("position", "absolute", "width", "1px", "height", "1px", "padding", "0", "margin", "-1px",
			"overflow", "hidden", "clip", "rect(0, 0, 0, 0)", "white-space", "nowrap", "border-width", "0"),
		"not-sr-only": ds("position", "static", "width", "auto", "height", "auto", "padding", "0", "margin", "0",
			"overflow", "visible", "clip", "auto", "white-space", "normal"),
		"grow": ds("flex-grow", "1"), "shrink": ds("flex-shrink", "1"),
		"break-normal": ds("overflow-wrap", "normal", "word-break", "normal"),
		"break-words":  ds("overflow-wrap", "break-word"), "break-all": ds("word-break", "break-all"),
		"break-keep":     ds("word-break", "keep-all"),
		"transform-none": ds("transform", "none"),
		"paused":         ds("animation-play-state", "paused"), "running": ds("animation-play-state", "running"),
		"appearance-none": ds("appearance", "none"), "appearance-auto": ds("appearance", "auto"),
		"resize-none": ds("resize", "none"),
		"group":       nil, "peer": nil,
		"tabular-nums": ds("font-variant-numeric", "tabular-nums"), "normal-nums": ds("font-variant-numeric", "normal"),
		"ordinal": ds("font-variant-numeric", "ordinal"), "slashed-zero": ds("font-variant-numeric", "slashed-zero"),
		"lining-nums": ds("font-variant-numeric", "lining-nums"), "oldstyle-nums": ds("font-variant-numeric", "oldstyle-nums"),
		"proportional-nums": ds("font-variant-numeric", "proportional-nums"),
	}
}

// enumOf describes root-key as property: value with the value mapped.
func enumOf(property string, m map[string]string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if x, ok := m[v]; ok {
			return ds(property, x)
		}
		if a, ok := arbitrary(v); ok && v != "" {
			return ds(property, a)
		}
		return nil
	}
}

// keywords is the identity map of words.
func keywords(words ...string) map[string]string {
	m := make(map[string]string, len(words))
	for _, w := range words {
		m[w] = w
	}
	return m
}

func merge(a, b map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// spaceProps describes a spacing-scale utility on the properties.
func spaceProps(properties ...string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if v == "auto" {
			return props(properties, "auto")
		}
		s, ok := c.spacing(v)
		if !ok {
			return nil
		}
		return props(properties, s)
	}
}

// sizeProps describes a sizing utility on the properties.
func sizeProps(vertical, containers bool, properties ...string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if v == "" {
			return nil
		}
		s, ok := c.size(v, vertical, containers)
		if !ok {
			return nil
		}
		return props(properties, s)
	}
}

// colorProp describes a color utility on the property.
func colorProp(property ...string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if v == "" {
			return nil
		}
		s, ok := c.color(v)
		if !ok {
			return nil
		}
		return props(property, s)
	}
}

// number reads a non-negative number value.
func number(v string) (float64, bool) {
	n, err := strconv.ParseFloat(v, 64)
	return n, err == nil && n >= 0
}

// percentOf describes a filter function with a percent value (brightness-50
// is brightness(0.5)), bare or numeric.
func filterFn(property, fn string, bare string) descFn {
	return func(c *descCtx, v string) []Declaration {
		switch {
		case v == "":
			if bare == "" {
				return nil
			}
			return ds(property, fn+"("+bare+")")
		case isNumber(v):
			n, _ := number(v)
			return ds(property, fn+"("+numF(n/100)+")")
		}
		if a, ok := arbitrary(v); ok {
			return ds(property, fn+"("+a+")")
		}
		return nil
	}
}

func isNumber(v string) bool { _, ok := number(v); return ok }

// sideTable maps the side suffix of border, rounded... to properties.
type descSideProps struct{ width, color []string }

var descBorderSides map[string]descSideProps

func descInitBorderSides() {
	descBorderSides = map[string]descSideProps{
		"":   {[]string{"border-width"}, []string{"border-color"}},
		"x":  {[]string{"border-inline-width"}, []string{"border-inline-color"}},
		"y":  {[]string{"border-block-width"}, []string{"border-block-color"}},
		"t":  {[]string{"border-top-width"}, []string{"border-top-color"}},
		"r":  {[]string{"border-right-width"}, []string{"border-right-color"}},
		"b":  {[]string{"border-bottom-width"}, []string{"border-bottom-color"}},
		"l":  {[]string{"border-left-width"}, []string{"border-left-color"}},
		"s":  {[]string{"border-inline-start-width"}, []string{"border-inline-start-color"}},
		"e":  {[]string{"border-inline-end-width"}, []string{"border-inline-end-color"}},
		"bs": {[]string{"border-block-start-width"}, []string{"border-block-start-color"}},
		"be": {[]string{"border-block-end-width"}, []string{"border-block-end-color"}},
	}
}

func borderFn(side string) descFn {
	sp := descBorderSides[side]
	return func(c *descCtx, v string) []Declaration {
		switch {
		case v == "":
			return props(sp.width, "1px")
		case isNumber(v):
			return props(sp.width, v+"px")
		case v == "solid" || v == "dashed" || v == "dotted" || v == "double" || v == "none" || v == "hidden":
			if side == "" {
				return ds("border-style", v)
			}
			return nil
		}
		if a, ok := arbitrary(v); ok {
			if strings.HasPrefix(a, "length:") {
				return props(sp.width, strings.TrimPrefix(a, "length:"))
			}
			if _, ok := c.color(v); !ok {
				return props(sp.width, a)
			}
		}
		if s, ok := c.color(v); ok {
			return props(sp.color, s)
		}
		return nil
	}
}

var descRadiusSides map[string][]string

func descInitRadiusSides() {
	descRadiusSides = map[string][]string{
		"":   {"border-radius"},
		"t":  {"border-top-left-radius", "border-top-right-radius"},
		"r":  {"border-top-right-radius", "border-bottom-right-radius"},
		"b":  {"border-bottom-right-radius", "border-bottom-left-radius"},
		"l":  {"border-top-left-radius", "border-bottom-left-radius"},
		"s":  {"border-start-start-radius", "border-end-start-radius"},
		"e":  {"border-start-end-radius", "border-end-end-radius"},
		"tl": {"border-top-left-radius"}, "tr": {"border-top-right-radius"},
		"br": {"border-bottom-right-radius"}, "bl": {"border-bottom-left-radius"},
		"ss": {"border-start-start-radius"}, "se": {"border-start-end-radius"},
		"ee": {"border-end-end-radius"}, "es": {"border-end-start-radius"},
	}
}

func roundedFn(side string) descFn {
	properties := descRadiusSides[side]
	return func(c *descCtx, v string) []Declaration {
		switch v {
		case "full":
			return props(properties, "calc(infinity * 1px)")
		case "none":
			return props(properties, "0")
		}
		if s, ok := lengthOf(c.th.Radius, v); ok {
			return props(properties, s)
		}
		return nil
	}
}

// shadowFn describes shadow, inset-shadow, drop-shadow and text-shadow.
func shadowFn(kind string) descFn {
	return func(c *descCtx, v string) []Declaration {
		var layers map[string][]Shadow
		var property string
		switch kind {
		case "shadow":
			layers, property = c.th.Shadows, "box-shadow"
		case "inset-shadow":
			layers, property = c.th.InsetShadows, "box-shadow"
		case "drop-shadow":
			layers, property = c.th.DropShadows, "filter"
		case "text-shadow":
			layers, property = c.th.TextShadows, "text-shadow"
		}
		key := v
		if key == "" {
			key = "DEFAULT"
		}
		if v == "none" {
			if kind == "drop-shadow" {
				return ds(property, "drop-shadow(0 0 #0000)")
			}
			return ds(property, "none")
		}
		if l, ok := layers[key]; ok {
			s := shadowCSS(l)
			if kind == "drop-shadow" {
				s = "drop-shadow(" + s + ")"
			}
			return ds(property, s)
		}
		if a, ok := arbitrary(v); ok {
			if _, isColor := c.color(v); !isColor {
				return ds(property, a)
			}
		}
		if s, ok := c.color(v); ok {
			return ds("--tw-"+kind+"-color", s)
		}
		return nil
	}
}

func gridTemplate(property string) descFn {
	return func(c *descCtx, v string) []Declaration {
		switch v {
		case "none", "subgrid":
			return ds(property, v)
		}
		if n, ok := number(v); ok && n >= 1 {
			return ds(property, "repeat("+v+", minmax(0, 1fr))")
		}
		if a, ok := arbitrary(v); ok {
			return ds(property, a)
		}
		return nil
	}
}

// gridLine describes col-span-2, col-start-3...
func gridLine(property, kind string) descFn {
	return func(c *descCtx, v string) []Declaration {
		switch {
		case kind == "span" && v == "full":
			return ds(property, "1 / -1")
		case kind == "span" && isNumber(v):
			return ds(property, "span "+v+" / span "+v)
		case kind != "span" && v == "auto":
			return ds(property, "auto")
		case kind != "span" && isNumber(v):
			return ds(property, v)
		}
		if a, ok := arbitrary(v); ok {
			return ds(property, a)
		}
		return nil
	}
}

func durationFn(property string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if n, ok := number(v); ok && v != "" {
			return ds(property, numF(n)+"ms")
		}
		if a, ok := arbitrary(v); ok {
			return ds(property, a)
		}
		return nil
	}
}

// transformFn describes translate, scale, rotate and skew.
func transformFn(function string, axes ...string) descFn {
	return func(c *descCtx, v string) []Declaration {
		var arg string
		switch function {
		case "translate":
			s, ok := c.size(v, false, false)
			if !ok || v == "auto" || v == "none" {
				return nil
			}
			arg = s
		case "scale":
			n, ok := number(v)
			if !ok {
				if a, isArb := arbitrary(v); isArb {
					arg = a
					break
				}
				return nil
			}
			arg = numF(c.signed(n) / 100)
		case "rotate", "skew":
			n, ok := number(v)
			if !ok {
				if a, isArb := arbitrary(v); isArb {
					arg = a
					break
				}
				return nil
			}
			arg = numF(c.signed(n)) + "deg"
		}
		var parts []string
		for _, ax := range axes {
			parts = append(parts, function+ax+"("+arg+")")
		}
		return ds("transform", strings.Join(parts, " "))
	}
}

func (c *descCtx) signed(n float64) float64 {
	if c.neg {
		return -n
	}
	return n
}

// textFn describes text-*: alignment, wrapping, a size of the type scale or
// a color.
func textFn(c *descCtx, v string) []Declaration {
	switch v {
	case "left", "center", "right", "justify", "start", "end":
		return ds("text-align", v)
	case "wrap", "nowrap", "balance", "pretty":
		return ds("text-wrap", v)
	case "ellipsis", "clip":
		return ds("text-overflow", v)
	}
	size, lh := v, ""
	if i := lastTopSlash(v); i > 0 {
		size, lh = v[:i], v[i+1:]
	}
	if t, ok := c.th.Text[size]; ok {
		lineHeight := sig4(float64(t.LineHeight))
		if lh != "" {
			if s, ok := c.spacing(lh); ok {
				lineHeight = s
			}
		}
		return ds("font-size", t.Size.String(), "line-height", lineHeight)
	}
	if a, ok := arbitrary(v); ok {
		if strings.HasPrefix(a, "length:") {
			return ds("font-size", strings.TrimPrefix(a, "length:"))
		}
		if _, isColor := c.color(v); !isColor {
			return ds("font-size", a)
		}
	}
	if s, ok := c.color(v); ok {
		return ds("color", s)
	}
	return nil
}

func fontFn(c *descCtx, v string) []Declaration {
	if w, ok := c.th.FontWeights[v]; ok {
		return ds("font-weight", num(w))
	}
	if f, ok := c.th.Fonts[v]; ok {
		return ds("font-family", strings.Join(f, ", "))
	}
	if a, ok := arbitrary(v); ok {
		return ds("font-weight", a)
	}
	return nil
}

func bgFn(c *descCtx, v string) []Declaration {
	switch v {
	case "auto", "cover", "contain":
		return ds("background-size", v)
	case "repeat", "no-repeat", "repeat-x", "repeat-y", "round", "space":
		return ds("background-repeat", v)
	case "repeat-round", "repeat-space":
		return ds("background-repeat", strings.TrimPrefix(v, "repeat-"))
	case "fixed", "local", "scroll":
		return ds("background-attachment", v)
	case "none":
		return ds("background-image", "none")
	case "clip-text":
		return ds("background-clip", "text")
	}
	if p, ok := positionOf(v); ok {
		return ds("background-position", p)
	}
	for _, g := range []struct{ prefix, dir string }{
		{"gradient-to-", ""}, {"linear-to-", ""},
	} {
		if d, ok := strings.CutPrefix(v, g.prefix); ok {
			if css, ok := gradientDir[d]; ok {
				return ds("background-image", "linear-gradient("+css+", var(--tw-gradient-stops))")
			}
		}
	}
	if n, ok := strings.CutPrefix(v, "linear-"); ok && isNumber(n) {
		return ds("background-image", "linear-gradient("+c.neg2(n)+"deg, var(--tw-gradient-stops))")
	}
	if v == "radial" {
		return ds("background-image", "radial-gradient(in oklab, var(--tw-gradient-stops))")
	}
	if v == "conic" {
		return ds("background-image", "conic-gradient(in oklab, var(--tw-gradient-stops))")
	}
	if a, ok := arbitrary(v); ok {
		if strings.HasPrefix(a, "url(") {
			return ds("background-image", a)
		}
		if _, isColor := c.color(v); !isColor {
			return ds("background", a)
		}
	}
	if s, ok := c.color(v); ok {
		return ds("background-color", s)
	}
	return nil
}

func (c *descCtx) neg2(n string) string {
	if c.neg {
		return "-" + n
	}
	return n
}

var gradientDir map[string]string

func descInitGradientDir() {
	gradientDir = map[string]string{
		"t": "to top", "tr": "to top right", "r": "to right", "br": "to bottom right",
		"b": "to bottom", "bl": "to bottom left", "l": "to left", "tl": "to top left",
	}
}

func gradientStop(property string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if strings.HasSuffix(v, "%") && isNumber(strings.TrimSuffix(v, "%")) {
			return ds(property+"-position", v)
		}
		if s, ok := c.color(v); ok {
			return ds(property, s)
		}
		return nil
	}
}

func ringFn(inset bool) descFn {
	return func(c *descCtx, v string) []Declaration {
		w := ""
		switch {
		case v == "":
			w = "1px"
		case isNumber(v):
			w = v + "px"
		}
		if w != "" {
			s := "0 0 0 " + w + " var(--tw-ring-color)"
			if inset {
				s = "inset " + s
			}
			return ds("box-shadow", s)
		}
		if s, ok := c.color(v); ok {
			return ds("--tw-ring-color", s)
		}
		return nil
	}
}

func outlineFn(c *descCtx, v string) []Declaration {
	switch {
	case v == "":
		return ds("outline-style", "var(--tw-outline-style)", "outline-width", "1px")
	case v == "none" || v == "hidden":
		return ds("outline-style", "none")
	case v == "solid" || v == "dashed" || v == "dotted" || v == "double":
		return ds("outline-style", v)
	case isNumber(v):
		return ds("outline-style", "var(--tw-outline-style)", "outline-width", v+"px")
	}
	if s, ok := c.color(v); ok {
		return ds("outline-color", s)
	}
	return nil
}

func transitionFn(c *descCtx, v string) []Declaration {
	var list string
	switch v {
	case "":
		list = "color, background-color, border-color, outline-color, text-decoration-color, fill, stroke, opacity, box-shadow, transform, translate, scale, rotate, filter, backdrop-filter"
	case "all":
		list = "all"
	case "colors":
		list = "color, background-color, border-color, outline-color, text-decoration-color, fill, stroke"
	case "opacity":
		list = "opacity"
	case "shadow":
		list = "box-shadow"
	case "transform":
		list = "transform, translate, scale, rotate"
	case "none":
		return ds("transition-property", "none")
	default:
		return nil
	}
	return ds("transition-property", list,
		"transition-timing-function", c.th.DefaultTransition.Timing.String(),
		"transition-duration", strconv.Itoa(int(c.th.DefaultTransition.Duration.Milliseconds()))+"ms")
}

func easeFn(c *descCtx, v string) []Declaration {
	switch v {
	case "linear":
		return ds("transition-timing-function", "linear")
	case "initial":
		return ds("transition-timing-function", "ease")
	}
	if b, ok := c.th.Ease[v]; ok {
		return ds("transition-timing-function", b.String())
	}
	if a, ok := arbitrary(v); ok {
		return ds("transition-timing-function", a)
	}
	return nil
}

func animateFn(c *descCtx, v string) []Declaration {
	if v == "none" {
		return ds("animation", "none")
	}
	if a, ok := c.th.Animate[v]; ok {
		return ds("animation", a)
	}
	if a, ok := arbitrary(v); ok {
		return ds("animation", a)
	}
	return nil
}

func flexFn(c *descCtx, v string) []Declaration {
	switch v {
	case "row", "row-reverse", "col", "col-reverse":
		return ds("flex-direction", strings.Replace(v, "col", "column", 1))
	case "wrap", "wrap-reverse", "nowrap":
		return ds("flex-wrap", v)
	case "auto":
		return ds("flex", "1 1 auto")
	case "initial":
		return ds("flex", "0 1 auto")
	case "none":
		return ds("flex", "none")
	}
	if n, ok := number(v); ok && v != "" {
		return ds("flex", numF(n)+" 1 0%")
	}
	if f, ok := fraction(v); ok {
		return ds("flex", f)
	}
	if a, ok := arbitrary(v); ok {
		return ds("flex", a)
	}
	return nil
}

func growFn(property string) descFn {
	return func(c *descCtx, v string) []Declaration {
		switch {
		case v == "":
			return ds(property, "1")
		case isNumber(v):
			return ds(property, v)
		}
		if a, ok := arbitrary(v); ok {
			return ds(property, a)
		}
		return nil
	}
}

func orderFn(c *descCtx, v string) []Declaration {
	switch v {
	case "first":
		return ds("order", "-9999")
	case "last":
		return ds("order", "9999")
	case "none":
		return ds("order", "0")
	}
	if n, ok := number(v); ok {
		return ds("order", numF(c.signed(n)))
	}
	return nil
}

func zFn(c *descCtx, v string) []Declaration {
	if v == "auto" {
		return ds("z-index", "auto")
	}
	if n, ok := number(v); ok {
		return ds("z-index", numF(c.signed(n)))
	}
	if a, ok := arbitrary(v); ok {
		return ds("z-index", a)
	}
	return nil
}

func leadingFn(c *descCtx, v string) []Declaration {
	if v == "none" {
		return ds("line-height", "1")
	}
	if l, ok := c.th.Leading[v]; ok {
		return ds("line-height", num(l))
	}
	if s, ok := c.spacing(v); ok {
		return ds("line-height", s)
	}
	return nil
}

func spaceBetween(axis string) descFn {
	prop := "margin-left"
	if axis == "y" {
		prop = "margin-top"
	}
	return func(c *descCtx, v string) []Declaration {
		if s, ok := c.spacing(v); ok {
			return ds(prop, s+" /* on every child but the first */")
		}
		return nil
	}
}

func aspectFn(c *descCtx, v string) []Declaration {
	switch v {
	case "auto":
		return ds("aspect-ratio", "auto")
	case "square":
		return ds("aspect-ratio", "1 / 1")
	}
	if a, ok := c.th.Aspect[v]; ok {
		return ds("aspect-ratio", sig4(float64(a)))
	}
	if a, ok := arbitrary(v); ok {
		return ds("aspect-ratio", a)
	}
	if f, _, ok := strings.Cut(v, "/"); ok && f != "" {
		return ds("aspect-ratio", strings.Replace(v, "/", " / ", 1))
	}
	return nil
}

func fillStroke(property, widthProperty string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if widthProperty != "" && isNumber(v) {
			return ds(widthProperty, v)
		}
		if s, ok := c.color(v); ok {
			return ds(property, s)
		}
		if v == "none" {
			return ds(property, "none")
		}
		return nil
	}
}

func dividerFn(axis string) descFn {
	prop := "border-inline-width"
	if axis == "y" {
		prop = "border-block-width"
	}
	return func(c *descCtx, v string) []Declaration {
		switch {
		case v == "":
			return ds(prop, "1px /* between the children */")
		case isNumber(v):
			return ds(prop, v+"px /* between the children */")
		}
		return nil
	}
}

func lineClampFn(c *descCtx, v string) []Declaration {
	if v == "none" {
		return ds("-webkit-line-clamp", "unset")
	}
	if n, ok := number(v); ok && v != "" {
		return ds("overflow", "hidden", "display", "-webkit-box", "-webkit-box-orient", "vertical", "-webkit-line-clamp", numF(n))
	}
	return nil
}

func originFn(c *descCtx, v string) []Declaration {
	if a, ok := arbitrary(v); ok {
		return ds("transform-origin", a)
	}
	if p, ok := positionOf(v); ok {
		return ds("transform-origin", p)
	}
	return nil
}

func blurFn(property string) descFn {
	return func(c *descCtx, v string) []Declaration {
		if v == "none" {
			return ds(property, "none")
		}
		if s, ok := lengthOf(c.th.Blur, v); ok {
			return ds(property, "blur("+s+")")
		}
		return nil
	}
}

var descRoots map[string]descFn

func descInitRoots() {
	// Spacing.
	descRoots = map[string]descFn{
		"p": spaceProps("padding"), "px": spaceProps("padding-inline"), "py": spaceProps("padding-block"),
		"pt": spaceProps("padding-top"), "pr": spaceProps("padding-right"), "pb": spaceProps("padding-bottom"),
		"pl": spaceProps("padding-left"), "ps": spaceProps("padding-inline-start"), "pe": spaceProps("padding-inline-end"),
		"m": spaceProps("margin"), "mx": spaceProps("margin-inline"), "my": spaceProps("margin-block"),
		"mt": spaceProps("margin-top"), "mr": spaceProps("margin-right"), "mb": spaceProps("margin-bottom"),
		"ml": spaceProps("margin-left"), "ms": spaceProps("margin-inline-start"), "me": spaceProps("margin-inline-end"),
		"pbs": spaceProps("padding-block-start"), "pbe": spaceProps("padding-block-end"),
		"mbs": spaceProps("margin-block-start"), "mbe": spaceProps("margin-block-end"),
		"gap": spaceProps("gap"), "gap-x": spaceProps("column-gap"), "gap-y": spaceProps("row-gap"),
		"space-x": spaceBetween("x"), "space-y": spaceBetween("y"),
		"inset": sizeProps(false, false, "inset"), "inset-x": sizeProps(false, false, "inset-inline"),
		"inset-y": sizeProps(true, false, "inset-block"),
		"top":     sizeProps(true, false, "top"), "bottom": sizeProps(true, false, "bottom"),
		"left": sizeProps(false, false, "left"), "right": sizeProps(false, false, "right"),
		"inset-s": sizeProps(false, false, "inset-inline-start"), "inset-e": sizeProps(false, false, "inset-inline-end"),
		"inset-bs": sizeProps(true, false, "inset-block-start"), "inset-be": sizeProps(true, false, "inset-block-end"),
		"start": sizeProps(false, false, "inset-inline-start"), "end": sizeProps(false, false, "inset-inline-end"),

		// Sizing.
		"w": sizeProps(false, true, "width"), "h": sizeProps(true, false, "height"),
		"size":  sizeProps(false, true, "width", "height"),
		"min-w": sizeProps(false, true, "min-width"), "max-w": sizeProps(false, true, "max-width"),
		"min-h": sizeProps(true, false, "min-height"), "max-h": sizeProps(true, false, "max-height"),
		"basis":  sizeProps(false, true, "flex-basis"),
		"aspect": aspectFn,

		// Flex and grid.
		"flex": flexFn, "grow": growFn("flex-grow"), "shrink": growFn("flex-shrink"), "order": orderFn,
		"justify": enumOf("justify-content", map[string]string{"start": "flex-start", "end": "flex-end", "center": "center",
			"between": "space-between", "around": "space-around", "evenly": "space-evenly", "stretch": "stretch", "normal": "normal", "baseline": "baseline"}),
		"items": enumOf("align-items", map[string]string{"start": "flex-start", "end": "flex-end", "center": "center",
			"baseline": "baseline", "stretch": "stretch", "normal": "normal"}),
		"content": enumOf("align-content", map[string]string{"start": "flex-start", "end": "flex-end", "center": "center",
			"between": "space-between", "around": "space-around", "evenly": "space-evenly", "stretch": "stretch", "normal": "normal", "baseline": "baseline"}),
		"self": enumOf("align-self", map[string]string{"auto": "auto", "start": "flex-start", "end": "flex-end",
			"center": "center", "baseline": "baseline", "stretch": "stretch"}),
		"justify-items": enumOf("justify-items", keywords("start", "end", "center", "stretch", "normal")),
		"justify-self":  enumOf("justify-self", keywords("auto", "start", "end", "center", "stretch")),
		"place-content": enumOf("place-content", map[string]string{"center": "center", "start": "start", "end": "end",
			"between": "space-between", "around": "space-around", "evenly": "space-evenly", "stretch": "stretch", "baseline": "baseline", "normal": "normal"}),
		"place-items": enumOf("place-items", keywords("start", "end", "center", "stretch", "baseline")),
		"place-self":  enumOf("place-self", keywords("auto", "start", "end", "center", "stretch")),
		"grid-cols":   gridTemplate("grid-template-columns"), "grid-rows": gridTemplate("grid-template-rows"),
		"grid-flow": enumOf("grid-auto-flow", map[string]string{"row": "row", "col": "column", "dense": "dense",
			"row-dense": "row dense", "col-dense": "column dense"}),
		"col-span": gridLine("grid-column", "span"), "row-span": gridLine("grid-row", "span"),
		"col-start": gridLine("grid-column-start", ""), "col-end": gridLine("grid-column-end", ""),
		"row-start": gridLine("grid-row-start", ""), "row-end": gridLine("grid-row-end", ""),
		"col": gridLine("grid-column", ""), "row": gridLine("grid-row", ""),

		// Layout.
		"overflow":   enumOf("overflow", keywords("auto", "hidden", "clip", "visible", "scroll")),
		"overflow-x": enumOf("overflow-x", keywords("auto", "hidden", "clip", "visible", "scroll")),
		"overflow-y": enumOf("overflow-y", keywords("auto", "hidden", "clip", "visible", "scroll")),
		"overscroll": enumOf("overscroll-behavior", keywords("auto", "contain", "none")),
		"z":          zFn,
		"object": func(c *descCtx, v string) []Declaration {
			if p, ok := positionOf(v); ok {
				return ds("object-position", p)
			}
			return enumOf("object-fit", keywords("contain", "cover", "fill", "none", "scale-down"))(c, v)
		},

		// Colors and backgrounds.
		"bg": bgFn, "from": gradientStop("--tw-gradient-from"), "via": gradientStop("--tw-gradient-via"),
		"to":   gradientStop("--tw-gradient-to"),
		"fill": fillStroke("fill", ""), "stroke": fillStroke("stroke", "stroke-width"),
		"caret": colorProp("caret-color"), "accent": colorProp("accent-color"),
		"decoration": func(c *descCtx, v string) []Declaration {
			switch v {
			case "solid", "double", "dotted", "dashed", "wavy":
				return ds("text-decoration-style", v)
			case "auto", "from-font":
				return ds("text-decoration-thickness", v)
			}
			if isNumber(v) {
				return ds("text-decoration-thickness", v+"px")
			}
			if s, ok := c.color(v); ok {
				return ds("text-decoration-color", s)
			}
			return nil
		},
		"divide": func(c *descCtx, v string) []Declaration {
			switch v {
			case "solid", "dashed", "dotted", "double", "none":
				return ds("border-style", v+" /* between the children */")
			}
			if s, ok := c.color(v); ok {
				return ds("border-color", s+" /* between the children */")
			}
			return nil
		},
		"divide-x": dividerFn("x"), "divide-y": dividerFn("y"),

		// Borders and effects.
		"border": borderFn(""), "border-x": borderFn("x"), "border-y": borderFn("y"),
		"border-t": borderFn("t"), "border-r": borderFn("r"), "border-b": borderFn("b"), "border-l": borderFn("l"),
		"border-s": borderFn("s"), "border-e": borderFn("e"),
		"border-bs": borderFn("bs"), "border-be": borderFn("be"),
		"ring-offset": func(c *descCtx, v string) []Declaration {
			if isNumber(v) {
				return ds("--tw-ring-offset-width", v+"px")
			}
			if s, ok := c.color(v); ok {
				return ds("--tw-ring-offset-color", s)
			}
			return nil
		},
		"translate": transformFn("translate", "X", "Y"),
		"outline":   outlineFn,
		"outline-offset": func(c *descCtx, v string) []Declaration {
			if isNumber(v) {
				return ds("outline-offset", c.negate(v+"px"))
			}
			return nil
		},
		"ring": ringFn(false), "inset-ring": ringFn(true),
		"shadow": shadowFn("shadow"), "inset-shadow": shadowFn("inset-shadow"),
		"drop-shadow": shadowFn("drop-shadow"), "text-shadow": shadowFn("text-shadow"),
		"opacity": func(c *descCtx, v string) []Declaration {
			if n, ok := number(v); ok {
				return ds("opacity", numF(n/100))
			}
			if a, ok := arbitrary(v); ok {
				return ds("opacity", a)
			}
			return nil
		},
		"mix-blend": enumOf("mix-blend-mode", blendModes()),
		"blur":      blurFn("filter"), "backdrop-blur": blurFn("backdrop-filter"),
		"brightness": filterFn("filter", "brightness", ""), "contrast": filterFn("filter", "contrast", ""),
		"saturate": filterFn("filter", "saturate", ""), "grayscale": filterFn("filter", "grayscale", "100%"),
		"invert": filterFn("filter", "invert", "100%"), "sepia": filterFn("filter", "sepia", "100%"),
		"hue-rotate":          func(c *descCtx, v string) []Declaration { return rotateFilter("filter", c, v) },
		"backdrop-brightness": filterFn("backdrop-filter", "brightness", ""),
		"backdrop-contrast":   filterFn("backdrop-filter", "contrast", ""),
		"backdrop-saturate":   filterFn("backdrop-filter", "saturate", ""),
		"backdrop-grayscale":  filterFn("backdrop-filter", "grayscale", "100%"),
		"backdrop-invert":     filterFn("backdrop-filter", "invert", "100%"),
		"backdrop-sepia":      filterFn("backdrop-filter", "sepia", "100%"),

		// Rounded.
		"rounded": roundedFn(""), "rounded-t": roundedFn("t"), "rounded-r": roundedFn("r"), "rounded-b": roundedFn("b"),
		"rounded-l": roundedFn("l"), "rounded-s": roundedFn("s"), "rounded-e": roundedFn("e"),
		"rounded-tl": roundedFn("tl"), "rounded-tr": roundedFn("tr"), "rounded-br": roundedFn("br"), "rounded-bl": roundedFn("bl"),
		"rounded-ss": roundedFn("ss"), "rounded-se": roundedFn("se"), "rounded-ee": roundedFn("ee"), "rounded-es": roundedFn("es"),

		// Typography.
		"text": textFn, "font": fontFn,
		"tracking": func(c *descCtx, v string) []Declaration {
			if l, ok := c.th.Tracking[v]; ok {
				return ds("letter-spacing", l.String())
			}
			if a, ok := arbitrary(v); ok {
				return ds("letter-spacing", a)
			}
			return nil
		},
		"leading":    leadingFn,
		"whitespace": enumOf("white-space", keywords("normal", "nowrap", "pre", "pre-line", "pre-wrap", "break-spaces")),
		"underline-offset": func(c *descCtx, v string) []Declaration {
			if v == "auto" {
				return ds("text-underline-offset", "auto")
			}
			if isNumber(v) {
				return ds("text-underline-offset", v+"px")
			}
			return nil
		},
		"align":      enumOf("vertical-align", keywords("baseline", "top", "middle", "bottom", "text-top", "text-bottom", "sub", "super")),
		"line-clamp": lineClampFn,
		"hyphens":    enumOf("hyphens", keywords("none", "manual", "auto")),

		// Transforms and motion.
		"translate-x": transformFn("translate", "X"), "translate-y": transformFn("translate", "Y"),
		"scale": transformFn("scale", ""), "scale-x": transformFn("scale", "X"), "scale-y": transformFn("scale", "Y"),
		"rotate": transformFn("rotate", ""), "skew": transformFn("skew", "X", "Y"),
		"skew-x": transformFn("skew", "X"), "skew-y": transformFn("skew", "Y"),
		"origin":     originFn,
		"transition": transitionFn, "duration": durationFn("transition-duration"),
		"delay": durationFn("transition-delay"), "ease": easeFn, "animate": animateFn,

		// Interaction.
		"cursor": func(c *descCtx, v string) []Declaration {
			if v == "" {
				return nil
			}
			if a, ok := arbitrary(v); ok {
				return ds("cursor", a)
			}
			return ds("cursor", v)
		},
		"pointer-events": enumOf("pointer-events", keywords("none", "auto")),
		"select":         enumOf("user-select", keywords("none", "text", "all", "auto")),
		"resize":         enumOf("resize", map[string]string{"none": "none", "x": "horizontal", "y": "vertical"}),
	}
}

func rotateFilter(property string, c *descCtx, v string) []Declaration {
	if n, ok := number(v); ok {
		return ds(property, "hue-rotate("+numF(c.signed(n))+"deg)")
	}
	return nil
}

// ---------------------------------------------------------------------
// Variants.

var variantWords map[string]string

func descInitVariantWords() {
	variantWords = map[string]string{
		"hover": "on hover", "active": "while pressed", "focus": "when focused",
		"focus-visible": "when focused with the keyboard", "focus-within": "when focus is inside",
		"disabled": "when disabled", "enabled": "when enabled", "checked": "when checked",
		"invalid": "when invalid", "read-only": "when read-only", "placeholder-shown": "while the placeholder shows",
		"placeholder": "for the placeholder text", "selection": "for the selected text",
		"first": "on the first child", "last": "on the last child", "only": "on an only child",
		"odd": "on odd children", "even": "on even children",
		"dark": "in dark mode", "motion-reduce": "when the user prefers reduced motion",
		"motion-safe": "unless the user prefers reduced motion", "contrast-more": "in high contrast",
		"rtl": "in right-to-left text", "ltr": "in left-to-right text",
		"starting": "on the first frame (@starting-style)",
	}
}

// describeVariant says in words when a variant applies.
func describeVariant(th *Theme, v string) string {
	if w, ok := variantWords[v]; ok {
		return w
	}
	if bp, ok := th.Breakpoints[v]; ok {
		return "when the window is at least " + bp.String() + " wide (" + v + ")"
	}
	for _, prefix := range []string{"max-", "min-"} {
		if rest, ok := strings.CutPrefix(v, prefix); ok {
			var width string
			if bp, ok := th.Breakpoints[rest]; ok {
				width = bp.String() + " (" + rest + ")"
			} else if a, ok := arbitrary(rest); ok {
				width = a
			} else {
				break
			}
			if prefix == "max-" {
				return "when the window is narrower than " + width
			}
			return "when the window is at least " + width + " wide"
		}
	}
	if rest, ok := strings.CutPrefix(v, "group-"); ok {
		return "when a group ancestor is: " + describeVariant(th, rest)
	}
	if rest, ok := strings.CutPrefix(v, "peer-"); ok {
		return "when a preceding peer is: " + describeVariant(th, rest)
	}
	if rest, ok := strings.CutPrefix(v, "aria-"); ok {
		if a, ok := arbitrary(rest); ok {
			return "when aria-" + a
		}
		return "when aria-" + rest + " is true"
	}
	return "variant " + v
}

// VariantNames returns the variants that are plain names ("hover", "md",
// "group-focus"), sorted: what a class can be prefixed with, before the
// colon. The parametrized ones (min-[600px], aria-[sort=...]) are not
// listed.
func (p *Parser) VariantNames() []string { descLoad(); return variantNames(p.th) }

// DescribeVariant says in words when the variant v applies.
func (p *Parser) DescribeVariant(v string) string { descLoad(); return describeVariant(p.th, v) }
