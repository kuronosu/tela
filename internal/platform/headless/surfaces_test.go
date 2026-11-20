package headless

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The fake desktop surfaces of contract v16 (ADR-298).

var surfApp = platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/opt/notes"}

func TestSurfacesFake(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()

	// Start at login, and the user's switch.
	if on, err := d.StartsAtLogin(surfApp); on || err != nil {
		t.Errorf("StartsAtLogin = %v, %v", on, err)
	}
	if err := d.SetStartAtLogin(surfApp, true); err != nil {
		t.Fatal(err)
	}
	if on, _ := d.StartsAtLogin(surfApp); !on || !b.StartAtLogin(surfApp.ID) {
		t.Error("not on after SetStartAtLogin(true)")
	}
	b.SetStartsAtLogin(surfApp.ID, false)
	if on, _ := d.StartsAtLogin(surfApp); on {
		t.Error("on after the user turned it off")
	}

	// Tasks are copied, nil removes them.
	tasks := []platform.Task{{Title: "New note", Args: []string{"notes:new"}}}
	if err := d.SetTasks(surfApp, tasks); err != nil {
		t.Fatal(err)
	}
	tasks[0].Args[0] = "changed"
	if got := b.Tasks(surfApp.ID); len(got) != 1 || got[0].Args[0] != "notes:new" {
		t.Errorf("Tasks = %+v", got)
	}
	if err := d.SetTasks(surfApp, nil); err != nil || b.Tasks(surfApp.ID) != nil {
		t.Errorf("SetTasks(nil) = %v, left %+v", err, b.Tasks(surfApp.ID))
	}

	// Recent documents: most recent first, a file again moves up, a
	// missing one is an error, clearing empties the app's list only.
	dir := t.TempDir()
	a, c := filepath.Join(dir, "a.md"), filepath.Join(dir, "c.md")
	for _, p := range []string{a, c} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{a, c, a} {
		if err := d.AddRecentDocument(surfApp, p); err != nil {
			t.Fatal(err)
		}
	}
	other := platform.HandlerApp{ID: "com.example.Other", Name: "Other", Exec: "/opt/other"}
	_ = d.AddRecentDocument(other, c)
	if got := b.RecentDocuments(surfApp.ID); !slices.Equal(got, []string{a, c}) {
		t.Errorf("RecentDocuments = %v", got)
	}
	if err := d.AddRecentDocument(surfApp, filepath.Join(dir, "missing.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file = %v", err)
	}
	if err := d.ClearRecentDocuments(surfApp); err != nil || b.RecentDocuments(surfApp.ID) != nil || len(b.RecentDocuments(other.ID)) != 1 {
		t.Errorf("ClearRecentDocuments = %v: %v, %v", err, b.RecentDocuments(surfApp.ID), b.RecentDocuments(other.ID))
	}

	// Failures and missing services record nothing.
	boom := errors.New("access denied")
	b.SetSurfaceError(boom)
	if err := d.SetTasks(surfApp, tasks); !errors.Is(err, boom) || b.Tasks(surfApp.ID) != nil {
		t.Errorf("SetTasks with an error = %v", err)
	}
	b.SetSurfaceError(nil)
	b.SetUnsupported(ServiceStartAtLogin, ServiceTasks, ServiceRecentDocuments)
	if err := d.SetStartAtLogin(surfApp, true); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("unsupported SetStartAtLogin = %v", err)
	}
	if err := d.AddRecentDocument(surfApp, a); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("unsupported AddRecentDocument = %v", err)
	}
	if err := d.AddRecentDocument(surfApp, "a.md"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("a relative path where unsupported = %v, want fs.ErrInvalid", err)
	}
}
