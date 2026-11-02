package assoc

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// RegisterApp, UnregisterApp and AppStatus (ADR-217) on Linux, Windows
// and the Fake, against temporary directories and a MemRegistry only.

func (e *linuxEnv) appStatus() platform.AppStatus {
	e.t.Helper()
	s, err := e.a.AppStatus(notesApp)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func TestLinuxRegisterApp(t *testing.T) {
	e := newLinuxEnv(t)
	if s := e.appStatus(); s != (platform.AppStatus{}) {
		t.Errorf("initial AppStatus = %+v", s)
	}
	changed, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{})
	if !changed || err != nil {
		t.Fatalf("RegisterApp = %v, %v", changed, err)
	}
	want := `# Written by Tela (platform.Associations): the app's entry, which RegisterApp
# asked for, and the URL schemes and file types it handles.
[Desktop Entry]
Type=Application
Version=1.5
Name=Notes
Exec="/opt/notes/notes"
Terminal=false
NoDisplay=true
X-Tela-Registered=true
X-Tela-App=true
`
	if got := e.read(e.ownEntry()); got != want {
		t.Errorf("desktop entry:\n%s\nwant:\n%s", got, want)
	}
	if s := e.appStatus(); s != (platform.AppStatus{Registered: true, Requested: true}) {
		t.Errorf("AppStatus after RegisterApp = %+v", s)
	}
	if exists(e.userMimeapps()) {
		t.Error("RegisterApp wrote mimeapps.list")
	}
	// Look before writing.
	before := e.snapshot()
	if changed, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("RegisterApp again = %v, %v", changed, err)
	}
	if !sameSnapshot(before, e.snapshot()) {
		t.Error("RegisterApp again wrote something")
	}

	// A scheme shares the file; unregistering it keeps the entry, which
	// RegisterApp asked for.
	if _, err := e.register(scheme, false); err != nil {
		t.Fatal(err)
	}
	entry := e.read(e.ownEntry())
	if !strings.Contains(entry, "MimeType=x-scheme-handler/notes;\n") || !strings.Contains(entry, "X-Tela-App=true\n") ||
		!strings.Contains(entry, "Exec=\"/opt/notes/notes\" %u\n") {
		t.Errorf("entry after Register:\n%s", entry)
	}
	if changed, err := e.a.Unregister(notesApp, scheme); !changed || err != nil {
		t.Fatalf("Unregister = %v, %v", changed, err)
	}
	entry = e.read(e.ownEntry())
	if strings.Contains(entry, "MimeType") || !strings.Contains(entry, "Exec=\"/opt/notes/notes\"\n") || !strings.Contains(entry, "X-Tela-App=true\n") {
		t.Errorf("entry after the last Unregister:\n%s", entry)
	}
	if s := e.appStatus(); !s.Registered || !s.Requested {
		t.Errorf("AppStatus after the last Unregister = %+v", s)
	}

	// UnregisterApp with an association left keeps the association.
	if _, err := e.register(scheme, false); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.a.UnregisterApp(notesApp); !changed || err != nil {
		t.Fatalf("UnregisterApp = %v, %v", changed, err)
	}
	entry = e.read(e.ownEntry())
	if strings.Contains(entry, "X-Tela-App") || !strings.Contains(entry, "MimeType=x-scheme-handler/notes;\n") {
		t.Errorf("entry after UnregisterApp with a scheme:\n%s", entry)
	}
	if s := e.status(scheme); !s.Registered {
		t.Errorf("the scheme after UnregisterApp = %+v", s)
	}
	if s := e.appStatus(); !s.Registered || s.Requested {
		t.Errorf("AppStatus after UnregisterApp with a scheme = %+v", s)
	}
	// Now the last association removes the file.
	if _, err := e.a.Unregister(notesApp, scheme); err != nil {
		t.Fatal(err)
	}
	if exists(e.ownEntry()) {
		t.Error("the entry outlived its last association after UnregisterApp")
	}
	if s := e.appStatus(); s != (platform.AppStatus{}) {
		t.Errorf("AppStatus at the end = %+v", s)
	}
	if changed, err := e.a.UnregisterApp(notesApp); changed || err != nil {
		t.Errorf("UnregisterApp of nothing = %v, %v", changed, err)
	}

	// RegisterApp then UnregisterApp alone removes the file, and the
	// cache follows.
	if _, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.a.UnregisterApp(notesApp); !changed || err != nil {
		t.Fatalf("UnregisterApp = %v, %v", changed, err)
	}
	if exists(e.ownEntry()) {
		t.Error("UnregisterApp left the entry")
	}
	if got := e.read(filepath.Join(e.dirs.DataHome, "applications", "mimeinfo.cache")); got != "[MIME Cache]\n" {
		t.Errorf("mimeinfo.cache:\n%s", got)
	}
}

func TestLinuxRegisterAppOverAssociation(t *testing.T) {
	e := newLinuxEnv(t)
	if _, err := e.register(scheme, false); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("RegisterApp over the scheme's entry = %v, %v", changed, err)
	}
	entry := e.read(e.ownEntry())
	if !strings.Contains(entry, "MimeType=x-scheme-handler/notes;\n") || !strings.Contains(entry, "X-Tela-App=true\n") {
		t.Errorf("entry:\n%s", entry)
	}
	if s := e.status(scheme); !s.Registered {
		t.Errorf("the scheme after RegisterApp = %+v", s)
	}
}

func TestLinuxRegisterAppOthersEntry(t *testing.T) {
	// A package's entry of the app's ID: the desktop knows the app, and
	// RegisterApp neither writes nor takes it over.
	e := newLinuxEnv(t)
	system := filepath.Join(e.dirs.DataDirs[0], "applications", notesApp.ID+".desktop")
	e.write(system, "[Desktop Entry]\nType=Application\nName=Notes\nExec=notes\n")
	if s := e.appStatus(); s != (platform.AppStatus{Registered: true, Other: system}) {
		t.Errorf("AppStatus beside a package's entry = %+v", s)
	}
	before := e.snapshot()
	if changed, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("RegisterApp beside a package's entry = %v, %v", changed, err)
	}
	if !sameSnapshot(before, e.snapshot()) {
		t.Error("RegisterApp wrote beside a package's entry")
	}
	if changed, err := e.a.UnregisterApp(notesApp); changed || err != nil || !exists(system) {
		t.Errorf("UnregisterApp of a package's entry = %v, %v", changed, err)
	}

	// The user hid it: taken over only with Replace.
	e.write(e.ownEntry(), "[Desktop Entry]\nType=Application\nName=Notes\nHidden=true\n")
	if s := e.appStatus(); s != (platform.AppStatus{Other: e.ownEntry()}) {
		t.Errorf("AppStatus with a hidden entry = %+v", s)
	}
	if _, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("RegisterApp over a hidden entry = %v, want ErrAssociationTaken", err)
	}
	if changed, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{Replace: true}); !changed || err != nil {
		t.Fatalf("RegisterApp with Replace = %v, %v", changed, err)
	}
	if s := e.appStatus(); s != (platform.AppStatus{Registered: true, Requested: true}) {
		t.Errorf("AppStatus after Replace = %+v", s)
	}
}

func TestLinuxRegisterAppMovedExec(t *testing.T) {
	e := newLinuxEnv(t)
	if _, err := e.a.RegisterApp(notesApp, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	moved := notesApp
	moved.Exec = "/usr/bin/notes"
	if changed, err := e.a.RegisterApp(moved, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("RegisterApp with a new Exec = %v, %v", changed, err)
	}
	if !strings.Contains(e.read(e.ownEntry()), "Exec=\"/usr/bin/notes\"\n") {
		t.Errorf("entry:\n%s", e.read(e.ownEntry()))
	}
}

func TestWindowsRegisterApp(t *testing.T) {
	r, a := winSetup(t)
	if changed, err := a.RegisterApp(winApp, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("RegisterApp = %v, %v", changed, err)
	}
	if s, err := a.AppStatus(winApp); err != nil || s != (platform.AppStatus{Registered: true}) {
		t.Errorf("AppStatus = %+v, %v", s, err)
	}
	if changed, err := a.UnregisterApp(winApp); changed || err != nil {
		t.Errorf("UnregisterApp = %v, %v", changed, err)
	}
	if keys, _ := r.SubKeys("Software"); len(keys) != 0 {
		t.Errorf("RegisterApp wrote %q", keys)
	}
}

func TestFakeRegisterApp(t *testing.T) {
	f := &Fake{}
	if s, _ := f.AppStatus(notesApp); s != (platform.AppStatus{}) {
		t.Errorf("initial AppStatus = %+v", s)
	}
	if changed, err := f.RegisterApp(notesApp, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("RegisterApp = %v, %v", changed, err)
	}
	if changed, _ := f.RegisterApp(notesApp, platform.RegisterOptions{}); changed || f.Writes() != 1 {
		t.Errorf("RegisterApp again changed %v, %d writes", changed, f.Writes())
	}
	if apps := f.Apps(); len(apps) != 1 || apps[0].ID != notesApp.ID {
		t.Errorf("Apps = %+v", apps)
	}
	if s, _ := f.AppStatus(notesApp); s != (platform.AppStatus{Registered: true, Requested: true}) {
		t.Errorf("AppStatus = %+v", s)
	}
	if changed, _ := f.UnregisterApp(notesApp); !changed || len(f.Apps()) != 0 {
		t.Errorf("UnregisterApp = %v, apps %+v", changed, f.Apps())
	}
	// An association's entry counts; another's entry is not taken over.
	if _, err := f.Register(notesApp, scheme, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if s, _ := f.AppStatus(notesApp); !s.Registered || s.Requested {
		t.Errorf("AppStatus with a scheme = %+v", s)
	}
	other := platform.HandlerApp{ID: "org.other.App"}
	f.SetAppEntry(other.ID, "/usr/share/applications/org.other.App.desktop")
	other.Name, other.Exec = "Other", "/usr/bin/other"
	if changed, err := f.RegisterApp(other, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("RegisterApp beside another's entry = %v, %v", changed, err)
	}
	if changed, err := f.RegisterApp(other, platform.RegisterOptions{Replace: true}); !changed || err != nil {
		t.Errorf("RegisterApp with Replace = %v, %v", changed, err)
	}
}

func TestUnsupportedRegisterApp(t *testing.T) {
	var a platform.Associations = platform.UnsupportedAssociations{}
	if _, err := a.RegisterApp(notesApp, platform.RegisterOptions{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("RegisterApp = %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Error("RegisterApp without Exec did not panic")
		}
	}()
	a.RegisterApp(platform.HandlerApp{ID: "x", Name: "X"}, platform.RegisterOptions{})
}
