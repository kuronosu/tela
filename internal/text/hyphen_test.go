package text

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestLiangPatterns checks Liang's algorithm on Knuth's examples
// (The TeXbook, appendix H): the patterns of "hyphenation" hyphenate it as
// TeX does, exceptions win over patterns, and the alphabet comes from the
// patterns with their capitals.
func TestLiangPatterns(t *testing.T) {
	// The patterns of TeX's hyphen.tex that apply to "hyphenation"
	// (TeXbook, appendix H).
	pats := `% comment
.hy3ph he2n hena4 hen5at 1na n2at 1tio 2io o2n
`
	h := NewHyphenator("en-US", pats, "ta-ble pres-ent")
	if err := h.Load(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ word, want string }{
		{"hyphenation", "hy-phen-ation"},
		{"table", "ta-ble"},
		{"present", "pres-ent"},
		{"hyphenation.", "hy-phen-ation."}, // trailing punctuation is not part of the word
		{"“hyphenation”", "“hy-phen-ation”"},
		{"Hyphenation", "Hyphenation"}, // capitalized English words do not break (Blink)
		{"hyph", "hyph"},               // shorter than 5 letters
		{"hyphenatioñ", "hyphenatioñ"}, // a letter the patterns do not know
	} {
		if got := show(c.word, h.Hyphenate(c.word)); got != c.want {
			t.Errorf("%s: %s, want %s", c.word, got, c.want)
		}
	}
	// Capitals map to the patterns' letters in other languages.
	es := NewHyphenator("es", pats, "")
	if got := show("HYPHENATION", es.Hyphenate("HYPHENATION")); got != "HY-PHEN-ATION" {
		t.Errorf("HYPHENATION: %s", got)
	}
	// A word longer than Minikin's 62 code units is not hyphenated.
	long := strings.Repeat("hyphenation", 6)
	if got := es.Hyphenate(long); len(got) != 0 {
		t.Errorf("a %d-letter word breaks at %v", len(long), got)
	}
}

// show writes word with a hyphen at each rune offset of pts.
func show(word string, pts []int) string {
	var b strings.Builder
	for i, r := range []rune(word) {
		if slices.Contains(pts, i) {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestHyphenLimits checks the limits of the points: 2 letters before a
// point, 3 after (Minikin's MIN_SUFFIX, stricter than Blink's 2).
func TestHyphenLimits(t *testing.T) {
	h := NewHyphenator("es", "a1b b1c c1d d1e e1f f1g", "")
	if got := show("abcdefg", h.Hyphenate("abcdefg")); got != "ab-cd-efg" && got != "ab-c-d-efg" {
		t.Errorf("abcdefg: %s", got)
	}
	if got := h.Hyphenate("abcdefg"); !slices.Equal(got, []int{2, 3, 4}) {
		t.Errorf("abcdefg: points %v, want [2 3 4]", got)
	}
}

func TestHyphenatorErrors(t *testing.T) {
	for _, c := range []struct{ pat, exc, err string }{
		{"a1b 1", "", "has no letters"},
		{"a1-b", "", "unexpected '-'"},
		{"a1b", "ta-1ble", "unexpected '1'"},
		{"a1b", "--", "has no letters"},
	} {
		h := NewHyphenator("xx", c.pat, c.exc)
		err := h.Load()
		if err == nil || !strings.Contains(err.Error(), c.err) {
			t.Errorf("%q %q: error %v, want %q", c.pat, c.exc, err, c.err)
		}
		if pts := h.Hyphenate("abababab"); len(pts) != 0 {
			t.Errorf("a broken dictionary hyphenates: %v", pts)
		}
	}
	if _, err := EncodeHyphenPatterns("a1b 1", ""); err == nil {
		t.Error("EncodeHyphenPatterns accepts a pattern without letters")
	}
}

// TestHyphenCompactForm checks that the compact form holds the same
// dictionary as the patterns it was made from.
func TestHyphenCompactForm(t *testing.T) {
	pats := ".hy3ph he2n hena4 hen5at 1na n2at 1tio 2io o2n"
	enc, err := EncodeHyphenPatterns(pats+"\n% a comment\n", "ta-ble")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(enc, hyphenMagic) {
		t.Fatal("no magic")
	}
	p, e, err := decodeHyphenPatterns(enc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Fields(e)[0] != "ta-ble" || len(strings.Fields(p)) != 9 || strings.Contains(p, "%") {
		t.Errorf("decoded %q %q", p, e)
	}
	a, b := NewHyphenator("en", pats, "ta-ble"), NewHyphenator("en", enc, "")
	for _, w := range []string{"hyphenation", "table", "nation"} {
		if x, y := a.Hyphenate(w), b.Hyphenate(w); !slices.Equal(x, y) {
			t.Errorf("%s: %v from the patterns, %v from the compact form", w, x, y)
		}
	}
	if err := NewHyphenator("en", hyphenMagic+"garbage", "").Load(); err == nil {
		t.Error("a malformed compact form loads")
	}
	// The packages' dictionaries load.
	for _, h := range []*Hyphenator{testHyphenUS(), testHyphenGB(), testHyphenES(), testHyphenFR()} {
		if err := h.Load(); err != nil {
			t.Errorf("%s: %v", h.Lang(), err)
		}
	}
}

// TestHyphenLocale checks Blink's mapping of content languages to
// dictionaries and the collection's choice.
func TestHyphenLocale(t *testing.T) {
	for _, c := range []struct{ lang, want string }{
		{"en", "en-us"}, {"en-US", "en-us"}, {"en-us", "en-us"}, {"en-GB", "en-gb"},
		{"en-AU", "en-gb"}, {"en-CA", "en-gb"}, {"en-PR", "en-us"}, {"en-Latn-US", "en-gb"},
		{"es", "es"}, {"es-MX", "es"}, {"es-419", "es"}, {"fr-CA", "fr"},
		{"de", "de-1996"}, {"de-DE", "de-1996"}, {"de-1901", "de-1901"}, {"de-LI-1901", "de-ch-1901"},
		{"no", "nb"}, {"nb-NO", "nb"}, {"mn", "mn-cyrl"}, {"am", "und-ethi"}, {"zh-Hans", "zh"},
		{"EN_us", "en-us"},
	} {
		if got := hyphenLocale(c.lang); got != c.want {
			t.Errorf("hyphenLocale(%q) = %q, want %q", c.lang, got, c.want)
		}
	}
	us, gb, es := testHyphenUS(), testHyphenGB(), testHyphenES()
	c := NewCollection()
	if c.hyphenator("en") != nil {
		t.Error("a collection without dictionaries has one")
	}
	c.SetHyphenators([]*Hyphenator{us, gb, es})
	for _, k := range []struct {
		lang string
		want *Hyphenator
	}{{"en", us}, {"en-US", us}, {"en-GB", gb}, {"en-NZ", gb}, {"es-AR", es}, {"fr", nil}, {"", nil}} {
		if got := c.hyphenator(k.lang); got != k.want {
			t.Errorf("%q: %v, want %v", k.lang, got, k.want)
		}
	}
	// Without the mapped dictionary, one of the same language.
	c.SetHyphenators([]*Hyphenator{us})
	if c.hyphenator("en-GB") != us {
		t.Error("en-GB does not fall back to en-US")
	}
	if got := c.Hyphenators(); len(got) != 1 || got[0] != us {
		t.Errorf("Hyphenators() = %v", got)
	}
	c.SetHyphenators(nil)
	if c.hyphenator("en") != nil || c.Hyphenators() != nil {
		t.Error("SetHyphenators(nil) keeps dictionaries")
	}
}

// TestSoftHyphenLayout checks soft hyphens: a break opportunity with a
// hyphen at the end of the line, except with hyphens: none; invisible
// elsewhere.
func TestSoftHyphenLayout(t *testing.T) {
	coll := hyphenCollection(t)
	text := "Ex\u00adtra\u00ador\u00addi\u00adnary"
	lay := func(h Hyphens, w float32) *Layout {
		st := hyStyle(16, h, "")
		l := NewLayout(coll, &Paragraph{Text: text, Default: st})
		l.BreakLines(w)
		return l
	}
	one := lay(HyphensManual, 1000)
	if one.NumLines() != 1 || one.Line(0).Hyphen {
		t.Fatalf("unbroken: %d lines, hyphen %v", one.NumLines(), one.Line(0).Hyphen)
	}
	full := one.Width()
	l := lay(HyphensManual, full-1)
	if l.NumLines() != 2 || !l.Line(0).Hyphen || l.Line(1).Hyphen {
		t.Fatalf("broken: %q", lineTexts(l))
	}
	if got := lineTexts(l); got[0] != "Ex\u00adtra\u00ador\u00addi\u00ad" {
		t.Errorf("first line %q", got[0])
	}
	// The hyphen: U+2010 of Inter, after the last letter, in the width.
	h := l.hyphenFor(0)
	if g, _ := h.face.GlyphIndex('\u2010'); h.glyphs[0].id != g || h.face.Typeface().Family() != "Inter" {
		t.Errorf("hyphen glyph %d of %s", h.glyphs[0].id, h.face.Typeface().Family())
	}
	ln := l.Line(0)
	if ln.HyphenX <= 0 || ln.HyphenX+h.width-ln.Width > 0.001 {
		t.Errorf("hyphen at %.3f (%.3f wide), line %.3f wide", ln.HyphenX, h.width, ln.Width)
	}
	// It is painted: one glyph more than the letters.
	b := gfx.NewBuilder()
	l.Paint(b, gfx.Pt(0, 0))
	if n := countGlyphs(b.Finish()); n != len([]rune(strings.ReplaceAll(text, "\u00ad", "")))+1 {
		t.Errorf("%d glyphs painted", n)
	}
	// hyphens: none: no break at soft hyphens, nothing drawn for them.
	if l := lay(HyphensNone, full-1); l.NumLines() != 1 {
		t.Errorf("hyphens: none breaks: %q", lineTexts(l))
	}
	// The min-content width is the widest part with its hyphen.
	if mc := lay(HyphensManual, 0).MinContentWidth(); !(mc < full/2) {
		t.Errorf("min-content %.3f of %.3f", mc, full)
	}
	if mc := lay(HyphensNone, 0).MinContentWidth(); mc != full {
		t.Errorf("min-content with hyphens: none %.3f, want %.3f", mc, full)
	}
}

// TestHyphenationEdges breaks paragraphs with soft hyphens and hyphens:
// auto at many widths, with every wrap style, alignment and white-space
// mode: the lines cover the text in order, painting works, and a
// hyphenated line ends where a hyphen may be.
func TestHyphenationEdges(t *testing.T) {
	coll := hyphenCollection(t)
	texts := []string{
		"",
		"\u00ad",
		"a\u00ad\u00adb\u00ad",
		"internationalization",
		"Ex\u00adtra\u00ador\u00addi\u00adnary inter\u00adna\u00adtion\u00adal\u00adiza\u00adtion, “hyphenation.”",
		"The typographical considerations\tof extraordinary\nhyphenation 😀 العربية\u00adهي لغة 日本語の文章 internationalization",
		strings.Repeat("hyphenation", 7) + " " + strings.Repeat("a", 70),
	}
	for _, s := range texts {
		for _, para := range []ParagraphStyle{
			{}, {Align: AlignJustify}, {WrapStyle: TextWrapBalance}, {WrapStyle: TextWrapPretty},
			{Direction: DirectionRTL, Align: AlignCenter}, {WhiteSpace: WhiteSpacePreWrap},
			{OverflowWrap: OverflowWrapBreakWord}, {OverflowWrap: OverflowWrapAnywhere}, {LineClamp: 2},
			{TextOverflow: TextOverflowEllipsis, WhiteSpace: WhiteSpaceNowrap},
		} {
			p := &Paragraph{Style: para, Default: hyText(HyphensAuto, "en")}
			p.AddText(s, hyText(HyphensAuto, "en"))
			p.AddBox(InlineBox{Width: 12, Height: 12})
			p.AddText(" tail\u00adword", hyText(HyphensManual, "en", func(st *TextStyle) { st.LetterSpacing = 2 }))
			l := NewLayout(coll, p)
			_ = l.MinContentWidth()
			for w := float32(0); w <= 320; w += 9 {
				l.BreakLines(w)
				next := 0
				for i := range l.NumLines() {
					ln := l.Line(i)
					if ln.ClusterStart != next {
						t.Fatalf("%q at %g: line %d starts at cluster %d, want %d", s, w, i, ln.ClusterStart, next)
					}
					next = ln.ClusterEnd
					if ln.Hyphen && (ln.Ellipsis || ln.ClusterEnd <= ln.ClusterStart) {
						t.Fatalf("%q at %g: line %d has a hyphen and an ellipsis or no content", s, w, i)
					}
				}
				b := gfx.NewBuilder()
				l.PaintScaled(b, gfx.Pt(0, 0), 2)
				b.Finish()
			}
		}
	}
}

func countGlyphs(dl *gfx.DisplayList) int {
	n := 0
	for i := range dl.Len() {
		if r, ok := dl.At(i).(gfx.DrawGlyphRun); ok {
			n += len(r.Glyphs)
		}
	}
	return n
}

// TestAutoHyphenLayout checks hyphens: auto: the word that does not fit
// breaks at its last point that fits, the hyphen included, only with a
// dictionary of the text's language; BreakLines never shapes.
func TestAutoHyphenLayout(t *testing.T) {
	coll := hyphenCollection(t)
	lay := func(lang string, h Hyphens, w float32) *Layout {
		st := hyStyle(16, h, lang)
		l := NewLayout(coll, &Paragraph{Text: "Words and internationalization here", Default: st})
		l.BreakLines(w)
		return l
	}
	l := lay("en", HyphensAuto, 150)
	if got := fmt.Sprint(lineTexts(l)); got != "[Words and interna tionalization here]" {
		t.Errorf("lines %s", got)
	}
	if !l.Line(0).Hyphen || l.Line(1).Hyphen {
		t.Errorf("hyphens %v %v", l.Line(0).Hyphen, l.Line(1).Hyphen)
	}
	for i := range l.NumLines() {
		if ln := l.Line(i); ln.Width > 150+fitEpsilon {
			t.Errorf("line %d is %.3f wide", i, ln.Width)
		}
	}
	shaped := shapeCalls.Load()
	l.BreakLines(120)
	l.BreakLines(80)
	if shapeCalls.Load() != shaped {
		t.Error("BreakLines shaped")
	}
	// The last word of a paragraph does not break (Blink), unless it is
	// alone on its line.
	last := NewLayout(coll, &Paragraph{Text: "Words and internationalization", Default: hyStyle(16, HyphensAuto, "en")})
	last.BreakLines(150)
	if got := fmt.Sprint(lineTexts(last)); got != "[Words and  internationalization]" {
		t.Errorf("last word: %s", got)
	}
	last.BreakLines(100)
	if !last.Line(1).Hyphen {
		t.Errorf("a last word alone on its line does not break: %q", lineTexts(last))
	}
	for _, c := range []struct {
		lang string
		h    Hyphens
	}{{"", HyphensAuto}, {"ja", HyphensAuto}, {"en", HyphensManual}} {
		if l := lay(c.lang, c.h, 150); l.Line(0).Hyphen || l.Line(1).Hyphen {
			t.Errorf("lang %q hyphens %d hyphenates: %q", c.lang, c.h, lineTexts(l))
		}
	}
}
