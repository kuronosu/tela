package canvas

import (
	"fmt"
	"image"
	"math"
	"reflect"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/svg"
)

// DrawImage draws an image with its top left corner at x, y, one unit for
// each of its pixels. The image must not be modified while a canvas draws it:
// it is converted once, while it is drawn. Make it once too, not in each
// draw. A nil image panics in dev mode.
//
// Upright, an image covers whole pixels of the screen, so that images side by
// side leave no seams.
func (c *Canvas) DrawImage(img image.Image, x, y float64) {
	c.drawImage("DrawImage", img, nil, x, y, 0, 0)
}

// DrawImageSize draws an image scaled to a rectangle.
func (c *Canvas) DrawImageSize(img image.Image, x, y, width, height float64) {
	c.drawImage("DrawImageSize", img, nil, x, y, width, height)
}

// DrawImageRegion draws the rectangle sx, sy, sWidth, sHeight of an image, in
// its pixels from its top left one, scaled to the rectangle dx, dy, dWidth,
// dHeight of the canvas. What is outside the image draws nothing. A negative
// size is that of the same rectangle from its other side: it mirrors
// nothing.
func (c *Canvas) DrawImageRegion(img image.Image, sx, sy, sWidth, sHeight, dx, dy, dWidth, dHeight float64) {
	c.drawImage("DrawImageRegion", img, &[4]float64{sx, sy, sWidth, sHeight}, dx, dy, dWidth, dHeight)
}

// drawImage draws the region src of img (nil for all of it, in its size if
// the width and the height are 0 and the method is DrawImage).
func (c *Canvas) drawImage(method string, img image.Image, src *[4]float64, x, y, w, h float64) {
	r := c.use(method)
	if r == nil || !finite(x, y, w, h) || src != nil && !finite(src[:]...) {
		return
	}
	gi := r.image(c, method, img)
	if gi == nil || gi.Width() == 0 || gi.Height() == 0 {
		return
	}
	iw, ih := float64(gi.Width()), float64(gi.Height())
	if method == "DrawImage" {
		w, h = iw, ih
	}
	s := [4]float64{0, 0, iw, ih}
	if src != nil {
		s = *src
	}
	if w == 0 || h == 0 || s[2] == 0 || s[3] == 0 {
		return
	}
	upright(&x, &w)
	upright(&y, &h)
	upright(&s[0], &s[2])
	upright(&s[1], &s[3])
	// The part of the region that is in the image, and where it goes.
	kx, ky := w/s[2], h/s[3]
	x0, y0, x1, y1 := max(s[0], 0), max(s[1], 0), min(s[0]+s[2], iw), min(s[1]+s[3], ih)
	if !(x0 < x1 && y0 < y1) {
		return
	}
	dx0, dy0 := x+float64((x0-s[0])*kx), y+float64((y0-s[1])*ky)
	dx1, dy1 := x+w, y+h
	if s[0]+s[2] > iw {
		dx1 = x + float64((x1-s[0])*kx)
	}
	if s[1]+s[3] > ih {
		dy1 = y + float64((y1-s[1])*ky)
	}
	d := drawing{kind: drawImage, img: gi}
	if !c.ImageSmoothing {
		d.sampling = gfx.SampleNearest
	}
	m := &r.m
	inv, ok := m.invert()
	if !ok {
		return
	}
	d.under = !(m.b == 0 && m.c == 0 && m.a > 0 && m.d > 0)
	if d.under {
		d.m = *m
	}
	// What is drawn, in the current coordinates.
	at := [4]float64{dx0, dy0, dx1, dy1}
	d.src = gfx.Rect{X0: float32(x0), Y0: float32(y0), X1: float32(x1), Y1: float32(y1)}
	to, snapped := r.snapImage(&inv, at)
	if c.ImageSmoothing {
		// How large an image pixel is on the device, along the larger axis.
		k := max(kx*length(m.a, m.b), ky*length(m.c, m.d))
		if r.frame.Scale > 0 {
			k *= r.frame.Scale
		}
		if x0 > 0 || y0 > 0 || x1 < iw || y1 < ih {
			// The samples at the edges of a region read the image beyond it
			// (HTML Standard, "drawing images"), where those of gfx do not:
			// the whole image is drawn, and cut. A region drawn pixel for
			// pixel has no such samples.
			if v := m.point(dx0, dy0); k != 1 || kx != ky || d.under || !whole(v.X, v.Y, r.frame.Scale) {
				d.cut = true
				at = [4]float64{dx0 - float64(x0*kx), dy0 - float64(y0*ky), dx0 + float64((iw-x0)*kx), dy0 + float64((ih-y0)*ky)}
				d.src = gi.Bounds()
			}
		}
		n := 0
		for k > 0 && k <= 0.5 {
			k *= 2
			n++
		}
		if n > 0 {
			d.img = gi.Level(n)
			lx, ly := float32(float64(d.img.Width())/iw), float32(float64(d.img.Height())/ih)
			d.src = gfx.Rect{X0: d.src.X0 * lx, Y0: d.src.Y0 * ly, X1: d.src.X1 * lx, Y1: d.src.Y1 * ly}
		}
	}
	if !snapped {
		d.xs[0], d.nx = imagePiece{to[0], to[2], at[0], at[2]}, 1
		d.ys[0], d.ny = imagePiece{to[1], to[3], at[1], at[3]}, 1
	} else {
		d.cut = true
		px := 1.0
		if r.frame.Scale > 0 {
			px = 1 / r.frame.Scale
		}
		d.nx = imagePieces(&d.xs, at[0], at[2], to[0], to[2], px/length(m.a, m.b))
		d.ny = imagePieces(&d.ys, at[1], at[3], to[1], to[3], px/length(m.c, m.d))
	}
	if !d.under {
		for i := range d.xs[:d.nx] {
			d.xs[i] = d.xs[i].in(m.a, m.e)
		}
		for i := range d.ys[:d.ny] {
			d.ys[i] = d.ys[i].in(m.d, m.f)
		}
	}
	r.draw(c, &d, ink{alpha: c.alpha()})
}

// imagePiece is a part of a drawn image along one axis: the image is at p to
// q, and what shows of it lo to hi.
type imagePiece struct{ lo, hi, p, q float64 }

// in returns i in the coordinates k*x + t.
func (i imagePiece) in(k, t float64) imagePiece {
	return imagePiece{float64(k*i.lo) + t, float64(k*i.hi) + t, float64(k*i.p) + t, float64(k*i.q) + t}
}

// imagePieces makes out the parts in which an image at p to q shows in lo to
// hi, which are edges of pixels of the size px, and returns how many. The
// pixels at the ends whose centers are in the image, but not all of them, are
// parts of their own: the image is stretched from their centers to cover
// them, so that they are not a blend with what is behind.
func imagePieces(out *[3]imagePiece, p, q, lo, hi, px float64) (n int) {
	core := imagePiece{lo, hi, p, q}
	// A hundredth of a pixel is nothing to stretch for.
	if hair := px / 100; p > lo+hair {
		c := lo + px/2
		k := px / 2 / max(c-p, hair)
		out[n] = imagePiece{lo, lo + px, lo, c + float64((q-c)*k)}
		n, core.lo = n+1, lo+px
	}
	if hair := px / 100; q < hi-hair {
		c := hi - px/2
		k := px / 2 / max(q-c, hair)
		out[n] = imagePiece{hi - px, hi, c - float64((c-p)*k), hi}
		n, core.hi = n+1, hi-px
	}
	if core.hi > core.lo+px/2 {
		out[n] = core
		n++
	}
	return n
}

// snapImage returns the rectangle of the pixels of the device whose centers
// are in the rectangle at, both in the current coordinates, whose transform
// has the inverse inv, and whether it is another one. Chromium does not
// antialias the edges of images (BaseRenderingContext2D's
// ShouldDrawImageAntialiased): the images that cover an area, as the tiles of
// a map, leave no seams at any scale. It does when they are turned, or
// thinner than a pixel, which would draw nothing.
func (r *Recorder) snapImage(inv *matrix, at [4]float64) ([4]float64, bool) {
	if !r.m.axisAligned() {
		return at, false
	}
	to, size := r.snap(inv, at)
	return to, to != at && size >= 1
}

// snap returns the rectangle of the pixels whose centers are in the rectangle
// at, both in the current coordinates, whose transform keeps rectangles
// upright and has the inverse inv, and the shorter side that at has on the
// device. Edges that are pixels' stay as they are.
func (r *Recorder) snap(inv *matrix, at [4]float64) (to [4]float64, size float64) {
	m, scale := &r.m, 1.0
	if r.frame.Scale > 0 {
		scale = r.frame.Scale
	}
	a, b := m.vec(at[0], at[1]), m.vec(at[2], at[3])
	size = float64(min(math.Abs(b.x-a.x), math.Abs(b.y-a.y)) * scale)
	same := true
	for _, p := range [4]*float64{&a.x, &a.y, &b.x, &b.y} {
		v := float64(*p * scale)
		if u := gfx.RoundDevice(v); math.Abs(v-u) > gfx.DeviceTolerance(v) {
			*p, same = u/scale, false
		}
	}
	if same || !(max(math.Abs(a.x), math.Abs(a.y), math.Abs(b.x), math.Abs(b.y)) < 1<<24) {
		return at, size
	}
	a, b = inv.vec(a.x, a.y), inv.vec(b.x, b.y)
	return [4]float64{min(a.x, b.x), min(a.y, b.y), max(a.x, b.x), max(a.y, b.y)}, size
}

// upright makes the interval of length l from x one of a length of 0 or more.
func upright(x, l *float64) {
	if *l < 0 {
		*x, *l = *x+*l, -*l
	}
}

// whole reports whether the point is on the device's pixel grid.
func whole(x, y float32, scale float64) bool {
	if !(scale > 0) {
		scale = 1
	}
	dx, dy := float64(x)*scale, float64(y)*scale
	return dx == float64(int64(dx)) && dy == float64(int64(dy))
}

// image returns img converted, an argument of the method: once while the
// element draws it (Env.Image), and here once in a draw.
func (r *Recorder) image(c *Canvas, method string, img image.Image) *gfx.Image {
	if img == nil {
		if c.dev {
			panic(fmt.Sprintf("tela: Canvas.%s got a nil image: check the error of what decodes it, and draw nothing until it is loaded", method))
		}
		return nil
	}
	// Images by pointer, as nearly all are, are always hashable.
	v := reflect.ValueOf(img)
	pointer := v.Kind() == reflect.Pointer
	if pointer {
		if v.IsNil() {
			if c.dev {
				panic(fmt.Sprintf("tela: Canvas.%s got a nil %T: check the error of what decodes it, and draw nothing until it is loaded", method, img))
			}
			return nil
		}
		if gi := r.images[img]; gi != nil {
			return gi
		}
	}
	var gi *gfx.Image
	if r.frame.Env != nil {
		gi = r.frame.Env.Image(img)
	} else {
		gi = gfx.NewImage(img)
	}
	if pointer {
		if r.images == nil {
			r.images = map[image.Image]*gfx.Image{}
		}
		r.images[img] = gi
	}
	return gi
}

// Icon is tela.Icon, a value of which is assignable to it.
type Icon = struct{ SVG string }

// DrawIcon draws an icon (a tela.Icon) in a rectangle, as h.Icon does in its
// box: sharp at any scale, fitted by its viewBox, with the color of FillStyle
// as its currentColor. A gradient or a pattern paints the whole shape of the
// icon instead, whatever its own colors.
func (c *Canvas) DrawIcon(icon Icon, x, y, width, height float64) {
	r := c.use("DrawIcon")
	if r == nil || !finite(x, y, width, height) || width == 0 || height == 0 {
		return
	}
	upright(&x, &width)
	upright(&y, &height)
	var doc *svg.Doc
	if r.frame.Env != nil {
		doc = r.frame.Env.Icon(icon.SVG)
	} else {
		doc, _ = svg.Parse([]byte(icon.SVG))
	}
	d := drawing{kind: drawIcon, doc: doc}
	if _, ok := r.m.invert(); !ok {
		return
	}
	if doc == nil {
		r.draw(c, &d, ink{})
		return
	}
	m := &r.m
	d.under = !(m.b == 0 && m.c == 0 && m.a > 0 && m.d > 0)
	g := &r.m
	if d.under {
		d.m, g = *m, &identity
		d.dst = gfx.Rect{X0: float32(x), Y0: float32(y), X1: float32(x + width), Y1: float32(y + height)}
	} else {
		a, b := m.point(x, y), m.point(x+width, y+height)
		d.dst = gfx.Rect{X0: a.X, Y0: a.Y, X1: b.X, Y1: b.Y}
	}
	k := ink{alpha: c.alpha()}
	opts := svg.Options{CurrentColor: gfx.Transparent}
	switch s := c.FillStyle.(type) {
	case nil:
	case gfx.Color:
		if s.IsFinite() {
			opts.CurrentColor = s.Clamp()
		}
	default:
		paint, distorted := r.ink(inkFill, s, c.alpha(), g, c.ImageSmoothing)
		if distorted { // the gradient is not expressible where the icon is upright
			d.under, d.m = true, r.m
			d.dst = gfx.Rect{X0: float32(x), Y0: float32(y), X1: float32(x + width), Y1: float32(y + height)}
			paint, _ = r.ink(inkFill, s, c.alpha(), &identity, c.ImageSmoothing)
		}
		k = paint
		if k.paint == nil {
			r.draw(c, &d, ink{})
			return
		}
		opts.CurrentColor = gfx.Black
	}
	d.list = r.text.icon(doc, opts)
	if d.list == nil {
		r.draw(c, &d, ink{})
		return
	}
	if k.paint != nil {
		doc.DrawRecorded(r.aside, d.dst, d.list)
		d.mask = r.aside.Finish()
	}
	r.draw(c, &d, k)
}
