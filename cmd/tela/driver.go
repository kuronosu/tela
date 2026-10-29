package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
)

// How the commands run a package. A package main cannot be imported, so
// instead of building a separate main, `go test -overlay` adds a _test.go
// file to the target package. The file is generated for the contract the
// package implements (Previews, Preview or Scene), runs inside the package
// with access to its declarations, and writes its result to a file whose
// path arrives in an environment variable. The overlay means nothing is
// written into the package's directory.
//
// Screens are rendered through the public telatest package (Render,
// AriaSnapshot, LayoutJSON), so the commands work on any module that
// requires github.com/kuronosu/tela (ADR-059, which lifts the limitation
// of ADR-047). Only the phase-1 Scene contract still imports tela's
// internal packages, and so only works inside the tela module.

// mode is what the generated test does.
type mode string

const (
	modeSnapshot mode = "snapshot" // render to a PNG
	modeInspect  mode = "inspect"  // print the layout tree as JSON
	modeAria     mode = "aria"     // print the aria snapshot as YAML
)

// contract is how a package exposes its screens.
type contract int

const (
	contractNone     contract = iota
	contractPreview           // func Preview() tela.Node (or tela.Component)
	contractPreviews          // var Previews = tela.Previews{...}
	contractApp               // var App = tela.App{...}, whose Root is the screen
	contractScene             // func Scene(w, h float32) *gfx.DisplayList (phase 1)
)

func (c contract) String() string {
	switch c {
	case contractPreview:
		return "Preview"
	case contractPreviews:
		return "Previews"
	case contractApp:
		return "App"
	case contractScene:
		return "Scene"
	}
	return "none"
}

// contractHelp explains the contracts, for errors.
const contractHelp = `it must define one of

	func Preview() tela.Node        (or tela.Component)
	var Previews = tela.Previews{"Name": {Root: tela.C(Screen)}}
	var App = tela.App{Root: tela.C(Screen), ...}

(a package that defines App renders in the size, class, color scheme, theme and
aliases of that App; alone, its Root is the screen. The phase-1 contract,
func Scene(w, h float32) *gfx.DisplayList, also works for "tela snapshot")`

// Names of the environment variables that carry the parameters to the
// generated test (rather than test flags, which could clash with the
// package's own).
const (
	envMode    = "TELA_RUN_MODE"
	envW       = "TELA_RUN_W"
	envH       = "TELA_RUN_H"
	envScale   = "TELA_RUN_SCALE"
	envDark    = "TELA_RUN_DARK"
	envSizeSet = "TELA_RUN_SIZE_SET" // "true" when --size was given, which beats App.Width and Height
	envPreview = "TELA_RUN_PREVIEW"
	envOut     = "TELA_RUN_OUT"  // the result: a PNG, JSON or YAML
	envErr     = "TELA_RUN_ERR"  // a message for the user, when the run fails
	envInfo    = "TELA_RUN_INFO" // JSON runInfo
	envLog     = "TELA_RUN_LOG"  // what the run printed on standard error (dev warnings and class errors)
)

// generatedFile is the name of the generated test file inside the package
// directory (it exists only in the overlay).
const generatedFile = "zz_tela_run_test.go"

// goPackage is what `go list` says about a package.
type goPackage struct {
	Dir, Name string
	// GoFiles are the package's source files and its in-package test
	// files: the runner is compiled by go test, so a Preview declared in a
	// _test.go file counts.
	GoFiles []string
}

// listPackage returns the directory, name and files of pkg.
func listPackage(goBin, pkg string) (goPackage, error) {
	// One line per package: dir|name|file,file...
	cmd := exec.Command(goBin, "list", "-f", `{{.Dir}}|{{.Name}}|{{join .GoFiles ","}}{{if and .GoFiles .TestGoFiles}},{{end}}{{join .TestGoFiles ","}}`, pkg)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return goPackage{}, fmt.Errorf("cannot find package %q: %s\nuse a package path or a relative directory such as ./examples/counter", pkg, msg)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 1 {
		return goPackage{}, fmt.Errorf("%q does not name exactly one package (go list printed %d); give a single package, without wildcards", pkg, len(lines))
	}
	parts := strings.SplitN(lines[0], "|", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" {
		return goPackage{}, fmt.Errorf("%q does not name a Go package (go list printed %q)", pkg, lines[0])
	}
	var files []string
	if parts[2] != "" {
		files = strings.Split(parts[2], ",")
	}
	return goPackage{Dir: parts[0], Name: parts[1], GoFiles: files}, nil
}

// detected is what the source of a package declares.
type detected struct {
	contract contract
	// hasApp: the package declares a `var App` that may be a tela.App,
	// whose settings the run applies.
	hasApp bool
	// previews are the constant string keys of the literal assigned to the
	// package-level var Previews, sorted; nil when there is none or its keys
	// are not constants (the names are then known once the instance mounts).
	// They go into the description of the open tool of tela mcp, which a
	// client reads before any call.
	previews []string
}

// mayBeTelaApp reports whether the i-th name of a var spec can be a
// tela.App: a variable called App of an obviously other type (a literal of
// another type, say) belongs to the package and is left alone, while
// anything else (a call, a variable) may be the app. The generated runner
// decides those with a type assertion that compiles for any type
// (telaAppOf: a tela.App or a non-nil *tela.App), so a variable App of
// another type is ignored there too.
func mayBeTelaApp(spec *ast.ValueSpec, i int) bool {
	// isApp: tela.App (any import name), or App through a dot import.
	isApp := func(e ast.Expr) bool {
		if star, ok := e.(*ast.StarExpr); ok {
			e = star.X
		}
		switch e := e.(type) {
		case *ast.SelectorExpr:
			return e.Sel.Name == "App"
		case *ast.Ident:
			return e.Name == "App"
		}
		return false
	}
	if spec.Type != nil {
		return isApp(spec.Type)
	}
	if len(spec.Values) != len(spec.Names) {
		return true // var a, App = f(): one call for several names
	}
	v := spec.Values[i]
	if u, ok := v.(*ast.UnaryExpr); ok && u.Op == token.AND {
		v = u.X // &tela.App{...}
	}
	switch v := v.(type) {
	case *ast.CompositeLit:
		return isApp(v.Type)
	case *ast.BasicLit, *ast.FuncLit, *ast.UnaryExpr, *ast.BinaryExpr:
		return false
	}
	return true
}

// previewKeys returns the constant string keys of the composite literal that
// the i-th name of a var Previews spec is initialized with.
func previewKeys(spec *ast.ValueSpec, i int) []string {
	if i >= len(spec.Values) {
		return nil
	}
	lit, ok := spec.Values[i].(*ast.CompositeLit)
	if !ok {
		return nil
	}
	var keys []string
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if b, ok := kv.Key.(*ast.BasicLit); ok && b.Kind == token.STRING {
			if s, err := strconv.Unquote(b.Value); err == nil {
				keys = append(keys, s)
			}
		}
	}
	return keys
}

// scanPackage parses the declarations of the package files and reports which
// contract the package's source implements. It parses the declarations only,
// so it works before anything compiles. When several are defined, Previews
// wins over Preview, which wins over Scene, which wins over App (a package
// that defines App next to another contract still renders in that App).
func scanPackage(dir string, files []string) (detected, error) {
	var hasPreview, hasPreviews, hasScene, hasApp bool
	var previews []string
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return detected{}, fmt.Errorf("cannot read %s: %v", name, err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil {
					continue
				}
				switch d.Name.Name {
				case "Preview":
					hasPreview = true
				case "Scene":
					hasScene = true
				}
			case *ast.GenDecl:
				if d.Tok != token.VAR {
					continue
				}
				for _, s := range d.Specs {
					spec := s.(*ast.ValueSpec)
					for i, n := range spec.Names {
						switch n.Name {
						case "Previews":
							hasPreviews = true
							for _, k := range previewKeys(spec, i) {
								if !slices.Contains(previews, k) {
									previews = append(previews, k)
								}
							}
						case "App":
							hasApp = hasApp || mayBeTelaApp(spec, i)
						}
					}
				}
			}
		}
	}
	sort.Strings(previews)
	d := detected{hasApp: hasApp, previews: previews}
	switch {
	case hasPreviews:
		d.contract = contractPreviews
	case hasPreview:
		d.contract = contractPreview
	case hasScene:
		d.contract = contractScene
	case hasApp:
		d.contract = contractApp
	}
	return d, nil
}

// runInfo is what the generated test reports about the run.
type runInfo struct {
	// Preview is the name of the rendered preview ("" for a package that
	// defines Preview or Scene) and Names all the names, sorted.
	Preview string   `json:"preview"`
	Names   []string `json:"names"`
}

// runResult is the outcome of runInPackage.
type runResult struct {
	Output []byte // the PNG, the JSON or the YAML
	Info   runInfo
}

// runInPackage renders opts.Pkg with the generated test and returns its
// output file. ext is the extension of the temporary output ("png", "json",
// "yml").
// Warnings go to stderr, and so does what the run printed on its own
// standard error: the dev warnings and class errors of the screen, which
// the passing go test would otherwise hide.
func runInPackage(m mode, opts renderOptions, ext string, stderr io.Writer) (*runResult, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, errors.New("the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
	}
	pkg, err := listPackage(goBin, opts.Pkg)
	if err != nil {
		return nil, err
	}
	det, err := scanPackage(pkg.Dir, pkg.GoFiles)
	if err != nil {
		return nil, err
	}
	c := det.contract
	if err := checkContract(m, c, opts, pkg); err != nil {
		return nil, err
	}
	if c == contractScene && opts.Dark {
		fmt.Fprintf(stderr, "tela: warning: --dark has no effect on package %s: Scene builds a display list without styles\n", pkg.Name)
	}

	tmp, err := os.MkdirTemp("", "tela-run-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, generatedFile)
	if err := os.WriteFile(src, generateSource(pkg.Name, c, det.hasApp && c != contractScene), 0o644); err != nil {
		return nil, err
	}
	overlay := filepath.Join(tmp, "overlay.json")
	js, err := json.Marshal(map[string]map[string]string{"Replace": {filepath.Join(pkg.Dir, generatedFile): src}})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(overlay, js, 0o644); err != nil {
		return nil, err
	}

	// The test writes into the temporary directory, so that a failed run
	// never leaves a stale or half-written file at the requested path.
	outFile := filepath.Join(tmp, "out."+ext)
	errFile := filepath.Join(tmp, "error.txt")
	infoFile := filepath.Join(tmp, "info.json")
	logFile := filepath.Join(tmp, "stderr.txt")
	defer func() {
		if b, err := os.ReadFile(logFile); err == nil && len(b) > 0 {
			stderr.Write(b)
		}
	}()
	cmd := exec.Command(goBin, "test", "-count=1", "-run", "^TestTelaRun$", "-overlay", overlay, opts.Pkg)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		envMode+"="+string(m),
		envW+"="+strconv.FormatFloat(opts.W, 'g', -1, 64),
		envH+"="+strconv.FormatFloat(opts.H, 'g', -1, 64),
		envScale+"="+strconv.FormatFloat(opts.Scale, 'g', -1, 64),
		envDark+"="+strconv.FormatBool(opts.Dark),
		envSizeSet+"="+strconv.FormatBool(opts.SizeSet),
		envPreview+"="+opts.Preview,
		envOut+"="+outFile,
		envErr+"="+errFile,
		envInfo+"="+infoFile,
		envLog+"="+logFile,
	)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		if msg, rerr := os.ReadFile(errFile); rerr == nil && len(bytes.TrimSpace(msg)) > 0 {
			return nil, errors.New(string(bytes.TrimSpace(msg)))
		}
		return nil, explainFailure(opts.Pkg, pkg.Name, c, output.String())
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		// The test passed but wrote nothing: the package defines its own
		// TestTelaRun, or filtered it out.
		return nil, fmt.Errorf("go test succeeded but wrote no output; the output was:\n%s", strings.TrimRight(output.String(), "\n"))
	}
	res := &runResult{Output: data}
	if b, err := os.ReadFile(infoFile); err == nil {
		if err := json.Unmarshal(b, &res.Info); err != nil {
			return nil, fmt.Errorf("unreadable run info: %v", err)
		}
	}
	return res, nil
}

// checkContract rejects a command the package cannot serve, before any
// compilation.
func checkContract(m mode, c contract, opts renderOptions, pkg goPackage) error {
	switch {
	case c == contractNone:
		return fmt.Errorf("package %s (%s) cannot be rendered: %s", pkg.Name, opts.Pkg, contractHelp)
	case c == contractScene && m != modeSnapshot:
		return fmt.Errorf("package %s (%s) only defines Scene, a display list without a layout tree: tela inspect needs func Preview() tela.Node, a Previews variable or an App", pkg.Name, opts.Pkg)
	case opts.Preview != "" && c == contractPreview:
		return fmt.Errorf("--preview %q: package %s defines Preview(), a single screen; to choose among several, define var Previews = tela.Previews{...}", opts.Preview, pkg.Name)
	case opts.Preview != "" && c == contractApp:
		return fmt.Errorf("--preview %q: package %s defines App, whose Root is a single screen; to choose among several, define var Previews = tela.Previews{...}", opts.Preview, pkg.Name)
	case opts.Preview != "" && c == contractScene:
		return fmt.Errorf("--preview %q: package %s defines Scene, a single scene; to choose among several screens, define var Previews = tela.Previews{...}", opts.Preview, pkg.Name)
	}
	return nil
}

// explainFailure turns the output of a failed `go test` into an error. A
// missing or mistyped contract declaration, the usual mistake, gets a
// friendly message, and so does a Previews map written in the form before
// tela.Preview; anything else is passed through as go printed it.
func explainFailure(pkg, name string, c contract, output string) error {
	output = strings.TrimRight(output, "\n")
	// The compile errors of the generated file: <file>:<line>:<col>: msg.
	// (A panic's stack trace names the file with a line only.)
	if compileError.MatchString(output) {
		for _, d := range []contract{c, contractApp} {
			var want string
			switch d {
			case contractPreview:
				want = "func Preview() tela.Node   (or tela.Component)"
			case contractPreviews:
				want = "var Previews = tela.Previews{\"Name\": {Root: tela.C(Screen)}}   (a map[string]tela.Preview)"
			case contractApp:
				want = "var App = tela.App{...}"
			case contractScene:
				want = "func Scene(w, h float32) *gfx.DisplayList"
			}
			if want != "" && (strings.Contains(output, "cannot use "+d.String()) || strings.Contains(output, "undefined: "+d.String())) {
				return fmt.Errorf("package %s (%s) cannot be rendered: its %s has the wrong type; it must be\n\n\t%s\n\ngo reported:\n%s", name, pkg, d, want, output)
			}
		}
	}
	if strings.Contains(output, "as tela.Preview value in map literal") {
		return fmt.Errorf("go test failed:\n%s\n\nthe entries of tela.Previews are tela.Preview values; give each one a Root:\n\n\tbefore: \"Login\": LoginScreen\n\tafter:  \"Login\": {Root: tela.C(LoginScreen)}   (or a struct component: {Root: Login{}})", output)
	}
	return fmt.Errorf("go test failed:\n%s", output)
}

// compileError matches an error of the compiler in the generated file.
var compileError = regexp.MustCompile(regexp.QuoteMeta(generatedFile) + `:\d+:\d+: `)

// writeFileAtomic writes data to path, creating its directory. The data
// goes to a temporary file in the same directory that is renamed over
// path, so a failed write never leaves a truncated file at path.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, 0o644)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// generateSource returns the source of the generated test for a package
// called pkgName that implements contract c; hasApp says the package also
// declares `var App`, whose settings the run applies. All imports have unusual
// aliases because import names live in the file scope of a package whose
// declarations they could collide with.
func generateSource(pkgName string, c contract, hasApp bool) []byte {
	var buf bytes.Buffer
	err := generatedTemplate.Execute(&buf, struct {
		Package string
		Kind    string
		Scene   bool
		HasApp  bool
		Env     map[string]string
	}{
		Package: pkgName, Kind: c.String(), Scene: c == contractScene, HasApp: hasApp,
		Env: map[string]string{
			"Mode": envMode, "W": envW, "H": envH, "Scale": envScale, "Dark": envDark, "SizeSet": envSizeSet,
			"Preview": envPreview, "Out": envOut, "Err": envErr, "Info": envInfo, "Log": envLog,
		},
	})
	if err != nil {
		panic(err) // the template is a constant
	}
	return buf.Bytes()
}

var generatedTemplate = template.Must(template.New("run").Parse(`// Code generated by "tela"; DO NOT EDIT.

package {{.Package}}

import (
	telafmt "fmt"
	telaos "os"
	telastrconv "strconv"
	telatesting "testing"

{{- if .Scene}}
	telaimage "image"
	telapng "image/png"

	telagfx "github.com/kuronosu/tela/internal/gfx"
	telaraster "github.com/kuronosu/tela/internal/gfx/raster"
	telaplatform "github.com/kuronosu/tela/internal/platform"
{{- else}}
	telajson "encoding/json"
	telapng "image/png"
	telasort "sort"
	telastrings "strings"

	telatela "github.com/kuronosu/tela"
	telatest "github.com/kuronosu/tela/telatest"
{{- end}}
)
{{if not .Scene}}
var (
	_ = telasort.Strings
	_ = telastrings.Join
)
{{end}}
{{template "contract" .}}
func TestTelaRun(t *telatesting.T) {
	getenv := func(name string) string { return telaos.Getenv(name) }
	// What the run prints on standard error (dev warnings, class errors)
	// goes to a file that tela prints, since a passing go test hides it.
	if f, err := telaos.Create(getenv({{printf "%q" .Env.Log}})); err == nil {
		stderr := telaos.Stderr
		telaos.Stderr = f
		defer func() { telaos.Stderr = stderr; f.Close() }()
	}
	num := func(name string) float64 {
		v, err := telastrconv.ParseFloat(getenv(name), 64)
		if err != nil {
			t.Fatalf("%s is not a number: %v", name, err)
		}
		return v
	}
	// fail reports a problem to the user: tela prints the message instead
	// of go test's output.
	fail := func(format string, args ...any) {
		telaos.WriteFile(getenv({{printf "%q" .Env.Err}}), []byte(telafmt.Sprintf(format, args...)), 0o644)
		t.Fatalf(format, args...)
	}
	_ = fail
	write := func(env string, data []byte) {
		if err := telaos.WriteFile(getenv(env), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	w, h, scale := num({{printf "%q" .Env.W}}), num({{printf "%q" .Env.H}}), num({{printf "%q" .Env.Scale}})
	dark := getenv({{printf "%q" .Env.Dark}}) == "true"
	_ = dark

{{if .Scene}}
	size := telaplatform.Size{W: float32(w), H: float32(h)}
	phys := telaplatform.PhysicalSize(size, scale)
	if phys.X <= 0 || phys.Y <= 0 {
		t.Fatalf("empty image: %vx%v at scale %v", w, h, scale)
	}
	img := telaimage.NewRGBA(telaimage.Rect(0, 0, phys.X, phys.Y))
	telaraster.Clear(img, telaimage.Rectangle{}, telagfx.White)
	telaraster.Render(img, telaScene(size.W, size.H), telaraster.Options{Scale: float32(scale)})
	var buf telaBuffer
	if err := telapng.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	write({{printf "%q" .Env.Out}}, buf.b)
{{else}}
	root, preview, names, err := telaBuild(getenv({{printf "%q" .Env.Preview}}))
	if err != nil {
		fail("%v", err)
	}
	info, err := telajson.Marshal(map[string]any{"preview": preview, "names": names})
	if err != nil {
		t.Fatal(err)
	}
	write({{printf "%q" .Env.Info}}, info)
	opts := []telatest.Option{telatest.Size(float32(w), float32(h)), telatest.Scale(scale)}
	if dark {
		opts = append(opts, telatest.Dark())
	}
{{- if .HasApp}}
	// The App's settings; an explicit --size beats its Width and Height.
	if telaHasApp {
		opts = append(opts, telatest.FromApp(telaApp))
		if getenv({{printf "%q" .Env.SizeSet}}) == "true" {
			opts = append(opts, telatest.Size(float32(w), float32(h)))
		}
		if dark && telaApp.ColorScheme == telatela.Light {
			telafmt.Fprintln(telaos.Stderr, "tela: warning: --dark has no effect: App.ColorScheme is tela.Light, which overrides the system's preference")
		}
	}
{{- end}}

	switch getenv({{printf "%q" .Env.Mode}}) {
	case {{printf "%q" "snapshot"}}:
		img, err := telatest.Render(root, opts...)
		if err != nil {
			t.Fatal(err)
		}
		var buf telaBuffer
		if err := telapng.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, buf.b)
	case {{printf "%q" "inspect"}}:
		js, err := telatest.LayoutJSON(root, opts...)
		if err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, js)
	case {{printf "%q" "aria"}}:
		yaml, err := telatest.AriaSnapshot(root, opts...)
		if err != nil {
			t.Fatal(err)
		}
		if yaml != "" {
			yaml += "\n"
		}
		write({{printf "%q" .Env.Out}}, []byte(yaml))
	default:
		t.Fatalf("unknown mode %q", getenv({{printf "%q" .Env.Mode}}))
	}
{{end}}
}

// telaBuffer collects the output of an encoder.
type telaBuffer struct{ b []byte }

func (w *telaBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
` + contractTemplate))

// contractTemplate defines "contract", the declarations shared by the
// generated files of tela snapshot/inspect and tela mcp: telaApp (when the
// package has an App) and telaBuild, which returns the screen to mount for
// a preview name, the name and all the names (Previews), or an error for
// the user. Its data needs Kind and HasApp.
const contractTemplate = `{{define "contract"}}{{if .HasApp -}}
// The App of the package: its size, root class, color scheme, theme and
// class aliases apply to the run, as they do to its window. A variable App
// of another type (the package's server, say) is left alone: the type
// assertion compiles for any type.
var telaApp, telaHasApp = telaAppOf(App)

func telaAppOf(v any) (telatela.App, bool) {
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

{{end -}}
{{if eq .Kind "Preview" -}}
// The contract: func Preview() tela.Node or tela.Component. A
// func() tela.Node is mounted as tela.C(Preview), a component of its own,
// so that it may use hooks (ADR-175); a Preview that returns a Component
// is called and what it returns is mounted, as is.
func telaBuild(want string) (telatela.Component, string, []string, error) {
	if p, ok := any(Preview).(func() telatela.Node); ok {
		return telatela.C(p), "", nil, nil
	}
	var root telatela.Component = Preview()
	if root == nil {
		return nil, "", nil, telafmt.Errorf("Preview() returned nil; return the screen, such as tela.C(Screen) or a struct component")
	}
	return root, "", nil, nil
}
{{else if eq .Kind "Previews" -}}
// The contract: var Previews = tela.Previews{...}.
var telaPreviews telatela.Previews = Previews

func telaBuild(want string) (telatela.Component, string, []string, error) {
	names := make([]string, 0, len(telaPreviews))
	for name := range telaPreviews {
		names = append(names, name)
	}
	telasort.Strings(names)
	if len(names) == 0 {
		return nil, "", nil, telafmt.Errorf("Previews is empty; add at least one screen: tela.Previews{\"Name\": {Root: tela.C(Screen)}}")
	}
	name := want
	if name == "" {
		name = names[0]
	}
	p, ok := telaPreviews[name]
	if !ok {
		return nil, "", names, telafmt.Errorf("unknown preview %q; the package defines: %s", name, telastrings.Join(names, ", "))
	}
	if p.Root == nil {
		return nil, "", names, telafmt.Errorf("preview %q has no Root; give it one: {Root: tela.C(Screen)} or a struct component such as {Root: Screen{}}", name)
	}
	return p, name, names, nil
}
{{else if eq .Kind "App" -}}
// The contract: var App = tela.App{Root: ...}; its Root is the screen.
func telaBuild(want string) (telatela.Component, string, []string, error) {
	if !telaHasApp {
		return nil, "", nil, telafmt.Errorf("App is a %T, not a tela.App; define func Preview() tela.Node, or var App = tela.App{Root: tela.C(Screen)}", App)
	}
	if telaApp.Root == nil {
		return nil, "", nil, telafmt.Errorf("App has no Root; set it to the root component, such as tela.App{Root: tela.C(Screen)} or a struct component, tela.App{Root: Screen{}}, or define func Preview() tela.Node")
	}
	return telaApp.Root, "", nil, nil
}
{{else -}}
// The phase-1 contract: func Scene(w, h float32) *gfx.DisplayList.
var telaScene func(w, h float32) *telagfx.DisplayList = Scene
{{end}}
{{end}}`
