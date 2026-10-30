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
	add := func(u *unit, obj types.Object, typ *ast.FuncType, body *ast.BlockStmt) {
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
	for _, u := range r.units {
		for _, f := range u.files {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.FuncDecl:
					add(u, u.info.Defs[n.Name], n.Type, n.Body)
				case *ast.AssignStmt:
					if n.Tok == token.DEFINE && len(n.Lhs) == len(n.Rhs) {
						for i, l := range n.Lhs {
							if id, ok := l.(*ast.Ident); ok {
								if lit, ok := ast.Unparen(n.Rhs[i]).(*ast.FuncLit); ok {
									add(u, u.info.Defs[id], lit.Type, lit.Body)
								}
							}
						}
					}
				case *ast.ValueSpec:
					if len(n.Names) == len(n.Values) {
						for i, id := range n.Names {
							if lit, ok := ast.Unparen(n.Values[i]).(*ast.FuncLit); ok {
								add(u, u.info.Defs[id], lit.Type, lit.Body)
							}
						}
					}
				}
				return true
			})
		}
	}

	for changed := true; changed; {
		changed = false
		for _, tf := range funcs {
			p := &pass{r: r, u: tf.unit, info: tf.unit.info}
			// param returns the position of the parameter e names, or -1.
			param := func(e ast.Expr) int {
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
			mark := func(i int) {
				if i >= 0 && !tf.tokenArgs[i] {
					tf.tokenArgs[i] = true
					changed = true
				}
			}
			ast.Inspect(tf.body, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.AssignStmt:
					for _, l := range n.Lhs {
						if ix, ok := l.(*ast.IndexExpr); ok && isColorMap(p.info.TypeOf(ix.X)) {
							mark(param(ix.Index))
						}
					}
				case *ast.CallExpr:
					if callee := funcs[p.calleeObj(n)]; callee != nil {
						for i := range callee.tokenArgs {
							if i < len(n.Args) {
								mark(param(n.Args[i]))
							}
						}
					}
				}
				return true
			})
		}
	}
	return funcs
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
