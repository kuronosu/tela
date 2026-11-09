package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// The tests of this file are what the examples/i18n order summary used to
// check (it was merged into the gallery), on the Internationalization page and
// the header's Language menu: catalogs, plural categories, the formatter, kit's
// English defaults coming from the catalog, the calendar of each language and
// the window mirrored for Arabic.

// canvasIn is the Internationalization page, without the sidebar, in a
// language.
func canvasIn(t *testing.T, tag string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mount(t, Canvas{Page: "Internationalization", Locale: tag}, opts...)
}

var (
	deliveryButton = telatest.ByTestID("delivery")
	popover        = telatest.ByRole("application", "")
)

// oct is a day of October 2026, the month of the gallery's "today".
func oct(d int) aria.Date { return aria.Date{Year: 2026, Month: time.October, Day: d} }

// cell is the day cell of a date, named by the locale's full date.
func cell(tag string, d aria.Date) telatest.Locator {
	return telatest.ByRole("gridcell", i18n.For(tag).Date(d.Time(nil), i18n.Full))
}

// valueOf expects the row with the test id to read want.
func valueOf(a *telatest.App, id, want string) {
	a.ExpectCount(telatest.ByTestID(id).Filter(telatest.HasText(want)), 1)
}

// chooseLanguage picks a language in the header's menu, as a person does.
func chooseLanguage(a *telatest.App, name string) {
	a.Click(languageMenu)
	a.Click(telatest.ByRole("option", name))
}

// TestTranslations checks the page's texts in each language: arguments,
// plurals, the ordinal, nested keys and the formatter's numbers, percentages,
// dates, times and lists, and kit's own defaults from the catalog (the date
// picker's placeholder).
func TestTranslations(t *testing.T) {
	for _, c := range []struct {
		tag   string
		texts []string
	}{
		{"en", []string{"Hello, Ana!", "Step 3 of 4", "3 new messages", "3 items in your cart", "You are 3rd in line", "Discount", "Updated",
			"1,234.50", "15%", "Saturday, March 14, 2026", "3:30 PM", "apples, pears, and figs",
			"Delivery date", "Pick a date", "We do not deliver on Sundays."}},
		{"es", []string{"¡Hola, Ana!", "Paso 3 de 4", "3 mensajes nuevos", "3 artículos en tu carrito", "Eres el 3.º de la fila", "Descuento", "Actualizado",
			"1.234,50", "15 %", "sábado, 14 de marzo de 2026", "15:30", "manzanas, peras y higos",
			"Fecha de entrega", "Elige una fecha", "No entregamos los domingos."}},
		{"ar", []string{"مرحبا يا Ana!", "الخطوة 3 من 4", "3 رسائل جديدة", "3 عناصر في سلتك", "الخصم", "آخر تحديث",
			"1,234.50", "15\u200e%\u200e", "السبت، 14 مارس 2026", "3:30 م", "تفاح وكمثرى وتين",
			"تاريخ التوصيل", "اختر تاريخًا", "لا نوصل أيام الأحد."}},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := canvasIn(t, c.tag)
			for _, s := range c.texts {
				a.ExpectText(s)
			}
		})
	}
}

// TestPluralCategories walks the presets of the counter in Arabic, which has
// the six plural categories, and in English, which has two: the message and
// the category the language's rules choose change with the count.
func TestPluralCategories(t *testing.T) {
	a := canvasIn(t, "ar")
	for _, c := range []struct {
		n                 string // the preset, in the language's digits
		category, message string
	}{
		{"0", "zero", "لا توجد رسائل جديدة"},
		{"1", "one", "رسالة جديدة واحدة"},
		{"2", "two", "رسالتان جديدتان"},
		{"3", "few", "3 رسائل جديدة"},
		{"11", "many", "11 رسالة جديدة"},
		{"100", "other", "100 رسالة جديدة"},
	} {
		a.Click(telatest.ByRole("radio", c.n))
		a.Expect(telatest.ByRole("radio", c.n), telatest.Checked(true))
		valueOf(a, "category", c.category)
		valueOf(a, "plural-messages", c.message)
	}

	b := canvasIn(t, "en")
	for _, c := range []struct {
		n, category, message, items, ordinal string
	}{
		{"0", "other", "No new messages", "0 items in your cart", "You are 0th in line"},
		{"1", "one", "1 new message", "1 item in your cart", "You are 1st in line"},
		{"2", "other", "2 new messages", "2 items in your cart", "You are 2nd in line"},
		{"11", "other", "11 new messages", "11 items in your cart", "You are 11th in line"},
		{"100", "other", "100 new messages", "100 items in your cart", "You are 100th in line"},
	} {
		b.Click(telatest.ByRole("radio", c.n))
		valueOf(b, "category", c.category)
		valueOf(b, "plural-messages", c.message)
		valueOf(b, "plural-items", c.items)
		valueOf(b, "ordinal", c.ordinal)
	}
}

// TestCounterButtons counts with the minus and plus buttons, named from the
// catalog, down to zero (where minus is disabled) and up.
func TestCounterButtons(t *testing.T) {
	for _, c := range []struct{ tag, remove, add, zero string }{
		{"en", "Remove one", "Add one", "No new messages"},
		{"es", "Quitar uno", "Añadir uno", "Sin mensajes nuevos"},
		{"ar", "إزالة واحد", "إضافة واحد", "لا توجد رسائل جديدة"},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := canvasIn(t, c.tag)
			f := i18n.For(c.tag)
			remove, add := telatest.ByRole("button", c.remove), telatest.ByRole("button", c.add)
			for range 3 {
				a.Click(remove)
			}
			a.Expect(remove, telatest.Disabled(true))
			valueOf(a, "count", f.Number(0))
			valueOf(a, "plural-messages", c.zero)
			a.Click(add)
			valueOf(a, "count", f.Number(1))
			a.Expect(remove, telatest.Disabled(false))
		})
	}
}

// TestKitDefaultsFromTheCatalog: what kit defaults to English is the
// language's, wherever a page leaves the default: the date pickers and calendars,
// the dialogs' close button and the alert dialog's Cancel and Continue.
func TestKitDefaultsFromTheCatalog(t *testing.T) {
	for _, c := range []struct {
		tag                                                  string
		pick, rangeText, previous, next, close, cancel, cont string
	}{
		{"en", "Pick a date", "Pick a date range", "Previous", "Next", "Close", "Cancel", "Continue"},
		{"es", "Elige una fecha", "Elige un rango de fechas", "Anterior", "Siguiente", "Cerrar", "Cancelar", "Continuar"},
		{"ar", "اختر تاريخًا", "اختر نطاق تواريخ", "السابق", "التالي", "إغلاق", "إلغاء", "متابعة"},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := mount(t, Canvas{Page: "DatePicker", Locale: c.tag})
			a.ExpectText(c.pick)
			a.Click(telatest.ByRole("button", "Date"))
			a.ExpectCount(telatest.ByRole("button", c.previous), 1)
			a.ExpectCount(telatest.ByRole("button", c.next), 1)

			b := mount(t, Canvas{Page: "DateRangePicker", Locale: c.tag})
			b.ExpectText(c.rangeText)

			d := mount(t, Canvas{Page: "Dialog", Locale: c.tag})
			d.Click(telatest.ByRole("button", "Share"))
			d.Click(telatest.ByRole("button", c.close))
			d.ExpectCount(telatest.ByRole("dialog", ""), 0)

			e := mount(t, Canvas{Page: "AlertDialog", Locale: c.tag})
			e.Click(telatest.ByRole("button", "Publish"))
			e.ExpectCount(telatest.ByRole("button", c.cancel), 1)
			e.ExpectCount(telatest.ByRole("button", c.cont), 1)
		})
	}
}

// TestAriaSnapshots pin what `tela inspect --aria` prints for the page in
// each language, and with the calendar open: the names of the controls, the
// picker and the calendar come from the catalog.
func TestAriaSnapshots(t *testing.T) {
	for _, tag := range languages {
		canvasIn(t, tag).AriaGolden("kit_internationalization_" + tag + ".aria.yml")
	}
	for _, tag := range languages {
		a := canvasIn(t, tag)
		a.Click(deliveryButton)
		a.AriaGolden("kit_internationalization_" + tag + "_picker.aria.yml")
	}
	mount(t, Gallery{Page: "Internationalization", Locale: "ar"}).AriaGolden("kit_gallery_ar.aria.yml")
}

// TestCalendarFollowsTheLanguage opens the page's calendar in each language:
// the month and year, the weekday order, the digits and the direction are the
// language's.
func TestCalendarFollowsTheLanguage(t *testing.T) {
	for _, c := range []struct {
		tag, heading, first, last string
		rtl                       bool
	}{
		{"en", "October 2026", "Sunday", "Saturday", false},
		{"es", "octubre de 2026", "lunes", "domingo", false},
		{"ar", "أكتوبر 2026", "السبت", "الجمعة", true},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := canvasIn(t, c.tag)
			a.Click(deliveryButton)
			a.Expect(deliveryButton, telatest.Expanded(true))
			a.ExpectCount(popover, 1)
			a.ExpectRole("heading", c.heading)
			a.ExpectFocused(cell(c.tag, today))

			// The weekday columns start with the language's first weekday,
			// from the right in Arabic.
			a.ExpectCount(telatest.ByRole("columnheader", ""), 7)
			first := a.Bounds(telatest.ByRole("columnheader", c.first))
			last := a.Bounds(telatest.ByRole("columnheader", c.last))
			if (first.X > last.X) != c.rtl {
				t.Errorf("the first weekday %q is at x=%v and the last %q at x=%v: rtl is %v", c.first, first.X, c.last, last.X, c.rtl)
			}
			// The month buttons are named from the catalog and mirrored.
			prev := a.Bounds(telatest.ByRole("button", i18n.For(c.tag).T("kit.previous")))
			next := a.Bounds(telatest.ByRole("button", i18n.For(c.tag).T("kit.next")))
			if c.rtl == (prev.X < next.X) {
				t.Errorf("previous at x=%v and next at x=%v: rtl is %v", prev.X, next.X, c.rtl)
			}
			// Min is today and Sundays are unavailable.
			a.Expect(cell(c.tag, oct(24)), telatest.Disabled(true))
			a.Expect(cell(c.tag, oct(25)), telatest.Disabled(true))
			a.Expect(cell(c.tag, oct(26)), telatest.Disabled(false))
		})
	}
}

// TestChooseDeliveryDate picks a date in each language: the picker closes,
// and its button and the readout show the date as the language writes it.
func TestChooseDeliveryDate(t *testing.T) {
	for _, c := range []struct {
		tag, button, readout string
	}{
		{"en", "October 28, 2026", "Wednesday, October 28, 2026"},
		{"es", "28 de octubre de 2026", "miércoles, 28 de octubre de 2026"},
		{"ar", "28 أكتوبر 2026", "الأربعاء، 28 أكتوبر 2026"},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := canvasIn(t, c.tag)
			a.Click(deliveryButton)
			a.Click(cell(c.tag, oct(28)))
			a.ExpectCount(popover, 0)
			a.ExpectFocused(deliveryButton)
			a.ExpectText(c.button)
			a.ExpectText(c.readout)
			// Opening again focuses the chosen date.
			a.Press("Enter")
			a.ExpectFocused(cell(c.tag, oct(28)))
			a.Expect(cell(c.tag, oct(28)), telatest.Selected(true))
		})
	}
}

// TestLanguageMenuSwitchesTheWindow chooses the language in the header: the
// page, the picker and the date already chosen turn into the new language's
// while the page keeps its state, and the window mirrors for Arabic and comes
// back for Spanish.
func TestLanguageMenuSwitchesTheWindow(t *testing.T) {
	a := mount(t, Gallery{Page: "Internationalization"})
	a.Expect(languageMenu, telatest.HasName("English Language"))
	a.ExpectText("Hello, Ana!")
	a.Click(deliveryButton)
	a.Click(cell("en", oct(28)))
	a.ExpectText("Wednesday, October 28, 2026")

	introTab, pageTab := tabFor("Introduction"), tabFor("Alert Dialog")
	ltrTab, ltrMenu, ltrStyle := a.Bounds(pageTab), a.Bounds(languageMenu), a.Bounds(telatest.ByRole("radio", "Default"))
	if ltrTab.X > 40 {
		t.Fatalf("en: the sidebar is at x=%v, want it at the left edge", ltrTab.X)
	}

	chooseLanguage(a, "العربية")
	a.ExpectText("مرحبا يا Ana!")
	a.ExpectNoText("Hello, Ana!")
	a.Expect(languageMenu, telatest.HasName("العربية اللغة"))
	a.ExpectText("الأربعاء، 28 أكتوبر 2026") // the chosen date, in Arabic, state kept
	rtlTab, rtlMenu, rtlStyle := a.Bounds(pageTab), a.Bounds(languageMenu), a.Bounds(telatest.ByRole("radio", "Default"))
	if rtlTab.X < 1280-224 {
		t.Errorf("ar: the sidebar is at x=%v, want it at the right edge (mirrored)", rtlTab.X)
	}
	if rtlMenu.X > 40 || ltrMenu.X < 1000 {
		t.Errorf("ar: the Language menu is at x=%v (en: %v), want it at the left end of the header", rtlMenu.X, ltrMenu.X)
	}
	if rtlStyle.X >= ltrStyle.X-200 {
		t.Errorf("ar: the Style group is at x=%v (en: %v), want it mirrored", rtlStyle.X, ltrStyle.X)
	}

	// With the calendar open the weekdays run from the right and the
	// language's names are used; then Spanish.
	a.Click(deliveryButton)
	a.ExpectRole("heading", "أكتوبر 2026")
	a.Expect(cell("ar", oct(28)), telatest.Selected(true))
	a.Press("Escape")

	chooseLanguage(a, "Español")
	a.ExpectText("¡Hola, Ana!")
	a.ExpectText("miércoles, 28 de octubre de 2026")
	a.Expect(languageMenu, telatest.HasName("Español Idioma"))
	if b := a.Bounds(pageTab); b.X != ltrTab.X {
		t.Errorf("es: the sidebar at x=%v, want %v as in English", b.X, ltrTab.X)
	}
	if b := a.Bounds(languageMenu); b.X != ltrMenu.X {
		t.Errorf("es: the Language menu at x=%v, want %v as in English", b.X, ltrMenu.X)
	}
	a.Click(deliveryButton)
	a.ExpectRole("heading", "octubre de 2026")
	a.ExpectRole("columnheader", "lunes")
	a.Expect(cell("es", oct(28)), telatest.Selected(true))
	a.Press("Escape")

	// The language is the window's, not the page's: it stays when another
	// page opens, and the whole sidebar is in the new direction.
	a.Click(introTab)
	a.ExpectRole("heading", "Hello, kit.")
	a.Expect(languageMenu, telatest.HasName("Español Idioma"))
}

// TestLanguageMenuKeyboard chooses a language with the keyboard alone.
func TestLanguageMenuKeyboard(t *testing.T) {
	a := mount(t, Gallery{Page: "Internationalization"})
	a.Focus(languageMenu)
	a.Press("ArrowDown")
	a.Expect(telatest.ByRole("option", "English"), telatest.Focused(true))
	a.Press("ArrowDown")
	a.Press("Enter")
	a.ExpectText("¡Hola, Ana!")
	a.ExpectCount(telatest.ByRole("option", ""), 0)
	a.Expect(languageMenu, telatest.HasName("Español Idioma"))
	a.ExpectFocused(languageMenu)
}

// TestLanguageMenuOptions: each option is named in its own language.
func TestLanguageMenuOptions(t *testing.T) {
	a := mount(t, Gallery{})
	a.Click(languageMenu)
	a.ExpectCount(telatest.ByRole("option", ""), len(languages))
	for _, name := range []string{"English", "Español", "العربية"} {
		a.ExpectCount(telatest.ByRole("option", name), 1)
	}
}

// TestComponentPagesMirrorInArabic opens a few pages of components in Arabic:
// the window is right to left, so the sidebar is at the right.
func TestComponentPagesMirrorInArabic(t *testing.T) {
	for _, name := range []string{"Button", "Card", "Table"} {
		t.Run(name, func(t *testing.T) {
			ltr := mount(t, Gallery{Page: name})
			rtl := mount(t, Gallery{Page: name, Locale: "ar"})
			if lb, rb := ltr.Bounds(tabFor(find(name).Label)), rtl.Bounds(tabFor(find(name).Label)); rb.X <= lb.X || rb.X < 1000 {
				t.Errorf("%s: the sidebar tab is at x=%v in Arabic, %v in English: want it at the right", name, rb.X, lb.X)
			}
		})
	}
}
