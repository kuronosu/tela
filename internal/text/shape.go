package text

import (
	"math"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/go-text/typesetting/bidi"
	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/segmenter"
	"github.com/kuronosu/tela/internal/gfx"
)

// Shaping runs go-text's HarfBuzz port, whose faces and buffers are not safe
// for concurrent use: shapeMu serializes every use of them.
var (
	shapeMu  sync.Mutex
	shapeBuf *harfbuzz.Buffer // guarded by shapeMu
)

// shapeCalls counts HarfBuzz shaping calls, for tests that check that
// BreakLines never shapes.
var shapeCalls atomic.Int64

// hbFont returns the HarfBuzz font of f, created on first use. HarfBuzz
// works in font units (the scale is left at units per em), so positions
// are integers that layout scales to the font size. shapeMu must be held.
func (f *Face) hbFont() *harfbuzz.Font {
	if f.hb == nil {
		f.hb = harfbuzz.NewFont(f.ShapingFace())
	}
	return f.hb
}

// advanceUnits returns the advance of g in font units, the fusion-safe
// value of Face.Advance (an integer: variable advances are rounded).
func (f *Face) advanceUnits(g gfx.GlyphID) int32 {
	return int32(math.Round(float64(f.entry(g).advance) * f.upem))
}

// textSegments holds the UAX #14 and UAX #29 boundaries of the processed
// text: bit 0 a grapheme boundary before rune i, bit 1 a soft line break
// opportunity, bit 2 a mandatory break, bits 3 and 4 the start and the end
// of a word (a UAX #29 word segment that starts with a letter or a digit)
// before rune i.
const (
	segGrapheme = 1 << iota
	segSoft
	segMandatory
	segWordStart
	segWordEnd
)

func segmentText(runes []rune) []uint8 {
	out := make([]uint8, len(runes)+1)
	if len(runes) == 0 {
		return out
	}
	var seg segmenter.Segmenter
	seg.Init(runes)
	git := seg.GraphemeIterator()
	for git.Next() {
		out[git.Grapheme().Offset] |= segGrapheme
	}
	out[len(runes)] |= segGrapheme
	lit := seg.LineIterator()
	for lit.Next() {
		ln := lit.Line()
		end := ln.Offset + len(ln.Text)
		if ln.IsMandatoryBreak {
			out[end] |= segMandatory
		} else {
			out[end] |= segSoft
		}
	}
	markWords(&seg, runes, out)
	return out
}

// isForcedBreakChar reports the characters laid out as forced breaks (with
// no glyph): preserved newlines and the other mandatory breaks of UAX #14.
func isForcedBreakChar(r rune) bool {
	switch r {
	case '\n', '\r', 0x0B, 0x0C, 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// shape runs bidi, script itemization, font fallback and HarfBuzz over the
// processed text and builds the runs, clusters and glyphs.
func (l *Layout) shape() {
	pt := &l.text
	n := len(pt.runes)
	switch l.para.Direction {
	case DirectionRTL:
		l.base = 1
	case DirectionAuto:
		l.base = firstStrongDirection(pt.runes)
	}
	var bp bidi.Paragraph
	levels := bidiLevels(&bp, pt.runes, l.base, make([]uint8, 0, n))
	scripts := make([]language.Script, n)
	resolveScripts(pt.runes, scripts)
	segs := segmentText(pt.runes)
	if len(l.boxes) > l.nboxes {
		l.tagBreaks(segs)
	}
	l.segs = segs

	shapeMu.Lock()
	defer shapeMu.Unlock()
	if shapeBuf == nil {
		shapeBuf = harfbuzz.NewBuffer()
	}

	var fruns []Run
	for i := 0; i < n; {
		r := pt.runes[i]
		si := &l.sinfo[pt.style[i]]
		switch {
		case pt.box[i] >= 0:
			b := l.boxes[pt.box[i]]
			f := cfBox
			if int(pt.box[i]) >= l.nboxes {
				f |= cfTag
			}
			l.special(runBox, i, levels[i], si, b.Width, f)
			l.runs[len(l.runs)-1].box = pt.box[i]
			i++
			continue
		case r == '\t' && !l.para.WhiteSpace.collapses():
			l.special(runTab, i, levels[i], si, 0, cfTab|cfBidiWS)
			i++
			continue
		case isForcedBreakChar(r):
			l.special(runNewline, i, levels[i], si, 0, cfNewline|cfBidiWS)
			i++
			continue
		}
		// A text item: same level, script and shaping style.
		j := i + 1
		for j < n && pt.box[j] < 0 && levels[j] == levels[i] && scripts[j] == scripts[i] && (pt.split == nil || !pt.split[j]) &&
			!(pt.runes[j] == '\t' && !l.para.WhiteSpace.collapses()) && !isForcedBreakChar(pt.runes[j]) &&
			(pt.style[j] == pt.style[i] || l.styles[pt.style[j]].shapesLike(&l.styles[pt.style[i]])) {
			j++
		}
		st := &l.styles[pt.style[i]]
		fruns = l.coll.Itemize(pt.runes, i, j, st.query(), si.size, scripts[i], fruns[:0])
		for _, fr := range fruns {
			l.shapeRun(fr.Face, int32(fr.Start), int32(fr.End), levels[i], scripts[i], si, segs)
		}
		i = j
	}
	l.boxClus = make([]int32, len(l.boxes))
	for i := range l.boxClus {
		l.boxClus[i] = -1
	}
	for ci := range l.clusters {
		c := &l.clusters[ci]
		if c.flags&cfBox != 0 {
			l.boxClus[l.runs[c.run].box] = int32(ci)
		}
	}
	l.setBreaks(segs)
}

// special appends a run of one cluster without glyphs (inline box, tab or
// forced break) at rune i.
func (l *Layout) special(kind runKind, i int, level uint8, si *styleInfo, adv float32, flags clusterFlag) {
	ri := int32(len(l.runs))
	ci := int32(len(l.clusters))
	g := int32(len(l.glyphs))
	rn := run{kind: kind, level: level, face: si.face, size: si.size, r0: int32(i), r1: int32(i + 1), c0: ci, c1: ci + 1, box: -1}
	rn.ascent, rn.descent, rn.gap = si.ascent, si.descent, 0
	if si.normal && si.face != nil {
		rn.gap = qround(si.face.Metrics().LineGap, si.size)
	}
	l.runs = append(l.runs, rn)
	l.clusters = append(l.clusters, cluster{
		r0: int32(i), r1: int32(i + 1), g0: g, g1: g, run: ri,
		style: l.text.style[i], flags: flags, graph: 1, adv: adv,
	})
}

// shapeRun shapes the runes [r0, r1) with face f and appends the run, its
// clusters and its glyphs. shapeMu must be held.
func (l *Layout) shapeRun(f *Face, r0, r1 int32, level uint8, script language.Script, si *styleInfo, segs []uint8) {
	shapeCalls.Add(1)
	pt := &l.text
	hb := f.hbFont()
	buf := shapeBuf
	buf.Clear()
	buf.Props.Direction = harfbuzz.LeftToRight
	if level%2 == 1 {
		buf.Props.Direction = harfbuzz.RightToLeft
	}
	buf.Props.Script = script
	buf.Props.Language = si.lang
	if r0 == 0 {
		buf.Flags |= harfbuzz.Bot
	}
	if int(r1) == len(pt.runes) {
		buf.Flags |= harfbuzz.Eot
	}
	buf.AddRunes(pt.runes, int(r0), int(r1-r0))
	buf.Shape(hb, si.features)

	m := f.Metrics()
	ri := int32(len(l.runs))
	rn := run{kind: runText, level: level, face: f, size: si.size, r0: r0, r1: r1, box: -1,
		ascent: qround(m.Ascent, si.size), descent: qround(m.Descent, si.size), gap: qround(m.LineGap, si.size)}

	// Glyphs, scaled from font units to px. The nominal advances come from
	// Face.Advance, which is fusion-safe; HarfBuzz's own (go-text's float32
	// HVAR code) can round differently on another architecture.
	scale := float64(si.size) / f.upem
	g0 := int32(len(l.glyphs))
	for k := range buf.Info {
		gid := buf.Info[k].Glyph
		p := buf.Pos[k]
		xa := p.XAdvance
		if xa != 0 {
			xa += f.advanceUnits(gfx.GlyphID(gid)) - hb.GlyphHAdvance(gid)
		}
		l.glyphs = append(l.glyphs, glyphData{
			id:  gfx.GlyphID(gid),
			adv: float32(float64(xa) * scale),
			dx:  float32(float64(p.XOffset) * scale),
			dy:  -float32(float64(p.YOffset) * scale),
		})
	}

	// Cluster boundaries: HarfBuzz cluster starts that are also grapheme
	// boundaries and source character boundaries.
	cnt := int(r1 - r0)
	starts := make([]bool, cnt+1)
	starts[0], starts[cnt] = true, true
	for k := range buf.Info {
		if c := buf.Info[k].Cluster - int(r0); c >= 0 && c < cnt {
			starts[c] = true
		}
	}
	for p := 1; p < cnt; p++ {
		q := int(r0) + p
		if starts[p] && (segs[q]&segGrapheme == 0 || pt.src[q] <= pt.src[q-1]) {
			starts[p] = false
		}
	}
	atomOf := make([]int32, cnt)
	c0 := int32(len(l.clusters))
	for p := 0; p < cnt; {
		q := p + 1
		for q < cnt && !starts[q] {
			q++
		}
		a := int32(len(l.clusters)) - c0
		for k := p; k < q; k++ {
			atomOf[k] = a
		}
		graph := uint16(1)
		for k := p + 1; k < q; k++ {
			if segs[int(r0)+k]&segGrapheme != 0 && pt.src[int(r0)+k] > pt.src[int(r0)+k-1] {
				graph++
			}
		}
		l.clusters = append(l.clusters, cluster{r0: r0 + int32(p), r1: r0 + int32(q), g0: -1, g1: -1, run: ri,
			style: pt.style[int(r0)+p], graph: graph})
		p = q
	}
	for k := range buf.Info {
		c := buf.Info[k].Cluster - int(r0)
		c = min(max(c, 0), cnt-1)
		cl := &l.clusters[c0+atomOf[c]]
		gi := g0 + int32(k)
		if cl.g0 < 0 {
			cl.g0, cl.g1 = gi, gi+1
		} else {
			cl.g0, cl.g1 = min(cl.g0, gi), max(cl.g1, gi+1)
		}
	}
	for ci := c0; ci < int32(len(l.clusters)); ci++ {
		cl := &l.clusters[ci]
		if cl.g0 < 0 {
			cl.g0, cl.g1 = g0, g0
		}
		var adv float32
		for g := cl.g0; g < cl.g1; g++ {
			adv += l.glyphs[g].adv
		}
		cl.flags = clusterFlags(pt.runes[cl.r0:cl.r1])
		if cl.flags&cfInvisible != 0 {
			// Controls and lone format characters draw nothing and take no
			// space (HarfBuzz hides default ignorables but not C0 and C1
			// controls; Blink treats those as zero-width spaces).
			cl.g1 = cl.g0
			adv = 0
		}
		st := &l.styles[cl.style]
		if cl.flags&(cfInvisible|cfCursive) == 0 {
			adv += st.LetterSpacing
		}
		if cl.flags&cfWordSep != 0 {
			adv += st.WordSpacing
		}
		cl.adv = adv
	}
	rn.c0, rn.c1 = c0, int32(len(l.clusters))
	l.runs = append(l.runs, rn)
}

// clusterFlags classifies the runes of a cluster.
func clusterFlags(rs []rune) clusterFlag {
	var f clusterFlag
	r := rs[0]
	if len(rs) == 1 {
		switch {
		case r == ' ':
			f |= cfSpace | cfWordSep
		case r == 0xA0 || r == 0x1361 || r == 0x10100 || r == 0x10101 || r == 0x1039F || r == 0x1091F:
			f |= cfWordSep
		case r == 0x2007 || r == 0x202F:
			// no-break spaces: neither hang nor separate words
		case unicode.Is(unicode.Zs, r):
			f |= cfOtherSpace
		}
	}
	ws, inv := true, true
	for _, c := range rs {
		if !bidiWhitespace(c) {
			ws = false
		}
		if !(unicode.Is(unicode.Cf, c) || unicode.Is(unicode.Cc, c)) {
			inv = false
		}
	}
	if ws {
		f |= cfBidiWS
	}
	if inv {
		f |= cfInvisible
	}
	switch sc := language.LookupScript(r); {
	case sc == language.Han || sc == language.Hiragana || sc == language.Katakana || sc == language.Bopomofo,
		r >= 0xFF01 && r <= 0xFF60, r >= 0x3001 && r <= 0x303F:
		if !unicode.Is(unicode.Zs, r) {
			f |= cfCJK
		}
	}
	if unicode.IsLetter(r) || unicode.IsNumber(r) {
		f |= cfLetter
	}
	if cursiveScript(language.LookupScript(r)) {
		f |= cfCursive
	}
	return f
}

// cursiveScript reports the scripts whose letters join, which take no
// letter-spacing (CSS Text 3, section 8.2.1; Chromium does the same).
func cursiveScript(s language.Script) bool {
	switch s {
	case language.Arabic, language.Syriac, language.Mongolian, language.Nko, language.Phags_Pa,
		language.Mandaic, language.Manichaean, language.Psalter_Pahlavi, language.Sogdian,
		language.Old_Uyghur, language.Hanifi_Rohingya, language.Chorasmian, language.Adlam:
		return true
	}
	return false
}
