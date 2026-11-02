//go:build darwin

package darwin

import (
	"structs"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// Text input: NSRange, NSString as UTF-16, the secure event input of
// password fields and, for tests, the Text Input Sources API of HIToolbox
// (selecting an input method).

// _NSRange is Foundation's NSRange: a location and a length in UTF-16 code
// units for the text APIs. Use it through the NSRange alias; the
// underscore name is what makes the Objective-C type encoding of a method
// registered with RegisterClass "{_NSRange=QQ}", Foundation's own.
type _NSRange struct {
	_        structs.HostLayout
	Location uint
	Length   uint
}

// NSRange is Foundation's NSRange, passed and returned by value (in
// registers: two NSUIntegers).
type NSRange = _NSRange

// NSNotFound is Foundation's NSNotFound (NSIntegerMax): the location of
// "no range".
const NSNotFound = 1<<63 - 1

var (
	selGetCharactersRange = objc.RegisterName("getCharacters:range:")
	selIsKindOfClass      = objc.RegisterName("isKindOfClass:")
	selString             = objc.RegisterName("string")
	selInitWithString     = objc.RegisterName("initWithString:")
	selAutorelease        = objc.RegisterName("autorelease")
)

// HasProtocol reports whether the Objective-C runtime knows the protocol
// name (a protocol object exists once some loaded image references it).
func HasProtocol(name string) bool { return objc.GetProtocol(name) != nil }

// ProtocolID returns the protocol name as an object, for
// conformsToProtocol: (0 when the runtime does not know it).
func ProtocolID(name string) ID {
	p := objc.GetProtocol(name)
	if p == nil {
		return 0
	}
	return ID(uintptr(unsafe.Pointer(p)))
}

// SelName returns the name of a selector.
func SelName(s SEL) string { return selGetName(s) }

var selGetName func(s SEL) string

// StringUTF16 returns the UTF-16 code units of a text object as the text
// APIs receive it: an NSString, or an NSAttributedString (whose string is
// taken). Unlike StringBytes it keeps unpaired surrogates, so offsets
// counted by the sender stay valid. nil for nil or empty text.
func StringUTF16(s ID) []uint16 {
	if s == 0 {
		return nil
	}
	if cls := objc.GetClass("NSAttributedString"); cls != 0 && sendBool(s, selIsKindOfClass, cls) {
		s = s.Send(selString)
		if s == 0 {
			return nil
		}
	}
	n := sendUint(s, selLength)
	if n == 0 || n > 1<<30 {
		return nil
	}
	buf := make([]uint16, n)
	s.Send(selGetCharactersRange, unsafe.Pointer(&buf[0]), NSRange{Length: uint(n)})
	return buf
}

// NSAttributedStringAutoreleased returns an autoreleased NSAttributedString
// holding s without attributes, for methods that return one to AppKit.
func NSAttributedStringAutoreleased(s string) ID {
	cls := objc.GetClass("NSAttributedString")
	str := NSString(s)
	defer Release(str)
	a := ID(cls).Send(selAlloc).Send(selInitWithString, str)
	if a == 0 {
		return 0
	}
	return a.Send(selAutorelease)
}

var (
	secureOnce     sync.Once
	enableSecure   func() int32
	disableSecure  func() int32
	isSecureOn     func() bool
	secureLoadFail bool
)

func loadSecure() bool {
	secureOnce.Do(func() {
		if err := loadOptional(frameworkHIToolbox, func(lib uintptr) {
			bind(&enableSecure, lib, "EnableSecureEventInput")
			bind(&disableSecure, lib, "DisableSecureEventInput")
			bind(&isSecureOn, lib, "IsSecureEventInputEnabled")
		}); err != nil {
			secureLoadFail = true
		}
	})
	return !secureLoadFail
}

// SetSecureEventInput enables (true) or disables the secure event input of
// the process (HIToolbox's EnableSecureEventInput, which NSSecureTextField
// uses): other processes cannot read the keys typed while it is on. The
// calls nest, so each enable must be balanced by a disable. It reports
// false when HIToolbox is missing.
func SetSecureEventInput(on bool) bool {
	if !loadSecure() {
		return false
	}
	if on {
		enableSecure()
	} else {
		disableSecure()
	}
	return true
}

// SecureEventInputEnabled reports whether some process has the secure
// event input on (IsSecureEventInputEnabled).
func SecureEventInputEnabled() bool { return loadSecure() && isSecureOn() }

// The Text Input Sources API (HIToolbox/TextInputSources.h), bound by
// LoadTextInputSources, for tests that select an input method. Input
// sources and lists are CF objects: the Create and Copy functions return
// them owned (CFRelease); TISGetInputSourceProperty returns a borrowed
// value. The OSStatus results are 0 on success.
var (
	TISCreateInputSourceList          func(properties uintptr, includeAllInstalled bool) uintptr
	TISCopyCurrentKeyboardInputSource func() uintptr
	TISSelectInputSource              func(source uintptr) int32
	TISEnableInputSource              func(source uintptr) int32
	TISDisableInputSource             func(source uintptr) int32
	TISGetInputSourceProperty         func(source, key uintptr) uintptr

	// TISPropertyInputSourceID is kTISPropertyInputSourceID, the key of
	// an input source's identifier (a CFString).
	TISPropertyInputSourceID uintptr
)

var (
	tisOnce sync.Once
	tisErr  error
)

// LoadTextInputSources opens HIToolbox and binds the Text Input Sources
// functions, once.
func LoadTextInputSources() error {
	tisOnce.Do(func() {
		tisErr = loadOptional(frameworkHIToolbox, func(lib uintptr) {
			bind(&TISCreateInputSourceList, lib, "TISCreateInputSourceList")
			bind(&TISCopyCurrentKeyboardInputSource, lib, "TISCopyCurrentKeyboardInputSource")
			bind(&TISSelectInputSource, lib, "TISSelectInputSource")
			bind(&TISEnableInputSource, lib, "TISEnableInputSource")
			bind(&TISDisableInputSource, lib, "TISDisableInputSource")
			bind(&TISGetInputSourceProperty, lib, "TISGetInputSourceProperty")
			TISPropertyInputSourceID = symbolValue(lib, "kTISPropertyInputSourceID")
		})
	})
	return tisErr
}
