//go:build linux

package atspi

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// realRegistry starts at-spi2-core's registry daemon on a private bus
// (which then is the accessibility bus) and returns the bus address. The
// test is skipped when at-spi2-registryd is not installed.
func realRegistry(t *testing.T) string {
	t.Helper()
	exe := ""
	for _, p := range []string{"/usr/lib/at-spi2-registryd", "/usr/libexec/at-spi2-registryd",
		"/usr/lib/at-spi2-core/at-spi2-registryd", "/usr/lib/x86_64-linux-gnu/at-spi2-registryd"} {
		if _, err := os.Stat(p); err == nil {
			exe = p
			break
		}
	}
	if exe == "" {
		t.Skip("at-spi2-registryd is not installed")
	}
	addr := privateBus(t)
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "AT_SPI_BUS_ADDRESS="+addr, "DBUS_SESSION_BUS_ADDRESS="+addr)
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start %s: %v", exe, err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	conn := connect(t, addr)
	eventually(t, "the registry", func() bool {
		var has bool
		conn.BusObject().Call(busIface+".NameHasOwner", 0, registryName).Store(&has)
		return has
	})
	return addr
}

// TestRealRegistry embeds the bridge in at-spi2-core's own registry and
// walks from the desktop down to a button as an AT does, then presses
// it; with python3's gi Atspi installed it reads the same tree through
// libatspi, the library Orca uses.
func TestRealRegistry(t *testing.T) {
	addr := realRegistry(t)
	a := newAT(t, addr)
	// An AT listening to focus changes, as Orca registers its listeners.
	if err := a.conn.Object(registryName, registryPath).Call(registryIface+".RegisterEvent", 0,
		"object:state-changed:focused", []string{}, "").Err; err != nil {
		t.Fatalf("RegisterEvent: %v", err)
	}

	h := newHarness(t, "", map[string]string{"AT_SPI_BUS_ADDRESS": addr})
	win := &stubWin{name: "main"}
	w := h.b.NewWindow(win, "Login")
	if d := h.next("the activation"); d.e != (platform.AccessibilityEvent{Active: true}) {
		t.Fatalf("got %v", d.e)
	}
	w.Update(loginTree())
	eventually(t, "the tree", func() bool { return w.snap.Load().view.tree != nil })

	// From the desktop to the application.
	var apps []ref
	if err := a.conn.Object(registryName, desktopRootPath).Call(ifaceAccessible+".GetChildren", 0).Store(&apps); err != nil {
		t.Fatalf("desktop GetChildren: %v", err)
	}
	unique := h.b.name()
	found := false
	for _, app := range apps {
		if app.Name == unique && app.Path == appRootPath {
			found = true
		}
	}
	if !found {
		t.Fatalf("the desktop's children %v lack the app %s", apps, unique)
	}
	a.app = unique
	if p := refOf(a.prop(appRootPath, ifaceAccessible, "Parent")); p.Path != desktopRootPath {
		t.Errorf("app parent %v", p)
	}

	// Down to the button by name.
	var button dbus.ObjectPath
	var walk func(p dbus.ObjectPath, depth int)
	walk = func(p dbus.ObjectPath, depth int) {
		if depth > 10 || button != "" {
			return
		}
		if a.role(p) == rolePushButton && a.name(p) == "Sign in" {
			button = p
			return
		}
		for _, c := range a.children(p) {
			walk(c.Path, depth+1)
		}
	}
	walk(appRootPath, 0)
	if button == "" {
		t.Fatal("no button \"Sign in\" under the app")
	}
	var ok bool
	a.must(button, ifaceAction+".DoAction", []any{int32(0)}, &ok)
	if d := h.next("the press"); !ok || d.e != (platform.AccessActionEvent{Node: 6, Action: platform.AccessActionDefault}) {
		t.Errorf("DoAction %v, got %v", ok, d.e)
	}

	// The registry told the bridge about the listener: focus events go
	// out, the others do not.
	a.drain()
	u := loginTree()
	u.Nodes = u.Nodes[7:]
	u.Nodes[0].Checked = platform.ToggleTrue
	u.Nodes[0].States |= platform.AccessFocused
	u.Focus = 8
	w.Update(u)
	evs := a.waitEvent("focused", func(e event) bool { return e.name == "StateChanged" && e.detail == "focused" })
	for _, e := range evs {
		if e.detail == "checked" {
			t.Errorf("state-changed:checked went out without a listener: %v", evs)
		}
	}

	libatspi(t, addr, "tela-test")
	libatspiEvents(t, addr, w)

	// Close unembeds: the desktop loses the app.
	h.b.Close()
	eventually(t, "the app removed from the desktop", func() bool {
		var apps []ref
		a.conn.Object(registryName, desktopRootPath).Call(ifaceAccessible+".GetChildren", 0).Store(&apps)
		for _, app := range apps {
			if app.Name == unique {
				return false
			}
		}
		return true
	})
}

// libatspiScript walks the desktop with libatspi and prints, for the app
// named argv[1], one line per object: depth, role name, name,
// and the text of the Text interface.
const libatspiScript = `
import sys, gi
gi.require_version('Atspi', '2.0')
from gi.repository import Atspi
desk = Atspi.get_desktop(0)
for i in range(desk.get_child_count()):
    app = desk.get_child_at_index(i)
    if app is None or app.get_name() != sys.argv[1]:
        continue
    def walk(o, d):
        text = ''
        t = o.get_text_iface()
        if t is not None:
            text = Atspi.Text.get_text(t, 0, -1)
        states = o.get_state_set()
        print('%d|%s|%s|%s|%s' % (d, o.get_role_name(), o.get_name(), text,
            'focused' if states.contains(Atspi.StateType.FOCUSED) else ''))
        for j in range(o.get_child_count()):
            walk(o.get_child_at_index(j), d + 1)
    walk(app, 0)
`

// libatspi checks the tree as libatspi reads it.
func libatspi(t *testing.T, addr, appName string) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Log("python3 is not installed: libatspi not checked")
		return
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "walk.py")
	if err := os.WriteFile(script, []byte(libatspiScript), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, py, "-I", script, appName)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AT_SPI_BUS_ADDRESS="+addr, "DBUS_SESSION_BUS_ADDRESS="+addr)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "ValueError") || strings.Contains(string(out), "ModuleNotFoundError") {
			t.Logf("python3 has no gi Atspi: libatspi not checked (%s)", strings.TrimSpace(string(out)))
			return
		}
		t.Fatalf("libatspi walk: %v\n%s", err, out)
	}
	got := strings.TrimSpace(string(out))
	want := strings.Join([]string{
		"0|application|tela-test||",
		"1|frame|Login||",
		"2|heading|Welcome|Welcome|",
		"2|entry|Email||",
		"2|button|Sign in||",
		"2|check box|Remember me||focused",
	}, "\n")
	if got != want {
		t.Errorf("libatspi reads\n%s\nwant\n%s", got, want)
	}
}

// libatspiEventsScript prints the events libatspi delivers for the app
// named argv[1]: type, the source's name, detail1 and the data.
const libatspiEventsScript = `
import sys, gi
gi.require_version('Atspi', '2.0')
from gi.repository import Atspi, GLib
def cb(e):
    try:
        app = e.source.get_application()
        if app is None or app.get_name() != sys.argv[1]:
            return
        data = e.any_data if isinstance(e.any_data, str) else ''
        print('%s|%s|%d|%s' % (e.type, e.source.get_name(), e.detail1, data), flush=True)
    except Exception as ex:
        print('error|%s' % ex, flush=True)
l = Atspi.EventListener.new(cb)
for ev in ('object:state-changed:focused', 'object:state-changed:checked', 'object:announcement',
           'object:property-change:accessible-name', 'object:children-changed'):
    l.register(ev)
GLib.timeout_add(200, lambda: print('ready', flush=True) and False)
GLib.timeout_add(8000, lambda: Atspi.event_quit() and False)
Atspi.event_main()
`

// libatspiEvents checks that libatspi parses the bridge's events: a
// change of focus, a state, a name and an announcement.
func libatspiEvents(t *testing.T, addr string, w *Window) {
	py, err := exec.LookPath("python3")
	if err != nil {
		return
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "events.py")
	if err := os.WriteFile(script, []byte(libatspiEventsScript), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, py, "-I", script, "tela-test")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AT_SPI_BUS_ADDRESS="+addr, "DBUS_SESSION_BUS_ADDRESS="+addr)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	lines := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	select {
	case l := <-lines:
		if l != "ready" {
			t.Logf("libatspi events not checked: %q", l)
			return
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the libatspi listener did not start")
	}
	// The registry tells the bridge about the new listeners first.
	time.Sleep(300 * time.Millisecond)
	u := loginTree()
	u.Nodes[5].Name = "Signing in"
	u.Nodes[7].Checked = platform.ToggleFalse
	u.Nodes[4].States |= platform.AccessFocused
	u.Focus = 5
	u.Nodes = append(u.Nodes[4:6:6], u.Nodes[7])
	u.Announcements = []platform.Announcement{{Text: "Signed in", Assertive: true}}
	w.Update(u)
	want := []string{
		"object:state-changed:checked|Remember me|0|",
		"object:property-change:accessible-name|Signing in|0|Signing in",
		"object:state-changed:focused|Email|1|",
		"object:announcement|Login|2|Signed in",
	}
	var got []string
	for !slicesContains(got, want[len(want)-1]) {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("libatspi delivered %q, want %q", got, want)
			}
			got = append(got, l)
		case <-time.After(10 * time.Second):
			t.Fatalf("libatspi delivered %q, want %q", got, want)
		}
	}
	for _, wl := range want {
		if !slicesContains(got, wl) {
			t.Errorf("libatspi delivered %q, lacking %q", got, wl)
		}
	}
	cmd.Process.Kill()
}

func slicesContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
