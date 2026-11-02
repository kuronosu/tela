package telatest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/telatest"
)

// keyLog is a focusable box that records the modifiers of its keydowns and
// their CmdOrCtrl.
func keyLog(log *[]string) func() tela.Node {
	return func() tela.Node {
		return Div(Class("size-40"), TabIndex(0), TestID("keys"), AutoFocus(true),
			OnKeyDown(func(e tela.KeyboardEvent) {
				if e.Key == "s" {
					*log = append(*log, fmt.Sprintf("ctrl=%v meta=%v cmd=%v", e.Ctrl, e.Meta, e.CmdOrCtrl()))
				}
			}))
	}
}

func take(log *[]string) string {
	s := strings.Join(*log, ",")
	*log = (*log)[:0]
	return s
}

// PlatformKeys sets what ControlOrMeta presses and what CmdOrCtrl reports;
// without it both follow the host.
func TestPlatformKeys(t *testing.T) {
	hostMac := platform.HostFlavor() == platform.FlavorMac
	for _, c := range []struct {
		name string
		opts []telatest.Option
		mac  bool
	}{
		{"host", nil, hostMac},
		{"darwin", []telatest.Option{telatest.PlatformKeys("darwin")}, true},
		{"linux", []telatest.Option{telatest.PlatformKeys("linux")}, false},
		{"windows", []telatest.Option{telatest.PlatformKeys("windows")}, false},
		{"freebsd", []telatest.Option{telatest.PlatformKeys("freebsd")}, false},
		{"last wins", []telatest.Option{telatest.PlatformKeys("windows"), telatest.PlatformKeys("darwin")}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var log []string
			app := telatest.MountFunc(t, keyLog(&log), append(c.opts, telatest.Size(200, 200))...)
			app.ExpectFocused(telatest.ByTestID("keys"))
			app.Press("ControlOrMeta+s")
			want := "ctrl=true meta=false cmd=true"
			if c.mac {
				want = "ctrl=false meta=true cmd=true"
			}
			if got := take(&log); got != want {
				t.Errorf("ControlOrMeta+s: %q, want %q", got, want)
			}
			app.Press("Control+s")
			app.Press("Meta+s")
			want = fmt.Sprintf("ctrl=true meta=false cmd=%v,ctrl=false meta=true cmd=%v", !c.mac, c.mac)
			if got := take(&log); got != want {
				t.Errorf("Control+s, Meta+s: %q, want %q", got, want)
			}
		})
	}
}

// The flavor reaches the text fields' bindings (Command+A, Option+Left on
// a Mac) and the trees of child windows, through Mount and MountApp.
func TestPlatformKeysFieldsAndWindows(t *testing.T) {
	field := func() tela.Node {
		v, setV := tela.UseState("one two three")
		return Div(Class("p-4"), Input(AriaLabel("Name"), Value(v), OnChange(func(e tela.ChangeEvent) { setV.Set(e.Value) })))
	}
	name := telatest.ByRole("textbox", "Name")
	app := telatest.MountApp(t, tela.App{Root: tela.C(field), Width: 300, Height: 200}, telatest.PlatformKeys("darwin"))
	app.Focus(name)
	app.Press("Meta+ArrowRight") // the line end
	app.Press("Alt+ArrowLeft")   // one word back
	app.Type("big ")
	app.ExpectValue(name, "one two big three")
	app.Press("Control+a") // the line start (Emacs), not select all
	app.Type(">")
	app.ExpectValue(name, ">one two big three")
	app.Press("ControlOrMeta+a")
	app.Type("x")
	app.ExpectValue(name, "x")

	var log []string
	win := func() tela.Node {
		return Div(tela.Component(tela.Window{Title: "Child", Width: 200, Height: 200, OnClose: func() {}, Root: tela.C(keyLog(&log))}))
	}
	for _, goos := range []string{"darwin", "linux"} {
		main := telatest.MountFunc(t, win, telatest.Size(200, 200), telatest.PlatformKeys(goos))
		child := main.Window("Child")
		child.ExpectFocused(telatest.ByTestID("keys"))
		child.Press("ControlOrMeta+s")
		want := "ctrl=true meta=false cmd=true"
		if goos == "darwin" {
			want = "ctrl=false meta=true cmd=true"
		}
		if got := take(&log); got != want {
			t.Errorf("%s child window: %q, want %q", goos, got, want)
		}
		main.Close()
	}
}

// Apps with different flavors run in parallel without interfering: the
// flavor belongs to each app, not to the process.
func TestPlatformKeysParallel(t *testing.T) {
	for i, goos := range []string{"darwin", "linux", "windows", "darwin", "linux", "windows"} {
		t.Run(fmt.Sprint(goos, i), func(t *testing.T) {
			t.Parallel()
			var log []string
			app := telatest.MountFunc(t, keyLog(&log), telatest.Size(200, 200), telatest.PlatformKeys(goos))
			app.ExpectFocused(telatest.ByTestID("keys"))
			want := "ctrl=true meta=false cmd=true"
			if goos == "darwin" {
				want = "ctrl=false meta=true cmd=true"
			}
			for range 20 {
				app.Press("ControlOrMeta+s")
				if got := take(&log); got != want {
					t.Fatalf("ControlOrMeta+s: %q, want %q", got, want)
				}
			}
		})
	}
}
