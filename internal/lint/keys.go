package lint

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
)

// The key rule, the static form of the two warnings dev mode gives about
// keys (internal/core element.go, checkKeys):
//
//   - "duplicate key": two siblings given to one element have the same
//     constant key (h.Key, the key argument of tela.C, tela.CP and
//     tela.Memo, tela.WithKey). The runtime compares the keys of the
//     children of one list, whatever their types.
//   - "no key": the items of a Map or MapIndex have no key. The runtime
//     warns when two or more items of the list have none; statically, the
//     callback must return, on every path, an element (h.Div...) that has
//     no Key among its arguments and nothing in them that could be one.
//     Anything else (a component, a helper's result, an argument that is
//     a computed prop) is not reported: it may be keyed.
//
// Both are warnings in dev mode, so they have the severity "warning".

// checkKeysCall runs the key rule on a call.
func (p *pass) checkKeysCall(call *ast.CallExpr, fn *types.Func) {
	if p.isElementFunc(fn) {
		p.checkDuplicateKeys(call)
	}
	if p.isFunc(fn, "h", "Map") || p.isFunc(fn, "h", "MapIndex") {
		p.checkMapKeys(call)
	}
}

// isElementFunc reports whether fn is a function of package h that takes
// children: h.Div, h.Button, h.Fragment...
func (p *pass) isElementFunc(fn *types.Func) bool {
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != relPkg("h") {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() != nil || !sig.Variadic() || sig.Params().Len() == 0 {
		return false
	}
	last := sig.Params().At(sig.Params().Len() - 1).Type()
	sl, ok := last.(*types.Slice)
	return ok && isCoreType(sl.Elem(), "Child")
}

// constKey is a constant key and where it was written.
type constKey struct {
	id  string // type and value
	val string // the value for messages
	pos token.Pos
}

// checkDuplicateKeys reports the children of an element call that share a
// constant key.
func (p *pass) checkDuplicateKeys(call *ast.CallExpr) {
	if call.Ellipsis.IsValid() {
		return
	}
	seen := map[string]constKey{}
	for _, arg := range call.Args {
		k, ok := p.childKey(arg)
		if !ok {
			continue
		}
		if first, dup := seen[k.id]; dup {
			pos := p.r.l.fset.Position(first.pos)
			p.report(k.pos, RuleKey, SeverityWarning, fmt.Sprintf(
				"duplicate key %s among the children of this element (first at %s:%d:%d); keys must be unique among siblings, or the duplicates are remounted on every build",
				k.val, p.r.rel(pos.Filename), pos.Line, pos.Column), "")
			continue
		}
		seen[k.id] = k
	}
}

// childKey returns the constant key that the node written as arg carries:
// an element or fragment with an h.Key argument, tela.C, tela.CP or
// tela.Memo with one as their key argument, or tela.WithKey.
func (p *pass) childKey(arg ast.Expr) (constKey, bool) {
	call, ok := ast.Unparen(arg).(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() {
		return constKey{}, false
	}
	fn := p.callee(call)
	switch {
	case fn == nil:
		return constKey{}, false
	case p.isFunc(fn, "", "WithKey") && len(call.Args) == 2:
		return p.constKeyOf(call.Args[1])
	case p.isElementFunc(fn), p.isFunc(fn, "", "C"), p.isFunc(fn, "", "CP"), p.isFunc(fn, "", "Memo"):
		var found constKey
		var ok bool
		for _, a := range call.Args {
			if kc, isCall := ast.Unparen(a).(*ast.CallExpr); isCall && p.isFunc(p.callee(kc), "h", "Key") && len(kc.Args) == 1 {
				found, ok = p.constKeyOf(kc.Args[0]) // the last Key wins
			}
		}
		return found, ok
	}
	return constKey{}, false
}

// constKeyOf returns the key written as the constant expression e.
func (p *pass) constKeyOf(e ast.Expr) (constKey, bool) {
	tv, ok := p.info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() == constant.Unknown {
		return constKey{}, false
	}
	t := types.Default(tv.Type)
	return constKey{
		id:  types.TypeString(t, nil) + "\x00" + tv.Value.ExactString(),
		val: tv.Value.String(),
		pos: e.Pos(),
	}, true
}

// checkMapKeys reports a Map or MapIndex whose callback certainly returns
// unkeyed elements.
func (p *pass) checkMapKeys(call *ast.CallExpr) {
	if len(call.Args) != 2 {
		return
	}
	if cl, ok := ast.Unparen(call.Args[0]).(*ast.CompositeLit); ok && len(cl.Elts) < 2 {
		return // dev mode warns about two or more items
	}
	body, bp := p.funcBody(call.Args[1])
	if body == nil {
		return
	}
	returns := collectReturns(body)
	if len(returns) == 0 {
		return
	}
	for _, ret := range returns {
		if len(ret.Results) != 1 || !bp.certainlyUnkeyed(ret.Results[0]) {
			return
		}
	}
	p.report(call.Pos(), RuleKey, SeverityWarning,
		"the items of this list have no key: the function given to Map returns elements without a Key; give each item a Key (its ID, not its index) so that its state follows it when the list changes", "")
}

// funcBody returns the body of a function literal, or of a linted function
// named by e, with the pass that has its types.
func (p *pass) funcBody(e ast.Expr) (*ast.BlockStmt, *pass) {
	switch f := ast.Unparen(e).(type) {
	case *ast.FuncLit:
		return f.Body, p
	case *ast.Ident, *ast.SelectorExpr:
		var id *ast.Ident
		if x, ok := f.(*ast.Ident); ok {
			id = x
		} else {
			id = f.(*ast.SelectorExpr).Sel
		}
		if fn, ok := p.info.Uses[id].(*types.Func); ok {
			if d := p.r.decls[fn.Origin()]; d != nil {
				return d.decl.Body, &pass{r: p.r, u: d.unit, info: d.unit.info}
			}
		}
	}
	return nil, nil
}

// collectReturns returns the return statements of a function body, not
// those of the function literals inside it.
func collectReturns(body *ast.BlockStmt) []*ast.ReturnStmt {
	var out []*ast.ReturnStmt
	ast.Inspect(body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			out = append(out, n)
		}
		return true
	})
	return out
}

// certainlyUnkeyed reports whether e is an element of package h written
// with no argument that is or could be a Key.
func (p *pass) certainlyUnkeyed(e ast.Expr) bool {
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok || call.Ellipsis.IsValid() {
		return false
	}
	if fn := p.callee(call); fn == nil || !p.isElementFunc(fn) {
		return false
	}
	for _, a := range call.Args {
		if p.mayBeKey(a) {
			return false
		}
	}
	return true
}

// mayBeKey reports whether argument a of an element could be its Key.
func (p *pass) mayBeKey(a ast.Expr) bool {
	t := p.info.TypeOf(a)
	switch {
	case isCoreType(t, "Node"):
		return false // a child
	case isCoreType(t, "Prop"), isCoreType(t, "Child"):
	default:
		return false // a style prop or a component value: never a Key
	}
	call, ok := ast.Unparen(a).(*ast.CallExpr)
	if !ok {
		return true // a variable of type Prop or Child
	}
	fn := p.callee(call)
	if fn == nil || fn.Pkg() == nil || (fn.Pkg().Path() != relPkg("h") && fn.Pkg().Path() != relPkg("")) {
		return true // a helper of the linted code: it may return a Key
	}
	switch fn.Name() {
	case "Key", "If", "IfElse":
		return true
	}
	// The other props of h and tela (Class, OnClick, TestID...) are not
	// keys; a function of h that returns a Child with a Key inside is
	// not known.
	sig, _ := fn.Type().(*types.Signature)
	return sig == nil || sig.Results().Len() != 1 || !isCoreType(sig.Results().At(0).Type(), "Prop")
}
