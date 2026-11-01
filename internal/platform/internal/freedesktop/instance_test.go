//go:build linux

package freedesktop

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

func TestInstanceBusName(t *testing.T) {
	for _, id := range []string{"com.example.Notes", "org.tela-test.App_2", "a.b"} {
		if got := instanceBusName(id); got != id {
			t.Errorf("instanceBusName(%q) = %q, want the ID itself", id, got)
		}
	}
	derived := map[string]string{}
	for _, id := range []string{"notes", "Notes", "a+b", "a_b", "1.example.App", "com..x", "com.example.", ".com.example",
		"com.example.Ñandú", "x-y", strings.Repeat("abcdefgh.", 40) + "z"} {
		name := instanceBusName(id)
		if !validBusName(name) {
			t.Errorf("instanceBusName(%q) = %q, not a valid bus name", id, name)
		}
		if !strings.HasPrefix(name, "dev.telaui.Instance.h") {
			t.Errorf("instanceBusName(%q) = %q, want a derived name", id, name)
		}
		if instanceBusName(id) != name {
			t.Errorf("instanceBusName(%q) is not deterministic", id)
		}
		if other, ok := derived[name]; ok {
			t.Errorf("instanceBusName(%q) = instanceBusName(%q) = %q", id, other, name)
		}
		derived[name] = id
		if p := busNamePath(name); !p.IsValid() {
			t.Errorf("busNamePath(%q) = %q, not a valid object path", name, p)
		}
	}
	if got := instanceBusName("notes"); !strings.HasSuffix(got, "_notes") {
		t.Errorf("instanceBusName(notes) = %q, want the ID readable at the end", got)
	}
	if got := busNamePath("org.tela-test.App"); got != "/org/tela_test/App" {
		t.Errorf("busNamePath = %q, want /org/tela_test/App", got)
	}
	for _, s := range []string{"", "a", "a..b", "1a.b", "a.1b", ":1.2", "a.b/c", "a.b c", strings.Repeat("a.", 128) + "b"} {
		if validBusName(s) {
			t.Errorf("validBusName(%q) = true", s)
		}
	}
}

func TestClaimInstance(t *testing.T) {
	addr := privateBus(t)
	first, second := newHarness(t, addr), newHarness(t, addr)
	const id = "dev.telaui.TestClaimInstance"

	primary, err := first.d.ClaimInstance(id, platform.Launch{Args: []string{"own"}})
	if err != nil || !primary {
		t.Fatalf("first ClaimInstance = %v, %v; want true", primary, err)
	}
	if primary, err := first.d.ClaimInstance(id, platform.Launch{}); err != nil || !primary {
		t.Fatalf("ClaimInstance again in the primary = %v, %v; want true", primary, err)
	}

	l := platform.Launch{Args: []string{"notes:open?id=3", "file.txt"}, Dir: "/home/u", Token: "token-1"}
	primary, err = second.d.ClaimInstance(id, l)
	if err != nil || primary {
		t.Fatalf("second ClaimInstance = %v, %v; want false", primary, err)
	}
	e, ok := first.event("the forwarded launch").(platform.LaunchEvent)
	if !ok || !slices.Equal(e.Launch.Args, l.Args) || e.Launch.Dir != l.Dir || e.Launch.Token != l.Token {
		t.Fatalf("event = %#v, want LaunchEvent %+v", e, l)
	}
	// A launch without arguments.
	if primary, err := second.d.ClaimInstance(id, platform.Launch{}); err != nil || primary {
		t.Fatalf("third ClaimInstance = %v, %v; want false", primary, err)
	}
	if e := first.event("an empty launch").(platform.LaunchEvent); e.Launch.Args != nil || e.Launch.Dir != "" || e.Launch.Token != "" {
		t.Errorf("event = %#v, want an empty Launch", e)
	}

	// org.freedesktop.Application reaches the primary too.
	client := connect(t, addr)
	obj := client.Object(id, busNamePath(id))
	if err := obj.Call(ifaceApplication+".Activate", 0, map[string]dbus.Variant{"activation-token": dbus.MakeVariant("tok-a")}).Err; err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if e := first.event("Activate").(platform.LaunchEvent); e.Launch.Args != nil || e.Launch.Token != "tok-a" {
		t.Errorf("Activate gave %+v", e.Launch)
	}
	if err := obj.Call(ifaceApplication+".Open", 0, []string{"file:///tmp/a%20b.txt", "notes:x"},
		map[string]dbus.Variant{"desktop-startup-id": dbus.MakeVariant("tok-x11")}).Err; err != nil {
		t.Fatalf("Open: %v", err)
	}
	if e := first.event("Open").(platform.LaunchEvent); !slices.Equal(e.Launch.Args, []string{"/tmp/a b.txt", "notes:x"}) || e.Launch.Token != "tok-x11" {
		t.Errorf("Open gave %+v", e.Launch)
	}
	if err := obj.Call(ifaceApplication+".ActivateAction", 0, "quit", []dbus.Variant{}, map[string]dbus.Variant{}).Err; err == nil {
		t.Error("ActivateAction succeeded; the app has no actions")
	}
	var xml string
	if err := obj.Call("org.freedesktop.DBus.Introspectable.Introspect", 0).Store(&xml); err != nil || !strings.Contains(xml, ifaceInstance) {
		t.Errorf("Introspect = %q, %v", xml, err)
	}

	// When the primary's loop ends, the name is released: the next process
	// is the primary.
	first.d.Close()
	eventually(t, "the name released", func() bool {
		var owned bool
		client.BusObject().Call(busIface+".NameHasOwner", 0, id).Store(&owned)
		return !owned
	})
	primary, err = second.d.ClaimInstance(id, platform.Launch{})
	if err != nil || !primary {
		t.Fatalf("ClaimInstance after the primary closed = %v, %v; want true", primary, err)
	}
	if len(first.events) != 0 {
		t.Errorf("events after Close: %+v", first.events)
	}
}

func TestClaimInstanceDerivedName(t *testing.T) {
	addr := privateBus(t)
	first, second := newHarness(t, addr), newHarness(t, addr)
	if primary, err := first.d.ClaimInstance("notes", platform.Launch{}); err != nil || !primary {
		t.Fatalf("first ClaimInstance = %v, %v", primary, err)
	}
	if primary, err := second.d.ClaimInstance("notes", platform.Launch{Args: []string{"x"}}); err != nil || primary {
		t.Fatalf("second ClaimInstance = %v, %v", primary, err)
	}
	if e := first.event("launch").(platform.LaunchEvent); !slices.Equal(e.Launch.Args, []string{"x"}) {
		t.Errorf("launch = %+v", e.Launch)
	}
	// Another ID is another instance.
	if primary, err := second.d.ClaimInstance("notes2", platform.Launch{}); err != nil || !primary {
		t.Fatalf("ClaimInstance of another ID = %v, %v; want true", primary, err)
	}
}

func TestClaimInstanceErrors(t *testing.T) {
	h := newHarness(t, "unix:path=/nonexistent/tela-test-bus")
	if _, err := h.d.ClaimInstance("dev.telaui.X", platform.Launch{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("ClaimInstance without a bus = %v, want ErrUnsupported", err)
	}

	// The name belongs to a process that is not a Tela app: an error, not
	// a claim and not a forward.
	addr := privateBus(t)
	other := connect(t, addr)
	own(t, other, "dev.telaui.Taken")
	h = newHarness(t, addr)
	primary, err := h.d.ClaimInstance("dev.telaui.Taken", platform.Launch{})
	if err == nil || primary || errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("ClaimInstance of a foreign name = %v, %v; want false and an error", primary, err)
	}

	h.d.Close()
	if _, err := h.d.ClaimInstance("dev.telaui.Y", platform.Launch{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("ClaimInstance after Close = %v, want ErrUnsupported", err)
	}
	defer func() {
		if recover() == nil {
			t.Error("ClaimInstance with an invalid app ID did not panic")
		}
	}()
	h.d.ClaimInstance("bad id", platform.Launch{})
}
