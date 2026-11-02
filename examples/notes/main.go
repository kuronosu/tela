// Command notes is a small plain-text notes app that shows the desktop
// services of phases 12 and 13: package desktop (native open and save
// dialogs, a message box, notifications, a tray icon, the app's folders,
// settings and a remembered window position; and now a global shortcut, a
// single instance with deep links, registration of a URL scheme and a file
// type, and secure storage), drag and drop, two windows, and a custom title
// bar. It is the acceptance test of both phases.
// Its look is "Graphite, Tela light with amber": stone in the light scheme,
// graphite in the dark one, and one amber accent (which Preferences can
// change) in both.
//
// What it shows:
//
//   - A frameless window with a kit.TitleBar (kit.Plain, in the arrangement of
//     the platform: Windows, GNOME or macOS) that holds the app's badge and the Open and
//     Save buttons. App.Frameless, h.WindowDrag and h.WindowControl are
//     inside the kit components; the OS moves, resizes and snaps the
//     window.
//   - Three panes under it. The rail has New note, the views (All notes,
//     Pinned, Unsaved), the folders and the #tags of the library, with
//     their counts, and Preferences. Choosing one shows the overview: a
//     grid of cards of that filter, with a search field. A card opens the
//     note: the list of the same filter and the editor (the folder menu,
//     the tags, the pin, delete, a title and the text, and a status line).
//     Cards have a context menu (pin, delete); deleting asks only when it
//     would lose something, and has an Undo toast. Escape
//     in the list, or choosing the rail's item again, goes back to the
//     overview.
//   - Tags are the #hashtags written in the text of a note, as in Bear: the
//     rail lists them all and the cards show them. A note's folder is
//     chosen from the editor's header (or "New folder...").
//   - Native dialogs: desktop.OpenFile reads a .txt or .md file into a new
//     note (titled with the file's name), desktop.SaveFile asks where a
//     note without a file goes. Both are modal to the main window
//     (desktop.Parent) and answer on the UI thread, so the callbacks set
//     state directly. A canceled dialog does nothing; an error is a toast.
//   - A second window: Preferences is a tela.Window that exists while the
//     state says it is open (its OnClose sets that state to false). It
//     shares the app's one component tree, and its position is remembered
//     (desktop.SavedWindow). Its theme and accent are applied to the whole
//     app with tela.SetColorScheme and tela.SetTheme.
//   - Settings: desktop.UseSetting is a useState stored in settings.json.
//     The Preferences window writes them and the main window, which reads
//     the same keys, follows with no plumbing between them.
//   - The library: desktop.UseStoreSetting keeps the notes in notes.json
//     under desktop.DataDir, written as you type.
//   - A tray icon (desktop.Tray) with a menu that follows the window (Show
//     Notes or Hide Notes: a hidden window still builds). With "Keep
//     running in the tray" on, closing the window hides it (App.OnClose)
//     and the tray brings it back; Quit is in the tray menu. With it off,
//     closing asks before it throws unsaved changes away
//     (desktop.ShowMessage) and quits.
//   - A notification (desktop.Notify) when a file is saved, if the setting
//     is on.
//   - The main window opens where it was closed: desktop.PersistApp.
//
// Phase 13 adds, on Linux and Windows:
//
//   - Dropped files: the whole window takes drops (h.OnDragEnter,
//     OnDragOver, OnDragLeave and OnDrop, see drop.go). .txt and .md files
//     become notes exactly as Open makes them, a file that is already open
//     selects its note, and dropped text becomes a note. While a drag that
//     carries files or text is over the window, an overlay says "Drop to
//     import"; any other drag gets none and is refused.
//   - A global shortcut: Quick note, Ctrl+Alt+N, is a desktop.GlobalShortcut
//     mounted while a setting is on (Preferences, Shortcuts). From any
//     application it shows and raises the window (also from the tray) and
//     makes a note. Preferences shows what the OS answered: the keys that
//     were bound (the Wayland portal may choose others), registered with no
//     keys (Hyprland: with the hyprland.conf line that binds it), or taken,
//     refused, removed, not available. On Linux the portal needs the app's
//     desktop entry, so main calls desktop.RegisterApp first, in the built
//     program (registerApp, in link.go).
//   - One instance and deep links: main is
//     tela.Main(desktop.SingleInstance(...)), and desktop.UseLaunch handles
//     every start of the program, the first one and the ones another process
//     hands over before it exits. Files open as notes; notes://new?title=...&text=...
//     makes a note and notes://open?id=3 opens one ("Copy link" in a card's
//     menu makes the link). See link.go.
//   - Registration: at start Notes makes itself a candidate handler of
//     notes: links (desktop.Register: only its own entries, nothing when
//     nothing changes, never another application's, and never the default).
//     Preferences, General, "Links and files" shows the status, registers or
//     unregisters the scheme, offers to open .md files with Notes
//     (desktop.FileType) and has a separate "Make default" button, which
//     on Windows opens the Default apps settings. See links.go.
//   - Locked notes: a note locked from the editor's header or a card's menu
//     is stored encrypted in notes.json (AES-256-GCM) with a key that
//     desktop.SetSecret keeps in the system keyring. The keyring is asked off
//     the UI thread. A locked note's card has a lock and no preview, and
//     the editor says "This note is locked" with an Unlock button. An
//     unlocked note stays so until it is locked again or the window is
//     hidden. Without a keyring the lock is off. See lock.go.
//
// Try it. Build the program first (go build -o notes ./examples/notes): under
// go run it is a temporary build, which Notes does not register.
//
//   - Drop: drag a .md file from the file manager on the window.
//   - Quick note: press Ctrl+Alt+N in another application (on Hyprland and
//     other Wayland desktops the compositor asks to bind it: the shortcuts
//     portal).
//   - Deep link, from a terminal: `xdg-open 'notes://new?text=hi'` on Linux,
//     `start notes://new?text=hi` on Windows (cmd), or the program itself,
//     `notes 'notes://new?text=hi'` or `notes todo.md`: the running window
//     comes up with the result. The links work through the OS once Notes is
//     registered (Preferences, General).
//   - Locked notes: Lock note in an open note's header.
//
// The notes live in the library as you type, so the question when quitting
// is about files: whether to write the notes that changed since their last
// Save, or leave them as drafts in the library.
//
// `tela snapshot ./examples/notes` renders the main window without a
// window or a tray, with a library in a temporary folder; add --dark for
// the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
)

var theme = themeFor(accents[0].Name)

// App is the main window; `tela snapshot` and the tests read it too. main
// wraps it in desktop.PersistApp, which loads the saved placement from
// disk, so reading App itself touches nothing.
var App = tela.App{
	Title:     "Notes",
	AppID:     "dev.telaui.notes",
	Width:     960,
	Height:    640,
	MinWidth:  720,
	MinHeight: 400,
	// The title bar is the app's own; on macOS the OS keeps its traffic
	// lights over it (kit.TitleBar leaves room for them), elsewhere this is
	// Frameless.
	TitleBarOverlay: true,
	Icon:            appIcon(),
	Theme:           &theme,
	RootClass:       "overflow-hidden bg-background text-foreground",
	// What closing the window and quitting the app do depends on state, so
	// the screen registers them in the gate (close.go).
	OnClose: gate.closeMain,
	OnQuit:  gate.quitApp,
	Root:    Notes{},
}

// Previews are the screens, for `tela snapshot --preview "3 Preferences"
// --size 540x440 ./examples/notes`.
var Previews = tela.Previews{
	"1 Notes":       {Root: Notes{}},
	"2 Overview":    {Root: Notes{Overview: true}},
	"3 Preferences": {Root: Preferences{}},
}

func main() {
	desktop.SetAppID(App.AppID) // before RegisterApp, which names the entry after it
	registerApp()
	autoRegister.Store(true) // register the notes: scheme at start (never in tests or snapshots)
	tela.Main(desktop.SingleInstance(withAppearance(desktop.PersistApp(App))))
}
