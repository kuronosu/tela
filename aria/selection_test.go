package aria_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kuronosu/tela/aria"
)

func TestSelection(t *testing.T) {
	s := aria.SelectionOf("b", "a", "b")
	if !s.IsSet() || s.IsAll() || s.IsEmpty() || s.Len() != 2 {
		t.Errorf("SelectionOf: set %v all %v empty %v len %d", s.IsSet(), s.IsAll(), s.IsEmpty(), s.Len())
	}
	if !s.Has("a") || s.Has("c") {
		t.Error("Has")
	}
	if got := s.Keys(); !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Errorf("Keys %v (insertion order, no duplicates)", got)
	}
	if got := s.In([]string{"a", "b", "c"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("In %v", got)
	}
	if k, ok := s.First(); k != "b" || !ok {
		t.Errorf("First %q %v", k, ok)
	}

	// Immutable: the methods return new values.
	w := s.With("c")
	wo := s.Without("a")
	if s.Len() != 2 || w.Len() != 3 || wo.Len() != 1 || !w.Has("c") || wo.Has("a") {
		t.Errorf("With/Without changed the receiver or got it wrong: %v %v %v", s, w, wo)
	}
	if got := s.Toggle("a"); got.Has("a") || !s.Toggle("z").Has("z") {
		t.Errorf("Toggle %v", got)
	}
	keys := s.Keys()
	keys[0] = "changed"
	if !s.Has("b") {
		t.Error("Keys returned the internal slice")
	}

	// The zero Selection is "not given"; the empty one is a value.
	var zero aria.Selection[string]
	empty := aria.SelectionOf[string]()
	if zero.IsSet() || !empty.IsSet() || !zero.IsEmpty() || !empty.IsEmpty() || zero.Has("a") || zero.Len() != 0 {
		t.Error("zero and empty selections")
	}
	if zero.Equal(empty) || !zero.Equal(zero) || !empty.Equal(aria.SelectionOf[string]()) {
		t.Error("zero is not equal to empty")
	}
	if got := zero.With("a"); !got.IsSet() || !got.Has("a") {
		t.Errorf("zero.With %v", got)
	}

	// Equality is by keys, whatever their order; == is by identity.
	if !aria.SelectionOf("a", "b").Equal(aria.SelectionOf("b", "a")) || aria.SelectionOf("a").Equal(aria.SelectionOf("a", "b")) {
		t.Error("Equal")
	}
	if same := s; same != s || s == aria.SelectionOf("b", "a") {
		t.Error("== compares identity")
	}
	m := map[aria.Selection[string]]bool{s: true} // usable as a map key
	if !m[s] {
		t.Error("map key")
	}

	// All.
	all := aria.All[string]()
	if !all.IsAll() || all.IsEmpty() || !all.Has("anything") || all.Len() != 0 || all.Keys() != nil {
		t.Error("All")
	}
	if !all.Equal(aria.All[string]()) || all.Equal(aria.SelectionOf("a")) {
		t.Error("All equality")
	}
	order := []string{"x", "y", "z"}
	if got := all.In(order); !reflect.DeepEqual(got, order) {
		t.Errorf("All.In %v", got)
	}
	if got := all.Resolve(order); got.IsAll() || got.Len() != 3 || !got.Has("y") {
		t.Errorf("Resolve %v", got)
	}
	if got := all.Without("x"); !got.IsAll() {
		t.Error("Without on All is a no-op; resolve first")
	}
	if got := all.Resolve(order).Without("x"); got.Has("x") || got.Len() != 2 {
		t.Errorf("Resolve then Without %v", got)
	}
	if got := s.Resolve(order); !got.Equal(s) {
		t.Error("Resolve of an explicit selection")
	}

	for sel, want := range map[string]string{
		zero.String(): "unset", empty.String(): "{}", all.String(): "all", s.String(): "{b a}",
	} {
		if sel != want {
			t.Errorf("String %q, want %q", sel, want)
		}
	}
}

func ExampleSelection() {
	sel := aria.SelectionOf("banana", "cherry")
	fmt.Println(sel.Has("banana"), sel.Has("apple"))
	fmt.Println(sel.With("apple").Without("cherry"))
	// Keys listed in the order of a collection:
	fmt.Println(sel.In([]string{"apple", "banana", "cherry", "date"}))
	// React Aria's "all", resolved against the keys:
	fmt.Println(aria.All[string]().Resolve([]string{"apple", "banana"}))
	// Output:
	// true false
	// {banana apple}
	// [banana cherry]
	// {apple banana}
}
