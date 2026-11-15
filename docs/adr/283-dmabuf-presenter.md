# ADR-283: Platform contract v13, windows that present dmabufs

- **Date:** 2026-11-15
- **Status:** Accepted. Implements the zero-copy path of ADR-281 on Wayland.

## Decision

`internal/platform` gets an optional interface, `DmabufPresenter`, that a window may implement (found by a type assertion, like `AlphaPresenter`). Only Wayland windows do, over `zwp_linux_dmabuf_v1`. `Present` is unchanged and always works.

- `DmabufFeedback()` reports the OS's main device (major and minor of one of its DRM nodes) and the accepted pairs of DRM fourcc and modifier, preferred first, with a `Serial` that changes only with the content. No event announces a change; the caller compares the serial once a frame.
- `CreateDmabuf(DmabufDescriptor)` registers a buffer (size, fourcc, modifier, planes of fd, offset and stride, `Alpha`) and waits for the OS to import it. The descriptors stay the caller's. An error means `Present` for that window.
- `PresentDmabuf(buf, damage)` follows `Present`'s rules (size, damage in buffer pixels, the first present after a configure is full and shows the window, a hidden window drops it) without a copy: the buffer holds the whole frame, already rendered.
- `DmabufBuffer.Busy()` says whether the OS still reads a buffer. The caller asks when it needs one; no event is delivered. `Destroy` is allowed while busy.
- The two kinds of present alternate on one window; the first of a kind is full. A window hidden and shown is mapped by its next present, for which the backend requests a frame.
- A `Transparent` window reports no feedback: its input mask (ADR-249) needs the pixels, so its renderer reads back and uses `Present`. Buffers with `Alpha` are refused.

Wayland registers with `create` and a round trip, not `create_immed`: a failed import is then a `failed` event, where wlroots, KWin and Mutter answer `create_immed` with a fatal protocol error. What is fatal either way (a format the feedback does not list, planes outside their dmabuf) is checked before the request. The global is bound at its first use.

## Rationale

The platform layer knows neither `gfx` nor Vulkan, so the contract speaks in file descriptors and DRM codes. Asking instead of events keeps the sealed event set, and fits a renderer that needs a free buffer at a frame, not at a release. A registration is paid once per swapchain, so a round trip there costs less than an app killed by a buffer it could have shown through `wl_shm`. Measured on Hyprland 0.56 with an RTX 3060 Ti as main device: 570 pairs, buffers released as the next commit replaces them, two buffers enough.

## Changes

`internal/platform/dmabuf.go` and `doc.go` ("Presenting dmabufs"); `internal/platform/wayland/dmabuf.go` and the vendored `linux-dmabuf-v1.xml`; `internal/vk/dmabuf.go` (the export side). ARCHITECTURE §15 should list contract v13 and `zwp_linux_dmabuf_v1` in the Wayland row.
