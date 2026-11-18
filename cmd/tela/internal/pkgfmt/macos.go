package pkgfmt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/cmd/tela/internal/appbundle"
	"github.com/kuronosu/tela/internal/plist"
)

// The macOS bundle of the declaration (ADR-294, ADR-295): what Info.plist
// holds beyond the bundle's identity, and the entitlements of the hardened
// runtime. tela build (the development bundle) and tela package (the app
// and dmg formats) write the same keys.
//
// Schemes and file types are declared at build time, so a signed bundle
// never needs the registration of the running app (internal/platform/assoc,
// ADR-228) to edit its Info.plist, which the signature seals: registration
// finds each scheme and file type declared and only asks Launch Services
// to read the bundle. The entries written here carry no TelaRegistered
// mark, so registration never changes or removes them.
//
//   - A scheme: a CFBundleURLTypes entry {CFBundleURLName "<ID>.<scheme>",
//     CFBundleURLSchemes [scheme], CFBundleTypeRole Viewer}, as
//     registration writes it (a scheme's role is Viewer).
//   - A file type whose extension has a public type of the system (.txt is
//     public.plain-text, see publicTypes): a CFBundleDocumentTypes entry for
//     that type, rank Alternate, as registration writes it: the app does not
//     own a public format, and an Alternate handler never takes the default
//     from the app that does.
//   - Any other file type: the app owns it. A UTExportedTypeDeclarations
//     entry declares the type <ID>.<ext> (the identifier registration would
//     import, conforming to public.data and public.content, with the
//     extension and the MIME type as its tags), and the document type has
//     rank Owner: the app is the type's creator and the one Launch Services
//     opens its files with. Exported, not imported (registration's choice
//     at run time): a type the bundle defines at build time is the app's
//     own format.
//   - The role: Editor for HandlerDefault and HandlerEditor, Viewer, Shell;
//     HandlerNone writes role None and rank None (the app declares the
//     type, its name and icon, and is never asked to open it).
//
// Document type entries also list the extension (CFBundleTypeExtensions),
// which Launch Services ignores next to LSItemContentTypes, so the
// registration of a running app finds the entry by its extension even when
// the system reports another content type for it than the one written
// here.

// publicTypes are the system's types of common extensions: a file type
// with one of these is declared with the system's type and rank Alternate,
// not exported.
var publicTypes = map[string]string{
	"txt": "public.plain-text", "text": "public.plain-text",
	"md": "net.daringfireball.markdown", "markdown": "net.daringfireball.markdown",
	"rtf": "public.rtf", "html": "public.html", "htm": "public.html",
	"xml": "public.xml", "json": "public.json", "yaml": "public.yaml", "yml": "public.yaml",
	"csv": "public.comma-separated-values-text", "tsv": "public.tab-separated-values-text",
	"pdf": "com.adobe.pdf", "png": "public.png", "jpg": "public.jpeg", "jpeg": "public.jpeg",
	"gif": "com.compuserve.gif", "tif": "public.tiff", "tiff": "public.tiff", "heic": "public.heic",
	"webp": "org.webmproject.webp", "svg": "public.svg-image", "bmp": "com.microsoft.bmp",
	"mp3": "public.mp3", "wav": "com.microsoft.waveform-audio", "aiff": "public.aiff-audio",
	"m4a": "com.apple.m4a-audio", "mp4": "public.mpeg-4", "mov": "com.apple.quicktime-movie",
	"zip": "public.zip-archive", "gz": "org.gnu.gnu-zip-archive", "tar": "public.tar-archive",
	"c": "public.c-source", "h": "public.c-header", "swift": "public.swift-source",
	"py": "public.python-script", "sh": "public.shell-script", "js": "com.netscape.javascript-source",
	"css": "public.css", "ics": "com.apple.ical.ics", "vcf": "public.vcard",
	"epub": "org.idpf.epub-container",
}

// appCategories maps the freedesktop.org main categories of App.Category
// to LSApplicationCategoryType (Apple's app categories), the nearest one:
//
//	AudioVideo, Video  public.app-category.video
//	Audio              public.app-category.music
//	Development        public.app-category.developer-tools
//	Education, Science public.app-category.education
//	Game               public.app-category.games
//	Graphics           public.app-category.graphics-design
//	Office             public.app-category.productivity
//	Network, Settings, System, Utility (and none)
//	                   public.app-category.utilities
var appCategories = map[string]string{
	"AudioVideo":  "video",
	"Video":       "video",
	"Audio":       "music",
	"Development": "developer-tools",
	"Education":   "education",
	"Science":     "education",
	"Game":        "games",
	"Graphics":    "graphics-design",
	"Office":      "productivity",
	"Network":     "utilities",
	"Settings":    "utilities",
	"System":      "utilities",
	"Utility":     "utilities",
}

// AppCategory returns the LSApplicationCategoryType of the app's category.
func AppCategory(app App) string {
	c, ok := appCategories[app.category()]
	if !ok {
		c = "utilities"
	}
	return "public.app-category." + c
}

// usageKeys are the Info.plist usage descriptions of Services: macOS shows
// the text when it asks the user, and ends an app that uses the service
// without one.
var usageKeys = []struct {
	key string
	get func(tela.Services) string
}{
	{"NSCameraUsageDescription", func(s tela.Services) string { return s.Camera }},
	{"NSMicrophoneUsageDescription", func(s tela.Services) string { return s.Microphone }},
	{"NSLocationWhenInUseUsageDescription", func(s tela.Services) string { return s.Location }},
	{"NSContactsUsageDescription", func(s tela.Services) string { return s.Contacts }},
	{"NSCalendarsUsageDescription", func(s tela.Services) string { return s.Calendars }},
	// macOS 14 asks for full access to the calendars with this one.
	{"NSCalendarsFullAccessUsageDescription", func(s tela.Services) string { return s.Calendars }},
	{"NSPhotoLibraryUsageDescription", func(s tela.Services) string { return s.Photos }},
	{"NSBluetoothAlwaysUsageDescription", func(s tela.Services) string { return s.Bluetooth }},
}

// BundleOwned are the Info.plist keys BundleKeys may write
// (appbundle.Spec.Owned): rebuilding a development bundle removes the ones
// the declaration no longer has. UTImportedTypeDeclarations is among them
// although BundleKeys never writes it, so that the types registration
// imported are dropped once the declaration exports them.
var BundleOwned = func() []string {
	keys := []string{"CFBundleIconFile", "CFBundleURLTypes", "CFBundleDocumentTypes", "UTExportedTypeDeclarations",
		"UTImportedTypeDeclarations", "LSApplicationCategoryType", "NSHumanReadableCopyright"}
	for _, u := range usageKeys {
		keys = append(keys, u.key)
	}
	return keys
}()

// IconFile is the name of the app's icon in Contents/Resources,
// <package>.icns; "" when the app has no icon.
func IconFile(app App) string {
	if app.Icon == nil {
		return ""
	}
	return app.Package + ".icns"
}

// utiPart makes s usable in a Uniform Type Identifier, as registration
// does (assoc.ownTypeID): characters other than ASCII letters, digits,
// '-' and '.' (and non-ASCII) become '-'.
func utiPart(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0x80 || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			return r
		}
		return '-'
	}, s)
}

// OwnTypeID is the identifier of the type the app exports for an extension
// (given without its dot): <ID>.<ext>, the one registration would import.
func OwnTypeID(id, ext string) string { return utiPart(id) + "." + utiPart(strings.ToLower(ext)) }

// docRoleRank returns the CFBundleTypeRole and LSHandlerRank of a file
// type; own says the app exports the type.
func docRoleRank(r tela.HandlerRole, own bool) (role, rank string) {
	rank = "Alternate"
	if own {
		rank = "Owner"
	}
	switch r {
	case tela.HandlerViewer:
		return "Viewer", rank
	case tela.HandlerShell:
		return "Shell", rank
	case tela.HandlerNone:
		return "None", "None"
	}
	return "Editor", rank
}

// publisherName is the publisher without an e-mail address ("Example Ltd
// <dev@example.com>" is "Example Ltd").
func publisherName(p string) string {
	if i := strings.IndexByte(p, '<'); i >= 0 && strings.HasSuffix(p, ">") {
		p = p[:i]
	}
	return strings.TrimSpace(p)
}

// BundleKeys returns the Info.plist keys of the declaration (see above):
// the icon, the schemes and file types, the category, the usage
// descriptions and the copyright (from the publisher).
func BundleKeys(app App) plist.Dict {
	d := plist.Dict{"LSApplicationCategoryType": AppCategory(app)}
	if f := IconFile(app); f != "" {
		d["CFBundleIconFile"] = f
	}
	if p := publisherName(app.Publisher); p != "" {
		d["NSHumanReadableCopyright"] = "© " + p
	}
	for _, u := range usageKeys {
		if s := strings.TrimSpace(u.get(app.Services)); s != "" {
			d[u.key] = s
		}
	}
	var urlTypes plist.Array
	seen := map[string]bool{}
	for _, s := range app.Schemes {
		s = strings.ToLower(s)
		if seen[s] {
			continue
		}
		seen[s] = true
		urlTypes = append(urlTypes, plist.Dict{
			"CFBundleURLName":    app.ID + "." + s,
			"CFBundleURLSchemes": plist.Array{s},
			"CFBundleTypeRole":   "Viewer",
		})
	}
	if urlTypes != nil {
		d["CFBundleURLTypes"] = urlTypes
	}
	var docTypes, exported plist.Array
	seen = map[string]bool{}
	for _, ft := range app.FileTypes {
		ext := strings.ToLower(strings.TrimPrefix(ft.Ext, "."))
		if ext == "" || seen[ext] {
			continue
		}
		seen[ext] = true
		uti, public := publicTypes[ext]
		if !public {
			uti = OwnTypeID(app.ID, ext)
			tags := plist.Dict{"public.filename-extension": plist.Array{ext}}
			if ft.MIME != "" {
				tags["public.mime-type"] = plist.Array{ft.MIME}
			}
			exported = append(exported, plist.Dict{
				"UTTypeIdentifier":       uti,
				"UTTypeDescription":      ft.Description,
				"UTTypeConformsTo":       plist.Array{"public.data", "public.content"},
				"UTTypeTagSpecification": tags,
			})
		}
		role, rank := docRoleRank(ft.Role, !public)
		docTypes = append(docTypes, plist.Dict{
			"CFBundleTypeName":       ft.Description,
			"CFBundleTypeRole":       role,
			"LSHandlerRank":          rank,
			"LSItemContentTypes":     plist.Array{uti},
			"CFBundleTypeExtensions": plist.Array{ext},
		})
	}
	if docTypes != nil {
		d["CFBundleDocumentTypes"] = docTypes
	}
	if exported != nil {
		d["UTExportedTypeDeclarations"] = exported
	}
	return d
}

var bundleVersionRE = regexp.MustCompile(`^v?([0-9]+\.[0-9]+\.[0-9]+)`)

// BundleVersion is the version as CFBundleShortVersionString and
// CFBundleVersion take it: the X.Y.Z it starts with (1.2.0-rc.1 is 1.2.0),
// "0.0.0" without one. The program carries the whole version.
func BundleVersion(v string) string {
	if m := bundleVersionRE.FindStringSubmatch(v); m != nil {
		return m[1]
	}
	return "0.0.0"
}

// BundleSpec returns the bundle of app: its identity, version and
// BundleKeys. The icon file is added by WriteBundle.
func BundleSpec(app App) appbundle.Spec {
	return appbundle.Spec{
		ID:      app.ID,
		Name:    app.Name,
		Version: BundleVersion(app.Version),
		Extra:   BundleKeys(app),
		Owned:   BundleOwned,
	}
}

// WriteBundle writes (or updates, appbundle.Write) the bundle of app at
// dir, with the program exe and the icon drawn into work.
func WriteBundle(dir string, app App, exe, work string) (appbundle.Result, error) {
	spec := BundleSpec(app)
	if f := IconFile(app); f != "" {
		icns, err := app.Icon.ICNS()
		if err != nil {
			return appbundle.Result{}, fmt.Errorf("the icon: %v", err)
		}
		p := filepath.Join(work, f)
		if err := os.WriteFile(p, icns, 0o644); err != nil {
			return appbundle.Result{}, err
		}
		spec.Resources = map[string]string{f: p}
	}
	return appbundle.Write(dir, spec, exe)
}

// entitlementKeys are the hardened runtime's entitlements of Services: the
// runtime refuses an app the camera, the microphone and the personal data
// without them, even when the user allows it. Bluetooth has none outside
// the App Sandbox (its usage description is enough).
var entitlementKeys = []struct {
	key string
	get func(tela.Services) string
}{
	{"com.apple.security.device.camera", func(s tela.Services) string { return s.Camera }},
	{"com.apple.security.device.audio-input", func(s tela.Services) string { return s.Microphone }},
	{"com.apple.security.personal-information.location", func(s tela.Services) string { return s.Location }},
	{"com.apple.security.personal-information.addressbook", func(s tela.Services) string { return s.Contacts }},
	{"com.apple.security.personal-information.calendars", func(s tela.Services) string { return s.Calendars }},
	{"com.apple.security.personal-information.photos-library", func(s tela.Services) string { return s.Photos }},
}

// Entitlements returns the app's entitlements file, the property list
// codesign takes with --entitlements: one key per service of Services the
// hardened runtime guards (an empty dictionary when none).
//
// There is no App Sandbox (com.apple.security.app-sandbox): a sandboxed
// app reaches only the files the user picks in the system's own dialogs
// (through a powerbox Tela's file dialogs do not go through yet), cannot
// register global shortcuts (the event tap) and cannot write the Info.plist
// of another bundle or register schemes outside its own, so a sandboxed
// variant of the formats is later work. Network, Notifications, Secrets,
// GlobalShortcuts and Files need no entitlement outside the sandbox, and
// Go programs need none of the runtime's exceptions (no JIT, no unsigned
// executable memory, only system libraries loaded).
func Entitlements(app App) []byte {
	d := plist.Dict{}
	for _, e := range entitlementKeys {
		if strings.TrimSpace(e.get(app.Services)) != "" {
			d[e.key] = true
		}
	}
	b, err := plist.Marshal(d)
	if err != nil {
		panic(err) // booleans always encode
	}
	return b
}
