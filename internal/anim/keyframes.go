package anim

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/style"
)

// Direction is animation-direction.
type Direction uint8

const (
	DirectionNormal Direction = iota
	DirectionReverse
	DirectionAlternate
	DirectionAlternateReverse
)

// FillMode is animation-fill-mode.
type FillMode uint8

const (
	FillNone FillMode = iota
	FillForwards
	FillBackwards
	FillBoth
)

// Infinite is the iteration count of animation-iteration-count: infinite.
var Infinite = math.Inf(1)

// Spec is a CSS animation: what the animation shorthand of a class such as
// animate-spin ("spin 1s linear infinite") sets.
type Spec struct {
	// Name is the name of the @keyframes rule.
	Name     string
	Duration time.Duration
	// Curve is animation-timing-function, the default easing of every
	// segment between two keyframes; a keyframe's own easing replaces it
	// for the segment that starts there. Nil is ease (the CSS initial
	// value).
	Curve      Curve
	Delay      time.Duration
	Iterations float64 // Infinite for infinite
	Direction  Direction
	Fill       FillMode
}

// Endless reports whether the animation never finishes by itself.
func (s *Spec) Endless() bool { return math.IsInf(s.Iterations, 1) }

// activeDuration is the duration of all iterations; zero when the animation
// has no duration.
func (s *Spec) activeDuration() time.Duration {
	if s.Endless() {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(float64(s.Duration) * s.Iterations)
}

// End returns the time the animation finishes, counted from the moment it
// started, and false for an endless animation.
func (s *Spec) End() (time.Duration, bool) {
	if s.Endless() {
		return 0, false
	}
	return s.Delay + s.activeDuration(), true
}

// Phase is where an animation is in its timeline.
type Phase uint8

const (
	// PhaseBefore is the delay: the animation applies its first keyframe
	// only with a backwards fill.
	PhaseBefore Phase = iota
	// PhaseActive is the time the animation runs.
	PhaseActive
	// PhaseAfter is the time past its end: its last keyframe applies only
	// with a forwards fill.
	PhaseAfter
)

// Progress returns the directed progress in [0, 1] of the animation
// elapsed after it started, the phase it is in, and whether it has an
// effect (it has one while active, and before and after with the fill
// modes that extend it). It follows the timing model of Web Animations
// Level 1.
func (s *Spec) Progress(elapsed time.Duration) (p float64, phase Phase, applies bool) {
	local := elapsed - s.Delay
	dur := s.Duration
	active := s.activeDuration()
	switch {
	case dur <= 0:
		// An animation without duration is over at once.
		if local < 0 {
			return 0, PhaseBefore, s.Fill == FillBackwards || s.Fill == FillBoth
		}
		return s.directed(math.Max(s.Iterations-1, 0), 1), PhaseAfter, s.Fill == FillForwards || s.Fill == FillBoth
	case local < 0:
		return s.directed(0, 0), PhaseBefore, s.Fill == FillBackwards || s.Fill == FillBoth
	case local >= active:
		// The end of the last iteration; an iteration count with a
		// fraction ends partway.
		iter := math.Ceil(s.Iterations) - 1
		frac := s.Iterations - iter
		return s.directed(iter, frac), PhaseAfter, s.Fill == FillForwards || s.Fill == FillBoth
	}
	overall := float64(local) / float64(dur)
	iter := math.Floor(overall)
	return s.directed(iter, overall-iter), PhaseActive, true
}

// directed applies animation-direction to the progress frac of iteration
// iter.
func (s *Spec) directed(iter, frac float64) float64 {
	rev := false
	switch s.Direction {
	case DirectionReverse:
		rev = true
	case DirectionAlternate:
		rev = math.Mod(iter, 2) == 1
	case DirectionAlternateReverse:
		rev = math.Mod(iter, 2) == 0
	}
	if rev {
		return 1 - frac
	}
	return frac
}

// Keyframe is one keyframe of an animation: the values it sets at an
// offset. Only the animated properties exist: opacity and transform, the
// ones that animate without relayout.
type Keyframe struct {
	// Offset is the position in [0, 1].
	Offset float64
	// Easing is animation-timing-function inside the keyframe: the easing
	// of the segment that starts here. Nil uses the animation's.
	Easing Curve
	// Opacity and Transform are set when non-nil. The transform is applied
	// on top of the box's own translate, rotate and scale (ComposeTransform);
	// "transform: none" is IdentityTransform.
	Opacity   *float32
	Transform *style.Transform
}

// Keyframes is a @keyframes rule: keyframes sorted by offset.
type Keyframes struct {
	Name   string
	Frames []Keyframe
}

func f32(v float32) *float32 { return &v }

func tf(t style.Transform) *style.Transform { return &t }

func translateY(pct float32) style.Transform {
	t := IdentityTransform()
	t.TranslateY = style.PctLP(pct)
	return t
}

func scaleBy(s float32) style.Transform {
	t := IdentityTransform()
	t.ScaleX, t.ScaleY = s, s
	return t
}

func rotateBy(deg float32) style.Transform {
	t := IdentityTransform()
	t.Rotate = deg
	return t
}

// Tailwind CSS v4's keyframes (internal/style/gen/tailwind/theme.css).
var builtinKeyframes = map[string]*Keyframes{
	"spin": {Name: "spin", Frames: []Keyframe{
		{Offset: 1, Transform: tf(rotateBy(360))},
	}},
	"ping": {Name: "ping", Frames: []Keyframe{
		{Offset: 0.75, Transform: tf(scaleBy(2)), Opacity: f32(0)},
		{Offset: 1, Transform: tf(scaleBy(2)), Opacity: f32(0)},
	}},
	"pulse": {Name: "pulse", Frames: []Keyframe{
		{Offset: 0.5, Opacity: f32(0.5)},
	}},
	"bounce": {Name: "bounce", Frames: []Keyframe{
		{Offset: 0, Transform: tf(translateY(-25)), Easing: CubicBezier{0.8, 0, 1, 1}},
		{Offset: 0.5, Transform: tf(IdentityTransform()), Easing: CubicBezier{0, 0, 0.2, 1}},
		{Offset: 1, Transform: tf(translateY(-25)), Easing: CubicBezier{0.8, 0, 1, 1}},
	}},
}

// Builtin returns the keyframes of Tailwind's animate-* utilities: spin,
// ping, pulse and bounce.
func Builtin(name string) (*Keyframes, bool) {
	k, ok := builtinKeyframes[name]
	return k, ok
}

// Values are the animated values at one instant.
type Values struct {
	HasOpacity   bool
	Opacity      float32
	HasTransform bool
	// Transform is the animated transform, to compose over the box's
	// (ComposeTransform).
	Transform style.Transform
}

// At samples the keyframes at the directed progress p in [0, 1]. def is the
// animation's timing function; baseOpacity is the box's own opacity, the
// value an opacity animation has where its keyframes set none (CSS's
// implicit 0% and 100% keyframes; a transform's implicit value is none).
func (k *Keyframes) At(p float64, def Curve, baseOpacity float32) Values {
	if def == nil {
		def = Ease
	}
	var v Values
	// Each property interpolates between the keyframes that set it.
	if a, b, from, to := k.around(p, func(f *Keyframe) bool { return f.Opacity != nil }); a != nil || b != nil {
		v.HasOpacity = true
		x, y := baseOpacity, baseOpacity
		easing := def
		if a != nil {
			x = *a.Opacity
			if a.Easing != nil {
				easing = a.Easing
			}
		}
		if b != nil {
			y = *b.Opacity
		}
		if to <= from {
			v.Opacity = x
		} else {
			v.Opacity = Lerp(x, y, easing.At(segment(p, from, to)))
		}
	}
	if a, b, from, to := k.around(p, func(f *Keyframe) bool { return f.Transform != nil }); a != nil || b != nil {
		v.HasTransform = true
		x, y := IdentityTransform(), IdentityTransform()
		easing := def
		if a != nil {
			x = *a.Transform
			if a.Easing != nil {
				easing = a.Easing
			}
		}
		if b != nil {
			y = *b.Transform
		}
		if to <= from {
			v.Transform = x
		} else {
			v.Transform = LerpTransform(x, y, easing.At(segment(p, from, to)))
		}
	}
	return v
}

// around returns the keyframes that set a property (has) on both sides of
// p: the last at or before it and the first after it, with their offsets;
// a missing one is the implicit keyframe at 0% or 100%.
func (k *Keyframes) around(p float64, has func(*Keyframe) bool) (a, b *Keyframe, from, to float64) {
	to = 1
	for i := range k.Frames {
		f := &k.Frames[i]
		if !has(f) {
			continue
		}
		if f.Offset <= p {
			a, from = f, f.Offset
		} else if b == nil {
			b, to = f, f.Offset
		}
	}
	return
}

// segment returns the progress of p inside [from, to].
func segment(p, from, to float64) float64 {
	if to <= from {
		return 1
	}
	return clamp01((p - from) / (to - from))
}

// ParseSpec parses the animation shorthand of an animate-* class: a name, a
// duration, an easing function, a delay, an iteration count (a number or
// infinite), a direction and a fill mode in any order. The first time is the
// duration, the second the delay. A play state (running) is accepted and
// ignored. An error says what is wrong.
func ParseSpec(s string) (Spec, error) {
	sp := Spec{Iterations: 1}
	fields := splitTop(strings.TrimSpace(s))
	if len(fields) == 0 {
		return sp, fmt.Errorf("empty animation; use e.g. \"spin 1s linear infinite\"")
	}
	times := 0
	for _, f := range fields {
		lf := strings.ToLower(f)
		switch {
		case lf == "infinite":
			sp.Iterations = Infinite
		case lf == "normal":
			sp.Direction = DirectionNormal
		case lf == "reverse":
			sp.Direction = DirectionReverse
		case lf == "alternate":
			sp.Direction = DirectionAlternate
		case lf == "alternate-reverse":
			sp.Direction = DirectionAlternateReverse
		case lf == "forwards":
			sp.Fill = FillForwards
		case lf == "backwards":
			sp.Fill = FillBackwards
		case lf == "both":
			sp.Fill = FillBoth
		case lf == "running":
		case lf == "paused":
			return sp, fmt.Errorf("animation-play-state: paused is not supported")
		case lf == "none":
			return sp, nil
		case isTime(lf):
			d, err := parseTime(lf)
			if err != nil {
				return sp, err
			}
			switch times {
			case 0:
				sp.Duration = d
			case 1:
				sp.Delay = d
			default:
				return sp, fmt.Errorf("too many times in animation %q: a duration and a delay at most", s)
			}
			times++
		case isEasing(lf):
			c, err := ParseEasing(lf)
			if err != nil {
				return sp, err
			}
			sp.Curve = c
		default:
			if n, err := strconv.ParseFloat(lf, 64); err == nil {
				if n < 0 {
					return sp, fmt.Errorf("the iteration count of %q cannot be negative", s)
				}
				sp.Iterations = n
				continue
			}
			if sp.Name != "" {
				return sp, fmt.Errorf("animation %q names two keyframes (%q and %q); give one", s, sp.Name, f)
			}
			sp.Name = f
		}
	}
	return sp, nil
}

func isTime(s string) bool {
	if len(s) < 2 {
		return false
	}
	c := s[0]
	if !(c >= '0' && c <= '9' || c == '.' || c == '-' || c == '+') {
		return false
	}
	return strings.HasSuffix(s, "ms") || strings.HasSuffix(s, "s")
}

func parseTime(s string) (time.Duration, error) {
	unit := time.Second
	num := strings.TrimSuffix(s, "s")
	if strings.HasSuffix(s, "ms") {
		unit = time.Millisecond
		num = strings.TrimSuffix(s, "ms")
	}
	n, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a time; use e.g. 1s or 300ms", s)
	}
	return time.Duration(n * float64(unit)), nil
}

func isEasing(s string) bool {
	switch s {
	case "linear", "ease", "ease-in", "ease-out", "ease-in-out", "step-start", "step-end":
		return true
	}
	return strings.HasPrefix(s, "cubic-bezier(") || strings.HasPrefix(s, "steps(")
}

// splitTop splits s at whitespace outside parentheses.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '(':
			depth++
		case c == ')':
			depth--
		case (c == ' ' || c == '\t') && depth == 0:
			if start >= 0 {
				out = append(out, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, s[start:])
	}
	return out
}
