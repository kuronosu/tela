# ADR-035: System fonts are opt-in per `Collection`

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

System fonts are opt-in per `Collection` (`EnableSystemFonts`), refused under `go test` unless `TELA_SYSTEM_FONTS=1`, and come after the embedded fonts in the fallback order (extends ADR-031). Discovery uses go-text `fontscan` lazily (first unknown family or uncovered cluster); only the matched face is parsed, through the same code as embedded fonts. Fallback ranks the families supporting a script once per script (sans designs first, then the `Lang` region for CJK, then fewest characters, then name) so one script draws in one family

## Rationale

Embedded fonts keep default rendering identical on every machine; system fonts only fill gaps (CJK, Hebrew, Indic, Thai). Per-cluster ranking let narrow fonts take single clusters of a script
