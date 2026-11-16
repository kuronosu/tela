package core

// The GPU renderer of a Host's windows (ADR-281, ADR-282).
//
// A build with the tag telagpu can draw the frames of its windows on the
// GPU (internal/gfx/gpu) instead of with the CPU rasterizer
// (internal/gfx/raster). Nothing of it is linked without the tag: this
// file holds what both builds share, gpuhost_off.go the Host's hooks of a
// build without the tag (they do nothing, and rasterize is
// Tree.RasterizeFrame), and gpuhost_on.go and gpuframe.go the GPU path.
//
// # Choosing
//
// Run opens the GPU after the main window exists: the device
// TELA_GPU_DEVICE names, else the first discrete one (gpu.Open). The
// environment variable TELA_RENDERER chooses: "cpu" keeps the CPU
// rasterizer, "gpu" and nothing at all ask for the GPU. When no device
// initializes, the app is drawn by the CPU rasterizer and one line of the
// error log says why. A Host that Run did not start (NewHost: tests,
// telatest, the headless backend) never opens one by itself, so headless
// rendering, goldens, telatest and the tela commands always rasterize on
// the CPU.
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
// (gpu.Target.Shift) and in the framebuffer, each damaged rectangle
// rendered as a region, and the rectangles read back into the window's
// framebuffer in one submission. The framebuffer then holds the frame,
// bit for bit what the target holds, and everything after it is as on the
// CPU path: Window.Present with the same damage list, the input mask of a
// transparent window, the drag images cut from the frame, the debug
// overlays (rasterized by the CPU over a copy of the frame read back, as
// they are over the CPU's frame).
//
// The frame runs on the UI thread and waits for the GPU: for each render,
// each shift and the read. What the CPU rasterizer keeps between frames
// (the cache of filtered backdrops, the snapshots that damaged rectangles
// start from: ADR-200, ADR-275) is not used: the GPU path resets it when it
// takes a tree over, and the frame a rasterizer draws after the other one
// is a full frame (layerState.rasGPU).
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
// # Memory at rest
//
// The Renderer's memory (the atlases, the image textures, the offscreen
// targets, the buffers of uploads and readbacks) is trimmed RasterSettle
// after the last frame of a window drawn by the GPU, when all of them
// rest, by the Host's timer like the rasterizer's scratch memory
// (ADR-241). The windows' targets keep their pixels: the trim asks for no
// frame.

// EnvRenderer is the environment variable that chooses the renderer of
// the windows' frames in a build with the tag telagpu: "cpu" or "gpu"
// (ADR-281). Unset, the GPU is used when a device initializes.
const EnvRenderer = "TELA_RENDERER"
