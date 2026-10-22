//go:build teladev

package core

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
)

// Which values the dev state keeps (ADR-113), and how they are encoded.
//
// A value is kept when encoding/gob encodes it in full. Gob silently drops
// unexported fields and cannot encode funcs or channels, so the type is
// checked by reflection first (devKeepOf, cached per type): booleans,
// numbers, strings, slices, arrays and maps of kept types, pointers to
// them (gob flattens pointers: aliasing is lost), structs whose fields are
// all exported and kept, and types with GobEncode/GobDecode,
// MarshalBinary/UnmarshalBinary or MarshalText/UnmarshalText (time.Time)
// are kept; funcs, channels, unsafe pointers and structs with unexported
// fields are not. Interfaces, and types that can hold themselves (a tree of
// pointers), are checked again on the value: the dynamic value of an
// interface must be kept (and registered with gob.Register by the app, as
// gob requires; a value of an unregistered type fails to encode and
// resets), and a value that holds itself (a cycle of pointers) is not kept,
// since gob would never return.
//
// Tela's own types that cannot be kept are the handles a build makes again
// (Setter, Callback, Dispatcher, *Value, ElementRef, Node, Prop) and the
// internal state of Tela's hooks: their slots reset without a warning.
//
// The encoding of a slot starts with a tag byte: the zero value, a few
// basic kinds encoded directly (the common case, much faster than gob), or
// gob.

const (
	devTagZero byte = iota + 1
	devTagBool
	devTagInt
	devTagUint
	devTagFloat
	devTagString
	devTagGob
)

// devWhy is why a value is not kept ("" when it is), and whether that
// goes without a warning.
type devWhy struct {
	reason string
	silent bool
}

// devKept is a hook slot whose value the dev state keeps.
type devKept interface {
	devEncode() ([]byte, devWhy)
	devDecode([]byte) error
}

func (s *stateSlot[T]) devEncode() ([]byte, devWhy) { return devEncodeValue(s.value) }

func (s *stateSlot[T]) devDecode(b []byte) error {
	v, err := devDecodeValue[T](b)
	if err == nil {
		s.value = v
	}
	return err
}

// A Value moved by Animate is kept at its target: animations restart from
// rest.
func (v *Value[T]) devEncode() ([]byte, devWhy) {
	x := *v.cur.Load()
	if a, ok := v.anim.(devAnimTarget); ok {
		if to, ok := a.devTarget(); ok {
			if tt, ok := to.(T); ok {
				x = tt
			}
		}
	}
	return devEncodeValue(x)
}

func (v *Value[T]) devDecode(b []byte) error {
	x, err := devDecodeValue[T](b)
	if err == nil {
		v.cur.Store(&x)
	}
	return err
}

func (r *Ref[T]) devEncode() ([]byte, devWhy) { return devEncodeValue(r.Current) }

func (r *Ref[T]) devDecode(b []byte) error {
	x, err := devDecodeValue[T](b)
	if err == nil {
		r.Current = x
	}
	return err
}

// devAnimTarget is a Value's animation (valueAnim), which reports where it
// is going.
type devAnimTarget interface{ devTarget() (any, bool) }

func (a *valueAnim[T]) devTarget() (any, bool) {
	switch {
	case a.hasNext:
		return a.nextTo, true
	case a.running:
		return a.toT, true
	}
	return nil, false
}

// ---------------------------------------------------------------------
// Encoding.

var errDevValue = errors.New("the saved value does not fit the slot")

// devEncodeValue encodes v for a hook slot of type T, or reports why it
// is not kept.
func devEncodeValue[T any](v T) ([]byte, devWhy) {
	rt := reflect.TypeFor[T]()
	k := devKeepOf(rt)
	if k.bad != "" {
		return nil, devWhy{reason: devReason(rt, k), silent: k.silent}
	}
	rv := reflect.ValueOf(&v).Elem()
	if k.walk {
		var w devWalker
		if bad, bt, silent := w.walk(rv); bad != "" {
			return nil, devWhy{reason: devReason(rt, devKeep{bad: bad, badType: bt}), silent: silent}
		}
	}
	if rv.IsZero() {
		return []byte{devTagZero}, devWhy{}
	}
	if !k.custom {
		switch rv.Kind() {
		case reflect.Bool:
			b := byte(0)
			if rv.Bool() {
				b = 1
			}
			return []byte{devTagBool, b}, devWhy{}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return binary.AppendVarint([]byte{devTagInt}, rv.Int()), devWhy{}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return binary.AppendUvarint([]byte{devTagUint}, rv.Uint()), devWhy{}
		case reflect.Float32, reflect.Float64:
			return binary.LittleEndian.AppendUint64([]byte{devTagFloat}, math.Float64bits(rv.Float())), devWhy{}
		case reflect.String:
			return append([]byte{devTagString}, rv.String()...), devWhy{}
		}
	}
	var buf bytes.Buffer
	buf.WriteByte(devTagGob)
	if err := devGobEncode(&buf, &v); err != nil {
		hint := ""
		if bytes.Contains([]byte(err.Error()), []byte("not registered")) {
			hint = "; register the types an interface holds with gob.Register, in an init function"
		}
		return nil, devWhy{reason: fmt.Sprintf("holds %s, which encoding/gob cannot encode (%v%s)", devA(devShortType(rt)), err, hint)}
	}
	return buf.Bytes(), devWhy{}
}

// devDecodeValue decodes a value encoded by devEncodeValue for type T.
func devDecodeValue[T any](b []byte) (T, error) {
	var v T
	if len(b) == 0 {
		return v, errDevValue
	}
	rv := reflect.ValueOf(&v).Elem()
	switch b[0] {
	case devTagZero:
		return v, nil
	case devTagBool:
		if rv.Kind() != reflect.Bool || len(b) != 2 {
			return v, errDevValue
		}
		rv.SetBool(b[1] == 1)
	case devTagInt:
		x, n := binary.Varint(b[1:])
		if n <= 0 || n != len(b)-1 || !rv.CanInt() || rv.OverflowInt(x) {
			return v, errDevValue
		}
		rv.SetInt(x)
	case devTagUint:
		x, n := binary.Uvarint(b[1:])
		if n <= 0 || n != len(b)-1 || !rv.CanUint() || rv.OverflowUint(x) {
			return v, errDevValue
		}
		rv.SetUint(x)
	case devTagFloat:
		if len(b) != 9 || !rv.CanFloat() {
			return v, errDevValue
		}
		rv.SetFloat(math.Float64frombits(binary.LittleEndian.Uint64(b[1:])))
	case devTagString:
		if rv.Kind() != reflect.String {
			return v, errDevValue
		}
		rv.SetString(string(b[1:]))
	case devTagGob:
		if err := devGobDecode(b[1:], &v); err != nil {
			var zero T
			return zero, err
		}
	default:
		return v, errDevValue
	}
	return v, nil
}

// devGobEncode encodes v with gob, turning a panic into an error.
func devGobEncode(buf *bytes.Buffer, v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gob: %v", r)
		}
	}()
	return gob.NewEncoder(buf).Encode(v)
}

// devGobDecode decodes b with gob into v, turning a panic into an error.
func devGobDecode(b []byte, v any) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("gob: %v", r)
		}
	}()
	return gob.NewDecoder(bytes.NewReader(b)).Decode(v)
}

// ---------------------------------------------------------------------
// Keepable types.

// devKeep is what is known about keeping values of a type.
type devKeep struct {
	// bad describes what cannot be kept ("a func()", "a sync.Mutex with
	// the unexported field state"), "" when values can be kept; badType is
	// that type; silent marks a Tela type (no warning).
	bad     string
	badType reflect.Type
	silent  bool
	// walk: values must be checked too (interfaces inside, or a type that
	// can hold itself).
	walk bool
	// custom: gob encodes the type with its own methods.
	custom bool
}

var devKeeps sync.Map // reflect.Type → devKeep

// devKeepOf returns what is known about keeping values of type rt.
func devKeepOf(rt reflect.Type) devKeep {
	if v, ok := devKeeps.Load(rt); ok {
		return v.(devKeep)
	}
	k := devKeepType(rt, map[reflect.Type]bool{})
	devKeeps.Store(rt, k)
	return k
}

var (
	devGobEncoder  = reflect.TypeFor[gob.GobEncoder]()
	devGobDecoder  = reflect.TypeFor[gob.GobDecoder]()
	devBinEncoder  = reflect.TypeFor[encoding.BinaryMarshaler]()
	devBinDecoder  = reflect.TypeFor[encoding.BinaryUnmarshaler]()
	devTextEncoder = reflect.TypeFor[encoding.TextMarshaler]()
	devTextDecoder = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// devMarshaler reports whether gob encodes rt with the type's own methods
// in both directions.
func devMarshaler(rt reflect.Type) bool {
	pt := reflect.PointerTo(rt)
	has := func(enc, dec reflect.Type) bool {
		return (rt.Implements(enc) || pt.Implements(enc)) && pt.Implements(dec)
	}
	return has(devGobEncoder, devGobDecoder) || has(devBinEncoder, devBinDecoder) || has(devTextEncoder, devTextDecoder)
}

// devKeepType computes devKeepOf; seen holds the composite types being
// checked (a type met again holds itself).
func devKeepType(rt reflect.Type, seen map[reflect.Type]bool) devKeep {
	if rt.Kind() != reflect.Interface && devMarshaler(rt) {
		return devKeep{custom: true}
	}
	if seen[rt] {
		return devKeep{walk: true}
	}
	switch rt.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.String:
		return devKeep{}
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return devNot(rt, devA(devShortType(rt)))
	case reflect.Interface:
		return devKeep{walk: true}
	case reflect.Pointer, reflect.Slice, reflect.Array:
		seen[rt] = true
		k := devKeepType(rt.Elem(), seen)
		delete(seen, rt)
		return devInside(rt, k)
	case reflect.Map:
		seen[rt] = true
		kk := devKeepType(rt.Key(), seen)
		ke := devKeepType(rt.Elem(), seen)
		delete(seen, rt)
		if kk.bad != "" {
			return devInside(rt, kk)
		}
		ke.walk = ke.walk || kk.walk
		return devInside(rt, ke)
	case reflect.Struct:
		seen[rt] = true
		defer delete(seen, rt)
		walk, fields := false, 0
		for i := range rt.NumField() {
			f := rt.Field(i)
			if f.Name == "_" {
				continue // gob skips blank fields, which hold nothing
			}
			if !f.IsExported() {
				return devNot(rt, fmt.Sprintf("%s with the unexported field %s", devA(devShortType(rt)), f.Name))
			}
			k := devKeepType(f.Type, seen)
			if k.bad != "" {
				return devInside(rt, k)
			}
			walk = walk || k.walk
			fields++
		}
		if fields == 0 {
			// gob rejects a struct type without exported fields wherever it
			// appears (map[string]struct{} included).
			return devNot(rt, devA(devShortType(rt))+" (encoding/gob cannot encode a struct without exported fields)")
		}
		return devKeep{walk: walk}
	}
	return devNot(rt, devA(devShortType(rt)))
}

// devNot is a type whose values cannot be kept, described by bad.
func devNot(rt reflect.Type, bad string) devKeep {
	return devKeep{bad: bad, badType: rt, silent: devFrameworkType(rt)}
}

// devInside is what holding a value described by k makes of rt: a Tela
// type inside one that cannot be kept stays silent, and so does a Tela
// type around it.
func devInside(rt reflect.Type, k devKeep) devKeep {
	if k.bad != "" && devFrameworkType(rt) {
		k.silent = true
	}
	return k
}

// devFrameworkType reports whether rt is a named type of Tela's packages.
func devFrameworkType(rt reflect.Type) bool {
	return rt.Name() != "" && devFrameworkPkg(rt.PkgPath())
}

// devReason returns the reason of a slot of type rt not kept because of
// k: "holds a func()", "holds an app.Form, which holds a func()".
func devReason(rt reflect.Type, k devKeep) string {
	if k.badType == rt {
		return "holds " + k.bad
	}
	return fmt.Sprintf("holds %s, which holds %s", devA(devShortType(rt)), k.bad)
}

// devShortType names a type in messages ("app.Item", "func(int)").
func devShortType(rt reflect.Type) string { return rt.String() }

// devA puts "a" or "an" before a type name.
func devA(s string) string {
	if s != "" {
		switch s[0] {
		case 'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U':
			return "an " + s
		}
	}
	return "a " + s
}

// devWalker checks the values of types that devKeepOf cannot decide:
// the dynamic values of interfaces, and cycles.
type devWalker struct {
	// path holds the pointers, maps and slices on the way to the value
	// being checked: meeting one again is a cycle.
	path map[uintptr]bool
}

// walk returns what in v cannot be kept ("" when everything can).
func (w *devWalker) walk(v reflect.Value) (bad string, badType reflect.Type, silent bool) {
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return "", nil, false
		}
		e := v.Elem()
		k := devKeepOf(e.Type())
		if k.bad != "" {
			if k.badType == e.Type() {
				return "an interface value that is " + k.bad, k.badType, k.silent
			}
			return k.bad, k.badType, k.silent
		}
		if k.walk {
			return w.walk(e)
		}
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if v.IsNil() {
			return "", nil, false
		}
		if k := devKeepOf(v.Type()); !k.walk {
			return "", nil, false
		}
		p := v.Pointer()
		if v.Kind() == reflect.Slice && v.Len() == 0 {
			return "", nil, false
		}
		if w.path[p] {
			return "a value that holds itself (a cycle of pointers), which encoding/gob cannot encode", v.Type(), false
		}
		if w.path == nil {
			w.path = map[uintptr]bool{}
		}
		w.path[p] = true
		defer delete(w.path, p)
		switch v.Kind() {
		case reflect.Pointer:
			return w.walk(v.Elem())
		case reflect.Slice:
			return w.walkList(v)
		}
		it := v.MapRange()
		for it.Next() {
			if bad, bt, s := w.walk(it.Key()); bad != "" {
				return bad, bt, s
			}
			if bad, bt, s := w.walk(it.Value()); bad != "" {
				return bad, bt, s
			}
		}
	case reflect.Array:
		if k := devKeepOf(v.Type()); k.walk {
			return w.walkList(v)
		}
	case reflect.Struct:
		if k := devKeepOf(v.Type()); !k.walk {
			return "", nil, false
		}
		for i := range v.NumField() {
			if bad, bt, s := w.walk(v.Field(i)); bad != "" {
				return bad, bt, s
			}
		}
	}
	return "", nil, false
}

func (w *devWalker) walkList(v reflect.Value) (string, reflect.Type, bool) {
	if k := devKeepOf(v.Type().Elem()); !k.walk {
		return "", nil, false
	}
	for i := range v.Len() {
		if bad, bt, s := w.walk(v.Index(i)); bad != "" {
			return bad, bt, s
		}
	}
	return "", nil, false
}
