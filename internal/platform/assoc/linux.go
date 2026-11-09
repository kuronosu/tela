package assoc

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/kuronosu/tela/internal/platform"
)

// The Linux implementation (freedesktop.org Desktop Entry, Association
// between MIME types and applications, and shared-mime-info
// specifications). It is plain file I/O on the directories of LinuxDirs,
// so it builds and is tested on every OS.
//
// What it writes, and nothing else:
//
//   - $XDG_DATA_HOME/applications/<ID>.desktop: the app's own desktop
//     entry, written from scratch and marked X-Tela-Registered=true. Its
//     MimeType lists x-scheme-handler/<scheme> for each scheme and the
//     MIME type of each file type; Exec runs HandlerApp.Exec and Args,
//     each argument quoted, followed by %u (schemes only), %F (file types
//     only) or %U (both: the specification allows one field code, so
//     files may then arrive as file:// URIs). NoDisplay=true: the entry
//     makes the app a handler without adding a launcher to the menus (an
//     installer writes that). It is removed when its last type is
//     unregistered, unless RegisterApp asked for it (ADR-217): then it
//     carries X-Tela-App=true, and stays, with no MimeType, until
//     UnregisterApp. The same file is the app's entry for the
//     GlobalShortcuts portal, which refuses an app ID without one.
//   - $XDG_DATA_HOME/mime/packages/<ID>.xml: a shared-mime-info package
//     mapping an extension to its MIME type ("*.note"), only when no MIME
//     type claims the extension yet (or with Replace, at weight 80 so it
//     wins over another type's glob). Without it a new type is never
//     detected, so files would not open with the app.
//   - The [Default Applications] group of $XDG_CONFIG_HOME/mimeapps.list:
//     the app's desktop ID put first for the type by MakeDefault (the
//     earlier defaults kept after it, as fallbacks), and removed by
//     Unregister (also from $XDG_CONFIG_HOME/<desktop>-mimeapps.list).
//     Every other line of the file is kept as it was.
//
// Roles (platform.HandlerRole): the desktop entry specification has none,
// so HandlerEditor, HandlerViewer and HandlerShell write the same entry.
// A file type with HandlerNone is declared, not handled: the MIME package
// is written as for any file type, the entry does not list the type (it is
// removed from the MimeType of Tela's entry, and the entry itself when
// nothing else keeps it), and the user's defaults naming the app for the
// type are removed as Unregister removes them. Another program's entry of
// the app's ID is then no conflict, since nothing is written to it.
//
// The caches. GIO (checked with GLib 2.90) and xdg-utils' generic
// xdg-mime find the applications of a MIME type only through
// $XDG_DATA_HOME/applications/mimeinfo.cache, not the MimeType keys of
// the entries, so after changing the entry this package rebuilds that
// cache itself, as update-desktop-database would (every entry of the
// directory, Hidden ones left out), with no external program. File type
// detection reads the compiled MIME database (mime.cache, globs2), never
// the package XML, and its binary format is not worth reimplementing:
// after changing the package it runs update-mime-database on
// $XDG_DATA_HOME/mime (LinuxDirs.Run), the only external program here. It
// is optional (from shared-mime-info, installed wherever a desktop is):
// when it is missing or fails the package is still written, and the next
// run of update-mime-database by anything compiles it; until then files
// of a new type are not detected as that type.
//
// Every file is written atomically (a temporary file in the same
// directory, synced, then renamed over the old one; a symbolic link is
// followed, so a mimeapps.list kept in a dotfiles repository stays a
// link) and only when its content changes.

// telaKey marks the desktop entries Tela wrote; appKey the ones
// RegisterApp asked for, which stay without associations.
const (
	telaKey = "X-Tela-Registered"
	appKey  = "X-Tela-App"
)

const (
	entryGroup    = "Desktop Entry"
	defaultsGroup = "Default Applications"
	addedGroup    = "Added Associations"
	removedGroup  = "Removed Associations"
)

// Linux returns the associations of a Linux desktop with the given
// directories (see the package documentation).
func Linux(dirs LinuxDirs) platform.Associations { return &linuxAssoc{dirs: dirs} }

type linuxAssoc struct {
	dirs LinuxDirs
}

// linuxMu serializes the read-modify-write cycles of the process's calls
// (the methods are safe from any goroutine). Other processes are not
// locked out: an atomic rename keeps every file whole.
var linuxMu sync.Mutex

// mimeOf returns the MIME type an association is registered under.
func mimeOf(a platform.Association) string {
	if a.Scheme != "" {
		return "x-scheme-handler/" + strings.ToLower(a.Scheme)
	}
	return a.MIMEType
}

func hasType(types []string, typ string) bool {
	return slices.ContainsFunc(types, func(t string) bool { return strings.EqualFold(t, typ) })
}

func (l *linuxAssoc) appsDir() string { return filepath.Join(l.dirs.DataHome, "applications") }

func (l *linuxAssoc) ownEntryPath(id string) string {
	return filepath.Join(l.appsDir(), id+".desktop")
}

func (l *linuxAssoc) ownPackagePath(id string) string {
	return filepath.Join(l.dirs.DataHome, "mime", "packages", id+".xml")
}

func (l *linuxAssoc) check() error {
	if !filepath.IsAbs(l.dirs.DataHome) {
		return fmt.Errorf("assoc: LinuxDirs.DataHome %q is not an absolute path ($XDG_DATA_HOME)", l.dirs.DataHome)
	}
	return nil
}

// --- reading the desktop's state ---

// desktopEntry is the effective desktop entry of a desktop file ID: the
// first file with that ID in the data directories' order.
type desktopEntry struct {
	path  string
	rank  int // index of its applications directory: 0 is the user's
	mime  []string
	hide  bool // Hidden=true: the entry counts as deleted
	tela  bool // the user's entry, written by Tela
	app   bool // a Tela entry RegisterApp asked for (appKey)
	raw   []byte
	entry *keyFile
}

// scanEntries returns the effective desktop entries by desktop file ID
// ("com.example.Notes.desktop"; a file in a subdirectory "kde/x.desktop"
// has the ID "kde-x.desktop"), searching $XDG_DATA_HOME/applications, then
// each of $XDG_DATA_DIRS.
func (l *linuxAssoc) scanEntries() map[string]*desktopEntry {
	ents := map[string]*desktopEntry{}
	dirs := append([]string{l.dirs.DataHome}, l.dirs.DataDirs...)
	for rank, base := range dirs {
		if base == "" {
			continue
		}
		root := filepath.Join(base, "applications")
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".desktop") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return nil
			}
			id := strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")
			if ents[id] != nil {
				return nil // shadowed by a directory searched earlier
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			kf := parseKeyFile(raw)
			e := &desktopEntry{path: path, rank: rank, raw: raw, entry: kf}
			if v, ok := kf.get(entryGroup, "MimeType"); ok {
				e.mime = splitList(v)
			}
			if v, _ := kf.get(entryGroup, "Hidden"); strings.TrimSpace(v) == "true" {
				e.hide = true
			}
			if v, _ := kf.get(entryGroup, telaKey); rank == 0 && strings.TrimSpace(v) == "true" {
				e.tela = true
				v, _ := kf.get(entryGroup, appKey)
				e.app = strings.TrimSpace(v) == "true"
			}
			ents[id] = e
			return nil
		})
	}
	return ents
}

func installed(ents map[string]*desktopEntry, id string) bool {
	e := ents[id]
	return e != nil && !e.hide
}

// mimeappsPaths returns the mimeapps.list files in the order of the
// specification: in each of $XDG_CONFIG_HOME, $XDG_CONFIG_DIRS,
// $XDG_DATA_HOME/applications and $XDG_DATA_DIRS/applications (the last
// two deprecated but still read), the <desktop>-mimeapps.list of each
// desktop of $XDG_CURRENT_DESKTOP, then mimeapps.list.
func (l *linuxAssoc) mimeappsPaths() []string {
	var out []string
	add := func(dir string) {
		if dir == "" {
			return
		}
		for _, d := range l.dirs.Desktops {
			out = append(out, filepath.Join(dir, d+"-mimeapps.list"))
		}
		out = append(out, filepath.Join(dir, "mimeapps.list"))
	}
	add(l.dirs.ConfigHome)
	for _, d := range l.dirs.ConfigDirs {
		add(d)
	}
	add(l.appsDir())
	for _, d := range l.dirs.DataDirs {
		add(filepath.Join(d, "applications"))
	}
	return out
}

// mimeapps is the content of the mimeapps.list files, in order.
type mimeapps []*keyFile

func (l *linuxAssoc) readMimeapps() mimeapps {
	var m mimeapps
	for _, p := range l.mimeappsPaths() {
		if raw, err := os.ReadFile(p); err == nil {
			m = append(m, parseKeyFile(raw))
		}
	}
	return m
}

func listOf(kf *keyFile, group, typ string) []string {
	v, ok := kf.get(group, typ)
	if !ok {
		return nil
	}
	return splitList(v)
}

// defaultFor returns the explicit default application of typ: the first
// installed desktop ID listed for it under [Default Applications], in file
// order; "" when there is none.
func (m mimeapps) defaultFor(typ string, ents map[string]*desktopEntry) string {
	for _, kf := range m {
		for _, id := range listOf(kf, defaultsGroup, typ) {
			if installed(ents, id) {
				return id
			}
		}
	}
	return ""
}

// candidates returns the applications associated with typ, most preferred
// first: those of [Added Associations], then the installed desktop
// entries whose MimeType lists it (in directory order, then by ID),
// without those of [Removed Associations].
func (m mimeapps) candidates(typ string, ents map[string]*desktopEntry) []string {
	removed := map[string]bool{}
	for _, kf := range m {
		for _, id := range listOf(kf, removedGroup, typ) {
			removed[id] = true
		}
	}
	var out []string
	add := func(id string) {
		if !removed[id] && installed(ents, id) && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for _, kf := range m {
		for _, id := range listOf(kf, addedGroup, typ) {
			add(id)
		}
	}
	var listing []string
	for id, e := range ents {
		if hasType(e.mime, typ) {
			listing = append(listing, id)
		}
	}
	slices.SortFunc(listing, func(a, b string) int {
		if c := ents[a].rank - ents[b].rank; c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})
	for _, id := range listing {
		add(id)
	}
	return out
}

func readMaybe(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return raw, err
}

func (l *linuxAssoc) readPackage(id string) (*mimePackage, []byte, error) {
	path := l.ownPackagePath(id)
	raw, err := readMaybe(path)
	if err != nil {
		return nil, nil, fmt.Errorf("assoc: %w", err)
	}
	if raw == nil {
		return &mimePackage{}, nil, nil
	}
	p, err := parseMIMEPackage(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("assoc: %s is not a MIME package: %v", path, err)
	}
	return p, raw, nil
}

// --- Status ---

func (l *linuxAssoc) Status(app platform.HandlerApp, a platform.Association) (platform.AssociationStatus, error) {
	platform.CheckHandlerApp("Associations.Status", app, false)
	platform.CheckAssociation("Associations.Status", a)
	if err := l.check(); err != nil {
		return platform.AssociationStatus{}, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	return l.status(app, a)
}

func (l *linuxAssoc) status(app platform.HandlerApp, a platform.Association) (platform.AssociationStatus, error) {
	var st platform.AssociationStatus
	typ := mimeOf(a)
	ownID := app.ID + ".desktop"
	ents := l.scanEntries()
	own := ents[ownID]

	// A file type works only while its extension maps to its MIME type.
	mapped, heldBy := true, ""
	if a.Extension != "" {
		pkg, _, err := l.readPackage(app.ID)
		if err != nil {
			return st, err
		}
		claims := l.extClaims(a.Extension, l.ownPackagePath(app.ID))
		if _, ok := claims[strings.ToLower(typ)]; !ok && !strings.EqualFold(pkg.globType(a.Extension), typ) {
			mapped = false
			heldBy = topClaim(claims)
		}
	}
	st.Registered = mapped && own != nil && !own.hide && hasType(own.mime, typ)

	m := l.readMimeapps()
	if def := m.defaultFor(typ, ents); def != "" {
		st.Default = def == ownID && mapped
		if def != ownID {
			st.Other = def
		}
	} else {
		// No explicit default: the sole candidate is the default, as
		// every implementation picks it; among several, none is reported.
		cands := m.candidates(typ, ents)
		st.Default = mapped && len(cands) == 1 && cands[0] == ownID
		for _, id := range cands {
			if id != ownID {
				st.Other = id
				break
			}
		}
	}
	if st.Other == "" && heldBy != "" {
		st.Other = heldBy
	}
	if a.Role == platform.HandlerNone {
		// A declaration: the type exists, whoever installed it, and the
		// app is no handler of it.
		st.Registered, st.Default = mapped, false
		return st, nil
	}
	if st.Other == "" && own != nil && !own.tela && !hasType(own.mime, typ) {
		st.Other = own.path // what Register would refuse to replace
	}
	return st, nil
}

// --- Register ---

func (l *linuxAssoc) Register(app platform.HandlerApp, a platform.Association, opts platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.Register", app, true)
	platform.CheckAssociation("Associations.Register", a)
	if err := l.check(); err != nil {
		return false, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	return l.register(app, a, opts.Replace)
}

// change is a planned write: data nil removes the file.
type change struct {
	path string
	data []byte
}

func (l *linuxAssoc) register(app platform.HandlerApp, a platform.Association, replace bool) (bool, error) {
	typ := mimeOf(a)
	ownID := app.ID + ".desktop"
	ents := l.scanEntries()
	own := ents[ownID]

	// Plan everything first, so a refusal writes nothing.
	var changes []change
	switch {
	case a.Role == platform.HandlerNone:
		// The app declares the type (the MIME package below) without
		// opening it: Tela's entry must not list it, and no default of the
		// user's may name the app for it (GIO opens a default even when
		// its entry does not list the type).
		if own != nil && own.tela && hasType(own.mime, typ) {
			types := withoutType(own.mime, typ)
			if len(types) == 0 && !own.app {
				changes = append(changes, change{own.path, nil})
			} else {
				changes = append(changes, change{own.path, desktopFile(app, types, own.app)})
			}
		}
		drops, err := l.dropDefaults(typ, ownID)
		if err != nil {
			return false, err
		}
		changes = append(changes, drops...)
	case own != nil && own.tela:
		types := own.mime
		if !hasType(types, typ) {
			types = append(slices.Clone(types), typ)
		}
		if data := desktopFile(app, types, own.app); !bytes.Equal(data, own.raw) {
			changes = append(changes, change{l.ownEntryPath(app.ID), data})
		}
	case own != nil && !own.hide && hasType(own.mime, typ):
		// The app's entry that someone else wrote (a package, an
		// installer) already lists the type: nothing to add.
	case own != nil && !replace:
		// A user file Tela did not write, or a system entry that the
		// user's file would shadow.
		return false, platform.TakenError(a, own.path)
	default:
		changes = append(changes, change{l.ownEntryPath(app.ID), desktopFile(app, []string{typ}, false)})
	}

	if a.Extension != "" {
		pkg, raw, err := l.readPackage(app.ID)
		if err != nil {
			return false, err
		}
		claims := l.extClaims(a.Extension, l.ownPackagePath(app.ID))
		cur := pkg.globType(a.Extension)
		ours := strings.EqualFold(cur, typ)
		_, known := claims[strings.ToLower(typ)]
		switch {
		case known:
			// Another package maps the extension to the type already.
			if cur != "" && !ours {
				pkg.removeGlob(a.Extension) // an older registration of ours under another type
			}
		case len(claims) > 0 && !ours:
			if !replace {
				return false, platform.TakenError(a, topClaim(claims))
			}
			pkg.setGlob(a.Extension, typ, a.Description, replaceWeight)
		case !ours:
			pkg.setGlob(a.Extension, typ, a.Description, 0)
		}
		pkg.setComment(typ, a.Description)
		if data := pkg.bytes(app.ID); !bytes.Equal(data, raw) {
			// The package goes first, so the type exists when the entry
			// that names it appears.
			changes = append([]change{{l.ownPackagePath(app.ID), data}}, changes...)
		}
	}
	return l.apply(changes)
}

// desktopFile renders the app's desktop entry for the MIME types; asked
// marks an entry RegisterApp asked for.
func desktopFile(app platform.HandlerApp, types []string, asked bool) []byte {
	var b strings.Builder
	if asked {
		b.WriteString("# Written by Tela (platform.Associations): the app's entry, which RegisterApp\n")
		b.WriteString("# asked for, and the URL schemes and file types it handles.\n")
	} else {
		b.WriteString("# Written by Tela (platform.Associations): the URL schemes and file types\n")
		b.WriteString("# this app handles. Unregister removes it.\n")
	}
	b.WriteString("[" + entryGroup + "]\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Version=1.5\n")
	b.WriteString("Name=" + escapeValue(app.Name) + "\n")
	b.WriteString("Exec=" + execValue(app, types) + "\n")
	if app.Icon != "" {
		b.WriteString("Icon=" + escapeValue(app.Icon) + "\n")
	}
	b.WriteString("Terminal=false\n")
	b.WriteString("NoDisplay=true\n")
	if len(types) > 0 {
		b.WriteString("MimeType=" + joinList(types) + "\n")
	}
	b.WriteString(telaKey + "=true\n")
	if asked {
		b.WriteString(appKey + "=true\n")
	}
	return []byte(b.String())
}

// execValue returns the Exec value: the quoted executable and arguments,
// then the field code for the types (none without types).
func execValue(app platform.HandlerApp, types []string) string {
	var args []string
	for _, a := range append([]string{app.Exec}, app.Args...) {
		args = append(args, quoteExecArg(a))
	}
	return withFieldCode(escapeValue(strings.Join(args, " ")), fieldCode(types))
}

// fieldCode returns the Exec field code for the MIME types: %u for a URI
// (schemes), %F for file paths, %U when the entry handles both (the
// specification allows a single field code; launchers then pass files as
// file:// URIs or paths), and none without types (an entry RegisterApp
// asked for).
func fieldCode(types []string) string {
	if len(types) == 0 {
		return ""
	}
	schemes, files := false, false
	for _, t := range types {
		if strings.HasPrefix(strings.ToLower(t), "x-scheme-handler/") {
			schemes = true
		} else {
			files = true
		}
	}
	switch {
	case schemes && files:
		return "%U"
	case schemes:
		return "%u"
	}
	return "%F"
}

// withFieldCode replaces the field code at the end of a raw Exec value
// ("" removes it).
func withFieldCode(exec, code string) string {
	for _, c := range []string{"%u", "%U", "%f", "%F"} {
		if strings.HasSuffix(exec, " "+c) {
			exec = strings.TrimSuffix(exec, " "+c)
			break
		}
	}
	if code == "" {
		return exec
	}
	return exec + " " + code
}

// setComment sets the comment of the package's type typ, if it defines it.
func (p *mimePackage) setComment(typ, comment string) {
	for i := range p.types {
		if strings.EqualFold(p.types[i].typ, typ) {
			p.types[i].comment = comment
		}
	}
}

// apply makes the planned changes and runs the database updates of the
// directories it changed.
func (l *linuxAssoc) apply(changes []change) (bool, error) {
	updateApps, updateMIME := false, false
	changed := false
	for _, c := range changes {
		var err error
		var did bool
		if c.data == nil {
			did, err = removeFile(c.path)
		} else {
			did, err = true, writeFileAtomic(c.path, c.data)
		}
		if err != nil {
			return changed, fmt.Errorf("assoc: %w", err)
		}
		if !did {
			continue
		}
		changed = true
		switch filepath.Dir(c.path) {
		case l.appsDir():
			updateApps = true
		case filepath.Join(l.dirs.DataHome, "mime", "packages"):
			updateMIME = true
		}
	}
	if updateMIME {
		l.run("update-mime-database", filepath.Join(l.dirs.DataHome, "mime"))
	}
	if updateApps {
		cache := filepath.Join(l.appsDir(), "mimeinfo.cache")
		data := mimeinfoCache(l.appsDir())
		if old, err := readMaybe(cache); err != nil || !bytes.Equal(old, data) {
			if err := writeFileAtomic(cache, data); err != nil {
				return changed, fmt.Errorf("assoc: %w", err)
			}
		}
	}
	return changed, nil
}

// mimeinfoCache renders the mimeinfo.cache of an applications directory
// as update-desktop-database does: a [MIME Cache] group mapping each MIME
// type listed by an entry of the directory (subdirectories included) to
// the desktop IDs listing it. Hidden entries are left out. Types and IDs
// are sorted, so the content depends only on the entries.
func mimeinfoCache(dir string) []byte {
	byType := map[string][]string{}
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".desktop") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		kf := parseKeyFile(raw)
		if v, _ := kf.get(entryGroup, "Hidden"); strings.TrimSpace(v) == "true" {
			return nil
		}
		v, _ := kf.get(entryGroup, "MimeType")
		id := strings.ReplaceAll(filepath.ToSlash(rel), "/", "-")
		for _, t := range splitList(v) {
			t = strings.ToLower(t)
			if !slices.Contains(byType[t], id) {
				byType[t] = append(byType[t], id)
			}
		}
		return nil
	})
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	slices.Sort(types)
	var b strings.Builder
	b.WriteString("[MIME Cache]\n")
	for _, t := range types {
		ids := byType[t]
		slices.Sort(ids)
		b.WriteString(t + "=" + joinList(ids) + "\n")
	}
	return []byte(b.String())
}

// --- Unregister ---

func (l *linuxAssoc) Unregister(app platform.HandlerApp, a platform.Association) (bool, error) {
	platform.CheckHandlerApp("Associations.Unregister", app, false)
	platform.CheckAssociation("Associations.Unregister", a)
	if err := l.check(); err != nil {
		return false, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()

	typ := mimeOf(a)
	ownID := app.ID + ".desktop"
	var changes []change
	if own := l.scanEntries()[ownID]; own != nil && own.tela && hasType(own.mime, typ) {
		types := withoutType(own.mime, typ)
		if len(types) == 0 && !own.app {
			changes = append(changes, change{own.path, nil})
		} else {
			// Unregister has no Exec: edit the entry instead of
			// rendering it again. An entry RegisterApp asked for stays,
			// without MimeType.
			if len(types) == 0 {
				own.entry.del(entryGroup, "MimeType")
			} else {
				own.entry.set(entryGroup, "MimeType", joinList(types))
			}
			if exec, ok := own.entry.get(entryGroup, "Exec"); ok {
				own.entry.set(entryGroup, "Exec", withFieldCode(exec, fieldCode(types)))
			}
			changes = append(changes, change{own.path, own.entry.bytes()})
		}
	}
	if a.Extension != "" {
		pkg, _, err := l.readPackage(app.ID)
		if err != nil {
			return false, err
		}
		if pkg.removeGlob(a.Extension) {
			changes = append(changes, change{l.ownPackagePath(app.ID), pkg.bytes(app.ID)})
		}
	}
	drops, err := l.dropDefaults(typ, ownID)
	if err != nil {
		return false, err
	}
	return l.apply(append(changes, drops...))
}

// withoutType returns types without typ (any spelling).
func withoutType(types []string, typ string) []string {
	return slices.DeleteFunc(slices.Clone(types), func(t string) bool { return strings.EqualFold(t, typ) })
}

// dropDefaults plans the removal of the defaults of typ that name the
// desktop ID id from the user's mimeapps.list and <desktop>-mimeapps.list
// files.
func (l *linuxAssoc) dropDefaults(typ, id string) ([]change, error) {
	if l.dirs.ConfigHome == "" {
		return nil, nil
	}
	var paths []string
	for _, d := range l.dirs.Desktops {
		paths = append(paths, filepath.Join(l.dirs.ConfigHome, d+"-mimeapps.list"))
	}
	paths = append(paths, filepath.Join(l.dirs.ConfigHome, "mimeapps.list"))
	var changes []change
	for _, p := range paths {
		raw, err := readMaybe(p)
		if err != nil {
			return nil, fmt.Errorf("assoc: %w", err)
		}
		if raw == nil {
			continue
		}
		kf := parseKeyFile(raw)
		if dropDefault(kf, typ, id) {
			changes = append(changes, change{p, kf.bytes()})
		}
	}
	return changes, nil
}

// dropDefault removes id from the [Default Applications] entries of typ
// (any spelling of the key), deleting an entry left empty.
func dropDefault(kf *keyFile, typ, id string) bool {
	changed := false
	for _, k := range kf.keys(defaultsGroup) {
		if !strings.EqualFold(k, typ) {
			continue
		}
		v, _ := kf.get(defaultsGroup, k)
		ids := splitList(v)
		rest := slices.DeleteFunc(slices.Clone(ids), func(s string) bool { return s == id })
		if len(rest) == len(ids) {
			continue
		}
		changed = true
		if len(rest) == 0 {
			kf.del(defaultsGroup, k)
		} else {
			kf.set(defaultsGroup, k, joinList(rest))
		}
	}
	return changed
}

// --- MakeDefault ---

func (l *linuxAssoc) MakeDefault(app platform.HandlerApp, a platform.Association) (platform.DefaultResult, error) {
	platform.CheckHandlerApp("Associations.MakeDefault", app, true)
	platform.CheckMakeDefault("Associations.MakeDefault", a)
	if err := l.check(); err != nil {
		return platform.DefaultNone, err
	}
	if !filepath.IsAbs(l.dirs.ConfigHome) {
		return platform.DefaultNone, fmt.Errorf("assoc: LinuxDirs.ConfigHome %q is not an absolute path ($XDG_CONFIG_HOME)", l.dirs.ConfigHome)
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	if _, err := l.register(app, a, false); err != nil {
		return platform.DefaultNone, err
	}
	typ := mimeOf(a)
	ownID := app.ID + ".desktop"
	ents := l.scanEntries()

	// A desktop-specific file of the user comes before mimeapps.list; its
	// default cannot be overridden by writing mimeapps.list, and that
	// file belongs to the desktop environment.
	for _, d := range l.dirs.Desktops {
		p := filepath.Join(l.dirs.ConfigHome, d+"-mimeapps.list")
		raw, _ := readMaybe(p)
		if raw == nil {
			continue
		}
		for _, id := range listOf(parseKeyFile(raw), defaultsGroup, typ) {
			if installed(ents, id) {
				if id != ownID {
					return platform.DefaultNone, fmt.Errorf("assoc: the default handler of %s is set to %s in %s, which comes before mimeapps.list; change it in the desktop's settings", typ, id, p)
				}
				break
			}
		}
	}

	p := filepath.Join(l.dirs.ConfigHome, "mimeapps.list")
	raw, err := readMaybe(p)
	if err != nil {
		return platform.DefaultNone, fmt.Errorf("assoc: %w", err)
	}
	kf := parseKeyFile(raw)
	key, old := typ, []string(nil)
	for _, k := range kf.keys(defaultsGroup) {
		if strings.EqualFold(k, typ) {
			key = k
			v, _ := kf.get(defaultsGroup, k)
			old = splitList(v)
			break
		}
	}
	ids := append([]string{ownID}, slices.DeleteFunc(slices.Clone(old), func(s string) bool { return s == ownID })...)
	if !slices.Equal(ids, old) {
		kf.set(defaultsGroup, key, joinList(ids))
		if _, err := l.apply([]change{{p, kf.bytes()}}); err != nil {
			return platform.DefaultNone, err
		}
	}
	return platform.DefaultSet, nil
}

// --- RegisterApp, UnregisterApp, AppStatus (ADR-217) ---

func (l *linuxAssoc) RegisterApp(app platform.HandlerApp, opts platform.RegisterOptions) (bool, error) {
	platform.CheckHandlerApp("Associations.RegisterApp", app, true)
	if err := l.check(); err != nil {
		return false, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	own := l.scanEntries()[app.ID+".desktop"]
	var types []string
	switch {
	case own != nil && own.tela:
		// Tela's own entry (associations, or an earlier RegisterApp):
		// mark it, keeping its types, and bring Exec and Name up to date.
		types = own.mime
	case own != nil && opts.Replace:
		// Take it over, from scratch.
	case own != nil && own.hide:
		return false, fmt.Errorf("%w: the desktop entry %s hides the app (set RegisterOptions.Replace to take it over)", platform.ErrAssociationTaken, own.path)
	case own != nil:
		// A package's or an installer's entry already makes the
		// desktop know the app: nothing to write, nothing taken over.
		return false, nil
	}
	data := desktopFile(app, types, true)
	if own != nil && own.tela && bytes.Equal(data, own.raw) {
		return false, nil
	}
	return l.apply([]change{{l.ownEntryPath(app.ID), data}})
}

func (l *linuxAssoc) UnregisterApp(app platform.HandlerApp) (bool, error) {
	platform.CheckHandlerApp("Associations.UnregisterApp", app, false)
	if err := l.check(); err != nil {
		return false, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	own := l.scanEntries()[app.ID+".desktop"]
	if own == nil || !own.tela || !own.app {
		return false, nil
	}
	if len(own.mime) == 0 {
		return l.apply([]change{{own.path, nil}})
	}
	// Associations still need the entry: only the request goes, and
	// Unregister of the last association removes the file.
	own.entry.del(entryGroup, appKey)
	return l.apply([]change{{own.path, own.entry.bytes()}})
}

func (l *linuxAssoc) AppStatus(app platform.HandlerApp) (platform.AppStatus, error) {
	platform.CheckHandlerApp("Associations.AppStatus", app, false)
	if err := l.check(); err != nil {
		return platform.AppStatus{}, err
	}
	linuxMu.Lock()
	defer linuxMu.Unlock()
	var st platform.AppStatus
	if own := l.scanEntries()[app.ID+".desktop"]; own != nil {
		st.Registered = !own.hide
		st.Requested = own.tela && own.app
		if !own.tela {
			st.Other = own.path
		}
	}
	return st, nil
}
