package text

import (
	"math"
	"sort"
	"sync"

	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gfx"
)

// COLR (v0 and v1) color glyphs are converted to gfx display lists in em
// space. The conversion of each part of the paint graph, and what is
// approximated, is documented in doc.go ("Color glyphs").

// colorEntry is the cached color data of one glyph.
type colorEntry struct {
	paint   tables.PaintTable
	bounds  gfx.Rect // em; what GlyphBounds returns
	clip    gfx.Rect // em; the COLR clip box
	hasClip bool

	once   sync.Once
	usesFG bool             // the graph uses palette index 0xFFFF
	shared *gfx.DisplayList // the list for every fg, when !usesFG

	mu   sync.Mutex
	byFG map[gfx.Color]*gfx.DisplayList // when usesFG
}

var notColor = &colorEntry{}

// maxFGLists bounds the per-foreground cache of a color glyph that uses the
// foreground color.
const maxFGLists = 32

// color returns the color data of glyph g, or nil for an outline glyph.
func (f *Face) color(g gfx.GlyphID) *colorEntry {
	if f.colors == nil || int(g) >= len(f.colors) {
		return nil
	}
	slot := &f.colors[g]
	c := slot.Load()
	if c == nil {
		c = f.loadColor(tables.GlyphID(g))
		if !slot.CompareAndSwap(nil, c) {
			c = slot.Load()
		}
	}
	if c == notColor {
		return nil
	}
	return c
}

func (f *Face) loadColor(gid tables.GlyphID) *colorEntry {
	colr := f.tf.ft.COLR
	pt, ok := colr.Search(gid)
	if !ok {
		return notColor
	}
	c := &colorEntry{paint: pt}
	if r, ok := f.clipBox(gid); ok {
		c.clip, c.hasClip, c.bounds = r, true, r
	} else {
		// No clip box: the bounds of what the graph paints, with unbounded
		// paints limited to the font's bounding box (head).
		c.bounds = c.list(f, gfx.GlyphID(gid), gfx.Black).Bounds()
	}
	return c
}

// clipBox returns the COLR clip box of gid in em space.
func (f *Face) clipBox(gid tables.GlyphID) (gfx.Rect, bool) {
	colr := f.tf.ft.COLR
	cb, ok := colr.ClipList.Search(gid)
	if !ok {
		return gfx.Rect{}, false
	}
	var x0, y0, x1, y1 float64
	switch b := cb.(type) {
	case tables.ClipBoxFormat1:
		x0, y0, x1, y1 = float64(b.XMin), float64(b.YMin), float64(b.XMax), float64(b.YMax)
	case tables.ClipBoxFormat2:
		p := painter{f: f, colr: colr}
		x0 = float64(b.XMin) + p.delta(b.VarIndexBase, 0)
		y0 = float64(b.YMin) + p.delta(b.VarIndexBase, 1)
		x1 = float64(b.XMax) + p.delta(b.VarIndexBase, 2)
		y1 = float64(b.YMax) + p.delta(b.VarIndexBase, 3)
	default:
		return gfx.Rect{}, false
	}
	u := f.upem
	r := gfx.Rect{X0: float32(x0 / u), Y0: float32(-y1 / u), X1: float32(x1 / u), Y1: float32(-y0 / u)}
	return r, !r.Empty()
}

// list returns the display list of the color glyph for foreground fg.
func (c *colorEntry) list(f *Face, g gfx.GlyphID, fg gfx.Color) *gfx.DisplayList {
	c.once.Do(func() {
		l, uses := f.buildColor(c, tables.GlyphID(g), fg)
		c.usesFG = uses
		if uses {
			c.byFG = map[gfx.Color]*gfx.DisplayList{fg: l}
		} else {
			c.shared = l
		}
	})
	if !c.usesFG {
		return c.shared
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if l := c.byFG[fg]; l != nil {
		return l
	}
	l, _ := f.buildColor(c, tables.GlyphID(g), fg)
	if len(c.byFG) >= maxFGLists {
		clear(c.byFG)
	}
	c.byFG[fg] = l
	return l
}

// buildColor converts the paint graph of a color glyph to a display list.
func (f *Face) buildColor(c *colorEntry, gid tables.GlyphID, fg gfx.Color) (*gfx.DisplayList, bool) {
	colr := f.tf.ft.COLR
	p := &painter{
		f:      f,
		colr:   colr,
		b:      gfx.NewBuilder(),
		fg:     fg,
		ctm:    gfx.Identity,
		active: map[tables.GlyphID]bool{gid: true},
	}
	if len(f.tf.ft.CPAL) > 0 {
		p.cpal = f.tf.ft.CPAL[0]
	}
	u := f.upem
	h := f.tf.head
	p.cover = gfx.Rect{X0: float32(float64(h.XMin) / u), Y0: float32(-float64(h.YMax) / u), X1: float32(float64(h.XMax) / u), Y1: float32(-float64(h.YMin) / u)}
	if c.hasClip {
		p.cover = c.clip
		p.b.PushClipRect(c.clip)
	}
	// Unbounded fills overshoot the area they must cover, so that their
	// edges never coincide with the clip's (which would double the
	// anti-aliasing of the edge pixels).
	p.cover = p.cover.Outset(coverMargin)
	p.paint(c.paint)
	if c.hasClip {
		p.b.PopClip()
	}
	return p.b.Finish(), p.usesFG
}

const (
	coverMargin   = 0.25 // em
	maxPaintDepth = 64
)

// painter walks a COLR paint graph and records it into a Builder.
//
// Coordinates: COLR geometry is in font units with y up. Every point is
// mapped to em space (x/upem, -y/upem) and every transform T of the graph is
// conjugated by that map, so the recorded list has no root transform and
// the glyph outlines are the em-space paths of GlyphOutline.
type painter struct {
	f      *Face
	colr   *tables.COLR1
	cpal   []tables.ColorRecord
	b      *gfx.Builder
	fg     gfx.Color
	usesFG bool

	ctm    gfx.Affine // current local em space -> glyph em space
	cover  gfx.Rect   // glyph em space area that unbounded paints fill
	depth  int
	active map[tables.GlyphID]bool // PaintColrGlyph cycle guard
}

func (p *painter) pt(x, y float64) gfx.Point {
	return gfx.Point{X: float32(x / p.f.upem), Y: float32(-y / p.f.upem)}
}

// fontAffine returns the em-space conjugate of the font-space transform
// x' = xx*x + xy*y + dx, y' = yx*x + yy*y + dy.
func (p *painter) fontAffine(xx, yx, xy, yy, dx, dy float64) gfx.Affine {
	u := p.f.upem
	return gfx.Affine{float32(xx), float32(-yx), float32(-xy), float32(yy), float32(dx / u), float32(-dy / u)}
}

// around returns t applied around the font-space point (cx, cy).
func (p *painter) around(t gfx.Affine, cx, cy float64) gfx.Affine {
	c := p.pt(cx, cy)
	return gfx.Translate(c.X, c.Y).Mul(t).Mul(gfx.Translate(-c.X, -c.Y))
}

// delta returns the COLR variation delta at varIndexBase + i for the
// face's coordinates, in the units of the varied field.
func (p *painter) delta(base, i uint32) float64 {
	if base == 0xFFFFFFFF || len(p.f.coords) == 0 || p.colr.ItemVariationStore == nil {
		return 0
	}
	idx := base + i
	var vsi tables.VariationStoreIndex
	if m := p.colr.VarIndexMap; m != nil && len(m.Map) > 0 {
		if int(idx) >= len(m.Map) {
			idx = uint32(len(m.Map) - 1)
		}
		vsi = m.Map[idx]
	} else {
		vsi = tables.VariationStoreIndex{DeltaSetOuter: uint16(idx >> 16), DeltaSetInner: uint16(idx)}
	}
	return varStoreDelta(p.colr.ItemVariationStore, vsi, p.f.coords)
}

func f2dot14(v tables.Coord) float64 { return float64(v) / 16384 }

// paletteColor returns CPAL palette 0's entry idx (0xFFFF: the foreground
// color) with its alpha multiplied by alpha.
func (p *painter) paletteColor(idx uint16, alpha float64) gfx.Color {
	var c gfx.Color
	switch {
	case idx == 0xFFFF:
		c = p.fg
		p.usesFG = true
	case int(idx) < len(p.cpal):
		r := p.cpal[idx]
		c = gfx.RGBA8(r.Red, r.Green, r.Blue, r.Alpha)
	default:
		return gfx.Transparent
	}
	c.A = float32(float64(c.A) * min(max(alpha, 0), 1))
	return c
}

// paint records paint node n in the current local space.
func (p *painter) paint(n tables.PaintTable) {
	if p.depth >= maxPaintDepth {
		return
	}
	p.depth++
	defer func() { p.depth-- }()

	switch v := n.(type) {
	case tables.PaintColrLayers:
		layers, err := p.colr.LayerList.Resolve(v)
		if err != nil {
			return
		}
		for _, l := range layers {
			p.paint(l)
		}
	case tables.PaintColrLayersResolved: // COLRv0
		for _, l := range v {
			path, ok := p.f.GlyphOutline(gfx.GlyphID(l.GlyphID))
			if ok {
				p.b.FillPath(path, gfx.NonZero, p.paletteColor(l.PaletteIndex, 1))
			}
		}
	case tables.PaintGlyph:
		p.glyph(tables.GlyphID(v.GlyphID), v.Paint)
	case tables.PaintColrGlyph:
		p.colrGlyph(tables.GlyphID(v.GlyphID))
	case tables.PaintComposite:
		p.composite(v)
	default:
		if t, child, ok := p.transformOf(n); ok {
			p.withTransform(t, child)
			return
		}
		p.fillUnbounded(n)
	}
}

// transformOf returns the em-space transform of a transform node and its
// child.
func (p *painter) transformOf(n tables.PaintTable) (gfx.Affine, tables.PaintTable, bool) {
	var b uint32
	d := func(i uint32) float64 { return p.delta(b, i) }
	switch v := n.(type) {
	case tables.PaintTransform:
		t := v.Transform
		return p.fontAffine(float64(t.Xx), float64(t.Yx), float64(t.Xy), float64(t.Yy), float64(t.Dx), float64(t.Dy)), v.Paint, true
	case tables.PaintVarTransform:
		t := v.Transform
		b = t.VarIndexBase
		const s = 1.0 / 65536
		return p.fontAffine(float64(t.Xx)+d(0)*s, float64(t.Yx)+d(1)*s, float64(t.Xy)+d(2)*s, float64(t.Yy)+d(3)*s,
			float64(t.Dx)+d(4)*s, float64(t.Dy)+d(5)*s), v.Paint, true
	case tables.PaintTranslate:
		return p.fontAffine(1, 0, 0, 1, float64(v.Dx), float64(v.Dy)), v.Paint, true
	case tables.PaintVarTranslate:
		b = v.VarIndexBase
		return p.fontAffine(1, 0, 0, 1, float64(v.Dx)+d(0), float64(v.Dy)+d(1)), v.Paint, true
	case tables.PaintScale:
		return p.fontAffine(f2dot14(v.ScaleX), 0, 0, f2dot14(v.ScaleY), 0, 0), v.Paint, true
	case tables.PaintVarScale:
		b = v.VarIndexBase
		return p.fontAffine(f2dot14(v.ScaleX)+d(0)/16384, 0, 0, f2dot14(v.ScaleY)+d(1)/16384, 0, 0), v.Paint, true
	case tables.PaintScaleAroundCenter:
		t := p.fontAffine(f2dot14(v.ScaleX), 0, 0, f2dot14(v.ScaleY), 0, 0)
		return p.around(t, float64(v.CenterX), float64(v.CenterY)), v.Paint, true
	case tables.PaintVarScaleAroundCenter:
		b = v.VarIndexBase
		t := p.fontAffine(f2dot14(v.ScaleX)+d(0)/16384, 0, 0, f2dot14(v.ScaleY)+d(1)/16384, 0, 0)
		return p.around(t, float64(v.CenterX)+d(2), float64(v.CenterY)+d(3)), v.Paint, true
	case tables.PaintScaleUniform:
		s := f2dot14(v.Scale)
		return p.fontAffine(s, 0, 0, s, 0, 0), v.Paint, true
	case tables.PaintVarScaleUniform:
		b = v.VarIndexBase
		s := f2dot14(v.Scale) + d(0)/16384
		return p.fontAffine(s, 0, 0, s, 0, 0), v.Paint, true
	case tables.PaintScaleUniformAroundCenter:
		s := f2dot14(v.Scale)
		return p.around(p.fontAffine(s, 0, 0, s, 0, 0), float64(v.CenterX), float64(v.CenterY)), v.Paint, true
	case tables.PaintVarScaleUniformAroundCenter:
		b = v.VarIndexBase
		s := f2dot14(v.Scale) + d(0)/16384
		return p.around(p.fontAffine(s, 0, 0, s, 0, 0), float64(v.CenterX)+d(1), float64(v.CenterY)+d(2)), v.Paint, true
	case tables.PaintRotate:
		return p.rotate(f2dot14(v.Angle)), v.Paint, true
	case tables.PaintVarRotate:
		b = v.VarIndexBase
		return p.rotate(f2dot14(v.Angle) + d(0)/16384), v.Paint, true
	case tables.PaintRotateAroundCenter:
		return p.around(p.rotate(f2dot14(v.Angle)), float64(v.CenterX), float64(v.CenterY)), v.Paint, true
	case tables.PaintVarRotateAroundCenter:
		b = v.VarIndexBase
		return p.around(p.rotate(f2dot14(v.Angle)+d(0)/16384), float64(v.CenterX)+d(1), float64(v.CenterY)+d(2)), v.Paint, true
	case tables.PaintSkew:
		return p.skew(f2dot14(v.XSkewAngle), f2dot14(v.YSkewAngle)), v.Paint, true
	case tables.PaintVarSkew:
		b = v.VarIndexBase
		return p.skew(f2dot14(v.XSkewAngle)+d(0)/16384, f2dot14(v.YSkewAngle)+d(1)/16384), v.Paint, true
	case tables.PaintSkewAroundCenter:
		return p.around(p.skew(f2dot14(v.XSkewAngle), f2dot14(v.YSkewAngle)), float64(v.CenterX), float64(v.CenterY)), v.Paint, true
	case tables.PaintVarSkewAroundCenter:
		b = v.VarIndexBase
		return p.around(p.skew(f2dot14(v.XSkewAngle)+d(0)/16384, f2dot14(v.YSkewAngle)+d(1)/16384),
			float64(v.CenterX)+d(2), float64(v.CenterY)+d(3)), v.Paint, true
	}
	return gfx.Affine{}, nil, false
}

// rotate returns the rotation by turns*180 degrees counter-clockwise (font
// space, y up). Multiples of 90 degrees are exact; other angles use the
// deterministic fmath.Sincos (ADR-027).
func (p *painter) rotate(turns float64) gfx.Affine {
	var s, c float64
	switch q := turns * 2; {
	case q == math.Trunc(q):
		k := int(math.Mod(q, 4))
		if k < 0 {
			k += 4
		}
		s, c = [4]float64{0, 1, 0, -1}[k], [4]float64{1, 0, -1, 0}[k]
	default:
		s, c = fmath.Sincos(float64(turns * math.Pi))
	}
	return p.fontAffine(c, s, -s, c, 0, 0)
}

// skew returns the COLR skew: x' = x - tan(ax)*y, y' = y + tan(ay)*x, with
// the angles in half turns (180 degrees per 1.0), counter-clockwise.
func (p *painter) skew(ax, ay float64) gfx.Affine {
	return p.fontAffine(1, fmath.Tan(float64(ay*math.Pi)), -fmath.Tan(float64(ax*math.Pi)), 1, 0, 0)
}

func (p *painter) withTransform(t gfx.Affine, child tables.PaintTable) {
	if t == gfx.Identity {
		p.paint(child)
		return
	}
	if !t.IsFinite() || t.Det() == 0 {
		return // a singular transform paints nothing
	}
	saved := p.ctm
	p.b.PushTransform(t)
	p.ctm = saved.Mul(t)
	p.paint(child)
	p.ctm = saved
	p.b.PopTransform()
}

// localCover returns the cover area in the current local space.
func (p *painter) localCover() (gfx.Rect, bool) {
	inv, ok := p.ctm.Invert()
	if !ok {
		return gfx.Rect{}, false
	}
	return transformRect(inv, p.cover), true
}

// transformRect returns the bounding box of r transformed by a, computed
// like transformPath (fusion-safe).
func transformRect(a gfx.Affine, r gfx.Rect) gfx.Rect {
	a0, a1, a2, a3, a4, a5 := float64(a[0]), float64(a[1]), float64(a[2]), float64(a[3]), float64(a[4]), float64(a[5])
	var out gfx.Rect
	for i, q := range [4]gfx.Point{{X: r.X0, Y: r.Y0}, {X: r.X1, Y: r.Y0}, {X: r.X1, Y: r.Y1}, {X: r.X0, Y: r.Y1}} {
		x, y := float64(q.X), float64(q.Y)
		px, py := float32(a0*x+a2*y+a4), float32(a1*x+a3*y+a5)
		if i == 0 {
			out = gfx.Rect{X0: px, Y0: py, X1: px, Y1: py}
			continue
		}
		out = gfx.Rect{X0: min(out.X0, px), Y0: min(out.Y0, py), X1: max(out.X1, px), Y1: max(out.Y1, py)}
	}
	return out
}

// fillUnbounded fills the whole cover area with a solid or gradient node.
func (p *painter) fillUnbounded(n tables.PaintTable) {
	cover, ok := p.localCover()
	if !ok {
		return
	}
	if paint := p.simplePaint(n, cover); paint != nil {
		p.b.FillRect(cover, paint)
	}
}

// glyph records PaintGlyph: the child paint clipped to the outline of gid.
// A solid or gradient child (possibly under transforms) becomes a FillPath
// of the outline; anything else is clipped with PushClipPath.
func (p *painter) glyph(gid tables.GlyphID, child tables.PaintTable) {
	path, ok := p.f.GlyphOutline(gfx.GlyphID(gid))
	if !ok {
		return // an empty outline clips everything away
	}
	t, c := gfx.Identity, child
	for {
		tt, inner, ok := p.transformOf(c)
		if !ok {
			break
		}
		t, c = t.Mul(tt), inner
	}
	if isSimplePaint(c) {
		if t == gfx.Identity {
			if paint := p.simplePaint(c, path.ControlBounds()); paint != nil {
				p.b.FillPath(path, gfx.NonZero, paint)
			}
			return
		}
		inv, ok := t.Invert()
		if !ok || !t.IsFinite() {
			return
		}
		local := transformPath(path, inv)
		saved := p.ctm
		p.b.PushTransform(t)
		p.ctm = saved.Mul(t)
		if paint := p.simplePaint(c, local.ControlBounds()); paint != nil {
			p.b.FillPath(local, gfx.NonZero, paint)
		}
		p.ctm = saved
		p.b.PopTransform()
		return
	}
	p.b.PushClipPath(path, gfx.NonZero)
	p.paint(child)
	p.b.PopClip()
}

func isSimplePaint(n tables.PaintTable) bool {
	switch n.(type) {
	case tables.PaintSolid, tables.PaintVarSolid,
		tables.PaintLinearGradient, tables.PaintVarLinearGradient,
		tables.PaintRadialGradient, tables.PaintVarRadialGradient,
		tables.PaintSweepGradient, tables.PaintVarSweepGradient:
		return true
	}
	return false
}

// colrGlyph records PaintColrGlyph: another color glyph's graph, clipped
// to its clip box.
func (p *painter) colrGlyph(gid tables.GlyphID) {
	if p.active[gid] {
		return // a cycle
	}
	pt, ok := p.colr.Search(gid)
	if !ok {
		return
	}
	p.active[gid] = true
	defer delete(p.active, gid)
	if r, ok := p.f.clipBox(gid); ok {
		p.b.PushClipRect(r)
		p.paint(pt)
		p.b.PopClip()
		return
	}
	p.paint(pt)
}

// compositeBlend maps the COLR blend modes to gfx.
var compositeBlend = map[tables.CompositeMode]gfx.BlendMode{
	tables.CompositeScreen:        gfx.Screen,
	tables.CompositeOverlay:       gfx.Overlay,
	tables.CompositeDarken:        gfx.Darken,
	tables.CompositeLighten:       gfx.Lighten,
	tables.CompositeColorDodge:    gfx.ColorDodge,
	tables.CompositeColorBurn:     gfx.ColorBurn,
	tables.CompositeHardLight:     gfx.HardLight,
	tables.CompositeSoftLight:     gfx.SoftLight,
	tables.CompositeDifference:    gfx.Difference,
	tables.CompositeExclusion:     gfx.Exclusion,
	tables.CompositeMultiply:      gfx.Multiply,
	tables.CompositeHslHue:        gfx.Hue,
	tables.CompositeHslSaturation: gfx.Saturation,
	tables.CompositeHslColor:      gfx.ColorBlend,
	tables.CompositeHslLuminosity: gfx.Luminosity,
	tables.CompositePlus:          gfx.PlusLighter,
}

// composite records PaintComposite. Blend modes and Plus are exact: an
// isolated group holds the backdrop, and the source is a layer blended onto
// it. Porter-Duff modes are mapped onto drawing order and mask layers; see
// doc.go.
func (p *painter) composite(v tables.PaintComposite) {
	src, bd := v.SourcePaint, v.BackdropPaint
	if mode, ok := compositeBlend[v.CompositeMode]; ok {
		p.b.PushLayer(1, gfx.SrcOver)
		p.paint(bd)
		p.b.PushLayer(1, mode)
		p.paint(src)
		p.b.PopLayer()
		p.b.PopLayer()
		return
	}
	switch v.CompositeMode {
	case tables.CompositeClear:
	case tables.CompositeSrc, tables.CompositeSrcOut:
		p.paint(src)
	case tables.CompositeDest, tables.CompositeDestOut:
		p.paint(bd)
	case tables.CompositeDestOver:
		p.paint(src)
		p.paint(bd)
	case tables.CompositeSrcIn:
		p.paintIn(src, bd)
	case tables.CompositeDestIn:
		p.paintIn(bd, src)
	case tables.CompositeSrcAtop:
		p.paint(bd)
		p.paintIn(src, bd)
	case tables.CompositeDestAtop:
		p.paint(src)
		p.paintIn(bd, src)
	default: // SrcOver, Xor
		p.paint(bd)
		p.paint(src)
	}
}

// paintIn paints an "in" composite: a's pixels multiplied by mask's
// alpha. A mask that is a solid color is a layer with its alpha as
// opacity; any other mask is recorded into a list of its own and used as
// the alpha mask of a layer holding a (gfx.Mask), which is exact.
func (p *painter) paintIn(a, mask tables.PaintTable) {
	if alpha, ok := p.solidAlpha(mask); ok {
		if alpha > 0 {
			p.b.PushLayer(alpha, gfx.SrcOver)
			p.paint(a)
			p.b.PopLayer()
		}
		return
	}
	// The mask list's root space is the current local space.
	saved := p.b
	p.b = gfx.NewBuilder()
	p.paint(mask)
	list := p.b.Finish()
	p.b = saved
	p.b.PushMaskLayer(gfx.Mask{List: list, Mode: gfx.MaskAlpha}, 1, gfx.SrcOver)
	p.paint(a)
	p.b.PopLayer()
}

// solidAlpha returns the alpha of a node that is a solid color, possibly
// under transforms.
func (p *painter) solidAlpha(n tables.PaintTable) (float32, bool) {
	for {
		_, child, ok := p.transformOf(n)
		if !ok {
			break
		}
		n = child
	}
	switch v := n.(type) {
	case tables.PaintSolid:
		return p.paletteColor(v.PaletteIndex, f2dot14(v.Alpha)).A, true
	case tables.PaintVarSolid:
		return p.paletteColor(v.PaletteIndex, f2dot14(v.Alpha)+p.delta(v.VarIndexBase, 0)/16384).A, true
	}
	return 0, false
}

// ---------------------------------------------------------------------------
// Solids and gradients
// ---------------------------------------------------------------------------

// cstop is a color stop of a COLR color line, in font semantics.
type cstop struct {
	t float64
	c gfx.Color
}

// colorLine returns the sorted stops of a color line. Stops with equal
// offsets keep their order (hard stops).
func (p *painter) colorLine(cl tables.ColorLine) []cstop {
	out := make([]cstop, len(cl.ColorStops))
	for i, s := range cl.ColorStops {
		out[i] = cstop{f2dot14(s.StopOffset), p.paletteColor(s.PaletteIndex, f2dot14(s.Alpha))}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].t < out[j].t })
	return out
}

func (p *painter) varColorLine(cl tables.VarColorLine) []cstop {
	out := make([]cstop, len(cl.ColorStops))
	for i, s := range cl.ColorStops {
		t := f2dot14(s.StopOffset) + p.delta(s.VarIndexBase, 0)/16384
		a := f2dot14(s.Alpha) + p.delta(s.VarIndexBase, 1)/16384
		out[i] = cstop{t, p.paletteColor(s.PaletteIndex, a)}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].t < out[j].t })
	return out
}

// simplePaint converts a solid or gradient node to a gfx paint for the
// current local space; cover is the local area it must be correct over
// (used to unroll repeating gradients). It is nil when the node paints
// nothing.
func (p *painter) simplePaint(n tables.PaintTable, cover gfx.Rect) gfx.Paint {
	switch v := n.(type) {
	case tables.PaintSolid:
		return p.paletteColor(v.PaletteIndex, f2dot14(v.Alpha))
	case tables.PaintVarSolid:
		return p.paletteColor(v.PaletteIndex, f2dot14(v.Alpha)+p.delta(v.VarIndexBase, 0)/16384)
	case tables.PaintLinearGradient:
		return p.linear(p.colorLine(v.ColorLine), v.ColorLine.Extend,
			float64(v.X0), float64(v.Y0), float64(v.X1), float64(v.Y1), float64(v.X2), float64(v.Y2), cover)
	case tables.PaintVarLinearGradient:
		d := func(i uint32) float64 { return p.delta(v.VarIndexBase, i) }
		return p.linear(p.varColorLine(v.ColorLine), v.ColorLine.Extend,
			float64(v.X0)+d(0), float64(v.Y0)+d(1), float64(v.X1)+d(2), float64(v.Y1)+d(3), float64(v.X2)+d(4), float64(v.Y2)+d(5), cover)
	case tables.PaintRadialGradient:
		return p.radial(p.colorLine(v.ColorLine), v.ColorLine.Extend,
			float64(v.X0), float64(v.Y0), float64(v.Radius0), float64(v.X1), float64(v.Y1), float64(v.Radius1), cover)
	case tables.PaintVarRadialGradient:
		d := func(i uint32) float64 { return p.delta(v.VarIndexBase, i) }
		return p.radial(p.varColorLine(v.ColorLine), v.ColorLine.Extend,
			float64(v.X0)+d(0), float64(v.Y0)+d(1), float64(v.Radius0)+d(2), float64(v.X1)+d(3), float64(v.Y1)+d(4), float64(v.Radius1)+d(5), cover)
	case tables.PaintSweepGradient:
		return p.sweep(p.colorLine(v.ColorLine), v.ColorLine.Extend,
			float64(v.CenterX), float64(v.CenterY), f2dot14(v.StartAngle), f2dot14(v.EndAngle))
	case tables.PaintVarSweepGradient:
		d := func(i uint32) float64 { return p.delta(v.VarIndexBase, i) }
		return p.sweep(p.varColorLine(v.ColorLine), v.ColorLine.Extend,
			float64(v.CenterX)+d(0), float64(v.CenterY)+d(1), f2dot14(v.StartAngle)+d(2)/16384, f2dot14(v.EndAngle)+d(3)/16384)
	}
	return nil
}

// fontRange returns the range of the linear function t = a + b·x + c·y
// (font units) over the corners of the em-space rectangle r.
func (p *painter) fontRange(r gfx.Rect, a, b, c float64) (lo, hi float64) {
	u := p.f.upem
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, q := range [4]gfx.Point{{X: r.X0, Y: r.Y0}, {X: r.X1, Y: r.Y0}, {X: r.X0, Y: r.Y1}, {X: r.X1, Y: r.Y1}} {
		x, y := float64(q.X)*u, -float64(q.Y)*u
		t := a + float64(b*x) + float64(c*y)
		lo, hi = min(lo, t), max(hi, t)
	}
	return lo, hi
}

// linear converts PaintLinearGradient. The gradient line runs from p0 to
// p3, the projection of p1 on the line through p0 perpendicular to p0p2
// (OpenType COLR, "Linear gradients").
func (p *painter) linear(stops []cstop, ext tables.Extend, x0, y0, x1, y1, x2, y2 float64, cover gfx.Rect) gfx.Paint {
	if len(stops) == 0 {
		return nil
	}
	if len(stops) == 1 {
		return stops[0].c
	}
	// Perpendicular to p0p2.
	nx, ny := y2-y0, -(x2 - x0)
	nn := float64(nx*nx) + float64(ny*ny)
	dx, dy := x1-x0, y1-y0
	if nn == 0 || (dx == 0 && dy == 0) {
		return nil // degenerate: nothing is drawn (as Skia)
	}
	k := (float64(dx*nx) + float64(dy*ny)) / nn
	vx, vy := float64(k*nx), float64(k*ny) // p0 -> p3
	vv := float64(vx*vx) + float64(vy*vy)
	if vv == 0 {
		return nil
	}
	// t of a font-space point q: (q - p0)·v / |v|².
	tA, tB := stops[0].t, stops[len(stops)-1].t
	if ext != tables.ExtendPad {
		a := -(float64(x0*vx) + float64(y0*vy)) / vv
		tA, tB = p.fontRange(cover, a, vx/vv, vy/vv)
	}
	if tA == tB {
		tB = tA + 1e-6
	}
	gs := sampleColorLine(stops, ext, tA, tB)
	return gfx.LinearGradient{
		Start:  p.pt(x0+float64(tA*vx), y0+float64(tA*vy)),
		End:    p.pt(x0+float64(tB*vx), y0+float64(tB*vy)),
		Stops:  gs,
		Interp: gfx.InterpSRGB,
	}
}

// radial converts PaintRadialGradient. Concentric gradients map to
// gfx.RadialGradient, two-point ones to gfx.FocalGradient.
func (p *painter) radial(stops []cstop, ext tables.Extend, x0, y0, r0, x1, y1, r1 float64, cover gfx.Rect) gfx.Paint {
	if len(stops) == 0 {
		return nil
	}
	if len(stops) == 1 {
		return stops[0].c
	}
	if r0 < 0 {
		r0 = 0
	}
	if r1 < 0 {
		r1 = 0
	}
	if x0 != x1 || y0 != y1 {
		return p.twoPoint(stops, ext, x0, y0, r0, x1, y1, r1)
	}
	if r0 == r1 {
		return nil // degenerate (as Skia)
	}
	// A point at distance d from the center is on circle t = (d - r0) / (r1 - r0).
	// The gfx gradient maps offset o to d = o·R.
	var R float64
	if ext == tables.ExtendPad {
		R = max(r0+float64(stops[0].t*(r1-r0)), r0+float64(stops[len(stops)-1].t*(r1-r0)), 0)
	} else {
		u := p.f.upem
		for _, q := range [4]gfx.Point{{X: cover.X0, Y: cover.Y0}, {X: cover.X1, Y: cover.Y0}, {X: cover.X0, Y: cover.Y1}, {X: cover.X1, Y: cover.Y1}} {
			ddx, ddy := float64(q.X)*u-x0, -float64(q.Y)*u-y0
			R = max(R, math.Sqrt(float64(ddx*ddx)+float64(ddy*ddy)))
		}
	}
	if !(R > 0) {
		return stops[len(stops)-1].c
	}
	tA, tB := -r0/(r1-r0), (R-r0)/(r1-r0)
	return gfx.RadialGradient{
		Center: p.pt(x0, y0),
		Radius: gfx.Point{X: float32(R / p.f.upem), Y: float32(R / p.f.upem)},
		Stops:  sampleColorLine(stops, ext, tA, tB),
		Interp: gfx.InterpSRGB,
	}
}

// sweep converts PaintSweepGradient. COLR angles are counter-clockwise in
// font space (y up), in half turns with a bias of 1 (OpenType 1.9.1:
// degrees = (value + 1) * 180); a point's angle θ is taken in [0°, 360°)
// and t = (θ - start) / (end - start). A gfx.ConicGradient turns clockwise
// on screen from StartAngle (0 = up): with StartAngle at the +x axis
// (π/2), offset o corresponds to θ = 360°·(1 - o).
func (p *painter) sweep(stops []cstop, ext tables.Extend, cx, cy, start, end float64) gfx.Paint {
	if len(stops) == 0 {
		return nil
	}
	if len(stops) == 1 {
		return stops[0].c
	}
	a0, a1 := float64((start+1)*180), float64((end+1)*180)
	if a0 == a1 {
		return nil // degenerate
	}
	// t at o = 0 (θ = 360°) and at o = 1 (θ = 0°).
	tA, tB := (360-a0)/(a1-a0), (0-a0)/(a1-a0)
	return gfx.ConicGradient{
		Center:     p.pt(cx, cy),
		StartAngle: math.Pi / 2,
		Stops:      sampleColorLine(stops, ext, tA, tB),
		Interp:     gfx.InterpSRGB,
	}
}

// maxPeriods bounds how many periods of a repeating color line are
// unrolled; beyond it the line is padded instead.
const maxPeriods = 256

// sampleColorLine returns gfx stops for offsets o in [0, 1] such that the
// color at o is the COLR color line (with its extend mode) at
// t = tA + o·(tB - tA). stops are sorted and non-empty.
func sampleColorLine(stops []cstop, ext tables.Extend, tA, tB float64) []gfx.GradientStop {
	lo, hi := min(tA, tB), max(tA, tB)
	line := unroll(stops, ext, lo, hi)
	out := make([]gfx.GradientStop, 0, len(line)+2)
	add := func(t float64, c gfx.Color) {
		o := (t - tA) / (tB - tA)
		out = append(out, gfx.GradientStop{Offset: float32(min(max(o, 0), 1)), Color: c})
	}
	add(lo, evalLine(line, lo))
	for _, s := range line {
		if s.t > lo && s.t < hi {
			add(s.t, s.c)
		}
	}
	add(hi, evalLine(line, hi))
	if tA > tB {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}
	return out
}

// unroll expands a repeating or reflecting color line into explicit stops
// over [lo, hi]. A padded line, or one that would need more than
// maxPeriods periods, is returned unchanged (evalLine pads it).
func unroll(stops []cstop, ext tables.Extend, lo, hi float64) []cstop {
	t0, t1 := stops[0].t, stops[len(stops)-1].t
	period := t1 - t0
	if ext == tables.ExtendPad || !(period > 0) {
		return stops
	}
	k0, k1 := math.Floor((lo-t0)/period), math.Floor((hi-t0)/period)
	if k1-k0 >= maxPeriods {
		return stops
	}
	out := make([]cstop, 0, int(k1-k0+1)*len(stops))
	for k := k0; k <= k1; k++ {
		base := t0 + float64(k*period)
		if ext == tables.ExtendReflect && math.Mod(math.Abs(k), 2) == 1 {
			for i := len(stops) - 1; i >= 0; i-- {
				out = append(out, cstop{base + (t1 - stops[i].t), stops[i].c})
			}
			continue
		}
		for _, s := range stops {
			out = append(out, cstop{base + (s.t - t0), s.c})
		}
	}
	return out
}

// evalLine returns the color of a sorted stop list at t, padding beyond its
// ends. Colors are interpolated premultiplied, in sRGB.
func evalLine(line []cstop, t float64) gfx.Color {
	if t <= line[0].t {
		return line[0].c
	}
	n := len(line)
	if t >= line[n-1].t {
		return line[n-1].c
	}
	i := sort.Search(n, func(i int) bool { return line[i].t > t }) // line[i-1].t <= t < line[i].t
	a, b := line[i-1], line[i]
	if b.t == a.t {
		return b.c
	}
	f := (t - a.t) / (b.t - a.t)
	return lerpPremul(a.c, b.c, f)
}

// lerpPremul interpolates two colors premultiplied and returns the result
// unpremultiplied.
func lerpPremul(a, b gfx.Color, f float64) gfx.Color {
	l := func(x, y float64) float64 { return x + float64(f*(y-x)) }
	aa, ba := float64(a.A), float64(b.A)
	alpha := l(aa, ba)
	if alpha <= 0 {
		return gfx.Transparent
	}
	c := func(x, y float32) float32 {
		return float32(l(float64(float64(x)*aa), float64(float64(y)*ba)) / alpha)
	}
	return gfx.Color{R: c(a.R, b.R), G: c(a.G, b.G), B: c(a.B, b.B), A: float32(alpha)}
}

// ---------------------------------------------------------------------------
// Two-point radial gradients
// ---------------------------------------------------------------------------

// twoPoint converts a two-point conical gradient, the circles
// C(t) = (c0 + t·(c1 - c0), r0 + t·(r1 - r0)), to a gfx.FocalGradient
// between two circles of the same family, C(tA) at offset 0 and C(tB) at
// offset 1, whose stops sample the color line over [tA, tB]: the stops'
// range for pad, the range of t over the cover area for repeat and
// reflect. Both circles keep a radius of 0 or more (the parts of the range
// where r(t) < 0 paint nothing), so the family, and the rule that a point
// takes the largest t whose circle passes through it, are those of COLR.
func (p *painter) twoPoint(stops []cstop, ext tables.Extend, x0, y0, r0, x1, y1, r1 float64) gfx.Paint {
	dx, dy, dr := x1-x0, y1-y0, r1-r0
	tA, tB := stops[0].t, stops[len(stops)-1].t
	if ext != tables.ExtendPad {
		lo, hi, ok := p.twoPointRange(stops, x0, y0, r0, dx, dy, dr)
		if !ok {
			return nil
		}
		tA, tB = lo, hi
	}
	if tB-tA < 1e-6 {
		tB = tA + 1e-6
	}
	// Keep the radii of the end circles non-negative: r(t) >= 0 for
	// t >= tz when the circles grow, t <= tz when they shrink.
	switch {
	case dr > 0:
		tz := -r0 / dr
		if tB <= tz { // everything painted lies beyond tB: the last color
			tA, tB = tz, tz+1
		} else {
			tA = max(tA, tz)
		}
	case dr < 0:
		tz := -r0 / dr
		if tA >= tz { // everything painted lies before tA: the first color
			tA, tB = tz-1, tz
		} else {
			tB = min(tB, tz)
		}
	}
	circle := func(t float64) (gfx.Point, float32) {
		return p.pt(x0+float64(t*dx), y0+float64(t*dy)), float32(max(r0+float64(t*dr), 0) / p.f.upem)
	}
	fc, fr := circle(tA)
	ec, er := circle(tB)
	return gfx.FocalGradient{Focal: fc, FocalRadius: fr, Center: ec, Radius: er,
		Stops: sampleColorLine(stops, ext, tA, tB), Interp: gfx.InterpSRGB}
}

// twoPointRange returns the range of t that the points of the cover area
// can take: from the circle of radius 0 (or the line's start) to the first
// circle, searching outwards, that contains the whole cover area. Cones
// whose circles never cover it get a range of about 10^6.
func (p *painter) twoPointRange(stops []cstop, x0, y0, r0, dx, dy, dr float64) (lo, hi float64, ok bool) {
	cover, ok := p.localCover()
	if !ok {
		return 0, 0, false
	}
	u := p.f.upem
	covers := func(t float64) bool {
		cx, cy, r := x0+float64(t*dx), y0+float64(t*dy), r0+float64(t*dr)
		if r <= 0 {
			return false
		}
		for _, q := range [4]gfx.Point{{X: cover.X0, Y: cover.Y0}, {X: cover.X1, Y: cover.Y0}, {X: cover.X0, Y: cover.Y1}, {X: cover.X1, Y: cover.Y1}} {
			ex, ey := float64(q.X)*u-cx, -float64(q.Y)*u-cy
			if float64(ex*ex)+float64(ey*ey) > float64(r*r) {
				return false
			}
		}
		return true
	}
	t0, t1 := stops[0].t, stops[len(stops)-1].t
	grow := dr > 0 || (dr == 0 && r0 > 0)
	dir, far, near := 1.0, t1, t0
	if !grow {
		dir, far, near = -1, t0, t1
	}
	for step := 1.0; !covers(far) && step < 1e6; step *= 2 {
		far += dir * step
	}
	if dr != 0 {
		tz := -r0 / dr
		if grow {
			near = max(near, tz)
		} else {
			near = min(near, tz)
		}
	} else {
		near = far - dir*1e6
	}
	if !((far-near)*dir > 0) {
		return 0, 0, false // no circle with a positive radius
	}
	return min(near, far), max(near, far), true
}

// ---------------------------------------------------------------------------
// Path helpers (fusion-safe: every product of float32 values is computed
// in float64, where it is exact)
// ---------------------------------------------------------------------------

// transformPath returns path transformed by a.
func transformPath(path gfx.Path, a gfx.Affine) gfx.Path {
	pts := path.Points()
	tp := make([]gfx.Point, len(pts))
	a0, a1, a2, a3, a4, a5 := float64(a[0]), float64(a[1]), float64(a[2]), float64(a[3]), float64(a[4]), float64(a[5])
	for i, q := range pts {
		x, y := float64(q.X), float64(q.Y)
		tp[i] = gfx.Point{X: float32(a0*x + a2*y + a4), Y: float32(a1*x + a3*y + a5)}
	}
	return rebuild(path.Verbs(), tp)
}

// rebuild makes a path from verbs and points.
func rebuild(verbs []gfx.Verb, pts []gfx.Point) gfx.Path {
	var p gfx.Path
	i := 0
	for _, v := range verbs {
		switch v {
		case gfx.MoveTo:
			p.MoveTo(pts[i])
			i++
		case gfx.LineTo:
			p.LineTo(pts[i])
			i++
		case gfx.QuadTo:
			p.QuadTo(pts[i], pts[i+1])
			i += 2
		case gfx.CubicTo:
			p.CubicTo(pts[i], pts[i+1], pts[i+2])
			i += 3
		case gfx.Close:
			p.Close()
		}
	}
	return p
}
