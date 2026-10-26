package core

import (
	"strings"
	"testing"
	"time"
)

// Tests of UseTimeoutAfter, the engine of tela.UseTimeout (ADR-171).

// timeoutRig mounts a component that calls UseTimeoutAfter with the
// rig's d, deps and label, on a tree whose clock the test moves.
type timeoutRig struct {
	t     *testing.T
	tr    *Tree
	d     time.Duration
	dep   string
	label string
	show  bool
	calls []string
	// rebuild mounts the root again: the component builds with the rig's
	// current values.
	rebuild func()
}

func newTimeoutRig(t *testing.T, d time.Duration) *timeoutRig {
	r := &timeoutRig{t: t, d: d, dep: "x", label: "a", show: true}
	child := func() Node {
		l := r.label // f is the latest build's closure
		UseTimeoutAfter(r.d, func() { r.calls = append(r.calls, l) }, r.dep)
		return Text("x")
	}
	root := func() Node { return div(If(r.show, Component(child))) }
	r.tr = devTree(t, Component(root), nil)
	r.rebuild = func() { r.tr.Mount(Component(root)) }
	return r
}

func (r *timeoutRig) at(ms int) {
	now := time.Duration(ms) * time.Millisecond
	r.tr.Animate(now)
	r.tr.TickGestures(now)
}

// build rebuilds the component and runs the effects at ms.
func (r *timeoutRig) build(ms int) {
	r.at(ms)
	r.rebuild()
	frameAndFlush(r.tr)
}

func (r *timeoutRig) deadline() (time.Duration, bool) { return r.tr.NextDeadline() }

func TestUseTimeoutAfterFiresOnce(t *testing.T) {
	r := newTimeoutRig(t, 300*time.Millisecond)
	r.at(1000)
	frameAndFlush(r.tr) // the effect starts the timer at 1000 ms
	if dl, ok := r.deadline(); !ok || dl != 1300*time.Millisecond {
		t.Fatalf("deadline %v %v, want 1.3s", dl, ok)
	}
	r.at(1299)
	if len(r.calls) != 0 {
		t.Fatalf("fired early: %q", r.calls)
	}
	r.at(1300)
	if len(r.calls) != 1 {
		t.Fatalf("calls %q, want one at d", r.calls)
	}
	// Idle after firing: nothing waits, so no timer is armed.
	if dl, ok := r.deadline(); ok || len(r.tr.intervals) != 0 {
		t.Errorf("a fired timeout still waits: %v %v (%d scheduled)", dl, ok, len(r.tr.intervals))
	}
	r.at(5000)
	r.build(5000) // same d and deps: it does not restart
	r.at(9000)
	if len(r.calls) != 1 {
		t.Errorf("calls %q after rebuilds with the same d and deps, want one per mount", r.calls)
	}
	if _, ok := r.deadline(); ok {
		t.Error("a rebuild with the same deps restarted the timeout")
	}
}

func TestUseTimeoutAfterNonPositiveNeverFires(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second} {
		r := newTimeoutRig(t, d)
		r.at(1000)
		frameAndFlush(r.tr)
		if _, ok := r.deadline(); ok {
			t.Errorf("d=%v: a timeout is pending", d)
		}
		r.at(100000)
		if len(r.calls) != 0 {
			t.Errorf("d=%v fired: %q", d, r.calls)
		}
	}
	// d <= 0 drops a pending timer.
	r := newTimeoutRig(t, time.Second)
	r.at(0)
	frameAndFlush(r.tr)
	if _, ok := r.deadline(); !ok {
		t.Fatal("no timer pending")
	}
	r.d = 0
	r.build(100)
	if _, ok := r.deadline(); ok {
		t.Error("d=0 left the timer pending")
	}
	r.at(10000)
	if len(r.calls) != 0 {
		t.Errorf("fired after d=0: %q", r.calls)
	}
}

func TestUseTimeoutAfterRestartsOnD(t *testing.T) {
	r := newTimeoutRig(t, time.Second)
	r.at(0)
	frameAndFlush(r.tr)
	r.at(400)
	r.d = 2 * time.Second
	r.build(500) // restarts from the frame that shows the change
	if dl, _ := r.deadline(); dl != 2500*time.Millisecond {
		t.Errorf("deadline %v after a new d, want 2.5s", dl)
	}
	r.at(1500) // the old deadline passed
	if len(r.calls) != 0 {
		t.Fatalf("the old timer fired: %q", r.calls)
	}
	r.at(2500)
	if len(r.calls) != 1 {
		t.Fatalf("calls %q, want one", r.calls)
	}
	// A new d after it fired runs it again.
	r.d = time.Second
	r.build(3000)
	r.at(4000)
	if len(r.calls) != 2 {
		t.Errorf("calls %q, want a second run after the new d", r.calls)
	}
}

func TestUseTimeoutAfterRestartsOnDeps(t *testing.T) {
	r := newTimeoutRig(t, time.Second)
	r.at(0)
	frameAndFlush(r.tr)
	r.at(1000)
	if len(r.calls) != 1 {
		t.Fatalf("calls %q, want one", r.calls)
	}
	r.rebuild()
	frameAndFlush(r.tr) // same deps: nothing
	if _, ok := r.deadline(); ok {
		t.Fatal("same deps restarted it")
	}
	r.dep = "y"
	r.build(2000) // changed deps, after it fired
	if dl, ok := r.deadline(); !ok || dl != 3000*time.Millisecond {
		t.Fatalf("deadline %v %v, want 3s", dl, ok)
	}
	r.at(3000)
	if len(r.calls) != 2 {
		t.Errorf("calls %q, want a second run after the deps changed", r.calls)
	}
	// Changed deps before it fires replace the pending run.
	r.dep = "z"
	r.build(4000)
	r.dep = "w"
	r.build(4500)
	r.at(5000)
	if len(r.calls) != 2 {
		t.Errorf("the replaced run fired: %q", r.calls)
	}
	r.at(5500)
	if len(r.calls) != 3 {
		t.Errorf("calls %q, want one more at 5.5s", r.calls)
	}
}

func TestUseTimeoutAfterLatestFunction(t *testing.T) {
	r := newTimeoutRig(t, time.Second)
	r.at(0)
	frameAndFlush(r.tr)
	r.label = "b"
	r.rebuild() // same deps: no restart, but f is the new closure
	frameAndFlush(r.tr)
	r.at(1000)
	if got := strings.Join(r.calls, " "); got != "b" {
		t.Errorf("calls %q, want the latest build's function (b)", got)
	}
}

func TestUseTimeoutAfterUnmountCancels(t *testing.T) {
	r := newTimeoutRig(t, time.Second)
	r.at(0)
	frameAndFlush(r.tr)
	r.show = false
	r.build(500)
	if _, ok := r.deadline(); ok || len(r.tr.intervals) != 0 {
		t.Errorf("an unmounted timeout is still scheduled (%d)", len(r.tr.intervals))
	}
	r.at(10000)
	if len(r.calls) != 0 {
		t.Errorf("fired after unmount: %q", r.calls)
	}
	// A mount again is a new timer, from its own frame.
	r.show = true
	r.build(11000)
	r.at(12000)
	if len(r.calls) != 1 {
		t.Errorf("calls %q after mounting again, want one", r.calls)
	}
}

// A function may set state or unmount its own component; ticking is safe.
func TestUseTimeoutAfterSetsState(t *testing.T) {
	var set Setter[bool]
	shown := false
	comp := func() Node {
		v, s := UseState(true)
		set = s
		shown = v
		UseTimeoutAfter(time.Second, func() { set.Set(false) }, "m")
		return Text("x")
	}
	tr := devTree(t, Component(comp), nil)
	tr.Animate(0)
	frameAndFlush(tr)
	tr.Animate(time.Second)
	tr.TickGestures(time.Second)
	frameAndFlush(tr)
	if shown {
		t.Error("the state set by the timeout was not applied")
	}
	if _, ok := tr.NextDeadline(); ok {
		t.Error("the fired timeout keeps the scheduler awake")
	}
}

func TestUseTimeoutAfterNilAndSlotMismatch(t *testing.T) {
	comp := func() Node {
		UseTimeoutAfter(time.Second, nil)
		return Text("x")
	}
	if msg := panicMessage(func() { devTree(t, Component(comp), nil) }); !strings.Contains(msg, "UseTimeout(d, nil)") {
		t.Errorf("panic %q", msg)
	}
	// The one-shot Timeout of the aria hooks and UseTimeoutAfter are
	// different hooks.
	useAfter := true
	swap := func() Node {
		if useAfter {
			UseTimeoutAfter(time.Second, func() {})
		} else {
			UseTimeout()
		}
		return Text("x")
	}
	tr := devTree(t, Component(swap), nil)
	frameAndFlush(tr)
	useAfter = false
	if msg := panicMessage(func() { tr.Mount(Component(swap)); frameAndFlush(tr) }); !strings.Contains(msg, "hook order") {
		t.Errorf("panic %q, want the hook order message", msg)
	}
}
