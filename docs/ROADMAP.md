# Tela Roadmap

> **Living** document, updated at the end of every session. Design lives in
> [`ARCHITECTURE.md`](ARCHITECTURE.md); this file only tracks **what is left and
> where we are**.

## Current status

- **Phase in progress:** Phase 3 (tree, style, layout), not started; its plan needs approval.
- **Phase 2 done** (2026-10-12): mixed-script paragraphs (Latin, Arabic, CJK, emoji) render correctly and are covered by goldens (`internal/text`, `examples/text`).
- **Phase 2 delivered:** embedded fonts (`internal/assets/fonts`: Inter, JetBrains Mono, Noto Sans Arabic, Noto Color Emoji COLRv1; CJK subset for tests only), `gfx.DrawGlyphRun` + `gfx.Font` and the raster glyph cache (1/4 px subpixel x, whole-px baselines; ADR-029), `internal/text` font layer (variable instances, deterministic glyf/gvar port, COLRv1 to DisplayList, CSS matching, per-grapheme fallback; ADRs 030–032), Parley-style paragraph layout (bidi, HarfBuzz via go-text, UAX #14, white-space, justify, ellipsis, line-clamp, inline boxes, intrinsic sizes; ADR-034), cursors/selection/hit-testing/navigation (ADR-036), optional system fonts (ADR-035), `examples/text`. go-text pinned to an unreleased commit for a kerning/`ccmp` fix (ADR-033). `gfx.Affine` made fusion-safe.
- **Verified:** text layout matches headless Chromium on 34 of 38 reference paragraphs within 0.052 px (the rest bounded and explained); carets and selections match Chromium 152 (1,216 carets, 144 selections; 9,743 of 9,838 clicks); COLRv1 emoji within mean 0.2–1.6 levels of Chromium (flags up to 3.7, soft-light approximated). Text digests identical on amd64, amd64 v3 and arm64 (qemu); text tests pass under Wine. The `examples/text` window is pixel-identical to `tela snapshot` on Wayland (scale 2) and X11 except the CJK label (system bold CJK face); idle CPU 0 ticks.
- **Known limitations:** phase 1's list still applies (stroke self-overlap pixels, `Rotate(π/2)` not exact, color conversions not proven fusion-safe, conic atan per pixel, no mipmaps, `Path.Stroke` allocs, rotated shadows slow, `DrawShadow` edge clipping vs Blink, σ/offset caps, blur layers in bands, worker scratch memory; X11 back/forward lost-release ambiguity). Platform: the event-delivery and frame-pacing loop is duplicated in `wayland`, `x11` and `headless`; unify it before writing `win32`. Text: COLR blend composites (flags) and non-solid SrcIn masks are approximated and focal radial gradients emulated until `gfx`/`raster` gain blend modes, a focal gradient and mask layers (`exactBlend` in `internal/text`); line ends are not reshaped (≤ 0.4 px vs Chromium); URL breaks follow UAX #14, not Blink's table; no hyphenation, skip-ink or synthetic bold/italic; CJK glyph forms ignore language (JP vs SC); interned faces are never evicted; system font files are read fully (no mmap); serif/cursive/fantasy generics have no system mapping; go-text's `WordIterator` drops a word starting where the previous ends (worked around; report upstream); `SelectedText` returns source bytes (collapsed spaces included); an inline box at a wrapped line's edge has one side without a cursor; Windows/macOS system font discovery only cross-compiled; glyph raster switches between cached and path modes under rotation (up to 1/8 px x, 1/2 px y jump); text blending is ~77% of warm text cost (a coverage LUT would help).
- **Next step:** propose the phase-3 plan (tree, style, layout) and wait for approval. Read ARCHITECTURE §6–§10 and the §20 references (Flutter, React, Compose, Taffy, Tailwind) first.

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
- **2026-10-12:** phase 2 implemented and verified (fonts, glyph raster cache, `internal/text` font layer, paragraph layout, cursor/selection, system fonts, `examples/text`); ADRs 029–036; `gfx.Affine` fusion fix; go-text pinned to e49e2bd. Checked against Chromium (layout, carets, emoji), arm64 (qemu), Wine, and Wayland/X11 windows on Hyprland. Phase 2 done. Pending: phase-3 plan.
