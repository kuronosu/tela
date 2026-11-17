package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// fileDiff is what changed in one file against the base.
type fileDiff struct {
	Path    string
	Status  string // "A" added, "M" modified, "D" deleted
	Added   []diffLine
	Removed []diffLine
}

// diffLine is a changed line with its line number (in the new file for an
// added line, in the old file for a removed one).
type diffLine struct {
	N    int
	Text string
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// parseDiff parses the output of `git diff -U0 --no-renames`.
func parseDiff(text string) []fileDiff {
	var files []fileDiff
	var cur *fileDiff
	inHunk := false
	oldN, newN := 0, 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			files = append(files, fileDiff{Status: "M", Path: headerPath(strings.TrimPrefix(line, "diff --git "))})
			cur = &files[len(files)-1]
			inHunk = false
			continue
		}
		if cur == nil {
			continue
		}
		if !inHunk {
			switch {
			case strings.HasPrefix(line, "new file mode"):
				cur.Status = "A"
			case strings.HasPrefix(line, "deleted file mode"):
				cur.Status = "D"
			}
		}
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			inHunk = true
			oldN, _ = strconv.Atoi(m[1])
			newN, _ = strconv.Atoi(m[2])
			continue
		}
		if !inHunk {
			continue
		}
		switch {
		case strings.HasPrefix(line, "+"):
			cur.Added = append(cur.Added, diffLine{N: newN, Text: line[1:]})
			newN++
		case strings.HasPrefix(line, "-"):
			cur.Removed = append(cur.Removed, diffLine{N: oldN, Text: line[1:]})
			oldN++
		}
	}
	return files
}

// headerPath reads the path from "diff --git a/p b/p", whatever two-letter
// prefixes git uses (diff.mnemonicPrefix gives "c/" and "w/"). The diff is
// made without renames, so both sides are the same path.
func headerPath(rest string) string {
	n := (len(rest) - 5) / 2
	if n <= 0 || len(rest) != 2*n+5 {
		return ""
	}
	return rest[len(rest)-n:]
}

// goldenChange is a PNG under testdata/ that changed.
type goldenChange struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added, modified, removed
}

func statusWord(s string) string {
	switch s {
	case "A":
		return "added"
	case "D":
		return "removed"
	}
	return "modified"
}

// changedGoldens lists PNG files under any testdata directory that were
// added, modified or removed.
func changedGoldens(files []fileDiff) []goldenChange {
	var out []goldenChange
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".png") {
			continue
		}
		if !strings.HasPrefix(f.Path, "testdata/") && !strings.Contains(f.Path, "/testdata/") {
			continue
		}
		out = append(out, goldenChange{Path: f.Path, Status: statusWord(f.Status)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// approvalItem is a change to the apparatus that the owner should look at.
type approvalItem struct {
	Change string `json:"change"` // added, changed, removed
	What   string `json:"what"`
	Where  string `json:"where"`
	Text   string `json:"text,omitempty"`
}

var (
	envRead    = regexp.MustCompile(`(?:Getenv|LookupEnv)\(\s*"(TELA_[A-Za-z0-9_]*)"`)
	skipCall   = regexp.MustCompile(`\b(?:t|b|tb)\.Skip(?:f|Now)?\b`)
	ignoreLine = regexp.MustCompile(`//\s*(?:lint:ignore|lint:file-ignore|tela:ignore)\b`)
)

// Directories and files whose any change needs approval.
var (
	apparatusDirs  = []string{".github/", ".claude/", "internal/gate/"}
	apparatusFiles = []string{"AGENTS.md", "CLAUDE.md", "staticcheck.conf", "internal/gfx/gpu/compare_test.go"}
)

// classify lists what in the diff needs the owner's approval. baseHas
// reports whether a name appears anywhere in the base tree.
func classify(files []fileDiff, baseHas func(name string) bool) []approvalItem {
	var out []approvalItem
	for _, f := range files {
		change := map[string]string{"A": "added", "D": "removed", "M": "changed"}[f.Status]
		inGate := strings.HasPrefix(f.Path, "internal/gate/")
		switch {
		case hasAnyPrefix(f.Path, apparatusDirs):
			out = append(out, approvalItem{Change: change, What: "apparatus file", Where: f.Path})
		case contains(apparatusFiles, f.Path):
			what := "apparatus file"
			if f.Path == "internal/gfx/gpu/compare_test.go" {
				what = "GPU comparison tolerances"
			}
			out = append(out, approvalItem{Change: change, What: what, Where: f.Path})
		case f.Status == "A" && strings.HasPrefix(f.Path, "docs/adr/"):
			out = append(out, approvalItem{Change: change, What: "new ADR file", Where: f.Path})
		case f.Status == "A" && strings.HasPrefix(f.Path, "docs/") && strings.HasSuffix(f.Path, ".md"):
			out = append(out, approvalItem{Change: change, What: "new doc file", Where: f.Path})
		}
		// The lines of the gate's own files are not scanned: the files are
		// listed as a whole and hold these patterns as test fixtures.
		if inGate || !strings.HasSuffix(f.Path, ".go") {
			continue
		}
		for _, l := range f.Added {
			out = append(out, scanLine(f.Path, l, "added", baseHas)...)
		}
		for _, l := range f.Removed {
			out = append(out, scanLine(f.Path, l, "removed", nil)...)
		}
	}
	return out
}

// scanLine reports the patterns one added or removed line matches.
func scanLine(path string, l diffLine, change string, baseHas func(string) bool) []approvalItem {
	where := path + ":" + strconv.Itoa(l.N)
	text := strings.TrimSpace(l.Text)
	var out []approvalItem
	for _, m := range envRead.FindAllStringSubmatch(l.Text, -1) {
		// An added read of a variable the base already knows is not new
		// apparatus; a removed read is always listed.
		if change == "added" && baseHas != nil && baseHas(m[1]) {
			continue
		}
		out = append(out, approvalItem{Change: change, What: m[1] + " read", Where: where, Text: text})
	}
	if skipCall.MatchString(l.Text) {
		out = append(out, approvalItem{Change: change, What: "test skip", Where: where, Text: text})
	}
	if ignoreLine.MatchString(l.Text) {
		out = append(out, approvalItem{Change: change, What: "ignore directive", Where: where, Text: text})
	}
	return out
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// gitRunner runs git with the arguments and returns its trimmed output.
type gitRunner func(args ...string) (string, error)

// resolveBase picks the revision the reports compare against: the flag
// when given; else the merge base of HEAD and main; when that is HEAD itself
// (working on main) and origin/main exists, the merge base with origin/main.
// It returns "" when nothing applies (only the working tree is compared).
func resolveBase(git gitRunner, flag string) string {
	if flag != "" {
		if rev, err := git("rev-parse", "--verify", "--quiet", flag+"^{commit}"); err == nil {
			return rev
		}
		return ""
	}
	head, err := git("rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return ""
	}
	exists := func(ref string) bool {
		_, err := git("rev-parse", "--verify", "--quiet", ref+"^{commit}")
		return err == nil
	}
	mergeBase := func(ref string) string {
		if !exists(ref) {
			return ""
		}
		mb, err := git("merge-base", "HEAD", ref)
		if err != nil {
			return ""
		}
		return mb
	}
	mb := mergeBase("main")
	if mb == "" || mb == head {
		if o := mergeBase("origin/main"); o != "" {
			return o
		}
	}
	return mb
}
