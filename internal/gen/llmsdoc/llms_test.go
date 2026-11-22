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
	root      = "../../.."
	docPath   = root + "/docs/llms.txt"
	recipeDir = root + "/docs/llms"

	lucidePath      = "github.com/kuronosu/tela/icons/lucide"
	localLucidePath = "github.com/kuronosu/tela/examples/settings/lucide"

	// settingsFirstLine starts the block of the compact settings screen
	// (llms/settings-screen.md), a whole package main.
	settingsFirstLine = "// Command settings is a settings screen on kit:"

	// coreMaxBytes keeps the core short enough that every agent reads it
	// whole (about 9k tokens): what every screen needs and an index of the
	// recipes (ADR-313).
	coreMaxBytes = 36_000
)

// recipeMaxBytes is the size limit of each recipe, by file name without
// the extension: a recipe is read for one question, so it stays within
// about 7k tokens. A recipe without a row here fails TestSizes, and so does
// a row without a recipe; raising a limit is a trade, named in an ADR
// (ADR-290).
var recipeMaxBytes = map[string]int{
	"accessibility":       7_000,
	"animation":           5_000,
	"components":          8_000,
	"desktop-integration": 19_000,
	"dev":                 5_500,
	"i18n":                7_500,
	"kit":                 24_000,
	"settings-screen":     9_500,
	"styling":             11_000,
	"testing":             7_500,
	"windows":             16_000,
}

// docFile is the core or one recipe.
type docFile struct {
	name string // "llms.txt" or "llms/<recipe>.md", as the document says it
	path string
	text string
}

// readDocs reads the core and the recipes, the core first, the recipes in
// the order of their names.
func readDocs(t *testing.T) []docFile {
	t.Helper()
	core, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	docs := []docFile{{name: "llms.txt", path: docPath, text: string(core)}}
	matches, err := filepath.Glob(filepath.Join(recipeDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(matches)
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		docs = append(docs, docFile{name: "llms/" + filepath.Base(m), path: m, text: string(b)})
	}
	return docs
}

// readRecipe returns one recipe by name, such as "kit".
func readRecipe(t *testing.T, name string) docFile {
	t.Helper()
	for _, d := range readDocs(t) {
		if d.name == "llms/"+name+".md" {
			return d
		}
	}
	t.Fatalf("docs/llms/%s.md does not exist", name)
	return docFile{}
}

// block is one fenced Go block of a document.
type block struct {
	file string // the document's name
	line int    // line of the file where the code starts (1-based)
	code string
}

func (b block) where() string { return fmt.Sprintf("docs/%s:%d", b.file, b.line) }

// fenced returns the blocks of a document that open with "```" and info:
// "go" for the code that must compile, "go mistake" for the wrong code of a
// mistake the guide lists (see TestMistakeSamples).
func fenced(d docFile, info string) []block {
	var out []block
	lines := strings.Split(d.text, "\n")
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "```"+info {
			continue
		}
		start := i + 1
		j := start
		for j < len(lines) && strings.TrimSpace(lines[j]) != "```" {
			j++
		}
		out = append(out, block{file: d.name, line: start + 1, code: strings.Join(lines[start:j], "\n")})
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

// finding is one problem of one block: it does not compile, or tela lint
// reports it.
type finding struct {
	text string
	lint bool // reported by a tela lint rule rather than the compiler
}

// lintBlocks type-checks and lints the blocks as `tela lint` does, written
// as packages under testdata/<prefix>-*/ while it runs, and returns the
// findings by block index (-1: not tied to one block). A finding names the
// line of its document. Blocks of one package other than main (package
// notes) are one package, so a block can use what an earlier block of that
// package declares.
func lintBlocks(t *testing.T, prefix string, blocks []block) map[int][]finding {
	t.Helper()
	dir, err := os.MkdirTemp("testdata", prefix+"-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	got := map[int][]finding{}
	offsets := map[int]int{} // block index -> line of its file at the generated file's line 1
	pkgs := map[string]int{} // package name -> the block whose directory holds it
	var patterns []string
	for i, b := range blocks {
		src, pre, err := source(b)
		if err != nil {
			got[i] = append(got[i], finding{text: fmt.Sprintf("%s: %v", b.where(), err)})
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
	if len(patterns) == 0 {
		return got
	}
	modRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := lint.Run(lint.Options{Dir: modRoot, Patterns: patterns})
	if err != nil {
		t.Fatal(err)
	}
	// place rewrites the position in a generated file to the document's
	// and returns the block it is in.
	place := func(s string) (string, int) {
		idx := -1
		out := snippetRef.ReplaceAllStringFunc(s, func(m string) string {
			sub := snippetRef.FindStringSubmatch(m)
			i, _ := strconv.Atoi(sub[1])
			line, _ := strconv.Atoi(sub[2])
			idx = i
			return fmt.Sprintf("docs/%s:%d%s", blocks[i].file, offsets[i]+line-1, sub[3])
		})
		return out, idx
	}
	for _, e := range res.LoadErrors {
		text, i := place(e)
		got[i] = append(got[i], finding{text: "does not compile: " + text})
	}
	for _, d := range res.Diagnostics {
		text, i := place(d.String())
		got[i] = append(got[i], finding{text: "tela lint: " + text, lint: true})
	}
	if res.Packages+len(res.LoadErrors) < len(patterns) {
		got[-1] = append(got[-1], finding{text: fmt.Sprintf("linted %d packages and %d failed, of %d blocks", res.Packages, len(res.LoadErrors), len(patterns))})
	}
	return got
}

// TestSnippets type-checks every ```go block of the core and the recipes
// and lints it as `tela lint` does.
func TestSnippets(t *testing.T) {
	needGo(t)
	if testing.Short() {
		t.Skip("type-checks the module's packages from source")
	}
	var blocks []block
	for _, d := range readDocs(t) {
		blocks = append(blocks, fenced(d, "go")...)
	}
	if len(blocks) == 0 {
		t.Fatal("no ```go block in docs/llms.txt and docs/llms/")
	}
	got := lintBlocks(t, "snippets", blocks)
	for _, fs := range got {
		for _, f := range fs {
			t.Error(f.text)
		}
	}
}

// TestSettingsScreenRenders mounts the compact settings screen of
// llms/settings-screen.md headlessly, in dev mode, as `tela inspect` does:
// it must start and report no dev message (unknown classes, missing keys,
// misused hooks) on standard error.
func TestSettingsScreenRenders(t *testing.T) {
	needGo(t)
	if testing.Short() {
		t.Skip("builds and runs the tela command")
	}
	var shown *block
	blocks := fenced(readRecipe(t, "settings-screen"), "go")
	for i, b := range blocks {
		if strings.HasPrefix(b.code, settingsFirstLine) {
			if shown != nil {
				t.Fatalf("two blocks start with %q", settingsFirstLine)
			}
			shown = &blocks[i]
		}
	}
	if shown == nil {
		t.Fatalf("no ```go block of llms/settings-screen.md starts with %q", settingsFirstLine)
	}
	if !strings.Contains(shown.code, "\npackage main\n") || !strings.Contains(shown.code, "var App = tela.App{") {
		t.Fatalf("%s: the settings block must be a whole package main with var App", shown.where())
	}
	dir, err := os.MkdirTemp("testdata", "render-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	code := strings.ReplaceAll(shown.code, strconv.Quote(lucidePath), strconv.Quote(localLucidePath))
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(code+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(modRoot, abs)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/tela", "inspect", "--size", "1040x800", "./"+filepath.ToSlash(rel))
	cmd.Dir = modRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("tela inspect failed: %v\n%s", err, stderr.String())
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		t.Errorf("the settings screen of docs/llms/settings-screen.md reports dev messages:\n%s", msg)
	}
	for _, want := range []string{`heading "Settings"`, `switch "Weekly digest"`, `radio "System"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("the aria snapshot of the settings screen lacks %s:\n%s", want, stdout.String())
		}
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

// TestKitCatalog: every struct component of kit is named in the kit section of
// the core and in llms/kit.md, and the catalog lines of that recipe
// (- `kit.Name{Field, Field}`) name fields that exist.
func TestKitCatalog(t *testing.T) {
	core := readDocs(t)[0]
	recipe := readRecipe(t, "kit")
	fields, components := kitTypes(t)
	if len(components) < 30 {
		t.Fatalf("found %d kit components, want more than 30: is the parser right?", len(components))
	}
	_, kitSection, ok := sectionOf(core.text, "kit")
	if !ok {
		t.Fatal(`docs/llms.txt has no "## kit" section`)
	}
	coreKit := strings.Join(kitSection, "\n")
	for _, c := range components {
		if !regexp.MustCompile(`\b` + c + `\b`).MatchString(coreKit) {
			t.Errorf("the kit section of docs/llms.txt does not name %s (the core lists every component)", c)
		}
		if !regexp.MustCompile(`\bkit\.` + c + `\b`).MatchString(recipe.text) {
			t.Errorf("docs/llms/kit.md does not mention kit.%s", c)
		}
	}
	listed := map[string]bool{}
	for i, line := range strings.Split(recipe.text, "\n") {
		m := catalogLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		listed[m[1]] = true
		set := fields[m[1]]
		if set == nil {
			t.Errorf("docs/llms/kit.md:%d: kit.%s is not an exported struct type of kit", i+1, m[1])
			continue
		}
		for _, f := range strings.Split(m[2], ",") {
			f = strings.TrimSpace(f)
			if f != "" && f != "…" && !set[f] {
				t.Errorf("docs/llms/kit.md:%d: kit.%s has no field %s", i+1, m[1], f)
			}
		}
	}
	for _, c := range components {
		if !listed[c] {
			t.Errorf("the kit catalog of docs/llms/kit.md has no line - `kit.%s{...}`", c)
		}
	}
}

// TestSizes keeps the core and each recipe within its budget (ADR-313).
func TestSizes(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range readDocs(t) {
		if d.name == "llms.txt" {
			if n := len(d.text); n > coreMaxBytes {
				t.Errorf("docs/llms.txt is %d bytes, more than %d: move what only some screens need into a recipe of docs/llms/", n, coreMaxBytes)
			}
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(d.name, "llms/"), ".md")
		seen[name] = true
		max, ok := recipeMaxBytes[name]
		if !ok {
			t.Errorf("docs/%s has no size limit: add a row to recipeMaxBytes (the budget is part of ADR-313)", d.name)
			continue
		}
		if n := len(d.text); n > max {
			t.Errorf("docs/%s is %d bytes, more than its limit of %d: split the recipe or cut it", d.name, n, max)
		}
	}
	for name := range recipeMaxBytes {
		if !seen[name] {
			t.Errorf("recipeMaxBytes has a limit for docs/llms/%s.md, which does not exist", name)
		}
	}
}

// indexLine is a line of the core's recipe index.
var indexLine = regexp.MustCompile(`^- \[([a-z0-9-]+)\]\(llms/([a-z0-9-]+)\.md\): (.+)$`)

// maxIndexLine keeps an index entry to one short line: the recipe's name,
// and the questions it answers.
const maxIndexLine = 220

// TestIndex: the core's "## Recipes" section has one line for each recipe,
// "- [name](llms/name.md): what it answers", and every line names a recipe
// that exists. Each recipe starts with its title.
func TestIndex(t *testing.T) {
	docs := readDocs(t)
	core := docs[0].text
	_, section, ok := sectionOf(core, "Recipes")
	if !ok {
		t.Fatal(`docs/llms.txt has no "## Recipes" section: the core lists the recipes of docs/llms/, one line each`)
	}
	listed := map[string]bool{}
	for _, line := range section {
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		m := indexLine.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("recipe index line %q: write - [name](llms/name.md): what questions it answers", line)
			continue
		}
		if m[1] != m[2] {
			t.Errorf("recipe index line %q: the text and the path name different recipes", line)
		}
		if len(line) > maxIndexLine {
			t.Errorf("recipe index line of %s has %d characters, more than %d: one line says what the recipe answers", m[1], len(line), maxIndexLine)
		}
		listed[m[2]] = true
	}
	for _, d := range docs[1:] {
		name := strings.TrimSuffix(strings.TrimPrefix(d.name, "llms/"), ".md")
		if !listed[name] {
			t.Errorf("the recipe index of docs/llms.txt does not list %s", d.name)
		}
		delete(listed, name)
		if !strings.HasPrefix(d.text, "# ") {
			t.Errorf("docs/%s must start with its title, a line that begins with \"# \"", d.name)
		}
	}
	for name := range listed {
		t.Errorf("the recipe index lists docs/llms/%s.md, which does not exist", name)
	}
}

// sectionOf returns the lines of the "## title" section of a document, up
// to the next "## " heading, and the line number (1-based) of the first.
func sectionOf(text, title string) (first int, lines []string, ok bool) {
	all := strings.Split(text, "\n")
	for i, l := range all {
		if !strings.HasPrefix(l, "## ") || strings.TrimSpace(strings.TrimPrefix(l, "## ")) != title {
			continue
		}
		j := i + 1
		for j < len(all) && !strings.HasPrefix(all[j], "## ") {
			j++
		}
		return i + 2, all[i+1 : j], true
	}
	return 0, nil, false
}

// needGo skips a test that runs the go command when there is none: under
// Wine, and on a Windows machine without Go, the test binary runs alone.
func needGo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the go command is not in PATH")
	}
}
