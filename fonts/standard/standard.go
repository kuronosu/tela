// Package standard is all of Tela's fonts in one list: Inter (sans-serif),
// JetBrains Mono (monospace), Noto Sans Arabic (Arabic), Noto Color Emoji
// (emoji) and Source Serif 4 (serif). It is the set tela snapshot, tela mcp
// and telatest draw with, so a screenshot looks the same on every machine,
// and the one an app imports to look the same everywhere. Importing it
// adds 10 MB to the binary (each font is read and parsed only when text
// first needs it); import the packages of the fonts the app needs
// (fonts/inter, fonts/jetbrainsmono, fonts/sourceserif, fonts/notoemoji,
// fonts/notoarabic) to embed less.
//
//	import "github.com/kuronosu/tela/fonts/standard"
//
//	tela.Main(tela.App{
//	    Title: "Notes",
//	    Fonts: standard.Fonts,
//	    Root:  Notes(),
//	})
//
// The licenses are the SIL Open Font License 1.1 of each font; see the
// packages of the fonts.
package standard

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/fonts/inter"
	"github.com/kuronosu/tela/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/fonts/notoarabic"
	"github.com/kuronosu/tela/fonts/notoemoji"
	"github.com/kuronosu/tela/fonts/sourceserif"
)

// Fonts is Inter, JetBrains Mono, Noto Sans Arabic, Noto Color Emoji and
// Source Serif 4, in the order text falls back through them. Do not modify
// the slice; append it to your own fonts with append([]tela.Font{mine},
// standard.Fonts...) (a copy).
var Fonts = []tela.Font{inter.Sans, jetbrainsmono.Mono, notoarabic.Arabic, notoemoji.Emoji, sourceserif.Serif}
