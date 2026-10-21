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
