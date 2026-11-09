package kit

import (
	"testing"

	"github.com/kuronosu/tela/desktop"
)

// A radio item stays a radio in a native menu, as in a drawn one, even
// with a Role (which dev mode rejects): it is checked as the group says
// and choosing it runs the menu's choice, never the role.
func TestNativeRadioItemIgnoresRole(t *testing.T) {
	v := "a"
	items := []MenuItem[string]{{Kind: MenuRadioGroup, Key: "g", Value: &v, Items: []MenuItem[string]{
		{Key: "a", Label: "A", Role: desktop.RoleCopy}, {Key: "b", Label: "B"}}}}
	var chosen []string
	store := &menuStore[string]{checked: map[string]bool{}, radio: map[string]string{}}
	out := nativeItems(items, store, false, func(k string) { chosen = append(chosen, k) })
	if len(out) != 2 {
		t.Fatalf("items %+v", out)
	}
	a := out[0]
	if a.Role != desktop.RoleNone || !a.Checkbox || !a.Checked || a.OnSelect == nil {
		t.Fatalf("radio item with a role %+v, want a checked check item with an OnSelect and no role", a)
	}
	a.OnSelect()
	if len(chosen) != 1 || chosen[0] != "a" {
		t.Errorf("chosen %v", chosen)
	}
}
