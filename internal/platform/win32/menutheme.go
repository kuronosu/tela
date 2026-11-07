package win32

// The pure half of dark popup menus (menutheme_windows.go; ADR-239).
//
// Windows draws the menu bar's drop-downs, context menus and tray menus
// light unless the process opts in to dark mode through uxtheme's unnamed
// exports (the ones Explorer and Notepad++ call). The menus follow the
// process-wide mode they set; the bar itself is ADR-234's drawing.

// appMode is uxtheme's PreferredAppMode.
type appMode uint32

const (
	appModeDefault    appMode = 0 // the system's own: light menus
	appModeAllowDark  appMode = 1 // follow the system's mode
	appModeForceDark  appMode = 2
	appModeForceLight appMode = 3
)

// minMenuThemeBuild is the first Windows build with SetPreferredAppMode:
// Windows 10 1903. On 1809 the same ordinal is AllowDarkModeForApp(bool),
// which can only follow the system, so menus stay light there.
const minMenuThemeBuild = 18362

// menuThemeSupported reports whether a Windows build has the uxtheme
// exports.
func menuThemeSupported(build uint32) bool { return build >= minMenuThemeBuild }

// menuAppMode is the mode that makes popup menus follow the settings the
// backend tracks: dark when the system's app mode is dark, light when it is
// light, and the system's default under high contrast, whose colors the
// user chose. It forces the mode instead of using AllowDark so that the
// menus follow the backend's own reading of the settings (the same one the
// bar is drawn from), not a second one made by uxtheme.
func menuAppMode(darkMode, highContrast bool) appMode {
	switch {
	case highContrast:
		return appModeDefault
	case darkMode:
		return appModeForceDark
	}
	return appModeForceLight
}
