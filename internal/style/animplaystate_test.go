package style

import "testing"

// animation-play-state: paused and running (tw-animate-css) and the
// arbitrary property (ADR-263).

func TestAnimationPlayStateClasses(t *testing.T) {
	for _, c := range []struct {
		class  string
		paused bool
	}{
		{"paused", true},
		{"running", false},
		{"[animation-play-state:paused]", true},
		{"[animation-play-state:running]", false},
		{"[animation-play-state:initial]", false},
		{"paused running", false},
		{"running paused", true},
		{"[animation-play-state:running] paused", true},
	} {
		p := style(t, c.class).AnimProps
		if p.Set&AnimPlayState == 0 || p.Paused != c.paused {
			t.Errorf("%s: %+v, want paused %v", c.class, p, c.paused)
		}
	}
	if p := style(t, "animate-spin").AnimProps; p.Set&AnimPlayState != 0 {
		t.Errorf("animate-spin sets the play state: %+v", p)
	}
	if c := Compile("[animation-play-state:sideways]"); c.Err == nil {
		t.Error("[animation-play-state:sideways] is accepted")
	}
}
