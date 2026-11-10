package i18n

import (
	"compress/flate"
	"io"
	"sort"
	"strings"
	"sync"

	"golang.org/x/text/language"
)

// Currencies and compact notation (ADR-266), from currencies_gen.go and
// localenumbers_gen.go.

// localeText is a generated row of per-locale text: the locale and its
// entries.
type localeText struct{ loc, text string }

// compactRow is a generated row of compact patterns: "locale/system" and
// the short, long and currency texts ("": the next row).
type compactRow struct{ key, short, long, currency string }

// currencyNameRow is a generated row of currency names: the locale, the
// unit pattern ("": the ancestor's) and the names.
type currencyNameRow struct{ loc, unit, names string }

// currencyDigitsOf returns the digits of a currency's minor unit: 2 unless
// currencyDigits says otherwise (0 for JPY, 3 for BHD).
func currencyDigitsOf(code string) int {
	for i := 0; i+4 <= len(currencyDigits); i += 4 {
		if currencyDigits[i:i+3] == code {
			return int(currencyDigits[i+3] - '0')
		}
	}
	return 2
}

// regionCurrency returns the current currency of a region ("" if none).
func regionCurrency(region string) string {
	n := len(regionCurrencies) / 5
	i := sort.Search(n, func(i int) bool { return regionCurrencies[i*5:i*5+2] >= region })
	if i < n && regionCurrencies[i*5:i*5+2] == region {
		return regionCurrencies[i*5+2 : i*5+5]
	}
	return ""
}

// localeCurrency returns the currency of a locale, for messages'
// {n, number, currency}: its -u-cu- ("es-u-cu-usd"), else its region's
// (likely, as x/text's: "es" is Spain, EUR), else XXX (no currency), as
// ICU does.
func localeCurrency(t language.Tag) string {
	if cu := t.TypeForKey("cu"); len(cu) == 3 && isASCIILetters(cu) {
		return strings.ToUpper(cu)
	}
	r, _ := t.Region()
	if c := regionCurrency(r.String()); c != "" {
		return c
	}
	return "XXX"
}

// entry finds "\x1e<code>\x1f..." in a row's text and returns what
// follows the code, up to the next entry.
func entry(text, code string) (string, bool) {
	i := strings.Index(text, "\x1e"+code+"\x1f")
	if i < 0 {
		return "", false
	}
	s := text[i+len(code)+2:]
	if j := strings.IndexByte(s, '\x1e'); j >= 0 {
		s = s[:j]
	}
	return s, true
}

// symbolsRow returns the currency symbols row of a locale name.
func symbolsRow(loc string) (string, bool) {
	i := sort.Search(len(currencySymbols), func(i int) bool { return currencySymbols[i].loc >= loc })
	if i < len(currencySymbols) && currencySymbols[i].loc == loc {
		return currencySymbols[i].text, true
	}
	return "", false
}

// currencyText returns how a currency is shown: its symbol, narrow symbol
// or code (a currency without data is its code). Names are
// currencyName's.
func (d *locData) currencyText(code string, m CurrencyDisplayMode) string {
	if m == CurrencyCode {
		return code
	}
	for _, name := range append(localeChain(d.tag), "und") {
		row, ok := symbolsRow(name)
		if !ok {
			continue
		}
		e, ok := entry(row, code)
		if !ok {
			continue
		}
		sym, narrow, hasNarrow := strings.Cut(e, "\x1f")
		if m == CurrencyNarrowSymbol && hasNarrow {
			return narrow
		}
		return sym
	}
	return code
}

// pluralForm picks a plural form from a generated group: the "other"
// form, then "\x1f", the category's letter and the form of each category
// that differs from it.
func pluralForm(group, category string) string {
	if category != "other" {
		if s, ok := formOf(group, category[0]); ok {
			return s
		}
	}
	other, _, _ := strings.Cut(group, "\x1f")
	return other
}

// formOf returns the form of a group whose letter is c (z, o, t, f, m, 0
// or 1), if it has one.
func formOf(group string, c byte) (string, bool) {
	_, rest, ok := strings.Cut(group, "\x1f")
	if !ok {
		return "", false
	}
	for _, f := range strings.Split(rest, "\x1f") {
		if f != "" && f[0] == c {
			return f[1:], true
		}
	}
	return "", false
}

// generatedChain returns the generated locale of d and its generated
// ancestors by truncation (es-MX, es).
func (d *locData) generatedChain() []string {
	name := d.generatedLocale()
	chain := []string{name}
	for p := parentName(name); p != ""; p = parentName(p) {
		i := sort.SearchStrings(dateTags[:], p)
		if i < len(dateTags) && dateTags[i] == p {
			chain = append(chain, p)
		}
	}
	return chain
}

// parentName truncates a locale name by one subtag ("" for a language).
func parentName(name string) string {
	i := strings.LastIndexByte(name, '-')
	if i < 0 {
		return ""
	}
	return name[:i]
}

// inflatedNames are the names of currencyNames' rows once inflated.
var inflatedNames [len(currencyNames)]struct {
	once sync.Once
	text string
}

// nameRow returns the unit pattern and the inflated names of a generated
// locale.
func nameRow(loc string) (unit, names string, ok bool) {
	i := sort.Search(len(currencyNames), func(i int) bool { return currencyNames[i].loc >= loc })
	if i == len(currencyNames) || currencyNames[i].loc != loc {
		return "", "", false
	}
	in := &inflatedNames[i]
	in.once.Do(func() {
		if z := currencyNames[i].names; z != "" {
			b, err := io.ReadAll(flate.NewReader(strings.NewReader(z)))
			if err != nil {
				panic("i18n: currency names of " + loc + " are corrupt: " + err.Error())
			}
			in.text = string(b)
		}
	})
	return currencyNames[i].unit, in.text, true
}

// currencyName writes an amount with the currency's name: the number as
// a decimal with the currency's digits, in the locale's unit pattern
// ("{0} {1}"), both in the plural form of the number as shown.
func (d *locData) currencyName(n num, c numberConfig, nl *numberLocale) string {
	dc := c
	dc.style = styleDecimal
	var f formatted
	var text string
	if c.notation == NotationCompact && !n.nan && !n.inf {
		kind := compactShort
		if c.compactDisplay == CompactLong {
			kind = compactLong
		}
		var p *numberPattern
		f, p = d.compact(n, dc, nl, kind, true)
		if p == nil {
			p = nl.pattern(nl.decimalPat)
		}
		pre, suf := p.affixes(signOf(c.signDisplay, n, f.zero))
		text = nl.assemble(pre, f.body, suf, "", signOf(c.signDisplay, n, f.zero))
	} else {
		p := nl.pattern(nl.decimalPat)
		f = d.digitsOfAmount(n, dc, nl, p, c.currency)
		sign := signOf(c.signDisplay, n, f.zero)
		pre, suf := p.affixes(sign)
		text = nl.assemble(pre, f.body, suf, "", sign)
	}
	cat := pluralOf(d.tag, f.ip, f.fp)
	unit, name := "{0} {1}", c.currency
	unitFound, nameFound := false, false
	for _, loc := range d.generatedChain() {
		u, names, ok := nameRow(loc)
		if !ok {
			continue
		}
		if !unitFound && u != "" {
			unit, unitFound = pluralForm(u, cat), true
		}
		if !nameFound {
			if g, ok := entry(names, c.currency); ok {
				name, nameFound = pluralForm(g, cat), true
			}
		}
	}
	return strings.NewReplacer("{0}", text, "{1}", name).Replace(unit)
}

// digitsOfAmount is digitsOf for an amount written with the decimal
// pattern: the currency's digits and separators (de-AT groups amounts
// with "." and other numbers with a space).
func (d *locData) digitsOfAmount(n num, c numberConfig, nl *numberLocale, p *numberPattern, code string) formatted {
	if n.nan || n.inf || c.notation == NotationScientific || c.notation == NotationEngineering {
		return d.digitsOf(n, c, nl, p, true)
	}
	r := roundingOf(c, currencyDigitsOf(code), true)
	q := r.apply(n.dec, n.neg)
	ip, fp := r.fraction(q)
	g := groupingOf(c.grouping, p, int(nl.minGroup), false)
	return formatted{body: nl.body(ip, fp, g, true), zero: q.isZero(), ip: ip, fp: fp}
}

// ---------------------------------------------------------------------
// Compact notation.

// compactKind is a set of compact patterns.
type compactKind uint8

const (
	compactShort compactKind = iota
	compactLong
	compactCurrency
)

// compactTable is a locale's compact patterns of one kind: for each
// magnitude from 10^3, the generated group of plural forms ("": none, the
// number is written in full).
type compactTable []string

var compactTables sync.Map // "locale/system/kind" -> *compactTable

func compactRowOf(key string) *compactRow {
	i := sort.Search(len(compactPatterns), func(i int) bool { return compactPatterns[i].key >= key })
	if i < len(compactPatterns) && compactPatterns[i].key == key {
		return &compactPatterns[i]
	}
	return nil
}

// compactTableOf returns the compact patterns of a kind for the locale's
// generated locale and a numbering system: the system's under the
// locale's generated chain, else the Latin ones; as ICU, a locale without
// long patterns uses its short ones.
func (d *locData) compactTableOf(kind compactKind, sys string) compactTable {
	chain := d.generatedChain()
	key := chain[0] + "/" + sys + "/" + string('0'+rune(kind))
	if v, ok := compactTables.Load(key); ok {
		return v.(compactTable)
	}
	find := func(kind compactKind) string {
		for _, s := range []string{sys, "latn"} {
			for _, loc := range chain {
				r := compactRowOf(loc + "/" + s)
				if r == nil {
					continue
				}
				text := r.currency
				switch kind {
				case compactShort:
					text = r.short
				case compactLong:
					text = r.long
				}
				if text != "" {
					return text
				}
			}
		}
		return ""
	}
	text := find(kind)
	if text == "" && kind == compactLong {
		text = find(compactShort)
	}
	t := compactTable(strings.Split(text, "\x1e"))
	v, _ := compactTables.LoadOrStore(key, t)
	return v.(compactTable)
}

// zerosOf counts the zeros of a compact pattern's positive subpattern,
// outside quotes.
func zerosOf(p string) int {
	pos, _, _ := strings.Cut(p, ";")
	n, quoted := 0, false
	for i := 0; i < len(pos); i++ {
		switch pos[i] {
		case '\'':
			quoted = !quoted
		case '0':
			if !quoted {
				n++
			}
		}
	}
	return n
}

// group returns the plural forms of a magnitude: its own, or the
// largest's above it ("" below 10^3).
func (t compactTable) group(mag int) string {
	if mag < 3 || len(t) == 0 {
		return ""
	}
	return t[min(mag-3, len(t)-1)]
}

// shiftFor returns the power of ten a number of magnitude mag is divided
// by in compact notation: from the "other" pattern of its magnitude (or
// the largest), 0 below the first, for a magnitude without patterns or
// for a "0" pattern (no abbreviation).
func (t compactTable) shiftFor(mag int) int {
	other := pluralForm(t.group(mag), "other")
	if other == "0" || other == "" {
		return 0
	}
	return min(mag, len(t)+2) - (zerosOf(other) - 1)
}

// compact writes a number in compact notation: the number divided by the
// power of ten of its magnitude's pattern, rounded, and the pattern of the
// plural form it shows (nil when the number has no abbreviation). As
// ICU's, a rounding that reaches the next magnitude (999,999 to 1000K)
// uses that magnitude's pattern (1M).
func (d *locData) compact(n num, c numberConfig, nl *numberLocale, kind compactKind, amount bool) (formatted, *numberPattern) {
	t := d.compactTableOf(kind, nl.sys)
	r := roundingOf(c, 0, false)
	v := n.dec.shift(c.scale)
	mag := v.magnitude()
	shift := t.shiftFor(mag)
	q := r.apply(v.shift(-shift), n.neg)
	if !q.isZero() && q.magnitude()+shift > mag {
		if s := t.shiftFor(mag + 1); s != shift {
			shift = s
			q = r.apply(v.shift(-shift), n.neg)
		}
	}
	ip, fp := r.fraction(q)
	plain := nl.pattern(nl.decimalPat)
	g := groupingOf(c.grouping, plain, int(nl.minGroup), true)
	f := formatted{body: nl.body(ip, fp, g, amount), zero: q.isZero(), ip: ip, fp: fp}
	final := q.magnitude() + shift
	if q.isZero() || shift == 0 || final < 3 {
		return f, nil
	}
	group := t.group(final)
	src := pluralForm(group, pluralOf(d.tag, ip, fp))
	if !n.neg && string(q.digits) == "1" && q.exp == 1 {
		if s, ok := formOf(group, '1'); ok {
			src = s // CLDR's explicit 1: "mille" in French
		}
	}
	if src == "0" || src == "" {
		return f, nil
	}
	if zerosOf(src) == 0 {
		f.body = "" // the pattern is the whole text
	}
	return f, parseCompact(src)
}

var compactPatternCache sync.Map // pattern -> *numberPattern

// parseCompact parses a CLDR compact pattern ("0K", "¤0K", "0 Mio'.'",
// "elfu 0;elfu -0") into affixes with markers.
func parseCompact(src string) *numberPattern {
	if v, ok := compactPatternCache.Load(src); ok {
		return v.(*numberPattern)
	}
	sub := func(s string, neg bool) (prefix, suffix string) {
		var pre, post strings.Builder
		state := 0 // 0 prefix, 1 number, 2 suffix
		add := func(s string) {
			if state == 0 {
				pre.WriteString(s)
			} else {
				state = 2
				post.WriteString(s)
			}
		}
		for i := 0; i < len(s); i++ {
			ch := s[i]
			switch {
			case ch == '\'':
				if i+1 < len(s) && s[i+1] == '\'' {
					add("'")
					i++
					continue
				}
				j := strings.IndexByte(s[i+1:], '\'')
				if j < 0 {
					add(s[i+1:])
					i = len(s)
					continue
				}
				add(s[i+1 : i+1+j])
				i += j + 1
			case ch == '0' && state < 2:
				state = 1
			case ch == '-' && neg:
				add(string(markSign))
			case strings.HasPrefix(s[i:], "¤"):
				add(string(markCurrency))
				i += len("¤") - 1
			default:
				add(s[i : i+1])
			}
		}
		return pre.String(), post.String()
	}
	pos, neg, hasNeg := strings.Cut(src, ";")
	p := &numberPattern{}
	p.posPrefix, p.posSuffix = sub(pos, false)
	if hasNeg {
		p.negPrefix, p.negSuffix = sub(neg, true)
	} else {
		p.negPrefix, p.negSuffix = string(markSign)+p.posPrefix, p.posSuffix
	}
	v, _ := compactPatternCache.LoadOrStore(src, p)
	return v.(*numberPattern)
}
