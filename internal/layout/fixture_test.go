package layout

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The conformance suite: Taffy v0.14.0's block, flex, blockflex and leaf
// fixtures (ADR-039), laid out by Tela and compared with the boxes Chromium
// measured (testdata/taffy/chromium.json; see taffy_chromium_test.go). Each
// box edge must be within fixtureTolerance of Chromium's; the test also
// reports how many fixtures match within Chromium's LayoutUnit (1/64 px).
// Fixtures that use features outside Tela's layout, or that depend on
// margin collapsing (ADR-006), are listed with their reason in
// testdata/taffy/excluded.txt.
//
// TELA_LAYOUT_VERBOSE=1 logs every failure in full.

const (
	fixtureTolerance = 0.5
	layoutUnit       = 1.0 / 64
)

// marginCollapsingReason is the exclusion reason of fixtures whose
// Chromium layout depends on margin collapsing. They are still laid out:
// the test checks that they differ from Chromium (else the exclusion is
// stale) and that collapsing could apply in them.
const marginCollapsingReason = "margin collapsing (ADR-006)"

// loadExclusions reads testdata/taffy/excluded.txt: "name: reason" lines.
func loadExclusions(t *testing.T) map[string]string {
	f, err := os.Open(filepath.Join(taffyFixtures.dir, "excluded.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ex := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, reason, ok := strings.Cut(line, ": ")
		if !ok {
			t.Fatalf("excluded.txt: malformed line %q", line)
		}
		if _, dup := ex[name]; dup {
			t.Fatalf("excluded.txt: %s listed twice", name)
		}
		ex[name] = reason
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ex
}

// boxDiff compares a laid out tree with Chromium's boxes. It returns the
// largest edge difference and a description of the first box beyond tol.
func boxDiff(n *testNode, want *chromiumBox, path string, tol float64) (float64, string) {
	l := n.data.Layout()
	// A display: none box has no box in Chromium (getBoundingClientRect
	// is empty at the viewport's origin): only check that it is empty.
	if n.style.Display == DisplayNone {
		if l.Size.W != 0 || l.Size.H != 0 {
			return math.Inf(1), fmt.Sprintf("%s: display: none box has size %gx%g", path, l.Size.W, l.Size.H)
		}
		return 0, ""
	}
	got := [4]float64{float64(l.Location.X), float64(l.Location.Y), float64(l.Size.W), float64(l.Size.H)}
	exp := [4]float64{want.X, want.Y, want.W, want.H}
	worst := 0.0
	msg := ""
	// Compare the edges: left, top, right, bottom.
	edges := [4]float64{got[0] - exp[0], got[1] - exp[1], got[0] + got[2] - exp[0] - exp[2], got[1] + got[3] - exp[1] - exp[3]}
	for _, d := range edges {
		d = math.Abs(d)
		if math.IsNaN(d) {
			d = math.Inf(1)
		}
		worst = math.Max(worst, d)
	}
	if worst > tol {
		msg = fmt.Sprintf("%s: got x=%g y=%g w=%g h=%g, want x=%g y=%g w=%g h=%g", path, got[0], got[1], got[2], got[3], exp[0], exp[1], exp[2], exp[3])
	}
	if len(n.children) != len(want.C) {
		return math.Inf(1), fmt.Sprintf("%s: %d children, Chromium has %d", path, len(n.children), len(want.C))
	}
	for i, c := range n.children {
		d, m := boxDiff(c, want.C[i], fmt.Sprintf("%s/%d", path, i), tol)
		worst = math.Max(worst, d)
		if msg == "" {
			msg = m
		}
	}
	return worst, msg
}

// scrollDiff compares the scroll sizes of the scroll containers in a tree
// with Chromium's scrollWidth and scrollHeight: the larger of the client
// size (padding box minus scrollbars) and the extent of the scrollable
// overflow, which Chromium snaps to whole pixels. It returns the number of
// scroll containers compared and a description of the first mismatch.
func scrollDiff(n *testNode, want *chromiumBox, path string) (int, string) {
	count, msg := 0, ""
	// Chromium's classic scrollbars appear in an overflow: auto axis when
	// the content overflows; Tela's auto reserves no space (overlay
	// scrollbars), so those scroll sizes differ by design.
	auto := n.style.OverflowX == OverflowAuto || n.style.OverflowY == OverflowAuto
	if n.style.isScrollContainer() && !auto {
		count++
		l := n.data.Layout()
		cw := l.Size.W - l.Border.Left - l.Border.Right - l.ScrollbarSize.W
		ch := l.Size.H - l.Border.Top - l.Border.Bottom - l.ScrollbarSize.H
		sw := math.Max(float64(cw), float64(l.ScrollableOverflow.X1))
		sh := math.Max(float64(ch), float64(l.ScrollableOverflow.Y1))
		if math.Abs(sw-want.SW) > 1 || math.Abs(sh-want.SH) > 1 {
			msg = fmt.Sprintf("%s: scroll size %gx%g, Chromium %gx%g", path, sw, sh, want.SW, want.SH)
		}
	}
	for i, c := range n.children {
		if i >= len(want.C) {
			break
		}
		k, m := scrollDiff(c, want.C[i], fmt.Sprintf("%s/%d", path, i))
		count += k
		if msg == "" {
			msg = m
		}
	}
	return count, msg
}

func TestTaffyFixtures(t *testing.T) {
	res := taffyFixtures.load(t)
	exclusions := loadExclusions(t)
	names := taffyFixtures.files(t)
	verbose := os.Getenv("TELA_LAYOUT_VERBOSE") == "1"

	type stats struct{ total, included, passing, exact int }
	perCat := map[string]*stats{}
	byReason := map[string]int{}
	seen := map[string]bool{}
	scrollFixtures := 0

	for _, name := range names {
		cat := strings.SplitN(name, "/", 2)[0]
		st := perCat[cat]
		if st == nil {
			st = &stats{}
			perCat[cat] = st
		}
		st.total++
		seen[name] = true

		ft, err := buildFixture(taffyFixtures.source(t, name), float32(res.ViewportW), float32(res.ViewportH))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(ft.unparsed) > 0 {
			t.Errorf("%s: unparsed declarations %q", name, ft.unparsed)
		}
		reason, excluded := exclusions[name]
		if excluded {
			byReason[reason]++
		}
		// Unsupported features must be listed.
		if len(ft.unsupported) > 0 {
			if !excluded {
				t.Errorf("%s uses %s: add it to excluded.txt", name, ft.unsupported[0])
			} else if !containsString(ft.unsupported, reason) {
				t.Errorf("%s: excluded for %q, but it uses %q", name, reason, ft.unsupported)
			}
			continue
		}
		want := res.Fixtures[name]
		if want == nil {
			t.Errorf("%s: no Chromium result; regenerate with TELA_CHROMIUM=1", name)
			continue
		}

		var e Engine
		e.Compute(ft.viewport, DefiniteSpace(float32(res.ViewportW)), DefiniteSpace(float32(res.ViewportH)))
		worst, msg := boxDiff(ft.root, want, "root", fixtureTolerance)

		if excluded {
			if reason != marginCollapsingReason {
				t.Errorf("%s: excluded for %q, which the parser does not detect", name, reason)
			} else if worst <= fixtureTolerance {
				t.Errorf("%s: excluded for margin collapsing but matches Chromium; remove it from excluded.txt", name)
			} else if !hasCollapsibleMargins(ft.root, false) {
				t.Errorf("%s: excluded for margin collapsing, but no margins can collapse in it", name)
			}
			continue
		}
		st.included++
		if k, msg := scrollDiff(ft.root, want, "root"); k > 0 {
			scrollFixtures++
			if msg != "" {
				t.Errorf("%s: %s", name, msg)
			}
		}
		if worst <= fixtureTolerance {
			st.passing++
			if worst <= layoutUnit+1e-4 {
				st.exact++
			} else if verbose {
				_, msg := boxDiff(ft.root, want, "root", layoutUnit+1e-4)
				t.Logf("%s: within %g px but not 1/64 px: %s", name, fixtureTolerance, msg)
			}
			continue
		}
		if verbose {
			t.Errorf("%s: off by %.4g px\n\t%s\n%s", name, worst, msg, dumpTree(ft.root, want, ""))
		} else {
			t.Errorf("%s: off by %.4g px: %s", name, worst, msg)
		}
	}
	for name := range exclusions {
		if !seen[name] {
			t.Errorf("excluded.txt lists %s, which is not a fixture", name)
		}
	}

	cats := make([]string, 0, len(perCat))
	for c := range perCat {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	var all stats
	for _, c := range cats {
		s := perCat[c]
		t.Logf("%-9s total %3d, included %3d, passing %3d (within 1/64 px: %3d)", c, s.total, s.included, s.passing, s.exact)
		all.total += s.total
		all.included += s.included
		all.passing += s.passing
		all.exact += s.exact
	}
	t.Logf("%-9s total %3d, included %3d, passing %3d (within 1/64 px: %3d)", "all", all.total, all.included, all.passing, all.exact)
	t.Logf("scroll sizes of scroll containers compared in %d fixtures", scrollFixtures)
	reasons := make([]string, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)
	for _, r := range reasons {
		t.Logf("excluded: %3d %s", byReason[r], r)
	}
}

// TestTelaCases lays out Tela's own cases (testdata/cases, the fixture
// format) and compares them with Chromium: behaviour the Taffy fixtures do
// not exercise, such as containing blocks through static boxes and order.
func TestTelaCases(t *testing.T) {
	res := telaCases.load(t)
	exact := 0
	names := telaCases.files(t)
	for _, name := range names {
		ft, err := buildFixture(telaCases.source(t, name), float32(res.ViewportW), float32(res.ViewportH))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(ft.unparsed) > 0 || len(ft.unsupported) > 0 {
			t.Errorf("%s: unparsed %q, unsupported %q", name, ft.unparsed, ft.unsupported)
		}
		want := res.Fixtures[name]
		if want == nil {
			t.Fatalf("%s: no Chromium result; regenerate with TELA_CHROMIUM=1", name)
		}
		var e Engine
		e.Compute(ft.viewport, DefiniteSpace(float32(res.ViewportW)), DefiniteSpace(float32(res.ViewportH)))
		worst, msg := boxDiff(ft.root, want, "root", fixtureTolerance)
		switch {
		case worst > fixtureTolerance:
			t.Errorf("%s: off by %.4g px: %s\n%s", name, worst, msg, dumpTree(ft.root, want, ""))
		case worst <= layoutUnit+1e-4:
			exact++
		}
		if _, msg := scrollDiff(ft.root, want, "root"); msg != "" {
			t.Errorf("%s: %s", name, msg)
		}
	}
	t.Logf("%d cases, %d within 1/64 px", len(names), exact)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// dumpTree prints a laid out tree next to Chromium's boxes.
func dumpTree(n *testNode, want *chromiumBox, indent string) string {
	l := n.data.Layout()
	var b strings.Builder
	fmt.Fprintf(&b, "%sgot  x=%-8g y=%-8g w=%-8g h=%-8g\n", indent, l.Location.X, l.Location.Y, l.Size.W, l.Size.H)
	if want != nil {
		fmt.Fprintf(&b, "%swant x=%-8g y=%-8g w=%-8g h=%-8g\n", indent, want.X, want.Y, want.W, want.H)
	}
	for i, c := range n.children {
		var w *chromiumBox
		if want != nil && i < len(want.C) {
			w = want.C[i]
		}
		b.WriteString(dumpTree(c, w, indent+"  "))
	}
	return b.String()
}

// hasCollapsibleMargins reports whether CSS would collapse two adjoining
// non-zero vertical margins somewhere in the subtree of n (a conservative
// check of the margin-collapsing exclusions): between a block-level child
// and its block parent's top or bottom, or between adjacent block-level
// siblings, inside block containers that do not establish a new block
// formatting context. inFlowBlockLevel says whether n itself is an in-flow
// child of a block container.
func hasCollapsibleMargins(n *testNode, inFlowBlockLevel bool) bool {
	st := &n.style
	nonZero := func(d Dim) bool { return d.Unit != UnitInitial && d.Value != 0 }
	isBlockContainer := st.Display == DisplayBlock
	// A block box in flow whose margins can reach its children: no BFC of
	// its own (not a scroll container, not absolutely positioned, no
	// align-content).
	parentCollapses := isBlockContainer && inFlowBlockLevel && !st.isScrollContainer() && st.AlignContent == AlignNormal
	if isBlockContainer {
		var inFlow []*testNode
		for _, c := range n.children {
			if c.style.Display != DisplayNone && c.style.Position != PositionAbsolute {
				inFlow = append(inFlow, c)
			}
		}
		for i, c := range inFlow {
			if parentCollapses && (nonZero(c.style.Margin.Top) || nonZero(c.style.Margin.Bottom)) {
				return true
			}
			if i > 0 && nonZero(inFlow[i-1].style.Margin.Bottom) && nonZero(c.style.Margin.Top) {
				return true
			}
			// An empty box lets its own margins, or its neighbours',
			// collapse through it.
			if len(c.children) == 0 && c.measure == nil && (nonZero(c.style.Margin.Top) || nonZero(c.style.Margin.Bottom)) {
				return true
			}
		}
	}
	for _, c := range n.children {
		childInFlow := isBlockContainer && c.style.Position != PositionAbsolute
		if hasCollapsibleMargins(c, childInFlow) {
			return true
		}
	}
	return false
}
