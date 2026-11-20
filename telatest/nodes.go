package telatest

import (
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
