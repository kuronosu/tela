package main

import (
	"bufio"
	"encoding/json"
	"io"
	"sort"
	"strings"
)

// event is one line of `go test -json` (test2json plus the build events of
// go test itself).
type event struct {
	Action      string
	Package     string
	Test        string
	Output      string
	Elapsed     float64
	ImportPath  string
	FailedBuild string
}

// testResult is the outcome of one test or subtest.
type testResult struct {
	Name    string
	Action  string // pass, fail, skip, or "" when it never finished
	Elapsed float64
	Output  []string
}

// pkgResult is the outcome of one package.
type pkgResult struct {
	Name        string
	Action      string // pass, fail, skip
	Elapsed     float64
	Cached      bool
	NoTests     bool
	FailedBuild string
	Output      []string
	Tests       map[string]*testResult
	Order       []string // test names in order of first appearance
}

// testRun is a parsed `go test -json` stream.
type testRun struct {
	Pkgs     map[string]*pkgResult
	PkgOrder []string
	Stray    []string // lines that were not JSON
	buildOut map[string][]string
}

// parseTestJSON reads a `go test -json` stream. Lines that are not JSON are
// kept in Stray; the stream never fails on its content.
func parseTestJSON(r io.Reader) (*testRun, error) {
	run := &testRun{Pkgs: map[string]*pkgResult{}, buildOut: map[string][]string{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil || ev.Action == "" {
			run.Stray = append(run.Stray, line)
			continue
		}
		run.add(ev)
	}
	return run, sc.Err()
}

func (r *testRun) pkg(name string) *pkgResult {
	p := r.Pkgs[name]
	if p == nil {
		p = &pkgResult{Name: name, Tests: map[string]*testResult{}}
		r.Pkgs[name] = p
		r.PkgOrder = append(r.PkgOrder, name)
	}
	return p
}

func (r *testRun) add(ev event) {
	switch ev.Action {
	case "build-output":
		r.buildOut[ev.ImportPath] = append(r.buildOut[ev.ImportPath], splitLines(ev.Output)...)
		return
	case "build-fail":
		return
	}
	if ev.Package == "" {
		return
	}
	p := r.pkg(ev.Package)
	if ev.Test == "" {
		switch ev.Action {
		case "output":
			for _, l := range splitLines(ev.Output) {
				if strings.Contains(l, "(cached)") {
					p.Cached = true
				}
				if strings.Contains(l, "[no test files]") {
					p.NoTests = true
				}
				p.Output = append(p.Output, l)
			}
		case "pass", "fail", "skip":
			p.Action = ev.Action
			p.Elapsed = ev.Elapsed
			if ev.FailedBuild != "" {
				p.FailedBuild = ev.FailedBuild
			}
		}
		return
	}
	t := p.Tests[ev.Test]
	if t == nil {
		t = &testResult{Name: ev.Test}
		p.Tests[ev.Test] = t
		p.Order = append(p.Order, ev.Test)
	}
	switch ev.Action {
	case "output":
		t.Output = append(t.Output, splitLines(ev.Output)...)
	case "pass", "fail", "skip":
		t.Action = ev.Action
		t.Elapsed = ev.Elapsed
	}
}

// splitLines splits an output chunk into lines without their newline.
func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// testFailure is one failing test (or package) of a run.
type testFailure struct {
	Package string
	Test    string // empty for a package-level failure
	Reason  string // "failed", "build failed", "package failed"
	Output  []string
}

// failures lists what failed: the leaf failing tests (a parent whose
// subtest failed is left out, the subtest carries the output), build
// failures, and packages that failed with no failing test (a panic, a
// timeout, an os.Exit).
func (r *testRun) failures() []testFailure {
	var out []testFailure
	for _, name := range r.PkgOrder {
		p := r.Pkgs[name]
		if p.Action != "fail" {
			continue
		}
		if p.FailedBuild != "" {
			out = append(out, testFailure{Package: name, Reason: "build failed", Output: r.buildOut[p.FailedBuild]})
			continue
		}
		n := len(out)
		for _, tn := range p.Order {
			t := p.Tests[tn]
			if t.Action != "fail" || hasFailingChild(p, tn) {
				continue
			}
			out = append(out, testFailure{Package: name, Test: tn, Reason: "failed", Output: testOutput(t.Output)})
		}
		if len(out) > n {
			continue
		}
		detail := testOutput(p.Output)
		var unfinished []string
		for _, tn := range p.Order {
			if p.Tests[tn].Action == "" {
				unfinished = append(unfinished, tn)
			}
		}
		if len(unfinished) > 0 {
			detail = append(detail, "did not finish: "+strings.Join(unfinished, ", "))
		}
		out = append(out, testFailure{Package: name, Reason: "package failed", Output: detail})
	}
	return out
}

func hasFailingChild(p *pkgResult, name string) bool {
	prefix := name + "/"
	for tn, t := range p.Tests {
		if t.Action == "fail" && strings.HasPrefix(tn, prefix) {
			return true
		}
	}
	return false
}

// testOutput drops the "=== RUN"-style markers, which carry no information.
func testOutput(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(l, "=== ") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// slow is one entry of the slowest lists.
type slow struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds"`
}

// slowest returns the n slowest tests and packages that actually ran (not
// served from the test cache and with test files).
func (r *testRun) slowest(n int) (tests, pkgs []slow) {
	for _, name := range r.PkgOrder {
		p := r.Pkgs[name]
		if p.Cached || p.NoTests || (p.Action != "pass" && p.Action != "fail") {
			continue
		}
		pkgs = append(pkgs, slow{Name: name, Seconds: p.Elapsed})
		for _, tn := range p.Order {
			t := p.Tests[tn]
			if t.Action == "" {
				continue
			}
			tests = append(tests, slow{Name: name + " " + tn, Seconds: t.Elapsed})
		}
	}
	return top(tests, n), top(pkgs, n)
}

func top(s []slow, n int) []slow {
	sort.SliceStable(s, func(i, j int) bool { return s[i].Seconds > s[j].Seconds })
	if len(s) > n {
		s = s[:n]
	}
	return s
}

// counts returns the number of packages, how many came from the cache, and
// how many tests finished.
func (r *testRun) counts() (pkgs, cached, tests int) {
	for _, p := range r.Pkgs {
		if p.NoTests {
			continue
		}
		pkgs++
		if p.Cached {
			cached++
		}
		for _, t := range p.Tests {
			if t.Action != "" {
				tests++
			}
		}
	}
	return
}
