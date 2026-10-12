# Tela Roadmap

> **Living** document, updated at the end of every session. Design lives in
> [`ARCHITECTURE.md`](ARCHITECTURE.md); this file only tracks **what is left and
> where we are**.

## Current status

- **Phase in progress:** Phase 2 (text), not started; its plan needs approval.
- **Phase 1 done** (2026-10-11): all criteria pass.
- **Phase 1 delivered:** `internal/gfx`, platform interface + `headless`, Wayland generator + wire runtime, X11 backend, Wayland backend (0% idle CPU measured on both windowed backends), raster (path AA, rect/rrect fast paths, transforms, gradients, images, clips, opacity layers, region rendering, golden helper), stroking, shadows and layer filters (analytic erf rounded-rect shadows, Gaussian blur, drop-shadow, color matrices; Tailwind presets match Chromium within ±3 levels away from edges), parallel rendering (full-width bands, bit-identical to sequential; 5–6× on 18 threads at 1080p), backend selection (`internal/platform/auto`), `examples/shapes` and `tela snapshot`.
- **Verified on Hyprland:** the `examples/shapes` window is pixel-identical to `tela snapshot` on Wayland (scale 2) and on X11 (Xwayland, zero scaling). Idle CPU is 0 ticks on both; X11 has no wakeups at all, Wayland wakes only to answer Hyprland's `xdg_wm_base.ping` (~0.7/s). On real Windows 11 (VM), the gfx, raster, platform, headless, auto, golden, shapes and cmd/tela tests pass, goldens bit-identical.
- **Known limitations:** stroke outlines that self-overlap in tight curves overestimate a few edge pixels (worst seen: one pixel, 72 levels); `gfx.Rotate(π/2)` is not exact, so it misses axis-aligned fast paths; color conversions in `gfx/color.go` are not yet proven fusion-safe; conic gradients cost an atan per pixel; bilinear downscaling aliases (no mipmaps); `Path.Stroke` allocates per call; rotated shadows take the per-pixel path (10–14× the axis-aligned cost); `DrawShadow` clips with the box's coverage even under an opaque background, so anti-aliased box edges differ from Blink by up to 29 levels (phase 3 paint can pass an "unclipped" hint, needs an ADR); drop-shadow offsets are rounded to whole device pixels when σ ≥ 2; device σ capped at 256 px and offsets at 512 px; a filter layer costs its area grown by the reach in every band, so frames dominated by large blurs gain only 1.6–3.3× from parallel bands (a shared pre-pass belongs with phase-4 layer caching); worker Renderers keep their own scratch (about 18× the memory of a sequential render on filter-heavy frames). Platform: the event-delivery and frame-pacing loop is duplicated in `wayland`, `x11` and `headless` (copies already drifted once); unify it into a shared helper before writing `win32`. X11 cannot tell a lost release of only the back/forward buttons from a normal re-enter (no core state bits).
- **Next step:** propose the phase-2 plan (text) and wait for approval. Read ARCHITECTURE §11 and the §20 references for text first.

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

- [ ] Embedded fonts (sans, mono, emoji) with licenses in `internal/assets/fonts`
- [ ] Shaping and bidi with `go-text/typesetting`; UAX #14 line breaking; font fallback
- [ ] Parley-style text layout: styled runs → lines, cursor positions, selection rects, hit-testing
- [ ] Glyph raster cache with subpixel positioning
- [ ] Ellipsis and line-clamp
- [ ] Optional system font discovery

**Done when:** mixed-script paragraphs (Latin, Arabic, CJK, emoji) render correctly, covered by goldens.

### Phase 3: Tree, style, layout

- [ ] Tailwind v4 theme generator (`go generate`): default theme + class vocabulary
- [ ] `internal/style`: `ComputedStyle`; class parser (spacing, sizing, colors, borders, radius, shadows, typography, flex, grid); arbitrary values; `/NN` opacity; theme and tokens
- [ ] Suggestions for unknown classes
- [ ] `internal/layout`: constraints protocol, block, full flex, absolute/relative positioning; port Taffy flex fixtures
- [ ] `internal/core`: Node/Element/Box, keyed reconciliation, phased scheduler with dirty flags
- [ ] Packages `tela` and `h`:
  - `Run`, `App`, `Node`, `Child`
  - Elements: `Div`, `Span`, `Text`, `Textf`, `H1`–`H6`, `P`, `Button`, `Img`
  - Props and helpers: `Class`, `Style`, `Key`, `If`, `Map`, `Fragment`
- [ ] `tela snapshot` and `tela inspect --layout` on components (`Preview()` contract)

**Done when:** the `Counter` example from ARCHITECTURE §1 (non-interactive) renders identically to its HTML + Tailwind equivalent, compared visually against browser screenshots.

### Phase 4: Interaction and state

- [ ] Hooks:
  - `UseState`, `UseReducer`, `UseEffect`, `UseLayoutEffect`, `UseMemo`, `UseCallback`, `UseRef`, `UseContext`, `UseId`
  - Component boundaries `tela.C` and `tela.CP`
  - Dev-mode detection of invalid hook order
- [ ] Automatic skipping for comparable props; `UseValue` with phase-deferred reads
- [ ] Input:
  - Hit-testing, event bubbling, `OnClick` and friends
  - Gesture arena (tap, drag, long-press)
- [ ] Variants `hover:`, `active:`, `focus:`, `focus-visible:`, `disabled:`, `group-*`
- [ ] Keyboard:
  - Focus and tab order
  - Physical/logical/text model
  - XKB parser (Wayland) and X11 keyboard
  - Vendor a patched `jezek/xgb` (core, `xproto`, `bigreq`) that reads GenericEvents, before XInput2 (ADR-025)
- [ ] Scrolling:
  - Scroll (`overflow-auto`) with its own layer and smooth scrolling
  - Repaint boundaries and damage tracking
  - `h.VirtualList`
- [ ] Semantics tree; `tela inspect --aria` (Playwright-style YAML)
- [ ] `telatest`: `Mount`, `Click`, `Type`, `ByRole`, `ByText`, `Golden`, `AriaSnapshot`, controllable clock, auto-wait
- [ ] Debug overlays (`TELA_DEBUG=layout,paint-flash,layers,fps,semantics`)

**Done when:** `Counter` is interactive, and a 100,000-row `VirtualList` (10,000 without virtualization) scrolls smoothly on CPU rendering.

### Phase 5: Windows and text editing

- [ ] `win32` backend: window, message loop, present, per-monitor DPI v2, keyboard, mouse, cursor, clipboard
- [ ] `Input` and `Textarea`: cursor, selection, undo/redo, clipboard
- [ ] IME on Wayland (`text-input-v3`) and Windows (IMM32)
- [ ] CI with `go test -exec wine` and a `windows-latest` runner

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
