package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const snapshotUsage = `Usage:

	tela snapshot [--size WxH] [--scale N] [-o out.png] <pkg>

Renders the scene of a package headlessly and writes it as a PNG.

<pkg> is a package path or a relative directory, such as ./examples/shapes.
The package must define

	func Scene(w, h float32) *gfx.DisplayList

which builds the scene for a window of w x h logical pixels. (This is the
temporary contract of phase 1; the Preview() contract arrives with the public
API in phase 3.)

Flags:

	--size WxH   logical size of the window (default 960x640)
	--scale N    display scale; fractional scales such as 1.25 are allowed
	             (default 1). The PNG is (W*N)x(H*N) pixels, rounded like the
	             window backends do
	-o file      output file (default snapshot.png)
`

// Limits that keep a typo from allocating gigabytes.
const (
	maxLogicalSide  = 32768
	maxPhysicalSide = 32768
	maxPhysicalPix  = 1 << 28 // 256 Mpx, 1 GiB of RGBA
)

// snapshotOptions are the parsed arguments of `tela snapshot`.
type snapshotOptions struct {
	Pkg   string
	W, H  float64 // logical size
	Scale float64
	Out   string
}

// parseSnapshotArgs parses the arguments of `tela snapshot`. Flags and the
// package may come in any order.
func parseSnapshotArgs(args []string) (snapshotOptions, error) {
	opts := snapshotOptions{W: 960, H: 640, Scale: 1, Out: "snapshot.png"}
	var size string
	fs := flag.NewFlagSet("tela snapshot", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&size, "size", "", "")
	fs.Float64Var(&opts.Scale, "scale", opts.Scale, "")
	fs.StringVar(&opts.Out, "o", opts.Out, "")

	var pkgs []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return opts, err
			}
			return opts, fmt.Errorf("%v (see \"tela help snapshot\")", err)
		}
		args = fs.Args()
		if len(args) > 0 {
			pkgs = append(pkgs, args[0])
			args = args[1:]
		}
	}
	switch len(pkgs) {
	case 0:
		return opts, errors.New("missing <pkg>: say which package to snapshot, for example \"tela snapshot ./examples/shapes\"")
	case 1:
		opts.Pkg = pkgs[0]
	default:
		return opts, fmt.Errorf("expected one <pkg>, got %d (%s)", len(pkgs), strings.Join(pkgs, ", "))
	}
	if size != "" {
		w, h, err := parseSize(size)
		if err != nil {
			return opts, err
		}
		opts.W, opts.H = w, h
	}
	if !(opts.Scale > 0) || math.IsInf(opts.Scale, 0) || opts.Scale > 64 {
		return opts, fmt.Errorf("invalid --scale %v: use a number > 0 and <= 64, for example 1, 2 or 1.25", opts.Scale)
	}
	if opts.Out == "" {
		return opts, errors.New("empty -o: give an output file such as -o snapshot.png")
	}
	px, py := physicalSide(opts.W, opts.Scale), physicalSide(opts.H, opts.Scale)
	if px > maxPhysicalSide || py > maxPhysicalSide || px*py > maxPhysicalPix {
		return opts, fmt.Errorf("a %gx%g window at scale %g is %dx%d pixels, which is too large; lower --size or --scale", opts.W, opts.H, opts.Scale, px, py)
	}
	return opts, nil
}

// physicalSide mirrors platform.PhysicalSize for one axis, only to bound the
// image size before running anything (the generated test uses the real one).
func physicalSide(logical, scale float64) int { return int(math.Round(logical * scale)) }

// parseSize parses "WxH" with positive numbers.
func parseSize(s string) (w, h float64, err error) {
	bad := fmt.Errorf("invalid --size %q: use WxH with positive numbers, for example 960x640", s)
	ws, hs, ok := strings.Cut(strings.ToLower(s), "x")
	if !ok {
		return 0, 0, bad
	}
	w, err1 := strconv.ParseFloat(ws, 64)
	h, err2 := strconv.ParseFloat(hs, 64)
	if err1 != nil || err2 != nil || !(w > 0) || !(h > 0) || w > maxLogicalSide || h > maxLogicalSide {
		return 0, 0, bad
	}
	return w, h, nil
}

// runSnapshot implements `tela snapshot`.
func runSnapshot(args []string, stdout, stderr io.Writer) int {
	opts, err := parseSnapshotArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, snapshotUsage)
			return 0
		}
		fmt.Fprintf(stderr, "tela snapshot: %v\n\n%s", err, snapshotUsage)
		return 2
	}
	if err := snapshot(opts, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "tela snapshot: %v\n", err)
		return 1
	}
	return 0
}

// Names of the environment variables that carry the parameters to the
// generated test (rather than test flags, which could clash with the
// package's own).
const (
	envW     = "TELA_SNAPSHOT_W"
	envH     = "TELA_SNAPSHOT_H"
	envScale = "TELA_SNAPSHOT_SCALE"
	envOut   = "TELA_SNAPSHOT_OUT"
)

// snapshotFile is the name of the generated test file inside the package
// directory (it exists only in the overlay).
const snapshotFile = "zz_tela_snapshot_test.go"

// snapshot renders opts.Pkg to opts.Out by running a generated test inside
// the package. internal/ packages cannot be imported from outside the module,
// so instead of building a separate main, `go test -overlay` adds a _test.go
// file to the target package, which can then call its Scene function.
func snapshot(opts snapshotOptions, stdout, stderr io.Writer) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return errors.New("the go command was not found in PATH; install Go (https://go.dev/dl) and try again")
	}
	out, err := filepath.Abs(opts.Out)
	if err != nil {
		return err
	}
	if st, err := os.Stat(out); err == nil && st.IsDir() {
		return fmt.Errorf("%s is a directory; give a file name such as %s", out, filepath.Join(out, "snapshot.png"))
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	dir, name, err := listPackage(goBin, opts.Pkg)
	if err != nil {
		return err
	}

	tmp, err := os.MkdirTemp("", "tela-snapshot-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, snapshotFile)
	if err := os.WriteFile(src, snapshotSource(name), 0o644); err != nil {
		return err
	}
	overlay := filepath.Join(tmp, "overlay.json")
	js, err := json.Marshal(map[string]map[string]string{"Replace": {filepath.Join(dir, snapshotFile): src}})
	if err != nil {
		return err
	}
	if err := os.WriteFile(overlay, js, 0o644); err != nil {
		return err
	}

	// The test writes into the temporary directory, so that a failed run
	// never leaves a stale or half-written file at the requested path.
	rendered := filepath.Join(tmp, "snapshot.png")
	cmd := exec.Command(goBin, "test", "-count=1", "-run", "^TestTelaSnapshot$", "-overlay", overlay, opts.Pkg)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		envW+"="+strconv.FormatFloat(opts.W, 'g', -1, 64),
		envH+"="+strconv.FormatFloat(opts.H, 'g', -1, 64),
		envScale+"="+strconv.FormatFloat(opts.Scale, 'g', -1, 64),
		envOut+"="+rendered,
	)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return explainFailure(opts.Pkg, name, output.String())
	}
	png, err := os.ReadFile(rendered)
	if err != nil {
		// The test passed but wrote nothing: the package defines its own
		// TestTelaSnapshot, or filtered it out.
		return fmt.Errorf("go test succeeded but wrote no image; the output was:\n%s", strings.TrimRight(output.String(), "\n"))
	}
	if err := os.WriteFile(out, png, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s\n", out)
	return nil
}

// listPackage returns the directory and the package name of pkg.
func listPackage(goBin, pkg string) (dir, name string, err error) {
	cmd := exec.Command(goBin, "list", "-f", "{{.Dir}}\n{{.Name}}", pkg)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", "", fmt.Errorf("cannot find package %q: %s\nuse a package path or a relative directory such as ./examples/shapes", pkg, msg)
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 || lines[0] == "" || lines[1] == "" {
		return "", "", fmt.Errorf("%q does not name exactly one package (go list printed %q); give a single package, without ...", pkg, strings.TrimSpace(stdout.String()))
	}
	return lines[0], lines[1], nil
}

// explainFailure turns the output of a failed `go test` into an error. A
// missing or mistyped Scene function, the usual mistake, gets a friendly
// message; anything else is passed through as go printed it.
func explainFailure(pkg, name, output string) error {
	output = strings.TrimRight(output, "\n")
	if strings.Contains(output, snapshotFile) &&
		(strings.Contains(output, "undefined: Scene") || strings.Contains(output, "cannot use Scene")) {
		return fmt.Errorf("package %s (%s) cannot be snapshotted: it must define\n\n"+
			"\tfunc Scene(w, h float32) *gfx.DisplayList\n\n"+
			"(github.com/kuronosu/tela/internal/gfx). This is the temporary snapshot contract until Preview() exists.\n\n"+
			"go reported:\n%s", name, pkg, output)
	}
	return fmt.Errorf("go test failed:\n%s", output)
}

// snapshotSource returns the source of the generated test for a package
// called pkgName. All imports have unusual aliases because import names live
// in the file scope of a package whose declarations they could collide with.
func snapshotSource(pkgName string) []byte {
	return []byte(fmt.Sprintf(snapshotTemplate, pkgName, envW, envH, envScale, envOut))
}

const snapshotTemplate = `// Code generated by "tela snapshot"; DO NOT EDIT.

package %s

import (
	telaimage "image"
	telapng "image/png"
	telaos "os"
	telastrconv "strconv"
	telatesting "testing"

	telagfx "github.com/kuronosu/tela/internal/gfx"
	telaraster "github.com/kuronosu/tela/internal/gfx/raster"
	telaplatform "github.com/kuronosu/tela/internal/platform"
)

// telaSnapshotScene pins the contract: Scene(w, h float32) *gfx.DisplayList.
var telaSnapshotScene func(w, h float32) *telagfx.DisplayList = Scene

func TestTelaSnapshot(t *telatesting.T) {
	num := func(name string) float64 {
		v, err := telastrconv.ParseFloat(telaos.Getenv(name), 64)
		if err != nil {
			t.Fatalf("%%s is not a number: %%v", name, err)
		}
		return v
	}
	w, h, scale := num(%q), num(%q), num(%q)
	size := telaplatform.Size{W: float32(w), H: float32(h)}
	phys := telaplatform.PhysicalSize(size, scale)
	if phys.X <= 0 || phys.Y <= 0 {
		t.Fatalf("empty image: %%vx%%v at scale %%v", w, h, scale)
	}
	img := telaimage.NewRGBA(telaimage.Rect(0, 0, phys.X, phys.Y))
	telaraster.Clear(img, telaimage.Rectangle{}, telagfx.White)
	telaraster.Render(img, telaSnapshotScene(size.W, size.H), telaraster.Options{Scale: float32(scale)})

	f, err := telaos.Create(telaos.Getenv(%q))
	if err != nil {
		t.Fatal(err)
	}
	if err := telapng.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
`
