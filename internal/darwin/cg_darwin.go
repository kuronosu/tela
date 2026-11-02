//go:build darwin

package darwin

import (
	"math"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// CoreGraphics constants.
const (
	// CGImageAlphaPremultipliedLast is RGBA with premultiplied alpha, Go's
	// image.RGBA layout (with the default byte order).
	CGImageAlphaPremultipliedLast = 1
	// CGRenderingIntentDefault is kCGRenderingIntentDefault.
	CGRenderingIntentDefault = 0

	// Options of CGWindowListCreateImage.
	CGWindowListOptionIncludingWindow = 1 << 3
	CGWindowImageBoundsIgnoreFraming  = 1 << 0
	CGWindowImageBestResolution       = 1 << 3
)

// CGRectNull is CoreGraphics' null rectangle; CGWindowListCreateImage
// takes it as "the bounds of the windows captured".
var CGRectNull = CGRect{Origin: CGPoint{X: math.Inf(1), Y: math.Inf(1)}}

// CoreGraphics functions, bound by Load.
var (
	CGColorSpaceCreateWithName     func(name uintptr) uintptr
	CGColorSpaceRelease            func(space uintptr)
	CGDataProviderCreateWithCFData func(data uintptr) uintptr
	CGDataProviderRelease          func(provider uintptr)
	CGImageCreate                  func(width, height, bitsPerComponent, bitsPerPixel, bytesPerRow int, space uintptr, bitmapInfo uint32, provider, decode uintptr, shouldInterpolate bool, intent int32) uintptr
	CGImageRelease                 func(image uintptr)
	CGImageGetWidth                func(image uintptr) int
	CGImageGetHeight               func(image uintptr) int
	// CGBitmapContextCreate with nil data makes CoreGraphics allocate the
	// pixels; CGBitmapContextGetData returns them (top row first).
	CGBitmapContextCreate  func(data unsafe.Pointer, width, height, bitsPerComponent, bytesPerRow int, space uintptr, bitmapInfo uint32) uintptr
	CGBitmapContextGetData func(context uintptr) unsafe.Pointer
	CGContextRelease       func(context uintptr)
	CGContextDrawImage     func(context uintptr, rect CGRect, image uintptr)
	CGContextScaleCTM      func(context uintptr, sx, sy CGFloat)
	CGContextTranslateCTM  func(context uintptr, tx, ty CGFloat)
	// CGSessionCopyCurrentDictionary returns nil when the process is not
	// part of a window server (Aqua) session, such as an SSH login.
	CGSessionCopyCurrentDictionary func() uintptr
)

// CGColorSpaceSRGB is the value of kCGColorSpaceSRGB (a CFStringRef), the
// name of the sRGB color space. Set by Load.
var CGColorSpaceSRGB uintptr

var cgLib uintptr

func loadCG(lib uintptr) {
	cgLib = lib
	bind(&CGColorSpaceCreateWithName, lib, "CGColorSpaceCreateWithName")
	bind(&CGColorSpaceRelease, lib, "CGColorSpaceRelease")
	bind(&CGDataProviderCreateWithCFData, lib, "CGDataProviderCreateWithCFData")
	bind(&CGDataProviderRelease, lib, "CGDataProviderRelease")
	bind(&CGImageCreate, lib, "CGImageCreate")
	bind(&CGImageRelease, lib, "CGImageRelease")
	bind(&CGImageGetWidth, lib, "CGImageGetWidth")
	bind(&CGImageGetHeight, lib, "CGImageGetHeight")
	bind(&CGBitmapContextCreate, lib, "CGBitmapContextCreate")
	bind(&CGBitmapContextGetData, lib, "CGBitmapContextGetData")
	bind(&CGContextRelease, lib, "CGContextRelease")
	bind(&CGContextDrawImage, lib, "CGContextDrawImage")
	bind(&CGContextScaleCTM, lib, "CGContextScaleCTM")
	bind(&CGContextTranslateCTM, lib, "CGContextTranslateCTM")
	bind(&CGSessionCopyCurrentDictionary, lib, "CGSessionCopyCurrentDictionary")
}

var (
	windowImageOnce sync.Once
	windowImage     func(bounds CGRect, listOption, windowID, imageOption uint32) uintptr
)

// CGWindowListCreateImage captures windows into a new image (+1), or
// returns 0 when the system refuses or no longer has the function (it is
// deprecated since macOS 14 in favor of ScreenCaptureKit, and capturing
// other applications' windows needs the screen recording permission). It
// is bound on first use so that Load does not depend on it.
func CGWindowListCreateImage(bounds CGRect, listOption, windowID, imageOption uint32) uintptr {
	windowImageOnce.Do(func() {
		if cgLib == 0 {
			return
		}
		if _, err := purego.Dlsym(cgLib, "CGWindowListCreateImage"); err == nil {
			bind(&windowImage, cgLib, "CGWindowListCreateImage")
		}
	})
	if windowImage == nil {
		return 0
	}
	return windowImage(bounds, listOption, windowID, imageOption)
}
