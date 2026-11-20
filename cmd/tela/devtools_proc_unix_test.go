//go:build !windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The stand-in for tela is a shell script, so this runs where there is a
// shell.
// startDevtools runs exe devtools -addr ADDR and stops it on request.
func TestStartDevtools(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	exe := filepath.Join(dir, "fake-tela")
	script := "#!/bin/sh\necho \"$@\" > " + log + "\nexec sleep 60\n"
	if err := os.WriteFile(exe, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var errb bytes.Buffer
	stop, err := startDevtools(exe, "/run/tela/tools.sock", &errb)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if b, _ := os.ReadFile(log); strings.TrimSpace(string(b)) == "devtools -addr /run/tela/tools.sock" {
			break
		}
		if time.Now().After(deadline) {
			b, _ := os.ReadFile(log)
			t.Fatalf("the program was started with %q", b)
		}
		time.Sleep(5 * time.Millisecond)
	}
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stop did not end the program")
	}
	stop() // stopping again does nothing
	if _, err := startDevtools(filepath.Join(dir, "missing"), "x", &errb); err == nil {
		t.Error("a missing program started")
	}
}
