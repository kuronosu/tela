package xkb

import (
	"sync"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/platform"
)

// namedKeysyms maps keysyms to the W3C named key values (platform.Key*).
// The choices follow the W3C UI Events key specification and what
// browsers report on Linux, with winit's keysym_to_key as a cross-check;
// where they differ the W3C/browser value wins (Super is "Meta", like the
// DOM; the play/pause media key is "MediaPlayPause", like Chromium).
// Keysyms that are characters (letters, digits, keypad digits and
// operators, the space bar) are not listed: their Key is the character.
var namedKeysyms = []struct {
	name string
	key  platform.Key
}{
	// TTY function keys.
	{"BackSpace", platform.KeyBackspace},
	{"Tab", platform.KeyTab},
	{"ISO_Left_Tab", platform.KeyTab}, // Shift+Tab
	{"Linefeed", platform.KeyEnter},
	{"Clear", platform.KeyClear},
	{"Return", platform.KeyEnter},
	{"Pause", platform.KeyPause},
	{"Scroll_Lock", platform.KeyScrollLock},
	{"Sys_Req", platform.KeyPrintScreen}, // Alt+PrintScreen
	{"Escape", platform.KeyEscape},
	{"Delete", platform.KeyDelete},

	// International and IME keys.
	{"Multi_key", platform.KeyCompose},
	{"Codeinput", platform.KeyCodeInput},
	{"SingleCandidate", platform.KeySingleCandidate},
	{"MultipleCandidate", platform.KeyAllCandidates},
	{"PreviousCandidate", platform.KeyPreviousCandidate},
	{"Kanji", platform.KeyKanjiMode},
	{"Muhenkan", platform.KeyNonConvert},
	{"Henkan_Mode", platform.KeyConvert},
	{"Romaji", platform.KeyRomaji},
	{"Hiragana", platform.KeyHiragana},
	{"Katakana", platform.KeyKatakana},
	{"Hiragana_Katakana", platform.KeyHiraganaKatakana},
	{"Zenkaku", platform.KeyZenkaku},
	{"Hankaku", platform.KeyHankaku},
	{"Zenkaku_Hankaku", platform.KeyZenkakuHankaku},
	{"Kana_Lock", platform.KeyKanaMode},
	{"Kana_Shift", platform.KeyKanaMode},
	{"Eisu_Shift", platform.KeyAlphanumeric},
	{"Eisu_toggle", platform.KeyAlphanumeric},
	{"Hangul", platform.KeyHangulMode},
	{"Hangul_Hanja", platform.KeyHanjaMode},
	{"Hangul_Jeonja", platform.KeyJunjaMode},
	{"Mode_switch", platform.KeyModeChange},

	// Cursor control.
	{"Home", platform.KeyHome},
	{"Left", platform.KeyArrowLeft},
	{"Up", platform.KeyArrowUp},
	{"Right", platform.KeyArrowRight},
	{"Down", platform.KeyArrowDown},
	{"Prior", platform.KeyPageUp},
	{"Next", platform.KeyPageDown},
	{"End", platform.KeyEnd},
	{"Begin", platform.KeyClear},

	// Misc functions.
	{"Select", platform.KeySelect},
	{"Print", platform.KeyPrintScreen},
	{"Execute", platform.KeyExecute},
	{"Insert", platform.KeyInsert},
	{"Undo", platform.KeyUndo},
	{"Redo", platform.KeyRedo},
	{"Menu", platform.KeyContextMenu},
	{"Find", platform.KeyFind},
	{"Cancel", platform.KeyCancel},
	{"Help", platform.KeyHelp},
	{"Break", platform.KeyPause},
	{"Num_Lock", platform.KeyNumLock},

	// Keypad keys without a character (Num Lock off). The digits and
	// operators are characters ("1", "+"), as browsers report them.
	{"KP_Tab", platform.KeyTab},
	{"KP_Enter", platform.KeyEnter},
	{"KP_F1", platform.KeyF1},
	{"KP_F2", platform.KeyF2},
	{"KP_F3", platform.KeyF3},
	{"KP_F4", platform.KeyF4},
	{"KP_Home", platform.KeyHome},
	{"KP_Left", platform.KeyArrowLeft},
	{"KP_Up", platform.KeyArrowUp},
	{"KP_Right", platform.KeyArrowRight},
	{"KP_Down", platform.KeyArrowDown},
	{"KP_Prior", platform.KeyPageUp},
	{"KP_Next", platform.KeyPageDown},
	{"KP_End", platform.KeyEnd},
	{"KP_Begin", platform.KeyClear},
	{"KP_Insert", platform.KeyInsert},
	{"KP_Delete", platform.KeyDelete},

	// Function keys.
	{"F1", platform.KeyF1}, {"F2", platform.KeyF2}, {"F3", platform.KeyF3}, {"F4", platform.KeyF4},
	{"F5", platform.KeyF5}, {"F6", platform.KeyF6}, {"F7", platform.KeyF7}, {"F8", platform.KeyF8},
	{"F9", platform.KeyF9}, {"F10", platform.KeyF10}, {"F11", platform.KeyF11}, {"F12", platform.KeyF12},
	{"F13", platform.KeyF13}, {"F14", platform.KeyF14}, {"F15", platform.KeyF15}, {"F16", platform.KeyF16},
	{"F17", platform.KeyF17}, {"F18", platform.KeyF18}, {"F19", platform.KeyF19}, {"F20", platform.KeyF20},
	{"F21", platform.KeyF21}, {"F22", platform.KeyF22}, {"F23", platform.KeyF23}, {"F24", platform.KeyF24},

	// Modifiers. Super is the DOM's "Meta" (the Windows/Command key).
	{"Shift_L", platform.KeyShift},
	{"Shift_R", platform.KeyShift},
	{"Control_L", platform.KeyControl},
	{"Control_R", platform.KeyControl},
	{"Caps_Lock", platform.KeyCapsLock},
	{"Shift_Lock", platform.KeyCapsLock},
	{"Meta_L", platform.KeyMeta},
	{"Meta_R", platform.KeyMeta},
	{"Alt_L", platform.KeyAlt},
	{"Alt_R", platform.KeyAlt},
	{"Super_L", platform.KeyMeta},
	{"Super_R", platform.KeyMeta},
	{"Hyper_L", platform.KeyHyper},
	{"Hyper_R", platform.KeyHyper},
	{"ISO_Level3_Shift", platform.KeyAltGraph},
	{"ISO_Level3_Latch", platform.KeyAltGraph},
	{"ISO_Level3_Lock", platform.KeyAltGraph},
	{"ISO_Next_Group", platform.KeyGroupNext},
	{"ISO_Prev_Group", platform.KeyGroupPrevious},
	{"ISO_First_Group", platform.KeyGroupFirst},
	{"ISO_Last_Group", platform.KeyGroupLast},

	// 3270 terminal keys.
	{"3270_Attn", platform.KeyAttn},
	{"3270_CursorSelect", platform.KeyCrSel},
	{"3270_ExSelect", platform.KeyExSel},
	{"3270_EraseEOF", platform.KeyEraseEof},
	{"3270_Play", platform.KeyPlay},
	{"3270_PrintScreen", platform.KeyPrintScreen},

	// XF86 media, browser, launcher and power keys.
	{"XF86MonBrightnessUp", platform.KeyBrightnessUp},
	{"XF86MonBrightnessDown", platform.KeyBrightnessDown},
	{"XF86Standby", platform.KeyStandby},
	{"XF86Sleep", platform.KeyStandby},
	{"XF86Suspend", platform.KeyStandby},
	{"XF86Hibernate", platform.KeyHibernate},
	{"XF86WakeUp", platform.KeyWakeUp},
	{"XF86PowerOff", platform.KeyPowerOff},
	{"XF86PowerDown", platform.KeyPower},
	{"XF86LogOff", platform.KeyLogOff},
	{"XF86Eject", platform.KeyEject},
	{"XF86AudioLowerVolume", platform.KeyAudioVolumeDown},
	{"XF86AudioRaiseVolume", platform.KeyAudioVolumeUp},
	{"XF86AudioMute", platform.KeyAudioVolumeMute},
	{"XF86AudioMicMute", platform.KeyMicrophoneVolumeMute},
	{"XF86AudioPlay", platform.KeyMediaPlayPause},
	{"XF86AudioPause", platform.KeyMediaPause},
	{"XF86AudioStop", platform.KeyMediaStop},
	{"XF86AudioPrev", platform.KeyMediaTrackPrevious},
	{"XF86AudioNext", platform.KeyMediaTrackNext},
	{"XF86AudioRecord", platform.KeyMediaRecord},
	{"XF86AudioRewind", platform.KeyMediaRewind},
	{"XF86AudioForward", platform.KeyMediaFastForward},
	{"XF86AudioMedia", platform.KeyLaunchMediaPlayer},
	{"XF86Music", platform.KeyLaunchMusicPlayer},
	{"XF86Close", platform.KeyClose},
	{"XF86Copy", platform.KeyCopy},
	{"XF86Cut", platform.KeyCut},
	{"XF86Paste", platform.KeyPaste},
	{"XF86New", platform.KeyNew},
	{"XF86Open", platform.KeyOpen},
	{"XF86Save", platform.KeySave},
	{"XF86Send", platform.KeyMailSend},
	{"XF86Reply", platform.KeyMailReply},
	{"XF86MailForward", platform.KeyMailForward},
	{"XF86Spell", platform.KeySpellCheck},
	{"XF86ZoomIn", platform.KeyZoomIn},
	{"XF86ZoomOut", platform.KeyZoomOut},
	{"XF86Back", platform.KeyBrowserBack},
	{"XF86Forward", platform.KeyBrowserForward},
	{"XF86Stop", platform.KeyBrowserStop},
	{"XF86Refresh", platform.KeyBrowserRefresh},
	{"XF86Reload", platform.KeyBrowserRefresh},
	{"XF86Favorites", platform.KeyBrowserFavorites},
	{"XF86HomePage", platform.KeyBrowserHome},
	{"XF86Search", platform.KeyBrowserSearch},
	{"XF86Mail", platform.KeyLaunchMail},
	{"XF86Calendar", platform.KeyLaunchCalendar},
	{"XF86Calculator", platform.KeyLaunchApplication2},
	{"XF86MyComputer", platform.KeyLaunchApplication1},
	{"XF86WWW", platform.KeyLaunchWebBrowser},
	{"XF86ScreenSaver", platform.KeyLaunchScreenSaver},
	{"XF86Phone", platform.KeyLaunchPhone},
	{"XF86WebCam", platform.KeyLaunchWebCam},
	{"XF86Word", platform.KeyLaunchWordProcessor},
	{"XF86Excel", platform.KeyLaunchSpreadsheet},
}

var namedKeyMap = sync.OnceValue(func() map[Keysym]platform.Key {
	m := make(map[Keysym]platform.Key, len(namedKeysyms))
	for _, n := range namedKeysyms {
		ks, ok := KeysymFromName(n.name)
		if !ok {
			panic("xkb: unknown keysym name in namedKeysyms: " + n.name)
		}
		m[ks] = n.key
	}
	return m
})

// deadSpacing gives the spacing character of each dead key: what a dead
// key types when no Compose table is available, and the text of the
// usual "dead key, space" sequence.
var deadSpacing = map[string]rune{
	"dead_grave":              '`',
	"dead_acute":              '´',
	"dead_circumflex":         '^',
	"dead_tilde":              '~',
	"dead_macron":             '¯',
	"dead_breve":              '˘',
	"dead_abovedot":           '˙',
	"dead_diaeresis":          '¨',
	"dead_abovering":          '˚',
	"dead_doubleacute":        '˝',
	"dead_caron":              'ˇ',
	"dead_cedilla":            '¸',
	"dead_ogonek":             '˛',
	"dead_iota":               'ͺ',
	"dead_voiced_sound":       '゛',
	"dead_semivoiced_sound":   '゜',
	"dead_belowdot":           '.',
	"dead_hook":               '̉',
	"dead_horn":               '̛',
	"dead_stroke":             '/',
	"dead_abovecomma":         '̓',
	"dead_abovereversedcomma": '̔',
	"dead_doublegrave":        '̏',
	"dead_belowring":          '˳',
	"dead_belowmacron":        'ˍ',
	"dead_belowcircumflex":    'ꞈ',
	"dead_belowtilde":         '˷',
	"dead_belowbreve":         '̮',
	"dead_belowdiaeresis":     '̤',
	"dead_invertedbreve":      '̑',
	"dead_belowcomma":         ',',
	"dead_currency":           '¤',
	"dead_lowline":            '_',
	"dead_aboveverticalline":  'ˈ',
	"dead_belowverticalline":  'ˌ',
	"dead_longsolidusoverlay": '/',
	"dead_greek":              'µ',
}

var deadSpacingMap = sync.OnceValue(func() map[Keysym]string {
	m := make(map[Keysym]string, len(deadSpacing))
	for name, r := range deadSpacing {
		ks, ok := KeysymFromName(name)
		if !ok {
			panic("xkb: unknown keysym name in deadSpacing: " + name)
		}
		m[ks] = string(r)
	}
	return m
})

// keysymText returns the text a keysym types: its character, or "" for
// keysyms without one and for control characters (Return, Tab,
// BackSpace, Escape, Delete and the C0/C1 ranges), which are handled
// through the logical key instead (ADR-050).
func keysymText(ks Keysym) string {
	r := ks.ToUTF32()
	if r == 0 || r < 0x20 || (r >= 0x7f && r < 0xa0) || !utf8.ValidRune(r) {
		return ""
	}
	return string(r)
}

// keysymKey returns the logical key of a keysym: a named key, KeyDead for
// dead keys, the keysym's character, or KeyUnidentified.
func keysymKey(ks Keysym) platform.Key {
	if k, ok := namedKeyMap()[ks]; ok {
		return k
	}
	if ks.IsDead() {
		return platform.KeyDead
	}
	if t := keysymText(ks); t != "" {
		return platform.Key(t)
	}
	return platform.KeyUnidentified
}
