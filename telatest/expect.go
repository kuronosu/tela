package telatest

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// The expectations wait, as Playwright's expect does: they settle the app
// and check, and while the check fails they let a frame of app time pass
// and check again, until it holds or the timeout expires (then the test
// fails with what the window shows).

// ExpectText waits until the window shows text: it is contained in the
// window's text content (whitespace normalized, hidden elements left out,
// block boundaries counting as spaces; case sensitive).
//
//	app.ExpectText("Count: 1")
func (a *App) ExpectText(text string) {
	a.tb.Helper()
	a.mustBeOpen()
	want := normalize(text)
	if want == "" {
		a.fatalf("ExpectText with an empty text is always true; pass the text to look for")
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		page := a.pageText()
		if strings.Contains(page, want) {
			return true, ""
		}
		msg := fmt.Sprintf("expected the window to show the text %q", want)
		if sug := closest(want, a.textCandidates()); len(sug) > 0 {
			msg += fmt.Sprintf("; did you mean %s?", quoteAll(sug))
		}
		return false, msg
	})
}

// ExpectNoText waits until the window does not show text (see
// [App.ExpectText]).
func (a *App) ExpectNoText(text string) {
	a.tb.Helper()
	a.mustBeOpen()
	want := normalize(text)
	if want == "" {
		a.fatalf("ExpectNoText with an empty text is never true; pass the text that must be gone")
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		if !strings.Contains(a.pageText(), want) {
			return true, ""
		}
		return false, fmt.Sprintf("expected the window not to show the text %q, but it does", want)
	})
}

// textCandidates returns the texts of the leaf elements, for suggestions.
func (a *App) textCandidates() []string {
	tc := newTexts()
	var out []string
	a.tree.Semantics().Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateHidden) {
			return false
		}
		if n.Kind == semantics.KindElement {
			leaf := true
			for _, c := range n.Children {
				if c.Kind == semantics.KindElement {
					leaf = false
				}
			}
			if t := tc.norm(n); leaf && t != "" && len(t) <= 80 {
				out = append(out, t)
			}
		}
		return true
	})
	return out
}

// ExpectRole waits until an element with the role and accessible name is
// visible (see [ByRole]; an empty name accepts any). It passes when one or
// more elements match: use [App.ExpectCount] to assert there is exactly one.
//
//	app.ExpectRole("heading", "Settings")
func (a *App) ExpectRole(role, name string) {
	a.tb.Helper()
	l := ByRole(role, name)
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	a.mustBeOpen()
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		if len(a.find(l)) > 0 {
			return true, ""
		}
		return false, a.missMessage(l)
	})
}

// ExpectCount waits until l matches exactly n elements.
func (a *App) ExpectCount(l Locator, n int) {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		got := len(a.find(l))
		if got == n {
			return true, ""
		}
		msg := fmt.Sprintf("expected %s to match %d element(s), it matches %d", l, n, got)
		if got == 0 {
			msg += "\n" + a.missMessage(l)
		}
		return false, msg
	})
}

// ExpectFocused waits until the element l locates has the keyboard focus.
func (a *App) ExpectFocused(l Locator) {
	a.tb.Helper()
	a.mustBeOpen()
	if p := l.problem(); p != "" {
		a.fatalf("%s", p)
	}
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		nodes := a.find(l)
		switch len(nodes) {
		case 0:
			return false, a.missMessage(l)
		case 1:
			if nodes[0].Has(semantics.StateFocused) {
				return true, ""
			}
			focused := "nothing"
			a.tree.Semantics().Walk(func(n *semantics.Node) bool {
				if n.Has(semantics.StateFocused) {
					focused = a.describeMatch(n)
				}
				return true
			})
			return false, fmt.Sprintf("expected %s to be focused, but %s is", describeNode(nodes[0]), focused)
		}
		a.fatalf("%s", a.strictMessage(l, nodes))
		return false, ""
	})
}

// ExpectAria waits until the window's aria snapshot equals yaml (see
// [App.AriaSnapshot]), like Playwright's toMatchAriaSnapshot in exact
// mode: after trimming the surrounding blank lines and removing the
// indentation common to all lines, so a raw string can be indented like
// the code around it.
//
//	app.ExpectAria(`
//	    - heading "Count: 1" [level=1]
//	    - button "Increment"
//	`)
func (a *App) ExpectAria(yaml string) {
	a.tb.Helper()
	a.mustBeOpen()
	want := dedent(yaml)
	a.waitFor(func() (bool, string) {
		a.tb.Helper()
		got := semantics.Snapshot(a.tree.Semantics())
		if got == want {
			return true, ""
		}
		return false, "the aria snapshot differs from the expected one.\n" + lineDiff(want, got)
	})
}

// dedent trims blank lines at both ends and the indentation common to the
// other lines.
func dedent(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	indent := -1
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " \t"))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	for i, l := range lines {
		if len(l) >= indent && indent > 0 {
			lines[i] = strings.TrimRight(l[indent:], " \t")
		} else {
			lines[i] = strings.TrimRight(l, " \t")
		}
	}
	return strings.Join(lines, "\n")
}

// lineDiff reports where two multi-line texts differ, with both in full.
func lineDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	i := 0
	for i < len(w) && i < len(g) && w[i] == g[i] {
		i++
	}
	var b strings.Builder
	switch {
	case i < len(w) && i < len(g):
		fmt.Fprintf(&b, "first difference at line %d:\n  expected: %s\n  actual:   %s\n", i+1, w[i], g[i])
	case i < len(w):
		fmt.Fprintf(&b, "the window's snapshot ends at line %d; expected more:\n  %s\n", i, w[i])
	default:
		fmt.Fprintf(&b, "the window's snapshot has more lines after line %d:\n  %s\n", i, g[i])
	}
	b.WriteString("expected:\n  " + strings.ReplaceAll(want, "\n", "\n  "))
	b.WriteString("\nactual:\n  " + strings.ReplaceAll(got, "\n", "\n  "))
	return b.String()
}

// ---------------------------------------------------------------------
// Goldens.

// GoldenOption configures [App.Golden].
type GoldenOption struct{ opts []golden.Option }

// Tolerance lets a golden comparison accept small differences: pixels
// whose channels differ by at most maxDelta (0 to 255, on premultiplied
// values) count as equal, and up to maxPixels pixels may differ by more.
// Without it a golden must match bit by bit: Tela's pixels are the same on
// every platform (ADR-027), so only a deliberate change should alter one.
func Tolerance(maxDelta uint8, maxPixels int) GoldenOption {
	return GoldenOption{[]golden.Option{golden.MaxDelta(maxDelta), golden.MaxPixels(maxPixels)}}
}

// Golden compares the window's content, after settling the app, with the
// PNG testdata/<name> next to the test (a name without the ".png" extension
// gets it), bit by bit unless a [Tolerance] is given. TELA_UPDATE=1 writes
// the golden instead of comparing (ADR-022); review the PNG before
// committing it. On a mismatch, <name>_actual.png and <name>_diff.png are
// written next to the golden.
//
// The picture is physical pixels, so a golden depends on [Size] and
// [Scale]; it also shows the overlay scrollbars while they are visible:
// advance the clock to hide them first when that matters.
func (a *App) Golden(name string, opts ...GoldenOption) {
	a.tb.Helper()
	var gopts []golden.Option
	for _, o := range opts {
		gopts = append(gopts, o.opts...)
	}
	golden.Image(a.tb, name, a.Screenshot(), gopts...)
}

// AriaGolden compares the app's aria snapshot, with a final newline (what
// `tela inspect` prints), with the text file testdata/<name>
// (TELA_UPDATE=1 writes it). Use it for snapshots too long to write in the
// test; [App.ExpectAria] is better for short ones.
func (a *App) AriaGolden(name string) {
	a.tb.Helper()
	snap := a.AriaSnapshot()
	if snap != "" {
		snap += "\n"
	}
	golden.Text(a.tb, name, snap)
}
