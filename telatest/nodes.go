package telatest

import (
	"sort"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
)

// AriaSnapshotNodes is [App.AriaSnapshot] with, for each line of the YAML,
// the identity of the element the line prints (0 for a text line and the
// /url line of a link). An element keeps its identity for as long as it is
// mounted, across rebuilds, whatever happens to its name or its siblings;
// [App.NodeLocator] turns it back into a locator. It is for tools that
// hold on to elements between snapshots, such as `tela mcp`'s refs; tests
// name elements with [ByRole], [ByText] and [ByTestID].
func (a *App) AriaSnapshotNodes() (yaml string, nodes []uint64) {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	y, ids := semantics.SnapshotIDs(a.tree.Semantics())
	nodes = make([]uint64, len(ids))
	for i, id := range ids {
		nodes[i] = uint64(id)
	}
	return y, nodes
}

// NodeLocator returns the locator of the element that [App.AriaSnapshotNodes]
// gave the identity id. It matches that element for as long as it is
// mounted and visible, and nothing once it is unmounted or hidden. It is a
// method, not a function like [ByRole], so that a tool compiled against an
// older telatest can ask for it through an interface.
func (a *App) NodeLocator(id uint64) Locator { return Locator{kind: locNode, nodeID: id} }

// NodeOf returns the identity of the element l locates (the one
// [App.AriaSnapshotNodes] gives its line), waiting for it as an action
// does: it fails the test when nothing matches by the timeout, or when
// several elements do. It is for tools that name elements by identity,
// such as `tela mcp`'s state of an element given by role and name.
func (a *App) NodeOf(l Locator) uint64 {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	var id semantics.NodeID
	a.waitSettled(func() (bool, string) {
		a.tb.Helper()
		nodes := a.find(l)
		switch len(nodes) {
		case 0:
			return false, a.missMessage(l)
		case 1:
		default:
			a.fatalf("%s", a.strictMessage(l, nodes))
		}
		id = nodes[0].ID
		return true, ""
	})
	return uint64(id)
}

// Scrollables returns the scroll containers of the window a user can
// scroll (overflow auto or scroll, with content that overflows), as the
// identities of their elements ([App.AriaSnapshotNodes]; [App.NodeLocator]
// turns one into a locator), the largest visible area first; hidden ones
// and those wholly outside the window are left out. It
// is for tools that scroll "the page" without naming an element, as
// `tela mcp`'s scroll does; tests scroll the container they mean with
// [App.Scroll].
func (a *App) Scrollables() []uint64 {
	a.tb.Helper()
	a.mustBeOpen()
	a.settleQuery()
	a.mustBeOpen()
	vp := a.tree.Viewport()
	view := gfx.Rect{X1: vp.W, Y1: vp.H}
	type scroller struct {
		id   semantics.NodeID
		area float32
	}
	var found []scroller
	a.tree.Semantics().Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) {
			return false
		}
		if n.Has(semantics.StateScrollable) {
			if r := n.Bounds.Intersect(view); !r.Empty() {
				found = append(found, scroller{n.ID, r.Width() * r.Height()})
			}
		}
		return true
	})
	sort.SliceStable(found, func(i, j int) bool { return found[i].area > found[j].area })
	ids := make([]uint64, len(found))
	for i, s := range found {
		ids[i] = uint64(s.id)
	}
	return ids
}
