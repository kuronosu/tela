package update

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kuronosu/tela/internal/platform/assoc"
)

// MarkerName is the file that makes a folder a Windows zip install that
// updates itself: tela package writes it next to the executable, and
// every zip artifact must hold one, at its top level or in its one
// top-level folder. Its content is a [Marker] in JSON.
const MarkerName = "tela-install.json"

// Marker is the content of the [MarkerName] file:
//
//	{
//	  "schema": 1,
//	  "format": "zip",
//	  "appID": "com.example.notes",
//	  "version": "1.2.0",
//	  "exe": "Notes.exe",
//	  "files": ["Notes.exe", "LICENSE.txt", "tela-install.json"]
//	}
//
// Apply replaces only the entries of the folder that the old and the new
// marker list in Files (top-level names; a folder counts as one entry),
// so files the user put in the folder stay.
type Marker struct {
	// Schema is 1.
	Schema int `json:"schema"`
	// Format is "zip".
	Format string `json:"format"`
	// AppID and Version are the app's.
	AppID   string `json:"appID"`
	Version string `json:"version"`
	// Exe is the program's file name in the folder, which Restart starts.
	Exe string `json:"exe"`
	// Files are the top-level entries the package put in the folder, the
	// marker included. Empty means every top-level entry of the zip.
	Files []string `json:"files,omitempty"`
}

// install is the running program's install.
type install struct {
	format string
	// path is what Apply replaces: the AppImage file, the .app bundle,
	// the zip's folder.
	path string
	// exe is the program Restart starts, inside path for the bundle and
	// the folder.
	exe string
	// marker is the zip folder's.
	marker *Marker
}

// updateDir is the folder next to the install that holds its swap state
// and the old copy: <dir>/.<name>.update for the AppImage file and the
// bundle, <folder>/.update for the zip folder (whose parent may not be
// the user's to write).
func (in *install) updateDir() string {
	if in.format == "zip" {
		return filepath.Join(in.path, ".update")
	}
	return filepath.Join(filepath.Dir(in.path), "."+filepath.Base(in.path)+".update")
}

// sysEnv is what detection reads of the system, replaced in tests.
type sysEnv struct {
	goos       string
	getenv     func(string) string
	executable func() (string, error)
	// root prefixes the system paths read (/.flatpak-info, dpkg's
	// database), "" outside tests.
	root string
	// msix reports whether the process runs from an MSIX package
	// (assoc.InPackage); always false off Windows.
	msix func() bool
}

func osEnv() sysEnv {
	return sysEnv{goos: runtime.GOOS, getenv: os.Getenv, executable: os.Executable, msix: assoc.InPackage}
}

// detectEnv is the environment detection reads; tests replace it.
var detectEnv = osEnv

// findInstall returns the running program's install as format, or as the
// detected format when format is "". Managed installs are a
// *ManagedError, an unknown one ErrNotInstalled.
//
// Detection, in order: an MSIX package (GetCurrentPackageFullName
// succeeds); flatpak ($FLATPAK_ID, or /.flatpak-info); an AppImage
// ($APPIMAGE, with the executable inside $APPDIR when that is set, so a
// program started from an AppImage app is not taken for it); a macOS
// bundle (the executable is <Name>.app/Contents/MacOS/<exe>); a zip
// folder (a MarkerName file next to the executable); a deb (an executable
// under /usr or /opt listed by a dpkg *.list file).
func findInstall(format string) (*install, error) { return detect(format, true) }

// findSelf returns the running program's install when it updates itself:
// what the start of Run, MarkHealthy, RolledBack and Restart read, without
// dpkg's database.
func findSelf() (*install, error) { return detect("", false) }

func detect(format string, scanDpkg bool) (*install, error) {
	e := detectEnv()
	exe, err := e.executable()
	if err != nil {
		return nil, fmt.Errorf("update: cannot find the running program: %w", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	managed := func(f, manager string) (*install, error) {
		return nil, &ManagedError{Format: f, Manager: manager}
	}
	switch format {
	case "msix":
		return managed("msix", "msix")
	case "flatpak":
		return managed("flatpak", "flatpak")
	case "deb":
		return managed("deb", "apt")
	case "appimage":
		if p := e.getenv("APPIMAGE"); p != "" {
			return &install{format: "appimage", path: p, exe: p}, nil
		}
		return nil, fmt.Errorf("%w: Config.Format is appimage but $APPIMAGE is not set", ErrNotInstalled)
	case "app":
		if in := bundleOf(exe); in != nil {
			return in, nil
		}
		return nil, fmt.Errorf("%w: Config.Format is app but %s is not inside a .app bundle", ErrNotInstalled, exe)
	case "zip":
		if in := zipOf(exe); in != nil {
			return in, nil
		}
		// Without a marker: the folder of the executable, every entry
		// of the new zip replacing its namesake.
		dir := filepath.Dir(exe)
		return &install{format: "zip", path: dir, exe: exe, marker: &Marker{Schema: 1, Format: "zip", Exe: filepath.Base(exe)}}, nil
	case "":
	default:
		return nil, fmt.Errorf("update: Config.Format %q is not one of appimage, zip, app, deb, flatpak, msix", format)
	}

	if e.goos == "windows" && e.msix != nil && e.msix() {
		return managed("msix", "msix")
	}
	if e.goos == "linux" {
		if e.getenv("FLATPAK_ID") != "" || exists(e.root+"/.flatpak-info") {
			return managed("flatpak", "flatpak")
		}
		if p := e.getenv("APPIMAGE"); p != "" {
			dir := e.getenv("APPDIR")
			if dir == "" || within(exe, dir) {
				return &install{format: "appimage", path: p, exe: p}, nil
			}
		}
	}
	if e.goos == "darwin" {
		if in := bundleOf(exe); in != nil {
			return in, nil
		}
	}
	if in := zipOf(exe); in != nil {
		return in, nil
	}
	if scanDpkg && e.goos == "linux" && (strings.HasPrefix(exe, "/usr/") || strings.HasPrefix(exe, "/opt/")) && dpkgOwns(e.root, exe) {
		return managed("deb", "apt")
	}
	return nil, fmt.Errorf("%w (%s): updates need an AppImage, a Windows zip or a macOS bundle made by tela package", ErrNotInstalled, exe)
}

// bundleOf returns the bundle of an executable at
// <Name>.app/Contents/MacOS/<exe>, or nil.
func bundleOf(exe string) *install {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	app := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(app, ".app") {
		return nil
	}
	return &install{format: "app", path: app, exe: exe}
}

// zipOf returns the zip install of an executable whose folder holds a
// marker of format zip, or nil.
func zipOf(exe string) *install {
	dir := filepath.Dir(exe)
	m, err := readMarker(filepath.Join(dir, MarkerName))
	if err != nil || m.Format != "zip" {
		return nil
	}
	return &install{format: "zip", path: dir, exe: exe, marker: m}
}

func readMarker(path string) (*Marker, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Marker
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("update: %s: %w", path, err)
	}
	if m.Schema != 1 || m.Exe == "" || strings.ContainsAny(m.Exe, `/\`) {
		return nil, fmt.Errorf("update: %s: want schema 1 and the exe's file name", path)
	}
	return &m, nil
}

// dpkgOwns reports whether a dpkg file list (/var/lib/dpkg/info/*.list)
// names path.
func dpkgOwns(root, path string) bool {
	lists, _ := filepath.Glob(root + "/var/lib/dpkg/info/*.list")
	for _, l := range lists {
		f, err := os.Open(l)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		found := false
		for sc.Scan() {
			if sc.Text() == path {
				found = true
				break
			}
		}
		f.Close()
		if found {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// within reports whether path is dir or inside it.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
