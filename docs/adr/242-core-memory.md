# ADR-242: Decoded images released when unshown; exact framebuffers

- **Date:** 2026-11-07
- **Status:** Accepted

## Decision

Amends ADR-079 and ADR-098 (their caches) and the host's framebuffer (phase 17). Details in `internal/core/assets.go` ("Releasing images") and `host.go`.

- **Images.** One cache (`assets.slots`) holds every decoded source (`url()` paths, `SrcBytes`, `SrcFS`, icons, `tela.BgImage` values) through a `weak.Pointer`. The holders are what shows it: the box of an `Img` or `Icon`, the layers whose lists paint a background or mask (`layer.uses`) and the frame that composited a deferred layer's mask (`frame.uses`). The images looked up last are also held strongly up to 4 MiB (`keepBytes`, least recently used out; a larger image is never held there). An image nothing holds is collected; its slot is removed by a `runtime.AddCleanup`. A lookup while it lives returns the same image; afterwards the source is decoded again into the same pixels. Failures stay cached. The 256 MiB LRUs and the process-lifetime path cache are gone.
- **Framebuffer.** The first framebuffer of a window is exact. A resize still reuses memory up to twice the size and grows by a quarter, and the host trims the buffer to its size (a copy, same pixels) once the size has been stable for 500 ms (`fbSettle`, on the host timer). A window the app hides drops its framebuffer; the next frame rasterizes in full.

## Why

`memreport` (ADR-240): the dashboard kept both 8.8 MiB wallpapers after a theme switch, and every window paid 25 % framebuffer headroom (4 MiB at 2560x1600, 8 MiB at 4K). Weak references follow what is shown without reference counts that every path must balance.
