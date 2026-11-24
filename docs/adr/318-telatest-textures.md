# ADR-318: telatest drives textures

- **Date:** 2026-11-23
- **Status:** Accepted. Implements the testing of ADR-315.

## Decision

- **The test is the producer.** `App.Texture(l)` returns the `tela.Texture` an element shows; `SubmitTexture(l, img)` submits a frame and settles, so it is on screen when it returns; `TextureSize` and `TextureStats` report the size (device pixels) and the counters. It works on an element whose component starts no producer.
- **A `NextFrame` loop never keeps settling busy.** Settling and `Flush` run frames that leave the waiting goroutines waiting (`core.TextureHooks.HoldFrames`); a frame asked only for them is not work (`Tree.OnlyTextureFrames`), as with endless animations. `Advance` and the waits release them at every frame; `StepTexture(l)` runs one frame and waits, in real time, until the producers waiting for it wait again, so a loop advances deterministically.
- **`Pending` kinds:** `PendingNextFrame` (goroutines in `NextFrame` of a texture on screen: counted by `ExpectIdle`, as an endless animation) and `PendingFence` (frames waiting for fences or copies: not counted, as `PendingGo`).
- **Fake device:** `FakeTextureDevice` (`WithTextureDevice`; `Texture.Device` returns its description; `Import` takes the formats it lists) makes `FakeTextureBuffer`s whose pixels the test sets, and `FakeFence` the test signals; settling waits, in real time, for the frame that follows a signal. `ExpectReleases(held)` counts the Release functions given to `Submit`; the end of the test fails for one never called or called twice.

## Why

A frame submitted from a goroutine, or after a fence, arrives in real time; a golden must not depend on it. A producer paced by the window would ask for a frame after every frame, so settling could not end.

## Changes

`telatest` (`texture.go`, `PendingKind`s), ARCHITECTURE §17; `internal/core/texture_testhook.go` (hooks, fake importer, accessors, called only by `telatest`, so examples/hello does not link them), three lines in `texture.go`, `texture_tree.go`, `pending.go` and `host.go`; `internal/testutil/idle` skips `PendingFence`.
