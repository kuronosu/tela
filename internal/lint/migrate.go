package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strconv"
	"strings"
)

// The migrate rule (ADR-305): every change that breaks the public API ships
// a rewrite that `tela lint -fix` applies to the old spelling, so that an
// app moves to the new release by running one command. The rewrites are the
// entries of Migrations (migrations.go), each with an example of old code
// and the new code it becomes, which a test runs.
//
// Old code does not type-check against the new API, so the rule reads what
// go/types resolved of it: the type of the operand of a selector, the
// package an identifier names, an import path. A package that fails to
// type-check is checked for migrations alone (the other rules need all of
// its types). A migration reports nothing when the old name still exists,
// so one that is re-added later does not flag the new code.

// Migration is the rewrite for one breaking change of the public API. Set
// exactly one of Member, Ident, Import and Custom.
type Migration struct {
	// ID names the migration: lower case words joined by dashes. A
	// "Breaking" entry of CHANGELOG.md cites it as migrate: ID.
	ID string
	// Since is the release that broke the API, such as "v0.14.1".
	Since string
	// Note says what changed, for the message of a finding. A rename gets
	// one from its fields when it is empty.
	Note string

	// Member renames a method or field of a type of Tela.
	Member *MemberRename
	// Ident renames a function, type, variable or constant of a package of
	// Tela.
	Ident *IdentRename
	// Import moves a package to another import path.
	Import *ImportMove
	// Custom is for what a rename does not say: it is called for every
	// node of the linted files.
	Custom func(p *pass, n ast.Node, m *Migration)

	// Example is old code and the new code -fix makes of it; every
	// migration has one, and a test checks it.
	Example Example
}

// MemberRename renames the method or field Old of the type Pkg.Type to New.
// Pkg is the path of the package inside the module ("telatest").
type MemberRename struct {
	Pkg, Type, Old, New string
}

// IdentRename renames Old of the package Pkg to New.
type IdentRename struct {
	Pkg, Old, New string
}

// ImportMove moves the package at the import path Old to New. NewName is
// the name of the package at New when it differs from the last element of
// Old; the uses of the old name become it.
type ImportMove struct {
	Old, New string
	OldName  string
	NewName  string
}

// Example is a file of old code and what -fix makes of it, as the Go source
// of a file of package example. Both are gofmt-formatted; New must compile.
type Example struct {
	Old, New string
}

// checkMigrations runs the migrations of the run on the node being visited.
func (p *pass) checkMigrations(n ast.Node) {
	for i := range p.r.migrations {
		m := &p.r.migrations[i]
		switch {
		case m.Member != nil:
			p.memberRename(m, n)
		case m.Ident != nil:
			p.identRename(m, n)
		case m.Import != nil:
			p.importMove(m, n)
		case m.Custom != nil:
			m.Custom(p, n, m)
		}
	}
}

func (m *Migration) message(what string) string {
	if m.Note != "" {
		return m.Note
	}
	return fmt.Sprintf("%s was removed in %s", what, m.Since)
}

// reportMigration reports a finding of m at pos, with the edits that fix it.
func (p *pass) reportMigration(m *Migration, pos token.Pos, msg string, edits []edit) {
	p.reportFix(pos, RuleMigrate, SeverityError, msg+"; tela lint -fix rewrites it (migrate: "+m.ID+")", "", edits)
}

// replaceEdit replaces the source of [from, to) with text.
func (p *pass) replaceEdit(from, to token.Pos, text, imp string) []edit {
	fset := p.r.l.fset
	start, end := fset.PositionFor(from, false), fset.PositionFor(to, false)
	if !start.IsValid() || start.Filename != end.Filename {
		return nil
	}
	return []edit{{file: start.Filename, start: start.Offset, end: end.Offset, text: text, imp: imp}}
}

// memberRename reports a selector x.Old whose x is a Pkg.Type that has no
// member Old, and a composite literal of that type with the field Old.
func (p *pass) memberRename(m *Migration, n ast.Node) {
	r := m.Member
	switch n := n.(type) {
	case *ast.SelectorExpr:
		if n.Sel.Name != r.Old {
			return
		}
		t := p.info.TypeOf(n.X)
		if !p.isTelaType(t, r.Pkg, r.Type) || p.hasMember(t, r.Old) {
			return
		}
		p.reportMigration(m, n.Sel.Pos(), m.message(fmt.Sprintf("%s.%s.%s", r.Pkg, r.Type, r.Old)),
			p.replaceEdit(n.Sel.Pos(), n.Sel.End(), r.New, ""))
	case *ast.CompositeLit:
		t := p.info.TypeOf(n)
		if !p.isTelaType(t, r.Pkg, r.Type) || p.hasMember(t, r.Old) {
			return
		}
		for _, el := range n.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok || !isIdent(kv.Key, r.Old) {
				continue
			}
			key := kv.Key.(*ast.Ident)
			p.reportMigration(m, key.Pos(), m.message(fmt.Sprintf("the field %s.%s.%s", r.Pkg, r.Type, r.Old)),
				p.replaceEdit(key.Pos(), key.End(), r.New, ""))
		}
	}
}

// hasMember reports whether the type t (or a pointer to it) has a field or
// method called name.
func (p *pass) hasMember(t types.Type, name string) bool {
	if t == nil {
		return false
	}
	if _, isPtr := types.Unalias(t).(*types.Pointer); !isPtr {
		t = types.NewPointer(t)
	}
	obj, _, _ := types.LookupFieldOrMethod(t, true, p.u.pkg, name)
	return obj != nil
}

// identRename reports pkg.Old, where pkg is the import of Pkg and has no
// Old, and Old alone in a file that dot-imports Pkg.
func (p *pass) identRename(m *Migration, n ast.Node) {
	r := m.Ident
	switch n := n.(type) {
	case *ast.SelectorExpr:
		id, ok := n.X.(*ast.Ident)
		if !ok || n.Sel.Name != r.Old {
			return
		}
		pn, ok := p.info.Uses[id].(*types.PkgName)
		if !ok || pn.Imported().Path() != relPkg(r.Pkg) || pn.Imported().Scope().Lookup(r.Old) != nil {
			return
		}
		p.reportMigration(m, n.Sel.Pos(), m.message(fmt.Sprintf("%s.%s", r.Pkg, r.Old)),
			p.replaceEdit(n.Sel.Pos(), n.Sel.End(), r.New, ""))
	case *ast.Ident:
		if n.Name != r.Old || p.info.Uses[n] != nil || p.info.Defs[n] != nil || !p.dotImports(n.Pos(), relPkg(r.Pkg)) {
			return
		}
		// A name that is a field, a method or a key is not the package's.
		if parent, ok := p.parentOf(n); ok {
			switch x := parent.(type) {
			case *ast.SelectorExpr:
				if x.Sel == n {
					return
				}
			case *ast.KeyValueExpr:
				if x.Key == n {
					return
				}
			}
		}
		p.reportMigration(m, n.Pos(), m.message(r.Old),
			p.replaceEdit(n.Pos(), n.End(), r.New, ""))
	}
}

// dotImports reports whether the file at pos imports the package at the
// import path with a dot.
func (p *pass) dotImports(pos token.Pos, importPath string) bool {
	f := p.r.fileAST[p.r.l.fset.Position(pos).Filename]
	if f == nil {
		return false
	}
	for _, imp := range f.Imports {
		if imp.Name != nil && imp.Name.Name == "." {
			if v, err := strconv.Unquote(imp.Path.Value); err == nil && v == importPath {
				return true
			}
		}
	}
	return false
}

// importMove reports the import of the old path, and renames the uses of
// the old package name when the package's name changed.
func (p *pass) importMove(m *Migration, n ast.Node) {
	r := m.Import
	spec, ok := n.(*ast.ImportSpec)
	if !ok {
		return
	}
	if v, err := strconv.Unquote(spec.Path.Value); err != nil || v != r.Old {
		return
	}
	edits := p.replaceEdit(spec.Path.Pos(), spec.Path.End(), strconv.Quote(r.New), "")
	oldName := r.OldName
	if oldName == "" {
		oldName = path.Base(r.Old)
	}
	if edits != nil && spec.Name == nil && r.NewName != "" && r.NewName != oldName {
		file := p.r.fileAST[p.r.l.fset.Position(spec.Pos()).Filename]
		if file == nil {
			edits = nil
		} else {
			ast.Inspect(file, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				if pn, ok := p.info.Uses[id].(*types.PkgName); ok && pn.Imported().Path() == r.Old && id.Name == oldName {
					edits = append(edits, p.replaceEdit(id.Pos(), id.End(), r.NewName, "")...)
				}
				return true
			})
		}
	}
	p.reportMigration(m, spec.Path.Pos(), m.message("the package "+r.Old+" moved to "+r.New+" in "+m.Since+" and"), edits)
}

// pkgQualifier returns what a reference to the package at importPath is
// written with in the file at pos: its local name and a dot, nothing when
// the file dot-imports it, and the import to add when the file has none.
// ok is false when the name the package would take is already used in the
// file for something else.
func (p *pass) pkgQualifier(pos token.Pos, importPath string) (qual, addImport string, ok bool) {
	f := p.r.fileAST[p.r.l.fset.Position(pos).Filename]
	if f == nil {
		return "", "", false
	}
	name := path.Base(importPath)
	taken := map[string]bool{}
	for _, imp := range f.Imports {
		v, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		local := path.Base(v)
		if imp.Name != nil {
			local = imp.Name.Name
		}
		if v == importPath {
			switch local {
			case ".":
				return "", "", true
			case "_":
			default:
				return local + ".", "", true
			}
			continue
		}
		taken[local] = true
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				taken[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				switch s := s.(type) {
				case *ast.ValueSpec:
					for _, id := range s.Names {
						taken[id.Name] = true
					}
				case *ast.TypeSpec:
					taken[s.Name.Name] = true
				}
			}
		}
	}
	if taken[name] {
		return "", "", false
	}
	return name + ".", importPath, true
}

// sourceText returns the source of [from, to) of the file it is in.
func (p *pass) sourceText(from, to token.Pos) (string, bool) {
	fset := p.r.l.fset
	start, end := fset.PositionFor(from, false), fset.PositionFor(to, false)
	if !start.IsValid() || start.Filename != end.Filename {
		return "", false
	}
	src, err := p.r.source(start.Filename)
	if err != nil || end.Offset > len(src) {
		return "", false
	}
	return string(src[start.Offset:end.Offset]), true
}

// launchForwarded is the migration of desktop.Launch.Forwarded, which
// Launch.Source replaced (ADR-253): a read becomes a comparison with
// desktop.LaunchForwarded, `Forwarded: true` in a literal becomes `Source:
// desktop.LaunchForwarded`; a write or another value is reported without a
// fix.
func launchForwarded(p *pass, n ast.Node, m *Migration) {
	switch n := n.(type) {
	case *ast.SelectorExpr:
		if n.Sel.Name != "Forwarded" {
			return
		}
		t := p.info.TypeOf(n.X)
		if !p.isTelaType(t, "desktop", "Launch") || p.hasMember(t, "Forwarded") {
			return
		}
		msg := m.message("desktop.Launch.Forwarded")
		qual, imp, ok := p.pkgQualifier(n.Pos(), relPkg("desktop"))
		x, okText := p.sourceText(n.X.Pos(), n.X.End())
		parent, hasParent := p.parentOf(n)
		if !ok || !okText || !hasParent {
			p.reportMigration(m, n.Sel.Pos(), msg, nil)
			return
		}
		if as, isAssign := parent.(*ast.AssignStmt); isAssign {
			for _, l := range as.Lhs {
				if l == ast.Expr(n) {
					p.reportMigration(m, n.Sel.Pos(), msg, nil)
					return
				}
			}
		}
		from, to, op, wrap := n.Pos(), n.End(), "==", false
		if u, isUnary := parent.(*ast.UnaryExpr); isUnary && u.Op == token.NOT {
			from, to, op = u.Pos(), u.End(), "!="
		} else {
			wrap = needsParens(parent)
		}
		text := x + ".Source " + op + " " + qual + "LaunchForwarded"
		if wrap {
			text = "(" + text + ")"
		}
		p.reportMigration(m, n.Sel.Pos(), msg, p.replaceEdit(from, to, text, imp))
	case *ast.CompositeLit:
		t := p.info.TypeOf(n)
		if !p.isTelaType(t, "desktop", "Launch") || p.hasMember(t, "Forwarded") {
			return
		}
		for _, el := range n.Elts {
			kv, ok := el.(*ast.KeyValueExpr)
			if !ok || !isIdent(kv.Key, "Forwarded") {
				continue
			}
			msg := m.message("desktop.Launch.Forwarded")
			qual, imp, ok := p.pkgQualifier(kv.Pos(), relPkg("desktop"))
			if v, isConst := p.constBool(kv.Value); ok && isConst && v {
				p.reportMigration(m, kv.Key.Pos(), msg, p.replaceEdit(kv.Pos(), kv.End(), "Source: "+qual+"LaunchForwarded", imp))
			} else {
				p.reportMigration(m, kv.Key.Pos(), msg, nil)
			}
		}
	}
}

// needsParens reports whether a comparison written where a selector was,
// inside parent, needs parentheses.
func needsParens(parent ast.Node) bool {
	switch x := parent.(type) {
	case *ast.IfStmt, *ast.ReturnStmt, *ast.AssignStmt, *ast.ValueSpec, *ast.ExprStmt, *ast.CallExpr,
		*ast.ParenExpr, *ast.ForStmt, *ast.KeyValueExpr, *ast.CompositeLit, *ast.SwitchStmt, *ast.CaseClause:
		return false
	case *ast.BinaryExpr:
		return x.Op != token.LAND && x.Op != token.LOR
	}
	return true
}

// constBool returns the value of a constant bool expression.
func (p *pass) constBool(e ast.Expr) (val, ok bool) {
	tv, found := p.info.Types[e]
	if !found || tv.Value == nil || tv.Value.String() != "true" && tv.Value.String() != "false" {
		return false, false
	}
	return tv.Value.String() == "true", true
}

// migrationProblems returns the problems of the entries of a table that
// need no go command to see (the tests of the table add the ones that do).
func migrationProblems(ms []Migration) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range ms {
		n := 0
		for _, set := range []bool{m.Member != nil, m.Ident != nil, m.Import != nil, m.Custom != nil} {
			if set {
				n++
			}
		}
		switch {
		case m.ID == "" || strings.ToLower(m.ID) != m.ID || strings.ContainsAny(m.ID, " _."):
			out = append(out, fmt.Sprintf("migration %q: the ID is lower case words joined by dashes", m.ID))
		case seen[m.ID]:
			out = append(out, fmt.Sprintf("migration %q is listed twice", m.ID))
		case n != 1:
			out = append(out, fmt.Sprintf("migration %q: set exactly one of Member, Ident, Import and Custom", m.ID))
		case !strings.HasPrefix(m.Since, "v0."):
			out = append(out, fmt.Sprintf("migration %q: Since %q is not a release such as v0.14.1", m.ID, m.Since))
		case m.Example.Old == "" || m.Example.New == "":
			out = append(out, fmt.Sprintf("migration %q has no example: give the old code and the new code it becomes", m.ID))
		}
		seen[m.ID] = true
	}
	return out
}
