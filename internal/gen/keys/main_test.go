package main

import (
	"bytes"
	"os"
	"testing"
)

func readSources(t *testing.T) (key, code []byte) {
	t.Helper()
	key, err := os.ReadFile("../../platform/key.go")
	if err != nil {
		t.Fatal(err)
	}
	code, err = os.ReadFile("../../platform/keycode.go")
	if err != nil {
		t.Fatal(err)
	}
	return key, code
}

// TestGeneratedFileIsUpToDate fails when the tela package's keys_gen.go
// does not match the platform's key constants.
func TestGeneratedFileIsUpToDate(t *testing.T) {
	want, err := build(readSources(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../../../keys_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("keys_gen.go is stale: run \"go generate\" in the module root and commit the result")
	}
}
