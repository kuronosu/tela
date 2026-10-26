package main

import (
	"fmt"
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
	spinner     = telatest.ByClass("animate-spin")
)

// The messages the tests wait for.
const (
	needEmail    = "Enter your e-mail address."
	badEmail     = "Enter a valid e-mail address, like you@example.com."
	needPassword = "Enter your password."
	rejected     = "Incorrect e-mail or password"
	welcome      = "You're signed in"
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
		} {
			mount(t, p, s[0], s[1])
		}
		mount(t, Page{Status: Loading, Initial: valid}, s[0], s[1], telatest.NoSettle())
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

// TestGoldenDark renders the form, the alert and the welcome in the dark
// scheme, and the form at scale 2.
func TestGoldenDark(t *testing.T) {
	for _, name := range []string{"1 Sign in", "3 Wrong password", "5 Signed in"} {
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
}

// TestAriaSnapshot pins what `tela inspect --aria` prints for the form and
// for the welcome: named fields, the buttons, and no panel text hidden.
func TestAriaSnapshot(t *testing.T) {
	mount(t, Page{}, windowW, windowH).AriaGolden("login.aria.yml")
	mount(t, Page{Status: SignedIn, Initial: valid}, windowW, windowH).AriaGolden("login_signed_in.aria.yml")
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
// value swapping dots for text in the aria snapshot.
func TestPasswordToggle(t *testing.T) {
	a := mount(t, Page{}, windowW, windowH)
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
// form still fits.
func TestResponsive(t *testing.T) {
	const panel = "Pure Go. One binary."
	a := mount(t, Page{}, windowW, windowH)
	a.ExpectText(panel)
	a.Resize(700, 800)
	a.ExpectNoText(panel)
	a.ExpectRole("button", "Sign in")
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
}
