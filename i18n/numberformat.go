package i18n

import (
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
)

// Number symbols and patterns (ADR-259). golang.org/x/text has CLDR 32's,
// which differ from ICU's (CLDR 48): the minus sign of Persian and Urdu,
// the bidi marks around negatives, Nepali grouping, the minimum grouping
// digits of Spanish and Polish... So x/text only rounds: it formats the
// number with English symbols and no grouping, and the text is rewritten
// here with the symbols and patterns of numberformats_gen.go.

// numberSymbols is what a locale writes a number with in one numbering
// system: the symbols, the affixes of the percent pattern (with the
// percent sign in them), the primary and secondary grouping sizes of the
// decimal pattern (0: no grouping) and the minimum grouping digits (the
// first separator needs that many digits ahead of the last group).
type numberSymbols struct {
	decimal, group, minus, infinity, nan string
	pctPrefix, pctSuffix                 string
	primary, secondary, minGroup         uint8
}

// symbolsFor returns the symbols of a tag in a numbering system: the first
// of the tag's names (localeChain) and the root that has data for the
// system. A system no name has gets the Latin symbols and false.
func symbolsFor(t language.Tag, sys string) (numberSymbols, bool) {
	chain := localeChain(t)
	for _, key := range []string{sys, "latn"} {
		for _, name := range chain {
			if s, ok := numberTable[name+"/"+key]; ok {
				return s, key == sys
			}
		}
		if s, ok := numberTable["und/"+key]; ok {
			return s, key == sys
		}
	}
	return numberTable["und/latn"], false
}

// roundPrinter writes the digits of a number rounded as x/text does, in
// English and without grouping.
var roundPrinter = message.NewPrinter(language.English)

// write formats v (any type number.Decimal takes) with the symbols s and
// the digits (nil: ASCII).
func (s *numberSymbols) write(v any, percent bool, opts []number.Option, digits []string, noGrouping bool) string {
	opts = append(opts[:len(opts):len(opts)], number.NoSeparator())
	var text string
	if percent {
		text = roundPrinter.Sprint(number.Percent(v, opts...))
		text = strings.TrimSuffix(text, "%")
	} else {
		text = roundPrinter.Sprint(number.Decimal(v, opts...))
	}
	neg := strings.HasPrefix(text, "-")
	text = strings.TrimPrefix(text, "-")

	var body string
	switch text {
	case "NaN":
		body, neg = s.nan, false
	case "∞":
		body = s.infinity
	default:
		ip, frac, hasFrac := strings.Cut(text, ".")
		var b strings.Builder
		if noGrouping {
			b.WriteString(ip)
		} else {
			s.group3(&b, ip)
		}
		if hasFrac {
			b.WriteString("\x00") // the decimal separator, after the digits are mapped
			b.WriteString(frac)
		}
		body = b.String()
		if digits != nil {
			body = replaceDigits(body, digits)
		}
		body = strings.Replace(body, "\x00", s.decimal, 1)
	}

	var out strings.Builder
	if neg {
		out.WriteString(s.minus)
	}
	if percent {
		out.WriteString(s.pctPrefix)
	}
	out.WriteString(body)
	if percent {
		out.WriteString(s.pctSuffix)
	}
	return out.String()
}

// group3 writes the integer digits ip with the group separator: the
// primary size from the right, then the secondary one, only when ip has
// at least minGroup digits ahead of the last group.
func (s *numberSymbols) group3(b *strings.Builder, ip string) {
	p, sec := int(s.primary), int(s.secondary)
	if p == 0 || len(ip) < p+int(s.minGroup) {
		b.WriteString(ip)
		return
	}
	if sec == 0 {
		sec = p
	}
	// Positions (from the left) where a separator goes: before the last
	// p digits, then every sec digits.
	cut := len(ip) - p
	var starts []int
	for ; cut > 0; cut -= sec {
		starts = append(starts, cut)
	}
	prev := 0
	for i := len(starts) - 1; i >= 0; i-- {
		b.WriteString(ip[prev:starts[i]])
		b.WriteString(s.group)
		prev = starts[i]
	}
	b.WriteString(ip[prev:])
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
