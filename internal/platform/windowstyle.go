package platform

import "fmt"

// WindowColor is an 8-bit sRGB color for the window chrome the OS draws
// (WindowOptions.TitleBar). The zero value, with Set false, means "the
// system's own color".
type WindowColor struct {
	R, G, B uint8
	// Set is false for the system's color.
	Set bool
}

// TitleBarColors are the colors of the title bar and border the OS draws
// for a decorated window. A color that is not Set keeps the system's. Only
// Windows 11 (build 22000 and later) honors them; every other backend
// ignores them (see "Window styling" in the package documentation).
type TitleBarColors struct {
	// Background is the title bar's background (the caption).
	Background WindowColor
	// Text is the color of the title and of the caption buttons' glyphs.
	Text WindowColor
	// Border is the color of the window's border (also drawn around a
	// Frameless window by Windows 11).
	Border WindowColor
}

// IsZero reports whether every color is the system's.
func (c TitleBarColors) IsZero() bool { return c == TitleBarColors{} }

// Backdrop is the system material behind a window: Mica or Acrylic on
// Windows 11. The zero value is the system's choice.
type Backdrop uint8

const (
	// BackdropDefault leaves the choice to the system.
	BackdropDefault Backdrop = iota
	// BackdropNone turns the system backdrop off: a plain window.
	BackdropNone
	// BackdropMica is the Mica material (DWMSBT_MAINWINDOW).
	BackdropMica
	// BackdropMicaAlt is Mica Alt, the deeper tinted Mica of tabbed
	// windows (DWMSBT_TABBEDWINDOW).
	BackdropMicaAlt
	// BackdropAcrylic is the translucent, blurred Acrylic material
	// (DWMSBT_TRANSIENTWINDOW).
	BackdropAcrylic
	numBackdrops
)

var backdropNames = [numBackdrops]string{"Default", "None", "Mica", "MicaAlt", "Acrylic"}

// String returns the name of the backdrop ("Mica").
func (b Backdrop) String() string {
	if b < numBackdrops {
		return backdropNames[b]
	}
	return fmt.Sprintf("Backdrop(%d)", uint8(b))
}

// Valid reports whether b is one of the constants.
func (b Backdrop) Valid() bool { return b < numBackdrops }

// BackdropNames returns the names of the valid backdrops, in order.
func BackdropNames() []string { return append([]string(nil), backdropNames[:]...) }

// ClientBackdrop reports whether b is a material that shows through the
// window's client area where the app draws nothing (Mica, Mica Alt and
// Acrylic), as opposed to the system's choice or none.
func (b Backdrop) ClientBackdrop() bool {
	return b == BackdropMica || b == BackdropMicaAlt || b == BackdropAcrylic
}

// AlphaPresenter is implemented by windows that show the alpha channel of
// the frames they present: a Windows 11 window whose Backdrop is Mica,
// MicaAlt or Acrylic and whose OS supports it. Present's pixels are
// premultiplied, and a transparent one shows the backdrop. The layer above
// asks after creating the window and after every SetBackdrop; when it
// reports true, a root without a background is rendered transparent
// instead of white. Windows that do not implement it, or report false,
// get opaque frames as always.
type AlphaPresenter interface {
	PresentsAlpha() bool
}

// CheckBackdrop panics with the valid names when b is not a Backdrop.
func CheckBackdrop(what string, b Backdrop) {
	if !b.Valid() {
		panicf("platform: %s is %v; the valid backdrops are %v", what, b, backdropNames[:])
	}
}
