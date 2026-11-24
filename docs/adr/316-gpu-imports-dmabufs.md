# ADR-316: The GPU renderer imports dmabufs

- **Date:** 2026-11-23
- **Status:** Accepted. Implements ADR-315's GPU path on Linux.

## Decision

An optional device interface, `gpu.Importer`: `DeviceID`, `ImportFormats` (pairs of fourcc and modifier, `own` and `foreign`) and `Import`, once per buffer. The `*Imported` is the `gfx.External` of the images that show the buffer: drawn from the memory, without uploads or budget.

- **Ownership:** each `Target.Draw` takes the buffer from the foreign queue family (general layout) and gives it back in one submission it waits for. After Draw the GPU reads nothing; after `ErrDeviceLost`, once `Device.Close` returned.
- **Sampled or copied:** linear rows the device does not sample as they lie, or that are behind a discrete GPU's bus, are copied on the GPU once per frame. Everything else is sampled where it lies.
- **A wrong descriptor is an error, never a lost device:** it is checked against the dmabuf, and the driver's account of the image against it; on i915 the pages of a foreign dmabuf are asked for first.
- **Goroutines:** `Import` and `Imported.Close` run where the application calls them, beside the device's own goroutine, and wait for a `Draw` in progress only. Handing them to the UI thread would deadlock with an `OnResize` that waits for the goroutine that imports. What the device does not take wraps `ErrImportUnsupported`, and core maps it when it is linear.
- **Streams:** an image of `gfx.NewStreamImage` gives up the textures of those before it, and a frame's new images take them.

## Why

NVIDIA 615 rounds strides up, ignores offsets (which loses the device) and samples some strides wrong, without errors; NVIDIA's video memory on i915 hangs Mesa 17 s and loses the device (its system memory works). 1280x720 per frame: RTX 3060 Ti 0.10 ms from its own memory, 0.45 from the UHD 630's (1.0 sampled over the bus); UHD 630 1.6 ms from either. A stream of pixels made a `VkImage` per frame and held 214 MiB; now one texture, RTX 2.6 to 0.7 ms, UHD 7.1 to 4.1.

## Changes

ARCHITECTURE §13. `internal/vk/dmabuf_import*.go`, `internal/gfx/gpu/import.go` and `vulkan_import_linux.go`, `gfx.NewStreamImage`.
