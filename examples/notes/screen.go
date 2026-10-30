package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/examples/notes/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Notes is the whole app: the main window's content and, declared inside
// it, the tray icon and the Preferences window. TitleBar is the look whose
// arrangement the (plain) title bar follows; the zero value is the
// platform's (Windows, GNOME or macOS), and the tests name one so the
// goldens match on every OS. Overview
// starts with no note open (a library that has never been used starts with
// the welcome note open).
type Notes struct {
	TitleBar kit.Variant
	Overview bool
}

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

// welcomeID is the welcome note, which a library of the first run opens.
const welcomeID = 1

// screen is the content of the main window.
type screen struct {
	TitleBar kit.Variant
	Overview bool
}

func (s screen) Build() tela.Node {
	win := tela.UseWindow()
	toast := kit.UseToast()
	lib := tela.UseOnce(func() library { return openLibrary(win.IsZero()) })
	// The first run has the welcome notes: they are the default of the
	// setting, used only while the store has no "notes" key.
	first := tela.UseOnce(func() []Note { return welcomeNotes(now()) })
	start := welcomeID
	if s.Overview {
		start = 0
	}
	notes, setNotes := desktop.UseStoreSetting(lib.store, "notes", first)
	selected, setSelected := desktop.UseStoreSetting(lib.store, "selected", start)
	flt, setFilter := tela.UseState(viewAll)
	query, setQuery := tela.UseState("")
	prefs := usePrefs()
	useAppearance(prefs)
	prefsOpen, setPrefsOpen := tela.UseState(false)
	trayOK, setTrayOK := tela.UseState(true)
	// confirming is the note the delete dialog asks about (0: none).
	confirming, setConfirming := tela.UseState(0)

	tela.UseMount(func() func() {
		if lib.err != nil {
			toast.Error("Your notes could not be read",
				kit.ToastDescription("The old file was kept next to the library as notes.json.corrupt."))
		}
		return nil
	})

	cur, hasCur := find(notes, selected)
	shown := visible(notes, flt, query)

	// --- Notes -----------------------------------------------------

	// reveal opens a note, and when the list would not have it (a filter
	// or a search that does not match it) goes back to all the notes.
	reveal := func(n Note) {
		if !flt.Match(n) || !matchesQuery(n, query) {
			setFilter.Set(viewAll)
			setQuery.Set("")
		}
		setSelected.Set(n.ID)
	}
	// blank is a note for the place the user is in: in the folder, or
	// pinned, that the rail has chosen.
	blank := func() Note {
		n := Note{ID: nextID(notes), Modified: now()}
		switch {
		case flt.Kind == kindFolder:
			n.Folder = flt.Name
		case flt == viewPinned:
			n.Pinned = true
		}
		return n
	}
	add := func(n Note) {
		setNotes.Update(func(ns []Note) []Note { return append(append([]Note(nil), ns...), n) })
		reveal(n)
	}
	newNote := func() { add(blank()) }
	// choose shows the overview of a filter: no note is open.
	choose := func(f filter) {
		setFilter.Set(f)
		setSelected.Set(0)
	}
	editTitle := func(title string) {
		setNotes.Update(func(ns []Note) []Note { return editContent(ns, cur.ID, func(n *Note) { n.Title = title }) })
	}
	editText := func(text string) {
		setNotes.Update(func(ns []Note) []Note { return editContent(ns, cur.ID, func(n *Note) { n.Text = text }) })
	}
	moveToFolder := func(name string) {
		setNotes.Update(func(ns []Note) []Note {
			name := folderNamed(ns, name)
			return edit(ns, cur.ID, func(n *Note) { n.Folder = name })
		})
	}
	pinNote := func(id int) {
		setNotes.Update(func(ns []Note) []Note { return edit(ns, id, func(n *Note) { n.Pinned = !n.Pinned }) })
	}
	togglePin := func() { pinNote(cur.ID) }

	// deleteNote removes a note from the library, selects the next one of
	// the list (or the one before it, or none: the overview) when it was
	// the open one, and offers to undo it in a toast.
	deleteNote := func(n Note) {
		wasOpen := n.ID == selected
		if next, ok := neighbor(shown, n.ID); wasOpen {
			if ok {
				setSelected.Set(next.ID)
			} else {
				setSelected.Set(0)
			}
		}
		setNotes.Update(func(ns []Note) []Note { return without(ns, n.ID) })
		toast.Show("Deleted "+n.Heading(), kit.ToastAction("Undo", func() {
			setNotes.Update(func(ns []Note) []Note { return append(append([]Note(nil), ns...), n) })
			if wasOpen {
				setSelected.Set(n.ID)
			}
		}))
	}
	// askDelete deletes a note at once, unless it has changes that are not
	// in a file or it is tied to one: then the dialog asks first.
	askDelete := func(id int) {
		n, ok := find(notes, id)
		switch {
		case !ok:
		case n.Dirty() || n.Path != "":
			setConfirming.Set(id)
		default:
			deleteNote(n)
		}
	}
	deleteOpen := func() { askDelete(cur.ID) }
	pending, _ := find(notes, confirming)

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
				reveal(n)
				return
			}
			text, err := readTextFile(path)
			if err != nil {
				toast.Error("Could not open "+filepath.Base(path), kit.ToastDescription(err.Error()))
				return
			}
			n := blank()
			n.Title, n.Text, n.Saved, n.Path = fileTitle(path), text, text, path
			add(n)
		}, desktop.Title("Open a note"), desktop.Filter("Text", "*.txt", "*.md"), desktop.Parent(win))
		if err != nil {
			dialogError(toast, "open", err)
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
			dialogError(toast, "save", err)
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
	// A hidden window still builds and runs its effects (it only draws
	// nothing), so the tray's handlers set state as usual and its menu
	// follows the window (notesTray). toggleMain reads the window's state
	// when it runs (win.State() outside a build only reads), so two quick
	// clicks act on what the window is, not on what the last build saw.

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
		text := fmt.Sprintf("Save %q to a file before quitting?", unsaved[0].Heading())
		if len(unsaved) > 1 {
			text = fmt.Sprintf("Save %q and %d more notes to files before quitting?", unsaved[0].Heading(), len(unsaved)-1)
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
		if !(e.Ctrl || e.Meta) || e.Alt {
			return
		}
		key := string(e.Key)
		if e.Shift {
			if !strings.EqualFold(string(key), "d") {
				return
			}
			key = "D"
		}
		switch key {
		case "D":
			// Ctrl+Shift+D deletes the open note. (Delete and Ctrl+Backspace
			// edit the text of the field that has the focus.)
			e.PreventDefault()
			if hasCur {
				askDelete(cur.ID)
			}
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

	var pane tela.Child
	if hasCur {
		pane = Fragment(
			List{Notes: shown, Current: cur.ID, Query: query, Previews: prefs.ShowPreviews,
				OnQuery: setQuery.Set, OnSelect: setSelected.Set, OnBack: func() { choose(flt) },
				OnPin: pinNote, OnDelete: askDelete},
			tela.WithKey(Editor{Note: cur, Prefs: prefs, Folders: folders(notes),
				OnTitle: editTitle, OnText: editText, OnFolder: moveToFolder, OnPin: togglePin, OnDelete: deleteOpen}, cur.ID),
		)
	} else {
		pane = Overview{Notes: shown, Filter: flt, Query: query, OnQuery: setQuery.Set, OnSelect: setSelected.Set,
			OnPin: pinNote, OnDelete: askDelete}
	}

	return Div(Class("flex h-screen flex-col"), shortcuts,
		notesTray{
			Toggle: toggleMain,
			New:    func() { showMain(); newNote() },
			Prefs:  showPrefs,
			Failed: func() { setTrayOK.Set(false) }, // no tray: closing quits
		},
		kit.AlertDialog{
			Open: new(confirming != 0), OnOpenChange: func(open bool) {
				if !open {
					setConfirming.Set(0)
				}
			},
			Title:       Text(fmt.Sprintf("Delete %q?", pending.Heading())),
			Description: Text(deleteWarning(pending)),
			Cancel:      Text("Keep"), Action: Text("Delete"), Destructive: true,
			OnAction: func() { deleteNote(pending) },
		},
		If(prefsOpen, tela.Component(desktop.SavedWindow{
			Key: "preferences",
			Window: tela.Window{
				Title: "Preferences", Width: 540, Height: 440, MinWidth: 460, MinHeight: 340,
				Icon: appIcon(),
				// On Windows 11 the window is Mica: the root keeps 60% of
				// its background so the material shows through (a root
				// with no background would leave a white window where
				// Mica is missing: Windows 10). Elsewhere the backdrop is
				// ignored and the root is opaque.
				Backdrop:  tela.BackdropMica,
				RootClass: glass("bg-background", "bg-background/60") + " text-foreground",
				OnClose:   func() { setPrefsOpen.Set(false) },
				Root:      Preferences{},
			},
		})),
		kit.TitleBar{
			Variant: kit.Plain,
			Look:    s.TitleBar,
			Class:   "bg-sidebar",
			Leading: Span(Class("flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"),
				Icon(icons.NotebookPen, Class("size-3.5"))),
			Title: Span(Class("font-semibold tracking-tight"), Text("Notes")),
			Trailing: Fragment(
				tool("Open", icons.FolderOpen, openFile),
				tool("Save", icons.Save, save),
			),
		},
		Div(Class("flex min-h-0 flex-1"),
			Rail{Notes: notes, Filter: flt, OnFilter: choose, OnNew: newNote, OnPrefs: showPrefs},
			Div(Class("flex min-w-0 flex-1 rounded-ss-xl border-s border-t border-border bg-background"), pane),
		),
	)
}

// deleteWarning is what the delete dialog says about a note: its file stays
// on disk, and its changes are lost.
func deleteWarning(n Note) string {
	var parts []string
	if n.Path != "" {
		parts = append(parts, "The file "+n.Path+" is not deleted: only the note leaves your library.")
	}
	if n.Dirty() {
		parts = append(parts, "Its changes, which are not saved to a file, are lost.")
	}
	return strings.Join(parts, " ")
}

// notesTray is the tray icon. A click toggles the main window, and the
// menu's second item follows it: "Show Notes" while the window is hidden
// or minimized, "Hide Notes" while it is shown. It is a component of its
// own so that only it builds again when the window's state changes (the
// state includes the focus, which changes often).
type notesTray struct {
	// Toggle shows the main window, or hides it; New makes a note and
	// Prefs opens Preferences, both showing the window; Failed runs when
	// the desktop has no tray.
	Toggle, New, Prefs, Failed func()
}

func (t notesTray) Build() tela.Node {
	st := tela.UseWindow().State()
	toggle := "Hide Notes"
	if st.Hidden || st.Minimized {
		toggle = "Show Notes"
	}
	return Fragment(desktop.Tray{
		Icon: appIcon(), Tooltip: "Notes",
		OnClick: t.Toggle,
		OnError: func(error) { t.Failed() },
		Menu: []desktop.MenuItem{
			{Label: "New note", OnSelect: t.New},
			{Label: toggle, OnSelect: t.Toggle},
			{Label: "Preferences…", OnSelect: t.Prefs},
			{Separator: true},
			{Label: "Quit", OnSelect: tela.Quit},
		},
	})
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

// dialogError tells why a file dialog did not open. A Linux desktop without
// the XDG Desktop Portal (a minimal session, a container) has no file
// dialogs at all: say what to install instead of the error.
func dialogError(toast kit.ToastHandle, what string, err error) {
	if errors.Is(err, desktop.ErrUnsupported) {
		toast.Error("This desktop has no file dialogs", kit.ToastDescription(
			"Install xdg-desktop-portal and a backend such as xdg-desktop-portal-gtk, then start Notes again."))
		return
	}
	toast.Error("Could not show the "+what+" dialog", kit.ToastDescription(err.Error()))
}
