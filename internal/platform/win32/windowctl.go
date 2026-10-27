//go:build windows

package win32

import (
	"fmt"
	"image"
	"slices"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Window control (phase 12, ADR-178, ADR-179): state requests, frameless
// windows with native hit-testing, parents and modality. The pure half
// (hit-testing, the non-client mouse messages, placement math, icon pixels)
// is frame.go. See "Window control" in the package documentation.

// showRequest is a state request made while the window is not shown:
// ShowWindow would show it, so it is applied by the show that the next
// Present or SetVisible(true) makes.
type showRequest uint8

const (
	showAsIs showRequest = iota
	showMinimized
	showMaximized
	showRestored
)

// fullscreenSave is what a fullscreen window returns to: its styles and
// placement (winit's and Chromium's borderless fullscreen; Raymond Chen's
// "How do I switch a window between normal and fullscreen?").
type fullscreenSave struct {
	style, exStyle uint32
	placement      windowPlacement
	offset         image.Point // workspace to screen offset of its monitor
	maximized      bool        // maximized when it leaves (SetMaximized changes it)
}

// --- creation ---

// parentOf checks WindowOptions.Parent: an open window of this backend.
func (b *Backend) parentOf(p platform.Window) *Window {
	if p == nil {
		return nil
	}
	pw, ok := p.(*Window)
	if !ok || pw.b != b {
		panic(fmt.Sprintf("win32: WindowOptions.Parent is a %T of another backend; the parent must be an open window of the same backend", p))
	}
	if pw.Closed() {
		panic("win32: WindowOptions.Parent is closed; the parent must be an open window")
	}
	return pw
}

// initialPlacement returns the DPI a new window is created at and the
// position of its outer top-left (cwUseDefault when Windows chooses): at
// WindowOptions.Position on the monitor there, centered over the parent
// (as dialog boxes are), or where Windows puts new windows on the primary
// monitor.
func (w *Window) initialPlacement() (x, y uintptr) {
	opts := w.opts
	x, y = cwUseDefault, cwUseDefault
	switch {
	case opts.HasPosition:
		w.dpi = monitorDPI(monitorFromRect(rect{int32(opts.Position.X), int32(opts.Position.Y), int32(opts.Position.X) + 1, int32(opts.Position.Y) + 1}))
		off := w.frameOffset(w.style, w.exStyle)
		x, y = uintptr(int32(opts.Position.X+off.X)), uintptr(int32(opts.Position.Y+off.Y))
	case w.parent != nil:
		w.dpi = w.parent.dpi
		outer := w.outerSize(clampPhysical(platform.PhysicalSize(opts.Size, w.scale())))
		mi := monitorInfoOf(monitorFromWindow(w.parent.hwnd))
		p := centerOver(getWindowRect(w.parent.hwnd).image(), image.Pt(int(outer.width()), int(outer.height())), mi.work.image())
		x, y = uintptr(int32(p.X)), uintptr(int32(p.Y))
	default:
		w.dpi = primaryDPI()
	}
	return x, y
}

// initControl applies the options that need the window to exist, right
// after CreateWindowExW: the frameless frame, the hidden flag and
// fullscreen. The window is not shown yet.
func (w *Window) initControl() {
	w.onTop = w.opts.AlwaysOnTop
	if w.opts.Frameless {
		w.extendFrame()
		// The window got its first WM_NCCALCSIZE during CreateWindowExW;
		// this makes DWM and the frame agree with it.
		setWindowPos(w.hwnd, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
	}
	w.syncHidden()
	if isZoomed(w.hwnd) {
		w.pos = w.normalContentOrigin()
	} else {
		w.pos = clientOrigin(w.hwnd)
	}
	if w.opts.Fullscreen {
		w.enterFullscreen()
	}
}

// extendFrame keeps the DWM shadow (and the rounded corners of Windows 11)
// of a frameless window, which has no non-client area for DWM to draw
// them around: a one-pixel top margin of the frame extended into the
// client area, which the app's opaque pixels cover.
func (w *Window) extendFrame() {
	if !available(procDwmExtendFrameIntoClientArea) {
		return
	}
	m := margins{top: 1}
	procDwmExtendFrameIntoClientArea.Call(w.hwnd, uintptr(unsafe.Pointer(&m)))
}

// --- reported state ---

// updateState reports the window's state through the loop (coalesced; the
// loop drops it if nothing changed). It runs at every WM_SIZE and WM_MOVE
// and after the requests that change the state without them. In the normal
// state (not minimized, maximized or fullscreen) it records the content
// origin, which is what Position reports in every state: where the
// window's content goes when it is normal again, so a saved position and
// the maximized flag restore the window like Windows' own placement.
func (w *Window) updateState() {
	if !w.ready || w.Closed() {
		return
	}
	iconic, zoomed := isIconic(w.hwnd), isZoomed(w.hwnd)
	if w.fs == nil && !iconic && !zoomed {
		w.pos = clientOrigin(w.hwnd)
	}
	fsMax := w.fs != nil && w.fs.maximized
	w.b.dirty = true
	w.b.loop.UpdateState(w, reportedState(iconic, zoomed, w.fs != nil, fsMax, w.opts.Frameless, w.pos))
}

// syncHidden pauses the window's frames while it is hidden by the app or by
// Windows with its minimized owner (a minimized window has an empty size,
// which pauses them too).
func (w *Window) syncHidden() { w.SetHidden(!w.visible || w.ownerHidden) }

// onShowWindow handles WM_SHOWWINDOW: Windows hides owned windows when
// their owner is minimized and shows them again when it is restored.
func (w *Window) onShowWindow(lparam uintptr) {
	switch lparam {
	case swParentClosing:
		w.ownerHidden = true
	case swParentOpening:
		w.ownerHidden = false
	default:
		return
	}
	w.syncHidden()
}

// --- showing and modality ---

// show shows the window, applying the state requested while it was not
// shown. The first show activates the window, like a new window on every
// platform; later shows (SetVisible(true)) do not, except for a modal
// window, which takes the input from its parent as a dialog box does.
func (w *Window) show() {
	first := !w.everShown
	// Disabled before the show, as DialogBox disables the owner before
	// showing the dialog.
	w.setBlocking(true)
	cmd := uintptr(swShowNA)
	if first || w.opts.Modal {
		cmd = swShow
	}
	switch w.pending {
	case showMinimized:
		cmd = swShowMinNoActive
	case showMaximized:
		cmd = swShowMaximized
	case showRestored:
		cmd = swRestore
	}
	w.pending = showAsIs
	w.shown, w.everShown = true, true
	procShowWindow.Call(w.hwnd, cmd)
}

// setBlocking makes a modal child disable (on) or re-enable its parent.
// The parent counts its visible modal children and is enabled again when
// the last one goes. It is enabled before the child is hidden or
// destroyed: otherwise Windows, finding no enabled window of the app to
// activate, activates another application (what EndDialog does too).
func (w *Window) setBlocking(on bool) {
	p := w.parent
	if !w.opts.Modal || p == nil || on == w.blocking {
		return
	}
	w.blocking = on
	if p.Closed() {
		return
	}
	if on {
		p.blockers++
		if p.blockers == 1 {
			procEnableWindow.Call(p.hwnd, 0)
		}
		return
	}
	p.blockers--
	if p.blockers == 0 {
		procEnableWindow.Call(p.hwnd, 1)
	}
}

// --- platform.Window: window control ---

// Minimize minimizes the window (ShowWindow SW_MINIMIZE). A window that is
// not shown is minimized when it is shown.
func (w *Window) Minimize() {
	if w.Closed() {
		return
	}
	if !w.shown {
		w.pending = showMinimized
		return
	}
	if !isIconic(w.hwnd) {
		procShowWindow.Call(w.hwnd, swMinimize)
	}
}

// SetMaximized maximizes the window or restores it (ShowWindow SW_MAXIMIZE
// and SW_RESTORE); either un-minimizes it. In fullscreen it changes the
// state the window returns to. A window that is not shown changes when it
// is shown.
func (w *Window) SetMaximized(maximized bool) {
	if w.Closed() {
		return
	}
	if w.fs != nil {
		if w.shown && isIconic(w.hwnd) {
			procShowWindow.Call(w.hwnd, swRestore)
		}
		if w.fs.maximized != maximized {
			w.fs.maximized = maximized
			w.updateState()
		}
		return
	}
	if !w.shown {
		switch {
		case maximized:
			w.pending = showMaximized
		case isZoomed(w.hwnd) || w.pending != showAsIs:
			w.pending = showRestored
		}
		return
	}
	iconic, zoomed := isIconic(w.hwnd), isZoomed(w.hwnd)
	switch {
	case maximized && (iconic || !zoomed):
		procShowWindow.Call(w.hwnd, swShowMaximized)
	case !maximized && iconic:
		procShowWindow.Call(w.hwnd, swRestore)
		if isZoomed(w.hwnd) { // it was maximized before it was minimized
			procShowWindow.Call(w.hwnd, swRestore)
		}
	case !maximized && zoomed:
		procShowWindow.Call(w.hwnd, swRestore)
	}
}

// SetFullscreen makes the window cover its monitor without a frame, or
// returns it to its saved placement (and maximized state).
func (w *Window) SetFullscreen(fullscreen bool) {
	if w.Closed() {
		return
	}
	if fullscreen == (w.fs != nil) {
		if w.shown && isIconic(w.hwnd) {
			procShowWindow.Call(w.hwnd, swRestore)
		}
		return
	}
	if fullscreen {
		w.enterFullscreen()
	} else {
		w.leaveFullscreen()
	}
	w.updateState()
}

// enterFullscreen saves the styles and placement and resizes the window to
// its monitor without caption and borders. A maximized window loses
// WS_MAXIMIZE through the style (no restore animation); it is maximized
// again when it leaves.
func (w *Window) enterFullscreen() {
	if w.shown && isIconic(w.hwnd) {
		procShowWindow.Call(w.hwnd, swRestore)
	}
	fs := &fullscreenSave{style: w.style, exStyle: w.exStyle}
	fs.maximized = isZoomed(w.hwnd) || (!w.shown && w.pending == showMaximized)
	if !w.shown {
		w.pending = showAsIs
	}
	fs.placement.length = uint32(unsafe.Sizeof(fs.placement))
	procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&fs.placement)))
	mi := monitorInfoOf(monitorFromWindow(w.hwnd))
	fs.offset = workspaceOffset(mi)
	w.fs = fs

	w.style &^= wsOverlappedWindow | wsMaximize
	w.exStyle &^= wsExFrameEdges
	setWindowLong(w.hwnd, gwlStyle, getWindowLong(w.hwnd, gwlStyle)&^(wsOverlappedWindow|wsMaximize))
	setWindowLong(w.hwnd, gwlExStyle, getWindowLong(w.hwnd, gwlExStyle)&^wsExFrameEdges)
	m := mi.monitor
	flags := uintptr(swpNoOwnerZOrder | swpFrameChanged | swpNoActivate)
	if !w.shown {
		flags |= swpNoZOrder
	}
	procSetWindowPos.Call(w.hwnd, 0 /* HWND_TOP */, uintptr(m.left), uintptr(m.top), uintptr(m.width()), uintptr(m.height()), flags)
}

// leaveFullscreen restores the saved styles and placement.
func (w *Window) leaveFullscreen() {
	fs := w.fs
	w.fs = nil
	w.style, w.exStyle = fs.style&^wsMaximize, fs.exStyle
	// Only the bits fullscreen removed come back: WS_VISIBLE, WS_DISABLED
	// and WS_EX_TOPMOST may have changed since.
	setWindowLong(w.hwnd, gwlStyle, getWindowLong(w.hwnd, gwlStyle)|fs.style&wsOverlappedWindow)
	setWindowLong(w.hwnd, gwlExStyle, getWindowLong(w.hwnd, gwlExStyle)|fs.exStyle&wsExFrameEdges)
	wp := fs.placement
	switch {
	case !w.shown:
		wp.showCmd = swHide
		if fs.maximized {
			w.pending = showMaximized
		}
	case fs.maximized:
		wp.showCmd = swShowMaximized
	default:
		wp.showCmd = swShowNoActivate
	}
	procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&wp)))
	setWindowPos(w.hwnd, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoOwnerZOrder|swpNoActivate|swpFrameChanged)
}

// SetVisible shows or hides the window (SW_SHOW / SW_HIDE). A window that
// was never presented is shown by its next Present. Hiding a modal window
// re-enables its parent first.
func (w *Window) SetVisible(visible bool) {
	if w.Closed() || visible == w.visible {
		return
	}
	w.visible = visible
	w.syncHidden()
	if visible {
		if w.dib.pix != nil {
			w.show() // the retained frame answers WM_PAINT
		}
		return
	}
	if w.shown {
		w.setBlocking(false)
		w.shown = false
		procShowWindow.Call(w.hwnd, swHide)
	}
}

// SetAlwaysOnTop moves the window to the topmost band (HWND_TOPMOST) or
// back (HWND_NOTOPMOST).
func (w *Window) SetAlwaysOnTop(onTop bool) {
	if w.Closed() || onTop == w.onTop {
		return
	}
	w.onTop = onTop
	after := hwndNoTopmost
	if onTop {
		after = hwndTopmost
	}
	procSetWindowPos.Call(w.hwnd, after, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
}

// SetMinSize changes the minimum content size (WM_GETMINMAXINFO); a normal
// window below it is resized.
func (w *Window) SetMinSize(size platform.Size) {
	platform.CheckSize("SetMinSize size", size)
	if !w.Closed() {
		w.opts.MinSize = size
		w.applyLimits()
	}
}

// SetMaxSize changes the maximum content size (WM_GETMINMAXINFO); a normal
// window above it is resized.
func (w *Window) SetMaxSize(size platform.Size) {
	platform.CheckSize("SetMaxSize size", size)
	if !w.Closed() {
		w.opts.MaxSize = size
		w.applyLimits()
	}
}

func (w *Window) applyLimits() {
	w.opts.Size = w.opts.ClampSize(w.opts.Size)
	if w.fs != nil || isIconic(w.hwnd) || isZoomed(w.hwnd) {
		return
	}
	scale := w.scale()
	want := clampPhysical(platform.PhysicalSize(w.opts.ClampSize(platform.LogicalSize(w.client, scale)), scale))
	if want != w.client {
		w.resizeClient(want)
	}
}

// SetPosition moves the window so that its content's top-left is at pos
// (physical desktop pixels). A minimized, maximized or fullscreen window
// gets pos as the position it returns to.
func (w *Window) SetPosition(pos image.Point) {
	if w.Closed() {
		return
	}
	if w.fs == nil && !isIconic(w.hwnd) && !isZoomed(w.hwnd) {
		// The frame's size depends on the DPI of the monitor the window
		// lands on, which moving there may change (WM_DPICHANGED): one
		// more move puts the content where it was asked.
		for range 2 {
			off := w.frameOffset(w.style, w.exStyle)
			setWindowPos(w.hwnd, int32(pos.X+off.X), int32(pos.Y+off.Y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
			if clientOrigin(w.hwnd) == pos {
				break
			}
		}
		return
	}
	style, exStyle := w.style, w.exStyle
	if w.fs != nil {
		style, exStyle = w.fs.style, w.fs.exStyle
	}
	off := w.frameOffset(style, exStyle)
	w.setNormalRect(func(r image.Rectangle) image.Rectangle {
		return r.Add(pos.Add(off).Sub(r.Min))
	})
	w.pos = pos
	w.updateState()
}

// Activate restores a minimized window and brings it to the foreground.
// Windows lets a process take the foreground only in some conditions (it
// is the foreground process, it received the last input event, it was
// given the right with AllowSetForegroundWindow, no menu is open...);
// when SetForegroundWindow is refused, the task bar button flashes until
// the user activates the window (FlashWindowEx), which is what Windows
// does for windows that ask for attention. A hidden window stays hidden.
func (w *Window) Activate() {
	if w.Closed() {
		return
	}
	if !w.shown {
		if w.pending == showMinimized {
			w.pending = showAsIs
		}
		return
	}
	if isIconic(w.hwnd) {
		procShowWindow.Call(w.hwnd, swRestore)
	}
	if r, _, _ := procSetForegroundWindow.Call(w.hwnd); r == 0 {
		fi := flashWInfo{size: uint32(unsafe.Sizeof(flashWInfo{})), hwnd: w.hwnd, flags: flashwTray | flashwTimerNoFG}
		procFlashWindowEx.Call(uintptr(unsafe.Pointer(&fi)))
	}
}

// SetIcon sets the title bar and task bar icons (WM_SETICON ICON_SMALL and
// ICON_BIG) at the sizes of the window's DPI, picked with platform.PickIcon
// and scaled to the exact size. It is rebuilt when the DPI changes.
func (w *Window) SetIcon(images []image.Image) {
	platform.CheckIcon("SetIcon", images)
	if w.Closed() {
		return
	}
	w.icon = platform.CopyIcon(images)
	w.applyIcon()
}

// applyIcon creates the icons for the current DPI, installs them and
// destroys the previous ones.
func (w *Window) applyIcon() {
	old := w.icons
	w.icons = [2]uintptr{}
	if len(w.icon) > 0 {
		for i, metric := range [2]int{smCxSmIcon, smCxIcon} {
			side := systemMetric(metric, w.dpi)
			if side <= 0 {
				side = int(16 * (i + 1) * int(w.dpi) / baseDPI)
			}
			w.icons[i] = createIcon(iconBGRA(platform.PickIcon(w.icon, side), side), side)
		}
	}
	procSendMessageW.Call(w.hwnd, wmSetIcon, iconSmall, w.icons[0])
	procSendMessageW.Call(w.hwnd, wmSetIcon, iconBig, w.icons[1])
	destroyIcons(old)
}

func destroyIcons(icons [2]uintptr) {
	for _, h := range icons {
		if h != 0 {
			procDestroyIcon.Call(h)
		}
	}
}

// createIcon makes an icon from top-down, non-premultiplied BGRA pixels: a
// 32-bit DIB section as the color bitmap (its alpha channel is the
// transparency) and an empty monochrome mask (CreateIconIndirect, as GLFW
// does). It returns 0 on failure.
func createIcon(bgra []byte, side int) uintptr {
	hdr := bitmapInfo{bitmapInfoHeader: bitmapInfoHeader{
		size: bitmapInfoHeaderSize, width: int32(side), height: -int32(side), planes: 1, bitCount: 32, compression: biRGB,
	}}
	var bits uintptr
	color, _, _ := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&hdr)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == 0 {
		return 0
	}
	defer procDeleteObject.Call(color)
	copy(unsafe.Slice((*byte)(ptrFromUintptr(bits)), len(bgra)), bgra)
	// Monochrome rows are padded to 16 bits.
	mask := make([]byte, (side+15)/16*2*side)
	hmask, _, _ := procCreateBitmap.Call(uintptr(side), uintptr(side), 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	if hmask == 0 {
		return 0
	}
	defer procDeleteObject.Call(hmask)
	ii := iconInfo{icon: 1, mask: hmask, color: color}
	h, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	return h
}

// SetHitRegions records the custom title bar's regions; WM_NCHITTEST
// answers from them.
func (w *Window) SetHitRegions(regions []platform.HitRegion) {
	platform.CheckHitRegions(regions)
	if w.Closed() || platform.EqualHitRegions(w.regions, regions) {
		return
	}
	w.regions = slices.Clone(regions)
}

// RestoreToken returns "": Windows restores the position from the saved
// geometry, no token is needed.
func (w *Window) RestoreToken() string { return "" }

// --- frameless windows and hit-testing ---

// onNCCalcSize handles WM_NCCALCSIZE for a frameless window: the whole
// window is the client area (WS_CAPTION and WS_THICKFRAME stay, so the
// window snaps, animates and has a system menu), except that a maximized
// window is clipped to the work area of its monitor (see maximizedClient).
// lParam points to the rectangle either way: NCCALCSIZE_PARAMS starts with
// the proposed window rectangle.
func (w *Window) onNCCalcSize(lparam uintptr) {
	r := (*rect)(ptrFromUintptr(lparam))
	if r == nil || w.fs != nil || !isZoomed(w.hwnd) {
		return
	}
	mi := monitorInfoOf(monitorFromRect(*r))
	*r = rectFrom(maximizedClient(r.image(), mi.work.image()))
}

// resizeBorder returns the width of the resize band of a frameless window in
// physical pixels (the DPI's resize frame: SM_CXSIZEFRAME plus
// SM_CXPADDEDBORDER), or 0 when it has none.
func (w *Window) resizeBorder() int {
	if !w.opts.Frameless || w.opts.FixedSize || w.fs != nil || isZoomed(w.hwnd) {
		return 0
	}
	return max(systemMetric(smCxSizeFrame, w.dpi)+systemMetric(smCxPaddedBorder, w.dpi), 1)
}

// onNCHitTest answers WM_NCHITTEST. DefWindowProc answers the OS frame of
// decorated windows; inside the client area, the resize band of a
// frameless window and the app's hit regions decide (clientHit).
func (w *Window) onNCHitTest(lparam uintptr) uintptr {
	def := defWindowProc(w.hwnd, wmNCHitTest, 0, lparam)
	if def != htClient || (len(w.regions) == 0 && !w.opts.Frameless) {
		return def
	}
	x, y := w.screenPointToClient(lparam)
	return clientHit(hitInput{
		pt:         image.Pt(x, y),
		client:     w.client,
		scale:      w.scale(),
		border:     w.resizeBorder(),
		fullscreen: w.fs != nil,
		regions:    w.regions,
	})
}

// screenPointToClient converts the screen position of a non-client mouse
// message or WM_NCHITTEST to client coordinates.
func (w *Window) screenPointToClient(lparam uintptr) (x, y int) {
	sx, sy := pointFromLParam(lparam)
	return screenToClient(w.hwnd, sx, sy)
}

// inClient reports whether a point in client coordinates lies in the
// client area.
func (w *Window) inClient(x, y int) bool {
	return x >= 0 && y >= 0 && x < w.client.X && y < w.client.Y
}

// onNCMouseMove handles WM_NCMOUSEMOVE. Over the app's own title bar (a
// caption or button region, which WM_NCHITTEST answers as non-client) the
// move is a pointer move like any other, and the window tracks the leave
// of the non-client area; over the OS's frame (a resize band, the title
// bar of a decorated window) the pointer has left the app's content.
func (w *Window) onNCMouseMove(wparam, lparam uintptr) {
	x, y := w.screenPointToClient(lparam)
	w.mods = w.modifiers()
	if !isAppHit(wparam) || !w.inClient(x, y) {
		if w.ptr.inside {
			w.ptr.leaveMessage(w.emitFn)
		}
		return
	}
	w.ptr.move(logicalPoint(x, y, w.scale()), true, w.emitFn)
	w.track(true)
}

// onNCButton handles a non-client button message. Over the app's window
// buttons (HTMINBUTTON, HTMAXBUTTON, HTCLOSE answered from hit regions) it
// becomes the client press or release the app's button expects, and it
// reports true: DefWindowProc must not see it, or it would run its own
// button tracking and draw the classic buttons. Caption presses and the
// OS's frame are left to DefWindowProc (move, double-click maximize, window
// menu, resize).
func (w *Window) onNCButton(msg uint32, wparam, lparam uintptr) bool {
	btn, down, hit, ok := ncButtonFromMessage(msg, wparam)
	if !ok || !isAppButton(hit) {
		return false
	}
	x, y := w.screenPointToClient(lparam)
	if !w.inClient(x, y) {
		return false
	}
	w.button(btn, down, x, y)
	return true
}

// pointerOverApp reports whether the mouse is over the window's content:
// its client area or the app's title bar regions, and not under another
// window. A leave message (WM_MOUSELEAVE, WM_NCMOUSELEAVE) of a window with
// hit regions is only a leave if this is false: the pointer moving between
// the client area and a caption region crosses Windows' client/non-client
// boundary, which the app must not see.
func (w *Window) pointerOverApp() bool {
	var p point
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return false
	}
	if windowFromPoint(p) != w.hwnd {
		return false
	}
	lp := uintptr(uint16(int16(p.x))) | uintptr(uint16(int16(p.y)))<<16
	if !isAppHit(w.onNCHitTest(lp)) {
		return false
	}
	return w.inClient(screenToClient(w.hwnd, int(p.x), int(p.y)))
}

// cursorOverAppRegion reports whether the mouse is over the client area, for
// WM_SETCURSOR answers that came from the app's hit regions.
func (w *Window) cursorOverAppRegion() bool {
	var p point
	if r, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&p))); r == 0 {
		return false
	}
	return w.inClient(screenToClient(w.hwnd, int(p.x), int(p.y)))
}

// --- placement helpers ---

// frameOffset returns the offset from the content's top-left to the
// window's outer top-left for a window with the given styles at the
// current DPI (negative or zero components); zero for a frameless window.
func (w *Window) frameOffset(style, exStyle uint32) image.Point {
	if w.opts.Frameless {
		return image.Point{}
	}
	r := adjustWindowRect(rect{}, style, exStyle, w.dpi)
	return image.Pt(int(r.left), int(r.top))
}

// normalContentOrigin returns the content origin of the window's normal
// placement (rcNormalPosition), for a window that is not normal now.
func (w *Window) normalContentOrigin() image.Point {
	wp := windowPlacement{length: uint32(unsafe.Sizeof(windowPlacement{}))}
	procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&wp)))
	r := wp.rcNormalPosition.image().Add(workspaceOffset(monitorInfoOf(monitorFromWindow(w.hwnd))))
	return r.Min.Sub(w.frameOffset(w.style, w.exStyle))
}

// setNormalRect changes the outer rectangle (screen coordinates) the window
// returns to when it is normal again: the saved placement of a fullscreen
// window, otherwise rcNormalPosition through SetWindowPlacement with the
// window's current show state.
func (w *Window) setNormalRect(change func(image.Rectangle) image.Rectangle) {
	if fs := w.fs; fs != nil {
		r := fs.placement.rcNormalPosition.image().Add(fs.offset)
		fs.placement.rcNormalPosition = rectFrom(change(r).Sub(fs.offset))
		return
	}
	wp := windowPlacement{length: uint32(unsafe.Sizeof(windowPlacement{}))}
	procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&wp)))
	off := workspaceOffset(monitorInfoOf(monitorFromWindow(w.hwnd)))
	wp.rcNormalPosition = rectFrom(change(wp.rcNormalPosition.image().Add(off)).Sub(off))
	switch {
	case !isWindowVisible(w.hwnd):
		wp.showCmd = swHide
	case isIconic(w.hwnd):
		wp.showCmd = swShowMinNoActive
	case isZoomed(w.hwnd):
		wp.showCmd = swShowMaximized
	default:
		wp.showCmd = swShowNoActivate
	}
	procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&wp)))
}

// workspaceOffset returns what turns the workspace coordinates of
// WINDOWPLACEMENT into screen coordinates on a monitor: the offset of its
// work area from its origin (non-zero when the task bar is at the top or
// the left; Chromium converts the same way).
func workspaceOffset(mi monitorInfo) image.Point {
	return mi.work.image().Min.Sub(mi.monitor.image().Min)
}

// --- thin wrappers ---

func (r rect) image() image.Rectangle {
	return image.Rect(int(r.left), int(r.top), int(r.right), int(r.bottom))
}

func rectFrom(r image.Rectangle) rect {
	return rect{int32(r.Min.X), int32(r.Min.Y), int32(r.Max.X), int32(r.Max.Y)}
}

func clientOrigin(hwnd uintptr) image.Point {
	p := clientToScreen(hwnd, image.Point{})
	return image.Pt(int(p.x), int(p.y))
}

func getWindowRect(hwnd uintptr) (r rect) {
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return r
}

func getWindowLong(hwnd, index uintptr) uint32 {
	r, _, _ := procGetWindowLongW.Call(hwnd, index)
	return uint32(r)
}

func setWindowLong(hwnd, index uintptr, v uint32) {
	procSetWindowLongW.Call(hwnd, index, uintptr(v))
}

func isWindowVisible(hwnd uintptr) bool {
	r, _, _ := procIsWindowVisible.Call(hwnd)
	return r != 0
}

func monitorFromWindow(hwnd uintptr) uintptr {
	m, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultNr)
	return m
}

func monitorFromRect(r rect) uintptr {
	m, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), monitorDefaultNr)
	return m
}

func monitorInfoOf(mon uintptr) monitorInfo {
	mi := monitorInfo{size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if mon == 0 {
		return monitorInfo{}
	}
	if r, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return monitorInfo{}
	}
	return mi
}

// windowFromPoint calls WindowFromPoint, which takes a POINT by value: one
// register on 64-bit Windows (x in the low half), two stack words on
// 32-bit.
func windowFromPoint(p point) uintptr {
	var r uintptr
	if unsafe.Sizeof(uintptr(0)) == 8 {
		v := uint64(uint32(p.x)) | uint64(uint32(p.y))<<32
		r, _, _ = procWindowFromPoint.Call(uintptr(v))
	} else {
		r, _, _ = procWindowFromPoint.Call(uintptr(p.x), uintptr(p.y))
	}
	return r
}

// systemMetric returns a system metric at a DPI (GetSystemMetricsForDpi,
// Windows 10 1607), or at the system DPI on older systems.
func systemMetric(index int, dpi uint32) int {
	if available(procGetSystemMetricsForDpi) {
		r, _, _ := procGetSystemMetricsForDpi.Call(uintptr(index), uintptr(dpi))
		return int(int32(r))
	}
	r, _, _ := procGetSystemMetrics.Call(uintptr(index))
	return int(int32(r))
}
