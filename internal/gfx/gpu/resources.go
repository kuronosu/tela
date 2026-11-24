package gpu

import (
	"image"
	"math"
	"slices"
	"sync/atomic"
	"unsafe"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// Limits bounds what an Encoder keeps on the device. The zero value of a
// field means its default.
type Limits struct {
	// CoveragePage is the width and height of a page of the coverage atlas
	// (default 2048; an even number, at least 16, and at most the device's
	// largest texture) and CoveragePages the largest number of pages
	// (default 16: 64 MiB). An entry is at most half a page wide and high;
	// larger pictures are cut into tiles.
	CoveragePage, CoveragePages int
	// ColorPage and ColorPages are the same for the color atlas (default
	// 1024 and 16: 64 MiB).
	ColorPage, ColorPages int
	// ImageBytes is the memory of the image textures kept between frames
	// (default 256 MiB). A frame that draws more than that keeps what it
	// draws.
	ImageBytes int64
	// MaxTextureSize is the largest width and height of a texture of the
	// device (default 4096, which every Vulkan device has). An image larger
	// than that is not drawn, and reported; so is a layer whose surface,
	// grown for its filters, is larger.
	MaxTextureSize int
	// TargetBytes is the memory of the offscreen targets kept between
	// frames, those of layers and filters (default 256 MiB). A frame that
	// draws with more than that keeps what it draws with.
	TargetBytes int64
}

func (l Limits) withDefaults() Limits {
	def := func(v *int, d int) {
		if *v <= 0 {
			*v = d
		}
	}
	def(&l.CoveragePage, 2048)
	def(&l.CoveragePages, 16)
	def(&l.ColorPage, 1024)
	def(&l.ColorPages, 16)
	def(&l.MaxTextureSize, 4096)
	// A page holds four tiles, and a tile is worth an instance.
	l.CoveragePage, l.ColorPage = max(l.CoveragePage&^1, minPage), max(l.ColorPage&^1, minPage)
	if l.ImageBytes <= 0 {
		l.ImageBytes = 256 << 20
	}
	if l.TargetBytes <= 0 {
		l.TargetBytes = 256 << 20
	}
	return l
}

// minPage is the smallest page of an atlas.
const minPage = 16

// idleFrames is the number of frames (EndFrame calls) that a picture in an
// atlas, an image texture or a cached shape no frame draws is kept for:
// about a second at 60 Hz. It is longer than the rasterizer's scratch
// memory lives (raster's maxIdleFrames), because bringing a picture back
// costs a rasterization and an upload, not only an allocation.
const idleFrames = 60

var nextResourceID atomic.Uint64

// resources is what an Encoder keeps from frame to frame, and the device
// mirrors: the atlases and the image textures, with the tables that find
// a picture again (paths, shapes, clip masks).
type resources struct {
	limits Limits
	// id names this set of resources and serial counts the frames that
	// changed it (Frame.Res). dirty: the frame being encoded changes it.
	id     uint64
	serial uint64
	dirty  bool
	// frame numbers the Encode calls; mark is the frame of the last
	// EndFrame.
	frame, mark uint64

	atlas  [NumAtlases]atlas
	images map[uint64]*imageTex
	// levels holds, by the ID of an image, the IDs of its mip levels in
	// images that patterns drew.
	levels map[uint64][]uint64
	// opaque remembers, by ID, whether the images seen are opaque
	// (cull.go).
	opaque map[uint64]bool
	// imageBytes is the memory of the images; released are the IDs the
	// next frame tells the device to destroy.
	imageBytes int64
	released   []uint64

	// targets is the pool of offscreen targets (target.go).
	targets     []*poolTarget
	targetBytes int64
	nextTarget  int

	paths   pathTable
	styles  map[uint64][]*styleRec
	shapes  map[coverKey]*coverShape
	maskIDs map[maskDef]*maskID
	nextID  uint64

	masker raster.Masker
	// Scratch for rasterizing tiles, and for cutting a shape into pieces.
	tmp, tmp2 []uint8
	cells     []uint8
	rectBuf   []image.Rectangle
}

func (r *resources) init(l Limits) {
	l = l.withDefaults()
	*r = resources{limits: l, id: nextResourceID.Add(1)}
	r.atlas[AtlasCoverage].init(1, l.CoveragePage, l.CoveragePages)
	r.atlas[AtlasColor].init(4, l.ColorPage, l.ColorPages)
	r.images = map[uint64]*imageTex{}
	r.levels = map[uint64][]uint64{}
	r.styles = map[uint64][]*styleRec{}
	r.shapes = map[coverKey]*coverShape{}
	r.maskIDs = map[maskDef]*maskID{}
	r.paths.byHash = map[uint64][]*pathRec{}
}

func (r *resources) newID() uint64 {
	r.nextID++
	return r.nextID
}

// scratch returns a zeroed buffer of n bytes kept in *buf.
func scratch(buf *[]uint8, n int) []uint8 {
	*buf = slices.Grow((*buf)[:0], n)[:n]
	clear(*buf)
	return *buf
}

// endFrame ages what the frames since the previous call did not use and
// releases what was idle for more than idleFrames frames.
func (r *resources) endFrame() {
	for i := range r.atlas {
		r.atlas[i].endFrame(r.mark)
	}
	for id, im := range r.images {
		if im.used > r.mark {
			im.idle = 0
		} else if im.idle++; im.idle > idleFrames {
			r.releaseImage(id, im)
		}
	}
	for k, s := range r.shapes {
		if s.used > r.mark {
			s.idle = 0
		} else if s.idle++; s.idle > idleFrames {
			delete(r.shapes, k)
		}
	}
	for k, m := range r.maskIDs {
		if m.used > r.mark {
			m.idle = 0
		} else if m.idle++; m.idle > idleFrames {
			delete(r.maskIDs, k)
		}
	}
	for h, list := range r.styles {
		kept := list[:0]
		for _, s := range list {
			if s.used > r.mark {
				s.idle = 0
			} else if s.idle++; s.idle > idleFrames {
				continue
			}
			kept = append(kept, s)
		}
		clear(list[len(kept):])
		if len(kept) == 0 {
			delete(r.styles, h)
		} else {
			r.styles[h] = kept
		}
	}
	r.paths.endFrame(r.mark)
	r.endFrameTargets(r.mark)
	r.masker.EndFrame()
	r.mark = r.frame
	// Only a frame with new pictures needs the scratch: a large one is not
	// kept for the next.
	if cap(r.tmp) > 1<<16 {
		r.tmp, r.tmp2 = nil, nil
	}
	if cap(r.cells) > 1<<16 {
		r.cells = nil
	}
}

// ---------------------------------------------------------------------------
// Images
// ---------------------------------------------------------------------------

// imageTex is an image the device holds a texture of.
type imageTex struct {
	bytes int64
	used  uint64
	idle  uint16
	// stream is the stream the image is a frame of (gfx.Image.Stream), or 0.
	stream uint64
	// base is the ID of the image of which it is the mip level a pattern
	// chose, or 0.
	base uint64
}

// markImages marks the images that dl draws, those of its patterns and the
// lists of its masks included, as used by the frame being encoded.
func (r *resources) markImages(dl *gfx.DisplayList, depth int) {
	if dl == nil || depth > 8 {
		return
	}
	for _, c := range dl.All() {
		switch c := c.(type) {
		case gfx.DrawImage:
			if c.Image != nil {
				r.markImage(c.Image.ID())
			}
		case gfx.PushLayer:
			r.markImages(c.Mask.List, depth+1)
		default:
			if img := patternImage(c); img != nil {
				r.markPattern(img.ID())
			}
		}
	}
}

// patternImage returns the image of the pattern a command paints with, nil
// when it paints with none.
func patternImage(c gfx.Command) *gfx.Image {
	var p gfx.Paint
	switch c := c.(type) {
	case gfx.FillRect:
		p = c.Paint
	case gfx.FillRRect:
		p = c.Paint
	case gfx.FillPath:
		p = c.Paint
	case gfx.StrokePath:
		p = c.Paint
	case gfx.DrawGlyphRun:
		p = c.Paint
	}
	if p, ok := p.(gfx.ImagePattern); ok {
		return p.Image
	}
	return nil
}

// markImage marks an image, if the device holds it, as used by the frame
// being encoded.
func (r *resources) markImage(id uint64) {
	if im := r.images[id]; im != nil {
		im.used = r.frame
	}
}

// markPattern is markImage for the image of a pattern, and for its mip
// levels that patterns drew: which of them a list draws depends on the
// transforms, and together they are a third of the image.
func (r *resources) markPattern(id uint64) {
	r.markImage(id)
	for _, l := range r.levels[id] {
		r.markImage(l)
	}
}

// setBase notes that the image id, im, is a mip level of base that a pattern
// draws.
func (r *resources) setBase(id uint64, im *imageTex, base uint64) {
	im.base = base
	r.levels[base] = append(r.levels[base], id)
}

func (r *resources) releaseImage(id uint64, im *imageTex) {
	if im.base != 0 {
		if l := slices.DeleteFunc(r.levels[im.base], func(l uint64) bool { return l == id }); len(l) > 0 {
			r.levels[im.base] = l
		} else {
			delete(r.levels, im.base)
		}
	}
	delete(r.images, id)
	r.imageBytes -= im.bytes
	r.released = append(r.released, id)
}

// ---------------------------------------------------------------------------
// Paths and stroke styles by content
// ---------------------------------------------------------------------------

// pathTable gives every distinct path a number, so that a path is found
// again by its content, wherever its memory is: a list recorded again
// every frame draws the same shapes.
//
// perf: a path is hashed every time it is drawn (a pass over its points);
// the points are compared only with a path of the same hash.
type pathTable struct {
	byHash map[uint64][]*pathRec
}

type pathRec struct {
	id   uint64
	path gfx.Path // shares the list's storage (lists are immutable)
	used uint64
	idle uint16
}

func (t *pathTable) endFrame(mark uint64) {
	for h, list := range t.byHash {
		kept := list[:0]
		for _, p := range list {
			if p.used > mark {
				p.idle = 0
			} else if p.idle++; p.idle > idleFrames {
				continue
			}
			kept = append(kept, p)
		}
		clear(list[len(kept):])
		if len(kept) == 0 {
			delete(t.byHash, h)
		} else {
			t.byHash[h] = kept
		}
	}
}

const fnvOffset, fnvPrime = 14695981039346656037, 1099511628211

func hashPath(p gfx.Path) uint64 {
	h := uint64(fnvOffset)
	for _, v := range p.Verbs() {
		h = (h ^ uint64(v)) * fnvPrime
	}
	for _, pt := range p.Points() {
		h = (h ^ uint64(math.Float32bits(pt.X))) * fnvPrime
		h = (h ^ uint64(math.Float32bits(pt.Y))) * fnvPrime
	}
	return h
}

// samePath reports whether two paths have the same verbs and points, bit
// for bit. Paths that share their storage are equal without a look at it.
func samePath(a, b gfx.Path) bool {
	av, bv := a.Verbs(), b.Verbs()
	ap, bp := a.Points(), b.Points()
	if len(av) != len(bv) || len(ap) != len(bp) {
		return false
	}
	if len(av) == 0 {
		return true
	}
	if unsafe.SliceData(av) == unsafe.SliceData(bv) && unsafe.SliceData(ap) == unsafe.SliceData(bp) {
		return true
	}
	if !slices.Equal(av, bv) {
		return false
	}
	for i := range ap {
		if math.Float32bits(ap[i].X) != math.Float32bits(bp[i].X) || math.Float32bits(ap[i].Y) != math.Float32bits(bp[i].Y) {
			return false
		}
	}
	return true
}

// pathID returns the number of p's content.
func (r *resources) pathID(p gfx.Path) uint64 {
	h := hashPath(p)
	for _, rec := range r.paths.byHash[h] {
		if samePath(rec.path, p) {
			rec.used = r.frame
			return rec.id
		}
	}
	rec := &pathRec{id: r.newID(), path: p, used: r.frame}
	r.paths.byHash[h] = append(r.paths.byHash[h], rec)
	return rec.id
}

type styleRec struct {
	id    uint64
	style gfx.StrokeStyle
	used  uint64
	idle  uint16
}

// styleID returns the number of a stroke style's content.
func (r *resources) styleID(s *gfx.StrokeStyle) uint64 {
	f := math.Float32bits
	h := uint64(fnvOffset)
	for _, v := range [...]uint32{f(s.Width), f(s.MiterLimit), uint32(s.Join), uint32(s.Cap), f(s.DashOffset)} {
		h = (h ^ uint64(v)) * fnvPrime
	}
	for _, d := range s.Dashes {
		h = (h ^ uint64(f(d))) * fnvPrime
	}
search:
	for _, rec := range r.styles[h] {
		o := &rec.style
		if f(o.Width) != f(s.Width) || f(o.MiterLimit) != f(s.MiterLimit) || o.Join != s.Join || o.Cap != s.Cap ||
			f(o.DashOffset) != f(s.DashOffset) || len(o.Dashes) != len(s.Dashes) {
			continue
		}
		for i := range s.Dashes {
			if f(o.Dashes[i]) != f(s.Dashes[i]) {
				continue search
			}
		}
		rec.used = r.frame
		return rec.id
	}
	rec := &styleRec{id: r.newID(), style: *s, used: r.frame}
	r.styles[h] = append(r.styles[h], rec)
	return rec.id
}
