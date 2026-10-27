package platform

import (
	"errors"
	"image"
	"testing"
)

func TestErrUnsupported(t *testing.T) {
	if !errors.Is(ErrUnsupported, errors.ErrUnsupported) {
		t.Errorf("ErrUnsupported does not wrap errors.ErrUnsupported")
	}
}

func TestCheckURI(t *testing.T) {
	for _, ok := range []string{"https://example.com", "mailto:a@b.c", "x-scheme:thing"} {
		if err := CheckURI(ok); err != nil {
			t.Errorf("CheckURI(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "example.com", "/tmp/x", "file:///tmp/x", "FILE:///x", "%zz"} {
		if CheckURI(bad) == nil {
			t.Errorf("CheckURI(%q) accepted", bad)
		}
	}
}

func TestMessageButtons(t *testing.T) {
	for _, c := range []struct {
		b       MessageButtons
		has     []MessageAnswer
		dismiss MessageAnswer
	}{
		{ButtonsOK, []MessageAnswer{AnswerOK}, AnswerOK},
		{ButtonsOKCancel, []MessageAnswer{AnswerOK, AnswerCancel}, AnswerCancel},
		{ButtonsYesNo, []MessageAnswer{AnswerYes, AnswerNo}, AnswerNo},
		{ButtonsYesNoCancel, []MessageAnswer{AnswerYes, AnswerNo, AnswerCancel}, AnswerCancel},
	} {
		n := 0
		for a := AnswerOK; a <= AnswerNo; a++ {
			if c.b.Has(a) {
				n++
			}
		}
		if n != len(c.has) {
			t.Errorf("buttons %d have %d answers, want %d", c.b, n, len(c.has))
		}
		for _, a := range c.has {
			if !c.b.Has(a) {
				t.Errorf("buttons %d lack %v", c.b, a)
			}
		}
		if got := c.b.DismissAnswer(); got != c.dismiss || !c.b.Has(got) {
			t.Errorf("buttons %d dismiss as %v, want %v", c.b, got, c.dismiss)
		}
	}
}

func TestDesktopChecks(t *testing.T) {
	CheckFileDialogOptions("x", FileDialogOptions{Filters: []FileFilter{{Name: "All", Patterns: []string{"*"}}}})
	expectPanic(t, "no patterns", func() { CheckFileDialogOptions("x", FileDialogOptions{Filters: []FileFilter{{Name: "a"}}}) })
	expectPanic(t, "invalid pattern", func() {
		CheckFileDialogOptions("x", FileDialogOptions{Filters: []FileFilter{{Patterns: []string{""}}}})
	})
	expectPanic(t, "invalid Buttons", func() { CheckMessageBoxOptions(MessageBoxOptions{Buttons: 4}) })
	expectPanic(t, "twice", func() {
		CheckNotification(Notification{Actions: []NotificationAction{{ID: "a"}, {ID: "a"}}})
	})
	expectPanic(t, "non-empty", func() { CheckNotification(Notification{Actions: []NotificationAction{{}}}) })
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	menu := []MenuItem{
		{ID: 1, Label: "a"},
		{Separator: true},
		{Label: "sub", Submenu: []MenuItem{{ID: 2, Label: "b"}, {Label: "subsub", Submenu: []MenuItem{{ID: 3}}}}},
	}
	CheckTrayOptions(TrayOptions{Icon: icon, Menu: menu})
	if it, ok := FindMenuItem(menu, 3); !ok || it.ID != 3 {
		t.Errorf("FindMenuItem(3) = %+v, %v", it, ok)
	}
	if _, ok := FindMenuItem(menu, 0); ok {
		t.Errorf("FindMenuItem found ID 0 (a separator or submenu)")
	}
	expectPanic(t, "Icon is empty", func() { CheckTrayOptions(TrayOptions{}) })
	expectPanic(t, "Menu[2].Submenu[1].Submenu[0]", func() {
		CheckTrayOptions(TrayOptions{Icon: icon, Menu: []MenuItem{{ID: 1}, {Separator: true},
			{Submenu: []MenuItem{{ID: 2}, {Submenu: []MenuItem{{ID: 1}}}}}}})
	})
}

func TestUnsupportedDesktop(t *testing.T) {
	var d Desktop = UnsupportedDesktop{}
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	errs := []error{
		d.OpenURI(nil, "https://x"),
		d.OpenPath(nil, "x"),
		d.OpenFiles(FileDialogOptions{}, func([]string, error) {}),
		d.SaveFile(FileDialogOptions{}, func(string, error) {}),
		d.PickFolder(FileDialogOptions{}, func(string, error) {}),
		d.MessageBox(MessageBoxOptions{}, func(MessageAnswer, error) {}),
		d.CloseNotification(1),
		d.UpdateTray(1, TrayOptions{Icon: icon}),
		d.RemoveTray(1),
	}
	_, err := d.Notify(Notification{})
	errs = append(errs, err)
	_, err = d.NewTray(TrayOptions{Icon: icon})
	errs = append(errs, err)
	for i, err := range errs {
		if !errors.Is(err, ErrUnsupported) {
			t.Errorf("call %d: %v, want ErrUnsupported", i, err)
		}
	}
	// Arguments are checked even without the service.
	expectPanic(t, "nil callback", func() { _ = d.PickFolder(FileDialogOptions{}, nil) })
	expectPanic(t, "Icon is empty", func() { _, _ = d.NewTray(TrayOptions{}) })
}

func TestDesktopEventEnums(t *testing.T) {
	if NotificationActionInvoked.String() != "action" || TrayMenuItem.String() != "menu-item" || AnswerCancel.String() != "cancel" {
		t.Errorf("enum names")
	}
	var _ Event = WindowStateEvent{}
	var _ Event = NotificationEvent{}
	var _ Event = TrayEvent{}
}
