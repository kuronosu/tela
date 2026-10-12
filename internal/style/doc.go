// Package style is Tela's style layer (ARCHITECTURE section 10): the theme
// and design tokens, and the Tailwind v4 class vocabulary built on them.
//
// style imports only gfx and text, and sits just above the text layer
// (ARCHITECTURE section 5).
//
// # Theme
//
// Theme holds every token a Tailwind class can name: the color palette,
// the spacing base, breakpoints, type scale, radii, shadows, easing curves
// and font stacks. The default theme is generated, not hand-copied: the
// generator in internal/style/gen reads the vendored Tailwind v4 theme.css
// and writes theme_gen.go (run "go generate ./internal/style"). OKLCH colors
// are converted to sRGB at generation time (ADR-018, ADR-037).
//
// DefaultTheme returns a fresh copy on every call, so a caller may edit
// the maps of the value it receives. Theme.Keys enumerates the keys of a
// namespace in sorted order; the class parser uses it to expand the
// vocabulary behind "did you mean" suggestions (ADR-037).
package style
