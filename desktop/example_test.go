package desktop_test

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
)

// An open button: the dialog answers on the UI thread, and a desktop
// without file dialogs falls back to a path field.
func ExampleOpenFile() {
	_ = tela.C(func() tela.Node {
		path, setPath := tela.UseState("")
		typing, setTyping := tela.UseState(false)
		open := func() {
			err := desktop.OpenFile(func(p string, err error) {
				if err == nil && p != "" { // "" is cancel
					setPath.Set(p)
				}
			}, desktop.Title("Open a note"), desktop.Filter("Notes", "*.md", "*.txt"))
			if errors.Is(err, desktop.ErrUnsupported) {
				setTyping.Set(true) // no portal: let the user type the path
			}
		}
		return Div(
			Button(OnClick(open), Text("Open…")),
			If(typing, Input(Bind(path, setPath))),
			Text(path),
		)
	})
}

// A question before closing a document.
func ExampleShowMessage() {
	_ = tela.C(func() tela.Node {
		win := tela.UseWindow()
		return Button(OnClick(func() {
			desktop.ShowMessage(desktop.Message{
				Kind:    desktop.MessageQuestion,
				Title:   "Unsaved changes",
				Text:    "Save the changes before closing?",
				Buttons: desktop.ButtonsYesNoCancel,
			}, func(a desktop.Answer) {
				if a != desktop.AnswerCancel {
					win.Close()
				}
			})
		}), Text("Close"))
	})
}

// A notification that brings the app back when clicked.
func ExampleNotify() {
	_ = tela.C(func() tela.Node {
		win := tela.UseWindow()
		return Button(OnClick(func() {
			_, err := desktop.Notify(desktop.Notification{
				Title:   "Export finished",
				Body:    "notes.pdf was saved in Documents.",
				OnClick: func() { win.Show() },
			})
			if err != nil {
				fmt.Println("no notifications:", err)
			}
		}), Text("Export"))
	})
}

// An app that lives in the tray: closing the window hides it, and the
// tray's menu shows it again or quits.
func ExampleTray() {
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16)), image.NewRGBA(image.Rect(0, 0, 32, 32))}
	notes := tela.C(func() tela.Node {
		win := tela.UseWindow()
		trayOK, setTrayOK := tela.UseState(true)
		return Div(
			desktop.Tray{
				Icon:    icon,
				Tooltip: "Notes",
				OnClick: func() { win.Show() },
				OnError: func(error) { setTrayOK.Set(false) },
				Menu: []desktop.MenuItem{
					{Label: "Show Notes", OnSelect: func() { win.Show() }},
					{Separator: true},
					{Label: "Quit", OnSelect: tela.Quit},
				},
			},
			If(!trayOK, Text("The tray is not available: closing the window quits.")),
		)
	})
	_ = tela.App{
		Title:   "Notes",
		Root:    notes,
		OnClose: func(w tela.WindowHandle) { w.Hide() }, // keep running in the tray
	}
}

// A setting read and written by a component.
func ExampleUseSetting() {
	_ = tela.C(func() tela.Node {
		dark, setDark := desktop.UseSetting("dark", false)
		return Button(OnClick(func() { setDark.Set(!dark) }), Textf("Dark mode: %v", dark))
	})
}

// A store of the app's own, in its state folder.
func ExampleOpenStore() {
	dir, _ := os.MkdirTemp("", "store")
	defer os.RemoveAll(dir)

	s, err := desktop.OpenStore(filepath.Join(dir, "recent.json"))
	if err != nil {
		fmt.Println(err)
		return
	}
	_ = s.Set("recent", []string{"groceries.md", "ideas.md"})
	var recent []string
	s.Get("recent", &recent)
	fmt.Println(recent)
	_ = s.Flush()
	// Output: [groceries.md ideas.md]
}

// The main window opens where the user left it.
func ExamplePersistApp() {
	app := desktop.PersistApp(tela.App{
		AppID: "com.example.notes",
		Title: "Notes", Width: 900, Height: 640,
		Root: tela.C(func() tela.Node { return Text("Notes") }),
	})
	_ = app // tela.Main(app)
}

// A tool window that remembers where it was.
func ExampleSavedWindow() {
	_ = tela.C(func() tela.Node {
		open, setOpen := tela.UseState(false)
		return Div(
			Button(OnClick(func() { setOpen.Set(true) }), Text("Inspector")),
			If(open, tela.Component(desktop.SavedWindow{
				Key: "inspector",
				Window: tela.Window{
					Title: "Inspector", Width: 360, Height: 600,
					OnClose: func() { setOpen.Set(false) },
					Root:    tela.C(func() tela.Node { return Text("Properties") }),
				},
			})),
		)
	})
}
