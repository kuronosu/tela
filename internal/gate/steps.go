package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// staticcheckVersion is the pinned staticcheck release. It is the one place
// that names it. It runs through `go run`, so the gate never depends on a
// staticcheck binary on PATH.
const staticcheckVersion = "2026.2.2"

// modulePath is trimmed from package paths in reports.
const modulePath = "github.com/kuronosu/tela"

// maxTail is how many lines of a command's output a failure shows.
const maxTail = 30

// gate holds what the steps share.
type gate struct {
	root      string
	sem       chan struct{} // bounds concurrent subprocesses
	flakeDocs []flakeDoc    // documents that name known flakes
}

// flakeDoc is a document whose lines may name a flaky test.
type flakeDoc struct {
	file  string
	lines []string
}

// flakeFiles are searched in order for the name of a failing test: open
// tasks in the roadmap, small limitations in LIMITATIONS.md (ADR-290).
var flakeFiles = []string{"docs/ROADMAP.md", "docs/LIMITATIONS.md"}

func newGate(root string) *gate {
	g := &gate{root: root, sem: make(chan struct{}, 4)}
	for _, f := range flakeFiles {
		if data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			g.flakeDocs = append(g.flakeDocs, flakeDoc{f, strings.Split(string(data), "\n")})
		}
	}
	return g
}

// cmdSpec is one subprocess: a go command with extra environment.
type cmdSpec struct {
	label     string
	args      []string
	env       []string // KEY=VALUE additions (GOOS, GOARCH)
	stripTela bool     // remove every TELA_* variable from the environment
}

type cmdOut struct {
	spec   cmdSpec
	stdout []byte
	stderr []byte
	err    error
	dur    time.Duration
}

func (c cmdOut) ok() bool { return c.err == nil }

// combined is the output as a person reads it.
func (c cmdOut) combined() string {
	return strings.TrimRight(string(c.stderr)+string(c.stdout), "\n")
}

// buildEnv returns the environment of a subprocess: the gate's own, without
// the variables the gate sets itself, with CGO_ENABLED=0 and the additions.
func buildEnv(base []string, extra []string, stripTela bool) []string {
	out := make([]string, 0, len(base)+len(extra)+1)
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "CGO_ENABLED", "GOOS", "GOARCH":
			continue
		}
		if stripTela && strings.HasPrefix(strings.ToUpper(k), "TELA_") {
			continue
		}
		out = append(out, kv)
	}
	out = append(out, "CGO_ENABLED=0")
	return append(out, extra...)
}

// run runs one subprocess in the repository root, waiting for a free slot.
func (g *gate) run(s cmdSpec) cmdOut {
	g.sem <- struct{}{}
	defer func() { <-g.sem }()
	start := time.Now()
	cmd := exec.Command(s.args[0], s.args[1:]...)
	cmd.Dir = g.root
	cmd.Env = buildEnv(os.Environ(), s.env, s.stripTela)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	return cmdOut{spec: s, stdout: so.Bytes(), stderr: se.Bytes(), err: err, dur: time.Since(start)}
}

// runAll runs the specs concurrently and returns their results in order.
func (g *gate) runAll(specs []cmdSpec) []cmdOut {
	outs := make([]cmdOut, len(specs))
	var wg sync.WaitGroup
	for i, s := range specs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outs[i] = g.run(s)
		}()
	}
	wg.Wait()
	return outs
}

// git runs git in the repository root and returns its trimmed output.
func (g *gate) git(args ...string) (string, error) {
	s, err := g.gitRaw(args...)
	return strings.TrimSpace(s), err
}

func (g *gate) gitRaw(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.quotepath=false"}, args...)...)
	cmd.Dir = g.root
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// tail returns the last n lines of s.
func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = append([]string{fmt.Sprintf("... (%d earlier lines left out)", len(lines)-n)}, lines[len(lines)-n:]...)
	}
	return strings.Join(lines, "\n")
}

func cmdInfos(outs []cmdOut) []commandInfo {
	infos := make([]commandInfo, len(outs))
	for i, o := range outs {
		infos[i] = commandInfo{Label: o.spec.label, Seconds: o.dur.Seconds(), OK: o.ok()}
	}
	return infos
}

// commandStep runs the specs concurrently; every one that exits non-zero is
// a failure carrying the tail of its output.
func (g *gate) commandStep(specs []cmdSpec) stepResult {
	outs := g.runAll(specs)
	r := stepResult{Commands: cmdInfos(outs)}
	for _, o := range outs {
		if !o.ok() {
			r.Failures = append(r.Failures, failure{Title: o.spec.label, Detail: failureDetail(o)})
		}
	}
	return r
}

func failureDetail(o cmdOut) string {
	if out := o.combined(); out != "" {
		return tail(out, maxTail)
	}
	return o.err.Error()
}

func goCmd(label string, env []string, args ...string) cmdSpec {
	return cmdSpec{label: label, args: append([]string{"go"}, args...), env: env}
}

func goos(os string) []string { return []string{"GOOS=" + os} }

// fmtStep checks that every Go file of the repository is gofmt-formatted,
// in process. The files come from git, not from walking the tree: gofmt -l .
// also walks node_modules.
func (g *gate) fmtStep() stepResult {
	start := time.Now()
	out, err := g.gitRaw("ls-files", "-z", "-co", "--exclude-standard", "--", "*.go")
	if err != nil {
		return stepResult{Failures: []failure{{Title: "git ls-files", Detail: err.Error()}}}
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	bad := make([]string, len(files))
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				bad[i] = checkFormatted(filepath.Join(g.root, filepath.FromSlash(files[i])))
			}
		}()
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	r := stepResult{Commands: []commandInfo{{Label: fmt.Sprintf("format %d files", len(files)), Seconds: time.Since(start).Seconds(), OK: true}}}
	var names []string
	for i, b := range bad {
		if b != "" {
			names = append(names, files[i]+": "+b)
		}
	}
	if len(names) > 0 {
		r.Failures = []failure{{Title: "files that gofmt would change (run gofmt -w on them)", Detail: strings.Join(names, "\n")}}
		r.Commands[0].OK = false
	}
	return r
}

// checkFormatted returns "" when the file is formatted (or gone, a deleted
// file git still lists), else a description of the problem.
func checkFormatted(path string) string {
	src, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return err.Error()
	}
	res, err := format.Source(src)
	if err != nil {
		return err.Error()
	}
	if !bytes.Equal(res, src) {
		return "not formatted"
	}
	return ""
}

func (g *gate) vetStep() stepResult {
	return g.commandStep([]cmdSpec{
		goCmd("go vet ./... (linux)", goos("linux"), "vet", "./..."),
		goCmd("go vet ./... (windows)", goos("windows"), "vet", "./..."),
		goCmd("go vet ./... (darwin)", goos("darwin"), "vet", "./..."),
		goCmd("go vet -tags telagpu ./... (linux)", goos("linux"), "vet", "-tags", "telagpu", "./..."),
		goCmd("go vet -tags telagpu ./... (windows)", goos("windows"), "vet", "-tags", "telagpu", "./..."),
		goCmd("go vet -tags telagpu ./... (darwin)", goos("darwin"), "vet", "-tags", "telagpu", "./..."),
		goCmd("go vet -tags telagpu ./... (linux/arm64)", []string{"GOOS=linux", "GOARCH=arm64"}, "vet", "-tags", "telagpu", "./..."),
	})
}

func (g *gate) buildStep() stepResult {
	// Linux only: every other system and tag is type-checked by vet, and a
	// build for each would link every program of the repository again,
	// which is most of the gate's time after an edit of internal/core. CI's
	// build matrix builds all six targets.
	return g.commandStep([]cmdSpec{
		goCmd("go build ./... (linux)", goos("linux"), "build", "./..."),
	})
}

// gpuDepsStep checks that a program built without the telagpu tag does not
// link the GPU path.
func (g *gate) gpuDepsStep() stepResult {
	o := g.run(goCmd("go list -deps ./examples/hello", goos("linux"), "list", "-deps", "./examples/hello"))
	r := stepResult{Commands: cmdInfos([]cmdOut{o})}
	if !o.ok() {
		r.Failures = []failure{{Title: o.spec.label, Detail: failureDetail(o)}}
		return r
	}
	var leaked []string
	for _, line := range strings.Split(string(o.stdout), "\n") {
		for _, bad := range []string{"gfx/gpu", "internal/vk", "purego"} {
			if strings.Contains(line, bad) {
				leaked = append(leaked, line)
				break
			}
		}
	}
	if len(leaked) > 0 {
		r.Failures = []failure{{
			Title:  "a build without telagpu links the GPU path (ADR-281)",
			Detail: "./examples/hello depends on:\n" + strings.Join(leaked, "\n"),
		}}
	}
	return r
}

// sizeBudget is the largest size of the examples/hello binary on
// linux/amd64, in bytes (ADR-290: 21,548,342 on 2026-11-16). Raising it is
// a trade: name in an ADR what the bytes bought.
const sizeBudget = 22_000_000

// sizeStep builds examples/hello for linux/amd64 and checks its size
// against sizeBudget: what every app pays before its own code.
func (g *gate) sizeStep() stepResult {
	dir, err := os.MkdirTemp("", "tela-gate-size")
	if err != nil {
		return stepResult{Failures: []failure{{Title: "size", Detail: err.Error()}}}
	}
	defer os.RemoveAll(dir)
	bin := filepath.Join(dir, "hello")
	o := g.run(goCmd("go build ./examples/hello (linux/amd64)", []string{"GOOS=linux", "GOARCH=amd64"}, "build", "-o", bin, "./examples/hello"))
	r := stepResult{Commands: cmdInfos([]cmdOut{o})}
	if !o.ok() {
		r.Failures = []failure{{Title: o.spec.label, Detail: failureDetail(o)}}
		return r
	}
	fi, err := os.Stat(bin)
	if err != nil {
		r.Failures = []failure{{Title: "size", Detail: err.Error()}}
		return r
	}
	r.Size = &sizeInfo{Bytes: fi.Size(), Budget: sizeBudget}
	if fi.Size() > sizeBudget {
		r.Failures = []failure{{
			Title:  "examples/hello is over its size budget",
			Detail: fmt.Sprintf("%d bytes on linux/amd64, budget %d (sizeBudget in internal/gate/steps.go): find what grew with `go tool nm -size -sort size`", fi.Size(), sizeBudget),
		}}
	}
	return r
}

func (g *gate) staticcheckStep() stepResult {
	pkg := "honnef.co/go/tools/cmd/staticcheck@" + staticcheckVersion
	return g.commandStep([]cmdSpec{
		{label: "staticcheck ./...", args: []string{"go", "run", pkg, "./..."}},
		{label: "staticcheck -tags telagpu ./internal/... .", args: []string{"go", "run", pkg, "-tags", "telagpu", "./internal/...", "."}},
	})
}

func (g *gate) lintStep() stepResult {
	return g.commandStep([]cmdSpec{goCmd("tela lint ./...", nil, "run", "./cmd/tela", "lint", "./...")})
}

func (g *gate) docsStep() stepResult {
	r := stepResult{Docs: checkDocs(g.root, docBudgets)}
	for _, d := range r.Docs {
		switch {
		case d.Err != "":
			r.Failures = append(r.Failures, failure{Title: d.Label, Detail: d.Err})
		case d.Words > d.Budget:
			r.Failures = append(r.Failures, failure{
				Title:  fmt.Sprintf("%s is over its word budget", d.Label),
				Detail: fmt.Sprintf("%d words, budget %d: shorten it (the budgets are in the table docBudgets of internal/gate/docs.go)", d.Words, d.Budget),
			})
		}
	}
	return r
}

// testStep runs the short tests with Go's test cache and parses the JSON.
func (g *gate) testStep() stepResult {
	outs := g.runAll([]cmdSpec{
		{label: "go test -short ./...", args: []string{"go", "test", "-short", "-json", "./..."}, stripTela: true},
		{label: "go test -short -tags telagpu ./internal/vk/... ./internal/gfx/gpu/...", args: []string{"go", "test", "-short", "-json", "-tags", "telagpu", "./internal/vk/...", "./internal/gfx/gpu/..."}, stripTela: true},
	})
	r := stepResult{Commands: cmdInfos(outs)}
	sum := &testSummary{}
	var tests, pkgs []slow
	for _, o := range outs {
		run, _ := parseTestJSON(bytes.NewReader(o.stdout))
		p, c, t := run.counts()
		sum.Packages += p
		sum.Cached += c
		sum.Tests += t
		st, sp := run.slowest(10)
		tests, pkgs = append(tests, st...), append(pkgs, sp...)
		fails := run.failures()
		for _, f := range fails {
			r.Failures = append(r.Failures, g.describeFailure(f))
		}
		if !o.ok() && len(fails) == 0 {
			detail := strings.Join(append(append([]string{}, run.Stray...), string(o.stderr)), "\n")
			r.Failures = append(r.Failures, failure{Title: o.spec.label + " exited with an error", Detail: tail(strings.TrimSpace(detail), maxTail)})
		}
	}
	sum.SlowestTests, sum.SlowestPackages = top(tests, 10), top(pkgs, 10)
	r.Tests = sum
	return r
}

// describeFailure turns a failing test into a report entry, labeling a
// test that the roadmap or LIMITATIONS.md names: likely a known flake. It
// still fails the step.
func (g *gate) describeFailure(f testFailure) failure {
	short := shortPkg(f.Package)
	title := short + " " + f.Test
	if f.Test == "" {
		title = short + " (" + f.Reason + ")"
	}
	out := failure{Title: title, Detail: tail(strings.Join(f.Output, "\n"), maxTail)}
	for _, d := range g.flakeDocs {
		if line, ok := findFlake(d.lines, f.Test); ok {
			out.Note = fmt.Sprintf("named in %s:%d (a known flake?)", d.file, line)
			break
		}
	}
	return out
}

func shortPkg(p string) string {
	if p == modulePath {
		return "."
	}
	return strings.TrimPrefix(p, modulePath+"/")
}
