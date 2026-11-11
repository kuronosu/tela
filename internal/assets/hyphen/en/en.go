// Package en embeds the English hyphenation dictionaries, in the compact
// form internal/text reads (text.NewHyphenator), generated from the TeX
// patterns in ../src by ../gen.
//
// The package is a leaf: it depends on nothing but the standard library and
// is imported only by the packages that choose to embed English
// hyphenation (hyphen/en, tests; ADR-270), so a binary that does not
// import it carries no patterns. Licenses: en-US, Copyright (C) 1990,
// 2004, 2005 Gerard D.C. Kuiken (copying and distribution permitted with
// the notice) and Copyright 2008 TeX Users Group (exceptions, freely
// usable); en-GB, MIT, Copyright (C) 1992, 1996, 2005, 2016 Dominik
// Wujastyk, Graham Toal (../src/*/; THIRD_PARTY.md).
package en

import _ "embed"

// US is the dictionary of American English (Chromium's hyph-en-us: the
// patterns of ushyphmax.tex and the exceptions of hyphen.tex and
// ushyphex.tex).
//
//go:embed en-us.hyph
var US string

// GB is the dictionary of British English (Chromium's hyph-en-gb:
// ukhyphen.tex).
//
//go:embed en-gb.hyph
var GB string
