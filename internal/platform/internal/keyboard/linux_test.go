//go:build linux

package keyboard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/evdev"
	"github.com/kuronosu/tela/internal/platform/internal/xkb"
)

// testKeymap is a core keymap with both Shifts on Shift, Left Control on
// Control and the A key.
func testKeymap(t *testing.T) *xkb.Keymap {
	t.Helper()
	const per = 2
	m := xkb.CoreMapping{MinKeycode: 8, KeysymsPerKeycode: per, Keysyms: make([]xkb.Keysym, per*248)}
	set := func(c platform.KeyCode, ks xkb.Keysym) uint32 {
		n, ok := evdev.Code(c)
		if !ok {
			t.Fatalf("no evdev code for %v", c)
		}
		kc := n + evdev.X11Offset
		m.Keysyms[(kc-8)*per] = ks
		return kc
	}
	shiftL := set(platform.CodeShiftLeft, 0xffe1)
	shiftR := set(platform.CodeShiftRight, 0xffe2)
	ctrlL := set(platform.CodeControlLeft, 0xffe3)
	set(platform.CodeKeyA, 'a')
	m.Modifiers[0] = []uint32{shiftL, shiftR}
	m.Modifiers[2] = []uint32{ctrlL}
	km, err := xkb.NewCoreKeymap(m)
	if err != nil {
		t.Fatal(err)
	}
	return km
}

func TestOtherModifiers(t *testing.T) {
	km := testKeymap(t)
	press := func(h *Held, c platform.KeyCode) {
		h.Accept(platform.KeyEvent{Action: platform.KeyPress, Code: c})
	}
	var h Held
	press(&h, platform.CodeShiftLeft)
	if got := h.OtherModifiers(km, platform.CodeShiftLeft); got != 0 {
		t.Errorf("Shift alone: others set %v", got)
	}
	press(&h, platform.CodeKeyA)
	press(&h, platform.CodeShiftRight)
	press(&h, platform.CodeControlLeft)
	for _, c := range []struct {
		code platform.KeyCode
		want platform.Modifiers
	}{
		{platform.CodeShiftLeft, platform.ModShift | platform.ModCtrl}, // the right Shift stays
		{platform.CodeControlLeft, platform.ModShift},
		{platform.CodeKeyA, platform.ModShift | platform.ModCtrl},
		{platform.CodeKeyB, platform.ModShift | platform.ModCtrl}, // not held
	} {
		if got := h.OtherModifiers(km, c.code); got != c.want {
			t.Errorf("release of %v: others set %v, want %v", c.code, got, c.want)
		}
	}
}

func TestLoadCompose(t *testing.T) {
	want := &xkb.ComposeTable{}
	if got, err := LoadCompose(func() *xkb.ComposeTable { return want }); got != want || err != nil {
		t.Errorf("override: %p, %v", got, err)
	}
	// Under go test the system is not read.
	t.Setenv("TELA_SYSTEM_COMPOSE", "")
	dir := t.TempDir()
	file := filepath.Join(dir, "XCompose")
	if err := os.WriteFile(file, []byte("<dead_acute> <e> : \"é\" eacute\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	t.Setenv("XCOMPOSEFILE", file)
	t.Setenv("XDG_CONFIG_HOME", empty)
	t.Setenv("HOME", empty)
	t.Setenv("XLOCALEDIR", empty)
	if got, err := LoadCompose(nil); got != nil || err != nil {
		t.Errorf("under go test: %v, %v", got, err)
	}
	// TELA_SYSTEM_COMPOSE reads the user's file.
	t.Setenv("TELA_SYSTEM_COMPOSE", "1")
	if got, err := LoadCompose(nil); got == nil || err != nil {
		t.Errorf("with TELA_SYSTEM_COMPOSE: %v, %v", got, err)
	}
	// No Compose file at all is not an error.
	t.Setenv("XCOMPOSEFILE", filepath.Join(empty, "missing"))
	if got, err := LoadCompose(nil); got != nil || err != nil {
		t.Errorf("no Compose file: %v, %v", got, err)
	}
}
