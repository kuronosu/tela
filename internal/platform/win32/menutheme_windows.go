//go:build windows

package win32

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// Dark popup menus (ADR-239; menutheme.go has the why). uxtheme.dll is
// loaded the first time a mode is applied, and only on a build that has the
// exports. The functions are looked up by ordinal: Explorer's own calls are
// unnamed.
const (
	ordRefreshImmersiveColorPolicyState = 104
	ordSetPreferredAppMode              = 135
	ordFlushMenuThemes                  = 136
)

// uxthemeProcs are the addresses of the uxtheme exports, zero when missing.
type uxthemeProcs struct {
	setPreferredAppMode, refreshPolicy, flushMenuThemes uintptr
}

var menuThemeProcs uxthemeProcs

// loadMenuTheme resolves the exports, once. All three are needed; with any
// missing the menus stay light.
func (b *Backend) loadMenuTheme() {
	if b.menuThemeLoaded {
		return
	}
	b.menuThemeLoaded = true
	if !menuThemeSupported(windowsBuild()) {
		return
	}
	dll := windows.NewLazySystemDLL("uxtheme.dll")
	if dll.Load() != nil {
		return
	}
	h := windows.Handle(dll.Handle())
	var p uxthemeProcs
	var e1, e2, e3 error
	p.setPreferredAppMode, e1 = windows.GetProcAddressByOrdinal(h, ordSetPreferredAppMode)
	p.refreshPolicy, e2 = windows.GetProcAddressByOrdinal(h, ordRefreshImmersiveColorPolicyState)
	p.flushMenuThemes, e3 = windows.GetProcAddressByOrdinal(h, ordFlushMenuThemes)
	if e1 != nil || e2 != nil || e3 != nil {
		return
	}
	menuThemeProcs = p
}

// applyMenuTheme makes the popup menus (the bar's drop-downs, context menus
// and tray menus) follow the settings. It does nothing where uxtheme lacks
// the exports, and when the mode is the one already set.
func (b *Backend) applyMenuTheme() {
	b.loadMenuTheme()
	p := menuThemeProcs
	if p.setPreferredAppMode == 0 {
		return
	}
	mode := menuAppMode(b.settings.DarkMode, b.settings.HighContrast)
	if b.menuThemeSet && mode == b.menuThemeMode {
		return
	}
	b.menuThemeSet, b.menuThemeMode = true, mode
	syscall.SyscallN(p.setPreferredAppMode, uintptr(mode))
	syscall.SyscallN(p.refreshPolicy)
	syscall.SyscallN(p.flushMenuThemes)
}
