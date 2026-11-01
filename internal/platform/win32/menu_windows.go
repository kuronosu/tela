//go:build windows

package win32

import (
	"errors"
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Window menus ("Native menus" in package platform), built like the tray's
// (buildMenu, buildPopupMenu):
//
//   - The menu bar is a CreateMenu menu set with SetMenu, on framed windows
//     only (a frameless window's client area covers the frame, where the
//     bar would be). Setting or removing it grows or shrinks the window by
//     the change of its client area, measured, which covers a bar that
//     wraps to several lines; a maximized or minimized window changes the
//     size it returns to instead. The outer size computations
//     (adjustRect) add one line of menu bar from then on. A fullscreen
//     window shows no bar: it is detached while fullscreen and attached
//     again when the window leaves it. WM_COMMAND from the bar (16-bit
//     command IDs, mapped back by the window's table) is a MenuEvent.
//     Mnemonics are not supported: '&' is escaped, and Alt and F10 do not
//     open the bar (the keyboard translation swallows them, keyboard.go),
//     so the bar is for the mouse; apps keep their own shortcuts.
//   - Popup menus run TrackPopupMenuEx with TPM_RETURNCMD outside the
//     delivery pass (a posted message, like the dialogs), one at a time,
//     after SetForegroundWindow and followed by a posted WM_NULL (the
//     documented quirk, KB135788). The answer goes through the loop.
//   - Win32 menus are light even in dark mode (the dark menus of Explorer
//     use undocumented uxtheme exports, which Tela does not call).

// Messages and flags of the menus (winuser.h).
const (
	wmCommand = 0x0111
	wmNull    = 0x0000

	tpmLeftAlign   = 0x0000
	tpmTopAlign    = 0x0000
	tpmRightButton = 0x0002
	tpmRightAlign  = 0x0008
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	smMenuDropAlignment = 40
)

// popupRequest is a PopupMenu waiting to be shown.
type popupRequest struct {
	w    *Window
	at   platform.Point
	menu []platform.MenuItem
	done func(platform.MenuItemID, error)
}

// adjustWindowRectMenu is adjustWindowRect with the bMenu argument: an
// outer size that also holds one line of menu bar.
func adjustWindowRectMenu(client rect, style, exStyle, dpi uint32, menu bool) rect {
	var m uintptr
	if menu {
		m = 1
	}
	r := client
	if available(procAdjustWindowRectExForDpi) {
		procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), m, uintptr(exStyle), uintptr(dpi))
	} else {
		procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&r)), uintptr(style), m, uintptr(exStyle))
	}
	return r
}

// adjustRect returns the outer rectangle of a client rectangle for the
// window in its normal state with the given styles: with its menu bar,
// when it has one.
func (w *Window) adjustRect(client rect, style, exStyle uint32) rect {
	return adjustWindowRectMenu(client, style, exStyle, w.dpi, w.integ.menu != 0)
}

// SetMenu sets the window's menu bar; nil or empty removes it. Frameless
// windows return ErrUnsupported (the layers above draw their own).
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	if w.opts.Frameless {
		return platform.ErrUnsupported
	}
	if w.Closed() {
		return nil
	}
	var h uintptr
	var cmds []platform.MenuItemID
	if len(menu) > 0 {
		var entries []menuEntry
		entries, cmds = buildMenu(menu)
		if h = buildMenuBar(entries); h == 0 {
			return errors.New("win32: SetMenu: creating the menu failed")
		}
	}
	in := &w.integ
	old := in.menu
	in.menu, in.menuCmds = h, cmds
	if w.fs != nil {
		w.growNormal(old != 0, h != 0, w.fs.style, w.fs.exStyle)
	} else {
		w.attachMenu(old != 0, h)
	}
	if old != 0 {
		procDestroyMenu.Call(old) // detached above; destroys its submenus
	}
	return nil
}

// buildMenuBar creates a menu bar with the entries (0 on failure).
func buildMenuBar(entries []menuEntry) uintptr {
	bar, _, _ := procCreateMenu.Call()
	if bar == 0 {
		return 0
	}
	if !fillMenu(bar, entries) {
		procDestroyMenu.Call(bar)
		return 0
	}
	return bar
}

// attachMenu sets the menu bar h (0: none) on the window, which had one
// or not, keeping the client size.
func (w *Window) attachMenu(had bool, h uintptr) {
	normal := !isIconic(w.hwnd) && !isZoomed(w.hwnd)
	before := getClientRect(w.hwnd)
	procSetMenu.Call(w.hwnd, h)
	if !normal {
		w.growNormal(had, h != 0, w.style, w.exStyle)
		return
	}
	after := getClientRect(w.hwnd)
	if dy := before.height() - after.height(); dy != 0 {
		r := getWindowRect(w.hwnd)
		setWindowPos(w.hwnd, 0, 0, r.width(), r.height()+dy, swpNoMove|swpNoZOrder|swpNoActivate)
	}
}

// growNormal changes the height of the rectangle the window returns to by
// one line of menu bar, when the bar comes or goes.
func (w *Window) growNormal(had, has bool, style, exStyle uint32) {
	if had == has {
		return
	}
	without := adjustWindowRectMenu(rect{}, style, exStyle, w.dpi, false)
	with := adjustWindowRectMenu(rect{}, style, exStyle, w.dpi, true)
	dy := int(with.height() - without.height())
	if !has {
		dy = -dy
	}
	w.setNormalRect(func(r image.Rectangle) image.Rectangle {
		r.Max.Y += dy
		return r
	})
}

// menuForFullscreen detaches the menu bar when the window enters
// fullscreen and attaches it again when it leaves (the saved placement
// already has room for it).
func (w *Window) menuForFullscreen(fullscreen bool) {
	if m := w.integ.menu; m != 0 {
		if fullscreen {
			m = 0
		}
		procSetMenu.Call(w.hwnd, m)
	}
}

// onMenuCommand handles WM_COMMAND: a choice in the menu bar is a
// MenuEvent. It reports whether the message was one.
func (w *Window) onMenuCommand(wparam, lparam uintptr) bool {
	id, ok := menuCommand(w.integ.menuCmds, wparam, lparam)
	if ok {
		w.b.enqueue(w, platform.MenuEvent{Item: id})
	}
	return ok
}

// PopupMenu shows a popup menu at a point of the window, outside the
// delivery pass; done gets the chosen item, or 0 when the menu was
// dismissed (or the window closed first).
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	b := w.b
	if b.closed {
		return errClosed
	}
	if w.Closed() {
		return errors.New("win32: PopupMenu on a closed window")
	}
	in := &b.integ
	in.popups = append(in.popups, popupRequest{w: w, at: at, menu: cloneMenu(menu), done: done})
	if !in.popupPosted {
		in.popupPosted = true
		b.postLater(wmTelaPopup)
	}
	return nil
}

// runPopup shows the next popup menu, unless one is open (the next starts
// when it closes).
func (b *Backend) runPopup() {
	in := &b.integ
	in.popupPosted = false
	switch {
	case in.popupOpen || len(in.popups) == 0:
		return
	case b.pumping:
		in.popupPosted = true
		b.postLater(wmTelaPopup)
		return
	}
	req := in.popups[0]
	in.popups = in.popups[1:]
	var item platform.MenuItemID
	var err error
	if !req.w.Closed() {
		in.popupOpen = true
		item, err = req.w.trackPopup(req.at, req.menu)
		in.popupOpen = false
	}
	b.answer(func() { req.done(item, err) })
	if len(in.popups) > 0 && !in.popupPosted {
		in.popupPosted = true
		b.postLater(wmTelaPopup)
	}
	b.pump()
}

// trackPopup shows a popup menu with its top-left corner at a point of the
// window (logical pixels) and returns the chosen item, or 0.
func (w *Window) trackPopup(at platform.Point, items []platform.MenuItem) (platform.MenuItemID, error) {
	entries, cmds := buildMenu(items)
	menu := buildPopupMenu(entries)
	if menu == 0 {
		return 0, errors.New("win32: PopupMenu: creating the menu failed")
	}
	defer procDestroyMenu.Call(menu) // and its submenus
	x, y := physicalPoint(at, w.scale())
	p := point{x, y}
	procClientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&p)))
	flags := uintptr(tpmLeftAlign | tpmTopAlign | tpmRightButton | tpmNoNotify | tpmReturnCmd)
	if available(procGetSystemMetricsForDpi) {
		// Right-to-left systems open menus to the left of the point.
		if r, _, _ := procGetSystemMetricsForDpi.Call(smMenuDropAlignment, uintptr(w.dpi)); r != 0 {
			flags |= tpmRightAlign
		}
	}
	procSetForegroundWindow.Call(w.hwnd)
	cmd, _, _ := procTrackPopupMenuEx.Call(menu, flags, uintptr(p.x), uintptr(p.y), w.hwnd, 0)
	postMessage(w.hwnd, wmNull, 0, 0)
	return popupCommand(cmds, cmd), nil
}
