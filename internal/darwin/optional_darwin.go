//go:build darwin

package darwin

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

// Frameworks that Load does not open: a backend that misses them still
// works (it paces frames with a timer, or treats every keyboard as ANSI),
// so they are opened on first use and their absence is reported, not
// fatal.
const (
	frameworkCV        = "/System/Library/Frameworks/CoreVideo.framework/CoreVideo"
	frameworkHIToolbox = "/System/Library/Frameworks/Carbon.framework/Frameworks/HIToolbox.framework/HIToolbox"
)

// CoreVideo's display link (CVDisplayLink.h), bound by LoadCoreVideo. The
// API is deprecated since macOS 15 in favor of NSView/NSScreen displayLink
// (macOS 14+), but present; the output callback runs on a thread of
// CoreVideo, not the main thread, with the signature
//
//	CVReturn (*)(CVDisplayLinkRef link, const CVTimeStamp *now,
//	             const CVTimeStamp *outputTime, CVOptionFlags flagsIn,
//	             CVOptionFlags *flagsOut, void *context)
//
// The functions return a CVReturn: 0 (kCVReturnSuccess) on success.
// CVDisplayLinkStop returns after a running callback finished.
var (
	CVDisplayLinkCreateWithActiveCGDisplays func(link *uintptr) int32
	CVDisplayLinkSetOutputCallback          func(link, callback, context uintptr) int32
	CVDisplayLinkSetCurrentCGDisplay        func(link uintptr, display uint32) int32
	CVDisplayLinkStart                      func(link uintptr) int32
	CVDisplayLinkStop                       func(link uintptr) int32
	CVDisplayLinkRelease                    func(link uintptr)
)

var (
	cvOnce sync.Once
	cvErr  error
)

// LoadCoreVideo opens CoreVideo and binds the display link functions, once;
// an error means they are unavailable.
func LoadCoreVideo() error {
	cvOnce.Do(func() {
		cvErr = loadOptional(frameworkCV, func(lib uintptr) {
			bind(&CVDisplayLinkCreateWithActiveCGDisplays, lib, "CVDisplayLinkCreateWithActiveCGDisplays")
			bind(&CVDisplayLinkSetOutputCallback, lib, "CVDisplayLinkSetOutputCallback")
			bind(&CVDisplayLinkSetCurrentCGDisplay, lib, "CVDisplayLinkSetCurrentCGDisplay")
			bind(&CVDisplayLinkStart, lib, "CVDisplayLinkStart")
			bind(&CVDisplayLinkStop, lib, "CVDisplayLinkStop")
			bind(&CVDisplayLinkRelease, lib, "CVDisplayLinkRelease")
		})
	})
	return cvErr
}

var (
	kbOnce          sync.Once
	lmGetKbdType    func() uint8
	kbGetLayoutType func(keyboardType int16) uint32
)

// kKeyboardISO is the PhysicalKeyboardLayoutType of ISO keyboards, the
// four-character code 'ISO ' (HIToolbox/Keyboards.h).
const kKeyboardISO = 0x49534F20

// KeyboardISO reports whether the keyboard used last is an ISO keyboard
// (KBGetLayoutType(LMGetKbdType()) == kKeyboardISO, as SDL asks): on those
// the keyboard driver swaps the virtual key codes of the key left of 1 and
// the key between the left Shift and Z. False when HIToolbox is missing.
// Call it on the main thread.
func KeyboardISO() bool {
	kbOnce.Do(func() {
		if err := loadOptional(frameworkHIToolbox, func(lib uintptr) {
			bind(&lmGetKbdType, lib, "LMGetKbdType")
			bind(&kbGetLayoutType, lib, "KBGetLayoutType")
		}); err != nil {
			lmGetKbdType, kbGetLayoutType = nil, nil
		}
	})
	if lmGetKbdType == nil || kbGetLayoutType == nil {
		return false
	}
	return kbGetLayoutType(int16(lmGetKbdType())) == kKeyboardISO
}

// loadOptional opens the framework at path and runs bindAll on it,
// turning a missing library or symbol into an error.
func loadOptional(path string, bindAll func(lib uintptr)) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("darwin: %v", r)
		}
	}()
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return fmt.Errorf("darwin: opening %s: %w", path, err)
	}
	bindAll(lib)
	return nil
}
