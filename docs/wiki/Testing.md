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

func Counter() tela.Node { return tela.C(counter) }

func counter() tela.Node {
	count, setCount := tela.UseState(0)
	return Div(Class("flex flex-col items-start gap-4 p-6"),
		H1(Class("text-3xl font-bold"), Textf("Count: %d", count)),
		Button(Class("rounded-lg bg-indigo-500 px-4 py-2 text-white"),
			OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }),
			Text("Increment")),
	)
}

func TestCounter(t *testing.T) {
	app := telatest.Mount(t, Counter(), telatest.Size(400, 300))

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
| [`telatest.NoDev`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#NoDev) | Turns dev mode off, to test what a release build does. Apps run in dev mode by default: a dev error (an unknown class) **fails the test** at the action that triggered it, a dev panic (an unknown role, a hook outside its component) fails it like any panic, and a dev warning (a list without keys, nil dependencies...) is a test log line. `TELA_DEV=1` or `TELA_DEV=0` overrides the option. ([`telatest.Dev`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Dev) is kept and changes nothing) |
| [`telatest.AllowDev`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#AllowDev) | Substrings of dev errors that do not fail the test |
| [`telatest.Timeout`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Timeout) | How long waits last before failing (default 5 s of wall-clock time) |
| [`telatest.Debug`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Debug), [`telatest.DebugClock`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#DebugClock) | Debug overlays drawn into screenshots; see [CLI](CLI#debug-overlays) |
| [`telatest.CheckDamage`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#CheckDamage) | Compares every frame with a full rendering (for tests of rendering) |

[`telatest.MountFunc`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#MountFunc)(t, fn) is `Mount(t, tela.C(fn))`, for a test
that mounts a function (a closure written in the test, or the private
function of a component). To mount a component with a public wrapper, as
above, pass the wrapper's result: `Mount(t, Counter())`.

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
| [`telatest.ByTestID`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ByTestID)(id) | The element with that `h.TestID(id)`, like `data-testid`: for elements a user cannot name (a card, a chart). The id is never part of the aria snapshot or of what assistive technologies see |

Narrow a locator with [`Locator.Filter`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Locator.Filter) (with [`telatest.HasText`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#HasText) or
[`telatest.HasClass`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#HasClass)) and [`Locator.Nth`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Locator.Nth) (0 is the first, -1 the last).

An action needs a locator to match **exactly one** element (strictness, as in
Playwright). When it matches none or several, the test fails with the aria
snapshot and the closest names:

```go
func TestLocators(t *testing.T) {
	app := telatest.Mount(t, Counter())

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
| [`App.Click`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Click), [`App.DoubleClick`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.DoubleClick) | Press and release the primary button; `Click(l, telatest.Delay(d))` holds it for `d` of app time (a long press of the mouse); `Click(l, telatest.Right())` presses the secondary button instead, so `h.OnContextMenu` runs and a `kit.ContextMenu` opens (Playwright's `click({button: 'right'})`) |
| [`App.Tap`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Tap), [`App.LongPress`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.LongPress) | A finger on the element: `Tap` is a touch press and release (a click from a touch pointer); `LongPress` holds the touch past the long-press threshold (500 ms of app time), so `h.OnLongPress` runs and a context menu opens, as it does for touch and pen only |
| [`App.Hover`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Hover) | Moves the pointer onto the element so `hover:` and `group-hover:` apply |
| [`App.Focus`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Focus) | Focuses the element, like `focus()` |
| [`App.Scroll`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Scroll) | Scrolls the wheel over the element by `(dx, dy)` pixels, like a touchpad |
| [`App.Drag`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Drag), [`App.DragBy`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.DragBy) | Press, move in steps ([`telatest.Steps`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Steps)) and release: a real drag |
| [`App.ClickAt`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ClickAt)(x, y), [`App.DragFromTo`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.DragFromTo)(from, to) | The same without locators, at window coordinates in logical pixels: for a canvas, a chart or a slider track (find the box with `App.Bounds`); `ClickAt` takes the same options as `Click`, so `app.ClickAt(x, y, telatest.Right())` opens a context menu at a point |
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
few frames. This is decided in app time, so a slow machine never cuts a finite
animation short.

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
| [`App.Expect`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Expect)(l, states...) | The located element has all the states: [`telatest.Checked`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Checked), `Pressed`, `Selected`, `Expanded`, `Disabled`, `Focused`, `Invalid` (each takes a `bool`), [`telatest.Sort`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Sort)(order), [`telatest.Current`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Current)(token), [`telatest.HasName`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#HasName) and [`telatest.HasValue`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#HasValue); the widget properties the aria snapshot does not print, [`telatest.HasPopup`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#HasPopup)(kind), [`telatest.ActiveDescendant`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#ActiveDescendant)(loc) (or [`NoActiveDescendant`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#NoActiveDescendant)()) and [`telatest.Controls`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#Controls)(loc), where `loc` locates the one element the reference must point to. The failure shows the element's actual states and its aria line |
| [`App.ExpectAria`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.ExpectAria) | The aria snapshot equals the YAML (dedented, so a raw string may be indented) |
| [`App.WaitFor`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.WaitFor)(what, cond) | `cond()` reports true: for conditions locators cannot see |

Inspection helpers that do not wait: [`App.Count`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Count), [`App.Bounds`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Bounds)
(a `tela.Rect` in window coordinates, like `ElementRef.Bounds`),
[`App.PixelAt`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.PixelAt)(x, y) (the color of the frame at a logical point; at scale 2 it reads the device pixel `(2x, 2y)`),
[`App.AriaSnapshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.AriaSnapshot), [`App.LayoutJSON`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.LayoutJSON) (the layout tree of `tela inspect --layout`, for the app as it is now), [`App.Screenshot`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Screenshot), [`App.Cursor`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Cursor) (the
CSS name of the mouse cursor) and [`App.Stats`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Stats) (counters of builds,
skipped builds and layout and paint passes, to assert how narrow an update was).
[`App.Measure`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Measure)(f) runs `f`, settles, and returns the difference of the counters:
`st := app.Measure(func() { app.Click(inc) })`, then `st.Builds`.
[`App.Resize`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Resize)(w, h) and [`App.SetScale`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.SetScale)(s) change the window's
logical size or display scale and keep the state, as a user resizing the window or moving it to
another monitor: breakpoint variants and layout follow.

```go
func TestSignup(t *testing.T) {
	app := telatest.Mount(t, Signup(), telatest.Size(400, 300), telatest.Dev())

	email := telatest.ByRole("textbox", "Email")
	app.Fill(email, "ana@example.com")
	app.ExpectValue(email, "ana@example.com")

	app.Press("Enter") // implicit submission
	app.ExpectText("Welcome, ana@example.com")
}
```

```go
func Signup() tela.Node { return tela.C(signup) }

func signup() tela.Node {
	email, setEmail := tela.UseState("")
	welcome, setWelcome := tela.UseState("")
	return Form(OnSubmit(func(tela.SubmitEvent) { setWelcome.Set("Welcome, " + email) }),
		Label(For("email"), Text("Email")),
		Input(ID("email"), Bind(email, setEmail)),
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
	app := telatest.Mount(t, LongPress())

	app.Hover(telatest.ByRole("button", ""))
	app.MouseDown()
	app.Advance(600 * time.Millisecond) // the arena recognizes a long press at 500 ms
	app.MouseUp()

	app.ExpectText("Held")
}
```

```go
func LongPress() tela.Node { return tela.C(longPress) }

func longPress() tela.Node {
	held, setHeld := tela.UseState(false)
	return Button(OnLongPress(func(tela.PointerEvent) { setHeld.Set(true) }),
		IfElse(held, Text("Held"), Text("Press and hold")))
}
```

For a touch long press there is no need to hold by hand:
`app.LongPress(telatest.ByRole("button", "Press and hold"))` presses a finger,
moves the clock past the threshold and lifts it.

[`App.Now`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.Now) returns the clock.

### Capturing an animation

Actions settle the app, so a screenshot after `Click` shows the end of the
transitions it started. To look at the frames in between, mount with
[`telatest.NoSettle`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#NoSettle): the events of an action, `Screenshot` and `Golden`,
and the queries `AriaSnapshot`, `Count`, `Bounds`, `Stats` and `Cursor`,
then handle only the work due (state updates and the frames they ask for)
and leave running transitions and animations where they are; the clock moves
with `Advance`.

```go
func TestDialogEntrance(t *testing.T) {
	app := telatest.Mount(t, Dialog(), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Open"))
	app.Advance(100 * time.Millisecond)
	app.Golden("dialog_100ms.png") // 100 ms into the entrance
}
```

Actions still wait for their target and settle the app before acting, so the
animations an earlier step started end before the next action. Expectations
keep polling, but their first try handles only the work due and the clock moves
only between failed tries, a frame at a time. See [Animation](Animation) for
what animates.

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

[`App.GoldenScales`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#App.GoldenScales)(name, 1, 2) compares one golden per display scale,
`testdata/<name>_x1.png`, `<name>_x2.png`, re-rendering the app at each scale
and keeping its state.

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
	return telatest.Render(Counter(), telatest.Size(400, 300), telatest.Scale(2))
}
```

## Testing the app as it runs

[`telatest.MountApp`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#MountApp)(t, App) mounts `App.Root` in a window configured like
the app: its size, `RootClass`, `ColorScheme`, `Theme`, `ClassAliases`,
scrollbar and scroll settings and `Dev`. It is the same `App` that
`tela.Main(App)` runs and that `tela snapshot` reads from the package, so the
test, the PNG and the window agree; options after it override the app:

```go
var App = tela.App{Title: "Notes", Width: 900, Height: 600, RootClass: "bg-background", Root: Notes()}

func TestNotes(t *testing.T) {
	app := telatest.MountApp(t, App)
	app.ExpectText("Notes")
}
```

[`telatest.FromApp`](https://pkg.go.dev/github.com/kuronosu/tela/telatest#FromApp)(App) is the same as an option, for a test that mounts
something else (a struct component, a [`tela.Preview`](https://pkg.go.dev/github.com/kuronosu/tela#Preview)) in the app's window:
`telatest.Mount(t, Previews["mocked"], telatest.FromApp(App))`.

`MountApp` also applies the fields of the OS window: `Title`, the size
limits, `FixedSize`, `Frameless`, `AlwaysOnTop`, `Hidden`, `Icon`, `Placement`
and `OnClose` (`FromApp` does not, so tools that only render the screen see it
visible). The options `telatest.Title`, `MinSize`, `MaxSize`, `FixedSize`,
`Frameless`, `AlwaysOnTop`, `StartHidden`, `Icon`, `Placement`, `OnClose` and
`Screen` (the size of the fake screen that maximized windows fill) set them by
hand for `Mount`.

## Several windows

An app has one window per mounted `tela.Window`, besides its main window. The
headless backend plays a fake window manager: it grants every request at once,
gives hidden and minimized windows no frames, and records parents, modality,
hit regions and caption presses. `Mount` returns the `*telatest.App` of the
main window; `app.Windows()`, `app.Window(title)`, `app.WindowAt(i)`,
`app.LastWindow()` and `app.WaitForWindow(f)` (Playwright's
`waitForEvent('popup')`) return one scoped to another window, with the same
queries, actions, expectations, goldens and pixels, and always the same value
for a window. Selecting waits for the window to open; a click or a key in a
window that is not the active one activates it first.

```go
func TestSettingsWindow(t *testing.T) {
	app := telatest.MountApp(t, App)
	settings := app.WaitForWindow(func() {
		app.Click(telatest.ByRole("button", "Settings"))
	})
	settings.ExpectText("Preferences")
	settings.Golden("settings.png")
	if !app.Blocked() { // Settings is Modal: the main window takes no input
		t.Error("the main window is not blocked")
	}

	settings.RequestClose() // the close button; the window's OnClose decides
	settings.ExpectClosed()
	app.ExpectWindowCount(1)
}
```

What the user does with the window manager: `RequestClose`, `Minimize`,
`Maximize`, `Unmaximize`, `SetFullscreen`, `Restore`, `Move`, `Resize` and
`BringToFront`. The window itself: `Title`, `WindowState`, `WindowSize`,
`Placement`, `Modal`, `Parent`, `Blocks` and `BlockedBy`, with the
expectations `ExpectTitle`, `ExpectWindowState(telatest.Maximized(true),
telatest.Active(false))`, `ExpectClosed`, `ExpectWindowCount` and
`ExpectNoWindow`.

For a custom title bar (`h.WindowDrag`, `h.WindowControl`), `HitRegions` and
`HitAt` read what the window sent to the OS; a click on the drag area is the
OS's press (`CaptionDrags`, and a double click maximizes), and the control
buttons act on click:

```go
app.ClickAt(20, 18)                 // a press on the drag area starts a move
app.DoubleClick(telatest.ByTestID("titlebar"))
app.ExpectWindowState(telatest.Maximized(true))
app.Click(telatest.ByRole("button", "Close"))
app.ExpectQuit()
```

`ExpectQuit` waits for `tela.Quit`, or for the main window to be closed when
the app has no `OnClose`. The app is then over, as when `tela.Main` returns:
every window is unmounted, the cleanups of the effects have run, and using the
app fails the test; `HasQuit` and `ExpectRunning` check that it quit or did not
(an `OnClose` that hides the window keeps it running).

## Desktop services

The headless backend is also a fake desktop for package `desktop` (see
[Windows and Desktop](Windows-and-Desktop)). It shows nothing: it records the
file dialogs, message boxes, notifications and tray icons the app asks for, and
the test answers and clicks them as the user would. **Nothing answers by
itself**: a dialog stays open until the test answers it, and the callback of the
app runs then, on the UI thread, as in a real run.

```go
func TestOpenAndClose(t *testing.T) {
	telatest.TempDesktopDirs(t) // settings and window placements go to a temporary folder
	app := telatest.MountApp(t, App)

	app.Click(telatest.ByRole("button", "Open…"))
	app.AnswerFileDialog("/home/ana/ideas.md") // no paths: the user cancels
	app.ExpectText("ideas.md")

	app.Click(telatest.ByRole("button", "Close"))
	app.AnswerMessage(desktop.AnswerYes) // "Save the changes?" — Yes
	app.ExpectWindowState(telatest.Hidden(true))

	app.ClickTrayItem("Show Notes") // a submenu: ClickTrayItem("Theme", "Dark")
	app.ExpectWindowState(telatest.Hidden(false))
}
```

| Call | Does |
|---|---|
| `telatest.TempDesktopDirs(t)` | Points `ConfigDir`, `DataDir`, the settings store and saved window placements at a new temporary directory for the test (through `TELA_DESKTOP_DIR`), and flushes the stores before it is removed. Call it before the app reads a setting; like `t.Setenv`, not in parallel tests |
| `app.FileDialogs()` | The open dialogs: `Kind`, `Title`, `Directory`, `DefaultName`, `Filters` |
| `app.AnswerFileDialog(paths...)` | Answers the oldest one; no paths cancels. Fails when none is open |
| `app.MessageBoxes()` | The open message boxes: `Kind`, `Title`, `Text`, `Buttons`, and `Drawn` |
| `app.AnswerMessage(desktop.AnswerNo)` | Answers the oldest one. A box drawn by Tela gets a real click on its button; fails when the box has no such button |
| `app.Notifications()` | Every notification shown, oldest first, closed ones included: `Title`, `Body`, `Actions` (labels), `Closed` |
| `app.ClickNotification(title)`, `ClickNotificationAction(title, label)`, `DismissNotification(title)` | The user clicks it, one of its buttons, or closes it (its `OnClose` runs) |
| `app.Tray()`, `app.Trays()` | The icon: `Tooltip`, `Menu` (`TrayItem`s) and how many times it changed |
| `app.ClickTray()`, `app.ClickTrayItem(path...)` | A click on the icon; a choice in its menu, named by label (labels of the submenus first). Fail unless the app shows exactly one icon |
| `app.Unsupported(telatest.FileDialogs, ...)` | From now on the desktop lacks those services (`FileDialogs`, `MessageBoxes`, `Notifications`, `TrayIcons`, `OpenURLs`): the calls return errors wrapping `desktop.ErrUnsupported`, and a message box is drawn. Without arguments, everything is supported again |

`Unsupported` is how you test the fallbacks: `app.Unsupported(telatest.FileDialogs)`
and the app must show its path field instead.

## Testing a package of screens

Write one test per behavior, and use `telatest.Dev()` so mistakes the
framework would only report fail the test. The `examples/` directory has
complete suites: goldens at scale 1 and 2, aria snapshots, interaction tests
and, for `counter`, a comparison with Chromium.
