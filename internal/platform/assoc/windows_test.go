package assoc

import (
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The Windows associations against a MemRegistry: the owner's conditions
// of ADR-209 and the Fake's behavior, without touching a real registry.

var winApp = platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: `C:\Program Files\Notes\notes.exe`}

func winSetup(t *testing.T) (*MemRegistry, platform.Associations) {
	t.Helper()
	r := &MemRegistry{}
	return r, Windows(r)
}

func mustStatus(t *testing.T, a platform.Associations, assoc platform.Association) platform.AssociationStatus {
	t.Helper()
	s, err := a.Status(winApp, assoc)
	if err != nil {
		t.Fatalf("Status(%v): %v", assoc, err)
	}
	return s
}

func regValue(r *MemRegistry, key, name string) string {
	v, _, _ := r.String(key, name)
	return v
}

func TestWindowsRegisterScheme(t *testing.T) {
	r, a := winSetup(t)
	if s := mustStatus(t, a, scheme); s != (platform.AssociationStatus{}) {
		t.Fatalf("initial status = %+v", s)
	}
	changed, err := a.Register(winApp, scheme, platform.RegisterOptions{})
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	for _, c := range []struct{ key, name, want string }{
		{`Software\Classes\notes`, "URL Protocol", ""},
		{`Software\Classes\notes`, "", "URL:Notes"},
		{`Software\Classes\notes\shell\open\command`, "", `"C:\Program Files\Notes\notes.exe" "%1"`},
		{`Software\Classes\notes\DefaultIcon`, "", `"C:\Program Files\Notes\notes.exe",0`},
		{`Software\Classes\notes`, markerAppID, "com.example.Notes"},
		{`Software\com.example.Notes\Capabilities\URLAssociations`, "notes", "notes"},
		{`Software\com.example.Notes\Capabilities`, "ApplicationName", "Notes"},
		{`Software\RegisteredApplications`, "com.example.Notes", `Software\com.example.Notes\Capabilities`},
	} {
		if v, ok, _ := r.String(c.key, c.name); !ok || v != c.want {
			t.Errorf("%s %q = %q (%v), want %q", c.key, c.name, v, ok, c.want)
		}
	}
	if r.Notified() != 1 {
		t.Errorf("the shell was told %d times, want once", r.Notified())
	}
	if s := mustStatus(t, a, scheme); !s.Registered || !s.Default || s.Other != "" {
		t.Errorf("status after Register = %+v", s)
	}
	// Look before writing: again, nothing changes and nothing is written.
	w, n := r.Writes(), r.Notified()
	if changed, err := a.Register(winApp, scheme, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("second Register = %v, %v", changed, err)
	}
	if r.Writes() != w || r.Notified() != n {
		t.Errorf("an unchanged Register wrote %d values and told the shell %d times", r.Writes()-w, r.Notified()-n)
	}
	// Only HKCU: HKLM is never written (a MemRegistry has no write path to
	// it), and no key outside Software exists.
	for _, line := range strings.Split(r.Dump(), "\n") {
		if !strings.HasPrefix(line, `Software\`) {
			t.Errorf("unexpected key %q", line)
		}
	}
	// A moved executable updates the command in place: the key is the app's.
	moved := winApp
	moved.Exec, moved.Args = `D:\apps\notes.exe`, []string{"--uri", "a b"}
	if changed, err := a.Register(moved, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register of a moved executable = %v, %v", changed, err)
	}
	if got := regValue(r, `Software\Classes\notes\shell\open\command`, ""); got != `"D:\apps\notes.exe" --uri "a b" "%1"` {
		t.Errorf("command = %q", got)
	}
}

func TestWindowsSchemeTaken(t *testing.T) {
	r, a := winSetup(t)
	other := `"C:\Other\other.exe" "%1"`
	_ = r.SetString(`Software\Classes\notes\shell\open\command`, "", other)
	_ = r.SetString(`Software\Classes\notes`, "URL Protocol", "")
	w := r.Writes()
	if s := mustStatus(t, a, scheme); s.Registered || s.Default || s.Other != `C:\Other\other.exe` {
		t.Errorf("status beside another app's scheme = %+v", s)
	}
	_, err := a.Register(winApp, scheme, platform.RegisterOptions{})
	if !errors.Is(err, platform.ErrAssociationTaken) || !strings.Contains(err.Error(), "other.exe") {
		t.Fatalf("Register over another app = %v", err)
	}
	if r.Writes() != w || r.Notified() != 0 {
		t.Errorf("a refused Register wrote something (%d writes, %d notifications)", r.Writes()-w, r.Notified())
	}
	if res, err := a.MakeDefault(winApp, scheme); !errors.Is(err, platform.ErrAssociationTaken) || res != platform.DefaultNone {
		t.Errorf("MakeDefault over another app = %v, %v", res, err)
	}
	if changed, err := a.Register(winApp, scheme, platform.RegisterOptions{Replace: true}); !changed || err != nil {
		t.Fatalf("Register with Replace = %v, %v", changed, err)
	}
	if s := mustStatus(t, a, scheme); !s.Registered || !s.Default {
		t.Errorf("status after Replace = %+v", s)
	}

	// A machine-wide registration is another app's too: a per-user key
	// would shadow it.
	r2, a2 := winSetup(t)
	r2.SetMachineString(`Software\Classes\notes\shell\open\command`, "", `"C:\Program Files\Else\else.exe" "%1"`)
	if _, err := a2.Register(winApp, scheme, platform.RegisterOptions{}); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("Register over a machine-wide scheme = %v", err)
	}
	if r2.Writes() != 0 {
		t.Errorf("a refused Register wrote something")
	}
	// The app's own machine-wide copy is not another app.
	r3, a3 := winSetup(t)
	r3.SetMachineString(`Software\Classes\notes\shell\open\command`, "", `"c:/program files/notes/notes.exe" "%1"`)
	if _, err := a3.Register(winApp, scheme, platform.RegisterOptions{}); err != nil {
		t.Errorf("Register beside its own machine-wide copy = %v", err)
	}

	// A key the app wrote, whose command another app then replaced, is
	// no longer the app's.
	r4, a4 := winSetup(t)
	_, _ = a4.Register(winApp, scheme, platform.RegisterOptions{})
	_ = r4.SetString(`Software\Classes\notes\shell\open\command`, "", `"C:\Usurper\u.exe" "%1"`)
	if s := mustStatus(t, a4, scheme); s.Registered || s.Other != `C:\Usurper\u.exe` {
		t.Errorf("status of a taken-over key = %+v", s)
	}
	if _, err := a4.Register(winApp, scheme, platform.RegisterOptions{}); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("Register over a taken-over key = %v", err)
	}
	// Unregister leaves it alone.
	if changed, _ := a4.Unregister(winApp, scheme); changed && regValue(r4, `Software\Classes\notes\shell\open\command`, "") == "" {
		t.Errorf("Unregister removed another app's scheme key")
	}
	if regValue(r4, `Software\Classes\notes\shell\open\command`, "") == "" {
		t.Errorf("Unregister removed another app's command")
	}

	// A key of the same executable without the markers (an installer's)
	// is adopted, not refused.
	r5, a5 := winSetup(t)
	_ = r5.SetString(`Software\Classes\notes\shell\open\command`, "", `"C:\Program Files\Notes\NOTES.EXE" "%1"`)
	if changed, err := a5.Register(winApp, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register over its own unmarked key = %v, %v", changed, err)
	}
}

func TestWindowsUserChoiceOfScheme(t *testing.T) {
	r, a := winSetup(t)
	_, _ = a.Register(winApp, scheme, platform.RegisterOptions{})
	uc := `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\notes\UserChoice`
	// The user chose another app for the link type.
	_ = r.SetString(uc, "ProgId", "OtherApp.notes")
	if s := mustStatus(t, a, scheme); !s.Registered || s.Default || s.Other != "OtherApp.notes" {
		t.Errorf("status under another choice = %+v", s)
	}
	if res, err := a.MakeDefault(winApp, scheme); res != platform.DefaultAsked || err != nil {
		t.Fatalf("MakeDefault under another choice = %v, %v", res, err)
	}
	if got := r.Opened(); len(got) != 1 || got[0] != "ms-settings:defaultapps" {
		t.Errorf("settings opened = %q", got)
	}
	// The user chose the app (the capabilities name the scheme as ProgID).
	_ = r.SetString(uc, "ProgId", "notes")
	if s := mustStatus(t, a, scheme); !s.Default || s.Other != "" {
		t.Errorf("status under the app's choice = %+v", s)
	}
	if res, err := a.MakeDefault(winApp, scheme); res != platform.DefaultSet || err != nil || len(r.Opened()) != 1 {
		t.Errorf("MakeDefault under the app's choice = %v, %v (opened %q)", res, err, r.Opened())
	}
	// No choice at all: the app's key is what Windows uses.
	_ = r.DeleteKey(uc)
	if res, err := a.MakeDefault(winApp, scheme); res != platform.DefaultSet || err != nil || len(r.Opened()) != 1 {
		t.Errorf("MakeDefault without a choice = %v, %v (opened %q)", res, err, r.Opened())
	}
}

func TestWindowsRegisterFileType(t *testing.T) {
	r, a := winSetup(t)
	changed, err := a.Register(winApp, noteFile, platform.RegisterOptions{})
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	for _, c := range []struct{ key, name, want string }{
		{`Software\Classes\com.example.Notes.note`, "", "Notes document"},
		{`Software\Classes\com.example.Notes.note\shell\open\command`, "", `"C:\Program Files\Notes\notes.exe" "%1"`},
		{`Software\Classes\com.example.Notes.note\DefaultIcon`, "", `"C:\Program Files\Notes\notes.exe",0`},
		{`Software\Classes\.note\OpenWithProgids`, "com.example.Notes.note", ""},
		{`Software\Classes\.note`, "", "com.example.Notes.note"}, // nothing else opens the type
		{`Software\com.example.Notes\Capabilities\FileAssociations`, ".note", "com.example.Notes.note"},
	} {
		if v, ok, _ := r.String(c.key, c.name); !ok || v != c.want {
			t.Errorf("%s %q = %q (%v), want %q", c.key, c.name, v, ok, c.want)
		}
	}
	if s := mustStatus(t, a, noteFile); !s.Registered || !s.Default {
		t.Errorf("status = %+v", s)
	}
	w := r.Writes()
	if changed, _ := a.Register(winApp, noteFile, platform.RegisterOptions{}); changed || r.Writes() != w {
		t.Errorf("registering again changed = %v, %d writes", changed, r.Writes()-w)
	}
	// An icon of the app is the handler's icon.
	icon := winApp
	icon.Icon = `C:\Program Files\Notes\note.ico`
	_, _ = a.Register(icon, noteFile, platform.RegisterOptions{})
	if got := regValue(r, `Software\Classes\com.example.Notes.note\DefaultIcon`, ""); got != icon.Icon {
		t.Errorf("DefaultIcon = %q", got)
	}
}

func TestWindowsFileTypeKeepsOtherDefault(t *testing.T) {
	// The extension names another ProgID: the app becomes a candidate and
	// the default stays, until Replace.
	r, a := winSetup(t)
	_ = r.SetString(`Software\Classes\.note`, "", "OtherEditor.note")
	w := r.Writes()
	if _, err := a.Register(winApp, noteFile, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := regValue(r, `Software\Classes\.note`, ""); got != "OtherEditor.note" {
		t.Fatalf("the extension's default was overwritten with %q", got)
	}
	if s := mustStatus(t, a, noteFile); !s.Registered || s.Default || s.Other != "OtherEditor.note" {
		t.Errorf("status beside another default = %+v", s)
	}
	if r.Writes() == w {
		t.Errorf("Register wrote nothing")
	}
	if _, err := a.Register(winApp, noteFile, platform.RegisterOptions{Replace: true}); err != nil {
		t.Fatal(err)
	}
	if got := regValue(r, `Software\Classes\.note`, ""); got != "com.example.Notes.note" {
		t.Errorf("Replace left the default at %q", got)
	}

	// A machine-wide default counts too: a per-user default would hide it.
	r2, a2 := winSetup(t)
	r2.SetMachineString(`Software\Classes\.note`, "", "SystemEditor.note")
	_, _ = a2.Register(winApp, noteFile, platform.RegisterOptions{})
	if _, ok, _ := r2.String(`Software\Classes\.note`, ""); ok {
		t.Errorf("a machine-wide default was shadowed")
	}
	if s := mustStatus(t, a2, noteFile); !s.Registered || s.Default || s.Other != "SystemEditor.note" {
		t.Errorf("status beside a machine-wide default = %+v", s)
	}
}

func TestWindowsMakeDefaultOfFileType(t *testing.T) {
	r, a := winSetup(t)
	uc := `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.note\UserChoice`
	_ = r.SetString(`Software\Classes\.note`, "", "OtherEditor.note")
	_ = r.SetString(uc, "ProgId", "OtherEditor.note")
	res, err := a.MakeDefault(winApp, noteFile)
	if res != platform.DefaultAsked || err != nil {
		t.Fatalf("MakeDefault = %v, %v", res, err)
	}
	if got := r.Opened(); len(got) != 1 || got[0] != "ms-settings:defaultapps" {
		t.Errorf("settings opened = %q", got)
	}
	if s := mustStatus(t, a, noteFile); s.Default || s.Other != "OtherEditor.note" {
		t.Errorf("an asked default changed the status: %+v", s)
	}
	// The user picks the app on the page.
	_ = r.SetString(uc, "ProgId", "com.example.Notes.note")
	if s := mustStatus(t, a, noteFile); !s.Registered || !s.Default || s.Other != "" {
		t.Errorf("status after the user's choice = %+v", s)
	}
	if res, _ := a.MakeDefault(winApp, noteFile); res != platform.DefaultSet || len(r.Opened()) != 1 {
		t.Errorf("MakeDefault when it is the default = %v (opened %q)", res, r.Opened())
	}
	// A settings page that cannot be opened is an error, not "asked".
	r.SetOpenError(errors.New("no shell"))
	_ = r.SetString(uc, "ProgId", "OtherEditor.note")
	if res, err := a.MakeDefault(winApp, noteFile); err == nil || res != platform.DefaultNone {
		t.Errorf("MakeDefault with a failing shell = %v, %v", res, err)
	}
}

func TestWindowsUnregister(t *testing.T) {
	r, a := winSetup(t)
	// Another app's entries and the app's own settings stay.
	_ = r.SetString(`Software\Classes\.txt`, "", "txtfile")
	_ = r.SetString(`Software\Classes\.note\OpenWithProgids`, "Other.note", "")
	_ = r.SetString(`Software\com.example.Notes`, "Theme", "dark")
	_ = r.SetString(`Software\RegisteredApplications`, "Other", `Software\Other\Capabilities`)
	before := r.Dump()
	for _, assoc := range []platform.Association{scheme, noteFile} {
		if _, err := a.Register(winApp, assoc, platform.RegisterOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	_ = r.SetString(`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\notes\UserChoice`, "ProgId", "notes")
	if changed, err := a.Unregister(winApp, scheme); !changed || err != nil {
		t.Fatalf("Unregister(scheme) = %v, %v", changed, err)
	}
	if s := mustStatus(t, a, scheme); s.Registered || s.Default {
		t.Errorf("status after Unregister = %+v", s)
	}
	// The capabilities stay while the file type needs them.
	if v, ok, _ := r.String(`Software\RegisteredApplications`, "com.example.Notes"); !ok || v == "" {
		t.Errorf("the registered application went with the scheme")
	}
	if changed, err := a.Unregister(winApp, noteFile); !changed || err != nil {
		t.Fatalf("Unregister(file type) = %v, %v", changed, err)
	}
	if after := r.Dump(); after != before {
		t.Errorf("Unregister left traces.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	w, n := r.Writes(), r.Notified()
	for _, assoc := range []platform.Association{scheme, noteFile} {
		if changed, err := a.Unregister(winApp, assoc); changed || err != nil {
			t.Errorf("Unregister of nothing = %v, %v", changed, err)
		}
	}
	if r.Writes() != w || r.Notified() != n {
		t.Errorf("Unregister of nothing wrote or notified")
	}
}

func TestWindowsRegistryError(t *testing.T) {
	boom := errors.New("access denied")
	a := Windows(failingRegistry{&MemRegistry{}, boom})
	if _, err := a.Status(winApp, scheme); !errors.Is(err, boom) {
		t.Errorf("Status = %v", err)
	}
	if changed, err := a.Register(winApp, scheme, platform.RegisterOptions{}); changed || !errors.Is(err, boom) {
		t.Errorf("Register = %v, %v", changed, err)
	}
}

type failingRegistry struct {
	*MemRegistry
	err error
}

func (f failingRegistry) String(string, string) (string, bool, error) { return "", false, f.err }

func TestWindowsChecksItsArguments(t *testing.T) {
	_, a := winSetup(t)
	mustPanic(t, "Register without Exec", func() {
		_, _ = a.Register(platform.HandlerApp{ID: "x", Name: "X"}, scheme, platform.RegisterOptions{})
	})
	mustPanic(t, "Status of an empty association", func() { _, _ = a.Status(winApp, platform.Association{}) })
	mustPanic(t, "Register of a standard scheme", func() {
		_, _ = a.Register(winApp, platform.Association{Scheme: "https"}, platform.RegisterOptions{})
	})
}

func TestWindowsNilRegistryOffWindows(t *testing.T) {
	// Windows(nil) is the real HKCU on Windows; elsewhere it is unsupported.
	if nativeRegistry != nil {
		t.Skip("running on Windows: Windows(nil) is the real registry")
	}
	if _, err := Windows(nil).Register(winApp, scheme, platform.RegisterOptions{}); !errors.Is(err, platform.ErrUnsupported) {
		t.Errorf("Windows(nil).Register = %v", err)
	}
}

func TestCommandLine(t *testing.T) {
	for _, tc := range []struct{ arg, want string }{
		{"plain", "plain"},
		{"", `""`},
		{"a b", `"a b"`},
		{`say "hi"`, `"say \"hi\""`},
		{`C:\dir\`, `C:\dir\`},
		{`C:\my dir\`, `"C:\my dir\\"`},
		{`a\"b`, `"a\\\"b"`},
	} {
		if got := quoteArg(tc.arg); got != tc.want {
			t.Errorf("quoteArg(%q) = %s, want %s", tc.arg, got, tc.want)
		}
	}
	for cmd, want := range map[string]string{
		`"C:\Program Files\a\a.exe" "%1"`: `C:\Program Files\a\a.exe`,
		`C:\a\a.exe %1`:                   `C:\a\a.exe`,
		`  "C:\a.exe"`:                    `C:\a.exe`,
		``:                                ``,
	} {
		if got := commandExe(cmd); got != want {
			t.Errorf("commandExe(%q) = %q, want %q", cmd, got, want)
		}
	}
	if !sameExe(`C:\A\b.exe`, `c:/a/B.EXE`) || sameExe(`C:\a.exe`, `C:\b.exe`) || sameExe("", "") {
		t.Errorf("sameExe")
	}
}
