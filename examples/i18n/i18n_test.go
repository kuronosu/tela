package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

// mountIn opens the screen with its locale fixed by i18n.Provider (the
// preview of that language), in the window of App; dev mode is on, so any
// dev message fails the test.
func mountIn(t *testing.T, tag string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = Previews[tag]
	return telatest.MountApp(t, app, opts...)
}

// mountLive opens the screen as main does, with the locale set at run time
// by i18n.SetLocale (process-wide: tests that use it must not run in
// parallel with others that change the locale).
func mountLive(t *testing.T, tag string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	i18n.SetLocale(tag)
	t.Cleanup(func() { i18n.SetLocale("") })
	return telatest.MountApp(t, App, opts...)
}

var (
	deliveryButton = telatest.ByTestID("delivery")
	languageSelect = telatest.ByTestID("language")
	popover        = telatest.ByRole("application", "")
)

// march is a day of March 2026, the month of the order.
func march(d int) aria.Date { return aria.Date{Year: 2026, Month: time.March, Day: d} }

// cell is the day cell of a date of the delivery calendar, named by the
// locale's full date.
func cell(tag string, d aria.Date) telatest.Locator {
	return telatest.ByRole("gridcell", i18n.For(tag).Date(d.Time(nil), i18n.Full))
}

func TestGolden(t *testing.T) {
	mountIn(t, "en").GoldenScales("i18n_en", 1, 2)
	mountIn(t, "es").GoldenScales("i18n_es", 1)
	mountIn(t, "ar").GoldenScales("i18n_ar", 1, 2)

	// With the delivery date chosen, in Arabic: the button and the summary
	// show it in Arabic digits.
	a := mountIn(t, "ar")
	a.Click(deliveryButton)
	a.Click(cell("ar", march(17)))
	a.GoldenScales("i18n_chosen_ar", 1)
}

func TestGoldenDark(t *testing.T) {
	mountIn(t, "en", telatest.Dark()).GoldenScales("i18n_dark_en", 1)
	mountIn(t, "ar", telatest.Dark()).GoldenScales("i18n_dark_ar", 1)
	a := mountIn(t, "ar", telatest.Dark())
	a.Click(deliveryButton)
	a.GoldenScales("i18n_picker_dark_ar", 1)
}

// TestGoldenOpen are the delivery calendar and the language list open, in
// each language: the month and weekday names, their order, the digits and
// the mirrored layout are the locale's.
func TestGoldenOpen(t *testing.T) {
	for _, tag := range Languages {
		a := mountIn(t, tag)
		a.Click(deliveryButton)
		a.ExpectFocused(cell(tag, today))
		if tag == "ar" {
			a.GoldenScales("i18n_picker_"+tag, 1, 2)
		} else {
			a.GoldenScales("i18n_picker_"+tag, 1)
		}

		b := mountIn(t, tag)
		b.Click(languageSelect)
		b.ExpectCount(telatest.ByRole("option", ""), 3)
		b.GoldenScales("i18n_select_"+tag, 1)
	}
}

func TestTranslations(t *testing.T) {
	for _, c := range []struct {
		tag   string
		texts []string
	}{
		{"en", []string{"Your order", "Hello, Ana!", "3 new messages", "4 items in your cart", "You are 2nd in line",
			"Step 3 of 4", "1,234.50", "15%", "3:30 PM", "apples, pears, and figs",
			"Order summary", "Delivery date", "Not chosen yet", "Pick a date", "We do not deliver on Sundays."}},
		{"es", []string{"Tu pedido", "¡Hola, Ana!", "3 mensajes nuevos", "4 artículos en tu carrito",
			"Paso 3 de 4", "1.234,50", "15 %", "15:30", "manzanas, peras y higos",
			"Resumen del pedido", "Fecha de entrega", "Sin elegir", "Elige una fecha", "No entregamos los domingos."}},
		{"ar", []string{"طلبك", "مرحبا يا Ana!", "٣ رسائل جديدة", "٤ عناصر في سلتك", "الخطوة ٣ من ٤",
			"ملخص الطلب", "تاريخ التوصيل", "لم يُحدد بعد", "اختر تاريخًا", "لا نوصل أيام الأحد."}},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := mountIn(t, c.tag)
			for _, s := range c.texts {
				a.ExpectText(s)
			}
		})
	}
}

func TestArabicPlurals(t *testing.T) {
	a := mountIn(t, "ar")
	remove, add := telatest.ByRole("button", "إزالة"), telatest.ByRole("button", "إضافة")
	for _, c := range []struct {
		clicks telatest.Locator
		n      int
		want   string
	}{
		{remove, 3, "لا توجد رسائل جديدة"}, // zero
		{add, 1, "رسالة جديدة واحدة"},      // one
		{add, 1, "رسالتان جديدتان"},        // two
		{add, 9, "١١ رسالة جديدة"},         // many
	} {
		for range c.n {
			a.Click(c.clicks)
		}
		a.ExpectText(c.want)
	}
}

// TestAriaSnapshots pin what `tela inspect --aria` prints in each language:
// the names of the controls, the Select and the DatePicker come from the
// catalog; with the calendar open, so do the month, the weekdays and the
// dates.
func TestAriaSnapshots(t *testing.T) {
	for _, tag := range Languages {
		mountIn(t, tag).AriaGolden("i18n_" + tag + ".aria.yml")
	}
	for _, tag := range Languages {
		a := mountIn(t, tag)
		a.Click(deliveryButton)
		a.AriaGolden("i18n_" + tag + "_picker.aria.yml")
	}
}

// TestSwitchMirrors switches the language at run time with the Select and
// checks that Arabic mirrors the layout, and that Spanish brings it back.
func TestSwitchMirrors(t *testing.T) {
	a := mountLive(t, "en")
	a.ExpectText("Your order")
	back, checkout := telatest.ByTestID("back"), telatest.ByTestID("checkout")
	progress, bar := telatest.ByTestID("progress"), telatest.ByClass("h-full", "flex-1")
	ltrBack, ltrCheckout, ltrBar := a.Bounds(back), a.Bounds(checkout), a.Bounds(bar)
	ltrSelect, track := a.Bounds(languageSelect), a.Bounds(progress)
	if ltrBack.X > ltrCheckout.X {
		t.Fatalf("en: Back at x=%v is not before Checkout at x=%v", ltrBack.X, ltrCheckout.X)
	}

	a.Click(languageSelect)
	a.Click(telatest.ByRole("option", "العربية"))
	a.ExpectText("طلبك")
	a.ExpectNoText("Your order")
	a.Expect(languageSelect, telatest.HasName("العربية اللغة"))
	rtlBack, rtlCheckout, rtlBar := a.Bounds(back), a.Bounds(checkout), a.Bounds(bar)
	if rtlBack.X < rtlCheckout.X {
		t.Errorf("ar: Back at x=%v is not after Checkout at x=%v (mirrored)", rtlBack.X, rtlCheckout.X)
	}
	// The footer spans the same content box: Back is at its right edge and
	// Checkout at its left one.
	if right := ltrCheckout.X + ltrCheckout.Width; rtlBack.X+rtlBack.Width < right-16 {
		t.Errorf("ar: Back ends at %v, want near the right edge of the content, %v", rtlBack.X+rtlBack.Width, right)
	}
	if d := rtlCheckout.X - ltrBack.X; d < -0.01 || d > 0.01 {
		t.Errorf("ar: Checkout at x=%v, want at the content's left edge %v", rtlCheckout.X, ltrBack.X)
	}
	// The progress bar fills from the right: kit.Progress slides its
	// indicator toward the start, which is the left edge in English and
	// the right one in Arabic.
	if rtlBar.Width != ltrBar.Width || ltrBar.X >= track.X || rtlBar.X <= track.X {
		t.Errorf("ar: indicator %+v, en %+v in the track %+v: want it slid right, not left", rtlBar, ltrBar, track)
	}
	// The language Select is at the start of the header: the left edge's
	// opposite in Arabic.
	if sel := a.Bounds(languageSelect); sel.X >= ltrSelect.X || sel.X > track.X+16 {
		t.Errorf("ar: the language Select at x=%v, in English at %v: want it at the left, the end of the header", sel.X, ltrSelect.X)
	}

	a.Click(languageSelect)
	a.Click(telatest.ByRole("option", "Español"))
	a.ExpectText("Tu pedido")
	if b := a.Bounds(back); b.X != ltrBack.X {
		t.Errorf("es: Back at x=%v, want %v as in English", b.X, ltrBack.X)
	}
	if b := a.Bounds(bar); b.X != ltrBar.X {
		t.Errorf("es: the indicator at x=%v, want %v as in English", b.X, ltrBar.X)
	}
	if b := a.Bounds(languageSelect); b.X != ltrSelect.X {
		t.Errorf("es: the Select at x=%v, want %v as in English", b.X, ltrSelect.X)
	}
}

// TestSelectKeyboard chooses a language with the keyboard.
func TestSelectKeyboard(t *testing.T) {
	a := mountLive(t, "en")
	a.Focus(languageSelect)
	a.Press("ArrowDown")
	a.Expect(telatest.ByRole("option", "English"), telatest.Focused(true))
	a.Press("ArrowDown")
	a.Press("Enter")
	a.ExpectText("Tu pedido")
	a.ExpectCount(telatest.ByRole("option", ""), 0)
	a.Expect(languageSelect, telatest.HasName("Español Idioma"))
	if got := i18n.Locale(); got != "es" {
		t.Errorf("the locale is %q after choosing Español, want es", got)
	}
}

// TestCalendarFollowsTheLocale opens the calendar in each language: the
// month and year, the weekday order, the digits and the direction are the
// locale's.
func TestCalendarFollowsTheLocale(t *testing.T) {
	for _, c := range []struct {
		tag, heading, first, last string
		rtl                       bool
	}{
		{"en", "March 2026", "Sunday", "Saturday", false},
		{"es", "marzo de 2026", "lunes", "domingo", false},
		{"ar", "مارس ٢٠٢٦", "السبت", "الجمعة", true},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := mountIn(t, c.tag)
			a.Click(deliveryButton)
			a.Expect(deliveryButton, telatest.Expanded(true))
			a.ExpectCount(popover, 1)
			a.ExpectRole("heading", c.heading)
			a.ExpectFocused(cell(c.tag, today))

			// The weekday columns start with the locale's first weekday,
			// from the right in Arabic.
			a.ExpectCount(telatest.ByRole("columnheader", ""), 7)
			first := a.Bounds(telatest.ByRole("columnheader", c.first))
			last := a.Bounds(telatest.ByRole("columnheader", c.last))
			if (first.X > last.X) != c.rtl {
				t.Errorf("the first weekday %q is at x=%v and the last %q at x=%v: rtl is %v", c.first, first.X, c.last, last.X, c.rtl)
			}
			// The month buttons are named from the catalog and mirrored.
			prev := a.Bounds(telatest.ByRole("button", i18n.For(c.tag).T("previousMonth")))
			next := a.Bounds(telatest.ByRole("button", i18n.For(c.tag).T("nextMonth")))
			if c.rtl == (prev.X < next.X) {
				t.Errorf("previous at x=%v and next at x=%v: rtl is %v", prev.X, next.X, c.rtl)
			}
			// Min is the order's day and Sundays are unavailable.
			a.Expect(cell(c.tag, march(13)), telatest.Disabled(true))
			a.Expect(cell(c.tag, march(15)), telatest.Disabled(true))
			a.Expect(cell(c.tag, march(16)), telatest.Disabled(false))
		})
	}
}

// TestChooseDeliveryDate picks a date in each language: the picker closes,
// and its button and the summary row show the date as the locale writes it.
func TestChooseDeliveryDate(t *testing.T) {
	for _, c := range []struct {
		tag, button, summary string
	}{
		{"en", "March 19, 2026", "Thursday, March 19, 2026"},
		{"es", "19 de marzo de 2026", "jueves, 19 de marzo de 2026"},
		{"ar", "١٩ مارس ٢٠٢٦", "الخميس، ١٩ مارس ٢٠٢٦"},
	} {
		t.Run(c.tag, func(t *testing.T) {
			a := mountIn(t, c.tag)
			a.Click(deliveryButton)
			a.Click(cell(c.tag, march(19)))
			a.ExpectCount(popover, 0)
			a.ExpectFocused(deliveryButton)
			a.ExpectText(c.button)
			a.ExpectText(c.summary)
			a.ExpectNoText(i18n.For(c.tag).T("summary.pending"))
			// Opening again focuses the chosen date.
			a.Press("Enter")
			a.ExpectFocused(cell(c.tag, march(19)))
			a.Expect(cell(c.tag, march(19)), telatest.Selected(true))
		})
	}
}

// TestLocaleChangesWithThePickerOpen changes the language while the
// calendar is open (from outside the window's controls, as a shortcut or a
// system language change would) and again while it is closed: the popover
// and the chosen date stay, and every text of the calendar becomes the new
// language's.
func TestLocaleChangesWithThePickerOpen(t *testing.T) {
	a := mountLive(t, "en")
	a.Click(deliveryButton)
	a.ExpectRole("heading", "March 2026")
	a.ExpectRole("columnheader", "Sunday")
	a.Click(cell("en", march(18)))
	a.Click(deliveryButton)
	a.ExpectCount(popover, 1)

	i18n.SetLocale("ar")
	a.WaitFor("the calendar in Arabic", func() bool { return a.Count(telatest.ByRole("heading", "مارس ٢٠٢٦")) == 1 })
	a.ExpectCount(popover, 1) // still open
	a.Expect(deliveryButton, telatest.Expanded(true))
	a.ExpectRole("columnheader", "السبت")
	a.ExpectNoText("Sunday")
	a.ExpectText("١٨ مارس ٢٠٢٦") // the chosen date, in Arabic digits
	a.Expect(cell("ar", march(18)), telatest.Selected(true))
	prev := a.Bounds(telatest.ByRole("button", "الشهر السابق"))
	next := a.Bounds(telatest.ByRole("button", "الشهر التالي"))
	if prev.X < next.X {
		t.Errorf("ar: previous at x=%v is not right of next at x=%v", prev.X, next.X)
	}

	// Closed: the same, then Spanish.
	a.Press("Escape")
	a.ExpectCount(popover, 0)
	i18n.SetLocale("es")
	a.WaitFor("the screen in Spanish", func() bool { return a.Count(telatest.ByText("Tu pedido")) == 1 })
	a.ExpectText("18 de marzo de 2026")
	a.Expect(deliveryButton, telatest.HasName("Fecha de entrega, 18 de marzo de 2026"))
	a.Click(deliveryButton)
	a.ExpectRole("heading", "marzo de 2026")
	a.ExpectRole("columnheader", "lunes")
	a.Expect(cell("es", march(18)), telatest.Selected(true))
}
