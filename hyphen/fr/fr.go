// Package fr embeds the French hyphenation dictionary for apps whose text
// uses the hyphens-auto class (ADR-270): the patterns Chromium hyphenates
// French with. Importing it adds about 4 KB to the binary; an app that
// does not import it carries no French patterns.
//
//	import "github.com/kuronosu/tela/hyphen/fr"
//
//	tela.Main(tela.App{
//	    Title:       "Notes",
//	    Hyphenation: []tela.Hyphenation{fr.French},
//	    Root:        Notes(),
//	})
//
// Text breaks with it where its language is French (h.Lang("fr"), or
// tela.SetDefaultLang for the whole app), in every region ("fr-FR",
// "fr-CA"...). The dictionary is compiled when text first needs it.
//
// License: MIT, Copyright Daniel Flipo, Bernard Gaulle 1994-2002. See
// THIRD_PARTY.md.
package fr

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/hyphen/fr"
)

// French hyphenates French (Chromium's fr dictionary: hyph-fr.tex V2.12).
var French = tela.HyphenationPatterns("fr", asset.French, "")
