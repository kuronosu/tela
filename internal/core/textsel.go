package core

import (
	"image"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Selection of static text (ADR-271; ARCHITECTURE sections 11 and 14).
// Chromium is the reference: its SelectionController, FrameSelection and
// TextIterator.
//
// # The range
//
// Text outside text fields is selectable unless its user-select is none
// (inherited, so select-none on a container covers what is inside it
// and select-text inside that gives it back), as on the web. A selection
// is a range of the document, from an anchor (where it started) to a
// focus (where it ends now), each a cursor in the text.Layout of a
// paragraph box (selPos). Document order is the order of the box tree,
// with the content of an atomic inline (an inline-block badge, a button
// in the text) at the place of its inline box in its paragraph's text.
// The range lives in one selection root: the window's content (the root
// box without its portals), or one portal (an overlay: a dialog, a
// popover, a menu), as Chromium's document-wide range would cross into
// what is appended to body; a range never leaves its root, and a drag
// over another root extends the range to the position the point gives
// in its own root. Text fields are not part of any range: their
// selection is their own (field.go), as an editable's is in Chromium,
// and focusing one clears the static selection. Each window has its own
// selection. The selection belongs to the Tree; positions in boxes that
// leave the tree clear it, and a paragraph whose text changes keeps
// cursors clamped to its new text.
//
// # Painting
//
// Every paragraph that the range covers highlights its part (and, when
// the range goes on past its end, the break of its last line), with the
// selection colors of text fields: the selection: background and text
// colors, or Chromium's (fieldpaint.go). An atomic inline is not
// highlighted as a box: its own text is. The text of a positioned inline
// element, painted after the rest of its paragraph (inline.go), has its
// highlight painted with it, over its background and moved by its
// offset, as Chromium paints the highlight with each text fragment. The
// highlight is part of what painting a paragraph reads (paragraph.ver),
// so a change records again the layers that paint the paragraphs whose
// part changed, and damage tracking rasterizes what changed: no layer of
// its own, and no frame when nothing changes. A change of the range (a
// drag, a Shift press) computes the parts of the paragraphs between the
// old and the new place of each end that moved, not of the whole range
// (updateHighlights); a frame that changed the tree walks the whole range
// (checkSel).
//
// # The pointer
//
// A primary press of a mouse or a pen that neither pointerdown nor
// mousedown handlers cancelled may start a selection (Chromium's
// CanStartSelection): not on a Button (which starts none from its text
// either: the owner's rule, where Chromium starts one from a button's
// text and none from its padding), a Draggable element or an element with
// drag gesture handlers (they drag), a title bar's drag area or window
// control, text whose user-select is none, nor
// a press that an aria press hook handled (React Aria's usePress sets
// user-select: none on the pressed element), nor a Mac context click;
// such a press leaves the selection as it is. A press on a link clears
// it and starts none (Chromium would drag the link). Touch selects
// nothing (no touch selection yet). Otherwise the press puts a
// collapsed selection (a caret, which shows nothing) at the position
// under the pointer, which clears what was selected; with Shift it
// extends the selection instead (from the anchor; under the Mac flavor
// from the end farther from the press, Chromium's non-directional
// selection); a double click selects a word and a triple click a
// paragraph, with the break after it (Chromium's paragraph granularity).
// The content of an element whose user-select is all is selected as a
// whole: a press in it selects all of it, and an end that a drag or a
// Shift press brings into it goes to its far edge.
// Moving with the button held extends the focus to the position under
// the pointer by the unit of the press, without a drag slop, also over
// other roots and outside the window, and scrolls the scroll containers
// around the press's text toward a pointer past their edges, by the
// distance past the edge every frame (Chromium's selection autoscroll).
// The click of such a press still fires on release, as the DOM fires it
// after a selection drag inside one element; a drag gesture of another
// element that wins the arena (OnDrag, a Draggable) collapses the
// selection the press made.
//
// A single press without Shift inside the selection does not change it:
// past the drag slop it starts a drag and drop of the selected text
// (text/plain, copy), with the selected text alone as its image, as a
// field's selection is dragged (fielddnd.go); released in the device
// pixel where it was pressed, it clears the selection.
//
// The position under a point (posAt) is Chromium's PositionForPoint: the
// box hit under it (its paragraph for an inline element), then down to a
// paragraph, choosing among the children of a block container the last
// one whose top is above the point or else the first whose bottom is
// below it, and among those of a flex or grid container the nearest one;
// the cursor in a paragraph is its HitTest. A point that leads to no
// selectable paragraph takes the start of the next one in document order,
// or the end of the previous one.
//
// # Keys
//
// With the focus outside text fields: Ctrl+C or Ctrl+Insert (Command+C
// under the Mac flavor, ADR-221) copies the selection as plain text
// (selectedText), and Ctrl+A (Command+A) selects all the text of the
// root of the focused element (else of the selection, else of the
// window's content). Escape leaves the selection, and the arrows do not
// move it (Chromium without caret browsing). The ContextMenu key and
// Shift+F10 open a context menu at the selection when the selection has
// the focus (Chromium's SelectionHasFocus): nothing is focused, or the
// focused element contains the selection's start, or the range contains
// the focused element.

// selPos is a position of the static selection: a cursor of a paragraph
// box's text.Layout.
type selPos struct {
	bx  *Box
	cur text.Cursor
}

// selUnit is the unit a selection is made and extended by.
type selUnit uint8

const (
	selChar selUnit = iota
	selWord
	selPara
)

// textSel is the static selection of a Tree (UI thread).
type textSel struct {
	// root is the selection root the range is in; anchor.bx is nil when
	// there is no selection.
	root          *Box
	anchor, focus selPos
	// unit is the unit of the press that made the selection, and
	// ustart and uend the unit it first selected (a word, a paragraph),
	// which a drag keeps inside the selection.
	unit         selUnit
	ustart, uend selPos
	// drag is the pointer whose moves extend the selection, until its
	// release.
	drag *pointerState
	// inSel is a single press inside the selection, at the device pixel
	// pressAt (devicePixel), which a drag past the slop drags and a
	// release in that pixel clears.
	inSel   *pointerState
	pressAt image.Point
	// hl are the paragraph boxes that paint a highlight, in no order, and
	// hlAt the index of each in hl; gen numbers the walks that compute
	// them (paragraph.selGen).
	hl   []*Box
	hlAt map[*Box]int
	gen  uint64
	// shown says that the highlights are those of the range [hs, he] of
	// root shownRoot: a change of the range from there updates only the
	// paragraphs between the old and the new place of each end
	// (updateHighlights).
	shown     bool
	shownRoot *Box
	hs, he    selPos
	// touch is the scratch of updateHighlights, and touched the number of
	// paragraphs whose part the last change of the highlights computed
	// (tests).
	touch   []*Box
	touched int
}

// paraSel is what a paragraph paints of the static selection.
type paraSel struct {
	on   bool
	sel  text.Selection
	more bool // the range goes on past the end of its text
}

// active reports whether there is a selection (a caret included).
func (s *textSel) active() bool { return s.anchor.bx != nil }

// list puts paragraph box bx in hl when on, else takes it out.
func (s *textSel) list(bx *Box, on bool) {
	i, in := s.hlAt[bx]
	switch {
	case on && !in:
		if s.hlAt == nil {
			s.hlAt = make(map[*Box]int)
		}
		s.hlAt[bx] = len(s.hl)
		s.hl = append(s.hl, bx)
	case !on && in:
		last := len(s.hl) - 1
		s.hl[i] = s.hl[last]
		s.hlAt[s.hl[i]] = i
		s.hl[last] = nil
		s.hl = s.hl[:last]
		delete(s.hlAt, bx)
	}
}

// ---------------------------------------------------------------------
// Boxes and positions.

// paraBox reports whether bx lays out text: a paragraph with a layout.
func paraBox(bx *Box) bool {
	return bx != nil && bx.mode == modeParagraph && bx.para != nil && bx.para.lay != nil
}

// selectableBox reports whether the text of paragraph box bx can be
// selected: it has text, is visible, its user-select is not none, and its
// element takes input (not inert, not outside the active modal).
func selectableBox(bx *Box) bool {
	if !paraBox(bx) || bx.para.empty || bx.cs == nil || bx.cs.UserSelect == style.UserSelectNone {
		return false
	}
	if cs := paintStyle(bx); cs.Visibility != style.VisibilityVisible {
		return false
	}
	o := bx.owner
	return o != nil && o.mounted && !o.inert && o.outside == outsideNo
}

// selRoot returns the selection root of bx: the portal box it is in, or
// the root box; nil for a box out of the tree.
func (t *Tree) selRoot(bx *Box) *Box {
	for b := bx; b != nil; b = b.parent {
		if b.portal || b == t.rootBox {
			return b
		}
		if b.ls.Display == layout.DisplayNone {
			return nil
		}
	}
	return nil
}

// cmpCur orders two cursors of one layout: by offset, Upstream (before
// the inline boxes at the offset) before Downstream.
func cmpCur(a, b text.Cursor) int {
	switch {
	case a.Offset != b.Offset:
		if a.Offset < b.Offset {
			return -1
		}
		return 1
	case a.Affinity == b.Affinity:
		return 0
	case a.Affinity == text.Upstream:
		return -1
	}
	return 1
}

// atomSlot returns the text offset of the inline box of atomic inline k
// of paragraph box p, and its index among p's atoms.
func atomSlot(p, k *Box) (off, idx int, ok bool) {
	if !paraBox(p) {
		return 0, 0, false
	}
	for i, a := range p.para.atoms {
		if a == k {
			return p.para.boxes[p.para.atomBox[i]].At, i, true
		}
	}
	return 0, 0, false
}

// chainOfBox appends bx and its ancestors, up to the root, to dst.
func chainOfBox(dst []*Box, bx *Box) []*Box {
	for b := bx; b != nil; b = b.parent {
		dst = append(dst, b)
	}
	return dst
}

// cmpPos orders two positions in document order (-1, 0, 1). Both must be
// in the tree.
func (t *Tree) cmpPos(a, b selPos) int {
	if a.bx == b.bx {
		return cmpCur(a.cur, b.cur)
	}
	ca := chainOfBox(nil, a.bx)
	cb := chainOfBox(nil, b.bx)
	i, j := len(ca)-1, len(cb)-1
	for i >= 0 && j >= 0 && ca[i] == cb[j] {
		i, j = i-1, j-1
	}
	if i+1 >= len(ca) {
		return 0 // different trees: unordered
	}
	c := ca[i+1] // the common ancestor
	// inSlot orders cursor cur of c with the content of its atom k.
	inSlot := func(cur text.Cursor, k *Box) int {
		off, _, ok := atomSlot(c, k)
		if !ok {
			return -1 // not an atom: after the text
		}
		if cur.Offset < off || cur.Offset == off && cur.Affinity == text.Upstream {
			return -1
		}
		return 1
	}
	switch {
	case i < 0: // a is in c itself, b in one of its children
		return inSlot(a.cur, cb[j])
	case j < 0:
		return -inSlot(b.cur, ca[i])
	}
	ka, kb := ca[i], cb[j]
	if oa, ia, ok := atomSlot(c, ka); ok {
		if ob, ib, ok := atomSlot(c, kb); ok {
			switch {
			case oa != ob:
				if oa < ob {
					return -1
				}
				return 1
			case ia < ib:
				return -1
			}
			return 1
		}
	}
	for _, k := range c.kids {
		switch k {
		case ka:
			return -1
		case kb:
			return 1
		}
	}
	return 0
}

// docBounds returns the first and last cursors of the text of paragraph
// box bx.
func docBounds(bx *Box) (start, end text.Cursor) {
	all := bx.para.lay.SelectAll()
	return all.Anchor, all.Focus
}

// clampPos returns p with its cursor snapped to the current text of its
// paragraph (which may have changed since it was taken).
func clampPos(p selPos) selPos {
	lay := p.bx.para.lay
	if n := len(lay.Text()); p.cur.Offset > n {
		p.cur.Offset = n
	}
	p.cur = lay.SnapCursor(p.cur)
	return p
}

// posValid reports whether p is a position of the tree in root.
func (t *Tree) posValid(p selPos, root *Box) bool {
	return p.bx != nil && paraBox(p.bx) && t.inTree(p.bx) && t.selRoot(p.bx) == root
}

// ---------------------------------------------------------------------
// The document walk.

// rangeWalk walks the boxes of a range in document order and reports
// the part of each paragraph it covers (emit), with the line breaks that
// the blocks between the parts give their text (nl: 1 for a block
// boundary, 2 when a paragraph or heading with a bottom margin of at
// least half its font size ends, Chromium's extra newline).
type rangeWalk struct {
	start, end selPos
	state      uint8 // 0 before the start, 1 inside, 2 past the end
	nl         int
	// chain is start.bx and its ancestors (chainOfBox): before the start
	// the walk enters only the boxes on it, as no other box before the
	// start holds a part of the range, so a walk costs the range it
	// covers and not the boxes before it.
	chain []*Box
	// emit reports a part [a, b] of paragraph bx; it returns whether it
	// took the pending line breaks.
	emit func(bx *Box, a, b text.Cursor, nl int) bool
}

// walk runs w over the subtree of bx, an atomic inline when atom; ci is
// the index of bx in w.chain, or -1 when it is not on it.
func (w *rangeWalk) walk(bx *Box, atom bool, ci int) {
	if w.state == 2 || w.state == 0 && ci < 0 || bx.ls.Display == layout.DisplayNone || bx.cs == nil {
		return
	}
	if !atom {
		w.boundary(1)
	}
	if paraBox(bx) {
		w.para(bx, ci)
	} else {
		for _, k := range bx.kids {
			if w.state == 2 {
				break
			}
			if !k.portal {
				w.walk(k, false, w.on(k, ci))
			}
		}
	}
	if !atom {
		n := 1
		if extraNewline(bx) {
			n = 2
		}
		w.boundary(n)
	}
}

// boundary notes a block boundary worth n line breaks.
func (w *rangeWalk) boundary(n int) {
	if w.state == 1 {
		w.nl = max(w.nl, n)
	}
}

// on returns the index in w.chain of k, a child of the box at index ci (-1
// for a box not on it), or -1.
func (w *rangeWalk) on(k *Box, ci int) int {
	if ci > 0 && w.chain[ci-1] == k {
		return ci - 1
	}
	return -1
}

// para walks paragraph box bx (at index ci of w.chain): its text cut at
// its atomic inlines, whose content comes at their place.
func (w *rangeWalk) para(bx *Box, ci int) {
	pa := bx.para
	cur, end := docBounds(bx)
	for i, k := range pa.atoms {
		if w.state == 2 {
			return
		}
		if k.parent != bx {
			continue
		}
		at := pa.boxes[pa.atomBox[i]].At
		w.seg(bx, cur, text.Cursor{Offset: at, Affinity: text.Upstream})
		w.walk(k, true, w.on(k, ci))
		cur = text.Cursor{Offset: at, Affinity: text.Downstream}
	}
	w.seg(bx, cur, end)
	for _, k := range bx.kids {
		if w.state == 2 {
			return
		}
		kci := w.on(k, ci)
		if w.state == 0 && kci < 0 {
			continue
		}
		if _, _, ok := atomSlot(bx, k); !ok && !k.portal {
			w.walk(k, false, kci) // after the text
		}
	}
}

// seg reports the part of [a, b] of paragraph bx inside the range.
func (w *rangeWalk) seg(bx *Box, a, b text.Cursor) {
	if w.state == 0 {
		if w.start.bx != bx || cmpCur(w.start.cur, a) < 0 || cmpCur(w.start.cur, b) > 0 {
			return
		}
		w.state, w.nl = 1, 0
		a = w.start.cur
	}
	if w.state != 1 {
		return
	}
	if w.end.bx == bx && cmpCur(w.end.cur, a) >= 0 && cmpCur(w.end.cur, b) <= 0 {
		w.emit(bx, a, w.end.cur, w.nl)
		w.state = 2
		return
	}
	if w.emit(bx, a, b, w.nl) {
		w.nl = 0
	}
}

// extraNewline reports a paragraph or heading whose bottom margin is at
// least half its font size: copied text has a blank line after it
// (Chromium's ShouldEmitExtraNewlineForNode).
func extraNewline(bx *Box) bool {
	switch bx.kind {
	case style.KindP, style.KindH1, style.KindH2, style.KindH3, style.KindH4, style.KindH5, style.KindH6:
	default:
		return false
	}
	if bx.el == nil || bx.cs == nil {
		return false
	}
	return 2*bx.ld.Layout().Margin.Bottom >= bx.cs.Text.FontSize
}

// ordered returns the ends of the selection in document order.
func (t *Tree) selOrdered() (start, end selPos) {
	s := &t.tsel
	if t.cmpPos(s.anchor, s.focus) <= 0 {
		return s.anchor, s.focus
	}
	return s.focus, s.anchor
}

// walkRange runs emit over the range [start, end], from the deepest box
// that holds both.
func (t *Tree) walkRange(start, end selPos, emit func(bx *Box, a, b text.Cursor, nl int) bool) {
	ca := chainOfBox(nil, start.bx)
	cb := chainOfBox(nil, end.bx)
	i, j := len(ca)-1, len(cb)-1
	for i > 0 && j > 0 && ca[i-1] == cb[j-1] {
		i, j = i-1, j-1
	}
	w := rangeWalk{start: start, end: end, emit: emit, chain: ca}
	from := ca[i] // the deepest box that holds both
	_, _, atom := atomSlot(from.parent, from)
	w.walk(from, atom, i)
}

// partOf returns what paragraph box bx paints of the range [start, end]:
// the part a walk of the whole range gives it, from a walk of its own
// subtree (where its atomic inlines' text comes).
func (t *Tree) partOf(bx *Box, start, end selPos) paraSel {
	var ps paraSel
	got := false
	w := rangeWalk{start: start, end: end, emit: func(b *Box, x, y text.Cursor, _ int) bool {
		if b == bx {
			if !got {
				ps.sel, got = text.NewSelection(x, y), true
			} else {
				ps.sel.Focus = y
			}
		}
		return true
	}}
	first, last := docBounds(bx)
	in := func(p selPos) int { // the index of bx in p's chain, or -1
		i := 0
		for b := p.bx; b != nil; b = b.parent {
			if b == bx {
				return i
			}
			i++
		}
		return -1
	}
	ci := in(start)
	switch {
	case ci >= 0: // the start is in bx's text or in what it holds
		w.chain = chainOfBox(nil, start.bx)
	case t.cmpPos(start, selPos{bx: bx, cur: first}) > 0:
		return paraSel{} // bx's subtree is before the start
	default:
		w.state = 1
	}
	if in(end) < 0 && t.cmpPos(end, selPos{bx: bx, cur: first}) < 0 {
		return paraSel{} // and after the end
	}
	w.walk(bx, true, ci)
	if !got {
		return paraSel{}
	}
	ps.on = true
	ps.more = bx != end.bx && cmpCur(ps.sel.Focus, last) == 0
	if ps.sel.IsCollapsed() && !ps.more {
		ps.on = false
	}
	return ps
}

// ---------------------------------------------------------------------
// Highlights.

// setSel replaces the selection and updates what the paragraphs paint.
func (t *Tree) setSel(root *Box, anchor, focus selPos) {
	s := &t.tsel
	s.root, s.anchor, s.focus = root, anchor, focus
	if !t.updateHighlights() {
		t.refreshHighlights()
	}
}

// clearSel removes the selection.
func (t *Tree) clearSel() {
	s := &t.tsel
	if !s.active() && len(s.hl) == 0 {
		return
	}
	s.root, s.anchor, s.focus = nil, selPos{}, selPos{}
	s.unit, s.ustart, s.uend = selChar, selPos{}, selPos{}
	s.drag, s.inSel = nil, nil
	t.refreshHighlights()
}

// refreshHighlights computes the part of the selection each paragraph
// paints and marks the paragraphs whose part changed, walking the whole
// range: for a new tree or a new selection (setSel updates the
// highlights of a range that changed with updateHighlights).
func (t *Tree) refreshHighlights() {
	s := &t.tsel
	s.gen++
	gen := s.gen
	old := s.hl
	var next []*Box
	s.touched = 0
	if s.active() && s.anchor != s.focus {
		start, end := t.selOrdered()
		t.walkRange(start, end, func(bx *Box, a, b text.Cursor, _ int) bool {
			if !selectableBox(bx) {
				return false
			}
			pa := bx.para
			if pa.selGen != gen {
				s.touched++
				pa.selGen = gen
				pa.nsel = paraSel{on: true, sel: text.NewSelection(a, b)}
				next = append(next, bx)
			} else {
				pa.nsel.sel.Focus = b
			}
			return true
		})
		for _, bx := range next {
			pa := bx.para
			_, docEnd := docBounds(bx)
			pa.nsel.more = bx != end.bx && cmpCur(pa.nsel.sel.Focus, docEnd) == 0
			if pa.nsel.sel.IsCollapsed() && !pa.nsel.more {
				pa.nsel.on = false
			}
		}
	}
	for _, bx := range next {
		if pa := bx.para; pa.hl != pa.nsel {
			pa.hl = pa.nsel
			t.paraSelChanged(bx)
		}
	}
	for _, bx := range old {
		if pa := bx.para; pa != nil && pa.selGen != gen && pa.hl.on {
			pa.hl = paraSel{}
			t.paraSelChanged(bx)
		}
	}
	clear(s.hlAt)
	clear(old)
	s.hl = old[:0]
	for _, bx := range next {
		if bx.para.hl.on {
			s.list(bx, true)
		}
	}
	s.shown = s.active() && s.anchor != s.focus
	s.shownRoot, s.hs, s.he = nil, selPos{}, selPos{}
	if s.shown {
		s.shownRoot = s.root
		s.hs, s.he = t.selOrdered()
	}
}

// updateHighlights updates the highlights after a change of the range,
// from the range they show (textSel.shown): only the paragraphs between
// the old and the new place of each end that moved can change their part,
// so only those are walked and computed again; the others keep theirs. A
// drag that extends the selection by a word computes one paragraph, not
// every paragraph of the range. It reports false, having changed nothing,
// when the highlights must be computed from the whole range
// (refreshHighlights): there is no range shown, or it is in another root,
// or one of the ends left the tree.
func (t *Tree) updateHighlights() bool {
	s := &t.tsel
	root := s.root
	if !s.shown || s.shownRoot != root || !s.active() || s.anchor == s.focus {
		return false
	}
	if !t.posValid(s.hs, root) || !t.posValid(s.he, root) || !t.posValid(s.anchor, root) || !t.posValid(s.focus, root) {
		return false
	}
	ns, ne := t.selOrdered()
	os, oe := s.hs, s.he
	s.gen++
	gen := s.gen
	touched := s.touch[:0]
	note := func(bx *Box, _, _ text.Cursor, _ int) bool {
		if pa := bx.para; pa.selGen != gen {
			pa.selGen = gen
			touched = append(touched, bx)
		}
		return true
	}
	between := func(a, b selPos) {
		if a == b {
			return
		}
		if t.cmpPos(a, b) > 0 {
			a, b = b, a
		}
		t.walkRange(a, b, note)
	}
	between(os, ns)
	between(oe, ne)
	s.touched = 0
	for _, bx := range touched {
		var ps paraSel
		if selectableBox(bx) {
			s.touched++
			ps = t.partOf(bx, ns, ne)
		}
		if pa := bx.para; pa.hl != ps {
			pa.hl = ps
			t.paraSelChanged(bx)
		}
		s.list(bx, ps.on)
	}
	clear(touched)
	s.touch = touched[:0]
	s.hs, s.he = ns, ne
	return true
}

// paraSelChanged records that what paragraph box bx paints of the
// selection changed: the layers that paint it are recorded again in the
// next frame (record.go).
func (t *Tree) paraSelChanged(bx *Box) {
	bx.para.ver++
	t.markPlace(bx)
	if bx.pcs != nil && !t.inFrame {
		p := painter{t: t, scale: float64(t.scale)}
		p.notePlaced(bx)
	}
	t.requestRepaint()
}

// repaintSel marks every highlighted paragraph changed (the window's
// focus changes the selection's colors).
func (t *Tree) repaintSel() {
	for _, bx := range t.tsel.hl {
		if bx.para != nil && bx.para.hl.on {
			t.paraSelChanged(bx)
		}
	}
}

// checkSel drops a selection whose ends left the tree or their root, and
// clamps its cursors to the text of their paragraphs; then the parts are
// computed again (a frame that changed the tree, before paint).
func (t *Tree) checkSel() {
	s := &t.tsel
	if !s.active() && len(s.hl) == 0 {
		return
	}
	if s.active() {
		if s.root == nil || t.selRoot(s.root) != s.root || !t.posValid(s.anchor, s.root) || !t.posValid(s.focus, s.root) {
			t.clearSel()
			return
		}
		s.anchor, s.focus = clampPos(s.anchor), clampPos(s.focus)
		if s.unit != selChar {
			if t.posValid(s.ustart, s.root) && t.posValid(s.uend, s.root) {
				s.ustart, s.uend = clampPos(s.ustart), clampPos(s.uend)
			} else {
				s.unit = selChar
			}
		}
	}
	t.refreshHighlights()
}

// ---------------------------------------------------------------------
// Positions under a point.

// paraPoint returns the window point p in the coordinates of the text
// layout of paragraph box bx.
func (t *Tree) paraPoint(bx *Box, p gfx.Point) gfx.Point {
	q := t.boxPoint(bx, p).Add(bx.u.Origin())
	q = toSpace(q, bx.space, contentSpace(bx))
	return q.Sub(contentRect(bx.u, bx.ld.Layout()).Origin())
}

// paraToWindow returns rectangle r of the text layout of paragraph box bx
// in window coordinates (the bounding box through transforms).
func (t *Tree) paraToWindow(bx *Box, r gfx.Rect) gfx.Rect {
	o := t.paraOrigin(bx)
	return t.transformUp(bx, r.Translate(o.X, o.Y))
}

// paraOrigin returns the origin of the text layout of paragraph box bx:
// its content box in window coordinates before transforms, scrolled.
func (t *Tree) paraOrigin(bx *Box) gfx.Point {
	l := bx.ld.Layout()
	o := t.semOrigin(bx).Add(gfx.Point{X: l.Border.Left + l.Padding.Left, Y: l.Border.Top + l.Padding.Top})
	if s := bx.scr; s != nil {
		o = o.Sub(s.soff)
	}
	return o
}

// paraTextMatrix returns the map from the text coordinates of paragraph
// box bx to window coordinates as they are painted (paraToWindow's, as
// one matrix: a rotation or a skew is kept; transformMatrix).
func (t *Tree) paraTextMatrix(bx *Box) gfx.Affine {
	return t.transformMatrix(bx, t.paraOrigin(bx))
}

// posAt returns the position for the window point p in selection root
// root (see "The pointer" at the top of textsel.go), and false when root
// has no selectable text.
func (t *Tree) posAt(root *Box, p gfx.Point) (selPos, bool) {
	t.hitTest(p)
	hb := t.in.hit.box
	t.in.hit.box, t.in.hit.onText = nil, nil
	return t.posUnder(root, hb, p)
}

// posAtPointer is posAt at the position of pointer ps, from the hit test
// that routed its last event while it holds (pointerHitAt).
func (t *Tree) posAtPointer(root *Box, ps *pointerState) (selPos, bool) {
	return t.posUnder(root, t.pointerHitAt(ps).box, ps.pos)
}

// posUnder is posAt for the box hb that a hit test at p found.
func (t *Tree) posUnder(root, hb *Box, p gfx.Point) (selPos, bool) {
	if hb == nil || t.selRoot(hb) != root {
		hb = root
	}
	return t.posIn(root, hb, p)
}

// posIn returns the position for p in the subtree of bx (Chromium's
// PositionForPoint of the box hit).
func (t *Tree) posIn(root, bx *Box, p gfx.Point) (selPos, bool) {
	for b := bx; b != nil; {
		if paraBox(b) {
			if selectableBox(b) {
				lp := t.paraPoint(b, p)
				return selPos{b, b.para.lay.HitTest(lp.X, lp.Y)}, true
			}
			return t.nearestPos(root, b)
		}
		k := t.childAt(b, p)
		if k == nil {
			return t.nearestPos(root, b)
		}
		b = k
	}
	return selPos{}, false
}

// hitCandidate reports whether child k of a box may hold the position
// of a point (Chromium's IsHitTestCandidate): displayed, visible, with a
// height, in flow, and not a portal.
func hitCandidate(k *Box) bool {
	if k.portal || k.ls.Display == layout.DisplayNone || k.cs == nil || k.pcs == nil || k.hiddenAtom || outOfFlowStyle(k.cs) {
		return false
	}
	return k.pcs.Visibility == style.VisibilityVisible && k.ld.Layout().Size.H > 0
}

// childAt returns the child of bx whose subtree holds the position of p:
// in a block container the last child whose top is at or above p, else
// the first whose bottom is below it (Chromium's
// PositionForPointInBlockFlowDirection); in a flex or grid container the
// nearest child (PositionForPointByClosestChild).
func (t *Tree) childAt(bx *Box, p gfx.Point) *Box {
	if bx.ls.Display == layout.DisplayBlock {
		var last *Box
		for i := len(bx.kids) - 1; i >= 0; i-- {
			if hitCandidate(bx.kids[i]) {
				last = bx.kids[i]
				break
			}
		}
		if last == nil {
			return nil
		}
		if t.boxPoint(last, p).Y >= 0 {
			return last
		}
		for _, k := range bx.kids {
			if hitCandidate(k) && t.boxPoint(k, p).Y < k.ld.Layout().Size.H {
				return k
			}
		}
		return last
	}
	var best *Box
	bestD := float32(0)
	for _, k := range bx.kids {
		if !hitCandidate(k) {
			continue
		}
		q := t.boxPoint(k, p)
		sz := k.ld.Layout().Size
		dx := max(0, -q.X, q.X-sz.W)
		dy := max(0, -q.Y, q.Y-sz.H)
		d := dx*dx + dy*dy
		if best != nil && d > bestD {
			continue
		}
		best, bestD = k, d
		if d == 0 {
			break
		}
	}
	return best
}

// nearestPos returns the start of the first selectable paragraph after
// bx in document order in root, else the end of the last one before it
// (Chromium's canonical position, which prefers the next candidate).
func (t *Tree) nearestPos(root, bx *Box) (selPos, bool) {
	var prev, next *Box
	passed := false
	var walk func(b *Box) bool
	walk = func(b *Box) bool {
		if b.ls.Display == layout.DisplayNone || b.cs == nil {
			return true
		}
		if b == bx {
			passed = true
		}
		if selectableBox(b) {
			if passed && b != bx {
				next = b
				return false
			}
			if !passed {
				prev = b
			}
		}
		for _, k := range b.kids {
			if k.portal && b == t.rootBox {
				continue
			}
			if !walk(k) {
				return false
			}
		}
		if b == bx {
			passed = true
		}
		return true
	}
	walk(root)
	switch {
	case next != nil:
		s, _ := docBounds(next)
		return selPos{next, s}, true
	case prev != nil:
		_, e := docBounds(prev)
		return selPos{prev, e}, true
	}
	return selPos{}, false
}

// unitAt returns the unit of selection u around position pos (a word or
// a paragraph of its text, or the position itself), with the point p
// that gave it.
func (t *Tree) unitAt(u selUnit, pos selPos, p gfx.Point) (start, end selPos) {
	if u == selChar || !paraBox(pos.bx) {
		return pos, pos
	}
	lay := pos.bx.para.lay
	lp := t.paraPoint(pos.bx, p)
	if u == selWord {
		w := lay.SelectWordAt(lp.X, lp.Y)
		return selPos{pos.bx, w.Anchor}, selPos{pos.bx, w.Focus}
	}
	ps := lay.SelectParagraphAt(lp.X, lp.Y)
	start, end = selPos{pos.bx, ps.Anchor}, selPos{pos.bx, ps.Focus}
	if _, de := docBounds(pos.bx); cmpCur(end.cur, de) == 0 {
		// The paragraph break: to the start of the next paragraph
		// (Chromium's paragraph granularity includes it).
		if n, ok := t.nextParaStart(t.tsel.root, pos.bx); ok {
			end = n
		}
	}
	return start, end
}

// nextParaStart returns the start of the first selectable paragraph
// after bx's text in document order in root.
func (t *Tree) nextParaStart(root, bx *Box) (selPos, bool) {
	if root == nil {
		return selPos{}, false
	}
	_, de := docBounds(bx)
	var found selPos
	ok := false
	w := rangeWalk{start: selPos{bx, de}, end: selPos{}, chain: chainOfBox(nil, bx)}
	w.emit = func(b *Box, a, _ text.Cursor, _ int) bool {
		if b != bx && selectableBox(b) && !ok {
			found, ok = selPos{b, a}, true
			w.state = 2
		}
		return true
	}
	ci := -1
	for i, b := range w.chain {
		if b == root {
			ci = i
			break
		}
	}
	w.walk(root, false, ci)
	return found, ok
}

// ---------------------------------------------------------------------
// Pointer.

// canStartSel reports whether a press on el may change the selection
// (see "The pointer" at the top of textsel.go); link reports a press on a
// link, which clears the selection without starting one.
func canStartSel(el *Element) (ok, link bool) {
	if el == nil {
		return false, false
	}
	var first *Element
	region := false // the nearest window region was seen
	for e := el; e != nil; e = e.styleParent() {
		if e.kind != elemPrimitive {
			continue
		}
		if first == nil {
			first = e
		}
		d := e.node.d
		switch {
		case e.box.kind == style.KindButton, e.box.kind.IsTextField(), d.draggable, d.evMask&dragMask != 0:
			return false, false
		case e.box.kind == style.KindA && d.href != "":
			link = true
		}
		if !region && d.winRegion != regionNone {
			// A title bar's drag area or a window control moves or
			// controls the window (Chromium's app-region: drag is not
			// even hit); a no-drag area inside one is content.
			if d.winRegion != regionNoDrag {
				return false, false
			}
			region = true
		}
	}
	if first == nil || first.box.cs == nil || first.box.cs.UserSelect == style.UserSelectNone {
		return false, false
	}
	return true, link
}

// selPress handles a primary press of ps on target that may select (see
// "The pointer"), after the focus moved (ps.noSelect holds what its
// pointerdown handlers said).
func (t *Tree) selPress(ps *pointerState, target *Element) {
	s := &t.tsel
	s.drag, s.inSel = nil, nil
	if ps.key.kind == platform.PointerTouch || ps.noSelect ||
		t.flavor == platform.FlavorMac && ps.mods&platform.ModCtrl != 0 {
		return // no touch selection; a Mac context click
	}
	if isTextFieldEl(target) {
		t.clearSel() // the field's own selection (focusing it cleared this)
		return
	}
	ok, link := canStartSel(target)
	if !ok {
		return
	}
	bx := t.geomBox(target)
	if bx == nil {
		return
	}
	root := t.selRoot(bx)
	if root == nil {
		return
	}
	shift := ps.mods&platform.ModShift != 0
	if link {
		if !shift {
			t.clearSel()
		}
		return
	}
	pos, found := t.posAtPointer(root, ps)
	if !found {
		t.clearSel()
		return
	}
	switch {
	case shift && s.active() && s.root == root:
		t.selExtendTo(pos, ps.pos, true)
	case ps.clickCount == 1 && t.onSel(ps.pos):
		// A press on the selection may drag it (selDragRecognizer).
		s.inSel, s.pressAt = ps, t.devicePixel(ps.pos)
		return
	case ps.clickCount == 2:
		s.unit = selWord
		s.ustart, s.uend = t.expandAll(t.unitAt(selWord, pos, ps.pos))
		t.setSel(root, s.ustart, s.uend)
	case ps.clickCount >= 3:
		s.unit = selPara
		s.root = root
		s.ustart, s.uend = t.expandAll(t.unitAt(selPara, pos, ps.pos))
		t.setSel(root, s.ustart, s.uend)
	default:
		// A caret, or the whole of a user-select: all element.
		s.unit = selChar
		a, b := t.expandAll(pos, pos)
		t.setSel(root, a, b)
	}
	s.drag = ps
}

// selExtendTo moves the focus to pos, given by the window point p, by the
// selection's unit: a drag, or a Shift press (click) that extends it as
// Chromium does (non-directionally under the Mac flavor).
func (t *Tree) selExtendTo(pos selPos, p gfx.Point, click bool) {
	s := &t.tsel
	if s.unit != selChar {
		ts, te := t.unitAt(s.unit, pos, p)
		us, ue := s.ustart, s.uend
		if click {
			// A Shift press after a double or triple click extends by the
			// unit from the current selection.
			us, ue = t.selOrdered()
		}
		if t.cmpPos(ts, us) < 0 {
			t.setSel(s.root, ue, ts)
		} else {
			f := te
			if t.cmpPos(te, ue) < 0 {
				f = ue
			}
			t.setSel(s.root, us, f)
		}
		return
	}
	anchor := s.anchor
	pos = t.allEdge(pos, t.cmpPos(pos, anchor) >= 0)
	if click && t.flavor == platform.FlavorMac && s.anchor != s.focus {
		// ExtendSelectionAsNonDirectional: the end farther from the
		// press stays.
		start, end := t.selOrdered()
		switch {
		case t.cmpPos(pos, start) < 0:
			anchor = end
		case t.cmpPos(pos, end) > 0:
			anchor = start
		default:
			ds := t.textDistance(start, pos)
			de := t.textDistance(pos, end)
			if ds <= de {
				anchor = end
			} else {
				anchor = start
			}
		}
	}
	t.setSel(s.root, anchor, pos)
}

// textDistance returns the length of the text between positions a and b
// (a before b), for the non-directional Shift press.
func (t *Tree) textDistance(a, b selPos) int {
	n := 0
	t.walkRange(a, b, func(bx *Box, x, y text.Cursor, _ int) bool {
		n += max(y.Offset-x.Offset, 0)
		return true
	})
	return n
}

// selPointerMove extends the selection dragged by ps to the pointer.
func (t *Tree) selPointerMove(ps *pointerState) {
	s := &t.tsel
	if s.drag != ps {
		return
	}
	if !ps.pressing || !s.active() {
		s.drag = nil
		return
	}
	t.selDragTo(ps)
	if t.selAutoscrolling() {
		t.requestRepaint()
	}
}

// selDragTo extends the selection to the position of pointer ps.
func (t *Tree) selDragTo(ps *pointerState) {
	s := &t.tsel
	if !t.posValid(s.anchor, s.root) {
		t.clearSel()
		return
	}
	if pos, ok := t.posAtPointer(s.root, ps); ok {
		t.selExtendTo(pos, ps.pos, false)
	}
}

// selPointerUp ends the press of ps: a release (cancel false) where a
// press inside the selection was made, with no drag, clears it. Where is
// the device pixel, as Chromium compares the release with the press
// (SelectionController::HandleMouseReleaseEvent, its event positions
// floored to whole pixels of the root frame, which are device pixels).
func (t *Tree) selPointerUp(ps *pointerState, cancel bool) {
	s := &t.tsel
	if s.drag == ps {
		s.drag = nil
	}
	if s.inSel == ps {
		s.inSel = nil
		if !cancel && t.devicePixel(ps.pos) == s.pressAt && s.active() {
			t.clearSel()
		}
	}
}

// devicePixel returns the device pixel that holds window point p:
// Chromium's gfx::ToFlooredPoint of an event's position.
func (t *Tree) devicePixel(p gfx.Point) image.Point {
	k := float64(t.scale)
	return image.Point{X: int(math.Floor(float64(p.X) * k)), Y: int(math.Floor(float64(p.Y) * k))}
}

// selDragWon is called when a drag gesture of an element won the press
// of ps: the selection the press made collapses at its anchor.
func (t *Tree) selDragWon(ps *pointerState) {
	s := &t.tsel
	if s.drag == ps {
		s.drag = nil
		if s.active() {
			t.setSel(s.root, s.anchor, s.anchor)
		}
	}
	if s.inSel == ps {
		s.inSel = nil
	}
}

// onSel reports whether the window point p is on the highlight of
// the selection.
func (t *Tree) onSel(p gfx.Point) bool {
	for _, bx := range t.tsel.hl {
		pa := bx.para
		if pa == nil || !pa.hl.on || pa.hl.sel.IsCollapsed() {
			continue
		}
		lp := t.paraPoint(bx, p)
		for _, r := range pa.lay.HighlightRects(pa.hl.sel, false, nil) {
			if r.Contains(lp) {
				return true
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------
// Autoscroll.

// selAutoscroll scrolls the scroll containers around the dragged
// selection's anchor so that the pointer comes into view, as Chromium's
// LayoutBox::Autoscroll scrolls a 1 px square at the pointer into view
// (align to edge if needed), innermost first; with move false it only
// reports whether one would move.
func (t *Tree) selAutoscroll(move bool) bool {
	s := &t.tsel
	ps := s.drag
	if ps == nil || !ps.pressing || !s.active() || !t.inTree(s.anchor.bx) {
		return false
	}
	r := gfx.Rect{X0: ps.pos.X, Y0: ps.pos.Y, X1: ps.pos.X + 1, Y1: ps.pos.Y + 1}
	moved := false
	for b := s.anchor.bx; ; {
		cb := containingBlock(b)
		if cb == nil || fixedToWindow(b, cb) {
			break
		}
		if sc := cb.scr; sc != nil && (sc.userX || sc.userY) {
			c := t.clientRect(cb)
			d := gfx.Point{X: nearest(r.X0, r.X1, c.X0, c.X1), Y: nearest(r.Y0, r.Y1, c.Y0, c.Y1)}
			if !sc.userX {
				d.X = 0
			}
			if !sc.userY {
				d.Y = 0
			}
			from := sc.pos
			to := sc.clamp(from.Add(sc.flip(d)))
			if to != from {
				if !move {
					return true
				}
				t.scrollDirect(sc, to)
				moved = true
				m := sc.flip(to.Sub(from))
				r = r.Translate(-m.X, -m.Y)
			}
		}
		b = cb
	}
	return moved
}

// selAutoscrolling reports whether a selection drag scrolls (frames go
// on).
func (t *Tree) selAutoscrolling() bool { return t.selAutoscroll(false) }

// animateSel runs one autoscroll step of a selection drag (Animate), then
// extends the selection to the text that came under the pointer.
func (t *Tree) animateSel() {
	if t.selAutoscroll(true) {
		t.selDragTo(t.tsel.drag)
	}
}

// ---------------------------------------------------------------------
// Text and keys.

// selectedText returns the text of the selection as Chromium copies it
// (TextIterator): the rendered text of each selectable paragraph's part,
// a line break between the parts of different blocks and two after a
// paragraph or heading with a bottom margin of at least half its font
// size, a break where the range starts at the end of a block before
// another, and none after the last part; atomic inlines are inline.
func (t *Tree) selectedText() string {
	s := &t.tsel
	if !s.active() || s.anchor == s.focus || !t.posValid(s.anchor, s.root) || !t.posValid(s.focus, s.root) {
		return ""
	}
	start, end := t.selOrdered()
	var sb strings.Builder
	t.walkRange(start, end, func(bx *Box, a, b text.Cursor, nl int) bool {
		if !selectableBox(bx) {
			return false
		}
		txt := bx.para.lay.RenderedText(text.NewSelection(a, b))
		if txt == "" {
			nl = min(nl, 1)
		}
		for range nl {
			sb.WriteByte('\n')
		}
		sb.WriteString(txt)
		return true
	})
	return sb.String()
}

// SelectedText returns the text of the window's selection of static text
// as copying it would (Chromium's getSelection().toString()), "" when
// nothing is selected. A focused text field's selection is its own
// (ElementRef.SelectionRange). For telatest. Call it on the UI thread.
func (t *Tree) SelectedText() string { return t.selectedText() }

// selectAll selects all the text of a root: the focused element's, else
// the selection's, else the window's content.
func (t *Tree) selectAll() {
	var root *Box
	if el := t.foc.el; el != nil && el.mounted {
		if bx := t.geomBox(el); bx != nil {
			root = t.selRoot(bx)
		}
	}
	if root == nil && t.tsel.active() {
		root = t.tsel.root
	}
	if root == nil {
		root = t.rootBox
	}
	if root == nil {
		return
	}
	if a, b, ok := t.selBounds(root); ok {
		t.tsel.unit = selChar
		t.setSel(root, a, b)
	}
}

// selBounds returns the first and last positions of the selectable text
// in the subtree of bx, in document order: the walk goes down before it
// goes on, so the first selectable paragraph starts first, but the text
// of a paragraph after an atomic inline comes after the atom's.
func (t *Tree) selBounds(bx *Box) (a, b selPos, ok bool) {
	var walk func(bx *Box)
	walk = func(bx *Box) {
		if bx.ls.Display == layout.DisplayNone || bx.cs == nil {
			return
		}
		if selectableBox(bx) {
			s, e := docBounds(bx)
			if a.bx == nil {
				a = selPos{bx, s}
			}
			if end := (selPos{bx, e}); b.bx == nil || t.cmpPos(end, b) > 0 {
				b = end
			}
		}
		for _, k := range bx.kids {
			if !(k.portal && bx == t.rootBox) {
				walk(k)
			}
		}
	}
	walk(bx)
	return a, b, a.bx != nil
}

// allRange returns the first and last positions of the outermost element
// with user-select: all that holds the text at position p, whose content
// is selected as a whole (Chromium's
// ExpandSelectionToRespectUserSelectAll), and whether there is one.
func (t *Tree) allRange(p selPos) (a, b selPos, ok bool) {
	var all *Element
	for x := selPosElement(p); x != nil; x = x.styleParent() {
		if x.kind != elemPrimitive {
			continue
		}
		if x.box.cs == nil || x.box.cs.UserSelect != style.UserSelectAll {
			break
		}
		all = x
	}
	if all == nil {
		return selPos{}, selPos{}, false
	}
	if pb := all.box.inPara; pb != nil && paraBox(pb) {
		// An inline element: its text in its paragraph.
		for i, el := range pb.para.inl {
			if el == all && i < len(pb.para.inlines) {
				in := pb.para.inlines[i]
				lay := pb.para.lay
				return selPos{pb, lay.SnapCursor(text.Cursor{Offset: in.Start, Affinity: text.Upstream})},
					selPos{pb, lay.SnapCursor(text.Cursor{Offset: in.End, Affinity: text.Downstream})}, true
			}
		}
		return selPos{}, selPos{}, false
	}
	if bx := t.geomBox(all); bx != nil {
		return t.selBounds(bx)
	}
	return selPos{}, selPos{}, false
}

// allEdge moves position p, the focus of a selection, to the edge of the
// user-select: all element that holds it in the direction the selection
// grows (its end when forward).
func (t *Tree) allEdge(p selPos, forward bool) selPos {
	if a, b, ok := t.allRange(p); ok {
		if forward {
			return b
		}
		return a
	}
	return p
}

// expandAll grows the selection [a, b] (in document order) to the whole
// of the user-select: all elements that hold its ends.
func (t *Tree) expandAll(a, b selPos) (selPos, selPos) {
	if s, _, ok := t.allRange(a); ok && t.cmpPos(s, a) < 0 {
		a = s
	}
	if _, e, ok := t.allRange(b); ok && t.cmpPos(e, b) > 0 {
		b = e
	}
	return a, b
}

// selKey runs the selection's key bindings for keydown e, with the focus
// outside text fields; it reports whether e was one.
func (t *Tree) selKey(e KeyboardEvent) bool {
	var copyKey, all bool
	if t.flavor == platform.FlavorMac {
		if e.Meta && !e.Ctrl && !e.Alt && !e.Shift {
			switch shortcutLetter(e) {
			case "c":
				copyKey = true
			case "a":
				all = true
			}
		}
	} else if e.Ctrl && !e.Alt && !e.Meta && !e.Shift {
		switch {
		case shortcutLetter(e) == "c", e.Key == KeyValue(platform.KeyInsert):
			copyKey = true
		case shortcutLetter(e) == "a":
			all = true
		}
	}
	switch {
	case copyKey:
		if txt := t.selectedText(); txt != "" {
			t.writeClipboard(txt)
		}
		return true
	case all:
		t.selectAll()
		return true
	}
	return false
}

// ---------------------------------------------------------------------
// Painting.

// selHighlight returns the highlight rectangles of paragraph box bx, with
// their lines, and the selection's colors (fieldpaint.go's
// selectionColors: a nil text paint keeps the text's own colors); none
// when the paragraph paints no part of the selection. When layered (the
// paragraph paints its positioned inlines in their own layers, inline.go)
// they are those of the text of paint layer layer only
// (text.Layout.HighlightLineRects).
func (p *painter) selHighlight(bx *Box, layered bool, layer int) (rects []text.LineRect, bg gfx.Color, fg gfx.Paint) {
	pa := bx.para
	if !pa.hl.on || bx.pcs == nil || bx.pcs.Visibility != style.VisibilityVisible {
		return nil, gfx.Color{}, nil
	}
	bg, fg = p.t.selectionColors(bx.pcs)
	if !layered {
		layer = text.AllLayers
	}
	return pa.lay.HighlightLineRects(pa.hl.sel, pa.hl.more, layer, nil), bg, fg
}

// paintSelBg paints the highlight of the rectangles of line li (all of
// them when li < 0) of a paragraph whose content box is at origin,
// snapped like box edges (as fields snap theirs).
func (p *painter) paintSelBg(rects []text.LineRect, origin gfx.Point, bg gfx.Color, li int) {
	for _, sr := range rects {
		if li < 0 || sr.Line == li {
			p.b.FillRect(p.snapRect(sr.Rect.Translate(origin.X, origin.Y)), bg)
		}
	}
}

// paintSelText paints the selected text again in the selection's text
// color fg, clipped to each rectangle of line li (all lines when li < 0);
// when layered, only the text of paint layer layer
// (text.Layout.PaintLineLayerWith).
func (p *painter) paintSelText(lay *text.Layout, rects []text.LineRect, origin gfx.Point, fg gfx.Paint, li int, layered bool, layer int) {
	if fg == nil {
		return
	}
	for _, sr := range rects {
		if li >= 0 && sr.Line != li {
			continue
		}
		p.b.PushClipRect(p.snapRect(sr.Rect.Translate(origin.X, origin.Y)))
		if layered {
			lay.PaintLineLayerWith(p.b, origin, sr.Line, float32(p.scale), fg, layer)
		} else {
			lay.PaintLinesWith(p.b, origin, sr.Line, sr.Line+1, float32(p.scale), fg)
		}
		p.b.PopClip()
	}
}
