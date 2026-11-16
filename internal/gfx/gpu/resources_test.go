package gpu

import (
	"bytes"
	"image"
	"math/rand"
	"reflect"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/gfxtest"
	"github.com/kuronosu/tela/internal/gfx/raster"
)

// The tests of the resources an Encoder keeps on the device, without a
// device: what a frame uploads, where instances read, what is found again.

// atlasSim holds the atlases in memory and applies the uploads of frames
// to them, as a device does.
type atlasSim struct {
	t     *testing.T
	info  [NumAtlases]AtlasInfo
	pages [NumAtlases][][]byte
}

func bppOf(atlas int) int {
	if atlas == AtlasColor {
		return 4
	}
	return 1
}

// upload applies the uploads of a pass.
func (s *atlasSim) upload(f *Frame, p *Pass) {
	s.t.Helper()
	for i, info := range f.Atlases {
		if info.Pages > 0 && s.info[i].PageSize != 0 && s.info[i].PageSize != info.PageSize {
			s.t.Fatalf("the page size of atlas %d changed from %d to %d", i, s.info[i].PageSize, info.PageSize)
		}
		if info.Pages > 0 {
			s.info[i].PageSize = info.PageSize
		}
		for len(s.pages[i]) < info.Pages {
			s.pages[i] = append(s.pages[i], make([]byte, info.PageSize*info.PageSize*bppOf(i)))
		}
	}
	for _, u := range p.Uploads {
		size, bpp := s.info[u.Atlas].PageSize, bppOf(u.Atlas)
		if u.Page < 0 || u.Page >= f.Atlases[u.Atlas].Pages || u.Rect.Empty() || !u.Rect.In(image.Rect(0, 0, size, size)) || u.Offset%4 != 0 {
			s.t.Fatalf("upload %+v into an atlas of %d pages of %d", u, f.Atlases[u.Atlas].Pages, size)
		}
		n := u.Rect.Dx() * bpp
		for y := 0; y < u.Rect.Dy(); y++ {
			src := f.Data[u.Offset+y*n:][:n]
			copy(s.pages[u.Atlas][u.Page][((u.Rect.Min.Y+y)*size+u.Rect.Min.X)*bpp:], src)
		}
	}
}

// read returns the pixels an instance reads for the device pixels r, whose
// texels are r moved by d in a page.
func (s *atlasSim) read(atlas, page int, r image.Rectangle, d image.Point) []byte {
	s.t.Helper()
	size, bpp := s.info[atlas].PageSize, bppOf(atlas)
	tr := r.Add(d)
	if page < 0 || page >= len(s.pages[atlas]) || !tr.In(image.Rect(0, 0, size, size)) {
		s.t.Fatalf("an instance reads %v of page %d of atlas %d, which has %d pages of %d", tr, page, atlas, len(s.pages[atlas]), size)
	}
	var out []byte
	for y := tr.Min.Y; y < tr.Max.Y; y++ {
		out = append(out, s.pages[atlas][page][(y*size+tr.Min.X)*bpp:][:tr.Dx()*bpp]...)
	}
	return out
}

// instanceRead is what one instance reads from the atlases: its own pixels
// and its clip mask's, over its quad.
type instanceRead struct {
	quad      image.Rectangle
	own, mask []byte
}

// draw executes the frame on the simulated atlases and returns what every
// instance reads, in drawing order.
func (s *atlasSim) draw(f *Frame) []instanceRead {
	s.t.Helper()
	var out []instanceRead
	for pi := range f.Passes {
		p := &f.Passes[pi]
		s.upload(f, p)
		for _, b := range p.Batches {
			for i := b.First; i < b.First+b.Count; i++ {
				flags := head(f, i)[0]
				bd, org, tex := floats(f, i, SlotBounds), floats(f, i, SlotOrigin), slotInts(f, i, SlotTex)
				rd := instanceRead{quad: image.Rect(int(bd[0]), int(bd[1]), int(bd[2]), int(bd[3]))}
				vis := rd.quad.Intersect(b.Scissor)
				switch {
				case flags&FlagKindMask == FlagCoverAtlas:
					rd.own = s.read(AtlasCoverage, int(org[2]), vis, image.Pt(int(tex[0]), int(tex[1])))
				case flags>>FlagPaintShift&7 == PaintSprite:
					rd.own = s.read(AtlasColor, int(org[2]), vis, image.Pt(int(tex[0]), int(tex[1])))
				}
				if flags&FlagMask != 0 {
					rd.mask = s.read(AtlasCoverage, int(org[3]), vis, image.Pt(int(tex[2]), int(tex[3])))
				}
				out = append(out, rd)
			}
		}
	}
	return out
}

func uploads(f *Frame) int {
	n := 0
	for i := range f.Passes {
		n += len(f.Passes[i].Uploads)
	}
	return n
}

func TestAtlasPacking(t *testing.T) {
	for h, want := range map[int]int{1: 1, 15: 15, 16: 16, 17: 18, 31: 32, 33: 36, 100: 104, 600: 640, 1024: 1024} {
		if got := bucket(h); got != want || got < h {
			t.Errorf("bucket(%d) = %d, want %d", h, got, want)
		}
	}
	var a atlas
	a.init(1, 256, 3)
	rng := rand.New(rand.NewSource(1))
	type placed struct {
		e *atlasEntry
		r image.Rectangle
	}
	var live []placed
	check := func() {
		t.Helper()
		pagesLive := make([]int, len(a.pages))
		for i, p := range live {
			if p.e.page >= len(a.pages) || !p.r.In(image.Rect(0, 0, 256, 256)) || p.r != p.e.rect() {
				t.Fatalf("entry %d at %v of page %d, of %d pages", i, p.r, p.e.page, len(a.pages))
			}
			pagesLive[p.e.page]++
			for _, q := range live[:i] {
				if q.e.page == p.e.page && q.r.Overlaps(p.r) {
					t.Fatalf("entries overlap in page %d: %v and %v", p.e.page, q.r, p.r)
				}
			}
		}
		for i, pg := range a.pages {
			if pg.live != pagesLive[i] {
				t.Fatalf("page %d counts %d entries, has %d", i, pg.live, pagesLive[i])
			}
		}
		if len(a.entries) != len(live) {
			t.Fatalf("%d entries in the map, %d placed", len(a.entries), len(live))
		}
	}
	id := uint64(0)
	add := func(w, h int) bool {
		id++
		e := a.add(atlasKey{kind: keyCover, id: id}, tilePixels, w, h, 1)
		if e == nil {
			return false
		}
		live = append(live, placed{e, e.rect()})
		return true
	}
	// Fill it with pictures of the sizes of glyphs and icons, until it is
	// full: it uses its three pages, and most of their area (pages this
	// small lose more to the ends of their shelves than real ones).
	area := 0
	for {
		w, h := 4+rng.Intn(28), 6+rng.Intn(30)
		if !add(w, h) {
			break
		}
		area += w * h
	}
	check()
	if len(a.pages) != 3 || a.peak != 3 {
		t.Fatalf("a full atlas has %d pages (peak %d), want 3", len(a.pages), a.peak)
	}
	if fill := float64(area) / (3 * 256 * 256); fill < 0.7 {
		t.Errorf("a full atlas holds pictures over %.0f%% of its area", 100*fill)
	}
	// Evict every other entry: their room is used again, by pictures of the
	// same sizes, without a new page.
	n := len(live)
	kept := live[:0]
	freed := 0
	for i, p := range live {
		if i%2 == 0 {
			a.remove(p.e)
			freed += p.r.Dx() * p.r.Dy()
		} else {
			kept = append(kept, p)
		}
	}
	live = kept
	check()
	again := 0
	for i := 0; i < n; i++ {
		w, h := 4+rng.Intn(28), 6+rng.Intn(30)
		if add(w, h) {
			again += w * h
		}
	}
	check()
	if again < freed/2 {
		t.Errorf("after evicting %d pixels of pictures, only %d fit again", freed, again)
	}
	// Evict everything: the pages are empty and go.
	for _, p := range live {
		a.remove(p.e)
	}
	live = live[:0]
	check()
	for i, pg := range a.pages {
		if pg.live != 0 || len(pg.shelves) != 0 || pg.nextY != 0 {
			t.Errorf("page %d of an empty atlas: %+v", i, pg)
		}
	}
	a.shrink()
	if len(a.pages) != 0 {
		t.Errorf("an empty atlas keeps %d pages", len(a.pages))
	}
	// A picture as large as a tile fits; uniform pictures take no room.
	for i := 0; i < 4; i++ {
		if !add(128, 128) {
			t.Fatalf("tile %d does not fit", i)
		}
	}
	if len(a.pages) != 1 {
		t.Errorf("four tiles do not fill one page (%d pages)", len(a.pages))
	}
	if e := a.add(atlasKey{kind: keyCover, id: 1 << 40}, tileFull, 0, 0, 1); e == nil || e.sh != nil || len(a.pages) != 1 {
		t.Errorf("a uniform picture: %+v, %d pages", e, len(a.pages))
	}
}

func TestAtlasEviction(t *testing.T) {
	var a atlas
	a.init(1, 64, 1)
	key := func(i int) atlasKey { return atlasKey{kind: keyCover, id: uint64(i)} }
	// Frames 1 to 8 add two pictures each; frame 9 uses those of frame 2.
	for f := 1; f <= 8; f++ {
		for k := 0; k < 2; k++ {
			if a.add(key(10*f+k), tilePixels, 8, 8, uint64(f)) == nil {
				t.Fatal("the atlas is full")
			}
		}
	}
	k20 := key(20)
	if a.get(&k20, 9) == nil {
		t.Fatal("a picture is missing")
	}
	// Older than 4 frames at frame 9: those of frames 1 to 5, but the one
	// frame 9 uses.
	if !a.evict(9, 4) {
		t.Error("nothing was evicted")
	}
	for f := 1; f <= 8; f++ {
		for k := 0; k < 2; k++ {
			_, has := a.entries[key(10*f+k)]
			if want := f > 5 || (f == 2 && k == 0); has != want {
				t.Errorf("after evicting what frames 1 to 5 used last: the picture %d of frame %d is there: %v", k, f, has)
			}
		}
	}
	// What the frame uses stays, whatever the age.
	k60 := key(60)
	a.get(&k60, 9)
	a.evict(9, 1)
	if len(a.entries) != 2 {
		t.Errorf("%d pictures left, want the 2 the frame uses", len(a.entries))
	}
	if a.evict(9, 1) {
		t.Error("an eviction that removes nothing reports otherwise")
	}
	// EndFrame: a picture goes after idleFrames frames without a use.
	a.reset()
	a.add(key(1), tilePixels, 8, 8, 1)
	a.add(key(2), tilePixels, 8, 8, 1)
	mark := uint64(0)
	for f := uint64(1); f <= idleFrames+2; f++ {
		k := key(2)
		a.get(&k, f) // used by every frame
		a.endFrame(mark)
		mark = f
		_, has := a.entries[key(1)]
		if want := f <= idleFrames+1; has != want {
			t.Fatalf("after %d frames ended, the unused picture is there: %v", f, has)
		}
	}
	if _, has := a.entries[key(2)]; !has || len(a.pages) != 1 {
		t.Errorf("the picture every frame uses is gone, or its page (%d pages)", len(a.pages))
	}
}

// TestEncodeUploadsOnlyNew checks that a frame uploads the pictures that
// are new to the atlases, and one that draws what the frames before drew,
// nothing.
func TestEncodeUploadsOnlyNew(t *testing.T) {
	scene := func(extra string) *gfx.DisplayList {
		return gfxtest.Build(func(b *gfx.Builder) {
			text(b, "SolA il oSi", 14, 8.3, 20, slate)
			text(b, "*%"+extra, 20, 8, 50, rose)
			b.FillPath(gfxtest.StarPath(60, 90, 20), gfx.NonZero, indigo)
			b.StrokePath(gfxtest.TightCurve(100, 60), gfx.NewStrokeStyle(3), emerald)
			b.PushClipPath(gfx.CirclePath(gfx.Pt(150, 30), 20), gfx.NonZero)
			b.FillRect(gfx.RectXYWH(120, 0, 60, 60), amber)
			b.PopClip()
			b.DrawImage(gfxtest.TestImage, gfx.Rect{}, gfx.RectXYWH(150, 70, 30, 30), gfx.SampleLinear)
		})
	}
	var e Encoder
	e.GlyphCache = raster.NewGlyphCache(1 << 20)
	sim := &atlasSim{t: t}
	size := image.Pt(200, 120)
	f := e.Encode(scene(""), size, Options{})
	first := append([]byte(nil), f.Instances...)
	// 5 distinct glyphs at these pens ("SolAi": the l and the o repeat at
	// other quarter pixels, 9 masks), 2 sprites, a star, a stroke, a mask.
	cov, col := 0, 0
	for _, u := range f.Passes[0].Uploads {
		if u.Atlas == AtlasCoverage {
			cov++
		} else {
			col++
		}
	}
	if col != 2 || cov < 5+3 || len(f.Images) != 1 || len(f.Unsupported) != 0 {
		t.Fatalf("the first frame uploads %d coverage pictures, %d sprites and %d images (unsupported %v)", cov, col, len(f.Images), f.Unsupported)
	}
	if f.Res.Base != 0 || f.Res.Next != 1 || f.Res.ID == 0 {
		t.Errorf("the first frame: %+v", f.Res)
	}
	if f.Atlases[AtlasCoverage] != (AtlasInfo{PageSize: 2048, Pages: 1}) || f.Atlases[AtlasColor] != (AtlasInfo{PageSize: 1024, Pages: 1}) {
		t.Errorf("atlases %+v", f.Atlases)
	}
	reads := sim.draw(f)
	id := f.Res.ID

	// The same list again, and a region of it: nothing is uploaded, the
	// resources do not change, the instances are the same.
	f = e.Encode(scene(""), size, Options{})
	if uploads(f) != 0 || len(f.Data) != 0 || len(f.Images) != 0 || f.Res != (ResourceSync{ID: id, Base: 1, Next: 1}) {
		t.Errorf("the second frame uploads %d pictures, %d bytes, %d images: %+v", uploads(f), len(f.Data), len(f.Images), f.Res)
	}
	if !bytes.Equal(f.Instances, first) {
		t.Error("the second frame has other instances")
	}
	if got := sim.draw(f); !reflect.DeepEqual(got, reads) {
		t.Error("the second frame reads other pixels")
	}
	f = e.Encode(scene(""), size, Options{Region: image.Rect(0, 0, 80, 60)})
	if uploads(f) != 0 || instances(f) == 0 || instances(f) >= len(first)/InstanceSize {
		t.Errorf("a region uploads %d pictures and has %d instances", uploads(f), instances(f))
	}

	// One more glyph: one more picture.
	f = e.Encode(scene("A"), size, Options{})
	if uploads(f) != 1 || f.Res != (ResourceSync{ID: id, Base: 1, Next: 2}) {
		t.Errorf("a frame with one new glyph uploads %d pictures: %+v", uploads(f), f.Res)
	}
	sim.draw(f)

	// After Trim everything is uploaded again, as new resources.
	e.Trim()
	f = e.Encode(scene(""), size, Options{})
	if uploads(f) != cov+col || len(f.Images) != 1 || f.Res.ID == id || f.Res.Base != 0 || f.Res.Next != 1 {
		t.Errorf("after Trim a frame uploads %d pictures and %d images: %+v", uploads(f), len(f.Images), f.Res)
	}
	if !bytes.Equal(f.Instances, first) {
		t.Error("after Trim the frame has other instances")
	}
}

// TestEncodeGlyphKeys checks that glyphs share a picture exactly when the
// rasterizer's cache shares their mask or sprite.
func TestEncodeGlyphKeys(t *testing.T) {
	count := func(scale float32, draw func(b *gfx.Builder)) (cov, col int) {
		e := Encoder{GlyphCache: raster.NewGlyphCache(1 << 20)}
		f := e.Encode(gfxtest.Build(draw), image.Pt(600, 200), Options{Scale: scale})
		if len(f.Unsupported) != 0 {
			t.Fatalf("unsupported: %v", f.Unsupported)
		}
		for i := range f.Passes {
			for _, u := range f.Passes[i].Uploads {
				if u.Atlas == AtlasCoverage {
					cov++
				} else {
					col++
				}
			}
		}
		return cov, col
	}
	o := func(b *gfx.Builder, size, x, y float32, p gfx.Paint) {
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: font, Size: size, Origin: gfx.Pt(x, y), Glyphs: []gfx.Glyph{{ID: gfxtest.GlyphO}}, Paint: p})
	}
	for _, c := range []struct {
		name     string
		scale    float32
		draw     func(b *gfx.Builder)
		cov, col int
	}{
		{"whole pixels apart", 1, func(b *gfx.Builder) {
			for i := 0; i < 8; i++ {
				o(b, 20, 10+float32(i)*30, 40+float32(i)*3, slate)
			}
		}, 1, 0},
		{"every quarter pixel", 1, func(b *gfx.Builder) {
			for i := 0; i < 9; i++ {
				o(b, 20, 10+float32(i)*30.25, 40, slate) // 0, 1/4, 1/2, 3/4, 0, ...
			}
		}, 4, 0},
		{"between quarter pixels, and any y", 1, func(b *gfx.Builder) {
			for i := 0; i < 9; i++ {
				o(b, 20, 10+float32(i)*30.03, 40+float32(i)*0.3, slate) // rounds to the quarter 0 or 1
			}
		}, 2, 0},
		{"sizes", 1, func(b *gfx.Builder) {
			for i := 0; i < 5; i++ {
				o(b, 12+float32(i), 10+float32(i)*30, 40, slate)
			}
		}, 5, 0},
		{"the scale is part of the size", 1.5, func(b *gfx.Builder) {
			o(b, 20, 10, 40, slate)
			o(b, 30, 60, 40, slate)
		}, 2, 0},
		{"paints share a mask", 1, func(b *gfx.Builder) {
			o(b, 20, 10, 40, slate)
			o(b, 20, 40, 40, rose.WithAlpha(0.5))
			o(b, 20, 70, 40, gfx.LinearGradient{Start: gfx.Pt(70, 20), End: gfx.Pt(90, 40), Stops: stops(sky, rose)})
		}, 1, 0},
		{"a rotation is a path per place", 1, func(b *gfx.Builder) {
			b.PushTransform(gfx.Rotate(0.1))
			o(b, 20, 20, 40, slate)
			o(b, 20, 50, 40, slate)
			b.PopTransform()
		}, 2, 0},
		{"a sprite with the foreground per color, without it for all", 1, func(b *gfx.Builder) {
			for i, col := range []gfx.Color{slate, rose, rose.WithAlpha(0.5)} {
				b.DrawGlyphRun(font.Run("*%", 20, 10+float32(i)*50, 40, col))
			}
		}, 0, 3 + 1},
	} {
		if cov, col := count(c.scale, c.draw); cov != c.cov || col != c.col {
			t.Errorf("%s: %d coverage pictures and %d sprites, want %d and %d", c.name, cov, col, c.cov, c.col)
		}
	}
}

// TestEncodePathCache checks which paths share a picture: the same shape,
// by content, under the same transform up to whole pixels.
func TestEncodePathCache(t *testing.T) {
	star := gfxtest.StarPath(30, 30, 20)
	round := gfx.NewStrokeStyle(3)
	round.Join = gfx.JoinRound
	dashed := gfx.NewStrokeStyle(3)
	dashed.Dashes = []float32{4, 2}
	var e Encoder
	size := image.Pt(300, 300)
	frame := func(draw func(b *gfx.Builder)) int {
		t.Helper()
		f := e.Encode(gfxtest.Build(draw), size, Options{})
		if len(f.Unsupported) != 0 {
			t.Fatalf("unsupported: %v", f.Unsupported)
		}
		return uploads(f)
	}
	at := func(b *gfx.Builder, m gfx.Affine, draw func()) {
		b.PushTransform(m)
		draw()
		b.PopTransform()
	}
	if n := frame(func(b *gfx.Builder) {
		// Whole pixels apart, twice in a frame, and a copy of the path.
		b.FillPath(star, gfx.NonZero, slate)
		at(b, gfx.Translate(100, 37), func() { b.FillPath(star, gfx.NonZero, rose) })
		at(b, gfx.Translate(-12, 200), func() { b.FillPath(star.Clone(), gfx.NonZero, slate) })
	}); n != 1 {
		t.Errorf("a path at three places whole pixels apart: %d pictures, want 1", n)
	}
	for _, c := range []struct {
		name string
		draw func(b *gfx.Builder)
		want int
	}{
		{"the same again, elsewhere", func(b *gfx.Builder) {
			at(b, gfx.Translate(7, 9), func() { b.FillPath(star, gfx.NonZero, amber) })
		}, 0},
		{"the other rule", func(b *gfx.Builder) { b.FillPath(star, gfx.EvenOdd, slate) }, 1},
		{"a fraction of a pixel away", func(b *gfx.Builder) {
			at(b, gfx.Translate(40.5, 0), func() { b.FillPath(star, gfx.NonZero, slate) })
			at(b, gfx.Translate(140.5, 20), func() { b.FillPath(star, gfx.NonZero, slate) })
		}, 1},
		{"scaled", func(b *gfx.Builder) {
			at(b, gfx.Scale(1.5, 1.5), func() { b.FillPath(star, gfx.NonZero, slate) })
		}, 1},
		{"a stroke, and the same stroke", func(b *gfx.Builder) {
			b.StrokePath(star, round, slate)
			at(b, gfx.Translate(100, 100), func() { b.StrokePath(star, round, rose) })
		}, 1},
		{"another style", func(b *gfx.Builder) { b.StrokePath(star, dashed, slate) }, 1},
		{"an equal style, made again", func(b *gfx.Builder) {
			again := gfx.NewStrokeStyle(3)
			again.Dashes = []float32{4, 2}
			b.StrokePath(star, again, slate)
		}, 0},
		{"another path", func(b *gfx.Builder) { b.FillPath(gfxtest.StarPath(30, 30, 20.5), gfx.NonZero, slate) }, 1},
	} {
		if n := frame(c.draw); n != c.want {
			t.Errorf("%s: %d pictures, want %d", c.name, n, c.want)
		}
	}
	// A shape that turns a little every frame is a new picture every frame.
	for i := 0; i < 3; i++ {
		if n := frame(func(b *gfx.Builder) {
			at(b, gfx.RotateAbout(0.1*float32(i+1), gfx.Pt(150, 150)), func() { b.FillPath(star, gfx.NonZero, slate) })
		}); n != 1 {
			t.Errorf("a rotating path, frame %d: %d pictures, want 1", i, n)
		}
	}
	// The tolerance of the flattening is part of what a picture is.
	old := raster.FlattenTolerance
	raster.FlattenTolerance = 0.01
	n := frame(func(b *gfx.Builder) { b.FillPath(gfx.CirclePath(gfx.Pt(50, 50), 30), gfx.NonZero, slate) })
	raster.FlattenTolerance = old
	if m := frame(func(b *gfx.Builder) { b.FillPath(gfx.CirclePath(gfx.Pt(50, 50), 30), gfx.NonZero, slate) }); n != 1 || m != 1 {
		t.Errorf("a path under two flattening tolerances: %d and %d pictures, want 1 and 1", n, m)
	}
}

// coverageOf executes a frame of one filled shape on the simulated
// atlases and returns the coverage its instances give the pixels of size:
// what an instance reads from the atlas, or 255 over a plain rectangle. It
// fails when two instances draw the same pixel, and returns the pixels
// uploaded.
func coverageOf(t *testing.T, sim *atlasSim, f *Frame, size image.Point) (cov []uint8, uploaded int) {
	t.Helper()
	cov = make([]uint8, size.X*size.Y)
	seen := make([]bool, size.X*size.Y)
	reads := sim.draw(f)
	n := 0
	for pi := range f.Passes {
		for _, u := range f.Passes[pi].Uploads {
			uploaded += u.Rect.Dx() * u.Rect.Dy()
		}
		for _, b := range f.Passes[pi].Batches {
			for i := b.First; i < b.First+b.Count; i++ {
				rd := reads[n]
				n++
				vis := rd.quad.Intersect(b.Scissor)
				plain := head(f, i)[0]&FlagKindMask == 0
				if plain {
					// A plain rectangle: the piece itself.
					if sh, bd, org := floats(f, i, SlotShape), floats(f, i, SlotBounds), floats(f, i, SlotOrigin); sh != [4]float32{bd[0] - org[0], bd[1] - org[1], bd[2] - org[0], bd[3] - org[1]} {
						t.Errorf("instance %d: a covered piece fills %v, its quad is %v", i, sh, bd)
					}
				}
				k := 0
				for y := vis.Min.Y; y < vis.Max.Y; y++ {
					for x := vis.Min.X; x < vis.Max.X; x++ {
						if seen[y*size.X+x] {
							t.Fatalf("instance %d draws the pixel (%d, %d) another one drew", i, x, y)
						}
						seen[y*size.X+x] = true
						if plain {
							cov[y*size.X+x] = 255
						} else {
							cov[y*size.X+x] = rd.own[k]
						}
						k++
					}
				}
			}
		}
	}
	return cov, uploaded
}

// TestEncodePieces checks that a filled path drawn by pieces (pieces.go)
// has the rasterizer's coverage at every pixel, in a full render and in
// regions, with any size of tile; that the pieces do not overlap; that
// what is inside a shape is plain rectangles and what is outside is not
// drawn; and that the border of a panel uploads its edges, not its bounds.
func TestEncodePieces(t *testing.T) {
	const w, h = 400, 400
	var ring, border, cross, diag gfx.Path
	ring.AddRect(gfx.RectXYWH(10.5, 10.5, 300, 300))
	ring.AddRect(gfx.RectXYWH(140, 140, 40, 40))
	// The band of a 1.5 pixel border with round corners, as core records
	// it: the inner shape runs the other way.
	border.AddRRect(gfx.NewRRect(gfx.RectXYWH(20.25, 30.75, 350, 330), 41.2))
	inner := gfx.NewRRect(gfx.RectXYWH(21.75, 32.25, 347, 327), 39.7)
	var back gfx.Path
	back.AddRRect(inner)
	mirror := gfx.Affine{-1, 0, 0, 1, inner.Rect.X0 + inner.Rect.X1, 0}
	border = appendPath(border, back.Transform(mirror))
	cross.AddRect(gfx.RectXYWH(0, 90, 192, 12))
	cross.AddRect(gfx.RectXYWH(90, 0, 12, 192))
	diag.MoveTo(gfx.Pt(5, 8))
	diag.LineTo(gfx.Pt(390, 371))
	diag.LineTo(gfx.Pt(380, 395))
	diag.Close()
	var blob, loop gfx.Path
	blob.MoveTo(gfx.Pt(20, 120))
	blob.QuadTo(gfx.Pt(60, 0), gfx.Pt(120, 80))
	blob.QuadTo(gfx.Pt(390, 160), gfx.Pt(100, 200))
	blob.QuadTo(gfx.Pt(0, 220), gfx.Pt(20, 120))
	blob.Close()
	loop.MoveTo(gfx.Pt(100, 300))
	loop.CubicTo(gfx.Pt(500, 20), gfx.Pt(-100, 40), gfx.Pt(320, 380)) // open: closed by the fill
	// Vertical sides whose rows are not all the same: a wide rectangle
	// crosses a tall one, another starts inside its side's cells, and two
	// sides meet on one line.
	var stack gfx.Path
	stack.AddRect(gfx.RectXYWH(100.5, 10, 3.25, 380))
	stack.AddRect(gfx.RectXYWH(20, 150.5, 300, 97.25))
	stack.AddRect(gfx.RectXYWH(103.75, 70.3, 40, 200))
	stack.AddRect(gfx.RectXYWH(300, 20, 60.5, 100))
	stack.AddRect(gfx.RectXYWH(300, 137, 60.5, 240))
	scenes := []struct {
		name string
		path gfx.Path
		rule gfx.FillRule
		// most is the largest share of the target's pixels the shape may
		// upload at the default page size (0: not checked).
		most float64
	}{
		{"a ring of rectangles", ring, gfx.EvenOdd, 0.3},
		{"a border", border, gfx.NonZero, 0.2},
		{"a cross", cross, gfx.NonZero, 0.2},
		{"rectangles across each other", stack, gfx.NonZero, 0},
		{"rectangles across each other, even-odd", stack, gfx.EvenOdd, 0},
		{"a thin triangle", diag, gfx.NonZero, 0.3},
		{"a blob of quadratic curves", blob, gfx.NonZero, 0},
		{"a cubic loop", loop, gfx.EvenOdd, 0},
		{"a disc", gfx.CirclePath(gfx.Pt(200.3, 190.7), 180), gfx.NonZero, 0.3},
		{"a star", gfxtest.StarPath(200, 200, 190), gfx.EvenOdd, 0},
		{"a small star", gfxtest.StarPath(30, 30, 20.5), gfx.NonZero, 0},
	}
	for _, sc := range scenes {
		dl := gfxtest.Build(func(b *gfx.Builder) { b.FillPath(sc.path, sc.rule, slate) })
		want := make([]uint8, w*h)
		var m raster.Masker
		m.Fill(want, image.Rect(0, 0, w, h), gfx.Identity, sc.path, sc.rule)
		for _, page := range []int{0, 128, 48, 16} {
			e := Encoder{Limits: Limits{CoveragePage: page, CoveragePages: 64}}
			sim := &atlasSim{t: t}
			f := e.Encode(dl, image.Pt(w, h), Options{})
			got, up := coverageOf(t, sim, f, image.Pt(w, h))
			if len(f.Unsupported) > 0 {
				t.Fatalf("%s, pages of %d: %v", sc.name, page, f.Unsupported)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%s, pages of %d: the pixel (%d, %d) has the coverage %d, the rasterizer's is %d", sc.name, page, i%w, i/w, got[i], want[i])
				}
			}
			if page == 0 && sc.most > 0 && float64(up) > sc.most*w*h {
				t.Errorf("%s: %d pixels uploaded, want at most %.0f%% of the target's %d", sc.name, up, 100*sc.most, w*h)
			}
			// Regions, in an Encoder of their own and in the one that drew
			// the whole: the same pixels.
			for _, region := range []image.Rectangle{image.Rect(0, 0, 70, 70), image.Rect(150, 20, 260, 45), image.Rect(190, 190, 215, 330), image.Rect(300, 300, 400, 400)} {
				for _, enc := range []*Encoder{{Limits: e.Limits}, &e} {
					rs := sim
					if enc != &e {
						rs = &atlasSim{t: t}
					}
					f := enc.Encode(dl, image.Pt(w, h), Options{Region: region})
					got, _ := coverageOf(t, rs, f, image.Pt(w, h))
					for y := region.Min.Y; y < region.Max.Y; y++ {
						for x := region.Min.X; x < region.Max.X; x++ {
							if got[y*w+x] != want[y*w+x] {
								t.Fatalf("%s, pages of %d, the region %v: the pixel (%d, %d) has the coverage %d, the rasterizer's is %d", sc.name, page, region, x, y, got[y*w+x], want[y*w+x])
							}
						}
					}
				}
			}
		}
	}

	// What is inside is plain rectangles, without the atlas; what is
	// outside (the hole, the corners between a cross's arms) is not drawn.
	e := Encoder{}
	f := e.Encode(gfxtest.Build(func(b *gfx.Builder) { b.FillPath(ring, gfx.EvenOdd, slate) }), image.Pt(w, h), Options{})
	plain, area := 0, 0
	for i := range instances(f) {
		bd := floats(f, i, SlotBounds)
		area += int(bd[2]-bd[0]) * int(bd[3]-bd[1])
		if head(f, i)[0]&FlagKindMask == 0 {
			plain += int(bd[2]-bd[0]) * int(bd[3]-bd[1])
		}
	}
	if plain < 60000 || area > 301*301-10*10 {
		t.Errorf("a ring of 301 pixels with a hole of 40: %d pixels of plain rectangles and %d drawn, want most of it plain and the middle of the hole left out", plain, area)
	}
	// A region draws the pieces on it only, and uploads no more.
	e2 := Encoder{}
	f = e2.Encode(gfxtest.Build(func(b *gfx.Builder) { b.FillPath(border, gfx.NonZero, slate) }), image.Pt(w, h), Options{Region: image.Rect(100, 100, 200, 200)})
	if instances(f) != 0 || uploads(f) != 0 {
		t.Errorf("a region inside a border: %d instances and %d uploads, want none", instances(f), uploads(f))
	}
	// A shape far larger than the target: only what is on it.
	huge := gfxtest.Build(func(b *gfx.Builder) { b.FillPath(gfx.CirclePath(gfx.Pt(0, 0), 5000), gfx.NonZero, slate) })
	f = e2.Encode(huge, image.Pt(100, 100), Options{})
	if n := instances(f); n < 1 || n > 4 || uploads(f) != 0 {
		t.Errorf("a disc of 10000 pixels over a target of 100: %d instances, %d uploads, want up to 4 covered pieces", n, uploads(f))
	}
	// A shape that moves by a part of a pixel every frame (an animated
	// row) is a new picture every frame, of its edges only.
	for i := range 3 {
		e3 := &e2
		dl := gfxtest.Build(func(b *gfx.Builder) {
			b.PushTransform(gfx.Translate(0, 0.3*float32(i+1)))
			b.FillPath(border, gfx.NonZero, slate)
			b.PopTransform()
		})
		f := e3.Encode(dl, image.Pt(w, h), Options{})
		if len(f.Data) == 0 || len(f.Data) > w*h/5 {
			t.Errorf("a border moved by %.1f pixel: %d bytes uploaded, want its edges (under %d)", 0.3*float32(i+1), len(f.Data), w*h/5)
		}
	}
}

// appendPath returns a with the elements of b after its own.
func appendPath(a, b gfx.Path) gfx.Path {
	for el := range b.Elements() {
		switch el.Verb {
		case gfx.MoveTo:
			a.MoveTo(el.Pts[0])
		case gfx.LineTo:
			a.LineTo(el.Pts[0])
		case gfx.QuadTo:
			a.QuadTo(el.Pts[0], el.Pts[1])
		case gfx.CubicTo:
			a.CubicTo(el.Pts[0], el.Pts[1], el.Pts[2])
		case gfx.Close:
			a.Close()
		}
	}
	return a
}

// TestEncodeFlush checks what happens when a frame needs more pictures
// than the atlas holds: the frame goes on in a new pass that starts from
// an empty atlas, and every instance still reads its own pixels.
func TestEncodeFlush(t *testing.T) {
	dl := gfxtest.Build(func(b *gfx.Builder) {
		for i := 0; i < 6; i++ {
			text(b, "SolA il oSi Aolis", 14+float32(i), 4.3+float32(i)*0.4, 20+float32(i)*24, slate)
		}
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(4.5, 4.5, 200, 160), 30))
		b.PushClipPath(gfx.CirclePath(gfx.Pt(110, 90), 96), gfx.NonZero)
		for i := 0; i < 5; i++ {
			text(b, "*%@", 22+float32(i)*3, 150.2, 30+float32(i)*30, rose)
			b.FillPath(gfxtest.StarPath(40+float32(i)*30.3, 150, 22), gfx.EvenOdd, indigo)
			b.StrokePath(gfxtest.TightCurve(30+float32(i)*20.7, 40), gfx.NewStrokeStyle(2+float32(i)), emerald)
		}
		b.PopClip()
		b.PopClip()
	})
	size := image.Pt(240, 180)
	// The reference: the same page size, so the same tiles, and room for
	// everything.
	big := Encoder{Limits: Limits{CoveragePage: 64, CoveragePages: 1000, ColorPage: 64, ColorPages: 1000}, GlyphCache: raster.NewGlyphCache(1 << 22)}
	bf := big.Encode(dl, size, Options{})
	if len(bf.Passes) != 1 || len(bf.Unsupported) != 0 {
		t.Fatalf("the reference: %d passes, unsupported %v", len(bf.Passes), bf.Unsupported)
	}
	want := (&atlasSim{t: t}).draw(bf)

	for _, pages := range []int{1, 2, 5} {
		e := Encoder{Limits: Limits{CoveragePage: 64, CoveragePages: pages, ColorPage: 64, ColorPages: pages}, GlyphCache: big.GlyphCache}
		sim := &atlasSim{t: t}
		for round := 0; round < 3; round++ {
			f := e.Encode(dl, size, Options{})
			if len(f.Unsupported) != 0 {
				t.Fatalf("%d pages: unsupported %v", pages, f.Unsupported)
			}
			if f.Atlases[AtlasCoverage].Pages > pages || f.Atlases[AtlasColor].Pages > pages {
				t.Fatalf("%d pages: the frame has %+v", pages, f.Atlases)
			}
			if pages == 1 && len(f.Passes) < 3 {
				t.Errorf("one page of 64 pixels: %d passes, want several", len(f.Passes))
			}
			next := 0
			for _, p := range f.Passes {
				for _, b := range p.Batches {
					if b.First != next {
						t.Fatalf("%d pages: a batch starts at instance %d after %d", pages, b.First, next)
					}
					next += b.Count
				}
			}
			if next != instances(f) || instances(f) != instances(bf) {
				t.Fatalf("%d pages: %d instances in batches, %d in the frame, %d in the reference", pages, next, instances(f), instances(bf))
			}
			got := sim.draw(f)
			for i := range got {
				if !reflect.DeepEqual(got[i], want[i]) {
					t.Fatalf("%d pages, round %d: instance %d reads other pixels than in an atlas with room (quad %v, reference %v)", pages, round, i, got[i].quad, want[i].quad)
				}
			}
			if f.Res.Next != f.Res.Base+1 {
				t.Errorf("%d pages: a frame that uploads leaves the resources as they were: %+v", pages, f.Res)
			}
		}
	}

	// Two frames that alternate in an atlas with room for either but not
	// for both: each evicts what only the other used (or flushes, when
	// what is left is too scattered), and every instance reads its own
	// pixels all the same.
	var lists [2]*gfx.DisplayList
	var refs [2][]instanceRead
	limit := 0
	for k, em := range []float32{15, 22} {
		lists[k] = gfxtest.Build(func(b *gfx.Builder) {
			for i := 0; i < 6; i++ {
				text(b, "oSi Aolis SolA il", em+float32(i), 4.1+float32(i)*0.4, 22+float32(i)*26, slate)
			}
		})
		big.Trim()
		f := big.Encode(lists[k], size, Options{})
		refs[k] = (&atlasSim{t: t}).draw(f)
		limit = max(limit, f.Atlases[AtlasCoverage].Pages)
	}
	e := Encoder{Limits: Limits{CoveragePage: 64, CoveragePages: limit}, GlyphCache: big.GlyphCache}
	sim := &atlasSim{t: t}
	evicted := false
	for round := 0; round < 4; round++ {
		for k, list := range lists {
			f := e.Encode(list, size, Options{})
			if len(f.Unsupported) != 0 || f.Atlases[AtlasCoverage].Pages > limit {
				t.Fatalf("round %d, list %d: unsupported %v, %d pages of %d", round, k, f.Unsupported, f.Atlases[AtlasCoverage].Pages, limit)
			}
			if got := sim.draw(f); !reflect.DeepEqual(got, refs[k]) {
				t.Fatalf("round %d, list %d: the instances read other pixels than in an atlas with room", round, k)
			}
			evicted = evicted || (round > 0 && uploads(f) > 0)
			// The same frame again finds everything, unless a flush dropped
			// what the frame drew before it: then it brings that back, and
			// the frame after finds everything.
			flushed := len(f.Passes) > 1
			f = e.Encode(list, size, Options{})
			if got := sim.draw(f); !reflect.DeepEqual(got, refs[k]) {
				t.Fatalf("round %d, list %d, again: the instances read other pixels", round, k)
			}
			if uploads(f) != 0 && !flushed {
				t.Errorf("round %d, list %d: drawn again, it uploads %d pictures", round, k, uploads(f))
			}
			if f = e.Encode(list, size, Options{}); uploads(f) != 0 {
				t.Errorf("round %d, list %d: drawn a third time, it uploads %d pictures", round, k, uploads(f))
			}
		}
	}
	if !evicted {
		t.Error("nothing was uploaded after the first round: both lists fit, and the test tests nothing")
	}
}

// TestEncodeClipStack checks how clips are carried: by the scissor, by
// the instance, by a mask, and what a mask holds.
func TestEncodeClipStack(t *testing.T) {
	fill := func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(0, 0, 200, 200), gfx.Black) }
	rr := gfx.NewRRect(gfx.RectXYWH(20.5, 20.5, 120, 100), 24)
	star := gfxtest.StarPath(80, 70, 60)
	disc := gfx.CirclePath(gfx.Pt(90, 80), 50)
	const size = 200
	type want struct {
		clip  uint32 // the instance's clip slot
		masks int    // pictures uploaded
		mask  bool   // the instance reads a mask
	}
	var k raster.Masker
	cover := func(shapes ...any) []uint8 {
		// The product of the coverages, as the rasterizer nests them.
		rect := image.Rect(0, 0, size, size)
		var acc []uint8
		for _, s := range shapes {
			buf := make([]uint8, size*size)
			switch s := s.(type) {
			case gfx.Path:
				k.Fill(buf, rect, gfx.Identity, s, gfx.NonZero)
			case gfx.RRect:
				k.RRect(buf, rect, gfx.Identity, s)
			}
			if acc == nil {
				acc = buf
				continue
			}
			for i := range acc {
				acc[i] = mul8(buf[i], acc[i])
			}
		}
		return acc
	}
	inner := gfx.NewRRect(gfx.RectXYWH(20.25, 30.25, 60, 60), 10)
	third := gfx.NewRRect(gfx.RectXYWH(30.25, 30.25, 100, 90), 30)
	small := gfx.CirclePath(gfx.Pt(80, 80), 48)
	for _, c := range []struct {
		name string
		// draw pushes clips, calls fill inside them and pops them.
		draw func(b *gfx.Builder, fill func())
		want want
		// The coverage the mask holds, or nil.
		mask []uint8
	}{
		{"a rounded rectangle", func(b *gfx.Builder, fill func()) {
			b.PushClipRRect(rr)
			fill()
			b.PopClip()
		}, want{ClipDevice, 0, false}, nil},
		{"a rotated rounded rectangle", func(b *gfx.Builder, fill func()) {
			b.PushTransform(gfx.RotateAbout(0.2, gfx.Pt(80, 70)))
			b.PushClipRRect(rr)
			fill()
			b.PopClip()
			b.PopTransform()
		}, want{ClipLocal, 0, false}, nil},
		{"a path", func(b *gfx.Builder, fill func()) {
			b.PushClipPath(star, gfx.NonZero)
			fill()
			b.PopClip()
		}, want{0, 1, true}, cover(star)},
		{"a path in a rounded rectangle", func(b *gfx.Builder, fill func()) {
			b.PushClipRRect(rr)
			b.PushClipPath(star, gfx.NonZero)
			fill()
			b.PopClip()
			b.PopClip()
		}, want{ClipDevice, 1, true}, cover(star)},
		{"a rounded rectangle in a path", func(b *gfx.Builder, fill func()) {
			b.PushClipPath(star, gfx.NonZero)
			b.PushClipRRect(rr)
			fill()
			b.PopClip()
			b.PopClip()
		}, want{ClipDevice, 1, true}, cover(star)},
		{"a path in a path", func(b *gfx.Builder, fill func()) {
			b.PushClipPath(star, gfx.NonZero)
			b.PushClipPath(disc, gfx.NonZero)
			fill()
			b.PopClip()
			b.PopClip()
		}, want{0, 1, true}, cover(star, disc)},
		{"two rounded rectangles that share edges", func(b *gfx.Builder, fill func()) {
			b.PushClipRRect(rr)
			b.PushClipRRect(inner)
			fill()
			b.PopClip()
			b.PopClip()
		}, want{ClipDevice, 1, true}, cover(inner)},
		{"a rounded rectangle in the middle of another", func(b *gfx.Builder, fill func()) {
			b.PushClipRRect(rr)
			b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(40.5, 50.5, 60, 40), 10))
			fill()
			b.PopClip()
			b.PopClip()
		}, want{ClipDevice, 0, false}, nil},
		{"three paths and two rounded rectangles", func(b *gfx.Builder, fill func()) {
			b.PushClipPath(star, gfx.NonZero)
			b.PushClipRRect(rr)
			b.PushClipPath(disc, gfx.NonZero)
			b.PushClipRRect(third)
			b.PushClipPath(small, gfx.NonZero)
			fill()
			for i := 0; i < 5; i++ {
				b.PopClip()
			}
		}, want{ClipDevice, 1, true}, cover(star, disc, third, small)},
		{"a scissor inside a path", func(b *gfx.Builder, fill func()) {
			b.PushClipPath(disc, gfx.NonZero)
			b.PushClipRect(gfx.RectXYWH(60, 60, 50, 50))
			fill()
			b.PopClip()
			b.PopClip()
		}, want{0, 1, true}, cover(disc)},
	} {
		var e Encoder
		dl := gfxtest.Build(func(b *gfx.Builder) { c.draw(b, func() { fill(b) }) })
		f := e.Encode(dl, image.Pt(size, size), Options{})
		if len(f.Unsupported) != 0 || instances(f) == 0 {
			t.Errorf("%s: %d instances, unsupported %v", c.name, instances(f), f.Unsupported)
			continue
		}
		// The fill is one instance, or its parts: the part where the mask
		// covers entirely, which does not read it, and the pieces along
		// the mask's tiles. A part carries the clip unless it lies where
		// the clip covers entirely.
		got := want{masks: uploads(f)}
		if got.masks > 1 {
			got.masks = 1 // the tiles of one mask
		}
		for i := range instances(f) {
			flags := head(f, i)[0]
			if clip := flags >> FlagClipShift & 3; clip != 0 {
				if got.clip != 0 && got.clip != clip {
					t.Errorf("%s: the parts carry clips of two kinds", c.name)
				}
				got.clip = clip
			}
			got.mask = got.mask || flags&FlagMask != 0
		}
		if got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
			continue
		}
		if c.mask == nil {
			continue
		}
		// Every pixel the mask leaves is drawn once, by a part that reads
		// the mask's value there, or by one that reads none where it is
		// 255.
		sim := &atlasSim{t: t}
		sc := f.Passes[0].Batches[0].Scissor
		drawn := make([]bool, size*size)
		for n, rd := range sim.draw(f) {
			vis := rd.quad.Intersect(sc)
			i := 0
			for y := vis.Min.Y; y < vis.Max.Y; y++ {
				for x := vis.Min.X; x < vis.Max.X; x++ {
					got, w := uint8(255), c.mask[y*size+x]
					if head(f, n)[0]&FlagMask != 0 {
						got = rd.mask[i]
					}
					if got != w {
						t.Fatalf("%s: the mask of pixel (%d, %d) is %d, the rasterizer's nested clips give %d", c.name, x, y, got, w)
					}
					if drawn[y*size+x] {
						t.Fatalf("%s: the pixel (%d, %d) is drawn twice", c.name, x, y)
					}
					drawn[y*size+x] = true
					i++
				}
			}
		}
		for y := sc.Min.Y; y < sc.Max.Y; y++ {
			for x := sc.Min.X; x < sc.Max.X; x++ {
				if c.mask[y*size+x] != 0 && !drawn[y*size+x] {
					t.Fatalf("%s: the pixel (%d, %d), where the mask is %d, is not drawn", c.name, x, y, c.mask[y*size+x])
				}
			}
		}
		// A second frame finds the mask again.
		if f = e.Encode(dl, image.Pt(size, size), Options{}); uploads(f) != 0 {
			t.Errorf("%s: the second frame uploads %d pictures", c.name, uploads(f))
		}
	}
}

// TestEncodeImages checks the textures of images: created once, named by
// the batches that read them, released when unused or over the budget.
func TestEncodeImages(t *testing.T) {
	a, b2 := gfxtest.TestImage, photo
	both := gfxtest.Build(func(b *gfx.Builder) {
		b.FillRect(gfx.RectXYWH(0, 0, 10, 10), slate)
		b.DrawImage(a, gfx.Rect{}, gfx.RectXYWH(10, 10, 6, 6), gfx.SampleNearest)
		b.FillRect(gfx.RectXYWH(0, 20, 10, 10), slate)
		b.DrawImage(a, gfx.Rect{X0: 1, Y0: 2, X1: 5, Y1: 4}, gfx.RectXYWH(30.5, 10, 12, 8), gfx.SampleLinear)
		b.DrawImage(b2, gfx.Rect{}, gfx.RectXYWH(50, 10, 48, 32), gfx.SampleLinear)
		b.FillRect(gfx.RectXYWH(0, 40, 10, 10), slate)
		b.DrawImage(a, gfx.Rect{}, gfx.RectXYWH(10, 60, 6, 6), gfx.SampleNearest)
	})
	onlyA := gfxtest.Build(func(b *gfx.Builder) {
		b.DrawImage(a, gfx.Rect{}, gfx.RectXYWH(10, 10, 6, 6), gfx.SampleNearest)
	})
	onlyB := gfxtest.Build(func(b *gfx.Builder) {
		b.DrawImage(b2, gfx.Rect{}, gfx.RectXYWH(50, 10, 48, 32), gfx.SampleLinear)
	})
	size := image.Pt(120, 80)
	var e Encoder
	f := e.Encode(both, size, Options{})
	if len(f.Images) != 2 || f.Images[0].ID != a.ID() || f.Images[1].ID != b2.ID() || f.Images[1].Width != 96 || f.Images[1].Height != 64 || len(f.Images[1].Pix) != 4*96*64 {
		t.Fatalf("the first frame's images: %+v", f.Images)
	}
	// Instances that read no image share the draw of those that do; the
	// image changes the draw.
	wantBatches := []Batch{
		{Pipeline: PipelineQuad, Scissor: image.Rectangle{Max: size}, First: 0, Count: 4, Image: a.ID()},
		{Pipeline: PipelineQuad, Scissor: image.Rectangle{Max: size}, First: 4, Count: 2, Image: b2.ID()},
		{Pipeline: PipelineQuad, Scissor: image.Rectangle{Max: size}, First: 6, Count: 1, Image: a.ID()},
	}
	if got := batches(f); !reflect.DeepEqual(got, wantBatches) {
		t.Errorf("batches %+v, want %+v", got, wantBatches)
	}
	// One to one at whole pixels: the image coordinates of a pixel center
	// are those of the image pixel's center, clamped to its indices.
	if h := head(f, 1)[0]; h>>FlagPaintShift&7 != PaintImageNearest {
		t.Errorf("flags %#x, want a nearest image", h)
	}
	if u, v, lim := floats(f, 1, SlotPaint), floats(f, 1, SlotPaint+1), floats(f, 1, SlotPaint+2); u != [4]float32{1, 0, 0, 0} || v != [4]float32{0, 1, 0, 0} || lim != [4]float32{0, 5, 0, 5} {
		t.Errorf("a 1:1 image: u %v, v %v, limits %v", u, v, lim)
	}
	// A part of the image, three times its size: the centers of its pixels
	// bound the samples.
	if h := head(f, 3)[0]; h>>FlagPaintShift&7 != PaintImageLinear {
		t.Errorf("flags %#x, want a bilinear image", h)
	}
	if u, lim := floats(f, 3, SlotPaint), floats(f, 3, SlotPaint+2); u != [4]float32{1.0 / 3, 0, 1 - 0.5/3, 0} || lim != [4]float32{1, 4, 2, 3} {
		t.Errorf("a part of an image: u %v, limits %v", u, lim)
	}
	if f.Res.Next != f.Res.Base+1 {
		t.Errorf("a frame with new images leaves the resources as they were: %+v", f.Res)
	}

	if f = e.Encode(both, size, Options{}); len(f.Images) != 0 || len(f.Releases) != 0 || f.Res.Next != f.Res.Base {
		t.Errorf("the second frame: %d images, releases %v, %+v", len(f.Images), f.Releases, f.Res)
	}
	// Frames that draw only one image: the other goes after idleFrames
	// frames, and the frame after tells the device.
	for i := 0; i <= idleFrames; i++ {
		e.EndFrame()
		if f = e.Encode(onlyA, size, Options{}); len(f.Releases) != 0 || len(f.Images) != 0 {
			t.Fatalf("frame %d without the image: releases %v, %d images", i, f.Releases, len(f.Images))
		}
	}
	e.EndFrame()
	if f = e.Encode(onlyA, size, Options{}); !reflect.DeepEqual(f.Releases, []uint64{b2.ID()}) || f.Res.Next != f.Res.Base+1 {
		t.Fatalf("after %d frames without the image: releases %v, %+v", idleFrames+1, f.Releases, f.Res)
	}
	if f = e.Encode(both, size, Options{}); len(f.Images) != 1 || f.Images[0].ID != b2.ID() || len(f.Releases) != 0 {
		t.Errorf("the image drawn again: images %+v, releases %v", f.Images, f.Releases)
	}
	// Given up, and drawn by the very next frame: the device still has it.
	for i := 0; i <= idleFrames+1; i++ {
		e.Encode(onlyA, size, Options{})
		e.EndFrame()
	}
	if f = e.Encode(both, size, Options{}); len(f.Images) != 0 || len(f.Releases) != 0 {
		t.Errorf("an image given up and drawn by the next frame: images %+v, releases %v", f.Images, f.Releases)
	}

	// A budget smaller than the two: a frame that draws both keeps both,
	// and a frame that draws one releases the other.
	e = Encoder{Limits: Limits{ImageBytes: int64(len(b2.Pix()))}}
	if f = e.Encode(both, size, Options{}); len(f.Images) != 2 || len(f.Releases) != 0 {
		t.Errorf("over the budget in one frame: %d images, releases %v", len(f.Images), f.Releases)
	}
	e = Encoder{Limits: Limits{ImageBytes: int64(len(b2.Pix()))}}
	if f = e.Encode(onlyA, size, Options{}); len(f.Images) != 1 {
		t.Fatalf("%d images", len(f.Images))
	}
	if f = e.Encode(onlyB, size, Options{}); len(f.Images) != 1 || !reflect.DeepEqual(f.Releases, []uint64{a.ID()}) {
		t.Errorf("over the budget across frames: %d images, releases %v, want the first image released", len(f.Images), f.Releases)
	}
	if f = e.Encode(onlyA, size, Options{}); len(f.Images) != 1 || !reflect.DeepEqual(f.Releases, []uint64{b2.ID()}) {
		t.Errorf("and back: %d images, releases %v", len(f.Images), f.Releases)
	}
}

// TestEncodeEndFrame checks that what frames stop drawing is given up, and
// that Trim gives up everything.
func TestEncodeEndFrame(t *testing.T) {
	scene := gfxtest.Build(func(b *gfx.Builder) {
		text(b, "SolA il*", 20, 8.3, 30, slate)
		b.FillPath(gfxtest.StarPath(60, 70, 20), gfx.NonZero, indigo)
		b.StrokePath(gfxtest.TightCurve(100, 40), gfx.NewStrokeStyle(3), emerald)
		b.PushClipPath(gfx.CirclePath(gfx.Pt(150, 30), 20), gfx.NonZero)
		b.FillRect(gfx.RectXYWH(120, 0, 60, 60), amber)
		b.PopClip()
	})
	rects := gfxtest.Build(func(b *gfx.Builder) { b.FillRect(gfx.RectXYWH(0, 0, 10, 10), slate) })
	size := image.Pt(200, 100)
	e := Encoder{GlyphCache: raster.NewGlyphCache(1 << 20)}
	f := e.Encode(scene, size, Options{})
	n := uploads(f)
	if n == 0 || f.Atlases[AtlasCoverage].Pages != 1 || f.Atlases[AtlasColor].Pages != 1 {
		t.Fatalf("%d uploads, atlases %+v", n, f.Atlases)
	}
	// Drawn every few frames, everything stays.
	for i := 0; i < 3*idleFrames; i++ {
		list := rects
		if i%(idleFrames/2) == 0 {
			list = scene
		}
		if f = e.Encode(list, size, Options{}); uploads(f) != 0 {
			t.Fatalf("frame %d uploads %d pictures", i, uploads(f))
		}
		e.EndFrame()
	}
	// Not drawn for more than idleFrames frames, everything goes: the
	// pictures, their pages, and what found them again.
	for i := 0; i <= idleFrames+1; i++ {
		e.Encode(rects, size, Options{})
		e.EndFrame()
	}
	r := e.res
	if f = e.Encode(rects, size, Options{}); f.Atlases[AtlasCoverage].Pages != 0 || f.Atlases[AtlasColor].Pages != 0 {
		t.Errorf("atlases %+v after frames that draw nothing from them", f.Atlases)
	}
	if len(r.atlas[0].entries)+len(r.atlas[1].entries)+len(r.shapes)+len(r.maskIDs)+len(r.paths.byHash)+len(r.styles) != 0 {
		t.Errorf("kept: %d and %d pictures, %d shapes, %d masks, %d paths, %d styles", len(r.atlas[0].entries), len(r.atlas[1].entries),
			len(r.shapes), len(r.maskIDs), len(r.paths.byHash), len(r.styles))
	}
	id := f.Res.ID
	if f = e.Encode(scene, size, Options{}); uploads(f) != n || f.Res.ID != id || f.Atlases[AtlasCoverage].Pages != 1 {
		t.Errorf("drawn again: %d uploads (%d at first), %+v", uploads(f), n, f.Res)
	}
	e.Trim()
	if f = e.Encode(scene, size, Options{}); uploads(f) != n || f.Res.ID == id || f.Res.Base != 0 {
		t.Errorf("after Trim: %d uploads, %+v", uploads(f), f.Res)
	}
}

// TestEncodeExtremes encodes what comes from textures under transforms and
// sizes far from the usual: nothing panics, the work stays bounded by the
// target, and every instance lies in the target with finite values.
func TestEncodeExtremes(t *testing.T) {
	star := gfxtest.StarPath(0, 0, 1)
	wide := gfx.NewStrokeStyle(0.3)
	wide.Dashes = []float32{0.5, 0.2}
	content := func(b *gfx.Builder) {
		b.FillPath(star, gfx.EvenOdd, slate)
		b.FillPath(blob(), gfx.NonZero, gfx.LinearGradient{Start: gfx.Pt(-1, -1), End: gfx.Pt(1, 1), Stops: stops(sky, rose)})
		b.StrokePath(gfx.CirclePath(gfx.Pt(0, 0), 0.8), wide, rose)
		b.DrawGlyphRun(gfx.DrawGlyphRun{Font: font, Size: 1.5, Origin: gfx.Pt(-1, 0.5), Glyphs: font.Shape("So@", 1.5), Paint: indigo})
		b.DrawImage(photo, gfx.Rect{}, gfx.RectXYWH(-1, -1, 2, 2), gfx.SampleLinear)
		b.DrawImage(gfxtest.TestImage, gfx.Rect{X0: 1, Y0: 1, X1: 1.001, Y1: 5}, gfx.RectXYWH(-0.5, -0.5, 1, 1), gfx.SampleNearest)
	}
	clips := func(b *gfx.Builder, inner func()) {
		b.PushClipRRect(gfx.NewRRect(gfx.RectXYWH(-0.9, -0.9, 1.8, 1.8), 0.4))
		b.PushClipPath(gfx.CirclePath(gfx.Pt(0.1, 0), 0.85), gfx.NonZero)
		b.PushClipRRect(gfx.RRectXY(gfx.RectXYWH(-0.8, -0.95, 1.7, 1.8), 0.6, 0.2))
		inner()
		b.PopClip()
		b.PopClip()
		b.PopClip()
	}
	size := image.Pt(300, 200)
	var e Encoder
	e.GlyphCache = raster.NewGlyphCache(1 << 22)
	for _, m := range []gfx.Affine{
		gfx.Translate(150, 100).Mul(gfx.Scale(60, 60)),                       // the usual
		gfx.Translate(150, 100).Mul(gfx.Scale(1e-4, 1e-4)),                   // a speck
		gfx.Translate(150, 100).Mul(gfx.Scale(3e4, 3e4)),                     // far larger than the target
		gfx.Translate(150, 100).Mul(gfx.Scale(1e9, 1e9)),                     // beyond the rasterizer's box
		gfx.Translate(-4e6, 3e6).Mul(gfx.Scale(4e6, 3e6)),                    // a huge translation that brings it back
		gfx.Translate(5e8, 5e8).Mul(gfx.Scale(50, 50)),                       // far away
		gfx.Translate(150, 100).Mul(gfx.Rotate(1)).Mul(gfx.Scale(5e5, 1e-3)), // a needle
		gfx.Translate(150, 100).Mul(gfx.Skew(1e6, 0)).Mul(gfx.Scale(40, 40)),
		gfx.Translate(150, 100).Mul(gfx.Scale(-80, 40)),
		{0, 50, -70, 0, 150.5, 100.25}, // a quarter turn
		{60, 30, 60, 30, 150, 100},     // singular
		{1e30, 0, 0, 1e30, 0, 0},
		{1e-30, 0, 0, 1e-30, 150, 100},
	} {
		for _, clipped := range []bool{false, true} {
			dl := gfxtest.Build(func(b *gfx.Builder) {
				b.PushTransform(m)
				if clipped {
					clips(b, func() { content(b) })
				} else {
					content(b)
				}
				b.PopTransform()
			})
			for _, region := range []image.Rectangle{{}, image.Rect(120, 80, 190, 130)} {
				f := e.Encode(dl, size, Options{Scale: 1.5, Region: region})
				if len(f.Unsupported) != 0 {
					t.Errorf("transform %v: unsupported %v", m, f.Unsupported)
				}
				if n := instances(f); n > 2000 {
					t.Errorf("transform %v: %d instances", m, n)
				}
				if len(f.Data) > 64<<20 {
					t.Errorf("transform %v: %d bytes uploaded for a target of %v", m, len(f.Data), size)
				}
				for i := 0; i < instances(f); i++ {
					for slot := 0; slot < InstanceSlots; slot++ {
						if slot == SlotHead || slot == SlotTex {
							continue
						}
						for _, v := range floats(f, i, slot) {
							if v != v || v-v != 0 {
								t.Fatalf("transform %v: instance %d, slot %d: %v", m, i, slot, floats(f, i, slot))
							}
						}
					}
					if b := floats(f, i, SlotBounds); !(b[0] < b[2] && b[1] < b[3]) || b[0] < 0 || b[1] < 0 || int(b[2]) > size.X || int(b[3]) > size.Y {
						t.Fatalf("transform %v: instance %d: bounds %v outside the target", m, i, b)
					}
				}
				(&atlasSim{t: t}).draw(f) // every upload and every read lies in its page
			}
			e.EndFrame()
		}
	}
}
