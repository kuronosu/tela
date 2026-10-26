package lint

import (
	"go/ast"
	"go/types"

	"github.com/kuronosu/tela/internal/core"
)

// The role, aria and prop rules: a constant argument that dev mode would
// reject with a panic. Each calls the pure check the prop itself calls
// (internal/core validate.go), so the message is the one dev mode prints.
//
//	role  h.Role
//	aria  h.AriaCurrent, h.AriaSort, h.AriaHasPopup, h.AriaOrientation,
//	      h.AriaAutoComplete (core.AriaTokenProps)
//	prop  h.Dir; h.Type, and the Type field of kit.Button and kit.Input,
//	      with the element's own set of types; tela.Ease

func (p *pass) checkPropCall(call *ast.CallExpr, fn *types.Func) {
	if fn.Pkg() == nil {
		return
	}
	switch fn.Pkg().Path() {
	case relPkg("h"):
	case relPkg(""):
		if p.isFunc(fn, "", "Ease") {
			p.checkArg(call, 0, RuleProp, core.CheckEase)
		}
		return
	default:
		return
	}
	switch name := fn.Name(); name {
	case "Role":
		p.checkArg(call, 0, RuleRole, core.CheckRole)
	case "Dir":
		p.checkArg(call, 0, RuleProp, core.CheckDir)
	case "Type":
		if !p.isFunc(fn, "h", name) {
			return
		}
		element := p.typeParent(call)
		p.checkArg(call, 0, RuleProp, func(s string) string { return core.CheckFieldType(element, s) })
	default:
		for _, prop := range core.AriaTokenProps() {
			if name == prop && p.isFunc(fn, "h", name) {
				p.checkArg(call, 0, RuleAria, func(s string) string { return core.CheckAriaToken(prop, s) })
			}
		}
	}
}

// checkArg runs check on the constant argument i of call and reports its
// message.
func (p *pass) checkArg(call *ast.CallExpr, i int, rule string, check func(string) string) {
	if len(call.Args) <= i || call.Ellipsis.IsValid() {
		return
	}
	s, ok := p.constString(call.Args[i])
	if !ok {
		return
	}
	if msg := check(s); msg != "" {
		p.report(call.Args[i].Pos(), rule, SeverityError, msg, "")
	}
}

// typeParent returns "input" or "button" when call, an h.Type, is an
// argument of h.Input or h.Button, else "".
func (p *pass) typeParent(call *ast.CallExpr) string {
	if len(p.stack) < 2 {
		return ""
	}
	parent, ok := p.stack[len(p.stack)-2].(*ast.CallExpr)
	if !ok {
		return ""
	}
	fn := p.callee(parent)
	switch {
	case p.isFunc(fn, "h", "Input"):
		return "input"
	case p.isFunc(fn, "h", "Button"):
		return "button"
	}
	return ""
}

// checkFieldValue checks the literal-validated fields of the kit
// components: kit.Button.Type and kit.Input.Type.
func (p *pass) checkFieldValue(field *types.Var, owner *types.Named, val ast.Expr) {
	field = field.Origin()
	if field.Name() != "Type" || field.Pkg() == nil || field.Pkg().Path() != relPkg("kit") || owner.Obj().Pkg() == nil {
		return
	}
	var element string
	switch owner.Obj().Name() {
	case "Button":
		element = "button"
	case "Input":
		element = "input"
	default:
		return
	}
	s, ok := p.constString(val)
	if !ok || s == "" { // empty leaves the element's default
		return
	}
	if msg := core.CheckFieldType(element, s); msg != "" {
		p.report(val.Pos(), RuleProp, SeverityError, msg, "")
	}
}
