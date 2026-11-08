# Tela Roadmap

> **Living** document: what is left and where we are, kept short (ADR-165).
> Design lives in [`ARCHITECTURE.md`](ARCHITECTURE.md) and decisions in
> [`adr/`](adr/README.md); finished phases, old status notes and the session
> log are in [`HISTORY.md`](HISTORY.md), versions in [`CHANGELOG.md`](../CHANGELOG.md).
> Future phases have no number: a phase takes the next one when it starts, and
> every open limitation is a task of the phase that fixes it (ADR-218).

## Current status

- **Done:** phases 1–18. Latest version `v0.13.6` (2026-11-08, phase 18: font system). Versions follow ADR-208: a release that breaks the public API raises the minor version, any other the patch.
- **Phase in progress:** none. Phase 18 is on `phase-18-fonts`, to be merged into `main` locally.
- **Website:** `https://telaui.dev` is live with the landing page and the punched-card logo (ADR-206, ADR-207), and serves the docs of the latest release tag. Only tags deploy it: a release tag `v*`, or a site tag `site-YYYY-MM-DD` for a change to the site between releases, which the owner pushes (ADR-229).
- **Next step:** the next phase in order, Accessibility. R11–R13 of the 2026-10-31 complexity review are still not started.
- **Machine and process notes:** live window checks run in a separate headless sway with a virtual pointer, not in the owner's Hyprland session (Hyprland 0.56 changes antialiased edges by ±1–2 levels when compositing; method in the session memory); no local Wine (it froze the host; CI keeps its Wine job); the owner runs `go test` once at the end of a batch of agent work, not during it; the race detector on `internal/core` fits in one process on a quiet machine (about 6 minutes; shard it when the machine is loaded), the arm64 run of `internal/gfx/raster` passes the 10-minute default timeout in one process, so it runs sharded; CI runs the full `go test ./...` with `-timeout 20m` because `examples/dashboard` alone takes 7–9 minutes on a 4-vCPU runner (its damage checks play one scale each by default, `TELA_CHECK_DAMAGE=1` plays all, `TELA_PERF=1` adds the x2 sessions) (build the test binary once, split `-test.run` across processes, one per `TestDamageMatchesFullRender` scene). Under qemu, `internal/gfx/raster` takes about 9.5 minutes on a loaded machine: run the arm64 check with `-timeout 40m`. Subagents never push; the main session pushes the work branch when the owner allows it (ADR-220).

## Phases

In order. Each phase ends with verifiable criteria and is not done unless they pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references). What needs hardware or a desktop the project does not have goes to [Live checks](#live-checks-not-blocking) and does not block the phase. Phases 1–18 are done (HISTORY.md).

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
- [ ] Windows 11 overlay windows: DWM's caption glyphs follow the system scheme when the app's theme differs and it gives no `TitleBar.Background` (sample the bar's pixels); the access keys Win32 gives a native menu bar are Latin letters and digits only
- [ ] The fallback title bar (ADR-238): its button labels get the i18n hook; it follows a `Dir` set below the app's outermost element; a press on it counts as outside an open dialog (an outside-press dialog closes); decide whether an explicit opt-out is needed besides a `WindowDrag` area
- [ ] Transparent windows (asked by the owner, 2026-11-08): a window whose unpainted pixels show the desktop, so the root's first element floats with its own radius and `shadow-*` (an `App`/`Window` option such as `Transparent`, with `Frameless`). Needs an ADR. Wayland: premultiplied `ARGB8888` buffers and an empty opaque region (today `XRGB`/`XBGR`); X11: a 32-bit ARGB visual and colormap for the window (today the root visual; needs a compositing manager, else an opaque fallback); Windows: per-pixel alpha through DWM (`DwmExtendFrameIntoClientArea` with -1 margins and a premultiplied surface) or a layered window; macOS: a non-opaque `NSWindow` with `clearColor` and an alpha `CALayer`. Clicks on transparent pixels pass to what is below (the Wayland input region, X11 SHAPE input, `WM_NCHITTEST` returning `HTTRANSPARENT`, the view's `hitTest:`), kept in sync with what the frame painted; the window larger than the card so the shadow fits; damage and present keep alpha; headless and goldens render with alpha; a `telatest` option
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
- [ ] System CJK fallback on the Windows 11 VM: a Japanese and Chinese paragraph draws in two families, "Noto Sans JP Thin" (a static Thin family the user installed, taken at weight 400) and Microsoft YaHei; `TestSystemFontsLive` with `TELA_SYSTEM_FONTS=1` fails there, on `main` too. Rank a family's weight distance and keep one family per script
- [ ] A growing textarea (`field-sizing-content`) with long text can widen its grid column past the card unless the column has `min-w-0`; check against Chromium first (Chromium lets it shrink; found while merging signup into login)
- [ ] Text: selectable static text (`P`, `Span`; today only `Input` and `Textarea`); at x1.5 a baseline on an exact half device pixel can be drawn one row higher; hyphenation, skip-ink, synthetic bold and italic; CJK glyph forms follow the language; a test for a style run that starts inside a grapheme cluster (the itemizer then segments the run alone)
- [ ] Paint: border widths snap to device pixels at x1.5, as in Chromium
- [ ] Engine: `ScrollOffset` read in the layout effect that scrolled returns the new offset (ADR-176); a box shown again that flips in the same frame restarts its entrance
- [ ] Re-measure the scale-2 tolerances that about 12 Chromium comparison tests in `internal/core` and `internal/text` justify with ADR-048: ADR-159 superseded it (line boxes in device pixels, as Blink), so they may be looser than needed

**Done when:** every case above has a fixture or golden that matches Chromium up to text antialiasing, and static text can be selected and copied.

### CPU performance

The frame cost of the engine and the CPU rasterizer; the GPU backend builds on it. Baseline, the Frosted graphite dashboard (1080p, 12 raster workers, backdrop cache of ADR-200; quiet i5-10400, 2026-10-31): p50 4.2 ms, p95 17.8 ms at x1 over 726 frames; p50 9.0 ms, p95 40 ms at x2.

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
- [ ] `telatest.TrayIcon` reports `Template` (ADR-237)
- [ ] `telatest`: plays Windows' `DefaultAsked` for file types and gives a start launch; a wait on a long timer of the app's clock advances to the next deadline when nothing else is due (today one frame per try, about 120 tries for 2 s, so it depends on the machine's speed)
- [ ] `watch`: `TestDebounce/native` saw a batch 401 ms after the last write (before the debounce) once on the Linux runner (`7ae8f0e`, 2026-11-02), and both `native` and `poll` 290–340 ms after it under Wine (`047ca54`, 2026-11-08); find why `TestDirectoryMovedInAndOut/native` failed once on `windows-latest` (`c9aff8d`: nothing reported for a directory moved into the tree, no error; the watcher and the test now report more); the Windows poller holds each directory open while it lists it, so a rename of that directory fails for that moment
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
- [ ] The app bar's key equivalents: cocoa matches them itself when AppKit's matching takes none (CI's CGEvent-built keys); with real keys, check whether that fallback is still needed
- [ ] Phase 18: the system faces resolve (`system-ui` to SF through ".SF NS" or "System Font", `ui-serif` to New York, `ui-monospace` to SF Mono) and Apple Color Emoji's sbix glyphs sit where CoreText puts them (Tela follows HarfBuzz's reading of the origin offsets)
- [ ] Phase 16: a drag from Photos or Mail (file promises) and one carrying another app's private types reach an `OnDrop` target (CI cannot start another app's drag)
- [ ] After Accessibility: VoiceOver reads, navigates and operates `login`
- [ ] After the GPU backend: Metal draws the dashboard at 60 fps at scale 2
- [ ] After packaging: a signed and notarized `.app`/`.dmg` installs and updates

### Windows on real hardware

The VM has no GPU and runs at scale 1.

- [ ] Scales above 1 (per-monitor DPI)
- [ ] Windows 11 rounded corners and Mica (the VM draws square corners and a solid backdrop); a frameless window with a client-area backdrop extends its DWM frame, which may hide the snap layouts flyout again
- [ ] The tray and notification icons' alpha, by eye
- [ ] Phase 16 by hand: the snap layouts flyout, hover and presses on an overlay window's DWM buttons, its caption buttons across monitors of different DPI, with a taskbar at the top and with a monitor above a maximized one (its 8 px hidden band), and a toast's buttons; `TitleBarOverlay` on Windows 10 22H2 (today frameless there)
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
- [ ] Phase 16: the fallback title bar on GNOME on Wayland (look, `button-layout` from GNOME Tweaks, the top resize band beside it, double and right click)
- [ ] Phase 16: an X11 touchpad fling on native Xorg with xf86-input-libinput; the Wayland suspended state on GNOME or KWin (Hyprland sends none); a file dialog modal to its window and a link opened with focus, on Wayland

## By design or outside Tela

Not planned; the owner can reopen any of them.

- No margin collapsing, floats or tables (ARCHITECTURE §10); 3D transforms are out of scope (see Audio and video).
- Linux file dialogs need the XDG Desktop Portal (`xdg-desktop-portal` with a backend) and have no fallback dialog (the owner's choice, 2026-10-27).
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
