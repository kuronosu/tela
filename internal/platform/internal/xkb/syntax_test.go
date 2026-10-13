package xkb

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// legacyKeymap is written in the style of older libxkbcommon dumps (and
// of xkbcomp -xkb): Group1 indices, modifier maps by keysym, explicit
// virtual modifier maps, more syntax than the fixtures use, plus a key
// with several keysyms on a level and an out-of-range group policy.
const legacyKeymap = `
xkb_keymap {
xkb_keycodes "test" {
	minimum = 8;
	maximum = 255;
	<AC01> = 38;  <AC02> = 39; <LFSH> = 50; <RALT> = 108; <LCTL> = 37;
	<AE01> = 10;  <NMLK> = 77; <KP1> = 87; <MULT> = 200; <REDR> = 201;
	<LWIN> = 133; <CLMP> = 202; <WRAP> = 203;
	indicator 1 = "Caps Lock";
	alias <ALGR> = <RALT>;
};
xkb_types "test" {
	virtual_modifiers NumLock,Alt,LevelThree,Super;
	type "ONE_LEVEL" { modifiers= none; level_name[Level1]= "Any"; };
	type "TWO_LEVEL" { modifiers= Shift; map[Shift]= Level2; level_name[Level1]= "Base"; level_name[Level2]= "Shift"; };
	type "ALPHABETIC" { modifiers= Shift+Lock; map[Shift]= Level2; map[Lock]= Level2; };
	type "KEYPAD" { modifiers= Shift+NumLock; map[None]= Level1; map[Shift]= Level2; map[NumLock]= Level2; map[Shift+NumLock]= Level1; };
	type "FOUR_LEVEL" { modifiers= Shift+LevelThree; map[Shift]= 2; map[LevelThree]= 3; map[Shift+LevelThree]= 4; };
};
xkb_compatibility "test" {
	virtual_modifiers NumLock,Alt,LevelThree,Super;
	interpret.useModMapMods= AnyLevel;
	interpret.repeat= False;
	interpret.locking= False;
	interpret ISO_Level3_Shift+AnyOf(all) { virtualModifier= LevelThree; useModMapMods=level1; action= SetMods(modifiers=LevelThree,clearLocks); };
	interpret Num_Lock+AnyOf(all) { virtualModifier= NumLock; action= LockMods(modifiers=NumLock); };
	interpret Alt_L+AnyOf(all) { virtualModifier= Alt; action= SetMods(modifiers=modMapMods,clearLocks); };
	interpret Super_L+AnyOf(all) { virtualModifier= Super; action= SetMods(modifiers=modMapMods,clearLocks); };
	interpret Shift_L+AnyOfOrNone(all) { action= SetMods(modifiers=Shift,clearLocks); };
	interpret Any+AnyOf(all) { action= SetMods(modifiers=modMapMods,clearLocks); };
	group 2 = AltGr;
	indicator "Caps Lock" { !allowExplicit; whichModState= Locked; modifiers= Lock; };
};
xkb_symbols "test" {
	name[Group1]="Test";
	key <AC01> { type[Group1]= "ALPHABETIC", symbols[Group1]= [ a, A ] };
	key <AC02> { [ s, S ], [ Cyrillic_yeru, Cyrillic_YERU ], [ Greek_sigma, Greek_SIGMA ] };
	key <AE01> { type= "FOUR_LEVEL", [ 1, exclam, onesuperior, exclamdown ] };
	key <LFSH> { [ Shift_L ] };
	key <LCTL> { repeat= No, [ Control_L ] };
	key <RALT> { type[Group1]= "ONE_LEVEL", symbols[Group1]= [ ISO_Level3_Shift ] };
	key <NMLK> { [ Num_Lock ] };
	key <KP1> { [ KP_End, KP_1 ] };
	key <LWIN> { virtualMods= Super, [ Super_L ] };
	key <MULT> { [ { a, b }, { A, B } ] };
	key <REDR> { groupsRedirect= Group1, [ x, X ], [ y, Y ] };
	key <CLMP> { groupsClamp, [ x, X ], [ y, Y ] };
	key <WRAP> { [ x, X ], [ y, Y ] };
	modifier_map Shift { Shift_L };
	modifier_map Control { <LCTL> };
	modifier_map Mod2 { Num_Lock };
	modifier_map Mod4 { <LWIN> };
	modifier_map Mod5 { <ALGR> };
};
};
`

func TestLegacySyntax(t *testing.T) {
	km, err := ParseKeymap([]byte(legacyKeymap))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]uint32{"NumLock": 0x10, "LevelThree": 0x80, "Super": 0x40, "Alt": 0} {
		if got, ok := km.ModMask(name); !ok || got != want {
			t.Errorf("ModMask(%s) = %#x %v, want %#x", name, got, ok, want)
		}
	}
	if km.NumGroups() != 3 {
		t.Errorf("NumGroups = %d, want 3", km.NumGroups())
	}
	if km.Repeats(37) || km.Repeats(50) || !km.Repeats(38) {
		t.Errorf("repeats: LCTL %v (explicit No), LFSH %v (interpretation), AC01 %v (default)", km.Repeats(37), km.Repeats(50), km.Repeats(38))
	}
	st := NewState(km)
	for _, c := range []struct {
		kc             uint32
		dep, lock, grp uint32
		key            platform.Key
		text           string
	}{
		{38, 0, 0, 0, "a", "a"},
		{38, 0, 0x2, 0, "A", "A"},
		{39, 0, 0, 1, "ы", "ы"},
		{39, 0x1, 0, 1, "Ы", "Ы"},
		{10, 0x80, 0, 0, "¹", "¹"},
		{10, 0x81, 0, 0, "¡", "¡"},
		{87, 0, 0x10, 0, "1", "1"},
		{87, 0x1, 0x10, 0, platform.KeyEnd, ""},
		{200, 0, 0, 0, "ab", "ab"}, // several keysyms on one level
		{200, 0x1, 0, 0, "AB", "AB"},
		{201, 0, 0, 1, "y", "y"},
		{201, 0, 0, 2, "x", "x"}, // beyond the key's groups: redirected to group 1
		{202, 0, 0, 2, "y", "y"}, // clamped to the last group
		{203, 0, 0, 2, "x", "x"}, // wrapped
		{39, 0, 0, 2, "σ", "σ"},
		{39, 0, 0, 4, "ы", "ы"}, // the effective group wraps into the keymap's groups first
		{38, 0, 0, 3, "a", "a"}, // wraps
		{108, 0, 0, 0, platform.KeyAltGraph, ""},
		{133, 0, 0, 0, platform.KeyMeta, ""},
	} {
		st.UpdateMask(c.dep, 0, c.lock, c.grp)
		got := st.Lookup(c.kc)
		if got.Key != c.key || got.Text != c.text {
			t.Errorf("key %d dep %#x lock %#x group %d: %q %q, want %q %q", c.kc, c.dep, c.lock, c.grp, got.Key, got.Text, c.key, c.text)
		}
	}
	st.UpdateMask(0x40|0x8, 0, 0, 0)
	if m := st.Modifiers(); m != platform.ModSuper|platform.ModAlt {
		t.Errorf("Modifiers with Mod4+Mod1 and Alt unmapped = %v, want Super|Alt (Mod1 fallback)", m)
	}
	if l := st.Lookup(999); l.Key != platform.KeyUnidentified || l.Text != "" {
		t.Errorf("unknown keycode: %+v", l)
	}
}
