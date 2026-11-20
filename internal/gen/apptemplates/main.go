// Command apptemplates generates the app templates of "tela new -template"
// (ADR-298) from the examples: cmd/tela/templates/notes, settings and
// dashboard. It is run by "go generate" in cmd/tela (see new.go there), and
// a test fails when the output on disk is stale.
//
// For each example it copies the Go sources, the assets and the text
// goldens, and:
//
//   - gives every .go file a .tmpl suffix, so the go command does not read
//     the copies as packages of this module (embed.FS keeps the names);
//   - rewrites the example's own import path to the placeholder that
//     "tela new" replaces with the new module path;
//   - drops what a new app cannot use or should not carry: go.mod, go.sum,
//     the README, files that need a build tag of this repository (telagpu,
//     teladev), "go:build ignore" programs, and every pixel golden
//     (testdata/*.png: about 11 MB for the three, and bound to the rendering
//     of the Tela release that made them);
//   - removes from the tests, declaration by declaration, whatever uses a
//     package of this repository's internal/ tree or compares a pixel golden
//     (Golden, GoldenScales), and whatever depends on a declaration removed
//     that way; the text goldens (AriaGolden) stay;
//   - records the requirements of the example's go.mod besides tela in
//     requires.txt, so "tela new" writes them to the new go.mod.
//
// Flags:
//
//	-root dir   the module root (default: found above the working directory)
//	-v          list what was removed from the tests
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Placeholder is the module path that stands for the new app's module in
// the generated templates; cmd/tela's new.go has the same constant.
const Placeholder = "example.invalid/app"

const (
	telaModule  = "github.com/kuronosu/tela"
	outDir      = "cmd/tela/templates"
	tmplSuffix  = ".tmpl"
	requiresTxt = "requires.txt"
)

type spec struct {
	Name    string // the -template value and the directory under outDir
	Dir     string // the example's directory, from the module root
	Package string // the example's import path
}

var specs = []spec{
	{"notes", "examples/notes", telaModule + "/examples/notes"},
	{"settings", "examples/settings", telaModule + "/examples/settings"},
	{"dashboard", "examples/dashboard", telaModule + "/examples/dashboard"},
}

func main() {
	root := flag.String("root", "", "module root (default: found above the working directory)")
	verbose := flag.Bool("v", false, "list what was removed from the tests")
	flag.Parse()
	if *root == "" {
		r, err := findRoot()
		if err != nil {
			fatal(err)
		}
		*root = r
	}
	var log func(string)
	if *verbose {
		log = func(s string) { fmt.Fprintln(os.Stderr, s) }
	}
	files, err := build(*root, log)
	if err != nil {
		fatal(err)
	}
	if err := write(*root, files); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "apptemplates:", err)
	os.Exit(1)
}

func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil && bytes.HasPrefix(b, []byte("module "+telaModule+"\n")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod of %s not found above the working directory; pass -root", telaModule)
		}
		dir = parent
	}
}

// build returns the templates as a map from slash paths relative to root
// (cmd/tela/templates/notes/main.go.tmpl...) to contents.
func build(root string, log func(string)) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, s := range specs {
		if err := buildOne(root, s, out, log); err != nil {
			return nil, fmt.Errorf("%s: %w", s.Name, err)
		}
	}
	return out, nil
}

func buildOne(root string, s spec, out map[string][]byte, log func(string)) error {
	base := filepath.Join(root, filepath.FromSlash(s.Dir))
	// Collect the files per directory: the tests of a package are cut
	// together (a declaration removed in one file may be used in another).
	dirs := map[string][]string{}
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skipFile(rel) {
			return nil
		}
		dirs[path.Dir(rel)] = append(dirs[path.Dir(rel)], rel)
		return nil
	})
	if err != nil {
		return err
	}
	prefix := outDir + "/" + s.Name + "/"
	for dir, rels := range dirs {
		var goFiles []string
		for _, rel := range rels {
			if strings.HasSuffix(rel, ".go") {
				goFiles = append(goFiles, rel)
				continue
			}
			b, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			out[prefix+rel] = b
		}
		sources := map[string][]byte{}
		for _, rel := range goFiles {
			b, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(rel)))
			if err != nil {
				return err
			}
			sources[rel] = b
		}
		kept, err := cutTests(sources, func(msg string) {
			if log != nil {
				log(s.Name + ": " + msg)
			}
		})
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		for rel, b := range kept {
			if skipGoFile(rel, b) {
				continue
			}
			b = rewrite(b, s)
			out[prefix+rel+tmplSuffix] = b
		}
	}
	if b, err := os.ReadFile(filepath.Join(base, "go.mod")); err == nil {
		if req := extraRequires(b); req != "" {
			out[prefix+requiresTxt] = []byte(req)
		}
	}
	return nil
}

// skipFile reports whether a file of an example never goes into a template.
func skipFile(rel string) bool {
	base := path.Base(rel)
	switch {
	case rel == "go.mod", rel == "go.sum", rel == "README.md":
		return true
	case strings.Contains(base, "_actual."), strings.Contains(base, "_diff."):
		return true
	case strings.HasPrefix(rel, "testdata/") && strings.HasSuffix(rel, ".png"):
		return true // pixel goldens: see the package comment
	}
	switch path.Ext(base) {
	case ".go", ".svg", ".jpg", ".jpeg", ".png", ".yml":
		return false
	}
	return base != "LICENSE" // not a source or an asset: a built binary, for one
}

var buildLine = regexp.MustCompile(`(?m)^//go:build (.*)$`)

// skipGoFile drops the programs that "go run" runs alone and the files that
// only build with a tag of this repository.
func skipGoFile(rel string, src []byte) bool {
	head := src
	if i := bytes.Index(head, []byte("\npackage ")); i >= 0 {
		head = head[:i]
	}
	m := buildLine.FindSubmatch(head)
	if m == nil {
		return false
	}
	c := string(m[1])
	return c == "ignore" || strings.Contains(c, "telagpu") || strings.Contains(c, "teladev")
}

var comment = regexp.MustCompile(`(?m)^(\s*//.*)$`)

// rewrite points the example's imports at the placeholder module, and the
// paths its comments give for running it at the new app's own directory.
func rewrite(src []byte, s spec) []byte {
	src = bytes.ReplaceAll(src, []byte(s.Package), []byte(Placeholder))
	local := "./" + s.Dir
	return comment.ReplaceAllFunc(src, func(line []byte) []byte {
		line = bytes.ReplaceAll(line, []byte(local+"/"), []byte("./"))
		return bytes.ReplaceAll(line, []byte(local), []byte("."))
	})
}

var requireLine = regexp.MustCompile(`^\s*(\S+)\s+(v\S+)\s*$`)

// extraRequires lists the direct requirements of a go.mod besides tela, one
// "path version" per line.
func extraRequires(gomod []byte) string {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(string(gomod), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "require (":
			in = true
			continue
		case t == ")":
			in = false
			continue
		case strings.HasPrefix(t, "require "):
			t = strings.TrimPrefix(t, "require ")
		case !in:
			continue
		}
		if strings.Contains(t, "// indirect") {
			continue
		}
		if m := requireLine.FindStringSubmatch(t); m != nil && m[1] != telaModule {
			fmt.Fprintf(&b, "%s %s\n", m[1], m[2])
		}
	}
	return b.String()
}

// cutTests returns the sources of a directory with the tests cut down to
// what a new app can run (see the package comment). Non-test files are
// returned unchanged; a test file left with no declarations is dropped.
func cutTests(sources map[string][]byte, log func(string)) (map[string][]byte, error) {
	type unit struct {
		file   string
		decl   ast.Decl
		start  token.Pos
		end    token.Pos
		names  []string // top-level names it declares
		recv   string   // receiver type of a method
		idents map[string]bool
		banned bool // uses an internal package or compares a pixel golden
		gone   bool
	}
	// isEntry is a declaration the test runner or the go command calls.
	isEntry := func(u *unit) bool {
		fd, ok := u.decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil {
			return false
		}
		n := fd.Name.Name
		return n == "init" || n == "TestMain" || strings.HasPrefix(n, "Test") ||
			strings.HasPrefix(n, "Benchmark") || strings.HasPrefix(n, "Fuzz") || strings.HasPrefix(n, "Example")
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	var units []*unit
	for _, rel := range sortedKeys(sources) {
		if !strings.HasSuffix(rel, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, rel, sources[rel], parser.ParseComments)
		if err != nil {
			return nil, err
		}
		files[rel] = f
		bannedNames := map[string]bool{}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, telaModule+"/internal/") {
				bannedNames[importName(imp)] = true
			}
		}
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				continue
			}
			u := &unit{file: rel, decl: d, start: d.Pos(), end: d.End(), idents: map[string]bool{}}
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Doc != nil {
					u.start = d.Doc.Pos()
				}
				if d.Recv != nil {
					u.recv = recvName(d.Recv)
				} else {
					u.names = append(u.names, d.Name.Name)
				}
			case *ast.GenDecl:
				if d.Doc != nil {
					u.start = d.Doc.Pos()
				}
				for _, sp := range d.Specs {
					switch sp := sp.(type) {
					case *ast.ValueSpec:
						for _, n := range sp.Names {
							u.names = append(u.names, n.Name)
						}
					case *ast.TypeSpec:
						u.names = append(u.names, sp.Name.Name)
					}
				}
			}
			isTest := false
			if fd, ok := d.(*ast.FuncDecl); ok && fd.Recv == nil {
				for _, p := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
					isTest = isTest || strings.HasPrefix(fd.Name.Name, p)
				}
			}
			ast.Inspect(d, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.SelectorExpr:
					if id, ok := n.X.(*ast.Ident); ok && bannedNames[id.Name] {
						u.banned = true
					}
					if isTest && (n.Sel.Name == "Golden" || n.Sel.Name == "GoldenScales") {
						u.banned = true
					}
					// The selector's own name is a field or method, not a
					// top-level declaration.
					ast.Inspect(n.X, func(m ast.Node) bool {
						if id, ok := m.(*ast.Ident); ok {
							u.idents[id.Name] = true
						}
						return true
					})
					return false
				case *ast.Ident:
					u.idents[n.Name] = true
				}
				return true
			})
			units = append(units, u)
		}
	}
	for _, u := range units {
		u.gone = u.banned
	}
	// The identifiers the non-test files use: they keep what they name.
	outside := map[string]bool{}
	for rel, src := range sources {
		if strings.HasSuffix(rel, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), rel, src, 0)
		if err != nil {
			return nil, err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				outside[id.Name] = true
			}
			return true
		})
	}
	// A declaration that uses a removed one goes too, and so does a helper
	// that only removed declarations used, until nothing changes.
	for changed := true; changed; {
		changed = false
		removed := map[string]bool{}
		for _, u := range units {
			if u.gone {
				for _, n := range u.names {
					removed[n] = true
				}
			}
		}
		for _, u := range units {
			if u.gone {
				continue
			}
			hit := removed[u.recv]
			for id := range u.idents {
				hit = hit || removed[id]
			}
			if hit {
				u.gone, changed = true, true
			}
		}
		for _, u := range units {
			if u.gone || len(u.names) == 0 || isEntry(u) {
				continue
			}
			byRemoved, byKept := false, false
			for _, name := range u.names {
				byKept = byKept || outside[name]
			}
			for _, v := range units {
				if v == u || v.recv != "" && slices.Contains(u.names, v.recv) {
					continue
				}
				for _, name := range u.names {
					if v.idents[name] {
						byRemoved = byRemoved || v.gone
						byKept = byKept || !v.gone
					}
				}
			}
			if byRemoved && !byKept {
				u.gone, changed = true, true
			}
		}
	}

	out := map[string][]byte{}
	for rel, src := range sources {
		if !strings.HasSuffix(rel, "_test.go") {
			out[rel] = src
		}
	}
	for _, rel := range sortedKeys(files) {
		f := files[rel]
		type cut struct{ from, to int }
		var cuts []cut
		left := 0
		for _, u := range units {
			if u.file != rel {
				continue
			}
			if !u.gone {
				left++
				continue
			}
			cuts = append(cuts, cut{fset.Position(u.start).Offset, fset.Position(u.end).Offset})
			label := strings.Join(u.names, ", ")
			if label == "" {
				label = "method of " + u.recv
			}
			log(fmt.Sprintf("%s: removed %s", rel, label))
		}
		if left == 0 {
			log(rel + ": dropped (nothing left)")
			continue
		}
		src := sources[rel]
		sort.Slice(cuts, func(i, j int) bool { return cuts[i].from > cuts[j].from })
		for _, c := range cuts {
			to := c.to
			for to < len(src) && src[to] == '\n' {
				to++
			}
			src = append(append([]byte{}, src[:c.from]...), src[to:]...)
		}
		_ = f
		src, err := pruneImports(rel, src)
		if err != nil {
			return nil, err
		}
		out[rel] = src
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// recvName is the type name of a method's receiver.
func recvName(r *ast.FieldList) string {
	if len(r.List) == 0 {
		return ""
	}
	t := r.List[0].Type
	for {
		switch x := t.(type) {
		case *ast.StarExpr:
			t = x.X
		case *ast.IndexExpr:
			t = x.X
		case *ast.IndexListExpr:
			t = x.X
		case *ast.Ident:
			return x.Name
		default:
			return ""
		}
	}
}

// importName is the name an import binds in its file.
func importName(imp *ast.ImportSpec) string {
	if imp.Name != nil {
		return imp.Name.Name
	}
	p, _ := strconv.Unquote(imp.Path.Value)
	return path.Base(p)
}

// pruneImports removes the imports no selector uses and formats the source.
func pruneImports(name string, src []byte) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%s after the cuts: %w", name, err)
	}
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				used[id.Name] = true
			}
		}
		return true
	})
	type cut struct{ from, to int }
	var cuts []cut
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.IMPORT {
			continue
		}
		var unused []*ast.ImportSpec
		for _, sp := range g.Specs {
			imp := sp.(*ast.ImportSpec)
			if n := importName(imp); n != "_" && n != "." && !used[n] {
				unused = append(unused, imp)
			}
		}
		switch {
		case len(unused) == 0:
		case len(unused) == len(g.Specs):
			cuts = append(cuts, cut{fset.Position(g.Pos()).Offset, fset.Position(g.End()).Offset})
		default:
			for _, imp := range unused {
				cuts = append(cuts, cut{fset.Position(imp.Pos()).Offset, fset.Position(imp.End()).Offset})
			}
		}
	}
	sort.Slice(cuts, func(i, j int) bool { return cuts[i].from > cuts[j].from })
	for _, c := range cuts {
		to := c.to
		for to < len(src) && src[to] == '\n' {
			to++
		}
		src = append(append([]byte{}, src[:c.from]...), src[to:]...)
	}
	return format.Source(src)
}

// write replaces the generated directories under outDir with files.
func write(root string, files map[string][]byte) error {
	for _, s := range specs {
		if err := os.RemoveAll(filepath.Join(root, outDir, s.Name)); err != nil {
			return err
		}
	}
	for _, name := range sortedKeys(files) {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

// onDisk reads the generated directories under outDir, as build returns them.
func onDisk(root string) (map[string][]byte, error) {
	out := map[string][]byte{}
	for _, s := range specs {
		dir := filepath.Join(root, outDir, s.Name)
		err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = b
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	return out, nil
}
