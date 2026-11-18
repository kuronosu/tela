package pkgfmt

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/cmd/tela/internal/appicon"
)

// The freedesktop.org metadata of an app, shared by the Linux formats:
// the desktop entry, the AppStream metainfo, the shared-mime-info package
// of its file types and the hicolor icons.

// opens reports whether the app opens files of ft (any role but
// HandlerNone).
func opens(ft tela.FileType) bool { return ft.Role != tela.HandlerNone }

// MIMETypes returns the MimeType list of the desktop entry: the file
// types the app opens, then x-scheme-handler/<scheme> for each scheme,
// without duplicates.
func MIMETypes(app App) []string {
	var out []string
	seen := map[string]bool{}
	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	for _, ft := range app.FileTypes {
		if opens(ft) {
			add(ft.MIME)
		}
	}
	for _, s := range app.Schemes {
		add("x-scheme-handler/" + strings.ToLower(s))
	}
	return out
}

// desktopValue escapes a string value of a desktop entry.
func desktopValue(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	return r.Replace(s)
}

// DesktopEntry returns the app's .desktop file, with exec the program's
// name or path: Name, Comment (the description), Exec (with %U when the
// app opens files or URLs), Icon (the app ID, when it has an icon),
// Categories, StartupWMClass (the app ID: the Wayland app_id and the X11
// class Tela sets) and MimeType.
func DesktopEntry(app App, exec string) []byte {
	var b strings.Builder
	b.WriteString("[Desktop Entry]\nType=Application\nVersion=1.5\n")
	fmt.Fprintf(&b, "Name=%s\n", desktopValue(app.Name))
	if d := strings.TrimSpace(app.Description); d != "" {
		fmt.Fprintf(&b, "Comment=%s\n", desktopValue(d))
	}
	mimes := MIMETypes(app)
	if len(mimes) > 0 {
		fmt.Fprintf(&b, "Exec=%s %%U\n", exec)
	} else {
		fmt.Fprintf(&b, "Exec=%s\n", exec)
	}
	if app.Icon != nil {
		fmt.Fprintf(&b, "Icon=%s\n", app.ID)
	}
	b.WriteString("Terminal=false\n")
	fmt.Fprintf(&b, "Categories=%s;\n", app.category())
	fmt.Fprintf(&b, "StartupWMClass=%s\n", app.ID)
	if len(mimes) > 0 {
		fmt.Fprintf(&b, "MimeType=%s;\n", strings.Join(mimes, ";"))
	}
	return []byte(b.String())
}

func xmlText(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// developerID is the reverse-DNS ID of the developer in the metainfo:
// the app ID without its last part.
func developerID(id string) string {
	if i := strings.LastIndexByte(id, '.'); i > 0 {
		return id[:i]
	}
	return id
}

// Metainfo returns the AppStream metainfo of the app (its file is
// <ID>.metainfo.xml): the ID, name, summary and description, the
// developer (Publisher), the desktop entry it launches, the program and
// the MIME types it provides, and one release, the version on the date of
// released. The metadata license is CC0-1.0, the one AppStream
// recommends; the project's own license is not declared.
func Metainfo(app App, released time.Time) []byte {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<component type=\"desktop-application\">\n")
	fmt.Fprintf(&b, "  <id>%s</id>\n", xmlText(app.ID))
	b.WriteString("  <metadata_license>CC0-1.0</metadata_license>\n")
	fmt.Fprintf(&b, "  <name>%s</name>\n", xmlText(app.Name))
	fmt.Fprintf(&b, "  <summary>%s</summary>\n", xmlText(strings.TrimRight(app.summary(), ".")))
	if d := strings.TrimSpace(app.Description); d != "" {
		fmt.Fprintf(&b, "  <description>\n    <p>%s</p>\n  </description>\n", xmlText(d))
	}
	if p := strings.TrimSpace(app.Publisher); p != "" {
		fmt.Fprintf(&b, "  <developer id=\"%s\">\n    <name>%s</name>\n  </developer>\n", xmlText(developerID(app.ID)), xmlText(p))
	}
	fmt.Fprintf(&b, "  <launchable type=\"desktop-id\">%s.desktop</launchable>\n", xmlText(app.ID))
	b.WriteString("  <provides>\n")
	fmt.Fprintf(&b, "    <binary>%s</binary>\n", xmlText(app.Package))
	for _, m := range MIMETypes(app) {
		if !strings.HasPrefix(m, "x-scheme-handler/") {
			fmt.Fprintf(&b, "    <mediatype>%s</mediatype>\n", xmlText(m))
		}
	}
	b.WriteString("  </provides>\n")
	b.WriteString("  <content_rating type=\"oars-1.1\"/>\n")
	fmt.Fprintf(&b, "  <releases>\n    <release version=\"%s\" date=\"%s\"/>\n  </releases>\n", xmlText(app.Version), released.UTC().Format("2006-01-02"))
	b.WriteString("</component>\n")
	return []byte(b.String())
}

// MIMEPackage returns the shared-mime-info package that defines the app's
// file types (its file is mime/packages/<ID>.xml), or nil when it has
// none.
func MIMEPackage(app App) []byte {
	if len(app.FileTypes) == 0 {
		return nil
	}
	type def struct {
		comment string
		globs   []string
	}
	defs := map[string]*def{}
	var order []string
	for _, ft := range app.FileTypes {
		d := defs[ft.MIME]
		if d == nil {
			d = &def{comment: ft.Description}
			defs[ft.MIME] = d
			order = append(order, ft.MIME)
		}
		g := "*" + strings.ToLower(ft.Ext)
		if !contains(d.globs, g) {
			d.globs = append(d.globs, g)
		}
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<mime-info xmlns=\"http://www.freedesktop.org/standards/shared-mime-info\">\n")
	for _, m := range order {
		d := defs[m]
		fmt.Fprintf(&b, "  <mime-type type=\"%s\">\n    <comment>%s</comment>\n", xmlText(m), xmlText(d.comment))
		for _, g := range d.globs {
			fmt.Fprintf(&b, "    <glob pattern=\"%s\"/>\n", xmlText(g))
		}
		b.WriteString("  </mime-type>\n")
	}
	b.WriteString("</mime-info>\n")
	return []byte(b.String())
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// file is a file of a package's tree.
type file struct {
	path   string // slash-separated, relative
	mode   fs.FileMode
	data   []byte
	target string // symbolic links
}

// Icons returns the app's icons in the hicolor theme, under share/icons:
// a PNG per size of appicon.HicolorSizes, and the SVG source as the
// scalable one.
func Icons(app App, share string) ([]file, error) {
	if app.Icon == nil {
		return nil, nil
	}
	pngs, err := app.Icon.Hicolor()
	if err != nil {
		return nil, err
	}
	var out []file
	for _, n := range appicon.HicolorSizes {
		if m := app.Icon.MaxSize(); m != 0 && n > m {
			continue
		}
		out = append(out, file{path: path.Join(share, fmt.Sprintf("icons/hicolor/%dx%d/apps/%s.png", n, n, app.ID)), mode: 0o644, data: pngs[n]})
	}
	if app.IconSVG != nil {
		out = append(out, file{path: path.Join(share, "icons/hicolor/scalable/apps/"+app.ID+".svg"), mode: 0o644, data: app.IconSVG})
	}
	return out, nil
}

// usrFiles is the tree the deb and the AppImage share, under usr: the
// program as bin/<package>, the desktop entry, the metainfo, the MIME
// package and the icons.
func usrFiles(app App, program []byte, released time.Time) ([]file, error) {
	fs := []file{
		{path: "usr/bin/" + app.Package, mode: 0o755, data: program},
		{path: "usr/share/applications/" + app.ID + ".desktop", mode: 0o644, data: DesktopEntry(app, app.Package)},
		{path: "usr/share/metainfo/" + app.ID + ".metainfo.xml", mode: 0o644, data: Metainfo(app, released)},
	}
	if m := MIMEPackage(app); m != nil {
		fs = append(fs, file{path: "usr/share/mime/packages/" + app.ID + ".xml", mode: 0o644, data: m})
	}
	icons, err := Icons(app, "usr/share")
	if err != nil {
		return nil, err
	}
	fs = append(fs, icons...)
	sort.Slice(fs, func(i, j int) bool { return fs[i].path < fs[j].path })
	return fs, nil
}

// readProgram reads the built program.
func readProgram(job Job) ([]byte, error) {
	b, err := os.ReadFile(job.Program)
	if err != nil {
		return nil, fmt.Errorf("reading the built program: %v", err)
	}
	return b, nil
}
