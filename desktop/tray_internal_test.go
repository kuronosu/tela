package desktop

import (
	"image"
	"testing"
)

// An item's AccessKey reaches the platform menu (ADR-259), sameMenu
// tells two menus apart by it, and an access key that no keyboard types
// directly panics.
func TestMenuAccessKey(t *testing.T) {
	a, _ := platformMenu("test", []MenuItem{{Label: "ファイル", AccessKey: 'F', Submenu: []MenuItem{{Label: "開く", AccessKey: 'O', OnSelect: func() {}}}}})
	if a[0].AccessKey != 'F' || a[0].Submenu[0].AccessKey != 'O' {
		t.Fatalf("access keys %q, %q; want F and O", a[0].AccessKey, a[0].Submenu[0].AccessKey)
	}
	b, _ := platformMenu("test", []MenuItem{{Label: "ファイル", AccessKey: 'F', Submenu: []MenuItem{{Label: "開く", OnSelect: func() {}}}}})
	if sameMenu(a, b) {
		t.Error("sameMenu holds for menus that differ only in an AccessKey")
	}
	defer func() {
		if recover() == nil {
			t.Error("AccessKey '開' did not panic")
		}
	}()
	platformMenu("test", []MenuItem{{Label: "開く", AccessKey: '開', OnSelect: func() {}}})
}

// The platform options of a Tray carry Template, and sameTray tells two
// options apart by it.
func TestTrayOptionsTemplate(t *testing.T) {
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	plain, _ := trayOptions(Tray{Icon: icon, Tooltip: "x"})
	tmpl, _ := trayOptions(Tray{Icon: icon, Tooltip: "x", Template: true})
	if plain.Template || !tmpl.Template {
		t.Fatalf("Template: plain %v, template %v; want false and true", plain.Template, tmpl.Template)
	}
	if sameTray(plain, tmpl) {
		t.Errorf("sameTray holds for options that differ only in Template")
	}
	if again, _ := trayOptions(Tray{Icon: icon, Tooltip: "x", Template: true}); !sameTray(tmpl, again) {
		t.Errorf("sameTray does not hold for equal options with Template")
	}
}
