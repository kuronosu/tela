package main

import (
	"bytes"
	"errors"
	"flag"
	"go/parser"
	"go/token"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/testutil/golden"
)

func TestParseSnapshotArgsDefaults(t *testing.T) {
	got, err := parseSnapshotArgs([]string{"./examples/counter"})
	if err != nil {
		t.Fatal(err)
	}
	want := snapshotOptions{Pkg: "./examples/counter", W: 960, H: 640, Scale: 1, Out: "snapshot.png"}
	if got.Pkg != want.Pkg || got.W != want.W || got.H != want.H || got.Scale != want.Scale || got.Out != want.Out {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseSnapshotArgsFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--size", "800x600", "--scale", "1.25", "-o", "out/a.png", "pkg"},
		{"pkg", "--size=800x600", "--scale=1.25", "-o=out/a.png"},
		{"--size", "800X600", "pkg", "-scale", "1.25", "-o", "out/a.png"},
	} {
		got, err := parseSnapshotArgs(args)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if got.Pkg != "pkg" || got.W != 800 || got.H != 600 || got.Scale != 1.25 || got.Out != "out/a.png" {
			t.Errorf("%v: got %+v", args, got)
		}
	}
}

func TestParseSnapshotArgsErrors(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "missing <pkg>"},
		{[]string{"a", "b"}, "expected one <pkg>"},
		{[]string{"--size", "800", "p"}, "invalid --size"},
		{[]string{"--size", "0x600", "p"}, "invalid --size"},
		{[]string{"--size", "-5x600", "p"}, "invalid --size"},
		{[]string{"--size", "axb", "p"}, "invalid --size"},
		{[]string{"--scale", "0", "p"}, "invalid --scale"},
		{[]string{"--scale", "-1", "p"}, "invalid --scale"},
		{[]string{"--scale", "NaN", "p"}, "invalid --scale"},
		{[]string{"--scale", "Inf", "p"}, "invalid --scale"},
		{[]string{"-o", "", "p"}, "empty -o"},
		{[]string{"--frobnicate", "p"}, "flag provided but not defined"},
		{[]string{"--size", "30000x30000", "p"}, "too large"},
	} {
		_, err := parseSnapshotArgs(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: error = %v, want it to contain %q", tc.args, err, tc.want)
		}
	}
	if _, err := parseSnapshotArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: error = %v, want flag.ErrHelp", err)
	}
}

func TestRunDispatch(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run(nil, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "Usage") {
		t.Errorf("no args: code %d, stderr %q", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"help"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "snapshot") {
		t.Errorf("help: code %d, stdout %q", code, out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"help", "snapshot"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "--scale") {
		t.Errorf("help snapshot: code %d, stdout %q", code, out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"frobnicate"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), `unknown command "frobnicate"`) {
		t.Errorf("unknown command: code %d, stderr %q", code, errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"snapshot"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "missing <pkg>") {
		t.Errorf("snapshot without package: code %d, stderr %q", code, errOut.String())
	}
}

func TestParseSnapshotArgsRenderFlags(t *testing.T) {
	got, err := parseSnapshotArgs([]string{"--dark", "--preview", "Login", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Dark || got.Preview != "Login" || got.Pkg != "pkg" {
		t.Errorf("got %+v", got)
	}
	got, err = parseSnapshotArgs([]string{"pkg"})
	if err != nil || got.Dark || got.Preview != "" {
		t.Errorf("defaults: %+v, %v", got, err)
	}
}

func TestParseInspectArgs(t *testing.T) {
	got, err := parseInspectArgs([]string{"--layout", "--size", "400x300", "--dark", "--preview=Alert", "./p"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Pkg != "./p" || !got.Layout || got.W != 400 || got.H != 300 || !got.Dark || got.Preview != "Alert" {
		t.Errorf("got %+v", got)
	}
	got, err = parseInspectArgs([]string{"./p"})
	if err != nil || got.W != 960 || got.H != 640 || got.Layout {
		t.Errorf("defaults: %+v, %v", got, err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "missing <pkg>"},
		{[]string{"a", "b"}, "expected one <pkg>"},
		{[]string{"--aria", "p"}, "phase 4"},
		{[]string{"--layout", "--aria", "p"}, "phase 4"},
		{[]string{"--size", "x", "p"}, "invalid --size"},
		{[]string{"--scale", "2", "p"}, "flag provided but not defined"},
		{[]string{"-o", "x.png", "p"}, "flag provided but not defined"},
	} {
		if _, err := parseInspectArgs(tc.args); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: error = %v, want it to contain %q", tc.args, err, tc.want)
		}
	}
	if _, err := parseInspectArgs([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: error = %v, want flag.ErrHelp", err)
	}
}

func TestInspectDispatch(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"help", "inspect"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "--layout") {
		t.Errorf("help inspect: code %d, stdout %q", code, out.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"inspect", "--aria", "p"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "phase 4") || out.Len() != 0 {
		t.Errorf("inspect --aria: code %d, stdout %q, stderr %q", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	if code := run([]string{"inspect"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "missing <pkg>") {
		t.Errorf("inspect without package: code %d, stderr %q", code, errOut.String())
	}
}

func TestGenerateSource(t *testing.T) {
	for _, c := range []contract{contractPreview, contractPreviews, contractScene} {
		src := generateSource("main", c)
		f, err := parser.ParseFile(token.NewFileSet(), generatedFile, src, parser.AllErrors)
		if err != nil {
			t.Fatalf("%v: generated source does not parse: %v\n%s", c, err, src)
		}
		if f.Name.Name != "main" {
			t.Errorf("%v: package = %q, want main", c, f.Name.Name)
		}
		if c != contractScene && !strings.Contains(string(src), `"TELA_RUN_MODE"`) {
			t.Errorf("%v: generated source does not read the mode", c)
		}
		for _, want := range []string{"func TestTelaRun(", `"TELA_RUN_W"`, `"TELA_RUN_OUT"`, `"TELA_RUN_ERR"`, "= " + c.String(), "PhysicalSize"} {
			if !strings.Contains(string(src), want) {
				t.Errorf("%v: generated source lacks %q", c, want)
			}
		}
	}
	if other := generateSource("widgets", contractPreview); !strings.Contains(string(other), "package widgets\n") {
		t.Errorf("package name not substituted:\n%s", other)
	}
}

func TestDetectContract(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  contract
	}{
		{"none", map[string]string{"a.go": "package a\nfunc Other() {}\nvar X = 1\n"}, contractNone},
		{"preview", map[string]string{"a.go": "package a\nfunc Preview() int { return 1 }\n"}, contractPreview},
		{"previews", map[string]string{"a.go": "package a\nvar Previews = map[string]func(){}\n"}, contractPreviews},
		{"previews in a group", map[string]string{"a.go": "package a\nvar (\n\tx = 1\n\tPreviews = 2\n)\n"}, contractPreviews},
		{"scene", map[string]string{"a.go": "package a\nfunc Scene(w, h float32) {}\n"}, contractScene},
		{"previews wins", map[string]string{"a.go": "package a\nfunc Preview() {}\nfunc Scene() {}\n", "b.go": "package a\nvar Previews = 1\n"}, contractPreviews},
		{"preview wins over scene", map[string]string{"a.go": "package a\nfunc Preview() {}\nfunc Scene() {}\n"}, contractPreview},
		{"methods do not count", map[string]string{"a.go": "package a\ntype T int\nfunc (T) Preview() {}\n"}, contractNone},
	} {
		var files []string
		for name, src := range tc.files {
			write(name, src)
			files = append(files, name)
		}
		got, err := detectContract(dir, files)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %v, %v; want %v", tc.name, got, err, tc.want)
		}
		for name := range tc.files {
			os.Remove(filepath.Join(dir, name))
		}
	}
	write("bad.go", "package a\nfunc {")
	if _, err := detectContract(dir, []string{"bad.go"}); err == nil || !strings.Contains(err.Error(), "bad.go") {
		t.Errorf("syntax error: %v", err)
	}
}

func TestCheckContract(t *testing.T) {
	pkg := goPackage{Name: "x"}
	for _, tc := range []struct {
		m    mode
		c    contract
		prev string
		want string // "" means no error
	}{
		{modeSnapshot, contractNone, "", "func Preview() tela.Node"},
		{modeSnapshot, contractPreview, "", ""},
		{modeSnapshot, contractPreviews, "A", ""},
		{modeSnapshot, contractScene, "", ""},
		{modeInspect, contractScene, "", "only defines Scene"},
		{modeSnapshot, contractPreview, "A", "single screen"},
		{modeSnapshot, contractScene, "A", "single scene"},
	} {
		err := checkContract(tc.m, tc.c, renderOptions{Pkg: "./x", Preview: tc.prev}, pkg)
		switch {
		case tc.want == "" && err != nil, tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%v %v preview %q: error %v, want %q", tc.m, tc.c, tc.prev, err, tc.want)
		}
	}
}

func TestExplainFailure(t *testing.T) {
	wrong := "# example.com/x [example.com/x.test]\n./" + generatedFile + ":19:92: cannot use Preview (value of type func() int) as func() core.Node value in variable declaration"
	if err := explainFailure("./x", "x", contractPreview, wrong); !strings.Contains(err.Error(), "func Preview() tela.Node") || !strings.Contains(err.Error(), "wrong type") {
		t.Errorf("friendly message missing for mistyped Preview: %v", err)
	}
	scene := "./" + generatedFile + ":19:92: cannot use Scene (value of type func() int) as func(w float32, h float32) *gfx.DisplayList value"
	if err := explainFailure("./x", "x", contractScene, scene); !strings.Contains(err.Error(), "func Scene(w, h float32) *gfx.DisplayList") {
		t.Errorf("friendly message missing for mistyped Scene: %v", err)
	}
	previews := "./" + generatedFile + ":19:92: cannot use Previews (variable of type int) as map[string]func() core.Node value"
	if err := explainFailure("./x", "x", contractPreviews, previews); !strings.Contains(err.Error(), "tela.Previews") {
		t.Errorf("friendly message missing for mistyped Previews: %v", err)
	}
	other := "x.go:3:1: syntax error"
	if err := explainFailure("./x", "x", contractPreview, other); strings.Contains(err.Error(), "wrong type") || !strings.Contains(err.Error(), "syntax error") {
		t.Errorf("unrelated failure must pass through: %v", err)
	}
}

// readPNG decodes a PNG into non-premultiplied 8-bit RGBA.
func readPNG(t *testing.T, path string) *image.NRGBA {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out
}

// The integration tests run `go test` in a child process.

// TestSnapshotScene renders the phase-1 Scene package of testdata/scene and
// compares it with its goldens (one level of tolerance: the scene's own
// float code may be fused differently on other architectures).
func TestSnapshotScene(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on testdata/scene")
	}
	for _, tc := range []struct {
		scale  string
		golden string
	}{
		{"1", "scene_x1"},
		{"2", "scene_x2"},
	} {
		t.Run("scale"+tc.scale, func(t *testing.T) {
			outPath := filepath.Join(t.TempDir(), "sub", "out.png")
			var stdout, stderr bytes.Buffer
			code := run([]string{"snapshot", "--size", "160x120", "--scale", tc.scale, "-o", outPath, "./testdata/scene"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
			}
			golden.Image(t, tc.golden, readPNG(t, outPath), golden.MaxDelta(1))
		})
	}
}

func TestSnapshotSizeAndFractionalScale(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on testdata/scene")
	}
	outPath := filepath.Join(t.TempDir(), "o.png")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"snapshot", "--size", "400x300", "--scale", "1.25", "-o", outPath, "../../examples/counter"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit code %d\nstderr: %s", code, stderr.String())
	}
	if got := readPNG(t, outPath).Rect; got != image.Rect(0, 0, 500, 375) {
		t.Errorf("size = %v, want 500x375", got)
	}
}

func TestSnapshotPackageWithoutScene(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on a package")
	}
	outPath := filepath.Join(t.TempDir(), "o.png")
	var stdout, stderr bytes.Buffer
	code := run([]string{"snapshot", "-o", outPath, "../../internal/platform/auto"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit code %d, want 1\nstderr: %s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "func Preview() tela.Node") || !strings.Contains(stderr.String(), "Previews") {
		t.Errorf("stderr lacks the friendly message:\n%s", stderr.String())
	}
	if _, err := os.Stat(outPath); err == nil {
		t.Errorf("%s was written for a failed snapshot", outPath)
	}
}

func TestSnapshotUnknownPackage(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"snapshot", "-o", filepath.Join(t.TempDir(), "o.png"), "./does/not/exist"}, &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "cannot find package") {
		t.Errorf("exit code %d, stderr %q", code, stderr.String())
	}
}
