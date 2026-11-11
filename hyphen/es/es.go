// Package es embeds the Spanish hyphenation dictionary for apps whose
// text uses the hyphens-auto class (ADR-270): the patterns Chromium
// hyphenates Spanish with. Importing it adds about 10 KB to the binary;
// an app that does not import it carries no Spanish patterns.
//
//	import "github.com/kuronosu/tela/hyphen/es"
//
//	tela.Main(tela.App{
//	    Title:       "Notas",
//	    Hyphenation: []tela.Hyphenation{es.Spanish},
//	    Root:        Notas(),
//	})
//
// Text breaks with it where its language is Spanish (h.Lang("es"), or
// tela.SetDefaultLang for the whole app), in every region ("es-ES",
// "es-MX", "es-419"...). The dictionary is compiled when text first needs
// it.
//
// License: MIT, Copyright (c) 1993, 1997 Javier Bezos, Copyright (c)
// 2001-2015 Javier Bezos and CervanTeX. See THIRD_PARTY.md.
package es

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/hyphen/es"
)

// Spanish hyphenates Spanish (Chromium's es dictionary: hyph-es.tex 4.7).
var Spanish = tela.HyphenationPatterns("es", asset.Spanish, "")
