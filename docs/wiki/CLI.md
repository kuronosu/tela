# CLI

The `tela` command renders a package's screen without a window, so you (or an
agent) can look at a UI in a terminal: write the UI, `tela snapshot` it, look
at the PNG, `tela inspect` it if something is off, fix it. It works in any
module that requires Tela; it needs the `go` command in `PATH`.

Install it once and use it from any module:

```bash
CGO_ENABLED=0 go install github.com/kuronosu/tela/cmd/tela@latest

tela new my-app
cd my-app
tela dev
```

`tela new` is the quickest start; the rest of this page is about the commands you use in any module:

```bash
tela inspect .
tela snapshot -o screen.png .
tela dev .
```

`go install` puts `tela` in `$(go env GOPATH)/bin` (or `GOBIN`); make sure that
directory is in your `PATH`. Run it again to update, and check what you have
with `tela version`. Keep the installed command and the `tela` module your
app requires at the same version.

Without installing it, `go run` does the same each time, which is slower to
type and to start:

```bash
# From your own module (the tela module is already a dependency)
go run github.com/kuronosu/tela/cmd/tela inspect .

# From a clone of the repository
CGO_ENABLED=0 go run ./cmd/tela snapshot -o counter.png ./examples/counter
```

| Command | Does |
|---|---|
| `tela new` | Creates an app in a new directory (see [tela new](#tela-new)) |
| `tela snapshot` | Renders a screen to a PNG, or with `--film` the frames of its motion as a contact sheet and an animated PNG |
| `tela inspect` | Prints a screen's aria snapshot (YAML) or layout tree (JSON) |
| `tela lint` | Checks the Go source for the mistakes that dev mode reports at run time: unknown classes, roles and aria tokens, duplicate or missing keys, misused hooks, a redundant `Fragment`, a prop where a node goes (see [tela lint](#tela-lint)) |
| `tela dev` | Runs an app and reloads it on every save, keeping the window and the state |
| `tela mcp` | Serves a package's screen to agents over the Model Context Protocol: snapshot, screenshot, click, fill, film, test... on an instance that stays up and rebuilds on edits; `--attach` drives a running `tela dev`'s real windows instead (see [tela mcp](#tela-mcp)) |
| `tela lsp` | Serves class diagnostics, completion and hover to editors over the Language Server Protocol, beside gopls (see [tela lsp](#tela-lsp)) |
| `tela build` | Builds a program; for `GOOS=darwin`, as a macOS application bundle `<Name>.app`, for `GOOS=windows` as a GUI program with the app's icon and version information (see [tela build and tela run](#tela-build-and-tela-run)) |
| `tela run` | Builds a program and runs it, like `go run`; on macOS as a bundle in the user cache, so notifications, URL schemes and the Dock name work |
| `tela package` | Builds a program for each target and writes its packages: on Linux an AppImage, a deb and a flatpak manifest, the same bytes for the same inputs (see [tela package](#tela-package)) |
| `tela icons gen` | Generates a Go package of `tela.Icon` variables from a directory of SVG files (see [Icons](Icons)) |
| `tela version` | Prints the version of the tela module the binary was built from, its VCS revision when known, and the Go version and platform |
| `tela help [command]` | Shows the help |

Exit codes: 0 on success, 1 on failure (the message says what and how to fix
it), 2 on a usage error. `tela lint` exits 1 when it reports something, and 2 when
a package cannot be loaded. `tela add` is deferred, not available.

## What a package must define

The commands run the package's own code. The package defines one of:

```go
package screens

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is a single screen. A func() tela.Node Preview is mounted as
// tela.C(Preview), so it may use hooks itself.
func Preview() tela.Node { return Login() }

// or Previews, several named screens.
var Previews = tela.Previews{
	"Login":    {Root: Login()},
	"Settings": {Root: Settings{InitialTab: "account"}},
}

// Login is a function component: the public name carries its tela.C.
func Login() tela.Node { return tela.C(login) }

func login() tela.Node {
	return Div(Class("flex min-h-screen items-center justify-center bg-slate-100"),
		Div(Class("rounded-xl bg-white p-8 shadow-lg"), H1(Class("text-2xl font-bold"), Text("Log in"))))
}

// Settings is a struct component: its fields seed its state.
type Settings struct{ InitialTab string }

func (s Settings) Build() tela.Node {
	tab, _ := tela.UseState(s.InitialTab)
	return Div(Class("p-8"), H1(Class("text-2xl font-bold"), Textf("Settings: %s", tab)))
}
```

[`tela.Previews`](https://pkg.go.dev/github.com/kuronosu/tela#Previews) maps names to [`tela.Preview`](https://pkg.go.dev/github.com/kuronosu/tela#Preview) values. A
`Root` is a component: a node such as `Login()`, or a struct component
value, so it may use hooks. `--preview NAME` selects one; without it the CLI
renders the first name in byte order ([`Previews.Names`](https://pkg.go.dev/github.com/kuronosu/tela#Previews.Names)), so a name such
as `"1 Login"` can pick the default. When a package defines both, `Previews`
wins. `Preview()` may return a `tela.Node` or a `tela.Component`. What a
preview gives is mounted as is: a function component goes through its public
wrapper, which returns its `tela.C` node, as in `App.Root` (`Root: Notes()`,
or `Root: Notes{}` for a struct component).

The screen is drawn on a white canvas: give it its own background, or the
`App` a `RootClass`.

An `examples/` package such as `counter` shows the shape: `main` opens the
window, and `Preview()` makes the same screen available to the CLI.

### Starting a preview in a given state

The local state of a screen is seeded through the fields of a struct
component (`Settings{InitialTab: "account"}` above, with
`tela.UseState(s.InitialTab)`): no global is needed. State that comes from a
context is seeded with `Wrap`, which receives the root as a node and returns
it inside the providers the screen reads:

```go
var Previews = tela.Previews{
	"open":   {Root: Main{InitialTab: "open"}},
	"mocked": {
		Root: Main{},
		Wrap: func(n tela.Node) tela.Node { return AppState.Provider(mockState, n) },
	},
}
```

A preview may give a theme and a color scheme of its own, which win over the
package's `App` and over `--dark`:

```go
var Previews = tela.Previews{
	"Light": {Root: Main{}},
	"Dark":  {Root: Main{}, ColorScheme: tela.Dark},
	"Brand": {Root: Main{}, Theme: &brandTheme},
}
```

A `tela.Preview` is itself a component, so a test mounts one the way the CLI
does, its `Theme` and `ColorScheme` included: `telatest.Mount(t, Previews["mocked"])`.

Before `v0.7.0` the entries were functions (`"Login": Login`); write
`"Login": {Root: Login()}` now.

### One `App` for the window, `tela snapshot` and tests

When the package declares the `tela.App` its `main` runs as a package-level
variable (a `tela.App` or a `*tela.App`), the CLI renders in that window, so
the PNG matches what the window shows; a variable `App` of another type is
left alone:

```go
var App = tela.App{
	Title:       "Notes",
	Width:       900,
	Height:      600,
	RootClass:   "bg-background text-foreground",
	ColorScheme: tela.Dark,
	Theme:       &theme,
	Root:        Notes(),
}

func main() { tela.Main(App) }
```

| `App` field | In `tela snapshot` and `tela inspect` |
|---|---|
| `Width`, `Height` | The size of the window, unless `--size` is given (both must be set; otherwise 960x640) |
| `RootClass`, `ColorScheme`, `Theme`, `ClassAliases` | Applied, as in the window; the app's `ColorScheme` overrides `--dark`, as it overrides the system in a window, and a preview's `Theme` and `ColorScheme` override the app's |
| `ClassicScrollbars`, `InstantScroll`, `NativeMenus`, `IgnoreTextScale`, `ThemeTransitions`, `Dev` | Applied (`Dev` false silences the dev messages) |
| `Root` | The screen when the package has no `Preview` or `Previews`; otherwise ignored |
| `Title`, `AppID`, `DevState` | Ignored (`DevState` is for `tela dev`) |
| `MinWidth`, `MinHeight`, `MaxWidth`, `MaxHeight`, `FixedSize`, `Frameless`, `AlwaysOnTop`, `Hidden`, `Icon`, `TitleBar`, `Backdrop`, `Placement`, `OnClose` | Ignored: they shape the OS window, not its content. The windows a `tela.Window` opens are not rendered (the main window only), and the `desktop` calls return an error (no running app) |

[`telatest.MountApp`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#MountApp)(t, App) mounts the same `App` in a test, and
[`telatest.FromApp`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#FromApp)(App) gives the options of an `App` to
`Mount` and `Render`.

The screen is rendered at rest, as a window shows it after a moment: passive
effects have run (`UseEffect`, `UseMount`), the state they set and the toasts
they show are in the PNG and in `tela inspect`, and finite animations are
finished. A screen that never goes quiet (an endless animation, state set on
every frame) is shown at its frame after a few seconds of the app's time (state
set on every frame also gets a note on standard error).

The screen's dev warnings (a list without keys, a `nil` dependency) and class
errors are printed on standard error, so a command's output on standard output
stays clean.

## tela new

```bash
tela new my-app
tela new -module github.com/you/my-app my-app
tela new .            # in an empty directory
tela new -template notes my-notes
```

Creates the directory (it may exist if it is empty) with a `go.mod`, a
`main.go` with a counter and a `var App`, a `main_test.go` that clicks its
button through `telatest`, a README and a `.gitignore`, and the files for
coding agents (below). Then it runs `go get` and `go mod tidy`, so the app is
ready:

```bash
cd my-app
tela dev                      # or: CGO_ENABLED=0 go run .
CGO_ENABLED=0 go test ./...
```

| Flag | Does |
|---|---|
| `-module path` | The module path (default: the directory name, lowercased) |
| `-template name` | What to start from: `counter` (the default), `notes`, `settings` or `dashboard`; see below |
| `-skip-deps` | Do not run `go get` and `go mod tidy` (offline, or no `go` command); the command prints what to run |

The app requires the `tela` module at the version of the `tela` binary when
that is a release (`go install ...@v0.9.1` gives apps on v0.9.1), and at the
latest release when the binary was built from a checkout. A directory that is
not empty is refused and left untouched.

### Files for coding agents

Every template also writes three small files, so an agent that opens the
directory knows the loop without being told:

| File | Is |
|---|---|
| `AGENTS.md` | A short guide: read `llms.txt` (online, and the copy of the module at the version in `go.mod`, found with `go list -m -f '{{.Dir}}' github.com/kuronosu/tela`), write the UI, look at it with the `tela mcp` tools or `tela snapshot` and `tela inspect`, then `tela lint ./...` and `CGO_ENABLED=0 go test ./...` before finishing |
| `CLAUDE.md` | One line, `@AGENTS.md`, the import Claude Code reads |
| `.mcp.json` | The server entry that Claude Code and other MCP clients read: it starts `tela mcp .` in the app's directory, so `tela` must be in `PATH` |

They are the app's to edit.

### Templates

`counter` is the small app above. The other three start from a complete app,
the same one that is in `examples/` of the repository, with its tests:

| Template | Is | Needs |
|---|---|---|
| `notes` | A notes app: Markdown notes kept in SQLite, sign-in and sync (`oauth`), a router, a preview (`kit.Markdown`), desktop integration | `modernc.org/sqlite`, written to `go.mod` (see [SQLite](SQLite)) |
| `settings` | A preferences window: a form of sections, validation that blocks Save, a save bar, dialogs | |
| `dashboard` | An analytics dashboard: panes of glass over a wallpaper, a task board you can drag, dialogs, a command palette | |

They are copies made by `go generate ./cmd/tela` (a test fails when they are
out of date): the Go sources and assets of the example with its own import
paths changed to your module path, and its tests, including the text goldens
(`AriaGolden`). Left out are the tests that need this repository (benchmarks,
the GPU backend, memory and damage checks use its `internal/` packages) and
the **pixel goldens**: they depend on the Tela release that drew them, and
would add about 11 MB to the `tela` binary. A golden test of your own writes
its PNG the first time you run it with `TELA_UPDATE=1`. The window title, the
app name and the `notes:` link scheme are the example's: rename them when the
app becomes yours.

## tela snapshot

```text
tela snapshot [--size WxH] [--scale N] [--dark] [--preview NAME] [-o out.png] <pkg>
tela snapshot --film SPAN [--every DUR] [--apng anim.png] [flags] <pkg>
```

| Flag | Meaning |
|---|---|
| `--size WxH` | Logical size of the window (default: the package's `App.Width` and `App.Height`, else `960x640`) |
| `--scale N` | Display scale; fractional values such as `1.25` are allowed (default 1). The PNG is `W*N` by `H*N` pixels |
| `--dark` | Render in dark mode: `dark:` variants apply |
| `--preview NAME` | The named screen of a `Previews` variable |
| `-o file` | Output file (default `snapshot.png`, or `film.png` with `--film`: the contact sheet) |
| `--film SPAN` | Film the first `SPAN` (`600ms`, `1.5s`) of the app's clock instead of a still: see [Film](#film) |
| `--every DUR` | With `--film`: the time between frames (default `50ms`, at least `1ms`) |
| `--apng file` | With `--film`: also write every frame as an animated PNG |

```bash
tela snapshot --size 480x200 --scale 2 --dark -o counter-dark.png ./examples/counter
```

### Film

A still shows a screen at rest: finite animations are finished. `--film` shows
it moving. The screen is mounted with the controllable clock of `telatest`
stopped where it is (`NoSettle`) and its clock is advanced `--every` at a time
up to `SPAN`, a screenshot at each step, so the entrance a passive effect
starts, a `transition`, a `UseSpring`, a spinner or a `Presence` showing a
child can be seen. The clock is the app's, not the wall's: the film is the
same on every run and every machine.

```bash
tela snapshot --film 600ms --every 50ms --apng motion.png -o motion-sheet.png ./myapp
```

The output is a **contact sheet**: a grid of the frames, each scaled down
(the sheet stays within 1600 pixels a side, with the largest frames that fit)
and captioned with its time (`0 ms`, `50 ms`...). A film of more than 24
frames is sampled evenly for the sheet, first and last frame always in it. At
most 60 frames and 30 s: `SPAN / every` must stay within 60.

`--apng` writes every frame as an **animated PNG**, with the standard
library's `image/png` data inside the `acTL`, `fcTL` and `fdAT` chunks of the
format: it plays in browsers and image viewers, and a program that does not
animate shows the first frame. Frames after the first hold only the box that
changed, so a small motion in a big window is a small file; each lasts until
the next frame's time and the last stays a second. Browsers play frames
shorter than 20 ms slower than asked. A film starts at the mount; to film
what an action starts, use the `film` tool of [`tela mcp`](#tela-mcp).

Rendering uses Tela's own rasterizer and its standard fonts (the fonts of the
package's `App.Fonts` first, never the system's), so the PNG is the same on
every machine. It matches what a window shows when the app brings its own
fonts; an app that uses the system's fonts looks like Inter, JetBrains Mono
and Source Serif here and like the system's faces in its window
([Fonts](Fonts)). Snapshots of the
bundled examples are in
[`docs/images`](https://github.com/kuronosu/tela/tree/main/docs/images).

## tela inspect

```text
tela inspect [--aria | --layout] [--size WxH] [--dark] [--preview NAME] <pkg>
```

`--aria` (the default) prints the semantics tree as a Playwright aria
snapshot, one line per role with its accessible name and states; elements
without a role are left out and their text merged:

```yaml
- heading "Settings" [level=1] [src=settings.go:42]
- switch "Email notifications" [checked] [src=settings.go:47]
- button "Save changes" [disabled] [src=settings.go:55]
```

Each element line ends with `[src=file:line]`, the line of your source that
made the element (dev mode; an element made by a library component such as
`kit.Button` is shown at the line that wrote the component). The path is
relative to the package's directory when the file is inside it
(`settings.go:42`, `ui/card.go:7`); a file of another directory of the module
or of another module is named by its module path
(`example.com/app/internal/ui/card.go:7`).

`--layout` prints the layout tree as JSON, one object per box: its border box
in logical pixels (`rect`, from the top-left of the window), the resolved
margin, border and padding, the text and line count of paragraphs, the
element's classes, a summary of its computed style (display, size, flex
settings, color, background, font, opacity), **the class errors** of the
element and its source line (`"src": "settings.go:42"`, with `element`, the
identity of the element, for the tools of `tela dev`). A box painted elsewhere (a transform, scrolled content) also has
`bounds`, which is what `ElementRef.Bounds` returns. The order of the fields is
stable.

## tela dev

```text
tela dev [--size WxH] [--scale N] [--dark] [--preview NAME] [--no-state] [--devtools] [-v] [pkg]
```

Runs the app and reloads it every time you save, in the same window and with
its state: the counter keeps its count, the text field its text, caret and
undo history, the list its scroll position, the focused element its focus.
A reload of `examples/dashboard` takes about 0.6 s from the save to the new
frame.

```bash
tela dev ./examples/dashboard
tela dev --preview Empty ./ui        # one entry of a package's Previews
```

Go cannot replace code in a running process, so `tela dev` owns the window
and runs the app as a child process. On a save it builds the app in the
background (the running app stays usable), starts the new build with the old
one's state, and shows its first frame in place of the old one's: the window
never closes. `<pkg>` defaults to `.`; a package `main` is built as is, and
any other package (or `--preview`) gets a generated `main` that runs the same
screen `tela snapshot` renders, with the window settings of its `var App`.

**What triggers a rebuild:** any non-test `.go` file of the module, `go.mod`,
`go.sum`, files the module embeds, and files under directories registered with
`tela.Assets(os.DirFS(...))`.

**What is kept:** the values of `UseState`, `UseReducer`, `UseValue` and
`UseRef` that `encoding/gob` can encode in full (no funcs, channels or
unexported fields), matched by the component's place in the tree and the
hook's position, as React Fast Refresh does; the text, selection and undo
history of text fields; scroll offsets; the focus; the color scheme, theme
and locale set at run time. A component whose hooks changed (one added,
removed or of another type) starts fresh, with its children; in dev mode
every value that cannot be kept is named once on the terminal. Animations
restart from rest and effects run again, as on a new mount. State outside
the tree (package variables, controllers) is kept with
[`App.DevState`](https://pkg.go.dev/github.com/kuronosu/tela#DevState):

```go
// main.go
var devState tela.DevState // assigned only in dev builds

func main() { tela.Main(tela.App{Root: Notes(), DevState: devState}) }

// devstate.go
//go:build teladev

package main

func init() {
	devState = tela.DevState{
		Save:    func() ([]byte, error) { return json.Marshal(store) },
		Restore: func(b []byte) error { return json.Unmarshal(b, &store) },
	}
}
```

**Errors:** a build error keeps the running app and shows the compiler's
output in a panel docked at the bottom of the window; a panic shows its stack
over the last frame. Both are also printed on the terminal; `Esc` hides the
panel and the next good build clears it.

**Release builds do not change:** dev builds use the `teladev` build tag, and
everything of the dev loop compiles only with it; a plain `go build` links
none of it. Dev builds also skip the VCS stamp, the symbol table and DWARF,
and inlining in the app's package, to build faster (panics keep their
`file:line`, but a debugger cannot attach).

| Flag | Does |
|---|---|
| `--size WxH` | The window's logical size (default: the app's) |
| `--scale N` | Renders at scale N instead of the display's |
| `--dark` | The app sees the system in dark mode (`App.ColorScheme` still wins) |
| `--preview NAME` | Runs the preview `NAME` of the package's `Previews` |
| `--no-state` | Starts every build fresh |
| `-v` | Prints the timing of every step of a reload |

Keys, when standard input is a terminal: `r` reloads keeping state, `R`
reloads with fresh state, `q` quits (as does closing the window or Ctrl+C).
Linux (Wayland, X11), Windows and macOS (Cocoa, macOS 13 or later).

**Windows and desktop services under `tela dev`** (ADR-184): the window is the
host's and the app runs in a child process, and everything about windows and
services is forwarded: the window options (`Frameless`, size limits, icons...),
window controls and the hit regions of a custom title bar, file dialogs,
message boxes, notifications and the tray. The windows a
[`tela.Window`](Windows-and-Desktop#several-windows) opens are real windows too,
matched across a reload by their place in the tree, so a preferences window stays
where it was while you edit its content (it opens anew if its `Frameless`,
`FixedSize`, `Modal` or parent changed). A window that was hidden to the tray stays
hidden, and trays and notifications are not duplicated by the effects that run
again on the new build. Known limits: a window that a new build opens only later
(from an effect) closes and reopens on reload, and a dialog opened at startup
shows again on every reload.

**Windows and Microsoft Defender:** Defender scans every new executable on its
first run, and every reload is a new build: on a Windows 11 machine that made a
reload of `examples/dashboard` take 2.2 s instead of 1.1 s. When the build of a
reload takes more than 400 ms to start, `tela dev` prints once the
directories to exclude from real-time scanning, its build directory
(`%TEMP%\tela-dev`, shared by all sessions when it is your own) and the Go build cache
(`go env GOCACHE`), with the PowerShell command to run as administrator:

```powershell
Add-MpPreference -ExclusionPath "$env:TEMP\tela-dev", "$(go env GOCACHE)"
```

`tela dev` never changes these settings itself; whether to exclude them is
your call.

## tela devtools

```text
tela devtools [-addr ADDRESS]
tela dev --devtools [pkg]
```

A DevTools window for the app a `tela dev` session runs, in the spirit of
Chrome's and React's: it is a Tela app built with `kit`, inside the `tela`
binary. Start the session, then the window (or let `tela dev --devtools` start
both):

```bash
tela dev ./examples/settings      # in one terminal
tela devtools                     # in another
tela dev --devtools ./examples/settings   # or both at once
```

`tela devtools` attaches to the session's tools socket, which `tela dev` opens
while it runs (and writes to `<user cache>/tela/dev/<pid>.json`). With one
session running it needs no argument; with several, name one by the address
`tela dev -v` prints. Closing the DevTools leaves the session running; when
the session ends the DevTools close too. Release builds and plain `go run`
never listen, so there is nothing to attach to outside `tela dev`.

The window follows the app live and survives its reloads. Four panels:

- **Elements**: the element tree of the window (components, primitives and
  text, with keys and, for components, where in the source they are made),
  folding, a search over kinds, keys, text and classes, a *Components* view
  that leaves out the primitives, and a window picker when the app has several
  windows. The crosshair turns on **pick mode** in the app: the box under the
  pointer is highlighted, a click selects it here (and does not reach the
  app), Esc cancels. Hovering a row highlights its element in the app. The
  selection shows its source line (`settings.go:42`), **why it rendered** (the
  causes of its last builds, newest first: a `UseState` and the line that
  declared it, new props and which fields differ, a context, a parent that
  built it), its hooks with their values, its props, the box model (margin,
  border, padding and content, as Chrome draws it) and its computed style.
- **Accessibility**: the semantics tree as `tela inspect` prints it. Selecting
  a node selects its element, with the same details.
- **Profiler**: *Record*, use the app, *Stop*. Each frame is a bar stacked by
  phase (input, animate, build, style, layout, paint, semantics, raster,
  present, effects) against the 16.7 ms line; a summary counts the frames,
  the ones over budget and the slowest phase. Click a frame for its time per
  phase, its counts and damage, and the elements it built with the reason of
  each build; a row opens the element in the Elements panel.
- **Logs**: what the app printed on standard error, the warnings and errors
  of its trees (unknown classes, duplicate keys, state that a reload cannot
  keep; once each, though the app prints them on standard error too) and
  `tela dev`'s own lines (building, a failed build with its errors),
  filterable by level, source and text. Entries logged while the app is idle
  arrive at once.

The look follows the system's color scheme: graphite in the dark, a calm grey
in the light.

## tela lint

```text
tela lint [-json] [-tests] [-fix] [packages]
```

Dev mode finds a typo in a class only on a screen that renders it. `tela lint`
finds it in the source, so it fails in CI with dev mode off (ADR-173). It
checks the packages (default `./...`, written as for `go build`; it needs the
`go` command in `PATH`) and calls the runtime's own checks, so the messages and
the "did you mean" are the ones of dev mode.

```text
$ tela lint ./...
ui/home.go:42:31: class: unknown class "rounded-xxl"; did you mean "rounded-2xl"?
ui/home.go:57:12: role: Role("buton"): unknown ARIA role "buton"; did you mean "button"?
ui/list.go:23:3: hooks: hook tela.UseState is called conditionally (inside an if); ...
```

Only **constant** strings are checked: a literal, a `const`, a concatenation of
constants (also through dot imports, renamed imports and `const` names).
Anything computed (`"p-" + strconv.Itoa(n)`) is skipped, never guessed.

| Rule | Checks |
|---|---|
| `class` | The classes of `h.Class` and `h.Exit`; the `Class` and `...Class` fields of the `kit` components, the string fields of the `kit` recipes (`kit.Style` and the `...Style` structs, also when assigned: `s.Button.Root = "..."`), `tela.App.RootClass`, `telatest.RootClass`; the values of `ClassAliases`. Unknown, unsupported and invalid classes are reported at the class inside the string. The class aliases (`tela.App.ClassAliases`, `telatest.ClassAliases`) and the theme colors (map literals of `tela.SchemeColor`, such as `Theme.Colors` and `theme.Colors["brand"] = ...`, and the constant arguments of calls to a function that stores a color under a name parameter, `set("gold", ...)` with `set := func(name string, ...) { t.Colors[name] = ... }`, also through a second function that passes the name on) declared in the linted packages are known: lint `./...` when you use them, and write the color names as constants (a name built at run time cannot be known) |
| `role` | `h.Role` with a role that is not a WAI-ARIA 1.2 role |
| `aria` | `h.AriaCurrent`, `h.AriaSort`, `h.AriaHasPopup`, `h.AriaOrientation` and `h.AriaAutoComplete` with a value outside their tokens |
| `prop` | `h.Dir`, `h.Type` (against the types of the `h.Input` or `h.Button` it is given to) and the `Type` of `kit.Button` and `kit.Input`, and `tela.Ease`, with a value they do not take |
| `key` | Two siblings with the same constant `h.Key` (or `tela.WithKey`, or the key of `tela.C`, `tela.CP`, `tela.Memo`), and an `h.Map` or `h.MapIndex` whose function returns elements without a `Key`. Dev mode only warns about these, so their severity is `warning` |
| `hooks` | A hook (`Use...` of `tela`, `aria`, `kit`, `i18n`, or a custom hook of your code) called conditionally, in a loop, after an early return, from a function that is neither a component nor a custom hook, or in a function literal that is not a component; and a component (a function or `Build` method that uses hooks) called directly instead of through `tela.C`, `tela.CP` or an element. A function with the shape `func() tela.Node` or `func(P) tela.Node` is given the benefit of the doubt. A function is a custom hook when it is named `UseXxx` or `useXxx` and calls a hook |
| `fragment` | `h.Fragment(x)` with a single child and no spread: a struct component is a `tela.Node` (ADR-205), so write `x`. It has a fix (`-fix`), and is reported only where removing the wrapper still type-checks and keeps the child's identity (its state): `x` is not a `Prop` or a `tela.Child` that could hold one (`Fragment(Key(k))` sets the key of the fragment), the place takes a `tela.Node` or `tela.Child` (an argument, an element or a field of a literal, a result, an assignment to a declared type; not `n := Fragment(x)`, an `any` or a generic parameter), no comment sits inside the call, the first argument of `tela.WithKey` is a struct literal, and the same function (or the same `IfElse`) has no `Fragment` of several children as an alternative: a child that moves between `Fragment(x)` and `Fragment(x, y)` keeps its state, and one that moves between a bare `x` and `Fragment(x, y)` does not |
| `prop-node` | A `Prop` or `StyleProp` (`h.Class`, `h.Key`, `h.OnClick`, `tela.Width`...) where a `tela.Node` is expected: the result of a function or of a `Map` callback, an element of a `[]tela.Node`, a field of type `tela.Node` or `tela.Component`, an argument of `tela.WithKey`, an `If` of props. A prop has a `Build` method, so it compiles, and it panics when the node is consumed. Props go to an element: `Div(Class("p-4"), ...)`. A `tela.Child` takes props, so it is not reported |

Each finding is `file:line:col: rule: message`, sorted by position, with paths
relative to the current directory. The exit status is 0 when nothing is
reported, 1 when something is, and 2 on a usage error or when a package cannot
be loaded (a package that does not type-check is reported on standard error and
skipped).

```text
//tela:ignore class -- tests the dev-mode error
Div(Class("rounded-xxl"))
```

A `//tela:ignore rule` comment on the line of a finding, or alone on the line
above it, silences it. Several rules are separated by commas, with or without
spaces (`class,role` or `class, role`); text after the list is a reason (by
convention after `--`), and a `//tela:ignore` without rules (bare, or with
only a reason: `//tela:ignore -- generated`) covers every rule.

| Flag | Does |
|---|---|
| `-json` | Prints the findings as a JSON array of `{file, line, col, rule, severity, message, suggestion}` (`suggestion` only when the message proposes a replacement; `severity` is `error` or `warning`; `[]` when there is nothing) |
| `-tests` | Lints the `_test.go` files too (by default they are skipped: tests often use a wrong class on purpose) |
| `-fix` | Applies the fixes of the findings that have one (today the `fragment` rule): the files are rewritten and formatted with gofmt, an import that the fix left unused is removed, and a file whose fixed text would not parse is left as it was. It prints `fixed N findings in M files` on standard error and reports what is left. Review the result with `git diff` |

Limits: only the files that the current platform and build tags select are read;
aliases and theme colors come from the linted packages, so lint `./...` when you
use them; a theme color whose name is built at run time is unknown, so a class
that uses it is reported (write the names as constants, or silence it with
`//tela:ignore class`); the `Type` of the `aria` options and the `Variant` and
`Size` of the `kit` components are not checked.

Run it in CI after `go vet`:

```bash
CGO_ENABLED=0 go run github.com/kuronosu/tela/cmd/tela@latest lint ./...
```

## tela mcp

```text
tela mcp [--size WxH] [--scale N] [--dark] [--preview NAME] [-v] [pkg]
tela mcp --attach [-v] [addr]
```

A [Model Context Protocol](https://modelcontextprotocol.io) server over
standard input and output (ADR-174): an agent drives the package's screen, the
one `tela snapshot` renders, through tools, the way Playwright MCP drives a
browser. The screen is mounted headlessly with `telatest` in a process that
stays up between calls, so a call answers in milliseconds. `<pkg>` defaults to
`.` and may not exist yet when the server starts: the agent can write it and
call `snapshot`.

Add it to your client with the command line that starts it. For Claude Code:

```bash
claude mcp add tela -- tela mcp ./myapp
```

or in a client's JSON configuration (`.mcp.json`, Claude Desktop, Cursor...):

```json
{
  "mcpServers": {
    "tela": { "command": "tela", "args": ["mcp", "./myapp"] }
  }
}
```

Before each call the server checks the package's sources (the files `tela dev`
watches, plus the package's own `_test.go` files). When they changed, it
rebuilds and mounts the screen afresh, so the state is reset, and the result
says so. A compile error is the result of that call, with the findings of
`tela lint`; the last good build keeps answering the other tools until the
source compiles again. A rebuild also reports the lint findings of the new
source, and every result carries the dev errors and warnings the screen logged
(an unknown class at run time with its "did you mean", a list without keys).

The screen is the main window's: the windows a `tela.Window` opens and the `desktop`
services are not reachable through the tools yet.

| Tool | Does |
|---|---|
| `snapshot` | The aria snapshot (the YAML of `tela inspect`), each element that can be acted on marked `[ref=eN]` |
| `screenshot` | A PNG of the window, or of one element (`ref`) with an 8 px margin |
| `click` | Clicks an element, or a point (`x`, `y`); `button: "right"`, `double: true` |
| `hover` | Moves the pointer over an element |
| `fill` | Replaces the text of a field; `submit: true` presses Enter after |
| `press` | Presses a key or combo in Playwright's syntax (`Enter`, `Shift+Tab`, `Control+a`), on the focused element or on one it focuses first |
| `scroll` | Scrolls the wheel over an element by `dx`, `dy`; without an element, at a point (`x`, `y`) or at the center of the window |
| `drag` | Drags `from` an element or point `to` another, or by `dx`, `dy` |
| `resize` | Changes the window's `width` and `height` or `scale`, keeping the state |
| `open` | Mounts another `preview`, `dark` mode, size or scale afresh; without arguments, lists the previews |
| `wait` | Advances the app's clock by `ms` (timers, animations), or waits for a `text` to appear or `textGone` to go |
| `inspect` | The layout tree of `tela inspect --layout`, whole or of one element |
| `state` | Why a component rendered and what it holds: props, hooks (kind, call site, value), build count and the reason of each of its last builds, for a `ref` (its nearest component) or for the screen's components that have props or hooks (`max`) |
| `logs` | The dev warnings and errors and the app's output since the last `logs` call, with levels (`level`, `all`) |
| `perf` | The last `frames` frames, or with `action: true` the frames the last click, press or wait caused: time per phase, what each built, styled, laid out and repainted, and a summary |
| `prefer` | `reducedMotion`, `highContrast`, `textScale`, `locale`, `direction` and `dark`; without arguments, the current ones |
| `lint` | `tela lint -json` on the package |
| `film` | The frames of the motion an `action` starts, every `every` ms over `span` ms of the app's clock, as a contact sheet image (and, with `apng`, an animated PNG): see [`film`](#film-and-test) |
| `test` | Writes the session's actions, and what each changed, as a Go `telatest` test in the package, then runs `go vet` and the test: see [`test`](#film-and-test) |

An element is named by the `ref` of the latest snapshot, or by `role` and
`name`, `text` or `testId` (`telatest`'s locators; `nth` picks one of several).
Every tool that acts answers with the new snapshot. A ref is the element
itself, not its role and name: it stays with the element across actions
and rebuilds of the tree whatever happens to its label or its siblings (a
counter's text changes, rows before it are removed), for as long as the
element is mounted, and a ref whose element was unmounted is the error `ref
e4 is gone: take a new snapshot`. After the Go source changed and the screen
was mounted afresh the refs are matched again by role, name and position, so
`screenshot {"ref": "e5"}` right after an edit shows the same element
rebuilt. A failed action (an element not found, a disabled button, a panic in
a handler) is an error result of that call: the instance keeps running, and
after a panic it mounts the screen afresh. An element that is not on the
screen fails at once when nothing is pending (no animation, timer or queued
update); while something is, the call waits up to two seconds for it to
appear. Use `wait {"text": "Loaded"}` for data that arrives from a goroutine.
The client's `notifications/cancelled` stops a wait or an action in progress,
keeps the app's state and sends no response for that call.

**Source lines.** Each element line of `snapshot` and of every acting tool
ends with its ref and then `[src=file:line]`, the line of the app's source
that made it, so the agent edits the right place without searching; the
paths are those of [`tela inspect`](#tela-inspect), and so is the `"src"` of
`inspect`.

```yaml
- button "Count: 1" [ref=e4] [src=counter.go:21]
```

**`state`, `perf`, `logs`.** `state {"ref": "e4"}` answers for the component
that rendered the element: the setter behind its last build is named with its
line.

```text
### app.Counter
hook #1 UseState at counter.go:14 = 1
built 2 time(s); why, newest first:
- frame 2: UseState #1 at counter.go:14 was set
- frame 1: mounted
```

`perf {"action": true}` lists the frames the last action caused. Its times
are those of the instance on this machine (headless, no GPU): compare frames
with each other, and read the counts (built, styled, laid out, repainted
pixels) as the exact part. `logs` lists what the screen and the app logged
since its last call: the dev warnings and errors of Tela (`dev`) and what the
app printed (`app`; a `log/slog` or `log` line carrying `ERROR` or `WARN` gets
that level); every other result already carries the ones it caused.

**`prefer`.** What the user of the app prefers, to see the screen in it:
`{"reducedMotion": true}`, `{"textScale": 1.5}`, `{"highContrast": true}`,
`{"dark": true}` and `{"locale": "es"}` change while the app runs, as in the
system's settings: the state is kept, the components that read them rebuild,
and the answer says each one is `live`. The locale is
[`tela/i18n`](Internationalization)'s (a package that does not import it says
the preference has no effect). `{"direction": "rtl"}` puts an `h.Dir` around
the whole screen, which is part of the tree: the screen is mounted afresh
(state reset, refs new) with every preference set, and the answer says so.
The preferences stay across rebuilds and `open`. `dark` is also an argument of
`open`, which always mounts afresh.

### `film` and `test`

**`film`.** A transition, a spring or a `Presence` exit is over by the time
`screenshot` looks. `film` captures the motion on the app's clock and returns it
as one image the agent can judge:

```json
{"span": 600, "every": 50, "action": {"click": "e5"}}
```

`action` starts the motion: one tool and its arguments, `{"click": "e5"}`,
`{"hover": "e3"}`, `{"press": "Escape"}`, or an object for the others
(`{"fill": {"ref": "e7", "text": "ana"}}`, `scroll`, `drag`), resolved like the
tool itself (refs, role and name). Without it, the app as it is is filmed
(a spinner, a timer, a `UseInterval`). The result is the **contact sheet** (a
grid of at most 24 frames, each scaled down and captioned with its time; at most
60 frames and 30 000 ms in all) and the snapshot of the app once settled, so the
last frame can be compared with where it ends up. `apng` also writes every frame
as an animated PNG to a path. The same sheet and animation come from
[`tela snapshot --film`](#film) for the motion a mount starts.

**`test`.** The session becomes a regression test:

```json
{"name": "checkout flow"}
```

writes `checkout_flow_test.go` in the package (or `path`, a `_test.go` file in
its directory; a relative path is relative to it), then runs `go vet` and
`go test -run ^TestCheckoutFlow$` on it and reports the result with the source.
What it records is the acting calls since the screen was last mounted: `click`,
`fill`, `press`, `hover`, `scroll`, `drag`, `wait`, `resize`, the actions of
`film`, and `prefer` where `telatest` has the method (dark mode, reduced motion,
high contrast, text scale). The mount has the session's preview, size, scale,
dark mode and preferences (and the package's `App`); `open` starts another
mount in the same test; a change of the Go source starts the recording over,
because the steps no longer say what the screen does. Each action is the
`telatest` call for it, and an element is the most specific stable locator for
it, never a node identity:

```go
app := telatest.Mount(t, Previews["Form"], telatest.Size(480, 320))
app.Click(telatest.ByRole("button", "Count: 0"))
app.ExpectRole("button", "Count: 1")
app.Fill(telatest.ByRole("textbox", "Name"), "Ana")
app.ExpectValue(telatest.ByRole("textbox", "Name"), "Ana")
app.Press("Enter")
app.ExpectText("Hello, Ana!")
app.Advance(100 * time.Millisecond)
```

A ref becomes `ByRole(role, name)`, with `.Nth(i)` only when several elements of
the snapshot look alike; a `testId` the agent gave stays `ByTestID`, and
`text` stays `ByText`. After each action come checks of **what changed** in the
snapshot (an element that appeared, a state that flipped to `Checked(true)` or
`Expanded(true)`, a field's value, a text shown or gone, an element removed),
at most eight, not a copy of the whole snapshot: the test breaks when the
behavior does, not when the layout around it is edited. The file begins with a
comment that marks it as the tool's; the tool replaces such a file and refuses
to overwrite another one. If the test does not compile or vet it is not kept
(the error shows what was written); if it compiles but fails it is kept, with
the failure.

Neither tool is available over [`--attach`](#tela-mcp---attach) yet: they need
the headless app's controllable clock and mount.

The instance is built against the `tela` your module requires, which may be
older than the `tela` binary: the source lines, `state`, `perf` and `prefer`
need the module to be on a version with the DevTools additions of `telatest`,
and `film` and `tela snapshot --film` one with `App.Film`
(`go get github.com/kuronosu/tela@latest`); without them the other tools keep
working and these say what to update.

```text
→ snapshot {}
Built and mounted in 1.1s.
- heading "Settings" [level=1] [ref=e1]
- switch "Weekly digest" [ref=e12]
- button "Save changes" [disabled] [ref=e32]
→ click {"ref": "e12"}
Clicked switch "Weekly digest" [ref=e12].
- switch "Weekly digest" [checked] [ref=e12]
- button "Save changes" [ref=e32]
```

With `-v` the server logs the protocol, the builds and the instance's output on
standard error; standard output carries only protocol messages. The instance is
a test binary built with `go test -c` from a generated `_test.go` file in an
overlay (nothing is written into the package), so the server works in any
module that requires Tela.

### tela mcp --attach

```text
tela mcp --attach[=addr] [-v] [addr]
```

The same server, driving the **real windows** of a running [`tela dev`](#tela-dev)
session instead of a headless instance (ADR-299). It talks to the session's
tools socket: with no `addr` it finds the only session that runs (the discovery
files `tela dev` writes in the user cache); with several it fails and lists
them, so name one by the socket path; with none it says to start `tela dev`,
and every tool answers that until a session runs. Start it in the directory
`tela dev` runs in, so the source lines and `lint` resolve the same paths:

```bash
claude mcp add tela -- tela mcp --attach
```

The tools are the headless ones with a `window` argument (an ID of the new
`windows` tool, default the first window the app shows) and these differences,
because the app, its clock and its window are the person's and `tela dev`'s:

| Tool | In attach mode |
|---|---|
| `windows` | New: lists ID, title, logical size, display scale and which window has the keyboard focus |
| `snapshot` | The real window's aria snapshot, with refs and `[src=file:line]`. Refs are semantics node IDs per window, as in the headless server; after a `tela dev` reload (a new child, whose IDs start again) they are matched again by role, name and position, and the result says the app was built again |
| `screenshot` | The frame the window presents, in real pixels at the display's scale (the answer says it: 480x400 logical at scale 2 is 960x800 pixels); with a `ref`, that element with an 8 px margin |
| `click`, `hover`, `fill`, `press`, `scroll`, `drag` | Inject pointer, key and text events into the real window at the element's painted position (the path a person's input takes), wait until the app has drawn no frame for 100 ms (at most 5 s: an animation never settles, and the answer says so), and answer with the new snapshot. `fill` clicks the field, selects all and types the text, so `OnChange` fires per character; `press` with a `ref` clicks the field to focus it, and refuses an element a click would activate (use `Tab`). An element's center must be inside the window: scroll it into view first. `testId` is not available (the snapshot has no test ids): use `ref`, `role` and `name`, or `text` |
| `wait` | Waits real time (`ms`, at most 60 s) and/or up to 10 s for a `text` to appear or `textGone` to go: the app's clock runs by itself |
| `inspect`, `state`, `perf`, `logs` | Read the running app through the instrumentation `tela dev` turns on for the attached client: the layout tree, the component behind a `ref`, the frames' time per phase (`action: true` for the frames of the last acting call) and what `tela dev` logged (the dev warnings and errors of Tela, and the app's standard error; standard output is not kept) |
| `lint` | `tela lint -json` on the session's package, as `tela dev` was given it |
| `resize`, `open`, `prefer` | Not available (they answer so): the window and the system's preferences are the person's, and there are no previews to mount; use the headless server |

`[src=]` is known for what the app built since the session was instrumented
(the first call): an element built earlier has none until it rebuilds, so a
snapshot taken right after attaching shows few until the app changes or
reloads. The paths are relative to the directory `tela mcp --attach` runs in,
like `tela dev`'s; the `Site` of a hook is shown as that process sees it.

## tela lsp

```text
tela lsp [-v]
```

A Language Server Protocol server on standard input and output, for your editor
to run **beside gopls** on the Go files of a Tela app. It knows what Tela knows
about Tailwind classes and nothing else about Go, so it answers three things
and leaves the rest to gopls:

- **Diagnostics:** the findings of [`tela lint`](#tela-lint) (unknown classes
  with "did you mean", roles, aria tokens, keys, hooks) for the open files, from
  their unsaved text. They are published when a file opens, a moment after you
  stop typing and on save. The aliases of `App.ClassAliases` and the theme
  colors declared in the package are known. A file that does not compile is
  still linted with what type-checks.
- **Completion** inside a class string: utilities and theme keys, variants
  (`hover:`, `md:`, `dark:`, stacked), your class aliases and arbitrary values
  (`w-[...]`), each with the CSS it produces (`p-4` is `padding: 1rem`, a color
  its hex).
- **Hover** over a class: the CSS it produces once the theme is resolved and
  when it applies (`hover:`, `dark:`, `md:`); over an alias, its expansion.

A string is a class string when it is an argument of `Class(...)`, the value of
a field named `Class` or ending in `Class` (`kit.Button{Class: "..."}`,
`App.RootClass`), or a value of `ClassAliases`; once the file has been linted
every string that `tela lint` checks is one (the fields of a `kit` recipe, for
example). Completion and hover read the text, not the type checker, so they
answer at once and while the file does not compile. Diagnostics need the `go`
command in `PATH`; a package takes about 150 ms to lint again after the first
run (the first one builds the export data of its dependencies, as `tela lint`
does), and the editor sees them 300 ms after the last keystroke.

Configure the editor to start `tela lsp` for Go files, next to gopls. In VS Code,
an extension that runs a stdio language server, such as Generic LSP Client:

```json
{
  "glspc.server.command": "tela",
  "glspc.server.commandArguments": ["lsp"],
  "glspc.server.languageId": ["go"]
}
```

Neovim 0.11 or later:

```lua
vim.lsp.config('tela', { cmd = { 'tela', 'lsp' }, filetypes = { 'go' }, root_markers = { 'go.mod' } })
vim.lsp.enable('tela')
```

Helix, in `languages.toml`:

```toml
[language-server.tela]
command = "tela"
args = ["lsp"]

[[language]]
name = "go"
language-servers = ["gopls", "tela"]
```

Zed only starts the language servers that an extension declares, so it needs
one that runs `tela lsp`; there is none yet. With `-v` the server logs what it
does on standard error; standard output carries only protocol messages.

## tela build and tela run

```bash
tela run ./cmd/notes                 # build and run; on macOS, as a bundle
tela run . --some-flag value         # what follows the package goes to the program
tela build -o dist ./cmd/notes       # dist/Notes.app on macOS, the program elsewhere
GOOS=darwin GOARCH=arm64 tela build -o dist .   # from Linux or Windows
GOOS=windows tela build -version 1.2.0 -o dist . # dist/notes.exe: GUI program, icon, version
```

Both build the program of a package `main` (the default is `.`) with
`CGO_ENABLED=0` for the `GOOS` and `GOARCH` of the environment. Plain `go run`
and `go build` stay unbundled, and they are fine for everything that needs no
bundle.

On macOS some services belong to an *app*, not to a process: user
notifications, URL schemes and file types, the keychain's access group, the
name in the Dock and the menu bar. An app is a bundle, `<Name>.app`, with an
`Info.plist` that names it. `tela build` for darwin writes one:

```text
Notes.app/Contents/Info.plist
Notes.app/Contents/MacOS/Notes
Notes.app/Contents/Resources/notes.icns     # with an icon
```

The `Info.plist` has `CFBundleIdentifier` (the app's `AppID`), `CFBundleName`
and `CFBundleDisplayName` (its title), `CFBundleExecutable`, the package type,
the version (`CFBundleShortVersionString`, `CFBundleVersion`: the module's
version when the build knows it, else `0.0.0`), `LSMinimumSystemVersion` 13.0
and the high-resolution and graphics-switching keys, then what the rest of the
declaration gives, as `tela package` writes it ([macOS](#macos)): the icon
(`icon.svg` or `icon.png` in the package's folder, or `-icon`, drawn into
`Resources/<name>.icns`), the category, the copyright, the usage descriptions
and the URL schemes and file types.

The ID and the name are read from the source, without running it: the
`AppID` and `Title` of the `tela.App` passed to `tela.Main` in `main`, or of the
package-level `var App`. Literals, constants and concatenations work, and so
does an `App` that a function wraps (`tela.Main(desktop.SingleInstance(App))`).
Without an ID the command stops and says to set `tela.App.AppID`, or to pass
`-id`. `CFBundleIdentifier` takes letters, digits, hyphens and periods only.

`tela run` builds the same bundle in the user cache folder, under a folder of
the app's ID that stays the same from run to run, so Launch Services sees one
bundle per app:

```text
~/Library/Caches/tela/run/dev.example.notes/Notes.app
```

and runs the executable inside it directly (not through `open`), so the
terminal keeps standard input, output and error and Ctrl+C, and the program's
exit status is `tela run`'s. `-v` prints where the bundle is. Rebuilding writes
only what changed, and keeps the entries that a running app added to the
`Info.plist` when it registered a URL scheme or a file type that the
declaration does not declare.

| Flag | Does |
|---|---|
| `-o path` | (build) A folder to create `<Name>.app` in, or a path ending in `.app`; otherwise the executable file or folder. Default: the current folder |
| `-id ID` | The app's ID, instead of `App.AppID` |
| `-name NAME` | The app's name, instead of `App.Title` (default: the package's name) |
| `-version V` | The app's version (`1.2.3`, `1.2.3-rc.1`), written into the program; default: the module's version |
| `-icon file` | (build) The icon of a bundle or a Windows program, instead of `icon.svg` or `icon.png` in the package's folder |
| `-console` | (build) A Windows program keeps the console subsystem instead of the GUI one |
| `-tags list`, `-ldflags f`, `-gcflags f`, `-trimpath` | Passed to `go build`; tela adds `-X` for the version and, for a Windows GUI program, `-H=windowsgui` to `-ldflags` (yours wins for `-H`); no `teladev` tag, no stripping |
| `-v` | Say where the ID and the name came from, the icon (and, for `run`, where the bundle is) |

Signing: neither command runs `codesign`. The Go linker ad-hoc signs
`darwin/arm64` executables, and that signature covers the executable only, not
the `Info.plist`. A bundle made this way runs on the machine that built it; it
has no developer identity and is not notarized. Signing, notarization and
installers are `tela package`'s. On Linux `tela build` writes the plain executable and `tela run`
builds and runs it in a temporary folder; `-id` and `-name` are ignored. `tela
run` always runs on the machine's own platform.

### Windows

For `GOOS=windows` the program is a GUI program: `tela build` links it with
`-H=windowsgui`, so Windows opens no console window next to it (`-console` keeps
the console subsystem, for a command-line tool; a GUI program has no standard
output, so it cannot print). `tela run` keeps the console, since it is a
development command.

The executable also gets the resources that Explorer, the task bar and Task
Manager show, added to the linked file (the package's folder is not touched; a
package with a `.syso` of its own keeps its resources and gets none):

- the **icon**, from `icon.svg` or `icon.png` in the package's folder, or
  `-icon`: an SVG is drawn at 16, 24, 32, 48, 64, 128 and 256 px; a PNG must be
  square and at least 512 px, and is scaled down. A package without one gets no
  icon.
- the **version information**: product name (`App.Title`, or `-name`), company
  (`App.Publisher`), description (`App.Description`) and the version.

### The declaration and the version

`App` carries more than the window: `Publisher`, `Description`, `Category`,
`Schemes`, `FileTypes` and `Services` describe the app to the packages (URL
schemes, document types, what the app asks the system for). They are read from
the source like `AppID` and `Title`, so they must be constants (a literal, a
constant, a concatenation; a slice or struct literal of them, or a variable that
holds one); anything else is an error that names the field. `Run` ignores them.

The version the program reports (`internal/appver`, which the update check
reads) is `-version` when given, else the module's version the go command
stamps from a version control tag. `-version` is written with the linker's
`-X` flag, merged into your `-ldflags`; macOS's `Info.plist` and the Windows
version information carry the same value.

## tela package

```bash
tela package ./cmd/notes                                  # this machine's GOOS and GOARCH, its default formats
tela package -os linux -arch amd64,arm64 ./cmd/notes      # dist/notes-1.2.0-linux-amd64.AppImage, .deb, ... for both
tela package -format flatpak -o out .                     # the flatpak manifest and its folder
tela package -sign-cmd "gpg --detach-sign --armor" .      # a .asc next to each package
tela package -os darwin -arch universal .                 # dist/notes-1.2.0-darwin-universal.zip
tela package -os windows -arch amd64,arm64 \
  -appinstaller-url https://example.com/notes ./cmd/notes  # zip, msix and .appinstaller for both
```

Builds the program of a package `main` as `tela build` does (the same flags:
`-id`, `-name`, `-version`, `-icon`, `-console`, `-tags`, `-ldflags`, `-gcflags`,
`-trimpath`, `-v`) for every `-os` with every `-arch` (default: the `GOOS` and
`GOARCH` of the environment), then writes its packages into `-o` (default
`./dist`), named `<name>-<version>-<os>-<arch>.<ext>` as
[`tela update manifest`](Updates) reads them. `<name>` is the last part of the
app ID, lowercase (`com.example.Notes` gives `notes`), also the installed
program's name.

| OS | `-format` | Writes |
|---|---|---|
| linux | `appimage` (default) | An AppImage on Tela's own runtime, which updates itself with package `update` |
| linux | `deb` (default) | A Debian package |
| linux | `flatpak` | A flatpak-builder manifest and the folder it builds from; the `.flatpak` bundle too when `flatpak-builder` and `flatpak` are installed and the target is this machine's architecture |
| windows | `zip` (default) | The program in a folder, which updates itself with package `update` |
| windows | `msix` (default) | An MSIX package, and its `.appinstaller` file with `-appinstaller-url` |
| darwin | `app` (default) | The bundle `<Name>.app` in a zip, which updates itself with package `update` |
| darwin | `dmg` | A disk image with the bundle and a link to `/Applications`, made by `hdiutil`: on a Mac only |

Everything comes from the package's `tela.App`, read from the source as
`tela build` reads it: `AppID` is required, `Title` is the name, and
`Publisher`, `Description`, `Category` (a freedesktop.org main category,
`Utility` by default), `Schemes`, `FileTypes` and `Services` fill the metadata.
They are checked before anything is built (the schemes and file types as
`tela.Run` checks them, the category, what each format needs), so a mistake
costs no build. The icon is `icon.svg` or `icon.png` (512 px or more) in the
package's folder, or `-icon`. The version is `-version`, else the module's
version as the go command stamps it, else `0.0.0`.

Each Linux package holds the program as `usr/bin/<name>` and the freedesktop.org
metadata, written once for all of them:

- `<AppID>.desktop`: `Name`, `Comment` (the description), `Exec` (with `%U` when
  the app opens files or URLs), `Icon` (the app ID), `Categories`,
  `StartupWMClass` (the app ID, which Tela sets as the Wayland app ID and the X11
  class) and `MimeType` (the file types the app opens and
  `x-scheme-handler/<scheme>` for its schemes);
- `<AppID>.metainfo.xml`, the AppStream metainfo: ID, name, summary, developer
  (`Publisher`), the desktop entry it launches, the program and media types it
  provides, and the release with its version and date (metadata license
  CC0-1.0);
- a shared-mime-info package for the file types, `mime/packages/<AppID>.xml`;
- the icon in the hicolor theme at 16 to 512 px, and the SVG as the scalable
  one.

**Same bytes.** The same inputs (the program's source, the go toolchain, tela's
version) give the same files: every time is `SOURCE_DATE_EPOCH` when it is set,
else the time of the commit the go command stamped into the program, else
1980-01-01; entries are sorted, owned by root, with fixed modes. The flatpak
bundle and the dmg are the exceptions: flatpak and `hdiutil` write them.

### AppImage

The AppImage project's runtime mounts the payload with FUSE and links libfuse3;
Tela writes its own runtime, in Go, without FUSE. `tela package` builds it from
the copy of its source in the `tela` command (so it is the runtime of the tela
that packages), for the target architecture, and writes the AppImage type 2
layout: the runtime with `AI\x02` at offset 8 and the sections `.upd_info`,
`.sha256_sig` and `.sig_key` that AppImage tools read (the signature is empty),
then a squashfs image (version 4.0, zlib) where the ELF file ends. The payload:

```text
AppRun -> usr/bin/<name>
<AppID>.desktop
<AppID>.png, .DirIcon -> <AppID>.png
usr/bin/<name>
usr/share/applications/<AppID>.desktop
usr/share/metainfo/<AppID>.metainfo.xml
usr/share/mime/packages/<AppID>.xml
usr/share/icons/hicolor/...
```

On the first start of a version the runtime checks the payload's SHA-256
(recorded in its section `.tela_payload`; a damaged or incomplete download is
refused with a message), extracts it into
`$XDG_CACHE_HOME/tela-appimage/<AppID>/<version>-<hash>/` (else
`~/.cache/...`), and `exec`s its `AppRun` with `APPIMAGE`, `APPDIR`, `ARGV0` and
`OWD` set and the arguments forwarded, so the app's process is the one you
started. Extraction holds a lock, so a second instance started at the same time
waits for it, and goes into a temporary folder renamed into place when complete;
what a crashed start left is removed by the next one. Each later start checks
the extracted files against a sealed list (types, modes, sizes; a file's content
only when its time changed) and extracts again when something was changed or
deleted, then removes the other versions of the app that no running instance
uses (each instance holds a shared lock on its folder). The options are the
usual ones: `--appimage-extract [pattern]` (into `./squashfs-root`),
`--appimage-offset`, `--appimage-updateinformation`, `--appimage-signature`,
`--appimage-version`, `--appimage-help`, `--appimage-extract-and-run`; there is
no `--appimage-mount`.

An arm64 AppImage is built from any machine. To run one under `qemu-user` on
another architecture, start it through `qemu-aarch64` itself: the AppImage magic
at offset 8 keeps it from matching a `binfmt_misc` entry, as with any AppImage.

### deb

An ar archive of `debian-binary`, `control.tar.gz` and `data.tar.gz`. The
control file has `Package` (the name), `Version` (the version made valid for
Debian: `1.2.0-rc.1` is `1.2.0~rc.1`, so it sorts before `1.2.0`),
`Architecture` (`amd64`, `arm64`, ...), `Maintainer` (`Publisher`: write it as
`Name <email>`; without one it is `unknown`, and tela says so), `Description`,
`Section` (from the category), `Priority` and `Installed-Size`; no `Depends`,
since the program is static. `postinst` and `postrm` run `update-mime-database`
and `update-desktop-database` when they are installed. apt manages the install,
so package `update` reports `ErrManaged` there.

### flatpak

`<name>-<version>-linux-<arch>-flatpak/` holds `<AppID>.json`, a flatpak-builder
manifest for `org.freedesktop.Platform` 25.08 (the Freedesktop SDK of August
2025, supported until August 2027) whose `simple` build installs the prebuilt
files of `files/`. The app ID needs three parts or more. The permissions
(`finish-args`):

| From | Permission |
|---|---|
| always | `--socket=wayland`, `--socket=fallback-x11`, `--share=ipc`, `--device=dri`; the tray: `--talk-name=org.kde.StatusNotifierWatcher` and `--own-name=org.kde.*` (Tela's tray item owns `org.kde.StatusNotifierItem-<pid>-<n>`) |
| `Services.Network` | `--share=network` |
| `Services.Notifications` | `--talk-name=org.freedesktop.Notifications` |
| `Services.Secrets` | `--talk-name=org.freedesktop.secrets` |
| `Services.Files` | `--filesystem=home` |
| `Services.Microphone` | `--socket=pulseaudio` |
| `Services.Bluetooth` | `--allow=bluetooth` |
| `Services.Camera`, `Location`, `GlobalShortcuts` | none: their portals ask the user |
| `Services.Contacts`, `Calendars`, `Photos` | none: flatpak has no permission for them (tela says so) |

The file chooser, settings, trash and open-URI portals need no permission, and
flatpak gives the accessibility bus by itself. With `flatpak-builder` and
`flatpak` installed, `tela package` also runs them to write
`<name>-<version>-linux-<arch>.flatpak` (for this machine's architecture);
otherwise it says how. flatpak manages the install, so package `update` reports
`ErrManaged` there.

### macOS

`<name>-<version>-darwin-<arch>.zip` holds the bundle at its top level, as
`ditto -c -k --keepParent` makes it, so Finder's Archive Utility, `ditto -x -k`
and package `update` unpack it with the program's executable bit and any
symbolic link (each entry records the Unix host and its mode):

```text
Notes.app/Contents/Info.plist
Notes.app/Contents/MacOS/Notes
Notes.app/Contents/Resources/notes.icns
Notes.app/Contents/_CodeSignature/CodeResources   # signed
```

`-arch` takes `amd64`, `arm64` or `universal`: both programs in one file (a fat
Mach-O written by tela, slices aligned to 16 KiB, each one the linker's file
byte for byte), named `...-darwin-universal.zip`; `tela update manifest` lists
it for `darwin/amd64` and `darwin/arm64`. Unsigned, nothing changes the program
after the linker: the arm64 program keeps the linker's ad-hoc signature, which
Apple silicon needs to run it (the amd64 one has none).

The `Info.plist`, from the declaration:

| From | Keys |
|---|---|
| the icon | `CFBundleIconFile` (`<name>.icns` in `Resources`, 16 to 1024 px) |
| the version | `CFBundleShortVersionString` and `CFBundleVersion`: its `X.Y.Z` (`1.2.0-rc.1` is `1.2.0`); the program carries all of it |
| `Category` | `LSApplicationCategoryType`: `AudioVideo` and `Video` are `video`, `Audio` `music`, `Development` `developer-tools`, `Education` and `Science` `education`, `Game` `games`, `Graphics` `graphics-design`, `Office` `productivity`, the rest (`Network`, `Settings`, `System`, `Utility`, none) `utilities`, each as `public.app-category.<x>` |
| `Publisher` | `NSHumanReadableCopyright`: `© <publisher>` (without an e-mail address) |
| `Services` | `NSCameraUsageDescription`, `NSMicrophoneUsageDescription`, `NSLocationWhenInUseUsageDescription`, `NSContactsUsageDescription`, `NSCalendarsUsageDescription` and `NSCalendarsFullAccessUsageDescription` (macOS 14), `NSPhotoLibraryUsageDescription`, `NSBluetoothAlwaysUsageDescription`: macOS shows the text when it asks, and ends an app that uses a service without one |
| `Schemes` | `CFBundleURLTypes`: `<AppID>.<scheme>`, role Viewer |
| `FileTypes` | `CFBundleDocumentTypes`, and `UTExportedTypeDeclarations` for the app's own types |

A file type is the app's own unless its extension has a public type of the
system (`.txt`, `.md`, `.json`, `.png`, `.pdf` and other common ones): the
bundle exports the type `<AppID>.<ext>` (conforming to `public.data` and
`public.content`, with the extension and MIME type as tags) and declares the
document type with rank `Owner`, so Launch Services opens those files with the
app. A public type is declared with rank `Alternate`: the app is offered, and
never takes the default from the app that owns the format. The role is Editor by
default, or Viewer or Shell; `HandlerNone` declares the type with role and rank
None. Because everything is declared when the bundle is built, the registration
of a running app (`desktop`) finds it declared and only asks Launch Services to
read the bundle: it never edits a signed bundle's `Info.plist`, which the
signature seals.

`<name>.entitlements`, next to the packages, holds the hardened runtime's
entitlements of `Services`, which `codesign` signs with: `Camera` is
`com.apple.security.device.camera`, `Microphone`
`com.apple.security.device.audio-input`, `Location`, `Contacts`, `Calendars` and
`Photos` `com.apple.security.personal-information.location`, `.addressbook`,
`.calendars` and `.photos-library`. There is no App Sandbox: a sandboxed app
reaches only the files the user picks through the system's own dialogs, cannot
register global shortcuts, and cannot have its schemes and file types
registered the way Tela does; a sandboxed variant is later work. The other
services need no entitlement outside the sandbox.
### Windows zip

`<name>-<version>-windows-<arch>.zip` holds one folder:

```text
<name>/<name>.exe          the program, as tela build makes it (GUI, icon, version information)
<name>/tela-install.json   the install's marker for package update
```

The marker is [`update.Marker`](https://pkg.go.dev/github.com/kuronosu/tela/update#Marker):
the format `zip`, the app ID, the version, the program and the entries the
package put in the folder. Unpacked anywhere the user can write, the folder is
an install that updates itself: package `update` finds the marker next to the
running program and replaces only the entries it lists, so files the user adds
to the folder stay.

### MSIX

`<name>-<version>-windows-<arch>.msix` is written in Go, as the MSIX SDK's
packer writes one: the program, the tiles `Assets/Square44x44Logo.png`,
`Square150x150Logo.png` and `StoreLogo.png` (drawn from the icon; a gray square
without one), `AppxManifest.xml`, `AppxBlockMap.xml` (the SHA-256 of every
64 KiB block of every file and of the manifest, each block deflated on its own)
and `[Content_Types].xml`, in a zip with no folders or data descriptors and the
zip64 end records. The manifest comes from the declaration:

| Element | From |
|---|---|
| `Identity` | `Name` the app ID (3 to 50 characters), `Version` the version as four numbers (`1.2.3` is `1.2.3.0`; a pre-release or build metadata has no place in it, which tela says, and Windows updates only to a higher version), `ProcessorArchitecture` `x64`, `arm64` or `x86`, `Publisher` (below) |
| `Properties` | `DisplayName` the name, `PublisherDisplayName` `App.Publisher` without an e-mail in angle brackets (else the name), `Logo` the store logo |
| `Dependencies` | `Windows.Desktop` from 10.0.17763.0 (Windows 10 1809, the first with toasts for desktop apps), tested on 10.0.26100.0 |
| `Application` | `App`, the program with `EntryPoint="Windows.FullTrustApplication"`, its tiles and description |
| `Extensions` | a `uap:Protocol` per scheme (lowercase), a `uap:FileTypeAssociation` per MIME type the app opens (types of role `HandlerNone` have none), the toast activator: a `com:ExeServer` of the program with `-ToastActivated` and the class `ToastActivatorCLSID(AppID)`, named in `desktop:ToastNotificationActivation` (ADR-297) |
| `Capabilities` | `runFullTrust` always, and from `App.Services`: |

| Service | Capability |
|---|---|
| `Network` | `internetClient` |
| `Contacts` | `uap:Capability contacts` |
| `Calendars` | `uap:Capability appointments` |
| `Photos` | `uap:Capability picturesLibrary` |
| `Files` | `rescap:Capability broadFileSystemAccess` |
| `Camera`, `Microphone`, `Location`, `Bluetooth` | `DeviceCapability` `webcam`, `microphone`, `location`, `bluetooth` |
| `Notifications`, `Secrets`, `GlobalShortcuts` | none: desktop programs need none |

Windows starts the program with the URL (for a scheme) or the file's path (for a
file type) as its argument, as it does for an install that registered them.

**The publisher.** An MSIX's `Publisher` is a distinguished name that must be
the subject of the certificate that signs it: give it with
`-msix-publisher "CN=Example Ltd, O=Example Ltd, C=US"`. Without the flag it is
`CN=<App.Publisher>` (quoted when it holds `,+="<>#;`), and when `-sign` does
not sign the package with signtool, tela adds Windows 11's mark of unsigned
packages, `OID.2.25.311729368913984317654407730594956997722=1`, so it installs
without a signature:

```powershell
Add-AppxPackage -AllowUnsigned .\notes-1.2.0-windows-amd64.msix
```

Such a package cannot be signed afterwards (its publisher is no certificate's
subject): package again with `-msix-publisher` to sign.

**Updates.** With `-appinstaller-url <URL>`, the URL of the folder where the
files are published, tela also writes `<name>-windows-<arch>.appinstaller`, the
same name for every version: its `Uri` is `<URL>/<name>-windows-<arch>.appinstaller`
and its package's `<URL>/<name>-<version>-windows-<arch>.msix`. People install
from it (App Installer opens it from a link or a file), and Windows checks it
for a higher version at each start of the app (`OnLaunch` with
`HoursBetweenUpdateChecks="0"`, without a prompt: the update found at one start
is downloaded then, only the blocks that changed, and installed at the next
start) and every 8 hours (`AutomaticBackgroundTask`). Publish
the new `.msix` and the new `.appinstaller` over the old one for each release.
App Installer installs only signed packages, and the server must send the files
with their types (`application/msix`, `application/appinstaller`). package
`update` reports `ErrManaged` in an MSIX install: the `.appinstaller` updates
it. Without the flag, tela says that no `.appinstaller` was written.

### Signing

Linux packages have no signing tool to run: `-sign` does nothing there and says
so. `-sign-cmd command` runs a command on each file to sign, every package file
(and, for Windows, each program before it is packaged; for macOS, the program
and the bundle before the zip is made): an argument `{}` is replaced by the
file's path, else the path is added at the end. The output says `(signed)` or
`(unsigned)` after each file; unsigned packages are written all the same.

On a Mac, `-sign` runs Apple's tools for the darwin packages:

```bash
xcrun notarytool store-credentials notes --apple-id you@example.com --team-id TEAMID
tela package -os darwin -arch universal -format app,dmg -sign \
    -codesign-identity "Developer ID Application: Example Ltd (TEAMID)" \
    -notary-profile notes .
```

`codesign --force --options runtime --timestamp --entitlements <name>.entitlements
-s <identity>` signs the program, then the bundle (then `codesign --verify
--deep --strict` checks it); a dmg is signed as a whole. With `-notary-profile`
(the keychain profile `notarytool store-credentials` saved), `xcrun notarytool
submit --wait` sends the bundle (in a zip) or the dmg to Apple's notary service,
and `xcrun stapler staple` attaches the ticket once it is accepted (a rejection
says how to read the log); the app zip is made after the stapling. Without
`-notary-profile` the packages are signed but not notarized, and Gatekeeper
refuses them once downloaded. `-codesign-identity -` signs ad hoc, for a local
check of the hardened runtime. `security find-identity -v -p codesigning` lists
your identities.

On Linux or Windows there is no `codesign`: `-sign` says so and writes the
darwin packages unsigned. [rcodesign](https://github.com/indygreg/apple-platform-rs)
(apple-codesign) signs and notarizes on any system; run it through `-sign-cmd`,
for example `-sign-cmd "rcodesign sign {}"`, with its own options for the
identity. A Mac checks a package with:

```bash
ditto -x -k notes-1.2.0-darwin-universal.zip /tmp/n   # or open it in Finder
codesign --verify --deep --strict --verbose=2 "/tmp/n/Notes.app"
spctl --assess --type execute --verbose=2 "/tmp/n/Notes.app"   # Gatekeeper: "Notarized Developer ID"
xcrun stapler validate "/tmp/n/Notes.app"
```

For Windows targets on a Windows machine, `-sign` runs `signtool` (from
`PATH`, else the newest Windows SDK) on each program, after its icon and version
information are added and before it goes into the zip and the MSIX, and on each
MSIX once it is written:

```text
signtool sign /fd SHA256 /tr <-timestamp-url> /td SHA256 <certificate> <file>
```

| Flag | Certificate |
|---|---|
| `-cert-file f.pfx` | `/f f.pfx`, with `-cert-password-file f` read into `/p` (signtool takes the password on its command line) |
| `-cert-sha1 hash` | `/sha1 hash`: the certificate of that thumbprint in the store |
| neither | `/a`: the best certificate signtool finds in the store |
| `-timestamp-url URL` | the RFC 3161 time-stamp server, by default `http://timestamp.digicert.com` |

The zip itself is not signed by signtool; the output says that its program is.

To try the signed path without a bought certificate, make a self-signed one and
trust it for package installs (the `TrustedPeople` store of the machine, from an
elevated PowerShell), then give its subject as the publisher:

```powershell
$c = New-SelfSignedCertificate -Type CodeSigningCert -Subject "CN=Notes Test" `
    -CertStoreLocation Cert:\CurrentUser\My -NotBefore (Get-Date).AddDays(-1)
Export-PfxCertificate -Cert $c -FilePath test.pfx -Password (Read-Host -AsSecureString)
Export-Certificate -Cert $c -FilePath test.cer
Import-Certificate -FilePath test.cer -CertStoreLocation Cert:\LocalMachine\TrustedPeople
tela package -os windows -sign -cert-file test.pfx -cert-password-file pw.txt `
    -msix-publisher "CN=Notes Test" .
```

`-NotBefore` a day back keeps the certificate valid at the time-stamp server's
time when the machine's clock runs ahead of it: otherwise `signtool verify /pa`
says 0x800B0101 ("not within its validity period").
On Linux and macOS signtool is not available, which tela says: sign with
`-sign-cmd` and a signer that runs there, such as
[jsign](https://ebourg.github.io/jsign/), which signs executables and MSIX
packages in place.

## tela icons gen

```text
tela icons gen -pkg <name> [-o outdir] [-r] [-prefix text] [-suffix text] [-skip-invalid] [-minify=false] [-source text] [-license name] [-license-file file] [-deprecated file] <dir>
```

Reads the `*.svg` files of `<dir>` and writes `outdir/icons_gen.go` with one
exported `tela.Icon` variable per file. It validates and minifies every file
and does not need the `go` command or a module. The flags, the naming rules and
what the minifier removes are on the [Icons](Icons) page; `tela help icons gen`
prints them.

```bash
tela icons gen -pkg icons -o ./icons -license-file LICENSE -license ISC ./svg
```

## tela update

```text
tela update keygen [-o file]
tela update manifest [-m manifest.json] -version V [flags] <file>...
tela update sign -key file [-key file...] [-o file] <manifest>
```

Make the ed25519 key that signs an app's updates, add a release to its update
manifest from the packaged files (sizes, SHA-256 sums, targets read from the
file names; the whole output folder of `tela package` may be given, and what
is not a package of the release is skipped) and sign the manifest for package
`update`. The whole flow, with `examples/notes` shipped from start to update,
the manifest's format and its flags are on the [Updates](Updates) page.

## Debug overlays

For a running app, `TELA_DEBUG` draws diagnostics on top of the window with
Tela's own renderer, at no cost when off:

```bash
TELA_DEBUG=layout,paint-flash CGO_ENABLED=0 go run ./examples/dashboard
```

| Flag | Shows |
|---|---|
| `layout` | The box model: margin, border and padding colored |
| `paint-flash` | Flashes the regions that were repainted (fading over 300 ms) |
| `layers` | Layer borders |
| `fps` | Frame timing |
| `semantics` | Semantics bounds and labels |

`all` turns on every flag; an unknown name prints one line with the valid ones.
In tests the environment variable is ignored, so goldens do not depend on it:
pass [`telatest.Debug`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Debug)("layout") to [`telatest.Mount`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Mount) (or to [`telatest.Render`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Render), whose
image then includes the overlays), and use [`App.Advance`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Advance) to look at the
fading ones. The CLI does not draw overlays.

## Environment variables

| Variable | Effect |
|---|---|
| `TELA_DEV=1`, `TELA_DEV=0` | Turns dev mode on or off, whatever the build and `tela.App.Dev` say (by default it is on unless the binary is a release build: built with `-trimpath` or installed with `go install pkg@version`). Dev mode reports unknown classes, hook misuse and other mistakes with the fix |
| `TELA_DEBUG=...` | Debug overlays (above) |
| `TELA_UPDATE=1` | `go test` writes goldens instead of comparing them; review the diff |
| `TELA_BACKEND=...` | Chooses the window backend: `wayland`, `x11` (Linux), `win32` (Windows) or `cocoa` (macOS), or `headless`. On Linux, Wayland is tried first by default, then X11 |
| `TELA_CHECK_DAMAGE=1` | In `telatest`, compares every frame with a full rendering (slow) |

## For agents

The loop is *write, `tela snapshot`, look at the PNG, `tela inspect` when
something is off, fix*, or the same loop through the tools of
[`tela mcp`](#tela-mcp), on a screen that stays up between edits. Unknown classes arrive with suggestions, `inspect
--layout` shows why a box is where it is, and `inspect` shows what a screen
reader would announce. The same data is available from Go with
[`telatest.Render`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Render), [`telatest.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#AriaSnapshot) and [`telatest.LayoutJSON`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#LayoutJSON); see
[Testing](Testing#without-a-test).
