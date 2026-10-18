package anim

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Curve is an easing function: it maps the linear progress of an animation,
// a number in [0, 1], to the eased progress that interpolates the values.
// A curve may leave [0, 1] in between (cubic-bezier() with y values outside
// it overshoots), but At(0) is 0 and At(1) is 1.
//
// Curves are immutable values; implementations are safe for concurrent use.
type Curve interface {
	At(progress float64) float64
}

// Linear is the identity curve (CSS linear).
type Linear struct{}

// At implements Curve.
func (Linear) At(x float64) float64 { return clamp01(x) }

// CubicBezier is a CSS cubic-bezier(x1, y1, x2, y2) easing curve (CSS
// Easing Functions Level 1). The end points are (0, 0) and (1, 1); x1 and
// x2 must be in [0, 1].
type CubicBezier struct{ X1, Y1, X2, Y2 float64 }

// The CSS easing keywords (CSS Easing Functions Level 1).
var (
	Ease      = CubicBezier{0.25, 0.1, 0.25, 1}
	EaseIn    = CubicBezier{0.42, 0, 1, 1}
	EaseOut   = CubicBezier{0, 0, 0.58, 1}
	EaseInOut = CubicBezier{0.42, 0, 0.58, 1}
)

// Tailwind CSS v4's easing tokens (--ease-in, --ease-out, --ease-in-out),
// which differ from the CSS keywords of the same names, and its default
// transition timing function. They are the curves of the ease-in, ease-out
// and ease-in-out utilities and of the bare "transition" class.
var (
	TailwindEaseIn    = CubicBezier{0.4, 0, 1, 1}
	TailwindEaseOut   = CubicBezier{0, 0, 0.2, 1}
	TailwindEaseInOut = CubicBezier{0.4, 0, 0.2, 1}
	TailwindDefault   = TailwindEaseInOut
)

// bezierCoeffs returns the polynomial form of one component of the curve,
// c(t) = ((a*t + b)*t + c)*t, with every product that feeds an addition
// rounded explicitly so that the result is the same on every architecture
// (ADR-041).
func bezierCoeffs(p1, p2 float64) (a, b, c float64) {
	c = float64(3 * p1)
	b = float64(3*(p2-p1)) - c
	a = 1 - c - b
	return
}

func bezierSample(a, b, c, t float64) float64 {
	v := float64(a * t)
	v = float64((v + b) * t)
	return float64((v + c) * t)
}

func bezierDeriv(a, b, c, t float64) float64 {
	v := float64(float64(3*a) * t)
	v = float64((v + float64(2*b)) * t)
	return v + c
}

// At implements Curve. It solves x(t) = progress for t with Newton's
// method, falling back to bisection (as WebKit's UnitBezier and Chromium's
// gfx::CubicBezier do), and returns y(t).
func (c CubicBezier) At(x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	ax, bx, cx := bezierCoeffs(c.X1, c.X2)
	ay, by, cy := bezierCoeffs(c.Y1, c.Y2)
	const eps = 1e-7
	t := x
	for range 8 {
		f := bezierSample(ax, bx, cx, t) - x
		if f < eps && f > -eps {
			return bezierSample(ay, by, cy, t)
		}
		d := bezierDeriv(ax, bx, cx, t)
		if d < 1e-6 && d > -1e-6 {
			break
		}
		t -= f / d
	}
	lo, hi := 0.0, 1.0
	t = x
	for range 60 {
		f := bezierSample(ax, bx, cx, t)
		if f < x {
			lo = t
		} else {
			hi = t
		}
		if hi-lo < 1e-12 {
			break
		}
		t = (lo + hi) / 2
	}
	return bezierSample(ay, by, cy, t)
}

// StepPosition says where a Steps curve jumps.
type StepPosition uint8

const (
	// JumpEnd (the default, CSS "end") jumps at the end of each interval.
	JumpEnd StepPosition = iota
	// JumpStart (CSS "start") jumps at the start of each interval.
	JumpStart
	// JumpNone has no jump at either end; the curve stays at 0 and 1 for a
	// whole interval each. It needs at least two steps.
	JumpNone
	// JumpBoth jumps at both ends.
	JumpBoth
)

// Steps is the CSS steps(n, position) easing curve.
type Steps struct {
	N   int
	Pos StepPosition
}

// At implements Curve, following CSS Easing Functions Level 1, section 2.3.
func (s Steps) At(x float64) float64 {
	n := max(s.N, 1)
	jumps := n
	switch s.Pos {
	case JumpNone:
		jumps = n - 1
	case JumpBoth:
		jumps = n + 1
	}
	jumps = max(jumps, 1)
	x = clamp01(x)
	cur := int(math.Floor(float64(x * float64(n))))
	if s.Pos == JumpStart || s.Pos == JumpBoth {
		cur++
	}
	cur = min(max(cur, 0), jumps)
	return float64(cur) / float64(jumps)
}

// StepStart and StepEnd are the CSS step-start and step-end keywords.
var (
	StepStart = Steps{1, JumpStart}
	StepEnd   = Steps{1, JumpEnd}
)

// ParseEasing parses a CSS <easing-function>: the keywords linear, ease,
// ease-in, ease-out, ease-in-out, step-start and step-end, cubic-bezier(x1,
// y1, x2, y2) and steps(n[, position]). The error says what to write
// instead.
func ParseEasing(s string) (Curve, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "linear":
		return Linear{}, nil
	case "ease":
		return Ease, nil
	case "ease-in":
		return EaseIn, nil
	case "ease-out":
		return EaseOut, nil
	case "ease-in-out":
		return EaseInOut, nil
	case "step-start":
		return StepStart, nil
	case "step-end":
		return StepEnd, nil
	}
	name, args, ok := cutFunc(s)
	if !ok {
		return nil, fmt.Errorf("%q is not an easing function; use a keyword (linear, ease, ease-in, ease-out, ease-in-out, step-start, step-end), cubic-bezier(0.4, 0, 0.2, 1) or steps(4)", s)
	}
	switch name {
	case "cubic-bezier":
		if len(args) != 4 {
			return nil, fmt.Errorf("cubic-bezier() takes four numbers, got %q", s)
		}
		var v [4]float64
		for i, a := range args {
			f, err := strconv.ParseFloat(a, 64)
			if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
				return nil, fmt.Errorf("invalid number %q in %q", a, s)
			}
			v[i] = f
		}
		if v[0] < 0 || v[0] > 1 || v[2] < 0 || v[2] > 1 {
			return nil, fmt.Errorf("the x values of cubic-bezier() must be between 0 and 1, got %q", s)
		}
		return CubicBezier{v[0], v[1], v[2], v[3]}, nil
	case "steps":
		if len(args) < 1 || len(args) > 2 {
			return nil, fmt.Errorf("steps() takes a step count and an optional position, got %q", s)
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return nil, fmt.Errorf("the step count of %q must be a positive integer", s)
		}
		pos := JumpEnd
		if len(args) == 2 {
			switch args[1] {
			case "jump-end", "end":
				pos = JumpEnd
			case "jump-start", "start":
				pos = JumpStart
			case "jump-none":
				pos = JumpNone
			case "jump-both":
				pos = JumpBoth
			default:
				return nil, fmt.Errorf("unknown step position %q in %q; use jump-start, jump-end, jump-none, jump-both, start or end", args[1], s)
			}
		}
		if pos == JumpNone && n < 2 {
			return nil, fmt.Errorf("steps(%d, jump-none) needs at least two steps", n)
		}
		return Steps{n, pos}, nil
	}
	return nil, fmt.Errorf("%q is not an easing function; use cubic-bezier(...) or steps(...)", s)
}

// cutFunc splits "name(a, b, c)" into its name and trimmed arguments.
func cutFunc(s string) (name string, args []string, ok bool) {
	i := strings.IndexByte(s, '(')
	if i <= 0 || !strings.HasSuffix(s, ")") {
		return "", nil, false
	}
	name = s[:i]
	for _, a := range strings.Split(s[i+1:len(s)-1], ",") {
		args = append(args, strings.TrimSpace(a))
	}
	return name, args, true
}

func clamp01(x float64) float64 {
	switch {
	case x < 0 || x != x:
		return 0
	case x > 1:
		return 1
	}
	return x
}
