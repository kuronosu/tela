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

// devBuildFlags are the go build flags of every dev build, in one place
// (ADR-116). goflags is the GOFLAGS the go command applies (environment or
// go env -w): a flag given on the command line replaces the same flag of
// GOFLAGS, so a flag the user sets there is left to them. The output path,
// the overlay and the package are added by the builder.
//
//   - -tags teladev turns the dev-loop runtime on (ADR-111).
//   - -buildvcs=false skips the version-control stamp, which runs git (or
//     another VCS) on every build. Dev mode stays on: the supervisor sets
//     TELA_DEV=1, and a binary without the stamp and without -trimpath is
//     not a release build anyway (ADR-085).
//   - -ldflags=-s -w: no symbol table and no DWARF, so the link writes less.
//     Panics keep their file:line (the runtime reads pclntab, which stays),
//     and so do runtime.FuncForPC and the component names; a debugger
//     cannot attach to a dev build.
//   - -gcflags=-l: no inlining in the package built (the app's main
//     package, or the generated runner), which the compiler spends most
//     time on there; Tela and the other packages keep their usual flags
//     and their cached builds.
//
// Measured on examples/dashboard (ADR-116): -ldflags saves about 110 ms of
// a 600 ms build, -gcflags=-l about 90 ms, -buildvcs=false about 25 ms in a
// git checkout.
func devBuildFlags(goflags string) []string {
	flags := []string{"-tags", devBuildTags}
	if !hasGoFlag(goflags, "buildvcs") {
		flags = append(flags, "-buildvcs=false")
	}
	if !hasGoFlag(goflags, "ldflags") {
		flags = append(flags, "-ldflags=-s -w")
	}
	if !hasGoFlag(goflags, "gcflags") {
		flags = append(flags, "-gcflags=-l")
	}
	return flags
}

// hasGoFlag reports whether goflags (a space-separated GOFLAGS list) sets
// the build flag name.
func hasGoFlag(goflags, name string) bool {
	for _, f := range strings.Fields(goflags) {
		f = strings.TrimLeft(f, "-")
		if k, _, _ := strings.Cut(f, "="); k == name {
			return true
		}
	}
	return false
}

// goEnvFlags returns the GOFLAGS the go command uses in dir, which may come
// from go env -w as well as from the environment; "" when it cannot tell.
func goEnvFlags(goBin, dir string, env []string) string {
	cmd := exec.Command(goBin, "env", "GOFLAGS")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return os.Getenv("GOFLAGS")
	}
	return strings.TrimSpace(string(out))
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
	// goflags is the user's GOFLAGS (devBuildFlags), filled by devMain.
	goflags string
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
	args := append([]string{"build"}, devBuildFlags(b.goflags)...)
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
