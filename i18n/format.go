package i18n

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"
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

	printer *message.Printer // numbers

	dates *dateLocale // nil until first used (dateInit)
	once  sync.Once
	// digits are the locale's digits 0 to 9 when they are not ASCII.
	digits []string
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
	d := &locData{tag: t, name: t.String(), lang: b, dir: dirOf(t), printer: message.NewPrinter(t)}
	// Native digits: format 0 to 9 without grouping and see whether they
	// are ASCII.
	digits := make([]string, 10)
	ascii := true
	for i := range digits {
		digits[i] = d.printer.Sprint(number.Decimal(i, number.NoSeparator()))
		if digits[i] != string(rune('0'+i)) {
			ascii = false
		}
	}
	if !ascii {
		d.digits = digits
	}
	return d
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

// NumberOption changes how [FormatNumber] writes a number.
type NumberOption func(*numberConfig)

type numberConfig struct {
	minFrac, maxFrac int // -1: the locale's default (0 and 3)
	percent          bool
	noGrouping       bool
}

// MinFractionDigits writes at least n digits after the decimal separator
// (FormatNumber(2, MinFractionDigits(2)) is "2.00" in English).
func MinFractionDigits(n int) NumberOption {
	return func(c *numberConfig) { c.minFrac = max(n, 0) }
}

// MaxFractionDigits writes at most n digits after the decimal separator,
// rounding half to even; the default is 3.
func MaxFractionDigits(n int) NumberOption {
	return func(c *numberConfig) { c.maxFrac = max(n, 0) }
}

// Percent multiplies the number by 100 and writes it as a percentage with
// the locale's symbol and spacing ("12%" in English, "12 %" in French). The
// default maximum is 0 fraction digits.
func Percent() NumberOption { return func(c *numberConfig) { c.percent = true } }

// NoGrouping leaves out the thousands separators.
func NoGrouping() NumberOption { return func(c *numberConfig) { c.noGrouping = true } }

// Number formats v for the Formatter's locale: grouping, decimal
// separator and digits follow it ("1.234,5" in Spanish, "1,234.5" in
// English).
func (f Formatter) Number(v float64, opts ...NumberOption) string {
	c := numberConfig{minFrac: -1, maxFrac: -1}
	for _, o := range opts {
		o(&c)
	}
	return f.formatNumber(v, c)
}

func (f Formatter) formatNumber(v any, c numberConfig) string {
	d := f.loc()
	var opts []number.Option
	if c.noGrouping {
		opts = append(opts, number.NoSeparator())
	}
	if c.minFrac >= 0 || c.maxFrac >= 0 {
		lo, hi := c.minFrac, c.maxFrac
		if lo < 0 {
			lo = 0
		}
		if hi < 0 {
			hi = max(lo, 3)
			if c.percent {
				hi = lo
			}
		}
		hi = max(hi, lo)
		opts = append(opts, number.MinFractionDigits(lo), number.MaxFractionDigits(hi))
	}
	if c.percent {
		return d.printer.Sprint(number.Percent(v, opts...))
	}
	return d.printer.Sprint(number.Decimal(v, opts...))
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
