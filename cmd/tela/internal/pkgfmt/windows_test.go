package pkgfmt

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform/assoc"
	"github.com/kuronosu/tela/update"
)

func TestMSIXVersion(t *testing.T) {
	for v, want := range map[string]string{"1.2.3": "1.2.3.0", "v0.14.6": "0.14.6.0", "2": "2.0.0.0", "1.2": "1.2.0.0", "1.2.3-rc.1": "1.2.3.0", "1.0.0+b5": "1.0.0.0"} {
		got, note, err := MSIXVersion(v)
		if err != nil || got != want || (note != "") != strings.ContainsAny(strings.TrimPrefix(v, "v"), "-+") {
			t.Errorf("MSIXVersion(%q) = %q, %q, %v; want %q", v, got, note, err, want)
		}
	}
	for _, v := range []string{"1.2.3.4", "70000.0.0", "x.1.2", "1..2"} {
		if _, _, err := MSIXVersion(v); err == nil {
			t.Errorf("MSIXVersion(%q) accepted", v)
		}
	}
}

func TestMSIXPublisher(t *testing.T) {
	for _, tc := range []struct {
		publisher string
		o         MSIXOptions
		want      string
	}{
		{"Example Ltd <dev@example.com>", MSIXOptions{}, "CN=Example Ltd, " + UnsignedPublisherOID},
		{"Example Ltd", MSIXOptions{Signed: true}, "CN=Example Ltd"},
		{"A, B & \"C\"", MSIXOptions{Signed: true}, `CN="A, B & ""C"""`},
		{"", MSIXOptions{Signed: true}, "CN=Notes\tPro"},
		{"x", MSIXOptions{Publisher: "CN=Real, O=Real, C=US"}, "CN=Real, O=Real, C=US"},
	} {
		app := App{Name: "Notes\tPro", Publisher: tc.publisher}
		got := MSIXPublisher(app, tc.o)
		if got != tc.want {
			t.Errorf("MSIXPublisher(%q, %+v) = %q, want %q", tc.publisher, tc.o, got, tc.want)
		}
		if err := CheckPublisher(got); err != nil {
			t.Errorf("%q: %v", got, err)
		}
	}
	for _, p := range []string{"Example", "CN=a,O=b", "CN=a<b", "XX=a", ""} {
		if CheckPublisher(p) == nil {
			t.Errorf("CheckPublisher(%q) accepted it", p)
		}
	}
}

func TestCheckMSIX(t *testing.T) {
	f := NewMSIX(MSIXOptions{})
	ok := App{ID: "com.example.Notes", Schemes: []string{"Notes"}}
	if err := f.Check(ok, Target{"windows", "amd64"}); err != nil {
		t.Errorf("valid: %v", err)
	}
	for _, tc := range []struct {
		app  App
		arch string
		want string
	}{
		{ok, "riscv64", "not windows/riscv64"},
		{App{ID: "ab"}, "amd64", "3 to 50"},
		{App{ID: "con.example.x"}, "amd64", "device name"},
		{App{ID: "com.example.x", Schemes: []string{"a"}}, "arm64", "MSIX protocol"},
	} {
		if err := f.Check(tc.app, Target{"windows", tc.arch}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v %s: %v, want %q", tc.app, tc.arch, err, tc.want)
		}
	}
	if err := NewMSIX(MSIXOptions{Publisher: "Example"}).Check(ok, Target{"windows", "amd64"}); err == nil || !strings.Contains(err.Error(), "-msix-publisher") {
		t.Errorf("a bad -msix-publisher: %v", err)
	}
}

// fullApp is notesApp with every service declared.
func fullApp(t *testing.T) App {
	app := notesApp(t)
	app.Services = tela.Services{Camera: "c", Microphone: "m", Location: "l", Contacts: "k", Calendars: "d", Photos: "p", Bluetooth: "b", Network: true, Files: true, Notifications: true, Secrets: true}
	return app
}

func TestMSIXManifest(t *testing.T) {
	app := fullApp(t)
	id := MSIXIdentity{Name: app.ID, Version: "1.0.0.0", Publisher: MSIXPublisher(app, MSIXOptions{}), Arch: "x64"}
	m := MSIXManifest(app, id, "notes.exe")
	// Well-formed, and the parts read back.
	var pkg struct {
		Identity struct {
			Name, Version, Publisher, ProcessorArchitecture string `xml:",attr"`
		}
		Properties struct {
			DisplayName, PublisherDisplayName, Logo string
		}
		Dependencies struct {
			TargetDeviceFamily struct {
				Name, MinVersion string `xml:",attr"`
			}
		}
		Applications struct {
			Application struct {
				Id, Executable, EntryPoint string `xml:",attr"`
				Extensions                 struct {
					Extension []struct {
						Category string `xml:",attr"`
						Protocol *struct {
							Name string `xml:",attr"`
						} `xml:"Protocol"`
						FTA *struct {
							Name        string `xml:",attr"`
							DisplayName string
							Types       []struct {
								ContentType string `xml:",attr"`
								Ext         string `xml:",chardata"`
							} `xml:"SupportedFileTypes>FileType"`
						} `xml:"FileTypeAssociation"`
						Server *struct {
							Executable, Arguments string `xml:",attr"`
							Class                 struct {
								Id string `xml:",attr"`
							}
						} `xml:"ComServer>ExeServer"`
						Toast *struct {
							ToastActivatorCLSID string `xml:",attr"`
						} `xml:"ToastNotificationActivation"`
					} `xml:",any"`
				}
			}
		}
		Capabilities struct {
			Caps []struct {
				XMLName xml.Name
				Name    string `xml:",attr"`
			} `xml:",any"`
		}
	}
	if err := xml.Unmarshal(m, &pkg); err != nil {
		t.Fatalf("%v\n%s", err, m)
	}
	if pkg.Identity.Name != "com.example.Notes" || pkg.Identity.Version != "1.0.0.0" || pkg.Identity.ProcessorArchitecture != "x64" ||
		pkg.Identity.Publisher != "CN=A & B, "+UnsignedPublisherOID {
		t.Errorf("Identity %+v", pkg.Identity)
	}
	if pkg.Properties.DisplayName != "Notes\tPro" || pkg.Properties.PublisherDisplayName != "A & B" || pkg.Properties.Logo != `Assets\StoreLogo.png` {
		t.Errorf("Properties %+v", pkg.Properties)
	}
	if d := pkg.Dependencies.TargetDeviceFamily; d.Name != "Windows.Desktop" || d.MinVersion != "10.0.17763.0" {
		t.Errorf("Dependencies %+v", d)
	}
	a := pkg.Applications.Application
	if a.Id != "App" || a.Executable != "notes.exe" || a.EntryPoint != "Windows.FullTrustApplication" {
		t.Errorf("Application %+v", a)
	}
	clsid := assoc.ToastActivatorCLSID(app.ID)
	var protocols, ftas []string
	var server, toast bool
	for _, e := range a.Extensions.Extension {
		switch {
		case e.Protocol != nil:
			protocols = append(protocols, e.Protocol.Name)
		case e.FTA != nil:
			var exts []string
			for _, ft := range e.FTA.Types {
				exts = append(exts, ft.ContentType+" "+ft.Ext)
			}
			ftas = append(ftas, e.FTA.Name+": "+e.FTA.DisplayName+": "+strings.Join(exts, ", "))
		case e.Server != nil:
			server = e.Category == "windows.comServer" && e.Server.Executable == "notes.exe" && e.Server.Arguments == assoc.ToastActivatorFlag && e.Server.Class.Id == clsid
		case e.Toast != nil:
			toast = e.Category == "windows.toastNotificationActivation" && e.Toast.ToastActivatorCLSID == clsid
		}
	}
	if !reflect.DeepEqual(protocols, []string{"notes"}) {
		t.Errorf("protocols %v", protocols)
	}
	if !reflect.DeepEqual(ftas, []string{"note: Note: application/x-note .note, application/x-note .note2"}) {
		t.Errorf("file type associations %q", ftas)
	}
	if !server || !toast {
		t.Errorf("the toast activator: server %v, toast activation %v\n%s", server, toast, m)
	}
	var caps []string
	for _, c := range pkg.Capabilities.Caps {
		caps = append(caps, c.XMLName.Local+" "+c.Name)
	}
	want := []string{"Capability internetClient", "Capability contacts", "Capability appointments", "Capability picturesLibrary",
		"Capability runFullTrust", "Capability broadFileSystemAccess",
		"DeviceCapability webcam", "DeviceCapability microphone", "DeviceCapability location", "DeviceCapability bluetooth"}
	if !reflect.DeepEqual(caps, want) {
		t.Errorf("capabilities %q\nwant %q", caps, want)
	}
	// A plain app has runFullTrust only, and no protocol or association.
	plain := string(MSIXManifest(App{ID: "a.b.c", Name: "C"}, MSIXIdentity{Name: "a.b.c", Version: "1.0.0.0", Publisher: "CN=C", Arch: "arm64"}, "c.exe"))
	if strings.Count(plain, "Capability Name=") != 1 || strings.Contains(plain, "uap:Protocol") || strings.Contains(plain, "FileTypeAssociation") {
		t.Errorf("a plain app's manifest:\n%s", plain)
	}
}

// readMSIX reads a package back: its entries in order, their data, and
// checks the zip's structure (no folders, no data descriptors, the zip64
// end records) and the block map against the data.
func readMSIX(t *testing.T, b []byte) (names []string, data map[string][]byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b[len(b)-22-20-56:], []byte{0x50, 0x4b, 0x06, 0x06}) {
		t.Errorf("no zip64 end of central directory record")
	}
	data = map[string][]byte{}
	entries := map[string]*zip.File{}
	for _, f := range zr.File {
		names = append(names, f.Name)
		if strings.HasSuffix(f.Name, "/") {
			t.Errorf("a folder entry %s", f.Name)
		}
		if f.Flags != 0 {
			t.Errorf("%s: flags %#x", f.Name, f.Flags)
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data[f.Name], err = io.ReadAll(r)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		entries[f.Name] = f
	}
	n := len(names)
	if n < 3 || names[n-3] != "AppxManifest.xml" || names[n-2] != "AppxBlockMap.xml" || names[n-1] != "[Content_Types].xml" {
		t.Errorf("entries %v", names)
	}
	// The block map, recomputed.
	var bm struct {
		XMLName    xml.Name
		HashMethod string `xml:",attr"`
		Files      []struct {
			Name    string `xml:",attr"`
			Size    int    `xml:",attr"`
			LfhSize int64  `xml:",attr"`
			Blocks  []struct {
				Hash string `xml:",attr"`
				Size int    `xml:",attr"`
			} `xml:"Block"`
		} `xml:"File"`
	}
	if err := xml.Unmarshal(data["AppxBlockMap.xml"], &bm); err != nil {
		t.Fatal(err)
	}
	if bm.XMLName.Space != "http://schemas.microsoft.com/appx/2010/blockmap" || bm.HashMethod != "http://www.w3.org/2001/04/xmlenc#sha256" {
		t.Errorf("block map %v %s", bm.XMLName, bm.HashMethod)
	}
	var listed []string
	for _, bf := range bm.Files {
		name := strings.ReplaceAll(bf.Name, `\`, "/")
		listed = append(listed, name)
		f := entries[opcName(name)]
		if f == nil {
			t.Errorf("the block map lists %s, which the package lacks", bf.Name)
			continue
		}
		d := data[f.Name]
		if bf.Size != len(d) {
			t.Errorf("%s: block map size %d, file %d", name, bf.Size, len(d))
		}
		off, err := f.DataOffset()
		if err != nil {
			t.Fatal(err)
		}
		lfh := b[off-bf.LfhSize:]
		if binary.LittleEndian.Uint32(lfh) != 0x04034b50 || 30+int64(binary.LittleEndian.Uint16(lfh[26:]))+int64(binary.LittleEndian.Uint16(lfh[28:])) != bf.LfhSize {
			t.Errorf("%s: LfhSize %d does not reach its local file header", name, bf.LfhSize)
		}
		if want := (len(d) + blockSize - 1) / blockSize; len(bf.Blocks) != want {
			t.Errorf("%s: %d blocks, want %d", name, len(bf.Blocks), want)
		}
		raw := b[off : off+int64(f.CompressedSize64)]
		total := 0
		for i, bl := range bf.Blocks {
			chunk := d[i*blockSize : min((i+1)*blockSize, len(d))]
			sum := sha256.Sum256(chunk)
			if bl.Hash != base64.StdEncoding.EncodeToString(sum[:]) {
				t.Errorf("%s: block %d's hash differs from its data", name, i)
			}
			switch f.Method {
			case zip.Deflate:
				// Each block inflates on its own.
				got, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw[total : total+bl.Size])))
				if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Errorf("%s: block %d: %v", name, i, err)
				}
				if !bytes.Equal(got, chunk) {
					t.Errorf("%s: block %d does not inflate alone to its data (%d bytes of %d)", name, i, len(got), len(chunk))
				}
				total += bl.Size
			case zip.Store:
				if bl.Size != 0 {
					t.Errorf("%s: a stored file's block has a size", name)
				}
			}
		}
		if f.Method == zip.Deflate && total+2 != len(raw) {
			t.Errorf("%s: the blocks are %d bytes of %d", name, total, len(raw))
		}
	}
	if want := names[:n-2]; !reflect.DeepEqual(listed, want) {
		t.Errorf("the block map lists %v, want %v", listed, want)
	}
	return names, data
}

func writeJob(t *testing.T, f Format, app App, arch string) (Job, []Output) {
	t.Helper()
	dir := t.TempDir()
	prog := filepath.Join(dir, "prog.exe")
	// More than two blocks, compressible and not.
	body := bytes.Repeat([]byte("MZ tela program "), 9000)
	for i := range 40000 {
		body = append(body, byte(i*7919>>3))
	}
	if err := os.WriteFile(prog, body, 0o755); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	job := Job{App: app, Target: Target{"windows", arch}, Program: prog, Time: time.Date(2025, 11, 17, 10, 20, 30, 0, time.UTC),
		OutDir: filepath.Join(dir, "out"), Work: t.TempDir(), Log: &log, Sign: NoSigner{}}
	outs, err := f.Write(job)
	if err != nil {
		t.Fatal(err)
	}
	return job, outs
}

func TestMSIXPackage(t *testing.T) {
	app := fullApp(t)
	app.Version = "1.2.0-rc.1"
	f := NewMSIX(MSIXOptions{AppInstallerURL: "https://example.com/notes/"})
	job, outs := writeJob(t, f, app, "arm64")
	if len(outs) != 2 || filepath.Base(outs[0].Path) != "notes-1.2.0-rc.1-windows-arm64.msix" || filepath.Base(outs[1].Path) != "notes-windows-arm64.appinstaller" {
		t.Fatalf("outputs %+v", outs)
	}
	if !strings.Contains(outs[0].Note, "-AllowUnsigned") {
		t.Errorf("note %q", outs[0].Note)
	}
	b, err := os.ReadFile(outs[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	names, data := readMSIX(t, b)
	want := []string{"Assets/Square150x150Logo.png", "Assets/Square44x44Logo.png", "Assets/StoreLogo.png", "notes.exe", "AppxManifest.xml", "AppxBlockMap.xml", "[Content_Types].xml"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("entries %v", names)
	}
	prog, _ := os.ReadFile(job.Program)
	if !bytes.Equal(data["notes.exe"], prog) {
		t.Errorf("the program differs")
	}
	if !bytes.HasPrefix(data["Assets/Square44x44Logo.png"], []byte("\x89PNG")) {
		t.Errorf("the tile is not a PNG")
	}
	if !bytes.Contains(data["AppxManifest.xml"], []byte(`Version="1.2.0.0"`)) || !bytes.Contains(data["AppxManifest.xml"], []byte(`ProcessorArchitecture="arm64"`)) {
		t.Errorf("manifest:\n%s", data["AppxManifest.xml"])
	}
	ct := string(data["[Content_Types].xml"])
	for _, s := range []string{`<Default Extension="exe" ContentType="application/x-msdownload"/>`, `<Default Extension="png" ContentType="image/png"/>`, `<Override PartName="/AppxManifest.xml"`, `<Override PartName="/AppxBlockMap.xml"`} {
		if !strings.Contains(ct, s) {
			t.Errorf("[Content_Types].xml lacks %s:\n%s", s, ct)
		}
	}
	if !strings.Contains(job.Log.(*bytes.Buffer).String(), "MSIX version of 1.2.0-rc.1 is 1.2.0.0") {
		t.Errorf("the version's note is missing: %s", job.Log)
	}
	ai, _ := os.ReadFile(outs[1].Path)
	for _, s := range []string{`Uri="https://example.com/notes/notes-windows-arm64.appinstaller"`, `Uri="https://example.com/notes/notes-1.2.0-rc.1-windows-arm64.msix"`,
		`<MainPackage Name="com.example.Notes" Publisher="CN=A &amp; B, ` + UnsignedPublisherOID + `" Version="1.2.0.0" ProcessorArchitecture="arm64"`,
		`<OnLaunch HoursBetweenUpdateChecks="0"/>`, `<AutomaticBackgroundTask/>`} {
		if !strings.Contains(string(ai), s) {
			t.Errorf(".appinstaller lacks %s:\n%s", s, ai)
		}
	}
	if err := xml.Unmarshal(ai, new(struct{})); err != nil {
		t.Errorf(".appinstaller: %v", err)
	}

	// Determinism: the same job gives the same bytes.
	job2 := job
	job2.OutDir = t.TempDir()
	outs2, err := f.Write(job2)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(outs2[0].Path)
	if !bytes.Equal(b, b2) {
		t.Errorf("two packages of the same job differ")
	}

	// Without an icon, placeholder tiles; without a URL, no .appinstaller.
	app.Icon = nil
	_, outs = writeJob(t, NewMSIX(MSIXOptions{Signed: true}), app, "amd64")
	if len(outs) != 1 || !strings.Contains(outs[0].Note, "-appinstaller-url") || !strings.Contains(outs[0].Note, "must be signed") {
		t.Errorf("outputs %+v", outs)
	}
	b, _ = os.ReadFile(outs[0].Path)
	if _, data := readMSIX(t, b); !bytes.HasPrefix(data["Assets/StoreLogo.png"], []byte("\x89PNG")) {
		t.Errorf("no placeholder tile")
	}
}

func TestOPCName(t *testing.T) {
	for in, want := range map[string]string{"a/b c.txt": "a/b%20c.txt", "notes+x.exe": "notes+x.exe", "é.png": "%C3%A9.png", "50%.txt": "50%25.txt"} {
		if got := opcName(in); got != want {
			t.Errorf("opcName(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := writeMSIX(time.Time{}, []opcPart{{name: "AppxManifest.xml"}}, nil); err == nil {
		t.Errorf("a payload file named AppxManifest.xml was accepted")
	}
	// An empty file, and one of exactly one block.
	b, err := writeMSIX(time.Time{}, []opcPart{{name: "empty.txt", deflate: true}, {name: "one.bin", data: make([]byte, blockSize), deflate: true}}, []byte("<Package/>"))
	if err != nil {
		t.Fatal(err)
	}
	readMSIX(t, b)
}

func TestZipFormat(t *testing.T) {
	app := notesApp(t)
	app.Version = "1.2.0"
	job, outs := writeJob(t, zipFormat{}, app, "amd64")
	if len(outs) != 1 || filepath.Base(outs[0].Path) != "notes-1.2.0-windows-amd64.zip" || outs[0].Note != "" {
		t.Fatalf("outputs %+v", outs)
	}
	b, _ := os.ReadFile(outs[0].Path)
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range zr.File {
		got = append(got, f.Name+" "+f.Mode().String())
		if !f.Modified.Equal(job.Time) {
			t.Errorf("%s: time %v", f.Name, f.Modified)
		}
	}
	if want := []string{"notes/ drwxr-xr-x", "notes/notes.exe -rwxr-xr-x", "notes/tela-install.json -rw-r--r--"}; !reflect.DeepEqual(got, want) {
		t.Errorf("entries %q, want %q", got, want)
	}
	r, _ := zr.Open("notes/" + update.MarkerName)
	var m update.Marker
	if err := json.NewDecoder(r).Decode(&m); err != nil {
		t.Fatal(err)
	}
	want := update.Marker{Schema: 1, Format: "zip", AppID: "com.example.Notes", Version: "1.2.0", Exe: "notes.exe", Files: []string{"notes.exe", "tela-install.json"}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("marker %+v", m)
	}
	job.OutDir = t.TempDir()
	job.ProgramSigned = true
	outs2, _ := zipFormat{}.Write(job)
	b2, _ := os.ReadFile(outs2[0].Path)
	if !bytes.Equal(b, b2) {
		t.Errorf("two zips of the same job differ")
	}
	if outs2[0].Note != "the program in it is signed" {
		t.Errorf("note %q", outs2[0].Note)
	}
}

func TestSigntool(t *testing.T) {
	s := Signtool{CertFile: `C:\c.pfx`, PasswordFile: "p"}
	if got := strings.Join(s.Args("a.exe", "pw"), " "); got != `sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 /f C:\c.pfx /p pw a.exe` {
		t.Errorf("args %s", got)
	}
	if got := strings.Join((Signtool{CertSHA1: "AB12", TimestampURL: "http://ts"}).Args("a.msix", ""), " "); got != "sign /fd SHA256 /tr http://ts /td SHA256 /sha1 AB12 a.msix" {
		t.Errorf("args %s", got)
	}
	if got := (Signtool{}).Args("a.exe", ""); got[len(got)-2] != "/a" {
		t.Errorf("args %v", got)
	}
	// Files that are not signtool's are left alone without running it.
	for _, tc := range []struct{ path, what, goos string }{{"a.zip", SignPackage, "windows"}, {"a.exe", SignPackage, "windows"}, {"a.msix", SignProgram, "windows"}, {"a.exe", SignProgram, "linux"}} {
		if ok, err := (Signtool{Path: "/nonexistent/signtool"}).Sign(tc.path, tc.what, Target{tc.goos, "amd64"}); ok || err != nil {
			t.Errorf("%+v: %v %v", tc, ok, err)
		}
	}
	if !versionLess("10.0.9.0", "10.0.22621.0") || versionLess("10.0.22621.0", "10.0.19041.0") {
		t.Errorf("versionLess")
	}
}
