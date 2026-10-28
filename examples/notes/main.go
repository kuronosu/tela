// Command notes is a small plain-text notes app that shows the desktop
// services of phase 12: package desktop (native open and save dialogs, a
// message box, notifications, a tray icon, the app's folders, settings
// and a remembered window position), two windows, and a custom title bar.
//
// What it shows:
//
//   - A frameless window with a kit.TitleBar (the look of the platform:
//     Windows, GNOME or macOS) that holds Open, Save and Preferences
//     buttons. App.Frameless, h.WindowDrag and h.WindowControl are inside
//     the kit components; the OS moves, resizes and snaps the window.
//   - Native dialogs: desktop.OpenFile reads a .txt or .md file into a new
//     note, desktop.SaveFile asks where a note without a file goes. Both
//     are modal to the main window (desktop.Parent) and answer on the UI
//     thread, so the callbacks set state directly. A canceled dialog does
//     nothing; an error is a toast.
//   - A second window: Preferences is a tela.Window that exists while the
//     state says it is open (its OnClose sets that state to false). It
//     shares the app's one component tree, and its position is remembered
//     (desktop.SavedWindow).
//   - Settings: desktop.UseSetting is a useState stored in settings.json.
//     The Preferences window writes four settings and the main window,
//     which reads the same keys, follows with no plumbing between them.
//   - The library: desktop.UseStoreSetting keeps the notes in notes.json
//     under desktop.DataDir, written as you type.
//   - A tray icon (desktop.Tray) with a menu. With "Keep running in the
//     tray" on, closing the window hides it (App.OnClose) and the tray
//     brings it back; Quit is in the tray menu. With it off, closing asks
//     before it throws unsaved changes away (desktop.ShowMessage) and quits.
//   - A notification (desktop.Notify) when a file is saved, if the setting
//     is on.
//   - The main window opens where it was closed: desktop.PersistApp.
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
	"github.com/kuronosu/tela/kit"
)

var theme = kit.Theme()

// App is the main window; `tela snapshot` and the tests read it too. main
// wraps it in desktop.PersistApp, which loads the saved placement from
// disk, so reading App itself touches nothing.
var App = tela.App{
	Title:     "Notes",
	AppID:     "dev.telaui.notes",
	Width:     960,
	Height:    640,
	MinWidth:  560,
	MinHeight: 360,
	Frameless: true,
	Icon:      appIcon(),
	Theme:     &theme,
	RootClass: "overflow-hidden bg-background text-foreground",
	// What closing the window does depends on state, so the screen
	// registers it in the gate (close.go).
	OnClose: gate.closeMain,
	Root:    Notes{},
}

// Previews are the two windows, for `tela snapshot --preview "2 Preferences"
// --size 460x420 ./examples/notes`.
var Previews = tela.Previews{
	"1 Notes":       {Root: Notes{}},
	"2 Preferences": {Root: Preferences{}},
}

func main() { tela.Main(desktop.PersistApp(App)) }
