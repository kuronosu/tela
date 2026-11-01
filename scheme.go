package tela

import "github.com/kuronosu/tela/internal/core"

// ColorScheme is a color scheme: what the app asks for ([System], [Light]
// or [Dark]) in [App].ColorScheme and [SetColorScheme], and the effective
// scheme [UseColorScheme] returns ([Light] or [Dark]). The dark: classes
// apply in the effective [Dark] scheme.
type ColorScheme = core.ColorScheme

const (
	// System follows the system's light or dark preference. It is the
	// zero value, so an [App] follows the system unless it sets
	// ColorScheme.
	System ColorScheme = core.ColorSchemeSystem
	// Light is the light scheme: dark: classes do not apply.
	Light ColorScheme = core.ColorSchemeLight
	// Dark is the dark scheme: dark: classes apply.
	Dark ColorScheme = core.ColorSchemeDark
)

// UseColorScheme returns the color scheme at the component's position,
// [Light] or [Dark]: the one the nearest element above it with an
// h.Scheme prop gives, else the app's, the one the app set
// ([App].ColorScheme, [SetColorScheme]) or, with [System], the system's
// preference. The component builds again when it changes, like with a
// state hook. It is a hook: call it at the top level of a component.
//
//	func Logo() tela.Node {
//	    src := lightLogo
//	    if tela.UseColorScheme() == tela.Dark {
//	        src = darkLogo
//	    }
//	    return Img(SrcImage(src), Alt("Tela"))
//	}
//
// Classes do not need it: dark:bg-slate-900 already follows the scheme.
func UseColorScheme() ColorScheme { return core.UseColorScheme() }

// UseReducedMotion reports whether the system asks for reduced motion
// (the motion-reduce: classes apply, transitions take no time and
// scrolling jumps). The component builds again when it changes. It is a
// hook: call it at the top level of a component.
func UseReducedMotion() bool { return core.UseReducedMotion() }

// UseHighContrast reports whether the system asks for high contrast
// (Windows' contrast themes, GNOME's high contrast): the contrast-more:
// classes apply. The component builds again when it changes. It is a
// hook: call it at the top level of a component.
//
// Classes do not need it: border-border contrast-more:border-foreground
// already follows the setting.
func UseHighContrast() bool { return core.UseHighContrast() }

// UseTextScale returns how much the user asked the system to scale text
// (Windows' "Text size", GNOME's text scaling factor): 1.25 for text 25%
// larger. Tela applies it like a browser's default font size: 1rem is
// the theme's rem (16 px) times the scale, so the text-* sizes and the
// rem-based spacing of Tailwind (p-4, gap-2, w-64, the breakpoints) grow
// together while px values (border, text-[13px], w-[320px]) do not. It is
// 1 when the system reports nothing, when [App].IgnoreTextScale is set,
// and in tests and snapshots. The component builds again when it changes.
// It is a hook: call it at the top level of a component.
//
// Classes do not need it; it is for sizes computed in code.
func UseTextScale() float64 { return core.UseTextScale() }

// UseAccentColor returns the user's accent color (the color the system
// uses for selections and focused controls), and false when the system
// has none or does not report it. The component builds again when it
// changes. It is a hook: call it at the top level of a component.
//
//	accent, ok := tela.UseAccentColor()
//	if !ok {
//	    accent = tela.Hex("#4f46e5")
//	}
//	return Div(Style(tela.Bg(accent)), ...)
func UseAccentColor() (Color, bool) { return core.UseAccentColor() }

// SetColorScheme sets the color scheme of the running app: [Light] or
// [Dark] override the system's preference, [System] follows it again.
// Every element is styled again (dark: classes) and the components that
// call [UseColorScheme] build again. It is safe from any goroutine, like
// a state setter: the change applies on the UI thread before the next
// frame. Called before [Run], it applies when the window opens (it wins
// over [App].ColorScheme). Like [Post], it goes to the app that receives
// posted functions (telatest.Mount's app in tests).
//
//	Button(OnClick(func(tela.MouseEvent) { tela.SetColorScheme(tela.Dark) }), Text("Dark"))
//
// It panics for a value other than System, Light and Dark.
func SetColorScheme(s ColorScheme) { core.SetColorScheme(s) }
