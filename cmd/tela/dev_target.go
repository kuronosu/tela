package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// The target of tela dev: what is built and how. A package main is built as
// is. A non-main package gets a generated main package in a subdirectory
// that exists only in a build overlay (go build -overlay), which imports it
// and runs its screen with tela.Main. A package main run with --preview gets
// a generated file in the package itself whose init function runs the
// preview and exits before the package's own main: a package main cannot be
// imported, and its Previews are reachable only from inside it. Either way
// nothing is written into the package's directory.

// devRunnerDir is the overlay-only directory of the generated main of a
// non-main package, inside the package directory (so it may import the
// package's internal packages).
const devRunnerDir = "zz_tela_dev_main"

// devInitFile is the overlay-only file added to a package main for
// --preview.
const devInitFile = "zz_tela_dev_preview.go"

// listedPackage is what go list -json says about a package.
type listedPackage struct {
	ImportPath string
	Dir        string
	Name       string
	GoFiles    []string
	EmbedFiles []string
	Standard   bool
	Module     *listedModule
	Error      *struct{ Err string }
}

type listedModule struct {
	Path    string
	Dir     string
	Main    bool
	Replace *listedModule
	Version string
}

// listFields are the fields go list prints (-json=...).
const listFields = "ImportPath,Dir,Name,GoFiles,EmbedFiles,Standard,Module,Error"

// goList runs go list -e -json with the tags of the dev builds (devTags of
// goflags, the user's GOFLAGS) in dir and decodes the stream of packages.
func goList(goBin, dir, goflags string, args ...string) ([]listedPackage, error) {
	all := append([]string{"list", "-e", "-json=" + listFields, "-tags", devTags(goflags)}, args...)
	cmd := exec.Command(goBin, all...)
	cmd.Dir = dir
	cmd.Env = goEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	var pkgs []listedPackage
	dec := json.NewDecoder(&stdout)
	for {
		var p listedPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			return nil, fmt.Errorf("unreadable go list output: %v", err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// devTarget is a resolved package to run.
type devTarget struct {
	pkg      listedPackage
	modDir   string   // the module root: the go command's working directory
	contract contract // for a generated runner
	hasApp   bool
	// arg is the package argument of go build, relative to modDir.
	arg string
	// overlay maps the absolute paths of generated files to their
	// contents (empty for a package main built as is).
	overlay map[string][]byte
	// goflags is the GOFLAGS the go command applies in modDir (readGoEnv),
	// for the flags and tags of the builds and of go list.
	goflags string
	// gocache and gotmpdir are GOCACHE and the user's GOTMPDIR there (the
	// slow-start hint and the builds' temporary files, dev_hint.go).
	gocache, gotmpdir string
}

// describe says what runs, for the start line.
func (t *devTarget) describe() string {
	switch {
	case len(t.overlay) == 0:
		return "package main"
	case t.contract == contractPreviews:
		return "its Previews"
	case t.contract == contractPreview:
		return "its Preview()"
	}
	return "its App"
}

// resolveDevTarget finds the package and decides how to build it.
func resolveDevTarget(goBin string, opts devOptions) (*devTarget, error) {
	// This go list runs here, with the GOFLAGS that apply here; the builds
	// and the later go list run in the target's module, with its own.
	here := readGoEnv(goBin, "", goEnv())
	pkgs, err := goList(goBin, "", here.flags, opts.Pkg)
	if err != nil {
		return nil, fmt.Errorf("cannot find package %q: %s\nuse a package path or a relative directory such as ./examples/counter", opts.Pkg, err)
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("%q does not name exactly one package (go list printed %d); give a single package, without wildcards", opts.Pkg, len(pkgs))
	}
	p := pkgs[0]
	if p.Dir == "" || p.Name == "" {
		msg := "no Go files"
		if p.Error != nil {
			msg = p.Error.Err
		}
		return nil, fmt.Errorf("cannot find package %q: %s\nuse a package path or a relative directory such as ./examples/counter", opts.Pkg, msg)
	}
	if p.Module == nil || p.Module.Dir == "" {
		return nil, fmt.Errorf("package %s (%s) is not in a module; tela dev runs packages of a module that requires github.com/kuronosu/tela", p.Name, opts.Pkg)
	}
	t := &devTarget{pkg: p, modDir: p.Module.Dir}
	// Usually tela dev runs in the target's module, whose GOFLAGS are those
	// read here; go env runs again only in another module (or workspace).
	env := here
	if !here.sameIn(t.modDir) {
		env = readGoEnv(goBin, t.modDir, goEnv())
	}
	t.goflags, t.gocache, t.gotmpdir = env.flags, env.gocache, env.gotmpdir
	rel, err := filepath.Rel(t.modDir, p.Dir)
	if err != nil {
		return nil, err
	}
	t.arg = "./" + filepath.ToSlash(rel)
	if p.Name == "main" && opts.Preview == "" {
		return t, nil
	}

	det, err := scanPackage(p.Dir, p.GoFiles)
	if err != nil {
		return nil, err
	}
	t.contract, t.hasApp = det.contract, det.hasApp
	switch t.contract {
	case contractNone:
		if p.Name == "main" {
			return nil, fmt.Errorf("--preview %q: package main (%s) defines no Previews; %s", opts.Preview, opts.Pkg, contractHelp)
		}
		return nil, fmt.Errorf("package %s (%s) cannot be run: it is not a package main and %s", p.Name, opts.Pkg, contractHelp)
	case contractScene:
		return nil, fmt.Errorf("package %s (%s) only defines Scene, a display list: tela dev runs an app; define func Preview() tela.Node, a Previews variable or an App", p.Name, opts.Pkg)
	}
	if opts.Preview != "" && t.contract != contractPreviews {
		what := "Preview(), a single screen"
		if t.contract == contractApp {
			what = "App, whose Root is a single screen"
		}
		return nil, fmt.Errorf("--preview %q: package %s defines %s; to choose among several, define var Previews = tela.Previews{...}", opts.Preview, p.Name, what)
	}
	src, err := generateDevRunner(devRunner{
		Main:       p.Name == "main",
		ImportPath: p.ImportPath,
		Title:      filepath.Base(p.Dir),
		Kind:       t.contract.String(),
		HasApp:     t.hasApp,
		Preview:    opts.Preview,
		Width:      defaultWidth,
		Height:     defaultHeight,
	})
	if err != nil {
		return nil, err
	}
	if p.Name == "main" {
		t.overlay = map[string][]byte{filepath.Join(p.Dir, devInitFile): src}
	} else {
		t.overlay = map[string][]byte{filepath.Join(p.Dir, devRunnerDir, "main.go"): src}
		t.arg += "/" + devRunnerDir
	}
	return t, nil
}

// depSet is what the watcher follows besides the assets: the roots of the
// modules whose packages the app builds from source (the main module, the
// modules of a go.work and local replace directives) and the files their
// packages embed.
type depSet struct {
	modules []string
	embeds  []string
	// pkgDirs are the directories of the packages built from those
	// modules (a package under testdata, which only a direct build
	// reaches, counts through them).
	pkgDirs []string
}

// listDeps lists the dependencies of the target with go list -deps.
func listDeps(goBin string, t *devTarget) (depSet, error) {
	pkgs, err := goList(goBin, t.modDir, t.goflags, "-deps", t.pkg.Dir)
	if err != nil {
		return depSet{}, fmt.Errorf("listing the dependencies of %s: %v", t.pkg.ImportPath, err)
	}
	var d depSet
	seen := map[string]bool{}
	for _, p := range pkgs {
		m := p.Module
		if p.Standard || m == nil {
			continue
		}
		dir := ""
		switch {
		case m.Main:
			dir = m.Dir
		case m.Replace != nil && m.Replace.Version == "" && m.Replace.Dir != "":
			dir = m.Replace.Dir // a local directory
		default:
			continue // the module cache: read-only
		}
		if dir != "" && !seen[dir] {
			seen[dir] = true
			d.modules = append(d.modules, filepath.Clean(dir))
		}
		d.pkgDirs = append(d.pkgDirs, filepath.Clean(p.Dir))
		for _, f := range p.EmbedFiles {
			d.embeds = append(d.embeds, filepath.Join(p.Dir, filepath.FromSlash(f)))
		}
	}
	if !seen[t.modDir] {
		d.modules = append(d.modules, filepath.Clean(t.modDir))
	}
	return d, nil
}

// devRunner are the parameters of the generated runner.
type devRunner struct {
	Main          bool // a file in the package main (init); otherwise a main package importing it
	ImportPath    string
	Title         string // the window title when the App gives none
	Kind          string // contract: Preview, Previews or App
	HasApp        bool
	Preview       string
	Width, Height float64
}

func generateDevRunner(r devRunner) ([]byte, error) {
	var buf bytes.Buffer
	ref := "telatarget."
	if r.Main {
		ref = ""
	}
	err := devRunnerTemplate.Execute(&buf, struct {
		devRunner
		Ref string
	}{r, ref})
	return buf.Bytes(), err
}

var devRunnerTemplate = template.Must(template.New("dev").Parse(`// Code generated by "tela dev"; DO NOT EDIT.

package main

import (
	telafmt "fmt"
	telaos "os"
	telasort "sort"
	telastrings "strings"

	telatela "github.com/kuronosu/tela"
{{- if not .Main}}
	telatarget {{printf "%q" .ImportPath}}
{{- end}}
)

var (
	_ = telasort.Strings
	_ = telastrings.Join
)

{{if .Main -}}
// The package's main does not run: this init runs the preview and exits.
func init() {
	telaDevRun()
	telaos.Exit(0)
}
{{- else -}}
func main() { telaDevRun() }
{{- end}}

// telaDevFail reports a problem of the package's contract and exits.
func telaDevFail(format string, args ...any) {
	telafmt.Fprintf(telaos.Stderr, "tela dev: "+format+"\n", args...)
	telaos.Exit(1)
}

func telaDevRun() {
	app := telatela.App{Title: {{printf "%q" .Title}}, Width: {{.Width}}, Height: {{.Height}}}
{{- if .HasApp}}
	// The App of the package: its window settings apply, as in tela snapshot.
	if a, ok := telaDevAppOf({{.Ref}}App); ok {
		w, h, title := app.Width, app.Height, app.Title
		app = a
		if app.Width <= 0 || app.Height <= 0 {
			app.Width, app.Height = w, h
		}
		if app.Title == "" {
			app.Title = title
		}
	}
{{- end}}
	app.Root = telaDevRoot()
	telatela.Main(app)
}
{{if .HasApp}}
func telaDevAppOf(v any) (telatela.App, bool) {
	switch a := v.(type) {
	case telatela.App:
		return a, true
	case *telatela.App:
		if a != nil {
			return *a, true
		}
	}
	return telatela.App{}, false
}
{{end}}
{{- if eq .Kind "Preview"}}
func telaDevRoot() telatela.Component {
	var root telatela.Component = {{.Ref}}Preview()
	if root == nil {
		telaDevFail("Preview() returned nil; return the screen, such as tela.C(Screen) or a struct component")
	}
	return root
}
{{- else if eq .Kind "Previews"}}
func telaDevRoot() telatela.Component {
	var previews telatela.Previews = {{.Ref}}Previews
	names := make([]string, 0, len(previews))
	for name := range previews {
		names = append(names, name)
	}
	telasort.Strings(names)
	if len(names) == 0 {
		telaDevFail("Previews is empty; add at least one screen: tela.Previews{\"Name\": {Root: tela.C(Screen)}}")
	}
	name := {{printf "%q" .Preview}}
	if name == "" {
		name = names[0]
	}
	p, ok := previews[name]
	if !ok {
		telaDevFail("unknown preview %q; the package defines: %s", name, telastrings.Join(names, ", "))
	}
	if p.Root == nil {
		telaDevFail("preview %q has no Root; give it one: {Root: tela.C(Screen)}", name)
	}
	return p
}
{{- else}}
func telaDevRoot() telatela.Component {
	a, ok := telaDevAppOf({{.Ref}}App)
	if !ok {
		telaDevFail("App is a %T, not a tela.App; define func Preview() tela.Node, or var App = tela.App{Root: tela.C(Screen)}", {{.Ref}}App)
	}
	if a.Root == nil {
		telaDevFail("App has no Root; set it to the root component, such as tela.App{Root: tela.C(Screen)}")
	}
	return a.Root
}
{{- end}}
`))
