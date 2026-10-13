// Package style is Tela's style layer (ARCHITECTURE section 10): the theme
// and design tokens, the Tailwind v4 class parser, and the resolution of
// classes and typed props into a ComputedStyle.
//
// style imports only gfx and text, and sits just above the text layer
// (ARCHITECTURE section 5). It does not know layout or core: core maps a
// ComputedStyle onto layout's own style, onto text.TextStyle and
// text.ParagraphStyle (ComputedStyle.TextStyle and ParagraphStyle), and
// paints boxes from it with gfx.
//
// # Pipeline
//
//	class string --Parser.Compile--> *Classes (rules: variants + Op)
//	                                      |
//	element kind defaults + Classes + typed props (Ops)
//	                                      |
//	parent *ComputedStyle, Context, state --Compute--> *ComputedStyle
//
// Precedence is ARCHITECTURE section 10's: the defaults of the element kind
// (KindDefaults, from Preflight, ADR-038), then the classes in order of
// appearance, then the typed props; on conflict the last wins. There is no
// cascade or specificity. Inheritance applies to the text properties of
// TextProps, plus cursor, visibility, pointer-events and user-select, which
// inherit in browsers and would otherwise break "pointer-events-none" on a
// container and the cursor of a button's label.
//
// # Parsing and interning
//
// Parser.Compile parses a class string once and interns the result, keyed by
// the exact string, in a bounded two-generation cache that is safe for
// concurrent use. A string with invalid classes still yields the rules of
// the valid ones, and Classes.Err lists every problem as an
// UnknownClassError (with the closest valid class by edit distance), an
// UnsupportedClassError (valid Tailwind that Tela does not implement, with
// what to use instead) or an InvalidClassError (a bad arbitrary value).
// A Parser is bound to a theme; Compile uses the default one.
//
// The class vocabulary behind suggestions (Parser.Vocabulary) is generated
// from the same registry the parser uses, expanded with the theme keys, so
// a suggestion is always a class that parses (ADR-037).
//
// # Computation
//
// Compute applies the ops in two passes: first the ops that set the font
// size, then everything else, so that em lengths resolve against the
// element's final font size regardless of class order. Several Tailwind
// features are built from shared "variables" (--tw-shadow, --tw-ring-*,
// --tw-gradient-*, --tw-leading, --tw-duration): they accumulate in a
// scratch state and are combined at the end, so "shadow-lg
// shadow-indigo-500" and "shadow-indigo-500 shadow-lg" are the same, and
// leading-6 wins over the line height of text-sm in any order.
//
// Lengths that depend on layout stay symbolic: LP (pixels plus percent) and
// Dim (auto, min-content, max-content, fit-content or an LP). rem, em, vw,
// vh and ch are resolved to pixels using the Context (theme root size,
// element font size, viewport). Gradients keep their CSS meaning (angle or
// corner keyword, stop positions) and become gfx paints for a given box
// with Gradient.Paint.
//
// Variants are conditions (Condition) evaluated against the element state,
// the states of its group ancestors and peers, dark mode, reduced motion and
// the window width (VariantContext).
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
// vocabulary behind "did you mean" suggestions.
//
// COVERAGE.md lists which Tailwind utilities are supported, partially
// supported or not supported, and what to use instead.
package style
