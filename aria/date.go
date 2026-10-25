package aria

import (
	"fmt"
	"time"
)

// Date is a day of the Gregorian calendar, without a time of day or a time
// zone: what a calendar selects ("the 5th of October 2026"), the same
// wherever it is read. The zero Date means "no date".
//
// It is a comparable value, unlike a time.Time (whose == also compares the
// zone pointer and the monotonic clock), so it can be a map key, the state
// of tela.UseState, a prop that skips a build when it is equal, and it has
// no daylight saving time to trip date arithmetic on: AddDays(1) is always
// the next day. Convert with [DateOf] and [Date.Time].
//
//	d := aria.Date{Year: 2026, Month: time.October, Day: 5}
//	d.AddMonths(1)            // 2026-11-05
//	d.AddDays(-5)             // 2026-09-30
//	aria.Date{2026, 1, 31}.AddMonths(1) // 2026-02-28: the day is clamped
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the calendar day of t in t's own time zone.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// Today returns the current day in the local time zone. Calendars take
// their notion of today from an option (CalendarOptions.Today), so tests
// can fix it; this is the default.
func Today() Date { return DateOf(time.Now()) }

// IsZero reports whether d is the zero Date, no date at all.
func (d Date) IsZero() bool { return d == Date{} }

// IsValid reports whether d is a day that exists: January 1 of year 1 to
// December 31 of year 9999, with the right number of days in its month.
func (d Date) IsValid() bool {
	return d.Year >= 1 && d.Year <= 9999 && d.Month >= time.January && d.Month <= time.December &&
		d.Day >= 1 && d.Day <= daysIn(d.Year, d.Month)
}

// Time returns midnight of d in loc (UTC when loc is nil).
func (d Date) Time(loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, loc)
}

// Weekday returns the day of the week of d.
func (d Date) Weekday() time.Weekday { return d.utc().Weekday() }

// utc is d at noon UTC, the base of the arithmetic (noon, so that no
// zone's offset moves it to another day).
func (d Date) utc() time.Time { return time.Date(d.Year, d.Month, d.Day, 12, 0, 0, 0, time.UTC) }

// AddDays returns the day n days after d (before it when n is negative).
func (d Date) AddDays(n int) Date { return DateOf(d.utc().AddDate(0, 0, n)) }

// AddMonths returns the same day of the month n months after d; when that
// month is shorter the day is the last one of it (January 31 plus one
// month is February 28 or 29).
func (d Date) AddMonths(n int) Date {
	m := int(d.Month) - 1 + n
	y := d.Year + floorDiv(m, 12)
	m = m - floorDiv(m, 12)*12 + 1
	day := min(d.Day, daysIn(y, time.Month(m)))
	return Date{y, time.Month(m), day}
}

// AddYears returns the same day n years after d, with the day clamped like
// [Date.AddMonths] (February 29 plus one year is February 28).
func (d Date) AddYears(n int) Date { return d.AddMonths(12 * n) }

// Compare returns -1, 0 or 1 when d is before, the same as or after o.
func (d Date) Compare(o Date) int {
	switch {
	case d.Year != o.Year:
		return cmpInt(d.Year, o.Year)
	case d.Month != o.Month:
		return cmpInt(int(d.Month), int(o.Month))
	}
	return cmpInt(d.Day, o.Day)
}

// Before reports whether d is a day before o.
func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }

// After reports whether d is a day after o.
func (d Date) After(o Date) bool { return d.Compare(o) > 0 }

// String returns d as ISO 8601, "2026-10-05" (and "none" for the zero
// Date). Unlike a localized date it is unique per day, so it is the key of
// a calendar cell: Key(day.String()).
func (d Date) String() string {
	if d.IsZero() {
		return "none"
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// DateRange is a span of days, both ends included: the value of a range
// calendar. It is the zero value when nothing is selected; Start is never
// after End in a range a calendar makes.
type DateRange struct {
	Start, End Date
}

// IsZero reports whether r is the empty selection.
func (r DateRange) IsZero() bool { return r == DateRange{} }

// Contains reports whether d is in r, its ends included.
func (r DateRange) Contains(d Date) bool {
	return !r.IsZero() && d.Compare(r.Start) >= 0 && d.Compare(r.End) <= 0
}

// firstOfMonth is the first day of d's month.
func (d Date) firstOfMonth() Date { return Date{d.Year, d.Month, 1} }

// monthsUntil is the number of months from the month of d to the month of o.
func (d Date) monthsUntil(o Date) int {
	return (o.Year-d.Year)*12 + int(o.Month) - int(d.Month)
}

func daysIn(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// orderedRange returns a and b as a range, the earlier one first.
func orderedRange(a, b Date) DateRange {
	if b.Before(a) {
		a, b = b, a
	}
	return DateRange{a, b}
}
