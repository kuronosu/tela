package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// Incremental semantics (ADR-277, amending ADR-246's full rebuilds).
//
// # Marks
//
// Every input of an element's semantics node marks the element when it
// changes (semMark): its node after a build (props), its children (the
// reconciliation, a Presence's exit), its text, its style (display,
// visibility; position and what makes it a containing block also move the
// subtree), its interaction states (disabled, invalid), inertness and the
// modal overlay's outside, its text field (value, selection, display),
// its scroll container's scrollability, and the Values its AriaLabelFn,
// AriaValueNowFn and AriaValueTextFn read. Geometry marks the whole
// subtree of a box whose bounds may have moved (semGeom): a layout result
// that changed (layout.ChangeObserver), a paint-phase transform or
// position, a scroll offset, a sticky or floating shift, a box's scroll
// space, a paragraph laid out again. The focus and the viewport are
// compared at the start of a build. A mark sets semKids on the ancestors
// (up to the tree's root), so that the build finds the marked elements
// from the root: an element with flags set has all its ancestors marked
// semKids, until the build visits it.
//
// # Build
//
// An element that is not marked, whose inherited state (semInherit) is
// the one of its last build, gives the node it gave then, with its whole
// subtree: the new tree shares it with the last one. The others are built
// again; a node built again that equals its old one (same values, same
// children) is replaced by the old one, so that an unchanged subtree keeps
// its pointer across trees and consumers (the OS accessibility bridges,
// access.go) compare subtrees by pointer. The ancestors of a marked
// element are built again too: a name computed from content, the bounds
// of an element without a box of its own and the set positions of a set
// container (semantics.NumberSet, run when a container is built, with
// copies of the shared nodes it renumbers) depend on the descendants.
//
// Names, descriptions and references that read elements outside the
// subtree (AriaLabelledBy, AriaDescribedBy, the labels of a field or a
// button, aria-controls and -activedescendant) make the element
// volatile: it records what it read (semDep), and when one of those was
// built again in the pass (or unmounted, or a ref moved, or an ID or a
// Label's For changed anywhere), a second pass builds it again.
//
// # Check
//
// TELA_CHECK_SEMANTICS=1 (on in core's tests) builds a full tree after
// every Semantics call, with the set positions of FillSetPositions, and
// panics on the first node that differs from the incremental tree.

// envCheckSemantics turns on the check of incremental semantics trees.
const envCheckSemantics = "TELA_CHECK_SEMANTICS"

// checkSemantics reports TELA_CHECK_SEMANTICS=1 (read once; core's tests
// turn it on).
var checkSemantics = os.Getenv(envCheckSemantics) == "1"

// Semantics flags of an Element (semFlags).
const (
	semSelf uint8 = 1 << iota // its own node may have changed
	semKids                   // a descendant is marked
	semGeom                   // the bounds of its subtree may have changed: build all of it
)

// semElemState is the incremental semantics state of an Element.
type semElemState struct {
	// node is the node the element gave in its last build (nil for none:
	// an empty text), in the tree as last built; in is the inherited
	// state it was built with.
	node *semantics.Node
	in   semInherit
	// gen is the pass that made node (0: never built), seen the last pass
	// that built the element (even when it kept its old node).
	gen, seen uint64
	flags     uint8
}

// semDep is what a volatile element's semantics read outside its subtree:
// an element (built again, or unmounted: the element is built again too),
// or a ref and the element it was attached to.
type semDep struct {
	ref *elemRef
	el  *Element
}

// semVolatile is the record of a volatile element.
type semVolatile struct {
	deps []semDep
	// labels: it reads the labels of an ID or a Label ancestor, which any
	// change of the IDs and Labels' For may change.
	labels bool
}

// semMark marks el's semantics with flags f (see the top of the file).
// Nothing is marked before a tree is built: the first build is a full one.
func (t *Tree) semMark(el *Element, f uint8) {
	if el == nil || el.tree != t || t.sem.root == nil {
		return
	}
	t.sem.pending = true
	if el.sem.flags&f == f {
		return // its ancestors are marked already
	}
	el.sem.flags |= f
	for a := el.parent; a != nil && a.tree == t && a.sem.flags&semKids == 0; a = a.parent {
		a.sem.flags |= semKids
	}
}

// semGeomBox marks the semantics bounds of bx's subtree stale: the
// subtree of its owner (the root for a box without one).
func (t *Tree) semGeomBox(bx *Box) {
	if t.sem.root == nil {
		return
	}
	el := bx.owner
	if el == nil || bx == t.rootBox {
		el = t.root
	}
	if el.mounted {
		t.semMark(el, semGeom)
	}
}

// LayoutChanged implements layout.ChangeObserver: the box's bounds, and
// its subtree's, may have moved.
func (b *Box) LayoutChanged() { b.tree.semGeomBox(b) }

// semStyled notes the computed style cs that replaces old on bx (nil when
// it had none): the display and the visibility are semantics, the
// position and what makes a box a containing block move its subtree.
func (t *Tree) semStyled(bx *Box, old, cs *style.ComputedStyle) {
	if t.sem.root == nil || bx.el == nil || old == cs {
		return
	}
	if old == nil || cs == nil || old.Display != cs.Display || old.Visibility != cs.Visibility {
		t.semMark(bx.el, semSelf)
	}
	if old == nil || cs == nil || old.Position != cs.Position || containsByStyle(old) != containsByStyle(cs) || old.Transform != cs.Transform {
		t.semGeomBox(bx)
	}
}

// semPainted notes the paint-phase style of bx after a place or a
// repaint, old being the one before: its transform and position move the
// bounds of its subtree.
func (t *Tree) semPainted(bx *Box, old *style.ComputedStyle) {
	cs := bx.pcs
	if t.sem.root == nil || old == cs {
		return
	}
	if old == nil || cs == nil || old.Transform != cs.Transform || old.Position != cs.Position {
		t.semGeomBox(bx)
	}
}

// semScrollable notes the extents or axes of scroll container s: its
// node's scrollable state follows them.
func (t *Tree) semScrollable(s *scroller) {
	on := s.userX && s.max.X > 0 || s.userY && s.max.Y > 0
	if on == s.semScroll {
		return
	}
	s.semScroll = on
	if el := s.box.el; el != nil {
		t.semMark(el, semSelf)
	}
}

// semLabelsChanged notes a change of the IDs or of a Label's For: the
// elements named by their labels are built again.
func (t *Tree) semLabelsChanged() {
	if t.sem.root != nil {
		t.sem.labels = true
		t.sem.pending = true
	}
}

// semUnmount forgets the semantics of el, unmounted.
func (t *Tree) semUnmount(el *Element) {
	s := &t.sem
	if el.semID != 0 && s.byID[semantics.NodeID(el.semID)] == el {
		delete(s.byID, semantics.NodeID(el.semID))
	}
	if s.volatile != nil {
		delete(s.volatile, el)
	}
}

// semPending reports whether something changed since the tree was last
// built.
func (t *Tree) semPending() bool {
	s := &t.sem
	return s.root == nil || s.pending || s.focus != t.foc.el || s.viewport != t.viewport || s.rootEl != t.root
}

// buildSemantics builds the semantics tree: in full when none was built,
// else again from the marks.
func (t *Tree) buildSemantics() {
	s := &t.sem
	if s.byID == nil {
		s.byID = make(map[semantics.NodeID]*Element)
	}
	clear(s.fns)
	s.told = false
	s.reuse = s.root != nil
	if s.reuse && s.focus != t.foc.el {
		if f := s.focus; f != nil && f.mounted {
			t.semMark(f, semSelf)
		}
		t.semMark(t.foc.el, semSelf)
	}
	first := s.gen + 1
	t.semPass()
	if s.reuse && len(s.volatile) > 0 {
		again := false
		for el, v := range s.volatile {
			if !el.mounted {
				delete(s.volatile, el)
				continue
			}
			if el.sem.seen != first && (s.labels && v.labels || semDepsChanged(v.deps, first)) {
				t.semMark(el, semSelf)
				again = true
			}
		}
		if again {
			t.semPass()
		}
	}
	s.pending, s.labels = false, false
	s.focus, s.viewport, s.rootEl = t.foc.el, t.viewport, t.root
}

// semDepsChanged reports whether a volatile element's deps changed in the
// pass gen.
func semDepsChanged(deps []semDep, gen uint64) bool {
	for _, d := range deps {
		if d.ref != nil && d.ref.el != d.el {
			return true
		}
		if e := d.el; e != nil && (!e.mounted || e.sem.seen == gen) {
			return true
		}
	}
	return false
}

// semPass builds the tree once (see the top of the file).
func (t *Tree) semPass() {
	s := &t.sem
	s.gen++
	root := &semantics.Node{Kind: semantics.KindRoot}
	if t.root != nil && t.rootBox.cs != nil {
		root.ID = t.semID(t.root)
		root.Bounds = gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}
		if sc := t.rootBox.scr; sc != nil && (sc.userX && sc.max.X > 0 || sc.userY && sc.max.Y > 0) {
			root.States |= semantics.StateScrollable
			root.Actions |= semantics.ActionScroll
		}
		force := t.root.sem.flags&semGeom != 0 || !s.reuse
		sticky := false
		inh := semInherit{blockify: blockifies(t.rootBox.cs), invisible: t.rootBox.cs.Visibility != style.VisibilityVisible}
		s.portal = nil
		for _, c := range t.root.children {
			t.semElement(c, root, inh, force)
			sticky = sticky || c.sem.flags != 0
		}
		// The portals' content after the root's, as if appended to the
		// body (portal.go).
		for _, p := range t.portalsIn(t.root, nil) {
			s.portal = p
			t.semElement(p, root, semInherit{}, force)
			sticky = sticky || p.sem.flags != 0
		}
		s.portal = nil
		if s.checking {
			semantics.FillSetPositions(root)
		} else {
			t.root.sem.flags = 0
			if sticky {
				t.root.sem.flags = semKids
			}
		}
	}
	if !s.checking {
		s.root = root
	}
	s.last = root
}

// semReuse appends the node of el's last build to parent when el can
// keep it (see the top of the file), and reports whether it did.
func (t *Tree) semReuse(el *Element, parent *semantics.Node, inh semInherit, force bool) bool {
	s := &t.sem
	es := &el.sem
	if !s.reuse || force || s.checking || es.flags != 0 || es.gen == 0 || es.in != inh {
		return false
	}
	if es.node != nil {
		parent.Children = append(parent.Children, es.node)
	}
	return true
}

// semKeep records the node n built for el (nil for none), with inherited
// state inh, and returns the node to put in the tree: el's old node (old,
// made in pass oldGen) when n equals it. sticky reports that a child of el
// is still marked (it was not visited): el stays marked semKids.
func (t *Tree) semKeep(el *Element, old *semantics.Node, oldGen uint64, n *semantics.Node, inh semInherit, sticky bool) *semantics.Node {
	s := &t.sem
	if s.checking {
		return n
	}
	es := &el.sem
	es.in, es.seen = inh, s.gen
	es.flags = 0
	if sticky {
		es.flags = semKids
	}
	if old != nil && n != nil && old != n && semNodeEqual(old, n) {
		es.node, es.gen = old, oldGen
		return old
	}
	es.node, es.gen = n, s.gen
	return n
}

// semWritable returns a node that may be modified in place of x, a node
// of the tree being built: x when this pass made it, else a copy that
// takes its place (semantics.NumberSet).
func (t *Tree) semWritable(x *semantics.Node) *semantics.Node {
	s := &t.sem
	el := s.byID[x.ID]
	if el == nil || el.sem.node != x {
		panic(fmt.Sprintf("core: semantics node %d is not its element's", x.ID))
	}
	if el.sem.gen == s.gen {
		return x
	}
	c := *x
	el.sem.node, el.sem.gen = &c, s.gen
	return &c
}

// semExplicitSet returns the AriaPosInSet and AriaSetSize of the element
// of node n (0 when unset).
func (t *Tree) semExplicitSet(n *semantics.Node) (pos, size int) {
	if el := t.sem.byID[n.ID]; el != nil && el.kind == elemPrimitive {
		if a := el.node.d.aria; a != nil && a.x != nil {
			return int(a.x.posInSet), int(a.x.setSize)
		}
	}
	return 0, 0
}

// semNumber numbers the items of n, the node of a set container being
// built (not hidden, no hidden ancestor).
func (t *Tree) semNumber(n *semantics.Node) {
	if t.sem.checking {
		return // the check numbers its tree with FillSetPositions
	}
	semantics.NumberSet(n, t.semExplicitSet, t.semWritable)
}

// semTrackVolatile records whether primitive el, just built with
// inherited state inh, reads elements outside its subtree, and what.
func (t *Tree) semTrackVolatile(el *Element, inh semInherit) {
	s := &t.sem
	if s.checking {
		return
	}
	var v semVolatile
	if a := el.node.d.aria; a != nil {
		for _, r := range a.labelledBy {
			v.deps = append(v.deps, semDep{ref: r, el: r.el})
		}
		if r := a.describedBy; r != nil {
			v.deps = append(v.deps, semDep{ref: r, el: r.el})
		}
		if w := a.w; w != nil {
			if r := w.controls; r != nil {
				v.deps = append(v.deps, semDep{ref: r, el: r.el})
			}
			if r := w.activeDesc; r != nil {
				v.deps = append(v.deps, semDep{ref: r, el: r.el})
			}
		}
	}
	if (el.box.fld != nil || el.box.kind == style.KindButton) && (el.node.d.id != "" || inh.inLabel) {
		v.labels = true
		for _, l := range t.labelsOf(el) {
			v.deps = append(v.deps, semDep{el: l})
		}
		if inh.inLabel {
			for a := el.styleParent(); a != nil; a = a.styleParent() {
				if a.box.kind == style.KindLabel {
					v.deps = append(v.deps, semDep{el: a})
					break
				}
			}
		}
	}
	if len(v.deps) == 0 && !v.labels {
		delete(s.volatile, el)
		return
	}
	if s.volatile == nil {
		s.volatile = make(map[*Element]semVolatile)
	}
	s.volatile[el] = v
}

// semNodeEqual reports whether a and b hold the same values and the same
// children (by pointer).
func semNodeEqual(a, b *semantics.Node) bool {
	if a.ID != b.ID || a.Kind != b.Kind || a.Role != b.Role || a.ExplicitRole != b.ExplicitRole ||
		a.Decorative != b.Decorative || a.Name != b.Name || a.Description != b.Description ||
		a.Value != b.Value || a.Range != b.Range || a.Text != b.Text || a.URL != b.URL ||
		a.Level != b.Level || a.PosInSet != b.PosInSet || a.SetSize != b.SetSize ||
		a.RowCount != b.RowCount || a.RowIndex != b.RowIndex ||
		a.Checked != b.Checked || a.Pressed != b.Pressed || a.Selected != b.Selected || a.Expanded != b.Expanded ||
		a.Invalid != b.Invalid || a.Sort != b.Sort || a.Current != b.Current ||
		a.States != b.States || a.Actions != b.Actions ||
		a.Live != b.Live || a.Atomic != b.Atomic || a.LiveRoot != b.LiveRoot ||
		a.KeyShortcuts != b.KeyShortcuts || a.Placeholder != b.Placeholder ||
		a.HasPopup != b.HasPopup || a.Modal != b.Modal || a.MultiSelectable != b.MultiSelectable ||
		a.Orientation != b.Orientation || a.AutoComplete != b.AutoComplete ||
		a.Controls != b.Controls || a.ActiveDescendant != b.ActiveDescendant ||
		a.Inline != b.Inline || a.Bounds != b.Bounds || len(a.Children) != len(b.Children) {
		return false
	}
	if (a.Selection == nil) != (b.Selection == nil) || a.Selection != nil && *a.Selection != *b.Selection {
		return false
	}
	for i, c := range a.Children {
		if c != b.Children[i] {
			return false
		}
	}
	return true
}

// checkSemanticsTree builds a full tree and panics where it differs from
// root, the incremental one (TELA_CHECK_SEMANTICS).
func (t *Tree) checkSemanticsTree(root *semantics.Node) {
	s := &t.sem
	saved := s.fns
	s.fns = nil
	s.checking = true
	func() {
		defer func() { s.checking = false }()
		t.semPass()
	}()
	s.fns = saved
	s.gen-- // the check's pass made no node of the tree
	full := s.last
	s.last = root
	if path, diff := semDiff(root, full, "root"); diff != "" {
		panic(fmt.Sprintf("core: TELA_CHECK_SEMANTICS: the incremental semantics tree differs from a full build at %s: %s\nincremental:\n%s\nfull:\n%s",
			path, diff, semDump(root), semDump(full)))
	}
}

// semDiff returns the path and a description of the first difference
// between a and b, or "".
func semDiff(a, b *semantics.Node, path string) (string, string) {
	ca, cb := *a, *b
	ca.Children, cb.Children = nil, nil
	sa, sb := semFields(&ca), semFields(&cb)
	if sa != sb {
		return path, fmt.Sprintf("incremental %s, full %s", sa, sb)
	}
	if len(a.Children) != len(b.Children) {
		return path, fmt.Sprintf("%d children, full %d", len(a.Children), len(b.Children))
	}
	for i := range a.Children {
		if p, d := semDiff(a.Children[i], b.Children[i], fmt.Sprintf("%s/%d(#%d)", path, i, b.Children[i].ID)); d != "" {
			return p, d
		}
	}
	return "", ""
}

// semFields prints the values of n (without its children).
func semFields(n *semantics.Node) string {
	sel := ""
	if n.Selection != nil {
		sel = fmt.Sprintf("%+v", *n.Selection)
	}
	c := *n
	c.Selection = nil
	return fmt.Sprintf("%+v sel=%s", c, sel)
}

// semDump prints the tree rooted at n, one node per line (the first 200).
func semDump(n *semantics.Node) string {
	var b strings.Builder
	lines := 0
	var walk func(n *semantics.Node, depth int)
	walk = func(n *semantics.Node, depth int) {
		if lines++; lines > 200 {
			if lines == 201 {
				b.WriteString("...\n")
			}
			return
		}
		fmt.Fprintf(&b, "%s#%d %v %s %q %q pos=%d/%d states=%b bounds=%v\n", strings.Repeat("  ", depth), n.ID, n.Kind, n.Role, n.Name, n.Text, n.PosInSet, n.SetSize, n.States, n.Bounds)
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	walk(n, 0)
	return b.String()
}
