package style

import (
	"math/bits"
	"testing"
)

// TestAriaTable pins the aria table: each aria state flag is in exactly
// one row, the rows cover the whole run of flags from StateAriaBusy to
// StateAriaCurrentTrue, and the derived hint and vocabularies are the text
// the messages always had.
func TestAriaTable(t *testing.T) {
	var seen StateFlags
	for _, r := range ariaTable {
		if r.State&seen != 0 {
			t.Errorf("%s %q: flag %b is in two rows", r.Attr, r.Token, r.State)
		}
		if bits.OnesCount64(uint64(r.State)) > 1 {
			t.Errorf("%s %q: more than one flag", r.Attr, r.Token)
		}
		if r.Token == "" && r.State == 0 {
			t.Errorf("%s: a boolean attribute without a flag", r.Attr)
		}
		if r.Unset && r.State != 0 {
			t.Errorf("%s %q: an unset token cannot have a variant", r.Attr, r.Token)
		}
		seen |= r.State
	}
	run := StateAriaCurrentTrue<<1 - StateAriaBusy
	if seen != run || AriaStates != run {
		t.Errorf("table flags %b, AriaStates %b; want the run %b from StateAriaBusy to StateAriaCurrentTrue", seen, AriaStates, run)
	}
	if got := AriaTokens("sort"); len(got) != 4 || got[0] != "ascending" || got[3] != "none" {
		t.Errorf("AriaTokens(sort) = %v", got)
	}
	if got := AriaTokens("current"); len(got) != 7 || got[0] != "page" || got[6] != "false" {
		t.Errorf("AriaTokens(current) = %v", got)
	}
	if AriaTokens("busy") != nil || AriaTokenState("busy", "") != 0 || AriaBoolState("sort") != 0 {
		t.Error("a boolean attribute has no tokens, a token attribute no boolean state")
	}
	if AriaBoolState("readonly") != StateAriaReadOnly || AriaTokenState("sort", "other") != 0 ||
		!AriaTokenUnset("sort", "none") || !AriaTokenUnset("current", "false") || AriaTokenUnset("sort", "other") {
		t.Error("AriaBoolState, AriaTokenState or AriaTokenUnset is wrong")
	}
	const wantHint = "the aria variants test the props AriaBusy, AriaChecked, AriaDisabled, AriaExpanded, AriaHidden, AriaInvalid, AriaPressed, AriaReadOnly, AriaRequired and AriaSelected when they are true: aria-busy:, aria-checked:, aria-disabled:, aria-expanded:, aria-hidden:, aria-invalid:, aria-pressed:, aria-readonly:, aria-required:, aria-selected:, AriaSort with aria-[sort=ascending]: and aria-[sort=descending]:, and AriaCurrent with aria-[current=page]:, aria-[current=step]:, aria-[current=location]:, aria-[current=date]:, aria-[current=time]: and aria-[current=true]: (also group-aria-checked: and peer-aria-[sort=ascending]:)"
	if ariaVariantHint != wantHint {
		t.Errorf("ariaVariantHint = %q", ariaVariantHint)
	}
}
