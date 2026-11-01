package platform

import (
	"strings"
	"testing"
)

func TestParseAccelerator(t *testing.T) {
	for _, c := range []struct {
		in   string
		mac  bool
		want Accelerator
	}{
		{"Ctrl+Alt+N", false, Accelerator{ModCtrl | ModAlt, "n"}},
		{"ctrl + shift + k", false, Accelerator{ModCtrl | ModShift, "k"}},
		{"Shift+F5", false, Accelerator{ModShift, KeyF5}},
		{"super+space", false, Accelerator{ModSuper, " "}},
		{"Ctrl++", false, Accelerator{ModCtrl, "+"}},
		{"Ctrl+Plus", false, Accelerator{ModCtrl, "+"}},
		{"Alt+Minus", false, Accelerator{ModAlt, "-"}},
		{"Control+Option+Esc", false, Accelerator{ModCtrl | ModAlt, KeyEscape}},
		{"Win+Up", false, Accelerator{ModSuper, KeyArrowUp}},
		{"Cmd+pagedown", false, Accelerator{ModSuper, KeyPageDown}},
		{"MediaPlayPause", false, Accelerator{0, KeyMediaPlayPause}},
		{"CmdOrCtrl+S", false, Accelerator{ModCtrl, "s"}},
		{"CommandOrControl+S", true, Accelerator{ModSuper, "s"}},
		{"Ctrl+/", false, Accelerator{ModCtrl, "/"}},
		{"Ctrl+Return", false, Accelerator{ModCtrl, KeyEnter}},
	} {
		got, err := parseAccelerator(c.in, c.mac)
		if err != nil || got != c.want {
			t.Errorf("parseAccelerator(%q, %v) = %+v, %v; want %+v", c.in, c.mac, got, err, c.want)
		}
	}
	// What String formats parses back to the same value.
	for _, a := range []Accelerator{{ModCtrl | ModAlt | ModShift | ModSuper, "k"}, {ModShift, " "}, {0, KeyF12}, {ModCtrl, "+"}} {
		if got, err := ParseAccelerator(a.String()); err != nil || got != a {
			t.Errorf("ParseAccelerator(%q) = %+v, %v; want %+v", a.String(), got, err, a)
		}
	}
	for _, c := range []struct{ in, msg string }{
		{"", "empty"},
		{"Ctrl+", "empty part"},
		{"Ctrl", "ends with the modifier"},
		{"Hyper+K", "not a modifier"},
		{"Ctrl+Ctrl+K", "appears twice"},
		{"Ctrl+Shift", "ends with the modifier"},
		{"Ctrl+Foo", "neither one character"},
		{"Ctrl+CapsLock", "cannot be a shortcut's key"},
	} {
		_, err := ParseAccelerator(c.in)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("ParseAccelerator(%q) = %v; want an error containing %q", c.in, err, c.msg)
		}
	}
}
