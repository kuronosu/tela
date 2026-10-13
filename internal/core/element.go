package core

import (
	"fmt"
	"slices"

	"github.com/kuronosu/tela/internal/style"
)

// elemKind is the kind of an Element.
type elemKind uint8

const (
	elemPrimitive elemKind = iota + 1
	elemText
	elemComponent
)

// Element is the persistent instance of a Node in the tree (ARCHITECTURE
// section 6): it holds the hook state of a component, the text of a text
// node or the Box of a primitive, its identity among its siblings and its
// children. Elements live on the UI thread.
type Element struct {
	tree   *Tree
	parent *Element
	depth  int32
	kind   elemKind
	node   Node // the node of the last build or update

	// Identity among the siblings (see childID).
	scope, sub int32
	key        any
	keyed      bool

	children []*Element

	// Components.
	comp    *compDesc
	hooks   []any        // hook slots, in call order (ADR-040)
	hookIdx int          // next slot during a build
	builds  int          // number of builds so far
	hookDev []hookRecord // dev mode: kind and call site of each slot
	fx      []*effectSlot
	// prov is the state of a Provider (a component without a function).
	prov *providerState
	// vobs holds the Values the last build read (value.go).
	vobs *observer

	// Text.
	text string

	// Primitives.
	box *Box
	// istate is the interaction state of a primitive (input.go): hovered
	// (by hoverN pointers), active (pressed by activeN pointers),
	// disabled (its Disabled prop).
	istate          style.StateFlags
	hoverN, activeN int16
	// groupKids marks the ancestors of an element whose classes use
	// group-* variants, so that a state change of a group finds them
	// without walking unrelated subtrees (never cleared).
	groupKids bool

	mounted    bool
	needsBuild bool // a component whose state or context changed
	// dirtyKids marks the ancestors of a component marked needsBuild, so
	// that the build phase finds it from the root, in tree order, through
	// components that skip their build (React's childLanes). The marked
	// set is closed upwards: an Element marked has its parent marked.
	dirtyKids  bool
	needsStyle bool // a primitive whose style inputs changed
	needsBoxes bool // a primitive whose box structure or paragraph may have changed
	reused     bool // scratch flag of reconcile
	warnedKeys bool // dev mode: the missing-keys warning was given
	boxFrame   uint64
}

// childID is the identity of a child among its siblings. Direct children
// of a primitive (or the result of a component) have scope -1 and their
// position as sub; the children of a fragment at position i have scope i
// and their position inside the fragment (nested fragments flattened).
// Keyed children are matched by scope and key, wherever they are.
type childID struct {
	scope, sub int32
	key        any
}

// keyPair is the key of a child of a keyed fragment: the fragment's key
// and the child's own key or position.
type keyPair struct{ outer, inner any }

func (el *Element) id() childID {
	if el.keyed {
		return childID{scope: el.scope, sub: -1, key: el.key}
	}
	return childID{scope: el.scope, sub: el.sub}
}

// flatEntry is a child node with its identity, after flattening fragments.
type flatEntry struct {
	n          Node
	scope, sub int32
	key        any
	keyed      bool
	dynamic    bool // comes from a Map or List
}

func (e *flatEntry) id() childID {
	if e.keyed {
		return childID{scope: e.scope, sub: -1, key: e.key}
	}
	return childID{scope: e.scope, sub: e.sub}
}

// sameType reports whether el can be updated with node n: same kind and,
// for primitives, the same element kind; for components, the same
// function.
func (el *Element) sameType(n Node) bool {
	d := n.d
	switch el.kind {
	case elemText:
		return d.kind == nodeText
	case elemPrimitive:
		return d.kind == nodeElement && d.elem == el.box.kind
	case elemComponent:
		return d.kind == nodeComponent && d.comp.typ == el.comp.typ && d.comp.ctx == el.comp.ctx
	}
	return false
}

// matches reports whether el is the Element for entry e: same identity
// and type.
func (el *Element) matches(e *flatEntry) bool {
	if el.keyed != e.keyed || el.scope != e.scope {
		return false
	}
	if el.keyed {
		if !safeEqual(el.key, e.key) {
			return false
		}
	} else if el.sub != e.sub {
		return false
	}
	return el.sameType(e.n)
}

// primitive returns the nearest primitive at or above el (the root at the
// latest).
func (el *Element) primitive() *Element {
	for p := el; p != nil; p = p.parent {
		if p.kind == elemPrimitive {
			return p
		}
	}
	return nil
}

// styleParent returns the nearest primitive ancestor of el: the element
// whose computed style el inherits from (display: contents elements
// included, as in CSS).
func (el *Element) styleParent() *Element {
	if el.parent == nil {
		return nil
	}
	return el.parent.primitive()
}

// component returns the nearest component ancestor of el below its
// style parent, if any: the component that produced it. Providers are
// skipped.
func (el *Element) component() *Element {
	for p := el.parent; p != nil && p.kind != elemPrimitive; p = p.parent {
		if p.kind == elemComponent && p.prov == nil {
			return p
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// Flattening.

// flatten appends the entries of children (direct children: scope -1) to
// out.
func flatten(out []flatEntry, children []Node) []flatEntry {
	for i, c := range children {
		d := c.d
		if d == nil {
			continue // a hole: position i stays taken
		}
		if d.kind == nodeFragment {
			var sub, inner int32
			var prefix any
			if d.keyed {
				prefix = d.key
			}
			out = flattenFragment(out, d, int32(i), &sub, prefix, &inner, d.dynamic)
			continue
		}
		out = append(out, flatEntry{n: c, scope: -1, sub: int32(i), key: d.key, keyed: d.keyed})
	}
	return out
}

// flattenFragment appends the children of fragment f, which sits at
// position scope of its parent's children. sub counts the positions of the
// whole list (nested fragments flattened). Inside a keyed fragment the
// children get the key keyPair{fragment key, own key or position in the
// keyed fragment}, so a keyed group keeps its children's identities when
// it moves; inner counts those positions.
func flattenFragment(out []flatEntry, f *desc, scope int32, sub *int32, prefix any, inner *int32, dynamic bool) []flatEntry {
	for _, c := range f.children {
		d := c.d
		if d == nil {
			*sub++
			*inner++
			continue
		}
		if d.kind == nodeFragment {
			if d.keyed {
				var n int32
				out = flattenFragment(out, d, scope, sub, keyPair{prefix, d.key}, &n, dynamic || d.dynamic)
			} else {
				out = flattenFragment(out, d, scope, sub, prefix, inner, dynamic || d.dynamic)
			}
			*inner++
			continue
		}
		e := flatEntry{n: c, scope: scope, sub: *sub, key: d.key, keyed: d.keyed, dynamic: dynamic}
		if prefix != nil {
			k := any(*inner)
			if d.keyed {
				k = d.key
			}
			e.key, e.keyed = keyPair{prefix, k}, true
		}
		out = append(out, e)
		*sub++
		*inner++
	}
	return out
}

// ---------------------------------------------------------------------
// Reconciliation.

// reconcile updates the children of parent to match the nodes (ARCHITECTURE
// section 6): a child with the same identity (key, or position when it has
// none) and type as an old one reuses it, with its state; the others are
// mounted, and the old children left over are unmounted.
func (t *Tree) reconcile(parent *Element, children []Node) {
	buf := t.getFlat()
	entries := flatten(buf[:0], children)
	t.reconcileEntries(parent, entries)
	t.putFlat(entries)
}

// reconcileOne reconciles the single result of a component build.
func (t *Tree) reconcileOne(parent *Element, n Node) {
	buf := t.getFlat()
	var entries []flatEntry
	switch d := n.d; {
	case d == nil:
		entries = buf[:0]
	case d.kind == nodeFragment:
		var sub, inner int32
		var prefix any
		if d.keyed {
			prefix = d.key
		}
		entries = flattenFragment(buf[:0], d, 0, &sub, prefix, &inner, d.dynamic)
	default:
		entries = append(buf[:0], flatEntry{n: n, scope: -1, key: d.key, keyed: d.keyed})
	}
	t.reconcileEntries(parent, entries)
	t.putFlat(entries)
}

func (t *Tree) reconcileEntries(parent *Element, entries []flatEntry) {
	if t.dev && !parent.warnedKeys {
		t.checkKeys(parent, entries)
	}
	old := parent.children
	// Common prefix and suffix: the usual updates (nothing moved, an
	// append, a removal) need no identity map.
	lo := 0
	for lo < len(old) && lo < len(entries) && old[lo].matches(&entries[lo]) {
		lo++
	}
	if lo == len(old) && lo == len(entries) {
		for i := range entries {
			t.update(old[i], entries[i].n)
		}
		return
	}
	hiOld, hiNew := len(old), len(entries)
	for hiOld > lo && hiNew > lo && old[hiOld-1].matches(&entries[hiNew-1]) {
		hiOld--
		hiNew--
	}

	kids := make([]*Element, len(entries))
	for i := 0; i < lo; i++ {
		kids[i] = old[i]
		t.update(old[i], entries[i].n)
	}
	// The middle: match by identity.
	var idx map[childID]*Element
	if hiOld > lo && hiNew > lo {
		idx = make(map[childID]*Element, hiOld-lo)
		for _, o := range old[lo:hiOld] {
			if _, dup := idx[o.id()]; !dup {
				idx[o.id()] = o
			}
		}
	}
	for i := lo; i < hiNew; i++ {
		e := &entries[i]
		id := e.id()
		if o, ok := idx[id]; ok && o.sameType(e.n) {
			delete(idx, id)
			o.reused = true
			o.sub = e.sub
			t.update(o, e.n)
			kids[i] = o
			continue
		}
		kids[i] = t.mount(parent, e)
	}
	for _, o := range old[lo:hiOld] {
		if o.reused {
			o.reused = false
			continue
		}
		t.unmount(o)
	}
	for i, j := hiNew, hiOld; i < len(entries); i, j = i+1, j+1 {
		kids[i] = old[j]
		old[j].sub = entries[i].sub
		t.update(old[j], entries[i].n)
	}
	parent.children = kids
	// The content of the nearest primitive changed: its boxes (and the
	// structural variants of its children) must be computed again.
	p := parent.primitive()
	t.markBoxes(p)
	t.markStructural(p)
}

// checkKeys warns, once per parent, about dynamic lists (Map, List) with
// two or more children without keys, and about duplicate keys.
func (t *Tree) checkKeys(parent *Element, entries []flatEntry) {
	unkeyed := 0
	var seen map[childID]bool
	for i := range entries {
		e := &entries[i]
		if e.dynamic && !e.keyed {
			unkeyed++
		}
		if e.keyed {
			if seen == nil {
				seen = map[childID]bool{}
			}
			id := e.id()
			if seen[id] {
				parent.warnedKeys = true
				t.logf("tela: duplicate key %v among the children of %s; keys must be unique among siblings, or the duplicates are remounted on every build", e.key, t.describe(parent))
				return
			}
			seen[id] = true
		}
	}
	if unkeyed >= 2 {
		parent.warnedKeys = true
		t.logf("tela: %d children of a list (Map or List) under %s have no key; give each item a Key (its ID) so that its state follows it when the list changes", unkeyed, t.describe(parent))
	}
}

// describe names an element for messages.
func (t *Tree) describe(el *Element) string {
	switch el.kind {
	case elemPrimitive:
		if el.parent == nil {
			return "the root"
		}
		return el.node.d.describe()
	case elemComponent:
		return "<" + el.comp.name() + ">"
	}
	return "text"
}

// update brings el up to date with n, which has el's identity and type.
func (t *Tree) update(el *Element, n Node) {
	old := el.node.d
	d := n.d
	switch el.kind {
	case elemText:
		el.node = n
		if el.text != d.text {
			el.text = d.text
			t.markBoxes(el.primitive())
		}
	case elemPrimitive:
		if old == d {
			// The same node: the subtree is unchanged, except for the
			// components below marked for building.
			if el.dirtyKids {
				t.descend(el)
			}
			return
		}
		el.node = n
		bx := el.box
		if old.disabled != d.disabled {
			t.setState(el, style.StateDisabled, d.disabled)
		}
		if !slices.Equal(old.classes, d.classes) {
			bx.classesDirty = true
			t.markStyle(el)
		} else if len(old.props) > 0 || len(d.props) > 0 {
			// Typed props hold closures and cannot be compared.
			t.markStyle(el)
		}
		if d.elem == style.KindImg {
			t.updateImage(el)
		}
		t.reconcile(el, d.children)
		el.dirtyKids = false
	case elemComponent:
		if d.comp.ctx != nil {
			t.updateProvider(el, n)
			return
		}
		if !el.needsBuild && (old == d || canSkip(el.comp, d.comp)) {
			// Automatic skipping (ARCHITECTURE section 7): the same node,
			// or equal props, and no state or context change. The new
			// node is kept, so a later build of its own runs the newest
			// closure.
			el.node, el.comp = n, d.comp
			t.stats.Skips++
			if el.dirtyKids {
				t.descend(el)
			}
			return
		}
		el.node = n
		el.comp = d.comp
		t.build(el)
	}
}

// descend builds the components below el marked for building, in tree
// order, without building el or updating the rest of its subtree: el was
// skipped, or its node did not change.
func (t *Tree) descend(el *Element) {
	for _, c := range el.children {
		switch {
		case c.needsBuild:
			t.build(c)
		case c.dirtyKids:
			t.descend(c)
		}
	}
	el.dirtyKids = false
}

// mount creates the Element of entry e under parent, and its subtree.
func (t *Tree) mount(parent *Element, e *flatEntry) *Element {
	d := e.n.d
	el := &Element{
		tree: t, parent: parent, depth: parent.depth + 1, node: e.n,
		scope: e.scope, sub: e.sub, key: e.key, keyed: e.keyed, mounted: true,
	}
	t.stats.Mounts++
	switch d.kind {
	case nodeText:
		el.kind = elemText
		el.text = d.text
	case nodeElement:
		el.kind = elemPrimitive
		el.box = newBox(t, el, d.elem)
		if d.disabled {
			el.istate = style.StateDisabled
		}
		t.markStyle(el)
		t.markBoxes(el)
		if d.elem == style.KindImg {
			t.updateImage(el)
		}
		t.reconcile(el, d.children)
	case nodeComponent:
		el.kind = elemComponent
		el.comp = d.comp
		if d.comp.ctx != nil {
			t.mountProvider(el)
			break
		}
		if t.dev && d.comp.anon != nil {
			t.reportAnon(d.comp)
		}
		t.build(el)
	default:
		panic(fmt.Sprintf("core: cannot mount node kind %d", d.kind))
	}
	return el
}

// unmount removes el and its subtree. Pending state updates for them are
// dropped, effects that did not run yet never run, and the cleanups of
// the effects that ran are handed to the next commit, parents first
// (React's order); context and Value subscriptions end.
func (t *Tree) unmount(el *Element) {
	el.mounted = false
	t.stats.Unmounts++
	for _, h := range el.hooks {
		if u, ok := h.(unmounter); ok {
			u.unmount(t)
		}
	}
	for _, c := range el.children {
		t.unmount(c)
	}
	el.hooks = nil
	el.fx = nil
	dropObserver(el.vobs)
	if el.box != nil {
		el.box.detach()
	}
}

// markStyle queues a primitive for the style phase.
func (t *Tree) markStyle(el *Element) {
	if el.needsStyle {
		return
	}
	el.needsStyle = true
	t.styleQ = append(t.styleQ, el)
	t.dirty()
}

// markBoxes queues a primitive for the box phase: its box structure or
// paragraph may have changed.
func (t *Tree) markBoxes(el *Element) {
	if el == nil || el.needsBoxes {
		return
	}
	el.needsBoxes = true
	t.boxQ = append(t.boxQ, el)
	t.dirty()
}

// markBuild queues a component for the build phase and marks its
// ancestors dirtyKids.
func (t *Tree) markBuild(el *Element) {
	if el.needsBuild {
		return
	}
	el.needsBuild = true
	for p := el.parent; p != nil && !p.dirtyKids; p = p.parent {
		p.dirtyKids = true
	}
	t.buildQ = append(t.buildQ, el)
	t.dirty()
}

// markStructural restyles the children of p whose classes use structural
// variants (first:, last:, odd:...), after p's children changed.
func (t *Tree) markStructural(p *Element) {
	if !p.box.structuralKids {
		return
	}
	var walk func(el *Element)
	walk = func(el *Element) {
		for _, c := range el.children {
			switch c.kind {
			case elemPrimitive:
				if c.box.structural {
					t.markStyle(c)
				}
			case elemComponent:
				walk(c)
			}
		}
	}
	walk(p)
}

// ---------------------------------------------------------------------
// Scratch buffers for flattening (reconcile recurses, so each level takes
// its own).

func (t *Tree) getFlat() []flatEntry {
	if n := len(t.flatFree); n > 0 {
		b := t.flatFree[n-1]
		t.flatFree = t.flatFree[:n-1]
		return b
	}
	return make([]flatEntry, 0, 8)
}

func (t *Tree) putFlat(b []flatEntry) {
	clear(b) // drop references to nodes
	t.flatFree = append(t.flatFree, b[:0])
}

// safeEqual compares two values with ==, reporting false instead of
// panicking when their dynamic types are not comparable.
func safeEqual(a, b any) (eq bool) {
	defer func() {
		if recover() != nil {
			eq = false
		}
	}()
	return a == b
}
