// Package jetbrainsmono embeds JetBrains Mono 2.304 (variable: wght axis),
// upright and italic, as raw font file bytes.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed JetBrains Mono (the
// deterministic font set of tests, internal/stdfonts, and the opt-in font
// packages; ADR-244). The files are licensed under the SIL Open Font
// License 1.1 (OFL.txt next to them; sources and checksums in
// THIRD_PARTY.md). Parsing is done by internal/text.
package jetbrainsmono

import _ "embed"

// Family is the family name the files are registered under.
const Family = "JetBrains Mono"

// Upright is JetBrainsMono-wght.ttf: JetBrains Mono 2.304, upright, variable
// (wght 100 to 800). The slice is shared: callers must not modify it.
//
//go:embed JetBrainsMono-wght.ttf
var Upright []byte

// Italic is JetBrainsMono-Italic-wght.ttf: JetBrains Mono 2.304, italic,
// variable (wght 100 to 800). The slice is shared: callers must not modify
// it.
//
//go:embed JetBrainsMono-Italic-wght.ttf
var Italic []byte
