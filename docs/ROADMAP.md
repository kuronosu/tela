# Tela Roadmap

> **Living** document: what is left and where we are, kept short (ADR-165).
> Design lives in [`ARCHITECTURE.md`](ARCHITECTURE.md) and decisions in
> [`adr/`](adr/README.md); finished phases, old status notes and the session
> log are in [`HISTORY.md`](HISTORY.md), versions in [`CHANGELOG.md`](../CHANGELOG.md).
> Future phases have no number: a phase takes the next one when it starts, and
> every open limitation is a task of the phase that fixes it (ADR-218), or, when
> it is small and no planned phase covers it, an entry of Known limitations (ADR-267).

## Current status

- **Done:** phases 1–22. Latest version `v0.14.2` (2026-11-09, phase 22: desktop, kit and text details). Versions follow ADR-208: a release that breaks the public API raises the minor version, any other the patch; the owner may release a small break, such as a rename, as a patch.
- **Phase in progress:** 23, Layout, text and engine fidelity, on `phase-23-layout-text-engine-fidelity`. Owner's answers (2026-11-11): static text is selectable by default as on the web (`user-select: auto`; controls and kit chrome `select-none` where Chromium starts no selection), a selection crosses paragraphs inside one container; soft hyphens always and `hyphens-auto` through opt-in language packages, like fonts; `h.Lang` (HTML's `lang`, inherited), set by `i18n`'s provider from the active locale, the system's language otherwise; the work branch may be pushed for CI (ADR-220).
- **Website:** `https://telaui.dev` is live with the landing page and the punched-card logo (ADR-206, ADR-207), and serves the docs of the latest release tag. Only tags deploy it: a release tag `v*`, or a site tag `site-YYYY-MM-DD` for a change to the site between releases, which the owner pushes (ADR-229).
- **Next step:** phase 23's tasks are merged (ADR-268 to ADR-273); the full suite, the race detector, a review over the phase and CI on the work branch remain before closing it. R11–R13 of the 2026-10-31 complexity review are still not started.
- **Machine and process notes:** live window checks run in a separate headless sway with a virtual pointer, not in the owner's Hyprland session (Hyprland 0.56 changes antialiased edges by ±1–2 levels when compositing; method in the session memory); no local Wine (it froze the host; CI keeps its Wine job); the owner runs `go test` once at the end of a batch of agent work, not during it; the race detector on `internal/core` takes about 15 minutes in one process (2026-11-09; run it with `-timeout 30m`, or shard it), the arm64 run of `internal/gfx/raster` passes the 10-minute default timeout in one process, so it runs sharded; CI runs the full `go test ./...` with `-timeout 20m` because `examples/dashboard` alone takes 7–9 minutes on a 4-vCPU runner (its damage checks play one scale each by default, `TELA_CHECK_DAMAGE=1` plays all, `TELA_PERF=1` adds the x2 sessions) (build the test binary once, split `-test.run` across processes, one per `TestDamageMatchesFullRender` scene). Under qemu, `internal/gfx/raster` takes about 9.5 minutes on a loaded machine: run the arm64 check with `-timeout 40m`. Subagents never push; the main session pushes the work branch when the owner allows it (ADR-220).

## Phases

In order. Each phase ends with verifiable criteria and is not done unless they pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references). What needs hardware or a desktop the project does not have goes to [Live checks](#live-checks-not-blocking) and does not block the phase. Phases 1–22 are done (HISTORY.md).

### Phase 23: Layout, text and engine fidelity

Differences from Chromium and missing typography; each fix comes with a fixture or golden.

- [x] Layout: two `w-full aspect-square max-h-[20px]` flex items are clamped by the transferred max (Chromium 140×20, Tela 20×20); a ratio item with an auto or keyword width in a column of indefinite width is 5×5 where Chromium gives 30×30; grid items' percentage min sizes keep the old rule; `grid-template-areas` is unreachable from classes; an out-of-flow box with auto insets inside a paragraph sits below the paragraph (Chromium: at its place on the line); an absolute child of a `relative` inline span is placed against the nearest positioned block
- [x] System CJK fallback on the Windows 11 VM: a Japanese and Chinese paragraph draws in two families, "Noto Sans JP Thin" (a static Thin family the user installed, taken at weight 400) and Microsoft YaHei; `TestSystemFontsLive` with `TELA_SYSTEM_FONTS=1` fails there, on `main` too. Rank a family's weight distance and keep one family per script (fixed with a synthetic index of the VM's fonts; the VM run is in Live checks)
- [x] A growing textarea (`field-sizing-content`) with long text can widen its grid column past the card unless the column has `min-w-0`; check against Chromium first (Chromium lets it shrink; found while merging signup into login)
- [x] Text: selectable static text (`P`, `Span`; today only `Input` and `Textarea`), and a keyboard context menu over a selection in it opens at the selection; at x1.5 a baseline on an exact half device pixel can be drawn one row higher; hyphenation, skip-ink, synthetic bold and italic; CJK glyph forms follow the language; a test for a style run that starts inside a grapheme cluster (the itemizer then segments the run alone); a Textarea's placeholder is not checked against Chromium (only the Input's rule was measured, ADR-265)
- [x] Paint: border widths snap to device pixels at x1.5, as in Chromium
- [x] Engine: `ScrollOffset` read in the layout effect that scrolled returns the new offset (ADR-176); a box shown again that flips in the same frame restarts its entrance; drag-and-drop autoscroll covers text fields only (Chromium also scrolls scroll containers under a drag); a rotated field's drag image is drawn upright
- [x] Re-measure the scale-2 tolerances that about 12 Chromium comparison tests in `internal/core` and `internal/text` justify with ADR-048: ADR-159 superseded it (line boxes in device pixels, as Blink), so they may be looser than needed

**Done when:** every case above has a fixture or golden that matches Chromium up to text antialiasing, and static text can be selected and copied.

### CPU performance

The frame cost of the engine and the CPU rasterizer; the GPU backend builds on it. Baseline, the Frosted graphite dashboard (1080p, 12 raster workers, backdrop cache of ADR-200; quiet i5-10400, 2026-10-31): p50 4.2 ms, p95 17.8 ms at x1 over 726 frames; p50 9.0 ms, p95 40 ms at x2.

- [ ] Semantics: every commit while an assistive technology is active, and every `AriaLabelFn` change, rebuilds the whole semantics tree (no incremental `needsSemantics` marks, ADR-246); `platform.AccessTree.Apply` copies the whole node map per update and each bridge then rebuilds its own view (`buildView`, `newUIASnapshot`, `newAXTree`), O(nodes) for a one-node diff
- [ ] A static text selection's change walks the whole range, bounded by the deepest common ancestor of its ends (ADR-271)
- [ ] A box with any `…Fn` style prop records its list again whenever its element builds (`sameStyle` compares `Deferred` and a gradient Fn's placeholder layer by slice identity)
- [ ] Fonts: the sprite cache keys bitmap and COLR glyphs by text color, so one emoji in two colors is cached twice
- [ ] Linux: the portal's settings are read on the UI thread in `NewHost` while the font index loads beside it (3 → 8 ms); read them off the UI thread at `Run` start
- [ ] go-text's `NewFont` still copies glyf and gvar (ADR-243); a stub needs each glyph's bounding box and, for variable fonts, its points, so ADR-245's technique does not carry over cheaply
- [ ] The decoded-bitmap cache's keys hold `*Typeface`, so a dropped collection's typefaces stay alive until their images are evicted (little memory since ADR-245)
- [ ] Recording: a paint records again only the layers whose content changed (ADR-204), but the root list (everything outside scroll content, deferred layers and hoisted chunks) is recorded whole when anything in it changes (a hover on the dashboard's sidebar: 346 commands); adding or removing a hoisted box records its stacking context's list; a mounted `TextColorFn`, a box that starts or stops being a layer and layout-animation corrections still record everything
- [ ] Layout and walks: frames that build, restyle or lay out still place every box; a frame that is not a pure scroll walks, lays out and re-records every element, including those a scroll container hides (a plain 2000-row `kit.Table`: 9–15 ms per such frame; `kit.Table.RowHeight` makes it virtual, ADR-199); no paint culling of rows a scroll container hides; mounting the Calendar page takes 20–28 ms
- [ ] Damage: a scroll that moves most of the window is a full frame (the damage matcher gives up, ~28 ms at x1); a theme switch and the palette opening are full frames (56 ms at x1)
- [ ] Caches: backdrop filters inside layers, or over content that changes every frame, are not cached
- [ ] macOS: every `Present` copies the whole retained frame into a new `CFData` and `CGImage`, whatever the damage (about 20 MB per frame for a 2880x1800 Retina window); an `IOSurface`-backed layer would copy only the damage
- [ ] Allocation churn: the dashboard's 1080p session allocates 859 MiB in about 1150 ticks (1359 MiB at 4K x2; 780 and 1066 before ADR-241, whose 15-frame aging of the rasterizer's scratch trades allocations for a lower heap during activity) (`memreport -profiles`, `alloc_space`): display list recording (`gfx.(*Builder).record`, `recordPush`, `pop`), dashed strokes flattened again every frame (`dashSubpaths`, 89 MiB), `layout.(*Engine).flexItems`, `style.Compute`; it also sets the peak heap, about twice the live one
- [ ] Wayland: a third `wl_shm` buffer, allocated once when the compositor held two, stays until the next resize (one frame of shared memory, 33 MB at 4K)
- [ ] `examples/dashboard/README.md`'s Performance section was measured before ADR-200 and ADR-204 (676 frames, p50 15.2 ms, p95 34.4 ms at x1): measure again and rewrite it

**Done when:** the dashboard's p95 at x1 is under 16.7 ms on the CPU, a scroll of most of the window and a theme switch cost less than a full frame, and a frame of a plain 2000-row `kit.Table` that is not a scroll no longer lays out the hidden rows.

### GPU backend

The hardest technical item (`tela-hard`). It changes no public API.

- [ ] First: an ADR on presenting from the GPU without libwayland or libxcb, since Tela speaks both protocols in Go: dmabuf buffers through `zwp_linux_dmabuf_v1` and DRI3/Present, or a copy into `wl_shm`. It also settles what purego means for Linux binaries (dynamic linking)
- [ ] Vulkan (Linux, Windows) and Metal (macOS) via purego/syscall over the same DisplayList, with the fixed shader set of ARCHITECTURE §13
- [ ] D3D11 on Windows optional
- [ ] The CPU rasterizer stays the reference for goldens; GPU output is checked against it within a tolerance

**Done when:** the `dashboard` example renders at 60 fps at scale 2 on the GPU backend on the owner's Linux machine (RTX 3060 Ti and UHD 630), and GPU snapshots stay within the agreed tolerance of the CPU goldens; Metal and Windows GPUs are in the live checks (the Windows VM has no GPU).

### Packaging and auto-update

- [ ] `tela build` / `tela package`: installers and bundles (`.msi`/MSIX, `.app`/`.dmg` on the development bundle of the macOS desktop phase, AppImage, flatpak, `.deb`), icons, metadata, code-signing hooks (macOS notarization needs an Apple developer account); a signed bundle's `Info.plist` cannot be edited at run time, so a packaged Mac app declares its schemes and file types at build time (`assoc` refuses to edit a signed bundle, ADR-228)
- [ ] Windows targets are GUI programs by default (`-H=windowsgui`, no console window next to the app; ADR-127), with panics and the app's log written to a file in its data directory, since a GUI program has no standard output
- [ ] Auto-update: signed manifests, delta or full downloads, atomic replace, rollback
- [ ] A toast clicked after the app exited starts it (a COM activator registered with `ToastActivatorCLSID`, ADR-235); today it only dismisses the toast
- [ ] Crash reporting hook and structured logging

**Done when:** a sample app is packaged for three platforms from one command and updates itself from a signed manifest.

### DevTools and agents

Inspecting and driving a running app.

- [ ] Inspect's and LayoutJSON's `font` field shows the first name of the stack (`ui-sans-serif`), not the family that drew the text
- [ ] Live inspector: element tree, computed style, hooks and state, re-render counts, accessibility tree
- [ ] Frame profiler
- [ ] Connect to a running app and pick an element
- [ ] `tela mcp` on a running app's real windows, all of them (today headless only, and only the main window)
- [ ] `tela mcp`: refs keep node identity (today role, name and position); `scroll` without an element; a locator that matches nothing does not always wait 2 s; `notifications/cancelled` is honored
- [ ] `tela snapshot` shows a passive effect's state and toasts set by effects (it renders once)
- [ ] Debug-overlay labels are laid out at the display scale (today at scale 1)

**Done when:** DevTools shows why a given element re-rendered and where a frame spent its time, and `tela mcp` drives a running app's windows.

### Dev loop and testing

`tela dev`, `telatest`, `watch` and the test infrastructure.

- [ ] `tela dev` keeps what a swap loses today: kit components' aria hook state held in refs (silent resets), a renamed component's state, an IME composition
- [ ] `tela dev` with desktop services: a drop's final effect set inside `OnDrop` reaches the source; a refused or taken shortcut status is not cached until `tela dev` restarts; a window a new build opens only later (from an effect) does not close and reopen on reload; a dialog opened at startup does not show again on every reload; a reload while a Wayland window is minimized does not stall; the Windows 11 backdrop reaches the client area
- [ ] Dev builds can be debugged (DWARF)
- [ ] `telatest`: clicks with modifiers (Playwright's `click({modifiers})`; aria's Cmd+click tests mount through `rawwin` today); menu accelerators' `CmdOrCtrl` follow the host, not `PlatformKeys` (ADR-221)
- [ ] A public accessibility check in `telatest` (such as `App.ExpectAccessible()`, needs an ADR) from the axe-like rules of `telatest/a11y_audit_test.go`
- [ ] `telatest.TrayIcon` reports `Template` (ADR-237)
- [ ] `telatest`: plays Windows' `DefaultAsked` for file types and gives a start launch; a wait on a long timer of the app's clock advances to the next deadline when nothing else is due (today one frame per try, about 120 tries for 2 s, so it depends on the machine's speed)
- [ ] `watch`: find why `TestDirectoryMovedInAndOut/native` failed once on `windows-latest` (`c9aff8d`: nothing reported for a directory moved into the tree, no error; the watcher and the test now report more); the Windows poller holds each directory open while it lists it, so a rename of that directory fails for that moment
- [ ] CI: the win32 tests that read the screen (title bar colors, backdrop, frameless modal and glass pixels) or open real dialogs run only with `TELA_LIVE=1`, so CI checks their attributes but not their pixels; the win32 keyboard tests inject their "slow" keys 40 ms apart from a goroutine and check the focus only at the start, so they rely on running alone; a global-shortcut press injected with `keybd_event` or `SendInput` from a scheduled task never arrives, so only a real key press checks it
- [ ] `docs/llms.txt` is a few bytes under `llmsdoc`'s 100 KB limit: raise the limit or split the file; phase 20's proposed lines (transparent windows, built-in labels, `BackgroundGradientFn`, `Progress.ValueFn`, `ToggleGroup.Indicator`, Preview themes) and phase 22's (Linux backdrops, `i18n` number options and skeletons, access keys) wait for it
- [ ] `telatest`: no public way to read `aria-valuenow` (only `HasValue`; a `HasRange(now, min, max)` state); `Stats.Layouts` counts layout phases, including empty ones of restyle-only frames, so "no relayout" cannot be asserted there (count boxes laid out)
- [ ] CI: `cmd/tela`'s out-of-repository tests (dev, mcp, new, outside modules) skip when `go mod tidy` cannot resolve offline; the macOS job warms the module cache first, the Linux and Windows jobs do not, so there they likely skip; lint runs staticcheck for Linux only (`watch.(*Watcher).lost` is unused on darwin)
- [ ] `telatest`: after frames have run, a primary click at a point with no rendered content under it (the empty area of an `h-full` page) does not blur the focused element (found in phase 22; happens on `main` too)
- [ ] Tests that rasterize a `text.Layout` at x2 should lay it out at that scale (`Paragraph.Scale`): `TestChromiumClipText` laid out at 1 and drew at 2 (fixed in phase 23); check the others
- [ ] `cmd/tela`'s `TestOverlayCloseButtonAndClear` timed out once on `macos-latest` (2026-11-09, the panel's pixel (0,0) was `{1 0 0}` instead of `{0 0 0}` for 60 s) and passed on a re-run: find the race

**Done when:** each item has a test that failed before it, on Linux and Windows CI.

### Canvas, SVG and PDF

Vector output on the `DisplayList`.

- [ ] `Canvas` (2D): immediate-mode drawing surface (paths, gradients, text, images) with an animation-frame callback
- [ ] SVG: text, filters, images, patterns and markers
- [ ] Printing and PDF export: a vector PDF backend over the DisplayList (fonts embedded, gradients, clip, opacity), print dialogs, page-break CSS subset

**Done when:** a `Canvas` example animates at 60 fps, the new SVG features match their resvg test cases, and a page exports to a PDF that two viewers render identically.

### Audio and video

Highest uncertainty of the roadmap: pure-Go decoders limit the codecs.

- [ ] Audio playback and capture: pure-Go decoders (WAV, MP3, OGG/Vorbis, FLAC, Opus where one exists), output through ALSA/PulseAudio/PipeWire, WASAPI, CoreAudio
- [ ] Video playback: patent-free subset first (VP8/VP9/AV1 if a pure-Go decoder is viable), A/V sync, a `Video` component with `kit` controls; an ADR settles codec scope before work starts
- [ ] 3D (exploratory, ADR first; transforms stay 2D until then) lives here or in the backlog

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

## Known limitations (not blocking)

Small gaps that no phase is planned for (ADR-267). They block nothing; a phase that works in the area, or a real application that needs one, picks it up and moves it into its tasks. A limitation found while working goes here unless it is a bug of the work itself or belongs to a planned phase's subject.

- **Windows (ADR-261):** on Wayland and X11 the compositor runs the resize, so `platform.ResizeFloor` is not applied there and a card in a wide margin can be shrunk past its handles (`set_min_size` / `WM_NORMAL_HINTS` at the press); only portal content punches holes in hit regions (an `absolute z-10` sibling over a resize band still resizes)
- **Menus (ADR-260):** Windows' "always underline access keys" setting is not read; context and dropdown menus opened by a touch long press show underlines; a submenu wider than the whole window still overflows on one side (its width is not limited)
- **kit (ADR-263):** `Input` and `Alert` rebuild once after mount to apply their measured padding (one extra frame), and its constants live in Go, not in the recipe; a `Preview` below the root silently ignores its Theme and ColorScheme (no dev warning); the Chromium pages of the Tela-only pieces need the Tailwind CLI to be regenerated
- **i18n (ADR-266):** plural rules are still x/text's CLDR 32 (Icelandic and Macedonian pick other currency-name and compact forms than ICU for some numbers); compact percentages use the percent pattern (ICU uses the percent unit's); currency names and compact patterns cover the 113 generated locales only, and currencies no region uses today show their code; no rounding increments, cash digits, integer width, units, per mille or options after `scientific/` in skeletons; ICU 78.3's data differs from cldr-json 48 in a few places (fr-CH grouping, the new European English variants' currency formats, `my`'s currency-name pattern)
- **Desktop (ADR-262):** an app started hidden (a tray app) gets no `AtStart` for a Linux D-Bus–activated open; the stat poller misses a write that keeps size and modification time (stores and native watchers see it)

- **Text (ADR-269, ADR-270, ADR-273):** line-through is neither snapped nor given Chromium's thickness (the font's strikeout metrics, about a row thinner at 18px x2); the caret can stop inside a grapheme that a shaping-changing span splits (Chromium skips it); a hyphenation point inside a ligature is not used (Blink reshapes), a word across spans of different styles takes the dictionary of the style where the line overflows, and only hyphenated line ends are unkerned; the selection highlight does not cover a hyphen, and the caret at a hyphenated line's end sits before it; no German dictionary (about 120 KB, license to check)
- **Fonts and language (ADR-269):** the system font index counts a variable font at its default weight (go-text's index has no axes); the macOS system language is not read (CFLocale), only the environment, which a Finder launch lacks; a child window's root does not take the language of an `i18n.Provider` above its `Window`; a paragraph's installed CJK family prefers full coverage over the language's region (Japanese with a few Simplified-only characters draws wholly in a Chinese font); synthesis uses the @font-face rule for system fonts too (Blink on Linux: request > face weight + 200), and color fonts are never slanted
- **Selection (ADR-271):** a span's own `select-none` or `selection:` classes inside a selectable paragraph are ignored; images in a selection get no tint; no trailing space on a Windows double click (fields neither); no Linux PRIMARY selection, and copy is text/plain only; a Mac right click does not select the word; a selection end whose `VirtualList` row unmounts clears the selection; a static-selection drag cannot be cancelled by the app; text in a `relative` span with insets: its selection is painted under the span's background, and hit-testing ignores the vertical offset
- **Layout (ADR-268):** a scrolling item (textarea, `overflow-auto` box) inside a `display:grid` wrapper counts by its min-content (Chromium: its minimum contribution), so one unbreakable long word in a textarea still widens a `1fr` column; a ratio grid measuring its content sizes a `minmax(min-content, …)` row unconstrained; an auto grid column of an `aspect-*` item with `min-h-0` takes the width its content height transfers (Chromium: its content width); an inline's own `dir` is not isolated; a line holding only out-of-flow anchors after a forced break makes an extra line box; positioned spans are painted after their own paragraph, not interleaved with the stacking context's other positioned boxes; `top-[%]` on a span counts as 0 even when the block's height is definite
- **Drag and drop (ADR-272):** after an autoscroll step moves content under a resting drag, the drop target and a field's drop caret update only at the next platform drag event; an Input's drag band is horizontal only (Chromium's inner editor is always inside its vertical band); a step bubbling from an inner container to an outer one ignores transforms between them; `starting:` does not apply when an ancestor's `display: none` ends (ADR-080)
- **Floating:** size limits ignore the device-pixel snap of the position, so a menu limited to the room can end up to half a device pixel past the window at x1.5

## Live checks (not blocking)

Checks that need hardware or a desktop the project does not have at hand. They never block a phase. Each list runs in one session when the owner can arrange it, and the results go to HISTORY.md. Phases add their items here as they finish.

### macOS: one session on a real Mac (rented or borrowed)

This session gates v1.0's "macOS supported". Items a phase adds after the session wait for a later one.

- [ ] Every example at Retina scale; trackpad scrolling, momentum and gestures; Cmd shortcuts and word movement
- [ ] A Control-click on a one-button trackpad opens a kit context menu
- [ ] `looks/gallery` in its aqua designs, and `detect.Look()` choosing by `kern.osproductversion`
- [ ] Keyboard: the ISO swap of kVK_ISO_Section and the grave key, `charactersIgnoringModifiers` with Cmd+Shift, dead keys without the IME, the Caps Lock `flagsChanged:` pattern, the Globe/Fn key's keycode on new keyboards
- [ ] Japanese through the macOS IME, and dead keys with real key presses (CI's synthetic events start a dead-key composition but do not finish it)
- [ ] `examples/notes`: tray, notifications, file dialogs, two windows, a persisted position, a file dropped from Finder, the global shortcut in the background, a deep link into the running instance, a note locked in the Keychain, the Trash
- [ ] Phase 15 by hand (`tela run ./examples/notes`): notifications in the bundle (the authorization prompt, actions, a click), the traffic lights over notes' bar and their hover glyphs after they move, the vibrancy backdrops, a file dialog's "Format:" popup, Cmd+Q, the Dock's Quit and a logout with and without `OnQuit`'s dialog, a second start from Terminal taking the focus (macOS 14's activation yield), the global shortcut with a non-US layout, `open dist/Notes.app` and a `tela-notes:` link from Safari, and whether macOS accepts the development bundle's ad-hoc signature (the Go linker's, without a seal over `Info.plist`)
- [ ] The app bar's key equivalents: cocoa matches them itself when AppKit's matching takes none (CI's CGEvent-built keys); with real keys, check whether that fallback is still needed
- [ ] Phase 18: the system faces resolve (`system-ui` to SF through ".SF NS" or "System Font", `ui-serif` to New York, `ui-monospace` to SF Mono) and Apple Color Emoji's sbix glyphs sit where CoreText puts them (Tela follows HarfBuzz's reading of the origin offsets)
- [ ] Phase 16: a drag from Photos or Mail (file promises) and one carrying another app's private types reach an `OnDrop` target (CI cannot start another app's drag)
- [ ] Phase 19: VoiceOver reads, navigates and operates `login` (announcements and layout changes are posted on the NSWindow; check VoiceOver speaks them, and whether it offers AXScrollToVisible)
- [ ] Phase 20: a transparent window shows the desktop around its card, clicks on the margin and the shadow reach the window below and clicks on the card do not, no system shadow or rim, and a click right after the pointer moves onto a faint shadow pixel is not lost; native message box buttons and the app, Edit and Window menus in Spanish; a file or link that starts the bundled app reaches `UseLaunch` once as `LaunchOpened` after the start launch, a second process's as `LaunchForwarded`; notes' Cmd+Q asks about unsaved notes through `UseQuitRequest`; the store's 1 s polling sees `defaults`-style edits; `.note` registered as Viewer then None (Finder keeps name and icon, drops the app from Open With); a saved position off every screen opens inside `visibleFrame` (titled and frameless, a secondary screen left of or above the primary)
- [ ] Phase 21: `h.WindowResize` handles resize a transparent and a frameless window, with their cursors (the loop's 32 pt floor, the title bar's offset on a decorated window); a caption click closes an overlay and a real drag is no click; the global monitor hands the pointer back promptly; the masked vibrancy shows only behind the card; the kqueue watcher (`internal/watch`'s native tests, stores, `tela dev`); `Launch.AtStart` on a file that starts the bundled app; the role reported for a developer-written Info.plist entry
- [ ] Phase 22: the diagonal resize cursors (private ones on macOS 13/14, `frameResizeCursorFromPosition:inDirections:` on 15+, whose enum values come from the SDK header); a press on a rounded corner's outer pixel resizes and the alpha-1 pixels stay invisible; the resize floor with a wide shadow margin; a store edited in place past `ulimit -n`; `RENAME_SWAP` on APFS; the text-only drag image
- [ ] After the GPU backend: Metal draws the dashboard at 60 fps at scale 2
- [ ] After packaging: a signed and notarized `.app`/`.dmg` installs and updates

### Windows on real hardware

The VM has no GPU and runs at scale 1.

- [ ] Scales above 1 (per-monitor DPI), the UIA bridge's bounding rectangles included
- [ ] Narrator on `login` (NVDA is checked; Narrator keeps no speech log)
- [ ] Windows 11 rounded corners and Mica (the VM draws square corners and a solid backdrop); a frameless window with a client-area backdrop extends its DWM frame, which may hide the snap layouts flyout again
- [ ] The tray and notification icons' alpha, by eye
- [ ] Phase 16 by hand: the snap layouts flyout, hover and presses on an overlay window's DWM buttons, its caption buttons across monitors of different DPI, with a taskbar at the top and with a monitor above a maximized one (its 8 px hidden band), and a toast's buttons; `TitleBarOverlay` on Windows 10 22H2 (today frameless there)
- [ ] Phase 20: a transparent window (clicks through the margin and shadow, hover at the card's edge, a `WindowDrag` move, a DPI change, minimize from the taskbar; Windows 10 too); DWM caption glyphs of a `TitleBarOverlay` window follow an app theme that differs from the system's, after the first frame and a theme switch; a Japanese and a Russian menu bar ("ファイル(F)", "Файл" underlined, Alt+key with the IME on and off, the dark bar's suffix); translated native message box buttons (the CBT hook); a saved position on a disconnected monitor opens on the primary (mixed DPI, a top or left taskbar, maximized); notes shows no menu bar in its main and Preferences windows; a file type registered with None keeps its description and icon and leaves "Open with"; a store reloads when another program edits its file (ReadDirectoryChangesW)
- [ ] Phase 21 (also at 150%): a transparent window with `h.WindowResize` resizes from each side and corner (a small one, 100x30, too), `WS_THICKFRAME` adds no border, shadow or rounded corners and it maximizes sensibly; a caption click closes an outside-press overlay, a drag and Aero Snap do not; a press on a shadow pixel right after the content changed under a resting cursor reaches the window below; a message box with long translated buttons; `MenuItem.AccessKey` in a Japanese menu bar; the visible-frame fit (`DWMWA_EXTENDED_FRAME_BOUNDS` against the window rectangle after show: 7,0,7,7 at 100%)
- [ ] Phase 22 (also at 150%): the corner's outer pixel of a transparent card resizes and the margin passes through; a frameless window shrinks below 136×39 but no further than two corners; new handles without a repaint update the layered window; renamed message box buttons padded in dialog units; access keys and a lone Alt on a drawn bar with real keys
- [ ] Phase 23 (the VM is enough): `TELA_SYSTEM_FONTS=1 go test ./internal/text -run TestSystemFontsLive -v` (with `TELA_SYSTEM_FONTS_PNG`) draws the Japanese and Chinese paragraph in one family and nothing in Noto Sans JP Thin
- [ ] After the GPU backend: Vulkan (and D3D11 if built) on a real GPU

### Linux desktops

On the owner's Hyprland, a headless sway, or a GNOME or KDE session.

- [ ] X11 drag and drop, drags out of a Wayland window, and clipboard data (images, files, rich text) on both, with real apps (so far a fake compositor and a fake X server; a drop from Nautilus into a Wayland window is checked)
- [ ] The Trash (the portal, and the home trash)
- [ ] The Secret Service's unlock prompts through `examples/notes`
- [ ] X11 at scale 2 (`Xft.dpi`); X11 window positions and interactive moves on a real EWMH window manager
- [ ] Wayland maximize, the window menu, icons, `xdg-dialog` and session restore on a compositor that has them (sway lacks them)
- [ ] "Open with" dialogs list an app registered with `NoDisplay=true`
- [ ] The tray and notification icons' alpha, by eye
- [ ] Phase 20: X11 `RRGetMonitors` on a multi-monitor Xorg 1.18+, a removed monitor's saved position opening on the primary, `_NET_WORKAREA` panels avoided, the fallback on Xvfb; text dropped from another app into a field (notes imports it as a note, a plain Input inserts it)
- [ ] Phase 21: resizing a transparent window by hand on Hyprland (Wayland and Xwayland); a compositing manager that starts without MANAGER (an old picom) picked up live; X11 on mutter, KWin and xfwm4: `_NET_REQUEST_FRAME_EXTENTS` answered before map, the title bar below the panel, mutter's `_GTK_WORKAREAS_D<n>` with two monitors; text drags on every backend (a move reported back to the source, Ctrl/Option copies, within a window, across windows and apps)
- [ ] Phase 22: `ext-background-effect-v1` blur behind the card on KDE Plasma 6.5+ Wayland, `_KDE_NET_WM_BLUR_BEHIND_REGION` on KWin X11, nothing elsewhere; the handles' input region and the bare commit of `SetHitRegions` cause no flicker on Hyprland; a D-Bus–activated open (`gapplication launch`) arrives with `AtStart`; an X11 window manager that answers frame extents late delays only the first window; drag autoscroll and the drag image on Wayland and X11; a lone Alt after a compositor Alt+drag does not open the bar
- [ ] Phase 20: on Hyprland, clicks on a transparent window's margin and shadow reach the window below (only screenshots were checked)
- [ ] Phase 23: a real drag over a scroll container autoscrolls it on Wayland and X11, and static text selection with real presses (double and triple click timing, Ctrl+C into another app)
- [ ] Phase 16: the fallback title bar on GNOME on Wayland (look, `button-layout` from GNOME Tweaks, the top resize band beside it, double and right click)
- [ ] Phase 16: an X11 touchpad fling on native Xorg with xf86-input-libinput; the Wayland suspended state on GNOME or KWin (Hyprland sends none); a file dialog modal to its window and a link opened with focus, on Wayland

## By design or outside Tela

Not planned; the owner can reopen any of them.

- No margin collapsing, floats or tables (ARCHITECTURE §10); 3D transforms are out of scope (see Audio and video).
- Linux file dialogs need the XDG Desktop Portal (`xdg-desktop-portal` with a backend) and have no fallback dialog (the owner's choice, 2026-10-27).
- Phase 22 decisions: Linux and Windows associations have no Editor/Viewer/Shell roles and macOS cannot clear a user's default (ADR-262); on Windows, and on file systems without an atomic exchange (NFS, SMB, FAT), a store write in the instant between its check and the rename can still be lost (ADR-262); KDE's `org_kde_kwin_blur` is LGPL, so older KWin Wayland versions get no blur (ADR-261); a FixedSize Win32 transparent window has no thick frame, and macOS's own resize loop does no edge snapping (ADR-261); `looks/aqua`'s Default and Link buttons keep macOS's accent at 2.8–4.6:1 (ADR-264); a date range under RTL reads start-to-end from the right, as in Chromium (ADR-264).
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
- **Looks (`looks/`, ADR-197, ADR-223):** done for Windows 11, GNOME, KDE and macOS (`win11`, `adwaita`, `breeze`, `aqua` in two designs, `detect`, and a gallery)

## Deferred (not discarded)

Out of scope for now, but kept open: a decision to add the platform items needs an ADR amending the non-goals in ARCHITECTURE §1, and the platform layer must not preclude them.

- **WebAssembly / web:** a canvas-backed backend
- **Android / iOS:** touch, virtual keyboard, app lifecycle, safe areas
- **Touch input on the desktop** (ADR-265, ADR-267): no backend reports `PointerTouch` (Wayland's `wl_touch` is not bound, Win32 and X11 deliver touches as emulated mouse input), so there is no touch text selection (long press, handles) and no long-press drag of a selection with a touch `StartDrag` (Wayland `start_drag` with the touch-down serial)
- **Embedded web content:** a webview or HTML component (contradicts "no webview", ADR-001)
- **Context-click selection in grid, tree and tag collections** (ADR-247): when Tela has such collections with selection, a right or Control click on an unselected item selects it first, as list boxes do (owner, 2026-11-08).
- **No transitions on a theme or scheme change** (owner, 2026-10-31). `transition-colors` animates every color change, so `tela.SetColorScheme` or `tela.SetTheme` fade every element that has it, as browsers do: in `examples/dashboard` the board cards' hover fade turns a theme switch into ~150 ms of frames that rasterize the cards under the glass (p50 3.7 → 19 ms at x1, 5.6 → 66 ms at x2). Tela could apply a scheme or theme change without transitions (web libraries' `disableTransitionOnChange`), by default or as an option; it needs an ADR.
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
