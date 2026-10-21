// Package telatest tests Tela UIs without a window: it mounts a tree in a
// headless window, simulates a user with real pointer and key events, finds
// elements by role and text like Playwright, and compares the pixels with
// goldens (ARCHITECTURE section 17).
//
//	func TestCounter(t *testing.T) {
//	    app := telatest.Mount(t, tela.C(Counter), telatest.Size(400, 300))
//	    app.Click(telatest.ByRole("button", "Increment"))
//	    app.ExpectText("Count: 1")
//	    app.Golden("counter.png") // testdata/counter.png; TELA_UPDATE=1 regenerates
//	}
//
// # Locators
//
// [ByRole], [ByText] and [ByClass] describe an element by what a user (or
// an assistive technology) sees, resolved against the semantics tree each
// time they are used; [ByTestID] finds the element that carries an
// h.TestID, for those a user cannot name (like data-testid, it is not in
// the aria snapshot). A locator that matches none or several elements makes
// an action fail with the aria snapshot and the closest names; narrow it
// with [Locator.Filter] and [Locator.Nth]. Hidden elements are never
// matched.
//
// # Actions and auto-wait
//
// [App.Click], [App.DoubleClick], [App.Hover], [App.Focus], [App.Scroll],
// [App.Drag], [App.DragBy], [App.Press], [App.Type] and [App.Fill] act like
// a user. The pointer actions move the pointer to the element's center and
// send pointer events through hit-testing and the gesture arena (hover and
// :active states, focus on press, click counts, drags). The keyboard
// actions send key events to the focused element (typing into a focused
// text field; Fill replaces a field's value like Playwright's fill).
//
// Before acting, an action waits until the element is visible, enabled, in
// view and not covered by another element. Before and after, the app is
// settled: pending state updates are applied and frames run, advancing the
// app's clock to each one, until nothing is dirty and no animation or
// scroll fling is running: transitions (hover:, focus:, dark: changes with
// transition classes) and finite animations run to their end, so a
// screenshot after an action shows the settled state. Infinite animations
// (animate-spin, animate-pulse) are never waited for, and use [App.Advance]
// to step them. An app that never goes quiet otherwise (a goroutine that
// updates state all the time) is not waited for either: the first time it
// is logged, and actions then run only a few frames.
//
// [App.Advance] moves the clock by hand: it is how a test waits for a
// long-press timer, an animation or a fade. To capture the frames of an
// animation, mount with [NoSettle]: actions, screenshots and the queries
// ([App.AriaSnapshot], [App.Count], [App.Bounds], [App.Stats],
// [App.Cursor]) then leave running animations where they are, and only
// Advance moves them (ADR-097).
//
// The expectations ([App.ExpectText], [App.ExpectNoText], [App.ExpectRole],
// [App.ExpectCount], [App.ExpectFocused], [App.ExpectValue],
// [App.ExpectAria] and [App.Expect]) retry until they hold or the [Timeout]
// expires, letting a frame of app time and a moment of real time pass
// between tries, so state set by a goroutine arrives (under [NoSettle] the
// first try does not move the clock: it handles only the work due).
// [App.Expect] checks
// the states of an element ([Checked], [Disabled], [Expanded], [Selected],
// [Pressed], [Focused], [HasName], [HasValue]) without matching strings of
// the aria snapshot.
//
// [App.ClickAt] and [App.DragFromTo] act at window coordinates, for what a
// locator cannot name; [App.Bounds] returns an element's box as a tela.Rect.
// [App.PixelAt] reads the frame, [App.Measure] returns the work the tree
// did during some actions, and [MountFunc] is Mount for one component.
// [MountApp] mounts a tela.App's Root in the app's own window (its size,
// root class, color scheme, theme and class aliases), the App that
// tela.Main runs and `tela snapshot` reads from the package; [FromApp]
// gives those settings to Mount, Render and the other entry points.
//
// # The clock
//
// The window's clock is manual: it starts at zero and moves only when the
// app waits or the test advances it, so animations, scroll flings and timers
// are deterministic. Frames are paced like a 60 Hz display.
//
// # Goldens and snapshots
//
// [App.Golden] compares the window's pixels with testdata/<name>, bit by bit
// unless a [Tolerance] is given (Tela renders the same pixels on every
// platform, ADR-027); [App.GoldenScales] makes one golden per display
// scale. TELA_UPDATE=1 go test regenerates goldens (ADR-022).
// [App.AriaSnapshot] returns the YAML that `tela inspect` prints, and
// [App.ExpectAria] and [App.AriaGolden] compare it.
//
// # Debug overlays
//
// [Debug] turns on the overlays of TELA_DEBUG (layout, paint-flash, layers,
// fps, semantics) in a mounted app or a headless render, so a golden shows
// them. The environment variable itself is ignored: a test only gets
// overlays it asks for. A paint flash fades over 300 ms of app time and the
// fps meter redraws on every frame; neither keeps actions waiting, so use
// [App.Advance] to look at them, and [DebugClock] to script the frame
// times the meter shows.
//
// # Headless rendering without a test
//
// [Render], [AriaSnapshot] and [LayoutJSON] mount a tree, run one frame and
// return the image, the aria snapshot or the layout JSON, for tools. `tela
// snapshot` and `tela inspect` are built on them.
package telatest
