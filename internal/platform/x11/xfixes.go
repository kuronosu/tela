//go:build linux

package x11

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// XFixes, hand-written on the vendored xgb like XInput2 and XKB (ext.go):
// only what follows a selection's owner, SelectSelectionInput (XFixes 2.0),
// for the compositing manager's _NET_WM_CM_S<screen> (transparent.go). A
// manager that takes the selection without the ICCCM MANAGER message is
// seen at once, as GTK 3 (gdk_x11_display_request_selection_notification)
// and Qt (QXcbConnection's xfixes selection events) see it.

const (
	xfixesOpQueryVersion                 = 0
	xfixesOpSelectSelectionInput         = 2
	xfixesSelectionNotify                = 0 // event offset from the first event
	xfixesSetSelectionOwnerMask          = 1 << 0
	xfixesSelectionWindowDestroy         = 1 << 1
	xfixesSelectionClientClose           = 1 << 2
	xfixesClientMajor, xfixesClientMinor = 5, 0
)

// xfixesExt is the XFixes extension, queried at the first watch: ok when
// the server has version 2.0 or later and the version was negotiated
// (QueryVersion must come before any other XFixes request).
type xfixesExt struct {
	checked bool
	ok      bool
	extension
}

// selectionNotify is a decoded XFixesSelectionNotify: the selection got
// a new owner (owner, 0 when none) through SetSelectionOwner, or lost it
// because its window was destroyed or its client closed (subtype).
type selectionNotify struct {
	raw       []byte
	subtype   byte
	window    xproto.Window
	owner     xproto.Window
	selection xproto.Atom
}

func (e selectionNotify) Bytes() []byte { return e.raw }

func (e selectionNotify) String() string {
	return fmt.Sprintf("XFixesSelectionNotify{subtype %d, window %d, owner %d, selection %d}", e.subtype, e.window, e.owner, e.selection)
}

// decodeSelectionNotify decodes the 32-byte event: code, subtype,
// sequence, window, owner, selection, timestamp, selection timestamp.
func decodeSelectionNotify(raw []byte) (selectionNotify, bool) {
	if len(raw) < 32 {
		return selectionNotify{}, false
	}
	return selectionNotify{
		raw:       raw,
		subtype:   raw[1],
		window:    xproto.Window(le.Uint32(raw[4:])),
		owner:     xproto.Window(le.Uint32(raw[8:])),
		selection: xproto.Atom(le.Uint32(raw[12:])),
	}, true
}

// selectSelectionInputRequest encodes XFixesSelectSelectionInput: window,
// selection, event mask.
func selectSelectionInputRequest(opcode byte, win xproto.Window, sel xproto.Atom, mask uint32) []byte {
	req := newRequest(opcode, xfixesOpSelectSelectionInput, 16)
	le.PutUint32(req[4:], uint32(win))
	le.PutUint32(req[8:], uint32(sel))
	le.PutUint32(req[12:], mask)
	return req
}

// --- connServer ---

func (s connServer) watchSelection(sel xproto.Atom) bool {
	b := s.b
	if !b.xfixes.checked {
		b.xfixes.checked = true
		if e := b.queryExtension("XFIXES"); e.present {
			req := newRequest(e.opcode, xfixesOpQueryVersion, 12)
			le.PutUint32(req[4:], xfixesClientMajor)
			le.PutUint32(req[8:], xfixesClientMinor)
			if r, err := b.roundTrip(req, 16); err == nil && le.Uint32(r[8:]) >= 2 {
				b.xfixes = xfixesExt{checked: true, ok: true, extension: e}
			}
		}
	}
	if !b.xfixes.ok || sel == 0 {
		return false
	}
	b.sendRequest(selectSelectionInputRequest(b.xfixes.opcode, b.root, sel,
		xfixesSetSelectionOwnerMask|xfixesSelectionWindowDestroy|xfixesSelectionClientClose))
	return true
}
