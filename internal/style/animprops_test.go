package style

import (
	"math"
	"strings"
	"testing"
	"time"
)

// The animation longhands of tw-animate-css and the arbitrary properties
// (ADR-256).

func TestAnimationPropsClasses(t *testing.T) {
	ms := time.Millisecond
	for _, c := range []struct {
		class string
		check func(p AnimationProps) bool
	}{
		{"duration-300", func(p AnimationProps) bool { return p.Set&AnimDuration != 0 && p.Duration == 300*ms }},
		{"duration-[1.5s]", func(p AnimationProps) bool { return p.Duration == 1500*ms }},
		{"animation-duration-500", func(p AnimationProps) bool { return p.Duration == 500*ms }},
		{"[animation-duration:2s]", func(p AnimationProps) bool { return p.Duration == 2000*ms }},
		{"[animation-duration:200ms]", func(p AnimationProps) bool { return p.Duration == 200*ms }},
		{"ease-in", func(p AnimationProps) bool { return p.Set&AnimTiming != 0 && p.Timing == (CubicBezier{0.4, 0, 1, 1}) }},
		{"ease-linear", func(p AnimationProps) bool { return p.Timing == (CubicBezier{0, 0, 1, 1}) }},
		{"ease-[cubic-bezier(0.1,0.2,0.3,0.4)]", func(p AnimationProps) bool { return p.Timing == (CubicBezier{0.1, 0.2, 0.3, 0.4}) }},
		{"[animation-timing-function:ease-in-out]", func(p AnimationProps) bool { return p.Timing == (CubicBezier{0.42, 0, 0.58, 1}) }},
		{"repeat-3", func(p AnimationProps) bool { return p.Set&AnimIterations != 0 && p.Iterations == 3 }},
		{"repeat-0", func(p AnimationProps) bool { return p.Set&AnimIterations != 0 && p.Iterations == 0 }},
		{"repeat-infinite", func(p AnimationProps) bool { return math.IsInf(p.Iterations, 1) }},
		{"repeat-initial", func(p AnimationProps) bool { return p.Iterations == 1 }},
		{"repeat-[2.5]", func(p AnimationProps) bool { return p.Iterations == 2.5 }},
		{"[animation-iteration-count:infinite]", func(p AnimationProps) bool { return math.IsInf(p.Iterations, 1) }},
		{"[animation-iteration-count:4]", func(p AnimationProps) bool { return p.Iterations == 4 }},
		{"direction-reverse", func(p AnimationProps) bool { return p.Set&AnimDirection != 0 && p.Direction == "reverse" }},
		{"direction-alternate-reverse", func(p AnimationProps) bool { return p.Direction == "alternate-reverse" }},
		{"direction-initial", func(p AnimationProps) bool { return p.Set&AnimDirection != 0 && p.Direction == "normal" }},
		{"[animation-direction:alternate]", func(p AnimationProps) bool { return p.Direction == "alternate" }},
		{"fill-mode-forwards", func(p AnimationProps) bool { return p.Set&AnimFillMode != 0 && p.Fill == "forwards" }},
		{"fill-mode-both", func(p AnimationProps) bool { return p.Fill == "both" }},
		{"fill-mode-initial", func(p AnimationProps) bool { return p.Fill == "none" }},
		{"[animation-fill-mode:backwards]", func(p AnimationProps) bool { return p.Fill == "backwards" }},
	} {
		if p := style(t, c.class).AnimProps; !c.check(p) {
			t.Errorf("%s: %+v", c.class, p)
		}
	}
}

func TestAnimationPropsLeaveTheTransitionAlone(t *testing.T) {
	// duration-* and ease-* keep their transition meaning.
	s := style(t, "transition duration-300 ease-in")
	if s.Transition.Duration != 300*time.Millisecond || s.Transition.Timing != (CubicBezier{0.4, 0, 1, 1}) {
		t.Errorf("transition %+v", s.Transition)
	}
	// The new utilities and the animation-only ones set nothing there.
	s = style(t, "animation-duration-500 repeat-2 direction-reverse fill-mode-both")
	if s.Transition.DurationSet || s.Transition.Duration != 0 {
		t.Errorf("animation-duration-500 changed the transition: %+v", s.Transition)
	}
	// SVG paints named fill-* are not mistaken for fill-mode-*.
	if s := style(t, "fill-red-500 fill-mode-both"); s.AnimProps.Fill != "both" || s.Fill == (SVGPaint{}) {
		t.Errorf("fill-red-500 fill-mode-both: paint %+v, mode %q", s.Fill, s.AnimProps.Fill)
	}
}

func TestAnimationPropsLastWinsAndVariants(t *testing.T) {
	if d := style(t, "duration-100 [animation-duration:2s]").AnimProps.Duration; d != 2*time.Second {
		t.Errorf("last class: %v", d)
	}
	if d := style(t, "[animation-duration:2s] animation-duration-100").AnimProps.Duration; d != 100*time.Millisecond {
		t.Errorf("last class: %v", d)
	}
	if c := Compile("hover:repeat-infinite md:[animation-direction:reverse]"); c.Err != nil {
		t.Errorf("variants: %v", c.Err)
	}
}

func TestAnimationPropsErrors(t *testing.T) {
	for _, c := range []struct{ class, want string }{
		{"[animation-duration:fast]", "use a time"},
		{"[animation-duration:-1s]", "use a time"},
		{"[animation-iteration-count:-1]", "invalid iteration count"},
		{"[animation-iteration-count:lots]", "use a number"},
		{"[animation-direction:sideways]", "use one of normal, reverse, alternate, alternate-reverse"},
		{"[animation-fill-mode:always]", "use one of none, forwards, backwards, both"},
		{"direction-[up]", "use one of normal"},
		{"fill-mode-[x]", "use one of none"},
		{"[animation-timing-function:fast]", "not an easing curve"},
	} {
		err := Compile(c.class).Err
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one with %q", c.class, err, c.want)
		}
	}
	// A value that is not a keyword is no utility at all, with a suggestion.
	if err := Compile("direction-sideways").Err; err == nil {
		t.Error("direction-sideways compiled")
	}
	if err := Compile("repeat-lots").Err; err == nil {
		t.Error("repeat-lots compiled")
	}
}

func TestAnimationPropsVocabulary(t *testing.T) {
	p := DefaultParser()
	v := p.Vocabulary()
	for _, c := range []string{"repeat-infinite", "repeat-3", "direction-reverse", "direction-alternate", "fill-mode-forwards", "fill-mode-both", "animation-duration-300"} {
		found := false
		for _, x := range v {
			found = found || x == c
		}
		if !found {
			t.Errorf("vocabulary lacks %q", c)
		}
	}
	for typo, want := range map[string]string{
		"repeat-infinit":     "repeat-infinite",
		"fill-mode-forward":  "fill-mode-forwards",
		"direction-alternat": "direction-alternate",
	} {
		if got := p.Suggest(typo); got != want {
			t.Errorf("Suggest(%q) = %q, want %q", typo, got, want)
		}
	}
}
