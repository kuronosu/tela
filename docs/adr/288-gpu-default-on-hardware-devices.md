# ADR-288: The GPU is the default on every hardware device

- **Date:** 2026-11-16
- **Status:** Accepted. Amends ADR-284 and ADR-285 (the owner's decision).

## Decision

Under `telagpu` the GPU draws the windows by default on every hardware device, integrated ones included. Only a software device (`VK_PHYSICAL_DEVICE_TYPE_CPU`: lavapipe, llvmpipe, SwiftShader) still needs `TELA_RENDERER=gpu`; without it the CPU rasterizer draws, silently (one line when `TELA_GPU_DEVICE` named the device). `gpu.Config.DiscreteOnly` and `ErrNotDiscrete` become `HardwareOnly` and `ErrSoftwareDevice`.

The device choice stays: `TELA_GPU_DEVICE`, else the device of the main window's dmabuf feedback, else the first discrete device, else the first integrated one (`vk.SelectDevice` now prefers an integrated device to a software one listed before it).

## Why

ADR-284 kept integrated devices off because the UHD 630 was slower than the rasterizer. After ADR-287 it is faster: the dashboard's session at 2880x1800 x2 has 99 of 704 frames over 16.7 ms on the CPU and 25–26 on the UHD 630. A software device rasterizes on the CPU through a shader interpreter and stays slower than Tela's own rasterizer.

## Changes

ARCHITECTURE §13. `internal/gfx/gpu` (`Config`, `selectDevice`), `internal/vk/select.go`, core's `openGPU`; `TestSelectDevice`, `TestGPURendererChoice`.
