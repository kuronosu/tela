// Package en embeds the English hyphenation dictionaries for apps whose
// text uses the hyphens-auto class (ADR-270): the patterns Chromium
// hyphenates English with. Importing it adds about 46 KB to the binary;
// an app that does not import it carries no English patterns.
//
//	import "github.com/kuronosu/tela/hyphen/en"
//
//	tela.Main(tela.App{
//	    Title:       "Notes",
//	    Hyphenation: []tela.Hyphenation{en.US, en.GB},
//	    Root:        Notes(),
//	})
//
// Text breaks with them where its language is English (h.Lang("en"), or
// tela.SetDefaultLang for the whole app): "en", "en-US" and a few US
// regions use US; "en-GB" and the other English regions use GB (as in
// Chromium), or US when the app brings only that one. A dictionary is
// compiled when text first needs it.
//
// Licenses: the American English patterns are Copyright (C) 1990, 2004,
// 2005 Gerard D.C. Kuiken (copying and distribution permitted, with or
// without modification, provided the copyright notice and this notice
// are preserved) and its exceptions Copyright 2008 TeX Users Group (may
// be freely used, modified and distributed); the British English patterns
// are Copyright (C) 1992, 1996, 2005, 2016 Dominik Wujastyk, Graham Toal,
// under the MIT license. See THIRD_PARTY.md.
package en

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/hyphen/en"
)

// US hyphenates American English (Chromium's en-US dictionary: Knuth's and
// Kuiken's patterns, ushyphmax.tex, and the TUGboat exceptions).
var US = tela.HyphenationPatterns("en-US", asset.US, "")

// GB hyphenates British English (Chromium's en-GB dictionary:
// ukhyphen.tex).
var GB = tela.HyphenationPatterns("en-GB", asset.GB, "")
