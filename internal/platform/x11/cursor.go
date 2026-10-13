//go:build linux

package x11

import (
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
)

// Glyph indices in the core "cursor" font (X11/cursorfont.h).
const (
	xcBottomLeftCorner = 12
	xcBottomRightCorn  = 14
	xcBottomSide       = 16
	xcCircle           = 24
	xcCrosshair        = 34
	xcFleur            = 52
	xcHand1            = 58
	xcHand2            = 60
	xcLeftPtr          = 68
	xcLeftSide         = 70
	xcPlus             = 90
	xcQuestionArrow    = 92
	xcRightSide        = 96
	xcSbHDoubleArrow   = 108
	xcSbVDoubleArrow   = 116
	xcTopLeftCorner    = 134
	xcTopRightCorner   = 136
	xcTopSide          = 138
	xcWatch            = 150
	xcXterm            = 152
)

// cursorGlyph maps a cursor to a glyph of the core cursor font. ok is false
// for CursorNone, which is an invisible cursor rather than a glyph.
//
// The core font predates CSS cursors, so several shapes have no glyph and
// fall back to the closest one or to left_ptr (context-menu, alias, copy,
// zoom-in/out, vertical-text). winit and GTK load the user's Xcursor theme
// by CSS name first and only fall back to the core font; theme loading
// (Xcursor files, or the RENDER-based cursors Xwayland supports) is a later
// improvement.
func cursorGlyph(c platform.Cursor) (glyph uint16, ok bool) {
	switch c {
	case platform.CursorNone:
		return 0, false
	case platform.CursorHelp:
		return xcQuestionArrow, true
	case platform.CursorPointer:
		return xcHand2, true
	case platform.CursorProgress, platform.CursorWait:
		return xcWatch, true
	case platform.CursorCell:
		return xcPlus, true
	case platform.CursorCrosshair:
		return xcCrosshair, true
	case platform.CursorText, platform.CursorVerticalText:
		return xcXterm, true
	case platform.CursorMove, platform.CursorAllScroll, platform.CursorGrabbing:
		return xcFleur, true
	case platform.CursorGrab:
		return xcHand1, true
	case platform.CursorNoDrop, platform.CursorNotAllowed:
		return xcCircle, true
	case platform.CursorEResize:
		return xcRightSide, true
	case platform.CursorNResize:
		return xcTopSide, true
	case platform.CursorNEResize, platform.CursorNESWResize:
		return xcTopRightCorner, true
	case platform.CursorNWResize, platform.CursorNWSEResize:
		return xcTopLeftCorner, true
	case platform.CursorSResize:
		return xcBottomSide, true
	case platform.CursorSEResize:
		return xcBottomRightCorn, true
	case platform.CursorSWResize:
		return xcBottomLeftCorner, true
	case platform.CursorWResize:
		return xcLeftSide, true
	case platform.CursorEWResize, platform.CursorColResize:
		return xcSbHDoubleArrow, true
	case platform.CursorNSResize, platform.CursorRowResize:
		return xcSbVDoubleArrow, true
	}
	return xcLeftPtr, true
}

// cursorFor returns the server cursor for c, creating it on first use. It
// returns 0 (None: inherit the parent's cursor) if the cursor cannot be
// created (no cursor font, XIDs exhausted). Failures are cached like
// successes: xgb hands out XIDs from a counter and an unused one is never
// returned, so retrying on every SetCursor would leak one per call. The
// cursor XID is allocated only once the font is known to be usable.
func (b *Backend) cursorFor(c platform.Cursor) xproto.Cursor {
	if id, ok := b.cursors[c]; ok {
		return id
	}
	id := b.createCursor(c)
	b.cursors[c] = id
	return id
}

// createCursor creates the server cursor for c, or returns 0.
func (b *Backend) createCursor(c platform.Cursor) xproto.Cursor {
	glyph, ok := cursorGlyph(c)
	if !ok {
		id, err := xproto.NewCursorId(b.conn)
		if err != nil || !b.createInvisibleCursor(id) {
			return 0
		}
		return id
	}
	font, ok := b.cursorFont()
	if !ok {
		return 0
	}
	id, err := xproto.NewCursorId(b.conn)
	if err != nil {
		return 0
	}
	// Black glyph with a white outline, the classic X cursor colors.
	xproto.CreateGlyphCursor(b.conn, id, font, font, glyph, glyph+1, 0, 0, 0, 0xffff, 0xffff, 0xffff)
	return id
}

// cursorFont opens the core cursor font once.
func (b *Backend) cursorFont() (xproto.Font, bool) {
	if b.cursorFt != 0 {
		return b.cursorFt, b.fontOK
	}
	id, err := xproto.NewFontId(b.conn)
	if err != nil {
		return 0, false
	}
	const name = "cursor"
	b.cursorFt = id
	b.fontOK = xproto.OpenFontChecked(b.conn, id, uint16(len(name)), name).Check() == nil
	return id, b.fontOK
}

// createInvisibleCursor makes id a fully transparent 1x1 cursor: a
// depth-1 pixmap cleared to 0 used as both source and mask.
func (b *Backend) createInvisibleCursor(id xproto.Cursor) bool {
	pix, err := xproto.NewPixmapId(b.conn)
	if err != nil {
		return false
	}
	gc, err := xproto.NewGcontextId(b.conn)
	if err != nil {
		return false
	}
	xproto.CreatePixmap(b.conn, 1, pix, xproto.Drawable(b.root), 1, 1)
	xproto.CreateGC(b.conn, gc, xproto.Drawable(pix), xproto.GcForeground, []uint32{0})
	xproto.PolyFillRectangle(b.conn, xproto.Drawable(pix), gc, []xproto.Rectangle{{Width: 1, Height: 1}})
	xproto.CreateCursor(b.conn, id, pix, pix, 0, 0, 0, 0, 0, 0, 0, 0)
	xproto.FreeGC(b.conn, gc)
	xproto.FreePixmap(b.conn, pix)
	return true
}
