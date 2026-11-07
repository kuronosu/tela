package win32

// The pure half of the dark menu bar (menubar_dark_windows.go).
//
// Windows draws a menu bar light whatever the app theme is. Explorer's
// dark menus come from undocumented uxtheme exports (SetPreferredAppMode,
// AllowDarkModeForWindow), a DLL Tela does not load (ARCHITECTURE §2). The
// bar can be drawn dark with user32 and gdi32 alone: a window whose bar is
// themed gets two undocumented messages, WM_UAHDRAWMENU for the bar's
// background and WM_UAHDRAWMENUITEM for each item, which an app may answer
// by painting (Notepad++, and the win32-darkmode samples, do). The drop-down
// menus are the system's own and stay light: darkening them needs uxtheme
// (owner-drawing them would lose their access keys, screen reader text and
// the system's metrics).

// Owner-draw item states (ODS_*) the bar reports in itemState.
const (
	odsSelected = 0x0001 // the item's menu is open, or the item is pressed
	odsGrayed   = 0x0002
	odsDisabled = 0x0004
	odsHotLight = 0x0040 // under the mouse
	odsNoAccel  = 0x0100 // access keys are not underlined now
)

// barState is how a bar item is drawn.
type barState uint8

const (
	barNormal barState = iota
	barHot
	barPressed
	barDisabled
)

// barItemState maps the owner-draw state of an item to how it is drawn: a
// disabled item is dimmed whatever else it is, an open (or pressed) menu is
// pressed, the item under the mouse hot.
func barItemState(itemState uint32) barState {
	switch {
	case itemState&(odsGrayed|odsDisabled) != 0:
		return barDisabled
	case itemState&odsSelected != 0:
		return barPressed
	case itemState&odsHotLight != 0:
		return barHot
	}
	return barNormal
}

// winColor is a COLORREF (0x00BBGGRR).
type winColor uint32

func rgb(r, g, b uint8) winColor { return winColor(r) | winColor(g)<<8 | winColor(b)<<16 }

// The dark bar's colors, those of Windows 11's dark title bar and menus
// (#202020 and its hover and pressed greys).
var darkBar = struct {
	background, hot, pressed winColor
	text, disabled           winColor
}{
	background: rgb(0x20, 0x20, 0x20),
	hot:        rgb(0x38, 0x38, 0x38),
	pressed:    rgb(0x40, 0x40, 0x40),
	text:       rgb(0xFF, 0xFF, 0xFF),
	disabled:   rgb(0x7A, 0x7A, 0x7A),
}

// fill returns the background color of an item in a state.
func (s barState) fill() winColor {
	switch s {
	case barHot:
		return darkBar.hot
	case barPressed:
		return darkBar.pressed
	}
	return darkBar.background
}

// ink returns the text color of an item in a state.
func (s barState) ink() winColor {
	if s == barDisabled {
		return darkBar.disabled
	}
	return darkBar.text
}

// drawTextFlags returns the DrawTextW flags of a bar item's label: centered
// on one line, with the access key underlined unless the system hides it
// now (ODS_NOACCEL: the user has not pressed Alt and the "underline access
// keys" setting is off).
func drawTextFlags(itemState uint32) uint32 {
	const (
		dtCenter     = 0x0001
		dtVCenter    = 0x0004
		dtSingleLine = 0x0020
		dtHidePrefix = 0x00100000
	)
	f := uint32(dtCenter | dtVCenter | dtSingleLine)
	if itemState&odsNoAccel != 0 {
		f |= dtHidePrefix
	}
	return f
}

// darkBarOn reports whether a window draws its bar dark: the system app
// theme is dark and high contrast, whose colors the user chose, is off.
func darkBarOn(darkMode, highContrast bool) bool { return darkMode && !highContrast }
