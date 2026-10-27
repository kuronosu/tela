package win32

import (
	"image"
	"strings"
	utf16pkg "unicode/utf16"

	xdraw "golang.org/x/image/draw"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of the tray icons and balloon notifications
// (tray_windows.go): the callback decoding, menu building, text limits and
// icon pixels.

// Callback events of the notification area icons (shellapi.h).
const (
	ninSelect           = 0x0400 // WM_USER: primary click
	ninKeySelect        = 0x0401 // NIN_SELECT | NINF_KEY: Enter or Space
	ninBalloonShow      = 0x0402
	ninBalloonHide      = 0x0403 // the balloon went away otherwise (icon deleted)
	ninBalloonTimeout   = 0x0404 // timed out or closed by the user
	ninBalloonUserClick = 0x0405
	trayContextMenu     = 0x007B // WM_CONTEXTMENU: secondary click, Shift+F10, menu key
)

// trayCallback decodes the callback message of an icon in
// NOTIFYICON_VERSION_4: the event in the low word of lParam, the icon's ID
// in the high word, and the anchor point (screen coordinates of the click,
// or of the icon for keyboard events) in wParam.
func trayCallback(wparam, lparam uintptr) (event, uid uint16, x, y int) {
	x, y = pointFromLParam(wparam)
	return loword(lparam), hiword(lparam), x, y
}

// balloonEvent maps a callback event of a notification's icon to the
// NotificationEvent it reports.
func balloonEvent(event uint16) (platform.NotificationEventKind, bool) {
	switch event {
	case ninBalloonUserClick:
		return platform.NotificationActivated, true
	case ninBalloonTimeout, ninBalloonHide:
		return platform.NotificationClosed, true
	}
	return 0, false
}

// putUTF16 stores s in dst as a NUL-terminated UTF-16 string, cut to fit
// (never between the two halves of a surrogate pair). Invalid UTF-8
// becomes U+FFFD and NUL characters are dropped.
func putUTF16(dst []uint16, s string) {
	clear(dst)
	if len(dst) == 0 {
		return
	}
	u := utf16pkg.Encode([]rune(sanitizeTitle(s)))
	n := min(len(u), len(dst)-1)
	if n > 0 && n < len(u) && utf16pkg.IsSurrogate(rune(u[n-1])) && u[n-1] < 0xDC00 {
		n-- // a high surrogate whose low half was cut
	}
	copy(dst, u[:n])
}

// balloonText returns the title and text of a balloon. Windows removes the
// balloon when its text is empty, so a notification without a body shows
// its title as the text, and one with neither a single space.
func balloonText(title, body string) (string, string) {
	switch {
	case body != "":
		return title, body
	case title != "":
		return "", title
	}
	return "", " "
}

// Menu item flags of AppendMenuW (winuser.h).
const (
	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfPopup     = 0x0010
	mfSeparator = 0x0800
)

// menuEntry is an item of a native popup menu, built by buildMenu.
type menuEntry struct {
	label string // escaped for AppendMenuW
	flags uint32
	cmd   uint32 // command ID: an index into the command table plus one
	sub   []menuEntry
}

// buildMenu converts a tray menu to native menu entries. Command IDs are
// small sequential numbers (1, 2, …) rather than the app's MenuItemIDs,
// which are 32-bit, while WM_COMMAND carries 16; cmds maps them back
// (cmds[cmd-1]).
func buildMenu(items []platform.MenuItem) (entries []menuEntry, cmds []platform.MenuItemID) {
	var walk func(items []platform.MenuItem) []menuEntry
	walk = func(items []platform.MenuItem) []menuEntry {
		out := make([]menuEntry, 0, len(items))
		for _, it := range items {
			e := menuEntry{label: menuLabel(it.Label)}
			switch {
			case it.Separator:
				e = menuEntry{flags: mfSeparator}
			case len(it.Submenu) > 0:
				e.flags = mfPopup
				e.sub = walk(it.Submenu)
			default:
				cmds = append(cmds, it.ID)
				e.cmd = uint32(len(cmds))
				if it.Checkable && it.Checked {
					e.flags |= mfChecked
				}
			}
			if it.Disabled && !it.Separator {
				e.flags |= mfGrayed
			}
			out = append(out, e)
		}
		return out
	}
	entries = walk(items)
	return entries, cmds
}

// menuLabel escapes a plain label for a menu: '&' would mark the next
// character as the mnemonic, so it is doubled, and a tab would start the
// accelerator column, so it becomes a space.
func menuLabel(s string) string {
	s = strings.ReplaceAll(s, "&", "&&")
	return strings.ReplaceAll(s, "\t", " ")
}

// shellIconSide returns the side in pixels of an icon at dpi: base pixels
// (16 for small icons, 32 for large ones) scaled like GetSystemMetricsForDpi
// does, the fallback where it is missing.
func shellIconSide(base int, dpi uint32) int {
	if dpi == 0 {
		dpi = baseDPI
	}
	return (base*int(dpi) + baseDPI/2) / baseDPI
}

// shellIconBGRA returns the color bitmap of a side × side 32-bit icon
// showing img: top-down rows of B, G, R, A with straight (not
// premultiplied) alpha, as icon bitmaps hold it. An image of another size
// is scaled to fit (Catmull-Rom, keeping its aspect ratio, centered on a
// transparent square).
func shellIconBGRA(img image.Image, side int) []byte {
	src := platform.IconNRGBA(img)
	dst := src
	if src.Rect.Dx() != side || src.Rect.Dy() != side {
		dst = image.NewNRGBA(image.Rect(0, 0, side, side))
		w, h := src.Rect.Dx(), src.Rect.Dy()
		fw, fh := side, side
		if w > h {
			fh = max(1, (h*side+w/2)/w)
		} else if h > w {
			fw = max(1, (w*side+h/2)/h)
		}
		x0, y0 := (side-fw)/2, (side-fh)/2
		xdraw.CatmullRom.Scale(dst, image.Rect(x0, y0, x0+fw, y0+fh), src, src.Rect, xdraw.Src, nil)
	}
	out := make([]byte, 4*side*side)
	for y := range side {
		row := dst.Pix[y*dst.Stride : y*dst.Stride+4*side]
		for x := range side {
			p := row[4*x : 4*x+4]
			o := out[4*(y*side+x):]
			o[0], o[1], o[2], o[3] = p[2], p[1], p[0], p[3]
		}
	}
	return out
}

// shellIconMaskSize returns the size in bytes of the 1-bit AND mask of a
// side × side icon: rows padded to 16 bits, as monochrome bitmaps are.
func shellIconMaskSize(side int) int { return (side + 15) / 16 * 2 * side }

// iconIDs allocates the icon IDs of Shell_NotifyIconW. Version 4
// callbacks carry the ID in 16 bits. IDs go round from 1 rather than
// reusing the smallest free one, so that a callback still on its way for a
// removed icon does not reach the next icon at once.
type iconIDs struct {
	used map[uint16]bool
	last uint16
}

// take returns a free ID, or 0 when all 65535 are in use.
func (ids *iconIDs) take() uint16 {
	if ids.used == nil {
		ids.used = map[uint16]bool{}
	}
	for range 0xFFFF {
		ids.last++
		if ids.last == 0 {
			ids.last = 1
		}
		if !ids.used[ids.last] {
			ids.used[ids.last] = true
			return ids.last
		}
	}
	return 0
}

// release frees id.
func (ids *iconIDs) release(id uint16) { delete(ids.used, id) }
