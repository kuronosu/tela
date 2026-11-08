package tela

import (
	"io/fs"

	"github.com/kuronosu/tela/internal/core"
)

// Font is a font family an app brings with it, for [App].Fonts: font files
// (variable fonts and static ones, TrueType and OpenType, color emoji
// fonts included) under one family name, and the roles it plays. Without
// App.Fonts an app draws its text with the fonts installed on the system,
// and its binary carries no font file (ADR-244); a Font is how an app
// that wants one fixed look embeds its own.
//
// Make one with [FontData] (files in memory, usually //go:embed) or
// [FontFS] (files of an fs.FS), or take one of Tela's packages: fonts/inter,
// fonts/jetbrainsmono, fonts/notoemoji, fonts/notoarabic, fonts/sourceserif,
// or fonts/standard for all of them.
//
//	//go:embed Lora.ttf Lora-Italic.ttf
//	var loraFiles embed.FS
//
//	tela.Main(tela.App{
//	    Fonts: []tela.Font{
//	        tela.FontFS(loraFiles, "Lora", "*.ttf").Serif(),
//	        inter.Sans,
//	    },
//	    Root: Notes(),
//	})
//
// A font is used by the CSS font-family names: its own family name
// ("font-['Lora']") and, through the methods below, the generic families the
// theme's font-sans, font-serif and font-mono stack end with. A Font is a
// value: the methods return a modified copy and the copies share the
// parsed font, which is read and parsed when text first needs the family
// (a font that is never drawn costs only its bytes in the binary). App
// fonts come before the system's: they win the generic names they stand
// for and are tried first when a character needs a fallback.
type Font core.Font

// FontData returns the font of files already in memory, usually embedded
// with //go:embed. Give the files of one family: an upright file and an
// italic one, or one variable font. family is the name the font is used by
// (font-['Lora']); "" takes the family name the first file declares.
//
// A mistake (no files, an empty file) makes [Run] return an error before
// the window opens; a file that is not a font is logged when text first
// needs the family, and the font is left out.
//
//	//go:embed Lora[wght].ttf
//	var lora []byte
//
//	tela.App{Fonts: []tela.Font{tela.FontData("Lora", lora).Serif()}}
func FontData(family string, files ...[]byte) Font {
	return Font(core.FontData(family, files...))
}

// FontFS returns the font of the files of fsys that the patterns match
// (fs.Glob patterns, relative to the root of fsys), read when text first
// needs the family: the files of an embed.FS are not copied until then,
// and files of os.DirFS are not opened at all if the text never needs the
// font. family is the name the font is used by; "" takes the family name
// the first file declares, which reads that file when the app starts.
//
// [Run] checks the patterns before it opens the window and returns an
// error naming one that matches nothing. A file that cannot be read or
// parsed later is logged, and the font is left out.
//
//	//go:embed fonts/*.ttf
//	var fonts embed.FS
//
//	tela.App{Fonts: []tela.Font{tela.FontFS(fonts, "Lora", "fonts/Lora*.ttf").Serif()}}
func FontFS(fsys fs.FS, family string, patterns ...string) Font {
	return Font(core.FontFS(fsys, family, patterns...))
}

// Sans makes the font the family of the generic names ui-sans-serif,
// system-ui and sans-serif, which the theme's font-sans ends with: the
// default font of the app and of everything that does not name another.
//
//	tela.App{Fonts: []tela.Font{tela.FontData("Roboto", roboto).Sans()}}
func (f Font) Sans() Font {
	return Font(core.Font(f).WithGenerics("ui-sans-serif", "system-ui", "sans-serif"))
}

// Serif makes the font the family of ui-serif and serif (font-serif).
//
//	tela.App{Fonts: []tela.Font{tela.FontData("Lora", lora).Serif()}}
func (f Font) Serif() Font {
	return Font(core.Font(f).WithGenerics("ui-serif", "serif"))
}

// Mono makes the font the family of ui-monospace and monospace (font-mono,
// code).
//
//	tela.App{Fonts: []tela.Font{tela.FontData("Fira Code", fira).Mono()}}
func (f Font) Mono() Font {
	return Font(core.Font(f).WithGenerics("ui-monospace", "monospace"))
}

// Emoji makes the font the family of the emoji generic name, which emoji
// are drawn with. A color font (COLRv1, CBDT or sbix) draws them in color.
//
//	tela.App{Fonts: []tela.Font{tela.FontData("Twemoji", twemoji).Emoji()}}
func (f Font) Emoji() Font {
	return Font(core.Font(f).WithGenerics("emoji"))
}

// Scripts makes the font a fallback for the scripts, ISO 15924 codes such
// as "Arab", "Hebr", "Deva", "Thai", "Hani" or "Cyrl": text in one of them
// that the requested families cannot draw uses this font before the
// system's. A code that is not an ISO 15924 code is a programmer error: it
// panics in dev mode with the valid examples, and is ignored in a release
// build (ARCHITECTURE section 19).
//
//	tela.App{Fonts: []tela.Font{tela.FontData("Noto Sans Hebrew", hebrew).Scripts("Hebr")}}
func (f Font) Scripts(scripts ...string) Font {
	return Font(core.Font(f).WithScripts(scripts...))
}

// coreFonts converts the fonts of an App.
func coreFonts(fonts []Font) []core.Font {
	if len(fonts) == 0 {
		return nil
	}
	out := make([]core.Font, len(fonts))
	for i, f := range fonts {
		out[i] = core.Font(f)
	}
	return out
}
