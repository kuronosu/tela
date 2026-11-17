// Package idle is the idle budget of the examples' screens (ARCHITECTURE
// §3.5 "0% CPU when idle", §8 "Scheduler"; ADR-290): once a screen has
// settled and nobody touches it, it runs no frame, builds, styles, lays out
// and paints nothing, and has nothing pending: no frame request, no running
// animation, no deadline, no timer armed. The budget is counted on the
// app's clock, which only moves when the test moves it, so it is
// deterministic and runs in the gate.
//
// It is checked twice. [Watch] and [Zero] read the work through telatest,
// as an app's own test would. [Mount] puts the screen on core.Host over the
// headless backend (internal/testutil/rawwin) for what telatest does not
// show: whether a frame is requested and whether the host's timer is armed
// ([Host.Busy]).
//
// It imports telatest, so only tests of packages above telatest (the
// examples) may use it.
package idle

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

// The stretch of app time the budget watches a settled screen for: Frames
// steps of Frame (a second of frames at 60 Hz), then one Long step.
const (
	Frame  = time.Second / 60
	Frames = 60
	Long   = 5 * time.Second
	// stretch names the stretch in messages.
	stretch = "1 s of frames and 5 s more"
)

// settleLimit bounds the app time Mount and Host.SettleEndless wait for a
// screen to settle.
const settleLimit = 10 * time.Second

// Watch moves the clock of a settled app through the stretch and returns
// the work its tree did meanwhile.
func Watch(a *telatest.App) telatest.Stats {
	return a.Measure(func() {
		for range Frames {
			a.Advance(Frame)
		}
		a.Advance(Long)
	})
}

// Zero fails tb when w, the work of a settled screen over the stretch, is
// any work at all. A restyle-only frame also counts a layout pass
// (Stats.Layouts counts the phases, empty ones too), so a frame shows here
// as a frame, a style or a paint even when it lays out nothing.
func Zero(tb testing.TB, screen string, w telatest.Stats) {
	tb.Helper()
	if s := Work(w); s != "" {
		tb.Errorf("%s is not idle: in %s of app time after it settled, with nobody touching it, it ran %s",
			screen, stretch, s)
	}
}

// Work names the work in w ("" when there is none): frames, builds,
// mounts, styles, layout passes, boxes laid out and paint passes.
func Work(w telatest.Stats) string {
	var parts []string
	add := func(n int, one, many string) {
		switch n {
		case 0:
		case 1:
			parts = append(parts, "1 "+one)
		default:
			parts = append(parts, fmt.Sprintf("%d %s", n, many))
		}
	}
	add(w.Frames, "frame", "frames")
	add(w.Builds, "build", "builds")
	add(w.Mounts, "mount", "mounts")
	add(w.Unmounts, "unmount", "unmounts")
	add(w.Styles, "style computation", "style computations")
	add(w.Layouts, "layout pass", "layout passes")
	add(w.LaidOut, "box laid out", "boxes laid out")
	add(w.Paints, "paint pass", "paint passes")
	return strings.Join(parts, ", ")
}

// Host is a screen on core.Host over the headless backend.
type Host struct {
	tb  testing.TB
	B   *headless.Backend
	H   *core.Host
	Win *headless.Window
	// stalls counts the steps in a row that did not move the clock.
	stalls int
}

// Mount shows root in a focused headless window of w by h at scale 1 with
// the configuration cfg (dev mode on, rawwin.Mount), and runs its first
// frames.
func Mount(t *testing.T, root tela.Component, w, h float32, cfg core.Config) *Host {
	t.Helper()
	b, host, win := rawwin.Mount(t, root, w, h, 1, cfg)
	return &Host{tb: t, B: b, H: host, Win: win}
}

// Settle waits, moving the clock through every frame and timer, until
// nothing is pending (Busy is empty) or settleLimit of app time passed.
// The host's own rest timers (the trims of the framebuffer and of the
// rasterizer's scratch, a second after the last frame, ADR-241) fire
// during this wait.
func (h *Host) Settle() *Host {
	h.settle(func() bool { return len(h.Busy()) == 0 })
	return h
}

// SettleEndless waits, as Settle does, until the only thing left is an
// endless animation (animate-spin, animate-pulse) and the frames it asks
// for.
func (h *Host) SettleEndless() *Host {
	h.settle(h.OnlyEndless)
	return h
}

// OnlyEndless reports whether everything pending belongs to endless
// animations: their frame request, and no deadline, timer or other work.
func (h *Host) OnlyEndless() bool {
	t := h.H.Tree()
	_, timer := h.H.TimerArmed()
	_, deadline := t.NextDeadline()
	return t.Animating() && t.OnlyEndlessAnimations() && !timer && !deadline && !h.H.NeedsFrame()
}

// settle moves the clock to each frame and timer until done or settleLimit.
func (h *Host) settle(done func() bool) {
	end := h.B.Now() + settleLimit
	for {
		h.B.Drain(h.H)
		if done() || h.B.Now() >= end {
			return
		}
		h.step(end)
	}
}

// step moves the clock to the next frame or timer due, at most to end, and
// delivers what is due then.
func (h *Host) step(end time.Duration) {
	now := h.B.Now()
	next := end
	if at, ok := h.B.NextFrameTime(); ok && at < next {
		next = at
	}
	if at, ok := h.H.TimerArmed(); ok && at < next {
		next = at
	}
	if next > now {
		h.B.Advance(next - now)
		h.stalls = 0
	} else if h.stalls++; h.stalls > 1000 {
		h.tb.Fatalf("idle: a frame or timer stays due without the clock moving: %v", h.Busy())
	}
	h.B.Wake()
	h.B.Drain(h.H)
}

// Run moves the clock through the stretch, stopping at every frame and
// timer on the way, and calls each (if not nil) after each stop with the
// work of the tree since the previous stop. It returns the work of the
// whole stretch.
func (h *Host) Run(each func(w core.Stats)) core.Stats {
	t := h.H.Tree()
	first := t.Stats()
	last := first
	stop := func() {
		if each != nil {
			s := t.Stats()
			each(diff(last, s))
			last = s
		}
	}
	for range Frames {
		end := h.B.Now() + Frame
		for h.B.Now() < end {
			h.step(end)
			stop()
		}
	}
	for end := h.B.Now() + Long; h.B.Now() < end; {
		h.step(end)
		stop()
	}
	return diff(first, t.Stats())
}

// Busy names what keeps the app from being idle: events to deliver, a
// frame request, a dirty tree, a running animation, a deadline of the tree
// (UseInterval and UseTimeout, a caret blink, a long press, a scrollbar
// fade) and the host's timer. It is empty for an idle app.
func (h *Host) Busy() []string {
	var why []string
	now := h.B.Now()
	t := h.H.Tree()
	if h.B.Pending() {
		why = append(why, "events waiting to be delivered")
	}
	if h.Win.FrameRequested() {
		why = append(why, "a frame request")
	}
	if h.H.NeedsFrame() {
		why = append(why, "a configured window whose frame has not run")
	}
	if t.Dirty() {
		why = append(why, "a dirty tree (state updates, changed elements or boxes to repaint)")
	}
	if t.Animating() {
		if t.OnlyEndlessAnimations() {
			why = append(why, "an endless animation (animate-spin, animate-pulse, animate-ping, animate-bounce)")
		} else {
			why = append(why, "a running transition or animation")
		}
	}
	if at, ok := t.NextDeadline(); ok {
		why = append(why, fmt.Sprintf("a deadline of the tree in %v (a UseInterval or UseTimeout, a caret blink, a long press, a scrollbar fade)", at-now))
	}
	if at, ok := h.H.TimerArmed(); ok {
		why = append(why, fmt.Sprintf("the host's timer, armed for %v from now", at-now))
	}
	if at, ok := h.H.PausedWork(); ok {
		why = append(why, fmt.Sprintf("a paced commit of a window without frames in %v", at-now))
	}
	return why
}

// Idle runs the stretch and fails the test, naming what kept the app busy,
// when the app worked or had anything pending at any stop.
func (h *Host) Idle(screen string) {
	h.tb.Helper()
	if why := h.Busy(); len(why) > 0 {
		h.tb.Errorf("%s did not settle in %v of app time: %s", screen, settleLimit, strings.Join(why, "; "))
		return
	}
	presents := h.Win.Presents()
	var busy []string
	w := h.Run(func(core.Stats) {
		if busy == nil {
			busy = h.Busy()
		}
	})
	if len(busy) > 0 {
		h.tb.Errorf("%s is not idle: once settled it still had %s", screen, strings.Join(busy, "; "))
	}
	if s := Work(w); s != "" {
		h.tb.Errorf("%s is not idle: in %s of app time after it settled it ran %s", screen, stretch, s)
	}
	if n := h.Win.Presents() - presents; n != 0 {
		h.tb.Errorf("%s is not idle: it presented %d frames after it settled", screen, n)
	}
}

// Within reports whether every rectangle of damage lies inside r.
func Within(damage []image.Rectangle, r image.Rectangle) bool {
	for _, d := range damage {
		if !d.In(r) {
			return false
		}
	}
	return true
}

// diff returns after minus before, field by field.
func diff(before, after core.Stats) core.Stats {
	return core.Stats{
		Frames: after.Frames - before.Frames, Builds: after.Builds - before.Builds, Skips: after.Skips - before.Skips,
		Mounts: after.Mounts - before.Mounts, Unmounts: after.Unmounts - before.Unmounts,
		Styles: after.Styles - before.Styles, Structures: after.Structures - before.Structures,
		Shapes: after.Shapes - before.Shapes, Breaks: after.Breaks - before.Breaks,
		Layouts: after.Layouts - before.Layouts, Paints: after.Paints - before.Paints,
		LaidOut: after.LaidOut - before.LaidOut, Remapped: after.Remapped - before.Remapped, Placed: after.Placed - before.Placed,
	}
}
