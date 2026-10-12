package text

import (
	"fmt"
	"sync"

	"github.com/go-text/typesetting/language"
	"github.com/kuronosu/tela/internal/assets/fonts"
)

// Names of the embedded families.
const (
	FamilyInter          = "Inter"
	FamilyJetBrainsMono  = "JetBrains Mono"
	FamilyNotoSansArabic = "Noto Sans Arabic"
	FamilyNotoColorEmoji = "Noto Color Emoji"
)

var embeddedFamilies = []struct {
	family string
	fonts  []fonts.Font
}{
	{FamilyInter, []fonts.Font{fonts.InterVariable, fonts.InterVariableItalic}},
	{FamilyJetBrainsMono, []fonts.Font{fonts.JetBrainsMono, fonts.JetBrainsMonoItalic}},
	{FamilyNotoSansArabic, []fonts.Font{fonts.NotoSansArabic}},
	{FamilyNotoColorEmoji, []fonts.Font{fonts.NotoColorEmoji}},
}

var (
	embeddedOnce sync.Once
	embeddedTFs  [][]*Typeface // per embeddedFamilies entry
)

// embeddedTypefaces parses the embedded fonts once per process; typefaces
// are immutable and shared by every Collection.
func embeddedTypefaces() [][]*Typeface {
	embeddedOnce.Do(func() {
		embeddedTFs = make([][]*Typeface, len(embeddedFamilies))
		for i, ef := range embeddedFamilies {
			for _, f := range ef.fonts {
				tfs, err := ParseTypefaces(fonts.Data(f), ef.family)
				if err != nil {
					panic(fmt.Sprintf("text: embedded font %s: %v", fonts.Name(f), err))
				}
				embeddedTFs[i] = append(embeddedTFs[i], tfs...)
			}
		}
	})
	return embeddedTFs
}

// NewDefaultCollection returns a collection with the embedded fonts
// registered (as the last fallbacks) and the generic families mapped:
//
//   - "sans-serif", "system-ui", "ui-sans-serif": Inter
//   - "monospace", "ui-monospace": JetBrains Mono
//   - "emoji": Noto Color Emoji
//
// and Arabic text falling back to Noto Sans Arabic. Fonts registered later
// are tried before the embedded ones. The embedded fonts are parsed once
// per process and their faces are shared between collections.
func NewDefaultCollection() *Collection {
	c := NewCollection()
	for _, tfs := range embeddedTypefaces() {
		for _, tf := range tfs {
			c.add(tf, true)
		}
	}
	c.SetGeneric("sans-serif", FamilyInter)
	c.SetGeneric("system-ui", FamilyInter)
	c.SetGeneric("ui-sans-serif", FamilyInter)
	c.SetGeneric("monospace", FamilyJetBrainsMono)
	c.SetGeneric("ui-monospace", FamilyJetBrainsMono)
	c.SetGeneric("emoji", FamilyNotoColorEmoji)
	c.SetScriptFallback(language.Arabic, FamilyNotoSansArabic)
	return c
}
