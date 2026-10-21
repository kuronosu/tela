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

func TestAriaStatesOfProps(t *testing.T) {
	a := &ariaProps{}
	for _, p := range []Prop{AriaBusy(true), AriaChecked(true), AriaDisabled(false), AriaExpanded(true), AriaHidden(true),
		AriaPressed(true), AriaReadOnly(true), AriaRequired(true), AriaSelected(false)} {
		p.apply(&desc{aria: a})
	}
	const want = style.StateAriaBusy | style.StateAriaChecked | style.StateAriaExpanded | style.StateAriaHidden | style.StateAriaPressed | style.StateAriaReadOnly | style.StateAriaRequired
	if got := ariaStates(a); got != want {
		t.Errorf("ariaStates = %b, want %b", got, want)
	}
	// A mixed value is not "true".
	a.checked = semantics.ToggleMixed
	if got := ariaStates(a); got&style.StateAriaChecked != 0 {
		t.Errorf("a mixed checkbox matches aria-checked: %b", got)
	}
	if ariaStates(nil) != 0 {
		t.Error("nil props have states")
	}
}

// Class errors of the new variants name the closest variant.
func TestNewVariantClassErrors(t *testing.T) {
	for class, want := range map[string]string{
		"max-smm:w-4":               `did you mean "max-sm:"?`,
		"aria-chekced:w-4":          `did you mean "aria-checked:"?`,
		"aria-[sort=ascending]:w-4": "Tela has no prop for the attribute selector [sort=ascending]",
		"min-[wide]:w-4":            "the width must be a length in px, rem or em",
	} {
		var logs []string
		tr := newTestTree(t, 100, 100, Component(func() Node { return div(Class(class)) }), &logs)
		tr.Frame()
		if len(logs) != 1 || !strings.Contains(logs[0], want) {
			t.Errorf("Class(%q): logs %q, want one containing %q", class, logs, want)
		}
	}
}
