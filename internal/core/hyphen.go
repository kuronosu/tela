package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kuronosu/tela/internal/text"
)

// Hyphenation is a hyphenation dictionary an app brings (tela.Hyphenation,
// ADR-270): the patterns of one language that hyphens-auto breaks words
// with. Like fonts, no dictionary is in a binary unless the app imports
// one (the hyphen/... packages) or makes its own. The zero Hyphenation is
// invalid: Check reports it.
type Hyphenation struct {
	h   *text.Hyphenator
	err error
}

// HyphenationPatterns returns the dictionary of TeX hyphenation patterns
// and exceptions for the language lang (text.NewHyphenator), compiled when
// text first needs it.
func HyphenationPatterns(lang, patterns, exceptions string) Hyphenation {
	switch {
	case strings.TrimSpace(lang) == "":
		return Hyphenation{err: errors.New("tela.HyphenationPatterns: give the language of the patterns, a BCP 47 tag such as \"it\" or \"pt-BR\"")}
	case !validLangTag(strings.TrimSpace(lang)):
		return Hyphenation{err: fmt.Errorf("tela.HyphenationPatterns(%q): the language is a BCP 47 tag, such as \"it\" or \"pt-BR\"", lang)}
	case strings.TrimSpace(patterns) == "":
		return Hyphenation{err: fmt.Errorf("tela.HyphenationPatterns(%q): the patterns are empty; give the text of a TeX patterns file (hyph-utf8's hyph-*.pat.txt), embedded with //go:embed", lang)}
	}
	return Hyphenation{h: text.NewHyphenator(lang, patterns, exceptions)}
}

// Check reports a dictionary that cannot be used: the zero value, or a
// mistake HyphenationPatterns found. Patterns that do not compile are
// found when text first needs them (Verify finds them now).
func (h Hyphenation) Check() error {
	switch {
	case h.err != nil:
		return h.err
	case h.h == nil:
		return errors.New("tela: a zero tela.Hyphenation; take one from a hyphen/... package (hyphen/en's en.US) or tela.HyphenationPatterns")
	}
	return nil
}

// Verify is Check, and compiles the patterns to report those that do not
// compile (telatest calls it, so a broken dictionary fails the test).
func (h Hyphenation) Verify() error {
	if err := h.Check(); err != nil {
		return err
	}
	return h.h.Load()
}

// Lang returns the language of the dictionary ("" for an invalid one).
func (h Hyphenation) Lang() string {
	if h.h == nil {
		return ""
	}
	return h.h.Lang()
}

// Hyphenators returns the dictionaries of hs that Check accepts, for
// text.Collection.SetHyphenators.
func Hyphenators(hs []Hyphenation) []*text.Hyphenator {
	var out []*text.Hyphenator
	for _, h := range hs {
		if h.Check() == nil {
			out = append(out, h.h)
		}
	}
	return out
}

// CheckHyphenation reports the first dictionary of hs that cannot be used
// (Check).
func CheckHyphenation(hs []Hyphenation) error {
	for _, h := range hs {
		if err := h.Check(); err != nil {
			return err
		}
	}
	return nil
}
