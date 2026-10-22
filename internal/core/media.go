package core

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"reflect"
	"unsafe"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/svg"
)

// Images and icons (ADR-098).
//
// # Sources
//
// An Img takes a decoded image (Src, SrcImage) or a source that Tela
// decodes: a path in the file systems registered with tela.Assets
// (SrcPath, resolved like the url() of a background: assets.go), bytes
// (SrcBytes) or a file of any fs.FS (SrcFS). An Icon (style.KindSVG)
// takes the text of an SVG document (IconSrc). The format is detected by
// content: PNG, JPEG, GIF (its first frame) and WebP are decoded into a
// gfx.Image; SVG is parsed into an svg.Doc and stays vector.
//
// Decoding happens once per content, when the element is mounted or its
// source changes (not when it is painted): the result is cached by
// cleaned path for SrcPath (the background cache), by content hash for
// SrcBytes, by (file system, path) for SrcFS, and by text for icons, in a
// cache bounded by size (mediaCacheBytes, least recently used first). A
// box keeps its decoded source while the node gives the same one (the
// same path, the same slice, the same file system and path, the same
// text), so rebuilding with the same source costs a comparison. A source
// that cannot be loaded or decoded paints nothing and is reported once in
// the log (reportOnce), like a missing background image.
//
// # Painting
//
// The content box shows the image per object-fit and object-position
// (CSS Images 3): the concrete object size from the natural size (an
// SVG's Intrinsic size, its Size when it has none) and ratio, placed in
// the content box and clipped to it (with its rounded corners) when it
// overflows. A raster image is drawn from the mip level just larger than
// its device size (as background images are, ADR-079). An SVG document
// is drawn into that rectangle by its viewBox and preserveAspectRatio, at
// the device scale (svg.Doc.DrawRecorded of a list recorded once per
// svg.Options and kept by the box), with the element's text color as
// currentColor and the fill, stroke and stroke-width of its style as the
// root overrides: classes on the element, or inherited from an ancestor
// (like Tailwind's [&_svg]:... patterns), beat the root's presentation
// attributes. Tint paints the image's alpha with a color: the image is
// the mask of a layer filled with it.

// srcKind is the kind of an Img or Icon source.
type srcKind uint8

const (
	srcNone srcKind = iota
	srcPath
	srcBytes
	srcFS
	srcIcon
)

// srcSpec is a source given to an Img or an Icon.
type srcSpec struct {
	kind srcKind
	path string // srcPath, srcFS
	data []byte // srcBytes
	fsys fs.FS  // srcFS
	icon string // srcIcon: the SVG text
}

// same reports whether s and o are the same source: the same kind and
// path, the same byte slice (pointer and length), the same file system,
// the same icon text.
func (s *srcSpec) same(o *srcSpec) bool {
	if s.kind != o.kind || s.path != o.path || s.icon != o.icon || len(s.data) != len(o.data) {
		return false
	}
	if len(s.data) > 0 && unsafe.SliceData(s.data) != unsafe.SliceData(o.data) {
		return false
	}
	return s.fsys == nil && o.fsys == nil || s.fsys != nil && o.fsys != nil && safeEqual(s.fsys, o.fsys)
}

// mediaProps are the Img and Icon props beyond Src and SrcImage.
type mediaProps struct {
	src     srcSpec
	tint    gfx.Color
	hasTint bool
}

// tintValue is a tint, comparable: on reports that there is one.
type tintValue struct {
	c  gfx.Color
	on bool
}

// tintOf returns the tint of m (nil-safe).
func (m *mediaProps) tintOf() tintValue {
	if m == nil || !m.hasTint {
		return tintValue{}
	}
	return tintValue{m.tint, true}
}

func (d *desc) mediaProps() *mediaProps {
	if d.media == nil {
		d.media = &mediaProps{}
	}
	return d.media
}

// SrcPath sets the image of an Img to a file of the file systems
// registered with RegisterAssets (tela.Assets).
func SrcPath(path string) Prop {
	return Prop{name: "SrcPath", apply: func(d *desc) {
		d.src, d.img = nil, nil
		d.mediaProps().src = srcSpec{kind: srcPath, path: path}
	}}
}

// SrcBytes sets the image of an Img to encoded data (PNG, JPEG, GIF,
// WebP or SVG). The slice must not be modified afterwards.
func SrcBytes(data []byte) Prop {
	return Prop{name: "SrcBytes", apply: func(d *desc) {
		d.src, d.img = nil, nil
		d.mediaProps().src = srcSpec{kind: srcBytes, data: data}
	}}
}

// SrcFS sets the image of an Img to the file path of fsys.
func SrcFS(fsys fs.FS, path string) Prop {
	return Prop{name: "SrcFS", apply: func(d *desc) {
		d.src, d.img = nil, nil
		d.mediaProps().src = srcSpec{kind: srcFS, fsys: fsys, path: path}
	}}
}

// IconSrc sets the SVG document of an Icon (style.KindSVG) primitive.
func IconSrc(svgText string) Prop {
	return Prop{name: "IconSrc", apply: func(d *desc) {
		d.mediaProps().src = srcSpec{kind: srcIcon, icon: svgText}
	}}
}

// Tint paints the image of an Img with color c through its alpha.
func Tint(c gfx.Color) Prop {
	return Prop{name: "Tint", apply: func(d *desc) {
		m := d.mediaProps()
		m.tint, m.hasTint = c, true
	}}
}

// checkMediaProps panics for media props on a primitive that does not
// take them.
func checkMediaProps(kind style.Kind, d *desc) {
	m := d.media
	if m == nil {
		return
	}
	switch {
	case m.src.kind == srcIcon && kind != style.KindSVG:
		panic(fmt.Sprintf("core: IconSrc on <%v>: use Icon", kind))
	case m.src.kind != srcNone && m.src.kind != srcIcon && kind != style.KindImg:
		panic(fmt.Sprintf("core: SrcPath, SrcBytes or SrcFS on <%v>: only Img takes an image source", kind))
	case m.hasTint && kind != style.KindImg:
		panic(fmt.Sprintf("core: Tint on <%v>: only Img takes a tint; an Icon follows its text color (text-*)", kind))
	}
}

// ---------------------------------------------------------------------
// Decoded sources.

// imageAsset is a decoded image: a raster image or an SVG document.
type imageAsset struct {
	raster *gfx.Image
	doc    *svg.Doc
}

// natural returns the natural width and height of the image (0 when it
// has none) and its natural ratio (0 when it has none).
func (a *imageAsset) natural() (w, h, ratio float32) {
	switch {
	case a == nil:
	case a.raster != nil:
		w, h = float32(a.raster.Width()), float32(a.raster.Height())
		if w > 0 && h > 0 {
			ratio = w / h
		}
	case a.doc != nil:
		return a.doc.Intrinsic()
	}
	return w, h, ratio
}

// size returns the size the image is drawn at when nothing else sets it:
// the natural size, or for an SVG without one its Size (from the ratio,
// the viewBox, else 300 x 150).
func (a *imageAsset) size() (w, h float32) {
	if a == nil {
		return 0, 0
	}
	if a.doc != nil {
		s := a.doc.Size()
		return s.W, s.H
	}
	w, h, _ = a.natural()
	return w, h
}

// cost estimates the memory an asset keeps, for the cache bound.
func (a *imageAsset) cost(srcLen int) int {
	if a != nil && a.raster != nil {
		return len(a.raster.Pix())
	}
	return 16 * srcLen // a parsed document: a few times its text
}

// decodeAsset decodes PNG, JPEG, GIF (first frame), WebP or SVG data,
// recognized by their content.
func decodeAsset(data []byte) (*imageAsset, error) {
	if isSVG(data) {
		doc, err := svg.Parse(data)
		if err != nil {
			return nil, err
		}
		return &imageAsset{doc: doc}, nil
	}
	img, err := decodeImage(data)
	if err != nil {
		return nil, err
	}
	return &imageAsset{raster: gfx.NewImage(img)}, nil
}

// isSVG reports whether data looks like an XML document: after a byte
// order mark and white space it starts with "<" and is not one of the
// raster formats.
func isSVG(data []byte) bool {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	data = bytes.TrimLeft(data, " \t\r\n")
	return len(data) > 0 && data[0] == '<'
}

// mediaCacheBytes bounds the decoded sources kept for SrcBytes, SrcFS and
// icons. The most recent entry is kept even when larger.
const mediaCacheBytes = 256 << 20

// mediaKey keys the media cache: [32]byte (the SHA-256 of SrcBytes
// data), fsKey or iconKey.
type (
	fsKey struct {
		fsys fs.FS
		path string
	}
	iconKey string
)

type mediaEntry struct {
	key   any
	asset *imageAsset
	err   error
	cost  int
}

// mediaCache is a least-recently-used cache of decoded sources, guarded
// by assets.mu.
type mediaCache struct {
	m     map[any]*mediaEntry
	order []*mediaEntry // least recently used first
	bytes int
	// decodes counts the decodes and parses that missed the cache
	// (tests).
	decodes int
}

func (c *mediaCache) get(k any) (*mediaEntry, bool) {
	e, ok := c.m[k]
	if ok {
		for i, x := range c.order {
			if x == e {
				copy(c.order[i:], c.order[i+1:])
				c.order[len(c.order)-1] = e
				break
			}
		}
	}
	return e, ok
}

func (c *mediaCache) add(e *mediaEntry) {
	if c.m == nil {
		c.m = map[any]*mediaEntry{}
	}
	if old, ok := c.m[e.key]; ok { // added meanwhile by another tree
		*e = *old
		return
	}
	c.m[e.key] = e
	c.order = append(c.order, e)
	c.bytes += e.cost
	for c.bytes > mediaCacheBytes && len(c.order) > 1 {
		old := c.order[0]
		c.order[0] = nil
		c.order = c.order[1:]
		delete(c.m, old.key)
		c.bytes -= old.cost
	}
}

// cachedMedia returns the decoded source of key k, calling load once.
func cachedMedia(k any, srcLen int, load func() (*imageAsset, error)) (*imageAsset, error) {
	assets.mu.Lock()
	if e, ok := assets.media.get(k); ok {
		assets.mu.Unlock()
		return e.asset, e.err
	}
	assets.media.decodes++
	assets.mu.Unlock()
	a, err := load()
	e := &mediaEntry{key: k, asset: a, err: err, cost: a.cost(srcLen)}
	assets.mu.Lock()
	assets.media.add(e)
	assets.mu.Unlock()
	return e.asset, e.err
}

// ParseIcon parses the SVG text of an icon, or returns the cached result
// of an earlier parse of the same text.
func ParseIcon(text string) (*svg.Doc, error) {
	a, err := cachedMedia(iconKey(text), len(text), func() (*imageAsset, error) {
		doc, err := svg.Parse([]byte(text))
		if err != nil {
			return nil, err
		}
		return &imageAsset{doc: doc}, nil
	})
	if err != nil {
		return nil, err
	}
	return a.doc, nil
}

// loadSource returns the decoded image of a source.
func loadSource(s *srcSpec) (*imageAsset, error) {
	switch s.kind {
	case srcPath:
		a, err := loadAsset(s.path)
		if err != nil {
			return nil, err
		}
		return a, nil
	case srcBytes:
		if len(s.data) == 0 {
			return nil, errors.New("SrcBytes got no data")
		}
		a, err := cachedMedia(sha256.Sum256(s.data), len(s.data), func() (*imageAsset, error) { return decodeAsset(s.data) })
		if err != nil {
			return nil, fmt.Errorf("the %d bytes of SrcBytes cannot be decoded: %v", len(s.data), err)
		}
		return a, nil
	case srcFS:
		p, err := assetPath(s.path)
		if err != nil {
			return nil, err
		}
		load := func() (*imageAsset, error) {
			if s.fsys == nil {
				return nil, fmt.Errorf("SrcFS(nil, %q): no file system", s.path)
			}
			data, err := fs.ReadFile(s.fsys, p)
			if err != nil {
				return nil, fmt.Errorf("%q in %s: %v", s.path, describeFS(s.fsys), err)
			}
			a, err := decodeAsset(data)
			if err != nil {
				return nil, fmt.Errorf("%q in %s cannot be decoded: %v", s.path, describeFS(s.fsys), err)
			}
			return a, nil
		}
		var a *imageAsset
		if s.fsys != nil && hashable(reflect.ValueOf(s.fsys)) {
			a, err = cachedMedia(fsKey{s.fsys, p}, 0, load)
		} else {
			a, err = load() // not hashable: the box keeps it while the source is the same
		}
		if err != nil {
			return nil, err
		}
		return a, nil
	case srcIcon:
		doc, err := ParseIcon(s.icon)
		if err != nil {
			return nil, fmt.Errorf("the SVG cannot be parsed: %v", err)
		}
		return &imageAsset{doc: doc}, nil
	}
	return nil, nil
}

// hashable reports whether v can be a map key without panicking. A
// comparable type is not enough: an interface inside it (a struct field,
// an array element) may hold a value of a type that is not comparable,
// such as the fstest.MapFS of a struct with an fs.FS field, and hashing
// it panics ("hash of unhashable type"). So the dynamic value is walked:
// maps, slices and functions are not hashable, interfaces are what they
// hold, structs and arrays what their parts are. Pointers are not
// followed (a pointer hashes by address), so the walk ends.
func hashable(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Invalid:
		return true // a nil interface
	case reflect.Map, reflect.Slice, reflect.Func:
		return false
	case reflect.Interface:
		return v.IsNil() || hashable(v.Elem())
	case reflect.Struct:
		if !hasInterface(v.Type()) {
			return v.Type().Comparable()
		}
		for i := range v.NumField() {
			if !hashable(v.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Array:
		if !hasInterface(v.Type()) {
			return v.Type().Comparable()
		}
		for i := range v.Len() {
			if !hashable(v.Index(i)) {
				return false
			}
		}
		return true
	}
	return true
}

// hasInterface reports whether values of type t hold interfaces in
// place (in fields or array elements, not behind pointers).
func hasInterface(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Interface:
		return true
	case reflect.Struct:
		for i := range t.NumField() {
			if hasInterface(t.Field(i).Type) {
				return true
			}
		}
	case reflect.Array:
		return hasInterface(t.Elem())
	}
	return false
}

// reportKey is the key of a source's load error in reportOnce.
func (s *srcSpec) reportKey() string {
	switch s.kind {
	case srcPath:
		return "img-path:" + s.path
	case srcBytes:
		return fmt.Sprintf("img-bytes:%x", sha256.Sum256(s.data))
	case srcFS:
		return fmt.Sprintf("img-fs:%s:%s", describeFS(s.fsys), s.path)
	case srcIcon:
		return fmt.Sprintf("icon:%x", sha256.Sum256([]byte(s.icon)))
	}
	return "img"
}

// ---------------------------------------------------------------------
// Boxes.

// boxMedia is the decoded source of an Img or Icon box given by a
// srcSpec, and the SVG list recorded for the last options it was drawn
// with.
type boxMedia struct {
	src   srcSpec
	asset *imageAsset
	// rec is the document recorded for recOpts (svg.Doc.Record); recOK
	// reports that it is valid.
	rec     *gfx.DisplayList
	recOpts svg.Options
	recOK   bool
}

// doc returns the SVG document of bx, or nil.
func (bx *Box) doc() *svg.Doc {
	if bx.med == nil || bx.med.asset == nil {
		return nil
	}
	return bx.med.asset.doc
}

// imageSize returns the size of the image of bx when nothing else sets
// it, and its natural ratio (0 for none).
func (bx *Box) imageSize() (w, h, ratio float32) {
	switch {
	case bx.med != nil && bx.med.asset != nil:
		w, h = bx.med.asset.size()
		_, _, ratio = bx.med.asset.natural()
	case bx.img != nil && bx.img.Width() > 0 && bx.img.Height() > 0:
		w, h = float32(bx.img.Width()), float32(bx.img.Height())
		ratio = w / h
	}
	return w, h, ratio
}

// updateMedia loads the source s of el's box when it changed.
func (t *Tree) updateMedia(el *Element, s *srcSpec) {
	b := el.box
	if b.med != nil && b.med.src.same(s) {
		return
	}
	a, err := loadSource(s)
	if err != nil {
		what := "Img source"
		if s.kind == srcIcon {
			what = "Icon"
		}
		t.reportOnce(s.reportKey(), "tela: "+what+": "+err.Error())
	}
	ow, oh, or := b.imageSize()
	if b.med == nil {
		b.med = &boxMedia{}
	}
	*b.med = boxMedia{src: *s, asset: a}
	b.img, b.imgSrc = nil, nil
	if a != nil {
		b.img = a.raster
	}
	if w, h, r := b.imageSize(); w != ow || h != oh || r != or {
		t.engine.MarkDirty(b)
		b.queueRemap() // the natural aspect ratio is part of the layout style
	}
	t.markMediaPaint(b)
}

// markMediaPaint repaints b after a change of its image or tint.
func (t *Tree) markMediaPaint(b *Box) {
	if b.parent != nil || b.inPara != nil {
		t.markPaint(b)
	}
	t.dirty()
}

// svgOptions returns the options of the SVG document of bx: its text
// color (with the TextColorFn that reaches it) as currentColor, as the
// root's color too when the element sets its own text color, and the
// fill, stroke and stroke width of its style as root overrides.
func (p *painter) svgOptions(bx *Box) svg.Options {
	cs := bx.pcs
	cur := cs.Text.Color
	if p.t.textColorFns > 0 && bx.el != nil {
		if c, ok := p.t.fnTextColor(bx.el); ok {
			cur = c
		}
	}
	o := svg.Options{CurrentColor: cur}
	if cs.TextColorSet || cs.HasTextColorFn() {
		o.Root.Color, o.Root.HasColor = cur, true
	}
	o.Root.Fill = svgPaint(cs.Fill, cur)
	o.Root.Stroke = svgPaint(cs.Stroke, cur)
	if cs.StrokeWidthSet {
		o.Root.StrokeWidth, o.Root.HasStrokeWidth = cs.StrokeWidth, true
	}
	return o
}

func svgPaint(p style.SVGPaint, cur gfx.Color) svg.Paint {
	switch p.Kind {
	case style.SVGPaintNone:
		return svg.Paint{Kind: svg.PaintNone}
	case style.SVGPaintColor:
		return svg.Paint{Kind: svg.PaintColor, Color: p.Color}
	case style.SVGPaintCurrent:
		if p.Color.A == 1 {
			return svg.Paint{Kind: svg.PaintCurrentColor}
		}
		return svg.Paint{Kind: svg.PaintColor, Color: p.Resolve(cur)}
	}
	return svg.Paint{}
}

// svgList returns the document of bx recorded for opts, recording it
// again only when the options changed.
func (bx *Box) svgList(opts svg.Options) *gfx.DisplayList {
	m := bx.med
	if !m.recOK || m.recOpts != opts {
		m.rec, m.recOpts, m.recOK = m.asset.doc.Record(opts), opts, true
		bx.tree.svgRecords++
	}
	return m.rec
}

// objectRect returns where the image is drawn in content box c, per
// object-fit and object-position: nw, nh are its natural size (0 when it
// has none) and ratio its natural ratio (0 when none).
func objectRect(cs *style.ComputedStyle, c gfx.Rect, nw, nh, ratio float32) gfx.Rect {
	cw, ch := c.Width(), c.Height()
	w, h := cw, ch
	contain := func(cover bool) (float32, float32) {
		if !(ratio > 0) {
			return cw, ch
		}
		// The largest (contain) or smallest (cover) rectangle of the ratio
		// that fits in or covers the box.
		if (float64(cw)/float64(ch) > float64(ratio)) != cover {
			return float32(float64(ch) * float64(ratio)), ch
		}
		return cw, float32(float64(cw) / float64(ratio))
	}
	natural := func() (float32, float32) {
		switch {
		case nw > 0 && nh > 0:
			return nw, nh
		case nw > 0 && ratio > 0:
			return nw, float32(float64(nw) / float64(ratio))
		case nh > 0 && ratio > 0:
			return float32(float64(nh) * float64(ratio)), nh
		case ratio > 0:
			return contain(false)
		}
		return cw, ch
	}
	switch cs.ObjectFit {
	case style.ObjectContain:
		w, h = contain(false)
	case style.ObjectCover:
		w, h = contain(true)
	case style.ObjectNone:
		w, h = natural()
	case style.ObjectScaleDown:
		w, h = natural()
		if cw2, ch2 := contain(false); cw2 < w || ch2 < h {
			w, h = cw2, ch2
		}
	default:
		return c
	}
	x := c.X0 + cs.ObjectPosX.Resolve(cw-w)
	y := c.Y0 + cs.ObjectPosY.Resolve(ch-h)
	return gfx.Rect{X0: x, Y0: y, X1: x + w, Y1: y + h}
}

// mipLevel returns the mip level of img to draw it at w x h device
// pixels: the smallest level not smaller than that (ADR-079).
func mipLevel(img *gfx.Image, w, h float64) *gfx.Image {
	kx, ky := w/float64(img.Width()), h/float64(img.Height())
	if k := max(kx, ky); k > 0 && k < 1 {
		if n := int(math.Floor(math.Log2(1 / k))); n > 0 {
			return img.Level(n)
		}
	}
	return img
}

// paintReplaced paints the image of an Img or Icon box in its content
// box.
func (p *painter) paintReplaced(bx *Box) {
	doc := bx.doc()
	if bx.img == nil && doc == nil {
		return
	}
	g := p.geom(bx)
	c := p.contentRRect(bx, &g)
	if c.Rect.Empty() {
		return
	}
	cs := bx.pcs
	nw, nh, ratio := bx.imageNatural()
	dst := objectRect(cs, c.Rect, nw, nh, ratio)
	if dst.Empty() {
		return
	}
	// A replaced element's content is clipped to the content box's
	// rounded corners (CSS Backgrounds 3 section 5.3), and to the content
	// box when it overflows it (object-fit: cover, none).
	clip := 0
	switch {
	case !c.IsRect():
		p.b.PushClipRRect(c)
		clip = 1
	case !c.Rect.ContainsRect(dst):
		p.b.PushClipRect(c.Rect)
		clip = 1
	}
	var tint *gfx.Color
	if m := bx.el.node.d.media; m != nil && m.hasTint {
		tint = &m.tint
	}
	b := p.b
	var mb *gfx.Builder
	if tint != nil {
		mb = p.t.lay.builder()
		p.b = mb
	}
	if bx.img != nil {
		src := mipLevel(bx.img, float64(dst.Width())*p.scale, float64(dst.Height())*p.scale)
		p.b.DrawImage(src, gfx.Rect{}, dst, gfx.SampleLinear)
	} else {
		doc.DrawRecorded(p.b, dst, bx.svgList(p.svgOptions(bx)))
	}
	if tint != nil {
		p.b = b
		list := mb.Finish()
		p.t.lay.free = append(p.t.lay.free, mb)
		if list.Len() > 0 {
			b.PushMaskLayer(gfx.Mask{List: list, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
			b.FillRect(dst, *tint)
			b.PopLayer()
		}
	}
	for ; clip > 0; clip-- {
		p.b.PopClip()
	}
}

// imageNatural returns the natural size and ratio of the image of bx
// (zeros for what it does not have).
func (bx *Box) imageNatural() (w, h, ratio float32) {
	if bx.med != nil && bx.med.asset != nil {
		return bx.med.asset.natural()
	}
	if bx.img != nil && bx.img.Width() > 0 && bx.img.Height() > 0 {
		w, h = float32(bx.img.Width()), float32(bx.img.Height())
		return w, h, w / h
	}
	return 0, 0, 0
}
