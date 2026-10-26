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

// The text-wrap comparison lays out the paragraphs of wrapCases here and in
// Chromium with text-wrap: balance or pretty (same font files through
// @font-face, one div per case) and compares the lines: the first visible
// character of each line, and the left and right edges of its visible
// characters, which also checks that balanced lines are aligned in the
// full width of the box. Chromium's results are committed in
// testdata/chromium/wrap.json, so the test runs without Chromium. To
// regenerate them:
//
//	TELA_CHROMIUM=1 go test -run TestChromiumWrapStyle ./internal/text/
//
// which writes testdata/chromium/wrap.html and runs chromium --headless
// --dump-dom on it (Chromium 154 for the committed data).
//
// This file uses the exported API of the package only, plus the helpers of
// chromium_layout_test.go.

type wrapCase struct {
	name  string
	width float32
	para  ParagraphStyle
	base  TextStyle
	spans []chromiumSpan
	// greedy reports that the lines are the greedy ones (balancing or
	// prettifying does not apply); otherwise they must differ from them,
	// so that every case exercises what it is about.
	greedy bool
}

func (c *wrapCase) paragraph() *Paragraph {
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

func balanced(mods ...func(*ParagraphStyle)) ParagraphStyle {
	p := ParagraphStyle{WrapStyle: TextWrapBalance}
	for _, m := range mods {
		m(&p)
	}
	return p
}

const (
	wrapHeadline = "Balanced headlines read better than ragged ones do"
	wrapFox      = "The quick brown fox jumps over the lazy dog while the band plays on and on into the night"
	wrapArabic   = "العربية هي لغة سامية يتحدث بها أكثر من أربعمائة مليون شخص، وهي لغة رسمية في اثنتين وعشرين دولة."
	// wrapTypography ends in a short word alone on the last line.
	wrapTypography = "Typography is the art and technique of arranging type to make written language legible, readable and appealing when displayed"
)

var wrapCases = []wrapCase{
	// Score line breaker: 2 to 6 lines.
	{name: "two-lines", width: 300, para: balanced(), base: ts(16, lh24), spans: one(wrapHeadline, ts(16, lh24))},
	{name: "three-lines", width: 300, para: balanced(), base: ts(16, lh24), spans: one(wrapFox, ts(16, lh24))},
	{name: "four-lines", width: 220, para: balanced(), base: ts(16, lh24), spans: one(wrapFox+" again", ts(16, lh24))},
	{name: "five-lines", width: 280, para: balanced(), base: ts(16, lh24), spans: one(wrapFox+" "+wrapHeadline+" in print", ts(16, lh24))},
	{name: "six-lines", width: 300, para: balanced(), base: ts(16, lh24), spans: one(wrapFox+" "+wrapFox+" and more words", ts(16, lh24))},
	// Seven greedy lines: over Blink's limit, the lines stay greedy.
	{name: "seven-lines", width: 240, para: balanced(), base: ts(16, lh24), spans: one(wrapFox+" "+wrapFox+" and more words", ts(16, lh24)), greedy: true},
	{name: "one-line", width: 500, para: balanced(), base: ts(16, lh24), spans: one(wrapHeadline, ts(16, lh24)), greedy: true},
	{name: "heading", width: 420, para: balanced(), base: ts(36, LineHeightPixels(40)), spans: one("Build beautiful desktop apps in pure Go, without cgo", ts(36, LineHeightPixels(40), func(s *TextStyle) { s.Weight = 700 }))},
	{name: "center", width: 300, para: balanced(func(p *ParagraphStyle) { p.Align = AlignCenter }), base: ts(16, lh24), spans: one(wrapFox, ts(16, lh24))},
	{name: "right", width: 240, para: balanced(func(p *ParagraphStyle) { p.Align = AlignRight }), base: ts(18, lh28), spans: one(wrapHeadline+" in print", ts(18, lh28))},
	{name: "justify", width: 300, para: balanced(func(p *ParagraphStyle) { p.Align = AlignJustify }), base: ts(16, lh24), spans: one(wrapFox, ts(16, lh24))},
	{name: "rtl", width: 240, para: balanced(func(p *ParagraphStyle) { p.Direction = DirectionRTL }), base: ts(18, lh28), spans: one(wrapArabic, ts(18, lh28))},
	{name: "rtl-center", width: 300, para: balanced(func(p *ParagraphStyle) { p.Direction = DirectionRTL; p.Align = AlignCenter }), base: ts(16, lh24), spans: one(wrapArabic, ts(16, lh24))},
	{name: "bidi", width: 240, para: balanced(), base: ts(16, lh24), spans: one("The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done.", ts(16, lh24))},
	{name: "spans", width: 280, para: balanced(), base: ts(16, lh24), spans: []chromiumSpan{
		{text: "Plain words, ", style: ts(16, lh24)},
		{text: "bold words", style: ts(16, lh24, func(s *TextStyle) { s.Weight = 700 })},
		{text: " and ", style: ts(16, lh24)},
		{text: "larger", style: ts(20, lh24)},
		{text: " ones that wrap", style: ts(16, lh24)},
	}},
	{name: "inline-boxes", width: 200, para: balanced(), base: ts(16, lh24), spans: []chromiumSpan{
		{text: "Icons ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 16, Height: 16}},
		{text: " sit on the baseline and ", style: ts(16, lh24)},
		{box: &InlineBox{Width: 40, Height: 16}},
		{text: " take room on the line", style: ts(16, lh24)},
	}},
	{name: "cjk", width: 300, para: balanced(), base: ts(16, lh24), spans: one("日本語の文章は、漢字、ひらがな、カタカナを組み合わせて書かれます。", ts(16, lh24))},
	// Bisection: fewer than 3 break opportunities, or tabs.
	{name: "three-words", width: 200, para: balanced(), base: ts(16, lh24), spans: one("Extraordinary headline typography", ts(16, lh24))},
	{name: "two-words", width: 150, para: balanced(), base: ts(16, lh24), spans: one("Supercalifragilistic words", ts(16, lh24)), greedy: true},
	{name: "tab", width: 300, para: balanced(func(p *ParagraphStyle) { p.WhiteSpace = WhiteSpacePreWrap }), base: ts(16, lh24), spans: one("Name:\tBalanced headlines read better than ragged ones", ts(16, lh24))},
	// Forced breaks: only the part before the first one is balanced, and
	// bisection does not apply.
	{name: "forced-break", width: 300, para: balanced(func(p *ParagraphStyle) { p.WhiteSpace = WhiteSpacePreLine }), base: ts(16, lh24), spans: one(wrapHeadline+"\n"+wrapHeadline, ts(16, lh24))},
	{name: "forced-break-few", width: 200, para: balanced(func(p *ParagraphStyle) { p.WhiteSpace = WhiteSpacePreLine }), base: ts(16, lh24), spans: one("Extraordinary headline typography\n"+wrapHeadline, ts(16, lh24)), greedy: true},
	{name: "pre-wrap", width: 200, para: balanced(func(p *ParagraphStyle) { p.WhiteSpace = WhiteSpacePreWrap }), base: ts(16, lh24), spans: one("Keep   these   spaces  in   balanced  lines  of  text  that  wrap", ts(16, lh24))},
	// A word broken by overflow-wrap: Blink's second pass of the line
	// (RetryAfterOverflow) disables both algorithms.
	{name: "break-word", width: 130, para: balanced(func(p *ParagraphStyle) { p.OverflowWrap = OverflowWrapBreakWord }), base: ts(16, lh24), spans: one("Antidisestablishmentarianism is a long word", ts(16, lh24)), greedy: true},
	// An overflowing word: no balancing.
	{name: "overflow", width: 140, para: balanced(), base: ts(16, lh24), spans: one("A Supercalifragilisticexpialidocious word overflows here", ts(16, lh24)), greedy: true},
	// line-clamp: only the visible lines are balanced (here two full
	// lines, which stay greedy instead of sharing the hidden text).
	{name: "clamp", width: 300, para: balanced(func(p *ParagraphStyle) { p.LineClamp = 2 }), base: ts(16, lh24), spans: one(wrapFox+" until the morning", ts(16, lh24)), greedy: true},
	{name: "clamp-fits", width: 300, para: balanced(func(p *ParagraphStyle) { p.LineClamp = 3 }), base: ts(16, lh24), spans: one(wrapHeadline, ts(16, lh24))},
	// text-wrap: pretty: a last line of one short word.
	{name: "pretty", width: 320, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: ts(16, lh24), spans: one(wrapTypography, ts(16, lh24))},
	// Five lines: the first one stays, the last four are chosen again.
	{name: "pretty-long", width: 260, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: ts(16, lh24), spans: one(wrapTypography, ts(16, lh24))},
	{name: "pretty-two-words", width: 300, para: ParagraphStyle{WrapStyle: TextWrapPretty}, base: ts(16, lh24), spans: one(wrapHeadline+" in print", ts(16, lh24)), greedy: true},
	{name: "pretty-forced-break", width: 320, para: ParagraphStyle{WrapStyle: TextWrapPretty, WhiteSpace: WhiteSpacePreLine}, base: ts(16, lh24), spans: one(wrapTypography+"\n"+wrapTypography, ts(16, lh24))},
	// Real markup: examples/signup's quote (text-2xl leading-snug
	// font-medium tracking-tight text-balance in a 320 px column) and a
	// wrapped kit.Tooltip (text-xs text-balance, 176 px of content).
	{name: "signup-quote", width: 320, para: balanced(), base: ts(24, LineHeightMultiple(1.375), func(s *TextStyle) { s.Weight = 500; s.LetterSpacing = -0.6 }),
		spans: one("“It is the form I would have written in React, shipped as one static binary.”", ts(24, LineHeightMultiple(1.375), func(s *TextStyle) { s.Weight = 500; s.LetterSpacing = -0.6 }))},
	{name: "tooltip", width: 176, para: balanced(), base: ts(12, LineHeightPixels(16)), spans: one("A longer explanation that does not fit on a single line of the tooltip and has to wrap", ts(12, LineHeightPixels(16)))},
}

// cssWrap returns the CSS declarations of a case's paragraph.
func cssWrap(c *wrapCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "width: %gpx; %s %s", c.width, cssPara(c.para), cssStyle(c.base))
	b.WriteString([...]string{"", " text-wrap-style: balance;", " text-wrap-style: pretty;"}[c.para.WrapStyle])
	if k := c.para.LineClamp; k > 0 {
		// Tailwind's line-clamp-N.
		fmt.Fprintf(&b, " overflow: hidden; display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: %d;", k)
	}
	return b.String()
}

func chromiumWrapHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(chromiumFonts)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; background: #fff; }\n.case { margin: 0 0 40px 8px; }\n</style></head><body>\n")
	for i := range wrapCases {
		c := &wrapCases[i]
		fmt.Fprintf(&b, "<div class=\"case\" id=\"c%d\" style=\"%s\">", i, html.EscapeString(cssWrap(c)))
		for _, s := range c.spans {
			if s.box != nil {
				fmt.Fprintf(&b, "<span style=\"display: inline-block; width: %gpx; height: %gpx; background: #4f46e5;\"></span>", s.box.Width, s.box.Height)
				continue
			}
			fmt.Fprintf(&b, "<span style=\"%s\">%s</span>", html.EscapeString(cssStyle(s.style)), html.EscapeString(s.text))
		}
		b.WriteString("</div>\n")
	}
	// A line is a run of characters whose rects' vertical centers are
	// inside the union of the line's rects so far; white space and
	// characters without a rect (collapsed, hidden) are skipped.
	fmt.Fprintf(&b, `<pre id="out"></pre>
<script>
const n = %d;
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const out = {};
  for (let i = 0; i < n; i++) {
    const div = document.getElementById("c" + i);
    const box = div.getBoundingClientRect();
    const lines = [];
    let cur = null, cp = 0;
    const walker = document.createTreeWalker(div, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      let off = 0;
      for (const ch of node.data) {
        const r = document.createRange();
        r.setStart(node, off);
        r.setEnd(node, off + ch.length);
        off += ch.length;
        const q = [...r.getClientRects()].find(q => q.width > 0);
        if (q && !/\s/.test(ch)) {
          const mid = q.top + q.height / 2 - box.top;
          if (!cur || mid > cur.bottom) {
            cur = {first: cp, left: q.left - box.left, right: q.right - box.left, bottom: q.bottom - box.top};
            lines.push(cur);
          }
          cur.left = Math.min(cur.left, q.left - box.left);
          cur.right = Math.max(cur.right, q.right - box.left);
          cur.bottom = Math.max(cur.bottom, q.bottom - box.top);
        }
        cp++;
      }
    }
    out["c" + i] = {width: box.width, height: box.height, lines: lines.map(l => [l.first, l.left, l.right])};
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script></body></html>
`, len(wrapCases))
	return b.String()
}

type chromiumWrapResult struct {
	Width  float64      `json:"width"`
	Height float64      `json:"height"`
	Lines  [][3]float64 `json:"lines"` // first visible code point, left and right edges
}

// regenerateChromiumWrap writes wrap.html in dir, lays it out in Chromium
// and writes its results to wrap.json.
func regenerateChromiumWrap(dir string) error {
	htmlPath := filepath.Join(dir, "wrap.html")
	if err := os.WriteFile(htmlPath, []byte(chromiumWrapHTML()), 0o644); err != nil {
		return err
	}
	abs, _ := filepath.Abs(htmlPath)
	cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--force-device-scale-factor=1",
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
	// One case per line, with its name.
	var out strings.Builder
	out.WriteString("{\n")
	for i := range wrapCases {
		k := fmt.Sprintf("c%d", i)
		r := parsed[k]
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
		if i == len(wrapCases)-1 {
			sep = ""
		}
		fmt.Fprintf(&out, "%q: %s%s\n", k, v, sep)
	}
	out.WriteString("}\n")
	return os.WriteFile(filepath.Join(dir, "wrap.json"), []byte(out.String()), 0o644)
}

// wrapLines returns the lines of l in the form of chromiumWrapResult: the
// first visible code point of each line that has one, and the left and
// right edges of its visible text clusters other than white space.
func wrapLines(l *Layout) [][3]float64 {
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
		var left, right float64
		for ci := ln.ClusterStart; ci < ln.ClusterEnd; ci++ {
			cl := l.Cluster(ci)
			if cl.Line != li || cl.Advance <= 0 || cl.Flags&(ClusterBox|ClusterSpace|ClusterNewline) != 0 {
				continue
			}
			x0, x1 := float64(cl.X), float64(cl.X+cl.Advance)
			if first < 0 {
				first, left, right = cpOf[cl.TextStart], x0, x1 // clusters are in logical order
				continue
			}
			left, right = min(left, x0), max(right, x1)
		}
		if first >= 0 {
			out = append(out, [3]float64{float64(first), left, right})
		}
	}
	return out
}

// wrapEdgeTolerance bounds the differences of line edges, in px, as
// edgeTolerance does for TestChromiumLayout (Chromium positions in
// LayoutUnits); the largest one is 0.037 px (Chromium 154).
const wrapEdgeTolerance = 0.15

// compareWrap lays out case c at its width and returns its differences
// from Chromium's lines res, and whether the lines are the greedy ones.
func compareWrap(coll *Collection, c *wrapCase, res chromiumWrapResult) (problems []string, greedy bool) {
	l := NewLayout(coll, c.paragraph())
	l.BreakLines(c.width)
	got := wrapLines(l)
	g := c.paragraph()
	g.Style.WrapStyle = TextWrapAuto
	gl := NewLayout(coll, g)
	gl.BreakLines(c.width)
	greedy = fmt.Sprint(lineStarts(wrapLines(gl))) == fmt.Sprint(lineStarts(got))
	want := res.Lines
	if k := c.para.LineClamp; k > 0 && len(want) > k {
		// Chromium lays out the clamped lines too; the last visible one
		// ends with the ellipsis, which hides some of its text.
		want = want[:k]
	}
	if len(got) != len(want) {
		return []string{fmt.Sprintf("%d lines %v, Chromium %d %v", len(got), lineStarts(got), len(want), lineStarts(want))}, greedy
	}
	for i := range got {
		g, w := got[i], want[i]
		if g[0] != w[0] {
			problems = append(problems, fmt.Sprintf("line %d starts at code point %g, Chromium at %g", i, g[0], w[0]))
			continue
		}
		if c.para.LineClamp > 0 && i == len(got)-1 {
			continue // truncated differently: the ellipsis
		}
		if d := math.Max(math.Abs(g[1]-w[1]), math.Abs(g[2]-w[2])); d > wrapEdgeTolerance {
			problems = append(problems, fmt.Sprintf("line %d spans [%.3f, %.3f], Chromium [%.3f, %.3f]", i, g[1], g[2], w[1], w[2]))
		}
	}
	return problems, greedy
}

func lineStarts(lines [][3]float64) []int {
	out := make([]int, len(lines))
	for i, ln := range lines {
		out[i] = int(ln[0])
	}
	return out
}

func TestChromiumWrapStyle(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		if err := regenerateChromiumWrap(dir); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "wrap.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "wrap.html")); err != nil || string(h) != chromiumWrapHTML() {
		t.Fatal("testdata/chromium/wrap.html is out of date: the cases changed; regenerate with TELA_CHROMIUM=1")
	}
	var results map[string]chromiumWrapResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	coll := layoutCollection(t)
	for i := range wrapCases {
		c := &wrapCases[i]
		t.Run(c.name, func(t *testing.T) {
			res, ok := results[fmt.Sprintf("c%d", i)]
			if !ok {
				t.Fatal("no Chromium result; regenerate with TELA_CHROMIUM=1")
			}
			problems, greedy := compareWrap(coll, c, res)
			if greedy != c.greedy {
				problems = append(problems, fmt.Sprintf("the lines are greedy: %v, want %v", greedy, c.greedy))
			}
			if len(problems) > 0 {
				t.Errorf("differences from Chromium:\n%s", strings.Join(problems, "\n"))
			}
		})
	}
}
