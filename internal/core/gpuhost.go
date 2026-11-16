package core

// The GPU renderer of a Host's windows (ADR-281, ADR-282).
//
// A build with the tag telagpu can draw the frames of its windows on the
// GPU (internal/gfx/gpu) instead of with the CPU rasterizer
// (internal/gfx/raster). Nothing of it is linked without the tag: this
// file holds what both builds share, gpuhost_off.go the Host's hooks of a
// build without the tag (they do nothing, and rasterize is
// Tree.RasterizeFrame), and gpuhost_on.go, gpuframe.go and gpuchain.go the
// GPU path.
//
// # Choosing
//
// Run opens the GPU after the main window exists: the device
// TELA_GPU_DEVICE names, else the one the main window's display server
// composites with, when the window says (its dmabuf feedback names a DRM
// node, and a Vulkan device has it), else the first discrete one
// (gpu.Open). The environment variable TELA_RENDERER chooses: "cpu" keeps
// the CPU rasterizer, "gpu" asks for the GPU whatever the device, and
// nothing at all for the GPU only when the device chosen is a discrete
// one: an integrated or a software device draws a page of glass slower
// than the rasterizer does (ADR-284), so it is used when asked for, and
// otherwise the app is drawn by the CPU rasterizer, without a word
// (a line of the error log when TELA_GPU_DEVICE named the device). When
// no device initializes, the app is drawn by the CPU rasterizer and one
// line of the error log says why. A Host that Run did not start (NewHost:
// tests, telatest, the headless backend) never opens one by itself, so
// headless rendering, goldens, telatest and the tela commands always
// rasterize on the CPU.
//
// # A frame
//
// The device and its Renderer belong to the Host and are shared by its
// windows, child windows included (a device mirrors the textures of one
// Renderer). Each window has a target of its framebuffer's size, made at
// its first GPU frame and again when the size changes, and released with
// the framebuffer when the app hides the window, and when the window is
// minimized or closes.
//
// A frame is Tree.RasterizeFrameGPU (gpuframe.go) in the place of
// Tree.RasterizeFrame: the same damage between the frame the target holds
// and the new one (frameDamage), the scrolled pixels moved on the device
// (gpu.Target.Shift), and each damaged rectangle rendered as a region.
// The target is the frame; how it reaches the window is one of two ways,
// chosen per window and per frame (Host.direct).
//
// Read back: the damaged rectangles are read into the window's
// framebuffer in one submission (and the scrolled pixels moved in it), so
// the framebuffer holds the frame, bit for bit what the target holds, and
// everything after it is as on the CPU path: Window.Present with the same
// damage list, the input mask of a transparent window, the drag images
// cut from the frame, the debug overlays (rasterized by the CPU over a
// copy of the frame read back, as they are over the CPU's frame).
//
// Without a copy (gpuchain.go, ADR-283): the window presents dmabufs
// (platform.DmabufPresenter) and reports a feedback, and the device
// exports images in a format and a modifier the feedback lists
// (gpu.Exporter) and is the device the feedback names. The window has a
// swapchain of exported images, each registered once; a frame takes one
// the OS does not read, copies into it on the device what the frame has
// that the buffer lacks (each buffer keeps its own account: they lag by
// different numbers of frames), waits for the GPU and gives it to the
// window with the frame's damage (PresentDmabuf). Nothing crosses main
// memory. Two buffers at first, a third or a fourth when the OS holds
// that many (an X server that flips needs three), given up again when
// presents stop using them; a frame that finds four busy is not
// presented: its damage is kept and the next frame tries again. A resize,
// a change of the format the feedback prefers, hiding and minimizing
// destroy the buffers (the OS keeps the memory of the frame on screen).
// TELA_PRESENT=copy forces the first way, TELA_PRESENT=dmabuf the second
// also from another device than the display server's; a window that
// could be presented without a copy and is not says why in one line of
// the error log.
//
// A frame a backend asks for. A window whose frames are dmabufs is not
// shown again by its backend alone, which asks for a frame instead
// (platform, "Presenting dmabufs"); nothing tells the Host why a
// FrameEvent came, and a frame presents nothing when nothing changed. So
// the Host notes itself when a frame is owed (viewGPU.owed): the app
// showed the window again, or a present left its buffer free, which only
// a dropped present does (a hidden window); the next FrameEvent then
// presents the whole frame, changed or not. The same flag carries a
// frame that found every buffer busy, with a frame request, since no
// event tells that a buffer was released.
//
// The readers of the framebuffer. Presented without a copy, a frame never
// reaches the framebuffer, which is left behind (GPUSurface.Behind). A
// drag image and Host.FramePixels read the whole target back when they
// are asked (winView.syncPixels). The debug overlays turn the window to
// the first way while they are on: the first frame read back after
// direct ones is drawn and read in full. A transparent window reports no
// feedback (its input mask needs the pixels) and is always read back.
//
// The frame runs on the UI thread and waits for the GPU: for each render,
// each shift, and the read or the copy. What the CPU rasterizer keeps
// between frames (the cache of filtered backdrops, the snapshots that
// damaged rectangles start from: ADR-200, ADR-275) is not used: the GPU
// path resets it when it takes a tree over, and the frame a rasterizer
// draws after the other one is a full frame (layerState.rasGPU).
//
// # Fallback
//
// An error of the GPU (the device lost, a failed allocation, something
// larger than the device's textures: gpu.Result.Unsupported) never costs a
// frame: RasterizeFrameGPU draws that frame in full with the CPU
// rasterizer and returns the error. The Host then draws that window with
// the CPU rasterizer for good, or every window when the device is lost,
// which it closes, and writes one line to the error log.
//
// Presenting without a copy falls back to reading back, for that window
// and for good, with one line of the log: a buffer the display server
// does not import (CreateDmabuf: a driver whose modifiers it lists but
// whose memory it cannot use), an image the device cannot export, a copy
// or a present that fails. The frame is read back whole and presented
// with Window.Present at once, so it is not lost either; when the device
// is gone the CPU rasterizer draws it.
//
// # Memory at rest
//
// The Renderer's memory (the atlases, the image textures, the offscreen
// targets, the buffers of uploads and readbacks) is trimmed RasterSettle
// after the last frame of a window drawn by the GPU, when all of them
// rest, by the Host's timer like the rasterizer's scratch memory
// (ADR-241), and the swapchains give up the buffers beyond two. The
// windows' targets keep their pixels: the trim asks for no frame.

// EnvRenderer is the environment variable that chooses the renderer of
// the windows' frames in a build with the tag telagpu: "cpu" or "gpu"
// (ADR-281). Unset, the GPU is used when a device initializes.
const EnvRenderer = "TELA_RENDERER"
