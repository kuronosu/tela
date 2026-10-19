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
	spread := false
	for k := range fs {
		x, y := fs[k].reach()
		spread = spread || x > 0 || y > 0
	}
	if !spread {
		// Per-pixel filters only: read the pixels this render draws and no
		// others. Bands rendered in parallel (BackdropReach is 0) must not
		// read rows that other bands are writing.
		if in = in.Intersect(b.clip); in.Empty() {
			return
		}
	}
	out := in.Intersect(b.clip)
	if out.Empty() {
		return
	}
	buf, win, out := r.filterBackdrop(b, in, out, fs)
	r.drawBackdrop(c.Shape, buf, win, out)
	r.putBuf(buf)
}

// filterBackdrop reads the pixels of the surface of b that the filters fs
// (with total reach rx, ry) read to compute their output over out, a part
// of in, the device bounds of the shape: only the pixels drawn are
// computed, each from the pixels within the filters' reach of it, mirrored
// at the edges of in (a mirrored pixel lies within the reach too), so the
// filters run over win, out grown by the reach. It returns the filtered
// window (a buffer from getBuf), win and out.
func (r *Renderer) filterBackdrop(b *blitter, in, out image.Rectangle, fs []devFilter) ([]uint8, image.Rectangle, image.Rectangle) {
	var rx, ry int
	for k := range fs {
		x, y := fs[k].reach()
		rx, ry = rx+x, ry+y
	}
	ext := image.Rect(in.Min.X-rx, in.Min.Y-ry, in.Max.X+rx, in.Max.Y+ry)
	win := image.Rect(out.Min.X-rx, out.Min.Y-ry, out.Max.X+rx, out.Max.Y+ry).Intersect(ext)
	w, h := win.Dx(), win.Dy()
	buf := r.getBuf(4 * w * h)
	iw, ih := in.Dx(), in.Dy()
	// The columns of win inside in are copied in one piece; the others are
	// mirrored.
	cx0, cx1 := max(win.Min.X, in.Min.X), min(win.Max.X, in.Max.X)
	r.blurS.par.run(&r.blurS, h, rowGrain, func(_ *blurScratch, lo, hi int) {
		for y := lo; y < hi; y++ {
			sy := in.Min.Y + mirror(win.Min.Y+y-in.Min.Y, ih)
			row := buf[4*y*w : 4*(y+1)*w]
			src := b.pix[b.offset(in.Min.X, sy):]
			if cx0 < cx1 {
				copy(row[4*(cx0-win.Min.X):4*(cx1-win.Min.X)], src[4*(cx0-in.Min.X):4*(cx1-in.Min.X)])
			}
			for x := win.Min.X; x < win.Max.X; x++ {
				if x >= in.Min.X && x < in.Max.X {
					x = max(x, cx1-1)
					continue
				}
				sx := mirror(x-in.Min.X, iw)
				copy(row[4*(x-win.Min.X):4*(x-win.Min.X)+4], src[4*sx:4*sx+4])
			}
		}
	})
	r.applyFiltersOut(buf, win, fs, out)
	return buf, win, out
}

// drawBackdrop paints a filtered backdrop, buf over win, through the
// shape, inside out.
func (r *Renderer) drawBackdrop(shape gfx.RRect, buf []uint8, win, out image.Rectangle) {
	b := &r.blit
	saved := b.clip
	if b.clip = b.clip.Intersect(out); !b.clip.Empty() {
		sh := surfShader{pix: buf, rect: win}
		b.setShader(&sh)
		r.coverRRect(shape)
	}
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

// BackdropRegion returns the device pixels that a render of region (a
// rectangle inside bounds, the destination's pixels) must cover so that
// its backdrop filters read only pixels the same render paints: region
// grown by what every backdrop filter that spreads pixels (a blur, a drop
// shadow) reads to compute its output inside the grown region, the pixels
// of its read bounds within its reach of that output (BackdropReadRegion),
// from the last filter to the first, clipped to bounds. The pixels of the
// region rendered with Options.Region on the grown region are those of a
// full render (the grown region's other pixels need not be), and rendering
// it costs its area instead of the whole frame.
//
// A backdrop filter inside a layer with filters reads that layer's surface,
// whose extent depends on the region; when one can reach the region,
// BackdropRegion returns bounds (the whole frame), the previous behavior.
func BackdropRegion(dl *gfx.DisplayList, scale float32, region, bounds image.Rectangle) image.Rectangle {
	if scale == 0 {
		scale = 1
	}
	region = region.Intersect(bounds)
	if dl == nil || dl.BackdropReach() <= 0 || region.Empty() {
		return region
	}
	// The pixels a backdrop filter reads must be exact when it reads them:
	// painted by this render, including the output of earlier backdrop
	// filters. Its own output is read only by later ones, so one pass
	// from the last filter to the first suffices.
	for k := dl.NumBackdrops() - 1; k >= 0; k-- {
		i, _, reach, inFilter := dl.Backdrop(k)
		if reach <= 0 || !devRootRect(dl.BoundsAt(i), scale).Overlaps(region) {
			continue
		}
		if inFilter {
			return bounds
		}
		if rd := BackdropReadRegion(dl, scale, k, region).Intersect(bounds); !rd.Empty() {
			region = region.Union(rd)
		}
	}
	return region
}

// BackdropReadRegion returns the device pixels that the k-th backdrop
// filter of dl (in list order, see gfx.DisplayList.Backdrop) reads to
// compute its output inside region: the part of its read bounds within its
// device reach of its output bounds in region. A blur or a drop shadow
// spreads a pixel no farther than the reach, and the mirrored pixels beyond
// the read bounds' edges reflect pixels within the reach too, so the output
// inside region depends on these pixels only; conversely, when pixels it
// reads change, its output changes only within the reach of them
// (BackdropOutputRegion).
func BackdropReadRegion(dl *gfx.DisplayList, scale float32, k int, region image.Rectangle) image.Rectangle {
	i, read, reach, _ := dl.Backdrop(k)
	o := devRootRect(dl.BoundsAt(i), scale).Intersect(region)
	if o.Empty() {
		return image.Rectangle{}
	}
	n := deviceReach(reach, scale)
	return growXY(o, n, n).Intersect(devRootRect(read, scale))
}

// BackdropOutputRegion returns the device pixels of the output of the k-th
// backdrop filter of dl that change when the pixels of changed that it
// reads change: its output bounds within its device reach of them.
func BackdropOutputRegion(dl *gfx.DisplayList, scale float32, k int, changed image.Rectangle) image.Rectangle {
	i, read, reach, _ := dl.Backdrop(k)
	in := devRootRect(read, scale).Intersect(changed)
	if in.Empty() {
		return image.Rectangle{}
	}
	n := deviceReach(reach, scale)
	return growXY(in, n, n).Intersect(devRootRect(dl.BoundsAt(i), scale))
}

// deviceReach returns a root-space filter reach in whole device pixels,
// with a pixel of margin for the float32 rounding of the reach: no kernel
// reads farther (newKernel keeps every kernel within the reach).
func deviceReach(reach, scale float32) int {
	if !(reach > 0) {
		return 0
	}
	return int(math.Ceil(float64(reach)*float64(scale))) + 1
}

// devRootRect returns the device pixels a root-space rectangle can touch
// at the display scale (pixelBounds' margin included).
func devRootRect(r gfx.Rect, scale float32) image.Rectangle {
	s := float64(scale)
	return pixelBounds(float64(r.X0)*s, float64(r.Y0)*s, float64(r.X1)*s, float64(r.Y1)*s)
}
