package lint

import (
	"go/ast"
	"go/token"
	"go/types"
)

// tokenFunc is a function of the linted code (a declaration, or a function
// literal bound to a variable) that may take a theme token name as an
// argument.
type tokenFunc struct {
	unit   *unit
	typ    *ast.FuncType
	body   *ast.BlockStmt
	params []*types.Var // one per parameter position, nil for unnamed ones
	// tokenArgs are the parameter positions that name a theme color: the
	// function stores a color under that parameter as the key
	// (X.Colors[name] = ...), or passes it on to another token function.
	tokenArgs map[int]bool
}

// collectTokenFuncs finds the functions that register theme colors by a
// name they are given, such as
//
//	set := func(name, light, dark string) { t.Colors[name] = ... }
//	set("gold", ...)
//
// and returns them by the object a call to them resolves to. A parameter
// names a token when its function assigns to a color map under it, or hands
// it to a parameter that does (checked until nothing changes). Only the
// constant arguments of calls to such a parameter are token names, so an
// unrelated string constant elsewhere is never one.
func (r *run) collectTokenFuncs() map[types.Object]*tokenFunc {
	funcs := map[types.Object]*tokenFunc{}
	for _, u := range r.units {
		for _, f := range u.files {
			ast.Inspect(f, func(n ast.Node) bool {
				addTokenFuncs(funcs, u, n)
				return true
			})
		}
	}

	for changed := true; changed; {
		changed = false
		for _, tf := range funcs {
			if tf.markTokenArgs(r, funcs) {
				changed = true
			}
		}
	}
	return funcs
}

// addTokenFuncs adds to funcs the function that n declares or binds to a
// variable, if it is one.
func addTokenFuncs(funcs map[types.Object]*tokenFunc, u *unit, n ast.Node) {
	switch n := n.(type) {
	case *ast.FuncDecl:
		addTokenFunc(funcs, u, u.info.Defs[n.Name], n.Type, n.Body)
	case *ast.AssignStmt:
		if n.Tok != token.DEFINE || len(n.Lhs) != len(n.Rhs) {
			return
		}
		for i, l := range n.Lhs {
			if id, ok := l.(*ast.Ident); ok {
				addFuncLit(funcs, u, id, n.Rhs[i])
			}
		}
	case *ast.ValueSpec:
		if len(n.Names) != len(n.Values) {
			return
		}
		for i, id := range n.Names {
			addFuncLit(funcs, u, id, n.Values[i])
		}
	}
}

// addFuncLit adds to funcs the function literal e that the variable id is
// bound to; any other e is ignored.
func addFuncLit(funcs map[types.Object]*tokenFunc, u *unit, id *ast.Ident, e ast.Expr) {
	if lit, ok := ast.Unparen(e).(*ast.FuncLit); ok {
		addTokenFunc(funcs, u, u.info.Defs[id], lit.Type, lit.Body)
	}
}

// addTokenFunc adds to funcs the function with the given type and body,
// under the object a call to it resolves to.
func addTokenFunc(funcs map[types.Object]*tokenFunc, u *unit, obj types.Object, typ *ast.FuncType, body *ast.BlockStmt) {
	if obj == nil || body == nil {
		return
	}
	if fn, ok := obj.(*types.Func); ok {
		obj = fn.Origin()
	}
	tf := &tokenFunc{unit: u, typ: typ, body: body, tokenArgs: map[int]bool{}}
	for _, f := range typ.Params.List {
		if len(f.Names) == 0 {
			tf.params = append(tf.params, nil)
		}
		for _, n := range f.Names {
			v, _ := u.info.Defs[n].(*types.Var)
			tf.params = append(tf.params, v)
		}
	}
	funcs[obj] = tf
}

// markTokenArgs marks the parameters of tf that name a token, as far as the
// body of tf shows, and reports whether it marked a new one.
func (tf *tokenFunc) markTokenArgs(r *run, funcs map[types.Object]*tokenFunc) bool {
	p := &pass{r: r, u: tf.unit, info: tf.unit.info}
	changed := false
	ast.Inspect(tf.body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.AssignStmt:
			changed = tf.markAssign(p, n) || changed
		case *ast.CallExpr:
			changed = tf.markCall(p, funcs, n) || changed
		}
		return true
	})
	return changed
}

// markAssign marks the parameters that the assignment as uses as the key of
// a color map (X.Colors[name] = ...), and reports whether it marked a new
// one.
func (tf *tokenFunc) markAssign(p *pass, as *ast.AssignStmt) bool {
	changed := false
	for _, l := range as.Lhs {
		if ix, ok := l.(*ast.IndexExpr); ok && isColorMap(p.info.TypeOf(ix.X)) {
			changed = tf.mark(tf.param(ix.Index)) || changed
		}
	}
	return changed
}

// markCall marks the parameters that call hands to a parameter of another
// token function that names a token, and reports whether it marked a new
// one.
func (tf *tokenFunc) markCall(p *pass, funcs map[types.Object]*tokenFunc, call *ast.CallExpr) bool {
	callee := funcs[p.calleeObj(call)]
	if callee == nil {
		return false
	}
	changed := false
	for i := range callee.tokenArgs {
		if i < len(call.Args) {
			changed = tf.mark(tf.param(call.Args[i])) || changed
		}
	}
	return changed
}

// mark marks the parameter at position i, if there is one, and reports
// whether it was not marked yet.
func (tf *tokenFunc) mark(i int) bool {
	if i < 0 || tf.tokenArgs[i] {
		return false
	}
	tf.tokenArgs[i] = true
	return true
}

// param returns the position of the parameter that e names, or -1.
func (tf *tokenFunc) param(e ast.Expr) int {
	id, ok := ast.Unparen(e).(*ast.Ident)
	if !ok {
		return -1
	}
	v, _ := tf.unit.info.Uses[id].(*types.Var)
	for i, pv := range tf.params {
		if v != nil && pv == v {
			return i
		}
	}
	return -1
}

// calleeObj returns the object a call resolves to by name: a declared
// function or method, or the variable a function value is kept in. It is
// nil for any other callee.
func (p *pass) calleeObj(call *ast.CallExpr) types.Object {
	fun := ast.Unparen(call.Fun)
	for {
		switch f := fun.(type) {
		case *ast.IndexExpr:
			fun = ast.Unparen(f.X)
			continue
		case *ast.IndexListExpr:
			fun = ast.Unparen(f.X)
			continue
		}
		break
	}
	var id *ast.Ident
	switch f := fun.(type) {
	case *ast.Ident:
		id = f
	case *ast.SelectorExpr:
		id = f.Sel
	}
	if id == nil {
		return nil
	}
	obj := p.info.Uses[id]
	if fn, ok := obj.(*types.Func); ok {
		return fn.Origin()
	}
	return obj
}

// tokenCallEntries returns the theme colors that a call to a token
// function names with constant arguments.
func (p *pass) tokenCallEntries(call *ast.CallExpr, funcs map[types.Object]*tokenFunc) []*colorEntry {
	obj := p.calleeObj(call)
	if obj == nil {
		return nil
	}
	tf := funcs[obj]
	if tf == nil {
		return nil
	}
	var out []*colorEntry
	for i := range tf.tokenArgs {
		if i < len(call.Args) {
			if name, ok := p.constString(call.Args[i]); ok {
				out = append(out, &colorEntry{name, call.Args[i], p.u})
			}
		}
	}
	return out
}
