//go:build windows

package win32

import (
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

// readSettings reads the system settings: the app color mode, whether the
// system animates window contents, and the double-click time.
func readSettings() platform.SystemSettings {
	light, known := readAppsUseLightTheme()
	ms, _, _ := procGetDoubleClickTime.Call()
	return settingsFrom(light, known, clientAreaAnimation(), uint32(ms))
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
// animation setting and the double-click time, among many others. It reads
// the three values again and, if any differs, updates the title bars and
// queues a SettingsEvent. Every window receives the broadcast; only the
// first one finds a change.
func (b *Backend) settingsChanged() {
	s := readSettings()
	if s == b.settings {
		return
	}
	b.settings = s
	for _, w := range b.loop.Windows() {
		w.applyTheme(s.DarkMode)
	}
	b.enqueue(nil, platform.SettingsEvent{Settings: s})
}

// applyTheme makes the title bar follow the color mode
// (DWMWA_USE_IMMERSIVE_DARK_MODE). It does nothing where DWM has no such
// attribute (before Windows 10 1809, Wine).
func (w *Window) applyTheme(dark bool) {
	if !available(procDwmSetWindowAttribute) {
		return
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
