# notes

A small plain-text notes app, and the acceptance test of phase 12 ("desktop essentials"): it uses a tray icon, native open and save dialogs, two windows and a remembered window position, on Linux and Windows.

Its look is "Graphite, Tela light with amber": stone in the light scheme (the palette of `examples/hello`), graphite in the dark one, and one amber accent in both that Preferences can change. The theme (`theme.go`) is `kit.Theme()` with two tokens of its own, `sidebar` and `gold`, and a radius of 10.

The main window is frameless and draws its own title bar (`kit.TitleBar`, in the look of the platform, in the `sidebar` color) with the app's badge and the Open and Save buttons. Under it are three panes:

- **The rail**: New note, the views (All notes, Pinned, Unsaved), the folders and the #tags of the library, each with its count, and Preferences at the bottom.
- **The list and the editor**, when a note is open: the search field and the cards of the notes of the chosen filter, and the note: a header with its folder menu, its tags, an Unsaved mark and the pin, a title field, the text, and a status line (the file's path or "In your library", words, characters).
- **The overview**, when no note is open: the title of the filter with its count, the search field and a grid of cards.

A rail item always shows the overview of its filter, also the one that is already chosen, and a card opens its note with the list of the same filter next to it. The way back to the overview from a note is the rail's item (again) or Escape in the list: the first Escape clears the search, the second goes back. The app icon is drawn in code (`icon.go`), so the example has no image file.

The library:

- A note has a **title** (the editor's title field, separate from the text), a **text** (the body: the only thing a file holds), an optional **folder**, a **pin** and the date it last changed. With no title, a note is called by its file's name, else its first line, else "Untitled note". Opening a file makes a note titled with the file's name without its extension.
- **Tags** are the `#hashtags` written in the text, as in Bear (a `#` that starts the text or follows a space, then letters, digits, `-` or `_`; `# Heading`, `C#` and URL anchors are not tags). The rail lists all of them and the cards and the header show them: there is no other tag editor.
- **Folders** exist while a note is in one. A note's folder is chosen from its header: a menu of the folders, "No folder" and "New folder…", which asks for a name. A new note (or an opened file) goes into the folder that the rail has chosen, and is pinned in the Pinned view; where it would not show in the list (a tag, Unsaved, a search), the list goes back to all notes.
- **Deleting:** the trash button of the editor's header, Ctrl+Shift+D (Delete and Ctrl+Backspace edit the text, so they are not used), or Delete in the context menu of a card (a right click, or Shift+F10), which also pins. A note with changes that are not saved to a file, or tied to a file, asks first (`kit.AlertDialog`, which says that a file on disk is not deleted); any other is deleted at once, with a toast that has an Undo action. The note after it in the list opens (else the one before, else the overview).
- Pinned notes come first, then the most recently changed. Dates read "Today, 09:41", "Yesterday, 22:03", the weekday within the week, else "Oct 12". The clock is the package variable `now`, which the tests replace.
- The first run has four short welcome notes that show folders, tags and a pin. A `notes.json` of an earlier version (no title, folder, pin or date) loads and its notes look the same, without any of them.

What it shows:

- **Native dialogs** (`desktop.OpenFile`, `desktop.SaveFile`). Open reads a `.txt` or `.md` file into a new note and remembers its path (a file already open just selects its note); Save writes the note to its file, or asks where with the title as the suggested name. Both are modal to the main window (`desktop.Parent`), answer on the UI thread (the callback sets state directly), and treat cancel as "do nothing". Errors are `kit` toasts.
- **A second window.** Preferences is a `tela.Window` declared inside the main window's tree, so it exists exactly while `prefsOpen` is true, and its `OnClose` sets that state to false. It shares the app's component tree: it reads the same settings the main window reads, with no plumbing. It is a column of sections (Appearance, Editor, General, Shortcuts) and the rows of the chosen one.
- **Windows 11 look.** Preferences sets `Backdrop: tela.BackdropMica` and, on Windows, a root of `bg-background/60`, so on Windows 11 22H2 the window is Mica (in its title bar and through the 40% the root leaves clear); everywhere else the backdrop is ignored and the root is opaque.
- **Settings as state.** `desktop.UseSetting(key, default)` is a `useState` stored in `settings.json`: theme, accent, previews in the list, font size, line spacing, word wrap, "Keep running in the tray", "Notify when a file is saved". The Preferences window and the main window call it with the same keys, and a change in one builds the other.
- **Theme and accent at runtime.** Theme (Light, Dark or System) is `tela.SetColorScheme` and the accent swatches are `tela.SetTheme` with a theme whose `primary`, `gold` and `ring` tokens change (`theme.go`, `useAppearance`); both apply to every window. `withAppearance` gives `tela.Main` the stored values, so the first frame already has them.
- **The library** is `desktop.UseStoreSetting` on `notes.json` in `desktop.DataDir()`: every edit is stored (coalesced and atomic), a missing file starts the welcome notes, and a corrupt file is kept as `notes.json.corrupt` with a toast saying so. The store decodes what it reads over the default value note by note, so the welcome notes are the default only when the file has no library: otherwise an old note with no title would take the welcome note's.
- **Window placement.** The main window is `desktop.PersistApp(App)` and Preferences is a `desktop.SavedWindow{Key: "preferences"}`: both open where they were closed (position too on Windows and X11; on Wayland the size and the maximized state, and the position where the compositor restores sessions).
- **A tray icon** (`desktop.Tray`): click toggles the window, the menu has New note, Show Notes or Hide Notes (following the window), Preferences… and Quit. With "keep running in the tray" on and a tray that works (`OnError` clears it), closing the window hides it (`App.OnClose`).
- **A question before quitting** (`desktop.ShowMessage`, Yes/No/Cancel): with the tray off and notes that changed since their last Save, closing asks whether to save them. Yes saves each (a save dialog for a note with no file) and quits only if every save worked; No quits and leaves the changes in the library as drafts; Cancel keeps the app running.
- **A notification** (`desktop.Notify`, "Saved <name>") after a successful save when the setting is on; a desktop without notifications gets a toast instead.
- **Shortcuts:** Ctrl+N, Ctrl+O, Ctrl+S and Ctrl+, (Cmd on macOS), Ctrl+Shift+D (delete the open note), and Escape in the list.

How it handles a hidden window: a hidden window draws nothing, but its components still build and its effects run (ADR-185), so the tray's handlers set state as usual and the tray menu follows the window. The tray is a small component of its own (`notesTray`) that reads `UseWindow().State()`, so only it builds again when the window's state changes; its toggle reads the state when it runs (`win.State()` outside a build only reads), so two quick clicks act on what the window is.

`App.OnClose` is a plain function outside the component tree, but what closing does depends on state (the tray setting, the unsaved notes). The screen registers its latest handler in a small gate (`close.go`) after every build.

## Run it

- In a window: `CGO_ENABLED=0 go run ./examples/notes`.
- Without a window or a tray: `CGO_ENABLED=0 go run ./cmd/tela snapshot --size 960x640 [--dark] -o notes.png ./examples/notes`; the other screens are previews: `--preview "2 Overview"` and `--preview "3 Preferences" --size 540x440`. Under `tela snapshot` the library is a throwaway one in a temporary folder.
- Tests: `CGO_ENABLED=0 go test ./examples/notes`. They use `telatest.TempDesktopDirs`, so no test touches the real folders, and drive both windows through the headless window manager: dialogs answered with `AnswerFileDialog`, the message box with `AnswerMessage`, notifications read with `Notifications`, the tray clicked with `ClickTrayItem`, `ExpectWindowState` and `ExpectQuit` for close, and placements checked after a restart. Goldens (`testdata/notes_x1.png` and `notes_dark_x2.png` for the note, `notes_overview*.png` for the overview, `notes_edited.png`, `notes_folder_menu.png`, `preferences.png`, `preferences_dark.png` and `notes.aria.yml`; all with a fixed clock) are regenerated with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/notes`.
