# Tela Roadmap

> **Living** document: what is left and where we are, kept short (ADR-165).
> Design lives in [`ARCHITECTURE.md`](ARCHITECTURE.md) and decisions in
> [`adr/`](adr/README.md); finished phases, old status notes and the session
> log are in [`HISTORY.md`](HISTORY.md), versions in [`CHANGELOG.md`](../CHANGELOG.md).

## Current status

- **Done:** phases 1–12. Latest version `v0.12.2` (2026-10-28; a Textarea that does not wrap scrolls horizontally, ADR-188), tagged, not pushed; pushed up to `v0.12.1` (`main`, tags and the wiki from `docs/wiki` at `v0.12.1`).
- **Phase in progress:** none; phase 13 (desktop integration, the rest) is next.
- **Uncommitted:** a codebase-wide `/simplify` cleanup (2026-10-28, HISTORY.md) and the fixes of its `/code-review` (ADR-189), verified, for the owner to review and commit (a `v0.12.3` patch if released).
- **Next step:** the owner tries `examples/notes` with word wrap off in the GNOME container (`/home/ubuntu/notes-v3`) and pushes `main`, `v0.12.2` and the wiki; then plan phase 13 (drag and drop, native menus, global shortcuts, single instance and deep links, system theme, secure storage), whose done criterion extends `examples/notes`.
- **Machine and process notes:** live window checks run in a separate headless sway with a virtual pointer, not in the owner's Hyprland session (Hyprland 0.56 changes antialiased edges by ±1–2 levels when compositing; method in the session memory); no local Wine (it froze the host; CI keeps its Wine job); the owner runs `go test` once at the end of a batch of agent work, not during it; the race detector on `internal/core` and the arm64 run of `internal/gfx/raster` pass the 10-minute default timeout in one process, so they run sharded (build the test binary once, split `-test.run` across processes, one per `TestDamageMatchesFullRender` scene).

## Open known limitations

Still true as of `v0.12.0`; the per-phase lists in HISTORY.md include items fixed since.

- **Layout:** two `w-full aspect-square max-h-[20px]` flex items are clamped by the transferred max (Chromium 140×20, Tela 20×20); a ratio item with an auto or keyword width in a column of indefinite width is 5×5 where Chromium gives 30×30; grid items' percentage min sizes keep the old rule; no margin collapsing, floats or tables (by design, ARCHITECTURE §10); `grid-template-areas` is unreachable from classes.
- **Text:** at x1.5 a baseline on an exact half device pixel can be drawn one row higher; no hyphenation, skip-ink or synthetic bold/italic; CJK glyph forms ignore the language; `examples/text` and debug-overlay labels lay text out at scale 1; interned font faces are never evicted; no test covers a style run that starts inside a grapheme cluster (the itemizer then segments the run alone).
- **Paint and performance:** Chromium snaps border widths to device pixels at x1.5 and Tela does not; a frame that builds or restyles re-records every layer (one per hoisted box: 357 on the Calendar page); mounting the Calendar page takes 20–28 ms; no paint culling of rows hidden by a scroll container (large tables); the dashboard misses 16 ms even at x1 (p95 on the desktop, 1080p: 27 ms with 4 raster workers, 21 ms with 12; ~66 ms at x2; v0.9.2 was 24/20 ms); 3D transforms are out of scope.
- **Engine:** a passive effect's state never reaches `tela snapshot`, which renders once; an out-of-flow box with auto insets inside a paragraph sits below the paragraph (Chromium: at its place on the line), and an absolute child of a `relative` inline span is placed against the nearest positioned block; `ScrollOffset` read in the layout effect that scrolled returns the old offset (ADR-176).
- **Input and focus:** a box shown again that flips in the same frame does not restart its entrance; X11 has no IME and no touchpad fling.
- **API gaps:** no submenus; `tela snapshot` does not show toasts set by effects; no `h.AriaKeyShortcuts` or deferred accessible name (`h.AriaLabelFn`); `Disabled` missing on Tooltip, menu and Select triggers; `aria.UseContextMenuTrigger` exposes no opening number to key a menu per opening (proposed `ContextMenuTriggerResult.Opening`; kit counts openings itself); `kit.ContextMenu` areas are not focusable by default (Radix's model; the demo shows `TabIndex(0)`); SVG has no text, filters, images, patterns or markers; from the phase-10 kit migrations: `kit.Table` has no "never unsorted" sort mode and cannot hide the unsorted arrow; `kit.Field` has no slot by the description or rich labels, and a group inside a `Field` (not a `FieldSet`) is not named by its label; `examples/kit` still shows control-after with `flex-row-reverse`; no `kit.Alert`, Badge tone or dot, loading state on `kit.Button`, leading icon on `kit.Input`, caption on `kit.Separator`, slot beside a `Field`'s label, `animation-delay` utilities, Avatar group or color; Toggle, Slider and Checkbox cannot be a `Trigger` (no tooltips on them) and a trigger takes one overlay; `tela.Preview` takes no theme or color scheme; no `BackgroundGradientFn` and no deferred (`…Fn`) forms of Progress and ToggleGroup; no sort variants for `aria-sort` `other`/`none`; a context menu opened from the keyboard does not know the bounds of the focused element (it opens at the area's corner); `ar-EG` formats Latin digits with the vendored CLDR data (ICU: Arabic-Indic); the README screenshots of the examples migrated to kit are old.
- **Agent tools:** `tela mcp` runs headless only (no shared real window); its refs are role, name and position, not node identity; `scroll` needs an element; a locator that matches nothing waits 2 s; `notifications/cancelled` is ignored.
- **Dev loop:** kit components keep aria hook state in refs that `tela dev` cannot keep (silent resets); a renamed component resets; dev builds cannot be debugged (no DWARF); an IME composition is lost across a swap.
- **Desktop:** GNOME's AppIndicator extension applies a tray menu's changes only when the menu opens, so the old label shows for a moment (its design; GTK apps behave the same); a decorated window on a compositor without server-side decorations (GNOME on Wayland) has no title bar unless the app draws one when `State().ClientSideDecorations` says so (no automatic fallback bar yet; `examples/notes` draws `kit.TitleBar`); Linux file dialogs need the XDG Desktop Portal (`xdg-desktop-portal` with a backend) and have no fallback dialog (the owner's choice, 2026-10-27, after an Ubuntu XFCE container without it); Wayland file dialogs have no parent window (no `zxdg_exporter_v2` yet) and no activation tokens are passed to the portal; Wayland cannot place windows or keep them on top, and maximize, the window menu, icons, `xdg-dialog` and session restore are unverified live (sway lacks them; hide and show, notifications and the tray work on Hyprland 0.56, whose Omarchy window rules did not keep the restored size of `examples/notes`); a Wayland window shown again gets a new session restore token (ADR-186); work of a minimized Wayland window waits for the compositor's next frame callback (`xdg_toplevel.suspended` is not used yet); `App.OnClose` is outside the component tree (`examples/notes` bridges it with a package variable) and message boxes have fixed button labels; X11 window positions and interactive moves are unverified on a real EWMH window manager; Windows notifications have no actions (balloons; WinRT toasts later) and each one adds a notification-area icon while it lives; tray icons are not redrawn on DPI changes; the drawn message box's labels are English only (no i18n hook); a saved window position is not clamped to the monitors present at the next start; `desktop` stores do not see changes other programs make to their files; under `tela dev`, a window a new build opens only later (from an effect) closes and reopens on reload, a dialog opened at startup shows again on every reload, and a reload while a Wayland window is minimized can stall (as before); `tela mcp` sees only the main window; the tray and notification icons' alpha is not checked visually; `TestLiveIME` fails on the Windows VM at `v0.11.0` too (the VM's Japanese IME does not compose, an environment issue). Windows 11: rounded corners and Mica are unverified on real hardware (the VM has no GPU and draws square corners and a solid backdrop), and a frameless window with a client-area backdrop extends its DWM frame, which may hide the snap layouts flyout again (unverified); under `tela dev` the backdrop does not reach the client area. `cmd/tela`'s `TestOverlayCloseButtonAndClear` and `TestDevBuildFailure` each failed once under the load of the full suite (timing of the `tela dev` harness; both pass alone and with their package).
- **Platforms:** Windows verified on a VM at scale 1 only (`v0.10.4` test binaries and live gallery; native `go test ./...` at phases 11 and 12, the last on 2026-10-28, and the win32 live tests); X11 live only at scale 1 (no `Xft.dpi`); macOS is phase 14; `wtype` cannot type into Xwayland windows (Hyprland, not Tela).

## Phases

Each phase ends with verifiable criteria. A phase is not done unless its criteria pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references). Phases 1–12 are done (HISTORY.md).

### Phase 13: Desktop integration, the rest (`v0.13.0`)

- [ ] Drag and drop with the OS (files in, out, between windows); clipboard beyond text (images, files, rich text)
- [ ] Native application and context menus where the OS has them, with a drawn fallback
- [ ] Global shortcuts, single-instance apps, deep links and file associations
- [ ] System theme and accent color, high contrast, text scale, with change notifications
- [ ] Secure storage: Credential Manager, Secret Service (pure Go, no cgo)

**Done when:** the "notes" example accepts a dropped file, registers a global shortcut, reopens in its running instance from a deep link and keeps a token in secure storage, on Linux and Windows.

### Phase 14: macOS (and X11 IME) (`v0.14.0`)

- [ ] `cocoa` backend via purego, with CI on `macos-latest`: window, event loop, present, HiDPI, keyboard, pointer, clipboard, settings
- [ ] IME on macOS (`NSTextInputClient`) and on X11 (XIM)
- [ ] macOS parity for every phase-12 and phase-13 item

**Done when:** every example runs on macOS with snapshots pixel-identical up to text antialiasing, Japanese can be typed through the IME on macOS and on X11, and the phase-12/13 examples run on all three platforms.

### Phase 15: Accessibility (`v0.15.0`)

Strictly after phase 14: the bridges for AT-SPI, UI Automation and NSAccessibility are verified together (the owner chose strict order over starting the Linux and Windows bridges in parallel).

- [ ] AT-SPI over D-Bus (pure Go)
- [ ] UI Automation over COM
- [ ] NSAccessibility
- [ ] The semantics tree covers `aria` and `kit` roles, states, actions and live regions

**Done when:** a screen reader reads, navigates and operates the `signup` example on Linux, Windows and macOS.

### Phase 16: GPU backend (`v0.16.0`)

Independent of phases 14–15 and the hardest technical item (`tela-hard`).

- [ ] Vulkan (Linux, Windows) and Metal (macOS) via purego/syscall over the same DisplayList, with the fixed shader set of ARCHITECTURE §13
- [ ] D3D11 on Windows optional
- [ ] The CPU rasterizer stays the reference for goldens; GPU output is checked against it within a tolerance

**Done when:** the `dashboard` example renders at 60 fps at scale 2 on the GPU backend (the CPU one does not reach it) and GPU snapshots stay within the agreed tolerance of the CPU goldens.

### Phase 17: Packaging and auto-update (`v0.17.0`)

- [ ] `tela build` / `tela package`: installers and bundles (`.msi`/MSIX, `.app`/`.dmg`, AppImage, flatpak, `.deb`), icons, metadata, code-signing hooks
- [ ] Windows targets are GUI programs by default (`-H=windowsgui`, no console window next to the app; ADR-127), with panics and the app's log written to a file in its data directory, since a GUI program has no standard output
- [ ] Auto-update: signed manifests, delta or full downloads, atomic replace, rollback
- [ ] Crash reporting hook and structured logging

**Done when:** a sample app is packaged for three platforms from one command and updates itself from a signed manifest.

### Phase 18: DevTools (`v0.18.0`)

- [ ] Live inspector: element tree, computed style, hooks and state, re-render counts, accessibility tree
- [ ] Frame profiler
- [ ] Connect to a running app and pick an element

**Done when:** DevTools shows why a given element re-rendered and where a frame spent its time.

### Phase 19: Canvas and PDF (`v0.19.0`)

Both sit on the `DisplayList`, so they are cheap next to the rest.

- [ ] `Canvas` (2D): immediate-mode drawing surface (paths, gradients, text, images) with an animation-frame callback
- [ ] Printing and PDF export: a vector PDF backend over the DisplayList (fonts embedded, gradients, clip, opacity), print dialogs, page-break CSS subset

**Done when:** a `Canvas` example animates at 60 fps and a page exports to a PDF that two viewers render identically.

### Phase 20: Audio and video (`v0.20.0`)

Highest uncertainty of the roadmap: pure-Go decoders limit the codecs.

- [ ] Audio playback and capture: pure-Go decoders (WAV, MP3, OGG/Vorbis, FLAC, Opus where one exists), output through ALSA/PulseAudio/PipeWire, WASAPI, CoreAudio
- [ ] Video playback: patent-free subset first (VP8/VP9/AV1 if a pure-Go decoder is viable), A/V sync, a `Video` component with `kit` controls; an ADR settles codec scope before work starts
- [ ] 3D (exploratory, ADR first; transforms stay 2D until then) lives here or in the backlog

**Done when:** a "player" example plays an audio file and a video with synced controls.

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

## Deferred (not discarded)

Out of scope for now, but kept open: a decision to add the platform items needs an ADR amending the non-goals in ARCHITECTURE §1, and the platform layer must not preclude them.

- **WebAssembly / web:** a canvas-backed backend
- **Android / iOS:** touch, virtual keyboard, app lifecycle, safe areas
- **Embedded web content:** a webview or HTML component (contradicts "no webview", ADR-001)
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
