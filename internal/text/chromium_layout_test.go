package text

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// The Chromium comparison lays out the paragraphs of chromiumCases here and
// in Chromium (same font files through @font-face, one div per case) and
// compares, character by character, which line each character is on, the
// left and right edges of each cluster, the baselines and the paragraph
// height. Chromium's results (Range.getClientRects of every character) are
// committed in testdata/chromium/layout.json, so the test runs without
// Chromium. To regenerate them:
//
//	TELA_CHROMIUM=1 go test -run TestChromiumLayout ./internal/text/
//
// which writes testdata/chromium/layout.html and runs
// chromium --headless --dump-dom on it.

const chromiumFonts = `@font-face { font-family: "Inter"; src: url("../../../assets/fonts/inter/InterVariable.ttf"); font-weight: 100 900; font-style: normal; }
@font-face { font-family: "Inter"; src: url("../../../assets/fonts/inter/InterVariable-Italic.ttf"); font-weight: 100 900; font-style: italic; }
@font-face { font-family: "JetBrains Mono"; src: url("../../../assets/fonts/jetbrainsmono/JetBrainsMono-wght.ttf"); font-weight: 100 800; font-style: normal; }
@font-face { font-family: "Noto Sans Arabic"; src: url("../../../assets/fonts/notosansarabic/NotoSansArabic-wdth-wght.ttf"); font-weight: 100 900; font-stretch: 62.5% 100%; }
@font-face { font-family: "Noto Color Emoji"; src: url("../../../assets/fonts/notocoloremoji/Noto-COLRv1.ttf"); }
@font-face { font-family: "Noto Sans JP"; src: url("../fonts/NotoSansJP-Subset.otf"); }`

// allFamilies is the family list of the comparison: Chromium falls back
// through it per character, like Collection.Itemize does.
var allFamilies = []string{"Inter", "Noto Sans Arabic", "Noto Sans JP", "Noto Color Emoji"}

type chromiumSpan struct {
	text  string
	style TextStyle
	box   *InlineBox // an inline-block placeholder instead of text
}

type chromiumCase struct {
	name  string
	width float32
	para  ParagraphStyle
	base  TextStyle
	spans []chromiumSpan
}

func (c *chromiumCase) paragraph() *Paragraph {
	p := &Paragraph{Style: c.para, Default: c.base}
	for _, s := range c.spans {
		if s.box != nil {
			p.AddBox(*s.box)
			continue
		}
		p.AddText(s.text, s.style)
	}
	return p
}

func ts(size float32, lh LineHeight, mods ...func(*TextStyle)) TextStyle {
	s := TextStyle{Families: allFamilies, Size: size, LineHeight: lh}
	for _, m := range mods {
		m(&s)
	}
	return s
}

func one(text string, st TextStyle) []chromiumSpan { return []chromiumSpan{{text: text, style: st}} }

var (
	lh24 = LineHeightPixels(24)
	lh28 = LineHeightPixels(28)
)

const (
	latinText  = "Typography is the art and technique of arranging type to make written language legible, readable and appealing when displayed. Office efficiency: 42% faster, (really) affine-flows."
	arabicText = "العربية هي لغة سامية يتحدث بها أكثر من أربعمائة مليون شخص، وهي لغة رسمية في اثنتين وعشرين دولة."
	cjkText    = "日本語の文章は、漢字、ひらがな、カタカナを組み合わせて書かれます。句読点や閉じ括弧が行頭に来ないようにするのが、基本的な決まりです。"
	mixedCJK   = "混合排版时，英文单词和数字也会出现在中文句子里，例如 Tela 是一个用 Go 语言写成的界面库，版本号是 2.0。"
)

var chromiumCases = []chromiumCase{
	{name: "latin", width: 300, base: ts(16, lh24), spans: one(latinText, ts(16, lh24))},
	{name: "latin-narrow", width: 97, base: ts(16, lh24), spans: one(latinText, ts(16, lh24))},
	{name: "latin-justify", width: 280, para: ParagraphStyle{Align: AlignJustify}, base: ts(16, lh24), spans: one(latinText, ts(16, lh24))},
	{name: "latin-center", width: 260, para: ParagraphStyle{Align: AlignCenter}, base: ts(14, LineHeightPixels(20)), spans: one(latinText, ts(14, LineHeightPixels(20)))},
	{name: "latin-right", width: 260, para: ParagraphStyle{Align: AlignRight}, base: ts(18, lh28), spans: one(latinText, ts(18, lh28, func(s *TextStyle) { s.Weight = 600 }))},
	{name: "latin-normal-lh", width: 300, base: ts(16, LineHeight{}), spans: one(latinText, ts(16, LineHeight{}))},
	{name: "arabic", width: 260, para: ParagraphStyle{Direction: DirectionRTL}, base: ts(18, lh28), spans: one(arabicText, ts(18, lh28))},
	{name: "arabic-justify", width: 240, para: ParagraphStyle{Direction: DirectionRTL, Align: AlignJustify}, base: ts(18, lh28), spans: one(arabicText, ts(18, lh28))},
	{name: "bidi-ltr", width: 600, base: ts(16, lh24), spans: one("The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done.", ts(16, lh24))},
	{name: "bidi-ltr-wrap", width: 170, base: ts(16, lh24), spans: one("The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done.", ts(16, lh24))},
	{name: "bidi-rtl", width: 600, para: ParagraphStyle{Direction: DirectionRTL}, base: ts(16, lh24), spans: one("مرحبا بكم في Tela version 2.0 مع 15 ميزة (جديدة) اليوم.", ts(16, lh24))},
	{name: "bidi-rtl-wrap", width: 150, para: ParagraphStyle{Direction: DirectionRTL}, base: ts(16, lh24), spans: one("مرحبا بكم في Tela version 2.0 مع 15 ميزة (جديدة) اليوم.", ts(16, lh24))},
	{name: "cjk", width: 300, base: ts(16, lh24), spans: one(cjkText, ts(16, lh24))},
	{name: "cjk-mixed", width: 280, base: ts(16, lh24), spans: one(mixedCJK, ts(16, lh24))},
	{name: "cjk-keep-all", width: 300, para: ParagraphStyle{WordBreak: WordBreakKeepAll}, base: ts(16, lh24), spans: one(mixedCJK, ts(16, lh24))},
	{name: "cjk-justify", width: 290, para: ParagraphStyle{Align: AlignJustify}, base: ts(16, lh24), spans: one(cjkText, ts(16, lh24))},
	{name: "emoji", width: 230, base: ts(16, lh24), spans: one("Hello 👋 world 🌍🎉 family 👨‍👩‍👧‍👦 flags 🇯🇵🇧🇷 thumbs 👍🏽 done ✅ ok", ts(16, lh24))},
	{name: "letter-spacing", width: 600, base: ts(16, lh24), spans: one("a->b office => ok", ts(16, lh24, func(s *TextStyle) { s.LetterSpacing = 2 }))},
	{name: "letter-spacing-wrap", width: 150, base: ts(16, lh24), spans: one("Spaced out letters wrap like this", ts(16, lh24, func(s *TextStyle) { s.LetterSpacing = 1.5 }))},
	{name: "word-spacing", width: 300, base: ts(16, lh24), spans: one("Words with extra word spacing wrap differently here", ts(16, lh24, func(s *TextStyle) { s.WordSpacing = 6 }))},
	{name: "pre-wrap", width: 220, para: ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, base: ts(16, lh24), spans: one("Keep   these spaces\n\tand a tab\tinside\nthen a long line that has to wrap somewhere", ts(16, lh24))},
	{name: "pre", width: 200, para: ParagraphStyle{WhiteSpace: WhiteSpacePre}, base: ts(14, LineHeightPixels(20)), spans: one("func main() {\n\tx := 1 // a comment that overflows\n}", ts(14, LineHeightPixels(20), func(s *TextStyle) { s.Families = []string{"JetBrains Mono"} }))},
	{name: "pre-line", width: 220, para: ParagraphStyle{WhiteSpace: WhiteSpacePreLine}, base: ts(16, lh24), spans: one("  Collapse   spaces\n   but keep\nnewlines and wrap the long ones nicely", ts(16, lh24))},
	{name: "normal-collapse", width: 400, base: ts(16, lh24), spans: one("  Spaces   and\ttabs\n and   newlines   collapse ", ts(16, lh24))},
	{name: "nowrap", width: 120, para: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}, base: ts(16, lh24), spans: one("This line never wraps at all", ts(16, lh24))},
	{name: "break-all", width: 120, para: ParagraphStyle{WordBreak: WordBreakBreakAll}, base: ts(16, lh24), spans: one("Visit https://example.com/a/very/long/path?query=1 now", ts(16, lh24))},
	{name: "break-word", width: 120, para: ParagraphStyle{OverflowWrap: OverflowWrapBreakWord}, base: ts(16, lh24), spans: one("Visit https://example.com/a/very/long/path?query=1 now", ts(16, lh24))},
	{name: "anywhere", width: 120, para: ParagraphStyle{OverflowWrap: OverflowWrapAnywhere}, base: ts(16, lh24), spans: one("Supercalifragilisticexpialidocious words", ts(16, lh24))},
	{name: "spans", width: 260, base: ts(16, lh24), spans: []chromiumSpan{
		{text: "Small text, ", style: ts(12, lh24)},
		{text: "bold words", style: ts(16, lh24, func(s *TextStyle) { s.Weight = 700 })},
		{text: " and ", style: ts(16, lh24)},
		{text: "Large", style: ts(28, LineHeightPixels(32))},
		{text: " then italic ", style: ts(16, lh24, func(s *TextStyle) { s.Style = StyleItalic })},
		{text: "mono code()", style: ts(15, lh24, func(s *TextStyle) { s.Families = []string{"JetBrains Mono"} })},
		{text: " with kerning AVATAR To", style: ts(16, lh24)},
	}},
	{name: "uppercase", width: 300, base: ts(16, lh24), spans: one("straße ﬁnal ǰ title", ts(16, lh24, func(s *TextStyle) { s.Transform = TransformUppercase }))},
	{name: "capitalize", width: 300, base: ts(16, lh24), spans: one("don't stop the ﬁsh-market now", ts(16, lh24, func(s *TextStyle) { s.Transform = TransformCapitalize }))},
	{name: "inline-boxes", width: 220, base: ts(16, lh24), spans: []chromiumSpan{
		{text: "Icons ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 16, Height: 16}},
		{text: " on the baseline and ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 14, Height: 14, Align: BoxMiddle}},
		{text: " in the middle, ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 12, Height: 12, Align: BoxTextTop}},
		{text: " at the text top and ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 12, Height: 12, Align: BoxTextBottom}},
		{text: " bottom.", style: ts(16, lh24)},
	}},
	{name: "inline-box-tall", width: 260, base: ts(16, lh24), spans: []chromiumSpan{
		{text: "A tall ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 30, Height: 40}},
		{text: " box grows the line, a top-aligned ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 20, Height: 50, Align: BoxTop}},
		{text: " one too, and text wraps after them.", style: ts(16, lh24)},
	}},
	{name: "rtl-center", width: 200, para: ParagraphStyle{Direction: DirectionRTL, Align: AlignCenter}, base: ts(16, lh24), spans: one(arabicText, ts(16, lh24))},
	{name: "rtl-end", width: 200, para: ParagraphStyle{Direction: DirectionRTL, Align: AlignEnd}, base: ts(16, lh24), spans: one(arabicText, ts(16, lh24))},
	{name: "rtl-letter-spacing", width: 600, para: ParagraphStyle{Direction: DirectionRTL}, base: ts(16, lh24), spans: one("مرحبا بكم Tela 2.0", ts(16, lh24, func(s *TextStyle) { s.LetterSpacing = 3 }))},
	{name: "auto-direction", width: 300, para: ParagraphStyle{Direction: DirectionAuto}, base: ts(16, lh24), spans: one("123 مرحبا Tela", ts(16, lh24))},
	{name: "tabular", width: 300, base: ts(16, lh24), spans: one("1111 0000 9999", ts(16, lh24, func(s *TextStyle) { s.Numeric = NumericTabular }))},
}

// cssStyle returns the CSS declarations of a text style.
func cssStyle(s TextStyle) string {
	var fams []string
	for _, f := range s.Families {
		fams = append(fams, fmt.Sprintf("%q", f))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "font-family: %s; font-size: %gpx;", strings.Join(fams, ", "), s.size())
	if s.Weight != 0 {
		fmt.Fprintf(&b, " font-weight: %g;", s.Weight)
	}
	if s.Style == StyleItalic {
		b.WriteString(" font-style: italic;")
	}
	switch s.LineHeight.Kind {
	case LineHeightNormal:
		b.WriteString(" line-height: normal;")
	case LineHeightScale:
		fmt.Fprintf(&b, " line-height: %g;", s.LineHeight.Value)
	case LineHeightPx:
		fmt.Fprintf(&b, " line-height: %gpx;", s.LineHeight.Value)
	}
	if s.LetterSpacing != 0 {
		fmt.Fprintf(&b, " letter-spacing: %gpx;", s.LetterSpacing)
	}
	if s.WordSpacing != 0 {
		fmt.Fprintf(&b, " word-spacing: %gpx;", s.WordSpacing)
	}
	switch s.Transform {
	case TransformUppercase:
		b.WriteString(" text-transform: uppercase;")
	case TransformLowercase:
		b.WriteString(" text-transform: lowercase;")
	case TransformCapitalize:
		b.WriteString(" text-transform: capitalize;")
	}
	if s.Numeric&NumericTabular != 0 {
		b.WriteString(" font-variant-numeric: tabular-nums;")
	}
	return b.String()
}

func cssPara(p ParagraphStyle) string {
	var b strings.Builder
	b.WriteString([...]string{"direction: ltr;", "direction: rtl;", ""}[p.Direction]) // auto: the dir attribute
	b.WriteString([...]string{" text-align: start;", " text-align: end;", " text-align: left;", " text-align: right;", " text-align: center;", " text-align: justify;"}[p.Align])
	b.WriteString([...]string{" white-space: normal;", " white-space: nowrap;", " white-space: pre;", " white-space: pre-wrap;", " white-space: pre-line;", " white-space: break-spaces;"}[p.WhiteSpace])
	b.WriteString([...]string{"", " word-break: break-all;", " word-break: keep-all;"}[p.WordBreak])
	b.WriteString([...]string{"", " overflow-wrap: break-word;", " overflow-wrap: anywhere;"}[p.OverflowWrap])
	if p.TabSize != 0 {
		fmt.Fprintf(&b, " tab-size: %g;", p.TabSize)
	}
	return b.String()
}

func chromiumLayoutHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(chromiumFonts)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; background: #fff; }\n.case { margin: 0 0 40px 8px; }\n</style></head><body>\n")
	for i, c := range chromiumCases {
		dir := ""
		if c.para.Direction == DirectionAuto {
			dir = ` dir="auto"`
		}
		fmt.Fprintf(&b, "<div class=\"case\" id=\"c%d\"%s style=\"%s\">", i, dir, html.EscapeString(fmt.Sprintf("width: %gpx; %s %s", c.width, cssPara(c.para), cssStyle(c.base))))
		for _, s := range c.spans {
			if s.box != nil {
				align := [...]string{"baseline", "middle", "text-top", "text-bottom", "top", "bottom"}[s.box.Align]
				fmt.Fprintf(&b, "<span class=\"ibox\" style=\"display: inline-block; width: %gpx; height: %gpx; vertical-align: %s; background: #4f46e5;\"></span>", s.box.Width, s.box.Height, align)
				continue
			}
			fmt.Fprintf(&b, "<span style=\"%s\">%s</span>", html.EscapeString(cssStyle(s.style)), html.EscapeString(s.text))
		}
		b.WriteString("</div>\n")
	}
	fmt.Fprintf(&b, `<pre id="out"></pre>
<script>
const n = %d;
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const out = {};
  for (let i = 0; i < n; i++) {
    const div = document.getElementById("c" + i);
    const box = div.getBoundingClientRect();
    const chars = [];
    const walker = document.createTreeWalker(div, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      let off = 0;
      for (const ch of node.data) {
        const r = document.createRange();
        r.setStart(node, off);
        r.setEnd(node, off + ch.length);
        const rects = [...r.getClientRects()].filter(q => q.width > 0 || q.height > 0);
        chars.push(rects.length ? rects.map(q => [q.left - box.left, q.top - box.top, q.width, q.height]) : null);
        off += ch.length;
      }
    }
    const boxes = [...div.querySelectorAll(".ibox")].map(e => { const q = e.getBoundingClientRect(); return [q.left - box.left, q.top - box.top, q.width, q.height]; });
    out["c" + i] = {height: box.height, chars, boxes};
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script></body></html>
`, len(chromiumCases))
	return b.String()
}

type chromiumResult struct {
	Height float64       `json:"height"`
	Chars  [][][]float64 `json:"chars"` // per code point: rects [left, top, width, height], or null
	Boxes  [][]float64   `json:"boxes"` // inline boxes: [left, top, width, height]
}

func regenerateChromiumLayout(t *testing.T, dir string) {
	htmlPath := filepath.Join(dir, "layout.html")
	if err := os.WriteFile(htmlPath, []byte(chromiumLayoutHTML()), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(htmlPath)
	cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--force-device-scale-factor=1",
		"--allow-file-access-from-files", "--virtual-time-budget=10000", "--window-size=1200,800", "--dump-dom", "file://"+abs)
	outb, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	s := string(outb)
	i, j := strings.Index(s, "BEGIN"), strings.Index(s, "END</pre>")
	if i < 0 || j < 0 {
		t.Fatalf("chromium output has no result:\n%s", s[:min(len(s), 2000)])
	}
	js := html.UnescapeString(s[i+5 : j])
	var parsed map[string]chromiumResult
	if err := json.Unmarshal([]byte(js), &parsed); err != nil {
		t.Fatal(err)
	}
	// One case per line: compact, but diffs show which cases changed.
	var out strings.Builder
	out.WriteString("{\n")
	for i := range chromiumCases {
		k := fmt.Sprintf("c%d", i)
		v, err := json.Marshal(parsed[k])
		if err != nil {
			t.Fatal(err)
		}
		sep := ","
		if i == len(chromiumCases)-1 {
			sep = ""
		}
		fmt.Fprintf(&out, "%q: %s%s\n", k, v, sep)
	}
	out.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "layout.json"), []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// chromiumMismatch is the result of comparing one case.
type chromiumMismatch struct {
	lines    []string // line break differences
	edges    []string // cluster edge differences beyond the tolerance
	maxEdge  float64  // largest edge difference
	baseline []string
	height   string
}

// edgeTolerance is the largest difference of cluster edges accepted, in px:
// Chromium positions in LayoutUnits (1/64 px) and rounds each glyph
// advance, so long lines accumulate a few hundredths.
const edgeTolerance = 0.15

func compareChromium(t *testing.T, c *chromiumCase, res chromiumResult) chromiumMismatch {
	l := NewLayout(layoutCollection(t), c.paragraph())
	l.BreakLines(c.width)
	return compareLayout(t, c.name, l, res)
}

// compareLayout compares the lines, cluster edges, inline boxes and height
// of l with Chromium's.
func compareLayout(t *testing.T, name string, l *Layout, res chromiumResult) chromiumMismatch {
	var mm chromiumMismatch
	src := l.Text()
	// Code point index of every byte offset.
	cpOf := make([]int, len(src)+1)
	k := 0
	for i := 0; i < len(src); {
		_, n := utf8.DecodeRuneInString(src[i:])
		for j := 0; j < n; j++ {
			cpOf[i+j] = k
		}
		i += n
		k++
	}
	cpOf[len(src)] = k
	if k != len(res.Chars) {
		t.Fatalf("%s: %d code points here, %d in Chromium", name, k, len(res.Chars))
	}

	// Chromium's baseline of each visible character, from its rect top and
	// the quantized ascent of its style's first available font.
	type cinfo struct {
		left, right, base float64
		ok                bool
	}
	cps := make([]cinfo, k)
	for ci := range l.NumClusters() {
		cl := l.Cluster(ci)
		if cl.Flags&ClusterBox != 0 {
			continue
		}
		// Chromium's text rects have the height of the style's first
		// available font, whatever font draws the character.
		asc := float64(l.sinfo[l.clusters[ci].style].ascent)
		for cp := cpOf[cl.TextStart]; cp < cpOf[cl.TextEnd]; cp++ {
			rects := res.Chars[cp]
			if len(rects) == 0 {
				continue
			}
			r := rects[0]
			if r[2] <= 0 {
				continue
			}
			cps[cp] = cinfo{r[0], r[0] + r[2], r[1] + asc, true}
		}
	}
	// Chromium's lines: a new line starts where the baseline changes.
	type cline struct {
		first       int // first visible code point
		base        float64
		left, right float64
	}
	var clines []cline
	for cp, ci := range cps {
		if !ci.ok {
			continue
		}
		if len(clines) == 0 || math.Abs(ci.base-clines[len(clines)-1].base) > 4 {
			clines = append(clines, cline{cp, ci.base, ci.left, ci.right})
		}
		cl := &clines[len(clines)-1]
		cl.left, cl.right = math.Min(cl.left, ci.left), math.Max(cl.right, ci.right)
	}
	// Ours: the first visible, non-removed cluster of each line.
	type oline struct {
		first       int
		base        float64
		left, right float64
	}
	var olines []oline
	for li := range l.NumLines() {
		ln := l.Line(li)
		first := -1
		for ci := ln.ClusterStart; ci < ln.ClusterEnd && first < 0; ci++ {
			cl := l.Cluster(ci)
			if cl.Line != li || cl.Advance <= 0 {
				continue
			}
			// The first character of the cluster that Chromium shows
			// (collapsed spaces before it belong to it here).
			for cp := cpOf[cl.TextStart]; cp < cpOf[cl.TextEnd]; cp++ {
				if cps[cp].ok {
					first = cp
					break
				}
			}
		}
		if first >= 0 {
			olines = append(olines, oline{first, float64(ln.Baseline), float64(ln.X), float64(ln.X + ln.Width)})
		}
	}
	for i := 0; i < max(len(clines), len(olines)); i++ {
		switch {
		case i >= len(clines):
			mm.lines = append(mm.lines, fmt.Sprintf("extra line %d starting at %q", i, excerpt(src, cpOf, olines[i].first)))
		case i >= len(olines):
			mm.lines = append(mm.lines, fmt.Sprintf("missing line %d starting at %q", i, excerpt(src, cpOf, clines[i].first)))
		case clines[i].first != olines[i].first:
			mm.lines = append(mm.lines, fmt.Sprintf("line %d starts at %q [%.2f, %.2f], Chromium at %q [%.2f, %.2f]", i,
				excerpt(src, cpOf, olines[i].first), olines[i].left, olines[i].right,
				excerpt(src, cpOf, clines[i].first), clines[i].left, clines[i].right))
		default:
			if d := olines[i].base - clines[i].base; math.Abs(d) > 0.5 {
				mm.baseline = append(mm.baseline, fmt.Sprintf("line %d baseline %g, Chromium %g", i, olines[i].base, clines[i].base))
			}
		}
	}
	if len(mm.lines) == 0 {
		// Cluster edges: the union of Chromium's character rects.
		for ci := range l.NumClusters() {
			cl := l.Cluster(ci)
			if cl.Line < 0 || cl.Advance <= 0 || cl.Flags&(ClusterBox|ClusterHanging) != 0 {
				continue
			}
			lo, hi, ok := math.Inf(1), math.Inf(-1), false
			for cp := cpOf[cl.TextStart]; cp < cpOf[cl.TextEnd]; cp++ {
				if cps[cp].ok {
					lo, hi, ok = math.Min(lo, cps[cp].left), math.Max(hi, cps[cp].right), true
				}
			}
			if !ok {
				continue
			}
			d := math.Max(math.Abs(lo-float64(cl.X)), math.Abs(hi-float64(cl.X+cl.Advance)))
			mm.maxEdge = math.Max(mm.maxEdge, d)
			if d > edgeTolerance {
				mm.edges = append(mm.edges, fmt.Sprintf("%q: [%.3f, %.3f], Chromium [%.3f, %.3f]", src[cl.TextStart:cl.TextEnd], cl.X, cl.X+cl.Advance, lo, hi))
			}
		}
	}
	for i, want := range res.Boxes {
		r, ok := l.BoxRect(i)
		if !ok {
			mm.edges = append(mm.edges, fmt.Sprintf("box %d hidden, Chromium at %v", i, want))
			continue
		}
		d := math.Max(math.Max(math.Abs(float64(r.X0)-want[0]), math.Abs(float64(r.Y0)-want[1])),
			math.Max(math.Abs(float64(r.Width())-want[2]), math.Abs(float64(r.Height())-want[3])))
		mm.maxEdge = math.Max(mm.maxEdge, d)
		if d > edgeTolerance {
			mm.edges = append(mm.edges, fmt.Sprintf("box %d at %v, Chromium [%g %g %g %g]", i, r, want[0], want[1], want[2], want[3]))
		}
	}
	if d := float64(l.Height()) - res.Height; math.Abs(d) > 0.5 {
		mm.height = fmt.Sprintf("height %g, Chromium %g", l.Height(), res.Height)
	}
	return mm
}

func excerpt(src string, cpOf []int, cp int) string {
	for i := range src {
		if cpOf[i] == cp {
			s := src[i:]
			if r := []rune(s); len(r) > 12 {
				s = string(r[:12])
			}
			return s
		}
	}
	return ""
}

// knownDifference describes an understood difference from Chromium and
// bounds it, so that the test still catches regressions.
type knownDifference struct {
	why     string
	maxEdge float64 // largest cluster edge difference allowed
	lines   bool    // line breaks may differ
}

var knownChromiumDifferences = map[string]knownDifference{
	// Inter at weight 600 kerns a period followed by a space (-0.37 px at
	// 18 px). Chromium reshapes a line end that HarfBuzz marks unsafe to
	// break, so the period at the end of a line loses that kerning; here
	// shaping happens once per paragraph (Parley's model) and the period
	// keeps it. Right-aligned lines that end in such a pair start 0.39 px
	// further right than in Chromium.
	"latin-right": {why: "line ends are not reshaped", maxEdge: 0.45},
	// The same at an emergency break inside a word: the "r" before the
	// break keeps its kerning with the next letter.
	"break-all": {why: "line ends are not reshaped", maxEdge: 0.3},
	// UAX #14 (go-text's segmenter) allows a break after "/" and "?" in
	// "https://example.com/a/very/long/path?query=1"; Blink breaks ASCII
	// text with its own pair table, which has no opportunity there, so it
	// breaks the URL only at emergency opportunities.
	"break-word": {why: "UAX #14 breaks after / and ? in URLs, Blink's ASCII table does not", lines: true},
	// text-transform: capitalize maps the ligature "ﬁ" at the start of a
	// word to "Fi" (full case mapping, as CSS Text 3 requires); Chromium
	// leaves it unchanged (simple, one-to-one title case mapping).
	"capitalize": {why: "capitalize uses the full case mapping of U+FB01", maxEdge: 4.5},
}

func TestChromiumLayout(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateChromiumLayout(t, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "layout.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "layout.html")); err != nil || string(h) != chromiumLayoutHTML() {
		t.Fatal("testdata/chromium/layout.html is out of date: the cases changed; regenerate with TELA_CHROMIUM=1")
	}
	var results map[string]chromiumResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	for i := range chromiumCases {
		c := &chromiumCases[i]
		t.Run(c.name, func(t *testing.T) {
			res, ok := results[fmt.Sprintf("c%d", i)]
			if !ok {
				t.Fatal("no Chromium result; regenerate with TELA_CHROMIUM=1")
			}
			mm := compareChromium(t, c, res)
			t.Logf("max cluster edge difference %.3f px", mm.maxEdge)
			var problems []string
			problems = append(problems, mm.lines...)
			problems = append(problems, mm.edges...)
			problems = append(problems, mm.baseline...)
			if mm.height != "" {
				problems = append(problems, mm.height)
			}
			if len(problems) == 0 {
				if _, ok := knownChromiumDifferences[c.name]; ok {
					t.Errorf("no difference from Chromium any more: remove the case from knownChromiumDifferences")
				}
				return
			}
			msg := strings.Join(problems, "\n")
			if k, ok := knownChromiumDifferences[c.name]; ok {
				if (len(mm.lines) == 0 || k.lines) && mm.maxEdge <= k.maxEdge+1e-9 && len(mm.baseline) == 0 && (k.lines || mm.height == "") {
					t.Logf("known difference (%s):\n%s", k.why, msg)
					return
				}
				t.Errorf("differences from Chromium beyond the known ones (%s):\n%s", k.why, msg)
				return
			}
			t.Errorf("differences from Chromium:\n%s", msg)
		})
	}
}
