package lint

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// modulePath is the import path of the Tela module. Calls and types are
// identified by it and a name, through go/types, so renamed and dot imports
// are seen through and a different package with the same names is not
// mistaken for Tela's.
const modulePath = "github.com/kuronosu/tela"

// pass walks the files of one unit and runs the rules on what it finds.
type pass struct {
	r    *run
	u    *unit
	info *types.Info

	// stack holds the ancestors of the node being visited, the node last.
	stack []ast.Node
}

// checkAll runs the rules on every unit, and returns the internal errors
// (a panic while linting a file is reported, not allowed to stop the run).
func (r *run) checkAll() (errs []string) {
	for _, u := range r.units {
		p := &pass{r: r, u: u, info: u.info}
		for _, f := range u.files {
			func() {
				defer func() {
					if v := recover(); v != nil {
						errs = append(errs, fmt.Sprintf("%s: internal error: %v", r.rel(r.l.fset.Position(f.Package).Filename), v))
					}
				}()
				p.stack = p.stack[:0]
				ast.Inspect(f, func(n ast.Node) bool {
					if n == nil {
						p.stack = p.stack[:len(p.stack)-1]
						return false
					}
					p.stack = append(p.stack, n)
					switch n := n.(type) {
					case *ast.CallExpr:
						p.checkCall(n)
					case *ast.CompositeLit:
						p.checkComposite(n)
					case *ast.AssignStmt:
						p.checkAssign(n)
					}
					return true
				})
			}()
		}
	}
	r.checkAliasValues()
	return errs
}

// report records a finding at pos, unless a //tela:ignore comment covers
// it.
func (p *pass) report(pos token.Pos, rule, severity, msg, suggestion string) {
	position := p.r.l.fset.Position(pos)
	if p.r.ignored(position.Filename, position.Line, rule) {
		return
	}
	msg = strings.TrimPrefix(msg, "tela: ")
	if suggestion == "" {
		suggestion = suggestionOf(msg)
	}
	p.r.diags = append(p.r.diags, Diagnostic{
		File:       p.r.rel(position.Filename),
		Line:       position.Line,
		Col:        position.Column,
		Rule:       rule,
		Severity:   severity,
		Message:    msg,
		Suggestion: suggestion,
	})
}

// callee returns the function a call calls, when it is a declared function
// or method (not a function value).
func (p *pass) callee(call *ast.CallExpr) *types.Func {
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
	fn, _ := p.info.Uses[id].(*types.Func)
	if fn == nil {
		return nil
	}
	return fn.Origin()
}

// relPkg returns the path of the package at rel ("" for the root) in the
// Tela module.
func relPkg(rel string) string {
	if rel == "" {
		return modulePath
	}
	return modulePath + "/" + rel
}

// isFunc reports whether fn is the package-level function name of the Tela
// package at rel.
func (p *pass) isFunc(fn *types.Func, rel, name string) bool {
	if fn == nil || fn.Pkg() == nil || fn.Name() != name || fn.Pkg().Path() != relPkg(rel) {
		return false
	}
	sig, _ := fn.Type().(*types.Signature)
	return sig != nil && sig.Recv() == nil
}

// namedOf returns the named type of t, looking through aliases and one
// pointer.
func namedOf(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		t = types.Unalias(ptr.Elem())
	}
	n, _ := t.(*types.Named)
	return n
}

// isTelaType reports whether t is the type name of the Tela package at rel.
func (p *pass) isTelaType(t types.Type, rel, name string) bool {
	n := namedOf(t)
	if n == nil {
		return false
	}
	o := n.Obj()
	return o.Pkg() != nil && o.Name() == name && o.Pkg().Path() == relPkg(rel)
}

// isCoreType reports whether t is the type name of internal/core, where
// tela.Node, tela.Prop and tela.Child are defined.
func isCoreType(t types.Type, name string) bool {
	n := namedOf(t)
	if n == nil {
		return false
	}
	o := n.Obj()
	return o.Pkg() != nil && o.Name() == name && o.Pkg().Path() == relPkg("internal/core")
}

// checkCall runs the rules that start at a call.
func (p *pass) checkCall(call *ast.CallExpr) {
	fn := p.callee(call)
	if fn == nil {
		return
	}
	p.checkClassCall(call, fn)
	p.checkPropCall(call, fn)
	p.checkKeysCall(call, fn)
	p.checkHookCall(call)
	p.checkComponentCall(call, fn)
}
