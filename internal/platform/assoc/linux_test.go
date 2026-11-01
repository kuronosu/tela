package assoc

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// linuxEnv is a throwaway XDG tree: the user's directories and one system
// data and config directory, with the commands recorded.
type linuxEnv struct {
	t    *testing.T
	dirs LinuxDirs
	runs []string
	a    platform.Associations
}

func newLinuxEnv(t *testing.T) *linuxEnv {
	t.Helper()
	root := t.TempDir()
	e := &linuxEnv{t: t}
	e.dirs = LinuxDirs{
		DataHome:   filepath.Join(root, "home", "data"),
		ConfigHome: filepath.Join(root, "home", "config"),
		DataDirs:   []string{filepath.Join(root, "usr", "share")},
		ConfigDirs: []string{filepath.Join(root, "etc", "xdg")},
		Run: func(name string, args ...string) error {
			e.runs = append(e.runs, name+" "+strings.Join(args, " "))
			return errors.New("ignored")
		},
	}
	e.a = Linux(e.dirs)
	return e
}

func (e *linuxEnv) write(path, content string) {
	e.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func (e *linuxEnv) read(path string) string {
	e.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func (e *linuxEnv) ownEntry() string {
	return filepath.Join(e.dirs.DataHome, "applications", notesApp.ID+".desktop")
}

func (e *linuxEnv) ownPackage() string {
	return filepath.Join(e.dirs.DataHome, "mime", "packages", notesApp.ID+".xml")
}

func (e *linuxEnv) userMimeapps() string { return filepath.Join(e.dirs.ConfigHome, "mimeapps.list") }

func (e *linuxEnv) status(a platform.Association) platform.AssociationStatus {
	e.t.Helper()
	s, err := e.a.Status(notesApp, a)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func (e *linuxEnv) register(a platform.Association, replace bool) (bool, error) {
	return e.a.Register(notesApp, a, platform.RegisterOptions{Replace: replace})
}

// snapshot returns every file below the tree with its content, to check
// that a call wrote nothing.
func (e *linuxEnv) snapshot() map[string]string {
	e.t.Helper()
	out := map[string]string{}
	root := filepath.Dir(filepath.Dir(e.dirs.DataHome))
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = fi.ModTime().String() + "\x00" + string(b)
		}
		return nil
	})
	return out
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestLinuxRegisterScheme(t *testing.T) {
	e := newLinuxEnv(t)
	if s := e.status(scheme); s != (platform.AssociationStatus{}) {
		t.Errorf("initial status = %+v", s)
	}
	changed, err := e.register(scheme, false)
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	want := `# Written by Tela (platform.Associations): the URL schemes and file types
# this app handles. Unregister removes it.
[Desktop Entry]
Type=Application
Version=1.5
Name=Notes
Exec="/opt/notes/notes" %u
Terminal=false
NoDisplay=true
MimeType=x-scheme-handler/notes;
X-Tela-Registered=true
`
	if got := e.read(e.ownEntry()); got != want {
		t.Errorf("desktop entry:\n%s\nwant:\n%s", got, want)
	}
	if len(e.runs) != 0 {
		t.Errorf("commands = %q", e.runs)
	}
	if got := e.read(filepath.Join(e.dirs.DataHome, "applications", "mimeinfo.cache")); got != "[MIME Cache]\nx-scheme-handler/notes=com.example.Notes.desktop;\n" {
		t.Errorf("mimeinfo.cache:\n%s", got)
	}
	if exists(e.userMimeapps()) {
		t.Errorf("Register wrote mimeapps.list")
	}
	// The sole candidate is the default.
	if s := e.status(scheme); !s.Registered || !s.Default || s.Other != "" {
		t.Errorf("status after Register = %+v", s)
	}
	// Look before writing.
	before, runs := e.snapshot(), len(e.runs)
	if changed, err := e.register(scheme, false); changed || err != nil {
		t.Errorf("registering again = %v, %v", changed, err)
	}
	if !sameSnapshot(before, e.snapshot()) || len(e.runs) != runs {
		t.Errorf("registering again wrote something or ran a command")
	}
	// The scheme is compared without regard to case.
	if s := e.status(platform.Association{Scheme: "NOTES"}); !s.Registered {
		t.Errorf("status of NOTES = %+v", s)
	}
	// A moved executable is a change.
	moved := notesApp
	moved.Exec = "/usr/bin/notes"
	if changed, _ := e.a.Register(moved, scheme, platform.RegisterOptions{}); !changed {
		t.Errorf("a new Exec changed nothing")
	}
	if !strings.Contains(e.read(e.ownEntry()), "Exec=\"/usr/bin/notes\" %u\n") {
		t.Errorf("the entry has the old Exec:\n%s", e.read(e.ownEntry()))
	}
}

func TestLinuxRegisterFileType(t *testing.T) {
	e := newLinuxEnv(t)
	if _, err := e.register(scheme, false); err != nil {
		t.Fatal(err)
	}
	e.runs = nil
	if changed, err := e.register(noteFile, false); !changed || err != nil {
		t.Fatalf("Register(file) = %v, %v", changed, err)
	}
	entry := e.read(e.ownEntry())
	if !strings.Contains(entry, "MimeType=x-scheme-handler/notes;application/x-notes-note;\n") ||
		!strings.Contains(entry, "Exec=\"/opt/notes/notes\" %U\n") {
		t.Errorf("desktop entry with both:\n%s", entry)
	}
	wantPkg := `<?xml version="1.0" encoding="UTF-8"?>
<!-- Written by Tela for com.example.Notes (platform.Associations); removed by Unregister. -->
<mime-info xmlns="http://www.freedesktop.org/standards/shared-mime-info">
  <mime-type type="application/x-notes-note">
    <comment>Notes document</comment>
    <glob pattern="*.note"/>
  </mime-type>
</mime-info>
`
	if got := e.read(e.ownPackage()); got != wantPkg {
		t.Errorf("MIME package:\n%s\nwant:\n%s", got, wantPkg)
	}
	if !slices.Equal(e.runs, []string{"update-mime-database " + filepath.Join(e.dirs.DataHome, "mime")}) {
		t.Errorf("commands = %q", e.runs)
	}
	if s := e.status(noteFile); !s.Registered || !s.Default {
		t.Errorf("status = %+v", s)
	}
	// A second extension of the same type shares its definition.
	notes2 := noteFile
	notes2.Extension = ".NOTES"
	if _, err := e.register(notes2, false); err != nil {
		t.Fatal(err)
	}
	if got := e.read(e.ownPackage()); !strings.Contains(got, `<glob pattern="*.note"/>`+"\n"+`    <glob pattern="*.notes"/>`) {
		t.Errorf("second extension:\n%s", got)
	}
	if entry := e.read(e.ownEntry()); strings.Count(entry, "application/x-notes-note") != 1 {
		t.Errorf("the type is listed twice:\n%s", entry)
	}
	// Unregistering one extension keeps the type for the other.
	if changed, _ := e.a.Unregister(notesApp, notes2); !changed {
		t.Errorf("Unregister(.notes) changed nothing")
	}
	if got := e.read(e.ownPackage()); strings.Contains(got, "*.notes") || !strings.Contains(got, "*.note\"") {
		t.Errorf("after Unregister(.notes):\n%s", got)
	}
	// Unregistering the type removes it from the entry and the package.
	if changed, _ := e.a.Unregister(notesApp, noteFile); !changed {
		t.Errorf("Unregister(.note) changed nothing")
	}
	if exists(e.ownPackage()) {
		t.Errorf("the empty package was kept")
	}
	entry = e.read(e.ownEntry())
	if strings.Contains(entry, "x-notes-note") || !strings.Contains(entry, "Exec=\"/opt/notes/notes\" %u\n") {
		t.Errorf("entry after Unregister(.note):\n%s", entry)
	}
	// The last type removes the entry.
	if changed, _ := e.a.Unregister(notesApp, scheme); !changed || exists(e.ownEntry()) {
		t.Errorf("Unregister of the last type = %v; entry exists %v", changed, exists(e.ownEntry()))
	}
	before := e.snapshot()
	if changed, err := e.a.Unregister(notesApp, scheme); changed || err != nil {
		t.Errorf("Unregister of nothing = %v, %v", changed, err)
	}
	if !sameSnapshot(before, e.snapshot()) {
		t.Errorf("Unregister of nothing wrote something")
	}
}

func TestLinuxOtherApplications(t *testing.T) {
	e := newLinuxEnv(t)
	sys := filepath.Join(e.dirs.DataDirs[0], "applications")
	other := "[Desktop Entry]\nType=Application\nName=Other\nExec=other %u\nMimeType=x-scheme-handler/notes;application/x-notes-note;\n"
	e.write(filepath.Join(sys, "org.other.Editor.desktop"), other)
	e.write(filepath.Join(e.dirs.DataDirs[0], "mime", "globs2"), "# comment\n50:application/x-notes-note:*.note\n50:text/markdown:*.md\n")

	// Another candidate, no explicit default: neither is the default.
	if s := e.status(scheme); s.Registered || s.Default || s.Other != "org.other.Editor.desktop" {
		t.Errorf("status beside another candidate = %+v", s)
	}
	if changed, err := e.register(scheme, false); !changed || err != nil {
		t.Fatalf("Register beside another app = %v, %v", changed, err)
	}
	if s := e.status(scheme); !s.Registered || s.Default || s.Other != "org.other.Editor.desktop" {
		t.Errorf("status of one of two candidates = %+v", s)
	}
	// The other app's files are untouched.
	if e.read(filepath.Join(sys, "org.other.Editor.desktop")) != other {
		t.Errorf("the other app's entry changed")
	}

	// A type the system already knows needs no MIME package.
	if _, err := e.register(noteFile, false); err != nil {
		t.Fatal(err)
	}
	if exists(e.ownPackage()) {
		t.Errorf("a package was written for a known type")
	}

	// A system default for another app is reported and left alone.
	e.write(filepath.Join(e.dirs.ConfigDirs[0], "mimeapps.list"), "[Default Applications]\nx-scheme-handler/notes=missing.desktop;org.other.Editor.desktop;\n")
	if s := e.status(scheme); s.Default || s.Other != "org.other.Editor.desktop" {
		t.Errorf("status beside a system default = %+v", s)
	}

	// MakeDefault writes the user's mimeapps.list, keeping the rest.
	user := "# my associations\n[Added Associations]\ntext/plain=gedit.desktop;\n\n[Default Applications]\ntext/plain=gedit.desktop;\nx-scheme-handler/notes=org.other.Editor.desktop;\n"
	e.write(e.userMimeapps(), user)
	if r, err := e.a.MakeDefault(notesApp, scheme); r != platform.DefaultSet || err != nil {
		t.Fatalf("MakeDefault = %v, %v", r, err)
	}
	wantUser := strings.Replace(user, "notes=org.other.Editor.desktop;", "notes=com.example.Notes.desktop;org.other.Editor.desktop;", 1)
	if got := e.read(e.userMimeapps()); got != wantUser {
		t.Errorf("mimeapps.list:\n%s\nwant:\n%s", got, wantUser)
	}
	if s := e.status(scheme); !s.Registered || !s.Default || s.Other != "" {
		t.Errorf("status after MakeDefault = %+v", s)
	}
	// Again: nothing to write.
	before := e.snapshot()
	if _, err := e.a.MakeDefault(notesApp, scheme); err != nil {
		t.Fatal(err)
	}
	if !sameSnapshot(before, e.snapshot()) {
		t.Errorf("a second MakeDefault wrote something")
	}
	// MakeDefault of a file type adds the group's key.
	if _, err := e.a.MakeDefault(notesApp, noteFile); err != nil {
		t.Fatal(err)
	}
	if got := e.read(e.userMimeapps()); !strings.Contains(got, "x-scheme-handler/notes=com.example.Notes.desktop;org.other.Editor.desktop;\napplication/x-notes-note=com.example.Notes.desktop;\n") {
		t.Errorf("mimeapps.list with the file type:\n%s", got)
	}

	// Unregister removes the app's defaults and nothing else.
	if _, err := e.a.Unregister(notesApp, scheme); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.Unregister(notesApp, noteFile); err != nil {
		t.Fatal(err)
	}
	if got := e.read(e.userMimeapps()); got != user {
		t.Errorf("mimeapps.list after Unregister:\n%s\nwant:\n%s", got, user)
	}
	if exists(e.ownEntry()) {
		t.Errorf("the entry was kept")
	}
	if s := e.status(scheme); s.Registered || s.Default || s.Other != "org.other.Editor.desktop" {
		t.Errorf("status after Unregister = %+v", s)
	}
}

func TestLinuxConflicts(t *testing.T) {
	e := newLinuxEnv(t)
	// A user entry of the app's ID that Tela did not write.
	foreign := "[Desktop Entry]\nType=Application\nName=Notes (hand-made)\nExec=notes\n"
	e.write(e.ownEntry(), foreign)
	before := e.snapshot()
	_, err := e.register(scheme, false)
	if !errors.Is(err, platform.ErrAssociationTaken) || !strings.Contains(err.Error(), strconv.Quote(e.ownEntry())) {
		t.Errorf("Register over a foreign entry = %v", err)
	}
	if !sameSnapshot(before, e.snapshot()) || len(e.runs) != 0 {
		t.Errorf("a refused Register wrote something")
	}
	if s := e.status(scheme); s.Registered || s.Other != e.ownEntry() {
		t.Errorf("status beside a foreign entry = %+v", s)
	}
	if _, err := e.a.MakeDefault(notesApp, scheme); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("MakeDefault over a foreign entry = %v", err)
	}
	if exists(e.userMimeapps()) {
		t.Errorf("a refused MakeDefault wrote mimeapps.list")
	}
	if changed, err := e.register(scheme, true); !changed || err != nil {
		t.Errorf("Register with Replace = %v, %v", changed, err)
	}
	if got := e.read(e.ownEntry()); !strings.Contains(got, telaKey+"=true") {
		t.Errorf("Replace did not take the entry over:\n%s", got)
	}

	// A foreign entry that already lists the type: registered as it is.
	e2 := newLinuxEnv(t)
	sysEntry := filepath.Join(e2.dirs.DataDirs[0], "applications", notesApp.ID+".desktop")
	e2.write(sysEntry, "[Desktop Entry]\nType=Application\nName=Notes\nExec=notes %u\nMimeType=x-scheme-handler/notes;\n")
	if changed, err := e2.register(scheme, false); changed || err != nil {
		t.Errorf("Register of a type the system entry lists = %v, %v", changed, err)
	}
	if exists(e2.ownEntry()) {
		t.Errorf("a user entry now shadows the system one")
	}
	if s := e2.status(scheme); !s.Registered || !s.Default {
		t.Errorf("status of a system registration = %+v", s)
	}
	// It does not list the file type: the user's entry would shadow it.
	if _, err := e2.register(noteFile, false); !errors.Is(err, platform.ErrAssociationTaken) || !strings.Contains(err.Error(), strconv.Quote(sysEntry)) {
		t.Errorf("Register that would shadow the system entry = %v", err)
	}
	// Unregister never touches it.
	if changed, err := e2.a.Unregister(notesApp, scheme); changed || err != nil {
		t.Errorf("Unregister of the system's registration = %v, %v", changed, err)
	}

	// An extension another type claims.
	e3 := newLinuxEnv(t)
	e3.write(filepath.Join(e3.dirs.DataDirs[0], "mime", "globs2"), "50:text/markdown:*.md\n")
	md := platform.Association{Extension: ".md", MIMEType: "application/x-notes-note", Description: "Notes document"}
	_, err = e3.register(md, false)
	if !errors.Is(err, platform.ErrAssociationTaken) || !strings.Contains(err.Error(), "text/markdown") {
		t.Errorf("Register of a claimed extension = %v", err)
	}
	if exists(e3.ownEntry()) || exists(e3.ownPackage()) {
		t.Errorf("a refused Register wrote files")
	}
	if s := e3.status(md); s.Registered || s.Other != "text/markdown" {
		t.Errorf("status of a claimed extension = %+v", s)
	}
	if _, err := e3.register(md, true); err != nil {
		t.Fatal(err)
	}
	if got := e3.read(e3.ownPackage()); !strings.Contains(got, `<glob pattern="*.md" weight="80"/>`) {
		t.Errorf("package with Replace:\n%s", got)
	}
	if s := e3.status(md); !s.Registered {
		t.Errorf("status after Replace = %+v", s)
	}
	if changed, _ := e3.register(md, false); changed {
		t.Errorf("registering a taken-over extension again changed something")
	}
	// Another user package (not compiled yet) claims an extension too.
	e3.write(filepath.Join(e3.dirs.DataHome, "mime", "packages", "other.xml"),
		`<?xml version="1.0"?><mime-info xmlns="`+mimeNS+`"><mime-type type="text/x-other"><glob pattern="*.oth"/></mime-type></mime-info>`)
	oth := platform.Association{Extension: ".oth", MIMEType: "application/x-notes-note", Description: "Notes document"}
	if _, err := e3.register(oth, false); !errors.Is(err, platform.ErrAssociationTaken) || !strings.Contains(err.Error(), "text/x-other") {
		t.Errorf("Register of an extension a user package claims = %v", err)
	}
}

func TestLinuxDesktopSpecificDefault(t *testing.T) {
	e := newLinuxEnv(t)
	e.dirs.Desktops = []string{"gnome"}
	e.a = Linux(e.dirs)
	e.write(filepath.Join(e.dirs.DataDirs[0], "applications", "org.other.Editor.desktop"), "[Desktop Entry]\nType=Application\nName=Other\nExec=other %u\nMimeType=x-scheme-handler/notes;\n")
	gnome := filepath.Join(e.dirs.ConfigHome, "gnome-mimeapps.list")
	e.write(gnome, "[Default Applications]\nx-scheme-handler/notes=org.other.Editor.desktop\n")
	if _, err := e.a.MakeDefault(notesApp, scheme); err == nil || !strings.Contains(err.Error(), gnome) {
		t.Errorf("MakeDefault under a desktop-specific default = %v", err)
	}
	if exists(e.userMimeapps()) {
		t.Errorf("mimeapps.list was written")
	}
	// Unregister removes the app from the desktop-specific file too.
	e.write(gnome, "[Default Applications]\nx-scheme-handler/notes=com.example.Notes.desktop;org.other.Editor.desktop;\n")
	if s := e.status(scheme); !s.Default {
		t.Errorf("status with the desktop-specific default = %+v", s)
	}
	if changed, _ := e.a.Unregister(notesApp, scheme); !changed {
		t.Errorf("Unregister changed nothing")
	}
	if got := e.read(gnome); got != "[Default Applications]\nx-scheme-handler/notes=org.other.Editor.desktop;\n" {
		t.Errorf("gnome-mimeapps.list after Unregister:\n%s", got)
	}
}

func TestLinuxSymlinkedMimeapps(t *testing.T) {
	e := newLinuxEnv(t)
	target := filepath.Join(t.TempDir(), "dotfiles", "mimeapps.list")
	e.write(target, "[Default Applications]\n")
	if err := os.MkdirAll(e.dirs.ConfigHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, e.userMimeapps()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.a.MakeDefault(notesApp, scheme); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(e.userMimeapps()); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("mimeapps.list is no longer a link")
	}
	if got := e.read(target); got != "[Default Applications]\nx-scheme-handler/notes=com.example.Notes.desktop;\n" {
		t.Errorf("the link's target:\n%s", got)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(target), ".*tmp*")); len(left) != 0 {
		t.Errorf("temporary files left: %q", left)
	}
}

func TestLinuxNoDataHome(t *testing.T) {
	a := Linux(LinuxDirs{})
	if _, err := a.Register(notesApp, scheme, platform.RegisterOptions{}); err == nil {
		t.Errorf("Register without DataHome succeeded")
	}
	if _, err := Linux(LinuxDirs{DataHome: t.TempDir()}).MakeDefault(notesApp, scheme); err == nil {
		t.Errorf("MakeDefault without ConfigHome succeeded")
	}
}

// parseExec splits an Exec value as the Desktop Entry specification
// says: the string escapes first, then the quoting rules; it returns the
// arguments and the field code.
func parseExec(t *testing.T, raw string) []string {
	t.Helper()
	v := unescapeValue(raw)
	var args []string
	for i := 0; i < len(v); {
		switch {
		case v[i] == ' ':
			i++
		case v[i] == '"':
			var b strings.Builder
			i++
			for ; i < len(v) && v[i] != '"'; i++ {
				if v[i] == '\\' && i+1 < len(v) && strings.IndexByte("\"`$\\", v[i+1]) >= 0 {
					i++
				}
				b.WriteByte(v[i])
			}
			if i == len(v) {
				t.Fatalf("unterminated quote in %q", v)
			}
			i++
			args = append(args, strings.ReplaceAll(b.String(), "%%", "%"))
		default:
			j := strings.IndexByte(v[i:], ' ')
			if j < 0 {
				j = len(v) - i
			}
			args = append(args, v[i:i+j])
			i += j
		}
	}
	return args
}

func TestExecQuoting(t *testing.T) {
	app := platform.HandlerApp{
		ID:   "com.example.Notes",
		Name: " Notes\tapp",
		Exec: `/opt/my "notes"/bin\notes $HOME`,
		Args: []string{"--open", "100%", "a`b", "x;y", "line\nbreak", "~#*?()<>|&"},
	}
	exec := execValue(app, []string{"x-scheme-handler/notes"})
	want := append(append([]string{app.Exec}, app.Args...), "%u")
	if got := parseExec(t, exec); !slices.Equal(got, want) {
		t.Errorf("Exec %s parses as %q, want %q", exec, got, want)
	}
	if !strings.Contains(exec, `bin\\\\notes`) {
		t.Errorf("a literal backslash is not four backslashes in %s", exec)
	}
	if got := unescapeValue(escapeValue(app.Name)); got != app.Name {
		t.Errorf("Name round trip = %q", got)
	}
	if got := splitList(joinList([]string{"a;b.desktop", "c.desktop"})); !slices.Equal(got, []string{"a;b.desktop", "c.desktop"}) {
		t.Errorf("list round trip = %q", got)
	}
}

func TestKeyFileEdit(t *testing.T) {
	kf := parseKeyFile([]byte("# c\n[A]\nx = 1\n\n[B]\ny=2\n"))
	if v, ok := kf.get("A", "X"); !ok || v != "1" {
		t.Errorf("get = %q, %v", v, ok)
	}
	kf.set("A", "z", "3")
	kf.set("B", "y", "4")
	kf.set("C", "w", "5")
	if !kf.del("A", "x") || kf.del("A", "missing") {
		t.Errorf("del")
	}
	if got := string(kf.bytes()); got != "# c\n[A]\nz=3\n\n[B]\ny=4\n\n[C]\nw=5\n" {
		t.Errorf("edited:\n%s", got)
	}
}

// TestLinuxWithSystemTools runs the real update-desktop-database and
// update-mime-database on the temporary directories, and asks GIO
// (gio mime) and xdg-utils (xdg-mime) what they make of the result, with
// the environment pointing at those directories only. Each tool is
// optional; the test skips what is not installed.
func TestLinuxWithSystemTools(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux tools")
	}
	if testing.Short() {
		t.Skip("runs external programs")
	}
	root := t.TempDir()
	dirs := LinuxDirs{
		DataHome:   filepath.Join(root, "data"),
		ConfigHome: filepath.Join(root, "config"),
		DataDirs:   []string{filepath.Join(root, "sys")},
		ConfigDirs: []string{filepath.Join(root, "etc")},
	}
	a := Linux(dirs)
	// GIO ignores an entry whose executable does not exist.
	app := notesApp
	app.Exec = filepath.Join(root, "bin", "my notes")
	if err := os.MkdirAll(filepath.Dir(app.Exec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(app.Exec, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Register(app, scheme, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Register(app, noteFile, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(dirs.DataHome, "applications", app.ID+".desktop")
	if exe, err := exec.LookPath("desktop-file-validate"); err == nil {
		if out, err := exec.Command(exe, entry).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "" {
			t.Errorf("desktop-file-validate: %v\n%s", err, out)
		}
	}
	// Our mimeinfo.cache is the one update-desktop-database writes.
	if exe, err := exec.LookPath("update-desktop-database"); err == nil {
		ours, _ := os.ReadFile(filepath.Join(dirs.DataHome, "applications", "mimeinfo.cache"))
		if err := exec.Command(exe, "-q", filepath.Join(dirs.DataHome, "applications")).Run(); err != nil {
			t.Fatal(err)
		}
		theirs, _ := os.ReadFile(filepath.Join(dirs.DataHome, "applications", "mimeinfo.cache"))
		if string(ours) != string(theirs) {
			t.Errorf("mimeinfo.cache:\n%s\nupdate-desktop-database writes:\n%s", ours, theirs)
		}
	}
	if _, err := exec.LookPath("update-mime-database"); err == nil {
		globs, err := os.ReadFile(filepath.Join(dirs.DataHome, "mime", "globs2"))
		if err != nil || !strings.Contains(string(globs), "application/x-notes-note:*.note") {
			t.Errorf("globs2 (%v):\n%s", err, globs)
		}
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + root,
		"XDG_DATA_HOME=" + dirs.DataHome,
		"XDG_CONFIG_HOME=" + dirs.ConfigHome,
		"XDG_DATA_DIRS=" + dirs.DataDirs[0],
		"XDG_CONFIG_DIRS=" + dirs.ConfigDirs[0],
		"DBUS_SESSION_BUS_ADDRESS=unix:path=/nonexistent/tela-test-bus",
		"GIO_USE_VFS=local",
	}
	if exe, err := exec.LookPath("gio"); err == nil {
		cmd := exec.Command(exe, "mime", "x-scheme-handler/notes")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "com.example.Notes.desktop") {
			t.Errorf("gio mime x-scheme-handler/notes: %v\n%s", err, out)
		}
		t.Logf("gio mime:\n%s", out)
		if _, err := exec.LookPath("update-mime-database"); err == nil {
			doc := filepath.Join(root, "doc.note")
			if err := os.WriteFile(doc, []byte("hello"), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd = exec.Command(exe, "info", "-a", "standard::content-type", doc)
			cmd.Env = env
			out, err = cmd.CombinedOutput()
			if err != nil || !strings.Contains(string(out), "application/x-notes-note") {
				t.Errorf("gio info of a .note file: %v\n%s", err, out)
			}
		}
	}
	if _, err := a.MakeDefault(app, scheme); err != nil {
		t.Fatal(err)
	}
	if exe, err := exec.LookPath("xdg-mime"); err == nil {
		cmd := exec.Command(exe, "query", "default", "x-scheme-handler/notes")
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(out)) != "com.example.Notes.desktop" {
			t.Errorf("xdg-mime query default: %v\n%s", err, out)
		}
	}
}
