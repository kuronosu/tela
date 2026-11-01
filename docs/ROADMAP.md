# Tela Roadmap

> **Living** document: what is left and where we are, kept short (ADR-165).
> Design lives in [`ARCHITECTURE.md`](ARCHITECTURE.md) and decisions in
> [`adr/`](adr/README.md); finished phases, old status notes and the session
> log are in [`HISTORY.md`](HISTORY.md), versions in [`CHANGELOG.md`](../CHANGELOG.md).

## Current status

- **Done:** phases 1–12. Latest version `v0.13.0` (2026-10-31): the work between phases 12 and 13 (ADR-205, `looks`, the examples of ADR-195, the website), released so the website's docs and landing page match `go install …@latest`. ADR-208 now sets the version rule: releases follow the public API, so phase 13 is `v0.13.1` unless it breaks it. `main` and the tag are pushed; the GitHub wiki is `docs/wiki` at `v0.13.0` (`6bc2425`).
- **Phase in progress:** phase 13 (desktop integration, the rest). The owner approved its design (ADR-209: HTML drag and drop, native menus opt-in per menu or globally, rem text scale, locked notes, registration that never takes defaults silently). Steps as in phase 12: A, the platform contract v3 with the headless fake (done, ADR-210); B, the backends and core (in progress: seven agents in worktrees, Wayland, X11, Linux session services, Linux secrets and associations, Win32 window level, Win32 services, core with ADR-211); C, core, the public API, `telatest` and `tela dev`; D, `examples/notes` and the docs.
- **CI (2026-11-01):** every failure of the last 60 runs is explained (HISTORY.md, 2026-11-01). Three pushes fixed the races they showed: expected values computed before the state settled, effects observed before the event that records them, fixed wall-clock windows, screen reads under other windows. Then a read-only audit of every package's tests, by area, found the rest before the fourth push (the same patterns, Windows file locks, timeouts used as bounds on `tela dev`'s supervisor), all fixed and, where Linux allows, reproduced with injected delays; on `windows-latest` the win32 tests now run alone before the rest of the suite, and the Wine job has `-timeout 20m`.
- **Website:** `https://telaui.dev` is live with the landing page and the punched-card logo (ADR-206, ADR-207), and serves the `v0.13.0` docs. A release commit publishes its own docs before its tag (ADR-208).
- **Next step:** phase 13 (desktop integration, the rest: drag and drop, native menus, global shortcuts, single instance and deep links, system theme, secure storage), whose done criterion extends the redesigned `notes`. The examples are the nine of ADR-195 and the looks are the `looks/` module (ADR-197); its CI job passes since `v0.13.0`. The owner also still tries `examples/notes` with word wrap off in the GNOME container (`/home/ubuntu/notes-v3`). The complexity refactors R1–R10 (2026-10-31) are pushed; R11–R13 of that review are not started: split `resolveMotion` into collecting and building, the text fields' key bindings as one table per flavor (with phase 14's first item), and explicit process states for `tela dev`'s supervisor.
- **Machine and process notes:** live window checks run in a separate headless sway with a virtual pointer, not in the owner's Hyprland session (Hyprland 0.56 changes antialiased edges by ±1–2 levels when compositing; method in the session memory); no local Wine (it froze the host; CI keeps its Wine job); the owner runs `go test` once at the end of a batch of agent work, not during it; the race detector on `internal/core` fits in one process on a quiet machine (about 6 minutes; shard it when the machine is loaded), the arm64 run of `internal/gfx/raster` passes the 10-minute default timeout in one process, so it runs sharded; CI runs the full `go test ./...` with `-timeout 20m` because `examples/dashboard` alone takes 7–9 minutes on a 4-vCPU runner (its damage checks play one scale each by default, `TELA_CHECK_DAMAGE=1` plays all, `TELA_PERF=1` adds the x2 sessions) (build the test binary once, split `-test.run` across processes, one per `TestDamageMatchesFullRender` scene). Under qemu, `internal/gfx/raster` takes about 9.5 minutes on a loaded machine: run the arm64 check with `-timeout 40m`.

## Open known limitations

Still true as of `v0.12.0`; the per-phase lists in HISTORY.md include items fixed since.

- **Layout:** two `w-full aspect-square max-h-[20px]` flex items are clamped by the transferred max (Chromium 140×20, Tela 20×20); a ratio item with an auto or keyword width in a column of indefinite width is 5×5 where Chromium gives 30×30; grid items' percentage min sizes keep the old rule; no margin collapsing, floats or tables (by design, ARCHITECTURE §10); `grid-template-areas` is unreachable from classes.
- **Text:** at x1.5 a baseline on an exact half device pixel can be drawn one row higher; no hyphenation, skip-ink or synthetic bold/italic; CJK glyph forms ignore the language; debug-overlay labels lay text out at scale 1; static text (`P`, `Span`) cannot be selected, only `Input` and `Textarea` text; a growing textarea (`field-sizing-content`) with long text can widen its grid column past the card unless the column has `min-w-0` (Chromium lets it shrink; found while merging signup into login, unverified against Chromium); interned font faces are never evicted; no test covers a style run that starts inside a grapheme cluster (the itemizer then segments the run alone).
- **Paint and performance:** Chromium snaps border widths to device pixels at x1.5 and Tela does not; a paint records again only the layers whose content changed (ADR-204), but the root list (everything outside scroll content, deferred layers and hoisted chunks) is recorded whole when anything in it changes (a hover on the dashboard's sidebar: 346 commands), adding or removing a hoisted box records its stacking context's list, and a mounted `TextColorFn`, a box that starts or stops being a layer and layout-animation corrections still record everything; frames that build, restyle or lay out still place every box; mounting the Calendar page takes 20–28 ms; no paint culling of rows hidden by a scroll container (large tables); the Frosted graphite dashboard on the CPU (1080p, 12 raster workers, with the backdrop cache of ADR-200; quiet i5-10400, 2026-10-31): p50 4.2 ms, p95 17.8 ms at x1 over 726 frames (the session grew from 676 with the board cards' transitions, `2f5a037`), p50 9.0 ms, p95 40 ms at x2; a scroll that moves most of the window is still a full frame (the damage matcher gives up, ~28 ms at x1), and a theme switch or the palette opening are full frames (56 ms at x1); backdrop filters inside layers, or over content that changes every frame, are not cached; a frame that is not a pure scroll walks, lays out and re-records every element, including those a scroll container hides (a plain 2000-row `kit.Table` costs 9–15 ms per such frame; `kit.Table.RowHeight` makes it virtual, ADR-199); 3D transforms are out of scope.
- **Engine:** a passive effect's state never reaches `tela snapshot`, which renders once; an out-of-flow box with auto insets inside a paragraph sits below the paragraph (Chromium: at its place on the line), and an absolute child of a `relative` inline span is placed against the nearest positioned block; `ScrollOffset` read in the layout effect that scrolled returns the old offset (ADR-176).
- **Input and focus:** a box shown again that flips in the same frame does not restart its entrance; X11 has no IME and no touchpad fling.
- **API gaps:** no submenus; `tela snapshot` does not show toasts set by effects; no `h.AriaKeyShortcuts` or deferred accessible name (`h.AriaLabelFn`); `Disabled` missing on Tooltip, menu and Select triggers; `aria.UseContextMenuTrigger` exposes no opening number to key a menu per opening (proposed `ContextMenuTriggerResult.Opening`; kit counts openings itself); `kit.ContextMenu` areas are not focusable by default (Radix's model; the demo shows `TabIndex(0)`); SVG has no text, filters, images, patterns or markers; from the phase-10 kit migrations: `kit.Table` has no "never unsorted" sort mode and cannot hide the unsorted arrow; kit's toast viewport and close labels ("Notifications", "Close toast") have no field, so they stay English in the gallery's Spanish and Arabic; the gallery's English demo texts put their final period at the wrong end under RTL; `kit.Field` has no slot by the description or rich labels, and a group inside a `Field` (not a `FieldSet`) is not named by its label; `examples/kit` still shows control-after with `flex-row-reverse`; no `kit.Alert`, Badge tone or dot, loading state on `kit.Button`, leading icon on `kit.Input`, caption on `kit.Separator`, slot beside a `Field`'s label, `animation-delay` utilities, Avatar group or color; Toggle, Slider and Checkbox cannot be a `Trigger` (no tooltips on them) and a trigger takes one overlay; `tela.Preview` takes no theme or color scheme; no `BackgroundGradientFn` and no deferred (`…Fn`) forms of Progress and ToggleGroup; no sort variants for `aria-sort` `other`/`none`; a context menu opened from the keyboard does not know the bounds of the focused element (it opens at the area's corner); `ar-EG` formats Latin digits with the vendored CLDR data (ICU: Arabic-Indic); the README screenshots of the examples migrated to kit are old. No embedded serif font and no API for an app to ship its own fonts (`font-serif` draws Inter); left for later by the owner (2026-10-30) because of binary size.
- **Agent tools:** `tela mcp` runs headless only (no shared real window); its refs are role, name and position, not node identity; `scroll` needs an element; a locator that matches nothing waits 2 s; `notifications/cancelled` is ignored.
- **Dev loop:** kit components keep aria hook state in refs that `tela dev` cannot keep (silent resets); a renamed component resets; dev builds cannot be debugged (no DWARF); an IME composition is lost across a swap. `watch`'s `TestDirectoryMovedInAndOut/native` failed once on `windows-latest` (`c9aff8d`): nothing was reported for a directory moved into the tree, and no error either; the cause is unknown. The Windows watcher now reports the errors it dropped (a failed walk of a new directory, a failed `Lstat` of a new name) and the test counts overflowed batches, so a next failure says more. On Windows the poller (used only on request or when native watching fails) holds each directory open while it lists it, so a rename of that directory fails for that moment ("being used by another process"). `telatest` waits on a long timer of the app's clock one frame per try (about 120 tries for 2 s), so such a wait depends on the machine's speed; advancing to the next deadline when nothing else is due would make it constant. The win32 keyboard tests inject their "slow" keys 40 ms apart from a goroutine and check the focus only at the start: on CI they rely on running alone.
- **Desktop:** GNOME's AppIndicator extension applies a tray menu's changes only when the menu opens, so the old label shows for a moment (its design; GTK apps behave the same); a decorated window on a compositor without server-side decorations (GNOME on Wayland) has no title bar unless the app draws one when `State().ClientSideDecorations` says so (no automatic fallback bar yet; `examples/notes` draws `kit.TitleBar`); Linux file dialogs need the XDG Desktop Portal (`xdg-desktop-portal` with a backend) and have no fallback dialog (the owner's choice, 2026-10-27, after an Ubuntu XFCE container without it); Wayland file dialogs have no parent window (no `zxdg_exporter_v2` yet) and no activation tokens are passed to the portal; Wayland cannot place windows or keep them on top, and maximize, the window menu, icons, `xdg-dialog` and session restore are unverified live (sway lacks them; hide and show, notifications and the tray work on Hyprland 0.56, whose Omarchy window rules did not keep the restored size of `examples/notes`); a Wayland window shown again gets a new session restore token (ADR-186); work of a minimized Wayland window waits for the compositor's next frame callback (`xdg_toplevel.suspended` is not used yet); `App.OnClose` is outside the component tree (`examples/notes` bridges it with a package variable) and message boxes have fixed button labels; X11 window positions and interactive moves are unverified on a real EWMH window manager; Windows notifications have no actions (balloons; WinRT toasts later) and each one adds a notification-area icon while it lives; tray icons are not redrawn on DPI changes; the drawn message box's labels are English only (no i18n hook); a saved window position is not clamped to the monitors present at the next start; `desktop` stores do not see changes other programs make to their files; under `tela dev`, a window a new build opens only later (from an effect) closes and reopens on reload, a dialog opened at startup shows again on every reload, and a reload while a Wayland window is minimized can stall (as before); `tela mcp` sees only the main window; the tray and notification icons' alpha is not checked visually; `TestLiveIME` fails on the Windows VM at `v0.11.0` too (the VM's Japanese IME does not compose, an environment issue). Windows 11: rounded corners and Mica are unverified on real hardware (the VM has no GPU and draws square corners and a solid backdrop), and a frameless window with a client-area backdrop extends its DWM frame, which may hide the snap layouts flyout again (unverified); under `tela dev` the backdrop does not reach the client area.
- **Platforms:** the win32 tests that read the screen (title bar colors, backdrop, frameless modal and glass pixels) and those that open real dialogs run only with `TELA_LIVE=1`, so CI checks their attributes but not their pixels; Windows verified on a VM at scale 1 only (`v0.10.4` test binaries and live gallery; native `go test ./...` at phases 11 and 12, the last on 2026-10-28, and the win32 live tests); X11 live only at scale 1 (no `Xft.dpi`); macOS is phase 14; `wtype` cannot type into Xwayland windows (Hyprland, not Tela).

## Phases

Each phase ends with verifiable criteria. A phase is not done unless its criteria pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references). Phases 1–12 are done (HISTORY.md).

### Phase 13: Desktop integration, the rest (`v0.13.1`)

- [ ] Drag and drop with the OS (files in, out, between windows); clipboard beyond text (images, files, rich text)
- [ ] Native application and context menus where the OS has them, with a drawn fallback
- [ ] Global shortcuts, single-instance apps, deep links and file associations
- [ ] System theme and accent color, high contrast, text scale, with change notifications
- [ ] Secure storage: Credential Manager, Secret Service (pure Go, no cgo)

**Done when:** the "notes" example accepts a dropped file, registers a global shortcut, reopens in its running instance from a deep link and keeps a token in secure storage, on Linux and Windows.

### Phase 14: macOS (and X11 IME)

- [ ] First: one host-platform flavor (Linux, Windows, Mac) read by `core`'s key bindings and word rules, `aria`'s `ctrlKey` and `telatest`'s `ControlOrMeta`, overridable in tests so Mac behavior runs off-Mac (codebase review 2026-10-29, C11; a public "primary modifier" query needs an ADR)
- [ ] `cocoa` backend via purego, with CI on `macos-latest`: window, event loop, present, HiDPI, keyboard, pointer, clipboard, settings
- [ ] IME on macOS (`NSTextInputClient`) and on X11 (XIM)
- [ ] macOS parity for every phase-12 and phase-13 item

**Done when:** every example runs on macOS with snapshots pixel-identical up to text antialiasing, Japanese can be typed through the IME on macOS and on X11, and the phase-12/13 examples run on all three platforms.

### Phase 15: Accessibility

Strictly after phase 14: the bridges for AT-SPI, UI Automation and NSAccessibility are verified together (the owner chose strict order over starting the Linux and Windows bridges in parallel).

- [ ] AT-SPI over D-Bus (pure Go)
- [ ] UI Automation over COM
- [ ] NSAccessibility
- [ ] The semantics tree covers `aria` and `kit` roles, states, actions and live regions

**Done when:** a screen reader reads, navigates and operates the `login` example (with `signup` merged in, ADR-195) on Linux, Windows and macOS.

### Phase 16: GPU backend

Independent of phases 14–15 and the hardest technical item (`tela-hard`).

- [ ] Vulkan (Linux, Windows) and Metal (macOS) via purego/syscall over the same DisplayList, with the fixed shader set of ARCHITECTURE §13
- [ ] D3D11 on Windows optional
- [ ] The CPU rasterizer stays the reference for goldens; GPU output is checked against it within a tolerance

**Done when:** the `dashboard` example renders at 60 fps at scale 2 on the GPU backend (the CPU one does not reach it) and GPU snapshots stay within the agreed tolerance of the CPU goldens.

### Phase 17: Packaging and auto-update

- [ ] `tela build` / `tela package`: installers and bundles (`.msi`/MSIX, `.app`/`.dmg`, AppImage, flatpak, `.deb`), icons, metadata, code-signing hooks
- [ ] Windows targets are GUI programs by default (`-H=windowsgui`, no console window next to the app; ADR-127), with panics and the app's log written to a file in its data directory, since a GUI program has no standard output
- [ ] Auto-update: signed manifests, delta or full downloads, atomic replace, rollback
- [ ] Crash reporting hook and structured logging

**Done when:** a sample app is packaged for three platforms from one command and updates itself from a signed manifest.

### Phase 18: DevTools

- [ ] Live inspector: element tree, computed style, hooks and state, re-render counts, accessibility tree
- [ ] Frame profiler
- [ ] Connect to a running app and pick an element

**Done when:** DevTools shows why a given element re-rendered and where a frame spent its time.

### Phase 19: Canvas and PDF

Both sit on the `DisplayList`, so they are cheap next to the rest.

- [ ] `Canvas` (2D): immediate-mode drawing surface (paths, gradients, text, images) with an animation-frame callback
- [ ] Printing and PDF export: a vector PDF backend over the DisplayList (fonts embedded, gradients, clip, opacity), print dialogs, page-break CSS subset

**Done when:** a `Canvas` example animates at 60 fps and a page exports to a PDF that two viewers render identically.

### Phase 20: Audio and video

Highest uncertainty of the roadmap: pure-Go decoders limit the codecs.

- [ ] Audio playback and capture: pure-Go decoders (WAV, MP3, OGG/Vorbis, FLAC, Opus where one exists), output through ALSA/PulseAudio/PipeWire, WASAPI, CoreAudio
- [ ] Video playback: patent-free subset first (VP8/VP9/AV1 if a pure-Go decoder is viable), A/V sync, a `Video` component with `kit` controls; an ADR settles codec scope before work starts
- [ ] 3D (exploratory, ADR first; transforms stay 2D until then) lives here or in the backlog

**Done when:** a "player" example plays an audio file and a video with synced controls.

## Website (ADR-206)

- [x] `site/`: Astro + Starlight built from `docs/wiki` by `site/scripts/sync-wiki.mjs`, links and anchors checked at build; `.github/workflows/site.yml` checks pull requests and deploys `main`'s site with the latest tag's docs; live at `https://telaui.dev` since 2026-10-31 (DNS on Cloudflare, DNS only; Pages with HTTPS)
- [x] Landing page and logo (ADR-207, 2026-10-31): the owner chose prototype D and the Jacquard punched-card mark after rendered rounds; `site/snippets` re-rendered by CI (`render-snippets.sh --check`); `telaui.dev/llms.txt` served; the design history (prototypes, the 42-site survey, logo rounds) is in the owner's `~/Documents/tela-website/`, outside the repo
- [ ] Before the next release: the comparison table (`site/src/data/compare.json`) is dated 2026-10-31; re-check its cells against `compare-sources.md` when a compared project ships a major version (Wails v3.0.0 added an MCP server five days before)
- [ ] Decide when the GitHub wiki retires (until then both are published from `docs/wiki`); wikidoc's reference pages still say "start at the wiki home", and on the site the reference pages' contents list "Overview" twice (Starlight's page top and the package overview)

## Road to v1.0

`v1.0.0` is not scheduled; these gate it.

- Phase 7's breaking changes are done and their deprecations removed. From phase 10 (`kit` on the final API) on, **breaking changes need an ADR and a deprecation cycle**.
- The API freezes when phases 7–15 are done: stable `tela`, `h`, `aria`, `kit` and `telatest`; Linux (Wayland and X11), Windows and macOS supported; accessibility working on all three. The GPU backend (16) changes no public API and does not gate it.
- Godoc and wiki cover every exported symbol with a runnable example; `llms.txt` matches the API.
- At least two non-trivial applications outside the repository build against each release in CI.

## Separate packages

Optional, outside the core module. Until `tela add` exists (see "Deferred") they are plain Go modules that applications import. They are not scheduled; each starts when a real application needs it.

- **Charts:** line, bar, area, pie, scatter, axes, tooltips, built on `Canvas` (phase 19) or plain `Div`/SVG until then
- **DataGrid / TreeView:** sortable, resizable, editable cells, row selection, grouping, tree expansion, on top of `VirtualList`; the simple `Table` in `kit` stays in core
- **Looks (`looks/`, ADR-197):** done for Windows 11, GNOME and KDE (`win11`, `adwaita`, `breeze`, `detect`, and a gallery); phase 14 adds `looks/aqua` and its entry in `detect.For`

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
