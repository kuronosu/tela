// Package notocoloremoji embeds Noto Color Emoji 2.051, the COLRv1 build
// (vector, scalable), as raw font file bytes. The bitmap CBDT build is
// deliberately not used.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed it (the
// deterministic font set of tests, internal/stdfonts, and the opt-in font
// packages; ADR-244). The file is licensed under the SIL Open Font License
// 1.1 (OFL.txt next to it; source and checksum in THIRD_PARTY.md). Parsing
// is done by internal/text.
package notocoloremoji

import _ "embed"

// Family is the family name the file is registered under.
const Family = "Noto Color Emoji"

// Data is Noto-COLRv1.ttf. The slice is shared: callers must not modify it.
//
//go:embed Noto-COLRv1.ttf
var Data []byte
