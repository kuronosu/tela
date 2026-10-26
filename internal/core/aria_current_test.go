package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// AriaCurrent (ADR-171): the node's Current, tokens ASCII case-insensitive,
// "false" and "" the same as unset; the aria snapshot does not print it
// (Playwright 1.61.1's does not).
func TestAriaCurrent(t *testing.T) {
	tr, snap := snapshotOf(t, Component(func() Node {
		return div(
			div(Role("link"), AriaCurrent("page"), Text("A")),
			div(Role("link"), AriaCurrent("STEP"), Text("B")),
			div(Role("link"), AriaCurrent(" location "), Text("C")),
			div(Role("link"), AriaCurrent("date"), Text("D")),
			div(Role("link"), AriaCurrent("time"), Text("E")),
			div(Role("link"), AriaCurrent("true"), Text("F")),
			div(Role("link"), AriaCurrent("false"), Text("G")),
			div(Role("link"), AriaCurrent(""), Text("H")),
			div(Role("link"), Text("I")))
	}))
	for name, want := range map[string]string{
		"A": "page", "B": "step", "C": "location", "D": "date", "E": "time", "F": "true", "G": "", "H": "", "I": "",
	} {
		if n := mustNode(t, tr, "link", name); n.Current != want {
			t.Errorf("%s: Current %q, want %q", name, n.Current, want)
		}
	}
	if strings.Contains(snap, "current") {
		t.Errorf("the snapshot prints aria-current:\n%s", snap)
	}
}

func TestAriaCurrentInvalidToken(t *testing.T) {
	devTree(t, div(), nil) // dev mode on
	msg := panicMessage(func() { AriaCurrent("pge") })
	if !strings.Contains(msg, `AriaCurrent("pge"): unknown value; did you mean "page"?`) ||
		!strings.Contains(msg, `"page", "step", "location", "date", "time", "true", "false"`) {
		t.Errorf("panic %q", msg)
	}
	if msg := panicMessage(func() { AriaCurrent("") }); msg != "" {
		t.Errorf("AriaCurrent(\"\") panics: %q", msg)
	}
	// Outside dev mode an unknown token is ignored.
	devAny.Store(false)
	defer devAny.Store(true)
	tr := newTestTree(t, 100, 100, div(Role("link"), AriaCurrent("upward"), Text("X")), nil)
	tr.Frame()
	if n := mustNode(t, tr, "link", "X"); n.Current != "" {
		t.Errorf("an invalid AriaCurrent outside dev mode is not ignored: %q", n.Current)
	}
}

// Each token has its own state flag, for aria-[current=…]: and its group-
// and peer- forms; "false" and unset have none.
func TestAriaCurrentStates(t *testing.T) {
	for token, want := range map[string]style.StateFlags{
		"page": style.StateAriaCurrentPage, "step": style.StateAriaCurrentStep, "location": style.StateAriaCurrentLocation,
		"date": style.StateAriaCurrentDate, "time": style.StateAriaCurrentTime, "true": style.StateAriaCurrentTrue,
		"false": 0, "": 0,
	} {
		a := &ariaProps{}
		AriaCurrent(token).apply(&desc{aria: a})
		if got := ariaStates(a); got != want {
			t.Errorf("AriaCurrent(%q): states %b, want %b", token, got, want)
		}
	}
	if style.AriaStates&style.StateAriaCurrentTrue == 0 {
		t.Error("the current flags are not in style.AriaStates, so group- and peer- would not see them")
	}
}
