package core

import (
	"fmt"
	"maps"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Theme tokens and scoped color schemes (ARCHITECTURE section 10, "Theme
// and tokens"; ADR-089).
//
// A Theme is turned into the style.Theme of the tree's Parser: its colors
// become style tokens (merged over the default ones, so every shadcn/ui
// token always resolves), its radius the radius scale and its fonts the
// sans and mono families. Classes keep tokens symbolic and Compute
// resolves them for the element's scheme, so a scheme change (system,
// SetColorScheme, or a Scheme prop) restyles without compiling anything.
//
// The Scheme prop gives a primitive and its subtree a color scheme of
// their own: style inherits it through ComputedStyle.Scheme, and the
// components below read it with UseColorScheme and UseTheme, which find
// the nearest primitive ancestor with the prop. When the prop of a
// primitive changes, the components below it that read the scheme build
// again (markSchemeReaders); without a scoped ancestor they read the
// tree's Value, which the environment updates.

// SchemeColor is the value of a theme color token in the light and the
// dark scheme.
type SchemeColor = style.SchemeColor

// Theme holds the tokens of an app (tela.Theme has the same fields and
// converts to it).
type Theme struct {
	// Colors maps token names to their light and dark values. Missing
	// tokens keep the default theme's values.
	Colors map[string]SchemeColor
	// Radius is the base radius in logical pixels (rounded-lg); 0 keeps
	// Tailwind's radius scale.
	Radius float32
	// FontSans and FontMono are the families of font-sans (and of the
	// root text) and of font-mono; "" keeps the defaults.
	FontSans, FontMono string
}

// DefaultTheme returns the default theme: shadcn/ui's neutral colors,
// Tailwind's radius scale and the embedded fonts.
func DefaultTheme() Theme {
	return Theme{Colors: maps.Clone(style.DefaultTheme().Tokens)}
}

// StyleTheme returns the style theme of th: the default theme with th's
// colors merged over its tokens, its radius scale and its fonts. It
// returns an error for token names that classes cannot use.
func StyleTheme(th *Theme) (*style.Theme, error) {
	s := style.DefaultTheme()
	if th == nil {
		return s, nil
	}
	for k, v := range th.Colors {
		s.Tokens[k] = v
	}
	if err := s.CheckTokens(); err != nil {
		return nil, err
	}
	s.SetRadius(th.Radius)
	if th.FontSans != "" {
		s.Fonts["sans"] = append([]string{th.FontSans}, s.Fonts["sans"]...)
	}
	if th.FontMono != "" {
		s.Fonts["mono"] = append([]string{th.FontMono}, s.Fonts["mono"]...)
	}
	return s, nil
}

// NewParser returns the class parser of a theme and class aliases, or the
// shared default parser when both are empty. It panics with the reason
// when a token name or an alias name cannot be used in classes: they are
// the app's literal configuration, so this is a programmer error.
func NewParser(th *Theme, aliases map[string]string) *style.Parser {
	if th == nil && len(aliases) == 0 {
		return style.DefaultParser()
	}
	st, err := StyleTheme(th)
	if err == nil {
		var p *style.Parser
		if p, err = style.NewParserAliases(st, aliases); err == nil {
			return p
		}
	}
	panic("tela: " + err.Error())
}

// themeParser returns the parser of cfg: its Parser, or one for its Theme
// and ClassAliases.
func themeParser(cfg *Config) *style.Parser {
	if cfg.Parser != nil {
		return cfg.Parser
	}
	return NewParser(cfg.Theme, cfg.ClassAliases)
}

// SetTheme replaces the theme of the tree: classes are compiled again
// against it (keeping the class aliases), every element is styled again
// and the components that call UseTheme build again. Call it on the UI
// thread. It panics like NewParser for token names classes cannot use.
func (t *Tree) SetTheme(th Theme) {
	t.setParser(NewParser(&th, t.parser.Aliases()))
}

// setParser makes p the tree's parser.
func (t *Tree) setParser(p *style.Parser) {
	t.parser = p
	t.cx.Theme = p.Theme()
	t.reported = map[string]bool{} // report the class errors of the new theme
	if t.env.theme != nil {
		t.env.theme.storeUI(p.Theme())
	}
	if t.root == nil {
		return
	}
	var walk func(el *Element)
	walk = func(el *Element) {
		if el.kind == elemPrimitive {
			el.box.classesDirty = true
			t.markStyle(el)
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(t.root)
	t.dirty()
}

// SetTheme sets the theme of the running app (the Tree that receives
// Post), like Tree.SetTheme: safe from any goroutine. Before the app runs,
// it is applied when it starts.
func SetTheme(th Theme) {
	th.Colors = maps.Clone(th.Colors) // the caller may reuse its map
	if _, err := StyleTheme(&th); err != nil {
		panic("tela: SetTheme: " + err.Error())
	}
	postUpdate(update{postTree: func(t *Tree) { t.SetTheme(th) }})
}

// ---------------------------------------------------------------------
// The Scheme prop.

// SchemeProp gives a primitive and its subtree the color scheme s (Light
// or Dark): their dark: classes and theme tokens follow it, and so do
// UseColorScheme and UseTheme in the components below. System removes the
// override: the subtree follows its parent.
func SchemeProp(s ColorScheme) Prop {
	if s > ColorSchemeDark {
		panic(fmt.Sprintf("h: Scheme(%v): pass tela.Light or tela.Dark (or tela.System to follow the parent)", s))
	}
	return Prop{name: "Scheme", apply: func(d *desc) { d.scheme = s }}
}

// styleScheme converts the Scheme prop of a primitive for style.Input.
func styleScheme(s ColorScheme) style.Scheme {
	switch s {
	case ColorSchemeLight:
		return style.SchemeLight
	case ColorSchemeDark:
		return style.SchemeDark
	}
	return style.SchemeInherit
}

// schemeSlot marks the hook slots that read the color scheme at their
// position (UseColorScheme, UseTheme), so that a Scheme prop change above
// them builds their component again.
type schemeSlot struct{}

// schemeAt returns the color scheme at component el's position: the
// Scheme prop of its nearest primitive ancestor that has one, and whether
// one does; without one, the tree's effective scheme.
func (t *Tree) schemeAt(el *Element) (dark, scoped bool) {
	for p := el.parent; p != nil; p = p.parent {
		if p.kind == elemPrimitive && p.node.d != nil {
			switch p.node.d.scheme {
			case ColorSchemeLight:
				return false, true
			case ColorSchemeDark:
				return true, true
			}
		}
	}
	return t.env.effectiveDark(), false
}

// readScheme returns the scheme at the position of el, the component being
// built, and subscribes its build to the tree's scheme when no ancestor
// scopes it.
func readScheme(el *Element) bool {
	dark, scoped := el.tree.schemeAt(el)
	if !scoped {
		dark = el.tree.env.dark.Get()
	}
	return dark
}

// markSchemeReaders builds again the components below el whose hooks read
// the color scheme at their position, after el's Scheme prop changed.
func (t *Tree) markSchemeReaders(el *Element) {
	var walk func(e *Element)
	walk = func(e *Element) {
		for _, c := range e.children {
			if c.kind == elemComponent {
				for _, h := range c.hooks {
					if _, ok := h.(*schemeSlot); ok {
						t.markBuild(c)
						break
					}
				}
			}
			walk(c)
		}
	}
	walk(el)
}

// schemeHookSlot takes the hook slot of UseColorScheme or UseTheme.
func schemeHookSlot(kind hookKind) *Element {
	el, i := nextSlot(kind)
	if i == len(el.hooks) {
		el.hooks = append(el.hooks, &schemeSlot{})
	}
	return el
}

// ---------------------------------------------------------------------
// UseTheme.

// ThemeValues are the theme's values at a component's position: the
// tokens resolved for the color scheme there.
type ThemeValues struct {
	th   *style.Theme
	dark bool
	dev  bool
}

// UseTheme returns the theme's values for the color scheme at the
// component's position (the nearest Scheme prop above it, else the app's
// scheme). The component builds again when that scheme or the theme
// changes.
func UseTheme() ThemeValues {
	el := schemeHookSlot(hookTheme)
	dark := readScheme(el)
	th := el.tree.env.theme.Get()
	return ThemeValues{th: th, dark: dark, dev: el.tree.dev}
}

// Dark reports whether the values are the dark scheme's.
func (v ThemeValues) Dark() bool { return v.dark }

func (v ThemeValues) theme() *style.Theme {
	if v.th == nil {
		return style.DefaultParser().Theme()
	}
	return v.th
}

// Lookup returns the value of a color token, or of a palette color
// ("red-500", "white"), and whether the theme has it.
func (v ThemeValues) Lookup(name string) (gfx.Color, bool) {
	th := v.theme()
	if c, ok := th.TokenColor(name, v.dark); ok {
		return c, true
	}
	switch name {
	case "transparent":
		return gfx.Transparent, true
	}
	c, ok := th.Colors[name]
	return c, ok
}

// Color returns the value of a color token, or of a palette color. An
// unknown name is transparent; in dev mode it panics with the closest
// token.
func (v ThemeValues) Color(name string) gfx.Color {
	c, ok := v.Lookup(name)
	if !ok && v.dev {
		msg := fmt.Sprintf("tela: ThemeValues.Color(%q): the theme has no color %q", name, name)
		if s := v.theme().SuggestToken(name); s != "" {
			msg += fmt.Sprintf("; did you mean %q?", s)
		}
		panic(msg)
	}
	return c
}

// Radius returns the radius of rounded-lg in logical pixels: the theme's
// Radius, or Tailwind's 8 px when the theme keeps Tailwind's scale.
func (v ThemeValues) Radius() float32 {
	th := v.theme()
	return th.Radius["lg"].Resolve(th.RootFontSize, th.RootFontSize)
}

// FontSans and FontMono return the first family of font-sans and
// font-mono.
func (v ThemeValues) FontSans() string { return first(v.theme().Fonts["sans"]) }

// FontMono returns the first family of font-mono.
func (v ThemeValues) FontMono() string { return first(v.theme().Fonts["mono"]) }

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// Tokens returns the names of the theme's color tokens, sorted.
func (v ThemeValues) Tokens() []string { return v.theme().TokenNames() }
