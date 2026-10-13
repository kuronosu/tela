package gesture

import "time"

// PointerID identifies a pointer sequence in the arena. The caller makes
// it unique among the pointers that are down at the same time.
type PointerID uint32

// Kind is the device behind a pointer. It selects the slops and the
// double-click limits.
type Kind uint8

// The pointer kinds.
const (
	Mouse Kind = iota // mouse or touchpad cursor
	Touch             // finger on a touchscreen
	Pen               // stylus
)

// Point is a position in logical pixels.
type Point struct{ X, Y float32 }

// Sub returns p - q.
func (p Point) Sub(q Point) Point { return Point{p.X - q.X, p.Y - q.Y} }

// beyond reports whether the distance from p to q exceeds slop. The
// products are rounded explicitly (no fused multiply-add).
func beyond(p, q Point, slop float32) bool {
	dx, dy := float64(p.X-q.X), float64(p.Y-q.Y)
	d2 := float64(dx*dx) + float64(dy*dy)
	s := float64(slop)
	return d2 > float64(s*s)
}

// Event is a pointer event fed to the Manager.
type Event struct {
	Pointer PointerID
	Kind    Kind
	// Pos is the pointer position in logical pixels (window coordinates).
	Pos Point
	// Time is when it happened, in the backend clock.
	Time time.Duration
}

// Member is a participant of a pointer's arena.
type Member interface {
	// AcceptGesture tells the member it won the arena of pointer p.
	AcceptGesture(p PointerID)
	// RejectGesture tells the member it lost the arena of pointer p.
	RejectGesture(p PointerID)
}

// Disposition is how a member resolves itself.
type Disposition uint8

// The dispositions.
const (
	Accepted Disposition = iota + 1
	Rejected
)

// arenaState is the arena of one pointer.
type arenaState struct {
	members []Member
	open    bool
	// eager is the first member that accepted while the arena was open.
	eager Member
}

// Arena is Flutter's GestureArenaManager: one arena per pointer, in which
// the members compete until one wins. The zero Arena is ready to use. It
// is not safe for concurrent use.
type Arena struct {
	arenas map[PointerID]*arenaState
}

// Entry is a member's ticket in a pointer's arena, through which it
// resolves itself.
type Entry struct {
	a *Arena
	p PointerID
	m Member
}

// Resolve accepts or rejects the entry's member. It does nothing when the
// arena was already resolved or the member already rejected.
func (e Entry) Resolve(d Disposition) {
	if e.a != nil {
		e.a.resolve(e.p, e.m, d)
	}
}

// Add adds m to the arena of pointer p, opening the arena if needed.
func (a *Arena) Add(p PointerID, m Member) Entry {
	if a.arenas == nil {
		a.arenas = map[PointerID]*arenaState{}
	}
	st := a.arenas[p]
	if st == nil {
		st = &arenaState{open: true}
		a.arenas[p] = st
	}
	st.members = append(st.members, m)
	return Entry{a: a, p: p, m: m}
}

// Close closes the arena of pointer p to new members and resolves it if
// it can: to its only member, or to a member that accepted while it was
// open.
func (a *Arena) Close(p PointerID) {
	st := a.arenas[p]
	if st == nil {
		return
	}
	st.open = false
	a.tryResolve(p, st)
}

// Sweep resolves an arena that is still undecided when its pointer is
// released: the first member wins and the others are rejected.
func (a *Arena) Sweep(p PointerID) {
	st := a.arenas[p]
	if st == nil {
		return
	}
	delete(a.arenas, p)
	if len(st.members) == 0 {
		return
	}
	first := st.members[0]
	for _, m := range st.members[1:] {
		m.RejectGesture(p)
	}
	first.AcceptGesture(p)
}

// Pending reports whether pointer p has an undecided arena.
func (a *Arena) Pending(p PointerID) bool { return a.arenas[p] != nil }

func (a *Arena) resolve(p PointerID, m Member, d Disposition) {
	st := a.arenas[p]
	if st == nil {
		return // already resolved
	}
	i := indexOf(st.members, m)
	if i < 0 {
		return // already rejected
	}
	if d == Rejected {
		st.members = append(st.members[:i], st.members[i+1:]...)
		if st.eager == m {
			// An eager winner that withdraws before Close no longer
			// wins (Flutter keeps it and would accept it at Close).
			st.eager = nil
		}
		m.RejectGesture(p)
		if !st.open {
			a.tryResolve(p, st)
		}
		return
	}
	if st.open {
		if st.eager == nil {
			st.eager = m
		}
		return
	}
	a.resolveFor(p, st, m)
}

func (a *Arena) tryResolve(p PointerID, st *arenaState) {
	switch {
	case len(st.members) == 0:
		delete(a.arenas, p)
	case len(st.members) == 1:
		delete(a.arenas, p)
		st.members[0].AcceptGesture(p)
	case st.eager != nil:
		a.resolveFor(p, st, st.eager)
	}
}

// resolveFor makes m the winner: the arena is gone before the callbacks
// run, so a callback that resolves again does nothing.
func (a *Arena) resolveFor(p PointerID, st *arenaState, m Member) {
	delete(a.arenas, p)
	for _, o := range st.members {
		if o != m {
			o.RejectGesture(p)
		}
	}
	m.AcceptGesture(p)
}

func indexOf(ms []Member, m Member) int {
	for i, x := range ms {
		if x == m {
			return i
		}
	}
	return -1
}
