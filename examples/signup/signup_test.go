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
	nameBox     = telatest.ByRole("textbox", "Display name")
	emailBox    = telatest.ByRole("textbox", "E-mail")
	passwordBox = telatest.ByRole("textbox", "Password")
	bioBox      = telatest.ByRole("textbox", "Short bio (optional)")
	termsBox    = telatest.ByRole("checkbox", termsLabel)
	submit      = telatest.ByRole("button", "Create account")
	showButton  = telatest.ByRole("button", "Show password")
)

// mount opens the page in the window of main (its theme and root class);
// dev mode is on, so any dev message fails the test.
func mount(t *testing.T, p Page, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mountRoot(t, p, w, h, opts...)
}

// mountRoot is mount for any root, such as a preview.
func mountRoot(t *testing.T, root tela.Component, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = root
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.MountApp(t, app, opts...)
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {700, 600}, {420, 900}, {1600, 700}} {
		mount(t, Page{}, s[0], s[1])
		mount(t, Page{Initial: filled, Touched: true, Created: true}, s[0], s[1])
	}
}

// TestGolden renders each preview at scale 1 and 2.
func TestGolden(t *testing.T) {
	for _, name := range Previews.Names() {
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("%s/x%g", name, scale), func(t *testing.T) {
				a := mountRoot(t, Previews[name], windowW, windowH, telatest.Scale(scale))
				a.Golden(fmt.Sprintf("signup_%s_x%g.png", strings.ReplaceAll(strings.ToLower(name[2:]), " ", "_"), scale))
			})
		}
	}
}

func TestGoldenDark(t *testing.T) {
	a := mountRoot(t, Previews["3 Invalid"], windowW, windowH, telatest.Dark())
	a.Golden("signup_invalid_dark_x1.png")
}

// TestAriaSnapshot pins what `tela inspect --aria` prints for the empty form
// and for the confirmation: every field is a textbox named by its Label.
func TestAriaSnapshot(t *testing.T) {
	mount(t, Page{}, windowW, windowH).AriaGolden("signup.aria.yml")
	mount(t, Page{Initial: filled, Created: true}, windowW, windowH).AriaGolden("signup_created.aria.yml")
}

// TestSubmitFlow fills the form and submits it with the button. The button
// is disabled until the form is valid.
func TestSubmitFlow(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.ExpectText("Fill in your name, e-mail and password, and accept the terms.")

	a.Fill(nameBox, "Ana García")
	a.Fill(emailBox, "ana@example.com")
	a.Fill(passwordBox, "correct-horse-battery")
	a.ExpectValue(nameBox, "Ana García")
	a.ExpectValue(passwordBox, "correct-horse-battery")

	// Without the terms the button is still disabled (the recipe's
	// pointer-events-none lets the pointer through to the column below it),
	// so a press on it does nothing.
	a.Expect(submit, telatest.Disabled(true))
	press(a, submit)
	a.ExpectNoText("Welcome")

	a.Click(termsBox)
	a.ExpectNoText("Fill in your name")
	a.Click(submit)
	a.ExpectText("Welcome, Ana García!")
	a.ExpectText("We sent a confirmation link to ana@example.com.")
	a.ExpectCount(telatest.ByRole("status", ""), 1)

	// A second submit is refused, and editing a field clears the banner.
	a.Expect(submit, telatest.Disabled(true))
	press(a, submit)
	a.ExpectCount(telatest.ByRole("status", ""), 1)
	a.Click(nameBox)
	a.Type("!")
	a.ExpectNoText("Welcome")
	a.ExpectValue(nameBox, "Ana García!")
}

// press presses the pointer over the center of l, like a user, whether or not
// the element takes it.
func press(a *telatest.App, l telatest.Locator) {
	r := a.Bounds(l)
	a.MouseMove(r.X+r.Width/2, r.Y+r.Height/2)
	a.MouseDown()
	a.MouseUp()
}

// TestImplicitSubmission types the form key by key, then presses Enter in the
// e-mail field: the form clicks its first submit button.
func TestImplicitSubmission(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
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

// TestEmailInvalid checks what the Field's Error gives: the red border (the
// recipe's aria-invalid: classes) and the message in place of the help
// text, which wait for the first blur.
func TestEmailInvalid(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.Click(emailBox)
	a.Type("ana@")
	a.ExpectNoText("Enter a valid e-mail address")
	if borderIsRed(a, emailBox) {
		t.Error("the border is red before the field was left")
	}

	a.Press("Tab") // leave the field: touched
	a.ExpectText("Enter a valid e-mail address, like you@example.com.")
	a.ExpectNoText("We only use it to confirm")
	a.Expect(emailBox, telatest.Invalid(true))
	if !borderIsRed(a, emailBox) {
		t.Error("the border of an invalid address is not red")
	}

	a.Click(emailBox)
	a.Press("End")
	a.Type("example.com")
	a.ExpectNoText("Enter a valid e-mail address")
	a.ExpectText("We only use it to confirm")
	a.Expect(emailBox, telatest.Invalid(false))
	if borderIsRed(a, emailBox) {
		t.Error("the border of a valid address is red")
	}
}

// borderIsRed looks for the destructive color on the top border of a field,
// in the middle of its width (the box edge may fall between two pixel rows).
func borderIsRed(a *telatest.App, l telatest.Locator) bool {
	r := a.Bounds(l)
	img := a.Screenshot()
	for y := int(r.Y) - 1; y <= int(r.Y)+1; y++ {
		if c := img.RGBAAt(int(r.X+r.Width/2), y); c.R > 200 && c.G < 100 && c.B < 100 {
			return true
		}
	}
	return false
}

// TestPasswordToggle checks the show/hide button: AriaPressed, and the
// value swapping dots for text in the aria snapshot.
func TestPasswordToggle(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.Fill(passwordBox, "s3cret!!")
	if snap := a.AriaSnapshot(); !strings.Contains(snap, `textbox "Password": ••••••••`) {
		t.Errorf("the password is not masked:\n%s", snap)
	}
	a.ExpectRole("button", "Show password")
	a.Expect(showButton, telatest.Pressed(false))
	a.Click(showButton)
	if snap := a.AriaSnapshot(); !strings.Contains(snap, `textbox "Password": s3cret!!`) {
		t.Errorf("the password is not shown:\n%s", snap)
	}
	a.Expect(showButton, telatest.Pressed(true))
	a.ExpectValue(passwordBox, "s3cret!!") // the edit state survived the type change
	a.Click(showButton)
	a.Expect(showButton, telatest.Pressed(false))
	if snap := a.AriaSnapshot(); !strings.Contains(snap, `textbox "Password": ••••••••`) {
		t.Errorf("the password is not masked again:\n%s", snap)
	}
}

// TestPasswordTooltip checks that the show/hide button says what it does
// when the pointer rests on it, and the wording follows its state.
func TestPasswordTooltip(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.Hover(showButton)
	a.ExpectText("Show password")
	a.Click(showButton) // a press closes the tooltip
	a.MouseMove(5, 5)
	a.ExpectNoText("Show password")
	a.Hover(showButton)
	a.ExpectText("Hide password")
}

func TestPasswordStrength(t *testing.T) {
	for p, want := range map[string]int{"": 0, "abc": 1, "abcdefgh": 1, "abcdefg1": 2, "Abcdefg1": 3, "Abcdefg1!": 4, "abcdefghijkl": 2} {
		if got := Strength(p); got != want {
			t.Errorf("Strength(%q) = %d, want %d", p, got, want)
		}
	}
	a := mount(t, Page{}, windowW, windowH)
	for _, w := range []struct{ pass, word string }{{"abc", "Weak"}, {"abcdefg1", "Fair"}, {"Abcdefg1", "Good"}, {"Abcdefg1!", "Strong"}} {
		a.Fill(passwordBox, w.pass)
		a.ExpectText(w.word)
	}
}

// TestBio checks the counter, MaxLength and field-sizing-content with
// min-h and max-h; and that typing in a BindValue field rebuilds nothing.
func TestBio(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
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
	a := mount(t, Page{}, windowW, windowH)
	a.Click(telatest.ByText("E-mail", telatest.Exact()))
	a.ExpectFocused(emailBox)
	a.Click(telatest.ByText("Short bio"))
	a.ExpectFocused(bioBox)

	a.Click(telatest.ByText(termsLabel, telatest.Exact()))
	a.Expect(termsBox, telatest.Checked(true))
}

// TestTermsKeyboard checks the terms checkbox with the keyboard: Tab to
// it, Space toggles it.
func TestTermsKeyboard(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.Focus(termsBox)
	a.Press("Space")
	a.Expect(termsBox, telatest.Checked(true))
	a.Press("Space")
	a.Expect(termsBox, telatest.Checked(false))
}

// TestFocusRing checks that focusing a field paints the ring around it.
func TestFocusRing(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	r := a.Bounds(emailBox)
	ring := func() color.RGBA { return a.Screenshot().RGBAAt(int(r.X+r.Width/2), int(r.Y+r.Height)+1) }
	rest := ring()
	a.Click(emailBox)
	if focus := ring(); focus == rest {
		t.Errorf("no focus ring under the focused field: %v both", rest)
	}
}

// TestAnyLanguage checks that any language reaches the field and the
// counters: the screen has no filter on what a name is.
func TestAnyLanguage(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.Fill(nameBox, "田中 太郎")
	a.ExpectValue(nameBox, "田中 太郎")
	a.Fill(bioBox, "日本語でも書けます。")
	a.ExpectText("10/160")
}
