// Portions of this file follow react-stately's Selection and
// SelectionManager (https://github.com/adobe/react-spectrum,
// packages/@react-stately/selection), Copyright 2020 Adobe, licensed under
// the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"fmt"
	"strings"
)

// Selection is an immutable set of keys, the selection of the collection
// hooks ([UseListBox], [UseMenu] and [UseTabs]): an explicit set of keys
// or [All], React Aria's "all". Its methods never change the receiver, so
// a Selection can be kept in tela.UseState and shared freely.
//
//	sel := aria.SelectionOf("b", "c")
//	sel.Has("b")              // true
//	sel.With("a").Keys()      // [b c a]
//	sel.Without("b").Keys()   // [c]
//
// The zero Selection is "not given": a hook option left at it is
// uncontrolled, and the hook keeps the selection itself. Pass
// [SelectionOf] with no keys for a controlled, empty selection:
//
//	// Uncontrolled: the hook keeps the selection.
//	aria.ListBoxOptions[string]{Keys: keys, SelectionMode: "multiple", DefaultSelected: aria.SelectionOf("b")}
//	// Controlled: the app keeps it.
//	sel, setSel := tela.UseState(aria.SelectionOf("b"))
//	aria.ListBoxOptions[string]{Keys: keys, SelectionMode: "multiple", Selected: sel, OnSelectionChange: setSel.Set}
//
// A Selection is comparable with == (it holds a pointer), but two
// selections with the same keys are not == unless they are the same value:
// use [Selection.Equal] to compare keys.
type Selection[K comparable] struct{ s *selectionSet[K] }

type selectionSet[K comparable] struct {
	all  bool
	keys []K // in the order they were added
	set  map[K]struct{}
}

// SelectionOf returns the selection of exactly the given keys (duplicates
// count once). With no keys it is the empty selection, which, unlike the
// zero Selection, is a value: it makes a controlled hook show nothing
// selected.
func SelectionOf[K comparable](keys ...K) Selection[K] {
	s := &selectionSet[K]{set: make(map[K]struct{}, len(keys))}
	for _, k := range keys {
		s.add(k)
	}
	return Selection[K]{s}
}

// All returns the selection of every key that can be selected: React
// Aria's "all". A collection hook shows every enabled key selected; its
// keys are not listed in the value, so [Selection.Len] is 0 and
// [Selection.Keys] empty for it: use [Selection.Resolve] to turn it into
// the explicit keys of a collection.
func All[K comparable]() Selection[K] { return Selection[K]{&selectionSet[K]{all: true}} }

func (s *selectionSet[K]) add(k K) {
	if _, ok := s.set[k]; ok {
		return
	}
	s.set[k] = struct{}{}
	s.keys = append(s.keys, k)
}

func (s *selectionSet[K]) clone() *selectionSet[K] {
	c := &selectionSet[K]{all: s.all, keys: append([]K(nil), s.keys...), set: make(map[K]struct{}, len(s.keys)+1)}
	for _, k := range s.keys {
		c.set[k] = struct{}{}
	}
	return c
}

// IsSet reports whether s is a selection. The zero Selection is not: it
// means "none given" in the options of the hooks (uncontrolled).
func (s Selection[K]) IsSet() bool { return s.s != nil }

// IsAll reports whether s is [All].
func (s Selection[K]) IsAll() bool { return s.s != nil && s.s.all }

// IsEmpty reports whether no key is selected: the zero Selection and
// [SelectionOf] with no keys. [All] is not empty.
func (s Selection[K]) IsEmpty() bool { return s.s == nil || !s.s.all && len(s.s.keys) == 0 }

// Has reports whether k is selected. For [All] it is always true (a hook
// still never selects a disabled key).
func (s Selection[K]) Has(k K) bool {
	if s.s == nil {
		return false
	}
	if s.s.all {
		return true
	}
	_, ok := s.s.set[k]
	return ok
}

// Len returns the number of keys of an explicit selection. [All] has no
// count of its own and returns 0: use len(s.In(keys)) for the number of
// selected items, or resolve it with [Selection.Resolve].
func (s Selection[K]) Len() int {
	if s.s == nil {
		return 0
	}
	return len(s.s.keys)
}

// Keys returns the keys of an explicit selection in the order they were
// added (nil for [All] and the zero Selection). The slice is a copy. Use
// [Selection.In] to list them in the order of a collection.
func (s Selection[K]) Keys() []K {
	if s.s == nil || len(s.s.keys) == 0 {
		return nil
	}
	return append([]K(nil), s.s.keys...)
}

// In returns the selected keys among order, in the order of order: the
// selection of a collection listed in the order of its keys. For [All] it
// is every key of order.
func (s Selection[K]) In(order []K) []K {
	var out []K
	for _, k := range order {
		if s.Has(k) {
			out = append(out, k)
		}
	}
	return out
}

// First returns the first key of an explicit selection, in the order the
// keys were added, and whether there is one: the key of a single
// selection.
func (s Selection[K]) First() (K, bool) {
	if s.s != nil && len(s.s.keys) > 0 {
		return s.s.keys[0], true
	}
	var zero K
	return zero, false
}

// Resolve returns s as an explicit selection of keys among order: for
// [All] the keys of order, for an explicit selection s itself.
func (s Selection[K]) Resolve(order []K) Selection[K] {
	if s.IsAll() {
		return SelectionOf(order...)
	}
	return s
}

// With returns s with the keys added. Adding to [All] returns it as is.
func (s Selection[K]) With(keys ...K) Selection[K] {
	if s.IsAll() {
		return s
	}
	c := &selectionSet[K]{set: map[K]struct{}{}}
	if s.s != nil {
		c = s.s.clone()
	}
	for _, k := range keys {
		c.add(k)
	}
	return Selection[K]{c}
}

// Without returns s with the keys removed. Removing from [All] returns it
// as is, since it does not list its keys: resolve it first.
func (s Selection[K]) Without(keys ...K) Selection[K] {
	if s.IsAll() {
		return s
	}
	c := &selectionSet[K]{set: map[K]struct{}{}}
	if s.s != nil {
		c = s.s.clone()
	}
	drop := make(map[K]struct{}, len(keys))
	for _, k := range keys {
		drop[k] = struct{}{}
	}
	var kept []K
	for _, k := range c.keys {
		if _, d := drop[k]; d {
			delete(c.set, k)
			continue
		}
		kept = append(kept, k)
	}
	c.keys = kept
	return Selection[K]{c}
}

// Toggle returns s without k when it has it and with k when it does not.
func (s Selection[K]) Toggle(k K) Selection[K] {
	if s.Has(k) {
		return s.Without(k)
	}
	return s.With(k)
}

// Equal reports whether s and o select the same keys, whatever their
// order: two [All] are equal, an [All] and an explicit selection are not,
// and the zero Selection equals only itself.
func (s Selection[K]) Equal(o Selection[K]) bool {
	switch {
	case s.s == nil || o.s == nil:
		return s.s == o.s
	case s.s == o.s:
		return true
	case s.s.all || o.s.all:
		return s.s.all && o.s.all
	case len(s.s.keys) != len(o.s.keys):
		return false
	}
	for _, k := range s.s.keys {
		if _, ok := o.s.set[k]; !ok {
			return false
		}
	}
	return true
}

// String returns "all", "unset" for the zero Selection, or the keys in
// braces.
func (s Selection[K]) String() string {
	switch {
	case s.s == nil:
		return "unset"
	case s.s.all:
		return "all"
	}
	parts := make([]string, len(s.s.keys))
	for i, k := range s.s.keys {
		parts[i] = fmt.Sprint(k)
	}
	return "{" + strings.Join(parts, " ") + "}"
}
