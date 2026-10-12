//go:build linux

package x11

import (
	"encoding/binary"
	"image"
	"strings"
	"unicode/utf8"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// atoms holds the non-predefined atoms the backend uses.
type atoms struct {
	wmProtocols     xproto.Atom
	wmDeleteWindow  xproto.Atom
	netWMPing       xproto.Atom
	netWMName       xproto.Atom
	utf8String      xproto.Atom
	netWMPID        xproto.Atom
	netWMWindowType xproto.Atom
	netWMTypeNormal xproto.Atom
}

// internAtoms interns every atom with one round trip: all requests are sent
// before the first reply is awaited.
func internAtoms(conn *xgb.Conn) (atoms, error) {
	var a atoms
	list := []struct {
		name string
		dst  *xproto.Atom
	}{
		{"WM_PROTOCOLS", &a.wmProtocols},
		{"WM_DELETE_WINDOW", &a.wmDeleteWindow},
		{"_NET_WM_PING", &a.netWMPing},
		{"_NET_WM_NAME", &a.netWMName},
		{"UTF8_STRING", &a.utf8String},
		{"_NET_WM_PID", &a.netWMPID},
		{"_NET_WM_WINDOW_TYPE", &a.netWMWindowType},
		{"_NET_WM_WINDOW_TYPE_NORMAL", &a.netWMTypeNormal},
	}
	cookies := make([]xproto.InternAtomCookie, len(list))
	for i, e := range list {
		cookies[i] = xproto.InternAtom(conn, false, uint16(len(e.name)), e.name)
	}
	for i, c := range cookies {
		r, err := c.Reply()
		if err != nil {
			return a, err
		}
		*list[i].dst = r.Atom
	}
	return a, nil
}

// setProperty replaces a property. data is already encoded in the client
// byte order (xgb connects as little-endian); its length must be a multiple
// of format/8.
func (b *Backend) setProperty(w xproto.Window, prop, typ xproto.Atom, format byte, data []byte) {
	n := uint32(len(data)) / uint32(format/8)
	xproto.ChangeProperty(b.conn, xproto.PropModeReplace, w, prop, typ, format, n, data)
}

// card32s encodes values as a format-32 property payload.
func card32s(values ...uint32) []byte {
	buf := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[4*i:], v)
	}
	return buf
}

// ICCCM WM_SIZE_HINTS flags.
const (
	hintPMinSize = 1 << 4
	hintPMaxSize = 1 << 5
)

// sizeHints encodes WM_NORMAL_HINTS (ICCCM 4.1.2.3, 18 CARD32): a minimum
// size when minSize is non-zero, and min = max = size when fixed.
func sizeHints(size, minSize image.Point, fixed bool) []byte {
	v := make([]uint32, 18)
	if fixed {
		v[0] = hintPMinSize | hintPMaxSize
		v[5], v[6] = uint32(size.X), uint32(size.Y)
		v[7], v[8] = uint32(size.X), uint32(size.Y)
	} else if minSize.X > 0 || minSize.Y > 0 {
		v[0] = hintPMinSize
		v[5], v[6] = uint32(max(minSize.X, 1)), uint32(max(minSize.Y, 1))
	}
	return card32s(v...)
}

// wmHints encodes WM_HINTS (ICCCM 4.1.2.4, 9 CARD32) with InputHint = True
// (the window manager gives the window keyboard focus) and StateHint =
// NormalState.
func wmHints() []byte {
	const inputHint, stateHint, normalState = 1, 2, 1
	return card32s(inputHint|stateHint, 1, normalState, 0, 0, 0, 0, 0, 0)
}

// wmClass returns the WM_CLASS payload ("instance\0class\0") for appID.
// With an AppID both parts are the AppID, which is what .desktop files
// match through StartupWMClass. Without one, exe (platform.DefaultAppID,
// the executable name as invoked) is the instance and its capitalized form
// the class, as Xlib and GTK do.
func wmClass(appID, exe string) []byte {
	instance, class := appID, appID
	if appID == "" {
		instance = exe
		if instance == "" {
			instance = "tela"
		}
		r, n := utf8.DecodeRuneInString(instance)
		class = strings.ToUpper(string(r)) + instance[n:]
	}
	return []byte(instance + "\x00" + class + "\x00")
}

// latin1 converts s to ISO 8859-1 for the STRING type of WM_NAME, replacing
// characters outside Latin-1 with '?'. Window managers that understand
// UTF-8 read _NET_WM_NAME instead.
func latin1(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if r < 0x100 {
			out = append(out, byte(r))
		} else {
			out = append(out, '?')
		}
	}
	return out
}
