//go:build darwin

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPackageMacTools checks the macOS formats with macOS's own tools,
// which only a Mac has (the macOS CI runner): the app zip unpacked by
// ditto (the engine of Finder's Archive Utility) keeps its modes and runs,
// the universal program has both slices and the linker's signature of the
// arm64 one, the dmg is made by hdiutil, and -sign with the ad-hoc identity
// "-" signs the bundle with the hardened runtime and the entitlements so
// that codesign verifies it. Notarization needs Apple credentials and is
// not run here.
func TestPackageMacTools(t *testing.T) {
	skipShort(t)
	for _, tool := range []string{"ditto", "codesign", "hdiutil", "lipo"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("%s, which every Mac has: %v", tool, err)
		}
	}
	dir := bundleModule(t, map[string]string{"notes/main.go": packageAppSource, "notes/icon.svg": packageIcon})
	t.Chdir(dir)
	sh := func(name string, args ...string) string {
		t.Helper()
		out, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	pack := func(out string, extra ...string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		args := append([]string{"package", "-os", "darwin", "-arch", "universal", "-format", "app,dmg", "-version", "1.0.0", "-o", out}, extra...)
		if code := run(append(args, "./notes"), &stdout, &stderr); code != 0 {
			t.Fatalf("tela package: code %d\n%s%s", code, stdout.String(), stderr.String())
		}
		t.Logf("%s%s", stdout.String(), stderr.String())
	}

	pack("dist")
	unpacked := filepath.Join(dir, "unpacked")
	sh("ditto", "-x", "-k", filepath.Join("dist", "notes-1.0.0-darwin-universal.zip"), unpacked)
	bundle := filepath.Join(unpacked, "Notes Demo.app")
	exe := filepath.Join(bundle, "Contents", "MacOS", "Notes Demo")
	if archs := strings.Fields(sh("lipo", "-archs", exe)); len(archs) != 2 || archs[0] != "x86_64" || archs[1] != "arm64" {
		t.Errorf("lipo -archs: %v", archs)
	}
	// The program runs: the kernel checks the signature of the slice it
	// runs (arm64 on the runner), its pages against the linker's code
	// directory, and nothing of the bundle.
	if out := sh(exe, "x"); !strings.Contains(out, "notes says hi") {
		t.Errorf("the unpacked program printed %q", out)
	}
	// The arm64 slice's signature is valid inside the universal file. It
	// is checked on a copy outside the bundle: codesign --verify of a
	// bundle's main executable verifies the bundle, whose resources
	// (Info.plist, the icon) the linker's signature does not seal, and
	// fails with "code has no resources but signature indicates they must
	// be present" (errSecCSResourcesNotFound) for any unsigned bundle,
	// thin or universal. Sealing them is -sign's work, checked below.
	loose := filepath.Join(dir, "loose-program")
	if b, err := os.ReadFile(exe); err != nil {
		t.Fatal(err)
	} else if err := os.WriteFile(loose, b, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("codesign", "--verify", "--verbose=4", "--arch", "arm64", loose).CombinedOutput(); err != nil {
		info, _ := exec.Command("codesign", "-d", "--verbose=4", "--arch", "arm64", loose).CombinedOutput()
		fat, _ := exec.Command("lipo", "-detailed_info", loose).CombinedOutput()
		t.Errorf("codesign --verify --arch arm64 %s (the universal program, copied out of the bundle): %v\n%s\ncodesign -d:\n%s\nlipo -detailed_info:\n%s", loose, err, out, info, fat)
	}
	sh("hdiutil", "imageinfo", filepath.Join("dist", "notes-1.0.0-darwin-universal.dmg"))

	pack("signed", "-sign", "-codesign-identity", "-")
	signed := filepath.Join(dir, "unpacked-signed")
	sh("ditto", "-x", "-k", filepath.Join("signed", "notes-1.0.0-darwin-universal.zip"), signed)
	bundle = filepath.Join(signed, "Notes Demo.app")
	sh("codesign", "--verify", "--deep", "--strict", bundle)
	if out := sh("codesign", "-dv", bundle); !strings.Contains(out, "runtime") || !strings.Contains(out, "Identifier=dev.example.Notes") {
		t.Errorf("codesign -dv: %s", out)
	}
	if out := sh("codesign", "-d", "--entitlements", "-", bundle); !strings.Contains(out, "com.apple.security.device.camera") {
		t.Errorf("the entitlements: %s", out)
	}
	sh("codesign", "--verify", filepath.Join("signed", "notes-1.0.0-darwin-universal.dmg"))
	if _, err := os.Stat(filepath.Join("signed", "notes.entitlements")); err != nil {
		t.Error(err)
	}
}
