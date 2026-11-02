package desktop_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// ADR-217: RegisterApp in a running app goes to its backend, looks before
// writing and leaves another program's entry alone; a desktop that
// refuses the app ID fails the shortcut with ErrAppNotRegistered; a
// desktop that assigns no keys reports Bound with an empty Trigger.

func TestRegisterAppInApp(t *testing.T) {
	app := telatest.MountApp(t, tela.App{AppID: "com.example.Notes", Root: tela.C(func() tela.Node { return Text("notes") })})
	// The ID explicit: an earlier test of the package may have called
	// desktop.SetAppID, which wins over the App's.
	forApp := desktop.ForApp(desktop.HandlerApp{ID: "com.example.Notes", Exec: "/opt/notes/notes"})
	if s, err := desktop.AppStatus(forApp); err != nil || s != (desktop.AppRegistration{}) {
		t.Errorf("AppStatus = %+v, %v", s, err)
	}
	if changed, err := desktop.RegisterApp(forApp); !changed || err != nil {
		t.Fatalf("RegisterApp = %v, %v", changed, err)
	}
	if changed, err := desktop.RegisterApp(forApp); changed || err != nil {
		t.Errorf("RegisterApp again = %v, %v; want nothing written", changed, err)
	}
	apps := app.RegisteredApps()
	if len(apps) != 1 || apps[0].ID != "com.example.Notes" || apps[0].Name != "Notes" || apps[0].Exec != "/opt/notes/notes" {
		t.Errorf("registered apps %+v", apps)
	}
	if s, err := desktop.AppStatus(forApp); err != nil || s != (desktop.AppRegistration{Registered: true, Requested: true}) {
		t.Errorf("AppStatus after RegisterApp = %+v, %v", s, err)
	}
	if changed, err := desktop.UnregisterApp(forApp); !changed || err != nil {
		t.Errorf("UnregisterApp = %v, %v", changed, err)
	}
	if apps := app.RegisteredApps(); len(apps) != 0 {
		t.Errorf("after UnregisterApp %+v", apps)
	}

	// A package's entry: nothing written without Replace.
	app.HoldAppEntry("com.example.Notes", "/usr/share/applications/com.example.Notes.desktop")
	if changed, err := desktop.RegisterApp(forApp); changed || err != nil {
		t.Errorf("RegisterApp beside a package's entry = %v, %v", changed, err)
	}
	if s, _ := desktop.AppStatus(forApp); !s.Registered || s.Requested || s.Other == "" {
		t.Errorf("AppStatus beside a package's entry = %+v", s)
	}
	if changed, err := desktop.RegisterApp(forApp, desktop.Replace()); !changed || err != nil {
		t.Errorf("RegisterApp with Replace = %v, %v", changed, err)
	}

	app.Unsupported(telatest.Associations)
	if _, err := desktop.RegisterApp(forApp); !errors.Is(err, desktop.ErrUnsupported) {
		t.Errorf("RegisterApp without registration: %v", err)
	}
}

func TestGlobalShortcutAppNotRegistered(t *testing.T) {
	var got desktop.ShortcutStatusEvent
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(false)
		return Div(Button(OnClick(func() { setOn.Set(true) }), Text("On")),
			If(on, tela.Component(desktop.GlobalShortcut{ID: "x", Keys: "Ctrl+K", Description: "X",
				OnStatus: func(e desktop.ShortcutStatusEvent) { got = e }})))
	})
	app.FailShortcuts(fmt.Errorf("%w: no entry com.example.x.desktop; call desktop.RegisterApp", desktop.ErrAppNotRegistered))
	app.Click(telatest.ByRole("button", "On"))
	if got.Status != desktop.ShortcutFailed || !errors.Is(got.Err, desktop.ErrAppNotRegistered) ||
		errors.Is(got.Err, desktop.ErrUnsupported) || !strings.Contains(got.Err.Error(), "com.example.x.desktop") {
		t.Errorf("status %+v, want Failed wrapping ErrAppNotRegistered and naming the entry", got)
	}
	if len(app.GlobalShortcuts()) != 0 {
		t.Errorf("a refused shortcut was registered: %+v", app.GlobalShortcuts())
	}
}

func TestGlobalShortcutUnassigned(t *testing.T) {
	var got []desktop.ShortcutStatusEvent
	pressed := 0
	app := telatest.MountFunc(t, func() tela.Node {
		return tela.Component(desktop.GlobalShortcut{ID: "x", Keys: "Ctrl+Alt+N", Description: "X",
			OnPress:  func() { pressed++ },
			OnStatus: func(e desktop.ShortcutStatusEvent) { got = append(got, e) }})
	})
	app.SetShortcutUnassigned("x")
	if e := got[len(got)-1]; e.Status != desktop.ShortcutBound || e.Trigger != "" || e.Err != nil {
		t.Errorf("status %+v, want Bound with no trigger", e)
	}
	app.PressShortcut("x") // the user bound it in the compositor
	if pressed != 1 {
		t.Errorf("presses = %d", pressed)
	}
}
