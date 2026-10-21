package main

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1100, 860

// The fields by their accessible names: the text of their Label.
var (
	nameBox     = telatest.ByRole("textbox", "Display name (any language)")
	emailBox    = telatest.ByRole("textbox", "E-mail")
	passwordBox = telatest.ByRole("textbox", "Password")
	bioBox      = telatest.ByRole("textbox", "Short bio (optional)")
	termsBox    = telatest.ByRole("checkbox", termsLabel)
	submit      = telatest.ByRole("button", "Create account")
	showButton  = telatest.ByRole("button", "Show password")
)

// mount opens the screen in dev mode, where any dev message fails the test.
func mount(t *testing.T, p Props, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(w, h), telatest.Dev()}, opts...)
	return telatest.Mount(t, tela.CP(Page, p), opts...)
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {700, 600}, {420, 900}, {1600, 700}} {
		mount(t, Props{}, s[0], s[1])
		mount(t, Props{Initial: filled, Touched: true, Created: true}, s[0], s[1])
	}
}

// TestGolden renders each preview at scale 1 and 2.
func TestGolden(t *testing.T) {
	for _, name := range Previews.Names() {
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("%s/x%g", name, scale), func(t *testing.T) {
				a := telatest.Mount(t, Previews[name], telatest.Size(windowW, windowH), telatest.Scale(scale), telatest.Dev())
				a.Golden(fmt.Sprintf("signup_%s_x%g.png", strings.ReplaceAll(strings.ToLower(name[2:]), " ", "_"), scale))
			})
		}
	}
}

func TestGoldenDark(t *testing.T) {
	a := telatest.Mount(t, Previews["3 Invalid"], telatest.Size(windowW, windowH), telatest.Dark(), telatest.Dev())
	a.Golden("signup_invalid_dark_x1.png")
}

// TestAriaSnapshot pins what `tela inspect --aria` prints for the empty form
// and for the confirmation: every field is a textbox named by its Label.
func TestAriaSnapshot(t *testing.T) {
	mount(t, Props{}, windowW, windowH).AriaGolden("signup.aria.yml")
	mount(t, Props{Initial: filled, Created: true}, windowW, windowH).AriaGolden("signup_created.aria.yml")
}

// TestSubmitFlow fills the form and submits it with the button. The button
// is disabled until the form is valid.
func TestSubmitFlow(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.ExpectText("Fill in your name, e-mail and password, and accept the terms.")

	a.Fill(nameBox, "Ana García")
	a.Fill(emailBox, "ana@example.com")
	a.Fill(passwordBox, "correct-horse-battery")
	a.ExpectValue(nameBox, "Ana García")
	a.ExpectValue(passwordBox, "correct-horse-battery")

	// Without the terms the button is still disabled: a click is refused by
	// the actionability checks, and a press does nothing.
	a.Hover(submit)
	if got := a.Cursor(); got != "not-allowed" {
		t.Errorf("cursor over the disabled button is %q, want not-allowed", got)
	}
	a.MouseDown()
	a.MouseUp()
	a.ExpectNoText("Welcome")

	a.Click(termsBox)
	a.ExpectNoText("Fill in your name")
	a.Click(submit)
	a.ExpectText("Welcome, Ana García!")
	a.ExpectText("We sent a confirmation link to ana@example.com.")
	a.ExpectCount(telatest.ByRole("status", ""), 1)

	// A second submit is refused, and editing a field clears the banner.
	a.Hover(submit)
	a.MouseDown()
	a.MouseUp()
	a.ExpectCount(telatest.ByRole("status", ""), 1)
	a.Click(nameBox)
	a.Type("!")
	a.ExpectNoText("Welcome")
	a.ExpectValue(nameBox, "Ana García!")
}

// TestImplicitSubmission types the form key by key, then presses Enter in the
// e-mail field: the form clicks its first submit button.
func TestImplicitSubmission(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.Press("Tab") // the name field has the focus already; Tab goes to the e-mail
	a.ExpectFocused(emailBox)
	a.Type("ana@example.com")
	a.Press("Tab")
	a.ExpectFocused(passwordBox)
	// ... nothing to submit yet: the button is disabled, and Enter does
	// nothing, as in a browser.
	a.Press("Enter")
	a.ExpectNoText("Welcome")

	a.Click(nameBox)
	a.Type("Ana")
	a.Fill(passwordBox, "hunter22")
	a.Click(termsBox)
	a.Click(emailBox)
	a.Press("Enter")
	a.ExpectText("Welcome, Ana!")
}

// TestEmailInvalid checks the two things Type("email") gives: the invalid:
// border and the peer-invalid: message, which wait for the first blur.
func TestEmailInvalid(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.Click(emailBox)
	a.Type("ana@")
	a.ExpectNoText("Enter a valid e-mail address")
	if c := borderPixel(a, emailBox); c.R > 100 && c.G < 100 {
		t.Errorf("the border is red (%v) before the field was left", c)
	}

	a.Press("Tab") // leave the field: touched
	a.ExpectText("Enter a valid e-mail address, like you@example.com.")
	a.ExpectNoText("We only use it to confirm")
	if c := borderPixel(a, emailBox); c.R < 200 || c.G > 100 || c.B > 100 {
		t.Errorf("the border of an invalid address is %v, want red", c)
	}

	a.Click(emailBox)
	a.Press("End")
	a.Type("example.com")
	a.ExpectNoText("Enter a valid e-mail address")
	a.ExpectText("We only use it to confirm")
	if c := borderPixel(a, emailBox); c.R > 100 && c.G < 100 {
		t.Errorf("the border of a valid address is %v, want not red", c)
	}
}

// borderPixel samples the middle of the top border of a field, in the
// middle of its width.
func borderPixel(a *telatest.App, l telatest.Locator) color.RGBA {
	xBox := a.Bounds(l)
	x, y, w := xBox.X, xBox.Y, xBox.Width
	return a.Screenshot().RGBAAt(int(x+w/2), int(y))
}

// TestPasswordToggle checks the show/hide button: AriaPressed, and the
// value swapping dots for text in the aria snapshot.
func TestPasswordToggle(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.Fill(passwordBox, "s3cret!!")
	if snap := a.AriaSnapshot(); !strings.Contains(snap, `textbox "Password": ••••••••`) {
		t.Errorf("the password is not masked:\n%s", snap)
	}
	a.ExpectRole("button", "Show password")
	a.Click(showButton)
	a.ExpectText("Hide")
	if snap := a.AriaSnapshot(); !strings.Contains(snap, `textbox "Password": s3cret!!`) || !strings.Contains(snap, "[pressed]") {
		t.Errorf("the password is not shown, or the toggle not pressed:\n%s", snap)
	}
	a.ExpectValue(passwordBox, "s3cret!!") // the edit state survived the type change
	a.Click(showButton)
	a.ExpectText("Show")
}

func TestPasswordStrength(t *testing.T) {
	for p, want := range map[string]int{"": 0, "abc": 1, "abcdefgh": 1, "abcdefg1": 2, "Abcdefg1": 3, "Abcdefg1!": 4, "abcdefghijkl": 2} {
		if got := Strength(p); got != want {
			t.Errorf("Strength(%q) = %d, want %d", p, got, want)
		}
	}
	a := mount(t, Props{}, windowW, windowH)
	for _, w := range []struct{ pass, word string }{{"abc", "Weak"}, {"abcdefg1", "Fair"}, {"Abcdefg1", "Good"}, {"Abcdefg1!", "Strong"}} {
		a.Fill(passwordBox, w.pass)
		a.ExpectText(w.word)
	}
}

// TestBio checks the counter, MaxLength and field-sizing-content with
// min-h and max-h; and that typing in a BindValue field rebuilds nothing.
func TestBio(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.ExpectText("0/160")
	h0 := a.Bounds(bioBox).Height
	if h0 != 72 {
		t.Errorf("an empty bio is %v px tall, want min-h 72", h0)
	}

	a.Click(bioBox)
	before := a.Stats()
	a.Type("Go ⚡")
	a.ExpectText("4/160")
	if builds := a.Stats().Builds - before.Builds; builds != 0 {
		t.Errorf("typing in the bio rebuilt %d components, want 0 (BindValue + TextFn)", builds)
	}

	// Growing: more lines make the field taller, up to max-h-40 (160 px).
	a.Fill(bioBox, strings.Repeat("line\n", 4)+"last")
	h1 := a.Bounds(bioBox).Height
	if h1 <= h0 || h1 >= 160 {
		t.Errorf("a 5-line bio is %v px tall, want between 72 and 160 exclusive", h1)
	}
	a.Fill(bioBox, strings.Repeat("line\n", 12)+"last")
	if h2 := a.Bounds(bioBox).Height; h2 != 160 {
		t.Errorf("a long bio is %v px tall, want max-h 160", h2)
	}
	a.Fill(bioBox, "")
	if h3 := a.Bounds(bioBox).Height; h3 != 72 {
		t.Errorf("an emptied bio is %v px tall, want 72", h3)
	}

	// MaxLength counts UTF-16 code units, like HTML: an emoji is 2.
	a.Fill(bioBox, strings.Repeat("😀", 100))
	a.ExpectText("160/160")
}

// TestLabels checks that a Label focuses its field, and the one of the terms
// toggles its checkbox.
func TestLabels(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.Click(telatest.ByText("E-mail", telatest.Exact()))
	a.ExpectFocused(emailBox)
	a.Click(telatest.ByText("Short bio"))
	a.ExpectFocused(bioBox)

	a.Click(telatest.ByText("I agree to the"))
	a.WaitFor("the terms to be checked", func() bool { return termsChecked(a) })
}

// termsChecked reports whether the aria snapshot shows the terms checked.
func termsChecked(a *telatest.App) bool {
	return strings.Contains(a.AriaSnapshot(), `checkbox "`+termsLabel+`" [checked]`)
}

// TestTermsKeyboard checks the terms checkbox with the keyboard: Tab to
// it, Space toggles it.
func TestTermsKeyboard(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.Focus(termsBox)
	a.Press("Space")
	a.WaitFor("the terms to be checked", func() bool { return termsChecked(a) })
	a.Press("Space")
	a.WaitFor("the terms to be unchecked", func() bool { return !termsChecked(a) })
}

// TestFocusRing checks that focusing a field paints the ring around it.
func TestFocusRing(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	xBox := a.Bounds(emailBox)
	x, y, w, h := xBox.X, xBox.Y, xBox.Width, xBox.Height
	ring := func() color.RGBA { return a.Screenshot().RGBAAt(int(x+w/2), int(y+h)+1) }
	rest := ring()
	a.Click(emailBox)
	if focus := ring(); focus == rest {
		t.Errorf("no focus ring under the focused field: %v both", rest)
	}
}

// TestAnyLanguage checks that any language reaches the field and the
// counters: the screen has no filter on what a name is.
func TestAnyLanguage(t *testing.T) {
	a := mount(t, Props{}, windowW, windowH)
	a.Fill(nameBox, "田中 太郎")
	a.ExpectValue(nameBox, "田中 太郎")
	a.Fill(bioBox, "日本語でも書けます。")
	a.ExpectText("10/160")
}
