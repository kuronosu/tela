# notes

A small plain-text notes app, and the acceptance test of phase 12 ("desktop essentials"): it uses a tray icon, native open and save dialogs, two windows and a remembered window position, on Linux and Windows.

The main window is frameless and draws its own title bar (`kit.TitleBar`, in the look of the platform) with Open, Save and Preferences buttons. A sidebar lists the notes with a New note button; the editor is a `kit.Textarea` with a status line (path, words, characters). The app icon is drawn in code (`icon.go`), so the example has no image file.

What it shows:

- **Native dialogs** (`desktop.OpenFile`, `desktop.SaveFile`). Open reads a `.txt` or `.md` file into a new note and remembers its path (a file already open just selects its note); Save writes the note to its file, or asks where with the title as the suggested name. Both are modal to the main window (`desktop.Parent`), answer on the UI thread (the callback sets state directly), and treat cancel as "do nothing". Errors are `kit` toasts.
- **A second window.** Preferences is a `tela.Window` declared inside the main window's tree, so it exists exactly while `prefsOpen` is true, and its `OnClose` sets that state to false. It shares the app's component tree: it reads the same settings the main window reads, with no plumbing.
- **Settings as state.** `desktop.UseSetting(key, default)` is a `useState` stored in `settings.json`: font size, word wrap, "Keep running in the tray when the window is closed", "Notify when a file is saved". The Preferences window and the main window call it with the same keys, and a change in one builds the other.
- **The library** is `desktop.UseStoreSetting` on `notes.json` in `desktop.DataDir()`: every edit is stored (coalesced and atomic), a missing file starts the welcome note, and a corrupt file is kept as `notes.json.corrupt` with a toast saying so.
- **Window placement.** The main window is `desktop.PersistApp(App)` and Preferences is a `desktop.SavedWindow{Key: "preferences"}`: both open where they were closed (position too on Windows and X11; on Wayland the size and the maximized state, and the position where the compositor restores sessions).
- **A tray icon** (`desktop.Tray`): click toggles the window, the menu has New note, Show Notes, Preferences… and Quit. With "keep running in the tray" on and a tray that works (`OnError` clears it), closing the window hides it (`App.OnClose`).
- **A question before quitting** (`desktop.ShowMessage`, Yes/No/Cancel): with the tray off and notes that changed since their last Save, closing asks whether to save them. Yes saves each (a save dialog for a note with no file) and quits only if every save worked; No quits and leaves the changes in the library as drafts; Cancel keeps the app running.
- **A notification** (`desktop.Notify`, "Saved <name>") after a successful save when the setting is on; a desktop without notifications gets a toast instead.
- **Shortcuts:** Ctrl+N, Ctrl+O, Ctrl+S and Ctrl+, (Cmd on macOS).

How it handles a hidden window: a hidden window gets no frames, so its components do not build while it is hidden. The tray's handlers therefore read the window's state when they run (`win.State()` outside a build only reads), and the tray menu has one fixed "Show Notes" item instead of a Show/Hide label that could not update.

`App.OnClose` is a plain function outside the component tree, but what closing does depends on state (the tray setting, the unsaved notes). The screen registers its latest handler in a small gate (`close.go`) after every build.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/notes`.
- Without a window or a tray: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 960x640 [--dark] -o notes.png ./examples/notes`; the Preferences window is a preview: `--preview "2 Preferences" --size 460x420`. Under `tela snapshot` the library is a throwaway one in a temporary folder.
- Tests: `CGO_ENABLED=0 go test ./examples/notes`. They use `telatest.TempDesktopDirs`, so no test touches the real folders, and drive both windows through the headless window manager: dialogs answered with `AnswerFileDialog`, the message box with `AnswerMessage`, notifications read with `Notifications`, the tray clicked with `ClickTrayItem`, `ExpectWindowState` and `ExpectQuit` for close, and placements checked after a restart. Goldens (`testdata/notes_x1.png`, `notes_dark_x2.png`, `notes_edited.png`, `preferences.png`, and `notes.aria.yml`) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/notes`.
