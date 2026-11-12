package text

import (
	"encoding/binary"

	"github.com/go-text/typesetting/font"
)

// go-text's font.NewFont parses glyf whole (every point of every glyph,
// plus a copy of the table) and, for a variable font, gvar: about 3.5 MiB
// of heap and 6 ms for Inter, 14 MiB and 20 ms for the COLRv1 emoji font,
// while Tela loads TrueType outlines itself (glyf.go). What go-text does
// with them, as Tela calls it (horizontal text, ppem never set, no
// outlines asked of go-text for a glyf font):
//
//   - glyph extents, for HarfBuzz's fallback mark positioning, which runs
//     only when GPOS is not applied (no GPOS lookups, kerx, or a Hebrew
//     run, Common characters next to Hebrew included, with a GPOS lacking
//     the 'hebr' script), and for its line metrics
//     fallback, which Tela does not call. At the default instance (and in
//     a static font) they are the glyph header's bounding box and the hmtx
//     side bearing; at other instances, the bounds of the varied points;
//   - the advances of a variable font without HVAR (gvar's phantom points);
//   - contour points: GPOS anchors of format 2 (read only when ppem is
//     set), kerx control point actions, GDEF caret format 2 (not called),
//     vertical origins (vertical text).
//
// So go-text gets a glyf of headers (goTextGlyfStub: each glyph's bounding
// box, no contours, 12 bytes) that gives every extent it computes at the
// default instance, unless the font has kerx; and, for a variable font
// with HVAR and GPOS lookups, no gvar: its extents at other instances are
// then wrong, and read only for a Hebrew run when the GPOS has no Hebrew
// script, which Face.hbFontFor shapes with a second parse of the font that
// has its own tables (made once, when such a run first needs it). Other
// variable fonts keep both tables. TestGoTextOutlineStubs and
// TestHebrewRunsShapeWithOutlines check the extents, advances and shaping.
// A glyf table with a malformed glyph, which go-text would drop whole,
// gets the stub's extents instead.

// goTextFont parses src with go-text (font.NewFont), with the outline
// stubs when tf, whose glyf, axes and hvar are set, allows them.
func goTextFont(src tableSource, tf *Typeface) (*font.Font, error) {
	ft, out, err := goTextFontOutlines(src, tf)
	if err == nil && out.hideGvar {
		tf.goTextNoGvar, tf.goTextSrc = true, src
	}
	return ft, err
}

// goTextFullFont returns go-text's parse of the font with its own outline
// tables, made on first use, or nil if it fails; tf.goTextNoGvar must be
// set.
func (tf *Typeface) goTextFullFont() *font.Font {
	tf.goTextFullOnce.Do(func() {
		tf.goTextFull, _ = font.NewFont(goTextLoader(tf.goTextSrc, goTextOutlines{}))
	})
	return tf.goTextFull
}

// goTextFontOutlines is goTextFont, which also returns what go-text was
// given of the outlines.
func goTextFontOutlines(src tableSource, tf *Typeface) (*font.Font, goTextOutlines, error) {
	var out goTextOutlines
	if tf.glyf != nil && src.data != nil && src.table(tagKerx) == nil &&
		(len(tf.axes) == 0 || (tf.hvar != nil && src.table(tagGPOS) != nil)) {
		if glyf, loca, ok := goTextGlyfStub(tf.glyf); ok {
			out = goTextOutlines{glyf: glyf, loca: loca, hideGvar: len(tf.axes) > 0}
		}
	}
	ft, err := font.NewFont(goTextLoader(src, out))
	if err == nil && out.hideGvar && ft.GPOS.Lookups == nil {
		// A second parse, for a variable font whose marks HarfBuzz may
		// position from glyph extents at any instance.
		out = goTextOutlines{}
		ft, err = font.NewFont(goTextLoader(src, out))
	}
	return ft, out, err
}

// goTextGlyfStub returns glyf and loca tables, in the font's loca format,
// in which every glyph of g is its header alone: the same number of
// contours sign aside (zero contours), the same bounding box, no
// instructions, so 12 bytes. ok is false when a glyph's data is shorter
// than a header (go-text would drop the font's glyf) or the stub does not
// fit a short loca.
func goTextGlyfStub(g *glyfTables) (glyf, loca []byte, ok bool) {
	n := g.nGlyphs
	size := 0
	for i := range n {
		start, end := g.offset(i), g.offset(i+1) // checked by newGlyfTables
		switch {
		case start == end:
		case end-start < 10:
			return nil, nil, false
		default:
			size += 12
		}
	}
	if !g.longLoca && size > 2*0xFFFF {
		return nil, nil, false
	}
	glyf = make([]byte, 0, size)
	if g.longLoca {
		loca = make([]byte, 0, 4*(n+1))
	} else {
		loca = make([]byte, 0, 2*(n+1))
	}
	put := func(off int) {
		if g.longLoca {
			loca = binary.BigEndian.AppendUint32(loca, uint32(off))
		} else {
			loca = binary.BigEndian.AppendUint16(loca, uint16(off/2))
		}
	}
	for i := range n {
		put(len(glyf))
		start, end := g.offset(i), g.offset(i+1)
		if start == end {
			continue
		}
		glyf = append(glyf, 0, 0)                        // numberOfContours
		glyf = append(glyf, g.glyf[start+2:start+10]...) // xMin, yMin, xMax, yMax
		glyf = append(glyf, 0, 0)                        // instructionLength
	}
	put(len(glyf))
	return glyf, loca, true
}
