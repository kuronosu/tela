// Package inter embeds Inter 4.1 (variable: weights 100 to 900 and optical
// sizes, upright and italic) for apps that want it instead of the system's
// sans-serif font. Importing it adds 1.8 MB to the binary; an app that does
// not import it carries no font file.
//
//	import "github.com/kuronosu/tela/fonts/inter"
//
//	tela.Main(tela.App{
//	    Title: "Notes",
//	    Fonts: []tela.Font{inter.Sans},
//	    Root:  Notes(),
//	})
//
// With it, font-sans (the default font of the app) and the generic names
// ui-sans-serif, system-ui and sans-serif draw Inter even on a machine
// that has other fonts; every other family still resolves as before. The
// font is read and parsed when text first uses it. It can also be used by
// name, font-['Inter'].
//
// Inter is licensed under the SIL Open Font License 1.1, Copyright 2016
// The Inter Project Authors (https://github.com/rsms/inter).
package inter

import (
	"github.com/kuronosu/tela"
	asset "github.com/kuronosu/tela/internal/assets/fonts/inter"
)

// Sans is Inter, upright and italic, as the sans-serif family.
var Sans = tela.FontData(asset.Family, asset.Upright, asset.Italic).Sans()
