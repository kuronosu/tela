package win32

import (
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// settingsFrom builds the platform settings from the three values Windows
// keeps them in:
//
//   - appsUseLightTheme is the AppsUseLightTheme DWORD of
//     HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize;
//     0 means the dark app mode. known is false when the value does not
//     exist (before Windows 10 1809, Wine): the light theme.
//   - clientAreaAnimation is SPI_GETCLIENTAREAANIMATION: whether the
//     system plays animations inside windows. "Show animations in Windows"
//     turned off (the user asked for less motion) is ReducedMotion.
//   - doubleClickMs is GetDoubleClickTime.
func settingsFrom(appsUseLightTheme uint32, known, clientAreaAnimation bool, doubleClickMs uint32) platform.SystemSettings {
	return platform.SystemSettings{
		DarkMode:            known && appsUseLightTheme == 0,
		ReducedMotion:       !clientAreaAnimation,
		DoubleClickInterval: time.Duration(doubleClickMs) * time.Millisecond,
	}
}
