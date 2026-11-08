// Package sourceserif4 embeds Source Serif 4 (version 4.004, variable: opsz
// and wght axes), upright and italic, as raw font file bytes.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed Source Serif 4 (the
// deterministic font set of tests, internal/stdfonts, and the opt-in font
// package fonts/sourceserif; ADR-244), so a binary that does not import it
// carries no Source Serif. The files are licensed under the SIL Open Font
// License 1.1 (OFL.txt next to them; sources and checksums in
// THIRD_PARTY.md). Parsing is done by internal/text.
package sourceserif4

import _ "embed"

// Family is the family name the files are registered under.
const Family = "Source Serif 4"

// Upright is SourceSerif4-opsz-wght.ttf: Source Serif 4, upright, variable
// (opsz 8 to 60, wght 200 to 900). The slice is shared: callers must not
// modify it.
//
//go:embed SourceSerif4-opsz-wght.ttf
var Upright []byte

// Italic is SourceSerif4-Italic-opsz-wght.ttf: Source Serif 4, italic,
// variable (opsz, wght). The slice is shared: callers must not modify it.
//
//go:embed SourceSerif4-Italic-opsz-wght.ttf
var Italic []byte
