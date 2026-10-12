package text

import (
	"github.com/kuronosu/tela/internal/gfx"
)

// Metrics are the font-wide metrics of a Face, in em units: multiply by the
// font size (or use Scale) to get pixels. Vertical offsets follow the y-down
// em space of the glyphs: positive values are below the baseline.
type Metrics struct {
	// UnitsPerEm is the font's design grid.
	UnitsPerEm int
	// Ascent is the distance from the baseline to the top of the line box
	// (positive, above the baseline), Descent the distance from the
	// baseline to its bottom (positive, below the baseline) and LineGap the
	// extra leading. CSS line-height: normal is Ascent + Descent + LineGap.
	//
	// They come from OS/2 sTypo* when the font sets USE_TYPO_METRICS
	// (fsSelection bit 7), else from hhea, else from OS/2 sTypo*, with the
	// MVAR deltas hasc, hdsc and hlgp: HarfBuzz's rule, which is what
	// Chromium (through Skia and FreeType) and Firefox use on Linux.
	Ascent, Descent, LineGap float32
	// CapHeight and XHeight are the heights of flat capitals and of the
	// lowercase x above the baseline (positive). From OS/2 version 2
	// (with MVAR cpht and xhgt), else measured on the outlines of 'H' and
	// 'x'.
	CapHeight, XHeight float32
	// UnderlineOffset is the y offset of the top of the underline (post
	// table; positive: below the baseline) and UnderlineThickness its
	// thickness. MVAR undo and unds apply.
	UnderlineOffset, UnderlineThickness float32
	// StrikeoutOffset is the y offset of the top of the strikeout line (OS/2;
	// negative: above the baseline) and StrikeoutThickness its thickness.
	// MVAR stro and strs apply.
	StrikeoutOffset, StrikeoutThickness float32
}

// Scale returns m with every length multiplied by size.
func (m Metrics) Scale(size float32) Metrics {
	s := func(v float32) float32 { return v * size }
	return Metrics{
		UnitsPerEm: m.UnitsPerEm,
		Ascent:     s(m.Ascent), Descent: s(m.Descent), LineGap: s(m.LineGap),
		CapHeight: s(m.CapHeight), XHeight: s(m.XHeight),
		UnderlineOffset: s(m.UnderlineOffset), UnderlineThickness: s(m.UnderlineThickness),
		StrikeoutOffset: s(m.StrikeoutOffset), StrikeoutThickness: s(m.StrikeoutThickness),
	}
}

// Metrics returns the font-wide metrics in em units.
func (f *Face) Metrics() Metrics { return f.metrics }

func (f *Face) computeMetrics() Metrics {
	tf := f.tf
	c := f.coords
	u := f.upem
	em := func(v float64) float32 { return float32(v / u) }
	m := Metrics{UnitsPerEm: tf.upem}

	var asc, desc, gap float64
	useTypo := tf.hasOS2 && tf.os2.FsSelection&(1<<7) != 0
	switch {
	case useTypo:
		asc, desc, gap = float64(tf.os2.STypoAscender), float64(tf.os2.STypoDescender), float64(tf.os2.STypoLineGap)
	case tf.hhea.Ascender != 0 || tf.hhea.Descender != 0:
		asc, desc, gap = float64(tf.hhea.Ascender), float64(tf.hhea.Descender), float64(tf.hhea.LineGap)
	case tf.hasOS2:
		asc, desc, gap = float64(tf.os2.STypoAscender), float64(tf.os2.STypoDescender), float64(tf.os2.STypoLineGap)
	default:
		asc, desc = float64(0.8*u), float64(-0.2*u)
	}
	asc += tf.mvarDelta("hasc", c)
	desc += tf.mvarDelta("hdsc", c)
	gap += tf.mvarDelta("hlgp", c)
	// Ascender is above the baseline and descender below, whatever the
	// signs in the font (HarfBuzz's _fix_ascender_descender).
	if asc < 0 {
		asc = -asc
	}
	if desc > 0 {
		desc = -desc
	}
	m.Ascent, m.Descent, m.LineGap = em(asc), em(-desc), em(max(gap, 0))

	if v, ok := tf.os2Higher(10); ok && v > 0 {
		m.CapHeight = em(float64(v) + tf.mvarDelta("cpht", c))
	} else {
		m.CapHeight = f.runeTop('H')
	}
	if v, ok := tf.os2Higher(8); ok && v > 0 {
		m.XHeight = em(float64(v) + tf.mvarDelta("xhgt", c))
	} else {
		m.XHeight = f.runeTop('x')
	}

	m.UnderlineOffset = em(-(float64(tf.post.UnderlinePosition) + tf.mvarDelta("undo", c)))
	m.UnderlineThickness = em(float64(tf.post.UnderlineThickness) + tf.mvarDelta("unds", c))
	if tf.hasOS2 {
		m.StrikeoutOffset = em(-(float64(tf.os2.YStrikeoutPosition) + tf.mvarDelta("stro", c)))
		m.StrikeoutThickness = em(float64(tf.os2.YStrikeoutSize) + tf.mvarDelta("strs", c))
	}
	// Fallbacks for fonts that leave the decorations unset.
	if m.UnderlineThickness <= 0 {
		m.UnderlineThickness = 0.05
	}
	if m.StrikeoutThickness <= 0 {
		m.StrikeoutThickness = m.UnderlineThickness
	}
	if m.StrikeoutOffset == 0 {
		m.StrikeoutOffset = -m.XHeight / 2
	}
	return m
}

// runeTop returns the height of the top of r's outline above the baseline,
// in em, or 0.
func (f *Face) runeTop(r rune) float32 {
	g, ok := f.GlyphIndex(r)
	if !ok {
		return 0
	}
	b := f.entry(g).bounds
	if b == (gfx.Rect{}) {
		return 0
	}
	return -b.Y0
}
