package assoc

import (
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

var (
	notesApp = platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/opt/notes/notes"}
	scheme   = platform.Association{Scheme: "notes"}
	noteFile = platform.Association{Extension: ".note", MIMEType: "application/x-notes-note", Description: "Notes document"}
)

func TestLinuxDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("XDG paths are Linux paths: \"/d\" is not absolute on Windows")
	}
	home := func() (string, error) { return "/home/u", nil }
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	d := linuxDirs(env(nil), home)
	if d.DataHome != "/home/u/.local/share" || d.ConfigHome != "/home/u/.config" ||
		!slices.Equal(d.DataDirs, []string{"/usr/local/share", "/usr/share"}) || !slices.Equal(d.ConfigDirs, []string{"/etc/xdg"}) || d.Desktops != nil {
		t.Errorf("defaults = %+v", d)
	}
	d = linuxDirs(env(map[string]string{
		"XDG_DATA_HOME":       "/d",
		"XDG_CONFIG_HOME":     "relative", // ignored: the specification requires absolute paths
		"XDG_DATA_DIRS":       "/a:rel:/b",
		"XDG_CONFIG_DIRS":     ":",
		"XDG_CURRENT_DESKTOP": "ubuntu:GNOME",
	}), home)
	if d.DataHome != "/d" || d.ConfigHome != "/home/u/.config" || !slices.Equal(d.DataDirs, []string{"/a", "/b"}) ||
		!slices.Equal(d.ConfigDirs, []string{"/etc/xdg"}) || !slices.Equal(d.Desktops, []string{"ubuntu", "gnome"}) {
		t.Errorf("from the environment = %+v", d)
	}
}

// TestOSArgumentChecks: both OS implementations check their arguments like
// every platform.Associations.
func TestOSArgumentChecks(t *testing.T) {
	for name, a := range map[string]platform.Associations{
		"linux":   Linux(LinuxDirs{DataHome: t.TempDir(), ConfigHome: t.TempDir()}),
		"windows": Windows(&MemRegistry{}),
		"macos":   MacOS(newMemLS()),
	} {
		mustPanic(t, name+" Register without Exec", func() {
			_, _ = a.Register(platform.HandlerApp{ID: "x", Name: "X"}, scheme, platform.RegisterOptions{})
		})
		mustPanic(t, name+" Status of an empty association", func() { _, _ = a.Status(notesApp, platform.Association{}) })
	}
}

func TestMemRegistry(t *testing.T) {
	r := &MemRegistry{}
	if err := r.SetString(`Software\Classes\notes\shell\open\command`, "", `"C:\notes.exe" "%1"`); err != nil {
		t.Fatal(err)
	}
	_ = r.SetString(`Software\Classes\notes`, "URL Protocol", "")
	if r.Writes() != 2 {
		t.Errorf("Writes = %d, want 2", r.Writes())
	}
	_ = r.SetString(`software\CLASSES\Notes`, "url protocol", "") // same value, other case: no write
	if r.Writes() != 2 {
		t.Errorf("an unchanged value was written (Writes = %d)", r.Writes())
	}
	if v, ok, _ := r.String(`SOFTWARE\Classes\notes\shell\open\command`, ""); !ok || v != `"C:\notes.exe" "%1"` {
		t.Errorf("String = %q, %v", v, ok)
	}
	if _, ok, _ := r.String(`Software\Classes\other`, ""); ok {
		t.Errorf("a missing key has a value")
	}
	if sub, _ := r.SubKeys(`Software\Classes`); !slices.Equal(sub, []string{"notes"}) {
		t.Errorf("SubKeys = %q", sub)
	}
	if names, _ := r.ValueNames(`Software\Classes\notes`); !slices.Equal(names, []string{"URL Protocol"}) {
		t.Errorf("ValueNames = %q", names)
	}
	_ = r.DeleteValue(`Software\Classes\notes`, "missing")
	_ = r.DeleteKey(`Software\Classes\missing`)
	if r.Writes() != 2 {
		t.Errorf("deleting what does not exist counted as a write")
	}
	_ = r.DeleteKey(`Software\Classes\NOTES`)
	if sub, _ := r.SubKeys(`Software\Classes`); len(sub) != 0 {
		t.Errorf("DeleteKey left %q", sub)
	}
	if d := r.Dump(); d != "" {
		t.Errorf("Dump after deleting everything below Classes = %q", d)
	}
}

func TestFakeRules(t *testing.T) {
	f := &Fake{}
	st := func(a platform.Association) platform.AssociationStatus {
		t.Helper()
		s, err := f.Status(notesApp, a)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	if s := st(scheme); s != (platform.AssociationStatus{}) {
		t.Errorf("initial status = %+v", s)
	}
	// A candidate, and the default while it is the only one.
	if changed, err := f.Register(notesApp, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	if s := st(scheme); !s.Registered || !s.Default {
		t.Errorf("status after Register = %+v", s)
	}
	// Look before writing.
	if changed, _ := f.Register(notesApp, scheme, platform.RegisterOptions{}); changed || f.Writes() != 1 {
		t.Errorf("registering again changed = %v, Writes = %d", changed, f.Writes())
	}
	// Another app's default is left alone and reported.
	f.SetDefault(noteFile, "org.other.Editor")
	if _, err := f.Register(notesApp, noteFile, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if s := st(noteFile); !s.Registered || s.Default || s.Other != "org.other.Editor" {
		t.Errorf("status beside another default = %+v", s)
	}
	// A conflict is refused without Replace and nothing is written.
	other := platform.Association{Scheme: "taken"}
	f.SetConflict(other, "org.other.App")
	w := f.Writes()
	if _, err := f.Register(notesApp, other, platform.RegisterOptions{}); !errors.Is(err, platform.ErrAssociationTaken) || !strings.Contains(err.Error(), "org.other.App") {
		t.Errorf("Register over a conflict = %v", err)
	}
	if f.Writes() != w || st(other).Registered {
		t.Errorf("a refused Register wrote something")
	}
	if changed, err := f.Register(notesApp, other, platform.RegisterOptions{Replace: true}); !changed || err != nil {
		t.Errorf("Register with Replace = %v, %v", changed, err)
	}
	// MakeDefault: set at once, or asked (Windows file types).
	f.SetAskForFileDefaults(true)
	if r, err := f.MakeDefault(notesApp, noteFile); r != platform.DefaultAsked || err != nil {
		t.Errorf("MakeDefault(file) = %v, %v", r, err)
	}
	if st(noteFile).Default || len(f.Asked()) != 1 {
		t.Errorf("an asked default changed the default, or was not recorded")
	}
	f.ChooseDefault(noteFile, notesApp.ID)
	if !st(noteFile).Default {
		t.Errorf("the user's choice is not the default")
	}
	if r, err := f.MakeDefault(notesApp, scheme); r != platform.DefaultSet || err != nil {
		t.Errorf("MakeDefault(scheme) = %v, %v", r, err)
	}
	// Unregister removes the app's own entries and defaults only.
	f.SetDefault(scheme, notesApp.ID)
	if changed, _ := f.Unregister(notesApp, scheme); !changed {
		t.Errorf("Unregister changed nothing")
	}
	if s := st(scheme); s.Registered || s.Default {
		t.Errorf("status after Unregister = %+v", s)
	}
	if changed, _ := f.Unregister(notesApp, scheme); changed {
		t.Errorf("Unregister of nothing changed something")
	}
	if regs := f.Registrations(); len(regs) != 2 || regs[0].Association != ".note" || regs[1].Association != "taken:" {
		t.Errorf("Registrations = %+v", regs)
	}
	// Failure injection.
	boom := errors.New("read-only home")
	f.SetError(boom)
	if _, err := f.Status(notesApp, scheme); err != boom {
		t.Errorf("Status with an injected error = %v", err)
	}
	// A failed MakeDefault did nothing: the zero DefaultResult.
	if r, err := f.MakeDefault(notesApp, scheme); err != boom || r != platform.DefaultNone {
		t.Errorf("MakeDefault with an injected error = %v, %v; want DefaultNone", r, err)
	}
	if platform.DefaultNone != 0 || platform.DefaultNone.String() != "none" {
		t.Errorf("DefaultNone is %d (%v), want the zero value", platform.DefaultNone, platform.DefaultNone)
	}
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s did not panic", what)
		}
	}()
	f()
}
