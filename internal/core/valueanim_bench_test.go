package core

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestUseSpringUnchangedTargetIsCheap: a rebuild whose targets do not
// change does not resolve the options (a spring defined by its duration
// solves for its physics and allocates): 200 UseSprings add no
// allocations to the rebuild.
func TestUseSpringUnchangedTargetIsCheap(t *testing.T) {
	opts := []AnimOption{Initial(float32(0)), Duration(500 * time.Millisecond), Bounce(0.2)}
	allocs := func(springs int) float64 {
		var set Setter[int]
		tr := newTestTree(t, 200, 100, Component(func() Node {
			n, s := UseState(0)
			set = s
			for range springs {
				UseSpring(float32(1), opts...)
			}
			return div(Class("size-4"), Textf("%d", n))
		}), nil)
		if tr.dev {
			t.Skip("dev mode resolves the options on every build")
		}
		tr.Frame()
		i := 0
		return testing.AllocsPerRun(50, func() {
			i++
			set.Set(i)
			tr.Frame()
		})
	}
	base, with := allocs(0), allocs(200)
	if with-base > 20 {
		t.Errorf("200 UseSprings with unchanged targets add %v allocations per rebuild (%v -> %v)", with-base, base, with)
	}
}

// TestUseSpringDevChecksEveryBuild: in dev mode a mistake in the options
// panics on the build that makes it, even when the target holds.
func TestUseSpringDevChecksEveryBuild(t *testing.T) {
	var set Setter[bool]
	tr := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(string) {}})
	tr.Mount(Component(func() Node {
		bad, s := UseState(false)
		set = s
		opts := []AnimOption{Initial(float32(0)), Duration(time.Second)}
		if bad {
			opts = append(opts, Ease("linear"))
		}
		UseSpring(float32(1), opts...)
		return div()
	}))
	tr.Frame()
	set.Set(true)
	defer func() {
		if msg := fmt.Sprint(recover()); !strings.Contains(msg, "Ease applies to tweens") {
			t.Errorf("panic %q, want the Ease mistake", msg)
		}
	}()
	tr.Frame()
}

// BenchmarkUseSpring200Unchanged rebuilds a component with 200 UseSpring
// calls (springs defined by Duration and Bounce, the costliest options to
// resolve) and 200 UseTween calls whose targets do not change: the hooks
// only compare the target.
//
//	go test -run xxx -bench UseSpring200 -benchmem ./internal/core/
func BenchmarkUseSpring200Unchanged(b *testing.B) {
	var set Setter[int]
	tr := newTestTree(b, 200, 100, Component(func() Node {
		n, s := UseState(0)
		set = s
		for range 200 {
			UseSpring(float32(1), Initial(float32(0)), Duration(500*time.Millisecond), Bounce(0.2))
			UseTween(float32(1), Initial(float32(0)), Duration(200*time.Millisecond), Ease("ease-out"))
		}
		return div(Class("size-4"), Textf("%d", n))
	}), nil)
	tr.Frame()
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		i++
		set.Set(i)
		tr.Frame()
	}
}
