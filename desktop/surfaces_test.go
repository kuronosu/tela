package desktop_test

import (
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The desktop surfaces (ADR-298) in telatest: start at login, the tasks
// and the recent documents are recorded by the headless desktop, a task
// arrives through UseLaunch as a forwarded launch, and the failures reach
// the app.

func TestStartAtLogin(t *testing.T) {
	desktop.SetAppIDForTest(t, "com.example.surfaces")
	results := make(chan error, 1)
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(false)
		tela.UseMount(func() func() {
			v, err := desktop.StartsAtLogin()
			if err == nil {
				setOn.Set(v)
			}
			return nil
		})
		return Div(
			Textf("at login: %v", on),
			Button(OnClick(func() {
				err := desktop.SetStartAtLogin(!on)
				if err == nil {
					setOn.Set(!on)
				}
				results <- err
			}), Text("Toggle")),
		)
	})
	app.ExpectText("at login: false")
	app.Click(telatest.ByRole("button", "Toggle"))
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	app.ExpectText("at login: true")
	if !app.StartsAtLogin() {
		t.Error("App.StartsAtLogin is false after SetStartAtLogin(true)")
	}
	app.SetStartsAtLogin(false)
	if on, err := desktop.StartsAtLogin(); on || err != nil {
		t.Errorf("StartsAtLogin after the user turned it off = %v, %v", on, err)
	}

	app.Unsupported(telatest.StartAtLogin)
	app.Click(telatest.ByRole("button", "Toggle"))
	if err := <-results; !errors.Is(err, desktop.ErrUnsupported) || !strings.Contains(err.Error(), "SetStartAtLogin(false)") {
		t.Errorf("SetStartAtLogin without the service = %v, want ErrUnsupported", err)
	}
	app.Unsupported()
	boom := errors.New("access denied")
	app.FailSurfaces(boom)
	app.Click(telatest.ByRole("button", "Toggle"))
	if err := <-results; !errors.Is(err, boom) {
		t.Errorf("SetStartAtLogin with FailSurfaces = %v", err)
	}
}

func TestTasks(t *testing.T) {
	desktop.SetAppIDForTest(t, "com.example.surfaces")
	var launches []desktop.Launch
	icon := []image.Image{image.NewNRGBA(image.Rect(0, 0, 16, 16))}
	app := telatest.MountApp(t, desktop.SingleInstance(tela.App{
		AppID: "com.example.surfaces",
		Root: tela.C(func() tela.Node {
			last, setLast := tela.UseState("")
			tela.UseMount(func() func() {
				if err := desktop.SetTasks([]desktop.Task{
					{Title: "New note", Args: []string{"notes:new"}, Icon: icon, Description: "An empty note"},
					{Title: "Search", Args: []string{"--search"}},
				}); err != nil {
					setLast.Set(err.Error())
				}
				return nil
			})
			desktop.UseLaunch(func(l desktop.Launch) {
				launches = append(launches, l)
				setLast.Set(strings.Join(l.Args, " "))
			})
			return Textf("last: %s", last)
		}),
	}))
	got := app.Tasks()
	if len(got) != 2 || got[0].Title != "New note" || got[0].Description != "An empty note" || len(got[0].Icon) != 1 ||
		!slices.Equal(got[1].Args, []string{"--search"}) {
		t.Fatalf("Tasks = %+v", got)
	}
	app.RunTask("New note")
	app.ExpectText("last: notes:new")
	if len(launches) != 1 || launches[0].Source != desktop.LaunchForwarded || launches[0].AtLogin || !slices.Equal(launches[0].URLs(), []string{"notes:new"}) {
		t.Errorf("the task's launch = %+v", launches)
	}
	app.RunTask("Search")
	app.ExpectText("last: --search")

	if err := desktop.SetTasks(nil); err != nil || app.Tasks() != nil {
		t.Errorf("SetTasks(nil) = %v, left %+v", err, app.Tasks())
	}
	func() {
		defer func() {
			if p := recover(); p == nil || !strings.HasPrefix(p.(string), "desktop.SetTasks: tasks[0] has no Title") {
				t.Errorf("a task without a Title panicked with %v", p)
			}
		}()
		_ = desktop.SetTasks([]desktop.Task{{Args: []string{"x"}}})
	}()
	app.Unsupported(telatest.Tasks)
	if err := desktop.SetTasks([]desktop.Task{{Title: "x"}}); !errors.Is(err, desktop.ErrUnsupported) {
		t.Errorf("SetTasks without the service = %v", err)
	}
}

func TestRecentDocuments(t *testing.T) {
	desktop.SetAppIDForTest(t, "com.example.surfaces")
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	for _, p := range []string{a, b} {
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := telatest.MountFunc(t, func() tela.Node { return Text("editor") })
	for _, p := range []string{a, b, a} {
		if err := desktop.AddRecentDocument(p); err != nil {
			t.Fatal(err)
		}
	}
	if got := app.RecentDocuments(); !slices.Equal(got, []string{a, b}) {
		t.Errorf("RecentDocuments = %v, want [a b], most recent first", got)
	}
	// A relative path is the working directory's.
	t.Chdir(dir)
	if err := desktop.AddRecentDocument("b.md"); err != nil {
		t.Fatal(err)
	}
	if got := app.RecentDocuments(); len(got) == 0 || got[0] != b {
		t.Errorf("a relative path was recorded as %v", got)
	}
	if err := desktop.AddRecentDocument("missing.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a missing file = %v, want fs.ErrNotExist", err)
	}
	if err := desktop.ClearRecentDocuments(); err != nil || app.RecentDocuments() != nil {
		t.Errorf("ClearRecentDocuments = %v, left %v", err, app.RecentDocuments())
	}
}

func TestSurfacesWithoutApp(t *testing.T) {
	if err := desktop.SetStartAtLogin(true); err == nil || !strings.Contains(err.Error(), "running app") {
		t.Errorf("SetStartAtLogin without an app = %v", err)
	}
	if err := desktop.AddRecentDocument("/x"); err == nil {
		t.Error("AddRecentDocument without an app succeeded")
	}
}

func TestLaunchAtLogin(t *testing.T) {
	desktop.SetStartedAtLoginForTest(t, true)
	desktop.SetStartLaunchForTest(t, t.TempDir())
	var got []desktop.Launch
	app := telatest.MountFunc(t, func() tela.Node {
		desktop.UseLaunch(func(l desktop.Launch) { got = append(got, l) })
		return Text("tray app")
	})
	app.InjectLaunch(desktop.Launch{Args: []string{"x"}})
	if len(got) != 2 || !got[0].AtLogin || got[0].Source != desktop.LaunchStart || got[1].AtLogin {
		t.Errorf("launches %+v: want the start AtLogin, the forwarded one not", got)
	}
}
