package main

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

// The registration goes through the headless desktop, which plays the OS:
// nothing here touches the real applications folder or the registry.

var (
	registerBtn    = telatest.ByRole("button", "Register")
	unregisterBtn  = telatest.ByRole("button", "Unregister")
	makeDefault    = telatest.ByRole("button", "Make default")
	markdownSwitch = telatest.ByRole("switch", "Open .md files with Notes")
)

// autoRegistering makes the app register its scheme at start, as main does.
func autoRegistering(t *testing.T) {
	t.Helper()
	autoRegister.Store(true)
	t.Cleanup(func() { autoRegister.Store(false) })
}

// registered returns the registration of an association, if any.
func registered(a *telatest.App, as desktop.Association) (telatest.Registration, bool) {
	for _, r := range a.Registrations() {
		if r.Association == as.String() {
			return r, true
		}
	}
	return telatest.Registration{}, false
}

func TestNothingIsRegisteredInTests(t *testing.T) {
	sandbox(t)
	a := mount(t)
	if regs := a.Registrations(); len(regs) != 0 {
		t.Errorf("a test registered %+v", regs)
	}
}

// At start Notes registers the notes: scheme: a candidate, not the default
// (another application is).
func TestRegistersTheSchemeAtStart(t *testing.T) {
	sandbox(t)
	autoRegistering(t)
	a := mount(t)
	a.SetDefaultHandler(notesScheme, "Other App")
	a.WaitFor("the scheme to be registered", func() bool { _, ok := registered(a, notesScheme); return ok })
	r, ok := registered(a, notesScheme)
	if !ok || r.App.ID != "dev.telaui.notes" || r.App.Name != "Notes" || r.Default {
		t.Fatalf("registration = %+v (found %v), want Notes as a candidate that is not the default", r, ok)
	}
	if _, ok := registered(a, markdownType); ok {
		t.Error(".md files were registered without being asked")
	}
}

// Nothing registers it when the user turned it off, when another
// application holds the scheme (it is not taken), or for a temporary build.
func TestDoesNotRegisterWhenItShouldNot(t *testing.T) {
	for _, c := range []struct {
		name string
		// start sets things up before the app starts; then holds is what
		// another application does after it started, with the setting on
		// (turning it on is what makes the app try again).
		start func(t *testing.T)
		other func(a *telatest.App)
	}{
		{"setting off", func(t *testing.T) { setSetting(t, keyRegisterLinks, false) }, nil},
		{"held by another", func(t *testing.T) { setSetting(t, keyRegisterLinks, false) },
			func(a *telatest.App) { a.HoldAssociation(notesScheme, "Other App") }},
		{"temporary build", func(t *testing.T) { blockRegistration(t, "temporary") }, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			sandbox(t)
			c.start(t)
			autoRegistering(t)
			base := registerCalls.Load()
			a := mount(t)
			a.WaitFor("the start to consider registering", func() bool { return registerCalls.Load() > base })
			if c.other != nil {
				c.other(a)
				setSetting(t, keyRegisterLinks, true)
				a.WaitFor("the app to try again", func() bool { return registerCalls.Load() > base+1 })
			}
			if regs := a.Registrations(); len(regs) != 0 {
				t.Errorf("registered %+v", regs)
			}
			a.ExpectNoText("Could not") // and says nothing: Preferences tells
		})
	}
}

func TestPreferencesLinksStatus(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a, "General")
	prefs.ExpectText("Links and files")
	prefs.ExpectText("Open notes: links")
	prefs.ExpectText("Not registered.")
	prefs.ExpectText("Notes is not offered for .md files.")
	prefs.ExpectCount(makeDefault, 0) // nothing to make default yet
	prefs.ExpectCount(unregisterBtn, 0)
}

// Register and Unregister, and what they leave in the OS.
func TestPreferencesRegisterAndUnregister(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.SetDefaultHandler(notesScheme, "Other App")
	prefs := openPreferences(a, "General")

	prefs.Click(registerBtn)
	prefs.ExpectText("Registered. Other App is the default.")
	prefs.ExpectRole("button", "Unregister")
	r, ok := registered(a, notesScheme)
	if !ok || r.Default || r.App.ID != "dev.telaui.notes" {
		t.Errorf("registration = %+v (found %v)", r, ok)
	}

	prefs.Click(unregisterBtn)
	prefs.ExpectText("Held by Other App.") // it is the default of the scheme, not Notes
	if _, ok := registered(a, notesScheme); ok {
		t.Error("Unregister left Notes registered")
	}
	// It is the user's choice, kept for the next start.
	var wanted bool
	if !desktop.Settings().Get(keyRegisterLinks, &wanted) || wanted {
		t.Errorf("the setting after Unregister = %v, want false", wanted)
	}
	a.Close()

	// The next start does not register it again.
	autoRegistering(t)
	base := registerCalls.Load()
	b := mount(t)
	b.WaitFor("the start to consider registering", func() bool { return registerCalls.Load() > base })
	if _, ok := registered(b, notesScheme); ok {
		t.Error("a scheme the user unregistered was registered at start")
	}
}

// Another application's scheme is never taken: the error says whose it is.
func TestPreferencesRegisterRefusesWhatAnotherHolds(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.HoldAssociation(notesScheme, "Other App")
	prefs := openPreferences(a, "General")
	prefs.ExpectText("Held by Other App.")
	prefs.Click(registerBtn)
	prefs.ExpectText("holds notes:. Notes does not take it over.")
	if regs := a.Registrations(); len(regs) != 0 {
		t.Errorf("registered over another application: %+v", regs)
	}
}

// Make default is a button of its own, and only there when it applies.
func TestPreferencesMakeDefault(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.SetDefaultHandler(notesScheme, "Other App")
	prefs := openPreferences(a, "General")
	prefs.ExpectCount(makeDefault, 0)

	prefs.Click(registerBtn)
	prefs.ExpectText("Registered. Other App is the default.")
	if r, _ := registered(a, notesScheme); r.Default {
		t.Fatal("registering made Notes the default")
	}
	prefs.Click(makeDefault)
	prefs.ExpectText("Registered, and the default for notes: links.")
	prefs.ExpectCount(makeDefault, 0)
	if r, _ := registered(a, notesScheme); !r.Default {
		t.Error("Make default did not make Notes the default")
	}

	// The default changes in the system's settings: Preferences notices
	// when it gets the focus again.
	a.Click(editor)
	a.SetDefaultHandler(notesScheme, "Other App")
	prefs.Click(telatest.ByRole("button", "General"))
	prefs.ExpectText("Registered. Other App is the default.")
}

func TestPreferencesMarkdownFiles(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.SetDefaultHandler(markdownType, "Other Editor")
	prefs := openPreferences(a, "General")
	prefs.ExpectText("Other Editor opens .md files. Notes is not offered.")
	prefs.Expect(markdownSwitch, telatest.Checked(false))

	prefs.Click(markdownSwitch)
	prefs.Expect(markdownSwitch, telatest.Checked(true))
	prefs.ExpectText("Notes is in “Open with”. Other Editor is the default.")
	r, ok := registered(a, markdownType)
	if !ok || r.Default {
		t.Fatalf(".md registration = %+v (found %v), want a candidate that is not the default", r, ok)
	}
	prefs.ExpectCount(makeDefault.Nth(0), 1)
	prefs.Click(makeDefault)
	prefs.ExpectText("Notes is the default app for .md files.")
	if r, _ := registered(a, markdownType); !r.Default {
		t.Error("Make default did not make Notes the default for .md")
	}

	prefs.Click(markdownSwitch)
	prefs.Expect(markdownSwitch, telatest.Checked(false))
	if _, ok := registered(a, markdownType); ok {
		t.Error("turning the switch off left Notes registered for .md")
	}
}

// Windows leaves the choice of a file type's default to the user.
func TestDefaultAskedMessage(t *testing.T) {
	if got := defaultMessage(desktop.DefaultAsked, markdownType); !strings.Contains(got, "Windows opened the Default apps settings") || !strings.Contains(got, ".md") {
		t.Errorf("message = %q", got)
	}
	if got := defaultMessage(desktop.DefaultSet, markdownType); got != "" {
		t.Errorf("message after setting the default = %q, want none", got)
	}
}

// A desktop that cannot register applications says so.
func TestPreferencesRegisterUnsupported(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.Associations)
	prefs := openPreferences(a, "General")
	prefs.ExpectText("This desktop cannot register applications.")
}

// A program that is not installed cannot be registered: Preferences says why.
func TestPreferencesTemporaryBuild(t *testing.T) {
	sandbox(t)
	blockRegistration(t, "This is a temporary build (go run).")
	a := mount(t)
	prefs := openPreferences(a, "General")
	prefs.ExpectText("This is a temporary build (go run).")
	prefs.Expect(registerBtn, telatest.Disabled(true))
	prefs.Expect(markdownSwitch, telatest.Disabled(true))
}

func TestGoldenPreferencesGeneral(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.SetDefaultHandler(notesScheme, "Firefox")
	prefs := openPreferences(a, "General")
	prefs.Click(registerBtn)
	prefs.ExpectText("Registered. Firefox is the default.")
	prefs.MouseMove(300, 420)
	prefs.Golden("preferences_general.png")
}
