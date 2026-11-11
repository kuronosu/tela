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
	"unicode"
	"unicode/utf8"
)

// The hyphenation comparison lays out the paragraphs of chromiumHyphenCases here
// and in Chromium (same font files through @font-face, one div per case)
// and compares the lines: the first visible character of each line, and
// the left and right edges of its content, the hyphen included. Chromium's
// results are committed in testdata/chromium/hyphens.json, so the test
// runs without Chromium. Chromium hyphenates (hyphens: auto) only with its
// hyphen-data component, which headless Chromium reads from the profile:
// regenerating needs the directory Chromium downloaded it to:
//
//	TELA_CHROMIUM=1 TELA_CHROMIUM_HYPHEN=~/.config/chromium/hyphen-data/120.0.6050.0 \
//	  go test -run TestChromiumHyphens ./internal/text/
//
// which writes testdata/chromium/hyphens.html, copies the dictionaries to a
// temporary profile and runs chromium --headless --dump-dom on the page
// (Chromium 155 for the committed data).

type chromiumHyphenCase struct {
	name     string
	width    float32
	para     ParagraphStyle
	base     TextStyle
	spans    []chromiumSpan
	minWidth bool // width: min-content, compared with MinContentWidth
}

// hyStyle is ts with hyphens and a language.
func hyStyle(size float32, h Hyphens, lang string, mods ...func(*TextStyle)) TextStyle {
	return ts(size, lh24, append([]func(*TextStyle){func(s *TextStyle) { s.Hyphens, s.Language = h, lang }}, mods...)...)
}

const (
	shyExtra = "Ex\u00adtra\u00ador\u00addi\u00adnary in\u00adter\u00adna\u00adtion\u00adal\u00adiza\u00adtion"
	autoEn   = "Extraordinary internationalization of hyphenation"
	autoText = "The typographical considerations of hyphenation demonstrate extraordinary internationalization requirements everywhere"
	autoES   = "La hiperinflación latinoamericana desestabilizó extraordinariamente las economías de la región"
	autoFR   = "La typographie extraordinaire exige des considérations internationales particulièrement développées"
	autoGB   = "The internationalisation programme demonstrated extraordinary behaviour everywhere"
)

func one1(text string, st TextStyle) []chromiumSpan { return []chromiumSpan{{text: text, style: st}} }

var chromiumHyphenCases = []chromiumHyphenCase{
	// Soft hyphens (hyphens: manual, the initial value).
	{name: "soft", width: 100, base: hyStyle(16, HyphensManual, ""), spans: one1(shyExtra, hyStyle(16, HyphensManual, ""))},
	{name: "soft-narrow", width: 58, base: hyStyle(16, HyphensManual, ""), spans: one1(shyExtra, hyStyle(16, HyphensManual, ""))},
	{name: "soft-none", width: 100, base: hyStyle(16, HyphensNone, ""), spans: one1(shyExtra, hyStyle(16, HyphensNone, ""))},
	{name: "soft-tracking", width: 110, base: hyStyle(16, HyphensManual, "", func(s *TextStyle) { s.LetterSpacing = 3 }),
		spans: one1(shyExtra, hyStyle(16, HyphensManual, "", func(s *TextStyle) { s.LetterSpacing = 3 }))},
	{name: "soft-justify", width: 140, para: ParagraphStyle{Align: AlignJustify}, base: hyStyle(16, HyphensManual, ""),
		spans: one1("Some "+shyExtra+" text here", hyStyle(16, HyphensManual, ""))},
	{name: "soft-center", width: 120, para: ParagraphStyle{Align: AlignCenter}, base: hyStyle(16, HyphensManual, ""),
		spans: one1("Some "+shyExtra+" text here", hyStyle(16, HyphensManual, ""))},
	{name: "soft-rtl", width: 90, para: ParagraphStyle{Direction: DirectionRTL}, base: hyStyle(16, HyphensManual, ""),
		spans: one1("العربية\u00adهي لغة سا\u00adمية يتحدث\u00adبها أكثر", hyStyle(16, HyphensManual, ""))},
	{name: "soft-spans", width: 100, base: hyStyle(16, HyphensManual, ""), spans: []chromiumSpan{
		{text: "Ex\u00adtra\u00ador", style: hyStyle(16, HyphensManual, "")},
		{text: "\u00addi\u00adnary in\u00adter", style: hyStyle(16, HyphensManual, "", func(s *TextStyle) { s.Weight = 700 })},
		{text: "\u00adna\u00adtion\u00adal", style: hyStyle(20, HyphensManual, "")},
		{text: "\u00adiza\u00adtion", style: hyStyle(16, HyphensManual, "")},
	}},
	{name: "soft-span-none", width: 100, base: hyStyle(16, HyphensManual, ""), spans: []chromiumSpan{
		{text: "Ex\u00adtra\u00ador\u00addi\u00adnary ", style: hyStyle(16, HyphensManual, "")},
		{text: "in\u00adter\u00adna\u00adtion\u00adal\u00adiza\u00adtion", style: hyStyle(16, HyphensNone, "")},
	}},
	{name: "soft-balance", width: 120, para: ParagraphStyle{WrapStyle: TextWrapBalance}, base: hyStyle(16, HyphensManual, ""),
		spans: one1(shyExtra+" of things", hyStyle(16, HyphensManual, ""))},
	{name: "soft-pretty", width: 170, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: hyStyle(16, HyphensManual, ""),
		spans: one1("Typography is the art of ar\u00adrang\u00ading type to make writ\u00adten lan\u00adguage leg\u00adi\u00adble and ap\u00adpeal\u00ading to us", hyStyle(16, HyphensManual, ""))},
	{name: "soft-pretty-narrow", width: 120, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: hyStyle(16, HyphensManual, ""),
		spans: one1("Typography is the art of ar\u00adrang\u00ading type to make writ\u00adten lan\u00adguage leg\u00adi\u00adble and ap\u00adpeal\u00ading to us", hyStyle(16, HyphensManual, ""))},
	{name: "soft-clamp", width: 100, para: ParagraphStyle{LineClamp: 2}, base: hyStyle(16, HyphensManual, ""),
		spans: one1(shyExtra+" and more", hyStyle(16, HyphensManual, ""))},
	{name: "soft-overflow", width: 24, base: hyStyle(16, HyphensManual, ""), spans: one1(shyExtra, hyStyle(16, HyphensManual, ""))},
	{name: "soft-pre-wrap", width: 100, para: ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, base: hyStyle(16, HyphensManual, ""),
		spans: one1("Ex\u00adtra\u00ador\u00addi\u00adnary  in\u00adter\u00adna\u00adtion\u00adal", hyStyle(16, HyphensManual, ""))},
	{name: "soft-nowrap", width: 100, para: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap}, base: hyStyle(16, HyphensManual, ""),
		spans: one1(shyExtra, hyStyle(16, HyphensManual, ""))},
	{name: "soft-break-word", width: 50, para: ParagraphStyle{OverflowWrap: OverflowWrapBreakWord}, base: hyStyle(16, HyphensManual, ""),
		spans: one1("Antidisestablish\u00admentarianism", hyStyle(16, HyphensManual, ""))},
	{name: "soft-min-content", minWidth: true, base: hyStyle(16, HyphensManual, ""), spans: one1(shyExtra, hyStyle(16, HyphensManual, ""))},
	// hyphens: auto with a dictionary.
	{name: "auto-en", width: 100, base: hyStyle(16, HyphensAuto, "en"), spans: one1(autoEn, hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-en-text", width: 160, base: hyStyle(16, HyphensAuto, "en"), spans: one1(autoText, hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-en-narrow", width: 64, base: hyStyle(16, HyphensAuto, "en"), spans: one1(autoText, hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-en-justify", width: 180, para: ParagraphStyle{Align: AlignJustify}, base: hyStyle(16, HyphensAuto, "en-US"), spans: one1(autoText, hyStyle(16, HyphensAuto, "en-US"))},
	{name: "auto-en-caps", width: 120, base: hyStyle(16, HyphensAuto, "en"), spans: one1("Internationalization Considerations demonstrated", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-en-uppercase", width: 150, base: hyStyle(16, HyphensAuto, "en"), spans: one1("typographical considerations", hyStyle(16, HyphensAuto, "en", func(s *TextStyle) { s.Transform = TransformUppercase }))},
	{name: "auto-en-last-word", width: 150, base: hyStyle(16, HyphensAuto, "en"), spans: one1("Words before internationalization", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-en-one-word", width: 80, base: hyStyle(16, HyphensAuto, "en"), spans: one1("internationalization", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-en-punct", width: 130, base: hyStyle(16, HyphensAuto, "en"), spans: one1("He said “internationalization,” and (considerations) again", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-en-gb", width: 120, base: hyStyle(16, HyphensAuto, "en-GB"), spans: one1(autoGB, hyStyle(16, HyphensAuto, "en-GB"))},
	{name: "auto-en-au", width: 120, base: hyStyle(16, HyphensAuto, "en-AU"), spans: one1(autoGB, hyStyle(16, HyphensAuto, "en-AU"))},
	{name: "auto-es", width: 130, base: hyStyle(16, HyphensAuto, "es"), spans: one1(autoES, hyStyle(16, HyphensAuto, "es"))},
	{name: "auto-es-mx-justify", width: 150, para: ParagraphStyle{Align: AlignJustify}, base: hyStyle(16, HyphensAuto, "es-MX"), spans: one1(autoES, hyStyle(16, HyphensAuto, "es-MX"))},
	{name: "auto-es-caps", width: 100, base: hyStyle(16, HyphensAuto, "es"), spans: one1("Latinoamericana Extraordinariamente", hyStyle(16, HyphensAuto, "es"))},
	{name: "auto-fr", width: 140, base: hyStyle(16, HyphensAuto, "fr"), spans: one1(autoFR, hyStyle(16, HyphensAuto, "fr"))},
	{name: "auto-no-lang", width: 100, base: hyStyle(16, HyphensAuto, ""), spans: one1(autoEn, hyStyle(16, HyphensAuto, ""))},
	{name: "auto-unknown-lang", width: 100, base: hyStyle(16, HyphensAuto, "zz"), spans: one1(autoEn, hyStyle(16, HyphensAuto, "zz"))},
	{name: "auto-soft", width: 110, base: hyStyle(16, HyphensAuto, "en"), spans: one1("The extra\u00adordinarily internationalization", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-span-manual", width: 100, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Some typographical ", style: hyStyle(16, HyphensAuto, "en")},
		{text: "considerations", style: hyStyle(16, HyphensManual, "en")},
		{text: " demonstrated", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-balance", width: 200, para: ParagraphStyle{WrapStyle: TextWrapBalance}, base: hyStyle(16, HyphensAuto, "en"), spans: one1(autoText, hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-pretty", width: 170, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: hyStyle(16, HyphensAuto, "en"), spans: one1(autoText+" again", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-pretty-short", width: 130, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: hyStyle(16, HyphensAuto, "en"),
		spans: one1("Good typography makes the reading of extraordinary paragraphs easy", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-pretty-justify", width: 110, para: ParagraphStyle{WrapStyle: TextWrapPretty, Align: AlignJustify}, base: hyStyle(16, HyphensAuto, "en"),
		spans: one1("Good typography makes the reading of extraordinary paragraphs easy", hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-clamp", width: 120, para: ParagraphStyle{LineClamp: 2}, base: hyStyle(16, HyphensAuto, "en"), spans: one1(autoText, hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-min-content", minWidth: true, base: hyStyle(16, HyphensAuto, "en"), spans: one1(autoText, hyStyle(16, HyphensAuto, "en"))},
	{name: "auto-es-min-content", minWidth: true, base: hyStyle(16, HyphensAuto, "es"), spans: one1(autoES, hyStyle(16, HyphensAuto, "es"))},
	// Out-of-flow boxes (anchors; an empty absolutely positioned span in
	// Chromium) after, inside and before hyphenated words: Blink's
	// out-of-flow items have no text, the word is the same without them.
	{name: "auto-anchor-after", width: 150, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Words and internationalization ", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: " more text", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-anchor-glued", width: 150, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Words and internationalization", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: " more text", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-anchor-inside", width: 190, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Words and interna", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: "tionalization more text", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-anchor-inside-narrow", width: 110, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Words and interna", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: "tionalization more text", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-anchor-last", width: 150, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Words before internationalization ", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
	}},
	{name: "auto-anchor-min-content", minWidth: true, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Words and internationalization ", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: " more text", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-anchor-balance", width: 200, para: ParagraphStyle{WrapStyle: TextWrapBalance}, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "The typographical considerations ", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: " of hyphenation demonstrate extraordinary internationalization requirements everywhere", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-anchor-inside-balance", width: 150, para: ParagraphStyle{WrapStyle: TextWrapBalance}, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: "Words and interna", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: "tionalization more text", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "auto-anchor-pretty", width: 170, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: hyStyle(16, HyphensAuto, "en"), spans: []chromiumSpan{
		{text: autoText + " ", style: hyStyle(16, HyphensAuto, "en")}, hyAnchor,
		{text: " again", style: hyStyle(16, HyphensAuto, "en")},
	}},
	{name: "soft-anchor", width: 100, base: hyStyle(16, HyphensManual, ""), spans: []chromiumSpan{
		{text: "Ex\u00adtra\u00ador\u00addi\u00ad", style: hyStyle(16, HyphensManual, "")}, hyAnchor,
		{text: "nary in\u00adter\u00adna\u00adtion\u00adal\u00adiza\u00adtion", style: hyStyle(16, HyphensManual, "")},
	}},
	{name: "soft-anchor-none", width: 100, base: hyStyle(16, HyphensNone, ""), spans: []chromiumSpan{
		{text: "Ex\u00adtra\u00ador\u00addi\u00ad", style: hyStyle(16, HyphensNone, "")}, hyAnchor,
		{text: "nary in\u00adter\u00adna\u00adtion\u00adal\u00adiza\u00adtion", style: hyStyle(16, HyphensNone, "")},
	}},
}

// hyAnchor is an out-of-flow box of the hyphenation cases: an anchor, an
// empty absolutely positioned span in Chromium.
var hyAnchor = chromiumSpan{box: &InlineBox{Anchor: true}}

func (c *chromiumHyphenCase) paragraph() *Paragraph {
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

// cssHyphens returns the CSS of a style's hyphens.
func cssHyphens(h Hyphens) string {
	return [...]string{" hyphens: manual;", " hyphens: none;", " hyphens: auto;"}[h]
}

func cssHyphenCase(c *chromiumHyphenCase) string {
	var b strings.Builder
	if c.minWidth {
		b.WriteString("width: min-content; ")
	} else {
		fmt.Fprintf(&b, "width: %gpx; ", c.width)
	}
	b.WriteString(cssPara(c.para) + " " + cssStyle(c.base) + cssHyphens(c.base.Hyphens))
	b.WriteString([...]string{"", " text-wrap-style: balance;", " text-wrap-style: pretty;"}[c.para.WrapStyle])
	if k := c.para.LineClamp; k > 0 {
		fmt.Fprintf(&b, " overflow: hidden; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: %d;", k)
	}
	return b.String()
}

func chromiumHyphenHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(chromiumFonts)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; background: #fff; }\n.case { margin: 0 0 40px 8px; }\n</style></head><body>\n")
	for i := range chromiumHyphenCases {
		c := &chromiumHyphenCases[i]
		lang := ""
		if c.base.Language != "" {
			lang = fmt.Sprintf(" lang=%q", c.base.Language)
		}
		fmt.Fprintf(&b, "<div class=\"case\" id=\"c%d\"%s style=\"%s\">", i, lang, html.EscapeString(cssHyphenCase(c)))
		for _, s := range c.spans {
			if s.box != nil {
				if !s.box.Anchor {
					panic("the hyphenation cases have only out-of-flow boxes")
				}
				b.WriteString(`<span style="position: absolute;"></span>`)
				continue
			}
			fmt.Fprintf(&b, "<span style=\"%s\">%s</span>", html.EscapeString(cssStyle(s.style)+cssHyphens(s.style.Hyphens)),
				strings.ReplaceAll(html.EscapeString(s.text), "\u00ad", "&shy;"))
		}
		b.WriteString("</div>\n")
	}
	// The lines are the client rects of the whole content grouped by their
	// vertical middle; a line starts at the caret offset of the point just
	// inside its start edge (the left one, the right one in a
	// right-to-left paragraph), counted in code points of the paragraph's
	// text; its edges include the hyphen's rect.
	fmt.Fprintf(&b, `<pre id="out"></pre>
<script>
const n = %d;
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const out = {};
  for (let i = 0; i < n; i++) {
    const div = document.getElementById("c" + i);
    div.scrollIntoView({block: "start"}); // hit-testing needs it in the viewport
    const box = div.getBoundingClientRect();
    const rtl = getComputedStyle(div).direction === "rtl";
    // Code point offsets of the text nodes.
    const nodes = [];
    let cp = 0;
    const walker = document.createTreeWalker(div, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      nodes.push([node, cp]);
      cp += [...node.data].length;
    }
    const cpOf = (node, off) => {
      for (const [nd, base] of nodes) {
        if (nd === node) return base + [...nd.data.slice(0, off)].length;
      }
      return -1;
    };
    const r = document.createRange();
    r.selectNodeContents(div);
    const lines = [];
    for (const q of r.getClientRects()) {
      if (q.width <= 0) continue;
      const mid = q.top + q.height / 2;
      let ln = lines.find(l => mid > l.top && mid < l.bottom);
      if (!ln) {
        ln = {top: q.top, bottom: q.bottom, left: q.left, right: q.right};
        lines.push(ln);
      }
      ln.left = Math.min(ln.left, q.left);
      ln.right = Math.max(ln.right, q.right);
    }
    lines.sort((a, b) => a.top - b.top);
    const res = [];
    for (const ln of lines) {
      const x = rtl ? ln.right - 0.5 : ln.left + 0.5;
      const p = document.caretPositionFromPoint(x, (ln.top + ln.bottom) / 2);
      res.push([p ? cpOf(p.offsetNode, p.offset) : -1, ln.left - box.left, ln.right - box.left]);
    }
    out["c" + i] = {width: box.width, height: box.height, lines: res};
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script></body></html>
`, len(chromiumHyphenCases))
	return b.String()
}

// regenerateChromiumHyphens writes hyphens.html in dir, lays it out in
// Chromium with the hyphenation dictionaries of hyphenDir and writes its
// results to hyphens.json.
func regenerateChromiumHyphens(dir, hyphenDir string) error {
	htmlPath := filepath.Join(dir, "hyphens.html")
	if err := os.WriteFile(htmlPath, []byte(chromiumHyphenHTML()), 0o644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(htmlPath)
	profile, err := os.MkdirTemp("", "tela-chromium-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profile)
	if hyphenDir != "" {
		dst := filepath.Join(profile, "hyphen-data", filepath.Base(hyphenDir))
		if err := os.CopyFS(dst, os.DirFS(hyphenDir)); err != nil {
			return err
		}
	}
	cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--force-device-scale-factor=1", "--user-data-dir="+profile,
		"--allow-file-access-from-files", "--virtual-time-budget=10000", "--window-size=1200,800", "--dump-dom", "file://"+abs)
	outb, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("chromium: %v", err)
	}
	s := string(outb)
	i, j := strings.Index(s, "BEGIN"), strings.Index(s, "END</pre>")
	if i < 0 || j < 0 {
		return fmt.Errorf("chromium output has no result:\n%s", s[:min(len(s), 2000)])
	}
	var parsed map[string]chromiumWrapResult
	if err := json.Unmarshal([]byte(html.UnescapeString(s[i+5:j])), &parsed); err != nil {
		return err
	}
	var out strings.Builder
	out.WriteString("{\n")
	for i := range chromiumHyphenCases {
		k := fmt.Sprintf("c%d", i)
		r := parsed[k]
		r.Width = math.Round(r.Width*1000) / 1000
		for li := range r.Lines {
			for e := 1; e < 3; e++ {
				r.Lines[li][e] = math.Round(r.Lines[li][e]*1000) / 1000
			}
		}
		v, err := json.Marshal(r)
		if err != nil {
			return err
		}
		sep := ","
		if i == len(chromiumHyphenCases)-1 {
			sep = ""
		}
		fmt.Fprintf(&out, "%q: %s%s\n", k, v, sep)
	}
	out.WriteString("}\n")
	return os.WriteFile(filepath.Join(dir, "hyphens.json"), []byte(out.String()), 0o644)
}

// hyphenLines returns the lines of l in the form of chromiumWrapResult:
// the first code point of each line that is not white space or a soft
// hyphen, and the left and right edges of its clusters with an advance,
// trailing white space excluded, and its hyphen.
func hyphenLines(l *Layout) [][3]float64 {
	src := l.Text()
	cpOf := make([]int, len(src)+1)
	k := 0
	for i := 0; i < len(src); {
		_, n := utf8.DecodeRuneInString(src[i:])
		for j := range n {
			cpOf[i+j] = k
		}
		i += n
		k++
	}
	cpOf[len(src)] = k
	var out [][3]float64
	for li := range l.NumLines() {
		ln := l.Line(li)
		first := -1
		left, right := math.Inf(1), math.Inf(-1)
		for ci := ln.ClusterStart; ci < ln.ClusterEnd; ci++ {
			cl := l.Cluster(ci)
			if cl.Line != li {
				continue
			}
			r, _ := utf8.DecodeRuneInString(src[cl.TextStart:])
			if first < 0 && !unicode.IsSpace(r) && r != softHyphen {
				first = cpOf[cl.TextStart]
			}
			if cl.Advance <= 0 || cl.Flags&(ClusterSpace|ClusterNewline|ClusterHanging) != 0 {
				continue
			}
			left, right = math.Min(left, float64(cl.X)), math.Max(right, float64(cl.X+cl.Advance))
		}
		if ln.Hyphen {
			hw := l.lines[li].hyw
			left, right = math.Min(left, float64(ln.HyphenX)), math.Max(right, float64(ln.HyphenX+hw))
		}
		if first >= 0 {
			out = append(out, [3]float64{float64(first), left, right})
		}
	}
	return out
}

// hyphenCollection returns the test fonts with the hyphenation
// dictionaries of the hyphen packages.
func hyphenCollection(t testing.TB) *Collection {
	c := testCollection(t)
	c.SetHyphenators([]*Hyphenator{testHyphenUS(), testHyphenGB(), testHyphenES(), testHyphenFR()})
	return c
}

func TestChromiumHyphens(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := regenerateChromiumHyphens(dir, os.Getenv("TELA_CHROMIUM_HYPHEN")); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "hyphens.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "hyphens.html")); err != nil || string(h) != chromiumHyphenHTML() {
		t.Fatal("testdata/chromium/hyphens.html is out of date: the cases changed; regenerate with TELA_CHROMIUM=1")
	}
	var results map[string]chromiumWrapResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	coll := hyphenCollection(t)
	for i := range chromiumHyphenCases {
		c := &chromiumHyphenCases[i]
		t.Run(c.name, func(t *testing.T) {
			res, ok := results[fmt.Sprintf("c%d", i)]
			if !ok {
				t.Fatal("no Chromium result; regenerate with TELA_CHROMIUM=1")
			}
			l := NewLayout(coll, c.paragraph())
			w := c.width
			if c.minWidth {
				w = l.MinContentWidth()
				if d := math.Abs(float64(w) - res.Width); d > wrapEdgeTolerance {
					t.Errorf("min-content width %.3f, Chromium %.3f", w, res.Width)
				}
			}
			l.BreakLines(w)
			got := hyphenLines(l)
			want := res.Lines
			if k := c.para.LineClamp; k > 0 && len(want) > k {
				want = want[:k]
			}
			if len(got) != len(want) {
				t.Fatalf("%d lines %v, Chromium %d %v", len(got), lineStarts(got), len(want), lineStarts(want))
			}
			var problems []string
			for i := range got {
				g, w := got[i], want[i]
				if g[0] != w[0] {
					problems = append(problems, fmt.Sprintf("line %d starts at code point %g, Chromium at %g", i, g[0], w[0]))
					continue
				}
				if c.para.LineClamp > 0 && i == len(got)-1 {
					// Chromium's rects include the text the ellipsis
					// hides (and the hyphen it replaces); the golden
					// compares the pixels.
					continue
				}
				d := math.Max(math.Abs(g[1]-w[1]), math.Abs(g[2]-w[2]))
				hyphenMaxEdge = max(hyphenMaxEdge, d)
				if d > wrapEdgeTolerance {
					problems = append(problems, fmt.Sprintf("line %d spans [%.3f, %.3f], Chromium [%.3f, %.3f]", i, g[1], g[2], w[1], w[2]))
				}
			}
			if c.para.WrapStyle != TextWrapAuto {
				// Every balanced or pretty case moves greedy breaks.
				p := c.paragraph()
				p.Style.WrapStyle = TextWrapAuto
				g := NewLayout(coll, p)
				g.BreakLines(w)
				if fmt.Sprint(lineStarts(hyphenLines(g))) == fmt.Sprint(lineStarts(got)) {
					problems = append(problems, "the lines are the greedy ones")
				}
			}
			if len(problems) > 0 {
				t.Errorf("differences from Chromium:\n%s", strings.Join(problems, "\n"))
			}
		})
	}
	t.Logf("largest line edge difference: %.3f px", hyphenMaxEdge)
}

// hyphenMaxEdge is the largest line edge difference of TestChromiumHyphens.
var hyphenMaxEdge float64
