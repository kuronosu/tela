package telatest_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// Notes sets its jump list, Dock menu or launcher tasks once, and opens
// what a task's launch names.
func ExampleApp_RunTask() {
	notes := func() tela.Node {
		opened, setOpened := tela.UseState("nothing")
		tela.UseMount(func() func() {
			_ = desktop.SetTasks([]desktop.Task{
				{Title: "New note", Args: []string{"notes:new"}},
				{Title: "Search notes", Args: []string{"notes:search"}},
			})
			return nil
		})
		desktop.UseLaunch(func(l desktop.Launch) {
			if urls := l.URLs(); len(urls) > 0 {
				setOpened.Set(urls[0])
			}
		})
		return Textf("opened %s", opened)
	}
	app := telatest.MountApp(t, desktop.SingleInstance(tela.App{Root: tela.C(notes)}))

	for _, task := range app.Tasks() {
		fmt.Println(task.Title, task.Args)
	}
	app.RunTask("Search notes")
	app.ExpectText("opened notes:search")
	fmt.Println("ok")
	// Output:
	// New note [notes:new]
	// Search notes [notes:search]
	// ok
}

// An "Open at login" switch.
func ExampleApp_StartsAtLogin() {
	settings := func() tela.Node {
		on, setOn := tela.UseState(false)
		return Button(OnClick(func() {
			if desktop.SetStartAtLogin(!on) == nil {
				setOn.Set(!on)
			}
		}), Textf("Open at login: %v", on))
	}
	app := telatest.Mount(t, tela.C(settings))

	app.Click(telatest.ByRole("button", "Open at login: false"))
	fmt.Println(app.StartsAtLogin())
	app.SetStartsAtLogin(false) // the user turns it off in the OS's settings
	on, _ := desktop.StartsAtLogin()
	fmt.Println(on)
	// Output:
	// true
	// false
}

// An editor adds each file it opens to the app's recent documents.
func ExampleApp_RecentDocuments() {
	dir, _ := os.MkdirTemp("", "telatest-recent")
	defer os.RemoveAll(dir)
	for _, name := range []string{"a.md", "b.md"} {
		_ = os.WriteFile(filepath.Join(dir, name), nil, 0o600)
	}
	editor := func() tela.Node {
		open := func(name string) func() {
			return func() { _ = desktop.AddRecentDocument(filepath.Join(dir, name)) }
		}
		return Div(
			Button(OnClick(open("a.md")), Text("Open a.md")),
			Button(OnClick(open("b.md")), Text("Open b.md")),
		)
	}
	app := telatest.Mount(t, tela.C(editor))

	app.Click(telatest.ByRole("button", "Open a.md"))
	app.Click(telatest.ByRole("button", "Open b.md"))
	for _, p := range app.RecentDocuments() {
		fmt.Println(filepath.Base(p))
	}
	_ = desktop.ClearRecentDocuments()
	fmt.Println(len(app.RecentDocuments()))
	// Output:
	// b.md
	// a.md
	// 0
}
