package lint

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"sort"
	"strconv"
)

// Fixes (tela lint -fix). A rule that knows a safe rewrite attaches it to
// its finding as edits: byte ranges of the file as it was read, each
// replaced by a text (empty to delete), with the import the text needs. The fixes of a file are
// applied together, the file is formatted with gofmt (a removed wrapper
// changes the indentation of what it wrapped) and it is written only if the
// result still parses, so a file is never left broken.

// edit deletes the bytes [start, end) of file.
type edit struct {
	file       string
	start, end int
	// text replaces the bytes (empty deletes them), and imp is the import
	// path the text needs the file to have.
	text, imp string
}

// fixKey identifies a finding, to find its edits.
type fixKey struct {
	file      string
	line, col int
	rule      string
}

// reportFix is report with the edits of a fix (nil for none).
func (p *pass) reportFix(pos token.Pos, rule, severity, msg, suggestion string, edits []edit) {
	n := len(p.r.diags)
	p.report(pos, rule, severity, msg, suggestion)
	if len(p.r.diags) == n || edits == nil {
		return
	}
	d := &p.r.diags[n]
	d.Fixable = true
	if p.r.fixes == nil {
		p.r.fixes = map[fixKey][]edit{}
	}
	p.r.fixes[fixKeyOf(*d)] = edits
}

func fixKeyOf(d Diagnostic) fixKey { return fixKey{d.File, d.Line, d.Col, d.Rule} }

// applyFixes applies the fixes of the findings in diags, and returns the
// findings that were fixed and the number of files written. A file whose
// fixed text does not parse or cannot be written is left as it was, and its
// findings stay.
func (r *run) applyFixes(diags []Diagnostic) (fixed []Diagnostic, files int, err error) {
	byFile := map[string][]edit{}
	diagsOf := map[string][]Diagnostic{}
	for _, d := range diags {
		if !d.Fixable {
			continue
		}
		edits := r.fixes[fixKeyOf(d)]
		if len(edits) == 0 {
			continue
		}
		file := edits[0].file
		byFile[file] = append(byFile[file], edits...)
		diagsOf[file] = append(diagsOf[file], d)
	}
	names := make([]string, 0, len(byFile))
	for f := range byFile {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, name := range names {
		src, rerr := r.source(name)
		if rerr != nil {
			err = rerr
			continue
		}
		out, ok := applyEdits(src, byFile[name])
		if !ok {
			err = fmt.Errorf("%s: the fixes overlap or do not match the file; it is left as it was", r.rel(name))
			continue
		}
		out = dropUnusedImports(src, out)
		for _, e := range byFile[name] {
			if e.imp != "" {
				out = addImport(out, e.imp)
			}
		}
		formatted, ferr := format.Source(out)
		if ferr != nil {
			err = fmt.Errorf("%s: the fixed file does not parse (%v); it is left as it was", r.rel(name), ferr)
			continue
		}
		if !bytes.Equal(formatted, src) {
			mode := os.FileMode(0o644)
			if info, serr := os.Stat(name); serr == nil {
				mode = info.Mode().Perm()
			}
			if werr := os.WriteFile(name, formatted, mode); werr != nil {
				err = werr
				continue
			}
		}
		files++
		fixed = append(fixed, diagsOf[name]...)
	}
	return fixed, files, err
}

// applyEdits replaces the ranges of src with their texts. Identical edits (the same finding
// reached twice) apply once; overlapping different ones are a failure.
func applyEdits(src []byte, edits []edit) ([]byte, bool) {
	sort.Slice(edits, func(i, j int) bool {
		if edits[i].start != edits[j].start {
			return edits[i].start < edits[j].start
		}
		return edits[i].end < edits[j].end
	})
	var out []byte
	pos := 0
	var last edit
	for i, e := range edits {
		if i > 0 && e == last {
			continue
		}
		if e.start < pos || e.end < e.start || e.end > len(src) {
			return nil, false
		}
		out = append(out, src[pos:e.start]...)
		out = append(out, e.text...)
		pos = e.end
		last = e
	}
	return append(out, src[pos:]...), true
}

// dropUnusedImports removes the imports that the edits left without a use:
// a named import (not a dot or blank one) that old used through a selector
// and updated no longer does. The local name is the one written, or the
// last element of the path, so an import whose package name differs from
// it is left alone.
func dropUnusedImports(old, updated []byte) []byte {
	fset := token.NewFileSet()
	oldFile, err := parser.ParseFile(fset, "old.go", old, 0)
	if err != nil {
		return updated
	}
	newFset := token.NewFileSet()
	newFile, err := parser.ParseFile(newFset, "new.go", updated, 0)
	if err != nil {
		return updated
	}
	oldUses, newUses := selectorBases(oldFile), selectorBases(newFile)
	var cuts [][2]int
	for _, decl := range newFile.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		var drop []*ast.ImportSpec
		for _, spec := range gen.Specs {
			imp := spec.(*ast.ImportSpec)
			name := ""
			if imp.Name != nil {
				name = imp.Name.Name
			} else if p, err := strconv.Unquote(imp.Path.Value); err == nil {
				name = path.Base(p)
			}
			if name == "" || name == "_" || name == "." || !oldUses[name] || newUses[name] {
				continue
			}
			drop = append(drop, imp)
		}
		if len(drop) == 0 {
			continue
		}
		if len(drop) == len(gen.Specs) {
			cuts = append(cuts, lineSpan(updated, newFset, gen.Pos(), gen.End()))
			continue
		}
		for _, imp := range drop {
			cuts = append(cuts, lineSpan(updated, newFset, imp.Pos(), imp.End()))
		}
	}
	if len(cuts) == 0 {
		return updated
	}
	var out []byte
	pos := 0
	for _, c := range cuts {
		out = append(out, updated[pos:c[0]]...)
		pos = c[1]
	}
	return append(out, updated[pos:]...)
}

// selectorBases returns the names x of the selectors x.Sel in f that are
// not a local variable.
func selectorBases(f *ast.File) map[string]bool {
	uses := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Obj == nil {
				uses[id.Name] = true
			}
		}
		return true
	})
	return uses
}

// lineSpan returns the byte range of the lines that [from, to) is on, with
// the line break.
func lineSpan(src []byte, fset *token.FileSet, from, to token.Pos) [2]int {
	start, end := fset.Position(from).Offset, fset.Position(to).Offset
	for start > 0 && src[start-1] != '\n' {
		start--
	}
	for end < len(src) && src[end] != '\n' {
		end++
	}
	if end < len(src) {
		end++
	}
	return [2]int{start, end}
}

// addImport adds the import of path to src, which already parses, unless it
// has it. The import goes into the first import declaration that has
// parentheses, as a new line (format.Source sorts the block), or after the
// last import (the package clause when there is none) as a declaration of
// its own.
func addImport(src []byte, importPath string) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, parser.ImportsOnly)
	if err != nil {
		return src
	}
	quoted := strconv.Quote(importPath)
	for _, imp := range f.Imports {
		if imp.Path.Value == quoted && (imp.Name == nil || imp.Name.Name != "_") {
			return src
		}
	}
	insert := func(off int, text string) []byte {
		out := append([]byte(nil), src[:off]...)
		out = append(out, text...)
		return append(out, src[off:]...)
	}
	for _, d := range f.Decls {
		if gen, ok := d.(*ast.GenDecl); ok && gen.Tok == token.IMPORT && gen.Lparen.IsValid() {
			return insert(fset.Position(gen.Lparen).Offset+1, "\n\t"+quoted)
		}
	}
	end := fset.Position(f.Name.End()).Offset
	for _, d := range f.Decls {
		if gen, ok := d.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			end = fset.Position(gen.End()).Offset
		}
	}
	return insert(end, "\n\nimport "+quoted)
}
