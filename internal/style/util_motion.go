package style

import (
	"fmt"
	"strings"
	"time"
)

func registerMotion(r *registry) {
	// translate.
	for _, t := range []struct {
		root string
		x, y bool
	}{{"translate", true, true}, {"translate-x", true, false}, {"translate-y", false, true}} {
		t := t
		r.dimUtil(t.root, dimFull|dimFrac|dimSpacing|dimNeg, axisX, func(e *env, d Dim) {
			if t.x {
				e.s.Transform.TranslateX = d.LP
			}
			if t.y {
				e.s.Transform.TranslateY = d.LP
			}
		})
	}
	r.static("translate-none", op(func(e *env) {
		e.s.Transform.TranslateX, e.s.Transform.TranslateY = LP{}, LP{}
	}))

	// scale.
	for _, t := range []struct {
		root string
		x, y bool
	}{{"scale", true, true}, {"scale-x", true, false}, {"scale-y", false, true}} {
		t := t
		r.add(t.root, true, func(c *pctx, a *arg) (Op, error) {
			var v float32
			switch {
			case a.hasMod:
				return Op{}, errNoMatch
			case a.arb:
				n, ok := numberArg(a)
				if !ok {
					return Op{}, fmt.Errorf("%q is not a number; use e.g. scale-[1.02]", a.inner)
				}
				v = n
			default:
				n, ok := parseFloat(a.value)
				if !ok || n < 0 || a.value[0] == '+' {
					return Op{}, errNoMatch
				}
				v = n / 100
			}
			if a.neg {
				v = -v
			}
			return op(func(e *env) {
				if t.x {
					e.s.Transform.ScaleX = v
				}
				if t.y {
					e.s.Transform.ScaleY = v
				}
			}), nil
		}, func(*Theme) []string { return []string{"0", "50", "75", "90", "95", "100", "105", "110", "125", "150"} })
	}
	r.static("scale-none", op(func(e *env) { e.s.Transform.ScaleX, e.s.Transform.ScaleY = 1, 1 }))

	// rotate and skew take degrees.
	angle := func(root string, set func(*env, float32)) {
		r.add(root, true, func(c *pctx, a *arg) (Op, error) {
			if a.hasMod {
				return Op{}, errNoMatch
			}
			var deg float32
			if a.arb {
				d, ok := parseAngle(a.inner)
				if !ok {
					return Op{}, fmt.Errorf("%q is not an angle; use e.g. %s-[17deg] or %s-[0.25turn]", a.inner, root, root)
				}
				deg = d
			} else {
				n, ok := parseFloat(a.value)
				if !ok || n < 0 || a.value[0] == '+' {
					return Op{}, errNoMatch
				}
				deg = n
			}
			if a.neg {
				deg = -deg
			}
			return op(func(e *env) { set(e, deg) }), nil
		}, func(*Theme) []string { return []string{"0", "1", "2", "3", "6", "12", "45", "90", "180"} })
	}
	angle("rotate", func(e *env, d float32) { e.s.Transform.Rotate = d })
	angle("skew", func(e *env, d float32) { e.s.Transform.SkewX, e.s.Transform.SkewY = d, d })
	angle("skew-x", func(e *env, d float32) { e.s.Transform.SkewX = d })
	angle("skew-y", func(e *env, d float32) { e.s.Transform.SkewY = d })
	r.static("rotate-none", op(func(e *env) { e.s.Transform.Rotate = 0 }))

	// transform origin.
	origins := map[string]Op{}
	for _, k := range sortedMapKeys(backgroundPositions) {
		p := backgroundPositions[k]
		origins[k] = op(func(e *env) { e.s.Transform.OriginX, e.s.Transform.OriginY = PctLP(p[0]), PctLP(p[1]) })
	}
	r.enum("origin", false, origins)
	r.add("origin", false, func(c *pctx, a *arg) (Op, error) {
		if !a.arb || a.hasMod {
			return Op{}, errNoMatch
		}
		x, y, err := parsePositionPair(strings.Fields(a.inner))
		if err != nil {
			return Op{}, err
		}
		return op(func(e *env) { e.s.Transform.OriginX, e.s.Transform.OriginY = x, y }), nil
	}, nil)

	r.static("transform", Op{})
	r.static("transform-gpu", Op{})
	r.static("transform-cpu", Op{})
	r.static("transform-none", op(func(e *env) {
		t := &e.s.Transform
		t.TranslateX, t.TranslateY, t.Rotate, t.ScaleX, t.ScaleY, t.SkewX, t.SkewY = LP{}, LP{}, 0, 1, 1, 0, 0
	}))

	registerTransitions(r)
}

// transitionProps maps CSS property names to the sets they transition.
var transitionProps = map[string]TransitionProps{
	"all": TransitionAll, "none": TransitionNone,
	"color": TransitionColor, "background-color": TransitionBackgroundColor, "background": TransitionBackgroundColor,
	"border-color": TransitionBorderColor, "outline-color": TransitionOutlineColor,
	"text-decoration-color": TransitionDecorationColor, "opacity": TransitionOpacity,
	"box-shadow": TransitionShadow, "transform": TransitionTransform, "translate": TransitionTransform,
	"scale": TransitionTransform, "rotate": TransitionTransform, "filter": TransitionFilter,
	"backdrop-filter": TransitionBackdropFilter, "width": TransitionWidth, "height": TransitionHeight,
	"padding": TransitionSpacing, "margin": TransitionSpacing, "gap": TransitionSpacing,
	"top": TransitionInset, "right": TransitionInset, "bottom": TransitionInset, "left": TransitionInset,
	"inset": TransitionInset, "border-radius": TransitionBorderRadius, "border-width": TransitionBorderWidth,
	"font-size": TransitionFontSize, "clip-path": TransitionClipPath, "fill": 0, "stroke": 0,
}

func registerTransitions(r *registry) {
	setProps := func(p TransitionProps) Op {
		return op(func(e *env) { e.s.Transition.Properties = p })
	}
	r.add("transition", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		switch {
		case a.raw == "":
			return setProps(TransitionDefault), nil
		case a.arb:
			var p TransitionProps
			for _, name := range splitTop(a.inner, ',') {
				name = strings.TrimSpace(name)
				v, ok := transitionProps[name]
				if !ok {
					return Op{}, fmt.Errorf("cannot transition the property %q; use one of %s", name,
						strings.Join(sortedMapKeys(transitionProps), ", "))
				}
				p |= v
			}
			return setProps(p), nil
		}
		switch a.value {
		case "all":
			return setProps(TransitionAll), nil
		case "none":
			return setProps(TransitionNone), nil
		case "colors":
			return setProps(TransitionColors), nil
		case "opacity":
			return setProps(TransitionOpacity), nil
		case "shadow":
			return setProps(TransitionShadow), nil
		case "transform":
			return setProps(TransitionTransform), nil
		}
		return Op{}, errNoMatch
	}, func(*Theme) []string { return []string{"", "all", "none", "colors", "opacity", "shadow", "transform"} })

	timeValue := func(c *pctx, a *arg) (time.Duration, error) {
		if a.hasMod {
			return 0, errNoMatch
		}
		if a.arb {
			if d, ok := parseTime(a.inner); ok {
				return d, nil
			}
			if n, ok := parseFloat(a.inner); ok {
				return time.Duration(float64(n) * float64(time.Millisecond)), nil
			}
			return 0, fmt.Errorf("%q is not a time; use e.g. [1.5s] or [300ms]", a.inner)
		}
		n, ok := parseFloat(a.value)
		if !ok || n < 0 || a.value[0] == '+' {
			return 0, errNoMatch
		}
		return time.Duration(float64(n) * float64(time.Millisecond)), nil
	}
	times := func(root string, set func(*env, time.Duration)) {
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			d, err := timeValue(c, a)
			if err != nil {
				return Op{}, err
			}
			return op(func(e *env) { set(e, d) }), nil
		}, func(*Theme) []string {
			return []string{"0", "75", "100", "150", "200", "300", "500", "700", "1000"}
		})
	}
	// duration-* also sets animation-duration (tw-animate-css, ADR-256), as
	// delay-* sets the delay, and animation-duration-* is the animation
	// alone.
	times("duration", func(e *env, d time.Duration) {
		e.s.Transition.Duration, e.sc.durSet = d, true
		e.s.AnimProps.setDuration(d)
	})
	times("animation-duration", func(e *env, d time.Duration) { e.s.AnimProps.setDuration(d) })
	registerAnimationUtilities(r)
	// delay-* sets transition-delay (Tailwind v4) and animation-delay
	// (tw-animate-css, which shadcn/ui's projects load, ADR-248): it is how
	// a list staggers its animate-* classes, delay-100 delay-200...
	times("delay", func(e *env, d time.Duration) {
		e.s.Transition.Delay = d
		e.s.AnimationDelay, e.s.AnimationDelaySet = d, true
	})
	// animation-delay-* is the animation-delay alone (the plugins
	// tailwindcss-animation-delay and tailwindcss-motion), and it may be
	// negative: -animation-delay-300 starts the animation 300ms in.
	r.add("animation-delay", true, func(c *pctx, a *arg) (Op, error) {
		d, err := timeValue(c, a)
		if err != nil {
			return Op{}, err
		}
		if a.neg {
			d = -d
		}
		return op(func(e *env) { e.s.AnimationDelay, e.s.AnimationDelaySet = d, true }), nil
	}, func(*Theme) []string {
		return []string{"0", "75", "100", "150", "200", "300", "500", "700", "1000"}
	})

	r.add("ease", false, func(c *pctx, a *arg) (Op, error) {
		var cb CubicBezier
		switch {
		case a.hasMod:
			return Op{}, errNoMatch
		case a.arb:
			v, err := parseCubicBezier(a.inner)
			if err != nil {
				return Op{}, err
			}
			cb = v
		case a.value == "linear":
			cb = CubicBezier{0, 0, 1, 1}
		case a.value == "initial":
			cb = CubicBezier{0.25, 0.1, 0.25, 1}
		default:
			v, ok := c.th.Ease[a.value]
			if !ok {
				return Op{}, errNoMatch
			}
			cb = v
		}
		return op(func(e *env) {
			e.s.Transition.Timing, e.sc.timingSet = cb, true
			e.s.AnimProps.setTiming(cb) // and the animation's (ADR-256)
		}), nil
	}, func(th *Theme) []string { return append(th.Keys(NamespaceEase), "linear", "initial") })

	r.add("animate", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		var v string
		switch {
		case a.arb:
			v = a.inner
		case a.value == "none":
		default:
			s, ok := c.th.Animate[a.value]
			if !ok {
				return Op{}, errNoMatch
			}
			v = s
		}
		return op(func(e *env) { e.s.Animation = v }), nil
	}, func(th *Theme) []string { return append(th.Keys(NamespaceAnimate), "none") })
}

// parseCubicBezier parses cubic-bezier(x1, y1, x2, y2) or the CSS easing
// keywords.
func parseCubicBezier(s string) (CubicBezier, error) {
	switch s = strings.TrimSpace(s); s {
	case "linear":
		return CubicBezier{0, 0, 1, 1}, nil
	case "ease":
		return CubicBezier{0.25, 0.1, 0.25, 1}, nil
	case "ease-in":
		return CubicBezier{0.42, 0, 1, 1}, nil
	case "ease-out":
		return CubicBezier{0, 0, 0.58, 1}, nil
	case "ease-in-out":
		return CubicBezier{0.42, 0, 0.58, 1}, nil
	}
	if !strings.HasPrefix(s, "cubic-bezier(") || !strings.HasSuffix(s, ")") {
		return CubicBezier{}, fmt.Errorf("%q is not an easing curve; use cubic-bezier(0.4, 0, 0.2, 1) or a keyword", s)
	}
	parts := splitTop(s[len("cubic-bezier("):len(s)-1], ',')
	if len(parts) != 4 {
		return CubicBezier{}, fmt.Errorf("cubic-bezier() takes four numbers, got %q", s)
	}
	var v [4]float32
	for i, p := range parts {
		n, ok := parseFloat(strings.TrimSpace(p))
		if !ok {
			return CubicBezier{}, fmt.Errorf("invalid number %q in %q", strings.TrimSpace(p), s)
		}
		v[i] = n
	}
	if v[0] < 0 || v[0] > 1 || v[2] < 0 || v[2] > 1 {
		return CubicBezier{}, fmt.Errorf("the x values of cubic-bezier() must be between 0 and 1, got %q", s)
	}
	return CubicBezier{v[0], v[1], v[2], v[3]}, nil
}

// parseAnimationDelay parses the value of [animation-delay:...], a CSS time
// that may be negative ([animation-delay:-0.3s], [animation-delay:150ms]).
func parseAnimationDelay(v string) (Op, error) {
	v = strings.TrimSpace(v)
	d, ok := parseTime(v)
	if !ok && v == "0" {
		d, ok = 0, true
	}
	if !ok {
		return Op{}, fmt.Errorf("invalid animation delay %q; use a time such as [animation-delay:150ms] or [animation-delay:-0.3s]", v)
	}
	return op(func(e *env) { e.s.AnimationDelay, e.s.AnimationDelaySet = d, true }), nil
}
