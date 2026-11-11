package tela

import "github.com/kuronosu/tela/internal/core"

// Hyphenation is the hyphenation dictionary of a language, for
// [App].Hyphenation: what the hyphens-auto class breaks words with
// (ADR-270). Text is hyphenated only where its language is known (h.Lang,
// or [SetDefaultLang] for the whole app) and the app brings a dictionary
// for it, as browsers do; without App.Hyphenation a binary carries no
// patterns. Soft hyphens (U+00AD, "\u00ad" in Go strings) break words
// whatever the dictionaries, unless the class hyphens-none is set.
//
// Take one from Tela's packages, hyphen/en (en.US, en.GB), hyphen/es
// (es.Spanish) and hyphen/fr (fr.French), or make one with
// [HyphenationPatterns]:
//
//	import "github.com/kuronosu/tela/hyphen/en"
//
//	tela.Main(tela.App{
//	    Hyphenation: []tela.Hyphenation{en.US},
//	    Root:        Article(),
//	})
//
//	P(Class("hyphens-auto"), Lang("en"), Text("Internationalization..."))
//
// A text's language picks a dictionary as Chromium does: "en" and "en-US"
// use American English, "en-GB" and the other English regions British
// English, "es-MX" Spanish; without that one, a dictionary of the same
// language. A word breaks only when it does not fit on its line, at a
// point that leaves 2 letters before the hyphen and 3 after it; Chromium's
// rules apply (no break in capitalized English words, nor in the last word
// of a paragraph). The patterns are compiled when text first needs them.
type Hyphenation core.Hyphenation

// HyphenationPatterns returns the dictionary of TeX hyphenation patterns
// for the text whose language is lang, a BCP 47 tag ("it", "pt-BR",
// "de-1996"; the dictionary Chromium calls so), for languages without a
// Tela package. patterns is the text of a patterns file in hyph-utf8's
// format (hyph-*.pat.txt: letters with the digits of Liang's algorithm
// between them, such as "hy3ph", separated by white space; % starts a
// comment) and exceptions that of an exceptions file (hyph-*.hyp.txt:
// words with hyphens where they break, such as "ta-ble"), "" for none.
// The letters of the patterns, and their capitals, are the alphabet: a
// word with another letter does not break.
//
// An empty pattern text or a malformed tag makes [Run] return an error
// before the window opens; patterns that do not compile are logged when
// text first needs them, and the text is not hyphenated (in tests,
// telatest fails at Mount).
//
//	//go:embed hyph-it.pat.txt
//	var italian string
//
//	tela.App{Hyphenation: []tela.Hyphenation{tela.HyphenationPatterns("it", italian, "")}}
func HyphenationPatterns(lang, patterns, exceptions string) Hyphenation {
	return Hyphenation(core.HyphenationPatterns(lang, patterns, exceptions))
}

// coreHyphenation converts the dictionaries of an App.
func coreHyphenation(hs []Hyphenation) []core.Hyphenation {
	if len(hs) == 0 {
		return nil
	}
	out := make([]core.Hyphenation, len(hs))
	for i, h := range hs {
		out[i] = core.Hyphenation(h)
	}
	return out
}
