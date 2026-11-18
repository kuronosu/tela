package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/update"
)

// tela runs the command line in dir and returns its exit status and
// output.
func telaIn(t *testing.T, dir string, args ...string) (int, string) {
	t.Helper()
	t.Chdir(dir)
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String() + errb.String()
}

var publicKeyRE = regexp.MustCompile(`"(ed25519:[A-Za-z0-9+/=]+)"`)

func keygen(t *testing.T, dir, name string) update.PublicKey {
	t.Helper()
	code, out := telaIn(t, dir, "update", "keygen", "-o", name)
	if code != 0 {
		t.Fatalf("keygen: %d %s", code, out)
	}
	m := publicKeyRE.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("keygen printed no public key:\n%s", out)
	}
	k := update.PublicKey(m[1])
	if _, err := k.Key(); err != nil || !strings.Contains(out, "key ID "+k.ID()) {
		t.Fatalf("keygen printed %q (%v):\n%s", k, err, out)
	}
	return k
}

func TestUpdateKeygen(t *testing.T) {
	dir := t.TempDir()
	keygen(t, dir, "a.key")
	if st, err := os.Stat(filepath.Join(dir, "a.key")); err != nil || runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("key file %v, %v; want mode 0600", st.Mode(), err)
	}
	if code, out := telaIn(t, dir, "update", "keygen", "-o", "a.key"); code != 1 || !strings.Contains(out, "exists") {
		t.Errorf("overwriting a key: %d %s", code, out)
	}
}

func TestUpdateManifestAndSign(t *testing.T) {
	dir := t.TempDir()
	pub := keygen(t, dir, "old.key")
	pub2 := keygen(t, dir, "new.key")
	writeFile(t, filepath.Join(dir, "dist", "notes-1.2.0-linux-amd64.AppImage"), "appimage")
	writeFile(t, filepath.Join(dir, "dist", "notes-1.2.0-windows-arm64.zip"), "zip")
	writeFile(t, filepath.Join(dir, "dist", "notes-1.2.0-darwin-arm64.zip"), "bundle")
	writeFile(t, filepath.Join(dir, "dist", "raw.bin"), "raw")

	code, out := telaIn(t, dir, "update", "manifest", "-m", "dist/manifest.json", "-app-id", "com.example.notes",
		"-version", "v1.2.0", "-rollout", "0.25", "-activation", "app", "-notes", "Faster.", "-expires", "30d",
		"dist/notes-1.2.0-linux-amd64.AppImage", "dist/notes-1.2.0-windows-arm64.zip")
	if code != 0 {
		t.Fatalf("manifest: %d %s", code, out)
	}
	m := readManifest(t, filepath.Join(dir, "dist", "manifest.json"))
	r := m.Channels["stable"][0]
	if m.AppID != "com.example.notes" || r.Version != "1.2.0" || *r.Rollout != 0.25 || r.Activation != update.ActivateByApp || r.Notes != "Faster." || len(r.Artifacts) != 2 {
		t.Fatalf("manifest %+v, release %+v", m, r)
	}
	if d := m.Expires.Sub(m.Published); d != 30*24*time.Hour {
		t.Errorf("valid for %v, want 30 days", d)
	}
	a := r.Artifacts[1]
	if a.OS != "windows" || a.Arch != "arm64" || a.Format != "zip" || a.Size != 3 || a.URL != "notes-1.2.0-windows-arm64.zip" ||
		a.SHA256 != "4a70fe9aa6436e02c2dea340fbd1e352e4ef2d8ce6ca52ad25d4b95471fc8bf2" {
		t.Errorf("artifact %+v", a)
	}

	// A second run adds files and keeps the release's other fields.
	code, out = telaIn(t, dir, "update", "manifest", "-m", "dist/manifest.json", "-version", "1.2.0", "-base-url", "https://cdn.example.com/notes/",
		"dist/notes-1.2.0-darwin-arm64.zip", "dist/raw.bin=linux/riscv64/appimage", "dist/notes-1.2.0-linux-amd64.AppImage")
	if code != 0 {
		t.Fatalf("second manifest run: %d %s", code, out)
	}
	m = readManifest(t, filepath.Join(dir, "dist", "manifest.json"))
	r = m.Channels["stable"][0]
	if len(m.Channels["stable"]) != 1 || *r.Rollout != 0.25 || r.Notes != "Faster." || len(r.Artifacts) != 4 {
		t.Fatalf("after the second run: %+v", r)
	}
	formats := map[string]string{}
	for _, a := range r.Artifacts {
		formats[a.OS+"/"+a.Arch] = a.Format + " " + a.URL
	}
	if formats["darwin/arm64"] != "app https://cdn.example.com/notes/notes-1.2.0-darwin-arm64.zip" || formats["linux/riscv64"] != "appimage https://cdn.example.com/notes/raw.bin" ||
		formats["linux/amd64"] != "appimage https://cdn.example.com/notes/notes-1.2.0-linux-amd64.AppImage" {
		t.Errorf("artifacts %v", formats)
	}
	if !strings.Contains(out, "Sign it") {
		t.Errorf("no hint to sign:\n%s", out)
	}

	// Signed by both keys, either verifies.
	code, out = telaIn(t, dir, "update", "sign", "-key", "old.key", "-key", "new.key", "dist/manifest.json")
	if code != 0 {
		t.Fatalf("sign: %d %s", code, out)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "dist", "manifest.json"))
	sig, _ := os.ReadFile(filepath.Join(dir, "dist", "manifest.json.sig"))
	for _, k := range []update.PublicKey{pub, pub2} {
		if err := update.Verify(data, sig, []update.PublicKey{k}); err != nil {
			t.Errorf("key %s: %v", k.ID(), err)
		}
	}

	// Extending without a release refreshes the times only.
	before := m.Published
	code, out = telaIn(t, dir, "update", "manifest", "-m", "dist/manifest.json", "-key", "new.key")
	if code != 0 || !strings.Contains(out, "no longer matches") && !strings.Contains(out, "Signed") {
		t.Fatalf("extend: %d %s", code, out)
	}
	m = readManifest(t, filepath.Join(dir, "dist", "manifest.json"))
	if m.Published.Before(before) || len(m.Channels["stable"][0].Artifacts) != 4 {
		t.Errorf("extend: published %v (was %v)", m.Published, before)
	}
	data, _ = os.ReadFile(filepath.Join(dir, "dist", "manifest.json"))
	sig, _ = os.ReadFile(filepath.Join(dir, "dist", "manifest.json.sig"))
	if err := update.Verify(data, sig, []update.PublicKey{pub2}); err != nil {
		t.Errorf("-key on manifest did not sign it: %v", err)
	}
}

// TestUpdateManifestFolder: the files may be a whole folder of tela
// package's outputs; what is not a package of the release is skipped and
// said.
func TestUpdateManifestFolder(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"notes-1.2.0-linux-amd64.AppImage", "notes-1.1.0-linux-amd64.AppImage", "notes.entitlements",
		"notes-windows-amd64.appinstaller", "notes-1.2.0-linux-amd64.AppImage.asc", "notes-1.2.0-linux-amd64-flatpak/files/x", "manifest.json.sig"} {
		writeFile(t, filepath.Join(dir, "dist", filepath.FromSlash(n)), "x")
	}
	t.Chdir(dir)
	files, _ := filepath.Glob(filepath.Join("dist", "*"))
	code, out := telaIn(t, dir, append([]string{"update", "manifest", "-m", "dist/manifest.json", "-app-id", "a.b", "-version", "1.2.0"}, files...)...)
	if code != 0 {
		t.Fatalf("manifest: %d %s", code, out)
	}
	for _, s := range []string{
		"notes-1.1.0-linux-amd64.AppImage: not version 1.2.0", "notes.entitlements: not a package", "notes-windows-amd64.appinstaller: not a package",
		"notes-1.2.0-linux-amd64.AppImage.asc: not a package", "notes-1.2.0-linux-amd64-flatpak: a folder", "manifest.json.sig: not a package", "with 1 files",
	} {
		if !strings.Contains(out, s) {
			t.Errorf("the output lacks %q:\n%s", s, out)
		}
	}
	if r := readManifest(t, filepath.Join(dir, "dist", "manifest.json")).Channels["stable"][0]; len(r.Artifacts) != 1 || r.Artifacts[0].URL != "notes-1.2.0-linux-amd64.AppImage" {
		t.Errorf("the artifacts: %+v", r.Artifacts)
	}
}

func TestUpdateUsageErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes-1.2.0-plan9-amd64.zip"), "x")
	writeFile(t, filepath.Join(dir, "notes-1.3.0-linux-amd64.AppImage"), "x")
	writeFile(t, filepath.Join(dir, "release.zip"), "x")
	for _, args := range [][]string{
		{"update"},
		{"update", "frobnicate"},
		{"update", "manifest", "-version", "1.2.0"},                                                       // no app ID
		{"update", "manifest", "-app-id", "a.b", "-version", "1.2"},                                       // not semver
		{"update", "manifest", "-app-id", "a.b", "-version", "1.2.0", "-rollout", "2"},                    // rollout
		{"update", "manifest", "-app-id", "a.b", "-version", "1.2.0", "-activation", "later"},             // activation
		{"update", "manifest", "-app-id", "a.b", "-version", "1.2.0", "notes-1.2.0-plan9-amd64.zip"},      // OS
		{"update", "manifest", "-app-id", "a.b", "-version", "1.2.0", "notes-1.3.0-linux-amd64.AppImage"}, // version
		{"update", "manifest", "-app-id", "a.b", "-version", "1.2.0", "release.zip"},                      // no target in the name
		{"update", "manifest", "-app-id", "a.b", "-expires", "soon"},
		{"update", "sign", "manifest.json"}, // no key
	} {
		if code, out := telaIn(t, dir, args...); code != 2 {
			t.Errorf("%v: exit %d, want 2:\n%s", args, code, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "manifest.json")); err == nil {
		t.Errorf("a failed run wrote the manifest")
	}
	for _, topic := range []string{"keygen", "sign", "manifest"} {
		if code, out := telaIn(t, dir, "help", "update", topic); code != 0 || !strings.Contains(out, "tela update "+topic) {
			t.Errorf("help update %s: %d %s", topic, code, out)
		}
	}
}

// sign refuses manifests that a client would refuse.
func TestUpdateSignChecks(t *testing.T) {
	dir := t.TempDir()
	keygen(t, dir, "k.key")
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	for name, doc := range map[string]string{
		"expired":   `{"schema":1,"appID":"a.b","published":"` + past + `","expires":"` + past + `","channels":{}}`,
		"schema 2":  `{"schema":2,"appID":"a.b","published":"` + past + `","expires":"2999-01-01T00:00:00Z"}`,
		"no app ID": `{"schema":1,"published":"` + past + `","expires":"2999-01-01T00:00:00Z"}`,
		"not JSON":  `schema: 1`,
	} {
		writeFile(t, filepath.Join(dir, "m.json"), doc)
		if code, out := telaIn(t, dir, "update", "sign", "-key", "k.key", "m.json"); code != 1 {
			t.Errorf("%s: exit %d:\n%s", name, code, out)
		}
	}
}

func readManifest(t *testing.T, path string) *update.Manifest {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m update.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
