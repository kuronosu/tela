//go:build linux

package atspi

import (
	"image"
	"math"
	"strings"
	"unicode"

	"github.com/kuronosu/tela/internal/platform"
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

// view is the exposed tree of one AccessTree.
type view struct {
	tree *platform.AccessTree
	// root is the exposed root (the frame). For the empty tree, before the
	// first update, it is a node of ID 0 with no children.
	root  *vnode
	nodes map[platform.AccessID]*vnode
	// controlledBy holds the reverse of aria-controls: the exposed nodes
	// that control each node.
	controlledBy map[platform.AccessID][]platform.AccessID
}

// vnode is an exposed node.
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

// buildView returns the exposed view of t (nil: the empty tree).
func buildView(t *platform.AccessTree) *view {
	v := &view{tree: t, nodes: map[platform.AccessID]*vnode{}}
	root, ok := t.Node(t.Root())
	if !ok {
		root = &emptyRoot
	}
	v.root = v.expose(root, 0)
	for _, x := range v.nodes {
		if c := x.n.Controls; c != 0 && v.nodes[c] != nil {
			if v.controlledBy == nil {
				v.controlledBy = map[platform.AccessID][]platform.AccessID{}
			}
			v.controlledBy[c] = append(v.controlledBy[c], x.n.ID)
		}
	}
	return v
}

// expose adds the exposed node of n, with parent parent, and its
// descendants.
func (v *view) expose(n *platform.AccessNode, parent platform.AccessID) *vnode {
	x := &vnode{n: n, parent: parent, role: atspiRole(n)}
	v.nodes[n.ID] = x
	switch {
	case n.Kind == platform.AccessText:
		x.text, x.hasText = n.Text, true
	case textField(n):
		x.text, x.hasText = n.Value, true
	case n.Kind == platform.AccessElement && childrenPresentational(n.Role):
	case n.Kind == platform.AccessElement && v.foldable(n):
		x.text, x.hasText = v.foldText(n), true
	default:
		v.collect(n.Children, x)
	}
	return x
}

// collect exposes the nodes ids as children of x, lifting the children of
// ignored nodes.
func (v *view) collect(ids []platform.AccessID, x *vnode) {
	for _, id := range ids {
		c, ok := v.tree.Node(id)
		if !ok {
			continue
		}
		if c.Ignored() {
			v.collect(c.Children, x)
			continue
		}
		cx := v.expose(c, x.n.ID)
		cx.index = len(x.children)
		x.children = append(x.children, id)
	}
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
		c := v.nodes[x.children[i]]
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
	return s.view.nodes[id]
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
