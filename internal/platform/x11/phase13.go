//go:build linux

package x11

import (
	"strconv"
	"strings"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Phase-13 desktop integration (ADR-209, ADR-210): the clipboard's Data
// (clipboard.go, selection.go), drag and drop with XDND version 5
// (dnd.go, dragsource.go), global shortcuts falling back to key grabs
// (shortcuts.go) and ActivateWithToken (here). Linux has no native
// menus: SetMenu and PopupMenu return ErrUnsupported, and the layers
// above draw their menus.

// integration is the backend's phase-13 state (UI thread).
type integration struct {
	selWin  xproto.Window // the hidden window that owns CLIPBOARD and reads selections
	owned   map[xproto.Atom]*ownedSelection
	incrOut []*incrOut    // INCR transfers being served
	reads   []*conversion // selection reads, the first in progress
	drag    *dragSource   // the app's drag in progress
	sc      shortcuts
}

// ActivateWithToken activates the window with the startup notification
// ID another process of the app received (DESKTOP_STARTUP_ID): it sets
// _NET_STARTUP_ID on the window (read by the window manager at map, and
// for focus stealing prevention), sends _NET_ACTIVE_WINDOW with source
// indication 1 (application) and the timestamp of the ID's "_TIME<n>"
// suffix when it has one (the time of the user's action in the other
// process; else the latest input time), and ends the launch feedback with
// the startup notification "remove" message. "" is Activate.
func (w *Window) ActivateWithToken(token string) {
	if token == "" || !sendableToken(token) {
		w.Activate()
		return
	}
	if w.Closed() || !w.visible {
		return
	}
	b := w.b
	b.setProperty(w.id, b.atoms.netStartupID, b.atoms.utf8String, 8, []byte(token))
	w.deiconify()
	if w.mapped {
		t := b.userTime
		if ts, ok := tokenTime(token); ok {
			t = ts
		}
		var active xproto.Window
		for _, o := range b.loop.Windows() {
			if o.focused {
				active = o.id
			}
		}
		b.srv.clientMessage(w.id, b.atoms.netActiveWindow, sourceApplication, t, uint32(active))
	}
	b.startupRemove(token)
}

// sendableToken reports whether a token fits a property and a startup
// message: no NUL or newline, not huge.
func sendableToken(s string) bool { return !strings.ContainsAny(s, "\x00\n") && len(s) < 4096 }

// tokenTime returns the timestamp of a startup ID's "_TIME<n>" suffix
// (the startup notification specification's convention).
func tokenTime(token string) (uint32, bool) {
	i := strings.LastIndex(token, "_TIME")
	if i < 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(token[i+len("_TIME"):], 10, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// startupRemove sends the startup notification message "remove: ID=..."
// to the root window: a series of format-8 ClientMessages of 20 bytes,
// the first of type _NET_STARTUP_INFO_BEGIN and the rest
// _NET_STARTUP_INFO, the text NUL-terminated (startup-notification
// specification 0.1), sent from the backend's own hidden window.
func (b *Backend) startupRemove(token string) {
	win := b.selectionWindow()
	if win == 0 {
		return
	}
	for i, chunk := range startupChunks("remove: ID=" + startupQuote(token)) {
		typ := b.atoms.netStartupInfo
		if i == 0 {
			typ = b.atoms.netStartupInfoBegn
		}
		ev := xproto.ClientMessageEvent{Format: 8, Window: win, Type: typ, Data: xproto.ClientMessageDataUnionData8New(chunk)}
		b.xs.sendEvent(b.root, xproto.EventMaskPropertyChange, ev.Bytes())
	}
}

// startupQuote quotes a value of a startup notification message when it
// holds a space, a quote or a backslash (the specification's escaping).
func startupQuote(v string) string {
	if !strings.ContainsAny(v, " \"\\") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(v) + `"`
}

// startupChunks splits a message, with its terminating NUL, into the
// 20-byte pieces of the ClientMessages (the last one padded with NULs).
func startupChunks(msg string) [][]byte {
	b := append([]byte(msg), 0)
	var out [][]byte
	for len(b) > 0 {
		c := make([]byte, 20)
		n := copy(c, b)
		out = append(out, c)
		b = b[n:]
	}
	return out
}

// SetMenu returns ErrUnsupported: X11 has no native menu bar.
func (w *Window) SetMenu(menu []platform.MenuItem) error {
	platform.CheckMenu("SetMenu", menu)
	return platform.ErrUnsupported
}

// PopupMenu returns ErrUnsupported: X11 has no native popup menu.
func (w *Window) PopupMenu(at platform.Point, menu []platform.MenuItem, done func(platform.MenuItemID, error)) error {
	platform.CheckPopupMenu(at, menu, done != nil)
	return platform.ErrUnsupported
}
