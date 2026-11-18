package pkgfmt

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/assoc"
	"github.com/kuronosu/tela/internal/plist"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// macApp is notesApp with every service and a public file type.
func macApp(t *testing.T) App {
	app := notesApp(t)
	app.Name = "Notes Pro"
	app.Publisher = "Example Ltd <dev@example.com>"
	app.Version = "1.2.0-rc.1"
	app.FileTypes = append(app.FileTypes,
		tela.FileType{Ext: ".txt", MIME: "text/plain", Description: "Text", Role: tela.HandlerViewer})
	app.Services = tela.Services{
		Camera: "Take a picture", Microphone: "Record a memo", Location: "Tag notes with places",
		Contacts: "Share with contacts", Calendars: "Link meetings", Photos: "Attach photos",
		Bluetooth: "Find a pen", Network: true, Files: true,
	}
	return app
}

// goldenFile compares got with testdata/<name>; TELA_UPDATE=1 rewrites it.
func goldenFile(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv(golden.UpdateEnv) == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; create it with %s=1 go test and review it", err, golden.UpdateEnv)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("%s differs:\n%s\nregenerate with %s=1 if the change is intended", path, got, golden.UpdateEnv)
	}
}

func TestInfoPlistGolden(t *testing.T) {
	b, err := plist.Marshal(BundleSpec(macApp(t)).Info())
	if err != nil {
		t.Fatal(err)
	}
	goldenFile(t, "notes.Info.plist", b)
	goldenFile(t, "notes.entitlements", Entitlements(macApp(t)))

	d := BundleKeys(App{ID: "a.b.c", Name: "C", Package: "c"})
	if !reflect.DeepEqual(d, plist.Dict{"LSApplicationCategoryType": "public.app-category.utilities"}) {
		t.Errorf("a plain app's keys: %v", d)
	}
	if e := string(Entitlements(App{})); !strings.Contains(e, "<dict/>") && !strings.Contains(e, "<dict>\n</dict>") {
		t.Errorf("no services:\n%s", e)
	}
}

func TestAppCategory(t *testing.T) {
	for _, c := range Categories {
		if got := AppCategory(App{Category: c}); !strings.HasPrefix(got, "public.app-category.") || got == "public.app-category." {
			t.Errorf("%s: %q", c, got)
		}
	}
	if AppCategory(App{Category: "Game"}) != "public.app-category.games" || AppCategory(App{}) != "public.app-category.utilities" {
		t.Errorf("Game or none")
	}
}

func TestBundleVersion(t *testing.T) {
	for v, want := range map[string]string{"1.2.3": "1.2.3", "v1.2.3": "1.2.3", "1.2.0-rc.1": "1.2.0", "": "0.0.0", "x": "0.0.0",
		"0.14.7-0.20261117184909-84c135c35505+dirty": "0.14.7"} {
		if got := BundleVersion(v); got != want {
			t.Errorf("BundleVersion(%q) = %q, want %q", v, got, want)
		}
	}
}

// fakeLS is Launch Services for the registration check: it knows the
// bundle once registered, and the content types of types.
type fakeLS struct {
	registered []string
	types      map[string]string
}

func (f *fakeLS) RegisterBundle(p string) error               { f.registered = append(f.registered, p); return nil }
func (f *fakeLS) BundlePaths(string) ([]string, error)        { return f.registered, nil }
func (f *fakeLS) SchemeHandlers(string) ([]string, error)     { return nil, nil }
func (f *fakeLS) DefaultSchemeHandler(string) (string, error) { return "", nil }
func (f *fakeLS) SetDefaultSchemeHandler(string, string) error {
	return errors.New("not used")
}
func (f *fakeLS) TypeHandlers(string) ([]string, error)     { return nil, nil }
func (f *fakeLS) DefaultTypeHandler(string) (string, error) { return "", nil }
func (f *fakeLS) SetDefaultTypeHandler(string, string) error {
	return errors.New("not used")
}
func (f *fakeLS) TypeOfExtension(ext string) (string, error) {
	if u, ok := f.types[ext]; ok {
		return u, nil
	}
	return "dyn.ah62d4rv4ge8" + ext, nil
}

// bundleRoot returns a new folder for a bundle that registration accepts.
// Launch Services ignores bundles in the temporary folders, and so does
// registration on a Mac (os.TempDir() and /private/var/folders, where
// t.TempDir is): there the folder is under <user cache>/tela/test,
// removed at the end of the test. Elsewhere those folders do not count,
// and t.TempDir serves.
func bundleRoot(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		return t.TempDir()
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("no user cache folder for the bundle: %v", err)
	}
	base := filepath.Join(cache, "tela", "test")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "pkgfmt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// TestRegistrationFindsDeclarations is the agreement with the
// registration of a running app (ADR-228): in a signed bundle (sealed by
// Contents/_CodeSignature) every scheme and file type of the declaration
// registers without an edit of Info.plist, which would fail, only by
// asking Launch Services to read the bundle. The system may report
// another content type for a public extension than the one declared.
func TestRegistrationFindsDeclarations(t *testing.T) {
	app := macApp(t)
	root := bundleRoot(t)
	exe := filepath.Join(root, "prog")
	os.WriteFile(exe, []byte("program"), 0o755)
	bundle := filepath.Join(root, "Notes Pro.app")
	res, err := WriteBundle(bundle, app, exe, root)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(bundle, "Contents", "Resources", "notes.icns")); err != nil || !bytes.HasPrefix(b, []byte("icns")) {
		t.Errorf("the icon: %v", err)
	}
	os.MkdirAll(filepath.Join(bundle, "Contents", "_CodeSignature"), 0o755)
	infoPath := filepath.Join(bundle, "Contents", "Info.plist")
	before, _ := os.ReadFile(infoPath)
	for _, types := range []map[string]string{{"txt": "public.plain-text"}, {"txt": "com.example.other-text"}} {
		ls := &fakeLS{types: types}
		a := assoc.MacOS(ls)
		h := platform.HandlerApp{ID: app.ID, Name: app.Name, Exec: res.Exe}
		var all []platform.Association
		for _, s := range app.Schemes {
			all = append(all, platform.Association{Scheme: s})
		}
		for _, ft := range app.FileTypes {
			all = append(all, platform.Association{Extension: ft.Ext, MIMEType: ft.MIME, Description: ft.Description, Role: platform.HandlerRole(ft.Role)})
		}
		for _, as := range all {
			if _, err := a.Register(h, as, platform.RegisterOptions{}); err != nil {
				t.Errorf("%v with %v: %v", as, types, err)
			}
		}
		if len(ls.registered) == 0 {
			t.Errorf("nothing asked Launch Services to read the bundle")
		}
	}
	if after, _ := os.ReadFile(infoPath); !bytes.Equal(before, after) {
		t.Errorf("registration changed the Info.plist")
	}
}

// TestRebuildKeepsRegistrations: a development bundle rebuilt from a
// declaration keeps what registration added that the declaration does not
// declare, drops what it now declares, and drops keys it no longer has.
func TestRebuildKeepsRegistrations(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "prog")
	os.WriteFile(exe, []byte("program"), 0o755)
	bundle := filepath.Join(root, "N.app")
	app := App{ID: "com.example.n", Name: "N", Package: "n", Services: tela.Services{Camera: "see"}}
	if _, err := WriteBundle(bundle, app, exe, root); err != nil {
		t.Fatal(err)
	}
	// What registration adds at run time.
	infoPath := filepath.Join(bundle, "Contents", "Info.plist")
	raw, _ := os.ReadFile(infoPath)
	info, _ := plist.UnmarshalDict(raw)
	mark := func(d plist.Dict) plist.Dict { d["TelaRegistered"] = true; return d }
	info["CFBundleURLTypes"] = plist.Array{
		mark(plist.Dict{"CFBundleURLName": "com.example.n.web", "CFBundleURLSchemes": plist.Array{"web"}}),
		mark(plist.Dict{"CFBundleURLName": "com.example.n.n", "CFBundleURLSchemes": plist.Array{"n"}}),
	}
	info["UTImportedTypeDeclarations"] = plist.Array{mark(plist.Dict{"UTTypeIdentifier": "com.example.n.nn"})}
	raw, _ = plist.Marshal(info)
	os.WriteFile(infoPath, raw, 0o644)

	app.Services = tela.Services{}
	app.Schemes = []string{"n"}
	app.FileTypes = []tela.FileType{{Ext: ".nn", MIME: "application/x-nn", Description: "N"}}
	if _, err := WriteBundle(bundle, app, exe, root); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(infoPath)
	info, _ = plist.UnmarshalDict(raw)
	if _, ok := info["NSCameraUsageDescription"]; ok {
		t.Errorf("the camera's usage string stayed")
	}
	if _, ok := info["UTImportedTypeDeclarations"]; ok {
		t.Errorf("the imported type stayed though the declaration exports it: %v", info["UTImportedTypeDeclarations"])
	}
	urls, _ := info["CFBundleURLTypes"].(plist.Array)
	var names []string
	for _, u := range urls {
		names = append(names, u.(plist.Dict)["CFBundleURLName"].(string))
	}
	if !reflect.DeepEqual(names, []string{"com.example.n.n", "com.example.n.web"}) {
		t.Errorf("URL types %v", names)
	}
	if _, marked := urls[0].(plist.Dict)["TelaRegistered"]; marked {
		t.Errorf("the declared scheme kept the registration's entry")
	}
	if res, err := WriteBundle(bundle, app, exe, root); err != nil || res.InfoChanged {
		t.Errorf("a third write changed the Info.plist (%v)", err)
	}
}

// readZip returns the entries of a zip by name.
func readZip(t *testing.T, b []byte) (map[string]*zip.File, []string) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]*zip.File{}
	var order []string
	for _, f := range zr.File {
		m[f.Name] = f
		order = append(order, f.Name)
	}
	return m, order
}

func TestZipBundle(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Café.app")
	for p, c := range map[string]string{"Contents/Info.plist": "plist", "Contents/MacOS/Café": "prog", "Contents/Frameworks/A.framework/Versions/A/A": "lib"} {
		os.MkdirAll(filepath.Join(app, filepath.Dir(p)), 0o755)
		mode := fs.FileMode(0o600)
		if strings.Contains(p, "MacOS") || strings.HasSuffix(p, "/A") {
			mode = 0o700
		}
		os.WriteFile(filepath.Join(app, p), []byte(c), mode)
	}
	os.MkdirAll(filepath.Join(app, "Contents", "Resources"), 0o700)
	unix := runtime.GOOS != "windows"
	if unix {
		os.Symlink("Versions/A/A", filepath.Join(app, "Contents/Frameworks/A.framework/A"))
	}
	when := time.Date(2026, 11, 17, 10, 20, 31, 0, time.UTC)
	var a, b bytes.Buffer
	if err := ZipBundle(&a, app, when); err != nil {
		t.Fatal(err)
	}
	ZipBundle(&b, app, when)
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Errorf("two zips of the same bundle differ")
	}
	files, order := readZip(t, a.Bytes())
	if order[0] != "Café.app/" {
		t.Errorf("the first entry is %q, not the bundle's folder", order[0])
	}
	for _, n := range order {
		if !strings.HasPrefix(n, "Café.app/") {
			t.Errorf("%s is outside the bundle's folder", n)
		}
		f := files[n]
		if f.CreatorVersion>>8 != 3 || !f.Modified.Equal(when) || strings.HasSuffix(n, "/") != (f.Method == zip.Store) {
			t.Errorf("%s: made by %#x, flags %#x, time %v", n, f.CreatorVersion, f.Flags, f.Modified)
		}
		if f.Flags&0x800 == 0 {
			t.Errorf("%s: the UTF-8 flag is not set", n)
		}
		if i := strings.LastIndex(strings.TrimSuffix(n, "/"), "/"); i > 0 {
			if _, ok := files[n[:i+1]]; !ok {
				t.Errorf("%s comes without its folder", n)
			}
		}
	}
	for n, want := range map[string]fs.FileMode{
		"Café.app/":                    fs.ModeDir | 0o755,
		"Café.app/Contents/Resources/": fs.ModeDir | 0o755,
		"Café.app/Contents/Info.plist": 0o644,
		"Café.app/Contents/MacOS/Café": 0o755,
	} {
		if f := files[n]; f == nil || f.Mode() != want {
			t.Errorf("%s: %v, want %v", n, f, want)
		}
	}
	if unix {
		l := files["Café.app/Contents/Frameworks/A.framework/A"]
		if l == nil || l.Mode()&fs.ModeSymlink == 0 {
			t.Fatalf("the link: %v", l)
		}
		r, _ := l.Open()
		target, _ := io.ReadAll(r)
		if string(target) != "Versions/A/A" {
			t.Errorf("the link holds %q", target)
		}
	}
	r, _ := files["Café.app/Contents/MacOS/Café"].Open()
	if b, _ := io.ReadAll(r); string(b) != "prog" {
		t.Errorf("the program holds %q", b)
	}
}

// TestZipBundleWithoutExecBits: on Windows, whose folders keep no
// executable bits, the bundle's executables are told by their place and
// content, or the zip would unpack a program macOS cannot start.
func TestZipBundleWithoutExecBits(t *testing.T) {
	defer func(h string) { hostOS = h }(hostOS)
	hostOS = "windows"
	app := filepath.Join(t.TempDir(), "N.app")
	machO := string([]byte{0xcf, 0xfa, 0xed, 0xfe, 7, 0, 0, 1})
	fat := string([]byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 2})
	want := map[string]fs.FileMode{
		"Contents/Info.plist":                                   0o644,
		"Contents/MacOS/N":                                      0o755, // by its place, whatever it holds
		"Contents/Frameworks/F.framework/Versions/A/F":          0o755, // Mach-O
		"Contents/Helpers/tool":                                 0o755, // universal Mach-O
		"Contents/Resources/run.sh":                             0o755, // a script
		"Contents/Resources/data.bin":                           0o644,
		"Contents/Library/LoginItems/H.app/Contents/MacOS/H":    0o755,
		"Contents/Library/LoginItems/H.app/Contents/Info.plist": 0o644,
		"Contents/Resources/MacOS/notes.txt":                    0o644, // a MacOS folder outside a Contents
	}
	data := map[string]string{"Contents/Frameworks/F.framework/Versions/A/F": machO, "Contents/Helpers/tool": fat,
		"Contents/Resources/run.sh": "#!/bin/sh\n"}
	for p := range want {
		os.MkdirAll(filepath.Join(app, filepath.Dir(p)), 0o755)
		c, ok := data[p]
		if !ok {
			c = "plain"
		}
		if err := os.WriteFile(filepath.Join(app, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var b bytes.Buffer
	if err := ZipBundle(&b, app, time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	files, _ := readZip(t, b.Bytes())
	for p, mode := range want {
		if f := files["N.app/"+p]; f == nil || f.Mode() != mode {
			t.Errorf("%s: %v, want %v", p, f, mode)
		}
	}
}

func TestMacFormatsCheck(t *testing.T) {
	app, _ := Lookup("app")
	if err := app.Check(App{}, Target{"darwin", "universal"}); err != nil {
		t.Errorf("app universal: %v", err)
	}
	if err := app.Check(App{}, Target{"darwin", "386"}); err == nil || !strings.Contains(err.Error(), "amd64, arm64 or universal") {
		t.Errorf("app 386: %v", err)
	}
	dmg, _ := Lookup("dmg")
	defer func(h string) { hostOS = h }(hostOS)
	hostOS = "linux"
	if err := dmg.Check(App{}, Target{"darwin", "arm64"}); err == nil || !strings.Contains(err.Error(), "needs a Mac") || !strings.Contains(err.Error(), "-format app") {
		t.Errorf("dmg on Linux: %v", err)
	}
}

func TestMacSigner(t *testing.T) {
	var ran []string
	notary := `{"id":"abc","status":"Accepted","message":"ok"}`
	s := MacSigner{Identity: "Developer ID Application: X (T)", NotaryProfile: "p", Entitlements: "/out/n.entitlements",
		Run: func(name string, args ...string) ([]byte, error) {
			ran = append(ran, name+" "+strings.Join(args, " "))
			if len(args) > 1 && args[0] == "notarytool" {
				return []byte(notary), nil
			}
			return nil, nil
		}}
	mac := Target{"darwin", "arm64"}
	for _, c := range []struct {
		path, what string
		signed     bool
	}{{"/w/N", SignProgram, true}, {"/w/N.app", SignBundle, true}, {"/o/n.zip", SignPackage, false}, {"/o/n.dmg", SignPackage, true}} {
		if ok, err := s.Sign(c.path, c.what, mac); ok != c.signed || err != nil {
			t.Errorf("%s %s: %v %v", c.what, c.path, ok, err)
		}
	}
	if ok, _ := s.Sign("/o/n.deb", SignPackage, Target{"linux", "amd64"}); ok {
		t.Errorf("signed a Linux package")
	}
	if ok, err := (Chain{CommandSigner{}, s}).Notarize("/w/up.zip", "/w/N.app", mac); !ok || err != nil {
		t.Errorf("Notarize: %v %v", ok, err)
	}
	want := []string{
		"codesign --force --options runtime --timestamp --entitlements /out/n.entitlements -s Developer ID Application: X (T) /w/N",
		"codesign --force --options runtime --timestamp --entitlements /out/n.entitlements -s Developer ID Application: X (T) /w/N.app",
		"codesign --verify --deep --strict /w/N.app",
		"codesign --force --timestamp -s Developer ID Application: X (T) /o/n.dmg",
		"xcrun notarytool submit /w/up.zip --keychain-profile p --wait --output-format json",
		"xcrun stapler staple /w/N.app",
	}
	if !reflect.DeepEqual(ran, want) {
		t.Errorf("ran\n%s\nwant\n%s", strings.Join(ran, "\n"), strings.Join(want, "\n"))
	}
	notary = `{"id":"abc","status":"Invalid","message":"bad"}`
	if _, err := s.Notarize("/w/up.zip", "/w/N.app", mac); err == nil || !strings.Contains(err.Error(), "xcrun notarytool log abc --keychain-profile p") {
		t.Errorf("an invalid submission: %v", err)
	}
	if ok, _ := (MacSigner{Identity: "x"}).Notarize("a", "b", mac); ok {
		t.Errorf("notarized without a profile")
	}
}
