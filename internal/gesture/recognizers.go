package gesture

import "time"

// ---------------------------------------------------------------------
// Tap.

// Tap recognizes a tap: the press is released and nothing else won the
// arena. It feeds DOM click semantics: it does not care how far the
// pointer moved (the caller decides the click target), and it loses only
// to another recognizer that accepts (a drag, a long press).
type Tap struct {
	// OnTap is called once when the tap won and the pointer was released,
	// with the release event.
	OnTap func(up Event)
	// OnReject is called when another recognizer won (optional).
	OnReject func()

	entry     Entry
	upEv      Event
	released  bool
	won, done bool
}

func (r *Tap) down(e Event, entry Entry) { r.entry = entry }
func (r *Tap) move(Event)                {}
func (r *Tap) deadline() (time.Duration, bool) {
	return 0, false
}
func (r *Tap) tick(time.Duration) {}

func (r *Tap) up(e Event) {
	r.upEv, r.released = e, true
	r.fire()
}

func (r *Tap) cancel(Event) {
	r.entry.Resolve(Rejected)
	r.done = true
}

// AcceptGesture implements Member.
func (r *Tap) AcceptGesture(PointerID) {
	r.won = true
	r.fire()
}

// RejectGesture implements Member.
func (r *Tap) RejectGesture(PointerID) {
	if r.done {
		return
	}
	r.done = true
	if r.OnReject != nil {
		r.OnReject()
	}
}

func (r *Tap) fire() {
	if !r.won || !r.released || r.done {
		return
	}
	r.done = true
	if r.OnTap != nil {
		r.OnTap(r.upEv)
	}
}

// ---------------------------------------------------------------------
// Drag.

// DragUpdate describes a drag at one event.
type DragUpdate struct {
	// Pos is the pointer position.
	Pos Point
	// Delta is the movement since the previous update (since the press
	// for the start).
	Delta Point
	// Offset is the movement since the press.
	Offset Point
	// Velocity is the pointer velocity in logical pixels per second,
	// estimated from the recent moves; at the end it is the release
	// velocity (zero when the pointer stopped before the release, or the
	// drag was cancelled).
	Velocity Point
	Time     time.Duration
}

// Axis restricts the movement a Drag measures against its slop.
type Axis uint8

// The axes.
const (
	// AxisFree measures the distance in any direction (a pan).
	AxisFree Axis = iota
	// AxisHorizontal measures only the horizontal movement.
	AxisHorizontal
	// AxisVertical measures only the vertical movement.
	AxisVertical
)

// Drag recognizes a drag: the pointer moves farther than Slop from the
// press. It accepts at that move, and then reports the start (at that
// move), updates and the end. Offsets are measured from the press
// position, so a dragged element follows the pointer exactly. It rejects
// itself when the pointer is released or cancelled before.
//
// With Axis set it only counts the movement along that axis (Flutter's
// VerticalDragGestureRecognizer: a vertical scroller does not take a
// horizontal swipe). With Eager it accepts at the press and starts there,
// before any movement (a scrollbar thumb follows the pointer from the
// first pixel).
type Drag struct {
	// Slop is the distance before the drag starts; zero means
	// DragSlop(kind of the press).
	Slop float32
	// Axis is the movement measured against the slop.
	Axis Axis
	// Eager accepts the gesture at the press: the drag starts at the
	// press (OnStart with a zero offset) and wins the arena when it
	// closes, unless another eager member accepted first.
	Eager                    bool
	OnStart, OnUpdate, OnEnd func(DragUpdate)

	entry   Entry
	press   Point
	pressAt time.Duration
	last    Point
	vt      VelocityTracker
	slop    float32
	won     bool
	started bool
	done    bool
}

func (r *Drag) down(e Event, entry Entry) {
	r.entry = entry
	r.press, r.last, r.pressAt = e.Pos, e.Pos, e.Time
	r.slop = r.Slop
	if r.slop == 0 {
		r.slop = DragSlop(e.Kind)
	}
	r.vt.Reset()
	r.vt.Add(e.Time, e.Pos)
	if r.Eager {
		r.entry.Resolve(Accepted)
	}
}

// pastSlop reports whether p is farther than the slop from the press,
// along the recognizer's axis.
func (r *Drag) pastSlop(p Point) bool {
	switch r.Axis {
	case AxisHorizontal:
		d := p.X - r.press.X
		return d > r.slop || -d > r.slop
	case AxisVertical:
		d := p.Y - r.press.Y
		return d > r.slop || -d > r.slop
	}
	return beyond(p, r.press, r.slop)
}

func (r *Drag) deadline() (time.Duration, bool) { return 0, false }
func (r *Drag) tick(time.Duration)              {}

func (r *Drag) move(e Event) {
	if r.done {
		return
	}
	r.vt.Add(e.Time, e.Pos)
	if !r.started {
		if !r.pastSlop(e.Pos) {
			return
		}
		if !r.won {
			r.entry.Resolve(Accepted)
			if !r.won {
				return // lost: another recognizer had won
			}
		}
		r.started = true
		r.last = e.Pos
		d := e.Pos.Sub(r.press)
		if r.OnStart != nil {
			r.OnStart(DragUpdate{Pos: e.Pos, Delta: d, Offset: d, Velocity: r.vt.Velocity(), Time: e.Time})
		}
		return
	}
	d := e.Pos.Sub(r.last)
	r.last = e.Pos
	if r.OnUpdate != nil {
		r.OnUpdate(DragUpdate{Pos: e.Pos, Delta: d, Offset: e.Pos.Sub(r.press), Velocity: r.vt.Velocity(), Time: e.Time})
	}
}

func (r *Drag) up(e Event) {
	if r.done {
		return
	}
	if !r.started {
		r.done = true
		r.entry.Resolve(Rejected)
		return
	}
	r.done = true
	r.vt.Add(e.Time, e.Pos)
	d := e.Pos.Sub(r.last)
	if r.OnEnd != nil {
		r.OnEnd(DragUpdate{Pos: e.Pos, Delta: d, Offset: e.Pos.Sub(r.press), Velocity: r.vt.Velocity(), Time: e.Time})
	}
}

func (r *Drag) cancel(e Event) {
	if r.done {
		return
	}
	r.done = true
	if !r.started {
		r.entry.Resolve(Rejected)
		return
	}
	if r.OnEnd != nil {
		r.OnEnd(DragUpdate{Pos: r.last, Offset: r.last.Sub(r.press), Time: e.Time})
	}
}

// AcceptGesture implements Member.
func (r *Drag) AcceptGesture(PointerID) {
	r.won = true
	if r.Eager && !r.started && !r.done {
		r.started = true
		if r.OnStart != nil {
			r.OnStart(DragUpdate{Pos: r.press, Time: r.pressAt})
		}
	}
}

// RejectGesture implements Member.
func (r *Drag) RejectGesture(PointerID) { r.done = true }

// Started reports whether the drag started.
func (r *Drag) Started() bool { return r.started }

// ---------------------------------------------------------------------
// Long press.

// LongPress recognizes a long press: the pointer stays down, within Slop
// of the press, for Duration. It accepts then and calls OnLongPress. It
// rejects itself when the pointer moves farther or is released before.
type LongPress struct {
	// Duration is the press time; zero means LongPressTimeout.
	Duration time.Duration
	// Slop is the distance the pointer may move; zero means
	// DragSlop(kind of the press).
	Slop float32
	// OnLongPress is called when the long press is recognized, with the
	// press event moved to the latest position and the time of the
	// deadline.
	OnLongPress func(e Event)

	entry Entry
	press Event
	last  Point
	slop  float32
	at    time.Duration
	armed bool
	won   bool
	done  bool
}

func (r *LongPress) down(e Event, entry Entry) {
	r.entry, r.press, r.last = entry, e, e.Pos
	r.slop = r.Slop
	if r.slop == 0 {
		r.slop = DragSlop(e.Kind)
	}
	d := r.Duration
	if d == 0 {
		d = LongPressTimeout
	}
	r.at = e.Time + d
	r.armed = true
}

func (r *LongPress) move(e Event) {
	if !r.armed {
		return
	}
	r.last = e.Pos
	if beyond(e.Pos, r.press.Pos, r.slop) {
		r.giveUp()
	}
}

func (r *LongPress) up(Event)     { r.giveUp() }
func (r *LongPress) cancel(Event) { r.giveUp() }

func (r *LongPress) giveUp() {
	if !r.armed {
		return
	}
	r.armed = false
	if !r.won {
		r.entry.Resolve(Rejected)
	}
}

func (r *LongPress) deadline() (time.Duration, bool) {
	return r.at, r.armed && !r.done
}

func (r *LongPress) tick(now time.Duration) {
	if !r.armed || r.done || now < r.at {
		return
	}
	r.armed = false
	if !r.won {
		r.entry.Resolve(Accepted)
		if !r.won {
			return
		}
	}
	if r.OnLongPress != nil {
		e := r.press
		e.Pos, e.Time = r.last, r.at
		r.OnLongPress(e)
	}
}

// AcceptGesture implements Member.
func (r *LongPress) AcceptGesture(PointerID) { r.won = true }

// RejectGesture implements Member.
func (r *LongPress) RejectGesture(PointerID) {
	r.done = true
	r.armed = false
}

// ---------------------------------------------------------------------
// Click counting (double tap).

// ClickCounter counts consecutive presses as the DOM's click count
// (MouseEvent.detail): a press of the same button and kind within the
// double-click time of the previous press and within its distance counts
// one more; anything else starts again at 1. The limits are Interval and
// platform.DoubleClickSlop for mice and pens (the double-click rule of
// ADR-192), DoubleTapTime and DoubleTapSlop for touch.
//
// It is the double-tap recognizer of the arena model without its delay: a
// double click is the second click of a sequence, so single clicks are
// reported immediately, as in browsers.
type ClickCounter struct {
	// Interval is the double-click time of mice and pens: the OS's
	// (platform.SystemSettings.DoubleClickInterval). Not positive means
	// platform.DefaultDoubleClickInterval.
	Interval time.Duration

	count  int
	button uint8
	kind   Kind
	pos    Point
	at     time.Duration
}

// Press records a press and returns its click count.
func (c *ClickCounter) Press(kind Kind, button uint8, pos Point, at time.Duration) int {
	limit, dist := clickLimits(kind, c.Interval)
	if c.count > 0 && button == c.button && kind == c.kind && at-c.at <= limit && at >= c.at && !beyond(pos, c.pos, dist) {
		c.count++
	} else {
		c.count = 1
	}
	c.button, c.kind, c.pos, c.at = button, kind, pos, at
	return c.count
}

// Count returns the click count of the latest press (0 after Reset).
func (c *ClickCounter) Count() int { return c.count }

// Reset starts a new sequence: the next press counts 1. Called when a
// press did not click (a drag or a long press won).
func (c *ClickCounter) Reset() { c.count = 0 }
