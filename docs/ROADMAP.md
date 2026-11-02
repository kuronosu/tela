# Tela Roadmap

> **Living** document: what is left and where we are, kept short (ADR-165).
> Design lives in [`ARCHITECTURE.md`](ARCHITECTURE.md) and decisions in
> [`adr/`](adr/README.md); finished phases, old status notes and the session
> log are in [`HISTORY.md`](HISTORY.md), versions in [`CHANGELOG.md`](../CHANGELOG.md).
> Future phases have no number: a phase takes the next one when it starts, and
> every open limitation is a task of the phase that fixes it (ADR-218).

## Current status

- **Done:** phases 1–14. Latest version `v0.13.2` (2026-11-02, phase 14: the macOS backend; tagged on `0b9079d` with the owner's approval). Versions follow ADR-208: a release that breaks the public API raises the minor version, any other the patch.
- **Phase in progress:** 15, macOS desktop integration, on the work branch `phase-15-macos-desktop` (pushed by the main session for `macos-latest` CI, ADR-220). Design: ADR-224.
- **Website:** `https://telaui.dev` is live with the landing page and the punched-card logo (ADR-206, ADR-207), and serves the `v0.13.1` docs. A release commit publishes its own docs before its tag (ADR-208).
- **Next step:** close phase 15: the last `macos-latest` failure (`TestExamplesCocoa/notes`: the reference render lacks the traffic lights' rectangle), a full CI run on the branch, a review, then `v0.13.3` (no public API breaks). R11–R13 of the 2026-10-31 complexity review are still not started.
- **Machine and process notes:** live window checks run in a separate headless sway with a virtual pointer, not in the owner's Hyprland session (Hyprland 0.56 changes antialiased edges by ±1–2 levels when compositing; method in the session memory); no local Wine (it froze the host; CI keeps its Wine job); the owner runs `go test` once at the end of a batch of agent work, not during it; the race detector on `internal/core` fits in one process on a quiet machine (about 6 minutes; shard it when the machine is loaded), the arm64 run of `internal/gfx/raster` passes the 10-minute default timeout in one process, so it runs sharded; CI runs the full `go test ./...` with `-timeout 20m` because `examples/dashboard` alone takes 7–9 minutes on a 4-vCPU runner (its damage checks play one scale each by default, `TELA_CHECK_DAMAGE=1` plays all, `TELA_PERF=1` adds the x2 sessions) (build the test binary once, split `-test.run` across processes, one per `TestDamageMatchesFullRender` scene). Under qemu, `internal/gfx/raster` takes about 9.5 minutes on a loaded machine: run the arm64 check with `-timeout 40m`. Subagents never push; the main session pushes the work branch when the owner allows it (ADR-220).

## Phases

In order. Each phase ends with verifiable criteria and is not done unless they pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references). What needs hardware or a desktop the project does not have goes to [Live checks](#live-checks-not-blocking) and does not block the phase. Phases 1–14 are done (HISTORY.md).

### Phase 15: macOS desktop integration

Phases 12 and 13 on macOS. Started 2026-11-02 on the work branch `phase-15-macos-desktop`; the owner's design is ADR-224. Verified on `macos-latest` CI; the rest goes to the macOS live checks.

- [x] First: the design ADR (ADR-224: `tela build`/`tela run`, `TitleBarOverlay`, one `desktop.AppMenuBar`, `App.OnQuit`, windows not sheets)
- [x] Contract and public API: a quit request event and `tela.App.OnQuit`; `TitleBarOverlay` and the system buttons' rectangle in the window state, read by `kit.TitleBar`; the app menu bar with roles (`desktop.AppMenuBar`, `kit.Menubar` as the app's bar); headless fakes, telatest helpers, remote protocol
- [x] `tela build` and `tela run`: a development `.app` (Info.plist with the app's ID); packaging reuses it
- [x] Window control: minimize, zoom, fullscreen, always-on-top, position, min/max size, child and modal windows, hit regions of a frameless window, `TitleBarOverlay` with the traffic lights, a vibrancy backdrop
- [x] Services: open URI and path, file dialogs, message boxes (sheets with a parent), notifications (in a bundle), tray (`NSStatusItem`)
- [x] Native menus: the app's menu bar with the app, Edit and Window menus, the quit request, and context menus
- [x] Drag and drop, pasteboard formats
- [x] Global shortcuts (Carbon hot keys), single instance and Launch Services' launches, deep links and file types (Info.plist and Launch Services, through `assoc`), Keychain, Trash
- [x] `examples/notes` on macOS: `TitleBarOverlay`, an app menu bar, `OnQuit`; its link scheme is now `tela-notes:` (Apple's Notes owns `notes:`), and `TestNotesMacOS` builds it with `tela build`, registers it, and reaches the running instance with a link and with a second start

**Done when:** `examples/notes` meets the phase-12 and phase-13 criteria on `macos-latest` where CI can drive them; the rest is in the macOS live checks.

### Platform parity

Gaps in the existing backends. Verified on the owner's Linux machine, the Windows VM and `macos-latest` CI.

- [ ] IME on X11 (XIM)
- [ ] Touchpad fling on X11
- [ ] The X11 drag source shows a drag image and offers `XdndActionAsk`
- [ ] Wayland: file dialogs get their parent window (`zxdg_exporter_v2`) and activation tokens are passed to the portal; a minimized window's work stops waiting for the compositor's next frame callback (`xdg_toplevel.suspended`)
- [ ] A fallback title bar when the compositor has no server-side decorations (GNOME on Wayland); today the app draws one when `State().ClientSideDecorations` says so (`examples/notes` draws `kit.TitleBar`)
- [ ] Win32: the native menu bar opens with Alt and F10 and follows dark mode (it is mouse-only and stays light); drop reads get a timeout (a hung source blocks the UI thread)
- [ ] Windows notifications with actions (WinRT toasts; today balloons), without a notification-area icon per notification
- [ ] Tray icons are redrawn on DPI changes
- [ ] `TitleBarOverlay` keeps the OS's caption buttons on Windows too (DWM's buttons over the client area, Windows Controls Overlay); today it is `Frameless` there, and `kit.TitleBar`'s room for buttons at the end is untested
- [ ] macOS: drags that offer only another app's private types are not seen, file promises (Photos, Mail) are not received, and a drag without an image shows none (the file's icon would do); a tray icon can be a template image (a contract field); a message box without a parent is app-modal (`runModal`) and blocks the app's other windows; global hot keys are not registered again when the keyboard layout changes (Win32 neither)
- [ ] macOS: the app bar's key equivalents are also matched by the backend when AppKit's own matching takes none (CI's CGEvent-built keys); check with real keys whether the fallback is needed

**Done when:** Japanese can be typed through XIM on X11, and each item works on the owner's Linux machine, the Windows VM or `macos-latest`.

### Font system

How Tela ships and loads fonts. Before the API freeze, since it can add public API and change which fonts an app carries by default.

- [ ] Baseline: `examples/hello` is 25.6 MB (20.2 MB with `-s -w`), and 7.2 MB of it are the six embedded fonts (Noto Color Emoji alone 5 MB, Inter's two files 1.8 MB, Noto Sans Arabic 0.8 MB, JetBrains Mono 0.6 MB). Every app parses all six at start (about 70 ms of work, 40 ms of it Noto Color Emoji, whose outlines go-text parses up front), and keeps them in memory whether its text needs them or not
- [ ] An ADR on the options, keeping ADR-008's deterministic pixels for tests and snapshots: fewer or subset embedded fonts; fonts an app opts into (separate modules like `icons/lucide`, or build tags); compressed embedding (the standard library's `compress/flate`; WOFF2 needs a Brotli decoder, a new dependency); system fonts by default with the embedded ones for tests (go-text's `fontscan` already indexes them, opt-in and internal today)
- [ ] Parse a font when text first needs it, not at start
- [ ] An API for an app to ship its own fonts, and a serif face for `font-serif` (today it draws Inter; deferred by the owner on 2026-10-30 because of binary size)

**Done when:** the ADR's choice is in, an app can ship its own fonts, goldens stay bit-identical wherever the default fonts did not change, and `examples/hello`'s binary size, start-up parse time and font memory are measured before and after.

### Accessibility

Strictly after the two macOS phases: the bridges for AT-SPI, UI Automation and NSAccessibility are verified together (the owner chose strict order over starting the Linux and Windows bridges in parallel).

- [ ] AT-SPI over D-Bus (pure Go)
- [ ] UI Automation over COM
- [ ] NSAccessibility
- [ ] The semantics tree covers `aria` and `kit` roles, states, actions and live regions
- [ ] `h.AriaKeyShortcuts` and a deferred accessible name (`h.AriaLabelFn`)

**Done when:** a screen reader reads, navigates and operates the `login` example (with `signup` merged in, ADR-195) on Linux and Windows; VoiceOver on macOS is in the macOS live checks.

### API and kit gaps

What building the examples showed missing in `tela`, `h`, `aria`, `kit` and `desktop`.

- [ ] aria: on a Mac a Control+click on a listbox option opens the context menu and does not select it (ADR-221: Tela's collections select on click, React Aria's at press); decide whether the press should select first
- [ ] Menus and triggers: submenus; `kit.MenuItem` has no `Role` (an Edit menu that mixes the app's items with standard ones needs `desktop.AppMenuBar`); `Disabled` on Tooltip, menu and Select triggers; Toggle, Slider and Checkbox as `Trigger`s (so they can have tooltips), and more than one overlay per trigger; `aria.UseContextMenuTrigger` exposes an opening number to key a menu per opening (proposed `ContextMenuTriggerResult.Opening`; kit counts openings itself); a context menu opened from the keyboard knows the focused element's bounds (it opens at the area's corner); decide whether `kit.ContextMenu` areas are focusable by default (Radix's model says no; the demo shows `TabIndex(0)`)
- [ ] `kit.ToggleStyle.Icon` is one class for every Toggle variant, so `looks/aqua` cannot tint the sidebar's glyphs with the accent as macOS does; small `looks/aqua` fidelity gaps: Sequoia's pop-up button chevron sits in an accent tile (`kit.Select` with `Icon: ChevronsUpDown` matches macOS) and its toolbar search field is bordered, not borderless gray; in Tahoe dark, Destructive text over a saturated backdrop has low contrast
- [ ] New kit pieces: `kit.Alert`, Badge tone and dot, a loading state on `kit.Button`, a leading icon on `kit.Input`, a caption on `kit.Separator`, Avatar group and color, `animation-delay` utilities
- [ ] `kit.Field`: a slot by the description, rich labels, a slot beside the label, and a group inside a `Field` (not a `FieldSet`) named by its label
- [ ] `kit.Table`: a "never unsorted" sort mode and a way to hide the unsorted arrow; sort variants for `aria-sort` `other`/`none`
- [ ] Deferred forms: `BackgroundGradientFn`, and `…Fn` forms of Progress and ToggleGroup; `tela.Preview` takes a theme and a color scheme
- [ ] i18n: fields for kit's toast viewport and close labels ("Notifications", "Close toast" stay English in the gallery's Spanish and Arabic); message boxes' button labels, native and drawn, get an i18n hook (and the labels Tela gives macOS's app, Edit and Window menus and the save panel's "Format:" popup); `ar-EG` formats Arabic-Indic digits as ICU does (the vendored CLDR data gives Latin)
- [ ] App and desktop: `App.OnClose` inside the component tree (`examples/notes` bridges it with a package variable); text fields accept text drops by default, as HTML's do; a saved window position is clamped to the monitors present at the next start; `desktop` stores see changes other programs make to their files; a `LaunchEvent` does not say whether it came from another process or from Launch Services (a link or file opened with the app, also at its start), so `desktop.UseLaunch` reports those `Forwarded`; app-scoped events that arrive before the first frame's commit have no listeners yet on any backend (cocoa holds launches until then); `desktop.AppMenuBar` has no way to apply only where the app's bar is native (on Windows it becomes every framed window's bar, so `notes` declares it only under the macOS look); `platform.Association` has no role, so file types register as Editor
- [ ] Examples: `notes`' welcome note and its Preferences shortcut list say Ctrl on a Mac and lack Cmd+W and Cmd+1–3; the gallery's English demo texts end their sentences at the right side under RTL; `examples/kit` shows control-after without `flex-row-reverse`; new README screenshots of the examples migrated to kit

**Done when:** each item has its API, an ADR where it adds public API, and a test or golden, and the kit gallery shows the new pieces in its three locales.

### Layout, text and engine fidelity

Differences from Chromium and missing typography; each fix comes with a fixture or golden.

- [ ] Layout: two `w-full aspect-square max-h-[20px]` flex items are clamped by the transferred max (Chromium 140×20, Tela 20×20); a ratio item with an auto or keyword width in a column of indefinite width is 5×5 where Chromium gives 30×30; grid items' percentage min sizes keep the old rule; `grid-template-areas` is unreachable from classes; an out-of-flow box with auto insets inside a paragraph sits below the paragraph (Chromium: at its place on the line); an absolute child of a `relative` inline span is placed against the nearest positioned block
- [ ] A growing textarea (`field-sizing-content`) with long text can widen its grid column past the card unless the column has `min-w-0`; check against Chromium first (Chromium lets it shrink; found while merging signup into login)
- [ ] Text: selectable static text (`P`, `Span`; today only `Input` and `Textarea`); at x1.5 a baseline on an exact half device pixel can be drawn one row higher; hyphenation, skip-ink, synthetic bold and italic; CJK glyph forms follow the language; a test for a style run that starts inside a grapheme cluster (the itemizer then segments the run alone)
- [ ] Paint: border widths snap to device pixels at x1.5, as in Chromium
- [ ] Engine: `ScrollOffset` read in the layout effect that scrolled returns the new offset (ADR-176); a box shown again that flips in the same frame restarts its entrance
- [ ] Re-measure the scale-2 tolerances that about 12 Chromium comparison tests in `internal/core` and `internal/text` justify with ADR-048: ADR-159 superseded it (line boxes in device pixels, as Blink), so they may be looser than needed

**Done when:** every case above has a fixture or golden that matches Chromium up to text antialiasing, and static text can be selected and copied.

### Memory

Find where RAM goes and reduce it. Before CPU performance, whose caches trade memory for speed.

- [ ] A repeatable measurement: Go heap profiles (pprof `inuse_space`, `alloc_space`) and the process's RSS for `examples/hello`, the kit gallery, `dashboard` and `notes`, at x1 and x2, at rest and through the dashboard's scripted session, run by one command so regressions show
- [ ] Check the suspects the profile ranks (none measured yet): the parsed fonts (see Font system), interned font faces that are never evicted, framebuffers and the copies made around `Present` (one RGBA buffer of a 1080p window at x2 is about 33 MB), the raster workers' buffers, the backdrop cache (ADR-200), the layers' display lists, glyph caches
- [ ] Reduce the largest consumers, each change measured and the goldens unchanged

**Done when:** the measurement runs from one command with a recorded baseline, and the largest consumers it found are reduced or explained, with the before and after numbers for each example.

### CPU performance

The frame cost of the engine and the CPU rasterizer; the GPU backend builds on it. Baseline, the Frosted graphite dashboard (1080p, 12 raster workers, backdrop cache of ADR-200; quiet i5-10400, 2026-10-31): p50 4.2 ms, p95 17.8 ms at x1 over 726 frames; p50 9.0 ms, p95 40 ms at x2.

- [ ] Recording: a paint records again only the layers whose content changed (ADR-204), but the root list (everything outside scroll content, deferred layers and hoisted chunks) is recorded whole when anything in it changes (a hover on the dashboard's sidebar: 346 commands); adding or removing a hoisted box records its stacking context's list; a mounted `TextColorFn`, a box that starts or stops being a layer and layout-animation corrections still record everything
- [ ] Layout and walks: frames that build, restyle or lay out still place every box; a frame that is not a pure scroll walks, lays out and re-records every element, including those a scroll container hides (a plain 2000-row `kit.Table`: 9–15 ms per such frame; `kit.Table.RowHeight` makes it virtual, ADR-199); no paint culling of rows a scroll container hides; mounting the Calendar page takes 20–28 ms
- [ ] Damage: a scroll that moves most of the window is a full frame (the damage matcher gives up, ~28 ms at x1); a theme switch and the palette opening are full frames (56 ms at x1)
- [ ] Caches: backdrop filters inside layers, or over content that changes every frame, are not cached
- [ ] macOS: every `Present` copies the whole retained frame into a new `CFData` and `CGImage`, whatever the damage (about 20 MB per frame for a 2880x1800 Retina window); an `IOSurface`-backed layer would copy only the damage
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
- [ ] Crash reporting hook and structured logging

**Done when:** a sample app is packaged for three platforms from one command and updates itself from a signed manifest.

### DevTools and agents

Inspecting and driving a running app.

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
- [ ] `telatest`: plays Windows' `DefaultAsked` for file types and gives a start launch; a wait on a long timer of the app's clock advances to the next deadline when nothing else is due (today one frame per try, about 120 tries for 2 s, so it depends on the machine's speed)
- [ ] `watch`: find why `TestDirectoryMovedInAndOut/native` failed once on `windows-latest` (`c9aff8d`: nothing reported for a directory moved into the tree, no error; the watcher and the test now report more); the Windows poller holds each directory open while it lists it, so a rename of that directory fails for that moment
- [ ] CI: the win32 tests that read the screen (title bar colors, backdrop, frameless modal and glass pixels) or open real dialogs run only with `TELA_LIVE=1`, so CI checks their attributes but not their pixels; the win32 keyboard tests inject their "slow" keys 40 ms apart from a goroutine and check the focus only at the start, so they rely on running alone; a global-shortcut press injected with `keybd_event` or `SendInput` from a scheduled task never arrives, so only a real key press checks it
- [ ] `docs/llms.txt` is near `llmsdoc`'s 100 KB limit
- [ ] CI: `cmd/tela`'s out-of-repository tests (dev, mcp, new, outside modules) skip when `go mod tidy` cannot resolve offline; the macOS job warms the module cache first, the Linux and Windows jobs do not, so there they likely skip; lint runs staticcheck for Linux only (`watch.(*Watcher).lost` is unused on darwin)
- [ ] `tela dev` on macOS polls the tree every 300 ms; an FSEvents watcher

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
- [ ] After Accessibility: VoiceOver reads, navigates and operates `login`
- [ ] After the GPU backend: Metal draws the dashboard at 60 fps at scale 2
- [ ] After packaging: a signed and notarized `.app`/`.dmg` installs and updates

### Windows on real hardware

The VM has no GPU and runs at scale 1.

- [ ] Scales above 1 (per-monitor DPI)
- [ ] Windows 11 rounded corners and Mica (the VM draws square corners and a solid backdrop); a frameless window with a client-area backdrop extends its DWM frame, which may hide the snap layouts flyout again
- [ ] The tray and notification icons' alpha, by eye
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

## By design or outside Tela

Not planned; the owner can reopen any of them.

- No margin collapsing, floats or tables (ARCHITECTURE §10); 3D transforms are out of scope (see Audio and video).
- Linux file dialogs need the XDG Desktop Portal (`xdg-desktop-portal` with a backend) and have no fallback dialog (the owner's choice, 2026-10-27).
- Wayland cannot place windows or keep them on top; a Wayland window shown again gets a new session restore token (ADR-186).
- GNOME's AppIndicator extension applies a tray menu's changes only when the menu opens, so the old label shows for a moment (its design; GTK apps behave the same).
- xdg-desktop-portal-hyprland assigns no keys to global shortcuts: the user binds `appID:id` in the compositor's config.
- Environment: Omarchy's window rules on Hyprland 0.56 did not keep the restored size of `examples/notes`; `wtype` cannot type into Xwayland windows (Hyprland); the Windows VM's Japanese IME does not compose, so `TestLiveIME` fails there.

## Website (ADR-206)

- [x] `site/`: Astro + Starlight built from `docs/wiki` by `site/scripts/sync-wiki.mjs`, links and anchors checked at build; `.github/workflows/site.yml` checks pull requests and deploys `main`'s site with the latest tag's docs; live at `https://telaui.dev` since 2026-10-31 (DNS on Cloudflare, DNS only; Pages with HTTPS)
- [x] Landing page and logo (ADR-207, 2026-10-31): the owner chose prototype D and the Jacquard punched-card mark after rendered rounds; `site/snippets` re-rendered by CI (`render-snippets.sh --check`); `telaui.dev/llms.txt` served; the design history (prototypes, the 42-site survey, logo rounds) is in the owner's `~/Documents/tela-website/`, outside the repo
- [ ] Before the next release: the comparison table (`site/src/data/compare.json`) is dated 2026-10-31; re-check its cells against `compare-sources.md` when a compared project ships a major version (Wails v3.0.0 added an MCP server five days before)
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
- **Embedded web content:** a webview or HTML component (contradicts "no webview", ADR-001)
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
