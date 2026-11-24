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
// level is an image with its own ID, chosen by whoever draws, or for an image
// pattern, which is a paint, by patternPaint). The texture
// is created by the first frame that draws the image and released when no
// frame has drawn it for a while, when the textures exceed their budget
// and it is the one unused for longest, or when a frame draws a later image
// of the same stream. The memory of an imported buffer is a texture already
// (import.go).

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
	e.image = e.useImage(img, img)
	e.emitShape(&in, flags, st, quad, sh.inside(), false)
	e.image = 0
}

// useImage makes sure the device has the texture of img when the frame is
// drawn, and returns the ID batches name it by. The memory of an imported
// buffer the device samples is the texture. Any other new image is uploaded
// by the frame, gives up the textures of the images it replaces in its
// stream, and makes room in the budget by releasing the textures no frame
// used for longest.
//
// base is the image of which img is a mip level, or img itself. Whoever
// draws a DrawImage chose its level, and the list names that one; a pattern's
// is chosen here, and the list names the base (resources.markPattern). The
// levels of a frame of a stream replace those of the frames before it. Those
// of an imported buffer are computed on the CPU, as the rasterizer's are,
// from pixels that are read back.
func (e *Encoder) useImage(img, base *gfx.Image) uint64 {
	r := e.res
	f := &e.frame
	from := e.imported(img)
	if from != nil && !from.copied {
		if !slices.Contains(f.Imports, from) {
			f.Imports = append(f.Imports, from)
		}
		return from.id
	}
	id := img.ID()
	if im := r.images[id]; im != nil {
		im.used = r.frame
		return id
	}
	up := ImageUpload{ID: id, Width: img.Width(), Height: img.Height(), From: from}
	im := &imageTex{bytes: 4 * int64(img.Width()) * int64(img.Height()), used: r.frame, stream: base.Stream()}
	if from != nil {
		im.stream = from.stream
	} else {
		up.Pix = img.Pix()
	}
	if base != img {
		if m := e.imported(base); m != nil {
			im.stream = m.stream
		}
		im.base = base.ID()
		r.levels[im.base] = append(r.levels[im.base], id)
	}
	r.images[id] = im
	r.imageBytes += im.bytes
	if i := slices.Index(f.Releases, id); i >= 0 {
		// Given up, and drawn again before the device was told: it still
		// has the texture.
		f.Releases = slices.Delete(f.Releases, i, i+1)
	} else {
		f.Images = append(f.Images, up)
	}
	if im.stream != 0 {
		// The frames of the stream before this one are not drawn again: a
		// stream holds one texture, not those of a second of frames.
		for k, o := range r.images {
			if o.stream == im.stream && k < id && o.used != r.frame {
				e.giveUp(k, o)
			}
		}
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
		e.giveUp(oldID, old)
	}
	return id
}

// giveUp releases the texture of an image that the frame being encoded has
// not drawn: the device may destroy it with this frame, unless the frame
// draws it yet (useImage).
func (e *Encoder) giveUp(id uint64, im *imageTex) {
	r := e.res
	r.releaseImage(id, im)
	e.frame.Releases = append(e.frame.Releases, id)
	r.released = r.released[:len(r.released)-1]
}
