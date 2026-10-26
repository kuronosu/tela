package kit_test

import "testing"

// The right-to-left sheets (ADR-151): each is the sheet of a component with
// its panel under dir="rtl", and its golden is the light panel at scale 1
// (testdata/<name>_rtl_light_x1.png). The cells are the component's own, so
// every recipe is looked at mirrored. The sheets that have Chromium
// counterparts are in rtl_chromium_test.go.

// rtlOf returns the sheet laid out right to left.
func rtlOf(s *sheet) *sheet {
	c := *s
	c.name += "_rtl"
	c.rtl = true
	return &c
}

func TestRTLGoldens(t *testing.T) {
	sheets := map[string]func(testing.TB) *sheet{
		"alertdialog":   func(testing.TB) *sheet { return alertDialogSheet() },
		"avatar":        func(testing.TB) *sheet { return avatarSheet() },
		"badge":         func(testing.TB) *sheet { return badgeSheet() },
		"button":        func(testing.TB) *sheet { return buttonSheet() },
		"calendar":      calendarSheet,
		"rangecalendar": rangeCalendarSheet,
		"card":          func(testing.TB) *sheet { return cardSheet() },
		"checkbox":      func(testing.TB) *sheet { return checkboxSheet() },
		"colorinput":    func(testing.TB) *sheet { return colorInputSheet() },
		"colorpicker":   func(testing.TB) *sheet { return colorPickerSheet() },
		"contextmenu":   func(testing.TB) *sheet { return contextSheet() },
		"datepicker":    func(testing.TB) *sheet { return datePickerSheet() },
		"dialog":        func(testing.TB) *sheet { return dialogSheet() },
		"dropdownmenu":  func(testing.TB) *sheet { return dropdownSheet() },
		"field":         func(testing.TB) *sheet { return fieldSheet() },
		"fieldset":      func(testing.TB) *sheet { return fieldSetSheet() },
		"input":         func(testing.TB) *sheet { return inputSheet() },
		"label":         func(testing.TB) *sheet { return labelSheet() },
		"menubar":       func(testing.TB) *sheet { return menubarSheet() },
		"popover":       func(testing.TB) *sheet { return popoverSheet() },
		"progress":      func(testing.TB) *sheet { return progressSheet() },
		"radiogroup":    func(testing.TB) *sheet { return radioGroupSheet() },
		"select":        func(testing.TB) *sheet { return selectSheet() },
		"slider":        func(testing.TB) *sheet { return sliderSheet() },
		"switch":        func(testing.TB) *sheet { return switchSheet() },
		"table":         func(testing.TB) *sheet { return tableSheet() },
		"tabs":          func(testing.TB) *sheet { return tabsSheet() },
		"textarea":      func(testing.TB) *sheet { return textareaSheet() },
		"toast":         toastSheet,
		"toggle":        func(testing.TB) *sheet { return toggleSheet() },
		"togglegroup":   func(testing.TB) *sheet { return toggleGroupSheet() },
		"tooltip":       func(testing.TB) *sheet { return tooltipSheet() },
	}
	for name, mk := range sheets {
		t.Run(name, func(t *testing.T) { rtlOf(mk(t)).golden(t) })
	}
}

// TestChromiumRTL compares the right-to-left sheets of the components whose
// shadcn markup, in its logical classes (rtlMarkup), Chromium renders
// right to left: the boxes of the probes and the pixels around each cell.
func TestChromiumRTL(t *testing.T) {
	sheets := map[string]func() *sheet{
		"alertdialog":  alertDialogSheet,
		"avatar":       avatarSheet,
		"badge":        badgeSheet,
		"button":       buttonSheet,
		"card":         cardSheet,
		"checkbox":     checkboxSheet,
		"contextmenu":  contextSheet,
		"dialog":       dialogSheet,
		"dropdownmenu": dropdownSheet,
		"field":        fieldSheet,
		"fieldset":     fieldSetSheet,
		"input":        inputSheet,
		"label":        labelSheet,
		"popover":      popoverSheet,
		"menubar":      menubarSheet,
		"progress":     progressSheet,
		"radiogroup":   radioGroupSheet,
		"select":       selectSheet,
		"slider":       sliderSheet,
		"switch":       switchSheet,
		"table":        tableSheet,
		"tabs":         tabsSheet,
		"textarea":     textareaSheet,
		"toggle":       toggleSheet,
		"togglegroup":  toggleGroupSheet,
		"tooltip":      tooltipSheet,
	}
	for name, mk := range sheets {
		t.Run(name, func(t *testing.T) { rtlOf(mk()).chromium(t) })
	}
}
