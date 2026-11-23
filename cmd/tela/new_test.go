package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"slices"
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

// unexpandedAction matches a field of newData left in a written file (the
// AGENTS.md has a literal {{.Dir}}, the go list template it tells to run).
var unexpandedAction = regexp.MustCompile(`\{\{\s*\.(Module|Binary|Title|GoVersion|Template|Description)\b`)

func TestNewCreatesApp(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "My Notes")
	code, stdout, stderr := runNewCmd(t, "-skip-deps", dir)
	if code != 0 {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	for file, wants := range map[string][]string{
		"go.mod":       {"module my-notes\n", "go " + templateGoVersion + "\n"},
		"main.go":      {"// Command my-notes is", `Title:  "My Notes"`, "func main() { tela.Main(App) }"},
		"main_test.go": {"telatest.MountApp(t, App)", `ByRole("button", "Increment")`},
		"README.md":    {"# My Notes", "tela dev"},
		".gitignore":   {"/my-notes\n", "/my-notes.exe\n"},
		"AGENTS.md":    {"My Notes is a [Tela]", "https://telaui.dev/llms.txt", "https://telaui.dev/llms/NAME.md", "go list -m -f '{{.Dir}}' github.com/kuronosu/tela)/docs/llms.txt", "docs/llms/", "tela check .", "tela lint ./...", "CGO_ENABLED=0 go test ./..."},
		"CLAUDE.md":    {"@AGENTS.md\n"},
		".mcp.json":    {`"command": "tela"`, `"args": ["mcp", "."]`},
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
		if unexpandedAction.Match(b) {
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

// TestNewAgentFiles checks the files for coding agents of every template: the
// guide names only commands and flags the CLI has, CLAUDE.md only imports it
// and .mcp.json is a valid server entry that starts "tela mcp" on the app.
func TestNewAgentFiles(t *testing.T) {
	for _, name := range templateNames {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "my-app")
			if code, stdout, stderr := runNewCmd(t, "-skip-deps", "-template", name, "-module", "example.com/me/my-app", dir); code != 0 {
				t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
			if err != nil {
				t.Fatal(err)
			}
			if n := len(strings.Fields(string(agents))); n > 400 {
				t.Errorf("AGENTS.md has %d words; keep it under 400", n)
			}
			for _, want := range []string{"tela snapshot", "tela inspect", "tela lint", "tela mcp", "tela dev", "CGO_ENABLED=0", "llms.txt"} {
				if !strings.Contains(string(agents), want) {
					t.Errorf("AGENTS.md does not mention %q", want)
				}
			}
			if unexpandedAction.Match(agents) {
				t.Errorf("AGENTS.md has an unexpanded template action:\n%s", agents)
			}
			if claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); string(claude) != "@AGENTS.md\n" {
				t.Errorf("CLAUDE.md = %q, want only the import of AGENTS.md", claude)
			}
			b, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
			if err != nil {
				t.Fatal(err)
			}
			var cfg struct {
				MCPServers map[string]struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(b, &cfg); err != nil {
				t.Fatalf(".mcp.json is not valid JSON: %v\n%s", err, b)
			}
			srv := cfg.MCPServers["tela"]
			if srv.Command != "tela" || !slices.Equal(srv.Args, []string{"mcp", "."}) {
				t.Errorf(".mcp.json server = %+v, want tela mcp .", srv)
			}
			// What "tela mcp ." accepts is what the file says.
			if _, err := parseMCPArgs(srv.Args[1:]); err != nil {
				t.Errorf(".mcp.json arguments: %v", err)
			}
		})
	}
}

// TestNewTemplates writes each example template and checks what it has:
// the module path in place of the placeholder, the requirements of the
// example, no template suffixes, no repository-only files.
func TestNewTemplates(t *testing.T) {
	for _, c := range []struct {
		name     string
		requires string   // in go.mod
		files    []string // written
		imports  string   // a file that imports the example's icons package, by name
		pkg      string
	}{
		{"notes", "\tmodernc.org/sqlite v", []string{"main.go", "store.go", "icons/icons_gen.go", "icons/LICENSE", "testdata/notes.aria.yml", "notes_test.go"}, "editor.go", "icons"},
		{"settings", "", []string{"main.go", "form.go", "lucide/icons_gen.go", "testdata/settings.aria.yml", "settings_test.go"}, "main.go", "lucide"},
		{"dashboard", "", []string{"main.go", "overview.go", "icons/icons_gen.go", "img/dunes-dark.jpg", "testdata/palette.aria.yml", "dashboard_test.go"}, "main.go", "icons"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "my-app")
			code, stdout, stderr := runNewCmd(t, "-skip-deps", "-template", c.name, "-module", "example.com/me/my-app", dir)
			if code != 0 {
				t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			for _, f := range c.files {
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
					t.Errorf("the template did not write %s: %v", f, err)
				}
			}
			b, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
			if !strings.HasPrefix(string(b), "module example.com/me/my-app\n") || !strings.Contains(string(b), c.requires) {
				t.Errorf("go.mod (want it to contain %q):\n%s", c.requires, b)
			}
			if r, _ := os.ReadFile(filepath.Join(dir, "README.md")); !strings.Contains(string(r), "`"+c.name+"` template") {
				t.Errorf("README.md does not say which template it is:\n%s", r)
			}
			if src, _ := os.ReadFile(filepath.Join(dir, c.imports)); !strings.Contains(string(src), `"example.com/me/my-app/`+c.pkg+`"`) {
				t.Errorf("%s does not import example.com/me/my-app/%s:\n%s", c.imports, c.pkg, src)
			}
			filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(dir, p)
				base := d.Name()
				switch {
				case strings.HasSuffix(base, ".tmpl"), base == "go.sum", strings.HasSuffix(base, ".png") && strings.Contains(rel, "testdata"):
					t.Errorf("%s should not be written", rel)
				}
				if strings.HasSuffix(base, ".go") || base == "go.mod" {
					src, _ := os.ReadFile(p)
					for _, bad := range []string{templateModule, "kuronosu/tela/examples", "kuronosu/tela/internal"} {
						if strings.Contains(string(src), bad) {
							t.Errorf("%s still mentions %s", rel, bad)
						}
					}
				}
				return nil
			})
		})
	}
}

// templateModule is the placeholder the generator puts in the templates:
// internal/gen/apptemplates has the same constant.
func TestTemplateModulePlaceholder(t *testing.T) {
	b, err := templates.ReadFile("templates/notes/editor.go.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"`+templateModule+`/icons"`) {
		t.Errorf("templates/notes/editor.go.tmpl does not import %s/icons: the generator and new.go disagree on the placeholder", templateModule)
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
		{"unknown template", []string{"-template", "blog", "-skip-deps", filepath.Join(t.TempDir(), "x")}, 2, `unknown template "blog": choose one of counter, notes, settings, dashboard`},
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

// TestNewAppBuildsAndPasses generates each template, points it at this
// checkout of tela and runs go vet and go test on it, so the templates
// cannot rot.
func TestNewAppBuildsAndPasses(t *testing.T) {
	skipShort(t)
	for _, name := range templateNames {
		t.Run(name, func(t *testing.T) { testNewApp(t, name) })
	}
}

func testNewApp(t *testing.T, name string) {
	dir := filepath.Join(t.TempDir(), "gen")
	if code, stdout, stderr := runNewCmd(t, "-skip-deps", "-template", name, dir); code != 0 {
		t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	// Resolve offline: require tela from this checkout and take its own
	// requirements and checksums, which the module cache already holds
	// (go mod tidy would ask for the test-only modules of its dependencies).
	// The notes template also needs what examples/notes requires (SQLite),
	// with that module's checksums.
	root := repoRoot(t)
	modRoot := root
	if _, err := os.Stat(filepath.Join(root, "examples", name, "go.mod")); err == nil {
		modRoot = filepath.Join(root, "examples", name)
	}
	gomod, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	telaMod, err := os.ReadFile(filepath.Join(modRoot, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var requires []string
	for _, block := range regexp.MustCompile(`(?s)\nrequire \(.*?\n\)\n`).FindAll(telaMod, -1) {
		var keep []string
		for _, line := range strings.Split(string(block), "\n") {
			// What the generated go.mod already requires, and tela itself
			// (replaced below), stay out.
			fields := strings.Fields(line)
			if len(fields) > 0 && (fields[0] == telaModule || strings.Contains(string(gomod), "\t"+fields[0]+" ")) {
				continue
			}
			keep = append(keep, line)
		}
		requires = append(requires, strings.Join(keep, "\n"))
	}
	sum, err := os.ReadFile(filepath.Join(modRoot, "go.sum"))
	if err != nil || len(requires) == 0 {
		t.Fatalf("go.mod or go.sum of %s unreadable: %v", modRoot, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "go.mod"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(f, "\nrequire %s v0.0.0\n\nreplace %s => %s\n%s\n", telaModule, telaModule, filepath.ToSlash(root), strings.Join(requires, "\n"))
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
	// The modules the template adds must be in the module cache.
	for _, mod := range templateRequirements(t, name) {
		if out, err := goRun("mod", "download", mod); err != nil {
			offlineUnresolved(t, "cannot resolve %s offline (go mod download: %v):\n%s", mod, err, out)
		}
	}
	for _, args := range [][]string{{"vet", "./..."}, {"test", "./..."}} {
		if out, err := goRun(args...); err != nil {
			t.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	if name != "counter" {
		return
	}

	// The app is a package tela understands: Previews of the App, aria text.
	t.Chdir(dir)
	var out, errOut bytes.Buffer
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
	if code := run([]string{"inspect", "."}, &out, &errOut); code != 0 {
		t.Fatalf("inspect: code %d\nstderr: %s", code, errOut.String())
	}
	if want := "- text: COUNT\n- heading \"0\" [level=1]\n- button \"Increment\"\n"; stripSrc(out.String()) != want {
		t.Errorf("inspect printed\n%s\nwant\n%s", out.String(), want)
	}
}

// templateRequirements are the "path@version" of the modules a template
// requires besides tela.
func templateRequirements(t *testing.T, name string) []string {
	t.Helper()
	b, err := templates.ReadFile("templates/" + name + "/requires.txt")
	if err != nil {
		return nil
	}
	var mods []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		path, version, _ := strings.Cut(line, " ")
		mods = append(mods, path+"@"+version)
	}
	return mods
}
