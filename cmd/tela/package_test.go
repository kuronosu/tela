package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestParsePackageArgs(t *testing.T) {
	o, err := parsePackageArgs([]string{"-os", "linux,windows", "-arch", "amd64, arm64", "-format", "deb", "-o", "out", "-version", "1.2.3", "-sign-cmd", "gpg --detach-sign '--local-user=a b'", "-sign", "-v", "./app"})
	if err != nil {
		t.Fatal(err)
	}
	want := packageOptions{
		build:   buildOptions{Pkg: "./app", Version: "1.2.3", Verbose: true},
		formats: []string{"deb"}, oses: []string{"linux", "windows"}, arches: []string{"amd64", "arm64"},
		out: "out", sign: true, signCmd: []string{"gpg", "--detach-sign", "--local-user=a b"},
	}
	if !reflect.DeepEqual(o, want) {
		t.Errorf("got %+v\nwant %+v", o, want)
	}
	if o, _ := parsePackageArgs(nil); o.out != "dist" || o.build.Pkg != "." {
		t.Errorf("defaults: %+v", o)
	}
	for _, args := range [][]string{{"a", "b"}, {"-o", ""}, {"-sign-cmd", " "}, {"-version", "1 2"}} {
		if _, err := parsePackageArgs(args); err == nil {
			t.Errorf("%q: no error", args)
		}
	}
	if _, err := parsePackageArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v", err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"help", "package"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "tela package") {
		t.Errorf("help package: %d %q", code, out.String())
	}
	out.Reset()
	run([]string{"help"}, &out, &errOut)
	if !strings.Contains(out.String(), "\tpackage ") {
		t.Errorf("the command list lacks package")
	}
}

func TestPackageTime(t *testing.T) {
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	got, from, err := packageTime("/nonexistent")
	if err != nil || got.Unix() != 1700000000 || from != "SOURCE_DATE_EPOCH" {
		t.Errorf("%v %q %v", got, from, err)
	}
	t.Setenv("SOURCE_DATE_EPOCH", "soon")
	if _, _, err := packageTime("/nonexistent"); err == nil {
		t.Errorf("a bad SOURCE_DATE_EPOCH was accepted")
	}
	os.Unsetenv("SOURCE_DATE_EPOCH")
	if got, _, _ := packageTime("/nonexistent"); !got.Equal(fixedEpoch) {
		t.Errorf("no time: %v", got)
	}
}

const packageIcon = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect x="1" y="1" width="14" height="14" rx="3" fill="#336699"/></svg>`

// packageApp is the fixture's program: a declared tela.App, and a main
// that prints and exits rather than opening a window.
const packageAppSource = `package main

import (
	"fmt"
	"os"

	"github.com/kuronosu/tela"
)

var App = tela.App{
	Title:       "Notes Demo",
	AppID:       "dev.example.Notes",
	Publisher:   "Example Ltd <dev@example.com>",
	Description: "Takes notes.",
	Category:    "Office",
	Schemes:     []string{"notes"},
	FileTypes: []tela.FileType{
		{Ext: ".note", MIME: "application/x-notes-note", Description: "Notes document"},
		{Ext: ".nbak", MIME: "application/x-notes-backup", Description: "Notes backup", Role: tela.HandlerNone},
	},
	Services: tela.Services{Network: true, Notifications: true, Files: true, Camera: "Take pictures"},
}

func main() {
	fmt.Println("notes says hi", os.Getenv("APPIMAGE") != "", os.Args[1:])
}
`

// readDeb returns the members of a deb and the files of its two tars.
func readDeb(t *testing.T, path string) (members []string, control, data map[string]*tar.Header, contents map[string][]byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("!<arch>\n")) {
		t.Fatalf("%s is not an ar archive", path)
	}
	b = b[8:]
	contents = map[string][]byte{}
	readTar := func(gz []byte, prefix string) map[string]*tar.Header {
		zr, err := gzip.NewReader(bytes.NewReader(gz))
		if err != nil {
			t.Fatal(err)
		}
		tr := tar.NewReader(zr)
		out := map[string]*tar.Header{}
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			out[h.Name] = h
			c, _ := io.ReadAll(tr)
			contents[prefix+h.Name] = c
		}
		return out
	}
	for len(b) >= 60 {
		name := strings.TrimSpace(string(b[:16]))
		size, err := strconv.Atoi(strings.TrimSpace(string(b[48:58])))
		if err != nil || string(b[58:60]) != "`\n" {
			t.Fatalf("bad ar header %q", b[:60])
		}
		body := b[60 : 60+size]
		members = append(members, name)
		switch name {
		case "debian-binary":
			if string(body) != "2.0\n" {
				t.Errorf("debian-binary %q", body)
			}
		case "control.tar.gz":
			control = readTar(body, "control:")
		case "data.tar.gz":
			data = readTar(body, "")
		}
		b = b[60+size+size%2:]
	}
	return members, control, data, contents
}

// readTree returns every file under dir, by relative path with slashes.
func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(dir, p)
		if d.Type().IsRegular() {
			b, _ := os.ReadFile(p)
			out[filepath.ToSlash(rel)] = b // the same names on every system
		}
		return nil
	})
	return out
}

func TestPackageLinux(t *testing.T) {
	skipShort(t)
	dir := bundleModule(t, map[string]string{
		"notes/main.go":  packageAppSource,
		"notes/icon.svg": packageIcon,
		"noid/main.go":   "package main\n\nimport \"github.com/kuronosu/tela\"\n\nvar App = tela.App{Title: \"No ID\"}\n\nfunc main() {}\n",
		"badscheme/main.go": "package main\n\nimport \"github.com/kuronosu/tela\"\n\n" +
			"var App = tela.App{Title: \"S\", AppID: \"dev.example.s\", Schemes: []string{\"https\"}}\n\nfunc main() {}\n",
		"badcat/main.go": "package main\n\nimport \"github.com/kuronosu/tela\"\n\n" +
			"var App = tela.App{Title: \"C\", AppID: \"dev.example.c\", Category: \"office\"}\n\nfunc main() {}\n",
		"shortid/main.go": "package main\n\nimport \"github.com/kuronosu/tela\"\n\nvar App = tela.App{Title: \"Short\", AppID: \"example.short\"}\n\nfunc main() {}\n",
	})
	t.Chdir(dir)
	t.Setenv("SOURCE_DATE_EPOCH", "1763337600") // 2025-11-17
	// The signer is this test binary (fakeChild's "sign" mode), which runs
	// on every system: a shell script does not run on Windows.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(envFake, "sign")
	signer := `"` + exe + `"`

	pack := func(out string, extra ...string) (string, string) {
		t.Helper()
		var stdout, stderr bytes.Buffer
		args := append([]string{"package", "-os", "linux", "-arch", "amd64,arm64", "-format", "appimage,deb,flatpak", "-version", "1.2.0-rc.1", "-o", out}, extra...)
		if code := run(append(args, "./notes"), &stdout, &stderr); code != 0 {
			t.Fatalf("tela package: code %d\n%s%s", code, stdout.String(), stderr.String())
		}
		return stdout.String(), stderr.String()
	}
	out1, out2 := filepath.Join(dir, "dist1"), filepath.Join(dir, "dist2")
	stdout, stderr := pack(out1, "-sign", "-sign-cmd", signer)
	if !strings.Contains(stderr, "no signing tool to run for Linux") {
		t.Errorf("-sign on Linux says nothing:\n%s", stderr)
	}
	if strings.Contains(stdout, "(unsigned)") || !strings.Contains(stdout, "(signed)") {
		t.Errorf("signed outputs:\n%s", stdout)
	}
	pack(out2)

	// The same bytes, and the names tela update manifest reads.
	t1, t2 := readTree(t, out1), readTree(t, out2)
	var names []string
	for n, b := range t1 {
		if strings.HasSuffix(n, ".sig") {
			if string(b) != "signed\n" {
				t.Errorf("%s: %q", n, b)
			}
			continue
		}
		names = append(names, n)
		if !bytes.Equal(b, t2[n]) {
			t.Errorf("%s differs between two runs", n)
		}
	}
	sort.Strings(names)
	for _, arch := range []string{"amd64", "arm64"} {
		for _, ext := range []string{".AppImage", ".deb"} {
			n := "notes-1.2.0-rc.1-linux-" + arch + ext
			if t1[n] == nil {
				t.Errorf("no %s in %v", n, names)
				continue
			}
			if t1[n+".sig"] == nil {
				t.Errorf("%s was not signed", n)
			}
			goos, goarch, format, err := targetOfName(n, "1.2.0-rc.1")
			if err != nil || goos != "linux" || goarch != arch || format != map[string]string{".AppImage": "appimage", ".deb": "deb"}[ext] {
				t.Errorf("tela update manifest reads %s as %s/%s/%s (%v)", n, goos, goarch, format, err)
			}
		}
		if t1["notes-1.2.0-rc.1-linux-"+arch+"-flatpak/dev.example.Notes.json"] == nil {
			t.Errorf("no flatpak manifest for %s", arch)
		}
	}

	// The deb.
	members, control, data, contents := readDeb(t, filepath.Join(out1, "notes-1.2.0-rc.1-linux-amd64.deb"))
	if !reflect.DeepEqual(members, []string{"debian-binary", "control.tar.gz", "data.tar.gz"}) {
		t.Errorf("members %v", members)
	}
	ctl := string(contents["control:./control"])
	for _, want := range []string{"Package: notes\n", "Version: 1.2.0~rc.1\n", "Architecture: amd64\n", "Maintainer: Example Ltd <dev@example.com>\n", "Section: text\n", "Description: Takes notes.\n", "Installed-Size: "} {
		if !strings.Contains(ctl, want) {
			t.Errorf("control lacks %q:\n%s", want, ctl)
		}
	}
	if strings.Contains(ctl, "Depends") {
		t.Errorf("control has Depends:\n%s", ctl)
	}
	for _, s := range []string{"./postinst", "./postrm"} {
		if h := control[s]; h == nil || h.Mode != 0o755 || !strings.Contains(string(contents["control:"+s]), "update-mime-database") {
			t.Errorf("%s: %+v", s, h)
		}
	}
	wantFiles := []string{
		"./usr/bin/notes", "./usr/share/applications/dev.example.Notes.desktop",
		"./usr/share/metainfo/dev.example.Notes.metainfo.xml", "./usr/share/mime/packages/dev.example.Notes.xml",
		"./usr/share/icons/hicolor/16x16/apps/dev.example.Notes.png", "./usr/share/icons/hicolor/512x512/apps/dev.example.Notes.png",
		"./usr/share/icons/hicolor/scalable/apps/dev.example.Notes.svg",
	}
	for _, f := range wantFiles {
		h := data[f]
		if h == nil {
			t.Errorf("the deb lacks %s", f)
			continue
		}
		if h.Uid != 0 || h.Gid != 0 || h.ModTime.Unix() != 1763337600 {
			t.Errorf("%s: owner %d:%d, time %v", f, h.Uid, h.Gid, h.ModTime)
		}
	}
	if data["./usr/bin/notes"].Mode != 0o755 {
		t.Errorf("the program's mode is %o", data["./usr/bin/notes"].Mode)
	}
	desktop := string(contents["./usr/share/applications/dev.example.Notes.desktop"])
	for _, want := range []string{"Name=Notes Demo\n", "Comment=Takes notes.\n", "Exec=notes %U\n", "Icon=dev.example.Notes\n", "Categories=Office;\n", "StartupWMClass=dev.example.Notes\n", "MimeType=application/x-notes-note;x-scheme-handler/notes;\n"} {
		if !strings.Contains(desktop, want) {
			t.Errorf("the desktop entry lacks %q:\n%s", want, desktop)
		}
	}
	if v, err := exec.LookPath("desktop-file-validate"); err == nil {
		p := filepath.Join(t.TempDir(), "dev.example.Notes.desktop")
		os.WriteFile(p, []byte(desktop), 0o644)
		if out, err := exec.Command(v, p).CombinedOutput(); err != nil || len(out) > 0 {
			t.Errorf("desktop-file-validate: %v\n%s", err, out)
		}
	}
	meta := string(contents["./usr/share/metainfo/dev.example.Notes.metainfo.xml"])
	for _, want := range []string{"<id>dev.example.Notes</id>", "<developer id=\"dev.example\">", "<name>Example Ltd &lt;dev@example.com&gt;</name>", `<release version="1.2.0-rc.1" date="2025-11-17"/>`, "<launchable type=\"desktop-id\">dev.example.Notes.desktop</launchable>", "<mediatype>application/x-notes-note</mediatype>"} {
		if !strings.Contains(meta, want) {
			t.Errorf("the metainfo lacks %q:\n%s", want, meta)
		}
	}
	mime := string(contents["./usr/share/mime/packages/dev.example.Notes.xml"])
	if !strings.Contains(mime, `<glob pattern="*.nbak"/>`) || !strings.Contains(mime, "<comment>Notes document</comment>") {
		t.Errorf("the MIME package:\n%s", mime)
	}
	if dpkg, err := exec.LookPath("dpkg-deb"); err == nil {
		if out, err := exec.Command(dpkg, "--info", filepath.Join(out1, "notes-1.2.0-rc.1-linux-amd64.deb")).CombinedOutput(); err != nil {
			t.Errorf("dpkg-deb --info: %v\n%s", err, out)
		}
	}

	// The flatpak manifest.
	var m struct {
		ID, Command    string
		RuntimeVersion string   `json:"runtime-version"`
		FinishArgs     []string `json:"finish-args"`
		Modules        []struct {
			BuildCommands []string `json:"build-commands"`
		}
	}
	if err := json.Unmarshal(t1["notes-1.2.0-rc.1-linux-amd64-flatpak/dev.example.Notes.json"], &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != "dev.example.Notes" || m.Command != "notes" || m.RuntimeVersion == "" || len(m.Modules) != 1 {
		t.Errorf("manifest %+v", m)
	}
	for _, want := range []string{"--socket=wayland", "--socket=fallback-x11", "--share=ipc", "--device=dri", "--share=network", "--talk-name=org.freedesktop.Notifications", "--filesystem=home"} {
		if !slices.Contains(m.FinishArgs, want) {
			t.Errorf("finish-args lack %s: %v", want, m.FinishArgs)
		}
	}
	if !strings.Contains(stderr, "Services.Camera: through the Camera portal") {
		t.Errorf("the camera's mapping is not said:\n%s", stderr)
	}
	if t1["notes-1.2.0-rc.1-linux-amd64-flatpak/files/notes"] == nil || !slices.Contains(m.Modules[0].BuildCommands, "install -Dm755 notes /app/bin/notes") {
		t.Errorf("the flatpak's program: %v", m.Modules[0].BuildCommands)
	}

	// The arm64 AppImage's runtime.
	f, err := elf.Open(filepath.Join(out1, "notes-1.2.0-rc.1-linux-arm64.AppImage"))
	if err != nil {
		t.Fatal(err)
	}
	if f.Machine != elf.EM_AARCH64 || f.Section(".upd_info") == nil || f.Section(".tela_payload") == nil {
		t.Errorf("the arm64 AppImage: %v", f.Machine)
	}
	f.Close()

	// The AppImage runs here.
	if runtime.GOOS == "linux" && runtime.GOARCH == "amd64" {
		cmd := exec.Command(filepath.Join(out1, "notes-1.2.0-rc.1-linux-amd64.AppImage"), "a", "b")
		cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+t.TempDir())
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "notes says hi true [a b]") {
			t.Errorf("running the AppImage: %v\n%s", err, out)
		}
	}

	// Mistakes, reported before anything is built.
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-os", "linux", "./noid"}, "has no app ID"},
		{[]string{"-os", "linux", "./badscheme"}, "standard scheme"},
		{[]string{"-os", "linux", "./badcat"}, `write it as "Office"`},
		{[]string{"-os", "linux", "-format", "flatpak", "./shortid"}, "not a flatpak application ID"},
		{[]string{"-os", "linux", "-format", "nope", "./notes"}, "no such format"},
		{[]string{"-os", "linux", "-format", "msix", "./notes"}, "not among the targets"},
		{[]string{"-os", "linux,windows", "-format", "deb", "./notes"}, "no format for windows"},
		{[]string{"-os", "linux", "-arch", "386", "-format", "appimage", "./notes"}, "64-bit"},
		{[]string{"-os", "plan9", "./notes"}, "does not package for plan9"},
	} {
		var so, se bytes.Buffer
		empty := filepath.Join(t.TempDir(), "out")
		code := run(append([]string{"package", "-o", empty}, tc.args...), &so, &se)
		if code != 1 || !strings.Contains(se.String(), tc.want) {
			t.Errorf("%v: code %d, stderr %q, want %q", tc.args, code, se.String(), tc.want)
		}
		if _, err := os.Stat(empty); err == nil {
			t.Errorf("%v wrote %s", tc.args, empty)
		}
	}
}
