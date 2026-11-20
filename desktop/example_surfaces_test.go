package desktop_test

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
)

// An "Open at login" switch that shows what the desktop will do, and a
// tray app that opens no window when the desktop started it at login.
func ExampleSetStartAtLogin() {
	_ = tela.C(func() tela.Node {
		on, setOn := tela.UseState(false)
		problem, setProblem := tela.UseState("")
		tela.UseMount(func() func() {
			if v, err := desktop.StartsAtLogin(); err == nil {
				setOn.Set(v)
			}
			return nil
		})
		win := tela.UseWindow()
		desktop.UseLaunch(func(l desktop.Launch) {
			if l.AtLogin {
				win.Hide() // stay in the tray until the user asks
			}
		})
		return Div(
			Button(OnClick(func() {
				if err := desktop.SetStartAtLogin(!on); err != nil {
					setProblem.Set(err.Error())
					return
				}
				setOn.Set(!on)
			}), Textf("Open at login: %v", on)),
			Text(problem),
		)
	})
}

// The tasks of a notes app: the jump list on Windows, the Dock menu on
// macOS, the launcher's menu on Linux. A chosen task reaches the running
// app through UseLaunch.
func ExampleSetTasks() {
	app := desktop.SingleInstance(tela.App{
		AppID: "com.example.Notes",
		Root: tela.C(func() tela.Node {
			screen, setScreen := tela.UseState("list")
			tela.UseMount(func() func() {
				_ = desktop.SetTasks([]desktop.Task{
					{Title: "New note", Args: []string{"notes:new"}},
					{Title: "Search notes", Args: []string{"notes:search"}},
				})
				return nil
			})
			desktop.UseLaunch(func(l desktop.Launch) {
				for _, u := range l.URLs() {
					switch u {
					case "notes:new":
						setScreen.Set("editor")
					case "notes:search":
						setScreen.Set("search")
					}
				}
			})
			return Text(screen)
		}),
	})
	_ = app // tela.Main(app)
}

// An editor's recent files: the OS lists them beside the app, and a menu
// item clears them.
func ExampleAddRecentDocument() {
	_ = tela.C(func() tela.Node {
		open := func(path string) {
			// ... read the file ...
			_ = desktop.AddRecentDocument(path)
		}
		return Div(
			Button(OnClick(func() { open("/home/ana/notes/groceries.md") }), Text("Open groceries.md")),
			Button(OnClick(func() { _ = desktop.ClearRecentDocuments() }), Text("Clear Recent")),
		)
	})
}
