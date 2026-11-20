package core

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The short form of values the instrumentation shows (hook values, props,
// keys): like %v (%+v for structs), but bounded in length and depth so a
// big slice or a deep tree costs what it shows, deterministic (maps by
// sorted keys, no addresses) and safe (a String method that panics is
// skipped; unexported fields are read without Interface, which panics on
// them, so their String methods are not called). Tela's handles (Setter,
// *Value, Callback...) are their type's name, nodes their tag ("<div>").

const (
	// maxValueLen bounds a value's short form, in bytes (an ellipsis
	// ends a longer one).
	maxValueLen = 120
	// maxValueDepth bounds how deep structs, pointers, slices and maps
	// are followed.
	maxValueDepth = 3
	// maxValueItems bounds the items of a slice, an array or a map shown.
	maxValueItems = 8
	// maxValueString bounds a string's runes shown.
	maxValueString = 60
)

// shortValue returns the short form of v.
func shortValue(v any) string { return shortRV(reflect.ValueOf(v)) }

// shortRV returns the short form of v.
func shortRV(v reflect.Value) string {
	w := shortWriter{max: maxValueLen}
	w.value(v, 0)
	return w.b.String()
}

type shortWriter struct {
	b    strings.Builder
	max  int
	full bool
}

// str writes s, cut with an ellipsis where the budget ends.
func (w *shortWriter) str(s string) {
	if w.full {
		return
	}
	if w.b.Len()+len(s) <= w.max {
		w.b.WriteString(s)
		return
	}
	n := max(w.max-w.b.Len(), 0)
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	w.b.WriteString(s[:n])
	w.b.WriteString("…")
	w.full = true
}

var (
	stringerType = reflect.TypeFor[fmt.Stringer]()
	errorType    = reflect.TypeFor[error]()
	nodeRType    = reflect.TypeFor[node]()
)

func (w *shortWriter) value(v reflect.Value, depth int) {
	if w.full {
		return
	}
	if !v.IsValid() {
		w.str("nil")
		return
	}
	rt := v.Type()
	if rt == nodeRType {
		w.str(nodeOfValue(v).String())
		return
	}
	if name, ok := handleName(rt); ok {
		w.str(name)
		return
	}
	if s, ok := callString(v); ok {
		w.str(s)
		return
	}
	switch v.Kind() {
	case reflect.Bool:
		w.str(strconv.FormatBool(v.Bool()))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		w.str(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		w.str(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32:
		w.str(strconv.FormatFloat(v.Float(), 'g', -1, 32))
	case reflect.Float64:
		w.str(strconv.FormatFloat(v.Float(), 'g', -1, 64))
	case reflect.Complex64, reflect.Complex128:
		w.str(strconv.FormatComplex(v.Complex(), 'g', -1, 128))
	case reflect.String:
		s := v.String()
		if utf8.RuneCountInString(s) > maxValueString {
			s = truncateRunes(s, maxValueString)
		}
		w.str(strconv.Quote(s))
	case reflect.Pointer:
		switch {
		case v.IsNil():
			w.str("nil")
		case depth >= maxValueDepth:
			w.str("&…")
		default:
			w.str("&")
			w.value(v.Elem(), depth+1)
		}
	case reflect.Interface:
		if v.IsNil() {
			w.str("nil")
			return
		}
		w.value(v.Elem(), depth)
	case reflect.Struct:
		if depth > 0 && rt.Name() != "" {
			w.str(goName(rt.String()))
		}
		if depth >= maxValueDepth {
			w.str("{…}")
			return
		}
		w.str("{")
		first := true
		for i := range rt.NumField() {
			f := rt.Field(i)
			if f.Name == "_" {
				continue
			}
			if !first {
				w.str(" ")
			}
			first = false
			w.str(f.Name + ":")
			w.value(v.Field(i), depth+1)
		}
		w.str("}")
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			w.str("[]")
			return
		}
		if depth >= maxValueDepth {
			w.str(fmt.Sprintf("[len %d]", v.Len()))
			return
		}
		w.str("[")
		n := v.Len()
		for i := range min(n, maxValueItems) {
			if i > 0 {
				w.str(" ")
			}
			w.value(v.Index(i), depth+1)
		}
		if n > maxValueItems {
			w.str(fmt.Sprintf(" …+%d", n-maxValueItems))
		}
		w.str("]")
	case reflect.Map:
		if v.IsNil() || v.Len() == 0 {
			w.str("map[]")
			return
		}
		if depth >= maxValueDepth || v.Len() > 4*maxValueItems {
			w.str(fmt.Sprintf("map[len %d]", v.Len()))
			return
		}
		type kv struct {
			k string
			v reflect.Value
		}
		var items []kv
		it := v.MapRange()
		for it.Next() {
			items = append(items, kv{shortRVDepth(it.Key(), depth+1), it.Value()})
		}
		slices.SortFunc(items, func(a, b kv) int { return strings.Compare(a.k, b.k) })
		w.str("map[")
		for i, e := range items {
			if i == maxValueItems {
				w.str(fmt.Sprintf(" …+%d", len(items)-maxValueItems))
				break
			}
			if i > 0 {
				w.str(" ")
			}
			w.str(e.k + ":")
			w.value(e.v, depth+1)
		}
		w.str("]")
	case reflect.Func:
		if v.IsNil() {
			w.str("nil")
			return
		}
		w.str("func")
	case reflect.Chan:
		if v.IsNil() {
			w.str("nil")
			return
		}
		w.str("chan")
	default:
		w.str(rt.String())
	}
}

// nodeOfValue returns the node v holds. A node in an unexported field
// (a struct component's `body tela.Node`) cannot be read with Interface:
// its one field, the pointer to its content, is read as a pointer, as fmt
// reads unexported pointers.
func nodeOfValue(v reflect.Value) node {
	if v.CanInterface() {
		return v.Interface().(node)
	}
	return node{d: (*desc)(v.Field(0).UnsafePointer())}
}

// handleName names a handle of Tela's (a Setter, Callback or Dispatcher,
// a *Value, a *Ref, an ElementRef): what a build makes again, shown as its
// type, not its insides.
func handleName(rt reflect.Type) (string, bool) {
	base, ptr := rt, ""
	if base.Kind() == reflect.Pointer {
		base, ptr = base.Elem(), "*"
	}
	if base.PkgPath() != corePkg || base.Kind() != reflect.Struct {
		return "", false
	}
	name := base.Name()
	switch {
	case ptr == "" && rt.Implements(zeroHandleType),
		ptr != "" && (strings.HasPrefix(name, "Value[") || strings.HasPrefix(name, "Ref[")),
		name == "ElementRef":
		return ptr + goName(name), true
	}
	return "", false
}

// shortRVDepth is shortRV for a value depth levels deep (a map key).
func shortRVDepth(v reflect.Value, depth int) string {
	w := shortWriter{max: maxValueLen}
	w.value(v, depth)
	return w.b.String()
}

// callString returns the String or Error method's result for a value of
// a named type that has one, as %v would print it; false when it has none,
// its value cannot be read (an unexported field), or the method panics.
func callString(v reflect.Value) (s string, ok bool) {
	rt := v.Type()
	named := rt.Name() != "" || rt.Kind() == reflect.Pointer && rt.Elem().Name() != ""
	if !named || !v.CanInterface() || rt.Kind() == reflect.Interface {
		return "", false
	}
	if rt.Kind() == reflect.Pointer && v.IsNil() {
		return "", false
	}
	if !rt.Implements(errorType) && !rt.Implements(stringerType) {
		return "", false
	}
	defer func() {
		if recover() != nil {
			s, ok = "", false
		}
	}()
	switch x := v.Interface().(type) {
	case error:
		return x.Error(), true
	case fmt.Stringer:
		return x.String(), true
	}
	return "", false
}
