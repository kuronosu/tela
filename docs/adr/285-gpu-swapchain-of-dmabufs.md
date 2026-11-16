# ADR-285: The GPU path presents dmabufs from a swapchain

- **Date:** 2026-11-16
- **Status:** Accepted

## Decision

Under `telagpu`, a window that implements `platform.DmabufPresenter` is presented without a copy. Its persistent target stays the canonical frame, so core's damage logic does not change. A swapchain of exported images (`gpu.Exporter`: transfer-only images, so every modifier a device exports can be used) is registered once. Every frame's damage is added to an account per buffer; a present takes the free buffer with the least to copy, copies its account from the target, waits for the GPU, and calls `PresentDmabuf` with the damage since the last present. Two buffers to start, up to four when the server keeps them busy (Xwayland flips need three); with all busy the frame waits for the next one.

**Path per window.** Zero copy needs feedback from the window, a format and explicit modifier both sides accept, and the renderer on the device the feedback names. Otherwise, and on any failure, the window uses readback (ADR-284) for good, with one log line. Transparent windows and windows with a debug overlay use readback. `TELA_PRESENT=copy|dmabuf` forces a path. Readers of the framebuffer (drag images, `Host.FramePixels`) read the target back on demand.

**Device.** `TELA_GPU_DEVICE`, else the device of the main window's feedback, else the first discrete one; a device that is not discrete is used only with `TELA_RENDERER=gpu` (ADR-284).

**Owed presents.** Core presents in full on the next frame, even when nothing changed, after the app shows a window again and after a present that left its buffer free (a dropped one, by the contract). The contract is unchanged.

## Rationale

Measured on the dashboard's session at 2880x1800 x2 on the RTX 3060 Ti: 100 of 704 frames over 16.7 ms on the CPU, 28 with readback, 6 without a copy (p95 26.2, 15.7 and 9.4 ms). The copy into a buffer costs 0.16 ms a frame, where readback and the backend's copy cost 13 ms of a full frame. Keeping one canonical target makes buffers that lag by different numbers of frames a matter of copying, not of rendering.

## Changes

ARCHITECTURE §13. `internal/core/gpuchain.go`, `internal/gfx/gpu/export.go`; live tests on Xwayland compare the server's pixels with the CPU rasterizer.
