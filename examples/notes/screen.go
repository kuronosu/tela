package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Notes is the whole app: the main window's content and, declared inside
// it, the tray icon and the Preferences window. TitleBar is the look of
// the title bar; the zero value is the platform's (Windows, GNOME or
// macOS), and the tests name one so the goldens match on every OS.
type Notes struct{ TitleBar kit.Variant }

func (n Notes) Build() tela.Node {
	// The Toaster is the context of kit.UseToast, for the errors.
	return Fragment(kit.Toaster{Children: screen(n)})
}

// library is the store of the notes (notes.json) and what went wrong
// opening it.
type library struct {
	store *desktop.Store
	err   error
}

// openLibrary opens notes.json in the app's data folder. A file that
// cannot be read gives an error and an empty library, and the store keeps
// the unreadable file as notes.json.corrupt instead of replacing it. In
// tela snapshot, where there is no window and the folders are the
// developer's, the library is a throwaway one in a temporary folder that
// does not exist until something is typed (the store creates it at its
// first write, which a snapshot never makes).
func openLibrary(preview bool) library {
	dir, err := "", error(nil)
	if !preview {
		dir, err = desktop.DataDir()
	}
	if preview || err != nil {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("tela-notes-%d-%d", os.Getpid(), time.Now().UnixNano()))
	}
	store, err := desktop.OpenStore(filepath.Join(dir, "notes.json"))
	return library{store, err}
}

// screen is the content of the main window.
type screen struct{ TitleBar kit.Variant }

func (s screen) Build() tela.Node {
	win := tela.UseWindow()
	toast := kit.UseToast()
	lib := tela.UseOnce(func() library { return openLibrary(win.IsZero()) })
	notes, setNotes := desktop.UseStoreSetting(lib.store, "notes", welcomeNotes())
	selected, setSelected := desktop.UseStoreSetting(lib.store, "selected", 0)
	prefs := usePrefs()
	prefsOpen, setPrefsOpen := tela.UseState(false)
	trayOK, setTrayOK := tela.UseState(true)

	tela.UseMount(func() func() {
		if lib.err != nil {
			toast.Error("Your notes could not be read",
				kit.ToastDescription("The old file was kept next to the library as notes.json.corrupt."))
		}
		return nil
	})

	cur, hasCur := find(notes, selected)

	// --- Notes -----------------------------------------------------

	newNote := func() {
		id := nextID(notes)
		setNotes.Update(func(ns []Note) []Note { return append(append([]Note(nil), ns...), Note{ID: id}) })
		setSelected.Set(id)
	}
	editText := func(text string) {
		setNotes.Update(func(ns []Note) []Note { return edit(ns, cur.ID, func(n *Note) { n.Text = text }) })
	}

	// --- Files -----------------------------------------------------

	// openFile asks for a text file and reads it into a note; a file that
	// is already open in a note just selects it.
	openFile := func() {
		err := desktop.OpenFile(func(path string, err error) {
			if err != nil {
				toast.Error("Could not open the file", kit.ToastDescription(err.Error()))
				return
			}
			if path == "" {
				return // canceled
			}
			if n, ok := byPath(notes, path); ok {
				setSelected.Set(n.ID)
				return
			}
			text, err := readTextFile(path)
			if err != nil {
				toast.Error("Could not open "+filepath.Base(path), kit.ToastDescription(err.Error()))
				return
			}
			id := nextID(notes)
			setNotes.Update(func(ns []Note) []Note {
				return append(append([]Note(nil), ns...), Note{ID: id, Text: text, Saved: text, Path: path})
			})
			setSelected.Set(id)
		}, desktop.Title("Open a note"), desktop.Filter("Text", "*.txt", "*.md"), desktop.Parent(win))
		if err != nil {
			toast.Error("Could not show the open dialog", kit.ToastDescription(err.Error()))
		}
	}

	// saveNote writes n to its file, asking where first when it has none,
	// and then tells done whether it was saved (not when the dialog was
	// canceled or the file could not be written).
	saveNote := func(n Note, done func(saved bool)) {
		write := func(path string) {
			name := filepath.Base(path)
			if err := writeTextFile(path, n.Text); err != nil {
				toast.Error("Could not save "+name, kit.ToastDescription(err.Error()))
				done(false)
				return
			}
			setNotes.Update(func(ns []Note) []Note {
				return edit(ns, n.ID, func(x *Note) { x.Path, x.Saved = path, n.Text })
			})
			if prefs.NotifyOnSave {
				// A desktop without notifications gets the toast.
				if _, err := desktop.Notify(desktop.Notification{Title: "Saved " + name, Body: path}); err != nil {
					toast.Success("Saved " + name)
				}
			}
			done(true)
		}
		if n.Path != "" {
			write(n.Path)
			return
		}
		err := desktop.SaveFile(func(path string, err error) {
			switch {
			case err != nil:
				toast.Error("Could not save the note", kit.ToastDescription(err.Error()))
				done(false)
			case path == "":
				done(false) // canceled
			default:
				write(path)
			}
		}, desktop.Title("Save the note"), desktop.DefaultName(n.FileName()), desktop.Filter("Text", "*.txt", "*.md"), desktop.Parent(win))
		if err != nil {
			toast.Error("Could not show the save dialog", kit.ToastDescription(err.Error()))
			done(false)
		}
	}
	save := func() {
		if hasCur {
			saveNote(cur, func(bool) {})
		}
	}

	// --- Windows ---------------------------------------------------
	//
	// A hidden window gets no frames, so this component does not build
	// while the window is hidden: what runs then is what was captured
	// before (the tray's handlers). That is why toggleMain reads the
	// window's state when it runs (win.State() outside a build only
	// reads), instead of using what the last build saw, and why the tray
	// has one fixed "Show Notes" item.

	showMain := func() {
		win.Show()
		win.Focus()
	}
	showPrefs := func() {
		showMain()
		setPrefsOpen.Set(true)
	}
	toggleMain := func() {
		if st := win.State(); st.Hidden || st.Minimized {
			showMain()
		} else {
			win.Hide()
		}
	}

	// closeMain is App.OnClose: keep running in the tray, or quit after
	// asking about the notes that changed since they were saved.
	closeMain := func(w tela.WindowHandle) {
		if prefs.KeepInTray && trayOK {
			w.Hide()
			return
		}
		unsaved := dirty(notes)
		if len(unsaved) == 0 {
			tela.Quit()
			return
		}
		text := fmt.Sprintf("Save %q to a file before quitting?", unsaved[0].Title())
		if len(unsaved) > 1 {
			text = fmt.Sprintf("Save %q and %d more notes to files before quitting?", unsaved[0].Title(), len(unsaved)-1)
		}
		err := desktop.ShowMessage(desktop.Message{
			Kind: desktop.MessageQuestion, Title: "Unsaved changes", Buttons: desktop.ButtonsYesNoCancel, Parent: w,
			Text: text + "\n\nIf you choose No, the changes stay in your library as drafts.",
		}, func(a desktop.Answer) {
			switch a {
			case desktop.AnswerYes:
				saveAll(unsaved, saveNote, func(ok bool) {
					if ok {
						tela.Quit()
					}
				})
			case desktop.AnswerNo:
				tela.Quit()
			}
			// Cancel keeps the app running.
		})
		if err != nil {
			tela.Quit() // no message box to ask with; the drafts are kept
		}
	}
	tela.UseEffectDo(func() { gate.set(closeMain) })
	tela.UseMount(func() func() { return func() { gate.set(nil) } })

	// --- The screen ------------------------------------------------

	shortcuts := OnKeyDown(func(e tela.KeyboardEvent) {
		if !(e.Ctrl || e.Meta) || e.Alt || e.Shift {
			return
		}
		switch e.Key {
		case "s":
			e.PreventDefault()
			save()
		case "o":
			e.PreventDefault()
			openFile()
		case "n":
			e.PreventDefault()
			newNote()
		case ",":
			e.PreventDefault()
			showPrefs()
		}
	})

	return Div(Class("flex h-screen flex-col"), shortcuts,
		desktop.Tray{
			Icon: appIcon(), Tooltip: "Notes",
			OnClick: toggleMain,
			OnError: func(error) { setTrayOK.Set(false) }, // no tray: closing quits
			Menu: []desktop.MenuItem{
				{Label: "New note", OnSelect: func() { showMain(); newNote() }},
				{Label: "Show Notes", OnSelect: showMain},
				{Label: "Preferences…", OnSelect: showPrefs},
				{Separator: true},
				{Label: "Quit", OnSelect: tela.Quit},
			},
		},
		If(prefsOpen, tela.Component(desktop.SavedWindow{
			Key: "preferences",
			Window: tela.Window{
				Title: "Preferences", Width: 460, Height: 420, MinWidth: 380, MinHeight: 320,
				Icon:      appIcon(),
				RootClass: "bg-background text-foreground",
				OnClose:   func() { setPrefsOpen.Set(false) },
				Root:      Preferences{},
			},
		})),
		kit.TitleBar{
			Variant: s.TitleBar,
			Leading: Icon(icons.FileText, Class("size-4 text-amber-500")),
			Title: Span(Class("flex min-w-0 items-center gap-2"),
				Span(Class("font-semibold"), Text("Notes")),
				If(hasCur, Span(Class("truncate font-normal text-muted-foreground"), Text("— "+cur.Title()))),
				If(hasCur && cur.Dirty(), Span(Role("img"), AriaLabel("Unsaved changes"), Class("size-2 shrink-0 rounded-full bg-amber-500"))),
			),
			Trailing: Fragment(
				tool("Open", icons.FolderOpen, openFile),
				tool("Save", icons.Save, save),
				tool("Preferences", icons.Settings, showPrefs),
			),
		},
		Div(Class("flex min-h-0 flex-1"),
			Sidebar{Notes: notes, Current: cur.ID, OnSelect: setSelected.Set, OnNew: newNote},
			If(hasCur, Editor{Note: cur, Prefs: prefs, OnChange: editText}),
			If(!hasCur, Div(Class("flex flex-1 items-center justify-center text-sm text-muted-foreground"), Text("No notes yet."))),
		),
	)
}

// saveAll saves the notes one after the other (each may ask for a file
// name) and tells done whether all were saved.
func saveAll(ns []Note, save func(Note, func(bool)), done func(ok bool)) {
	if len(ns) == 0 {
		done(true)
		return
	}
	save(ns[0], func(ok bool) {
		if !ok {
			done(false)
			return
		}
		saveAll(ns[1:], save, done)
	})
}

// tool is an icon button of the title bar with a tooltip.
func tool(label string, icon tela.Icon, onClick func()) tela.Node {
	return Fragment(kit.Tooltip{
		Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icon, AriaLabel: label, OnClick: onClick},
		Content: Text(label), Placement: "bottom",
	})
}
