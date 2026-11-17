package pkgfmt

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/cmd/tela/internal/appicon"
)

func TestPackageName(t *testing.T) {
	for _, tc := range []struct{ id, title, want string }{
		{"com.example.Notes", "Notes", "notes"},
		{"dev.telaui.hello", "Tela", "hello"},
		{"com.example.My_App", "", "my-app"},
		{"com.example.x", "Text Editor", "text-editor"},
		{"com.example.x", "", "app"},
		{"org.gnome.TextEditor", "", "texteditor"},
	} {
		if got := PackageName(tc.id, tc.title); got != tc.want {
			t.Errorf("PackageName(%q, %q) = %q, want %q", tc.id, tc.title, got, tc.want)
		}
	}
}

func TestDebianVersion(t *testing.T) {
	for v, want := range map[string]string{
		"1.2.3":      "1.2.3",
		"v1.2.3":     "1.2.3",
		"1.2.3-rc.1": "1.2.3~rc.1",
		"0.14.7-0.20261117184909-84c135c35505+dirty": "0.14.7~0.20261117184909.84c135c35505+dirty",
		"1.0.0+build_5": "1.0.0+build.5",
		"x":             "0~x",
	} {
		if got := DebianVersion(v); got != want {
			t.Errorf("DebianVersion(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestCheckFlatpakID(t *testing.T) {
	for id, ok := range map[string]bool{
		"com.example.Notes":     true,
		"org.example.my-app":    true,
		"dev.telaui.hello":      true,
		"example.notes":         false,
		"com.my-co.notes":       false,
		"com.example.1notes":    false,
		"com..notes":            false,
		"com.example.notes app": false,
	} {
		if err := CheckFlatpakID(id); (err == nil) != ok {
			t.Errorf("CheckFlatpakID(%q) = %v", id, err)
		}
	}
}

func TestCheckCategory(t *testing.T) {
	if CheckCategory("") != nil || CheckCategory("Office") != nil {
		t.Errorf("valid categories refused")
	}
	if err := CheckCategory("office"); err == nil || !strings.Contains(err.Error(), `"Office"`) {
		t.Errorf("office: %v", err)
	}
	if err := CheckCategory("Productivity"); err == nil || !strings.Contains(err.Error(), "Utility") {
		t.Errorf("Productivity: %v", err)
	}
}

func notesApp(t *testing.T) App {
	t.Helper()
	icon, err := appicon.Decode([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 4 4"><rect width="4" height="4" fill="red"/></svg>`), ".svg")
	if err != nil {
		t.Fatal(err)
	}
	return App{
		ID: "com.example.Notes", Name: "Notes\tPro", Package: "notes", Version: "1.0.0",
		Publisher: "A & B", Description: "Takes notes.", Category: "Office",
		Schemes: []string{"Notes", "notes"},
		FileTypes: []tela.FileType{
			{Ext: ".note", MIME: "application/x-note", Description: "Note"},
			{Ext: ".NOTE2", MIME: "application/x-note", Description: "Note"},
			{Ext: ".bak", MIME: "application/x-note-bak", Description: "Backup <old>", Role: tela.HandlerNone},
		},
		Icon: icon,
	}
}

func TestDesktopEntry(t *testing.T) {
	d := string(DesktopEntry(notesApp(t), "notes"))
	for _, want := range []string{"Name=Notes\\tPro\n", "Exec=notes %U\n", "Icon=com.example.Notes\n", "MimeType=application/x-note;x-scheme-handler/notes;\n"} {
		if !strings.Contains(d, want) {
			t.Errorf("lacks %q:\n%s", want, d)
		}
	}
	plain := App{ID: "a.b.c", Name: "C", Package: "c"}
	d = string(DesktopEntry(plain, "c"))
	if !strings.Contains(d, "Exec=c\n") || strings.Contains(d, "Icon=") || strings.Contains(d, "MimeType") || !strings.Contains(d, "Categories=Utility;\n") {
		t.Errorf("a plain app:\n%s", d)
	}
}

func TestXMLFiles(t *testing.T) {
	app := notesApp(t)
	for name, b := range map[string][]byte{"metainfo": Metainfo(app, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)), "mime": MIMEPackage(app)} {
		dec := xml.NewDecoder(bytes.NewReader(b))
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Errorf("%s is not well-formed XML: %v\n%s", name, err, b)
				break
			}
		}
	}
	m := string(MIMEPackage(app))
	if strings.Count(m, "<mime-type ") != 2 || !strings.Contains(m, `<glob pattern="*.note2"/>`) || !strings.Contains(m, "Backup &lt;old&gt;") {
		t.Errorf("MIME package:\n%s", m)
	}
	mi := string(Metainfo(app, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)))
	if !strings.Contains(mi, `date="2026-01-02"`) || !strings.Contains(mi, "<summary>Takes notes</summary>") || strings.Contains(mi, "x-note-bak") {
		t.Errorf("metainfo:\n%s", mi)
	}
	if MIMEPackage(App{}) != nil {
		t.Errorf("a MIME package without file types")
	}
}

func TestFinishArgs(t *testing.T) {
	args, notes := FinishArgs(App{Services: tela.Services{Network: true, Secrets: true, Microphone: "talk", Contacts: "who", Bluetooth: "pair"}})
	for _, want := range []string{"--share=network", "--talk-name=org.freedesktop.secrets", "--socket=pulseaudio", "--allow=bluetooth", "--socket=wayland"} {
		if !strings.Contains(strings.Join(args, " "), want) {
			t.Errorf("lacks %s: %v", want, args)
		}
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "Contacts") {
		t.Errorf("notes %v", notes)
	}
	if args, _ := FinishArgs(App{}); strings.Contains(strings.Join(args, " "), "network") {
		t.Errorf("network without Services.Network: %v", args)
	}
}

func TestFormats(t *testing.T) {
	for _, goos := range []string{"linux", "windows", "darwin"} {
		for _, n := range Defaults(goos) {
			f, ok := Lookup(n)
			if !ok || f.OS() != goos {
				t.Errorf("default %s of %s: %v", n, goos, ok)
			}
		}
	}
	f, _ := Lookup("msix")
	if err := f.Check(App{}, Target{"windows", "amd64"}); err == nil || !strings.Contains(err.Error(), "not implemented yet") {
		t.Errorf("msix: %v", err)
	}
	if got := FileName(App{Package: "notes", Version: "1.2.0"}, Target{"linux", "arm64"}, ".deb"); got != "notes-1.2.0-linux-arm64.deb" {
		t.Errorf("FileName %q", got)
	}
}

func TestArchives(t *testing.T) {
	when := time.Unix(1700000000, 0)
	ar := arArchive(when, []arMember{{"debian-binary", []byte("2.0\n")}, {"odd", []byte("abc")}})
	want := "!<arch>\n" +
		"debian-binary   1700000000  0     0     100644  4         `\n2.0\n" +
		"odd             1700000000  0     0     100644  3         `\nabc\n"
	if string(ar) != want {
		t.Errorf("ar:\n%q\nwant\n%q", ar, want)
	}
	a, err := tarGz([]file{{path: "usr/bin/x", mode: 0o755, data: []byte("x")}}, when)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := tarGz([]file{{path: "usr/bin/x", mode: 0o755, data: []byte("x")}}, when)
	if !bytes.Equal(a, b) {
		t.Errorf("two tars of the same files differ")
	}
}
