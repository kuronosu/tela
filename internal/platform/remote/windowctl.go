package remote

import (
	"bytes"
	"fmt"
	"image"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Window control (phase 12, ADR-179, ADR-184).
//
// The child forwards every request to the host (msgWindowCtl), which
// keeps the child's view of the requests that are state (visible,
// always-on-top, size limits, icon, hit regions) and applies them to the
// real window while the child is the active one; at a promotion the new
// child's view replaces the old one (applyWindow). Requests that are
// actions (minimize, maximize, fullscreen, position, activate) reach the
// real window only from the active child: a build that is not shown yet
// must not move the window under the one that is. The outcome comes back
// as the real window's WindowStateEvent and ConfigureEvent, which every
// child mapped onto it gets; the host keeps the latest state and gives it
// to a child that maps onto the window later, after its ConfigureEvent.

// --- child side ---

// State returns the state delivered by the latest WindowStateEvent.
func (w *Window) State() platform.WindowState {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.WindowState.State()
}

// ctl sends a window control request if the window is open.
func (w *Window) ctl(op uint8, fill func(*enc)) {
	if !w.open() {
		return
	}
	w.send(newMsg(msgWindowCtl), func(e *enc) {
		e.u8(op)
		if fill != nil {
			fill(e)
		}
	})
}

// Minimize asks the host to minimize its window.
func (w *Window) Minimize() { w.ctl(ctlMinimize, nil) }

// SetMaximized asks the host to maximize its window or restore it.
func (w *Window) SetMaximized(maximized bool) {
	w.ctl(ctlMaximized, func(e *enc) { e.boolean(maximized) })
}

// SetFullscreen asks the host to make its window fullscreen or leave
// fullscreen.
func (w *Window) SetFullscreen(fullscreen bool) {
	w.ctl(ctlFullscreen, func(e *enc) { e.boolean(fullscreen) })
}

// SetVisible shows or hides the window. A hidden window gets no
// FrameEvents; a frame requested while hidden is delivered once it is
// shown again.
func (w *Window) SetVisible(visible bool) {
	b := w.b
	b.mu.Lock()
	if w.WindowState.Closed() || w.visible == visible {
		b.mu.Unlock()
		return
	}
	w.visible = visible
	owed := visible && w.frameOwed
	if owed {
		w.frameOwed, w.frameAsked = false, true
	}
	b.mu.Unlock()
	w.send(newMsg(msgWindowCtl), func(e *enc) { e.u8(ctlVisible); e.boolean(visible) })
	if owed {
		w.send(newMsg(msgRequestFrame))
	}
}

// SetAlwaysOnTop asks the host to keep its window above normal windows.
func (w *Window) SetAlwaysOnTop(onTop bool) {
	b := w.b
	b.mu.Lock()
	skip := w.WindowState.Closed() || w.onTop == onTop
	w.onTop = onTop
	b.mu.Unlock()
	if !skip {
		w.ctl(ctlOnTop, func(e *enc) { e.boolean(onTop) })
	}
}

// SetMinSize changes the minimum content size. Invalid sizes panic.
func (w *Window) SetMinSize(size platform.Size) {
	platform.CheckSize("SetMinSize size", size)
	b := w.b
	b.mu.Lock()
	skip := w.WindowState.Closed() || w.min == size
	w.min = size
	b.mu.Unlock()
	if !skip {
		w.ctl(ctlMinSize, func(e *enc) { e.size(size) })
	}
}

// SetMaxSize changes the maximum content size. Invalid sizes panic.
func (w *Window) SetMaxSize(size platform.Size) {
	platform.CheckSize("SetMaxSize size", size)
	b := w.b
	b.mu.Lock()
	skip := w.WindowState.Closed() || w.max == size
	w.max = size
	b.mu.Unlock()
	if !skip {
		w.ctl(ctlMaxSize, func(e *enc) { e.size(size) })
	}
}

// SetPosition asks the host to move its window (physical desktop pixels).
func (w *Window) SetPosition(pos image.Point) {
	w.ctl(ctlPosition, func(e *enc) { e.i64(int64(pos.X)); e.i64(int64(pos.Y)) })
}

// Activate asks the host to raise and focus its window.
func (w *Window) Activate() { w.ctl(ctlActivate, nil) }

// SetIcon sets the window's icon. The images are encoded (copied) before
// it returns. Invalid images panic.
func (w *Window) SetIcon(images []image.Image) {
	platform.CheckIcon("SetIcon", images)
	w.ctl(ctlIcon, func(e *enc) { e.images(images) })
}

// SetTitleBarColors asks the host to color its window's title bar and
// border (Windows 11); equal requests are not resent.
func (w *Window) SetTitleBarColors(colors platform.TitleBarColors) {
	b := w.b
	b.mu.Lock()
	skip := w.WindowState.Closed() || w.titleBar == colors
	w.titleBar = colors
	b.mu.Unlock()
	if !skip {
		w.ctl(ctlTitleBar, func(e *enc) { e.titleBar(colors) })
	}
}

// SetBackdrop asks the host to give its window a system backdrop. An
// invalid Backdrop panics.
func (w *Window) SetBackdrop(backdrop platform.Backdrop) {
	platform.CheckBackdrop("SetBackdrop backdrop", backdrop)
	b := w.b
	b.mu.Lock()
	skip := w.WindowState.Closed() || w.backdrop == backdrop
	w.backdrop = backdrop
	b.mu.Unlock()
	if !skip {
		w.ctl(ctlBackdrop, func(e *enc) { e.u8(uint8(backdrop)) })
	}
}

// SetHitRegions declares the custom title bar's regions; equal lists are
// not resent. Invalid regions panic.
func (w *Window) SetHitRegions(regions []platform.HitRegion) {
	platform.CheckHitRegions(regions)
	b := w.b
	b.mu.Lock()
	skip := w.WindowState.Closed() || platform.EqualHitRegions(w.regions, regions)
	if !skip {
		w.regions = slices.Clone(regions)
	}
	b.mu.Unlock()
	if !skip {
		w.ctl(ctlHitRegions, func(e *enc) { e.regions(regions) })
	}
}

// RestoreToken returns the restore token of the host's real window ("" until
// the host told it).
func (w *Window) RestoreToken() string {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.token
}

// HostHidden waits until the host mapped the window onto a real window (at
// most timeout) and reports whether that real window was hidden then: a
// window that takes over the real window of a previous build which the
// user hid (close to tray) finds it hidden. ok is false if the host did
// not answer in time or the connection ended. The tela dev child runtime
// uses it to adopt that state (ADR-184).
func (w *Window) HostHidden(timeout time.Duration) (hidden, ok bool) {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-w.info:
	case <-w.b.c.done:
		return false, false
	case <-t.C:
		return false, false
	}
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.hostHidden, true
}

// --- host side ---

// windowCtl decodes a window control request of the child (reader
// goroutine) and posts it to the UI thread.
func (c *Child) windowCtl(d *dec) error {
	h := c.h
	id, op := d.u32(), d.u8()
	var (
		flag    bool
		size    platform.Size
		pos     image.Point
		icon    []image.Image
		regions []platform.HitRegion
		colors  platform.TitleBarColors
		backdrp platform.Backdrop
	)
	switch op {
	case ctlMinimize, ctlActivate:
	case ctlMaximized, ctlFullscreen, ctlVisible, ctlOnTop:
		flag = d.boolean()
	case ctlMinSize, ctlMaxSize:
		size = d.size()
		if d.err == nil {
			if err := try(func() { platform.CheckSize("size limit", size) }); err != nil {
				return err
			}
		}
	case ctlPosition:
		pos = image.Point{X: int(d.i64()), Y: int(d.i64())}
	case ctlIcon:
		icon = d.images()
	case ctlTitleBar:
		colors = d.titleBar()
	case ctlBackdrop:
		backdrp = d.backdrop()
	case ctlHitRegions:
		regions = d.regions()
		if d.err == nil {
			if err := try(func() { platform.CheckHitRegions(regions) }); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("remote: unknown window control operation %d", op)
	}
	return c.op(d, id, func(cw *childWin, rw *realWin, active bool) {
		switch op {
		case ctlMinimize:
			if active {
				rw.w.Minimize()
			}
		case ctlMaximized:
			if active {
				rw.w.SetMaximized(flag)
			}
		case ctlFullscreen:
			if active {
				rw.w.SetFullscreen(flag)
			}
		case ctlPosition:
			if active {
				rw.w.SetPosition(pos)
			}
		case ctlActivate:
			if active {
				rw.w.Activate()
			}
		case ctlVisible:
			cw.visible = flag
			if active {
				h.applyVisible(rw, flag)
			}
			if flag && cw.wantFrame {
				rw.w.RequestFrame()
				h.offscreenFrame(rw, cw)
			}
			if !flag {
				h.checkHidden(cw.c)
			}
		case ctlOnTop:
			cw.onTop = flag
			if active {
				h.applyWindow(rw, cw)
			}
		case ctlMinSize:
			cw.min = size
			if active {
				h.applyWindow(rw, cw)
			}
		case ctlMaxSize:
			cw.max = size
			if active {
				h.applyWindow(rw, cw)
			}
		case ctlIcon:
			cw.icon = icon
			if active {
				h.applyWindow(rw, cw)
			}
		case ctlTitleBar:
			cw.titleBar = colors
			if active {
				h.applyWindow(rw, cw)
			}
		case ctlBackdrop:
			cw.backdrop = backdrp
			if active {
				h.applyWindow(rw, cw)
			}
		case ctlHitRegions:
			cw.regions = regions
			if active {
				h.applyWindow(rw, cw)
			}
		}
	})
}

// applyVisible shows or hides the real window.
func (h *Host) applyVisible(rw *realWin, visible bool) {
	if rw.closed || rw.visible == visible {
		return
	}
	rw.visible = visible
	rw.w.SetVisible(visible)
	if visible {
		rw.full = true // the real window shows the retained frame; the next present is full anyway
	}
}

// applyWindow gives the real window the state requests of its active child
// window cw that differ from what the host last asked of it: when cw takes
// the window over, and when cw changes one of them.
func (h *Host) applyWindow(rw *realWin, cw *childWin) {
	if rw.closed {
		return
	}
	h.applyVisible(rw, cw.visible)
	if rw.onTop != cw.onTop {
		rw.onTop = cw.onTop
		rw.w.SetAlwaysOnTop(cw.onTop)
	}
	if rw.min != cw.min {
		rw.min = cw.min
		rw.w.SetMinSize(cw.min)
	}
	if rw.max != cw.max {
		rw.max = cw.max
		rw.w.SetMaxSize(cw.max)
	}
	if rw.titleBar != cw.titleBar {
		rw.titleBar = cw.titleBar
		rw.w.SetTitleBarColors(cw.titleBar)
	}
	if rw.backdrop != cw.backdrop {
		rw.backdrop = cw.backdrop
		rw.w.SetBackdrop(cw.backdrop)
	}
	if !equalIcons(rw.icon, cw.icon) { // none set: the default icon
		rw.icon = cw.icon
		rw.w.SetIcon(cw.icon)
	}
	if !platform.EqualHitRegions(rw.regions, cw.regions) {
		rw.regions = cw.regions
		rw.w.SetHitRegions(cw.regions)
	}
}

// equalIcons reports whether two decoded icons have the same images.
func equalIcons(a, b []image.Image) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, ok1 := a[i].(*image.NRGBA)
		y, ok2 := b[i].(*image.NRGBA)
		if !ok1 || !ok2 || x.Rect != y.Rect || !bytes.Equal(x.Pix, y.Pix) {
			return false
		}
	}
	return true
}
