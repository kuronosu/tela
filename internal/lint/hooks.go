package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kuronosu/tela/internal/core"
)

// The hooks rule: React's rules of hooks as dev mode applies them
// (ADR-085, ADR-088), seen in the source.
//
// A hook is a call to a function named UseXxx of package tela, aria, kit or
// i18n, or to a custom hook: a function or method of the linted code named
// UseXxx or useXxx that calls a hook (so a function that merely has a
// hook's name, such as useTLS, is not one). A hook call is reported when:
//
//   - it is conditional: inside an if or else, a switch or select case, the
//     right side of && or ||;
//   - it is in a loop: a for or range body (the body of a range over a
//     function included);
//   - it comes after an early return: a return inside a conditional
//     statement above it in the same function;
//   - its function is neither a component nor a custom hook: a function that
//     is not named as a hook and has not the shape of a component, a
//     function literal that is not a component either, or no function at
//     all. A component is a method Build, or a function or function literal
//     of the shape func() tela.Node or func(P) tela.Node; a function with
//     that shape that is not a Build method may be a component, and is
//     given the benefit of the doubt. Function literals that are called on
//     the spot are part of the function around them.
//
// A call of a component is reported too: a function component or a Build
// method whose body calls hooks, called directly (Counter(), c.Build())
// instead of being given to tela.C, tela.CP or an element, runs its hooks
// in the state of the caller (ADR-085, ADR-088).
//
// The function literal given to h.Map, h.MapIndex or h.Range runs inside
// the build once per item, so its hooks are in a loop, whatever its shape.

// isHookCall reports whether call calls a hook.
func (p *pass) isHookCall(call *ast.CallExpr) bool {
	fn := p.callee(call)
	if fn == nil {
		return false
	}
	return p.r.hooks[fn] || isTelaHook(fn)
}

// isTelaHook reports whether fn is a hook exported by a Tela package.
func isTelaHook(fn *types.Func) bool {
	if fn.Pkg() == nil || !fn.Exported() || !hookNameFunc(fn) {
		return false
	}
	switch fn.Pkg().Path() {
	case relPkg(""), relPkg("aria"), relPkg("kit"), relPkg("i18n"), relPkg("desktop"):
	default:
		return false
	}
	sig, _ := fn.Type().(*types.Signature)
	return sig != nil && sig.Recv() == nil
}

// funcKind says what a function that contains a hook call is.
type funcKind int

const (
	funcNone      funcKind = iota // no function: a package-level initializer
	funcComponent                 // a component, or a function that may be
	funcHook                      // a custom hook
	funcOther                     // neither
	funcListItem                  // the function given to h.Map and the like
)

// checkHookCall reports the misuse of a hook call.
func (p *pass) checkHookCall(call *ast.CallExpr) {
	if !p.isHookCall(call) {
		return
	}
	name := p.hookName(call)

	fnIdx := p.enclosingFunc()
	if fnIdx < 0 {
		p.report(call.Pos(), RuleHooks, SeverityError, fmt.Sprintf(
			"hook %s is called outside of any function; hooks run only while a component builds, at the top level of its function", name), "")
		return
	}
	fnNode := p.stack[fnIdx]
	kind := p.classify(fnIdx)
	switch kind {
	case funcOther:
		p.report(call.Pos(), RuleHooks, SeverityError, p.otherMessage(name, fnNode), "")
		return
	case funcListItem:
		p.report(call.Pos(), RuleHooks, SeverityError, fmt.Sprintf(
			"hook %s is called in the function given to %s, which runs once per item inside the build, so the hook is called in a loop; make the item a component (a struct with a Build method, or tela.CP(Row, props)) and call the hook there",
			name, p.listHelper(fnIdx)), "")
		return
	}

	if reason := p.flowReason(fnIdx); reason != "" {
		p.report(call.Pos(), RuleHooks, SeverityError, fmt.Sprintf(
			"hook %s is called %s; hooks must be called in the same order on every build: call it at the top level of the function, before any condition, loop or return, and use its result inside them", name, reason), "")
		return
	}
	if ret := p.earlyReturn(fnNode, call.Pos()); ret != token.NoPos {
		line := p.r.l.fset.Position(ret).Line
		p.report(call.Pos(), RuleHooks, SeverityError, fmt.Sprintf(
			"hook %s is called after an early return (line %d); hooks must be called in the same order on every build: call it before the return", name, line), "")
	}
}

// hookName returns how a hook is written in messages: tela.UseState.
func (p *pass) hookName(call *ast.CallExpr) string {
	fn := p.callee(call)
	if fn.Pkg() != nil && fn.Pkg() != p.u.pkg {
		return fn.Pkg().Name() + "." + fn.Name()
	}
	return fn.Name()
}

// enclosingFunc returns the index in the stack of the function that holds
// the node being visited, looking through function literals that are
// called on the spot; -1 when there is none.
func (p *pass) enclosingFunc() int {
	for i := len(p.stack) - 2; i >= 0; i-- {
		switch f := p.stack[i].(type) {
		case *ast.FuncDecl:
			return i
		case *ast.FuncLit:
			if p.calledOnTheSpot(i, f) {
				continue
			}
			return i
		}
	}
	return -1
}

// calledOnTheSpot reports whether the function literal at stack[i] is the
// function of a call (func() { ... }()) that is neither deferred nor a go
// statement.
func (p *pass) calledOnTheSpot(i int, lit *ast.FuncLit) bool {
	if i < 1 {
		return false
	}
	call, ok := p.stack[i-1].(*ast.CallExpr)
	if !ok || ast.Unparen(call.Fun) != ast.Expr(lit) {
		return false
	}
	if i >= 2 {
		switch p.stack[i-2].(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return false
		}
	}
	return true
}

// classify says what the function at stack[i] is.
func (p *pass) classify(i int) funcKind {
	switch f := p.stack[i].(type) {
	case *ast.FuncDecl:
		if core.IsHookName(f.Name.Name) {
			return funcHook
		}
		if fn, ok := p.info.Defs[f.Name].(*types.Func); ok && componentShape(fn.Type()) {
			return funcComponent
		}
		return funcOther
	case *ast.FuncLit:
		if p.assignedToHook(i) {
			return funcHook
		}
		if p.listHelper(i) != "" {
			return funcListItem
		}
		if componentShape(p.info.TypeOf(f)) {
			return funcComponent
		}
		return funcOther
	}
	return funcOther
}

// componentShape reports whether t is func() tela.Node or func(P)
// tela.Node.
func componentShape(t types.Type) bool {
	if t == nil {
		return false
	}
	sig, ok := t.Underlying().(*types.Signature)
	if !ok || sig.Variadic() || sig.Params().Len() > 1 || sig.Results().Len() != 1 {
		return false
	}
	return isCoreType(sig.Results().At(0).Type(), "Node")
}

// assignedToHook reports whether the function literal at stack[i] is the
// value of a variable named as a hook (useDrag := func() ...).
func (p *pass) assignedToHook(i int) bool {
	if i < 1 {
		return false
	}
	lit := p.stack[i]
	switch s := p.stack[i-1].(type) {
	case *ast.AssignStmt:
		for k, r := range s.Rhs {
			if r == lit && k < len(s.Lhs) {
				if id, ok := s.Lhs[k].(*ast.Ident); ok {
					return core.IsHookName(id.Name)
				}
			}
		}
	case *ast.ValueSpec:
		for k, v := range s.Values {
			if v == lit && k < len(s.Names) {
				return core.IsHookName(s.Names[k].Name)
			}
		}
	}
	return false
}

// listHelper returns "h.Map", "h.MapIndex", "h.MapKeyed" or "h.Range" when
// the function literal at stack[i] is an argument of that call, else "".
func (p *pass) listHelper(i int) string {
	if i < 1 {
		return ""
	}
	call, ok := p.stack[i-1].(*ast.CallExpr)
	if !ok {
		return ""
	}
	for _, name := range [...]string{"Map", "MapIndex", "MapKeyed", "Range"} {
		if p.isFunc(p.callee(call), "h", name) {
			for _, a := range call.Args {
				if a == p.stack[i] {
					return "h." + name
				}
			}
		}
	}
	return ""
}

// otherMessage explains a hook called from a function that is not a
// component or a hook.
func (p *pass) otherMessage(hook string, fn ast.Node) string {
	if f, ok := fn.(*ast.FuncDecl); ok {
		what := "function " + f.Name.Name
		if f.Recv != nil {
			what = "method " + f.Name.Name
		}
		return fmt.Sprintf(
			"hook %s is called from %s, which is neither a component (a Build method, func() tela.Node or func(P) tela.Node) nor a custom hook (a function named UseXxx or useXxx), so its state would be kept in the hook slots of whatever component calls it; if it is a hook, rename it to use%s, and if it is a component, give it a component's signature and render it with tela.C or tela.CP",
			hook, what, upperFirst(f.Name.Name))
	}
	return fmt.Sprintf(
		"hook %s is called inside a function literal that is neither a component (func() tela.Node or func(P) tela.Node) nor a custom hook, which may run outside the build (a handler, an effect, a callback); hooks run only while a component builds, at the top level of its function: call the hook there and use its result in the function literal", hook)
}

func upperFirst(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// flowReason says why the hook call being visited is not unconditional in
// the function at stack[fnIdx], or "".
func (p *pass) flowReason(fnIdx int) string {
	for i := len(p.stack) - 2; i > fnIdx; i-- {
		child := p.stack[i+1]
		switch a := p.stack[i].(type) {
		case *ast.IfStmt:
			if child == ast.Node(a.Body) || (a.Else != nil && child == ast.Node(a.Else)) {
				return "conditionally (inside an if)"
			}
		case *ast.CaseClause:
			return "conditionally (inside a switch case)"
		case *ast.CommClause:
			return "conditionally (inside a select case)"
		case *ast.ForStmt:
			if child == ast.Node(a.Body) || (a.Cond != nil && child == ast.Node(a.Cond)) || (a.Post != nil && child == ast.Node(a.Post)) {
				return "in a loop"
			}
		case *ast.RangeStmt:
			if child == ast.Node(a.Body) {
				return "in a loop"
			}
		case *ast.BinaryExpr:
			if (a.Op == token.LAND || a.Op == token.LOR) && child == ast.Node(a.Y) {
				return "conditionally (after " + a.Op.String() + ")"
			}
		}
	}
	return ""
}

// earlyReturn returns the position of a return statement above pos in
// function fn that sits inside a conditional or loop statement which ends
// before pos, or NoPos.
func (p *pass) earlyReturn(fn ast.Node, pos token.Pos) token.Pos {
	var body *ast.BlockStmt
	switch f := fn.(type) {
	case *ast.FuncDecl:
		body = f.Body
	case *ast.FuncLit:
		body = f.Body
	}
	if body == nil {
		return token.NoPos
	}
	found := token.NoPos
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if found != token.NoPos || n.Pos() >= pos {
			return false
		}
		if _, ok := n.(*ast.FuncLit); ok && n != fn {
			return false
		}
		stack = append(stack, n)
		if r, ok := n.(*ast.ReturnStmt); ok {
			for _, a := range stack {
				switch a.(type) {
				case *ast.IfStmt, *ast.CaseClause, *ast.CommClause, *ast.ForStmt, *ast.RangeStmt:
					if a.End() <= pos {
						found = r.Pos()
					}
				}
			}
		}
		return true
	})
	return found
}

// checkComponentCall reports a direct call of a function component or a
// Build method that uses hooks.
func (p *pass) checkComponentCall(call *ast.CallExpr, fn *types.Func) {
	d := p.r.decls[fn]
	if d == nil || core.IsHookName(fn.Name()) || !componentShape(fn.Type()) || !p.r.usesHooks(fn, d) {
		return
	}
	sig := fn.Type().(*types.Signature)
	if sig.Recv() != nil {
		recv := namedOf(sig.Recv().Type())
		if fn.Name() != "Build" || recv == nil || sig.Params().Len() != 0 {
			return
		}
		name := recv.Obj().Name()
		p.report(call.Pos(), RuleHooks, SeverityError, fmt.Sprintf(
			"%s.Build uses hooks and is called directly, so its state would be kept in the hook slots of the component that calls it; pass %s{...} as a child instead of calling Build, such as Div(%s{...}), and return Fragment(%s{...}) where a node is expected", name, name, name, name), "")
		return
	}
	p.report(call.Pos(), RuleHooks, SeverityError, fmt.Sprintf(
		"%s uses hooks, so it is a component, and calling it directly would keep its state in the hook slots of the component that calls it; give it a boundary instead: tela.C(%s), or tela.CP(%s, props) when it takes props", fn.Name(), fn.Name(), fn.Name()), "")
}

// usesHooks reports whether the body of fn, not counting the function
// literals in it, calls a hook.
func (r *run) usesHooks(fn *types.Func, d *funcDecl) bool {
	if v, ok := r.usesHooksCache[fn]; ok {
		return v
	}
	p := &pass{r: r, u: d.unit, info: d.unit.info}
	found := false
	ast.Inspect(d.decl.Body, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.CallExpr:
			if p.isHookCall(n) {
				found = true
			}
		}
		return !found
	})
	if r.usesHooksCache == nil {
		r.usesHooksCache = map[*types.Func]bool{}
	}
	r.usesHooksCache[fn] = found
	return found
}
