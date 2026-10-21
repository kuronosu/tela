package core

import (
	"slices"
	"time"
)

// UseInterval (ADR-085) calls a function on the UI thread at a fixed
// period, on the frame clock of the tree, like the long-press and caret
// timers: NextDeadline reports the earliest interval due, a Host arms its
// one timer for it, and TickGestures runs the due functions. No goroutine
// is involved, an idle interval costs one timer, and telatest's Advance
// moves it deterministically. A tree that no clock drives (Render) never
// runs it.

// intervalSlot is the slot of one UseInterval call. Its effect starts the
// interval after the frame that shows the component, and again when the
// period changes; its cleanup stops it.
type intervalSlot struct {
	eff effectSlot
	f   func() // the latest build's
	d   time.Duration
	// next is when f runs next (tree clock); on whether the interval
	// runs (in the tree's list).
	next time.Duration
	on   bool
}

func (s *intervalSlot) unmount(t *Tree) {
	s.stop()
	s.eff.unmount(t)
}

// start starts the interval with period d (d <= 0: paused).
func (s *intervalSlot) start(d time.Duration) {
	s.stop()
	if d <= 0 {
		return
	}
	t := s.eff.el.tree
	s.d, s.next, s.on = d, t.clock()+d, true
	t.intervals = append(t.intervals, s)
}

// stop removes the interval from its tree's list.
func (s *intervalSlot) stop() {
	if !s.on {
		return
	}
	s.on = false
	t := s.eff.el.tree
	if i := slices.Index(t.intervals, s); i >= 0 {
		t.intervals = slices.Delete(t.intervals, i, i+1)
	}
}

// UseInterval calls f on the UI thread every d, starting d after the
// frame that first shows the component; d <= 0 pauses it, and a new d
// restarts it from the frame that shows the change. f is always the
// latest build's function, so it sees the latest state without
// dependencies.
func UseInterval(d time.Duration, f func()) {
	if f == nil {
		panic("tela: UseInterval(d, nil): pass the function to call every d")
	}
	el, i := nextSlot(hookInterval)
	var s *intervalSlot
	if i < len(el.hooks) {
		var ok bool
		if s, ok = el.hooks[i].(*intervalSlot); !ok {
			el.tree.slotMismatch(el, i, "UseInterval")
		}
		s.f = f
		s.eff.update(i, s.effect(d), []any{d})
		return
	}
	s = &intervalSlot{f: f}
	s.eff = effectSlot{el: el, kind: hookInterval, effect: s.effect(d), deps: []any{d}, pending: true}
	el.fx = append(el.fx, &s.eff)
	el.hooks = append(el.hooks, s)
}

// effect returns the effect that starts the interval with period d.
func (s *intervalSlot) effect(d time.Duration) func() func() {
	return func() func() {
		s.start(d)
		return s.stop
	}
}

// intervalDeadline returns when the next interval is due.
func (t *Tree) intervalDeadline() (at time.Duration, ok bool) {
	for _, s := range t.intervals {
		if !ok || s.next < at {
			at, ok = s.next, true
		}
	}
	return at, ok
}

// tickIntervals runs the functions of the intervals due at now, each once
// however many periods passed (like a time.Ticker that drops ticks), and
// schedules their next run.
func (t *Tree) tickIntervals(now time.Duration) {
	if len(t.intervals) == 0 {
		return
	}
	// A function may start or stop intervals, or unmount components.
	for _, s := range slices.Clone(t.intervals) { // perf: allocates per tick
		if !s.on || !s.eff.el.mounted || now < s.next {
			continue
		}
		for s.next <= now {
			s.next += s.d
		}
		s.f()
	}
}
