package anim

import (
	"testing"
	"time"
)

func TestTickerRemoveStopsRequestingFrames(t *testing.T) {
	var tk Ticker
	a := &counter{until: 100}
	tk.Add(a)
	tk.Advance(time.Millisecond)
	if !tk.Active() {
		t.Fatal("a running animator does not keep the ticker active")
	}
	tk.Remove(a)
	if tk.Active() || tk.OnlyEndless() || tk.Len() != 0 {
		t.Errorf("after Remove: active %v only endless %v len %d", tk.Active(), tk.OnlyEndless(), tk.Len())
	}
	// An animator added while advancing joins the same pass.
	b := &adder{tk: &tk}
	tk.Add(b)
	tk.Advance(2 * time.Millisecond)
	if b.child == nil || b.child.n != 1 || !tk.Active() {
		t.Errorf("child %+v, active %v", b.child, tk.Active())
	}
}

type adder struct {
	tk    *Ticker
	child *counter
}

func (a *adder) Advance(time.Duration) State {
	if a.child == nil {
		a.child = &counter{until: 10}
		a.tk.Add(a.child)
	}
	return Done
}
