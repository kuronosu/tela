package core

import (
	"strings"
	"sync"
	"testing"
)

func TestValueLatest(t *testing.T) {
	var v *Value[int]
	comp := func() Node {
		v = UseValue(0)
		return div(Text("x"))
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	if v.Latest() != 0 {
		t.Fatalf("initial Latest %d", v.Latest())
	}
	v.Set(7)
	if v.Get() != 0 || v.Latest() != 7 {
		t.Errorf("after Set(7): Get %d, Latest %d", v.Get(), v.Latest())
	}
	v.Update(func(n int) int { return n * 2 })
	v.Update(func(n int) int { return n + 1 })
	if v.Get() != 0 || v.Latest() != 15 {
		t.Errorf("after two Updates: Get %d, Latest %d", v.Get(), v.Latest())
	}
	v.Set(1)
	v.Update(func(n int) int { return n + 100 })
	if got := v.Latest(); got != 101 {
		t.Errorf("Set then Update: Latest %d, want 101", got)
	}
	tr.Frame()
	if v.Get() != 101 || v.Latest() != 101 {
		t.Errorf("after the frame: Get %d, Latest %d", v.Get(), v.Latest())
	}
	// Nothing is pending any more.
	if len(v.pend) != 0 {
		t.Errorf("%d operations pending", len(v.pend))
	}
}

func TestValueLatestAppliesInQueueOrderAcrossValues(t *testing.T) {
	// An Update function that reads another Value's Latest sees what was
	// queued before it.
	var a, b *Value[int]
	comp := func() Node {
		a, b = UseValue(1), UseValue(0)
		return div()
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	a.Set(5)
	b.Update(func(int) int { return a.Latest() * 10 })
	if b.Latest() != 50 {
		t.Errorf("Latest %d, want 50", b.Latest())
	}
	tr.Frame()
	if b.Get() != 50 {
		t.Errorf("applied %d, want 50", b.Get())
	}
	// An Update function may call Latest of the Value, which waits on
	// the lock only for a moment (it runs outside it): not from the
	// function being folded, which would recurse, so use another Value.
	b.Update(func(n int) int { return n + a.Latest() })
	if b.Latest() != 55 {
		t.Errorf("Latest %d, want 55", b.Latest())
	}
	tr.Frame()
	if b.Get() != 55 {
		t.Errorf("applied %d, want 55", b.Get())
	}
}

func TestValueLatestDoesNotSubscribe(t *testing.T) {
	builds := 0
	var v *Value[int]
	comp := func() Node {
		builds++
		v = UseValue(0)
		_ = v.Latest()
		return div(Text("x"))
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	v.Set(3)
	tr.Frame()
	if builds != 1 {
		t.Errorf("Latest subscribed the component: %d builds", builds)
	}
	if v.Get() != 3 {
		t.Errorf("Get %d", v.Get())
	}
}

func TestValueLatestFromGoroutines(t *testing.T) {
	var v *Value[int]
	comp := func() Node {
		v = UseValue(0)
		return div()
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	const goroutines, each = 8, 50
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				v.Update(func(n int) int { return n + 1 })
				// Never behind the value applied before it, never ahead of
				// the total. Get must be read first: a Get after Latest may
				// see updates that other goroutines queued (and the UI
				// thread applied) in between, so Latest < Get is possible
				// in that order.
				g := v.Get()
				if l := v.Latest(); l < g || l > goroutines*each {
					t.Errorf("Latest %d, Get %d", l, g)
					return
				}
			}
		}()
	}
	// The UI thread applies updates meanwhile.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
loop:
	for {
		select {
		case <-done:
			break loop
		default:
			tr.ApplyUpdates()
		}
	}
	if got := v.Latest(); got != goroutines*each {
		t.Errorf("Latest %d, want %d", got, goroutines*each)
	}
	tr.Frame()
	if got := v.Get(); got != goroutines*each {
		t.Errorf("Get %d, want %d", got, goroutines*each)
	}
}

func TestValueLatestSetsKeepTheLast(t *testing.T) {
	var v *Value[string]
	comp := func() Node {
		v = UseValue("")
		return div()
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); v.Set(string(rune('a' + i))) }()
	}
	wg.Wait()
	want := v.Latest() // whichever Set queued last
	tr.Frame()
	if got := v.Get(); got != want {
		t.Errorf("applied %q, Latest said %q", got, want)
	}
}

func TestValueLatestOnWrongValue(t *testing.T) {
	var nilValue *Value[int]
	if msg := panicMessage(func() { nilValue.Latest() }); !strings.Contains(msg, "Latest called on a nil *Value") {
		t.Errorf("nil: %q", msg)
	}
	if msg := panicMessage(func() { new(Value[int]).Latest() }); !strings.Contains(msg, "not made by tela.UseValue") {
		t.Errorf("zero: %q", msg)
	}
}
