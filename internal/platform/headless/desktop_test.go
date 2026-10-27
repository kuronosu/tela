package headless

import (
	"errors"
	"fmt"
	"image"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

var trayIcon = []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}

func TestDialogResponder(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	b.SetDialogResponder(func(d Dialog) DialogResponse {
		switch d.Kind {
		case DialogOpenFiles:
			return DialogResponse{Paths: []string{"/a.png", "/b.png"}}
		case DialogMessageBox:
			return DialogResponse{Answer: platform.AnswerNo}
		}
		return DialogResponse{} // canceled
	})
	d := b.Desktop()
	var got []string
	opts := platform.FileDialogOptions{Parent: w, Title: "Open", Multiple: true,
		Filters: []platform.FileFilter{{Name: "Images", Patterns: []string{"*.png"}}}}
	if err := d.OpenFiles(opts, func(paths []string, err error) { got = append(got, fmt.Sprint(paths, err)) }); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveFile(platform.FileDialogOptions{Name: "x.txt"}, func(path string, err error) {
		got = append(got, fmt.Sprintf("save %q %v", path, err))
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.MessageBox(platform.MessageBoxOptions{Kind: platform.MessageQuestion, Text: "Save?", Buttons: platform.ButtonsYesNo},
		func(a platform.MessageAnswer, err error) { got = append(got, fmt.Sprint(a, err)) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a callback ran inside its request: %v", got)
	}
	b.Drain(&recorder{})
	if want := `[[/a.png /b.png] <nil> save "" <nil> no <nil>]`; fmt.Sprint(got) != want {
		t.Errorf("answers = %s, want %s", fmt.Sprint(got), want)
	}
	ds := b.Dialogs()
	if len(ds) != 3 || ds[0].ID != 1 || ds[0].File.Parent != w || ds[1].File.Name != "x.txt" || ds[2].Message.Text != "Save?" {
		t.Errorf("Dialogs = %+v", ds)
	}
	if len(b.OpenDialogs()) != 0 {
		t.Errorf("answered dialogs are still open")
	}
}

func TestDialogsStayOpenUntilAnswered(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	var got []string
	_ = d.PickFolder(platform.FileDialogOptions{}, func(p string, err error) { got = append(got, p) })
	_ = d.SaveFile(platform.FileDialogOptions{}, func(p string, err error) { got = append(got, p) })
	b.Drain(&recorder{})
	open := b.OpenDialogs()
	if len(open) != 2 || len(got) != 0 {
		t.Fatalf("OpenDialogs = %+v, answers %v", open, got)
	}
	expectPanic(t, "only an OpenFiles dialog with Multiple", func() {
		b.AnswerDialog(open[1].ID, DialogResponse{Paths: []string{"/a", "/b"}})
	})
	b.AnswerDialog(open[1].ID, DialogResponse{Paths: []string{"/out.txt"}})
	b.AnswerDialog(open[0].ID, DialogResponse{}) // canceled
	b.Drain(&recorder{})
	if fmt.Sprint(got) != `[/out.txt ]` {
		t.Errorf("answers = %q", got)
	}
	expectPanic(t, "no such open dialog", func() { b.AnswerDialog(open[0].ID, DialogResponse{}) })
	failed := errors.New("portal gone")
	var gotErr error
	_ = d.OpenFiles(platform.FileDialogOptions{}, func(_ []string, err error) { gotErr = err })
	b.AnswerDialog(3, DialogResponse{Err: failed})
	b.Drain(&recorder{})
	if gotErr != failed {
		t.Errorf("error = %v", gotErr)
	}
}

func TestMessageBoxAnswersAndUnsupported(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	_ = d.MessageBox(platform.MessageBoxOptions{Buttons: platform.ButtonsOKCancel}, func(platform.MessageAnswer, error) {})
	expectPanic(t, "not a button", func() { b.AnswerDialog(1, DialogResponse{Answer: platform.AnswerYes}) })
	b.SetUnsupported(ServiceMessageBox)
	called := false
	err := d.MessageBox(platform.MessageBoxOptions{}, func(platform.MessageAnswer, error) { called = true })
	if !errors.Is(err, platform.ErrUnsupported) || !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("MessageBox on an unsupported backend = %v", err)
	}
	b.Drain(&recorder{})
	if called || len(b.Dialogs()) != 1 {
		t.Errorf("an unsupported request was recorded or answered")
	}
	expectPanic(t, "invalid Kind", func() { _ = d.MessageBox(platform.MessageBoxOptions{Kind: 9}, func(platform.MessageAnswer, error) {}) })
	b.SetUnsupported()
	if err := d.MessageBox(platform.MessageBoxOptions{}, func(platform.MessageAnswer, error) {}); err != nil {
		t.Errorf("SetUnsupported() did not restore the service: %v", err)
	}
}

func TestOpenURIAndPath(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	if err := d.OpenURI(nil, "https://example.com/a?b"); err != nil {
		t.Fatal(err)
	}
	if err := d.OpenPath(nil, "notes.txt"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"example.com", "file:///etc/hosts"} {
		if d.OpenURI(nil, bad) == nil {
			t.Errorf("OpenURI(%q) accepted", bad)
		}
	}
	if d.OpenPath(nil, "") == nil {
		t.Errorf("OpenPath(\"\") accepted")
	}
	b.SetOpenResponder(func(r OpenRequest) error { return errors.New("no handler") })
	if d.OpenURI(nil, "x-unknown:thing") == nil {
		t.Errorf("the open responder's error was not returned")
	}
	got := b.Opened()
	if len(got) != 2 || got[0].Target != "https://example.com/a?b" || got[0].Path || !got[1].Path {
		t.Errorf("Opened = %+v", got)
	}
	b.SetUnsupported(ServiceOpen)
	b.SetOpenResponder(nil)
	if err := d.OpenPath(nil, "a"); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("OpenPath on an unsupported backend = %v", err)
	}
}

func TestNotifications(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	id, err := d.Notify(platform.Notification{Title: "Saved", Body: "notes.txt", Icon: trayIcon,
		Actions: []platform.NotificationAction{{ID: "open", Label: "Open"}}})
	if err != nil || id != 1 {
		t.Fatalf("Notify = %d, %v", id, err)
	}
	id2, _ := d.Notify(platform.Notification{Title: "Second"})
	r := &recorder{}
	b.InjectApp(platform.NotificationEvent{ID: id, Kind: platform.NotificationActionInvoked, Action: "open"})
	b.InjectApp(platform.NotificationEvent{ID: id2, Kind: platform.NotificationActivated})
	b.InjectApp(platform.NotificationEvent{ID: id2, Kind: platform.NotificationClosed})
	expectEvents(t, b, r, "app:notification(1 action open) app:notification(2 activated ) app:notification(2 closed )")
	expectPanic(t, "has no action", func() {
		b.InjectApp(platform.NotificationEvent{ID: id, Kind: platform.NotificationActionInvoked, Action: "delete"})
	})
	expectPanic(t, "is not open", func() { b.InjectApp(platform.NotificationEvent{ID: id2}) })
	_ = d.CloseNotification(id)
	_ = d.CloseNotification(99)
	ns := b.Notifications()
	if len(ns) != 2 || !ns[0].Closed || !ns[1].Closed || ns[0].Notification.Title != "Saved" || len(ns[0].Notification.Icon) != 1 {
		t.Errorf("Notifications = %+v", ns)
	}
	expectPanic(t, "default", func() {
		_, _ = d.Notify(platform.Notification{Actions: []platform.NotificationAction{{ID: "default"}}})
	})
}

func TestTrays(t *testing.T) {
	b := New(Options{})
	d := b.Desktop()
	menu := []platform.MenuItem{
		{ID: 1, Label: "Show"},
		{Separator: true},
		{ID: 2, Label: "Dark", Checkable: true},
		{Label: "More", Submenu: []platform.MenuItem{{ID: 3, Label: "About"}, {ID: 4, Label: "Off", Disabled: true}}},
	}
	id, err := d.NewTray(platform.TrayOptions{Icon: trayIcon, Tooltip: "Notes", Menu: menu})
	if err != nil || id != 1 {
		t.Fatalf("NewTray = %d, %v", id, err)
	}
	menu[0].Label = "changed" // the backend kept a copy
	r := &recorder{}
	b.InjectApp(platform.TrayEvent{Tray: id, Kind: platform.TrayActivated})
	b.InjectApp(platform.TrayEvent{Tray: id, Kind: platform.TrayMenuItem, Item: 3})
	expectEvents(t, b, r, "app:tray(1 activated 0) app:tray(1 menu-item 3)")
	expectPanic(t, "disabled", func() { b.InjectApp(platform.TrayEvent{Tray: id, Kind: platform.TrayMenuItem, Item: 4}) })
	expectPanic(t, "no menu item", func() { b.InjectApp(platform.TrayEvent{Tray: id, Kind: platform.TrayMenuItem, Item: 9}) })
	trays := b.Trays()
	if len(trays) != 1 || trays[0].Options.Menu[0].Label != "Show" || trays[0].Options.Tooltip != "Notes" {
		t.Errorf("Trays = %+v", trays)
	}
	if err := d.UpdateTray(id, platform.TrayOptions{Icon: trayIcon, Tooltip: "Notes (2)"}); err != nil {
		t.Fatal(err)
	}
	if tr := b.Trays()[0]; tr.Updates != 1 || tr.Options.Tooltip != "Notes (2)" {
		t.Errorf("after UpdateTray: %+v", tr)
	}
	if err := d.UpdateTray(7, platform.TrayOptions{Icon: trayIcon}); err == nil {
		t.Errorf("UpdateTray of an unknown tray succeeded")
	}
	expectPanic(t, "reuses ID", func() {
		_, _ = d.NewTray(platform.TrayOptions{Icon: trayIcon, Menu: []platform.MenuItem{{ID: 1}, {ID: 1}}})
	})
	if err := d.RemoveTray(id); err != nil || len(b.Trays()) != 0 {
		t.Errorf("RemoveTray: %v, %d trays left", err, len(b.Trays()))
	}
	expectPanic(t, "does not exist", func() { b.InjectApp(platform.TrayEvent{Tray: id}) })
	expectPanic(t, "not an app-scoped", func() { b.InjectApp(platform.FocusEvent{}) })

	// Trays go away when Run returns.
	if _, err := d.NewTray(platform.TrayOptions{Icon: trayIcon}); err != nil {
		t.Fatal(err)
	}
	b.Quit()
	if err := b.Run(r); err != nil {
		t.Fatal(err)
	}
	if len(b.Trays()) != 0 {
		t.Errorf("a tray survived Run")
	}
}
