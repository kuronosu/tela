//go:build linux

package x11

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/kuronosu/tela/internal/platform"
)

// Live check on X11 (Xwayland works) of the AT-SPI bridge (internal/platform/internal/atspi) with
// a real window and the session's real accessibility bus and registry:
// the test reads the window's tree from the registry's desktop down to a
// button by name, as Orca does, and presses it. When the session's
// org.a11y.Status IsEnabled is false the test sets it for its duration
// and restores it (Orca sets it the same way when it starts). On Hyprland
// the window opens silently on TELA_LIVE_WORKSPACE (default 7), so it
// never takes the focus.

const liveAccessAppID = "dev.telaui.liveaccessx11"

func TestLiveAccessibility(t *testing.T) {
	b := liveBackend(t)
	session, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skipf("no session bus: %v", err)
	}
	defer session.Close()
	bus := session.Object("org.a11y.Bus", "/org/a11y/bus")
	var enabled dbus.Variant
	if err := bus.Call("org.freedesktop.DBus.Properties.Get", 0, "org.a11y.Status", "IsEnabled").Store(&enabled); err != nil {
		t.Skipf("no org.a11y.Bus on the session bus: %v", err)
	}
	if on, _ := enabled.Value().(bool); !on {
		if err := bus.Call("org.freedesktop.DBus.Properties.Set", 0, "org.a11y.Status", "IsEnabled", dbus.MakeVariant(true)).Err; err != nil {
			t.Fatalf("enable accessibility: %v", err)
		}
		t.Log("org.a11y.Status IsEnabled set to true for the test")
		defer bus.Call("org.freedesktop.DBus.Properties.Set", 0, "org.a11y.Status", "IsEnabled", dbus.MakeVariant(false))
	}
	var addr string
	if err := bus.Call("org.a11y.Bus.GetAddress", 0).Store(&addr); err != nil {
		t.Fatalf("GetAddress: %v", err)
	}
	a11y, err := dbus.Connect(addr)
	if err != nil {
		t.Fatalf("connect to the accessibility bus: %v", err)
	}
	defer a11y.Close()

	placeAccessWindow(t)
	title := fmt.Sprintf("tela live accessibility x11 %d", os.Getpid())
	win, err := b.NewWindow(platform.WindowOptions{Title: title, AppID: liveAccessAppID, Size: platform.Size{W: 240, H: 160}})
	if err != nil {
		t.Fatal(err)
	}
	w := win.(*Window)
	tree := platform.AccessUpdate{Root: 1, Focus: 1, Nodes: []platform.AccessNode{
		{ID: 1, Kind: platform.AccessRoot, Children: []platform.AccessID{2, 4}, Bounds: platform.Rect{W: 240, H: 160}},
		{ID: 2, Role: "button", Name: "Press me", States: platform.AccessFocusable, Children: []platform.AccessID{3},
			Actions: platform.AccessActionsOf(platform.AccessActionDefault, platform.AccessActionFocus),
			Bounds:  platform.Rect{X: 20, Y: 20, W: 120, H: 32}},
		{ID: 3, Kind: platform.AccessText, Text: "Press me", States: platform.AccessInline, Bounds: platform.Rect{X: 30, Y: 26, W: 80, H: 20}},
		{ID: 4, Role: "paragraph", Children: []platform.AccessID{5}, Bounds: platform.Rect{X: 20, Y: 70, W: 200, H: 20}},
		{ID: 5, Kind: platform.AccessText, Text: "Read through AT-SPI.", States: platform.AccessInline, Bounds: platform.Rect{X: 20, Y: 70, W: 160, H: 20}},
	}}

	found := make(chan string, 1) // the button's path, or "" when not found
	pressed := false
	activated := false
	time.AfterFunc(30*time.Second, b.Quit)
	err = b.Run(platform.HandlerFunc(func(_ platform.Window, e platform.Event) {
		switch e := e.(type) {
		case platform.FrameEvent:
			fb := image.NewRGBA(image.Rectangle{Max: w.Metrics().Physical()})
			fill(fb, 1, fb.Rect)
			if err := w.Present(fb, nil); err != nil {
				t.Errorf("Present: %v", err)
				b.Quit()
			}
		case platform.AccessibilityEvent:
			t.Logf("AccessibilityEvent %+v", e)
			if e.Active && !activated {
				activated = true
				w.SetAccessibility(tree)
				go func() { found <- liveFindButton(t, a11y, title, "Press me") }()
			}
		case platform.AccessActionEvent:
			t.Logf("AccessActionEvent %+v", e)
			if e.Node == 2 && e.Action == platform.AccessActionDefault {
				pressed = true
				time.AfterFunc(300*time.Millisecond, b.Quit)
			}
		}
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !activated {
		t.Fatal("the bridge never became active")
	}
	select {
	case p := <-found:
		if p == "" {
			t.Fatal("the AT did not find the button")
		}
		t.Logf("the AT found the button at %s", p)
	default:
		t.Fatal("the AT's walk did not finish")
	}
	if !pressed {
		t.Fatal("DoAction did not reach the window")
	}
}

// liveFindButton walks from the registry's desktop to the frame titled
// title and its push button named name, reads the paragraph's text, and
// presses the button. It returns the button's path ("" on failure).
func liveFindButton(t *testing.T, conn *dbus.Conn, title, name string) string {
	type ref struct {
		Name string
		Path dbus.ObjectPath
	}
	children := func(r ref) []ref {
		var kids []ref
		conn.Object(r.Name, r.Path).Call("org.a11y.atspi.Accessible.GetChildren", 0).Store(&kids)
		return kids
	}
	prop := func(r ref, p string) any {
		var v dbus.Variant
		conn.Object(r.Name, r.Path).Call("org.freedesktop.DBus.Properties.Get", 0, "org.a11y.atspi.Accessible", p).Store(&v)
		return v.Value()
	}
	role := func(r ref) uint32 {
		var role uint32
		conn.Object(r.Name, r.Path).Call("org.a11y.atspi.Accessible.GetRole", 0).Store(&role)
		return role
	}
	desktop := ref{"org.a11y.atspi.Registry", "/org/a11y/atspi/accessible/root"}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, app := range children(desktop) {
			for _, frame := range children(app) {
				if prop(frame, "Name") != title {
					continue
				}
				t.Logf("app %v (%v), frame %v", app, prop(app, "Name"), frame)
				var button ref
				for _, c := range children(frame) {
					switch role(c) {
					case 43: // push button
						if prop(c, "Name") == name {
							button = c
						}
					case 73: // paragraph
						var text string
						conn.Object(c.Name, c.Path).Call("org.a11y.atspi.Text.GetText", 0, int32(0), int32(-1)).Store(&text)
						t.Logf("paragraph text %q", text)
					}
				}
				if button.Path == "" {
					continue
				}
				var ok bool
				if err := conn.Object(button.Name, button.Path).Call("org.a11y.atspi.Action.DoAction", 0, int32(0)).Store(&ok); err != nil || !ok {
					t.Errorf("DoAction: %v %v", ok, err)
					return ""
				}
				return string(button.Path)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}

// placeAccessWindow opens the test's window silently on the hidden
// workspace on Hyprland (see placeLiveWindow), and skips the test when
// it cannot.
func placeAccessWindow(t *testing.T) {
	t.Helper()
	if os.Getenv("HYPRLAND_INSTANCE_SIGNATURE") == "" {
		return
	}
	ws := os.Getenv("TELA_LIVE_WORKSPACE")
	if ws == "" {
		ws = "7"
	}
	rule := `tela_live_access_x11 = hl.window_rule({ name = "tela-live-access-x11", match = { class = "` + liveAccessAppID + `" }, workspace = "` + ws + ` silent" })`
	if out, err := exec.Command("hyprctl", "eval", rule).CombinedOutput(); err != nil || strings.Contains(strings.ToLower(string(out)), "error") {
		// The window would take the focus on the current workspace.
		t.Skipf("hyprctl eval (window rule): %v %s; not opening a window that would take the focus", err, out)
	}
	t.Cleanup(func() {
		exec.Command("hyprctl", "eval", `if tela_live_access_x11 then tela_live_access_x11:set_enabled(false) end`).Run()
	})
}
