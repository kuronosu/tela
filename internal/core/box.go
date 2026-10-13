package core

import (
	"image"
	"math"
	"slices"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// boxMode says what a box lays out.
type boxMode uint8

const (
	// modeContainer lays out child boxes (block or flex).
	modeContainer boxMode = iota
	// modeParagraph is a leaf that lays out inline content: one
	// text.Layout (ARCHITECTURE section 11).
	modeParagraph
	// modeImage is a replaced leaf: an Img.
	modeImage
)

// Box is a render object (ARCHITECTURE sections 4 and 6): a computed
// style, a layout style, layout results and what to paint. A primitive's
// Element owns one Box; anonymous boxes (no Element) wrap runs of inline
// content between block-level siblings (CSS anonymous block boxes) and
// text runs of flex containers (anonymous flex items).
//
// Box implements layout.Node; paragraph and image boxes are leaves that
// implement layout.Measurer.
//
// Not every Box is in the box tree: an inline primitive (Span, Strong...)
// inside a paragraph contributes styled text to the paragraph's layout and
// has no geometry of its own (inPara), and a display: contents primitive
// lifts its children into its parent.
type Box struct {
	tree *Tree
	el   *Element // nil for anonymous boxes
	// owner is the primitive whose box structure created this box: el,
	// or the enclosing primitive of an anonymous box.
	owner *Element
	kind  style.Kind

	parent *Box   // layout parent; nil for the root and boxes not in the tree
	kids   []*Box // layout children
	anon   []*Box // anonymous boxes of this box's structure, reused between passes

	cs       *style.ComputedStyle
	anonFrom *style.ComputedStyle // anonymous boxes: the owner's style cs inherits from
	// deferLayout and deferPaint report phase-deferred props of the
	// layout and paint phases in cs (ARCHITECTURE section 7), resolved
	// when the layout style is mapped and when the box is painted. The
	// Values they read are tracked by layoutObs and paintObs (value.go).
	deferLayout, deferPaint bool
	layoutObs, paintObs     *observer
	// relayoutQueued: in Tree.relayoutQ. needsPaint: in Tree.paintQ.
	relayoutQueued bool
	needsPaint     bool

	ls layout.Style
	ld layout.Data

	mode boxMode
	para *paragraph

	// Img.
	img    *gfx.Image
	imgSrc image.Image

	// Compiled classes of the element's Class props.
	classes        []*style.Classes
	classesDirty   bool
	structural     bool // the classes use first:, last:, only:, odd: or even:
	structuralKids bool // some child's classes do

	// built reports that the box's own structure (kids or paragraph) is
	// current: it is a block-level or flex-level box, not inline content.
	built bool
	// inPara is the paragraph box whose text includes this inline box.
	inPara *Box

	// Paint pass scratch state (painter.place): the paint-phase style and
	// the absolute, unsnapped border box.
	pcs *style.ComputedStyle
	abs gfx.Rect

	// Per-frame scratch state (valid when the frame matches).
	inlineFrame uint64
	inlineVal   bool
	sibFrame    uint64
	sibIndex    int32
	sibCount    int32
	restyled    bool
	remapQueued bool
}

func newBox(t *Tree, el *Element, kind style.Kind) *Box {
	return &Box{tree: t, el: el, owner: el, kind: kind, ls: layout.DefaultStyle(), classesDirty: true}
}

// detach takes an unmounted box out of the tree and ends its Value
// subscriptions.
func (b *Box) detach() {
	b.parent = nil
	b.kids = nil
	b.anon = nil
	b.inPara = nil
	b.para = nil
	b.built = false
	dropObserver(b.layoutObs)
	dropObserver(b.paintObs)
}

// LayoutStyle implements layout.Node.
func (b *Box) LayoutStyle() *layout.Style { return &b.ls }

// LayoutChildCount implements layout.Node.
func (b *Box) LayoutChildCount() int { return len(b.kids) }

// LayoutChild implements layout.Node.
func (b *Box) LayoutChild(i int) layout.Node { return b.kids[i] }

// LayoutData implements layout.Node.
func (b *Box) LayoutData() *layout.Data { return &b.ld }

// Measure implements layout.Measurer for paragraph and image leaves.
func (b *Box) Measure(in layout.MeasureInput) layout.Measurement {
	switch b.mode {
	case modeParagraph:
		if b.para != nil {
			return b.para.measure(b.tree, in)
		}
	case modeImage:
		return b.measureImage(in)
	}
	return knownOnly(in)
}

func knownOnly(in layout.MeasureInput) layout.Measurement {
	var m layout.Measurement
	if in.HasWidth {
		m.Width = in.Width
	}
	if in.HasHeight {
		m.Height = in.Height
	}
	return m
}

// measureImage returns the natural size of the image (1 image pixel = 1
// logical pixel, like CSS), keeping its aspect ratio when one dimension is
// known.
func (b *Box) measureImage(in layout.MeasureInput) layout.Measurement {
	if b.img == nil || b.img.Width() == 0 || b.img.Height() == 0 {
		return knownOnly(in)
	}
	nw, nh := float32(b.img.Width()), float32(b.img.Height())
	m := layout.Measurement{Width: nw, Height: nh}
	switch {
	case in.HasWidth && in.HasHeight:
		m.Width, m.Height = in.Width, in.Height
	case in.HasWidth:
		m.Width, m.Height = in.Width, float32(in.Width*nh)/nw
	case in.HasHeight:
		m.Width, m.Height = float32(in.Height*nw)/nh, in.Height
	}
	return m
}

// updateImage converts the Img source of el's node when it changed.
func (t *Tree) updateImage(el *Element) {
	d := el.node.d
	b := el.box
	img := d.img
	if img == nil && d.src != nil {
		if b.imgSrc != nil && safeEqual(b.imgSrc, d.src) && b.img != nil {
			return
		}
		img = gfx.NewImage(d.src)
	}
	if img == b.img {
		return
	}
	old := b.img
	b.img, b.imgSrc = img, d.src
	if old == nil || img == nil || old.Width() != img.Width() || old.Height() != img.Height() {
		t.engine.MarkDirty(b)
		b.queueRemap() // the natural aspect ratio is part of the layout style
	}
	t.dirty()
}

// queueRemap asks the layout phase to map the layout styles of b's
// children and of b again.
func (b *Box) queueRemap() {
	if b.remapQueued {
		return
	}
	b.remapQueued = true
	b.tree.remapQ = append(b.tree.remapQ, b)
	if b.parent != nil && !b.parent.remapQueued {
		b.parent.remapQueued = true
		b.tree.remapQ = append(b.tree.remapQ, b.parent)
	}
}

// contentRect returns the content box of a box whose border box is r.
func contentRect(r gfx.Rect, l *layout.Layout) gfx.Rect {
	return r.InsetEdges(l.Border.Left+l.Padding.Left, l.Border.Top+l.Padding.Top,
		l.Border.Right+l.Padding.Right, l.Border.Bottom+l.Padding.Bottom)
}

// ---------------------------------------------------------------------
// Paragraphs.

// paragraph is the inline content of a paragraph box: the text.Layout of
// its text runs, and what it was built from, to tell a change that needs
// shaping again (text or text style) from one that only needs new line
// breaks (width).
type paragraph struct {
	text   string
	spans  []text.Span
	boxes  []text.InlineBox
	def    text.TextStyle
	pstyle text.ParagraphStyle
	// empty reports text that collapses to nothing (white space only):
	// the box has no line boxes, as in CSS.
	empty bool
	lay   *text.Layout

	// brokeAt is the width of the last BreakLines (+Inf after NewLayout)
	// and alignedAt the width of the last alignment.
	brokeAt, alignedAt float32
}

// breakAt breaks the lines at width w unless they already are.
func (p *paragraph) breakAt(t *Tree, w float32) {
	if p.brokeAt == w {
		return
	}
	p.lay.BreakLines(w)
	p.brokeAt, p.alignedAt = w, w
	t.stats.Breaks++
}

// prepare breaks and aligns the lines for a content box of width w, before
// painting. When the lines cannot change (both widths fit the whole
// content), only the alignment is redone.
func (p *paragraph) prepare(t *Tree, w float32) {
	if p.lay == nil || (p.brokeAt == w && p.alignedAt == w) {
		return
	}
	const eps = 1.0 / 1024 // text's fitting tolerance
	if maxW := p.lay.MaxContentWidth(); p.brokeAt >= maxW && w+eps >= maxW {
		if p.alignedAt != w { // lines broken at +Inf stay aligned at w across paints
			p.lay.AlignTo(w)
			p.alignedAt = w
		}
		return
	}
	p.breakAt(t, w)
}

// measure implements the layout.Measurer contract for the paragraph: the
// content-box size at a width, with the first baseline.
func (p *paragraph) measure(t *Tree, in layout.MeasureInput) layout.Measurement {
	if p.empty || p.lay == nil {
		return knownOnly(in)
	}
	l := p.lay
	minW, maxW := l.MinContentWidth(), l.MaxContentWidth()
	var w float32
	switch {
	case in.HasWidth:
		w = in.Width
	case in.AvailableWidth.Kind == layout.MinContentSpace:
		w = minW
	case in.AvailableWidth.Kind == layout.MaxContentSpace:
		w = maxW
	default:
		// CSS fit-content: min(max-content, max(min-content, available)).
		w = min(maxW, max(minW, in.AvailableWidth.Value))
	}
	if w >= maxW {
		// Every width at or above max-content gives the same lines.
		if p.brokeAt < maxW {
			p.breakAt(t, float32(math.Inf(1)))
		}
	} else {
		p.breakAt(t, w)
	}
	m := layout.Measurement{Width: w, Height: l.Height()}
	if in.HasHeight {
		m.Height = in.Height
	}
	if l.NumLines() > 0 {
		m.Baseline, m.HasBaseline = l.Line(0).Baseline, true
	}
	return m
}

// paraPart is a piece of inline content: a text and the style of the
// inline element it is in (nil for the paragraph box itself).
type paraPart struct {
	text string
	cs   *style.ComputedStyle
}

// paraAssembler collects the inline content of one paragraph.
type paraAssembler struct {
	parts []paraPart
	boxes []text.InlineBox
	n     int // bytes of text so far
	sb    strings.Builder
	spans []text.Span
}

func (a *paraAssembler) reset() {
	clear(a.parts)
	a.parts, a.boxes, a.spans, a.n = a.parts[:0], a.boxes[:0], a.spans[:0], 0
}

func (a *paraAssembler) addText(s string, cs *style.ComputedStyle) {
	if s == "" {
		return
	}
	a.parts = append(a.parts, paraPart{s, cs})
	a.n += len(s)
}

// addSpace adds an empty inline box of width w: the margins of
// space-x-* between inline elements.
func (a *paraAssembler) addSpace(w float32) {
	a.boxes = append(a.boxes, text.InlineBox{ID: len(a.boxes), At: a.n, Width: w})
}

// build returns the text and its spans (text without a span takes the
// paragraph's default style).
func (a *paraAssembler) build() (string, []text.Span) {
	var s string
	switch len(a.parts) {
	case 0:
	case 1:
		s = a.parts[0].text
	default:
		a.sb.Reset()
		a.sb.Grow(a.n)
		for _, p := range a.parts {
			a.sb.WriteString(p.text)
		}
		s = a.sb.String()
	}
	off := 0
	for i, p := range a.parts {
		end := off + len(p.text)
		if p.cs != nil {
			if i > 0 && a.parts[i-1].cs == p.cs && len(a.spans) > 0 && a.spans[len(a.spans)-1].End == off {
				a.spans[len(a.spans)-1].End = end
			} else {
				a.spans = append(a.spans, text.Span{Start: off, End: end, Style: p.cs.TextStyle()})
			}
		}
		off = end
	}
	return s, a.spans
}

// commitParagraph replaces the paragraph of box b with the assembled
// content, shaping it again only when the text or a text style changed.
// It reports whether the paragraph has content (is not empty).
func (t *Tree) commitParagraph(b *Box, a *paraAssembler) bool {
	s, spans := a.build()
	pstyle := b.cs.ParagraphStyle()
	def := b.cs.TextStyle()
	empty := len(a.boxes) == 0 && collapsesAway(s, pstyle.WhiteSpace)
	p := b.para
	if p != nil && p.text == s && p.pstyle == pstyle && sameTextStyle(&p.def, &def) &&
		sameSpans(p.spans, spans) && slices.Equal(p.boxes, a.boxes) {
		return !p.empty
	}
	if p == nil {
		p = new(paragraph)
		b.para = p
	}
	p.text, p.pstyle, p.def, p.empty = s, pstyle, def, empty
	p.spans = slices.Clone(spans)
	p.boxes = slices.Clone(a.boxes)
	p.lay = nil
	if !empty {
		p.lay = text.NewLayout(t.fonts, &text.Paragraph{Text: s, Style: pstyle, Default: def, Spans: p.spans, Boxes: p.boxes})
		t.stats.Shapes++
	}
	inf := float32(math.Inf(1))
	p.brokeAt, p.alignedAt = inf, inf
	t.engine.MarkDirty(b)
	t.dirty()
	return !empty
}

// collapsesAway reports whether text s produces no line box under the
// white-space mode ws: it is empty, or white space that collapses.
func collapsesAway(s string, ws text.WhiteSpace) bool {
	switch ws {
	case text.WhiteSpaceNormal, text.WhiteSpaceNowrap:
		return strings.Trim(s, " \t\n\r\f") == ""
	case text.WhiteSpacePreLine:
		return strings.Trim(s, " \t") == ""
	}
	return s == ""
}

func sameSpans(a, b []text.Span) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Start != b[i].Start || a[i].End != b[i].End || !sameTextStyle(&a[i].Style, &b[i].Style) {
			return false
		}
	}
	return true
}

// sameTextStyle reports whether two text styles certainly lay out and
// paint the same. Paints other than solid colors compare as different.
func sameTextStyle(a, b *text.TextStyle) bool {
	return slices.Equal(a.Families, b.Families) && a.Size == b.Size && a.Weight == b.Weight &&
		a.Style == b.Style && a.Stretch == b.Stretch && a.LetterSpacing == b.LetterSpacing &&
		a.WordSpacing == b.WordSpacing && a.LineHeight == b.LineHeight && samePaint(a.Paint, b.Paint) &&
		a.Decoration.Lines == b.Decoration.Lines && a.Decoration.Thickness == b.Decoration.Thickness &&
		samePaint(a.Decoration.Paint, b.Decoration.Paint) && a.Transform == b.Transform &&
		slices.Equal(a.Features, b.Features) && a.Numeric == b.Numeric && a.Language == b.Language
}

func samePaint(a, b gfx.Paint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ca, ok1 := a.(gfx.Color)
	cb, ok2 := b.(gfx.Color)
	return ok1 && ok2 && ca == cb
}
