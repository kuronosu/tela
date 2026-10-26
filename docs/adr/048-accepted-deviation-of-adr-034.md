# ADR-048: Accepted deviation of ADR-034

- **Date:** 2026-10-12
- **Status:** Superseded by ADR-159 (in part or in full; see it)

## Decision

Accepted deviation of ADR-034: at scale 2 Blink rounds line metrics in device pixels, so text baselines may differ from Chromium by one device pixel (0.5 logical px); Tela keeps quantizing line boxes in logical pixels

## Rationale

Same layout at every scale and in `tela inspect`; the difference is measured (§1 Counter: heading +1, button text -1 device px at x2; all box edges within 0.01 px) and invisible at normal viewing
