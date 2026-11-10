package i18n

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"
)

// Direction is the writing direction of a locale: [LTR] or [RTL].
type Direction uint8

const (
	// LTR is left-to-right text, the zero value.
	LTR Direction = iota
	// RTL is right-to-left text (Arabic, Hebrew, Persian, Urdu...).
	RTL
)

func (d Direction) String() string {
	if d == RTL {
		return "RTL"
	}
	return "LTR"
}

// rtlScripts are the scripts written right to left (CLDR's
// characterOrder).
var rtlScripts = map[string]bool{
	"Arab": true, "Hebr": true, "Thaa": true, "Syrc": true, "Nkoo": true,
	"Adlm": true, "Rohg": true, "Mand": true, "Samr": true, "Mend": true,
	"Hung": true, "Phnx": true,
}

// Dir returns the writing direction of the locale tag: [RTL] for the
// languages written in an Arabic, Hebrew, Thaana, Syriac, N'Ko or Adlam
// script (Arabic, Hebrew, Persian, Urdu, Pashto, Sindhi, Dhivehi...),
// [LTR] otherwise. The script is the tag's own ("sr-Latn", "ku-Arab") or
// the language's default ("ar" is Arabic script, "he" Hebrew). A tag that
// is not valid BCP 47 is LTR.
//
// Its String ("RTL", "LTR") is a value of h.Dir, which lays a subtree out
// in that direction: Div(Dir(i18n.Dir(locale).String()), ...).
func Dir(tag string) Direction {
	t, err := parseTag(tag)
	if err != nil {
		return LTR
	}
	return dirOf(t)
}

func dirOf(t language.Tag) Direction {
	s, _ := t.Script()
	if rtlScripts[s.String()] {
		return RTL
	}
	return LTR
}

// parseTag parses a BCP 47 tag; underscores are accepted (es_MX).
func parseTag(tag string) (language.Tag, error) {
	return language.Parse(strings.ReplaceAll(strings.TrimSpace(tag), "_", "-"))
}

// locData is what a locale needs to format, shared by all Formatters of
// the locale.
type locData struct {
	tag  language.Tag
	name string // canonical BCP 47 name: "es-MX"
	lang language.Base
	dir  Direction

	dates *dateLocale // nil until first used (dateInit)
	// dateIdx is the index in dateTags of the generated locale whose
	// dates, compact patterns and currency names the locale uses.
	dateIdx int
	once    sync.Once
	// digits are the locale's digits 0 to 9 when they are not ASCII (its
	// numbering system, numbering.go); numbers are the symbols and
	// patterns it writes them with (numberformat.go).
	digits  []string
	numbers *numberLocale
}

var locCache sync.Map // string (as given) -> *locData

// forTag returns the data of a locale tag; the tag must be valid.
func forTag(tag string) (*locData, error) {
	if v, ok := locCache.Load(tag); ok {
		return v.(*locData), nil
	}
	t, err := parseTag(tag)
	if err != nil {
		return nil, fmt.Errorf("%q is not a BCP 47 language tag (%v); use a tag such as \"en\", \"es-MX\" or \"pt-BR\"", tag, err)
	}
	d := newLoc(t)
	actual, _ := locCache.LoadOrStore(tag, d)
	return actual.(*locData), nil
}

func newLoc(t language.Tag) *locData {
	b, _ := t.Base()
	nl := newNumberLocale(t, "")
	return &locData{tag: t, name: t.String(), lang: b, dir: dirOf(t), digits: nl.digits, numbers: nl}
}

// Formatter formats messages, numbers, dates and lists for one locale.
// [For] returns the Formatter of a locale and [UseFormatter] the one of
// the component's locale (rebuilding the component when it changes); the
// zero Formatter formats for the current locale ([Locale]), which is what
// the package-level [T], [FormatNumber], [FormatDate] and [FormatList]
// use. A Formatter is a small comparable value, safe to pass in props and
// to use from any goroutine.
type Formatter struct{ d *locData }

// For returns the Formatter of the locale tag (any BCP 47 tag: "en",
// "es-MX", "zh-Hant"; underscores are accepted). Messages come from the
// loaded catalogs with the usual fallback (es-MX, es, the fallback
// locale). It panics if tag is not a valid tag. Use it to format for a
// locale other than the current one, such as the name of a language in
// its own locale.
func For(tag string) Formatter {
	d, err := forTag(tag)
	if err != nil {
		panic("i18n: For(" + fmt.Sprintf("%q", tag) + "): " + err.Error())
	}
	return Formatter{d}
}

func (f Formatter) loc() *locData {
	if f.d == nil {
		return currentLoc()
	}
	return f.d
}

// Locale returns the locale of the Formatter: a BCP 47 tag such as
// "es-MX".
func (f Formatter) Locale() string { return f.loc().name }

// Dir returns the writing direction of the Formatter's locale.
func (f Formatter) Dir() Direction { return f.loc().dir }

// ---------------------------------------------------------------------
// Numbers.

// NumberOption changes how [Formatter.Number] writes a number. The options
// are those of JavaScript's Intl.NumberFormat, with the same defaults:
// [Percent] and [Currency] are its style, [SignDisplay], [Notation],
// [CompactDisplay], [CurrencyDisplay], [CurrencySign] and [RoundingMode] its
// options of the same names (but ties go to the even digit by default),
// [MinFractionDigits] and [MaxFractionDigits] its fraction
// digits and [NoGrouping] useGrouping: false. When two options set the
// same thing (Percent and Currency), the last one wins.
type NumberOption func(*numberConfig)

// numberStyle is Intl's style: a decimal number, a percentage or an
// amount of money.
type numberStyle uint8

const (
	styleDecimal numberStyle = iota
	stylePercent
	styleCurrency
)

// groupingMode is ICU's grouping strategy (skeletons' group-*).
type groupingMode uint8

const (
	groupAuto      groupingMode = iota // the locale's minimum grouping digits (2 in compact notation)
	groupOff                           // no grouping
	groupMin2                          // at least 2 digits before the first separator
	groupAlways                        // from 4 digits on, whatever the locale's minimum
	groupThousands                     // groups of 3 from 4 digits on, whatever the locale's sizes
)

// numberConfig is how a number is written: the options of
// [Formatter.Number] and the styles and skeletons of messages.
type numberConfig struct {
	minFrac, maxFrac int // -1: the default for the style and notation
	minSig, maxSig   int // significant digits (skeletons' @@#); 0: none
	style            numberStyle
	scale            int // the number is multiplied by 10^scale (Percent: 2)
	currency         string
	currencyDisplay  CurrencyDisplayMode
	currencySign     CurrencySignMode
	signDisplay      SignDisplayMode
	notation         NotationMode
	compactDisplay   CompactDisplayMode
	grouping         groupingMode
	numbering        string // a numbering system of a skeleton ("" the locale's)
	rounding         Rounding
}

func defaultNumberConfig() numberConfig { return numberConfig{minFrac: -1, maxFrac: -1} }

// MinFractionDigits writes at least n digits after the decimal separator
// (FormatNumber(2, MinFractionDigits(2)) is "2.00" in English).
func MinFractionDigits(n int) NumberOption {
	return func(c *numberConfig) { c.minFrac = max(n, 0) }
}

// MaxFractionDigits writes at most n digits after the decimal separator,
// rounding as [RoundingMode] says (half to even by default); the default
// is 3 (0 for a percentage, the currency's digits for an amount, and in
// compact notation one fraction digit at most, for two significant
// digits: 1.2K, 12K, 123K).
func MaxFractionDigits(n int) NumberOption {
	return func(c *numberConfig) { c.maxFrac = max(n, 0) }
}

// Percent multiplies the number by 100 and writes it as a percentage with
// the locale's symbol and spacing ("12%" in English, "12 %" in French). The
// default maximum is 0 fraction digits.
func Percent() NumberOption {
	return func(c *numberConfig) { c.style, c.scale = stylePercent, 2 }
}

// NoGrouping leaves out the thousands separators.
func NoGrouping() NumberOption { return func(c *numberConfig) { c.grouping = groupOff } }

// Rounding is how a number is rounded to the digits it shows:
// Intl.NumberFormat's roundingMode. The zero value, [RoundHalfEven], is
// the default (ICU's; Intl's default is [RoundHalfExpand]).
type Rounding uint8

const (
	// RoundHalfEven rounds to the nearest, ties to the even digit (2.5 is
	// 2, 3.5 is 4), the default.
	RoundHalfEven Rounding = iota
	// RoundCeil rounds toward +∞ (1.21 is 1.3, -1.29 is -1.2).
	RoundCeil
	// RoundFloor rounds toward -∞ (1.29 is 1.2, -1.21 is -1.3).
	RoundFloor
	// RoundExpand rounds away from zero (1.21 is 1.3, -1.21 is -1.3).
	RoundExpand
	// RoundTrunc rounds toward zero (1.29 is 1.2, -1.29 is -1.2).
	RoundTrunc
	// RoundHalfCeil rounds to the nearest, ties toward +∞ (-2.5 is -2).
	RoundHalfCeil
	// RoundHalfFloor rounds to the nearest, ties toward -∞ (2.5 is 2).
	RoundHalfFloor
	// RoundHalfExpand rounds to the nearest, ties away from zero (2.5 is 3,
	// -2.5 is -3): Intl.NumberFormat's default.
	RoundHalfExpand
	// RoundHalfTrunc rounds to the nearest, ties toward zero (2.5 is 2).
	RoundHalfTrunc
)

// RoundingMode says how a number is rounded to the digits it shows, in
// every style and notation (FormatNumber(1.21, MaxFractionDigits(1),
// RoundingMode(RoundCeil)) is "1.3"). The digits rounded are the shortest
// decimal ones of a float64 (2.675, not its binary 2.67499...).
func RoundingMode(m Rounding) NumberOption {
	return func(c *numberConfig) { c.rounding = m }
}

// SignDisplayMode says which numbers show a sign: Intl.NumberFormat's
// signDisplay. The signs are the locale's (fa writes "\u200e−", ar-EG
// "\u061c-").
type SignDisplayMode uint8

const (
	// SignAuto shows the minus sign of negative numbers, -0 and numbers
	// that round to it included ("-0"), the default.
	SignAuto SignDisplayMode = iota
	// SignAlways also shows a plus sign on positive numbers and zero
	// ("+3", "+0", "-0").
	SignAlways
	// SignExceptZero shows a plus or minus sign on numbers that are not
	// zero once rounded ("+3", "0", "-3").
	SignExceptZero
	// SignNegative shows the minus sign of negative numbers that are not
	// zero once rounded ("0" for -0).
	SignNegative
	// SignNever shows no sign.
	SignNever
)

// SignDisplay says which numbers show their sign
// (FormatNumber(5, SignDisplay(SignAlways)) is "+5").
func SignDisplay(m SignDisplayMode) NumberOption {
	return func(c *numberConfig) { c.signDisplay = m }
}

// Currency writes the number as an amount of money in the currency, an
// ISO 4217 code ("EUR", "usd"), with the locale's currency pattern and
// symbol: "$1,234.50" in English for "USD", "1.234,50 €" in German for
// "EUR", "-₹12,34,567.50" in Hindi. The digits after the decimal
// separator are the currency's (2, 0 for "JPY", 3 for "BHD"), unless
// [MinFractionDigits] or [MaxFractionDigits] say otherwise. A currency
// that no region uses today is written with its code ("XYZ 1,234.50"). It
// panics if code is not three ASCII letters.
func Currency(code string) NumberOption {
	if len(code) != 3 || !isASCIILetters(code) {
		panic(fmt.Sprintf("i18n: Currency(%q): not an ISO 4217 currency code; use three letters, such as \"USD\", \"EUR\" or \"JPY\"", code))
	}
	code = strings.ToUpper(code)
	return func(c *numberConfig) { c.style, c.scale, c.currency = styleCurrency, 0, code }
}

func isASCIILetters(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i] | 0x20; c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}

// CurrencyDisplayMode is how [Currency] names the currency:
// Intl.NumberFormat's currencyDisplay.
type CurrencyDisplayMode uint8

const (
	// CurrencySymbol is the locale's symbol, the default: "$" for USD in
	// English, "US$" in Spanish, "CA$" for CAD in English, "$" in en-CA.
	CurrencySymbol CurrencyDisplayMode = iota
	// CurrencyNarrowSymbol is the shortest symbol ("$" for USD, CAD, MXN...).
	CurrencyNarrowSymbol
	// CurrencyCode is the ISO code: "USD 1,234.50", "1.234,50 EUR".
	CurrencyCode
	// CurrencyName is the name in the plural form the number takes:
	// "1.00 US dollars", "1 US dollar" with MaxFractionDigits(0),
	// "2 złote polskie".
	CurrencyName
)

// CurrencyDisplay says how [Currency] names the currency.
func CurrencyDisplay(m CurrencyDisplayMode) NumberOption {
	return func(c *numberConfig) { c.currencyDisplay = m }
}

// CurrencySignMode is how [Currency] writes negative amounts:
// Intl.NumberFormat's currencySign.
type CurrencySignMode uint8

const (
	// CurrencySignStandard writes them with the minus sign, the default.
	CurrencySignStandard CurrencySignMode = iota
	// CurrencySignAccounting writes them as the locale's accounting
	// pattern does: "($1,234.50)" in English, "(1 234,50 €)" in French;
	// locales without one use the minus sign.
	CurrencySignAccounting
)

// CurrencySign says how [Currency] writes negative amounts.
func CurrencySign(m CurrencySignMode) NumberOption {
	return func(c *numberConfig) { c.currencySign = m }
}

// NotationMode is how a number is written: Intl.NumberFormat's notation.
type NotationMode uint8

const (
	// NotationStandard writes every digit, the default.
	NotationStandard NotationMode = iota
	// NotationScientific writes a mantissa from 1 to 10 and the exponent
	// with the locale's symbol: "1.235E4", "١٫٢٣أس\u061c-٣" in ar-EG.
	NotationScientific
	// NotationEngineering is scientific notation with an exponent that is
	// a multiple of 3: "12.345E3".
	NotationEngineering
	// NotationCompact writes large numbers with the locale's abbreviations
	// ([CompactDisplay]): "1.2K", "1,2 Mio." in German, "1.2万" in
	// Japanese, "12 लाख" in Hindi; numbers below the locale's first
	// abbreviation are written in full ("1234" in German and Japanese).
	NotationCompact
)

// Notation says how a number is written.
func Notation(m NotationMode) NumberOption {
	return func(c *numberConfig) { c.notation = m }
}

// CompactDisplayMode is how [NotationCompact] abbreviates:
// Intl.NumberFormat's compactDisplay.
type CompactDisplayMode uint8

const (
	// CompactShort writes "1.2K", "1,2 M" (fr), the default.
	CompactShort CompactDisplayMode = iota
	// CompactLong writes words in the plural form the number takes: "1.2
	// thousand", "2 millones", "5 миллионов".
	CompactLong
)

// CompactDisplay says how [NotationCompact] abbreviates. An amount of
// money in compact notation is always short ("$1.2M"), except with
// [CurrencyName] ("1.2 million US dollars" is "1.2M US dollars" short).
func CompactDisplay(m CompactDisplayMode) NumberOption {
	return func(c *numberConfig) { c.compactDisplay = m }
}

// Number formats v for the Formatter's locale: grouping, decimal
// separator and digits follow it ("1.234,5" in Spanish, "1,234.5" in
// English); see [NumberOption] for percentages, currencies, signs and
// notations.
func (f Formatter) Number(v float64, opts ...NumberOption) string {
	c := defaultNumberConfig()
	for _, o := range opts {
		o(&c)
	}
	return f.formatNumber(v, c)
}

func (f Formatter) formatNumber(v any, c numberConfig) string {
	return f.loc().formatNumber(v, c)
}

// ---------------------------------------------------------------------
// Lists.

// List joins items as a sentence fragment for the Formatter's locale:
// "a, b and c" in English, "a, b y c" in Spanish, "a、b、c" in Japanese. A
// single item is returned as is, an empty list is "". Locales without
// data use English patterns.
func (f Formatter) List(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	dl := f.loc().dateData()
	if len(items) == 2 {
		return fillList(dl.str(dfListTwo), items[0], items[1])
	}
	start, middle, end := dl.str(dfListStart), dl.str(dfListMiddle), dl.str(dfListEnd)
	// CLDR: the end pattern joins the last two, start the first two, and
	// middle the rest, building the result from the right.
	acc := fillList(end, items[len(items)-2], items[len(items)-1])
	for i := len(items) - 3; i >= 1; i-- {
		acc = fillList(middle, items[i], acc)
	}
	return fillList(start, items[0], acc)
}

func fillList(pattern, a, b string) string {
	return strings.NewReplacer("{0}", a, "{1}", b).Replace(pattern)
}

// ---------------------------------------------------------------------
// Package-level formatting in the current locale.

// FormatNumber formats v for the current locale ([Locale]); see
// [Formatter.Number]. Like [T], it does not subscribe a component to
// locale changes: call [UseLocale] (or use [UseFormatter]) in components.
func FormatNumber(v float64, opts ...NumberOption) string { return Formatter{}.Number(v, opts...) }

// FormatDate formats the date of t for the current locale in the given
// style ([Short], [Medium], [Long], [Full]); see [Formatter.Date].
func FormatDate(t time.Time, style DateStyle) string { return Formatter{}.Date(t, style) }

// FormatTime formats the time of day of t for the current locale; see
// [Formatter.Time].
func FormatTime(t time.Time, style DateStyle) string { return Formatter{}.Time(t, style) }

// FormatDateTime formats the date and time of t for the current locale;
// see [Formatter.DateTime].
func FormatDateTime(t time.Time, dateStyle, timeStyle DateStyle) string {
	return Formatter{}.DateTime(t, dateStyle, timeStyle)
}

// FormatList joins items for the current locale ("a, b and c"); see
// [Formatter.List].
func FormatList(items []string) string { return Formatter{}.List(items) }
