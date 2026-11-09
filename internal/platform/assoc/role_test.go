package assoc

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/plist"
)

// Handler roles (platform.HandlerRole) on every OS writer and the fake:
// the role is not part of a registration's identity, a Register with
// another role changes it, and HandlerNone declares a file type without
// making the app a handler.

func withRole(a platform.Association, r platform.HandlerRole) platform.Association {
	a.Role = r
	return a
}

var declaredFile = withRole(noteFile, platform.HandlerNone)

// --- Linux ---

// Editor, Viewer and Shell write the same entry; HandlerNone keeps the
// MIME package and takes the type out of the entry (and the user's
// default naming the app); another role brings it back.
func TestLinuxRoles(t *testing.T) {
	e := newLinuxEnv(t)
	if _, err := e.register(noteFile, false); err != nil {
		t.Fatal(err)
	}
	editor := e.snapshot()
	for _, r := range []platform.HandlerRole{platform.HandlerEditor, platform.HandlerViewer, platform.HandlerShell} {
		if changed, err := e.register(withRole(noteFile, r), false); changed || err != nil {
			t.Errorf("Register as %v = %v, %v; want nothing written", r, changed, err)
		}
	}
	if !sameSnapshot(editor, e.snapshot()) {
		t.Errorf("another role wrote something")
	}
	if r, err := e.a.MakeDefault(notesApp, noteFile); r != platform.DefaultSet || err != nil {
		t.Fatalf("MakeDefault = %v, %v", r, err)
	}
	pkg := e.read(e.ownPackage())

	if changed, err := e.register(declaredFile, false); !changed || err != nil {
		t.Fatalf("Register with HandlerNone = %v, %v", changed, err)
	}
	if exists(e.ownEntry()) {
		t.Errorf("the entry of the only type was kept:\n%s", e.read(e.ownEntry()))
	}
	if got := e.read(e.ownPackage()); got != pkg {
		t.Errorf("the MIME package changed:\n%s", got)
	}
	if got := e.read(e.userMimeapps()); strings.Contains(got, notesApp.ID) {
		t.Errorf("the default naming the app was kept:\n%s", got)
	}
	if s := e.status(noteFile); s.Registered || s.Default || s.Role != platform.HandlerNone {
		t.Errorf("status as a handler = %+v; want not registered", s)
	}
	if s := e.status(declaredFile); !s.Registered || s.Default {
		t.Errorf("status of the declaration = %+v", s)
	}
	before := e.snapshot()
	if changed, err := e.register(declaredFile, false); changed || err != nil {
		t.Errorf("Register with HandlerNone again = %v, %v", changed, err)
	}
	if !sameSnapshot(before, e.snapshot()) {
		t.Errorf("HandlerNone again wrote something")
	}

	// With a scheme in the entry, the entry stays without the type.
	if _, err := e.register(scheme, false); err != nil {
		t.Fatal(err)
	}
	if changed, err := e.register(withRole(noteFile, platform.HandlerViewer), false); !changed || err != nil {
		t.Fatalf("Register as a viewer = %v, %v", changed, err)
	}
	if entry := e.read(e.ownEntry()); !strings.Contains(entry, "MimeType=x-scheme-handler/notes;application/x-notes-note;\n") {
		t.Errorf("entry as a viewer:\n%s", entry)
	}
	if _, err := e.register(declaredFile, false); err != nil {
		t.Fatal(err)
	}
	if entry := e.read(e.ownEntry()); !strings.Contains(entry, "MimeType=x-scheme-handler/notes;\n") || !strings.Contains(entry, "Exec=/opt/notes/notes %u\n") {
		t.Errorf("entry after HandlerNone:\n%s", entry)
	}
	// Unregister addresses the type whatever the role.
	if changed, err := e.a.Unregister(notesApp, noteFile); !changed || err != nil || exists(e.ownPackage()) {
		t.Errorf("Unregister of the declaration = %v, %v; package exists %v", changed, err, exists(e.ownPackage()))
	}
}

// A declaration writes nothing to the app's entry, so an entry of the ID
// that Tela did not write is no conflict; an extension another MIME type
// claims still is.
func TestLinuxNoneConflicts(t *testing.T) {
	e := newLinuxEnv(t)
	const theirs = "[Desktop Entry]\nType=Application\nName=Notes\nExec=notes\n"
	e.write(e.ownEntry(), theirs)
	if changed, err := e.register(declaredFile, false); !changed || err != nil {
		t.Errorf("Register with HandlerNone beside another entry = %v, %v", changed, err)
	}
	if got := e.read(e.ownEntry()); got != theirs {
		t.Errorf("the other entry changed:\n%s", got)
	}
	if s := e.status(declaredFile); !s.Registered || s.Other != "" {
		t.Errorf("status of the declaration beside another entry = %+v", s)
	}
	if _, err := e.register(noteFile, false); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("Register as a handler beside another entry = %v; want ErrAssociationTaken", err)
	}

	f := newLinuxEnv(t)
	f.write(filepath.Join(f.dirs.DataDirs[0], "mime", "globs2"), "50:text/x-other:*.note\n")
	if _, err := f.register(declaredFile, false); !errors.Is(err, platform.ErrAssociationTaken) {
		t.Errorf("HandlerNone of a claimed extension = %v; want ErrAssociationTaken", err)
	}
	if s := f.status(declaredFile); s.Registered {
		t.Errorf("status of a claimed extension = %+v", s)
	}
}

// --- Windows ---

// Editor, Viewer and Shell write the open verb; HandlerNone keeps the
// ProgID's description and icon without the verb, the "Open with" entry
// and the capabilities, and drops a user's choice naming it.
func TestWindowsRoles(t *testing.T) {
	r, a := winSetup(t)
	const (
		progKey = `Software\Classes\com.example.Notes.note`
		extKey  = `Software\Classes\.note`
		choice  = `Software\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.note\UserChoice`
	)
	if _, err := a.Register(winApp, noteFile, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	editor := r.Dump()
	for _, role := range []platform.HandlerRole{platform.HandlerEditor, platform.HandlerViewer, platform.HandlerShell} {
		if changed, err := a.Register(winApp, withRole(noteFile, role), platform.RegisterOptions{}); changed || err != nil {
			t.Errorf("Register as %v = %v, %v; want nothing written", role, changed, err)
		}
	}
	if r.Dump() != editor {
		t.Errorf("another role wrote something")
	}
	if strings.Contains(editor, `\shell\edit`) {
		t.Errorf("an edit verb was written:\n%s", editor)
	}
	_ = r.SetString(choice, "ProgId", "com.example.Notes.note")

	n := r.Notified()
	if changed, err := a.Register(winApp, declaredFile, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register with HandlerNone = %v, %v", changed, err)
	}
	if r.Notified() != n+1 {
		t.Errorf("the shell was not told")
	}
	for _, c := range []struct{ key, name, want string }{
		{progKey, "", "Notes document"},
		{progKey + `\DefaultIcon`, "", `"C:\Program Files\Notes\notes.exe",0`},
		{extKey, "", "com.example.Notes.note"},
	} {
		if v, ok, _ := r.String(c.key, c.name); !ok || v != c.want {
			t.Errorf("%s %q = %q (%v), want %q", c.key, c.name, v, ok, c.want)
		}
	}
	dump := r.Dump()
	for _, gone := range []string{progKey + `\shell`, `OpenWithProgids`, `Capabilities`, `RegisteredApplications`, `UserChoice`} {
		if strings.Contains(dump, gone) {
			t.Errorf("%s left after HandlerNone:\n%s", gone, dump)
		}
	}
	if s := mustStatus(t, a, noteFile); s.Registered || s.Default || s.Role != platform.HandlerNone {
		t.Errorf("status as a handler = %+v; want not registered", s)
	}
	if s := mustStatus(t, a, declaredFile); !s.Registered || s.Default {
		t.Errorf("status of the declaration = %+v", s)
	}
	w := r.Writes()
	if changed, _ := a.Register(winApp, declaredFile, platform.RegisterOptions{}); changed || r.Writes() != w {
		t.Errorf("HandlerNone again changed = %v, %d writes", changed, r.Writes()-w)
	}
	// Another role brings the handler back.
	if changed, err := a.Register(winApp, withRole(noteFile, platform.HandlerViewer), platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register as a viewer = %v, %v", changed, err)
	}
	if got := r.Dump(); got != editor {
		t.Errorf("as a viewer:\n%s\nwant:\n%s", got, editor)
	}
	// Unregister of the declaration leaves nothing.
	_, _ = a.Register(winApp, declaredFile, platform.RegisterOptions{})
	if changed, err := a.Unregister(winApp, noteFile); !changed || err != nil {
		t.Errorf("Unregister of the declaration = %v, %v", changed, err)
	}
	if got := r.Dump(); got != "" {
		t.Errorf("Unregister left:\n%s", got)
	}
}

// --- macOS ---

// The role is the entry's CFBundleTypeRole; HandlerNone also sets
// LSHandlerRank None; another role changes Tela's entry, never the
// developer's.
func TestMacOSRoles(t *testing.T) {
	l, a, app := macSetup(t)
	docEntry := func() plist.Dict {
		t.Helper()
		info, _ := readInfo(t, app.Exec)
		docs := entriesOf(info, keyDocTypes)
		if len(docs) != 1 {
			t.Fatalf("CFBundleDocumentTypes = %v", info[keyDocTypes])
		}
		return docs[0].(plist.Dict)
	}
	if _, err := a.Register(app, noteFile, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if changed, _ := a.Register(app, withRole(noteFile, platform.HandlerEditor), platform.RegisterOptions{}); changed {
		t.Errorf("HandlerEditor is a file type's default role: Register wrote")
	}
	calls := l.calls()
	if changed, err := a.Register(app, withRole(noteFile, platform.HandlerViewer), platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register as a viewer = %v, %v", changed, err)
	}
	if d := docEntry(); d[keyRole] != "Viewer" || d[keyRank] != "Alternate" || !marked(d) {
		t.Errorf("viewer entry = %v", d)
	}
	if l.calls() != calls+1 {
		t.Errorf("Launch Services did not read the bundle again")
	}
	if s := macStatus(t, a, app, noteFile); !s.Registered || !s.Default || s.Role != platform.HandlerViewer {
		t.Errorf("status as a viewer = %+v", s)
	}
	if _, err := a.Register(app, withRole(noteFile, platform.HandlerShell), platform.RegisterOptions{}); err != nil || docEntry()[keyRole] != "Shell" {
		t.Errorf("shell entry = %v, %v", docEntry(), err)
	}

	if changed, err := a.Register(app, declaredFile, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register with HandlerNone = %v, %v", changed, err)
	}
	if d := docEntry(); d[keyRole] != "None" || d[keyRank] != "None" || fmt.Sprint(d["LSItemContentTypes"]) != "[com.example.Notes.note]" {
		t.Errorf("declaration entry = %v", d)
	}
	if s := macStatus(t, a, app, noteFile); s.Registered || s.Default || s.Role != platform.HandlerNone {
		t.Errorf("status as a handler = %+v; want not registered", s)
	}
	if s := macStatus(t, a, app, declaredFile); !s.Registered || s.Default {
		t.Errorf("status of the declaration = %+v", s)
	}
	_, raw := readInfo(t, app.Exec)
	if changed, err := a.Register(app, declaredFile, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("HandlerNone again = %v, %v", changed, err)
	}
	if _, raw2 := readInfo(t, app.Exec); string(raw) != string(raw2) {
		t.Errorf("HandlerNone again wrote")
	}

	// A scheme's role is its CFBundleURLTypes entry's; the developer's
	// entry keeps its own.
	links := platform.Association{Scheme: "notes"}
	if _, err := a.Register(app, links, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if changed, err := a.Register(app, withRole(links, platform.HandlerEditor), platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register of the scheme as an editor = %v, %v", changed, err)
	}
	info, _ := readInfo(t, app.Exec)
	if e := schemeEntry(info, "notes"); e[keyRole] != "Editor" || !marked(e) {
		t.Errorf("scheme entry = %v", e)
	}
	if changed, err := a.Register(app, platform.Association{Scheme: "notes-dev", Role: platform.HandlerEditor}, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("Register of the developer's scheme = %v, %v; want nothing written", changed, err)
	}
	info, _ = readInfo(t, app.Exec)
	if e := schemeEntry(info, "notes-dev"); e[keyRole] != nil {
		t.Errorf("the developer's entry changed: %v", e)
	}
	checkKept(t, info)

	if changed, err := a.Unregister(app, noteFile); !changed || err != nil {
		t.Errorf("Unregister of the declaration = %v, %v", changed, err)
	}
	if info, _ := readInfo(t, app.Exec); info[keyDocTypes] != nil || info[keyImported] != nil {
		t.Errorf("Unregister left %v, %v", info[keyDocTypes], info[keyImported])
	}
}

// An entry Tela wrote for several types gives the one whose role changes
// an entry of its own.
func TestMacOSRoleSplitsSharedEntry(t *testing.T) {
	d := plist.Dict{keyDocTypes: plist.Array{plist.Dict{
		"CFBundleTypeName":   "Docs",
		keyRole:              "Editor",
		keyRank:              docRank,
		"LSItemContentTypes": plist.Array{"a.one", "a.two"},
		markerKey:            true,
	}}}
	if !addDocType(d, "a.two", "two", "Two", "Viewer", docRank) {
		t.Fatal("addDocType changed nothing")
	}
	docs := entriesOf(d, keyDocTypes)
	if len(docs) != 2 {
		t.Fatalf("entries = %v", docs)
	}
	one, two := docs[0].(plist.Dict), docs[1].(plist.Dict)
	if fmt.Sprint(one["LSItemContentTypes"]) != "[a.one]" || one[keyRole] != "Editor" ||
		fmt.Sprint(two["LSItemContentTypes"]) != "[a.two]" || two[keyRole] != "Viewer" || !marked(two) {
		t.Errorf("split entries = %v, %v", one, two)
	}
	if addDocType(d, "a.two", "two", "Two", "Viewer", docRank) {
		t.Errorf("the same role again changed something")
	}
}

// --- the fake ---

func TestFakeRoles(t *testing.T) {
	f := &Fake{}
	if _, err := f.Register(notesApp, noteFile, platform.RegisterOptions{}); err != nil {
		t.Fatal(err)
	}
	if regs := f.Registrations(); len(regs) != 1 || regs[0].Role != platform.HandlerEditor {
		t.Errorf("registrations = %+v", regs)
	}
	if changed, _ := f.Register(notesApp, withRole(noteFile, platform.HandlerEditor), platform.RegisterOptions{}); changed {
		t.Errorf("HandlerEditor is the default role: Register wrote")
	}
	f.SetDefault(noteFile, notesApp.ID)
	w := f.Writes()
	if changed, err := f.Register(notesApp, declaredFile, platform.RegisterOptions{}); !changed || err != nil || f.Writes() != w+1 {
		t.Errorf("Register with HandlerNone = %v, %v", changed, err)
	}
	if regs := f.Registrations(); len(regs) != 1 || regs[0].Role != platform.HandlerNone {
		t.Errorf("registrations = %+v", regs)
	}
	if s, _ := f.Status(notesApp, noteFile); s.Registered || s.Default || s.Role != platform.HandlerNone {
		t.Errorf("status as a handler = %+v", s)
	}
	if s, _ := f.Status(notesApp, declaredFile); !s.Registered || s.Default {
		t.Errorf("status of the declaration = %+v", s)
	}
	if changed, _ := f.Unregister(notesApp, noteFile); !changed || len(f.Registrations()) != 0 {
		t.Errorf("Unregister of the declaration = %v, %+v", changed, f.Registrations())
	}
	if regs, _ := f.Register(notesApp, scheme, platform.RegisterOptions{}); !regs || f.Registrations()[0].Role != platform.HandlerViewer {
		t.Errorf("a scheme registers as a viewer: %+v", f.Registrations())
	}
	mustPanic(t, "a scheme with HandlerNone", func() {
		_, _ = f.Register(notesApp, withRole(scheme, platform.HandlerNone), platform.RegisterOptions{})
	})
	mustPanic(t, "MakeDefault with HandlerNone", func() { _, _ = f.MakeDefault(notesApp, declaredFile) })
}

// Every implementation refuses the roles that make no sense, before any
// I/O.
func TestRolesChecked(t *testing.T) {
	e := newLinuxEnv(t)
	_, win := winSetup(t)
	_, mac, app := macSetup(t)
	for name, a := range map[string]platform.Associations{"linux": e.a, "windows": win, "macos": mac, "unsupported": platform.UnsupportedAssociations{}} {
		h := notesApp
		if name == "macos" {
			h = app
		}
		mustPanic(t, name+" Register of a scheme with HandlerNone", func() {
			_, _ = a.Register(h, withRole(scheme, platform.HandlerNone), platform.RegisterOptions{})
		})
		mustPanic(t, name+" MakeDefault with HandlerNone", func() { _, _ = a.MakeDefault(h, declaredFile) })
		mustPanic(t, name+" an invalid role", func() { _, _ = a.Status(h, withRole(noteFile, 99)) })
	}
}
