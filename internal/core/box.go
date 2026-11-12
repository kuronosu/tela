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
	// modeField is a text field: a leaf that lays out and paints its
	// editor's text (field.go).
	modeField
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
	// anonLines: an anonymous paragraph box that holds all the in-flow
	// content of its owner takes the owner's text-overflow and line-clamp.
	anonLines bool
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

	// Img and Icon: a raster image (from Src, SrcImage or a decoded
	// source) and the source given by SrcPath, SrcBytes, SrcFS or an
	// Icon with what it decoded to (media.go).
	img    *gfx.Image
	imgSrc image.Image
	med    *boxMedia

	// Compiled classes of the element's Class props.
	classes        []*style.Classes
	classesDirty   bool
	structural     bool // the classes use first:, last:, only:, odd: or even:
	structuralKids bool // some child's classes do
	divides        bool // the classes use divide-*: the children are styled by their position (ADR-203)
	// The interaction states the classes test on the element itself
	// (hover:, active:, disabled:, enabled: as disabled), on its group
	// ancestors (group-*) and on its peer siblings (peer-*); isGroup and
	// isPeer report the marker classes group and peer (input.go).
	stateUse, groupUse, peerUse style.StateFlags
	isGroup, isPeer             bool
	// startingUse reports starting: rules: a box that starts being
	// rendered transitions from its starting style (anim.go).
	startingUse bool
	// styledFrame is Tree.frame when the box was styled for the first
	// time; one at or after Tree.frameStart was styled in the current
	// Frame, whose rerun starts its animations again (Tree.restyle).
	// shownFrame is Tree.frame when its own display: none last ended, for
	// the rerun likewise (ADR-272).
	styledFrame, shownFrame uint64

	// built reports that the box's own structure (kids or paragraph) is
	// current: it is a block-level or flex-level box, not inline content.
	built bool
	// inPara is the paragraph box whose text includes this inline box.
	inPara *Box
	// anchor is, for an out-of-flow box in a paragraph's inline content,
	// that paragraph's box (structure.go), anchorOff the byte offset of
	// its place in the text, anchorBox the index of its anchor in the
	// paragraph's boxes (text.InlineBox.Anchor) and anchorCB the inline
	// element of the paragraph that is its containing block, or -1
	// (layout.InlinePlacer, ADR-268).
	anchor    *Box
	anchorOff int32
	anchorBox int32
	anchorCB  int32
	// hiddenAtom reports an atomic inline that its paragraph's
	// text-overflow or line-clamp hides: it is not painted nor hit.
	hiddenAtom bool

	// Paint pass scratch state (painter.place): the paint-phase style and
	// the unsnapped border box where it is painted (u: its layout
	// position without the scroll offsets, in the coordinates of the
	// scroll space it is painted in; see layer.go). space is that scroll
	// space: the nearest scroll container with a content layer on the
	// box's containing-block chain (nil for the window).
	pcs   *style.ComputedStyle
	u     gfx.Rect
	space *scroller
	// hitBounds contains everything the box and its descendants paint
	// that can be hit (their border boxes, transformed), in the
	// coordinates of its scroll space, after the box's own transform
	// (hitSub before it): a hit test skips a subtree whose bounds miss
	// the point (hit.go).
	hitBounds, hitSub gfx.Rect
	// ink contains what the box paints in flow (painter.paintFlow: its
	// decorations, content, scrollbars and outline, and its in-flow
	// descendants, or the scrollport of a scroller's content layer), in u
	// coordinates: layers skip boxes whose ink misses their interest rect
	// (layer.go). hasPart reports positioned or stacking descendants
	// (painter.collect skips subtrees without them).
	ink     gfx.Rect
	hasPart bool
	// blendDesc reports descendants with a mix-blend-mode in the box's
	// stacking context scope (not inside nested stacking contexts): a
	// stacking context with them is an isolated group (cssfx.go).
	blendDesc bool
	// layerSC reports whether the box is a layer of its own (a stacking
	// context with paint-phase deferred props, layer.go) at the last full
	// recording.
	layerSC bool
	// inkKey and selfInkRect cache painter.selfInk.
	inkKey      inkKey
	selfInkRect gfx.Rect
	// rin and rout are what painting the box and placing it read at its
	// last place, rinAt and routAt the epochs at which they last changed,
	// rinN and routN how many lists read them (record.go).
	rin           paintIn
	rout          paintOut
	rinAt, routAt uint64
	rinN, routN   int32
	sb            styleBits
	// grp reports that the box's painting is recorded apart, in a group
	// of its own (record.go, "Groups"); wt is the weight of its painting
	// (the boxes it paints in its list outside groups, a drawing as two,
	// and for a stacking context the participants it paints), and parts
	// how many participants of its enclosing stacking context are below
	// it, both as of its last place and at most groupMin.
	grp       bool
	wt, parts uint16
	// glayer is the box's group layer, nil when it has none (adopt);
	// rcol is what a stacking context that collects its participants
	// through it read (kdep), rcolAt and rcolN as for rin.
	glayer *layer
	rcol   colIn
	rcolAt uint64
	rcolN  int32

	// anim holds the running transitions and keyframe animation of the box
	// (anim.go); nil when none.
	anim *boxAnim
	// lanim is the layout animation state of a box whose element has
	// AnimateLayout or LayoutID (layoutanim.go); nil otherwise.
	lanim *layoutAnim
	// scr is the scroll state of a scroll container (scroll.go).
	scr *scroller
	// portal marks the box of a Portal: a portal layer (portal.go).
	portal bool
	// stick is the scroll space of a sticky box (sticky.go), nil until it
	// is first painted sticky; the box is in it when space == stick.
	stick *scroller
	// flt is the state of a floating element (floating.go): its own scroll
	// space, its Place function and its size limits; nil for other boxes.
	flt *floatState
	// fld is the state of a text field (field.go).
	fld *field

	// Per-frame scratch state (valid when the frame matches).
	inlineFrame uint64
	inlineVal   bool
	// inlineOOF: an inline element whose content has out-of-flow
	// elements (inlineLevel), hoisted out of its paragraph.
	inlineOOF bool
	sibFrame  uint64
	sibIndex  int32
	sibCount  int32
	restyled  bool
	// remapQueued: in Tree.remapQ (its children's layout styles are
	// mapped again); remapSelf: in Tree.remapSelfQ (its own is);
	// mappedAt is Tree.remapGen when its layout style was last mapped.
	remapQueued, remapSelf bool
	mappedAt               uint64

	// Partial places (place.go): placeDirty marks the box for the next
	// place, dirtyBelow a box below it; placeDeep is the place in which
	// its whole subtree is placed; placedKids and placedCB are its
	// children and whether it contained positioned boxes at its last
	// place; listed says which list of the sticky boxes and floating
	// elements it is in.
	placeDirty, dirtyBelow bool
	placeDeep              uint64
	placedKids             kidsRef
	placedCB               bool
	listed                 uint8
	hasListed              bool // a listed box is below it
	// kidIdx is the box's index in its parent's children; aggs are the
	// sums of a box with many children by runs of aggChunk, aggDirty the
	// runs that hold a marked child and aggAll that they all may (paint.go:
	// aggChunks).
	kidIdx   int32
	aggs     []kidAgg
	aggDirty []uint64
	aggAll   bool
}

func newBox(t *Tree, el *Element, kind style.Kind) *Box {
	return &Box{tree: t, el: el, owner: el, kind: kind, ls: layout.DefaultStyle(), classesDirty: true}
}

// detach takes an unmounted box out of the tree and ends its Value
// subscriptions.
func (b *Box) detach() {
	if b.scr != nil {
		b.tree.dropScroller(b)
	}
	b.tree.dropAnim(b)
	if b.cs != nil && b.cs.HasTextColorFn() {
		b.tree.textColorFns--
	}
	if b.lanim != nil {
		b.tree.dropLayoutAnim(b) // before the parent link goes: it records the box
	}
	b.parent = nil
	b.kids = nil
	b.anon = nil
	b.inPara = nil
	b.anchor = nil
	b.para = nil
	b.built = false
	dropObserver(b.layoutObs)
	dropObserver(b.paintObs)
	if f := b.flt; f != nil {
		dropObserver(f.obs)
		dropObserver(f.aobs)
	}
}

// LayoutStyle implements layout.Node.
func (b *Box) LayoutStyle() *layout.Style { return &b.ls }

// LayoutChildCount implements layout.Node.
func (b *Box) LayoutChildCount() int { return len(b.kids) }

// LayoutChild implements layout.Node.
func (b *Box) LayoutChild(i int) layout.Node { return b.kids[i] }

// LayoutData implements layout.Node.
func (b *Box) LayoutData() *layout.Data { return &b.ld }

// InlineContent implements layout.InlineContainer: a paragraph box whose
// children are its atomic inlines.
func (b *Box) InlineContent() bool {
	return b.mode == modeParagraph && b.para != nil && len(b.para.atoms) > 0 && len(b.kids) == len(b.para.atoms)
}

// MeasureInline implements layout.InlineContainer.
func (b *Box) MeasureInline(in layout.MeasureInput, a *layout.Atoms) layout.Measurement {
	return b.para.measureInline(b.tree, in, a)
}

// Measure implements layout.Measurer for paragraph and image leaves.
func (b *Box) Measure(in layout.MeasureInput) layout.Measurement {
	switch b.mode {
	case modeParagraph:
		if b.para != nil {
			return b.para.measure(b.tree, in)
		}
	case modeImage:
		return b.measureImage(in)
	case modeField:
		if b.fld != nil {
			return b.fld.measure(in)
		}
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
// logical pixel, like CSS; an SVG's size without one, media.go), keeping
// its aspect ratio when one dimension is known.
func (b *Box) measureImage(in layout.MeasureInput) layout.Measurement {
	nw, nh, _ := b.imageSize()
	if !(nw > 0 && nh > 0) {
		return knownOnly(in)
	}
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
	if m := d.media; m != nil && m.src.kind != srcNone {
		t.updateMedia(el, &m.src)
		return
	}
	if b.med != nil {
		// From a decoded source back to Src or SrcImage (or none).
		b.med = nil
		b.img, b.imgSrc = nil, nil
		t.markPlace(b)
		t.markContent(b)
		b.queueRemap()
		t.markMediaPaint(b)
	}
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
	t.markPlace(b)
	if old == nil || img == nil || old.Width() != img.Width() || old.Height() != img.Height() {
		t.markContent(b)
		b.queueRemap() // the natural aspect ratio is part of the layout style
	}
	t.dirty()
}

// markContent tells layout that the content of box b changed (its
// paragraph's text or text styles, its image, its field's text) and not
// its layout style: layout measures it again and lays out its ancestors
// only if its results changed (layout.Engine.MarkContent, ADR-280). A
// paragraph whose parent reads more of it than its size, its atomic
// inlines (its children) or the out-of-flow boxes anchored in its text,
// is marked dirty.
func (t *Tree) markContent(b *Box) {
	if len(b.kids) > 0 || b.para != nil && hasAnchors(b.para.boxes) {
		t.engine.MarkDirty(b)
		return
	}
	t.engine.MarkContent(b)
}

// hasAnchors reports whether a paragraph's inline boxes hold the place of
// an out-of-flow box.
func hasAnchors(boxes []text.InlineBox) bool {
	for i := range boxes {
		if boxes[i].Anchor {
			return true
		}
	}
	return false
}

// queueRemap asks the layout phase to map the layout styles of b's
// children and of b again. b is mapped on its own (Tree.remapSelfQ), not
// with its siblings: a sibling's layout style reads its parent, not b
// (mapLayoutStyle), and a change that moves the parent's last element (a
// child that starts or stops being display: contents, mounts or unmounts)
// changes the parent's children, which queues the parent.
func (b *Box) queueRemap() {
	if !b.remapQueued {
		b.remapQueued = true
		b.tree.remapQ = append(b.tree.remapQ, b)
	}
	if !b.remapSelf {
		b.remapSelf = true
		b.tree.remapSelfQ = append(b.tree.remapSelfQ, b)
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
	// em is the largest font size of the text, and ink the bounds of the
	// lines (relative to the content box) for the lines of inkAt (ink).
	em    float32
	ink   gfx.Rect
	inkAt [3]float32
	// relOff is the largest relative offset of its inline elements
	// (offsetInlines), which the ink includes, and posInl the positioned
	// ones (relative or sticky), painted after the rest (inline.go).
	relOff float32
	posInl []int
	// positions are the positions and insets of its inline elements at
	// the last commit (samePositions).
	positions []inlinePosition

	// brokeAt is the width of the last BreakLines (+Inf after NewLayout)
	// and alignedAt the width of the last alignment.
	brokeAt, alignedAt float32

	// targets map the text to the elements it belongs to, for
	// hit-testing (hit.go): entry i covers the bytes from the end of
	// entry i-1 to end, which belong to the innermost primitive el.
	targets []paraTarget

	// inlines are the inline elements as given to the layout, inl their
	// elements (by index); atoms are the boxes of the atomic inlines (the
	// paragraph box's children) and atomBox their inline box indices.
	inlines []text.Inline
	inl     []*Element
	atoms   []*Box
	atomBox []int
	// edgeK is the display scale the inlines' edges were assembled at:
	// their border widths are snapped to its device pixels (ADR-273).
	edgeK float32
	// spanEls are the elements whose styles the spans have (span i is
	// style i+1 of the layout), for TextColorFn (paint.go).
	spanEls []*Element
	// final holds the atoms' sizes of the last full layout; sized reports
	// that the layout's boxes have them (a size query since may have
	// changed them).
	final []layout.AtomSize
	sized bool
	// frags caches the inline fragments for the lines of fragAt.
	frags  []text.InlineFragment
	fragAt [2]float32
	// ver counts the changes of what painting the paragraph reads beyond
	// its layout object and width: a commit, new paints, a new scale, a
	// new style of an inline element, its part of the static selection
	// (record.go).
	ver uint64
	// hl is what the paragraph paints of the static selection; nsel and
	// selGen are the scratch state of the walk that computes it
	// (textsel.go).
	hl, nsel paraSel
	selGen   uint64
}

// paraTarget is a run of paragraph text that belongs to one element.
type paraTarget struct {
	end int
	el  *Element
}

// targetAt returns the element whose text holds byte offset off.
func (p *paragraph) targetAt(off int) *Element {
	i, j := 0, len(p.targets)
	for i < j {
		h := int(uint(i+j) >> 1)
		if p.targets[h].end <= off {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(p.targets) {
		return p.targets[i].el
	}
	return nil
}

// rescale quantizes the line boxes for the tree's scale
// (text.Layout.SetScale) and forgets what depends on them: the lines are
// broken again and box b is laid out again.
func (p *paragraph) rescale(t *Tree, b *Box) {
	if p.edgeK != t.scale {
		// Border widths of its inline elements are snapped at the old
		// scale in the edges it was shaped with: assembled again.
		for _, el := range p.inl {
			if el != nil && el.mounted && el.box.cs != nil && hasBorderWidth(el.box.cs) {
				t.markBoxes(el)
			}
		}
	}
	if p.lay == nil || p.lay.Scale() == t.scale {
		return
	}
	p.lay.SetScale(t.scale)
	p.ver++
	t.markPlace(b)
	p.inkAt, p.fragAt = [3]float32{}, [2]float32{}
	p.brokeAt = float32(math.NaN())
	t.markContent(b)
	t.dirty()
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
	if p.lay == nil {
		return
	}
	if !p.sized && len(p.final) == len(p.atoms) && len(p.atoms) > 0 {
		// A size query changed the atomic inlines since the layout:
		// give them back their sizes.
		for i, bi := range p.atomBox {
			p.setAtom(bi, p.final[i])
		}
		p.sized, p.fragAt = true, [2]float32{}
		p.brokeAt = float32(math.NaN())
	}
	defer p.offsetInlines(w)
	if p.brokeAt == w && p.alignedAt == w {
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

// offsetInlines marks the positioned inline elements of the paragraph
// (text.Layout.SetInlinePositioned) and gives the layout the relative
// offsets of those with position: relative (SetInlineOffset), for a
// content box of width w, the containing block of their insets (CSS 2.1
// section 9.4.3, as Chromium moves a relative span with its content):
// left wins over right in a left-to-right paragraph, right in a
// right-to-left one, top over bottom. Percentages of left and right are of
// w; those of top and bottom count as 0 (the containing block's height,
// which the paragraph does not know, is mostly auto). It forgets the
// fragments and the ink when an offset changes.
func (p *paragraph) offsetInlines(w float32) {
	if p.lay == nil || len(p.inl) == 0 {
		return
	}
	var maxOff float32
	changed := false
	p.posInl = p.posInl[:0]
	for k := range p.inl {
		var dx, dy float32
		cs := p.inlineStyle(k)
		pos := cs != nil && (cs.Position == style.PositionRelative || cs.Position == style.PositionSticky)
		if pos {
			p.posInl = append(p.posInl, k)
			if cs.Position == style.PositionRelative {
				dx, dy = inlineRelative(cs, w, p.lay.RTL())
			}
		}
		maxOff = max(maxOff, float32(math.Abs(float64(dx))), float32(math.Abs(float64(dy))))
		if p.lay.SetInlinePositioned(k, pos) {
			changed = true
		}
		if p.lay.SetInlineOffset(k, dx, dy) {
			changed = true
		}
	}
	if maxOff != p.relOff {
		p.relOff = maxOff
		p.inkAt = [3]float32{}
	}
	if changed {
		p.fragAt = [2]float32{}
		p.ver++ // what painting reads (record.go)
	}
}

// inlineRelative returns the relative offset of an inline element with
// style cs in a containing block of width w (offsetInlines).
func inlineRelative(cs *style.ComputedStyle, w float32, rtl bool) (dx, dy float32) {
	set := func(d style.Dim) bool { return d.Kind == style.DimLength }
	in := &cs.Inset
	l, r := set(in.Left), set(in.Right)
	switch {
	case l && (!r || !rtl):
		dx = in.Left.LP.Resolve(w)
	case r:
		dx = -in.Right.LP.Resolve(w)
	}
	switch {
	case set(in.Top):
		dy = in.Top.LP.Px
	case set(in.Bottom):
		dy = -in.Bottom.LP.Px
	}
	if !finite32(dx) {
		dx = 0
	}
	if !finite32(dy) {
		dy = 0
	}
	return dx, dy
}

// inkRect returns a rectangle that contains what the paragraph paints for
// a content box at origin c, its lines prepared (prepare): the extent of
// the lines grown by twice the largest font size (glyphs overhang their
// line boxes, ascenders, accents and emoji included).
func (p *paragraph) inkRect(c gfx.Point) gfx.Rect {
	if p.lay == nil || p.empty {
		return gfx.Rect{}
	}
	key := [3]float32{p.brokeAt, p.alignedAt, 1}
	if p.inkAt != key {
		l := p.lay
		r := gfx.Rect{X0: 0, Y0: 0, X1: l.Width(), Y1: l.Height()}
		for i := range l.NumLines() {
			ln := l.Line(i)
			r.X0 = min(r.X0, ln.X)
			r.X1 = max(r.X1, ln.X+ln.Width+ln.Hanging)
			r.Y1 = max(r.Y1, ln.Top+ln.Height)
		}
		p.ink, p.inkAt = r.Outset(2*p.em+1+p.relOff), key
	}
	return p.ink.Translate(c.X, c.Y)
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
	if n := l.NumLines(); n > 0 {
		m.Baseline, m.HasBaseline = l.Line(0).Baseline, true
		m.LastBaseline, m.HasLastBaseline = l.Line(n-1).Baseline, true
	}
	return m
}

// measureInline is measure for a paragraph with atomic inlines (CSS 2.1
// sections 10.3.9 and 10.8): their min- and max-content contributions are
// part of the paragraph's intrinsic widths; each is sized fit-content in
// the paragraph's width, which sets its inline box (the margin box, its
// baseline above the bottom); in a full layout the lines are aligned in
// the content box and each atomic inline is placed where its inline box
// is (a hidden one, truncated or clamped, is not placed).
func (p *paragraph) measureInline(t *Tree, in layout.MeasureInput, a *layout.Atoms) layout.Measurement {
	if p.empty || p.lay == nil || a.Len() != len(p.atoms) {
		return knownOnly(in)
	}
	l := p.lay
	for i, bi := range p.atomBox {
		mn, mx := a.Intrinsic(i)
		l.SetBoxIntrinsic(bi, mn, mx)
	}
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
		w = min(maxW, max(minW, in.AvailableWidth.Value))
	}
	full := a.Full()
	if full {
		p.final = p.final[:0]
	}
	// perf: every request breaks the lines again (the boxes' sizes depend
	// on the width).
	for i, bi := range p.atomBox {
		s := a.Size(i, w)
		p.setAtom(bi, s)
		if full {
			p.final = append(p.final, s)
		}
	}
	p.sized, p.fragAt = full, [2]float32{}
	p.brokeAt = float32(math.NaN())
	if w >= maxW && !full {
		p.breakAt(t, float32(math.Inf(1)))
	} else {
		p.breakAt(t, w)
	}
	if full {
		l.AlignTo(w)
		p.alignedAt = w
		p.offsetInlines(w)
		for i, bi := range p.atomBox {
			r, ok := l.BoxRect(bi)
			if ok {
				a.Place(i, r.X0, r.Y0)
			}
			if a := p.atoms[i]; a.hiddenAtom == ok {
				a.hiddenAtom = !ok
				t.markPlace(a)
			}
		}
	}
	m := layout.Measurement{Width: w, Height: l.Height()}
	if in.HasHeight {
		m.Height = in.Height
	}
	if n := l.NumLines(); n > 0 {
		m.Baseline, m.HasBaseline = l.Line(0).Baseline, true
		m.LastBaseline, m.HasLastBaseline = l.Line(n-1).Baseline, true
	}
	return m
}

// setAtom gives inline box bi the size of an atomic inline: its margin box,
// the baseline Descent above its bottom (the bottom itself without a
// baseline).
func (p *paragraph) setAtom(bi int, s layout.AtomSize) {
	var d float32
	if s.HasBaseline {
		d = s.Height - s.Baseline
	}
	p.lay.SetBoxSize(bi, s.Width, s.Height, d)
}

// paraPart is a piece of inline content: a text and the style of the
// inline element it is in (nil for the paragraph box itself), the
// innermost primitive it belongs to and the text element it comes from.
type paraPart struct {
	text string
	cs   *style.ComputedStyle
	el   *Element
	src  *Element
}

// paraAssembler collects the inline content of one paragraph.
type paraAssembler struct {
	parts []paraPart
	boxes []text.InlineBox
	n     int // bytes of text so far
	sb    strings.Builder
	spans []text.Span
	// inlines are the inline elements (text.Inline, tree order) and inl
	// their elements; atoms are the boxes of the atomic inlines and
	// atomBox their indices in boxes; open is the stack of open inlines.
	inlines []text.Inline
	inl     []*Element
	atoms   []*Box
	atomBox []int
	open    int
	// spanEls are the elements whose styles the spans have.
	spanEls []*Element
	// oof are the out-of-flow elements of the content, in tree order:
	// boxes of the block that holds the paragraph, not part of its text
	// (structure.go). stack holds the indices of the open inlines.
	oof   []*Element
	stack []int
}

func (a *paraAssembler) reset() {
	clear(a.parts)
	clear(a.inlines)
	clear(a.inl)
	clear(a.atoms)
	clear(a.spanEls)
	a.spanEls = a.spanEls[:0]
	clear(a.oof)
	a.oof, a.stack = a.oof[:0], a.stack[:0]
	a.parts, a.boxes, a.spans, a.n = a.parts[:0], a.boxes[:0], a.spans[:0], 0
	a.inlines, a.inl, a.atoms, a.atomBox, a.open = a.inlines[:0], a.inl[:0], a.atoms[:0], a.atomBox[:0], 0
}

// openInline starts inline element el at the end of the content and
// returns its index; closeInline ends it.
func (a *paraAssembler) openInline(el *Element) int {
	cs := el.box.cs
	left, right := inlineEdges(cs, el.box.tree.scale)
	a.inlines = append(a.inlines, text.Inline{
		ID: len(a.inlines), Start: a.n, End: a.n, BoxStart: len(a.boxes), BoxEnd: len(a.boxes),
		Depth: a.open, Style: cs.TextStyle(), Left: left, Right: right,
	})
	a.inl = append(a.inl, el)
	a.open++
	a.stack = append(a.stack, len(a.inlines)-1)
	return len(a.inlines) - 1
}

func (a *paraAssembler) closeInline(k int) {
	a.inlines[k].End, a.inlines[k].BoxEnd = a.n, len(a.boxes)
	a.open--
	a.stack = a.stack[:len(a.stack)-1]
}

// addOutOfFlow records out-of-flow element el at the end of the content of
// the paragraph of box para: its place on the line, an anchor box of the
// text (text.InlineBox.Anchor), and the innermost open inline element that
// is positioned (relative or sticky), its containing block (ADR-268).
func (a *paraAssembler) addOutOfFlow(el *Element, para *Box) {
	a.oof = append(a.oof, el)
	bx := el.box
	bx.anchor, bx.anchorOff, bx.anchorBox, bx.anchorCB = para, int32(a.n), int32(len(a.boxes)), -1
	a.boxes = append(a.boxes, text.InlineBox{ID: len(a.boxes), At: a.n, Anchor: true})
	for i := len(a.stack) - 1; i >= 0; i-- {
		k := a.stack[i]
		if cs := a.inl[k].box.cs; cs != nil && (cs.Position == style.PositionRelative || cs.Position == style.PositionSticky) {
			bx.anchorCB = int32(k)
			break
		}
	}
}

// inlineEdges returns the widths that the left and right edges of an
// inline element with style cs take on its lines: margin, border and
// padding (CSS inline boxes; percentages and auto margins count as 0), at
// display scale k.
func inlineEdges(cs *style.ComputedStyle, k float32) (left, right float32) {
	e := edgeWidths(cs, k)
	return e.margin.Left + e.border.Left + e.padding.Left, e.margin.Right + e.border.Right + e.padding.Right
}

// addAtom adds the atomic inline whose box is b: an inline box sized at
// layout (box.go: measureInline).
func (a *paraAssembler) addAtom(b *Box) {
	a.atomBox = append(a.atomBox, len(a.boxes))
	a.atoms = append(a.atoms, b)
	a.boxes = append(a.boxes, text.InlineBox{ID: len(a.boxes), At: a.n, Align: b.cs.VerticalAlign})
}

func (a *paraAssembler) addText(s string, cs *style.ComputedStyle, el, src *Element) {
	if s == "" {
		return
	}
	a.parts = append(a.parts, paraPart{s, cs, el, src})
	a.n += len(s)
}

// setSources records on each text element where its text is in the
// paragraph of box b (the semantics tree's text bounds).
func (a *paraAssembler) setSources(b *Box) {
	off := 0
	for _, part := range a.parts {
		if part.src != nil {
			part.src.paraBox, part.src.paraOff = b, int32(off)
		}
		off += len(part.text)
	}
}

// setTargets records in p which element each part of the text belongs
// to.
func (a *paraAssembler) setTargets(p *paragraph) {
	clear(p.targets)
	p.targets = p.targets[:0]
	off := 0
	for _, part := range a.parts {
		off += len(part.text)
		if n := len(p.targets); n > 0 && p.targets[n-1].el == part.el {
			p.targets[n-1].end = off
			continue
		}
		p.targets = append(p.targets, paraTarget{end: off, el: part.el})
	}
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
				a.spanEls = append(a.spanEls, p.el)
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
	// CSS 2.1 section 9.4.2: a line box without text, preserved white
	// space, inline boxes or inlines with margins, borders or padding
	// does not exist.
	// Anchors are not content: a line of out-of-flow boxes alone is none.
	empty := !hasContentBoxes(a.boxes) && !inlineEdged(a.inlines) && collapsesAway(s, pstyle.WhiteSpace)
	p := b.para
	if p != nil {
		p.ver++ // its elements, paints or text may change (record.go)
		t.markPlace(b)
		// The edges were just assembled at the tree's scale (inlineEdges).
		p.edgeK = t.scale
	}
	if p != nil && !p.samePositions(a) {
		// A relative inline element moved: its absolute children are laid
		// out again (offsetInlines).
		t.engine.MarkDirty(b)
		t.dirty()
	}
	if p != nil && p.text == s && p.pstyle == pstyle && sameTextStyle(&p.def, &def) &&
		sameSpans(p.spans, spans) && slices.Equal(p.boxes, a.boxes) && sameInlines(p.inlines, a.inlines) {
		a.setTargets(p)
		a.setSources(b)
		p.setElements(a)
		p.rescale(t, b)
		return !p.empty
	}
	if p != nil && p.text == s && p.pstyle == pstyle && sameTextShape(&p.def, &def) &&
		sameSpansBy(p.spans, spans, sameTextShape) && slices.Equal(p.boxes, a.boxes) && sameInlinesBy(p.inlines, a.inlines, sameTextShape) {
		// Only paints changed (a text color: a state class, a selected
		// date): the shaping, the lines and the box's layout hold, so the
		// layout takes the new paints and nothing is laid out again
		// (ADR-161).
		p.recolor(&def, spans, a.inlines)
		a.setTargets(p)
		a.setSources(b)
		p.setElements(a)
		p.rescale(t, b)
		t.dirty()
		return !p.empty
	}
	if p == nil {
		p = &paragraph{edgeK: t.scale}
		b.para = p
		p.samePositions(a)
	}
	a.setTargets(p)
	a.setSources(b)
	p.setElements(a)
	p.inlines = slices.Clone(a.inlines)
	p.text, p.pstyle, p.def, p.empty = s, pstyle, def, empty
	p.em = def.Size
	for i := range spans {
		p.em = max(p.em, spans[i].Style.Size)
	}
	p.spans = slices.Clone(spans)
	p.boxes = slices.Clone(a.boxes)
	t.shapeParagraph(b, p)
	return !empty
}

// shapeParagraph shapes the text of paragraph p, the paragraph of box b,
// and forgets what depends on its layout: the lines are broken again and
// b is laid out again.
func (t *Tree) shapeParagraph(b *Box, p *paragraph) {
	p.final, p.sized, p.fragAt = nil, false, [2]float32{}
	t.semGeomBox(b) // the bounds of its texts and inline elements
	t.markPlace(b)
	p.inkAt = [3]float32{}
	p.lay = nil
	if !p.empty {
		p.lay = text.NewLayout(t.fonts, &text.Paragraph{Text: p.text, Style: p.pstyle, Default: p.def, Spans: p.spans, Boxes: p.boxes, Inlines: p.inlines, Scale: t.scale})
		t.stats.Shapes++
	}
	inf := float32(math.Inf(1))
	p.brokeAt, p.alignedAt = inf, inf
	t.markContent(b)
	t.dirty()
}

// reshape shapes the paragraph again, with the same text and styles: the
// fonts resolve them to other faces (the desktop's families changed,
// fonts.go).
func (p *paragraph) reshape(t *Tree, b *Box) {
	p.ver++ // a new layout (record.go)
	t.shapeParagraph(b, p)
}

// inlinePosition is what offsetInlines reads of an inline element's style.
type inlinePosition struct {
	pos   style.Position
	inset style.Sides[style.Dim]
}

// samePositions reports whether the inline elements assembled in a have
// the positions and insets that p recorded, and records them.
func (p *paragraph) samePositions(a *paraAssembler) bool {
	old := p.positions
	same := len(old) == len(a.inl)
	p.positions = p.positions[:0]
	for k, el := range a.inl {
		var ip inlinePosition
		if cs := el.box.cs; cs != nil {
			ip = inlinePosition{cs.Position, cs.Inset}
		}
		if same && old[k] != ip { // read before the append writes index k
			same = false
		}
		p.positions = append(p.positions, ip)
	}
	return same
}

// hasContentBoxes reports whether boxes has a box that is not an anchor.
func hasContentBoxes(boxes []text.InlineBox) bool {
	for i := range boxes {
		if !boxes[i].Anchor {
			return true
		}
	}
	return false
}

// inlineEdged reports whether an inline has a left or right edge.
func inlineEdged(ins []text.Inline) bool {
	for i := range ins {
		if ins[i].Left != 0 || ins[i].Right != 0 {
			return true
		}
	}
	return false
}

// sameInlines reports whether two inline lists lay out and paint the
// same.
func sameInlines(a, b []text.Inline) bool { return sameInlinesBy(a, b, sameTextStyle) }

// sameInlinesBy reports whether two inline lists are the same, comparing
// their text styles with same.
func sameInlinesBy(a, b []text.Inline, same func(a, b *text.TextStyle) bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := &a[i], &b[i]
		if x.ID != y.ID || x.Start != y.Start || x.End != y.End || x.BoxStart != y.BoxStart || x.BoxEnd != y.BoxEnd ||
			x.Depth != y.Depth || x.Left != y.Left || x.Right != y.Right || !same(&x.Style, &y.Style) {
			return false
		}
	}
	return true
}

// recolor gives paragraph p the paints of def, spans and inlines, which
// are its styles but for their paints (sameTextShape): its layout takes
// them without shaping or breaking its lines again (text.Layout.SetPaint),
// since a NewLayout of the new styles would lay the text out the same.
func (p *paragraph) recolor(def *text.TextStyle, spans []text.Span, inlines []text.Inline) {
	set := func(i int, cur, next *text.TextStyle) {
		if samePaint(cur.Paint, next.Paint) && samePaint(cur.Decoration.Paint, next.Decoration.Paint) {
			return
		}
		cur.Paint, cur.Decoration.Paint = next.Paint, next.Decoration.Paint
		if p.lay != nil {
			p.lay.SetPaint(i, next.Paint, next.Decoration.Paint)
		}
	}
	set(0, &p.def, def)
	for i := range spans {
		set(1+i, &p.spans[i].Style, &spans[i].Style)
	}
	for j := range inlines {
		set(1+len(spans)+j, &p.inlines[j].Style, &inlines[j].Style)
	}
}

// setElements records the elements of the assembled inlines and atoms.
func (p *paragraph) setElements(a *paraAssembler) {
	clear(p.inl)
	p.inl = append(p.inl[:0], a.inl...)
	clear(p.spanEls)
	p.spanEls = append(p.spanEls[:0], a.spanEls...)
	clear(p.atoms)
	p.atoms = append(p.atoms[:0], a.atoms...)
	p.atomBox = append(p.atomBox[:0], a.atomBox...)
	if len(p.atoms) == 0 {
		p.atoms = nil
	}
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

func sameSpans(a, b []text.Span) bool { return sameSpansBy(a, b, sameTextStyle) }

// sameSpansBy reports whether two span lists are the same, comparing their
// text styles with same.
func sameSpansBy(a, b []text.Span, same func(a, b *text.TextStyle) bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Start != b[i].Start || a[i].End != b[i].End || !same(&a[i].Style, &b[i].Style) {
			return false
		}
	}
	return true
}

// sameTextStyle reports whether two text styles certainly lay out and
// paint the same. Paints other than solid colors compare as different.
func sameTextStyle(a, b *text.TextStyle) bool {
	return sameTextShape(a, b) && samePaint(a.Paint, b.Paint) && samePaint(a.Decoration.Paint, b.Decoration.Paint)
}

// sameTextShape reports whether two text styles certainly lay out the
// same: they are equal but for their paints (the text's and the
// decorations'), which a text layout only paints with.
func sameTextShape(a, b *text.TextStyle) bool {
	return slices.Equal(a.Families, b.Families) && a.Size == b.Size && a.Weight == b.Weight &&
		a.Style == b.Style && a.Stretch == b.Stretch && a.LetterSpacing == b.LetterSpacing &&
		a.WordSpacing == b.WordSpacing && a.LineHeight == b.LineHeight &&
		a.Decoration.Lines == b.Decoration.Lines && a.Decoration.Thickness == b.Decoration.Thickness &&
		a.Transform == b.Transform && slices.Equal(a.Features, b.Features) && a.Numeric == b.Numeric && a.Language == b.Language &&
		a.Hyphens == b.Hyphens
}

func samePaint(a, b gfx.Paint) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ca, ok1 := a.(gfx.Color)
	cb, ok2 := b.(gfx.Color)
	return ok1 && ok2 && ca == cb
}
