package telatest

import (
	"image/color"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform"
)

// System settings at run time. The options [Dark], [ReducedMotion],
// [HighContrast], [TextScale] and [AccentColor] set what the system says
// when the app is mounted; these methods change it while the app runs, as
// the user does in the system's settings: the headless backend delivers a
// SettingsEvent, the tree restyles, and the components that read the
// setting (tela.UseColorScheme, UseReducedMotion, UseHighContrast,
// UseTextScale, UseAccentColor) rebuild. Each settles the app.
//
//	app := telatest.Mount(t, Panel())
//	app.SetDarkMode(true)
//	app.Golden("panel_dark.png")

// updateSettings changes the system settings of the headless backend and
// delivers them.
func (a *App) updateSettings(change func(s *platform.SystemSettings)) {
	a.tb.Helper()
	a.mustBeOpen()
	s := a.backend.Settings()
	change(&s)
	a.backend.SetSettings(s)
	a.settleAfter()
}

// SetDarkMode changes whether the system prefers the dark color scheme:
// the dark: variants apply and tela.UseColorScheme reports it, unless the
// app sets its own scheme (tela.App.ColorScheme, tela.SetColorScheme,
// [ColorScheme]).
func (a *App) SetDarkMode(dark bool) {
	a.tb.Helper()
	a.updateSettings(func(s *platform.SystemSettings) { s.DarkMode = dark })
}

// SetReducedMotion changes whether the system asks for reduced motion:
// the motion-reduce: variants apply, and transitions that start from now
// on take no time (tela.UseReducedMotion).
func (a *App) SetReducedMotion(reduced bool) {
	a.tb.Helper()
	a.updateSettings(func(s *platform.SystemSettings) { s.ReducedMotion = reduced })
}

// SetHighContrast changes whether the system asks for high contrast: the
// contrast-more: variants apply and tela.UseHighContrast reports it.
func (a *App) SetHighContrast(high bool) {
	a.tb.Helper()
	a.updateSettings(func(s *platform.SystemSettings) { s.HighContrast = high })
}

// SetTextScale changes the system's text scale: 1rem becomes the theme's
// rem times scale, so type, rem-based spacing and breakpoints change
// (tela.UseTextScale), unless the app ignores it (tela.App.IgnoreTextScale).
// scale is above 0 and at most 4; 1 is the default.
func (a *App) SetTextScale(scale float64) {
	a.tb.Helper()
	if !(scale > 0 && scale <= platform.MaxTextScale) {
		a.fatalf("SetTextScale(%v): the text scale is above 0 and at most %d; 1 is the default", scale, platform.MaxTextScale)
	}
	a.updateSettings(func(s *platform.SystemSettings) { s.TextScale = scale })
}

// SetAccentColor changes the system's accent color (tela.UseAccentColor
// returns it). Use [App.ClearAccentColor] for a system without one.
func (a *App) SetAccentColor(color tela.Color) {
	a.tb.Helper()
	a.updateSettings(func(s *platform.SystemSettings) {
		s.AccentColor, s.HasAccentColor = color.Clamp().NRGBA8(), true
	})
}

// ClearAccentColor makes the system report no accent color, as a desktop
// without one does.
func (a *App) ClearAccentColor() {
	a.tb.Helper()
	a.updateSettings(func(s *platform.SystemSettings) { s.AccentColor, s.HasAccentColor = color.NRGBA{}, false })
}
