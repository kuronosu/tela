// Package jetbrainsmono embeds JetBrains Mono 2.304 (variable: weights 100
// to 800, upright and italic) for apps that want it instead of the
// system's monospace font. Importing it adds 0.6 MB to the binary; an app
// that does not import it carries no font file.
//
//	import "github.com/kuronosu/tela/fonts/jetbrainsmono"
//
//	tela.Main(tela.App{
//	    Title: "Terminal",
//	    Fonts: []tela.Font{jetbrainsmono.Mono},
//	    Root:  Terminal(),
//	})
//
// With it, font-mono (and code, kbd and pre) and the generic names
// ui-monospace and monospace draw JetBrains Mono. It can also be used by
// name, font-['JetBrains_Mono'].
//
// JetBrains Mono is licensed under the SIL Open Font License 1.1,
// Copyright 2020 The JetBrains Mono Project Authors
// (https://github.com/JetBrains/JetBrainsMono).
package jetbrainsmono

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/fonts/jetbrainsmono"
)

// Mono is JetBrains Mono, upright and italic, as the monospace family.
var Mono = tela.FontData(asset.Family, asset.Upright, asset.Italic).Mono()
