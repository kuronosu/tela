package core

import (
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/kuronosu/tela/internal/text"
)

// NoFontMessage is what Run prints, once, when the app's text finds no
// font at all: nothing is installed (a minimal container) and the app
// embeds none (ADR-244).
const NoFontMessage = `tela: no font is installed and the app embeds none, so text draws nothing; embed one: import "github.com/kuronosu/tela/fonts/inter" (or "github.com/kuronosu/tela/fonts/standard" for all of Tela's fonts) and add it to tela.App.Fonts`

// systemFonts is the process's collection of the installed fonts, shared
// by every tree without Config.Fonts when no default font set is
// registered: the system index is loaded, and a system font parsed, once
// per process. systemFontsErr is why the system fonts could not be
// enabled (under go test, ADR-035); the collection is then empty.
var systemFonts = sync.OnceValues(func() (*text.Collection, error) {
	return text.NewSystemCollection(text.SystemFontOptions{})
})

// defaultFonts returns the fonts of a tree without Config.Fonts: a new
// collection of the default font set a package registered
// (text.RegisterDefaultFonts; internal/stdfonts does from its init, and
// only tests and telatest link it), else the system's fonts. An app's
// binary links no font package unless it embeds a font, so its text uses
// the installed fonts and the operating system's generic families.
func defaultFonts() (*text.Collection, error) {
	if c := text.NewDefaultCollection(); c != nil {
		return c, nil
	}
	return systemFonts()
}

// runFonts returns the fonts of Run without Options.Fonts (defaultFonts),
// reporting to w why the system fonts are off, the font files that cannot
// be used, and, once, that there is no font at all (NoFontMessage).
func runFonts(w io.Writer) *text.Collection {
	c, err := defaultFonts()
	reportFonts(c, err, w)
	return c
}

// reportFonts makes c report to w: err (why the system fonts are off), the
// font files that cannot be used, and NoFontMessage the first time text
// finds no font.
func reportFonts(c *text.Collection, err error, w io.Writer) {
	if err != nil {
		fmt.Fprintln(w, "tela: system fonts disabled:", err)
	}
	c.SetLogger(log.New(w, "tela: ", 0))
	c.SetNoFontHandler(func() { fmt.Fprintln(w, NoFontMessage) })
}
