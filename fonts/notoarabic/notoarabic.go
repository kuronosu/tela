// Package notoarabic embeds Noto Sans Arabic 2.012 (variable: weights and
// widths) as the fallback for Arabic text, for apps that want the same
// Arabic letters on every machine instead of the system's. Importing it
// adds 0.8 MB to the binary; an app that does not import it carries no
// font file.
//
//	import "github.com/kuronosu/tela/fonts/notoarabic"
//
//	tela.Main(tela.App{
//	    Title: "Reader",
//	    Fonts: []tela.Font{inter.Sans, notoarabic.Arabic},
//	    Root:  Reader(),
//	})
//
// Arabic text (script Arab) that the requested family cannot draw uses it
// before the system's fonts. It can also be used by name,
// font-['Noto_Sans_Arabic'].
//
// Noto Sans Arabic is licensed under the SIL Open Font License 1.1,
// Copyright 2022 The Noto Project Authors (https://github.com/notofonts/arabic).
package notoarabic

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/fonts/notosansarabic"
)

// Arabic is Noto Sans Arabic as the fallback of the Arab script.
var Arabic = tela.FontData(asset.Family, asset.Data).Scripts("Arab")
