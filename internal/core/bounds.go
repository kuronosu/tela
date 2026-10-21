package core

import (
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
)

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

// boundsState is the Tree's list of UseBounds and UseSize slots (UI
// thread).
type boundsState struct {
	follows []*boundsFollow
	// frames is the frame count of the last check (-1: check again).
	frames int
	// sizes are the UseSize slots.
	sizes []*sizeFollow
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

// sizeFollow is the slot of one UseSize call: a Value that follows the
// border-box size of the element ref is attached to.
type sizeFollow struct {
	ref *elemRef
	v   *Value[Size]
}

func (f *sizeFollow) unmount(t *Tree) {
	f.v.unmount(t)
	s := &t.bnd
	if i := slices.Index(s.sizes, f); i >= 0 {
		s.sizes = slices.Delete(s.sizes, i, i+1)
	}
}

// UseSize returns a Value that follows the size of the element ref is
// attached to: its border box as laid out (ResizeObserver's
// borderBoxSize), unaffected by scrolling and transforms, so a scroll or
// an animated transform never changes it. It is the zero Size while the
// element is not mounted or not displayed, and for an inline element of
// a paragraph or a display: contents one, which have no box of their own.
// The Value is updated in every frame that lays out, after layout and
// before paint: a paint-phase deferred prop that reads it (TranslateFn,
// OpacityFn, BgFn) follows in that frame; a component that reads it
// rebuilds, and a layout-phase prop lays out, in the next frame. The same
// Value is returned on every build.
func UseSize(r ElementRef) *Value[Size] {
	if r.r == nil {
		panic("tela: UseSize(zero ElementRef): pass the ElementRef returned by tela.UseElementRef and attach it with h.Ref to the element")
	}
	el, i := nextSlot(hookSize)
	if i < len(el.hooks) {
		f, ok := el.hooks[i].(*sizeFollow)
		if !ok {
			el.tree.slotMismatch(el, i, "UseSize")
		}
		f.ref = r.r // a build: the frame lays out and checks it
		return f.v
	}
	f := &sizeFollow{ref: r.r, v: newValue(el, Size{})}
	el.hooks = append(el.hooks, f)
	el.tree.bnd.sizes = append(el.tree.bnd.sizes, f)
	return f.v
}

// elemSize returns the border-box size of the element r is attached to
// as laid out, zero without a box of its own in the box tree.
func (t *Tree) elemSize(r *elemRef) Size {
	el := r.el
	if el == nil || !el.mounted || el.kind != elemPrimitive || !t.ownBox(el, false) || !t.inTree(el.box) {
		return Size{}
	}
	l := el.box.ld.Layout()
	return Size{Width: l.Size.W, Height: l.Size.H}
}

// syncSizes updates the UseSize Values after a layout, inside the frame
// (PublishValue: paint-phase readers follow in this frame, the others in
// the next).
func (t *Tree) syncSizes() {
	for _, f := range t.bnd.sizes {
		if f.v.el.mounted {
			if s := t.elemSize(f.ref); s != *f.v.cur.Load() {
				PublishValue(f.v, s)
			}
		}
	}
}
