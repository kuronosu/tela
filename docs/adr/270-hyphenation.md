# ADR-270: Hyphenation: soft hyphens always, dictionaries opt-in

- **Date:** 2026-11-11
- **Status:** Accepted. Amends ADR-166 and ADR-244; §5, §11.

## Decision

- **`hyphens-none`, `hyphens-manual` (initial), `hyphens-auto`**, inherited. A soft hyphen (U+00AD) breaks unless `none`; the line ends with U+2010 (else U+002D), unspaced, underlined with the text; an ellipsis replaces it.
- **`hyphens-auto`** breaks the word that overflows a line as Blink with Minikin: Liang's algorithm, 5 to 62 letters, 2 before the point and 3 after, no capitalized English word, no last word of a paragraph, Blink's locale map; also in min-content and `text-balance`/`text-pretty` (hyphen penalty).
- **Opt-in dictionaries (owner)**: `tela.Hyphenation`, `App.Hyphenation`, `tela.HyphenationPatterns` over TeX patterns; `hyphen/en`, `hyphen/es`, `hyphen/fr` import only `tela` and `internal/assets/hyphen`. Tests use the app's only.
- **Data**: Chromium's TeX patterns, compressed by `go generate`; a package adds to a binary 54 KB (en), 16 (es), 12 (fr).

## Why

Against Chromium 155, 45 paragraphs break alike and 70 000 words per dictionary hyphenate as its `.hyb` files.

## What it changes

A line broken at a soft hyphen now shows a hyphen.
