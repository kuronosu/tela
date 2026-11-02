//go:build darwin

package darwin

import (
	"errors"
	"fmt"
	"sync"
	"unicode/utf16"
)

// The Carbon Event Manager's hot keys (HIToolbox/CarbonEvents.h), the
// current keyboard layout's characters (Text Input Sources and
// UCKeyTranslate) for the global shortcuts, and CGEventPost for the tests
// that press a hot key. HIToolbox and CoreServices are opened on first use
// (LoadCarbon): a backend without them still works, without global
// shortcuts.
//
// Calling conventions through purego (see the package documentation):
//
//   - RegisterEventHotKey takes the EventHotKeyID structure (two UInt32)
//     by value. An 8-byte structure of integers travels in one integer
//     register on arm64 (AAPCS64: a composite of at most 16 bytes, its
//     memory image) and on amd64 (System V: class INTEGER, the memory
//     image), so it is passed as one uint64 holding the signature in the
//     low half and the ID in the high half. Its six arguments all go in
//     registers on both architectures.
//   - GetEventParameter has seven arguments: the seventh goes on the stack
//     on amd64, an 8-byte pointer, which purego lays out like the C ABI.
//   - UCKeyTranslate has ten: on arm64 the last two, on amd64 the last
//     four, go on the stack, all of them 8-byte pointers or UniCharCount
//     (unsigned long); its narrow arguments (UInt16, UInt32) are among the
//     register ones.

// Carbon modifier bits (HIToolbox/Events.h), as RegisterEventHotKey takes
// them.
const (
	CmdKey     = 1 << 8
	ShiftKey   = 1 << 9
	OptionKey  = 1 << 11
	ControlKey = 1 << 12
)

// Carbon event constants.
const (
	// KEventClassKeyboard is kEventClassKeyboard, 'keyb'.
	KEventClassKeyboard = 0x6B657962
	// KEventHotKeyPressed and KEventHotKeyReleased are the kinds of the hot
	// key events.
	KEventHotKeyPressed  = 5
	KEventHotKeyReleased = 6
	// KEventHotKeyExclusive registers a hot key that no other application
	// can register while this one holds it (RegisterEventHotKey's
	// kEventHotKeyExclusive option).
	KEventHotKeyExclusive = 1 << 0

	// EventHotKeyExistsErr: the combination is registered already, by
	// another application with KEventHotKeyExclusive or by this one.
	EventHotKeyExistsErr = -9878
	// EventHotKeyInvalidErr: the hot key is not valid.
	EventHotKeyInvalidErr = -9879
	// eventNotHandledErr: a handler passes the event on.
	eventNotHandledErr = -9874

	kEventParamDirectObject = 0x2D2D2D2D // '----'
	typeEventHotKeyID       = 0x686B6964 // 'hkid'

	kUCKeyActionDown              = 0
	kUCKeyTranslateNoDeadKeysMask = 1
)

// EventHotKeyID is HIToolbox's EventHotKeyID: an application-chosen
// signature and number that the hot key events carry back.
type EventHotKeyID struct {
	Signature uint32
	ID        uint32
}

// Shapes of the Carbon functions; see the comment at the top of the file.
var (
	registerEventHotKey       func(code, mods uint32, id uint64, target uintptr, options uint32, out *uintptr) int32
	unregisterEventHotKey     func(ref uintptr) int32
	getApplicationEventTarget func() uintptr
	installEventHandler       func(target, handler uintptr, numTypes uint, list *[2]uint32, userData uintptr, out *uintptr) int32
	getEventKind              func(event uintptr) uint32
	getEventParameter         func(event uintptr, name, typ uint32, outType *uint32, size uint, outSize *uint, out *EventHotKeyID) int32

	tisCopyCurrentKeyboardLayoutInputSource      func() uintptr
	tisCopyCurrentASCIICapableKeyboardLayoutSrc  func() uintptr
	tisGetInputSourceProperty                    func(source, key uintptr) uintptr
	tisPropertyInputSourceID, tisPropertyKeyData uintptr
	carbonLMGetKbdType                           func() uint8
	cfDataGetBytePtr                             func(data uintptr) uintptr
	ucKeyTranslate                               func(layout uintptr, code, action uint16, mods, kbdType, options uint32, dead *uint32, max uint, n *uint, chars *uint16) int32
)

const frameworkCoreServices = "/System/Library/Frameworks/CoreServices.framework/CoreServices"

var (
	carbonOnce sync.Once
	carbonErr  error
)

// LoadCarbon opens HIToolbox and CoreServices and binds the hot key and
// keyboard layout functions, once; an error means the system has no
// Carbon hot keys (it always has them since macOS 10.0, so this is a
// broken system or a sandbox that hides the frameworks).
func LoadCarbon() error {
	carbonOnce.Do(func() {
		carbonErr = loadOptional(frameworkHIToolbox, func(lib uintptr) {
			bind(&registerEventHotKey, lib, "RegisterEventHotKey")
			bind(&unregisterEventHotKey, lib, "UnregisterEventHotKey")
			bind(&getApplicationEventTarget, lib, "GetApplicationEventTarget")
			bind(&installEventHandler, lib, "InstallEventHandler")
			bind(&getEventKind, lib, "GetEventKind")
			bind(&getEventParameter, lib, "GetEventParameter")
			bind(&tisCopyCurrentKeyboardLayoutInputSource, lib, "TISCopyCurrentKeyboardLayoutInputSource")
			bind(&tisCopyCurrentASCIICapableKeyboardLayoutSrc, lib, "TISCopyCurrentASCIICapableKeyboardLayoutInputSource")
			bind(&tisGetInputSourceProperty, lib, "TISGetInputSourceProperty")
			bind(&carbonLMGetKbdType, lib, "LMGetKbdType")
			tisPropertyInputSourceID = symbolValue(lib, "kTISPropertyInputSourceID")
			tisPropertyKeyData = symbolValue(lib, "kTISPropertyUnicodeKeyLayoutData")
		})
		if carbonErr != nil {
			return
		}
		carbonErr = loadOptional(frameworkCoreServices, func(lib uintptr) {
			bind(&ucKeyTranslate, lib, "UCKeyTranslate")
		})
		if carbonErr != nil {
			return
		}
		carbonErr = loadOptional(frameworkCF, func(lib uintptr) {
			bind(&cfDataGetBytePtr, lib, "CFDataGetBytePtr")
		})
	})
	return carbonErr
}

// RegisterEventHotKey registers a system-wide hot key on the application
// event target: the virtual key code (kVK_*) with the Carbon modifiers
// (CmdKey, ShiftKey, OptionKey, ControlKey), reported to the handler of
// InstallHotKeyHandler with id. options is 0 or KEventHotKeyExclusive.
// status is the OSStatus (0, EventHotKeyExistsErr…). Main thread; call
// LoadCarbon first.
func RegisterEventHotKey(code, mods uint32, id EventHotKeyID, options uint32) (ref uintptr, status int32) {
	packed := uint64(id.Signature) | uint64(id.ID)<<32
	status = registerEventHotKey(code, mods, packed, getApplicationEventTarget(), options, &ref)
	return ref, status
}

// UnregisterEventHotKey removes a hot key RegisterEventHotKey returned.
func UnregisterEventHotKey(ref uintptr) int32 {
	if ref == 0 {
		return 0
	}
	return unregisterEventHotKey(ref)
}

var hotKeyHandler struct {
	sync.Mutex
	once sync.Once
	err  error
	fn   func(kind uint32, id EventHotKeyID) bool
}

// InstallHotKeyHandler makes fn the receiver of the hot key events: a
// Carbon event handler for kEventHotKeyPressed and kEventHotKeyReleased,
// installed once per process on the application event target, calls fn
// on the main thread (from the event dispatch of [NSApp run]) with the
// kind and the hot key's ID. fn reports whether it handled the event;
// unhandled events go on to the next handler. A later call replaces fn
// and installs nothing. Call LoadCarbon first.
func InstallHotKeyHandler(fn func(kind uint32, id EventHotKeyID) bool) error {
	hotKeyHandler.Lock()
	hotKeyHandler.fn = fn
	hotKeyHandler.Unlock()
	hotKeyHandler.once.Do(func() {
		if installEventHandler == nil {
			hotKeyHandler.err = errors.New("darwin: InstallHotKeyHandler before LoadCarbon")
			return
		}
		cb := NewCallback(func(_, event, _ uintptr) uintptr {
			var id EventHotKeyID
			if st := getEventParameter(event, kEventParamDirectObject, typeEventHotKeyID, nil, 8, nil, &id); st != 0 {
				return statusResult(eventNotHandledErr)
			}
			kind := getEventKind(event)
			hotKeyHandler.Lock()
			f := hotKeyHandler.fn
			hotKeyHandler.Unlock()
			if f == nil || !f(kind, id) {
				return statusResult(eventNotHandledErr)
			}
			return 0
		})
		specs := [2][2]uint32{
			{KEventClassKeyboard, KEventHotKeyPressed},
			{KEventClassKeyboard, KEventHotKeyReleased},
		}
		var ref uintptr
		if st := installEventHandler(getApplicationEventTarget(), cb, 2, &specs[0], 0, &ref); st != 0 {
			hotKeyHandler.err = fmt.Errorf("darwin: InstallEventHandler returned OSStatus %d", st)
		}
	})
	return hotKeyHandler.err
}

// KeyLayoutID returns the identifier of the current keyboard layout
// ("com.apple.keylayout.US"), or "" when there is none. Main thread
// (Text Input Sources asserts it on recent macOS); call LoadCarbon first.
func KeyLayoutID() string {
	src := currentLayoutSource()
	if src == 0 {
		return ""
	}
	defer CFRelease(src)
	return GoString(ID(tisGetInputSourceProperty(src, tisPropertyInputSourceID)))
}

// currentLayoutSource returns the current keyboard layout input source
// (+1), or the current ASCII-capable one when the current input method
// has no layout of its own; 0 when neither exists.
func currentLayoutSource() uintptr {
	src := tisCopyCurrentKeyboardLayoutInputSource()
	if src != 0 && tisGetInputSourceProperty(src, tisPropertyKeyData) != 0 {
		return src
	}
	if src != 0 {
		CFRelease(src)
	}
	src = tisCopyCurrentASCIICapableKeyboardLayoutSrc()
	if src != 0 && tisGetInputSourceProperty(src, tisPropertyKeyData) != 0 {
		return src
	}
	if src != 0 {
		CFRelease(src)
	}
	return 0
}

// KeyLayoutCharacters returns the identifier of the current keyboard
// layout and, for each virtual key code below 128, the text its key types
// without modifiers (plain) and with Shift (shifted), translated by
// UCKeyTranslate with the keyboard type of the keyboard used last and no
// dead key state (a dead key gives its accent). Keys that type nothing
// are "". Main thread; call LoadCarbon first.
func KeyLayoutCharacters() (id string, plain, shifted [128]string, err error) {
	src := currentLayoutSource()
	if src == 0 {
		return "", plain, shifted, errors.New("darwin: no keyboard layout with Unicode data (kTISPropertyUnicodeKeyLayoutData)")
	}
	defer CFRelease(src)
	id = GoString(ID(tisGetInputSourceProperty(src, tisPropertyInputSourceID)))
	layout := cfDataGetBytePtr(tisGetInputSourceProperty(src, tisPropertyKeyData))
	if layout == 0 {
		return id, plain, shifted, errors.New("darwin: the keyboard layout has no data")
	}
	kbd := uint32(carbonLMGetKbdType())
	translate := func(code uint16, mods uint32) string {
		var dead uint32
		var n uint
		var buf [8]uint16
		if st := ucKeyTranslate(layout, code, kUCKeyActionDown, mods, kbd, kUCKeyTranslateNoDeadKeysMask, &dead, uint(len(buf)), &n, &buf[0]); st != 0 {
			return ""
		}
		return string(utf16.Decode(buf[:min(n, uint(len(buf)))]))
	}
	for code := range uint16(128) {
		plain[code] = translate(code, 0)
		shifted[code] = translate(code, ShiftKey>>8&0xFF)
	}
	return id, plain, shifted, nil
}

// statusResult is the OSStatus st as a callback's result (C reads its low
// 32 bits).
func statusResult(st int32) uintptr { return uintptr(uint32(st)) }

// CGEventTapLocation values for CGEventPost.
const (
	// CGHIDEventTap is kCGHIDEventTap: where the HID system enters events,
	// so they go through the window server like the keyboard's.
	CGHIDEventTap = 0
)

var (
	postOnce        sync.Once
	cgEventPost     func(tap uint32, event uintptr)
	cgPreflightPost func() bool
)

// CGEventPost posts the event at tap (CGHIDEventTap), into the system's
// input stream: for tests that press a global hot key. Posting needs the
// accessibility (or, on macOS 10.15 and later, the "post event")
// permission; without it the system drops the event silently. ok is false
// when CoreGraphics lacks the function.
func CGEventPost(tap uint32, event uintptr) (ok bool) {
	loadPost()
	if cgEventPost == nil {
		return false
	}
	cgEventPost(tap, event)
	return true
}

// CGPreflightPostEventAccess reports whether the process may post events
// (macOS 10.15 and later; known is false where the function is missing).
func CGPreflightPostEventAccess() (allowed, known bool) {
	loadPost()
	if cgPreflightPost == nil {
		return false, false
	}
	return cgPreflightPost(), true
}

func loadPost() {
	postOnce.Do(func() {
		if loadOptional(frameworkCG, func(lib uintptr) { bind(&cgEventPost, lib, "CGEventPost") }) != nil {
			cgEventPost = nil
		}
		if loadOptional(frameworkCG, func(lib uintptr) { bind(&cgPreflightPost, lib, "CGPreflightPostEventAccess") }) != nil {
			cgPreflightPost = nil
		}
	})
}
