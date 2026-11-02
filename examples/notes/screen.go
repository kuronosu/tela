package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
// it, the tray icon and the Preferences window. Look is the platform whose
// conventions the app follows: the arrangement of the (plain) title bars,
// the Mica glass of Preferences on Windows and the Cmd key on macOS. The
// zero value is the platform the app runs on (Windows, GNOME or macOS), and
// the tests name one so the goldens match on every OS. Overview
// starts with no note open (a library that has never been used starts with
// the welcome note open).
type Notes struct {
	Look     kit.Variant
	Overview bool
}

func (n Notes) Build() tela.Node {
	// The Toaster is the context of kit.UseToast, for the errors.
	return kit.Toaster{Children: screen(n)}
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
	Look     kit.Variant
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
	// The locked notes (lock.go): vlt holds the key and the content of the
	// notes that are unlocked, and lives on the UI thread; bump builds the
	// screen again after it changes. busy is true while the keyring is
	// being asked, noKeyring that the desktop has none, keyGone that it
	// has lost the key.
	vlt := tela.UseOnce(func() *vault { return &vault{} })
	_, setRev := tela.UseState(0)
	bump := func() { setRev.Update(func(n int) int { return n + 1 }) }
	busy, setBusy := tela.UseState(false)
	noKeyring, setNoKeyring := tela.UseState(false)
	keyGone, setKeyGone := tela.UseState(false)
	// trashAsk is the file of a note that was just locked, which a dialog
	// offers to move to the Trash (zero: no dialog). noTrash is true once
	// the desktop said it cannot move a file there (ErrUnsupported): the
	// dialog then only says where the file is.
	trashAsk, setTrashAsk := tela.UseState(trashOffer{})
	noTrash, setNoTrash := tela.UseState(false)
	// quickInfo is what the OS said about the global shortcut, for
	// Preferences.
	quickInfo, setQuickInfo := tela.UseState(quickNoteInfo{})
	// latest is the library of the latest build, for what finishes later.
	latest := tela.UseRef(notes)
	latest.Current = notes

	tela.UseMount(func() func() {
		// A library of an earlier build may hold a locked note with the
		// path of its file in the clear: drop it (lock.go).
		if _, found := detachLocked(latest.Current); found {
			setNotes.Update(func(ns []Note) []Note {
				out, _ := detachLocked(ns)
				return out
			})
		}
		if lib.err != nil {
			toast.Error("Your notes could not be read",
				kit.ToastDescription("The old file was kept next to the library as notes.json.corrupt."))
		}
		return nil
	})

	// all is the library as the screen shows it: the notes that are unlocked
	// have their content back.
	all := vlt.views(notes)
	cur, hasCur := find(all, selected)
	shown := visible(all, flt, query)

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
	// freshID is an ID no note has, nor any that was handed out before it
	// and is still on its way into the library (a launch can make several
	// notes before the screen builds again).
	issued := tela.UseRef(0)
	freshID := func() int {
		id := max(nextID(latest.Current), issued.Current+1)
		issued.Current = id
		return id
	}
	// place is a note without an ID for the place the user is in: in the
	// folder, or pinned, that the rail has chosen.
	place := func() Note {
		n := Note{Modified: now()}
		switch {
		case flt.Kind == kindFolder:
			n.Folder = flt.Name
		case flt == viewPinned:
			n.Pinned = true
		}
		return n
	}
	blank := func() Note {
		n := place()
		n.ID = freshID()
		return n
	}
	add := func(n Note) {
		setNotes.Update(func(ns []Note) []Note { return append(append([]Note(nil), ns...), n) })
		reveal(n)
	}
	newNote := func() { add(blank()) }
	// addText makes a note of text that came from outside (dropped, or in
	// a link): like a note that was typed, it is not in a file.
	addText := func(title, text string) {
		n := blank()
		n.Title, n.Text = title, text
		add(n)
	}
	// openFiles makes notes of the text files at paths, as Open does: one
	// already open selects its note, and a file that cannot be used says
	// why, without stopping the others. Unless anyFile (the user chose or
	// named the files), only .txt and .md files are taken.
	openFiles := func(paths []string, anyFile bool) {
		in := importFiles(all, paths, place(), freshID, anyFile)
		if len(in.Fresh) > 0 {
			setNotes.Update(func(ns []Note) []Note { return append(append([]Note(nil), ns...), in.Fresh...) })
		}
		if n, ok := find(in.Fresh, in.Open); ok {
			reveal(n)
		} else if n, ok := find(all, in.Open); ok {
			reveal(n)
		}
		if len(in.Problems) > 0 {
			p := in.Problems[0]
			reason := p.Reason
			if more := len(in.Problems) - 1; more > 0 {
				reason += fmt.Sprintf(" %s more could not be opened either.", plural(more, "file"))
			}
			toast.Error("Could not open "+p.Name, kit.ToastDescription(reason))
		}
		if len(in.Fresh) > 1 {
			toast.Show("Opened " + plural(len(in.Fresh), "note"))
		}
	}
	// Every start of the program is a launch: its own, and the ones that a
	// second process hands over before it exits (desktop.SingleInstance).
	// Files open as notes, and notes: links make or show a note (link.go).
	// The window is raised by SingleInstance.
	desktop.UseLaunch(func(l desktop.Launch) {
		if files := l.Files(); len(files) > 0 {
			openFiles(files, true)
		}
		for _, raw := range l.URLs() {
			k, err := parseLink(raw)
			switch {
			case errors.Is(err, errNotOurs):
			case err != nil:
				toast.Error("Could not open the link", kit.ToastDescription(err.Error()))
			case k.Action == "new":
				addText(k.Title, k.Text)
			default:
				if n, ok := find(all, k.ID); ok {
					reveal(n)
				} else {
					toast.Error(fmt.Sprintf("There is no note %d", k.ID), kit.ToastDescription("The link is for a note that is not in this library."))
				}
			}
		}
	})
	// Notes makes itself the handler of notes: links (never the default)
	// when it starts, and when the user turns the setting on again
	// (link.go).
	wantLinks := prefs.RegisterLinks
	tela.UseGo(func(ctx context.Context) { registerAtStart(wantLinks) }, wantLinks)
	// copyLink puts the link of a note on the clipboard.
	copyLink := func(id int) {
		if err := desktop.WriteClipboardText(fmt.Sprintf("notes://open?id=%d", id)); err != nil {
			toast.Error("Could not copy the link", kit.ToastDescription(err.Error()))
			return
		}
		toast.Show("Link copied")
	}
	// The window takes drops of files and text (dropped.go).
	drop := useDrop(func(dt *tela.DataTransfer) {
		if files := dt.Files(); len(files) > 0 {
			openFiles(files, false)
		} else if text := dt.Text(); strings.TrimSpace(text) != "" {
			addText("", text)
		}
	})
	// choose shows the overview of a filter: no note is open.
	choose := func(f filter) {
		setFilter.Set(f)
		setSelected.Set(0)
	}
	// patch changes the content of a note (its Title, Text and Saved). A
	// locked note that is unlocked is changed in the vault and sealed again
	// for the library; touch dates the change now.
	patch := func(id int, touch bool, f func(*Note)) {
		if vlt.isOpen(id) {
			sealed, err := vlt.change(id, f)
			if err != nil {
				toast.Error("Could not encrypt the note", kit.ToastDescription(err.Error()))
				return
			}
			at := now()
			setNotes.Update(func(ns []Note) []Note {
				return edit(ns, id, func(n *Note) {
					n.Sealed = sealed
					if touch {
						n.Modified = at
					}
				})
			})
			bump()
			return
		}
		if n, ok := find(latest.Current, id); ok && n.Locked {
			return // locked: there is no content to change
		}
		if touch {
			setNotes.Update(func(ns []Note) []Note { return editContent(ns, id, f) })
		} else {
			setNotes.Update(func(ns []Note) []Note { return edit(ns, id, f) })
		}
	}
	editTitle := func(title string) { patch(cur.ID, true, func(n *Note) { n.Title = title }) }
	editText := func(text string) { patch(cur.ID, true, func(n *Note) { n.Text = text }) }
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
		name := n.Heading()
		if stored, ok := find(latest.Current, n.ID); ok {
			n = stored // the locked form: an undo never brings the content back in the clear
		}
		vlt.lock(n.ID)
		wasOpen := n.ID == selected
		if next, ok := neighbor(shown, n.ID); wasOpen {
			if ok {
				setSelected.Set(next.ID)
			} else {
				setSelected.Set(0)
			}
		}
		setNotes.Update(func(ns []Note) []Note { return without(ns, n.ID) })
		toast.Show("Deleted "+name, kit.ToastAction("Undo", func() {
			setNotes.Update(func(ns []Note) []Note { return append(append([]Note(nil), ns...), n) })
			if wasOpen {
				setSelected.Set(n.ID)
			}
		}))
	}
	// askDelete deletes a note at once, unless it has changes that are not
	// in a file or it is tied to one: then the dialog asks first.
	askDelete := func(id int) {
		n, ok := find(all, id)
		switch {
		case !ok:
		case n.Dirty() || n.Path != "":
			setConfirming.Set(id)
		default:
			deleteNote(n)
		}
	}
	deleteOpen := func() { askDelete(cur.ID) }
	pending, _ := find(all, confirming)

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
			if n, ok := byPath(all, path); ok {
				reveal(n)
				return
			}
			openFiles([]string{path}, true)
		}, desktop.Title("Open a note"), desktop.Filter("Text", "*.txt", "*.md"), desktop.Parent(win))
		if err != nil {
			dialogError(toast, "open", err)
		}
	}

	// saveNote writes n to its file, asking where first when it has none,
	// and then tells done whether it was saved (not when the dialog was
	// canceled or the file could not be written).
	saveNote := func(n Note, done func(saved bool)) {
		if n.Locked {
			// Never in a file: see lock.go. (Quitting never asks about a
			// locked note, which is never Dirty.)
			done(false)
			return
		}
		write := func(path string) {
			name := filepath.Base(path)
			if err := writeTextFile(path, n.Text); err != nil {
				toast.Error("Could not save "+name, kit.ToastDescription(err.Error()))
				done(false)
				return
			}
			setNotes.Update(func(ns []Note) []Note { return edit(ns, n.ID, func(x *Note) { x.Path = path }) })
			patch(n.ID, false, func(x *Note) { x.Saved = n.Text })
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
		switch {
		case !hasCur:
		case cur.Locked:
			// The Save button is off for a locked note; Ctrl+S says why.
			toast.Show(lockedSaveTitle, kit.ToastDescription(lockedSaveHint))
		default:
			saveNote(cur, func(bool) {})
		}
	}

	// --- Locked notes ----------------------------------------------
	//
	// The keyring can ask the user to unlock it and wait, so the key is
	// read off the UI thread, in a goroutine that gives the outcome back
	// with tela.Post. busy keeps a second request from starting meanwhile.
	// What finishes later reads the library from latest, not from this
	// build.

	// keyringProblem tells the user why the keyring did not help.
	keyringProblem := func(err error) {
		switch {
		case errors.Is(err, desktop.ErrSecretCanceled):
			// The user dismissed the prompt: nothing to report but that
			// nothing happened.
			toast.Show("The keyring stayed locked", kit.ToastDescription("Nothing changed."))
		case errors.Is(err, desktop.ErrSecretNotFound):
			setKeyGone.Set(true)
			toast.Error("The key for your locked notes is gone",
				kit.ToastDescription("Their encrypted text is still in your library, but it cannot be read without that key. Nothing was deleted."))
		case errors.Is(err, desktop.ErrUnsupported):
			setNoKeyring.Set(true)
			toast.Error("This desktop has no system keyring", kit.ToastDescription(noKeyringHint))
		default:
			toast.Error("Could not use the keyring", kit.ToastDescription(err.Error()))
		}
	}
	lockHint := ""
	if noKeyring {
		lockHint = noKeyringTip
	}

	// finishLock seals the note with the key; it runs on the UI thread
	// when the keyring answered.
	finishLock := func(id int, key []byte, err error) {
		if err != nil {
			keyringProblem(err)
			return
		}
		n, ok := find(latest.Current, id)
		if !ok || n.Locked {
			return
		}
		sealed, err := seal(key, id, content{n.Title, n.Text, n.Saved})
		if err != nil {
			toast.Error("Could not lock the note", kit.ToastDescription(err.Error()))
			return
		}
		setKeyGone.Set(false)
		// lockedForm detaches the note from its file, whose name could
		// say what the note is about; the file itself still has the text,
		// so the user is asked about it.
		setNotes.Update(func(ns []Note) []Note {
			return edit(ns, id, func(x *Note) { *x = lockedForm(*x, sealed) })
		})
		if n.Path != "" {
			setTrashAsk.Set(trashOffer{Path: n.Path, Heading: n.Heading()})
		}
	}
	// trashFile moves the file of a note that was locked to the Trash, as
	// the user asked in the dialog. desktop.MoveToTrash blocks, so it runs
	// in a goroutine that gives the outcome back with tela.Post. It never
	// deletes: a desktop that cannot move the file to the Trash leaves it,
	// and the dialog says where it is.
	trashFile := func(o trashOffer) {
		name, folder := splitPath(o.Path)
		go func() {
			err := desktop.MoveToTrash(o.Path)
			tela.Post(func() {
				switch {
				case err == nil:
					toast.Show("Moved "+name+" to the Trash", kit.ToastDescription("You can restore it from there."))
				case errors.Is(err, desktop.ErrUnsupported):
					setNoTrash.Set(true)
					setTrashAsk.Set(o)
				case errors.Is(err, fs.ErrNotExist):
					toast.Show(name+" was already gone", kit.ToastDescription("There is no file "+name+" in "+folder+" any more."))
				default:
					toast.Error("Could not move "+name+" to the Trash",
						kit.ToastDescription(err.Error()+". The file is still in "+folder+"."))
				}
			})
		}()
	}
	// finishUnlock decrypts the note with the key and keeps it unlocked.
	finishUnlock := func(id int, key []byte, err error) {
		if err != nil {
			keyringProblem(err)
			return
		}
		n, ok := find(latest.Current, id)
		if !ok || !n.Locked {
			return
		}
		c, err := unseal(key, id, n.Sealed)
		if err != nil {
			toast.Error("Could not unlock the note", kit.ToastDescription(err.Error()+". Its encrypted text is still in your library."))
			return
		}
		setKeyGone.Set(false)
		vlt.unlock(id, key, c)
		bump()
	}
	// askKey reads the key in a goroutine (making one when create) and
	// gives it to finish on the UI thread.
	askKey := func(create bool, finish func(key []byte, err error)) {
		if busy {
			return
		}
		setBusy.Set(true)
		have := vlt.key
		go func() {
			key, err := have, error(nil)
			if key == nil {
				key, err = fetchKey(create)
			}
			tela.Post(func() {
				setBusy.Set(false)
				finish(key, err)
			})
		}()
	}
	// toggleLock is the lock button and the first item of a card's lock
	// menu: it locks a note (the first time making the key), locks an
	// unlocked one again, and unlocks a locked one.
	toggleLock := func(id int) {
		n, ok := find(latest.Current, id)
		switch {
		case !ok:
		case n.Locked && vlt.isOpen(id):
			vlt.lock(id)
			bump()
		case n.Locked:
			askKey(false, func(key []byte, err error) { finishUnlock(id, key, err) })
		default:
			// Only the first lock makes a key: with locked notes in the
			// library and no key, a new one would not open them.
			askKey(!hasLocked(latest.Current), func(key []byte, err error) { finishLock(id, key, err) })
		}
	}
	// removeLock stores an unlocked note in the clear again.
	removeLock := func(id int) {
		c, ok := vlt.open[id]
		if !ok {
			return
		}
		setNotes.Update(func(ns []Note) []Note {
			return edit(ns, id, func(x *Note) { *x = plainForm(*x, c) })
		})
		vlt.lock(id)
		bump()
	}
	locks := Locks{Lock: toggleLock, RemoveLock: removeLock, Hint: lockHint, Busy: busy}

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
		unsaved := dirty(all)
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
				OnPin: pinNote, OnDelete: askDelete, OnCopyLink: copyLink, Locks: locks},
			// The editor is new for a note that is unlocked or locked, so
			// its fields start in the right place.
			tela.WithKey(Editor{Note: cur, Prefs: prefs, Folders: folders(all),
				OnTitle: editTitle, OnText: editText, OnFolder: moveToFolder, OnPin: togglePin, OnDelete: deleteOpen,
				OnLock: func() { toggleLock(cur.ID) }, LockHint: lockHint, Busy: busy, KeyGone: keyGone},
				editorKey{cur.ID, cur.Hidden()}),
		)
	} else {
		pane = Overview{Notes: shown, Filter: flt, Query: query, OnQuery: setQuery.Set, OnSelect: setSelected.Set,
			OnPin: pinNote, OnDelete: askDelete, OnCopyLink: copyLink, Locks: locks}
	}

	return quickNoteCtx.Provider(quickInfo, Div(Class("flex h-screen flex-col"), shortcuts, drop.Props,
		// The global shortcut that makes a note from any application, and
		// raises the window to show it.
		If(prefs.QuickNote, tela.Component(desktop.GlobalShortcut{
			ID: quickNoteID, Keys: quickNoteKeys, Description: "Make a note in Notes",
			OnPress:  func() { showMain(); newNote() },
			OnStatus: func(e desktop.ShortcutStatusEvent) { setQuickInfo.Set(infoOf(e)) },
		})),
		// Hiding the window locks the notes again.
		lockGuard{OnHidden: func() {
			vlt.clear()
			bump()
		}},
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
		// After locking a note that had a file: move the file to the
		// Trash? Only on the user's word, and never deleting it.
		kit.AlertDialog{
			Open: new(trashAsk.Path != "" && !noTrash), OnOpenChange: func(open bool) {
				if !open {
					setTrashAsk.Set(trashOffer{})
				}
			},
			Title:       Text("Move the file to the Trash?"),
			Description: Text(trashAsk.question()),
			Cancel:      Text("Keep file"), Action: Text("Move to Trash"), Destructive: true,
			OnAction: func() { trashFile(trashAsk) },
		},
		// The same, on a desktop that cannot move it: where the file is.
		kit.Dialog{
			Open: new(trashAsk.Path != "" && noTrash), OnOpenChange: func(open bool) {
				if !open {
					setTrashAsk.Set(trashOffer{})
				}
			},
			Title:       Text("The file still holds the text"),
			Description: Text(trashAsk.notice()),
			HideClose:   true,
			Footer: Div(Class("flex justify-end"),
				kit.Button{Children: Text("OK"), OnClick: func() { setTrashAsk.Set(trashOffer{}) }}),
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
				RootClass: glass(s.Look, "bg-background", "bg-background/60") + " text-foreground",
				OnClose:   func() { setPrefsOpen.Set(false) },
				Root:      Preferences{Look: s.Look},
			},
		})),
		kit.TitleBar{
			Variant: kit.Plain,
			Look:    s.Look,
			Class:   "bg-sidebar",
			Leading: Span(Class("flex size-6 items-center justify-center rounded-md bg-primary text-primary-foreground"),
				Icon(icons.NotebookPen, Class("size-3.5"))),
			Title: Span(Class("font-semibold tracking-tight"), Text("Notes")),
			Trailing: Fragment(
				tool("Open", icons.FolderOpen, openFile),
				saveTool(hasCur && cur.Locked, save),
			),
		},
		Div(Class("flex min-h-0 flex-1"),
			Rail{Notes: all, Filter: flt, OnFilter: choose, OnNew: newNote, OnPrefs: showPrefs},
			Div(Class("relative flex min-w-0 flex-1 rounded-ss-xl border-s border-t border-border bg-background"), pane,
				If(drop.Over, dropOverlay{})),
		),
	))
}

// editorKey is the key of the editor: a note, and whether its content is
// hidden by a lock.
type editorKey struct {
	ID     int
	Hidden bool
}

// lockedSaveTitle and lockedSaveHint are why a locked note is not saved to
// a file: the tooltip of the Save button says both, the toast of Ctrl+S
// has the first as its title.
const (
	lockedSaveTitle = "Locked notes stay in your library."
	lockedSaveHint  = "Remove the lock to save to a file."
)

// trashOffer is a file the trash dialog asks about: the file of a note
// that was just locked (Path), and what the note is called (Heading).
type trashOffer struct{ Path, Heading string }

// question is the text of the dialog that offers to move the file to the
// Trash.
func (o trashOffer) question() string {
	name, folder := splitPath(o.Path)
	return fmt.Sprintf("%q is locked in your library and no longer tied to a file. The file %s, in %s, still holds its text unencrypted.", o.Heading, name, folder)
}

// notice is the text of the dialog on a desktop that cannot move the file
// to the Trash.
func (o trashOffer) notice() string {
	name, folder := splitPath(o.Path)
	return fmt.Sprintf("%q is locked in your library, but this desktop cannot move its file to the Trash. The file %s, in %s, still holds its text unencrypted: delete it yourself if you no longer need it.", o.Heading, name, folder)
}

// splitPath returns the name of a file and its folder, as the path writes
// them: at its last slash or backslash, without cleaning, so a path shows
// the same on every OS.
func splitPath(path string) (name, folder string) {
	i := strings.LastIndexAny(path, `/\`)
	if i < 0 {
		return path, "."
	}
	folder = path[:i]
	if folder == "" || strings.HasSuffix(folder, ":") {
		folder = path[:i+1] // "/" and "C:\"
	}
	return path[i+1:], folder
}

// noKeyringTip is the short form of it, for tooltips.
const noKeyringTip = "Locked notes need a system keyring, and this desktop has none."

// noKeyringHint is why locked notes are not available on a desktop with no
// system keyring.
const noKeyringHint = "Locked notes keep their key in the system keyring (GNOME Keyring, KWallet or the Windows Credential Manager), and this desktop has none."

// lockGuard locks the notes again when the window is hidden (to the tray):
// the content of an unlocked note does not stay in memory for as long as
// the app runs in the background. It is a component of its own so that only
// it builds again when the window's state changes.
type lockGuard struct{ OnHidden func() }

func (g lockGuard) Build() tela.Node {
	hidden := tela.UseWindow().State().Hidden
	f := tela.UseRef(g.OnHidden)
	f.Current = g.OnHidden
	tela.UseEffectDo(func() {
		if hidden {
			f.Current()
		}
	}, hidden)
	return nil
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
	return desktop.Tray{
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
	}
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

// saveTool is the Save button of the title bar. While a locked note is open
// it is off, and its tooltip says why: like the lock button without a
// keyring, it is disabled for assistive technologies but not inert, so the
// tooltip still opens.
func saveTool(locked bool, onClick func()) tela.Node {
	if !locked {
		return tool("Save", icons.Save, onClick)
	}
	return kit.Tooltip{
		Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icons.Save, AriaLabel: "Save",
			Props: AriaDisabled(true), Class: "opacity-50"},
		Content:   Text(lockedSaveTitle + " " + lockedSaveHint),
		Placement: "bottom",
	}
}

// tool is an icon button of the title bar with a tooltip.
func tool(label string, icon tela.Icon, onClick func()) tela.Node {
	return kit.Tooltip{
		Trigger: kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: icon, AriaLabel: label, OnClick: onClick},
		Content: Text(label), Placement: "bottom",
	}
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
