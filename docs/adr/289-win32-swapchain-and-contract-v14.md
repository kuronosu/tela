# ADR-289: Windows presents GPU frames through a swapchain on the window

- **Date:** 2026-11-16
- **Status:** Accepted. Implements ADR-281's Windows path; amends ADR-284, ADR-285.

## Decision

**Contract v14.** An optional `platform.Win32SurfacePresenter` on Win32 windows, found by a type assertion like `DmabufPresenter`: `Win32Surface()` returns the HINSTANCE and HWND, the client size the swapchain must have (empty when minimized) and `Full` (the next present must be whole: WM_PAINT, a reconnect, a show, a Present before); `SurfacePresented(damage)` tells the window of each present, the first of which shows it. While frames come that way the window paints nothing from its stale copy and asks for a frame instead. Transparent (layered), client-area backdrop and TitleBarOverlay windows report no surface and keep `Present`. The caller destroys its swapchain before a `Present` and before closing the window.

**Renderer.** `gpu.WindowPresenter` (the Vulkan device on Windows; the instance enables `VK_KHR_surface` and `VK_KHR_win32_surface`, the device `VK_KHR_swapchain` and `VK_KHR_incremental_present`) makes a `gpu.Swapchain`: FIFO, B8G8R8A8_UNORM (else R8G8B8A8_UNORM) in SRGB_NONLINEAR, the surface's minimum image count plus one, opaque alpha. Images are acquired without waiting; each keeps an account of what it lacks and gets only that from the target, presented with incremental rectangles. Out of date and suboptimal remake it; no image free or another size retries at the next frame; a zero extent presents nothing.

**Core.** A Win32 window takes this path instead of readback; any error closes the swapchain and reads back for good (ADR-285's rule), as do debug overlays. Minimized windows keep their swapchain, hidden ones do not.

## Why

Readback and `SetDIBitsToDevice` cost a copy through main memory per frame. Pacing stays the backend's `DwmFlush`: one frame per refresh never fills FIFO's queue, and a non-blocking acquire keeps the UI thread out of the presentation engine. None of this was run on Windows yet.

## Changes

ARCHITECTURE §13, §15. `internal/platform/win32surface.go`, `win32/surface.go`; `internal/vk/surface.go`; `internal/gfx/gpu/swapchain.go`, `vulkan_swapchain_windows.go`; `internal/core/gpuwin.go`; the live test `live_gpu_windows_test.go`.
