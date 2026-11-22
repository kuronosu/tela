# Recipe: desktop integration

Questions: drag and drop, the clipboard, global shortcuts, one instance and launches, links and file types, secrets in the OS keyring, the trash, native menus and the app's menu bar, the system's contrast, text scale and accent; how to test each.

**Drag and drop, clipboard, shortcuts, launches, links, secrets, menus, settings.**

- **Drag and drop is HTML's.** A target takes `h.OnDragEnter`, `OnDragOver`, `OnDragLeave` and `OnDrop` (they bubble) and accepts by calling `e.PreventDefault()` in `OnDragOver`. Before the drop only the types are known (`e.DataTransfer.Has("Files")`, `Types()`); the data arrives in `OnDrop` (`Files()` absolute paths, `Text()`, `HTML()`, `Image()`, `GetData(mime)`). Enter fires on the new element before leave fires on the old one, so "a drag is over me" is a counter (+1 on enter, -1 on leave, 0 on drop). A source is `h.Draggable(true)`: its `OnDragStart` fills `e.DataTransfer` (`SetText`, `SetHTML`, `SetFiles`, `SetImage`, `SetData(mime, s)`, `EffectAllowed = tela.DropMove`); with no data the press stays the `OnDrag` gesture. `OnDragEnd` reads `e.DataTransfer.DropEffect` (`tela.DropNone|DropCopy|DropMove|DropLink`). It works between windows and with other apps.
- **Clipboard:** `desktop.ReadClipboard(done, types...)` (`done(*tela.DataTransfer, err)` on the UI thread) and `desktop.WriteClipboard(&dt)` (every representation at once) carry the same `DataTransfer`; `ReadClipboardText` and `WriteClipboardText` for text only. Text fields keep their own paste.
- **`desktop.GlobalShortcut{ID, Keys, Description, OnPress, OnRelease, OnStatus}`** is a component: the shortcut is registered while it is mounted (keep `ID` stable; several allowed). The OS decides, so show `OnStatus`: `ShortcutBound` (`Trigger` is what the user presses: the Wayland portal asks the user and may bind other keys; `""` on Hyprland, which assigns none: the user adds `bind = CTRL ALT, N, global, <AppID>:<ID>` to hyprland.conf), `ShortcutTaken`, `ShortcutRefused`, `ShortcutRevoked`, `ShortcutFailed` (`Err` wraps `ErrUnsupported` when the desktop has none, `ErrAppNotRegistered` without the app's desktop entry). Check `Keys` typed by a user with `desktop.ParseKeys`.
- **`desktop.RegisterApp()`** in `main`, before `tela.Run`, in the built program (give the app an `AppID`): the Linux shortcut portal needs the app's desktop entry. It writes only that entry (per user, no menu launcher), nothing when it is there or another program's is; nothing on Windows. `UnregisterApp`, `AppStatus`.
- **One instance and launches:** `desktop.SingleInstance(app)` (needs `AppID`) makes a second start forward its arguments to the running app and exit with status 0; the running app raises its main window (shown, from the tray too). `desktop.UseLaunch(func(desktop.Launch))` is a hook that gets the process's own start once and every forwarded start; `l.URLs()` (`notes:open?id=3`), `l.Files()` (absolute paths).
- **Links and file types:** `desktop.Register(desktop.URLScheme("notes"))` and `desktop.FileType(".note", "application/x-notes-note", "Notes document")` make the app a *candidate* handler, per user and without a running app (an installer can call it); `changed` is false when nothing was written. It **does not make the app the default** (that is `desktop.MakeDefault`, only when the user asks; for Windows file types it opens Settings and returns `DefaultAsked`) and **never takes another app's association** (`ErrAssociationTaken` unless you pass `desktop.Replace()`, after asking). `desktop.RegistrationStatus` says `Registered`, `Default` and `Other`; `Unregister` removes it. It does file or registry I/O: call it at start or off the UI thread. Under `go run` or `tela dev` the executable is temporary: give `desktop.ForApp(desktop.HandlerApp{Exec: path})` or skip (`ErrDevExecutable`).
- **Secrets:** `desktop.SetSecret(name, []byte)`, `Secret(name)`, `DeleteSecret(name)` use the OS keyring (Credential Manager, Secret Service, Keychain), per app ID. They block (the OS may ask the user to unlock the keyring): **call them from a goroutine, never from a handler or an effect** (dev mode warns). Errors: `ErrSecretNotFound`, `ErrSecretCanceled` (the user dismissed the prompt; do not retry in a loop), `ErrUnsupported` (no keyring: turn the feature off).
- **Trash:** `desktop.MoveToTrash(path)` moves a file or folder to the trash (Recycle Bin), never deleting it: a file the trash cannot take stays and the error wraps `ErrUnsupported`; a missing one wraps `fs.ErrNotExist`. It blocks: ask the user first, then call it from a goroutine and `tela.Post` the outcome.
- **Native menus:** `kit.ContextMenu` and `kit.Menubar` are drawn by default. `Native: kit.NativeOn` (or `kit.NativeOff`) per menu, or `tela.App{NativeMenus: true}` for all (a menu's own setting wins), uses the OS's menu where there is one (Windows now: context menus, and a menu bar on windows with a title bar of the OS) and draws it elsewhere (Linux, frameless windows), with no code of yours. A native menu drops icons, the destructive color, `KeepOpen` and classes. Below kit: `desktop.PopupMenu(win, at, items, done)` and `desktop.MenuBar{Menus, OnError}` return or report `ErrUnsupported` so you can draw your own. **The app's menu bar**, declared once anywhere: `desktop.AppMenuBar{Menus, AppItems, OnError}` or `kit.Menubar{AppMenu: true, AppItems}`: macOS's bar for every window (`AppItems` go in the app menu), on Windows the bar of every framed window without its own `desktop.MenuBar` (which macOS lacks), else `ErrUnsupported` and kit draws it. `desktop.MenuItem{Role: desktop.RoleCopy}` (Electron's roles: undo, redo, cut, paste, selectAll, minimize, zoom, close) gets label and action, `{Role: desktop.RoleEditMenu}` or `RoleWindowMenu` their standard items. On macOS an `Accelerator` is the key equivalent (the key does not reach the app): write `CmdOrCtrl+N`. `kit.MenuItem.AccessKey` and `kit.MenubarMenu.AccessKey` set the mnemonics of drawn and native menus on Windows and Linux (recipe accessibility).
- **Settings v2:** `contrast-more:` is the variant for the system's high contrast (`tela.UseHighContrast()` in code). The system's text scale works like a browser's default font size: 1rem is the theme's rem times the scale, so `text-*` and rem spacing (`p-4`, `gap-2`, `w-64`, breakpoints) grow together while px values (`border`, `text-[13px]`, `w-[320px]`) do not; `tela.UseTextScale()` returns it for sizes computed in code, and `tela.App{IgnoreTextScale: true}` opts a fixed layout out. `tela.UseAccentColor() (Color, bool)` is the system accent (false: none). In tests and snapshots the scale is 1 and the accent absent unless set.

```go
package integration

import (
	"errors"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
)

// DropZone takes files dropped from a file manager.
type DropZone struct{ OnFiles func([]string) }

func (d DropZone) Build() tela.Node {
	depth, setDepth := tela.UseState(0) // enters minus leaves
	files := func(dt *tela.DataTransfer) bool { return dt.Has("Files") }
	border, label := "border-border contrast-more:border-foreground", "Drop files here"
	if depth > 0 {
		border, label = "border-primary bg-primary/5", "Release to add them"
	}
	return Div(Class("rounded-lg border-2 border-dashed p-8 text-center", border), TestID("drop"),
		OnDragEnter(func(e tela.DragEvent) {
			if files(e.DataTransfer) {
				setDepth.Update(func(n int) int { return n + 1 })
			}
		}),
		OnDragOver(func(e tela.DragEvent) {
			if files(e.DataTransfer) {
				e.PreventDefault() // accept: the drop comes here
			}
		}),
		OnDragLeave(func(e tela.DragEvent) {
			if files(e.DataTransfer) {
				setDepth.Update(func(n int) int { return max(n-1, 0) })
			}
		}),
		OnDrop(func(e tela.DragEvent) {
			setDepth.Set(0)
			d.OnFiles(e.DataTransfer.Files())
		}),
		Text(label))
}

// Card is dragged to a target of the app, or to another application as text.
type Card struct {
	ID, Title string
	OnMoved   func(id string)
}

func (c Card) Build() tela.Node {
	return Div(Draggable(true), TestID("card-"+c.ID), Class("cursor-grab rounded-md border p-3"),
		OnDragStart(func(e tela.DragEvent) {
			e.DataTransfer.SetData("application/x-card", c.ID)
			e.DataTransfer.SetText(c.Title)
			e.DataTransfer.EffectAllowed = tela.DropMove
		}),
		OnDragEnd(func(e tela.DragEvent) {
			if e.DataTransfer.DropEffect == tela.DropMove {
				c.OnMoved(c.ID) // a target took it
			}
		}),
		Text(c.Title))
}

// CopyTitle puts a title on the clipboard as text; Paste reads it back.
func CopyTitle(title string) error {
	var dt tela.DataTransfer
	dt.SetText(title)
	return desktop.WriteClipboard(&dt)
}

func Paste(done func(text string)) error {
	return desktop.ReadClipboard(func(dt *tela.DataTransfer, err error) {
		if err == nil {
			done(dt.Text())
		}
	})
}

// Quick makes a note from anywhere (a global shortcut) and from links and files (UseLaunch).
type Quick struct{}

func (Quick) Build() tela.Node {
	win := tela.UseWindow()
	notes, setNotes := tela.UseState([]string{})
	hint, setHint := tela.UseState("")
	add := func(s string) { setNotes.Update(func(n []string) []string { return append(n, s) }) }
	desktop.UseLaunch(func(l desktop.Launch) { // the start, then every forwarded start
		for _, u := range l.URLs() { // notes:new?text=hi
			add(u)
		}
		for _, p := range l.Files() {
			add(p)
		}
	})
	return Div(Class("p-4"),
		desktop.GlobalShortcut{
			ID: "quick-note", Keys: "Ctrl+Alt+N", Description: "Create a note",
			OnPress: func() { win.Show(); add("Untitled note") },
			OnStatus: func(e desktop.ShortcutStatusEvent) {
				if e.Status != desktop.ShortcutBound {
					setHint.Set(e.Err.Error()) // taken, refused, removed or unsupported
				}
			},
		},
		P(Textf("%d notes", len(notes))), P(Text(hint)))
}

// App is a single-instance app: a second start hands its launch to the first.
var App = desktop.SingleInstance(tela.App{AppID: "com.example.notes", Title: "Notes", Root: Quick{}})

// RegisterLinks makes the app a candidate for notes: links; the default only if the user asks.
// It does file or registry I/O: call it at start, off the UI thread.
func RegisterLinks(makeDefault bool) (desktop.Registration, error) {
	scheme := desktop.URLScheme("notes")
	if _, err := desktop.Register(scheme); err != nil {
		return desktop.Registration{}, err // errors.Is(err, desktop.ErrAssociationTaken): another app holds it
	}
	if makeDefault {
		if _, err := desktop.MakeDefault(scheme); err != nil {
			return desktop.Registration{}, err
		}
	}
	return desktop.RegistrationStatus(scheme)
}

// Vault unlocks a key kept in the OS's keyring. The secret calls block: use a goroutine.
type Vault struct{}

func (Vault) Build() tela.Node {
	state, setState := tela.UseState("locked")
	unlock := func() {
		go func() { // the keyring may be waiting for the user
			_, err := desktop.Secret("library-key")
			switch {
			case err == nil:
				setState.Set("unlocked")
			case errors.Is(err, desktop.ErrSecretNotFound):
				setState.Set("no key yet")
			case errors.Is(err, desktop.ErrSecretCanceled):
				setState.Set("the keyring stayed locked")
			default: // desktop.ErrUnsupported: no keyring
				setState.Set("no keyring")
			}
		}()
	}
	return Div(Button(OnClick(unlock), Text("Unlock")), Text(state))
}
```

```go
package integration

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// NoteMenu opens the OS's context menu where there is one, the drawn one elsewhere.
func NoteMenu(title string, onDelete func()) tela.Node {
	return kit.ContextMenu[string]{
		Native: kit.NativeOn, // NativeAuto (zero) follows tela.App{NativeMenus: true}
		Items: []kit.MenuItem[string]{
			{Key: "copy", Label: "Copy link", Shortcut: "Ctrl+L"},
			{Kind: kit.MenuSeparator},
			{Key: "delete", Label: "Delete", Variant: kit.Destructive, OnSelect: onDelete},
		},
		Children: Div(Class("rounded-lg border p-4"), Text(title)),
	}
}

// Readout follows the system's contrast, text scale and accent color.
type Readout struct{}

func (Readout) Build() tela.Node {
	scale := tela.UseTextScale() // 1.25: 1rem is 20 px
	accent, ok := tela.UseAccentColor()
	if !ok {
		accent = tela.Hex("#4f46e5")
	}
	return Div(Class("rounded-lg border border-border p-4 text-sm contrast-more:border-2 contrast-more:border-foreground"),
		Style(tela.Bg(accent)), Textf("Text scale %.2f", scale))
}
```

**Integration services in tests.** The headless desktop plays the OS for each feature, and nothing touches the real clipboard, keyring, registry, shortcuts or menus. Call `telatest.TempDesktopDirs(t)` first.

- **Drag and drop:** `app.DropFiles(loc, paths...)` and `app.DropData(loc, payload)` (payloads `telatest.DragFiles`, `DragText`, `DragHTML`, `DragPNG`, `DragType(mime, s)`, combined with `.And`) drop what another application drags and return the `tela.DropEffect` the app accepted (`tela.DropNone`: refused). Steps: `app.HoverDrag(loc, telatest.Carrying(p))` (the drag-over state), `app.LeaveDrag()`, `app.CancelDrag()`, `app.DropOn(loc)`. A drag of your own `Draggable`: `app.DragTo(from, to)` (also `DragToWindow(from, otherWindow, to)`), or `app.StartDrag(from)` then `DropOutside(tela.DropMove)` for a drop in another application.
- **Shortcuts and launches:** `app.GlobalShortcuts()` (what is registered), `app.PressShortcut(id)`, `ReleaseShortcut(id)`, `app.SetShortcutStatus(id, desktop.ShortcutTaken, "")` (also `ShortcutRefused`, `ShortcutRevoked`, `ShortcutBound` with a trigger), `app.SetShortcutUnassigned(id)`, `app.FailShortcuts(err)`; `app.InjectLaunch(desktop.Launch{Args: ...})` plays a second start. `desktop.SingleInstance` claims nothing in tests.
- **Clipboard, secrets, registration:** `app.Clipboard()` / `app.SetClipboard(&dt)`; `app.StoredSecret(name)`, `app.SetStoredSecret(name, b)`, `app.FailSecrets(desktop.ErrSecretCanceled)` (`nil` stops failing); `app.Registrations()`, `app.HoldAssociation(assoc, "Other App")` (another app owns it), `app.SetDefaultHandler(assoc, "Other App")`, `app.RegisteredApps()`. `app.Trashed()` (the paths `desktop.MoveToTrash` moved; the headless trash moves them into a temporary folder), `app.FailTrash(err)`. `app.Unsupported(telatest.Secrets)` (also `GlobalShortcuts`, `NativeMenus`, `ClipboardData`, `Associations`, `Trash`) tests the fallbacks.
- **Menus and settings:** `telatest.PreferNativeMenus()` is `App.NativeMenus`; `app.PopupMenus()`, `app.ChoosePopupItem(path...)`, `app.DismissPopupMenu()` answer a native context menu, `app.MenuBar()` and `app.ChooseMenuItem("File", "New")` a native menu bar, `app.AppMenuBar()`, `AppMenuItems()` and `ChooseAppMenuItem("File", "New")` the app's (`WithUnsupported(telatest.AppMenu)` plays Windows). `app.RequestQuit()` plays Cmd+Q; options `TitleBarOverlay()` and `TitleBarControls(rect)` (`PlatformKeys("darwin")` gives the Mac's). The options `telatest.HighContrast()`, `TextScale(1.25)`, `AccentColor(c)` set the system's settings at mount; `app.SetDarkMode(bool)`, `SetHighContrast`, `SetTextScale`, `SetAccentColor`, `SetReducedMotion` change them while it runs.

```go
package integration

import (
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/telatest"
)

func TestDropAndShortcut(t *testing.T) {
	telatest.TempDesktopDirs(t)
	var dropped []string
	zone := telatest.Mount(t, DropZone{OnFiles: func(f []string) { dropped = f }})
	path := filepath.Join(t.TempDir(), "ideas.md")
	zone.HoverDrag(telatest.ByTestID("drop"), telatest.Carrying(telatest.DragFiles(path)))
	zone.ExpectText("Release to add them") // the drag-over state
	if zone.DropOn(telatest.ByTestID("drop")) == tela.DropNone || len(dropped) != 1 {
		t.Fatalf("the drop was refused: %v", dropped)
	}

	app := telatest.MountApp(t, App)
	app.PressShortcut("quick-note")
	app.ExpectText("1 notes")
	app.InjectLaunch(desktop.Launch{Args: []string{"notes:open?id=3"}})
	app.ExpectText("2 notes")
	app.SetShortcutStatus("quick-note", desktop.ShortcutTaken, "")
	app.ExpectText("taken by another application")
}

func TestVaultAndLinks(t *testing.T) {
	telatest.TempDesktopDirs(t)
	app := telatest.Mount(t, Vault{})
	app.FailSecrets(desktop.ErrSecretCanceled) // the user dismissed the unlock prompt
	app.Click(telatest.ByRole("button", "Unlock"))
	app.ExpectText("the keyring stayed locked")
	app.FailSecrets(nil)
	app.SetStoredSecret("library-key", []byte("k"))
	app.Click(telatest.ByRole("button", "Unlock"))
	app.ExpectText("unlocked")

	app.HoldAssociation(desktop.URLScheme("notes"), "Other App")
	if _, err := RegisterLinks(false); err == nil {
		t.Fatal("registered over another application")
	}
	app.HoldAssociation(desktop.URLScheme("notes"), "")
	app.SetDefaultHandler(desktop.URLScheme("notes"), "Other App") // the user's default
	if reg, err := RegisterLinks(false); err != nil || !reg.Registered || reg.Default {
		t.Fatalf("a candidate, not the default: %+v, %v", reg, err)
	}
}

func TestNativeMenuAndSettings(t *testing.T) {
	app := telatest.Mount(t, NoteMenu("Groceries", func() {}), telatest.Size(300, 200))
	app.Click(telatest.ByText("Groceries"), telatest.Right())
	app.ChoosePopupItem("Delete")

	ro := telatest.Mount(t, Readout{}, telatest.TextScale(1.5), telatest.HighContrast())
	ro.ExpectText("Text scale 1.50")
	ro.SetTextScale(2)
	ro.ExpectText("Text scale 2.00")
}
```
