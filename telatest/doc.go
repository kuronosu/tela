// Package telatest tests Tela UIs without a window: it mounts a tree in a
// headless window, simulates a user with real pointer and key events, finds
// elements by role and text like Playwright, and compares the pixels with
// goldens (ARCHITECTURE section 17).
//
//	func TestCounter(t *testing.T) {
//	    app := telatest.Mount(t, Counter(), telatest.Size(400, 300))
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
// [App.Click] (with [Right] for the secondary button and [Delay] for a held
// one), [App.DoubleClick], [App.Tap], [App.LongPress], [App.Hover],
// [App.Focus], [App.Scroll], [App.Drag], [App.DragBy], [App.Press],
// [App.Type] and [App.Fill] act like a user. The pointer actions move the
// pointer to the element's center and send pointer events through
// hit-testing and the gesture arena (hover and :active states, focus on
// press, click counts, drags); Tap and LongPress use a touch pointer. The keyboard
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
// is logged, and actions then run only a few frames. Both are decided in
// app time ([Timeout] of it), so a slow machine never cuts a finite
// animation short.
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
// root class, color scheme, theme and class aliases, and the fields of the
// OS window: Title, size limits, Frameless, Hidden, Placement, OnClose...),
// the App that tela.Main runs and `tela snapshot` reads from the package;
// [FromApp] gives the settings that shape what is drawn to Mount, Render and
// the other entry points.
//
// # Windows
//
// An app has one window per tela.Window that is mounted, besides its main
// window. The headless backend plays a fake window manager (ADR-179): it
// grants every request at once, gives hidden and minimized windows no
// frames, and records parents, modality, hit regions and caption presses.
// The app still builds and runs the effects of a hidden or minimized
// window, as on a desktop (ADR-185): a tray item clicked while the main
// window is hidden updates its state, its tray menu and the windows it
// opens; settling waits for that work, and the window draws it when shown.
//
// An [App] is scoped to one window. Mount returns the one of the main
// window; [App.Windows] lists the open windows (in the order they opened,
// the main window first), [App.Window] selects one by title, [App.WindowAt]
// by index and [App.LastWindow] is the newest, and [App.WaitForWindow] runs
// a step and returns the window it opened, like Playwright's
// context.pages() and page.waitForEvent('popup'). Each returns an App with
// the same queries, actions, expectations, goldens and pixels as the main
// one, working on that window: it is always the same App for the same
// window.
//
//	settings := app.WaitForWindow(func() {
//	    app.Click(telatest.ByRole("button", "Settings"))
//	})
//	settings.ExpectText("Preferences")
//	settings.Golden("settings.png")
//
// Selecting a window waits for it to open, like a locator waits for its
// element; the clock, settling and the end of the app belong to the whole
// app, so an action in one window waits for the work it causes in the
// others. A pointer press or a key in a window that is not the active one
// activates it first, as the user clicking or typing there does.
//
// The window itself: [App.Title], [App.WindowState] (tela.WindowState, as
// UseWindow sees it), [App.WindowSize], [App.Placement], [App.Modal],
// [App.Parent], [App.Blocks], [App.BlockedBy] and [App.Closed]; the
// expectations [App.ExpectTitle], [App.ExpectWindowState] (with [Maximized],
// [Fullscreen], [Minimized], [Hidden], [Active] and [ClientDecorations]),
// [App.ExpectClosed], [App.ExpectWindowCount] and [App.ExpectNoWindow]. What
// the user does with the window manager: [App.RequestClose] (the close
// button), [App.Minimize], [App.Maximize], [App.Unmaximize],
// [App.SetFullscreen], [App.Restore], [App.Move], [App.Resize] and
// [App.BringToFront]. A window blocked by a modal one drops input.
//
// Custom title bars: [App.HitRegions] and [App.HitAt] read the regions
// h.WindowDrag and h.WindowControl send to the OS, a click on the drag area
// ([App.Click], [App.ClickAt], [App.DoubleClick]) is the OS's press
// ([App.CaptionDrags], [App.WindowMenus], a double click maximizes), and the
// controls are buttons that act on click.
//
// Quitting: [App.ExpectQuit] waits for tela.Quit or a close request of the
// main window without an OnClose; the app is then over, as when tela.Main
// returns: every window unmounted, the cleanups of the effects run, and
// further use fails the test. [App.HasQuit] and [App.ExpectRunning] check
// that it did or did not. The window options ([Title], [MinSize],
// [MaxSize], [FixedSize], [Frameless], [AlwaysOnTop], [TitleBar], [Backdrop], [StartHidden],
// [Icon], [Placement], [OnClose], [Screen]) set the main window's fields
// for Mount.
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
