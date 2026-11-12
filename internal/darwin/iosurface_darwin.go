//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// frameworkIOSurface is IOSurface.framework (ADR-278): shared pixel
// buffers that a CALayer shows as its contents without a copy.
const frameworkIOSurface = "/System/Library/Frameworks/IOSurface.framework/IOSurface"

// IOSurface constants (IOSurfaceRef.h, IOSurfaceTypes.h).
const (
	// IOSurfaceLockReadOnly is kIOSurfaceLockReadOnly: the caller only
	// reads while the surface is locked.
	IOSurfaceLockReadOnly = 1
	// IOSurfacePixelFormatBGRA is the four-character code 'BGRA'
	// (kCVPixelFormatType_32BGRA): bytes blue, green, red, alpha in
	// memory, the format CoreAnimation shows natively.
	IOSurfacePixelFormatBGRA = 0x42475241
)

// IOSurface functions (IOSurfaceRef.h), bound by LoadIOSurface. A surface
// is an IOSurfaceRef (a CFTypeRef, toll-free bridged with the IOSurface
// class): IOSurfaceCreate returns it +1, CFRelease releases it. The CPU
// may touch its memory (IOSurfaceGetBaseAddress, rows
// IOSurfaceGetBytesPerRow bytes apart) only between IOSurfaceLock and
// IOSurfaceUnlock with the same options; both return a kern_return_t, 0
// on success, and take a seed pointer that may be nil. Every argument is
// pointer-sized or passed in a register, so none goes on the stack (see
// the package rules).
var (
	// IOSurfaceRef IOSurfaceCreate(CFDictionaryRef properties)
	IOSurfaceCreate func(properties ID) uintptr
	// kern_return_t IOSurfaceLock(IOSurfaceRef buffer, IOSurfaceLockOptions options, uint32_t *seed)
	IOSurfaceLock func(surface uintptr, options uint32, seed *uint32) int32
	// kern_return_t IOSurfaceUnlock(IOSurfaceRef buffer, IOSurfaceLockOptions options, uint32_t *seed)
	IOSurfaceUnlock func(surface uintptr, options uint32, seed *uint32) int32
	// void *IOSurfaceGetBaseAddress(IOSurfaceRef buffer)
	IOSurfaceGetBaseAddress func(surface uintptr) unsafe.Pointer
	// size_t IOSurfaceGetBytesPerRow(IOSurfaceRef buffer)
	IOSurfaceGetBytesPerRow func(surface uintptr) uint
	// size_t IOSurfaceGetWidth(IOSurfaceRef buffer), IOSurfaceGetHeight
	IOSurfaceGetWidth  func(surface uintptr) uint
	IOSurfaceGetHeight func(surface uintptr) uint
	// Boolean IOSurfaceIsInUse(IOSurfaceRef buffer): the window server
	// (or another process) still uses the surface.
	IOSurfaceIsInUse func(surface uintptr) bool
	// void IOSurfaceSetValue(IOSurfaceRef buffer, CFStringRef key, CFTypeRef value)
	IOSurfaceSetValue func(surface uintptr, key, value ID)
	// size_t IOSurfaceAlignProperty(CFStringRef property, size_t value)
	IOSurfaceAlignProperty func(property ID, value uint) uint
)

// The property keys of IOSurfaceCreate (CFStringRef constants of the
// framework), set by LoadIOSurface.
var (
	ioSurfaceWidth           ID // kIOSurfaceWidth
	ioSurfaceHeight          ID // kIOSurfaceHeight
	ioSurfaceBytesPerElement ID // kIOSurfaceBytesPerElement
	ioSurfaceBytesPerRow     ID // kIOSurfaceBytesPerRow
	ioSurfaceAllocSize       ID // kIOSurfaceAllocSize
	ioSurfacePixelFormat     ID // kIOSurfacePixelFormat
)

var (
	ioSurfaceOnce sync.Once
	ioSurfaceErr  error
)

// LoadIOSurface opens IOSurface.framework and binds its functions, once;
// an error means they are unavailable. Call it after Load.
func LoadIOSurface() error {
	ioSurfaceOnce.Do(func() {
		ioSurfaceErr = loadOptional(frameworkIOSurface, func(lib uintptr) {
			bind(&IOSurfaceCreate, lib, "IOSurfaceCreate")
			bind(&IOSurfaceLock, lib, "IOSurfaceLock")
			bind(&IOSurfaceUnlock, lib, "IOSurfaceUnlock")
			bind(&IOSurfaceGetBaseAddress, lib, "IOSurfaceGetBaseAddress")
			bind(&IOSurfaceGetBytesPerRow, lib, "IOSurfaceGetBytesPerRow")
			bind(&IOSurfaceGetWidth, lib, "IOSurfaceGetWidth")
			bind(&IOSurfaceGetHeight, lib, "IOSurfaceGetHeight")
			bind(&IOSurfaceIsInUse, lib, "IOSurfaceIsInUse")
			bind(&IOSurfaceSetValue, lib, "IOSurfaceSetValue")
			bind(&IOSurfaceAlignProperty, lib, "IOSurfaceAlignProperty")
			ioSurfaceWidth = ID(symbolValue(lib, "kIOSurfaceWidth"))
			ioSurfaceHeight = ID(symbolValue(lib, "kIOSurfaceHeight"))
			ioSurfaceBytesPerElement = ID(symbolValue(lib, "kIOSurfaceBytesPerElement"))
			ioSurfaceBytesPerRow = ID(symbolValue(lib, "kIOSurfaceBytesPerRow"))
			ioSurfaceAllocSize = ID(symbolValue(lib, "kIOSurfaceAllocSize"))
			ioSurfacePixelFormat = ID(symbolValue(lib, "kIOSurfacePixelFormat"))
		})
	})
	return ioSurfaceErr
}

// NewIOSurfaceBGRA returns a new IOSurface (+1: CFRelease it) of width by
// height pixels, four bytes per pixel in IOSurfacePixelFormatBGRA, with
// rows and size aligned by IOSurfaceAlignProperty (as Mozilla's
// MacIOSurface asks), tagged with the color space named colorSpace (a
// CFStringRef such as kCGColorSpaceSRGB; 0 for none). The surface's own
// IOSurfaceGetBytesPerRow is the stride to use, whatever was asked.
// Call it inside an autorelease pool, after LoadIOSurface succeeded.
func NewIOSurfaceBGRA(width, height int, colorSpace uintptr) (uintptr, error) {
	if IOSurfaceCreate == nil {
		return 0, errors.New("darwin: IOSurface is not loaded")
	}
	if width <= 0 || height <= 0 {
		return 0, fmt.Errorf("darwin: an IOSurface of %dx%d pixels", width, height)
	}
	bpr := IOSurfaceAlignProperty(ioSurfaceBytesPerRow, uint(width)*4)
	size := IOSurfaceAlignProperty(ioSurfaceAllocSize, bpr*uint(height))
	props := NewDictionary()
	DictionarySet(props, ioSurfaceWidth, NSNumberInt(width))
	DictionarySet(props, ioSurfaceHeight, NSNumberInt(height))
	DictionarySet(props, ioSurfaceBytesPerElement, NSNumberInt(4))
	DictionarySet(props, ioSurfaceBytesPerRow, NSNumberInt(int(bpr)))
	DictionarySet(props, ioSurfaceAllocSize, NSNumberInt(int(size)))
	DictionarySet(props, ioSurfacePixelFormat, NSNumberInt(IOSurfacePixelFormatBGRA))
	s := IOSurfaceCreate(props)
	if s == 0 {
		return 0, fmt.Errorf("darwin: IOSurfaceCreate of %dx%d BGRA pixels (%d bytes per row) failed", width, height, bpr)
	}
	if colorSpace != 0 {
		// The key Chromium and Firefox set (CFSTR("IOSurfaceColorSpace")):
		// CoreAnimation color-matches the surface from this space, as it
		// does a CGImage from its own.
		IOSurfaceSetValue(s, AutoString("IOSurfaceColorSpace"), ID(colorSpace))
	}
	return s, nil
}
