// Package evdev maps Linux evdev key codes (linux/input-event-codes.h) to
// physical keys (platform.KeyCode, W3C "code" values). Both Linux backends
// use it: a Wayland wl_keyboard.key carries the evdev code, and an X11
// keycode is the evdev code plus 8 (the offset of the xf86-input-evdev and
// libinput drivers, which Xwayland and every modern Xorg use).
//
// The table follows winit's scancode_to_physicalkey and extends it, through
// the USB HID usages that both the kernel (drivers/hid/hid-input.c) and the
// W3C code specification are defined against, with the legacy editing keys
// (KEY_AGAIN … KEY_HELP), the browser, launch and power keys, the numpad
// parentheses and KEY_FN. Codes with no W3C equivalent (KEY_MACRO,
// KEY_KPPLUSMINUS, KEY_KPJPCOMMA, brightness and display keys, …) map to
// platform.UnidentifiedKeyCode with the evdev code, so they remain
// distinguishable.
package evdev

import (
	"sync"

	"github.com/kuronosu/tela/internal/platform"
)

// X11Offset is the difference between an X11 (and XKB) keycode and the
// evdev code of the same key.
const X11Offset = 8

// codes maps evdev codes to physical keys. The comments name the KEY_*
// constants of linux/input-event-codes.h.
var codes = [...]platform.KeyCode{
	1:   platform.CodeEscape,             // KEY_ESC
	2:   platform.CodeDigit1,             // KEY_1
	3:   platform.CodeDigit2,             // KEY_2
	4:   platform.CodeDigit3,             // KEY_3
	5:   platform.CodeDigit4,             // KEY_4
	6:   platform.CodeDigit5,             // KEY_5
	7:   platform.CodeDigit6,             // KEY_6
	8:   platform.CodeDigit7,             // KEY_7
	9:   platform.CodeDigit8,             // KEY_8
	10:  platform.CodeDigit9,             // KEY_9
	11:  platform.CodeDigit0,             // KEY_0
	12:  platform.CodeMinus,              // KEY_MINUS
	13:  platform.CodeEqual,              // KEY_EQUAL
	14:  platform.CodeBackspace,          // KEY_BACKSPACE
	15:  platform.CodeTab,                // KEY_TAB
	16:  platform.CodeKeyQ,               // KEY_Q
	17:  platform.CodeKeyW,               // KEY_W
	18:  platform.CodeKeyE,               // KEY_E
	19:  platform.CodeKeyR,               // KEY_R
	20:  platform.CodeKeyT,               // KEY_T
	21:  platform.CodeKeyY,               // KEY_Y
	22:  platform.CodeKeyU,               // KEY_U
	23:  platform.CodeKeyI,               // KEY_I
	24:  platform.CodeKeyO,               // KEY_O
	25:  platform.CodeKeyP,               // KEY_P
	26:  platform.CodeBracketLeft,        // KEY_LEFTBRACE
	27:  platform.CodeBracketRight,       // KEY_RIGHTBRACE
	28:  platform.CodeEnter,              // KEY_ENTER
	29:  platform.CodeControlLeft,        // KEY_LEFTCTRL
	30:  platform.CodeKeyA,               // KEY_A
	31:  platform.CodeKeyS,               // KEY_S
	32:  platform.CodeKeyD,               // KEY_D
	33:  platform.CodeKeyF,               // KEY_F
	34:  platform.CodeKeyG,               // KEY_G
	35:  platform.CodeKeyH,               // KEY_H
	36:  platform.CodeKeyJ,               // KEY_J
	37:  platform.CodeKeyK,               // KEY_K
	38:  platform.CodeKeyL,               // KEY_L
	39:  platform.CodeSemicolon,          // KEY_SEMICOLON
	40:  platform.CodeQuote,              // KEY_APOSTROPHE
	41:  platform.CodeBackquote,          // KEY_GRAVE
	42:  platform.CodeShiftLeft,          // KEY_LEFTSHIFT
	43:  platform.CodeBackslash,          // KEY_BACKSLASH
	44:  platform.CodeKeyZ,               // KEY_Z
	45:  platform.CodeKeyX,               // KEY_X
	46:  platform.CodeKeyC,               // KEY_C
	47:  platform.CodeKeyV,               // KEY_V
	48:  platform.CodeKeyB,               // KEY_B
	49:  platform.CodeKeyN,               // KEY_N
	50:  platform.CodeKeyM,               // KEY_M
	51:  platform.CodeComma,              // KEY_COMMA
	52:  platform.CodePeriod,             // KEY_DOT
	53:  platform.CodeSlash,              // KEY_SLASH
	54:  platform.CodeShiftRight,         // KEY_RIGHTSHIFT
	55:  platform.CodeNumpadMultiply,     // KEY_KPASTERISK
	56:  platform.CodeAltLeft,            // KEY_LEFTALT
	57:  platform.CodeSpace,              // KEY_SPACE
	58:  platform.CodeCapsLock,           // KEY_CAPSLOCK
	59:  platform.CodeF1,                 // KEY_F1
	60:  platform.CodeF2,                 // KEY_F2
	61:  platform.CodeF3,                 // KEY_F3
	62:  platform.CodeF4,                 // KEY_F4
	63:  platform.CodeF5,                 // KEY_F5
	64:  platform.CodeF6,                 // KEY_F6
	65:  platform.CodeF7,                 // KEY_F7
	66:  platform.CodeF8,                 // KEY_F8
	67:  platform.CodeF9,                 // KEY_F9
	68:  platform.CodeF10,                // KEY_F10
	69:  platform.CodeNumLock,            // KEY_NUMLOCK
	70:  platform.CodeScrollLock,         // KEY_SCROLLLOCK
	71:  platform.CodeNumpad7,            // KEY_KP7
	72:  platform.CodeNumpad8,            // KEY_KP8
	73:  platform.CodeNumpad9,            // KEY_KP9
	74:  platform.CodeNumpadSubtract,     // KEY_KPMINUS
	75:  platform.CodeNumpad4,            // KEY_KP4
	76:  platform.CodeNumpad5,            // KEY_KP5
	77:  platform.CodeNumpad6,            // KEY_KP6
	78:  platform.CodeNumpadAdd,          // KEY_KPPLUS
	79:  platform.CodeNumpad1,            // KEY_KP1
	80:  platform.CodeNumpad2,            // KEY_KP2
	81:  platform.CodeNumpad3,            // KEY_KP3
	82:  platform.CodeNumpad0,            // KEY_KP0
	83:  platform.CodeNumpadDecimal,      // KEY_KPDOT
	85:  platform.CodeLang5,              // KEY_ZENKAKUHANKAKU (HID LANG5)
	86:  platform.CodeIntlBackslash,      // KEY_102ND
	87:  platform.CodeF11,                // KEY_F11
	88:  platform.CodeF12,                // KEY_F12
	89:  platform.CodeIntlRo,             // KEY_RO
	90:  platform.CodeLang3,              // KEY_KATAKANA (HID LANG3)
	91:  platform.CodeLang4,              // KEY_HIRAGANA (HID LANG4)
	92:  platform.CodeConvert,            // KEY_HENKAN
	93:  platform.CodeKanaMode,           // KEY_KATAKANAHIRAGANA
	94:  platform.CodeNonConvert,         // KEY_MUHENKAN
	96:  platform.CodeNumpadEnter,        // KEY_KPENTER
	97:  platform.CodeControlRight,       // KEY_RIGHTCTRL
	98:  platform.CodeNumpadDivide,       // KEY_KPSLASH
	99:  platform.CodePrintScreen,        // KEY_SYSRQ
	100: platform.CodeAltRight,           // KEY_RIGHTALT
	102: platform.CodeHome,               // KEY_HOME
	103: platform.CodeArrowUp,            // KEY_UP
	104: platform.CodePageUp,             // KEY_PAGEUP
	105: platform.CodeArrowLeft,          // KEY_LEFT
	106: platform.CodeArrowRight,         // KEY_RIGHT
	107: platform.CodeEnd,                // KEY_END
	108: platform.CodeArrowDown,          // KEY_DOWN
	109: platform.CodePageDown,           // KEY_PAGEDOWN
	110: platform.CodeInsert,             // KEY_INSERT
	111: platform.CodeDelete,             // KEY_DELETE
	113: platform.CodeAudioVolumeMute,    // KEY_MUTE
	114: platform.CodeAudioVolumeDown,    // KEY_VOLUMEDOWN
	115: platform.CodeAudioVolumeUp,      // KEY_VOLUMEUP
	116: platform.CodePower,              // KEY_POWER
	117: platform.CodeNumpadEqual,        // KEY_KPEQUAL
	119: platform.CodePause,              // KEY_PAUSE
	121: platform.CodeNumpadComma,        // KEY_KPCOMMA
	122: platform.CodeLang1,              // KEY_HANGEUL (HID LANG1)
	123: platform.CodeLang2,              // KEY_HANJA (HID LANG2)
	124: platform.CodeIntlYen,            // KEY_YEN
	125: platform.CodeMetaLeft,           // KEY_LEFTMETA
	126: platform.CodeMetaRight,          // KEY_RIGHTMETA
	127: platform.CodeContextMenu,        // KEY_COMPOSE (HID Application)
	128: platform.CodeBrowserStop,        // KEY_STOP (HID AC Stop)
	129: platform.CodeAgain,              // KEY_AGAIN
	130: platform.CodeProps,              // KEY_PROPS
	131: platform.CodeUndo,               // KEY_UNDO
	132: platform.CodeSelect,             // KEY_FRONT (HID Select)
	133: platform.CodeCopy,               // KEY_COPY
	134: platform.CodeOpen,               // KEY_OPEN (HID Execute)
	135: platform.CodePaste,              // KEY_PASTE
	136: platform.CodeFind,               // KEY_FIND
	137: platform.CodeCut,                // KEY_CUT
	138: platform.CodeHelp,               // KEY_HELP
	140: platform.CodeLaunchApp2,         // KEY_CALC (HID AL Calculator)
	142: platform.CodeSleep,              // KEY_SLEEP
	143: platform.CodeWakeUp,             // KEY_WAKEUP
	144: platform.CodeLaunchApp1,         // KEY_FILE (HID AL Local Machine Browser)
	155: platform.CodeLaunchMail,         // KEY_MAIL
	156: platform.CodeBrowserFavorites,   // KEY_BOOKMARKS
	158: platform.CodeBrowserBack,        // KEY_BACK
	159: platform.CodeBrowserForward,     // KEY_FORWARD
	161: platform.CodeEject,              // KEY_EJECTCD
	163: platform.CodeMediaTrackNext,     // KEY_NEXTSONG
	164: platform.CodeMediaPlayPause,     // KEY_PLAYPAUSE
	165: platform.CodeMediaTrackPrevious, // KEY_PREVIOUSSONG
	166: platform.CodeMediaStop,          // KEY_STOPCD
	171: platform.CodeMediaSelect,        // KEY_CONFIG (HID AL Consumer Control Configuration)
	172: platform.CodeBrowserHome,        // KEY_HOMEPAGE
	173: platform.CodeBrowserRefresh,     // KEY_REFRESH
	179: platform.CodeNumpadParenLeft,    // KEY_KPLEFTPAREN
	180: platform.CodeNumpadParenRight,   // KEY_KPRIGHTPAREN
	183: platform.CodeF13,                // KEY_F13
	184: platform.CodeF14,                // KEY_F14
	185: platform.CodeF15,                // KEY_F15
	186: platform.CodeF16,                // KEY_F16
	187: platform.CodeF17,                // KEY_F17
	188: platform.CodeF18,                // KEY_F18
	189: platform.CodeF19,                // KEY_F19
	190: platform.CodeF20,                // KEY_F20
	191: platform.CodeF21,                // KEY_F21
	192: platform.CodeF22,                // KEY_F22
	193: platform.CodeF23,                // KEY_F23
	194: platform.CodeF24,                // KEY_F24
	205: platform.CodeSuspend,            // KEY_SUSPEND
	217: platform.CodeBrowserSearch,      // KEY_SEARCH
	464: platform.CodeFn,                 // KEY_FN
}

// KeyCode returns the physical key of an evdev code. Codes without a W3C
// equivalent (and KEY_RESERVED, 0) return platform.UnidentifiedKeyCode(code).
func KeyCode(code uint32) platform.KeyCode {
	if code < uint32(len(codes)) {
		if c := codes[code]; c != platform.CodeUnidentified {
			return c
		}
	}
	if code > 1<<31-1 {
		return platform.CodeUnidentified
	}
	return platform.UnidentifiedKeyCode(code)
}

// X11KeyCode returns the physical key of an X11 keycode (evdev code + 8).
// Keycodes below 8 do not exist in X11 and return CodeUnidentified.
func X11KeyCode(keycode uint32) platform.KeyCode {
	if keycode < X11Offset {
		return platform.CodeUnidentified
	}
	return KeyCode(keycode - X11Offset)
}

var fromKeyCode = sync.OnceValue(func() map[platform.KeyCode]uint32 {
	m := make(map[platform.KeyCode]uint32, len(codes))
	for code, c := range codes {
		if c != platform.CodeUnidentified {
			m[c] = uint32(code)
		}
	}
	return m
})

// Code returns the evdev code of a physical key: the inverse of KeyCode,
// including the native code of an unidentified key. ok is false for keys
// that have no evdev code (CodeUnidentified, NumpadHash, F-keys above F24
// and other W3C codes Linux keyboards do not report). Tests and synthetic
// input use it.
func Code(c platform.KeyCode) (code uint32, ok bool) {
	if n, native := c.Native(); native {
		return n, true
	}
	code, ok = fromKeyCode()[c]
	return code, ok
}
