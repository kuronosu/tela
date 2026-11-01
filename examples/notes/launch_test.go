package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

func TestParseLink(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want link
	}{
		{"notes://new", link{Action: "new"}},
		{"notes://new?title=Groceries&text=milk%20and%20eggs", link{Action: "new", Title: "Groceries", Text: "milk and eggs"}},
		{"notes://new/?text=a+b", link{Action: "new", Text: "a b"}}, // the slash a browser adds
		{"NOTES://NEW?title=%20Hi%20", link{Action: "new", Title: "Hi"}},
		{"notes:new?text=hi", link{Action: "new", Text: "hi"}}, // no slashes
		{"notes://open?id=3", link{Action: "open", ID: 3}},
		{"notes:open?id=12", link{Action: "open", ID: 12}},
		{"notes://open/?id=3", link{Action: "open", ID: 3}},
	} {
		got, err := parseLink(c.raw)
		if err != nil || got != c.want {
			t.Errorf("parseLink(%q) = %+v, %v; want %+v", c.raw, got, err, c.want)
		}
	}
	for _, raw := range []string{"https://example.com", "mailto:ana@example.com", "other://new"} {
		if _, err := parseLink(raw); !errors.Is(err, errNotOurs) {
			t.Errorf("parseLink(%q) error = %v, want errNotOurs", raw, err)
		}
	}
	for _, raw := range []string{"notes://open", "notes://open?id=x", "notes://open?id=0", "notes://delete?id=1", "notes://", "notes:"} {
		if _, err := parseLink(raw); err == nil || errors.Is(err, errNotOurs) {
			t.Errorf("parseLink(%q) error = %v, want one that says how to write it", raw, err)
		}
	}
}

// A second start with a link makes a note, and the window comes to the front.
func TestLaunchNewLink(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://new?title=Call%20Ana&text=About%20the%20roof%0Aat%20noon"}})
	a.ExpectRole("button", "Call Ana, unsaved changes")
	a.ExpectValue(titleBox, "Call Ana")
	a.ExpectValue(editor, "About the roof\nat noon")

	// Without a title the first line names it; with nothing it is a new
	// note, as Ctrl+N makes.
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://new?text=hi"}})
	a.ExpectValue(editor, "hi")
	a.InjectLaunch(desktop.Launch{Args: []string{"notes:new"}})
	a.ExpectFocused(titleBox)
	a.ExpectValue(editor, "")
}

// The note is made in the place the user is in, as New note does.
func TestLaunchNewLinkGoesToTheChosenFolder(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(railItem("Home"))
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://new?title=Rent"}})
	a.Click(railItem("Home"))
	a.ExpectCount(card("Rent"), 1)
}

func TestLaunchOpenLink(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://open?id=3"}})
	a.ExpectValue(titleBox, "Groceries")
	a.InjectLaunch(desktop.Launch{Args: []string{"notes:open?id=1"}})
	a.ExpectValue(titleBox, "Welcome to Notes")

	// A note that is not there says so.
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://open?id=99"}})
	a.ExpectText("There is no note 99")
	a.ExpectValue(titleBox, "Welcome to Notes")

	// From the overview a note opens too, and one the list would not show
	// (another folder) takes the user to all notes.
	a.Click(railItem("Ideas"))
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://open?id=3"}})
	a.ExpectValue(titleBox, "Groceries")
}

// The link a card copies is the one that opens it.
func TestCopyLink(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Click(card("Groceries"), telatest.Right())
	a.Click(telatest.ByRole("menuitem", "Copy link"))
	a.ExpectText("Link copied")
	link := a.Clipboard().Text()
	if link != "notes://open?id=3" {
		t.Fatalf("clipboard = %q, want notes://open?id=3", link)
	}
	a.Click(card("Welcome to Notes"))
	a.InjectLaunch(desktop.Launch{Args: []string{link}})
	a.ExpectValue(titleBox, "Groceries")
}

func TestLaunchBadLinks(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://delete?id=1"}})
	a.ExpectText("Could not open the link")
	a.ExpectText("notes://new?title=...&text=... and notes://open?id=3")

	a.InjectLaunch(desktop.Launch{Args: []string{"notes://open"}})
	a.ExpectText("with the number of a note")

	// A URL of another scheme and flags are not Notes's.
	before := a.Count(telatest.ByRole("button", ""))
	a.InjectLaunch(desktop.Launch{Args: []string{"--minimized", "https://example.com"}})
	if n := a.Count(telatest.ByRole("button", "")); n != before {
		t.Errorf("a launch of no concern changed the screen: %d buttons, want %d", n, before)
	}
}

// Files named on the command line open as notes: absolute, relative to the
// directory of the process that was started, and file: URIs.
func TestLaunchFiles(t *testing.T) {
	sandbox(t)
	a := mount(t)
	abs := write(t, "todo.md", "Todo\r\n- ship it")
	dir := t.TempDir()
	writeIn(t, dir, "log.txt", "Monday")
	writeIn(t, dir, "plan.rs", "fn main() {}") // not a text type, but the user named it

	a.InjectLaunch(desktop.Launch{Args: []string{abs}})
	a.ExpectValue(titleBox, "todo")
	a.ExpectValue(editor, "Todo\n- ship it")

	a.InjectLaunch(desktop.Launch{Args: []string{"log.txt"}, Dir: dir})
	a.ExpectValue(titleBox, "log")
	a.ExpectText(filepath.Join(dir, "log.txt"))

	a.InjectLaunch(desktop.Launch{Args: []string{"file:///" + strings.TrimPrefix(filepath.ToSlash(filepath.Join(dir, "plan.rs")), "/")}})
	a.ExpectValue(titleBox, "plan")

	// Two files and a link in one launch; the same file again selects.
	a.InjectLaunch(desktop.Launch{Args: []string{abs, "log.txt", "notes://new?title=Third"}, Dir: dir})
	a.ExpectCount(card("todo"), 1)
	a.ExpectCount(card("log"), 1)
	a.ExpectValue(titleBox, "Third")

	// A file that is not there is a message.
	a.InjectLaunch(desktop.Launch{Args: []string{"missing.md"}, Dir: dir})
	a.ExpectText("Could not open missing.md")
}

// A launch brings a hidden window back, with its result.
func TestLaunchShowsTheWindow(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.ClickTrayItem("Hide Notes")
	a.ExpectWindowState(telatest.Hidden(true))
	a.InjectLaunch(desktop.Launch{Args: []string{"notes://new?title=From%20a%20link"}})
	a.ExpectWindowState(telatest.Hidden(false))
	a.ExpectValue(titleBox, "From a link")
}

// writeIn makes a file in dir.
func writeIn(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := writeTextFile(filepath.Join(dir, name), content); err != nil {
		t.Fatal(err)
	}
}
