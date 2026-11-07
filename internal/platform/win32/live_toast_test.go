//go:build windows

package win32

import (
	"fmt"
	"image/color"
	"os"
	"sync/atomic"
	"testing"
	"time"

	winreg "golang.org/x/sys/windows/registry"

	"github.com/kuronosu/tela/internal/platform"
)

// Live tests of the WinRT toasts (toast_windows.go). They need an attended
// Windows 10 1809+ or 11 desktop (TELA_LIVE=1, like the other shell tests).
// TestLiveToast drives everything WinRT can be made to do without a user;
// TestLiveToastClick (TELA_LIVE_CLICK=1 as well) waits for a person to
// click a button and then a toast's body.

const liveToastAppID = "dev.telaui.tela-live-toast"

// liveToastBackend returns a backend whose app ID is liveToastAppID, and
// removes the AUMID registration afterwards when the test created it.
func liveToastBackend(t *testing.T) *Backend {
	t.Helper()
	requireLiveShell(t)
	key := aumidKeyPrefix + toastAUMID(liveToastAppID)
	names, _ := hkcuRegistry{}.ValueNames(key)
	if names == nil {
		t.Cleanup(func() { _ = winreg.DeleteKey(winreg.CURRENT_USER, key) })
	}
	b := liveBackend(t)
	b.Desktop().SetAppID(liveToastAppID)
	return b
}

func liveToast(title string) platform.Notification {
	return platform.Notification{
		Title: title, Body: "Tela live test: " + time.Now().Format(time.TimeOnly), Icon: testIcon(),
		Actions: []platform.NotificationAction{{ID: "first", Label: "First"}, {ID: "second", Label: "Second"}},
	}
}

// TestLiveToast shows a toast with two actions, replaces it, hides it
// through WinRT (whose Dismissed event, ApplicationHidden, must arrive
// through the delegate as NotificationClosed), then shows and withdraws a
// second one (CloseNotification reports nothing). No delegate object is
// left alive afterwards, and the AUMID is registered with Tela's marker.
func TestLiveToast(t *testing.T) {
	b := liveToastBackend(t)
	d := b.Desktop()
	comServers.Lock()
	baseline := len(comServers.m)
	comServers.Unlock()
	var (
		started, closing atomic.Bool
		first, second    platform.NotificationID
		events           []platform.NotificationEvent
		skip             string
	)
	runLive(t, b, openWindow(t, b), func(w platform.Window, e platform.Event) {
		tst := &b.desk.toast
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, w, color.RGBA{R: 255, G: 255, A: 255})
			if started.Swap(true) {
				return
			}
			id, err := d.Notify(liveToast("Toast with two actions"))
			if err != nil {
				t.Errorf("Notify: %v", err)
				b.Quit()
				return
			}
			if tst.err != nil || tst.entries[id] == nil {
				skip = fmt.Sprintf("the notification was a balloon: %v", tst.err)
				b.Quit()
				return
			}
			first = id
			serial := tst.entries[id].serial
			if err := b.replaceToast(id, liveToast("Replaced toast")); err != nil {
				t.Errorf("replaceToast: %v", err)
			}
			if en := tst.entries[id]; en == nil || en.serial == serial || len(tst.entries) != 1 {
				t.Errorf("after replaceToast: entry %+v, %d entries", en, len(tst.entries))
			}
			time.AfterFunc(2*time.Second, func() { b.Wake() })
		case platform.WakeEvent:
			if en := tst.entries[first]; en != nil && !closing.Load() {
				// What a user's dismissal or a timeout does, but through the
				// app's notifier: WinRT raises Dismissed on our delegate.
				closing.Store(true)
				if hr := en.notifier.call(vtNotifierHide, uintptr(en.toast)); hr < 0 {
					t.Errorf("ToastNotifier.Hide = %#x", uint32(hr))
				}
				time.AfterFunc(5*time.Second, func() { b.Wake() }) // in case the event never comes
				return
			}
			if second != 0 {
				b.Quit() // the second toast was withdrawn a second ago and reported nothing
				return
			}
			t.Errorf("no NotificationClosed within 5 s of ToastNotifier.Hide")
			b.Quit()
		case platform.NotificationEvent:
			events = append(events, e)
			if second != 0 {
				return
			}
			id, err := d.Notify(liveToast("Withdrawn toast"))
			if err != nil || tst.entries[id] == nil {
				t.Errorf("second Notify = %d, %v (entries %v)", id, err, tst.entries)
				b.Quit()
				return
			}
			second = id
			if err := d.CloseNotification(id); err != nil {
				t.Errorf("CloseNotification: %v", err)
			}
			if err := d.CloseNotification(id); err != nil {
				t.Errorf("CloseNotification twice: %v", err)
			}
			if len(tst.entries) != 0 {
				t.Errorf("%d toasts left after CloseNotification", len(tst.entries))
			}
			time.AfterFunc(time.Second, func() { b.Wake() })
		}
	})
	if skip != "" {
		t.Skip(skip)
	}
	if want := []platform.NotificationEvent{{ID: first, Kind: platform.NotificationClosed}}; fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("notification events = %+v, want %+v", events, want)
	}
	if second != 0 && second != first+1 { // IDs are shared with the balloons and never reused
		t.Errorf("second ID %d after %d", second, first)
	}
	comServers.Lock()
	left := len(comServers.m) - baseline
	comServers.Unlock()
	if left != 0 {
		t.Errorf("%d COM objects of the toasts still alive after Run", left)
	}
	key := aumidKeyPrefix + toastAUMID(liveToastAppID)
	for _, name := range []string{aumidMarker, "DisplayName"} {
		if _, ok, err := (hkcuRegistry{}).String(key, name); !ok || err != nil {
			t.Errorf("HKCU\\%s has no %s (%v)", key, name, err)
		}
	}
}

// TestLiveToastClick asks a person to click the "Second" button of a toast,
// then the body of another, and checks the events (TELA_LIVE_CLICK=1).
func TestLiveToastClick(t *testing.T) {
	if os.Getenv("TELA_LIVE_CLICK") != "1" {
		t.Skip("set TELA_LIVE_CLICK=1 (with TELA_LIVE=1) and click the toasts within 60 s")
	}
	b := liveToastBackend(t)
	d := b.Desktop()
	var (
		started atomic.Bool
		ids     []platform.NotificationID
		events  []platform.NotificationEvent
	)
	timer := time.AfterFunc(120*time.Second, b.Quit)
	defer timer.Stop()
	openWindow(t, b)()
	notify := func(title string) {
		id, err := d.Notify(liveToast(title))
		if err != nil || b.desk.toast.entries[id] == nil {
			t.Errorf("Notify = %d, %v (toasts: %v)", id, err, b.desk.toast.err)
			b.Quit()
			return
		}
		ids = append(ids, id)
	}
	err := b.Run(platform.HandlerFunc(func(w platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			presentSolid(t, w, color.RGBA{R: 255, G: 255, A: 255})
			if !started.Swap(true) {
				notify("Click the button \"Second\"")
			}
		case platform.NotificationEvent:
			events = append(events, e)
			if len(events) == 1 {
				notify("Click this toast (not a button)")
				return
			}
			b.Quit()
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("toasts shown: %v; events %+v", ids, events)
	}
	want := []platform.NotificationEvent{
		{ID: ids[0], Kind: platform.NotificationActionInvoked, Action: "second"},
		{ID: ids[1], Kind: platform.NotificationActivated},
	}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Errorf("events = %+v, want %+v", events, want)
	}
}
