package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
)

// The app's ID and title for the bundle of tela build and tela run
// (ADR-227), read from the source without running it: the package's
// tela.App literal, found the way a person finds it.
//
//  1. The argument of tela.Main (or tela.Run) in func main: a literal,
//     a package-level or local variable, or either one inside calls that
//     wrap the app (tela.Main(desktop.SingleInstance(desktop.PersistApp(App)))
//     reads the literal of App). A call is looked through by its arguments:
//     the first one that leads to a tela.App literal is the app.
//  2. Otherwise the package-level var App, resolved the same way.
//
// The fields AppID and Title are read as constant strings: string
// literals, constants (also of the package's own packages, or computed from
// constants), and concatenations. go/types evaluates them; the packages of
// the main module that the package imports are type-checked from source for
// it, and everything else is replaced by an empty package, since only the
// constants of the app's own code matter and nothing is compiled. A value
// that is not constant (a function call, a variable) is reported with the
// way out, never guessed.

// appSource is what the package's tela.App literal says.
type appSource struct {
	// Found says an App literal was found.
	Found bool
	// ID and Title are the constant values of the fields; empty when the
	// field is missing. IDErr and TitleErr say the field is there but is
	// not a constant string.
	ID, Title       string
	IDErr, TitleErr error
}

// telaImportPath is the import path of the tela package.
const telaImportPath = "github.com/kuronosu/tela"

// readAppSource finds the App of the package target, with deps the other
// packages go list printed (by import path).
func readAppSource(target listedPackage, deps map[string]listedPackage) (appSource, error) {
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range target.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(target.Dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return appSource{}, fmt.Errorf("reading %s: %v", name, err)
		}
		files = append(files, f)
	}
	imp := &sourceImporter{fset: fset, pkgs: deps, done: map[string]*types.Package{}}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: imp, Error: func(error) {}}
	_, _ = conf.Check(target.ImportPath, fset, files, info) // the errors are expected: see sourceImporter

	s := &appScan{info: info, vars: map[string]ast.Expr{}, tela: map[string]bool{}}
	var mainFn *ast.FuncDecl
	for _, f := range files {
		for _, im := range f.Imports {
			path, _ := strconv.Unquote(im.Path.Value)
			if path != telaImportPath {
				continue
			}
			switch {
			case im.Name == nil:
				s.tela["tela"] = true
			case im.Name.Name == ".":
				s.dot = true
			case im.Name.Name != "_":
				s.tela[im.Name.Name] = true
			}
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						s.addVars(s.vars, vs)
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.Name == "main" && d.Body != nil {
					mainFn = d
				}
			}
		}
	}

	var lit *ast.CompositeLit
	if mainFn != nil {
		s.locals = map[string]ast.Expr{}
		ast.Inspect(mainFn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if len(n.Lhs) == len(n.Rhs) {
					for i, l := range n.Lhs {
						if id, ok := l.(*ast.Ident); ok {
							if _, dup := s.locals[id.Name]; !dup {
								s.locals[id.Name] = n.Rhs[i]
							}
						}
					}
				}
			case *ast.ValueSpec:
				s.addVars(s.locals, n)
			}
			return true
		})
		ast.Inspect(mainFn.Body, func(n ast.Node) bool {
			if lit != nil {
				return false
			}
			if call, ok := n.(*ast.CallExpr); ok && s.isTelaFunc(call.Fun, "Main", "Run") && len(call.Args) > 0 {
				lit = s.literal(call.Args[0], 0)
			}
			return lit == nil
		})
	}
	if lit == nil {
		if e, ok := s.vars["App"]; ok {
			lit = s.literal(e, 0)
		}
	}
	if lit == nil {
		return appSource{}, nil
	}
	out := appSource{Found: true}
	out.ID, out.IDErr = s.stringField(lit, "AppID")
	out.Title, out.TitleErr = s.stringField(lit, "Title")
	return out, nil
}

// appScan is the state of one search.
type appScan struct {
	info *types.Info
	// vars are the package-level variables with an initializer, locals
	// those declared in main; the expression is the one assigned.
	vars, locals map[string]ast.Expr
	// tela are the names tela is imported under, dot whether it is
	// dot-imported.
	tela map[string]bool
	dot  bool
}

// addVars records the initializers of a var spec in m.
func (s *appScan) addVars(m map[string]ast.Expr, vs *ast.ValueSpec) {
	if len(vs.Names) != len(vs.Values) {
		return
	}
	for i, n := range vs.Names {
		if _, dup := m[n.Name]; !dup {
			m[n.Name] = vs.Values[i]
		}
	}
}

// isTelaFunc reports whether e names one of the functions of the tela
// package (tela.Main, or Main with a dot import).
func (s *appScan) isTelaFunc(e ast.Expr, names ...string) bool {
	has := func(n string) bool {
		for _, x := range names {
			if x == n {
				return true
			}
		}
		return false
	}
	switch e := e.(type) {
	case *ast.SelectorExpr:
		id, ok := e.X.(*ast.Ident)
		return ok && s.tela[id.Name] && has(e.Sel.Name)
	case *ast.Ident:
		return s.dot && has(e.Name)
	}
	return false
}

// isAppType reports whether e is the type tela.App (or App, dot-imported).
func (s *appScan) isAppType(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.SelectorExpr:
		id, ok := e.X.(*ast.Ident)
		return ok && s.tela[id.Name] && e.Sel.Name == "App"
	case *ast.Ident:
		return s.dot && e.Name == "App"
	}
	return false
}

// maxResolve bounds the variables and calls looked through.
const maxResolve = 16

// literal returns the tela.App literal that e is or wraps, or nil.
func (s *appScan) literal(e ast.Expr, depth int) *ast.CompositeLit {
	if depth > maxResolve {
		return nil
	}
	switch e := e.(type) {
	case *ast.ParenExpr:
		return s.literal(e.X, depth+1)
	case *ast.UnaryExpr:
		return s.literal(e.X, depth+1) // &tela.App{...}
	case *ast.StarExpr:
		return s.literal(e.X, depth+1) // *app
	case *ast.CompositeLit:
		if s.isAppType(e.Type) {
			return e
		}
	case *ast.CallExpr:
		for _, a := range e.Args {
			if l := s.literal(a, depth+1); l != nil {
				return l
			}
		}
	case *ast.Ident:
		if v, ok := s.locals[e.Name]; ok {
			return s.literal(v, depth+1)
		}
		if v, ok := s.vars[e.Name]; ok {
			return s.literal(v, depth+1)
		}
	}
	return nil
}

// stringField returns the constant string value of the field name of lit:
// "" and nil when the field is missing, and an error when it is there but
// not constant.
func (s *appScan) stringField(lit *ast.CompositeLit, name string) (string, error) {
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != name {
			continue
		}
		if tv, ok := s.info.Types[kv.Value]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
			return constant.StringVal(tv.Value), nil
		}
		if bl, ok := kv.Value.(*ast.BasicLit); ok && bl.Kind == token.STRING {
			if v, err := strconv.Unquote(bl.Value); err == nil {
				return v, nil
			}
		}
		return "", fmt.Errorf("the %s of the tela.App is not a constant string that tela can read without running the program (a literal, a constant or a concatenation of them)", name)
	}
	return "", nil
}

// sourceImporter resolves the imports of the package whose constants are
// read. A package of the main module is type-checked from its source (no
// function bodies), so that a constant defined in it resolves; every other
// package is an empty, complete package, so that the uses of it fail to
// resolve quietly. Its errors are ignored by the caller, since the
// declarations the constants depend on are the only ones that matter.
type sourceImporter struct {
	fset *token.FileSet
	pkgs map[string]listedPackage
	done map[string]*types.Package
}

func (im *sourceImporter) Import(path string) (*types.Package, error) {
	if path == "unsafe" {
		return types.Unsafe, nil
	}
	if p, ok := im.done[path]; ok {
		if p == nil {
			return nil, errors.New("import cycle")
		}
		return p, nil
	}
	lp, ok := im.pkgs[path]
	name := filepath.Base(path)
	if ok && lp.Name != "" {
		name = lp.Name
	}
	if !ok || lp.Standard || lp.Module == nil || !lp.Module.Main || len(lp.GoFiles) == 0 {
		p := types.NewPackage(path, name)
		p.MarkComplete()
		im.done[path] = p
		return p, nil
	}
	im.done[path] = nil // in progress
	var files []*ast.File
	for _, f := range lp.GoFiles {
		af, err := parser.ParseFile(im.fset, filepath.Join(lp.Dir, f), nil, parser.SkipObjectResolution)
		if err == nil {
			files = append(files, af)
		}
	}
	conf := types.Config{Importer: im, IgnoreFuncBodies: true, Error: func(error) {}}
	p, _ := conf.Check(path, im.fset, files, nil)
	im.done[path] = p
	return p, nil
}
