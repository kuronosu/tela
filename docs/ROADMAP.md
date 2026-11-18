# Tela Roadmap

> **Living** document: what is left and where we are, kept short (ADR-165).
> Design lives in [`ARCHITECTURE.md`](ARCHITECTURE.md) and decisions in
> [`adr/`](adr/README.md); finished phases, old status notes and the session
> log are in [`HISTORY.md`](HISTORY.md), versions in [`CHANGELOG.md`](../CHANGELOG.md).
> Future phases have no number: a phase takes the next one when it starts, and
> every open limitation is a task of the phase that fixes it (ADR-218), or, when
> it is small and no planned phase covers it, an entry of Known limitations in
> [`LIMITATIONS.md`](LIMITATIONS.md) (ADR-267, ADR-290).

## Current status

- **Done:** phases 1–26. Latest version `v0.14.7` (2026-11-17, phase 26: Packaging and auto-update, ADR-294 to ADR-297; the tag waits for the owner to push). Versions follow ADR-208: a release that breaks the public API raises the minor version, any other the patch; the owner may release a small break, such as a rename, as a patch.
- **Phase in progress:** none. Phase 26 is merged into `main` locally; the owner pushes `main` and the tags `v0.14.5`, `v0.14.6` and `v0.14.7`.
- **Website:** `https://telaui.dev` is live with the landing page and the punched-card logo (ADR-206, ADR-207), and serves the docs of the latest release tag. Only tags deploy it: a release tag `v*`, or a site tag `site-YYYY-MM-DD` for a change to the site between releases, which the owner pushes (ADR-229).
- **Next step:** the next phase in order, App building blocks. Phase 26's Mac checks wait for a macOS CI run or a Mac (`docs/LIMITATIONS.md`). The GPU backend's Windows code has never run: its live check needs a Windows machine with a GPU. R11–R13 of the 2026-10-31 complexity review are still not started.
- **Machine and process notes:** live window checks run in a separate headless sway with a virtual pointer, not in the owner's Hyprland session (Hyprland 0.56 changes antialiased edges by ±1–2 levels when compositing; method in the session memory); no local Wine (it froze the host; CI keeps its Wine job); the owner runs `go test` once at the end of a batch of agent work, not during it; the race detector on `internal/core` takes about 15 minutes in one process (2026-11-09; run it with `-timeout 30m`, or shard it; longer since phase 24's `TELA_CHECK_SEMANTICS` and `TELA_CHECK_LAYOUT` are on in core's tests), the arm64 run of `internal/gfx/raster` passes the 10-minute default timeout in one process, so it runs sharded; CI runs the full `go test ./...` with `-timeout 20m` because `examples/dashboard` alone takes 7–9 minutes on a 4-vCPU runner (its damage checks play one scale each by default, `TELA_CHECK_DAMAGE=1` plays all, `TELA_PERF=1` adds the x2 sessions) (build the test binary once, split `-test.run` across processes, one per `TestDamageMatchesFullRender` scene). Under qemu, `internal/gfx/raster` takes about 9.5 minutes on a loaded machine: run the arm64 check with `-timeout 40m`. Subagents never push; the main session pushes the work branch when the owner allows it (ADR-220).

## Phases

In order. Each phase ends with verifiable criteria and is not done unless they pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references). What needs hardware or a desktop the project does not have goes to [Live checks](#live-checks-not-blocking) and does not block the phase. Phases 1–26 are done (HISTORY.md).

### App building blocks

What a desktop app needs besides its interface: loading data, signing in, navigating, showing formatted text and being present in the system. Each piece is usable alone. Agents: `tela-impl` for the API, router, Markdown, OAuth and templates; `tela-core` for the desktop surfaces.

- [ ] First: an ADR on the phase's public API, and on Markdown parsing (port goldmark, MIT and pure Go, or a parser of Tela's own)
- [ ] `tela.UseAsync[T]`: runs `f(ctx)` on a goroutine and returns `Async[T]` (`Loading`, `Err`, `Data`, `Reload()`); cancels the context when the deps change and drops stale results; keeps `Data` across `tela dev` reloads. Also `tela.UseDebounced`
- [ ] Navigation: a router with a stack, parameters, back and forward; the mouse's side buttons and Alt+Left/Right (Cmd+[ and ] on the Mac) navigate as in a browser; deep links from `desktop.UseLaunch` open routes; screen transitions through `LayoutID`
- [ ] `kit.Markdown`: CommonMark with GFM tables, task lists and strikethrough; selectable text, code blocks in monospace, links opened with `desktop.OpenURI`, images from `tela.Assets`, the theme's tokens. The editor is outside the phase
- [ ] OAuth for desktop apps: authorization code with PKCE and a loopback redirect (RFC 8252) in the system browser, tokens kept with `desktop` secrets, refresh with rotation, sign-out; optionally the device code flow (RFC 8628); tested against a local fake server
- [ ] SQLite: a recipe and an example in a module of its own (its own `go.mod`, like `looks/`) with a pure-Go SQLite; the core module gains no dependency
- [ ] Desktop surfaces (platform contract v15): start at login (`SMAppService` on macOS, the `Run` key on Windows, XDG autostart on Linux); the Dock menu on macOS; jump lists and recent documents (`ICustomDestinationList` and `SHAddToRecentDocs` on Windows, `NSDocumentController` on macOS, `recently-used.xbel` on Linux)
- [ ] Templates: `tela new -template notes|settings|dashboard`, generated from the examples

**Done when:** `examples/notes` signs in against the fake OAuth server, loads and syncs its notes with `UseAsync`, keeps them in SQLite (from the example's module), shows them as Markdown, navigates back and forward from the mouse and the keyboard, starts at login and appears in the recent documents, with `telatest` tests and goldens on Linux and Windows CI (macOS on CI where it can be driven). Outside the phase: a Markdown editor, sync and offline beyond the example, a webview (ADR-001), push and background tasks.

### DevTools and agents

Inspecting and driving a running app.

- [ ] Inspect's and LayoutJSON's `font` field shows the first name of the stack (`ui-sans-serif`), not the family that drew the text
- [ ] Live inspector: element tree, computed style, hooks and state, re-render counts, accessibility tree
- [ ] Frame profiler
- [ ] Connect to a running app and pick an element
- [ ] `tela mcp` on a running app's real windows, all of them (today headless only, and only the main window), for what headless does not show: IME, menus, dialogs, the tray, the accessibility backends, the display scale; a session could attach through the `remote` host of `tela dev`
- [ ] `tela mcp`: refs keep node identity (today role, name and position); `scroll` without an element; a locator that matches nothing does not always wait 2 s; `notifications/cancelled` is honored
- [ ] `tela snapshot` shows a passive effect's state and toasts set by effects (it renders once)
- [ ] Debug-overlay labels are laid out at the display scale (today at scale 1)
- [ ] `tela lsp`: completion, hover and diagnostics of Tailwind classes inside `Class("...")` and `ClassAliases`, on the lint engine
- [ ] `tela snapshot`, `inspect` and `mcp` give each element the line that created it (`button "Save" [ref=e4] [src=settings.go:42]`), from the call stacks dev mode already captures for hooks
- [ ] `tela mcp` writes the session it just drove as a `telatest` test (`Click`, `Fill`, `Advance`, `ExpectAria`), so what an agent checked stays as a regression test
- [ ] `tela mcp` and `tela snapshot` film motion: frames every N ms over a span of the app's clock, as a contact sheet or an APNG (transitions, springs, `UsePresence`)
- [ ] `tela mcp`: `state` (a component's hook values, which `tela dev` already serializes), `logs` (the app's log and dev warnings), `perf` (time per phase of each frame), `prefer` (reduced motion, high contrast, text scale, locale, direction; `open` covers only dark mode and size); merge tools rather than add them, the set stays small
- [ ] `tela new` also writes `AGENTS.md` and `CLAUDE.md` (read `llms.txt`, the `tela mcp` loop) and a `.mcp.json` that starts `tela mcp`

**Done when:** DevTools shows why a given element re-rendered and where a frame spent its time, and `tela mcp` drives a running app's windows.

### Dev loop and testing

`tela dev`, `telatest`, `watch` and the test infrastructure.

- [ ] `tela dev` keeps what a swap loses today: kit components' aria hook state held in refs (silent resets), a renamed component's state, an IME composition
- [ ] `tela dev` with desktop services: a drop's final effect set inside `OnDrop` reaches the source; a refused or taken shortcut status is not cached until `tela dev` restarts; a window a new build opens only later (from an effect) does not close and reopen on reload; a dialog opened at startup does not show again on every reload; a reload while a Wayland window is minimized does not stall; the Windows 11 backdrop reaches the client area
- [ ] Dev builds can be debugged (DWARF)
- [ ] `telatest`: clicks with modifiers (Playwright's `click({modifiers})`; aria's Cmd+click tests mount through `rawwin` today); menu accelerators' `CmdOrCtrl` follow the host, not `PlatformKeys` (ADR-221)
- [ ] A public accessibility check in `telatest` (such as `App.ExpectAccessible()`, needs an ADR) from the axe-like rules of `telatest/a11y_audit_test.go`
- [ ] `tela check` and `tela verify` (CLI and `tela mcp`): report clipped or overflowing text, overlapping elements, low contrast, small targets and focus problems, for every `Preview` in light and dark, at several widths, with large text, in RTL and in another language
- [ ] Every change that breaks the public API ships a `tela lint -fix` rule that rewrites the old spelling
- [ ] `telatest.TrayIcon` reports `Template` (ADR-237)
- [ ] `telatest`: plays Windows' `DefaultAsked` for file types and gives a start launch; a wait on a long timer of the app's clock advances to the next deadline when nothing else is due (today one frame per try, about 120 tries for 2 s, so it depends on the machine's speed)
- [ ] `watch`: find why `TestDirectoryMovedInAndOut/native` failed once on `windows-latest` (`c9aff8d`: nothing reported for a directory moved into the tree, no error; the watcher and the test now report more); the Windows poller holds each directory open while it lists it, so a rename of that directory fails for that moment
- [ ] CI: the win32 tests that read the screen (title bar colors, backdrop, frameless modal and glass pixels) or open real dialogs run only with `TELA_LIVE=1`, so CI checks their attributes but not their pixels; the win32 keyboard tests inject their "slow" keys 40 ms apart from a goroutine and check the focus only at the start, so they rely on running alone; a global-shortcut press injected with `keybd_event` or `SendInput` from a scheduled task never arrives, so only a real key press checks it
- [ ] `docs/llms.txt` is a few bytes under `llmsdoc`'s 100 KB limit (about 25k tokens an agent reads whole): split it into a short core every agent reads and recipes looked up per question; phase 20's proposed lines (transparent windows, built-in labels, `BackgroundGradientFn`, `Progress.ValueFn`, `ToggleGroup.Indicator`, Preview themes) and phase 22's (Linux backdrops, `i18n` number options and skeletons, access keys) wait for it, and so does `App.ExpectIdle` (ADR-291), and so does `App.ExpectIdle` (ADR-291)
- [ ] Common mistakes in `llms.txt` with a lifecycle: only a mistake reproduced on the current API, written as symptom, cause and fix, marked when `tela lint` could catch it, and removed when lint or a dev error does
- [ ] `telatest`: no public way to read `aria-valuenow` (only `HasValue`; a `HasRange(now, min, max)` state); `Stats.Layouts` counts layout phases, including empty ones of restyle-only frames, so "no relayout" cannot be asserted there (count boxes laid out)
- [ ] CI: `cmd/tela`'s out-of-repository tests (dev, mcp, new, outside modules) skip when `go mod tidy` cannot resolve offline; the macOS job warms the module cache first, the Linux and Windows jobs do not, so there they likely skip; lint runs staticcheck for Linux only (`watch.(*Watcher).lost` is unused on darwin)
- [ ] `telatest`: after frames have run, a primary click at a point with no rendered content under it (the empty area of an `h-full` page) does not blur the focused element (found in phase 22; happens on `main` too)
- [ ] Tests that rasterize a `text.Layout` at x2 should lay it out at that scale (`Paragraph.Scale`): `TestChromiumClipText` laid out at 1 and drew at 2 (fixed in phase 23); check the others
- [ ] `internal/platform/x11`'s `TestCreationFrameExtentsLate` and `TestCreationFrameExtentsWait` failed once under the race detector while other packages ran beside it (2026-11-12: "0 timers fired, want 1", and the late answer missed the shown window); they pass alone and in the package's own race run: find the timing they depend on
- [ ] `internal/watch`'s `TestDebounce/native` failed once under Wine in CI (2026-11-16: "batch 426ms after the last write, before the debounce"); it passed on a rerun of the job
- [ ] `internal/platform/remote`'s `TestAccessibilityForwarded` failed once in a full run (2026-11-16: "an action reached a child that had not sent its tree"); it did not fail again in 80 runs alone or 30 under load
- [ ] `internal/platform/remote`'s `TestDisconnectKeepsLastFrame` failed once on a heavily loaded machine (2026-11-11): the child's clean `Quit` reached the host as `recvmsg: connection reset by peer`, so `Err` was not nil; it passes 500 times in a row otherwise. A clean disconnect should not depend on whether unread data was in the socket
- [ ] `cmd/tela`'s `TestOverlayCloseButtonAndClear` timed out once on `macos-latest` (2026-11-09, the panel's pixel (0,0) was `{1 0 0}` instead of `{0 0 0}` for 60 s) and passed on a re-run, and `TestOverlayResizeRedraws` failed the same way on 2026-11-11 (passed in the next run of the same commit): find the race
- [x] Counted budgets in the gate (ADR-290, ADR-291): `examples/hello`, `examples/kit` and the dashboard are idle once settled (`App.ExpectIdle`); the dashboard's work per hover, key and scroll frame and its allocations per steady frame (`examples/dashboard/budget_test.go`); the `examples/hello` binary at most 22 MB (the gate's `size` step)
- [x] Timed budgets at phase close, a table in ARCHITECTURE §18 (measured 2026-11-17, `TestLiveColdStart` in `examples/hello`): cold start to first frame (hello, kit; Wayland and X11), the `tela dev` swap, `VirtualList` p95 at 100,000 rows, dashboard frames over 16.7 ms (CPU, RTX 3060 Ti, UHD 630), `gpu.Open`; the README's numbers come from that table
- [ ] The gate after an edit of a package nothing depends on: under 10 s (the gate does not check it)
- [ ] The gate after an edit of `internal/core` (12 threads): 46 s when its exported API is unchanged, 122 s when it changes (2026-11-16, vet of 7 targets and 24 test binaries rebuilt). The full `-short` suite takes 54 s; `examples/dashboard` sets it (48 s, CPU-bound: `TestInteractionDamage`, `TestScrollDamage`, `TestDragDamage`) since posts reach their own app (ADR-292), `devNow` no longer reads a goroutine id and `internal/lint` reads export data (ADR-293)

**Done when:** each item has a test that failed before it, on Linux and Windows CI.

### External textures

GPU content an application renders itself, composed by Tela (the owner, 2026-11-15): a 3D view, a video decoded in hardware, a map, a game view. Flutter's `Texture` model: the application draws with the graphics API it chooses and hands Tela a texture; Tela never exposes its own device. Placed before Canvas and Audio and video because both build on it: one model for content drawn by the application inside the tree, and `Video` shows its frames through it.

- [ ] First: an ADR on the public element (such as `h.Texture`) and its source type: what the application hands over on each platform, who allocates, how a new frame is announced, and how sizes are negotiated on resize and scale changes
- [ ] Linux: a dmabuf (descriptors, fourcc, modifier, planes) imported by the GPU renderer as an image source, on the exchange the GPU backend's presenting already has (`internal/vk`'s external memory); Windows: a shared handle; macOS: an IOSurface
- [ ] Composed as any image: clips, rounded corners, opacity, transforms, filters and blend modes, hit testing, a semantics role and name
- [ ] Frames: the application announces a new frame from any goroutine and only that element is damaged; a fence or an acquire and release pair so that Tela never reads a frame being drawn; 0% CPU while the content rests
- [ ] Without the GPU backend (a default build, or `TELA_RENDERER=cpu`) the same element works through a copy: pixels the application gives, or the mapped buffer
- [ ] `telatest`: a fake source, so screens with a texture have headless goldens
- [ ] An example whose application renders an animated scene with its own code

**Done when:** the example shows animated content rendered by the application inside a rounded, clipped card at 60 fps at scale 2 on Linux with no copy through the CPU, and the same binary without the GPU backend shows it through the copy; Windows and macOS go to the live checks.

### Canvas, SVG and PDF

Vector output on the `DisplayList`.

- [ ] `Canvas` (2D): immediate-mode drawing surface (paths, gradients, text, images) with an animation-frame callback, with the method names of the browser's Canvas 2D API and compared with Chromium's output
- [ ] SVG: text, filters, images, patterns and markers
- [ ] Printing and PDF export: a vector PDF backend over the DisplayList (fonts embedded, gradients, clip, opacity), print dialogs, page-break CSS subset

**Done when:** a `Canvas` example animates at 60 fps, the new SVG features match their resvg test cases, and a page exports to a PDF that two viewers render identically.

### Audio and video

Highest uncertainty of the roadmap: pure-Go decoders limit the codecs.

- [ ] Audio playback and capture: pure-Go decoders (WAV, MP3, OGG/Vorbis, FLAC, Opus where one exists), output through ALSA/PulseAudio/PipeWire, WASAPI, CoreAudio
- [ ] Video playback: patent-free subset first (VP8/VP9/AV1 if a pure-Go decoder is viable), A/V sync, a `Video` component with `kit` controls; an ADR settles codec scope before work starts
- [ ] 3D (exploratory, ADR first; transforms stay 2D until then) lives here or in the backlog; an application can already show its own 3D rendering through External textures

**Done when:** a "player" example plays an audio file and a video with synced controls.

### Accessibility gaps

What phase 19's bridges (ADR-246) leave out, found while building and checking them.

- [ ] AT-SPI gaps: lines split only at hard newlines (no layout in the tree), character extents estimated, no text attributes; word segmentation is a simple rule, not UAX #29; no hypertext (a paragraph with a link exposes label objects); Selection over direct children only, no SelectAll/ClearSelection; no EditableText cut and paste; relations only controller-for/controlled-by (no labelled-by, described-by); announcements only through the Announcement signal (Orca 45+); Wayland screen extents are window coordinates; `Cache.GetItems` empty; not checked at HiDPI
- [ ] UIA gaps: never deactivates once a client asks; no MSAA answer for `OBJID_CLIENT`; approximate text units and range rectangles, `RangeFromPoint` returns the start, `FindAttribute` finds nothing; Scroll pattern percents are placeholders; no AutomationId, ClassName, Window pattern on dialogs, Grid or Table patterns, LabeledBy, DescribedBy or document TextPattern; dead node objects stay while UIA holds them (no per-node `UiaDisconnectProvider`); not checked above 100% DPI
- [ ] NSAccessibility: an AX client in the app's own process is answered off the main thread, so the bridge then sends AppKit read-only messages there (`firstResponder`, the superclass's answers, element creation) and uses the view rectangle recorded at the last sync; the CI client test runs in-process, so a test with an out-of-process client should check the main-thread path VoiceOver uses; TelaAccessElement answers the old attribute API itself because AppKit's bridge to the protocol methods ignored them on macOS 26 (declaring protocol conformance was not tried)
- [ ] NSAccessibility gaps: never deactivates once an AX client asks; hard lines only, `accessibilityFrameForRange:` is the element's frame; English role descriptions for web subroles; no Confirm/Cancel or scroll-by, no `accessibilityRangeForPosition:` or attributed strings; KeyShortcuts, Controls, ActiveDescendant, Sort and Current not exposed; `option` is AXStaticText, no AXOutlineRow in treegrids
- [ ] Both bridges and AT-SPI: setting a range value steps one increment or decrement toward it (the tree has no settable number)
- [ ] Accessibility updates: set value works only on text fields (sliders and spinbuttons step with increment and decrement); live-region announcements count texts and image names only, not an `aria-label` alone; a busy region that appears after activation announces its old content when it stops being busy; a window's tree is sent only after its first frame; an error that is both a field's description and an alert is spoken twice by Orca while the field has focus (the description change and the announcement)
- [ ] `tela dev` and accessibility: an assistive technology's action that arrives while input is frozen for a swap is dropped; every reload replaces the bridge's tree (node IDs change with the child's serial), so the reader loses its position
- [ ] Phase 23: the content language (`h.Lang`) is not exposed (AT-SPI `Locale`, UIA's culture, `AXLanguage`; a field through the platform contract and the remote protocol), and a static text selection is not either (semantics carries a selection only for fields)
- [ ] Semantics gaps: radios outside a radiogroup are not grouped by name and tree items without groups are not split by `aria-level` for set positions (Chromium does both); `IncrementNode`/`DecrementNode` need a focusable widget (Chromium sends the key without focusing); a dev warning for a focusable element with no role or name (a focusable `kit.ContextMenu` area has neither unless the app gives them)

## Known limitations and live checks

In [`LIMITATIONS.md`](LIMITATIONS.md) (ADR-290): small gaps no phase is planned for (ADR-267), and checks that need hardware or a desktop the project lacks (ADR-218). Neither blocks a phase.

## By design or outside Tela

Not planned; the owner can reopen any of them.

- No margin collapsing, floats or tables (ARCHITECTURE §10); 3D transforms are out of scope (see Audio and video).
- Linux file dialogs need the XDG Desktop Portal (`xdg-desktop-portal` with a backend) and have no fallback dialog (the owner's choice, 2026-10-27).
- Phase 22 decisions: Linux and Windows associations have no Editor/Viewer/Shell roles and macOS cannot clear a user's default (ADR-262); on Windows, and on file systems without an atomic exchange (NFS, SMB, FAT), a store write in the instant between its check and the rename can still be lost (ADR-262); KDE's `org_kde_kwin_blur` is LGPL, so older KWin Wayland versions get no blur (ADR-261); a FixedSize Win32 transparent window has no thick frame, and macOS's own resize loop does no edge snapping (ADR-261); `looks/aqua`'s Default and Link buttons keep macOS's accent at 2.8–4.6:1 (ADR-264); a date range under RTL reads start-to-end from the right, as in Chromium (ADR-264).
- Phase 25 decision (the owner, 2026-11-16): shadows under a skew stay different between the backends. The GPU evaluates them in the element's local space, as CSS defines (blur, then transform); the CPU rasterizer blurs the transformed shape in device space with the two marginal sigmas, as Skia does for a non-composited layer, and differs by up to 15 levels over the whole shadow. Skews with shadows are rare in a UI; the comparison test has overrides with this cause.
- Phase 23 decisions: a `Button` starts no text selection (Chromium starts one from its text; the owner's rule for controls), though its text is copied when a selection crosses it; `hyphenate-character` and `hyphenate-limit-chars` have no Tailwind utilities; Tela hyphenates as Chromium on Linux and Windows everywhere (Chromium on macOS uses CoreFoundation's points); text with no declared language hyphenates in the default language, as Blink's `LocaleOrDefault`.
- Wayland cannot place windows or keep them on top; a Wayland window shown again gets a new session restore token (ADR-186).
- GNOME's AppIndicator extension applies a tray menu's changes only when the menu opens, so the old label shows for a moment (its design; GTK apps behave the same).
- xdg-desktop-portal-hyprland assigns no keys to global shortcuts: the user binds `appID:id` in the compositor's config.
- X11 XIM: no trigger-key (dynamic) event flow, authentication or string conversion; an `XIM_ERROR` with an empty body (xcb-imdkit, IMdkit) during `CREATE_IC` costs the 5 s handshake timeout; CNS 11643 and Big5 in COMPOUND_TEXT decode as U+FFFD; balloon notifications (Windows before 1809, Wine) keep one notification-area icon each (ADR-231, ADR-235).
- Environment: Omarchy's window rules on Hyprland 0.56 did not keep the restored size of `examples/notes`; `wtype` cannot type into Xwayland windows (Hyprland); the Windows VM's Japanese IME does not compose, so `TestLiveIME` fails there.

## Website (ADR-206)

- [x] `site/`: Astro + Starlight built from `docs/wiki` by `site/scripts/sync-wiki.mjs`, links and anchors checked at build; `.github/workflows/site.yml` checks pull requests and deploys `main`'s site with the latest tag's docs; live at `https://telaui.dev` since 2026-10-31 (DNS on Cloudflare, DNS only; Pages with HTTPS)
- [x] Landing page and logo (ADR-207, 2026-10-31): the owner chose prototype D and the Jacquard punched-card mark after rendered rounds; `site/snippets` re-rendered by CI (`render-snippets.sh --check`); `telaui.dev/llms.txt` served; the design history (prototypes, the 42-site survey, logo rounds) is in the owner's `~/Documents/tela-website/`, outside the repo
- [ ] Before the next release: the comparison table (`site/src/data/compare.json`) is dated 2026-10-31; re-check its cells against `compare-sources.md` when a compared project ships a major version (Wails v3.0.0 added an MCP server five days before)
- [ ] Two release tags pushed together deploy in parallel and the last to finish wins: on 2026-11-08 `v0.13.5`'s deploy ended after `v0.13.6`'s and the site served the older docs until `v0.13.6`'s run was re-run. Deploy only when the tag is the latest `v*` tag (or re-check after the `pages` concurrency group)
- [ ] Decide when the GitHub wiki retires (until then both are published from `docs/wiki`); wikidoc's reference pages still say "start at the wiki home", and on the site the reference pages' contents list "Overview" twice (Starlight's page top and the package overview)

## Road to v1.0

`v1.0.0` is not scheduled; these gate it.

- Phase 7's breaking changes are done and their deprecations removed. From phase 10 (`kit` on the final API) on, **breaking changes need an ADR and a deprecation cycle**.
- The API freezes when the phases up to Accessibility are done: stable `tela`, `h`, `aria`, `kit` and `telatest`; Linux (Wayland and X11), Windows and macOS supported, macOS once its live-check session passes; accessibility working on all three. The GPU backend changes no public API and does not gate it.
- Godoc and wiki cover every exported symbol with a runnable example; `llms.txt` matches the API.
- At least two non-trivial applications outside the repository build against each release in CI.

## Separate packages

Optional, outside the core module. Until `tela add` exists (see "Deferred") they are plain Go modules that applications import. They are not scheduled; each starts when a real application needs it.

- **Charts:** line, bar, area, pie, scatter, axes, tooltips, built on `Canvas` (Canvas, SVG and PDF) or plain `Div`/SVG until then
- **DataGrid / TreeView:** sortable, resizable, editable cells, row selection, grouping, tree expansion, on top of `VirtualList`; the simple `Table` in `kit` stays in core
- **Third-party packages with an implementation per system:** a documented convention (build tags or files per OS, no cgo, a fake for headless tests) so others can publish services like `desktop`'s
- **Looks (`looks/`, ADR-197, ADR-223):** done for Windows 11, GNOME, KDE and macOS (`win11`, `adwaita`, `breeze`, `aqua` in two designs, `detect`, and a gallery)

## Deferred (not discarded)

Out of scope for now, but kept open: a decision to add the platform items needs an ADR amending the non-goals in ARCHITECTURE §1, and the platform layer must not preclude them.

- **WebAssembly / web:** a canvas-backed backend
- **Android / iOS:** touch, virtual keyboard, app lifecycle, safe areas
- **Touch input on the desktop** (ADR-265, ADR-267): no backend reports `PointerTouch` (Wayland's `wl_touch` is not bound, Win32 and X11 deliver touches as emulated mouse input), so there is no touch text selection (long press, handles) and no long-press drag of a selection with a touch `StartDrag` (Wayland `start_drag` with the touch-down serial)
- **Embedded web content:** a webview or HTML component (contradicts "no webview", ADR-001)
- **Context-click selection in grid, tree and tag collections** (ADR-247): when Tela has such collections with selection, a right or Control click on an unselected item selects it first, as list boxes do (owner, 2026-11-08).
- **`tela add` and a component registry.** Picked up when `kit` has real users or a third party wants to publish components. `kit` is written meanwhile as self-contained files (no `internal/` imports, tokens instead of fixed colors), so a move to copied-source distribution stays cheap. Design questions for the ADR (options go to `API_PROPOSALS.md`):
  - **Unit of distribution:** copied source (shadcn style), Go modules, or a hybrid (copied source for UI components, modules for logic-heavy packages)
  - **Registry format:** static JSON index plus files over HTTPS with content hashes, the Go module proxy, or a git repository
  - **Updates:** a lockfile with per-file hashes and a three-way `tela diff` / `tela update`, so local edits survive
  - **Dependencies:** between items, on Go modules (pure Go only) and on theme tokens
  - **Trust:** diff before writing, no install-time code execution, hash pinning, optional signatures, a registry allowlist, a license recorded per item
  - **Third-party registries**, registry authoring tools, and non-interactive / `--json` modes with `tela mcp` tools for agents

## Discarded

Considered on 2026-10-20 and not planned. They can be reopened by the owner.

- **Visual designer** (Scene Builder / XAML Designer style): `Preview()`, `tela snapshot` and `tela inspect` serve the same loop for code and agents
- **Component gallery** (Storybook style): partly covered by `Preview()` and the `examples/`
