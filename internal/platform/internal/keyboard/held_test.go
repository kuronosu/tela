package keyboard

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

func press(c platform.KeyCode, k platform.Key) platform.KeyEvent {
	return platform.KeyEvent{Action: platform.KeyPress, Code: c, Key: k, Location: c.Location(), Mods: platform.ModShift}
}

func TestHeld(t *testing.T) {
	var h Held
	shift := press(platform.CodeShiftLeft, platform.KeyShift)
	a := press(platform.CodeKeyA, "A")
	a.Text = "A"
	unknown := press(platform.UnidentifiedKeyCode(0x2f0), platform.KeyUnidentified)

	for _, e := range []platform.KeyEvent{shift, a, unknown} {
		if !h.Accept(e) {
			t.Fatalf("press of %v dropped", e.Code)
		}
	}
	rep := a
	rep.Repeat = true
	if !h.Accept(rep) || h.Len() != 3 {
		t.Errorf("repeat of a held key: len %d", h.Len())
	}
	if h.Accept(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeKeyB, Repeat: true}) {
		t.Errorf("repeat of a key that is not held was accepted")
	}
	if h.Accept(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeKeyB}) {
		t.Errorf("release of a key that is not held was accepted")
	}
	if h.Accept(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.UnidentifiedKeyCode(0x2f1)}) {
		t.Errorf("release of another unidentified key was accepted")
	}
	if !h.Accept(platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeKeyA}) || h.IsHeld(platform.CodeKeyA) {
		t.Errorf("release of a held key")
	}
	if !h.Accept(press(platform.CodeShiftLeft, platform.KeyShift)) || h.Len() != 2 {
		t.Errorf("a second press of a held key added an entry: len %d", h.Len())
	}

	rel := h.ReleaseAll(5 * time.Millisecond)
	if len(rel) != 2 || h.Len() != 0 || h.ReleaseAll(0) != nil {
		t.Fatalf("ReleaseAll = %v, len after %d", rel, h.Len())
	}
	want := []platform.KeyEvent{
		{Action: platform.KeyRelease, Code: platform.CodeShiftLeft, Key: platform.KeyShift, Location: platform.KeyLocationLeft, Synthetic: true, Time: 5 * time.Millisecond},
		{Action: platform.KeyRelease, Code: unknown.Code, Key: platform.KeyUnidentified, Synthetic: true, Time: 5 * time.Millisecond},
	}
	for i := range want {
		if rel[i] != want[i] {
			t.Errorf("synthetic release %d = %+v, want %+v", i, rel[i], want[i])
		}
	}
}
