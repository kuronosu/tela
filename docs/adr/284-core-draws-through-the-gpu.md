# ADR-284: Core draws windows through the GPU

- **Date:** 2026-11-15
- **Status:** Accepted

## Decision

With the build tag `telagpu`, the Host opens one GPU device in `Run` and one `gpu.Renderer`, shared by every window; `NewHost` alone opens none, so headless rendering, `telatest` and the `tela` commands stay on the CPU. Each window has a surface with a target of the framebuffer's size, released with the framebuffer when the window is hidden, and when it is minimized or closed.

A frame (`Tree.RasterizeFrameGPU`) takes the rectangles and shifts of the frame's damage, without the CPU's plans, snapshots and backdrop cache, with a merge policy of its own (a rectangle costs the pixels it draws and reads). It shifts and renders them on the GPU and reads them back into the framebuffer, which `Window.Present` shows as before. So everything that reads the framebuffer still works: input masks, drag images, debug overlays.

**Fallback.** Any GPU error draws the same frame in full with the CPU rasterizer and leaves that window on the CPU, with one log line; a lost device moves every window. The first frame of either rasterizer after the other is full.

**Default device.** Under the tag the GPU is the default only when the chosen device is a discrete GPU. An integrated or software device is used when `TELA_RENDERER=gpu` asks for it.

The tolerance constants of ADR-282 live in `internal/gfx/gfxtest`, shared by the comparisons of `gpu`, `core` and the examples.

## Rationale

Measured on the dashboard's session at 3840x2160 x2: 134 of 731 frames over 16.7 ms on the CPU, 55 on the RTX 3060 Ti, 444 on the UHD 630, whose fill rate (1.6 Gpx/s) is a tenth of the RTX's. An integrated GPU is slower than the rasterizer on glass-heavy pages, so it must not be the default (amends ADR-281). Reading back keeps the platform contract and every reader of the framebuffer unchanged; presenting without a copy (ADR-283) removes the readback and the backend's copy, about 21 ms of a full 4K frame.

## Changes

ARCHITECTURE §13. `internal/core/gpuhost*.go`, `gpuframe.go`; `gpu.Target.Shift`, `ReadPixels` of several rectangles, `ErrDeviceLost`.
