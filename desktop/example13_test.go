package desktop_test

import (
	"context"
	"errors"
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
)

// A shortcut that works from any application, and a hint when the OS
// would not bind it.
func ExampleGlobalShortcut() {
	_ = tela.C(func() tela.Node {
		win := tela.UseWindow()
		hint, setHint := tela.UseState("")
		return Div(
			desktop.GlobalShortcut{
				ID:          "new-note",
				Keys:        "Ctrl+Alt+N",
				Description: "Create a note",
				OnPress:     func() { win.Show() },
				OnStatus: func(e desktop.ShortcutStatusEvent) {
					if e.Status == desktop.ShortcutBound {
						setHint.Set("Press " + e.Trigger + " anywhere for a new note")
					} else {
						setHint.Set(e.Err.Error())
					}
				},
			},
			Text(hint),
		)
	})
}

// One window for every start of the app: a second start, from a file
// manager or a notes: link, opens its files and links in the running app.
func ExampleSingleInstance() {
	app := desktop.SingleInstance(tela.App{
		AppID: "com.example.Notes",
		Title: "Notes",
		Root: tela.C(func() tela.Node {
			open, setOpen := tela.UseState([]string(nil))
			desktop.UseLaunch(func(l desktop.Launch) {
				setOpen.Update(func(o []string) []string { return append(o, append(l.Files(), l.URLs()...)...) })
			})
			return Div(Map(open, func(p string) tela.Node { return Div(Key(p), Text(p)) }))
		}),
	})
	_ = app // tela.Main(app)
}

// Copy rich text, and paste whatever the clipboard holds.
func ExampleReadClipboard() {
	_ = tela.C(func() tela.Node {
		pasted, setPasted := tela.UseState("")
		copyNote := func() {
			var dt tela.DataTransfer
			dt.SetHTML("<b>Groceries</b>: milk, eggs")
			dt.SetText("Groceries: milk, eggs")
			_ = desktop.WriteClipboard(&dt)
		}
		paste := func() {
			_ = desktop.ReadClipboard(func(dt *tela.DataTransfer, err error) {
				switch {
				case err != nil:
				case len(dt.Files()) > 0:
					setPasted.Set(fmt.Sprintf("%d files", len(dt.Files())))
				default:
					setPasted.Set(dt.Text())
				}
			})
		}
		return Div(
			Button(OnClick(copyNote), Text("Copy")),
			Button(OnClick(paste), Text("Paste")),
			Text(pasted),
		)
	})
}

// A key kept in the OS's keyring, read off the UI thread.
func ExampleSecret() {
	_ = tela.C(func() tela.Node {
		status, setStatus := tela.UseState("unlocking…")
		tela.UseGo(func(ctx context.Context) {
			key, err := desktop.Secret("notes-key")
			switch {
			case errors.Is(err, desktop.ErrSecretNotFound):
				setStatus.Set("no key yet")
			case errors.Is(err, desktop.ErrSecretCanceled):
				setStatus.Set("the keyring stayed locked")
			case errors.Is(err, desktop.ErrUnsupported):
				setStatus.Set("locked notes need a keyring")
			case err == nil:
				setStatus.Set(fmt.Sprintf("a %d-byte key", len(key)))
			}
		}, tela.Once)
		return Text(status)
	})
}

// Register notes: links and .note files for the user, and become their
// default handler only when asked.
func ExampleRegister() {
	notes := desktop.URLScheme("notes")
	doc := desktop.FileType(".note", "application/x-notes-note", "Notes document")
	for _, a := range []desktop.Association{notes, doc} {
		if _, err := desktop.Register(a); err != nil {
			fmt.Println(err) // errors.Is(err, desktop.ErrAssociationTaken): ask before desktop.Replace()
		}
	}
	makeDefault := func() {
		r, err := desktop.MakeDefault(doc)
		if err == nil && r == desktop.DefaultAsked {
			fmt.Println("Choose Notes in the Default apps settings")
		}
	}
	_ = makeDefault
}

// A context menu: the OS's where there is one, else a drawn menu.
func ExamplePopupMenu() {
	_ = tela.C(func() tela.Node {
		drawnAt, setDrawnAt := tela.UseState[*tela.Point](nil)
		items := []desktop.MenuItem{
			{Label: "Copy", Accelerator: "Ctrl+C", OnSelect: func() {}},
			{Label: "Paste", Accelerator: "Ctrl+V", OnSelect: func() {}},
		}
		return Div(Class("h-full"),
			OnContextMenu(func(e tela.MouseEvent) {
				at := tela.Point{X: e.X, Y: e.Y}
				if err := desktop.PopupMenu(tela.WindowHandle{}, at, items, nil); errors.Is(err, desktop.ErrUnsupported) {
					setDrawnAt.Set(&at) // draw the menu at at
				}
			}),
			If(drawnAt != nil, Text("drawn menu")),
		)
	})
}

// A native menu bar, with a drawn one where windows have none.
func ExampleMenuBar() {
	_ = tela.C(func() tela.Node {
		drawn, setDrawn := tela.UseState(false)
		menus := []desktop.MenuItem{
			{Label: "File", Submenu: []desktop.MenuItem{
				{Label: "New", Accelerator: "Ctrl+N", OnSelect: func() {}},
				{Separator: true},
				{Label: "Quit", OnSelect: tela.Quit},
			}},
		}
		return Div(
			desktop.MenuBar{Menus: menus, OnError: func(error) { setDrawn.Set(true) }},
			If(drawn, Text("a drawn menu bar")),
		)
	})
}
