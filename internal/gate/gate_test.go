package main

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseTestJSON(t *testing.T) {
	run, err := parseTestJSON(strings.NewReader(readFixture(t, "gotest.jsonl")))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"go: some stray line that is not JSON"}; !reflect.DeepEqual(run.Stray, want) {
		t.Errorf("Stray = %q, want %q", run.Stray, want)
	}
	if got := len(run.PkgOrder); got != 6 {
		t.Fatalf("%d packages, want 6: %v", got, run.PkgOrder)
	}
	if !run.Pkgs["m/b"].Cached || run.Pkgs["m/a"].Cached {
		t.Errorf("cached: m/b = %v, m/a = %v, want true, false", run.Pkgs["m/b"].Cached, run.Pkgs["m/a"].Cached)
	}
	if !run.Pkgs["m/c"].NoTests {
		t.Errorf("m/c should have no test files")
	}
	pkgs, cached, tests := run.counts()
	if pkgs != 5 || cached != 1 || tests != 9 {
		t.Errorf("counts = %d packages, %d cached, %d tests; want 5, 1, 9", pkgs, cached, tests)
	}
}

func TestFailures(t *testing.T) {
	run, _ := parseTestJSON(strings.NewReader(readFixture(t, "gotest.jsonl")))
	fails := run.failures()
	type key struct{ pkg, test, reason string }
	var got []key
	for _, f := range fails {
		got = append(got, key{f.Package, f.Test, f.Reason})
	}
	want := []key{
		{"m/a", "TestSub/native", "failed"}, // TestSub itself is left out: its subtest carries the output
		{"m/a", "TestPlain", "failed"},
		{"m/d", "", "package failed"},
		{"m/e", "", "build failed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("failures = %v, want %v", got, want)
	}
	if out := strings.Join(fails[0].Output, "\n"); !strings.Contains(out, "batch too early") || strings.Contains(out, "=== RUN") {
		t.Errorf("output of the failing subtest = %q, want its lines without the === markers", out)
	}
	if out := strings.Join(fails[2].Output, "\n"); !strings.Contains(out, "panic: boom") || !strings.Contains(out, "did not finish: TestPanics, TestSlowParallel") {
		t.Errorf("package failure output = %q", out)
	}
	if out := strings.Join(fails[3].Output, "\n"); !strings.Contains(out, "undefined: nope") {
		t.Errorf("build failure output = %q", out)
	}
}

func TestSlowestSkipsCachedPackages(t *testing.T) {
	run, _ := parseTestJSON(strings.NewReader(readFixture(t, "gotest.jsonl")))
	tests, pkgs := run.slowest(3)
	wantTests := []slow{{"m/f TestSlow", 4.5}, {"m/a TestSub", 2.01}, {"m/a TestSub/native", 2}}
	if !reflect.DeepEqual(tests, wantTests) {
		t.Errorf("slowest tests = %v, want %v (TestFast took 9s but was cached)", tests, wantTests)
	}
	wantPkgs := []slow{{"m/f", 4.7}, {"m/a", 2.72}, {"m/d", 0.31}}
	if !reflect.DeepEqual(pkgs, wantPkgs) {
		t.Errorf("slowest packages = %v, want %v", pkgs, wantPkgs)
	}
}

func TestFindFlake(t *testing.T) {
	roadmap := strings.Split(readFixture(t, "roadmap.md"), "\n")
	cases := []struct {
		test string
		line int
	}{
		{"TestSub/native", 3},        // the subtest path itself
		{"TestSub/native/deeper", 3}, // a child of a listed subtest
		{"TestPanics", 5},            // a top-level name
		{"TestSub/good", 0},          // named in the roadmap but not in backticks
		{"TestSub", 0},               // the parent is not listed, only one of its subtests
		{"TestUnknown", 0},
		{"", 0},
	}
	for _, c := range cases {
		line, ok := findFlake(roadmap, c.test)
		if line != c.line || ok != (c.line != 0) {
			t.Errorf("findFlake(%q) = %d, %v; want line %d", c.test, line, ok, c.line)
		}
	}
}

func TestParseDiff(t *testing.T) {
	files := parseDiff(readFixture(t, "diff.txt"))
	byPath := map[string]fileDiff{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	if got := len(files); got != 13 {
		t.Fatalf("%d files, want 13", got)
	}
	thing := byPath["internal/core/thing.go"]
	if thing.Status != "M" || len(thing.Added) != 5 || len(thing.Removed) != 2 {
		t.Fatalf("thing.go = %+v", thing)
	}
	if thing.Added[0].N != 6 || thing.Added[3].N != 9 || thing.Added[4].N != 24 {
		t.Errorf("added line numbers = %v, want 6..9 and 24", thing.Added)
	}
	if thing.Removed[0].N != 20 || thing.Removed[1].N != 21 {
		t.Errorf("removed line numbers = %v, want 20, 21", thing.Removed)
	}
	if got := byPath["internal/core/gone.go"]; got.Status != "D" || len(got.Removed) != 2 {
		t.Errorf("gone.go = %+v", got)
	}
	if got := byPath["testdata/new.png"]; got.Status != "A" {
		t.Errorf("new.png = %+v, want status A", got)
	}
}

func TestParseDiffMnemonicPrefixes(t *testing.T) {
	text := "diff --git c/dir with space/x.go w/dir with space/x.go\nindex 1..2 100644\n--- c/dir with space/x.go\n+++ w/dir with space/x.go\n@@ -3 +3 @@\n-old\n+new\n"
	files := parseDiff(text)
	if len(files) != 1 || files[0].Path != "dir with space/x.go" || len(files[0].Added) != 1 || files[0].Added[0].N != 3 {
		t.Errorf("files = %+v", files)
	}
}

func TestChangedGoldens(t *testing.T) {
	got := changedGoldens(parseDiff(readFixture(t, "diff.txt")))
	want := []goldenChange{
		{"internal/style/testdata/a.png", "modified"},
		{"internal/style/testdata/gone.png", "removed"},
		{"testdata/new.png", "added"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("goldens = %v, want %v", got, want)
	}
}

func TestClassify(t *testing.T) {
	known := func(name string) bool { return name == "TELA_GATEFIXTURE_KNOWN" }
	items := classify(parseDiff(readFixture(t, "diff.txt")), known)
	var got []string
	for _, it := range items {
		got = append(got, it.Change+" | "+it.What+" | "+it.Where)
	}
	want := []string{
		"changed | apparatus file | .github/workflows/ci.yml",
		"added | apparatus file | internal/gate/main.go", // its lines are not scanned
		"added | new ADR file | docs/adr/290-gate.md",
		"added | new doc file | docs/notes.md",
		// docs/ROADMAP.md changed: not apparatus
		"changed | apparatus file | AGENTS.md",
		"changed | GPU comparison tolerances | internal/gfx/gpu/compare_test.go",
		"added | TELA_GATEFIXTURE_NEW read | internal/core/thing.go:6", // TELA_GATEFIXTURE_KNOWN is in the base
		"added | test skip | internal/core/thing.go:8",
		"added | ignore directive | internal/core/thing.go:9",
		"removed | test skip | internal/core/thing.go:20",
		"removed | ignore directive | internal/core/thing.go:21",
		"removed | TELA_GATEFIXTURE_OLD read | internal/core/gone.go:2",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("approval items:\n got  %s\n want %s", strings.Join(got, "\n      "), strings.Join(want, "\n      "))
	}
}

func TestWordCount(t *testing.T) {
	if got := wordCount("  one two\n\tthree  \n\nfour "); got != 4 {
		t.Errorf("wordCount = %d, want 4", got)
	}
	arch := readFixture(t, "arch.md")
	// Sections 1 (heading 3 words "## 1. One" = 3; body 4; sub heading and body
	// 8 + 1), 3 and 19 count; 2, 20 and the title do not.
	if got, want := sectionWords(arch, []int{1}), 3+4+8+1; got != want {
		t.Errorf("section 1 = %d words, want %d", got, want)
	}
	if got, want := sectionWords(arch, []int{1, 3, 19}), 16+(3+2)+(3+3); got != want {
		t.Errorf("sections 1, 3, 19 = %d words, want %d", got, want)
	}
	if got := sectionWords(arch, []int{7}); got != 0 {
		t.Errorf("a missing section = %d words, want 0", got)
	}
}

func TestDocBudgetsOfTheRepository(t *testing.T) {
	counts := checkDocs("../..", docBudgets)
	if len(counts) != len(docBudgets) {
		t.Fatalf("%d counts for %d budgets", len(counts), len(docBudgets))
	}
	for _, c := range counts {
		if c.Err != "" {
			t.Errorf("%s: %s", c.Label, c.Err)
		}
	}
}

func TestResolveBase(t *testing.T) {
	// git answers by a table keyed by the arguments.
	fake := func(answers map[string]string) gitRunner {
		return func(args ...string) (string, error) {
			if out, ok := answers[strings.Join(args, " ")]; ok {
				return out, nil
			}
			return "", os.ErrNotExist
		}
	}
	const (
		revMain = "rev-parse --verify --quiet main^{commit}"
		revOrig = "rev-parse --verify --quiet origin/main^{commit}"
	)
	cases := []struct {
		name    string
		flag    string
		answers map[string]string
		want    string
	}{
		{"on a branch: merge base with main", "", map[string]string{
			"rev-parse --verify --quiet HEAD": "h1", revMain: "m1", "merge-base HEAD main": "b1",
			revOrig: "o1", "merge-base HEAD origin/main": "b0",
		}, "b1"},
		{"on main: merge base with origin/main", "", map[string]string{
			"rev-parse --verify --quiet HEAD": "h1", revMain: "h1", "merge-base HEAD main": "h1",
			revOrig: "o1", "merge-base HEAD origin/main": "b0",
		}, "b0"},
		{"on main without origin/main: HEAD itself", "", map[string]string{
			"rev-parse --verify --quiet HEAD": "h1", revMain: "h1", "merge-base HEAD main": "h1",
		}, "h1"},
		{"no main at all: the working tree only", "", map[string]string{
			"rev-parse --verify --quiet HEAD": "h1",
		}, ""},
		{"explicit flag", "v1.0", map[string]string{
			"rev-parse --verify --quiet v1.0^{commit}": "t1",
		}, "t1"},
		{"unknown flag", "nope", map[string]string{}, ""},
	}
	for _, c := range cases {
		if got := resolveBase(fake(c.answers), c.flag); got != c.want {
			t.Errorf("%s: base = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestBuildEnv(t *testing.T) {
	base := []string{"PATH=/bin", "CGO_ENABLED=1", "GOOS=plan9", "goarch=arm", "TELA_UPDATE=1", "TELA_DEBUG=layout", "HOME=/h"}
	got := buildEnv(base, []string{"GOOS=windows"}, true)
	want := []string{"PATH=/bin", "HOME=/h", "CGO_ENABLED=0", "GOOS=windows"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("env for tests = %v, want %v", got, want)
	}
	got = buildEnv(base, nil, false)
	want = []string{"PATH=/bin", "TELA_UPDATE=1", "TELA_DEBUG=layout", "HOME=/h", "CGO_ENABLED=0"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("env without stripping = %v, want %v", got, want)
	}
}

func TestSelectSteps(t *testing.T) {
	en, err := selectSteps(options{tests: false, skip: "lint"})
	if err != nil {
		t.Fatal(err)
	}
	if en["test"] || en["lint"] || !en["vet"] {
		t.Errorf("enabled = %v, want test and lint off, vet on", en)
	}
	en, _ = selectSteps(options{tests: true, only: "fmt, docs"})
	if !en["fmt"] || !en["docs"] || en["vet"] || en["test"] {
		t.Errorf("-only: enabled = %v", en)
	}
	if _, err := selectSteps(options{tests: true, only: "tset"}); err == nil || !strings.Contains(err.Error(), "unknown step") {
		t.Errorf("an unknown step should be an error naming the steps, got %v", err)
	}
}

func TestCheckFormatted(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) string {
		p := dir + "/" + name
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if got := checkFormatted(write("ok.go", "package x\n\nfunc f() {}\n")); got != "" {
		t.Errorf("formatted file: %q", got)
	}
	if got := checkFormatted(write("bad.go", "package x\nfunc  f( ) {}\n")); got != "not formatted" {
		t.Errorf("unformatted file: %q", got)
	}
	if got := checkFormatted(write("broken.go", "package x\nfunc (")); got == "" || got == "not formatted" {
		t.Errorf("a syntax error should be reported, got %q", got)
	}
	if got := checkFormatted(dir + "/missing.go"); got != "" {
		t.Errorf("a file git lists but that was deleted is not a failure, got %q", got)
	}
}

func TestCollapseApparatus(t *testing.T) {
	var items []approvalItem
	for _, f := range []string{"a.go", "b.go", "c.go", "d.go"} {
		items = append(items, approvalItem{Change: "added", What: "apparatus file", Where: "internal/gate/" + f})
	}
	items = append(items,
		approvalItem{Change: "changed", What: "apparatus file", Where: ".github/workflows/ci.yml"},
		approvalItem{Change: "added", What: "test skip", Where: "x.go:3"})
	got := collapseApparatus(items)
	if len(got) != 3 || got[0].Where != "internal/gate/ (4 files)" || got[1].Where != ".github/workflows/ci.yml" || got[2].What != "test skip" {
		t.Errorf("collapsed = %+v", got)
	}
}

// TestFailureKeepsErrorLines is the failure of 2026-11-22: an aria snapshot
// printed after the error filled the last lines, so the gate showed no
// error line at all.
func TestFailureKeepsErrorLines(t *testing.T) {
	out := []string{"    notes_test.go:88: aria snapshot of the list:"}
	for i := range 120 {
		out = append(out, fmt.Sprintf("        - listitem \"Note %d\"", i))
	}
	out = append(out, "    notes_test.go:91: got 120 items, want 121")
	for i := range 60 {
		out = append(out, fmt.Sprintf("        - row %d", i))
	}
	out = append(out, "--- FAIL: TestAriaSnapshot (0.41s)")
	f := (&gate{}).describeFailure(testFailure{Package: modulePath + "/examples/notes", Test: "TestAriaSnapshot", Reason: "failed", Output: out})
	for _, want := range []string{
		"notes_test.go:88: aria snapshot of the list:",
		`- listitem "Note 0"`, // the start of a dump says what it is
		"notes_test.go:91: got 120 items, want 121",
		"--- FAIL: TestAriaSnapshot (0.41s)",
	} {
		if !strings.Contains(f.Detail, want) {
			t.Errorf("detail lacks %q:\n%s", want, f.Detail)
		}
	}
	if strings.Contains(f.Detail, `"Note 60"`) {
		t.Errorf("detail keeps the middle of the dump:\n%s", f.Detail)
	}
	if n := strings.Count(f.Detail, "\n") + 1; n > 2*maxTail {
		t.Errorf("detail has %d lines, want at most %d:\n%s", n, 2*maxTail, f.Detail)
	}
}

// TestFailureKeepsManyErrorLines bounds a test that reports an error per
// item: the first errors and the last are kept.
func TestFailureKeepsManyErrorLines(t *testing.T) {
	var out []string
	for i := range 200 {
		out = append(out, fmt.Sprintf("    x_test.go:%d: item %d is wrong", 10+i, i))
	}
	out = append(out, "--- FAIL: TestItems (0.01s)")
	f := (&gate{}).describeFailure(testFailure{Package: "m/x", Test: "TestItems", Reason: "failed", Output: out})
	for _, want := range []string{"x_test.go:10: item 0 is wrong", "x_test.go:209: item 199 is wrong", "--- FAIL: TestItems"} {
		if !strings.Contains(f.Detail, want) {
			t.Errorf("detail lacks %q:\n%s", want, f.Detail)
		}
	}
	if n := strings.Count(f.Detail, "\n") + 1; n > 2*maxTail {
		t.Errorf("detail has %d lines, want at most %d:\n%s", n, 2*maxTail, f.Detail)
	}
}

// TestFailureKeepsPanicHead keeps the first lines of a panic (the test
// that timed out, the panic value) when a goroutine dump follows.
func TestFailureKeepsPanicHead(t *testing.T) {
	out := []string{
		"panic: test timed out after 10m0s",
		"\trunning tests:",
		"\t\tTestCopyActions (10m0s)",
		"",
	}
	for i := range 200 {
		out = append(out, fmt.Sprintf("goroutine %d [chan receive]:", i), "\t/x/y.go:12 +0x45", "")
	}
	out = append(out, "FAIL\tgithub.com/kuronosu/tela/cmd/tela/internal/devtools\t600.1s")
	f := (&gate{}).describeFailure(testFailure{Package: modulePath + "/cmd/tela/internal/devtools", Reason: "package failed", Output: out})
	for _, want := range []string{"panic: test timed out after 10m0s", "TestCopyActions (10m0s)", "devtools\t600.1s"} {
		if !strings.Contains(f.Detail, want) {
			t.Errorf("detail lacks %q:\n%s", want, f.Detail)
		}
	}
	if n := strings.Count(f.Detail, "\n") + 1; n > 2*maxTail {
		t.Errorf("detail has %d lines, want at most %d:\n%s", n, 2*maxTail, f.Detail)
	}
}

// TestFailureShortOutputWhole keeps a short output as it is.
func TestFailureShortOutputWhole(t *testing.T) {
	out := []string{"    a_test.go:3: got 1", "        more", "--- FAIL: TestA (0.00s)"}
	f := (&gate{}).describeFailure(testFailure{Package: "m/a", Test: "TestA", Reason: "failed", Output: out})
	if want := strings.Join(out, "\n"); f.Detail != want {
		t.Errorf("detail = %q, want %q", f.Detail, want)
	}
}

// The test step turns go test's vet off because the vet step covers what
// it checked: every test command has a vet command for the same module
// and tags on each system the tests may run on, for the host's
// architecture (vet runs a superset of go test's analyzers, test files
// included).
func TestTestStepVetIsCovered(t *testing.T) {
	tags := func(args []string) string {
		for i, a := range args {
			if a == "-tags" && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}
	type key struct{ dir, tags, goos string }
	vetted := map[key]bool{}
	for _, s := range vetSpecs() {
		goos, arch := "", ""
		for _, e := range s.env {
			if v, ok := strings.CutPrefix(e, "GOOS="); ok {
				goos = v
			}
			if v, ok := strings.CutPrefix(e, "GOARCH="); ok {
				arch = v
			}
		}
		if arch == "" {
			vetted[key{s.dir, tags(s.args), goos}] = true
		}
	}
	n := 0
	for _, s := range testSpecs() {
		if !slices.Contains(s.args, "-vet=off") {
			continue
		}
		n++
		for _, goos := range []string{"linux", "windows", "darwin"} {
			if !vetted[key{s.dir, tags(s.args), goos}] {
				t.Errorf("%s: no vet command for dir %q, tags %q, GOOS=%s", s.label, s.dir, tags(s.args), goos)
			}
		}
	}
	if n == 0 {
		t.Error("no test command turns vet off: this test checks nothing")
	}
}
