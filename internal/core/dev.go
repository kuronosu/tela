package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// Dev-mode diagnostics of hooks and components (ARCHITECTURE section 3:
// errors that teach): the hook-order check with call sites, dependency
// warnings and anonymous functions given to ComponentProps or Memo.

// devAny reports whether dev mode may be on in some Tree of the process
// (TELA_DEV=1, or a Tree created with Config.Dev). Node constructors,
// which do not know their Tree, record call sites only then.
var devAny atomic.Bool

func init() {
	if os.Getenv(EnvDev) == "1" {
		devAny.Store(true)
	}
}

var (
	// corePkg and telaPkg are the import paths of this package and of
	// the public package tela, whose frames call sites skip.
	corePkg = reflect.TypeFor[Node]().PkgPath()
	telaPkg = strings.TrimSuffix(corePkg, "/internal/core")
)

// callStack is a few return PCs captured with runtime.Callers; resolved
// to a call site only when a message needs it.
type callStack [8]uintptr

// capture records the stack above the caller of capture's caller: skip
// 0 is the function that calls capture's caller.
func (s *callStack) capture(skip int) {
	runtime.Callers(skip+3, s[:])
}

// site returns the first frame of s that is user code: not a hook or
// node constructor of this package and not a wrapper in package tela.
// It reads "pkg.Func (file.go:12)".
func (s *callStack) site() string {
	n := 0
	for n < len(s) && s[n] != 0 {
		n++
	}
	if n == 0 {
		return "an unknown location"
	}
	frames := runtime.CallersFrames(s[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && !apiFrame(f.Function) {
			fn := f.Function
			if i := strings.LastIndexByte(fn, '/'); i >= 0 {
				fn = fn[i+1:]
			}
			return fmt.Sprintf("%s (%s:%d)", fn, filepath.Base(f.File), f.Line)
		}
		if !more {
			return "an unknown location"
		}
	}
}

// apiFrame reports whether fn is a frame between the user's call and the
// capture: a wrapper of package tela, or a hook or node constructor of
// core (Use..., use..., ComponentProps, Memo).
func apiFrame(fn string) bool {
	if rest, ok := strings.CutPrefix(fn, telaPkg+"."); ok {
		return !strings.Contains(rest, "/")
	}
	if rest, ok := strings.CutPrefix(fn, corePkg+"."); ok {
		return strings.HasPrefix(rest, "Use") || strings.HasPrefix(rest, "use") ||
			strings.HasPrefix(rest, "ComponentProps") || strings.HasPrefix(rest, "Memo") ||
			strings.HasPrefix(rest, "noteAnon") || strings.HasPrefix(rest, "nextSlot")
	}
	return false
}

// ---------------------------------------------------------------------
// Anonymous component functions.

// anonSite is an anonymous function given to ComponentProps or Memo, and
// where.
type anonSite struct {
	api   string // "CP" or "Memo"
	name  string // the function's runtime name
	stack callStack
}

// anonNames caches, per code pointer, the runtime name of anonymous
// functions and method values ("" for named functions).
var anonNames sync.Map

// noteAnon returns the call site of a ComponentProps or Memo whose
// function (code pointer pc) is anonymous, or nil.
func noteAnon(pc uintptr, api string) *anonSite {
	var name string
	if v, ok := anonNames.Load(pc); ok {
		name = v.(string)
	} else {
		if f := runtime.FuncForPC(pc); f != nil && isAnonymous(f.Name()) {
			name = f.Name()
		}
		anonNames.Store(pc, name)
	}
	if name == "" {
		return nil
	}
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	a := &anonSite{api: api, name: name}
	a.stack.capture(0)
	return a
}

// isAnonymous reports whether a runtime function name is a closure
// ("pkg.Outer.func1", "pkg.Outer.func1.2", "pkg.init.func1") or a method
// value ("pkg.T.M-fm"), whose captured variables or receiver are not part
// of the props.
func isAnonymous(name string) bool {
	if strings.HasSuffix(name, "-fm") {
		return true
	}
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	parts := strings.Split(name, ".")
	for _, s := range parts[1:] {
		if s == "" {
			continue
		}
		if d, ok := strings.CutPrefix(s, "func"); ok && d != "" && allDigits(d) {
			return true
		}
		if allDigits(s) {
			return true
		}
	}
	return false
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// reportAnon warns, once per call site, about a component node made from
// an anonymous function or a method value.
func (t *Tree) reportAnon(c *compDesc) {
	a := c.anon
	site := a.stack.site()
	if !t.firstReport("anon:" + site) {
		return
	}
	what := "an anonymous function (" + a.name + "): the variables it captures are not compared with the props, so the component skips its build while they change"
	if strings.HasSuffix(a.name, "-fm") {
		what = "a method value (" + a.name + "): its receiver is not compared with the props, so the component skips its build while the receiver changes"
	}
	t.logf("tela: %s(...) at %s gets %s. Make the component a named function and pass everything it uses in its props, or use tela.C, which never skips because of props",
		a.api, site, what)
}

// firstReport reports whether key is reported for the first time in this
// tree, recording it.
func (t *Tree) firstReport(key string) bool {
	if t.reported[key] {
		return false
	}
	t.reported[key] = true
	return true
}

// ---------------------------------------------------------------------
// Hook order.

// hookKind is the kind of a hook slot, for the dev-mode order check.
type hookKind uint8

const (
	hookState hookKind = iota + 1 // UseState, UseStateSet, UseStateUpdate
	hookReducer
	hookEffect
	hookLayoutEffect
	hookMemo
	hookCallback
	hookRef
	hookContext
	hookID
	hookValue
	hookElementRef
	hookScroll
)

var hookNames = [...]string{
	hookState:        "UseState",
	hookReducer:      "UseReducer",
	hookEffect:       "UseEffect",
	hookLayoutEffect: "UseLayoutEffect",
	hookMemo:         "UseMemo",
	hookCallback:     "UseCallback",
	hookRef:          "UseRef",
	hookContext:      "UseContext",
	hookID:           "UseId",
	hookValue:        "UseValue",
	hookElementRef:   "UseElementRef",
	hookScroll:       "UseScroll",
}

func (k hookKind) String() string { return hookNames[k] }

// hookRecord is what dev mode remembers of a hook slot from the build that
// created it: its kind and where it was called.
type hookRecord struct {
	kind  hookKind
	stack callStack
	where string // stack.site(), resolved on first use
}

// orderAdvice ends every hook-order panic.
const orderAdvice = "Hooks must run in the same order on every build: call them at the top level of the component, never inside conditions, loops or after an early return. Move the conditional part into its own component with tela.C and render that conditionally (h.If)"

// checkHook is the dev-mode order check of hook slot i of el, called
// with the kind the build calls now. The first build records the kind
// and the call site; later builds panic with both call sites when the
// kind differs or the hook is new.
func (t *Tree) checkHook(el *Element, i int, kind hookKind) {
	if i < len(el.hookDev) {
		if r := &el.hookDev[i]; r.kind != kind {
			var now callStack
			now.capture(0)
			panic(fmt.Sprintf("tela: the hook order of %s changed: hook %d is %s at %s in this build, but it was %s at %s when the component first built. %s",
				t.describe(el), i+1, kind, now.site(), r.kind, el.hookSite(i), orderAdvice))
		}
		return
	}
	var r hookRecord
	r.kind = kind
	r.stack.capture(0)
	if el.builds > 1 {
		panic(fmt.Sprintf("tela: %s called %s at %s, a hook its previous builds did not call (they called %d hooks). %s",
			t.describe(el), kind, r.stack.site(), len(el.hookDev), orderAdvice))
	}
	el.hookDev = append(el.hookDev, r)
}

// hookSite returns where hook slot i of el was first called, in dev mode.
func (el *Element) hookSite(i int) string {
	if i >= len(el.hookDev) {
		return "an unknown location"
	}
	r := &el.hookDev[i]
	if r.where == "" {
		r.where = r.stack.site()
	}
	return r.where
}

// hookCountPanic reports a build of el that called fewer or more hooks
// (now) than its previous build (before).
func (t *Tree) hookCountPanic(el *Element, now, before int) {
	if t.dev && now < before && now < len(el.hookDev) {
		r := &el.hookDev[now]
		panic(fmt.Sprintf("tela: %s called %d hooks in this build and %d in the previous one: %s at %s was not called. %s",
			t.describe(el), now, before, r.kind, el.hookSite(now), orderAdvice))
	}
	panic(fmt.Sprintf("tela: %s called %d hooks in this build and %d in the previous one; call hooks at the top level of the component, never inside conditions or loops (or move the conditional part into its own component with tela.C)",
		t.describe(el), now, before))
}

// slotMismatch panics because hook slot i of el holds another type of
// hook than the one called now (want, such as "UseState[int]").
func (t *Tree) slotMismatch(el *Element, i int, want string) {
	where := ""
	if t.dev && i < len(el.hookDev) {
		var now callStack
		now.capture(0)
		where = fmt.Sprintf(" (called at %s; the first build called %s at %s)", now.site(), el.hookDev[i].kind, el.hookSite(i))
	}
	panic(fmt.Sprintf("tela: hook %d of %s was %s in the previous build and is %s now%s; hooks must be called in the same order on every build (never inside conditions or loops)",
		i+1, t.describe(el), slotName(el.hooks[i]), want, where))
}

// slotName names the hook that owns a slot value, for messages:
// "UseState[int]".
func slotName(s any) string {
	if e, ok := s.(*effectSlot); ok {
		if e.layout {
			return "UseLayoutEffect"
		}
		return "UseEffect"
	}
	name := strings.TrimPrefix(fmt.Sprintf("%T", s), "*core.")
	for _, r := range [...][2]string{
		{"stateSlot", "UseState"}, {"reducerSlot", "UseReducer"}, {"memoSlot", "UseMemo"},
		{"cbSlot", "UseCallback"}, {"Ref", "UseRef"}, {"ctxSlot", "UseContext"}, {"idSlot", "UseId"},
		{"Value", "UseValue"}, {"elemRef", "UseElementRef"},
	} {
		if rest, ok := strings.CutPrefix(name, r[0]); ok {
			return r[1] + rest
		}
	}
	return name
}

// ---------------------------------------------------------------------
// Dependencies.

// depsChanged compares the dependencies of hook slot i of el with those
// of its previous build: no dependencies means changed (run on every
// build); a different count is changed (reported in dev mode); otherwise
// the values are compared with ==. A value that is not comparable (a
// func, a slice, a map, or a struct or interface holding one) is changed
// and reported once per call site in dev mode. It never panics.
func (t *Tree) depsChanged(el *Element, i int, old, deps []any) bool {
	if len(old) != len(deps) {
		if t.dev && t.firstReport("deps-count:"+el.hookSite(i)) {
			t.logf("tela: %s at %s got %d dependencies in this build and %d in the previous one, so it counts as changed; pass the same dependencies on every build (none runs it on every build, a single nil only on mount)",
				el.hookDev[i].kind, el.hookSite(i), len(deps), len(old))
		}
		return true
	}
	if len(deps) == 0 {
		return true
	}
	changed := false
	for k, d := range deps {
		if ty := reflect.TypeOf(d); ty != nil && !ty.Comparable() {
			t.reportDep(el, i, k, ty)
			changed = true
			continue
		}
		eq, ok := cmpEqual(old[k], d)
		if !ok {
			t.reportDep(el, i, k, reflect.TypeOf(d))
			changed = true
		} else if !eq {
			changed = true
		}
	}
	return changed
}

// checkDeps reports, in dev mode, the dependencies of a new hook slot
// that are not comparable.
func (t *Tree) checkDeps(el *Element, i int, deps []any) {
	if !t.dev {
		return
	}
	for k, d := range deps {
		if ty := reflect.TypeOf(d); ty != nil && !ty.Comparable() {
			t.reportDep(el, i, k, ty)
		}
	}
}

// reportDep warns, once per call site, about dependency k of hook slot i
// of el, which cannot be compared.
func (t *Tree) reportDep(el *Element, i, k int, ty reflect.Type) {
	if !t.dev || !t.firstReport("dep:"+el.hookSite(i)) {
		return
	}
	t.logf("tela: dependency %d of %s at %s is a %v, which cannot be compared with ==, so it counts as changed on every build. Use tela.UseCallback for a function, and an ID or a comparable value (a string, a number, a struct of those) for the rest",
		k+1, el.hookDev[i].kind, el.hookSite(i), ty)
}

// cmpEqual compares a and b with ==; ok is false when == panicked (their
// dynamic type is not comparable).
func cmpEqual(a, b any) (eq, ok bool) {
	defer func() {
		if recover() != nil {
			eq, ok = false, false
		}
	}()
	return a == b, true
}
