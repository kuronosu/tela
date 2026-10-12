package raster

import (
	"image"
	"math"

	"github.com/kuronosu/tela/internal/gfx"
)

// Backdrop filters (CSS backdrop-filter).
//
// A BackdropFilter reads the pixels already painted on the current surface
// (the destination image, or the buffer of the innermost layer: a layer is a
// backdrop root) inside the device bounds of its shape, extends them beyond
// those bounds by mirroring them (Skia's kMirror: the pixel at -1 repeats
// the pixel at 0) as far as the filters reach, runs the filters, and paints
// the result over the surface through the shape (anti-aliased, through the
// active clip) with source-over. This is what Chromium does: its backdrop
// blur never pulls in colors from outside the element, and its edges
// mirror the inside (measured with Chromium 152: a box whose left edge sits
// on a red/blue boundary stays pure blue at its first column).
//
// The filters therefore read every pixel of the shape's bounds, which can
// lie outside the region being rendered. Render renders lists with
// backdrop filters that reach beyond a pixel (DisplayList.BackdropReach > 0)
// on one goroutine and over the whole destination, so that region and band
// rendering keep giving the pixels of a full render (see Renderer.Render).

// surfShader paints the pixels of a premultiplied buffer that covers rect
// in device space: the filtered backdrop.
type surfShader struct {
	pix  []uint8
	rect image.Rectangle
}

func (s *surfShader) shadeSpan(x, y int, dst []uint8) {
	w := s.rect.Dx()
	i := 4 * ((y-s.rect.Min.Y)*w + x - s.rect.Min.X)
	copy(dst, s.pix[i:i+len(dst)])
}

func (s *surfShader) opaque() bool { return false }

// backdropFilter executes a BackdropFilter command.
func (r *Renderer) backdropFilter(c gfx.BackdropFilter) {
	b := &r.blit
	in := r.backdropBounds(c.Shape.Rect).Intersect(r.surfBounds)
	if in.Empty() || !in.Overlaps(b.clip) {
		return
	}
	first := len(r.dfs)
	r.prepareFilters(c.Filters)
	fs := r.dfs[first:]
	defer func() {
		clear(fs)
		r.dfs = r.dfs[:first]
	}()
	var rx, ry int
	for k := range fs {
		x, y := fs[k].reach()
		rx, ry = rx+x, ry+y
	}
	ext := image.Rect(in.Min.X-rx, in.Min.Y-ry, in.Max.X+rx, in.Max.Y+ry)
	w, h := ext.Dx(), ext.Dy()
	buf := r.getBuf(4 * w * h)
	defer r.putBuf(buf)
	iw, ih := in.Dx(), in.Dy()
	for y := 0; y < h; y++ {
		sy := in.Min.Y + mirror(ext.Min.Y+y-in.Min.Y, ih)
		row := buf[4*y*w : 4*(y+1)*w]
		src := b.pix[b.offset(in.Min.X, sy):]
		// The columns of in, copied in one piece, then the mirrored
		// margins.
		copy(row[4*rx:4*(rx+iw)], src[:4*iw])
		for x := 0; x < rx; x++ {
			sx := mirror(x-rx, iw)
			copy(row[4*x:4*x+4], src[4*sx:4*sx+4])
			sx = mirror(iw+x, iw)
			copy(row[4*(rx+iw+x):4*(rx+iw+x)+4], src[4*sx:4*sx+4])
		}
	}
	r.applyFilters(buf, ext, fs)
	sh := surfShader{pix: buf, rect: ext}
	saved := b.clip
	b.clip = b.clip.Intersect(in)
	b.setShader(&sh)
	r.coverRRect(c.Shape)
	b.clip = saved
	b.sh, b.solid = nil, nil
}

// backdropBounds returns the device pixels that hold a local rectangle
// under the current transform: the pixels its bounding box touches,
// without the margin of pixelBounds.
func (r *Renderer) backdropBounds(rect gfx.Rect) image.Rectangle {
	m := &r.ctm
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, p := range [4]gfx.Point{{X: rect.X0, Y: rect.Y0}, {X: rect.X1, Y: rect.Y0}, {X: rect.X1, Y: rect.Y1}, {X: rect.X0, Y: rect.Y1}} {
		x, y := apply(m, p)
		x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
	}
	if !(x0 <= x1 && y0 <= y1) {
		return image.Rectangle{}
	}
	return image.Rect(int(math.Floor(clampLim(x0))), int(math.Floor(clampLim(y0))),
		int(math.Ceil(clampLim(x1))), int(math.Ceil(clampLim(y1))))
}

// mirror maps an index to [0, n) by reflecting at the ends with the edge
// pixel repeated (-1 -> 0, n -> n-1), any number of times.
func mirror(i, n int) int {
	p := 2 * n
	i %= p
	if i < 0 {
		i += p
	}
	if i >= n {
		i = p - 1 - i
	}
	return i
}

// copyRect copies the pixels of rect from src to dst (rect lies inside
// both).
func copyRect(dst, src *image.RGBA, rect image.Rectangle) {
	n := 4 * rect.Dx()
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		copy(dst.Pix[dst.PixOffset(rect.Min.X, y):][:n], src.Pix[src.PixOffset(rect.Min.X, y):][:n])
	}
}
