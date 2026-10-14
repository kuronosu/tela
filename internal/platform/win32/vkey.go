package win32

import (
	"strconv"

	"github.com/kuronosu/tela/internal/platform"
)

// Virtual-key codes (WinUser.h) that the keyboard translation names.
const (
	vkBack       = 0x08
	vkTab        = 0x09
	vkClear      = 0x0C
	vkReturn     = 0x0D
	vkPause      = 0x13
	vkCapital    = 0x14
	vkKana       = 0x15 // also VK_HANGUL
	vkJunja      = 0x17
	vkFinal      = 0x18
	vkKanji      = 0x19 // also VK_HANJA
	vkEscape     = 0x1B
	vkConvert    = 0x1C
	vkNonConvert = 0x1D
	vkAccept     = 0x1E
	vkModeChange = 0x1F
	vkSpace      = 0x20
	vkPrior      = 0x21
	vkNext       = 0x22
	vkEnd        = 0x23
	vkHome       = 0x24
	vkLeft       = 0x25
	vkUp         = 0x26
	vkRight      = 0x27
	vkDown       = 0x28
	vkSelect     = 0x29
	vkPrint      = 0x2A
	vkExecute    = 0x2B
	vkSnapshot   = 0x2C
	vkInsert     = 0x2D
	vkDelete     = 0x2E
	vkHelp       = 0x2F
	vkApps       = 0x5D
	vkSleep      = 0x5F
	vkNumpad0    = 0x60
	vkNumpad9    = 0x69
	vkMultiply   = 0x6A
	vkAdd        = 0x6B
	vkSeparator  = 0x6C
	vkSubtract   = 0x6D
	vkDecimal    = 0x6E
	vkDivide     = 0x6F
	vkF1         = 0x70
	vkF4         = 0x73
	vkF10        = 0x79
	vkF24        = 0x87
	vkNumLock    = 0x90
	vkScroll     = 0x91
	vkLShift     = 0xA0
	vkRShift     = 0xA1
	vkLControl   = 0xA2
	vkRControl   = 0xA3
	vkLMenu      = 0xA4
	vkRMenu      = 0xA5
	vkLaunchApp2 = 0xB7
	vkOEM102     = 0xE2
	vkProcessKey = 0xE5
	vkPacket     = 0xE7
	vkAttn       = 0xF6
	vkCrSel      = 0xF7
	vkExSel      = 0xF8
	vkEraseEOF   = 0xF9
	vkPlay       = 0xFA
	vkOEMClear   = 0xFE
)

// Primary language identifiers whose layouts give some virtual keys another
// meaning (VK_KANA is VK_HANGUL, VK_KANJI is VK_HANJA).
const (
	langJapanese = 0x11
	langKorean   = 0x12
)

// numpadVK reports whether vk is a virtual key that only exists for the
// numeric keypad with NumLock on. They produce characters, and the character
// depends on the layout (the decimal key types "." or ","), so they go
// through the layout like letters do.
func numpadVK(vk uint32) bool { return vk >= vkNumpad0 && vk <= vkDivide }

// numpadFallback is the character of a numpad virtual key when the layout
// gives none (ToUnicodeEx fails for the divide key on some systems).
func numpadFallback(vk uint32) string {
	switch {
	case vk >= vkNumpad0 && vk <= vkNumpad9:
		return string(rune('0' + vk - vkNumpad0))
	case vk == vkMultiply:
		return "*"
	case vk == vkAdd:
		return "+"
	case vk == vkSeparator:
		return ","
	case vk == vkSubtract:
		return "-"
	case vk == vkDecimal:
		return "."
	case vk == vkDivide:
		return "/"
	}
	return ""
}

// namedKey returns the logical key of a virtual key that has a name (not a
// character): modifiers, navigation, editing, function and media keys. ok is
// false for the keys that produce characters (letters, digits, OEM keys, the
// space bar, the numpad with NumLock on) and for virtual keys Tela does not
// know; those are resolved through the layout.
//
// code is the physical key of the message, because the generic modifier
// virtual keys (VK_SHIFT, VK_CONTROL, VK_MENU) say nothing about the side.
// altGrLayout says whether the active layout has an AltGr key, which makes a
// right Alt an AltGraph. lang is the primary language id of the layout.
func namedKey(vk uint32, code platform.KeyCode, altGrLayout bool, lang uint16) (key platform.Key, ok bool) {
	// Keys identified by position, not by virtual key. Windows swaps
	// NumLock and Pause when Ctrl is held (Ctrl+NumLock is Pause); the
	// physical key is what the user pressed.
	switch code {
	case platform.CodeNumLock:
		return platform.KeyNumLock, true
	case platform.CodePause:
		return platform.KeyPause, true
	case platform.CodeAltRight:
		if vk == vkMenu || vk == vkRMenu {
			if altGrLayout {
				return platform.KeyAltGraph, true
			}
			return platform.KeyAlt, true
		}
	}
	if vk >= vkF1 && vk <= vkF24 {
		return platform.Key("F" + strconv.Itoa(int(vk-vkF1)+1)), true
	}
	switch vk {
	case vkShift, vkLShift, vkRShift:
		return platform.KeyShift, true
	case vkControl, vkLControl, vkRControl:
		return platform.KeyControl, true
	case vkMenu, vkLMenu, vkRMenu:
		return platform.KeyAlt, true
	case vkLWin, vkRWin:
		return platform.KeyMeta, true
	case vkBack:
		return platform.KeyBackspace, true
	case vkTab:
		return platform.KeyTab, true
	case vkClear, vkOEMClear:
		return platform.KeyClear, true
	case vkReturn:
		return platform.KeyEnter, true
	case vkPause:
		return platform.KeyPause, true
	case vkCapital:
		return platform.KeyCapsLock, true
	case vkKana: // VK_HANGUL
		switch lang {
		case langKorean:
			return platform.KeyHangulMode, true
		case langJapanese:
			return platform.KeyKanaMode, true
		}
	case vkJunja:
		return platform.KeyJunjaMode, true
	case vkFinal:
		return platform.KeyFinalMode, true
	case vkKanji: // VK_HANJA
		switch lang {
		case langKorean:
			return platform.KeyHanjaMode, true
		case langJapanese:
			return platform.KeyKanjiMode, true
		}
	case vkEscape:
		return platform.KeyEscape, true
	case vkConvert:
		return platform.KeyConvert, true
	case vkNonConvert:
		return platform.KeyNonConvert, true
	case vkAccept:
		return platform.KeyAccept, true
	case vkModeChange:
		return platform.KeyModeChange, true
	case vkPrior:
		return platform.KeyPageUp, true
	case vkNext:
		return platform.KeyPageDown, true
	case vkEnd:
		return platform.KeyEnd, true
	case vkHome:
		return platform.KeyHome, true
	case vkLeft:
		return platform.KeyArrowLeft, true
	case vkUp:
		return platform.KeyArrowUp, true
	case vkRight:
		return platform.KeyArrowRight, true
	case vkDown:
		return platform.KeyArrowDown, true
	case vkSelect:
		return platform.KeySelect, true
	case vkPrint:
		return platform.KeyPrint, true
	case vkExecute:
		return platform.KeyExecute, true
	case vkSnapshot:
		return platform.KeyPrintScreen, true
	case vkInsert:
		return platform.KeyInsert, true
	case vkDelete:
		return platform.KeyDelete, true
	case vkHelp:
		return platform.KeyHelp, true
	case vkApps:
		return platform.KeyContextMenu, true
	case vkSleep:
		return platform.KeyStandby, true
	case vkNumLock:
		return platform.KeyNumLock, true
	case vkScroll:
		return platform.KeyScrollLock, true
	case vkProcessKey:
		return platform.KeyProcess, true
	case vkAttn:
		return platform.KeyAttn, true
	case vkCrSel:
		return platform.KeyCrSel, true
	case vkExSel:
		return platform.KeyExSel, true
	case vkEraseEOF:
		return platform.KeyEraseEof, true
	case vkPlay:
		return platform.KeyPlay, true
	case 0xF0: // VK_OEM_ATTN: Eisu on Japanese keyboards
		return platform.KeyAlphanumeric, true
	case 0xF1: // VK_OEM_FINISH
		return platform.KeyKatakana, true
	case 0xF2: // VK_OEM_COPY
		return platform.KeyHiragana, true
	case 0xF3: // VK_OEM_AUTO
		return platform.KeyHankaku, true
	case 0xF4: // VK_OEM_ENLW
		return platform.KeyZenkaku, true
	case 0xF5: // VK_OEM_BACKTAB
		return platform.KeyRomaji, true
	case 0xA6:
		return platform.KeyBrowserBack, true
	case 0xA7:
		return platform.KeyBrowserForward, true
	case 0xA8:
		return platform.KeyBrowserRefresh, true
	case 0xA9:
		return platform.KeyBrowserStop, true
	case 0xAA:
		return platform.KeyBrowserSearch, true
	case 0xAB:
		return platform.KeyBrowserFavorites, true
	case 0xAC:
		return platform.KeyBrowserHome, true
	case 0xAD:
		return platform.KeyAudioVolumeMute, true
	case 0xAE:
		return platform.KeyAudioVolumeDown, true
	case 0xAF:
		return platform.KeyAudioVolumeUp, true
	case 0xB0:
		return platform.KeyMediaTrackNext, true
	case 0xB1:
		return platform.KeyMediaTrackPrevious, true
	case 0xB2:
		return platform.KeyMediaStop, true
	case 0xB3:
		return platform.KeyMediaPlayPause, true
	case 0xB4:
		return platform.KeyLaunchMail, true
	case 0xB5:
		return platform.KeyLaunchMediaPlayer, true
	case 0xB6:
		return platform.KeyLaunchApplication1, true
	case vkLaunchApp2:
		return platform.KeyLaunchApplication2, true
	}
	return "", false
}

// Generic modifier virtual keys and the Windows keys. (The left and right
// ones are above.)
const (
	vkShift   = 0x10
	vkControl = 0x11
	vkMenu    = 0x12
	vkLWin    = 0x5B
	vkRWin    = 0x5C
)
