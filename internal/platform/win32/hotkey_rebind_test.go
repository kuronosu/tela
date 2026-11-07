package win32

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestHotkeyRebind(t *testing.T) {
	// Two layouts for ';' and 'z': the US one and one where they move.
	us := map[rune]uint32{';': 0xBA, 'z': 'Z'}
	de := map[rune]uint32{';': 0xBC, 'z': 'Y'}
	none := map[rune]uint32{}
	scan := func(l map[rune]uint32) func(rune) (uint32, bool) {
		return func(r rune) (uint32, bool) {
			vk, ok := l[r]
			return vk, ok
		}
	}
	for _, tc := range []struct {
		name  string
		key   platform.Key
		held  uint32
		layer map[rune]uint32
		vk    uint32
		again bool
		fails bool
	}{
		{"a named key never moves", "F12", vkF1 + 11, de, vkF1 + 11, false, false},
		{"same key in the new layout", ";", 0xBA, us, 0xBA, false, false},
		{"punctuation moves", ";", 0xBA, de, 0xBC, true, false},
		{"a letter moves", "z", 'Z', de, 'Y', true, false},
		{"found where it had none", ";", 0, us, 0xBA, true, false},
		{"still none", "$", 0, none, 0, false, true},
		{"lost its key", "$", 0x34, none, 0, true, true},
		{"letters fall back to their own key", "k", 'K', none, 'K', false, false},
	} {
		vk, again, err := hotkeyRebind(tc.key, tc.held, scan(tc.layer))
		if vk != tc.vk || again != tc.again || (err != nil) != tc.fails {
			t.Errorf("%s: hotkeyRebind = %#x, %v, %v; want %#x, %v, failing %v", tc.name, vk, again, err, tc.vk, tc.again, tc.fails)
		}
	}
}

// fakeHotkeys is RegisterHotKey's table: a combination is taken once, by
// this process or by another app.
type fakeHotkeys struct {
	taken map[[2]uint32]string // (modifiers, vk) -> owner
	log   []string
}

func (f *fakeHotkeys) unbind(r *hotkeyReg) {
	delete(f.taken, [2]uint32{hotkeyModifiers(r.s.Accelerator.Mods), r.vk})
	r.registered = false
	f.log = append(f.log, "unbind "+r.s.ID)
}

func (f *fakeHotkeys) bind(r *hotkeyReg) (platform.ShortcutStatus, string) {
	k := [2]uint32{hotkeyModifiers(r.s.Accelerator.Mods), r.vk}
	if _, ok := f.taken[k]; ok {
		return platform.ShortcutTaken, "taken"
	}
	f.taken[k] = r.s.ID
	r.registered = true
	f.log = append(f.log, "bind "+r.s.ID)
	return platform.ShortcutBound, ""
}

// TestRelayoutHotkeysTradingKeys: Ctrl+A and Ctrl+Q trade keys from QWERTY
// to AZERTY. Registered one by one, the first would collide with the
// other's old registration and report ShortcutTaken; every moving hot key
// is released first, so both stay bound and nothing is reported.
func TestRelayoutHotkeysTradingKeys(t *testing.T) {
	ctrl := platform.ModCtrl
	reg := func(id string, key platform.Key, vk uint32) *hotkeyReg {
		return &hotkeyReg{s: platform.Shortcut{ID: id, Accelerator: platform.Accelerator{Mods: ctrl, Key: key}},
			vk: vk, registered: true, status: platform.ShortcutBound}
	}
	a, q, z := reg("a", "a", 'A'), reg("q", "q", 'Q'), reg("z", "z", 'Z')
	other := reg("other", "m", 0xBC) // another app's Ctrl+(the key AZERTY types m with)
	m := &hotkeyReg{s: platform.Shortcut{ID: "m", Accelerator: platform.Accelerator{Mods: ctrl, Key: "m"}},
		vk: 'M', registered: true, status: platform.ShortcutBound}
	f := &fakeHotkeys{taken: map[[2]uint32]string{}}
	for _, r := range []*hotkeyReg{a, q, z, m} {
		f.taken[[2]uint32{hotkeyModifiers(ctrl), r.vk}] = r.s.ID
	}
	f.taken[[2]uint32{hotkeyModifiers(ctrl), other.vk}] = "another app"
	azerty := map[rune]uint32{'a': 'Q', 'q': 'A', 'z': 'W', 'm': 0xBC}
	var reports []string
	relayoutHotkeys([]*hotkeyReg{a, m, q, z}, func(r rune) (uint32, bool) { vk, ok := azerty[r]; return vk, ok },
		f.unbind, f.bind, func(r *hotkeyReg, s platform.ShortcutStatus, reason string) {
			reports = append(reports, r.s.ID+" "+s.String()+" "+reason)
		})
	if a.vk != 'Q' || !a.registered || q.vk != 'A' || !q.registered || z.vk != 'W' || !z.registered {
		t.Errorf("after the layout change a = %+v, q = %+v, z = %+v; want bound under Q, A and W", a, q, z)
	}
	if m.registered || m.status != platform.ShortcutTaken {
		t.Errorf("m = %+v, want Taken (another app holds the key)", m)
	}
	want := "[m taken the keyboard layout changed: taken]"
	if got := fmt.Sprint(reports); got != want {
		t.Errorf("reports = %s, want %s (only the status that changed)", got, want)
	}
	if got := fmt.Sprint(f.log); got != "[unbind a unbind m unbind q unbind z bind a bind q bind z]" {
		t.Errorf("calls = %s, want every unbind before the binds", got)
	}

	// Back to QWERTY: m finds its key again and reports Bound.
	reports = nil
	qwerty := map[rune]uint32{'a': 'A', 'q': 'Q', 'z': 'Z', 'm': 'M'}
	relayoutHotkeys([]*hotkeyReg{a, m, q, z}, func(r rune) (uint32, bool) { vk, ok := qwerty[r]; return vk, ok },
		f.unbind, f.bind, func(r *hotkeyReg, s platform.ShortcutStatus, reason string) {
			reports = append(reports, r.s.ID+" "+s.String()+" "+reason)
		})
	if got := fmt.Sprint(reports); got != "[m bound ]" {
		t.Errorf("reports back on QWERTY = %s, want [m bound ]", got)
	}
	if !a.registered || !m.registered || !q.registered || !z.registered {
		t.Errorf("back on QWERTY not all are bound: a %+v m %+v q %+v z %+v", a, m, q, z)
	}
}
