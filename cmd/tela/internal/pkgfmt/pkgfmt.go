// Package pkgfmt holds the package formats of tela package (ADR-295):
// each Format turns a built program and the app's declaration (ADR-294)
// into the files of one kind of package, deterministically.
//
// tela package does the build (the one of tela build) and calls the
// formats; a format only writes files. The formats of Linux are here
// (appimage, deb and flatpak) and those of macOS (app, dmg, macformats.go).
// Those of Windows (zip, msix) are registered as not yet written, and
// replace those entries.
//
// Determinism: every file a format writes depends only on the Job. Times
// are Job.Time, entries are sorted, owners are root (0:0) and modes are
// fixed (0755 for programs and folders, 0644 for the rest).
package pkgfmt

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/cmd/tela/internal/appicon"
)

// App is the app a package is made of.
type App struct {
	// ID is the app's reverse-DNS ID (tela.App.AppID).
	ID string
	// Name is the name people see (tela.App.Title, or -name).
	Name string
	// Package is the name of the package and of the installed program:
	// lowercase letters, digits, '+', '-' and '.' (see PackageName).
	Package string
	// Version is the version written into the program, without a
	// leading "v": semantic versioning (1.2.3, 1.2.3-rc.1).
	Version string
	// The rest of the declaration.
	Publisher, Description, Category string
	Schemes                          []string
	FileTypes                        []tela.FileType
	Services                         tela.Services
	// Icon is the icon, nil when the app has none; IconSVG its source
	// when it is an SVG file (the hicolor theme's scalable icon).
	Icon    *appicon.Source
	IconSVG []byte
}

// Target is a platform to package for.
type Target struct{ OS, Arch string }

func (t Target) String() string { return t.OS + "/" + t.Arch }

// Job is the work of one format for one target.
type Job struct {
	App    App
	Target Target
	// Program is the built executable.
	Program string
	// Time is the time of every file in the packages.
	Time time.Time
	// OutDir is where the packages go; Work is a folder of the format's
	// own, removed afterwards.
	OutDir, Work string
	// GoBin and GoEnv run the go command, for what a format builds (the
	// AppImage runtime).
	GoBin string
	GoEnv []string
	// Tool is tela's version, recorded where a format has a place for it.
	Tool string
	// Sign signs the files a format produces; see Signer.
	Sign Signer
	// Log receives notes for the person packaging (what was skipped and
	// why); Verbose says whether to say more.
	Log     io.Writer
	Verbose bool
}

// Output is a file (or folder) a format wrote.
type Output struct {
	Path string
	// Signed says a signer signed it.
	Signed bool
	// Note is said next to it ("not built: flatpak-builder is not
	// installed").
	Note string
}

// Format writes one kind of package.
type Format interface {
	// Name is the -format name ("appimage").
	Name() string
	// OS is the GOOS it packages for.
	OS() string
	// Check reports what the format cannot take of app for t, before
	// anything is built.
	Check(app App, t Target) error
	// Write writes the package of job into job.OutDir.
	Write(job Job) ([]Output, error)
}

// What a Signer is asked to sign.
const (
	// SignProgram is an executable before it goes into a package (a
	// Windows .exe).
	SignProgram = "program"
	// SignBundle is a macOS .app folder.
	SignBundle = "bundle"
	// SignPackage is a finished package file.
	SignPackage = "package"
)

// Signer signs a file in place. what is SignProgram, SignBundle or
// SignPackage, and t the target. It returns false when nothing signed the
// file (no signer for that kind): the package is still written, marked
// unsigned.
type Signer interface {
	Sign(path, what string, t Target) (bool, error)
}

// NoSigner signs nothing.
type NoSigner struct{}

func (NoSigner) Sign(string, string, Target) (bool, error) { return false, nil }

var registry = map[string]Format{}

// Register adds f, replacing a format of the same name.
func Register(f Format) { registry[f.Name()] = f }

// Lookup returns the format of that name.
func Lookup(name string) (Format, bool) {
	f, ok := registry[name]
	return f, ok
}

// Names returns the formats' names, sorted.
func Names() []string {
	var out []string
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Defaults returns the formats tela package writes for goos without
// -format.
func Defaults(goos string) []string {
	switch goos {
	case "linux":
		return []string{"appimage", "deb"}
	case "windows":
		return []string{"zip", "msix"}
	case "darwin":
		return []string{"app"}
	}
	return nil
}

// FileName is the name of a package file: <package>-<version>-<os>-<arch>
// and ext (".AppImage"), as tela update manifest reads it.
func FileName(app App, t Target, ext string) string {
	return fmt.Sprintf("%s-%s-%s-%s%s", app.Package, app.Version, t.OS, t.Arch, ext)
}

var packageChars = regexp.MustCompile(`[^a-z0-9+.-]+`)

// PackageName derives the package's name: the last part of the app ID
// ("notes" for com.example.Notes), lowercase, other characters as '-'; the
// title when that gives fewer than two characters. Debian's rules: it
// starts with a letter or digit and has two characters or more.
func PackageName(id, title string) string {
	for _, s := range []string{id[strings.LastIndexByte(id, '.')+1:], title} {
		n := packageChars.ReplaceAllString(strings.ToLower(s), "-")
		n = strings.TrimLeft(n, "+.-")
		n = strings.TrimRight(n, "-.")
		if len(n) >= 2 {
			return n
		}
	}
	return "app"
}

// Categories are the freedesktop.org main categories tela.App.Category
// takes.
var Categories = []string{"AudioVideo", "Audio", "Video", "Development", "Education", "Game", "Graphics", "Network", "Office", "Science", "Settings", "System", "Utility"}

// CheckCategory reports whether c is empty or a main category.
func CheckCategory(c string) error {
	if c == "" {
		return nil
	}
	for _, m := range Categories {
		if c == m {
			return nil
		}
		if strings.EqualFold(c, m) {
			return fmt.Errorf("App.Category %q: write it as %q", c, m)
		}
	}
	return fmt.Errorf("App.Category %q is not a freedesktop.org main category; use one of %s", c, strings.Join(Categories, ", "))
}

// category returns the app's category, "Utility" when none.
func (a App) category() string {
	if a.Category == "" {
		return "Utility"
	}
	return a.Category
}

// summary is the one-line description: Description, else the name.
func (a App) summary() string {
	if s := strings.TrimSpace(a.Description); s != "" {
		return s
	}
	return a.Name
}

// errNotYet is the error of the formats the next step of the phase
// writes.
var errNotYet = errors.New("not implemented yet")

// pending is a format that is not written yet.
type pending struct{ name, goos string }

func (p pending) Name() string { return p.name }
func (p pending) OS() string   { return p.goos }
func (p pending) Check(App, Target) error {
	return fmt.Errorf("the %s format (%s) is %w; package for Linux (-os linux) for now", p.name, p.goos, errNotYet)
}
func (p pending) Write(Job) ([]Output, error) { return nil, p.Check(App{}, Target{}) }

func init() {
	Register(appImageFormat{})
	Register(debFormat{})
	Register(flatpakFormat{})
	Register(appFormat{})
	Register(dmgFormat{})
	for _, p := range []pending{{"zip", "windows"}, {"msix", "windows"}} {
		Register(p)
	}
}

// writeOut writes data to dir/name through a temporary file renamed into
// place, so an interrupted run leaves no half package.
func writeOut(dir, name string, data []byte, perm os.FileMode) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, ".tela-package-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), perm); err != nil {
		return "", err
	}
	return p, os.Rename(tmp.Name(), p)
}

// sign signs a finished package file.
func sign(job Job, p string) (Output, error) {
	out := Output{Path: p}
	if job.Sign == nil {
		return out, nil
	}
	ok, err := job.Sign.Sign(p, SignPackage, job.Target)
	if err != nil {
		return out, fmt.Errorf("signing %s: %v", filepath.Base(p), err)
	}
	out.Signed = ok
	return out, nil
}
