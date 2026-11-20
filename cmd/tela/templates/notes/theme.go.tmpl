package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/kit"
)

// accent is a color the user can choose in Preferences. The theme has
// three tokens of it: primary (the fill of the New note button and the
// switches, with dark text on it in both schemes), gold (the same hue as
// text and icons: darker in the light scheme so that it reads on stone) and
// ring (the focus ring).
type accent struct {
	Name, Label string
	// Fill is primary in both schemes.
	Fill string
	// GoldLight and GoldDark are the text color of the accent in the
	// light and the dark scheme.
	GoldLight, GoldDark string
	// RingLight is the focus ring in the light scheme; the dark scheme's
	// is the fill.
	RingLight string
}

// accents are the swatches of Preferences; the first is the default.
var accents = []accent{
	{"amber", "Amber", "#efb02e", "#a4600a", "#f0b13a", "#d99a1d"},
	{"green", "Green", "#5fb878", "#2f7d4a", "#6fcf8b", "#3f9d5f"},
	{"blue", "Blue", "#5b9dff", "#1f5fc7", "#7db2ff", "#3b7fe6"},
	{"rose", "Rose", "#f0708f", "#c0305a", "#f58aa5", "#e0486f"},
	{"stone", "Stone", "#a8a29e", "#57534d", "#d6d3d1", "#79716b"},
}

// accentNamed returns the accent with the name, or the default one.
func accentNamed(name string) accent {
	for _, a := range accents {
		if a.Name == name {
			return a
		}
	}
	return accents[0]
}

// themeFor is the theme of the app with an accent: the kit's, with the
// stone palette of the Tela examples in the light scheme and graphite in
// the dark one. "sidebar" (the title bar and the rail) and "gold" are
// tokens of its own, so classes like bg-sidebar and text-gold follow the
// scheme and the accent.
func themeFor(name string) tela.Theme {
	a := accentNamed(name)
	t := kit.Theme()
	t.Radius = 10
	set := func(token, light, dark string) {
		t.Colors[token] = tela.SchemeColor{Light: tela.Hex(light), Dark: tela.Hex(dark)}
	}
	// Light is stone (Tailwind: 50 fafaf9, 100 f5f5f4, 200 e7e5e4, 300
	// d6d3d1, 500 79716b, 800 292524): the frame is stone-200, the notes
	// pane stone-100 and the cards stone-50 with a stone-300 border.
	set("background", "#f5f5f4", "#131314")
	// The tokens of the app's own are assigned with constant keys, which is
	// how tela lint learns that bg-sidebar and text-gold exist.
	t.Colors["sidebar"] = tela.SchemeColor{Light: tela.Hex("#e7e5e4"), Dark: tela.Hex("#0d0d0e")}
	t.Colors["gold"] = tela.SchemeColor{Light: tela.Hex(a.GoldLight), Dark: tela.Hex(a.GoldDark)}
	set("foreground", "#292524", "#ececee")
	set("card", "#fafaf9", "#1a1a1c")
	set("card-foreground", "#292524", "#ececee")
	set("popover", "#fafaf9", "#1a1a1c")
	set("popover-foreground", "#292524", "#ececee")
	set("primary", a.Fill, a.Fill)
	set("primary-foreground", "#1b1405", "#1b1405")
	set("secondary", "#e7e5e4", "#232326")
	set("secondary-foreground", "#292524", "#ececee")
	set("muted", "#e7e5e4", "#1f1f22")
	set("muted-foreground", "#79716b", "#8d8d93")
	set("accent", "#d6d3d1", "#242427")
	set("accent-foreground", "#292524", "#ececee")
	set("border", "#d6d3d1", "#2a2a2d")
	set("input", "#c4c0bc", "#3a3a3f")
	set("ring", a.RingLight, a.Fill)
	return t
}

// scheme is the color scheme of the setting "theme": "light", "dark" or
// "system" (anything else).
func scheme(theme string) tela.ColorScheme {
	switch theme {
	case "light":
		return tela.Light
	case "dark":
		return tela.Dark
	}
	return tela.System
}

// withAppearance returns app with the theme and the accent of the
// settings, so that the first frame already has them. useAppearance keeps
// them following the settings afterwards. Call it after desktop.PersistApp,
// which names the app's folders.
func withAppearance(app tela.App) tela.App {
	var themeName, accentName string
	s := desktop.Settings()
	s.Get(keyTheme, &themeName)
	s.Get(keyAccent, &accentName)
	t := themeFor(accentName)
	app.Theme = &t
	app.ColorScheme = scheme(themeName)
	return app
}

// appearance is the theme and the accent the app is showing.
type appearance struct{ theme, accent string }

// useAppearance applies the theme and the accent of the settings to the
// whole app (every window) when they change. The app starts with the
// defaults (App), so nothing is applied until a setting differs from them:
// a color scheme asked for by the host (tela snapshot --dark) is left alone
// while the setting is "system". When the settings have other values at
// the start, withAppearance already applied them and this applies them
// once more, which changes nothing.
func useAppearance(p Prefs) {
	shown := tela.UseOnce(func() *appearance { return &appearance{theme: "system", accent: accents[0].Name} })
	tela.UseEffectDo(func() {
		if p.Theme != shown.theme {
			shown.theme = p.Theme
			tela.SetColorScheme(scheme(p.Theme))
		}
		if p.Accent != shown.accent {
			shown.accent = p.Accent
			tela.SetTheme(themeFor(p.Accent))
		}
	}, p.Theme, p.Accent)
}
