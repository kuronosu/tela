package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// pkgSpec is one page to generate.
type pkgSpec struct {
	Dir        string // directory, relative to the module root
	ImportPath string
	Out        string // output file, relative to the module root; empty for standard output
	Page       string // wiki page name, for links from other pages
}

// defaultSpecs are the public packages of the module.
func defaultSpecs(modPath string) []pkgSpec {
	return []pkgSpec{
		{Dir: ".", ImportPath: modPath, Out: "docs/wiki/API-Reference-tela.md", Page: "API-Reference-tela"},
		{Dir: "h", ImportPath: modPath + "/h", Out: "docs/wiki/API-Reference-h.md", Page: "API-Reference-h"},
		{Dir: "aria", ImportPath: modPath + "/aria", Out: "docs/wiki/API-Reference-aria.md", Page: "API-Reference-aria"},
		{Dir: "telatest", ImportPath: modPath + "/telatest", Out: "docs/wiki/API-Reference-telatest.md", Page: "API-Reference-telatest"},
		{Dir: "kit", ImportPath: modPath + "/kit", Out: "docs/wiki/API-Reference-kit.md", Page: "API-Reference-kit"},
	}
}

// pageName is the wiki page of a package's reference.
func pageName(importPath string) string { return "API-Reference-" + path.Base(importPath) }

// modulePath returns the module path declared by root/go.mod.
func modulePath(root string) (string, error) {
	src, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("%v (run from the module root, or pass -root)", err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", fmt.Errorf("%s/go.mod has no module line", root)
}

// pkgInfo is a loaded package: its documentation and the table of
// anchors the doc links resolve against.
type pkgInfo struct {
	spec  pkgSpec
	fset  *token.FileSet
	files []*ast.File // the non-test files (their imports resolve aliases)
	doc   *doc.Package
	// aliases maps the name of a type declared as an alias of a type of
	// another package of the module to that type.
	aliases map[string]*alias
	// syms maps "Name" and "Recv.Name" to the anchor (without '#') of the
	// section that documents the symbol.
	syms map[string]string
}

// alias is a type declared as `type A = pkg.B` where pkg is a package of
// the module.
type alias struct {
	target string // "core.MouseEvent", as written
	pkg    *pkgInfo
	typ    *doc.Type
}

// generator holds the packages of one run.
type generator struct {
	root, modPath string
	pkgs          map[string]*pkgInfo // public packages by import path
	order         []*pkgInfo
	internal      map[string]*pkgInfo // packages aliases point to, by import path
}

// generate renders the pages of specs, in order.
func generate(root, modPath string, specs []pkgSpec) ([][]byte, error) {
	g := &generator{root: root, modPath: modPath, pkgs: map[string]*pkgInfo{}, internal: map[string]*pkgInfo{}}
	for _, spec := range specs {
		p, err := load(filepath.Join(root, spec.Dir), spec, true)
		if err != nil {
			return nil, err
		}
		g.pkgs[spec.ImportPath] = p
		g.order = append(g.order, p)
	}
	for _, p := range g.order {
		if err := g.resolveAliases(p); err != nil {
			return nil, err
		}
		p.buildSyms()
	}
	pages := make([][]byte, len(g.order))
	for i, p := range g.order {
		out, err := g.render(p)
		if err != nil {
			return nil, fmt.Errorf("%s: %v", p.spec.ImportPath, err)
		}
		pages[i] = out
	}
	return pages, nil
}

// load parses the package in dir, with the linux/amd64 build context. With
// tests, the examples of its _test.go files are documented too.
func load(dir string, spec pkgSpec, tests bool) (*pkgInfo, error) {
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = "linux", "amd64", false
	bp, err := ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", dir, err)
	}
	names := append([]string(nil), bp.GoFiles...)
	nSrc := len(names)
	if tests {
		names = append(names, bp.TestGoFiles...)
		names = append(names, bp.XTestGoFiles...)
	}
	fset := token.NewFileSet()
	p := &pkgInfo{spec: spec, fset: fset, aliases: map[string]*alias{}, syms: map[string]string{}}
	var all []*ast.File
	for i, name := range names {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		all = append(all, f)
		if i < nSrc {
			p.files = append(p.files, f)
		}
	}
	p.doc, err = doc.NewFromFiles(fset, all, spec.ImportPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", dir, err)
	}
	return p, nil
}

// resolveAliases finds the types of p declared as aliases of types of
// other packages of the module, and loads those.
func (g *generator) resolveAliases(p *pkgInfo) error {
	for _, t := range p.doc.Types {
		spec := typeSpec(t)
		if spec == nil || !spec.Assign.IsValid() {
			continue
		}
		sel := aliasSelector(spec.Type)
		if sel == nil {
			continue
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		importPath := p.importPathOf(pkgIdent.Name)
		rel, ok := strings.CutPrefix(importPath, g.modPath+"/")
		if !ok {
			continue
		}
		ip := g.internal[importPath]
		if ip == nil {
			var err error
			ip, err = load(filepath.Join(g.root, filepath.FromSlash(rel)), pkgSpec{ImportPath: importPath}, false)
			if err != nil {
				return err
			}
			g.internal[importPath] = ip
		}
		for _, it := range ip.doc.Types {
			if it.Name == sel.Sel.Name {
				p.aliases[t.Name] = &alias{target: pkgIdent.Name + "." + it.Name, pkg: ip, typ: it}
				renameReceivers(it, t.Name)
			}
		}
	}
	return nil
}

// Short names for the go/doc types the code passes around.
type (
	docType  = doc.Type
	docFunc  = doc.Func
	docValue = doc.Value
)

// typeSpec returns the declaration of t.
func typeSpec(t *doc.Type) *ast.TypeSpec {
	if t.Decl == nil {
		return nil
	}
	for _, s := range t.Decl.Specs {
		if ts, ok := s.(*ast.TypeSpec); ok && ts.Name.Name == t.Name {
			return ts
		}
	}
	return nil
}

// importPathOf returns the import path the file-level name refers to in
// the package's files ("" when none matches).
func (p *pkgInfo) importPathOf(name string) string {
	for _, f := range p.files {
		for _, imp := range f.Imports {
			ipath := strings.Trim(imp.Path.Value, `"`)
			local := path.Base(ipath)
			if imp.Name != nil {
				local = imp.Name.Name
			}
			if local == name {
				return ipath
			}
		}
	}
	return ""
}

// aliasSelector returns the pkg.Name of an alias's right-hand side, with
// or without type arguments.
func aliasSelector(e ast.Expr) *ast.SelectorExpr {
	switch e := e.(type) {
	case *ast.SelectorExpr:
		return e
	case *ast.IndexExpr:
		return aliasSelector(e.X)
	case *ast.IndexListExpr:
		return aliasSelector(e.X)
	}
	return nil
}

// renameReceivers makes the methods of t read as declared on name, the
// alias a user writes.
func renameReceivers(t *docType, name string) {
	for _, m := range t.Methods {
		if m.Decl == nil || m.Decl.Recv == nil || len(m.Decl.Recv.List) == 0 {
			continue
		}
		renameBase(m.Decl.Recv.List[0].Type, name)
		star := ""
		if strings.HasPrefix(m.Recv, "*") {
			star = "*"
		}
		m.Recv = star + name
	}
}

// renameBase renames the type identifier of a receiver expression.
func renameBase(e ast.Expr, name string) {
	switch e := e.(type) {
	case *ast.Ident:
		e.Name = name
	case *ast.StarExpr:
		renameBase(e.X, name)
	case *ast.IndexExpr:
		renameBase(e.X, name)
	case *ast.IndexListExpr:
		renameBase(e.X, name)
	case *ast.ParenExpr:
		renameBase(e.X, name)
	}
}

// funcTitle returns the heading of function f: "func Name", or "func Name
// (deprecated)" for a deprecated function whose anchor would be another
// function's, since GitHub's anchors ignore case (UseId, deprecated for
// UseID).
func (p *pkgInfo) funcTitle(f *docFunc) string {
	title := "func " + f.Name
	if !isDeprecated(f.Doc) {
		return title
	}
	for _, g := range p.allFuncs() {
		if g != f && strings.EqualFold(g.Name, f.Name) {
			return title + " (deprecated)"
		}
	}
	return title
}

// allFuncs returns the package's functions, those go/doc groups under a
// type included.
func (p *pkgInfo) allFuncs() []*docFunc {
	fs := append([]*docFunc(nil), p.doc.Funcs...)
	for _, t := range p.doc.Types {
		fs = append(fs, t.Funcs...)
	}
	return fs
}

// isDeprecated reports whether a doc comment has a paragraph that starts
// with "Deprecated: " (the Go convention).
func isDeprecated(text string) bool {
	for _, para := range strings.Split(text, "\n\n") {
		if strings.HasPrefix(strings.TrimSpace(para), "Deprecated: ") {
			return true
		}
	}
	return false
}

// buildSyms fills the anchor table of p.
func (p *pkgInfo) buildSyms() {
	d := p.doc
	for _, v := range d.Consts {
		for _, n := range v.Names {
			p.syms[n] = "constants"
		}
	}
	for _, v := range d.Vars {
		for _, n := range v.Names {
			p.syms[n] = "variables"
		}
	}
	for _, f := range d.Funcs {
		p.syms[f.Name] = anchorOf(p.funcTitle(f))
	}
	for _, t := range d.Types {
		ta := anchorOf("type " + t.Name)
		p.syms[t.Name] = ta
		for _, v := range append(append([]*docValue(nil), t.Consts...), t.Vars...) {
			for _, n := range v.Names {
				p.syms[n] = ta
			}
		}
		for _, f := range t.Funcs {
			p.syms[f.Name] = anchorOf(p.funcTitle(f))
		}
		for _, f := range members(t.Decl, t.Name) {
			p.syms[t.Name+"."+f] = ta
		}
		methods := t.Methods
		if a := p.aliases[t.Name]; a != nil {
			methods = append(append([]*docFunc(nil), methods...), a.typ.Methods...)
			for _, f := range members(a.typ.Decl, a.typ.Name) {
				p.syms[t.Name+"."+f] = ta
			}
		}
		for _, m := range methods {
			p.syms[t.Name+"."+m.Name] = anchorOf("func (" + m.Recv + ") " + m.Name)
		}
	}
}

// members returns the exported field names of a struct type and the
// method names of an interface type declared in decl.
func members(decl *ast.GenDecl, name string) []string {
	if decl == nil {
		return nil
	}
	var out []string
	for _, s := range decl.Specs {
		ts, ok := s.(*ast.TypeSpec)
		if !ok || ts.Name.Name != name {
			continue
		}
		var fields *ast.FieldList
		switch t := ts.Type.(type) {
		case *ast.StructType:
			fields = t.Fields
		case *ast.InterfaceType:
			fields = t.Methods
		}
		if fields == nil {
			continue
		}
		for _, f := range fields.List {
			for _, n := range f.Names {
				if n.IsExported() {
					out = append(out, n.Name)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// lookupSym reports whether a [Name] or [Recv.Name] doc link resolves in
// the package.
func (p *pkgInfo) lookupSym(recv, name string) bool {
	_, ok := p.syms[symKey(recv, name)]
	return ok
}

func symKey(recv, name string) string {
	if recv == "" {
		return name
	}
	return recv + "." + name
}
