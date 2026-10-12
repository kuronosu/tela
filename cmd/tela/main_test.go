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
)

func TestParseSnapshotArgsDefaults(t *testing.T) {
	got, err := parseSnapshotArgs([]string{"./examples/shapes"})
	if err != nil {
		t.Fatal(err)
	}
	want := snapshotOptions{Pkg: "./examples/shapes", W: 960, H: 640, Scale: 1, Out: "snapshot.png"}
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
		{[]string{"--dark", "p"}, "flag provided but not defined"},
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

func TestSnapshotSource(t *testing.T) {
	src := snapshotSource("main")
	f, err := parser.ParseFile(token.NewFileSet(), snapshotFile, src, parser.AllErrors)
	if err != nil {
		t.Fatalf("generated source does not parse: %v\n%s", err, src)
	}
	if f.Name.Name != "main" {
		t.Errorf("package = %q, want main", f.Name.Name)
	}
	for _, want := range []string{"func TestTelaSnapshot(", `"TELA_SNAPSHOT_W"`, `"TELA_SNAPSHOT_H"`, `"TELA_SNAPSHOT_SCALE"`, `"TELA_SNAPSHOT_OUT"`, "= Scene", "PhysicalSize"} {
		if !strings.Contains(string(src), want) {
			t.Errorf("generated source lacks %q", want)
		}
	}
	if other := snapshotSource("widgets"); !strings.Contains(string(other), "package widgets\n") {
		t.Errorf("package name not substituted:\n%s", other)
	}
}

func TestExplainFailure(t *testing.T) {
	missing := "# example.com/x [example.com/x.test]\n./" + snapshotFile + ":19:92: undefined: Scene\nFAIL example.com/x [build failed]\n"
	err := explainFailure("./x", "x", missing)
	if !strings.Contains(err.Error(), "func Scene(w, h float32) *gfx.DisplayList") {
		t.Errorf("friendly message missing for undefined Scene: %v", err)
	}
	wrong := "./" + snapshotFile + ":19:92: cannot use Scene (value of type func() int) as func(w float32, h float32) *gfx.DisplayList value"
	if err := explainFailure("./x", "x", wrong); !strings.Contains(err.Error(), "must define") {
		t.Errorf("friendly message missing for mistyped Scene: %v", err)
	}
	other := "x.go:3:1: syntax error"
	if err := explainFailure("./x", "x", other); strings.Contains(err.Error(), "must define") || !strings.Contains(err.Error(), "syntax error") {
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

func TestSnapshotShapes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on examples/shapes")
	}
	for _, tc := range []struct {
		scale  string
		golden string
	}{
		{"1", "shapes_x1.png"},
		{"2", "shapes_x2.png"},
	} {
		t.Run("scale"+tc.scale, func(t *testing.T) {
			outPath := filepath.Join(t.TempDir(), "sub", "out.png")
			var stdout, stderr bytes.Buffer
			code := run([]string{"snapshot", "--scale", tc.scale, "-o", outPath, "../../examples/shapes"}, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("exit code %d\nstdout: %s\nstderr: %s", code, stdout.String(), stderr.String())
			}
			got := readPNG(t, outPath)
			want := readPNG(t, filepath.Join("..", "..", "examples", "shapes", "testdata", tc.golden))
			if got.Rect != want.Rect {
				t.Fatalf("size = %v, want %v", got.Rect, want.Rect)
			}
			// One level of tolerance, as in examples/shapes TestGolden: the
			// scene's own float code may be fused differently on other
			// architectures.
			for i := range got.Pix {
				if d := int(got.Pix[i]) - int(want.Pix[i]); d < -1 || d > 1 {
					t.Fatalf("snapshot differs from examples/shapes/testdata/%s at byte %d: %d vs %d", tc.golden, i, got.Pix[i], want.Pix[i])
				}
			}
		})
	}
}

func TestSnapshotSizeAndFractionalScale(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test on examples/shapes")
	}
	outPath := filepath.Join(t.TempDir(), "o.png")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"snapshot", "--size", "400x300", "--scale", "1.25", "-o", outPath, "../../examples/shapes"}, &stdout, &stderr); code != 0 {
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
	if !strings.Contains(stderr.String(), "func Scene(w, h float32) *gfx.DisplayList") {
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
