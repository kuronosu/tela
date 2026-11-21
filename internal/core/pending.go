package core

import (
	"fmt"
	"time"
)

// Pending names what keeps the tree from being idle (ARCHITECTURE §3.5,
// §8 "Scheduler"; ADR-291): queued updates, changed elements or boxes
// waiting for a frame, running animations (endless or not), scroll
// animations, debug overlays asking for frames, and each deadline of the
// tree (a long press, overlay scrollbars waiting to fade, a caret blink,
// every UseInterval and UseTimeout running, a drag's autoscroll), with its
// distance from now on the tree's clock. It is empty for an idle tree.
// telatest's App.Pending and the idle budgets read it; what belongs to the
// window and the Host (a frame request, events to deliver, the Host's
// timer) is theirs to add.
func (t *Tree) Pending(now time.Duration) []string {
	var why []string
	if t.updatesQueued() {
		why = append(why, "queued updates (state or Value updates, or functions given to tela.Post, not yet applied)")
	}
	if t.changed {
		why = append(why, "changed elements or boxes waiting for a frame")
	} else if t.repaint {
		why = append(why, "boxes waiting to be repainted")
	}
	if t.Animating() {
		if t.anim.tk.OnlyEndless() {
			why = append(why, "an endless animation (animate-spin, animate-pulse, animate-ping, animate-bounce): it asks for a frame at every vsync")
		} else {
			why = append(why, "a running transition or animation")
		}
	}
	if t.scrollAnimating() {
		why = append(why, "a scroll animation, fling or scrollbar fade running")
	}
	if t.DebugWantsFrame() {
		why = append(why, "the debug overlays (TELA_DEBUG) asking for frames")
	}
	in := func(at time.Duration) string {
		if at <= now {
			return "due now"
		}
		return "in " + (at - now).String()
	}
	if at, ok := t.in.arena.NextDeadline(); ok {
		why = append(why, "a gesture deadline (a long press) "+in(at))
	}
	if at, ok := t.scrollDeadline(); ok {
		why = append(why, "overlay scrollbars that start fading "+in(at))
	}
	if at, ok := t.caretDeadline(); ok {
		why = append(why, fmt.Sprintf("the caret of the focused text field, which blinks next %s (it stops %v after the last input)", in(at), caretIdle))
	}
	for _, s := range t.intervals {
		why = append(why, t.describeInterval(s)+", next "+in(s.next))
	}
	if at, ok := t.dragAutoscrollDeadline(); ok {
		why = append(why, "the autoscroll of a drag "+in(at))
	}
	for _, r := range t.AsyncRuns() {
		why = append(why, r+" in flight (its goroutine has not returned)")
	}
	return why
}

// describeInterval names a running interval slot: its hook, its period,
// its component and, in dev mode, where the hook is called.
func (t *Tree) describeInterval(s *intervalSlot) string {
	el := s.eff.el
	var what string
	switch s.kind() {
	case hookInterval:
		what = fmt.Sprintf("a UseInterval every %v", s.d)
	default:
		what = fmt.Sprintf("a %s of %v", s.kind(), s.d)
	}
	what += " in " + t.describe(el)
	for i, h := range el.hooks {
		if h == any(s) {
			if i < len(el.hookDev) {
				what += " (called at " + el.hookSite(i) + ")"
			}
			break
		}
	}
	return what
}
