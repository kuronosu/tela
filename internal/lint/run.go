package lint

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/kuronosu/tela/internal/style"
)

// unit is what the passes run on: the type-checked files of one package (or
// of its test files).
type unit struct {
	pkg   *types.Package
	info  *types.Info
	files []*ast.File
	// ctxOnly marks a package of Options.Context: its aliases and colors
	// are collected, nothing in it is checked.
	ctxOnly bool
	// migrateOnly marks a package that did not type-check: it is checked
	// for the migrate rule alone, which reads what resolved.
	migrateOnly bool
}

// run is the state of one Run, shared by the passes of all packages: the
// class parser (it knows the aliases and theme colors of every linted
// package), the declarations of the linted functions and the set of custom
// hooks.
type run struct {
	opts Options
	dir  string
	cwd  string

	l     *loader
	units []*unit

	// overlay is Options.Overlay with cleaned paths; overlayJSON is the
	// file of the go command's -overlay flag for it, or "".
	overlay     map[string][]byte
	overlayJSON string

	parser    *style.Parser
	aliasList []*aliasEntry

	// decls maps a linted function or method to its declaration.
	decls map[*types.Func]*funcDecl
	// hooks is the set of functions that call hooks and are named as
	// hooks: Tela's own, and the custom hooks of the linted code.
	hooks map[*types.Func]bool

	// varInit maps a variable to the expression that initializes it, when
	// it is declared with one (var x = e, x := e).
	varInit map[types.Object]ast.Expr
	// varUnit is the unit that declares the variable of varInit.
	varUnit map[types.Object]*unit

	// consts maps a constant to its declaration, when it is declared with
	// a value of its own (not by the repetition of the one above).
	consts map[*types.Const]*constDecl

	// migrations are the rewrites of the migrate rule.
	migrations []Migration

	// ctxOnly are the import paths of the packages read as context.
	ctxOnly map[string]bool

	// fileAST and ignores serve the //tela:ignore comments, by file name.
	fileAST map[string]*ast.File
	ignores map[string]ignoreSet

	// usesHooksCache remembers which linted functions call hooks.
	usesHooksCache map[*types.Func]bool

	diags []Diagnostic
	// classStrings are the literals the class rule saw.
	classStrings []ClassString
	seenClassLit map[ClassString]bool
	// fixes are the edits of the fixable findings.
	fixes map[fixKey][]edit
}

// constDecl is the value of a declared constant and the unit that has its
// types.
type constDecl struct {
	expr ast.Expr
	unit *unit
}

type funcDecl struct {
	decl *ast.FuncDecl
	unit *unit
}

func (r *run) run(patterns []string) (*Result, error) {
	cwd, err := os.Getwd()
	if err == nil {
		r.cwd = cwd
	}
	overlayJSON, cleanup, err := r.writeOverlay()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	r.overlayJSON = overlayJSON
	pkgs, err := goList(r.dir, r.overlayJSON, patterns)
	if err != nil {
		return nil, err
	}
	if r.opts.Tests {
		more, err := r.testDeps(pkgs)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, more...)
	}
	r.migrations = r.opts.Migrations
	if r.migrations == nil {
		r.migrations = Migrations
	}
	r.ctxOnly = map[string]bool{}
	if len(r.opts.Context) > 0 {
		pkgs = r.addContext(pkgs)
	}
	r.l = newLoader(pkgs)
	r.l.overlay = r.overlay
	r.l.useExports(r.dir)

	res := &Result{}
	var targets []*listPackage
	for _, p := range pkgs {
		if p.DepOnly || p.Standard {
			continue
		}
		if len(p.GoFiles) == 0 && r.ctxOnly[p.ImportPath] {
			continue
		}
		if len(p.GoFiles) == 0 {
			// Only test files (or none at all): there is nothing to check
			// unless the tests are, and a package with an error and no
			// files at all is a pattern that matched nothing.
			if p.Error != nil && len(p.TestGoFiles) == 0 && len(p.XTestGoFiles) == 0 {
				res.LoadErrors = append(res.LoadErrors, p.ImportPath+": "+strings.TrimSpace(p.Error.Err))
				continue
			}
			if !r.opts.Tests {
				continue
			}
		}
		targets = append(targets, p)
	}

	// Type-check the targets in parallel; they share their dependencies.
	checks := make([]*checked, len(targets))
	var wg sync.WaitGroup
	for i, p := range targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			checks[i] = r.l.load(p.ImportPath)
		}()
	}
	wg.Wait()

	for i, p := range targets {
		c := checks[i]
		if r.ctxOnly[p.ImportPath] {
			// Read for what it declares: a package that does not
			// type-check is read as far as it resolved, and its
			// problems are not the run's.
			if c.pkg != nil && c.info != nil {
				r.units = append(r.units, &unit{pkg: c.pkg, info: c.info, files: c.files, ctxOnly: true})
			}
			continue
		}
		if len(c.errs) > 0 || c.pkg == nil {
			msg := p.ImportPath + ": "
			if len(c.errs) > 0 {
				msg += c.errs[0].Error()
				if len(c.errs) > 1 {
					msg += " (and " + strconv.Itoa(len(c.errs)-1) + " more errors)"
				}
			} else {
				msg += "could not be type-checked"
			}
			res.LoadErrors = append(res.LoadErrors, strings.ReplaceAll(msg, r.cwd+string(filepath.Separator), ""))
			if c.pkg == nil || c.info == nil {
				continue
			}
			if !r.opts.Lenient {
				// Old code does not type-check against the new API: what
				// resolved of it is checked for the migrations, which are
				// what fixes it.
				r.units = append(r.units, &unit{pkg: c.pkg, info: c.info, files: c.files, migrateOnly: true})
				continue
			}
		}
		r.units = append(r.units, &unit{pkg: c.pkg, info: c.info, files: c.files})
		if r.opts.Tests {
			r.addTestUnits(p, c)
		}
	}
	res.Packages = 0
	for _, u := range r.units {
		if !u.ctxOnly {
			res.Packages++
		}
	}
	r.fileAST = map[string]*ast.File{}
	for _, u := range r.units {
		for _, f := range u.files {
			r.fileAST[r.l.fset.Position(f.Package).Filename] = f
		}
	}

	r.collect()
	res.LoadErrors = append(res.LoadErrors, r.checkAll()...)

	sortDiagnostics(r.diags)
	res.Diagnostics = dedupe(r.diags)
	res.Parser = r.parser
	res.ClassStrings = r.classStrings
	if r.opts.Fix {
		var left []Diagnostic
		fixed, files, ferr := r.applyFixes(res.Diagnostics)
		isFixed := map[fixKey]bool{}
		for _, d := range fixed {
			isFixed[fixKeyOf(d)] = true
		}
		for _, d := range res.Diagnostics {
			if !isFixed[fixKeyOf(d)] {
				left = append(left, d)
			}
		}
		res.Diagnostics, res.Fixed, res.FixedFiles, res.FixError = left, fixed, files, ferr
	}
	sort.Strings(res.LoadErrors)
	return res, nil
}

// testDeps lists the packages that the test files of the targets import
// and the list does not have.
func (r *run) testDeps(pkgs []*listPackage) ([]*listPackage, error) {
	have := map[string]bool{}
	for _, p := range pkgs {
		have[p.ImportPath] = true
	}
	var missing []string
	seen := map[string]bool{}
	for _, p := range pkgs {
		if p.DepOnly || p.Standard {
			continue
		}
		for _, list := range [][]string{p.TestImports, p.XTestImports} {
			for _, imp := range list {
				if imp == "C" || imp == "unsafe" || have[imp] || seen[imp] {
					continue
				}
				seen[imp] = true
				missing = append(missing, imp)
			}
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	sort.Strings(missing)
	more, err := goList(r.dir, r.overlayJSON, missing)
	if err != nil {
		return nil, err
	}
	for _, p := range more {
		p.DepOnly = true // dependencies of the tests, not targets
	}
	return more, nil
}

// addTestUnits type-checks the test files of target p and adds them as
// units: the in-package ones together with the package's files, the
// external ones (package p_test) as a package of their own.
func (r *run) addTestUnits(p *listPackage, c *checked) {
	if len(p.TestGoFiles) > 0 {
		tests, _ := r.l.files(p, p.TestGoFiles, true)
		all := append(append([]*ast.File(nil), c.files...), tests...)
		pkg, info := r.l.checkTests(p, all, p.ImportPath)
		if pkg != nil {
			r.units = append(r.units, &unit{pkg: pkg, info: info, files: tests})
		}
	}
	if len(p.XTestGoFiles) > 0 {
		xtests, _ := r.l.files(p, p.XTestGoFiles, true)
		pkg, info := r.l.checkTests(p, xtests, p.ImportPath+"_test")
		if pkg != nil {
			r.units = append(r.units, &unit{pkg: pkg, info: info, files: xtests})
		}
	}
}

// rel returns the path of a file for output.
func (r *run) rel(name string) string {
	if r.opts.AbsPaths {
		return name
	}
	return filepath.ToSlash(relPath(r.cwd, name))
}

// writeOverlay cleans Options.Overlay into r.overlay and writes the file
// that the go command's -overlay flag takes: the buffers in a temporary
// directory, and a JSON file that maps each path to its buffer. It returns
// that file's name ("" without an overlay) and the function that removes
// the directory.
func (r *run) writeOverlay() (string, func(), error) {
	if len(r.opts.Overlay) == 0 {
		return "", func() {}, nil
	}
	dir, err := os.MkdirTemp("", "tela-overlay-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(dir) }
	r.overlay = make(map[string][]byte, len(r.opts.Overlay))
	replace := make(map[string]string, len(r.opts.Overlay))
	i := 0
	for name, src := range r.opts.Overlay {
		name = filepath.Clean(name)
		r.overlay[name] = src
		file := filepath.Join(dir, strconv.Itoa(i)+".go")
		i++
		if err := os.WriteFile(file, src, 0o600); err != nil {
			cleanup()
			return "", nil, err
		}
		replace[name] = file
	}
	data, err := json.Marshal(map[string]any{"Replace": replace})
	if err != nil {
		cleanup()
		return "", nil, err
	}
	file := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		cleanup()
		return "", nil, err
	}
	return file, cleanup, nil
}

// dedupe removes findings that repeat another one in position, rule and
// message; ds is sorted.
func dedupe(ds []Diagnostic) []Diagnostic {
	out := ds[:0]
	for i, d := range ds {
		if i > 0 && d == ds[i-1] {
			continue
		}
		out = append(out, d)
	}
	return out
}

// addContext lists Options.Context and adds to pkgs, as targets that are
// only read, the packages of it that mention what declares aliases and
// theme colors, and what they import from the same module (the maps are
// often declared in a package of their own). The other packages of the
// listing are added as dependencies. A context that cannot be listed adds
// nothing: it is a help, not a requirement.
func (r *run) addContext(pkgs []*listPackage) []*listPackage {
	ctx, err := goList(r.dir, r.overlayJSON, r.opts.Context)
	if err != nil {
		return pkgs
	}
	have := map[string]*listPackage{}
	for _, p := range pkgs {
		have[p.ImportPath] = p
	}
	byPath := map[string]*listPackage{}
	for _, p := range ctx {
		byPath[p.ImportPath] = p
	}
	var visit func(p *listPackage)
	visit = func(p *listPackage) {
		if r.ctxOnly[p.ImportPath] || p.Standard || p.Module == nil {
			return
		}
		if t := have[p.ImportPath]; t != nil && !t.DepOnly {
			return // linted: not context
		}
		r.ctxOnly[p.ImportPath] = true
		for _, imp := range p.Imports {
			if q := byPath[imp]; q != nil && q.Module != nil && q.Module.Path == p.Module.Path {
				visit(q)
			}
		}
	}
	for _, p := range ctx {
		if !p.DepOnly && !p.Standard && p.Module != nil && mentionsAliases(p) {
			visit(p)
		}
	}
	for _, p := range ctx {
		switch have[p.ImportPath] {
		case nil:
			q := *p
			q.DepOnly = !r.ctxOnly[p.ImportPath]
			pkgs = append(pkgs, &q)
		default:
			if r.ctxOnly[p.ImportPath] {
				have[p.ImportPath].DepOnly = false
			}
		}
	}
	return pkgs
}

// mentionsAliases reports whether a file of p names ClassAliases or
// SchemeColor, so that it may declare class aliases or theme colors.
func mentionsAliases(p *listPackage) bool {
	// Tela's own packages mention both everywhere (the App type, the
	// themes of the looks); only the apps and fixtures inside its module
	// declare aliases of their own.
	if p.Module.Path == modulePath && !strings.Contains(p.ImportPath, "/examples/") && !strings.Contains(p.ImportPath, "/testdata/") {
		return false
	}
	for _, name := range p.GoFiles {
		data, err := os.ReadFile(filepath.Join(p.Dir, name))
		if err == nil && (bytes.Contains(data, []byte("ClassAliases")) || bytes.Contains(data, []byte("SchemeColor"))) {
			return true
		}
	}
	return false
}
