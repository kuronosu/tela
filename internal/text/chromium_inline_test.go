package text

import (
	"cmp"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The inline comparison lays out paragraphs with inlines (CSS inline boxes
// with margins, borders and padding) here and in Chromium, and compares
// the fragments of every inline (Element.getClientRects: one border box
// per fragment) besides what TestChromiumLayout compares (line breaks,
// cluster edges, inline boxes, height). Chromium's results are committed
// in testdata/chromium/inline.json; regenerate them with
//
//	TELA_CHROMIUM=1 go test -run TestChromiumInline ./internal/text/
//
// which writes testdata/chromium/inline.html and runs chromium --headless
// --dump-dom on it.

// inode is a piece of the content of an inline case: a text in the style
// of the enclosing inline, an inline-block placeholder, or an inline.
type inode struct {
	text string
	box  *InlineBox
	in   *ispec
}

// ispec is an inline: its text style, its edges (top, right, bottom,
// left; only the left and right margins apply to an inline) and its
// content.
type ispec struct {
	style          TextStyle
	margin, border [4]float32
	padding        [4]float32
	kids           []inode
}

type inlineCase struct {
	name  string
	width float32
	para  ParagraphStyle
	base  TextStyle
	kids  []inode
}

func tx(s string) inode { return inode{text: s} }

func sp(style TextStyle, padding [4]float32, kids ...inode) inode {
	return inode{in: &ispec{style: style, padding: padding, kids: kids}}
}

// edged returns an inline with margins, borders and padding.
func edged(style TextStyle, margin, border, padding [4]float32, kids ...inode) inode {
	return inode{in: &ispec{style: style, margin: margin, border: border, padding: padding, kids: kids}}
}

func px(top, right, bottom, left float32) [4]float32 { return [4]float32{top, right, bottom, left} }

func (s *ispec) left() float32  { return s.margin[3] + s.border[3] + s.padding[3] }
func (s *ispec) right() float32 { return s.margin[1] + s.border[1] + s.padding[1] }

// paragraph builds the Paragraph of the case and the spec of each inline,
// in Paragraph.Inlines order.
func (c *inlineCase) paragraph() (*Paragraph, []*ispec) {
	p := &Paragraph{Style: c.para, Default: c.base}
	var specs []*ispec
	var walk func(kids []inode, st TextStyle)
	walk = func(kids []inode, st TextStyle) {
		for _, k := range kids {
			switch {
			case k.box != nil:
				p.AddBox(*k.box)
			case k.in != nil:
				i := p.OpenInline(Inline{ID: len(specs), Style: k.in.style, Left: k.in.left(), Right: k.in.right()})
				specs = append(specs, k.in)
				walk(k.in.kids, k.in.style)
				p.CloseInline(i)
			default:
				p.AddText(k.text, st)
			}
		}
	}
	walk(c.kids, c.base)
	return p, specs
}

var (
	mono14  = ts(14, LineHeightPixels(20), func(s *TextStyle) { s.Families = []string{"JetBrains Mono"} })
	sans16  = ts(16, lh24)
	bold16  = ts(16, lh24, func(s *TextStyle) { s.Weight = 700 })
	rtlPara = ParagraphStyle{Direction: DirectionRTL}
	chip    = px(2, 4, 2, 4)
	wide    = px(0, 10, 0, 4)
	nopad   = px(0, 0, 0, 0)
)

var inlineCases = []inlineCase{
	{name: "code-chip", width: 400, base: sans16, kids: []inode{
		tx("Run "), sp(mono14, chip, tx("go test")), tx(" now"),
	}},
	{name: "wrap-3-lines", width: 120, base: sans16, kids: []inode{
		tx("Some "), edged(sans16, px(0, 3, 0, 5), px(1, 2, 1, 2), px(2, 6, 2, 6), tx("decorated text that wraps over three lines")), tx(" end."),
	}},
	{name: "nested", width: 400, base: sans16, kids: []inode{
		tx("A "), edged(sans16, nopad, px(0, 1, 0, 1), px(0, 8, 0, 8), tx("link with "), sp(mono14, chip, tx("code()")), tx(" inside")), tx(" done."),
	}},
	{name: "nested-wrap", width: 110, base: sans16, kids: []inode{
		tx("A "), edged(sans16, nopad, px(0, 1, 0, 1), px(0, 8, 0, 8), tx("link with "), sp(mono14, chip, tx("code()")), tx(" inside")), tx(" done."),
	}},
	{name: "margins", width: 400, base: sans16, kids: []inode{
		tx("x"), edged(sans16, px(0, 7, 0, 9), px(0, 2, 0, 3), px(4, 6, 4, 5), tx(" y ")), tx("z"),
	}},
	{name: "empty", width: 400, base: sans16, kids: []inode{
		tx("a "), sp(sans16, wide), tx(" b"), sp(sans16, wide), tx("c"),
	}},
	{name: "empty-at-break", width: 60, base: sans16, kids: []inode{
		tx("aaa "), sp(sans16, wide), tx("bbbbbbbb"),
	}},
	{name: "edge-moves", width: 60, base: sans16, kids: []inode{
		tx("aaa "), sp(sans16, wide, tx("bbbbbbbb")),
	}},
	{name: "edge-stays", width: 60, base: sans16, kids: []inode{
		tx("aaa"), sp(sans16, wide, tx(" bbbbbbbb")),
	}},
	{name: "end-edge-trailing-space", width: 60, base: sans16, kids: []inode{
		sp(sans16, wide, tx("aaa ")), tx("bbbbbbbb"),
	}},
	{name: "collapsed", width: 400, base: sans16, kids: []inode{
		tx("a "), sp(sans16, wide, tx(" ")), tx("b"),
	}},
	{name: "bidi-split", width: 400, base: sans16, kids: []inode{
		tx("abc "), sp(sans16, wide, tx("xyz عربي")), tx("يي def"),
	}},
	{name: "bidi-split-inside", width: 400, base: sans16, kids: []inode{
		tx("abc "), sp(sans16, wide, tx("xyz عر")), tx("بي def"),
	}},
	{name: "rtl-run", width: 400, base: sans16, kids: []inode{
		tx("عربي "), sp(sans16, wide, tx("مرحبا")), tx(" بكم"),
	}},
	{name: "rtl-para", width: 400, para: rtlPara, base: sans16, kids: []inode{
		tx("abc "), sp(sans16, wide, tx("xyz عربي")), tx("يي def"),
	}},
	{name: "rtl-para-wrap", width: 130, para: rtlPara, base: sans16, kids: []inode{
		tx("مرحبا "), edged(sans16, px(0, 2, 0, 6), px(0, 1, 0, 3), px(0, 7, 0, 5), tx("بكم في عالم النصوص العربية")), tx(" نهاية."),
	}},
	{name: "strut", width: 200, base: sans16, kids: []inode{
		tx("x "), sp(ts(40, lh24), nopad, sp(ts(10, LineHeightPixels(10)), nopad, tx("small"))), tx(" y"),
	}},
	{name: "vertical-edges", width: 400, base: sans16, kids: []inode{
		tx("x"), edged(bold16, nopad, px(3, 1, 2, 1), px(4, 6, 4, 6), tx("y")), tx("z"),
	}},
	{name: "box-inside", width: 400, base: sans16, kids: []inode{
		tx("Icon "), sp(sans16, chip, tx("a "), inode{box: &InlineBox{Width: 16, Height: 16}}, tx(" b")), tx(" end"),
	}},
	{name: "ellipsis", width: 120, para: ParagraphStyle{WhiteSpace: WhiteSpaceNowrap, TextOverflow: TextOverflowEllipsis}, base: sans16, kids: []inode{
		tx("aaa "), sp(sans16, wide, tx("bb")), tx(" cccccccccccccccc"),
	}},
	{name: "start-line", width: 70, base: sans16, kids: []inode{
		sp(sans16, wide, tx("first")), tx(" "), sp(sans16, wide, tx("second")), tx(" "), sp(sans16, wide, tx("third")),
	}},
	{name: "kerning", width: 400, base: sans16, kids: []inode{
		tx("AV"), sp(sans16, nopad, tx("AV")), tx(" AV"), sp(sans16, px(0, 0, 0, 5), tx("AV")),
	}},
}

func cssEdges(e [4]float32) string { return fmt.Sprintf("%gpx %gpx %gpx %gpx", e[0], e[1], e[2], e[3]) }

func chromiumInlineHTML() string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(chromiumFonts)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; background: #fff; }\n.case { margin: 0 0 40px 8px; }\n" +
		".i { border-style: solid; border-color: rgba(30, 64, 175, 0.6); background: rgba(250, 204, 21, 0.35); }\n" +
		".ibox { display: inline-block; background: #4f46e5; }\n</style></head><body>\n")
	var walk func(kids []inode)
	walk = func(kids []inode) {
		for _, k := range kids {
			switch {
			case k.box != nil:
				fmt.Fprintf(&b, "<span class=\"ibox\" style=\"width: %gpx; height: %gpx;\"></span>", k.box.Width, k.box.Height)
			case k.in != nil:
				s := k.in
				fmt.Fprintf(&b, "<span class=\"i\" style=\"%s\">", html.EscapeString(fmt.Sprintf("%s margin: 0 %gpx 0 %gpx; border-width: %s; padding: %s;",
					cssStyle(s.style), s.margin[1], s.margin[3], cssEdges(s.border), cssEdges(s.padding))))
				walk(s.kids)
				b.WriteString("</span>")
			default:
				b.WriteString(html.EscapeString(k.text))
			}
		}
	}
	for i, c := range inlineCases {
		extra := ""
		if c.para.TextOverflow == TextOverflowEllipsis {
			extra = " overflow: hidden; text-overflow: ellipsis;" // text-overflow needs a clipping block
		}
		fmt.Fprintf(&b, "<div class=\"case\" id=\"c%d\" style=\"%s\">", i, html.EscapeString(fmt.Sprintf("width: %gpx; %s %s%s", c.width, cssPara(c.para), cssStyle(c.base), extra)))
		walk(c.kids)
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
    const rel = q => [q.left - box.left, q.top - box.top, q.width, q.height];
    const chars = [];
    const walker = document.createTreeWalker(div, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      let off = 0;
      for (const ch of node.data) {
        const r = document.createRange();
        r.setStart(node, off);
        r.setEnd(node, off + ch.length);
        const rects = [...r.getClientRects()].filter(q => q.width > 0 || q.height > 0);
        chars.push(rects.length ? rects.map(rel) : null);
        off += ch.length;
      }
    }
    const boxes = [...div.querySelectorAll(".ibox")].map(e => rel(e.getBoundingClientRect()));
    const inlines = [...div.querySelectorAll(".i")].map(e => [...e.getClientRects()].map(rel));
    out["c" + i] = {height: box.height, chars, boxes, inlines};
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script></body></html>
`, len(inlineCases))
	return b.String()
}

type chromiumInlineResult struct {
	chromiumResult
	Inlines [][][]float64 `json:"inlines"` // per inline, its fragments' border boxes
}

func regenerateChromiumInline(t *testing.T, dir string) {
	htmlPath := filepath.Join(dir, "inline.html")
	if err := os.WriteFile(htmlPath, []byte(chromiumInlineHTML()), 0o644); err != nil {
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
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(html.UnescapeString(s[i+5:j])), &parsed); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	out.WriteString("{\n")
	for i := range inlineCases {
		k := fmt.Sprintf("c%d", i)
		sep := ","
		if i == len(inlineCases)-1 {
			sep = ""
		}
		fmt.Fprintf(&out, "%q: %s%s\n", k, parsed[k], sep)
	}
	out.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "inline.json"), []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// inlineTolerance bounds the difference of fragment edges, in px (the
// same as cluster edges: Chromium positions in 1/64 px).
const inlineTolerance = 0.15

// knownInlineDifferences are the understood differences of the inline
// cases, bounded.
var knownInlineDifferences = map[string]knownDifference{}

// fragmentBorderBoxes returns the border box of every fragment of inline
// k: the margin box without the margins it has, and the content area
// grown by the vertical padding and borders.
func fragmentBorderBoxes(l *Layout, k int, s *ispec) [][4]float64 {
	var out [][4]float64
	for _, f := range l.InlineFragments(nil) {
		if f.Inline != k {
			continue
		}
		x0, x1 := f.X0, f.X1
		if f.Left {
			x0 += s.margin[3]
		}
		if f.Right {
			x1 -= s.margin[1]
		}
		top := f.Baseline - f.Ascent - s.padding[0] - s.border[0]
		bottom := f.Baseline + f.Descent + s.padding[2] + s.border[2]
		out = append(out, [4]float64{float64(x0), float64(top), float64(x1 - x0), float64(bottom - top)})
	}
	return out
}

func sortRects(rs [][4]float64) {
	slices.SortFunc(rs, func(a, b [4]float64) int {
		if c := cmp.Compare(math.Round(a[1]), math.Round(b[1])); c != 0 {
			return c
		}
		return cmp.Compare(a[0], b[0])
	})
}

func TestChromiumInline(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateChromiumInline(t, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "inline.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "inline.html")); err != nil || string(h) != chromiumInlineHTML() {
		t.Fatal("testdata/chromium/inline.html is out of date: the cases changed; regenerate with TELA_CHROMIUM=1")
	}
	var results map[string]chromiumInlineResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	worst := 0.0
	for i := range inlineCases {
		c := &inlineCases[i]
		t.Run(c.name, func(t *testing.T) {
			res, ok := results[fmt.Sprintf("c%d", i)]
			if !ok {
				t.Fatal("no Chromium result; regenerate with TELA_CHROMIUM=1")
			}
			p, specs := c.paragraph()
			l := NewLayout(layoutCollection(t), p)
			l.BreakLines(c.width)
			mm := compareLayout(t, c.name, l, res.chromiumResult)
			var problems []string
			problems = append(problems, mm.lines...)
			problems = append(problems, mm.edges...)
			problems = append(problems, mm.baseline...)
			if mm.height != "" {
				problems = append(problems, mm.height)
			}
			if len(res.Inlines) != len(specs) {
				t.Fatalf("%d inlines here, %d in Chromium", len(specs), len(res.Inlines))
			}
			maxFrag := 0.0
			for k, s := range specs {
				got := fragmentBorderBoxes(l, k, s)
				var want [][4]float64
				for _, r := range res.Inlines[k] {
					want = append(want, [4]float64{r[0], r[1], r[2], r[3]})
				}
				sortRects(got)
				sortRects(want)
				if len(got) != len(want) {
					problems = append(problems, fmt.Sprintf("inline %d: %d fragments %v, Chromium %d %v", k, len(got), got, len(want), want))
					continue
				}
				for f := range got {
					g, w := got[f], want[f]
					d := math.Max(math.Max(math.Abs(g[0]-w[0]), math.Abs(g[1]-w[1])),
						math.Max(math.Abs(g[0]+g[2]-w[0]-w[2]), math.Abs(g[1]+g[3]-w[1]-w[3])))
					maxFrag = math.Max(maxFrag, d)
					if d > inlineTolerance {
						problems = append(problems, fmt.Sprintf("inline %d fragment %d: [%.3f %.3f %.3f %.3f], Chromium [%.3f %.3f %.3f %.3f]",
							k, f, g[0], g[1], g[2], g[3], w[0], w[1], w[2], w[3]))
					}
				}
			}
			worst = math.Max(worst, math.Max(maxFrag, mm.maxEdge))
			t.Logf("max fragment edge difference %.3f px, max cluster edge difference %.3f px", maxFrag, mm.maxEdge)
			if len(problems) == 0 {
				if _, ok := knownInlineDifferences[c.name]; ok {
					t.Errorf("no difference from Chromium any more: remove the case from knownInlineDifferences")
				}
				return
			}
			msg := strings.Join(problems, "\n")
			if k, ok := knownInlineDifferences[c.name]; ok {
				if (len(mm.lines) == 0 || k.lines) && math.Max(maxFrag, mm.maxEdge) <= k.maxEdge+1e-9 && len(mm.baseline) == 0 {
					t.Logf("known difference (%s):\n%s", k.why, msg)
					return
				}
				t.Errorf("differences from Chromium beyond the known ones (%s):\n%s", k.why, msg)
				return
			}
			t.Errorf("differences from Chromium:\n%s", msg)
		})
	}
	t.Logf("largest edge difference over the inline cases: %.3f px", worst)
}
