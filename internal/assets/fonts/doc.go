// Package fonts embeds the fonts Tela ships with, as raw font file bytes.
//
// Snapshots and tests must render the same pixels on every machine, so they
// never touch system fonts (ARCHITECTURE section 11, ADR-008). The fonts
// below are the whole deterministic fallback chain:
//
//   - [InterVariable] and [InterVariableItalic]: the default sans-serif
//     (Inter 4.1, variable: opsz and wght axes).
//   - [JetBrainsMono] and [JetBrainsMonoItalic]: the default monospace font
//     (JetBrains Mono 2.304, variable: wght axis).
//   - [NotoSansArabic]: Arabic script fallback (variable: wdth and wght axes).
//   - [NotoColorEmoji]: color emoji, the COLRv1 build (vector, scalable). The
//     bitmap CBDT build is deliberately not used.
//
// All of them are licensed under the SIL Open Font License 1.1. The license
// texts sit next to the font files in this directory, and the sources,
// versions and checksums are listed in THIRD_PARTY.md at the repository root.
//
// The package is a leaf: it depends on nothing but the standard library and
// exposes only bytes, selected by a [Font] value (see [Data], [Name], [All]).
// Parsing is done by internal/text.
//
// CJK coverage is not embedded (the fonts are too large). Tests that need it
// use a subset font kept in internal/text/testdata/fonts.
package fonts
