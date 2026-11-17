// Command gate is the repository's verification gate: one program that runs
// the checks agents, the owner and CI's lint job would otherwise run as
// separate commands (format, vet, build, staticcheck, tela lint, tests, the
// word budgets of the docs), so local checks and CI cannot drift. Run it
// from the repository root:
//
//	go run ./internal/gate [flags]
//
// Every step runs even when another fails. It exits 0 when all passed, 1
// when a step failed, 2 on a usage error. Besides the steps it prints
// reports that never affect the exit status: PNG goldens that changed, and
// changes to the apparatus (CI, agent configuration, the gate, skips,
// ignore directives, new TELA_* variables) that the owner approves.
//
// It uses the standard library only and does not import any package of the
// repository: it runs cmd/tela as a subprocess (ARCHITECTURE §5).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// totalBudget is the wall time the whole gate should take. Going over is a
// warning, not a failure: the time depends on the machine.
const totalBudget = 60 * time.Second

// stepNames lists the steps in the order they are reported.
var stepNames = []string{"fmt", "vet", "build", "gpu-deps", "size", "staticcheck", "lint", "test", "docs"}

type options struct {
	tests   bool
	only    string
	skip    string
	json    bool
	verbose bool
	base    string
}

func main() { os.Exit(realMain(os.Args[1:])) }

func realMain(args []string) int {
	var o options
	fs := flag.NewFlagSet("gate", flag.ContinueOnError)
	fs.BoolVar(&o.tests, "tests", true, "run the test step (CI's lint job has separate test jobs)")
	fs.StringVar(&o.only, "only", "", "run only these steps (comma-separated: "+strings.Join(stepNames, ",")+")")
	fs.StringVar(&o.skip, "skip", "", "skip these steps (comma-separated)")
	fs.BoolVar(&o.json, "json", false, "print one JSON object instead of text")
	fs.BoolVar(&o.verbose, "v", false, "print every command with its time")
	fs.StringVar(&o.base, "base", "", "revision the reports compare against (default: merge base of HEAD and main)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if os.Getenv("TELA_UPDATE") != "" {
		fmt.Fprintln(os.Stderr, "gate: TELA_UPDATE is set. Regenerating goldens is not a fix: regenerate them separately (TELA_UPDATE=1 go test ./path), look at the diff, then run the gate without it.")
		return 2
	}
	enabled, err := selectSteps(o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gate:", err)
		return 2
	}
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gate:", err)
		return 2
	}
	g := newGate(root)

	start := time.Now()
	results := g.runSteps(enabled)
	rep := g.reports(o.base)
	total := time.Since(start)

	failed := false
	for _, r := range results {
		if !r.Skipped && !r.OK {
			failed = true
		}
	}
	if o.json {
		printJSON(results, rep, total)
	} else {
		printText(os.Stdout, results, rep, total, o.verbose)
	}
	if failed {
		return 1
	}
	return 0
}

// selectSteps applies -tests, -only and -skip, and returns the enabled
// steps by name.
func selectSteps(o options) (map[string]bool, error) {
	known := map[string]bool{}
	for _, n := range stepNames {
		known[n] = true
	}
	parse := func(flagName, list string) (map[string]bool, error) {
		set := map[string]bool{}
		for _, n := range strings.Split(list, ",") {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if !known[n] {
				return nil, fmt.Errorf("-%s: unknown step %q; steps are %s", flagName, n, strings.Join(stepNames, ", "))
			}
			set[n] = true
		}
		return set, nil
	}
	only, err := parse("only", o.only)
	if err != nil {
		return nil, err
	}
	skip, err := parse("skip", o.skip)
	if err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, n := range stepNames {
		on := len(only) == 0 || only[n]
		if skip[n] || (n == "test" && !o.tests) {
			on = false
		}
		enabled[n] = on
	}
	return enabled, nil
}

// repoRoot returns the top-level directory of the repository the gate is
// run in.
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("run the gate inside the repository (git rev-parse --show-toplevel failed: %v)", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// failure is one reason a step failed.
type failure struct {
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Note   string `json:"note,omitempty"` // e.g. "named in docs/ROADMAP.md:67 (a known flake?)"
}

// commandInfo is the time of one subprocess of a step.
type commandInfo struct {
	Label   string  `json:"label"`
	Seconds float64 `json:"seconds"`
	OK      bool    `json:"ok"`
}

// testSummary is what the test step reports besides failures.
type testSummary struct {
	Packages        int    `json:"packages"`
	Cached          int    `json:"cached"`
	Tests           int    `json:"tests"`
	SlowestTests    []slow `json:"slowest_tests,omitempty"`
	SlowestPackages []slow `json:"slowest_packages,omitempty"`
}

// stepResult is the outcome of one step.
type stepResult struct {
	Name     string        `json:"name"`
	OK       bool          `json:"ok"`
	Skipped  bool          `json:"skipped,omitempty"`
	Seconds  float64       `json:"seconds"`
	Failures []failure     `json:"failures,omitempty"`
	Commands []commandInfo `json:"commands,omitempty"`
	Docs     []docCount    `json:"docs,omitempty"`
	Tests    *testSummary  `json:"tests,omitempty"`
	Size     *sizeInfo     `json:"size,omitempty"`
}

// sizeInfo is the size step's measure of the examples/hello binary.
type sizeInfo struct {
	Bytes  int64 `json:"bytes"`
	Budget int64 `json:"budget"`
}

// runSteps runs the enabled steps concurrently and returns their results
// in report order. The test step is started first: it is the longest.
func (g *gate) runSteps(enabled map[string]bool) []stepResult {
	impl := map[string]func() stepResult{
		"fmt":         g.fmtStep,
		"vet":         g.vetStep,
		"build":       g.buildStep,
		"gpu-deps":    g.gpuDepsStep,
		"size":        g.sizeStep,
		"staticcheck": g.staticcheckStep,
		"lint":        g.lintStep,
		"test":        g.testStep,
		"docs":        g.docsStep,
	}
	results := make([]stepResult, len(stepNames))
	var wg sync.WaitGroup
	launch := func(i int) {
		name := stepNames[i]
		if !enabled[name] {
			results[i] = stepResult{Name: name, Skipped: true, OK: true}
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			r := impl[name]()
			r.Name = name
			r.Seconds = time.Since(start).Seconds()
			r.OK = len(r.Failures) == 0
			results[i] = r
		}()
	}
	for i := len(stepNames) - 1; i >= 0; i-- { // test (and docs) first
		if stepNames[i] == "test" {
			launch(i)
		}
	}
	for i, n := range stepNames {
		if n != "test" {
			launch(i)
		}
	}
	wg.Wait()
	return results
}

// jsonReport is the -json output.
type jsonReport struct {
	OK            bool         `json:"ok"`
	Seconds       float64      `json:"seconds"`
	BudgetSeconds float64      `json:"budget_seconds"`
	OverBudget    bool         `json:"over_budget"`
	Steps         []stepResult `json:"steps"`
	Reports       reportData   `json:"reports"`
}

func printJSON(results []stepResult, rep reportData, total time.Duration) {
	j := jsonReport{
		OK:            true,
		Seconds:       total.Seconds(),
		BudgetSeconds: totalBudget.Seconds(),
		OverBudget:    total > totalBudget,
		Steps:         results,
		Reports:       rep,
	}
	for _, r := range results {
		if !r.OK {
			j.OK = false
		}
	}
	if j.Reports.Goldens == nil {
		j.Reports.Goldens = []goldenChange{}
	}
	if j.Reports.Approval == nil {
		j.Reports.Approval = []approvalItem{}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(j)
}
