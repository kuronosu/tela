# Changelog

All notable changes to Tela are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/) as Go modules use it: one minor
version per completed roadmap phase while Tela is v0, patch versions for
fixes after a phase closes. Compatibility covers the public API only (`tela`,
`h`, the `tela` command's flags, the `Preview()`/`Previews` contract and the
`tela inspect` JSON); everything under `internal/` may change at any time.
In v0 a minor version may break the public API; such changes are listed
under **Breaking**. See ADR-049 in `docs/ARCHITECTURE.md`.

## [Unreleased]

### Added

- Hooks `UseReducer`, `UseEffect`, `UseLayoutEffect`, `UseMemo`,
  `UseCallback`, `UseRef`, `CreateContext`/`Provider`/`UseContext`, `UseId`,
  `UseStateSet` and `UseStateUpdate`.
- Component boundaries `tela.C`, `tela.CP` and `tela.Memo`; `CP` and `Memo`
  skip the build when the props are equal and no state or context they read
  changed.
- `tela.Post` runs a function on the UI thread.
- Dev mode (`TELA_DEV=1`): a change in hook order panics with both call
  sites; non-comparable dependencies, a changing dependency count and
  anonymous functions passed to `CP`/`Memo` are reported.
- `tela.UseValue` and `tela.Value[T]`: an observable value (`Get`, `Set`,
  `Update`, safe from any goroutine). Read inside `TranslateFn`/`OpacityFn`,
  a change repaints only; inside `WidthFn`/`HeightFn`, it lays out only that
  element; in a component's body, it rebuilds that component.
- Wayland keyboard input: key events with physical code, logical key, text
  and modifiers for any XKB layout (pure-Go keymap parser), dead keys and
  Compose sequences, and client-side key repeat. Pointer and scroll events
  now carry the keyboard modifiers.
- X11: keyboard input with the same contract as Wayland (layouts, AltGr,
  dead keys, Compose, server key repeat), smooth touchpad and
  high-resolution wheel scrolling through XInput2, and pointer modifiers
  from the real modifier mapping.
- Pointer input: `OnClick`, `OnMouseDown`, `OnMouseUp`, `OnMouseEnter` and
  `OnMouseLeave` now fire, plus the new `OnDoubleClick`, `OnContextMenu`,
  `OnMouseMove`, `OnPointerDown/Move/Up/Cancel/Enter/Leave`,
  `OnDragStart`/`OnDrag`/`OnDragEnd` and `OnLongPress`. Events bubble, with
  `StopPropagation` and `PreventDefault`. `tela.MouseEvent` gains `LocalX/Y`,
  `Buttons`, modifier fields and `ClickCount`; new `tela.PointerEvent`,
  `tela.DragEvent`, `tela.MouseButtons` and `tela.PointerKind`.
- `h.Disabled` and the `hover:`, `active:`, `disabled:`, `enabled:`,
  `group-*` and `peer-*` variants; only elements whose classes test a state
  are restyled. The window cursor follows `cursor-*` under the pointer.
- Keyboard events: `h.OnKeyDown`/`h.OnKeyUp` with `tela.KeyboardEvent`
  (`Key`, `Code`, `Location`, `Repeat`, modifiers); `tela.Key`/`tela.Code`
  constants for every W3C key and code.
- Focus: `h.TabIndex`, `h.AutoFocus`, `h.OnFocus`/`h.OnBlur` (bubbling),
  `tela.UseElementRef` + `h.Ref` with `Focus`/`Blur`/`Focused`; Tab and
  Shift+Tab navigation, Enter/Space activate buttons, a click focuses.
- `focus:`, `focus-visible:` (keyboard navigation, not clicks) and
  `focus-within:` variants, with their `group-`/`peer-` forms.
- The §1 Counter is interactive, by mouse and by keyboard.
- Scrolling: elements with `overflow-auto`/`overflow-scroll` scroll with the
  wheel, the touchpad (with kinetic flings on Wayland), touch, the keyboard
  (arrows, Page Up/Down, Home/End, Space) and their scrollbars;
  `overflow-hidden` scrolls from code. Scrolling chains to scrolling
  ancestors unless `overscroll-contain`/`overscroll-none`. The window
  scrolls like a browser page when its content is larger than the window
  (`RootClass: "overflow-hidden"` turns it off).
- `h.OnWheel` and `h.OnScroll`, with `tela.WheelEvent` and
  `tela.ScrollEvent`; `tela.ElementRef` gains `ScrollTo`, `ScrollBy`,
  `ScrollIntoView` and `ScrollOffset`; keyboard focus scrolls the focused
  element into view; `tela.UseScroll(ref)` returns Values that follow an
  element's scroll offsets, for scroll-linked effects without rebuilds.
- Tela draws the scrollbars: thin overlay bars that fade by default, or
  classic always-visible bars with `tela.App{ClassicScrollbars: true}` or
  `scrollbar-classic` (`scrollbar-overlay` per element). Wheel and keyboard
  scrolling animate; `tela.App{InstantScroll: true}` or `scroll-instant`
  turns that off, `scroll-smooth` animates programmatic scrolls, and the
  system's reduced-motion setting always disables it.
- `h.Role`, `h.Alt` and the `h.Aria…` props (label, labelledby, description,
  hidden, checked, pressed, selected, expanded, disabled, level, value
  now/min/max/text); unknown roles panic in dev mode with a suggestion. A
  semantics tree with implicit roles and accessible names.
- Examples `dashboard`, `pricing`, `settings`, `player` and `board`: full
  screens in the shadcn/ui zinc style with dark mode, goldens and
  interaction tests (switches and `disabled:` states, a progress bar driven
  by `UseValue` without rebuilds, drag and drop with `OnDrag*` and
  `TranslateFn`). The notes of `counter`, `shapes` and `text` saying that
  events arrive in phase 4 are gone.

### Changed

- `tela inspect` prints a Playwright-style aria snapshot (YAML) by default;
  `--layout` prints the JSON layout tree.
- **Breaking:** `tela.UseState` returns a `tela.Setter[T]` instead of
  `func(T)`. Write `setCount.Set(v)` or
  `setCount.Update(func(n int) int { return n + 1 })`, or use
  `tela.UseStateSet` for a plain function.

### Removed

- The `github.com/jezek/xgb` module dependency: a patched copy of its core,
  `xproto` and `bigreq` is vendored under `internal/` (ADR-025).

## [0.3.1] - 2026-10-12

### Added

- `tela version` prints the module version, the VCS revision and the Go
  version and platform.

### Changed

- `examples/shapes` is rewritten with the public API (`tela`, `h`, Tailwind
  classes): the same 12-card page as components, with real labels; shapes
  that need arbitrary paths are replaced by CSS equivalents (listed in its
  README). The phase-1 DisplayList scene lives on as a golden test of
  `internal/gfx/raster`. Its cards keep their size and wrap, so a tiled
  window of about 700 px shows three columns instead of squeezing four.

### Fixed

- A length with both a pixel and a percentage part (`w-[calc(100%-2rem)]`,
  percentage `space-*` added to a margin) kept only its percentage; layout
  now resolves `calc()` of a length and a percentage like Chromium.

## [0.3.0] - 2026-10-12

Phase 3: tree, style, layout. The ARCHITECTURE §1 `Counter` renders like its
HTML + Tailwind twin in Chromium.

### Added

- Public packages `tela` (`App`, `Run`, `Node`, `Child`, `UseState`,
  `Previews`, colors, typed style props) and `h` (elements, `Class`, `Style`,
  `Key`, `If`, `Map`, `Fragment`, event props, stored until phase 4).
- `tela snapshot` takes `Preview()` or `Previews` packages, with `--dark` and
  `--preview`; `tela inspect` prints the layout tree as JSON.
- `examples/counter`, compared with Chromium (layout within 0.01 px, pixels
  equal except glyph antialiasing).
- Tailwind CSS v4.3.3 theme generator (`go generate ./internal/style`).
- `internal/style`: `ComputedStyle`, an interned Tailwind class parser with
  arbitrary values, `/NN` opacity, variants and suggestions for unknown
  classes; Preflight element defaults.
- `internal/layout`: block, flexbox, relative and absolute positioning,
  intrinsic sizes, caching and relayout boundaries; all 828 included Taffy
  v0.14.0 fixtures match Chromium.
- `internal/core`: Node/Element/Box trees, keyed reconciliation, `UseState`,
  a frame-on-demand scheduler, paragraphs, headless rendering and CSS box
  painting (borders, radii, shadows, rings, outlines, gradients, filters,
  backdrop filters, transforms, clipping, stacking contexts) matched against
  Chromium.
- `internal/gfx/raster`: `BackdropFilter`.

### Changed

- Colors are extended sRGB until rasterization, clipped where Chromium clips
  them (ADR-046), instead of CSS gamut mapping.
- Deterministic trigonometry and color conversions everywhere pixels are
  computed (`internal/fmath`, ADR-041).

### Fixed

- Cross-architecture differences in `gfx.Rotate`, `Skew`, `HueRotate`,
  `Point.Len`, COLR transforms and color conversions.
- Review fixes: a data race between raster bands on per-pixel backdrop
  filters, a layout cache key collision, class-cache eviction of strings in
  use, a non-atomic snapshot write, previews declared in `_test.go` files.

## [0.2.0] - 2026-10-12

Phase 2: text. Mixed-script paragraphs (Latin, Arabic, CJK, emoji) render
correctly.

### Added

- Embedded fonts: Inter, JetBrains Mono, Noto Sans Arabic and Noto Color
  Emoji (COLRv1).
- `gfx.DrawGlyphRun` and a raster glyph cache (1/4 px subpixel positions).
- `internal/text`: variable font instances, COLRv1 glyphs, CSS font matching
  and per-grapheme fallback, Parley-style paragraph layout (bidi, shaping,
  UAX #14 line breaking, white-space, justification, ellipsis, line clamp,
  inline boxes), cursors, selection, hit-testing and navigation, optional
  system fonts.
- `examples/text`.

### Fixed

- Fused multiply-adds in `gfx.Affine`.
- Review fixes: a hang on malformed cmap ranges, system font scans blocking
  registered fonts, CJK region ranking.

## [0.1.0] - 2026-10-11

Phase 1: pixels on screen.

### Added

- `internal/gfx`: geometry, colors, paths, paints and the immutable
  DisplayList.
- `internal/gfx/raster`: deterministic CPU rasterizer with analytic
  antialiasing, gradients, clips, layers, strokes and dashes, analytic
  rounded-rect shadows, Gaussian blur and parallel bands.
- `internal/platform`: the backend interface, and headless, Wayland and X11
  backends.
- `examples/shapes` and `tela snapshot`.

[Unreleased]: https://github.com/kuronosu/tela/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/kuronosu/tela/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/kuronosu/tela/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/kuronosu/tela/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/kuronosu/tela/releases/tag/v0.1.0
