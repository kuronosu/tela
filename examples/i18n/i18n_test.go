package main

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 720, 720

// mountIn opens the screen with its locale fixed by i18n.Provider; dev mode
// is on, so any dev message fails the test.
func mountIn(t *testing.T, tag string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(windowW, windowH)}, opts...)
	return telatest.Mount(t, i18n.Provider(tag, tela.C(Store)), opts...)
}

func TestGolden(t *testing.T) {
	mountIn(t, "en").GoldenScales("i18n_en", 1)
	mountIn(t, "es").GoldenScales("i18n_es", 1)
	mountIn(t, "ar").GoldenScales("i18n_ar", 1, 2)
}

func TestTranslations(t *testing.T) {
	for _, c := range []struct {
		tag   string
		texts []string
	}{
		{"en", []string{"Your order", "Hello, Ana!", "3 new messages", "4 items in your cart", "You are 2nd in line",
			"Step 3 of 4", "1,234.50", "15%", "Tuesday, March 17, 2026", "3:30 PM", "apples, pears, and figs"}},
		{"es", []string{"Tu pedido", "¡Hola, Ana!", "3 mensajes nuevos", "4 artículos en tu carrito",
			"Paso 3 de 4", "1.234,50", "15 %", "martes, 17 de marzo de 2026", "15:30", "manzanas, peras y higos"}},
		{"ar", []string{"طلبك", "مرحبا يا Ana!", "٣ رسائل جديدة", "٤ عناصر في سلتك", "الخطوة ٣ من ٤"}},
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

// TestSwitchMirrors switches the language at run time with the buttons
// (i18n.SetLocale, process-wide: this test must not run in parallel with
// others that change the locale) and checks that Arabic mirrors the
// layout.
func TestSwitchMirrors(t *testing.T) {
	i18n.SetLocale("en")
	t.Cleanup(func() { i18n.SetLocale("") })
	a := telatest.MountFunc(t, Store, telatest.Size(windowW, windowH))
	a.ExpectText("Your order")
	back, checkout := telatest.ByTestID("back"), telatest.ByTestID("checkout")
	progress := telatest.ByTestID("progress")
	ltrBack, ltrCheckout, ltrBar := a.Bounds(back), a.Bounds(checkout), a.Bounds(progress)
	if ltrBack.X > ltrCheckout.X {
		t.Fatalf("en: Back at x=%v is not before Checkout at x=%v", ltrBack.X, ltrCheckout.X)
	}

	a.Click(telatest.ByRole("button", "العربية"))
	a.ExpectText("طلبك")
	a.Expect(telatest.ByRole("button", "العربية"), telatest.Pressed(true))
	rtlBack, rtlCheckout, rtlBar := a.Bounds(back), a.Bounds(checkout), a.Bounds(progress)
	if rtlBack.X < rtlCheckout.X {
		t.Errorf("ar: Back at x=%v is not after Checkout at x=%v (mirrored)", rtlBack.X, rtlCheckout.X)
	}
	// The footer spans the same content box: Back moves to its right
	// edge and Checkout to its left one.
	if right := ltrBack.X + ltrBack.Width; rtlBack.X+rtlBack.Width <= right {
		t.Errorf("ar: Back ends at %v, want at the right edge of the content", rtlBack.X+rtlBack.Width)
	}
	if d := rtlCheckout.X - ltrBack.X; d < -0.01 || d > 0.01 {
		t.Errorf("ar: Checkout at x=%v, want at the content's left edge %v", rtlCheckout.X, ltrBack.X)
	}
	// The progress bar fills from the right: same width, its right edge
	// where the left-to-right bar's track ends.
	if rtlBar.Width != ltrBar.Width || rtlBar.X <= ltrBar.X {
		t.Errorf("ar: progress %+v, en %+v: want the same width from the right", rtlBar, ltrBar)
	}

	a.Click(telatest.ByRole("button", "Español"))
	a.ExpectText("Tu pedido")
	if b := a.Bounds(back); b.X != ltrBack.X {
		t.Errorf("es: Back at x=%v, want %v as in English", b.X, ltrBack.X)
	}
}
