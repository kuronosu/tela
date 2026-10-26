//go:build teladev

package core

import (
	"fmt"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Identities of the dev state (ADR-113): what makes an Element of the
// next build the same as one of the previous build, and a hook slot the
// same slot.
//
// An Element's step is its identity among its siblings: its type (a
// primitive's tag, a component's identity) and its key, or without one
// its position (the scope and index that reconcile matches by). Its path
// is the steps from the root. A component's identity is:
//
//   - a struct component: "S:" and its type's full name, with its package
//     path ("S:example.com/app.Card");
//   - a function component: "F:" and the function's name from the runtime,
//     without the closure numbers ("F:example.com/app.Main.func" for
//     example.com/app.Main.func3), generic shapes and method value
//     suffixes: the compiler numbers the function literals of a function
//     in source order, so adding one before another would renumber it;
//     what tells literals of one function apart is their position in the
//     tree and their hook signature, as for an edited component;
//   - a Provider: "X:" and its context's type; a Presence: "Presence".
//
// A slot's signature entry is its hook's kind and its value's full type
// ("UseState[int]", "UseReducer[example.com/app.State,example.com/app.Action]"):
// the kind recorded in dev mode (hookRecord), or the kind of its storage
// otherwise (UseSpring and UseTween, UseColorScheme and UseTheme share
// one).

// devIdentAlias maps component identities to others: a test-only seam
// that makes two functions or types of one test binary the same component
// of two builds, since Go cannot give two functions one name.
var devIdentAlias atomic.Pointer[map[string]string]

// devCompIdents caches the identities of function components by code
// pointer.
var devCompIdents sync.Map // uintptr → string

// devCompIdent returns the identity of a component (see the comment at the
// top of the file).
func devCompIdent(c *compDesc) string {
	var id string
	switch {
	case c.ctx != nil:
		id = "X:" + devTypeString(reflect.TypeOf(c.ctx))
	case c.presence:
		id = "Presence"
	case c.ct != nil:
		id = "S:" + c.ct.name
	default:
		id = devFuncIdent(c.typ)
	}
	if m := devIdentAlias.Load(); m != nil {
		if a, ok := (*m)[id]; ok {
			return a
		}
	}
	return id
}

// devFuncIdent returns the identity of the function component whose code
// pointer is pc.
func devFuncIdent(pc uintptr) string {
	if v, ok := devCompIdents.Load(pc); ok {
		return v.(string)
	}
	id := "F:?"
	if f := runtime.FuncForPC(pc); f != nil {
		name := normFunc(f.Name())
		named, closure := funcParts(name)
		pkg := name[:len(name)-len(symbol(name))]
		id = "F:" + pkg + named
		if closure {
			id += ".func"
		}
	}
	devCompIdents.Store(pc, id)
	return id
}

// devStepParts returns the identity of el among its siblings: its type and
// its key or position.
func devStepParts(el *Element) (ident, id string) {
	switch el.kind {
	case elemPrimitive:
		ident = "P:" + el.box.kind.String()
		if el.box.portal {
			ident += "/portal"
		}
	case elemComponent:
		ident = devCompIdent(el.comp)
	default:
		ident = "T"
	}
	if el.keyed {
		return ident, "k:" + devKeyString(el.key)
	}
	b := make([]byte, 0, 12)
	b = strconv.AppendInt(b, int64(el.scope), 10)
	b = append(b, '/')
	b = strconv.AppendInt(b, int64(el.sub), 10)
	return ident, string(b)
}

// devStep returns devStepParts as one string.
func devStep(el *Element) string {
	ident, id := devStepParts(el)
	return ident + "@" + id
}

// devPath returns the path of el from the root, for keys of messages.
func devPath(el *Element) string {
	var parts []string
	for e := el; e != nil && e.parent != nil; e = e.parent {
		parts = append(parts, devStep(e))
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

// devKeyString returns a key as a string that is the same in every build
// for the same key: its full type and its Go syntax. A pointer key differs
// between processes, so its Element never keeps its state.
func devKeyString(k any) string {
	switch k := k.(type) {
	case nil:
		return "nil"
	case keyPair:
		return "(" + devKeyString(k.outer) + "," + devKeyString(k.inner) + ")"
	case string:
		return strconv.Quote(k)
	case int:
		return strconv.Itoa(k)
	}
	return devTypeString(reflect.TypeOf(k)) + "=" + fmt.Sprintf("%#v", k)
}

// devTypeStrings caches devTypeString.
var devTypeStrings sync.Map // reflect.Type → string

// devTypeString returns the full name of a type: named types with their
// package path ("example.com/app.Item", "[]example.com/app.Item"), so
// that two packages' types of one name differ.
func devTypeString(rt reflect.Type) string {
	if rt == nil {
		return "nil"
	}
	if v, ok := devTypeStrings.Load(rt); ok {
		return v.(string)
	}
	s := devTypeName(rt)
	devTypeStrings.Store(rt, s)
	return s
}

func devTypeName(rt reflect.Type) string {
	if rt.Name() != "" {
		if rt.PkgPath() != "" {
			return rt.PkgPath() + "." + rt.Name()
		}
		return rt.Name()
	}
	switch rt.Kind() {
	case reflect.Pointer:
		return "*" + devTypeString(rt.Elem())
	case reflect.Slice:
		return "[]" + devTypeString(rt.Elem())
	case reflect.Array:
		return "[" + strconv.Itoa(rt.Len()) + "]" + devTypeString(rt.Elem())
	case reflect.Map:
		return "map[" + devTypeString(rt.Key()) + "]" + devTypeString(rt.Elem())
	case reflect.Chan:
		switch rt.ChanDir() {
		case reflect.RecvDir:
			return "<-chan " + devTypeString(rt.Elem())
		case reflect.SendDir:
			return "chan<- " + devTypeString(rt.Elem())
		}
		return "chan " + devTypeString(rt.Elem())
	case reflect.Struct:
		var b strings.Builder
		b.WriteString("struct{")
		for i := range rt.NumField() {
			f := rt.Field(i)
			if i > 0 {
				b.WriteString("; ")
			}
			if f.Anonymous {
				b.WriteString("embed ")
			}
			b.WriteString(f.Name)
			b.WriteByte(' ')
			b.WriteString(devTypeString(f.Type))
			if f.Tag != "" {
				b.WriteString(" " + strconv.Quote(string(f.Tag)))
			}
		}
		b.WriteString("}")
		return b.String()
	case reflect.Func:
		var b strings.Builder
		b.WriteString("func(")
		for i := range rt.NumIn() {
			if i > 0 {
				b.WriteString(", ")
			}
			if rt.IsVariadic() && i == rt.NumIn()-1 {
				b.WriteString("..." + devTypeString(rt.In(i).Elem()))
				continue
			}
			b.WriteString(devTypeString(rt.In(i)))
		}
		b.WriteString(")")
		if rt.NumOut() > 0 {
			b.WriteString(" (")
			for i := range rt.NumOut() {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(devTypeString(rt.Out(i)))
			}
			b.WriteString(")")
		}
		return b.String()
	}
	return rt.String() // interfaces: their method set
}

// ---------------------------------------------------------------------
// Signatures.

// devTyped is a hook slot whose hook is generic: it reports its storage
// kind and its value type.
type devTyped interface {
	devSlot() (hookKind, string)
}

func (s *stateSlot[T]) devSlot() (hookKind, string) {
	return hookState, devTypeString(reflect.TypeFor[T]())
}
func (s *reducerSlot[S, A]) devSlot() (hookKind, string) {
	return hookReducer, devTypeString(reflect.TypeFor[S]()) + "," + devTypeString(reflect.TypeFor[A]())
}
func (v *Value[T]) devSlot() (hookKind, string) {
	return hookValue, devTypeString(reflect.TypeFor[T]())
}
func (r *Ref[T]) devSlot() (hookKind, string) { return hookRef, devTypeString(reflect.TypeFor[T]()) }
func (s *memoSlot[T]) devSlot() (hookKind, string) {
	return hookMemo, devTypeString(reflect.TypeFor[T]())
}
func (s *onceSlot[T]) devSlot() (hookKind, string) {
	return hookOnce, devTypeString(reflect.TypeFor[T]())
}
func (s *cbSlot[F]) devSlot() (hookKind, string) {
	return hookCallback, devTypeString(reflect.TypeFor[F]())
}
func (s *ctxSlot[T]) devSlot() (hookKind, string) {
	return hookContext, devTypeString(reflect.TypeFor[T]())
}
func (s *motionSlot[T]) devSlot() (hookKind, string) {
	return hookSpring, devTypeString(reflect.TypeFor[T]())
}

// devMotionSlot is the slot of UseSpring or UseTween.
type devMotionSlot interface{ devMotion() }

func (s *motionSlot[T]) devMotion() {}

// devSlotKind returns the kind of hook slot j of el: the one dev mode
// recorded, else the kind of the slot's storage.
func devSlotKind(el *Element, j int) hookKind {
	if j < len(el.hookDev) {
		return el.hookDev[j].kind
	}
	switch h := el.hooks[j].(type) {
	case devTyped:
		k, _ := h.devSlot()
		return k
	case *effectSlot:
		return h.kind
	case *intervalSlot:
		if h.byEffect {
			return hookTimeoutAfter
		}
		if h.once {
			return hookTimeout
		}
		return hookInterval
	case *presenceSlot:
		return hookPresence
	case *idSlot:
		return hookID
	case *elemRef:
		return hookElementRef
	case *scrollFollow:
		return hookScroll
	case *vlist:
		return hookVirtual
	case *boundsFollow:
		return hookBounds
	case *sizeFollow:
		return hookSize
	case *schemeSlot:
		return hookColorScheme
	case *dirSlot:
		return hookDirection
	case nil:
		return hookReducedMotion
	}
	return 0
}

// devSlotTypeName returns the value type of a slot ("" for hooks that are
// not generic).
func devSlotTypeName(h any) string {
	if s, ok := h.(devTyped); ok {
		_, typ := s.devSlot()
		return typ
	}
	return ""
}

// devSlotSig returns the signature entry of hook slot j of el.
func devSlotSig(el *Element, j int) string {
	k := devSlotKind(el, j)
	name := "?"
	if int(k) < len(hookNames) && k > 0 {
		name = hookNames[k]
	}
	if typ := devSlotTypeName(el.hooks[j]); typ != "" {
		return name + "[" + typ + "]"
	}
	return name
}

// devCompName names a component for reports and warnings: a struct
// component's or a named function's short name ("Counter"), or "a
// function literal in app.Main".
func devCompName(c *compDesc) string {
	if c.ct != nil {
		return c.ct.short
	}
	fn := normFunc(c.name())
	if isClosure(fn) {
		return describeFunc(fn)
	}
	return funcBase(fn)
}

// ---------------------------------------------------------------------
// Creators.

// devCreator is what created a hook slot: Tela's own code (a hook of
// package aria, i18n...), and whether its value follows the pointer.
type devCreator struct {
	framework, transient bool
}

// devCreators caches devCreatorOf by call stack.
var devCreators sync.Map // callStack → devCreator

// devTransient are the hooks of Tela's packages whose state follows the
// pointer, which the next build learns again from the platform: they are
// not kept (a hover restored over an element the pointer has left would
// stay until the pointer enters and leaves it).
var devTransient = map[string]bool{
	telaPkg + "/aria.UseHover":     true,
	telaPkg + "/aria.usePressCore": true,
}

// devCreatorOf returns what created hook slot j of el, from the call
// stack dev mode recorded (nothing is known without dev mode): the first
// frame outside this package and the wrappers of package tela.
func devCreatorOf(el *Element, j int) devCreator {
	if j >= len(el.hookDev) {
		return devCreator{}
	}
	st := el.hookDev[j].stack
	if v, ok := devCreators.Load(st); ok {
		return v.(devCreator)
	}
	n := 0
	for n < len(st) && st[n] != 0 {
		n++
	}
	var cr devCreator
	if n > 0 {
		frames := runtime.CallersFrames(st[:n])
		for {
			f, more := frames.Next()
			if c, ok := devClassify(f.Function, f.File); ok {
				cr = c
				break
			}
			if !more {
				break
			}
		}
	}
	devCreators.Store(st, cr)
	return cr
}

// devClassify classifies a frame of a hook's call stack (function fn in
// file): ok is false for frames that do not decide (this package, the
// wrappers of package tela), else the frame is the slot's creator.
func devClassify(fn, file string) (devCreator, bool) {
	test := strings.HasSuffix(file, "_test.go")
	switch {
	case fn == "", strings.HasPrefix(fn, corePkg+"."):
		return devCreator{}, false
	case strings.HasPrefix(fn, telaPkg+".") && !strings.Contains(fn[len(telaPkg)+1:], "/") && !test:
		return devCreator{}, false // a wrapper of package tela (tela.UseState)
	}
	name := normFunc(fn)
	pkg := strings.TrimSuffix(name[:len(name)-len(symbol(name))], ".")
	var cr devCreator
	cr.framework = devFrameworkPkg(pkg) && !test
	named, _ := funcParts(name)
	cr.transient = cr.framework && devTransient[pkg+"."+named]
	return cr, true
}

// devFrameworkPkg reports whether the package at path is Tela's own (not
// an example or the user's code that shares the module path prefix).
func devFrameworkPkg(path string) bool {
	if path == telaPkg {
		return true
	}
	rest, ok := strings.CutPrefix(path, telaPkg+"/")
	if !ok {
		return false
	}
	first, _, _ := strings.Cut(rest, "/")
	switch first {
	case "h", "aria", "i18n", "kit", "telatest", "internal", "icons", "cmd":
		return true
	}
	return false
}
