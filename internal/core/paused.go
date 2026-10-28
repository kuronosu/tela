package core

import "time"

// Windows without frames (ADR-185).
//
// A window's tree applies its updates, builds, lays out, runs its layout
// effects and paints in its frames (Host.frame), and runs its passive
// effects after them; the platform paces the frames. A window the app hid
// (WindowHandle.Hide, Hidden), a minimized one and the child of a
// minimized one (Windows hides owned windows with their owner, and X11
// window managers iconify transients with theirs) get none: every backend
// pauses their frames (internal/loop's SetHidden, an empty size), and the
// remote host drops them. Their trees are still the app's: a tray handler
// sets state in a hidden main window and its tray menu must follow, a
// setting changed in another window must reach it, an effect must store
// what changed, a Window declared in it must open, a timer's update must
// apply.
//
// So after every event (HandleEvent), the Host commits the work such a
// window has: components marked for building, effects queued by builds,
// Windows to open, change or close. A commit is the frame's pipeline
// without the raster and the present: Tree.Frame (updates, build, style,
// layout, layout effects and paint into display lists, so that the
// invariants of a frame hold: bounds, semantics, focus, hit regions)
// followed by FlushEffects. Nothing is rasterized: the window's
// framebuffer becomes stale, and the frame requested now, delivered once
// the window shows, rasterizes in full, so the window presents the
// current state whole and the damage of later frames starts from it.
// Layout-only and paint-only work (a resize, a Value read by a style
// closure) waits for that frame: nothing outside the window sees it.
// The animation clock does not advance without frames: transitions and
// springs started meanwhile run from where they are once the window
// shows.
//
// Commits are paced like frames: one per window per pausedInterval of the
// backend clock. The first after a quiet time runs at once; one asked for
// sooner waits for the Host's timer (nextDeadline), so a tree whose
// effects set state in a loop, or a goroutine that sets state all the
// time, costs no more while hidden than while shown.
//
// A window commits only after its first ConfigureEvent, which every
// backend delivers at once, hidden windows included: the tree then has
// the window's size and scale. Wayland cannot report a minimized window:
// the compositor's frame callbacks decide when its work runs.

// pausedInterval is the pace of the commits of a window without frames:
// the backends' default refresh interval.
const pausedInterval = time.Second / 60

// maxPausedCommits bounds the commits of one commitPaused call. Each
// window commits at most once per call, so only windows opened by the
// commits themselves, without end, reach it.
const maxPausedCommits = 64

// paused reports whether window v gets no frames: the app hid it, or it
// or a window it belongs to is minimized.
func (v *winView) paused() bool {
	if v.hidden {
		return true
	}
	for w := v; w != nil; w = w.parent {
		if w.os.Minimized {
			return true
		}
	}
	return false
}

// pausedWork reports whether v is an open window without frames whose
// tree has work to commit.
func (v *winView) pausedWork() bool {
	return v.sized && v.paused() && !v.dead.Load() && v.tree.commitPending()
}

// commitPending reports whether the tree has work that the app observes
// without its pixels: components to build, effects to run, Windows to
// open, change or close.
func (t *Tree) commitPending() bool {
	return len(t.buildQ) > 0 || len(t.winQ) > 0 ||
		len(t.layoutFx) > 0 || len(t.passiveFx) > 0 || len(t.readyFx) > 0 ||
		len(t.layoutGone) > 0 || len(t.passiveGone) > 0 || len(t.readyGone) > 0
}

// commitPaused commits the work of the windows without frames that is
// due, in window order, and arms the timer for the next one. It runs
// after each event, outside every handler and frame.
func (h *Host) commitPaused() {
	if h.ended {
		return
	}
	some := false
	for _, v := range h.views {
		if v.paused() {
			some = true
			break
		}
	}
	if !some {
		return
	}
	now := h.backend.Now()
	for range maxPausedCommits {
		v := h.duePaused(now)
		if v == nil {
			break
		}
		h.commitView(v, now)
	}
	h.armTimer()
}

// duePaused returns the first window without frames whose commit is due
// at now, or nil. It looks at h.views again on each call: a commit may
// open or close windows.
func (h *Host) duePaused(now time.Duration) *winView {
	for _, v := range h.views {
		if v.commitAt <= now && v.pausedWork() {
			return v
		}
	}
	return nil
}

// commitView commits the work of window v without a frame: the frame's
// pipeline up to paint, then the passive effects. The window's next frame
// rasterizes fully.
func (h *Host) commitView(v *winView, now time.Duration) {
	v.commitAt = now + pausedInterval
	t := v.tree
	if _, changed := t.Frame(); changed {
		v.stale = true
		v.win.RequestFrame() // delivered once the window shows
	}
	t.FlushEffects()
}

// PausedWork reports whether a window that gets no frames (hidden,
// minimized, or the child of a minimized window) has work for the Host to
// commit, and the backend time of the earliest such commit: in the past
// when it runs after the next event, later when the pace delays it (the
// Host's timer then wakes the loop). Tests that drive the Host on a fake
// clock (telatest) advance the clock to it and wake the loop.
func (h *Host) PausedWork() (at time.Duration, ok bool) {
	if h.ended {
		return 0, false
	}
	for _, v := range h.views {
		if v.pausedWork() && (!ok || v.commitAt < at) {
			at, ok = v.commitAt, true
		}
	}
	return at, ok
}
