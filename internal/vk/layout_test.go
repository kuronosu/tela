//go:build telagpu && (linux || windows)

package vk

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"
)

// The types of layout_table_test.go, which internal/layoutgen writes from
// the real Vulkan headers.

// structLayout is a C struct or union with its size and alignment, beside
// the Go type that stands for it.
type structLayout struct {
	name          string // C name
	goType        reflect.Type
	cSize, cAlign uintptr
	union         bool // only size and alignment compare
	fields        []fieldLayout
}

// fieldLayout is a member of a C struct, beside the Go field of the same
// name.
type fieldLayout struct {
	name             string // C name
	goOffset, goSize uintptr
	cOffset, cSize   uintptr
}

// constValue is a C constant beside the Go one, both widened to 64 bits
// with their sign.
type constValue struct {
	name      string // C name
	got, want uint64
}

// stringConst is a C string constant beside the Go one.
type stringConst struct {
	name, got, want string
}

// u64 widens a constant as the C program does: through the signed 64-bit
// value of a signed one.
func u64[T ~int | ~int32 | ~uint32 | ~uint64](v T) uint64 { return uint64(v) }

const regenerate = "run `go generate ./internal/vk` (needs the Vulkan headers) and check the diff"

// TestLayout compares every struct of the bindings with the layout the C
// compiler gives the struct of the Vulkan headers: size, alignment, and
// the offset and size of each member. The member lists are the header's,
// so a missing or misplaced field fails too.
func TestLayout(t *testing.T) {
	for _, s := range layoutTable {
		gt := s.goType
		if gt.Size() != s.cSize || uintptr(gt.Align()) != s.cAlign {
			t.Errorf("%s: Go size %d and alignment %d, C size %d and alignment %d", s.name, gt.Size(), gt.Align(), s.cSize, s.cAlign)
		}
		if s.union {
			continue
		}
		fields := 0
		for i := range gt.NumField() {
			if gt.Field(i).Name != "_" {
				fields++
			}
		}
		if fields != len(s.fields) {
			t.Errorf("%s: %d Go fields, %d C members", s.name, fields, len(s.fields))
		}
		end := uintptr(0)
		for _, f := range s.fields {
			if f.goOffset != f.cOffset || f.goSize != f.cSize {
				t.Errorf("%s.%s: Go offset %d and size %d, C offset %d and size %d", s.name, f.name, f.goOffset, f.goSize, f.cOffset, f.cSize)
			}
			if f.cOffset < end {
				t.Errorf("%s.%s: overlaps the member before it", s.name, f.name)
			}
			end = f.cOffset + f.cSize
		}
	}
}

// TestConstants compares every constant of the bindings with the value of
// the C constant it names.
func TestConstants(t *testing.T) {
	for _, c := range constTable {
		if c.got != c.want {
			t.Errorf("%s: Go %#x, C %#x", c.name, c.got, c.want)
		}
	}
	for _, c := range stringConstTable {
		if c.got != c.want {
			t.Errorf("%s: Go %q, C %q", c.name, c.got, c.want)
		}
	}
}

// TestLayoutTableCurrent fails when structs.go or enums.go has something
// the checked-in table does not: the table was not regenerated.
func TestLayoutTableCurrent(t *testing.T) {
	inTable := map[string]bool{}
	for _, s := range layoutTable {
		inTable[s.name] = true
	}
	types := 0
	for _, decl := range parseDecls(t, "structs.go", token.TYPE) {
		for _, spec := range decl.Specs {
			types++
			if name := "Vk" + spec.(*ast.TypeSpec).Name.Name; !inTable[name] {
				t.Errorf("structs.go: %s is not in the layout table; %s", name, regenerate)
			}
		}
	}
	if types != len(layoutTable) {
		t.Errorf("structs.go declares %d types and the layout table has %d; %s", types, len(layoutTable), regenerate)
	}

	for _, c := range constTable {
		inTable[c.name] = true
	}
	for _, c := range stringConstTable {
		inTable[c.name] = true
	}
	consts := 0
	for _, decl := range parseDecls(t, "enums.go", token.CONST) {
		for _, spec := range decl.Specs {
			consts++
			vs := spec.(*ast.ValueSpec)
			if cName := strings.TrimSpace(vs.Comment.Text()); !inTable[cName] {
				t.Errorf("enums.go: %s (%q) is not in the constant table; %s", vs.Names[0].Name, cName, regenerate)
			}
		}
	}
	if n := len(constTable) + len(stringConstTable); consts != n {
		t.Errorf("enums.go declares %d constants and the tables have %d; %s", consts, n, regenerate)
	}
}

// parseDecls returns the declarations of one kind in a file of the
// package.
func parseDecls(t *testing.T, name string, kind token.Token) []*ast.GenDecl {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	var out []*ast.GenDecl
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == kind {
			out = append(out, gen)
		}
	}
	return out
}
