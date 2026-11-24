//go:build darwin

package darwin

import (
	"errors"
	"sync"
)

// The IOSurface functions that read a surface the application made
// (IOSurfaceRef.h), besides those of iosurface_darwin.go, and the
// CoreFoundation ones that hold it: bound by LoadIOSurfaceImport, which
// needs no Load, since frames are read without a backend too (a headless
// app). All of them are plain C functions, safe from any thread.
var (
	// size_t IOSurfaceGetAllocSize(IOSurfaceRef buffer)
	IOSurfaceGetAllocSize func(surface uintptr) uint
	// size_t IOSurfaceGetBytesPerElement(IOSurfaceRef buffer)
	IOSurfaceGetBytesPerElement func(surface uintptr) uint
	// size_t IOSurfaceGetPlaneCount(IOSurfaceRef buffer): 0 for a surface
	// that is not planar.
	IOSurfaceGetPlaneCount func(surface uintptr) uint
	// OSType IOSurfaceGetPixelFormat(IOSurfaceRef buffer)
	IOSurfaceGetPixelFormat func(surface uintptr) uint32

	ioSurfaceGetTypeID func() uint              // CFTypeID IOSurfaceGetTypeID(void)
	ioSurfaceCFTypeID  func(cf uintptr) uint    // CFTypeID CFGetTypeID(CFTypeRef cf)
	ioSurfaceCFRetain  func(cf uintptr) uintptr // CFTypeRef CFRetain(CFTypeRef cf)
	ioSurfaceCFRelease func(cf uintptr)         // void CFRelease(CFTypeRef cf)
)

var (
	ioSurfaceImportOnce sync.Once
	ioSurfaceImportErr  error
)

// LoadIOSurfaceImport opens IOSurface.framework and CoreFoundation and binds
// the functions that read a surface of the application's, those of
// LoadIOSurface included, once; an error means they are unavailable.
func LoadIOSurfaceImport() error {
	ioSurfaceImportOnce.Do(func() {
		if ioSurfaceImportErr = LoadIOSurface(); ioSurfaceImportErr != nil {
			return
		}
		ioSurfaceImportErr = loadOptional(frameworkIOSurface, func(lib uintptr) {
			bind(&IOSurfaceGetAllocSize, lib, "IOSurfaceGetAllocSize")
			bind(&IOSurfaceGetBytesPerElement, lib, "IOSurfaceGetBytesPerElement")
			bind(&IOSurfaceGetPlaneCount, lib, "IOSurfaceGetPlaneCount")
			bind(&IOSurfaceGetPixelFormat, lib, "IOSurfaceGetPixelFormat")
			bind(&ioSurfaceGetTypeID, lib, "IOSurfaceGetTypeID")
		})
		if ioSurfaceImportErr != nil {
			return
		}
		ioSurfaceImportErr = loadOptional(frameworkCF, func(lib uintptr) {
			bind(&ioSurfaceCFTypeID, lib, "CFGetTypeID")
			bind(&ioSurfaceCFRetain, lib, "CFRetain")
			bind(&ioSurfaceCFRelease, lib, "CFRelease")
		})
	})
	return ioSurfaceImportErr
}

// RetainIOSurface takes a reference (CFRetain) to surface, which the caller
// holds meanwhile, to give back with ReleaseIOSurface. An error means that
// it is another CoreFoundation object; what is not one (a pointer already
// freed) crashes. LoadIOSurfaceImport succeeded before.
func RetainIOSurface(surface uintptr) error {
	if surface == 0 {
		return errors.New("the IOSurfaceRef is NULL")
	}
	if ioSurfaceCFTypeID(surface) != ioSurfaceGetTypeID() {
		return errors.New("the reference is not an IOSurfaceRef (that of a CVPixelBuffer is CVPixelBufferGetIOSurface, that of a Metal texture its iosurface property)")
	}
	ioSurfaceCFRetain(surface)
	return nil
}

// ReleaseIOSurface gives back a reference of RetainIOSurface.
func ReleaseIOSurface(surface uintptr) { ioSurfaceCFRelease(surface) }
