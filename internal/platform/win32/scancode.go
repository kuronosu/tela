package win32

import "github.com/kuronosu/tela/internal/platform"

// Windows reports the physical key of a keyboard message as a scan code: bits
// 16 to 23 of lParam, plus the "extended key" flag in bit 24, which stands
// for the E0 prefix of the PS/2 set 1 scan codes. Tela folds the two into
// one number with 0xE000 set for extended keys, the form
// MapVirtualKeyExW(MAPVK_VK_TO_VSC_EX) returns and the W3C UI Events "code"
// table lists for Windows. The table follows that specification, Firefox's
// NativeKeyToDOMCodeName.h and winit's scancode_to_physicalkey.

const extendedScancode = 0xE000

// scancodeFromLParam decodes the scan code of a WM_KEYDOWN, WM_KEYUP,
// WM_SYSKEYDOWN or WM_SYSKEYUP lParam; it is 0 for a device that reports
// none (some media keys).
func scancodeFromLParam(lparam uintptr) uint32 {
	sc := uint32(lparam>>16) & 0xFF
	if lparam&(1<<24) != 0 {
		sc |= extendedScancode
	}
	return sc
}

// keyRepeat reports whether a WM_KEYDOWN lParam is an auto-repeat: the
// previous-state bit (30) says the key was already down. A WM_KEYUP has it
// set too and is never a repeat; callers look at the message.
func keyRepeat(lparam uintptr) bool { return lparam&(1<<30) != 0 }

// codeFromScancode returns the physical key of a Windows scan code. Keys the
// table does not know come back as platform.UnidentifiedKeyCode with the scan
// code, so they still compare equal to themselves.
func codeFromScancode(sc uint32) platform.KeyCode {
	if c, ok := scancodes[sc]; ok {
		return c
	}
	if sc == 0 {
		return platform.CodeUnidentified
	}
	return platform.UnidentifiedKeyCode(sc)
}

var scancodes = map[uint32]platform.KeyCode{
	// Alphanumeric section.
	0x0029: platform.CodeBackquote,
	0x002B: platform.CodeBackslash,
	0x000E: platform.CodeBackspace,
	0x001A: platform.CodeBracketLeft,
	0x001B: platform.CodeBracketRight,
	0x0033: platform.CodeComma,
	0x000B: platform.CodeDigit0,
	0x0002: platform.CodeDigit1,
	0x0003: platform.CodeDigit2,
	0x0004: platform.CodeDigit3,
	0x0005: platform.CodeDigit4,
	0x0006: platform.CodeDigit5,
	0x0007: platform.CodeDigit6,
	0x0008: platform.CodeDigit7,
	0x0009: platform.CodeDigit8,
	0x000A: platform.CodeDigit9,
	0x000D: platform.CodeEqual,
	0x0056: platform.CodeIntlBackslash,
	0x0073: platform.CodeIntlRo,
	0x007D: platform.CodeIntlYen,
	0x001E: platform.CodeKeyA,
	0x0030: platform.CodeKeyB,
	0x002E: platform.CodeKeyC,
	0x0020: platform.CodeKeyD,
	0x0012: platform.CodeKeyE,
	0x0021: platform.CodeKeyF,
	0x0022: platform.CodeKeyG,
	0x0023: platform.CodeKeyH,
	0x0017: platform.CodeKeyI,
	0x0024: platform.CodeKeyJ,
	0x0025: platform.CodeKeyK,
	0x0026: platform.CodeKeyL,
	0x0032: platform.CodeKeyM,
	0x0031: platform.CodeKeyN,
	0x0018: platform.CodeKeyO,
	0x0019: platform.CodeKeyP,
	0x0010: platform.CodeKeyQ,
	0x0013: platform.CodeKeyR,
	0x001F: platform.CodeKeyS,
	0x0014: platform.CodeKeyT,
	0x0016: platform.CodeKeyU,
	0x002F: platform.CodeKeyV,
	0x002D: platform.CodeKeyX,
	0x0015: platform.CodeKeyY,
	0x002C: platform.CodeKeyZ,
	0x0011: platform.CodeKeyW,
	0x000C: platform.CodeMinus,
	0x0034: platform.CodePeriod,
	0x0028: platform.CodeQuote,
	0x0027: platform.CodeSemicolon,
	0x0035: platform.CodeSlash,

	// Functional keys of the alphanumeric section.
	0x0038: platform.CodeAltLeft,
	0xE038: platform.CodeAltRight,
	0x003A: platform.CodeCapsLock,
	0xE05D: platform.CodeContextMenu,
	0x001D: platform.CodeControlLeft,
	0xE01D: platform.CodeControlRight,
	0x001C: platform.CodeEnter,
	0xE05B: platform.CodeMetaLeft,
	0xE05C: platform.CodeMetaRight,
	0x002A: platform.CodeShiftLeft,
	0x0036: platform.CodeShiftRight,
	0x0039: platform.CodeSpace,
	0x000F: platform.CodeTab,
	0x0079: platform.CodeConvert,
	0x0070: platform.CodeKanaMode,
	0x0072: platform.CodeLang1, // Hangul/English on Korean keyboards
	0xE0F2: platform.CodeLang1, // the same key, extended on some Korean keyboards
	0x0071: platform.CodeLang2, // Hanja
	0xE0F1: platform.CodeLang2,
	0x007B: platform.CodeNonConvert,

	// Control pad and arrows (extended; the numpad sends the same scan codes
	// without the flag).
	0xE053: platform.CodeDelete,
	0xE04F: platform.CodeEnd,
	0xE047: platform.CodeHome,
	0xE052: platform.CodeInsert,
	0xE051: platform.CodePageDown,
	0xE049: platform.CodePageUp,
	0xE050: platform.CodeArrowDown,
	0xE04B: platform.CodeArrowLeft,
	0xE04D: platform.CodeArrowRight,
	0xE048: platform.CodeArrowUp,

	// Numpad. Windows flags NumLock as extended; Pause arrives as plain
	// 0x45 (Ctrl+Pause, Break, as 0xE046).
	0xE045: platform.CodeNumLock,
	0x0052: platform.CodeNumpad0,
	0x004F: platform.CodeNumpad1,
	0x0050: platform.CodeNumpad2,
	0x0051: platform.CodeNumpad3,
	0x004B: platform.CodeNumpad4,
	0x004C: platform.CodeNumpad5,
	0x004D: platform.CodeNumpad6,
	0x0047: platform.CodeNumpad7,
	0x0048: platform.CodeNumpad8,
	0x0049: platform.CodeNumpad9,
	0x004E: platform.CodeNumpadAdd,
	0x007E: platform.CodeNumpadComma,
	0x0053: platform.CodeNumpadDecimal,
	0xE035: platform.CodeNumpadDivide,
	0xE01C: platform.CodeNumpadEnter,
	0x0059: platform.CodeNumpadEqual,
	0x0037: platform.CodeNumpadMultiply,
	0x004A: platform.CodeNumpadSubtract,

	// Function section.
	0x0001: platform.CodeEscape,
	0x003B: platform.CodeF1,
	0x003C: platform.CodeF2,
	0x003D: platform.CodeF3,
	0x003E: platform.CodeF4,
	0x003F: platform.CodeF5,
	0x0040: platform.CodeF6,
	0x0041: platform.CodeF7,
	0x0042: platform.CodeF8,
	0x0043: platform.CodeF9,
	0x0044: platform.CodeF10,
	0x0057: platform.CodeF11,
	0x0058: platform.CodeF12,
	0x0064: platform.CodeF13,
	0x0065: platform.CodeF14,
	0x0066: platform.CodeF15,
	0x0067: platform.CodeF16,
	0x0068: platform.CodeF17,
	0x0069: platform.CodeF18,
	0x006A: platform.CodeF19,
	0x006B: platform.CodeF20,
	0x006C: platform.CodeF21,
	0x006D: platform.CodeF22,
	0x006E: platform.CodeF23,
	0x0076: platform.CodeF24,
	0xE037: platform.CodePrintScreen,
	0x0054: platform.CodePrintScreen, // Alt+PrintScreen (SysRq)
	0x0046: platform.CodeScrollLock,
	0x0045: platform.CodePause,
	0xE046: platform.CodePause, // Ctrl+Pause (Break)

	// Media and browser keys.
	0xE06A: platform.CodeBrowserBack,
	0xE066: platform.CodeBrowserFavorites,
	0xE069: platform.CodeBrowserForward,
	0xE032: platform.CodeBrowserHome,
	0xE067: platform.CodeBrowserRefresh,
	0xE065: platform.CodeBrowserSearch,
	0xE068: platform.CodeBrowserStop,
	0xE06B: platform.CodeLaunchApp1,
	0xE021: platform.CodeLaunchApp2,
	0xE06C: platform.CodeLaunchMail,
	0xE022: platform.CodeMediaPlayPause,
	0xE06D: platform.CodeMediaSelect,
	0xE024: platform.CodeMediaStop,
	0xE019: platform.CodeMediaTrackNext,
	0xE010: platform.CodeMediaTrackPrevious,
	0xE05E: platform.CodePower,
	0xE05F: platform.CodeSleep,
	0xE063: platform.CodeWakeUp,
	0xE02E: platform.CodeAudioVolumeDown,
	0xE020: platform.CodeAudioVolumeMute,
	0xE030: platform.CodeAudioVolumeUp,
}

// isFakeShift reports whether a scan code is one of the fake Shift presses
// that PS/2 keyboards send around the navigation keys (and Print Screen) with
// NumLock on or Shift held. Windows normally drops them; if one reaches the
// window it is not a key the user pressed.
func isFakeShift(sc uint32) bool { return sc == 0xE02A || sc == 0xE036 }

// nonRepeating reports whether a physical key never repeats: modifiers and
// locks (ADR-050). Windows repeats the last key pressed, a modifier
// included; the repeats of these keys are dropped.
func nonRepeating(c platform.KeyCode) bool {
	switch c {
	case platform.CodeShiftLeft, platform.CodeShiftRight, platform.CodeControlLeft, platform.CodeControlRight,
		platform.CodeAltLeft, platform.CodeAltRight, platform.CodeMetaLeft, platform.CodeMetaRight,
		platform.CodeCapsLock, platform.CodeNumLock, platform.CodeScrollLock, platform.CodeFn, platform.CodeFnLock:
		return true
	}
	return false
}
