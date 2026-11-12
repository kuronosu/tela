package core

import (
	"reflect"
	"testing"

	"github.com/kuronosu/tela/internal/semantics"
)

// TestSemNodeEqualCoversEveryField: changing any field of a node makes it
// unequal to the original for semNodeEqual (children by pointer), so a
// field added to semantics.Node but not to semNodeEqual fails here.
func TestSemNodeEqualCoversEveryField(t *testing.T) {
	kid := &semantics.Node{ID: 9}
	base := semantics.Node{ID: 1, Kind: semantics.KindElement, Selection: &semantics.TextSelection{Anchor: 1, Focus: 2},
		Children: []*semantics.Node{kid}}
	same := base
	same.Selection = &semantics.TextSelection{Anchor: 1, Focus: 2}
	if !semNodeEqual(&base, &same) {
		t.Fatal("a node is not equal to its copy")
	}
	typ := reflect.TypeOf(base)
	for i := range typ.NumField() {
		c := base
		c.Selection = &semantics.TextSelection{Anchor: 1, Focus: 2}
		f := reflect.ValueOf(&c).Elem().Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("x")
		case reflect.Bool:
			f.SetBool(!f.Bool())
		case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			f.SetUint(f.Uint() + 1)
		case reflect.Int:
			f.SetInt(f.Int() + 1)
		case reflect.Struct:
			g := f.Field(0)
			switch g.Kind() {
			case reflect.Float32, reflect.Float64:
				g.SetFloat(g.Float() + 1)
			default:
				t.Fatalf("field %s: unexpected struct field kind %v", typ.Field(i).Name, g.Kind())
			}
		case reflect.Pointer:
			c.Selection = &semantics.TextSelection{Anchor: 5}
		case reflect.Slice:
			c.Children = []*semantics.Node{{ID: 9}}
		default:
			t.Fatalf("field %s of kind %v: extend this test", typ.Field(i).Name, f.Kind())
		}
		if semNodeEqual(&base, &c) {
			t.Errorf("semNodeEqual misses field %s", typ.Field(i).Name)
		}
	}
}
