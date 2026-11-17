package main

import (
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela/cmd/tela/internal/appbundle"
	"github.com/kuronosu/tela/cmd/tela/internal/appicon"
	"github.com/kuronosu/tela/cmd/tela/internal/pkgfmt"
	"github.com/kuronosu/tela/internal/platform"
)

const packageUsage = `Usage:

	tela package [-format list] [-os list] [-arch list] [-o dir] [-version V]
	             [-icon file] [-id ID] [-name NAME] [-sign] [-sign-cmd command]
	             [-msix-publisher DN] [-appinstaller-url URL] [-cert-file f]
	             [-cert-password-file f] [-cert-sha1 hash] [-timestamp-url URL]
	             [-console] [-tags list] [-ldflags flags] [-gcflags flags]
	             [-trimpath] [-v] [pkg]

Builds the program of a package main (default ".") for each target, as tela
build does, and writes its packages, from any machine, without other tools
(ADR-295). The targets are every -os with every -arch (default: the GOOS and
GOARCH of the environment):

	tela package -os linux -arch amd64,arm64 ./cmd/notes

The formats (-format, comma-separated; default: each OS's defaults):

	linux    appimage  (default) an AppImage on Tela's own runtime, which
	                   updates itself (package update)
	         deb       (default) a Debian package
	         flatpak   a flatpak-builder manifest and the folder it builds
	                   from; the .flatpak bundle too when flatpak-builder
	                   and flatpak are installed
	windows  zip       (default) the program in a folder, which updates
	                   itself (package update)
	         msix      (default) an MSIX package, and its .appinstaller
	                   with -appinstaller-url
	darwin   app (default), dmg: not implemented yet

Each package file is named <name>-<version>-<os>-<arch>.<ext>, as tela
update manifest reads it, in the folder of -o (default ./dist). <name> is
the package name: the last part of the app ID, lowercase (com.example.Notes
is notes).

What goes into the packages comes from the package's tela.App, read without
running the program (see "tela help build"): AppID (required), Title (the
name; -name replaces it), Publisher, Description, Category (a freedesktop.org
main category, default Utility), Schemes, FileTypes and Services. They are
checked before anything is built: the schemes and file types as tela.Run
checks them, the category, and what each format needs (flatpak wants an ID
of three parts or more). The icon is icon.svg or icon.png (512 px or more)
in the package's folder, or -icon; every size is drawn from it. The version
is -version, else the main module's as the go command stamps it, else 0.0.0,
and is written into the program.

On Linux, the packages hold the program as usr/bin/<name>, a desktop entry
<AppID>.desktop (Name, Comment, Exec, Icon, Categories, StartupWMClass and
MimeType from the file types and schemes), the AppStream metainfo
<AppID>.metainfo.xml (name, summary, developer, the release and its date),
a shared-mime-info package for the file types and the icons of the hicolor
theme. A deb's Maintainer is App.Publisher ("unknown" without one, which tela
says). A flatpak's permissions come from App.Services: see the flatpak
section of docs/wiki/CLI.md.

On Windows, the zip holds the folder <name>/ with <name>.exe (a GUI program
with its icon and version information, as tela build makes it) and
tela-install.json, the marker through which package update finds the
install and replaces what the package put there. The MSIX holds <name>.exe,
its tiles and the AppxManifest.xml written from the declaration: the
identity (the app ID, the version as four numbers: 1.2.3 is 1.2.3.0, and a
pre-release has no place in it), Windows 10 1809 or later, a protocol per
scheme, a file type association per MIME type the app opens, the toast
activator, runFullTrust and the capabilities of App.Services. Its publisher
must be the subject of the certificate that signs it: give it with
-msix-publisher. Without it the publisher is CN=<App.Publisher>, and when
signtool does not sign the package it carries Windows 11's mark of unsigned
packages, so that it installs with
"Add-AppxPackage -AllowUnsigned <file>". With -appinstaller-url, the base
URL where the files are published, tela writes
<name>-windows-<arch>.appinstaller too, the file people install from and
through which Windows updates the install (at each start, and every 8
hours); package update leaves MSIX installs to it.

The AppImage starts with Tela's own runtime, written in Go without FUSE: on
the first start of a version it checks the content's SHA-256, extracts it
into ~/.cache/tela-appimage/<AppID>/<version>-<hash>/ under a lock (a second
instance waits for it), then runs it from there; later starts check that the
extracted files are unchanged (and extract again when they are not), and
remove the versions no running instance uses. It takes the options
--appimage-extract, --appimage-offset, --appimage-help and the like.

The same inputs give the same bytes (the flatpak bundle excepted, which
flatpak writes): every time in the packages is SOURCE_DATE_EPOCH when it is
set, else the time of the commit the go command stamped into the program,
else 1980-01-01; entries are sorted, owned by root, with fixed modes.

Signing: Linux packages have no signing tool to run, so -sign does nothing
there and says so. For Windows targets on Windows, -sign runs signtool (from
PATH or the Windows SDK) on each program before it is packaged and on each
MSIX: "signtool sign /fd SHA256 /tr <-timestamp-url> /td SHA256" with the
certificate of -cert-file (a .pfx; its password in -cert-password-file) or
-cert-sha1 (a thumbprint in the certificate store), else the best one in the
store (/a). Elsewhere signtool is not available, which tela says: sign with
-sign-cmd and a signer that runs there, such as jsign. -sign-cmd runs a command on each package file (and on
Windows on each program before it is packaged): every argument {} is replaced
by the file's path, else the path is added at the end:

	tela package -sign-cmd "gpg --detach-sign --armor" .

The output says which files are signed; unsigned packages are written all
the same.

Flags:

	-format list   the formats (default: appimage,deb for linux, zip,msix
	               for windows, app for darwin)
	-os list       the target systems (default: GOOS)
	-arch list     the target architectures (default: GOARCH)
	-o dir         where to write the packages (default ./dist)
	-version V     the version, such as 1.2.3 or 1.2.3-rc.1 (default: the
	               module's)
	-icon file     the icon, icon.svg or icon.png (default: the one in the
	               package's folder)
	-id ID         the app ID instead of App.AppID
	-name NAME     the name instead of App.Title
	-sign          sign with the system's tools (none on Linux; signtool
	               for Windows targets on Windows)
	-sign-cmd cmd  run cmd on each file to sign
	-msix-publisher DN
	               the MSIX publisher, the signing certificate's subject,
	               such as "CN=Example Ltd, O=Example Ltd, C=US"
	-appinstaller-url URL
	               write the .appinstaller for the files published at URL
	-cert-file f, -cert-password-file f, -cert-sha1 hash
	               signtool's certificate (with -sign)
	-timestamp-url URL
	               signtool's time-stamp server (default
	               http://timestamp.digicert.com)
	-console       Windows programs keep the console subsystem
	-tags, -ldflags, -gcflags, -trimpath
	               as for go build (see "tela help build")
	-v             say where each value came from
`

// packageOptions are the arguments of tela package.
type packageOptions struct {
	build   buildOptions
	formats []string
	oses    []string
	arches  []string
	out     string
	sign    bool
	signCmd []string
	// The Windows formats' flags: the MSIX publisher and App Installer
	// URL, signtool's certificate and time-stamp server.
	msixPublisher, appInstallerURL       string
	certFile, certPasswordFile, certSHA1 string
	timestampURL                         string
	// signtool is signtool.exe's path when -sign runs it (packageAll).
	signtool string
}

func splitList(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// parsePackageArgs parses the arguments of tela package.
func parsePackageArgs(args []string) (packageOptions, error) {
	var o packageOptions
	var formats, oses, arches, signCmd string
	fs := flag.NewFlagSet("tela package", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	o.build.register(fs)
	fs.StringVar(&o.build.Icon, "icon", "", "")
	fs.BoolVar(&o.build.Console, "console", false, "")
	fs.StringVar(&formats, "format", "", "")
	fs.StringVar(&oses, "os", "", "")
	fs.StringVar(&arches, "arch", "", "")
	fs.StringVar(&o.out, "o", "dist", "")
	fs.BoolVar(&o.sign, "sign", false, "")
	fs.StringVar(&signCmd, "sign-cmd", "", "")
	fs.StringVar(&o.msixPublisher, "msix-publisher", "", "")
	fs.StringVar(&o.appInstallerURL, "appinstaller-url", "", "")
	fs.StringVar(&o.certFile, "cert-file", "", "")
	fs.StringVar(&o.certPasswordFile, "cert-password-file", "", "")
	fs.StringVar(&o.certSHA1, "cert-sha1", "", "")
	fs.StringVar(&o.timestampURL, "timestamp-url", "", "")
	pkgs, err := parsePositionals("package", fs, args)
	if err != nil {
		return o, err
	}
	switch len(pkgs) {
	case 0:
		o.build.Pkg = "."
	case 1:
		o.build.Pkg = pkgs[0]
	default:
		return o, fmt.Errorf("expected at most one <pkg>, got %d (%s)", len(pkgs), strings.Join(pkgs, ", "))
	}
	o.formats, o.oses, o.arches = splitList(formats), splitList(oses), splitList(arches)
	if signCmd != "" {
		if o.signCmd = splitGoFlags(signCmd); len(o.signCmd) == 0 {
			return o, fmt.Errorf("-sign-cmd %q names no command", signCmd)
		}
	}
	if o.out == "" {
		return o, errors.New("-o is empty; give the folder to write the packages in")
	}
	if err := o.validateWindows(); err != nil {
		return o, err
	}
	return o, o.build.validate("package")
}

// validateWindows checks the flags of the Windows formats.
func (o packageOptions) validateWindows() error {
	if o.msixPublisher != "" {
		if err := pkgfmt.CheckPublisher(o.msixPublisher); err != nil {
			return fmt.Errorf("-msix-publisher: %v", err)
		}
	}
	if u := o.appInstallerURL; u != "" {
		pu, err := url.Parse(u)
		if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") || pu.Host == "" || pu.RawQuery != "" || pu.Fragment != "" {
			return fmt.Errorf("-appinstaller-url %q is not an http or https URL without a query: give the URL of the folder where the .appinstaller and .msix files go, such as https://example.com/notes", u)
		}
	}
	switch {
	case o.certFile != "" && o.certSHA1 != "":
		return errors.New("-cert-file and -cert-sha1 both choose the certificate; give one")
	case o.certPasswordFile != "" && o.certFile == "":
		return errors.New("-cert-password-file is the password of -cert-file, which is missing")
	case !o.sign && (o.certFile != "" || o.certSHA1 != "" || o.timestampURL != ""):
		return errors.New("-cert-file, -cert-sha1 and -timestamp-url are for signtool, which only -sign runs; add -sign")
	}
	return nil
}

// runPackage implements tela package.
func runPackage(args []string, stdout, stderr io.Writer) int {
	opts, err := parsePackageArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, packageUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela package: %v\n\n%s", err, packageUsage)
		return 2
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintln(stderr, "tela package: the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
		return 1
	}
	outs, err := packageAll(goBin, opts, stderr)
	for _, o := range outs {
		line := "wrote " + o.Path
		switch {
		case o.Signed:
			line += " (signed)"
		case strings.HasSuffix(o.Path, ".appinstaller"):
		case !strings.HasSuffix(o.Path, "-flatpak"):
			line += " (unsigned)"
		}
		if o.Note != "" {
			line += ": " + o.Note
		}
		fmt.Fprintln(stdout, line)
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela package: %v\n", err)
		return 1
	}
	return 0
}

// packagePlan is the work for one target, checked before any build.
type packagePlan struct {
	target  pkgfmt.Target
	opts    buildOptions
	bt      *buildTarget
	app     pkgfmt.App
	formats []pkgfmt.Format
}

// packageAll checks every target, then builds and packages each one.
func packageAll(goBin string, opts packageOptions, log io.Writer) ([]pkgfmt.Output, error) {
	if opts.sign && runtime.GOOS == "windows" {
		opts.signtool = pkgfmt.FindSigntool(runtime.GOARCH)
	}
	pkgfmt.Register(pkgfmt.NewMSIX(pkgfmt.MSIXOptions{
		Publisher: opts.msixPublisher, AppInstallerURL: opts.appInstallerURL,
		Signed: opts.signtool != "",
	}))
	plans, err := planPackages(goBin, opts, log)
	if err != nil {
		return nil, err
	}
	signer, err := packageSigner(opts, plans, log)
	if err != nil {
		return nil, err
	}
	out, err := filepath.Abs(opts.out)
	if err != nil {
		return nil, err
	}
	if plans[0].app.Icon == nil {
		fmt.Fprintf(log, "tela package: %s has no icon, so its packages have none (desktops show a generic one); put an icon.svg or an icon.png (512 px or more) next to main.go, or pass -icon\n", plans[0].app.Name)
	}
	var outs []pkgfmt.Output
	for _, p := range plans {
		o, err := packageTarget(goBin, opts, p, out, signer, log)
		outs = append(outs, o...)
		if err != nil {
			return outs, fmt.Errorf("%s: %v", p.target, err)
		}
	}
	return outs, nil
}

// planPackages resolves the targets and their formats and checks the
// declaration for each.
func planPackages(goBin string, opts packageOptions, log io.Writer) ([]packagePlan, error) {
	oses, arches := opts.oses, opts.arches
	if len(oses) == 0 || len(arches) == 0 {
		cmd := exec.Command(goBin, "env", "-json", "GOOS", "GOARCH")
		cmd.Env = goEnv()
		b, err := cmd.Output()
		var env struct{ GOOS, GOARCH string }
		if err == nil {
			err = json.Unmarshal(b, &env)
		}
		if err != nil {
			return nil, fmt.Errorf("go env: %v", err)
		}
		if len(oses) == 0 {
			oses = []string{env.GOOS}
		}
		if len(arches) == 0 {
			arches = []string{env.GOARCH}
		}
	}
	// The formats of each OS.
	byOS := map[string][]pkgfmt.Format{}
	if len(opts.formats) > 0 {
		for _, n := range opts.formats {
			f, ok := pkgfmt.Lookup(n)
			if !ok {
				return nil, fmt.Errorf("-format %s: no such format; the formats are %s", n, strings.Join(pkgfmt.Names(), ", "))
			}
			if !slices.Contains(oses, f.OS()) {
				return nil, fmt.Errorf("-format %s is a format for %s, which is not among the targets (-os %s)", n, f.OS(), strings.Join(oses, ","))
			}
			byOS[f.OS()] = append(byOS[f.OS()], f)
		}
	}
	for _, goos := range oses {
		if len(opts.formats) > 0 {
			if len(byOS[goos]) == 0 {
				return nil, fmt.Errorf("-format %s has no format for %s; its formats are %s", strings.Join(opts.formats, ","), goos, strings.Join(pkgfmt.Defaults(goos), ", "))
			}
			continue
		}
		names := pkgfmt.Defaults(goos)
		if len(names) == 0 {
			return nil, fmt.Errorf("tela package does not package for %s; the systems are linux, windows and darwin", goos)
		}
		for _, n := range names {
			f, _ := pkgfmt.Lookup(n)
			byOS[goos] = append(byOS[goos], f)
		}
	}

	var icon *appicon.Source
	var iconSVG []byte
	iconRead := false
	var plans []packagePlan
	for _, goos := range oses {
		for _, goarch := range arches {
			t := pkgfmt.Target{OS: goos, Arch: goarch}
			o := opts.build
			o.Env = []string{"GOOS=" + goos, "GOARCH=" + goarch}
			bt, err := resolveBuildTarget(goBin, o, "package")
			if err != nil {
				return nil, fmt.Errorf("%s: %v", t, err)
			}
			app, err := packageApp(o, bt, log)
			if err != nil {
				return nil, err
			}
			if !iconRead {
				iconRead = true
				path := o.Icon
				if path == "" {
					path = appicon.Find(bt.pkg.Dir)
				}
				if path != "" {
					if icon, err = appicon.Load(path); err != nil {
						return nil, err
					}
					if strings.EqualFold(filepath.Ext(path), ".svg") {
						if iconSVG, err = os.ReadFile(path); err != nil {
							return nil, err
						}
					}
					if o.Verbose {
						fmt.Fprintf(log, "icon %s\n", path)
					}
				}
			}
			app.Icon, app.IconSVG = icon, iconSVG
			for _, f := range byOS[goos] {
				if err := f.Check(app, t); err != nil {
					return nil, fmt.Errorf("%s: %s: %v", t, f.Name(), err)
				}
			}
			plans = append(plans, packagePlan{target: t, opts: o, bt: bt, app: app, formats: byOS[goos]})
		}
	}
	return plans, nil
}

// packageApp reads and checks the declaration of the package for one
// target (build constraints may give each its own files).
func packageApp(o buildOptions, bt *buildTarget, log io.Writer) (pkgfmt.App, error) {
	src, err := readAppSource(bt.pkg, bt.deps)
	if err != nil {
		return pkgfmt.App{}, err
	}
	if err := src.DeclarationErr(); err != nil {
		return pkgfmt.App{}, fmt.Errorf("%v\nmake it a constant: the packages are written from it", err)
	}
	app := pkgfmt.App{
		Publisher:   strings.TrimSpace(src.Publisher),
		Description: strings.TrimSpace(src.Description),
		Category:    strings.TrimSpace(src.Category),
		Schemes:     src.Schemes,
		FileTypes:   src.FileTypes,
		Services:    src.Services,
	}
	idFrom := "-id"
	app.ID = o.ID
	if app.ID == "" {
		idFrom = "App.AppID"
		switch {
		case src.IDErr != nil:
			return app, fmt.Errorf("%v\npass the ID with -id instead: tela package -id com.example.app %s", src.IDErr, o.Pkg)
		case src.ID == "":
			return app, fmt.Errorf("package %s has no app ID: the packages are named by it (the desktop entry, the metainfo, the AppImage's cache folder), so set tela.App.AppID, for example AppID: \"com.example.notes\", or pass -id", bt.pkg.ImportPath)
		}
		app.ID = src.ID
	}
	if err := appbundle.ValidID(app.ID); err != nil {
		return app, fmt.Errorf("%v (from %s)", err, idFrom)
	}
	nameFrom := "-name"
	app.Name = strings.TrimSpace(o.Name)
	if app.Name == "" {
		if src.TitleErr != nil {
			return app, fmt.Errorf("%v\npass the name with -name instead: tela package -name \"My App\" %s", src.TitleErr, o.Pkg)
		}
		nameFrom, app.Name = "App.Title", strings.TrimSpace(src.Title)
	}
	if app.Name == "" {
		nameFrom, app.Name = "the package name", packageBaseName(bt.pkg)
	}
	app.Package = pkgfmt.PackageName(app.ID, app.Name)
	if err := checkDeclaration(app); err != nil {
		return app, err
	}
	if o.Verbose {
		fmt.Fprintf(log, "%s/%s: app ID %q (from %s), name %q (from %s), package name %q\n", bt.goos, bt.goarch, app.ID, idFrom, app.Name, nameFrom, app.Package)
	}
	return app, nil
}

// checkDeclaration applies the rules of tela.Run's dev-mode checks to the
// schemes and file types (platform.CheckAssociation, which panics) and
// checks the category.
func checkDeclaration(app pkgfmt.App) (err error) {
	defer func() {
		if r := recover(); r != nil {
			msg := strings.TrimPrefix(fmt.Sprint(r), "platform: ")
			err = fmt.Errorf("the tela.App declaration: %s", msg)
		}
	}()
	for _, s := range app.Schemes {
		if strings.Contains(s, ":") {
			return fmt.Errorf("the tela.App declaration: App.Schemes: %q has a ':'; give the scheme without it (%q)", s, strings.TrimRight(s, ":/"))
		}
		platform.CheckAssociation("App.Schemes", platform.Association{Scheme: s})
	}
	for i, ft := range app.FileTypes {
		what := fmt.Sprintf("App.FileTypes[%d]", i)
		if ft.Ext == "" || ft.Ext[0] != '.' {
			return fmt.Errorf("the tela.App declaration: %s: the extension %q needs its dot, such as \".note\"", what, ft.Ext)
		}
		platform.CheckAssociation(what, platform.Association{
			Extension: ft.Ext, MIMEType: ft.MIME, Description: ft.Description, Role: platform.HandlerRole(ft.Role)})
	}
	if err := pkgfmt.CheckCategory(app.Category); err != nil {
		return fmt.Errorf("the tela.App declaration: %v", err)
	}
	return nil
}

// fixedEpoch is the time of the packages when nothing gives one: the
// earliest a zip file can record.
var fixedEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// packageTime returns the time of every file in the packages, and where it
// came from: SOURCE_DATE_EPOCH, the commit time the go command stamped
// into the program (vcs.time), or fixedEpoch.
func packageTime(exe string) (time.Time, string, error) {
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || n < 0 || n > 1<<32-1 {
			return time.Time{}, "", fmt.Errorf("SOURCE_DATE_EPOCH %q is not a count of seconds since 1970 (before 2106)", s)
		}
		return time.Unix(n, 0).UTC(), "SOURCE_DATE_EPOCH", nil
	}
	if info, err := buildinfo.ReadFile(exe); err == nil {
		for _, s := range info.Settings {
			if s.Key == "vcs.time" {
				if t, err := time.Parse(time.RFC3339, s.Value); err == nil && t.Unix() >= 0 {
					return t.UTC(), "the commit (vcs.time)", nil
				}
			}
		}
	}
	return fixedEpoch, "the fixed time (no SOURCE_DATE_EPOCH, no commit stamped)", nil
}

// toolVersion is tela's version, as tela version prints it.
func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// packageSigner returns the signer of -sign and -sign-cmd for the targets
// of plans: -sign adds each system's signing tools to the chain (Linux has
// none, which it says), -sign-cmd a CommandSigner.
func packageSigner(opts packageOptions, plans []packagePlan, log io.Writer) (pkgfmt.Signer, error) {
	var ch pkgfmt.Chain
	if opts.sign && slices.ContainsFunc(plans, func(p packagePlan) bool { return p.target.OS == "linux" }) {
		fmt.Fprintln(log, "tela package: -sign: there is no signing tool to run for Linux packages; sign them with -sign-cmd (for example -sign-cmd \"gpg --detach-sign --armor\")")
	}
	if opts.sign && slices.ContainsFunc(plans, func(p packagePlan) bool { return p.target.OS == "windows" }) {
		switch {
		case runtime.GOOS != "windows":
			fmt.Fprintf(log, "tela package: -sign: signtool is not available on %s, so it signs no Windows program or MSIX package; sign them with -sign-cmd and a signer that runs here, such as jsign (-sign-cmd \"jsign --keystore cert.p12 --storepass ...\"), or package on Windows\n", runtime.GOOS)
		case opts.signtool == "":
			fmt.Fprintln(log, "tela package: -sign: signtool was not found (in PATH or the Windows SDK), so the Windows programs and MSIX packages are not signed; install the Windows SDK's signing tools, or sign with -sign-cmd")
		default:
			if opts.msixPublisher == "" && slices.ContainsFunc(plans, func(p packagePlan) bool {
				return slices.ContainsFunc(p.formats, func(f pkgfmt.Format) bool { return f.Name() == "msix" })
			}) {
				fmt.Fprintf(log, "tela package: -sign: the MSIX publisher is %s, which must be the signing certificate's subject; give the subject with -msix-publisher\n", pkgfmt.MSIXPublisher(plans[0].app, pkgfmt.MSIXOptions{Signed: true}))
			}
			ch = append(ch, pkgfmt.Signtool{Path: opts.signtool, CertFile: opts.certFile, PasswordFile: opts.certPasswordFile,
				CertSHA1: opts.certSHA1, TimestampURL: opts.timestampURL, Out: log})
		}
	}
	if len(opts.signCmd) > 0 {
		if _, err := exec.LookPath(opts.signCmd[0]); err != nil {
			return nil, fmt.Errorf("-sign-cmd: %v", err)
		}
		ch = append(ch, pkgfmt.CommandSigner{Args: opts.signCmd, Out: log})
	}
	return ch, nil
}

// packageTarget builds the program for one target and writes its packages
// into out.
func packageTarget(goBin string, opts packageOptions, p packagePlan, out string, signer pkgfmt.Signer, log io.Writer) ([]pkgfmt.Output, error) {
	work, err := os.MkdirTemp("", "tela-package-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	o := p.opts
	name := p.app.Package
	if p.target.OS == "windows" {
		name += ".exe"
	}
	exe := filepath.Join(work, name)
	gui := p.target.OS == "windows" && !o.Console
	o.Ldflags = linkerFlags(o, p.bt.goflags, gui)
	var res *winResources
	if p.target.OS == "windows" {
		if res, err = newWinResources(o, p.bt, name, log); err != nil {
			return nil, err
		}
	}
	if err := goBuild(goBin, o, exe, log); err != nil {
		return nil, err
	}
	if res != nil {
		if err := res.apply(exe, log); err != nil {
			return nil, err
		}
	}
	// A Windows program is signed once, after its resources (winres
	// refuses a signed file) and before it goes into the zip and MSIX.
	programSigned := false
	if p.target.OS == "windows" && signer != nil {
		if programSigned, err = signer.Sign(exe, pkgfmt.SignProgram, p.target); err != nil {
			return nil, fmt.Errorf("signing %s: %v", name, err)
		}
	}
	app := p.app
	app.Version = strings.TrimPrefix(o.Version, "v")
	from := "-version"
	if app.Version == "" {
		from, app.Version = "the module", moduleVersionFull(exe)
	}
	if app.Version == "" {
		from, app.Version = "no version known", "0.0.0"
	}
	if app.Version[0] < '0' || app.Version[0] > '9' {
		return nil, fmt.Errorf("the version %q does not start with a digit; give one such as 1.2.3 with -version", app.Version)
	}
	t, tFrom, err := packageTime(exe)
	if err != nil {
		return nil, err
	}
	if o.Verbose {
		fmt.Fprintf(log, "%s: version %s (from %s), time %s (from %s)\n", p.target, app.Version, from, t.Format(time.RFC3339), tFrom)
	}
	var outs []pkgfmt.Output
	for _, f := range p.formats {
		fw, err := os.MkdirTemp(work, f.Name()+"-")
		if err != nil {
			return outs, err
		}
		job := pkgfmt.Job{
			App: app, Target: p.target, Program: exe, Time: t, OutDir: out, Work: fw,
			GoBin: goBin, GoEnv: goEnv(), Tool: toolVersion(), Sign: signer, ProgramSigned: programSigned, Log: log, Verbose: o.Verbose,
		}
		written, err := f.Write(job)
		outs = append(outs, written...)
		if err != nil {
			return outs, fmt.Errorf("%s: %v", f.Name(), err)
		}
	}
	return outs, nil
}
