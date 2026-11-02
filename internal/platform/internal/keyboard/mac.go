package keyboard

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// The OS-independent half of the macOS keyboard: what the cocoa backend's
// NSView keyDown:, keyUp: and flagsChanged: turn into platform.KeyEvents.
// It has no build tag so that it is tested on every OS; the backend passes
// the NSEvent's fields and queues what comes back through its Held set.
//
//   - Code is the physical key, from the virtual key code ([NSEvent
//     keyCode], a kVK_* constant of HIToolbox/Events.h) through macKeyCodes,
//     the same facts as the mac column of the W3C UI Events code table,
//     Chromium's dom_code_data.inc and winit's scancode_to_physicalkey.
//     Location is Code.Location() (the NumericPad flag is set for the
//     arrows too, so it says nothing about the location).
//   - Key is a named key when the characters are one of AppKit's
//     function-key characters (NSUpArrowFunctionKey = 0xF700 ...) or a
//     control character (Return, Tab, Backspace, Escape), or when the
//     physical key is one that has no characters (modifiers, Eisu, Kana,
//     the volume keys). Otherwise it is the character, read like winit
//     does: [NSEvent characters] (which applies Shift, Option and Caps
//     Lock) when neither Command nor Control is held, and
//     charactersIgnoringModifiers (Shift only) when one is, so that Cmd+S
//     has Key "s", Cmd+Shift+S "S" and Cmd+Option+S "s", as Ctrl shortcuts
//     do on the other platforms. A dead key (Option+E on the US layout) has
//     empty characters (AppKit's documentation of charactersIgnoringModifiers
//     says so): its Key is KeyDead.
//   - Text is the characters without control and function-key characters
//     (Option types characters on macOS: Option+A types "å"), or, while
//     Command or Control is held, the character of Key, as the contract
//     wants for Ctrl+C ("c"); the layers above do not insert it. After a
//     dead key the characters carry the composed text ("é") or the accent
//     and the character when they do not compose ("´x", Key "x").
//   - Mods: Shift, Control, Option (ModAlt) and Command (ModSuper); Caps
//     Lock, Fn, NumericPad and Help are not modifiers. macOS has no AltGr:
//     both Options are Alt and select characters, which Key and Text
//     reflect.
//   - flagsChanged: one event per modifier key, which does not say whether
//     the key went down or up. The device-dependent bits of modifierFlags
//     (NX_DEVICELSHIFTKEYMASK ...) tell the two sides apart, so releasing
//     one of two held Shifts is seen; events without them (posted by other
//     programs) fall back to the device-independent bit and the Held set,
//     as SDL and winit do. Caps Lock reports a change of the lock state: a
//     press and a release (SDL).
//
// What macOS does not report through these methods: the media and
// brightness keys of Apple keyboards arrive as NSSystemDefined events, the
// power key not at all.

// NSEventModifierFlags (NSEvent.h): the device-independent modifier bits of
// [NSEvent modifierFlags].
const (
	MacFlagCapsLock   uint64 = 1 << 16 // NSEventModifierFlagCapsLock (a lock, not a modifier)
	MacFlagShift      uint64 = 1 << 17 // NSEventModifierFlagShift
	MacFlagControl    uint64 = 1 << 18 // NSEventModifierFlagControl
	MacFlagOption     uint64 = 1 << 19 // NSEventModifierFlagOption
	MacFlagCommand    uint64 = 1 << 20 // NSEventModifierFlagCommand
	MacFlagNumericPad uint64 = 1 << 21 // NSEventModifierFlagNumericPad: keypad and arrow keys
	MacFlagHelp       uint64 = 1 << 22 // NSEventModifierFlagHelp
	MacFlagFunction   uint64 = 1 << 23 // NSEventModifierFlagFunction: Fn, and the function, arrow and navigation keys
)

// The device-dependent bits of modifierFlags (IOKit's IOLLEvent.h,
// NX_DEVICE*KEYMASK), set by the keyboard driver for each side.
const (
	MacDeviceLeftControl  uint64 = 0x0001 // NX_DEVICELCTLKEYMASK
	MacDeviceLeftShift    uint64 = 0x0002 // NX_DEVICELSHIFTKEYMASK
	MacDeviceRightShift   uint64 = 0x0004 // NX_DEVICERSHIFTKEYMASK
	MacDeviceLeftCommand  uint64 = 0x0008 // NX_DEVICELCMDKEYMASK
	MacDeviceRightCommand uint64 = 0x0010 // NX_DEVICERCMDKEYMASK
	MacDeviceLeftOption   uint64 = 0x0020 // NX_DEVICELALTKEYMASK
	MacDeviceRightOption  uint64 = 0x0040 // NX_DEVICERALTKEYMASK
	MacDeviceRightControl uint64 = 0x2000 // NX_DEVICERCTLKEYMASK
)

// Virtual key codes (HIToolbox/Events.h) that the translation names.
const (
	kvkANSIGrave    = 0x32 // kVK_ANSI_Grave
	kvkISOSection   = 0x0A // kVK_ISO_Section
	kvkRightCommand = 0x36 // kVK_RightCommand
	kvkCommand      = 0x37 // kVK_Command
	kvkShift        = 0x38 // kVK_Shift
	kvkCapsLock     = 0x39 // kVK_CapsLock
	kvkOption       = 0x3A // kVK_Option
	kvkControl      = 0x3B // kVK_Control
	kvkRightShift   = 0x3C // kVK_RightShift
	kvkRightOption  = 0x3D // kVK_RightOption
	kvkRightControl = 0x3E // kVK_RightControl
	kvkFunction     = 0x3F // kVK_Function
)

// macKeyCodes maps the virtual key codes of HIToolbox/Events.h to physical
// keys. The kVK_ name of each is in its comment; the codes below 0x80 that
// are missing are listed in macUnmapped.
var macKeyCodes = [0x80]platform.KeyCode{
	0x00: platform.CodeKeyA,            // kVK_ANSI_A
	0x01: platform.CodeKeyS,            // kVK_ANSI_S
	0x02: platform.CodeKeyD,            // kVK_ANSI_D
	0x03: platform.CodeKeyF,            // kVK_ANSI_F
	0x04: platform.CodeKeyH,            // kVK_ANSI_H
	0x05: platform.CodeKeyG,            // kVK_ANSI_G
	0x06: platform.CodeKeyZ,            // kVK_ANSI_Z
	0x07: platform.CodeKeyX,            // kVK_ANSI_X
	0x08: platform.CodeKeyC,            // kVK_ANSI_C
	0x09: platform.CodeKeyV,            // kVK_ANSI_V
	0x0A: platform.CodeIntlBackslash,   // kVK_ISO_Section (see MacKeyCode for ISO keyboards)
	0x0B: platform.CodeKeyB,            // kVK_ANSI_B
	0x0C: platform.CodeKeyQ,            // kVK_ANSI_Q
	0x0D: platform.CodeKeyW,            // kVK_ANSI_W
	0x0E: platform.CodeKeyE,            // kVK_ANSI_E
	0x0F: platform.CodeKeyR,            // kVK_ANSI_R
	0x10: platform.CodeKeyY,            // kVK_ANSI_Y
	0x11: platform.CodeKeyT,            // kVK_ANSI_T
	0x12: platform.CodeDigit1,          // kVK_ANSI_1
	0x13: platform.CodeDigit2,          // kVK_ANSI_2
	0x14: platform.CodeDigit3,          // kVK_ANSI_3
	0x15: platform.CodeDigit4,          // kVK_ANSI_4
	0x16: platform.CodeDigit6,          // kVK_ANSI_6
	0x17: platform.CodeDigit5,          // kVK_ANSI_5
	0x18: platform.CodeEqual,           // kVK_ANSI_Equal
	0x19: platform.CodeDigit9,          // kVK_ANSI_9
	0x1A: platform.CodeDigit7,          // kVK_ANSI_7
	0x1B: platform.CodeMinus,           // kVK_ANSI_Minus
	0x1C: platform.CodeDigit8,          // kVK_ANSI_8
	0x1D: platform.CodeDigit0,          // kVK_ANSI_0
	0x1E: platform.CodeBracketRight,    // kVK_ANSI_RightBracket
	0x1F: platform.CodeKeyO,            // kVK_ANSI_O
	0x20: platform.CodeKeyU,            // kVK_ANSI_U
	0x21: platform.CodeBracketLeft,     // kVK_ANSI_LeftBracket
	0x22: platform.CodeKeyI,            // kVK_ANSI_I
	0x23: platform.CodeKeyP,            // kVK_ANSI_P
	0x24: platform.CodeEnter,           // kVK_Return
	0x25: platform.CodeKeyL,            // kVK_ANSI_L
	0x26: platform.CodeKeyJ,            // kVK_ANSI_J
	0x27: platform.CodeQuote,           // kVK_ANSI_Quote
	0x28: platform.CodeKeyK,            // kVK_ANSI_K
	0x29: platform.CodeSemicolon,       // kVK_ANSI_Semicolon
	0x2A: platform.CodeBackslash,       // kVK_ANSI_Backslash
	0x2B: platform.CodeComma,           // kVK_ANSI_Comma
	0x2C: platform.CodeSlash,           // kVK_ANSI_Slash
	0x2D: platform.CodeKeyN,            // kVK_ANSI_N
	0x2E: platform.CodeKeyM,            // kVK_ANSI_M
	0x2F: platform.CodePeriod,          // kVK_ANSI_Period
	0x30: platform.CodeTab,             // kVK_Tab
	0x31: platform.CodeSpace,           // kVK_Space
	0x32: platform.CodeBackquote,       // kVK_ANSI_Grave (see MacKeyCode for ISO keyboards)
	0x33: platform.CodeBackspace,       // kVK_Delete (the key labeled delete is Backspace)
	0x35: platform.CodeEscape,          // kVK_Escape
	0x36: platform.CodeMetaRight,       // kVK_RightCommand
	0x37: platform.CodeMetaLeft,        // kVK_Command
	0x38: platform.CodeShiftLeft,       // kVK_Shift
	0x39: platform.CodeCapsLock,        // kVK_CapsLock
	0x3A: platform.CodeAltLeft,         // kVK_Option
	0x3B: platform.CodeControlLeft,     // kVK_Control
	0x3C: platform.CodeShiftRight,      // kVK_RightShift
	0x3D: platform.CodeAltRight,        // kVK_RightOption
	0x3E: platform.CodeControlRight,    // kVK_RightControl
	0x3F: platform.CodeFn,              // kVK_Function
	0x40: platform.CodeF17,             // kVK_F17
	0x41: platform.CodeNumpadDecimal,   // kVK_ANSI_KeypadDecimal
	0x43: platform.CodeNumpadMultiply,  // kVK_ANSI_KeypadMultiply
	0x45: platform.CodeNumpadAdd,       // kVK_ANSI_KeypadPlus
	0x47: platform.CodeNumLock,         // kVK_ANSI_KeypadClear: W3C names the keypad's Clear NumLock on a Mac
	0x48: platform.CodeAudioVolumeUp,   // kVK_VolumeUp
	0x49: platform.CodeAudioVolumeDown, // kVK_VolumeDown
	0x4A: platform.CodeAudioVolumeMute, // kVK_Mute
	0x4B: platform.CodeNumpadDivide,    // kVK_ANSI_KeypadDivide
	0x4C: platform.CodeNumpadEnter,     // kVK_ANSI_KeypadEnter
	0x4E: platform.CodeNumpadSubtract,  // kVK_ANSI_KeypadMinus
	0x4F: platform.CodeF18,             // kVK_F18
	0x50: platform.CodeF19,             // kVK_F19
	0x51: platform.CodeNumpadEqual,     // kVK_ANSI_KeypadEquals
	0x52: platform.CodeNumpad0,         // kVK_ANSI_Keypad0
	0x53: platform.CodeNumpad1,         // kVK_ANSI_Keypad1
	0x54: platform.CodeNumpad2,         // kVK_ANSI_Keypad2
	0x55: platform.CodeNumpad3,         // kVK_ANSI_Keypad3
	0x56: platform.CodeNumpad4,         // kVK_ANSI_Keypad4
	0x57: platform.CodeNumpad5,         // kVK_ANSI_Keypad5
	0x58: platform.CodeNumpad6,         // kVK_ANSI_Keypad6
	0x59: platform.CodeNumpad7,         // kVK_ANSI_Keypad7
	0x5A: platform.CodeF20,             // kVK_F20
	0x5B: platform.CodeNumpad8,         // kVK_ANSI_Keypad8
	0x5C: platform.CodeNumpad9,         // kVK_ANSI_Keypad9
	0x5D: platform.CodeIntlYen,         // kVK_JIS_Yen
	0x5E: platform.CodeIntlRo,          // kVK_JIS_Underscore
	0x5F: platform.CodeNumpadComma,     // kVK_JIS_KeypadComma
	0x60: platform.CodeF5,              // kVK_F5
	0x61: platform.CodeF6,              // kVK_F6
	0x62: platform.CodeF7,              // kVK_F7
	0x63: platform.CodeF3,              // kVK_F3
	0x64: platform.CodeF8,              // kVK_F8
	0x65: platform.CodeF9,              // kVK_F9
	0x66: platform.CodeLang2,           // kVK_JIS_Eisu (英数)
	0x67: platform.CodeF11,             // kVK_F11
	0x68: platform.CodeLang1,           // kVK_JIS_Kana (かな)
	0x69: platform.CodeF13,             // kVK_F13
	0x6A: platform.CodeF16,             // kVK_F16
	0x6B: platform.CodeF14,             // kVK_F14
	0x6D: platform.CodeF10,             // kVK_F10
	0x6E: platform.CodeContextMenu,     // kVK_ContextualMenu (macOS 10.12+)
	0x6F: platform.CodeF12,             // kVK_F12
	0x71: platform.CodeF15,             // kVK_F15
	0x72: platform.CodeInsert,          // kVK_Help: the key at the Insert position (a PC keyboard's Insert), as Chromium and winit say
	0x73: platform.CodeHome,            // kVK_Home
	0x74: platform.CodePageUp,          // kVK_PageUp
	0x75: platform.CodeDelete,          // kVK_ForwardDelete
	0x76: platform.CodeF4,              // kVK_F4
	0x77: platform.CodeEnd,             // kVK_End
	0x78: platform.CodeF2,              // kVK_F2
	0x79: platform.CodePageDown,        // kVK_PageDown
	0x7A: platform.CodeF1,              // kVK_F1
	0x7B: platform.CodeArrowLeft,       // kVK_LeftArrow
	0x7C: platform.CodeArrowRight,      // kVK_RightArrow
	0x7D: platform.CodeArrowDown,       // kVK_DownArrow
	0x7E: platform.CodeArrowUp,         // kVK_UpArrow
}

// macUnmapped are the virtual key codes below 0x80 that HIToolbox does not
// name and that no current keyboard sends; they come back as
// platform.UnidentifiedKeyCode, like every code from 0x80 up.
var macUnmapped = [...]uint16{
	0x34, // Enter of old PowerBooks (Inside Macintosh); Chromium and winit leave it unmapped
	0x42, // unassigned
	0x44, // unassigned
	0x46, // unassigned
	0x4D, // unassigned
	0x6C, // unassigned
	0x70, // unassigned
	0x7F, // the power key of ADB keyboards; macOS no longer sends it as a key
}

// MacKeyCode returns the physical key of a macOS virtual key code ([NSEvent
// keyCode]). Codes the table does not know come back as
// platform.UnidentifiedKeyCode with the virtual key code, so they still
// compare equal to themselves.
//
// iso reports an ISO keyboard (KBGetLayoutType(LMGetKbdType()) ==
// kKeyboardISO): there the keyboard driver swaps kVK_ISO_Section and
// kVK_ANSI_Grave, so the key left of 1 reports kVK_ISO_Section and the key
// between the left Shift and Z kVK_ANSI_Grave. MacKeyCode swaps them back,
// as SDL does, so the codes name the positions the W3C table gives.
func MacKeyCode(kvk uint16, iso bool) platform.KeyCode {
	if iso {
		switch kvk {
		case kvkISOSection:
			kvk = kvkANSIGrave
		case kvkANSIGrave:
			kvk = kvkISOSection
		}
	}
	if int(kvk) < len(macKeyCodes) && macKeyCodes[kvk] != platform.CodeUnidentified {
		return macKeyCodes[kvk]
	}
	return platform.UnidentifiedKeyCode(uint32(kvk))
}

// MacModifiers returns the modifiers that modifierFlags reports held:
// Shift, Control (ModCtrl), Option (ModAlt) and Command (ModSuper). Caps
// Lock is a lock and Fn, NumericPad and Help describe keys, so none of them
// is a modifier (ADR-050).
func MacModifiers(flags uint64) platform.Modifiers {
	var m platform.Modifiers
	if flags&MacFlagShift != 0 {
		m |= platform.ModShift
	}
	if flags&MacFlagControl != 0 {
		m |= platform.ModCtrl
	}
	if flags&MacFlagOption != 0 {
		m |= platform.ModAlt
	}
	if flags&MacFlagCommand != 0 {
		m |= platform.ModSuper
	}
	return m
}

// MacKey is what the translation reads of an NSEvent of type keyDown or
// keyUp.
type MacKey struct {
	KeyCode uint16 // [NSEvent keyCode], a kVK_* virtual key code
	// Flags is [NSEvent modifierFlags], device-dependent bits included.
	Flags uint64
	// Characters is [NSEvent characters]: the text of the key with every
	// modifier applied (Control gives control characters), composed with a
	// pending dead key, empty for a dead key.
	Characters string
	// CharactersIgnoringModifiers is [NSEvent charactersIgnoringModifiers]:
	// the character with Shift only.
	CharactersIgnoringModifiers string
	Repeat                      bool // [NSEvent isARepeat]; ignored by MacKeyUp
	// ISO reports an ISO keyboard (see MacKeyCode).
	ISO  bool
	Time time.Duration // the event's time in the backend's time base
}

// MacKeyDown translates a keyDown: event into a press (Repeat from
// isARepeat). ok is false for the modifier keys, which macOS reports
// through flagsChanged: (MacFlagsChanged), never through keyDown:.
func MacKeyDown(e MacKey) (ev platform.KeyEvent, ok bool) {
	return macKeyEvent(e, platform.KeyPress)
}

// MacKeyUp translates a keyUp: event into a release: Key with the same
// rules as the press, no Text. ok is false for the modifier keys.
func MacKeyUp(e MacKey) (ev platform.KeyEvent, ok bool) {
	return macKeyEvent(e, platform.KeyRelease)
}

func macKeyEvent(e MacKey, action platform.KeyAction) (platform.KeyEvent, bool) {
	if _, mod := macModKeys[e.KeyCode]; mod || e.KeyCode == kvkCapsLock {
		return platform.KeyEvent{}, false
	}
	code := MacKeyCode(e.KeyCode, e.ISO)
	ev := platform.KeyEvent{
		Action:   action,
		Code:     code,
		Location: code.Location(),
		Repeat:   action == platform.KeyPress && e.Repeat,
		Mods:     MacModifiers(e.Flags),
		Time:     e.Time,
	}
	ev.Key, ev.Text = macLogical(e, code)
	if action == platform.KeyRelease {
		ev.Text = ""
	}
	return ev, true
}

// macLogical returns the logical key and the text of a key event.
func macLogical(e MacKey, code platform.KeyCode) (platform.Key, string) {
	shortcut := e.Flags&(MacFlagControl|MacFlagCommand) != 0
	// The text a named key types: nothing, except the accent of a pending
	// dead key that Return or Tab types before acting (as on Windows).
	namedText := func() string {
		if shortcut {
			return ""
		}
		return macClean(e.Characters)
	}
	// A named key, from the characters: charactersIgnoringModifiers first,
	// where Control has not turned a letter into a control character
	// (Ctrl+H is "h" there, "\b" in characters).
	ign := e.CharactersIgnoringModifiers
	if ign == "" {
		ign = e.Characters
	}
	if k, ok := macNamedChar(ign); ok {
		return k, namedText()
	}
	// A named key from the physical key: modifiers and keys without
	// characters.
	if k, ok := macNamedCodes[code]; ok {
		return k, namedText()
	}
	// A character key without characters is a dead key: AppKit gives none
	// until the next key (Option+E on the US layout). A key the table does
	// not know is more likely one that types nothing.
	noChars := func() platform.Key {
		if code.Identified() {
			return platform.KeyDead
		}
		return platform.KeyUnidentified
	}
	if shortcut {
		// Command and Control are not reflected (Ctrl on the other
		// platforms, ADR-050): the character with Shift only.
		s := macClean(e.CharactersIgnoringModifiers)
		if s == "" {
			if e.CharactersIgnoringModifiers == "" {
				return noChars(), ""
			}
			return platform.KeyUnidentified, ""
		}
		k := macKeyFromText(s, "")
		if e.Flags&(MacFlagShift|MacFlagCapsLock) != 0 {
			// In case AppKit drops Shift or Caps Lock from
			// charactersIgnoringModifiers while Command or Control is
			// held: Cmd+Shift+S is "S", and so is Cmd+S with Caps Lock
			// (as Ctrl+S is on Linux). Only ASCII letters, which every
			// layout shifts the same way.
			k = macUpper(k)
		}
		return k, string(k)
	}
	text := macClean(e.Characters)
	if text == "" {
		if e.Characters == "" {
			return noChars(), ""
		}
		// Only control characters that name no key.
		if k := macKeyFromText(macClean(e.CharactersIgnoringModifiers), ""); k != "" {
			return k, ""
		}
		return platform.KeyUnidentified, ""
	}
	return macKeyFromText(text, macClean(e.CharactersIgnoringModifiers)), text
}

// macNamedChar names the key whose characters are s: one of AppKit's
// function-key characters or a control character that stands for a key.
func macNamedChar(s string) (platform.Key, bool) {
	r, _ := utf8.DecodeRuneInString(s)
	switch r {
	case 0x03, '\r': // keypad Enter (ETX), Return
		return platform.KeyEnter, true
	case '\t', 0x19: // Tab, Shift+Tab (EM, backtab)
		return platform.KeyTab, true
	case 0x1B:
		return platform.KeyEscape, true
	case 0x7F, '\b': // the delete key is DEL
		return platform.KeyBackspace, true
	}
	if !macFunctionChar(r) {
		return "", false
	}
	if r >= nsF1FunctionKey && int(r-nsF1FunctionKey) < len(macFKeys) {
		return macFKeys[r-nsF1FunctionKey], true
	}
	if k, ok := macFunctionKeys[r]; ok {
		return k, true
	}
	// F25 to F35 and the function keys of NeXT keyboards that W3C does
	// not name.
	return platform.KeyUnidentified, true
}

// The function-key characters of NSEvent.h: AppKit reserves U+F700 to
// U+F8FF for them, but uses only the start of the range, and U+F8FF is the
// Apple logo that Option+Shift+K types, so only U+F700 to U+F7FF are
// treated as keys (and never as text).
const (
	nsUpArrowFunctionKey = 0xF700
	nsF1FunctionKey      = 0xF704 // NSF1FunctionKey; F2 to F35 follow
	nsFunctionKeyLast    = 0xF7FF
)

func macFunctionChar(r rune) bool { return r >= nsUpArrowFunctionKey && r <= nsFunctionKeyLast }

// macFKeys are the named keys of NSF1FunctionKey to NSF24FunctionKey.
var macFKeys = [...]platform.Key{
	platform.KeyF1, platform.KeyF2, platform.KeyF3, platform.KeyF4, platform.KeyF5, platform.KeyF6,
	platform.KeyF7, platform.KeyF8, platform.KeyF9, platform.KeyF10, platform.KeyF11, platform.KeyF12,
	platform.KeyF13, platform.KeyF14, platform.KeyF15, platform.KeyF16, platform.KeyF17, platform.KeyF18,
	platform.KeyF19, platform.KeyF20, platform.KeyF21, platform.KeyF22, platform.KeyF23, platform.KeyF24,
}

// macFunctionKeys names the other function-key characters of NSEvent.h
// that W3C has a key for. NSHelpFunctionKey, which the key at the Insert
// position (kVK_Help) types, is Help, as browsers on a Mac report it.
var macFunctionKeys = map[rune]platform.Key{
	0xF700: platform.KeyArrowUp,     // NSUpArrowFunctionKey
	0xF701: platform.KeyArrowDown,   // NSDownArrowFunctionKey
	0xF702: platform.KeyArrowLeft,   // NSLeftArrowFunctionKey
	0xF703: platform.KeyArrowRight,  // NSRightArrowFunctionKey
	0xF727: platform.KeyInsert,      // NSInsertFunctionKey
	0xF728: platform.KeyDelete,      // NSDeleteFunctionKey (forward delete)
	0xF729: platform.KeyHome,        // NSHomeFunctionKey
	0xF72B: platform.KeyEnd,         // NSEndFunctionKey
	0xF72C: platform.KeyPageUp,      // NSPageUpFunctionKey
	0xF72D: platform.KeyPageDown,    // NSPageDownFunctionKey
	0xF72E: platform.KeyPrintScreen, // NSPrintScreenFunctionKey
	0xF72F: platform.KeyScrollLock,  // NSScrollLockFunctionKey
	0xF730: platform.KeyPause,       // NSPauseFunctionKey
	0xF732: platform.KeyPause,       // NSBreakFunctionKey
	0xF735: platform.KeyContextMenu, // NSMenuFunctionKey
	0xF738: platform.KeyPrint,       // NSPrintFunctionKey
	0xF739: platform.KeyClear,       // NSClearLineFunctionKey (the keypad's Clear)
	0xF73A: platform.KeyClear,       // NSClearDisplayFunctionKey
	0xF741: platform.KeySelect,      // NSSelectFunctionKey
	0xF742: platform.KeyExecute,     // NSExecuteFunctionKey
	0xF743: platform.KeyUndo,        // NSUndoFunctionKey
	0xF744: platform.KeyRedo,        // NSRedoFunctionKey
	0xF745: platform.KeyFind,        // NSFindFunctionKey
	0xF746: platform.KeyHelp,        // NSHelpFunctionKey
	0xF747: platform.KeyModeChange,  // NSModeSwitchFunctionKey
}

// macNamedCodes names the physical keys whose Key does not depend on the
// layout, for events whose characters do not name them: the modifiers and
// the keys that type nothing (Eisu, Kana, the volume keys), and the rest
// for robustness when AppKit gives no characters.
var macNamedCodes = map[platform.KeyCode]platform.Key{
	platform.CodeEnter:           platform.KeyEnter,
	platform.CodeNumpadEnter:     platform.KeyEnter,
	platform.CodeTab:             platform.KeyTab,
	platform.CodeBackspace:       platform.KeyBackspace,
	platform.CodeEscape:          platform.KeyEscape,
	platform.CodeDelete:          platform.KeyDelete,
	platform.CodeInsert:          platform.KeyHelp, // kVK_Help types NSHelpFunctionKey
	platform.CodeHome:            platform.KeyHome,
	platform.CodeEnd:             platform.KeyEnd,
	platform.CodePageUp:          platform.KeyPageUp,
	platform.CodePageDown:        platform.KeyPageDown,
	platform.CodeArrowUp:         platform.KeyArrowUp,
	platform.CodeArrowDown:       platform.KeyArrowDown,
	platform.CodeArrowLeft:       platform.KeyArrowLeft,
	platform.CodeArrowRight:      platform.KeyArrowRight,
	platform.CodeNumLock:         platform.KeyClear, // the keypad's Clear: a Mac has no Num Lock
	platform.CodeContextMenu:     platform.KeyContextMenu,
	platform.CodeLang1:           platform.KeyKanaMode, // かな
	platform.CodeLang2:           platform.KeyEisu,     // 英数
	platform.CodeAudioVolumeUp:   platform.KeyAudioVolumeUp,
	platform.CodeAudioVolumeDown: platform.KeyAudioVolumeDown,
	platform.CodeAudioVolumeMute: platform.KeyAudioVolumeMute,
	platform.CodeShiftLeft:       platform.KeyShift,
	platform.CodeShiftRight:      platform.KeyShift,
	platform.CodeControlLeft:     platform.KeyControl,
	platform.CodeControlRight:    platform.KeyControl,
	platform.CodeAltLeft:         platform.KeyAlt,
	platform.CodeAltRight:        platform.KeyAlt,
	platform.CodeMetaLeft:        platform.KeyMeta,
	platform.CodeMetaRight:       platform.KeyMeta,
	platform.CodeCapsLock:        platform.KeyCapsLock,
	platform.CodeFn:              platform.KeyFn,
	platform.CodeF1:              platform.KeyF1,
	platform.CodeF2:              platform.KeyF2,
	platform.CodeF3:              platform.KeyF3,
	platform.CodeF4:              platform.KeyF4,
	platform.CodeF5:              platform.KeyF5,
	platform.CodeF6:              platform.KeyF6,
	platform.CodeF7:              platform.KeyF7,
	platform.CodeF8:              platform.KeyF8,
	platform.CodeF9:              platform.KeyF9,
	platform.CodeF10:             platform.KeyF10,
	platform.CodeF11:             platform.KeyF11,
	platform.CodeF12:             platform.KeyF12,
	platform.CodeF13:             platform.KeyF13,
	platform.CodeF14:             platform.KeyF14,
	platform.CodeF15:             platform.KeyF15,
	platform.CodeF16:             platform.KeyF16,
	platform.CodeF17:             platform.KeyF17,
	platform.CodeF18:             platform.KeyF18,
	platform.CodeF19:             platform.KeyF19,
	platform.CodeF20:             platform.KeyF20,
}

// macClean removes what is never text from AppKit's characters: control
// characters (C0, DEL, C1: Return, Tab, Escape, Control+letter) and the
// function-key characters (ADR-050).
func macClean(s string) string {
	if !strings.ContainsFunc(s, macNotText) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if macNotText(r) {
			return -1
		}
		return r
	}, s)
}

func macNotText(r rune) bool { return r < 0x20 || (r >= 0x7F && r < 0xA0) || macFunctionChar(r) }

// macKeyFromText chooses the logical key for the text a key typed. A
// single character is the key ("é" after a dead acute accent). More than
// one means that a dead key did not compose with this one and typed its
// accent before the character: the key is then the character the key
// types alone, fallback (winit, and the Win32 backend), or the last
// character of the text when there is none.
func macKeyFromText(text, fallback string) platform.Key {
	switch {
	case text == "":
		return platform.Key(fallback)
	case macClusters(text) == 1:
		return platform.Key(text)
	case fallback != "" && macClusters(fallback) == 1:
		return platform.Key(fallback)
	}
	_, n := utf8.DecodeLastRuneInString(text)
	return platform.Key(text[len(text)-n:])
}

// macClusters approximates the number of user-perceived characters of
// text: the runes that do not extend the previous one (combining marks,
// joiners, variation selectors, emoji modifiers and what follows a joiner).
func macClusters(text string) int {
	n := 0
	joined := false
	for _, r := range text {
		switch {
		case r == 0x200D: // ZWJ
			joined = true
			continue
		case joined, unicode.Is(unicode.M, r), r >= 0xFE00 && r <= 0xFE0F, r >= 0x1F3FB && r <= 0x1F3FF:
			joined = false
			continue
		}
		n++
	}
	return n
}

// macUpper upper-cases a key that is one ASCII lowercase letter: the only
// letters whose shifted character every layout agrees on (Shift+é is "2"
// on the French layout, not "É").
func macUpper(k platform.Key) platform.Key {
	if len(k) == 1 && k[0] >= 'a' && k[0] <= 'z' {
		return platform.Key(string(rune(k[0] - 'a' + 'A')))
	}
	return k
}

// macModKey describes a modifier key that flagsChanged: reports.
type macModKey struct {
	code   platform.KeyCode
	key    platform.Key
	flag   uint64 // the device-independent bit of the modifier
	device uint64 // the device-dependent bit of this side (0 for Fn)
	pair   uint64 // the device-dependent bits of both sides
	other  uint16 // the virtual key code of the other side (for Fn, itself)
}

const (
	macShiftDevices   = MacDeviceLeftShift | MacDeviceRightShift
	macControlDevices = MacDeviceLeftControl | MacDeviceRightControl
	macOptionDevices  = MacDeviceLeftOption | MacDeviceRightOption
	macCommandDevices = MacDeviceLeftCommand | MacDeviceRightCommand
)

var macModKeys = map[uint16]macModKey{
	kvkShift:        {platform.CodeShiftLeft, platform.KeyShift, MacFlagShift, MacDeviceLeftShift, macShiftDevices, kvkRightShift},
	kvkRightShift:   {platform.CodeShiftRight, platform.KeyShift, MacFlagShift, MacDeviceRightShift, macShiftDevices, kvkShift},
	kvkControl:      {platform.CodeControlLeft, platform.KeyControl, MacFlagControl, MacDeviceLeftControl, macControlDevices, kvkRightControl},
	kvkRightControl: {platform.CodeControlRight, platform.KeyControl, MacFlagControl, MacDeviceRightControl, macControlDevices, kvkControl},
	kvkOption:       {platform.CodeAltLeft, platform.KeyAlt, MacFlagOption, MacDeviceLeftOption, macOptionDevices, kvkRightOption},
	kvkRightOption:  {platform.CodeAltRight, platform.KeyAlt, MacFlagOption, MacDeviceRightOption, macOptionDevices, kvkOption},
	kvkCommand:      {platform.CodeMetaLeft, platform.KeyMeta, MacFlagCommand, MacDeviceLeftCommand, macCommandDevices, kvkRightCommand},
	kvkRightCommand: {platform.CodeMetaRight, platform.KeyMeta, MacFlagCommand, MacDeviceRightCommand, macCommandDevices, kvkCommand},
	kvkFunction:     {platform.CodeFn, platform.KeyFn, MacFlagFunction, 0, 0, kvkFunction},
}

// MacFlagsChanged translates a flagsChanged: event, which macOS sends when
// a modifier key goes down or up, or when Caps Lock changes its state,
// without saying which. keyCode is the event's [NSEvent keyCode], flags its
// modifierFlags and prev the modifierFlags of the previous flagsChanged:
// event (or +[NSEvent modifierFlags] when the window gained focus); held is
// the window's Held set, only read, which tells a press from a release
// where the flags cannot (nil is an empty set). The events come back in
// order, for the backend to pass through held.Accept:
//
//   - Shift, Control, Option and Command: a press or a release of the
//     key's side. The device-dependent bit of the side says whether it is
//     down, so releasing one of two held Shifts is a release while Mods
//     keeps ModShift. Without device-dependent bits (events that other
//     programs post), the modifier turning on is a press, turning off a
//     release, and an event that leaves it on toggles the key in held.
//     When the modifier turns off, the other side, if held, is released
//     too (its own event was lost or coalesced).
//   - Fn: a press while MacFlagFunction is on, a release when it is off.
//   - Caps Lock: macOS reports the lock turning on and off, not the key, so
//     a change of MacFlagCapsLock from prev is a press followed by a
//     release (as SDL does), and no change gives nothing.
//   - A key that is already in the state the flags give (a press of a
//     held key, a release of a key held does not have) gives nothing, and
//     so does any other keyCode.
//
// Every event has the Mods of flags, no Text and no Repeat.
func MacFlagsChanged(held *Held, keyCode uint16, prev, flags uint64, t time.Duration) []platform.KeyEvent {
	mods := MacModifiers(flags)
	event := func(a platform.KeyAction, c platform.KeyCode, k platform.Key) platform.KeyEvent {
		return platform.KeyEvent{Action: a, Code: c, Key: k, Location: c.Location(), Mods: mods, Time: t}
	}
	isHeld := func(c platform.KeyCode) bool { return held != nil && held.IsHeld(c) }
	if keyCode == kvkCapsLock {
		if (prev^flags)&MacFlagCapsLock == 0 {
			return nil
		}
		return []platform.KeyEvent{
			event(platform.KeyPress, platform.CodeCapsLock, platform.KeyCapsLock),
			event(platform.KeyRelease, platform.CodeCapsLock, platform.KeyCapsLock),
		}
	}
	m, ok := macModKeys[keyCode]
	if !ok {
		return nil
	}
	var down bool
	switch {
	case flags&m.flag == 0:
		down = false
	case m.device != 0 && (flags|prev)&m.pair != 0:
		down = flags&m.device != 0
	case prev&m.flag == 0:
		down = true
	default:
		// Still on and nothing tells the sides apart: the other side may
		// be holding the modifier, so this key changed from what held says.
		down = !isHeld(m.code)
	}
	var out []platform.KeyEvent
	switch h := isHeld(m.code); {
	case down && !h:
		out = append(out, event(platform.KeyPress, m.code, m.key))
	case !down && h:
		out = append(out, event(platform.KeyRelease, m.code, m.key))
	}
	if flags&m.flag == 0 && m.other != keyCode {
		if o := macModKeys[m.other]; isHeld(o.code) {
			out = append(out, event(platform.KeyRelease, o.code, o.key))
		}
	}
	return out
}
