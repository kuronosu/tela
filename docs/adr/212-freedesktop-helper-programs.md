# ADR-212: Freedesktop helper programs on Linux

- **Date:** 2026-11-01
- **Status:** Accepted. Amends §2.

## Decision

On Linux, Tela may run two freedesktop helper programs when they are installed, and never fails without them:

- `xdg-open`: used by `OpenURI` and `OpenPath` when there is no portal (phase 12).
- `update-mime-database`: used by `assoc` after it writes a MIME package for an extension no type claims yet (ADR-209).

The other caches the desktop reads are written by Tela itself, in Go. For example, `mimeinfo.cache` is rebuilt byte for byte like `update-desktop-database`. No other external program is run.

## Rationale

§2 limits OS access to protocols, syscalls and D-Bus. GLib 2.90 finds a handler for a MIME type only through `mimeinfo.cache`, so writing that cache is required; Tela writes it itself. The shared-mime-info database is a binary format best left to its own tool. Without the tool, the only cost is that files of a new type are recognized later, once something else runs it.
