package core

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
	"runtime"
	"unsafe"
)

// IOSurfaces (ADR-315): no GPU of Tela's draws on macOS, so every frame is
// copied, as a mapped dmabuf is, on the goroutine that waited for its fence.
//
// The system's calls are in texture_iosurface_darwin.go; what decides about
// a surface and copies it is here, for the tests of every system.

// IOSurface is memory shared by a macOS IOSurface (Texture.Import).
type IOSurface struct {
	// Surface is the IOSurfaceRef (an IOSurface object is one too). It
	// stays the application's, which holds it during Import: Import takes a
	// reference of its own (CFRetain), and keeps it until the buffer is
	// closed and no frame of it is held. It does not raise the use count
	// (IOSurfaceIsInUse): TextureFrame.Release tells when the surface may be
	// drawn into again.
	Surface uintptr
	// Opaque says that the alpha of its pixels means nothing, as in a
	// Dmabuf's XR24: every pixel is opaque.
	Opaque bool
}

func (IOSurface) textureMemory() {}

// The pixel formats of IOSurfaces (OSTypes) that textures take.
const (
	surfaceBGRA = 'B'<<24 | 'G'<<16 | 'R'<<8 | 'A' // kCVPixelFormatType_32BGRA
	surfaceRGBA = 'R'<<24 | 'G'<<16 | 'B'<<8 | 'A' // kCVPixelFormatType_32RGBA
)

// surfaceFormats are the formats Import takes on macOS (Texture.Device).
func surfaceFormats() []TextureFormat {
	return []TextureFormat{{Fourcc: surfaceBGRA}, {Fourcc: surfaceRGBA}}
}

// osTypeString returns the four characters of t, which an OSType packs the
// first one highest, unlike a DRM fourcc.
func osTypeString(t uint32) string {
	if s := fourccString(bits.ReverseBytes32(t)); len(s) == 4 {
		return "'" + s + "'"
	}
	return fmt.Sprintf("0x%08x", t)
}

// surfaceLayout is what a surface says of itself.
type surfaceLayout struct {
	format        uint32 // an OSType
	planes        uint   // 0 for one that is not planar
	width, height uint
	element       uint // bytes per element
	stride        uint // bytes per row
	alloc         uint // the bytes of its memory
}

// check reports why a surface of layout l cannot be shown. With nil, reading
// extent bytes from its base address stays inside its memory.
func (l surfaceLayout) check() error {
	var err error
	switch {
	case l.format != surfaceBGRA && l.format != surfaceRGBA:
		err = fmt.Errorf("textures take 8-bit RGB in sRGB, the pixel formats 'BGRA' and 'RGBA': %w", ErrTextureUnsupported)
	case l.planes != 0 || l.element != 4:
		err = fmt.Errorf("it has %d planes and %d bytes per element; textures take surfaces that are not planar, of 4 bytes per pixel: %w", l.planes, l.element, ErrTextureUnsupported)
	case l.width == 0 || l.height == 0 || l.width > texMaxSide || l.height > texMaxSide:
		err = fmt.Errorf("its width and height must be 1 to %d", texMaxSide)
	case l.stride < 4*l.width:
		err = fmt.Errorf("%d bytes per row for rows of %d", l.stride, 4*l.width)
	case l.alloc > math.MaxInt || l.alloc < 4*l.width || (l.alloc-4*l.width)/l.stride < l.height-1:
		err = fmt.Errorf("%d rows %d bytes apart end beyond its memory, which has %d bytes", l.height, l.stride, l.alloc)
	default:
		return nil
	}
	return fmt.Errorf("tela: Texture.Import: a %dx%d IOSurface, %s: %w", l.width, l.height, osTypeString(l.format), err)
}

// extent returns the bytes from the first pixel of a checked layout to the
// end of its last one.
func (l surfaceLayout) extent() int {
	return int(l.stride)*int(l.height-1) + 4*int(l.width)
}

// surfaceMemory is a surface that a buffer holds until release. Its memory
// may be read between a lock that returned nil and unlock.
type surfaceMemory interface {
	layout() surfaceLayout
	lock() error
	base() unsafe.Pointer
	unlock()
	release()
}

// mappedSurface copies the frames of a surface.
type mappedSurface struct {
	mem    surfaceMemory
	l      surfaceLayout
	opaque bool
}

// newMappedSurface returns the copy of the surface mem, which it releases
// when it returns an error.
func newMappedSurface(mem surfaceMemory, opaque bool) (*mappedSurface, error) {
	l := mem.layout()
	if err := l.check(); err != nil {
		mem.release()
		return nil, err
	}
	return &mappedSurface{mem: mem, l: l, opaque: opaque}, nil
}

// read implements mappedTexture. The lock shows the CPU what the GPU wrote
// (a discrete one's memory is read back).
func (m *mappedSurface) read(dst []byte) error {
	// Nothing says that a thread may unlock what another one locked.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := m.mem.lock(); err != nil {
		return err
	}
	defer m.mem.unlock()
	p := m.mem.base()
	if p == nil {
		return errors.New("an IOSurface has no base address: the CPU cannot read it")
	}
	src := unsafe.Slice((*byte)(p), m.l.extent())
	return copyMapped(dst, "an IOSurface", src, int(m.l.width), int(m.l.height), int(m.l.stride), m.l.format == surfaceBGRA, m.opaque)
}

// close implements mappedTexture.
func (m *mappedSurface) close() { m.mem.release() }

// importSurface is Import of an IOSurface.
func (x *Texture) importSurface(s IOSurface) (*TextureBuffer, error) {
	if x.isClosed() {
		return nil, ErrTextureClosed
	}
	if s.Surface == 0 {
		return nil, errors.New("tela: Texture.Import: the Surface of the IOSurface is 0 (NULL); pass its IOSurfaceRef")
	}
	mem, w, h, err := mapIOSurface(s)
	if err != nil {
		return nil, err
	}
	return x.addBuffer(&TextureBuffer{x: x, w: w, h: h, opaque: s.Opaque, mem: mem})
}
