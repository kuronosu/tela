# ADR-240: Memory measurement (phase 17)

- **Date:** 2026-11-07
- **Status:** Accepted

## Decision

Memory is measured by one command, `go run ./internal/testutil/memreport`, a development tool outside the public API:

- It runs a `TestMemory` test (skipped unless `TELA_MEM` is set) in `examples/hello`, `kit`, `dashboard` and `notes`, one process per case: each example at its `App` size at x1 and x2, at rest (mounted and settled), and the dashboard through its scripted session (`bench_test.go`).
- Each case reports the Go heap after a GC (`HeapInuse`, `HeapAlloc`), the heap's peak and the bytes allocated during the case, the runtime's `Sys`, and the process's RSS (`VmRSS`, `RssAnon`, `RssFile`, `VmHWM` from `/proc/self/status`; Linux only).
- The live heap is split by subsystem (fonts, framebuffers and present copies, raster scratch, backdrop cache, display lists, glyph cache, images, trees and boxes, other) from `runtime.MemProfile` records at `MemProfileRate = 1`, in a process separate from the RSS one so profiling does not inflate it.
- The baseline is a checked-in file next to the tool; a run prints each number against it, and `-update` rewrites it.

## Why

A test binary mounts an example the way `tela snapshot` does, without a window or compositor, so the numbers repeat across machines; RSS needs a process per case because Go returns freed memory to the OS late. Grouping by subsystem (no pprof library: that would be a new dependency) shows where RAM goes in one table and measures the fonts' share for the Font system's ADR. Real backends keep their own presentation buffers (Wayland shm, X11 images, a DIB, a `CGImage`); the report lists them as computed sizes, since the headless backend's copy stands in for one.
