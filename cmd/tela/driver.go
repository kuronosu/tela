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
	"strconv"
	"strings"
	"text/template"
)

// How the commands run a package. internal/ packages cannot be imported from
// outside the module, and a package main cannot be imported at all, so
// instead of building a separate main, `go test -overlay` adds a _test.go
// file to the target package. The file is generated for the contract the
// package implements (Previews, Preview or Scene), runs inside the package
// with access to its declarations, and writes its result to a file whose
// path arrives in an environment variable. The overlay means nothing is
// written into the package's directory.

// mode is what the generated test does.
type mode string

const (
	modeSnapshot mode = "snapshot" // render to a PNG
	modeInspect  mode = "inspect"  // print the layout tree as JSON
)

// contract is how a package exposes its screens.
type contract int

const (
	contractNone     contract = iota
	contractPreview           // func Preview() tela.Node
	contractPreviews          // var Previews = tela.Previews{...}
	contractScene             // func Scene(w, h float32) *gfx.DisplayList (phase 1)
)

func (c contract) String() string {
	switch c {
	case contractPreview:
		return "Preview"
	case contractPreviews:
		return "Previews"
	case contractScene:
		return "Scene"
	}
	return "none"
}

// contractHelp explains the contracts, for errors.
const contractHelp = `it must define one of

	func Preview() tela.Node
	var Previews = tela.Previews{"Name": func() tela.Node { ... }}

(the phase-1 contract, func Scene(w, h float32) *gfx.DisplayList, also works for "tela snapshot")`

// Names of the environment variables that carry the parameters to the
// generated test (rather than test flags, which could clash with the
// package's own).
const (
	envMode    = "TELA_RUN_MODE"
	envW       = "TELA_RUN_W"
	envH       = "TELA_RUN_H"
	envScale   = "TELA_RUN_SCALE"
	envDark    = "TELA_RUN_DARK"
	envPreview = "TELA_RUN_PREVIEW"
	envOut     = "TELA_RUN_OUT"  // the result: a PNG or JSON
	envErr     = "TELA_RUN_ERR"  // a message for the user, when the run fails
	envInfo    = "TELA_RUN_INFO" // JSON runInfo
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

// detectContract reports which contract the package's source implements. It
// parses the declarations only, so it works before anything compiles. When
// several are defined, Previews wins over Preview, which wins over Scene.
func detectContract(dir string, files []string) (contract, error) {
	var hasPreview, hasPreviews, hasScene bool
	fset := token.NewFileSet()
	for _, name := range files {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return contractNone, fmt.Errorf("cannot read %s: %v", name, err)
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
					for _, n := range s.(*ast.ValueSpec).Names {
						if n.Name == "Previews" {
							hasPreviews = true
						}
					}
				}
			}
		}
	}
	switch {
	case hasPreviews:
		return contractPreviews, nil
	case hasPreview:
		return contractPreview, nil
	case hasScene:
		return contractScene, nil
	}
	return contractNone, nil
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
	Output []byte // the PNG or the JSON
	Info   runInfo
}

// runInPackage renders opts.Pkg with the generated test and returns its
// output file. ext is the extension of the temporary output ("png", "json").
// Warnings go to stderr.
func runInPackage(m mode, opts renderOptions, ext string, stderr io.Writer) (*runResult, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return nil, errors.New("the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
	}
	pkg, err := listPackage(goBin, opts.Pkg)
	if err != nil {
		return nil, err
	}
	c, err := detectContract(pkg.Dir, pkg.GoFiles)
	if err != nil {
		return nil, err
	}
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
	if err := os.WriteFile(src, generateSource(pkg.Name, c), 0o644); err != nil {
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
	cmd := exec.Command(goBin, "test", "-count=1", "-run", "^TestTelaRun$", "-overlay", overlay, opts.Pkg)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		envMode+"="+string(m),
		envW+"="+strconv.FormatFloat(opts.W, 'g', -1, 64),
		envH+"="+strconv.FormatFloat(opts.H, 'g', -1, 64),
		envScale+"="+strconv.FormatFloat(opts.Scale, 'g', -1, 64),
		envDark+"="+strconv.FormatBool(opts.Dark),
		envPreview+"="+opts.Preview,
		envOut+"="+outFile,
		envErr+"="+errFile,
		envInfo+"="+infoFile,
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
	case c == contractScene && m == modeInspect:
		return fmt.Errorf("package %s (%s) only defines Scene, a display list without a layout tree: tela inspect needs func Preview() tela.Node or a Previews variable", pkg.Name, opts.Pkg)
	case opts.Preview != "" && c == contractPreview:
		return fmt.Errorf("--preview %q: package %s defines Preview(), a single screen; to choose among several, define var Previews = tela.Previews{...}", opts.Preview, pkg.Name)
	case opts.Preview != "" && c == contractScene:
		return fmt.Errorf("--preview %q: package %s defines Scene, a single scene; to choose among several screens, define var Previews = tela.Previews{...}", opts.Preview, pkg.Name)
	}
	return nil
}

// explainFailure turns the output of a failed `go test` into an error. A
// missing or mistyped contract function, the usual mistake, gets a friendly
// message; anything else is passed through as go printed it.
func explainFailure(pkg, name string, c contract, output string) error {
	output = strings.TrimRight(output, "\n")
	if strings.Contains(output, generatedFile) {
		var want string
		switch c {
		case contractPreview:
			want = "func Preview() tela.Node"
		case contractPreviews:
			want = "var Previews = tela.Previews{\"Name\": func() tela.Node { ... }}   (a map[string]func() tela.Node)"
		case contractScene:
			want = "func Scene(w, h float32) *gfx.DisplayList"
		}
		if want != "" && (strings.Contains(output, "cannot use "+c.String()) || strings.Contains(output, "undefined: "+c.String())) {
			return fmt.Errorf("package %s (%s) cannot be rendered: its %s has the wrong type; it must be\n\n\t%s\n\ngo reported:\n%s", name, pkg, c, want, output)
		}
	}
	return fmt.Errorf("go test failed:\n%s", output)
}

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
// called pkgName that implements contract c. All imports have unusual
// aliases because import names live in the file scope of a package whose
// declarations they could collide with.
func generateSource(pkgName string, c contract) []byte {
	var buf bytes.Buffer
	err := generatedTemplate.Execute(&buf, struct {
		Package string
		Kind    string
		Scene   bool
		Env     map[string]string
	}{
		Package: pkgName, Kind: c.String(), Scene: c == contractScene,
		Env: map[string]string{
			"Mode": envMode, "W": envW, "H": envH, "Scale": envScale, "Dark": envDark,
			"Preview": envPreview, "Out": envOut, "Err": envErr, "Info": envInfo,
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
	telajson "encoding/json"
	telafmt "fmt"
	telaimage "image"
	telapng "image/png"
	telaos "os"
	telasort "sort"
	telastrconv "strconv"
	telastrings "strings"
	telatesting "testing"

{{- if not .Scene}}
	telacore "github.com/kuronosu/tela/internal/core"
{{- end}}
	telagfx "github.com/kuronosu/tela/internal/gfx"
	telaraster "github.com/kuronosu/tela/internal/gfx/raster"
	telaplatform "github.com/kuronosu/tela/internal/platform"
)

var (
	_ = telajson.Marshal
	_ = telafmt.Sprint
	_ = telaimage.Rect
	_ = telaraster.Clear
	_ = telasort.Strings
	_ = telastrings.Join
	_ = telagfx.White
)

{{if eq .Kind "Preview" -}}
// The contract: func Preview() tela.Node.
var telaPreview func() telacore.Node = Preview

// telaBuild returns the root to render and the info about the previews.
func telaBuild(want string) (telacore.Node, string, []string, error) {
	return telacore.Component(telaPreview), "", nil, nil
}
{{else if eq .Kind "Previews" -}}
// The contract: var Previews = tela.Previews{...}, a map[string]func() tela.Node.
var telaPreviews map[string]func() telacore.Node = Previews

func telaBuild(want string) (telacore.Node, string, []string, error) {
	names := make([]string, 0, len(telaPreviews))
	for name := range telaPreviews {
		names = append(names, name)
	}
	telasort.Strings(names)
	if len(names) == 0 {
		return telacore.Node{}, "", nil, telafmt.Errorf("Previews is empty; add at least one screen: tela.Previews{\"Name\": func() tela.Node { ... }}")
	}
	name := want
	if name == "" {
		name = names[0]
	}
	fn, ok := telaPreviews[name]
	if !ok {
		return telacore.Node{}, "", names, telafmt.Errorf("unknown preview %q; the package defines: %s", name, telastrings.Join(names, ", "))
	}
	if fn == nil {
		return telacore.Node{}, "", names, telafmt.Errorf("preview %q is nil; give it a func() tela.Node", name)
	}
	return telacore.Component(fn), name, names, nil
}
{{else -}}
// The phase-1 contract: func Scene(w, h float32) *gfx.DisplayList.
var telaScene func(w, h float32) *telagfx.DisplayList = Scene
{{end}}

func TestTelaRun(t *telatesting.T) {
	getenv := func(name string) string { return telaos.Getenv(name) }
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
	size := telaplatform.Size{W: float32(w), H: float32(h)}
	phys := telaplatform.PhysicalSize(size, scale)
	if phys.X <= 0 || phys.Y <= 0 {
		t.Fatalf("empty image: %vx%v at scale %v", w, h, scale)
	}
	dark := getenv({{printf "%q" .Env.Dark}}) == "true"
	_ = dark

{{if .Scene}}
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
	cfg := telacore.Config{Dark: dark}

	switch getenv({{printf "%q" .Env.Mode}}) {
	case {{printf "%q" "snapshot"}}:
		_, img := telacore.Render(root, telacore.RenderOptions{Config: cfg, Width: size.W, Height: size.H, Scale: float32(scale)})
		var buf telaBuffer
		if err := telapng.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, buf.b)
	case {{printf "%q" "inspect"}}:
		tree := telacore.NewTree(cfg)
		tree.SetViewport(telagfx.Size{W: size.W, H: size.H})
		tree.Mount(root)
		tree.Frame()
		var buf telaBuffer
		enc := telajson.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(tree.Inspect()); err != nil {
			t.Fatal(err)
		}
		write({{printf "%q" .Env.Out}}, buf.b)
	default:
		t.Fatalf("unknown mode %q", getenv({{printf "%q" .Env.Mode}}))
	}
{{end}}
}

// telaBuffer collects the output of an encoder.
type telaBuffer struct{ b []byte }

func (w *telaBuffer) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }
`))
