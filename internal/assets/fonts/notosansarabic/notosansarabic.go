// Package notosansarabic embeds Noto Sans Arabic 2.012 (variable: wdth and
// wght axes) as raw font file bytes.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed it (the
// deterministic font set of tests, internal/stdfonts, and the opt-in font
// packages; ADR-244). The file is licensed under the SIL Open Font License
// 1.1 (OFL.txt next to it; source and checksum in THIRD_PARTY.md). Parsing
// is done by internal/text.
package notosansarabic

import _ "embed"

// Family is the family name the file is registered under.
const Family = "Noto Sans Arabic"

// Data is NotoSansArabic-wdth-wght.ttf: every weight (100 to 900) and width
// (62.5% to 100%) of Noto Sans Arabic in one variable font. The slice is
// shared: callers must not modify it.
//
//go:embed NotoSansArabic-wdth-wght.ttf
var Data []byte
