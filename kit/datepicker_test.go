package kit_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's date picker (apps/v4/registry/new-york-v4/examples/
// date-picker-demo.tsx at kit.ShadcnCommit): an outline Button with a
// calendar icon, whose classes are the demo's.
const (
	shadcnDatePickerButton      = "w-[240px] justify-start text-left font-normal"
	shadcnDatePickerRangeButton = "w-[300px] justify-start text-left font-normal"
	shadcnCalendarIcon          = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-calendar"><path d="M8 2v3"></path><path d="M16 2v3"></path><rect x="3" y="3" width="18" height="18" rx="2"></rect><path d="M3 9h18"></path></svg>`
)

// datePickerHTML is the markup of shadcn's date picker button with text.
func datePickerHTML(probe, text string, empty, invalid, disabled, rng bool) string {
	class := shadcnDatePickerButton
	if rng {
		class = shadcnDatePickerRangeButton
	}
	class = cn(shadcnButton, shadcnButtonOutline, shadcnButtonSizeDefault, class)
	if empty {
		class = cn(class, "text-muted-foreground")
	}
	attrs := ""
	if invalid {
		attrs += ` aria-invalid="true"`
	}
	if disabled {
		attrs += " disabled"
	}
	return fmt.Sprintf(`<button type="button" data-probe=%q%s class="%s">%s<span>%s</span></button>`, probe, attrs, class, shadcnCalendarIcon, text)
}

func datePickerCells() []cell {
	oct := func(day int) aria.Date { return calDate(2026, time.October, day) }
	day := oct(14)
	r := aria.DateRange{Start: oct(12), End: oct(18)}
	mk := func(name string, c tela.Component, html string) cell {
		return cell{name: name, tela: c, html: html, probes: []string{name + "-button"}}
	}
	return []cell{
		mk("empty", kit.DatePicker{Props: h.TestID("empty-button")}, datePickerHTML("empty-button", "Pick a date", true, false, false, false)),
		mk("value", kit.DatePicker{DefaultValue: day, Props: h.TestID("value-button")}, datePickerHTML("value-button", "October 14, 2026", false, false, false, false)),
		mk("disabled", kit.DatePicker{Disabled: true, Props: h.TestID("disabled-button")}, datePickerHTML("disabled-button", "Pick a date", true, false, true, false)),
		mk("invalid", kit.DatePicker{DefaultValue: day, Invalid: true, Props: h.TestID("invalid-button")}, datePickerHTML("invalid-button", "October 14, 2026", false, true, false, false)),
		mk("range", kit.DateRangePicker{DefaultValue: r, Props: h.TestID("range-button")}, datePickerHTML("range-button", "Oct 12, 2026 - Oct 18, 2026", false, false, false, true)),
		mk("rangeempty", kit.DateRangePicker{Props: h.TestID("rangeempty-button")}, datePickerHTML("rangeempty-button", "Pick a date range", true, false, false, true)),
	}
}

func datePickerSheet() *sheet {
	return &sheet{name: "datepicker", cells: datePickerCells(), slot: 320}
}

func TestDatePickerGolden(t *testing.T)   { datePickerSheet().golden(t) }
func TestChromiumDatePicker(t *testing.T) { datePickerSheet().chromium(t) }

func pickerPage(c tela.Component) func() tela.Node {
	return func() tela.Node {
		return h.Div(h.Class("flex flex-col items-start gap-4 p-6"),
			h.P(h.Class("text-sm text-muted-foreground"), h.Text("Pick the day of your appointment.")),
			c,
			kit.Button{Variant: kit.Secondary, Children: h.Text("Save changes")})
	}
}

func pickerMount(t *testing.T, c tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.MountFunc(t, pickerPage(c), append([]telatest.Option{telatest.Size(800, 520), telatest.Theme(kit.Theme())}, opts...)...)
}

var pickerButton = telatest.ByTestID("picker")

func TestDatePickerOpensChoosesAndCloses(t *testing.T) {
	var got []aria.Date
	app := pickerMount(t, kit.DatePicker{Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), OnValueChange: func(day aria.Date) { got = append(got, day) }})
	app.Expect(pickerButton, telatest.HasPopup("dialog"), telatest.Expanded(false), telatest.HasName("Appointment"))
	app.ExpectCount(telatest.ByRole("application", ""), 0)
	app.Click(pickerButton)
	app.Expect(pickerButton, telatest.Expanded(true))
	// The focus goes to the focused date of the calendar: today.
	app.ExpectFocused(calCell(calToday))
	app.Click(calCell(calDate(2026, time.October, 28)))
	if fmt.Sprint(got) != "[2026-10-28]" {
		t.Errorf("OnValueChange got %v, want [2026-10-28]", got)
	}
	// Choosing closes the popover and gives the focus back to the button,
	// which shows the date and is named with it.
	app.ExpectCount(telatest.ByRole("application", ""), 0)
	app.ExpectFocused(pickerButton)
	app.ExpectText("October 28, 2026")
	app.Expect(pickerButton, telatest.HasName("Appointment, October 28, 2026"))
	// Opening again focuses the chosen date.
	app.Press("Enter")
	app.ExpectFocused(calCell(calDate(2026, time.October, 28)))
	app.Expect(calCell(calDate(2026, time.October, 28)), telatest.Selected(true))
}

func TestDatePickerKeyboardFlow(t *testing.T) {
	app := pickerMount(t, kit.DatePicker{Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker")})
	app.Focus(pickerButton)
	app.Press("Space")
	app.ExpectFocused(calCell(calToday))
	app.Press("ArrowLeft")
	app.Press("PageDown")
	app.ExpectRole("heading", "November 2026")
	app.Press("Enter")
	app.ExpectCount(telatest.ByRole("application", ""), 0)
	app.ExpectFocused(pickerButton)
	app.ExpectText("November 24, 2026")
}

func TestDatePickerEscapeAndOutsidePressChooseNothing(t *testing.T) {
	var got []aria.Date
	app := pickerMount(t, kit.DatePicker{Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), OnValueChange: func(day aria.Date) { got = append(got, day) }})
	app.Click(pickerButton)
	app.Press("ArrowRight")
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("application", ""), 0)
	app.ExpectFocused(pickerButton)
	app.Click(pickerButton)
	app.Click(telatest.ByText("Pick the day of your appointment."))
	app.ExpectCount(telatest.ByRole("application", ""), 0)
	app.ExpectText("Pick a date")
	if len(got) != 0 {
		t.Errorf("OnValueChange got %v, want nothing", got)
	}
	// Month buttons keep the popover open and the focus where it is.
	app.Click(pickerButton)
	app.Click(telatest.ByRole("button", "Next"))
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	app.ExpectRole("heading", "November 2026")
	app.ExpectFocused(telatest.ByRole("button", "Next"))
}

func TestDatePickerValueAndOpenAreControllable(t *testing.T) {
	chosen := calDate(2026, time.October, 3)
	var asked []aria.Date
	var opened []bool
	open := true
	app := pickerMount(t, kit.DatePicker{
		Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), Value: &chosen, Open: &open,
		OnValueChange: func(day aria.Date) { asked = append(asked, day) },
		OnOpenChange:  func(o bool) { opened = append(opened, o) },
	})
	app.Expect(calCell(chosen), telatest.Selected(true))
	app.Click(calCell(calDate(2026, time.October, 9)))
	// The parent stored neither: the date and the popover are as they were,
	// and both were asked for.
	app.Expect(calCell(chosen), telatest.Selected(true))
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	if fmt.Sprint(asked) != "[2026-10-09]" || fmt.Sprint(opened) != "[false]" {
		t.Errorf("asked for %v and %v, want [2026-10-09] and [false]", asked, opened)
	}
}

func TestDatePickerKeepOpenAndDefaults(t *testing.T) {
	app := pickerMount(t, kit.DatePicker{Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), KeepOpen: true, DefaultOpen: true,
		DefaultValue: calDate(2026, time.October, 3)})
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	app.Click(calCell(calDate(2026, time.October, 9)))
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	app.ExpectText("October 9, 2026")
}

func TestDatePickerLimitsFormatPlaceholderAndLocale(t *testing.T) {
	app := pickerMount(t, kit.DatePicker{
		Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), Placeholder: "Choose a day", DefaultOpen: true,
		Min: calDate(2026, time.October, 20), IsDateUnavailable: func(day aria.Date) bool { return day == calDate(2026, time.October, 27) },
		Format: func(f i18n.Formatter, day aria.Date) string { return f.Date(day.Time(nil), i18n.Short) },
	})
	app.ExpectText("Choose a day")
	app.Expect(calCell(calDate(2026, time.October, 19)), telatest.Disabled(true))
	pressDisabled(app, calCell(calDate(2026, time.October, 27)))
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	app.Click(calCell(calDate(2026, time.October, 26)))
	app.ExpectText("10/26/26")

	es := pickerMount(t, kit.DatePicker{Today: calToday, Locale: "es", AriaLabel: "Cita", Props: h.TestID("picker"), DefaultValue: calDate(2026, time.October, 25)})
	es.ExpectText("25 de octubre de 2026")
	es.Click(pickerButton)
	es.ExpectRole("heading", "octubre de 2026")
}

func TestDatePickerInsideAField(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Field{Label: "Appointment", Description: "Weekdays only.", Error: "Pick a weekday.",
			Children: kit.DatePicker{Today: calToday}})
	}, telatest.Size(600, 500), telatest.Theme(kit.Theme()))
	btn := telatest.ByRole("button", "Appointment")
	app.Expect(btn, telatest.Invalid(true))
	// The label points to the button: clicking it opens the popover.
	app.Click(telatest.ByText("Appointment"))
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	app.Press("Escape")
	app.ExpectFocused(btn)
}

func TestDatePickerDisabled(t *testing.T) {
	app := pickerMount(t, kit.DatePicker{Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), Disabled: true})
	app.Expect(pickerButton, telatest.Disabled(true))
	pressDisabled(app, pickerButton)
	app.ExpectCount(telatest.ByRole("application", ""), 0)
}

func TestDatePickerStyleAndClasses(t *testing.T) {
	s := kit.DefaultStyle()
	s.DatePicker.Root = "w-[320px] justify-start text-left font-normal"
	s.DatePicker.Content = "w-auto p-2"
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: tela.C(pickerPage(kit.DatePicker{
			Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), DefaultOpen: true, Class: "border-dashed", PopoverClass: "shadow-xl", CalendarClass: "p-0",
		}))})
	}, telatest.Size(800, 520), telatest.Theme(kit.Theme()))
	for _, class := range []string{"w-[320px]", "border-dashed", "shadow-xl", "p-2"} {
		if n := app.Count(telatest.ByClass(class)); n != 1 {
			t.Errorf("%d elements have %s, want 1", n, class)
		}
	}
	if w := app.Bounds(pickerButton).Width; w != 320 {
		t.Errorf("the button is %v wide, want 320", w)
	}
}

func TestDateRangePickerChoosesAndCloses(t *testing.T) {
	var got []aria.DateRange
	app := pickerMount(t, kit.DateRangePicker{Today: calToday, AriaLabel: "Stay", Props: h.TestID("picker"), OnValueChange: func(r aria.DateRange) { got = append(got, r) }})
	app.ExpectText("Pick a date range")
	app.Click(pickerButton)
	// Two months, side by side.
	app.ExpectCount(telatest.ByRole("grid", ""), 2)
	app.Click(calCell(calDate(2026, time.October, 28)))
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	if len(got) != 0 {
		t.Fatalf("the first press completed the range: %v", got)
	}
	app.Click(calCell(calDate(2026, time.November, 3)))
	app.ExpectCount(telatest.ByRole("application", ""), 0)
	app.ExpectFocused(pickerButton)
	app.ExpectText("Oct 28, 2026 - Nov 3, 2026")
	if fmt.Sprint(got) != "[{2026-10-28 2026-11-03}]" {
		t.Errorf("OnValueChange got %v, want [{2026-10-28 2026-11-03}]", got)
	}
}

func TestDateRangePickerEscapeCancelsTheAnchorBeforeClosing(t *testing.T) {
	var got []aria.DateRange
	app := pickerMount(t, kit.DateRangePicker{Today: calToday, AriaLabel: "Stay", Props: h.TestID("picker"), OnValueChange: func(r aria.DateRange) { got = append(got, r) }})
	btn := pickerButton
	app.Click(btn)
	app.Press("Enter") // anchors the range at today
	app.Press("ArrowRight")
	app.Expect(calCell(calDate(2026, time.October, 26)), telatest.Selected(true))
	// The first Escape cancels the anchor and leaves the popover open...
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	app.Expect(calCell(calDate(2026, time.October, 26)), telatest.Selected(false))
	app.Expect(calCell(calDate(2026, time.October, 25)), telatest.Selected(false))
	// ...the next one closes it, and nothing was chosen.
	app.Press("Escape")
	app.ExpectCount(telatest.ByRole("application", ""), 0)
	app.ExpectFocused(btn)
	app.ExpectText("Pick a date range")
	if len(got) != 0 {
		t.Errorf("OnValueChange got %v, want nothing", got)
	}
}

func TestDateRangePickerKeepOpenAndSingleMonth(t *testing.T) {
	app := pickerMount(t, kit.DateRangePicker{Today: calToday, AriaLabel: "Stay", Props: h.TestID("picker"), KeepOpen: true, VisibleMonths: 1, DefaultOpen: true})
	app.ExpectCount(telatest.ByRole("grid", ""), 1)
	app.Click(calCell(calDate(2026, time.October, 6)))
	app.Click(calCell(calDate(2026, time.October, 8)))
	app.ExpectCount(telatest.ByRole("application", ""), 1)
	app.ExpectText("Oct 6, 2026 - Oct 8, 2026")
}

func TestDatePickerOpenGolden(t *testing.T) {
	page := func() tela.Node {
		return h.Div(h.Class("flex flex-col items-start gap-4 p-6"),
			kit.DatePicker{Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), DefaultValue: calDate(2026, time.October, 14)})
	}
	openGolden(t, "datepicker", 400, 420, page, func(a *telatest.App) { a.Click(pickerButton) })
}

func TestDateRangePickerOpenGolden(t *testing.T) {
	page := func() tela.Node {
		return h.Div(h.Class("flex flex-col items-start gap-4 p-6"),
			kit.DateRangePicker{Today: calToday, AriaLabel: "Stay", Props: h.TestID("picker"), DefaultValue: aria.DateRange{
				Start: calDate(2026, time.October, 20), End: calDate(2026, time.November, 5)}})
	}
	openGolden(t, "daterangepicker", 800, 420, page, func(a *telatest.App) { a.Click(pickerButton) })
}

func TestDatePickerIdle(t *testing.T) {
	expectIdle(t, pickerPage(kit.DatePicker{Today: calToday, AriaLabel: "Appointment", Props: h.TestID("picker"), DefaultOpen: true}), nil)
	expectIdle(t, pickerPage(kit.DateRangePicker{Today: calToday, AriaLabel: "Stay", Props: h.TestID("picker"), DefaultOpen: true}), nil)
}
