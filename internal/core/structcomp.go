package core

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

// Struct components (ADR-088). A Component value other than a Node or a
// Prop, given to an element, becomes a component node:
//
//	type Card struct{ Title string; Children Component }
//	func (c Card) Build() Node { ... }
//
//	div(Card{Title: "Inbox", Children: Text("a")})
//
// Its identity among its siblings is its dynamic type (and its key), so
// two values of one type at one position share an Element and its hook
// state, and a value of another type remounts. Its build is Build, whose
// hooks belong to it (the hook-boundary check accepts Build and the
// function literals in it). When its parent builds again, the build is
// skipped if the new value equals the old one and no state, Value or
// context it read changed, as for ComponentProps; the comparison is
// componentEqual, which never panics: a value that cannot be compared
// counts as changed. In dev mode a pointer type panics (components are
// values, and Build has a value receiver), and so does a Prop held by a
// field of type Component, where it would silently apply to the element
// the field is given to.

// compType is what is known about the dynamic type of struct components,
// computed once per type (compTypes).
type compType struct {
	// pc is the code pointer of the type's Build method (compDesc.typ).
	pc uintptr
	// name is the full type name ("example.com/app.Card") and short the
	// name in messages ("Card").
	name, short string
	// owner is the hook-boundary function, the normalized name of the
	// Build method ("example.com/app.Card.Build"), and owners those of
	// the embedded types whose Build is promoted to the type, which
	// accept hooks too.
	owner  string
	owners []string
	// ptr marks a pointer type, and ptrRecv a pointer type whose
	// element type has no Build method (it is declared with a pointer
	// receiver); elem is the element type's short name.
	ptr, ptrRecv bool
	elem         string
	// eq is how two values of the type compare.
	eq eqKind
	// fields are the exported fields of type Component, or of slices or
	// arrays of Components, checked for Props in dev mode.
	fields []compField
}

// compField is a field of a struct component that holds Components.
type compField struct {
	index []int
	name  string
	list  bool // a slice or an array of Components
}

var (
	compTypes     sync.Map // reflect.Type → *compType
	componentType = reflect.TypeFor[Comp]()
)

// componentOf returns the component node of a struct component c (any
// Component but a Node or a Prop).
//
// perf: every struct component given to an element allocates a desc, a
// compDesc and the Build method value on each build of its parent, and
// looks its type up in compTypes; a compDesc embedded in the desc (or
// pooled per type) would halve that if profiles show it.
func componentOf(c Comp) Node {
	rt := reflect.TypeOf(c)
	ct := compTypeOf(rt)
	return Node{&desc{kind: nodeComponent, comp: &compDesc{
		typ: ct.pc, rtype: rt, ct: ct, build: c.Build, props: c, mode: compStruct,
	}}}
}

// compTypeOf returns what is known about the struct component type rt.
func compTypeOf(rt reflect.Type) *compType {
	if v, ok := compTypes.Load(rt); ok {
		return v.(*compType)
	}
	v, _ := compTypes.LoadOrStore(rt, newCompType(rt))
	return v.(*compType)
}

func newCompType(rt reflect.Type) *compType {
	ct := &compType{name: fullTypeName(rt), short: shortTypeName(rt), eq: eqKindOf(rt)}
	if m, ok := rt.MethodByName("Build"); ok && m.Func.IsValid() {
		ct.pc = m.Func.Pointer()
		if f := runtime.FuncForPC(ct.pc); f != nil {
			ct.owner = normFunc(f.Name())
		}
	}
	ct.owners = embeddedBuilds(rt, nil, 0)
	if rt.Kind() == reflect.Pointer {
		ct.ptr = true
		ct.elem = shortTypeName(rt.Elem())
		_, ok := rt.Elem().MethodByName("Build")
		ct.ptrRecv = !ok
	}
	if rt.Kind() == reflect.Struct {
		ct.fields = componentFields(rt)
	}
	return ct
}

// fullTypeName returns the full name of a type: "example.com/app.Card",
// "*example.com/app.Card", or the type literal of an unnamed one.
func fullTypeName(rt reflect.Type) string {
	if rt.Kind() == reflect.Pointer && rt.Name() == "" {
		return "*" + fullTypeName(rt.Elem())
	}
	if rt.Name() != "" && rt.PkgPath() != "" {
		return rt.PkgPath() + "." + rt.Name()
	}
	return rt.String()
}

// shortTypeName returns the name of a type without its package path:
// "Card", "*Card", "Box[int]".
func shortTypeName(rt reflect.Type) string {
	if rt.Kind() == reflect.Pointer && rt.Name() == "" {
		return "*" + shortTypeName(rt.Elem())
	}
	if rt.Name() != "" {
		return rt.Name()
	}
	return rt.String()
}

// embeddedBuilds appends the normalized names of the Build methods of the
// types embedded in rt (through pointers too, recursively, a few levels
// deep): a Build promoted from one of them is accepted as the boundary.
// An embedded interface (tela.Component, tela.Child) is skipped: its Build
// is whatever the field holds at run time, and reflect has no code for it
// (its Method.Func is the zero Value, whose Pointer panics).
func embeddedBuilds(rt reflect.Type, out []string, depth int) []string {
	if rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
		if depth == 0 {
			// A pointer type: its Build is the element type's.
			out = appendBuildName(out, rt)
		}
	}
	if rt.Kind() != reflect.Struct || depth > 4 {
		return out
	}
	for i := range rt.NumField() {
		f := rt.Field(i)
		if !f.Anonymous || f.Type.Kind() == reflect.Interface {
			continue
		}
		out = appendBuildName(out, f.Type)
		out = embeddedBuilds(f.Type, out, depth+1)
	}
	return out
}

// appendBuildName appends the normalized name of the code of rt's Build
// method, if rt has one with code (an interface type has none).
func appendBuildName(out []string, rt reflect.Type) []string {
	m, ok := rt.MethodByName("Build")
	if !ok || !m.Func.IsValid() {
		return out
	}
	if fn := runtime.FuncForPC(m.Func.Pointer()); fn != nil {
		out = append(out, normFunc(fn.Name()))
	}
	return out
}

// componentFields returns the exported fields of struct type rt (promoted
// ones included, through exported embedded fields only) whose type is
// Component, or a slice or an array of Components.
func componentFields(rt reflect.Type) []compField {
	var out []compField
	for _, f := range reflect.VisibleFields(rt) {
		list := false
		switch {
		case f.Type == componentType:
		case (f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Array) && f.Type.Elem() == componentType:
			list = true
		default:
			continue
		}
		if !exportedPath(rt, f.Index) {
			continue
		}
		out = append(out, compField{index: f.Index, name: f.Name, list: list})
	}
	return out
}

// exportedPath reports whether every field on the way to the field at
// index of rt is exported, so that its value can be read with Interface.
func exportedPath(rt reflect.Type, index []int) bool {
	for i := 1; i <= len(index); i++ {
		if !rt.FieldByIndex(index[:i]).IsExported() {
			return false
		}
	}
	return true
}

// checkStructComp is the dev-mode check of a struct component node c,
// when it is mounted or updated: a pointer type panics, and so does a
// Prop in a field of type Component (or in a slice of them).
func (t *Tree) checkStructComp(c *compDesc) {
	ct := c.ct
	if ct.ptr {
		panic(pointerComponentMessage(ct))
	}
	if len(ct.fields) == 0 {
		return
	}
	v := reflect.ValueOf(c.props)
	for _, f := range ct.fields {
		fv, err := v.FieldByIndexErr(f.index)
		if err != nil {
			continue // promoted through a nil embedded pointer
		}
		if !f.list {
			if p, ok := fieldProp(fv); ok {
				panic(propFieldMessage(ct, f.name, p))
			}
			continue
		}
		for k := range fv.Len() {
			if p, ok := fieldProp(fv.Index(k)); ok {
				panic(propFieldMessage(ct, fmt.Sprintf("%s[%d]", f.name, k), p))
			}
		}
	}
}

// fieldProp returns the Prop held by fv, a Component value, if any.
func fieldProp(fv reflect.Value) (Prop, bool) {
	if fv.IsNil() {
		return Prop{}, false
	}
	c, ok := fv.Interface().(Comp)
	if !ok {
		return Prop{}, false
	}
	return propOf(c)
}

func pointerComponentMessage(ct *compType) string {
	if ct.ptrRecv {
		return fmt.Sprintf("tela: a %s is given as a component, and %s.Build has a pointer receiver; declare it with a value receiver, func (c %s) Build() tela.Node, and pass %s{...}: a component is a value, which Tela compares with the previous one to skip builds (ARCHITECTURE section 7)",
			ct.short, ct.elem, ct.elem, ct.elem)
	}
	return fmt.Sprintf("tela: &%s{...} (a %s) is given as a component; pass the value, %s{...}, without the &: a component is a value, which Tela compares with the previous one to skip builds, and Build has a value receiver (ARCHITECTURE section 7)",
		ct.elem, ct.short, ct.elem)
}

func propFieldMessage(ct *compType, field string, p Prop) string {
	n := propName(p)
	return fmt.Sprintf("tela: %s.%s holds a Prop (%s), not a component. A field of type tela.Component holds what renders: a node (Text, Div...), several with Fragment(...), or a component; put the prop on an element inside it (%s: Div(%s(...), ...)), or declare the field as tela.Child if it is meant to take props",
		ct.short, field, n, field, n)
}

// ---------------------------------------------------------------------
// Equality.

// eqKind says how two values of a type compare (componentEqual).
type eqKind uint8

const (
	// eqNever: the type is not comparable (it holds a slice, a map or a
	// func), so two values always count as different.
	eqNever eqKind = iota
	// eqDirect: comparable and without interfaces inside, so == never
	// panics.
	eqDirect
	// eqDeep: comparable, but == may panic on a non-comparable value
	// inside an interface: compared field by field.
	eqDeep
)

var eqKinds sync.Map // reflect.Type → eqKind

// eqKindOf returns how values of type rt compare, computed once per type.
func eqKindOf(rt reflect.Type) eqKind {
	if v, ok := eqKinds.Load(rt); ok {
		return v.(eqKind)
	}
	k := eqDirect
	switch {
	case !rt.Comparable():
		k = eqNever
	case holdsInterface(rt):
		k = eqDeep
	}
	eqKinds.Store(rt, k)
	return k
}

// holdsInterface reports whether a comparable type has an interface type
// inside it (itself, a field, an array element), whose dynamic value may
// not be comparable.
func holdsInterface(rt reflect.Type) bool {
	switch rt.Kind() {
	case reflect.Interface:
		return true
	case reflect.Struct:
		for i := range rt.NumField() {
			if holdsInterface(rt.Field(i).Type) {
				return true
			}
		}
	case reflect.Array:
		return rt.Len() > 0 && holdsInterface(rt.Elem())
	}
	return false
}

// componentEqual reports whether two struct component values are equal,
// as == would, but without panicking: a value that cannot be compared (a
// type holding a slice, a map or a func, or an interface whose dynamic
// value is one) counts as different. A Node compares by identity (a
// fresh Div(...) differs, a memoized one is equal).
func componentEqual(a, b any) bool {
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if ta == nil {
		return true
	}
	switch eqKindOf(ta) {
	case eqDirect:
		return a == b
	case eqDeep:
		return valuesEqual(reflect.ValueOf(a), reflect.ValueOf(b))
	}
	return false
}

// valuesEqual compares two values of one comparable type like ==, field
// by field, reporting false where == would panic. It reads unexported
// fields without Interface.
func valuesEqual(v, u reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool() == u.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == u.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == u.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float() == u.Float()
	case reflect.Complex64, reflect.Complex128:
		return v.Complex() == u.Complex()
	case reflect.String:
		return v.String() == u.String()
	case reflect.Pointer, reflect.Chan, reflect.UnsafePointer:
		return v.Pointer() == u.Pointer()
	case reflect.Interface:
		if v.IsNil() || u.IsNil() {
			return v.IsNil() == u.IsNil()
		}
		ev, eu := v.Elem(), u.Elem()
		if ev.Type() != eu.Type() || eqKindOf(ev.Type()) == eqNever {
			return false
		}
		return valuesEqual(ev, eu)
	case reflect.Struct:
		rt := v.Type()
		for i := range v.NumField() {
			if rt.Field(i).Name == "_" {
				continue // == ignores blank fields
			}
			if !valuesEqual(v.Field(i), u.Field(i)) {
				return false
			}
		}
		return true
	case reflect.Array:
		for i := range v.Len() {
			if !valuesEqual(v.Index(i), u.Index(i)) {
				return false
			}
		}
		return true
	}
	return false // a func, a map or a slice: not comparable
}

// ---------------------------------------------------------------------
// Hook boundaries.

// ownsHooks reports whether fn, a normalized function name on the stack
// of a hook, belongs to the component c being built: its function or its
// Build method, a function literal in them, or the Build of an embedded
// type promoted to a struct component.
func (c *compDesc) ownsHooks(fn, owner string) bool {
	if ownedBy(fn, owner) {
		return true
	}
	if c.ct != nil {
		for _, o := range c.ct.owners {
			if ownedBy(fn, o) {
				return true
			}
		}
	}
	return false
}

// buildMethod returns the receiver type of fn when fn is a Build method
// or a function literal inside one ("Card" for "app.Card.Build" and
// "app.(*Card).Build.func1"): a struct component whose Build was called
// directly.
func buildMethod(fn string) (string, bool) {
	named, _ := funcParts(fn)
	recv, m, ok := strings.Cut(named, ".")
	if !ok || m != "Build" {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(recv, "(*"), ")"), true
}
