package appbundle

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/plist"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readInfo(t *testing.T, bundle string) plist.Dict {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := plist.UnmarshalDict(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWriteLayoutAndInfo(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog")
	writeFile(t, exe, "binary v1")
	bundle := filepath.Join(dir, "out", "Notes.app")

	res, err := Write(bundle, Spec{ID: "dev.telaui.notes", Name: "Notes", Version: "1.2.3"}, exe)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || !res.InfoChanged || !res.ExeChanged {
		t.Errorf("first write: %+v", res)
	}
	if want := filepath.Join(bundle, "Contents", "MacOS", "Notes"); res.Exe != want {
		t.Errorf("Exe = %s, want %s", res.Exe, want)
	}
	if b, _ := os.ReadFile(res.Exe); string(b) != "binary v1" {
		t.Errorf("executable holds %q", b)
	}
	if st, _ := os.Stat(res.Exe); st.Mode().Perm()&0o111 == 0 && os.PathSeparator == '/' {
		t.Errorf("executable mode %v is not executable", st.Mode())
	}
	if _, err := os.Stat(filepath.Join(bundle, "Contents", "Resources")); err == nil {
		t.Errorf("Resources was created without resources")
	}

	want := plist.Dict{
		"CFBundleIdentifier":                   "dev.telaui.notes",
		"CFBundleName":                         "Notes",
		"CFBundleDisplayName":                  "Notes",
		"CFBundleExecutable":                   "Notes",
		"CFBundlePackageType":                  "APPL",
		"CFBundleInfoDictionaryVersion":        "6.0",
		"CFBundleShortVersionString":           "1.2.3",
		"CFBundleVersion":                      "1.2.3",
		"LSMinimumSystemVersion":               "13.0",
		"NSHighResolutionCapable":              true,
		"NSSupportsAutomaticGraphicsSwitching": true,
	}
	if got := readInfo(t, bundle); !reflect.DeepEqual(got, want) {
		t.Errorf("Info.plist\n%v\nwant\n%v", got, want)
	}
}

func TestInfoDefaults(t *testing.T) {
	info := Spec{ID: "a.b", Name: "My/App: x", DisplayName: "My App"}.Info()
	if info["CFBundleExecutable"] != "My-App- x" || info["CFBundleDisplayName"] != "My App" || info["CFBundleShortVersionString"] != "0.0.0" || info["CFBundleVersion"] != "0.0.0" {
		t.Errorf("defaults: %v", info)
	}
}

func TestRewriteIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog")
	writeFile(t, exe, "binary v1")
	bundle := filepath.Join(dir, "Notes.app")
	spec := Spec{ID: "dev.telaui.notes", Name: "Notes"}
	res, err := Write(bundle, spec, exe)
	if err != nil {
		t.Fatal(err)
	}

	// Age the files; an unchanged rebuild must not touch them.
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	infoPath := filepath.Join(bundle, "Contents", "Info.plist")
	for _, p := range []string{infoPath, res.Exe} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	// The program was rebuilt: same bytes, new file with a new time.
	writeFile(t, exe, "binary v1")

	again, err := Write(bundle, spec, exe)
	if err != nil {
		t.Fatal(err)
	}
	if again.Created || again.InfoChanged || again.ExeChanged {
		t.Errorf("rewrite changed something: %+v", again)
	}
	for _, p := range []string{infoPath, res.Exe} {
		if st, _ := os.Stat(p); !st.ModTime().Equal(old) {
			t.Errorf("%s was rewritten (mtime %v)", p, st.ModTime())
		}
	}

	// A changed program replaces the executable only.
	writeFile(t, exe, "binary v2")
	third, err := Write(bundle, spec, exe)
	if err != nil {
		t.Fatal(err)
	}
	if third.InfoChanged || !third.ExeChanged {
		t.Errorf("new program: %+v", third)
	}
	if b, _ := os.ReadFile(res.Exe); string(b) != "binary v2" {
		t.Errorf("executable holds %q", b)
	}
	if st, _ := os.Stat(infoPath); !st.ModTime().Equal(old) {
		t.Errorf("Info.plist was rewritten for a new executable")
	}
	// No temporary files are left.
	entries, _ := os.ReadDir(filepath.Join(bundle, "Contents", "MacOS"))
	if len(entries) != 1 {
		t.Errorf("MacOS holds %v", entries)
	}
}

func TestRewriteKeepsRegisteredKeys(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog")
	writeFile(t, exe, "x")
	bundle := filepath.Join(dir, "Notes.app")
	spec := Spec{ID: "dev.telaui.notes", Name: "Notes", Version: "1.0.0"}
	if _, err := Write(bundle, spec, exe); err != nil {
		t.Fatal(err)
	}

	// The running app registers a scheme and an unknown tool adds a key.
	infoPath := filepath.Join(bundle, "Contents", "Info.plist")
	d := readInfo(t, bundle)
	d["CFBundleURLTypes"] = plist.Array{plist.Dict{"CFBundleURLName": "dev.telaui.notes", "CFBundleURLSchemes": plist.Array{"notes"}}}
	d["X-Other"] = "kept"
	d["CFBundleShortVersionString"] = "stale"
	out, err := plist.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(infoPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Write(bundle, spec, exe)
	if err != nil {
		t.Fatal(err)
	}
	got := readInfo(t, bundle)
	if got["X-Other"] != "kept" || got["CFBundleURLTypes"] == nil {
		t.Errorf("registered keys lost: %v", got)
	}
	if got["CFBundleShortVersionString"] != "1.0.0" || !res.InfoChanged {
		t.Errorf("a managed key was not restored: %v (changed %v)", got["CFBundleShortVersionString"], res.InfoChanged)
	}
	// Now it is stable again.
	if res, _ = Write(bundle, spec, exe); res.InfoChanged {
		t.Errorf("Info.plist changed on a second rewrite")
	}
}

func TestRewriteReplacesUnreadableInfo(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog")
	writeFile(t, exe, "x")
	bundle := filepath.Join(dir, "A.app")
	if err := os.MkdirAll(filepath.Join(bundle, "Contents"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(bundle, "Contents", "Info.plist"), "bplist00 not xml")
	if _, err := Write(bundle, Spec{ID: "a.b", Name: "A"}, exe); err != nil {
		t.Fatal(err)
	}
	if got := readInfo(t, bundle); got["CFBundleIdentifier"] != "a.b" {
		t.Errorf("Info.plist: %v", got)
	}
}

func TestRenameRemovesStaleExecutable(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog")
	writeFile(t, exe, "x")
	bundle := filepath.Join(dir, "A.app")
	if _, err := Write(bundle, Spec{ID: "a.b", Name: "Old"}, exe); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(bundle, Spec{ID: "a.b", Name: "New"}, exe); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(bundle, "Contents", "MacOS"))
	if len(entries) != 1 || entries[0].Name() != "New" {
		t.Errorf("MacOS holds %v, want only New", entries)
	}
}

func TestResources(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog")
	writeFile(t, exe, "x")
	icon := filepath.Join(dir, "icon.icns")
	writeFile(t, icon, "icns")
	bundle := filepath.Join(dir, "A.app")
	spec := Spec{ID: "a.b", Name: "A", Resources: map[string]string{"icon.icns": icon, "fr.lproj/Strings": icon}}
	if _, err := Write(bundle, spec, exe); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"icon.icns", "fr.lproj/Strings"} {
		if b, err := os.ReadFile(filepath.Join(bundle, "Contents", "Resources", filepath.FromSlash(rel))); err != nil || string(b) != "icns" {
			t.Errorf("%s: %q, %v", rel, b, err)
		}
	}
	spec.Resources = map[string]string{"../escape": icon}
	if _, err := Write(bundle, spec, exe); err == nil || !strings.Contains(err.Error(), "inside Contents/Resources") {
		t.Errorf("escaping resource path: %v", err)
	}
}

func TestValidID(t *testing.T) {
	for _, id := range []string{"a", "com.example.notes", "dev.telaui.my-app", "A1.b2"} {
		if err := ValidID(id); err != nil {
			t.Errorf("ValidID(%q) = %v", id, err)
		}
	}
	for id, want := range map[string]string{
		"":              "empty",
		"my app":        `has ' '`,
		"com.example/x": `has '/'`,
		"com_example":   `has '_'`,
		"com.exämple":   "ä",
		".x":            "empty part",
		"a..b":          "empty part",
		"x.":            "empty part",
	} {
		if err := ValidID(id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidID(%q) = %v, want an error with %q", id, err, want)
		}
	}
}

func TestWriteErrors(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "prog")
	writeFile(t, exe, "x")
	if _, err := Write(filepath.Join(dir, "A.app"), Spec{ID: "bad id", Name: "A"}, exe); err == nil {
		t.Error("an invalid ID was accepted")
	}
	if _, err := Write(filepath.Join(dir, "A.app"), Spec{ID: "a.b"}, exe); err == nil {
		t.Error("a missing name was accepted")
	}
	if _, err := Write(filepath.Join(dir, "A.app"), Spec{ID: "a.b", Name: "A"}, filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing executable was accepted")
	}
}

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"Notes":     "Notes",
		"My App":    "My App",
		"a/b\\c:d":  "a-b-c-d",
		"..hidden":  "hidden",
		"":          "App",
		"  ":        "App",
		"tab\there": "tab-here",
	} {
		if got := FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}
