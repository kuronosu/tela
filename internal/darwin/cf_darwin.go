//go:build darwin

package darwin

import "structs"

// CFRunLoopSourceContext is the context of a version 0 run loop source:
// Perform is called on the run loop's thread after CFRunLoopSourceSignal,
// with Info. The other callbacks may be 0.
type CFRunLoopSourceContext struct {
	_               structs.HostLayout
	Version         int
	Info            uintptr
	Retain          uintptr
	Release         uintptr
	CopyDescription uintptr
	Equal           uintptr
	Hash            uintptr
	Schedule        uintptr
	Cancel          uintptr
	Perform         uintptr
}

// CoreFoundation functions, bound by Load. CF objects are uintptr
// (CFTypeRef); allocators are 0 (the default).
var (
	CFRelease func(cf uintptr)

	// CFRunLoopGetMain returns the main thread's run loop; it, the source
	// signal and the wake-up are safe from any thread.
	CFRunLoopGetMain          func() uintptr
	CFRunLoopWakeUp           func(rl uintptr)
	CFRunLoopSourceCreate     func(allocator uintptr, order int, context *CFRunLoopSourceContext) uintptr
	CFRunLoopAddSource        func(rl, source, mode uintptr)
	CFRunLoopSourceSignal     func(source uintptr)
	CFRunLoopSourceInvalidate func(source uintptr)

	// CFDataCreate copies length bytes from bytes.
	CFDataCreate func(allocator uintptr, bytes *byte, length int) uintptr
)

// CFRunLoopCommonModes is the value of kCFRunLoopCommonModes: sources
// added in it also run in the event tracking mode of live resize,
// menus and drags. Set by Load.
var CFRunLoopCommonModes uintptr

func loadCF(lib uintptr) {
	bind(&CFRelease, lib, "CFRelease")
	bind(&CFRunLoopGetMain, lib, "CFRunLoopGetMain")
	bind(&CFRunLoopWakeUp, lib, "CFRunLoopWakeUp")
	bind(&CFRunLoopSourceCreate, lib, "CFRunLoopSourceCreate")
	bind(&CFRunLoopAddSource, lib, "CFRunLoopAddSource")
	bind(&CFRunLoopSourceSignal, lib, "CFRunLoopSourceSignal")
	bind(&CFRunLoopSourceInvalidate, lib, "CFRunLoopSourceInvalidate")
	bind(&CFDataCreate, lib, "CFDataCreate")
}
