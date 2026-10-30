package main

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1000, 760

// The controls by their accessible names.
var (
	emailBox    = telatest.ByRole("textbox", "E-mail")
	passwordBox = telatest.ByRole("textbox", "Password")
	showButton  = telatest.ByRole("button", "Show password")
	remember    = telatest.ByRole("checkbox", "Remember me")
	signIn      = telatest.ByRole("button", "Sign in")
	signingIn   = telatest.ByRole("button", "Signing in…")
	signOut     = telatest.ByRole("button", "Sign out")
	// The mode switches: their accessible name says where they go and
	// contains their visible text.
	signUpLink   = telatest.ByRole("button", "Sign up for an account")
	signInLink   = telatest.ByRole("button", "Sign in to your account")
	toSignIn     = telatest.ByRole("button", "Continue to sign in")
	nameBox      = telatest.ByRole("textbox", "Display name")
	bioBox       = telatest.ByRole("textbox", "Short bio (optional)")
	termsBox     = telatest.ByRole("checkbox", termsLabel)
	createButton = telatest.ByRole("button", "Create account")
	creating     = telatest.ByRole("button", "Creating account…")
	// spinner is the loading button: the spinner icon is part of the
	// button's content, which has no nodes of its own in the semantics
	// tree, so ByClass("animate-spin") would match nothing.
	spinner = signingIn
)

// The messages the tests wait for.
const (
	needEmail    = "Enter your e-mail address."
	badEmail     = "Enter a valid e-mail address, like you@example.com."
	needPassword = "Enter your password."
	rejected     = "Incorrect e-mail or password"
	welcome      = "You're signed in"
	created      = "Account created"
	needFields   = "Fill in every field and accept the terms."
	signInPanel  = "Desktop apps, woven from one codebase."
	createPanel  = "Start weaving in minutes, free."
)

// createPage is the Create account form, empty.
var createPage = Page{Mode: CreateAccount}

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

// flow mounts the page for a test that sends the form. The spinner runs
// until the answer comes, and a settled app would run the clock to the end
// of it, so the clock moves only with Advance (NoSettle): do the typing
// first, and after the submit use Advance and the expectations.
func flow(t *testing.T) *telatest.App {
	t.Helper()
	return mount(t, Page{}, windowW, windowH, telatest.NoSettle())
}

// submit fills the form and presses Enter in the password field.
func submit(a *telatest.App, email, password string) {
	a.Fill(emailBox, email)
	a.Fill(passwordBox, password)
	a.Press("Enter")
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {700, 600}, {420, 800}, {1400, 700}} {
		for _, p := range []Page{
			{},
			{Submitted: true, ShowPassword: true},
			{Status: Failed, Initial: Values{Email: "mei@example.com", Password: "x"}},
			{Status: SignedIn, Initial: valid},
			{Mode: CreateAccount},
			{Mode: CreateAccount, Initial: filled, Touched: true, ShowPassword: true},
			{Mode: CreateAccount, Initial: filled, Status: Created},
		} {
			mount(t, p, s[0], s[1])
		}
		mount(t, Page{Status: Loading, Initial: valid}, s[0], s[1], telatest.NoSettle())
		mount(t, Page{Mode: CreateAccount, Status: Loading, Initial: filled}, s[0], s[1], telatest.NoSettle())
	}
}

// slug names the golden files of a preview: "3 Wrong password" is
// "wrong_password".
func slug(preview string) string {
	return strings.ReplaceAll(strings.ToLower(preview[2:]), " ", "_")
}

// mountPreview opens a preview as its state needs: the signing-in one has
// a spinner that never ends, so it keeps the clock still and moves it by a
// fixed 300 ms (the focus ring's transition ends, the spinner is a quarter
// turn on), which is what the golden shows.
func mountPreview(t *testing.T, name string, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	if strings.Contains(name, "Signing in") {
		a := mountRoot(t, Previews[name], w, h, append(opts, telatest.NoSettle())...)
		a.Advance(300 * time.Millisecond)
		return a
	}
	return mountRoot(t, Previews[name], w, h, opts...)
}

// TestGolden renders each preview at scale 1 and 2.
func TestGolden(t *testing.T) {
	for _, name := range Previews.Names() {
		for _, scale := range []float64{1, 2} {
			t.Run(fmt.Sprintf("%s/x%g", name, scale), func(t *testing.T) {
				mountPreview(t, name, windowW, windowH, telatest.Scale(scale)).
					Golden(fmt.Sprintf("login_%s_x%g.png", slug(name), scale))
			})
		}
	}
}

// TestGoldenDark renders the form, the alert, the welcome, and the Create
// account form with an error and its confirmation in the dark scheme, and the
// form at scale 2.
func TestGoldenDark(t *testing.T) {
	for _, name := range []string{"1 Sign in", "3 Wrong password", "5 Signed in", "8 Create account invalid", "9 Account created"} {
		t.Run(name, func(t *testing.T) {
			mountPreview(t, name, windowW, windowH, telatest.Dark()).Golden(fmt.Sprintf("login_%s_dark_x1.png", slug(name)))
		})
	}
	t.Run("1 Sign in/x2", func(t *testing.T) {
		mountPreview(t, "1 Sign in", windowW, windowH, telatest.Dark(), telatest.Scale(2)).Golden("login_sign_in_dark_x2.png")
	})
}

// TestGoldenNarrow is the one-column layout, below md: (768 px).
func TestGoldenNarrow(t *testing.T) {
	mountPreview(t, "1 Sign in", 600, 800).Golden("login_narrow_x1.png")
	mountPreview(t, "3 Wrong password", 600, 800, telatest.Dark()).Golden("login_narrow_wrong_password_dark_x1.png")
	mountPreview(t, "7 Create account filled", 600, 900).Golden("login_narrow_create_account_filled_x1.png")
}

// TestAriaSnapshot pins what `tela inspect --aria` prints for the form and
// for the welcome, and for the Create account form and its confirmation:
// every field is a textbox named by its Label, the buttons, and no panel
// text hidden.
func TestAriaSnapshot(t *testing.T) {
	mount(t, Page{}, windowW, windowH).AriaGolden("login.aria.yml")
	mount(t, Page{Status: SignedIn, Initial: valid}, windowW, windowH).AriaGolden("login_signed_in.aria.yml")
	mount(t, createPage, windowW, windowH).AriaGolden("login_create_account.aria.yml")
	mount(t, Page{Mode: CreateAccount, Status: Created, Initial: filled}, windowW, windowH).AriaGolden("login_account_created.aria.yml")
}

// TestValidation sends an empty form and then fixes it: the errors wait for
// the first try, mark the fields invalid (the Field's Error) and go away as
// the values become valid.
func TestValidation(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.ExpectFocused(emailBox)
	a.Fill(emailBox, "mei@")
	a.ExpectNoText(badEmail) // nobody is scolded for a form they have not sent

	a.Click(signIn)
	a.ExpectText(badEmail)
	a.ExpectText(needPassword)
	a.Expect(emailBox, telatest.Invalid(true))
	a.Expect(passwordBox, telatest.Invalid(true))
	a.ExpectNoText(welcome)
	a.ExpectCount(spinner, 0) // an invalid form sends nothing

	a.Fill(emailBox, "")
	a.ExpectText(needEmail)
	a.Fill(emailBox, "mei@example.com")
	a.ExpectNoText(needEmail)
	a.ExpectNoText(badEmail)
	a.Expect(emailBox, telatest.Invalid(false))
	a.Fill(passwordBox, "x")
	a.ExpectNoText(needPassword)
	a.Expect(passwordBox, telatest.Invalid(false))
}

// TestPasswordToggle checks the show/hide button: AriaPressed, and the
// value swapping dots for text in the aria snapshot. The button is the same
// in both modes.
func TestPasswordToggle(t *testing.T) {
	for _, p := range []Page{{}, createPage} {
		testPasswordToggle(t, p)
	}
}

func testPasswordToggle(t *testing.T, p Page) {
	a := mount(t, p, windowW, windowH)
	a.Fill(passwordBox, "s3cret!!")
	if snap := a.AriaSnapshot(); !strings.Contains(snap, `textbox "Password": ••••••••`) {
		t.Errorf("the password is not masked:\n%s", snap)
	}
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

// TestSignInFlow sends a good form with Enter: the button turns into a
// disabled "Signing in…" with a spinner, the other ways in are disabled,
// the answer comes signInDelay later on the app's clock, and Sign out
// brings the form back.
func TestSignInFlow(t *testing.T) {
	a := flow(t)
	a.ExpectRole("button", "Sign in")
	a.ExpectCount(spinner, 0)

	submit(a, "mei@example.com", demoPassword)
	a.Expect(signingIn, telatest.Disabled(true))
	a.ExpectCount(spinner, 1)
	for _, other := range []string{"Passkey", "Magic link", "SSO"} {
		a.Expect(telatest.ByRole("button", other), telatest.Disabled(true))
	}

	a.Advance(signInDelay - 100*time.Millisecond)
	a.Expect(signingIn, telatest.Disabled(true))
	a.ExpectNoText(welcome)
	a.Advance(200 * time.Millisecond)
	a.ExpectText(welcome)
	a.ExpectText("Welcome back, mei@example.com.")
	a.ExpectCount(telatest.ByRole("status", ""), 1)
	a.ExpectCount(spinner, 0)
	a.ExpectCount(emailBox, 0)

	a.Click(signOut)
	a.ExpectRole("button", "Sign in")
	a.ExpectValue(emailBox, "mei@example.com")
	a.ExpectValue(passwordBox, "")
	a.ExpectNoText(welcome)
}

// TestWrongPassword is a rejected sign-in: an alert above the form, which
// the next edit takes away.
func TestWrongPassword(t *testing.T) {
	a := flow(t)
	a.ExpectNoText(rejected)
	submit(a, "mei@example.com", "hunter2")
	a.Expect(signingIn, telatest.Disabled(true))
	a.ExpectNoText(rejected) // the answer is not instant
	a.Advance(signInDelay + 100*time.Millisecond)

	a.ExpectText(rejected)
	a.ExpectText("The demo password is " + demoPassword + ".")
	a.Expect(signIn, telatest.Disabled(false))
	a.ExpectCount(spinner, 0)
	a.ExpectValue(passwordBox, "hunter2") // what was typed stays

	a.Click(passwordBox)
	a.Type("!")
	a.ExpectNoText(rejected)
	a.ExpectValue(passwordBox, "hunter2!")
}

// TestEnterSubmitsOnlyAValidForm checks Enter in a field is the Sign in
// button: an invalid form shows its errors and starts no request.
func TestEnterSubmitsOnlyAValidForm(t *testing.T) {
	a := flow(t)
	a.Fill(emailBox, "mei@example.com")
	a.Press("Enter")
	a.ExpectText(needPassword)
	a.ExpectCount(spinner, 0)
	a.Expect(signIn, telatest.Disabled(false))

	a.Fill(passwordBox, demoPassword)
	a.Press("Enter")
	a.Expect(signingIn, telatest.Disabled(true))
	a.Advance(signInDelay + 100*time.Millisecond)
	a.ExpectText(welcome)
}

// TestEachAttemptHasItsOwnTimeout sends the form twice: the second request
// takes as long as the first (UseTimeout restarts when the delay changes).
func TestEachAttemptHasItsOwnTimeout(t *testing.T) {
	a := flow(t)
	submit(a, "mei@example.com", "nope")
	a.Advance(signInDelay + 100*time.Millisecond)
	a.ExpectText(rejected)

	a.Fill(passwordBox, demoPassword)
	a.Press("Enter")
	a.Expect(signingIn, telatest.Disabled(true))
	a.Advance(signInDelay - 100*time.Millisecond)
	a.Expect(signingIn, telatest.Disabled(true))
	a.Advance(200 * time.Millisecond)
	a.ExpectText(welcome)
}

// TestRemember checks the checkbox with its label and with the keyboard.
func TestRemember(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.Expect(remember, telatest.Checked(false))
	a.Click(telatest.ByText("Remember me", telatest.Exact()))
	a.Expect(remember, telatest.Checked(true))
	a.Focus(remember)
	a.Press("Space")
	a.Expect(remember, telatest.Checked(false))
	a.Press("Space")
	a.Expect(remember, telatest.Checked(true))
}

// TestLabels checks that the Label of a Field focuses its control.
func TestLabels(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.Click(telatest.ByText("Password", telatest.Exact()))
	a.ExpectFocused(passwordBox)
	a.Click(telatest.ByText("E-mail", telatest.Exact()))
	a.ExpectFocused(emailBox)
}

// TestResponsive checks the brand panel hides below md: (768 px) and the
// form still fits, in both modes.
func TestResponsive(t *testing.T) {
	const panel = "Pure Go. One binary."
	a := mount(t, Page{}, windowW, windowH)
	a.ExpectText(panel)
	a.Resize(700, 800)
	a.ExpectNoText(panel)
	a.ExpectRole("button", "Sign in")
	a.Resize(windowW, windowH)
	a.ExpectText(panel)

	a = mount(t, createPage, windowW, windowH)
	a.ExpectText(panel)
	a.Resize(700, 900)
	a.ExpectNoText(panel)
	a.ExpectRole("button", "Create account")
	a.Resize(windowW, windowH)
	a.ExpectText(panel)
}

// TestSeededStates checks the previews start in the state they name.
func TestSeededStates(t *testing.T) {
	a := mountPreview(t, "2 Invalid", windowW, windowH)
	a.ExpectText(badEmail)
	a.ExpectText(needPassword)

	a = mountPreview(t, "3 Wrong password", windowW, windowH)
	a.ExpectText(rejected)

	a = mountPreview(t, "4 Signing in", windowW, windowH)
	a.Expect(signingIn, telatest.Disabled(true))
	a.ExpectCount(spinner, 1)

	a = mountPreview(t, "5 Signed in", windowW, windowH)
	a.ExpectText(welcome)

	a = mountPreview(t, "6 Create account", windowW, windowH)
	a.ExpectText(needFields)
	a.Expect(createButton, telatest.Disabled(true))

	a = mountPreview(t, "7 Create account filled", windowW, windowH)
	a.ExpectValue(nameBox, filled.Name)
	a.ExpectText("67/160")
	a.Expect(termsBox, telatest.Checked(true))
	a.Expect(createButton, telatest.Disabled(false))

	a = mountPreview(t, "8 Create account invalid", windowW, windowH)
	a.ExpectText(badEmail)
	a.Expect(emailBox, telatest.Invalid(true))
	a.Expect(createButton, telatest.Disabled(true))

	a = mountPreview(t, "9 Account created", windowW, windowH)
	a.ExpectText(created)
}

// TestModeSwitch moves the card between the two forms with the link under
// each: the e-mail stays, the password and the errors do not, and the brand
// panel changes its headline.
func TestModeSwitch(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
	a.ExpectText(signInPanel)
	a.ExpectCount(nameBox, 0)
	a.Fill(emailBox, "mei@")
	a.Fill(passwordBox, "secret")
	a.Click(signIn) // an error shows: "mei@" is not an address
	a.ExpectText(badEmail)

	a.Click(signUpLink)
	a.ExpectRole("heading", "Create your account")
	a.ExpectText(createPanel)
	a.ExpectNoText(signInPanel)
	a.ExpectCount(signIn, 0)
	a.ExpectCount(remember, 0)
	a.ExpectFocused(nameBox)
	a.ExpectValue(emailBox, "mei@") // typed once, kept
	a.ExpectValue(passwordBox, "")
	a.ExpectNoText(badEmail) // a new form, a new first try

	a.Click(signInLink)
	a.ExpectRole("heading", "Welcome back")
	a.ExpectText(signInPanel)
	a.ExpectCount(nameBox, 0)
	a.ExpectValue(emailBox, "mei@")
	a.ExpectNoText(badEmail)
}

// TestCreateFlow fills the Create account form and sends it with the button.
// The button is disabled until the form is valid; then it turns into a
// disabled "Creating account…" with a spinner, the confirmation comes
// signInDelay later on the app's clock, and "Continue to sign in" goes back
// to the sign-in form with the e-mail.
func TestCreateFlow(t *testing.T) {
	a := mount(t, createPage, windowW, windowH, telatest.NoSettle())
	a.ExpectText(needFields)
	a.Expect(createButton, telatest.Disabled(true))

	a.Fill(nameBox, "Ana García")
	a.Fill(emailBox, "ana@example.com")
	a.Fill(passwordBox, "correct-horse-battery")
	a.ExpectValue(nameBox, "Ana García")
	a.ExpectValue(passwordBox, "correct-horse-battery")

	// Without the terms the button is still disabled (the recipe's
	// pointer-events-none lets the pointer through to the column below it),
	// so a press on it does nothing.
	a.Expect(createButton, telatest.Disabled(true))
	press(a, createButton)
	a.ExpectCount(creating, 0)

	a.Click(termsBox)
	a.ExpectNoText(needFields)
	a.Expect(createButton, telatest.Disabled(false))
	a.Click(createButton)
	a.Expect(creating, telatest.Disabled(true))
	a.ExpectCount(spinner, 0) // that is the sign-in button, which is gone
	a.ExpectCount(telatest.ByRole("status", ""), 0)

	a.Advance(signInDelay - 100*time.Millisecond)
	a.Expect(creating, telatest.Disabled(true))
	a.ExpectNoText(created)
	a.Advance(200 * time.Millisecond)
	a.ExpectRole("heading", created)
	a.ExpectText("Welcome, Ana García. We sent a confirmation link to ana@example.com.")
	a.ExpectCount(telatest.ByRole("status", ""), 1)
	a.ExpectCount(nameBox, 0)
	a.ExpectCount(creating, 0)

	a.Click(toSignIn)
	a.ExpectRole("heading", "Welcome back")
	a.ExpectValue(emailBox, "ana@example.com")
	a.ExpectValue(passwordBox, "")
	a.ExpectNoText(created)
}

// press presses the pointer over the center of l, like a user, whether or not
// the element takes it.
func press(a *telatest.App, l telatest.Locator) {
	r := a.Bounds(l)
	a.MouseMove(r.X+r.Width/2, r.Y+r.Height/2)
	a.MouseDown()
	a.MouseUp()
}

// TestCreateImplicitSubmission types the form key by key, then presses Enter
// in the e-mail field: the form clicks its first submit button, and does
// nothing while it is disabled, as in a browser.
func TestCreateImplicitSubmission(t *testing.T) {
	a := mount(t, createPage, windowW, windowH, telatest.NoSettle())
	a.ExpectFocused(nameBox)
	a.Press("Tab") // the name field has the focus already; Tab goes to the e-mail
	a.ExpectFocused(emailBox)
	a.Type("ana@example.com")
	a.Press("Tab")
	a.ExpectFocused(passwordBox)
	// ... nothing to submit yet: the button is disabled.
	a.Press("Enter")
	a.ExpectCount(creating, 0)

	a.Click(nameBox)
	a.Type("Ana")
	a.Fill(passwordBox, "hunter22")
	a.Click(termsBox)
	a.Click(emailBox)
	a.Press("Enter")
	a.Expect(creating, telatest.Disabled(true))
	a.Advance(signInDelay + 100*time.Millisecond)
	a.ExpectText("Welcome, Ana.")
}

// TestEmailInvalid checks what the Field's Error gives in Create account:
// the red border (the recipe's aria-invalid: classes) and the message, which
// wait for the first blur.
func TestEmailInvalid(t *testing.T) {
	a := mount(t, createPage, windowW, windowH)
	a.Click(emailBox)
	a.Type("ana@")
	a.ExpectNoText(badEmail)
	if borderIsRed(a, emailBox) {
		t.Error("the border is red before the field was left")
	}

	a.Press("Tab") // leave the field: touched
	a.ExpectText(badEmail)
	a.Expect(emailBox, telatest.Invalid(true))
	if !borderIsRed(a, emailBox) {
		t.Error("the border of an invalid address is not red")
	}

	a.Click(emailBox)
	a.Press("End")
	a.Type("example.com")
	a.ExpectNoText(badEmail)
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

// TestPasswordTooltip checks that the show/hide button says what it does
// when the pointer rests on it, and the wording follows its state.
func TestPasswordTooltip(t *testing.T) {
	a := mount(t, createPage, windowW, windowH)
	a.Hover(showButton)
	a.ExpectText("Show password")
	a.Click(showButton) // a press closes the tooltip
	a.MouseMove(5, 5)
	a.ExpectNoText("Show password")
	a.Hover(showButton)
	a.ExpectText("Hide password")
}

// TestPasswordStrength checks Strength and the meter's word under the
// password of a new account (and that Sign in has no meter).
func TestPasswordStrength(t *testing.T) {
	for p, want := range map[string]int{"": 0, "abc": 1, "abcdefgh": 1, "abcdefg1": 2, "Abcdefg1": 3, "Abcdefg1!": 4, "abcdefghijkl": 2} {
		if got := Strength(p); got != want {
			t.Errorf("Strength(%q) = %d, want %d", p, got, want)
		}
	}
	a := mount(t, createPage, windowW, windowH)
	a.ExpectRole("progressbar", "Password strength")
	for _, w := range []struct{ pass, word string }{{"abc", "Weak"}, {"abcdefg1", "Fair"}, {"Abcdefg1", "Good"}, {"Abcdefg1!", "Strong"}} {
		a.Fill(passwordBox, w.pass)
		a.ExpectText(w.word)
	}
	a = mount(t, Page{}, windowW, windowH)
	a.ExpectCount(telatest.ByRole("progressbar", ""), 0)
}

// TestPasswordNeedsEightCharacters checks the rule of a new account: seven
// characters keep the button disabled, eight enable it.
func TestPasswordNeedsEightCharacters(t *testing.T) {
	a := mount(t, Page{Mode: CreateAccount, Initial: filled}, windowW, windowH)
	a.Expect(createButton, telatest.Disabled(false))
	a.Fill(passwordBox, "Tela-26")
	a.Expect(createButton, telatest.Disabled(true))
	a.Fill(passwordBox, "Tela-262")
	a.Expect(createButton, telatest.Disabled(false))
	a.Fill(nameBox, "  ")
	a.Expect(createButton, telatest.Disabled(true))
}

// TestBio checks the counter, MaxLength and field-sizing-content with
// min-h and max-h; and that typing in a BindValue field rebuilds nothing.
func TestBio(t *testing.T) {
	a := mount(t, createPage, windowW, windowH)
	a.ExpectText("0/160")
	h0 := a.Bounds(bioBox).Height
	if h0 != 64 {
		t.Errorf("an empty bio is %v px tall, want min-h 64", h0)
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
		t.Errorf("a 5-line bio is %v px tall, want between 64 and 160 exclusive", h1)
	}
	a.Fill(bioBox, strings.Repeat("line\n", 12)+"last")
	if h2 := a.Bounds(bioBox).Height; h2 != 160 {
		t.Errorf("a long bio is %v px tall, want max-h 160", h2)
	}
	a.Fill(bioBox, "")
	if h3 := a.Bounds(bioBox).Height; h3 != 64 {
		t.Errorf("an emptied bio is %v px tall, want 64", h3)
	}

	// MaxLength counts UTF-16 code units, like HTML: an emoji is 2.
	a.Fill(bioBox, strings.Repeat("😀", 100))
	a.ExpectText("160/160")
}

// TestBioSurvivesModeSwitch checks the bio is a Value of the page: leaving
// the form and coming back shows what was written.
func TestBioSurvivesModeSwitch(t *testing.T) {
	a := mount(t, createPage, windowW, windowH)
	a.Fill(bioBox, "Writes Go.")
	a.Click(signInLink)
	a.ExpectCount(bioBox, 0)
	a.Click(signUpLink)
	a.ExpectValue(bioBox, "Writes Go.")
	a.ExpectText("10/160")
}

// TestCreateLabels checks that a Label focuses its field, and the one of the
// terms toggles its checkbox.
func TestCreateLabels(t *testing.T) {
	a := mount(t, createPage, windowW, windowH)
	a.Click(telatest.ByText("Display name", telatest.Exact()))
	a.ExpectFocused(nameBox)
	a.Click(telatest.ByText("E-mail", telatest.Exact()))
	a.ExpectFocused(emailBox)
	a.Click(telatest.ByText("Short bio"))
	a.ExpectFocused(bioBox)

	a.Click(telatest.ByText(termsLabel, telatest.Exact()))
	a.Expect(termsBox, telatest.Checked(true))
}

// TestTermsKeyboard checks the terms checkbox with the keyboard: Space
// toggles it.
func TestTermsKeyboard(t *testing.T) {
	a := mount(t, createPage, windowW, windowH)
	a.Focus(termsBox)
	a.Press("Space")
	a.Expect(termsBox, telatest.Checked(true))
	a.Press("Space")
	a.Expect(termsBox, telatest.Checked(false))
}

// TestFocusRing checks that focusing a field paints the ring around it.
func TestFocusRing(t *testing.T) {
	a := mount(t, createPage, windowW, windowH)
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
	a := mount(t, createPage, windowW, windowH)
	a.Fill(nameBox, "田中 太郎")
	a.ExpectValue(nameBox, "田中 太郎")
	a.Fill(bioBox, "日本語でも書けます。")
	a.ExpectText("10/160")
}
