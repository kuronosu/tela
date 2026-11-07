//go:build windows

package win32

// The structures of the dark menu bar (menubar_dark_windows.go), laid out as
// C does (each field naturally aligned).

// Undocumented messages of the themed menu bar (uxtheme's "UAH" hooks).
const (
	wmUAHDrawMenu     = 0x0091 // lParam: *uahMenu; paint the bar's background
	wmUAHDrawMenuItem = 0x0092 // lParam: *uahDrawMenuItem; paint one item
)

// uahMenu is UAHMENU: the bar being drawn.
type uahMenu struct {
	hmenu uintptr
	hdc   uintptr // a DC of the window (not the client area): window coordinates
	flags uint32
}

// drawItemStruct is DRAWITEMSTRUCT.
type drawItemStruct struct {
	ctlType    uint32
	ctlID      uint32
	itemID     uint32
	itemAction uint32
	itemState  uint32
	hwndItem   uintptr
	hdc        uintptr
	rcItem     rect
	itemData   uintptr
}

// uahDrawMenuItem is UAHDRAWMENUITEM, of which only the prefix that
// matters is declared: the item, the bar and the item's position in it
// (UAHMENUITEM starts with iPosition; its metrics follow).
type uahDrawMenuItem struct {
	dis      drawItemStruct
	um       uahMenu
	position int32
}

// menuItemInfo is MENUITEMINFOW.
type menuItemInfo struct {
	size      uint32
	mask      uint32
	typ       uint32
	state     uint32
	id        uint32
	subMenu   uintptr
	checked   uintptr
	unchecked uintptr
	itemData  uintptr
	typeData  *uint16
	cch       uint32
	bitmap    uintptr
}

// menuBarInfo is MENUBARINFO.
type menuBarInfo struct {
	size    uint32
	bar     rect // in screen coordinates
	menu    uintptr
	wnd     uintptr
	focused uint32 // fBarFocused and fFocused
}

// logFont is LOGFONTW.
type logFont struct {
	height, width, escapement, orientation, weight       int32
	italic, underline, strikeOut, charSet                uint8
	outPrecision, clipPrecision, quality, pitchAndFamily uint8
	faceName                                             [32]uint16
}

// nonClientMetrics is NONCLIENTMETRICSW (Vista and later), of which the
// bar reads the menu font.
type nonClientMetrics struct {
	size                        uint32
	borderWidth, scrollWidth    int32
	scrollHeight                int32
	captionWidth, captionHeight int32
	captionFont                 logFont
	smCaptionWidth              int32
	smCaptionHeight             int32
	smCaptionFont               logFont
	menuWidth, menuHeight       int32
	menuFont                    logFont
	statusFont, messageFont     logFont
	paddedBorderWidth           int32
}
