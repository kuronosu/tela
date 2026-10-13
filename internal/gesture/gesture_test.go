package gesture

import (
	"math"
	"strings"
	"testing"
	"time"
)

const ms = time.Millisecond

// member records what the arena told it.
type member struct {
	name string
	log  *[]string
}

func (m *member) AcceptGesture(PointerID) { *m.log = append(*m.log, m.name+" accept") }
func (m *member) RejectGesture(PointerID) { *m.log = append(*m.log, m.name+" reject") }

func TestArenaSingleMemberWinsAtClose(t *testing.T) {
	var log []string
	var a Arena
	a.Add(1, &member{"a", &log})
	if len(log) != 0 {
		t.Fatalf("resolved while open: %v", log)
	}
	a.Close(1)
	if got := strings.Join(log, ","); got != "a accept" {
		t.Errorf("log %q", got)
	}
	if a.Pending(1) {
		t.Error("arena still pending")
	}
}

func TestArenaEagerWinner(t *testing.T) {
	var log []string
	var a Arena
	a.Add(1, &member{"a", &log})
	eb := a.Add(1, &member{"b", &log})
	eb.Resolve(Accepted) // while open: wins at close
	if len(log) != 0 {
		t.Fatalf("resolved while open: %v", log)
	}
	a.Close(1)
	if got := strings.Join(log, ","); got != "a reject,b accept" {
		t.Errorf("log %q", got)
	}
}

func TestArenaRejectionsLeaveLastMember(t *testing.T) {
	var log []string
	var a Arena
	ea := a.Add(1, &member{"a", &log})
	a.Add(1, &member{"b", &log})
	ec := a.Add(1, &member{"c", &log})
	a.Close(1)
	ea.Resolve(Rejected)
	ec.Resolve(Rejected)
	if got := strings.Join(log, ","); got != "a reject,c reject,b accept" {
		t.Errorf("log %q", got)
	}
	ea.Resolve(Accepted) // too late: nothing happens
	if len(log) != 3 {
		t.Errorf("late resolve changed the log: %v", log)
	}
}

func TestArenaSweepFirstWins(t *testing.T) {
	var log []string
	var a Arena
	a.Add(1, &member{"a", &log})
	a.Add(1, &member{"b", &log})
	a.Close(1)
	a.Sweep(1)
	if got := strings.Join(log, ","); got != "b reject,a accept" {
		t.Errorf("log %q", got)
	}
}

func TestArenaAcceptAfterCloseRejectsOthers(t *testing.T) {
	var log []string
	var a Arena
	a.Add(1, &member{"a", &log})
	eb := a.Add(1, &member{"b", &log})
	a.Add(1, &member{"c", &log})
	a.Close(1)
	eb.Resolve(Accepted)
	if got := strings.Join(log, ","); got != "a reject,c reject,b accept" {
		t.Errorf("log %q", got)
	}
}

func TestArenaPointersAreIndependent(t *testing.T) {
	var log []string
	var a Arena
	a.Add(1, &member{"a1", &log})
	a.Add(1, &member{"b1", &log})
	a.Add(2, &member{"a2", &log})
	a.Close(1)
	a.Close(2)
	if got := strings.Join(log, ","); got != "a2 accept" {
		t.Errorf("log %q", got)
	}
	if !a.Pending(1) {
		t.Error("arena 1 resolved")
	}
}

// harness drives a Manager with a tap, a drag and a long press.
type harness struct {
	m      Manager
	log    []string
	tap    *Tap
	drag   *Drag
	lp     *LongPress
	starts []DragUpdate
	ends   []DragUpdate
	upds   []DragUpdate
}

func newHarness(kind Kind, withTap, withDrag, withLP bool) *harness {
	h := &harness{}
	var rs []Recognizer
	if withLP {
		h.lp = &LongPress{OnLongPress: func(e Event) { h.log = append(h.log, "longpress") }}
		rs = append(rs, h.lp)
	}
	if withDrag {
		h.drag = &Drag{
			OnStart:  func(u DragUpdate) { h.log = append(h.log, "dragstart"); h.starts = append(h.starts, u) },
			OnUpdate: func(u DragUpdate) { h.log = append(h.log, "drag"); h.upds = append(h.upds, u) },
			OnEnd:    func(u DragUpdate) { h.log = append(h.log, "dragend"); h.ends = append(h.ends, u) },
		}
		rs = append(rs, h.drag)
	}
	if withTap {
		h.tap = &Tap{
			OnTap:    func(Event) { h.log = append(h.log, "tap") },
			OnReject: func() { h.log = append(h.log, "tap rejected") },
		}
		rs = append(rs, h.tap)
	}
	h.m.Down(Event{Pointer: 1, Kind: kind, Pos: Point{100, 100}, Time: 0}, rs...)
	return h
}

func (h *harness) move(x, y float32, at time.Duration) {
	h.m.Move(Event{Pointer: 1, Pos: Point{x, y}, Time: at})
}

func (h *harness) up(x, y float32, at time.Duration) {
	h.m.Up(Event{Pointer: 1, Pos: Point{x, y}, Time: at})
}

func (h *harness) result() string { return strings.Join(h.log, ",") }

func TestTapAlone(t *testing.T) {
	h := newHarness(Mouse, true, false, false)
	h.move(300, 300, 10*ms) // a click survives movement (DOM)
	h.up(300, 300, 20*ms)
	if got := h.result(); got != "tap" {
		t.Errorf("got %q", got)
	}
}

func TestTapBeatsDragWithoutMovement(t *testing.T) {
	h := newHarness(Mouse, true, true, true)
	h.move(102, 101, 10*ms) // within the mouse slop
	h.up(102, 101, 50*ms)
	if got := h.result(); got != "tap" {
		t.Errorf("got %q", got)
	}
	if _, ok := h.m.NextDeadline(); ok {
		t.Error("a deadline remains after the release")
	}
}

func TestDragBeatsTapAndLongPress(t *testing.T) {
	h := newHarness(Mouse, true, true, true)
	h.move(103, 100, 10*ms) // 3 px: within MouseSlop
	if h.result() != "" {
		t.Fatalf("early: %q", h.result())
	}
	h.move(106, 100, 20*ms) // past the slop
	h.move(110, 104, 30*ms)
	h.m.Tick(600 * ms) // the long press lost: nothing
	h.up(110, 104, 40*ms)
	if got := h.result(); got != "tap rejected,dragstart,drag,dragend" {
		t.Errorf("got %q", got)
	}
	s := h.starts[0]
	if s.Offset != (Point{6, 0}) || s.Delta != (Point{6, 0}) {
		t.Errorf("start %+v", s)
	}
	u := h.upds[0]
	if u.Delta != (Point{4, 4}) || u.Offset != (Point{10, 4}) {
		t.Errorf("update %+v", u)
	}
	e := h.ends[0]
	if e.Offset != (Point{10, 4}) || e.Delta != (Point{}) {
		t.Errorf("end %+v", e)
	}
}

func TestTouchSlopIsLarger(t *testing.T) {
	h := newHarness(Touch, true, true, false)
	h.move(110, 100, 10*ms) // 10 px: within TouchSlop
	h.up(110, 100, 20*ms)
	if got := h.result(); got != "tap" {
		t.Errorf("got %q", got)
	}
}

func TestLongPressBeatsTapAndDrag(t *testing.T) {
	h := newHarness(Mouse, true, true, true)
	d, ok := h.m.NextDeadline()
	if !ok || d != LongPressTimeout {
		t.Fatalf("deadline %v %v, want %v", d, ok, LongPressTimeout)
	}
	h.m.Tick(499 * ms)
	if h.result() != "" {
		t.Fatalf("fired early: %q", h.result())
	}
	h.m.Tick(500 * ms)
	if got := h.result(); got != "tap rejected,longpress" {
		t.Errorf("got %q", got)
	}
	h.move(200, 200, 600*ms) // the drag lost: nothing starts
	h.up(200, 200, 700*ms)
	if got := h.result(); got != "tap rejected,longpress" {
		t.Errorf("after release: %q", got)
	}
	if _, ok := h.m.NextDeadline(); ok {
		t.Error("deadline remains")
	}
}

func TestLongPressCancelledByMovement(t *testing.T) {
	h := newHarness(Touch, true, false, true)
	h.move(120, 100, 100*ms) // 20 px > TouchSlop
	if _, ok := h.m.NextDeadline(); ok {
		t.Error("the long press still waits after moving away")
	}
	h.m.Tick(time.Second)
	h.up(120, 100, time.Second)
	if got := h.result(); got != "tap" {
		t.Errorf("got %q", got)
	}
}

func TestLongPressAloneNeedsTime(t *testing.T) {
	h := newHarness(Mouse, false, false, true)
	h.up(100, 100, 100*ms)
	h.m.Tick(time.Second)
	if got := h.result(); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestDragAloneWaitsForSlop(t *testing.T) {
	// A lone recognizer wins the arena at once (Flutter), but the drag
	// still starts only past the slop.
	h := newHarness(Mouse, false, true, false)
	h.move(101, 100, 10*ms)
	h.up(101, 100, 20*ms)
	if got := h.result(); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestCancel(t *testing.T) {
	h := newHarness(Mouse, true, true, true)
	h.move(130, 100, 10*ms)
	h.m.Cancel(Event{Pointer: 1, Time: 20 * ms})
	if got := h.result(); got != "tap rejected,dragstart,dragend" {
		t.Errorf("got %q", got)
	}
	if h.ends[0].Velocity != (Point{}) {
		t.Errorf("cancelled drag velocity %+v", h.ends[0].Velocity)
	}
	if h.m.Tracking(1) {
		t.Error("still tracking")
	}
}

func TestNestedDragsInnermostWins(t *testing.T) {
	var log []string
	inner := &Drag{OnStart: func(DragUpdate) { log = append(log, "inner") }}
	outer := &Drag{OnStart: func(DragUpdate) { log = append(log, "outer") }}
	var m Manager
	m.Down(Event{Pointer: 1, Pos: Point{0, 0}}, inner, outer)
	m.Move(Event{Pointer: 1, Pos: Point{20, 0}, Time: 10 * ms})
	m.Move(Event{Pointer: 1, Pos: Point{30, 0}, Time: 20 * ms})
	m.Up(Event{Pointer: 1, Pos: Point{30, 0}, Time: 30 * ms})
	if got := strings.Join(log, ","); got != "inner" {
		t.Errorf("got %q", got)
	}
}

func TestVelocity(t *testing.T) {
	var v VelocityTracker
	for i := 0; i <= 10; i++ {
		v.Add(time.Duration(i)*8*ms, Point{float32(i * 4), float32(-i * 2)})
	}
	got := v.Velocity()
	if math.Abs(float64(got.X-500)) > 0.5 || math.Abs(float64(got.Y+250)) > 0.5 {
		t.Errorf("velocity %+v, want (500, -250)", got)
	}
	// A pause longer than 40 ms before the latest sample: stopped.
	v.Add(130*ms, Point{40, -20})
	if got := v.Velocity(); got != (Point{}) {
		t.Errorf("after a pause: %+v", got)
	}
	v.Reset()
	if got := v.Velocity(); got != (Point{}) {
		t.Errorf("after reset: %+v", got)
	}
}

func TestDragEndVelocity(t *testing.T) {
	h := newHarness(Mouse, false, true, false)
	for i := 1; i <= 10; i++ {
		h.move(100+float32(i*10), 100, time.Duration(i)*10*ms)
	}
	h.up(200, 100, 100*ms)
	e := h.ends[0]
	if math.Abs(float64(e.Velocity.X-1000)) > 1 || e.Velocity.Y != 0 {
		t.Errorf("end velocity %+v, want (1000, 0)", e.Velocity)
	}
	if e.Offset != (Point{100, 0}) {
		t.Errorf("end offset %+v", e.Offset)
	}
}

func TestClickCounter(t *testing.T) {
	var c ClickCounter
	at := func(x float32, d time.Duration) int { return c.Press(Mouse, 1, Point{x, 0}, d) }
	if n := at(0, 0); n != 1 {
		t.Errorf("first %d", n)
	}
	if n := at(3, 200*ms); n != 2 {
		t.Errorf("second %d", n)
	}
	if n := at(3, 400*ms); n != 3 {
		t.Errorf("third %d", n)
	}
	if n := at(3, 1000*ms); n != 1 {
		t.Errorf("after the window %d", n)
	}
	if n := at(20, 1100*ms); n != 1 {
		t.Errorf("too far %d", n)
	}
	if n := c.Press(Mouse, 2, Point{20, 0}, 1200*ms); n != 1 {
		t.Errorf("other button %d", n)
	}
	c.Reset()
	if n := c.Press(Mouse, 2, Point{20, 0}, 1300*ms); n != 1 {
		t.Errorf("after reset %d", n)
	}
	// Touch: 300 ms, 100 px.
	var tc ClickCounter
	tc.Press(Touch, 1, Point{0, 0}, 0)
	if n := tc.Press(Touch, 1, Point{60, 0}, 250*ms); n != 2 {
		t.Errorf("touch double tap %d", n)
	}
	if n := tc.Press(Touch, 1, Point{60, 0}, 600*ms); n != 1 {
		t.Errorf("touch late %d", n)
	}
}

func TestDragAxis(t *testing.T) {
	// A vertical drag ignores horizontal movement: an outer vertical
	// scroller does not take a horizontal swipe from an inner free drag.
	var log []string
	var m Manager
	inner := &Drag{OnStart: func(DragUpdate) { log = append(log, "inner") }}
	outer := &Drag{Axis: AxisVertical, OnStart: func(DragUpdate) { log = append(log, "outer") }}
	m.Down(Event{Pointer: 1, Kind: Touch, Pos: Point{100, 100}}, outer, inner)
	m.Move(Event{Pointer: 1, Pos: Point{130, 105}, Time: 10 * ms})
	if got := strings.Join(log, ","); got != "inner" {
		t.Errorf("horizontal swipe: %q, want inner", got)
	}
	m.Up(Event{Pointer: 1, Pos: Point{130, 105}, Time: 20 * ms})

	log = nil
	inner = &Drag{Axis: AxisHorizontal, OnStart: func(DragUpdate) { log = append(log, "inner") }}
	outer = &Drag{Axis: AxisVertical, OnStart: func(DragUpdate) { log = append(log, "outer") }}
	m.Down(Event{Pointer: 2, Kind: Touch, Pos: Point{100, 100}}, inner, outer)
	m.Move(Event{Pointer: 2, Pos: Point{110, 100}, Time: 10 * ms}) // within the slop
	m.Move(Event{Pointer: 2, Pos: Point{110, 130}, Time: 20 * ms}) // vertical, past the slop
	if got := strings.Join(log, ","); got != "outer" {
		t.Errorf("vertical swipe: %q, want outer", got)
	}
	m.Up(Event{Pointer: 2, Pos: Point{110, 130}, Time: 30 * ms})
}

func TestDragEager(t *testing.T) {
	// An eager drag (a scrollbar thumb) wins at the press, before a tap,
	// and starts with a zero offset.
	var log []string
	var starts, upds []DragUpdate
	d := &Drag{Eager: true,
		OnStart:  func(u DragUpdate) { log = append(log, "start"); starts = append(starts, u) },
		OnUpdate: func(u DragUpdate) { log = append(log, "drag"); upds = append(upds, u) },
		OnEnd:    func(DragUpdate) { log = append(log, "end") },
	}
	tap := &Tap{OnTap: func(Event) { log = append(log, "tap") }, OnReject: func() { log = append(log, "tap rejected") }}
	var m Manager
	m.Down(Event{Pointer: 1, Kind: Mouse, Pos: Point{50, 50}, Time: 5 * ms}, d, tap)
	if got := strings.Join(log, ","); got != "tap rejected,start" {
		t.Fatalf("at the press: %q", got)
	}
	if starts[0].Offset != (Point{}) || starts[0].Pos != (Point{50, 50}) || starts[0].Time != 5*ms {
		t.Errorf("start %+v", starts[0])
	}
	m.Move(Event{Pointer: 1, Pos: Point{50, 51}, Time: 10 * ms}) // one pixel moves the thumb
	m.Up(Event{Pointer: 1, Pos: Point{50, 51}, Time: 20 * ms})
	if got := strings.Join(log, ","); got != "tap rejected,start,drag,end" {
		t.Errorf("got %q", got)
	}
	if upds[0].Offset != (Point{0, 1}) {
		t.Errorf("update %+v", upds[0])
	}
}
