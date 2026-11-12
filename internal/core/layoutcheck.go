package core

import (
	"fmt"
	"os"

	"github.com/kuronosu/tela/internal/layout"
)

// Incremental layout and placement (ADR-280): a frame maps the layout
// styles of the boxes whose style or structure changed and of their
// children (remapPhase), lays out the boxes whose layout inputs changed
// (layout's caches and its content cut-off, layout.Engine.MarkContent),
// and places the boxes whose inputs changed and those below a box that
// moved (place.go). TELA_CHECK_LAYOUT=1 checks each of them against
// doing it all from scratch and panics at the first difference: the
// layout styles of every box, the layout of every box (a fresh layout
// with no caches), and the placement of every box (a full place). Core's
// tests turn it on.

// envCheckLayout turns on the checks of incremental layout and placement.
const envCheckLayout = "TELA_CHECK_LAYOUT"

// checkLayout reports TELA_CHECK_LAYOUT=1 (read once; core's tests set
// it).
var checkLayout = os.Getenv(envCheckLayout) == "1"

// checkLayoutStyles panics if the layout style of a box of the tree
// differs from the one its computed style maps to now: a change that
// remapPhase did not see. Boxes with layout-phase deferred props are
// left out (their Values queue them, and evaluating them subscribes).
func (t *Tree) checkLayoutStyles() {
	if t.rootBox == nil || t.rootBox.cs == nil {
		return
	}
	if ls := t.rootLayoutStyle(); ls != t.rootBox.ls {
		panic(fmt.Sprintf("tela: %s: the root's layout style is stale", envCheckLayout))
	}
	var walk func(p *Box)
	walk = func(p *Box) {
		last := spaceLast(p)
		for _, k := range p.kids {
			if !k.deferLayout && k.cs != nil {
				ls := mapLayoutStyle(k, p, hasSpaceAfter(k, last))
				keepGrid(&ls, &k.ls)
				if ls != k.ls {
					panic(fmt.Sprintf("tela: %s: the layout style of %s is stale:\nhas:  %+v\nwant: %+v", envCheckLayout, t.describeBox(k), k.ls, ls))
				}
			}
			walk(k)
		}
	}
	walk(t.rootBox)
}

// checkLayoutResults panics if the layout of a box of the tree differs
// from what a layout from scratch gives (layout.CheckFull).
func (t *Tree) checkLayoutResults() {
	if t.rootBox == nil {
		return
	}
	n, err := layout.CheckFull(t.rootBox, layout.DefiniteSpace(t.viewport.W), layout.DefiniteSpace(t.viewport.H))
	if err != nil {
		name := "?"
		if b, ok := n.(*Box); ok {
			name = t.describeBox(b)
		}
		panic(fmt.Sprintf("tela: %s: the layout of %s differs from a full layout: %v", envCheckLayout, name, err))
	}
}

// describeBox names box b for the checks' messages: its element, or the
// owner of an anonymous box.
func (t *Tree) describeBox(b *Box) string {
	switch {
	case b.el != nil:
		return t.describe(b.el)
	case b.owner != nil:
		return "an anonymous box of " + t.describe(b.owner)
	}
	return "an anonymous box"
}
