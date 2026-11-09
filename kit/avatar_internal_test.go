package kit

import "testing"

// An AvatarAuto avatar does not list and sort the recipe's AutoColors on
// every build: the sorted names are kept per recipe map.
func TestAvatarAutoColorAllocatesNothing(t *testing.T) {
	s := DefaultStyle().Avatar
	a := Avatar{Name: "mei tanaka", Color: AvatarAuto}
	want := a.colorClass(&s)
	if want == "" {
		t.Fatal("no color for a name")
	}
	if n := testing.AllocsPerRun(100, func() { a.colorClass(&s) }); n != 0 {
		t.Errorf("colorClass allocates %v times per call, want 0", n)
	}
	if got := a.colorClass(&s); got != want {
		t.Errorf("colorClass = %q, then %q", want, got)
	}
}

// A recipe edited in place is seen: the color a name had leaves the
// AutoColors, and the name gets another one of those left.
func TestAvatarAutoColorFollowsTheRecipe(t *testing.T) {
	s := DefaultStyle().Avatar
	a := Avatar{Name: "Mei Tanaka", Color: AvatarAuto}
	before := a.colorClass(&s)
	for c, on := range s.AutoColors {
		if on && s.Colors[c] == before {
			s.AutoColors[c] = false
		}
	}
	after := a.colorClass(&s)
	if after == "" || after == before {
		t.Errorf("color %q before, %q after the recipe dropped it", before, after)
	}
	for c := range s.AutoColors {
		delete(s.AutoColors, c)
	}
	if got := a.colorClass(&s); got != "" {
		t.Errorf("color %q with no AutoColors, want none", got)
	}
}
