# ADR-262: Stores, the watcher and platform gaps

- **Date:** 2026-11-09
- **Status:** Accepted. Amends ADR-253 and ADR-259 (no contract or protocol change).

## Decision

- **Stores** compare versions of the file by content (SHA-256), not stat. On Linux and macOS the write exchanges its file with the old one (`RENAME_EXCHANGE`, `RENAME_SWAP`; `RENAME_NOREPLACE`, `RENAME_EXCL` when there was none), reads what it replaced and merges a version it did not read. Windows (no atomic exchange) and file systems without the flags keep ADR-259's check before the rename.
- **Watcher:** kqueue polls the files past the descriptor limit; a flat watch reports a subdirectory that goes with `dir` true everywhere.
- **`Launch.AtStart`:** core marks a `LaunchOpened` held until the main window's first commit. A start's file on Windows and Linux is on the command line (`LaunchStart`); an app started hidden gets no mark.
- **By design:** no roles on Linux (the spec has none) or Windows (an `edit` verb is an action, not a role), `Role` reports `HandlerDefault`; Launch Services cannot clear a user's default.
- **X11:** after one missed wait for `_NET_FRAME_EXTENTS`, later windows map at once with the last extents of their kind (dialog or not).
- **Win32** message box padding is 16 dialog units (`MapDialogRect`).

## Why

Only an atomic exchange closes the gap without locks other programs ignore.
