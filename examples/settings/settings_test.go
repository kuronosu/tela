package main

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/testutil/damagetest"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

const windowW, windowH = 1100, 760

// The controls by their accessible names.
var (
	save    = telatest.ByRole("button", "Save")
	discard = telatest.ByRole("button", "Discard")

	email  = telatest.ByRole("textbox", "E-mail")
	name   = telatest.ByRole("textbox", "Name")
	custom = telatest.ByRole("textbox", "Long edge (px)")

	stamp      = telatest.ByRole("switch", "Stamp exported frames")
	wmText     = telatest.ByRole("textbox", "Watermark text")
	skipDups   = telatest.ByRole("switch", "Skip duplicates")
	lens       = telatest.ByRole("switch", "Lens corrections")
	straighten = telatest.ByRole("switch", "Auto-straighten horizons")
	sharpen    = telatest.ByRole("slider", "Sharpening")
	thumb      = telatest.ByRole("slider", "Thumbnail size")
	unsavedDot = telatest.ByClass("ml-auto", "size-1.5", "bg-primary")
)

// The messages the tests wait for.
const (
	clean     = "All changes are fixed."
	dirty     = "Unsaved changes: still in the bath."
	mustFix   = "Fix the highlighted fields to save."
	customBad = "Enter a whole number of pixels between 256 and 16384."
	emailBad  = "That address doesn't look right. Try name@domain.com."
	fixed     = "Settings fixed."
)

// section is the sidebar button of a section.
func section(name string) telatest.Locator { return telatest.ByRole("button", name) }

// radio is a radio by its whole accessible name (cards add their note).
func radio(name string) telatest.Locator { return telatest.ByRole("radio", name) }

// mount opens a root in the window of main (its theme and root class), in
// dev mode, where any dev message fails the test.
func mount(t *testing.T, root tela.Component, w, h float32, opts ...telatest.Option) *telatest.App {
	t.Helper()
	app := App
	app.Root = root
	opts = append([]telatest.Option{telatest.Size(w, h)}, opts...)
	return telatest.MountApp(t, app, opts...)
}

// open mounts the window as main does, on a section, in a window tall enough
// to show a whole section (a pointer cannot click what the footer covers);
// shortWindow is the 1100x760 of main.
func open(t *testing.T, start string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mount(t, kit.Toaster{Children: Preferences{Start: start}}, windowW, 1700, opts...)
}

func shortWindow(t *testing.T, start string, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return mount(t, kit.Toaster{Children: Preferences{Start: start}}, windowW, windowH, opts...)
}

func TestNoDevMessages(t *testing.T) {
	for _, s := range [][2]float32{{windowW, windowH}, {800, 600}, {1400, 900}} {
		for _, n := range Previews.Names() {
			mount(t, Previews[n], s[0], s[1])
		}
	}
	// The whole app, and every section after the one the state of an
	// export with a custom size and a watermark.
	a := mount(t, App.Root, windowW, 1700)
	for _, p := range pages {
		a.Click(section(p.Label))
	}
	// Account and General put different inputs in the same place: the
	// field keeps its ID while one input replaces the other.
	a.Click(section("General"))
	a.Click(section("Account"))
	a.Click(section("General"))
	a.Click(section("Export"))
	a.Click(radio("Custom"))
	a.Click(stamp)
	a.Click(section("Safelight"))
	a.Click(radio("System Follows the OS."))
}

// TestGolden renders each preview, light and dark, at scale 1, and the
// Safelight page, the richest one, at scale 2.
func TestGolden(t *testing.T) {
	for _, n := range Previews.Names() {
		t.Run(n, func(t *testing.T) {
			mount(t, Previews[n], windowW, windowH).Golden(fmt.Sprintf("settings_%s_x1.png", strings.ToLower(n)))
			mount(t, Previews[n], windowW, windowH, telatest.Dark()).Golden(fmt.Sprintf("settings_%s_dark_x1.png", strings.ToLower(n)))
		})
	}
	t.Run("Safelight/x2", func(t *testing.T) {
		mount(t, Previews["Safelight"], windowW, windowH, telatest.Scale(2)).Golden("settings_safelight_x2.png")
		mount(t, Previews["Safelight"], windowW, windowH, telatest.Scale(2), telatest.Dark()).Golden("settings_safelight_dark_x2.png")
	})
}

// TestGoldenStates are what a single frame of a preview cannot show: the
// export form with a custom size that is wrong and a watermark, the Discard
// confirmation, and the toast of Save.
func TestGoldenStates(t *testing.T) {
	a := mount(t, kit.Toaster{Children: Preferences{Start: "Export"}}, windowW, 1400, telatest.Dark())
	a.Click(radio("Custom"))
	a.Fill(custom, "100")
	a.Click(stamp)
	a.Fill(wmText, "© Ana Ruiz, Darkroom")
	a.Click(radio("Black"))
	a.ExpectText(customBad)
	a.Golden("settings_export_state_dark_x1.png")

	a = shortWindow(t, "Develop")
	a.Click(radio("Tri-X Grain, contrast, black and white."))
	a.Click(discard)
	a.ExpectText("Discard your changes?")
	a.Golden("settings_discard_x1.png")
	a.Click(telatest.ByRole("button", "Keep editing"))

	a.Click(save)
	a.MouseMove(5, 5)
	a.ExpectText(fixed)
	a.Golden("settings_saved_x1.png")
}

// TestAriaSnapshot pins what `tela inspect --aria ./examples/settings`
// prints for the first section.
func TestAriaSnapshot(t *testing.T) {
	mount(t, App.Root, windowW, windowH).AriaGolden("settings.aria.yml")
}

// TestAriaSections checks the sections that the previews show have names
// for every control: the form is usable with a screen reader.
func TestAriaSections(t *testing.T) {
	for _, n := range []string{"Develop", "Export", "Safelight", "Account"} {
		a := open(t, n)
		snap := a.AriaSnapshot()
		for _, bad := range []string{`button ""`, `textbox ""`, `slider ""`, `switch ""`, `radio ""`, `combobox ""`} {
			if strings.Contains(snap, bad) {
				t.Errorf("%s: a control has no name (%s):\n%s", n, bad, snap)
			}
		}
	}
}

// TestUnsavedState: the window knows when there are unsaved changes and
// says so, in the footer and on the section's button.
func TestUnsavedState(t *testing.T) {
	a := open(t, "Develop")
	a.ExpectText(clean)
	a.Expect(save, telatest.Disabled(true))
	a.Expect(discard, telatest.Disabled(true))
	a.ExpectCount(unsavedDot, 0)

	a.Click(lens)
	a.ExpectText(dirty)
	a.Expect(save, telatest.Disabled(false))
	a.Expect(discard, telatest.Disabled(false))
	a.ExpectCount(unsavedDot, 1)
	// The mark is on the section that changed, whichever page is open.
	a.Click(section("Export"))
	a.ExpectCount(unsavedDot, 1)
	a.ExpectText(dirty)
	a.Click(section("Develop"))
	a.Click(lens)
	a.ExpectText(clean)
	a.ExpectCount(unsavedDot, 0)
	a.Expect(save, telatest.Disabled(true))

	// Every section marks itself.
	for _, tc := range []struct {
		page string
		edit func(a *telatest.App)
	}{
		{"General", func(a *telatest.App) { a.Click(skipDups) }},
		{"Develop", func(a *telatest.App) { a.Focus(sharpen); a.Press("ArrowRight") }},
		{"Export", func(a *telatest.App) { a.Click(radio("PNG Lossless, with transparency.")) }},
		{"Safelight", func(a *telatest.App) { a.Focus(thumb); a.Press("ArrowRight") }},
		{"Account", func(a *telatest.App) { a.Fill(name, "Ana Maria Ruiz") }},
	} {
		b := open(t, tc.page)
		tc.edit(b)
		b.ExpectText(dirty)
		b.ExpectCount(unsavedDot, 1)
		b.Expect(section(tc.page), telatest.Current("page"))
	}
}

// TestDiscardAsksFirst: Discard opens a confirmation; "Keep editing" keeps
// the changes and the action puts the saved values back.
func TestDiscardAsksFirst(t *testing.T) {
	a := open(t, "Develop")
	a.Click(radio("Tri-X Grain, contrast, black and white."))
	a.Focus(sharpen)
	a.Press("End")
	a.ExpectText("150")

	a.Click(discard)
	a.ExpectText("Discard your changes?")
	a.Click(telatest.ByRole("button", "Keep editing"))
	a.ExpectNoText("Discard your changes?")
	a.ExpectText(dirty)
	a.Expect(radio("Tri-X Grain, contrast, black and white."), telatest.Checked(true))

	a.Click(discard)
	a.Click(discard)
	a.MouseMove(5, 5)
	a.ExpectNoText("Discard your changes?")
	a.ExpectText(clean)
	a.Expect(radio("Portra Soft skin, warm shadows."), telatest.Checked(true))
	a.ExpectText("40")
	a.ExpectCount(unsavedDot, 0)
}

// TestCustomSizeIsValidated: the custom long edge takes 256 to 16384; an
// error shows under the field, and Save is disabled until it is right.
func TestCustomSizeIsValidated(t *testing.T) {
	a := open(t, "Export")
	a.ExpectNoText("Long edge (px)")
	a.Click(radio("Custom"))
	a.Expect(custom, telatest.HasValue("3000"))
	a.ExpectNoText(customBad)
	// Choosing custom with the default 3000 is a change, and a good one.
	a.ExpectText(dirty)
	a.Expect(save, telatest.Disabled(false))

	for _, bad := range []string{"255", "16385", "", "12ab", "3000.5", "-4000", "99999999999999999999"} {
		a.Fill(custom, bad)
		a.ExpectText(customBad)
		a.Expect(custom, telatest.Invalid(true))
		a.Expect(save, telatest.Disabled(true))
		a.ExpectText(mustFix)
	}
	for _, good := range []string{"256", "16384", "4096"} {
		a.Fill(custom, good)
		a.ExpectNoText(customBad)
		a.Expect(custom, telatest.Invalid(false))
		a.Expect(save, telatest.Disabled(false))
		a.ExpectNoText(mustFix)
	}

	// An invalid size does not matter once the size is not custom.
	a.Fill(custom, "100")
	a.Expect(save, telatest.Disabled(true))
	a.Click(radio("2048"))
	a.ExpectNoText("Long edge (px)")
	a.Expect(save, telatest.Disabled(false))
}

// TestWatermark: the switch shows the text and the color of the stamp, and
// the preview under them follows both.
func TestWatermark(t *testing.T) {
	a := open(t, "Export")
	a.ExpectNoText("Watermark text")
	a.Click(stamp)
	a.Expect(stamp, telatest.Checked(true))
	a.Expect(wmText, telatest.HasValue("© Ana Ruiz"))
	a.Expect(radio("White"), telatest.Checked(true))
	a.ExpectCount(telatest.ByClass("text-[#ffffff]"), 1)

	a.Fill(wmText, "Darkroom 2026")
	a.ExpectText("Darkroom 2026")
	a.Click(radio("Safelight"))
	a.Expect(radio("Safelight"), telatest.Checked(true))
	a.Expect(radio("White"), telatest.Checked(false))
	a.ExpectCount(telatest.ByClass("text-[#e5412f]"), 1)
	a.Click(radio("Black"))
	a.ExpectCount(telatest.ByClass("text-[#000000]"), 1)
	a.ExpectText(dirty)

	a.Click(stamp)
	a.ExpectNoText("Watermark text")
	a.ExpectText(dirty) // the text and the color are still changed
}

// TestQualityOnlyForLossyFormats: JPEG and HEIC have a quality, PNG and TIFF
// say they have none.
func TestQualityOnlyForLossyFormats(t *testing.T) {
	a := open(t, "Export")
	quality := telatest.ByRole("slider", "Quality")
	a.ExpectCount(quality, 1)
	a.Click(radio("PNG Lossless, with transparency."))
	a.ExpectCount(quality, 0)
	a.ExpectText("PNG is lossless: there is no quality to set.")
	a.Click(radio("TIFF Lossless 16-bit, for print."))
	a.ExpectText("TIFF is lossless: there is no quality to set.")
	a.Click(radio("HEIC Half the size of JPEG, newer apps only."))
	a.ExpectCount(quality, 1)
	a.Focus(quality)
	a.Press("ArrowLeft")
	a.ExpectText("89")
}

// TestSaveAppliesTheThemeAndTheAccent: Save fixes the form, shows a toast
// and applies the theme and the accent at once.
func TestSaveAppliesTheThemeAndTheAccent(t *testing.T) {
	a := open(t, "Safelight")
	bg := func() color.RGBA { return a.Screenshot().RGBAAt(270, 380) }
	dot := func() color.RGBA { return a.Screenshot().RGBAAt(38, 38) }
	wantDot := func(c tela.Color) color.RGBA { return c.Premul8() }
	light := bg()
	if got, want := dot(), wantDot(safelight); got != want {
		t.Errorf("the lamp starts as %v, want the safelight red %v", got, want)
	}

	a.Click(radio("Dark The natural habitat."))
	a.Click(radio("Cyanotype"))
	a.ExpectText(dirty)
	// Nothing is applied before Save.
	if bg() != light {
		t.Error("the theme changed before Save")
	}
	a.ExpectNoText(fixed)
	a.Click(save)
	a.ExpectText(fixed)
	a.ExpectText("Out of the bath and into the light.")
	a.ExpectText(clean)
	a.Expect(save, telatest.Disabled(true))
	a.MouseMove(5, 5)
	if bg() == light {
		t.Error("Save did not switch to the dark scheme")
	}
	if got, want := dot(), wantDot(tela.Hex("#1f6f9c")); got != want {
		t.Errorf("the lamp is %v after Save, want the cyanotype blue %v", got, want)
	}
	a.Expect(radio("Dark The natural habitat."), telatest.Checked(true))
	a.Expect(radio("Cyanotype"), telatest.Checked(true))

	// And back: System follows the OS, which is light in a test.
	a.Click(radio("System Follows the OS."))
	a.Click(radio("Safelight"))
	a.Click(save)
	a.MouseMove(5, 5)
	if bg() != light {
		t.Error("Save with System did not follow the OS (light)")
	}
	if got, want := dot(), wantDot(safelight); got != want {
		t.Errorf("the lamp is %v, want the safelight red %v", got, want)
	}
}

// TestCustomAccent: a color of the picker is the accent, and is no preset.
func TestCustomAccent(t *testing.T) {
	a := open(t, "Safelight")
	a.Click(telatest.ByRole("button", "Custom accent color, #c0281c"))
	hex := telatest.ByRole("textbox", "Hex")
	a.Fill(hex, "#2e8b57")
	a.Press("Enter")
	a.Press("Escape")
	a.ExpectText(dirty)
	for _, p := range []string{"Safelight", "Amber", "Sepia", "Cyanotype", "Silver"} {
		a.Expect(radio(p), telatest.Checked(false))
	}
}

// TestSaveNeedsAValidForm: with a wrong e-mail Save is disabled, the footer
// says why, Ctrl+S does nothing, and once it is right, Ctrl+S saves from the
// field that has the focus.
func TestSaveNeedsAValidForm(t *testing.T) {
	a := open(t, "Account")
	a.Fill(email, "ana.ruiz@darkroom")
	a.ExpectText(emailBad)
	a.Expect(email, telatest.Invalid(true))
	a.Expect(save, telatest.Disabled(true))
	a.ExpectText(mustFix)
	a.Focus(email)
	a.Press("ControlOrMeta+s")
	a.ExpectNoText(fixed)
	a.ExpectText(mustFix)

	for _, bad := range []string{"", "ana", "ana@", "@darkroom.app", "ana ruiz@darkroom.app", "ana@darkroom.a"} {
		a.Fill(email, bad)
		a.ExpectText(emailBad)
		a.Expect(save, telatest.Disabled(true))
	}

	a.Fill(email, "ana.ruiz@darkroom.app")
	a.Expect(email, telatest.Invalid(false))
	a.ExpectNoText(emailBad)
	a.Expect(save, telatest.Disabled(true)) // same as saved: nothing to save
	a.Fill(email, "ana@darkroom.app")
	a.Expect(save, telatest.Disabled(false))
	a.ExpectText(dirty)
	a.Focus(email)
	a.Press("ControlOrMeta+s")
	a.ExpectText(fixed)
	a.ExpectText(clean)
}

// TestShortcutFromAnyControl: Ctrl+S (Cmd+S on a Mac) saves whichever
// control has the focus, and does nothing when there is nothing to save.
func TestShortcutFromAnyControl(t *testing.T) {
	a := open(t, "Develop")
	a.Focus(lens)
	a.Press("ControlOrMeta+s")
	a.ExpectNoText(fixed)

	a.Press("Space") // flips the focused switch
	a.Expect(lens, telatest.Checked(false))
	a.ExpectText(dirty)
	a.Press("ControlOrMeta+s")
	a.ExpectText(fixed)
	a.Expect(save, telatest.Disabled(true))

	a.Focus(straighten)
	a.Press("Space")
	a.ExpectText(dirty)
	a.Press("ControlOrMeta+s")
	a.ExpectText(clean)

	// A slider has the focus and the arrows: Ctrl+S still reaches the window.
	a.Focus(sharpen)
	a.Press("ArrowRight")
	a.Press("ControlOrMeta+s")
	a.ExpectText(clean)
	a.Expect(save, telatest.Disabled(true))
}

// TestShortcutPerPlatform: the shortcut is the platform's primary
// modifier (KeyboardEvent.CmdOrCtrl): Cmd+S on a Mac, where Ctrl+S does
// nothing, and Ctrl+S elsewhere, where Meta+S does nothing.
func TestShortcutPerPlatform(t *testing.T) {
	for _, c := range []struct{ goos, saves, ignored string }{
		{"darwin", "Meta+s", "Control+s"},
		{"linux", "Control+s", "Meta+s"},
		{"windows", "Control+s", "Meta+s"},
	} {
		t.Run(c.goos, func(t *testing.T) {
			a := open(t, "Develop", telatest.PlatformKeys(c.goos))
			a.Focus(lens)
			a.Press("Space")
			a.ExpectText(dirty)
			a.Press(c.ignored)
			a.ExpectText(dirty)
			a.Press(c.saves)
			a.ExpectText(clean)
		})
	}
}

// TestSignOutConfirms: Sign out everywhere asks first; Cancel does nothing.
func TestSignOutConfirms(t *testing.T) {
	a := open(t, "Account")
	trigger := telatest.ByRole("button", "Sign out everywhere")
	a.Click(trigger)
	a.ExpectText("Sign out of all devices?")
	a.ExpectNoText("Signed out of all devices.")
	a.Click(telatest.ByRole("button", "Cancel"))
	a.ExpectNoText("Sign out of all devices?")
	a.ExpectNoText("Signed out of all devices.")

	a.Click(trigger)
	a.Click(telatest.ByRole("button", "Sign out"))
	a.MouseMove(5, 5)
	a.ExpectNoText("Sign out of all devices?")
	a.ExpectText("Signed out of all devices.")
	// Signing out changes no preference.
	a.ExpectText(clean)
}

// TestKeyboardSidebar: Tab walks the sections in order and Enter opens the
// one that has the focus; the current one is aria-current.
func TestKeyboardSidebar(t *testing.T) {
	a := open(t, "General")
	a.Focus(section("General"))
	a.Expect(section("General"), telatest.Current("page"))
	for _, next := range []string{"Develop", "Export", "Safelight", "Account"} {
		a.Press("Tab")
		a.Expect(section(next), telatest.Focused(true))
		a.Press("Enter")
		a.Expect(section(next), telatest.Current("page"))
		a.ExpectRole("heading", next)
	}
	a.Press("Shift+Tab")
	a.Expect(section("Safelight"), telatest.Focused(true))
	a.Press("Space")
	a.Expect(section("Safelight"), telatest.Current("page"))
	a.Expect(section("Account"), telatest.Current("false"))
}

// TestKeyboardSections reaches and changes the controls of a section
// without a pointer: arrows choose a card, Space flips a switch, the arrows
// of a slider move it, and a select opens with the keyboard.
func TestKeyboardSections(t *testing.T) {
	a := open(t, "Develop")
	a.Focus(radio("Portra Soft skin, warm shadows."))
	a.Press("ArrowRight")
	a.Expect(radio("Tri-X Grain, contrast, black and white."), telatest.Checked(true))
	a.Press("ArrowDown")
	a.Press("ArrowLeft")
	a.ExpectText(dirty)

	a.Focus(sharpen)
	a.Press("ArrowRight")
	a.Press("ArrowRight")
	a.ExpectText("42")
	a.Press("Home")
	a.ExpectText("0")
	a.Press("End")
	a.ExpectText("150")

	a.Focus(straighten)
	a.Press("Space")
	a.Expect(straighten, telatest.Checked(true))

	// Tab order inside a section goes down the page.
	a.Click(section("General"))
	a.Focus(telatest.ByRole("button", "Change…"))
	a.Press("Tab")
	a.Expect(radio("Copy into the library"), telatest.Focused(true))

	a.Click(section("Export"))
	a.Focus(telatest.ByRole("button", "sRGB Color space"))
	a.Press("Enter")
	a.Press("ArrowDown")
	a.Press("Enter")
	a.ExpectRole("button", "Display P3 Color space")
}

// TestLibraryFolder: Change… asks the OS for a folder; a cancel changes
// nothing.
func TestLibraryFolder(t *testing.T) {
	a := open(t, "General")
	library := telatest.ByRole("textbox", "Library folder path")
	a.Expect(library, telatest.HasValue(defaults.Library))
	a.Click(telatest.ByRole("button", "Change…"))
	a.AnswerFileDialog()
	a.Expect(library, telatest.HasValue(defaults.Library))
	a.ExpectText(clean)

	a.Click(telatest.ByRole("button", "Change…"))
	a.AnswerFileDialog("/data/film")
	a.Expect(library, telatest.HasValue("/data/film"))
	a.ExpectText(dirty)
	a.Click(save)
	a.ExpectText(fixed)
}

// TestLanguage picks a language from the select.
func TestLanguage(t *testing.T) {
	a := open(t, "General")
	a.Click(telatest.ByRole("button", "English Interface language"))
	a.Click(telatest.ByRole("option", "日本語"))
	a.ExpectRole("button", "日本語 Interface language")
	a.ExpectText(dirty)
}

// TestDensityOnSave: the density of the saved preferences sets the space
// between the cards of every section, and takes effect on Save.
func TestDensityOnSave(t *testing.T) {
	a := open(t, "Safelight")
	space := func() float32 {
		theme, accent := telatest.ByRole("heading", "Theme"), telatest.ByRole("heading", "Accent")
		return a.Bounds(accent).Y - a.Bounds(theme).Y
	}
	normal := space()
	a.Click(radio("Compact"))
	a.Expect(save, telatest.Disabled(false))
	if space() != normal {
		t.Error("the density changed before Save")
	}
	a.Click(save)
	compact := space()
	a.Click(radio("Comfortable"))
	a.Click(save)
	comfortable := space()
	if !(compact < normal && normal < comfortable) {
		t.Errorf("the space between cards is %v (compact), %v (default), %v (comfortable): want it to grow", compact, normal, comfortable)
	}
	a.Click(section("Export"))
	a.Click(section("Safelight"))
	if space() != comfortable {
		t.Error("the saved density was lost when the section changed")
	}
}

// TestThumbnailSize: the preview of the contact sheet follows the slider.
func TestThumbnailSize(t *testing.T) {
	a := open(t, "Safelight")
	a.ExpectText("160 px")
	a.Focus(thumb)
	a.Press("End")
	a.ExpectText("256 px")
	a.Press("Home")
	a.ExpectText("96 px")
}

// TestGoesIdle: once a section has been drawn, a window nobody touches asks
// for no frames: no timer, no endless animation.
func TestGoesIdle(t *testing.T) {
	for _, n := range Previews.Names() {
		t.Run(n, func(t *testing.T) {
			a := mount(t, Previews[n], windowW, windowH, telatest.NoSettle())
			a.Advance(3 * time.Second)
			before := a.Stats()
			a.Advance(2 * time.Second)
			if after := a.Stats(); after.Frames != before.Frames {
				t.Errorf("still requesting frames 3 s after the mount: %d frames in the next 2 s (%+v)", after.Frames-before.Frames, after)
			}
		})
	}
}

// TestScrollDamage scrolls and hovers the sections in a short window at
// scales 1, 1.5 and 2, checking every frame against a full rendering.
func TestScrollDamage(t *testing.T) {
	for _, n := range []string{"Export", "Safelight"} {
		for _, scale := range []float64{1, 1.5, 2} {
			damagetest.ScrollAround(t, func(opts ...telatest.Option) *telatest.App {
				return mount(t, Previews[n], windowW, windowH, opts...)
			}, 900, 420, scale, 24)
		}
	}
}

// TestDamage plays the interactions most likely to paint wrong
// incrementally, checking every frame against a full rendering: a theme and
// accent switch that restyles the whole window, the dialogs and the toast.
func TestDamage(t *testing.T) {
	a := open(t, "Safelight", telatest.CheckDamage())
	a.Click(radio("Dark The natural habitat."))
	a.Click(radio("Amber"))
	a.Click(save)
	a.Advance(6 * time.Second)
	a.Click(radio("Light Paper white, for bright rooms."))
	a.Click(discard)
	a.Press("Escape")
	a.Click(discard)
	a.Click(discard)
	a.Click(section("Account"))
	a.Click(telatest.ByRole("button", "Sign out everywhere"))
	a.Press("Escape")
	a.Click(section("Export"))
	a.Click(radio("Custom"))
	a.Fill(custom, "12")
	a.Click(stamp)
	a.Press("ControlOrMeta+s")
}
