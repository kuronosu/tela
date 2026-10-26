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
| `tela snapshot` | Renders a screen to a PNG |
| `tela inspect` | Prints a screen's aria snapshot (YAML) or layout tree (JSON) |
| `tela lint` | Checks the Go source for the mistakes that dev mode reports at run time: unknown classes, roles and aria tokens, duplicate or missing keys, misused hooks (see [tela lint](#tela-lint)) |
| `tela dev` | Runs an app and reloads it on every save, keeping the window and the state |
| `tela mcp` | Serves a package's screen to agents over the Model Context Protocol: snapshot, screenshot, click, fill... on an instance that stays up and rebuilds on edits (see [tela mcp](#tela-mcp)) |
| `tela icons gen` | Generates a Go package of `tela.Icon` variables from a directory of SVG files (see [Icons](Icons)) |
| `tela version` | Prints the version of the tela module the binary was built from, its VCS revision when known, and the Go version and platform |
| `tela help [command]` | Shows the help |

Exit codes: 0 on success, 1 on failure (the message says what and how to fix
it), 2 on a usage error. `tela lint` exits 1 when it reports something, and 2 when
a package cannot be loaded. `tela add` is planned, not available yet.

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

## tela new

```bash
tela new my-app
tela new -module github.com/you/my-app my-app
tela new .            # in an empty directory
```

Creates the directory (it may exist if it is empty) with a `go.mod`, a
`main.go` with a counter and a `var App`, a `main_test.go` that clicks its
button through `telatest`, a README and a `.gitignore`. Then it runs `go get`
and `go mod tidy`, so the app is ready:

```bash
cd my-app
tela dev                      # or: CGO_ENABLED=0 go run .
CGO_ENABLED=0 go test ./...
```

| Flag | Does |
|---|---|
| `-module path` | The module path (default: the directory name, lowercased) |
| `-skip-deps` | Do not run `go get` and `go mod tidy` (offline, or no `go` command); the command prints what to run |

The app requires the `tela` module at the version of the `tela` binary when
that is a release (`go install ...@v0.9.1` gives apps on v0.9.1), and at the
latest release when the binary was built from a checkout. A directory that is
not empty is refused and left untouched.

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

## tela lint

```text
tela lint [-json] [-tests] [packages]
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
| `class` | The classes of `h.Class` and `h.Exit`; the `Class` and `...Class` fields of the `kit` components, the string fields of the `kit` recipes (`kit.Style` and the `...Style` structs, also when assigned: `s.Button.Root = "..."`), `tela.App.RootClass`, `telatest.RootClass`; the values of `ClassAliases`. Unknown, unsupported and invalid classes are reported at the class inside the string. The class aliases (`tela.App.ClassAliases`, `telatest.ClassAliases`) and the theme colors (map literals of `tela.SchemeColor`, such as `Theme.Colors` and `theme.Colors["brand"] = ...`) declared in the linted packages are known: lint `./...` when you use them, and write the color names as constants (a name built at run time cannot be known) |
| `role` | `h.Role` with a role that is not a WAI-ARIA 1.2 role |
| `aria` | `h.AriaCurrent`, `h.AriaSort`, `h.AriaHasPopup`, `h.AriaOrientation` and `h.AriaAutoComplete` with a value outside their tokens |
| `prop` | `h.Dir`, `h.Type` (against the types of the `h.Input` or `h.Button` it is given to) and the `Type` of `kit.Button` and `kit.Input`, and `tela.Ease`, with a value they do not take |
| `key` | Two siblings with the same constant `h.Key` (or `tela.WithKey`, or the key of `tela.C`, `tela.CP`, `tela.Memo`), and an `h.Map` or `h.MapIndex` whose function returns elements without a `Key`. Dev mode only warns about these, so their severity is `warning` |
| `hooks` | A hook (`Use...` of `tela`, `aria`, `kit`, `i18n`, or a custom hook of your code) called conditionally, in a loop, after an early return, from a function that is neither a component nor a custom hook, or in a function literal that is not a component; and a component (a function or `Build` method that uses hooks) called directly instead of through `tela.C`, `tela.CP` or an element. A function with the shape `func() tela.Node` or `func(P) tela.Node` is given the benefit of the doubt. A function is a custom hook when it is named `UseXxx` or `useXxx` and calls a hook |

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
above it, silences it. Several rules are separated by commas (`class,role`),
text after them is a reason, and a bare `//tela:ignore` covers every rule.

| Flag | Does |
|---|---|
| `-json` | Prints the findings as a JSON array of `{file, line, col, rule, severity, message, suggestion}` (`suggestion` only when the message proposes a replacement; `severity` is `error` or `warning`; `[]` when there is nothing) |
| `-tests` | Lints the `_test.go` files too (by default they are skipped: tests often use a wrong class on purpose) |

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

| Tool | Does |
|---|---|
| `snapshot` | The aria snapshot (the YAML of `tela inspect`), each element that can be acted on marked `[ref=eN]` |
| `screenshot` | A PNG of the window, or of one element (`ref`) with an 8 px margin |
| `click` | Clicks an element, or a point (`x`, `y`); `button: "right"`, `double: true` |
| `hover` | Moves the pointer over an element |
| `fill` | Replaces the text of a field; `submit: true` presses Enter after |
| `press` | Presses a key or combo in Playwright's syntax (`Enter`, `Shift+Tab`, `Control+a`), on the focused element or on one it focuses first |
| `scroll` | Scrolls the wheel over an element by `dx`, `dy` |
| `drag` | Drags `from` an element or point `to` another, or by `dx`, `dy` |
| `resize` | Changes the window's `width` and `height` or `scale`, keeping the state |
| `open` | Mounts another `preview`, `dark` mode, size or scale afresh; without arguments, lists the previews |
| `wait` | Advances the app's clock by `ms` (timers, animations), or waits for a `text` to appear or `textGone` to go |
| `inspect` | The layout tree of `tela inspect --layout`, whole or of one element |
| `lint` | `tela lint -json` on the package |

An element is named by the `ref` of the latest snapshot, or by `role` and
`name`, `text` or `testId` (`telatest`'s locators; `nth` picks one of several).
Every tool that acts answers with the new snapshot. A ref stays with its element
across actions and rebuilds while the element keeps its role and name (and,
until the next remount, its position when only the name changed, as a
counter's label does), so `screenshot {"ref": "e5"}` right after an edit shows
the same element rebuilt; a ref whose element is gone is an error that asks
for a new snapshot. A failed action (an element
not found, a disabled button, a panic in a handler) is an error result of that
call: the instance keeps running, and after a panic it mounts the screen afresh.

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
something is off, fix*, or the same loop through the tools of
[`tela mcp`](#tela-mcp), on a screen that stays up between edits. Unknown classes arrive with suggestions, `inspect
--layout` shows why a box is where it is, and `inspect` shows what a screen
reader would announce. The same data is available from Go with
[`telatest.Render`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Render), [`telatest.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#AriaSnapshot) and [`telatest.LayoutJSON`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#LayoutJSON); see
[Testing](Testing#without-a-test).
