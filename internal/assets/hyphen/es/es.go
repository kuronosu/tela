// Package es embeds the Spanish hyphenation dictionary, in the compact
// form internal/text reads (text.NewHyphenator), generated from the TeX
// patterns in ../src by ../gen.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed Spanish
// hyphenation (hyphen/es, tests; ADR-270). License: MIT, Copyright (c)
// 1993, 1997 Javier Bezos, Copyright (c) 2001-2015 Javier Bezos and
// CervanTeX (../src/es/; THIRD_PARTY.md).
package es

import _ "embed"

// Spanish is the dictionary of Spanish (Chromium's hyph-es: hyph-es.tex
// 4.7).
//
//go:embed es.hyph
var Spanish string
