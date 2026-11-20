//go:build linux

package freedesktop

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The desktop surfaces (ADR-298) of the Linux backends, on temporary XDG
// directories only: never the session's autostart folder, desktop
// entries or recent files.

func surfaceDesktop(t *testing.T) (*Desktop, assoc.LinuxDirs) {
	t.Helper()
	root := t.TempDir()
	dirs := assoc.LinuxDirs{
		DataHome:   filepath.Join(root, "data"),
		ConfigHome: filepath.Join(root, "config"),
		DataDirs:   []string{filepath.Join(root, "usr")},
		ConfigDirs: []string{filepath.Join(root, "etc")},
		Run:        func(string, ...string) error { return nil },
	}
	old, oldFlatpak := surfaceDirs, inFlatpak
	surfaceDirs = func() assoc.LinuxDirs { return dirs }
	inFlatpak = func() bool { return false }
	t.Cleanup(func() { surfaceDirs, inFlatpak = old, oldFlatpak })
	d := New(Options{Wake: func() {}, Post: func(func()) {}, Event: func(platform.Event) {}, Address: "unix:path=/nonexistent"})
	t.Cleanup(d.Close)
	return d, dirs
}

var surfApp = platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/opt/notes/notes"}

func TestSurfacesLinux(t *testing.T) {
	d, dirs := surfaceDesktop(t)

	if err := d.SetStartAtLogin(surfApp, true); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(dirs.ConfigHome, "autostart", "com.example.Notes.desktop"))
	if err != nil || !strings.Contains(string(entry), "Exec=/opt/notes/notes -tela-at-login\n") {
		t.Errorf("autostart entry %q, %v", entry, err)
	}
	if on, err := d.StartsAtLogin(surfApp); !on || err != nil {
		t.Errorf("StartsAtLogin = %v, %v", on, err)
	}

	// Tasks need the entry RegisterApp writes.
	icon := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	icon.Set(3, 3, color.NRGBA{G: 255, A: 255})
	tasks := []platform.Task{{Title: "New note", Args: []string{"notes:new"}, Icon: []image.Image{icon}}, {Title: "Search"}}
	if err := d.SetTasks(surfApp, tasks); !errors.Is(err, platform.ErrAppNotRegistered) {
		t.Errorf("SetTasks without the app's entry = %v, want ErrAppNotRegistered", err)
	}
	if _, err := assoc.Linux(dirs).RegisterApp(surfApp, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetTasks(surfApp, tasks); err != nil {
		t.Fatal(err)
	}
	iconPath := filepath.Join(dirs.DataHome, "tela", "tasks", "com.example.Notes", "1.png")
	raw, err := os.ReadFile(filepath.Join(dirs.DataHome, "applications", "com.example.Notes.desktop"))
	if err != nil || !strings.Contains(string(raw), "[Desktop Action tela-1]\nName=New note\nExec=/opt/notes/notes notes:new\nIcon="+iconPath+"\n") {
		t.Errorf("entry:\n%s (%v)", raw, err)
	}
	f, err := os.Open(iconPath)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil || color.NRGBAModel.Convert(img.At(3, 3)) != (color.NRGBA{G: 255, A: 255}) {
		t.Errorf("the icon file: %v", err)
	}
	// Without an icon the old file goes.
	if err := d.SetTasks(surfApp, []platform.Task{{Title: "Search"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(iconPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a stale icon stayed (%v)", err)
	}

	// Recent documents.
	file := filepath.Join(t.TempDir(), "list.md")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := d.AddRecentDocument(surfApp, file); err != nil {
		t.Fatal(err)
	}
	if got, err := assoc.LinuxRecent(dirs, surfApp.ID); err != nil || !slices.Equal(got, []string{file}) {
		t.Errorf("recent = %v, %v", got, err)
	}
	if err := d.AddRecentDocument(surfApp, file+".missing"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a missing file = %v", err)
	}
	if err := d.ClearRecentDocuments(surfApp); err != nil {
		t.Fatal(err)
	}
	if got, _ := assoc.LinuxRecent(dirs, surfApp.ID); len(got) != 0 {
		t.Errorf("recent after clearing = %v", got)
	}
}

func TestBackgroundOutcome(t *testing.T) {
	granted := map[string]dbus.Variant{"background": dbus.MakeVariant(true), "autostart": dbus.MakeVariant(true)}
	denied := map[string]dbus.Variant{"background": dbus.MakeVariant(true), "autostart": dbus.MakeVariant(false)}
	for _, c := range []struct {
		on      bool
		code    uint32
		results map[string]dbus.Variant
		ok      bool
	}{
		{true, 0, granted, true},
		{true, 0, denied, false},
		{false, 0, denied, true},
		{true, 1, nil, false},
		{false, 2, nil, false},
	} {
		if err := backgroundOutcome(c.on, c.code, c.results); (err == nil) != c.ok {
			t.Errorf("backgroundOutcome(%v, %d, %v) = %v", c.on, c.code, c.results, err)
		}
	}
}
