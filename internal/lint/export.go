package lint

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	goimporter "go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
)

// Dependencies from export data. Type-checking the standard library and
// Tela from source took most of a run, and every run did it again. The go
// command compiles the packages that are not being linted and keeps the
// result in its build cache (`go list -export` prints the file of each);
// the standard library's go/importer reads it, a fraction of the work, and
// the go command is what decides when a package has to be compiled again,
// so a change of sources, build tags, GOOS or GOARCH is never served stale.
//
// What is read from export data is the independent packages: those that
// are not linted and do not import (directly or not) one that is. The types
// of a package refer to the objects of what it imports, so a package that
// imports a linted one has to be checked from source together with it, to
// agree on the linted package's types; the independent ones cannot see it.
// A package that cannot be compiled (it has errors, which the source path
// ignores in a dependency) is checked from source, and so are the packages
// that import it.
//
// The importer of the independent packages is shared by the runs of the
// process that need exactly the same export files (a generation), so the
// tests, and tela mcp, import them once. Anything that goes wrong in the
// export path (no build cache, export data this Go version cannot read)
// leaves the run on the source path, which gives the same results.

// exportInfo is the part of `go list -export -json` that the loader uses.
type exportInfo struct {
	ImportPath string
	Export     string
	Error      *struct{ Err string }
}

// resolvedImports returns the import paths of lp as the loader names its
// packages (ImportMap applied, unsafe left out).
func resolvedImports(lp *listPackage) []string {
	out := make([]string, 0, len(lp.Imports))
	for _, imp := range lp.Imports {
		if imp == "unsafe" {
			continue
		}
		if m, ok := lp.ImportMap[imp]; ok {
			imp = m
		}
		out = append(out, imp)
	}
	return out
}

// independent returns the paths of the packages that are not targets, are
// not in an import cycle, have no load error and import only such packages.
func (l *loader) independent() []string {
	memo := map[string]bool{}
	var is func(path string) bool
	is = func(path string) bool {
		if v, ok := memo[path]; ok {
			return v
		}
		memo[path] = false // a cycle would not come back here, it is in l.cyclic
		lp := l.pkgs[path]
		if lp == nil || l.targets[path] || l.cyclic[path] || lp.Error != nil {
			return false
		}
		for _, imp := range resolvedImports(lp) {
			if !is(imp) {
				return false
			}
		}
		memo[path] = true
		return true
	}
	var out []string
	for path := range l.pkgs {
		if is(path) {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// useExports makes the loader import its independent dependencies from
// export data, when the go command and the importer can provide it.
func (l *loader) useExports(dir string) {
	paths := l.independent()
	if len(paths) == 0 {
		return
	}
	files := exportFiles(dir, paths, l.pkgs)

	// A package is read from export data when it has an export file and
	// everything it imports does.
	exported := map[string]bool{}
	memo := map[string]bool{}
	var has func(path string) bool
	has = func(path string) bool {
		if v, ok := memo[path]; ok {
			return v
		}
		memo[path] = false
		lp := l.pkgs[path]
		if lp == nil || files[path] == "" {
			return false
		}
		for _, imp := range resolvedImports(lp) {
			if !has(imp) {
				return false
			}
		}
		memo[path] = true
		return true
	}
	var ok []string
	for _, path := range paths {
		if has(path) {
			exported[path] = true
			ok = append(ok, path)
		}
	}
	if len(ok) == 0 {
		return
	}
	l.gen = generations.get(ok, files)
	if l.gen == nil {
		return
	}
	l.exported = exported
}

// The export files of the process. Listing them was about half of a run
// that reads the module as context (the go command hashes the sources of
// every package it lists), so the file the go command named for a package
// is remembered with what it was named for: the package's listing, the
// identity of each of its files (fileid.go) and, the same way, of each
// package it imports, with the environment and the go command. A package
// whose files are not trusted yet (modified in the last stampSlack) is
// listed every time, as is one whose export file is gone (go clean -cache).
var exportMemo = struct {
	mu sync.Mutex
	m  map[string]exportMemoEntry // by import path
}{m: map[string]exportMemoEntry{}}

// exportsListed counts the packages whose export file the go command was
// asked for, for tests.
var exportsListed atomic.Int64

type exportMemoEntry struct {
	key  string
	file string
}

// exportFiles returns the export file of each of paths that has one, from
// the memo or from the go command.
func exportFiles(dir string, paths []string, pkgs map[string]*listPackage) map[string]string {
	keys := exportKeys(paths, pkgs)
	files := make(map[string]string, len(paths))
	var missing []string
	exportMemo.mu.Lock()
	for _, p := range paths {
		e, ok := exportMemo.m[p]
		if k := keys[p]; ok && k != "" && e.key == k {
			files[p] = e.file
			continue
		}
		missing = append(missing, p)
	}
	exportMemo.mu.Unlock()
	for p, f := range files {
		if _, err := os.Stat(f); err != nil {
			delete(files, p)
			missing = append(missing, p)
		}
	}
	if len(missing) == 0 {
		return files
	}
	sort.Strings(missing)
	exportsListed.Add(int64(len(missing)))
	listed := listExports(dir, missing)
	exportMemo.mu.Lock()
	defer exportMemo.mu.Unlock()
	for p, f := range listed {
		files[p] = f
		if k := keys[p]; k != "" {
			exportMemo.m[p] = exportMemoEntry{k, f}
		}
	}
	return files
}

// exportKeys returns the key of each of paths, which are independent (they
// import only each other): "" for a package that cannot be remembered.
func exportKeys(paths []string, pkgs map[string]*listPackage) map[string]string {
	env := exportEnvKey()
	keys := make(map[string]string, len(paths))
	done := make(map[string]bool, len(paths))
	var key func(path string) string
	key = func(path string) string {
		if done[path] {
			return keys[path]
		}
		done[path] = true
		lp := pkgs[path]
		if lp == nil || env == "" {
			return ""
		}
		h := sha256.New()
		fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00", env, lp.ImportPath, lp.Dir, lp.Name, goVersion(lp))
		imps := make([]string, 0, len(lp.ImportMap))
		for k, v := range lp.ImportMap {
			imps = append(imps, k+"="+v)
		}
		sort.Strings(imps)
		fmt.Fprintf(h, "%q\x00", imps)
		for _, name := range lp.GoFiles {
			id, trusted, ok := statFile(filepath.Join(lp.Dir, name))
			if !ok || !trusted {
				return ""
			}
			fmt.Fprintf(h, "%s %d %d %d\x00", name, id.size, id.mod, id.change)
		}
		for _, imp := range resolvedImports(lp) {
			k := key(imp)
			if k == "" {
				return ""
			}
			fmt.Fprintf(h, "%s=%s\x00", imp, k)
		}
		k := hex.EncodeToString(h.Sum(nil))
		keys[path] = k
		return k
	}
	for _, p := range paths {
		key(p)
	}
	return keys
}

// exportEnvKey is what the export files depend on besides the sources: the
// environment (GOFLAGS, GOOS, GOARCH, GOEXPERIMENT, GOTOOLCHAIN, PATH...)
// and the go command it finds. It is "" when there is no go command.
func exportEnvKey() string {
	goCmd, err := exec.LookPath("go")
	if err != nil {
		return ""
	}
	id, _, ok := statFile(goCmd)
	if !ok {
		return ""
	}
	env := os.Environ()
	sort.Strings(env)
	h := sha256.New()
	fmt.Fprintf(h, "%s %d %d %d\x00%q", goCmd, id.size, id.mod, id.change, env)
	return hex.EncodeToString(h.Sum(nil))
}

// listExports runs `go list -export` for paths and returns the export file
// of each that has one.
func listExports(dir string, paths []string) map[string]string {
	args := append([]string{"list", "-e", "-export", "-json=ImportPath,Export,Error", "--"}, paths...)
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	// cmd.Environ, not os.Environ: it sets PWD to cmd.Dir. The parent's
	// PWD names another directory, so the go command would take the
	// resolved working directory (macOS's /private/var for /var) and
	// find the absolute directories it is given outside its module.
	cmd.Env = append(cmd.Environ(), "CGO_ENABLED=0")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	// A package that does not compile makes the command fail, but it still
	// lists the others: the exit status says nothing here.
	_ = cmd.Run()
	files := map[string]string{}
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var p exportInfo
		if err := dec.Decode(&p); err != nil {
			break
		}
		if p.Export != "" && p.Error == nil {
			files[p.ImportPath] = p.Export
		}
	}
	return files
}

// generation is an importer of export data and the packages it has read.
type generation struct {
	once sync.Once
	// ok is false when the export files could not be read.
	ok bool

	mu  sync.Mutex // the importer is not safe for concurrent use
	imp types.Importer
}

func (g *generation) Import(path string) (*types.Package, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.imp.Import(path)
}

// maxGenerations bounds the importers kept: a long-running process (tela
// mcp) gets a new one whenever a dependency is compiled again.
const maxGenerations = 8

var generations = &generationStore{m: map[string]*generation{}}

type generationStore struct {
	mu    sync.Mutex
	m     map[string]*generation
	order []string
}

// get returns the generation for the export files of paths: the key is the
// hash of the files' names, which the go command makes from their content,
// so equal keys are equal data. It is nil when the files cannot be read.
func (s *generationStore) get(paths []string, files map[string]string) *generation {
	h := sha256.New()
	for _, p := range paths {
		fmt.Fprintf(h, "%d:%s=%s\n", len(p), p, files[p])
	}
	key := hex.EncodeToString(h.Sum(nil))
	s.mu.Lock()
	g := s.m[key]
	if g == nil {
		g = &generation{}
		s.m[key] = g
		s.order = append(s.order, key)
		if len(s.order) > maxGenerations {
			delete(s.m, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.mu.Unlock()
	g.once.Do(func() {
		g.imp = goimporter.ForCompiler(token.NewFileSet(), "gc", func(path string) (io.ReadCloser, error) {
			f := files[path]
			if f == "" {
				return nil, fmt.Errorf("no export data for %q", path)
			}
			return os.Open(f)
		})
		// Read everything now: a file this Go version cannot read (it was
		// compiled by another one) must send the run to the source path,
		// not fail in the middle of it.
		g.ok = true
		for _, p := range paths {
			if _, err := g.Import(p); err != nil {
				g.ok = false
				break
			}
		}
	})
	if !g.ok {
		return nil
	}
	return g
}
