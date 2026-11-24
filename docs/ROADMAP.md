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

- **Done:** phases 1–30. Latest version `v0.14.11` (2026-11-24, phase 30: External textures, ADR-315 to ADR-318). Versions follow ADR-208: a release that breaks the public API raises the minor version, any other the patch; the owner may release a small break, such as a rename, as a patch.
- **Phase in progress:** 31, Canvas (branch `phase-31`; ADR-319). Next in order: SVG and PDF; Audio and video; Game input.
- **Website:** `https://telaui.dev` is live with the landing page and the punched-card logo (ADR-206, ADR-207), and serves the docs of the latest release tag. Only tags deploy it: a release tag `v*`, or a site tag `site-YYYY-MM-DD` for a change to the site between releases, which the owner pushes (ADR-229).
- **Next step:** the owner pushes `main` and a site tag `site-YYYY-MM-DD` on its head, so that the site gets the docs of `v0.14.11` (a broken anchor stopped its deploys from `v0.14.9` on). Phase 31's first tasks: the Porter-Duff operations and the Canvas element. Phase 28's DevTools window and `tela mcp --attach` wait for a live check on a real desktop (`docs/LIMITATIONS.md`). Phase 27's Windows and macOS surfaces wait for their live checks (`docs/LIMITATIONS.md`). Phase 26's Mac checks wait for a macOS CI run or a Mac (`docs/LIMITATIONS.md`). The GPU backend's Windows code ran without windows on an Intel Arc laptop (2026-11-23); its windowed live check waits. Phase 30's Windows windowed tests run on the owner's Intel Arc laptop next (Smart App Control blocks some test programs). R11–R13 of the 2026-10-31 complexity review are still not started.
- **Machine and process notes:** live window checks run in a separate headless sway with a virtual pointer, not in the owner's Hyprland session (Hyprland 0.56 changes antialiased edges by ±1–2 levels when compositing; method in the session memory); no Wine on the host (it froze it once): Wine runs in a Docker container limited to 4 CPUs and 8 GB (Ubuntu 24.04 with `wine` and `wine64`, `LANG=C.UTF-8`, the host's Go mounted read-only), as CI's job runs it; the owner runs `go test` once at the end of a batch of agent work, not during it; the race detector on `internal/core` takes about 15 minutes in one process (2026-11-09; run it with `-timeout 30m`, or shard it; longer since phase 24's `TELA_CHECK_SEMANTICS` and `TELA_CHECK_LAYOUT` are on in core's tests), the arm64 run of `internal/gfx/raster` passes the 10-minute default timeout in one process, so it runs sharded; CI runs the full `go test ./...` with `-timeout 20m` because `examples/dashboard` alone takes 7–9 minutes on a 4-vCPU runner (its damage checks play one scale each by default, `TELA_CHECK_DAMAGE=1` plays all, `TELA_PERF=1` adds the x2 sessions) (build the test binary once, split `-test.run` across processes, one per `TestDamageMatchesFullRender` scene). Under qemu, `internal/gfx/raster` takes about 9.5 minutes on a loaded machine: run the arm64 check with `-timeout 40m`. Subagents never push; the main session pushes the work branch when the owner allows it (ADR-220).

## Phases

In order. Each phase ends with verifiable criteria and is not done unless they pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references). What needs hardware or a desktop the project does not have goes to [Live checks](#live-checks-not-blocking) and does not block the phase. Phases 1–30 are done (HISTORY.md).

### Phase 31: Canvas

An immediate-mode 2D drawing surface with the lifecycle of a component (ADR-319).

- [x] `gfx` and the CPU rasterizer: the Porter-Duff operations as layer blends, and a tiled image paint
- [x] The same on the GPU backend, compared with the CPU rasterizer
- [x] `core`: the `Canvas` element, its draw in the paint phase (tracked reads, recording, damage), `RequestFrame` and its clock, dev checks; `telatest` drives it
- [x] The drawing API: paths and `Path2D`, transforms, styles, text, images, shadows, clips, compositing
- [x] `fill`, `stroke` and `stroke-width` transition (added 2026-11-24: a canvas starts with them)
- [ ] Compared with Chromium's output
- [ ] An example, the wiki, `llms.txt` with a recipe, and `tela lint`'s checks

**Done when:** the example animates at 60 fps at scale 2 on Linux with either renderer and is idle at rest, an equal redraw damages nothing, and the scenes compared with Chromium pass.

### SVG and PDF

Vector output on the `DisplayList`.

- [ ] SVG: text, filters, images, patterns and markers
- [ ] Printing and PDF export: a vector PDF backend over the DisplayList (fonts embedded, gradients, clip, opacity), print dialogs, page-break CSS subset

**Done when:** the new SVG features match their resvg test cases, and a page exports to a PDF that two viewers render identically.

### Audio and video

Highest uncertainty of the roadmap: pure-Go decoders limit the codecs.

- [ ] Audio playback and capture: pure-Go decoders (WAV, MP3, OGG/Vorbis, FLAC, Opus where one exists), output through ALSA/PulseAudio/PipeWire, WASAPI, CoreAudio
- [ ] A low-latency output stream the application fills from its own callback (a mixer, a game engine, a synthesizer), besides file playback; the ADR states the latency each backend reaches (added 2026-11-23 for applications that render their own content)
- [ ] Video playback: patent-free subset first (VP8/VP9/AV1 if a pure-Go decoder is viable), A/V sync, a `Video` component with `kit` controls; an ADR settles codec scope before work starts. Pure-Go decoders produce CPU frames, so `Video` shows them through External textures' element and its copy path; the GPU imports of that phase serve frames a hardware decoder leaves on the GPU. The Windows shared-handle import (it needs a keyed mutex or a fence, and the Intel Arc laptop to run on) and X11's fixed 60 Hz pacing, which could follow `FrameEvent.Refresh` (ADR-317), are here. Textures take 8-bit RGB only (ADR-315): NV12 and other YUV frames (BT.601/709, limited or full range), converted in the shader and in the copy, are this phase's
- [ ] Dmabuf imports on an Intel Arc (Meteor Lake, Linux 7.2, Mesa 26.2.5): `TestImportStrides` (`internal/gfx/gpu`), `TestVulkanDrawsTheScene` and `TestPainterOnTheWindow` (`examples/texture`) read wrong pixels, so the gate fails on that machine, and so do `internal/core`'s five `TestTextureGPU*` with `telagpu` (found 2026-11-24, cause not looked for). There, `TestPlain` (`internal/gfx/gpu`) also fails now and then under GPU contention, in blur scenes: 2 of 445 runs before phase 31, 4 of 317 after its GPU work, cause unknown
- [ ] 3D (exploratory, ADR first; transforms stay 2D until then) lives here or in the backlog; an application can already show its own 3D rendering through External textures

**Done when:** a "player" example plays an audio file and a video with synced controls.

### Game input

What an application that draws its own frames (External textures) needs from the window and the input devices, found on 2026-11-23 when asking whether an application could build a game engine on Tela. Today a window can go fullscreen; there is no pointer lock or relative motion and no gamepad. Opened at the owner's request (ADR-267).

- [ ] First: an ADR (a platform contract version) on pointer lock, relative motion and gamepads: who asks (an element, as a texture view does), what the OS grants, how the application learns it lost the lock
- [ ] Pointer lock and relative motion for a focused element: the pointer hidden and held in place, unscaled motion deltas; Wayland `zwp_pointer_constraints_v1` and `zwp_relative_pointer_v1`, X11 XInput2 raw motion with a grab, Win32 raw input with `ClipCursor`, Cocoa dissociating the cursor; released when the window loses focus, with a way out that the application cannot disable
- [ ] Gamepads, pure Go: evdev on Linux (and its permissions), Windows' XInput or HID, macOS' GameController through purego; buttons, axes, triggers, hot-plug and rumble, as events, a polling call or both (the ADR picks)
- [ ] Keys for a game view: a focused element that is not a text field gets physical keys, with auto-repeat told apart and no text composition; check what it gets today in each backend
- [ ] `telatest`: fake pointer lock and a fake gamepad, so a game view has headless tests
- [ ] An example: a camera over the External textures scene, moved with the locked mouse and a gamepad

**Done when:** the example turns its camera with the locked mouse and the gamepad on Linux and gives the pointer back when the window loses focus; Windows and macOS go to the live checks.

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
- [ ] `tela verify` findings (2026-11-22): kit's theme misses AA contrast for muted text on muted (Avatar fallback 4.34:1), destructive text on its tint (Alert, 4.26–4.36:1), the Badge tints (Warning, Verified) and the violet Button; kit's focusable `tabpanel` (`kit/tabs.go`) and `examples/counter`'s button show no focus ring; a checked radio card in `examples/settings` shows none either (`focus-visible:ring-[3px]` seems to lose to `aria-checked:ring-1`: check Tela's variant order against Tailwind's); kit's `TitleBar` window controls are 12x12 px (under 24x24)
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
- [ ] Decide whether `site.yml`'s check also runs on work branches: it runs on `main`, pull requests and tags only, so the broken anchor of phase 28 showed after the merge and the tag (needs the owner's approval, ADR-290)
- [ ] Decide when the GitHub wiki retires (until then both are published from `docs/wiki`); wikidoc's reference pages still say "start at the wiki home", and on the site the reference pages' contents list "Overview" twice (Starlight's page top and the package overview)

## Road to v1.0

`v1.0.0` is not scheduled; these gate it.

- Phase 7's breaking changes are done and their deprecations removed. From phase 10 (`kit` on the final API) on, **breaking changes need an ADR and a deprecation cycle**.
- The API freezes when the phases up to Accessibility are done: stable `tela`, `h`, `aria`, `kit` and `telatest`; Linux (Wayland and X11), Windows and macOS supported, macOS once its live-check session passes; accessibility working on all three. The GPU backend changes no public API and does not gate it.
- Godoc and wiki cover every exported symbol with a runnable example; `llms.txt` matches the API.
- At least two non-trivial applications outside the repository build against each release in CI.

## Separate packages

Optional, outside the core module. Until `tela add` exists (see "Deferred") they are plain Go modules that applications import. They are not scheduled; each starts when a real application needs it.

- **Charts:** line, bar, area, pie, scatter, axes, tooltips, built on `Canvas` (phase 31) or plain `Div`/SVG until then
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
