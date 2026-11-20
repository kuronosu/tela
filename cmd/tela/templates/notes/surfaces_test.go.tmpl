package main

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

var loginSwitch = telatest.ByRole("switch", "Open at login")

// Preferences has Open at login, which asks the desktop (desktop.SetStartAtLogin)
// and shows what it says.
func TestOpenAtLogin(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a, "General")
	prefs.Expect(loginSwitch, telatest.Checked(false))
	prefs.Click(loginSwitch)
	prefs.Expect(loginSwitch, telatest.Checked(true))
	if !a.StartsAtLogin() {
		t.Error("the switch did not ask the desktop to start Notes at login")
	}
	prefs.Click(loginSwitch)
	prefs.Expect(loginSwitch, telatest.Checked(false))
	if a.StartsAtLogin() {
		t.Error("the switch did not stop the start at login")
	}
}

// The switch shows the truth when Preferences opens: the user may have
// turned it on or off in the system's own settings.
func TestOpenAtLoginShowsWhatTheDesktopSays(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.SetStartsAtLogin(true)
	prefs := openPreferences(a, "General")
	prefs.Expect(loginSwitch, telatest.Checked(true))
}

func TestOpenAtLoginProblems(t *testing.T) {
	sandbox(t)
	a := mount(t)
	prefs := openPreferences(a, "General")
	a.FailSurfaces(errors.New("access to the registry was denied"))
	prefs.Click(loginSwitch)
	prefs.ExpectText("Could not change it: ")
	prefs.ExpectText("access to the registry was denied")
	prefs.Expect(loginSwitch, telatest.Checked(false)) // what is true: it did not change

	// A desktop with no such service says so, and the switch is off.
	b := mount(t)
	b.Unsupported(telatest.StartAtLogin)
	p := openPreferences(b, "General")
	p.ExpectText("This desktop cannot start Notes at login")
	p.Expect(loginSwitch, telatest.Disabled(true))
}

// A file opened or saved is a recent document of the desktop.
func TestRecentDocuments(t *testing.T) {
	sandbox(t)
	a := mount(t)
	todo := write(t, "todo.md", "Todo\n- ship it\n")
	a.Click(openBtn)
	a.AnswerFileDialog(todo)
	a.ExpectValue(titleBox, "todo")
	recent := func(path string) func() bool {
		return func() bool { return slices.Contains(a.RecentDocuments(), path) }
	}
	a.WaitFor("the opened file to be a recent document", recent(todo))

	// A dropped file too; an error does not.
	dropped := write(t, "dropped.txt", "From the file manager")
	a.DropFiles(telatest.ByRole("navigation", "Notes"), dropped)
	a.WaitFor("the dropped file to be a recent document", recent(dropped))

	// A note saved to a file.
	saved := filepath.Join(t.TempDir(), "saved.txt")
	typeNote(a, "A note for a file")
	a.Click(saveBtn)
	a.AnswerFileDialog(saved)
	a.WaitFor("the saved file to be a recent document", recent(saved))
	if got := a.RecentDocuments(); len(got) != 3 || got[0] != saved {
		t.Errorf("recent documents = %q, want the saved file first, then the dropped and the opened ones", got)
	}
}

// A desktop with no list of recent documents is no problem.
func TestRecentDocumentsUnsupported(t *testing.T) {
	sandbox(t)
	a := mount(t)
	a.Unsupported(telatest.RecentDocuments)
	a.Click(openBtn)
	a.AnswerFileDialog(write(t, "todo.md", "Todo\n"))
	a.ExpectValue(titleBox, "todo")
	a.ExpectNoText("Could not")
}

// New note is a task of the app's jump list, Dock menu or launcher; running
// one makes the note in the running Notes.
func TestTasks(t *testing.T) {
	sandbox(t)
	a := mount(t)
	var tasks []desktop.Task
	a.WaitFor("the app to set its tasks", func() bool { tasks = a.Tasks(); return len(tasks) > 0 })
	if len(tasks) != 1 || tasks[0].Title != "New note" || !slices.Equal(tasks[0].Args, []string{"tela-notes://new"}) {
		t.Fatalf("tasks = %+v, want New note with the link that makes a note", tasks)
	}
	a.ExpectCount(card("Untitled note"), 0)
	a.RunTask("New note")
	a.ExpectRole("button", "Untitled note")
	a.ExpectFocused(titleBox)
}

// Started at login, a program that keeps running in the tray stays there.
func TestStaysInTray(t *testing.T) {
	login := desktop.Launch{AtLogin: true, AtStart: true}
	for _, c := range []struct {
		l    desktop.Launch
		tray bool
		want bool
	}{
		{login, true, true},
		{login, false, false},
		{desktop.Launch{AtStart: true}, true, false},
		{desktop.Launch{Args: []string{"tela-notes://new"}}, true, false},
	} {
		if got := staysInTray(c.l, c.tray); got != c.want {
			t.Errorf("staysInTray(%+v, tray %v) = %v, want %v", c.l, c.tray, got, c.want)
		}
	}
}
