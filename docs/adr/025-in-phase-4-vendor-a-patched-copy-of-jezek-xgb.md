# ADR-025: In phase 4, vendor a patched copy of `jezek/xgb`

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

In phase 4, vendor a patched copy of `jezek/xgb` (core, `xproto`, `bigreq`; BSD-3, listed in `THIRD_PARTY.md`) whose reader handles GenericEvents

## Rationale

xgb v1.3.1 reads every event as 32 bytes, so XInput2 and Present GenericEvents would desync the connection; the patch is about 10 lines, far cheaper than our own X11 client. If more xgb limits appear, an own client generated from xcb-proto XML (like ADR-019) replaces it
