// Command memreport measures where the memory of the examples goes (ADR-240).
// It is a development tool, not public API: nothing outside the repository
// uses it.
//
//	go run ./internal/testutil/memreport                  # every case, against the baseline
//	go run ./internal/testutil/memreport -only dashboard  # one example
//	go run ./internal/testutil/memreport -cases 'x2/session|hello'
//	go run ./internal/testutil/memreport -update          # rewrite the baseline (of the cases run)
//	go run ./internal/testutil/memreport -profiles /tmp/mem   # keep the pprof files
//
// # What it does
//
// It builds the test binary of each example (go test -c, CGO_ENABLED=0) once
// and runs its TestMemory tests, which are skipped unless TELA_MEM is set
// (examples/*/memory_test.go, helpers in internal/testutil/memprobe). Every
// case runs in its own process, twice:
//
//   - TELA_MEM=rss: the runtime's default settings. This run gives the
//     numbers: the heap after a GC (HeapInuse, HeapAlloc), the heap's peak
//     during the case, the bytes allocated during it, the runtime's Sys and
//     the process's RSS (VmRSS, split in anonymous, file-backed and shared
//     memory; VmHWM is the peak RSS), from /proc/self/status.
//   - TELA_MEM=profile with GODEBUG=memprofilerate=1: every allocation is
//     recorded, and the live heap is split by subsystem from the runtime's
//     heap profile: fonts, glyph cache, framebuffers and present copies,
//     raster scratch, backdrop cache, display lists, images, text layout,
//     trees and other (internal/testutil/memprobe/rules.go has the rules).
//     Recording every allocation costs memory and time, so this run gives
//     the breakdown only, never the RSS.
//
// Cases: each example mounted at the size of its App and at scale 1 and 2
// ("x1/rest", "x2/rest"; telatest's headless window, with the host's
// framebuffer and the headless backend's copy of the presented frame), and
// for the dashboard also its rig of bench_test.go at 1920x1080 at rest
// ("rig-rest") and after the scripted session of about 1100 ticks
// ("session"; the rig has its own framebuffer and a BGRA buffer standing in
// for a backend's).
//
// # Reading the numbers
//
//   - heap in use is HeapInuse after runtime.GC and debug.FreeOSMemory: the
//     spans that hold live objects, including the free slots in them. It is
//     what the Go heap costs once the garbage is gone, and the number to
//     compare between runs.
//   - heap alloc is the bytes of live objects; heap peak is the largest
//     HeapAlloc seen while the case ran, so it includes the garbage a GC had
//     not collected yet (a GC runs when the heap doubles: the peak is about
//     the live heap plus the garbage of a few frames). live peak is the
//     largest live heap after a GC at the marks of the case (the end of each
//     scene of the session).
//   - allocated is the bytes allocated during the case (TotalAlloc): the
//     garbage a case makes, not what it keeps.
//   - RSS is what the process holds in RAM. A test binary's RSS includes the
//     test harness and the program text it touches (a few MB, "file"), the
//     runtime's own structures, and the heap's free spans that were not
//     returned to the OS yet (FreeOSMemory is called before reading). Anon
//     is the heap and stacks; file is the binary's own pages (the test binary
//     is built under the user cache directory, because the pages of a binary
//     on a tmpfs such as /tmp would count as shmem instead); shmem is shared
//     memory (the Wayland buffers of a real window would show here; the
//     headless backend has none).
//   - Every case runs in its own process because Go returns freed memory to
//     the OS late and keeps the high-water mark of earlier work: one case
//     after another in a process would measure the largest of them.
//   - The numbers of a Go heap are deterministic to a few percent. RSS
//     depends on the kernel and the machine; the breakdown does not depend on
//     either, except for the rasterizer's per-worker buffers, which depend on
//     GOMAXPROCS (the baseline records it). Compare runs on the same machine.
//
// Real backends keep their own presentation buffers, which the headless
// backend stands in for with one copy of the frame: the report lists them as
// computed sizes for the physical size of each case (see backends.go).
//
// # Flags
//
// See -h. -update rewrites the baseline file (baseline.json next to this
// file) with the cases run, keeping the others. -tolerance flags the numbers
// that grew over the baseline by more than that percentage with "!" (the heap
// in use, the live heap, the bytes allocated, the RSS and the subsystems; not
// the peaks, which move with the timing of the garbage collector); with
// -strict they also make the exit status 1. The exit status is also 1 when a
// case fails.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/testutil/memprobe"
)

// example is an example with a TestMemory test and its cases.
type example struct {
	name  string
	cases []string
}

var examples = []example{
	{"hello", []string{"x1/rest", "x2/rest"}},
	{"kit", []string{"x1/rest", "x2/rest"}},
	{"dashboard", []string{"x1/rest", "x2/rest", "x1/rig-rest", "x2/rig-rest", "x1/session", "x2/session"}},
	{"notes", []string{"x1/rest", "x2/rest"}},
}

// caseResult is what the two runs of a case gave.
type caseResult struct {
	key string // "dashboard/x2/session"
	rss *memprobe.Result
	// prof is the profile run's result: the breakdown, and a heap that
	// the recording inflated.
	prof *memprobe.Result
	err  error
}

func main() {
	log := func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }
	var (
		update    = flag.Bool("update", false, "rewrite the baseline with the cases run")
		only      = flag.String("only", "", "comma-separated examples to run (default: all)")
		casesRe   = flag.String("cases", "", "regexp: run only the cases whose \"example/case\" matches")
		profiles  = flag.String("profiles", "", "directory to keep the pprof heap and allocs profiles in")
		verbose   = flag.Bool("v", false, "show the output of the test processes")
		tolerance = flag.Float64("tolerance", 0, "flag numbers that grew over the baseline by more than this percentage (0: off)")
		strict    = flag.Bool("strict", false, "exit 1 when a number is flagged by -tolerance")
		sites     = flag.Int("sites", 0, "list the n largest allocation sites of the live heap of each case, with their category")
		basePath  = flag.String("baseline", "", "baseline file (default: baseline.json next to this tool's source)")
		noBase    = flag.Bool("nobaseline", false, "do not compare with the baseline")
		binFlag   = flag.String("bindir", "", "directory for the test binaries (default: under the user cache directory; see below)")
	)
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		log("memreport: %v", err)
		os.Exit(2)
	}
	if *basePath == "" {
		*basePath = filepath.Join(root, "internal", "testutil", "memreport", "baseline.json")
	}
	var re *regexp.Regexp
	if *casesRe != "" {
		if re, err = regexp.Compile(*casesRe); err != nil {
			log("memreport: -cases: %v", err)
			os.Exit(2)
		}
	}
	want := map[string]bool{}
	for n := range strings.SplitSeq(*only, ",") {
		if n = strings.TrimSpace(n); n != "" {
			want[n] = true
		}
	}
	for n := range want {
		if !hasExample(n) {
			log("memreport: -only: no example %q (have %s)", n, exampleNames())
			os.Exit(2)
		}
	}
	if *profiles != "" {
		if *profiles, err = filepath.Abs(*profiles); err != nil {
			log("memreport: %v", err)
			os.Exit(2)
		}
	}

	tmp, err := binDir(*binFlag)
	if err != nil {
		log("memreport: %v", err)
		os.Exit(2)
	}
	defer os.RemoveAll(tmp)

	start := time.Now()
	var results []*caseResult
	failed := false
	for _, ex := range examples {
		if len(want) > 0 && !want[ex.name] {
			continue
		}
		var cases []string
		for _, c := range ex.cases {
			if re == nil || re.MatchString(ex.name+"/"+c) {
				cases = append(cases, c)
			}
		}
		if len(cases) == 0 {
			continue
		}
		bin := filepath.Join(tmp, ex.name+".test")
		log("building %s ...", ex.name)
		if out, err := build(root, ex.name, bin); err != nil {
			log("memreport: build %s: %v\n%s", ex.name, err, out)
			failed = true
			continue
		}
		for _, c := range cases {
			r := &caseResult{key: ex.name + "/" + c}
			results = append(results, r)
			t0 := time.Now()
			log("running %s ...", r.key)
			r.rss, r.err = runCase(root, ex.name, bin, c, memprobe.ModeRSS, "", *verbose)
			if r.err == nil {
				r.prof, r.err = runCase(root, ex.name, bin, c, memprobe.ModeProfile, *profiles, *verbose)
			}
			if r.err != nil {
				failed = true
				log("  FAILED: %v", r.err)
			} else {
				log("  %.1fs", time.Since(t0).Seconds())
			}
		}
	}
	if len(results) == 0 && !failed {
		log("memreport: no case matches -only/-cases")
		os.Exit(2)
	}

	// base is what -update merges into; the report compares with cmp.
	base := &baseline{}
	if b, err := readBaseline(*basePath); err == nil {
		base = b
	} else if !errors.Is(err, os.ErrNotExist) {
		log("memreport: baseline %s: %v (ignored)", *basePath, err)
	}
	cmp := base
	if *noBase {
		cmp = &baseline{}
	}
	flagged := printReport(os.Stdout, results, cmp, *tolerance, *sites)
	fmt.Fprintf(os.Stdout, "\n%d cases in %.0fs\n", len(results), time.Since(start).Seconds())

	if *update {
		if failed {
			log("memreport: not updating the baseline: a case failed")
			os.Exit(1)
		}
		base.update(results, time.Now())
		if err := writeBaseline(*basePath, base); err != nil {
			log("memreport: %v", err)
			os.Exit(1)
		}
		log("baseline written to %s", *basePath)
	}
	if failed || (*strict && flagged > 0) {
		os.Exit(1)
	}
}

func hasExample(name string) bool {
	for _, e := range examples {
		if e.name == name {
			return true
		}
	}
	return false
}

func exampleNames() string {
	var n []string
	for _, e := range examples {
		n = append(n, e.name)
	}
	return strings.Join(n, ", ")
}

// moduleRoot is the directory of the repository's go.mod.
func moduleRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		return "", fmt.Errorf("go env GOMOD: %w", err)
	}
	mod := strings.TrimSpace(string(out))
	if mod == "" || mod == os.DevNull {
		return "", errors.New("run it inside the Tela repository: go run ./internal/testutil/memreport")
	}
	root := filepath.Dir(mod)
	if _, err := os.Stat(filepath.Join(root, "examples", "hello")); err != nil {
		return "", fmt.Errorf("%s is not the Tela repository (no examples/hello)", root)
	}
	return root, nil
}

// build compiles the test binary of an example.
func build(root, name, bin string) ([]byte, error) {
	cmd := exec.Command("go", "test", "-c", "-o", bin, "./examples/"+name)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	return cmd.CombinedOutput()
}

// runCase runs one case in its own process in one mode and returns its
// result. The test binary runs in the example's directory, as go test would.
func runCase(root, name, bin, c, mode, profiles string, verbose bool) (*memprobe.Result, error) {
	parts := strings.Split(c, "/")
	run := "^TestMemory$"
	for _, p := range parts {
		run += "/^" + regexp.QuoteMeta(p) + "$"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-test.run", run, "-test.v", "-test.timeout", "19m")
	cmd.Dir = filepath.Join(root, "examples", name)
	env := append(os.Environ(), memprobe.EnvVar+"="+mode, "CGO_ENABLED=0")
	if mode == memprobe.ModeProfile {
		// The runtime applies it before any package initializes.
		g := "memprofilerate=1"
		if old := os.Getenv("GODEBUG"); old != "" {
			g = old + "," + g
		}
		env = append(env, "GODEBUG="+g)
		if profiles != "" {
			env = append(env, memprobe.ProfilesEnvVar+"="+profiles)
		}
	}
	cmd.Env = env
	var out bytes.Buffer
	var w io.Writer = &out
	if verbose {
		w = io.MultiWriter(&out, os.Stderr)
	}
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s (%s mode): %w\n%s", c, mode, err, tail(out.String(), 40))
	}
	res, err := parseResult(out.String())
	if err != nil {
		return nil, fmt.Errorf("%s (%s mode): %w\n%s", c, mode, err, tail(out.String(), 40))
	}
	return res, nil
}

// parseResult finds the JSON line of the single case a process ran.
func parseResult(out string) (*memprobe.Result, error) {
	var res *memprobe.Result
	for line := range strings.SplitSeq(out, "\n") {
		rest, ok := strings.CutPrefix(line, memprobe.ResultPrefix)
		if !ok {
			continue
		}
		if res != nil {
			return nil, errors.New("more than one result line")
		}
		res = new(memprobe.Result)
		if err := json.Unmarshal([]byte(rest), res); err != nil {
			return nil, fmt.Errorf("bad result line: %w", err)
		}
	}
	if res == nil {
		if strings.Contains(out, "SKIP") || strings.Contains(out, "no tests to run") {
			return nil, errors.New("the case was skipped or not found: is TestMemory in the example's tests with this case name?")
		}
		return nil, errors.New("no result line (" + memprobe.ResultPrefix + "...) in the output")
	}
	return res, nil
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// binDir makes the directory the test binaries are built in. It is not the
// default temporary directory: on many systems /tmp is a tmpfs, and the
// pages of a binary mapped from there count as shared memory (RssShmem)
// instead of file-backed memory (RssFile) in the process's RSS. The user
// cache directory is on disk.
func binDir(dir string) (string, error) {
	if dir != "" {
		return os.MkdirTemp(dir, "memreport-")
	}
	if cache, err := os.UserCacheDir(); err == nil {
		d := filepath.Join(cache, "tela")
		if err := os.MkdirAll(d, 0o755); err == nil {
			return os.MkdirTemp(d, "memreport-")
		}
	}
	return os.MkdirTemp("", "memreport-")
}

// host describes the machine for the baseline's header.
func host() string {
	return fmt.Sprintf("%s/%s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}
