# CLI

The `tela` command renders a package's screen without a window, so you (or an
agent) can look at a UI in a terminal: write the UI, `tela snapshot` it, look
at the PNG, `tela inspect` it if something is off, fix it. It works in any
module that requires Tela; it needs the `go` command in `PATH`.

Install it once and use it from any module:

```bash
CGO_ENABLED=0 go install github.com/kuronosu/tela/cmd/tela@latest

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
| `tela snapshot` | Renders a screen to a PNG |
| `tela inspect` | Prints a screen's aria snapshot (YAML) or layout tree (JSON) |
| `tela dev` | Runs an app and reloads it on every save, keeping the window and the state |
| `tela icons gen` | Generates a Go package of `tela.Icon` variables from a directory of SVG files (see [Icons](Icons)) |
| `tela version` | Prints the version of the tela module the binary was built from, its VCS revision when known, and the Go version and platform |
| `tela help [command]` | Shows the help |

Exit codes: 0 on success, 1 on failure (the message says what and how to fix
it), 2 on a usage error. `tela lint`, `tela add` and `tela mcp`
are planned, not available yet.

## What a package must define

The commands run the package's own code. The package defines one of:

```go
package screens

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Preview is a single screen.
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

A `tela.Preview` is itself a component, so a test mounts one the way the CLI
does: `telatest.Mount(t, Previews["mocked"])`.

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
| `RootClass`, `ColorScheme`, `Theme`, `ClassAliases` | Applied, as in the window; the app's `ColorScheme` overrides `--dark`, as it overrides the system in a window |
| `ClassicScrollbars`, `InstantScroll`, `Dev` | Applied (`Dev` false silences the dev messages) |
| `Root` | The screen when the package has no `Preview` or `Previews`; otherwise ignored |
| `Title`, `AppID`, `DevState` | Ignored (`DevState` is for `tela dev`) |

[`telatest.MountApp`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#MountApp)(t, App) mounts the same `App` in a test, and
[`telatest.FromApp`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#FromApp)(App) gives the options of an `App` to
`Mount` and `Render`.

The screen's dev warnings (a list without keys, a `nil` dependency) and class
errors are printed on standard error, so a command's output on standard output
stays clean.

## tela snapshot

```text
tela snapshot [--size WxH] [--scale N] [--dark] [--preview NAME] [-o out.png] <pkg>
```

| Flag | Meaning |
|---|---|
| `--size WxH` | Logical size of the window (default: the package's `App.Width` and `App.Height`, else `960x640`) |
| `--scale N` | Display scale; fractional values such as `1.25` are allowed (default 1). The PNG is `W*N` by `H*N` pixels |
| `--dark` | Render in dark mode: `dark:` variants apply |
| `--preview NAME` | The named screen of a `Previews` variable |
| `-o file` | Output file (default `snapshot.png`) |

```bash
tela snapshot --size 480x200 --scale 2 --dark -o counter-dark.png ./examples/counter
```

Rendering uses Tela's own rasterizer and the embedded fonts, so the PNG is the
same on every machine and matches what a window shows. Snapshots of the
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
- heading "Settings" [level=1]
- switch "Email notifications" [checked]
- button "Save changes" [disabled]
```

`--layout` prints the layout tree as JSON, one object per box: its border box
in logical pixels (`rect`, from the top-left of the window), the resolved
margin, border and padding, the text and line count of paragraphs, the
element's classes, a summary of its computed style (display, size, flex
settings, color, background, font, opacity) and **the class errors** of the
element. A box painted elsewhere (a transform, scrolled content) also has
`bounds`, which is what `ElementRef.Bounds` returns. The order of the fields is
stable.

## tela dev

```text
tela dev [--size WxH] [--scale N] [--dark] [--preview NAME] [--no-state] [-v] [pkg]
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
Linux (Wayland, X11) and Windows; macOS comes with its backend.

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

## tela icons gen

```text
tela icons gen -pkg <name> [-o outdir] [-r] [-prefix text] [-suffix text] [-skip-invalid] [-minify=false] [-source text] [-license name] [-license-file file] <dir>
```

Reads the `*.svg` files of `<dir>` and writes `outdir/icons_gen.go` with one
exported `tela.Icon` variable per file. It validates and minifies every file
and does not need the `go` command or a module. The flags, the naming rules and
what the minifier removes are on the [Icons](Icons) page; `tela help icons gen`
prints them.

```bash
tela icons gen -pkg icons -o ./icons -license-file LICENSE -license ISC ./svg
```

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
| `TELA_BACKEND=...` | Chooses the window backend: `wayland`, `x11` (Linux) or `win32` (Windows), or `headless`. By default Wayland is tried first, then X11 |
| `TELA_CHECK_DAMAGE=1` | In `telatest`, compares every frame with a full rendering (slow) |

## For agents

The loop is *write, `tela snapshot`, look at the PNG, `tela inspect` when
something is off, fix*. Unknown classes arrive with suggestions, `inspect
--layout` shows why a box is where it is, and `inspect` shows what a screen
reader would announce. The same data is available from Go with
[`telatest.Render`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Render), [`telatest.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#AriaSnapshot) and [`telatest.LayoutJSON`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#LayoutJSON); see
[Testing](Testing#without-a-test).
