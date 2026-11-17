package pkgfmt

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform/assoc"
)

// msixFormat writes <package>-<version>-windows-<arch>.msix, the package
// Windows installs and updates (ADR-295), in pure Go:
//
//	<package>.exe                   the program
//	Assets/Square44x44Logo.png      the tiles, drawn from the icon
//	Assets/Square150x150Logo.png
//	Assets/StoreLogo.png
//	AppxManifest.xml                from the declaration (MSIXManifest)
//	AppxBlockMap.xml                the SHA-256 of every 64 KiB block
//	[Content_Types].xml
//
// in a zip written as the MSIX SDK's packer writes one (msix_zip.go). The
// package is unsigned unless a signer signs it (signtool, SignPackage),
// which adds AppxSignature.p7x. With MSIXOptions.AppInstallerURL it also
// writes <package>-windows-<arch>.appinstaller, through which Windows
// updates the install: package update reports ErrManaged for MSIX.
type msixFormat struct{ opts MSIXOptions }

// MSIXOptions are tela package's flags for the msix format.
type MSIXOptions struct {
	// Publisher is the package's publisher, a distinguished name that
	// must be the subject of the signing certificate (-msix-publisher).
	// "" derives one (see MSIXPublisher).
	Publisher string
	// AppInstallerURL is the base URL where the .appinstaller file and
	// the .msix are published (-appinstaller-url); "" writes no
	// .appinstaller.
	AppInstallerURL string
	// Signed says a signer will sign the package (-sign with signtool, or
	// -sign-cmd), so a derived publisher has no unsigned marker.
	Signed bool
}

// NewMSIX returns the msix format with the options o; tela package
// registers it in place of the default one.
func NewMSIX(o MSIXOptions) Format { return msixFormat{o} }

func (msixFormat) Name() string { return "msix" }
func (msixFormat) OS() string   { return "windows" }

// MSIXMinVersion is the Windows the package needs: 10.0.17763.0 (Windows
// 10 1809), the first with toasts for desktop apps' activators.
// MSIXMaxVersionTested is the newest it was tested on, Windows 11 24H2.
const (
	MSIXMinVersion       = "10.0.17763.0"
	MSIXMaxVersionTested = "10.0.26100.0"
)

// UnsignedPublisherOID is the attribute that marks the publisher of an
// unsigned package (Windows 11): a package whose Publisher has it installs
// without a signature, with Add-AppxPackage -AllowUnsigned, and cannot be
// signed (its publisher is no certificate's subject).
const UnsignedPublisherOID = "OID.2.25.311729368913984317654407730594956997722=1"

// msixArches are MSIX's names of the Go architectures.
var msixArches = map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}

// The publisher's grammar (ST_Publisher in the manifest schema): RDNs
// such as CN=Name, separated by ", ", a value without ,+="<>#; or quoted.
var (
	dnAttr      = `(CN|L|O|OU|E|C|S|STREET|T|G|I|SN|DC|SERIALNUMBER|Description|PostalCode|POBox|Phone|X21Address|dnQualifier|OID\.(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))+)`
	dnValuePat  = `([^,+="<>#;]+|".*")`
	publisherRE = regexp.MustCompile(`^` + dnAttr + `=` + dnValuePat + `(, ` + dnAttr + `=` + dnValuePat + `)*$`)
	protocolRE  = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]*$`)
)

// CheckPublisher checks a publisher distinguished name against the
// manifest's grammar.
func CheckPublisher(p string) error {
	if len(p) > 8192 || !publisherRE.MatchString(p) {
		return fmt.Errorf("the MSIX publisher %q is not a distinguished name as the manifest takes it: give the certificate's subject, such as \"CN=Example Ltd, O=Example Ltd, C=US\" (attributes separated by \", \", values with , + = \" < > # ; in quotes)", p)
	}
	return nil
}

// reservedNames are the names Windows keeps for devices, which a package
// name may not be.
var reservedNames = []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}

// CheckMSIXName checks that an app ID can be an MSIX package's name: 3 to
// 50 letters, digits, '-' and '.', not a device name.
func CheckMSIXName(id string) error {
	if len(id) < 3 || len(id) > 50 {
		return fmt.Errorf("the app ID %q has %d characters, and an MSIX package name has 3 to 50; pass a shorter one with -id", id, len(id))
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.') {
			return fmt.Errorf("the app ID %q has %q, which an MSIX package name cannot have", id, r)
		}
	}
	for _, n := range reservedNames {
		if strings.EqualFold(id, n) || strings.HasPrefix(strings.ToUpper(id), n+".") {
			return fmt.Errorf("the app ID %q starts with the Windows device name %s, which an MSIX package name cannot", id, n)
		}
	}
	return nil
}

func (f msixFormat) Check(app App, t Target) error {
	if _, ok := msixArches[t.Arch]; !ok {
		return fmt.Errorf("MSIX packages are built for windows/amd64, arm64 and 386, not %s", t)
	}
	if err := CheckMSIXName(app.ID); err != nil {
		return err
	}
	if f.opts.Publisher != "" {
		if err := CheckPublisher(f.opts.Publisher); err != nil {
			return fmt.Errorf("-msix-publisher: %v", err)
		}
	}
	for _, s := range app.Schemes {
		s = strings.ToLower(s)
		if len(s) < 2 || len(s) > 39 || !protocolRE.MatchString(s) {
			return fmt.Errorf("the scheme %q cannot be an MSIX protocol: it has 2 to 39 lowercase letters, digits, '.', '+' and '-', starting with a letter or digit", s)
		}
	}
	return nil
}

// MSIXVersion turns a semantic version into an MSIX one, four numbers of
// at most 65535: 1.2.3 is 1.2.3.0. A pre-release or build metadata has no
// place in it: 1.2.3-rc.1 is 1.2.3.0 too, which note says, since Windows
// updates only to a higher version.
func MSIXVersion(v string) (version, note string, err error) {
	v = strings.TrimPrefix(v, "v")
	core, rest := v, ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		core, rest = v[:i], v[i:]
	}
	parts := strings.Split(core, ".")
	if len(parts) > 3 {
		return "", "", fmt.Errorf("the version %q has more than three numbers", v)
	}
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 65535 {
			return "", "", fmt.Errorf("the version %q cannot be an MSIX version, whose numbers are 0 to 65535; give one with -version", v)
		}
	}
	version = strings.Join(parts, ".") + ".0"
	if rest != "" {
		note = fmt.Sprintf("the MSIX version of %s is %s: MSIX versions are four numbers, without a pre-release, and Windows updates only to a higher one", v, version)
	}
	return version, note, nil
}

// msixPublisherName is the publisher people see: App.Publisher without an
// e-mail address (publisherName), else the app's name.
func msixPublisherName(app App) string {
	if p := publisherName(app.Publisher); p != "" {
		return p
	}
	return app.Name
}

// dnValue writes v as an RDN's value: as it is, or in quotes (with "
// doubled) when it has a character the grammar reserves or surrounding
// spaces.
func dnValue(v string) string {
	if v != "" && !strings.ContainsAny(v, `,+="<>#;`) && strings.TrimSpace(v) == v {
		return v
	}
	return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
}

// MSIXPublisher returns the package's publisher: o.Publisher when given,
// else CN=<publisher's name>, with UnsignedPublisherOID added when the
// package will not be signed.
func MSIXPublisher(app App, o MSIXOptions) string {
	if o.Publisher != "" {
		return o.Publisher
	}
	p := "CN=" + dnValue(msixPublisherName(app))
	if !o.Signed {
		p += ", " + UnsignedPublisherOID
	}
	return p
}

// MSIXIdentity is the Identity of a package.
type MSIXIdentity struct {
	Name, Version, Publisher, Arch string
}

// The tiles of the package, in its Assets folder, and their sizes.
var msixTiles = []struct {
	file string
	size int
}{
	{"Assets/Square150x150Logo.png", 150},
	{"Assets/Square44x44Logo.png", 44},
	{"Assets/StoreLogo.png", 50},
}

// msixCapability is a capability of the manifest and the service it
// comes from.
type msixCapability struct {
	elem, name string
	uses       func(tela.Services) bool
}

// MSIXCapabilities maps the declaration's services (tela.Services) to the
// manifest's capabilities, in the order the schema wants them (the
// general capabilities, then the device ones). runFullTrust is always
// there: a Tela app is a desktop program. Notifications, Secrets and
// GlobalShortcuts need no capability on Windows.
var MSIXCapabilities = []msixCapability{
	{"Capability", "internetClient", func(s tela.Services) bool { return s.Network }},
	{"uap:Capability", "contacts", func(s tela.Services) bool { return s.Contacts != "" }},
	{"uap:Capability", "appointments", func(s tela.Services) bool { return s.Calendars != "" }},
	{"uap:Capability", "picturesLibrary", func(s tela.Services) bool { return s.Photos != "" }},
	{"rescap:Capability", "runFullTrust", func(tela.Services) bool { return true }},
	{"rescap:Capability", "broadFileSystemAccess", func(s tela.Services) bool { return s.Files }},
	{"DeviceCapability", "webcam", func(s tela.Services) bool { return s.Camera != "" }},
	{"DeviceCapability", "microphone", func(s tela.Services) bool { return s.Microphone != "" }},
	{"DeviceCapability", "location", func(s tela.Services) bool { return s.Location != "" }},
	{"DeviceCapability", "bluetooth", func(s tela.Services) bool { return s.Bluetooth != "" }},
}

// fileTypeAssoc is one uap:FileTypeAssociation: the extensions of one
// MIME type.
type fileTypeAssoc struct {
	name, mime, description string
	exts                    []string
}

// fileTypeAssocs groups the file types the app opens by MIME type, in
// their order. A type of role HandlerNone has no association: MSIX
// declares only the types an app opens.
func fileTypeAssocs(fts []tela.FileType) []fileTypeAssoc {
	var out []fileTypeAssoc
	byMIME := map[string]int{}
	names := map[string]bool{}
	for _, ft := range fts {
		if !opens(ft) {
			continue
		}
		ext := strings.ToLower(ft.Ext)
		i, ok := byMIME[ft.MIME]
		if !ok {
			name := strings.TrimPrefix(ext, ".")
			for n := 2; names[name]; n++ {
				name = fmt.Sprintf("%s-%d", strings.TrimPrefix(ext, "."), n)
			}
			names[name] = true
			i = len(out)
			byMIME[ft.MIME] = i
			out = append(out, fileTypeAssoc{name: name, mime: ft.MIME, description: ft.Description})
		}
		if !contains(out[i].exts, ext) {
			out[i].exts = append(out[i].exts, ext)
		}
	}
	return out
}

func winPath(p string) string { return strings.ReplaceAll(p, "/", `\`) }

// MSIXManifest returns the AppxManifest.xml of app with the identity id
// and the program exe (its file name at the package's root):
//
//   - Identity: the app ID as the name, the version, publisher and
//     architecture of id;
//   - Properties: the name, the publisher's name, the store logo;
//   - Dependencies: Windows.Desktop from MSIXMinVersion;
//   - Applications: one, App, the full-trust program with its tiles, a
//     uap:Protocol per scheme, a uap:FileTypeAssociation per MIME type it
//     opens, and the toast activator (ADR-297): a COM server of exe with
//     assoc.ToastActivatorFlag and the class assoc.ToastActivatorCLSID,
//     named as the ToastActivatorCLSID of the toast activation;
//   - Capabilities: MSIXCapabilities.
func MSIXManifest(app App, id MSIXIdentity, exe string) []byte {
	var b strings.Builder
	a := xmlText
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	b.WriteString(`<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"` +
		` xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"` +
		` xmlns:desktop="http://schemas.microsoft.com/appx/manifest/desktop/windows10"` +
		` xmlns:com="http://schemas.microsoft.com/appx/manifest/com/windows10"` +
		` xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"` +
		` IgnorableNamespaces="uap desktop com rescap">` + "\n")
	fmt.Fprintf(&b, "  <Identity Name=\"%s\" Version=\"%s\" Publisher=\"%s\" ProcessorArchitecture=\"%s\"/>\n", a(id.Name), a(id.Version), a(id.Publisher), a(id.Arch))
	b.WriteString("  <Properties>\n")
	fmt.Fprintf(&b, "    <DisplayName>%s</DisplayName>\n", a(app.Name))
	fmt.Fprintf(&b, "    <PublisherDisplayName>%s</PublisherDisplayName>\n", a(msixPublisherName(app)))
	fmt.Fprintf(&b, "    <Logo>%s</Logo>\n", winPath("Assets/StoreLogo.png"))
	b.WriteString("  </Properties>\n")
	b.WriteString("  <Dependencies>\n")
	fmt.Fprintf(&b, "    <TargetDeviceFamily Name=\"Windows.Desktop\" MinVersion=\"%s\" MaxVersionTested=\"%s\"/>\n", MSIXMinVersion, MSIXMaxVersionTested)
	b.WriteString("  </Dependencies>\n")
	b.WriteString("  <Resources>\n    <Resource Language=\"en-us\"/>\n  </Resources>\n")
	b.WriteString("  <Applications>\n")
	fmt.Fprintf(&b, "    <Application Id=\"App\" Executable=\"%s\" EntryPoint=\"Windows.FullTrustApplication\">\n", a(exe))
	fmt.Fprintf(&b, "      <uap:VisualElements DisplayName=\"%s\" Description=\"%s\" BackgroundColor=\"transparent\" Square150x150Logo=\"%s\" Square44x44Logo=\"%s\"/>\n",
		a(app.Name), a(app.summary()), winPath("Assets/Square150x150Logo.png"), winPath("Assets/Square44x44Logo.png"))
	b.WriteString("      <Extensions>\n")
	var schemes []string
	for _, s := range app.Schemes {
		if s = strings.ToLower(s); !contains(schemes, s) {
			schemes = append(schemes, s)
		}
	}
	for _, s := range schemes {
		fmt.Fprintf(&b, "        <uap:Extension Category=\"windows.protocol\">\n          <uap:Protocol Name=\"%s\"/>\n        </uap:Extension>\n", a(s))
	}
	for _, ft := range fileTypeAssocs(app.FileTypes) {
		b.WriteString("        <uap:Extension Category=\"windows.fileTypeAssociation\">\n")
		fmt.Fprintf(&b, "          <uap:FileTypeAssociation Name=\"%s\">\n", a(ft.name))
		if d := strings.TrimSpace(ft.description); d != "" {
			fmt.Fprintf(&b, "            <uap:DisplayName>%s</uap:DisplayName>\n", a(d))
		}
		b.WriteString("            <uap:SupportedFileTypes>\n")
		for _, e := range ft.exts {
			fmt.Fprintf(&b, "              <uap:FileType ContentType=\"%s\">%s</uap:FileType>\n", a(ft.mime), a(e))
		}
		b.WriteString("            </uap:SupportedFileTypes>\n          </uap:FileTypeAssociation>\n        </uap:Extension>\n")
	}
	clsid := assoc.ToastActivatorCLSID(app.ID)
	b.WriteString("        <com:Extension Category=\"windows.comServer\">\n          <com:ComServer>\n")
	fmt.Fprintf(&b, "            <com:ExeServer Executable=\"%s\" Arguments=\"%s\" DisplayName=\"Toast activator\">\n", a(exe), a(assoc.ToastActivatorFlag))
	fmt.Fprintf(&b, "              <com:Class Id=\"%s\" DisplayName=\"Toast activator\"/>\n", clsid)
	b.WriteString("            </com:ExeServer>\n          </com:ComServer>\n        </com:Extension>\n")
	b.WriteString("        <desktop:Extension Category=\"windows.toastNotificationActivation\">\n")
	fmt.Fprintf(&b, "          <desktop:ToastNotificationActivation ToastActivatorCLSID=\"%s\"/>\n", clsid)
	b.WriteString("        </desktop:Extension>\n")
	b.WriteString("      </Extensions>\n    </Application>\n  </Applications>\n")
	b.WriteString("  <Capabilities>\n")
	for _, c := range MSIXCapabilities {
		if c.uses(app.Services) {
			fmt.Fprintf(&b, "    <%s Name=\"%s\"/>\n", c.elem, c.name)
		}
	}
	b.WriteString("  </Capabilities>\n</Package>\n")
	return []byte(b.String())
}

// AppInstallerName is the .appinstaller file's name, the same for every
// version: <package>-windows-<arch>.appinstaller. Its URL must not change,
// since installs read it again to find their updates.
func AppInstallerName(app App, t Target) string {
	return fmt.Sprintf("%s-%s-%s.appinstaller", app.Package, t.OS, t.Arch)
}

// AppInstaller returns the .appinstaller file of the package msix (its
// file name) published under base: App Installer installs from it, and
// the install checks it for a higher version when the app starts
// (OnLaunch, every start) and every 8 hours in the background
// (AutomaticBackgroundTask). An update found at a start is installed
// after the app exits, without asking, as package update's next-start
// activation (ADR-296).
func AppInstaller(id MSIXIdentity, base, self, msix string) []byte {
	base = strings.TrimRight(base, "/")
	a := xmlText
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	fmt.Fprintf(&b, "<AppInstaller xmlns=\"http://schemas.microsoft.com/appx/appinstaller/2018\" Version=\"%s\" Uri=\"%s\">\n", a(id.Version), a(base+"/"+self))
	fmt.Fprintf(&b, "  <MainPackage Name=\"%s\" Publisher=\"%s\" Version=\"%s\" ProcessorArchitecture=\"%s\" Uri=\"%s\"/>\n", a(id.Name), a(id.Publisher), a(id.Version), a(id.Arch), a(base+"/"+msix))
	b.WriteString("  <UpdateSettings>\n    <OnLaunch HoursBetweenUpdateChecks=\"0\"/>\n    <AutomaticBackgroundTask/>\n  </UpdateSettings>\n")
	b.WriteString("</AppInstaller>\n")
	return []byte(b.String())
}

// placeholderTile is the tile of an app without an icon: a gray square.
func placeholderTile(size int) ([]byte, error) {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i := 0; i < len(img.Pix); i += 4 {
		copy(img.Pix[i:], []byte{0x78, 0x71, 0x6c, 0xff})
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f msixFormat) Write(job Job) ([]Output, error) {
	app := job.App
	version, note, err := MSIXVersion(app.Version)
	if err != nil {
		return nil, err
	}
	if note != "" {
		fmt.Fprintf(job.Log, "tela package: %s: %s\n", job.Target, note)
	}
	prog, err := readProgram(job)
	if err != nil {
		return nil, err
	}
	exe := app.Package + ".exe"
	parts := []opcPart{{name: exe, data: prog, deflate: true}}
	for _, t := range msixTiles {
		var png []byte
		if app.Icon != nil {
			png, err = app.Icon.PNG(t.size)
		} else {
			png, err = placeholderTile(t.size)
		}
		if err != nil {
			return nil, err
		}
		// PNG data is compressed already: stored, as makeappx does.
		parts = append(parts, opcPart{name: t.file, data: png})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].name < parts[j].name })
	id := MSIXIdentity{Name: app.ID, Version: version, Publisher: MSIXPublisher(app, f.opts), Arch: msixArches[job.Target.Arch]}
	b, err := writeMSIX(job.Time, parts, MSIXManifest(app, id, exe))
	if err != nil {
		return nil, err
	}
	name := FileName(app, job.Target, ".msix")
	p, err := writeOut(job.OutDir, name, b, 0o644)
	if err != nil {
		return nil, err
	}
	out, err := sign(job, p)
	if err != nil {
		return []Output{out}, err
	}
	switch {
	case out.Signed:
	case strings.Contains(id.Publisher, UnsignedPublisherOID):
		out.Note = "install it with Add-AppxPackage -AllowUnsigned (Windows 11)"
	default:
		out.Note = "a package must be signed by its publisher's certificate to install"
	}
	outs := []Output{out}
	if f.opts.AppInstallerURL == "" {
		outs[0].Note += "; no .appinstaller, through which installs update (give -appinstaller-url)"
		return outs, nil
	}
	self := AppInstallerName(app, job.Target)
	ai, err := writeOut(job.OutDir, self, AppInstaller(id, f.opts.AppInstallerURL, self, name), 0o644)
	if err != nil {
		return outs, err
	}
	return append(outs, Output{Path: ai, Note: "publish it and " + name + " at " + strings.TrimRight(f.opts.AppInstallerURL, "/") + "/"}), nil
}

// opcName is the zip item name of a package file (a part name without
// its leading '/'): each byte outside the URI's unreserved and sub-delim
// characters percent-encoded, as the Open Packaging Conventions write
// part names.
func opcName(name string) string {
	var b strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', strings.IndexByte("-._~!$&'()*+,;=@/", c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// contentType is the content type of a package file by its extension.
func contentType(ext string) string {
	switch ext {
	case "exe", "dll":
		return "application/x-msdownload"
	case "png":
		return "image/png"
	case "json":
		return "application/json"
	case "txt":
		return "text/plain"
	}
	return "application/octet-stream"
}

// contentTypes returns [Content_Types].xml for the payload's names: a
// Default per extension, an Override per name without one, and the
// manifest's and block map's.
func contentTypes(names []string) []byte {
	defaults := map[string]bool{}
	var overrides []string
	for _, n := range names {
		ext := strings.ToLower(strings.TrimPrefix(path.Ext(n), "."))
		if ext == "" {
			overrides = append(overrides, n)
			continue
		}
		defaults[ext] = true
	}
	var exts []string
	for e := range defaults {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	for _, e := range exts {
		fmt.Fprintf(&b, `<Default Extension="%s" ContentType="%s"/>`, xmlText(e), contentType(e))
	}
	for _, n := range overrides {
		fmt.Fprintf(&b, `<Override PartName="/%s" ContentType="application/octet-stream"/>`, xmlText(opcName(n)))
	}
	b.WriteString(`<Override PartName="/AppxManifest.xml" ContentType="application/vnd.ms-appx.manifest+xml"/>`)
	b.WriteString(`<Override PartName="/AppxBlockMap.xml" ContentType="application/vnd.ms-appx.blockmap+xml"/>`)
	b.WriteString("</Types>\n")
	return []byte(b.String())
}
