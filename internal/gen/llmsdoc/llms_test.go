package llmsdoc

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/lint"
)

const (
	root       = "../../.."
	docPath    = root + "/docs/llms.txt"
	examplePkg = root + "/examples/llms/settings"

	lucidePath      = "github.com/kuronosu/tela/icons/lucide"
	localLucidePath = "github.com/kuronosu/tela/examples/llms/settings/lucide"

	// settingsFirstLine starts the block that shows the example's main.go.
	settingsFirstLine = "// Command settings is the worked example of docs/llms.txt, which shows this"

	// maxBytes keeps the file short enough to be read whole (~25k tokens).
	maxBytes = 100_000
)

// block is one ```go block of llms.txt.
type block struct {
	line int // line of llms.txt where the code starts (1-based)
	code string
}

func readDoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// goBlocks returns the ```go blocks of the document.
func goBlocks(doc string) []block {
	var out []block
	lines := strings.Split(doc, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "```go" {
			continue
		}
		start := i + 1
		j := start
		for j < len(lines) && strings.TrimSpace(lines[j]) != "```" {
			j++
		}
		out = append(out, block{line: start + 1, code: strings.Join(lines[start:j], "\n")})
		i = j
	}
	return out
}

// header is what a declarations or statements block is put after: the
// usual imports, each used once.
const header = `package snippet

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"

	"` + localLucidePath + `"
)

var (
	_ = cmp.Compare[int]
	_ context.Context
	_ = errors.New
	_ = fmt.Sprint
	_ = strconv.Itoa
	_ = strings.Contains
	_ *testing.T
	_ time.Duration
	_ tela.Node
	_ aria.Date
	_ = Div
	_ = i18n.UseLocale
	_ kit.Button
	_ *telatest.App
	_ = lucide.Check
)

`

// source turns a block into a Go file, and returns how many lines come
// before the block's first line in it.
func source(b block) (string, int, error) {
	code := strings.ReplaceAll(b.code, strconv.Quote(lucidePath), strconv.Quote(localLucidePath))
	fset := token.NewFileSet()
	if f, err := parser.ParseFile(fset, "", code, parser.PackageClauseOnly); err == nil && f.Name != nil {
		return code, 0, nil
	}
	if strings.HasPrefix(code, "import") {
		return importsOnly(code)
	}
	pre := strings.Count(header, "\n")
	decls := header + code + "\n"
	if _, err := parser.ParseFile(fset, "", decls, 0); err == nil {
		return decls, pre, nil
	}
	open := header + "func snippetBody() tela.Node {\n"
	body := open + code + "\n"
	f, err := parser.ParseFile(fset, "", body+"}\n", 0)
	if err != nil {
		return "", 0, fmt.Errorf("neither a file, declarations nor statements: %v", err)
	}
	fn := f.Decls[len(f.Decls)-1].(*ast.FuncDecl)
	if n := len(fn.Body.List); n == 0 {
		body += "return nil\n"
	} else if _, ok := fn.Body.List[n-1].(*ast.ReturnStmt); !ok {
		body += "return nil\n"
	}
	return body + "}\n", strings.Count(open, "\n"), nil
}

// uses are blank uses of the packages a block may import, so that a block
// of imports alone compiles.
var uses = map[string]string{
	"github.com/kuronosu/tela":          "_ tela.Node",
	"github.com/kuronosu/tela/h":        "_ = Div",
	"github.com/kuronosu/tela/kit":      "_ kit.Button",
	"github.com/kuronosu/tela/aria":     "_ aria.Date",
	"github.com/kuronosu/tela/i18n":     "_ = i18n.UseLocale",
	"github.com/kuronosu/tela/telatest": "_ *telatest.App",
	localLucidePath:                     "_ = lucide.Check",
}

// importsOnly makes a file of a block that is only an import declaration.
func importsOnly(code string) (string, int, error) {
	src := "package snippet\n\n" + code + "\n"
	f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.ImportsOnly)
	if err != nil {
		return "", 0, err
	}
	src += "\nvar (\n"
	for _, im := range f.Imports {
		path, _ := strconv.Unquote(im.Path.Value)
		use, ok := uses[path]
		if !ok {
			return "", 0, fmt.Errorf("an import block may import only %v", sortedKeys(uses))
		}
		src += "\t" + use + "\n"
	}
	return src + ")\n", 2, nil
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var snippetRef = regexp.MustCompile(`(?:[^\s:]*/)?s\d+/snippet(\d+)\.go:(\d+)(:\d+)?`)

// TestSnippets type-checks every Go block and lints it as `tela lint`
// does. A finding names the line of llms.txt. The files of one package
// other than main (package notes) are one package, so a block can use
// what an earlier block of that package declares.
func TestSnippets(t *testing.T) {
	needGo(t)
	if testing.Short() {
		t.Skip("type-checks the module's packages from source")
	}
	blocks := goBlocks(readDoc(t))
	if len(blocks) == 0 {
		t.Fatal("no ```go block in docs/llms.txt")
	}
	dir, err := os.MkdirTemp("testdata", "snippets-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	offsets := map[int]int{} // block index -> llms.txt line of the file's line 1
	pkgs := map[string]int{} // package name -> the block whose directory holds it
	var patterns []string
	for i, b := range blocks {
		src, pre, err := source(b)
		if err != nil {
			t.Errorf("docs/llms.txt:%d: %v", b.line, err)
			continue
		}
		at := i
		if f, err := parser.ParseFile(token.NewFileSet(), "", src, parser.PackageClauseOnly); err == nil {
			if name := f.Name.Name; name != "main" && name != "snippet" {
				if j, ok := pkgs[name]; ok {
					at = j
				} else {
					pkgs[name] = i
				}
			}
		}
		pkg := filepath.Join(abs, fmt.Sprintf("s%02d", at))
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkg, fmt.Sprintf("snippet%02d.go", i)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		offsets[i] = b.line - pre
		if at == i {
			patterns = append(patterns, pkg)
		}
	}
	modRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := lint.Run(lint.Options{Dir: modRoot, Patterns: patterns})
	if err != nil {
		t.Fatal(err)
	}
	// where rewrites the position in a generated file to llms.txt's.
	where := func(s string) string {
		return snippetRef.ReplaceAllStringFunc(s, func(m string) string {
			sub := snippetRef.FindStringSubmatch(m)
			i, _ := strconv.Atoi(sub[1])
			line, _ := strconv.Atoi(sub[2])
			return fmt.Sprintf("docs/llms.txt:%d%s", offsets[i]+line-1, sub[3])
		})
	}
	for _, e := range res.LoadErrors {
		t.Errorf("does not compile: %s", where(e))
	}
	for _, d := range res.Diagnostics {
		t.Errorf("tela lint: %s", where(d.String()))
	}
	if res.Packages+len(res.LoadErrors) < len(patterns) {
		t.Errorf("linted %d packages and %d failed, of %d blocks", res.Packages, len(res.LoadErrors), len(patterns))
	}
}

// TestSettingsExampleIsVerbatim: the settings screen of llms.txt is the
// main.go of examples/llms/settings, which imports the subset of Lucide
// where llms.txt imports the lucide module.
func TestSettingsExampleIsVerbatim(t *testing.T) {
	var shown *block
	blocks := goBlocks(readDoc(t))
	for i, b := range blocks {
		if strings.HasPrefix(b.code, settingsFirstLine+"\n") {
			if shown != nil {
				t.Fatalf("two blocks start with %q", settingsFirstLine)
			}
			shown = &blocks[i]
		}
	}
	if shown == nil {
		t.Fatalf("no ```go block starts with %q", settingsFirstLine)
	}
	src, err := os.ReadFile(filepath.Join(examplePkg, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(src), strconv.Quote(localLucidePath), strconv.Quote(lucidePath), 1)
	if got := shown.code + "\n"; got != want {
		gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
		for i := 0; i < len(gl) && i < len(wl); i++ {
			if gl[i] != wl[i] {
				t.Fatalf("docs/llms.txt:%d differs from examples/llms/settings/main.go:%d:\n got: %q\nwant: %q", shown.line+i, i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("docs/llms.txt:%d: the block has %d lines, main.go %d", shown.line, len(gl), len(wl))
	}
}

// kitTypes returns the exported struct types of package kit with their
// fields, and the names of those that are struct components (a Build
// method).
func kitTypes(t *testing.T) (fields map[string]map[string]bool, components []string) {
	t.Helper()
	fset := token.NewFileSet()
	matches, err := filepath.Glob(filepath.Join(root, "kit", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	fields = map[string]map[string]bool{}
	build := map[string]bool{}
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.GenDecl:
				for _, s := range d.Specs {
					ts, ok := s.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					set := map[string]bool{}
					for _, fl := range st.Fields.List {
						for _, n := range fl.Names {
							set[n.Name] = true
						}
					}
					fields[ts.Name.Name] = set
				}
			case *ast.FuncDecl:
				if d.Name.Name != "Build" || d.Recv == nil || len(d.Recv.List) != 1 {
					continue
				}
				typ := d.Recv.List[0].Type
				if ix, ok := typ.(*ast.IndexExpr); ok {
					typ = ix.X
				}
				if ix, ok := typ.(*ast.IndexListExpr); ok {
					typ = ix.X
				}
				if id, ok := typ.(*ast.Ident); ok && id.IsExported() {
					build[id.Name] = true
				}
			}
		}
	}
	for name := range build {
		if fields[name] != nil {
			components = append(components, name)
		}
	}
	sort.Strings(components)
	return fields, components
}

var catalogLine = regexp.MustCompile("^- `kit\\.([A-Z]\\w*)(?:\\[\\w+\\])?\\{([^}`]*)\\}`")

// TestKitCatalog: every struct component of kit is named in llms.txt, and
// the catalog lines (- `kit.Name{Field, Field}`) name fields that exist.
func TestKitCatalog(t *testing.T) {
	doc := readDoc(t)
	fields, components := kitTypes(t)
	if len(components) < 30 {
		t.Fatalf("found %d kit components, want more than 30: is the parser right?", len(components))
	}
	for _, c := range components {
		if !regexp.MustCompile(`\bkit\.` + c + `\b`).MatchString(doc) {
			t.Errorf("docs/llms.txt does not mention kit.%s", c)
		}
	}
	listed := map[string]bool{}
	for i, line := range strings.Split(doc, "\n") {
		m := catalogLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		listed[m[1]] = true
		set := fields[m[1]]
		if set == nil {
			t.Errorf("docs/llms.txt:%d: kit.%s is not an exported struct type of kit", i+1, m[1])
			continue
		}
		for _, f := range strings.Split(m[2], ",") {
			f = strings.TrimSpace(f)
			if f != "" && f != "…" && !set[f] {
				t.Errorf("docs/llms.txt:%d: kit.%s has no field %s", i+1, m[1], f)
			}
		}
	}
	for _, c := range components {
		if !listed[c] {
			t.Errorf("the kit catalog has no line - `kit.%s{...}`", c)
		}
	}
}

// TestSize keeps llms.txt short enough to be read whole.
func TestSize(t *testing.T) {
	if n := len(readDoc(t)); n > maxBytes {
		t.Errorf("docs/llms.txt is %d bytes, more than %d (about 25k tokens): make it shorter", n, maxBytes)
	}
}

// needGo skips a test that runs the go command when there is none: under
// Wine, and on a Windows machine without Go, the test binary runs alone.
func needGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is not in PATH")
	}
}
