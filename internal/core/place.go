package core

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
)

// Partial places (ADR-280). A frame that builds, restyles or lays out
// places only the boxes whose place can have changed, instead of every box
// (painter.place): what placing a box reads is its own state (its paint
// style, its layout, its children, its paragraph, its image, its scroll
// state), its parent's origin and paint style, and, for an out-of-flow box,
// the containment and scroll space of its containing block. So:
//
//   - every change to a box's own state marks it (markPlace): a restyle or
//     an animation step, a new layout (layout.ChangeObserver), new
//     children, a new paragraph or paint of its text, a new image, a new
//     layout style; the boxes a frame that only repaints placed again
//     (update) and those whose hit bounds it grew are marked for the next
//     place;
//   - some boxes are placed on every place, as their place reads more than
//     their own state: the scroll containers (their offsets and interest
//     rects), the sticky boxes (their scroll container), the floating
//     elements and the boxes with a layout animation (their flips count
//     the places);
//   - the place walks from the root to the marked boxes (the ancestors of
//     a marked box note it, Box.dirtyBelow) and places them and the boxes
//     on the way (whose ink, hit bounds and weight sum their children's);
//     the other children keep what they had;
//   - a placed box places all its children when what they read of it
//     changed: its origin, its scroll space, its paint style or its
//     children; and its whole subtree when what the out-of-flow boxes in it
//     read changed: its scroll space, whether its content moves (a content
//     layer), its position or whether it contains positioned boxes.
//
// The sticky boxes and floating elements stay listed in tree order
// (Tree.stickies, floatTree.boxes); a partial place that finds one that
// starts or stops being one, or new children of a box with one below
// (a sort may reorder them), places every box instead. So do the first
// place, a new scale or viewport, and a frame that asks for it (needPlace,
// a floating element mounting). TELA_CHECK_LAYOUT=1 compares every
// partial place with a full one (checkPlace).

// placeState is the state of the partial places of a Tree.
type placeState struct {
	// gen numbers the places (Box.placeDeep).
	gen uint64
	// all asks the next place for a full one; scale and view are the
	// scale and viewport of the last place.
	all   bool
	scale float32
	view  gfx.Size
	// relist: a partial place found a box that starts or stops being
	// sticky or floating.
	relist bool
}

// markPlace marks box b for the next place: something its place reads of
// it changed. Its ancestors note a marked box below them (dirtyBelow); a
// box that moves to another parent marks that parent (its children
// change), so the marks of a box left out of the tree (display: none)
// are found when it comes back.
func (t *Tree) markPlace(b *Box) {
	if b == nil {
		return
	}
	b.placeDirty = true
	for c, a := b, b.parent; a != nil; c, a = a, a.parent {
		if r := a.runs; r != nil {
			// The run of c in a's sums (paint.go: aggChunks).
			if i := int(c.kidIdx); i < len(a.kids) && a.kids[i] == c && i/aggChunk/64 < len(r.dirty) {
				r.dirty[i/aggChunk/64] |= 1 << (i / aggChunk % 64)
			} else {
				r.all = true
			}
		}
		if a.dirtyBelow {
			break
		}
		a.dirtyBelow = true
	}
}

// placeAll asks the next place to place every box.
func (t *Tree) placeAll() { t.plc.all = true }

// beginPlace starts a place: it reports whether it may be partial (allow:
// the frame builds, restyles or lays out, and nothing asked for a full
// place), and for a partial one marks the boxes that every place places
// and those given (the boxes marked for painting).
func (t *Tree) beginPlace(p *painter, marked []*Box, allow bool) bool {
	P := &t.plc
	P.gen++
	partial := allow && !P.all && t.lay.root != nil && P.scale == t.scale && P.view == t.viewport
	P.all, P.relist = false, false
	P.scale, P.view = t.scale, t.viewport
	if !partial {
		return false
	}
	for _, s := range t.scr.list {
		if s != nil {
			t.markPlace(s.box)
		}
	}
	for _, b := range t.stickies {
		t.markPlace(b)
	}
	for _, b := range t.flt.boxes {
		t.markPlace(b)
	}
	live := t.la.boxes[:0]
	for _, b := range t.la.boxes {
		if b.lanim != nil && b.el != nil && b.el.mounted {
			live = append(live, b)
			t.markPlace(b)
		}
	}
	clear(t.la.boxes[len(live):])
	t.la.boxes = live
	for _, b := range marked {
		t.markPlace(b)
	}
	p.part = true
	return true
}

// listIn lists box bx, being placed, with the sticky boxes or the
// floating elements (which flag says, or neither): a full place lists it,
// a partial one notes when it is not where it was.
func (p *painter) listIn(bx *Box, flag uint8) {
	t := p.t
	if p.part {
		if bx.listed != flag {
			t.plc.relist = true
		}
		return
	}
	bx.listed = flag
	switch flag {
	case listedSticky:
		t.stickies = append(t.stickies, bx)
	case listedFloat:
		t.flt.boxes = append(t.flt.boxes, bx)
	}
}

// unlistAll empties the lists of sticky boxes and floating elements, for
// a full place.
func (t *Tree) unlistAll() {
	for _, b := range t.stickies {
		b.listed = 0
	}
	for _, b := range t.flt.boxes {
		b.listed = 0
	}
	clear(t.stickies)
	t.stickies = t.stickies[:0]
	clear(t.flt.boxes)
	t.flt.boxes = t.flt.boxes[:0]
}

// placeKid reports whether a partial place places child k of a box: it
// is marked, holds a marked box, or its parent places all its children.
func (p *painter) placeKid(k *Box, all bool) bool {
	return !p.part || all || k.placeDirty || k.dirtyBelow || k.placeDeep == p.t.plc.gen
}

// endPlace ends a partial place: the sticky boxes and floating elements
// that left the tree leave their lists. It reports false when the lists
// must be made again by a full place (a box started or stopped being
// one).
func (t *Tree) endPlace(p *painter) bool {
	p.part = false
	if t.plc.relist {
		return false
	}
	keep := func(list []*Box, sticky bool) []*Box {
		out := list[:0]
		for _, b := range list {
			if b.listed&listFlag(sticky) != 0 && t.inTree(b) && (sticky && isSticky(b) || !sticky && b.flt != nil) {
				out = append(out, b)
			} else {
				b.listed &^= listFlag(sticky)
			}
		}
		clear(list[len(out):])
		return out
	}
	t.stickies = keep(t.stickies, true)
	t.flt.boxes = keep(t.flt.boxes, false)
	return true
}

// Box.listed flags: the box is in Tree.stickies, in floatTree.boxes.
const (
	listedSticky = 1 << iota
	listedFloat
)

func listFlag(sticky bool) uint8 {
	if sticky {
		return listedSticky
	}
	return listedFloat
}

// placeSnap is what place gave a box, for checkPlace.
type placeSnap struct {
	pcs                  *style.ComputedStyle
	u, ink, hitSub, hitB gfx.Rect
	space                *scroller
	hasPart, blend, grp  bool
	layerSC, hidden      bool
	wt, parts            uint16
}

func snapOf(b *Box) placeSnap {
	return placeSnap{pcs: b.pcs, u: b.u, ink: b.ink, hitSub: b.hitSub, hitB: b.hitBounds, space: b.space,
		hasPart: b.hasPart, blend: b.blendDesc, grp: b.grp, layerSC: b.layerSC, hidden: b.hiddenAtom, wt: b.wt, parts: b.parts}
}

// checkPlace panics if a full place gives a box other than what the
// partial place p just made gave it (TELA_CHECK_LAYOUT). A full place
// after a correct partial one changes nothing, so it is run for real; the
// paint styles of boxes with deferred props are compared by content (each
// place evaluates them anew).
func (t *Tree) checkPlace(p *painter) {
	var boxes []*Box
	var snaps []placeSnap
	var walk func(b *Box)
	walk = func(b *Box) {
		for _, k := range b.kids {
			if k.ls.Display == layout.DisplayNone {
				continue
			}
			boxes = append(boxes, k)
			snaps = append(snaps, snapOf(k))
			walk(k)
		}
	}
	boxes = append(boxes, t.rootBox)
	snaps = append(snaps, snapOf(t.rootBox))
	walk(t.rootBox)
	stickies := append([]*Box(nil), t.stickies...)
	floats := append([]*Box(nil), t.flt.boxes...)
	st := t.stats
	q := painter{t: t, scale: p.scale, full: true}
	q.place(t.rootBox, nil)
	t.stats = st
	for i, b := range boxes {
		got, want := snaps[i], snapOf(b)
		if b.deferPaint && got.pcs != nil && want.pcs != nil && sameStyle(got.pcs, want.pcs, false) {
			got.pcs = want.pcs
		}
		if got != want {
			panic(fmt.Sprintf("tela: %s: a partial place of %s differs from a full one:\npartial: %+v\nfull:    %+v", envCheckLayout, t.describeBox(b), got, want))
		}
	}
	if !sameBoxes(stickies, t.stickies) || !sameBoxes(floats, t.flt.boxes) {
		panic(fmt.Sprintf("tela: %s: the sticky boxes or the floating elements of a partial place differ from a full one's: %d/%d sticky, %d/%d floating",
			envCheckLayout, len(stickies), len(t.stickies), len(floats), len(t.flt.boxes)))
	}
}

func sameBoxes(a, b []*Box) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
