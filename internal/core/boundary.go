package core

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Hook boundaries (ADR-085). A hook takes a slot of the component being
// built, so a hook called from a function that is not that component's
// gives its state to the wrong component: Card() called directly inside
// Page keeps its state in Page's slots, and the state of an item that a
// VirtualList renders without tela.CP moves with the visible range. In dev
// mode, the first build of each hook slot walks the stack from the hook to
// the build and accepts these callers:
//
//   - the component's function (Tela's own components included), and
//     function literals defined inside it (closures, also called through
//     helpers, range-over-func bodies);
//   - custom hooks, functions named UseXxx or useXxx, whose own callers
//     must in turn be accepted.
//
// Anything else panics with the fix: wrap the function in tela.C or
// tela.CP, or name it as a custom hook. Frames of Tela's packages (tela, h,
// internal/...) are skipped, except in _test.go files; generic
// instantiations ("F[...]") and method values ("-fm") are compared by
// their function. Outside dev mode nothing is checked, and the stack is
// never captured.

// boundaryDepth is how many return addresses the check captures. A walk
// that does not reach the build within them accepts the hook (no false
// positives on deep stacks).
const boundaryDepth = 32

// boundaryNames caches the boundary function of each component function,
// by code pointer.
var boundaryNames sync.Map // uintptr → string

// boundaryFunc returns the normalized name of the function that owns the
// hooks of a component built from c (normFunc of the component function),
// or "" when c has no function (Providers, Presence).
//
// A struct component (func (c Card) Build() tela.Node; ADR-088) returns
// the name of its Build method, so the hooks called from it are accepted
// (and those of the Build methods of embedded types; ownsHooks).
func (c *compDesc) boundaryFunc() string {
	if c.ct != nil {
		return c.ct.owner
	}
	if c.typ == 0 {
		return ""
	}
	if v, ok := boundaryNames.Load(c.typ); ok {
		return v.(string)
	}
	name := ""
	if f := runtime.FuncForPC(c.typ); f != nil {
		name = normFunc(f.Name())
	}
	boundaryNames.Store(c.typ, name)
	return name
}

// boundaryOK caches accepted stacks: the walk of a hook called again from
// the same place in the same component (the rows of a list) is skipped.
// Guarded by buildMu (checks run during builds); cleared when full.
var boundaryOK = map[boundaryKey]bool{}

type boundaryKey struct {
	typ uintptr
	pcs [boundaryDepth]uintptr
}

const boundaryCacheMax = 4096

// checkBoundary panics when hook slot i of el (of kind) was called from a
// function that neither is el's component function (or a closure in it)
// nor a custom hook (see the comment at the top of the file). pcs is the
// stack from nextSlot's caller up.
func (t *Tree) checkBoundary(el *Element, kind hookKind, pcs []uintptr) {
	if el.comp == nil {
		return
	}
	owner := el.comp.boundaryFunc()
	if owner == "" {
		return
	}
	key := boundaryKey{typ: el.comp.typ}
	copy(key.pcs[:], pcs)
	if boundaryOK[key] {
		return
	}
	frames := runtime.CallersFrames(pcs)
	for {
		f, more := frames.Next()
		switch {
		case f.Function == "":
		case strings.HasPrefix(f.Function, corePkg+".(*Tree)."):
			// The build machinery: everything between it and the hook
			// was accepted.
			more = false
		case frameworkFrame(f.Function, f.File):
		default:
			fn := normFunc(f.Function)
			if el.comp.ownsHooks(fn, owner) {
				more = false
				break
			}
			if isHookName(funcBase(fn)) {
				break // a custom hook: its caller decides
			}
			panic(boundaryMessage(kind, f, fn, owner))
		}
		if !more {
			break
		}
	}
	if len(boundaryOK) >= boundaryCacheMax {
		clear(boundaryOK)
	}
	boundaryOK[key] = true
}

// boundaryMessage explains a hook called from fn (frame f) while owner's
// component builds.
func boundaryMessage(kind hookKind, f runtime.Frame, fn, owner string) string {
	where := fmt.Sprintf("%s:%d", filepath.Base(f.File), f.Line)
	if recv, ok := buildMethod(fn); ok {
		// A struct component whose Build was called directly.
		return fmt.Sprintf("tela: %s at %s is called from %s, which is not the component being built (%s): %s.Build was called directly, so its state would be kept in the hook slots of %s. To fix it, pass %s{...} as a child instead of calling Build, such as Div(%s{...}), and return Fragment(%s{...}) where a node is expected (ARCHITECTURE section 7)",
			kind, where, describeFunc(fn), describeFunc(owner), recv, describeFunc(owner), recv, recv, recv)
	}
	base := funcBase(fn)
	fix := "move its hooks into a component function and render that with tela.C or tela.CP (for the items of a VirtualList: return tela.CP(Row, i) from renderItem)"
	if !isClosure(fn) {
		fix = fmt.Sprintf("wrap %s in a component boundary, tela.C(%s) or tela.CP(%s, props), so that it builds with its own state; if %s is a custom hook, name it Use%s or use%s",
			base, base, base, base, upperFirst(base), upperFirst(base))
	}
	return fmt.Sprintf("tela: %s at %s is called from %s, which is neither the component being built (%s) nor a custom hook (a function named UseXxx or useXxx), so its state would be kept in the hook slots of %s. To fix it, %s (ARCHITECTURE section 7)",
		kind, where, describeFunc(fn), describeFunc(owner), describeFunc(owner), fix)
}

// frameworkFrame reports whether a frame belongs to Tela's own code (the
// packages tela, h and internal/...), outside test files.
func frameworkFrame(fn, file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return false
	}
	rest, ok := strings.CutPrefix(fn, telaPkg)
	if !ok {
		return false
	}
	return strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, "/h.") || strings.HasPrefix(rest, "/internal/")
}

// normFunc normalizes a runtime function name for comparisons: generic
// instantiations lose their shape ("F[...]" → "F") and method values
// their wrapper suffix ("M-fm" → "M").
func normFunc(name string) string {
	name = strings.ReplaceAll(name, "[...]", "")
	return strings.TrimSuffix(name, "-fm")
}

// ownedBy reports whether function fn is owner or a function literal
// defined inside it ("owner.func1", "owner.func1.2", "owner-range1").
func ownedBy(fn, owner string) bool {
	return fn == owner || strings.HasPrefix(fn, owner+".") || strings.HasPrefix(fn, owner+"-")
}

// symbol returns the part of a function name after its package path:
// "Page.func1" for "example.com/app.Page.func1".
func symbol(fn string) string {
	i := strings.LastIndexByte(fn, '/')
	if j := strings.IndexByte(fn[i+1:], '.'); j >= 0 {
		return fn[i+1+j+1:]
	}
	return fn
}

// closurePart reports whether s is a name element the compiler gives to
// function literals: "func1", "1", "deferwrap1", "gowrap1".
func closurePart(s string) bool {
	for _, p := range [...]string{"func", "deferwrap", "gowrap"} {
		if d, ok := strings.CutPrefix(s, p); ok && allDigits(d) {
			return true
		}
	}
	return allDigits(s)
}

// funcParts splits the symbol of fn into its named part ("Page",
// "(*S).Render") and reports whether a function literal follows it.
func funcParts(fn string) (named string, closure bool) {
	sym := symbol(fn)
	if i := strings.Index(sym, "-range"); i >= 0 {
		sym, closure = sym[:i], true
	}
	parts := strings.Split(sym, ".")
	n := len(parts)
	for n > 1 && (closurePart(parts[n-1]) || parts[n-1] == "") {
		n--
		closure = true
	}
	return strings.Join(parts[:n], "."), closure
}

// funcBase returns the name of the named function fn is or is defined in:
// "Page" for "app.Page.func1", "Render" for "app.(*S).Render".
func funcBase(fn string) string {
	named, _ := funcParts(fn)
	if i := strings.LastIndexByte(named, '.'); i >= 0 {
		return named[i+1:]
	}
	return named
}

// isClosure reports whether fn is a function literal.
func isClosure(fn string) bool {
	_, c := funcParts(fn)
	return c
}

// describeFunc names a function for messages: "app.Page", or "a function
// literal in app.Page".
func describeFunc(fn string) string {
	named, closure := funcParts(fn)
	pkg := strings.TrimSuffix(fn[:len(fn)-len(symbol(fn))], ".")
	if i := strings.LastIndexByte(pkg, '/'); i >= 0 {
		pkg = pkg[i+1:]
	}
	if pkg != "" {
		named = pkg + "." + named
	}
	if closure {
		return "a function literal in " + named
	}
	return named
}

// isHookName reports whether a function name is a hook's: Use or use
// followed by an upper-case letter, a digit or nothing (UseTheme,
// useDrag; not User or useful).
func isHookName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Use")
	if !ok {
		rest, ok = strings.CutPrefix(name, "use")
	}
	if !ok {
		return false
	}
	return rest == "" || (rest[0] >= 'A' && rest[0] <= 'Z') || (rest[0] >= '0' && rest[0] <= '9')
}

func upperFirst(s string) string {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

// ---------------------------------------------------------------------
// Hooks outside a build.

// outsideBuildMessage explains a hook called while no component builds.
// The usual cause is a component function called directly where a node
// is expected (telatest.Mount(t, Counter()), a Preview that returns
// Counter()), whose hooks then run before any build: when the hook's
// caller is a named function that no Tela code (an event, an effect)
// called, the message says to wrap it in tela.C.
func outsideBuildMessage(hook string) string {
	generic := "tela: " + hook + " called outside a component build; call hooks at the top level of a component function while it builds (ARCHITECTURE section 7), not in event handlers, effects or goroutines"
	var pcs [boundaryDepth]uintptr
	// 0 is runtime.Callers, 1 outsideBuildMessage, 2 currentElement.
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	var user runtime.Frame
	found := false
	for {
		f, more := frames.Next()
		switch {
		case f.Function == "":
		case frameworkFrame(f.Function, f.File):
			if found {
				// Tela called the function: an event handler, an effect,
				// a posted function.
				return fmt.Sprintf("%s (called from %s at %s:%d)", generic, describeFunc(normFunc(user.Function)), filepath.Base(user.File), user.Line)
			}
		case !found:
			fn := normFunc(f.Function)
			if isHookName(funcBase(fn)) {
				break // a custom hook: look at its caller
			}
			user, found = f, true
			if isClosure(fn) {
				return fmt.Sprintf("%s (called from %s at %s:%d)", generic, describeFunc(fn), filepath.Base(f.File), f.Line)
			}
		}
		if !more {
			break
		}
	}
	if !found {
		return generic
	}
	fn := normFunc(user.Function)
	if recv, ok := buildMethod(fn); ok {
		return fmt.Sprintf("tela: %s called outside a component build, from %s (%s:%d), which ran as a plain method call. Pass the component value itself instead of calling Build, telatest.Mount(t, %s{...}) or Div(%s{...}), so that it builds with its own state. Hooks run only while a component builds (ARCHITECTURE section 7)",
			hook, describeFunc(fn), filepath.Base(user.File), user.Line, recv, recv)
	}
	base := funcBase(fn)
	return fmt.Sprintf("tela: %s called outside a component build, from %s (%s:%d), which ran as a plain function call. If %s is a component, give it a boundary instead of calling it: wrap the root in tela.C(%s) (telatest.Mount(t, tela.C(%s)), tela.App{Root: %s}, func Preview() tela.Node { return tela.C(%s) }) or use tela.CP(%s, props), so that it builds with its own state. Hooks run only while a component builds, at the top level of its function (ARCHITECTURE section 7)",
		hook, describeFunc(fn), filepath.Base(user.File), user.Line, base, base, base, base, base, base)
}
