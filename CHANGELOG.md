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
