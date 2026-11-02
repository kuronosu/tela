//go:build linux

package freedesktop

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// The app ID of the GlobalShortcuts portal (ADR-217), against a fake
// portal that behaves like xdg-desktop-portal 1.22: Register refuses an
// ID without a desktop entry, and CreateSession a connection that did not
// register. Everything runs on a private bus with temporary XDG
// directories.

func TestDefaultAppID(t *testing.T) {
	d := New(Options{Wake: func() {}, Post: func(func()) {}, Event: func(platform.Event) {}})
	if got := d.appID(); got != platform.DefaultAppID() {
		t.Errorf("app ID without Options.AppID = %q, want %q", got, platform.DefaultAppID())
	}
	d.SetAppID("dev.example.App")
	if got := d.appID(); got != "dev.example.App" {
		t.Errorf("app ID after SetAppID = %q", got)
	}
	defer func() {
		if recover() == nil {
			t.Error("SetAppID with a space did not panic")
		}
	}()
	d.SetAppID("bad id")
}

func TestNotificationsUseSetAppID(t *testing.T) {
	addr := privateBus(t)
	n := startNotifier(t, addr)
	h := newHarness(t, addr)
	h.d.SetAppID("dev.example.App")
	if _, err := h.d.Notify(platform.Notification{Title: "Hi"}); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the notification", func() bool { return len(n.all()) == 1 })
	got := n.all()[0]
	if got.AppName != "dev.example.App" || got.Hints["desktop-entry"].Value() != "dev.example.App" {
		t.Errorf("app name %q, desktop-entry %v; want the app ID", got.AppName, got.Hints["desktop-entry"])
	}
}

func TestShortcutsUseSetAppID(t *testing.T) {
	// The backend starts with the executable's name; the app's ID, set
	// before the first shortcut, is the one registered.
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{shortcuts: true, registry: true,
		dataDirs: []string{appEntries(t, "dev.example.App")}})
	h := newHarness(t, addr)
	h.d.SetAppID("dev.example.App")
	if err := h.d.RegisterShortcut(shortcut("show", platform.ModCtrl, "k")); err != nil {
		t.Fatalf("RegisterShortcut = %v", err)
	}
	if se := h.status("bound"); se.Status != platform.ShortcutBound {
		t.Fatalf("status = %+v", se)
	}
	if registered, refused := p.registrations(); !slices.Equal(registered, []string{"dev.example.App"}) || len(refused) != 0 {
		t.Errorf("registered %q, refused %q; want dev.example.App only", registered, refused)
	}
}

func TestShortcutsWrongAppID(t *testing.T) {
	// The bug of the live test: the executable's name has no entry, so
	// the portal refuses it and every shortcut with it.
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{shortcuts: true, registry: true,
		dataDirs: []string{appEntries(t, "dev.example.App")}})
	h := newHarness(t, addr) // Options.AppID "tela-test", as an executable's name
	err := h.d.RegisterShortcut(shortcut("show", platform.ModCtrl, "k"))
	if !errors.Is(err, platform.ErrAppNotRegistered) {
		t.Fatalf("RegisterShortcut under an ID without an entry = %v, want ErrAppNotRegistered", err)
	}
	if errors.Is(err, platform.ErrUnsupported) {
		t.Error("the refusal is ErrUnsupported too")
	}
	for _, want := range []string{"tela-test.desktop", "desktop.RegisterApp", "App info not found"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q: %v", want, err)
		}
	}
	h.quiet("after a refused registration")
	calls, _, _, _ := p.snapshot()
	if slices.Contains(calls, "CreateSession") || slices.Contains(calls, "CreateSession refused") {
		t.Errorf("a session was asked for after the refusal: %q", calls)
	}
	// The fixed ID works, on a new connection.
	h.d.SetAppID("dev.example.App")
	if err := h.d.RegisterShortcut(shortcut("show", platform.ModCtrl, "k")); err != nil {
		t.Fatalf("RegisterShortcut after SetAppID = %v", err)
	}
	if se := h.status("bound"); se.Status != platform.ShortcutBound {
		t.Fatalf("status = %+v", se)
	}
}

func TestShortcutsAfterRegisterApp(t *testing.T) {
	// No entry, then the app writes its own with RegisterApp: the next
	// RegisterShortcut is accepted.
	data := t.TempDir()
	addr := privateBus(t)
	startPortal13(t, addr, portal13Options{shortcuts: true, registry: true, dataDirs: []string{data}})
	h := newHarness(t, addr)
	h.d.SetAppID("dev.example.Notes")
	s := shortcut("quick-note", platform.ModCtrl|platform.ModAlt, "n")
	if err := h.d.RegisterShortcut(s); !errors.Is(err, platform.ErrAppNotRegistered) {
		t.Fatalf("RegisterShortcut without an entry = %v, want ErrAppNotRegistered", err)
	}
	if !strings.Contains(h.d.RegisterShortcut(s).Error(), "dev.example.Notes.desktop") {
		t.Error("the error does not name the entry")
	}
	app := platform.HandlerApp{ID: "dev.example.Notes", Name: "Notes", Exec: "/opt/notes/notes"}
	a := assoc.Linux(assoc.LinuxDirs{DataHome: data, Run: func(string, ...string) error { return nil }})
	if changed, err := a.RegisterApp(app, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("RegisterApp = %v, %v", changed, err)
	}
	if st, _ := a.AppStatus(app); !st.Registered {
		t.Fatalf("AppStatus after RegisterApp = %+v", st)
	}
	if err := h.d.RegisterShortcut(s); err != nil {
		t.Fatalf("RegisterShortcut after RegisterApp = %v", err)
	}
	if se := h.status("bound"); se != (platform.ShortcutStatusEvent{ID: "quick-note", Status: platform.ShortcutBound, Trigger: "CTRL+ALT+n"}) {
		t.Fatalf("status = %+v", se)
	}
}

func TestShortcutsAppIDChanges(t *testing.T) {
	// A new app ID (a tela dev reload of an app whose ID changed) binds
	// the set again under it, on a new connection; the binding stands, so
	// no status follows.
	addr := privateBus(t)
	p := startPortal13(t, addr, portal13Options{shortcuts: true, registry: true,
		dataDirs: []string{appEntries(t, "dev.example.A", "dev.example.B")}})
	h := newHarness(t, addr)
	h.d.SetAppID("dev.example.A")
	if err := h.d.RegisterShortcut(shortcut("show", platform.ModCtrl, "k")); err != nil {
		t.Fatal(err)
	}
	h.status("bound")
	h.d.SetAppID("dev.example.A") // unchanged: nothing
	h.d.SetAppID("dev.example.B")
	h.d.Dispatch()
	eventually(t, "the second session", func() bool {
		_, _, sessions, _ := p.snapshot()
		return len(sessions) == 2
	})
	h.quiet("after the new app ID")
	registered, _ := p.registrations()
	slices.Sort(registered)
	if !slices.Equal(registered, []string{"dev.example.A", "dev.example.B"}) {
		t.Errorf("registered %q, want A then B", registered)
	}
	_, _, sessions, closed := p.snapshot()
	if !closed[sessions[0]] {
		t.Error("the first session was not closed")
	}
}
