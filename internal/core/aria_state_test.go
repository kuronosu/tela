package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// AriaRequired, AriaReadOnly and AriaBusy set states of the semantics node;
// Playwright's aria snapshot prints none of them.
func TestAriaRequiredReadOnlyBusyStates(t *testing.T) {
	tr, snap := snapshotOf(t, Component(func() Node {
		return div(
			div(Role("textbox"), AriaLabel("Full"), AriaRequired(true), AriaReadOnly(true), AriaBusy(true)),
			div(Role("textbox"), AriaLabel("Empty"), AriaRequired(false), AriaReadOnly(false), AriaBusy(false)),
			div(Role("textbox"), AriaLabel("Plain")))
	}))
	if want := "- textbox \"Full\"\n- textbox \"Empty\"\n- textbox \"Plain\""; snap != want {
		t.Errorf("snapshot %q, want %q", snap, want)
	}
	full := mustNode(t, tr, "textbox", "Full")
	for name, s := range map[string]semantics.State{"required": semantics.StateRequired, "readonly": semantics.StateReadOnly, "busy": semantics.StateBusy} {
		if !full.Has(s) {
			t.Errorf("Full: no %s state", name)
		}
	}
	for _, name := range []string{"Empty", "Plain"} {
		if n := mustNode(t, tr, "textbox", name); n.States&(semantics.StateRequired|semantics.StateReadOnly|semantics.StateBusy) != 0 {
			t.Errorf("%s has states %b", name, n.States)
		}
	}
}

// ADR-131: the invalid state is AriaInvalid when set, true or false, else
// a text field's own validity (Chromium's GetInvalidState); a read-only
// or disabled field is barred from validation. The snapshot prints
// [invalid] from the prop alone (TestAriaSnapshotCases checks it against
// Playwright).
func TestAriaInvalidState(t *testing.T) {
	tr, snap := snapshotOf(t, Component(func() Node {
		return div(
			div(Role("textbox"), AriaLabel("Prop"), AriaInvalid(true)),
			div(Role("textbox"), AriaLabel("False"), AriaInvalid(false)),
			div(Role("textbox"), AriaLabel("Plain")),
			input(FieldType("email"), AriaLabel("Bad mail"), DefaultValue("bad")),
			input(FieldType("email"), AriaLabel("Good mail"), DefaultValue("ada@example.com")),
			input(FieldType("email"), AriaLabel("Overridden"), DefaultValue("bad"), AriaInvalid(false)),
			input(FieldType("email"), AriaLabel("Read-only"), DefaultValue("bad"), ReadOnly(true)),
			input(FieldType("email"), AriaLabel("Disabled"), DefaultValue("bad"), Disabled(true)),
			input(FieldType("email"), AriaLabel("Both"), DefaultValue("ada@example.com"), AriaInvalid(true)),
		)
	}))
	for name, want := range map[string]bool{
		"Prop": true, "False": false, "Plain": false, "Bad mail": true, "Good mail": false,
		"Overridden": false, "Read-only": false, "Disabled": false, "Both": true,
	} {
		if n := mustNode(t, tr, "textbox", name); n.Has(semantics.StateInvalid) != want {
			t.Errorf("%s: invalid %v, want %v", name, !want, want)
		}
	}
	if n := mustNode(t, tr, "textbox", "Prop"); n.Invalid != semantics.ToggleTrue {
		t.Errorf("Prop: Invalid %v", n.Invalid)
	}
	if n := mustNode(t, tr, "textbox", "Bad mail"); n.Invalid != semantics.ToggleUnset {
		t.Errorf("Bad mail: Invalid %v, want unset (the field's validity is a state, not the prop)", n.Invalid)
	}
	for _, line := range []string{`- textbox "Prop" [invalid]`, `- textbox "Bad mail": bad`, `- textbox "Both" [invalid]: ada@example.com`} {
		if !strings.Contains(snap, line+"\n") && !strings.HasSuffix(snap, line) {
			t.Errorf("snapshot has no line %q:\n%s", line, snap)
		}
	}
}

// AriaSort: the node's Sort, its tokens case-insensitive, "none" the same
// as unset; unknown values panic in dev mode and are ignored otherwise.
func TestAriaSort(t *testing.T) {
	tr, snap := snapshotOf(t, Component(func() Node {
		return div(Role("row"),
			div(Role("columnheader"), AriaSort("ascending"), Text("A")),
			div(Role("columnheader"), AriaSort("DESCENDING"), Text("B")),
			div(Role("columnheader"), AriaSort(" other "), Text("C")),
			div(Role("columnheader"), AriaSort("none"), Text("D")),
			div(Role("columnheader"), Text("E")))
	}))
	for name, want := range map[string]string{"A": "ascending", "B": "descending", "C": "other", "D": "", "E": ""} {
		if n := mustNode(t, tr, "columnheader", name); n.Sort != want {
			t.Errorf("%s: Sort %q, want %q", name, n.Sort, want)
		}
	}
	if strings.Contains(snap, "sort") {
		t.Errorf("the snapshot prints the sort:\n%s", snap)
	}
	devTree(t, div(), nil) // dev mode on
	if msg := panicMessage(func() { AriaSort("acending") }); !strings.Contains(msg, `AriaSort("acending"): unknown value; did you mean "ascending"?`) ||
		!strings.Contains(msg, `"ascending", "descending", "other", "none"`) {
		t.Errorf("panic %q", msg)
	}
	devAny.Store(false)
	defer devAny.Store(true)
	tr = newTestTree(t, 100, 100, div(Role("columnheader"), AriaSort("upward"), Text("X")), nil)
	tr.Frame()
	if n := mustNode(t, tr, "columnheader", "X"); n.Sort != "" {
		t.Errorf("an invalid AriaSort outside dev mode is not ignored: %q", n.Sort)
	}
}

func TestAriaStatesOfProps(t *testing.T) {
	a := &ariaProps{}
	for _, p := range []Prop{AriaBusy(true), AriaChecked(true), AriaDisabled(false), AriaExpanded(true), AriaHidden(true),
		AriaInvalid(true), AriaPressed(true), AriaReadOnly(true), AriaRequired(true), AriaSelected(false), AriaSort("ascending")} {
		p.apply(&desc{aria: a})
	}
	const want = style.StateAriaBusy | style.StateAriaChecked | style.StateChecked | style.StateAriaExpanded | style.StateAriaHidden |
		style.StateAriaInvalid | style.StateAriaPressed | style.StateAriaReadOnly | style.StateAriaRequired | style.StateAriaSortAscending
	if got := ariaStates(a); got != want {
		t.Errorf("ariaStates = %b, want %b", got, want)
	}
	for sort, want := range map[string]style.StateFlags{"descending": style.StateAriaSortDescending, "other": 0, "none": 0} {
		b := &ariaProps{}
		AriaSort(sort).apply(&desc{aria: b})
		if got := ariaStates(b); got != want {
			t.Errorf("AriaSort(%q): states %b, want %b", sort, got, want)
		}
	}
	b := &ariaProps{}
	AriaInvalid(false).apply(&desc{aria: b})
	if ariaStates(b) != 0 {
		t.Error("AriaInvalid(false) matches aria-invalid:")
	}
	// A mixed value is not "true".
	a.checked = semantics.ToggleMixed
	if got := ariaStates(a); got&(style.StateAriaChecked|style.StateChecked) != 0 {
		t.Errorf("a mixed checkbox matches aria-checked: %b", got)
	}
	if ariaStates(nil) != 0 {
		t.Error("nil props have states")
	}
}

// Class errors of the new variants name the closest variant.
func TestNewVariantClassErrors(t *testing.T) {
	for class, want := range map[string]string{
		"max-smm:w-4":           `did you mean "max-sm:"?`,
		"aria-chekced:w-4":      `did you mean "aria-checked:"?`,
		"aria-[sort=other]:w-4": "Tela has no prop for the attribute selector [sort=other]",
		"min-[wide]:w-4":        "the width must be a length in px, rem or em",
	} {
		var logs []string
		tr := newTestTree(t, 100, 100, Component(func() Node { return div(Class(class)) }), &logs)
		tr.Frame()
		if len(logs) != 1 || !strings.Contains(logs[0], want) {
			t.Errorf("Class(%q): logs %q, want one containing %q", class, logs, want)
		}
	}
}
