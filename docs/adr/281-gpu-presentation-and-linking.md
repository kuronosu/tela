# ADR-281: GPU presentation and linking

- **Date:** 2026-11-15
- **Status:** Accepted

## Decision

**Opt-in by build tag.** The GPU backend is compiled only with `-tags telagpu`. Without the tag nothing changes: the CPU rasterizer, and on Linux a static binary. With it, `TELA_RENDERER=cpu|gpu` chooses at run time; the default is the GPU when a device initializes, else the CPU rasterizer with one log line. Headless rendering, goldens and `telatest` always use the CPU rasterizer.

**Loading.** Vulkan is loaded at run time: `libvulkan.so.1` through purego on Linux, `vulkan-1.dll` through `syscall` on Windows. macOS uses Metal through purego. The bindings are a hand-written subset in the leaf `internal/vk`; the renderer is `internal/gfx/gpu`, beside `raster`.

**Presenting on Linux, without WSI.** Vulkan's Wayland and XCB surfaces need libwayland and libxcb objects, which Tela does not have. The renderer draws into images it owns and presents them in one of two ways:

1. *Readback:* the damage is copied into the window's `*image.RGBA` and presented with `Window.Present`, as today. It works on every backend and is the fallback, the path of tests, and the path of transparent windows (their input mask reads the frame).
2. *Zero copy:* the images are exported as dmabufs (`VK_EXT_external_memory_dma_buf`, `VK_EXT_image_drm_format_modifier`) and given to an optional window capability of the platform contract: `zwp_linux_dmabuf_v1` on Wayland, DRI3 `PixmapFromBuffers` with Present on X11. The platform sees file descriptors, a fourcc, a modifier and planes, never Vulkan or `gfx`. The renderer waits for the frame's fence before it hands a buffer over (NVIDIA has no implicit sync); explicit sync can follow.

The device is the compositor's main device when it reports one, else the first discrete one; `TELA_GPU_DEVICE` overrides. Windows presents through a `VK_KHR_win32_surface` swapchain on the HWND and macOS through a `CAMetalLayer`.

**Shaders** are GLSL sources compiled to SPIR-V by `go generate` (glslang, a development tool); the `.spv` files are checked in and embedded.

## Rationale

Measured on 2026-11-15: a `CGO_ENABLED=0` Linux binary that imports purego is dynamically linked (`libc.so.6`, `libdl.so.2`, `libpthread.so.0`, the `ld-linux` interpreter) and creates its threads through pthread. It does not start on a musl system, and Tela promises one static binary. GPU drivers are shared libraries, so no static way exists; the cost is paid only by apps that ask for it. A tag changes no public API; an opt-in package (as `fonts/…`) stays possible later. Both of the owner's devices (RTX 3060 Ti, UHD 630) export dmabufs with modifiers.

## Changes

ARCHITECTURE §2 (purego line), §5 (`internal/vk`, `internal/gfx/gpu`), §13 (GPU backend). The renderer's design and the tolerance against the CPU goldens get their own ADR.
