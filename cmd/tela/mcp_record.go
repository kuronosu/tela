package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// The recording of tela mcp's session: the acting calls the agent made, in
// order, with the aria snapshot before and after each, so that the test
// tool (mcp_testgen.go) can write them as a telatest test. A mount of the
// screen starts a segment; the steps after it belong to it. A rebuild of
// the source, or a crash, forgets the segments: the steps no longer say
// what the screen does.

// mcpStep is an acting call of the session.
type mcpStep struct {
	req mcpReq
	// before and after are the plain aria snapshots (no refs, no source
	// lines) around the call.
	before, after string
	// w and h are the window's logical size after the call.
	w, h float64
	// note is a remark the test carries as a comment ("the locale was set
	// to ar in the session; the test does not reproduce it").
	note string
	// lines are the Go lines of a step that has no request of its own
	// (the preferences set live).
	lines []string
}

// mcpSegment is a mount of the screen and what was done to it.
type mcpSegment struct {
	mount   mcpMount
	preview string // the preview that was mounted, "" for none
	steps   []mcpStep
}

// mcpRecorder is the session's recording.
type mcpRecorder struct{ segs []*mcpSegment }

// reset forgets everything: the screen was mounted afresh for a reason the
// steps cannot follow.
func (r *mcpRecorder) reset(m mcpMount) { r.segs = []*mcpSegment{{mount: m}} }

// newSegment starts a mount that follows the steps so far (open).
func (r *mcpRecorder) newSegment(m mcpMount, preview string) {
	if n := len(r.segs); n > 0 && len(r.segs[n-1].steps) == 0 {
		r.segs[n-1].mount, r.segs[n-1].preview = m, preview
		return
	}
	r.segs = append(r.segs, &mcpSegment{mount: m, preview: preview})
}

// add appends a step to the current segment.
func (r *mcpRecorder) add(st mcpStep, preview string) {
	if len(r.segs) == 0 {
		return
	}
	seg := r.segs[len(r.segs)-1]
	if seg.preview == "" {
		seg.preview = preview
	}
	seg.steps = append(seg.steps, st)
}

// count is the number of steps recorded.
func (r *mcpRecorder) count() int {
	n := 0
	for _, s := range r.segs {
		n += len(s.steps)
	}
	return n
}

// plainSnapshot returns yaml without the source marks of its element lines:
// the form telatest's ExpectAria compares against.
func plainSnapshot(yaml string) string {
	if !strings.Contains(yaml, "[src=") {
		return yaml
	}
	lines := strings.Split(yaml, "\n")
	for i, s := range lines {
		if l, ok := semantics.ParseSnapshotLine(s); ok && l.Src != "" {
			l.Src = ""
			lines[i] = l.WithSuffix("")
		}
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------------
// Locators.

// goExpr is the telatest locator the element of the step stands for, as Go
// source: the stable form of a ref (its role and name, with the Nth that
// tells it from the others like it), or the locator the agent gave. Never
// a node identity, which means something only in the instance that is
// running.
func (l *mcpLoc) goExpr() string {
	if l.Stable != nil {
		return l.Stable.goExpr()
	}
	var b strings.Builder
	switch {
	case l.TestID != "":
		fmt.Fprintf(&b, "telatest.ByTestID(%s)", strconv.Quote(l.TestID))
	case l.Role != "":
		fmt.Fprintf(&b, "telatest.ByRole(%s, %s)", strconv.Quote(l.Role), strconv.Quote(l.Name))
	case l.Exact:
		fmt.Fprintf(&b, "telatest.ByText(%s, telatest.Exact())", strconv.Quote(l.Text))
	default:
		fmt.Fprintf(&b, "telatest.ByText(%s)", strconv.Quote(l.Text))
	}
	if l.Nth != nil {
		fmt.Fprintf(&b, ".Nth(%d)", *l.Nth)
	}
	return b.String()
}

// ---------------------------------------------------------------------
// Expectations: what the snapshot showed after a step, as checks.

// snapLine is a line of an aria snapshot, parsed.
type snapLine struct {
	raw      string // the line without its indentation
	element  bool
	role     string
	name     string
	hasName  bool
	states   []string // the states of the key the test can check
	value    string   // the text after the key, or the text of a "text" line
	hasValue bool
	isText   bool
}

// checkableStates are the states of a snapshot key that telatest checks
// with a State, mapped to its constructor.
var checkableStates = map[string]string{
	"checked": "Checked", "disabled": "Disabled", "expanded": "Expanded",
	"invalid": "Invalid", "pressed": "Pressed", "selected": "Selected",
}

// unquoteValue reads the value after a key of a snapshot line.
func unquoteValue(s string) string {
	if strings.HasPrefix(s, `"`) {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}

func parseSnapshotLines(yaml string) []snapLine {
	var out []snapLine
	for _, line := range strings.Split(yaml, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		sl := snapLine{raw: trimmed}
		if v, ok := strings.CutPrefix(trimmed, "- text: "); ok {
			sl.isText, sl.value, sl.hasValue = true, unquoteValue(v), true
		} else if l, ok := semantics.ParseSnapshotLine(line); ok {
			sl.element, sl.role, sl.name, sl.hasName = true, l.Role, l.Name, l.HasName
			for _, tok := range strings.Fields(strings.NewReplacer("[", " ", "]", " ").Replace(l.States)) {
				if _, ok := checkableStates[tok]; ok {
					sl.states = append(sl.states, tok)
				}
			}
			if v, ok := strings.CutPrefix(l.Rest, ": "); ok {
				sl.value, sl.hasValue = unquoteValue(v), true
			}
		}
		out = append(out, sl)
	}
	return out
}

// maxExpectations is the most checks a step gets: a step that changes the
// whole screen is checked by the lines that matter most.
const maxExpectations = 8

// expectations returns the Go lines that check what changed between the
// snapshots before and after a step: the elements and texts that appeared,
// the states that flipped, the values a field took, and what went away. It
// checks the changes and not the whole snapshot, so the test breaks when
// the behavior does and not when the layout around it is edited.
func expectations(before, after string) []string {
	b, a := parseSnapshotLines(before), parseSnapshotLines(after)
	if before == after {
		return nil
	}
	// The lines that are in one snapshot and not in the other.
	left := map[string]int{}
	for _, l := range b {
		left[l.raw]++
	}
	var added []snapLine
	for _, l := range a {
		if left[l.raw] > 0 {
			left[l.raw]--
			continue
		}
		added = append(added, l)
	}
	var removed []snapLine
	right := map[string]int{}
	for _, l := range a {
		right[l.raw]++
	}
	for _, l := range b {
		if right[l.raw] > 0 {
			right[l.raw]--
			continue
		}
		removed = append(removed, l)
	}
	// count is how many elements of the after snapshot have the role and
	// name.
	count := func(role, name string) (n int) {
		for _, l := range a {
			if l.element && l.hasName && l.role == role && l.name == name {
				n++
			}
		}
		return n
	}
	// loc is the locator of a named element of the after snapshot; with an
	// Nth when several elements share its role and name.
	loc := func(target snapLine) string {
		expr := fmt.Sprintf("telatest.ByRole(%s, %s)", strconv.Quote(target.role), strconv.Quote(target.name))
		if count(target.role, target.name) <= 1 {
			return expr
		}
		idx := 0
		for _, l := range a {
			if l.element && l.hasName && l.role == target.role && l.name == target.name {
				if l.raw == target.raw {
					break
				}
				idx++
			}
		}
		return fmt.Sprintf("%s.Nth(%d)", expr, idx)
	}
	stateArgs := func(states []string, on bool) []string {
		var out []string
		for _, s := range states {
			out = append(out, fmt.Sprintf("telatest.%s(%v)", checkableStates[s], on))
		}
		return out
	}
	var out []string
	emit := func(line string) {
		if !slices.Contains(out, line) {
			out = append(out, line)
		}
	}
	usedRemoved := make([]bool, len(removed))
	renamedRoles := map[string]bool{}
	for _, l := range added {
		switch {
		case l.element && l.hasName:
			// The same element with other states or another value?
			prev := -1
			for i, r := range removed {
				if !usedRemoved[i] && r.element && r.hasName && r.role == l.role && r.name == l.name {
					prev = i
					break
				}
			}
			if prev >= 0 {
				usedRemoved[prev] = true
				r := removed[prev]
				var checks []string
				checks = append(checks, stateArgs(missing(l.states, r.states), true)...)
				checks = append(checks, stateArgs(missing(r.states, l.states), false)...)
				if len(checks) > 0 {
					emit(fmt.Sprintf("app.Expect(%s, %s)", loc(l), strings.Join(checks, ", ")))
				}
				if l.hasValue && l.value != r.value && fieldRole(l.role) {
					emit(fmt.Sprintf("app.ExpectValue(%s, %s)", loc(l), strconv.Quote(l.value)))
				}
				continue
			}
			renamedRoles[l.role] = true
			if len(l.states) > 0 {
				emit(fmt.Sprintf("app.Expect(%s, %s)", loc(l), strings.Join(stateArgs(l.states, true), ", ")))
			} else {
				emit(fmt.Sprintf("app.ExpectRole(%s, %s)", strconv.Quote(l.role), strconv.Quote(l.name)))
			}
			if l.hasValue && fieldRole(l.role) {
				emit(fmt.Sprintf("app.ExpectValue(%s, %s)", loc(l), strconv.Quote(l.value)))
			}
		case l.hasValue && l.value != "":
			emit(fmt.Sprintf("app.ExpectText(%s)", strconv.Quote(l.value)))
		}
	}
	// What went away: an element no longer there (and not renamed into
	// another of its role), a text no longer shown.
	var texts []string
	for _, l := range a {
		texts = append(texts, l.value)
		if l.hasName {
			texts = append(texts, l.name)
		}
	}
	shown := strings.Join(texts, "\n")
	for i, r := range removed {
		if usedRemoved[i] {
			continue
		}
		switch {
		case r.element && r.hasName:
			if count(r.role, r.name) == 0 && !renamedRoles[r.role] {
				emit(fmt.Sprintf("app.ExpectCount(telatest.ByRole(%s, %s), 0)", strconv.Quote(r.role), strconv.Quote(r.name)))
			}
		case r.hasValue && r.value != "" && !strings.Contains(shown, r.value):
			emit(fmt.Sprintf("app.ExpectNoText(%s)", strconv.Quote(r.value)))
		}
	}
	if len(out) > maxExpectations {
		more := len(out) - maxExpectations
		out = append(out[:maxExpectations], fmt.Sprintf("// ... and %d more changes in the snapshot", more))
	}
	return out
}

// missing returns the states of a that b lacks.
func missing(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}

// fieldRole reports whether elements of role are text fields, which
// ExpectValue reads.
func fieldRole(role string) bool { return role == "textbox" || role == "searchbox" }

// preferLines are the Go lines that put a test's app in the preferences cur
// from prev, as the prefer tool did live: dark mode, reduced motion, high
// contrast and the text scale. A locale or a direction has no line (the
// mount takes them, or the test cannot), and comes back as a comment.
func preferLines(prev, cur mcpMount) []string {
	var out []string
	if prev.Dark != cur.Dark {
		out = append(out, fmt.Sprintf("app.SetDarkMode(%v)", cur.Dark))
	}
	if prev.Prefer.ReducedMotion != cur.Prefer.ReducedMotion {
		out = append(out, fmt.Sprintf("app.SetReducedMotion(%v)", cur.Prefer.ReducedMotion))
	}
	if prev.Prefer.HighContrast != cur.Prefer.HighContrast {
		out = append(out, fmt.Sprintf("app.SetHighContrast(%v)", cur.Prefer.HighContrast))
	}
	if p, c := textScaleOf(prev.Prefer), textScaleOf(cur.Prefer); p != c {
		out = append(out, fmt.Sprintf("app.SetTextScale(%g)", c))
	}
	if prev.Prefer.Locale != cur.Prefer.Locale || prev.Prefer.LocaleSet != cur.Prefer.LocaleSet {
		out = append(out, fmt.Sprintf("// the session set the locale to %q; the test does not reproduce it", cur.Prefer.Locale))
	}
	return out
}

// textScaleOf is the text scale of a preference set (0 means 1).
func textScaleOf(p mcpPrefer) float64 {
	if p.TextScale <= 0 {
		return 1
	}
	return p.TextScale
}
