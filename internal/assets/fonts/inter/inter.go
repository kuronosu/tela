// Package inter embeds Inter 4.1 (variable: opsz and wght axes), upright and
// italic, as raw font file bytes.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed Inter (the
// deterministic font set of tests, internal/stdfonts, and the opt-in font
// packages; ADR-244), so a binary that does not import it carries no Inter.
// The files are licensed under the SIL Open Font License 1.1 (OFL.txt next
// to them; sources and checksums in THIRD_PARTY.md). Parsing is done by
// internal/text.
package inter

import _ "embed"

// Family is the family name the files are registered under.
const Family = "Inter"

// Upright is InterVariable.ttf: Inter 4.1, upright, variable (opsz 14 to 32,
// wght 100 to 900). The slice is shared: callers must not modify it.
//
//go:embed InterVariable.ttf
var Upright []byte

// Italic is InterVariable-Italic.ttf: Inter 4.1, italic, variable (opsz,
// wght). The slice is shared: callers must not modify it.
//
//go:embed InterVariable-Italic.ttf
var Italic []byte
