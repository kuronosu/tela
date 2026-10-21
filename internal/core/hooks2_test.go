package core

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Tests of the hooks of ADR-085: UseMount, UseOnce, UseEffectDo,
// UseUpdateEffect, UseInterval and UseGo.

// frameAndFlush runs a frame and its passive effects.
func frameAndFlush(tr *Tree) {
	tr.Frame()
	tr.FlushEffects()
}

func TestUseMount(t *testing.T) {
	var l effectLog
	var set Setter[int]
	show := true
	child := func() Node {
		n, s := UseState(0)
		set = s
		UseMount(l.effect("mount"))
		UseMount(func() func() { l.add("no cleanup"); return nil })
		return Textf("%d", n)
	}
	root := func() Node { return div(If(show, Component(child))) }
	var logs []string
	tr := devTree(t, Component(root), &logs)
	frameAndFlush(tr)
	expectEvents(t, &l, "mount", "mount", "no cleanup")
	set.Set(1)
	frameAndFlush(tr)
	expectEvents(t, &l, "rebuild")
	show = false
	tr.Mount(Component(root))
	frameAndFlush(tr)
	expectEvents(t, &l, "unmount", "~mount")
	if len(logs) != 0 {
		t.Errorf("logs %q", logs)
	}
	if msg := panicMessage(func() { devTree(t, Component(func() Node { UseMount(nil); return Text("x") }), nil) }); !strings.Contains(msg, "UseMount(nil)") {
		t.Errorf("UseMount(nil): %q", msg)
	}
}

func TestUseOnce(t *testing.T) {
	calls := 0
	var got []*int
	var set Setter[int]
	comp := func() Node {
		p := UseOnce(func() *int { calls++; return new(int) })
		_, s := UseState(0)
		set = s
		got = append(got, p)
		return Text("x")
	}
	tr := devTree(t, Component(comp), nil)
	set.Set(1)
	frameAndFlush(tr)
	set.Set(2)
	frameAndFlush(tr)
	if calls != 1 || len(got) != 3 || got[0] != got[1] || got[1] != got[2] {
		t.Errorf("init ran %d times; values %v", calls, got)
	}
	// Another type in the same slot is a hook-order mistake.
	other := false
	comp2 := func() Node {
		if other {
			UseOnce(func() string { return "" })
		} else {
			UseOnce(func() int { return 0 })
		}
		return Text("x")
	}
	tr = NewTree(Config{Fonts: testFonts})
	tr.Mount(Component(comp2))
	other = true
	if msg := panicMessage(func() { tr.Mount(Component(comp2)) }); !strings.Contains(msg, "UseOnce[int] in the previous build and is UseOnce[string]") {
		t.Errorf("slot mismatch: %q", msg)
	}
}

func TestUseEffectDoAndUpdateEffect(t *testing.T) {
	var l effectLog
	n := 0
	comp := func() Node {
		UseEffectDo(func() { l.add("do %d", n) }, n)
		UseUpdateEffect(l.effect("update"), n)
		UseUpdateEffect(func() func() { l.add("every update"); return nil })
		return Text("x")
	}
	tr := devTree(t, Component(comp), nil)
	frameAndFlush(tr)
	expectEvents(t, &l, "mount", "do 0") // no update effect on mount
	tr.Mount(Component(comp))
	frameAndFlush(tr)
	expectEvents(t, &l, "same deps", "every update")
	n = 1
	tr.Mount(Component(comp))
	frameAndFlush(tr)
	expectEvents(t, &l, "changed", "do 1", "update", "every update")
	n = 2
	tr.Mount(Component(comp))
	frameAndFlush(tr)
	expectEvents(t, &l, "changed again", "~update", "do 2", "update", "every update")
	tr.Unmount()
	expectEvents(t, &l, "unmount", "~update")
}

func TestUseInterval(t *testing.T) {
	var calls []string
	d := 100 * time.Millisecond
	label := "a"
	show := true
	child := func() Node {
		l := label // f is the latest build's closure
		UseInterval(d, func() { calls = append(calls, l) })
		return Text("x")
	}
	root := func() Node { return div(If(show, Component(child))) }
	tr := devTree(t, Component(root), nil)
	at := func(ms int) {
		now := time.Duration(ms) * time.Millisecond
		tr.Animate(now)
		tr.TickGestures(now)
	}
	at(1000)
	frameAndFlush(tr) // the effect starts the interval at 1000 ms
	if dl, ok := tr.NextDeadline(); !ok || dl != 1100*time.Millisecond {
		t.Fatalf("deadline %v %v, want 1.1s", dl, ok)
	}
	at(1050)
	at(1100)
	label = "b"
	tr.Mount(Component(root))
	frameAndFlush(tr)
	at(1200)
	at(1550) // three periods late: one call, the next at 1600
	if want := "a b b"; strings.Join(calls, " ") != want {
		t.Errorf("calls %q, want %q", calls, want)
	}
	if dl, _ := tr.NextDeadline(); dl != 1600*time.Millisecond {
		t.Errorf("deadline after a late tick %v, want 1.6s", dl)
	}
	// A new period restarts from the frame that shows it; <= 0 pauses.
	calls = nil
	d = 50 * time.Millisecond
	tr.Mount(Component(root))
	at(1560)
	frameAndFlush(tr)
	if dl, _ := tr.NextDeadline(); dl != 1610*time.Millisecond {
		t.Errorf("deadline after a new period %v, want 1.61s", dl)
	}
	d = 0
	tr.Mount(Component(root))
	frameAndFlush(tr)
	if dl, ok := tr.NextDeadline(); ok {
		t.Errorf("paused interval has a deadline %v", dl)
	}
	at(3000)
	d = time.Second
	tr.Mount(Component(root))
	frameAndFlush(tr)
	if dl, _ := tr.NextDeadline(); dl != 4000*time.Millisecond {
		t.Errorf("deadline after resuming %v, want 4s", dl)
	}
	// Unmounting stops it.
	show = false
	tr.Mount(Component(root))
	frameAndFlush(tr)
	at(10000)
	if len(calls) != 0 {
		t.Errorf("calls %q after pausing and unmounting", calls)
	}
	if _, ok := tr.NextDeadline(); ok || len(tr.intervals) != 0 {
		t.Errorf("an unmounted interval is still scheduled (%d)", len(tr.intervals))
	}
}

// TestUseIntervalLongGap: after a long gap (a suspend) a short interval
// fires once and jumps to the first period boundary after now in one
// step, instead of looping once per missed period on the UI thread.
func TestUseIntervalLongGap(t *testing.T) {
	calls := 0
	comp := func() Node {
		UseInterval(time.Nanosecond, func() { calls++ })
		return Text("x")
	}
	tr := devTree(t, Component(comp), nil)
	tr.Animate(time.Second)
	frameAndFlush(tr) // the interval starts at 1 s, next at 1 s + 1 ns
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Ten hours later: 3.6e13 missed periods.
		now := time.Second + 10*time.Hour + 7
		tr.Animate(now)
		tr.TickGestures(now)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the late tick did not return: it catches up one period at a time")
	}
	if calls != 1 {
		t.Errorf("%d calls, want 1 (once, not once per missed period)", calls)
	}
	if dl, _ := tr.NextDeadline(); dl != time.Second+10*time.Hour+8 {
		t.Errorf("deadline %v, want the first boundary after now (1s + 10h + 8ns)", dl)
	}
}

func TestUseGo(t *testing.T) {
	type run struct {
		id        int
		cancelled chan struct{}
	}
	runs := make(chan run, 8)
	var started atomic.Int32
	id := 1
	show := true
	child := func() Node {
		myID := id
		UseGo(func(ctx context.Context) {
			started.Add(1)
			r := run{id: myID, cancelled: make(chan struct{})}
			runs <- r
			<-ctx.Done()
			close(r.cancelled)
		}, id)
		return Text("x")
	}
	root := func() Node { return div(If(show, Component(child))) }
	tr := devTree(t, Component(root), nil)
	frameAndFlush(tr)
	first := <-runs
	if first.id != 1 {
		t.Errorf("first run id %d", first.id)
	}
	tr.Mount(Component(root)) // same deps: no new run
	frameAndFlush(tr)
	id = 2
	tr.Mount(Component(root))
	frameAndFlush(tr)
	waitClosed(t, first.cancelled, "the first run's ctx was not cancelled when the deps changed")
	second := <-runs
	if second.id != 2 {
		t.Errorf("second run id %d", second.id)
	}
	show = false
	tr.Mount(Component(root))
	frameAndFlush(tr)
	waitClosed(t, second.cancelled, "the ctx was not cancelled at unmount")
	if n := started.Load(); n != 2 {
		t.Errorf("%d runs, want 2", n)
	}
}

func waitClosed(t *testing.T, ch chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal(msg)
	}
}
