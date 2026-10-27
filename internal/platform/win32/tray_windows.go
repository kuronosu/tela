//go:build windows

package win32

import (
	"fmt"
	"image"
	"os"
	"slices"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Tray icons and balloon notifications through Shell_NotifyIconW
// (services.go has the overview).
//
//   - Every icon is added with NOTIFYICON_VERSION_4 and a 16-bit ID of its
//     own (no GUID: a GUID ties the icon to the executable's path), and its
//     callbacks come to the tray window as wmTelaTray. A primary click
//     (NIN_SELECT) and Enter or Space on a focused icon (NIN_KEYSELECT) are
//     TrayActivated; a secondary click, Shift+F10 or the menu key
//     (WM_CONTEXTMENU) open the menu.
//   - The menu is a popup menu built when it opens, shown with
//     TrackPopupMenuEx after SetForegroundWindow on the tray window, and
//     followed by a posted WM_NULL: without the first the menu does not
//     close when the user clicks elsewhere, without the second it may not
//     open the next time (the documented quirk of TrackPopupMenu, KB135788).
//     Its modal loop dispatches the backend's messages, so delivery goes
//     on. A menu asked for during a delivery pass (the callback was sent
//     while the pass peeked at the queue) is shown after the pass.
//   - Notifications are balloons (NIF_INFO), which Windows 10 and 11 show
//     as toasts. Windows shows one balloon per icon and its callbacks name
//     only the icon, so each notification gets an icon of its own, added
//     to the notification area for as long as the notification lives: a
//     click on the balloon (NIN_BALLOONUSERCLICK) is NotificationActivated,
//     its timeout or dismissal (NIN_BALLOONTIMEOUT) or its removal
//     (NIN_BALLOONHIDE) NotificationClosed, and the icon is removed then or
//     by CloseNotification (which withdraws the toast). Balloons have no
//     buttons: Notification.Actions are dropped (WinRT toasts, which have
//     them, need an AppUserModelID and a Start menu shortcut).
//   - When Explorer restarts, the taskbar broadcasts TaskbarCreated: the
//     tray icons are added again (at the current DPI), and notifications,
//     whose balloons are gone, are closed.
//   - A NIM_ADD that fails (Explorer is not running yet, or timed out under
//     load, which adds the icon anyway) does not fail NewTray, as in
//     Chromium: the icon is added again with TaskbarCreated.

// Shell_NotifyIconW messages and flags (shellapi.h).
const (
	nimAdd        = 0x0
	nimModify     = 0x1
	nimDelete     = 0x2
	nimSetVersion = 0x4

	nifMessage         = 0x01
	nifIcon            = 0x02
	nifTip             = 0x04
	nifInfo            = 0x10
	nifShowTip         = 0x80
	notifyIconVersion4 = 4

	niifUser             = 0x04
	niifLargeIcon        = 0x20
	niifRespectQuietTime = 0x80
)

// guid is GUID.
type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

// notifyIconData is NOTIFYICONDATAW (Vista and later). shellapi.h packs it
// to one byte on 32-bit Windows, where every field is already 4-byte
// aligned, so Go's natural layout matches on both 32 and 64 bits (956 and
// 976 bytes; TestNotifyIconDataLayout checks).
type notifyIconData struct {
	size            uint32
	hwnd            uintptr
	id              uint32
	flags           uint32
	callbackMessage uint32
	icon            uintptr
	tip             [128]uint16
	state           uint32
	stateMask       uint32
	info            [256]uint16
	version         uint32 // union with uTimeout, ignored since Vista
	infoTitle       [64]uint16
	infoFlags       uint32
	guidItem        guid
	balloonIcon     uintptr
}

// newTray adds a tray icon.
func (b *Backend) newTray(opts platform.TrayOptions) (platform.TrayID, error) {
	if _, err := b.trayWindow(); err != nil {
		return 0, err
	}
	d := &b.desk
	ic := &shellIcon{tip: opts.Tooltip, images: platform.CopyIcon(opts.Icon), menu: cloneMenu(opts.Menu)}
	h, err := createShellIcon(ic.images, smallIconSide())
	if err != nil {
		return 0, err
	}
	ic.hicon, ic.owned = h, true
	if ic.uid = d.ids.take(); ic.uid == 0 {
		procDestroyIcon.Call(h)
		return 0, fmt.Errorf("win32: NewTray: too many notification area icons")
	}
	d.lastTray++
	ic.tray = d.lastTray
	d.icons[ic.uid] = ic
	b.addIcon(ic)
	return ic.tray, nil
}

// updateTray replaces a tray icon's image, tooltip and menu.
func (b *Backend) updateTray(ic *shellIcon, opts platform.TrayOptions) error {
	images := platform.CopyIcon(opts.Icon)
	h, err := createShellIcon(images, smallIconSide())
	if err != nil {
		return err
	}
	old := ic.hicon
	ic.images, ic.tip, ic.menu, ic.hicon = images, opts.Tooltip, cloneMenu(opts.Menu), h
	if ic.added {
		nid := b.iconData(ic, nifIcon|nifTip|nifShowTip)
		ic.added = shellNotifyIcon(nimModify, &nid)
	}
	if !ic.added {
		b.addIcon(ic)
	}
	procDestroyIcon.Call(old)
	return nil
}

// showNotification shows a balloon on an icon of its own.
func (b *Backend) showNotification(n platform.Notification) (platform.NotificationID, error) {
	if _, err := b.trayWindow(); err != nil {
		return 0, err
	}
	d := &b.desk
	ic := &shellIcon{tip: n.Title}
	if ic.tip == "" {
		ic.tip = n.Body
	}
	// The icon in the notification area: the notification's, else the
	// app's first tray icon, else the executable's.
	images := n.Icon
	if len(images) == 0 {
		if tray := b.findIcon(func(ic *shellIcon) bool { return ic.tray != 0 }); tray != nil {
			images = tray.images
		}
	}
	if len(images) > 0 {
		h, err := createShellIcon(images, smallIconSide())
		if err != nil {
			return 0, err
		}
		ic.hicon, ic.owned = h, true
	} else {
		ic.hicon = b.appIcon()
	}
	if len(n.Icon) > 0 {
		if h, err := createShellIcon(n.Icon, largeIconSide()); err == nil {
			ic.big = h
		}
	}
	if ic.uid = d.ids.take(); ic.uid == 0 {
		releaseIconHandles(ic)
		return 0, fmt.Errorf("win32: Notify: too many notification area icons")
	}
	// Known before the shell sees it: its callbacks may be sent while
	// Shell_NotifyIconW waits for Explorer.
	d.lastNote++
	ic.note = d.lastNote
	d.icons[ic.uid] = ic
	if !b.addIcon(ic) {
		b.deleteIcon(ic)
		return 0, errNoNotificationArea
	}
	nid := b.iconData(ic, nifInfo)
	title, body := balloonText(n.Title, n.Body)
	putUTF16(nid.infoTitle[:], title)
	putUTF16(nid.info[:], body)
	nid.infoFlags = niifUser | niifRespectQuietTime
	if ic.big != 0 {
		nid.balloonIcon = ic.big
		nid.infoFlags |= niifLargeIcon
	}
	if !shellNotifyIcon(nimModify, &nid) {
		b.deleteIcon(ic)
		return 0, errNoNotificationArea
	}
	return ic.note, nil
}

// onTrayCallback handles the callback message of an icon.
func (b *Backend) onTrayCallback(wparam, lparam uintptr) {
	event, uid, x, y := trayCallback(wparam, lparam)
	ic := b.desk.icons[uid]
	switch {
	case ic == nil:
	case ic.note != 0:
		if kind, ok := balloonEvent(event); ok {
			b.deleteIcon(ic)
			b.enqueue(nil, platform.NotificationEvent{ID: ic.note, Kind: kind})
			b.pump()
		}
	case event == ninSelect || event == ninKeySelect:
		b.enqueue(nil, platform.TrayEvent{Tray: ic.tray, Kind: platform.TrayActivated})
		b.pump()
	case event == trayContextMenu:
		b.trayMenu(ic, x, y)
	}
}

// trayMenu shows the menu of a tray icon at (x, y), in physical screen
// pixels, and reports the chosen item.
func (b *Backend) trayMenu(ic *shellIcon, x, y int) {
	if len(ic.menu) == 0 {
		return
	}
	d := &b.desk
	if b.pumping {
		d.later = append(d.later, postedMessage{d.hwnd, wmTelaTrayMenu, uintptr(ic.uid), uintptr(uint16(x)) | uintptr(uint16(y))<<16})
		return
	}
	entries, cmds := buildMenu(ic.menu)
	menu := buildPopupMenu(entries)
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu) // and its submenus
	const (
		tpmLeftAlign        = 0x0000
		tpmRightAlign       = 0x0008
		tpmRightButton      = 0x0002
		tpmBottomAlign      = 0x0020
		tpmNoNotify         = 0x0080
		tpmReturnCmd        = 0x0100
		smMenuDropAlignment = 40
		wmNull              = 0x0000
	)
	flags := uintptr(tpmRightButton | tpmBottomAlign | tpmNoNotify | tpmReturnCmd | tpmLeftAlign)
	if available(procGetSystemMetricsForDpi) {
		// Right-to-left systems open menus to the left of the point.
		if r, _, _ := procGetSystemMetricsForDpi.Call(smMenuDropAlignment, uintptr(primaryDPI())); r != 0 {
			flags |= tpmRightAlign
		}
	}
	procSetForegroundWindow.Call(d.hwnd)
	cmd, _, _ := procTrackPopupMenuEx.Call(menu, flags, uintptr(x), uintptr(y), d.hwnd, 0)
	postMessage(d.hwnd, wmNull, 0, 0)
	if cmd == 0 || int(cmd) > len(cmds) || d.icons[ic.uid] != ic {
		return // dismissed, or the tray was removed while its menu was open
	}
	b.enqueue(nil, platform.TrayEvent{Tray: ic.tray, Kind: platform.TrayMenuItem, Item: cmds[cmd-1]})
	b.pump()
}

// buildPopupMenu creates a popup menu with the entries (0 on failure).
// Destroying it destroys its submenus.
func buildPopupMenu(entries []menuEntry) uintptr {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0
	}
	for _, e := range entries {
		var r uintptr
		switch {
		case e.flags&mfSeparator != 0:
			r, _, _ = procAppendMenuW.Call(menu, mfSeparator, 0, 0)
		case e.sub != nil:
			sub := buildPopupMenu(e.sub)
			if sub == 0 {
				break
			}
			r, _, _ = procAppendMenuW.Call(menu, uintptr(e.flags|mfString), sub, uintptr(unsafe.Pointer(utf16(e.label))))
			if r == 0 {
				procDestroyMenu.Call(sub)
			}
		default:
			r, _, _ = procAppendMenuW.Call(menu, uintptr(e.flags|mfString), uintptr(e.cmd), uintptr(unsafe.Pointer(utf16(e.label))))
		}
		if r == 0 {
			procDestroyMenu.Call(menu)
			return 0
		}
	}
	return menu
}

// taskbarRestarted adds the tray icons again after Explorer restarted, and
// closes the notifications, whose balloons went away with it.
func (b *Backend) taskbarRestarted() {
	closed := 0
	for _, ic := range b.sortedIcons() {
		if ic.note != 0 {
			b.deleteIcon(ic)
			b.enqueue(nil, platform.NotificationEvent{ID: ic.note, Kind: platform.NotificationClosed})
			closed++
			continue
		}
		// The taskbar may have come back at another DPI.
		if h, err := createShellIcon(ic.images, smallIconSide()); err == nil {
			procDestroyIcon.Call(ic.hicon)
			ic.hicon = h
		}
		ic.added = false
		b.addIcon(ic)
	}
	if closed > 0 {
		b.pump()
	}
}

// trayWindow returns the hidden tray window, creating it on first use.
func (b *Backend) trayWindow() (uintptr, error) {
	d := &b.desk
	if d.hwnd != 0 {
		return d.hwnd, nil
	}
	const (
		wsPopup        = 0x80000000
		wsExToolWindow = 0x00000080 // never on the taskbar or in Alt+Tab
		msgfltAllow    = 1
	)
	hwnd, _, e := procCreateWindowExW.Call(wsExToolWindow, uintptr(unsafe.Pointer(classNamePtr)), 0, wsPopup,
		0, 0, 0, 0, 0, 0, b.hinst, 0)
	if hwnd == 0 {
		return 0, winError("creating the tray window", e)
	}
	registry.mu.Lock()
	registry.backends[hwnd] = b
	registry.mu.Unlock()
	d.hwnd = hwnd
	d.icons = map[uint16]*shellIcon{}
	if m, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16("TaskbarCreated")))); m != 0 {
		d.taskbarCreated = uint32(m)
		// An elevated process does not receive the broadcast of Explorer,
		// which runs at a lower integrity level, unless it lets it in.
		if available(procChangeWindowMessageFilterEx) {
			procChangeWindowMessageFilterEx.Call(hwnd, m, msgfltAllow, 0)
		}
	}
	return hwnd, nil
}

// iconData returns the NOTIFYICONDATAW of an icon with the given flags,
// filled with its callback message, icon and tooltip.
func (b *Backend) iconData(ic *shellIcon, flags uint32) notifyIconData {
	nid := notifyIconData{
		hwnd:            b.desk.hwnd,
		id:              uint32(ic.uid),
		flags:           flags,
		callbackMessage: wmTelaTray,
		icon:            ic.hicon,
	}
	nid.size = uint32(unsafe.Sizeof(nid))
	putUTF16(nid.tip[:], ic.tip)
	return nid
}

// addIcon adds an icon to the notification area and selects version 4 of
// its callbacks. It reports whether the notification area took it.
func (b *Backend) addIcon(ic *shellIcon) bool {
	nid := b.iconData(ic, nifMessage|nifIcon|nifTip|nifShowTip)
	ic.added = shellNotifyIcon(nimAdd, &nid)
	if ic.added {
		nid.version = notifyIconVersion4
		shellNotifyIcon(nimSetVersion, &nid)
	}
	return ic.added
}

// removeIcon removes an icon from the notification area.
func (b *Backend) removeIcon(ic *shellIcon) {
	nid := b.iconData(ic, 0)
	shellNotifyIcon(nimDelete, &nid)
	ic.added = false
}

// deleteIcon removes an icon and forgets it.
func (b *Backend) deleteIcon(ic *shellIcon) {
	d := &b.desk
	b.removeIcon(ic)
	delete(d.icons, ic.uid)
	d.ids.release(ic.uid)
	releaseIconHandles(ic)
}

// findIcon returns the icon with the smallest ID for which match is true.
func (b *Backend) findIcon(match func(*shellIcon) bool) *shellIcon {
	for _, ic := range b.sortedIcons() {
		if match(ic) {
			return ic
		}
	}
	return nil
}

// sortedIcons returns the icons in ID order.
func (b *Backend) sortedIcons() []*shellIcon {
	icons := make([]*shellIcon, 0, len(b.desk.icons))
	for _, ic := range b.desk.icons {
		icons = append(icons, ic)
	}
	slices.SortFunc(icons, func(a, b *shellIcon) int { return int(a.uid) - int(b.uid) })
	return icons
}

// releaseIconHandles destroys the icon handles an icon owns.
func releaseIconHandles(ic *shellIcon) {
	if ic.owned && ic.hicon != 0 {
		procDestroyIcon.Call(ic.hicon)
	}
	if ic.big != 0 {
		procDestroyIcon.Call(ic.big)
	}
	ic.hicon, ic.owned, ic.big = 0, false, 0
}

// appIcon returns the executable's small icon, or the system's
// application icon when it has none.
func (b *Backend) appIcon() uintptr {
	d := &b.desk
	if d.appIcon != 0 {
		return d.appIcon
	}
	if exe, err := os.Executable(); err == nil {
		var small uintptr
		n, _, _ := procExtractIconExW.Call(uintptr(unsafe.Pointer(utf16(exe))), 0, 0, uintptr(unsafe.Pointer(&small)), 1)
		if uint32(n) != 0 && uint32(n) != ^uint32(0) && small != 0 {
			d.appIcon, d.appIconOwned = small, true
			return small
		}
	}
	const idiApplication = 32512
	d.appIcon, _, _ = procLoadIconW.Call(0, idiApplication)
	return d.appIcon
}

// shellNotifyIcon calls Shell_NotifyIconW.
func shellNotifyIcon(msg uint32, nid *notifyIconData) bool {
	r, _, _ := procShellNotifyIconW.Call(uintptr(msg), uintptr(unsafe.Pointer(nid)))
	return r != 0
}

// smallIconSide and largeIconSide return the sides of small (tray) and
// large (balloon) icons at the DPI of the primary monitor, where the
// taskbar's notification area is.
func smallIconSide() int { return iconSide(49, 16) } // SM_CXSMICON
func largeIconSide() int { return iconSide(11, 32) } // SM_CXICON

func iconSide(metric uintptr, base int) int {
	dpi := primaryDPI()
	if available(procGetSystemMetricsForDpi) {
		if r, _, _ := procGetSystemMetricsForDpi.Call(metric, uintptr(dpi)); int32(r) > 0 {
			return int(int32(r))
		}
	}
	return shellIconSide(base, dpi)
}

// createShellIcon creates an icon of side × side pixels from the image of
// images that fits best (platform.PickIcon), scaled when its size differs:
// a 32-bit top-down DIB section with straight alpha as the color bitmap
// and an empty AND mask, as CreateIconIndirect takes alpha icons. The
// caller destroys it with DestroyIcon.
func createShellIcon(images []image.Image, side int) (uintptr, error) {
	img := platform.PickIcon(images, side)
	if img == nil {
		return 0, fmt.Errorf("win32: no icon image")
	}
	pixels := shellIconBGRA(img, side)

	bmi := bitmapInfo{bitmapInfoHeader: bitmapInfoHeader{
		size:     uint32(bitmapInfoHeaderSize),
		width:    int32(side),
		height:   -int32(side), // top-down
		planes:   1,
		bitCount: 32,
	}}
	const dibRGBColors = 0
	var bits uintptr
	color, _, e := procCreateDIBSection.Call(0, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if color == 0 || bits == 0 {
		return 0, winError("CreateDIBSection for an icon", e)
	}
	defer procDeleteObject.Call(color)
	copy(unsafe.Slice((*byte)(ptrFromUintptr(bits)), len(pixels)), pixels)

	mask := make([]byte, shellIconMaskSize(side))
	maskBitmap, _, e := procCreateBitmap.Call(uintptr(side), uintptr(side), 1, 1, uintptr(unsafe.Pointer(&mask[0])))
	if maskBitmap == 0 {
		return 0, winError("CreateBitmap for an icon mask", e)
	}
	defer procDeleteObject.Call(maskBitmap)

	type iconInfo struct { // ICONINFO
		icon       int32
		xHotspot   uint32
		yHotspot   uint32
		maskBitmap uintptr
		color      uintptr
	}
	info := iconInfo{icon: 1, maskBitmap: maskBitmap, color: color}
	h, _, e := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&info)))
	if h == 0 {
		return 0, winError("CreateIconIndirect", e)
	}
	return h, nil
}

// cloneMenu returns a deep copy of a menu.
func cloneMenu(items []platform.MenuItem) []platform.MenuItem {
	items = slices.Clone(items)
	for i := range items {
		items[i].Submenu = cloneMenu(items[i].Submenu)
	}
	return items
}
