package main

import (
	"bytes"
	"os"
	"testing"
)

// The committed dates_gen.go is what the generator makes of the vendored
// CLDR files.
func TestGeneratedFileIsCurrent(t *testing.T) {
	want, err := generate("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../dates_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("i18n/dates_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

// The committed numbering_gen.go is what the generator makes of the
// vendored numbering data.
func TestNumberingFileIsCurrent(t *testing.T) {
	want, err := generateNumbering("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../numbering_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("i18n/numbering_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

// The committed numberformats_gen.go is what the generator makes of the
// vendored number symbols.
func TestNumberFormatsFileIsCurrent(t *testing.T) {
	want, err := generateNumberFormats("cldr")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../numberformats_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("i18n/numberformats_gen.go is out of date; run \"go generate ./i18n\"")
	}
}

func TestParsePattern(t *testing.T) {
	for _, c := range []struct {
		pattern, prefix, suffix string
		primary, secondary      int
	}{
		{"#,##0.###", "", "", 3, 3},
		{"#,##,##0.###", "", "", 3, 2},
		{"#,##0%", "", pctMark, 3, 3},
		{"#,##0\u00a0%", "", "\u00a0" + pctMark, 3, 3},
		{"%\u00a0#,##0", pctMark + "\u00a0", "", 3, 3},
		{"\u200e#,##0'%'\u200e", "\u200e", "%\u200e", 3, 3},
		{"0.###", "", "", 0, 0},
		{"#,##0.###;(#,##0.###)", "", "", 3, 3},
	} {
		prefix, suffix, primary, secondary, err := parsePattern(c.pattern)
		if err != nil || prefix != c.prefix || suffix != c.suffix || primary != c.primary || secondary != c.secondary {
			t.Errorf("parsePattern(%q) = %q, %q, %d, %d, %v; want %q, %q, %d, %d", c.pattern, prefix, suffix, primary, secondary, err, c.prefix, c.suffix, c.primary, c.secondary)
		}
	}
	for _, p := range []string{"#,##0\u2030", "\u00a4#,##0", "#,##0 'x", "abc"} {
		if _, _, _, _, err := parsePattern(p); err == nil {
			t.Errorf("parsePattern(%q): no error", p)
		}
	}
}

func TestParentOf(t *testing.T) {
	for name, want := range map[string]string{"ar": "", "ar-EG": "ar", "ff-Adlm-BF": "ff-Adlm"} {
		if got := parentOf(name); got != want {
			t.Errorf("parentOf(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestUnsupportedLetter(t *testing.T) {
	for p, want := range map[string]string{
		"EEEE, d 'de' MMMM 'de' y": "",
		"h:mm:ss a zzzz":           "",
		"{1} 'at' {0}":             "",
		"yyyy-QQQ":                 "Q",
		"y 'Q' w":                  "w",
		"y '' y":                   "",
	} {
		if got := unsupportedLetter(p); got != want {
			t.Errorf("unsupportedLetter(%q) = %q, want %q", p, got, want)
		}
	}
}
