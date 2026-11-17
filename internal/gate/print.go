package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// reportData is what the reports found; none of it affects the exit status.
type reportData struct {
	Base     string         `json:"base"` // revision compared against; "" is the working tree only
	Goldens  []goldenChange `json:"goldens_changed"`
	Approval []approvalItem `json:"needs_owner_approval"`
	Error    string         `json:"error,omitempty"`
}

// reports compares the repository against the base: the committed changes
// since the base plus the working tree and the untracked files.
func (g *gate) reports(baseFlag string) reportData {
	rep := reportData{}
	rep.Base = resolveBase(g.git, baseFlag)
	rev := rep.Base
	if rev == "" {
		rev = "HEAD"
	}
	diffText, err := g.gitRaw("diff", "--no-color", "--no-ext-diff", "--no-renames", "--src-prefix=a/", "--dst-prefix=b/", "-U0", rev)
	if err != nil {
		rep.Error = err.Error()
		return rep
	}
	files := parseDiff(diffText)
	if untracked, err := g.gitRaw("ls-files", "-z", "-o", "--exclude-standard"); err == nil {
		for _, p := range strings.Split(untracked, "\x00") {
			if p != "" {
				files = append(files, g.untrackedFile(p))
			}
		}
	}
	rep.Goldens = changedGoldens(files)
	rep.Approval = classify(files, func(name string) bool { return g.treeHas(rev, name) })
	return rep
}

// untrackedFile makes the diff of a file that is entirely new. Only Go
// files under 1 MB get lines: they are the only ones the classifier scans.
func (g *gate) untrackedFile(path string) fileDiff {
	f := fileDiff{Path: path, Status: "A"}
	if !strings.HasSuffix(path, ".go") {
		return f
	}
	full := filepath.Join(g.root, filepath.FromSlash(path))
	if st, err := os.Stat(full); err != nil || st.Size() > 1<<20 {
		return f
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return f
	}
	for i, l := range strings.Split(string(data), "\n") {
		f.Added = append(f.Added, diffLine{N: i + 1, Text: l})
	}
	return f
}

// treeHas reports whether name appears anywhere in the tree of rev.
func (g *gate) treeHas(rev, name string) bool {
	cmd := exec.Command("git", "grep", "-q", "-F", "-e", name, rev)
	cmd.Dir = g.root
	return cmd.Run() == nil
}

func printText(w io.Writer, results []stepResult, rep reportData, total time.Duration, verbose bool) {
	for _, r := range results {
		status := "ok"
		switch {
		case r.Skipped:
			status = "skip"
		case !r.OK:
			status = "FAIL"
		}
		dur := fmt.Sprintf("%.1fs", r.Seconds)
		if r.Skipped {
			dur = ""
		}
		fmt.Fprintf(w, "%-5s %-12s %7s\n", status, r.Name, dur)
		if verbose {
			for _, c := range r.Commands {
				mark := " "
				if !c.OK {
					mark = "!"
				}
				fmt.Fprintf(w, "    %s %6.1fs  %s\n", mark, c.Seconds, c.Label)
			}
		}
	}

	for _, r := range results {
		for _, f := range r.Failures {
			fmt.Fprintf(w, "\nFAIL %s: %s\n", r.Name, f.Title)
			if f.Note != "" {
				fmt.Fprintf(w, "  [%s]\n", f.Note)
			}
			printIndented(w, f.Detail)
		}
	}

	for _, r := range results {
		if z := r.Size; z != nil {
			fmt.Fprintf(w, "\nexamples/hello (linux/amd64): %d / %d bytes\n", z.Bytes, z.Budget)
		}
		if r.Docs != nil {
			fmt.Fprintln(w, "\nWord budgets:")
			for _, d := range r.Docs {
				state := ""
				switch {
				case d.Err != "":
					state = "  error: " + d.Err
				case d.Words > d.Budget:
					state = "  OVER"
				}
				fmt.Fprintf(w, "  %-36s %6d / %-6d words%s\n", d.Label, d.Words, d.Budget, state)
			}
		}
		if t := r.Tests; t != nil {
			fmt.Fprintf(w, "\nTests: %d packages (%d cached), %d tests finished\n", t.Packages, t.Cached, t.Tests)
			printSlow(w, "Slowest tests that ran (not cached):", t.SlowestTests)
			printSlow(w, "Slowest packages that ran (not cached):", t.SlowestPackages)
		}
	}

	if rep.Error != "" {
		fmt.Fprintf(w, "\nReports unavailable: %s\n", rep.Error)
	}
	base := "the working tree only"
	if rep.Base != "" {
		base = "base " + shortRev(rep.Base)
	}
	if len(rep.Goldens) > 0 {
		fmt.Fprintf(w, "\nGoldens changed (%s), look at them:\n", base)
		for _, gc := range rep.Goldens {
			fmt.Fprintf(w, "  %-9s %s\n", gc.Status, gc.Path)
		}
	}
	if len(rep.Approval) > 0 {
		fmt.Fprintf(w, "\nNeeds owner approval (apparatus changed against %s):\n", base)
		for _, it := range collapseApparatus(rep.Approval) {
			fmt.Fprintf(w, "  %-8s %s: %s", it.Change, it.What, it.Where)
			if it.Text != "" {
				fmt.Fprintf(w, "   %s", it.Text)
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintf(w, "\ntotal %.1fs (budget %.0fs)", total.Seconds(), totalBudget.Seconds())
	if total > totalBudget {
		fmt.Fprint(w, "  WARNING: over budget; the time depends on the machine, this is not a failure")
	}
	fmt.Fprintln(w)
	if failedSteps := failedNames(results); len(failedSteps) > 0 {
		fmt.Fprintf(w, "gate FAILED: %s\n", strings.Join(failedSteps, ", "))
	} else {
		fmt.Fprintln(w, "gate passed")
	}
}

func failedNames(results []stepResult) []string {
	var out []string
	for _, r := range results {
		if !r.Skipped && !r.OK {
			out = append(out, r.Name)
		}
	}
	return out
}

func printSlow(w io.Writer, title string, s []slow) {
	if len(s) == 0 {
		return
	}
	fmt.Fprintln(w, title)
	for _, e := range s {
		fmt.Fprintf(w, "  %7.2fs  %s\n", e.Seconds, e.Name)
	}
}

func printIndented(w io.Writer, s string) {
	if s == "" {
		return
	}
	for _, l := range strings.Split(s, "\n") {
		fmt.Fprintf(w, "    %s\n", l)
	}
}

func shortRev(rev string) string {
	if len(rev) > 10 {
		return rev[:10]
	}
	return rev
}

// collapseApparatus folds the files of one apparatus directory with the
// same change into one line when there are more than three, so a new
// directory such as internal/gate does not fill the report. The JSON
// output keeps every file.
func collapseApparatus(items []approvalItem) []approvalItem {
	type key struct{ change, dir string }
	count := map[key]int{}
	dirOf := func(it approvalItem) string {
		if it.What != "apparatus file" {
			return ""
		}
		for _, d := range apparatusDirs {
			if strings.HasPrefix(it.Where, d) {
				return d
			}
		}
		return ""
	}
	for _, it := range items {
		if d := dirOf(it); d != "" {
			count[key{it.Change, d}]++
		}
	}
	var out []approvalItem
	done := map[key]bool{}
	for _, it := range items {
		d := dirOf(it)
		k := key{it.Change, d}
		if d == "" || count[k] <= 3 {
			out = append(out, it)
			continue
		}
		if !done[k] {
			done[k] = true
			out = append(out, approvalItem{Change: it.Change, What: "apparatus files", Where: fmt.Sprintf("%s (%d files)", d, count[k])})
		}
	}
	return out
}
