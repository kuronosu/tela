//go:build linux

package atspi

import (
	"fmt"
	"image"
	"math"
	"os"
	"slices"
	"strings"
	"unicode"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/idmap"
)

// The exposed view of a window's tree: what the AT sees. It is computed
// once per update, on the bridge's worker, from the platform.AccessTree
// core sent, and never changes after: queries on godbus goroutines read
// it through the window's snapshot without locks.
//
// Rules:
//
//   - Nodes that platform.AccessNode.Ignored reports (layout-only
//     containers, none, presentation) are left out and their children
//     become their parent's, AccessKit's filter.
//   - An element whose role makes its children presentational (button,
//     checkbox, img, slider, switch, tab...; WAI-ARIA's "Children
//     Presentational") is a leaf: its content is its name already.
//   - A text field is a leaf whose text is its Value.
//   - An element whose content is only text (texts, and text-level
//     elements such as strong or code without anything of their own,
//     through ignored containers) folds that text: it is a leaf with the
//     Text interface over the text joined (a line break between blocks),
//     as a GTK label or a browser's paragraph holds its text. A heading,
//     a paragraph, a list item or a link with plain text is one object.
//   - Any other text is an object of its own, a label (ROLE_LABEL) named
//     by its text with the Text interface.

// view is the exposed tree of one AccessTree. Successive views share what
// did not change (persistent maps, ADR-277): updateView makes the next one
// from the nodes an update changed.
type view struct {
	tree *platform.AccessTree
	// root is the exposed root (the frame). For the empty tree, before the
	// first update, it is a node of ID 0 with no children.
	root  *vnode
	nodes idmap.Map[*vnode]
	// controlledBy holds the reverse of aria-controls: the exposed nodes
	// that control each node, in ID order.
	controlledBy idmap.Map[[]platform.AccessID]
}

// vnode is an exposed node. It is immutable once its view is published.
type vnode struct {
	n *platform.AccessNode
	// parent is the exposed parent (0 for the root) and index the node's
	// position among its children.
	parent   platform.AccessID
	index    int
	children []platform.AccessID
	role     uint32
	// text is the content of the Text interface when hasText.
	text    string
	hasText bool
}

var emptyRoot = platform.AccessNode{Kind: platform.AccessRoot}

// controlledByOf returns the exposed nodes that control node id
// (aria-controls), in ID order.
func (v *view) controlledByOf(id platform.AccessID) []platform.AccessID {
	ids, _ := v.controlledBy.Get(uint64(id))
	return ids
}

// get returns the exposed node id, nil when it is not exposed.
func (v *view) get(id platform.AccessID) *vnode {
	x, _ := v.nodes.Get(uint64(id))
	return x
}

// viewBuilder makes a view: in full (buildView) or from the last one
// (updateView).
type viewBuilder struct {
	v     *view
	nodes *idmap.Builder[*vnode]
	ctl   *idmap.Builder[[]platform.AccessID]
	// old is the last view (nil for a full build), dirty the nodes whose
	// exposed node must be computed again, touched the nodes exposed by
	// this build, in the order it exposed them (parents first), and
	// dropped the exposed children it found gone from their parents.
	old     *view
	dirty   map[platform.AccessID]bool
	touched []platform.AccessID
	isNew   map[platform.AccessID]bool
	dropped []platform.AccessID
}

// buildView returns the exposed view of t (nil: the empty tree).
func buildView(t *platform.AccessTree) *view {
	b := newViewBuilder(t, nil)
	root, ok := t.Node(t.Root())
	if !ok {
		root = &emptyRoot
	}
	b.v.root = b.expose(root, 0, 0)
	return b.finish()
}

func newViewBuilder(t *platform.AccessTree, old *view) *viewBuilder {
	b := &viewBuilder{v: &view{tree: t}, old: old, isNew: map[platform.AccessID]bool{}}
	if old != nil {
		b.nodes = idmap.NewBuilder(old.nodes)
		b.ctl = idmap.NewBuilder(old.controlledBy)
	} else {
		b.nodes = &idmap.Builder[*vnode]{}
		b.ctl = &idmap.Builder[[]platform.AccessID]{}
	}
	return b
}

// finish publishes the view.
func (b *viewBuilder) finish() *view {
	b.v.nodes = b.nodes.Map()
	b.v.controlledBy = b.ctl.Map()
	return b.v
}

// set records x as the exposed node of its ID, and its aria-controls.
func (b *viewBuilder) set(x *vnode) {
	id := x.n.ID
	var was platform.AccessID
	if b.old != nil {
		if o, ok := b.nodes.Get(uint64(id)); ok {
			was = o.n.Controls
		}
	}
	if !b.isNew[id] {
		b.isNew[id] = true
		b.touched = append(b.touched, id)
	}
	b.nodes.Set(uint64(id), x)
	if c := x.n.Controls; c != was {
		b.controls(was, id, false)
		b.controls(c, id, true)
	}
}

// controls adds (on) or removes node id from the controllers of target.
func (b *viewBuilder) controls(target, id platform.AccessID, on bool) {
	if target == 0 {
		return
	}
	list, _ := b.ctl.Get(uint64(target))
	i, found := slices.BinarySearch(list, id)
	switch {
	case on && !found:
		list = slices.Insert(slices.Clone(list), i, id)
	case !on && found:
		list = slices.Delete(slices.Clone(list), i, i+1)
	default:
		return
	}
	if len(list) == 0 {
		b.ctl.Delete(uint64(target))
	} else {
		b.ctl.Set(uint64(target), list)
	}
}

// expose makes the exposed node of n, with parent parent at index index,
// and its descendants' (but those it can keep from the last view).
func (b *viewBuilder) expose(n *platform.AccessNode, parent platform.AccessID, index int) *vnode {
	x := &vnode{n: n, parent: parent, index: index, role: atspiRole(n)}
	if b.old != nil {
		if o := b.old.get(n.ID); o != nil {
			b.dropped = append(b.dropped, o.children...)
		}
	}
	b.set(x)
	v := b.v
	switch {
	case n.Kind == platform.AccessText:
		x.text, x.hasText = n.Text, true
	case textField(n):
		x.text, x.hasText = n.Value, true
	case n.Kind == platform.AccessElement && childrenPresentational(n.Role):
	case n.Kind == platform.AccessElement && v.foldable(n):
		x.text, x.hasText = v.foldText(n), true
	default:
		b.collect(n.Children, x)
	}
	return x
}

// collect exposes the nodes ids as children of x, lifting the children of
// ignored nodes.
func (b *viewBuilder) collect(ids []platform.AccessID, x *vnode) {
	for _, id := range ids {
		c, ok := b.v.tree.Node(id)
		if !ok {
			continue
		}
		if c.Ignored() {
			b.collect(c.Children, x)
			continue
		}
		b.child(c, x.n.ID, len(x.children))
		x.children = append(x.children, id)
	}
}

// child exposes c at index of parent: the exposed node of the last view
// when c is not dirty and its node is the same (a copy when it moved),
// else a new one.
func (b *viewBuilder) child(c *platform.AccessNode, parent platform.AccessID, index int) {
	if b.old != nil && !b.dirty[c.ID] {
		if o := b.old.get(c.ID); o != nil && o.n == c {
			if o.parent != parent || o.index != index {
				m := *o
				m.parent, m.index = parent, index
				b.set(&m)
			} else if !b.isNew[c.ID] {
				b.isNew[c.ID] = true // kept as it is
			}
			return
		}
	}
	b.expose(c, parent, index)
}

// passes reports whether the exposed node of an ancestor of n reads n's
// subtree: n is an ignored or text-level element (a container's children
// are found through ignored nodes, a text is folded through both), not a
// text field.
func passes(n *platform.AccessNode) bool {
	return n.Kind == platform.AccessElement && (n.Ignored() || textLevel(n)) && !textField(n)
}

// updateView returns the view of t, made from old, the view of the tree
// that an update turned into t, which held the nodes changed and removed
// the nodes removed: only the exposed nodes that depend on the changed
// nodes are computed again. It also returns the nodes it exposed anew,
// parents first. The root of old's tree must be t's.
func updateView(old *view, t *platform.AccessTree, changed, removed []platform.AccessID) (*view, []platform.AccessID) {
	b := newViewBuilder(t, old)
	b.dirty = map[platform.AccessID]bool{}
	// The nodes whose exposed node reads a changed node: the node itself
	// and its ancestors up to the first that does not pass (passes). The
	// topmost of each such chain is computed again.
	var tops []platform.AccessID
	for _, id := range changed {
		if _, in := t.Node(id); !in {
			continue
		}
		b.dirty[id] = true
		top := id
		for p, _ := t.Parent(id); p != 0; p, _ = t.Parent(p) {
			top = p
			if b.dirty[p] {
				break
			}
			b.dirty[p] = true
			if pn, _ := t.Node(p); !passes(pn) {
				break
			}
		}
		tops = append(tops, top)
	}
	// Ancestors first: a top below another one is computed with it.
	depths := make(map[platform.AccessID]int, len(tops))
	for _, id := range tops {
		d := 0
		for p, _ := t.Parent(id); p != 0; p, _ = t.Parent(p) {
			d++
		}
		depths[id] = d
	}
	slices.SortStableFunc(tops, func(a, c platform.AccessID) int { return depths[a] - depths[c] })
	root, _ := t.Node(t.Root())
	b.v.root = old.root
	for _, id := range tops {
		if b.isNew[id] {
			continue
		}
		if id == root.ID {
			b.v.root = b.expose(root, 0, 0)
			continue
		}
		o := b.old.get(id)
		if o == nil || !b.exposedAlone(id, o) {
			continue
		}
		n, _ := t.Node(id)
		b.expose(n, o.parent, o.index)
	}
	if b.v.root == old.root && old.root.n != root {
		// The root's node changed alone (its bounds): its exposed node
		// follows.
		b.v.root = b.expose(root, 0, 0)
	}
	// The exposed nodes that left: removed from the tree, or no longer
	// found below their exposed parent, with what they exposed.
	var drop func(id platform.AccessID)
	drop = func(id platform.AccessID) {
		if b.isNew[id] {
			return
		}
		o, ok := b.nodes.Get(uint64(id))
		if !ok {
			return
		}
		b.nodes.Delete(uint64(id))
		b.controls(o.n.Controls, id, false)

		for _, c := range o.children {
			drop(c)
		}
	}
	for _, id := range removed {
		drop(id)
	}
	for _, id := range b.dropped {
		drop(id)
	}
	return b.finish(), b.touched
}

// checkViews reports TELA_CHECK_SEMANTICS=1: every view updateView makes
// is compared with a full build (treeChanged), as core checks its trees.
var checkViews = os.Getenv("TELA_CHECK_SEMANTICS") == "1"

// diffViews returns how a differs from b ("" when it does not): the
// exposed nodes, their fields, the root and aria-controls' reverse.
func diffViews(a, b *view) string {
	if a.nodes.Len() != b.nodes.Len() {
		return fmt.Sprintf("%d exposed nodes, full %d", a.nodes.Len(), b.nodes.Len())
	}
	if (a.root == nil) != (b.root == nil) || a.root != nil && a.root.n.ID != b.root.n.ID {
		return "the roots differ"
	}
	diff := ""
	b.nodes.Range(func(k uint64, y *vnode) bool {
		x := a.get(platform.AccessID(k))
		switch {
		case x == nil:
			diff = fmt.Sprintf("node %d is not exposed", k)
		case x.n != y.n && !x.n.Equal(y.n), x.parent != y.parent, x.index != y.index, x.role != y.role,
			x.text != y.text, x.hasText != y.hasText, !slices.Equal(x.children, y.children):
			diff = fmt.Sprintf("node %d: %+v, full %+v", k, *x, *y)
		case !slices.Equal(a.controlledByOf(platform.AccessID(k)), b.controlledByOf(platform.AccessID(k))):
			diff = fmt.Sprintf("node %d is controlled by %v, full %v", k, a.controlledByOf(platform.AccessID(k)), b.controlledByOf(platform.AccessID(k)))
		}
		return diff == ""
	})
	if diff == "" && a.controlledBy.Len() != b.controlledBy.Len() {
		diff = fmt.Sprintf("%d controlled nodes, full %d", a.controlledBy.Len(), b.controlledBy.Len())
	}
	return diff
}

// exposedAlone reports whether node id, the top of a chain of changed
// nodes, exposed in the last view as o, is to be exposed again on its
// own, at its old place: it is still exposed (no ancestor in the new tree
// makes its children presentational or is a text field; one that folds
// its text cannot, as id does not pass) and its exposed parent (its
// nearest ancestor that is not ignored) is o's and is not dirty, so that
// nothing moved it. When that parent is dirty, the chain that made it so
// exposes id again from above.
func (b *viewBuilder) exposedAlone(id platform.AccessID, o *vnode) bool {
	t := b.v.tree
	var parent platform.AccessID
	for p, _ := t.Parent(id); p != 0; p, _ = t.Parent(p) {
		pn, _ := t.Node(p)
		if pn.Kind == platform.AccessElement && (childrenPresentational(pn.Role) || textField(pn)) {
			return false
		}
		if parent == 0 && !pn.Ignored() {
			parent = p
		}
	}
	return parent == o.parent && !b.dirty[parent]
}

// foldable reports whether element n holds only text (see the top of the
// file), at least one text.
func (v *view) foldable(n *platform.AccessNode) bool {
	found := false
	var only func(ids []platform.AccessID) bool
	only = func(ids []platform.AccessID) bool {
		for _, id := range ids {
			c, ok := v.tree.Node(id)
			switch {
			case !ok:
			case c.Kind == platform.AccessText:
				found = true
			case c.Kind == platform.AccessElement && (c.Ignored() || textLevel(c)) && !textField(c):
				if !only(c.Children) {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	return len(n.Children) > 0 && only(n.Children) && found
}

// foldText returns the text under n, a line break between blocks (the
// text of nodes that are not inline).
func (v *view) foldText(n *platform.AccessNode) string {
	var b strings.Builder
	gap := false
	var walk func(id platform.AccessID)
	walk = func(id platform.AccessID) {
		c, ok := v.tree.Node(id)
		if !ok {
			return
		}
		block := c.Kind == platform.AccessElement && !c.Has(platform.AccessInline)
		if block {
			gap = true
		}
		if c.Kind == platform.AccessText {
			if gap && b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
				b.WriteByte('\n')
			}
			b.WriteString(c.Text)
			gap = false
		}
		for _, ch := range c.Children {
			walk(ch)
		}
		if block {
			gap = true
		}
	}
	for _, c := range n.Children {
		walk(c)
	}
	return b.String()
}

// name returns the accessible name of x: the node's, or a text's own
// text with white space collapsed.
func (x *vnode) name() string {
	if x.n.Kind == platform.AccessText {
		return strings.Join(strings.FieldsFunc(x.n.Text, unicode.IsSpace), " ")
	}
	return x.n.Name
}

// hit returns the deepest exposed node under x whose bounds hold the
// point (window logical px), the last child first (painted on top); x
// itself when no child does.
func (v *view) hit(x *vnode, px, py float32) *vnode {
	for i := len(x.children) - 1; i >= 0; i-- {
		c := v.get(x.children[i])
		if c != nil && contains(c.n.Bounds, px, py) {
			return v.hit(c, px, py)
		}
	}
	return x
}

func contains(r platform.Rect, x, y float32) bool {
	return x >= r.X && y >= r.Y && x < r.X+r.W && y < r.Y+r.H
}

// snapshot is a window as queries see it: the view and the window's own
// state. It is immutable once published (Window.snap); the worker
// replaces it.
type snapshot struct {
	view   *view
	title  string
	active bool // the window has the keyboard focus
	// scale is the window's scale (physical px per logical px), origin
	// the content area's top-left on the screen in physical px when
	// hasOrigin (X11); without it screen coordinates are window ones.
	scale     float64
	origin    image.Point
	hasOrigin bool
}

func (s *snapshot) root() *vnode { return s.view.root }

// node returns the exposed node id, nil when it is not exposed.
func (s *snapshot) node(id platform.AccessID) *vnode {
	if id == s.view.root.n.ID {
		return s.view.root
	}
	return s.view.get(id)
}

// extents returns r (logical px, window coordinates) in the AT-SPI
// coordinate type coord, in physical pixels as AccessKit reports them;
// parent is the exposed parent of the node, for coordParent.
func (s *snapshot) extents(r platform.Rect, coord uint32, parent *vnode) (x, y, w, h int32) {
	sc := float32(s.scale)
	if sc <= 0 {
		sc = 1
	}
	fx, fy := r.X*sc, r.Y*sc
	switch coord {
	case coordScreen:
		if s.hasOrigin {
			fx += float32(s.origin.X)
			fy += float32(s.origin.Y)
		}
	case coordParent:
		if parent != nil {
			fx -= parent.n.Bounds.X * sc
			fy -= parent.n.Bounds.Y * sc
		}
	}
	return round32(fx), round32(fy), round32(r.W * sc), round32(r.H * sc)
}

// point converts a point in the coordinate type coord (relative to
// parent's top-left for coordParent) to window logical px.
func (s *snapshot) point(x, y int32, coord uint32, parent *vnode) (float32, float32) {
	sc := float32(s.scale)
	if sc <= 0 {
		sc = 1
	}
	fx, fy := float32(x), float32(y)
	switch coord {
	case coordScreen:
		if s.hasOrigin {
			fx -= float32(s.origin.X)
			fy -= float32(s.origin.Y)
		}
	case coordParent:
		if parent != nil {
			fx += parent.n.Bounds.X * sc
			fy += parent.n.Bounds.Y * sc
		}
	}
	return fx / sc, fy / sc
}

func round32(v float32) int32 {
	r := math.Round(float64(v))
	if r > math.MaxInt32 {
		return math.MaxInt32
	}
	if r < math.MinInt32 {
		return math.MinInt32
	}
	return int32(r)
}
