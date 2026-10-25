package i18n

import (
	"strconv"
	"strings"
	"time"

	"golang.org/x/text/language"
)

// The calendar data of a locale: the names of the months and weekdays on
// their own (a calendar's heading and column headers, a month picker) and
// the first day of the week. They come from CLDR 48, like the date
// patterns (dates_gen.go, ADR-131).

// NameWidth is the width of a month or weekday name: [NameWide],
// [NameAbbreviated], [NameShort] or [NameNarrow], CLDR's widths. For
// English, October and Monday are:
//
//	NameWide         October   Monday
//	NameAbbreviated  Oct       Mon
//	NameShort        Oct       Mo
//	NameNarrow       O         M
type NameWidth uint8

const (
	// NameWide is the full name: "October", "Monday". The zero value.
	NameWide NameWidth = iota
	// NameAbbreviated is the abbreviated name: "Oct", "Mon".
	NameAbbreviated
	// NameShort is the short weekday name of calendar column headers:
	// "Mo", "Tu" (Spanish "LU", "MA"). CLDR has no short month names, so
	// for months it is the abbreviated one.
	NameShort
	// NameNarrow is the narrowest name, usually one letter and not
	// unique: "M", "T", "W", "T".
	NameNarrow
)

func (w NameWidth) String() string {
	switch w {
	case NameWide:
		return "NameWide"
	case NameAbbreviated:
		return "NameAbbreviated"
	case NameShort:
		return "NameShort"
	case NameNarrow:
		return "NameNarrow"
	}
	return "NameWidth(" + strconv.Itoa(int(w)) + ")"
}

func (w NameWidth) check() {
	if w > NameNarrow {
		panic("i18n: invalid NameWidth " + strconv.Itoa(int(w)) + "; use i18n.NameWide, i18n.NameAbbreviated, i18n.NameShort or i18n.NameNarrow")
	}
}

// MonthName returns the name of month m in the Formatter's locale, in its
// stand-alone form: the name on its own, as a calendar's heading or a
// month picker shows it ("октябрь" in Russian, where a date says
// "5 октября"; for most languages the two forms are the same). Dates
// written with [Formatter.Date] use the form that goes in a date. It
// panics if m is not January to December or width is not a [NameWidth].
//
//	i18n.For("es").MonthName(time.October, i18n.NameWide)        // "octubre"
//	i18n.For("en").MonthName(time.October, i18n.NameAbbreviated) // "Oct"
func (f Formatter) MonthName(m time.Month, width NameWidth) string {
	if m < time.January || m > time.December {
		panic("i18n: MonthName(" + strconv.Itoa(int(m)) + "): not a month; use time.January to time.December")
	}
	width.check()
	field := dfMonthsStandWide
	switch width {
	case NameAbbreviated, NameShort:
		field = dfMonthsStandAbbr
	case NameNarrow:
		field = dfMonthsStandNarrow
	}
	return f.loc().dateData().str(field + int(m) - 1)
}

// WeekdayName returns the name of weekday d in the Formatter's locale, in
// its stand-alone form (the name on its own, as a calendar's column
// headers show it). It panics if d is not time.Sunday to time.Saturday or
// width is not a [NameWidth].
//
//	i18n.For("en").WeekdayName(time.Monday, i18n.NameShort) // "Mo"
//	i18n.For("ja").WeekdayName(time.Monday, i18n.NameWide)  // "月曜日"
func (f Formatter) WeekdayName(d time.Weekday, width NameWidth) string {
	if d < time.Sunday || d > time.Saturday {
		panic("i18n: WeekdayName(" + strconv.Itoa(int(d)) + "): not a weekday; use time.Sunday to time.Saturday")
	}
	width.check()
	field := dfDaysStandWide
	switch width {
	case NameAbbreviated:
		field = dfDaysStandAbbr
	case NameShort:
		field = dfDaysStandShort
	case NameNarrow:
		field = dfDaysStandNarrow
	}
	return f.loc().dateData().str(field + int(d))
}

// FirstDayOfWeek returns the first day of the week in the Formatter's
// locale; see [FirstDayOfWeek].
func (f Formatter) FirstDayOfWeek() time.Weekday { return firstDayOf(f.loc().tag) }

// FirstDayOfWeek returns the day a week starts on for the locale tag, the
// first column of a calendar, from CLDR's week data, which goes by region:
// Sunday in the United States ("en", "en-US"), Japan and Saudi Arabia,
// Monday in most of Europe ("de", "es-ES", "en-GB"), Saturday in Egypt
// ("ar", "ar-EG"). The region is the tag's own ("pt-BR"), else the most
// likely one of its language ("en" is the United States, "ar" Egypt, "ja"
// Japan); regions without data, and tags that are not valid BCP 47, start
// on Monday (CLDR's world default). The Unicode extensions of the tag
// override it as in ICU: "-u-fw-" names the day ("en-US-u-fw-mon" starts
// on Monday) and "-u-rg-" the region whose preferences apply
// ("en-u-rg-gbzzzz" is the United Kingdom's).
func FirstDayOfWeek(tag string) time.Weekday {
	t, err := parseTag(tag)
	if err != nil {
		return time.Weekday(firstDayDefault)
	}
	return firstDayOf(t)
}

// cldrDays are CLDR's keys of the weekdays, by time.Weekday.
var cldrDays = [7]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

func firstDayOf(t language.Tag) time.Weekday {
	if fw := t.TypeForKey("fw"); fw != "" {
		for d, k := range cldrDays {
			if fw == k {
				return time.Weekday(d)
			}
		}
	}
	return firstDayOfRegion(regionOf(t))
}

// regionOf returns the region whose week data applies to t: the region
// of its "rg" extension (two letters followed by a subdivision or
// "zzzz"), else its own or its language's likely one (x/text's
// Tag.Region).
func regionOf(t language.Tag) string {
	if rg := t.TypeForKey("rg"); len(rg) >= 3 && isAlpha(rg[0]) && isAlpha(rg[1]) {
		return strings.ToUpper(rg[:2])
	}
	r, _ := t.Region()
	return r.String()
}

func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// firstDayOfRegion looks a two-letter region up in firstDayRegions.
func firstDayOfRegion(region string) time.Weekday {
	if len(region) == 2 {
		for d, codes := range firstDayRegions {
			for i := 0; i+2 <= len(codes); i += 2 {
				if codes[i:i+2] == region {
					return time.Weekday(d)
				}
			}
		}
	}
	return time.Weekday(firstDayDefault)
}
