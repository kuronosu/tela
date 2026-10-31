package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// The fragment and prop-node rules (ADR-205).
//
// fragment: a call of h.Fragment with exactly one argument, which is not a
// spread, is redundant since a struct component is a tela.Node: the
// argument can be written where the call is. A finding is reported only
// when its fix (tela lint -fix) is known to mean the same and to
// type-check, so every finding is fixable. Not reported:
//
//   - The argument is a Prop, a StyleProp, or a tela.Child that could hold
//     one (anything but a call of h.If or h.IfElse with arguments that are
//     not props): a Fragment takes Key as a prop, so Fragment(h.Key(k)) is
//     not a wrapper. A literal nil is not either (a Fragment of a hole is
//     a Fragment).
//   - The call is not where the argument's type is accepted. It must be an
//     argument or an element of a composite literal, a return value or an
//     assignment to a declared type, and the type expected there tela.Node
//     or tela.Child, not a type parameter. A short variable declaration
//     (n := Fragment(x)) would change the type of the variable, and so
//     would a selector, a comparison or an argument of type any.
//   - Its identity would change. A node directly in a parent's children
//     and the same node in a Fragment have a different identity (the
//     reconciler tells them by the scope of the fragment), so removing the
//     wrap would remount a stateful node when the same place holds both
//     forms at different times. That happens in a function that returns
//     Fragment(A{}) on one path and Fragment(A{}, B{}) on another (or
//     keeps them in one variable, a variable that is returned being the
//     same place as the results), or in IfElse(c, Fragment(A{}),
//     Fragment(A{}, B{})): the single call is a variant of a group, and it
//     is kept. Also kept: the first argument of tela.WithKey, unless it is
//     a composite literal, because the key goes to the fragment (its
//     children get keyPair{key, own key}) and to the child otherwise,
//     which overrides the key the child already has.
//   - A comment is inside the call, outside its argument: the fix would
//     delete it.
//
// prop-node: a Prop or StyleProp (h.Class, h.Style, h.Key, an event
// handler...) where a tela.Node is expected, for example the result of a
// function that returns a tela.Node, the result of a callback given to
// h.Map, an element of a []tela.Node, an argument of tela.WithKey. A Prop
// has a Build method, so it compiles, and it panics where the node is
// consumed. A tela.Child, which takes props, is not a tela.Node here.

const fragmentMessage = "redundant Fragment: a struct component is a tela.Node, write the node itself"

// checkFragment runs the fragment rule on a call of h.Fragment.
func (p *pass) checkFragment(call *ast.CallExpr, fn *types.Func) {
	if !p.isFunc(fn, "h", "Fragment") || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return
	}
	arg := call.Args[0]
	if isNilIdent(arg) || p.maybeProp(arg) {
		return
	}
	argType := p.info.TypeOf(arg)
	if argType == nil {
		return
	}
	parent, ok := p.parentOf(call)
	if ok && p.variantOfGroup(call, parent) {
		return
	}
	var edits []edit
	if ok && p.fragmentFixable(call, arg, argType, parent) {
		edits = p.removalEdits(call, arg)
	}
	if edits == nil {
		return
	}
	p.reportFix(call.Pos(), RuleFragment, SeverityWarning, fragmentMessage, "", edits)
}

func isNilIdent(e ast.Expr) bool {
	id, ok := ast.Unparen(e).(*ast.Ident)
	return ok && id.Name == "nil"
}

// maybeProp reports whether the Child e may be a Prop or a StyleProp: its
// type is one, or it is a tela.Child that is not provably free of them.
func (p *pass) maybeProp(e ast.Expr) bool {
	t := p.info.TypeOf(e)
	if t == nil {
		return false
	}
	if isCoreType(t, "Prop") || isCoreType(t, "StyleProp") {
		return true
	}
	if !isCoreType(t, "Child") {
		return false
	}
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok {
		return true
	}
	fn := p.callee(call)
	if !p.isFunc(fn, "h", "If") && !p.isFunc(fn, "h", "IfElse") {
		return true
	}
	for _, a := range call.Args[1:] { // the first is the condition
		if !isNilIdent(a) && p.maybeProp(a) {
			return true
		}
	}
	return false
}

// parentOf returns the parent of the node being visited (the top of the
// stack, which must be n), skipping parentheses.
func (p *pass) parentOf(n ast.Node) (ast.Node, bool) {
	i := len(p.stack) - 1
	if i < 0 || p.stack[i] != n {
		return nil, false
	}
	for i--; i >= 0; i-- {
		if _, paren := p.stack[i].(*ast.ParenExpr); !paren {
			return p.stack[i], true
		}
	}
	return nil, false
}

// fragmentFixable reports whether arg, of type argType, can take the place
// of call, whose parent is parent, and still compile.
func (p *pass) fragmentFixable(call *ast.CallExpr, arg ast.Expr, argType types.Type, parent ast.Node) bool {
	want, ok := p.expectedType(call, parent)
	if !ok || !(isCoreType(want, "Node") || isCoreType(want, "Child")) {
		return false
	}
	if _, isTP := types.Unalias(want).(*types.TypeParam); isTP || !types.AssignableTo(argType, want) {
		return false
	}
	if c, ok := parent.(*ast.CallExpr); ok && p.isFunc(p.callee(c), "", "WithKey") && len(c.Args) > 0 && c.Args[0] == ast.Expr(call) {
		if _, lit := ast.Unparen(arg).(*ast.CompositeLit); !lit {
			return false
		}
	}
	// expectedType only knows the places where the call is a whole
	// operand (an argument, an element, a result, a right-hand side), so
	// an argument such as &c needs no parentheses.
	return true
}

// expectedType returns the type that the context parent gives to the
// expression e, which is a child of parent: the type of the parameter, the
// result, the field or the element, or the declared type of the variable.
// It is not ok where the context does not say (a short variable
// declaration, a selector, a conversion, a generic parameter...).
func (p *pass) expectedType(e ast.Expr, parent ast.Node) (types.Type, bool) {
	switch par := parent.(type) {
	case *ast.ReturnStmt:
		i := indexOfExpr(par.Results, e)
		sig := p.enclosingSignature()
		if i < 0 || sig == nil || sig.Results().Len() != len(par.Results) {
			return nil, false
		}
		return sig.Results().At(i).Type(), true
	case *ast.CallExpr:
		i := indexOfExpr(par.Args, e)
		if i < 0 || par.Ellipsis.IsValid() {
			return nil, false
		}
		if id, ok := ast.Unparen(par.Fun).(*ast.Ident); ok {
			if bi, isBuiltin := p.info.Uses[id].(*types.Builtin); isBuiltin {
				// The recorded signature of a builtin is made of the
				// arguments' own types; only append says what it takes.
				if bi.Name() != "append" || i < 1 {
					return nil, false
				}
				sl, _ := p.info.TypeOf(par.Args[0]).Underlying().(*types.Slice)
				if sl == nil {
					return nil, false
				}
				return sl.Elem(), true
			}
		}
		tv, ok := p.info.Types[par.Fun]
		if !ok || tv.IsType() {
			return nil, false
		}
		sig, _ := tv.Type.Underlying().(*types.Signature)
		if sig == nil {
			return nil, false
		}
		t := paramType(sig, i)
		if t == nil {
			return nil, false
		}
		// The parameter of a generic function is a type parameter before
		// the call instantiates it: the argument decides it.
		if fn := p.callee(par); fn != nil {
			if osig, _ := fn.Type().(*types.Signature); osig != nil {
				if ot := paramType(osig, i); ot == nil || hasTypeParam(ot) {
					return nil, false
				}
			}
		}
		return t, true
	case *ast.CompositeLit:
		return p.elementType(par, e)
	case *ast.KeyValueExpr:
		if par.Value != e {
			return nil, false
		}
		lit, ok := p.grandparent(par).(*ast.CompositeLit)
		if !ok {
			return nil, false
		}
		return p.elementType(lit, par)
	case *ast.AssignStmt:
		if par.Tok != token.ASSIGN || len(par.Lhs) != len(par.Rhs) {
			return nil, false
		}
		i := indexOfExpr(par.Rhs, e)
		if i < 0 {
			return nil, false
		}
		if id, ok := par.Lhs[i].(*ast.Ident); ok && id.Name == "_" {
			return nil, false
		}
		t := p.info.TypeOf(par.Lhs[i])
		return t, t != nil
	case *ast.ValueSpec:
		if par.Type == nil || indexOfExpr(par.Values, e) < 0 {
			return nil, false
		}
		t := p.info.TypeOf(par.Type)
		return t, t != nil
	}
	return nil, false
}

func indexOfExpr(list []ast.Expr, e ast.Expr) int {
	for i, x := range list {
		if ast.Unparen(x) == e {
			return i
		}
	}
	return -1
}

// paramType returns the type that argument i of a call of sig is converted
// to, or nil.
func paramType(sig *types.Signature, i int) types.Type {
	params := sig.Params()
	n := params.Len()
	switch {
	case sig.Variadic() && i >= n-1:
		sl, _ := params.At(n - 1).Type().(*types.Slice)
		if sl == nil {
			return nil
		}
		return sl.Elem()
	case i < n:
		return params.At(i).Type()
	}
	return nil
}

// hasTypeParam reports whether t is, or is made of, a type parameter.
func hasTypeParam(t types.Type) bool {
	switch t := types.Unalias(t).(type) {
	case *types.TypeParam:
		return true
	case *types.Slice:
		return hasTypeParam(t.Elem())
	case *types.Array:
		return hasTypeParam(t.Elem())
	case *types.Pointer:
		return hasTypeParam(t.Elem())
	case *types.Map:
		return hasTypeParam(t.Key()) || hasTypeParam(t.Elem())
	case *types.Chan:
		return hasTypeParam(t.Elem())
	case *types.Signature:
		for _, tup := range []*types.Tuple{t.Params(), t.Results()} {
			for i := 0; i < tup.Len(); i++ {
				if hasTypeParam(tup.At(i).Type()) {
					return true
				}
			}
		}
	case *types.Named:
		if t.TypeArgs().Len() > 0 {
			for i := 0; i < t.TypeArgs().Len(); i++ {
				if hasTypeParam(t.TypeArgs().At(i)) {
					return true
				}
			}
		}
	}
	return false
}

// grandparent returns the parent of n (n being the parent of the node being
// visited).
func (p *pass) grandparent(n ast.Node) ast.Node {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i] == n {
			for j := i - 1; j >= 0; j-- {
				if _, paren := p.stack[j].(*ast.ParenExpr); !paren {
					return p.stack[j]
				}
			}
		}
	}
	return nil
}

// elementType returns the type of the element of lit that holds e: e itself
// when it is an element, or the value of the key-value pair e.
func (p *pass) elementType(lit *ast.CompositeLit, e ast.Expr) (types.Type, bool) {
	t := p.info.TypeOf(lit)
	if t == nil {
		return nil, false
	}
	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		t = ptr.Elem()
	}
	switch u := t.Underlying().(type) {
	case *types.Slice:
		return u.Elem(), true
	case *types.Array:
		return u.Elem(), true
	case *types.Map:
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			return u.Elem(), kv.Value != nil
		}
		return nil, false
	case *types.Struct:
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			id, _ := kv.Key.(*ast.Ident)
			if id == nil {
				return nil, false
			}
			for i := 0; i < u.NumFields(); i++ {
				if u.Field(i).Name() == id.Name {
					return u.Field(i).Type(), true
				}
			}
			return nil, false
		}
		if i := indexOfExpr(lit.Elts, e); i >= 0 && i < u.NumFields() {
			return u.Field(i).Type(), true
		}
	}
	return nil, false
}

// enclosingSignature returns the signature of the function the visited node
// is in.
func (p *pass) enclosingSignature() *types.Signature {
	for i := len(p.stack) - 1; i >= 0; i-- {
		switch f := p.stack[i].(type) {
		case *ast.FuncLit:
			sig, _ := p.info.TypeOf(f).(*types.Signature)
			return sig
		case *ast.FuncDecl:
			if obj, ok := p.info.Defs[f.Name].(*types.Func); ok {
				sig, _ := obj.Type().(*types.Signature)
				return sig
			}
			return nil
		}
	}
	return nil
}

// variantOfGroup reports whether the single-argument Fragment call is one of
// several forms that the same place takes at different times, and another
// of them is a group: an IfElse with a fragment of another shape in the
// other branch, or fragments of other shapes that the same function returns
// or keeps in the same variable. Removing the wrap from the single form
// would give its child another identity (see the rule's comment). When the
// place is not known to be apart from a group, the call is a variant.
func (p *pass) variantOfGroup(call *ast.CallExpr, parent ast.Node) bool {
	switch par := parent.(type) {
	case *ast.CallExpr:
		if p.isFunc(p.callee(par), "h", "IfElse") {
			for _, a := range par.Args {
				if c, ok := ast.Unparen(a).(*ast.CallExpr); ok && c != call && p.otherShapeFragment(c) {
					return true
				}
			}
		}
	case *ast.ReturnStmt, *ast.AssignStmt, *ast.ValueSpec:
		return p.variantInFunction(call)
	}
	return false
}

// A fragmentSource is an expression that goes to a place: the results of a
// function (the key is the function), a variable (the key is its object) or
// another operand (the key is its text).
type fragmentSource struct {
	expr ast.Expr
	key  any
}

// variantInFunction reports whether a fragment of another shape goes to the
// same place as call, which is a result or is assigned to a variable. The
// place of a result is the results of its function; the place of a variable
// is the variable. A variable that a function returns, or that is copied to
// another, is the same place as it (and its function's other results), so a
// group assigned to it in another branch, declared with it, or returned
// next to it is found. The scan goes through the whole declaration that
// holds call, closures included, so a variable that a closure sets is the
// variable the function returns.
func (p *pass) variantInFunction(call *ast.CallExpr) bool {
	root, ok := p.outermostFunc()
	if !ok {
		return true // a package-level variable: any function may set it
	}
	var (
		sources []fragmentSource
		parents = map[any]any{} // union-find over the keys
	)
	var find func(k any) any
	find = func(k any) any {
		if par, ok := parents[k]; ok && par != k {
			r := find(par)
			parents[k] = r
			return r
		}
		return k
	}
	union := func(a, b any) {
		if a != nil && b != nil {
			parents[find(a)] = find(b)
		}
	}
	// keyOf returns the key of the variable or operand e is assigned to.
	keyOf := func(e ast.Expr) any {
		e = ast.Unparen(e)
		if id, ok := e.(*ast.Ident); ok {
			if id.Name == "_" {
				return nil
			}
			if obj := p.info.ObjectOf(id); obj != nil {
				return obj
			}
		}
		return types.ExprString(e)
	}
	// valueKey returns the key of the variable that the expression e
	// copies, if it is one.
	valueKey := func(e ast.Expr) any {
		if id, ok := ast.Unparen(e).(*ast.Ident); ok {
			if v, ok := p.info.ObjectOf(id).(*types.Var); ok {
				return v
			}
		}
		return nil
	}
	assign := func(lhs, rhs ast.Expr) {
		k := keyOf(lhs)
		if k == nil {
			return
		}
		sources = append(sources, fragmentSource{rhs, k})
		union(k, valueKey(rhs))
	}
	var walk func(body ast.Node, fn ast.Node)
	walk = func(body ast.Node, fn ast.Node) {
		ast.Inspect(body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncLit:
				p.joinNamedResults(n, n.Type, union)
				walk(n.Body, n)
				return false
			case *ast.ReturnStmt:
				for _, r := range n.Results {
					sources = append(sources, fragmentSource{r, fn})
					union(fn, valueKey(r))
				}
			case *ast.AssignStmt:
				if len(n.Lhs) == len(n.Rhs) {
					for i, l := range n.Lhs {
						assign(l, n.Rhs[i])
					}
				}
			case *ast.ValueSpec:
				if len(n.Names) == len(n.Values) {
					for i, name := range n.Names {
						assign(name, n.Values[i])
					}
				}
			}
			return true
		})
	}
	switch root := root.(type) {
	case *ast.FuncDecl:
		p.joinNamedResults(root, root.Type, union)
		walk(root.Body, root)
	case *ast.FuncLit:
		p.joinNamedResults(root, root.Type, union)
		walk(root.Body, root)
	}
	var key any
	for _, s := range sources {
		if ast.Unparen(s.expr) == ast.Expr(call) {
			key = s.key
			break
		}
	}
	if key == nil {
		return false
	}
	// A variable of the package, set anywhere, is a place that this
	// function does not show.
	if v, ok := key.(*types.Var); ok && v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
		return true
	}
	place := find(key)
	for _, s := range sources {
		if c, ok := ast.Unparen(s.expr).(*ast.CallExpr); ok && c != call && find(s.key) == place && p.otherShapeFragment(c) {
			return true
		}
	}
	return false
}

// joinNamedResults joins the named results of the function fn, whose type
// is ft, with the function (a bare return returns them).
func (p *pass) joinNamedResults(fn ast.Node, ft *ast.FuncType, union func(a, b any)) {
	if ft.Results == nil {
		return
	}
	for _, f := range ft.Results.List {
		for _, name := range f.Names {
			if obj := p.info.Defs[name]; obj != nil {
				union(obj, fn)
			}
		}
	}
}

// outermostFunc returns the function declaration that holds the visited
// node or, in a package-level declaration, its outermost function literal.
func (p *pass) outermostFunc() (ast.Node, bool) {
	for _, n := range p.stack {
		switch n.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return n, true
		}
	}
	return nil, false
}

// otherShapeFragment reports whether c is a call of h.Fragment that is not
// a single-argument wrap (a group, a spread or an empty one).
func (p *pass) otherShapeFragment(c *ast.CallExpr) bool {
	if !p.isFunc(p.callee(c), "h", "Fragment") {
		return false
	}
	return len(c.Args) != 1 || c.Ellipsis.IsValid()
}

// removalEdits returns the edits that replace call by its argument, or nil
// when a comment would be lost or the positions cannot be resolved.
func (p *pass) removalEdits(call *ast.CallExpr, arg ast.Expr) []edit {
	fset := p.r.l.fset
	start := fset.PositionFor(call.Pos(), false)
	argStart := fset.PositionFor(arg.Pos(), false)
	argEnd := fset.PositionFor(arg.End(), false)
	end := fset.PositionFor(call.End(), false)
	if !start.IsValid() || start.Filename != end.Filename || start.Filename != argStart.Filename {
		return nil
	}
	if f := p.r.fileAST[start.Filename]; f != nil {
		for _, cg := range f.Comments {
			inHead := cg.End() > call.Pos() && cg.Pos() < arg.Pos()
			inTail := cg.End() > arg.End() && cg.Pos() < call.End()
			if inHead || inTail {
				return nil
			}
		}
	}
	return []edit{
		{file: start.Filename, start: start.Offset, end: argStart.Offset},
		{file: start.Filename, start: argEnd.Offset, end: end.Offset},
	}
}

// checkPropNode runs the prop-node rule on the expression e, the node being
// visited.
func (p *pass) checkPropNode(e ast.Expr) {
	t := p.info.TypeOf(e)
	if t == nil {
		return
	}
	name, ok := p.propName(e, t)
	if !ok {
		return
	}
	parent, ok := p.parentOf(e)
	if !ok {
		return
	}
	// A call that is the Fun of its parent is not a value in a node place.
	if c, isCall := parent.(*ast.CallExpr); isCall && ast.Unparen(c.Fun) == e {
		return
	}
	want, ok := p.expectedType(e, parent)
	if !ok || !isCoreType(want, "Node") {
		return
	}
	p.report(e.Pos(), RulePropNode, SeverityError, fmt.Sprintf(
		"%s is a Prop, not a node: it compiles where a tela.Node is expected (a Prop has a Build method) but panics when the node is consumed; props go to an element's arguments, such as Div(Class(...), Text(...))", name), "")
}

// propName returns how a message names e when e is a Prop or a StyleProp,
// or an If or IfElse of them.
func (p *pass) propName(e ast.Expr, t types.Type) (string, bool) {
	call, isCall := ast.Unparen(e).(*ast.CallExpr)
	if isCoreType(t, "Prop") || isCoreType(t, "StyleProp") {
		if isCall {
			if fn := p.callee(call); fn != nil && fn.Pkg() != nil {
				return fn.Pkg().Name() + "." + fn.Name() + "(...)", true
			}
		}
		return "this value", true
	}
	if isCall && isCoreType(t, "Child") {
		fn := p.callee(call)
		if !p.isFunc(fn, "h", "If") && !p.isFunc(fn, "h", "IfElse") {
			return "", false
		}
		props := 0
		for _, a := range call.Args[1:] { // the first is the condition
			if isNilIdent(a) {
				continue
			}
			at := p.info.TypeOf(a)
			if at == nil {
				return "", false
			}
			if _, ok := p.propName(a, at); !ok {
				return "", false
			}
			props++
		}
		if props > 0 {
			return fn.Pkg().Name() + "." + fn.Name() + "(...) of props", true
		}
	}
	return "", false
}
