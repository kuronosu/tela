package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// mount opens the screen as main does, in dev mode, where any dev message
// fails the test. (Save's tela.SetColorScheme changes the scheme of the
// mounted app only.)
func mount(t *testing.T, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.MountApp(t, App, opts...)
}

// TestGolden is the page as llms.txt describes it: light and dark, at
// scales 1 and 2.
func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, telatest.Scale(scale)).Golden(fmt.Sprintf("settings_x%g.png", scale))
			mount(t, telatest.Scale(scale), telatest.Dark()).Golden(fmt.Sprintf("settings_dark_x%g.png", scale))
		})
	}
}

// TestSaveShowsAToastAndAppliesTheTheme edits the form, saves it and looks
// at the toast and the theme it applied.
func TestSaveShowsAToastAndAppliesTheTheme(t *testing.T) {
	a := mount(t)
	save := telatest.ByRole("button", "Save changes")
	digest := telatest.ByRole("switch", "Weekly digest")

	a.Expect(save, telatest.Disabled(true))
	a.Click(digest)
	a.Expect(digest, telatest.Checked(true))
	a.Expect(save, telatest.Disabled(false))

	a.Click(telatest.ByRole("radio", "Dark"))
	a.Expect(telatest.ByRole("radio", "Dark"), telatest.Checked(true))
	a.Click(save)
	a.ExpectText("Settings saved")
	a.Expect(save, telatest.Disabled(true))
	a.MouseMove(5, 5)
	a.Golden("settings_saved_x1.png")

	// Back to the system's scheme, which the test window says is light.
	a.Click(telatest.ByRole("radio", "System"))
	a.Click(save)
	a.Expect(save, telatest.Disabled(true))
}

// TestLanguageSelect picks a language from the Select of the Field.
func TestLanguageSelect(t *testing.T) {
	a := mount(t)
	a.Click(telatest.ByRole("button", "English Language"))
	a.Click(telatest.ByRole("option", "Français"))
	a.ExpectRole("button", "Français Language")
	a.Expect(telatest.ByRole("button", "Save changes"), telatest.Disabled(false))
}

// TestNavigation follows the sidebar: the current page is marked with
// aria-current="page".
func TestNavigation(t *testing.T) {
	a := mount(t)
	settings, team := telatest.ByRole("button", "Settings"), telatest.ByRole("button", "Team")
	a.Expect(settings, telatest.Current("page"))
	a.Expect(team, telatest.Current("false"))

	a.Click(team)
	a.Expect(team, telatest.Current("page"))
	a.Expect(settings, telatest.Current("false"))
	a.ExpectText("Team is empty in this example.")
	a.ExpectNoText("Weekly digest")
}

// TestAria is the aria snapshot of the page.
func TestAria(t *testing.T) {
	mount(t).ExpectAria(`
		- navigation "Main":
		  - text: Acme
		  - button "Dashboard"
		  - button "Projects"
		  - button "Team"
		  - button "Settings"
		  - text: AL Ada Lovelace ada@example.com
		- heading "Settings" [level=1]
		- paragraph: Manage your notifications and how Acme looks.
		- button "Save changes" [disabled]
		- heading "Notifications" [level=2]
		- text: Choose what we tell you about.
		- group:
		  - text: Email notifications
		  - paragraph: A summary of activity in your workspace.
		  - switch "Email notifications" [checked]
		- group:
		  - text: Push notifications
		  - paragraph: Mentions and replies, on your devices.
		  - switch "Push notifications" [checked]
		- group:
		  - text: Weekly digest
		  - paragraph: A Monday recap of what your team shipped.
		  - switch "Weekly digest"
		- heading "Appearance" [level=2]
		- text: Applied when you save.
		- group "Theme":
		  - text: Theme
		  - radiogroup "Theme":
		    - radio "Light"
		    - radio "Dark"
		    - radio "System" [checked]
		- group:
		  - text: Language
		  - paragraph: Used for dates, numbers and messages.
		  - button "English Language": English
		- region "Notifications"
	`)
}
