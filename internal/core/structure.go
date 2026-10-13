package core

import (
	"slices"

	"github.com/kuronosu/tela/internal/style"
)

// The box phase turns the Element tree into the box tree (ARCHITECTURE
// sections 6 and 11). For each primitive marked by reconciliation or by
// the style phase it collects the content (children through components
// and display: contents) and decides what the box lays out:
//
//   - A block container whose content is all inline (text and inline
//     primitives whose own content is inline) is ONE paragraph: a leaf
//     with one text.Layout whose spans carry each inline element's text
//     style.
//   - A block container with mixed content wraps each run of consecutive
//     inline content in an anonymous paragraph box (CSS anonymous block
//     boxes); runs that are only collapsible white space are dropped.
//   - A flex container wraps each run of consecutive texts in an
//     anonymous paragraph box (anonymous flex item); every primitive child
//     is a flex item (blockified), inline ones included.
//   - An Img is a replaced leaf; its children are ignored.
//   - A display: none primitive generates no box (it is not part of the
//     content at all).
//
// Inline primitives that end up inside a paragraph have no box in the
// tree; their box records the paragraph (inPara) so that a change to them
// rebuilds that paragraph.

// boxPhase rebuilds the structure of the marked primitives, shallowest
// first.
func (t *Tree) boxPhase() {
	for len(t.boxQ) > 0 {
		q := t.boxQ
		t.boxQ = nil
		sortByDepth(q)
		for _, el := range q {
			if el.needsBoxes {
				t.restructure(el)
			}
		}
	}
}

// restructure rebuilds the structure that el's content belongs to: el's
// own, or that of the box el's content is lifted into.
func (t *Tree) restructure(el *Element) {
	el.needsBoxes = false
	if !el.mounted || el.boxFrame == t.frame || el.box.cs == nil {
		return
	}
	bx := el.box
	switch {
	case bx.cs.Display == style.DisplayNone && el.parent != nil:
		// Not in the box tree; built when it is shown again.
		el.boxFrame = t.frame
		bx.built = false
	case bx.cs.Display == style.DisplayContents && el.parent != nil:
		el.boxFrame = t.frame
		t.restructure(el.styleParent())
	case bx.inPara != nil:
		el.boxFrame = t.frame
		t.restructure(bx.inPara.owner)
	default:
		t.buildBoxes(el)
	}
}

// buildBoxes builds the structure of a block-level or flex-level
// primitive: its child boxes or its paragraph.
func (t *Tree) buildBoxes(el *Element) {
	el.boxFrame = t.frame
	el.needsBoxes = false
	bx := el.box
	bx.built = true
	bx.inPara = nil
	t.stats.Structures++

	items := t.collect(t.getItems(), el)
	kids := t.getBoxes()
	oldMode := bx.mode
	nAnon := 0
	switch {
	case bx.kind == style.KindImg:
		bx.mode = modeImage
		bx.para = nil
	case isFlexContainer(bx.cs):
		bx.mode = modeContainer
		bx.para = nil
		for i := 0; i < len(items); {
			if items[i].kind == elemText {
				j := i + 1
				for j < len(items) && items[j].kind == elemText {
					j++
				}
				a := t.anonBox(bx, nAnon)
				if t.setParagraph(a, items[i:j], el) {
					nAnon++
					kids = append(kids, a)
				}
				i = j
				continue
			}
			t.ensureBuilt(items[i])
			kids = append(kids, items[i].box)
			i++
		}
	default:
		inline := len(items) > 0
		for _, it := range items {
			if !t.inlineLevel(it) {
				inline = false
				break
			}
		}
		if inline {
			bx.mode = modeParagraph
			t.setParagraph(bx, items, el)
			break
		}
		bx.mode = modeContainer
		bx.para = nil
		for i := 0; i < len(items); {
			if t.inlineLevel(items[i]) {
				j := i + 1
				for j < len(items) && t.inlineLevel(items[j]) {
					j++
				}
				a := t.anonBox(bx, nAnon)
				if t.setParagraph(a, items[i:j], el) {
					nAnon++
					kids = append(kids, a)
				}
				i = j
				continue
			}
			t.ensureBuilt(items[i])
			kids = append(kids, items[i].box)
			i++
		}
	}
	for _, a := range bx.anon[nAnon:] {
		a.parent, a.para = nil, nil
	}
	clear(bx.anon[nAnon:])
	bx.anon = bx.anon[:nAnon]

	for _, k := range kids {
		k.parent = bx
	}
	if !slices.Equal(kids, bx.kids) || oldMode != bx.mode {
		if len(kids) == 0 {
			bx.kids = nil
		} else {
			bx.kids = slices.Clone(kids)
		}
		t.engine.MarkDirty(bx)
		bx.queueRemap()
		t.dirty()
	}
	t.putBoxes(kids)
	t.putItems(items)
}

// ensureBuilt builds the structure of a block-level child that has none
// yet (it was inline content, display: contents, or is new).
func (t *Tree) ensureBuilt(el *Element) {
	if el.boxFrame != t.frame && (!el.box.built || el.needsBoxes) {
		t.buildBoxes(el)
	}
}

// collect appends the content of el: its text and primitive children,
// looking through components and display: contents primitives.
func (t *Tree) collect(out []*Element, el *Element) []*Element {
	for _, c := range el.children {
		switch c.kind {
		case elemText:
			out = append(out, c)
		case elemComponent:
			out = t.collect(out, c)
		case elemPrimitive:
			if cs := c.box.cs; cs != nil {
				switch cs.Display {
				case style.DisplayContents:
					c.box.built, c.box.inPara, c.box.parent = false, nil, nil
					c.box.kids = nil
					out = t.collect(out, c)
					continue
				case style.DisplayNone:
					// No box, and no effect on its siblings (an inline
					// sibling run stays one paragraph).
					c.box.built, c.box.inPara, c.box.parent = false, nil, nil
					continue
				}
			}
			out = append(out, c)
		}
	}
	return out
}

// inlineLevel reports whether content item el is inline content: a text,
// or an inline primitive (display: inline) whose own content is inline. An
// inline primitive with block-level content is laid out as a block (CSS
// would split it around the blocks).
func (t *Tree) inlineLevel(el *Element) bool {
	if el.kind == elemText {
		return true
	}
	bx := el.box
	if bx.inlineFrame == t.frame {
		return bx.inlineVal
	}
	v := false
	if bx.cs.Display == style.DisplayInline && bx.kind != style.KindImg {
		v = true
		items := t.collect(t.getItems(), el)
		for _, c := range items {
			if !t.inlineLevel(c) {
				v = false
				break
			}
		}
		t.putItems(items)
	}
	bx.inlineFrame, bx.inlineVal = t.frame, v
	return v
}

// anonBox returns anonymous box i of owner's structure, creating it if
// needed, with its style inherited from owner.
func (t *Tree) anonBox(owner *Box, i int) *Box {
	var a *Box
	if i < len(owner.anon) {
		a = owner.anon[i]
	} else {
		a = &Box{tree: t, owner: owner.el, kind: style.KindDiv, ls: layout0}
		owner.anon = append(owner.anon, a)
	}
	if a.anonFrom != owner.cs {
		// An anonymous box inherits the owner's inherited properties and
		// has the initial values of the others (CSS 2.1 section 9.2.1.1).
		a.cs = style.Compute(&t.cx, style.Input{Kind: style.KindDiv, Parent: owner.cs})
		a.anonFrom = owner.cs
	}
	a.built = true
	a.mode = modeParagraph
	return a
}

// setParagraph assembles the inline content items into the paragraph of
// box b, which belongs to the structure of primitive owner. It reports
// whether the paragraph has content.
func (t *Tree) setParagraph(b *Box, items []*Element, owner *Element) bool {
	a := &t.asm
	a.reset()
	// space-x-* between inline elements: Tailwind puts a margin after
	// every element child but the last; text runs are not elements.
	var gap float32
	if ocs := owner.box.cs; ocs.SpaceX && !isFlexContainer(ocs) && ocs.ColumnGap.Pct == 0 {
		gap = ocs.ColumnGap.Px
	}
	elems := 0
	if gap > 0 {
		for _, it := range items {
			if it.kind == elemPrimitive {
				elems++
			}
		}
	}
	seen := 0
	for _, it := range items {
		if it.kind == elemText {
			a.addText(it.text, t.textStyle(it, b, owner), it.styleParent(), it)
			continue
		}
		t.addInline(a, it, b, owner)
		seen++
		if gap > 0 && seen < elems {
			a.addSpace(gap)
		}
	}
	return t.commitParagraph(b, a)
}

// addInline adds the content of inline primitive el to the paragraph of
// box para.
func (t *Tree) addInline(a *paraAssembler, el *Element, para *Box, owner *Element) {
	bx := el.box
	bx.inPara = para
	bx.built, bx.parent, bx.kids, bx.para, bx.mode = false, nil, nil, nil, modeContainer
	for i := range bx.anon {
		bx.anon[i].parent = nil
	}
	bx.anon = nil
	items := t.collect(t.getItems(), el)
	for _, c := range items {
		if c.kind == elemText {
			a.addText(c.text, t.textStyle(c, para, owner), c.styleParent(), c)
		} else {
			t.addInline(a, c, para, owner)
		}
	}
	t.putItems(items)
}

// textStyle returns the style of the text of element el in the paragraph
// of box para: the style of its nearest primitive ancestor (a text node
// inherits everything), or nil when that is the paragraph's own style.
func (t *Tree) textStyle(el *Element, para *Box, owner *Element) *style.ComputedStyle {
	sp := el.styleParent()
	if sp == nil || sp == owner || sp.box.cs == para.cs {
		return nil
	}
	return sp.box.cs
}

// isFlexContainer reports whether a box with style cs lays out its
// children as flex items.
func isFlexContainer(cs *style.ComputedStyle) bool {
	return cs.Display == style.DisplayFlex || cs.Display == style.DisplayInlineFlex
}

// sortByDepth sorts elements by depth, keeping the queue order of equal
// depths (deterministic).
func sortByDepth(q []*Element) {
	slices.SortStableFunc(q, func(a, b *Element) int { return int(a.depth - b.depth) })
}

// Scratch buffers (the box phase recurses, so each level takes its own).

func (t *Tree) getItems() []*Element {
	if n := len(t.itemFree); n > 0 {
		b := t.itemFree[n-1]
		t.itemFree = t.itemFree[:n-1]
		return b
	}
	return make([]*Element, 0, 8)
}

func (t *Tree) putItems(b []*Element) {
	clear(b)
	t.itemFree = append(t.itemFree, b[:0])
}

func (t *Tree) getBoxes() []*Box {
	if n := len(t.boxFree); n > 0 {
		b := t.boxFree[n-1]
		t.boxFree = t.boxFree[:n-1]
		return b
	}
	return make([]*Box, 0, 8)
}

func (t *Tree) putBoxes(b []*Box) {
	clear(b)
	t.boxFree = append(t.boxFree, b[:0])
}
