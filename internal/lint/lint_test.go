package lint

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The fixtures in testdata are small packages of Tela code. A line that
// ends in a comment
//
//	// want rule `part of the message` [rule `part`...]
//
// must get exactly those findings, one per pair; a line without one must get
// none. Each group of packages is linted in a run of its own, because the
// aliases and the theme colors of a run are shared by its packages.

var wantRE = regexp.MustCompile("(\\w+) `([^`]*)`")

type want struct {
	file string
	line int
	rule string
	part string
}

func (w want) String() string { return fmt.Sprintf("%s:%d: %s %q", w.file, w.line, w.rule, w.part) }

// wants reads the want comments of the Go files in dirs.
func wants(t *testing.T, dirs ...string) []want {
	t.Helper()
	var out []want
	for _, dir := range dirs {
		files, err := filepath.Glob(filepath.Join("testdata", dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no fixture files in testdata/%s", dir)
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for i, line := range strings.Split(string(data), "\n") {
				_, comment, ok := strings.Cut(line, "// want ")
				if !ok {
					continue
				}
				for _, m := range wantRE.FindAllStringSubmatch(comment, -1) {
					out = append(out, want{filepath.ToSlash(file), i + 1, m[1], m[2]})
				}
			}
		}
	}
	return out
}

// lintDirs lints the fixture packages in dirs.
func lintDirs(t *testing.T, tests bool, dirs ...string) *Result {
	t.Helper()
	var patterns []string
	for _, d := range dirs {
		patterns = append(patterns, "./testdata/"+d)
	}
	res, err := Run(Options{Patterns: patterns, Tests: tests})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) > 0 {
		t.Fatalf("load errors: %v", res.LoadErrors)
	}
	return res
}

// checkFixtures lints dirs and compares the findings with the want comments.
func checkFixtures(t *testing.T, dirs ...string) *Result {
	t.Helper()
	res := lintDirs(t, false, dirs...)
	compare(t, res.Diagnostics, wants(t, dirs...))
	return res
}

func compare(t *testing.T, got []Diagnostic, wanted []want) {
	t.Helper()
	used := make([]bool, len(got))
	for _, w := range wanted {
		found := false
		for i, d := range got {
			if !used[i] && d.File == w.file && d.Line == w.line && d.Rule == w.rule && strings.Contains(d.Message, w.part) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			t.Errorf("missing finding %s", w)
		}
	}
	for i, d := range got {
		if !used[i] {
			t.Errorf("unexpected finding %s", d)
		}
	}
}

func TestClasses(t *testing.T) {
	needGo(t)
	checkFixtures(t, "classes")
}

func TestClassesThroughImports(t *testing.T) {
	needGo(t)
	checkFixtures(t, "dotimport", "renamed")
}

func TestAliasesAndThemeColors(t *testing.T) {
	needGo(t)
	checkFixtures(t, "aliases")
}

func TestPropsRolesAria(t *testing.T) {
	needGo(t)
	checkFixtures(t, "props")
}

func TestKeys(t *testing.T) {
	needGo(t)
	res := checkFixtures(t, "keys")
	for _, d := range res.Diagnostics {
		if d.Severity != SeverityWarning {
			t.Errorf("%s: a key finding is a warning, got %q", d, d.Severity)
		}
	}
}

func TestHooks(t *testing.T) {
	needGo(t)
	res := checkFixtures(t, "hooks")
	for _, d := range res.Diagnostics {
		if d.Severity != SeverityError {
			t.Errorf("%s: a hooks finding is an error, got %q", d, d.Severity)
		}
	}
}

// Test files are linted only when asked for: the in-package ones and the
// external ones.
func TestTestFiles(t *testing.T) {
	needGo(t)
	if res := lintDirs(t, false, "withtests"); len(res.Diagnostics) != 0 {
		t.Errorf("without Tests: %v", res.Diagnostics)
	}
	res := lintDirs(t, true, "withtests")
	compare(t, res.Diagnostics, wants(t, "withtests"))
}

// Functions with the names of Tela's, from other packages, are not Tela's.
func TestLookalikesAreNotReported(t *testing.T) {
	needGo(t)
	res := lintDirs(t, false, "lookalike")
	for _, d := range res.Diagnostics {
		t.Errorf("unexpected finding %s", d)
	}
}

// A class error points at the class inside the literal.
func TestClassPosition(t *testing.T) {
	needGo(t)
	const file = "testdata/classes/classes.go"
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	line, col := 0, 0
	for i, l := range strings.Split(string(data), "\n") {
		if j := strings.Index(l, `"flex rounded-xxl"`); j >= 0 {
			line, col = i+1, strings.Index(l, "rounded-xxl")+1
			break
		}
	}
	if line == 0 {
		t.Fatal("the fixture changed")
	}
	res := lintDirs(t, false, "classes")
	for _, d := range res.Diagnostics {
		if d.File == file && d.Line == line {
			if d.Col != col {
				t.Errorf("column %d, want %d (the class)", d.Col, col)
			}
			if d.Suggestion != "rounded-2xl" {
				t.Errorf("suggestion %q", d.Suggestion)
			}
			return
		}
	}
	t.Errorf("no finding at %s:%d", file, line)
}

func TestOutputOrderAndFormat(t *testing.T) {
	needGo(t)
	res := lintDirs(t, false, "classes", "props")
	sorted := sort.SliceIsSorted(res.Diagnostics, func(i, j int) bool {
		a, b := res.Diagnostics[i], res.Diagnostics[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Col < b.Col
	})
	if !sorted {
		t.Error("diagnostics are not sorted by position")
	}
	var text bytes.Buffer
	WriteText(&text, res.Diagnostics)
	first := strings.SplitN(text.String(), "\n", 2)[0]
	if !regexp.MustCompile(`^testdata/classes/classes\.go:\d+:\d+: class: unknown class`).MatchString(first) {
		t.Errorf("first line %q", first)
	}

	var js bytes.Buffer
	if err := WriteJSON(&js, res.Diagnostics); err != nil {
		t.Fatal(err)
	}
	var back []map[string]any
	if err := json.Unmarshal(js.Bytes(), &back); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, js.String())
	}
	if len(back) != len(res.Diagnostics) {
		t.Fatalf("%d objects for %d findings", len(back), len(res.Diagnostics))
	}
	for _, k := range []string{"file", "line", "col", "rule", "severity", "message"} {
		if _, ok := back[0][k]; !ok {
			t.Errorf("JSON object lacks %q: %v", k, back[0])
		}
	}
	js.Reset()
	if err := WriteJSON(&js, nil); err != nil || strings.TrimSpace(js.String()) != "[]" {
		t.Errorf("no findings: %q, %v", js.String(), err)
	}
}

func TestIgnoreDirective(t *testing.T) {
	for _, tc := range []struct {
		text  string
		rules []string
		ok    bool
	}{
		{"//tela:ignore class", []string{"class"}, true},
		{"// tela:ignore class", []string{"class"}, true},
		{"//tela:ignore class,role", []string{"class", "role"}, true},
		{"//tela:ignore class, role", []string{"class", "role"}, true},
		{"//tela:ignore class , role -- why", []string{"class", "role"}, true},
		{"//tela:ignore class, key -- legacy", []string{"class", "key"}, true},
		{"//tela:ignore class,role,", []string{"class", "role"}, true},
		{"//tela:ignore class -- why", []string{"class"}, true},
		{"//tela:ignore class why, not role", []string{"class"}, true}, // the rest is a reason
		{"//tela:ignore", []string{"all"}, true},
		{"//tela:ignore   ", []string{"all"}, true},
		{"//tela:ignore -- generated code", []string{"all"}, true},
		{"//tela:ignore --", []string{"all"}, true},
		{"//tela:ignored class", nil, false},
		{"// something else", nil, false},
		{"/* tela:ignore class */", nil, false},
	} {
		rules, ok := parseIgnore(tc.text)
		if ok != tc.ok || strings.Join(rules, ",") != strings.Join(tc.rules, ",") {
			t.Errorf("%q: got %v, %v; want %v, %v", tc.text, rules, ok, tc.rules, tc.ok)
		}
	}
}

// A package that does not type-check is reported and skipped.
func TestBrokenPackageIsSkipped(t *testing.T) {
	needGo(t)
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/broken\n\ngo 1.22\n")
	write("main.go", "package main\n\nfunc main() { undefined() }\n")
	res, err := Run(Options{Dir: dir, Patterns: []string{"./..."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) != 1 || !strings.Contains(res.LoadErrors[0], "undefined") {
		t.Errorf("load errors: %v", res.LoadErrors)
	}
	if len(res.Diagnostics) != 0 {
		t.Errorf("findings: %v", res.Diagnostics)
	}
}

// A pattern that matches no package is a load error.
func TestNoPackages(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{})
	res, err := Run(Options{Dir: dir, Patterns: []string{"./nothing"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) == 0 {
		t.Error("a pattern without packages should be reported")
	}
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module example.com/m\n\ngo 1.22\n"
	for name, src := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// An import cycle is a load error, not a deadlock.
func TestImportCycle(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{
		"a/a.go":  "package a\n\nimport _ \"example.com/m/b\"\n",
		"b/b.go":  "package b\n\nimport _ \"example.com/m/a\"\n",
		"main.go": "package main\n\nimport _ \"example.com/m/a\"\n\nfunc main() {}\n",
	})
	res, err := Run(Options{Dir: dir, Patterns: []string{"./..."}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.LoadErrors) == 0 || !strings.Contains(strings.Join(res.LoadErrors, "\n"), "import cycle") {
		t.Errorf("load errors: %v", res.LoadErrors)
	}
}

// A directory with only test files is nothing to lint, not an error.
func TestPackageWithOnlyTests(t *testing.T) {
	needGo(t)
	dir := writeModule(t, map[string]string{
		"x_test.go": "package m\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n",
	})
	for _, tests := range []bool{false, true} {
		res, err := Run(Options{Dir: dir, Patterns: []string{"./..."}, Tests: tests})
		if err != nil || len(res.LoadErrors) != 0 || len(res.Diagnostics) != 0 {
			t.Errorf("tests=%v: %v, %v, %v", tests, err, res.LoadErrors, res.Diagnostics)
		}
	}
}

// The repository follows its own rules (CI runs tela lint ./...).
func TestRepositoryLintsClean(t *testing.T) {
	needGo(t)
	res, err := Run(Options{Dir: "../..", Patterns: []string{"./..."}})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.LoadErrors {
		t.Errorf("load error: %s", e)
	}
	for _, d := range res.Diagnostics {
		t.Errorf("%s", d)
	}
	if res.Packages < 50 {
		t.Errorf("only %d packages were linted", res.Packages)
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
