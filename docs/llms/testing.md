# Recipe: testing with telatest

Questions: how to mount a component and click, fill and assert; locators and expectations; goldens; the manual clock and animations; modifiers and platform keys; asserting that the app is idle; checking accessibility, contrast and cut-off text; several windows and desktop services (the recipes windows and desktop-integration).

`telatest` mounts a component in a headless window, drives it with real events, finds elements like Playwright and compares goldens (`testdata/<name>.png`; `TELA_UPDATE=1 go test ./...` writes them, then look at the PNG). Dev errors fail the test.

```go
func TestSettingsLike(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		on, setOn := tela.UseState(false)
		return Div(Class("p-6"),
			kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Label: "Weekly digest",
				Children: kit.Switch{Checked: &on, OnCheckedChange: setOn.Set}},
			If(on, P(Text("Subscribed"))))
	}, telatest.Size(480, 200), telatest.Theme(kit.Theme()))

	digest := telatest.ByRole("switch", "Weekly digest")
	app.Expect(digest, telatest.Checked(false))
	app.Click(digest) // also: Fill(loc, text), Press("Shift+Tab"), Press("ControlOrMeta+s") (Meta under telatest.PlatformKeys("darwin")), Hover, Click(loc, telatest.Right()), Tap, LongPress
	app.Expect(digest, telatest.Checked(true), telatest.Disabled(false))
	app.ExpectText("Subscribed")
	app.Advance(500 * time.Millisecond) // the clock is manual: timers, UseTimeout, animations
	app.Golden("digest_on.png")         // telatest.Dark(), telatest.Scale(2) for other renders
}
```

- **Mount:** `telatest.Mount(t, comp, opts...)` mounts any component (`Settings()`, `Page{}`, `Previews["x"]`); `MountFunc(t, func() tela.Node {...})` a function with hooks; `MountApp(t, App, opts...)` a `tela.App` as its window would (size, theme, scheme, window fields). Options: `Size(w, h)`, `Theme(t)`, `Dark()`, `Scale(n)`, `PlatformKeys(goos)` (one app, another platform's key conventions: `CmdOrCtrl`, text field bindings; so Mac behavior is tested on any OS), `Instrument()` (adds `[src=file:line]` to snapshots and violations), `NoSettle()`.
- **Locators:** `ByRole(role, name)`, `ByText(text)`, `ByTestID(id)` (`h.TestID`), `ByClass(...)`, then `.Nth(i)`, `.Filter(telatest.HasText("x"))`.
- **Expectations wait and retry** (auto-wait, from Playwright: pending frames, running animations and `UseAsync` runs settle first, with a timeout): `Expect(loc, Checked/Pressed/Selected/Expanded/Disabled/Focused/Invalid(bool), Current("page"), Sort, HasName, HasValue, HasRange(now, min, max))` (`HasRange` reads `aria-valuenow`, `-min` and `-max` of a slider, progress bar or meter; the snapshot prints only the value text), `ExpectText`, `ExpectNoText`, `ExpectRole`, `ExpectCount`, `ExpectValue`, `ExpectAria(yaml)`. `AriaSnapshot()` (the YAML of `tela inspect`), `Screenshot()`, `Bounds(loc)` (a `tela.Rect`), `Resize(w, h)`, `PixelAt`, `Measure` inspect without waiting.
- **Pointer options:** `app.Click(loc, telatest.Modifiers("Shift"))` (also `ControlOrMeta`, `Alt`, `Meta`; the names of Playwright's `click({modifiers})`) holds the keys for the pointer events of `Click`, `ClickAt`, `DoubleClick` and the drags, and sends no key events; `Right()` is the right button. `ClickAt(x, y)`, `DragFromTo`.
- **The clock is manual:** `app.Advance(d)` moves it; `NoSettle()` captures animation frames (actions, screenshots and queries then handle only the work due, and an expectation moves the clock between failed tries only for what is on it).
- **Idle (ADR-291):** `app.ExpectIdle()` fails, naming what is pending (`app.Pending()` returns `[]telatest.Pending`, each with a kind: a frame request, an animation, a `UseInterval` or `UseTimeout`, a caret blink, queued updates, a running `UseAsync`), unless the settled app would sleep; the clock does not move. Use it to assert that a screen costs nothing at rest (a leftover interval, a spinner that stays mounted).

```go
func TestRestsWhenDone(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		n, setN := tela.UseState(0)
		return Div(Button(OnClick(func() { setN.Update(func(n int) int { return n + 1 }) }), Text("Add")), Textf("n=%d", n))
	})
	app.Click(telatest.ByRole("button", "Add"))
	app.ExpectText("n=1")
	app.ExpectIdle() // fails, naming the pending work and when it is due, if the app would not sleep
}
```

**Accessibility, contrast and layout checks (ADR-306, ADR-312).** `app.ExpectAccessible(opts...)` fails the test with every violation of axe-like rules over the semantics tree and the frame, and `app.Audit(opts...)` returns them (`[]telatest.Violation`: `Rule`, `Impact`, `Message`, `Fix`, `Element`, `Source`, `Bounds`) without failing. The default rules need names and required attributes (`aria-command-name`, `aria-input-field-name`, `aria-required-attr`, `aria-valid-attr-value`, `focusable-name`...), the 24x24 target size (`target-size`, WCAG 2.5.8), contrast (`color-contrast`, 4.5:1, 3:1 for large text, on the colors the frame composites) and `click-events-have-key-events`; the rules that run when named are `text-clipped`, `text-overlap`, `target-overlap` and `focus-visible` (`WithTags("layout")` runs the first three). `telatest.Rules()` lists IDs, impacts and tags. Options: `WithRules(ids...)`, `WithTags("wcag2aa")`, `DisableRules(ids...)` (the way to accept one violation in one call), `Exclude(loc)` (a subtree); an unknown ID fails and names the closest. Call it in each state worth checking: an overlay open, a form with its errors, the empty list. `tela check` and `tela verify` run the same rules from the command line (recipe accessibility): run `tela check` before finishing UI work.

```go
func TestSavePanelAccessible(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("flex flex-col gap-4 p-6"),
			kit.Field{Label: "Email", Children: kit.Input{Type: "email"}},
			kit.Button{Children: Text("Save")})
	}, telatest.Size(480, 240), telatest.Theme(kit.Theme()), telatest.Instrument())
	app.ExpectAccessible()
	app.Click(telatest.ByRole("button", "Save"), telatest.Modifiers("Shift"))
	for _, v := range app.Audit(telatest.WithTags("layout")) { // cut-off or overlapping text
		t.Errorf("%s (%s): %s\n  fix: %s", v.Rule, v.Impact, v.Message, v.Fix)
	}
	app.ExpectAccessible(telatest.DisableRules("target-size"), telatest.Exclude(telatest.ByTestID("legacy")))
}
```

A test is not the only way to look: `tela mcp`'s `snapshot` and `screenshot` show the screen, and its `test` tool writes what you did in a session as a `telatest` test in the package.
