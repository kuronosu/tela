package text

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/kuronosu/tela/internal/gfx"
)

// Instance selects the variation coordinates of a Face. Zero fields leave
// the corresponding axis at its default. Values are clamped to the axes;
// axes the typeface does not have are ignored.
type Instance struct {
	// Weight is the CSS font-weight (wght axis).
	Weight float32
	// Stretch is the CSS font-stretch percentage (wdth axis).
	Stretch float32
	// OpticalSize is the opsz value. CSS font-optical-sizing: auto uses the
	// font size in CSS pixels; Collection rounds it to whole pixels so that
	// a typeface has at most one instance per pixel size (see Collection).
	OpticalSize float32
	// Oblique sets the slnt axis to -14 degrees (CSS "oblique" default
	// angle) on a typeface that has one.
	Oblique bool
	// Italic sets the ital axis to 1 on a typeface that has one.
	Italic bool
	// Variations are explicit axis values (CSS font-variation-settings),
	// applied last.
	Variations []font.Variation
}

// Face is an instance of a Typeface at fixed variation coordinates. It
// implements gfx.Font: glyph outlines and color glyphs in em space (y down,
// origin at the pen position on the baseline).
//
// Faces are interned: Typeface.Face returns the same *Face for the same
// coordinates, so a Face can be compared by pointer and its FontKey is
// stable for the life of the process. A Face is safe for concurrent use,
// except for the go-text face returned by ShapingFace.
type Face struct {
	tf     *Typeface
	coords []tables.Coord // normalized; nil at the default instance
	key    uint64
	name   string
	upem   float64

	metrics Metrics

	glyphs []atomic.Pointer[glyphEntry]
	colors []atomic.Pointer[colorEntry] // nil unless the font has COLR

	shapingOnce sync.Once
	shaping     *font.Face
}

// glyphEntry is the cached, immutable data of one glyph.
type glyphEntry struct {
	path    gfx.Path // em space; empty when the glyph has no outline
	bounds  gfx.Rect // control bounds of path
	advance float32  // em
}

// Face returns the instance of tf selected by inst. The same coordinates
// always return the same Face.
func (tf *Typeface) Face(inst Instance) *Face {
	coords, design := tf.coordsFor(inst)
	key := coordsKey(coords)
	tf.facesMu.Lock()
	defer tf.facesMu.Unlock()
	if f := tf.faces[key]; f != nil {
		return f
	}
	f := &Face{
		tf:     tf,
		coords: coords,
		key:    gfx.NewFontKey(),
		upem:   float64(tf.upem),
		glyphs: make([]atomic.Pointer[glyphEntry], tf.nGlyphs),
	}
	if tf.ft.COLR != nil {
		f.colors = make([]atomic.Pointer[colorEntry], tf.nGlyphs)
	}
	f.name = tf.name + tf.instanceName(coords, design)
	f.metrics = f.computeMetrics()
	tf.faces[key] = f
	return f
}

// coordsFor returns the normalized coordinates of inst, or nil for the
// default instance, and the design coordinates.
func (tf *Typeface) coordsFor(inst Instance) ([]tables.Coord, []float32) {
	if len(tf.axes) == 0 {
		return nil, nil
	}
	design := make([]float32, len(tf.axes))
	for i, a := range tf.axes {
		design[i] = a.Default
	}
	set := func(tag ot.Tag, v float32) {
		if i := tf.axisIndex(tag); i >= 0 && v == v && !math.IsInf(float64(v), 0) {
			design[i] = min(max(v, tf.axes[i].Min), tf.axes[i].Max)
		}
	}
	if inst.Weight > 0 {
		set(tagWght, inst.Weight)
	}
	if inst.Stretch > 0 {
		set(tagWdth, inst.Stretch)
	}
	if inst.OpticalSize > 0 {
		set(tagOpsz, inst.OpticalSize)
	}
	if inst.Oblique {
		set(tagSlnt, -14)
	}
	if inst.Italic {
		set(tagItal, 1)
	}
	for _, v := range inst.Variations {
		set(v.Tag, v.Value)
	}
	coords := tf.ft.NormalizeVariations(design)
	for _, c := range coords {
		if c != 0 {
			return coords, design
		}
	}
	return nil, design
}

func coordsKey(coords []tables.Coord) string {
	b := make([]byte, 2*len(coords))
	for i, c := range coords {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(c))
	}
	return string(b)
}

// instanceName describes the axes away from their defaults, such as
// " (wght 650)". Two design coordinates that normalize to the same
// instance share the name of the first one requested.
func (tf *Typeface) instanceName(coords []tables.Coord, design []float32) string {
	if coords == nil {
		return ""
	}
	var parts []string
	for i, a := range tf.axes {
		if coords[i] != 0 {
			parts = append(parts, fmt.Sprintf("%s %g", a.Tag, design[i]))
		}
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// Typeface returns the typeface the face is an instance of.
func (f *Face) Typeface() *Typeface { return f.tf }

// Coords returns the normalized variation coordinates (nil at the default
// instance). The slice must not be modified.
func (f *Face) Coords() []tables.Coord { return f.coords }

// FontKey implements gfx.Font.
func (f *Face) FontKey() uint64 { return f.key }

// FontName implements gfx.Font.
func (f *Face) FontName() string { return f.name }

// String returns FontName.
func (f *Face) String() string { return f.name }

// UnitsPerEm returns the font's design units per em.
func (f *Face) UnitsPerEm() int { return f.tf.upem }

// GlyphIndex returns the nominal glyph of r (cmap), without substitutions.
func (f *Face) GlyphIndex(r rune) (gfx.GlyphID, bool) {
	g, ok := f.tf.ft.NominalGlyph(r)
	return gfx.GlyphID(g), ok
}

// VariationGlyph returns the glyph of r followed by the variation selector
// vs, from the cmap format 14 subtable.
func (f *Face) VariationGlyph(r, vs rune) (gfx.GlyphID, bool) {
	g, ok := f.tf.ft.VariationGlyph(r, vs)
	return gfx.GlyphID(g), ok
}

// HasRune reports whether the cmap maps r to a glyph.
func (f *Face) HasRune(r rune) bool { return f.tf.HasRune(r) }

// Advance returns the horizontal advance of glyph g in em, at the face's
// coordinates. Variable advances are rounded to whole font units, as in
// HarfBuzz.
func (f *Face) Advance(g gfx.GlyphID) float32 { return f.entry(g).advance }

// ShapingFace returns a go-text face set to the face's coordinates, for
// shaping with go-text's HarfBuzz port. It is created once and shared: a
// go-text face is not safe for concurrent use, so callers must serialize
// their use of it (text layout runs on the UI thread). Raster workers never
// touch it.
func (f *Face) ShapingFace() *font.Face {
	f.shapingOnce.Do(func() {
		f.shaping = font.NewFace(f.tf.ft)
		if f.coords != nil {
			f.shaping.SetCoords(f.coords)
		}
	})
	return f.shaping
}

// GlyphOutline implements gfx.Font.
func (f *Face) GlyphOutline(g gfx.GlyphID) (gfx.Path, bool) {
	e := f.entry(g)
	return e.path, !e.path.IsEmpty()
}

// GlyphBounds implements gfx.Font: the control bounds of the outline, or
// for a color glyph its COLR clip box (or the bounds of its paint graph
// when the font has no clip box for it).
func (f *Face) GlyphBounds(g gfx.GlyphID) gfx.Rect {
	if c := f.color(g); c != nil {
		return c.bounds
	}
	return f.entry(g).bounds
}

// ColorGlyph implements gfx.Font for COLR (v0 and v1) glyphs. Bitmap
// (CBDT, sbix) and SVG glyphs are not supported: it returns nil for them.
func (f *Face) ColorGlyph(g gfx.GlyphID, fg gfx.Color) *gfx.DisplayList {
	c := f.color(g)
	if c == nil {
		return nil
	}
	return c.list(f, g, fg)
}

var emptyEntry = &glyphEntry{}

// entry returns the cached data of glyph g, loading it on first use. Two
// goroutines may load the same glyph at once; the first stored result wins
// (both are identical).
func (f *Face) entry(g gfx.GlyphID) *glyphEntry {
	if int(g) >= len(f.glyphs) {
		return emptyEntry
	}
	slot := &f.glyphs[g]
	if e := slot.Load(); e != nil {
		return e
	}
	e := f.loadGlyph(tables.GlyphID(g))
	if !slot.CompareAndSwap(nil, e) {
		e = slot.Load()
	}
	return e
}

func (f *Face) loadGlyph(gid tables.GlyphID) *glyphEntry {
	e := &glyphEntry{}
	tf := f.tf
	if tf.glyf != nil {
		pts := tf.glyf.points(gid, f.coords)
		n := len(pts) - phantomCount
		e.path = glyfPath(pts[:n], f.upem)
		adv := float64(tf.upem / 2)
		if !tf.hmtx.IsEmpty() {
			adv = float64(tf.hmtx.Advance(gid))
		}
		if f.coords != nil {
			if tf.hvar != nil {
				adv = tf.advance(gid, f.coords)
			} else {
				ph := pts[n:]
				adv = max(0, math.Round(ph[phantomRight].x-ph[phantomLeft].x))
			}
		}
		e.advance = float32(adv / f.upem)
	} else {
		e.path = f.cffPath(gid)
		e.advance = float32(tf.advance(gid, f.coords) / f.upem)
	}
	if !e.path.IsEmpty() {
		e.bounds = e.path.ControlBounds()
	}
	return e
}

// cffPath extracts a CFF or CFF2 outline through go-text.
//
// CFF charstrings only add and subtract, so CFF outlines are the same on
// every architecture; CFF2 blending (variable CFF2 fonts) multiplies and
// adds in float32 inside go-text and may differ in the last bits across
// architectures. No embedded font uses CFF2.
func (f *Face) cffPath(gid tables.GlyphID) gfx.Path {
	tf := f.tf
	tf.cffMu.Lock()
	tf.cff.SetCoords(f.coords)
	out, ok := tf.cff.GlyphDataOutline(font.GID(gid))
	tf.cffMu.Unlock()
	if !ok {
		return gfx.Path{}
	}
	var p gfx.Path
	em := func(q ot.SegmentPoint) gfx.Point {
		return gfx.Point{X: float32(float64(q.X) / f.upem), Y: float32(-float64(q.Y) / f.upem)}
	}
	open := false
	for _, s := range out.Segments {
		switch s.Op {
		case ot.SegmentOpMoveTo:
			if open {
				p.Close()
			}
			p.MoveTo(em(s.Args[0]))
			open = true
		case ot.SegmentOpLineTo:
			if open {
				p.LineTo(em(s.Args[0]))
			}
		case ot.SegmentOpQuadTo:
			if open {
				p.QuadTo(em(s.Args[0]), em(s.Args[1]))
			}
		case ot.SegmentOpCubeTo:
			if open {
				p.CubicTo(em(s.Args[0]), em(s.Args[1]), em(s.Args[2]))
			}
		}
	}
	if open {
		p.Close()
	}
	return p
}
