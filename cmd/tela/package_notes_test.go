package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotesPackage packages examples/notes for the three systems from one
// command, and makes the update manifest from the whole output folder:
// every package is an artifact, the rest (the macOS entitlements, the
// .appinstaller files) is skipped.
func TestNotesPackage(t *testing.T) {
	skipShort(t)
	root := repoRoot(t)
	out := t.TempDir()
	// notes is a module of its own (ADR-298): tela runs inside it.
	t.Chdir(filepath.Join(root, "examples", "notes"))
	var stdout, stderr bytes.Buffer
	if code := run([]string{"package", "-os", "linux,windows,darwin", "-arch", "amd64", "-version", "1.1.0",
		"-appinstaller-url", "https://example.com/notes", "-o", out, "."}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela package: %d\n%s%s", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "no icon") {
		t.Errorf("notes has no icon:\n%s", stderr.String())
	}
	t.Chdir(out)
	files, _ := filepath.Glob("*")
	code, log := telaIn(t, out, append([]string{"update", "manifest", "-app-id", "dev.telaui.notes", "-version", "1.1.0"}, files...)...)
	if code != 0 {
		t.Fatalf("tela update manifest over the folder: %d\n%s", code, log)
	}
	for _, s := range []string{"skipped notes.entitlements", "skipped notes-windows-amd64.appinstaller"} {
		if !strings.Contains(log, s) {
			t.Errorf("tela update manifest does not say %q:\n%s", s, log)
		}
	}
	m := readManifest(t, filepath.Join(out, "manifest.json"))
	var got []string
	for _, a := range m.Channels["stable"][0].Artifacts {
		got = append(got, a.OS+"/"+a.Arch+"/"+a.Format+" "+a.URL)
	}
	want := []string{
		"darwin/amd64/app notes-1.1.0-darwin-amd64.zip",
		"linux/amd64/appimage notes-1.1.0-linux-amd64.AppImage",
		"linux/amd64/deb notes-1.1.0-linux-amd64.deb",
		"windows/amd64/msix notes-1.1.0-windows-amd64.msix",
		"windows/amd64/zip notes-1.1.0-windows-amd64.zip",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the artifacts:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
