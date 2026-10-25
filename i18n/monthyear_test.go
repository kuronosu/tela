package i18n

import (
	"strings"
	"testing"
	"time"
)

func TestMonthYear(t *testing.T) {
	oct := time.Date(2026, 10, 25, 14, 30, 0, 0, time.UTC)
	for loc, want := range map[string]string{
		"en":    "October 2026",
		"en-GB": "October 2026",
		"es":    "octubre de 2026",
		"es-MX": "octubre de 2026",
		"ja":    "2026年10月",
		"hu":    "2026. október",
		"de":    "Oktober 2026",
		"fr":    "octobre 2026",
		"ko":    "2026년 10월",
		"zh":    "2026年10月",
		// The stand-alone month: a date says "5 октября".
		"ru": "октябрь 2026\u202fг.",
		"ar": "أكتوبر ٢٠٢٦",
	} {
		if got := For(loc).MonthYear(oct); got != want {
			t.Errorf("%s MonthYear = %q, want %q", loc, got, want)
		}
	}
	// A locale without data of its own resolves like the date patterns.
	if got := For("es-PE").MonthYear(oct); got != For("es-419").MonthYear(oct) {
		t.Errorf("es-PE MonthYear = %q", got)
	}
	// Every generated locale has a pattern that writes the year (in the
	// locale's digits) and a month that differs from the other months'.
	for _, tag := range dateTags {
		f := For(tag)
		year := f.loc().num(2026, 1)
		seen := map[string]bool{}
		for m := time.January; m <= time.December; m++ {
			got := f.MonthYear(time.Date(2026, m, 1, 0, 0, 0, 0, time.UTC))
			if !strings.Contains(got, year) || seen[got] {
				t.Errorf("%s MonthYear(%v) = %q: want the year %s and a text of its own", tag, m, got, year)
			}
			seen[got] = true
		}
	}
}

// The L and c pattern letters are the stand-alone names, M and E the ones
// that go in a date.
func TestStandAlonePatternLetters(t *testing.T) {
	mon := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct{ loc, pattern, want string }{
		{"ru", "LLLL / MMMM", "октябрь / октября"},
		{"ru", "LLL", "окт."},
		{"fi", "cccc / EEEE", "maanantai / maanantaina"},
	} {
		if got := For(tt.loc).loc().interpret(tt.pattern, mon, "", ""); got != tt.want {
			t.Errorf("%s %q = %q, want %q", tt.loc, tt.pattern, got, tt.want)
		}
	}
}
