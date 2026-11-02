package cocoa

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

func TestMacButton(t *testing.T) {
	want := []platform.PointerButton{
		platform.ButtonPrimary, platform.ButtonSecondary, platform.ButtonMiddle,
		platform.ButtonBack, platform.ButtonForward, platform.ButtonNone, platform.ButtonNone,
	}
	for n, w := range want {
		if got := macButton(n); got != w {
			t.Errorf("macButton(%d) = %v, want %v", n, got, w)
		}
	}
	if got := macButton(-1); got != platform.ButtonNone {
		t.Errorf("macButton(-1) = %v, want ButtonNone", got)
	}
}

func TestMacScrollEvent(t *testing.T) {
	pt := func(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }
	for _, c := range []struct {
		name  string
		in    macScroll
		ok    bool
		delta platform.Point
		steps platform.Point
		phase platform.ScrollPhase
	}{
		{"wheel notch up", macScroll{DY: 1}, true, pt(0, 0), pt(0, -1), platform.ScrollPhaseNone},
		{"wheel accelerated down", macScroll{DY: -3.5}, true, pt(0, 0), pt(0, 3.5), platform.ScrollPhaseNone},
		{"wheel horizontal", macScroll{DX: 2}, true, pt(0, 0), pt(-2, 0), platform.ScrollPhaseNone},
		{"wheel zero", macScroll{}, false, pt(0, 0), pt(0, 0), platform.ScrollPhaseNone},
		{"precise without phase", macScroll{DY: -12, Precise: true}, true, pt(0, 12), pt(0, 0), platform.ScrollPhaseNone},
		{"gesture began", macScroll{DY: -4, Precise: true, Phase: phaseBegan}, true, pt(0, 4), pt(0, 0), platform.ScrollPhaseUpdate},
		{"gesture changed", macScroll{DX: 3, DY: -6, Precise: true, Phase: phaseChanged}, true, pt(-3, 6), pt(0, 0), platform.ScrollPhaseUpdate},
		{"gesture may begin", macScroll{Precise: true, Phase: phaseMayBegin}, false, pt(0, 0), pt(0, 0), platform.ScrollPhaseUpdate},
		{"gesture stationary", macScroll{Precise: true, Phase: phaseStationary}, false, pt(0, 0), pt(0, 0), platform.ScrollPhaseUpdate},
		{"gesture ended", macScroll{Precise: true, Phase: phaseEnded}, true, pt(0, 0), pt(0, 0), platform.ScrollPhaseEnd},
		{"gesture ended with motion", macScroll{DY: -1, Precise: true, Phase: phaseEnded}, true, pt(0, 1), pt(0, 0), platform.ScrollPhaseEnd},
		{"gesture cancelled", macScroll{Precise: true, Phase: phaseCancelled}, true, pt(0, 0), pt(0, 0), platform.ScrollPhaseEnd},
		{"momentum began", macScroll{DY: -20, Precise: true, Momentum: phaseBegan}, false, pt(0, 0), pt(0, 0), platform.ScrollPhaseNone},
		{"momentum changed", macScroll{DY: -8, Precise: true, Momentum: phaseChanged}, false, pt(0, 0), pt(0, 0), platform.ScrollPhaseNone},
		{"momentum ended", macScroll{Precise: true, Momentum: phaseEnded}, false, pt(0, 0), pt(0, 0), platform.ScrollPhaseNone},
	} {
		delta, steps, phase, ok := macScrollEvent(c.in)
		if ok != c.ok {
			t.Errorf("%s: ok = %v, want %v", c.name, ok, c.ok)
			continue
		}
		if !ok {
			continue
		}
		if delta != c.delta || steps != c.steps || phase != c.phase {
			t.Errorf("%s: got delta %v steps %v phase %v, want delta %v steps %v phase %v",
				c.name, delta, steps, phase, c.delta, c.steps, c.phase)
		}
	}
}

// pointerScript feeds a pointerState and returns the emitted events, one
// per line: action, position, button, buttons.
type pointerScript struct {
	p   pointerState
	out []string
}

func (s *pointerScript) emit(e platform.PointerEvent) {
	line := fmt.Sprintf("%v %v,%v", e.Action, e.Pos.X, e.Pos.Y)
	if e.Button != platform.ButtonNone {
		line += fmt.Sprintf(" b%d", e.Button)
	}
	if e.Buttons != 0 {
		line += fmt.Sprintf(" held%d", e.Buttons)
	}
	s.out = append(s.out, line)
}

func (s *pointerScript) expect(t *testing.T, name string, want ...string) {
	t.Helper()
	got := strings.Join(s.out, "; ")
	if w := strings.Join(want, "; "); got != w {
		t.Errorf("%s:\n got  %s\n want %s", name, got, w)
	}
	s.out = nil
}

func TestPointerState(t *testing.T) {
	var s pointerScript
	size := platform.Size{W: 100, H: 80}
	at := func(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }
	move := func(x, y float32) { s.p.move(at(x, y), inBounds(at(x, y), size), s.emit) }
	press := func(b platform.PointerButton, down bool, x, y float32) { s.p.button(b, down, at(x, y), s.emit) }

	s.p.entered(at(1, 1), s.emit)
	move(5, 5)
	s.expect(t, "enter and move", "enter 1,1", "move 5,5")

	s.p.entered(at(5, 5), s.emit)
	s.expect(t, "second enter")

	press(platform.ButtonPrimary, true, 10, 10)
	press(platform.ButtonSecondary, true, 10, 10)
	press(platform.ButtonPrimary, false, 11, 10)
	press(platform.ButtonSecondary, false, 11, 10)
	s.expect(t, "two buttons", "down 10,10 b1 held1", "down 10,10 b2 held3", "up 11,10 b1 held2", "up 11,10 b2")

	press(platform.ButtonMiddle, false, 11, 10)
	press(platform.ButtonNone, true, 11, 10)
	s.expect(t, "release without press, unknown button")

	// A drag out of the view: the leave waits for the release.
	press(platform.ButtonPrimary, true, 50, 50)
	s.p.exited(at(99, 50), s.emit)
	move(-10, 50)
	press(platform.ButtonPrimary, false, -10, 50)
	s.expect(t, "drag out", "down 50,50 b1 held1", "move -10,50 held1", "up -10,50 b1", "leave -10,50")

	// A drag that comes back before the release: no leave.
	s.p.entered(at(1, 1), s.emit)
	press(platform.ButtonPrimary, true, 50, 50)
	move(120, 50)
	move(60, 50)
	press(platform.ButtonPrimary, false, 60, 50)
	s.expect(t, "drag out and back", "enter 1,1", "down 50,50 b1 held1", "move 120,50 held1", "move 60,50 held1", "up 60,50 b1")

	// An exit and a move outside without buttons.
	s.p.exited(at(100, 40), s.emit)
	move(110, 40)
	s.expect(t, "exit", "leave 100,40")

	// A press without a previous enter (the window appeared under the
	// pointer) enters first; a missed exit is a leave at the next move
	// outside.
	press(platform.ButtonPrimary, true, 20, 20)
	press(platform.ButtonPrimary, false, 20, 20)
	move(200, 20)
	s.expect(t, "press enters", "enter 20,20", "down 20,20 b1 held1", "up 20,20 b1", "leave 200,20")
}

func TestEventClock(t *testing.T) {
	c := eventClock{offset: 5 * time.Second}
	uptime := func() float64 { return 100 }
	if got := c.time(1.5, 6*time.Second, uptime); got != 6500*time.Millisecond {
		t.Errorf("time(1.5) = %v, want 6.5s", got)
	}
	// Far from the clock: re-based on the current uptime, and the event
	// is stamped now.
	if got := c.time(1000, 7*time.Second, uptime); got != 7*time.Second {
		t.Errorf("time(1000) = %v, want now (7s)", got)
	}
	if want := 7*time.Second - 100*time.Second; c.offset != want {
		t.Errorf("offset after re-basing = %v, want %v", c.offset, want)
	}
	if got := c.time(100.25, 7*time.Second, uptime); got != 7250*time.Millisecond {
		t.Errorf("time after re-basing = %v, want 7.25s", got)
	}
}
