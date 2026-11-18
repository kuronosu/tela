package main

import (
	"archive/zip"
	"bytes"
	"debug/macho"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/plist"
)

// The macOS formats, checked on any system (the checks that need a Mac are
// in package_darwin_test.go).

// zipEntries reads a zip file: its entries by name and their contents.
func zipEntries(t *testing.T, path string) (map[string]*zip.File, map[string][]byte) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	files, data := map[string]*zip.File{}, map[string][]byte{}
	for _, f := range zr.File {
		files[f.Name] = f
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	return files, data
}

func TestPackageDarwin(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{"notes/main.go": packageAppSource, "notes/icon.svg": packageIcon})
	t.Chdir(dir)
	t.Setenv("SOURCE_DATE_EPOCH", "1763337600")
	pack := func(out string, extra ...string) (string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		args := append([]string{"package", "-os", "darwin", "-arch", "amd64,arm64,universal", "-version", "1.2.0-rc.1", "-o", out}, extra...)
		if code := run(append(args, "./notes"), &stdout, &stderr); code != 0 {
			t.Fatalf("tela package: code %d\n%s%s", code, stdout.String(), stderr.String())
		}
		return stdout.String(), stderr.String()
	}
	out1, out2 := filepath.Join(dir, "dist1"), filepath.Join(dir, "dist2")
	stdout, _ := pack(out1)
	// Off a Mac, -sign says how else to sign darwin packages and writes
	// them unsigned: the same bytes.
	defer func(h string) { hostGOOS = h }(hostGOOS)
	hostGOOS = "linux"
	_, stderr := pack(out2, "-sign")
	if !strings.Contains(stderr, "codesign and notarytool are macOS's") || !strings.Contains(stderr, "rcodesign") {
		t.Errorf("-sign off a Mac says:\n%s", stderr)
	}
	if strings.Count(stdout, "notes.entitlements") != 1 || strings.Contains(stdout, "entitlements (unsigned)") || strings.Count(stdout, ".zip (unsigned)") != 3 {
		t.Errorf("stdout:\n%s", stdout)
	}
	t1, t2 := readTree(t, out1), readTree(t, out2)
	if len(t1) != 4 {
		t.Errorf("wrote %d files", len(t1))
	}
	for n, b := range t1 {
		if !bytes.Equal(b, t2[n]) {
			t.Errorf("%s differs between two runs", n)
		}
	}
	ent := string(t1["notes.entitlements"])
	if !strings.Contains(ent, "<key>com.apple.security.device.camera</key>") || strings.Contains(ent, "app-sandbox") {
		t.Errorf("entitlements:\n%s", ent)
	}

	exes := map[string][]byte{}
	for _, arch := range []string{"amd64", "arm64", "universal"} {
		name := "notes-1.2.0-rc.1-darwin-" + arch + ".zip"
		if goos, goarch, format, err := targetOfName(name, "1.2.0-rc.1"); err != nil || goos != "darwin" || goarch != arch || format != "app" {
			t.Errorf("tela update manifest reads %s as %s/%s/%s (%v)", name, goos, goarch, format, err)
		}
		files, data := zipEntries(t, filepath.Join(out1, name))
		const app = "Notes Demo.app/"
		exe := app + "Contents/MacOS/Notes Demo"
		for n, mode := range map[string]fs.FileMode{
			app: fs.ModeDir | 0o755, app + "Contents/": fs.ModeDir | 0o755, app + "Contents/Info.plist": 0o644,
			exe: 0o755, app + "Contents/Resources/notes.icns": 0o644,
		} {
			if f := files[n]; f == nil || f.Mode() != mode || f.CreatorVersion>>8 != 3 {
				t.Errorf("%s: %s: %v", name, n, f)
			}
		}
		for n := range files {
			if !strings.HasPrefix(n, app) {
				t.Errorf("%s: %s is not in the bundle's folder", name, n)
			}
		}
		if !bytes.HasPrefix(data[app+"Contents/Resources/notes.icns"], []byte("icns")) {
			t.Errorf("%s: the icon is not an icns file", name)
		}
		info, err := plist.UnmarshalDict(data[app+"Contents/Info.plist"])
		if err != nil {
			t.Fatal(err)
		}
		for k, want := range map[string]any{
			"CFBundleIdentifier": "dev.example.Notes", "CFBundleExecutable": "Notes Demo", "CFBundleIconFile": "notes.icns",
			"CFBundleShortVersionString": "1.2.0", "LSApplicationCategoryType": "public.app-category.productivity",
			"NSCameraUsageDescription": "Take pictures", "NSHumanReadableCopyright": "© Example Ltd",
		} {
			if info[k] != want {
				t.Errorf("%s: %s = %v, want %v", name, k, info[k], want)
			}
		}
		if urls, _ := info["CFBundleURLTypes"].(plist.Array); len(urls) != 1 {
			t.Errorf("%s: CFBundleURLTypes %v", name, info["CFBundleURLTypes"])
		}
		if docs, _ := info["CFBundleDocumentTypes"].(plist.Array); len(docs) != 2 || docs[1].(plist.Dict)["CFBundleTypeRole"] != "None" {
			t.Errorf("%s: CFBundleDocumentTypes %v", name, info["CFBundleDocumentTypes"])
		}
		exes[arch] = data[exe]
	}

	// The universal program: both thin programs, byte for byte, so the
	// linker's signature of the arm64 one is intact.
	fat, err := macho.NewFatFile(bytes.NewReader(exes["universal"]))
	if err != nil {
		t.Fatal(err)
	}
	if len(fat.Arches) != 2 {
		t.Fatalf("%d slices", len(fat.Arches))
	}
	for _, a := range fat.Arches {
		arch := map[macho.Cpu]string{macho.CpuAmd64: "amd64", macho.CpuArm64: "arm64"}[a.Cpu]
		slice := exes["universal"][a.Offset : a.Offset+a.Size]
		if arch == "" || !bytes.Equal(slice, exes[arch]) {
			t.Errorf("the %v slice is not the thin %s program", a.Cpu, arch)
		}
	}
	arm, err := macho.NewFile(bytes.NewReader(exes["arm64"]))
	if err != nil {
		t.Fatal(err)
	}
	signed := false
	for _, l := range arm.Loads {
		if raw := l.Raw(); len(raw) >= 16 && arm.ByteOrder.Uint32(raw) == 0x1d { // LC_CODE_SIGNATURE
			off, size := arm.ByteOrder.Uint32(raw[8:]), arm.ByteOrder.Uint32(raw[12:])
			signed = int(off+size) <= len(exes["arm64"])
		}
	}
	if !signed {
		t.Errorf("the arm64 program has no code signature in its file")
	}

	// Mistakes, before anything is built.
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"-os", "linux", "-arch", "universal", "./notes"}, 1, "universal is for darwin"},
		{[]string{"-os", "darwin", "-format", "dmg", "./notes"}, 1, "needs a Mac"},
		{[]string{"-os", "darwin", "-arch", "386", "./notes"}, 1, "amd64, arm64 or universal"},
		{[]string{"-os", "darwin", "-codesign-identity", "x", "./notes"}, 2, "add -sign"},
	} {
		if runtime.GOOS == "darwin" && strings.Contains(tc.want, "Mac") {
			continue
		}
		var so, se bytes.Buffer
		empty := filepath.Join(t.TempDir(), "out")
		if code := run(append([]string{"package", "-o", empty}, tc.args...), &so, &se); code != tc.code || !strings.Contains(se.String(), tc.want) {
			t.Errorf("%v: code %d, stderr %q, want %q", tc.args, code, se.String(), tc.want)
		}
		if _, err := os.Stat(empty); err == nil {
			t.Errorf("%v wrote %s", tc.args, empty)
		}
	}
}

// updaterSource is a program that updates itself as an app install: from
// the manifest at $NOTES_URL, signed by $NOTES_KEY.
const updaterSource = `package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/update"
)

var App = tela.App{Title: "Notes Demo", AppID: "dev.example.Notes", Schemes: []string{"notes"}}

func main() {
	desktop.SetAppID(App.AppID)
	ctx := context.Background()
	u, err := update.Check(ctx, update.Config{URL: os.Getenv("NOTES_URL"), PublicKeys: []update.PublicKey{update.PublicKey(os.Getenv("NOTES_KEY"))}, Format: "app"})
	if err == nil {
		err = u.Download(ctx, nil)
	}
	if err == nil {
		err = u.Apply()
	}
	if err != nil {
		fmt.Println("update:", err)
		os.Exit(1)
	}
	fmt.Println("applied", u.Version)
}
`

// TestPackageAppUpdates: the app format's zip is what package update
// swaps in for an installed bundle. The installed program is this
// system's (the bundle's layout is all update looks at), the new version
// the universal darwin package; the manifest serves it for this system.
func TestPackageAppUpdates(t *testing.T) {
	skipShort(t)
	if runtime.GOOS == "windows" {
		t.Skip("bundles keep executable bits and links, which Windows folders do not")
	}
	dir := bundleModule(t, map[string]string{"notes/main.go": updaterSource, "notes/icon.svg": packageIcon})
	t.Chdir(dir)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"package", "-os", "darwin", "-arch", "universal", "-version", "1.1.0", "-o", "dist", "./notes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela package: %d\n%s", code, stderr.String())
	}
	// The installed version 1.0.0.
	bundle := filepath.Join(dir, "Applications", "Notes Demo.app")
	installed := filepath.Join(bundle, "Contents", "MacOS", "Notes Demo")
	if code := run([]string{"build", "-version", "1.0.0", "-o", filepath.Join(dir, "prog"), "./notes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela build: %d\n%s", code, stderr.String())
	}
	if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "prog"), installed); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(bundle, "Contents", "Info.plist"), "the old Info.plist")

	key := keygen(t, dir, "k.key")
	zipName := "notes-1.1.0-darwin-universal.zip"
	if code, out := telaIn(t, dir, "update", "manifest", "-m", "dist/manifest.json", "-app-id", "dev.example.Notes", "-version", "1.1.0", "-key", "k.key",
		"dist/"+zipName, "dist/"+zipName+"="+runtime.GOOS+"/"+runtime.GOARCH+"/app"); code != 0 {
		t.Fatalf("manifest: %d %s", code, out)
	}
	m := readManifest(t, filepath.Join(dir, "dist", "manifest.json"))
	targets := map[string]bool{}
	for _, a := range m.Channels["stable"][0].Artifacts {
		targets[a.OS+"/"+a.Arch+"/"+a.Format] = a.URL == zipName
	}
	if !targets["darwin/amd64/app"] || !targets["darwin/arm64/app"] || targets["darwin/universal/app"] {
		t.Errorf("the universal zip's artifacts: %v", targets)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(dir, "dist"))))
	defer srv.Close()

	cmd := exec.Command(installed)
	cmd.Env = append(os.Environ(), "NOTES_URL="+srv.URL+"/manifest.json", "NOTES_KEY="+string(key), "TELA_DESKTOP_DIR="+filepath.Join(dir, "state"))
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "applied 1.1.0") {
		t.Fatalf("the update: %v\n%s", err, out)
	}
	exe, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := macho.NewFatFile(bytes.NewReader(exe)); err != nil {
		t.Errorf("the installed program is not the universal one: %v", err)
	}
	if st, _ := os.Stat(installed); st.Mode().Perm()&0o100 == 0 {
		t.Errorf("the installed program lost its executable bit: %v", st.Mode())
	}
	raw, _ := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if info, err := plist.UnmarshalDict(raw); err != nil || info["CFBundleShortVersionString"] != "1.1.0" {
		t.Errorf("the installed Info.plist: %v %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(bundle, "Contents", "Resources", "notes.icns")); err != nil {
		t.Errorf("the icon: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "Applications", ".Notes Demo.app.update", "old", "Contents", "Info.plist")); string(b) != "the old Info.plist" {
		t.Errorf("the old bundle is not kept: %q", b)
	}
}
