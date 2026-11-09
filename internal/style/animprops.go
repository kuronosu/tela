package style

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// AnimationLonghand is one of the animation longhands a class sets besides
// the delay (AnimationDelay): the bits of AnimationProps.Set.
type AnimationLonghand uint8

const (
	AnimDuration AnimationLonghand = 1 << iota
	AnimTiming
	AnimIterations
	AnimDirection
	AnimFillMode
)

// AnimationProps are the animation longhands that classes set and that
// replace those of the animate-* shorthand where Set has their bit
// (ADR-256): duration-*, animation-duration-*, ease-*, repeat-*,
// direction-*, fill-mode-* (tw-animate-css) and the arbitrary properties
// [animation-duration:...], [animation-timing-function:...],
// [animation-iteration-count:...], [animation-direction:...] and
// [animation-fill-mode:...]. The delay has its own fields
// (AnimationDelay, ADR-248).
type AnimationProps struct {
	Set      AnimationLonghand
	Duration time.Duration
	Timing   CubicBezier
	// Iterations is the iteration count; +Inf is infinite.
	Iterations float64
	// Direction is normal, reverse, alternate or alternate-reverse; Fill
	// is none, forwards, backwards or both.
	Direction string
	Fill      string
}

var (
	animDirections = []string{"normal", "reverse", "alternate", "alternate-reverse"}
	animFills      = []string{"none", "forwards", "backwards", "both"}
)

func inList(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// parseIterations parses animation-iteration-count: a non-negative number,
// infinite, or initial (1).
func parseIterations(v string) (float64, error) {
	switch v = strings.ToLower(strings.TrimSpace(v)); v {
	case "infinite":
		return math.Inf(1), nil
	case "initial":
		return 1, nil
	}
	if n, ok := parseFloat(v); ok && n >= 0 && v[0] != '+' {
		return float64(n), nil
	}
	return 0, fmt.Errorf("invalid iteration count %q; use a number such as 3, or infinite", v)
}

// parseKeyword parses a keyword of list, or initial (the first of list).
func parseKeyword(what string, list []string, v string) (string, error) {
	switch v = strings.ToLower(strings.TrimSpace(v)); {
	case v == "initial":
		return list[0], nil
	case inList(list, v):
		return v, nil
	}
	return "", fmt.Errorf("invalid animation %s %q; use one of %s", what, v, strings.Join(list, ", "))
}

// parseAnimationDuration parses the value of [animation-duration:...].
func parseAnimationDuration(v string) (Op, error) {
	v = strings.TrimSpace(v)
	d, ok := parseTime(v)
	if !ok && v == "0" {
		d, ok = 0, true
	}
	if !ok || d < 0 {
		return Op{}, fmt.Errorf("invalid animation duration %q; use a time such as [animation-duration:2s]", v)
	}
	return op(func(e *env) { e.s.AnimProps.setDuration(d) }), nil
}

// parseAnimationTiming parses the value of [animation-timing-function:...].
func parseAnimationTiming(v string) (Op, error) {
	cb, err := parseCubicBezier(v)
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) { e.s.AnimProps.setTiming(cb) }), nil
}

// parseAnimationIterations parses the value of [animation-iteration-count:...].
func parseAnimationIterations(v string) (Op, error) {
	n, err := parseIterations(v)
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) { e.s.AnimProps.setIterations(n) }), nil
}

// parseAnimationDirection parses the value of [animation-direction:...].
func parseAnimationDirection(v string) (Op, error) {
	d, err := parseKeyword("direction", animDirections, v)
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) { e.s.AnimProps.setDirection(d) }), nil
}

// parseAnimationFillMode parses the value of [animation-fill-mode:...].
func parseAnimationFillMode(v string) (Op, error) {
	f, err := parseKeyword("fill mode", animFills, v)
	if err != nil {
		return Op{}, err
	}
	return op(func(e *env) { e.s.AnimProps.setFill(f) }), nil
}

func (p *AnimationProps) setDuration(d time.Duration) { p.Duration, p.Set = d, p.Set|AnimDuration }
func (p *AnimationProps) setTiming(c CubicBezier)     { p.Timing, p.Set = c, p.Set|AnimTiming }
func (p *AnimationProps) setIterations(n float64)     { p.Iterations, p.Set = n, p.Set|AnimIterations }
func (p *AnimationProps) setDirection(d string)       { p.Direction, p.Set = d, p.Set|AnimDirection }
func (p *AnimationProps) setFill(f string)            { p.Fill, p.Set = f, p.Set|AnimFillMode }

// registerAnimationUtilities adds repeat-*, direction-* and fill-mode-*, the
// animation-duration-* alias, and the animation side of duration-* and
// ease-* (in util_motion.go).
func registerAnimationUtilities(r *registry) {
	keyword := func(root, what string, list []string, set func(*AnimationProps, string)) {
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			if a.hasMod {
				return Op{}, errNoMatch
			}
			v := a.value
			if a.arb {
				v = a.inner
			} else if v != "initial" && !inList(list, v) {
				return Op{}, errNoMatch
			}
			k, err := parseKeyword(what, list, v)
			if err != nil {
				return Op{}, err
			}
			return op(func(e *env) { set(&e.s.AnimProps, k) }), nil
		}, func(*Theme) []string { return append(append([]string{}, list...), "initial") })
	}
	keyword("direction", "direction", animDirections, (*AnimationProps).setDirection)
	keyword("fill-mode", "fill mode", animFills, (*AnimationProps).setFill)
	r.add("repeat", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		v := a.value
		if a.arb {
			v = a.inner
		} else if _, err := parseIterations(v); err != nil || v == "" {
			return Op{}, errNoMatch
		}
		n, err := parseIterations(v)
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) { e.s.AnimProps.setIterations(n) }), nil
	}, func(*Theme) []string { return []string{"0", "1", "2", "3", "infinite", "initial"} })
}
