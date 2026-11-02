package assoc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/plist"
)

// The macOS implementation (ADR-224, ADR-228). On macOS an app is its
// bundle: Launch Services knows applications by their bundle identifier
// and the bundle's Info.plist, and routes a URL or a file to an app only
// through what that Info.plist declares. So the app registers by editing
// its own Info.plist, then asking Launch Services to read it again. What
// it writes, and nothing else:
//
//   - A URL scheme: an entry of CFBundleURLTypes, {CFBundleURLName
//     "<ID>.<scheme>", CFBundleURLSchemes [scheme], CFBundleTypeRole
//     Viewer}.
//   - A file type: an entry of CFBundleDocumentTypes, {CFBundleTypeName
//     the description, CFBundleTypeRole Editor, LSHandlerRank Alternate,
//     LSItemContentTypes [the extension's content type]}. The content type
//     is the Uniform Type Identifier that claims the extension: the one
//     the Info.plist itself declares, else the one Launch Services knows
//     (UTTypeCreatePreferredIdentifierForTag: public.plain-text for .txt,
//     another app's declaration). An extension that nothing declares (a
//     dynamic "dyn." type) gets a UTImportedTypeDeclarations entry of its
//     own, {UTTypeIdentifier "<ID>.<ext>", UTTypeDescription,
//     UTTypeConformsTo [public.data, public.content],
//     UTTypeTagSpecification {public.filename-extension [ext],
//     public.mime-type [MIMEType]}}: without a declared type, files of the
//     extension never reach the app (Linux writes a MIME package for the
//     same reason, ADR-212). Imported, not exported: if an app that owns
//     the format exports a type for the extension later, the system
//     prefers that declaration and nothing here competes with it.
//
// Every entry written carries TelaRegistered = true, which Launch Services
// ignores; Unregister removes only marked entries (an entry the developer
// or the packaging step wrote is never touched, and a scheme or type
// already declared by any entry is not declared again). Every other key
// and entry of the Info.plist is kept; the file is decoded with
// internal/plist, re-encoded, and written atomically, only when an entry
// was added or removed (an untouched Info.plist keeps its bytes and its
// formatting). After a change, and when Launch Services does not list the
// bundle yet, it calls LSRegisterURL(bundle, inUpdate true), which reads
// the Info.plist again; nothing else is written, and the Launch Services
// database is the user's (no administrator rights).
//
// Ranks and defaults. LSHandlerRank Alternate makes the app a secondary
// handler of the type: Launch Services prefers an Owner or Default handler
// when it picks a default itself, so registering never makes the app the
// default of a type another app opens. When the app is the only handler
// (a scheme or a type of its own), Launch Services uses it anyway: that is
// the OS's rule for a sole candidate, not a default taken. Nothing an
// app declares displaces another app's: Launch Services keeps every
// bundle's claims side by side, so Register never returns
// ErrAssociationTaken on macOS and RegisterOptions.Replace changes
// nothing. MakeDefault is the explicit call that sets the user's choice:
// LSSetDefaultHandlerForURLScheme for a scheme and
// LSSetDefaultRoleHandlerForContentType (every role) for a content type.
// Both are deprecated since macOS 12 in favor of NSWorkspace's
// setDefaultApplicationAtURL:toOpenURLsWithScheme:completionHandler: and
// toOpenContentType:, which are asynchronous, may ask the user in a dialog
// (macOS asks before it changes some defaults, the web browser's first)
// and so need a running app on the main thread; the functions are
// synchronous, safe from any goroutine, need no running app (installers),
// show nothing for the app's own schemes and types, and still work. The
// user's choice is kept by Launch Services; Unregister cannot remove it
// (there is no API), but a choice naming an app that no longer declares
// the scheme or type is not used.
//
// The bundle. HandlerApp.Exec, else the running executable, must be the
// program inside <Name>.app/Contents/MacOS/ (symbolic links followed):
// `tela build` and `tela run` write such a bundle (ADR-227). A plain
// executable (go run, go build, a test binary) has no Info.plist, so every
// method returns an error wrapping ErrUnsupported that says to use them.
// The app's ID must be the bundle's CFBundleIdentifier (compared without
// regard to case, as Launch Services does): otherwise every method returns
// an error naming both, since Launch Services would register the bundle
// under its own identifier, not the app's. A bundle with a code signature
// seal (Contents/_CodeSignature: signed for distribution) is not edited,
// since the seal covers Info.plist; registering what its Info.plist does
// not declare yet returns an error wrapping ErrUnsupported (declare it
// before signing). Launch Services ignores bundles in temporary folders
// (os.TempDir(), /private/var/folders): LSRegisterURL returns noErr and
// the bundle is never listed, as CI showed on macOS 26 while bundles in
// the user cache and ~/Applications were listed at once. So Register,
// MakeDefault and RegisterApp refuse such a bundle with an error wrapping
// ErrUnsupported that says to build it elsewhere (`tela build -o`) or use
// `tela run`, which builds in the user cache; Status reports it not
// registered.
//
// RegisterApp, UnregisterApp and AppStatus (ADR-217): the bundle is the
// app's registration. RegisterApp calls LSRegisterURL on it when Launch
// Services does not list the bundle's path for the ID yet;
// UnregisterApp writes nothing (the bundle stays what associations need,
// and Launch Services forgets a bundle once it is deleted); AppStatus
// reports whether Launch Services knows the ID
// (LSCopyApplicationURLsForBundleIdentifier) and, as Other, a bundle of
// the ID at another path (an installed copy). Requested is always false.

// LaunchServices is the part of Launch Services the macOS associations use
// (see [MacOS]). The real one exists on darwin only; tests give a fake, so
// the logic runs on every OS. Bundle identifiers are compared without
// regard to case.
type LaunchServices interface {
	// RegisterBundle registers the application bundle at path (the .app
	// folder), or updates its record from its Info.plist: LSRegisterURL
	// with inUpdate true.
	RegisterBundle(path string) error
	// BundlePaths returns the paths of the bundles known with the bundle
	// identifier id (LSCopyApplicationURLsForBundleIdentifier).
	BundlePaths(id string) ([]string, error)
	// SchemeHandlers returns the bundle identifiers of the apps that handle
	// a URL scheme (LSCopyAllHandlersForURLScheme).
	SchemeHandlers(scheme string) ([]string, error)
	// DefaultSchemeHandler returns the bundle identifier of the scheme's
	// default handler, "" when there is none
	// (LSCopyDefaultHandlerForURLScheme).
	DefaultSchemeHandler(scheme string) (string, error)
	// SetDefaultSchemeHandler makes id the scheme's default handler
	// (LSSetDefaultHandlerForURLScheme).
	SetDefaultSchemeHandler(scheme, id string) error
	// TypeHandlers returns the bundle identifiers of the apps that handle
	// a content type in any role (LSCopyAllRoleHandlersForContentType).
	TypeHandlers(uti string) ([]string, error)
	// DefaultTypeHandler returns the bundle identifier of the content
	// type's default handler, "" when there is none
	// (LSCopyDefaultRoleHandlerForContentType).
	DefaultTypeHandler(uti string) (string, error)
	// SetDefaultTypeHandler makes id the default handler of the content
	// type in every role (LSSetDefaultRoleHandlerForContentType).
	SetDefaultTypeHandler(uti, id string) error
	// TypeOfExtension returns the preferred content type of a file name
	// extension given without its dot ("note"): a declared type, or a
	// dynamic one starting with "dyn." when nothing declares it
	// (UTTypeCreatePreferredIdentifierForTag).
	TypeOfExtension(ext string) (string, error)
}

// MacOS returns the associations of macOS through ls (see the package
// documentation and macos.go); nil means the real Launch Services, on
// darwin only (elsewhere, every method returns platform.ErrUnsupported).
func MacOS(ls LaunchServices) platform.Associations {
	if ls == nil {
		if nativeLaunchServices == nil {
			return platform.UnsupportedAssociations{}
		}
		ls = nativeLaunchServices()
	}
	return &macOS{ls: ls, exe: os.Executable, settle: 2 * time.Second, tempDirs: systemTempDirs}
}

// systemTempDirs are the temporary folders whose bundles Launch Services
// ignores: os.TempDir() ($TMPDIR, a folder of /var/folders) and every
// per-user folder of /private/var/folders.
func systemTempDirs() []string { return []string{os.TempDir(), "/private/var/folders"} }

// nativeLaunchServices returns the real Launch Services; set on darwin
// only (launchservices_darwin.go).
var nativeLaunchServices func() LaunchServices

type macOS struct {
	ls LaunchServices
	// exe returns the running executable, the bundle of the calls whose
	// HandlerApp has no Exec.
	exe func() (string, error)
	// settle bounds how long MakeDefault waits for Launch Services to
	// report the default it set.
	settle time.Duration
	// tempDirs returns the temporary folders (see inTempDir).
	tempDirs func() []string
}

// inTempDir returns the temporary folder that holds the bundle, "" when
// none does. Launch Services ignores bundles there: LSRegisterURL returns
// noErr and neither it nor `lsregister -f` makes the bundle known
// (checked on macOS 26 with bundles in /var/folders, the user cache and
// ~/Applications: only the first were never listed, whatever the
// executable). Symbolic links are resolved on both sides (/var is
// /private/var).
func (m *macOS) inTempDir(b *appBundle) string {
	if m.tempDirs == nil {
		return ""
	}
	dir := normDir(b.dir)
	for _, t := range m.tempDirs() {
		if t == "" {
			continue
		}
		root := normDir(t)
		if strings.EqualFold(dir, root) || len(dir) > len(root) && strings.EqualFold(dir[:len(root)], root) &&
			(dir[len(root)] == filepath.Separator || strings.HasSuffix(root, string(filepath.Separator))) {
			return t
		}
	}
	return ""
}

// normDir returns p cleaned, with symbolic links resolved when it exists.
func normDir(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

// tempDirError is the error of Register, MakeDefault and RegisterApp for
// a bundle in a temporary folder.
func tempDirError(call string, b *appBundle, tmp string) error {
	return fmt.Errorf("assoc: %s: the bundle %s is in a temporary folder (%s), and Launch Services ignores apps there: "+
		"build the app with `tela build -o <a folder outside the temporary folder>`, or run it with `tela run`, "+
		"which builds in the user cache: %w", call, b.dir, tmp, platform.ErrUnsupported)
}

var _ platform.Associations = (*macOS)(nil)

// macMu serializes the read-modify-write cycles of the process's calls on
// Info.plist files (the methods are safe from any goroutine). Another
// process is not locked out; the atomic rename keeps the file whole.
var macMu sync.Mutex

// Info.plist keys and values.
const (
	keyURLTypes = "CFBundleURLTypes"
	keyDocTypes = "CFBundleDocumentTypes"
	keyImported = "UTImportedTypeDeclarations"
	keyExported = "UTExportedTypeDeclarations"
	// markerKey marks the entries Tela wrote.
	markerKey = "TelaRegistered"

	docRole  = "Editor"
	docRank  = "Alternate"
	linkRole = "Viewer"
)

// --- the bundle ---

// appBundle is the bundle of a call: its folder, its Info.plist decoded.
type appBundle struct {
	dir, plistPath string
	info           plist.Dict
	sealed         bool // Contents/_CodeSignature exists
}

// bundleDir returns the <Name>.app folder whose Contents/MacOS holds the
// executable exec, symbolic links resolved; false when exec is not there.
func bundleDir(exec string) (string, bool) {
	p := filepath.Clean(exec)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	macOS := filepath.Dir(p)
	contents := filepath.Dir(macOS)
	dir := filepath.Dir(contents)
	if !strings.EqualFold(filepath.Base(macOS), "MacOS") || !strings.EqualFold(filepath.Base(contents), "Contents") ||
		!strings.EqualFold(filepath.Ext(dir), ".app") || filepath.Base(p) == "MacOS" {
		return "", false
	}
	return dir, true
}

// notBundleError is the error of a call whose executable is not inside a
// bundle.
func notBundleError(call, exec, why string) error {
	return fmt.Errorf("assoc: %s: %s %s; macOS registers URL schemes and file types in the app bundle's Info.plist "+
		"(<Name>.app/Contents/MacOS/<program>): build the app with `tela build`, or run it with `tela run`, "+
		"and register from that program (or give its path in HandlerApp.Exec): %w", call, exec, why, platform.ErrUnsupported)
}

// openBundle finds and reads the bundle of app (HandlerApp.Exec, else the
// running executable) and checks that its identifier is the app's ID.
func (m *macOS) openBundle(call string, app platform.HandlerApp) (*appBundle, error) {
	exec := app.Exec
	if exec == "" {
		e, err := m.exe()
		if err != nil {
			return nil, fmt.Errorf("assoc: %s: the program's path: %w (give it in HandlerApp.Exec)", call, err)
		}
		exec = e
	}
	dir, ok := bundleDir(exec)
	if !ok {
		return nil, notBundleError(call, exec, "is not the executable of an app bundle")
	}
	b := &appBundle{dir: dir, plistPath: filepath.Join(dir, "Contents", "Info.plist")}
	data, err := os.ReadFile(b.plistPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, notBundleError(call, exec, "is in a bundle without Contents/Info.plist")
	}
	if err != nil {
		return nil, fmt.Errorf("assoc: %s: %w", call, err)
	}
	if b.info, err = plist.UnmarshalDict(data); err != nil {
		return nil, fmt.Errorf("assoc: %s: %s: %w", call, b.plistPath, err)
	}
	id, _ := b.info["CFBundleIdentifier"].(string)
	if id == "" {
		return nil, notBundleError(call, exec, "is in a bundle whose Info.plist has no CFBundleIdentifier")
	}
	if !strings.EqualFold(id, app.ID) {
		return nil, fmt.Errorf("assoc: %s: the app ID is %q but the bundle %s has the CFBundleIdentifier %q, which is what Launch Services knows it by: "+
			"use one ID for both (App.AppID, desktop.SetAppID or HandlerApp.ID, then rebuild the bundle with `tela build` or `tela run`)",
			call, app.ID, dir, id)
	}
	if fi, err := os.Stat(filepath.Join(dir, "Contents", "_CodeSignature")); err == nil && fi.IsDir() {
		b.sealed = true
	}
	return b, nil
}

// save writes the edited Info.plist.
func (b *appBundle) save(call string) error {
	if b.sealed {
		return fmt.Errorf("assoc: %s: the bundle %s is signed (Contents/_CodeSignature), and editing its Info.plist would break the signature: "+
			"declare the scheme or file type in the Info.plist before signing: %w", call, b.dir, platform.ErrUnsupported)
	}
	data, err := plist.Marshal(b.info)
	if err != nil {
		return fmt.Errorf("assoc: %s: %s: %w", call, b.plistPath, err)
	}
	if err := writeFileAtomic(b.plistPath, data); err != nil {
		return fmt.Errorf("assoc: %s: %w", call, err)
	}
	return nil
}

// register asks Launch Services to read the bundle again.
func (m *macOS) register(call string, b *appBundle) error {
	if err := m.ls.RegisterBundle(b.dir); err != nil {
		return fmt.Errorf("assoc: %s: registering %s with Launch Services: %w", call, b.dir, err)
	}
	return nil
}

// knowsBundle reports whether Launch Services lists the bundle's path for
// id.
func (m *macOS) knowsBundle(b *appBundle, id string) (bool, error) {
	paths, err := m.ls.BundlePaths(id)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(paths, func(p string) bool { return samePath(p, b.dir) }), nil
}

// samePath compares two folder paths after resolving symbolic links
// (/var is /private/var), without regard to case, as macOS volumes are by
// default.
func samePath(a, b string) bool {
	norm := func(p string) string {
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return p
	}
	return strings.EqualFold(norm(a), norm(b))
}

// --- Info.plist entries ---

func entriesOf(d plist.Dict, key string) plist.Array { a, _ := d[key].(plist.Array); return a }

// stringsOf returns a value that is a string or an array of strings as a
// list.
func stringsOf(v any) []string {
	switch v := v.(type) {
	case string:
		return []string{v}
	case plist.Array:
		var out []string
		for _, s := range v {
			if s, ok := s.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func containsFold(list []string, s string) bool {
	return slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, s) })
}

func marked(e plist.Dict) bool { b, _ := e[markerKey].(bool); return b }

// setEntries stores the entries of key, removing the key when none are
// left.
func setEntries(d plist.Dict, key string, a plist.Array) {
	if len(a) == 0 {
		delete(d, key)
		return
	}
	d[key] = a
}

// dropFromList removes s from the list of e[key] (a string or an array);
// it reports whether s was there and whether the list is now empty.
func dropFromList(e plist.Dict, key, s string) (removed, empty bool) {
	list := stringsOf(e[key])
	if !containsFold(list, s) {
		return false, len(list) == 0
	}
	rest := plist.Array{}
	for _, x := range list {
		if !strings.EqualFold(x, s) {
			rest = append(rest, x)
		}
	}
	e[key] = rest
	return true, len(rest) == 0
}

// hasScheme reports whether any CFBundleURLTypes entry declares scheme.
func hasScheme(d plist.Dict, scheme string) bool {
	for _, v := range entriesOf(d, keyURLTypes) {
		if e, ok := v.(plist.Dict); ok && containsFold(stringsOf(e["CFBundleURLSchemes"]), scheme) {
			return true
		}
	}
	return false
}

// addScheme declares scheme for the app unless an entry declares it.
func addScheme(d plist.Dict, id, scheme string) bool {
	if hasScheme(d, scheme) {
		return false
	}
	d[keyURLTypes] = append(slices.Clone(entriesOf(d, keyURLTypes)), plist.Dict{
		"CFBundleURLName":    id + "." + strings.ToLower(scheme),
		"CFBundleURLSchemes": plist.Array{scheme},
		"CFBundleTypeRole":   linkRole,
		markerKey:            true,
	})
	return true
}

// removeScheme removes scheme from the entries Tela wrote, and an entry
// left without schemes.
func removeScheme(d plist.Dict, scheme string) bool {
	changed := false
	var keep plist.Array
	for _, v := range entriesOf(d, keyURLTypes) {
		if e, ok := v.(plist.Dict); ok && marked(e) {
			removed, empty := dropFromList(e, "CFBundleURLSchemes", scheme)
			changed = changed || removed
			if removed && empty {
				continue
			}
		}
		keep = append(keep, v)
	}
	if changed {
		setEntries(d, keyURLTypes, keep)
	}
	return changed
}

// extensionsOf returns the file name extensions a type declaration's
// UTTypeTagSpecification lists.
func extensionsOf(e plist.Dict) []string {
	spec, _ := e["UTTypeTagSpecification"].(plist.Dict)
	return stringsOf(spec["public.filename-extension"])
}

// declaredType returns the identifier of the type the Info.plist declares
// for ext (without its dot; exported declarations first), and whether Tela
// wrote that declaration; "" when it declares none.
func declaredType(d plist.Dict, ext string) (uti string, ours bool) {
	for _, key := range []string{keyExported, keyImported} {
		for _, v := range entriesOf(d, key) {
			e, ok := v.(plist.Dict)
			if !ok || !containsFold(extensionsOf(e), ext) {
				continue
			}
			if id, _ := e["UTTypeIdentifier"].(string); id != "" {
				return id, marked(e)
			}
		}
	}
	return "", false
}

// hasDocType reports whether any CFBundleDocumentTypes entry declares the
// content type uti, or (the older form) the extension ext.
func hasDocType(d plist.Dict, uti, ext string) bool {
	for _, v := range entriesOf(d, keyDocTypes) {
		e, ok := v.(plist.Dict)
		if !ok {
			continue
		}
		if containsFold(stringsOf(e["LSItemContentTypes"]), uti) || containsFold(stringsOf(e["CFBundleTypeExtensions"]), ext) {
			return true
		}
	}
	return false
}

// addDocType declares the app a handler of uti unless an entry does.
func addDocType(d plist.Dict, uti, ext, desc string) bool {
	if hasDocType(d, uti, ext) {
		return false
	}
	d[keyDocTypes] = append(slices.Clone(entriesOf(d, keyDocTypes)), plist.Dict{
		"CFBundleTypeName":   desc,
		"CFBundleTypeRole":   docRole,
		"LSHandlerRank":      docRank,
		"LSItemContentTypes": plist.Array{uti},
		markerKey:            true,
	})
	return true
}

// removeDocType removes uti from the entries Tela wrote, and an entry left
// without content types.
func removeDocType(d plist.Dict, uti string) bool {
	changed := false
	var keep plist.Array
	for _, v := range entriesOf(d, keyDocTypes) {
		if e, ok := v.(plist.Dict); ok && marked(e) {
			removed, empty := dropFromList(e, "LSItemContentTypes", uti)
			changed = changed || removed
			if removed && empty {
				continue
			}
		}
		keep = append(keep, v)
	}
	if changed {
		setEntries(d, keyDocTypes, keep)
	}
	return changed
}

// addImportedType declares the type uti for the extension ext.
func addImportedType(d plist.Dict, a platform.Association, uti, ext string) {
	d[keyImported] = append(slices.Clone(entriesOf(d, keyImported)), plist.Dict{
		"UTTypeIdentifier":  uti,
		"UTTypeDescription": a.Description,
		"UTTypeConformsTo":  plist.Array{"public.data", "public.content"},
		"UTTypeTagSpecification": plist.Dict{
			"public.filename-extension": plist.Array{ext},
			"public.mime-type":          plist.Array{a.MIMEType},
		},
		markerKey: true,
	})
}

// removeImportedType removes the declarations Tela wrote of the type uti.
func removeImportedType(d plist.Dict, uti string) bool {
	changed := false
	var keep plist.Array
	for _, v := range entriesOf(d, keyImported) {
		if e, ok := v.(plist.Dict); ok && marked(e) {
			if id, _ := e["UTTypeIdentifier"].(string); strings.EqualFold(id, uti) {
				changed = true
				continue
			}
		}
		keep = append(keep, v)
	}
	if changed {
		setEntries(d, keyImported, keep)
	}
	return changed
}

// ownTypeID is the identifier of the type Tela declares for an extension
// nothing declares: "<ID>.<ext>", with the characters a Uniform Type
// Identifier does not allow ('_') as '-'.
func ownTypeID(id, ext string) string {
	return utiPart(id) + "." + utiPart(strings.ToLower(ext))
}

func utiPart(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0x80 || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			return r
		}
		return '-'
	}, s)
}

func dynamicType(uti string) bool {
	return uti == "" || strings.HasPrefix(strings.ToLower(uti), "dyn.")
}

func extOf(a platform.Association) string {
	return strings.ToLower(strings.TrimPrefix(a.Extension, "."))
}

// typeOf returns the content type of a's extension: the one the Info.plist
// declares, else the one Launch Services knows (possibly dynamic).
func (m *macOS) typeOf(b *appBundle, a platform.Association) (string, error) {
	ext := extOf(a)
	if uti, _ := declaredType(b.info, ext); uti != "" {
		return uti, nil
	}
	return m.ls.TypeOfExtension(ext)
}

// --- Associations ---

func (m *macOS) Status(app platform.HandlerApp, a platform.Association) (platform.AssociationStatus, error) {
	const call = "Associations.Status"
	platform.CheckHandlerApp(call, app, false)
	platform.CheckAssociation(call, a)
	macMu.Lock()
	defer macMu.Unlock()
	b, err := m.openBundle(call, app)
	if err != nil {
		return platform.AssociationStatus{}, err
	}
	var (
		declared bool
		handlers []string
		def      string
	)
	if a.Scheme != "" {
		declared = hasScheme(b.info, a.Scheme)
		if handlers, err = m.ls.SchemeHandlers(a.Scheme); err == nil {
			def, err = m.ls.DefaultSchemeHandler(a.Scheme)
		}
	} else {
		var uti string
		if uti, err = m.typeOf(b, a); err == nil {
			declared = hasDocType(b.info, uti, extOf(a))
			if handlers, err = m.ls.TypeHandlers(uti); err == nil {
				def, err = m.ls.DefaultTypeHandler(uti)
			}
		}
	}
	if err != nil {
		return platform.AssociationStatus{}, fmt.Errorf("assoc: %s(%v): Launch Services: %w", call, a, err)
	}
	var st platform.AssociationStatus
	// A bundle in a temporary folder is never Launch Services' (another
	// copy of the app may be): it is not registered.
	declared = declared && m.inTempDir(b) == ""
	st.Registered = declared && containsFold(handlers, app.ID)
	switch {
	case strings.EqualFold(def, app.ID):
		st.Default = declared
	case def != "":
		st.Other = def
	default:
		for _, h := range handlers {
			if !strings.EqualFold(h, app.ID) {
				st.Other = h
				break
			}
		}
	}
	return st, nil
}

func (m *macOS) Register(app platform.HandlerApp, a platform.Association, _ platform.RegisterOptions) (bool, error) {
	const call = "Associations.Register"
	platform.CheckHandlerApp(call, app, true)
	platform.CheckAssociation(call, a)
	macMu.Lock()
	defer macMu.Unlock()
	_, _, changed, err := m.registerLocked(call, app, a)
	return changed, err
}

// registerLocked is Register; it also returns the bundle and the content
// type of a file type.
func (m *macOS) registerLocked(call string, app platform.HandlerApp, a platform.Association) (b *appBundle, uti string, changed bool, err error) {
	if b, err = m.openBundle(call, app); err != nil {
		return nil, "", false, err
	}
	if tmp := m.inTempDir(b); tmp != "" {
		return nil, "", false, tempDirError(call, b, tmp)
	}
	if a.Scheme != "" {
		changed = addScheme(b.info, app.ID, a.Scheme)
	} else {
		ext := extOf(a)
		if uti, err = m.typeOf(b, a); err != nil {
			return nil, "", false, fmt.Errorf("assoc: %s(%v): Launch Services: %w", call, a, err)
		}
		if dynamicType(uti) {
			uti = ownTypeID(app.ID, ext)
			addImportedType(b.info, a, uti, ext)
			changed = true
		}
		changed = addDocType(b.info, uti, ext, a.Description) || changed
	}
	if changed {
		if err := b.save(call); err != nil {
			return nil, "", false, err
		}
	} else if listed, err := m.listed(b, app, a, uti); err != nil {
		return nil, "", false, fmt.Errorf("assoc: %s(%v): Launch Services: %w", call, a, err)
	} else if listed {
		return b, uti, false, nil
	}
	if err := m.register(call, b); err != nil {
		return nil, "", false, err
	}
	return b, uti, true, nil
}

// listed reports whether Launch Services lists the app among a's handlers
// and knows the bundle's path.
func (m *macOS) listed(b *appBundle, app platform.HandlerApp, a platform.Association, uti string) (bool, error) {
	var handlers []string
	var err error
	if a.Scheme != "" {
		handlers, err = m.ls.SchemeHandlers(a.Scheme)
	} else {
		handlers, err = m.ls.TypeHandlers(uti)
	}
	if err != nil || !containsFold(handlers, app.ID) {
		return false, err
	}
	return m.knowsBundle(b, app.ID)
}

func (m *macOS) Unregister(app platform.HandlerApp, a platform.Association) (bool, error) {
	const call = "Associations.Unregister"
	platform.CheckHandlerApp(call, app, false)
	platform.CheckAssociation(call, a)
	macMu.Lock()
	defer macMu.Unlock()
	b, err := m.openBundle(call, app)
	if err != nil {
		return false, err
	}
	changed := false
	if a.Scheme != "" {
		changed = removeScheme(b.info, a.Scheme)
	} else {
		ext := extOf(a)
		uti, ours := declaredType(b.info, ext)
		if uti == "" {
			if uti, err = m.ls.TypeOfExtension(ext); err != nil {
				return false, fmt.Errorf("assoc: %s(%v): Launch Services: %w", call, a, err)
			}
		}
		changed = removeDocType(b.info, uti)
		if ours && !hasDocType(b.info, uti, ext) {
			changed = removeImportedType(b.info, uti) || changed
		}
	}
	if !changed {
		return false, nil
	}
	if err := b.save(call); err != nil {
		return false, err
	}
	if err := m.register(call, b); err != nil {
		return false, err
	}
	return true, nil
}

func (m *macOS) MakeDefault(app platform.HandlerApp, a platform.Association) (platform.DefaultResult, error) {
	const call = "Associations.MakeDefault"
	platform.CheckHandlerApp(call, app, true)
	platform.CheckAssociation(call, a)
	macMu.Lock()
	defer macMu.Unlock()
	_, uti, _, err := m.registerLocked(call, app, a)
	if err != nil {
		return platform.DefaultNone, err
	}
	current := func() (string, error) {
		if a.Scheme != "" {
			return m.ls.DefaultSchemeHandler(a.Scheme)
		}
		return m.ls.DefaultTypeHandler(uti)
	}
	// Set even when Launch Services reports the app already: that may be
	// the sole-candidate rule, which another app's registration would end,
	// and MakeDefault records the user's choice.
	if a.Scheme != "" {
		err = m.ls.SetDefaultSchemeHandler(a.Scheme, app.ID)
	} else {
		err = m.ls.SetDefaultTypeHandler(uti, app.ID)
	}
	if err != nil {
		return platform.DefaultNone, fmt.Errorf("assoc: %s(%v): Launch Services: %w", call, a, err)
	}
	// The functions return once the database took the choice; give it a
	// moment anyway before calling the result a failure.
	deadline := time.Now().Add(m.settle)
	for {
		def, err := current()
		if err != nil {
			return platform.DefaultNone, fmt.Errorf("assoc: %s(%v): Launch Services: %w", call, a, err)
		}
		if strings.EqualFold(def, app.ID) {
			return platform.DefaultSet, nil
		}
		if !time.Now().Before(deadline) {
			return platform.DefaultNone, fmt.Errorf("assoc: %s(%v): Launch Services accepted %s as the default handler but reports %q", call, a, app.ID, def)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// --- RegisterApp, UnregisterApp, AppStatus (ADR-217) ---

func (m *macOS) RegisterApp(app platform.HandlerApp, _ platform.RegisterOptions) (bool, error) {
	const call = "Associations.RegisterApp"
	platform.CheckHandlerApp(call, app, true)
	macMu.Lock()
	defer macMu.Unlock()
	b, err := m.openBundle(call, app)
	if err != nil {
		return false, err
	}
	if tmp := m.inTempDir(b); tmp != "" {
		return false, tempDirError(call, b, tmp)
	}
	known, err := m.knowsBundle(b, app.ID)
	if err != nil {
		return false, fmt.Errorf("assoc: %s: Launch Services: %w", call, err)
	}
	if known {
		return false, nil
	}
	if err := m.register(call, b); err != nil {
		return false, err
	}
	return true, nil
}

func (m *macOS) UnregisterApp(app platform.HandlerApp) (bool, error) {
	const call = "Associations.UnregisterApp"
	platform.CheckHandlerApp(call, app, false)
	macMu.Lock()
	defer macMu.Unlock()
	// The bundle is the registration; nothing to withdraw.
	_, err := m.openBundle(call, app)
	return false, err
}

func (m *macOS) AppStatus(app platform.HandlerApp) (platform.AppStatus, error) {
	const call = "Associations.AppStatus"
	platform.CheckHandlerApp(call, app, false)
	macMu.Lock()
	defer macMu.Unlock()
	b, err := m.openBundle(call, app)
	if err != nil {
		return platform.AppStatus{}, err
	}
	paths, err := m.ls.BundlePaths(app.ID)
	if err != nil {
		return platform.AppStatus{}, fmt.Errorf("assoc: %s: Launch Services: %w", call, err)
	}
	var st platform.AppStatus
	st.Registered = len(paths) > 0
	for _, p := range paths {
		if !samePath(p, b.dir) {
			st.Other = p
			break
		}
	}
	return st, nil
}
