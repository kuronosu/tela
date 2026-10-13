package xkb

import "github.com/kuronosu/tela/internal/platform"

// State is the modifier and group state of one keyboard, as the compositor
// reports it (wl_keyboard.modifiers): the client never runs XKB actions
// itself, it only resolves keys with this state. The zero State of a
// keymap has no modifiers and the first group.
type State struct {
	km                         *Keymap
	depressed, latched, locked uint32
	group                      uint32
	effective                  uint32
}

// NewState returns the state of a keyboard using km, with no modifiers.
func NewState(km *Keymap) *State { return &State{km: km} }

// Keymap returns the keymap of the state.
func (s *State) Keymap() *Keymap { return s.km }

// UpdateMask sets the state from the masks and the effective group of
// wl_keyboard.modifiers (xkb_state_update_mask with only the effective
// group, which is what Wayland sends).
func (s *State) UpdateMask(depressed, latched, locked, group uint32) {
	s.depressed, s.latched, s.locked = depressed, latched, locked
	s.effective = s.km.resolve(depressed | latched | locked)
	s.group = group
	if n := uint32(s.km.numGroups); n > 0 && s.group >= n {
		s.group %= n
	}
}

// Group returns the effective group (layout index).
func (s *State) Group() uint32 { return s.group }

// Modifiers returns the held shortcut modifiers of platform.Modifiers:
// Shift, Control, Alt (the real modifier the Alt virtual modifier maps to,
// usually Mod1) and Super (Mod4). AltGr (LevelThree), LevelFive and the
// lock states are never reported.
func (s *State) Modifiers() platform.Modifiers {
	// Depressed and latched modifiers are held (a latched modifier
	// applies to the next key, like a held one); locked ones are lock
	// states, not modifiers (ADR-050), so a Shift Lock or a locked Ctrl
	// (sticky keys) is not reported.
	held := s.km.resolve(s.depressed | s.latched)
	var m platform.Modifiers
	if held&s.km.shiftMask != 0 {
		m |= platform.ModShift
	}
	if held&s.km.ctrlMask != 0 {
		m |= platform.ModCtrl
	}
	if held&s.km.altMask != 0 {
		m |= platform.ModAlt
	}
	if held&s.km.superMask != 0 {
		m |= platform.ModSuper
	}
	return m
}

// Lookup is the result of looking up a key in a state.
type Lookup struct {
	// Sym is the keysym (xkb_state_key_get_one_sym, with the Caps Lock
	// transformation), or NoSymbol if the level has none or several.
	Sym Keysym
	// Key is the logical key; Text the text (no control characters).
	Key  platform.Key
	Text string
}

// Lookup returns the keysym, logical key and text of keycode kc (an XKB
// keycode: the evdev code + 8) in the current state, with Control
// ignored (ADR-050): Ctrl neither selects a level nor turns the text into
// a control character. It does not allocate.
func (s *State) Lookup(kc uint32) Lookup {
	k := s.km.keyAt(kc)
	if k == nil || len(k.groups) == 0 {
		return Lookup{Key: platform.KeyUnidentified}
	}
	g := wrapGroup(k, s.group)
	grp := &k.groups[g]
	t := grp.typ
	mods := s.effective &^ s.km.ctrlMask
	lv, preserve := 0, uint32(0)
	want := mods & t.mods
	for i := range t.entries {
		e := &t.entries[i]
		if e.active && e.mods == want {
			lv, preserve = e.level, e.preserve
			break
		}
	}
	if lv >= len(grp.levels) {
		return Lookup{Key: platform.KeyUnidentified}
	}
	l := &grp.levels[lv]
	// Caps Lock transformation: Lock active and not consumed by the
	// key's type (XKB consumed mode: the type's modifiers minus the
	// entry's preserved ones).
	consumed := t.mods &^ preserve
	if mods&s.km.lockMask != 0 && consumed&s.km.lockMask == 0 {
		return Lookup{Sym: l.capsOne, Key: l.capsKey, Text: l.capsText}
	}
	return Lookup{Sym: l.one, Key: l.key, Text: l.text}
}

// wrapGroup brings an effective group into a key's range with the key's
// out-of-range policy (XkbWrapGroupIntoRange).
func wrapGroup(k *key, g uint32) int {
	n := uint32(len(k.groups))
	if g < n {
		return int(g)
	}
	switch k.policy {
	case rangeRedirect:
		if uint32(k.redirect) >= n {
			return 0
		}
		return k.redirect
	case rangeClamp:
		return int(n - 1)
	}
	return int(g % n)
}

// HeldModifiers returns the shortcut modifiers keycode kc sets while it
// is held, if it is a plain modifier key (its first keysym is one of
// Shift, Control, Alt, Meta, Super or Hyper, left or right): the
// modifiers of its modifier map. It is 0 for every other key, lock keys
// included. A Wayland client uses it only to predict KeyEvent.Mods when
// the compositor's wl_keyboard.modifiers does not follow the key event in
// the same read.
func (km *Keymap) HeldModifiers(kc uint32) platform.Modifiers {
	k := km.keyAt(kc)
	if k == nil || len(k.groups) == 0 || len(k.groups[0].levels) == 0 {
		return 0
	}
	syms := k.groups[0].levels[0].syms
	if len(syms) != 1 || syms[0] < keysymShiftL || syms[0] > keysymHyperR || syms[0] == 0xffe5 || syms[0] == 0xffe6 {
		return 0 // not Shift_L … Hyper_R, or Caps_Lock / Shift_Lock
	}
	var m platform.Modifiers
	if k.modmap&km.shiftMask != 0 {
		m |= platform.ModShift
	}
	if k.modmap&km.ctrlMask != 0 {
		m |= platform.ModCtrl
	}
	if k.modmap&km.altMask != 0 {
		m |= platform.ModAlt
	}
	if k.modmap&km.superMask != 0 {
		m |= platform.ModSuper
	}
	return m
}

// Repeats reports whether keycode kc repeats when held (see
// [Keymap.Repeats]).
func (s *State) Repeats(kc uint32) bool { return s.km.Repeats(kc) }
