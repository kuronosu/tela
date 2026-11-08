package text

import (
	"sync"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/assets/fonts/inter"
	"github.com/kuronosu/tela/internal/assets/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/internal/assets/fonts/notocoloremoji"
	"github.com/kuronosu/tela/internal/assets/fonts/notosansarabic"
)

// The deterministic font set for this package's tests. internal/stdfonts
// is the same set for every other package, but it imports this package, so
// the tests here build it again from the leaf font packages and register it
// the same way: NewDefaultCollection works in them as it does in a test
// binary that links internal/stdfonts.

// Names of the standard families.
const (
	FamilyInter          = inter.Family
	FamilyJetBrainsMono  = jetbrainsmono.Family
	FamilyNotoSansArabic = notosansarabic.Family
	FamilyNotoColorEmoji = notocoloremoji.Family
)

// stdSources are shared by every collection of the tests, so each font is
// parsed once per test binary, as internal/stdfonts does.
var stdSources = sync.OnceValue(func() []*FontSource {
	return []*FontSource{
		{Family: FamilyInter, Data: [][]byte{inter.Upright, inter.Italic}, Generics: []string{"sans-serif", "system-ui", "ui-sans-serif"}},
		{Family: FamilyJetBrainsMono, Data: [][]byte{jetbrainsmono.Upright, jetbrainsmono.Italic}, Generics: []string{"monospace", "ui-monospace"}},
		{Family: FamilyNotoSansArabic, Data: [][]byte{notosansarabic.Data}, Scripts: []language.Script{language.Arabic}},
		{Family: FamilyNotoColorEmoji, Data: [][]byte{notocoloremoji.Data}, Generics: []string{"emoji"}},
	}
})

// newStdCollection returns a collection of the standard set.
func newStdCollection() *Collection {
	c := NewCollection()
	for _, s := range stdSources() {
		if err := c.AddSource(s); err != nil {
			panic(err)
		}
	}
	return c
}

func init() { RegisterDefaultFonts(newStdCollection) }

// stdTypefaces returns the typefaces of every standard source, parsed, in
// order (Inter upright and italic, JetBrains Mono upright and italic, Noto
// Sans Arabic, Noto Color Emoji).
func stdTypefaces() [][]*Typeface {
	var out [][]*Typeface
	for _, s := range stdSources() {
		tfs, err := s.Typefaces()
		if err != nil {
			panic(err)
		}
		out = append(out, tfs)
	}
	return out
}

// stdFont is one standard font file.
type stdFont int

const (
	fontInter stdFont = iota
	fontInterItalic
	fontJetBrainsMono
	fontJetBrainsMonoItalic
	fontNotoSansArabic
	fontNotoColorEmoji
)

// allStdFonts lists the standard font files in a stable order.
var allStdFonts = []stdFont{fontInter, fontInterItalic, fontJetBrainsMono, fontJetBrainsMonoItalic, fontNotoSansArabic, fontNotoColorEmoji}

// data returns the file bytes; they must not be modified.
func (f stdFont) data() []byte {
	return [...][]byte{inter.Upright, inter.Italic, jetbrainsmono.Upright, jetbrainsmono.Italic, notosansarabic.Data, notocoloremoji.Data}[f]
}

func (f stdFont) String() string {
	return [...]string{"Inter Variable", "Inter Variable Italic", "JetBrains Mono", "JetBrains Mono Italic", "Noto Sans Arabic", "Noto Color Emoji"}[f]
}
