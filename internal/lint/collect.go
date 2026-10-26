package lint

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/style"
)

// aliasEntry is a class alias found in the linted code: a key of a
// ClassAliases map.
type aliasEntry struct {
	name     string
	value    string
	hasValue bool // value is a constant
	key      ast.Expr
	valueExp ast.Expr
	unit     *unit
}

// colorEntry is a theme color name found in the linted code.
type colorEntry struct {
	name string
	expr ast.Expr
	unit *unit
}

// collect makes the first pass over every linted package: the
// declarations, the custom hooks, and the class aliases and theme colors
// that the class parser must know.
func (r *run) collect() {
	r.decls = map[*types.Func]*funcDecl{}
	r.varInit = map[types.Object]ast.Expr{}
	r.hooks = map[*types.Func]bool{}
	var aliases []*aliasEntry
	var colors []*colorEntry

	for _, u := range r.units {
		for _, f := range u.files {
			for _, d := range f.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					if fn, ok := u.info.Defs[d.Name].(*types.Func); ok && d.Body != nil {
						r.decls[fn] = &funcDecl{d, u}
					}
				case *ast.GenDecl:
					for _, s := range d.Specs {
						if vs, ok := s.(*ast.ValueSpec); ok && len(vs.Names) == len(vs.Values) {
							for i, n := range vs.Names {
								if obj := u.info.Defs[n]; obj != nil {
									r.varInit[obj] = vs.Values[i]
								}
							}
						}
					}
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				if as, ok := n.(*ast.AssignStmt); ok && as.Tok == token.DEFINE && len(as.Lhs) == len(as.Rhs) {
					for i, l := range as.Lhs {
						if id, ok := l.(*ast.Ident); ok {
							if obj := u.info.Defs[id]; obj != nil {
								r.varInit[obj] = as.Rhs[i]
							}
						}
					}
				}
				return true
			})
		}
	}

	r.findHooks()

	for _, u := range r.units {
		p := &pass{r: r, u: u, info: u.info}
		for _, f := range u.files {
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.CompositeLit:
					switch {
					case p.isTelaType(p.info.TypeOf(n), "", "App"):
						for _, el := range n.Elts {
							if kv, ok := el.(*ast.KeyValueExpr); ok && isIdent(kv.Key, "ClassAliases") {
								aliases = append(aliases, p.aliasEntries(kv.Value)...)
							}
						}
					case isColorMap(p.info.TypeOf(n)):
						colors = append(colors, p.colorEntries(n)...)
					}
				case *ast.AssignStmt:
					// theme.Colors["brand"] = ... and app.ClassAliases = ...
					for i, l := range n.Lhs {
						switch l := l.(type) {
						case *ast.IndexExpr:
							if isColorMap(p.info.TypeOf(l.X)) {
								if name, ok := p.constString(l.Index); ok {
									colors = append(colors, &colorEntry{name, l.Index, u})
								}
							}
						case *ast.SelectorExpr:
							if l.Sel.Name == "ClassAliases" && p.isTelaType(p.info.TypeOf(l.X), "", "App") && len(n.Lhs) == len(n.Rhs) {
								aliases = append(aliases, p.aliasEntries(n.Rhs[i])...)
							}
						}
					}
				case *ast.CallExpr:
					if fn := p.callee(n); fn != nil && p.isFunc(fn, "telatest", "ClassAliases") && len(n.Args) == 1 {
						aliases = append(aliases, p.aliasEntries(n.Args[0])...)
					}
				}
				return true
			})
		}
	}

	r.aliasList = aliases
	r.buildParser(aliases, colors)
}

// findHooks computes r.hooks: Tela's exported hooks, then the functions of
// the linted code named as hooks that call a hook, until nothing changes.
func (r *run) findHooks() {
	for {
		changed := false
		for fn, d := range r.decls {
			if r.hooks[fn] || !hookNameFunc(fn) {
				continue
			}
			p := &pass{r: r, u: d.unit, info: d.unit.info}
			found := false
			ast.Inspect(d.decl.Body, func(n ast.Node) bool {
				if found {
					return false
				}
				if call, ok := n.(*ast.CallExpr); ok && p.isHookCall(call) {
					found = true
				}
				return !found
			})
			if found {
				r.hooks[fn] = true
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

// buildParser makes the class parser: the default theme plus the colors
// found, and the aliases found. Names the runtime would reject are
// reported and left out.
func (r *run) buildParser(aliases []*aliasEntry, colors []*colorEntry) {
	th := style.DefaultTheme()
	seen := map[string]bool{}
	for _, c := range colors {
		if seen[c.name] {
			continue
		}
		seen[c.name] = true
		probe := style.DefaultTheme()
		probe.Tokens[c.name] = style.SchemeColor{}
		if err := probe.CheckTokens(); err != nil {
			p := &pass{r: r, u: c.unit, info: c.unit.info}
			p.report(c.expr.Pos(), RuleClass, SeverityError, "tela: "+err.Error()+" (App.Theme)", "")
			continue
		}
		th.Tokens[c.name] = style.SchemeColor{}
	}
	valid := map[string]string{}
	for _, a := range aliases {
		if err := style.CheckAliases(map[string]string{a.name: ""}); err != nil {
			p := &pass{r: r, u: a.unit, info: a.unit.info}
			p.report(a.key.Pos(), RuleClass, SeverityError, err.Error(), "")
			continue
		}
		valid[a.name] = a.value
	}
	p, err := style.NewParserAliases(th, valid)
	if err != nil {
		p = style.NewParser(th)
	}
	r.parser = p
}

// aliasEntries returns the aliases of a ClassAliases value: a map literal,
// or a variable initialized with one.
func (p *pass) aliasEntries(e ast.Expr) []*aliasEntry {
	cl := p.mapLit(e)
	if cl == nil {
		return nil
	}
	var out []*aliasEntry
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := p.constString(kv.Key)
		if !ok {
			continue
		}
		a := &aliasEntry{name: name, key: kv.Key, valueExp: kv.Value, unit: p.u}
		a.value, a.hasValue = p.constString(kv.Value)
		out = append(out, a)
	}
	return out
}

// isColorMap reports whether t is a map[string]tela.SchemeColor: the type
// of Theme.Colors, and of the maps a program builds its colors in.
func isColorMap(t types.Type) bool {
	if t == nil {
		return false
	}
	m, ok := types.Unalias(t).Underlying().(*types.Map)
	if !ok {
		return false
	}
	if b, ok := m.Key().Underlying().(*types.Basic); !ok || b.Kind() != types.String {
		return false
	}
	n := namedOf(m.Elem())
	return n != nil && n.Obj().Name() == "SchemeColor" && n.Obj().Pkg() != nil &&
		strings.HasPrefix(n.Obj().Pkg().Path(), modulePath)
}

// colorEntries returns the color names of a map literal of colors.
func (p *pass) colorEntries(cl *ast.CompositeLit) []*colorEntry {
	var out []*colorEntry
	for _, el := range cl.Elts {
		if kv, ok := el.(*ast.KeyValueExpr); ok {
			if name, ok := p.constString(kv.Key); ok {
				out = append(out, &colorEntry{name, kv.Key, p.u})
			}
		}
	}
	return out
}

// mapLit returns the map literal e is, or the one the variable e names was
// initialized with.
func (p *pass) mapLit(e ast.Expr) *ast.CompositeLit {
	for range 4 {
		switch x := ast.Unparen(e).(type) {
		case *ast.CompositeLit:
			return x
		case *ast.Ident:
			init, ok := p.r.varInit[p.info.Uses[x]]
			if !ok {
				return nil
			}
			e = init
		case *ast.SelectorExpr:
			init, ok := p.r.varInit[p.info.Uses[x.Sel]]
			if !ok {
				return nil
			}
			e = init
		default:
			return nil
		}
	}
	return nil
}

// constString returns the value of a constant string expression.
func (p *pass) constString(e ast.Expr) (string, bool) {
	tv, ok := p.info.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
		return "", false
	}
	return constant.StringVal(tv.Value), true
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

// hookNameFunc reports whether fn is named as a hook.
func hookNameFunc(fn *types.Func) bool {
	n := fn.Name()
	return core.IsHookName(n) && n != "Use" && n != "use"
}
