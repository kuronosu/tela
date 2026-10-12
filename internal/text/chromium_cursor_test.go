package text

import (
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
	"unicode/utf8"
)

// The cursor comparison lays out the paragraphs of cursorCases here and in
// Chromium (the fonts and setup of TestChromiumLayout, one div with one
// text node per case) and compares:
//
//   - carets: the rectangle of a collapsed DOM range at every code point
//     offset, against the caret of the same offset here (either affinity:
//     a DOM range has none);
//   - hit-testing: document.caretPositionFromPoint along the middle of
//     every line, against HitTest;
//   - selections: Range.getClientRects of some ranges, merged per line,
//     against SelectionRects without the line-break extension (which
//     Chromium paints but does not report);
//   - logical movement: Selection.modify("move", "forward"/"backward",
//     "character") from each end of the text, against MoveForward and
//     MoveBackward.
//
// Chromium's results are committed in testdata/chromium/cursor.json. To
// regenerate them:
//
//	TELA_CHROMIUM=1 go test -run TestChromiumCursor ./internal/text/

type cursorCase struct {
	name  string
	width float32
	para  ParagraphStyle
	text  string
	style TextStyle
}

var cursorCases = []cursorCase{
	{name: "latin-wrap", width: 300, text: latinText, style: ts(16, lh24)},
	{name: "latin-narrow", width: 140, text: latinText, style: ts(16, lh24)},
	{name: "latin-justify", width: 280, para: ParagraphStyle{Align: AlignJustify}, text: latinText, style: ts(16, lh24)},
	{name: "arabic", width: 260, para: ParagraphStyle{Direction: DirectionRTL}, text: arabicText, style: ts(18, lh28)},
	{name: "bidi-ltr", width: 600, text: "The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done.", style: ts(16, lh24)},
	{name: "bidi-ltr-wrap", width: 170, text: "The price is 120 دولار أمريكي (about 3 items) and مرحبا 2024 بكم done.", style: ts(16, lh24)},
	{name: "bidi-rtl", width: 600, para: ParagraphStyle{Direction: DirectionRTL}, text: "مرحبا بكم في Tela version 2.0 مع 15 ميزة (جديدة) اليوم.", style: ts(16, lh24)},
	{name: "bidi-rtl-wrap", width: 150, para: ParagraphStyle{Direction: DirectionRTL}, text: "مرحبا بكم في Tela version 2.0 مع 15 ميزة (جديدة) اليوم.", style: ts(16, lh24)},
	{name: "cjk", width: 300, text: cjkText, style: ts(16, lh24)},
	{name: "emoji", width: 230, text: "Hello 👋 world 🌍🎉 family 👨‍👩‍👧‍👦 flags 🇯🇵🇧🇷 thumbs 👍🏽 done ✅ ok", style: ts(16, lh24)},
	{name: "ligature", width: 600, text: "a->b c=>d office -> done", style: ts(16, lh24)},
	{name: "combining", width: 400, text: "été café ñand̃u Zürich", style: ts(16, lh24)},
	{name: "pre-wrap", width: 220, para: ParagraphStyle{WhiteSpace: WhiteSpacePreWrap}, text: "Keep   these spaces\n\tand a tab\tinside\nthen a long line that has to wrap somewhere", style: ts(16, lh24)},
	{name: "collapse", width: 150, text: "  Spaces   and\ttabs\n and   newlines   collapse ", style: ts(16, lh24)},
	{name: "letter-spacing", width: 150, text: "Spaced out letters wrap like this", style: ts(16, lh24, func(s *TextStyle) { s.LetterSpacing = 1.5 })},
}

func (c *cursorCase) layout(t testing.TB) *Layout {
	l := NewLayout(layoutCollection(t), &Paragraph{Text: c.text, Style: c.para, Default: c.style})
	l.BreakLines(c.width)
	return l
}

// hitStep is the spacing of the hit-test samples along a line: not a
// fraction of a pixel that lands on cluster midpoints systematically.
const hitStep = 1.37

// cursorProbe is what the test asks Chromium for one case: the y of each
// line's middle, the x range of the hit-test samples, and the selection
// ranges in code points.
type cursorProbe struct {
	Lines  []float64 `json:"lines"`
	X0, X1 float64
	Ranges [][2]int `json:"ranges"`
}

func cursorProbes(t testing.TB) []cursorProbe {
	var out []cursorProbe
	for i := range cursorCases {
		c := &cursorCases[i]
		l := c.layout(t)
		p := cursorProbe{X0: -4, X1: float64(c.width) + 8}
		for li := range l.NumLines() {
			ln := l.Line(li)
			p.Lines = append(p.Lines, float64(ln.Top+ln.Height/2))
		}
		n := utf8.RuneCountInString(c.text)
		// The whole text, a few lines' worth from the middle of each line
		// to the middle of the next, and short ranges.
		p.Ranges = append(p.Ranges, [2]int{0, n})
		cpOf := codePointIndex(c.text)
		for li := 0; li+1 < l.NumLines(); li++ {
			a, b := l.Line(li), l.Line(li+1)
			p.Ranges = append(p.Ranges, [2]int{cpOf[(a.TextStart+a.TextEnd)/2], cpOf[(b.TextStart+b.TextEnd)/2]})
		}
		seed := uint32(len(c.text))
		for range 6 {
			seed = seed*1664525 + 1013904223
			a := int(seed>>8) % n
			seed = seed*1664525 + 1013904223
			b := a + 1 + int(seed>>8)%12
			p.Ranges = append(p.Ranges, [2]int{a, min(b, n)})
		}
		// Snap the range ends to caret positions (no ranges inside
		// grapheme clusters).
		for k, r := range p.Ranges {
			for e := range r {
				off := byteOfCodePoint(c.text, r[e])
				off = l.SnapCursor(Cursor{off, Downstream}).Offset
				p.Ranges[k][e] = cpOf[off]
			}
		}
		out = append(out, p)
	}
	return out
}

// codePointIndex returns the code point index of every byte offset of s
// (the index of the code point that contains it), len(s) included.
func codePointIndex(s string) []int {
	out := make([]int, len(s)+1)
	k := 0
	for i := 0; i < len(s); {
		_, n := utf8.DecodeRuneInString(s[i:])
		for j := range n {
			out[i+j] = k
		}
		i += n
		k++
	}
	out[len(s)] = k
	return out
}

func byteOfCodePoint(s string, cp int) int {
	k := 0
	for i := range s {
		if k == cp {
			return i
		}
		k++
	}
	return len(s)
}

func chromiumCursorHTML(t testing.TB) string {
	probes := cursorProbes(t)
	pj, err := json.Marshal(probes)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html><head><meta charset=\"utf-8\"><style>\n")
	b.WriteString(chromiumFonts)
	b.WriteString("\nhtml, body { margin: 0; padding: 0; background: #fff; }\n.case { margin: 0 0 40px 8px; }\n</style></head><body>\n")
	for i, c := range cursorCases {
		fmt.Fprintf(&b, "<div class=\"case\" id=\"c%d\" style=\"%s\">%s</div>\n", i,
			html.EscapeString(fmt.Sprintf("width: %gpx; %s %s", c.width, cssPara(c.para), cssStyle(c.style))), html.EscapeString(c.text))
	}
	fmt.Fprintf(&b, `<pre id="out"></pre>
<script>
const probes = %s;
const r2 = v => Math.round(v * 1000) / 1000;
document.fonts.ready.then(() => Promise.all([...document.fonts].map(f => f.load()))).then(() => {
  const out = {};
  for (let i = 0; i < probes.length; i++) {
    const p = probes[i];
    const div = document.getElementById("c" + i);
    const node = div.firstChild;
    // caretPositionFromPoint only sees the viewport.
    div.scrollIntoView({block: "start"});
    const box = div.getBoundingClientRect();
    const u16 = [0];
    for (const ch of node.data) u16.push(u16[u16.length - 1] + ch.length);
    const cpOf = new Map(u16.map((u, k) => [u, k]));
    const rel = r => [r2(r.left - box.left), r2(r.top - box.top), r2(r.width), r2(r.height)];
    const res = {carets: [], hits: [], ranges: [], fwd: [], back: []};
    for (let k = 0; k < u16.length; k++) {
      const r = document.createRange();
      r.setStart(node, u16[k]);
      r.collapse(true);
      const rs = r.getClientRects();
      res.carets.push(rs.length ? rel(rs[0]) : null);
    }
    // Hit-testing: the transitions of the result along each line.
    for (const y of p.lines) {
      const runs = [];
      let last = "";
      for (let n = 0; ; n++) {
        const x = p.X0 + n * %g;
        if (x > p.X1) break;
        const cp = document.caretPositionFromPoint(box.left + x, box.top + y);
        let v = null;
        if (cp && cp.offsetNode === node && cpOf.has(cp.offset)) {
          const cr = cp.getClientRect();
          v = [cpOf.get(cp.offset), cr ? r2(cr.left - box.left) : null, cr ? r2(cr.top + cr.height / 2 - box.top) : null];
        }
        const key = JSON.stringify(v);
        if (key !== last) { runs.push([n, v]); last = key; }
      }
      res.hits.push(runs);
    }
    for (const [a, b] of p.ranges) {
      const r = document.createRange();
      r.setStart(node, u16[a]);
      r.setEnd(node, u16[b]);
      res.ranges.push([...r.getClientRects()].map(rel));
    }
    const sel = window.getSelection();
    sel.collapse(node, 0);
    for (let k = 0; k < 4 * u16.length; k++) {
      sel.modify("move", "forward", "character");
      if (sel.focusNode !== node) break;
      res.fwd.push(cpOf.has(sel.focusOffset) ? cpOf.get(sel.focusOffset) : -1);
      if (sel.focusOffset === node.data.length) break;
    }
    sel.collapse(node, node.data.length);
    for (let k = 0; k < 4 * u16.length; k++) {
      sel.modify("move", "backward", "character");
      if (sel.focusNode !== node) break;
      res.back.push(cpOf.has(sel.focusOffset) ? cpOf.get(sel.focusOffset) : -1);
      if (sel.focusOffset === 0) break;
    }
    out["c" + i] = res;
  }
  document.getElementById("out").textContent = "BEGIN" + JSON.stringify(out) + "END";
});
</script></body></html>
`, pj, hitStep)
	return b.String()
}

type chromiumCursorResult struct {
	Carets [][]float64 `json:"carets"` // per code point offset: [left, top, width, height] or null
	// Hits holds, per line, the runs of equal results along the line:
	// [sample index, [code point, caret x, caret middle y] or null].
	Hits   [][][2]json.RawMessage `json:"hits"`
	Ranges [][][]float64          `json:"ranges"`
	Fwd    []int                  `json:"fwd"`
	Back   []int                  `json:"back"`
}

func regenerateChromiumCursor(t *testing.T, dir string) {
	htmlPath := filepath.Join(dir, "cursor.html")
	if err := os.WriteFile(htmlPath, []byte(chromiumCursorHTML(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(htmlPath)
	cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--force-device-scale-factor=1",
		"--allow-file-access-from-files", "--virtual-time-budget=20000", "--window-size=1200,800", "--dump-dom", "file://"+abs)
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
	for i := range cursorCases {
		sep := ","
		if i == len(cursorCases)-1 {
			sep = ""
		}
		fmt.Fprintf(&out, "%q: %s%s\n", fmt.Sprintf("c%d", i), parsed[fmt.Sprintf("c%d", i)], sep)
	}
	out.WriteString("}\n")
	if err := os.WriteFile(filepath.Join(dir, "cursor.json"), []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// cursorTolerance is the largest caret or edge difference accepted (px):
// cluster edges agree within 0.06 px (TestChromiumLayout), and Chromium
// rounds rects to LayoutUnits.
const cursorTolerance = 0.15

// cursorStats counts the outcomes of one case.
type cursorStats struct {
	carets, caretMiss       int
	hits, hitSame, hitEquiv int
	hitElsewhere            int // same offset, but Chromium shows its caret somewhere else
	hitAmbiguous            int // within the tolerance of a midpoint between two caret stops
	hitCloser               int // another position, whose caret is closer to the pointer than Chromium's
	ranges                  int
	problems                []string
}

func (s *cursorStats) fail(format string, args ...any) {
	if len(s.problems) < 12 {
		s.problems = append(s.problems, fmt.Sprintf(format, args...))
	} else if len(s.problems) == 12 {
		s.problems = append(s.problems, "...")
	}
}

func compareChromiumCursor(t *testing.T, c *cursorCase, p cursorProbe, res chromiumCursorResult) *cursorStats {
	st := &cursorStats{}
	l := c.layout(t)
	src := c.text
	cpOf := codePointIndex(src)
	byteOf := make([]int, 0, len(src)+1)
	for i := range src {
		byteOf = append(byteOf, i)
	}
	byteOf = append(byteOf, len(src))
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) <= cursorTolerance }

	// Carets at every valid caret position.
	for k, r := range res.Carets {
		off := byteOf[k]
		if r == nil || l.SnapCursor(Cursor{off, Downstream}).Offset != off {
			continue
		}
		st.carets++
		li := l.lineAtY(float32(r[1] + r[3]/2))
		ok := false
		for _, a := range []Affinity{Downstream, Upstream} {
			if ca := l.Caret(Cursor{off, a}); ca.Line == li && near(ca.X, float32(r[0])) {
				ok = true
			}
		}
		if !ok {
			st.caretMiss++
			d, u := l.Caret(Cursor{off, Downstream}), l.Caret(Cursor{off, Upstream})
			st.fail("caret at %d (%q|%q): Chromium x=%.3f line %d, here %.3f line %d / %.3f line %d", off,
				excerptBefore(src, off), excerptAfter(src, off), r[0], li, d.X, d.Line, u.X, u.Line)
		}
	}

	// Hit-testing.
	for li, runs := range res.Hits {
		if li >= l.NumLines() {
			st.fail("Chromium has more lines")
			break
		}
		y := float32(p.Lines[li])
		ri := 0
		for n := 0; ; n++ {
			x := p.X0 + float64(n)*hitStep
			if x > p.X1 {
				break
			}
			for ri+1 < len(runs) && sampleIndex(runs[ri+1][0]) <= n {
				ri++
			}
			var v []*float64
			if err := json.Unmarshal(runs[ri][1], &v); err != nil || v == nil {
				continue
			}
			st.hits++
			ours := l.HitTest(float32(x), y)
			oc := l.Caret(ours)
			theirs := byteOf[int(*v[0])]
			if theirs == ours.Offset {
				st.hitSame++
				if v[1] != nil && (!near(oc.X, float32(*v[1])) || l.lineAtY(float32(*v[2])) != oc.Line) {
					st.hitElsewhere++
				}
				continue
			}
			equiv := false
			for _, a := range []Affinity{Downstream, Upstream} {
				if ca := l.Caret(Cursor{theirs, a}); ca.Line == oc.Line && near(ca.X, oc.X) {
					equiv = true
				}
			}
			if equiv {
				st.hitEquiv++
				continue
			}
			if l.nearMidpoint(oc.Line, float32(x)) {
				st.hitAmbiguous++
				continue
			}
			// Chromium can answer a position whose caret is away from the
			// pointer (at bidi boundaries and line starts, the logical
			// start of a right-to-left run drawn at its other end): accept
			// ours when its caret is closer to the pointer.
			if v[1] != nil {
				px := min(max(float32(x), 0), c.width)
				if math.Abs(float64(oc.X-px))+1 < math.Abs(float64(float32(*v[1])-px)) {
					st.hitCloser++
					continue
				}
			}
			st.fail("hit (%.2f, %.1f): here %v (x %.2f), Chromium %d (%q|%q)", x, y, ours, oc.X, theirs, excerptBefore(src, theirs), excerptAfter(src, theirs))
		}
	}

	// Selections.
	for k, rg := range p.Ranges {
		st.ranges++
		a, b := byteOf[rg[0]], byteOf[rg[1]]
		want := mergeByLine(l, res.Ranges[k])
		got := map[int][][2]float32{}
		for _, r := range l.selectionRects(NewSelection(Cursor{a, Downstream}, Cursor{b, Upstream}), nil, false) {
			li := l.lineAtY((r.Y0 + r.Y1) / 2)
			got[li] = append(got[li], [2]float32{r.X0, r.X1})
		}
		for li := range l.NumLines() {
			w, g := want[li], got[li]
			same := len(w) == len(g)
			for i := 0; same && i < len(w); i++ {
				same = near(w[i][0], g[i][0]) && near(w[i][1], g[i][1])
			}
			if !same {
				st.fail("range [%d, %d) %q line %d: here %v, Chromium %v", a, b, src[a:b], li, g, w)
			}
		}
	}

	// Logical movement. Chromium names a position in collapsed white space
	// by the first offset after the visible space, SnapCursor by the last
	// one (the same caret): compare snapped offsets.
	walk := func(m Movement, from Cursor) []int {
		var seq []int
		s := CaretSelection(from)
		for range 4 * len(src) {
			n := l.Move(s, m, false)
			if n.Focus == s.Focus {
				break
			}
			s = n
			seq = append(seq, cpOf[s.Focus.Offset])
		}
		return seq
	}
	snapped := func(cps []int) []int {
		var out []int
		for _, cp := range cps {
			if cp < 0 {
				out = append(out, cp)
				continue
			}
			out = append(out, cpOf[l.SnapCursor(Cursor{byteOf[cp], Downstream}).Offset])
		}
		return out
	}
	if fwd, want := walk(MoveForward, Cursor{0, Downstream}), snapped(res.Fwd); !slices.Equal(fwd, want) {
		st.fail("forward movement:\n here %v\n Chromium %v", fwd, want)
	}
	if back, want := walk(MoveBackward, Cursor{len(src), Downstream}), snapped(res.Back); !slices.Equal(back, want) {
		st.fail("backward movement:\n here %v\n Chromium %v", back, want)
	}
	return st
}

// sampleIndex decodes the sample index of a hit run.
func sampleIndex(m json.RawMessage) int {
	var n int
	_ = json.Unmarshal(m, &n)
	return n
}

// nearMidpoint reports whether x is within the comparison tolerance of the
// middle of two neighboring caret stops of line li, where a sub-pixel
// difference of positions can flip the result.
func (l *Layout) nearMidpoint(li int, x float32) bool {
	for _, el := range l.lineElems(li, nil) {
		for s := int32(0); s < el.g; s++ {
			m := (el.stopX(s) + el.stopX(s+1)) / 2
			if math.Abs(float64(m-x)) <= 2*cursorTolerance {
				return true
			}
		}
	}
	return false
}

// mergeByLine groups Chromium's rects by line of l and merges touching x
// intervals; empty rects are dropped.
func mergeByLine(l *Layout, rects [][]float64) map[int][][2]float32 {
	by := map[int][][2]float32{}
	for _, r := range rects {
		if r[2] <= 0.01 {
			continue
		}
		li := l.lineAtY(float32(r[1] + r[3]/2))
		by[li] = append(by[li], [2]float32{float32(r[0]), float32(r[0] + r[2])})
	}
	for li, iv := range by {
		slices.SortFunc(iv, func(a, b [2]float32) int {
			switch {
			case a[0] < b[0]:
				return -1
			case a[0] > b[0]:
				return 1
			}
			return 0
		})
		out := iv[:1]
		for _, v := range iv[1:] {
			if last := &out[len(out)-1]; v[0] <= last[1]+cursorTolerance {
				last[1] = max(last[1], v[1])
				continue
			}
			out = append(out, v)
		}
		by[li] = out
	}
	return by
}

func excerptBefore(s string, off int) string {
	r := []rune(s[:off])
	return string(r[max(0, len(r)-6):])
}

func excerptAfter(s string, off int) string {
	r := []rune(s[off:])
	return string(r[:min(len(r), 6)])
}

// knownCursorDifferences explains and bounds the differences from
// Chromium per case: the number of problems allowed.
var knownCursorDifferences = map[string]struct {
	why      string
	problems int
}{}

func TestChromiumCursor(t *testing.T) {
	dir := filepath.Join("testdata", "chromium")
	if os.Getenv("TELA_CHROMIUM") == "1" {
		regenerateChromiumCursor(t, dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "cursor.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	if h, err := os.ReadFile(filepath.Join(dir, "cursor.html")); err != nil || string(h) != chromiumCursorHTML(t) {
		t.Fatal("testdata/chromium/cursor.html is out of date: the cases changed; regenerate with TELA_CHROMIUM=1")
	}
	var results map[string]chromiumCursorResult
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	probes := cursorProbes(t)
	for i := range cursorCases {
		c := &cursorCases[i]
		t.Run(c.name, func(t *testing.T) {
			res, ok := results[fmt.Sprintf("c%d", i)]
			if !ok {
				t.Fatal("no Chromium result; regenerate with TELA_CHROMIUM=1")
			}
			st := compareChromiumCursor(t, c, probes[i], res)
			t.Logf("carets %d (%d differ); hits %d: %d same offset (%d with Chromium's caret elsewhere), %d equivalent positions, %d closer to the pointer than Chromium's, %d at a midpoint; ranges %d",
				st.carets, st.caretMiss, st.hits, st.hitSame, st.hitElsewhere, st.hitEquiv, st.hitCloser, st.hitAmbiguous, st.ranges)
			if len(st.problems) == 0 {
				return
			}
			msg := strings.Join(st.problems, "\n")
			if k, ok := knownCursorDifferences[c.name]; ok && len(st.problems) <= k.problems {
				t.Logf("known differences (%s):\n%s", k.why, msg)
				return
			}
			t.Errorf("differences from Chromium:\n%s", msg)
		})
	}
}
