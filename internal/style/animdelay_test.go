package style

import (
	"strings"
	"testing"
	"time"
)

// The animation-delay vocabulary (ADR-248): delay-* (tw-animate-css),
// animation-delay-* (tailwindcss-animation-delay) and [animation-delay:...].

func TestAnimationDelayClasses(t *testing.T) {
	for _, c := range []struct {
		class string
		want  time.Duration
	}{
		{"delay-150", 150 * time.Millisecond},
		{"delay-[0.3s]", 300 * time.Millisecond},
		{"animation-delay-300", 300 * time.Millisecond},
		{"-animation-delay-300", -300 * time.Millisecond},
		{"animation-delay-[1.5s]", 1500 * time.Millisecond},
		{"animation-delay-[-200ms]", -200 * time.Millisecond},
		{"[animation-delay:150ms]", 150 * time.Millisecond},
		{"[animation-delay:-0.3s]", -300 * time.Millisecond},
		{"[animation-delay:0]", 0},
		{"animation-delay-0", 0},
	} {
		s := style(t, c.class)
		if d := s.AnimationDelay - c.want; !s.AnimationDelaySet || d > time.Microsecond || d < -time.Microsecond {
			t.Errorf("%s: delay %v (set %v), want %v", c.class, s.AnimationDelay, s.AnimationDelaySet, c.want)
		}
	}
}

func TestDelayAlsoSetsTheTransitionDelay(t *testing.T) {
	s := style(t, "transition delay-200")
	if s.Transition.Delay != 200*time.Millisecond || s.AnimationDelay != 200*time.Millisecond {
		t.Errorf("transition delay %v, animation delay %v", s.Transition.Delay, s.AnimationDelay)
	}
	// animation-delay-* leaves the transition alone.
	s = style(t, "transition animation-delay-200")
	if s.Transition.Delay != 0 || s.AnimationDelay != 200*time.Millisecond {
		t.Errorf("transition delay %v, animation delay %v", s.Transition.Delay, s.AnimationDelay)
	}
}

func TestAnimationDelayLastWins(t *testing.T) {
	if d := style(t, "delay-100 animation-delay-300").AnimationDelay; d != 300*time.Millisecond {
		t.Errorf("last class: %v", d)
	}
	if d := style(t, "[animation-delay:2s] delay-100").AnimationDelay; d != 100*time.Millisecond {
		t.Errorf("last class: %v", d)
	}
	// Variants apply as for every property.
	c := Compile("[animation-delay:2s] hover:[animation-delay:1s]")
	if c.Err != nil {
		t.Fatal(c.Err)
	}
}

func TestAnimationDelayErrors(t *testing.T) {
	for _, c := range []struct{ class, want string }{
		{"[animation-delay:fast]", "use a time"},
		{"[animation-delay:150]", "use a time"},
		{"[animation-delay:150ms,1s]", "use a time"},
		{"animation-delay-[soon]", "not a time"},
		{"[animation-play-state:paused]", "[animation-delay:...]"},
	} {
		err := Compile(c.class).Err
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one with %q", c.class, err, c.want)
		}
	}
}

func TestAnimationDelaySuggestions(t *testing.T) {
	p := DefaultParser()
	for typo, want := range map[string]string{
		"animation-dealy-300": "animation-delay-300",
		"animtion-delay-300":  "animation-delay-300",
	} {
		if got := p.Suggest(typo); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", typo, got, want)
		}
	}
	v := p.Vocabulary()
	for _, c := range []string{"animation-delay-150", "-animation-delay-150", "delay-150"} {
		found := false
		for _, x := range v {
			found = found || x == c
		}
		if !found {
			t.Errorf("vocabulary lacks %q", c)
		}
	}
}
