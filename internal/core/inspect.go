package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// InspectNode describes one box of the laid-out tree, for `tela inspect
// --layout` (ARCHITECTURE section 17), tests and debugging. Values are
// plain data; the JSON tags give the serialized form.
type InspectNode struct {
	// Kind is the primitive ("root", "div", "h1", "button"...) or
	// "anonymous" for an anonymous paragraph box.
	Kind string `json:"kind"`
	// Key is the element's key formatted with %v, if it has one.
	Key string `json:"key,omitempty"`
	// Component is the function name of the component that produced the
	// element, if any ("main.Counter").
	Component string `json:"component,omitempty"`
	// Rect is the border box in absolute logical pixels (unrounded), as
	// layout placed it: scroll offsets and transforms are not applied.
	Rect gfx.Rect `json:"rect"`
	// Bounds is the box as painted in window coordinates: the bounding
	// box of the border box moved by the scroll offsets of the
	// containers around it and by the transforms of the box and its
	// ancestors (ElementRef.Bounds). Nil when it is Rect.
	Bounds *gfx.Rect `json:"bounds,omitempty"`
	// Margin, Border and Padding are the resolved edges.
	Margin  Edges `json:"margin"`
	Border  Edges `json:"border"`
	Padding Edges `json:"padding"`
	// Text is the text of a paragraph box, with the number of lines it
	// is broken into.
	Text  string `json:"text,omitempty"`
	Lines int    `json:"lines,omitempty"`
	// Inline lists the inline elements of a paragraph box ("strong",
	// "span"...), in order.
	Inline []string `json:"inline,omitempty"`
	// Fragments are the fragments of those inline elements, by line: one
	// per line an element is on (and per bidi run that separates its
	// content), with the index of the element in Inline and its border
	// box in absolute logical pixels (padding and borders included).
	Fragments []InspectFragment `json:"fragments,omitempty"`
	// InlineBox reports an atomic inline (an inline-block, inline-flex or
	// inline image box, a Button in text): a child of a paragraph box,
	// placed on one of its lines.
	InlineBox bool `json:"inlineBox,omitempty"`
	// Style summarizes the computed style.
	Style    StyleSummary  `json:"style"`
	Children []InspectNode `json:"children,omitempty"`
}

// jsonRect is the serialized form of a rectangle: the origin and the size
// in logical pixels, which is easier to read than two corners.
type jsonRect struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
}

// MarshalJSON writes the node with the same fields as the struct, in the
// same order, except that rect and bounds are {x, y, width, height}
// instead of the two corners of a gfx.Rect, margin, border and padding are
// left out when all four edges are zero, and text is not HTML-escaped ("&"
// stays "&").
func (n InspectNode) MarshalJSON() ([]byte, error) {
	var bounds *jsonRect
	if n.Bounds != nil {
		bounds = &jsonRect{n.Bounds.X0, n.Bounds.Y0, n.Bounds.Width(), n.Bounds.Height()}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	err := enc.Encode(struct {
		Kind      string        `json:"kind"`
		Key       string        `json:"key,omitempty"`
		Component string        `json:"component,omitempty"`
		Rect      jsonRect      `json:"rect"`
		Bounds    *jsonRect     `json:"bounds,omitempty"`
		Margin    Edges         `json:"margin,omitzero"`
		Border    Edges         `json:"border,omitzero"`
		Padding   Edges         `json:"padding,omitzero"`
		Text      string        `json:"text,omitempty"`
		Lines     int           `json:"lines,omitempty"`
		Inline    []string      `json:"inline,omitempty"`
		Fragments []jsonFrag    `json:"fragments,omitempty"`
		InlineBox bool          `json:"inlineBox,omitempty"`
		Style     StyleSummary  `json:"style"`
		Children  []InspectNode `json:"children,omitempty"`
	}{
		n.Kind, n.Key, n.Component,
		jsonRect{n.Rect.X0, n.Rect.Y0, n.Rect.Width(), n.Rect.Height()}, bounds,
		n.Margin, n.Border, n.Padding, n.Text, n.Lines, n.Inline, jsonFrags(n.Fragments), n.InlineBox, n.Style, n.Children,
	})
	return bytes.TrimRight(buf.Bytes(), "\n"), err
}

// InspectFragment is a fragment of an inline element of a paragraph.
type InspectFragment struct {
	// Inline is the element's index in InspectNode.Inline.
	Inline int `json:"inline"`
	// Line is the line of the paragraph it is on.
	Line int `json:"line"`
	// Rect is its border box in absolute logical pixels.
	Rect gfx.Rect `json:"rect"`
}

// jsonFrag is the serialized form of a fragment: its rect as {x, y,
// width, height}.
type jsonFrag struct {
	Inline int      `json:"inline"`
	Line   int      `json:"line"`
	Rect   jsonRect `json:"rect"`
}

func jsonFrags(fs []InspectFragment) []jsonFrag {
	var out []jsonFrag
	for _, f := range fs {
		out = append(out, jsonFrag{f.Inline, f.Line, jsonRect{f.Rect.X0, f.Rect.Y0, f.Rect.Width(), f.Rect.Height()}})
	}
	return out
}

// Edges are four edge lengths in logical pixels.
type Edges struct {
	Top    float32 `json:"top"`
	Right  float32 `json:"right"`
	Bottom float32 `json:"bottom"`
	Left   float32 `json:"left"`
}

// StyleSummary is a compact, CSS-like view of a computed style: enough to
// tell why a box got its geometry and color.
type StyleSummary struct {
	// Classes are the element's class strings, joined by spaces.
	Classes  string `json:"classes,omitempty"`
	Display  string `json:"display"`
	Position string `json:"position,omitempty"`
	Width    string `json:"width,omitempty"`
	Height   string `json:"height,omitempty"`
	MinWidth string `json:"minWidth,omitempty"`
	MaxWidth string `json:"maxWidth,omitempty"`
	// Flex is "row wrap gap 16px/16px justify center items center" for
	// flex containers.
	Flex     string `json:"flex,omitempty"`
	FlexItem string `json:"flexItem,omitempty"` // "grow 1 shrink 1 basis auto"
	// Grid is "cols 240px 1fr rows auto gap 16px/16px" for grid
	// containers (with auto tracks, flow and alignment when set).
	Grid string `json:"grid,omitempty"`
	// GridItem is "col 1 / span 2 row auto / auto" for elements with a
	// grid placement.
	GridItem   string  `json:"gridItem,omitempty"`
	Color      string  `json:"color"`
	Background string  `json:"background,omitempty"`
	Font       string  `json:"font"` // "700 48px/1 Inter"
	TextAlign  string  `json:"textAlign,omitempty"`
	Opacity    float32 `json:"opacity"`
	// Errors are the class errors of the element's classes.
	Errors []string `json:"errors,omitempty"`
}

// Inspect returns the box tree with its layout, after the last Frame.
func (t *Tree) Inspect() InspectNode {
	if t.rootBox == nil || t.rootBox.cs == nil {
		return InspectNode{Kind: "root"}
	}
	return t.inspect(t.rootBox, gfx.Point{})
}

func (t *Tree) inspect(b *Box, origin gfx.Point) InspectNode {
	l := b.ld.Layout()
	r := gfx.RectFromOriginSize(origin.Add(l.Location), l.Size)
	n := InspectNode{
		Kind:    b.kind.String(),
		Rect:    r,
		Bounds:  t.inspectBounds(b, r),
		Margin:  edges(l.Margin),
		Border:  edges(l.Border),
		Padding: edges(l.Padding),
		Style:   summarize(b),
	}
	if b.el == nil {
		n.Kind = "anonymous"
	} else {
		if b.el.keyed {
			n.Key = fmt.Sprint(b.el.key)
		}
		if c := b.el.component(); c != nil {
			n.Component = c.comp.name()
		}
	}
	if b.mode == modeParagraph && b.para != nil {
		n.Text = b.para.text
		if b.para.lay != nil {
			n.Lines = b.para.lay.NumLines()
		}
		// The inline elements, those whose paragraph it is, in order.
		if b.owner != nil {
			n.Inline = t.inlineKindsOf(b.owner, b, nil)
		}
		if p := b.para; p.lay != nil && len(p.inl) > 0 {
			c := contentRect(r, l)
			p.prepare(t, max(c.Width(), 0))
			for _, f := range p.fragments() {
				if cs := p.inlineStyle(f.Inline); cs != nil {
					n.Fragments = append(n.Fragments, InspectFragment{Inline: f.Inline, Line: f.Line,
						Rect: fragBorderBox(cs, &f).Translate(c.X0, c.Y0)})
				}
			}
		}
	}
	n.InlineBox = b.parent != nil && b.parent.mode == modeParagraph
	for _, k := range b.kids {
		n.Children = append(n.Children, t.inspect(k, r.Origin()))
	}
	return n
}

// inspectBounds returns the painted bounds of box b (ElementRef.Bounds)
// when they are not r, its layout rect: nil otherwise. Differences under a
// thousandth of a pixel are float noise of the two ways of summing the
// offsets.
func (t *Tree) inspectBounds(b *Box, r gfx.Rect) *gfx.Rect {
	if b == t.rootBox {
		return nil
	}
	w := t.boxWindowRect(b)
	same := func(a, b float32) bool { return a-b < 1e-3 && b-a < 1e-3 }
	if same(w.X0, r.X0) && same(w.Y0, r.Y0) && same(w.X1, r.X1) && same(w.Y1, r.Y1) {
		return nil
	}
	return &w
}

// inlineKindsOf lists the inline primitives under el whose paragraph is
// para.
func (t *Tree) inlineKindsOf(el *Element, para *Box, out []string) []string {
	for _, c := range el.children {
		switch c.kind {
		case elemPrimitive:
			if c.box.inPara == para {
				out = append(out, c.box.kind.String())
				out = t.inlineKindsOf(c, para, out)
			} else if c.box.cs != nil && c.box.cs.Display == style.DisplayContents {
				out = t.inlineKindsOf(c, para, out)
			}
		case elemComponent:
			out = t.inlineKindsOf(c, para, out)
		}
	}
	return out
}

func edges(in layout.Insets) Edges {
	return Edges{Top: in.Top, Right: in.Right, Bottom: in.Bottom, Left: in.Left}
}

func summarize(b *Box) StyleSummary {
	cs := b.cs
	s := StyleSummary{
		Display: cs.Display.String(),
		Color:   cs.Text.Color.String(),
	}
	if b.el != nil {
		srcs := b.el.node.d.classes
		s.Classes = strings.Join(srcs, " ")
		for _, c := range b.classes {
			if c.Err != nil {
				s.Errors = append(s.Errors, c.Err.Error())
			}
		}
	}
	if cs.Position != style.PositionStatic {
		s.Position = cs.Position.String()
	}
	dim := func(d style.Dim) string {
		if d.IsAuto() {
			return ""
		}
		return d.String()
	}
	s.Width, s.Height = dim(cs.Width), dim(cs.Height)
	s.MinWidth, s.MaxWidth = dim(cs.MinWidth), dim(cs.MaxWidth)
	if isFlexContainer(cs) {
		f := cs.FlexDirection.String()
		if cs.FlexWrap != style.FlexNoWrap {
			f += " wrap"
		}
		if cs.RowGap != (style.LP{}) || cs.ColumnGap != (style.LP{}) {
			f += fmt.Sprintf(" gap %v/%v", cs.RowGap, cs.ColumnGap)
		}
		if cs.JustifyContent != style.AlignAuto {
			f += " justify " + cs.JustifyContent.String()
		}
		if cs.AlignItems != style.AlignAuto {
			f += " items " + cs.AlignItems.String()
		}
		s.Flex = f
	}
	if isGridContainer(cs) {
		g := "cols " + cs.GridTemplateColumns.String() + " rows " + cs.GridTemplateRows.String()
		auto := func(ts []style.TrackSize) string {
			out := make([]string, len(ts))
			for i, t := range ts {
				out[i] = t.String()
			}
			return strings.Join(out, " ")
		}
		if len(cs.GridAutoColumns) > 0 {
			g += " auto-cols " + auto(cs.GridAutoColumns)
		}
		if len(cs.GridAutoRows) > 0 {
			g += " auto-rows " + auto(cs.GridAutoRows)
		}
		if cs.GridAutoFlow != style.GridFlowRow {
			g += " flow " + cs.GridAutoFlow.String()
		}
		if cs.RowGap != (style.LP{}) || cs.ColumnGap != (style.LP{}) {
			g += fmt.Sprintf(" gap %v/%v", cs.RowGap, cs.ColumnGap)
		}
		for _, a := range []struct {
			name string
			v    style.Align
		}{{"justify", cs.JustifyContent}, {"content", cs.AlignContent}, {"justify-items", cs.JustifyItems}, {"items", cs.AlignItems}} {
			if a.v != style.AlignAuto {
				g += " " + a.name + " " + a.v.String()
			}
		}
		s.Grid = g
	}
	if cs.GridColumn != (style.GridPlacement{}) || cs.GridRow != (style.GridPlacement{}) {
		s.GridItem = "col " + cs.GridColumn.String() + " row " + cs.GridRow.String()
	}
	if cs.FlexGrow != 0 || cs.FlexShrink != 1 || !cs.FlexBasis.IsAuto() {
		s.FlexItem = fmt.Sprintf("grow %g shrink %g basis %v", cs.FlexGrow, cs.FlexShrink, cs.FlexBasis)
	}
	if cs.BackgroundColor.A > 0 {
		s.Background = cs.BackgroundColor.String()
	}
	if len(cs.Backgrounds) > 0 {
		if s.Background != "" {
			s.Background += " "
		}
		s.Background += fmt.Sprintf("+%d image layer(s)", len(cs.Backgrounds))
	}
	family := "sans-serif"
	if len(cs.Text.FontFamily) > 0 {
		family = cs.Text.FontFamily[0]
	}
	lh := ""
	switch cs.Text.LineHeight.Kind {
	case text.LineHeightScale:
		lh = fmt.Sprintf("/%g", cs.Text.LineHeight.Value)
	case text.LineHeightPx:
		lh = fmt.Sprintf("/%gpx", cs.Text.LineHeight.Value)
	}
	s.Font = fmt.Sprintf("%g %gpx%s %s", cs.Text.FontWeight, cs.Text.FontSize, lh, family)
	if a := cs.Text.TextAlign; a != 0 {
		s.TextAlign = [...]string{"start", "end", "left", "right", "center", "justify"}[min(int(a), 5)]
	}
	s.Opacity = cs.Opacity
	return s
}
