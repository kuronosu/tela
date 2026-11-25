# ADR-320: The size budget of examples/hello after Canvas

- **Date:** 2026-11-24
- **Status:** Accepted (the owner's choice over cutting what phase 31 added). Amends ADR-290.

## Decision

`sizeBudget` of the gate goes from 22,000,000 to 22,100,000 bytes (examples/hello, linux/amd64). It measures 22,004,784.

What the bytes bought, in every app, used or not: the Porter-Duff operators and tiled images of the CPU rasterizer (phase 31's T1, about 24 KB) and the transitions of `fill`, `stroke` and `stroke-width` (about 13 KB). What was given up: about 37 KB of the binary of every app. The drawing API of Canvas is not among them: `internal/canvas` is linked only by apps that call `h.Canvas`.

## Why

The operators and the pattern paint are cases of switches that every display list goes through, so the linker keeps them. Having Canvas register them was not tried; the gain would be about 0.1% of the binary. The sizes are `-trimpath` builds before and after each task; the gate's own number depends on the checkout's path (a worktree's reads about 60 KB more).

## Changes

`internal/gate/steps.go`.
