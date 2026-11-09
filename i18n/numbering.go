package i18n

import (
	"golang.org/x/text/language"
)

// The numbering systems of locales (ADR-251, ADR-266). numbering_gen.go,
// generated from CLDR 48, says which locales do not write Latin digits,
// the digits of every numeric system, and the script ICU gives the
// language-region locales it aliases (scriptAliases).

// defaultNumbering returns the default numbering system of a locale
// according to CLDR 48 ("latn", "arab", "arabext", "beng"...), found as
// ICU finds it: the tag's own name, then language-script, language-region
// and the language (ar-AE is Latin, ar-EG Arabic-Indic). An explicit
// numbering system in the tag ("ar-u-nu-latn") is not looked at here.
func defaultNumbering(t language.Tag) string {
	for _, name := range localeChain(t) {
		if sys, ok := numberingDefaults[name]; ok {
			return sys
		}
	}
	return "latn"
}

// numberingOf returns the numbering system a tag writes in: the explicit
// one ("-u-nu-"), or sys when it is not "", if it is a numeric system CLDR
// knows, else the locale's default. Like ICU, an unknown or algorithmic
// system ("-u-nu-roman") gives the default.
func numberingOf(t language.Tag, sys string) string {
	if sys == "" {
		sys = t.TypeForKey("nu")
	}
	if _, ok := numberingDigits[sys]; ok || sys == "latn" {
		return sys
	}
	return defaultNumbering(t)
}

// localeChain returns the names a tag's CLDR data is looked up under, most
// specific first: language-script-region, language-script, language-region
// and the language. A language-region tag that ICU aliases (scriptAliases)
// gets its likely script.
func localeChain(t language.Tag) []string {
	b, s, r := t.Raw()
	base := b.String()
	script, region := s.String(), r.String()
	hasScript, hasRegion := script != "Zzzz", region != "ZZ"
	if !hasScript && hasRegion {
		script, hasScript = scriptAliases[base+"-"+region]
	}
	chain := make([]string, 0, 4)
	if hasScript && hasRegion {
		chain = append(chain, base+"-"+script+"-"+region)
	}
	if hasScript {
		chain = append(chain, base+"-"+script)
	}
	if hasRegion {
		chain = append(chain, base+"-"+region)
	}
	return append(chain, base)
}

// digitsOf returns the digits of a numbering system, nil for ASCII.
func digitsOf(sys string) []string {
	d, ok := numberingDigits[sys]
	if !ok {
		return nil
	}
	out := make([]string, 0, 10)
	for _, r := range d {
		out = append(out, string(r))
	}
	return out
}
