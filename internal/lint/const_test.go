package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A class string held in a constant is reported where the constant is
// declared, at the class, whoever uses it; the literal is a class string
// for the editors.
func TestConstClassIsReportedAtItsDeclaration(t *testing.T) {
	parallelLint(t)
	needGo(t)
	res := lintDirs(t, false, "constdecl", "constuse")
	const file = "testdata/constdecl/constdecl.go"
	if len(res.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want one, at the constant", res.Diagnostics)
	}
	d := res.Diagnostics[0]
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	line := 0
	col := 0
	for i, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "const Card") {
			line, col = i+1, strings.Index(l, "rounded-xxl")+1
		}
	}
	if d.File != file || d.Line != line || d.Col != col || d.Suggestion != "rounded-2xl" {
		t.Errorf("finding %+v, want %s:%d:%d with the suggestion rounded-2xl", d, file, line, col)
	}
	found := false
	for _, cs := range res.ClassStrings {
		if cs.File == file && string(data[cs.Start:cs.End]) == `"flex rounded-xxl"` {
			found = true
		}
	}
	if !found {
		t.Errorf("ClassStrings %v lacks the literal of the constant", res.ClassStrings)
	}
}

// A constant of a package that is not linted (its source is not read) is
// reported where it is used.
func TestConstClassOfAnotherPackageIsReportedAtTheUse(t *testing.T) {
	parallelLint(t)
	needGo(t)
	res := lintDirs(t, false, "constuse")
	if len(res.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %v, want one, at the use", res.Diagnostics)
	}
	if d := res.Diagnostics[0]; d.File != "testdata/constuse/constuse.go" || d.Rule != RuleClass {
		t.Errorf("finding %v, want a class finding in constuse.go", d)
	}
}

// Aliases that another package of the module declares are known when the
// run is given the module as its context, without linting it.
func TestContextPackagesDeclareAliases(t *testing.T) {
	parallelLint(t)
	needGo(t)
	res, err := Run(Options{Patterns: []string{"./testdata/ctx/ui"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) != 1 || !strings.Contains(res.Diagnostics[0].Message, `unknown class "ctx-btn"`) {
		t.Fatalf("without the context: %v, want the alias unknown", res.Diagnostics)
	}
	res, err = Run(Options{Patterns: []string{"./testdata/ctx/ui"}, Context: []string{"./testdata/ctx/..."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) > 0 || len(res.Diagnostics) != 0 {
		t.Errorf("with the context: %v %v, want nothing", res.Diagnostics, res.LoadErrors)
	}
	if res.Packages != 1 {
		t.Errorf("Packages = %d, want 1: the context is not linted", res.Packages)
	}
	if _, ok := res.Parser.Aliases()["ctx-btn"]; !ok {
		t.Errorf("the parser does not know the alias: %v", res.Parser.Aliases())
	}
}

// A buffer's own constant is a class string while the buffer is unsaved.
func TestConstClassInOverlay(t *testing.T) {
	parallelLint(t)
	needGo(t)
	dir, err := filepath.Abs(filepath.Join("testdata", "overlay"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "buffer.go")
	src := `package overlay

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

const card = "flex p-4 bg-rde-500"

func Buffer() tela.Node {
	return h.Div(h.Class(card))
}
`
	res, err := Run(Options{Dir: dir, Patterns: []string{"."}, AbsPaths: true, Overlay: map[string][]byte{file: []byte(src)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Diagnostics) != 1 || res.Diagnostics[0].Line != 8 || res.Diagnostics[0].Suggestion != "bg-red-500" {
		t.Fatalf("diagnostics = %v, want one at line 8", res.Diagnostics)
	}
	if len(res.ClassStrings) != 1 || src[res.ClassStrings[0].Start:res.ClassStrings[0].End] != `"flex p-4 bg-rde-500"` {
		t.Errorf("ClassStrings = %v, want the literal of the constant", res.ClassStrings)
	}
}
