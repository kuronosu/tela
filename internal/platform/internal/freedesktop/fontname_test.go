//go:build linux

package freedesktop

import "testing"

func TestPangoFamily(t *testing.T) {
	for _, c := range []struct{ desc, want string }{
		{"Adwaita Sans 11", "Adwaita Sans"},
		{"Cantarell 11", "Cantarell"},
		{"Ubuntu 10.5", "Ubuntu"},
		{"Sans 12px", "Sans"},
		{"Sans", "Sans"},
		{"DejaVu Sans Mono Book 10", "DejaVu Sans Mono"},
		{"Cantarell Bold Italic 10", "Cantarell"},
		{"Noto Sans Semi-Bold Condensed 9", "Noto Sans"},
		{"Noto Sans SemiBold 9", "Noto Sans"},
		{"Noto Sans semi-bold 9", "Noto Sans"},
		{"Fira Sans 600 11", "Fira Sans"},
		{"Fira Sans Small-Caps Rotated-Left 11", "Fira Sans"},
		{"Source Code Pro 1001 11", "Source Code Pro 1001"}, // not a weight
		{"DejaVu Sans, Noto Sans, 12", "DejaVu Sans"},
		{"DejaVu Sans,Noto Sans Bold 12", "DejaVu Sans"},
		{"Cantarell, 11", "Cantarell"},
		{`"Fira Sans" Bold 10`, "Fira Sans"},
		{`'Adwaita Sans 11'`, "Adwaita Sans"}, // copied from gsettings get
		{"Inter Display 11 @wght=500", "Inter Display"},
		{"Inter 11 @wght=500 #tnum=1", "Inter"},
		{"  Adwaita Sans   11  ", "Adwaita Sans"},
		{"Noto Sans CJK JP 11", "Noto Sans CJK JP"},
		{"Écran Sans 11", "Écran Sans"},
		{"Sans à 11", "Sans à"}, // 0xA0 inside a UTF-8 sequence is no space
		{"Bold 11", ""},
		{"11", ""},
		{"", ""},
		{", Sans 10", ""},
	} {
		if got := pangoFamily(c.desc); got != c.want {
			t.Errorf("pangoFamily(%q) = %q, want %q", c.desc, got, c.want)
		}
	}
}

func TestQtFamily(t *testing.T) {
	for _, c := range []struct {
		v    any
		want string
	}{
		{"Noto Sans,10,-1,5,50,0,0,0,0,0", "Noto Sans"},
		{"Hack,10,-1,5,400,0,0,0,0,0,0,0,0,0,0,1", "Hack"},
		{" Noto Sans Mono ,9,-1,5,50,0,0,0,0,0", "Noto Sans Mono"},
		{`"Noto Sans",10`, "Noto Sans"},
		{"Noto Sans", "Noto Sans"},
		{[]string{"Noto Sans", "10", "-1"}, "Noto Sans"},
		{[]any{"Noto Sans", "10"}, "Noto Sans"},
		{",10,-1", ""},
		{"", ""},
		{[]string{}, ""},
		{int32(10), ""},
		{"Bad\x00Name,10", ""},
	} {
		if got := qtFamily(c.v); got != c.want {
			t.Errorf("qtFamily(%#v) = %q, want %q", c.v, got, c.want)
		}
	}
}
