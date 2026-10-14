# Tela Roadmap

> **Living** document, updated at the end of every session. Design lives in
> [`ARCHITECTURE.md`](ARCHITECTURE.md); this file only tracks **what is left and
> where we are**.

## Current status

- **Phase in progress:** none; next is phase 6, whose plan needs approval. Phase 5 is tagged `v0.5.0` (8a4bf5d), with CI fixes in `v0.5.1` and docs plus `examples/native` in `v0.5.2`; phase 4 `v0.4.0` (9243106).
- **Phase 4 done** (2026-10-13): the §1 `Counter` is interactive by mouse and keyboard, live on Wayland and X11 and through `telatest`; scrolling at 800×600 logical x2, p95 frame time with raster over 1200 scripted frames is 2.28 ms for a 100,000-row `VirtualList` and 2.58 ms for a 10,000-row plain list (target ≤ 16 ms); live in a 2832×1692 window the `fps` overlay reads 6.5 ms (VirtualList) and 8.5 ms (plain list) per frame. Windows after interactive scrolling are pixel-identical to `tela snapshot` on Wayland and X11; 0 idle CPU; arm64 (qemu) and Wine pass. A `/code-review high` of the phase found 7 issues (worst: X11 `XI_DeviceChanged` decoded at the wrong offsets, with a test that shared the mistake); all fixed with regression tests.
- **Phase 4 delivered:** a shared platform event loop and `KeyEvent` with W3C names (ADR-050); hooks with comparable handles (`Setter`, `Dispatcher`, `Callback`, `Ref`, `ElementRef`), `deps ...any`, `C`/`CP`/`Memo` skipping, `Post`, dev hook-order checks (ADR-051); `UseValue` with phase-deferred reads (ADR-053); a pure-Go XKB subset checked against libxkbcommon, Wayland keyboard with Compose and client-side repeat (ADR-052); vendored xgb, XInput2 scrolling and the X11 keyboard (ADR-054); DOM pointer events with a Flutter gesture arena and interaction variants (ADR-055); focus, Tab order and keyboard dispatch (ADR-056); scrolling with viewport scrolling, overlay/classic scrollbars, smooth wheel and kinetic touchpad (ADR-057); a semantics tree and Playwright aria snapshots, `tela inspect --aria` by default (ADR-058); `telatest` and a public headless entry point, so the CLI works outside the tela module (ADR-059); `h.VirtualList` (ADR-060); layers as cached display lists with exact damage and pixel shifts (ADR-061); debug overlays (ADR-062); five example screens (`dashboard`, `pricing`, `settings`, `player`, `board`) plus `virtuallist`. The race detector is allowed for verification only (AGENTS.md); `staticcheck` is clean and part of the checks.
- **Phase 4 known limitations:** classic scrollbar gutters square off rounded corners; positioned descendants can paint over overlay scrollbars; no `sticky`; X11 has no touchpad fling (no gesture-end signal); gradients and images in scrolled content are re-rasterized on scroll (`examples/shapes` stays ~180 ms per frame at x2); a plain 10k-row list peaks at ~840 MB RSS after mounting ~90k elements; hit-testing got ~20% slower with scroll spaces (4.2 µs on 10k); X11 multi-layout AltGr levels are approximate (core mapping, ADR-054); `telatest` cannot isolate `tela.Post` between parallel tests; `Type` produces no text until phase 5's inputs.
- **Phase 3 done** (2026-10-12): the §1 `Counter` (`examples/counter`) renders like its HTML + Tailwind twin in Chromium 152: every box edge within 0.01 px (text baselines at scale 2 within 0.5 px, ADR-048), pixels equal except glyph antialiasing (whole-image mean 0.23 levels at x1, 0.21 at x2). The window is pixel-identical to `tela snapshot` on Wayland (scale 2) and X11, 0 idle CPU ticks; arm64 (qemu) and Wine pass.
- **Phase 3 delivered:** Tailwind v4.3.3 theme generator (ADR-037); `internal/style` (ComputedStyle, interned class parser with arbitrary values, `/NN`, variants evaluated against a context, suggestions, Preflight defaults; ADRs 038, 042); `internal/layout` (block, flex, relative/absolute, intrinsic sizes, caching, relayout boundaries; 828/828 included Taffy v0.14.0 fixtures match Chromium; ADRs 039, 043); `internal/core` (Node/Element/Box, keyed reconciliation, `UseState`, frame-on-demand scheduler, paragraphs, headless render, inspect walk; ADRs 040, 044) with CSS box painting matched against Chromium on a 56-cell gallery (ADR-045) and a raster `BackdropFilter`; public `tela` and `h`; `tela snapshot` (Previews/Preview/Scene, `--dark`, `--preview`) and `tela inspect` (ADR-047). Determinism fixes: stdlib trig and color conversions replaced by `internal/fmath` (moved from `gfx/internal`), pinned by digests (ADR-041); colors are extended sRGB like Chromium (ADR-046).
- **Phase 2 done** (2026-10-12): mixed-script paragraphs (Latin, Arabic, CJK, emoji) render correctly and are covered by goldens (`internal/text`, `examples/text`).
- **Phase 2 delivered:** embedded fonts (`internal/assets/fonts`: Inter, JetBrains Mono, Noto Sans Arabic, Noto Color Emoji COLRv1; CJK subset for tests only), `gfx.DrawGlyphRun` + `gfx.Font` and the raster glyph cache (1/4 px subpixel x, whole-px baselines; ADR-029), `internal/text` font layer (variable instances, deterministic glyf/gvar port, COLRv1 to DisplayList, CSS matching, per-grapheme fallback; ADRs 030–032), Parley-style paragraph layout (bidi, HarfBuzz via go-text, UAX #14, white-space, justify, ellipsis, line-clamp, inline boxes, intrinsic sizes; ADR-034), cursors/selection/hit-testing/navigation (ADR-036), optional system fonts (ADR-035), `examples/text`. go-text pinned to an unreleased commit for a kerning/`ccmp` fix (ADR-033). `gfx.Affine` made fusion-safe.
- **Verified:** text layout matches headless Chromium on 34 of 38 reference paragraphs within 0.052 px (the rest bounded and explained); carets and selections match Chromium 152 (1,216 carets, 144 selections; 9,743 of 9,838 clicks); COLRv1 emoji within mean 0.2–1.6 levels of Chromium (flags up to 3.7, soft-light approximated). Text digests identical on amd64, amd64 v3 and arm64 (qemu); text tests pass under Wine. The `examples/text` window is pixel-identical to `tela snapshot` on Wayland (scale 2) and X11 except the CJK label (system bold CJK face); idle CPU 0 ticks.
- **Known limitations:** phase 1's list still applies (stroke self-overlap pixels, `Rotate(π/2)` not exact, conic atan per pixel, no mipmaps, `Path.Stroke` allocs, rotated shadows slow, `DrawShadow` edge clipping vs Blink, σ/offset caps, blur layers in bands, worker scratch memory; X11 back/forward lost-release ambiguity). Text: COLR blend composites (flags) and non-solid SrcIn masks are approximated and focal radial gradients emulated until `gfx`/`raster` gain blend modes, a focal gradient and mask layers (`exactBlend` in `internal/text`); line ends are not reshaped (≤ 0.4 px vs Chromium); URL breaks follow UAX #14, not Blink's table; no hyphenation, skip-ink or synthetic bold/italic; CJK glyph forms ignore language (JP vs SC); interned faces are never evicted (each variable-font instance keeps its own Face and glyph caches, so animating an axis grows memory; bound it with phase 6's animations); system font files are read fully (no mmap); serif/cursive/fantasy generics have no system mapping; go-text's `WordIterator` drops a word starting where the previous ends (worked around; report upstream); `SelectedText` returns source bytes (collapsed spaces included); an inline box at a wrapped line's edge has one side without a cursor; Windows/macOS system font discovery only cross-compiled; glyph raster switches between cached and path modes under rotation (up to 1/8 px x, 1/2 px y jump); text blending is ~77% of warm text cost (a coverage LUT would help). Phase 3: grid lays out as block and `fixed`/`sticky` as absolute/relative; inline elements contribute only their text style (no inline backgrounds, padding or borders; no atomic inline-blocks inside paragraphs; planned in phase 5); `bg-clip-text` and `url()` backgrounds (planned in phase 6) and `mix-blend-mode` are not painted; a viewport change restyles every element; cold mount of 10k elements ~55 ms (shaping 37%); Large blurs are slow: `examples/shapes` rasterizes in ~180 ms per frame at scale 2 (two `blur-3xl` glows of 28rem), so resizing lags; phase 4's damage tracking makes unchanged glows free, a faster blur is still needed for full frames.
- **API gaps found by the phase-4 examples** (to triage when planning phase 5): (1) element size/bounds in handlers or hooks (`ElementRef.Bounds()`, a `DragEvent` drop target) — the player's seek bar hard-codes its width; (2) no phase-deferred text (`TextFn`), so a time label needs its own rebuilding component; (3) CSS border triangles (transparent adjacent borders need mitered joins) draw rectangles; (4) negative `space-x-*` has no effect; (5) conditional props are awkward (`If` takes only nodes; no `Prop` alias in `h`); (6) no indexed `Map`; (7) `Value.Get` in a handler returns the last applied value, not the latest set; (8) inspect rects ignore transforms.
- **Phase 5 done criteria** (2026-10-13): every example runs on real Windows 11 (VM, job runner): 9 of 11 windows are pixel-identical to `tela snapshot` and the other two differ for known reasons (player's live progress, text's system CJK fonts); all example test binaries pass natively with bit-exact goldens. Japanese typed through the IME into a real `Input`: Mozc on Hyprland (にほんご → 日本語, `OnChange` with `IsComposing`) and Microsoft IME on Windows in `examples/signup` (preedit underlined, candidate list under the caret, 日本語 committed). Wine runs of the examples started cleanly before Wine was banned locally.
- **Phase 5 delivered:** win32 backend (window, DPI, pointer, cursors, keyboard with layouts/AltGr/dead keys, clipboard, settings, IMM32 IME; ADRs 063, 067, 071); Wayland clipboard (ADR-064) and `text-input-v3` IME (ADR-068); `internal/text/edit` (ADR-065); inline decorations and atomic inline-blocks (ADR-066); `Input`, `Textarea`, `Form`, `Label` with the owner's API (ADR-069); the phase-4 API gaps (ADR-070); CI workflows (unverified until pushed); `examples/signup`; the VM job runner (outside the repo).
- **Phase 5 known limitations:** no local Wine (three hard freezes of this host on 2026-10-13 started seconds after Wine launches, suspected `ntsync` on kernel 7.2; CI keeps its Wine job); `invalid:` without `required`/`pattern`; `number` inputs are hints only; no X11 IME (phase 8); untested: Korean/Chinese IMEs on Windows, IMM32 reconversion, DPI ≠ 1 IME placement; `telatest` cannot register a CJK font, so goldens show CJK only through system fonts in live windows; left Ctrl+Alt is not AltGr on Windows (contract choice); a paragraph with atomic inlines re-breaks lines on every layout request; `UseBounds` readers see changes one frame late.
- **Phase-5 plan:** T0 VM job runner · T1 win32 base · T2 Wayland clipboard · T3 `internal/text/edit` · T4 inline decorations and atomic inline-blocks · T5 win32 keyboard, clipboard, settings · T6 IME contract + Wayland `text-input-v3` · T7 `Input`/`Textarea`/`Form`/`Label` · T8 IMM32 IME · T9 phase-4 API gaps (all eight) · T10 CI (Wine, `windows-latest`) · T11 forms example and Windows validation. Waves: A (T1–T4), B (T5, T6), C (T7, T9), D (T8, T10, T11).
- **Next step:** plan phase 6 (grid, fixed/sticky, animation, effects). Agents wrap heavy commands in `systemd-run --user --scope -p MemoryMax=… -p MemorySwapMax=0` and never run Wine locally.

## Phases

Each phase ends with verifiable criteria. A phase is not done unless its criteria pass with `CGO_ENABLED=0`. Before starting a task, read the matching row in ARCHITECTURE §20 (references).

### Phase 1: Pixels on screen

- [x] `internal/gfx`: geometry types, `Color` (hex and OKLCH parsing), `Path`, `RRect`, `DisplayList` and its commands
- [x] `internal/gfx/raster`: analytic-AA path fill, rect/rrect fast paths, linear/radial/conic gradients, clip, transform, opacity layers
- [x] Stroking (joins, caps, dashes) following kurbo (ADR-026)
- [x] Analytic erf rounded-rect shadows (incl. inset); separable Gaussian blur for other shapes (ADR-028)
- [x] Parallel tile rasterization (horizontal bands, dynamic scheduling)
- [x] `internal/platform`: `Backend`/`Window` interface and `headless` backend
- [x] Wayland XML → Go protocol generator (+ `wire` runtime)
- [x] `internal/platform/wayland`: connection, registry, `xdg_shell`, `wl_shm` via memfd, frame callbacks, pointer, integer scale
- [x] `internal/platform/x11`: window, `PutImage`, basic events
- [x] `examples/shapes` and a first `tela snapshot` (no UI tree yet: a fixed DisplayList, contract `Scene(w, h float32) *gfx.DisplayList` until `Preview()` in phase 3)

**Done when:**
1. A scene with rounded cards, gradients, shadows and blur looks identical on Wayland, X11 and the headless PNG.
2. Rasterizer goldens pass.
3. CPU is at 0% when idle.

### Phase 2: Text

- [x] Embedded fonts (sans, mono, emoji) with licenses in `internal/assets/fonts`
- [x] Shaping and bidi with `go-text/typesetting`; UAX #14 line breaking; font fallback
- [x] Parley-style text layout: styled runs → lines, cursor positions, selection rects, hit-testing
- [x] Glyph raster cache with subpixel positioning
- [x] Ellipsis and line-clamp
- [x] Optional system font discovery

**Done when:** mixed-script paragraphs (Latin, Arabic, CJK, emoji) render correctly, covered by goldens.

### Phase 3: Tree, style, layout

- [x] Tailwind v4 theme generator (`go generate`): default theme + class vocabulary
- [x] `internal/style`: `ComputedStyle`; class parser (spacing, sizing, colors, borders, radius, shadows, typography, flex, grid); arbitrary values; `/NN` opacity; theme and tokens
- [x] Suggestions for unknown classes
- [x] `internal/layout`: constraints protocol, block, full flex, absolute/relative positioning; port Taffy flex fixtures
- [x] `internal/core`: Node/Element/Box, keyed reconciliation, phased scheduler with dirty flags
- [x] Packages `tela` and `h`:
  - `Run`, `App`, `Node`, `Child`
  - Elements: `Div`, `Span`, `Text`, `Textf`, `H1`–`H6`, `P`, `Button`, `Img`
  - Props and helpers: `Class`, `Style`, `Key`, `If`, `Map`, `Fragment`
- [x] `tela snapshot` and `tela inspect --layout` on components (`Preview()` contract)

**Done when:** the `Counter` example from ARCHITECTURE §1 (non-interactive) renders identically to its HTML + Tailwind equivalent, compared visually against browser screenshots.

### Phase 4: Interaction and state

- [x] Hooks:
  - `UseState`, `UseReducer`, `UseEffect`, `UseLayoutEffect`, `UseMemo`, `UseCallback`, `UseRef`, `UseContext`, `UseId`
  - Component boundaries `tela.C` and `tela.CP`
  - Dev-mode detection of invalid hook order
- [x] Automatic skipping for comparable props; `UseValue` with phase-deferred reads
- [x] Input:
  - Hit-testing, event bubbling, `OnClick` and friends
  - Gesture arena (tap, drag, long-press)
- [x] Variants `hover:`, `active:`, `focus:`, `focus-visible:`, `disabled:`, `group-*`
- [x] Keyboard:
  - Focus and tab order
  - Physical/logical/text model
  - XKB parser (Wayland) and X11 keyboard
  - Vendor a patched `jezek/xgb` (core, `xproto`, `bigreq`) that reads GenericEvents, before XInput2 (ADR-025)
- [x] Scrolling:
  - Scroll (`overflow-auto`) with its own layer and smooth scrolling (scrolling done in T7a; the layer is T7b)
  - Repaint boundaries and damage tracking (done in T7b)
  - `h.VirtualList` (done in T8)
- [x] Semantics tree; `tela inspect --aria` (Playwright-style YAML)
- [x] `telatest`: `Mount`, `Click`, `Type`, `ByRole`, `ByText`, `Golden`, `AriaSnapshot`, controllable clock, auto-wait
- [x] Debug overlays (`TELA_DEBUG=layout,paint-flash,layers,fps,semantics`)

**Done when:** `Counter` is interactive, and a 100,000-row `VirtualList` (10,000 without virtualization) scrolls smoothly on CPU rendering.

### Phase 5: Windows and text editing

- [x] `win32` backend: window, message loop, present, per-monitor DPI v2, keyboard, mouse, cursor, clipboard
- [x] `Input` and `Textarea`: cursor, selection, undo/redo, clipboard
- [x] IME on Wayland (`text-input-v3`) and Windows (IMM32)
- [x] Inline element decorations in paragraphs: backgrounds, padding, borders and radii of `Span`/`Code`/`A` (e.g. `<code class="bg-slate-100 px-1 rounded">`), and atomic inline-blocks (buttons, badges) inside a paragraph (revisits ADR-044)
- [x] CI with `go test -exec wine` and a `windows-latest` runner

**Done when:** examples run on Windows (Wine and real), and Japanese text can be typed via IME on Wayland and Windows.

### Phase 6: Advanced CSS and animation

- [ ] Full grid (port Taffy grid fixtures)
- [ ] `fixed` and `sticky` positioning
- [ ] Animation:
  - Transitions (`transition`, `duration-*`, `ease-*`)
  - `UseAnimation`, `UseSpring`, `Presence`
  - `animate-*` classes
- [ ] Layout animations: `AnimateLayout`, `LayoutID`
- [ ] Effects: `backdrop-filter`, `filter`, `mix-blend-mode`, basic `clip-path`, full transforms
- [ ] Background images (`bg-[url(...)]`, `bg-cover`/`contain`, position) and `bg-clip-text` (gradient text)
- [ ] Portal layer, z-index and stacking contexts
- [ ] Responsive breakpoints, `dark:`, `motion-reduce:`
- [ ] Wayland fractional scaling

**Done when:** a "dashboard" example with glassmorphism, layout animations and transitions runs at 60 fps at 1080p on CPU rendering.

### Phase 7: aria, kit, tooling, LLM docs

- [ ] `aria`:
  - `UsePress`, `UseHover`, `UseFocusRing`, `UseFocusScope`, `UseOverlay`, `UseDialog`, `UseTooltip`
  - Collections: `UseListBox`, `UseMenu`, `UseSelect`, `UseTabs`, `UseCombobox`
- [ ] `aria.UseFloating` (Floating UI port: offset, flip, shift, size, arrow)
- [ ] `kit` (shadcn-style look): Button, Input, Checkbox, Switch, Select, Dialog, Popover, Tooltip, Tabs, Table, Toast, Card, Badge, Avatar
- [ ] CLI commands: `tela add`, `tela lint`, `tela dev`, `tela mcp`
- [ ] `docs/llms.txt` and `examples/` with full screens (dashboard, login, settings, chat)

**Done when:** an agent with no prior context, given only `llms.txt` and `tela mcp`, builds a beautiful settings screen in under 3 snapshot iterations.

### Phase 8: macOS, GPU, accessibility

- [ ] `cocoa` backend via purego, with CI on `macos-latest`
- [ ] Optional GPU backend (Vulkan, Metal) over the DisplayList
- [ ] Accessibility: AT-SPI, UI Automation, NSAccessibility
- [ ] IME on X11 (XIM) and macOS (`NSTextInputClient`)

## Session log

One line per session: date, what was done, what is pending.

- **2026-10-11:** project created with ARCHITECTURE and ROADMAP. Docs moved to English; references (§20) and ADRs 012–021 added. No code yet.
- **2026-10-11:** phase 1 implemented (gfx, raster with shadows/filters/parallel bands, platform + headless, Wayland, X11, auto selection, `examples/shapes`, `tela snapshot`); ADRs 022–028. Verified: goldens bit-identical on amd64 and arm64 (qemu), shadows within ±3 of Chromium away from edges, Wayland and X11 windows pixel-identical to `tela snapshot`, 0% idle CPU. A test that peaked at 2.8 GiB froze the machine next to the qemu run; fixed. Windows VM run passed the same day: phase 1 done. A `/code-review high internal/platform` found 8 defects (worst: Wayland loop spinning when a frame is requested without a Present), all fixed with regression tests and checked live on Hyprland.
- **2026-10-12:** phase 2 implemented and verified (fonts, glyph raster cache, `internal/text` font layer, paragraph layout, cursor/selection, system fonts, `examples/text`); ADRs 029–036; `gfx.Affine` fusion fix; go-text pinned to e49e2bd. Checked against Chromium (layout, carets, emoji), arm64 (qemu), Wine, and Wayland/X11 windows on Hyprland. Phase 2 done. A `/code-review high` of phase 2 found 7 issues (worst: a malformed cmap range hung the system font scan; the first system font scan blocked registered-font queries under the collection lock); 6 fixed with regression tests, face eviction recorded as a known limitation. Pending: phase-3 plan.
- **2026-10-12:** phase-2 `/code-review` fixes; phase 3 implemented and verified (Tailwind theme generator, `style`, `layout` with Taffy fixtures, `core` with CSS box painting against Chromium, `tela`/`h`, `tela snapshot`/`inspect`, `examples/counter`); ADRs 037–048. Found and fixed cross-architecture differences in stdlib trig and color conversions (ADR-041) and switched to Chromium-like extended sRGB colors (ADR-046). Phase 3 done. A `/code-review high` of phase 3 found 7 issues (worst: a data race between raster bands on per-pixel backdrop filters; a layout cache key collision between negative available space and known sizes); 6 fixed with regression tests, the region-render cost recorded as a known limitation. Pending: phase-4 plan.
- **2026-10-12:** versioning adopted (ADR-049): `CHANGELOG.md`, `tela version`, retroactive tags v0.1.0–v0.3.0. `examples/shapes` rewritten with the phase-3 public API (window pixel-identical to `tela snapshot` on Wayland and X11, 0 idle CPU); the phase-1 scene became a raster golden test. Pending: phase-4 plan.
- **2026-10-12:** `examples/shapes` made responsive (fixed-size cards wrap; tiled 701x846 windows overflowed); a suspected Wayland hang was not reproducible on an empty visible workspace (same tiled layout, resizes, closes in ~100 ms) and was most likely the test script addressing windows by title while the user ran the same example. Pending: phase-4 plan.
- **2026-10-12:** phase-3 leftovers: `calc()` of a length and a percentage in `layout` (`UnitCalc`, a Chromium-measured case) and in core's mapping (v0.3.1); inline decorations assigned to phase 5, background images and `bg-clip-text` to phase 6. Pending: phase-4 plan.
- **2026-10-12/13:** phase 4 implemented and verified (T1–T12 plus five example screens): platform loop, hooks, `UseValue`, XKB and keyboards, X11 XInput2, pointer input and gestures, focus, scrolling, layers and damage, semantics, `telatest`, `VirtualList`, debug overlays; ADRs 050–062. The owner chose the hooks' API (comparable `Setter` with `Set`/`Update`, wrappers, `Dispatcher`, latest-fn `Callback`); later API decisions were taken while the owner was away and are listed for review. Verified live on Hyprland (Wayland and X11), arm64 (qemu), Wine, `-race`. A `/code-review high` found 7 issues, all fixed. Phase 4 done; CHANGELOG moved to v0.4.0 and tagged `v0.4.0` with the owner's approval.
- **2026-10-13:** owner's `examples/hello` reviewed (English copy, `Preview()`, README, goldens, 800x450) and used as the README hero (`docs/images/hello.png`); README polished for GitHub (clone step, Windows/macOS wording). `LICENSE` added (BSD-3-Clause, 2026, Andres Martinez) with a README section. Pending: phase-5 plan.
- **2026-10-13:** phase 5 planned (owner chose the text-field and API-gap shapes up front) and implemented in parallel worktrees (T1–T11): win32 backend with keyboard, clipboard, settings and IMM32 IME; Wayland clipboard and text-input-v3; `internal/text/edit`; inline decorations; `Input`/`Textarea`/`Form`/`Label`; phase-4 API gaps; CI workflows; `examples/signup`; ADRs 063–071. Verified on real Windows 11 through a VM job runner and live on Hyprland (Mozc). The host hard-froze three times; the trigger was local Wine runs (now banned; suspected ntsync), not memory. `/code-review high` found 9 issues (worst: Alt+numpad characters lost on Windows, Alt chords typing into fields), all fixed with regression tests (ADR-072). CHANGELOG moved to v0.5.0.
- **2026-10-14:** `v0.5.0` tagged on 8a4bf5d with the owner's approval. Pending: phase-6 plan.
- **2026-10-14:** first CI runs on GitHub: windows-latest failed on CRLF checkouts (fixed with `.gitattributes`), the race job timed out, the Wine job hung on inherited pipes and lacked a keyboard layout; all fixed, all 12 jobs green. CHANGELOG v0.5.1. `v0.5.1` tagged on dc43ae2. Pending: a VM rerun of the live keyboard and IME tests (job 012, VM was off), phase-6 plan.
- **2026-10-14:** public API documented: `docs/wiki/` (12 guide pages + 3 generated API references via `internal/gen/wikidoc`, stdlib only), 24 runnable examples in `h` and `telatest`, pkg.go.dev fixes and badge. Known doc gaps: alias types (`MouseEvent`, `Value`...) are opaque on pkg.go.dev, detached top comments in `events.go`/`fields.go` appear nowhere, dot-import collisions (`Name`, `Label`, `Value`, `Type`, `Code`, `Key`) are not warned about, no `aria-*:` variants. Wiki not yet copied to `tela.wiki.git`. Next: `examples/native` (file explorer with Windows 11, Adwaita and Breeze themes, code-only, no new API).
- **2026-10-14:** `examples/native` (file explorer, Win11/Adwaita/Breeze x light/dark, 6 goldens + 3 at x2, interaction tests, arm64 checked). API gaps found: system dark mode is not readable from Go (only through the `dark:` variant), no public window-chrome controls or header-bar drag, no `clip-path`, `HasText("")` and partial `ExpectAria` missing. Fidelity to the real platforms is from memory, not measured; no real Windows run. Next: a reusable `tela/theme` or `kit` (owner's call), then phase 6.
- **2026-10-14:** `AppID` of the examples moved to `dev.telaui.*` (new domain); `v0.5.2` tagged with the owner's approval. Pending: wiki copy to `tela.wiki.git`, phase-6 plan.
