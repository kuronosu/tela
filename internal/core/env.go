package core

import (
	"fmt"

	"github.com/kuronosu/tela/internal/style"
)

// The environment (ARCHITECTURE section 10, environment variants; ADR-077):
// the color scheme and reduced motion.
//
// The system's preferences come from the platform (Config.Dark and
// Config.ReducedMotion, then SetEnvironment on every SettingsEvent). The
// app may override the color scheme (Config.ColorScheme, SetColorScheme,
// and the process-wide SetColorScheme for the running app): the effective
// scheme is the override, or the system's preference with System. The
// dark: variant follows the effective scheme, motion-reduce: the system's
// reduced motion. A change restyles every element and rebuilds the
// components that read the environment with UseColorScheme or
// UseReducedMotion (each reads a Value owned by the Tree, so only they
// rebuild). A Scheme prop overrides the scheme for a subtree (theme.go).

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
	// sysDark and sysReduced are the system's preferences; scheme the
	// app's override.
	sysDark, sysReduced bool
	scheme              ColorScheme
	// dark and reduced hold the effective values for the components
	// that read them (owned by the root element; nil before Mount).
	dark, reduced *Value[bool]
	// theme holds the parser's theme for UseTheme (theme.go).
	theme *Value[*style.Theme]
}

// effectiveDark reports whether the effective scheme is dark.
func (e *envState) effectiveDark() bool {
	return e.scheme == ColorSchemeDark || e.scheme == ColorSchemeSystem && e.sysDark
}

// initEnv creates the environment Values, owned by the root element.
func (t *Tree) initEnv() {
	t.env.dark = newValue(t.root, t.env.effectiveDark())
	t.env.reduced = newValue(t.root, t.env.sysReduced)
	t.env.theme = newValue(t.root, t.parser.Theme())
}

// applyEnv brings the style context and the environment Values to the
// current environment, restyling everything when it changed.
func (t *Tree) applyEnv() {
	dark, reduced := t.env.effectiveDark(), t.env.sysReduced
	if t.env.dark != nil {
		// On the UI thread, at once: the readers rebuild in the next
		// frame, with the new styles.
		t.env.dark.storeUI(dark)
		t.env.reduced.storeUI(reduced)
	}
	if dark == t.cx.Dark && reduced == t.cx.ReducedMotion {
		return
	}
	t.cx.Dark, t.cx.ReducedMotion = dark, reduced
	t.restyleAll()
}

// SetEnvironment sets the system's preferences: dark mode (the dark:
// variants, unless the app overrides the color scheme) and reduced motion
// (motion-reduce:). A Host calls it with the platform's settings.
func (t *Tree) SetEnvironment(dark, reducedMotion bool) {
	t.env.sysDark, t.env.sysReduced = dark, reducedMotion
	t.applyEnv()
}

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
