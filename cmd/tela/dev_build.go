package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// devBuildTags is the build tag of the dev-loop runtime (ADR-111).
const devBuildTags = "teladev"

// devBuildFlags are the go build flags of every dev build, in one place so
// they can be tuned for speed (T6): -tags teladev turns the dev-loop
// runtime on. The output path, the overlay and the package are added by the
// builder.
func devBuildFlags() []string {
	return []string{"-tags", devBuildTags}
}

// builder produces runnable builds. build is called on its own goroutine
// and must return soon after ctx is cancelled (a newer save).
type builder interface {
	build(ctx context.Context, gen int) buildResult
}

// buildResult is the outcome of one build.
type buildResult struct {
	gen int
	// What to run: the executable, its arguments and extra environment.
	exe  string
	args []string
	env  []string
	// owned says exe is a temporary build that tela dev deletes once it is
	// no longer needed.
	owned bool
	// output is what the compiler printed (the error text on failure).
	output string
	// err is nil on success, context.Canceled when a newer save cancelled
	// the build, or the failure.
	err        error
	start, end time.Time
}

// goBuilder runs go build with the teladev tag.
type goBuilder struct {
	goBin   string
	dir     string // working directory: the module root
	arg     string // the package, relative to dir
	overlay string // -overlay file, or ""
	outDir  string
	env     []string
}

// newGoBuilder prepares the builds of t into outDir, writing the overlay
// files of a generated runner there.
func newGoBuilder(goBin string, t *devTarget, outDir string) (*goBuilder, error) {
	b := &goBuilder{
		goBin:  goBin,
		dir:    t.modDir,
		arg:    t.arg,
		outDir: outDir,
		env:    append(os.Environ(), "CGO_ENABLED=0"),
	}
	if len(t.overlay) > 0 {
		replace := map[string]string{}
		i := 0
		for path, src := range t.overlay {
			i++
			f := filepath.Join(outDir, fmt.Sprintf("runner-%d.go", i))
			if err := os.WriteFile(f, src, 0o644); err != nil {
				return nil, err
			}
			replace[path] = f
		}
		js, err := json.Marshal(map[string]map[string]string{"Replace": replace})
		if err != nil {
			return nil, err
		}
		b.overlay = filepath.Join(outDir, "overlay.json")
		if err := os.WriteFile(b.overlay, js, 0o644); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// exeName is the binary of build gen.
func exeName(gen int) string {
	name := fmt.Sprintf("app-%d", gen)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// args returns the go command line for a build into out.
func (b *goBuilder) args(out string) []string {
	args := append([]string{"build"}, devBuildFlags()...)
	args = append(args, "-o", out)
	if b.overlay != "" {
		args = append(args, "-overlay", b.overlay)
	}
	return append(args, b.arg)
}

func (b *goBuilder) build(ctx context.Context, gen int) buildResult {
	res := buildResult{gen: gen, exe: filepath.Join(b.outDir, exeName(gen)), owned: true, start: time.Now()}
	cmd := exec.CommandContext(ctx, b.goBin, b.args(res.exe)...)
	cmd.Dir = b.dir
	cmd.Env = b.env
	// A cancelled build is interrupted, so that the go command stops its
	// compilers too, and killed if it does not exit soon.
	cmd.Cancel = func() error { return interruptProcess(cmd.Process) }
	cmd.WaitDelay = 2 * time.Second
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	res.end = time.Now()
	res.output = strings.TrimRight(out.String(), "\n")
	switch {
	case ctx.Err() != nil:
		res.err = ctx.Err()
	case err != nil:
		if res.output == "" {
			res.output = err.Error()
		}
		res.err = err
	}
	if res.err != nil {
		os.Remove(res.exe)
	}
	return res
}

// remove deletes the executable of an owned build.
func (r buildResult) remove() {
	if r.owned && r.exe != "" {
		os.Remove(r.exe)
	}
}

// isCancelled reports whether a build was cancelled by a newer one.
func (r buildResult) isCancelled() bool {
	return errors.Is(r.err, context.Canceled)
}
