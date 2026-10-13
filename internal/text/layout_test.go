package text

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/go-text/typesetting/harfbuzz"
	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/gfx"
)

var inter16 = TextStyle{Families: []string{"Inter", "Noto Sans Arabic", "Noto Sans JP", "Noto Color Emoji"}, Size: 16, LineHeight: LineHeightPixels(24)}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s:\n got %q\nwant %q", what, got, want)
	}
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// TestBidiReorder checks visual order (UAX #9 L2) with numbers, including
// an Arabic-Indic number after Latin text (levels 0 and 2 in one direction
// run) and L1 for trailing white space.
func TestBidiReorder(t *testing.T) {
	for _, c := range []struct {
		dir  Direction
		text string
		want string
	}{
		// Arabic letters are AL, so the digits after them are AN (level 2):
		// the number stays left to right, left of the Arabic word.
		{DirectionLTR, "abc مرحبا 123 def", "abc 123 ابحرم def"},
		// European digits after Hebrew stay EN at level 2.
		{DirectionLTR, "abc שלום 123 def", "abc 123 םולש def"},
		// Arabic-Indic digits after Latin: AN at level 2 next to level 0,
		// with no level 1 between; they keep their order.
		{DirectionLTR, "abc ١٢٣ def", "abc ١٢٣ def"},
		// An Arabic word and a number that follows it in an LTR paragraph,
		// then Latin text: "123" sits left of the Arabic word, not after "abc".
		{DirectionLTR, "x مرحبا ١٢٣ abc", "x ١٢٣ ابحرم abc"},
		// Right-to-left paragraph with a Latin phrase and a number.
		{DirectionRTL, "مرحبا Tela 2.0 بكم", "مكب Tela 2.0 ابحرم"},
		// Brackets around Latin text take the paragraph direction (rule
		// N0: the context before them is Arabic), so they are mirrored
		// glyphs at level 1: the source ")" is on the left.
		{DirectionRTL, "مرحبا (abc) بكم", "مكب )abc( ابحرم"},
		// Auto direction: the first strong character is Arabic.
		{DirectionAuto, "123 مرحبا abc", "abc ابحرم 123"},
		{DirectionAuto, "abc مرحبا", "abc ابحرم"},
	} {
		l := simple(t, c.text, ParagraphStyle{Direction: c.dir}, inter16, float32(math.Inf(1)))
		if got := visual(l)[0]; got != c.want {
			t.Errorf("%q (direction %d): visual %q, want %q", c.text, c.dir, got, c.want)
		}
	}
	// Trailing white space goes to the end edge of the paragraph's
	// direction (rule L1), even after an RTL word in an LTR paragraph.
	l := simple(t, "abc مرحبا   \nx", ParagraphStyle{WhiteSpace: WhiteSpacePre}, inter16, float32(math.Inf(1)))
	if got := visual(l)[0]; got != "abc ابحرم   \n" {
		t.Errorf("L1: visual %q", got)
	}
	if !l.RTL() == false {
		t.Error("RTL() for an LTR paragraph")
	}
}

// TestArabicJoining checks that Arabic letters take their joining forms:
// the glyphs of a word differ from the isolated forms, including the
// letters that need ccmp decomposition first (noon, qaf, teh marbuta: lost
// by go-text v0.3.5's set digest bug).
func TestArabicJoining(t *testing.T) {
	coll := layoutCollection(t)
	glyphs := func(l *Layout, ci int) []gfx.GlyphID {
		cl := l.Cluster(ci)
		var gs []gfx.GlyphID
		for g := cl.GlyphStart; g < cl.GlyphEnd; g++ {
			gs = append(gs, l.Glyph(g).ID)
		}
		slices.Sort(gs)
		return gs
	}
	isolated := func(r rune) []gfx.GlyphID {
		return glyphs(simple(t, string(r), ParagraphStyle{}, inter16, 1000), 0)
	}
	for _, c := range []struct {
		word   string
		joined int // letters in a joining form
	}{
		{"بسم", 3}, {"نص", 2}, {"قلب", 3}, {"تتصل", 4},
		// Dal and reh do not join to the next letter, so the teh marbuta
		// after reh is isolated.
		{"مدرسة", 4},
	} {
		l := NewLayout(coll, &Paragraph{Text: c.word, Default: inter16})
		rs := []rune(c.word)
		if l.NumClusters() != len(rs) {
			t.Fatalf("%q: %d clusters for %d letters", c.word, l.NumClusters(), len(rs))
		}
		joined := 0
		for i := range l.NumClusters() {
			cl := l.Cluster(i)
			r, _ := utf8DecodeRune(c.word[cl.TextStart:])
			if !slices.Equal(glyphs(l, i), isolated(r)) {
				joined++
			}
		}
		if joined != c.joined {
			t.Errorf("%q: %d of %d letters in a joining form, want %d", c.word, joined, len(rs), c.joined)
		}
	}
	// "نص" is 25.94 px wide at 16 px in HarfBuzz; with isolated forms
	// (go-text v0.3.5) it was 32.58 px.
	l := NewLayout(coll, &Paragraph{Text: "نص", Default: inter16})
	if math.Abs(float64(l.Width())-25.94) > 0.01 {
		t.Errorf(`"نص" is %g px wide, want 25.94 (joined forms)`, l.Width())
	}
}

func utf8DecodeRune(s string) (rune, int) {
	for _, r := range s {
		return r, len(string(r))
	}
	return 0, 0
}

// TestLigatureAndLetterSpacing checks Inter's "->" ligature (calt): one
// cluster of two graphemes, and two clusters when letter-spacing disables
// optional ligatures, as in Chromium.
func TestLigatureAndLetterSpacing(t *testing.T) {
	l := simple(t, "a->b", ParagraphStyle{}, inter16, 1000)
	if l.NumClusters() != 3 {
		t.Fatalf("%d clusters, want 3 (a, ->, b)", l.NumClusters())
	}
	lig := l.Cluster(1)
	if lig.Graphemes != 2 || lig.GlyphEnd-lig.GlyphStart != 1 || l.Text()[lig.TextStart:lig.TextEnd] != "->" {
		t.Errorf("ligature cluster %+v", lig)
	}
	ls := inter16
	ls.LetterSpacing = 2
	l2 := simple(t, "a->b", ParagraphStyle{}, ls, 1000)
	if l2.NumClusters() != 4 {
		t.Errorf("with letter-spacing: %d clusters, want 4", l2.NumClusters())
	}
	// Letter-spacing is added after every cluster, the last one included.
	plain := simple(t, "abc", ParagraphStyle{}, inter16, 1000)
	spaced := simple(t, "abc", ParagraphStyle{}, ls, 1000)
	if !near(spaced.Width(), plain.Width()+6) {
		t.Errorf("letter-spacing 2px on 3 letters: %g, want %g", spaced.Width(), plain.Width()+6)
	}
	// font-feature-settings: liga=0 does not undo calt; calt=0 does.
	nocalt := inter16
	nocalt.Features = []Feature{{"calt", 0}}
	if l := simple(t, "a->b", ParagraphStyle{}, nocalt, 1000); l.NumClusters() != 4 {
		t.Errorf("calt 0: %d clusters, want 4", l.NumClusters())
	}
}

// TestClusters checks that combining marks, flags, emoji sequences and
// Arabic vowel marks stay in the cluster of their base, and that clusters
// map to source bytes.
func TestClusters(t *testing.T) {
	for _, c := range []struct {
		text string
		want []string
	}{
		{"e\u0301a", []string{"e\u0301", "a"}},
		{"🇯🇵🇧🇷", []string{"🇯🇵", "🇧🇷"}},
		{"👍🏽x", []string{"👍🏽", "x"}},
		{"👨‍👩‍👧‍👦", []string{"👨‍👩‍👧‍👦"}},
		{"بِسْمِ", []string{"بِ", "سْ", "مِ"}},
		{"1️⃣", []string{"1️⃣"}},
	} {
		l := simple(t, c.text, ParagraphStyle{}, inter16, 1000)
		var got []string
		for i := range l.NumClusters() {
			cl := l.Cluster(i)
			got = append(got, c.text[cl.TextStart:cl.TextEnd])
		}
		eq(t, "clusters of "+c.text, got, c.want)
	}
}

// TestWhiteSpace checks the processed text and line breaks of the
// white-space modes.
func TestWhiteSpace(t *testing.T) {
	const text = "  a  b\t c\n  d   e  "
	inf := float32(math.Inf(1))
	for _, c := range []struct {
		ws   WhiteSpace
		want []string
	}{
		{WhiteSpaceNormal, []string{"a b c d e "}},
		{WhiteSpaceNowrap, []string{"a b c d e "}},
		{WhiteSpacePreLine, []string{"a b c\n", "d e "}},
		{WhiteSpacePre, []string{"  a  b\t c\n", "  d   e  "}},
		{WhiteSpacePreWrap, []string{"  a  b\t c\n", "  d   e  "}},
		{WhiteSpaceBreakSpaces, []string{"  a  b\t c\n", "  d   e  "}},
	} {
		l := simple(t, text, ParagraphStyle{WhiteSpace: c.ws}, inter16, inf)
		var got []string
		for i := range l.NumLines() {
			ln := l.Line(i)
			var b strings.Builder
			for ci := ln.ClusterStart; ci < ln.ClusterEnd; ci++ {
				b.WriteRune(l.text.runes[l.clusters[ci].r0])
			}
			got = append(got, b.String())
		}
		eq(t, "white-space "+[]string{"normal", "nowrap", "pre", "pre-wrap", "pre-line", "break-spaces"}[c.ws], got, c.want)
	}
	// Collapsed runs map to the source: the first cluster covers the
	// removed leading spaces, the kept space covers the whole run.
	l := simple(t, "  a   b", ParagraphStyle{}, inter16, inf)
	if a, sp := l.Cluster(0), l.Cluster(1); a.TextStart != 0 || a.TextEnd != 3 || sp.TextStart != 3 || sp.TextEnd != 6 {
		t.Errorf("collapsed mapping: %+v %+v", a, sp)
	}
	if l.ClusterAt(4) != 1 || l.ClusterAt(6) != 2 || l.ClusterAt(7) != 3 {
		t.Errorf("ClusterAt: %d %d %d", l.ClusterAt(4), l.ClusterAt(6), l.ClusterAt(7))
	}
	// Newlines between CJK characters are removed, others become spaces.
	if got := lineTexts(simple(t, "日本\n語 a\nb", ParagraphStyle{}, inter16, inf)); len(got) != 1 {
		t.Errorf("segment breaks: %q", got)
	}
	l = simple(t, "日本\n語 a\nb", ParagraphStyle{}, inter16, inf)
	procd := append([]rune(nil), l.text.runes...)
	if string(procd) != "日本語 a b" {
		t.Errorf("segment break transformation: %q", string(procd))
	}
	// CR LF is one forced break.
	if got := lineTexts(simple(t, "a\r\nb", ParagraphStyle{WhiteSpace: WhiteSpacePre}, inter16, inf)); !slices.Equal(got, []string{"a\r\n", "b"}) {
		t.Errorf("CR LF: %q", got)
	}
}

// TestTrailingSpaces checks that trailing spaces hang in pre-wrap and are
// removed in normal, and do not count for alignment.
func TestTrailingSpaces(t *testing.T) {
	w := float32(300)
	l := simple(t, "Hello   ", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap, Align: AlignRight}, inter16, w)
	ln := l.Line(0)
	hello := simple(t, "Hello", ParagraphStyle{}, inter16, w).Width()
	// Conditionally hanging: before the end of the paragraph, they fit, so
	// they count.
	if near(ln.Width, hello) || ln.Hanging != 0 {
		t.Errorf("pre-wrap spaces at the end that fit: width %g hang %g (text %g)", ln.Width, ln.Hanging, hello)
	}
	// At a soft wrap they hang and the right alignment ignores them.
	l = simple(t, "Hello    world", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap, Align: AlignRight}, inter16, 60)
	ln = l.Line(0)
	if !near(ln.Width, hello) || ln.Hanging <= 0 || !near(ln.X+ln.Width, 60) {
		t.Errorf("hanging spaces: %+v (Hello is %g)", ln, hello)
	}
	// In normal white space the trailing space is removed: zero advance.
	l = simple(t, "Hello world", ParagraphStyle{Align: AlignRight}, inter16, 60)
	ln = l.Line(0)
	sp := l.Cluster(5)
	if sp.Advance != 0 || sp.Flags&ClusterHanging == 0 || !near(ln.X+ln.Width, 60) || ln.Hanging != 0 {
		t.Errorf("removed space: %+v, line %+v", sp, ln)
	}
	// In a right-to-left paragraph hanging spaces are on the left.
	l = simple(t, "مرحبا    بكم", ParagraphStyle{Direction: DirectionRTL, WhiteSpace: WhiteSpacePreWrap}, inter16, 60)
	ln = l.Line(0)
	if !near(ln.X+ln.Hanging+ln.Width, 60) || ln.Hanging <= 0 {
		t.Errorf("RTL hanging: %+v", ln)
	}
}

// TestWordBreak checks break-all, keep-all, overflow-wrap and their
// effect on the min-content width.
func TestWordBreak(t *testing.T) {
	long := "Supercalifragilistic word"
	normal := simple(t, long, ParagraphStyle{}, inter16, 60)
	if lines := lineTexts(normal); len(lines) != 2 || lines[0] != "Supercalifragilistic " {
		t.Errorf("normal: %q", lines)
	}
	bw := simple(t, long, ParagraphStyle{OverflowWrap: OverflowWrapBreakWord}, inter16, 60)
	if lines := lineTexts(bw); len(lines) < 3 {
		t.Errorf("break-word: %q", lines)
	}
	for _, ln := range lineTexts(bw)[:2] {
		if w := simple(t, strings.TrimSpace(ln), ParagraphStyle{}, inter16, 1000).Width(); w > 60.01 {
			t.Errorf("break-word line %q is %g px wide", ln, w)
		}
	}
	any := simple(t, long, ParagraphStyle{OverflowWrap: OverflowWrapAnywhere}, inter16, 60)
	if !slices.Equal(lineTexts(any), lineTexts(bw)) {
		t.Errorf("anywhere %q != break-word %q", lineTexts(any), lineTexts(bw))
	}
	// break-word does not change min-content; anywhere does.
	if !near(bw.MinContentWidth(), normal.MinContentWidth()) || any.MinContentWidth() >= 20 {
		t.Errorf("min-content: normal %g, break-word %g, anywhere %g", normal.MinContentWidth(), bw.MinContentWidth(), any.MinContentWidth())
	}
	// break-all also breaks "word", which fits after "stic ".
	ba := simple(t, long, ParagraphStyle{WordBreak: WordBreakBreakAll}, inter16, 60)
	eq(t, "break-all", lineTexts(ba), []string{"Superc", "alifragili", "stic wor", "d"})
	if ba.MinContentWidth() >= 20 {
		t.Errorf("break-all min-content %g", ba.MinContentWidth())
	}
	// keep-all: no breaks between CJK characters, only at spaces.
	cjk := "日本語の文章 東京"
	ka := simple(t, cjk, ParagraphStyle{WordBreak: WordBreakKeepAll}, inter16, 40)
	eq(t, "keep-all", lineTexts(ka), []string{"日本語の文章 ", "東京"})
	if n := simple(t, cjk, ParagraphStyle{}, inter16, 40); n.NumLines() < 4 {
		t.Errorf("CJK normal: %q", lineTexts(n))
	}
	// Kinsoku (UAX #14): no closing punctuation at the start of a line.
	k := simple(t, "日本語。日本語", ParagraphStyle{}, inter16, 48)
	for _, ln := range lineTexts(k) {
		if strings.HasPrefix(ln, "。") {
			t.Errorf("line starts with a full stop: %q", lineTexts(k))
		}
	}
}

// TestJustify checks that justified lines fill the width, except the last
// one, by stretching the spaces only.
func TestJustify(t *testing.T) {
	const w = 200
	l := simple(t, "Justified text stretches its word separators so that every line but the last fills the box.", ParagraphStyle{Align: AlignJustify}, inter16, w)
	for i := range l.NumLines() {
		ln := l.Line(i)
		last := i == l.NumLines()-1
		if !last && !near(ln.X+ln.Width+lineExtra(l, i), w) {
			t.Errorf("line %d: x %g width %g extra %g, want to fill %d", i, ln.X, ln.Width, lineExtra(l, i), w)
		}
		if last && (ln.X != 0 || lineExtra(l, i) != 0) {
			t.Errorf("last line is not start-aligned: %+v", ln)
		}
	}
	// Letters keep their advance; spaces grow equally.
	plain := simple(t, "Justified text", ParagraphStyle{}, inter16, 1000)
	if l.Cluster(0).Advance != plain.Cluster(0).Advance {
		t.Error("justification changed a letter")
	}
}

// lineExtra returns the justification added to line i.
func lineExtra(l *Layout, i int) float32 {
	ln := &l.lines[i]
	var extra float32
	for c := ln.c0; c < ln.c1; c++ {
		if l.cline[c] == int32(i) {
			extra += l.cadv[c] - l.clusterAdvance(ln, c)
		}
	}
	return extra
}

// TestEllipsis checks text-overflow: ellipsis in both directions: the
// ellipsis sits at the end edge, whole clusters are hidden, the line fits.
func TestEllipsis(t *testing.T) {
	ps := ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}
	l := simple(t, "Hello wonderful world", ps, inter16, 100)
	ln := l.Line(0)
	vis := visual(l)[0]
	if !ln.Ellipsis || !strings.HasSuffix(vis, "…") || !strings.HasPrefix(vis, "Hello w") || ln.Width > 100.01 {
		t.Errorf("LTR: %q %+v", vis, ln)
	}
	if l.Width() > 100.01 {
		t.Errorf("width %g", l.Width())
	}
	// Fits: no ellipsis.
	if l := simple(t, "Hello", ps, inter16, 100); l.Line(0).Ellipsis {
		t.Error("ellipsis on a line that fits")
	}
	// RTL: the ellipsis on the left, the logical end of the text hidden.
	ps.Direction = DirectionRTL
	l = simple(t, "مرحبا بكم في هذا السطر الطويل", ps, inter16, 100)
	vis = visual(l)[0]
	runs := l.LineRuns(0, nil)
	if runs[0].Kind != RunEllipsis || !strings.HasSuffix(vis, "ابحرم") || l.Line(0).Width > 100.01 {
		t.Errorf("RTL: %q, runs %+v", vis, runs)
	}
	if !near(l.Line(0).X+l.Line(0).Width, 100) {
		t.Errorf("RTL truncated line not right-aligned: %+v", l.Line(0))
	}
	// Mixed bidi in an LTR paragraph: the visual right end is hidden,
	// which is the logical start of the Arabic word.
	ps.Direction = DirectionLTR
	l = simple(t, "abc مرحبا", ps, inter16, 50)
	if vis := visual(l)[0]; !strings.HasPrefix(vis, "abc ") || !strings.HasSuffix(vis, "…") || strings.Contains(vis, "م") {
		t.Errorf("bidi truncation: %q", vis)
	}
	// Clusters are never split: an emoji ZWJ sequence is hidden whole.
	l = simple(t, "ab👨‍👩‍👧‍👦👨‍👩‍👧‍👦", ps, inter16, 40)
	for ci := range l.NumClusters() {
		if cl := l.Cluster(ci); cl.Line >= 0 && strings.Contains(l.Text()[cl.TextStart:cl.TextEnd], "\u200d") && cl.Graphemes != 1 {
			t.Errorf("cluster %+v", cl)
		}
	}
	if vis := visual(l)[0]; strings.Count(vis, "👨") != strings.Count(vis, "👦") {
		t.Errorf("split emoji: %q", vis)
	}
	// The first cluster is never hidden.
	l = simple(t, "Wide", ps, inter16, 4)
	if vis := visual(l)[0]; vis != "W…" {
		t.Errorf("tiny width: %q", vis)
	}
}

// TestLineClamp checks line-clamp: the first N lines stay, the last one
// ends with an ellipsis, the height covers N lines.
func TestLineClamp(t *testing.T) {
	text := "one two three four five six seven eight nine ten eleven twelve"
	l := simple(t, text, ParagraphStyle{LineClamp: 2}, inter16, 90)
	if l.NumLines() != 2 || !l.Line(1).Ellipsis || l.Line(0).Ellipsis || l.Height() != 48 {
		t.Fatalf("clamp 2:\n%s", dump(l))
	}
	if vis := visual(l)[1]; !strings.HasSuffix(vis, "…") {
		t.Errorf("last line %q", vis)
	}
	hidden := 0
	for ci := range l.NumClusters() {
		if l.Cluster(ci).Line < 0 {
			hidden++
		}
	}
	if hidden == 0 {
		t.Error("nothing hidden")
	}
	if off := l.ClampOffset(); off != l.Line(1).TextEnd || off >= len(text) {
		t.Errorf("ClampOffset %d, second line ends at %d", off, l.Line(1).TextEnd)
	}
	// Fewer lines than the clamp: nothing happens.
	if l := simple(t, "one two", ParagraphStyle{LineClamp: 2}, inter16, 90); l.Line(l.NumLines() - 1).Ellipsis {
		t.Error("ellipsis without clamped lines")
	}
	// Unconstrained width with a forced break: the ellipsis follows the
	// first line.
	l = simple(t, "first\nsecond", ParagraphStyle{LineClamp: 1, WhiteSpace: WhiteSpacePreLine}, inter16, float32(math.Inf(1)))
	if vis := visual(l); len(vis) != 1 || !strings.HasPrefix(vis[0], "first") || !strings.HasSuffix(vis[0], "…") {
		t.Errorf("clamp 1 with newline: %q", vis)
	}
}

// TestInlineBoxes checks box positions, line breaks around boxes and the
// line box growth from tall boxes.
func TestInlineBoxes(t *testing.T) {
	p := &Paragraph{Default: inter16}
	p.AddText("icon ", inter16)
	p.AddBox(InlineBox{ID: 7, Width: 20, Height: 20})
	p.AddText(" text", inter16)
	l := NewLayout(layoutCollection(t), p)
	l.BreakLines(1000)
	r, ok := l.BoxRect(0)
	if !ok || !near(r.Width(), 20) || !near(r.Y1, l.Line(0).Baseline) {
		t.Errorf("baseline box rect %v, baseline %g", r, l.Line(0).Baseline)
	}
	icon := simple(t, "icon x", ParagraphStyle{}, inter16, 1000)
	if !near(r.X0, icon.Cluster(5).X) {
		t.Errorf("box x %g, want %g", r.X0, icon.Cluster(5).X)
	}
	// A box taller than the line grows the line box above the baseline.
	p.Boxes[0].Height = 40
	l = NewLayout(layoutCollection(t), p)
	if ln := l.Line(0); ln.Height <= 24 || !near(ln.Ascent, 40) {
		t.Errorf("tall box: %+v", ln)
	}
	// Middle alignment centers the box on the x-height.
	p.Boxes[0].Height, p.Boxes[0].Align = 10, BoxMiddle
	l = NewLayout(layoutCollection(t), p)
	r, _ = l.BoxRect(0)
	mid := l.Line(0).Baseline - l.sinfo[0].xHeight/2
	if !near((r.Y0+r.Y1)/2, mid) {
		t.Errorf("middle: %v, mid %g", r, mid)
	}
	// Boxes are break opportunities on both sides, even next to letters.
	q := &Paragraph{Default: inter16}
	q.AddText("abcdef", inter16)
	q.AddBox(InlineBox{Width: 30, Height: 10})
	q.AddText("ghijkl", inter16)
	l = NewLayout(layoutCollection(t), q)
	l.BreakLines(60)
	eq(t, "box breaks", visual(l), []string{"abcdef", "[box]", "ghijkl"})
	// A box is a bidi neutral: between Arabic words it is right to left.
	a := &Paragraph{Default: inter16}
	a.AddText("مرحبا ", inter16)
	a.AddBox(InlineBox{Width: 10, Height: 10})
	a.AddText(" بكم", inter16)
	l = NewLayout(layoutCollection(t), a)
	eq(t, "RTL box", visual(l), []string{"مكب [box] ابحرم"})
	// Hidden by truncation: no rect.
	n := &Paragraph{Style: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, Default: inter16}
	n.AddText("some long text", inter16)
	n.AddBox(InlineBox{Width: 10, Height: 10})
	l = NewLayout(layoutCollection(t), n)
	l.BreakLines(50)
	if _, ok := l.BoxRect(0); ok {
		t.Error("truncated box has a rect")
	}
}

// TestIntrinsicWidths checks min- and max-content widths.
func TestIntrinsicWidths(t *testing.T) {
	l := simple(t, "aa bbbb c", ParagraphStyle{}, inter16, 0)
	bbbb := simple(t, "bbbb", ParagraphStyle{}, inter16, 1000).Width()
	all := simple(t, "aa bbbb c", ParagraphStyle{}, inter16, 1000).Width()
	if !near(l.MinContentWidth(), bbbb) || !near(l.MaxContentWidth(), all) {
		t.Errorf("min %g (want %g), max %g (want %g)", l.MinContentWidth(), bbbb, l.MaxContentWidth(), all)
	}
	// Forced breaks bound max-content; nowrap makes min = max.
	l = simple(t, "short\nmuch longer line", ParagraphStyle{WhiteSpace: WhiteSpacePreLine}, inter16, 0)
	if want := simple(t, "much longer line", ParagraphStyle{}, inter16, 1000).Width(); !near(l.MaxContentWidth(), want) {
		t.Errorf("max-content with a newline: %g, want %g", l.MaxContentWidth(), want)
	}
	l = simple(t, "no wrap here", ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}, inter16, 0)
	if l.MinContentWidth() != l.MaxContentWidth() {
		t.Errorf("nowrap: min %g max %g", l.MinContentWidth(), l.MaxContentWidth())
	}
	// Breaking at the max-content width keeps one line; at the
	// min-content width every line fits.
	l = simple(t, "The quick brown fox jumps over the lazy dog", ParagraphStyle{}, inter16, 0)
	l.BreakLines(l.MaxContentWidth())
	if l.NumLines() != 1 {
		t.Errorf("at max-content: %d lines", l.NumLines())
	}
	l.BreakLines(l.MinContentWidth())
	for i := range l.NumLines() {
		if l.Line(i).Width > l.MinContentWidth()+0.01 {
			t.Errorf("line %d wider than min-content", i)
		}
	}
	// Empty text: zero widths, one line of the strut's height.
	l = simple(t, "", ParagraphStyle{}, inter16, 100)
	if l.MinContentWidth() != 0 || l.MaxContentWidth() != 0 || l.NumLines() != 1 || l.Height() != 24 {
		t.Errorf("empty: %s", dump(l))
	}
}

// TestRebreakWithoutShaping checks that BreakLines never shapes and gives
// the same result as a fresh layout.
func TestRebreakWithoutShaping(t *testing.T) {
	text := strings.Repeat("Tela breaks lines without shaping again: مرحبا بكم، 日本語の文章 😀. ", 6)
	l := simple(t, text, ParagraphStyle{Align: AlignJustify}, inter16, float32(math.Inf(1)))
	before := shapeCalls.Load()
	for _, w := range []float32{80, 150, 333.3, 1e6, 47, 0} {
		l.BreakLines(w)
		fresh := simple(t, text, ParagraphStyle{Align: AlignJustify}, inter16, w)
		if !slices.Equal(lineTexts(l), lineTexts(fresh)) || l.Height() != fresh.Height() || l.Width() != fresh.Width() {
			t.Errorf("width %g: rebreak differs from a fresh layout", w)
		}
		for ci := range l.NumClusters() {
			if l.Cluster(ci) != fresh.Cluster(ci) {
				t.Errorf("width %g: cluster %d %+v != %+v", w, ci, l.Cluster(ci), fresh.Cluster(ci))
				break
			}
		}
	}
	// The fresh layouts shaped; l did not: count only l's calls.
	l2 := simple(t, text, ParagraphStyle{}, inter16, 100)
	start := shapeCalls.Load()
	for _, w := range []float32{80, 150, 333.3} {
		l2.BreakLines(w)
		l2.AlignTo(w + 10)
		_ = l2.MinContentWidth()
	}
	if got := shapeCalls.Load() - start; got != 0 {
		t.Errorf("BreakLines shaped %d times", got)
	}
	_ = before
}

// TestTextTransform checks the transforms and the mapping of their output
// back to source bytes.
func TestTextTransform(t *testing.T) {
	up := inter16
	up.Transform = TransformUppercase
	l := simple(t, "straße", ParagraphStyle{}, up, 1000)
	if string(l.text.runes) != "STRASSE" {
		t.Errorf("uppercase: %q", string(l.text.runes))
	}
	// "SS" comes from "ß" (2 bytes at offset 4): one cluster mapping to it.
	last := l.Cluster(l.NumClusters() - 2)
	if l.Text()[last.TextStart:last.TextEnd] != "ß" || last.Graphemes != 1 {
		t.Errorf("ß cluster %+v (%q)", last, l.Text()[last.TextStart:last.TextEnd])
	}
	for ci := range l.NumClusters() {
		if cl := l.Cluster(ci); cl.TextEnd <= cl.TextStart {
			t.Errorf("empty source range %+v", cl)
		}
	}
	low := inter16
	low.Transform = TransformLowercase
	if l := simple(t, "ΟΔΟΣ İ", ParagraphStyle{}, low, 1000); string(l.text.runes) != "οδος i\u0307" {
		t.Errorf("lowercase: %q", string(l.text.runes))
	}
	low.Language = "tr"
	if l := simple(t, "Iİ", ParagraphStyle{}, low, 1000); string(l.text.runes) != "ıi" {
		t.Errorf("Turkish lowercase: %q", string(l.text.runes))
	}
	cap := inter16
	cap.Transform = TransformCapitalize
	if l := simple(t, "hello wörld, don't stop ﬁsh", ParagraphStyle{}, cap, 1000); string(l.text.runes) != "Hello Wörld, Don't Stop Fish" {
		t.Errorf("capitalize: %q", string(l.text.runes))
	}
}

// TestTabs checks tab stops at multiples of 8 spaces of the block's font.
func TestTabs(t *testing.T) {
	l := simple(t, "a\tb\t\tc", ParagraphStyle{WhiteSpace: WhiteSpacePre}, inter16, 1000)
	space := spaceAdvance(l.sinfo[0].face, 16)
	stop := 8 * space
	for _, c := range []struct {
		i    int
		want float32
	}{{2, stop}, {4, 2 * stop}, {5, 3 * stop}} {
		if x := l.Cluster(c.i).X; !near(x, c.want) {
			t.Errorf("cluster %d at %g, want %g", c.i, x, c.want)
		}
	}
	// tab-size 2 and a stop closer than half a space is skipped.
	l = simple(t, "\tx", ParagraphStyle{WhiteSpace: WhiteSpacePre, TabSize: 2}, inter16, 1000)
	if !near(l.Cluster(1).X, 2*space) {
		t.Errorf("tab-size 2: %g", l.Cluster(1).X)
	}
}

// TestLineHeights checks the line box: strut, span boxes, normal line
// height with fallback fonts, and stacking.
func TestLineHeights(t *testing.T) {
	// Inter 16px: ascent 15.5 and descent 3.875 round to 16 and 4.
	l := simple(t, "a\nb", ParagraphStyle{WhiteSpace: WhiteSpacePre}, inter16, 100)
	if ln := l.Line(0); ln.Height != 24 || ln.Baseline != 18 || l.Line(1).Top != 24 || l.Height() != 48 {
		t.Errorf("px line height: %s", dump(l))
	}
	mult := inter16
	mult.LineHeight = LineHeightMultiple(2)
	if l := simple(t, "a", ParagraphStyle{}, mult, 100); l.Height() != 32 || l.Line(0).Baseline != 22 {
		t.Errorf("line-height 2: %s", dump(l))
	}
	normal := inter16
	normal.LineHeight = LineHeight{}
	if l := simple(t, "a", ParagraphStyle{}, normal, 100); l.Height() != 20 {
		t.Errorf("normal: %s", dump(l))
	}
	// With line-height normal, Arabic glyphs from Noto Sans Arabic make the
	// line taller; with a fixed line height they do not.
	if l := simple(t, "a مرحبا", ParagraphStyle{}, normal, 1000); l.Height() <= 20 {
		t.Errorf("normal with fallback: %g", l.Height())
	}
	if l := simple(t, "a مرحبا", ParagraphStyle{}, inter16, 1000); l.Height() != 24 {
		t.Errorf("fixed with fallback: %g", l.Height())
	}
	// A larger span grows the line; the strut stays.
	p := &Paragraph{Default: inter16}
	p.AddText("small ", inter16)
	big := inter16
	big.Size, big.LineHeight = 32, LineHeightPixels(40)
	p.AddText("BIG", big)
	l = NewLayout(layoutCollection(t), p)
	if ln := l.Line(0); ln.Height < 40 {
		t.Errorf("big span: %+v", ln)
	}
	// A trailing newline makes a last, empty line.
	if l := simple(t, "a\n", ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, inter16, 100); l.NumLines() != 2 || l.Height() != 48 {
		t.Errorf("trailing newline: %s", dump(l))
	}
}

// TestPaint checks the commands: glyph runs split by paint, decorations
// below and above the text.
func TestPaint(t *testing.T) {
	p := &Paragraph{Default: inter16}
	red := inter16
	red.Paint = gfx.RGB8(255, 0, 0)
	red.Decoration = TextDecoration{Lines: DecorationUnderline | DecorationLineThrough}
	p.AddText("black ", inter16)
	p.AddText("red", red)
	p.AddText(" مرحبا 😀", inter16)
	l := NewLayout(layoutCollection(t), p)
	b := gfx.NewBuilder()
	l.Paint(b, gfx.Pt(10, 20))
	dl := b.Finish()
	var names []string
	for _, c := range dl.All() {
		names = append(names, c.Name())
	}
	// Underline, then the runs (Inter black, Inter red, Arabic, emoji; the
	// Builder drops the runs of the spaces between them, which paint
	// nothing), then the line-through.
	want := []string{"FillRect", "DrawGlyphRun", "DrawGlyphRun", "DrawGlyphRun", "DrawGlyphRun", "FillRect"}
	eq(t, "commands", names, want)
	// Glyph positions match cluster positions.
	for _, c := range dl.All() {
		if run, ok := c.(gfx.DrawGlyphRun); ok && run.Paint == gfx.Paint(gfx.RGB8(255, 0, 0)) {
			r := l.Cluster(6)
			if !near(run.Origin.X+run.Glyphs[0].X, 10+r.X) || !near(run.Origin.Y, 20+l.Line(0).Baseline) {
				t.Errorf("red run at %v + %v, cluster at %g", run.Origin, run.Glyphs[0], r.X)
			}
		}
	}
}

// TestEdgeInputs checks degenerate widths and inputs.
func TestEdgeInputs(t *testing.T) {
	l := simple(t, "abc def", ParagraphStyle{}, inter16, float32(math.NaN()))
	if l.NumLines() != 1 || !math.IsInf(float64(l.MaxWidth()), 1) {
		t.Errorf("NaN width: %s", dump(l))
	}
	l.BreakLines(-5)
	if l.MaxWidth() != 0 || l.NumLines() != 2 {
		t.Errorf("negative width: %s", dump(l))
	}
	// Invalid UTF-8 is laid out as U+FFFD, one per byte, and maps back.
	l = simple(t, "a\xffb", ParagraphStyle{}, inter16, 100)
	if l.NumClusters() != 3 || l.Cluster(1).TextStart != 1 || l.Cluster(1).TextEnd != 2 {
		t.Errorf("invalid UTF-8: %d clusters", l.NumClusters())
	}
	// Control characters take no space and draw nothing.
	ab := simple(t, "ab", ParagraphStyle{}, inter16, 100)
	l = simple(t, "a\x01\u200bb", ParagraphStyle{}, inter16, 100)
	if !near(l.Width(), ab.Width()) || l.Cluster(1).GlyphEnd != l.Cluster(1).GlyphStart {
		t.Errorf("controls: width %g, want %g", l.Width(), ab.Width())
	}
	// A collection without fonts is a programmer error.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("no panic for an empty collection")
			}
		}()
		NewLayout(NewCollection(), &Paragraph{Text: "a"})
	}()
	// Spans must be valid.
	for _, p := range []*Paragraph{
		{Text: "ab", Spans: []Span{{0, 3, TextStyle{}}}},
		{Text: "ab", Spans: []Span{{1, 2, TextStyle{}}, {0, 1, TextStyle{}}}},
		{Text: "é", Spans: []Span{{1, 2, TextStyle{}}}},
		{Text: "ab", Boxes: []InlineBox{{At: 5}}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("no panic for %+v", p)
				}
			}()
			NewLayout(layoutCollection(t), p)
		}()
	}
}

// TestScriptResolution checks Common characters and paired brackets.
func TestScriptResolution(t *testing.T) {
	for _, c := range []struct {
		text string
		want []language.Script
	}{
		{"(abc)", []language.Script{language.Latin, language.Latin, language.Latin, language.Latin, language.Latin}},
		{"а (b) в", []language.Script{language.Cyrillic, language.Cyrillic, language.Cyrillic, language.Latin, language.Cyrillic, language.Cyrillic, language.Cyrillic}},
		{"שלום (abc) x", []language.Script{language.Hebrew, language.Hebrew, language.Hebrew, language.Hebrew, language.Hebrew, language.Hebrew, language.Latin, language.Latin, language.Latin, language.Hebrew, language.Hebrew, language.Latin}},
		{"123", []language.Script{language.Common, language.Common, language.Common}},
		{"1a", []language.Script{language.Latin, language.Latin}},
	} {
		rs := []rune(c.text)
		got := make([]language.Script, len(rs))
		resolveScripts(rs, got)
		if !slices.Equal(got, c.want) {
			t.Errorf("%q: %v, want %v", c.text, got, c.want)
		}
	}
	_ = harfbuzz.LeftToRight
}
