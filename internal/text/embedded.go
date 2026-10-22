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
//
// The files are parsed concurrently, one goroutine each, and assembled in
// their fixed order, so the result is the same as a sequential parse. It is
// on the start-up path of every app (and of every reload of tela dev,
// ADR-116): go-text parses every outline of Noto Color Emoji up front,
// about 40 ms of the 70 ms the six files take one after another.
func embeddedTypefaces() [][]*Typeface {
	embeddedOnce.Do(func() {
		type job struct {
			tfs []*Typeface
			err error
		}
		jobs := make([][]job, len(embeddedFamilies))
		var wg sync.WaitGroup
		for i, ef := range embeddedFamilies {
			jobs[i] = make([]job, len(ef.fonts))
			for j, f := range ef.fonts {
				wg.Go(func() {
					jobs[i][j].tfs, jobs[i][j].err = ParseTypefaces(fonts.Data(f), ef.family)
				})
			}
		}
		wg.Wait()
		embeddedTFs = make([][]*Typeface, len(embeddedFamilies))
		for i, ef := range embeddedFamilies {
			for j, f := range ef.fonts {
				if err := jobs[i][j].err; err != nil {
					panic(fmt.Sprintf("text: embedded font %s: %v", fonts.Name(f), err))
				}
				embeddedTFs[i] = append(embeddedTFs[i], jobs[i][j].tfs...)
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
