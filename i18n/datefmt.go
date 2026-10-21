package i18n

import (
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/text/language"
)

// Date, time and list patterns come from the table generated from CLDR
// (dates_gen.go; golang.org/x/text has no date formatting). Dates are in
// the Gregorian calendar and the time zone of the time.Time given.

// DateStyle is the length of a formatted date or time: [Short], [Medium],
// [Long] or [Full]. For the English locale of the United States and a
// Monday afternoon:
//
//	Short   1/5/26        3:04 PM
//	Medium  Jan 5, 2026   3:04:05 PM
//	Long    January 5, 2026   3:04:05 PM UTC
//	Full    Monday, January 5, 2026   3:04:05 PM Coordinated Universal Time
//
// (Time zone names are not localized: Long and Full time styles show the
// zone's abbreviation, or an offset like GMT+1.)
type DateStyle uint8

const (
	// Short is the numeric form: 1/5/26.
	Short DateStyle = iota
	// Medium abbreviates the month: Jan 5, 2026.
	Medium
	// Long spells the month out: January 5, 2026.
	Long
	// Full adds the weekday: Monday, January 5, 2026.
	Full
)

func (s DateStyle) String() string {
	switch s {
	case Short:
		return "Short"
	case Medium:
		return "Medium"
	case Long:
		return "Long"
	case Full:
		return "Full"
	}
	return "DateStyle(" + strconv.Itoa(int(s)) + ")"
}

// tableIndex maps a style to its offset in a (full, long, medium, short)
// group of fields.
func (s DateStyle) tableIndex() int {
	if s > Full {
		panic("i18n: invalid DateStyle " + strconv.Itoa(int(s)) + "; use i18n.Short, i18n.Medium, i18n.Long or i18n.Full")
	}
	return 3 - int(s)
}

// dateLocale is the generated record of a locale.
type dateLocale struct{ rec *[dfNumFields]uint16 }

func (d dateLocale) str(field int) string {
	i := d.rec[field]
	start := uint32(0)
	if i > 0 {
		start = dateEnds[i-1]
	}
	return dateText[start:dateEnds[i]]
}

var (
	dateOnce    sync.Once
	dateIndex   map[language.Tag]int
	dateTagList []language.Tag
	dateMatcher language.Matcher
)

func dateInit() {
	tags := make([]language.Tag, len(dateTags))
	dateIndex = make(map[language.Tag]int, len(dateTags))
	dateTagList = tags
	for i, s := range dateTags {
		tags[i] = language.MustParse(s)
		dateIndex[tags[i]] = i
	}
	dateMatcher = language.NewMatcher(tags)
}

// dateData returns the date patterns of the locale: its own, its
// parent's (es-MX, es-419, es), the closest locale by language matching,
// or English.
func (d *locData) dateData() dateLocale {
	d.once.Do(func() {
		dateOnce.Do(dateInit)
		i, ok := resolveTag(d.tag, dateIndex, dateMatcher, dateTagList)
		if !ok {
			i = dateIndex[language.English]
		}
		d.dates = &dateLocale{rec: &dateData[i]}
	})
	return *d.dates
}

// resolveTag finds the entry of a locale set (index by tag; tags in the
// same order) for t: an exact match, then each CLDR parent (es-MX, es-419,
// es), then the language matcher when it finds the same language, or
// another one with high confidence (no and nb).
func resolveTag(t language.Tag, index map[language.Tag]int, m language.Matcher, tags []language.Tag) (int, bool) {
	for c := t; c != language.Und; c = c.Parent() {
		if i, ok := index[c]; ok {
			return i, true
		}
	}
	if m != nil {
		_, i, conf := m.Match(t)
		if conf == language.No {
			return 0, false
		}
		b, _ := t.Base()
		mb, _ := tags[i].Base()
		if b == mb || conf >= language.High {
			return i, true
		}
	}
	return 0, false
}

// Date formats the date of t (year, month, day, and the weekday for
// [Full]) for the Formatter's locale.
func (f Formatter) Date(t time.Time, style DateStyle) string {
	d := f.loc()
	return d.interpret(d.dateData().str(dfDate+style.tableIndex()), t, "", "")
}

// Time formats the time of day of t for the Formatter's locale: "3:04 PM"
// in English, "15:04" in Spanish.
func (f Formatter) Time(t time.Time, style DateStyle) string {
	d := f.loc()
	return d.interpret(d.dateData().str(dfTime+style.tableIndex()), t, "", "")
}

// DateTime formats a date and a time together, joined the way the locale
// does ("Jan 5, 2026, 3:04 PM").
func (f Formatter) DateTime(t time.Time, dateStyle, timeStyle DateStyle) string {
	d := f.loc()
	dl := d.dateData()
	date := d.interpret(dl.str(dfDate+dateStyle.tableIndex()), t, "", "")
	clock := d.interpret(dl.str(dfTime+timeStyle.tableIndex()), t, "", "")
	return d.interpret(dl.str(dfDateTime+dateStyle.tableIndex()), t, clock, date)
}

// num writes n padded with zeros to width digits, with the locale's
// digits.
func (d *locData) num(n, width int) string {
	s := strconv.Itoa(n)
	if len(s) < width {
		s = strings.Repeat("0", width-len(s)) + s
	}
	return d.nativeDigits(s)
}

func (d *locData) nativeDigits(s string) string {
	if d.digits == nil {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteString(d.digits[r-'0'])
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// interpret writes t following a CLDR (LDML) date pattern: letters
// repeated n times are fields, 'text' is quoted (” is an apostrophe),
// and {0} and {1} of a date-time pattern are replaced by arg0 and arg1.
func (d *locData) interpret(pattern string, t time.Time, arg0, arg1 string) string {
	dl := d.dateData()
	var b strings.Builder
	for i := 0; i < len(pattern); {
		c := pattern[i]
		switch {
		case c == '\'':
			if i+1 < len(pattern) && pattern[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			i++
			for i < len(pattern) {
				if pattern[i] == '\'' {
					if i+1 < len(pattern) && pattern[i+1] == '\'' {
						b.WriteByte('\'')
						i += 2
						continue
					}
					i++
					break
				}
				b.WriteByte(pattern[i])
				i++
			}
		case c == '{' && i+2 < len(pattern) && pattern[i+2] == '}' && (pattern[i+1] == '0' || pattern[i+1] == '1'):
			if pattern[i+1] == '0' {
				b.WriteString(arg0)
			} else {
				b.WriteString(arg1)
			}
			i += 3
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			n := 1
			for i+n < len(pattern) && pattern[i+n] == c {
				n++
			}
			b.WriteString(d.field(dl, c, n, t))
			i += n
		default:
			_, size := utf8.DecodeRuneInString(pattern[i:])
			b.WriteString(pattern[i : i+size])
			i += size
		}
	}
	return b.String()
}

// firstRune returns the first rune of s as a string (narrow names are
// approximated by the first letter of the abbreviation).
func firstRune(s string) string {
	_, n := utf8.DecodeRuneInString(s)
	return s[:n]
}

func (d *locData) field(dl dateLocale, letter byte, n int, t time.Time) string {
	switch letter {
	case 'G':
		i := 1
		if t.Year() <= 0 {
			i = 0
		}
		if n == 4 {
			return dl.str(dfErasWide + i)
		}
		return dl.str(dfErasAbbr + i)
	case 'y':
		y := t.Year()
		if y <= 0 {
			y = 1 - y
		}
		if n == 2 {
			return d.num(y%100, 2)
		}
		return d.num(y, n)
	case 'M', 'L':
		m := int(t.Month())
		switch {
		case n <= 2:
			return d.num(m, n)
		case n == 3:
			return dl.str(dfMonthsAbbr + m - 1)
		case n == 4:
			return dl.str(dfMonthsWide + m - 1)
		}
		return firstRune(dl.str(dfMonthsAbbr + m - 1))
	case 'd':
		return d.num(t.Day(), n)
	case 'E', 'c', 'e':
		wd := int(t.Weekday())
		switch {
		case (letter == 'c' || letter == 'e') && n <= 2:
			return d.num(wd+1, n)
		case n <= 3 || n == 6:
			return dl.str(dfDaysAbbr + wd)
		case n == 4:
			return dl.str(dfDaysWide + wd)
		}
		return firstRune(dl.str(dfDaysAbbr + wd))
	case 'a', 'b', 'B':
		if t.Hour() < 12 {
			return dl.str(dfAM)
		}
		return dl.str(dfPM)
	case 'h':
		h := t.Hour() % 12
		if h == 0 {
			h = 12
		}
		return d.num(h, n)
	case 'H':
		return d.num(t.Hour(), n)
	case 'K':
		return d.num(t.Hour()%12, n)
	case 'k':
		h := t.Hour()
		if h == 0 {
			h = 24
		}
		return d.num(h, n)
	case 'm':
		return d.num(t.Minute(), n)
	case 's':
		return d.num(t.Second(), n)
	case 'z', 'v':
		return d.zone(t)
	}
	return strings.Repeat(string(letter), n)
}

// zone writes the zone abbreviation of t ("CET"), or an offset ("GMT+1")
// when the zone has no abbreviation.
func (d *locData) zone(t time.Time) string {
	name, off := t.Zone()
	if name != "" && name[0] != '+' && name[0] != '-' {
		return name
	}
	if off == 0 {
		return "GMT"
	}
	sign := "+"
	if off < 0 {
		sign, off = "-", -off
	}
	s := "GMT" + sign + d.num(off/3600, 1)
	if m := off % 3600 / 60; m != 0 {
		s += ":" + d.num(m, 2)
	}
	return s
}
