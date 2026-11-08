// Package sourceserif embeds Source Serif 4 (version 4.004, variable:
// weights 200 to 900 and optical sizes, upright and italic) for apps that
// want it instead of the system's serif font. Importing it adds 2.1 MB to
// the binary; an app that does not import it carries no font file.
//
//	import "github.com/kuronosu/tela/fonts/sourceserif"
//
//	tela.Main(tela.App{
//	    Title: "Reader",
//	    Fonts: []tela.Font{sourceserif.Serif},
//	    Root:  Reader(),
//	})
//
// With it, font-serif and the generic names ui-serif and serif draw
// Source Serif 4. It can also be used by name, font-['Source_Serif_4'].
//
// Source Serif 4 is licensed under the SIL Open Font License 1.1,
// Copyright 2014 The Source Serif 4 Project Authors
// (https://github.com/adobe-fonts/source-serif).
package sourceserif

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/fonts/sourceserif4"
)

// Serif is Source Serif 4, upright and italic, as the serif family.
var Serif = tela.FontData(asset.Family, asset.Upright, asset.Italic).Serif()
