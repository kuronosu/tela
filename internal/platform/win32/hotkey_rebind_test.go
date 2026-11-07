package win32

import (
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
