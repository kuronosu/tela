//go:build teladev

package core

import (
	"fmt"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/text/edit"
)

// Seeding a tree from a snapshot (ADR-113).
//
// The saved nodes form a tree (seedNode) that mirrors the Elements of the
// previous build. An Element finds its node through its parent's
// (nodeFor): the node of the same step (devStep), not yet taken by another
// Element. A hook slot is seeded when its component's first build creates
// it (devSeed): if every signature entry up to it equals the saved one,
// its saved value replaces the initial one (a UseSpring or UseTween slot
// starts at its target instead). After the first build the whole
// signature is compared (devFirstBuild): a component whose hooks changed
// (one added, removed, moved, or of another type) drops its node, so that
// nothing in its subtree is seeded, and when some of its slots were seeded
// it builds again from scratch before its children mount and before its
// effects are queued: the hooks of the discarded build run twice (a UseOnce
// or UseMemo initializer included, but no effect).
//
// Engine state follows the Elements: text fields and the focus right after
// the first Mount (devMounted), before the first style phase, so that the
// first frame shows them; scroll offsets in the first frames, after layout
// (devFrame), when the extents are known. Elements that mount in the first
// devSeedFrames frames (the items of a VirtualList at a restored offset)
// are seeded too; then the snapshot is dropped.

// devRestore is a snapshot being restored into a tree.
type devRestore struct {
	t     *Tree
	snap  *devSnapshot
	root  *seedNode
	cache map[*Element]*seedNode
	rep   *DevSeedReport
	// frames counts the frames since the first mount.
	frames int
	// left counts the saved components no Element took yet and the engine
	// records not applied yet: the seed ends early when it reaches 0.
	left int
	// focusType and focusKey are the focus-visible heuristic's state.
	focusType focusType
	focusKey  bool
}

// seedNode is a saved node being restored.
type seedNode struct {
	rec    *devNode
	parent *seedNode
	kids   map[string]*seedNode
	// el is the Element that took the node.
	el *Element
	// dropped: a component whose signature changed (nothing below it is
	// seeded); mismatch: its signature differs at a slot already created.
	dropped, mismatch bool
	// verified is how many signature entries were found equal; seeded how
	// many slots took saved state (motion slots included), restored how
	// many took a saved value.
	verified, seeded, restored int
	slots                      map[int][]byte
	// pend counts the engine records pending in the node and below;
	// field, fieldX (an Input's own scroll), scroll and focus are the
	// node's own.
	pend                         int
	field, fieldX, scroll, focus bool
}

func newDevSeed(t *Tree, snap *devSnapshot) *devRestore {
	s := &devRestore{t: t, snap: snap, cache: map[*Element]*seedNode{}, rep: &t.devState().seedRep,
		focusType: focusType(snap.FocusType), focusKey: snap.FocusKey}
	s.root = s.build(&snap.Root, nil)
	return s
}

// build makes the seed node of rec and its subtree.
func (s *devRestore) build(rec *devNode, parent *seedNode) *seedNode {
	n := &seedNode{rec: rec, parent: parent, field: rec.Field != nil, fieldX: rec.Field != nil && rec.FieldX != 0,
		scroll: rec.Scroll != nil, focus: rec.Focus}
	for _, on := range [...]bool{n.field, n.fieldX, n.scroll, n.focus} {
		if on {
			n.pend++
		}
	}
	s.left += n.pend
	if rec.Comp {
		s.left++
	}
	if len(rec.Slots) > 0 {
		n.slots = make(map[int][]byte, len(rec.Slots))
		for _, sl := range rec.Slots {
			n.slots[sl.Index] = sl.Data
		}
	}
	if len(rec.Kids) > 0 {
		n.kids = make(map[string]*seedNode, len(rec.Kids))
		for i := range rec.Kids {
			k := &rec.Kids[i]
			key := s.snap.Idents[k.Ident] + "@" + k.ID
			if _, dup := n.kids[key]; dup {
				continue // duplicate keys: the first one
			}
			c := s.build(k, n)
			n.kids[key] = c
			n.pend += c.pend
		}
	}
	return n
}

// devSeedNow returns the seed being restored into t, or nil.
func (t *Tree) devSeedNow() *devRestore {
	if d := t.devState(); d != nil {
		return d.seed
	}
	return nil
}

// nodeFor returns the seed node of el: the node of its step under its
// parent's node, when no other Element took it. The answer is cached per
// Element for the seed's life (an Element keeps its parent; an unkeyed
// child that moves keeps the node of the step it mounted at).
func (s *devRestore) nodeFor(el *Element) *seedNode {
	if n, ok := s.cache[el]; ok {
		return n
	}
	var n *seedNode
	switch {
	case el.parent == nil:
		if el == s.t.root {
			n = s.root
			n.el = el
		}
	default:
		if p := s.nodeFor(el.parent); p != nil && !p.dropped && p.kids != nil {
			if c := p.kids[devStep(el)]; c != nil && s.claim(c, el) {
				n = c
			}
		}
	}
	s.cache[el] = n
	return n
}

// claim gives node n to el, unless another Element has it.
func (s *devRestore) claim(n *seedNode, el *Element) bool {
	if n.el == el {
		return true
	}
	if n.el != nil {
		return false
	}
	n.el = el
	if n.rec.Comp {
		s.left--
	}
	return true
}

// verify reports whether the signature entries of el's slots up to i are
// the saved ones.
func (n *seedNode) verify(el *Element, i int, snap *devSnapshot) bool {
	sig := n.rec.Sig
	for j := n.verified; j <= i; j++ {
		if j >= len(sig) || j >= len(el.hooks) || devSlotSig(el, j) != snap.Idents[sig[j]] {
			return false
		}
	}
	n.verified = max(n.verified, i+1)
	return true
}

// devSeed seeds hook slot i of el, just created by its component's first
// build, from the snapshot being restored: it reports whether the slot
// matched a saved one (and took its value, or for a motion slot, starts at
// its target).
func devSeed(el *Element, i int) bool {
	t := el.tree
	s := t.devSeedNow()
	if s == nil {
		return false
	}
	n := s.nodeFor(el)
	if n == nil || !n.rec.Comp || n.dropped || n.mismatch {
		return false
	}
	if !n.verify(el, i, s.snap) {
		n.mismatch = true
		return false
	}
	h := el.hooks[i]
	if _, ok := h.(devMotionSlot); ok {
		n.seeded++
		return true
	}
	k, ok := h.(devKept)
	if !ok {
		return false
	}
	data, ok := n.slots[i]
	if !ok {
		return false // not kept by the capture: the slot starts fresh
	}
	if err := k.devDecode(data); err != nil {
		if t.dev {
			kind := devSlotKind(el, i)
			t.warnOnce("devstate-decode:"+devPath(el)+"#"+fmt.Sprint(i), func() string {
				return fmt.Sprintf("tela: %s cannot be restored (%v); it starts fresh", devSlotTitle(el, i, kind), err)
			})
		}
		return false
	}
	n.seeded++
	n.restored++
	s.rep.Restored++
	return true
}

// devFirstBuild ends the first build of component el, whose function
// returned out: when its hook signature is not the saved one, its node is
// dropped (its subtree mounts fresh) and, if some of its slots were
// seeded, it builds again from scratch, returning that build's result.
func (t *Tree) devFirstBuild(el *Element, out Node) Node {
	s := t.devSeedNow()
	if s == nil {
		return out
	}
	n := s.nodeFor(el)
	if n == nil || !n.rec.Comp || n.dropped {
		return out
	}
	if !n.mismatch && len(el.hooks) == len(n.rec.Sig) && n.verify(el, len(el.hooks)-1, s.snap) {
		return out
	}
	n.dropped = true
	s.rep.Remounted = append(s.rep.Remounted, devCompName(el.comp)+": "+s.sigDiff(n, el))
	if n.seeded == 0 {
		return out // nothing was seeded: this build was a fresh one
	}
	s.rep.Restored -= n.restored
	n.seeded, n.restored = 0, 0
	for _, h := range el.hooks {
		if u, ok := h.(unmounter); ok {
			u.unmount(t)
		}
	}
	clear(el.hooks)
	el.hooks = el.hooks[:0]
	el.hookDev = nil
	clear(el.fx)
	el.fx = el.fx[:0]
	el.hookIdx = 0
	return el.comp.build()
}

// sigDiff describes how the signature of el differs from n's.
func (s *devRestore) sigDiff(n *seedNode, el *Element) string {
	saved := n.rec.Sig
	for j := 0; j < max(len(saved), len(el.hooks)); j++ {
		var was, is string
		if j < len(saved) {
			was = s.snap.Idents[saved[j]]
		}
		if j < len(el.hooks) {
			is = devSlotSig(el, j)
		}
		switch {
		case was == is:
			continue
		case was == "":
			return fmt.Sprintf("hook %d, %s, is new", j+1, is)
		case is == "":
			return fmt.Sprintf("hook %d, %s, is gone", j+1, was)
		}
		return fmt.Sprintf("hook %d was %s, is %s", j+1, was, is)
	}
	return "its hooks changed"
}

// ---------------------------------------------------------------------
// Engine state.

// devMounted restores, after the first Mount, the text fields and the
// focus of the mounted elements, before the first style phase.
func (t *Tree) devMounted() {
	s := t.devSeedNow()
	if s == nil {
		return
	}
	s.applyEngine(false)
	if s.left == 0 {
		t.devEndSeed()
	}
}

// devFrame runs in the first frames after the scroll phase: it restores
// the scroll offsets of the scroll containers (their extents are known
// now) and the engine state of the elements mounted since, settling the
// virtual lists that moved; after devSeedFrames frames, or when everything
// saved was found, the snapshot is dropped. It reports whether the frame
// was laid out.
func (t *Tree) devFrame(full bool) bool {
	s := t.devSeedNow()
	if s == nil {
		return full
	}
	if s.applyEngine(true) && len(t.virt.lists) > 0 && t.settleScroll(1, false) {
		full = true
	}
	s.frames++
	if s.left == 0 || s.frames >= devSeedFrames {
		t.devEndSeed()
	}
	return full
}

// devEndSeed drops the snapshot being restored.
func (t *Tree) devEndSeed() {
	d := t.devState()
	if d == nil || d.seed == nil {
		return
	}
	d.seedRep.Unused = d.seed.unused()
	d.seed = nil
}

// unused counts the saved components with slots that no Element took.
func (s *devRestore) unused() int {
	var walk func(n *seedNode) int
	walk = func(n *seedNode) int {
		c := 0
		if n.rec.Comp && len(n.slots) > 0 && n.el == nil {
			c++
		}
		for _, k := range n.kids {
			c += walk(k)
		}
		return c
	}
	return walk(s.root)
}

// applyEngine restores the pending engine records of the mounted elements:
// text fields and the focus before the first frame (inFrame false), scroll
// offsets (and the rest, for elements mounted since) in a frame after
// layout. It reports whether a scroll offset moved.
func (s *devRestore) applyEngine(inFrame bool) bool {
	if s.root.pend == 0 || s.t.root == nil {
		return false
	}
	moved := false
	s.walkEngine(s.t.root, s.nodeFor(s.t.root), inFrame, &moved)
	return moved
}

func (s *devRestore) walkEngine(el *Element, n *seedNode, inFrame bool, moved *bool) {
	if n == nil || n.pend == 0 {
		return
	}
	if el.kind == elemPrimitive {
		s.applyRecords(el, n, inFrame, moved)
	}
	if n.pend == 0 || n.kids == nil {
		return
	}
	for _, c := range el.children {
		if c.kind == elemText || c.exiting() {
			continue
		}
		if cn := s.nodeFor(c); cn != nil && cn.pend > 0 {
			s.walkEngine(c, cn, inFrame, moved)
		}
	}
}

// applyRecords restores the engine records of node n on primitive el:
// the editor of a text field and the focus as soon as el is found (before
// the first style phase for the elements of the first Mount), an Input's
// own scroll and a scroll container's offset in a frame, after layout.
func (s *devRestore) applyRecords(el *Element, n *seedNode, inFrame bool, moved *bool) {
	t := s.t
	if n.field {
		if f := el.box.fld; f != nil && t.devRestoreField(f, n.rec.Field) {
			s.rep.Fields++
		}
		s.done(n, &n.field)
	}
	if n.focus {
		// Before the first style phase only what the props tell is known;
		// the frame's focus fix-up blurs the element if its style hides it.
		ok := focusTarget(el) && !el.inert && el.outside == outsideNo
		if inFrame {
			ok = t.focusable(el)
		}
		if ok && t.foc.el == nil {
			t.devFocus(el, s.focusType, s.focusKey)
			s.rep.Focus = true
		}
		s.done(n, &n.focus)
	}
	if !inFrame {
		return
	}
	if n.fieldX {
		if f := el.box.fld; f != nil && !f.multi && f.bx.cs != nil && devFinite(n.rec.FieldX) {
			lo, hi := f.scrollXRange()
			f.scrollX = min(max(n.rec.FieldX, lo), hi)
			f.markPaint()
			f.reveal()
		}
		s.done(n, &n.fieldX)
	}
	if n.scroll {
		sc := el.box.scr
		if sc == nil || !devFinite(n.rec.Scroll.X) || !devFinite(n.rec.Scroll.Y) {
			s.done(n, &n.scroll) // no longer a scroll container (or a damaged offset)
			return
		}
		want := gfx.Point{X: n.rec.Scroll.X, Y: n.rec.Scroll.Y}
		if sc.pos != want {
			t.scrollDirect(sc, want)
			*moved = true
		}
		// A content that is still shorter (the items of a list that loads)
		// is tried again in the next seed frames.
		if sc.pos == want || s.frames+1 >= devSeedFrames {
			if sc.pos != (gfx.Point{}) {
				s.rep.Scrolls++
			}
			s.done(n, &n.scroll)
		}
	}
}

// devFocus gives el the focus as the previous build had it: the state of
// the focus-visible heuristic, the focus states and a text field's caret,
// without focus events and without scrolling it into view (the restored
// offsets show it where it was): the focus did not move, the app was
// rebuilt around it, as with Fast Refresh. A pending AutoFocus is
// dropped. The window's focus comes from the platform: OnFocus fires when
// the window gains it.
func (t *Tree) devFocus(el *Element, typ focusType, hadKey bool) {
	f := &t.foc
	f.lastType, f.hadKey = typ, hadKey
	f.gen++
	f.el, f.start, f.auto = el, nil, nil
	f.visible = f.lastType != focusMouse || f.hadKey || isTextFieldEl(el)
	t.syncFocus()
	if fld := textField(el); fld != nil {
		t.caretReset(fld)
	}
}

// devFinite reports whether x is a finite number.
func devFinite(x float32) bool { return !math.IsNaN(float64(x)) && !math.IsInf(float64(x), 0) }

// done marks the record *flag of n applied.
func (s *devRestore) done(n *seedNode, flag *bool) {
	*flag = false
	for p := n; p != nil; p = p.parent {
		p.pend--
	}
	s.left--
}

// devRestoreField restores the editor state st into text field f: always
// for an uncontrolled field, and for a controlled or bound one only when
// it shows the saved value (its selection and history then still apply).
// It reports whether it restored.
func (t *Tree) devRestoreField(f *field, st *edit.DevState) bool {
	fp := f.props()
	if (fp.hasValue || fp.bind != nil) && f.ed.Value() != st.Value {
		return false
	}
	composing := f.ed.Composing()
	r := f.ed.RestoreDevState(*st)
	if composing {
		t.ime.reset = true
	}
	f.edited = len(st.Undo) > 0
	f.raw = f.ed.RawValue()
	t.fieldChanged(f, r)
	t.fieldStates(f)
	f.selStart, f.selEnd, _ = f.ed.SelectionRange()
	return true
}
