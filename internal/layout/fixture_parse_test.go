package layout

import (
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// This file parses the Taffy fixtures (small HTML pages of nested divs
// with inline styles) into trees of test nodes. It understands exactly what
// the fixtures use: div elements, the style, id and class attributes, text
// with zero-width spaces, comments, and the inline CSS properties below,
// with test_base_style.css's defaults (div is display: flex and position:
// relative, the body's children are position: absolute, scrollbars are
// 15px wide).

// htmlElem is a parsed div.
type htmlElem struct {
	attrs    map[string]string
	children []*htmlElem
	text     strings.Builder
}

// parseHTML parses the body of a fixture into its top-level elements.
func parseHTML(src string) ([]*htmlElem, error) {
	root := &htmlElem{}
	stack := []*htmlElem{root}
	for i := 0; i < len(src); {
		switch {
		case strings.HasPrefix(src[i:], "<!--"):
			j := strings.Index(src[i:], "-->")
			if j < 0 {
				return nil, fmt.Errorf("unterminated comment")
			}
			i += j + 3
		case strings.HasPrefix(src[i:], "</"):
			j := strings.IndexByte(src[i:], '>')
			if j < 0 || len(stack) < 2 {
				return nil, fmt.Errorf("unbalanced end tag at %d", i)
			}
			if tag := strings.TrimSpace(src[i+2 : i+j]); tag != "div" {
				return nil, fmt.Errorf("unexpected end tag %q", tag)
			}
			stack = stack[:len(stack)-1]
			i += j + 1
		case src[i] == '<':
			el, n, err := parseStartTag(src[i:])
			if err != nil {
				return nil, err
			}
			top := stack[len(stack)-1]
			top.children = append(top.children, el)
			stack = append(stack, el)
			i += n
		default:
			j := strings.IndexByte(src[i:], '<')
			if j < 0 {
				j = len(src) - i
			}
			stack[len(stack)-1].text.WriteString(src[i : i+j])
			i += j
		}
	}
	if len(stack) != 1 {
		return nil, fmt.Errorf("unclosed elements")
	}
	return root.children, nil
}

// parseStartTag parses "<div attr="value" ...>".
func parseStartTag(s string) (*htmlElem, int, error) {
	if !strings.HasPrefix(s, "<div") {
		return nil, 0, fmt.Errorf("unexpected tag at %q", s[:min(len(s), 20)])
	}
	el := &htmlElem{attrs: map[string]string{}}
	i := 4
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\t' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			return nil, 0, fmt.Errorf("unterminated tag")
		}
		if s[i] == '>' {
			return el, i + 1, nil
		}
		j := i
		for j < len(s) && s[j] != '=' && s[j] != ' ' && s[j] != '>' && s[j] != '\n' {
			j++
		}
		name := s[i:j]
		i = j
		if i < len(s) && s[i] == '=' {
			q := s[i+1]
			if q != '"' && q != '\'' {
				return nil, 0, fmt.Errorf("unquoted attribute %q", name)
			}
			k := strings.IndexByte(s[i+2:], q)
			if k < 0 {
				return nil, 0, fmt.Errorf("unterminated attribute %q", name)
			}
			el.attrs[name] = html.UnescapeString(s[i+2 : i+2+k])
			i += 2 + k + 1
		} else {
			el.attrs[name] = ""
		}
	}
}

// textContent returns the trimmed text of an element without element
// children (Taffy's gentest records text only for those).
func (el *htmlElem) textContent() string {
	if len(el.children) > 0 {
		return ""
	}
	return strings.TrimSpace(html.UnescapeString(el.text.String()))
}

// cssDecls splits an inline style into declarations, dropping comments.
func cssDecls(s string) [][2]string {
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			break
		}
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			s = s[:i]
			break
		}
		s = s[:i] + s[i+2+j+2:]
	}
	var out [][2]string
	for _, d := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(d, ":")
		if !ok {
			continue
		}
		out = append(out, [2]string{strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)})
	}
	return out
}

// parseDim parses a length, percentage or sizing keyword.
func parseDim(v string) (Dim, bool) {
	v = strings.TrimSpace(v)
	switch v {
	case "auto", "none":
		return Auto, true
	case "min-content":
		return MinContent, true
	case "max-content":
		return MaxContent, true
	case "fit-content":
		return FitContent, true
	case "stretch", "-webkit-fill-available":
		return Stretch, true
	case "content":
		return Content, true
	case "0":
		return Px(0), true
	}
	if inner, ok := strings.CutPrefix(v, "calc("); ok && strings.HasSuffix(inner, ")") {
		// calc(a + b) or calc(a - b) of a length and a percentage.
		inner = inner[:len(inner)-1]
		sign, i := float32(1), strings.Index(inner, " + ")
		if i < 0 {
			sign, i = -1, strings.Index(inner, " - ")
		}
		if i < 0 {
			return Dim{}, false
		}
		a, okA := parseDim(inner[:i])
		b, okB := parseDim(inner[i+3:])
		if !okA || !okB {
			return Dim{}, false
		}
		var px, f float32
		for k, d := range []Dim{a, b} {
			m := float32(1)
			if k == 1 {
				m = sign
			}
			switch d.Unit {
			case UnitPx:
				px += m * d.Value
			case UnitPercent:
				f += m * d.Value
			default:
				return Dim{}, false
			}
		}
		return Calc(px, f), true
	}
	if strings.HasPrefix(v, "fit-content(") && strings.HasSuffix(v, ")") {
		d, ok := parseDim(v[len("fit-content(") : len(v)-1])
		switch {
		case ok && d.Unit == UnitPx:
			return FitContentPx(d.Value), true
		case ok && d.Unit == UnitPercent:
			return FitContentPercent(d.Value), true
		}
		return Dim{}, false
	}
	if num, ok := strings.CutSuffix(v, "px"); ok {
		f, err := strconv.ParseFloat(num, 32)
		return Px(float32(f)), err == nil
	}
	if num, ok := strings.CutSuffix(v, "%"); ok {
		f, err := strconv.ParseFloat(num, 64)
		return Percent(float32(f / 100)), err == nil
	}
	return Dim{}, false
}

// parseEdges parses a 1-4 value shorthand (top right bottom left).
func parseEdges(v string, parse func(string) (Dim, bool)) (Edges, bool) {
	parts := strings.Fields(v)
	ds := make([]Dim, len(parts))
	for i, p := range parts {
		d, ok := parse(p)
		if !ok {
			return Edges{}, false
		}
		ds[i] = d
	}
	switch len(ds) {
	case 1:
		return Edges{ds[0], ds[0], ds[0], ds[0]}, true
	case 2:
		return Edges{ds[0], ds[1], ds[0], ds[1]}, true
	case 3:
		return Edges{ds[0], ds[1], ds[2], ds[1]}, true
	case 4:
		return Edges{ds[0], ds[1], ds[2], ds[3]}, true
	}
	return Edges{}, false
}

// parseBorderWidth parses a border width (keywords as in Chromium).
// Chromium does not accept percentages: the declaration is dropped.
func parseBorderWidth(v string) (Dim, bool) {
	switch v {
	case "thin":
		return Px(1), true
	case "medium":
		return Px(3), true
	case "thick":
		return Px(5), true
	}
	d, ok := parseDim(v)
	return d, ok && d.Unit == UnitPx
}

// borderShorthandWidth extracts the width of a border shorthand ("5px
// solid black"); a missing width is medium (3px) with a style, else 0.
func borderShorthandWidth(v string) (Dim, bool) {
	width := Px(3)
	hasStyle := false
	for _, p := range strings.Fields(v) {
		if d, ok := parseBorderWidth(p); ok && d.Unit == UnitPx {
			width = d
			continue
		}
		switch p {
		case "solid", "dashed", "dotted", "double", "groove", "ridge", "inset", "outset":
			hasStyle = true
		case "none", "hidden":
			return Px(0), true
		}
	}
	if !hasStyle {
		return Px(0), true
	}
	return width, true
}

func parseAlign(v string) (Align, bool) {
	var safe Align
	fs := strings.Fields(v)
	if len(fs) == 2 {
		switch fs[0] {
		case "safe":
			safe = AlignSafe
		case "unsafe":
		default:
			return 0, false
		}
		v = fs[1]
	}
	a, ok := map[string]Align{
		"normal": AlignNormal, "auto": AlignNormal, "start": AlignStart, "end": AlignEnd,
		"flex-start": AlignFlexStart, "flex-end": AlignFlexEnd, "self-start": AlignSelfStart,
		"self-end": AlignSelfEnd, "center": AlignCenter, "baseline": AlignBaseline,
		"stretch": AlignStretch, "space-between": AlignSpaceBetween, "space-around": AlignSpaceAround,
		"space-evenly": AlignSpaceEvenly, "left": AlignStart, "right": AlignEnd,
	}[v]
	return a | safe, ok
}

func parseOverflow(v string) (Overflow, bool) {
	o, ok := map[string]Overflow{
		"visible": OverflowVisible, "clip": OverflowClip, "hidden": OverflowHidden,
		"scroll": OverflowScroll, "auto": OverflowAuto,
	}[v]
	return o, ok
}

func parseNumber(v string) (float32, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 32)
	return float32(f), err == nil
}

// fixtureStyle is a parsed element style plus what the parser could not
// represent.
type fixtureStyle struct {
	Style
	// unsupported lists features outside Tela's layout (exclusion
	// reasons); unparsed lists declarations the parser does not know.
	unsupported []string
	unparsed    []string
	// marginInlineStart and marginInlineEnd are resolved against the
	// direction once all declarations are read.
	marginInline [2]*Dim
}

// elementStyle computes the style of a fixture element: the base
// stylesheet's defaults, then the inline style.
func elementStyle(el *htmlElem, topLevel bool) *fixtureStyle {
	fs := &fixtureStyle{Style: DefaultStyle()}
	st := &fs.Style
	st.Display = DisplayFlex
	st.Position = PositionRelative
	st.ScrollbarWidth = 15
	if strings.Contains(" "+el.attrs["class"]+" ", " viewport ") {
		st.AlignItems = AlignStart
		st.JustifyContent = AlignStart
		st.Width, st.Height = MaxContent, MaxContent
	}
	if topLevel {
		st.Position = PositionAbsolute
	}
	for _, d := range cssDecls(el.attrs["style"]) {
		fs.apply(d[0], d[1])
	}
	// Computed value: if one axis is neither visible nor clip, visible in
	// the other axis becomes auto (CSS Overflow 3).
	scrolls := func(o Overflow) bool { return o != OverflowVisible && o != OverflowClip }
	if scrolls(st.OverflowX) && st.OverflowY == OverflowVisible {
		st.OverflowY = OverflowAuto
	}
	if scrolls(st.OverflowY) && st.OverflowX == OverflowVisible {
		st.OverflowX = OverflowAuto
	}
	for i, m := range fs.marginInline {
		if m == nil {
			continue
		}
		if (i == 0) == (st.Direction == LTR) {
			st.Margin.Left = *m
		} else {
			st.Margin.Right = *m
		}
	}
	return fs
}

// apply applies one declaration.
func (fs *fixtureStyle) apply(prop, v string) {
	st := &fs.Style
	ok := true
	dim := func(dst *Dim) {
		var d Dim
		d, ok = parseDim(v)
		if ok {
			*dst = d
		}
	}
	unsupported := func(reason string) { fs.unsupported = append(fs.unsupported, reason) }
	switch prop {
	case "display":
		switch v {
		case "block", "flow-root":
			st.Display = DisplayBlock
		case "flex":
			st.Display = DisplayFlex
		case "none":
			st.Display = DisplayNone
		case "grid":
			unsupported("grid")
		default:
			ok = false
		}
	case "position":
		switch v {
		case "relative":
			st.Position = PositionRelative
		case "absolute":
			st.Position = PositionAbsolute
		case "static":
			st.Position = PositionStatic
		default:
			ok = false
		}
	case "direction":
		switch v {
		case "ltr":
			st.Direction = LTR
		case "rtl":
			st.Direction = RTL
		default:
			ok = false
		}
	case "width":
		dim(&st.Width)
	case "height":
		dim(&st.Height)
	case "min-width":
		dim(&st.MinWidth)
	case "min-height":
		dim(&st.MinHeight)
	case "max-width":
		dim(&st.MaxWidth)
	case "max-height":
		dim(&st.MaxHeight)
	case "aspect-ratio":
		if a, b, found := strings.Cut(v, "/"); found {
			x, ok1 := parseNumber(a)
			y, ok2 := parseNumber(b)
			ok = ok1 && ok2 && y > 0
			if ok {
				st.AspectRatio = x / y
			}
		} else {
			st.AspectRatio, ok = parseNumber(v)
		}
	case "top":
		dim(&st.Inset.Top)
	case "right":
		dim(&st.Inset.Right)
	case "bottom":
		dim(&st.Inset.Bottom)
	case "left":
		dim(&st.Inset.Left)
	case "inset":
		st.Inset, ok = parseEdges(v, parseDim)
	case "margin":
		st.Margin, ok = parseEdges(v, parseDim)
	case "margin-top":
		dim(&st.Margin.Top)
	case "margin-right":
		dim(&st.Margin.Right)
	case "margin-bottom":
		dim(&st.Margin.Bottom)
	case "margin-left":
		dim(&st.Margin.Left)
	case "margin-inline-start", "margin-inline-end":
		d, okd := parseDim(v)
		ok = okd
		i := 0
		if prop == "margin-inline-end" {
			i = 1
		}
		fs.marginInline[i] = &d
	case "padding":
		st.Padding, ok = parseEdges(v, parseDim)
	case "padding-top":
		dim(&st.Padding.Top)
	case "padding-right":
		dim(&st.Padding.Right)
	case "padding-bottom":
		dim(&st.Padding.Bottom)
	case "padding-left":
		dim(&st.Padding.Left)
	case "border-width":
		if strings.Contains(v, "%") {
			break // invalid in Chromium: dropped
		}
		st.Border, ok = parseEdges(v, parseBorderWidth)
	case "border-top-width":
		st.Border.Top, ok = parseBorderWidth(v)
	case "border-right-width":
		st.Border.Right, ok = parseBorderWidth(v)
	case "border-bottom-width":
		st.Border.Bottom, ok = parseBorderWidth(v)
	case "border-left-width":
		st.Border.Left, ok = parseBorderWidth(v)
	case "border":
		var d Dim
		d, ok = borderShorthandWidth(v)
		st.Border = EdgesAll(d)
	case "border-top":
		st.Border.Top, ok = borderShorthandWidth(v)
	case "border-right":
		st.Border.Right, ok = borderShorthandWidth(v)
	case "border-bottom":
		st.Border.Bottom, ok = borderShorthandWidth(v)
	case "border-left":
		st.Border.Left, ok = borderShorthandWidth(v)
	case "flex-direction":
		var d FlexDirection
		d, ok = map[string]FlexDirection{"row": Row, "column": Column, "row-reverse": RowReverse, "column-reverse": ColumnReverse}[v]
		if ok {
			st.FlexDirection = d
		}
	case "flex-wrap":
		switch v {
		case "nowrap", "no-wrap":
			// "no-wrap" is invalid CSS: Chromium ignores it.
			if v == "nowrap" {
				st.FlexWrap = NoWrap
			}
		case "wrap":
			st.FlexWrap = Wrap
		case "wrap-reverse":
			st.FlexWrap = WrapReverse
		default:
			if strings.Contains(v, "balance") {
				unsupported("flex-wrap: balance")
			} else {
				ok = false
			}
		}
	case "flex-flow":
		for _, p := range strings.Fields(v) {
			if d, isDir := map[string]FlexDirection{"row": Row, "column": Column, "row-reverse": RowReverse, "column-reverse": ColumnReverse}[p]; isDir {
				st.FlexDirection = d
			} else {
				fs.apply("flex-wrap", p)
			}
		}
	case "flex-line-count":
		unsupported("flex-wrap: balance")
	case "flex":
		ok = fs.parseFlex(v)
	case "order":
		var o float32
		o, ok = parseNumber(v)
		st.Order = int32(o)
	case "flex-grow":
		st.FlexGrow, ok = parseNumber(v)
	case "flex-shrink":
		st.FlexShrink, ok = parseNumber(v)
	case "flex-basis":
		dim(&st.FlexBasis)
	case "align-items":
		st.AlignItems, ok = parseAlign(v)
	case "align-self":
		st.AlignSelf, ok = parseAlign(v)
	case "align-content":
		st.AlignContent, ok = parseAlign(v)
	case "justify-content":
		st.JustifyContent, ok = parseAlign(v)
	case "gap":
		parts := strings.Fields(v)
		var r, c Dim
		r, ok = parseDim(parts[0])
		c = r
		if len(parts) == 2 {
			var ok2 bool
			c, ok2 = parseDim(parts[1])
			ok = ok && ok2
		}
		st.RowGap, st.ColumnGap = r, c
	case "row-gap":
		dim(&st.RowGap)
	case "column-gap":
		dim(&st.ColumnGap)
	case "overflow":
		parts := strings.Fields(v)
		var x, y Overflow
		x, ok = parseOverflow(parts[0])
		y = x
		if len(parts) == 2 {
			var ok2 bool
			y, ok2 = parseOverflow(parts[1])
			ok = ok && ok2
		}
		st.OverflowX, st.OverflowY = x, y
	case "overflow-x":
		st.OverflowX, ok = parseOverflow(v)
	case "overflow-y":
		st.OverflowY, ok = parseOverflow(v)
	case "box-sizing":
		switch v {
		case "border-box":
		case "content-box":
			unsupported("box-sizing: content-box")
		default:
			ok = false
		}
	case "writing-mode":
		if v != "horizontal-tb" {
			unsupported("vertical writing modes")
		}
	case "text-align":
		unsupported("legacy -webkit text-align block alignment")
	case "float":
		if v != "none" {
			unsupported("float")
		}
	case "grid-template-rows", "grid-template-columns", "grid-column-start", "grid-column-end", "grid-row-start", "grid-row-end":
		unsupported("grid")
	case "background", "background-color", "color", "border-color", "border-style",
		// Invalid property names, which Chromium drops.
		"alignitems", "border-width-top", "border-width-bottom", "border-start-width", "border-end-width":
	default:
		ok = false
	}
	if !ok {
		fs.unparsed = append(fs.unparsed, prop+": "+v)
	}
}

// parseFlex parses the flex shorthand.
func (fs *fixtureStyle) parseFlex(v string) bool {
	st := &fs.Style
	switch v {
	case "none":
		st.FlexGrow, st.FlexShrink, st.FlexBasis = 0, 0, Auto
		return true
	case "auto":
		st.FlexGrow, st.FlexShrink, st.FlexBasis = 1, 1, Auto
		return true
	}
	parts := strings.Fields(v)
	grow, shrink, basis := float32(1), float32(1), Px(0)
	var nums []float32
	for _, p := range parts {
		if f, ok := parseNumber(p); ok && p != "0px" {
			nums = append(nums, f)
			continue
		}
		d, ok := parseDim(p)
		if !ok {
			return false
		}
		basis = d
	}
	// A single number with no basis sets the basis to 0%.
	if len(parts) == 1 && len(nums) == 1 {
		basis = Percent(0)
	}
	if len(nums) >= 1 {
		grow = nums[0]
	}
	if len(nums) >= 2 {
		shrink = nums[1]
	}
	if len(nums) == 3 {
		basis = Px(nums[2]) // "1 1 0"
	}
	st.FlexGrow, st.FlexShrink, st.FlexBasis = grow, shrink, basis
	return true
}

// testNode is a Node for tests.
type testNode struct {
	name     string
	style    Style
	children []*testNode
	data     Data
	// measure, if set, measures the content of a leaf.
	measure  func(MeasureInput) Measurement
	measures int
}

func (n *testNode) LayoutStyle() *Style    { return &n.style }
func (n *testNode) LayoutChildCount() int  { return len(n.children) }
func (n *testNode) LayoutChild(i int) Node { return n.children[i] }
func (n *testNode) LayoutData() *Data      { return &n.data }

func (n *testNode) Measure(in MeasureInput) Measurement {
	n.measures++
	if n.measure == nil {
		return Measurement{}
	}
	return n.measure(in)
}

// ahemMeasure measures text in the Ahem font at 10px with line-height 1:
// every glyph is a 10x10 square, the baseline is at 8px, and zero-width
// spaces are the only break opportunities (Taffy's AhemTextMeasureData).
func ahemMeasure(text string) func(MeasureInput) Measurement {
	const glyph = 10
	segs := strings.Split(text, "\u200b")
	var minLen, maxLen int
	lens := make([]int, len(segs))
	for i, s := range segs {
		lens[i] = utf8.RuneCountInString(s)
		minLen = max(minLen, lens[i])
		maxLen += lens[i]
	}
	return func(in MeasureInput) Measurement {
		var w float32
		switch {
		case in.HasWidth:
			w = in.Width
		case in.AvailableWidth.Kind == MinContentSpace:
			w = float32(minLen * glyph)
		case in.AvailableWidth.Kind == MaxContentSpace:
			w = float32(maxLen * glyph)
		default:
			w = min(in.AvailableWidth.Value, float32(maxLen*glyph))
		}
		w = max(w, float32(minLen*glyph))
		h := in.Height
		if !in.HasHeight {
			perLine := int(math.Floor(float64(w / glyph)))
			lines, cur := 1, 0
			for _, l := range lens {
				if cur+l > perLine {
					if cur > 0 {
						lines++
					}
					cur = l
				} else {
					cur += l
				}
			}
			h = float32(lines * glyph)
		}
		return Measurement{Width: w, Height: h, Baseline: 8, HasBaseline: true}
	}
}

// fixtureTree is a fixture parsed into test nodes, inside a viewport node
// (the initial containing block) and a body node.
type fixtureTree struct {
	viewport    *testNode
	root        *testNode // #test-root
	unsupported []string
	unparsed    []string
}

// buildFixture parses a fixture into a tree: viewport (block, the window
// size, the containing block of the absolutely positioned body children) >
// body (block, static) > the fixture's elements.
func buildFixture(src string, vw, vh float32) (*fixtureTree, error) {
	body, bodyStyle, err := fixtureBody(src)
	if err != nil {
		return nil, err
	}
	elems, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	ft := &fixtureTree{}
	vp := &testNode{name: "viewport", style: DefaultStyle()}
	vp.style.Position = PositionRelative
	vp.style.Width, vp.style.Height = Px(vw), Px(vh)
	bd := &testNode{name: "body", style: DefaultStyle()}
	for _, d := range cssDecls(bodyStyle) {
		bs := &fixtureStyle{Style: bd.style}
		bs.apply(d[0], d[1])
		bd.style = bs.Style
	}
	vp.children = []*testNode{bd}
	var build func(el *htmlElem, top bool) *testNode
	build = func(el *htmlElem, top bool) *testNode {
		fs := elementStyle(el, top)
		ft.unsupported = append(ft.unsupported, fs.unsupported...)
		ft.unparsed = append(ft.unparsed, fs.unparsed...)
		n := &testNode{name: el.attrs["id"], style: fs.Style}
		if t := el.textContent(); t != "" {
			n.measure = ahemMeasure(t)
		}
		for _, c := range el.children {
			n.children = append(n.children, build(c, false))
		}
		if el.attrs["id"] == "test-root" {
			ft.root = n
		}
		return n
	}
	for _, el := range elems {
		bd.children = append(bd.children, build(el, true))
	}
	if ft.root == nil {
		return nil, fmt.Errorf("no #test-root")
	}
	ft.viewport = vp
	return ft, nil
}
