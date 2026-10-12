# text

The phase-2 showcase: a page of cards with mixed-script paragraphs (Latin with kerning and the `->` ligature, justified right-to-left Arabic with bidi numbers, Japanese and Chinese, color emoji including ZWJ sequences, flags and skin tones), a JetBrains Mono code block with ligatures, `truncate` and `line-clamp-3`, underline and line-through decorations, an inline box on the baseline, and `tabular-nums` against proportional figures. The cards are placed by simple manual geometry (the layout engine arrives in phase 3): one to three columns, depending on the window width.

Every paragraph is shaped once (`text.NewLayout`); a resize only breaks the lines again (`Layout.BreakLines`).

- Run it in a window: `CGO_ENABLED=0 go run ./examples/text` (Wayland or X11; force one with `TELA_BACKEND=wayland|x11`).
- Render it without a window: `CGO_ENABLED=0 go run ./cmd/tela snapshot [--size 960x900] [--scale 2] -o text.png ./examples/text`.
- The goldens are in `testdata/`; regenerate them with `TELA_UPDATE=1 CGO_ENABLED=0 go test ./examples/text`.

## Interaction (window only)

- Press and drag selects text; double click selects a word, triple click a paragraph (a double click and drag extends by words). A selection never leaves the paragraph it started in.
- A single click leaves a caret (drawn without blinking).
- The pointer is an I-beam over text and an arrow elsewhere.
- The wheel scrolls when the page is taller than the window (the page is 894 px tall at 960 px wide; narrower windows make it taller).
- The app asks for a frame only when the selection or the scroll offset changes: no timers, so no CPU while idle.

## Fonts

`Scene` shapes with the default collection only (Inter, JetBrains Mono, Noto Sans Arabic, Noto Color Emoji), so output is identical on every machine. CJK is not embedded (ADR-031):

- the window calls `EnableSystemFonts`, so the OS provides the CJK glyphs (a Noto Sans CJK, Source Han Sans, Yu Gothic, Hiragino...). Without such a font installed the CJK lines draw as missing-glyph boxes;
- the tests register `internal/text/testdata/fonts/NotoSansJP-Subset.otf` in `init`. Because `tela snapshot` compiles the package's tests too, snapshots and goldens get the same font and render the CJK card deterministically. The CJK text only uses characters that subset covers (`TestNoMissingGlyphs` checks it).
