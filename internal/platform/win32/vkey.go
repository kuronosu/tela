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

// namedVK maps a virtual key to its logical key when the mapping does not
// depend on the physical key or the layout. The empty Key marks a virtual key
// without an entry here. The keys that do depend on them are in namedKey.
var namedVK = [256]platform.Key{
	vkShift:    platform.KeyShift,
	vkLShift:   platform.KeyShift,
	vkRShift:   platform.KeyShift,
	vkControl:  platform.KeyControl,
	vkLControl: platform.KeyControl,
	vkRControl: platform.KeyControl,
	vkMenu:     platform.KeyAlt,
	vkLMenu:    platform.KeyAlt,
	vkRMenu:    platform.KeyAlt,
	vkLWin:     platform.KeyMeta,
	vkRWin:     platform.KeyMeta,

	vkBack:     platform.KeyBackspace,
	vkTab:      platform.KeyTab,
	vkClear:    platform.KeyClear,
	vkOEMClear: platform.KeyClear,
	vkReturn:   platform.KeyEnter,
	vkPause:    platform.KeyPause,
	vkCapital:  platform.KeyCapsLock,

	vkJunja:      platform.KeyJunjaMode,
	vkFinal:      platform.KeyFinalMode,
	vkEscape:     platform.KeyEscape,
	vkConvert:    platform.KeyConvert,
	vkNonConvert: platform.KeyNonConvert,
	vkAccept:     platform.KeyAccept,
	vkModeChange: platform.KeyModeChange,

	vkPrior:    platform.KeyPageUp,
	vkNext:     platform.KeyPageDown,
	vkEnd:      platform.KeyEnd,
	vkHome:     platform.KeyHome,
	vkLeft:     platform.KeyArrowLeft,
	vkUp:       platform.KeyArrowUp,
	vkRight:    platform.KeyArrowRight,
	vkDown:     platform.KeyArrowDown,
	vkSelect:   platform.KeySelect,
	vkPrint:    platform.KeyPrint,
	vkExecute:  platform.KeyExecute,
	vkSnapshot: platform.KeyPrintScreen,
	vkInsert:   platform.KeyInsert,
	vkDelete:   platform.KeyDelete,
	vkHelp:     platform.KeyHelp,
	vkApps:     platform.KeyContextMenu,
	vkSleep:    platform.KeyStandby,
	vkNumLock:  platform.KeyNumLock,
	vkScroll:   platform.KeyScrollLock,

	vkProcessKey: platform.KeyProcess,
	vkAttn:       platform.KeyAttn,
	vkCrSel:      platform.KeyCrSel,
	vkExSel:      platform.KeyExSel,
	vkEraseEOF:   platform.KeyEraseEof,
	vkPlay:       platform.KeyPlay,
	0xF0:         platform.KeyAlphanumeric, // VK_OEM_ATTN: Eisu on Japanese keyboards
	0xF1:         platform.KeyKatakana,     // VK_OEM_FINISH
	0xF2:         platform.KeyHiragana,     // VK_OEM_COPY
	0xF3:         platform.KeyHankaku,      // VK_OEM_AUTO
	0xF4:         platform.KeyZenkaku,      // VK_OEM_ENLW
	0xF5:         platform.KeyRomaji,       // VK_OEM_BACKTAB

	0xA6:         platform.KeyBrowserBack,
	0xA7:         platform.KeyBrowserForward,
	0xA8:         platform.KeyBrowserRefresh,
	0xA9:         platform.KeyBrowserStop,
	0xAA:         platform.KeyBrowserSearch,
	0xAB:         platform.KeyBrowserFavorites,
	0xAC:         platform.KeyBrowserHome,
	0xAD:         platform.KeyAudioVolumeMute,
	0xAE:         platform.KeyAudioVolumeDown,
	0xAF:         platform.KeyAudioVolumeUp,
	0xB0:         platform.KeyMediaTrackNext,
	0xB1:         platform.KeyMediaTrackPrevious,
	0xB2:         platform.KeyMediaStop,
	0xB3:         platform.KeyMediaPlayPause,
	0xB4:         platform.KeyLaunchMail,
	0xB5:         platform.KeyLaunchMediaPlayer,
	0xB6:         platform.KeyLaunchApplication1,
	vkLaunchApp2: platform.KeyLaunchApplication2,
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
	if key, ok := positionKey(vk, code, altGrLayout); ok {
		return key, true
	}
	if vk >= vkF1 && vk <= vkF24 {
		return platform.Key("F" + strconv.Itoa(int(vk-vkF1)+1)), true
	}
	if key, ok := langKey(vk, lang); ok {
		return key, true
	}
	if vk < uint32(len(namedVK)) && namedVK[vk] != "" {
		return namedVK[vk], true
	}
	return "", false
}

// positionKey returns the logical key of the keys identified by position, not
// by virtual key. Windows swaps NumLock and Pause when Ctrl is held
// (Ctrl+NumLock is Pause); the physical key is what the user pressed.
func positionKey(vk uint32, code platform.KeyCode, altGrLayout bool) (platform.Key, bool) {
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
	return "", false
}

// langKey returns the logical key of a virtual key that depends on the
// language of the layout, and false for any other language.
func langKey(vk uint32, lang uint16) (platform.Key, bool) {
	switch {
	case vk == vkKana && lang == langKorean: // VK_HANGUL
		return platform.KeyHangulMode, true
	case vk == vkKana && lang == langJapanese:
		return platform.KeyKanaMode, true
	case vk == vkKanji && lang == langKorean: // VK_HANJA
		return platform.KeyHanjaMode, true
	case vk == vkKanji && lang == langJapanese:
		return platform.KeyKanjiMode, true
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
