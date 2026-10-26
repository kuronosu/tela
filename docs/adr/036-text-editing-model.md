# ADR-036: Text editing model

- **Date:** 2026-10-12
- **Status:** Accepted

## Decision

Text editing model: a cursor is a source byte offset on a grapheme boundary plus an affinity, canonical (Downstream unless Upstream draws a different caret); one caret per cursor with Parley's bidi rule (no split caret); Chromium's rules for line ends (the position after a forced break or a collapsed wrap space starts the next line), Home/End and selection highlights (a space-width extension across line breaks); visual arrows step one cluster, logical movement is available; word movement uses UAX #29 with macOS/GTK conventions plus the Windows ctrl+right variant; carets in truncated or clamped text sit at the ellipsis

## Rationale

Byte offsets match Go strings; canonical affinity makes cursor equality mean same caret. Checked against Chromium 152: 1,216 carets and 144 selections identical, 9,743 of 9,838 clicks give the same offset and the rest are explained (bidi naming, Chromium's far-end carets, sub-pixel ties). Limitation: a box at a wrapped line's edge has one side without a cursor
