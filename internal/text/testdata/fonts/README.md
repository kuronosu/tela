# Test-only CJK font

`NotoSansJP-Subset.otf` is a subset of Noto Sans CJK JP Regular. It exists only
for tests and goldens of CJK line breaking and mixed-script text. **It is not
embedded in any non-test package**; the production fallback fonts live in
`internal/assets/fonts`.

## Source

- Release: `Sans2.004` of https://github.com/notofonts/noto-cjk
- Archive: `06_NotoSansCJKjp.zip`
  - URL: https://github.com/notofonts/noto-cjk/releases/download/Sans2.004/06_NotoSansCJKjp.zip
  - sha256: `bf5b26feb83ee02533d4eb4a127bd90ed9d42bdd7ca7cb6e4030b2f21ef55cc5`
- File used: `NotoSansCJKjp-Regular.otf`
  - sha256: `68a3fc98800b2a27b371f2fb79991daf3633bd89309d4ffaa6946fd587f375b5`
- License: SIL Open Font License 1.1 (`OFL.txt` in this directory).

Why not `SubsetOTF/JP/NotoSansJP-Regular.otf`: that file only covers the
characters of the Japanese standards, so Simplified-only characters such as
`们`, `这`, `说` are missing. The full Pan-CJK JP file covers them, with the
Japanese glyph forms.

## Contents

The subset keeps:

- ASCII `U+0020-007E`;
- CJK Symbols and Punctuation `U+3000-303F`;
- Hiragana `U+3040-309F` and Katakana `U+30A0-30FF`;
- Halfwidth and Fullwidth Forms `U+FF00-FFEF`;
- every character of `cjk_sample.txt` (about 300 distinct Han characters).

All OpenType layout features (`--layout-features='*'`) and the `name` table are
kept; hinting is dropped. The result is about 280 KB.

`cjk_sample.txt` holds a few paragraphs of natural Japanese and Simplified
Chinese prose, plus one mixed-script paragraph. It is the input of the subset
and can be used as golden text. Adding a character to it requires regenerating
the font.

## Regeneration

`pyftsubset` comes from fontTools. It is a development tool only, never a
project dependency; use a throwaway venv.

```sh
python3 -m venv /tmp/venv && /tmp/venv/bin/pip install fonttools==4.66.1
curl -LO https://github.com/notofonts/noto-cjk/releases/download/Sans2.004/06_NotoSansCJKjp.zip
unzip 06_NotoSansCJKjp.zip NotoSansCJKjp-Regular.otf
/tmp/venv/bin/pyftsubset NotoSansCJKjp-Regular.otf \
  --unicodes='U+0020-007E,U+3000-303F,U+3040-309F,U+30A0-30FF,U+FF00-FFEF' \
  --text-file=cjk_sample.txt \
  --layout-features='*' --name-IDs='*' --name-legacy --name-languages='*' \
  --notdef-outline --glyph-names --no-hinting \
  --output-file=NotoSansJP-Subset.otf
```

Current output sha256:
`cc7759815817d9e0c4b875680bfaa3ee73373bfaa4bffc4661e5f65c6d9747c9`.
