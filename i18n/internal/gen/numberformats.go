package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The number symbols and patterns (ADR-259, ADR-266). golang.org/x/text
// formats numbers with CLDR 32, whose symbols differ from CLDR 48's (the
// minus sign of Persian and Urdu, the bidi marks around negatives, Nepali
// grouping, minimum grouping digits). The generator reads the symbols and
// the standard decimal, percent, currency and accounting patterns of every
// CLDR locale and every numbering system the locale has data for, vendored
// as numberFormats.json, and emits numberformats_gen.go: the (locale,
// system) pairs whose data differs from what their parent resolves to, or
// that no parent has, and the distinct patterns they point to.

// numberFormatsFile is the vendored digest of main.<locale>.numbers, relative
// to the cldr directory: {"<locale>": numberDigest}, one locale per line.
const numberFormatsFile = "numberFormats.json"

// numberSystemDigest is what is kept of the symbols and the standard
// patterns of one numbering system of a locale, under CLDR's own names.
// In the root locale's arab and arabext systems, which come from CLDR's
// root.xml (cldr-json publishes only its Latin data), an empty pattern or
// currencySpacing is root.xml's alias to the Latin data of the locale that
// asks (source="locale").
type numberSystemDigest struct {
	Decimal         string `json:"decimal"`
	Group           string `json:"group"`
	MinusSign       string `json:"minusSign"`
	PlusSign        string `json:"plusSign"`
	PercentSign     string `json:"percentSign"`
	Exponential     string `json:"exponential"`
	Infinity        string `json:"infinity"`
	NaN             string `json:"nan"`
	CurrencyDecimal string `json:"currencyDecimal,omitempty"`
	CurrencyGroup   string `json:"currencyGroup,omitempty"`
	DecimalFormat   string `json:"decimalFormat"`
	PercentFormat   string `json:"percentFormat"`
	// No scientificFormats: ICU's NumberFormatter, which Intl uses, writes
	// scientific notation without them (gu's "[#E0]" gives 1.235E4).
	CurrencyFormat   string `json:"currencyFormat"`
	AccountingFormat string `json:"accountingFormat"`
	// CurrencySpacing is currencySpacing's insertBetween, the same before
	// and after the symbol, with CLDR's standard matches (checked when
	// fetched).
	CurrencySpacing string `json:"currencySpacing"`
}

// numberDigest is what is kept of a locale's main.<locale>.numbers.
type numberDigest struct {
	MinimumGroupingDigits string                        `json:"minimumGroupingDigits"`
	Systems               map[string]numberSystemDigest `json:"systems"`
}

// The currencySpacing matches every CLDR 48 locale has, which the runtime
// implements: a symbol whose end next to the number is neither a symbol
// (S) nor a separator (Z), next to a digit.
const (
	standardCurrencyMatch    = "[[:^S:]&[:^Z:]]"
	standardSurroundingMatch = "[:digit:]"
)

// digestNumbers extracts the numberDigest from a locale's numbers object.
func digestNumbers(nums map[string]any) (numberDigest, error) {
	d := numberDigest{Systems: map[string]numberSystemDigest{}}
	d.MinimumGroupingDigits, _ = nums["minimumGroupingDigits"].(string)
	if d.MinimumGroupingDigits == "" {
		d.MinimumGroupingDigits = "1"
	}
	const symbolsPrefix = "symbols-numberSystem-"
	for key, v := range nums {
		sys, ok := strings.CutPrefix(key, symbolsPrefix)
		if !ok {
			continue
		}
		syms, ok := v.(map[string]any)
		if !ok {
			return d, fmt.Errorf("%s is not an object", key)
		}
		str := func(m map[string]any, where, k string) (string, error) {
			s, ok := m[k].(string)
			if !ok {
				return "", fmt.Errorf("%s: no %q", where, k)
			}
			return s, nil
		}
		var sd numberSystemDigest
		var err error
		for _, f := range []struct {
			dst *string
			key string
		}{
			{&sd.Decimal, "decimal"}, {&sd.Group, "group"}, {&sd.MinusSign, "minusSign"},
			{&sd.PlusSign, "plusSign"}, {&sd.PercentSign, "percentSign"},
			{&sd.Exponential, "exponential"}, {&sd.Infinity, "infinity"}, {&sd.NaN, "nan"},
		} {
			if *f.dst, err = str(syms, key, f.key); err != nil {
				return d, err
			}
		}
		sd.CurrencyDecimal, _ = syms["currencyDecimal"].(string)
		sd.CurrencyGroup, _ = syms["currencyGroup"].(string)
		formats := func(kind string) (map[string]any, error) {
			k := kind + "-numberSystem-" + sys
			m, ok := nums[k].(map[string]any)
			if !ok {
				return nil, fmt.Errorf("symbols for %s but no %s", sys, k)
			}
			return m, nil
		}
		for _, f := range []struct {
			dst       *string
			kind, key string
		}{
			{&sd.DecimalFormat, "decimalFormats", "standard"},
			{&sd.PercentFormat, "percentFormats", "standard"},
			{&sd.CurrencyFormat, "currencyFormats", "standard"},
			{&sd.AccountingFormat, "currencyFormats", "accounting"},
		} {
			m, err := formats(f.kind)
			if err != nil {
				return d, err
			}
			if *f.dst, err = str(m, f.kind+"-numberSystem-"+sys, f.key); err != nil {
				return d, err
			}
		}
		cf, _ := formats("currencyFormats")
		if sd.CurrencySpacing, err = currencySpacing(cf["currencySpacing"]); err != nil {
			return d, fmt.Errorf("currencyFormats-numberSystem-%s: %v", sys, err)
		}
		d.Systems[sys] = sd
	}
	if len(d.Systems) == 0 {
		return d, fmt.Errorf("no symbols")
	}
	return d, nil
}

// currencySpacing checks a currencySpacing object and returns its
// insertBetween.
func currencySpacing(v any) (string, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", fmt.Errorf("no currencySpacing")
	}
	var insert string
	for i, side := range []string{"beforeCurrency", "afterCurrency"} {
		s, ok := m[side].(map[string]any)
		if !ok {
			return "", fmt.Errorf("currencySpacing: no %s", side)
		}
		if s["currencyMatch"] != standardCurrencyMatch || s["surroundingMatch"] != standardSurroundingMatch {
			return "", fmt.Errorf("currencySpacing.%s has matches %v and %v; the runtime implements only %s and %s",
				side, s["currencyMatch"], s["surroundingMatch"], standardCurrencyMatch, standardSurroundingMatch)
		}
		ins, _ := s["insertBetween"].(string)
		if i == 1 && ins != insert {
			return "", fmt.Errorf("currencySpacing inserts %q before and %q after the symbol", insert, ins)
		}
		insert = ins
	}
	return insert, nil
}

// writeLines writes a JSON object sorted, one key per line.
func writeLines[V any](path string, m map[string]V) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, k := range keys {
		line, err := marshalNoEscape(m[k])
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, " %q: %s", k, line)
		if i < len(keys)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// marshalNoEscape is json.Marshal without HTML escaping (the patterns
// have no <, > or &, but keeping the text as is costs nothing).
func marshalNoEscape(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// Markers in the affixes of a parsed pattern, which the runtime replaces
// (numberformat.go): the currency, the sign (the minus sign of an explicit
// negative subpattern, or the plus sign) and the percent sign.
const (
	markCurrency = "\x01"
	markSign     = "\x02"
	markPercent  = "\x03"
)

// pattern is a parsed CLDR number pattern; it mirrors i18n's
// numberPattern. The negative affixes are the explicit negative
// subpattern's, else the sign marker before the positive prefix.
type pattern struct {
	posPrefix, posSuffix, negPrefix, negSuffix string
	primary, secondary                         int
}

// parseSubpattern reads one subpattern ("#,##0.###", "¤#,##0.00",
// "% #,##0", "(¤#,##0.00)"): its affixes, with markers, and the grouping
// sizes of its number. Padding, per mille, significant digits and
// exponents fail.
func parseSubpattern(p string) (prefix, suffix string, primary, secondary int, err error) {
	var pre, post, num strings.Builder
	state := 0 // 0 prefix, 1 number, 2 suffix
	add := func(s string) {
		switch state {
		case 0:
			pre.WriteString(s)
		default:
			state = 2
			post.WriteString(s)
		}
	}
	rs := []rune(p)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'':
			if i+1 < len(rs) && rs[i+1] == '\'' {
				add("'")
				i++
				continue
			}
			j := i + 1
			for j < len(rs) && rs[j] != '\'' {
				j++
			}
			if j == len(rs) {
				return "", "", 0, 0, fmt.Errorf("pattern %q: unterminated quote", p)
			}
			add(string(rs[i+1 : j]))
			i = j
		case state < 2 && strings.ContainsRune("#0,.", c):
			state = 1
			num.WriteRune(c)
		case c == '%':
			add(markPercent)
		case c == '¤':
			add(markCurrency)
		case c == '-':
			add(markSign)
		case strings.ContainsRune("‰*@E+;", c):
			return "", "", 0, 0, fmt.Errorf("pattern %q: unsupported %q", p, c)
		default:
			add(string(c))
		}
	}
	if num.Len() == 0 {
		return "", "", 0, 0, fmt.Errorf("pattern %q: no number", p)
	}
	intPart, _, _ := strings.Cut(num.String(), ".")
	last := strings.LastIndex(intPart, ",")
	if last >= 0 {
		primary = len(intPart) - last - 1
		secondary = primary
		if prev := strings.LastIndex(intPart[:last], ","); prev >= 0 {
			secondary = last - prev - 1
		}
	}
	return pre.String(), post.String(), primary, secondary, nil
}

// parsePattern parses a pattern with an optional negative subpattern. The
// positive subpattern may not have a sign; the grouping comes from it.
func parsePattern(p string) (pattern, error) {
	pos, neg, hasNeg := strings.Cut(p, ";")
	var r pattern
	var err error
	if r.posPrefix, r.posSuffix, r.primary, r.secondary, err = parseSubpattern(pos); err != nil {
		return r, err
	}
	if strings.Contains(r.posPrefix+r.posSuffix, markSign) {
		return r, fmt.Errorf("pattern %q: a sign in the positive subpattern", p)
	}
	if !hasNeg {
		r.negPrefix, r.negSuffix = markSign+r.posPrefix, r.posSuffix
		return r, nil
	}
	if r.negPrefix, r.negSuffix, _, _, err = parseSubpattern(neg); err != nil {
		return r, err
	}
	return r, nil
}

// numberRec is a row of the generated table; it mirrors i18n's
// numberSymbols. A pattern index of 0, a spacing of "" and a minGroup of 0
// stand for the Latin data of the locale (root.xml's aliases).
type numberRec struct {
	decimal, group, minus, plus, percent, exponential, infinity, nan string
	currencyDecimal, currencyGroup, spacing                          string
	decimalPat, percentPat, currencyPat, accountingPat               int
	minGroup                                                         int
}

// patternPool numbers the distinct patterns, from 1.
type patternPool struct {
	index map[pattern]int
	list  []pattern
}

func (pp *patternPool) add(src string) (int, error) {
	if src == "" {
		return 0, nil
	}
	p, err := parsePattern(src)
	if err != nil {
		return 0, err
	}
	if pp.index == nil {
		pp.index = map[pattern]int{}
	}
	if i, ok := pp.index[p]; ok {
		return i, nil
	}
	pp.list = append(pp.list, p)
	pp.index[p] = len(pp.list)
	return len(pp.list), nil
}

// numberRecord makes the table row of a locale's numbering system.
func numberRecord(minGroup string, sd numberSystemDigest, pp *patternPool) (numberRec, error) {
	r := numberRec{
		decimal: sd.Decimal, group: sd.Group, minus: sd.MinusSign, plus: sd.PlusSign,
		percent: sd.PercentSign, exponential: sd.Exponential, infinity: sd.Infinity, nan: sd.NaN,
		currencyDecimal: sd.CurrencyDecimal, currencyGroup: sd.CurrencyGroup, spacing: sd.CurrencySpacing,
	}
	if minGroup != "" {
		n, err := strconv.Atoi(minGroup)
		if err != nil || n < 1 || n > 9 {
			return r, fmt.Errorf("minimumGroupingDigits %q", minGroup)
		}
		r.minGroup = n
	}
	var err error
	for _, f := range []struct {
		dst  *int
		src  string
		kind string
	}{
		{&r.decimalPat, sd.DecimalFormat, "decimal"},
		{&r.percentPat, sd.PercentFormat, "percent"},
		{&r.currencyPat, sd.CurrencyFormat, "currency"},
		{&r.accountingPat, sd.AccountingFormat, "accounting"},
	} {
		if *f.dst, err = pp.add(f.src); err != nil {
			return r, fmt.Errorf("%s pattern: %v", f.kind, err)
		}
		if *f.dst == 0 {
			continue
		}
		p := pp.list[*f.dst-1]
		affixes := p.posPrefix + p.posSuffix + p.negPrefix + p.negSuffix
		switch f.kind {
		case "decimal":
			if p.posPrefix != "" || p.posSuffix != "" || p.negPrefix != markSign || p.negSuffix != "" {
				return r, fmt.Errorf("decimal pattern %q has affixes", f.src)
			}
		case "percent":
			if strings.Contains(affixes, markCurrency) || !strings.Contains(p.posPrefix+p.posSuffix, markPercent) {
				return r, fmt.Errorf("percent pattern %q", f.src)
			}
		default:
			if strings.Contains(affixes, markPercent) || !strings.Contains(p.posPrefix+p.posSuffix, markCurrency) {
				return r, fmt.Errorf("%s pattern %q", f.kind, f.src)
			}
		}
	}
	return r, nil
}

// generateNumberFormats returns numberformats_gen.go.
func generateNumberFormats(dir string) ([]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, numberFormatsFile))
	if err != nil {
		return nil, err
	}
	var digests map[string]numberDigest
	if err := json.Unmarshal(raw, &digests); err != nil {
		return nil, fmt.Errorf("%s: %v", numberFormatsFile, err)
	}
	if _, ok := digests["und"]; !ok {
		return nil, fmt.Errorf("%s: no \"und\" (the root locale)", numberFormatsFile)
	}
	names := parentsFirst(digests)
	var pool patternPool
	table := map[string]numberRec{}
	// resolve is what the runtime lookup finds for a locale and a system:
	// the locale, its parents and the root, if one of them has data for
	// the system (the runtime tells a system with data from one without,
	// which it formats as Latin, so a row equal to the Latin one stays).
	resolve := func(name, sys string) (numberRec, bool) {
		for n := name; n != ""; n = parentOf(n) {
			if r, ok := table[n+"/"+sys]; ok {
				return r, true
			}
		}
		r, ok := table["und/"+sys]
		return r, ok
	}
	for _, loc := range names {
		d := digests[loc]
		systems := make([]string, 0, len(d.Systems))
		for sys := range d.Systems {
			systems = append(systems, sys)
		}
		sort.Strings(systems)
		for _, sys := range systems {
			minGroup := d.MinimumGroupingDigits
			if loc == "und" && sys != "latn" {
				minGroup = "" // root.xml's systems: the locale's own
			}
			rec, err := numberRecord(minGroup, d.Systems[sys], &pool)
			if err != nil {
				return nil, fmt.Errorf("%s (%s): %v", loc, sys, err)
			}
			if inherited, ok := resolve(parentOf(loc), sys); ok && loc != "und" && rec == inherited {
				continue
			}
			table[loc+"/"+sys] = rec
		}
	}
	if len(pool.list) > 255 {
		return nil, fmt.Errorf("%d distinct patterns do not fit uint8 indices", len(pool.list))
	}

	var b bytes.Buffer
	writeHeader(&b)
	b.WriteString("// numberPatterns are the distinct standard decimal, percent, currency and\n")
	b.WriteString("// accounting patterns of numberTable; index 0 is none (the Latin pattern of\n")
	b.WriteString("// the locale). The fields are numberPattern's: the positive and negative\n")
	b.WriteString("// prefixes and suffixes, with \\x01 for the currency, \\x02 for the sign and\n")
	b.WriteString("// \\x03 for the percent sign, and the primary and secondary grouping sizes.\n")
	b.WriteString("var numberPatterns = [...]numberPattern{\n\t{},\n")
	for _, p := range pool.list {
		fmt.Fprintf(&b, "\t{%q, %q, %q, %q, %d, %d},\n", p.posPrefix, p.posSuffix, p.negPrefix, p.negSuffix, p.primary, p.secondary)
	}
	b.WriteString("}\n\n")
	b.WriteString("// numberTable has the number symbols and patterns of the (locale, numbering\n")
	b.WriteString("// system) pairs, keyed \"locale/system\", whose data differs from what their\n")
	b.WriteString("// parent (language-script-region, language-script, language, und) resolves to,\n")
	b.WriteString("// or that no parent has data for.\n")
	b.WriteString("// The fields are numberSymbols': decimal, group, minus, plus, percent sign,\n")
	b.WriteString("// exponential, infinity, NaN, the currency decimal and group (\"\": the plain\n")
	b.WriteString("// ones), the currency spacing, the indices in numberPatterns of the decimal,\n")
	b.WriteString("// percent, currency and accounting patterns, and the minimum grouping digits.\n")
	b.WriteString("// In the root's arab and arabext rows, which come from CLDR's root.xml, a\n")
	b.WriteString("// pattern 0, a spacing \"\" and a minimum 0 are the Latin data of the locale.\n")
	b.WriteString("var numberTable = map[string]numberSymbols{\n")
	keys := make([]string, 0, len(table))
	for k := range table {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r := table[k]
		fmt.Fprintf(&b, "\t%q: {%q, %q, %q, %q, %q, %q, %q, %q, %q, %q, %q, %d, %d, %d, %d, %d},\n",
			k, r.decimal, r.group, r.minus, r.plus, r.percent, r.exponential, r.infinity, r.nan,
			r.currencyDecimal, r.currencyGroup, r.spacing,
			r.decimalPat, r.percentPat, r.currencyPat, r.accountingPat, r.minGroup)
	}
	b.WriteString("}\n")
	return format.Source(b.Bytes())
}

// parentsFirst returns the keys of a table of locales with the root
// first, then by depth, then by name, so a locale comes after its
// parents.
func parentsFirst[V any](m map[string]V) []string {
	names := make([]string, 0, len(m))
	for loc := range m {
		names = append(names, loc)
	}
	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "und") != (names[j] == "und") {
			return names[i] == "und"
		}
		di, dj := strings.Count(names[i], "-"), strings.Count(names[j], "-")
		if di != dj {
			return di < dj
		}
		return names[i] < names[j]
	})
	return names
}

// writeHeader writes the header of a generated file.
func writeHeader(b *bytes.Buffer) {
	b.WriteString("// Code generated by internal/gen from the Unicode CLDR JSON data in\n")
	b.WriteString("// internal/gen/cldr; DO NOT EDIT.\n")
	b.WriteString("// Data files copyright (c) 1991-2025 Unicode, Inc. (Unicode License v3,\n")
	b.WriteString("// see internal/gen/cldr/LICENSE and THIRD_PARTY.md).\n\n")
	b.WriteString("package i18n\n\n")
}
