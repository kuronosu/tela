package main

import (
	"bytes"
	"debug/buildinfo"
	"debug/pe"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/kuronosu/tela/cmd/tela/internal/winres"
)

func TestLinkerFlags(t *testing.T) {
	const x = "-X github.com/kuronosu/tela/internal/appver.version=1.2.3"
	for _, tc := range []struct {
		name    string
		opts    buildOptions
		goflags string
		gui     bool
		want    string
	}{
		{"nothing to add", buildOptions{}, "", false, ""},
		{"the flag untouched", buildOptions{Ldflags: "-s -w"}, "", false, "-s -w"},
		{"goflags untouched", buildOptions{}, "-ldflags=-s", false, ""},
		{"version", buildOptions{Version: "1.2.3"}, "", false, x},
		{"version after the user's", buildOptions{Version: "1.2.3", Ldflags: "-s -w"}, "", false, "-s -w " + x},
		{"gui", buildOptions{}, "", true, "-H=windowsgui"},
		{"both", buildOptions{Version: "1.2.3"}, "", true, x + " -H=windowsgui"},
		{"goflags ldflags kept", buildOptions{}, "-mod=mod '-ldflags=-s -w'", true, "-s -w -H=windowsgui"},
		{"the flag replaces goflags", buildOptions{Ldflags: "-w"}, "-ldflags=-s", true, "-w -H=windowsgui"},
		{"the user's -H wins", buildOptions{Ldflags: "-s -H=windows"}, "", true, "-s -H=windows"},
		{"the user's -H wins in goflags", buildOptions{Version: "1.2.3"}, "'-ldflags=-H windows'", true, "-H windows " + x},
		{"a pattern", buildOptions{Ldflags: "all=-s", Version: "1.2.3"}, "", false, "all=-s " + x},
	} {
		if got := linkerFlags(tc.opts, tc.goflags, tc.gui); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestParseBuildArgsDeclaration(t *testing.T) {
	got, err := parseBuildArgs([]string{"-version", "1.2.3", "-icon", "a.svg", "-console", "./x"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "1.2.3" || got.Icon != "a.svg" || !got.Console || got.Pkg != "./x" {
		t.Errorf("got %+v", got)
	}
	run, err := parseRunArgs([]string{"-version", "2.0.0", "."})
	if err != nil || run.Version != "2.0.0" {
		t.Errorf("run: %+v, %v", run, err)
	}
	if _, err := parseRunArgs([]string{"-console", "."}); err == nil {
		t.Error("tela run takes no -console")
	}
	if _, err := parseBuildArgs([]string{"-version", "1 2"}); err == nil || !strings.Contains(err.Error(), "-version") {
		t.Errorf("a version with a space: %v", err)
	}
}

const winApp = `package main

import "github.com/kuronosu/tela"

const company = "Example"

var App = tela.App{
	Title:       "Notes Demo",
	AppID:       "dev.example.notes",
	Publisher:   company + " Inc.",
	Description: "Take notes",
}

func main() { tela.Main(App) }
`

const winIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><rect width="100" height="100" rx="22" fill="#2563eb"/><circle cx="50" cy="50" r="28" fill="#fff"/></svg>`

// peResources reads the resources of a Windows executable.
func peResources(t *testing.T, path string) (*pe.File, []winres.Resource) {
	t.Helper()
	f, err := pe.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	for _, s := range f.Sections {
		if s.Name != ".rsrc" {
			continue
		}
		data, err := s.Data()
		if err != nil {
			t.Fatal(err)
		}
		res, err := winres.Read(data[:s.VirtualSize], s.VirtualAddress)
		if err != nil {
			t.Fatal(err)
		}
		return f, res
	}
	return f, nil
}

// versionStrings returns the strings of a VS_VERSIONINFO resource: its
// StringFileInfo is a run of "key\0value\0" in UTF-16, which is found by
// looking for the keys.
func versionStrings(data []byte, keys ...string) map[string]string {
	u := make([]uint16, len(data)/2)
	for i := range u {
		u[i] = binary.LittleEndian.Uint16(data[2*i:])
	}
	text := string(utf16.Decode(u))
	out := map[string]string{}
	for _, k := range keys {
		if i := strings.Index(text, k+"\x00"); i >= 0 {
			rest := strings.TrimLeft(text[i+len(k)+1:], "\x00")
			out[k] = rest[:strings.IndexByte(rest, 0)]
		}
	}
	return out
}

// TestBuildWindowsResources builds a program for windows from whichever OS
// this is and reads the resources back from the executable.
func TestBuildWindowsResources(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{
		"notes/main.go":     winApp,
		"notes/icon.svg":    winIcon,
		"nonconst/main.go":  "package main\n\nimport (\n\t\"os\"\n\n\t\"github.com/kuronosu/tela\"\n)\n\nvar App = tela.App{Title: \"X\", Publisher: os.Getenv(\"P\")}\n\nfunc main() { tela.Main(App) }\n",
		"smallicon/main.go": winApp,
	})
	small := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	small.Pix[3] = 255
	var buf bytes.Buffer
	if err := png.Encode(&buf, small); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "smallicon", "icon.png"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	for _, arch := range []string{"amd64", "arm64"} {
		setTarget(t, "windows", arch)
		out := filepath.Join(dir, "dist-"+arch)
		if err := os.Mkdir(out, 0o755); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		if code := run([]string{"build", "-o", out, "-version", "3.1.4-rc.1", "./notes"}, &stdout, &stderr); code != 0 {
			t.Fatalf("%s: tela build: code %d\n%s", arch, code, stderr.String())
		}
		exe := filepath.Join(out, "notes.exe")
		f, res := peResources(t, exe)
		if res == nil {
			t.Fatalf("%s: no .rsrc section in %s", arch, exe)
		}
		// GUI subsystem by default.
		var subsystem uint16
		switch oh := f.OptionalHeader.(type) {
		case *pe.OptionalHeader64:
			subsystem = oh.Subsystem
		case *pe.OptionalHeader32:
			subsystem = oh.Subsystem
		}
		if subsystem != 2 {
			t.Errorf("%s: subsystem %d, want 2 (Windows GUI)", arch, subsystem)
		}
		var icons, groups int
		var version []byte
		for _, r := range res {
			switch r.Type {
			case winres.RTIcon:
				icons++
				if !bytes.HasPrefix(r.Data, []byte("\x89PNG")) {
					t.Errorf("%s: icon %d is not a PNG", arch, r.Name)
				}
			case winres.RTGroupIcon:
				groups++
				if n := binary.LittleEndian.Uint16(r.Data[4:]); int(n) != icons && icons != 0 {
					t.Errorf("%s: the icon group lists %d icons, %d found", arch, n, icons)
				}
			case winres.RTVersion:
				version = r.Data
			}
		}
		if groups != 1 || icons != 7 {
			t.Errorf("%s: %d icon groups and %d icons, want 1 and 7", arch, groups, icons)
		}
		got := versionStrings(version, "CompanyName", "FileDescription", "FileVersion", "ProductName", "ProductVersion", "OriginalFilename")
		for k, want := range map[string]string{
			"CompanyName": "Example Inc.", "FileDescription": "Take notes", "FileVersion": "3.1.4-rc.1",
			"ProductName": "Notes Demo", "ProductVersion": "3.1.4-rc.1", "OriginalFilename": "notes.exe",
		} {
			if got[k] != want {
				t.Errorf("%s: %s = %q, want %q", arch, k, got[k], want)
			}
		}
		// -version is written into the program for internal/appver.
		info, err := buildinfo.ReadFile(exe)
		if err != nil {
			t.Fatal(err)
		}
		var ldflags string
		for _, s := range info.Settings {
			if s.Key == "-ldflags" {
				ldflags = s.Value
			}
		}
		if !strings.Contains(ldflags, "internal/appver.version=3.1.4-rc.1") || !strings.Contains(ldflags, "-H=windowsgui") {
			t.Errorf("%s: -ldflags %q", arch, ldflags)
		}
		// The user's folder has no new file.
		entries, _ := os.ReadDir(filepath.Join(dir, "notes"))
		for _, e := range entries {
			if e.Name() != "main.go" && e.Name() != "icon.svg" {
				t.Errorf("%s: tela build left %s in the package's folder", arch, e.Name())
			}
		}
	}

	setTarget(t, "windows", "amd64")
	// -console keeps the console subsystem; -icon and the flags replace the source.
	other := filepath.Join(dir, "other.svg")
	if err := os.WriteFile(other, []byte(strings.Replace(winIcon, "#2563eb", "#dc2626", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	exe := filepath.Join(dir, "console.exe")
	if code := run([]string{"build", "-console", "-icon", other, "-name", "Renamed", "-o", exe, "./notes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("tela build -console: code %d\n%s", code, stderr.String())
	}
	f, res := peResources(t, exe)
	if oh, ok := f.OptionalHeader.(*pe.OptionalHeader64); !ok || oh.Subsystem != 3 {
		t.Errorf("-console: subsystem %v, want 3 (Windows console)", f.OptionalHeader)
	}
	for _, r := range res {
		if r.Type == winres.RTVersion {
			got := versionStrings(r.Data, "ProductName", "FileVersion")
			// Built in a folder outside version control: no version known.
			if got["ProductName"] != "Renamed" || got["FileVersion"] != "0.0.0" {
				t.Errorf("-name: %v", got)
			}
		}
	}

	// A mistake is reported before anything is built.
	stderr.Reset()
	if code := run([]string{"build", "-o", filepath.Join(dir, "x.exe"), "./nonconst"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "Publisher of the tela.App is not a constant string") {
		t.Errorf("non-constant Publisher: code %d\n%s", code, stderr.String())
	}
	stderr.Reset()
	if code := run([]string{"build", "-o", filepath.Join(dir, "x.exe"), "./smallicon"}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "at least 512x512") {
		t.Errorf("small icon: code %d\n%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "x.exe")); err == nil {
		t.Error("a failed build wrote its output")
	}

	// Another target is not touched: no resources, no GUI flag.
	setTarget(t, "linux", "amd64")
	plain := filepath.Join(dir, "plain")
	stderr.Reset()
	if code := run([]string{"build", "-o", plain, "./nonconst"}, &stdout, &stderr); code != 0 {
		t.Fatalf("linux build: code %d\n%s", code, stderr.String())
	}
}
