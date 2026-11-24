# ADR-315: External textures

- **Date:** 2026-11-23
- **Status:** Accepted (push model, sRGB-only pixels and no native handle: the owner's decisions)

## Decision

`tela.UseTexture()` returns a handle that lives as long as the component; `h.Texture(tex, …)` shows it, a replaced element (300x150 by default, as a `canvas`; `object-fit` and `object-position` from the last frame's size; role `img` when it has a name). The application allocates every buffer and draws with its own graphics API; Tela never exposes its device or a native window handle.

**Frames** are pushed from any goroutine: `SubmitImage(img *image.RGBA, release)`, or `Import(memory)` once per buffer (a `tela.Dmabuf` on Linux, an `IOSurface` on macOS) and `Submit(buf, TextureFrame{Fence, Release})`. Submit never blocks. `Fence` (`Wait(ctx) error`; `tela.SyncFile(fd)`) is waited off the UI thread: Tela never reads a frame being drawn.

**Backpressure: the latest finished frame wins.** A texture holds at most four frames: the shown one, one ready, and two waiting for their fences. A frame that becomes ready replaces the ready one and the older waiting ones. A third waiting frame replaces the newer of the two, so the older always finishes and a fast producer never starves the screen. Replaced frames are released and counted (`Stats`). A producer that must drop nothing paces itself with `NextFrame(ctx)`, which returns at the window's next frame, once no frame waits or is ready, and blocks while the texture is not shown.

**Release** runs exactly once per submitted frame, whatever happens to it (shown and replaced, dropped, an error from Submit, unmount, a lost device, quit), from a goroutine of Tela's, never inside Submit, once neither the CPU nor the GPU reads the buffer. The shown frame stays held until another replaces it. Three buffers are enough without fences, five with them. After unmount, Submit returns `ErrTextureClosed`.

**Sizes:** `OnResize` (UI thread, after layout) and `Size` give the content box in device pixels, with the scale; until a frame of the new size arrives the last one is stretched by `object-fit`. Transforms do not count. Shown by several elements, the largest.

**Pixels:** 8-bit RGB in sRGB, premultiplied, first row at the top: `image.RGBA`, and the fourccs AR24, XR24, AB24, XB24. Anything else is an error that says so. NV12 goes to Audio and video.

**Which device:** `Device()` names it (DRM render node, Vulkan UUID, LUID, Metal registry ID) with the pairs of fourcc and modifier Tela's renderer imports (not the compositor's: Tela composes the frame), also those from another GPU. Without the GPU backend, linear dmabufs and IOSurfaces are mapped and copied. So are the linear buffers of a GPU that is lost; `Submit` refuses its others (`ErrTextureUnsupported`). An IOSurface's format is its OSType (`'BGRA'`, `'RGBA'`; modifier 0); Tela retains it and does not raise its use count. Windows imports wait for a machine to run them on: pixels only.

Not covered: uncapped frame rates, variable refresh, tearing, direct scanout, several textures in one swapchain.

## Why

A pull model makes the window wait for the application. Wayland and X11 windows have no `wl_display` or `xcb_connection` to hand out, so a native handle would exist on two platforms only.

## Changes

ARCHITECTURE §7, §10, §13. `internal/gfx/external.go`.
