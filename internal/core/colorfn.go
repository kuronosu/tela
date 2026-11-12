package core

import (
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Deferred colors (ADR-082): BgFn and TextColorFn are paint-phase
// deferred props, like TranslateFn and OpacityFn. A change of a Value
// they read marks the box for painting (value.go); the paint update sees
// that the color changed and records the layers again, without building,
// computing styles, laying out or placing the boxes.
//
// A background is painted by its box, so BgFn needs nothing else, and
// neither does BackgroundGradientFn (ADR-250), whose gradient, a new one on
// every paint, the recording compares by content (paintIn.grad). The
// text color is inherited, but the descendants' styles are computed in
// the style phase, which a paint-phase change does not run. Instead, text
// is recolored when it is painted: each style of a paragraph's layout
// comes from an element (the paragraph's own box for its default style,
// the inline element of each span), and the text of a style takes the
// color of the nearest TextColorFn above (or at) that element, as long as
// no element on the way sets a text color of its own
// (ComputedStyle.TextColorSet), which is how CSS inheritance would
// propagate it. Decorations without a color of their own follow the text.
// Colors that the style phase resolved from currentColor (borders,
// outlines, a text field's text and caret) keep the color the classes
// give. textColorFns counts the boxes with a TextColorFn, so paragraphs
// pay for the walk only while some exist.

// The lists that paint recolored text read the TextColorFn above it
// without noting it: the boxes that paint text and drawings under an
// element whose paint-phase text color changes (its Value, a new
// TextColorFn, a text color class that starts or stops stopping the
// inheritance) are stamped as changed instead (stampText), so that the
// lists that paint them are recorded again (record.go).

// stampText stamps as changed the inputs of the boxes whose painting can
// read the text color element el gives at paint time: the paragraphs and
// drawings of its subtree (an inline element's: of its paragraph).
func (t *Tree) stampText(el *Element) {
	if el == nil {
		return
	}
	L := &t.lay
	stamp := func(b *Box) {
		if b != nil && b.rinN > 0 && b.rinAt != L.epoch {
			b.rinAt = L.epoch
			L.nchg++
		}
	}
	var walk func(e *Element)
	walk = func(e *Element) {
		if !e.mounted {
			return
		}
		if b := e.box; e.kind == elemPrimitive && b != nil {
			stamp(b.inPara)
			if b.mode == modeParagraph || b.mode == modeImage {
				stamp(b)
			}
			for _, a := range b.anon {
				if a.mode == modeParagraph {
					stamp(a)
				}
			}
		}
		for _, c := range e.children {
			walk(c)
		}
	}
	walk(el)
}

// textColorRestyled stamps the text that a restyle of el from old to cs
// recolors at paint time: a TextColorFn that comes or goes, a text color
// of its own that starts or stops stopping one above (TextColorSet), or,
// for an element whose paint-phase text color is not compared when the
// boxes are placed (inline, display: contents), a TextColorFn built again.
func (t *Tree) textColorRestyled(el *Element, old, cs *style.ComputedStyle) {
	if old == nil {
		return // a new element: its lists are new
	}
	fn := cs.HasTextColorFn()
	switch {
	case old.HasTextColorFn() != fn,
		t.textColorFns > 0 && old.TextColorSet != cs.TextColorSet,
		fn && (el.box.inPara != nil || contentsBox(el.box)):
		t.stampText(el)
	}
}

// countTextColorFn keeps Tree.textColorFns when a box's style changes from
// old to cs.
func (t *Tree) countTextColorFn(old, cs *style.ComputedStyle) {
	was := old != nil && old.HasTextColorFn()
	if now := cs.HasTextColorFn(); now != was {
		if now {
			t.textColorFns++
		} else {
			t.textColorFns--
		}
	}
}

// textPaints returns the paints that replace the colors of the text styles
// of paragraph box bx (text.Layout.PaintLineRecolored), or nil when no
// TextColorFn reaches them.
func (p *painter) textPaints(bx *Box) []gfx.Paint {
	t := p.t
	if t.textColorFns <= 0 {
		return nil
	}
	para := bx.para
	var out []gfx.Paint
	set := func(i int, el *Element) {
		if el == nil {
			return
		}
		if c, ok := t.fnTextColor(el); ok {
			if out == nil {
				out = make([]gfx.Paint, 1+len(para.spanEls))
			}
			out[i] = c
		}
	}
	set(0, bx.owner) // the default style: the paragraph box's (an anonymous one's owner's)
	for i, el := range para.spanEls {
		set(i+1, el)
	}
	return out
}

// fnTextColor returns the color a TextColorFn gives the text of element
// el: its own, or the nearest ancestor's whose color el inherits.
func (t *Tree) fnTextColor(el *Element) (gfx.Color, bool) {
	for e := el; e != nil; e = e.styleParent() {
		if e.kind != elemPrimitive || !e.mounted || e.box.cs == nil {
			return gfx.Color{}, false
		}
		cs := e.box.cs
		if cs.HasTextColorFn() {
			return t.deferredPaintStyle(e.box).Text.Color, true
		}
		if cs.TextColorSet {
			return gfx.Color{}, false
		}
	}
	return gfx.Color{}, false
}

// deferredPaintStyle returns the style box b paints with: its placed paint style
// when it is in the box tree, otherwise (an inline element) its style
// with its paint-phase deferred props evaluated now (tracking the Values
// they read, so that a change marks it).
func (t *Tree) deferredPaintStyle(b *Box) *style.ComputedStyle {
	if b.parent != nil && b.pcs != nil {
		return b.pcs
	}
	if !hasPhase(b.cs, style.PhasePaint) {
		return b.cs
	}
	return t.resolveDeferred(b, style.PhasePaint)
}

// contentsBox reports whether b is the box of a mounted display: contents
// element, which is not in the box tree but whose TextColorFn colors the
// text of its children.
func contentsBox(b *Box) bool {
	return b.parent == nil && b.el != nil && b.el.mounted && b.cs != nil && b.cs.Display == style.DisplayContents
}

// inlinePaintStyle is paragraph.inlineStyle with the inline element's
// paint-phase deferred props applied (a BgFn on a Span).
func (p *painter) inlinePaintStyle(para *paragraph, k int) *style.ComputedStyle {
	cs := para.inlineStyle(k)
	if cs == nil || !hasPhase(cs, style.PhasePaint) {
		return cs
	}
	return p.t.resolveDeferred(para.inl[k].box, style.PhasePaint)
}
