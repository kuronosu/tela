package anim

import (
	"math"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

func vec1(x float64) Vec { return Vec{N: 1, C: [4]float64{x}} }

func TestMotionTween(t *testing.T) {
	spec := MotionSpec{Duration: 200 * time.Millisecond, Curve: Linear{}}
	m := NewMotion(spec, vec1(10), vec1(30), Vec{}, time.Second)
	if x, _, done := m.At(time.Second); x.C[0] != 10 || done {
		t.Fatalf("at the start: %v done %v", x, done)
	}
	x, v, done := m.At(time.Second + 50*time.Millisecond)
	if math.Abs(x.C[0]-15) > 1e-9 || done {
		t.Fatalf("a quarter: %v", x)
	}
	if math.Abs(v.C[0]-100) > 1e-6 { // 20 units in 0.2 s
		t.Errorf("velocity %v, want 100/s", v.C[0])
	}
	if x, v, done := m.At(time.Second + 200*time.Millisecond); x.C[0] != 30 || v.C[0] != 0 || !done {
		t.Fatalf("at the end: %v %v %v", x, v, done)
	}
	// An eased tween follows its curve.
	e := NewMotion(MotionSpec{Duration: time.Second, Curve: Ease}, vec1(0), vec1(1), Vec{}, 0)
	if x, _, _ := e.At(500 * time.Millisecond); math.Abs(x.C[0]-Ease.At(0.5)) > 1e-12 {
		t.Errorf("eased: %v, want %v", x.C[0], Ease.At(0.5))
	}
	// A tween of no duration is over at once.
	z := NewMotion(MotionSpec{}, vec1(0), vec1(5), Vec{}, 0)
	if x, _, done := z.At(time.Nanosecond); !done || x.C[0] != 5 {
		t.Errorf("zero duration: %v %v", x, done)
	}
}

// TestMotionSpringRetarget: a spring that replaces another at time T
// starts at the position and with the velocity the first had at T, so
// the value and its speed are continuous (Motion, Compose).
func TestMotionSpringRetarget(t *testing.T) {
	spec := MotionSpec{Spring: true, Physics: Spring{Stiffness: 300, Damping: 30, Mass: 1}}
	a := NewMotion(spec, vec1(0), vec1(200), Vec{}, 0)
	at := 120 * time.Millisecond
	x, v, done := a.At(at)
	if done || v.C[0] <= 0 {
		t.Fatalf("not moving at %v: %v %v", at, x, v)
	}
	b := NewMotion(spec, x, vec1(-100), v, at)
	bx, bv, _ := b.At(at)
	if bx.C[0] != x.C[0] || bv.C[0] != v.C[0] {
		t.Fatalf("at the retarget: %v %v, want %v %v", bx, bv, x, v)
	}
	// A millisecond later both still agree to first order.
	ax, _, _ := a.At(at + time.Millisecond)
	bx, _, _ = b.At(at + time.Millisecond)
	if d := math.Abs(ax.C[0] - bx.C[0]); d > 0.05 {
		t.Errorf("1 ms after the retarget the paths differ by %v", d)
	}
	// The second run keeps going the old way for a while (the velocity
	// carries it) before it turns toward its target.
	if bx.C[0] <= x.C[0] {
		t.Errorf("the velocity was lost: %v then %v", x.C[0], bx.C[0])
	}
	// It settles exactly on the target.
	var end time.Duration
	for now := at; now < at+5*time.Second; now += time.Second / 60 {
		if p, _, done := b.At(now); done {
			if p.C[0] != -100 {
				t.Fatalf("ended at %v", p)
			}
			end = now
			break
		}
	}
	if end == 0 {
		t.Fatal("the spring never settled")
	}
}

func TestMotionSpringComponents(t *testing.T) {
	// Each component has its own distance, rest thresholds and velocity.
	spec := MotionSpec{Spring: true, Physics: DefaultSpring()}
	from := Vec{N: 2, C: [4]float64{0, 0}}
	to := Vec{N: 2, C: [4]float64{100, 1}}
	m := NewMotion(spec, from, to, Vec{}, 0)
	x, _, _ := m.At(100 * time.Millisecond)
	if !(x.C[0] > 0 && x.C[1] > 0) || math.Abs(x.C[0]/100-x.C[1]) > 1e-9 {
		t.Errorf("components without velocity follow the same normalized path: %v", x)
	}
	if m.rest[0] != DefaultRest(100) || m.rest[1] != DefaultRest(1) {
		t.Errorf("rest thresholds %v", m.rest)
	}
}

func TestMotionSpringMinDuration(t *testing.T) {
	s := SpringForDuration(300*time.Millisecond, 0)
	spec := MotionSpec{Spring: true, Physics: s, MinDuration: 300 * time.Millisecond}
	m := NewMotion(spec, vec1(0), vec1(1), Vec{}, 0)
	if _, _, done := m.At(299 * time.Millisecond); done {
		t.Error("ended before its duration")
	}
	var end time.Duration
	for now := time.Duration(0); now < 2*time.Second; now += time.Millisecond {
		if _, _, done := m.At(now); done {
			end = now
			break
		}
	}
	if end < 300*time.Millisecond || end > 400*time.Millisecond {
		t.Errorf("a critically damped 300 ms spring ended at %v", end)
	}
}

func TestMotionUndampedEnds(t *testing.T) {
	m := NewMotion(MotionSpec{Spring: true, Physics: Spring{Stiffness: 100, Damping: 0}}, vec1(0), vec1(10), Vec{}, 0)
	if _, _, done := m.At(19 * time.Second); done {
		t.Error("an undamped spring settled")
	}
	if x, _, done := m.At(20 * time.Second); !done || x.C[0] != 10 {
		t.Errorf("an undamped spring does not end at 20 s: %v %v", x, done)
	}
}

func TestColorVec(t *testing.T) {
	legacy := []gfx.Color{gfx.RGB8(255, 0, 0), gfx.RGB8(30, 58, 138), {R: 1, G: 1, B: 1, A: 0.5}}
	oklch := gfx.OKLCH(0.623, 0.214, 259.815, 1)
	if ColorsOklab(legacy[0], legacy[1]) || !ColorsOklab(legacy[0], oklch) {
		t.Error("the interpolation space of a pair")
	}
	for _, c := range append(legacy, oklch, gfx.Transparent) {
		for _, ok := range []bool{false, true} {
			got := VecColor(ColorVec(c, ok), ok)
			if c.A == 0 {
				if got != gfx.Transparent {
					t.Errorf("transparent -> %v", got)
				}
				continue
			}
			if !near32(got.R, c.R, 1e-5) || !near32(got.G, c.G, 1e-5) || !near32(got.B, c.B, 1e-5) || !near32(got.A, c.A, 1e-6) {
				t.Errorf("%v (oklab %v) -> %v", c, ok, got)
			}
		}
	}
	// A linear motion of the components is LerpColor.
	for _, pair := range [][2]gfx.Color{{legacy[0], legacy[1]}, {legacy[2], oklch}, {oklch, gfx.Transparent}} {
		a, b := pair[0], pair[1]
		ok := ColorsOklab(a, b)
		m := NewMotion(MotionSpec{Duration: time.Second, Curve: Linear{}}, ColorVec(a, ok), ColorVec(b, ok), Vec{}, 0)
		for _, p := range []float64{0.25, 0.5, 0.8} {
			x, _, _ := m.At(time.Duration(p * float64(time.Second)))
			got, want := VecColor(x, ok), LerpColor(a, b, p)
			if !near32(got.R, want.R, 1e-5) || !near32(got.G, want.G, 1e-5) || !near32(got.B, want.B, 1e-5) || !near32(got.A, want.A, 1e-5) {
				t.Errorf("%v -> %v at %v: %v, LerpColor %v", a, b, p, got, want)
			}
		}
	}
}

func near32(a, b, eps float32) bool { return math.Abs(float64(a-b)) <= float64(eps) }

func TestVecSame(t *testing.T) {
	n := math.NaN()
	if !vec1(n).Same(vec1(n)) || vec1(1).Same(vec1(2)) || vec1(1).Same(Vec{N: 2, C: [4]float64{1}}) {
		t.Error("Same")
	}
	if vec1(n).Finite() || vec1(math.Inf(1)).Finite() || !vec1(3).Finite() {
		t.Error("Finite")
	}
}
