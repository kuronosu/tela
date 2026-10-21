# Getting Started

This page takes you from an empty directory to a window with a working
counter, then shows how to render the same screen without a window.

## Requirements

- Go 1.27 or newer.
- To open a window: a Wayland or X11 session on Linux, or Windows. macOS
  builds, but has no window backend yet.
- Nothing else. There is no C toolchain, no GTK, no browser: build with
  `CGO_ENABLED=0` and ship one static binary.

## Create the module

```bash
mkdir counter && cd counter
go mod init example.com/counter
go get github.com/kuronosu/tela@latest
```

## Write the app

A Tela program is a tree of nodes built by function components. The elements
(`Div`, `Button`, `Text`...) live in package `h`, which is meant to be
dot-imported so markup reads like JSX; hooks, events and `Run` live in package
`tela`.

```go
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Counter shows a count and a button that adds one.
func Counter() tela.Node {
	count, setCount := tela.UseState(0)

	return Div(Class("flex flex-col items-center gap-6 p-10 rounded-3xl "+
		"bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl"),
		H1(Class("text-5xl font-bold text-white tracking-tight"),
			Textf("Count: %d", count)),
		Button(Class("px-6 py-3 rounded-xl bg-white/15 text-white "+
			"hover:bg-white/25 active:scale-95"),
			OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
			Text("Increment"),
		),
	)
}

// Preview lets `tela snapshot` and `tela inspect` render the screen.
// Counter has state, so it is wrapped in tela.C, like App.Root.
func Preview() tela.Node { return tela.C(Counter) }

func main() {
	tela.Main(tela.App{
		Title:     "Counter",
		Width:     480,
		Height:    320,
		RootClass: "bg-slate-100",
		Root:      Counter,
	})
}
```

The pieces:

- [`tela.UseState`](https://pkg.go.dev/github.com/kuronosu/tela#UseState) returns the state and a [`tela.Setter`](https://pkg.go.dev/github.com/kuronosu/tela#Setter). `setCount.Set(v)`
  replaces the state and rebuilds the component in the next frame;
  `setCount.Update(func(n int) int { return n + 1 })` computes it from the
  previous state.
- [`Class`](https://pkg.go.dev/github.com/kuronosu/tela/h#Class) takes [Tailwind v4](https://tailwindcss.com/docs) classes. Variants
  such as `hover:` and `active:` restyle the button under the pointer.
- [`OnClick`](https://pkg.go.dev/github.com/kuronosu/tela/h#OnClick) receives a [`tela.MouseEvent`](https://pkg.go.dev/github.com/kuronosu/tela#MouseEvent) by value.
- [`tela.App`](https://pkg.go.dev/github.com/kuronosu/tela#App) describes the window and [`tela.Main`](https://pkg.go.dev/github.com/kuronosu/tela#Main) opens it and runs the event loop
  until it is closed. When no display is available it prints the error and
  exits with status 1 ([`tela.Run`](https://pkg.go.dev/github.com/kuronosu/tela#Run) returns the error instead). Both panic when
  `Root` is nil.
- `Preview` returns `tela.C(Counter)`, not `Counter()`: calling a component
  with hooks directly would run its hooks inside `Preview`, and dev mode
  stops you with the fix.

## Run it

```bash
CGO_ENABLED=0 go run .
```

![The counter rendered by Tela](https://raw.githubusercontent.com/kuronosu/tela/main/docs/images/counter.png)

## Render it without a window

Because the whole pipeline is headless, Tela can draw a screen to a PNG and
print what it contains as text. That is the loop an agent (or you, in a
terminal) uses to check a UI: write, snapshot, look, inspect, fix. The
`tela` command comes with the module:

```bash
go run github.com/kuronosu/tela/cmd/tela snapshot --size 480x320 -o counter.png .
go run github.com/kuronosu/tela/cmd/tela inspect --size 480x320 .
```

```yaml
- 'heading "Count: 0" [level=1]'
- button "Increment"
```

The commands render the package's `Preview` function (or a [`tela.Previews`](https://pkg.go.dev/github.com/kuronosu/tela#Previews)
variable with several named screens). See [CLI](CLI).

## Test it

[`telatest.Mount`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Mount) runs the same tree in a headless window and drives it with real
pointer and key events:

```go
package main

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

func TestIncrement(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(480, 320))

	app.Click(telatest.ByRole("button", "Increment"))

	app.ExpectText("Count: 1")
}
```

See [Testing](Testing) for locators, expectations, goldens and the manual
clock.

## Dev mode

While you develop, Tela reports the mistakes that compile, with the fix. Dev
mode is on in every development build (`go run .`, `go build` in a source
checkout) and in every `telatest` test, and off in release builds (built with
`-trimpath`, or installed with `go install pkg@version`).
[`App.Dev`](https://pkg.go.dev/github.com/kuronosu/tela#App) overrides that, and `TELA_DEV=1` or `TELA_DEV=0` overrides both.

- An unknown class is printed with the closest one (`unknown class
  "rounded-xxl"; did you mean "rounded-2xl"?`); in a test it fails the test.
- An unknown role, input type or easing, a change in the order of hooks, and
  a hook called outside its component (`Counter()` where `tela.C(Counter)`
  belongs) panic with the fix.
- Likely mistakes are warnings, printed once per call site (test log lines in
  `telatest`): a list without keys, `UseEffect(f, nil)` (use `UseMount`), props
  that a [`tela.CP`](https://pkg.go.dev/github.com/kuronosu/tela#CP) component cannot compare, a `Value.Get` that misses queued
  changes (use `Latest`), a context without a Provider whose default would
  panic, a `Label` that labels nothing.

`telatest.NoDev()` tests what a release build does.

## Next steps

- [Styling](Styling) to make it look good.
- [Hooks](Hooks) for state, effects and context.
- [Events](Events) and [Forms and Text Input](Forms-and-Text-Input) for
  interaction.
- The [`examples/`](https://github.com/kuronosu/tela/tree/main/examples)
  directory has complete screens (dashboard, signup form, kanban board, music
  player), each with its tests.
