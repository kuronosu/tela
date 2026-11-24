//go:build linux && (amd64 || arm64)

package vulkan

import (
	"reflect"
	"testing"
)

// cLayout is a struct of the bindings, beside what C says of it.
type cLayout struct {
	v             any
	align         uintptr // Go's
	cSize, cAlign uintptr
	fields        []cField
}

type cField struct {
	name         string
	offset, size uintptr
}

// TestLayout compares the structs with the layout a C compiler gives them in
// the Vulkan headers, member by member.
func TestLayout(t *testing.T) {
	for _, l := range cLayouts {
		gt := reflect.TypeOf(l.v)
		if gt.Size() != l.cSize || l.align != l.cAlign {
			t.Errorf("%s: size %d and alignment %d, in C %d and %d", gt.Name(), gt.Size(), l.align, l.cSize, l.cAlign)
		}
		for _, f := range l.fields {
			gf, ok := gt.FieldByName(f.name)
			switch {
			case !ok:
				t.Errorf("%s has no %s", gt.Name(), f.name)
			case gf.Offset != f.offset || gf.Type.Size() != f.size:
				t.Errorf("%s.%s: offset %d and size %d, in C %d and %d", gt.Name(), f.name, gf.Offset, gf.Type.Size(), f.offset, f.size)
			}
		}
		if n := gt.NumField(); n != len(l.fields) && gt.Name() != "physicalDeviceLimits" {
			t.Errorf("%s: %d fields, in C %d", gt.Name(), n, len(l.fields))
		}
	}
}
