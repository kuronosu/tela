package lint

import (
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

	// fileAST and ignores serve the //tela:ignore comments, by file name.
	fileAST map[string]*ast.File
	ignores map[string]ignoreSet

	// usesHooksCache remembers which linted functions call hooks.
	usesHooksCache map[*types.Func]bool

	diags []Diagnostic
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
	pkgs, err := goList(r.dir, patterns)
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
	r.l = newLoader(pkgs)

	res := &Result{}
	var targets []*listPackage
	for _, p := range pkgs {
		if p.DepOnly || p.Standard {
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
			continue
		}
		r.units = append(r.units, &unit{pkg: c.pkg, info: c.info, files: c.files})
		if r.opts.Tests {
			r.addTestUnits(p, c)
		}
	}
	res.Packages = len(r.units)
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
	more, err := goList(r.dir, missing)
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
	return filepath.ToSlash(relPath(r.cwd, name))
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
