package xkb

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// exportCore writes km as an XKB X server presents it to core clients
// (xserver's XkbGetCoreMap in xkb/xkbUtils.c, and the real modifier map
// for GetModifierMapping). It is the input NewCoreKeymap gets from a
// real server.
func exportCore(km *Keymap) CoreMapping {
	width := func(k *key, g int) int { return len(k.groups[g].levels) }
	sym := func(k *key, g, l int) Keysym {
		if s := k.groups[g].levels[l].syms; len(s) > 0 {
			return s[0]
		}
		return NoSymbol
	}
	maxSyms := 0
	for i := range km.keys {
		k := &km.keys[i]
		nG := len(k.groups)
		if !k.exists || nG == 0 {
			continue
		}
		tmp := 0
		if w := width(k, 0); w <= 2 {
			tmp += 2
		} else {
			tmp += w + 2
		}
		if nG > 1 {
			w := width(k, 1)
			switch {
			case tmp <= 2 && w < 2:
				tmp += 2
			case tmp <= 2:
				tmp += w
			case w > 2:
				tmp += w - 2
			}
		}
		for g := 2; g < nG; g++ {
			tmp += width(k, g)
		}
		maxSyms = max(maxSyms, tmp)
	}
	mapWidth := maxSyms
	m := CoreMapping{MinKeycode: km.minKey, KeysymsPerKeycode: mapWidth}
	for i := range km.keys {
		k := &km.keys[i]
		core := make([]Keysym, mapWidth)
		nG := len(k.groups)
		if !k.exists {
			nG = 0
		}
		nOut := 2
		if nG > 0 {
			gw := width(k, 0)
			if gw > 0 {
				core[0] = sym(k, 0, 0)
			}
			if gw > 1 {
				core[1] = sym(k, 0, 1)
			}
			for n := 2; n < gw; n++ {
				core[2+n] = sym(k, 0, n)
			}
			if gw > 2 {
				nOut = gw
			}
		}
		if nG == 1 {
			gw := width(k, 0)
			if gw > 0 && mapWidth >= 3 {
				core[2] = core[0]
			}
			if gw > 1 && mapWidth >= 4 {
				core[3] = core[1]
			}
			idx := 2 + gw
			for gw > 2 && idx < mapWidth && idx < gw*2 {
				core[idx] = core[idx-gw+2]
				idx++
			}
			idx = max(2*gw, 4)
			for j := 3; j <= km.numGroups; j++ {
				for n := 0; n < gw && idx < maxSyms; n++ {
					core[idx] = sym(k, 0, n)
					idx++
				}
			}
		}
		nOut += 2
		if nG > 1 {
			gw := width(k, 1)
			if gw > 0 {
				core[2] = sym(k, 1, 0)
			}
			if gw > 1 {
				core[3] = sym(k, 1, 1)
			}
			for n := 2; n < gw; n++ {
				core[nOut+n-2] = sym(k, 1, n)
			}
			if gw > 2 {
				nOut += gw - 2
			}
		}
		for g := 2; g < nG; g++ {
			for s := range width(k, g) {
				core[nOut] = sym(k, g, s)
				nOut++
			}
		}
		m.Keysyms = append(m.Keysyms, core...)
		for mod := range numReal {
			if k.exists && k.modmap&(1<<mod) != 0 {
				m.Modifiers[mod] = append(m.Modifiers[mod], km.minKey+uint32(i))
			}
		}
	}
	return m
}

type coreState struct {
	name         string
	mods, locked string
	group        uint32
}

var coreStates = []coreState{
	{"base", "", "", 0},
	{"shift", "Shift", "", 0},
	{"caps", "", "Lock", 0},
	{"shiftcaps", "Shift", "Lock", 0},
	{"l3", "LevelThree", "", 0},
	{"shiftl3", "Shift+LevelThree", "", 0},
	{"capsl3", "LevelThree", "Lock", 0},
	{"num", "", "NumLock", 0},
	{"shiftnum", "Shift", "NumLock", 0},
	{"ctrl", "Control", "", 0},
	{"alt", "Alt", "", 0},
}

// automaticKey reports whether every group of k has the type the
// automatic choice gives its keysyms: only then can a core mapping, which
// has no types, express the key.
func automaticKey(k *key) bool {
	for _, g := range k.groups {
		levels := make([][]Keysym, len(g.levels))
		for l := range g.levels {
			levels[l] = g.levels[l].syms
		}
		if g.typ.name != coreAutomaticType(levels) {
			return false
		}
	}
	return true
}

// coreDiff is a key that resolves differently with the keymap built from
// the core export than with the XKB keymap.
type coreDiff struct {
	key       string
	state     coreState
	got, want Lookup
	// noLevel3 is the XKB result of the same key without LevelThree.
	noLevel3 Lookup
}

func (d coreDiff) String() string {
	return fmt.Sprintf("<%s> %s: %q/%q, want %q/%q", d.key, d.state.name, d.got.Key, d.got.Text, d.want.Key, d.want.Text)
}

// coreDiffs compares the keys of the fixture keymap with the keymap built
// from its core export, in the given states. Keys with explicit types
// other than the automatic one (PC_ALT_LEVEL2 for Print, the
// PC_*_SUPER_LEVEL2 keys, Hebrew letters typed FOUR_LEVEL_SEMIALPHABETIC)
// are skipped: a core mapping has no types. Differences where XKB has no
// keysym (an empty level of a four-level type, which the core list cannot
// tell from a two-level key) are left out too: the core keymap types the
// lower level there.
func coreDiffs(t *testing.T, name string, states []coreState) []coreDiff {
	t.Helper()
	km := loadFixture(t, name)
	ck, err := NewCoreKeymap(exportCore(km))
	if err != nil {
		t.Fatalf("%s: NewCoreKeymap: %v", name, err)
	}
	var skipped []string
	for i := range km.keys {
		if k := &km.keys[i]; k.exists && !automaticKey(k) {
			skipped = append(skipped, k.name)
		}
	}
	t.Logf("%s: %d keys with non-automatic types skipped: %s", name, len(skipped), strings.Join(skipped, " "))
	st, cst, plain := NewState(km), NewState(ck), NewState(km)
	l3 := maskOf(t, km, "LevelThree")
	var diffs []coreDiff
	for _, s := range states {
		mods, locked := maskOf(t, km, s.mods), maskOf(t, km, s.locked)
		st.UpdateMask(mods, 0, locked, s.group)
		cst.UpdateMask(mods, 0, locked, s.group)
		plain.UpdateMask(mods&^l3, 0, locked, s.group)
		if st.Modifiers() != cst.Modifiers() {
			t.Errorf("%s: Modifiers %s: %v, want %v", name, s.name, cst.Modifiers(), st.Modifiers())
		}
		for i := range km.keys {
			k := &km.keys[i]
			if !k.exists || !automaticKey(k) {
				continue
			}
			kc := km.minKey + uint32(i)
			want, got := st.Lookup(kc), cst.Lookup(kc)
			if want.Key == got.Key && want.Text == got.Text || want.Key == platform.KeyUnidentified && want.Text == "" {
				continue
			}
			diffs = append(diffs, coreDiff{key: k.name, state: s, got: got, want: want, noLevel3: plain.Lookup(kc)})
		}
	}
	return diffs
}

func reportDiffs(t *testing.T, diffs []coreDiff) {
	t.Helper()
	if len(diffs) == 0 {
		return
	}
	var lines []string
	for _, d := range diffs[:min(len(diffs), 40)] {
		lines = append(lines, d.String())
	}
	t.Errorf("%d differences from the XKB keymap:\n  %s", len(diffs), strings.Join(lines, "\n  "))
}

// TestCoreKeymapMatchesXKB builds keymaps from the core mapping an XKB
// server derives from the fixture keymaps and checks that every key
// resolves as with the full keymap, in the common modifier states (see
// coreDiffs for what is skipped).
func TestCoreKeymapMatchesXKB(t *testing.T) {
	for _, name := range []string{"us", "de", "fr", "es", "latam", "gb", "ru", "gr", "il", "us-intl", "us-altgr-intl", "us-nocaps", "us-compose-ralt", "jp"} {
		t.Run(name, func(t *testing.T) {
			reportDiffs(t, coreDiffs(t, name, coreStates))
		})
	}
}

func TestCoreKeymapEightLevels(t *testing.T) {
	states := append([]coreState(nil), coreStates...)
	states = append(states,
		coreState{"l5", "LevelFive", "", 0},
		coreState{"shiftl5", "Shift+LevelFive", "", 0},
		coreState{"l3l5", "LevelThree+LevelFive", "", 0},
	)
	reportDiffs(t, coreDiffs(t, "de-neo", states))
}

// TestCoreKeymapTwoGroups checks keymaps with two layouts. Levels 1 and 2
// of both groups must resolve exactly. The core list does not say which
// layout a key's AltGr levels belong to when both layouts share levels 1
// and 2 or only one has them (see NewCoreKeymap), so with LevelThree a
// layout without AltGr levels may type the other layout's (where XKB
// ignores AltGr), but the layout that has them must type them.
func TestCoreKeymapTwoGroups(t *testing.T) {
	var states []coreState
	for _, g := range []uint32{0, 1} {
		for _, s := range coreStates {
			s.group = g
			s.name = fmt.Sprintf("%s/g%d", s.name, g+1)
			states = append(states, s)
		}
	}
	for _, name := range []string{"us-ru-grp", "us-de", "de-us"} {
		t.Run(name, func(t *testing.T) {
			var bad []coreDiff
			for _, d := range coreDiffs(t, name, states) {
				// XKB ignores AltGr on this key and group: the core
				// keymap may give another level there.
				if strings.Contains(d.state.mods, "LevelThree") && d.want == d.noLevel3 {
					continue
				}
				bad = append(bad, d)
			}
			reportDiffs(t, bad)
		})
	}

	// The AltGr characters of the layout that has them.
	for _, c := range []struct {
		layout, key string
		group       uint32
		mods        string
		want        platform.Key
	}{
		{"us-de", "AD01", 1, "LevelThree", "@"},
		{"us-de", "AE07", 1, "LevelThree", "{"},
		{"us-de", "AE08", 1, "LevelThree", "["},
		{"us-de", "AD03", 1, "LevelThree", "€"},
		{"us-de", "AD03", 1, "Shift+LevelThree", "€"},
		{"us-de", "AD06", 0, "", "y"},
		{"us-de", "AD06", 1, "", "z"},
		{"us-de", "AD06", 1, "LevelThree", "←"},
		{"de-us", "AD01", 0, "LevelThree", "@"},
		{"de-us", "AE07", 0, "LevelThree", "{"},
		{"de-us", "AE02", 0, "Shift+LevelThree", "⅛"},
		{"de-us", "AD06", 0, "", "z"},
		{"de-us", "AD06", 1, "", "y"},
		{"us-ru-grp", "AE08", 1, "LevelThree", "₽"},
		{"us-ru-grp", "AD01", 1, "", "й"},
	} {
		km := loadFixture(t, c.layout)
		ck, err := NewCoreKeymap(exportCore(km))
		if err != nil {
			t.Fatal(err)
		}
		st := NewState(ck)
		st.UpdateMask(maskOf(t, km, c.mods), 0, 0, c.group)
		if got := st.Lookup(keycodeOf(t, km, c.key)); got.Key != c.want {
			t.Errorf("%s <%s> group %d %s: %q, want %q", c.layout, c.key, c.group+1, c.mods, got.Key, c.want)
		}
	}
}

// TestCoreKeymapHandWritten covers core mappings no XKB server writes:
// xmodmap-style lists with one keysym, Mode_switch on a modifier, and
// AltGr bound to Mod5 next to Alt on Mod1.
func TestCoreKeymapHandWritten(t *testing.T) {
	const (
		kcA     = 38
		kcRALT  = 108
		kcLALT  = 64
		kcSuper = 133
		kcMode  = 203
		kcQ     = 24
		kcKP1   = 87
		kcNum   = 77
	)
	per := 4
	m := CoreMapping{MinKeycode: 8, KeysymsPerKeycode: per, Keysyms: make([]Keysym, per*248)}
	set := func(kc int, syms ...Keysym) { copy(m.Keysyms[(kc-8)*per:], syms) }
	set(kcA, 'a')                        // one keysym: a lower/upper pair
	set(kcQ, 'q', 'Q', 0x6ca, 0x6ea)     // group 2 Cyrillic_shorti / SHORTI
	set(kcRALT, keysymLevel3Shift)       // AltGr
	set(kcLALT, keysymAltL, keysymMetaL) // Alt
	set(kcSuper, keysymSuperL)           // Super
	set(kcMode, keysymModeSwitch)        // Mode_switch
	set(kcKP1, 0xff9c, 0xffb1)           // KP_End, KP_1
	set(kcNum, keysymNumLock)            // Num_Lock
	m.Modifiers[3] = []uint32{kcLALT}    // Mod1: Alt
	m.Modifiers[4] = []uint32{kcNum}     // Mod2: NumLock
	m.Modifiers[5] = []uint32{kcMode}    // Mod3: Mode_switch
	m.Modifiers[6] = []uint32{kcSuper}   // Mod4: Super
	m.Modifiers[7] = []uint32{kcRALT, 0} // Mod5: AltGr (a zero entry is ignored)
	km, err := NewCoreKeymap(m)
	if err != nil {
		t.Fatal(err)
	}
	st := NewState(km)
	const shift, lock, mod1, mod2, mod3, mod4, mod5 = 1, 2, 8, 16, 32, 64, 128
	for _, c := range []struct {
		kc        uint32
		mods      uint32
		group     uint32
		key, text string
	}{
		{kcA, 0, 0, "a", "a"},
		{kcA, shift, 0, "A", "A"},
		{kcA, lock, 0, "A", "A"},
		{kcA, mod5, 0, "a", "a"},
		{kcQ, 0, 0, "q", "q"},
		{kcQ, shift, 0, "Q", "Q"},
		{kcQ, 0, 1, "й", "й"},
		{kcQ, shift, 1, "Й", "Й"},
		{kcKP1, 0, 0, string(platform.KeyEnd), ""},
		{kcKP1, mod2, 0, "1", "1"},
		{kcRALT, 0, 0, string(platform.KeyAltGraph), ""},
		{kcMode, 0, 0, string(platform.KeyModeChange), ""},
	} {
		st.UpdateMask(c.mods, 0, 0, c.group)
		if l := st.Lookup(c.kc); string(l.Key) != c.key || l.Text != c.text {
			t.Errorf("keycode %d mods %#x group %d: %q/%q, want %q/%q", c.kc, c.mods, c.group, l.Key, l.Text, c.key, c.text)
		}
	}
	for _, c := range []struct {
		mods uint32
		want platform.Modifiers
	}{
		{mod1, platform.ModAlt},
		{mod4, platform.ModSuper},
		{mod5, 0}, // AltGr is not Alt
		{mod3, 0}, // Mode_switch is not a modifier
		{mod2, 0}, // Num Lock is not either
		{shift | 4, platform.ModShift | platform.ModCtrl},
	} {
		st.UpdateMask(c.mods, 0, 0, 0)
		if got := st.Modifiers(); got != c.want {
			t.Errorf("mods %#x: Modifiers %v, want %v", c.mods, got, c.want)
		}
	}
	if mask, ok := km.ModMask("ModeSwitch"); !ok || mask != mod3 {
		t.Errorf("ModMask(ModeSwitch) = %#x, %v; want Mod3", mask, ok)
	}
	if mask, ok := km.ModMask("LevelThree"); !ok || mask != mod5 {
		t.Errorf("ModMask(LevelThree) = %#x, %v; want Mod5", mask, ok)
	}
	if km.Repeats(kcRALT) || km.Repeats(kcLALT) || !km.Repeats(kcA) {
		t.Errorf("repeat flags: AltGr %v, Alt %v, a %v", km.Repeats(kcRALT), km.Repeats(kcLALT), km.Repeats(kcA))
	}
	if got := km.HeldModifiers(kcLALT); got != platform.ModAlt {
		t.Errorf("HeldModifiers(Alt_L) = %v", got)
	}
	if got := km.HeldModifiers(kcRALT); got != 0 {
		t.Errorf("HeldModifiers(ISO_Level3_Shift) = %v", got)
	}

	for _, bad := range []CoreMapping{
		{MinKeycode: 8, KeysymsPerKeycode: 0},
		{MinKeycode: 8, KeysymsPerKeycode: 2, Keysyms: make([]Keysym, 3)},
		{MinKeycode: 8, KeysymsPerKeycode: 2},
	} {
		if _, err := NewCoreKeymap(bad); err == nil {
			t.Errorf("NewCoreKeymap(%+v) succeeded", bad)
		}
	}
}

func TestRepeatPeriod(t *testing.T) {
	for _, c := range []struct {
		in   []Keysym
		want int // 0: no repetition
	}{
		{[]Keysym{1, 2, 1, 2}, 2},
		{[]Keysym{1, 2, 3, 1, 2, 3}, 3},
		{[]Keysym{1, 0, 1}, 2},
		{[]Keysym{1, 2, 3, 1}, 3}, // cut short by the list width
		{[]Keysym{1, 2}, 0},
		{[]Keysym{1}, 0},
	} {
		if p, ok := repeatPeriod(c.in); p != c.want || ok != (c.want > 0) {
			t.Errorf("repeatPeriod(%v) = %d, %v; want %d", c.in, p, ok, c.want)
		}
	}
}
