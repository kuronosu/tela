package remote

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// ADR-217 under tela dev: the host's backend gets the child app's ID, so
// the shortcuts it holds are registered under that ID and not the host's
// (tela's executable); RegisterApp and its status are served by the
// host's backend like the associations; ErrAppNotRegistered crosses the
// connection.

func TestHostUsesChildAppID(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a")
	a.b.Desktop().SetAppID("dev.example.Child") // at the start of Run, before any window
	if got := p.hb.DesktopAppID(); got != "dev.example.Child" {
		t.Fatalf("the host's backend has the app ID %q, want the child's", got)
	}
	a.start()
	show := platform.Shortcut{ID: "show", Accelerator: platform.Accelerator{Mods: platform.ModCtrl, Key: "k"}, Description: "Show"}
	a.ui(func() {
		if err := a.b.Desktop().RegisterShortcut(show); err != nil {
			t.Errorf("RegisterShortcut: %v", err)
		}
	})
	a.waitEvent("the status", isStatus("show", platform.ShortcutBound))
	if got := p.hb.DesktopAppID(); got != "dev.example.Child" {
		t.Errorf("the app ID when the shortcut was registered = %q", got)
	}

	// A reload of the same app: the same ID.
	b := p.dial("b")
	b.b.Desktop().SetAppID("dev.example.Child")
	if got := p.hb.DesktopAppID(); got != "dev.example.Child" {
		t.Errorf("after the reload = %q", got)
	}

	// The portal's refusal reaches the child with its sentinel.
	p.hb.SetShortcutError(fmt.Errorf("%w: no dev.example.Child.desktop", platform.ErrAppNotRegistered))
	a.ui(func() {
		other := show
		other.ID, other.Accelerator.Key = "other", "o"
		err := a.b.Desktop().RegisterShortcut(other)
		if !errors.Is(err, platform.ErrAppNotRegistered) || err.Error() == "" {
			t.Errorf("RegisterShortcut refused by the desktop = %v, want ErrAppNotRegistered", err)
		}
	})
}

func TestRegisterAppForwarded(t *testing.T) {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("a").start()
	as := a.b.Desktop().Associations() // used off the UI thread
	app := platform.HandlerApp{ID: "dev.example.Notes", Name: "Notes", Exec: "/usr/bin/notes"}
	if st, err := as.AppStatus(app); err != nil || st != (platform.AppStatus{}) {
		t.Errorf("AppStatus = %+v, %v", st, err)
	}
	if changed, err := as.RegisterApp(app, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("RegisterApp = %v, %v", changed, err)
	}
	if apps := p.hb.Associations().Apps(); len(apps) != 1 || !reflect.DeepEqual(apps[0], app) {
		t.Errorf("the host's apps = %+v", apps)
	}
	if st, err := as.AppStatus(app); err != nil || st != (platform.AppStatus{Registered: true, Requested: true}) {
		t.Errorf("AppStatus after RegisterApp = %+v, %v", st, err)
	}
	other := platform.HandlerApp{ID: "dev.example.Other", Name: "Other", Exec: "/usr/bin/other"}
	p.hb.Associations().SetAppEntry(other.ID, "/usr/share/applications/dev.example.Other.desktop")
	if st, err := as.AppStatus(other); err != nil || st.Other == "" || !st.Registered {
		t.Errorf("AppStatus of another's entry = %+v, %v", st, err)
	}
	if changed, err := as.RegisterApp(other, platform.RegisterOptions{Replace: true}); !changed || err != nil {
		t.Errorf("RegisterApp with Replace = %v, %v", changed, err)
	}
	if changed, err := as.UnregisterApp(app); !changed || err != nil {
		t.Errorf("UnregisterApp = %v, %v", changed, err)
	}
	p.hb.SetUnsupported(headless.ServiceAssociations)
	if _, err := as.RegisterApp(app, platform.RegisterOptions{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("RegisterApp without associations = %v, want ErrUnsupported", err)
	}
}
