package i18n

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
)

// Number formatting (ADR-259, ADR-266). golang.org/x/text has CLDR 32's
// symbols and patterns, which differ from ICU's (CLDR 48): the minus sign
// of Persian and Urdu, the bidi marks around negatives, Nepali grouping,
// the minimum grouping digits of Spanish and Polish... and it has no
// currencies, signs or notations. So numbers are formatted here, as
// ICU's NumberFormatter does with Intl.NumberFormat's defaults, from the
// CLDR 48 tables of numberformats_gen.go (symbols and patterns of every
// locale), currencies_gen.go and localenumbers_gen.go: the value is turned
// into its shortest decimal digits (strconv's, like ICU's
// double-conversion), rounded half to even, and written with the locale's
// digits, symbols and the affixes of its pattern.

// numberSymbols is a generated row: what a locale writes numbers with in
// one numbering system. Its patterns are indices in numberPatterns.
type numberSymbols struct {
	decimal, group, minus, plus, percent, exponential, infinity, nan string
	currencyDecimal, currencyGroup, spacing                          string
	decimalPat, percentPat, currencyPat, accountingPat               uint8
	minGroup                                                         uint8
}

// numberPattern is a parsed CLDR pattern: the affixes of positive and
// negative numbers, with markers (\x01 the currency, \x02 the sign, \x03
// the percent sign), and the primary and secondary grouping sizes (0: no
// grouping).
type numberPattern struct {
	posPrefix, posSuffix, negPrefix, negSuffix string
	primary, secondary                         uint8
}

const (
	markCurrency = '\x01'
	markSign     = '\x02'
	markPercent  = '\x03'
)

// numberLocale is how a locale writes numbers in its numbering system: a
// row with no inherited field left, and the digits (nil: ASCII).
type numberLocale struct {
	numberSymbols
	sys    string
	digits []string
}

func (nl *numberLocale) pattern(i uint8) *numberPattern { return &numberPatterns[i] }

// lookupRow returns the generated row of a tag in a numbering system: the
// first of the tag's names (localeChain) and the root that has one.
func lookupRow(t language.Tag, sys string) (numberSymbols, bool) {
	for _, name := range localeChain(t) {
		if s, ok := numberTable[name+"/"+sys]; ok {
			return s, true
		}
	}
	s, ok := numberTable["und/"+sys]
	return s, ok
}

// newNumberLocale resolves how a tag writes numbers in a numbering system
// (sys "": the tag's own, numberingOf). As ICU does, the symbols are the
// system's under the tag's names, else the root's (arab and arabext), else
// the tag's Latin ones; a field the root leaves to the locale (root.xml's
// aliases) is the tag's Latin one.
func newNumberLocale(t language.Tag, sys string) *numberLocale {
	sys = numberingOf(t, sys)
	latn, _ := lookupRow(t, "latn")
	row, ok := lookupRow(t, sys)
	if !ok {
		row = latn
	}
	if row.decimalPat == 0 {
		row.decimalPat = latn.decimalPat
	}
	if row.percentPat == 0 {
		row.percentPat = latn.percentPat
	}
	if row.currencyPat == 0 {
		row.currencyPat = latn.currencyPat
	}
	if row.accountingPat == 0 {
		row.accountingPat = latn.accountingPat
	}
	if row.spacing == "" {
		row.spacing = latn.spacing
	}
	if row.minGroup == 0 {
		row.minGroup = latn.minGroup
	}
	return &numberLocale{numberSymbols: row, sys: sys, digits: digitsOf(sys)}
}

var numberLocales sync.Map // "tag\x00sys" -> *numberLocale

// numbersIn returns how the locale writes numbers in a numbering system
// (a skeleton's numbering-system), cached.
func (d *locData) numbersIn(sys string) *numberLocale {
	if sys == "" || sys == d.numbers.sys {
		return d.numbers
	}
	key := d.name + "\x00" + sys
	if v, ok := numberLocales.Load(key); ok {
		return v.(*numberLocale)
	}
	v, _ := numberLocales.LoadOrStore(key, newNumberLocale(d.tag, sys))
	return v.(*numberLocale)
}

// replaceDigits writes the ASCII digits of s with digits (nil: as they
// are).
func replaceDigits(s string, digits []string) string {
	if digits == nil {
		return s
	}
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

// ---------------------------------------------------------------------
// Decimal numbers.

// decimal is a non-negative decimal number: 0.digits × 10^exp, with
// ASCII digits and no leading or trailing zero; no digits is zero.
type decimal struct {
	digits []byte
	exp    int
}

func (d decimal) isZero() bool { return len(d.digits) == 0 }

// magnitude is the power of ten of the leading digit (0 for zero).
func (d decimal) magnitude() int {
	if d.isZero() {
		return 0
	}
	return d.exp - 1
}

// shift multiplies d by 10^n.
func (d decimal) shift(n int) decimal {
	if d.isZero() {
		return d
	}
	return decimal{d.digits, d.exp + n}
}

// roundAt rounds d, the absolute value of a number that is negative when
// neg is set, to a multiple of 10^pos in a rounding mode.
func (d decimal) roundAt(pos int, m Rounding, neg bool) decimal {
	keep := d.exp - pos // digits kept
	if d.isZero() || keep >= len(d.digits) {
		return d
	}
	// What is dropped (never zero: d has no trailing zeros) against half
	// of 10^pos.
	half := -1
	if keep >= 0 {
		switch first := d.digits[keep]; {
		case first > '5' || first == '5' && len(d.digits) > keep+1:
			half = 1
		case first == '5':
			half = 0
		}
	}
	odd := keep > 0 && (d.digits[keep-1]-'0')%2 == 1
	if !roundsAway(m, neg, half, odd) {
		if keep <= 0 {
			return decimal{}
		}
		return decimal{d.digits[:keep], d.exp}.trimmed()
	}
	var kept []byte
	exp := d.exp
	if keep > 0 {
		kept = append(kept, d.digits[:keep]...)
	} else {
		exp = pos // 1 × 10^pos once incremented
	}
	i := len(kept) - 1
	for i >= 0 && kept[i] == '9' {
		kept[i] = '0'
		i--
	}
	if i < 0 {
		kept = append([]byte{'1'}, kept...)
		exp++
	} else {
		kept[i]++
	}
	return decimal{kept, exp}.trimmed()
}

// trimmed drops the trailing zeros of d's digits.
func (d decimal) trimmed() decimal {
	k := d.digits
	for len(k) > 0 && k[len(k)-1] == '0' {
		k = k[:len(k)-1]
	}
	if len(k) == 0 {
		return decimal{}
	}
	return decimal{k, d.exp}
}

// roundsAway says whether a rounding mode moves a number away from zero
// (increments the kept digits of its absolute value): half compares what
// is dropped with half a unit (-1 below, 0 exactly, 1 above), odd is
// whether the last kept digit is odd, neg whether the number is negative
// (ceil and floor go toward an infinity, not away from zero).
func roundsAway(m Rounding, neg bool, half int, odd bool) bool {
	switch m {
	case RoundCeil:
		return !neg
	case RoundFloor:
		return neg
	case RoundExpand:
		return true
	case RoundTrunc:
		return false
	}
	if half != 0 {
		return half > 0
	}
	switch m {
	case RoundHalfCeil:
		return !neg
	case RoundHalfFloor:
		return neg
	case RoundHalfExpand:
		return true
	case RoundHalfTrunc:
		return false
	}
	return odd // RoundHalfEven
}

// split returns the integer digits (at least "0") and the fraction digits.
func (d decimal) split() (ip, fp string) {
	s := string(d.digits)
	switch {
	case d.isZero():
		return "0", ""
	case d.exp <= 0:
		return "0", strings.Repeat("0", -d.exp) + s
	case d.exp >= len(s):
		return s + strings.Repeat("0", d.exp-len(s)), ""
	}
	return s[:d.exp], s[d.exp:]
}

// num is a number to format: its sign, NaN or infinity, and its absolute
// value.
type num struct {
	neg, nan, inf bool
	dec           decimal
}

// decimalOf parses the digits of an unsigned integer or of strconv's
// exponent format ("1.2345e+03").
func decimalOf(mantissa string, exp int) decimal {
	digits := []byte(strings.Replace(mantissa, ".", "", 1))
	lead := 0
	for lead < len(digits) && digits[lead] == '0' {
		lead++
	}
	digits = digits[lead:]
	exp -= lead
	for len(digits) > 0 && digits[len(digits)-1] == '0' {
		digits = digits[:len(digits)-1]
	}
	if len(digits) == 0 {
		return decimal{}
	}
	return decimal{digits, exp}
}

// toNum converts a number argument: the int64, uint64 and float64 of
// toNumber (message.go), and the other built-in numeric types.
func toNum(v any) num {
	switch x := v.(type) {
	case int64:
		if x < 0 {
			u := uint64(-(x + 1)) + 1 // -math.MinInt64 overflows
			s := strconv.FormatUint(u, 10)
			return num{neg: true, dec: decimalOf(s, len(s))}
		}
		s := strconv.FormatInt(x, 10)
		return num{dec: decimalOf(s, len(s))}
	case uint64:
		s := strconv.FormatUint(x, 10)
		return num{dec: decimalOf(s, len(s))}
	case float64:
		n := num{neg: math.Signbit(x)}
		switch {
		case math.IsNaN(x):
			return num{nan: true}
		case math.IsInf(x, 0):
			n.inf = true
			return n
		case x == 0:
			return n
		}
		s := strconv.FormatFloat(math.Abs(x), 'e', -1, 64)
		mant, e, _ := strings.Cut(s, "e")
		ev, _ := strconv.Atoi(e)
		n.dec = decimalOf(mant, ev+1)
		return n
	}
	if nv, ok := toNumber(v); ok {
		return toNum(nv.orig)
	}
	return num{nan: true}
}

// ---------------------------------------------------------------------
// Rounding.

// rounding is how a number is rounded and padded: fraction digits, or
// significant digits, or (compact notation's default, Intl's
// roundingPriority "morePrecision") both, keeping the more precise.
type rounding struct {
	minFrac, maxFrac int
	minSig, maxSig   int // 0: fraction digits only
	morePrecision    bool
	mode             Rounding
}

// roundingOf resolves the digits of a configuration as ECMA-402's
// SetNumberFormatDigitOptions does; currencyDigits are the currency's
// minor unit digits when the number is an amount.
func roundingOf(c numberConfig, currencyDigits int, isAmount bool) rounding {
	if c.minSig > 0 || c.maxSig > 0 {
		lo, hi := max(c.minSig, 1), c.maxSig
		if hi == 0 {
			hi = 21
		}
		return rounding{mode: c.rounding, minSig: lo, maxSig: max(hi, lo)}
	}
	minDef, maxDef := 0, 3
	switch {
	case isAmount && c.notation == NotationStandard:
		minDef, maxDef = currencyDigits, currencyDigits
	case c.style == stylePercent:
		maxDef = 0
	}
	lo, hi := c.minFrac, c.maxFrac
	switch {
	case lo < 0 && hi < 0:
		if c.notation == NotationCompact {
			return rounding{mode: c.rounding, maxSig: 2, minSig: 1, morePrecision: true}
		}
		lo, hi = minDef, maxDef
	case lo < 0:
		lo = min(minDef, hi)
	case hi < 0:
		hi = max(maxDef, lo)
	}
	return rounding{mode: c.rounding, minFrac: lo, maxFrac: max(hi, lo)}
}

// apply rounds d.
func (r rounding) apply(d decimal, neg bool) decimal {
	switch {
	case r.morePrecision:
		return d.roundAt(min(-r.maxFrac, d.magnitude()-r.maxSig+1), r.mode, neg)
	case r.maxSig > 0:
		return d.roundAt(d.magnitude()-r.maxSig+1, r.mode, neg)
	}
	return d.roundAt(-r.maxFrac, r.mode, neg)
}

// fraction returns the fraction digits of a rounded number, padded to the
// minimum.
func (r rounding) fraction(d decimal) (ip, fp string) {
	ip, fp = d.split()
	n := r.minFrac
	if r.maxSig > 0 && !r.morePrecision {
		n = r.minSig - d.magnitude() - 1
	}
	if len(fp) < n {
		fp += strings.Repeat("0", n-len(fp))
	}
	return ip, fp
}

// ---------------------------------------------------------------------
// Writing.

// signKind is the sign a formatted number shows.
type signKind uint8

const (
	signNone signKind = iota
	signMinus
	signPlus
)

// signOf applies a sign display to a number that rounds to zero or not.
func signOf(m SignDisplayMode, n num, zero bool) signKind {
	switch m {
	case SignAlways:
		if n.neg {
			return signMinus
		}
		return signPlus
	case SignExceptZero:
		switch {
		case zero || n.nan:
			return signNone
		case n.neg:
			return signMinus
		}
		return signPlus
	case SignNegative:
		if n.neg && !zero {
			return signMinus
		}
	case SignNever:
	default: // SignAuto
		if n.neg {
			return signMinus
		}
	}
	return signNone
}

// affixes returns the prefix and suffix of a pattern for a sign: the
// negative subpattern for a minus; for a plus, the negative one with its
// minus made a plus when it has a sign, else the positive one after a
// plus sign (ICU's rule: "(¤#)" gives "+$1.00").
func (p *numberPattern) affixes(s signKind) (prefix, suffix string) {
	switch s {
	case signMinus:
		return p.negPrefix, p.negSuffix
	case signPlus:
		if strings.ContainsRune(p.negPrefix+p.negSuffix, markSign) {
			return p.negPrefix, p.negSuffix
		}
		return string(markSign) + p.posPrefix, p.posSuffix
	}
	return p.posPrefix, p.posSuffix
}

// grouping is how the integer digits are grouped.
type grouping struct {
	primary, secondary, minGroup int // primary 0: none
}

func groupingOf(m groupingMode, p *numberPattern, minGroup int, compact bool) grouping {
	g := grouping{int(p.primary), int(p.secondary), minGroup}
	switch m {
	case groupOff:
		return grouping{}
	case groupMin2:
		g.minGroup = max(g.minGroup, 2)
	case groupAlways:
		g.minGroup = 1
	case groupThousands:
		return grouping{3, 3, 1}
	default:
		if compact {
			g.minGroup = max(g.minGroup, 2) // ICU's compact notation groups from 5 digits
		}
	}
	return g
}

// writeInt writes the integer digits ip with the group separator: the
// primary size from the right, then the secondary one, only when ip has
// at least minGroup digits ahead of the last group.
func (g grouping) writeInt(b *strings.Builder, ip, sep string, digits []string) {
	p, sec := g.primary, g.secondary
	if p == 0 || len(ip) < p+g.minGroup {
		b.WriteString(replaceDigits(ip, digits))
		return
	}
	if sec == 0 {
		sec = p
	}
	cut := len(ip) - p
	var starts []int
	for ; cut > 0; cut -= sec {
		starts = append(starts, cut)
	}
	prev := 0
	for i := len(starts) - 1; i >= 0; i-- {
		b.WriteString(replaceDigits(ip[prev:starts[i]], digits))
		b.WriteString(sep)
		prev = starts[i]
	}
	b.WriteString(replaceDigits(ip[prev:], digits))
}

// body writes a rounded number with the locale's digits and separators.
func (nl *numberLocale) body(ip, fp string, g grouping, amount bool) string {
	dec, sep := nl.decimal, nl.group
	if amount {
		if nl.currencyDecimal != "" {
			dec = nl.currencyDecimal
		}
		if nl.currencyGroup != "" {
			sep = nl.currencyGroup
		}
	}
	var b strings.Builder
	g.writeInt(&b, ip, sep, nl.digits)
	if fp != "" {
		b.WriteString(dec)
		b.WriteString(replaceDigits(fp, nl.digits))
	}
	return b.String()
}

// formatted is a number written without its outer affixes: the text and
// the rounded value it shows, for plural forms.
type formatted struct {
	body   string
	zero   bool
	ip, fp string // the ASCII digits shown ("" for NaN and infinity)
}

// formatNumber formats v (a number argument) as c says.
func (d *locData) formatNumber(v any, c numberConfig) string {
	n := toNum(v)
	nl := d.numbersIn(c.numbering)
	if c.style == styleCurrency && c.currencyDisplay == CurrencyName {
		return d.currencyName(n, c, nl)
	}
	return d.format(n, c, nl)
}

// format writes a number in a style with its affixes.
func (d *locData) format(n num, c numberConfig, nl *numberLocale) string {
	amount := c.style == styleCurrency
	cur := ""
	if amount {
		cur = d.currencyText(c.currency, c.currencyDisplay)
	}
	var outer *numberPattern
	switch {
	case c.style == stylePercent:
		outer = nl.pattern(nl.percentPat)
	case amount && c.currencySign == CurrencySignAccounting:
		outer = nl.pattern(nl.accountingPat)
	case amount:
		outer = nl.pattern(nl.currencyPat)
	default:
		outer = nl.pattern(nl.decimalPat)
	}
	var f formatted
	var inner *numberPattern
	if c.notation == NotationCompact && !n.nan && !n.inf {
		kind := compactShort
		switch {
		case amount:
			kind = compactCurrency
		case c.compactDisplay == CompactLong:
			kind = compactLong
		}
		var p *numberPattern
		f, p = d.compact(n, c, nl, kind, amount)
		switch {
		case p == nil:
		case c.style == stylePercent:
			inner = p // "1.2M%"
		default:
			outer = p
		}
	} else {
		f = d.digitsOf(n, c, nl, outer, amount)
	}
	sign := signOf(c.signDisplay, n, f.zero)
	prefix, suffix := outer.affixes(sign)
	if inner != nil {
		f.body = nl.assemble(inner.posPrefix, f.body, inner.posSuffix, "", signNone)
	}
	return nl.assemble(prefix, f.body, suffix, cur, sign)
}

// digitsOf writes a number in standard, scientific or engineering
// notation, with the grouping of the pattern p.
func (d *locData) digitsOf(n num, c numberConfig, nl *numberLocale, p *numberPattern, amount bool) formatted {
	switch {
	case n.nan:
		return formatted{body: nl.nan}
	case n.inf:
		return formatted{body: nl.infinity}
	}
	r := roundingOf(c, currencyDigitsOf(c.currency), amount)
	v := n.dec.shift(c.scale)
	if c.notation == NotationScientific || c.notation == NotationEngineering {
		step := 1
		if c.notation == NotationEngineering {
			step = 3
		}
		e := floorTo(v.magnitude(), step)
		q := r.apply(v.shift(-e), n.neg)
		if !q.isZero() && q.magnitude() >= step {
			// Rounding carried into the next exponent (9.9996 to 10).
			e += step
			q = r.apply(v.shift(-e), n.neg)
		}
		if q.isZero() {
			e = 0
		}
		ip, fp := r.fraction(q)
		var b strings.Builder
		// ICU groups the mantissa too (tok's two-digit groups: "1 23,456E3").
		b.WriteString(nl.body(ip, fp, groupingOf(c.grouping, p, int(nl.minGroup), false), amount))
		b.WriteString(nl.exponential)
		if e < 0 {
			b.WriteString(nl.minus)
			e = -e
		}
		b.WriteString(replaceDigits(strconv.Itoa(e), nl.digits))
		return formatted{body: b.String(), zero: q.isZero(), ip: ip, fp: fp}
	}
	q := r.apply(v, n.neg)
	ip, fp := r.fraction(q)
	g := groupingOf(c.grouping, p, int(nl.minGroup), false)
	return formatted{body: nl.body(ip, fp, g, amount), zero: q.isZero(), ip: ip, fp: fp}
}

// floorTo rounds n down to a multiple of step.
func floorTo(n, step int) int {
	if n >= 0 {
		return n - n%step
	}
	return -((-n + step - 1) / step * step)
}

// assemble joins the affixes and the body, replacing the markers: the
// currency (with the locale's spacing when the symbol touches a digit
// with a letter, as "USD 1.00"), the sign and the percent sign.
func (nl *numberLocale) assemble(prefix, body, suffix, cur string, sign signKind) string {
	var b strings.Builder
	signText := nl.minus
	if sign == signPlus {
		signText = nl.plus
	}
	write := func(affix string, before bool) {
		for i := 0; i < len(affix); i++ {
			switch affix[i] {
			case markCurrency:
				if !before && i == 0 && spaced(cur, body, false) {
					b.WriteString(nl.spacing)
				}
				b.WriteString(cur)
				if before && i == len(affix)-1 && spaced(cur, body, true) {
					b.WriteString(nl.spacing)
				}
			case markSign:
				b.WriteString(signText)
			case markPercent:
				b.WriteString(nl.percent)
			default:
				b.WriteByte(affix[i])
			}
		}
	}
	write(prefix, true)
	b.WriteString(body)
	write(suffix, false)
	return b.String()
}

// spaced says whether CLDR's currencySpacing puts a space between a
// currency text and the number it touches: the end of the text next to
// the number is neither a symbol nor a separator ([[:^S:]&[:^Z:]]) and the
// number's end next to it is a digit ([:digit:]).
func spaced(cur, body string, before bool) bool {
	if cur == "" || body == "" {
		return false
	}
	var c, n rune
	if before {
		c, _ = utf8.DecodeLastRuneInString(cur)
		n, _ = utf8.DecodeRuneInString(body)
	} else {
		c, _ = utf8.DecodeRuneInString(cur)
		n, _ = utf8.DecodeLastRuneInString(body)
	}
	return !unicode.In(c, unicode.S, unicode.Z) && unicode.IsDigit(n)
}

// pluralOf returns the CLDR plural category of a number as it is shown
// (its integer and fraction digits: "1.00" is "other" in English), for
// the locale's rules (golang.org/x/text's, CLDR 32).
func pluralOf(t language.Tag, ip, fp string) string {
	if ip == "" {
		return "other" // NaN, infinity
	}
	digits := make([]byte, 0, len(ip)+len(fp))
	for i := 0; i < len(ip); i++ {
		digits = append(digits, ip[i]-'0')
	}
	for i := 0; i < len(fp); i++ {
		digits = append(digits, fp[i]-'0')
	}
	switch plural.Cardinal.MatchDigits(t, digits, len(ip), len(fp)) {
	case plural.Zero:
		return "zero"
	case plural.One:
		return "one"
	case plural.Two:
		return "two"
	case plural.Few:
		return "few"
	case plural.Many:
		return "many"
	}
	return "other"
}
