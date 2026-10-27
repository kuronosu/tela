//go:build linux

package x11

import (
	"encoding/binary"
	"image"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
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

	// Window control (windowctl.go).
	wmState           xproto.Atom // WM_STATE (ICCCM 4.1.3.1)
	wmChangeState     xproto.Atom // WM_CHANGE_STATE (ICCCM 4.1.4)
	netSupported      xproto.Atom
	netWMState        xproto.Atom
	netWMStateMaxVert xproto.Atom
	netWMStateMaxHorz xproto.Atom
	netWMStateFull    xproto.Atom
	netWMStateAbove   xproto.Atom
	netWMStateHidden  xproto.Atom
	netWMStateModal   xproto.Atom
	netWMTypeDialog   xproto.Atom
	netWMMoveResize   xproto.Atom
	netActiveWindow   xproto.Atom
	netWMIcon         xproto.Atom
	motifWMHints      xproto.Atom
	gtkShowWindowMenu xproto.Atom
}

// atomEntry is an atom name and the field of atoms it is stored in.
type atomEntry struct {
	name string
	dst  *xproto.Atom
}

// list returns every atom of a with its name.
func (a *atoms) list() []atomEntry {
	return []atomEntry{
		{"WM_PROTOCOLS", &a.wmProtocols},
		{"WM_DELETE_WINDOW", &a.wmDeleteWindow},
		{"_NET_WM_PING", &a.netWMPing},
		{"_NET_WM_NAME", &a.netWMName},
		{"UTF8_STRING", &a.utf8String},
		{"_NET_WM_PID", &a.netWMPID},
		{"_NET_WM_WINDOW_TYPE", &a.netWMWindowType},
		{"_NET_WM_WINDOW_TYPE_NORMAL", &a.netWMTypeNormal},
		{"WM_STATE", &a.wmState},
		{"WM_CHANGE_STATE", &a.wmChangeState},
		{"_NET_SUPPORTED", &a.netSupported},
		{"_NET_WM_STATE", &a.netWMState},
		{"_NET_WM_STATE_MAXIMIZED_VERT", &a.netWMStateMaxVert},
		{"_NET_WM_STATE_MAXIMIZED_HORZ", &a.netWMStateMaxHorz},
		{"_NET_WM_STATE_FULLSCREEN", &a.netWMStateFull},
		{"_NET_WM_STATE_ABOVE", &a.netWMStateAbove},
		{"_NET_WM_STATE_HIDDEN", &a.netWMStateHidden},
		{"_NET_WM_STATE_MODAL", &a.netWMStateModal},
		{"_NET_WM_WINDOW_TYPE_DIALOG", &a.netWMTypeDialog},
		{"_NET_WM_MOVERESIZE", &a.netWMMoveResize},
		{"_NET_ACTIVE_WINDOW", &a.netActiveWindow},
		{"_NET_WM_ICON", &a.netWMIcon},
		{"_MOTIF_WM_HINTS", &a.motifWMHints},
		{"_GTK_SHOW_WINDOW_MENU", &a.gtkShowWindowMenu},
	}
}

// internAtoms interns every atom with one round trip: all requests are sent
// before the first reply is awaited.
func internAtoms(conn *xgb.Conn) (atoms, error) {
	var a atoms
	list := a.list()
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
	b.srv.setProperty(w, prop, typ, format, data)
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
	hintUSPosition  = 1 << 0
	hintPPosition   = 1 << 2
	hintPMinSize    = 1 << 4
	hintPMaxSize    = 1 << 5
	hintPWinGravity = 1 << 9

	// gravityStatic is StaticGravity: positions given to the window
	// manager (the hints, ConfigureWindow) are those of the client
	// window itself, the content area's top-left, not of the frame.
	gravityStatic = 10
)

// normalHints is the content of WM_NORMAL_HINTS, in physical pixels.
type normalHints struct {
	size     image.Point
	min, max image.Point // a zero component is no limit on that axis
	fixed    bool        // min = max = size
	pos      image.Point // the requested position, if hasPos
	hasPos   bool
}

// sizeHints encodes WM_NORMAL_HINTS (ICCCM 4.1.2.3, 18 CARD32): the
// size limits (min = max = size when fixed), a user-specified position
// when one is requested, and always StaticGravity, so that every position
// the backend gives the window manager is the content area's (EWMH
// "Window geometry"; mutter, KWin, Xfwm4 and Openbox honor it, as they do
// for GTK windows moved with static gravity).
func sizeHints(h normalHints) []byte {
	v := make([]uint32, 18)
	v[0] = hintPWinGravity
	v[17] = gravityStatic
	if h.hasPos {
		// Both flags: window managers ignore PPosition alone at map for
		// their placement policy, but honor USPosition (Xlib and GTK set
		// it for explicit positions too).
		v[0] |= hintUSPosition | hintPPosition
		v[1], v[2] = uint32(int32(h.pos.X)), uint32(int32(h.pos.Y))
	}
	minS, maxS := h.min, h.max
	if h.fixed {
		minS, maxS = h.size, h.size
	}
	if minS.X > 0 || minS.Y > 0 {
		v[0] |= hintPMinSize
		v[5], v[6] = uint32(max(minS.X, 1)), uint32(max(minS.Y, 1))
	}
	if maxS.X > 0 || maxS.Y > 0 {
		v[0] |= hintPMaxSize
		mx, my := maxS.X, maxS.Y
		if mx <= 0 {
			mx = maxWindowSide
		}
		if my <= 0 {
			my = maxWindowSide
		}
		v[7], v[8] = uint32(mx), uint32(my)
	}
	return card32s(v...)
}

// ICCCM WM_STATE values (4.1.3.1), also used as WM_HINTS initial_state
// and in WM_CHANGE_STATE.
const (
	stateWithdrawn = 0
	stateNormal    = 1
	stateIconic    = 3
)

// wmHints encodes WM_HINTS (ICCCM 4.1.2.4, 9 CARD32) with InputHint = True
// (the window manager gives the window keyboard focus) and StateHint: the
// state the window starts in when it is mapped from withdrawn, Normal or
// Iconic (a window minimized before it was shown).
func wmHints(iconic bool) []byte {
	const inputHint, stateHint = 1, 2
	state := uint32(stateNormal)
	if iconic {
		state = stateIconic
	}
	return card32s(inputHint|stateHint, 1, state, 0, 0, 0, 0, 0, 0)
}

// motifNoDecorations encodes _MOTIF_WM_HINTS (5 CARD32: flags, functions,
// decorations, input mode, status) asking for no decorations: the flags
// say only the decorations field is set, and it is 0. Every X11 window
// manager that draws frames reads it (GTK, Qt, SDL and winit set the same
// value for undecorated windows); the functions (move, resize, close) are
// left to the window manager's defaults.
func motifNoDecorations() []byte {
	const mwmHintsDecorations = 1 << 1
	return card32s(mwmHintsDecorations, 0, 0, 0, 0)
}

// netWMIcon encodes _NET_WM_ICON: for every image, its width and height
// and then its pixels as non-premultiplied ARGB CARDINALs, row by row
// (EWMH). Every size is sent; window managers and task bars pick the one
// they need.
func netWMIcon(images []image.Image) []byte {
	n := 0
	for _, img := range images {
		b := img.Bounds()
		n += 2 + b.Dx()*b.Dy()
	}
	out := make([]byte, 0, 4*n)
	for _, img := range images {
		px := platform.IconNRGBA(img)
		w, h := px.Rect.Dx(), px.Rect.Dy()
		out = binary.LittleEndian.AppendUint32(out, uint32(w))
		out = binary.LittleEndian.AppendUint32(out, uint32(h))
		for y := range h {
			row := px.Pix[y*px.Stride : y*px.Stride+4*w]
			for x := 0; x < len(row); x += 4 {
				argb := uint32(row[x+3])<<24 | uint32(row[x])<<16 | uint32(row[x+1])<<8 | uint32(row[x+2])
				out = binary.LittleEndian.AppendUint32(out, argb)
			}
		}
	}
	return out
}

const changePropertyOpcode = 18

// encodeChangeProperty encodes a ChangeProperty request by hand, like
// PutImage (present.go): xgb's generated request has only the 16-bit
// length field, which an icon property easily exceeds. big selects the
// BIG-REQUESTS form. data is in the client byte order; its length is a
// multiple of format/8.
func encodeChangeProperty(big bool, mode byte, win xproto.Window, prop, typ xproto.Atom, format byte, data []byte) []byte {
	hdr := 24
	if big {
		hdr += bigHeaderExtra
	}
	total := hdr + (len(data)+3)&^3
	req := make([]byte, total)
	le := binary.LittleEndian
	req[0], req[1] = changePropertyOpcode, mode
	o := 4
	if big {
		le.PutUint32(req[4:], uint32(total/4))
		o = 8
	} else {
		le.PutUint16(req[2:], uint16(total/4))
	}
	le.PutUint32(req[o:], uint32(win))
	le.PutUint32(req[o+4:], uint32(prop))
	le.PutUint32(req[o+8:], uint32(typ))
	req[o+12] = format
	le.PutUint32(req[o+16:], uint32(len(data))/uint32(format/8))
	copy(req[hdr:], data)
	return req
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
