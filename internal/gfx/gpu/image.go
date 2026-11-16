package gpu

import (
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
)

// Images: DrawImage, with the geometry of raster/image.go. The covered
// area is the destination rectangle, a shape like any fill; the paint maps
// each pixel center to image coordinates with an affine function and the
// shader samples the image's texture as the rasterizer samples its pixels:
// the nearest pixel, or the four around the point with weights quantized
// to 1/256, clamped to the source rectangle.
//
// An image has a texture of its own on the device, named by its ID (a mip
// level is an image with its own ID, chosen by whoever draws). The texture
// is created by the first frame that draws the image and released when no
// frame has drawn it for a while, or when the textures exceed their budget
// and it is the one unused for longest.

// drawImage encodes a DrawImage.
func (e *Encoder) drawImage(c gfx.DrawImage) {
	st := e.top()
	img := c.Image
	src := c.Src.Intersect(img.Bounds())
	if src.Empty() {
		return
	}
	inv, ok := invert(&st.ctm)
	if !ok {
		return
	}
	// Src -> Dst scale factors, and the part of Dst that shows src.
	ku := float64(c.Src.Width()) / float64(c.Dst.Width())
	kv := float64(c.Src.Height()) / float64(c.Dst.Height())
	dst := gfx.Rect{
		X0: float32(float64(c.Dst.X0) + (float64(src.X0)-float64(c.Src.X0))/ku),
		Y0: float32(float64(c.Dst.Y0) + (float64(src.Y0)-float64(c.Src.Y0))/kv),
		X1: float32(float64(c.Dst.X0) + (float64(src.X1)-float64(c.Src.X0))/ku),
		Y1: float32(float64(c.Dst.Y0) + (float64(src.Y1)-float64(c.Src.Y0))/kv),
	}
	if c.Src == src {
		dst = c.Dst
	}
	if dst.Empty() || !(ku > 0 && kv > 0) || math.IsInf(ku, 0) || math.IsInf(kv, 0) {
		return
	}
	sh, bounds, ok := shapeFor(&st.ctm, gfx.RRect{Rect: dst})
	if !ok || !bounds.Overlaps(st.scissor) {
		return
	}
	if lim := e.res.limits.MaxTextureSize; img.Width() > lim || img.Height() > lim {
		e.unsupported("DrawImage (an image larger than the device's textures)")
		return
	}

	// u = Src.X0 + (x - Dst.X0) * ku with x the local point.
	ua, ub, uc := inv.compose(ku, 0, float64(c.Src.X0)-float64(float64(c.Dst.X0)*ku))
	va, vb, vc := inv.compose(0, kv, float64(c.Src.Y0)-float64(float64(c.Dst.Y0)*kv))
	w, h := float64(img.Width()), float64(img.Height())
	sx0, sy0, sx1, sy1 := float64(src.X0), float64(src.Y0), float64(src.X1), float64(src.Y1)

	var in instance
	quad := e.begin(&in, bounds)
	ox, oy := float64(quad.Min.X), float64(quad.Min.Y)
	var flags uint32
	if sh.local {
		flags |= FlagShapeLocal
	}
	sh.put(&in, SlotShape, ox, oy)
	// The functions of the pixel center, for centers relative to the
	// origin.
	in.floats(SlotPaint, ua, ub, uc+float64(ua*ox)+float64(ub*oy), 0)
	in.floats(SlotPaint+1, va, vb, vc+float64(va*ox)+float64(vb*oy), 0)
	if c.Sampling == gfx.SampleNearest {
		// The range of the pixel indices.
		ilo, ihi := math.Floor(sx0), min(math.Ceil(sx1)-1, w-1)
		jlo, jhi := math.Floor(sy0), min(math.Ceil(sy1)-1, h-1)
		ilo, jlo = max(min(ilo, ihi), 0), max(min(jlo, jhi), 0)
		in.floats(SlotPaint+2, ilo, ihi, jlo, jhi)
		flags |= PaintImageNearest << FlagPaintShift
	} else {
		// The range of u-0.5 and v-0.5: the pixel centers of src.
		centers := func(lo, hi, size float64) (float64, float64) {
			a, b := lo, hi-1
			if b < a { // narrower than a pixel: sample its middle
				a = (lo+hi)/2 - 0.5
				b = a
			}
			return min(max(a, 0), size-1), min(max(b, 0), size-1)
		}
		ulo, uhi := centers(sx0, sx1, w)
		vlo, vhi := centers(sy0, sy1, h)
		in.floats(SlotPaint+2, ulo, uhi, vlo, vhi)
		flags |= PaintImageLinear << FlagPaintShift
	}
	e.useImage(img)
	e.image = img.ID()
	e.emitShape(&in, flags, st, quad, sh.inside(), false)
	e.image = 0
}

// useImage makes sure the device has the texture of img when the frame is
// drawn: a new image is uploaded by the frame, and makes room in the
// budget by releasing the textures no frame used for longest.
func (e *Encoder) useImage(img *gfx.Image) {
	r := e.res
	id := img.ID()
	if im := r.images[id]; im != nil {
		im.used = r.frame
		return
	}
	im := &imageTex{bytes: int64(len(img.Pix())), used: r.frame}
	r.images[id] = im
	r.imageBytes += im.bytes
	f := &e.frame
	if i := slices.Index(f.Releases, id); i >= 0 {
		// Given up, and drawn again before the device was told: it still
		// has the texture.
		f.Releases = slices.Delete(f.Releases, i, i+1)
	} else {
		f.Images = append(f.Images, ImageUpload{ID: id, Width: img.Width(), Height: img.Height(), Pix: img.Pix()})
	}
	for r.imageBytes > r.limits.ImageBytes {
		var old *imageTex
		var oldID uint64
		for k, o := range r.images {
			if o.used != r.frame && (old == nil || o.used < old.used) {
				old, oldID = o, k
			}
		}
		if old == nil {
			break // the frame draws them all
		}
		r.releaseImage(oldID, old)
		// The device may destroy it once this frame is drawn.
		e.frame.Releases = append(e.frame.Releases, oldID)
		r.released = r.released[:len(r.released)-1]
	}
}
