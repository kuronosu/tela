package assoc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/plist"
)

// The macOS associations against temporary bundles and a fake Launch
// Services (memLS) that reads their Info.plist when they are registered,
// as the real one does: the owner's conditions of ADR-209 on every OS.
// macos_darwin_test.go runs the same calls against the real Launch
// Services.

// memLS is a LaunchServices in memory. A bundle's claims are what its
// Info.plist declared when it was last registered; defaults follow Launch
// Services' rules: the user's choice while its app still claims the type,
// else the best rank (Owner, Default, Alternate; None never), the first
// registered among equals.
type memLS struct {
	mu        sync.Mutex
	order     []string              // bundle paths, in registration order
	bundles   map[string]plist.Dict // path -> Info.plist at its registration
	schemeDef map[string]string     // lower-cased scheme -> bundle ID
	typeDef   map[string]string     // lower-cased UTI -> bundle ID
	system    map[string]string     // extension -> UTI the system declares
	registers int                   // RegisterBundle calls
	ignoreSet bool                  // the Set calls succeed and change nothing
	fail      error                 // every call fails with it
}

func newMemLS() *memLS {
	return &memLS{
		bundles:   map[string]plist.Dict{},
		schemeDef: map[string]string{},
		typeDef:   map[string]string{},
		system:    map[string]string{"txt": "public.plain-text", "md": "net.daringfireball.markdown"},
	}
}

func normPath(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

func (l *memLS) RegisterBundle(path string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fail != nil {
		return l.fail
	}
	data, err := os.ReadFile(filepath.Join(path, "Contents", "Info.plist"))
	if err != nil {
		return err
	}
	info, err := plist.UnmarshalDict(data)
	if err != nil {
		return err
	}
	p := normPath(path)
	if _, ok := l.bundles[p]; !ok {
		l.order = append(l.order, p)
	}
	l.bundles[p] = info
	l.registers++
	return nil
}

func (l *memLS) calls() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.registers
}

func idOf(info plist.Dict) string { id, _ := info["CFBundleIdentifier"].(string); return id }

func (l *memLS) BundlePaths(id string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, p := range l.order {
		if strings.EqualFold(idOf(l.bundles[p]), id) {
			out = append(out, p)
		}
	}
	return out, l.fail
}

func (l *memLS) SchemeHandlers(scheme string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.schemeHandlers(scheme), l.fail
}

func (l *memLS) schemeHandlers(scheme string) []string {
	var out []string
	for _, p := range l.order {
		if id := idOf(l.bundles[p]); hasScheme(l.bundles[p], scheme) && !containsFold(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func (l *memLS) DefaultSchemeHandler(scheme string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	hs := l.schemeHandlers(scheme)
	if d := l.schemeDef[strings.ToLower(scheme)]; containsFold(hs, d) {
		return d, l.fail
	}
	if len(hs) > 0 {
		return hs[0], l.fail
	}
	return "", l.fail
}

func (l *memLS) SetDefaultSchemeHandler(scheme, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.ignoreSet {
		l.schemeDef[strings.ToLower(scheme)] = id
	}
	return l.fail
}

// rankOf returns a bundle's rank for uti: 0 when it does not claim it.
func rankOf(info plist.Dict, uti string) int {
	for _, v := range entriesOf(info, keyDocTypes) {
		e, ok := v.(plist.Dict)
		if !ok || !containsFold(stringsOf(e["LSItemContentTypes"]), uti) {
			continue
		}
		switch r, _ := e["LSHandlerRank"].(string); r {
		case "Owner":
			return 4
		case "Alternate":
			return 2
		case "None":
			return 1
		default:
			return 3
		}
	}
	return 0
}

func (l *memLS) typeHandlers(uti string) (ids []string, best string) {
	top := 1
	for _, p := range l.order {
		id, r := idOf(l.bundles[p]), rankOf(l.bundles[p], uti)
		if r > 0 && !containsFold(ids, id) {
			ids = append(ids, id)
		}
		if r > top {
			top, best = r, id
		}
	}
	return ids, best
}

func (l *memLS) TypeHandlers(uti string) ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids, _ := l.typeHandlers(uti)
	return ids, l.fail
}

func (l *memLS) DefaultTypeHandler(uti string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids, best := l.typeHandlers(uti)
	if d := l.typeDef[strings.ToLower(uti)]; containsFold(ids, d) {
		return d, l.fail
	}
	return best, l.fail
}

func (l *memLS) SetDefaultTypeHandler(uti, id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.ignoreSet {
		l.typeDef[strings.ToLower(uti)] = id
	}
	return l.fail
}

func (l *memLS) TypeOfExtension(ext string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if u, ok := l.system[ext]; ok {
		return u, l.fail
	}
	for _, p := range l.order {
		if u, _ := declaredType(l.bundles[p], ext); u != "" {
			return u, l.fail
		}
	}
	return "dyn.ah62d4rv4ge8" + ext, l.fail
}

// writeBundle writes <root>/<name>.app with an Info.plist of id and the
// extra keys, and returns the path of its executable.
func writeBundle(t *testing.T, root, name, id string, extra plist.Dict) string {
	t.Helper()
	dir := filepath.Join(root, name+".app")
	if err := os.MkdirAll(filepath.Join(dir, "Contents", "MacOS"), 0o755); err != nil {
		t.Fatal(err)
	}
	info := plist.Dict{
		"CFBundleIdentifier":  id,
		"CFBundleName":        name,
		"CFBundleExecutable":  name,
		"CFBundlePackageType": "APPL",
	}
	for k, v := range extra {
		info[k] = v
	}
	data, err := plist.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Contents", "Info.plist"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "Contents", "MacOS", name)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func readInfo(t *testing.T, exe string) (plist.Dict, []byte) {
	t.Helper()
	p := filepath.Join(filepath.Dir(filepath.Dir(exe)), "Info.plist")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	d, err := plist.UnmarshalDict(data)
	if err != nil {
		t.Fatal(err)
	}
	return d, data
}

// macSetup returns a fake Launch Services, the associations on it, and the
// Notes app in a fresh bundle (with an unknown key and the developer's own
// URL type, which must survive every edit).
func macSetup(t *testing.T) (*memLS, platform.Associations, platform.HandlerApp) {
	t.Helper()
	l := newMemLS()
	m := MacOS(l).(*macOS)
	m.settle = 0
	m.exe = func() (string, error) { return "/usr/local/bin/notes-test", nil }
	m.tempDirs = nil // the bundles are in t.TempDir(); TestMacOSTempDir fakes one
	exe := writeBundle(t, t.TempDir(), "Notes", "com.example.Notes", plist.Dict{
		"XUnknownKey": plist.Dict{"nested": plist.Array{int64(1), "two"}},
		keyURLTypes: plist.Array{plist.Dict{
			"CFBundleURLName":    "com.example.Notes.dev",
			"CFBundleURLSchemes": plist.Array{"notes-dev"},
		}},
	})
	return l, m, platform.HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: exe}
}

func macStatus(t *testing.T, a platform.Associations, app platform.HandlerApp, as platform.Association) platform.AssociationStatus {
	t.Helper()
	s, err := a.Status(app, as)
	if err != nil {
		t.Fatalf("Status(%v): %v", as, err)
	}
	return s
}

// checkKept fails when the developer's entries or the unknown key are gone.
func checkKept(t *testing.T, info plist.Dict) {
	t.Helper()
	if !hasScheme(info, "notes-dev") {
		t.Errorf("the developer's URL type is gone: %v", info[keyURLTypes])
	}
	if u, ok := info["XUnknownKey"].(plist.Dict); !ok || fmt.Sprint(u["nested"]) != "[1 two]" {
		t.Errorf("an unknown key changed: %v", info["XUnknownKey"])
	}
	for _, k := range []string{"CFBundleIdentifier", "CFBundleName", "CFBundleExecutable", "CFBundlePackageType"} {
		if _, ok := info[k]; !ok {
			t.Errorf("%s is gone", k)
		}
	}
}

func TestMacOSRegisterScheme(t *testing.T) {
	l, a, app := macSetup(t)
	if s := macStatus(t, a, app, scheme); s != (platform.AssociationStatus{}) {
		t.Fatalf("initial status = %+v", s)
	}
	changed, err := a.Register(app, scheme, platform.RegisterOptions{})
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	info, raw := readInfo(t, app.Exec)
	var ours plist.Dict
	for _, v := range entriesOf(info, keyURLTypes) {
		if e := v.(plist.Dict); marked(e) {
			ours = e
		}
	}
	if ours == nil || fmt.Sprint(ours["CFBundleURLSchemes"]) != "[notes]" || ours["CFBundleURLName"] != "com.example.Notes.notes" ||
		ours["CFBundleTypeRole"] != "Viewer" || len(entriesOf(info, keyURLTypes)) != 2 {
		t.Errorf("CFBundleURLTypes = %v", info[keyURLTypes])
	}
	checkKept(t, info)
	if l.calls() != 1 {
		t.Errorf("LSRegisterURL called %d times, want once", l.calls())
	}
	// The only candidate: Launch Services makes it the handler by itself.
	if s := macStatus(t, a, app, scheme); s != (platform.AssociationStatus{Registered: true, Default: true, Role: platform.HandlerViewer}) {
		t.Errorf("status after Register = %+v", s)
	}

	// Again: nothing to write, nothing to register.
	changed, err = a.Register(app, scheme, platform.RegisterOptions{})
	if changed || err != nil {
		t.Fatalf("second Register = %v, %v", changed, err)
	}
	if _, raw2 := readInfo(t, app.Exec); !bytes.Equal(raw, raw2) || l.calls() != 1 {
		t.Errorf("the second Register wrote (%d registrations)", l.calls())
	}
	// A scheme the developer declared is registered already, and Launch
	// Services read it with the bundle.
	if changed, err := a.Register(app, platform.Association{Scheme: "notes-dev"}, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("Register(notes-dev) = %v, %v", changed, err)
	}
	if _, raw2 := readInfo(t, app.Exec); !bytes.Equal(raw, raw2) {
		t.Error("registering a scheme the Info.plist declares rewrote it")
	}
}

func TestMacOSOtherAppAndMakeDefault(t *testing.T) {
	l, a, app := macSetup(t)
	other := writeBundle(t, t.TempDir(), "Other", "com.other.Editor", plist.Dict{
		keyURLTypes: plist.Array{plist.Dict{"CFBundleURLSchemes": plist.Array{"notes"}}},
		keyDocTypes: plist.Array{plist.Dict{"LSItemContentTypes": plist.Array{"net.daringfireball.markdown"}}},
	})
	otherDir := filepath.Dir(filepath.Dir(filepath.Dir(other)))
	if err := l.RegisterBundle(otherDir); err != nil {
		t.Fatal(err)
	}
	_, otherRaw := readInfo(t, other)
	md := platform.Association{Extension: ".md", MIMEType: "text/markdown", Description: "Markdown document"}

	for _, as := range []platform.Association{scheme, md} {
		if s := macStatus(t, a, app, as); s != (platform.AssociationStatus{Other: "com.other.Editor"}) {
			t.Errorf("%v: status before = %+v", as, s)
		}
		// Registering is not taking: no Replace needed, the other app
		// stays the default.
		if changed, err := a.Register(app, as, platform.RegisterOptions{}); !changed || err != nil {
			t.Fatalf("%v: Register = %v, %v", as, changed, err)
		}
		if s := macStatus(t, a, app, as); s != (platform.AssociationStatus{Registered: true, Other: "com.other.Editor", Role: as.ResolvedRole()}) {
			t.Errorf("%v: status after Register = %+v", as, s)
		}
		r, err := a.MakeDefault(app, as)
		if r != platform.DefaultSet || err != nil {
			t.Fatalf("%v: MakeDefault = %v, %v", as, r, err)
		}
		if s := macStatus(t, a, app, as); s != (platform.AssociationStatus{Registered: true, Default: true, Role: as.ResolvedRole()}) {
			t.Errorf("%v: status after MakeDefault = %+v", as, s)
		}
	}
	if _, raw := readInfo(t, other); !bytes.Equal(raw, otherRaw) {
		t.Error("the other app's Info.plist changed")
	}
	// A known extension uses the system's type, with no declaration.
	info, _ := readInfo(t, app.Exec)
	if _, ok := info[keyImported]; ok {
		t.Errorf("a type was imported for .md: %v", info[keyImported])
	}
	if !hasDocType(info, "net.daringfireball.markdown", "md") {
		t.Errorf("CFBundleDocumentTypes = %v", info[keyDocTypes])
	}

	// Unregister: the app is no longer a candidate, so the user's choice
	// naming it is not used.
	for _, as := range []platform.Association{scheme, md} {
		if changed, err := a.Unregister(app, as); !changed || err != nil {
			t.Fatalf("%v: Unregister = %v, %v", as, changed, err)
		}
		if s := macStatus(t, a, app, as); s != (platform.AssociationStatus{Other: "com.other.Editor"}) {
			t.Errorf("%v: status after Unregister = %+v", as, s)
		}
	}
}

func TestMacOSFileTypeUnknownExtension(t *testing.T) {
	l, a, app := macSetup(t)
	_, before := readInfo(t, app.Exec)
	changed, err := a.Register(app, noteFile, platform.RegisterOptions{})
	if !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	info, raw := readInfo(t, app.Exec)
	imp := entriesOf(info, keyImported)
	if len(imp) != 1 {
		t.Fatalf("UTImportedTypeDeclarations = %v", info[keyImported])
	}
	decl := imp[0].(plist.Dict)
	spec, _ := decl["UTTypeTagSpecification"].(plist.Dict)
	if decl["UTTypeIdentifier"] != "com.example.Notes.note" || decl["UTTypeDescription"] != "Notes document" || !marked(decl) ||
		fmt.Sprint(spec["public.filename-extension"]) != "[note]" || fmt.Sprint(spec["public.mime-type"]) != "[application/x-notes-note]" ||
		fmt.Sprint(decl["UTTypeConformsTo"]) != "[public.data public.content]" {
		t.Errorf("imported type = %v", decl)
	}
	docs := entriesOf(info, keyDocTypes)
	if len(docs) != 1 {
		t.Fatalf("CFBundleDocumentTypes = %v", info[keyDocTypes])
	}
	doc := docs[0].(plist.Dict)
	if fmt.Sprint(doc["LSItemContentTypes"]) != "[com.example.Notes.note]" || doc["LSHandlerRank"] != "Alternate" ||
		doc["CFBundleTypeRole"] != "Editor" || doc["CFBundleTypeName"] != "Notes document" || !marked(doc) {
		t.Errorf("document type = %v", doc)
	}
	checkKept(t, info)
	if u, _ := l.TypeOfExtension("note"); u != "com.example.Notes.note" {
		t.Errorf("Launch Services knows .note as %q", u)
	}
	if s := macStatus(t, a, app, noteFile); s != (platform.AssociationStatus{Registered: true, Default: true, Role: platform.HandlerEditor}) {
		t.Errorf("status = %+v", s)
	}
	if changed, err := a.Register(app, noteFile, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("second Register = %v, %v", changed, err)
	}
	if _, raw2 := readInfo(t, app.Exec); !bytes.Equal(raw, raw2) {
		t.Error("the second Register wrote")
	}

	// Unregister removes both entries, and the keys they made.
	if changed, err := a.Unregister(app, noteFile); !changed || err != nil {
		t.Fatalf("Unregister = %v, %v", changed, err)
	}
	info, _ = readInfo(t, app.Exec)
	if _, ok := info[keyImported]; ok {
		t.Errorf("UTImportedTypeDeclarations left: %v", info[keyImported])
	}
	if _, ok := info[keyDocTypes]; ok {
		t.Errorf("CFBundleDocumentTypes left: %v", info[keyDocTypes])
	}
	checkKept(t, info)
	if after, _ := plist.Marshal(info); !bytes.Equal(after, mustCanon(t, before)) {
		t.Errorf("Register then Unregister left\n%s\nwant\n%s", after, before)
	}
	if s := macStatus(t, a, app, noteFile); s != (platform.AssociationStatus{}) {
		t.Errorf("status after Unregister = %+v", s)
	}
	if changed, err := a.Unregister(app, noteFile); changed || err != nil {
		t.Errorf("second Unregister = %v, %v", changed, err)
	}
}

func mustCanon(t *testing.T, data []byte) []byte {
	t.Helper()
	d, err := plist.UnmarshalDict(data)
	if err != nil {
		t.Fatal(err)
	}
	out, err := plist.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestMacOSUnregisterKeepsOthersEntries: entries the developer wrote are
// never removed, and an entry of several schemes loses only its own.
func TestMacOSUnregisterKeepsOthersEntries(t *testing.T) {
	l, a, app := macSetup(t)
	if changed, err := a.Unregister(app, platform.Association{Scheme: "notes-dev"}); changed || err != nil {
		t.Errorf("Unregister of the developer's scheme = %v, %v", changed, err)
	}
	if l.calls() != 0 {
		t.Errorf("Unregister with nothing to remove registered the bundle")
	}
	// A developer's document type for the same content type keeps the
	// imported declaration.
	exe := writeBundle(t, t.TempDir(), "Notes", "com.example.Notes", plist.Dict{
		keyDocTypes: plist.Array{plist.Dict{"LSItemContentTypes": plist.Array{"com.example.Notes.note"}, "CFBundleTypeRole": "Viewer"}},
		keyImported: plist.Array{
			plist.Dict{"UTTypeIdentifier": "com.example.Notes.note", markerKey: true,
				"UTTypeTagSpecification": plist.Dict{"public.filename-extension": "note"}},
		},
		keyURLTypes: plist.Array{plist.Dict{"CFBundleURLSchemes": plist.Array{"notes", "notes2"}, markerKey: true}},
	})
	app.Exec = exe
	if changed, err := a.Register(app, noteFile, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register = %v, %v", changed, err)
	}
	if info, _ := readInfo(t, exe); len(entriesOf(info, keyDocTypes)) != 1 || len(entriesOf(info, keyImported)) != 1 {
		t.Errorf("Register of a declared type added entries: %v", info)
	}
	if changed, err := a.Unregister(app, noteFile); changed || err != nil {
		t.Errorf("Unregister of the developer's document type = %v, %v", changed, err)
	}
	if changed, err := a.Unregister(app, scheme); !changed || err != nil {
		t.Fatalf("Unregister(notes) = %v, %v", changed, err)
	}
	info, _ := readInfo(t, exe)
	if !hasScheme(info, "notes2") || hasScheme(info, "notes") || len(entriesOf(info, keyImported)) != 1 {
		t.Errorf("after Unregister: %v", info)
	}
}

// TestMacOSNoReformat: a no-op keeps a hand-written Info.plist byte for
// byte, formatting included.
func TestMacOSNoReformat(t *testing.T) {
	l, a, app := macSetup(t)
	p := filepath.Join(filepath.Dir(filepath.Dir(app.Exec)), "Info.plist")
	hand := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
  <key>CFBundleURLTypes</key>
  <array><dict><key>CFBundleURLSchemes</key><array><string>notes</string></array></dict></array>
  <key>CFBundleIdentifier</key><string>com.example.Notes</string>
</dict></plist>
`)
	if err := os.WriteFile(p, hand, 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err := a.Register(app, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("Register = %v, %v (Launch Services did not list the bundle)", changed, err)
	}
	if changed, err := a.Register(app, scheme, platform.RegisterOptions{}); changed || err != nil {
		t.Fatalf("second Register = %v, %v", changed, err)
	}
	if got, _ := os.ReadFile(p); !bytes.Equal(got, hand) {
		t.Errorf("the Info.plist was rewritten:\n%s", got)
	}
	if l.calls() != 1 {
		t.Errorf("%d registrations, want 1", l.calls())
	}
}

func TestMacOSNotABundle(t *testing.T) {
	_, a, app := macSetup(t)
	for name, exec := range map[string]string{
		"plain":        "/opt/notes/notes",
		"no plist":     filepath.Join(t.TempDir(), "X.app", "Contents", "MacOS", "x"),
		"not Contents": "/Applications/Notes.app/MacOS/notes",
		"running":      "", // macSetup's exe: /usr/local/bin/notes-test
	} {
		if name == "no plist" {
			if err := os.MkdirAll(filepath.Dir(exec), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		app := app
		app.Exec = exec
		withExec := app
		if withExec.Exec == "" {
			withExec.Exec = "/usr/local/bin/notes-test"
		}
		errs := map[string]error{}
		_, errs["Status"] = a.Status(app, scheme)
		_, errs["Register"] = a.Register(withExec, scheme, platform.RegisterOptions{})
		_, errs["Unregister"] = a.Unregister(app, noteFile)
		_, errs["MakeDefault"] = a.MakeDefault(withExec, scheme)
		_, errs["RegisterApp"] = a.RegisterApp(withExec, platform.RegisterOptions{})
		_, errs["UnregisterApp"] = a.UnregisterApp(app)
		_, errs["AppStatus"] = a.AppStatus(app)
		for call, err := range errs {
			if !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(fmt.Sprint(err), "tela run") || !strings.Contains(fmt.Sprint(err), "tela build") {
				t.Errorf("%s: %s = %v, want ErrUnsupported naming tela build and tela run", name, call, err)
			}
		}
	}
	// Through a symbolic link into a bundle: the bundle's.
	link := filepath.Join(t.TempDir(), "notes")
	if !makeLink(t, app.Exec, link) {
		return
	}
	app.Exec = link
	if changed, err := a.Register(app, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register through a link = %v, %v", changed, err)
	}
}

func TestMacOSIDMismatch(t *testing.T) {
	l, a, app := macSetup(t)
	_, raw := readInfo(t, app.Exec)
	app.ID = "com.example.Other"
	_, err := a.Register(app, scheme, platform.RegisterOptions{})
	if err == nil || errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), `"com.example.Other"`) || !strings.Contains(err.Error(), `"com.example.Notes"`) {
		t.Errorf("Register with another ID = %v, want an error naming both IDs", err)
	}
	if _, err := a.Status(app, scheme); err == nil {
		t.Error("Status with another ID: no error")
	}
	if _, raw2 := readInfo(t, app.Exec); !bytes.Equal(raw, raw2) || l.calls() != 0 {
		t.Error("a refused call wrote")
	}
	// Bundle identifiers are compared without regard to case.
	app.ID = "COM.example.notes"
	if changed, err := a.Register(app, scheme, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register with the ID in another case = %v, %v", changed, err)
	}
}

func TestMacOSSignedBundle(t *testing.T) {
	_, a, app := macSetup(t)
	if err := os.Mkdir(filepath.Join(filepath.Dir(filepath.Dir(app.Exec)), "_CodeSignature"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, raw := readInfo(t, app.Exec)
	if _, err := a.Register(app, scheme, platform.RegisterOptions{}); !errors.Is(err, platform.ErrUnsupported) || !strings.Contains(err.Error(), "signed") {
		t.Errorf("Register in a signed bundle = %v", err)
	}
	if _, raw2 := readInfo(t, app.Exec); !bytes.Equal(raw, raw2) {
		t.Error("the signed bundle's Info.plist changed")
	}
	// What it declares already is registered with Launch Services.
	if changed, err := a.Register(app, platform.Association{Scheme: "notes-dev"}, platform.RegisterOptions{}); !changed || err != nil {
		t.Errorf("Register of a declared scheme in a signed bundle = %v, %v", changed, err)
	}
}

func TestMacOSMakeDefaultFails(t *testing.T) {
	l, a, app := macSetup(t)
	l.ignoreSet = true
	other := writeBundle(t, t.TempDir(), "Other", "com.other.Editor", plist.Dict{
		keyURLTypes: plist.Array{plist.Dict{"CFBundleURLSchemes": plist.Array{"notes"}}},
	})
	if err := l.RegisterBundle(filepath.Dir(filepath.Dir(filepath.Dir(other)))); err != nil {
		t.Fatal(err)
	}
	if r, err := a.MakeDefault(app, scheme); r != platform.DefaultNone || err == nil || !strings.Contains(err.Error(), "com.other.Editor") {
		t.Errorf("MakeDefault that Launch Services ignores = %v, %v", r, err)
	}
	l.ignoreSet, l.fail = false, errors.New("lsd is down")
	if r, err := a.MakeDefault(app, scheme); r != platform.DefaultNone || err == nil || !strings.Contains(err.Error(), "lsd is down") {
		t.Errorf("MakeDefault with Launch Services failing = %v, %v", r, err)
	}
	if _, err := a.Status(app, scheme); err == nil {
		t.Error("Status with Launch Services failing: no error")
	}
}

func TestMacOSRegisterApp(t *testing.T) {
	l, a, app := macSetup(t)
	if st, err := a.AppStatus(app); st != (platform.AppStatus{}) || err != nil {
		t.Errorf("AppStatus before = %+v, %v", st, err)
	}
	_, raw := readInfo(t, app.Exec)
	if changed, err := a.RegisterApp(app, platform.RegisterOptions{}); !changed || err != nil {
		t.Fatalf("RegisterApp = %v, %v", changed, err)
	}
	if changed, err := a.RegisterApp(app, platform.RegisterOptions{}); changed || err != nil {
		t.Errorf("second RegisterApp = %v, %v", changed, err)
	}
	if st, err := a.AppStatus(app); st != (platform.AppStatus{Registered: true}) || err != nil {
		t.Errorf("AppStatus = %+v, %v", st, err)
	}
	// An installed copy of the app elsewhere is Other.
	copyExe := writeBundle(t, t.TempDir(), "Notes", "com.example.Notes", nil)
	copyDir := filepath.Dir(filepath.Dir(filepath.Dir(copyExe)))
	if err := l.RegisterBundle(copyDir); err != nil {
		t.Fatal(err)
	}
	if st, err := a.AppStatus(app); !st.Registered || !samePath(st.Other, copyDir) || st.Requested || err != nil {
		t.Errorf("AppStatus with a copy = %+v, %v", st, err)
	}
	if changed, err := a.UnregisterApp(app); changed || err != nil {
		t.Errorf("UnregisterApp = %v, %v", changed, err)
	}
	if _, raw2 := readInfo(t, app.Exec); !bytes.Equal(raw, raw2) || l.calls() != 2 {
		t.Errorf("RegisterApp and UnregisterApp wrote the Info.plist, or registered %d times", l.calls())
	}
}

// TestMacOSConcurrent: calls from many goroutines keep every entry (one
// read-modify-write at a time).
func TestMacOSConcurrent(t *testing.T) {
	_, a, app := macSetup(t)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			if _, err := a.Register(app, platform.Association{Scheme: fmt.Sprintf("notes%d", i)}, platform.RegisterOptions{}); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	info, _ := readInfo(t, app.Exec)
	for i := range 8 {
		if !hasScheme(info, fmt.Sprintf("notes%d", i)) {
			t.Errorf("notes%d is missing", i)
		}
	}
}

func TestOwnTypeID(t *testing.T) {
	if got := ownTypeID("com.example.Notes", "My_Ext"); got != "com.example.Notes.my-ext" {
		t.Errorf("ownTypeID = %q", got)
	}
}
