package lint

import (
	"errors"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kuronosu/tela/internal/style"
)

// The class rule: every constant string that reaches the class parser is
// compiled with the runtime's own parser, and what dev mode would print
// for it is reported.
//
// The strings that reach the parser are:
//
//   - the arguments of h.Class and the argument of h.Exit;
//   - the argument of telatest.RootClass;
//   - the fields of the Tela packages (tela, kit, aria, h) named Class or
//     ending in Class (tela.App.RootClass, kit.Button.Class,
//     kit.Card.HeaderClass...), in composite literals and assignments;
//   - every string, string slice and string map field of a kit recipe, a
//     struct named ...Style in package kit (kit.ButtonStyle.Root, its
//     Variants and Sizes maps...), in composite literals and assignments;
//   - the values of the maps given as tela.App.ClassAliases and
//     telatest.ClassAliases (checked once the aliases are known).

// checkClassCall checks the call sinks: h.Class, h.Exit, telatest.RootClass.
func (p *pass) checkClassCall(call *ast.CallExpr, fn *types.Func) {
	switch {
	case p.isFunc(fn, "h", "Class"):
		if call.Ellipsis != token.NoPos {
			return
		}
		for _, a := range call.Args {
			p.classValue(a)
		}
	case p.isFunc(fn, "h", "Exit"), p.isFunc(fn, "telatest", "RootClass"):
		if len(call.Args) == 1 {
			p.classValue(call.Args[0])
		}
	}
}

// checkComposite checks the class fields set in a struct literal.
func (p *pass) checkComposite(cl *ast.CompositeLit) {
	named := namedOf(p.info.TypeOf(cl))
	if named == nil {
		return
	}
	st, ok := named.Underlying().(*types.Struct)
	if !ok {
		return
	}
	for i, el := range cl.Elts {
		var field *types.Var
		val := el
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			id, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			field, _ = p.info.Uses[id].(*types.Var)
			val = kv.Value
		} else if i < st.NumFields() {
			field = st.Field(i)
		}
		if field != nil {
			if p.isClassField(field, named) {
				p.classValue(val)
			}
			p.checkFieldValue(field, named, val)
		}
	}
}

// checkAssign checks the class fields assigned: x.Class = "...",
// style.Button.Root += "...", style.Button.Variants[kit.Ghost] = "...".
func (p *pass) checkAssign(as *ast.AssignStmt) {
	if (as.Tok != token.ASSIGN && as.Tok != token.ADD_ASSIGN) || len(as.Lhs) != len(as.Rhs) {
		return
	}
	for i, lhs := range as.Lhs {
		lhs = ast.Unparen(lhs)
		if ix, ok := lhs.(*ast.IndexExpr); ok {
			lhs = ast.Unparen(ix.X)
		}
		sel, ok := lhs.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		s := p.info.Selections[sel]
		if s == nil || s.Kind() != types.FieldVal {
			continue
		}
		field, _ := s.Obj().(*types.Var)
		owner := namedOf(p.info.TypeOf(sel.X))
		if field == nil || owner == nil {
			continue
		}
		if p.isClassField(field, owner) {
			p.classValue(as.Rhs[i])
		}
		if as.Tok == token.ASSIGN {
			p.checkFieldValue(field, owner, as.Rhs[i])
		}
	}
}

// isClassField reports whether a field of owner holds classes.
func (p *pass) isClassField(f *types.Var, owner *types.Named) bool {
	f = f.Origin()
	if !f.IsField() || f.Pkg() == nil || !stringish(f.Type()) {
		return false
	}
	path := f.Pkg().Path()
	switch path {
	case relPkg(""), relPkg("kit"), relPkg("aria"), relPkg("h"):
	default:
		return false
	}
	if n := f.Name(); n == "Class" || strings.HasSuffix(n, "Class") {
		return true
	}
	// A recipe holds classes in all of its string fields.
	return path == relPkg("kit") && owner != nil && owner.Obj().Pkg() != nil &&
		owner.Obj().Pkg().Path() == path && strings.HasSuffix(owner.Obj().Name(), "Style")
}

// stringish reports whether t is a string, or a slice, array or map that
// holds strings.
func stringish(t types.Type) bool {
	switch u := types.Unalias(t).Underlying().(type) {
	case *types.Basic:
		return u.Kind() == types.String
	case *types.Slice:
		return stringish(u.Elem())
	case *types.Array:
		return stringish(u.Elem())
	case *types.Map:
		return stringish(u.Elem())
	}
	return false
}

// classValue checks the class strings that expression e stands for: a
// constant string, or the constant strings in a slice or map literal.
func (p *pass) classValue(e ast.Expr) {
	e = ast.Unparen(e)
	if cl, ok := e.(*ast.CompositeLit); ok {
		if _, isStruct := types.Unalias(p.info.TypeOf(cl)).Underlying().(*types.Struct); isStruct {
			return // checked as a struct literal of its own
		}
		for _, el := range cl.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				el = kv.Value
			}
			p.classValue(el)
		}
		return
	}
	s, ok := p.constString(e)
	if !ok {
		return
	}
	p.classString(e, s)
}

// classString compiles s, a class string written at e, and reports what is
// wrong with it.
func (p *pass) classString(e ast.Expr, s string) {
	c := p.r.parser.Compile(s)
	if c.Err == nil {
		return
	}
	var all style.ClassErrors
	if !errors.As(c.Err, &all) {
		all = style.ClassErrors{c.Err}
	}
	for _, err := range all {
		msg, sugg := err.Error(), ""
		var unknown *style.UnknownClassError
		var invalid *style.InvalidClassError
		switch {
		case errors.As(err, &unknown):
			sugg = unknown.Suggestion
		case errors.As(err, &invalid):
			sugg = invalid.Suggestion
		}
		p.report(p.classPos(e, errClass(err)), RuleClass, SeverityError, msg, sugg)
	}
}

// errClass returns the class a class error is about, as written.
func errClass(err error) string {
	var alias *style.AliasError
	var unknown *style.UnknownClassError
	var unsupported *style.UnsupportedClassError
	var invalid *style.InvalidClassError
	switch {
	case errors.As(err, &alias):
		return alias.Alias
	case errors.As(err, &unknown):
		return unknown.Class
	case errors.As(err, &unsupported):
		return unsupported.Class
	case errors.As(err, &invalid):
		return invalid.Class
	}
	return ""
}

// classPos returns the position of class inside the string literal e, or
// the start of e when it is not found or e is not a plain literal.
func (p *pass) classPos(e ast.Expr, class string) token.Pos {
	lit, ok := ast.Unparen(e).(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING || class == "" {
		return e.Pos()
	}
	src := lit.Value
	for from := 0; from < len(src); {
		i := strings.Index(src[from:], class)
		if i < 0 {
			break
		}
		i += from
		end := i + len(class)
		if boundary(src, i-1) && boundary(src, end) {
			return lit.Pos() + token.Pos(i)
		}
		from = i + 1
	}
	return e.Pos()
}

// boundary reports whether src[i] separates classes: white space, a quote
// or the ends.
func boundary(src string, i int) bool {
	if i < 0 || i >= len(src) {
		return true
	}
	switch src[i] {
	case ' ', '\t', '\n', '\r', '"', '`':
		return true
	}
	return false
}

// checkAliasValues compiles the values of the aliases found, which may use
// each other.
func (r *run) checkAliasValues() {
	for _, a := range r.aliasList {
		if !a.hasValue {
			continue
		}
		p := &pass{r: r, u: a.unit, info: a.unit.info}
		p.classString(a.valueExp, a.value)
	}
}
