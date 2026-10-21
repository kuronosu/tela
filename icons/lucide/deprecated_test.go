package lucide

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// docIcon is a variable of icons_gen.go with what its documentation says.
type docIcon struct {
	Var  string // the Go variable
	File string // the Lucide file name ("alert-circle"), from "X is the icon "alert-circle"."
	Use  string // the variable named by the "Deprecated: use X." paragraph, or ""
}

var (
	isTheIcon = regexp.MustCompile(`^(\w+) is the icon ("[^"]+")\.$`)
	deprecate = regexp.MustCompile(`^Deprecated: use (\w+)\.$`)
)

func readDocs(t *testing.T) map[string]docIcon {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "icons_gen.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]docIcon{}
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		name := gd.Specs[0].(*ast.ValueSpec).Names[0].Name
		if gd.Doc == nil {
			t.Errorf("%s has no documentation", name)
			continue
		}
		paras := strings.Split(strings.TrimSpace(gd.Doc.Text()), "\n\n")
		m := isTheIcon.FindStringSubmatch(paras[0])
		if m == nil || m[1] != name {
			t.Errorf("%s: unexpected documentation %q", name, paras[0])
			continue
		}
		file, err := strconv.Unquote(m[2])
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		ic := docIcon{Var: name, File: file}
		for _, p := range paras[1:] {
			if m := deprecate.FindStringSubmatch(strings.TrimSpace(p)); m != nil {
				ic.Use = m[1]
			} else {
				t.Errorf("%s: unexpected paragraph %q", name, p)
			}
		}
		out[name] = ic
	}
	return out
}

// Lucide keeps the names of the icons it renamed; each of them is generated
// with a "Deprecated: use X." paragraph so that gopls and staticcheck flag
// its uses. deprecated.json (from gen_deprecated.go) says which they are.
func TestDeprecatedNamesPointAtTheCurrentIcon(t *testing.T) {
	data, err := os.ReadFile("deprecated.json")
	if err != nil {
		t.Fatal(err)
	}
	var list map[string]string
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) < 200 {
		t.Fatalf("deprecated.json has %d names, want at least 200", len(list))
	}
	docs := readDocs(t)
	byFile := map[string]docIcon{}
	for _, ic := range docs {
		byFile[ic.File] = ic
	}

	for old, current := range list {
		ic, ok := byFile[old]
		if !ok {
			t.Errorf("deprecated.json names %q, which is not an icon of icons_gen.go; regenerate it (see README.md)", old)
			continue
		}
		if ic.Use == "" {
			t.Errorf("%s (%q) is in deprecated.json but has no \"Deprecated: use X.\" comment", ic.Var, old)
			continue
		}
		use, ok := docs[ic.Use]
		if !ok {
			t.Errorf("%s: deprecated in favor of %s, which is not declared", ic.Var, ic.Use)
			continue
		}
		if use.File != current {
			t.Errorf("%s: deprecated in favor of %s (%q), deprecated.json says %q", ic.Var, use.Var, use.File, current)
		}
		if use.Use != "" {
			t.Errorf("%s: deprecated in favor of %s, which is deprecated too", ic.Var, use.Var)
		}
	}
	// And no icon carries the comment without being in the list.
	n := 0
	for _, ic := range docs {
		if ic.Use != "" {
			n++
			if _, ok := list[ic.File]; !ok {
				t.Errorf("%s is deprecated in icons_gen.go but not in deprecated.json", ic.Var)
			}
		}
	}
	if n != len(list) {
		t.Errorf("%d deprecated variables, %d names in deprecated.json", n, len(list))
	}
	// Known renames.
	for old, use := range map[string]string{"AlertCircle": "CircleAlert", "Axis3D": "Axis3d", "ActivitySquare": "SquareActivity"} {
		if docs[old].Use != use {
			t.Errorf("%s is deprecated in favor of %q, want %q", old, docs[old].Use, use)
		}
	}
	if docs["CircleAlert"].Use != "" || docs["Check"].Use != "" {
		t.Error("a current icon is marked deprecated")
	}
}
