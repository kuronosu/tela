# ADR-064: Wayland clipboard over `wl_data_device`

- **Date:** 2026-10-13
- **Status:** Accepted

## Decision

Wayland clipboard over `wl_data_device` (core protocol, manager version ≤ 3; no primary selection, no drag and drop). `WriteText` offers `text/plain;charset=utf-8`, `text/plain`, `UTF8_STRING`, `TEXT`, `STRING` (ISO 8859-1) plus a private `application/x-tela-source-<pid>-<n>` type naming the source, with the seat's newest input serial; a selection event carrying our latest source's type confirms ownership and any other ends it, and our own selection is read from memory. Transfers are non-blocking pipes serviced by the poll loop on the UI thread (no goroutines), ended after 5 s without progress (a read reports an error); received text follows GTK's MIME preference, declared UTF-8 replaces invalid sequences with U+FFFD, undeclared encodings fall back to ISO 8859-1

## Rationale

Compositors refuse `set_selection` silently (no focus, old serial), so GTK's "ignore selections while our source lives" can answer pastes with stale text; the marker costs one ignored MIME type. Poll-driven I/O keeps the loop single-threaded like key repeat, and a peer that never reads or writes cannot pin a goroutine
