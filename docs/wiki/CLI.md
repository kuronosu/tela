# CLI

The `tela` command renders a package's screen without a window, so you (or an
agent) can look at a UI in a terminal: write the UI, `tela snapshot` it, look
at the PNG, `tela inspect` it if something is off, fix it. It works in any
module that requires Tela; it needs the `go` command in `PATH`.

```bash
# From a clone of the repository
CGO_ENABLED=0 go run ./cmd/tela snapshot -o counter.png ./examples/counter

# From your own module (the tela module is already a dependency)
go run github.com/kuronosu/tela/cmd/tela inspect .

# Or install it
go install github.com/kuronosu/tela/cmd/tela@latest
```

| Command | Does |
|---|---|
| `tela snapshot` | Renders a screen to a PNG |
| `tela inspect` | Prints a screen's aria snapshot (YAML) or layout tree (JSON) |
| `tela version` | Prints the version of the tela module the binary was built from, its VCS revision when known, and the Go version and platform |
| `tela help [command]` | Shows the help |

Exit codes: 0 on success, 1 on failure (the message says what and how to fix
it), 2 on a usage error. `tela lint`, `tela dev`, `tela add` and `tela mcp`
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
func Preview() tela.Node { return tela.C(Login) }

// or Previews, several named screens.
var Previews = tela.Previews{
	"Login":    {Root: tela.C(Login)},
	"Settings": {Root: Settings{InitialTab: "account"}},
}

func Login() tela.Node {
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
`Root` is a component: a node such as `tela.C(Login)`, or a struct component
value, so it may use hooks. `--preview NAME` selects one; without it the CLI
renders the first name in byte order ([`Previews.Names`](https://pkg.go.dev/github.com/kuronosu/tela#Previews.Names)), so a name such
as `"1 Login"` can pick the default. When a package defines both, `Previews`
wins. `Preview()` may return a `tela.Node` or a `tela.Component`. What a
preview gives is mounted as is: a function component needs `tela.C`, as in
`App.Root` (`Root: tela.C(Notes)`, or `Root: Notes{}` for a struct component).

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
`"Login": {Root: tela.C(Login)}` now.

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
	Root:        tela.C(Notes),
}

func main() { tela.Main(App) }
```

| `App` field | In `tela snapshot` and `tela inspect` |
|---|---|
| `Width`, `Height` | The size of the window, unless `--size` is given (both must be set; otherwise 960x640) |
| `RootClass`, `ColorScheme`, `Theme`, `ClassAliases` | Applied, as in the window; the app's `ColorScheme` overrides `--dark`, as it overrides the system in a window |
| `ClassicScrollbars`, `InstantScroll`, `Dev` | Applied (`Dev` false silences the dev messages) |
| `Root` | The screen when the package has no `Preview` or `Previews`; otherwise ignored |
| `Title`, `AppID` | Ignored |

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
