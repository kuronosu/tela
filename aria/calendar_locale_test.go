package aria_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// calendarIn mounts a calendar in a locale and returns its result.
func calendarIn(t *testing.T, o aria.CalendarOptions) (*telatest.App, *aria.CalendarResult) {
	t.Helper()
	var res aria.CalendarResult
	if o.Today.IsZero() {
		o.Today = calToday
	}
	app := telatest.MountFunc(t, func() tela.Node {
		res = aria.UseCalendar(o)
		return renderCalendar(res)
	}, telatest.Size(400, 360))
	return app, &res
}

func TestCalendarFirstDayOfWeekFollowsTheLocale(t *testing.T) {
	for _, tt := range []struct {
		locale string
		first  time.Weekday
		row    string // the first cell of the first row of October 2026
	}{
		{"en-US", time.Sunday, "2026-09-27"},
		{"de", time.Monday, "2026-09-28"},
		{"ar-EG", time.Saturday, "2026-09-26"},
		{"es-ES", time.Monday, "2026-09-28"},
	} {
		_, res := calendarIn(t, aria.CalendarOptions{Locale: tt.locale})
		if res.FirstDayOfWeek() != tt.first {
			t.Errorf("%s: first day %v, want %v", tt.locale, res.FirstDayOfWeek(), tt.first)
		}
		weeks := res.Weeks(0)
		if got := weeks[0][0].String(); got != tt.row {
			t.Errorf("%s: the grid starts on %s, want %s", tt.locale, got, tt.row)
		}
		if weeks[0][0].Weekday() != tt.first {
			t.Errorf("%s: the first column is a %v", tt.locale, weeks[0][0].Weekday())
		}
	}
	// The option overrides the locale.
	_, res := calendarIn(t, aria.CalendarOptions{Locale: "en-US", FirstDayOfWeek: new(time.Wednesday)})
	if got := res.Weeks(0)[0][0].Weekday(); got != time.Wednesday {
		t.Errorf("FirstDayOfWeek option: the grid starts on a %v", got)
	}
	if got := res.WeekdayNames(i18n.NameShort); got[0] != "We" || got[6] != "Tu" {
		t.Errorf("weekday names start at the first day: %v", got)
	}
}

func TestCalendarWeekRows(t *testing.T) {
	// The weeks a month needs: 4 (February 2026 starts on a Sunday and
	// has 28 days), 5, 6 (August 2026 with Sunday first).
	for _, tt := range []struct {
		start aria.Date
		fixed bool
		rows  int
	}{
		{d(2026, time.February, 1), false, 4},
		{d(2026, time.October, 1), false, 5},
		{d(2026, time.August, 1), false, 6},
		{d(2026, time.February, 1), true, 6},
	} {
		_, res := calendarIn(t, aria.CalendarOptions{
			Locale: "en-US", DefaultFocusedDate: tt.start, FixedWeeks: tt.fixed,
		})
		weeks := res.Weeks(0)
		if len(weeks) != tt.rows {
			t.Errorf("%v fixed=%v: %d weeks, want %d", tt.start, tt.fixed, len(weeks), tt.rows)
		}
		for _, w := range weeks {
			if len(w) != 7 {
				t.Errorf("a week of %d days", len(w))
			}
		}
		// The grid holds every day of the month once and in order.
		days := 0
		var prev aria.Date
		for _, w := range weeks {
			for _, day := range w {
				if !prev.IsZero() && day != prev.AddDays(1) {
					t.Errorf("%v is not the day after %v", day, prev)
				}
				prev = day
				if !res.IsOutsideMonth(0, day) {
					days++
				}
			}
		}
		if want := tt.start.AddMonths(1).AddDays(-1).Day; days != want {
			t.Errorf("%v: %d days of the month in the grid, want %d", tt.start, days, want)
		}
	}
}

func TestCalendarNamesAndHeadings(t *testing.T) {
	oct := d(2026, time.October, 5)
	for _, tt := range []struct {
		locale  string
		heading string
		days    string // short names from the first day
		cell    string // the name of October 5
		digits  string // October 5's text
	}{
		{"en-US", "October 2026", "Su Mo Tu We Th Fr Sa", "Monday, October 5, 2026", "5"},
		{"es", "octubre de 2026", "LU MA MI JU VI SA DO", "lunes, 5 de octubre de 2026", "5"},
		{"ja", "2026年10月", "日 月 火 水 木 金 土", "2026年10月5日月曜日", "5"},
		{"ar", "أكتوبر ٢٠٢٦", "سبت أحد إثنين ثلاثاء أربعاء خميس جمعة", "الاثنين، ٥ أكتوبر ٢٠٢٦", "٥"},
		{"de", "Oktober 2026", "Mo. Di. Mi. Do. Fr. Sa. So.", "Montag, 5. Oktober 2026", "5"},
	} {
		app, res := calendarIn(t, aria.CalendarOptions{Locale: tt.locale, DefaultValue: &oct})
		if got := res.Heading(0); got != tt.heading {
			t.Errorf("%s: heading %q, want %q", tt.locale, got, tt.heading)
		}
		if got := res.Title; got != tt.heading {
			t.Errorf("%s: title %q, want %q", tt.locale, got, tt.heading)
		}
		if got := strings.Join(res.WeekdayNames(i18n.NameShort), " "); got != tt.days {
			t.Errorf("%s: weekdays %q, want %q", tt.locale, got, tt.days)
		}
		app.ExpectRole("gridcell", tt.cell)
		if got := res.DayText(oct); got != tt.digits {
			t.Errorf("%s: day text %q, want %q", tt.locale, got, tt.digits)
		}
		app.ExpectRole("heading", tt.heading)
	}
	// Narrow names, and the full names that name the columns.
	_, res := calendarIn(t, aria.CalendarOptions{Locale: "en-US"})
	if got := strings.Join(res.WeekdayNames(i18n.NameNarrow), ""); got != "SMTWTFS" {
		t.Errorf("narrow names %q", got)
	}
	if got := strings.Join(res.WeekdayNames(i18n.NameAbbreviated), " "); got != "Sun Mon Tue Wed Thu Fri Sat" {
		t.Errorf("abbreviated names %q", got)
	}
}

func TestCalendarUsesTheComponentsLocale(t *testing.T) {
	// Without a Locale option the calendar follows i18n's current one.
	i18n.SetLocale("de")
	t.Cleanup(func() { i18n.SetLocale("") })
	_, res := calendarIn(t, aria.CalendarOptions{})
	if res.Title != "Oktober 2026" || res.FirstDayOfWeek() != time.Monday {
		t.Errorf("title %q, first day %v", res.Title, res.FirstDayOfWeek())
	}
}

func TestCalendarRightToLeft(t *testing.T) {
	// Arabic is right to left by locale: the arrows are mirrored.
	app, _ := calendarIn(t, aria.CalendarOptions{
		Locale: "ar-EG", DefaultValue: new(d(2026, time.October, 14)),
	})
	app.Focus(cell2(d(2026, time.October, 14), "ar-EG"))
	app.Press("ArrowLeft")
	app.ExpectFocused(cell2(d(2026, time.October, 15), "ar-EG"))
	app.Press("ArrowRight")
	app.Press("ArrowRight")
	app.ExpectFocused(cell2(d(2026, time.October, 13), "ar-EG"))
	// Up, down, Home and End do not change: Home is the first day of the week
	// (Saturday for ar-EG).
	app.Press("Home")
	app.ExpectFocused(cell2(d(2026, time.October, 10), "ar-EG"))
	app.Press("End")
	app.ExpectFocused(cell2(d(2026, time.October, 16), "ar-EG"))
	app.Press("ArrowDown")
	app.ExpectFocused(cell2(d(2026, time.October, 23), "ar-EG"))

	// An English calendar set to rtl is mirrored too, and an Arabic one set
	// to ltr is not.
	app, _ = calendarIn(t, aria.CalendarOptions{
		Locale: "en-US", Direction: "rtl", DefaultValue: new(d(2026, time.October, 14)),
	})
	app.Focus(cell(d(2026, time.October, 14)))
	app.Press("ArrowLeft")
	app.ExpectFocused(cell(d(2026, time.October, 15)))
	app, _ = calendarIn(t, aria.CalendarOptions{
		Locale: "ar-EG", Direction: "ltr", DefaultValue: new(d(2026, time.October, 14)),
	})
	app.Focus(cell2(d(2026, time.October, 14), "ar-EG"))
	app.Press("ArrowLeft")
	app.ExpectFocused(cell2(d(2026, time.October, 13), "ar-EG"))
}

// cell2 locates a cell by its name in a locale.
func cell2(day aria.Date, locale string) telatest.Locator {
	return telatest.ByRole("gridcell", i18n.For(locale).Date(day.Time(time.UTC), i18n.Full))
}

func TestCalendarTwoMonths(t *testing.T) {
	app, res := calendarIn(t, aria.CalendarOptions{
		Locale: "en-US", VisibleMonths: 2, DefaultFocusedDate: d(2026, time.October, 15),
	})
	if res.Months() != 2 || res.Title != "October 2026 – November 2026" {
		t.Errorf("months %d, title %q", res.Months(), res.Title)
	}
	app.ExpectRole("grid", "October 2026")
	app.ExpectRole("grid", "November 2026")
	if res.Month(1) != d(2026, time.November, 1) {
		t.Errorf("month 1 is %v", res.Month(1))
	}
	// The second month holds its own dates, and the outside days show in
	// both grids without clashing.
	if !res.IsOutsideMonth(0, d(2026, time.November, 1)) || res.IsOutsideMonth(1, d(2026, time.November, 1)) {
		t.Error("November 1 is outside October's grid and inside November's")
	}
	app.Focus(cell(d(2026, time.October, 15)))
	// Moving inside the window does not move it.
	app.Press("PageDown")
	app.ExpectFocused(cell(d(2026, time.November, 15)))
	app.ExpectRole("grid", "October 2026")
	// Past the window it moves so that the focus is the first month.
	expectKeys(t, app, "PageDown", d(2026, time.December, 15))
	app.ExpectRole("grid", "December 2026")
	app.ExpectRole("grid", "January 2027")
	// The buttons page by the visible months.
	app.Click(telatest.ByRole("button", "Previous"))
	app.ExpectRole("grid", "October 2026")
	app.ExpectRole("grid", "November 2026")
	app.Click(telatest.ByRole("button", "Next"))
	app.Click(telatest.ByRole("button", "Next"))
	app.ExpectRole("grid", "February 2027")
	app.ExpectRole("grid", "March 2027")
}

func TestCalendarTwoMonthsRespectLimits(t *testing.T) {
	app, res := calendarIn(t, aria.CalendarOptions{
		Locale: "en-US", VisibleMonths: 2, Max: d(2026, time.November, 20),
		DefaultFocusedDate: d(2026, time.November, 10),
	})
	// Max is in November: the window ends there and starts in October.
	if res.Month(0) != d(2026, time.October, 1) {
		t.Errorf("window starts at %v, want October", res.Month(0))
	}
	app.Expect(telatest.ByRole("button", "Next"), telatest.Disabled(true))
	app.Expect(telatest.ByRole("button", "Previous"), telatest.Disabled(false))
}

func TestCalendarCustomLabels(t *testing.T) {
	app, _ := calendarIn(t, aria.CalendarOptions{Locale: "es", PreviousLabel: "Mes anterior", NextLabel: "Mes siguiente"})
	app.ExpectRole("button", "Mes anterior")
	app.ExpectRole("button", "Mes siguiente")
}

func TestDateArithmetic(t *testing.T) {
	for _, tt := range []struct {
		name      string
		got, want aria.Date
	}{
		{"31 days", d(2026, time.January, 1).AddDays(31), d(2026, time.February, 1)},
		{"back over a year", d(2026, time.January, 1).AddDays(-1), d(2025, time.December, 31)},
		{"a leap day", d(2028, time.February, 28).AddDays(1), d(2028, time.February, 29)},
		{"a common year", d(2027, time.February, 28).AddDays(1), d(2027, time.March, 1)},
		{"month end", d(2026, time.January, 31).AddMonths(1), d(2026, time.February, 28)},
		{"months back", d(2026, time.March, 31).AddMonths(-1), d(2026, time.February, 28)},
		{"months over years", d(2026, time.November, 5).AddMonths(14), d(2028, time.January, 5)},
		{"negative months", d(2026, time.January, 5).AddMonths(-13), d(2024, time.December, 5)},
		{"leap year back", d(2028, time.February, 29).AddYears(-4), d(2024, time.February, 29)},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: %v, want %v", tt.name, tt.got, tt.want)
		}
	}
	oct := d(2026, time.October, 5)
	if oct.Weekday() != time.Monday || !oct.IsValid() || d(2026, time.February, 30).IsValid() || (aria.Date{}).IsValid() {
		t.Error("Weekday or IsValid")
	}
	if aria.DateOf(time.Date(2026, 10, 5, 23, 59, 0, 0, time.UTC)) != oct {
		t.Error("DateOf")
	}
	if got := oct.Time(nil); !got.Equal(time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Time(nil) = %v", got)
	}
	if oct.Compare(d(2026, time.October, 6)) != -1 || !oct.Before(d(2027, time.January, 1)) || !oct.After(d(2026, time.September, 30)) {
		t.Error("Compare, Before or After")
	}
	if oct.String() != "2026-10-05" || (aria.Date{}).String() != "none" {
		t.Errorf("String = %q", oct.String())
	}
	r := aria.DateRange{Start: oct, End: d(2026, time.October, 8)}
	if !r.Contains(oct) || !r.Contains(d(2026, time.October, 8)) || r.Contains(d(2026, time.October, 9)) || (aria.DateRange{}).Contains(oct) {
		t.Error("DateRange.Contains")
	}
	// Dates are comparable and usable as map keys.
	seen := map[aria.Date]bool{oct: true}
	if !seen[d(2026, time.October, 5)] {
		t.Error("Date as a map key")
	}
	// Midnight, in another zone, is still the same day.
	tokyo := time.FixedZone("JST", 9*3600)
	if got := aria.DateOf(oct.Time(tokyo)); got != oct {
		t.Errorf("round trip through Tokyo = %v", got)
	}
	days := []aria.Date{oct.AddDays(2), oct, oct.AddDays(1)}
	slices.SortFunc(days, aria.Date.Compare)
	if days[0] != oct || days[2] != oct.AddDays(2) {
		t.Errorf("sorted = %v", days)
	}
}
