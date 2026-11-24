package tela

import "github.com/kuronosu/tela/internal/core"

// Texture is a handle on content that the application renders itself, with
// the graphics API it chooses (a 3D view, a decoded video, a map, an
// emulator's screen): the application pushes frames to it, and h.Texture
// shows the latest one as an element. [UseTexture] returns one; h.Texture
// has an example.
//
// The application allocates every buffer and draws on its own device; Tela
// never exposes its own. Pixels are 8-bit RGB in sRGB, with premultiplied
// alpha, the first row at the top.
//
// Its methods are safe from any goroutine:
//
//   - SubmitImage shows an *image.RGBA, the pixels of a frame drawn by the
//     CPU.
//   - Import takes memory of the application's, a [Dmabuf] or an
//     [IOSurface], once per buffer, and Submit shows it as a frame: sampled by the GPU without a
//     copy where Tela draws with one, copied otherwise. Submit's
//     documentation says exactly what happens to a frame that arrives while
//     another is pending: the latest finished frame wins.
//   - Size and OnResize give the size in device pixels that a frame should
//     have.
//   - NextFrame blocks until the window starts its next frame, to render at
//     its pace.
//   - Device says which device to render on, and Stats counts the frames.
//
// A texture whose component unmounted is closed: its methods return
// [ErrTextureClosed].
type Texture = core.Texture

// TextureBuffer is memory of the application's imported by a [Texture]
// (Texture.Import), to be shown by Texture.Submit any number of times. Close
// gives back what Tela holds of it, once no frame of it is held.
type TextureBuffer = core.TextureBuffer

// TextureFrame describes a frame given to Texture.Submit: the [Fence] that
// tells when its pixels are finished, nil if they are, and Release, called
// exactly once when Tela does not read the buffer any more.
type TextureFrame = core.TextureFrame

// TextureMemory is memory that Texture.Import imports: a [Dmabuf] on Linux,
// an [IOSurface] on macOS. No other type implements it.
type TextureMemory = core.TextureMemory

// Dmabuf is memory shared by a Linux dma-buf, to import into a [Texture]:
// its size, its DRM fourcc (AR24, XR24, AB24 or XB24), its DRM format
// modifier and its planes. Texture.Device lists the formats the device that
// draws the windows takes.
//
//	buf, err := tex.Import(tela.Dmabuf{
//	    Width: 1280, Height: 720,
//	    Fourcc: 0x34325241, // AR24, DRM_FORMAT_ARGB8888
//	    Planes: []tela.DmabufPlane{{FD: fd, Stride: 1280 * 4}},
//	})
type Dmabuf = core.Dmabuf

// DmabufPlane is a memory plane of a [Dmabuf]: the descriptor of the dma-buf
// that holds it (which stays the application's: Import keeps a duplicate),
// where it starts in it, and the bytes from a row to the next.
type DmabufPlane = core.DmabufPlane

// IOSurface is memory shared by a macOS IOSurface, to import into a
// [Texture]: its IOSurfaceRef, with any binding of the application's. Its
// size and pixel format, 'BGRA' or 'RGBA' (a Metal texture of BGRA8Unorm or
// RGBA8Unorm, _sRGB or not), are read from it, and its pixels are
// premultiplied sRGB, whatever color space it is tagged with:
//
//	buf, err := tex.Import(tela.IOSurface{Surface: ref})
//
// Import keeps a reference of its own (CFRetain) until the buffer is freed;
// the application's stays the application's. The use count that
// IOSurfaceIsInUse reads is not raised, so a pool that recycles by it must
// keep the surface out (a CVPixelBuffer retained) until the frame's Release.
// Tela draws with the CPU on macOS: each frame is copied, with the surface
// locked for reading, on a goroutine of Tela's.
//
// The [Fence] of a frame waits for the Metal work that draws it, to its
// completion (that it was scheduled is not enough). A channel filled by the
// completed handler of its command buffer does:
//
//	// metalFence gets what addCompletedHandler: was told: the error of the
//	// command buffer, nil when it finished. make(metalFence, 1)
//	type metalFence chan error
//
//	func (f metalFence) Wait(ctx context.Context) error {
//	    select {
//	    case err := <-f:
//	        return err
//	    case <-ctx.Done():
//	        return ctx.Err()
//	    }
//	}
//
// So does an MTLSharedEvent, with the block of an MTLSharedEventListener or a
// loop of waitUntilSignaledValue:timeoutMS: whose short timeout leaves time
// to look at ctx.
//
// On other systems Import returns an error that wraps
// [ErrTextureUnsupported].
type IOSurface = core.IOSurface

// Fence tells when the pixels of a frame given to Texture.Submit are
// finished: Wait blocks until they are, or returns an error, which drops the
// frame. Tela waits on a goroutine of its own and never reads the pixels
// before. [SyncFile] is the fence of a Linux sync file; any other wait of
// the application's graphics API (a Vulkan fence or timeline semaphore) is a
// type with that method:
//
//	// vkFence waits for a fence of the application's device.
//	type vkFence struct {
//	    dev   vk.Device
//	    fence vk.Fence
//	}
//
//	func (f vkFence) Wait(ctx context.Context) error {
//	    for ctx.Err() == nil {
//	        if done, err := f.dev.WaitFence(f.fence, 100*time.Millisecond); done || err != nil {
//	            return err
//	        }
//	    }
//	    return ctx.Err()
//	}
type Fence = core.Fence

// SyncFile returns the [Fence] of a Linux sync file, the descriptor that
// VK_KHR_external_fence_fd or EGL_ANDROID_native_fence_sync exports: it
// signals when the GPU work it stands for ends, and fails when that work
// failed. A negative fd is work that already ended, as in Vulkan.
//
// It takes the descriptor over: it is closed, once, when the wait ends, or
// when the frame the fence was submitted with is dropped or refused. Use the
// fence for one frame. On other systems its Wait returns an error that wraps
// [ErrTextureUnsupported].
func SyncFile(fd int) Fence { return core.SyncFile(fd) }

// TextureSize is the size a [Texture] is shown at (Texture.Size,
// Texture.OnResize): the content box of its element in device pixels,
// snapped to them as it is painted, so that a frame of that size is drawn
// pixel for pixel, and the display scale. Transforms do not count. The zero
// TextureSize means that no element shows it.
type TextureSize = core.TextureSize

// TextureStats counts the frames of a [Texture] (Texture.Stats): submitted,
// shown, dropped, and how old the last one was when it was presented.
type TextureStats = core.TextureStats

// TextureDevice describes the device that draws the app's windows
// (Texture.Device): which one it is, for the application to render on the
// same one, whether it imports memory, and the formats it takes.
type TextureDevice = core.TextureDevice

// TextureFormat is a pixel format of imported memory: a DRM fourcc and a DRM
// format modifier on Linux, the pixel format of an [IOSurface] on macOS.
type TextureFormat = core.TextureFormat

// FrameTime tells when a window's frame is shown (Texture.NextFrame).
type FrameTime = core.FrameTime

var (
	// ErrTextureClosed is returned by the methods of a [Texture] whose
	// component unmounted, or whose app quit.
	ErrTextureClosed = core.ErrTextureClosed
	// ErrTextureUnsupported is wrapped by the errors of Texture.Import for
	// memory, formats and modifiers that cannot be shown, and returned by
	// the fence of [SyncFile] where sync files do not exist.
	ErrTextureUnsupported = core.ErrTextureUnsupported
)

// UseTexture returns a [Texture], the same handle on every build, to show
// with h.Texture. It is closed when the component unmounts. A reload of tela
// dev makes a new one, whose frames start again.
//
// Push frames to it from an effect or a goroutine. This one renders at the
// window's frame rate, and sleeps while the texture is not on screen:
//
//	tex := tela.UseTexture()
//	tela.UseGo(func(ctx context.Context) {
//	    for {
//	        ft, err := tex.NextFrame(ctx)
//	        if err != nil {
//	            return
//	        }
//	        size := tex.Size()
//	        tex.SubmitImage(render(size.Width, size.Height, ft.Time), nil)
//	    }
//	}, tela.Once)
//	return Texture(tex, Class("w-full h-64 rounded-xl"), AriaLabel("Globe"))
func UseTexture() *Texture { return core.UseTexture() }
