package tela_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/platform"
)

// constsOfType returns the string constants of type typ declared in a Go
// file, by name.
func constsOfType(t *testing.T, path, typ string) map[string]string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, s := range g.Specs {
			vs := s.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != typ {
				continue
			}
			for i, n := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				out[n.Name] = v
			}
		}
	}
	return out
}

// TestKeyConstantsMatchPlatform checks that tela has a Key constant for
// every named key of the platform layer and a Code constant for every
// physical key code, with the W3C value, and nothing else.
func TestKeyConstantsMatchPlatform(t *testing.T) {
	keys := constsOfType(t, "keys_gen.go", "Key")
	platKeys := constsOfType(t, "internal/platform/key.go", "Key")
	if len(platKeys) < 150 {
		t.Fatalf("found %d platform keys; the parse is broken", len(platKeys))
	}
	for name, v := range platKeys {
		if keys[name] != v {
			t.Errorf("platform.%s = %q has no tela.%s constant with that value (got %q)", name, v, name, keys[name])
		}
	}
	for name, v := range keys {
		if !platform.Key(v).IsNamed() {
			t.Errorf("tela.%s = %q is not a platform named key", name, v)
		}
		if name != "Key"+v {
			t.Errorf("tela.%s has the value %q", name, v)
		}
	}
	codes := constsOfType(t, "keys_gen.go", "Code")
	n := 0
	for c := platform.KeyCode(1); c.Identified(); c++ {
		n++
		name := "Code" + c.String()
		if codes[name] != c.String() {
			t.Errorf("platform code %q has no tela.%s constant", c.String(), name)
		}
	}
	if len(codes) != n+1 { // and CodeUnidentified
		t.Errorf("tela has %d Code constants, the platform %d identified codes plus Unidentified", len(codes), n)
	}
	if codes["CodeUnidentified"] != "Unidentified" {
		t.Error("tela.CodeUnidentified missing")
	}
}

// TestKeyComparisons checks that keys and codes compare with constants
// and with string literals alike.
func TestKeyComparisons(t *testing.T) {
	e := tela.KeyboardEvent{Key: "Enter", Code: tela.CodeKeyA, Location: tela.KeyLocationNumpad}
	if e.Key != tela.KeyEnter || e.Code != "KeyA" || tela.Key(" ") != " " {
		t.Error("string comparisons")
	}
	if e.Location.String() != "numpad" {
		t.Errorf("location %v", e.Location)
	}
}
