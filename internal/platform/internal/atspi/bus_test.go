//go:build linux

package atspi

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// privateBus starts a dbus-daemon that knows no services and returns its
// address; it plays both the session bus and the accessibility bus. The
// test is skipped when dbus-daemon is not installed.
func privateBus(t *testing.T) string {
	t.Helper()
	exe, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed; the D-Bus tests need it")
	}
	dir, err := os.MkdirTemp("", "tela-atspi")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	conf := filepath.Join(dir, "bus.conf")
	err = os.WriteFile(conf, []byte(fmt.Sprintf(`<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-Bus Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:dir=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`, dir)), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "--config-file="+conf, "--nofork", "--print-address=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start dbus-daemon: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	addr := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(out).ReadString('\n')
		addr <- strings.TrimSpace(line)
	}()
	select {
	case a := <-addr:
		if a == "" {
			t.Fatal("dbus-daemon printed no address")
		}
		return a
	case <-time.After(10 * time.Second):
		t.Fatal("dbus-daemon did not start")
	}
	return ""
}

func connect(t *testing.T, addr string) *dbus.Conn {
	t.Helper()
	conn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatalf("connect to the private bus: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func own(t *testing.T, conn *dbus.Conn, name string) {
	t.Helper()
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("RequestName(%s) = %v, %v", name, reply, err)
	}
}

// --- fake org.a11y.Bus (the bus launcher) ---

type fakeLauncher struct {
	conn *dbus.Conn
	addr string

	mu      sync.Mutex
	enabled bool
}

func startLauncher(t *testing.T, addr string, enabled bool) *fakeLauncher {
	t.Helper()
	l := &fakeLauncher{conn: connect(t, addr), addr: addr, enabled: enabled}
	if err := l.conn.ExportMethodTable(map[string]any{
		"GetAddress": func() (string, *dbus.Error) { return l.addr, nil },
	}, a11yBusPath, a11yBusIface); err != nil {
		t.Fatal(err)
	}
	if err := l.conn.ExportMethodTable(map[string]any{
		"Get": func(iface, name string) (dbus.Variant, *dbus.Error) {
			l.mu.Lock()
			defer l.mu.Unlock()
			if iface == statusIface && name == "IsEnabled" {
				return dbus.MakeVariant(l.enabled), nil
			}
			return dbus.Variant{}, dbus.NewError(errUnknownProp, []any{name})
		},
		"GetAll": func(iface string) (map[string]dbus.Variant, *dbus.Error) {
			l.mu.Lock()
			defer l.mu.Unlock()
			// Like at-spi2-core 2.62 here: no ScreenReaderEnabled.
			return map[string]dbus.Variant{"IsEnabled": dbus.MakeVariant(l.enabled)}, nil
		},
	}, a11yBusPath, propsIface); err != nil {
		t.Fatal(err)
	}
	own(t, l.conn, a11yBusName)
	return l
}

func (l *fakeLauncher) setEnabled(v bool) {
	l.mu.Lock()
	l.enabled = v
	l.mu.Unlock()
	l.conn.Emit(a11yBusPath, propsIface+".PropertiesChanged", statusIface,
		map[string]dbus.Variant{"IsEnabled": dbus.MakeVariant(v)}, []string{})
}

// --- fake registry ---

type fakeRegistry struct {
	conn *dbus.Conn

	mu       sync.Mutex
	embedded []ref
	unembeds []ref
	events   []struct{ Bus, Event string }
}

func startRegistry(t *testing.T, addr string, events ...string) *fakeRegistry {
	t.Helper()
	r := &fakeRegistry{conn: connect(t, addr)}
	for _, e := range events {
		r.events = append(r.events, struct{ Bus, Event string }{":1.99", e})
	}
	if err := r.conn.ExportMethodTable(map[string]any{
		"Embed": func(plug ref) (ref, *dbus.Error) {
			r.mu.Lock()
			r.embedded = append(r.embedded, plug)
			r.mu.Unlock()
			// As registryd does, number the application.
			go r.conn.Object(plug.Name, plug.Path).Call(propsIface+".Set", 0, ifaceApplication, "Id", dbus.MakeVariant(int32(7)))
			return ref{r.conn.Names()[0], desktopRootPath}, nil
		},
		"Unembed": func(plug ref) *dbus.Error {
			r.mu.Lock()
			r.unembeds = append(r.unembeds, plug)
			r.mu.Unlock()
			return nil
		},
	}, desktopRootPath, socketIface); err != nil {
		t.Fatal(err)
	}
	if err := r.conn.ExportMethodTable(map[string]any{
		"GetRegisteredEvents": func() ([]struct{ Bus, Event string }, *dbus.Error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			if r.events == nil {
				// An old registry: the bridge sends every event.
				return nil, dbus.NewError(errUnknownMethod, []any{"GetRegisteredEvents"})
			}
			return append([]struct{ Bus, Event string }{}, r.events...), nil
		},
	}, registryPath, registryIface); err != nil {
		t.Fatal(err)
	}
	own(t, r.conn, registryName)
	return r
}

func (r *fakeRegistry) plugs() (embedded, unembedded []ref) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ref(nil), r.embedded...), append([]ref(nil), r.unembeds...)
}

// --- the UI thread ---

// stubWin stands for a backend's window: the bridge only hands it back.
type stubWin struct {
	platform.Window
	name string
}

type delivered struct {
	w *stubWin
	e platform.Event
}

// harness plays the backend: its loop is the test goroutine, which waits
// for Wake and calls Dispatch.
type harness struct {
	t      *testing.T
	b      *Bridge
	wake   chan struct{}
	events []delivered
}

func newHarness(t *testing.T, addr string, env map[string]string) *harness {
	t.Helper()
	h := &harness{t: t, wake: make(chan struct{}, 1)}
	h.b = New(Options{
		AppName: "tela-test",
		Address: addr,
		Wake: func() {
			select {
			case h.wake <- struct{}{}:
			default:
			}
		},
		Event:   func(w platform.Window, e platform.Event) { h.events = append(h.events, delivered{w.(*stubWin), e}) },
		Getenv:  func(k string) string { return env[k] },
		Logf:    func(format string, args ...any) { t.Logf(format, args...) },
		Timeout: 5 * time.Second,
	})
	t.Cleanup(h.b.Close)
	return h
}

// pump runs the loop until cond holds.
func (h *harness) pump(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case <-h.wake:
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s", what)
		}
		h.b.Dispatch()
	}
}

// next waits for the next delivered event.
func (h *harness) next(what string) delivered {
	h.t.Helper()
	h.pump(what, func() bool { return len(h.events) > 0 })
	d := h.events[0]
	h.events = h.events[1:]
	return d
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// --- the assistive technology ---

// at is a client that reads the app's objects as Orca would through
// libatspi, and records the events it receives.
type at struct {
	t    *testing.T
	conn *dbus.Conn
	app  string // the app's bus name

	sig chan *dbus.Signal
}

func newAT(t *testing.T, addr string) *at {
	t.Helper()
	a := &at{t: t, conn: connect(t, addr), sig: make(chan *dbus.Signal, 256)}
	a.conn.Signal(a.sig)
	for _, ifc := range []string{eventObjectIface, eventWindowIface} {
		if err := a.conn.AddMatchSignal(dbus.WithMatchInterface(ifc)); err != nil {
			t.Fatal(err)
		}
	}
	return a
}

// call calls method (interface.member) on path of the app and stores the
// reply in out.
func (a *at) call(path dbus.ObjectPath, method string, args []any, out ...any) error {
	c := a.conn.Object(a.app, path).Call(method, 0, args...)
	if c.Err != nil {
		return c.Err
	}
	if len(out) > 0 {
		return c.Store(out...)
	}
	return nil
}

func (a *at) must(path dbus.ObjectPath, method string, args []any, out ...any) {
	a.t.Helper()
	if err := a.call(path, method, args, out...); err != nil {
		a.t.Fatalf("%s on %s: %v", method, path, err)
	}
}

func (a *at) prop(path dbus.ObjectPath, iface, name string) any {
	a.t.Helper()
	var v dbus.Variant
	a.must(path, propsIface+".Get", []any{iface, name}, &v)
	return v.Value()
}

func (a *at) children(path dbus.ObjectPath) []ref {
	a.t.Helper()
	var kids []ref
	a.must(path, ifaceAccessible+".GetChildren", nil, &kids)
	return kids
}

func (a *at) role(path dbus.ObjectPath) uint32 {
	a.t.Helper()
	var r uint32
	a.must(path, ifaceAccessible+".GetRole", nil, &r)
	return r
}

func (a *at) name(path dbus.ObjectPath) string {
	a.t.Helper()
	return a.prop(path, ifaceAccessible, "Name").(string)
}

func (a *at) states(path dbus.ObjectPath) stateSet {
	a.t.Helper()
	var w []uint32
	a.must(path, ifaceAccessible+".GetState", nil, &w)
	return stateSet(w[0]) | stateSet(w[1])<<32
}

// event is a received AT-SPI event.
type event struct {
	path   dbus.ObjectPath
	name   string // "StateChanged"...
	detail string
	d1, d2 int32
	data   any
}

func (e event) String() string {
	return fmt.Sprintf("%s %s:%s(%d,%d,%v)", e.path, e.name, e.detail, e.d1, e.d2, e.data)
}

// drain returns the events received until none arrives for a while.
func (a *at) drain() []event {
	var out []event
	for {
		select {
		case s := <-a.sig:
			if len(s.Body) < 4 {
				continue
			}
			e := event{path: s.Path, name: s.Name[strings.LastIndex(s.Name, ".")+1:]}
			e.detail, _ = s.Body[0].(string)
			e.d1, _ = s.Body[1].(int32)
			e.d2, _ = s.Body[2].(int32)
			if v, ok := s.Body[3].(dbus.Variant); ok {
				e.data = v.Value()
			}
			out = append(out, e)
		case <-time.After(300 * time.Millisecond):
			return out
		}
	}
}

// waitEvent waits for an event matching f, returning the ones before it
// too.
func (a *at) waitEvent(what string, f func(event) bool) []event {
	a.t.Helper()
	var seen []event
	deadline := time.After(10 * time.Second)
	for {
		select {
		case s := <-a.sig:
			if len(s.Body) < 4 {
				continue
			}
			e := event{path: s.Path, name: s.Name[strings.LastIndex(s.Name, ".")+1:]}
			e.detail, _ = s.Body[0].(string)
			e.d1, _ = s.Body[1].(int32)
			e.d2, _ = s.Body[2].(int32)
			if v, ok := s.Body[3].(dbus.Variant); ok {
				e.data = v.Value()
			}
			seen = append(seen, e)
			if f(e) {
				return seen
			}
		case <-deadline:
			a.t.Fatalf("timed out waiting for %s; got %v", what, seen)
		}
	}
}
