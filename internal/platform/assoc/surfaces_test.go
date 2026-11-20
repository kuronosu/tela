package assoc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The Linux files of the desktop surfaces (ADR-298): the autostart entry
// and the actions of the app's desktop entry, against temporary
// directories only.

func (e *linuxEnv) autostart() string {
	return filepath.Join(e.dirs.ConfigHome, "autostart", notesApp.ID+".desktop")
}

func (e *linuxEnv) startsAtLogin() bool {
	e.t.Helper()
	on, err := LinuxStartsAtLogin(e.dirs, notesApp)
	if err != nil {
		e.t.Fatal(err)
	}
	return on
}

func (e *linuxEnv) setStartAtLogin(app platform.HandlerApp, on bool) bool {
	e.t.Helper()
	changed, err := LinuxSetStartAtLogin(e.dirs, app, on)
	if err != nil {
		e.t.Fatal(err)
	}
	return changed
}

func TestLinuxAutostartEntry(t *testing.T) {
	e := newLinuxEnv(t)
	if e.startsAtLogin() {
		t.Error("StartsAtLogin before anything was written")
	}
	app := notesApp
	app.Args = []string{"--profile", "work space"}
	if !e.setStartAtLogin(app, true) {
		t.Fatal("SetStartAtLogin(true) wrote nothing")
	}
	want := `# Written by Tela (desktop.SetStartAtLogin): starts the app when the user
# logs in. desktop.SetStartAtLogin(false) removes it.
[Desktop Entry]
Type=Application
Version=1.5
Name=Notes
Exec=/opt/notes/notes --profile "work space" -tela-at-login
Terminal=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
X-Tela-Registered=true
`
	if got := e.read(e.autostart()); got != want {
		t.Errorf("autostart entry:\n%s\nwant:\n%s", got, want)
	}
	if !e.startsAtLogin() {
		t.Error("StartsAtLogin after SetStartAtLogin(true) is false")
	}
	before := e.snapshot()
	if e.setStartAtLogin(app, true) {
		t.Error("SetStartAtLogin(true) again reported a change")
	}
	if !sameSnapshot(before, e.snapshot()) {
		t.Error("SetStartAtLogin(true) again wrote something")
	}
	// A moved executable rewrites it.
	moved := app
	moved.Exec = "/usr/bin/notes"
	if !e.setStartAtLogin(moved, true) || !strings.Contains(e.read(e.autostart()), "Exec=/usr/bin/notes ") {
		t.Error("a moved executable did not rewrite the entry")
	}
	if !e.setStartAtLogin(notesApp, false) || exists(e.autostart()) {
		t.Error("SetStartAtLogin(false) left Tela's entry")
	}
	if e.startsAtLogin() || e.setStartAtLogin(notesApp, false) {
		t.Error("off twice: still on, or a second change")
	}
}

func TestLinuxAutostartUserSwitches(t *testing.T) {
	e := newLinuxEnv(t)
	e.setStartAtLogin(notesApp, true)
	// The user turns it off in the desktop's settings.
	e.write(e.autostart(), strings.Replace(e.read(e.autostart()), "X-GNOME-Autostart-enabled=true", "X-GNOME-Autostart-enabled=false", 1))
	if e.startsAtLogin() {
		t.Error("an entry with X-GNOME-Autostart-enabled=false counts as on")
	}
	if !e.setStartAtLogin(notesApp, true) || !e.startsAtLogin() {
		t.Error("SetStartAtLogin(true) did not turn the user's switch on again")
	}

	// Someone else's entry of the ID: left alone while it starts the app,
	// turned off with Hidden=true and on again keeping the rest.
	own := "[Desktop Entry]\nType=Application\nName=Notes (mine)\nExec=notes --quiet\n# my comment\n"
	e.write(e.autostart(), own)
	if e.setStartAtLogin(notesApp, true) || e.read(e.autostart()) != own {
		t.Error("SetStartAtLogin(true) touched an entry that starts the app already")
	}
	if !e.setStartAtLogin(notesApp, false) {
		t.Fatal("SetStartAtLogin(false) left the user's entry on")
	}
	if got := e.read(e.autostart()); got != "[Desktop Entry]\nType=Application\nName=Notes (mine)\nExec=notes --quiet\nHidden=true\n# my comment\n" {
		t.Errorf("the user's entry turned off:\n%s", got)
	}
	if e.startsAtLogin() {
		t.Error("Hidden=true counts as on")
	}
	if !e.setStartAtLogin(notesApp, true) || e.read(e.autostart()) != own {
		t.Errorf("the user's entry turned on again:\n%s", e.read(e.autostart()))
	}
}

func TestLinuxAutostartSystemEntry(t *testing.T) {
	e := newLinuxEnv(t)
	system := filepath.Join(e.dirs.ConfigDirs[0], "autostart", notesApp.ID+".desktop")
	e.write(system, "[Desktop Entry]\nType=Application\nName=Notes\nExec=/usr/bin/notes\n")
	if !e.startsAtLogin() {
		t.Error("a system autostart entry counts as off")
	}
	if e.setStartAtLogin(notesApp, true) || exists(e.autostart()) {
		t.Error("SetStartAtLogin(true) wrote over a system entry that starts the app")
	}
	if !e.setStartAtLogin(notesApp, false) {
		t.Fatal("SetStartAtLogin(false) did not shadow the system entry")
	}
	if got := e.read(e.autostart()); !strings.Contains(got, "\nHidden=true\n") || !strings.Contains(got, "\nX-Tela-Registered=true\n") {
		t.Errorf("the override:\n%s", got)
	}
	if e.startsAtLogin() {
		t.Error("the override counts as on")
	}
	if !e.setStartAtLogin(notesApp, true) || !strings.Contains(e.read(e.autostart()), "-tela-at-login") || !e.startsAtLogin() {
		t.Error("SetStartAtLogin(true) did not replace the override with Tela's entry")
	}
	if !e.setStartAtLogin(notesApp, false) || !strings.Contains(e.read(e.autostart()), "Hidden=true") {
		t.Error("off again: the system entry is not shadowed")
	}
}

func TestLinuxSetActions(t *testing.T) {
	e := newLinuxEnv(t)
	tasks := []platform.Task{
		{Title: "New note", Args: []string{"notes:new"}},
		{Title: "Search; all", Args: []string{"--search", "two words"}},
	}
	// No entry: the documented error, and nothing written.
	if _, err := LinuxSetActions(e.dirs, notesApp.ID, tasks, nil); !errors.Is(err, platform.ErrAppNotRegistered) {
		t.Errorf("SetActions without an entry = %v, want ErrAppNotRegistered", err)
	}
	if changed, err := LinuxSetActions(e.dirs, notesApp.ID, nil, nil); changed || err != nil {
		t.Errorf("no tasks without an entry = %v, %v", changed, err)
	}
	if _, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	changed, err := LinuxSetActions(e.dirs, notesApp.ID, tasks, []string{"/home/a/.local/share/tela/tasks/com.example.Notes/1.png", ""})
	if !changed || err != nil {
		t.Fatalf("SetActions = %v, %v", changed, err)
	}
	want := `# Written by Tela (platform.Associations): the app's entry, which RegisterApp
# asked for, and the URL schemes and file types it handles.
[Desktop Entry]
Type=Application
Version=1.5
Name=Notes
Exec=/opt/notes/notes
Terminal=false
NoDisplay=true
X-Tela-Registered=true
X-Tela-App=true
Actions=tela-1;tela-2;

[Desktop Action tela-1]
Name=New note
Exec=/opt/notes/notes notes:new
Icon=/home/a/.local/share/tela/tasks/com.example.Notes/1.png

[Desktop Action tela-2]
Name=Search; all
Exec=/opt/notes/notes --search "two words"
`
	if got := e.read(e.ownEntry()); got != want {
		t.Errorf("entry:\n%s\nwant:\n%s", got, want)
	}
	before := e.snapshot()
	if changed, _ := LinuxSetActions(e.dirs, notesApp.ID, tasks, []string{"/home/a/.local/share/tela/tasks/com.example.Notes/1.png"}); changed || !sameSnapshot(before, e.snapshot()) {
		t.Error("the same tasks again wrote something")
	}
	// RegisterApp and the associations render the entry again: the
	// actions stay, after the field code.
	if _, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.register(scheme, false); err != nil {
		t.Fatal(err)
	}
	got := e.read(e.ownEntry())
	for _, s := range []string{"Exec=/opt/notes/notes %u\n", "MimeType=x-scheme-handler/notes;\n", "Actions=tela-1;tela-2;\n", "[Desktop Action tela-2]\nName=Search; all\nExec=/opt/notes/notes --search \"two words\"\n"} {
		if !strings.Contains(got, s) {
			t.Errorf("after Register the entry lacks %q:\n%s", s, got)
		}
	}
	if strings.Count(got, "[Desktop Action tela-1]") != 1 {
		t.Errorf("the actions were doubled:\n%s", got)
	}
	// A task's Exec has no field code even when the entry's has one.
	if _, err := LinuxSetActions(e.dirs, notesApp.ID, tasks[:1], nil); err != nil {
		t.Fatal(err)
	}
	got = e.read(e.ownEntry())
	if !strings.Contains(got, "Actions=tela-1;\n") || strings.Contains(got, "tela-2") || !strings.Contains(got, "[Desktop Action tela-1]\nName=New note\nExec=/opt/notes/notes notes:new\n") {
		t.Errorf("one task:\n%s", got)
	}
	// No tasks: the actions go.
	if changed, err := LinuxSetActions(e.dirs, notesApp.ID, nil, nil); !changed || err != nil {
		t.Errorf("no tasks = %v, %v", changed, err)
	}
	if got := e.read(e.ownEntry()); strings.Contains(got, "Action") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("no tasks left:\n%s", got)
	}
}

func TestLinuxSetActionsInstallerEntry(t *testing.T) {
	e := newLinuxEnv(t)
	installed := filepath.Join(e.dirs.DataDirs[0], "applications", notesApp.ID+".desktop")
	content := "[Desktop Entry]\nType=Application\nName=Notes\nExec=/usr/bin/notes %U\n"
	e.write(installed, content)
	_, err := LinuxSetActions(e.dirs, notesApp.ID, []platform.Task{{Title: "New note"}}, nil)
	if !errors.Is(err, platform.ErrAppNotRegistered) || !strings.Contains(err.Error(), installed) {
		t.Errorf("SetActions on an installer's entry = %v, want ErrAppNotRegistered naming it", err)
	}
	if e.read(installed) != content || exists(e.ownEntry()) {
		t.Error("an installer's entry was edited or shadowed")
	}
	if b, _ := os.ReadFile(installed); string(b) != content {
		t.Error("changed")
	}
}
