package raster

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Layer filters.
//
// A PushLayer with filters renders its content into a surface that is
// larger than the pixels it composites: the filters move content by up to
// their reach (blur radius, drop-shadow offset), so the surface extends that
// far beyond the region being rendered and beyond the clip active at
// PushLayer (the clip applies to the filtered result, not to the content).
// At PopLayer the filters run over the whole surface in list order, and the
// result is composited with the layer's opacity through the clip, exactly as
// for a layer without filters.
//
// Filter parameters are in the layer's local space. Under the current
// transform M, a Gaussian of standard deviation s becomes, along device x and
// y, Gaussians with the marginal standard deviations s*|row 0 of M| and
// s*|row 1 of M|: exact for scale/translate transforms and rotations, an
// approximation (the correlation of a skewed Gaussian is dropped) for skews.
// Offsets are mapped with M. Every kernel stays within the reach that
// gfx.FilterOutset promises (see newKernel), which is what keeps region
// rendering exact.

// filter kinds of devFilter.
const (
	filterBlur = iota
	filterDrop
	filterMatrix
)

// devFilter is a filter prepared for the device space of its layer.
type devFilter struct {
	kind   uint8
	kx, ky gaussKernel
	color  [4]uint8 // drop shadow: premultiplied color
	m      [20]float64
}

// reach returns the device support of the filter along x and y.
func (f *devFilter) reach() (int, int) {
	if f.kind == filterMatrix {
		return 0, 0
	}
	return f.kx.support(), f.ky.support()
}

// deviceSigmas returns the device standard deviations along x and y of a
// local Gaussian with standard deviation s under m: s times the norms of the
// rows of the linear part (the marginal standard deviations).
func deviceSigmas(m *gfx.Affine, s float64) (float64, float64) {
	a, b, c, d := float64(m[0]), float64(m[1]), float64(m[2]), float64(m[3])
	return float64(s * math.Sqrt(a*a+c*c)), float64(s * math.Sqrt(b*b+d*d))
}

// prepareFilters appends the device filters of fs under the current
// transform to r.dfs.
func (r *Renderer) prepareFilters(fs []gfx.Filter) {
	m := &r.ctm
	for _, f := range fs {
		var df devFilter
		switch f := f.(type) {
		case gfx.Blur:
			sx, sy := deviceSigmas(m, float64(f.StdDev))
			df.kind = filterBlur
			df.kx, r.kw = newKernel(sx, 0, r.kw)
			df.ky, r.kw = newKernel(sy, 0, r.kw)
		case gfx.DropShadow:
			sx, sy := deviceSigmas(m, float64(f.StdDev))
			ox, oy := float64(f.Offset.X), float64(f.Offset.Y)
			// Products of float32 values are exact in float64.
			dx := float64(m[0])*ox + float64(m[2])*oy
			dy := float64(m[1])*ox + float64(m[3])*oy
			df.kind = filterDrop
			df.kx, r.kw = newKernel(sx, dx, r.kw)
			df.ky, r.kw = newKernel(sy, dy, r.kw)
			df.color = premul8(f.Color)
		case gfx.ColorMatrix:
			df.kind = filterMatrix
			for i, v := range f.M {
				df.m[i] = float64(v)
			}
		}
		r.dfs = append(r.dfs, df)
	}
}

// contentBounds returns the union of the root-space bounds of the commands
// directly inside the block that starts at push index i.
func (r *Renderer) contentBounds(i int) gfx.Rect {
	var u gfx.Rect
	end := r.dl.Match(i)
	for j := i + 1; j < end; j++ {
		u = u.Union(r.dl.BoundsAt(j))
		if m := r.dl.Match(j); m > j {
			j = m // skip the nested block; its push has the union
		}
	}
	return u
}

// devBounds returns the device pixels that a root-space rectangle can
// touch: the bounding box of its corners mapped by r.root (exact for the
// display scale; the products in apply are exact in float64).
func (r *Renderer) devBounds(bb gfx.Rect) image.Rectangle {
	m := &r.root
	if m.IsScaleTranslate() {
		x0, y0 := apply(m, gfx.Pt(bb.X0, bb.Y0))
		x1, y1 := apply(m, gfx.Pt(bb.X1, bb.Y1))
		return pixelBounds(min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1))
	}
	return affineBounds(m, bb)
}

// growXY returns rect grown by dx on the left and right and dy on the top and
// bottom, clamped to ±coordLimit. An empty rect stays empty.
func growXY(rect image.Rectangle, dx, dy int) image.Rectangle {
	if rect.Empty() {
		return image.Rectangle{}
	}
	lim := image.Rect(-coordLimit, -coordLimit, coordLimit, coordLimit)
	return image.Rect(rect.Min.X-dx, rect.Min.Y-dy, rect.Max.X+dx, rect.Max.Y+dy).Intersect(lim)
}

// bbox returns the smallest rectangle containing a and b (either may be
// empty).
func bbox(a, b image.Rectangle) image.Rectangle {
	switch {
	case a.Empty():
		return b
	case b.Empty():
		return a
	}
	return a.Union(b)
}

// pushFilterLayer starts a layer with filters; see the comment at the top of
// this file. The state has been saved; op is the layer's 8-bit opacity.
func (r *Renderer) pushFilterLayer(i int, c gfx.PushLayer, op uint8) {
	b := &r.blit
	st := &r.stack[len(r.stack)-1]
	bbDev := r.devBounds(r.dl.BoundsAt(i))
	comp := b.clip.Intersect(bbDev) // the pixels the layer composites onto
	if comp.Empty() {
		r.narrow(image.Rectangle{})
		return
	}
	first := len(r.dfs)
	r.prepareFilters(c.Filters)
	fs := r.dfs[first:]
	// The total device reach. The surface covers every pixel within reach
	// of the composited ones: the passes of a blur and later filters read
	// intermediate values there, which must not depend on the region.
	var rx, ry int
	for k := range fs {
		x, y := fs[k].reach()
		rx, ry = rx+x, ry+y
	}
	region := growXY(r.region, rx, ry)
	bound := growXY(r.bound.Intersect(bbDev), rx, ry).Intersect(r.devBounds(r.contentBounds(i)))
	in := region.Intersect(bound) // where the content is drawn (inside surf)
	surf := growXY(comp, rx, ry)

	buf := r.getBuf(4 * surf.Dx() * surf.Dy())
	st.layer, st.pix, st.stride, st.origin, st.surf, st.opacity = true, b.pix, b.stride, b.origin, buf, op
	st.comp, st.surfRect, st.nf = comp, surf, len(fs)
	b.pix, b.stride, b.origin, b.cm = buf, 4*surf.Dx(), surf.Min, clipMask{}
	b.clip, r.region, r.bound = in, region, bound
	r.surfBounds = surf
	if in.Empty() {
		// No content can show, but the filters still run at PopLayer (a
		// color matrix can make transparent pixels visible).
		r.skipping = true
	}
}

// applyFilters runs the device filters fs over a premultiplied surface of
// rect's size.
func (r *Renderer) applyFilters(pix []uint8, rect image.Rectangle, fs []devFilter) {
	w, h := rect.Dx(), rect.Dy()
	for k := range fs {
		f := &fs[k]
		switch f.kind {
		case filterBlur:
			r.blurS.blurPlane(pix, 4*w, w, h, 4, &f.kx, &f.ky)
		case filterDrop:
			r.dropShadow(pix, w, h, f)
		case filterMatrix:
			colorMatrix(pix, &f.m)
		}
	}
}

// dropShadow paints the blurred, offset, tinted alpha of the surface behind
// its content: out = content over (color * blurred alpha).
func (r *Renderer) dropShadow(pix []uint8, w, h int, f *devFilter) {
	a := r.getBuf(w * h)
	for i := range a {
		a[i] = pix[4*i+3]
	}
	r.blurS.blurPlane(a, w, w, h, 1, &f.kx, &f.ky)
	c0, c1, c2, c3 := uint32(f.color[0]), uint32(f.color[1]), uint32(f.color[2]), uint32(f.color[3])
	for i, sa := range a {
		if sa == 0 {
			continue
		}
		p := pix[4*i : 4*i+4 : 4*i+4]
		// shadow = color*sa/255, under the content: s + shadow*(255-s.a)/255,
		// rounded once over the common denominator 255².
		k := uint32(sa) * (255 - uint32(p[3]))
		p[0] = uint8(div65025(uint32(p[0])*65025 + c0*k))
		p[1] = uint8(div65025(uint32(p[1])*65025 + c1*k))
		p[2] = uint8(div65025(uint32(p[2])*65025 + c2*k))
		p[3] = uint8(div65025(uint32(p[3])*65025 + c3*k))
	}
	r.putBuf(a)
}

// colorMatrix applies a 4x5 color matrix to every pixel of a premultiplied
// surface: un-premultiply, transform, clamp to [0, 1], premultiply (the CSS
// filter functions operate on non-premultiplied sRGB values).
func colorMatrix(pix []uint8, m *[20]float64) {
	var last, res [4]uint8
	valid := false
	for i := 0; i+3 < len(pix); i += 4 {
		p := pix[i : i+4 : i+4]
		in := [4]uint8{p[0], p[1], p[2], p[3]}
		if valid && in == last {
			p[0], p[1], p[2], p[3] = res[0], res[1], res[2], res[3]
			continue
		}
		var c [4]float64
		if a := in[3]; a > 0 {
			inv := 1 / float64(a)
			c = [4]float64{float64(in[0]) * inv, float64(in[1]) * inv, float64(in[2]) * inv, float64(a) / 255}
		}
		var o [4]float64
		for row := 0; row < 4; row++ {
			k := m[row*5 : row*5+5 : row*5+5]
			v := float64(k[0]*c[0]) + float64(k[1]*c[1]) + float64(k[2]*c[2]) + float64(k[3]*c[3]) + k[4]
			o[row] = clamp01(v)
		}
		oa := o[3]
		res = [4]uint8{unit8(float64(o[0] * oa)), unit8(float64(o[1] * oa)), unit8(float64(o[2] * oa)), unit8(oa)}
		p[0], p[1], p[2], p[3] = res[0], res[1], res[2], res[3]
		last, valid = in, true
	}
}
