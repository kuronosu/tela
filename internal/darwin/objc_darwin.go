//go:build darwin

package darwin

import (
	"fmt"
	"runtime"
	"structs"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Objective-C runtime types, shared with purego's objc package so that
// values pass between the two without conversions.
type (
	// ID is an Objective-C object pointer (nil is 0).
	ID = objc.ID
	// SEL is a registered selector.
	SEL = objc.SEL
	// Class is an Objective-C class.
	Class = objc.Class
)

// CGFloat is CoreGraphics' floating-point type on 64-bit systems.
type CGFloat = float64

// CGPoint, CGSize and CGRect are the CoreGraphics geometry structures
// (NSPoint, NSSize and NSRect on 64-bit macOS), passed and returned by
// value.
type (
	CGPoint struct {
		_    structs.HostLayout
		X, Y CGFloat
	}
	CGSize struct {
		_             structs.HostLayout
		Width, Height CGFloat
	}
	CGRect struct {
		_      structs.HostLayout
		Origin CGPoint
		Size   CGSize
	}
)

// Typed views of objc_msgSend, bound once by Load. A message send is a
// plain C call with the method's own signature after (self, _cmd); purego
// expands the variadic arguments into registers like fixed ones.
var (
	sendBool  func(id ID, sel SEL, args ...any) bool
	sendFloat func(id ID, sel SEL, args ...any) float64
	sendInt   func(id ID, sel SEL, args ...any) int
	sendUint  func(id ID, sel SEL, args ...any) uint64
	sendRect  func(id ID, sel SEL, args ...any) CGRect
	sendPoint func(id ID, sel SEL, args ...any) CGPoint
	sendRange func(id ID, sel SEL, args ...any) NSRange
	sendCStr  func(id ID, sel SEL, args ...any) string

	poolPush      func() uintptr
	poolPop       func(pool uintptr)
	pthreadMainNP func() int32
)

func loadObjC(lib, system uintptr) error {
	send, err := purego.Dlsym(lib, "objc_msgSend")
	if err != nil {
		return fmt.Errorf("darwin: looking up objc_msgSend: %w", err)
	}
	purego.RegisterFunc(&sendBool, send)
	purego.RegisterFunc(&sendFloat, send)
	purego.RegisterFunc(&sendInt, send)
	purego.RegisterFunc(&sendUint, send)
	// CGPoint (two doubles) comes back in registers on both
	// architectures: objc_msgSend.
	purego.RegisterFunc(&sendPoint, send)
	// NSRange (two NSUIntegers) too.
	purego.RegisterFunc(&sendRange, send)
	purego.RegisterFunc(&sendCStr, send)
	// x86-64 returns structures larger than 16 bytes through memory, and
	// the runtime has a separate entry point for those sends (arm64 returns
	// CGRect, four doubles, in registers through objc_msgSend).
	rectSend := send
	if runtime.GOARCH == "amd64" {
		if rectSend, err = purego.Dlsym(lib, "objc_msgSend_stret"); err != nil {
			return fmt.Errorf("darwin: looking up objc_msgSend_stret: %w", err)
		}
	}
	purego.RegisterFunc(&sendRect, rectSend)
	superSend, err := purego.Dlsym(lib, "objc_msgSendSuper")
	if err != nil {
		return fmt.Errorf("darwin: looking up objc_msgSendSuper: %w", err)
	}
	purego.RegisterFunc(&msgSendSuper, superSend)
	bind(&selGetName, lib, "sel_getName")
	bind(&poolPush, lib, "objc_autoreleasePoolPush")
	bind(&poolPop, lib, "objc_autoreleasePoolPop")
	bind(&pthreadMainNP, system, "pthread_main_np")
	return nil
}

// Sel returns the selector called name. It takes the runtime's lock:
// cache the result in a package variable.
func Sel(name string) SEL { return objc.RegisterName(name) }

// GetClass returns the class called name, or 0 if it is not loaded.
func GetClass(name string) Class { return objc.GetClass(name) }

// Send sends sel to id with args and returns the result as an object (or
// any integer that fits a register; 0 for void methods).
func Send(id ID, sel SEL, args ...any) ID { return id.Send(sel, args...) }

// SendClass sends sel to the class c (a class method).
func SendClass(c Class, sel SEL, args ...any) ID { return ID(c).Send(sel, args...) }

// SendBool sends sel and returns a BOOL result.
func SendBool(id ID, sel SEL, args ...any) bool { return sendBool(id, sel, args...) }

// SendFloat sends sel and returns a double (CGFloat, NSTimeInterval)
// result.
func SendFloat(id ID, sel SEL, args ...any) float64 { return sendFloat(id, sel, args...) }

// SendInt sends sel and returns an NSInteger result.
func SendInt(id ID, sel SEL, args ...any) int { return sendInt(id, sel, args...) }

// SendUint sends sel and returns an NSUInteger result (an NSEventType,
// modifier flags, a phase).
func SendUint(id ID, sel SEL, args ...any) uint64 { return sendUint(id, sel, args...) }

// SendRect sends sel and returns a CGRect (NSRect) result.
func SendRect(id ID, sel SEL, args ...any) CGRect { return sendRect(id, sel, args...) }

// SendPoint sends sel and returns a CGPoint (NSPoint) result.
func SendPoint(id ID, sel SEL, args ...any) CGPoint { return sendPoint(id, sel, args...) }

// SendRange sends sel and returns an NSRange result.
func SendRange(id ID, sel SEL, args ...any) NSRange { return sendRange(id, sel, args...) }

// objcSuper is the runtime's struct objc_super: the receiver and the class
// where the method lookup starts.
type objcSuper struct {
	_          structs.HostLayout
	receiver   ID
	superClass Class
}

var msgSendSuper func(super *objcSuper, sel SEL, args ...any) ID

// SendSuper sends sel to id, starting the method lookup at the class
// super: an overridden method calling the implementation it overrides
// passes its class's superclass. The class is explicit, not read from id,
// because the runtime may change an object's class under it (key-value
// observing subclasses NSApp's class, and a lookup from the object's class
// would call the override again, forever).
func SendSuper(id ID, super Class, sel SEL, args ...any) ID {
	return msgSendSuper(&objcSuper{receiver: id, superClass: super}, sel, args...)
}

// Method is one method of a class registered with RegisterClass: its
// selector and a Go function taking (self ID, _cmd SEL, arguments...).
type Method struct {
	Sel SEL
	Fn  any
}

// RegisterClass creates and registers the class name, a subclass of super
// conforming to the named protocols, with methods. A class name exists
// once per process: register each class once (and give it a unique
// prefix). Each method costs one purego callback.
func RegisterClass(name string, super Class, protocols []string, methods []Method) (Class, error) {
	var ps []*objc.Protocol
	for _, p := range protocols {
		proto := objc.GetProtocol(p)
		if proto == nil {
			return 0, fmt.Errorf("darwin: unknown protocol %s", p)
		}
		ps = append(ps, proto)
	}
	defs := make([]objc.MethodDef, len(methods))
	for i, m := range methods {
		defs[i] = objc.MethodDef{Cmd: m.Sel, Fn: m.Fn}
	}
	return objc.RegisterClass(name, super, ps, nil, defs)
}

// PoolPush starts an autorelease pool and returns its token for PoolPop.
// Pools nest and must be popped in reverse order on the same thread.
func PoolPush() uintptr { return poolPush() }

// PoolPop releases the objects autoreleased since PoolPush returned pool.
func PoolPop(pool uintptr) { poolPop(pool) }

// IsMainThread reports whether the calling thread is the process's main
// thread, the only one allowed to use AppKit.
func IsMainThread() bool { return pthreadMainNP() != 0 }

var (
	selAlloc              = objc.RegisterName("alloc")
	selInitWithUTF8String = objc.RegisterName("initWithUTF8String:")
	selRelease            = objc.RegisterName("release")
	selRetain             = objc.RegisterName("retain")
	classNSString         Class
)

// NSString returns a new NSString (+1: Release it) holding s. Invalid UTF-8
// becomes U+FFFD and NUL characters, which would end the C string early,
// become U+FFFD too: initWithUTF8String: returns nil for invalid input,
// and a nil title or string argument raises an exception in AppKit.
func NSString(s string) ID {
	if classNSString == 0 {
		classNSString = objc.GetClass("NSString")
	}
	return ID(classNSString).Send(selAlloc).Send(selInitWithUTF8String, CString(s))
}

var selUTF8String = objc.RegisterName("UTF8String")

// GoString returns the contents of the NSString s ("" for nil).
func GoString(s ID) string {
	if s == 0 {
		return ""
	}
	return sendCStr(s, selUTF8String)
}

// Release sends release to id (no-op for nil).
func Release(id ID) {
	if id != 0 {
		id.Send(selRelease)
	}
}

// Retain sends retain to id and returns it.
func Retain(id ID) ID {
	if id != 0 {
		id.Send(selRetain)
	}
	return id
}
