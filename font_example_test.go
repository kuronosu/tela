package tela_test

import (
	"embed"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/fonts/inter"
	"github.com/kuronosu/tela/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/fonts/notoarabic"
)

// Fonts brings Inter instead of the system's sans-serif font.
func ExampleApp_fonts() {
	tela.Main(tela.App{
		Title: "Notes",
		Fonts: []tela.Font{inter.Sans, jetbrainsmono.Mono},
		Root:  tela.C(func() tela.Node { return nil }),
	})
}

var loraFiles embed.FS

// FontFS reads the files of an embed.FS (or os.DirFS) when text first needs
// the family; Run checks the patterns before it opens the window.
func ExampleFontFS() {
	// //go:embed fonts/Lora*.ttf
	// var loraFiles embed.FS
	lora := tela.FontFS(loraFiles, "Lora", "fonts/Lora*.ttf").Serif()

	tela.Main(tela.App{
		Title: "Reader",
		Fonts: []tela.Font{lora},
		Root:  tela.C(func() tela.Node { return nil }),
	})
}

// FontData takes font files already in memory. With "" as the family, the
// font keeps the name its file declares.
func ExampleFontData() {
	// //go:embed Roboto.ttf RobotoItalic.ttf
	var roboto, robotoItalic []byte
	sans := tela.FontData("Roboto", roboto, robotoItalic).Sans()

	tela.Main(tela.App{
		Title: "Notes",
		Fonts: []tela.Font{sans},
		Root:  tela.C(func() tela.Node { return nil }),
	})
}

// Sans, Serif, Mono and Emoji make a font the family of the generic names
// the theme's font-sans, font-serif and font-mono end with; they combine.
func ExampleFont_Mono() {
	// //go:embed FiraCode.ttf
	var fira []byte
	code := tela.FontData("Fira Code", fira).Mono()
	_ = tela.App{Fonts: []tela.Font{code}}
}

// Scripts makes a font the fallback for the scripts of text that the
// requested family cannot draw. The codes are ISO 15924.
func ExampleFont_Scripts() {
	// //go:embed NotoSansHebrew.ttf
	var hebrew []byte
	f := tela.FontData("Noto Sans Hebrew", hebrew).Scripts("Hebr")
	_ = tela.App{Fonts: []tela.Font{f, notoarabic.Arabic}}
}
