package win32

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestStallGuardFires(t *testing.T) {
	var cancelled atomic.Int32
	g := newStallGuard(20*time.Millisecond, func() { cancelled.Add(1) })
	deadline := time.Now().Add(2 * time.Second)
	for cancelled.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if cancelled.Load() != 1 {
		t.Fatalf("cancel ran %d times, want 1", cancelled.Load())
	}
	if !g.Stalled() {
		t.Errorf("Stalled = false after the cancel")
	}
	if !g.stop() {
		t.Errorf("stop = false for a guard that fired")
	}
	time.Sleep(60 * time.Millisecond)
	if cancelled.Load() != 1 {
		t.Errorf("cancel ran again: %d", cancelled.Load())
	}
}

func TestStallGuardProgress(t *testing.T) {
	var cancelled atomic.Int32
	g := newStallGuard(80*time.Millisecond, func() { cancelled.Add(1) })
	// Calls that complete every 20 ms keep the guard quiet for longer than
	// its timeout...
	for range 15 {
		time.Sleep(20 * time.Millisecond)
		g.progress()
	}
	if cancelled.Load() != 0 || g.Stalled() {
		t.Fatalf("the guard fired (%d) while calls progressed", cancelled.Load())
	}
	// ...and stop disarms it: a call that hangs afterwards is not cancelled.
	if g.stop() {
		t.Errorf("stop = true for a guard that never fired")
	}
	time.Sleep(200 * time.Millisecond)
	if cancelled.Load() != 0 {
		t.Errorf("a stopped guard cancelled")
	}
}

func TestNilStallGuard(t *testing.T) {
	var g *stallGuard
	g.progress()
	if g.Stalled() || g.stop() {
		t.Errorf("a nil guard stalled")
	}
}

func TestDropStallError(t *testing.T) {
	old := dropStallTimeout
	defer func() { dropStallTimeout = old }()
	dropStallTimeout = 3 * time.Second
	if got := errDropStalled().Error(); got == "" || got[:6] != "win32:" {
		t.Errorf("errDropStalled = %q", got)
	}
	if dropStallTimeout > 30*time.Second || old < time.Second {
		t.Errorf("the default timeout %v is not a few seconds", old)
	}
}

func TestDarkBarStates(t *testing.T) {
	for _, tc := range []struct {
		state uint32
		want  barState
	}{
		{0, barNormal},
		{odsHotLight, barHot},
		{odsSelected, barPressed},
		{odsSelected | odsHotLight, barPressed},
		{odsGrayed, barDisabled},
		{odsDisabled | odsHotLight, barDisabled},
		{odsNoAccel, barNormal},
	} {
		if got := barItemState(tc.state); got != tc.want {
			t.Errorf("barItemState(%#x) = %v, want %v", tc.state, got, tc.want)
		}
	}
	if drawTextFlags(0)&0x00100000 != 0 || drawTextFlags(odsNoAccel)&0x00100000 == 0 {
		t.Errorf("DT_HIDEPREFIX follows ODS_NOACCEL: %#x, %#x", drawTextFlags(0), drawTextFlags(odsNoAccel))
	}
	if !darkBarOn(true, false) || darkBarOn(false, false) || darkBarOn(true, true) {
		t.Errorf("darkBarOn: dark mode without high contrast only")
	}
	if rgb(1, 2, 3) != 0x030201 {
		t.Errorf("rgb(1,2,3) = %#x, want COLORREF 0x030201", uint32(rgb(1, 2, 3)))
	}
	if barNormal.fill() != darkBar.background || barHot.fill() == darkBar.background ||
		barDisabled.ink() == darkBar.text || barHot.ink() != darkBar.text {
		t.Errorf("bar colors are mixed up")
	}
}

func TestIconNeedsRedraw(t *testing.T) {
	for _, tc := range []struct {
		owned      bool
		images     int
		have, want int
		redraw     bool
	}{
		{true, 2, 16, 32, true},
		{true, 2, 32, 32, false},
		{false, 2, 16, 32, false}, // the executable's icon: the shell scales it
		{true, 0, 16, 32, false},  // nothing to draw from
		{true, 1, 16, 0, false},   // an unknown side
	} {
		if got := iconNeedsRedraw(tc.owned, tc.images, tc.have, tc.want); got != tc.redraw {
			t.Errorf("iconNeedsRedraw(%+v) = %v", tc, got)
		}
	}
}
