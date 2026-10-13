package platform

import (
	"fmt"
	"sync"
)

// KeyCode is a physical key: a position on the keyboard, independent of the
// layout (the key labeled Q on a US keyboard is CodeKeyQ on an AZERTY one,
// where it types "a"). Its String is the W3C UI Events KeyboardEvent "code"
// value (https://www.w3.org/TR/uievents-code/), which is also the DOM
// KeyboardEvent.code a browser reports for the key.
//
// It follows winit's PhysicalKey: a key the backend cannot identify is
// still distinguishable, because the unidentified value carries the
// backend's native scancode ([UnidentifiedKeyCode]). Two presses of the same
// unknown key compare equal, so press and release can be matched and keys
// bound, and the zero KeyCode is [CodeUnidentified] (no native code known).
// Test for a known key with [KeyCode.Identified], not by comparing with
// CodeUnidentified.
//
// Names follow the W3C specification, not winit: MetaLeft/MetaRight for the
// Windows, Command and Super keys (winit says SuperLeft/SuperRight), and
// Meta and Super are separate legacy codes. Function keys go up to F24, the
// highest the Linux, Windows and macOS scancode sets report.
type KeyCode uint32

// The native scancode carried by an unidentified KeyCode is the backend's
// own: on Linux the evdev code (Wayland keycode - 8, X11 keycode - 8); on
// Win32 the scan code with 0xE000 set for extended keys; on macOS the
// virtual key code.
const (
	nativeKeyCodeFlag KeyCode = 1 << 31
	maxNativeKeyCode          = 1<<31 - 1
)

// Physical key codes, grouped as in the W3C code specification.
const (
	// CodeUnidentified is a key the backend could not identify and whose
	// native scancode is unknown. See also UnidentifiedKeyCode.
	CodeUnidentified KeyCode = iota

	// Writing system keys (alphanumeric section). The comments give the
	// US layout label.
	CodeBackquote     // `~ (半角/全角 on Japanese keyboards)
	CodeBackslash     // \| (# on UK layouts, row C)
	CodeBracketLeft   // [{
	CodeBracketRight  // ]}
	CodeComma         // ,<
	CodeDigit0        // 0)
	CodeDigit1        // 1!
	CodeDigit2        // 2@
	CodeDigit3        // 3#
	CodeDigit4        // 4$
	CodeDigit5        // 5%
	CodeDigit6        // 6^
	CodeDigit7        // 7&
	CodeDigit8        // 8*
	CodeDigit9        // 9(
	CodeEqual         // =+
	CodeIntlBackslash // between left Shift and Z on ISO keyboards
	CodeIntlRo        // ろ, between / and right Shift (Japanese)
	CodeIntlYen       // ¥, between = and Backspace (Japanese)
	CodeKeyA
	CodeKeyB
	CodeKeyC
	CodeKeyD
	CodeKeyE
	CodeKeyF
	CodeKeyG
	CodeKeyH
	CodeKeyI
	CodeKeyJ
	CodeKeyK
	CodeKeyL
	CodeKeyM
	CodeKeyN
	CodeKeyO
	CodeKeyP
	CodeKeyQ
	CodeKeyR
	CodeKeyS
	CodeKeyT
	CodeKeyU
	CodeKeyV
	CodeKeyW
	CodeKeyX
	CodeKeyY
	CodeKeyZ
	CodeMinus     // -_
	CodePeriod    // .>
	CodeQuote     // '"
	CodeSemicolon // ;:
	CodeSlash     // /?

	// Functional keys of the alphanumeric section.
	CodeAltLeft
	CodeAltRight // AltGr on many layouts
	CodeBackspace
	CodeCapsLock
	CodeContextMenu
	CodeControlLeft
	CodeControlRight
	CodeEnter
	CodeMetaLeft  // Windows, Command (⌘) or Super key
	CodeMetaRight // Windows, Command (⌘) or Super key
	CodeShiftLeft
	CodeShiftRight
	CodeSpace
	CodeTab
	CodeConvert    // 変換 (Japanese)
	CodeKanaMode   // カタカナ/ひらがな/ローマ字 (Japanese)
	CodeLang1      // 한/영 (Korean), かな (Apple Japanese)
	CodeLang2      // 한자 (Korean), 英数 (Apple Japanese)
	CodeLang3      // カタカナ (Japanese)
	CodeLang4      // ひらがな (Japanese)
	CodeLang5      // 半角/全角 (Japanese)
	CodeNonConvert // 無変換 (Japanese)

	// Control pad.
	CodeDelete
	CodeEnd
	CodeHelp
	CodeHome
	CodeInsert
	CodePageDown
	CodePageUp

	// Arrow pad.
	CodeArrowDown
	CodeArrowLeft
	CodeArrowRight
	CodeArrowUp

	// Numpad section.
	CodeNumLock
	CodeNumpad0
	CodeNumpad1
	CodeNumpad2
	CodeNumpad3
	CodeNumpad4
	CodeNumpad5
	CodeNumpad6
	CodeNumpad7
	CodeNumpad8
	CodeNumpad9
	CodeNumpadAdd
	CodeNumpadBackspace
	CodeNumpadClear
	CodeNumpadClearEntry
	CodeNumpadComma
	CodeNumpadDecimal
	CodeNumpadDivide
	CodeNumpadEnter
	CodeNumpadEqual
	CodeNumpadHash
	CodeNumpadMemoryAdd
	CodeNumpadMemoryClear
	CodeNumpadMemoryRecall
	CodeNumpadMemoryStore
	CodeNumpadMemorySubtract
	CodeNumpadMultiply
	CodeNumpadParenLeft
	CodeNumpadParenRight
	CodeNumpadStar
	CodeNumpadSubtract

	// Function section.
	CodeEscape
	CodeF1
	CodeF2
	CodeF3
	CodeF4
	CodeF5
	CodeF6
	CodeF7
	CodeF8
	CodeF9
	CodeF10
	CodeF11
	CodeF12
	CodeF13
	CodeF14
	CodeF15
	CodeF16
	CodeF17
	CodeF18
	CodeF19
	CodeF20
	CodeF21
	CodeF22
	CodeF23
	CodeF24
	CodeFn
	CodeFnLock
	CodePrintScreen
	CodeScrollLock
	CodePause

	// Media keys.
	CodeBrowserBack
	CodeBrowserFavorites
	CodeBrowserForward
	CodeBrowserHome
	CodeBrowserRefresh
	CodeBrowserSearch
	CodeBrowserStop
	CodeEject
	CodeLaunchApp1 // often "My Computer"
	CodeLaunchApp2 // often "Calculator"
	CodeLaunchMail
	CodeMediaPlayPause
	CodeMediaSelect
	CodeMediaStop
	CodeMediaTrackNext
	CodeMediaTrackPrevious
	CodePower
	CodeSleep
	CodeAudioVolumeDown
	CodeAudioVolumeMute
	CodeAudioVolumeUp
	CodeWakeUp

	// Legacy, non-standard and special keys.
	CodeHyper
	CodeSuper
	CodeTurbo
	CodeAbort
	CodeResume
	CodeSuspend
	CodeAgain
	CodeCopy
	CodeCut
	CodeFind
	CodeOpen
	CodePaste
	CodeProps
	CodeSelect
	CodeUndo
	CodeHiragana
	CodeKatakana

	numKeyCodes // number of identified codes + 1
)

var keyCodeNames = [numKeyCodes]string{
	CodeUnidentified: "Unidentified",

	CodeBackquote: "Backquote", CodeBackslash: "Backslash", CodeBracketLeft: "BracketLeft",
	CodeBracketRight: "BracketRight", CodeComma: "Comma",
	CodeDigit0: "Digit0", CodeDigit1: "Digit1", CodeDigit2: "Digit2", CodeDigit3: "Digit3", CodeDigit4: "Digit4",
	CodeDigit5: "Digit5", CodeDigit6: "Digit6", CodeDigit7: "Digit7", CodeDigit8: "Digit8", CodeDigit9: "Digit9",
	CodeEqual: "Equal", CodeIntlBackslash: "IntlBackslash", CodeIntlRo: "IntlRo", CodeIntlYen: "IntlYen",
	CodeKeyA: "KeyA", CodeKeyB: "KeyB", CodeKeyC: "KeyC", CodeKeyD: "KeyD", CodeKeyE: "KeyE", CodeKeyF: "KeyF",
	CodeKeyG: "KeyG", CodeKeyH: "KeyH", CodeKeyI: "KeyI", CodeKeyJ: "KeyJ", CodeKeyK: "KeyK", CodeKeyL: "KeyL",
	CodeKeyM: "KeyM", CodeKeyN: "KeyN", CodeKeyO: "KeyO", CodeKeyP: "KeyP", CodeKeyQ: "KeyQ", CodeKeyR: "KeyR",
	CodeKeyS: "KeyS", CodeKeyT: "KeyT", CodeKeyU: "KeyU", CodeKeyV: "KeyV", CodeKeyW: "KeyW", CodeKeyX: "KeyX",
	CodeKeyY: "KeyY", CodeKeyZ: "KeyZ",
	CodeMinus: "Minus", CodePeriod: "Period", CodeQuote: "Quote", CodeSemicolon: "Semicolon", CodeSlash: "Slash",

	CodeAltLeft: "AltLeft", CodeAltRight: "AltRight", CodeBackspace: "Backspace", CodeCapsLock: "CapsLock",
	CodeContextMenu: "ContextMenu", CodeControlLeft: "ControlLeft", CodeControlRight: "ControlRight",
	CodeEnter: "Enter", CodeMetaLeft: "MetaLeft", CodeMetaRight: "MetaRight", CodeShiftLeft: "ShiftLeft",
	CodeShiftRight: "ShiftRight", CodeSpace: "Space", CodeTab: "Tab", CodeConvert: "Convert",
	CodeKanaMode: "KanaMode", CodeLang1: "Lang1", CodeLang2: "Lang2", CodeLang3: "Lang3", CodeLang4: "Lang4",
	CodeLang5: "Lang5", CodeNonConvert: "NonConvert",

	CodeDelete: "Delete", CodeEnd: "End", CodeHelp: "Help", CodeHome: "Home", CodeInsert: "Insert",
	CodePageDown: "PageDown", CodePageUp: "PageUp",

	CodeArrowDown: "ArrowDown", CodeArrowLeft: "ArrowLeft", CodeArrowRight: "ArrowRight", CodeArrowUp: "ArrowUp",

	CodeNumLock: "NumLock",
	CodeNumpad0: "Numpad0", CodeNumpad1: "Numpad1", CodeNumpad2: "Numpad2", CodeNumpad3: "Numpad3",
	CodeNumpad4: "Numpad4", CodeNumpad5: "Numpad5", CodeNumpad6: "Numpad6", CodeNumpad7: "Numpad7",
	CodeNumpad8: "Numpad8", CodeNumpad9: "Numpad9",
	CodeNumpadAdd: "NumpadAdd", CodeNumpadBackspace: "NumpadBackspace", CodeNumpadClear: "NumpadClear",
	CodeNumpadClearEntry: "NumpadClearEntry", CodeNumpadComma: "NumpadComma", CodeNumpadDecimal: "NumpadDecimal",
	CodeNumpadDivide: "NumpadDivide", CodeNumpadEnter: "NumpadEnter", CodeNumpadEqual: "NumpadEqual",
	CodeNumpadHash: "NumpadHash", CodeNumpadMemoryAdd: "NumpadMemoryAdd", CodeNumpadMemoryClear: "NumpadMemoryClear",
	CodeNumpadMemoryRecall: "NumpadMemoryRecall", CodeNumpadMemoryStore: "NumpadMemoryStore",
	CodeNumpadMemorySubtract: "NumpadMemorySubtract", CodeNumpadMultiply: "NumpadMultiply",
	CodeNumpadParenLeft: "NumpadParenLeft", CodeNumpadParenRight: "NumpadParenRight", CodeNumpadStar: "NumpadStar",
	CodeNumpadSubtract: "NumpadSubtract",

	CodeEscape: "Escape",

	CodeF1: "F1", CodeF2: "F2", CodeF3: "F3", CodeF4: "F4", CodeF5: "F5", CodeF6: "F6", CodeF7: "F7", CodeF8: "F8",
	CodeF9: "F9", CodeF10: "F10", CodeF11: "F11", CodeF12: "F12", CodeF13: "F13", CodeF14: "F14", CodeF15: "F15",
	CodeF16: "F16", CodeF17: "F17", CodeF18: "F18", CodeF19: "F19", CodeF20: "F20", CodeF21: "F21", CodeF22: "F22",
	CodeF23: "F23", CodeF24: "F24",
	CodeFn: "Fn", CodeFnLock: "FnLock", CodePrintScreen: "PrintScreen", CodeScrollLock: "ScrollLock",
	CodePause: "Pause",

	CodeBrowserBack: "BrowserBack", CodeBrowserFavorites: "BrowserFavorites", CodeBrowserForward: "BrowserForward",
	CodeBrowserHome: "BrowserHome", CodeBrowserRefresh: "BrowserRefresh", CodeBrowserSearch: "BrowserSearch",
	CodeBrowserStop: "BrowserStop", CodeEject: "Eject", CodeLaunchApp1: "LaunchApp1", CodeLaunchApp2: "LaunchApp2",
	CodeLaunchMail: "LaunchMail", CodeMediaPlayPause: "MediaPlayPause", CodeMediaSelect: "MediaSelect",
	CodeMediaStop: "MediaStop", CodeMediaTrackNext: "MediaTrackNext", CodeMediaTrackPrevious: "MediaTrackPrevious",
	CodePower: "Power", CodeSleep: "Sleep", CodeAudioVolumeDown: "AudioVolumeDown",
	CodeAudioVolumeMute: "AudioVolumeMute", CodeAudioVolumeUp: "AudioVolumeUp", CodeWakeUp: "WakeUp",

	CodeHyper: "Hyper", CodeSuper: "Super", CodeTurbo: "Turbo", CodeAbort: "Abort", CodeResume: "Resume",
	CodeSuspend: "Suspend", CodeAgain: "Again", CodeCopy: "Copy", CodeCut: "Cut", CodeFind: "Find",
	CodeOpen: "Open", CodePaste: "Paste", CodeProps: "Props", CodeSelect: "Select", CodeUndo: "Undo",
	CodeHiragana: "Hiragana", CodeKatakana: "Katakana",
}

// UnidentifiedKeyCode returns the KeyCode of a key the backend cannot map
// to a W3C code, carrying its native scancode so that it still compares
// equal to itself and differently from other unknown keys. It panics if
// native does not fit in 31 bits.
func UnidentifiedKeyCode(native uint32) KeyCode {
	if native > maxNativeKeyCode {
		panic(fmt.Sprintf("platform: UnidentifiedKeyCode(%#x): native scancodes must fit in 31 bits", native))
	}
	return nativeKeyCodeFlag | KeyCode(native)
}

// Identified reports whether c is one of the W3C codes (not
// CodeUnidentified, with or without a native scancode).
func (c KeyCode) Identified() bool { return c > CodeUnidentified && c < numKeyCodes }

// Native returns the native scancode carried by an unidentified KeyCode
// (see UnidentifiedKeyCode); ok is false for identified codes and for
// CodeUnidentified.
func (c KeyCode) Native() (scancode uint32, ok bool) {
	if c&nativeKeyCodeFlag == 0 {
		return 0, false
	}
	return uint32(c &^ nativeKeyCodeFlag), true
}

// String returns the W3C "code" value ("KeyA", "ShiftLeft", "Numpad0"),
// "Unidentified" for unidentified keys, or "KeyCode(n)" for values that
// are neither.
func (c KeyCode) String() string {
	if c < numKeyCodes {
		return keyCodeNames[c]
	}
	if c&nativeKeyCodeFlag != 0 {
		return "Unidentified"
	}
	return fmt.Sprintf("KeyCode(%d)", uint32(c))
}

// GoString includes the native scancode of unidentified keys, for
// debugging (fmt's %#v).
func (c KeyCode) GoString() string {
	if n, ok := c.Native(); ok {
		return fmt.Sprintf("platform.UnidentifiedKeyCode(%#x)", n)
	}
	if c < numKeyCodes {
		return "platform.Code" + keyCodeNames[c]
	}
	return c.String()
}

var keyCodesByName = sync.OnceValue(func() map[string]KeyCode {
	m := make(map[string]KeyCode, numKeyCodes)
	for c := CodeUnidentified + 1; c < numKeyCodes; c++ {
		m[keyCodeNames[c]] = c
	}
	return m
})

// ParseKeyCode returns the KeyCode whose W3C "code" value is name ("KeyA",
// "Enter"); ok is false for unknown names and for "Unidentified".
func ParseKeyCode(name string) (c KeyCode, ok bool) {
	c, ok = keyCodesByName()[name]
	return c, ok
}

// Location returns the location of the physical key: left or right for the
// modifier keys that come in pairs, numpad for the numeric keypad (not
// NumLock, which the DOM reports as standard), standard otherwise. Backends
// set KeyEvent.Location from it.
func (c KeyCode) Location() KeyLocation {
	switch c {
	case CodeShiftLeft, CodeControlLeft, CodeAltLeft, CodeMetaLeft:
		return KeyLocationLeft
	case CodeShiftRight, CodeControlRight, CodeAltRight, CodeMetaRight:
		return KeyLocationRight
	}
	if c >= CodeNumpad0 && c <= CodeNumpadSubtract {
		return KeyLocationNumpad
	}
	return KeyLocationStandard
}
