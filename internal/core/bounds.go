package core

import "github.com/kuronosu/tela/internal/gfx"

// Element bounds (ADR-070): ElementRef.Bounds reads an element's border
// box in window coordinates, transforms included (the DOM's
// getBoundingClientRect), and UseBounds keeps a Value of it that follows
// layout, scrolling and transforms, like a ResizeObserver that also sees
// the position.

// Rect is a rectangle in logical pixels: the position of its top-left
// corner and its size.
type Rect struct{ X, Y, Width, Height float32 }

// Right returns the x of the right edge.
func (r Rect) Right() float32 { return r.X + r.Width }

// Bottom returns the y of the bottom edge.
func (r Rect) Bottom() float32 { return r.Y + r.Height }

// Contains reports whether the point (x, y) is inside r (the right and
// bottom edges are outside).
func (r Rect) Contains(x, y float32) bool {
	return x >= r.X && x < r.X+r.Width && y >= r.Y && y < r.Y+r.Height
}

func rectOf(r gfx.Rect) Rect { return Rect{r.X0, r.Y0, r.Width(), r.Height()} }

// Bounds returns the border box of the element in window coordinates and
// logical pixels, as it was last laid out and painted: moved by the scroll
// offsets of the containers around it and by its own transform and those
// of its ancestors, as the axis-aligned bounding box of the transformed
// box (getBoundingClientRect). The window's top-left corner is (0, 0).
// An inline element (a Span in a paragraph) has the union of its
// fragments, and an element with display: contents that of its children.
// It is the zero Rect for an element that is not mounted, is
// display: none, or has not been laid out yet. Call it from event
// handlers and effects; UseBounds gives a Value that follows it.
func (r ElementRef) Bounds() Rect {
	el := r.element()
	if el == nil {
		return Rect{}
	}
	return rectOf(el.tree.elemBounds(el))
}

// boundsFollow is the slot of one UseBounds call: a Value that follows the
// bounds of the element ref is attached to.
type boundsFollow struct {
	ref *elemRef
	v   *Value[Rect]
}

func (f *boundsFollow) unmount(t *Tree) {
	f.v.unmount(t)
	t.bnd.remove(f)
}

// boundsState is the Tree's list of UseBounds slots (UI thread).
type boundsState struct {
	follows []*boundsFollow
	// frames is the frame count of the last check (-1: check again).
	frames int
}

func (s *boundsState) remove(f *boundsFollow) {
	for i, x := range s.follows {
		if x == f {
			copy(s.follows[i:], s.follows[i+1:])
			s.follows[len(s.follows)-1] = nil
			s.follows = s.follows[:len(s.follows)-1]
			return
		}
	}
}

// UseBounds returns a Value that follows the bounds (ElementRef.Bounds) of
// the element ref is attached to: the zero Rect while it is not mounted or
// displayed. The Value is updated at the end of every frame that ran the
// pipeline, when the bounds changed since the last one: layout, scrolling
// and transforms (animated ones included) all move it. A component that
// reads it rebuilds, and a deferred style closure that reads it lays out
// or repaints, in the frame after the change; frames where the bounds are
// the same cost nothing more. The same Value is returned on every build.
func UseBounds(r ElementRef) *Value[Rect] {
	if r.r == nil {
		panic("tela: UseBounds(zero ElementRef): pass the ElementRef returned by tela.UseElementRef and attach it with h.Ref to the element")
	}
	el, i := nextSlot(hookBounds)
	if i < len(el.hooks) {
		f, ok := el.hooks[i].(*boundsFollow)
		if !ok {
			el.tree.slotMismatch(el, i, "UseBounds")
		}
		if f.ref != r.r {
			f.ref = r.r
			el.tree.bnd.frames = -1 // check at the end of this frame
		}
		return f.v
	}
	f := &boundsFollow{ref: r.r, v: newValue(el, Rect{})}
	el.hooks = append(el.hooks, f)
	el.tree.bnd.follows = append(el.tree.bnd.follows, f)
	el.tree.bnd.frames = -1
	return f.v
}

// boundsAfterFrame updates the UseBounds Values after a frame that ran
// the pipeline (afterFrame: outside Frame, so that a change requests the
// frame that reads it).
func (t *Tree) boundsAfterFrame() {
	s := &t.bnd
	if len(s.follows) == 0 || s.frames == t.stats.Frames {
		return
	}
	s.frames = t.stats.Frames
	for _, f := range s.follows {
		if !f.v.el.mounted {
			continue
		}
		b := ElementRef{f.ref}.Bounds()
		if b != *f.v.cur.Load() {
			f.v.storeUI(b)
		}
	}
}
