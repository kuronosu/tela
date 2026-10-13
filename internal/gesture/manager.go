package gesture

import "time"

// Recognizer is a member of a pointer's arena that receives the pointer's
// events: Tap, Drag and LongPress. Each recognizer tracks one press.
type Recognizer interface {
	Member
	// down starts tracking the press; entry resolves the recognizer in
	// the pointer's arena.
	down(e Event, entry Entry)
	move(e Event)
	up(e Event)
	cancel(e Event)
	// deadline reports when the recognizer wants tick to be called.
	deadline() (time.Duration, bool)
	tick(now time.Duration)
}

// Manager routes pointer events to the recognizers of each press and runs
// their arenas. The zero Manager is ready to use. It is not safe for
// concurrent use; the callbacks of the recognizers run synchronously
// inside its methods.
type Manager struct {
	arena  Arena
	routes map[PointerID][]Recognizer
	order  []PointerID // pointers with routes, in press order
}

// Down starts the press of pointer e.Pointer with the given recognizers
// (nil entries are skipped): they join the pointer's arena in the order
// given, receive the press, and the arena closes. A press of a pointer
// that is still down cancels the previous one first.
func (m *Manager) Down(e Event, rs ...Recognizer) {
	if _, ok := m.routes[e.Pointer]; ok {
		m.Cancel(e)
	}
	var list []Recognizer
	for _, r := range rs {
		if r != nil {
			list = append(list, r)
		}
	}
	if len(list) == 0 {
		return
	}
	if m.routes == nil {
		m.routes = map[PointerID][]Recognizer{}
	}
	m.routes[e.Pointer] = list
	m.order = append(m.order, e.Pointer)
	for _, r := range list {
		r.down(e, m.arena.Add(e.Pointer, r))
	}
	m.arena.Close(e.Pointer)
}

// Move routes a move of a pointer that is down.
func (m *Manager) Move(e Event) {
	for _, r := range m.routes[e.Pointer] {
		r.move(e)
	}
}

// Up routes the release of a pointer, sweeps its arena (the first member
// still undecided wins) and ends the press.
func (m *Manager) Up(e Event) {
	rs, ok := m.routes[e.Pointer]
	if !ok {
		return
	}
	m.end(e.Pointer)
	for _, r := range rs {
		r.up(e)
	}
	m.arena.Sweep(e.Pointer)
}

// Cancel aborts the press of a pointer: every recognizer gives up (an
// accepted drag ends with zero velocity) and no one wins what was still
// undecided.
func (m *Manager) Cancel(e Event) {
	rs, ok := m.routes[e.Pointer]
	if !ok {
		return
	}
	m.end(e.Pointer)
	for _, r := range rs {
		r.cancel(e)
	}
	// Recognizers reject themselves on cancel; drop what may remain.
	delete(m.arena.arenas, e.Pointer)
}

// Tracking reports whether a press of pointer p is being tracked.
func (m *Manager) Tracking(p PointerID) bool {
	_, ok := m.routes[p]
	return ok
}

func (m *Manager) end(p PointerID) {
	delete(m.routes, p)
	for i, q := range m.order {
		if q == p {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

// NextDeadline reports the earliest time at which a recognizer waits for
// Tick (a long press timeout). ok is false when none waits: no timer is
// needed.
func (m *Manager) NextDeadline() (t time.Duration, ok bool) {
	for _, p := range m.order {
		for _, r := range m.routes[p] {
			if d, has := r.deadline(); has && (!ok || d < t) {
				t, ok = d, true
			}
		}
	}
	return t, ok
}

// Tick lets the recognizers whose deadline is at or before now act on
// it (a long press is recognized).
func (m *Manager) Tick(now time.Duration) {
	// Callbacks may end presses; iterate over a copy.
	ptrs := append([]PointerID(nil), m.order...)
	for _, p := range ptrs {
		rs := append([]Recognizer(nil), m.routes[p]...)
		for _, r := range rs {
			if d, has := r.deadline(); has && d <= now {
				r.tick(now)
			}
		}
	}
}
