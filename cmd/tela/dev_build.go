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
	"slices"
	"strings"
	"time"
	"unicode"
)

// devBuildTags is the build tag of the dev-loop runtime (ADR-111).
const devBuildTags = "teladev"

// devBuildFlags are the go build flags of every dev build, in one place
// (ADR-116). goflags is the GOFLAGS the go command applies (environment or
// go env -w): a flag given on the command line replaces the same flag of
// GOFLAGS, so a flag the user sets there is left to them. The output path,
// the overlay and the package are added by the builder.
//
//   - -tags teladev turns the dev-loop runtime on (ADR-111); the tags of
//     GOFLAGS follow it (devTags), since -tags cannot be left to the user.
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
	flags := []string{"-tags", devTags(goflags)}
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

// hasGoFlag reports whether goflags (a GOFLAGS list) sets the build flag
// name.
func hasGoFlag(goflags, name string) bool {
	for _, f := range splitGoFlags(goflags) {
		if k, _, _ := goFlag(f); k == name {
			return true
		}
	}
	return false
}

// devTags is the -tags value of the dev builds and of go list: teladev,
// then the tags GOFLAGS sets, which a -tags on the command line would
// otherwise replace. It reads GOFLAGS as the go command does: an entry is
// one flag, so only "-tags=..." (or "--tags=...") sets tags, and the last
// one wins ("-tags a,b" is rejected by the go command as a non-flag
// "a,b"); the value is a comma-separated list, or the legacy
// space-separated one ("-tags=a b", quoted whole in GOFLAGS), split by
// tagsValue.
func devTags(goflags string) string {
	var user []string
	for _, f := range splitGoFlags(goflags) {
		if name, value, ok := goFlag(f); name == "tags" && ok {
			user = tagsValue(value)
		}
	}
	tags := []string{devBuildTags}
	for _, t := range user {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return strings.Join(tags, ",")
}

// tagsValue splits the value of a -tags flag as the go command does
// (cmd/go/internal/work's tagsFlag): a value with a space or a single quote
// is the legacy list, split by cmd/internal/quoted (a word may be quoted
// whole), anything else is split on commas. Tags that no build constraint
// can name (go/build/constraint takes letters, digits, '_' and '.') are
// dropped: they select nothing, and joined with commas into the command
// line's -tags they would be split differently ("a b" would make the go
// command read the whole list as the legacy form, teladev included).
func tagsValue(v string) []string {
	var words []string
	if strings.ContainsAny(v, " '") {
		words = splitGoFlags(v)
	} else {
		words = strings.Split(v, ",")
	}
	return slices.DeleteFunc(words, func(t string) bool { return !isBuildTag(t) })
}

// isBuildTag reports whether t can appear in a build constraint
// (go/build/constraint's isValidTag).
func isBuildTag(t string) bool {
	if t == "" {
		return false
	}
	for _, c := range t {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

// goFlag splits a GOFLAGS entry ("-name=value", "--name") into the flag's
// name and value; ok says whether it has a value. An entry that is not a
// flag (no leading '-', as the go command requires) has no name.
func goFlag(f string) (name, value string, ok bool) {
	if !strings.HasPrefix(f, "-") || strings.HasPrefix(f, "---") {
		return "", "", false
	}
	f = strings.TrimPrefix(f, "-")
	f = strings.TrimPrefix(f, "-")
	return strings.Cut(f, "=")
}

// splitGoFlags splits GOFLAGS into entries as the go command does
// (cmd/internal/quoted): separated by spaces, an entry may be quoted whole
// in single or double quotes to hold spaces ("-ldflags=-s -w").
func splitGoFlags(s string) []string {
	const space = " \t\n\r"
	var fs []string
	for {
		s = strings.TrimLeft(s, space)
		if s == "" {
			return fs
		}
		if q := s[0]; q == '"' || q == '\'' {
			i := strings.IndexByte(s[1:], q)
			if i < 0 { // unterminated: the go command rejects it
				return append(fs, s[1:])
			}
			fs = append(fs, s[1:1+i])
			s = s[2+i:]
			continue
		}
		i := strings.IndexAny(s, space)
		if i < 0 {
			return append(fs, s)
		}
		fs = append(fs, s[:i])
		s = s[i:]
	}
}

// goEnv is the environment of every go command tela dev runs: its builds,
// go list and go env.
func goEnv() []string {
	return append(os.Environ(), "CGO_ENABLED=0")
}

// goEnvInfo is what go env says in a directory: the GOFLAGS the go command
// uses there, which may come from go env -w or the go.env of the toolchain
// the directory selects as well as from the environment, and GOMOD and
// GOWORK, the files that select that toolchain.
type goEnvInfo struct {
	flags         string
	gomod, gowork string // empty when go env failed
}

// readGoEnv runs go env in dir with env. When it fails, flags is the
// environment's GOFLAGS.
func readGoEnv(goBin, dir string, env []string) goEnvInfo {
	cmd := exec.Command(goBin, "env", "-json", "GOFLAGS", "GOMOD", "GOWORK")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.Output()
	var v struct{ GOFLAGS, GOMOD, GOWORK string }
	if err == nil {
		err = json.Unmarshal(out, &v)
	}
	if err != nil {
		return goEnvInfo{flags: os.Getenv("GOFLAGS")}
	}
	return goEnvInfo{flags: strings.TrimSpace(v.GOFLAGS), gomod: v.GOMOD, gowork: v.GOWORK}
}

// sameIn reports whether go env, with the same environment, says the same
// in modDir, a module root: GOFLAGS depends on the directory only through
// the toolchain its go.work or go.mod selects (cmd/go/internal/toolchain),
// found by walking up from the directory. When the directory g was read in
// belongs to the module rooted at modDir, the walk from modDir is the end
// of the walk from there, so it finds the same files unless the go.work
// found lies below modDir.
func (g goEnvInfo) sameIn(modDir string) bool {
	modDir = filepath.Clean(modDir)
	if g.gomod == "" || filepath.Dir(g.gomod) != modDir {
		return false // another module, no module, or go env failed
	}
	switch g.gowork {
	case "", "off":
		return true
	}
	return under(modDir, []string{filepath.Dir(g.gowork)}) != ""
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
	// goflags is the user's GOFLAGS (devBuildFlags), the target's.
	goflags string
}

// newGoBuilder prepares the builds of t into outDir, writing the overlay
// files of a generated runner there.
func newGoBuilder(goBin string, t *devTarget, outDir string) (*goBuilder, error) {
	b := &goBuilder{
		goBin:   goBin,
		dir:     t.modDir,
		arg:     t.arg,
		outDir:  outDir,
		env:     goEnv(),
		goflags: t.goflags,
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
