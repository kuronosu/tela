package main

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/pkgfmt"
	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/update"
)

// updaterSource is a program that updates itself with package update when
// started as "notes update <manifest URL> <public key>", and otherwise
// prints its version.
const updaterSource = `package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kuronosu/tela/desktop"
	"github.com/kuronosu/tela/update"
)

var version = "none"

func main() {
	if len(os.Args) == 4 && os.Args[1] == "update" {
		desktop.SetAppID("dev.example.Notes")
		ctx := context.Background()
		u, err := update.Check(ctx, update.Config{URL: os.Args[2], PublicKeys: []update.PublicKey{update.PublicKey(os.Args[3])}})
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
		fmt.Println("applied", u.From, "->", u.Version)
		return
	}
	fmt.Println("notes", version)
}
`

// extractZip unpacks a zip into dir, keeping the modes, as a user
// unpacking the download would.
func extractZipTo(t *testing.T, file, dir string) {
	t.Helper()
	zr, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		p := filepath.Join(dir, filepath.FromSlash(f.Name))
		if f.Mode().IsDir() {
			os.MkdirAll(p, 0o755)
			continue
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, b, f.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
}

// TestZipUpdateRoundTrip packages versions 1 and 2 of a program as the
// Windows zip, installs version 1 by extracting its zip, and has that
// program update itself with package update from a signed manifest that
// offers version 2's zip: update finds the install by the zip's marker and
// swaps the folder's entries. The program is built for this machine, so
// it runs here; the zip's layout is the Windows one.
func TestZipUpdateRoundTrip(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{"notes/main.go": updaterSource})
	zipf, _ := pkgfmt.Lookup("zip")
	zips := map[string][]byte{}
	for _, v := range []string{"1.0.0", "2.0.0"} {
		exe := filepath.Join(dir, "build-"+v)
		cmd := exec.Command("go", "build", "-o", exe, "-ldflags", "-X main.version="+v+" -X "+appverVersion+"="+v, "./notes")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go build: %v\n%s", err, out)
		}
		app := pkgfmt.App{ID: "dev.example.Notes", Name: "Notes", Package: "notes", Version: v}
		outs, err := zipf.Write(pkgfmt.Job{App: app, Target: pkgfmt.Target{OS: "windows", Arch: "amd64"}, Program: exe,
			Time: time.Unix(1763337600, 0), OutDir: filepath.Join(dir, "dist"), Work: t.TempDir(), Log: io.Discard})
		if err != nil {
			t.Fatal(err)
		}
		if zips[v], err = os.ReadFile(outs[0].Path); err != nil {
			t.Fatal(err)
		}
	}

	// Install version 1, with a file of the user's in the folder.
	install := filepath.Join(t.TempDir(), "Programs")
	extractZipTo(t, filepath.Join(dir, "dist", "notes-1.0.0-windows-amd64.zip"), install)
	folder := filepath.Join(install, "notes")
	os.WriteFile(filepath.Join(folder, "notes.ini"), []byte("mine"), 0o644)
	exe := filepath.Join(folder, "notes.exe")
	runExe := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(exe, args...)
		cmd.Env = append(os.Environ(), desktop.DirEnv+"="+filepath.Join(dir, "desktop"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", exe, args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if got := runExe(); got != "notes 1.0.0" {
		t.Fatalf("the installed program says %q", got)
	}

	// The update server: version 2's zip and a manifest signed for it.
	_, key, _ := ed25519.GenerateKey(nil)
	sum := sha256.Sum256(zips["2.0.0"])
	now := time.Now().UTC()
	m := update.Manifest{Schema: 1, AppID: "dev.example.Notes", Published: now, Expires: now.Add(24 * time.Hour),
		Channels: map[string][]update.Release{"stable": {{Version: "2.0.0", Date: now, Artifacts: []update.Artifact{{
			OS: runtime.GOOS, Arch: runtime.GOARCH, Format: "zip", URL: "notes-2.0.0-windows-amd64.zip",
			Size: int64(len(zips["2.0.0"])), SHA256: hex.EncodeToString(sum[:]),
		}}}}}}
	mb, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := update.Sign(mb, key)
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string][]byte{"/manifest.json": mb, "/manifest.json.sig": sig, "/notes-2.0.0-windows-amd64.zip": zips["2.0.0"]}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, ok := docs[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b))
	}))
	defer srv.Close()
	pub := update.NewPublicKey(key.Public().(ed25519.PublicKey))

	if got := runExe("update", srv.URL+"/manifest.json", string(pub)); got != "applied 1.0.0 -> 2.0.0" {
		t.Fatalf("the update: %q", got)
	}
	if got := runExe(); got != "notes 2.0.0" {
		t.Errorf("after the update the program says %q", got)
	}
	var mk update.Marker
	if err := json.Unmarshal([]byte(readFileString(t, filepath.Join(folder, update.MarkerName))), &mk); err != nil || mk.Version != "2.0.0" || mk.Exe != "notes.exe" {
		t.Errorf("the marker after the update: %+v %v", mk, err)
	}
	if readFileString(t, filepath.Join(folder, "notes.ini")) != "mine" {
		t.Errorf("the user's file was not kept")
	}
	// The old copy is kept for a rollback.
	old := filepath.Join(folder, ".update", "old")
	if b, err := os.ReadFile(filepath.Join(old, "notes.exe")); err != nil || !bytes.Contains(b, []byte("1.0.0")) {
		t.Errorf("the old copy: %v", err)
	}
	if _, err := os.Stat(filepath.Join(old, "notes.ini")); err == nil {
		t.Errorf("the user's file was moved to the old copy")
	}
}

func readFileString(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestPackageWindows packages a declared app for Windows on both
// architectures, twice, and reads the outputs back.
func TestPackageWindows(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{
		"notes/main.go":  packageAppSource,
		"notes/icon.svg": packageIcon,
	})
	t.Chdir(dir)
	t.Setenv("SOURCE_DATE_EPOCH", "1763337600")
	pack := func(out string, extra ...string) (string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		args := append([]string{"package", "-os", "windows", "-arch", "amd64,arm64", "-version", "1.2.0", "-o", out}, extra...)
		if code := run(append(args, "./notes"), &stdout, &stderr); code != 0 {
			t.Fatalf("tela package: code %d\n%s%s", code, stdout.String(), stderr.String())
		}
		return stdout.String(), stderr.String()
	}
	out1, out2 := filepath.Join(dir, "dist1"), filepath.Join(dir, "dist2")
	stdout, stderr := pack(out1, "-appinstaller-url", "https://example.com/notes", "-sign")
	if runtime.GOOS != "windows" && !strings.Contains(stderr, "signtool is not available") {
		t.Errorf("-sign off Windows says nothing:\n%s", stderr)
	}
	pack(out2, "-appinstaller-url", "https://example.com/notes")
	t1, t2 := readTree(t, out1), readTree(t, out2)
	for _, arch := range []string{"amd64", "arm64"} {
		for _, n := range []string{"notes-1.2.0-windows-" + arch + ".zip", "notes-1.2.0-windows-" + arch + ".msix", "notes-windows-" + arch + ".appinstaller"} {
			if t1[n] == nil {
				t.Errorf("no %s:\n%s", n, stdout)
				continue
			}
			if !bytes.Equal(t1[n], t2[n]) {
				t.Errorf("%s differs between two runs", n)
			}
		}
		goos, goarch, format, err := targetOfName("notes-1.2.0-windows-"+arch+".msix", "1.2.0")
		if err != nil || goos != "windows" || goarch != arch || format != "msix" {
			t.Errorf("tela update manifest reads the msix as %s/%s/%s (%v)", goos, goarch, format, err)
		}
	}
	if !strings.Contains(stdout, "-AllowUnsigned") {
		t.Errorf("the output does not say how to install the unsigned MSIX:\n%s", stdout)
	}

	// The MSIX's program is the zip's, with its resources.
	read := func(b []byte, name string) []byte {
		t.Helper()
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			t.Fatal(err)
		}
		r, err := zr.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := io.ReadAll(r)
		return c
	}
	zexe := read(t1["notes-1.2.0-windows-amd64.zip"], "notes/notes.exe")
	if !bytes.Equal(zexe, read(t1["notes-1.2.0-windows-amd64.msix"], "notes.exe")) || !bytes.HasPrefix(zexe, []byte("MZ")) {
		t.Errorf("the zip's and the MSIX's programs differ")
	}
	man := string(read(t1["notes-1.2.0-windows-arm64.msix"], "AppxManifest.xml"))
	for _, s := range []string{`Name="dev.example.Notes" Version="1.2.0.0" Publisher="CN=Example Ltd, OID.`, `ProcessorArchitecture="arm64"`,
		`<uap:Protocol Name="notes"/>`, `<uap:FileType ContentType="application/x-notes-note">.note</uap:FileType>`,
		`<Capability Name="internetClient"/>`, `<DeviceCapability Name="webcam"/>`, `<rescap:Capability Name="broadFileSystemAccess"/>`} {
		if !strings.Contains(man, s) {
			t.Errorf("the manifest lacks %s:\n%s", s, man)
		}
	}
	if strings.Contains(man, "x-notes-backup") {
		t.Errorf("a type of role HandlerNone is associated:\n%s", man)
	}

	// Mistakes of the Windows flags.
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-msix-publisher", "Example"}, "-msix-publisher"},
		{[]string{"-appinstaller-url", "example.com/notes"}, "-appinstaller-url"},
		{[]string{"-sign", "-cert-file", "a.pfx", "-cert-sha1", "AB"}, "give one"},
		{[]string{"-cert-sha1", "AB"}, "add -sign"},
		{[]string{"-sign", "-cert-password-file", "p"}, "-cert-file, which is missing"},
	} {
		var so, se bytes.Buffer
		code := run(append(append([]string{"package", "-os", "windows"}, tc.args...), "./notes"), &so, &se)
		if code != 2 || !strings.Contains(se.String(), tc.want) {
			t.Errorf("%v: code %d, %q, want %q", tc.args, code, se.String(), tc.want)
		}
	}
}
