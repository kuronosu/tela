package i18n

import (
	"strings"
	"testing"
	"time"
)

func TestFirstDayOfWeek(t *testing.T) {
	for tag, want := range map[string]time.Weekday{
		// The region of the tag.
		"en-US": time.Sunday, "en-GB": time.Monday, "es-ES": time.Monday, "es-MX": time.Sunday,
		"pt-BR": time.Sunday, "de-AT": time.Monday,
		"ar-EG": time.Saturday, "ar-SA": time.Sunday, "ar-AE": time.Monday, "fa-IR": time.Saturday,
		"dv-MV": time.Friday,
		// No region: the language's likely one (en-US, de-DE, ar-EG, ja-JP,
		// he-IL, pt-BR).
		"en": time.Sunday, "es": time.Monday, "de": time.Monday, "fr": time.Monday,
		"ar": time.Saturday, "ja": time.Sunday, "he": time.Sunday, "pt": time.Sunday,
		// A macro region, an unknown language, a tag that is not BCP 47 and
		// underscores.
		"es-419": time.Monday, "qaa": time.Monday, "not a tag!": time.Monday, "en_US": time.Sunday,
		// Unicode extensions: the first day and the region override.
		"en-US-u-fw-mon": time.Monday, "de-u-fw-sun": time.Sunday, "en-u-rg-gbzzzz": time.Monday,
		"de-u-rg-uszzzz": time.Sunday,
	} {
		if got := FirstDayOfWeek(tag); got != want {
			t.Errorf("FirstDayOfWeek(%q) = %v, want %v", tag, got, want)
		}
	}
	if got := For("ar-SA").FirstDayOfWeek(); got != time.Sunday {
		t.Errorf("For(ar-SA).FirstDayOfWeek() = %v", got)
	}
	// The zero Formatter is the current locale's.
	isolate(t)
	SetLocale("en-US")
	if got := (Formatter{}).FirstDayOfWeek(); got != time.Sunday {
		t.Errorf("current locale en-US: %v", got)
	}
	SetLocale("en-GB")
	if got := (Formatter{}).FirstDayOfWeek(); got != time.Monday {
		t.Errorf("current locale en-GB: %v", got)
	}
}

// The generated table holds every region of the vendored data once, and
// the regions the world default covers are left out of it.
func TestFirstDayRegionsTable(t *testing.T) {
	seen := map[string]bool{}
	for d, codes := range firstDayRegions {
		if d == firstDayDefault && codes != "" {
			t.Errorf("the default day %d has regions %q", d, codes)
		}
		if len(codes)%2 != 0 {
			t.Fatalf("day %d: %q is not a list of two-letter codes", d, codes)
		}
		for i := 0; i < len(codes); i += 2 {
			r := codes[i : i+2]
			if seen[r] {
				t.Errorf("region %s is listed twice", r)
			}
			seen[r] = true
			if i > 0 && codes[i-2:i] >= r {
				t.Errorf("day %d: %s after %s is not sorted", d, r, codes[i-2:i])
			}
		}
	}
}

func TestMonthAndWeekdayNames(t *testing.T) {
	type names struct{ wide, abbr, short, narrow string }
	for _, tt := range []struct {
		loc     string
		month   names // October
		weekday names // Monday
	}{
		{"en", names{"October", "Oct", "Oct", "O"}, names{"Monday", "Mon", "Mo", "M"}},
		{"es", names{"octubre", "oct", "oct", "O"}, names{"lunes", "lun", "LU", "L"}},
		{"ja", names{"10月", "10月", "10月", "10"}, names{"月曜日", "月", "月", "月"}},
		{"ar", names{"أكتوبر", "أكتوبر", "أكتوبر", "ك"}, names{"الاثنين", "الاثنين", "إثنين", "ن"}},
		// Stand-alone forms: a date says "5 октября", a calendar "октябрь".
		{"ru", names{"октябрь", "окт.", "окт.", "О"}, names{"понедельник", "пн", "пн", "П"}},
		// A locale without data of its own resolves like the date
		// patterns (es-PE to es-419).
		{"es-PE", names{"octubre", "oct", "oct", "O"}, names{"lunes", "lun", "LU", "L"}},
	} {
		f := For(tt.loc)
		for _, c := range []struct {
			w            NameWidth
			month, wkday string
		}{
			{NameWide, tt.month.wide, tt.weekday.wide},
			{NameAbbreviated, tt.month.abbr, tt.weekday.abbr},
			{NameShort, tt.month.short, tt.weekday.short},
			{NameNarrow, tt.month.narrow, tt.weekday.narrow},
		} {
			if got := f.MonthName(time.October, c.w); got != c.month {
				t.Errorf("%s MonthName(October, %v) = %q, want %q", tt.loc, c.w, got, c.month)
			}
			if got := f.WeekdayName(time.Monday, c.w); got != c.wkday {
				t.Errorf("%s WeekdayName(Monday, %v) = %q, want %q", tt.loc, c.w, got, c.wkday)
			}
		}
	}
	// Every month and weekday of every generated locale has a name.
	for _, tag := range dateTags {
		f := For(tag)
		for w := NameWide; w <= NameNarrow; w++ {
			for m := time.January; m <= time.December; m++ {
				if f.MonthName(m, w) == "" {
					t.Errorf("%s: no %v name for %v", tag, w, m)
				}
			}
			for d := time.Sunday; d <= time.Saturday; d++ {
				if f.WeekdayName(d, w) == "" {
					t.Errorf("%s: no %v name for %v", tag, w, d)
				}
			}
		}
	}
	// The format forms the date patterns use are unchanged.
	if got := For("ru").Date(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), Long); !strings.Contains(got, "октября") {
		t.Errorf("ru long date %q, want the format form октября", got)
	}
}

func TestNameErrors(t *testing.T) {
	f := For("en")
	for name, call := range map[string]func(){
		"not a month":       func() { f.MonthName(13, NameWide) },
		"not a weekday":     func() { f.WeekdayName(7, NameWide) },
		"invalid NameWidth": func() { f.MonthName(time.May, NameWidth(9)) },
	} {
		func() {
			defer func() {
				if r, _ := recover().(string); !strings.Contains(r, name) {
					t.Errorf("panic %q, want one containing %q", r, name)
				}
			}()
			call()
		}()
	}
	if s := NameShort.String(); s != "NameShort" {
		t.Errorf("String() = %q", s)
	}
}
