# ADR-244: System fonts by default; embedded fonts are opt-in per app (phase 18)

- **Date:** 2026-11-08
- **Status:** Accepted

## Decision

Amends ADR-008, ADR-031 and ADR-035; §11.

- **No font in the binary by default.** Nothing outside tests imports the font files. An app's text uses the installed system fonts (go-text `fontscan`, as in ADR-035), on unless the program is a test binary.
- **Generic families per OS.** `system-ui`, `ui-sans-serif`, `sans-serif`, `ui-serif`, `serif`, `ui-monospace`, `monospace` and `emoji` resolve to an ordered list of installed families for the running OS (macOS: SF, New York, SF Mono, Apple Color Emoji; Windows: Segoe UI, Cambria, Cascadia Mono or Consolas, Segoe UI Emoji; Linux: the desktop's interface font where it can be read, then fontconfig's usual preferences). The theme's `font-sans`, `font-serif` and `font-mono` become `ui-sans-serif, system-ui, sans-serif`, `ui-serif, serif` and `ui-monospace, monospace`.
- **Opt-in fonts.** `tela.App.Fonts []tela.Font`. `tela.FontData(family, files...)` and `tela.FontFS(fsys, family, patterns...)` make a `Font`; `Sans()`, `Serif()`, `Mono()` and `Emoji()` make it the family of those generic names, and `Scripts(...)` a fallback for scripts. Packages in the core module embed Tela's fonts when imported: `fonts/inter` (`inter.Sans`), `fonts/jetbrainsmono`, `fonts/notoemoji`, `fonts/notoarabic`, a serif (chosen by the owner from rendered candidates), and `fonts/standard` (all of them). App fonts come before system fonts. A file is parsed when text first needs its family.
- **Bitmap color glyphs** (CBDT, sbix PNG) are drawn, scaled from the closest strike, so system emoji fonts work (Noto Color Emoji on most Linux distributions, Apple Color Emoji).
- **Tests stay deterministic.** `telatest`, `tela snapshot` and `tela mcp` use the app's fonts, then `fonts/standard`, never system fonts. Goldens change only where the serif face is drawn.

## Why

The six embedded fonts are 7.2 MB of every binary and 35 MiB of every app's heap, while hello draws only Inter upright. Browsers draw the system's fonts too, and an app that wants a fixed look imports one package.

## What it changes

Apps look like their OS unless they embed a font. Without any installed font (a minimal container), text has no glyphs and the app logs which package to import.
