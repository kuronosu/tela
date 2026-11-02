//go:build darwin

package darwin

import (
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// NSUTF8StringEncoding is Foundation's NSStringEncoding for UTF-8.
const NSUTF8StringEncoding = 4

// frameworkUTI is UniformTypeIdentifiers (macOS 11+), home of UTType.
const frameworkUTI = "/System/Library/Frameworks/UniformTypeIdentifiers.framework/UniformTypeIdentifiers"

var (
	selInitWithBytes      = objc.RegisterName("initWithBytes:length:encoding:")
	selDataUsingEncoding  = objc.RegisterName("dataUsingEncoding:")
	selLength             = objc.RegisterName("length")
	selGetBytesLength     = objc.RegisterName("getBytes:length:")
	selArrayWithObjsCount = objc.RegisterName("arrayWithObjects:count:")
)

// NSStringBytes returns a new NSString (+1: Release it) holding the UTF-8
// text b. Unlike NSString it keeps NUL characters (the length is given,
// not found); invalid UTF-8 becomes U+FFFD, since
// initWithBytes:length:encoding: returns nil for it.
func NSStringBytes(b []byte) ID {
	if len(b) == 0 {
		return NSString("")
	}
	if !utf8.Valid(b) {
		b = []byte(strings.ToValidUTF8(string(b), "�"))
	}
	if classNSString == 0 {
		classNSString = objc.GetClass("NSString")
	}
	return ID(classNSString).Send(selAlloc).Send(selInitWithBytes, unsafe.Pointer(&b[0]), uint(len(b)), uint(NSUTF8StringEncoding))
}

// StringBytes returns the UTF-8 bytes of the NSString s, NUL characters
// included (GoString stops at the first one); nil for nil.
func StringBytes(s ID) []byte {
	if s == 0 {
		return nil
	}
	data := s.Send(selDataUsingEncoding, uint(NSUTF8StringEncoding))
	if data == 0 {
		return nil
	}
	b := Bytes(data)
	if b == nil {
		b = []byte{}
	}
	return b
}

// NSData returns a new NSData (+1: Release it) holding a copy of b
// (CFData, which is toll-free bridged with NSData).
func NSData(b []byte) ID {
	var p *byte
	if len(b) > 0 {
		p = &b[0]
	}
	return ID(CFDataCreate(0, p, len(b)))
}

// Bytes returns a copy of the contents of the NSData d (nil for nil or
// empty data).
func Bytes(d ID) []byte {
	if d == 0 {
		return nil
	}
	n := sendInt(d, selLength)
	if n <= 0 {
		return nil
	}
	buf := make([]byte, n)
	d.Send(selGetBytesLength, unsafe.Pointer(&buf[0]), uint(n))
	return buf
}

// NSArray returns an autoreleased NSArray holding objs, in order.
func NSArray(objs ...ID) ID {
	cls := objc.GetClass("NSArray")
	if len(objs) == 0 {
		return ID(cls).Send(selArrayWithObjsCount, unsafe.Pointer(nil), uint(0))
	}
	return ID(cls).Send(selArrayWithObjsCount, unsafe.Pointer(&objs[0]), uint(len(objs)))
}

var (
	utiOnce sync.Once
	utiOK   bool
)

// LoadUniformTypeIdentifiers opens the UniformTypeIdentifiers framework
// once (AppKit usually loads it already) and reports whether its UTType
// class exists (macOS 11 and later). Like every framework it must be
// opened before its classes can be found without cgo.
func LoadUniformTypeIdentifiers() bool {
	utiOnce.Do(func() {
		if _, err := purego.Dlopen(frameworkUTI, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return
		}
		utiOK = objc.GetClass("UTType") != 0
	})
	return utiOK
}

var appKitStrings struct {
	sync.Mutex
	lib    uintptr
	values map[string]ID
}

// AppKitString returns the value of the NSString constant name that AppKit
// exports (NSAppearanceNameDarkAqua, a notification name), or 0 when AppKit
// does not have it. Call it after Load; results are cached.
func AppKitString(name string) ID {
	appKitStrings.Lock()
	defer appKitStrings.Unlock()
	if v, ok := appKitStrings.values[name]; ok {
		return v
	}
	if appKitStrings.values == nil {
		appKitStrings.values = map[string]ID{}
	}
	if appKitStrings.lib == 0 {
		// Load opened AppKit already: this returns the same handle.
		lib, err := purego.Dlopen(frameworkAppKit, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return 0
		}
		appKitStrings.lib = lib
	}
	var v ID
	if p, err := purego.Dlsym(appKitStrings.lib, name); err == nil && p != 0 {
		// p is the address of a C global (an NSString *), outside the Go
		// heap; the double indirection keeps vet's unsafeptr check quiet.
		v = ID(*(*uintptr)(*(*unsafe.Pointer)(unsafe.Pointer(&p))))
	}
	appKitStrings.values[name] = v
	return v
}
