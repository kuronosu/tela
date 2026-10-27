//go:build windows

package win32

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of the desktop services. They show real dialogs, menus, tray
// icons and toasts, and answer them as a user would by posting the
// messages their buttons send, so they run on Windows only: Wine's shell
// (notification area, common item dialogs) is not the one they check.

var (
	procFindWindowW = user32.NewProc("FindWindowW")
	procEndMenu     = user32.NewProc("EndMenu")
	procIsWindow    = user32.NewProc("IsWindow")
)

const (
	wmCommand  = 0x0111
	dialogTime = 10 * time.Second
)

// TestNotifyIconDataLayout pins NOTIFYICONDATAW, whose size Windows checks
// (cbSize) and whose fields it reads by offset.
func TestNotifyIconDataLayout(t *testing.T) {
	var nid notifyIconData
	w64 := unsafe.Sizeof(uintptr(0)) == 8
	pick := func(a, b uintptr) uintptr {
		if w64 {
			return a
		}
		return b
	}
	for name, tc := range map[string]struct{ got, want uintptr }{
		"size":        {unsafe.Sizeof(nid), pick(976, 956)},
		"hwnd":        {unsafe.Offsetof(nid.hwnd), pick(8, 4)},
		"icon":        {unsafe.Offsetof(nid.icon), pick(32, 20)},
		"tip":         {unsafe.Offsetof(nid.tip), pick(40, 24)},
		"state":       {unsafe.Offsetof(nid.state), pick(296, 280)},
		"info":        {unsafe.Offsetof(nid.info), pick(304, 288)},
		"version":     {unsafe.Offsetof(nid.version), pick(816, 800)},
		"infoTitle":   {unsafe.Offsetof(nid.infoTitle), pick(820, 804)},
		"infoFlags":   {unsafe.Offsetof(nid.infoFlags), pick(948, 932)},
		"guidItem":    {unsafe.Offsetof(nid.guidItem), pick(952, 936)},
		"balloonIcon": {unsafe.Offsetof(nid.balloonIcon), pick(968, 952)},
	} {
		if tc.got != tc.want {
			t.Errorf("NOTIFYICONDATAW %s = %d, want %d", name, tc.got, tc.want)
		}
	}
	if s := unsafe.Sizeof(guid{}); s != 16 {
		t.Errorf("sizeof(GUID) = %d, want 16", s)
	}
}

// openWindow returns a runLive setup that opens a window: its first
// FrameEvent starts each test.
func openWindow(t *testing.T, b *Backend) func() {
	return func() {
		if _, err := b.NewWindow(platform.WindowOptions{Title: "tela live desktop", Size: platform.Size{W: 320, H: 200}}); err != nil {
			t.Fatalf("NewWindow: %v", err)
		}
	}
}

func requireShell(t *testing.T) {
	t.Helper()
	requireInteractiveDesktop(t)
	if inWine() {
		t.Skip("Wine's shell is not the one these tests check; they run on Windows")
	}
}

// waitWindow waits for a top-level window of the class whose title is
// title ("" for any) and returns it, or 0 after the timeout.
func waitWindow(class, title string) uintptr {
	cls := utf16(class)
	var name *uint16
	if title != "" {
		name = utf16(title)
	}
	for deadline := time.Now().Add(dialogTime); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(name))); h != 0 {
			return h
		}
	}
	return 0
}

func isWindow(h uintptr) bool {
	r, _, _ := procIsWindow.Call(h)
	return r != 0
}

func testIcon() []image.Image {
	var images []image.Image
	for _, side := range []int{16, 32} {
		img := image.NewNRGBA(image.Rect(0, 0, side, side))
		for y := range side {
			for x := range side {
				img.SetNRGBA(x, y, color.NRGBA{R: 79, G: 70, B: 229, A: uint8(255 * (x + y) / (2*side - 2))})
			}
		}
		images = append(images, img)
	}
	return images
}

// TestLiveDialogs asks for a save dialog and a message box in one pass.
// Neither starts inside the call; the box waits for the dialog; events
// are delivered while the dialog is open; Cancel is "" with no error; the
// box answers the button pressed.
func TestLiveDialogs(t *testing.T) {
	requireShell(t)
	b := liveBackend(t)
	d := b.Desktop()
	saveTitle := fmt.Sprintf("tela live save %d", os.Getpid())
	boxTitle := fmt.Sprintf("tela live box %d", os.Getpid())
	var (
		started, wakeInDialog atomic.Bool
		order                 []string
		savePath              = "unset"
		saveErr, boxErr       error
		answer                platform.MessageAnswer
		driverErr             atomic.Value
	)
	fail := func(format string, args ...any) {
		driverErr.CompareAndSwap(nil, fmt.Sprintf(format, args...))
		b.Quit()
	}
	drive := func() {
		dlg := waitWindow("#32770", saveTitle)
		if dlg == 0 {
			fail("the save dialog did not open")
			return
		}
		if h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16("#32770"))), uintptr(unsafe.Pointer(utf16(boxTitle)))); h != 0 {
			fail("the message box opened while the save dialog was open")
		}
		b.Wake()
		for deadline := time.Now().Add(dialogTime); !wakeInDialog.Load() && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
		postMessage(dlg, wmCommand, idCancel, 0)
		box := waitWindow("#32770", boxTitle)
		if box == 0 {
			fail("the message box did not open after the dialog closed")
			return
		}
		postMessage(box, wmCommand, idYes, 0)
	}
	runLive(t, b, openWindow(t, b), func(w platform.Window, e platform.Event) {
		switch e.(type) {
		case platform.FrameEvent:
			presentSolid(t, w, color.RGBA{B: 255, A: 255})
			if started.Swap(true) {
				return
			}
			err := d.SaveFile(platform.FileDialogOptions{
				Parent: w, Title: saveTitle, Directory: t.TempDir(), Name: "tela.txt",
				Filters: []platform.FileFilter{{Name: "Text", Patterns: []string{"*.txt"}}},
			}, func(path string, err error) {
				order = append(order, "save")
				savePath, saveErr = path, err
			})
			if err != nil {
				fail("SaveFile: %v", err)
				return
			}
			err = d.MessageBox(platform.MessageBoxOptions{Parent: w, Kind: platform.MessageQuestion, Title: boxTitle,
				Text: "Answered by the test.", Buttons: platform.ButtonsYesNo}, func(a platform.MessageAnswer, err error) {
				order = append(order, "box")
				answer, boxErr = a, err
				b.Quit()
			})
			if err != nil {
				fail("MessageBox: %v", err)
				return
			}
			if len(order) != 0 {
				fail("a callback ran inside the request")
			}
			go drive()
		case platform.WakeEvent:
			if b.desk.modalRunning {
				wakeInDialog.Store(true)
			}
		}
	})
	if s, ok := driverErr.Load().(string); ok {
		t.Fatal(s)
	}
	if !wakeInDialog.Load() {
		t.Errorf("a Wake while the dialog was open was not delivered until it closed")
	}
	if fmt.Sprint(order) != "[save box]" {
		t.Errorf("callbacks ran in the order %v, want [save box]", order)
	}
	if savePath != "" || saveErr != nil {
		t.Errorf("canceled SaveFile = %q, %v; want \"\", nil", savePath, saveErr)
	}
	if answer != platform.AnswerYes || boxErr != nil {
		t.Errorf("MessageBox = %v, %v; want yes, nil", answer, boxErr)
	}
}

// TestLiveQuitClosesDialogs: Quit while a file dialog (and then a message
// box) is open makes Run return; their callbacks are not called.
func TestLiveQuitClosesDialogs(t *testing.T) {
	requireShell(t)
	for _, kind := range []string{"folder", "box"} {
		t.Run(kind, func(t *testing.T) {
			b := liveBackend(t)
			d := b.Desktop()
			title := fmt.Sprintf("tela live quit %s %d", kind, os.Getpid())
			var started, called atomic.Bool
			var stuck atomic.Value
			runLive(t, b, openWindow(t, b), func(w platform.Window, e platform.Event) {
				if _, ok := e.(platform.FrameEvent); !ok {
					return
				}
				presentSolid(t, w, color.RGBA{G: 255, A: 255})
				if started.Swap(true) {
					return
				}
				var err error
				if kind == "folder" {
					err = d.PickFolder(platform.FileDialogOptions{Parent: w, Title: title}, func(string, error) { called.Store(true) })
				} else {
					err = d.MessageBox(platform.MessageBoxOptions{Parent: w, Title: title, Text: "Quit closes it."},
						func(platform.MessageAnswer, error) { called.Store(true) })
				}
				if err != nil {
					t.Errorf("request: %v", err)
					b.Quit()
					return
				}
				go func() {
					h := waitWindow("#32770", title)
					if h == 0 {
						stuck.Store("the dialog did not open")
						b.Quit()
						return
					}
					b.Quit()
					time.Sleep(5 * time.Second)
					if isWindow(h) {
						stuck.Store("the dialog was still open 5 s after Quit")
						postMessage(h, wmCommand, idCancel, 0)
					}
				}()
			})
			if s, ok := stuck.Load().(string); ok {
				t.Error(s)
			}
			if called.Load() {
				t.Errorf("the callback ran although Run returned first")
			}
		})
	}
}

// TestLiveTray adds a tray icon and drives it with the callbacks Explorer
// sends: a click is TrayActivated, a secondary click opens the menu (the
// loop keeps delivering while it is open), a dismissed menu reports
// nothing.
func TestLiveTray(t *testing.T) {
	requireShell(t)
	b := liveBackend(t)
	d := b.Desktop()
	menu := []platform.MenuItem{
		{ID: 1, Label: "Open & show"},
		{Separator: true},
		{Label: "More", Submenu: []platform.MenuItem{{ID: 2, Label: "Checked", Checkable: true, Checked: true}}},
		{ID: 3, Label: "Disabled", Disabled: true},
	}
	var (
		tray       platform.TrayID
		uid        uint16
		started    atomic.Bool
		menuWake   atomic.Bool
		events     []platform.TrayEvent
		driverFail atomic.Value
	)
	callback := func(event uint16, x, y int) {
		postMessage(b.desk.hwnd, wmTelaTray, mouseLParam(x, y), uintptr(uid)<<16|uintptr(event))
	}
	runLive(t, b, func() {
		openWindow(t, b)()
		var err error
		tray, err = d.NewTray(platform.TrayOptions{Icon: testIcon(), Tooltip: "Tela live tray", Menu: menu})
		if err != nil {
			t.Fatalf("NewTray: %v", err)
		}
		if ic := b.findIcon(func(ic *shellIcon) bool { return ic.tray == tray }); ic != nil {
			uid = ic.uid
			if !ic.added {
				t.Logf("the notification area did not take the icon; driving its callbacks anyway")
			}
		}
	}, func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, w, color.RGBA{R: 255, A: 255})
			if !started.Swap(true) {
				callback(ninSelect, 0, 0)
			}
		case platform.TrayEvent:
			events = append(events, e)
			if e.Kind == platform.TrayActivated && len(events) == 1 {
				callback(trayContextMenu, 200, 200)
				go func() {
					if waitWindow("#32768", "") == 0 {
						driverFail.CompareAndSwap(nil, "the tray menu did not open")
					}
					b.Wake()
				}()
			}
		case platform.WakeEvent:
			// Inside the menu's modal loop: dismiss it.
			menuWake.Store(true)
			procEndMenu.Call()
			if err := d.UpdateTray(tray, platform.TrayOptions{Icon: testIcon(), Tooltip: "updated"}); err != nil {
				t.Errorf("UpdateTray: %v", err)
			}
			if err := d.RemoveTray(tray); err != nil {
				t.Errorf("RemoveTray: %v", err)
			}
			if err := d.UpdateTray(tray, platform.TrayOptions{Icon: testIcon()}); err == nil {
				t.Errorf("UpdateTray of a removed tray = nil, want an error")
			}
			if err := d.RemoveTray(tray); err != nil {
				t.Errorf("RemoveTray twice = %v, want nil", err)
			}
			b.Quit()
		}
	})
	if s, ok := driverFail.Load().(string); ok {
		t.Error(s)
	}
	if !menuWake.Load() {
		t.Errorf("no event was delivered while the menu was open")
	}
	if want := []platform.TrayEvent{{Tray: tray, Kind: platform.TrayActivated}}; fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("tray events = %+v, want %+v (a dismissed menu reports nothing)", events, want)
	}
	if len(b.desk.icons) != 0 || b.desk.hwnd != 0 {
		t.Errorf("after Run: %d icons, tray window %#x; want none", len(b.desk.icons), b.desk.hwnd)
	}
}

// TestLiveNotification shows a balloon (a toast on Windows 10 and 11) and
// drives it with the callbacks of a click; CloseNotification withdraws one
// without an event.
func TestLiveNotification(t *testing.T) {
	requireShell(t)
	b := liveBackend(t)
	d := b.Desktop()
	var (
		started atomic.Bool
		first   platform.NotificationID
		events  []platform.NotificationEvent
		skip    string
	)
	runLive(t, b, openWindow(t, b), func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, w, color.RGBA{R: 255, G: 255, A: 255})
			if started.Swap(true) {
				return
			}
			id, err := d.Notify(platform.Notification{Title: "Tela", Body: "Live test notification", Icon: testIcon(),
				Actions: []platform.NotificationAction{{ID: "dropped", Label: "Dropped"}}})
			if errors.Is(err, errNoNotificationArea) {
				skip = err.Error()
				b.Quit()
				return
			}
			if err != nil || id == 0 {
				t.Errorf("Notify = %d, %v", id, err)
				b.Quit()
				return
			}
			first = id
			ic := b.findIcon(func(ic *shellIcon) bool { return ic.note == id })
			postMessage(b.desk.hwnd, wmTelaTray, 0, uintptr(ic.uid)<<16|ninBalloonUserClick)
		case platform.NotificationEvent:
			events = append(events, e)
			if b.findIcon(func(ic *shellIcon) bool { return ic.note == e.ID }) != nil {
				t.Errorf("the icon of notification %d is still there after %v", e.ID, e.Kind)
			}
			id, err := d.Notify(platform.Notification{Body: "Withdrawn at once"})
			if err != nil {
				t.Errorf("second Notify: %v", err)
			} else if id == first {
				t.Errorf("second Notify reused ID %d", id)
			}
			if err := d.CloseNotification(id); err != nil {
				t.Errorf("CloseNotification: %v", err)
			}
			if err := d.CloseNotification(id); err != nil {
				t.Errorf("CloseNotification twice: %v", err)
			}
			if len(b.desk.icons) != 0 {
				t.Errorf("%d icons left after CloseNotification", len(b.desk.icons))
			}
			b.Quit()
		}
	})
	if skip != "" {
		t.Skip(skip)
	}
	if want := []platform.NotificationEvent{{ID: first, Kind: platform.NotificationActivated}}; fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("notification events = %+v, want %+v", events, want)
	}
}

// TestLiveOpenErrors checks the requests ShellExecuteW must refuse without
// opening anything.
func TestLiveOpenErrors(t *testing.T) {
	requireShell(t)
	b := liveBackend(t)
	d := b.Desktop()
	runLive(t, b, func() {
		if err := d.OpenURI(nil, `C:/Windows/notepad.exe`); err == nil {
			t.Errorf("OpenURI of a drive path = nil, want an error")
		}
		missing := filepath.Join(t.TempDir(), "missing.tela-test")
		if err := d.OpenPath(nil, missing); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("OpenPath(%q) = %v, want fs.ErrNotExist", missing, err)
		}
		if err := d.OpenPath(nil, ""); err == nil {
			t.Errorf("OpenPath(\"\") = nil, want an error")
		}
		b.Quit()
	}, func(platform.Window, platform.Event) {})
}
