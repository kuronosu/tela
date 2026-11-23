package core

import (
	"fmt"
	"sort"
	"time"
)

// PendingKind says what keeps a tree from being idle (ADR-291, ADR-308):
// telatest's App.Pending gives it to tools that tell work that can still
// change the screen (an update, an animation, a timer, a load) from work
// that only redraws it (a caret blink, a scrollbar fade, an endless
// spinner).
type PendingKind uint8

const (
	// PendingUpdate: state or Value updates, or functions given to
	// tela.Post, queued and not yet applied.
	PendingUpdate PendingKind = iota + 1
	// PendingChange: changed elements or boxes waiting for a frame, or
	// boxes waiting to be repainted.
	PendingChange
	// PendingAnimation: a running transition or animation that ends.
	PendingAnimation
	// PendingEndless: only endless animations run (animate-spin...).
	PendingEndless
	// PendingScroll: a scroll animation or a fling.
	PendingScroll
	// PendingScrollbar: overlay scrollbars fading, or waiting to.
	PendingScrollbar
	// PendingDebug: the debug overlays (TELA_DEBUG) asking for frames.
	PendingDebug
	// PendingLongPress: a gesture deadline.
	PendingLongPress
	// PendingCaret: the caret of the focused text field blinking.
	PendingCaret
	// PendingTimer: a UseInterval or UseTimeout running.
	PendingTimer
	// PendingAutoscroll: the autoscroll of a drag.
	PendingAutoscroll
	// PendingAsync: a UseAsync run in flight.
	PendingAsync
	// PendingGo: a UseGo goroutine that has not returned.
	PendingGo
)

// PendingItem is one item of [Tree.Pending]: its kind, its description
// (with the distance of its deadline, if it has one) and that distance (0
// for work due now or without a deadline).
type PendingItem struct {
	Kind PendingKind
	What string
	In   time.Duration
}

// Pending names what keeps the tree from being idle (ARCHITECTURE §3.5,
// §8 "Scheduler"; ADR-291, ADR-308): queued updates, changed elements or
// boxes waiting for a frame, running animations (endless or not), scroll
// animations, debug overlays asking for frames, each deadline of the tree
// (a long press, overlay scrollbars waiting to fade, a caret blink, every
// UseInterval and UseTimeout running, a drag's autoscroll), with its
// distance from now on the tree's clock, and the goroutines of UseAsync
// runs and of UseGo that have not returned. It is empty for an idle tree.
// telatest's App.Pending and the idle budgets read it; what belongs to the
// window and the Host (a frame request, events to deliver, the Host's
// timer) is theirs to add.
func (t *Tree) Pending(now time.Duration) []PendingItem {
	var why []PendingItem
	add := func(k PendingKind, what string) { why = append(why, PendingItem{Kind: k, What: what}) }
	if t.updatesQueued() {
		add(PendingUpdate, "queued updates (state or Value updates, or functions given to tela.Post, not yet applied)")
	}
	if t.changed {
		add(PendingChange, "changed elements or boxes waiting for a frame")
	} else if t.repaint {
		add(PendingChange, "boxes waiting to be repainted")
	}
	if t.Animating() {
		if t.anim.tk.OnlyEndless() {
			add(PendingEndless, "an endless animation (animate-spin, animate-pulse, animate-ping, animate-bounce): it asks for a frame at every vsync")
		} else {
			add(PendingAnimation, "a running transition or animation")
		}
	}
	moving, fading := t.scrollMotion()
	if moving {
		add(PendingScroll, "a scroll animation or fling running")
	}
	if fading {
		add(PendingScrollbar, "overlay scrollbars fading")
	}
	if t.DebugWantsFrame() {
		add(PendingDebug, "the debug overlays (TELA_DEBUG) asking for frames")
	}
	in := func(at time.Duration) string {
		if at <= now {
			return "due now"
		}
		return "in " + (at - now).String()
	}
	timed := func(k PendingKind, at time.Duration, what string) {
		why = append(why, PendingItem{Kind: k, What: what, In: max(at-now, 0)})
	}
	if at, ok := t.in.arena.NextDeadline(); ok {
		timed(PendingLongPress, at, "a gesture deadline (a long press) "+in(at))
	}
	if at, ok := t.scrollDeadline(); ok {
		timed(PendingScrollbar, at, "overlay scrollbars that start fading "+in(at))
	}
	if at, ok := t.caretDeadline(); ok {
		timed(PendingCaret, at, fmt.Sprintf("the caret of the focused text field, which blinks next %s (it stops %v after the last input)", in(at), caretIdle))
	}
	for _, s := range t.intervals {
		timed(PendingTimer, s.next, t.describeInterval(s)+", next "+in(s.next))
	}
	if at, ok := t.dragAutoscrollDeadline(); ok {
		timed(PendingAutoscroll, at, "the autoscroll of a drag "+in(at))
	}
	for _, r := range t.AsyncRuns() {
		add(PendingAsync, r+" in flight (its goroutine has not returned)")
	}
	for _, r := range t.goRunning() {
		add(PendingGo, r+" running (its function has not returned; a result it posts can still change the screen)")
	}
	return why
}

// scrollMotion reports whether a scroll container moves (an animation or
// a fling) and whether overlay scrollbars fade (scrollAnimating is either).
func (t *Tree) scrollMotion() (moving, fading bool) {
	for _, s := range t.scr.sets[setMoving] {
		if s.animating() {
			moving = true
			break
		}
	}
	for _, s := range t.scr.sets[setBars] {
		if s.bars.fading(t.scr.now) {
			fading = true
			break
		}
	}
	return moving, fading
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

// goRun is the goroutine of a UseGo slot, from its start to the return of
// its function (ADR-308). Unlike a UseAsync run, nothing waits for it: a
// UseGo goroutine may live as long as its component (a subscription), so
// it is only named, for the tools that must not give up on a screen that
// a result it posts can still change.
type goRun struct {
	el *Element
	i  int // the hook's index
}

// goStarted records that the goroutine of r started.
func (t *Tree) goStarted(r *goRun) {
	t.asyncMu.Lock()
	if t.goRuns == nil {
		t.goRuns = map[*goRun]struct{}{}
	}
	t.goRuns[r] = struct{}{}
	t.asyncMu.Unlock()
}

// goEnded records that the function of r returned. Any goroutine.
func (t *Tree) goEnded(r *goRun) {
	t.asyncMu.Lock()
	delete(t.goRuns, r)
	t.asyncMu.Unlock()
}

// goRunning names the UseGo goroutines that have not returned, sorted:
// "a UseGo goroutine in <Feed> (called at feed.go:21)". UI thread only.
func (t *Tree) goRunning() []string {
	t.asyncMu.Lock()
	runs := make([]*goRun, 0, len(t.goRuns))
	for r := range t.goRuns {
		runs = append(runs, r)
	}
	t.asyncMu.Unlock()
	names := make([]string, len(runs))
	for k, r := range runs {
		what := "a UseGo goroutine in " + t.describe(r.el)
		if r.i < len(r.el.hookDev) {
			what += " (called at " + r.el.hookSite(r.i) + ")"
		}
		names[k] = what
	}
	sort.Strings(names)
	return names
}
