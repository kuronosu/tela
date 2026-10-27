//go:build linux

package freedesktop

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
	"golang.org/x/sys/unix"

	"github.com/kuronosu/tela/internal/platform"
)

// privateBus starts a dbus-daemon with a session configuration that knows
// no services (nothing is activated), and returns its address. The test is
// skipped when dbus-daemon is not installed.
func privateBus(t *testing.T) string {
	t.Helper()
	exe, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon is not installed; the D-Bus tests need it")
	}
	// A short directory: socket paths are limited to 108 bytes.
	dir, err := os.MkdirTemp("", "tela-dbus")
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

// connect opens a connection to the private bus for a fake service or a
// test client; closed at the end of the test.
func connect(t *testing.T, addr string) *dbus.Conn {
	t.Helper()
	conn, err := dbus.Connect(addr)
	if err != nil {
		t.Fatalf("connect to the private bus: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// own makes conn the owner of name.
func own(t *testing.T, conn *dbus.Conn, name string) {
	t.Helper()
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("RequestName(%s) = %v, %v", name, reply, err)
	}
}

// harness plays the backend: its loop is the test goroutine, which waits
// for Wake, calls Dispatch and runs the posted callbacks, as a backend's
// delivery pass does.
type harness struct {
	t      *testing.T
	d      *Desktop
	wake   chan struct{}
	posted []func()
	events []platform.Event
	opener string // file the fake opener writes its argument to
}

func newHarness(t *testing.T, addr string) *harness {
	t.Helper()
	h := &harness{t: t, wake: make(chan struct{}, 1)}
	dir := t.TempDir()
	h.opener = filepath.Join(dir, "opened")
	script := filepath.Join(dir, "fake-xdg-open")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$1\" > "+h.opener+".tmp && mv "+h.opener+".tmp "+h.opener+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.d = New(Options{
		AppID:   "tela-test",
		Address: addr,
		Opener:  script,
		Wake: func() {
			select {
			case h.wake <- struct{}{}:
			default:
			}
		},
		Post:  func(fn func()) { h.posted = append(h.posted, fn) },
		Event: func(e platform.Event) { h.events = append(h.events, e) },
		Parent: func(w platform.Window) string {
			if w != nil {
				return "x11:2a"
			}
			return ""
		},
	})
	t.Cleanup(h.d.Close)
	return h
}

// pump runs the loop until cond holds, failing after a timeout.
func (h *harness) pump(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(10 * time.Second)
	for !cond() {
		select {
		case <-h.wake:
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s", what)
		}
		h.d.Dispatch()
		posted := h.posted
		h.posted = nil
		for _, fn := range posted {
			fn()
		}
	}
}

// event waits for the next app event.
func (h *harness) event(what string) platform.Event {
	h.t.Helper()
	h.pump(what, func() bool { return len(h.events) > 0 })
	e := h.events[0]
	h.events = h.events[1:]
	return e
}

// eventually polls cond (state of a fake service, which does not wake the
// harness) until it holds.
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

// --- fake XDG Desktop Portal ---

type portalCall struct {
	Method  string
	Parent  string
	Title   string
	URI     string // OpenURI
	FDPath  string // OpenFile (OpenURI interface): the file behind the descriptor
	Options map[string]dbus.Variant
	Handle  dbus.ObjectPath
}

type fakePortal struct {
	conn *dbus.Conn

	mu    sync.Mutex
	calls []portalCall
	// respond answers a FileChooser call: response code and URIs; nil
	// leaves the dialog open. The Response is sent shortly after the
	// method reply, or before it with early (only a client that
	// subscribed before calling sees it).
	respond     func(portalCall) (uint32, []string)
	early       bool
	chooser     uint32 // FileChooser version; 0: no such interface
	oldHandles  bool   // answer with a handle path of the portal's choosing
	refuseOpens bool   // OpenURI and OpenFile return an error
}

func startPortal(t *testing.T, addr string, chooserVersion uint32) *fakePortal {
	t.Helper()
	p := &fakePortal{conn: connect(t, addr), chooser: chooserVersion}
	chooser := func(method string) func(dbus.Sender, string, string, map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
		return func(sender dbus.Sender, parent, title string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
			token, _ := options["handle_token"].Value().(string)
			handle := requestPath(string(sender), token)
			if p.oldHandles {
				handle = dbus.ObjectPath(fmt.Sprintf("/org/freedesktop/portal/desktop/request/old%d", time.Now().UnixNano()))
			}
			c := portalCall{Method: method, Parent: parent, Title: title, Options: options, Handle: handle}
			p.mu.Lock()
			p.calls = append(p.calls, c)
			respond, early := p.respond, p.early
			p.mu.Unlock()
			if respond != nil {
				code, uris := respond(c)
				send := func() {
					p.conn.Emit(handle, ifaceRequest+".Response", code, map[string]dbus.Variant{"uris": dbus.MakeVariant(uris)})
				}
				if early {
					send() // before the reply: only a subscribed client sees it
				} else {
					go func() {
						time.Sleep(20 * time.Millisecond)
						send()
					}()
				}
			}
			return handle, nil
		}
	}
	if chooserVersion > 0 {
		if err := p.conn.ExportMethodTable(map[string]any{
			"OpenFile": chooser("OpenFile"),
			"SaveFile": chooser("SaveFile"),
		}, portalPath, ifaceFileChooser); err != nil {
			t.Fatal(err)
		}
	}
	refused := dbus.NewError("org.freedesktop.portal.Error.NotAllowed", []any{"refused"})
	if err := p.conn.ExportMethodTable(map[string]any{
		"OpenURI": func(parent, uri string, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.calls = append(p.calls, portalCall{Method: "OpenURI", Parent: parent, URI: uri, Options: options})
			if p.refuseOpens {
				return "", refused
			}
			return "/org/freedesktop/portal/desktop/request/x/y", nil
		},
		"OpenFile": func(parent string, fd dbus.UnixFD, options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
			target, _ := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
			unix.Close(int(fd))
			p.mu.Lock()
			defer p.mu.Unlock()
			p.calls = append(p.calls, portalCall{Method: "OpenFile", Parent: parent, FDPath: target, Options: options})
			if p.refuseOpens {
				return "", refused
			}
			return "/org/freedesktop/portal/desktop/request/x/z", nil
		},
	}, portalPath, ifaceOpenURI); err != nil {
		t.Fatal(err)
	}
	if err := p.conn.ExportMethodTable(map[string]any{
		"Get": func(iface, name string) (dbus.Variant, *dbus.Error) {
			switch {
			case name != "version":
			case iface == ifaceOpenURI:
				return dbus.MakeVariant(uint32(4)), nil
			case iface == ifaceFileChooser && p.chooser > 0:
				return dbus.MakeVariant(p.chooser), nil
			}
			return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []any{iface + "." + name})
		},
	}, portalPath, propsIface); err != nil {
		t.Fatal(err)
	}
	own(t, p.conn, portalName)
	return p
}

func (p *fakePortal) setResponder(early bool, fn func(portalCall) (uint32, []string)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.respond, p.early = fn, early
}

func (p *fakePortal) callsSoFar() []portalCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]portalCall(nil), p.calls...)
}

// --- fake notification server ---

type fakeNote struct {
	ID      uint32
	AppName string
	Summary string
	Body    string
	Actions []string
	Hints   map[string]dbus.Variant
	Closed  bool
}

type fakeNotifier struct {
	conn *dbus.Conn
	caps []string

	mu    sync.Mutex
	notes []fakeNote
}

func startNotifier(t *testing.T, addr string, caps ...string) *fakeNotifier {
	t.Helper()
	n := &fakeNotifier{conn: connect(t, addr), caps: caps}
	if err := n.conn.ExportMethodTable(map[string]any{
		"Notify": func(app string, replaces uint32, icon, summary, body string, actions []string, hints map[string]dbus.Variant, timeout int32) (uint32, *dbus.Error) {
			n.mu.Lock()
			defer n.mu.Unlock()
			id := uint32(100 + len(n.notes)) // server IDs differ from the app's
			n.notes = append(n.notes, fakeNote{ID: id, AppName: app, Summary: summary, Body: body, Actions: actions, Hints: hints})
			return id, nil
		},
		"CloseNotification": func(id uint32) *dbus.Error {
			n.mu.Lock()
			for i := range n.notes {
				if n.notes[i].ID == id {
					n.notes[i].Closed = true
				}
			}
			n.mu.Unlock()
			n.conn.Emit(notificationsPath, notificationsIface+".NotificationClosed", id, uint32(3))
			return nil
		},
		"GetCapabilities": func() ([]string, *dbus.Error) { return n.caps, nil },
		"GetServerInformation": func() (string, string, string, string, *dbus.Error) {
			return "fake", "tela", "1", "1.2", nil
		},
	}, notificationsPath, notificationsIface); err != nil {
		t.Fatal(err)
	}
	own(t, n.conn, notificationsName)
	return n
}

func (n *fakeNotifier) all() []fakeNote {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]fakeNote(nil), n.notes...)
}

func (n *fakeNotifier) emit(member string, args ...any) {
	n.conn.Emit(notificationsPath, notificationsIface+"."+member, args...)
}

// --- fake StatusNotifierWatcher ---

type registration struct {
	Service string
	Sender  string
}

type fakeWatcher struct {
	conn *dbus.Conn

	mu    sync.Mutex
	items []registration
}

func startWatcher(t *testing.T, addr string) *fakeWatcher {
	t.Helper()
	w := &fakeWatcher{conn: connect(t, addr)}
	if err := w.conn.ExportMethodTable(map[string]any{
		"RegisterStatusNotifierItem": func(sender dbus.Sender, service string) *dbus.Error {
			w.mu.Lock()
			defer w.mu.Unlock()
			w.items = append(w.items, registration{service, string(sender)})
			return nil
		},
		"RegisterStatusNotifierHost": func(service string) *dbus.Error { return nil },
	}, watcherPath, watcherIface); err != nil {
		t.Fatal(err)
	}
	own(t, w.conn, watcherName)
	return w
}

func (w *fakeWatcher) registered() []registration {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]registration(nil), w.items...)
}
