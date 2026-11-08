// Package stdfonts is the deterministic font set of Tela's tests and
// snapshots (ADR-008, ADR-244): Inter for sans-serif, system-ui and
// ui-sans-serif, JetBrains Mono for monospace and ui-monospace, Noto Color
// Emoji for emoji and Noto Sans Arabic for Arabic text. It is what
// text.NewDefaultCollection returned before Tela stopped embedding fonts
// by default, and the goldens are drawn with it.
//
// Importing the package embeds the four families (7.2 MB) and registers the
// set in internal/text (text.RegisterDefaultFonts) from its init, so that a
// tree without core.Config.Fonts uses it instead of the system's fonts.
// Only test files (a blank import in one _test.go file of each package that
// renders text) and telatest, which only tests link, import it: an app's
// binary carries no font file unless the app embeds one.
//
// The fonts are FontSources: a family is read and parsed when text first
// needs it, once per process, and the typefaces are shared by every
// collection NewCollection returns.
package stdfonts

import (
	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/assets/fonts/inter"
	"github.com/kuronosu/tela/internal/assets/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/internal/assets/fonts/notocoloremoji"
	"github.com/kuronosu/tela/internal/assets/fonts/notosansarabic"
	"github.com/kuronosu/tela/internal/text"
)

// The family names of the set.
const (
	Inter          = inter.Family
	JetBrainsMono  = jetbrainsmono.Family
	NotoSansArabic = notosansarabic.Family
	NotoColorEmoji = notocoloremoji.Family
)

// The sources, shared by every collection of the process so that each font
// is parsed once.
var (
	interSource = &text.FontSource{
		Family:   Inter,
		Data:     [][]byte{inter.Upright, inter.Italic},
		Generics: []string{"sans-serif", "system-ui", "ui-sans-serif"},
	}
	jetBrainsMonoSource = &text.FontSource{
		Family:   JetBrainsMono,
		Data:     [][]byte{jetbrainsmono.Upright, jetbrainsmono.Italic},
		Generics: []string{"monospace", "ui-monospace"},
	}
	notoSansArabicSource = &text.FontSource{
		Family:  NotoSansArabic,
		Data:    [][]byte{notosansarabic.Data},
		Scripts: []language.Script{language.Arabic},
	}
	notoColorEmojiSource = &text.FontSource{
		Family:   NotoColorEmoji,
		Data:     [][]byte{notocoloremoji.Data},
		Generics: []string{"emoji"},
	}
)

// Sources returns the sources of the set in registration order (Inter,
// JetBrains Mono, Noto Sans Arabic, Noto Color Emoji), which is also the
// order fallback walks them. The sources are shared: do not modify them.
func Sources() []*text.FontSource {
	return []*text.FontSource{interSource, jetBrainsMonoSource, notoSansArabicSource, notoColorEmojiSource}
}

// NewCollection returns a new collection of the set: the sources
// registered (nothing is parsed yet), the generic families mapped, and no
// system fonts.
func NewCollection() *text.Collection {
	c := text.NewCollection()
	for _, s := range Sources() {
		_ = c.AddSource(s) // never fails: the sources have a family and data
	}
	return c
}

func init() { text.RegisterDefaultFonts(NewCollection) }
