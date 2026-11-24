package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// staticcheckVersion is the pinned staticcheck release. It is the one place
// that names it. The gate installs it into a temporary folder, so it never
// depends on a staticcheck binary on PATH.
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
	dir       string   // working directory relative to the root ("" is the root): a nested module
	stripTela bool     // remove every TELA_* variable from the environment
	noSlot    bool     // start at once, without waiting for a slot (the tests: the longest commands)
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

// run runs one subprocess in the repository root, waiting for a free slot
// unless the spec has noSlot.
func (g *gate) run(s cmdSpec) cmdOut {
	if !s.noSlot {
		g.sem <- struct{}{}
		defer func() { <-g.sem }()
	}
	start := time.Now()
	cmd := exec.Command(s.args[0], s.args[1:]...)
	cmd.Dir = filepath.Join(g.root, filepath.FromSlash(s.dir))
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

// What trimOutput keeps of a long test output: the first and the last
// message heads, a few lines of each message, and the last lines.
const (
	firstHeads  = 6
	lastHeads   = 4
	headContext = 3
	keepLast    = 5
)

// logHead matches a line of t.Error, t.Fatal or t.Log ("    x_test.go:12:
// got 1"), at any indentation (subtests indent theirs); testFrame matches a
// stack frame in a test file ("\t/path/x_test.go:12 +0x45"), the place a
// panic came from.
var (
	logHead   = regexp.MustCompile(`^\s*[^\s:]+\.go:\d+: `)
	testFrame = regexp.MustCompile(`_test\.go:\d+ \+0x`)
)

// trimOutput returns the output of a failing test, up to limit lines as it
// is. A longer one keeps what says why it failed: each message head (a
// file.go:N: line, a --- FAIL line, a panic) with its first lines, and the
// last lines; long dumps after a message (an aria snapshot, a goroutine
// dump) are cut, and every cut is marked. The tail alone lost the error
// lines when a dump followed them (2026-11-22).
func trimOutput(lines []string, limit int) string {
	if len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	var heads []int
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case logHead.MatchString(l), strings.HasPrefix(t, "--- FAIL"),
			strings.HasPrefix(t, "panic: "), strings.HasPrefix(t, "fatal error: "):
			heads = append(heads, i)
		case testFrame.MatchString(l) && i > 0 && (len(heads) == 0 || heads[len(heads)-1] != i-1):
			heads = append(heads, i-1) // the function line; the frame follows as its context
		}
	}
	if len(heads) == 0 {
		return tail(strings.Join(lines, "\n"), limit)
	}
	if len(heads) > firstHeads+lastHeads {
		heads = append(heads[:firstHeads:firstHeads], heads[len(heads)-lastHeads:]...)
	}
	keep := make([]bool, len(lines))
	for _, h := range heads {
		keep[h] = true
		ind := indent(lines[h])
		for j := h + 1; j < len(lines) && j <= h+headContext; j++ {
			if strings.TrimSpace(lines[j]) == "" || indent(lines[j]) <= ind {
				break
			}
			keep[j] = true
		}
	}
	for i := max(0, len(lines)-keepLast); i < len(lines); i++ {
		keep[i] = true
	}
	var out []string
	for i := 0; i < len(lines); {
		if keep[i] {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && !keep[j] {
			j++
		}
		out = append(out, fmt.Sprintf("... (%d lines left out)", j-i))
		i = j
	}
	return strings.Join(out, "\n")
}

// indent is the width of a line's leading white space, a tab as 8.
func indent(s string) int {
	n := 0
	for _, r := range s {
		switch r {
		case ' ':
			n++
		case '\t':
			n += 8
		default:
			return n
		}
	}
	return n
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

// moduleDirs are the examples that are modules of their own (ADR-298,
// ADR-315): the root's ./... does not reach them, so each step that checks the
// repository runs again inside each one.
var moduleDirs = []string{"examples/notes", "examples/texture"}

// inModules runs spec in every module of moduleDirs.
func inModules(s cmdSpec) []cmdSpec {
	out := make([]cmdSpec, len(moduleDirs))
	for i, dir := range moduleDirs {
		out[i] = s
		out[i].label += " (" + dir + ")"
		out[i].dir = dir
	}
	return out
}

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

func (g *gate) vetStep() stepResult { return g.commandStep(vetSpecs()) }

// vetSpecs are the vet step's commands.
func vetSpecs() []cmdSpec {
	specs := []cmdSpec{
		goCmd("go vet ./... (linux)", goos("linux"), "vet", "./..."),
		goCmd("go vet ./... (windows)", goos("windows"), "vet", "./..."),
		goCmd("go vet ./... (darwin)", goos("darwin"), "vet", "./..."),
		goCmd("go vet -tags telagpu ./... (linux)", goos("linux"), "vet", "-tags", "telagpu", "./..."),
		goCmd("go vet -tags telagpu ./... (windows)", goos("windows"), "vet", "-tags", "telagpu", "./..."),
		goCmd("go vet -tags telagpu ./... (darwin)", goos("darwin"), "vet", "-tags", "telagpu", "./..."),
		goCmd("go vet -tags telagpu ./... (linux/arm64)", []string{"GOOS=linux", "GOARCH=arm64"}, "vet", "-tags", "telagpu", "./..."),
	}
	for _, os := range []string{"linux", "windows", "darwin"} {
		specs = append(specs, inModules(goCmd("go vet ./... ("+os+")", goos(os), "vet", "./..."))...)
	}
	return specs
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
// linux/amd64, in bytes (ADR-290: 21,548,342 on 2026-11-16; ADR-320:
// 22,004,784 on 2026-11-24). Raising it is
// a trade: name in an ADR what the bytes bought.
const sizeBudget = 22_100_000

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

// staticcheckDir returns the folder staticcheck is installed into, and what
// to do once the step is done. It is a folder of the user cache, one per
// version, kept between runs: go install leaves an up-to-date binary alone
// (it compares build IDs, so a new Go toolchain links it again), which
// saves linking staticcheck on every run (1.4 of 1.7 s). Without a user
// cache it is a temporary folder that the step removes.
func staticcheckDir() (dir string, done func(), err error) {
	if cache, err := os.UserCacheDir(); err == nil {
		dir = filepath.Join(cache, "tela-gate", "staticcheck-"+staticcheckVersion)
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir, func() {}, nil
		}
	}
	dir, err = os.MkdirTemp("", "tela-gate-staticcheck-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { os.RemoveAll(dir) }, nil
}

// staticcheckStep runs staticcheck for Linux (with and without telagpu),
// Windows and macOS: code that only one system uses (a helper a backend
// calls) is unused on the others. go run would build staticcheck for the
// GOOS it checks, so the step installs it for the host first.
func (g *gate) staticcheckStep() stepResult {
	pkg := "honnef.co/go/tools/cmd/staticcheck@" + staticcheckVersion
	bin, done, err := staticcheckDir()
	if err != nil {
		return stepResult{Failures: []failure{{Title: "staticcheck", Detail: err.Error()}}}
	}
	defer done()
	install := g.run(cmdSpec{label: "go install " + pkg, args: []string{"go", "install", pkg}, env: []string{"GOBIN=" + bin}})
	if !install.ok() {
		return stepResult{Commands: cmdInfos([]cmdOut{install}), Failures: []failure{{Title: install.spec.label, Detail: failureDetail(install)}}}
	}
	sc := filepath.Join(bin, "staticcheck")
	if runtime.GOOS == "windows" {
		sc += ".exe"
	}
	return g.commandStep(append([]cmdSpec{
		{label: "staticcheck ./...", args: []string{sc, "./..."}},
		{label: "staticcheck -tags telagpu ./internal/... .", args: []string{sc, "-tags", "telagpu", "./internal/...", "."}},
		{label: "staticcheck ./... (windows)", args: []string{sc, "./..."}, env: goos("windows")},
		{label: "staticcheck ./... (darwin)", args: []string{sc, "./..."}, env: goos("darwin")},
	}, inModules(cmdSpec{label: "staticcheck ./...", args: []string{sc, "./..."}})...))
}

func (g *gate) lintStep() stepResult {
	return g.commandStep(append([]cmdSpec{
		goCmd("tela lint ./...", nil, "run", "./cmd/tela", "lint", "./..."),
	}, inModules(goCmd("tela lint ./...", nil, "run", "github.com/kuronosu/tela/cmd/tela", "lint", "./..."))...))
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
// go test's own vet is off: it runs a subset of the analyzers that the vet
// step runs over the same packages and test files (linux, with and
// without telagpu, and the notes module), and it ran again for every
// package whose tests ran, before them.
func (g *gate) testStep() stepResult {
	outs := g.runAll(testSpecs())
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

// testSpecs are the test step's commands. The first, the longest, starts
// at once (noSlot): when it lost the race for a slot it waited 4 s behind
// vet and staticcheck, on a gate whose time it sets. The two others keep
// waiting for theirs: with all three started at once the gate took 201 s
// instead of 188 after an edit of internal/core, likely from more go
// commands compiling the same packages at the same time.
func testSpecs() []cmdSpec {
	return append([]cmdSpec{
		{label: "go test -short ./...", args: []string{"go", "test", "-short", "-vet=off", "-json", "./..."}, stripTela: true, noSlot: true},
		{label: "go test -short -tags telagpu ./internal/vk/... ./internal/gfx/gpu/...", args: []string{"go", "test", "-short", "-vet=off", "-json", "-tags", "telagpu", "./internal/vk/...", "./internal/gfx/gpu/..."}, stripTela: true},
	}, inModules(cmdSpec{label: "go test -short ./...", args: []string{"go", "test", "-short", "-vet=off", "-json", "./..."}, stripTela: true})...)
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
	out := failure{Title: title, Detail: trimOutput(f.Output, maxTail)}
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
