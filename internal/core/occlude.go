package core

import (
	"image"

	"github.com/kuronosu/tela/internal/gfx"
)

// Occlusion of damage (ADR-275).
//
// A command that paints opaque pixels with source-over sets them whatever
// lies below: an unmatched command painted before an opaque cover, inside
// the device pixels the cover paints fully, changes no pixel of either
// frame there, provided the cover is the same command in both frames (a
// matched pair, which lies after the unmatched commands of both lists that
// come before it: rpos < 2j). Its damage is dropped, so that neither the
// frame nor the backdrop filters that read those pixels see it: the root's
// background that follows the color scheme under a wallpaper that does
// not, which otherwise made every glass pane blur its unchanged backdrop
// again after a scheme switch.
//
// A cover is a FillRect or FillRRect of an opaque color or of a linear
// gradient whose stops are opaque, or a DrawImage of an opaque image,
// drawn inside clips and translations only (no layer: its opacity and
// blend apply to the whole group); the pixels it paints fully are its
// shape's inner device pixels, clipped to the inner pixels of the clips.

// minCover is the fewest device pixels a cover must paint fully to be
// considered: small opaque boxes hide little damage.
const minCover = 64 * 64

// cover is an opaque command of the new frame: its index and the device
// pixels it paints fully.
type cover struct {
	at int
	r  image.Rectangle
}

// occlude drops the damaged rectangles of unmatched commands that an
// opaque cover painted after them hides.
func (d *damager) occlude() {
	cs := d.covers()
	if len(cs) == 0 {
		return
	}
	n := 0
	for m, r := range d.rects {
		hidden := false
		if d.rkind[m] != rectOther {
			for _, c := range cs {
				if d.rpos[m] < 2*c.at && r.In(c.r) {
					hidden = true
					break
				}
			}
		}
		if !hidden {
			d.rects[n], d.rpos[n], d.rkind[n] = r, d.rpos[m], d.rkind[m]
			n++
		}
	}
	d.rects, d.rpos, d.rkind = d.rects[:n], d.rpos[:n], d.rkind[:n]
}

// covers returns the opaque covers of the new frame that are matched with
// an old command, outside the shifted units.
func (d *damager) covers() []cover {
	B := d.b.dl
	s := d.scratch
	type state struct {
		t    gfx.Point
		clip image.Rectangle
		ok   bool // only clips and translations around
	}
	cur := state{clip: d.bounds, ok: true}
	stack := make([]state, 0, 16)
	var out []cover
	for i := range B.Len() {
		m := B.Match(i)
		if m >= 0 && m < i { // a pop
			cur = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			continue
		}
		c := B.At(i)
		if m > i { // a push
			stack = append(stack, cur)
			switch c := c.(type) {
			case gfx.PushClipRect:
				cur.clip = cur.clip.Intersect(d.inner(gfx.RRect{Rect: c.Rect}, cur.t))
			case gfx.PushClipRRect:
				cur.clip = cur.clip.Intersect(d.inner(c.RRect, cur.t))
			case gfx.PushTransform:
				if isTranslation(c.Affine) {
					cur.t = cur.t.Add(gfx.Point{X: c.Affine[4], Y: c.Affine[5]})
				} else {
					cur.ok = false
				}
			default:
				cur.ok = false
			}
			continue
		}
		if !cur.ok || cur.clip.Empty() || s.pairB[i] < 0 || d.unitB[i] >= 0 {
			continue
		}
		var r image.Rectangle
		switch c := c.(type) {
		case gfx.FillRect:
			if opaquePaint(c.Paint) {
				r = d.inner(gfx.RRect{Rect: c.Rect}, cur.t)
			}
		case gfx.FillRRect:
			if opaquePaint(c.Paint) {
				r = d.inner(c.RRect, cur.t)
			}
		case gfx.DrawImage:
			if c.Image != nil && !c.Src.Empty() && inside(c.Src, c.Image.Bounds()) && s.opaqueImage(c.Image) {
				r = d.inner(gfx.RRect{Rect: c.Dst}, cur.t)
			}
		}
		if r = r.Intersect(cur.clip); r.Dx()*r.Dy() >= minCover {
			out = append(out, cover{at: i, r: r})
		}
	}
	return out
}

// opaquePaint reports whether a paint is opaque everywhere: an opaque
// color, or a linear gradient of opaque stops (it pads, reflects or
// repeats them).
func opaquePaint(p gfx.Paint) bool {
	switch p := p.(type) {
	case gfx.Color:
		return p.A >= 1
	case gfx.LinearGradient:
		if len(p.Stops) == 0 {
			return false
		}
		for _, st := range p.Stops {
			if st.Color.A < 1 {
				return false
			}
		}
		return true
	}
	return false
}

// opaqueImage reports whether every pixel of img is opaque, remembered by
// the image's id (images are immutable).
func (s *damageScratch) opaqueImage(img *gfx.Image) bool {
	if o, ok := s.opaque[img.ID()]; ok {
		return o
	}
	if s.opaque == nil || len(s.opaque) > 64 {
		s.opaque = map[uint64]bool{}
	}
	o := true
	pix := img.Pix()
	for i := 3; i < len(pix); i += 4 {
		if pix[i] != 255 {
			o = false
			break
		}
	}
	s.opaque[img.ID()] = o
	return o
}

// inside reports whether rectangle a lies in b.
func inside(a, b gfx.Rect) bool {
	return a.X0 >= b.X0 && a.Y0 >= b.Y0 && a.X1 <= b.X1 && a.Y1 <= b.Y1
}
