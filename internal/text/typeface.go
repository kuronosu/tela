package text

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

// Style is the slant of a typeface or of a request (CSS font-style).
type Style uint8

const (
	// StyleNormal is an upright face. It is the zero value.
	StyleNormal Style = iota
	// StyleItalic is a cursive or slanted face designed as italic.
	StyleItalic
	// StyleOblique is a slanted version of the upright design.
	StyleOblique
)

// String returns the CSS keyword.
func (s Style) String() string {
	switch s {
	case StyleNormal:
		return "normal"
	case StyleItalic:
		return "italic"
	case StyleOblique:
		return "oblique"
	}
	return fmt.Sprintf("Style(%d)", s)
}

// Axis is a variation axis of a typeface, in design units.
type Axis struct {
	Tag               ot.Tag
	Min, Default, Max float32
}

// Typeface is one parsed font file (or one font of a collection): the
// immutable data shared by every Face instance made from it. It is safe for
// concurrent use.
type Typeface struct {
	family  string // the name it is registered under, as given
	name    string // human-readable name, such as "Inter Italic"
	ft      *font.Font
	upem    int
	nGlyphs int

	style   Style
	weight  [3]float32 // min, default, max (CSS weight, 1-1000)
	stretch [3]float32 // min, default, max (CSS percentage)
	axes    []Axis

	glyf   *glyfTables // nil when the outlines are CFF or CFF2
	hmtx   tables.Hmtx
	hvar   *tables.HVAR
	mvar   *tables.MVAR
	os2    tables.Os2
	hasOS2 bool
	hhea   tables.Hhea
	post   tables.Post
	head   tables.Head
	names  tables.Name

	// cff extracts CFF and CFF2 outlines through go-text; a font.Face is not
	// safe for concurrent use, so it is guarded by cffMu. CFF2 faces set the
	// instance coordinates before each call.
	cffMu sync.Mutex
	cff   *font.Face

	facesMu sync.Mutex
	faces   map[string]*Face // interned instances, keyed by coordinates
}

var (
	tagWght = ot.MustNewTag("wght")
	tagWdth = ot.MustNewTag("wdth")
	tagOpsz = ot.MustNewTag("opsz")
	tagSlnt = ot.MustNewTag("slnt")
	tagItal = ot.MustNewTag("ital")
)

// ParseTypefaces parses a font file (TrueType, OpenType or a collection)
// and returns its typefaces. family overrides the family name stored in the
// font when it is not empty. data must not be modified afterwards.
func ParseTypefaces(data []byte, family string) ([]*Typeface, error) {
	lds, err := ot.NewLoaders(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("text: parsing font: %w", err)
	}
	out := make([]*Typeface, 0, len(lds))
	for i, ld := range lds {
		tf, err := newTypeface(ld, family)
		if err != nil {
			return nil, fmt.Errorf("text: parsing font %d of the file: %w", i, err)
		}
		out = append(out, tf)
	}
	return out, nil
}

func newTypeface(ld *ot.Loader, family string) (*Typeface, error) {
	ft, err := font.NewFont(ld)
	if err != nil {
		return nil, err
	}
	tf := &Typeface{ft: ft, upem: int(ft.Upem()), faces: map[string]*Face{}}

	raw := func(tag string) []byte {
		b, _ := ld.RawTable(ot.MustNewTag(tag))
		return b
	}
	tf.head, _, err = font.LoadHeadTable(ld, nil)
	if err != nil {
		return nil, err
	}
	maxp, _, err := tables.ParseMaxp(raw("maxp"))
	if err != nil {
		return nil, err
	}
	tf.nGlyphs = int(maxp.NumGlyphs)

	if hhea, _, err := tables.ParseHhea(raw("hhea")); err == nil {
		tf.hhea = hhea
		n := int(hhea.NumOfLongMetrics)
		if n <= tf.nGlyphs {
			tf.hmtx, _, _ = tables.ParseHmtx(raw("hmtx"), n, tf.nGlyphs-n)
		}
	}
	if os2, _, err := tables.ParseOs2(raw("OS/2")); err == nil {
		tf.os2, tf.hasOS2 = os2, true
	}
	tf.post, _, _ = tables.ParsePost(raw("post"))
	tf.names, _, _ = tables.ParseName(raw("name"))

	if fvar, _, err := tables.ParseFvar(raw("fvar")); err == nil {
		for _, a := range fvar.FvarRecords.Axis {
			tf.axes = append(tf.axes, Axis{Tag: a.Tag, Min: a.Minimum, Default: a.Default, Max: a.Maximum})
		}
	}
	if len(tf.axes) > 0 {
		if hvar, _, err := tables.ParseHVAR(raw("HVAR")); err == nil {
			tf.hvar = &hvar
		}
		if mvar, _, err := tables.ParseMVAR(raw("MVAR")); err == nil && len(mvar.ValueRecords) > 0 {
			tf.mvar = &mvar
		}
	}

	if glyfRaw := raw("glyf"); glyfRaw != nil && !ld.HasTable(ot.MustNewTag("CFF ")) && !ld.HasTable(ot.MustNewTag("CFF2")) {
		loca, err := tables.ParseLoca(raw("loca"), tf.nGlyphs, tf.head.IndexToLocFormat == 1)
		if err == nil {
			if glyf, err := tables.ParseGlyf(glyfRaw, loca); err == nil {
				tf.glyf = &glyfTables{glyf: glyf, hmtx: tf.hmtx, upem: tf.upem}
			}
		}
		if tf.glyf != nil && len(tf.axes) > 0 {
			if gvar, _, err := tables.ParseGvar(raw("gvar")); err == nil && len(gvar.GlyphVariationDatas) == len(tf.glyf.glyf) {
				for _, t := range gvar.SharedTuples.SharedTuples {
					tf.glyf.sharedTuples = append(tf.glyf.sharedTuples, t.Values)
				}
				tf.glyf.variations = gvar.GlyphVariationDatas
			}
		}
	}
	if tf.glyf == nil {
		tf.cff = font.NewFace(ft)
	}

	tf.describe(family)
	return tf, nil
}

// describe fills the family, name, style, weight and stretch.
func (tf *Typeface) describe(family string) {
	desc := tf.ft.Describe()
	if family == "" {
		family = desc.Family
	}
	tf.family = family
	sub := tf.nameString(17)
	if sub == "" {
		sub = tf.nameString(2)
	}
	tf.name = family
	if sub != "" && !strings.EqualFold(sub, "regular") {
		tf.name += " " + sub
	}

	// Style: OS/2 fsSelection ITALIC (bit 0) or OBLIQUE (bit 9), else the
	// head macStyle italic bit, else the italic angle.
	switch {
	case tf.hasOS2 && tf.os2.FsSelection&(1<<9) != 0:
		tf.style = StyleOblique
	case tf.hasOS2 && tf.os2.FsSelection&1 != 0, tf.head.MacStyle&2 != 0:
		tf.style = StyleItalic
	case desc.Aspect.Style == font.StyleItalic:
		tf.style = StyleItalic
	}

	w := float32(400)
	if tf.hasOS2 && tf.os2.USWeightClass >= 1 && tf.os2.USWeightClass <= 1000 {
		w = float32(tf.os2.USWeightClass)
	}
	tf.weight = [3]float32{w, w, w}
	s := float32(100)
	if tf.hasOS2 {
		if v := int(tf.os2.USWidthClass); v >= 1 && v <= 9 {
			s = [...]float32{50, 62.5, 75, 87.5, 100, 112.5, 125, 150, 200}[v-1]
		}
	}
	tf.stretch = [3]float32{s, s, s}
	for _, a := range tf.axes {
		switch a.Tag {
		case tagWght:
			tf.weight = [3]float32{a.Min, a.Default, a.Max}
		case tagWdth:
			tf.stretch = [3]float32{a.Min, a.Default, a.Max}
		}
	}
}

func (tf *Typeface) nameString(id tables.NameID) string {
	return strings.TrimSpace(tf.names.Name(id))
}

// Family returns the family name the typeface is registered under.
func (tf *Typeface) Family() string { return tf.family }

// Name returns a human-readable name, such as "Inter Italic".
func (tf *Typeface) Name() string { return tf.name }

// Style returns the slant of the typeface.
func (tf *Typeface) Style() Style { return tf.style }

// WeightRange returns the CSS weights the typeface covers: its wght axis
// for a variable font, else a single value from OS/2.
func (tf *Typeface) WeightRange() (lo, hi float32) { return tf.weight[0], tf.weight[2] }

// StretchRange returns the CSS font-stretch percentages the typeface
// covers: its wdth axis, else a single value from OS/2.
func (tf *Typeface) StretchRange() (lo, hi float32) { return tf.stretch[0], tf.stretch[2] }

// Axes returns the variation axes. The slice must not be modified.
func (tf *Typeface) Axes() []Axis { return tf.axes }

// UnitsPerEm returns the font's design units per em.
func (tf *Typeface) UnitsPerEm() int { return tf.upem }

// NumGlyphs returns the number of glyphs.
func (tf *Typeface) NumGlyphs() int { return tf.nGlyphs }

// IsColor reports whether the font has COLR color glyphs.
func (tf *Typeface) IsColor() bool { return tf.ft.COLR != nil }

// HasRune reports whether the cmap maps r to a glyph.
func (tf *Typeface) HasRune(r rune) bool {
	_, ok := tf.ft.NominalGlyph(r)
	return ok
}

// GoTextFont returns the go-text font. It is read-only and safe for
// concurrent use; callers must not modify it.
func (tf *Typeface) GoTextFont() *font.Font { return tf.ft }

func (tf *Typeface) axisIndex(tag ot.Tag) int {
	for i, a := range tf.axes {
		if a.Tag == tag {
			return i
		}
	}
	return -1
}

// os2Higher returns the int16 at offset off of the OS/2 data beyond version
// 0 (sxHeight is at 8, sCapHeight at 10, both from version 2).
func (tf *Typeface) os2Higher(off int) (int16, bool) {
	if !tf.hasOS2 || tf.os2.Version < 2 || len(tf.os2.HigherVersionData) < off+2 {
		return 0, false
	}
	return int16(binary.BigEndian.Uint16(tf.os2.HigherVersionData[off:])), true
}

// mvarDelta returns the MVAR delta of the metric tag at coords.
func (tf *Typeface) mvarDelta(tag string, coords []tables.Coord) float64 {
	if tf.mvar == nil || len(coords) == 0 {
		return 0
	}
	t := ot.MustNewTag(tag)
	recs := tf.mvar.ValueRecords
	for i, j := 0, len(recs); i < j; {
		h := i + (j-i)/2
		switch {
		case t < recs[h].ValueTag:
			j = h
		case recs[h].ValueTag < t:
			i = h + 1
		default:
			return varStoreDelta(&tf.mvar.ItemVariationStore, recs[h].Index, coords)
		}
	}
	return 0
}

// advance returns the advance width of gid in font units at coords, rounded
// to an integer like HarfBuzz does for variable fonts.
func (tf *Typeface) advance(gid tables.GlyphID, coords []tables.Coord) float64 {
	base := float64(tf.upem / 2)
	if !tf.hmtx.IsEmpty() {
		base = float64(tf.hmtx.Advance(gid))
	}
	if len(coords) == 0 {
		return base
	}
	if tf.hvar != nil {
		idx := advanceIndex(&tf.hvar.AdvanceWidthMapping, gid)
		return base + math.Round(varStoreDelta(&tf.hvar.ItemVariationStore, idx, coords))
	}
	if tf.glyf != nil {
		pts := tf.glyf.points(gid, coords)
		ph := pts[len(pts)-phantomCount:]
		return max(0, math.Round(ph[phantomRight].x-ph[phantomLeft].x))
	}
	return base
}

func advanceIndex(m *tables.DeltaSetMapping, gid tables.GlyphID) tables.VariationStoreIndex {
	if len(m.Map) == 0 {
		return tables.VariationStoreIndex{DeltaSetInner: uint16(gid)}
	}
	if int(gid) >= len(m.Map) {
		return m.Map[len(m.Map)-1]
	}
	return m.Map[gid]
}
