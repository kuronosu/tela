package anim

import (
	"math"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// bernstein evaluates one component of a cubic Bézier with end points 0 and
// 1 in its Bernstein form, the formula of CSS Easing Functions Level 1.
func bernstein(p1, p2, t float64) float64 {
	u := 1 - t
	return 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t
}

// reference solves x(t) = x by bisection on the Bernstein form.
func reference(c CubicBezier, x float64) float64 {
	lo, hi := 0.0, 1.0
	for range 80 {
		mid := (lo + hi) / 2
		if bernstein(c.X1, c.X2, mid) < x {
			lo = mid
		} else {
			hi = mid
		}
	}
	return bernstein(c.Y1, c.Y2, (lo+hi)/2)
}

func TestCubicBezierAgainstTheSpecFormula(t *testing.T) {
	curves := map[string]CubicBezier{
		"ease": Ease, "ease-in": EaseIn, "ease-out": EaseOut, "ease-in-out": EaseInOut,
		"tw-in": TailwindEaseIn, "tw-out": TailwindEaseOut, "tw-in-out": TailwindEaseInOut,
		"bounce-in": {0.8, 0, 1, 1}, "pulse": {0.4, 0, 0.6, 1},
		"overshoot": {0.34, 1.56, 0.64, 1}, "steep": {0.9, 0, 0.1, 1},
	}
	for name, c := range curves {
		for i := 0; i <= 100; i++ {
			x := float64(i) / 100
			if got, want := c.At(x), reference(c, x); math.Abs(got-want) > 1e-6 {
				t.Errorf("%s(%g) = %.9f, want %.9f", name, x, got, want)
			}
		}
	}
	// A few values quoted in the wild.
	if got := Ease.At(0.5); math.Abs(got-0.8024033) > 1e-6 {
		t.Errorf("ease(0.5) = %v, want 0.8024033", got)
	}
	if got := EaseInOut.At(0.5); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("ease-in-out(0.5) = %v", got)
	}
	if got := (Linear{}).At(0.3); got != 0.3 {
		t.Errorf("linear(0.3) = %v", got)
	}
	for _, c := range []Curve{Ease, TailwindDefault, Linear{}} {
		if c.At(0) != 0 || c.At(1) != 1 || c.At(-1) != 0 || c.At(2) != 1 {
			t.Errorf("%v does not clamp its ends", c)
		}
	}
}

func TestSteps(t *testing.T) {
	cases := []struct {
		s    Steps
		x    float64
		want float64
	}{
		{Steps{4, JumpEnd}, 0, 0}, {Steps{4, JumpEnd}, 0.24, 0}, {Steps{4, JumpEnd}, 0.25, 0.25},
		{Steps{4, JumpEnd}, 0.99, 0.75}, {Steps{4, JumpEnd}, 1, 1},
		{Steps{4, JumpStart}, 0, 0.25}, {Steps{4, JumpStart}, 0.25, 0.5}, {Steps{4, JumpStart}, 0.99, 1},
		{Steps{4, JumpNone}, 0, 0}, {Steps{4, JumpNone}, 0.3, 1.0 / 3}, {Steps{4, JumpNone}, 0.99, 1},
		{Steps{2, JumpBoth}, 0, 1.0 / 3}, {Steps{2, JumpBoth}, 0.5, 2.0 / 3}, {Steps{2, JumpBoth}, 0.99, 2.0 / 3}, {Steps{2, JumpBoth}, 1, 1},
		{StepStart, 0.01, 1}, {StepEnd, 0.99, 0},
	}
	for _, c := range cases {
		if got := c.s.At(c.x); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("%+v.At(%g) = %v, want %v", c.s, c.x, got, c.want)
		}
	}
}

func TestParseEasing(t *testing.T) {
	good := map[string]Curve{
		"linear": Linear{}, "ease": Ease, "ease-in": EaseIn, "step-end": StepEnd,
		"cubic-bezier(0.4, 0, 0.2, 1)": TailwindEaseInOut, " CUBIC-BEZIER(0,0,.2,1) ": TailwindEaseOut,
		"steps(4)": Steps{4, JumpEnd}, "steps(3, jump-start)": Steps{3, JumpStart}, "steps(2, end)": Steps{2, JumpEnd},
	}
	for in, want := range good {
		got, err := ParseEasing(in)
		if err != nil || got != want {
			t.Errorf("ParseEasing(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "bounce", "cubic-bezier(1,2,3)", "cubic-bezier(2,0,1,1)", "steps(0)", "steps(2, jump-none-ish)", "steps(1, jump-none)"} {
		if _, err := ParseEasing(bad); err == nil {
			t.Errorf("ParseEasing(%q) accepted", bad)
		}
	}
}

func TestSpringSettlesAndStartsAtRest(t *testing.T) {
	for name, s := range map[string]Spring{
		"default":    DefaultSpring(),
		"critical":   {Stiffness: 100, Damping: 20, Mass: 1},
		"overdamped": {Stiffness: 100, Damping: 40, Mass: 1},
		"stiff":      {Stiffness: 400, Damping: 15, Mass: 0.5},
		"velocity":   {Stiffness: 170, Damping: 26, Mass: 1, Velocity: 300},
	} {
		x0, v0 := s.At(0, 10, 110)
		if x0 != 10 || v0 != s.Velocity {
			t.Errorf("%s: At(0) = %v, %v", name, x0, v0)
		}
		r := DefaultRest(100)
		d := s.Duration(10, 110, r)
		if d <= 0 || d >= maxSettle {
			t.Errorf("%s: duration %v", name, d)
		}
		if !s.Settled(d.Seconds()+0.01, 10, 110, r) || s.Settled(d.Seconds()-0.05, 10, 110, r) && d > 100*time.Millisecond {
			t.Errorf("%s: settle detection around %v", name, d)
		}
		// The end is the target.
		if x, _ := s.At(30, 10, 110); math.Abs(x-110) > 1e-6 {
			t.Errorf("%s: At(30s) = %v", name, x)
		}
	}
	// A spring that does not move is at rest at once.
	if d := DefaultSpring().Duration(5, 5, DefaultRest(0)); d != 0 {
		t.Errorf("duration of a spring that stays: %v", d)
	}
}

func TestSpringDampingBehaviour(t *testing.T) {
	// Underdamped overshoots, critical and overdamped do not.
	peak := func(s Spring) float64 {
		best := 0.0
		for i := 0; i < 600; i++ {
			x, _ := s.At(float64(i)/200, 0, 1)
			best = max(best, x)
		}
		return best
	}
	if p := peak(DefaultSpring()); p < 1.1 {
		t.Errorf("default spring peaks at %v, want an overshoot", p)
	}
	if p := peak(Spring{Stiffness: 100, Damping: 20, Mass: 1}); p > 1+1e-9 {
		t.Errorf("critically damped spring overshoots: %v", p)
	}
	if p := peak(Spring{Stiffness: 100, Damping: 40, Mass: 1}); p > 1+1e-9 {
		t.Errorf("overdamped spring overshoots: %v", p)
	}
	// The closed form at a known point: stiffness 100, damping 10, mass 1
	// (omega 10, zeta 0.5) from 0 to 1 at t = 0.5 s is
	// 1 - e^-2.5 (cos 4.33 + sin 4.33 / sqrt(3)).
	w := 10 * math.Sqrt(0.75)
	want := 1 - math.Exp(-2.5)*(math.Cos(w*0.5)+5/w*math.Sin(w*0.5))
	if x, _ := DefaultSpring().At(0.5, 0, 1); math.Abs(x-want) > 1e-9 {
		t.Errorf("spring(0.5) = %v, want %v", x, want)
	}
	// The velocity matches the numerical derivative.
	s := Spring{Stiffness: 120, Damping: 12, Mass: 1.3, Velocity: 40}
	for _, tt := range []float64{0.05, 0.2, 0.7} {
		const h = 1e-6
		a, _ := s.At(tt-h, 0, 50)
		b, _ := s.At(tt+h, 0, 50)
		_, v := s.At(tt, 0, 50)
		if num := (b - a) / (2 * h); math.Abs(num-v) > 1e-3*max(1, math.Abs(num)) {
			t.Errorf("velocity at %g: %v, derivative %v", tt, v, num)
		}
	}
}

func TestSpringForDuration(t *testing.T) {
	for _, bounce := range []float64{0, 0.25, 0.5} {
		s := SpringForDuration(500*time.Millisecond, bounce)
		zeta := s.DampingRatio()
		if want := 1 - bounce; math.Abs(zeta-want) > 1e-9 {
			t.Errorf("bounce %g: damping ratio %v, want %v", bounce, zeta, want)
		}
		// The spring has moved most of the way by the duration and is at
		// rest not long after.
		x, _ := s.At(0.5, 0, 1)
		if math.Abs(1-x) > 0.05 {
			t.Errorf("bounce %g: at 500 ms the spring is at %v", bounce, x)
		}
		if d := s.Duration(0, 1, DefaultRest(1)); d > 3*time.Second {
			t.Errorf("bounce %g: settles in %v", bounce, d)
		}
	}
	// The same input gives the same spring (determinism).
	if SpringForDuration(300*time.Millisecond, 0.3) != SpringForDuration(300*time.Millisecond, 0.3) {
		t.Error("not deterministic")
	}
}

func near(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

func TestLerpColorSpaces(t *testing.T) {
	// Legacy sRGB colors interpolate in sRGB: black to white is gray 128.
	mid := LerpColor(gfx.Black, gfx.White, 0.5)
	if !near(float64(mid.R), 0.5, 1e-6) || mid.A != 1 {
		t.Errorf("black to white (sRGB): %v", mid)
	}
	// Oklch colors interpolate in Oklab: red-500 to blue-500 midpoint is
	// the Oklab midpoint, not the sRGB one.
	red := gfx.OKLCH(0.637, 0.237, 25.331, 1)
	blue := gfx.OKLCH(0.623, 0.214, 259.815, 1)
	m := LerpColor(red, blue, 0.5)
	rl, ra, rb := red.ToOklab()
	bl, ba, bb := blue.ToOklab()
	wl, wa, wb := gfx.Oklab((rl+bl)/2, (ra+ba)/2, (rb+bb)/2, 1).ToOklab()
	ml, ma, mb := m.ToOklab()
	if !near(float64(ml), float64(wl), 1e-4) || !near(float64(ma), float64(wa), 1e-4) || !near(float64(mb), float64(wb), 1e-4) {
		t.Errorf("oklch midpoint %v is not the Oklab midpoint", m)
	}
	srgb := gfx.Color{R: (red.R + blue.R) / 2, G: (red.G + blue.G) / 2, B: (red.B + blue.B) / 2, A: 1}
	if near(float64(m.G), float64(srgb.G), 1e-3) && near(float64(m.R), float64(srgb.R), 1e-3) {
		t.Errorf("oklch colors interpolated in sRGB")
	}
	// One non-legacy color is enough for Oklab: white to blue-500 differs
	// from the sRGB mix.
	w := LerpColor(gfx.White, blue, 0.5)
	if near(float64(w.R), float64(gfx.White.R+blue.R)/2, 1e-3) {
		t.Errorf("white to oklch blue interpolated in sRGB: %v", w)
	}
	// Endpoints are exact.
	if LerpColor(red, blue, 0) != red || LerpColor(red, blue, 1) != blue {
		t.Error("endpoints")
	}
	// Premultiplied: transparent to a color keeps the color's hue.
	h := LerpColor(gfx.Transparent, gfx.RGB8(255, 0, 0), 0.5)
	if !near(float64(h.R), 1, 1e-6) || h.G != 0 || !near(float64(h.A), 0.5, 1e-6) {
		t.Errorf("transparent to red: %v", h)
	}
	h = LerpColor(gfx.Transparent, blue, 0.25)
	if bl2, ba2, bb2 := h.ToOklab(); !near(float64(bl2), float64(bl), 1e-4) || !near(float64(ba2), float64(ba), 1e-4) || !near(float64(bb2), float64(bb), 1e-4) || !near(float64(h.A), 0.25, 1e-6) {
		t.Errorf("transparent to oklch blue: %v", h)
	}
}

func TestLerpScalarsAndLengths(t *testing.T) {
	if Lerp(10, 20, 0.25) != 12.5 || Lerp(10, 20, 0) != 10 || Lerp(10, 20, 1) != 20 {
		t.Error("Lerp")
	}
	if got := Lerp(0, 1, 1.5); got != 1.5 {
		// A curve that overshoots extrapolates; properties clamp what CSS
		// does not allow.
		t.Errorf("Lerp past the end = %v", got)
	}
	lp := LerpLP(style.LP{Px: 10, Pct: 0}, style.LP{Px: 0, Pct: 50}, 0.5)
	if lp != (style.LP{Px: 5, Pct: 25}) {
		t.Errorf("LerpLP = %v", lp)
	}
	d, ok := LerpDim(style.DimPx(100), style.DimPx(200), 0.5)
	if !ok || d != style.DimPx(150) {
		t.Errorf("LerpDim = %v %v", d, ok)
	}
	if d, ok := LerpDim(style.Auto, style.DimPx(200), 0.4); ok || d != style.Auto {
		t.Errorf("auto to px before the middle: %v %v", d, ok)
	}
	if d, ok := LerpDim(style.Auto, style.DimPx(200), 0.6); ok || d != style.DimPx(200) {
		t.Errorf("auto to px after the middle: %v %v", d, ok)
	}
}

func TestLerpTransformAndCompose(t *testing.T) {
	a := IdentityTransform()
	b := IdentityTransform()
	b.TranslateX, b.Rotate, b.ScaleX, b.ScaleY = style.PxLP(20), 90, 2, 2
	m := LerpTransform(a, b, 0.5)
	if m.TranslateX.Px != 10 || m.Rotate != 45 || m.ScaleX != 1.5 {
		t.Errorf("LerpTransform = %+v", m)
	}
	// Adding the animated components to the base is the CSS order for the
	// transforms animations use.
	base := IdentityTransform()
	base.TranslateX = style.PctLP(-50)
	base.Rotate = 45
	over := IdentityTransform()
	over.Rotate = 90
	over.TranslateY = style.PctLP(-25)
	c := ComposeTransform(base, over)
	if c.TranslateX != style.PctLP(-50) || c.TranslateY != style.PctLP(-25) || c.Rotate != 135 || c.ScaleX != 1 {
		t.Errorf("ComposeTransform = %+v", c)
	}
}

func TestLerpShadowsPairsTailwindSlots(t *testing.T) {
	shadow := []style.Shadow{
		{OffsetY: 4, Blur: 6, Spread: -1, Color: gfx.Black.WithAlpha(0.1)},
		{OffsetY: 2, Blur: 4, Spread: -2, Color: gfx.Black.WithAlpha(0.1)},
	}
	ring := style.Shadow{Spread: 2, Color: gfx.OKLCH(0.623, 0.214, 259.815, 1)}
	withRing := append([]style.Shadow{ring}, shadow...)
	// A ring appearing: the shadow layers stay where they are and the
	// ring grows from a transparent one.
	out, ok := LerpShadows(shadow, withRing, 0.5)
	if !ok || len(out) != 3 {
		t.Fatalf("LerpShadows: %v %v", out, ok)
	}
	if out[0].Spread != 1 || out[0].Color.A != 0.5 {
		t.Errorf("ring slot: %+v", out[0])
	}
	if out[1] != shadow[0] || out[2] != shadow[1] {
		t.Errorf("shadow slot moved: %+v %+v", out[1], out[2])
	}
	// No shadow to a shadow: layers grow from nothing.
	out, ok = LerpShadows(nil, shadow, 0.5)
	if !ok || len(out) != 2 || out[0].Blur != 3 || !near(float64(out[0].Color.A), 0.05, 1e-6) {
		t.Errorf("none to shadow: %v", out)
	}
	// An inset shadow meeting an outer one does not interpolate.
	inset := []style.Shadow{{Blur: 4, Inset: true, Color: gfx.Black}}
	if _, ok := LerpShadows(shadow[:1], inset, 0.5); !ok {
		// Different slots: padded pairwise, both fine.
		t.Error("inset and outer shadows of different slots must pair")
	}
	// Endpoints are the inputs.
	if out, _ := LerpShadows(shadow, withRing, 1); len(out) != 3 {
		t.Errorf("t=1: %v", out)
	}
}

func TestLerpFilters(t *testing.T) {
	a := style.FilterSet{Set: style.FilterBlur, Blur: 0}
	b := style.FilterSet{Set: style.FilterBlur | style.FilterBrightness, Blur: 8, Brightness: 1.5}
	m := LerpFilters(a, b, 0.5)
	if m.Blur != 4 || m.Brightness != 1.25 || m.Set != b.Set {
		t.Errorf("LerpFilters = %+v", m)
	}
}

func TestTransitionProgressAndReversing(t *testing.T) {
	tm := Timing{Duration: 300 * time.Millisecond, Delay: 100 * time.Millisecond, Curve: Linear{}}
	tr := Begin(PropOpacity, tm, 1000*time.Millisecond)
	if e, done := tr.Progress(1050 * time.Millisecond); e != 0 || done {
		t.Errorf("during the delay: %v %v", e, done)
	}
	if e, done := tr.Progress(1250 * time.Millisecond); !near(e, 0.5, 1e-9) || done {
		t.Errorf("halfway: %v %v", e, done)
	}
	if e, done := tr.Progress(1400 * time.Millisecond); e != 1 || !done {
		t.Errorf("at the end: %v %v", e, done)
	}
	if (Timing{}).Runs() || !(Timing{Delay: time.Millisecond}).Runs() || !(Timing{Duration: time.Millisecond}).Runs() {
		t.Error("Timing.Runs")
	}
	// Reversing at 50%: the way back takes half the duration.
	noDelay := Timing{Duration: 200 * time.Millisecond, Curve: Linear{}}
	first := Begin(PropOpacity, noDelay, 0)
	rev := first.Reverse(noDelay, 100*time.Millisecond)
	if rev.Duration != 100*time.Millisecond || !near(rev.ReverseFactor, 0.5, 1e-9) || rev.Start != 100*time.Millisecond {
		t.Errorf("reversed transition: %+v", rev)
	}
	// And reversing the reversal at its middle (progress 0.5 of a factor
	// 0.5 transition): factor = 0.5*0.5 + (1-0.5) = 0.75.
	rev2 := rev.Reverse(noDelay, 150*time.Millisecond)
	if !near(rev2.ReverseFactor, 0.75, 1e-9) || rev2.Duration != 150*time.Millisecond {
		t.Errorf("second reversal: %+v", rev2)
	}
}

func TestSpecParseAndProgress(t *testing.T) {
	sp, err := ParseSpec("spin 1s linear infinite")
	if err != nil || sp.Name != "spin" || sp.Duration != time.Second || sp.Curve != (Linear{}) || !sp.Endless() {
		t.Fatalf("spin: %+v %v", sp, err)
	}
	sp, err = ParseSpec("ping 1s cubic-bezier(0, 0, 0.2, 1) infinite")
	if err != nil || sp.Curve != (CubicBezier{0, 0, 0.2, 1}) {
		t.Fatalf("ping: %+v %v", sp, err)
	}
	sp, err = ParseSpec("fade 500ms ease-in 2 .25s 3 alternate both")
	if err != nil || sp.Duration != 500*time.Millisecond || sp.Delay != 250*time.Millisecond || sp.Iterations != 3 ||
		sp.Direction != DirectionAlternate || sp.Fill != FillBoth || sp.Curve != EaseIn || sp.Name != "fade" {
		// "2" is the iteration count then 3 overrides it, as in CSS.
		t.Fatalf("fade: %+v %v", sp, err)
	}
	for _, bad := range []string{"", "a b", "x 1s 2s 3s"} {
		if _, err := ParseSpec(bad); err == nil {
			t.Errorf("ParseSpec(%q) accepted", bad)
		}
	}

	// Progress: 1s, 2 iterations, alternate, delay 500 ms, both fills.
	s := Spec{Duration: time.Second, Iterations: 2, Direction: DirectionAlternate, Delay: 500 * time.Millisecond, Fill: FillBoth}
	for _, c := range []struct {
		at   time.Duration
		want float64
		ph   Phase
	}{
		{0, 0, PhaseBefore}, {500 * time.Millisecond, 0, PhaseActive}, {750 * time.Millisecond, 0.25, PhaseActive},
		{1500 * time.Millisecond, 1, PhaseActive}, {1750 * time.Millisecond, 0.75, PhaseActive},
		{2500 * time.Millisecond, 0, PhaseAfter}, {9 * time.Second, 0, PhaseAfter},
	} {
		p, ph, applies := s.Progress(c.at)
		if !near(p, c.want, 1e-9) || ph != c.ph || !applies {
			t.Errorf("progress at %v = %v %v %v, want %v %v", c.at, p, ph, applies, c.want, c.ph)
		}
	}
	// Without fills the delay and the end have no effect.
	s.Fill = FillNone
	if _, _, applies := s.Progress(0); applies {
		t.Error("no backwards fill: applies in the delay")
	}
	if _, _, applies := s.Progress(3 * time.Second); applies {
		t.Error("no forwards fill: applies after the end")
	}
	if end, ok := s.End(); !ok || end != 2500*time.Millisecond {
		t.Errorf("End = %v %v", end, ok)
	}
	// An infinite spin repeats.
	spin := Spec{Duration: time.Second, Iterations: Infinite}
	if p, ph, _ := spin.Progress(2250 * time.Millisecond); !near(p, 0.25, 1e-9) || ph != PhaseActive {
		t.Errorf("infinite: %v %v", p, ph)
	}
}

func TestBuiltinKeyframes(t *testing.T) {
	spin, _ := Builtin("spin")
	v := spin.At(0.5, Linear{}, 1)
	if !v.HasTransform || v.HasOpacity || !near(float64(v.Transform.Rotate), 180, 1e-4) {
		t.Errorf("spin(0.5) = %+v", v)
	}
	if v := spin.At(1, Linear{}, 1); v.Transform.Rotate != 360 {
		t.Errorf("spin(1) = %+v", v)
	}

	ping, _ := Builtin("ping")
	if v := ping.At(0.375, Linear{}, 1); !near(float64(v.Opacity), 0.5, 1e-6) || !near(float64(v.Transform.ScaleX), 1.5, 1e-6) {
		t.Errorf("ping(0.375) = %+v", v)
	}
	if v := ping.At(0.9, Linear{}, 1); v.Opacity != 0 || v.Transform.ScaleX != 2 {
		t.Errorf("ping(0.9) = %+v", v)
	}

	pulse, _ := Builtin("pulse")
	for _, c := range []struct{ p, want float64 }{{0, 1}, {0.5, 0.5}, {1, 1}} {
		if v := pulse.At(c.p, Linear{}, 1); !v.HasOpacity || v.HasTransform || !near(float64(v.Opacity), c.want, 1e-6) {
			t.Errorf("pulse(%g) = %+v", c.p, v)
		}
	}
	// Its opacity is relative to the box's own opacity at the ends.
	if v := pulse.At(0, Linear{}, 0.8); !near(float64(v.Opacity), 0.8, 1e-6) {
		t.Errorf("pulse(0) with opacity 0.8 = %+v", v)
	}

	bounce, _ := Builtin("bounce")
	if v := bounce.At(0, nil, 1); v.Transform.TranslateY != style.PctLP(-25) {
		t.Errorf("bounce(0) = %+v", v)
	}
	if v := bounce.At(0.5, nil, 1); v.Transform.TranslateY.Pct != 0 {
		t.Errorf("bounce(0.5) = %+v", v)
	}
	if v := bounce.At(1, nil, 1); v.Transform.TranslateY != style.PctLP(-25) {
		t.Errorf("bounce(1) = %+v", v)
	}
	// Each segment uses its keyframe's easing: the first half falls with
	// ease-in (slow start), the second rises with ease-out.
	if v := bounce.At(0.25, nil, 1); !(v.Transform.TranslateY.Pct < -12.5 && v.Transform.TranslateY.Pct > -25) {
		t.Errorf("bounce(0.25) = %+v", v)
	}
	if _, ok := Builtin("wiggle"); ok {
		t.Error("unknown builtin")
	}
}

type counter struct {
	n     int
	until int
	endl  bool
}

func (c *counter) Advance(now time.Duration) State {
	c.n++
	switch {
	case c.endl:
		return Endless
	case c.n >= c.until:
		return Done
	}
	return Running
}

func TestTicker(t *testing.T) {
	var tk Ticker
	if tk.Active() {
		t.Error("an empty ticker is active")
	}
	a, b := &counter{until: 2}, &counter{endl: true}
	tk.Add(a)
	tk.Add(a) // no duplicates
	tk.Add(b)
	if !tk.Active() || tk.Len() != 2 {
		t.Fatalf("after Add: active %v len %d", tk.Active(), tk.Len())
	}
	tk.Advance(10 * time.Millisecond)
	if !tk.Active() || tk.OnlyEndless() {
		t.Errorf("one running: active %v only endless %v", tk.Active(), tk.OnlyEndless())
	}
	tk.Advance(5 * time.Millisecond) // time never goes backwards
	if tk.Now() != 10*time.Millisecond {
		t.Errorf("Now = %v", tk.Now())
	}
	if a.n != 2 || tk.Len() != 1 || !tk.Active() || !tk.OnlyEndless() {
		t.Errorf("a finished: n %d len %d active %v only endless %v", a.n, tk.Len(), tk.Active(), tk.OnlyEndless())
	}
	tk.Remove(b)
	tk.Advance(20 * time.Millisecond)
	if tk.Active() || tk.Len() != 0 || b.n != 2 {
		t.Errorf("after Remove: active %v len %d b.n %d", tk.Active(), tk.Len(), b.n)
	}
}

func TestPropsMapTheTransitionBits(t *testing.T) {
	if got := Set(style.TransitionColors | style.TransitionOpacity | style.TransitionFontSize); len(got) != 7 ||
		got[0] != PropColor || got[5] != PropOpacity || got[6] != PropFontSize {
		t.Errorf("Set = %v", got)
	}
	if len(Set(style.TransitionAll)) != int(NumProps) {
		t.Error("TransitionAll does not cover every property")
	}
	for p := Prop(0); p < NumProps; p++ {
		if p.String() == "unknown" {
			t.Errorf("Prop(%d) has no name", p)
		}
		if p.Mask() == 0 || p.Mask()&(p.Mask()-1) != 0 {
			t.Errorf("%v mask %b is not one bit", p, p.Mask())
		}
	}
	if style.TransitionFontSize != PropFontSize.Mask() || style.TransitionTransform != PropTransform.Mask() {
		t.Error("masks out of step with style.TransitionProps")
	}
}

func TestBlendEqualAndCopy(t *testing.T) {
	var a, b style.ComputedStyle
	a.BackgroundColor, b.BackgroundColor = gfx.White, gfx.Black
	a.Opacity, b.Opacity = 1, 0
	a.Width, b.Width = style.DimPx(100), style.DimPx(300)
	a.Padding = style.AllSides(style.PxLP(4))
	b.Padding = style.AllSides(style.PxLP(8))
	a.Radius = style.Corners[style.LP]{TopLeft: style.PxLP(2)}
	b.Radius = style.Corners[style.LP]{TopLeft: style.PxLP(6)}
	a.Border.Top.Width, b.Border.Top.Width = 1, 3
	a.Text.FontSize, b.Text.FontSize = 10, 20
	var dst style.ComputedStyle
	for p := Prop(0); p < NumProps; p++ {
		Blend(p, &dst, &a, &b, 0.5)
	}
	if !near(float64(dst.BackgroundColor.R), 0.5, 1e-6) || dst.Opacity != 0.5 || dst.Width != style.DimPx(200) ||
		dst.Padding.Left != style.PxLP(6) || dst.Radius.TopLeft != style.PxLP(4) || dst.Border.Top.Width != 2 || dst.Text.FontSize != 15 {
		t.Errorf("Blend: %+v", dst)
	}
	if Equal(PropOpacity, &a, &b) || !Equal(PropOpacity, &a, &a) || Equal(PropSpacing, &a, &b) || !Equal(PropFilter, &a, &b) {
		t.Error("Equal")
	}
	Blend(PropBackgroundColor, &dst, &a, &b, 1)
	if dst.BackgroundColor != b.BackgroundColor {
		t.Error("t=1 is not exact")
	}
	if CanInterpolate(PropWidth, &style.ComputedStyle{}, &b) || !CanInterpolate(PropWidth, &a, &b) {
		t.Error("auto width interpolates")
	}
	if Prop(PropTransform).Class() != ClassLayer || PropShadow.Class() != ClassInk ||
		PropBackgroundColor.Class() != ClassPaint || PropWidth.Class() != ClassLayout {
		t.Error("classes")
	}
}
