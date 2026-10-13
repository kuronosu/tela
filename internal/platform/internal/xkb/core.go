package xkb

import (
	"fmt"
	"slices"
)

// CoreMapping is a keyboard mapping in the form of the X11 core protocol:
// the replies of GetKeyboardMapping and GetModifierMapping.
type CoreMapping struct {
	// MinKeycode is the keycode of the first keysym list (the server's
	// min-keycode, usually 8).
	MinKeycode uint32
	// KeysymsPerKeycode is the length of each keycode's keysym list.
	KeysymsPerKeycode int
	// Keysyms holds the keysym lists of consecutive keycodes from
	// MinKeycode, KeysymsPerKeycode each.
	Keysyms []Keysym
	// Modifiers holds, for each real modifier (Shift, Lock, Control,
	// Mod1 … Mod5), the keycodes bound to it. Zero entries are ignored.
	Modifiers [numReal][]uint32
}

// Keysyms of the modifier keys that give the virtual modifiers of a core
// keymap their real modifiers.
const (
	keysymMetaL        Keysym = 0xffe7
	keysymMetaR        Keysym = 0xffe8
	keysymAltL         Keysym = 0xffe9
	keysymAltR         Keysym = 0xffea
	keysymSuperL       Keysym = 0xffeb
	keysymSuperR       Keysym = 0xffec
	keysymHyperL       Keysym = 0xffed
	keysymLevel3Shift  Keysym = 0xfe03 // ISO_Level3_Shift; Latch and Lock follow
	keysymLevel3Lock   Keysym = 0xfe05
	keysymLevel5Shift  Keysym = 0xfe11 // ISO_Level5_Shift; Latch and Lock follow
	maxCoreGroupLevels        = 8
)

// coreTypes are the key types a core keymap uses: the ones the automatic
// type choice names, as xkeyboard-config defines them. Their virtual
// modifiers get their real modifiers from the modifier mapping.
const coreTypes = `xkb_keymap {
xkb_types "tela-core" {
	virtual_modifiers NumLock,Alt,Meta,Super,Hyper,LevelThree,LevelFive,ModeSwitch;
	type "ONE_LEVEL" { modifiers = none; };
	type "TWO_LEVEL" { modifiers = Shift; map[Shift] = 2; };
	type "ALPHABETIC" { modifiers = Shift+Lock; map[Shift] = 2; map[Lock] = 2; };
	type "KEYPAD" { modifiers = NumLock; map[NumLock] = 2; };
	type "FOUR_LEVEL" {
		modifiers = Shift+LevelThree;
		map[Shift] = 2; map[LevelThree] = 3; map[Shift+LevelThree] = 4;
	};
	type "FOUR_LEVEL_ALPHABETIC" {
		modifiers = Shift+Lock+LevelThree;
		map[Shift] = 2; map[Lock] = 2; map[LevelThree] = 3; map[Shift+LevelThree] = 4;
		map[Lock+LevelThree] = 4; map[Shift+Lock+LevelThree] = 3;
	};
	type "FOUR_LEVEL_SEMIALPHABETIC" {
		modifiers = Shift+Lock+LevelThree;
		map[Shift] = 2; map[Lock] = 2; map[LevelThree] = 3; map[Shift+LevelThree] = 4;
		map[Lock+LevelThree] = 3; preserve[Lock+LevelThree] = Lock;
		map[Shift+Lock+LevelThree] = 4; preserve[Shift+Lock+LevelThree] = Lock;
	};
	type "FOUR_LEVEL_KEYPAD" {
		modifiers = Shift+NumLock+LevelThree;
		map[Shift] = 1; preserve[Shift] = Shift; map[NumLock] = 2; map[LevelThree] = 3;
		map[Shift+NumLock] = 2; preserve[Shift+NumLock] = Shift;
		map[Shift+LevelThree] = 4; map[NumLock+LevelThree] = 3; map[Shift+NumLock+LevelThree] = 4;
	};
	type "EIGHT_LEVEL" {
		modifiers = Shift+LevelThree+LevelFive;
		map[Shift] = 2; map[LevelThree] = 3; map[Shift+LevelThree] = 4;
		map[LevelFive] = 5; map[Shift+LevelFive] = 6;
		map[LevelThree+LevelFive] = 7; map[Shift+LevelThree+LevelFive] = 8;
	};
	type "EIGHT_LEVEL_ALPHABETIC" {
		modifiers = Shift+Lock+LevelThree+LevelFive;
		map[Shift] = 2; map[Lock] = 2; map[LevelThree] = 3; map[Shift+LevelThree] = 4;
		map[Lock+LevelThree] = 4; map[Shift+Lock+LevelThree] = 3;
		map[LevelFive] = 5; map[Shift+LevelFive] = 6; map[Lock+LevelFive] = 6; map[Shift+Lock+LevelFive] = 5;
		map[LevelThree+LevelFive] = 7; map[Shift+LevelThree+LevelFive] = 8;
		map[Lock+LevelThree+LevelFive] = 8; map[Shift+Lock+LevelThree+LevelFive] = 7;
	};
	type "EIGHT_LEVEL_SEMIALPHABETIC" {
		modifiers = Shift+Lock+LevelThree+LevelFive;
		map[Shift] = 2; map[Lock] = 2; map[LevelThree] = 3; map[Shift+LevelThree] = 4;
		map[Lock+LevelThree] = 3; preserve[Lock+LevelThree] = Lock;
		map[Shift+Lock+LevelThree] = 4; preserve[Shift+Lock+LevelThree] = Lock;
		map[LevelFive] = 5; map[Shift+LevelFive] = 6; map[Lock+LevelFive] = 6; map[Shift+Lock+LevelFive] = 5;
		map[LevelThree+LevelFive] = 7; map[Shift+LevelThree+LevelFive] = 8;
		map[Lock+LevelThree+LevelFive] = 7; preserve[Lock+LevelThree+LevelFive] = Lock;
		map[Shift+Lock+LevelThree+LevelFive] = 8; preserve[Shift+Lock+LevelThree+LevelFive] = Lock;
	};
};
};`

// NewCoreKeymap builds a keymap from an X11 core keyboard mapping. An X11
// client that does not read the server's XKB keymap (the X11 backend reads
// only the core mapping) resolves keys with it.
//
// The keysym list of a keycode is read in the order an XKB server writes
// it (XKB protocol, section 12.4, as xserver's XkbGetCoreMap does): group
// 1 levels 1-2, group 2 levels 1-2, then the further levels of group 1,
// then those of group 2. In a mapping where every key's group 2 repeats
// group 1 (one layout: the "ABAB…" replication of one-group keys), each
// key has one group and the repeated tail of its extra levels is
// dropped. With two layouts the list does not say where group 1's extra
// levels end, nor whether "AB AB" + extra levels is one group or two
// groups sharing levels 1-2. The rules: an even repetition of the extra
// levels is one group; otherwise there are two groups, an even number of
// four or more extra levels is split in halves, and fewer go to the
// layout whose keys show a level-three or level-five shift (AltGr) if
// only one does, else to both. So the layout with AltGr levels always
// types them, and a layout without them may type the other's with AltGr
// where XKB would ignore AltGr. Groups 3 and 4 are not recovered (a core
// list cannot tell them apart); the effective group wraps into the key's
// groups. (Reading the XKB map itself, GetMap, would remove these
// ambiguities.)
//
// Each group gets the automatic key type for its keysyms (ONE_LEVEL,
// TWO_LEVEL, ALPHABETIC, KEYPAD, the FOUR_LEVEL and EIGHT_LEVEL types,
// as xkeyboard-config defines them), and a one-level group whose keysym
// has a case pair becomes ALPHABETIC (the XKB rule for core mappings, so
// "keycode 38 = a" types "A" with Shift). The virtual modifiers the types
// use get their real modifiers from the modifier mapping, like XKB's
// interpretations do: LevelThree from the keys with ISO_Level3_Shift,
// Latch or Lock, LevelFive from ISO_Level5_*, NumLock from Num_Lock, Alt
// from Alt_L/Alt_R, Super from Super_L/Super_R, and ModeSwitch (see
// [Keymap.ModMask]) from Mode_switch. Lookups therefore never treat AltGr
// as Alt (State.Modifiers). Modifier keys (Keysym.IsModifier) do not
// repeat; every other key does.
func NewCoreKeymap(m CoreMapping) (*Keymap, error) {
	per := m.KeysymsPerKeycode
	if per <= 0 || len(m.Keysyms)%per != 0 {
		return nil, fmt.Errorf("xkb: core mapping with %d keysyms per keycode and %d keysyms", per, len(m.Keysyms))
	}
	n := len(m.Keysyms) / per
	if n == 0 {
		return nil, fmt.Errorf("xkb: core mapping without keycodes")
	}
	if uint64(m.MinKeycode)+uint64(n)-1 > maxKeycode {
		return nil, fmt.Errorf("xkb: core mapping keycodes %d-%d out of range", m.MinKeycode, uint64(m.MinKeycode)+uint64(n)-1)
	}

	// First pass: trim the lists, and find the groups whose layout has a
	// level-three or level-five shift key, from the keys whose two groups
	// differ (a key that repeats group 1 says nothing about group 2).
	lists := make([][]Keysym, n)
	var levelShift [2]bool
	multi := false // some key has two different groups
	for i := range n {
		syms := m.Keysyms[i*per : (i+1)*per]
		for len(syms) > 0 && syms[len(syms)-1] == NoSymbol {
			syms = syms[:len(syms)-1]
		}
		lists[i] = syms
		if len(syms) <= 2 || repeatsGroup1(syms) {
			continue
		}
		multi = true
		for g := range 2 {
			for _, ks := range syms[2*g : min(2*g+2, len(syms))] {
				if ks >= keysymLevel3Shift && ks <= keysymLevel3Lock || ks >= keysymLevel5Shift && ks <= keysymISOLevel5L {
					levelShift[g] = true
				}
			}
		}
	}

	c := &compiler{
		km:           &Keymap{modNames: append([]string(nil), realModNames[:]...)},
		keycodes:     map[string]uint32{},
		aliases:      map[string]string{},
		vmodExplicit: map[int]uint32{},
		typeByName:   map[string]*keyType{},
		defaults:     interpDefaults{vmod: -1},
	}
	secs, err := parseKeymap([]byte(coreTypes))
	if err != nil {
		return nil, err
	}
	for _, s := range secs {
		if err := c.section(s); err != nil {
			return nil, err
		}
	}
	km := c.km
	km.minKey = m.MinKeycode
	km.keys = make([]key, n)

	// Second pass: the groups and levels of every key.
	for i := range n {
		groups := coreGroups(lists[i], multi, levelShift)
		k := &km.keys[i]
		for _, g := range groups {
			for len(g) > 0 && g[len(g)-1] == NoSymbol {
				g = g[:len(g)-1]
			}
			k.groups = append(k.groups, c.coreGroup(g))
		}
		// A trailing group without keysyms is no group (a key of the
		// first layout only).
		for len(k.groups) > 1 && emptyGroup(k.groups[len(k.groups)-1]) {
			k.groups = k.groups[:len(k.groups)-1]
		}
		if len(k.groups) == 1 && emptyGroup(k.groups[0]) {
			k.groups = nil
		}
		k.exists = len(k.groups) > 0
		k.explicitRepeat, k.explicitVMods = true, true
		if k.exists {
			first := k.groups[0].levels[0].syms
			k.repeats = len(first) == 0 || !first[0].IsModifier()
		}
	}

	// Modifier map, and the real modifiers of the virtual ones.
	vmods := map[string]uint32{}
	for mod, kcs := range m.Modifiers {
		for _, kc := range kcs {
			k := km.keyAt(kc)
			if kc == 0 || k == nil {
				continue
			}
			k.modmap |= 1 << mod
			if mod < 3 {
				continue // Shift, Lock and Control have names of their own
			}
			// Like xkeyboard-config's interpretations: the first level
			// of group 1 binds every virtual modifier, the second one
			// only Alt, Meta, Super and Hyper (the level-three and
			// level-five shifts are level-one only).
			g := k.groups[0]
			for l := range min(2, len(g.levels)) {
				for _, ks := range g.levels[l].syms {
					name := coreVirtualModifier(ks)
					if name != "" && (l == 0 || name == "Alt" || name == "Meta" || name == "Super" || name == "Hyper") {
						vmods[name] |= 1 << mod
					}
				}
			}
		}
	}
	for name, mask := range vmods {
		c.vmodExplicit[km.modIndex(name)] = mask
	}
	if err := c.finish(); err != nil {
		return nil, err
	}
	return km, nil
}

// repeatsGroup1 reports whether a trimmed core list of three or more
// keysyms repeats group 1's first two levels as group 2's.
func repeatsGroup1(syms []Keysym) bool {
	return syms[2] == syms[0] && (len(syms) == 3 || syms[3] == syms[1])
}

// coreGroups splits the trimmed core keysym list of one keycode into
// groups of levels; see NewCoreKeymap. multi tells whether the mapping has
// two layouts (some key has two different groups), levelShift which of
// them has an AltGr key.
func coreGroups(syms []Keysym, multi bool, levelShift [2]bool) [][]Keysym {
	n := len(syms)
	switch {
	case n == 0:
		return nil
	case n <= 2:
		return [][]Keysym{slices.Clone(syms)}
	case n <= 4 && repeatsGroup1(syms):
		return [][]Keysym{slices.Clone(syms[:2])}
	case n <= 4:
		return [][]Keysym{slices.Clone(syms[:2]), slices.Clone(syms[2:])}
	}
	extra := syms[4:]
	if repeatsGroup1(syms) {
		// A one-group key: "ABAB", its levels 3 and up, then as much of
		// them again as the list width allows ("CDE" + "CD").
		p, ok := repeatPeriod(extra)
		switch {
		case !multi && ok:
			return [][]Keysym{append(slices.Clone(syms[:2]), extra[:p]...)}
		case !multi:
			return [][]Keysym{append(slices.Clone(syms[:2]), extra...)}
		case ok && p%2 == 0:
			// With two layouts the same list can also be two groups
			// sharing levels 1-2: "AB" "AB" + "€" + "€" is a one-group
			// key with three levels or a four-level group next to a
			// two-level one. Levels come in pairs in xkeyboard-config
			// (four and eight-level types), so only an even repetition
			// is taken for one group.
			return [][]Keysym{append(slices.Clone(syms[:2]), extra[:p]...)}
		}
	}
	g1 := slices.Clone(syms[:2])
	g2 := slices.Clone(syms[2:4])
	switch {
	case len(extra) >= 4 && len(extra)%2 == 0:
		// Both layouts have the same number of extra levels (the common
		// case: two four-level groups).
		g1 = append(g1, extra[:len(extra)/2]...)
		g2 = append(g2, extra[len(extra)/2:]...)
	case levelShift[0] && !levelShift[1]:
		g1 = append(g1, extra...)
	case levelShift[1] && !levelShift[0]:
		g2 = append(g2, extra...)
	default:
		// Unknown owner: both groups get them. A layout without AltGr
		// then types the other layout's level 3 with AltGr, which is
		// better than losing it in the layout that has it.
		g1 = append(g1, extra...)
		g2 = append(g2, extra...)
	}
	return [][]Keysym{g1, g2}
}

// repeatPeriod finds the repetition XkbGetCoreMap writes after the extra
// levels of a one-group key ("CDE" + "CDE", possibly cut short by the list
// width): the shortest prefix of at least half the list that the rest
// repeats.
func repeatPeriod(extra []Keysym) (int, bool) {
	for p := (len(extra) + 1) / 2; p < len(extra); p++ {
		if slices.Equal(extra[p:], extra[:len(extra)-p]) {
			return p, true
		}
	}
	return 0, false
}

// coreGroup makes a key group of a core keysym list: one keysym per
// level, NoSymbol for an empty level, with the automatic type.
func (c *compiler) coreGroup(syms []Keysym) keyGroup {
	if len(syms) > maxCoreGroupLevels {
		syms = syms[:maxCoreGroupLevels]
	}
	if len(syms) == 1 {
		// XKB's rule for core mappings (XkbKeyTypesForCoreSymbols): a
		// single keysym with a case pair is a lower/upper pair.
		if lo, up := syms[0].ToLower(), syms[0].ToUpper(); lo != up {
			syms = []Keysym{lo, up}
		}
	}
	levels := make([][]Keysym, len(syms))
	for i, ks := range syms {
		if ks != NoSymbol {
			levels[i] = []Keysym{ks}
		}
	}
	t := c.typeByName[coreAutomaticType(levels)]
	g := keyGroup{typ: t, levels: make([]level, t.levels)}
	for l := range min(len(levels), t.levels) {
		g.levels[l].syms = levels[l]
	}
	return g
}

// coreAutomaticType is automaticType extended to the eight-level types
// (levels five to eight come from LevelFive, as in xkeyboard-config).
func coreAutomaticType(levels [][]Keysym) string {
	if len(levels) <= 4 {
		if len(levels) == 0 {
			return "ONE_LEVEL"
		}
		return automaticType(levels)
	}
	switch automaticType(levels[:4]) {
	case "FOUR_LEVEL_ALPHABETIC":
		return "EIGHT_LEVEL_ALPHABETIC"
	case "FOUR_LEVEL_SEMIALPHABETIC":
		return "EIGHT_LEVEL_SEMIALPHABETIC"
	}
	return "EIGHT_LEVEL"
}

func emptyGroup(g keyGroup) bool {
	for _, l := range g.levels {
		if len(l.syms) > 0 {
			return false
		}
	}
	return true
}

// coreVirtualModifier returns the virtual modifier a modifier key binds
// to the real modifier it is mapped to (the effect of xkeyboard-config's
// compat interpretations), or "".
func coreVirtualModifier(ks Keysym) string {
	switch {
	case ks == keysymAltL || ks == keysymAltR:
		return "Alt"
	case ks == keysymMetaL || ks == keysymMetaR:
		return "Meta"
	case ks == keysymSuperL || ks == keysymSuperR:
		return "Super"
	case ks == keysymHyperL || ks == keysymHyperR:
		return "Hyper"
	case ks >= keysymLevel3Shift && ks <= keysymLevel3Lock:
		return "LevelThree"
	case ks >= keysymLevel5Shift && ks <= keysymISOLevel5L:
		return "LevelFive"
	case ks == keysymNumLock:
		return "NumLock"
	case ks == keysymModeSwitch:
		return "ModeSwitch"
	}
	return ""
}
