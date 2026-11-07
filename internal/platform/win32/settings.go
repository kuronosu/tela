//go:build windows

package win32

import (
	"image/color"
	"unsafe"

	winreg "golang.org/x/sys/windows/registry"

	"github.com/kuronosu/tela/internal/platform"
)

const (
	personalizeKey        = `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`
	appsUseLightThemeName = "AppsUseLightTheme"

	spiGetClientAreaAnimation = 0x1042

	// DWMWA_USE_IMMERSIVE_DARK_MODE: 20 from Windows 10 2004 (and Windows
	// 11), 19 on 1809 and 1903.
	dwmwaUseImmersiveDarkMode       = 20
	dwmwaUseImmersiveDarkModeBefore = 19

	// DWMWA_WINDOW_CORNER_PREFERENCE and DWMWCP_ROUND (Windows 11).
	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2
)

const (
	dwmKey          = `Software\Microsoft\Windows\DWM`
	accentColorName = "AccentColor"

	accessibilityKey    = `Software\Microsoft\Accessibility`
	textScaleFactorName = "TextScaleFactor"
)

// readSettings reads the system settings: the app color mode, whether the
// system animates window contents, the double-click time, the accent color,
// high contrast and the text scale.
func readSettings() platform.SystemSettings {
	light, known := readAppsUseLightTheme()
	ms, _, _ := procGetDoubleClickTime.Call()
	accent, hasAccent := readAccentColor()
	percent, _ := readDWORD(accessibilityKey, textScaleFactorName)
	return settingsFrom(rawSettings{
		AppsUseLightTheme:   light,
		ThemeKnown:          known,
		ClientAreaAnimation: clientAreaAnimation(),
		DoubleClickMs:       uint32(ms),
		Accent:              accent,
		HasAccent:           hasAccent,
		HighContrast:        highContrastOn(),
		TextScalePercent:    percent,
	})
}

// readDWORD reads a DWORD of the current user's registry; ok is false when
// the key or the value does not exist or is not a number.
func readDWORD(key, name string) (value uint32, ok bool) {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, key, winreg.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// readAccentColor reads the accent color the user chose.
//
// HKCU\Software\Microsoft\Windows\DWM\AccentColor (ABGR) is the value the
// Settings app writes for "Accent color", and it keeps the accent when
// "Show accent color on title bars" is off. DwmGetColorizationColor (ARGB)
// is the color DWM blends window frames with: Windows 10 and 11 set it to
// the accent, but with an automatic accent or a frame colorization that is
// off it can lag behind or differ, so it is the fallback for systems that
// lack the registry value (older Windows, Wine), not the first choice.
// TestLiveSettings logs both, to compare them with the Settings app on a
// real machine.
func readAccentColor() (color.NRGBA, bool) {
	if v, ok := readDWORD(dwmKey, accentColorName); ok {
		return accentFromABGR(v), true
	}
	if !available(procDwmGetColorizationColor) {
		return color.NRGBA{}, false
	}
	var c, opaque uint32
	if hr, _, _ := procDwmGetColorizationColor.Call(uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&opaque))); hr != 0 {
		return color.NRGBA{}, false
	}
	return accentFromARGB(c), true
}

// highContrastOn reports HCF_HIGHCONTRASTON of SPI_GETHIGHCONTRAST (false
// when the query fails).
func highContrastOn() bool {
	hc := highContrast{size: uint32(unsafe.Sizeof(highContrast{}))}
	r, _, _ := procSystemParametersInfoW.Call(spiGetHighContrast, uintptr(hc.size), uintptr(unsafe.Pointer(&hc)), 0)
	return r != 0 && hc.flags&hcfHighContrastOn != 0
}

// readAppsUseLightTheme reads the app color mode of the current user; known
// is false when the value is missing (older Windows, Wine).
func readAppsUseLightTheme() (value uint32, known bool) {
	k, err := winreg.OpenKey(winreg.CURRENT_USER, personalizeKey, winreg.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(appsUseLightThemeName)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// clientAreaAnimation reports SPI_GETCLIENTAREAANIMATION. When the query
// fails the system is taken to animate (no reduced motion).
func clientAreaAnimation() bool {
	var on int32
	r, _, _ := procSystemParametersInfoW.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&on)), 0)
	return r == 0 || on != 0
}

// Settings returns the system settings as the backend last read them: at
// creation and at every WM_SETTINGCHANGE (a SettingsEvent reports the
// changes).
func (b *Backend) Settings() platform.SystemSettings { return b.settings }

// settingsChanged handles WM_SETTINGCHANGE, which Windows broadcasts to the
// top-level windows for the color mode (lParam "ImmersiveColorSet"), the
// animation setting, high contrast (SPI_SETHIGHCONTRAST), the text size and
// the double-click time, among many others, and also WM_DWMCOLORIZATIONCOLORCHANGED
// (the accent color) and WM_SYSCOLORCHANGE. It reads every value again and,
// if any differs, updates the title bars and queues a SettingsEvent. Every
// window receives the broadcast; only the first one finds a change.
func (b *Backend) settingsChanged() {
	s := readSettings()
	if s == b.settings {
		return
	}
	b.settings = s
	for _, w := range b.loop.Windows() {
		w.applyTheme(s.DarkMode)
		w.refreshBar() // the menu bar follows dark mode too (menubar_dark.go)
	}
	b.enqueue(nil, platform.SettingsEvent{Settings: s})
}

// applyTheme makes the title bar follow the color mode
// (DWMWA_USE_IMMERSIVE_DARK_MODE). It does nothing where DWM has no such
// attribute (before Windows 10 1809, Wine). The caption buttons DWM keeps
// over a TitleBarOverlay window follow the color behind them instead when
// there is one (captionBackground, captionDark).
func (w *Window) applyTheme(dark bool) {
	if !available(procDwmSetWindowAttribute) {
		return
	}
	if w.overlay {
		dark = captionDark(w.captionBackground(), dark)
	}
	var v int32
	if dark {
		v = 1
	}
	for _, attr := range [...]uintptr{dwmwaUseImmersiveDarkMode, dwmwaUseImmersiveDarkModeBefore} {
		if hr, _, _ := procDwmSetWindowAttribute.Call(w.hwnd, attr, uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v)); hr == 0 {
			return
		}
	}
}
