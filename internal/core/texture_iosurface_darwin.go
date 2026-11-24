package core

import (
	"fmt"
	"unsafe"

	"github.com/kuronosu/tela/internal/darwin"
)

// metalRegistryID is TextureDevice.MetalRegistryID.
func metalRegistryID() uint64 { return darwin.MainDisplayMetalRegistryID() }

// mapIOSurface returns the copy of s, and the size of s. Tests replace it.
var mapIOSurface = func(s IOSurface) (mappedTexture, int, int, error) {
	if err := darwin.LoadIOSurfaceImport(); err != nil {
		return nil, 0, 0, fmt.Errorf("tela: Texture.Import: an IOSurface: %v: %w", err, ErrTextureUnsupported)
	}
	if err := darwin.RetainIOSurface(s.Surface); err != nil {
		return nil, 0, 0, fmt.Errorf("tela: Texture.Import: an IOSurface: %w", err)
	}
	m, err := newMappedSurface(surfaceRef(s.Surface), s.Opaque)
	if err != nil {
		return nil, 0, 0, err
	}
	return m, int(m.l.width), int(m.l.height), nil
}

// surfaceRef is an IOSurfaceRef held by a reference of its own. It
// implements surfaceMemory.
type surfaceRef uintptr

func (s surfaceRef) layout() surfaceLayout {
	r := uintptr(s)
	return surfaceLayout{
		format:  darwin.IOSurfaceGetPixelFormat(r),
		planes:  darwin.IOSurfaceGetPlaneCount(r),
		width:   darwin.IOSurfaceGetWidth(r),
		height:  darwin.IOSurfaceGetHeight(r),
		element: darwin.IOSurfaceGetBytesPerElement(r),
		stride:  darwin.IOSurfaceGetBytesPerRow(r),
		alloc:   darwin.IOSurfaceGetAllocSize(r),
	}
}

func (s surfaceRef) lock() error {
	if kr := darwin.IOSurfaceLock(uintptr(s), darwin.IOSurfaceLockReadOnly, nil); kr != 0 {
		return fmt.Errorf("IOSurfaceLock failed (kern_return_t %#x)", uint32(kr))
	}
	return nil
}

func (s surfaceRef) base() unsafe.Pointer { return darwin.IOSurfaceGetBaseAddress(uintptr(s)) }

// unlock ends a read: nothing was written, so its result changes nothing.
func (s surfaceRef) unlock() {
	darwin.IOSurfaceUnlock(uintptr(s), darwin.IOSurfaceLockReadOnly, nil)
}

func (s surfaceRef) release() { darwin.ReleaseIOSurface(uintptr(s)) }
