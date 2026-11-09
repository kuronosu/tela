package i18n

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

// The numbering systems of locales (ADR-251). numbering_gen.go, generated
// from CLDR 48, says which locales do not write Latin digits.

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

// scriptAliases are the language-region locales that ICU's data aliases to
// their likely script, as pa_PK to pa_Arab_PK and zh_TW to zh_Hant_TW: the
// language-region IDs of the cldr-to-icu converter's config.xml that have
// no CLDR data of their own, aliased to their CLDR 48 likely subtags
// (LdmlConverter.getAliasMap; golang.org/x/text's likely subtags are CLDR
// 32's, where sd-IN is still Arabic). Other tags are not given a script:
// ICU reads az-IR as az (Latin digits), not az-Arab-IR.
var scriptAliases = map[string]string{
	"az-AZ": "Latn", "bs-BA": "Latn", "ff-CM": "Latn", "ff-GN": "Latn", "ff-MR": "Latn", "ff-SN": "Latn",
	"ks-IN": "Arab", "ku-SY": "Latn", "kxv-IN": "Latn", "mni-IN": "Beng", "pa-IN": "Guru", "pa-PK": "Arab",
	"sat-IN": "Olck", "sd-IN": "Deva", "sd-PK": "Arab", "shi-MA": "Tfng", "sr-BA": "Cyrl", "sr-ME": "Latn",
	"sr-RS": "Cyrl", "sr-XK": "Cyrl", "su-ID": "Latn", "uz-AF": "Arab", "uz-UZ": "Latn", "vai-LR": "Vaii",
	"yue-CN": "Hans", "yue-HK": "Hant", "zh-CN": "Hans", "zh-HK": "Hant", "zh-MO": "Hant", "zh-SG": "Hans",
	"zh-TW": "Hant",
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

// numberFormat is how a locale writes numbers: the digits (nil for ASCII)
// and the CLDR 48 symbols and patterns of its numbering system. When the
// tag names a system ("-u-nu-") that the tables have no symbols for,
// golang.org/x/text's printer formats the number instead (CLDR 32).
type numberFormat struct {
	digits  []string
	symbols numberSymbols
	legacy  *message.Printer
	remap   bool // the legacy printer writes ASCII digits anyway
}

// newNumberFormat chooses the numbering system of a locale: the tag's own
// ("-u-nu-"), else CLDR 48's default.
func newNumberFormat(t language.Tag) numberFormat {
	sys := t.TypeForKey("nu")
	explicit := sys != ""
	if !explicit {
		sys = defaultNumbering(t)
	}
	want := asciiDigits
	if d, ok := numberingDigits[sys]; ok {
		want = splitDigits(d)
	}
	syms, found := symbolsFor(t, sys)
	if found || !explicit {
		return numberFormat{digits: nonASCII(want), symbols: syms}
	}
	// A system of the tag that no locale of CLDR 48's tables uses by
	// default: x/text knows more of them (en-u-nu-arab).
	p := message.NewPrinter(t)
	own := digitsOf(p)
	if sys != "latn" && equalDigits(own, asciiDigits) && !equalDigits(want, asciiDigits) {
		return numberFormat{digits: nonASCII(want), legacy: p, remap: true}
	}
	return numberFormat{digits: nonASCII(own), legacy: p}
}

var asciiDigits = splitDigits("0123456789")

func splitDigits(s string) []string {
	out := make([]string, 0, 10)
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}

func digitsOf(p *message.Printer) []string {
	digits := make([]string, 10)
	for i := range digits {
		digits[i] = p.Sprint(number.Decimal(i, number.NoSeparator()))
	}
	return digits
}

func equalDigits(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// nonASCII returns digits, or nil when they are 0 to 9.
func nonASCII(digits []string) []string {
	if equalDigits(digits, asciiDigits) {
		return nil
	}
	return digits
}
