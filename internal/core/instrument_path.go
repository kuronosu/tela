package core

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// How a source site is shown to a reader of the app (the `[src=...]` of
// aria snapshots and the "src" of the layout JSON, ADR-299).
//
// A site is a path and a line, with the path as the app's author would
// open it:
//
//  1. a file inside the working directory is relative to it: `tela
//     snapshot`, `tela inspect` and `tela mcp` run the app in its package's
//     directory, so a site of the package is "settings.go:42" and one of a
//     subdirectory "ui/card.go:7";
//  2. any other file of a module is the path of the module followed by the
//     file's path in it ("example.com/app/internal/ui/card.go:7",
//     "github.com/kuronosu/tela/kit/button.go:88"), which names the package
//     whoever reads it from wherever;
//  3. a file outside any module (the standard library, a build without
//     module files, a path already shortened by -trimpath) is its base
//     name.
//
// A node that a library made (the elements a kit.Button builds) is shown
// at the nearest element above it that the app's own module made, as
// the line that wrote `kit.Button{...}` is the one an author edits
// (Tree.AppSite).

// Rel returns the site as the one-line path of the rules above:
// "settings.go:42", "" for no site.
func (s SourceSite) Rel() string {
	if s.File == "" {
		return ""
	}
	return displayPath(s.File) + ":" + strconv.Itoa(s.Line)
}

// pathState holds what displayPath learns from the file system, once per
// process: the working directory and the module of each directory.
var pathState struct {
	once sync.Once
	cwd  string // slash-separated, no trailing slash
	mu   sync.Mutex
	mods map[string]modInfo // by directory
	real map[string]string  // a directory with its symbolic links resolved
}

// modInfo is the module a directory belongs to.
type modInfo struct {
	root string // the directory of go.mod, slash-separated; "" for none
	path string // its module path
}

func pathInit() {
	pathState.once.Do(func() {
		if wd, err := os.Getwd(); err == nil {
			pathState.cwd = strings.TrimRight(filepath.ToSlash(wd), "/")
		}
		pathState.mods = map[string]modInfo{}
		pathState.real = map[string]string{}
	})
}

// RelTo is Rel with dir as the working directory of rule 1: the form the
// app's own process shows (its [src=] lines), for a tool that runs
// elsewhere and knows the app's working directory.
func (s SourceSite) RelTo(dir string) string {
	if s.File == "" {
		return ""
	}
	return displayPathIn(strings.TrimRight(filepath.ToSlash(dir), "/"), s.File) + ":" + strconv.Itoa(s.Line)
}

func displayPath(file string) string {
	pathInit()
	return displayPathIn(pathState.cwd, file)
}

// displayPathIn is displayPath with cwd, slash-separated, as the working
// directory.
func displayPathIn(cwd, file string) string {
	file = filepath.ToSlash(file)
	if !path.IsAbs(file) && !isWinAbs(file) {
		return file // already relative (-trimpath): module@version/file.go
	}
	file = onVolume(cwd, file)
	if rel, ok := underReal(cwd, file); ok {
		return rel
	}
	if m := moduleOfDir(path.Dir(file)); m.root != "" {
		rel, _ := under(m.root, file)
		return m.path + "/" + rel
	}
	return path.Base(file)
}

// isWinAbs reports a Windows drive path such as "C:/x/y.go" (the
// runtime writes them with forward slashes).
func isWinAbs(p string) bool {
	return len(p) > 2 && p[1] == ':' && p[2] == '/' && (p[0]|0x20) >= 'a' && (p[0]|0x20) <= 'z'
}

// onVolume returns file, a path rooted without a volume ("/src/a.go"), on
// the volume of cwd when cwd has one ("Z:/src/a.go"), as Windows reads a
// rooted path on the current drive. A binary built on Linux or macOS and
// run under Wine records its sources that way, while Wine's working
// directory is a drive path (Z: is its Unix root). A UNC path ("//host/")
// and every path on other systems are left alone.
func onVolume(cwd, file string) string {
	if isWinAbs(cwd) && strings.HasPrefix(file, "/") && !strings.HasPrefix(file, "//") {
		return cwd[:2] + file
	}
	return file
}

// under returns file relative to dir when it is inside it.
func under(dir, file string) (string, bool) {
	if dir == "" {
		return "", false
	}
	prefix := dir + "/"
	if !strings.HasPrefix(file, prefix) {
		// Windows paths compare without case.
		if !isWinAbs(dir) || len(file) < len(prefix) || !strings.EqualFold(file[:len(prefix)], prefix) {
			return "", false
		}
	}
	return file[len(prefix):], true
}

// underReal is under, also through symbolic links: on macOS the
// temporary directory /var/folders/... is /private/var/folders/..., and
// the working directory and the paths the compiler recorded may each be
// spelled either way.
func underReal(dir, file string) (string, bool) {
	if rel, ok := under(dir, file); ok || dir == "" {
		return rel, ok
	}
	rd, rf := realDir(dir), realDir(path.Dir(file))
	if rd == dir && rf == path.Dir(file) {
		return "", false
	}
	return under(rd, rf+"/"+path.Base(file))
}

// realDir returns dir, slash-separated, with its symbolic links resolved,
// cached per directory; dir itself when it cannot be resolved.
func realDir(dir string) string {
	pathInit()
	pathState.mu.Lock()
	r, ok := pathState.real[dir]
	pathState.mu.Unlock()
	if ok {
		return r
	}
	r = dir
	if e, err := filepath.EvalSymlinks(filepath.FromSlash(dir)); err == nil {
		r = strings.TrimRight(filepath.ToSlash(e), "/")
	}
	pathState.mu.Lock()
	pathState.real[dir] = r
	pathState.mu.Unlock()
	return r
}

// moduleOfDir finds the module of directory dir by looking for go.mod
// upwards.
func moduleOfDir(dir string) modInfo {
	pathInit()
	pathState.mu.Lock()
	defer pathState.mu.Unlock()
	var walked []string
	var m modInfo
	for d := dir; ; {
		if c, ok := pathState.mods[d]; ok {
			m = c
			break
		}
		walked = append(walked, d)
		if p, ok := readModulePath(filepath.FromSlash(d)); ok {
			m = modInfo{root: d, path: p}
			break
		}
		parent := path.Dir(d)
		if parent == d || parent == "." || len(walked) > 24 {
			break
		}
		d = parent
	}
	for _, d := range walked {
		pathState.mods[d] = m
	}
	return m
}

// readModulePath reads the module path of the go.mod in dir.
func readModulePath(dir string) (string, bool) {
	f, err := os.Open(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "module"); ok && (rest == "" || rest[0] == ' ' || rest[0] == '\t') {
			p := strings.Trim(strings.TrimSpace(rest), "\"`")
			return p, p != ""
		}
	}
	return "", false
}

// inAppModule reports whether file is in the module of the working
// directory (the app's, when the app runs in its package's directory).
// With no module known for the working directory, everything is the
// app's.
func inAppModule(file string) bool {
	file = filepath.ToSlash(file)
	if !path.IsAbs(file) && !isWinAbs(file) {
		return true
	}
	pathInit()
	app := moduleOfDir(pathState.cwd)
	if app.root == "" {
		return true
	}
	_, ok := underReal(app.root, onVolume(pathState.cwd, file))
	return ok
}

// AppSite returns where the app's own code made element id: its creation
// site when that is in the module of the working directory, else the
// site of the nearest ancestor that has one there (the line that wrote
// the library component whose build made it), else its own. The zero
// SourceSite when the instrumentation is off, the element is not mounted
// or no site is known (outside dev mode).
func (t *Tree) AppSite(id ElementID) SourceSite {
	ins := t.ins
	if ins == nil {
		return SourceSite{}
	}
	return ins.appSite(ins.element(id))
}

func (ins *instrument) appSite(el *Element) SourceSite {
	var first SourceSite
	for ; el != nil; el = el.parent {
		s := ins.siteOf(el)
		if s.IsZero() {
			continue
		}
		if inAppModule(s.File) {
			return s
		}
		if first.IsZero() {
			first = s
		}
	}
	return first
}

// siteOf returns the creation site of el, resolved. A component whose node
// was made before sites were captured (at package level, as the
// tela.C(form) of a Previews entry or of App.Root, which runs when the
// package is initialized) has the declaration of its function instead,
// as a struct component a build returned has (instrument_site.go, rule
// 3): the node is shared by every mount, so it is not written into.
func (ins *instrument) siteOf(el *Element) SourceSite {
	if d := el.node.d; d != nil && d.site != nil {
		return ins.creationSite(d.site)
	}
	if ins.dev && el.kind == elemComponent && el.comp != nil && el.comp.typ != 0 {
		return ins.creationSite(declSite(el.comp.typ))
	}
	return SourceSite{}
}
