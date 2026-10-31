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
// deleted (the rewrites so far only remove code). The fixes of a file are
// applied together, the file is formatted with gofmt (a removed wrapper
// changes the indentation of what it wrapped) and it is written only if the
// result still parses, so a file is never left broken.

// edit deletes the bytes [start, end) of file.
type edit struct {
	file       string
	start, end int
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
		src, rerr := os.ReadFile(name)
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

// applyEdits deletes the ranges from src. Identical edits (the same finding
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
