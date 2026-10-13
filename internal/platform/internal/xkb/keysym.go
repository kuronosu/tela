package xkb

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//go:generate go run gen_keysyms.go

// Keysym is an X11 keysym: the symbol a key produces at one level (a
// character such as 0x61 "a", or a function such as 0xff0d Return).
type Keysym uint32

// Keysyms with a meaning in this package.
const (
	NoSymbol   Keysym = 0
	VoidSymbol Keysym = 0xffffff

	keysymBackSpace  Keysym = 0xff08
	keysymClear      Keysym = 0xff0b
	keysymReturn     Keysym = 0xff0d
	keysymEscape     Keysym = 0xff1b
	keysymDelete     Keysym = 0xffff
	keysymKPSpace    Keysym = 0xff80
	keysymKPTab      Keysym = 0xff89
	keysymKPEnter    Keysym = 0xff8d
	keysymKPMultiply Keysym = 0xffaa
	keysymKP9        Keysym = 0xffb9
	keysymKPEqual    Keysym = 0xffbd
	keysymShiftL     Keysym = 0xffe1
	keysymHyperR     Keysym = 0xffee
	keysymISOLock    Keysym = 0xfe01
	keysymISOLevel5L Keysym = 0xfe13 // ISO_Level5_Lock
	keysymModeSwitch Keysym = 0xff7e
	keysymNumLock    Keysym = 0xff7f
	keysymMultiKey   Keysym = 0xff20
	keysymDeadFirst  Keysym = 0xfe50 // dead_grave
	keysymDeadLast   Keysym = 0xfe93 // dead_lowline … dead_longsolidusoverlay

	unicodeOffset Keysym = 0x01000000
	unicodeMin    Keysym = 0x01000100
	unicodeMax    Keysym = 0x0110ffff
	keysymMax     Keysym = 0x1fffffff
)

// keysymName is an entry of the generated name tables.
type keysymName struct {
	name string
	sym  Keysym
}

var nameMap = sync.OnceValue(func() map[string]Keysym {
	m := make(map[string]Keysym, len(keysymNames))
	for _, n := range keysymNames {
		if _, ok := m[n.name]; !ok {
			m[n.name] = n.sym
		}
	}
	return m
})

// KeysymFromName returns the keysym named name, like libxkbcommon's
// xkb_keysym_from_name without flags: a name of the keysym table
// ("a", "Return", "dead_acute", "XF86AudioMute"), a Unicode form "U20AC",
// or a hexadecimal value "0x1000e9". Names are case-sensitive.
func KeysymFromName(name string) (Keysym, bool) {
	if ks, ok := nameMap()[name]; ok {
		return ks, true
	}
	if len(name) >= 2 && name[0] == 'U' {
		hex := name[1:]
		if len(hex) > 8 || !isHex(hex) {
			return NoSymbol, false
		}
		v, _ := strconv.ParseUint(hex, 16, 32)
		if v > 0xff && v <= 0x10ffff {
			return unicodeOffset + Keysym(v), true
		}
		ks := UTF32ToKeysym(rune(v))
		return ks, ks != NoSymbol
	}
	if len(name) > 2 && name[0] == '0' && name[1] == 'x' {
		if !isHex(name[2:]) || len(name) > 10 {
			return NoSymbol, false
		}
		v, _ := strconv.ParseUint(name[2:], 16, 32)
		if Keysym(v) > keysymMax {
			return NoSymbol, false
		}
		return Keysym(v), true
	}
	if strings.HasPrefix(name, "XF86_") {
		return KeysymFromName("XF86" + name[5:])
	}
	return NoSymbol, false
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F') {
			return false
		}
	}
	return true
}

// String returns the keysym's canonical name, its Unicode form ("U20AC")
// or its hexadecimal value, like xkb_keysym_get_name.
func (ks Keysym) String() string {
	i := sort.Search(len(keysymCanonical), func(i int) bool { return keysymCanonical[i].sym >= ks })
	if i < len(keysymCanonical) && keysymCanonical[i].sym == ks {
		return keysymCanonical[i].name
	}
	if ks >= unicodeMin && ks <= unicodeMax {
		return "U" + strings.ToUpper(strconv.FormatUint(uint64(ks-unicodeOffset), 16))
	}
	return "0x" + strconv.FormatUint(uint64(ks)|1<<32, 16)[1:]
}

// ToUTF32 returns the character a keysym stands for, or 0, with the rules
// of libxkbcommon's xkb_keysym_to_utf32 (and the public-domain keysym
// table it is derived from): Latin-1 keysyms are their own code point,
// Unicode keysyms carry it, the legacy keysyms use the table of
// keysymdef.h, and the keypad and control keys give their ASCII value
// (Return is "\r", KP_1 is "1"). Dead keys give 0.
func (ks Keysym) ToUTF32() rune {
	switch {
	case ks >= 0x20 && ks <= 0x7e, ks >= 0xa0 && ks <= 0xff:
		return rune(ks)
	case ks == keysymKPSpace:
		return ' '
	case ks >= keysymBackSpace && ks <= keysymClear,
		ks >= keysymKPMultiply && ks <= keysymKP9,
		ks == keysymReturn, ks == keysymEscape, ks == keysymDelete,
		ks == keysymKPTab, ks == keysymKPEnter, ks == keysymKPEqual:
		return rune(ks & 0x7f)
	case ks >= unicodeOffset && ks <= unicodeMax:
		return rune(ks - unicodeOffset)
	case ks < 0x100:
		return 0
	}
	i := sort.Search(len(legacyUCS), func(i int) bool { return Keysym(legacyUCS[i][0]) >= ks })
	if i < len(legacyUCS) && Keysym(legacyUCS[i][0]) == ks {
		return rune(legacyUCS[i][1])
	}
	return 0
}

var ucsToLegacy = sync.OnceValue(func() map[rune]Keysym {
	m := make(map[rune]Keysym, len(legacyUCS))
	for _, e := range legacyUCS {
		if _, ok := m[rune(e[1])]; !ok { // the lowest keysym wins, like the table search
			m[rune(e[1])] = Keysym(e[0])
		}
	}
	return m
})

// UTF32ToKeysym returns the keysym of a character, preferring a legacy
// keysym when one exists (Cyrillic, Greek, …), like xkb_utf32_to_keysym.
// It returns NoSymbol for non-characters and surrogates.
func UTF32ToKeysym(r rune) Keysym {
	switch {
	case r >= 0x20 && r <= 0x7e, r >= 0xa0 && r <= 0xff:
		return Keysym(r)
	case r >= rune(keysymBackSpace&0x7f) && r <= rune(keysymClear&0x7f), r == rune(keysymReturn&0x7f), r == rune(keysymEscape&0x7f):
		return Keysym(r) | 0xff00
	case r == 0x7f:
		return keysymDelete
	case r < 0x20, r >= 0x80 && r < 0xa0:
		return NoSymbol
	case r >= 0xd800 && r <= 0xdfff, r >= 0xfdd0 && r <= 0xfdef, r > 0x10ffff, r&0xfffe == 0xfffe:
		return NoSymbol
	}
	if ks, ok := ucsToLegacy()[r]; ok {
		return ks
	}
	return unicodeOffset + Keysym(r)
}

// ToUpper returns the uppercase keysym of ks, or ks itself, like
// xkb_keysym_to_upper: legacy keysyms map to legacy keysyms where one
// exists, Unicode keysyms stay Unicode keysyms.
func (ks Keysym) ToUpper() Keysym { return ks.convertCase(true) }

// ToLower is the lowercase counterpart of ToUpper.
func (ks Keysym) ToLower() Keysym { return ks.convertCase(false) }

func (ks Keysym) convertCase(upper bool) Keysym {
	r := ks.ToUTF32()
	if r == 0 || (ks >= 0xff00 && ks <= 0xffff) || (ks >= unicodeOffset && ks < unicodeMin) {
		// No character, a keypad/control keysym, or a non-normalized
		// Unicode keysym below U+0100, which libxkbcommon leaves alone.
		return ks
	}
	var c rune
	switch {
	case upper && r == 'ß':
		c = 'ẞ' // Unicode's simple mapping has none; xkbcommon uses U+1E9E
	case upper:
		c = unicode.ToUpper(r)
	default:
		c = unicode.ToLower(r)
	}
	if c == r {
		return ks
	}
	if ks >= unicodeMin && ks <= unicodeMax && c > 0xff {
		return unicodeOffset + Keysym(c)
	}
	return UTF32ToKeysym(c)
}

// isLower and isUpperOrTitle are the predicates of the automatic key type
// choice (xkb_keysym_is_lower, xkb_keysym_is_upper_or_title).
func (ks Keysym) isLower() bool {
	r := ks.ToUTF32()
	return r != 0 && ks.ToUpper() != ks && (unicode.IsLower(r) || r == 'ß')
}

func (ks Keysym) isUpperOrTitle() bool {
	r := ks.ToUTF32()
	return r != 0 && ks.ToLower() != ks && (unicode.IsUpper(r) || unicode.IsTitle(r))
}

// IsKeypad reports whether ks is one of the keypad keysyms (KP_Space …
// KP_Equal).
func (ks Keysym) IsKeypad() bool { return ks >= keysymKPSpace && ks <= keysymKPEqual }

// IsModifier reports whether ks is a modifier keysym (Shift, Control,
// Alt, Super, the ISO level and group keys, Mode_switch, Num_Lock), like
// xkb_keysym_is_modifier. Compose ignores them.
func (ks Keysym) IsModifier() bool {
	return ks >= keysymShiftL && ks <= keysymHyperR ||
		ks >= keysymISOLock && ks <= keysymISOLevel5L ||
		ks == keysymModeSwitch || ks == keysymNumLock
}

// IsDead reports whether ks is a dead key (dead_grave … ).
func (ks Keysym) IsDead() bool { return ks >= keysymDeadFirst && ks <= keysymDeadLast }
