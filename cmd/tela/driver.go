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
// package implements (Previews, Preview or App), runs inside the package
// with access to its declarations, and writes its result to a file whose
// path arrives in an environment variable. The overlay means nothing is
// written into the package's directory.
//
// Screens are mounted through the public telatest package (Mount, then
// Screenshot, AriaSnapshot and LayoutJSON), so the commands work on any
// module that requires github.com/kuronosu/tela (ADR-059, which lifts the
// limitation of ADR-047). The generated file imports only public packages.
// Mount settles the app as a test does: passive effects run, the state they
// set and the toasts they show are built, and finite animations finish, so
// the output is the screen at rest, as a window shows it after a moment.
// A module whose tela predates that (genFeatures.Settle: telatest without
// NoSettle or App.LayoutJSON) gets the form of before, which renders one
// frame with telatest.Render, LayoutJSON and AriaSnapshot.

// mode is what the generated test does.
type mode string

const (
	modeSnapshot mode = "snapshot" // render to a PNG
	modeInspect  mode = "inspect"  // print the layout tree as JSON
	modeAria     mode = "aria"     // print the aria snapshot as YAML
	modeFilm     mode = "film"     // capture the frames of the mount's motion (JSON)
	modeCheck    mode = "check"    // audit the screens in a matrix of variants (JSON, check.go)
)

// contract is how a package exposes its screens.
type contract int

const (
	contractNone     contract = iota
	contractPreview           // func Preview() tela.Node
	contractPreviews          // var Previews = tela.Previews{...}
	contractApp               // var App = tela.App{...}, whose Root is the screen
)

func (c contract) String() string {
	switch c {
	case contractPreview:
		return "Preview"
	case contractPreviews:
		return "Previews"
	case contractApp:
		return "App"
	}
	return "none"
}

// singleScreen explains, for pkg, that a package without Previews has one
// screen and how to get several. Each command keeps its own rule for when to
// say it; this is only the wording. It is for the contracts that have a
// single screen (Preview, App).
func (c contract) singleScreen(pkg string) string {
	what := "Preview(), a single screen"
	switch c {
	case contractApp:
		what = "App, whose Root is a single screen"
	}
	return fmt.Sprintf("package %s defines %s; to choose among several, define var Previews = tela.Previews{\"Name\": {Root: tela.C(Screen)}}", pkg, what)
}

// contractHelp explains the contracts, for errors.
const contractHelp = `it must define one of

	func Preview() tela.Node
	var Previews = tela.Previews{"Name": {Root: tela.C(Screen)}}
	var App = tela.App{Root: tela.C(Screen), ...}

(a package that defines App renders in the size, class, color scheme, theme and
aliases of that App; alone, its Root is the screen)`

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
	envFilm    = "TELA_RUN_FILM"  // span of the film, in nanoseconds (checkFilm validated these exact values)
	envEvery   = "TELA_RUN_EVERY" // interval of the film, in nanoseconds
	envOut     = "TELA_RUN_OUT"   // the result: a PNG, JSON or YAML
	envErr     = "TELA_RUN_ERR"   // a message for the user, when the run fails
	envInfo    = "TELA_RUN_INFO"  // JSON runInfo
	envLog     = "TELA_RUN_LOG"   // what the run printed on standard error (dev warnings and class errors)
	envSpec    = "TELA_RUN_SPEC"  // JSON checkSpec, for modeCheck
)

// runFeatures is detectFeatures, replaced by a test that runs the form for
// an older tela.
var runFeatures = detectFeatures

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
func listPackage(goBin, pkg string) (goPackage, error) { return listPackageIn(goBin, "", pkg) }

// listPackageIn is listPackage run in dir (the working directory when it is
// empty): the directory of the module that has the package.
func listPackageIn(goBin, dir, pkg string) (goPackage, error) {
	// One line per package: dir|name|file,file...
	cmd := exec.Command(goBin, "list", "-f", `{{.Dir}}|{{.Name}}|{{join .GoFiles ","}}{{if and .GoFiles .TestGoFiles}},{{end}}{{join .TestGoFiles ","}}`, pkg)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	cmd.Dir = dir
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
// wins over Preview, which wins over App (a package
// that defines App next to another contract still renders in that App).
func scanPackage(dir string, files []string) (detected, error) {
	var hasPreview, hasPreviews, hasApp bool
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
				if d.Name.Name == "Preview" {
					hasPreview = true
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
	case hasApp:
		d.contract = contractApp
	}
	return d, nil
}

// runInfo is what the generated test reports about the run.
type runInfo struct {
	// Preview is the name of the rendered preview ("" for a package that
	// defines Preview or App) and Names all the names, sorted.
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

	tmp, err := os.MkdirTemp("", "tela-run-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, generatedFile)
	// The elements' source lines are part of what inspect prints.
	// The module's tela may predate the settled mount; the source lines
	// are only for inspect.
	feats := runFeatures(goBin, pkg.Dir)
	if m != modeCheck {
		feats.I18N = false
	}
	if m == modeSnapshot || m == modeFilm {
		feats.DevTools = false // a picture, or a film of the app as it is: not instrumented
	}
	gen := generateSource(pkg.Name, c, det.hasApp, feats)
	if m == modeCheck {
		if !feats.Audit {
			return nil, errors.New("this package's tela module predates telatest's accessibility audit (App.Audit); update it with go get github.com/kuronosu/tela@latest")
		}
		gen = generateCheckSource(pkg.Name, c, det.hasApp, feats)
	}
	if err := os.WriteFile(src, gen, 0o644); err != nil {
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
		envFilm+"="+strconv.FormatInt(int64(opts.Film), 10),
		envEvery+"="+strconv.FormatInt(int64(opts.Every), 10),
		envDark+"="+strconv.FormatBool(opts.Dark),
		envSizeSet+"="+strconv.FormatBool(opts.SizeSet),
		envPreview+"="+opts.Preview,
		envOut+"="+outFile,
		envErr+"="+errFile,
		envInfo+"="+infoFile,
		envLog+"="+logFile,
		envSpec+"="+opts.Spec,
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
	case opts.Preview != "" && (c == contractPreview || c == contractApp):
		return fmt.Errorf("--preview %q: %s", opts.Preview, c.singleScreen(pkg.Name))
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
				want = "func Preview() tela.Node"
			case contractPreviews:
				want = "var Previews = tela.Previews{\"Name\": {Root: tela.C(Screen)}}   (a map[string]tela.Preview)"
			case contractApp:
				want = "var App = tela.App{...}"
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
func generateSource(pkgName string, c contract, hasApp bool, feats ...genFeatures) []byte {
	var f genFeatures
	if len(feats) > 0 {
		f = feats[0]
	}
	var buf bytes.Buffer
	err := generatedTemplate.Execute(&buf, struct {
		Package  string
		Kind     string
		HasApp   bool
		Settle   bool
		DevTools bool
		Film     bool
		Env      map[string]string
	}{
		Package: pkgName, Kind: c.String(), HasApp: hasApp, Settle: f.Settle || f.Film || f.DevTools, DevTools: f.DevTools, Film: f.Film,
		Env: map[string]string{
			"Mode": envMode, "W": envW, "H": envH, "Scale": envScale, "Dark": envDark, "SizeSet": envSizeSet,
			"Preview": envPreview, "Out": envOut, "Err": envErr, "Info": envInfo, "Log": envLog,
			"Film": envFilm, "Every": envEvery,
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
	telareflect "reflect"
	teladebug "runtime/debug"
	telastrconv "strconv"
	telatesting "testing"

	telajson "encoding/json"
	telapng "image/png"
	telasort "sort"
	telastrings "strings"
	telatime "time"

	telatela "github.com/kuronosu/tela"
	telatest "github.com/kuronosu/tela/telatest"
)

var (
	_ = telasort.Strings
	_ = telastrings.Join
	_ = telatime.Second
	_ = teladebug.Stack
)

{{template "contract" .}}
// telaTB is the testing.TB the screen is mounted with: the real test's,
// except that what telatest logs goes to standard error as Render printed
// it (the dev messages as they are, the rest after "tela: ").
// quiet drops the dev messages, as a screen with dev mode off prints none.
type telaTB struct {
	telatesting.TB
	quiet bool
}

func (b telaTB) Logf(format string, args ...any) {
	msg := telafmt.Sprintf(format, args...)
	switch {
	case telastrings.HasPrefix(msg, "dev message: "), telastrings.HasPrefix(msg, "dev warning: "):
		if b.quiet {
			return
		}
		msg = msg[telastrings.Index(msg, ": ")+2:]
	case telastrings.HasPrefix(msg, "telatest: the app was still requesting frames"):
		msg = "tela: warning: the screen never goes quiet (an endless animation, or state that is set all the time); it is shown at its frame after a few seconds"
	default:
		msg = "tela: " + msg
	}
	telafmt.Fprintln(telaos.Stderr, msg)
}

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
	// The color scheme that overrides --dark, and whose it is.
	scheme, schemeOf := telatela.System, ""
	_, _ = scheme, schemeOf
{{- if .HasApp}}
	// The App's settings; an explicit --size beats its Width and Height.
	if telaHasApp {
		opts = append(opts, telatest.FromApp(telaApp))
		if getenv({{printf "%q" .Env.SizeSet}}) == "true" {
			opts = append(opts, telatest.Size(float32(w), float32(h)))
		}
		scheme, schemeOf = telaApp.ColorScheme, "App.ColorScheme"
	}
{{- end}}
	// A preview's Theme and ColorScheme beat the App's: telatest applies
	// them after the options (ADR-250).
	// The field is read by reflection: a tela before the one that added it
	// has no Preview.ColorScheme to name.
	if p, ok := root.(telatela.Preview); ok {
		if f := telareflect.ValueOf(p).FieldByName("ColorScheme"); f.IsValid() {
			if cs, ok := f.Interface().(telatela.ColorScheme); ok && cs != telatela.System {
				scheme, schemeOf = cs, telafmt.Sprintf("the ColorScheme of preview %q", preview)
			}
		}
	}
	if dark && scheme == telatela.Light {
		telafmt.Fprintf(telaos.Stderr, "tela: warning: --dark has no effect: %s is tela.Light, which overrides the system's preference\n", schemeOf)
	}

{{- if .Settle}}
	// The screen is mounted as a test mounts it and settled: the effects
	// that run after the first frame, the state they set and the toasts
	// they show are part of what is rendered. A panic in the tree is the
	// error of the run.
	defer func() {
		if r := recover(); r != nil {
			fail("the UI panicked: %v\n%s", r, teladebug.Stack())
		}
	}()
	// Mount keeps the dev messages for the test log whether dev mode is on
	// or not: telaTB prints them on standard error, except when it is off
	// (App.Dev, TELA_DEV).
	dev := true
{{- if .HasApp}}
	if telaHasApp && telaApp.Dev != nil {
		dev = *telaApp.Dev
	}
{{- end}}
	switch getenv("TELA_DEV") {
	case "0":
		dev = false
	case "1":
		dev = true
	}
	opts = append(opts, telatest.AllowDev(""))
	if getenv({{printf "%q" .Env.Mode}}) == {{printf "%q" "film"}} {
		// The film starts with the mount: its first frame is the screen as
		// the mount left it, with the motion it started still to run.
		opts = append(opts, telatest.NoSettle())
	}
{{- if .DevTools}}
	// The elements' source lines are part of what inspect prints.
	opts = append(opts, telatest.Instrument())
{{- end}}
	app := telatest.Mount(telaTB{t, !dev}, root, opts...)
{{- end}}

	switch getenv({{printf "%q" .Env.Mode}}) {
	case {{printf "%q" "snapshot"}}:
{{- if .Settle}}
		var buf telaBuffer
		if err := telapng.Encode(&buf, app.Screenshot()); err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, buf.b)
{{- else}}
		// The tela of this module predates the settled mount: one frame.
		img, err := telatest.Render(root, opts...)
		if err != nil {
			t.Fatal(err)
		}
		var buf telaBuffer
		if err := telapng.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, buf.b)
{{- end}}
	case {{printf "%q" "inspect"}}:
{{- if .Settle}}
		write({{printf "%q" .Env.Out}}, app.LayoutJSON())
{{- else}}
		js, err := telatest.LayoutJSON(root, opts...)
		if err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, js)
{{- end}}
	case {{printf "%q" "aria"}}:
{{- if .Settle}}
{{- if .DevTools}}
		yaml, _ := app.AriaSnapshotSources()
{{- else}}
		yaml := app.AriaSnapshot()
{{- end}}
{{- else}}
		yaml, err := telatest.AriaSnapshot(root, opts...)
		if err != nil {
			t.Fatal(err)
		}
{{- end}}
		if yaml != "" {
			yaml += "\n"
		}
		write({{printf "%q" .Env.Out}}, []byte(yaml))
	case {{printf "%q" "film"}}:
{{- if .Film}}
		type telaFrame struct {
			At  int64  ` + "`json:\"at\"`" + `
			PNG []byte ` + "`json:\"png\"`" + `
		}
		var frames []telaFrame
		span := telatime.Duration(num({{printf "%q" .Env.Film}})) // nanoseconds
		every := telatime.Duration(num({{printf "%q" .Env.Every}}))
		for _, f := range app.Film(span, every, nil) {
			var buf telaBuffer
			if err := telapng.Encode(&buf, f.Image); err != nil {
				t.Fatal(err)
			}
			frames = append(frames, telaFrame{f.At.Milliseconds(), buf.b})
		}
		js, err := telajson.Marshal(frames)
		if err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, js)
{{- else}}
		fail("this package's tela module predates telatest's App.Film; update it with go get github.com/kuronosu/tela@latest")
{{- end}}
	default:
		t.Fatalf("unknown mode %q", getenv({{printf "%q" .Env.Mode}}))
	}
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
// The contract: func Preview() tela.Node, mounted as tela.C(Preview): a
// component of its own, so that it may use hooks (ADR-175), and whatever
// it returns (an element, tela.C(Screen), a struct component) is its
// screen (ADR-205).
func telaBuild(want string) (telatela.Component, string, []string, error) {
	return telatela.C(Preview), "", nil, nil
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
{{end}}
{{end}}`
