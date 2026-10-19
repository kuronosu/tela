package style

import "testing"

// TestOwnValueFlags: the flags transitions use to tell a value of the
// element's own from one that follows its parent.
func TestOwnValueFlags(t *testing.T) {
	parent := style(t, "text-red-500 underline text-lg")
	for _, c := range []struct {
		classes             string
		fontSet             bool
		current             TransitionProps
		decorationInherited bool
	}{
		{"", false, TransitionBorderColor | TransitionOutlineColor, true},
		{"text-sm", true, TransitionBorderColor | TransitionOutlineColor, true},
		{"text-[20px]", true, TransitionBorderColor | TransitionOutlineColor, true},
		{"text-[1.5em]", false, TransitionBorderColor | TransitionOutlineColor, true},
		{"text-[150%]", false, TransitionBorderColor | TransitionOutlineColor, true},
		{"border-blue-500", false, TransitionOutlineColor, true},
		{"border-t-blue-500", false, TransitionOutlineColor, true},
		{"border-current outline-blue-500", false, TransitionBorderColor, true},
		{"underline", false, TransitionBorderColor | TransitionOutlineColor | TransitionDecorationColor, false},
		{"decoration-current", false, TransitionBorderColor | TransitionOutlineColor | TransitionDecorationColor, false},
		{"decoration-blue-500", false, TransitionBorderColor | TransitionOutlineColor, false},
	} {
		s := styleIn(t, &Context{}, KindDiv, parent, c.classes)
		if s.FontSizeSet != c.fontSet || s.CurrentColors != c.current || s.DecorationInherited != c.decorationInherited {
			t.Errorf("%q: FontSizeSet %v, CurrentColors %b, DecorationInherited %v; want %v, %b, %v", c.classes,
				s.FontSizeSet, s.CurrentColors, s.DecorationInherited, c.fontSet, c.current, c.decorationInherited)
		}
	}
	if s := styleIn(t, &Context{}, KindDiv, parent, "", FontSize(12)); !s.FontSizeSet {
		t.Error("the FontSize prop does not set FontSizeSet")
	}
}
