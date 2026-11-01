package xkb

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestAcceleratorKeysym(t *testing.T) {
	for _, c := range []struct {
		key  platform.Key
		want string
	}{
		{"k", "k"},
		{"1", "1"},
		{" ", "space"},
		{"/", "slash"},
		{"[", "bracketleft"},
		{"é", "eacute"},
		{"€", "EuroSign"},
		{"ж", "Cyrillic_zhe"},
		{"😀", "U1F600"},
		{platform.KeyEnter, "Return"},
		{platform.KeyPrintScreen, "Print"},
		{platform.KeyTab, "Tab"},
		{platform.KeyClear, "Clear"},
		{platform.KeyF1, "F1"},
		{platform.KeyF24, "F24"},
		{platform.KeyArrowLeft, "Left"},
		{platform.KeyPageUp, "Prior"},
		{platform.KeyEscape, "Escape"},
		{platform.KeyBackspace, "BackSpace"},
		{platform.KeyDelete, "Delete"},
		{platform.KeyMediaPlayPause, "XF86AudioPlay"},
		{platform.KeyAudioVolumeMute, "XF86AudioMute"},
	} {
		ks, ok := AcceleratorKeysym(c.key)
		if !ok || ks.String() != c.want {
			t.Errorf("AcceleratorKeysym(%q) = %v, %v; want %s", c.key, ks, ok, c.want)
		}
	}
	for _, k := range []platform.Key{"", "ab", platform.KeyUnidentified, "\x00"} {
		if ks, ok := AcceleratorKeysym(k); ok {
			t.Errorf("AcceleratorKeysym(%q) = %v, want none", k, ks)
		}
	}
	// Every named key with a keysym maps back to itself, never through a
	// keypad keysym.
	for _, n := range namedKeysyms {
		ks, ok := AcceleratorKeysym(n.key)
		if !ok {
			t.Errorf("AcceleratorKeysym(%q): no keysym", n.key)
			continue
		}
		if got := keysymKey(ks); got != n.key {
			t.Errorf("AcceleratorKeysym(%q) = %v, whose key is %q", n.key, ks, got)
		}
		if ks.IsKeypad() {
			t.Errorf("AcceleratorKeysym(%q) = %v, a keypad keysym", n.key, ks)
		}
	}
}
