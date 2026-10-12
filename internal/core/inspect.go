package core

import (
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
	// Rect is the border box in absolute logical pixels (unrounded).
	Rect gfx.Rect `json:"rect"`
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
	// Style summarizes the computed style.
	Style    StyleSummary  `json:"style"`
	Children []InspectNode `json:"children,omitempty"`
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
	Flex       string  `json:"flex,omitempty"`
	FlexItem   string  `json:"flexItem,omitempty"` // "grow 1 shrink 1 basis auto"
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
		if b.el != nil {
			n.Inline = t.inlineKinds(b.el, nil)
		} else if b.owner != nil {
			// The inline elements of an anonymous box: those whose
			// paragraph it is.
			n.Inline = t.inlineKindsOf(b.owner, b, nil)
		}
	}
	for _, k := range b.kids {
		n.Children = append(n.Children, t.inspect(k, r.Origin()))
	}
	return n
}

// inlineKinds lists the inline primitives under el, in order.
func (t *Tree) inlineKinds(el *Element, out []string) []string {
	for _, c := range el.children {
		switch c.kind {
		case elemPrimitive:
			if c.box.inPara != nil {
				out = append(out, c.box.kind.String())
			}
			out = t.inlineKinds(c, out)
		case elemComponent:
			out = t.inlineKinds(c, out)
		}
	}
	return out
}

// inlineKindsOf lists the inline primitives under el whose paragraph is
// para.
func (t *Tree) inlineKindsOf(el *Element, para *Box, out []string) []string {
	for _, c := range el.children {
		switch c.kind {
		case elemPrimitive:
			if c.box.inPara == para {
				out = append(out, c.box.kind.String())
				out = t.inlineKinds(c, out)
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
