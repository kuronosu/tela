package core

import (
	"fmt"
	"image/color"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// The environment (ARCHITECTURE section 10, environment variants; ADR-077,
// ADR-209): the color scheme, reduced motion, high contrast, the text
// scale and the accent color.
//
// The system's preferences come from the platform (the Config fields a
// Host fills from platform.SystemSettings, then SetSystemSettings on every
// SettingsEvent). The app may override the color scheme
// (Config.ColorScheme, SetColorScheme, and the process-wide SetColorScheme
// for the running app): the effective scheme is the override, or the
// system's preference with System. The dark: variant follows the
// effective scheme, motion-reduce: the system's reduced motion,
// contrast-more: its high contrast. A change restyles every element and
// rebuilds the components that read the environment with UseColorScheme,
// UseReducedMotion, UseHighContrast, UseTextScale or UseAccentColor (each
// reads a Value owned by the Tree, so only they rebuild). A Scheme prop
// overrides the scheme for a subtree (theme.go).
//
// The text scale works like a browser's default font size: 1rem is the
// theme's rem times the system's scale (theme.go: the tree's parser is
// the theme's with a scaled root font size), so text and the rem-based
// spacing of Tailwind's scales grow together while px values stay. It is
// on unless Config.IgnoreTextScale; the headless backend, snapshots and
// tests use 1. A change compiles the classes again and restyles and lays
// out everything, like a theme change.

// ColorScheme is a color scheme: the app's choice, or the effective scheme
// UseColorScheme returns (Light or Dark).
type ColorScheme uint8

const (
	// ColorSchemeSystem follows the system's preference (the zero
	// value).
	ColorSchemeSystem ColorScheme = iota
	// ColorSchemeLight is the light scheme: dark: variants do not apply.
	ColorSchemeLight
	// ColorSchemeDark is the dark scheme: dark: variants apply.
	ColorSchemeDark
)

func (s ColorScheme) String() string {
	switch s {
	case ColorSchemeSystem:
		return "System"
	case ColorSchemeLight:
		return "Light"
	case ColorSchemeDark:
		return "Dark"
	}
	return fmt.Sprintf("ColorScheme(%d)", uint8(s))
}

func checkScheme(s ColorScheme) {
	if s > ColorSchemeDark {
		panic(fmt.Sprintf("tela: SetColorScheme(%v): pass tela.Light, tela.Dark or tela.System", s))
	}
}

// envState is the environment of a Tree (UI thread).
type envState struct {
	// sysDark, sysReduced and sysContrast are the system's preferences;
	// scheme the app's override.
	sysDark, sysReduced, sysContrast bool
	scheme                           ColorScheme
	// sysTextScale is the system's text scale (1 when unknown), and
	// ignoreText the app's choice to ignore it (Config.IgnoreTextScale).
	sysTextScale float64
	ignoreText   bool
	// accent is the system's accent color.
	accent accentColor
	// dark, reduced, contrast, textScale and accentV hold the effective
	// values for the components that read them (owned by the root
	// element; nil before Mount).
	dark, reduced, contrast *Value[bool]
	textScale               *Value[float64]
	accentV                 *Value[accentColor]
	// theme holds the parser's theme for UseTheme (theme.go).
	theme *Value[*style.Theme]
}

// accentColor is the system's accent color, when it has one.
type accentColor struct {
	c  gfx.Color
	ok bool
}

// accentOf returns the accent color of platform settings.
func accentOf(has bool, c color.NRGBA) accentColor {
	if !has {
		return accentColor{}
	}
	return accentColor{c: gfx.RGBA8(c.R, c.G, c.B, c.A), ok: true}
}

// textScaleFactor returns the text scale factor of a setting (0 and
// invalid values are 1: platform.SystemSettings.TextScaleFactor).
func textScaleFactor(scale float64) float64 {
	return platform.SystemSettings{TextScale: scale}.TextScaleFactor()
}

// effectiveDark reports whether the effective scheme is dark.
func (e *envState) effectiveDark() bool {
	return e.scheme == ColorSchemeDark || e.scheme == ColorSchemeSystem && e.sysDark
}

// effectiveTextScale returns the scale of rem: the system's, or 1 when
// the app ignores it.
func (e *envState) effectiveTextScale() float64 {
	if e.ignoreText {
		return 1
	}
	return e.sysTextScale
}

// initEnv creates the environment Values, owned by the root element.
func (t *Tree) initEnv() {
	t.env.dark = newValue(t.root, t.env.effectiveDark())
	t.env.reduced = newValue(t.root, t.env.sysReduced)
	t.env.contrast = newValue(t.root, t.env.sysContrast)
	t.env.textScale = newValue(t.root, t.textScale)
	t.env.accentV = newValue(t.root, t.env.accent)
	t.env.theme = newValue(t.root, t.parser.Theme())
}

// applyEnv brings the style context, the parser and the environment
// Values to the current environment, restyling everything when it
// changed.
func (t *Tree) applyEnv() {
	dark, reduced, contrast := t.env.effectiveDark(), t.env.sysReduced, t.env.sysContrast
	if s := t.env.effectiveTextScale(); s != t.textScale {
		t.textScale = s
		t.useParser(scaledParser(t.parserBase, s))
	}
	if t.env.dark != nil {
		// On the UI thread, at once: the readers rebuild in the next
		// frame, with the new styles.
		t.env.dark.storeUI(dark)
		t.env.reduced.storeUI(reduced)
		t.env.contrast.storeUI(contrast)
		t.env.textScale.storeUI(t.textScale)
		t.env.accentV.storeUI(t.env.accent)
	}
	if dark == t.cx.Dark && reduced == t.cx.ReducedMotion && contrast == t.cx.HighContrast {
		return
	}
	t.cx.Dark, t.cx.ReducedMotion, t.cx.HighContrast = dark, reduced, contrast
	t.restyleAll()
}

// SetEnvironment sets the system's preferences: dark mode (the dark:
// variants, unless the app overrides the color scheme) and reduced motion
// (motion-reduce:). SetSystemSettings sets them all.
func (t *Tree) SetEnvironment(dark, reducedMotion bool) {
	t.env.sysDark, t.env.sysReduced = dark, reducedMotion
	t.applyEnv()
}

// SetSystemSettings sets the system's preferences: dark mode, reduced
// motion, the double-click time, high contrast (contrast-more:), the text
// scale (rem, unless Config.IgnoreTextScale) and the accent color. A Host
// calls it on every SettingsEvent.
func (t *Tree) SetSystemSettings(s platform.SystemSettings) {
	t.SetDoubleClickInterval(s.DoubleClickInterval)
	e := &t.env
	e.sysDark, e.sysReduced, e.sysContrast = s.DarkMode, s.ReducedMotion, s.HighContrast
	e.sysTextScale = s.TextScaleFactor()
	e.accent = accentOf(s.HasAccentColor, s.AccentColor)
	t.applyEnv()
}

// setSystem takes the system's settings into cfg (a Host's initial
// ones).
func (cfg *Config) setSystem(s platform.SystemSettings) {
	cfg.Dark, cfg.ReducedMotion = s.DarkMode, s.ReducedMotion
	cfg.DoubleClickInterval = s.DoubleClickInterval
	cfg.HighContrast, cfg.TextScale = s.HighContrast, s.TextScale
	a := accentOf(s.HasAccentColor, s.AccentColor)
	cfg.AccentColor, cfg.HasAccentColor = a.c, a.ok
}

// SetDoubleClickInterval sets the system's double-click time for mice and
// pens (zero: platform.DefaultDoubleClickInterval), the limit of
// MouseEvent.ClickCount and OnDoubleClick (ADR-192). A Host calls it with
// the platform's settings. Touch keeps its own limits.
func (t *Tree) SetDoubleClickInterval(d time.Duration) { t.in.clicks.Interval = d }

// SetColorScheme overrides the system's color scheme (ColorSchemeLight or
// ColorSchemeDark), or follows it again (ColorSchemeSystem). Call it on
// the UI thread.
func (t *Tree) SetColorScheme(s ColorScheme) {
	checkScheme(s)
	if devStateEnabled {
		t.devNoteScheme(s)
	}
	t.setScheme(s)
}

// setScheme applies the app's color scheme s to the tree and to the trees
// of its child windows (window.go): every window follows the app's.
func (t *Tree) setScheme(s ColorScheme) {
	t.env.scheme = s
	t.applyEnv()
	for _, w := range t.wins {
		if v := w.view; v != nil {
			v.tree.setScheme(s)
		}
	}
}

// ColorScheme returns the effective color scheme: ColorSchemeLight or
// ColorSchemeDark.
func (t *Tree) ColorScheme() ColorScheme {
	if t.env.effectiveDark() {
		return ColorSchemeDark
	}
	return ColorSchemeLight
}

// ReducedMotion reports the system's reduced-motion preference.
func (t *Tree) ReducedMotion() bool { return t.env.sysReduced }

// HighContrast reports the system's high-contrast preference.
func (t *Tree) HighContrast() bool { return t.env.sysContrast }

// TextScale returns the scale of rem: the system's text scale, or 1 when
// the app ignores it (Config.IgnoreTextScale).
func (t *Tree) TextScale() float64 { return t.textScale }

// SetColorScheme sets the color scheme of the running app (the Tree that
// receives Post): like Tree.SetColorScheme, safe from any goroutine. Before
// the app runs, it is applied when it starts.
func SetColorScheme(s ColorScheme) {
	checkScheme(s)
	postUpdate(update{postTree: func(t *Tree) { t.SetColorScheme(s) }})
}

// envSlot takes the hook slot of UseColorScheme or UseReducedMotion.
func envSlot(kind hookKind) *Element {
	el, i := nextSlot(kind)
	if i == len(el.hooks) {
		el.hooks = append(el.hooks, nil)
	}
	return el
}

// UseColorScheme returns the color scheme at the component's position
// (ColorSchemeLight or ColorSchemeDark): the Scheme prop of its nearest
// primitive ancestor that has one, else the effective scheme, the app's
// choice or the system's preference (theme.go). The component builds
// again when it changes.
func UseColorScheme() ColorScheme {
	el := schemeHookSlot(hookColorScheme)
	if readScheme(el) {
		return ColorSchemeDark
	}
	return ColorSchemeLight
}

// UseReducedMotion reports whether the system asks for reduced motion.
// The component builds again when it changes.
func UseReducedMotion() bool {
	el := envSlot(hookReducedMotion)
	return el.tree.env.reduced.Get()
}

// envHook is the slot of UseHighContrast, UseTextScale and UseAccentColor:
// their kind, for the hook signatures of dev-loop state (devSlotKind).
type envHook hookKind

// envKindSlot takes the hook slot of an environment hook of kind.
func envKindSlot(kind hookKind) *Element {
	el, i := nextSlot(kind)
	if i == len(el.hooks) {
		el.hooks = append(el.hooks, envHook(kind))
	}
	return el
}

// UseHighContrast reports whether the system asks for high contrast (the
// contrast-more: variants apply). The component builds again when it
// changes.
func UseHighContrast() bool {
	el := envKindSlot(hookHighContrast)
	return el.tree.env.contrast.Get()
}

// UseTextScale returns the scale of rem: the system's text scale (1.25
// when the user asks for text 25% larger), or 1 when the app ignores it
// (Config.IgnoreTextScale) or the system reports none. The component
// builds again when it changes.
func UseTextScale() float64 {
	el := envKindSlot(hookTextScale)
	return el.tree.env.textScale.Get()
}

// UseAccentColor returns the system's accent color, and whether the
// system has one. The component builds again when it changes.
func UseAccentColor() (gfx.Color, bool) {
	el := envKindSlot(hookAccentColor)
	a := el.tree.env.accentV.Get()
	return a.c, a.ok
}
