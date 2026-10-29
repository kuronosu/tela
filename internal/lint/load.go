package lint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Package loading: `go list` finds the packages and their files (it applies
// build constraints and resolves modules), and the standard library parses
// and type-checks them. The packages to lint are checked with their
// function bodies; everything they import is checked from source without
// bodies, once per run and shared by all of them.

// listPackage is the part of `go list -json` that the loader uses.
type listPackage struct {
	ImportPath   string
	Dir          string
	Name         string
	Standard     bool
	DepOnly      bool
	GoFiles      []string
	CgoFiles     []string
	TestGoFiles  []string
	XTestGoFiles []string
	Imports      []string
	TestImports  []string
	XTestImports []string
	ImportMap    map[string]string
	Module       *struct {
		Path      string
		GoVersion string
	}
	Error *struct{ Err string }
}

// goList runs `go list -e -deps -json` for the patterns in dir (with cgo
// off, like every Tela build) and returns the packages in the order go
// prints them (dependencies first).
func goList(dir string, patterns []string) ([]*listPackage, error) {
	args := []string{"list", "-e", "-deps", "-json=ImportPath,Dir,Name,Standard,DepOnly,GoFiles,CgoFiles,TestGoFiles,XTestGoFiles,Imports,TestImports,XTestImports,ImportMap,Module,Error", "--"}
	cmd := exec.Command("go", append(args, patterns...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) || stdout.Len() == 0 {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = err.Error()
			}
			return nil, fmt.Errorf("go list %s: %s", strings.Join(patterns, " "), msg)
		}
		return nil, err
	}
	var out []*listPackage
	dec := json.NewDecoder(&stdout)
	for dec.More() {
		var p listPackage
		if err := dec.Decode(&p); err != nil {
			return nil, fmt.Errorf("go list: %v", err)
		}
		out = append(out, &p)
	}
	return out, nil
}

// checked is a package after type-checking.
type checked struct {
	pkg   *types.Package
	files []*ast.File // target: GoFiles
	info  *types.Info // target only
	errs  []error     // target only: type errors
}

// loader parses and type-checks packages on demand, each one once, so
// packages that import the same dependency share its types.
type loader struct {
	fset  *token.FileSet
	sizes types.Sizes
	pkgs  map[string]*listPackage
	// targets are the packages to lint; they are checked with bodies.
	targets map[string]bool
	// cyclic are the packages in an import cycle, which cannot be checked.
	cyclic map[string]bool

	mu    sync.Mutex
	state map[string]*state
	// parsed caches the syntax of target files (with comments), by path.
	parsed map[string]*parsedFile
}

type state struct {
	once sync.Once
	c    *checked
}

type parsedFile struct {
	once sync.Once
	f    *ast.File
	err  error
}

func newLoader(pkgs []*listPackage) *loader {
	l := &loader{
		fset:    token.NewFileSet(),
		sizes:   types.SizesFor("gc", build.Default.GOARCH),
		pkgs:    map[string]*listPackage{},
		targets: map[string]bool{},
		cyclic:  map[string]bool{},
		state:   map[string]*state{},
		parsed:  map[string]*parsedFile{},
	}
	if l.sizes == nil {
		l.sizes = types.SizesFor("gc", "amd64")
	}
	l.add(pkgs)
	l.findCycles()
	return l
}

// findCycles marks the packages that are part of an import cycle: go list
// reports them, but checking one would wait for itself.
func (l *loader) findCycles() {
	const (
		visiting = 1
		done     = 2
	)
	mark := map[string]int{}
	var stack []string
	var visit func(path string)
	visit = func(path string) {
		switch mark[path] {
		case done:
			return
		case visiting:
			for i := len(stack) - 1; i >= 0; i-- {
				l.cyclic[stack[i]] = true
				if stack[i] == path {
					break
				}
			}
			return
		}
		lp := l.pkgs[path]
		if lp == nil {
			return
		}
		mark[path] = visiting
		stack = append(stack, path)
		for _, imp := range lp.Imports {
			if m, ok := lp.ImportMap[imp]; ok {
				imp = m
			}
			visit(imp)
		}
		stack = stack[:len(stack)-1]
		mark[path] = done
	}
	for path := range l.pkgs {
		visit(path)
	}
}

func (l *loader) add(pkgs []*listPackage) {
	for _, p := range pkgs {
		if _, ok := l.pkgs[p.ImportPath]; !ok {
			l.pkgs[p.ImportPath] = p
		}
		if !p.DepOnly {
			l.targets[p.ImportPath] = true
		}
	}
}

// parse parses a file once. Targets keep their comments (for the ignore
// directives); dependencies do not need them.
func (l *loader) parse(path string, comments bool) (*ast.File, error) {
	key := path
	if !comments {
		key += "\x00nc"
	}
	l.mu.Lock()
	pf := l.parsed[key]
	if pf == nil {
		pf = &parsedFile{}
		l.parsed[key] = pf
	}
	l.mu.Unlock()
	pf.once.Do(func() {
		mode := parser.SkipObjectResolution
		if comments {
			mode |= parser.ParseComments
		}
		pf.f, pf.err = parser.ParseFile(l.fset, path, nil, mode)
	})
	return pf.f, pf.err
}

// load returns the type-checked package with the import path, checking it
// (and what it imports) the first time.
func (l *loader) load(path string) *checked {
	l.mu.Lock()
	st := l.state[path]
	if st == nil {
		st = &state{}
		l.state[path] = st
	}
	l.mu.Unlock()
	st.once.Do(func() { st.c = l.check(path) })
	return st.c
}

// importer resolves the imports of one package through its ImportMap
// (go list's rewriting of vendored standard library paths).
type importer struct {
	l  *loader
	lp *listPackage
}

func (im importer) Import(path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	if m, ok := im.lp.ImportMap[path]; ok {
		path = m
	}
	lp := im.l.pkgs[path]
	switch {
	case lp == nil:
		return nil, fmt.Errorf("package %q is not in the go list output", path)
	case im.l.cyclic[path]:
		return nil, fmt.Errorf("import cycle not allowed: %q", path)
	case lp.Error != nil && len(lp.GoFiles) == 0:
		return nil, fmt.Errorf("cannot load package %q: %s", path, strings.TrimSpace(lp.Error.Err))
	}
	c := im.l.load(path)
	if c.pkg == nil {
		return nil, fmt.Errorf("package %q could not be loaded", path)
	}
	return c.pkg, nil
}

func goVersion(lp *listPackage) string {
	if lp.Module != nil && lp.Module.GoVersion != "" {
		return "go" + lp.Module.GoVersion
	}
	return ""
}

// files parses the Go files of lp named in names.
func (l *loader) files(lp *listPackage, names []string, comments bool) ([]*ast.File, []error) {
	var out []*ast.File
	var errs []error
	for _, name := range names {
		f, err := l.parse(filepath.Join(lp.Dir, name), comments)
		if err != nil {
			errs = append(errs, err)
		}
		if f != nil {
			out = append(out, f)
		}
	}
	return out, errs
}

func newInfo() *types.Info {
	return &types.Info{
		Types:      map[ast.Expr]types.TypeAndValue{},
		Defs:       map[*ast.Ident]types.Object{},
		Uses:       map[*ast.Ident]types.Object{},
		Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
}

func (l *loader) check(path string) *checked {
	lp := l.pkgs[path]
	target := l.targets[path]
	c := &checked{}
	if lp == nil {
		return c
	}
	if l.cyclic[path] {
		c.errs = append(c.errs, fmt.Errorf("import cycle not allowed"))
		return c
	}
	names := lp.GoFiles
	files, perrs := l.files(lp, names, target)
	c.files = files
	conf := types.Config{
		Importer:  importer{l, lp},
		GoVersion: goVersion(lp),
		Sizes:     l.sizes,
	}
	if target {
		c.info = newInfo()
		c.errs = append(c.errs, perrs...)
		conf.Error = func(err error) { c.errs = append(c.errs, err) }
	} else {
		// A dependency only provides declarations. Errors in it (an
		// assembly function, a cgo stub) cannot say anything about the
		// code being linted.
		conf.IgnoreFuncBodies = true
		conf.Error = func(error) {}
	}
	c.pkg, _ = conf.Check(path, l.fset, files, c.info)
	return c
}

// checkTests type-checks the package with its test files, for linting the
// test files. A package's own tests may import packages that import it, so
// the types of the two copies can disagree: errors are ignored and what was
// resolved is used.
func (l *loader) checkTests(lp *listPackage, files []*ast.File, path string) (*types.Package, *types.Info) {
	conf := types.Config{
		Importer:  importer{l, lp},
		GoVersion: goVersion(lp),
		Sizes:     l.sizes,
		Error:     func(error) {},
	}
	info := newInfo()
	pkg, _ := conf.Check(path, l.fset, files, info)
	return pkg, info
}

// relPath returns path relative to the working directory when it is
// inside it, else path.
func relPath(cwd, path string) string {
	if cwd == "" {
		return path
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
