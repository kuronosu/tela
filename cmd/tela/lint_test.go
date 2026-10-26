package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os/exec"
	"strings"
	"testing"
)

// The tests run `tela lint` on the packages of testdata/lint, from the
// directory of this package.

func lintRun(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(append([]string{"lint"}, args...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestLintClean(t *testing.T) {
	needGo(t)
	code, stdout, stderr := lintRun(t, "./testdata/lint/clean")
	if code != 0 || stdout != "" || stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q; want a silent success", code, stdout, stderr)
	}
}

func TestLintReportsAndExitsOne(t *testing.T) {
	needGo(t)
	code, stdout, stderr := lintRun(t, "./testdata/lint/dirty")
	if code != 1 {
		t.Fatalf("exit %d, want 1; stderr %q", code, stderr)
	}
	want := []string{
		`testdata/lint/dirty/dirty.go:10:3: hooks: hook tela.UseState is called conditionally (inside an if)`,
		`testdata/lint/dirty/dirty.go:12:25: class: unknown class "rounded-xxl"; did you mean "rounded-2xl"?`,
		`testdata/lint/dirty/dirty.go:12:45: role: Role("buton"): unknown ARIA role "buton"; did you mean "button"?`,
		`testdata/lint/dirty/dirty.go:12:78: key: duplicate key "a" among the children of this element`,
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != len(want) {
		t.Fatalf("%d lines, want %d:\n%s", len(lines), len(want), stdout)
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Errorf("line %d:\n got %q\nwant %q...", i, lines[i], w)
		}
	}
	if stderr != "" {
		t.Errorf("stderr %q", stderr)
	}
}

func TestLintJSON(t *testing.T) {
	needGo(t)
	code, stdout, _ := lintRun(t, "-json", "./testdata/lint/dirty")
	if code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	var got []struct {
		File       string `json:"file"`
		Line       int    `json:"line"`
		Col        int    `json:"col"`
		Rule       string `json:"rule"`
		Severity   string `json:"severity"`
		Message    string `json:"message"`
		Suggestion string `json:"suggestion"`
	}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, stdout)
	}
	if len(got) != 4 {
		t.Fatalf("%d objects, want 4: %+v", len(got), got)
	}
	class := got[1]
	if class.File != "testdata/lint/dirty/dirty.go" || class.Line != 12 || class.Rule != "class" ||
		class.Severity != "error" || class.Suggestion != "rounded-2xl" || !strings.Contains(class.Message, "unknown class") {
		t.Errorf("class object: %+v", class)
	}
	if key := got[3]; key.Rule != "key" || key.Severity != "warning" {
		t.Errorf("key object: %+v", key)
	}

	// A clean run prints an empty array.
	code, stdout, _ = lintRun(t, "--json", "./testdata/lint/clean")
	if code != 0 || strings.TrimSpace(stdout) != "[]" {
		t.Errorf("clean: exit %d, stdout %q", code, stdout)
	}
}

func TestLintPatternsAndFlagsInAnyOrder(t *testing.T) {
	needGo(t)
	code, stdout, _ := lintRun(t, "./testdata/lint/clean", "-json", "./testdata/lint/dirty")
	if code != 1 || !strings.HasPrefix(stdout, "[") || !strings.Contains(stdout, "dirty.go") {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
}

func TestLintBrokenPackageExitsTwo(t *testing.T) {
	needGo(t)
	code, stdout, stderr := lintRun(t, "./testdata/lint/broken", "./testdata/lint/dirty")
	if code != 2 {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr, "broken") || !strings.Contains(stderr, "undefined") {
		t.Errorf("stderr %q does not name the package and the error", stderr)
	}
	if !strings.Contains(stdout, "dirty.go") {
		t.Errorf("the other package is still linted; stdout %q", stdout)
	}
}

func TestLintUsageErrors(t *testing.T) {
	code, _, stderr := lintRun(t, "-frobnicate")
	if code != 2 || !strings.Contains(stderr, "flag provided but not defined") || !strings.Contains(stderr, "tela help lint") {
		t.Errorf("unknown flag: exit %d, stderr %q", code, stderr)
	}
	code, _, stderr = lintRun(t, "./testdata/lint/nothing-here")
	if code != 2 || stderr == "" {
		t.Errorf("no such package: exit %d, stderr %q", code, stderr)
	}
	if _, err := parseLintArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v, want flag.ErrHelp", err)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"help", "lint"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "tela lint [-json]") {
		t.Errorf("help lint: exit %d, stdout %q", code, out.String())
	}
}

func TestLintTestsFlag(t *testing.T) {
	opts, err := parseLintArgs([]string{"-tests", "-json", "./a", "./b"})
	if err != nil || !opts.Tests || !opts.JSON || len(opts.Patterns) != 2 {
		t.Errorf("got %+v, %v", opts, err)
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
