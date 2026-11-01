package win32

import (
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestBuildMenuForWindows(t *testing.T) {
	entries, cmds := buildMenu([]platform.MenuItem{
		{Label: "&File", Submenu: []platform.MenuItem{
			{ID: 10, Label: "Open…", Accelerator: "Ctrl+O"},
			{Separator: true, Accelerator: "ignored"},
			{ID: 11, Label: "Save & close", Accelerator: "Ctrl+Shift+S", Disabled: true},
			{ID: 12, Label: "Word wrap", Checkable: true, Checked: true},
		}},
		{ID: 20, Label: "Help\tme", Accelerator: "F1"},
	})
	if !slices.Equal(cmds, []platform.MenuItemID{10, 11, 12, 20}) {
		t.Fatalf("cmds = %v", cmds)
	}
	file := entries[0]
	if file.label != "&&File" || file.flags != mfPopup || len(file.sub) != 4 {
		t.Errorf("submenu entry = %+v", file)
	}
	want := []menuEntry{
		{label: "Open…\tCtrl+O", cmd: 1},
		{flags: mfSeparator},
		{label: "Save && close\tCtrl+Shift+S", cmd: 2, flags: mfGrayed},
		{label: "Word wrap", cmd: 3, flags: mfChecked},
	}
	for i, e := range want {
		if got := file.sub[i]; got.label != e.label || got.flags != e.flags || got.cmd != e.cmd {
			t.Errorf("entry %d = %+v, want %+v", i, got, e)
		}
	}
	if got := entries[1]; got.label != "Help me\tF1" || got.cmd != 4 {
		t.Errorf("top-level command = %+v", got)
	}
}

func TestMenuCommand(t *testing.T) {
	cmds := []platform.MenuItemID{10, 11}
	for _, tc := range []struct {
		wparam, lparam uintptr
		want           platform.MenuItemID
		ok             bool
	}{
		{1, 0, 10, true},
		{2, 0, 11, true},
		{0, 0, 0, false},
		{3, 0, 0, false},
		{1 | 1<<16, 0, 0, false}, // an accelerator
		{1, 0x1234, 0, false},    // a control's notification
	} {
		got, ok := menuCommand(cmds, tc.wparam, tc.lparam)
		if got != tc.want || ok != tc.ok {
			t.Errorf("menuCommand(%#x, %#x) = %d, %v; want %d, %v", tc.wparam, tc.lparam, got, ok, tc.want, tc.ok)
		}
	}
	if got := popupCommand(cmds, 2); got != 11 {
		t.Errorf("popupCommand(2) = %d", got)
	}
	for _, cmd := range []uintptr{0, 3, ^uintptr(0)} {
		if got := popupCommand(cmds, cmd); got != 0 {
			t.Errorf("popupCommand(%#x) = %d, want 0", cmd, got)
		}
	}
}

func TestPhysicalPoint(t *testing.T) {
	for _, tc := range []struct {
		p     platform.Point
		scale float64
		x, y  int32
	}{
		{platform.Point{X: 10, Y: 20}, 1, 10, 20},
		{platform.Point{X: 10.5, Y: 3}, 1.5, 16, 5}, // 15.75, 4.5: half away from zero
		{platform.Point{X: -2.5, Y: 0}, 1, -3, 0},
	} {
		if x, y := physicalPoint(tc.p, tc.scale); x != tc.x || y != tc.y {
			t.Errorf("physicalPoint(%v, %v) = %d, %d; want %d, %d", tc.p, tc.scale, x, y, tc.x, tc.y)
		}
	}
}
