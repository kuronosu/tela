package main

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

// The parts of the rail, the lists and the editor's header.
var (
	searchField = telatest.ByRole("searchbox", "Search notes")
	pinNote     = telatest.ByRole("button", "Pin note")
)

func railItem(name string) telatest.Locator { return telatest.ByRole("button", name) }

// card is the card of a note, in the list or in the overview.
func card(heading string) telatest.Locator { return telatest.ByRole("button", heading) }

// folderMenu is the button of the editor's header that opens the menu of
// the folders.
func folderMenu(name string) telatest.Locator {
	return telatest.ByRole("button", "Folder: "+name)
}

const groceriesText = "Coffee beans, oats, lemons, olive oil. Ask at the bakery if they still do the seeded rye.\n\n#errands"

// The overview of a filter, a card that opens a note, and the ways back:
// Escape in the list (which first clears the search), and the rail's item
// that is already chosen.
func TestOverviewOpenNoteAndBack(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectCount(editor, 1) // the first run opens the welcome note
	a.ExpectCount(searchField, 1)

	// A rail item shows the overview: no note, a heading and a card for
	// each note of the filter.
	a.Click(railItem("All notes"))
	a.ExpectCount(editor, 0)
	a.ExpectRole("heading", "All notes")
	for _, h := range []string{"Welcome to Notes", "Keyboard shortcuts", "Groceries", "Weekend in the mountains"} {
		a.ExpectRole("button", h)
	}

	// Another filter: the notes of the folder only.
	a.Click(railItem("Home"))
	a.ExpectRole("heading", "Home")
	a.ExpectRole("button", "Groceries")
	a.ExpectCount(card("Welcome to Notes"), 0)

	// A card opens the note, with the list of the same filter next to it.
	a.Click(card("Groceries"))
	a.ExpectValue(editor, groceriesText)
	a.ExpectValue(titleBox, "Groceries")
	a.Expect(card("Groceries"), telatest.Current("page"))
	a.Expect(railItem("Home"), telatest.Current("page"))
	a.ExpectCount(card("Welcome to Notes"), 0)

	// Escape in the list goes back to the overview of the filter.
	a.Click(searchField)
	a.Press("Escape")
	a.ExpectCount(editor, 0)
	a.ExpectRole("heading", "Home")

	// So does the item of the rail that is already chosen.
	a.Click(card("Groceries"))
	a.ExpectCount(editor, 1)
	a.Click(railItem("Home"))
	a.ExpectCount(editor, 0)
	a.ExpectRole("heading", "Home")

	// A search is cleared by the first Escape; the second goes back.
	a.Click(railItem("All notes"))
	a.Click(card("Groceries"))
	a.Click(searchField)
	a.Type("rye")
	a.ExpectCount(card("Welcome to Notes"), 0)
	a.Press("Escape")
	a.ExpectValue(searchField, "")
	a.ExpectCount(editor, 1)
	a.ExpectRole("button", "Welcome to Notes")
	a.Press("Escape")
	a.ExpectCount(editor, 0)
}

func TestFilterViewsAndCounts(t *testing.T) {
	sandbox(t)
	a := mount(t)
	with := func(name, count string) telatest.Locator { return railItem(name).Filter(telatest.HasText(count)) }
	a.ExpectCount(with("All notes", "4"), 1)
	a.ExpectCount(with("Pinned", "1"), 1)
	a.ExpectCount(with("Getting started", "2"), 1)
	a.ExpectCount(with("Home", "1"), 1)
	a.ExpectCount(with("Unsaved", "1"), 0) // no count while there is none

	// Pinned shows the pinned notes.
	a.Click(railItem("Pinned"))
	a.ExpectRole("heading", "Pinned")
	a.ExpectRole("button", "Welcome to Notes")
	a.ExpectCount(card("Groceries"), 0)

	// A tag.
	a.Click(railItem("#travel"))
	a.ExpectRole("heading", "#travel")
	a.ExpectRole("button", "Weekend in the mountains")
	a.ExpectCount(card("Groceries"), 0)

	// Unsaved is the notes that changed since their file: a new one that
	// has text.
	a.Click(railItem("All notes"))
	a.Click(card("Groceries"))
	a.Click(editor)
	a.Press("Control+End")
	a.Type(" more")
	a.ExpectCount(with("Unsaved", "1"), 1)
	a.Click(railItem("Unsaved"))
	a.ExpectRole("heading", "Unsaved")
	a.ExpectRole("button", "Groceries, unsaved changes")
	a.ExpectCount(card("Welcome to Notes"), 0)
}

func TestSearchFiltersTitleAndText(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(railItem("All notes"))

	a.Click(searchField)
	a.Type("oats") // in the text of Groceries
	a.ExpectRole("button", "Groceries")
	a.ExpectCount(card("Welcome to Notes"), 0)
	a.ExpectRole("heading", "All notes")

	a.Press("Control+a")
	a.Type("shortcuts") // in the title of a note
	a.ExpectRole("button", "Keyboard shortcuts")
	a.ExpectCount(card("Groceries"), 0)

	a.Press("Control+a")
	a.Type("zzz")
	a.ExpectText(`Nothing matches "zzz".`)

	// The search applies inside a filter too, and the rail keeps counting
	// all the notes.
	a.Press("Control+a")
	a.Type("oats")
	a.Click(railItem("Pinned"))
	a.ExpectText(`Nothing matches "oats".`)
	a.ExpectCount(railItem("All notes").Filter(telatest.HasText("4")), 1)
}

// Tags are the #hashtags of the text: the rail, the header and the cards
// follow what is typed.
func TestTagsFollowTheText(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"))
	a.ExpectCount(railItem("#weekly"), 0)
	a.Click(editor)
	a.Press("Control+End")
	a.Type(" #weekly")
	a.ExpectCount(railItem("#weekly"), 1)
	a.ExpectCount(telatest.ByText("#weekly", telatest.Exact()), 2) // the rail's chip and the header's tag

	// A heading and a word with a # inside are not tags.
	a.Type("\n# Heading and C#")
	a.ExpectCount(railItem("#heading"), 0)
	a.ExpectCount(railItem("#"), 0)
}

func TestNewNoteIsInTheChosenPlace(t *testing.T) {
	sandbox(t)
	a := mount(t)

	// In a folder: the note is in it and the list is still the folder's.
	a.Click(railItem("Home"))
	a.Click(newNote)
	a.ExpectRole("button", folderMenuName("Home"))
	a.ExpectRole("button", "Untitled note")
	a.ExpectRole("button", "Groceries")
	a.ExpectCount(card("Welcome to Notes"), 0)
	a.ExpectCount(railItem("Home").Filter(telatest.HasText("2")), 1)

	// In Pinned: the note is pinned.
	a.Click(railItem("Pinned"))
	a.Click(newNote)
	a.Expect(pinNote, telatest.Pressed(true))
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("2")), 1)

	// In a view or a tag it would not show: the list goes back to all the
	// notes and the new one is in it.
	a.Click(railItem("#travel"))
	a.Click(newNote)
	a.ExpectRole("button", "Groceries")
	a.Expect(railItem("All notes"), telatest.Current("page"))
	a.Expect(pinNote, telatest.Pressed(false))
}

func folderMenuName(name string) string { return "Folder: " + name }

func TestEditTitle(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectValue(titleBox, "Welcome to Notes")
	a.Click(titleBox)
	a.Press("Control+a")
	a.Type("Start here")
	a.ExpectValue(titleBox, "Start here")
	a.ExpectRole("button", "Start here") // the card
	a.ExpectCount(card("Welcome to Notes"), 0)
	a.ExpectValue(editor, welcomeText) // the text is not touched
	a.ExpectCount(unsaved, 0)          // the title is not what a file holds

	// Enter in the title goes to the text.
	a.Press("Enter")
	a.ExpectFocused(editor)
	a.ExpectValue(titleBox, "Start here")

	// The title names the file Save suggests, and the file has the text.
	a.Click(saveBtn)
	dialogs := a.FileDialogs()
	if len(dialogs) != 1 || dialogs[0].DefaultName != "Start here.txt" {
		t.Fatalf("dialogs after Save = %+v, want a save dialog suggesting Start here.txt", dialogs)
	}
	target := filepath.Join(t.TempDir(), "start.txt")
	a.AnswerFileDialog(target)
	if got := read(t, target); got != welcomeText {
		t.Errorf("the file has %q, want the text of the note only", got)
	}
	a.ExpectValue(titleBox, "Start here") // the title stays, whatever the file is called

	// And it is stored in the library.
	a.Close()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	b := mount(t)
	b.ExpectValue(titleBox, "Start here")
}

func TestMoveToFolders(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectCount(railItem("Recipes"), 0)

	// To a folder that exists.
	a.Click(folderMenu("Getting started"))
	a.Click(telatest.ByRole("menuitemradio", "Home"))
	a.ExpectRole("button", folderMenuName("Home"))
	a.ExpectCount(railItem("Home").Filter(telatest.HasText("2")), 1)
	a.ExpectCount(railItem("Getting started").Filter(telatest.HasText("1")), 1)

	// To a new one, which asks for its name; the rail lists it once a
	// note is in it. A name of an existing folder in another case is that
	// folder.
	a.Click(folderMenu("Home"))
	a.Click(telatest.ByRole("menuitem", "New folder…"))
	name := telatest.ByRole("textbox", "Folder name")
	create := telatest.ByRole("button", "Create")
	a.Expect(create, telatest.Disabled(true))
	a.Click(name)
	a.Type("Recipes")
	a.Click(create)
	a.ExpectRole("button", folderMenuName("Recipes"))
	a.ExpectCount(railItem("Recipes").Filter(telatest.HasText("1")), 1)
	a.ExpectCount(railItem("Home").Filter(telatest.HasText("1")), 1)

	a.Click(folderMenu("Recipes"))
	a.Click(telatest.ByRole("menuitem", "New folder…"))
	a.Click(name)
	a.Type("home")
	a.Press("Enter") // the form's submit
	a.ExpectRole("button", folderMenuName("Home"))
	a.ExpectCount(railItem("Recipes"), 0) // no note is in it any more
	a.ExpectCount(railItem("Home").Filter(telatest.HasText("2")), 1)

	// Cancel changes nothing; No folder takes the note out of its folder.
	a.Click(folderMenu("Home"))
	a.Click(telatest.ByRole("menuitem", "New folder…"))
	a.Click(telatest.ByRole("button", "Cancel"))
	a.ExpectRole("button", folderMenuName("Home"))
	a.Click(folderMenu("Home"))
	a.Click(telatest.ByRole("menuitemradio", "No folder"))
	a.ExpectRole("button", folderMenuName("No folder"))
	a.ExpectCount(railItem("Home").Filter(telatest.HasText("1")), 1)
}

func TestPinNote(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Expect(pinNote, telatest.Pressed(true)) // the welcome note is pinned
	a.Click(card("Groceries"))
	a.Expect(pinNote, telatest.Pressed(false))
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("1")), 1)

	a.Click(pinNote)
	a.Expect(pinNote, telatest.Pressed(true))
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("2")), 1)
	a.ExpectCount(unsaved, 0) // pinning is not an edit of the text

	a.Click(railItem("Pinned"))
	a.ExpectRole("button", "Groceries")
	a.ExpectRole("button", "Welcome to Notes")
	a.Click(card("Groceries"))
	a.Click(pinNote)
	a.Expect(pinNote, telatest.Pressed(false))
	a.ExpectCount(card("Groceries"), 0) // the list is the pinned notes: it left
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("1")), 1)

	// What is not pinned any more is not pinned at the next start either
	// (the library is decoded over the welcome notes' defaults).
	a.Click(railItem("All notes"))
	a.Click(card("Welcome to Notes"))
	a.Click(pinNote)
	a.Close()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	b := mount(t)
	b.ExpectCount(railItem("Pinned").Filter(telatest.HasText("0")), 0)
	b.Expect(pinNote, telatest.Pressed(false))
}

// The dates of the cards come from the clock of the app, and every edit of
// the title or the text sets one.
func TestDatesOfTheCards(t *testing.T) {
	sandbox(t)
	a := mount(t)
	for _, when := range []string{"Today, 09:41", "Today, 08:15", "Yesterday, 22:03", "Mon"} {
		a.ExpectText(when)
	}

	setClock(t, fixedNow.Add(24*time.Minute))
	a.Click(card("Groceries"))
	a.Click(editor)
	a.Type("!")
	a.ExpectText("Today, 10:05")
	a.ExpectNoText("Yesterday, 22:03")

	// Pinning is not an edit.
	a.Click(card("Keyboard shortcuts"))
	a.Click(pinNote)
	a.ExpectText("Today, 08:15")
}

// A library of an earlier version of Notes (no titles, folders, pins or
// dates) loads and shows its notes without any of them.
func TestOldLibraryLoads(t *testing.T) {
	sandbox(t)
	dir, err := desktop.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	old := `{"notes":[
		{"id":1,"text":"Groceries\nmilk and eggs","saved":"Groceries\nmilk and eggs"},
		{"id":2,"text":"Todo\n- ship it\n","saved":"Todo\n- ship it\n","path":"/home/ana/todo.md"}],
		"selected":1}`
	if err := os.WriteFile(filepath.Join(dir, "notes.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	a := mount(t)
	a.ExpectValue(editor, "Groceries\nmilk and eggs")
	a.ExpectValue(titleBox, "") // the title is the first line of the text
	a.ExpectRole("button", "Groceries")
	a.ExpectRole("button", "todo.md")
	a.ExpectCount(railItem("Pinned").Filter(telatest.HasText("0")), 0)
	a.ExpectCount(railItem("All notes").Filter(telatest.HasText("2")), 1)
	a.ExpectCount(railItem("Folders"), 0)
	a.ExpectNoText("Folders")
	a.ExpectNoText("Today")
	a.ExpectNoText("Mon")
	a.ExpectCount(unsaved, 0)
	a.ExpectRole("button", folderMenuName("No folder"))

	// The first edit dates the note.
	a.Click(editor)
	a.Type("!")
	a.ExpectText("Today, 09:41")
	a.ExpectRole("button", "Groceries, unsaved changes")
}

func TestShowPreviewsSetting(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ExpectText("Ctrl+N makes a note") // the preview of a card of the list
	prefs := openPreferences(a)
	prefs.Click(telatest.ByRole("switch", "Show previews in the list"))
	a.ExpectNoText("Ctrl+N makes a note")
	a.ExpectRole("button", "Keyboard shortcuts")

	// The overview always has them.
	a.Click(railItem("All notes"))
	a.ExpectText("Ctrl+N makes a note")
}

// The theme and the accent of Preferences apply to the whole app, and are
// still there at the next start.
func TestAppearancePreferences(t *testing.T) {
	sandbox(t)
	a := mount(t)
	// A pixel of the rail (stone-200 in the light scheme, graphite in the
	// dark one) and of the New note button (the accent).
	const railX, railY, btnX, btnY = 4, 400, 20, 65
	rgb := func(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }
	check := func(app *telatest.App, what string, x, y float32, want color.RGBA) {
		t.Helper()
		if got := app.PixelAt(x, y); got != want {
			t.Errorf("%s = %v, want %v", what, got, want)
		}
	}
	check(a, "rail, light", railX, railY, rgb(0xe7, 0xe5, 0xe4))
	check(a, "New note, amber", btnX, btnY, rgb(0xef, 0xb0, 0x2e))

	prefs := openPreferences(a)
	prefs.Expect(telatest.ByRole("button", "System"), telatest.Pressed(true))
	prefs.Expect(telatest.ByRole("button", "Amber"), telatest.Pressed(true))
	prefs.Click(telatest.ByRole("button", "Dark"))
	prefs.Expect(telatest.ByRole("button", "Dark"), telatest.Pressed(true))
	check(a, "rail, dark", railX, railY, rgb(0x0d, 0x0d, 0x0e))
	check(a, "New note, amber in the dark scheme", btnX, btnY, rgb(0xef, 0xb0, 0x2e))
	prefs.Click(telatest.ByRole("button", "Rose"))
	prefs.Expect(telatest.ByRole("button", "Rose"), telatest.Pressed(true))
	prefs.Expect(telatest.ByRole("button", "Amber"), telatest.Pressed(false))
	check(a, "New note, rose", btnX, btnY, rgb(0xf0, 0x70, 0x8f))
	check(a, "rail, still dark", railX, railY, rgb(0x0d, 0x0d, 0x0e))

	// System follows the host again: light here.
	prefs.Click(telatest.ByRole("button", "System"))
	check(a, "rail, system", railX, railY, rgb(0xe7, 0xe5, 0xe4))
	prefs.Click(telatest.ByRole("button", "Light"))
	prefs.Click(telatest.ByRole("button", "Blue"))
	check(a, "New note, blue", btnX, btnY, rgb(0x5b, 0x9d, 0xff))

	// Both are settings: the next start has them from its first frame.
	prefs.Click(telatest.ByRole("button", "Dark"))
	a.Close()
	if err := desktop.FlushAll(); err != nil {
		t.Fatal(err)
	}
	dir, err := desktop.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if data := read(t, filepath.Join(dir, "settings.json")); !strings.Contains(data, `"theme": "dark"`) || !strings.Contains(data, `"accent": "blue"`) {
		t.Errorf("settings.json = %s, want the theme and the accent", data)
	}
	b := mount(t)
	check(b, "rail at the next start", railX, railY, rgb(0x0d, 0x0d, 0x0e))
	check(b, "New note at the next start", btnX, btnY, rgb(0x5b, 0x9d, 0xff))
}

func TestPreferencesSections(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a)
	prefs.Expect(telatest.ByRole("button", "Appearance"), telatest.Current("page"))
	prefs.ExpectRole("button", "Rose")
	for _, s := range []struct{ section, control string }{
		{"Editor", "Line spacing"},
		{"General", "Notify when a file is saved"},
		{"Shortcuts", "Preferences"},
	} {
		prefs.Click(telatest.ByRole("button", s.section))
		prefs.Expect(telatest.ByRole("button", s.section), telatest.Current("page"))
		prefs.ExpectText(s.control)
	}
	prefs.ExpectText("New note")
	prefs.ExpectText("Back to all notes, from the list")
}
