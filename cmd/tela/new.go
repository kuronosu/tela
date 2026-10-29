package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"text/template"
	"unicode"
)

const newUsage = `Usage:

	tela new [flags] <dir>

Creates a Tela app in <dir>: a go.mod, a main.go with a counter, a test for
it that runs without a window, a README and a .gitignore. <dir> is created
when it does not exist; it may be an existing empty directory, or "." for the
current one. Then it adds the tela module to go.mod and runs go mod tidy, so
the app is ready to run:

	tela new my-app
	cd my-app
	tela dev

Flags:

	-module path   module path of the app (default: the name of <dir>, for
	               example github.com/you/my-app)
	-skip-deps     do not run go get and go mod tidy (no network, no go command)

The tela module is required at the version of this binary when it is a
release, and at the latest release otherwise.
`

//go:embed templates/app
var appTemplates embed.FS

// telaModule is the module the generated app requires.
const telaModule = "github.com/kuronosu/tela"

// templateGoVersion is the go directive of a generated go.mod: the one tela's
// own go.mod has (a test checks it).
const templateGoVersion = "1.27"

// templateFiles maps the files of templates/app to the names they are
// written under (a leading dot, or a name the go command would read, cannot
// be embedded as is).
var templateFiles = map[string]string{
	"go.mod.tmpl":       "go.mod",
	"main.go.tmpl":      "main.go",
	"main_test.go.tmpl": "main_test.go",
	"README.md.tmpl":    "README.md",
	"gitignore.tmpl":    ".gitignore",
}

// newData is what the templates see.
type newData struct {
	Module    string // module path
	Binary    string // name of the executable: the last element of Module
	Title     string // window title and README heading
	GoVersion string
}

// runNew implements `tela new`.
func runNew(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tela new", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	module := fs.String("module", "", "")
	skipDeps := fs.Bool("skip-deps", false, "")

	// Flags and the directory may come in any order, as in the other commands.
	dirs, err := parsePositionals("new", fs, args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, newUsage)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "tela new: %v\n", err)
		return 2
	}
	if len(dirs) != 1 {
		if len(dirs) == 0 {
			fmt.Fprintf(stderr, "tela new: missing <dir>: say where to create the app, for example \"tela new my-app\"\n")
		} else {
			fmt.Fprintf(stderr, "tela new: expected one <dir>, got %d (%s)\n", len(dirs), strings.Join(dirs, ", "))
		}
		return 2
	}

	dir, err := filepath.Abs(dirs[0])
	if err != nil {
		fmt.Fprintf(stderr, "tela new: %v\n", err)
		return 1
	}
	name := filepath.Base(dir)
	mod := *module
	if mod == "" {
		mod = moduleFromName(name)
	}
	if err := checkModulePath(mod); err != nil {
		fmt.Fprintf(stderr, "tela new: %v\n", err)
		if *module == "" {
			fmt.Fprintf(stderr, "the directory name %q does not make a module path; pass one with -module\n", name)
		}
		return 2
	}
	if err := checkTargetDir(dir); err != nil {
		fmt.Fprintf(stderr, "tela new: %v\n", err)
		return 1
	}

	binary := mod[strings.LastIndex(mod, "/")+1:]
	data := newData{Module: mod, Binary: binary, Title: titleFromName(binary), GoVersion: templateGoVersion}
	written, err := writeApp(dir, data)
	if err != nil {
		fmt.Fprintf(stderr, "tela new: %v\n", err)
		return 1
	}

	shown := dirs[0]
	fmt.Fprintf(stdout, "created %s (module %s):\n", shown, mod)
	for _, f := range written {
		fmt.Fprintf(stdout, "\t%s\n", f)
	}

	if !*skipDeps {
		if err := fetchDeps(dir, stderr); err != nil {
			fmt.Fprintf(stderr, "tela new: the app was created, but its dependencies were not added: %v\n"+
				"run these in %s once the problem is fixed:\n\n\tgo get %s@latest\n\tgo mod tidy\n", err, shown, telaModule)
			return 1
		}
	}

	var steps []string
	if shown != "." {
		steps = append(steps, "cd "+shown)
	}
	intro := "\ngo to the app and start it with live reload:\n\n"
	if *skipDeps {
		intro = "\nadd the dependencies, then start it with live reload:\n\n"
		steps = append(steps, "go get "+telaModule+"@latest", "go mod tidy")
	}
	steps = append(steps, "tela dev")
	next := intro + "\t" + strings.Join(steps, "\n\t") + "\n"
	fmt.Fprint(stdout, next)
	return 0
}

// writeApp renders the templates into dir, creating it, and returns the names
// of the files it wrote in a fixed order.
func writeApp(dir string, data newData) ([]string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(appTemplates, "templates/app")
	if err != nil {
		return nil, err
	}
	var written []string
	for _, e := range entries {
		out, ok := templateFiles[e.Name()]
		if !ok {
			return nil, fmt.Errorf("internal error: template %s has no output name", e.Name())
		}
		src, err := fs.ReadFile(appTemplates, "templates/app/"+e.Name())
		if err != nil {
			return nil, err
		}
		tmpl, err := template.New(e.Name()).Parse(string(src))
		if err != nil {
			return nil, fmt.Errorf("internal error: template %s: %v", e.Name(), err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("internal error: template %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, out), buf.Bytes(), 0o644); err != nil {
			return nil, err
		}
		written = append(written, out)
	}
	return written, nil
}

// checkTargetDir accepts a directory that does not exist or is empty.
func checkTargetDir(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("%s is not empty (it has %s); choose a new or empty directory", dir, entries[0].Name())
	}
	return nil
}

// fetchDeps adds the tela module to the go.mod of dir and tidies it. It
// runs without cgo like everything else in the project.
func fetchDeps(dir string, stderr io.Writer) error {
	if _, err := exec.LookPath("go"); err != nil {
		return errors.New("the go command is not in PATH")
	}
	for _, args := range [][]string{
		{"get", telaModule + "@" + telaVersion(debug.ReadBuildInfo())},
		{"mod", "tidy"},
	} {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprint(stderr, string(out))
			return fmt.Errorf("go %s: %v", strings.Join(args, " "), err)
		}
	}
	return nil
}

// telaVersion is the version of the tela module to require: the one this
// binary was built from when it is a release or a pseudo-version, "latest"
// when it was built from a checkout.
func telaVersion(info *debug.BuildInfo, ok bool) string {
	if ok && info.Main.Path == telaModule {
		if v := info.Main.Version; strings.HasPrefix(v, "v") && !strings.HasSuffix(v, "+dirty") {
			return v
		}
	}
	return "latest"
}

// moduleFromName makes a module path out of a directory name: lowercase
// letters, digits and "-", "_" or "." between them.
func moduleFromName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-_.")
}

// checkModulePath reports why path cannot be a module path, using a subset
// of golang.org/x/mod/module's rules (the same characters, no empty or dotted
// elements) so the go command accepts what tela new writes.
func checkModulePath(path string) error {
	if path == "" {
		return errors.New("empty module path")
	}
	for _, elem := range strings.Split(path, "/") {
		if elem == "" || strings.HasPrefix(elem, ".") || strings.HasSuffix(elem, ".") {
			return fmt.Errorf("invalid module path %q: elements cannot be empty or start or end with a dot", path)
		}
		for _, r := range elem {
			if !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) || strings.ContainsRune("-._~", r)) {
				return fmt.Errorf("invalid module path %q: %q is not allowed", path, r)
			}
		}
	}
	return nil
}

// titleFromName turns "my-app" into "My App" for the window title.
func titleFromName(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	if len(words) == 0 {
		return "Tela App"
	}
	return strings.Join(words, " ")
}
