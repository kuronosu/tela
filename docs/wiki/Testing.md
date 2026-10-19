# Testing

`telatest` tests Tela UIs **without a window**. It mounts a tree in a headless
window (Tela's real pipeline, with a window that is a record in memory), drives
it with real pointer and key events, finds elements by role and text like
Playwright, waits automatically, and compares pixels with goldens. Nothing
needs a display or a browser, and the pixels are the same on every platform.

```go
package counter_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func Counter() tela.Node {
	count, setCount := tela.UseState(0)
	return Div(Class("flex flex-col items-start gap-4 p-6"),
		H1(Class("text-3xl font-bold"), Textf("Count: %d", count)),
		Button(Class("rounded-lg bg-indigo-500 px-4 py-2 text-white"),
			OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
			Text("Increment")),
	)
}

func TestCounter(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))

	app.Click(telatest.ByRole("button", "Increment"))

	app.ExpectText("Count: 1")
	app.ExpectAria(`
		- 'heading "Count: 1" [level=1]'
		- button "Increment"
	`)
	app.Golden("counter.png") // testdata/counter.png; TELA_UPDATE=1 writes it
}
```

## Mounting

[`telatest.Mount`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Mount) shows a node in a headless window and returns an [`telatest.App`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App).
The window starts focused, and the app is unmounted when the test ends,
running the cleanups of its effects. Options configure it:

| Option | Does |
|---|---|
| [`telatest.Size`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Size)(w, h) | Logical window size (default 800x600) |
| [`telatest.Scale`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Scale) | Display scale; goldens are `w*scale` by `h*scale` pixels |
| [`telatest.Dark`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Dark) | The system prefers dark: `dark:` classes apply |
| [`telatest.ColorScheme`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ColorScheme)(s) | The app's color scheme, like `tela.App.ColorScheme` (`tela.Light`, `tela.Dark`, `tela.System`) |
| [`telatest.ReducedMotion`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ReducedMotion) | The system asks for reduced motion |
| [`telatest.ClassicScrollbars`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ClassicScrollbars), [`telatest.InstantScroll`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#InstantScroll), [`telatest.RootClass`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#RootClass) | The `tela.App` fields of the same name |
| [`telatest.Dev`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Dev) | Dev mode: every dev message (unknown class, missing keys, hook order) **fails the test** at the action that triggered it |
| [`telatest.AllowDev`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#AllowDev) | Substrings of dev messages that do not fail a `Dev` test |
| [`telatest.Timeout`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Timeout) | How long waits last before failing (default 5 s of wall-clock time) |
| [`telatest.Debug`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Debug), [`telatest.DebugClock`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#DebugClock) | Debug overlays drawn into screenshots; see [CLI](CLI#debug-overlays) |
| [`telatest.CheckDamage`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#CheckDamage) | Compares every frame with a full rendering (for tests of rendering) |

An `App` is used from the goroutine of its test. State setters and `tela.Post`
may be called from any goroutine; do not mount apps that use `Post` from
parallel tests.

## Locators

A [`telatest.Locator`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Locator) describes an element by what a user or an assistive
technology sees, resolved against the semantics tree **each time** it is used,
so it keeps working when the app rebuilds. Hidden elements are never matched.

| Constructor | Matches |
|---|---|
| [`telatest.ByRole`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ByRole)(role, name) | The role (explicit with `h.Role` or implicit) and, if `name` is not empty, exactly that accessible name |
| [`telatest.ByText`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ByText)(text) | The deepest elements whose text contains `text`, case-insensitively; add [`telatest.Exact`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Exact) for the whole text |
| [`telatest.ByClass`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ByClass)(classes...) | Elements with all those Tailwind classes. An escape hatch that ties the test to the styling |

Narrow a locator with [`Locator.Filter`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Locator.Filter) (with [`telatest.HasText`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#HasText) or
[`telatest.HasClass`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#HasClass)) and [`Locator.Nth`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Locator.Nth) (0 is the first, -1 the last).

An action needs a locator to match **exactly one** element (strictness, as in
Playwright). When it matches none or several, the test fails with the aria
snapshot and the closest names:

```go
func TestLocators(t *testing.T) {
	app := telatest.Mount(t, tela.C(Counter))

	app.Click(telatest.ByRole("button", "Increment"))
	app.Click(telatest.ByText("increment"))                   // case-insensitive
	app.Click(telatest.ByText("Increment", telatest.Exact())) // the whole text
	app.Click(telatest.ByRole("button", "").Nth(0))           // the only button

	app.ExpectCount(telatest.ByRole("button", ""), 1)
	app.ExpectText("Count: 4")
}
```

## Actions

Actions act like a user. The pointer ones move the pointer to the element's
center and send real pointer events through hit-testing and the gesture arena
(hover and `:active` states, focus on press, click counts, drags); the keyboard
ones send key events to the focused element.

| Action | Does |
|---|---|
| [`App.Click`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Click), [`App.DoubleClick`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.DoubleClick) | Press and release the primary button; `Click(l, telatest.Delay(d))` holds it for `d` of app time (a long press) |
| [`App.Hover`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Hover) | Moves the pointer onto the element so `hover:` and `group-hover:` apply |
| [`App.Focus`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Focus) | Focuses the element, like `focus()` |
| [`App.Scroll`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Scroll) | Scrolls the wheel over the element by `(dx, dy)` pixels, like a touchpad |
| [`App.Drag`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Drag), [`App.DragBy`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.DragBy) | Press, move in steps ([`telatest.Steps`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Steps)) and release: a real drag |
| [`App.MouseMove`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.MouseMove), [`App.MouseDown`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.MouseDown), [`App.MouseUp`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.MouseUp) | The pointer by hand |
| [`App.Press`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Press) | A key or combination, Playwright's syntax: `"Enter"`, `"Shift+Tab"`, `"Control+s"`, `"ArrowDown"`, `"a"` |
| [`App.Type`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Type) | Characters one key at a time (an upper case letter has no Shift, like Playwright's `keyboard.type`) |
| [`App.Fill`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Fill) | Replaces a text field's value like Playwright's `fill`, firing `OnBeforeInput` and `OnChange` once |

**Auto-wait.** Before acting, an action waits until the element is visible,
enabled, in view (it scrolls it into view) and not covered by another element.
Before and after, the app is *settled*: pending state updates are applied and
frames run until nothing is dirty and no animation or scroll fling is running.
An app that never goes quiet (an endless animation, a goroutine that sets state
all the time) is not waited for: it is logged once and actions then run only a
few frames.

## Expectations

Expectations retry, letting a frame of app time and a moment of real time pass
between tries, until they hold or the timeout expires. State set by a goroutine
arrives.

| Expectation | Passes when |
|---|---|
| [`App.ExpectText`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectText), [`App.ExpectNoText`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectNoText) | The window's text content contains (or no longer contains) the text |
| [`App.ExpectRole`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectRole)(role, name) | An element with that role and name is visible |
| [`App.ExpectCount`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectCount)(l, n) | The locator matches exactly `n` elements |
| [`App.ExpectFocused`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectFocused) | The located element has the keyboard focus |
| [`App.ExpectValue`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectValue) | The located text field holds the value |
| [`App.ExpectAria`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectAria) | The aria snapshot equals the YAML (dedented, so a raw string may be indented) |
| [`App.WaitFor`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.WaitFor)(what, cond) | `cond()` reports true: for conditions locators cannot see |

Inspection helpers that do not wait: [`App.Count`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Count), [`App.Bounds`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Bounds),
[`App.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.AriaSnapshot), [`App.Screenshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Screenshot), [`App.Cursor`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Cursor) (the
CSS name of the mouse cursor) and [`App.Stats`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Stats) (counters of builds,
skipped builds and layout and paint passes, to assert how narrow an update was).

```go
func TestSignup(t *testing.T) {
	app := telatest.Mount(t, tela.C(Signup), telatest.Size(400, 300), telatest.Dev())

	email := telatest.ByRole("textbox", "Email")
	app.Fill(email, "ana@example.com")
	app.ExpectValue(email, "ana@example.com")

	app.Press("Enter") // implicit submission
	app.ExpectText("Welcome, ana@example.com")
}
```

```go
func Signup() tela.Node {
	email, setEmail := tela.UseState("")
	welcome, setWelcome := tela.UseState("")
	return Form(OnSubmit(func(tela.SubmitEvent) { setWelcome.Set("Welcome, " + email) }),
		Label(For("email"), Text("Email")),
		Input(Id("email"), Bind(email, setEmail)),
		P(Text(welcome)),
	)
}
```

## The clock

The window's clock is **manual**. It starts at zero and moves only when the app
waits or the test calls [`App.Advance`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Advance), so animations, scroll flings and
timers are deterministic. Frames are paced like a 60 Hz display. A long-press
timer, or the fade of an overlay scrollbar, fires only when the clock reaches
it:

```go
func TestLongPress(t *testing.T) {
	app := telatest.Mount(t, tela.C(LongPress))

	app.Hover(telatest.ByRole("button", ""))
	app.MouseDown()
	app.Advance(600 * time.Millisecond) // the arena recognizes a long press at 500 ms
	app.MouseUp()

	app.ExpectText("Held")
}
```

```go
func LongPress() tela.Node {
	held, setHeld := tela.UseState(false)
	return Button(OnLongPress(func(tela.PointerEvent) { setHeld.Set(true) }),
		IfElse(held, Text("Held"), Text("Press and hold")))
}
```

[`App.Now`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Now) returns the clock.

### Capturing an animation

Actions settle the app, so a screenshot after `Click` shows the end of the
transitions it started. To look at the frames in between, mount with
[`telatest.NoSettle`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#NoSettle): the events of an action, and `Screenshot` and `Golden`,
then handle only the work due (state updates and the frames they ask for)
and leave running transitions and animations where they are; the clock moves
with `Advance`.

```go
func TestDialogEntrance(t *testing.T) {
	app := telatest.Mount(t, tela.C(Dialog), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Open"))
	app.Advance(100 * time.Millisecond)
	app.Golden("dialog_100ms.png") // 100 ms into the entrance
}
```

Actions still wait for their target and settle the app before acting, and
expectations, `AriaSnapshot`, `Count`, `Stats` and `Cursor` still settle it,
so the animations an earlier step started end before the next action. See
[Animation](Animation) for what animates.

## Goldens

[`App.Golden`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Golden) compares the window's pixels with `testdata/<name>` next
to the test, **bit by bit**: Tela renders the same pixels on every platform and
architecture, so only a deliberate change should alter a golden. Regenerate
them and review the PNG before committing:

```bash
TELA_UPDATE=1 CGO_ENABLED=0 go test ./...
```

On a mismatch, `<name>_actual.png` and `<name>_diff.png` are written next to
the golden. Opt in to fuzziness with [`telatest.Tolerance`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Tolerance)(maxDelta, maxPixels).
The picture is in physical pixels, so it depends on `Size` and `Scale`, and it
shows overlay scrollbars while they are visible: advance the clock to hide them
when that matters.

[`App.AriaGolden`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.AriaGolden) does the same for the aria snapshot, in a text file;
use it for snapshots too long to write in the test. For short ones prefer
`ExpectAria`.

## Without a test

[`telatest.Render`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Render), [`telatest.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#AriaSnapshot) and [`telatest.LayoutJSON`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#LayoutJSON) mount a tree,
run frames until it is quiet, and return the image, the aria snapshot or the
layout JSON. They need no `*testing.T`; `tela snapshot` and `tela inspect` are
built on them, and so can your own tools be.

```go
func Thumbnail() (*image.RGBA, error) {
	return telatest.Render(tela.C(Counter), telatest.Size(400, 300), telatest.Scale(2))
}
```

## Testing a package of screens

Write one test per behavior, and use `telatest.Dev()` so mistakes the
framework would only report fail the test. The `examples/` directory has
complete suites: goldens at scale 1 and 2, aria snapshots, interaction tests
and, for `counter`, a comparison with Chromium.
