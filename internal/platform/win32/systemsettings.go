package win32

import (
	"image/color"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// rawSettings are the values Windows keeps the system settings in, as the
// OS code (settings.go) reads them; settingsFrom turns them into the
// platform's SystemSettings.
type rawSettings struct {
	// AppsUseLightTheme is the AppsUseLightTheme DWORD of
	// HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize;
	// 0 means the dark app mode. ThemeKnown is false when the value does
	// not exist (before Windows 10 1809, Wine): the light theme.
	AppsUseLightTheme uint32
	ThemeKnown        bool
	// ClientAreaAnimation is SPI_GETCLIENTAREAANIMATION: whether the
	// system plays animations inside windows. "Show animations in
	// Windows" turned off (the user asked for less motion) is
	// ReducedMotion.
	ClientAreaAnimation bool
	// DoubleClickMs is GetDoubleClickTime.
	DoubleClickMs uint32
	// Accent is the accent color (see accentFromABGR) when HasAccent is
	// set.
	Accent    color.NRGBA
	HasAccent bool
	// HighContrast is HCF_HIGHCONTRASTON of SPI_GETHIGHCONTRAST.
	HighContrast bool
	// TextScalePercent is the TextScaleFactor DWORD of
	// HKCU\Software\Microsoft\Accessibility ("Text size" in the Settings
	// app): 100 to 225; 0 when the value does not exist (100).
	TextScalePercent uint32
}

// settingsFrom builds the platform settings from the values Windows keeps
// them in. It is pure: the OS code reads the values.
func settingsFrom(r rawSettings) platform.SystemSettings {
	return platform.SystemSettings{
		DarkMode:            r.ThemeKnown && r.AppsUseLightTheme == 0,
		ReducedMotion:       !r.ClientAreaAnimation,
		DoubleClickInterval: time.Duration(r.DoubleClickMs) * time.Millisecond,
		AccentColor:         r.Accent,
		HasAccentColor:      r.HasAccent,
		HighContrast:        r.HighContrast,
		TextScale:           textScaleFrom(r.TextScalePercent),
	}
}

// Windows' "Text size" slider goes from 100% to 225%.
const (
	minTextScalePercent = 100
	maxTextScalePercent = 225
)

// textScaleFrom converts the TextScaleFactor percentage to a factor: an
// absent value (0) is 100%, and values outside the slider's range are
// clamped to it.
func textScaleFrom(percent uint32) float64 {
	percent = min(max(percent, minTextScalePercent), maxTextScalePercent)
	return float64(percent) / 100
}

// accentFromABGR converts the DWORD of HKCU\Software\Microsoft\Windows\
// DWM\AccentColor, which holds 0xAABBGGRR, to an opaque color: the alpha
// byte is not an opacity there (it is 0xFF in practice).
func accentFromABGR(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v), G: uint8(v >> 8), B: uint8(v >> 16), A: 0xFF}
}

// accentFromARGB converts the color DwmGetColorizationColor returns,
// 0xAARRGGBB, to an opaque color (its alpha is the opacity of the
// colorization, not of the accent).
func accentFromARGB(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}
}

// highContrast is HIGHCONTRASTW, the argument of SPI_GETHIGHCONTRAST.
type highContrast struct {
	size          uint32 // cbSize: must be set before the call
	flags         uint32 // dwFlags
	defaultScheme *uint16
}

const (
	spiGetHighContrast = 0x0042
	hcfHighContrastOn  = 0x00000001
)
