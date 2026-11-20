package remote

import (
	"errors"
	"image"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// TestSurfaces checks that the desktop surfaces of protocol 17 (ADR-298)
// are the host backend's, served off its UI thread, with the child's app
// and its errors kept.
func TestSurfaces(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	d := a.b.Desktop() // kept and used off the UI thread
	app := platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/opt/notes/notes"}

	call := func(f func() error) error {
		done := make(chan error)
		go func() { done <- f() }()
		return <-done
	}
	if err := call(func() error { return d.SetStartAtLogin(app, true) }); err != nil {
		t.Fatalf("SetStartAtLogin: %v", err)
	}
	if !p.hb.StartAtLogin(app.ID) {
		t.Error("the host's backend does not start the app at login")
	}
	p.hb.SetStartsAtLogin(app.ID, false)
	var on bool
	if err := call(func() (err error) { on, err = d.StartsAtLogin(app); return err }); err != nil || on {
		t.Errorf("StartsAtLogin = %v, %v; want the host's false", on, err)
	}

	icon := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	icon.Set(1, 1, color.NRGBA{R: 200, A: 255})
	tasks := []platform.Task{
		{Title: "New note", Args: []string{"notes:new"}, Description: "Opens\nan empty note", Icon: []image.Image{icon}},
		{Title: "Search"},
	}
	if err := call(func() error { return d.SetTasks(app, tasks) }); err != nil {
		t.Fatalf("SetTasks: %v", err)
	}
	got := p.hb.Tasks(app.ID)
	if len(got) != 2 || got[0].Title != "New note" || !slices.Equal(got[0].Args, []string{"notes:new"}) ||
		got[0].Description != tasks[0].Description || len(got[0].Icon) != 1 || got[1].Title != "Search" || got[1].Args != nil {
		t.Fatalf("the host has the tasks %+v", got)
	}
	if c := color.NRGBAModel.Convert(got[0].Icon[0].At(1, 1)).(color.NRGBA); c != (color.NRGBA{R: 200, A: 255}) {
		t.Errorf("the icon's pixel arrived as %v", c)
	}

	file := filepath.Join(t.TempDir(), "a.md")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := call(func() error { return d.AddRecentDocument(app, file) }); err != nil {
		t.Fatalf("AddRecentDocument: %v", err)
	}
	if got := p.hb.RecentDocuments(app.ID); !slices.Equal(got, []string{file}) {
		t.Errorf("the host's recent documents %v", got)
	}
	if err := call(func() error { return d.AddRecentDocument(app, file+".missing") }); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file = %v, want fs.ErrNotExist", err)
	}
	if err := d.AddRecentDocument(app, "a.md"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("a relative path = %v, want fs.ErrInvalid (checked by the child)", err)
	}
	if err := call(func() error { return d.ClearRecentDocuments(app) }); err != nil || p.hb.RecentDocuments(app.ID) != nil {
		t.Errorf("ClearRecentDocuments = %v, left %v", err, p.hb.RecentDocuments(app.ID))
	}

	p.hb.SetUnsupported(headless.ServiceTasks)
	if err := call(func() error { return d.SetTasks(app, nil) }); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("SetTasks without the service = %v, want ErrUnsupported", err)
	}
	p.hb.SetUnsupported()
	boom := errors.New("access denied")
	p.hb.SetSurfaceError(boom)
	if err := call(func() error { return d.SetStartAtLogin(app, false) }); err == nil || err.Error() != boom.Error() {
		t.Errorf("SetStartAtLogin with an injected error = %v", err)
	}
}
