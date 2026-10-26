# ADR-027: Floating-point code in `gfx` and `raster` is fusion-safe

- **Date:** 2026-10-11
- **Status:** Accepted

## Decision

Floating-point code in `gfx` and `raster` is fusion-safe: products are rounded explicitly and transcendental functions come from `internal/gfx/internal/fmath` (ports of Go's `math`); arm64 under qemu is the check

## Rationale

Go may fuse multiply-add on arm64 and other architectures, which changes pixels; principle 4 requires the same pixels everywhere
