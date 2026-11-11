// Package fr embeds the French hyphenation dictionary, in the compact
// form internal/text reads (text.NewHyphenator), generated from the TeX
// patterns in ../src by ../gen.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed French
// hyphenation (hyphen/fr, tests; ADR-270). License: MIT, Copyright Daniel
// Flipo, Bernard Gaulle 1994-2002 (../src/fr/; THIRD_PARTY.md).
package fr

import _ "embed"

// French is the dictionary of French (Chromium's hyph-fr: hyph-fr.tex
// V2.12).
//
//go:embed fr.hyph
var French string
