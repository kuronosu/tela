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
	// once makes the slot of a Timeout: it runs f once, d after Start.
	once bool
	// byEffect marks the once slot of UseTimeoutAfter, whose eff starts it
	// (a Timeout's slot has no effect: its handlers start it).
	byEffect bool
	f        func() // the latest build's
	d        time.Duration
	// next is when f runs next (tree clock); on whether the interval
	// runs (in the tree's list).
	next time.Duration
	on   bool
}

// kind returns the hook that made the slot: UseInterval, UseTimeoutAfter
// (tela.UseTimeout) or UseTimeout (aria's Timeout). Only a call of the
// same hook takes the slot in a later build.
func (s *intervalSlot) kind() hookKind {
	switch {
	case s.byEffect:
		return hookTimeoutAfter
	case s.once:
		return hookTimeout
	}
	return hookInterval
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
		if s, ok = el.hooks[i].(*intervalSlot); !ok || s.kind() != hookInterval {
			el.tree.slotMismatch(el, i, hookInterval.String())
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
		if s.once {
			s.stop()
			s.f()
			continue
		}
		// The first period boundary after now, in one step: after a
		// suspend, a 1 ms interval would otherwise loop once per missed
		// period on the UI thread.
		s.next += ((now-s.next)/s.d + 1) * s.d
		s.f()
	}
}

// UseTimeoutAfter (the engine of tela.UseTimeout, ADR-171) calls f once on
// the UI thread, d after the frame that shows the component, on the same
// frame clock as UseInterval. d <= 0 cancels it; a change of d or of deps
// (compared like UseEffect's) restarts it from the frame that shows the
// change, even if it already fired; f is always the latest build's. It is a
// different hook from UseTimeout, whose Timeout the caller starts itself.
func UseTimeoutAfter(d time.Duration, f func(), deps ...any) {
	if f == nil {
		panic("tela: UseTimeout(d, nil): pass the function to call once, d after the component shows")
	}
	el, i := nextSlot(hookTimeoutAfter)
	// d goes last so that dev warnings number the caller's dependencies.
	all := append(slices.Clone(deps), d)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*intervalSlot)
		if !ok || s.kind() != hookTimeoutAfter {
			el.tree.slotMismatch(el, i, hookTimeoutAfter.String())
		}
		s.f = f
		s.eff.update(i, s.effect(d), all)
		return
	}
	el.tree.checkDeps(el, i, all)
	s := &intervalSlot{f: f, once: true, byEffect: true}
	s.eff = effectSlot{el: el, kind: hookTimeoutAfter, effect: s.effect(d), deps: all, pending: true}
	el.fx = append(el.fx, &s.eff)
	el.hooks = append(el.hooks, s)
}

// Timeout is a one-shot timer on the frame clock of the tree, made by
// UseTimeout: Start arms it, and its function runs once on the UI thread
// when the time comes, from TickGestures like UseInterval's. Starting it
// again replaces the pending run. It stops when the component unmounts.
// Its methods are for handlers and effects on the UI thread.
type Timeout struct{ s *intervalSlot }

// UseTimeout returns the Timeout of this hook slot, the same on every
// build. It is for package aria's tooltips: delays that a hover starts and
// a leave cancels, from the handler itself and not from the next effect.
func UseTimeout() Timeout {
	el, i := nextSlot(hookTimeout)
	if i < len(el.hooks) {
		s, ok := el.hooks[i].(*intervalSlot)
		if !ok || s.kind() != hookTimeout {
			el.tree.slotMismatch(el, i, hookTimeout.String())
		}
		return Timeout{s}
	}
	s := &intervalSlot{once: true}
	s.eff.el = el
	el.hooks = append(el.hooks, s)
	return Timeout{s}
}

// Start runs f once, d from now on the frame clock (at the next tick for
// d <= 0), replacing the pending run.
func (o Timeout) Start(d time.Duration, f func()) {
	if o.s == nil || !o.s.eff.el.mounted {
		return
	}
	if f == nil {
		panic("tela: Timeout.Start(d, nil): pass the function to call")
	}
	s := o.s
	s.stop()
	t := s.eff.el.tree
	d = max(d, 0)
	s.f, s.d, s.next, s.on = f, max(d, 1), t.clock()+d, true
	t.intervals = append(t.intervals, s)
}

// Stop cancels the pending run, if any.
func (o Timeout) Stop() {
	if o.s != nil {
		o.s.stop()
	}
}

// Pending reports whether a run is waiting.
func (o Timeout) Pending() bool { return o.s != nil && o.s.on }
