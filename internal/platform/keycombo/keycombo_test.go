package keycombo

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// brief prints events as "press Control mods=2".
func brief(evs []platform.KeyEvent) []string {
	var out []string
	for _, e := range evs {
		out = append(out, fmt.Sprintf("%v %s %q %d", e.Action, e.Key, e.Text, e.Mods))
	}
	return out
}

func TestPress(t *testing.T) {
	for _, c := range []struct {
		combo string
		fl    platform.Flavor
		want  []string
	}{
		{"Enter", platform.FlavorLinux, []string{`press Enter "" 0`, `release Enter "" 0`}},
		{"a", platform.FlavorLinux, []string{`press a "a" 0`, `release a "" 0`}},
		{"A", platform.FlavorLinux, []string{`press A "A" 0`, `release A "" 0`}},
		{"Shift+a", platform.FlavorLinux, []string{`press Shift "" 1`, `press A "A" 1`, `release A "" 1`, `release Shift "" 0`}},
		{"ControlOrMeta+s", platform.FlavorMac, []string{`press Meta "" 8`, `press s "s" 8`, `release s "" 8`, `release Meta "" 0`}},
		{"ControlOrMeta+s", platform.FlavorWindows, []string{`press Control "" 2`, `press s "s" 2`, `release s "" 2`, `release Control "" 0`}},
		{"Control++", platform.FlavorLinux, []string{`press Control "" 2`, `press + "+" 2`, `release + "" 2`, `release Control "" 0`}},
		{"Shift", platform.FlavorLinux, []string{`press Shift "" 1`, `release Shift "" 0`}},
	} {
		evs, err := Press(c.combo, c.fl)
		if got := brief(evs); err != nil || !slices.Equal(got, c.want) {
			t.Errorf("Press(%q): %v, %v; want %v", c.combo, got, err, c.want)
		}
		down, _ := Down(c.combo, c.fl)
		up, _ := Up(c.combo, c.fl)
		if !slices.Equal(brief(append(down, up...)), c.want) || len(down) == 0 || len(up) == 0 {
			t.Errorf("Down and Up of %q: %v, %v", c.combo, brief(down), brief(up))
		}
	}
	for _, c := range []struct {
		combo string
		kind  ErrorKind
		name  string
	}{{"", NoKey, ""}, {"Ctrl+s", NotModifier, "Ctrl"}, {"Esc", UnknownKey, "Esc"}} {
		_, err := Press(c.combo, platform.FlavorLinux)
		var e *Error
		if !errors.As(err, &e) || e.Kind != c.kind || e.Name != c.name || e.Error() == "" {
			t.Errorf("Press(%q): %v", c.combo, err)
		}
	}
}

func TestType(t *testing.T) {
	got := brief(Type("aB\n"))
	want := []string{`press a "a" 0`, `release a "" 0`, `press B "B" 0`, `release B "" 0`, `press Enter "" 0`, `release Enter "" 0`}
	if !slices.Equal(got, want) {
		t.Errorf("Type: %v, want %v", got, want)
	}
	if evs := Type("é"); len(evs) != 2 || evs[0].Code != platform.CodeUnidentified || evs[0].Text != "é" {
		t.Errorf("Type of a character the layout has no key for: %+v", evs)
	}
}
