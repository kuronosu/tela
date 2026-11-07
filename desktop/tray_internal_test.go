package desktop

import (
	"image"
	"testing"
)

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
