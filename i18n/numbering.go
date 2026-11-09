package i18n

import (
	"strings"

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
	b, s, r := t.Raw()
	base := b.String()
	script, region := s.String(), r.String()
	hasScript, hasRegion := script != "Zzzz", region != "ZZ"
	var chain []string
	if hasScript && hasRegion {
		chain = append(chain, base+"-"+script+"-"+region)
	}
	if hasScript {
		chain = append(chain, base+"-"+script)
	}
	if hasRegion {
		chain = append(chain, base+"-"+region)
	}
	chain = append(chain, base)
	for _, name := range chain {
		if sys, ok := numberingDefaults[name]; ok {
			return sys
		}
	}
	return "latn"
}

// numberFormat is how a locale writes numbers: the x/text printer, the
// digits (nil for ASCII) and whether the printer's ASCII digits must still
// be replaced by them.
type numberFormat struct {
	printer *message.Printer
	digits  []string
	remap   bool
}

// newNumberFormat chooses the numbering system of a locale: the tag's own
// ("-u-nu-"), else CLDR 48's default. golang.org/x/text has CLDR 32's
// defaults, so when its digits for the tag are not the ones wanted the
// printer is made for the tag with the system named, and for a system it
// does not know the digits are replaced after formatting.
func newNumberFormat(t language.Tag) numberFormat {
	own := message.NewPrinter(t)
	ownDigits := digitsOf(own)
	if t.TypeForKey("nu") != "" {
		return numberFormat{printer: own, digits: nonASCII(ownDigits)}
	}
	sys := defaultNumbering(t)
	want := asciiDigits
	if sys != "latn" {
		want = splitDigits(numberingDigits[sys])
	}
	if equalDigits(ownDigits, want) {
		return numberFormat{printer: own, digits: nonASCII(want)}
	}
	if nt, err := t.SetTypeForKey("nu", sys); err == nil {
		p := message.NewPrinter(nt)
		if equalDigits(digitsOf(p), want) {
			return numberFormat{printer: p, digits: nonASCII(want)}
		}
	}
	// x/text cannot write the system (newer than its CLDR): its ASCII
	// output is remapped.
	return numberFormat{printer: own, digits: nonASCII(want), remap: nonASCII(want) != nil && equalDigits(ownDigits, asciiDigits)}
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

// replaceDigits writes the ASCII digits of s with digits.
func replaceDigits(s string, digits []string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteString(digits[r-'0'])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
