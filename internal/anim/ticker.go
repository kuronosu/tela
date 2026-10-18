package anim

import "time"

// State is what an Animator reports after advancing.
type State uint8

const (
	// Done: the animator finished and leaves the ticker.
	Done State = iota
	// Running: it needs more frames, and will finish.
	Running
	// Endless: it needs frames for as long as it exists (an infinite
	// animation). A ticker that only holds endless animators keeps asking
	// for frames, but a test has nothing to wait for.
	Endless
)

// Animator is something the Ticker advances with the frame clock.
type Animator interface {
	// Advance moves the animator to now, the time of the frame. It may add
	// animators to the ticker (they are advanced in the same pass).
	Advance(now time.Duration) State
}

// Ticker advances animators with the frame clock (ARCHITECTURE section 8,
// phase 2). Time never comes from the wall clock: the scheduler passes the
// time of each frame, so a headless clock controls every animation. The
// Ticker does not request frames itself; its owner asks Active after each
// frame, and asks the platform for another while it is true, which keeps
// an idle app at 0% CPU.
//
// A Ticker is not safe for concurrent use: it lives on the UI thread.
type Ticker struct {
	items []*entry
	has   map[Animator]*entry
	now   time.Duration
}

type entry struct {
	a  Animator
	st State
	// fresh: added since the last Advance, so it has not reported a state
	// yet and counts as running.
	fresh   bool
	removed bool
}

// Add puts a to the ticker; adding an animator already in it does nothing.
// The animator is advanced by the next Advance (and in the current one if
// Add is called from an Advance).
func (t *Ticker) Add(a Animator) {
	if _, ok := t.has[a]; ok {
		return
	}
	if t.has == nil {
		t.has = map[Animator]*entry{}
	}
	e := &entry{a: a, fresh: true}
	t.has[a] = e
	t.items = append(t.items, e)
}

// Remove takes a out of the ticker without advancing it again.
func (t *Ticker) Remove(a Animator) {
	if e, ok := t.has[a]; ok {
		e.removed = true
		delete(t.has, a)
	}
}

// Now returns the time of the last Advance.
func (t *Ticker) Now() time.Duration { return t.now }

// Advance moves every animator to now. Time never goes backwards: an
// earlier now is taken as the last one.
func (t *Ticker) Advance(now time.Duration) {
	t.now = max(t.now, now)
	now = t.now
	// Animators added while advancing join the same pass.
	for i := 0; i < len(t.items); i++ {
		e := t.items[i]
		if e.removed {
			continue
		}
		e.st = e.a.Advance(now)
		e.fresh = false
		if e.st == Done && !e.removed {
			e.removed = true
			delete(t.has, e.a)
		}
	}
	// Compact the list.
	n := 0
	for _, e := range t.items {
		if !e.removed {
			t.items[n] = e
			n++
		}
	}
	clear(t.items[n:])
	t.items = t.items[:n]
}

// Active reports whether the ticker needs another frame: an animator is
// running or endless, or was added since the last Advance.
func (t *Ticker) Active() bool {
	for _, e := range t.items {
		if !e.removed && (e.fresh || e.st != Done) {
			return true
		}
	}
	return false
}

// OnlyEndless reports whether the animators that need frames are all
// endless: nothing the app is waiting for.
func (t *Ticker) OnlyEndless() bool {
	endless := false
	for _, e := range t.items {
		switch {
		case e.removed:
		case e.fresh || e.st == Running:
			return false
		case e.st == Endless:
			endless = true
		}
	}
	return endless
}

// Len returns the number of animators.
func (t *Ticker) Len() int { return len(t.has) }
