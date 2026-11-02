# Desktop integration

[Windows and Desktop](Windows-and-Desktop) covers an app's windows and the
basic services of the OS: dialogs, notifications, the tray, settings. This guide
covers what makes an app part of the desktop it runs on: files dragged in from a
file manager, the clipboard, a shortcut that works while another application has
the focus, links that open the app, one running copy per user, secrets kept in
the OS's keyring, native menus and the system's accessibility settings.

Everything here follows the model of the rest of Tela. What lives while it is
mounted is a component (`GlobalShortcut`), what answers later is a callback on
the UI thread (`ReadClipboard`), and what blocks stays off the UI thread (the
secrets). Each section ends with how a platform differs; the
[Platform notes](#platform-notes) collect them. For a complete app that uses
every feature, see
[`examples/notes`](https://github.com/kuronosu/tela/tree/main/examples/notes):
it accepts dropped files, has a global quick-note shortcut, reopens in its
running instance from `notes:` links, registers itself for them and keeps locked
notes encrypted with a key in the keyring.

- [Drag and drop](#drag-and-drop)
- [The clipboard](#the-clipboard)
- [Global shortcuts](#global-shortcuts)
- [One instance and launches](#one-instance-and-launches)
- [Links and file types](#links-and-file-types)
- [Secrets](#secrets)
- [Native menus](#native-menus)
- [System settings](#system-settings)
- [Under tela dev](#under-tela-dev)
- [Platform notes](#platform-notes)
- [Testing](#testing)

## Drag and drop

Drag and drop is HTML's. It works inside a window, between the windows of the
app and with other applications: a file manager drops files on your window, your
window drops text in an editor. The data goes through the OS where it can, so
the drag leaves the window.

There are two roles. A **target** accepts a drag, and a **source** starts one.
Elements that are neither keep the drag *gesture* (`OnDragStart`, `OnDrag`,
`OnDragEnd`, see [Events](Events#drag)), which moves things inside the window and
never involves the OS.

### Targets

A target has four handlers, all of which bubble:
[`OnDragEnter`](API-Reference-h#func-ondragenter),
[`OnDragOver`](API-Reference-h#func-ondragover),
[`OnDragLeave`](API-Reference-h#func-ondragleave) and
[`OnDrop`](API-Reference-h#func-ondrop). They get a
[`tela.DragEvent`](API-Reference-tela#type-dragevent) whose `DataTransfer` says
what is being dragged. A target **accepts the drag by calling
`e.PreventDefault()` in `OnDragOver`**, as in HTML. Without it the drop is refused
and `OnDrop` never runs.

Before the drop only the *types* are known (`Types()`, `Has("Files")`,
`Has("text/plain")`): the OS does not hand over the data until the user lets go.
`OnDrop` has all of it:

| Read | Gives |
|---|---|
| `Files()` | Absolute paths of the dragged files |
| `Text()`, `HTML()` | Plain text, an HTML fragment |
| `Image()` | A decoded PNG (`image.Image, ok`) |
| `GetData(mime)`, `Get(mime)` | A custom type, as a string or as bytes |

`DropEffect` starts as the effect the platform's convention picks from what the
source allows and the modifier keys held (Ctrl copies, Shift moves, Ctrl+Shift
links, else copy); a handler that accepts may change it in `OnDragOver`, and set
it to `tela.DropNone` in `OnDrop` to tell the source the data was not taken.

```go
type DropZone struct{ OnFiles func([]string) }

func (d DropZone) Build() tela.Node {
	depth, setDepth := tela.UseState(0) // enters minus leaves
	files := func(dt *tela.DataTransfer) bool { return dt.Has("Files") }
	border, label := "border-border contrast-more:border-foreground", "Drop files here"
	if depth > 0 {
		border, label = "border-primary bg-primary/5", "Release to add them"
	}
	return Div(Class("rounded-lg border-2 border-dashed p-8 text-center", border),
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
```

**Why a counter?** `OnDragEnter` and `OnDragLeave` follow HTML: they fire when the
element under the drag changes, the enter of the new element *before* the leave
of the old one, and they bubble. When the drag moves from the zone onto a child
of the zone, the zone gets enter (from the child) and then leave (from the
zone itself). A boolean would flicker off; counting enters and leaving at zero is
the standard fix. A drag that is canceled (Escape) or leaves the window also ends
with a leave. `examples/notes` wraps the counter in a hook, `useDrop`, that you
can copy.

### Sources

An element with [`Draggable(true)`](API-Reference-h#func-draggable) is a source.
When a press on it becomes a drag, its `OnDragStart` gets an empty
`DataTransfer` to fill. If the handler puts data in it, Tela starts a real drag:
the OS carries the data under the pointer, with a picture of the element (not on X11, which shows none), until
the user drops it.

```go
Div(Draggable(true), Class("cursor-grab rounded-md border p-3"),
	OnDragStart(func(e tela.DragEvent) {
		e.DataTransfer.SetData("application/x-card", card.ID) // for your own targets
		e.DataTransfer.SetText(card.Title)                    // for any other application
		e.DataTransfer.EffectAllowed = tela.DropMove
	}),
	OnDragEnd(func(e tela.DragEvent) {
		if e.DataTransfer.DropEffect == tela.DropMove {
			remove(card.ID) // the target took it: the source deletes its copy
		}
	}),
	Text(card.Title))
```

Put the best representation first and add simpler ones after it: a target picks
the one it understands (`SetHTML` then `SetText`; `SetFiles(paths...)` for a file
manager; `SetImage` for a picture; `SetData(mime, s)` for your own types, which
only your own targets will read). `SetDragImage` shows something other than the
element. The source's `OnDragEnd` runs once, when the drag is over, and
`DropEffect` says what happened (`DropNone` if it was canceled or refused).

While a real drag runs the pointer belongs to the OS: `OnDrag` does not run, and
the window's hover and `:active` states end. A handler that puts no data in the
`DataTransfer`, or calls `PreventDefault()`, keeps the press as the ordinary
drag gesture, so one element can be draggable in two ways.

### What the OS decides

- Files from a file manager arrive as paths; the files may be on a share or a
  removable disk, so open them off the UI thread if they can be large.
- A window under a modal child refuses drags.
- `OnDrop` needs no `PreventDefault`; its `DropEffect` is the outcome.
- Before the drop the getters (`Text()`, `Files()`, `GetData`...) return nothing.
  The drop reads the well-known types and the custom ones your handlers looked up
  before it, so check a custom type with `Has("application/x-card")` in
  `OnDragEnter` or `OnDragOver`.

## The clipboard

The clipboard carries the same `tela.DataTransfer` as a drag: text, HTML, PNG
images, file lists and custom MIME types. Text fields keep their own text paste;
this is for everything else (pasting an image into a canvas, copying a card).

```go
// Copy: every representation at once; another application pastes the one it understands.
func CopyCard(c Card) error {
	var dt tela.DataTransfer
	dt.SetData("application/x-card", c.ID)
	dt.SetText(c.Title)
	return desktop.WriteClipboard(&dt)
}

// Paste: done runs later, on the UI thread, so it may set state.
func Paste(attach func([]string), insert func(string)) error {
	return desktop.ReadClipboard(func(dt *tela.DataTransfer, err error) {
		switch {
		case err != nil:
		case len(dt.Files()) > 0:
			attach(dt.Files())
		default:
			insert(dt.Text())
		}
	})
}
```

[`ReadClipboard(done, types...)`](API-Reference-desktop#func-readclipboard) reads
the types you name, or with none the well-known ones (text, HTML, a PNG image,
files); an empty clipboard gives an empty `DataTransfer`, not an error.
[`ClipboardTypes`](API-Reference-desktop#func-clipboardtypes) calls back with the
types that are on it, without reading the data. [`WriteClipboard`](API-Reference-desktop#func-writeclipboard)
replaces the whole content; a nil `DataTransfer` empties it.
`ReadClipboardText` and `WriteClipboardText` are the short forms for text, and the
fallback where a desktop's clipboard carries only text (`ErrUnsupported`).

## Global shortcuts

[`desktop.GlobalShortcut`](API-Reference-desktop#type-globalshortcut) is a
component that registers a system-wide shortcut while it is mounted: `OnPress`
runs when the user presses the keys *anywhere*, even while another application
has the focus. It renders nothing where you put it.

```go
type Quick struct{}

func (Quick) Build() tela.Node {
	win := tela.UseWindow()
	hint, setHint := tela.UseState("")
	return Div(
		desktop.GlobalShortcut{
			ID:          "quick-note", // stable: the OS remembers the user's changes to it
			Keys:        "Ctrl+Alt+N",
			Description: "Create a note", // shown in the desktop's shortcut settings
			OnPress:     func() { win.Show(); newNote() },
			OnStatus: func(e desktop.ShortcutStatusEvent) {
				if e.Status != desktop.ShortcutBound {
					setHint.Set(e.Err.Error()) // "Ctrl+Alt+N is taken by another application"
				}
			},
		},
		Text(hint),
	)
}
```

**The OS has the last word**, so a shortcut is a request, not a fact. `OnStatus`
reports the outcome of the registration and every later change:

| Status | Meaning |
|---|---|
| `ShortcutBound` | It works. `Trigger` is what the user presses, which may not be `Keys`; `""` means the desktop assigned no keys yet (Hyprland, below) |
| `ShortcutTaken` | Another application holds the combination |
| `ShortcutRefused` | The user or the desktop declined to bind it |
| `ShortcutRevoked` | The user removed it later, in the desktop's shortcut settings |
| `ShortcutFailed` | It could not be registered: bad keys, an OS error, no global shortcuts on this desktop (`Err` wraps `ErrUnsupported`), or a Linux desktop that does not know the app (`Err` wraps `ErrAppNotRegistered`, below) |

Show the state in your preferences ("Quick note: Ctrl+Alt+N", or the reason it is
off) and never assume the shortcut works. A nil `OnStatus` reports anything but
`ShortcutBound` as a dev warning. Keys are written `Ctrl+Alt+N`, `Super+F5`:
check ones a user typed with [`desktop.ParseKeys`](API-Reference-desktop#func-parsekeys),
which returns the canonical spelling or an error. `OnRelease` reports the release
where the OS does (not on Windows).

The component re-registers only when `ID`, `Keys` or `Description` change, so
rebuilding it is free. Several can be mounted at once; if two mounted components
use the same `ID`, the latest wins. To let the user turn it off, render it
conditionally: `If(enabled, desktop.GlobalShortcut{...})`.

On Wayland, a compositor that implements the GlobalShortcuts portal asks the user
the first time, and the user may bind other keys: that is why `Trigger` exists.
Compositors without it (or sessions without a portal) report
`ShortcutFailed` with an `ErrUnsupported` error: offer the same action in the app
and say that the global shortcut is not available.

**Hyprland registers the shortcut but assigns no keys**: it ignores `Keys`, and
`OnStatus` gets `ShortcutBound` with an empty `Trigger`. The user binds it in
`hyprland.conf`, after which it fires as usual:

```
bind = CTRL ALT, N, global, com.example.notes:quick-note
```

The part after `global,` is the app ID ([`desktop.AppID`](API-Reference-desktop#func-appid)),
a colon and the shortcut's `ID`. Show the keys you asked for and that line
(`examples/notes` does, in Preferences).

**The Linux portal needs the app's desktop entry.** xdg-desktop-portal identifies
an app by its ID, and accepts it only when the desktop has an entry
`<AppID>.desktop`; without one every shortcut fails with an error wrapping
`desktop.ErrAppNotRegistered`, which names the entry. Give the app an ID
(`tela.App.AppID` or `desktop.SetAppID`; the executable's name is the default)
and call [`desktop.RegisterApp`](API-Reference-desktop#func-registerapp) at start,
in the built program, before `tela.Run`:

```go
func main() {
	desktop.SetAppID("com.example.notes")
	if _, err := desktop.RegisterApp(); err != nil && !errors.Is(err, desktop.ErrDevExecutable) {
		log.Print(err)
	}
	tela.Main(App)
}
```

`RegisterApp` writes only the app's own entry, per user
(`~/.local/share/applications/<AppID>.desktop`), with no launcher in the menus
(`NoDisplay=true`) and no file types. It looks first: it writes nothing when the
entry is up to date, or when a package already installed an entry of that ID,
which it never takes over without `desktop.Replace()`. Scheme and file-type
registration ([below](#links-and-file-types)) share the file; the entry stays while
either needs it. [`UnregisterApp`](API-Reference-desktop#func-unregisterapp)
removes it (keeping it for the associations that still need it), and
[`AppStatus`](API-Reference-desktop#func-appstatus) reports it. Windows needs no
entry, and nothing is written there. On X11, a refused ID makes Tela grab the keys
itself instead, so shortcuts work without the entry.

## One instance and launches

A second start of a document app should open the file in the running copy, and a
link clicked in a browser should reach it too.
[`desktop.SingleInstance(app)`](API-Reference-desktop#func-singleinstance) makes
the app a single-instance app, keyed by its `AppID` (so give it one):

```go
var App = desktop.SingleInstance(desktop.PersistApp(tela.App{
	AppID: "com.example.notes", Title: "Notes", Root: Notes{},
}))
```

A second process hands its arguments, working directory and activation token to
the first, then exits with status 0 without opening a window (`tela.Run` returns
nil at once). The first process raises its main window: shown if it was hidden in
the tray, and activated with the token so the compositor lets it come to the
front. The claim is made after the backend exists and before the first window, so
the second process never flashes a window.

The running app learns about the launch with the hook
[`desktop.UseLaunch`](API-Reference-desktop#func-uselaunch):

```go
desktop.UseLaunch(func(l desktop.Launch) {
	for _, u := range l.URLs() { // notes:open?id=3, https://...
		openLink(u)
	}
	for _, p := range l.Files() { // absolute; relative paths use the launcher's directory
		openNote(p)
	}
})
```

The handler gets the process's *own* start once, and every forwarded start after
that (`l.Forwarded` tells them apart). So the same code handles
`notes todo.md` typed into a terminal whether or not the app is running. `Files()`
and `URLs()` split the arguments: flags are neither, `file:` URIs (which Linux
launchers pass) count as files, and anything else with a scheme counts as a URL.
Anything else in `l.Args` is yours to parse.

`UseLaunch` is a hook: call it at the top of a component that is always mounted
(the root). The start is given to the handlers mounted with the first one, not to
those that mount later, so a screen that mounts again does not reopen the files.
When the desktop cannot tell instances apart (no D-Bus session on Linux), the app
simply runs on its own, with a dev warning that says why.

## Links and file types

To receive `notes:open?id=3` links or `.note` files from the OS, the app must be
*registered* as a handler. [`desktop.Register`](API-Reference-desktop#func-register)
does it at run time, with no installer and no administrator rights:

```go
// RegisterLinks makes the app a candidate for notes: links.
// It does file or registry I/O: call it at start, off the UI thread.
func RegisterLinks() (desktop.Registration, error) {
	scheme := desktop.URLScheme("notes")
	if _, err := desktop.Register(scheme); err != nil {
		return desktop.Registration{}, err
	}
	return desktop.RegistrationStatus(scheme)
}

var noteFiles = desktop.FileType(".note", "application/x-notes-note", "Notes document")
```

What registering does, and does not do, follows three rules the API enforces:

1. **It is per user.** Linux: the app gets its own `.desktop` file in
   `~/.local/share/applications` (and a MIME type for a new extension). Windows:
   keys under `HKEY_CURRENT_USER\Software\Classes`. Nothing needs administrator
   rights.
2. **It makes the app a candidate, not the default.** The app appears in "Open
   with", and a `notes:` link opens it when no other application is the default.
   Becoming *the* default is a separate request, [`desktop.MakeDefault`](API-Reference-desktop#func-makedefault),
   for when the user asks ("Make Notes the default app"). On Windows the user must
   choose the default for a *file type* themselves, so `MakeDefault` opens
   Settings' Default apps page and returns `DefaultAsked`: tell the user to pick
   the app there.
3. **It never takes what another application holds.** If another application
   owns the scheme, or its `.desktop` entry of the same ID was not written by Tela,
   `Register` returns an error wrapping `desktop.ErrAssociationTaken` and writes
   nothing. Pass `desktop.Replace()` only after asking the user.

`Register` looks before it writes: if the app is registered already it changes
nothing and returns `changed == false`, so calling it at every start is fine.
[`Unregister`](API-Reference-desktop#func-unregister) removes the app's
registration, and
[`RegistrationStatus`](API-Reference-desktop#func-registrationstatus) reports
whether the app is registered, whether it is the default and, in `Other`, what
holds the association instead. Show that in your preferences, with a Register and
an Unregister button and, when the app is registered but not the default, a "Make
default" one.

The functions work without a running app, so an installer or a `setup`
subcommand can call them with
[`desktop.ForApp(desktop.HandlerApp{ID, Name, Exec, Args, Icon})`](API-Reference-desktop#func-forapp)
to register the program it installed. By default the registered program is the
running executable, which is the right answer for a built app and the wrong one
for a temporary build: under `go run` register from the built program instead,
and under `tela dev` see [below](#under-tela-dev).

When the OS opens the app for a link or a file, it starts it with the link or the
path as an argument; [`UseLaunch`](#one-instance-and-launches) is how the app reads
it, with `SingleInstance` for the running copy.

## Secrets

[`desktop.SetSecret`](API-Reference-desktop#func-setsecret),
[`Secret`](API-Reference-desktop#func-secret) and
[`DeleteSecret`](API-Reference-desktop#func-deletesecret) keep small secrets
(passwords, tokens, encryption keys) in the OS's secure storage, named and scoped
to the app ID: the Credential Manager on Windows, the Secret Service (GNOME
Keyring, KWallet) on Linux. Never keep them in a plain file or in
`desktop.UseSetting`.

**These calls block**: the OS may show a prompt to unlock the keyring and wait for
the user. They are safe from any goroutine, so **call them from a goroutine, never
from a handler, an effect or a build**; dev mode warns when one runs on the UI
thread. Set state from the goroutine with a `Setter`, which is safe there:

```go
func (Vault) Build() tela.Node {
	state, setState := tela.UseState("locked")
	unlock := func() {
		go func() { // the keyring may be waiting for the user
			_, err := desktop.Secret("library-key")
			switch {
			case err == nil:
				setState.Set("unlocked")
			case errors.Is(err, desktop.ErrSecretNotFound):
				setState.Set("no key yet") // first run: make one and SetSecret it
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

The errors tell you what to do:

| Error | What happened | Do |
|---|---|---|
| `ErrSecretNotFound` | Nothing is stored under that name | Make the secret (first run), or say it is gone. Do not silently make a new key that cannot open data encrypted with the old one |
| `ErrSecretCanceled` | The user dismissed the unlock prompt | Nothing was read or written. Ask again when the user acts; never in a loop |
| `ErrUnsupported` | The desktop has no secure storage (a Linux session without a keyring) | Turn the feature off, with a reason the user can read |
| `ErrSecretTooLarge` | The value is above `desktop.MaxSecretSize` | Store a key, not the data |

The secrets need a running app (`tela.Run` or `telatest`); in `tela snapshot` they
return an error. `examples/notes` uses them for **locked notes**: a random AES key
made when the first note is locked and kept as the secret `library-key`, with the
notes in `notes.json` holding only ciphertext.

## The trash

[`desktop.MoveToTrash(path)`](API-Reference-desktop#func-movetotrash) moves a
file or folder to the user's trash (the Recycle Bin on Windows), where the user can
restore it. **It never deletes**: when the trash cannot take the file (a desktop
without one, a network drive, another disk than the trash), the file stays where it
is and the error wraps `ErrUnsupported`, so the app can say where the file still
is. A missing file wraps `fs.ErrNotExist`. Like the secrets it blocks: ask the user
first, then call it from a goroutine and give the outcome back with `tela.Post`.

```go
go func() {
	err := desktop.MoveToTrash(path)
	tela.Post(func() {
		switch {
		case err == nil:
			toast.Show("Moved " + filepath.Base(path) + " to the Trash")
		case errors.Is(err, desktop.ErrUnsupported):
			toast.Show("The file is still in " + filepath.Dir(path))
		default:
			toast.Error("Could not move the file", kit.ToastDescription(err.Error()))
		}
	})
}()
```

`examples/notes` offers it when a note opened from a file is locked: the note is
detached from the file, and the file, which still holds the text, can go to the
Trash after the user confirms.

## Native menus

`kit.ContextMenu` and `kit.Menubar` are drawn by Tela, in the look of the app, and
that stays the default: one look on every platform. A menu can opt in to the OS's
own menu, where the platform has one, with no other change:

```go
kit.ContextMenu[string]{
	Native: kit.NativeOn, // NativeAuto (the zero value) follows tela.App.NativeMenus
	Items: []kit.MenuItem[string]{
		{Key: "copy", Label: "Copy link", Shortcut: "Ctrl+L"},
		{Kind: kit.MenuSeparator},
		{Key: "delete", Label: "Delete", Variant: kit.Destructive, OnSelect: onDelete},
	},
	Children: Div(Class("rounded-lg border p-4"), Text(title)),
}
```

[`kit.NativeMode`](API-Reference-kit#type-nativemode) has three values: `NativeAuto`
follows the app, `NativeOn` asks for the OS's menu, and `NativeOff` keeps the menu
drawn. To make every menu native, set `tela.App{NativeMenus: true}`; a menu's own
`Native` wins over the app's in both directions, so one menu can stay drawn while
the rest are native.

Where the platform has no such menu, the drawn one is used without any check of
yours: Linux always draws, and a frameless Windows window has no native menu bar
(its title bar is yours). A native context menu leaves the drawn state, `Open` and
`OnOpenChange` untouched. `DropdownMenu` stays drawn: it hangs off a button and has
no OS counterpart.

What a native menu cannot show is dropped: icons, the destructive color, inset,
`KeepOpen`, classes and custom check or radio icons. Labels, shortcuts (shown as
the item's accelerator text), disabled items, separators, checkbox and radio items
(as check items) stay; a label becomes a disabled item and a group is
flattened. Choosing an item runs the same code as the drawn menu. The
accelerator text is only shown: handle the keys yourself with `OnKeyDown`.

Below kit, [`desktop.PopupMenu(win, at, items, done)`](API-Reference-desktop#func-popupmenu)
and the component [`desktop.MenuBar{Menus, OnError}`](API-Reference-desktop#type-menubar)
are the OS's menus, with `desktop.MenuItem` (the same type as the tray's, plus
`Accelerator`). They report `ErrUnsupported` where the platform has none, so you can
draw your own, as kit does:

```go
err := desktop.PopupMenu(win, tela.Point{X: e.X, Y: e.Y}, items, nil)
if errors.Is(err, desktop.ErrUnsupported) {
	setMenuOpen.Set(true) // draw the menu instead
}
```

## System settings

Tela follows more of the user's system settings than dark mode and reduced
motion: the accent color, high contrast and the text scale. Linux reads them from
the settings portal (so dark mode follows the system on Wayland and X11 too), and Windows
from the registry and its change notifications. A change reaches the running app
at once.

- **High contrast** is the `contrast-more:` variant: `border-border
  contrast-more:border-foreground`. `tela.UseHighContrast()` reads it in code.
  (`contrast-less:` is an error: Tela has no low-contrast setting.)
- **The text scale** works like a browser's default font size: 1rem is the
  theme's rem (16 px) times the scale. `text-*` sizes and rem-based Tailwind
  spacing (`p-4`, `gap-2`, `w-64`) and the breakpoints grow together, while px values
  (`border`, `text-[13px]`, `w-[320px]`) do not. It is on by default; a layout that
  cannot grow (a fixed-size tool window) sets `tela.App{IgnoreTextScale: true}`.
  `tela.UseTextScale()` returns it for sizes computed in code.
- **The accent color** is `tela.UseAccentColor()`, which returns the color and
  false when the system reports none.

```go
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

Snapshots and tests use a scale of 1, no accent and no high contrast unless set
(`telatest.TextScale`, `AccentColor`, `HighContrast`), so goldens are the same on
every machine. [Styling](Styling#high-contrast-and-the-text-scale) lists the
variant and the units.

## Under tela dev

`tela dev` runs your app as a child process of a long-lived host that owns the real
windows, so every service above goes through the host. A few consequences come
from that:

- **Drags** are forwarded to the active child, and the host answers the OS with
  the latest answer it has, one round trip late: on Windows, a drag that just
  entered is refused until the next `DragOver`, on X11 the answer waits for the next
  motion, and a `SetDropEffect` made while handling the drop does not reach the
  source. Your handlers are unchanged.
- **Global shortcuts** are held by the host, like trays. An equal re-registration is
  not sent again, so the Wayland portal does not ask on every save; changes apply
  when the new build is promoted, and the shortcut stays after a crash of the
  child. The host registers them under your app's ID, not its own, so the app's
  desktop entry must exist: run the built program once (`RegisterApp`), since
  `RegisterApp` under dev returns `ErrDevExecutable` like `Register`.
- **Single instance:** the host claims the app ID and keeps it across reloads. A
  second `tela dev` of the same app is a second instance: it forwards its launch to
  the first and exits, and the supervisor says why.
- **Registration needs an explicit `Exec`.** The running program is a temporary
  dev build, which the OS must not be told to run: `Register` and `MakeDefault` return
  `desktop.ErrDevExecutable` and write nothing. Give the installed program's path
  with `desktop.ForApp(desktop.HandlerApp{Exec: ...})`, or skip registration under
  dev (`examples/notes` does so, quietly).
- **`UseLaunch`** does not get the process's own start again on a reload, only the
  forwarded ones.
- **Secrets, associations, the clipboard and menus** go to the host's backend,
  served off its UI thread.

## Platform notes

Linux is Wayland and X11 over the XDG Desktop Portal and the freedesktop D-Bus
services. On macOS the clipboard works with every data type; drag and drop, menus,
shortcuts, single instance, registration, secrets and the Trash are the next phase
and return `ErrUnsupported`.

| | Linux | Windows |
|---|---|---|
| Drag and drop | Wayland `wl_data_device`, X11 XDND | OLE `DoDragDrop` and `RegisterDragDrop` |
| Clipboard | Text, HTML, PNG images, files and custom types on Wayland and X11 | The same types |
| Global shortcuts | Wayland: the GlobalShortcuts portal, which asks the user and may bind other keys (`Trigger`; none on Hyprland), and needs the app's desktop entry (`RegisterApp`); X11: the portal, else key grabs; `ShortcutFailed` with `ErrUnsupported` without a portal | `RegisterHotKey`: `ShortcutTaken` if another application holds the keys; no release events |
| Single instance | A well-known D-Bus name; without a D-Bus session the app runs on its own | A named mutex and a message to the first instance |
| Registration | The app's own `.desktop` file in `~/.local/share/applications`, `mimeinfo.cache` kept by Tela, and a MIME type for a new extension (made with `update-mime-database` when it is installed); `xdg-mime query default x-scheme-handler/notes` shows the default | `HKCU\Software\Classes`; the default for a file type is chosen by the user in Settings, Default apps |
| Secrets | The Secret Service (GNOME Keyring, KWallet) over D-Bus, with an encrypted session; `ErrUnsupported` without one | Credential Manager |
| Trash | The Trash portal, else the freedesktop home trash (`~/.local/share/Trash`) written by Tela for files on its disk; `ErrUnsupported` for files elsewhere | `IFileOperation` to the Recycle Bin; `ErrUnsupported` where Windows would delete the file instead (a network drive) |
| Native menus | Never: kit draws them | Context menus, and menu bars on windows with a title bar of the OS (not frameless) |
| Settings | The settings portal: dark mode, accent color, contrast, reduced motion, text scale | The registry, `SPI_GETHIGHCONTRAST` and `WM_SETTINGCHANGE` |

Linux runs two freedesktop helper programs when they are installed and works
without them: `xdg-open` (`tela.OpenURL` and `OpenPath` without a portal) and
`update-mime-database` (a MIME type for a new file extension; without it, files of a
new type are recognized later, once something else runs it). No other external
program is run.

## Testing

`telatest` plays the OS for all of this: a drag from a file manager, the user
pressing the shortcut, a second start with a link, the keyring, the registry, native
menus, the trash and the system's settings. Nothing touches the real clipboard,
keyring, registry, trash or shortcuts. See [Testing](Testing#drag-and-drop) for the drag helpers
and [the services](Testing#desktop-integration-in-tests) for the rest:

```go
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
	app.InjectLaunch(desktop.Launch{Args: []string{"notes:open?id=3"}})
	app.SetShortcutStatus("quick-note", desktop.ShortcutTaken, "")
	app.ExpectText("taken by another application")
}
```
