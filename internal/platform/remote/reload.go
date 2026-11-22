package remote

import (
	"bytes"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Desktop services across reloads (protocol 19, ADR-310). What a reload
// must neither lose nor replay, host side:
//
//   - The final effect of a drop: the child's SetDropEffect while it
//     handles the DragDrop (OnDrop) is sent back with msgDropDone, and the
//     host waits for it before its handler of the real DragDrop returns
//     (awaitDrop; see the top of phase13.go).
//   - A window a new build opens only after its promotion (from an
//     effect, which runs after the first frame): at the promotion a real
//     window that the new build has no window for, with no window of that
//     key opened anew either, waits for it (an orphan) instead of closing,
//     until ReloadGrace has passed; it keeps showing the old build's last
//     frame, takes no input, and a close request closes it.
//   - A dialog opened at startup (a file dialog or a message box asked
//     for during a child's start: while it is pending, and until
//     ReloadGrace after its promotion or, for a child that connected
//     active, after its first frame): the next build's start that asks
//     for the same dialog (the same operation and arguments, in order)
//     takes it over while it is open, and gets the same answer at once
//     once it was answered, instead of showing it again. Other dialogs,
//     and the same one asked for twice, are shown.
//   - A frame for a starting build under a window that gets none:
//     offscreenFrame gives it at once when the real window is hidden,
//     minimized or suspended (Wayland's suspended state), and after
//     a moment (defaultFrameFallback) when the real window delivers none without saying why
//     (a minimized Wayland window on a compositor without the suspended
//     state).
//   - The real window's alpha (platform.AlphaPresenter: the Windows 11 and
//     macOS client-area backdrops) goes to its child windows with
//     msgWindowInfo, followed by a WindowStateEvent, so the child renders
//     a transparent canvas where the real window shows its alpha. A child
//     window whose backdrop is not yet the real window's (a starting build
//     that changed it) is told false until its promotion applies it.
//
// Global shortcuts whose registration was taken, refused, revoked or
// failed are registered again for the next build (phase13.go).

// defaultReloadGrace is HostOptions.ReloadGrace's default: enough for the
// effects of a new build's first frames, short enough that a window the
// new build no longer opens does not linger.
const defaultReloadGrace = time.Second

// dropDoneTimeout bounds the host's wait for the final effect of a drop,
// on its UI thread (the child answers from its handler of the drop, a
// round trip of well under a millisecond on an idle machine).
const dropDoneTimeout = time.Second

// defaultFrameFallback is how long a starting build waits for a frame of
// its real window before the host gives it one offscreen.
const defaultFrameFallback = 100 * time.Millisecond

func (h *Host) reloadGrace() time.Duration {
	if h.opts.ReloadGrace > 0 {
		return h.opts.ReloadGrace
	}
	return defaultReloadGrace
}

// --- the drop's final effect ---

// dropDone is a child's msgDropDone.
type dropDone struct {
	id, session uint32
	effect      platform.DropEffect
}

// dropDone decodes a child's msgDropDone (reader goroutine) and hands it
// to the UI thread's awaitDrop, which may be waiting for it; one that
// comes late is dropped by the next wait.
func (c *Child) dropDone(d *dec) error {
	r := dropDone{id: d.u32(), session: d.u32(), effect: platform.DropEffect(d.u8())}
	if err := d.end(); err != nil {
		return err
	}
	if err := try(func() { platform.CheckDropEffect("the drop's final effect", r.effect, true) }); err != nil {
		return err
	}
	select {
	case c.drops <- r:
	default:
	}
	return nil
}

// drainDrops drops the answers of earlier drops that no wait took (a drop
// delivered from the queue of a frozen host, a wait that timed out), so
// they cannot fill the channel before the next drop's answer.
func drainDrops(c *Child) {
	for {
		select {
		case <-c.drops:
		default:
			return
		}
	}
}

// awaitDrop waits for the final effect of the drop just delivered to the
// active child window of rw and sets it on the real window, whose backend
// reads it once the host's handler of the DragDrop returns. UI thread; it
// blocks for at most dropDoneTimeout.
func (h *Host) awaitDrop(rw *realWin) {
	cw := rw.active
	if cw == nil || cw.dropAwait == 0 {
		return
	}
	session := cw.dropAwait
	cw.dropAwait = 0
	t := time.NewTimer(dropDoneTimeout)
	defer t.Stop()
	for {
		select {
		case r := <-cw.c.drops:
			if r.id != cw.id || r.session != session {
				continue // a late answer of an earlier drop
			}
			if !rw.closed {
				rw.w.SetDropEffect(r.effect)
			}
			return
		case <-cw.c.c.done:
			return
		case <-t.C:
			return // the answer before the drop stands
		}
	}
}

// --- windows a new build opens later ---

// orphan makes rw, which no window of the newly promoted child took over,
// wait for one until the reload grace is over.
func (h *Host) orphan(rw *realWin) {
	rw.orphanGen++
	gen := rw.orphanGen
	rw.orphan, rw.orphanDone = true, false
	time.AfterFunc(h.reloadGrace(), func() {
		h.post(func() {
			if rw.orphan && rw.orphanGen == gen {
				rw.orphanDone = true
				h.prune()
			}
		})
	})
}

// adopt ends rw's wait: a child window became its active one.
func (rw *realWin) adopt() {
	rw.orphanGen++
	rw.orphan, rw.orphanDone = false, false
}

// orphanInput handles an input event of rw while it waits for a window:
// a close request closes it (the app that would decide is gone); the rest
// is dropped. It reports whether rw is such a window.
func (h *Host) orphanInput(rw *realWin, e platform.Event) bool {
	if !rw.orphan {
		return false
	}
	if _, ok := e.(platform.CloseRequestedEvent); ok {
		h.closeReal(rw)
	}
	return true
}

// unclaimed closes or orphans the real windows c, promoted with windows,
// did not take over: a window whose key c's windows have (c opened one
// anew: its creation options changed) closes, and so does one no active
// child ever had; the others wait for c to open them (orphan).
func (h *Host) unclaimed(c *Child, used map[*realWin]bool) {
	keys := map[string]bool{}
	for _, cw := range c.windows {
		keys[cw.key] = true
	}
	for _, rw := range slices.Clone(h.order) {
		if rw.closed || used[rw] || rw.active != nil || len(rw.pending) > 0 {
			continue
		}
		if keys[rw.key] || !rw.wasActive {
			h.closeReal(rw)
			continue
		}
		h.orphan(rw)
	}
}

// --- dialogs opened at startup ---

// hostDialog is a dialog a child asked for during its start, held so the
// start of the next build can take it over (see the top of reload.go).
type hostDialog struct {
	op  uint8
	req []byte // the request's arguments, as encoded
	// claims are the children that asked for it, waiting (the answer goes
	// to those not detached, each with its token) or answered.
	claims  []dialogClaim
	claimed map[*Child]bool
	done    bool
	result  []byte // the answer after the token: outcome and fields
}

type dialogClaim struct {
	c     *Child
	token uint32
}

// startDialog decides about a dialog c asks for: show false means it is
// not shown, because c takes over the dialog the build it replaces asked
// for at its start (an answered one's answer is sent at once). Otherwise
// rec, when not nil, is the record of a startup dialog, whose answer
// method answers it (and which is dropped if showing it fails); nil means
// a dialog after c's start, answered as any other.
func (h *Host) startDialog(c *Child, op uint8, req []byte, token uint32) (rec *hostDialog, show bool) {
	if !h.inStart(c) {
		return nil, true
	}
	if from := c.from; from != nil {
		for _, d := range from.startDialogs {
			if d.claimed[c] || d.op != op || !bytes.Equal(d.req, req) {
				continue
			}
			d.claimed[c] = true
			c.startDialogs = append(c.startDialogs, d)
			if d.done {
				e := &enc{}
				e.u32(token)
				e.b = append(e.b, d.result...)
				c.ctl.post(kindDesktopDone, e.b)
			} else {
				d.claims = append(d.claims, dialogClaim{c, token})
			}
			return nil, false
		}
	}
	d := &hostDialog{op: op, req: req, claims: []dialogClaim{{c, token}}, claimed: map[*Child]bool{c: true}}
	c.startDialogs = append(c.startDialogs, d)
	return d, true
}

// answer is the answer of a startup dialog: kept for the next builds and
// sent to the children waiting for it that are not detached. It runs on
// the UI thread (the backends answer there).
func (d *hostDialog) answer(err error, fill func(*enc)) {
	e := &enc{}
	encodeOutcome(e, err)
	if err == nil {
		fill(e)
	}
	d.done, d.result = true, e.b
	for _, cl := range d.claims {
		if cl.c.state == stateDetached {
			continue
		}
		m := &enc{}
		m.u32(cl.token)
		m.b = append(m.b, d.result...)
		cl.c.ctl.post(kindDesktopDone, m.b)
	}
	d.claims = nil
}

// dropStartDialog forgets a startup dialog that could not be shown.
func (c *Child) dropStartDialog(d *hostDialog) {
	c.startDialogs = slices.DeleteFunc(c.startDialogs, func(x *hostDialog) bool { return x == d })
}

// inStart reports whether c is starting: pending, or active until the
// reload grace after its promotion (after its first frame for a child
// that connected active).
func (h *Host) inStart(c *Child) bool {
	switch c.state {
	case statePending:
		return true
	case stateActive:
		return c.startEnd == 0 || h.b.Now() < c.startEnd
	}
	return false
}

// startClock starts the end of c's start once it is active and has shown
// its first frame.
func (h *Host) startClock(c *Child) {
	if c.state == stateActive && c.firstSeen && c.startEnd == 0 {
		c.startEnd = h.b.Now() + h.reloadGrace()
	}
}

// becameActive records c as the build the next ones replace: their start
// takes over its startup dialogs.
func (h *Host) becameActive(c *Child) {
	if old := h.lastActive; old != nil && old != c {
		old.from = nil // a chain of one: the replaced build's own source is done with
	}
	h.lastActive = c
	h.startClock(c)
}

// --- the real window's alpha ---

// presentsAlpha reports whether w shows the alpha of the frames.
func presentsAlpha(w platform.Window) bool {
	p, ok := w.(platform.AlphaPresenter)
	return ok && p.PresentsAlpha()
}

// childAlpha is what cw is told about rw's alpha: rw's, while cw's
// backdrop is the one rw has.
func childAlpha(rw *realWin, cw *childWin) bool {
	return rw.alpha && cw.backdrop == rw.backdrop
}

// syncAlpha reads rw's alpha again and tells the child windows on it whose
// answer changed: a msgWindowInfo, then a WindowStateEvent.
func (h *Host) syncAlpha(rw *realWin) {
	if rw.closed {
		return
	}
	rw.alpha = presentsAlpha(rw.w)
	for _, cw := range rw.attached() {
		if childAlpha(rw, cw) != cw.alpha {
			h.sendInfo(rw, cw)
			h.syncState(rw, cw)
		}
	}
}

// --- frames for a starting build ---

// armFrameFallback gives cw, a starting build's window waiting for a frame
// of rw, one offscreen if rw delivers none within h.frameFallback.
func (h *Host) armFrameFallback(rw *realWin, cw *childWin) {
	if cw.fallback {
		return
	}
	cw.fallback = true
	time.AfterFunc(h.frameFallback, func() {
		h.post(func() {
			cw.fallback = false
			if cw.rw != rw || rw.closed || !h.wantsOffscreen(cw) {
				return
			}
			cw.wantFrame = false
			h.deliver(cw, platform.FrameEvent{Time: h.b.Now()})
		})
	})
}

// wantsOffscreen reports whether cw is the visible window of a starting
// build, configured, that asked for a frame before its first one.
func (h *Host) wantsOffscreen(cw *childWin) bool {
	if !cw.visible || !cw.wantFrame || cw.c.state != statePending || cw.sent.Scale == 0 {
		return false
	}
	select {
	case <-cw.c.firstFrame:
		return false
	default:
		return true
	}
}
