package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"testing"
)

func TestModuleFromName(t *testing.T) {
	for name, want := range map[string]string{
		"my-app":     "my-app",
		"My App":     "my-app",
		"Notes_2":    "notes_2",
		"..hidden":   "hidden",
		"café":       "caf",
		"日本語":        "",
		"app.v2":     "app.v2",
		"  spaced  ": "spaced",
	} {
		if got := moduleFromName(name); got != want {
			t.Errorf("moduleFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCheckModulePath(t *testing.T) {
	for _, ok := range []string{"app", "my-app", "example.com/me/app", "github.com/you/my_app.v2", "a~b"} {
		if err := checkModulePath(ok); err != nil {
			t.Errorf("checkModulePath(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "/app", "app/", "a//b", ".app", "app.", "a/../b", "my app", "café", "a:b", `a\b`} {
		if err := checkModulePath(bad); err == nil {
			t.Errorf("checkModulePath(%q) = nil, want an error", bad)
		}
	}
}

func TestTitleFromName(t *testing.T) {
	for name, want := range map[string]string{"my-app": "My App", "notes": "Notes", "a_b.c": "A B C", "---": "Tela App"} {
		if got := titleFromName(name); got != want {
			t.Errorf("titleFromName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestTelaVersion(t *testing.T) {
	info := func(path, version string) (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Main: debug.Module{Path: path, Version: version}}, true
	}
	for _, c := range []struct {
		path, version, want string
	}{
		{telaModule, "v0.9.1", "v0.9.1"},
		{telaModule, "v0.9.2-0.20261022000000-abcdef123456", "v0.9.2-0.20261022000000-abcdef123456"},
		{telaModule, "(devel)", "latest"},
		{telaModule, "", "latest"},
		{telaModule, "v0.9.1+dirty", "latest"},
		{"example.com/other", "v1.2.3", "latest"},
	} {
		if got := telaVersion(info(c.path, c.version)); got != c.want {
			t.Errorf("telaVersion(%s, %q) = %q, want %q", c.path, c.version, got, c.want)
		}
	}
	if got := telaVersion(nil, false); got != "latest" {
		t.Errorf("telaVersion without build info = %q, want latest", got)
	}
}

// templateGoVersion must follow the go directive of tela's own go.mod.
func TestTemplateGoVersion(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^go (\S+)$`).FindSubmatch(b)
	if m == nil || string(m[1]) != templateGoVersion {
		t.Errorf("templateGoVersion = %q, but go.mod has %q", templateGoVersion, m)
	}
}

func runNewCmd(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(append([]string{"new"}, args...), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestNewCreatesApp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My Notes")
	code, stdout, stderr := runNewCmd(t, "-skip-deps", dir)
	if code != 0 {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for file, wants := range map[string][]string{
		"go.mod":       {"module my-notes\n", "go " + templateGoVersion + "\n"},
		"main.go":      {"// Command my-notes is", `Title:     "My Notes"`, "func main() { tela.Main(App) }"},
		"main_test.go": {"telatest.MountApp(t, App)", `ByRole("button", "Increment")`},
		"README.md":    {"# My Notes", "tela dev"},
		".gitignore":   {"/my-notes\n", "/my-notes.exe\n"},
	} {
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Errorf("%s: %v", file, err)
			continue
		}
		for _, want := range wants {
			if !strings.Contains(string(b), want) {
				t.Errorf("%s does not contain %q:\n%s", file, want, b)
			}
		}
		if strings.Contains(string(b), "{{") {
			t.Errorf("%s has an unexpanded template action:\n%s", file, b)
		}
	}
	if !strings.Contains(stdout, "go.mod") || !strings.Contains(stdout, "tela dev") {
		t.Errorf("stdout should list the files and say what to do next:\n%s", stdout)
	}
	if !strings.Contains(stdout, "go get "+telaModule) {
		t.Errorf("-skip-deps should say how to add the dependencies:\n%s", stdout)
	}
}

func TestNewModuleFlagAndExistingEmptyDir(t *testing.T) {
	dir := t.TempDir() // exists and is empty
	code, stdout, stderr := runNewCmd(t, "-skip-deps", dir, "-module", "github.com/you/notes")
	if code != 0 {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.HasPrefix(string(b), "module github.com/you/notes\n") {
		t.Errorf("go.mod:\n%s", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(b) != "/notes\n/notes.exe\n" {
		t.Errorf(".gitignore:\n%s", b)
	}
}

func TestNewDot(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "here")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	code, stdout, stderr := runNewCmd(t, "-skip-deps", ".")
	if code != 0 {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	if b, _ := os.ReadFile("go.mod"); !strings.HasPrefix(string(b), "module here\n") {
		t.Errorf("go.mod:\n%s", b)
	}
	if strings.Contains(stdout, "cd .") {
		t.Errorf("stdout tells to cd into the current directory:\n%s", stdout)
	}
}

func TestNewErrors(t *testing.T) {
	notEmpty := t.TempDir()
	keep := filepath.Join(notEmpty, "keep.txt")
	if err := os.WriteFile(keep, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"no dir", nil, 2, "missing <dir>"},
		{"two dirs", []string{"a", "b"}, 2, "expected one <dir>"},
		{"unknown flag", []string{"-nope", "a"}, 2, "see \"tela help new\""},
		{"bad module", []string{"-module", "my app", "-skip-deps", filepath.Join(t.TempDir(), "x")}, 2, "invalid module path"},
		{"name makes no module", []string{"-skip-deps", filepath.Join(t.TempDir(), "日本語")}, 2, "pass one with -module"},
		{"not empty", []string{"-skip-deps", notEmpty}, 1, "keep.txt"},
	} {
		t.Run(c.name, func(t *testing.T) {
			code, _, stderr := runNewCmd(t, c.args...)
			if code != c.code || !strings.Contains(stderr, c.msg) {
				t.Errorf("exit code %d, stderr %q; want code %d containing %q", code, stderr, c.code, c.msg)
			}
		})
	}
	if b, _ := os.ReadFile(keep); string(b) != "mine" {
		t.Errorf("tela new touched a file in a directory it refused: %q", b)
	}
}

func TestNewHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"help", "new"}, &out, &errOut); code != 0 || out.String() != newUsage {
		t.Errorf("help new: code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	out.Reset()
	if code := run([]string{"new", "-h"}, &out, &errOut); code != 0 || out.String() != newUsage {
		t.Errorf("new -h: code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	out.Reset()
	run(nil, &out, &errOut)
	if !strings.Contains(errOut.String(), "\tnew ") {
		t.Errorf("the command list does not mention new:\n%s", errOut.String())
	}
}

// TestNewAppBuildsAndPasses generates an app, points it at this checkout of
// tela and runs go vet and go test on it, so the template cannot rot.
func TestNewAppBuildsAndPasses(t *testing.T) {
	skipShort(t)
	dir := filepath.Join(t.TempDir(), "gen")
	if code, stdout, stderr := runNewCmd(t, "-skip-deps", dir); code != 0 {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// Resolve offline: require tela from this checkout and take its own
	// requirements and checksums, which the module cache already holds
	// (go mod tidy would ask for the test-only modules of its dependencies).
	root := repoRoot(t)
	telaMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	requires := regexp.MustCompile(`(?s)\nrequire \(.*?\n\)\n`).Find(telaMod)
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil || requires == nil {
		t.Fatalf("go.mod or go.sum of tela unreadable: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "go.mod"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(f, "\nrequire %s v0.0.0\n\nreplace %s => %s\n%s", telaModule, telaModule, filepath.ToSlash(root), requires)
	f.Close()
	if err := os.WriteFile(filepath.Join(dir, "go.sum"), sum, 0o644); err != nil {
		t.Fatal(err)
	}

	env := append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	goRun := func(args ...string) ([]byte, error) {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = env
		return cmd.CombinedOutput()
	}
	for _, args := range [][]string{{"vet", "./..."}, {"test", "./..."}} {
		if out, err := goRun(args...); err != nil {
			t.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	// The app is a package tela understands: Previews of the App, aria text.
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
	if code := run([]string{"inspect", "."}, &out, &errOut); code != 0 {
		t.Fatalf("inspect: code %d\nstderr: %s", code, errOut.String())
	}
	if want := "- 'heading \"Count: 0\" [level=1]'\n- button \"Increment\"\n"; out.String() != want {
		t.Errorf("inspect printed\n%s\nwant\n%s", out.String(), want)
	}
}
