package core

import (
	"fmt"
	"reflect"
	"strings"
)

// Contexts (ARCHITECTURE section 7) pass a value down the tree without
// props. A Provider is a component Element without a function: it holds
// the provided value and the consumers that read it (UseContext). When
// the parent builds it with a value that is not == to the previous one
// (a value that cannot be compared always counts as changed), only those
// consumers are marked for building, like Flutter's InheritedWidget; the
// build phase reaches them through components that skip their build.

// Context is a value passed down the tree: Provider gives it to a
// subtree and UseContext reads the nearest one. Create contexts once, at
// package level, with CreateContext.
type Context[T any] struct {
	def T
}

// CreateContext returns a new context whose UseContext outside any
// Provider returns def.
func CreateContext[T any](def T) *Context[T] { return &Context[T]{def: def} }

// Provider returns a node that gives value to the UseContext calls of
// its children's subtrees. It has no box: the children become children
// of the provider's parent, like a Fragment, and only Key is accepted as
// a prop.
func (c *Context[T]) Provider(value T, children ...Child) Node {
	if c == nil {
		panic("tela: Provider on a nil *Context; create contexts with tela.CreateContext")
	}
	d := &desc{kind: nodeComponent, comp: &compDesc{ctx: c, value: value}, children: make([]Node, 0, len(children))}
	for _, ch := range children {
		d.addGroupChild(ch, "Context.Provider", "a provider has no box (wrap the children in a Div)")
	}
	return Node{d}
}

// providerState is the state of a Provider Element.
type providerState struct {
	value any
	subs  []*ctxSub
}

// ctxSub is a consumer's subscription to a Provider.
type ctxSub struct {
	el   *Element // the consumer
	prov *Element // the provider; nil reads the default
	idx  int      // index in prov's subs
}

func (s *ctxSub) subscribe(prov *Element) {
	s.prov = prov
	if prov != nil {
		s.idx = len(prov.prov.subs)
		prov.prov.subs = append(prov.prov.subs, s)
	}
}

func (s *ctxSub) unsubscribe() {
	p := s.prov
	if p == nil {
		return
	}
	s.prov = nil
	subs := p.prov.subs
	last := len(subs) - 1
	subs[s.idx] = subs[last]
	subs[s.idx].idx = s.idx
	subs[last] = nil
	p.prov.subs = subs[:last]
}

// findProvider returns the nearest Provider of context key above el, or
// nil. Above the root of a child window's tree the search goes on from
// the Window element that opened it (Tree.outer, window.go): one element
// tree, one tree of boxes per window. Elements never move to another
// parent, and a window's tree never to another Window element, so the
// result is the same for el's whole life.
func (el *Element) findProvider(key any) *Element {
	for p := el.parent; p != nil; {
		if p.prov != nil && p.comp.ctx == key {
			return p
		}
		if p.parent == nil {
			p = p.tree.outer
			continue
		}
		p = p.parent
	}
	return nil
}

// ctxSlot is the slot of one UseContext call.
type ctxSlot[T any] struct {
	ctx *Context[T]
	sub ctxSub
}

func (s *ctxSlot[T]) unmount(*Tree) { s.sub.unsubscribe() }

// UseContext returns the value of the nearest Provider of c above the
// component, or c's default without one. The component builds again
// when that Provider gets a value that is not == to the previous one,
// even when the components between them skip their builds.
func UseContext[T any](c *Context[T]) T {
	if c == nil {
		panic("tela: UseContext(nil): pass a context made with tela.CreateContext")
	}
	el, i := nextSlot(hookContext)
	var s *ctxSlot[T]
	if i < len(el.hooks) {
		var ok bool
		s, ok = el.hooks[i].(*ctxSlot[T])
		if !ok {
			el.tree.slotMismatch(el, i, "UseContext["+typeName[T]()+"]")
		}
		if s.ctx != c {
			s.sub.unsubscribe()
			s.ctx = c
			s.sub.subscribe(el.findProvider(c))
			if s.sub.prov == nil && el.tree.dev {
				el.tree.checkContextDefault(el, i, c, typeName[T](), any(c.def))
			}
		}
	} else {
		s = &ctxSlot[T]{ctx: c}
		s.sub.el = el
		el.hooks = append(el.hooks, s)
		s.sub.subscribe(el.findProvider(c))
		if s.sub.prov == nil && el.tree.dev {
			el.tree.checkContextDefault(el, i, c, typeName[T](), any(c.def))
		}
	}
	if s.sub.prov == nil {
		return c.def
	}
	v, _ := s.sub.prov.prov.value.(T)
	return v
}

// mountProvider mounts the Provider Element el.
func (t *Tree) mountProvider(el *Element) {
	d := el.node.d
	el.prov = &providerState{value: d.comp.value}
	t.reconcile(el, d.children)
}

// updateProvider brings Provider el up to date with n. A new value that
// is not == to the old one marks the consumers for building.
func (t *Tree) updateProvider(el *Element, n Node) {
	d := n.d
	if el.node.d == d {
		if el.dirtyKids {
			t.descend(el)
		}
		return
	}
	el.node, el.comp = n, d.comp
	if !safeEqual(el.prov.value, d.comp.value) {
		el.prov.value = d.comp.value
		for _, s := range el.prov.subs {
			// A consumer in a child window builds in its window's tree.
			s.el.tree.markBuild(s.el)
		}
	}
	t.reconcile(el, d.children)
	el.dirtyKids = false
}

// ---------------------------------------------------------------------
// Defaults that are not safe to use (dev mode, ADR-085).

// zeroHandle is implemented by the handles whose zero value panics when
// used (Setter, Callback, Dispatcher): a context default holding one is
// a trap when no Provider is above the reader.
type zeroHandle interface{ zeroHandle() bool }

var zeroHandleType = reflect.TypeFor[zeroHandle]()

// checkContextDefault warns, once per context, when slot i of el reads
// context c (of type typ) without a Provider above and c's default is not
// safe to use: nil, or holding a nil func, pointer, map, channel or
// interface, or a zero Setter, Callback or Dispatcher, in an exported
// field.
func (t *Tree) checkContextDefault(el *Element, i int, c any, typ string, def any) {
	why := zeroUnsafe(def)
	if why == "" {
		return
	}
	t.warnOnce(fmt.Sprintf("ctx-default:%p", c), func() string {
		return fmt.Sprintf("tela: UseContext at %s reads a Context[%s] with no Provider above %s, so it gets the default given to CreateContext, and %s: using it panics. Wrap the components that read it in ctx.Provider(value, ...), or give CreateContext a default that works (no-op functions, non-nil pointers)",
			el.hookSite(i), typ, t.describe(el), why)
	})
}

// zeroUnsafe says why a context default is not safe to use, or "".
func zeroUnsafe(def any) string {
	if def == nil {
		return "the default is nil"
	}
	return unsafeValue(reflect.ValueOf(def), "the default", 0)
}

// unsafeValue says why v (described as what) is not safe to use, looking
// into the exported fields of structs and the elements of small arrays,
// a few levels deep; pointers are not followed.
func unsafeValue(v reflect.Value, what string, depth int) string {
	ty := v.Type()
	if ty.Implements(zeroHandleType) && ty.Kind() == reflect.Struct && ty.NumField() > 0 {
		if f := v.Field(0); (f.Kind() == reflect.Pointer || f.Kind() == reflect.Interface) && f.IsNil() {
			return what + " is a zero " + ty.Name()
		}
		return ""
	}
	switch v.Kind() {
	case reflect.Func, reflect.Pointer, reflect.Map, reflect.Chan, reflect.Interface, reflect.UnsafePointer:
		if v.IsNil() {
			return what + " is a nil " + ty.String()
		}
	case reflect.Struct:
		if depth >= 4 {
			return ""
		}
		for k := range ty.NumField() {
			if f := ty.Field(k); f.IsExported() {
				if why := unsafeValue(v.Field(k), fieldPath(what, f.Name), depth+1); why != "" {
					return why
				}
			}
		}
	case reflect.Array:
		if depth >= 4 {
			return ""
		}
		for k := range min(v.Len(), 8) {
			if why := unsafeValue(v.Index(k), fmt.Sprintf("%s[%d]", what, k), depth+1); why != "" {
				return why
			}
		}
	}
	return ""
}

// fieldPath describes field name of what: "its field OnSave", then
// "its field Theme.OnSave".
func fieldPath(what, name string) string {
	if rest, ok := strings.CutPrefix(what, "its field "); ok {
		return "its field " + rest + "." + name
	}
	return "its field " + name
}
