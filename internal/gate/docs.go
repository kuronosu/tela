package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var sectionHeading = regexp.MustCompile(`^## (\d+)\. `)

// docBudget is a word budget for a file or for some of its sections.
type docBudget struct {
	Label    string // shown in the report
	File     string // slash-separated, relative to the repository root
	Sections []int  // numbered "## N." sections counted; empty means the whole file
	Budget   int
}

// docBudgets is the table of word budgets. To budget another file, add a
// row. Words are counted like `wc -w`.
var docBudgets = []docBudget{
	{Label: "AGENTS.md", File: "AGENTS.md", Budget: 1200},
	{Label: "docs/ARCHITECTURE.md", File: "docs/ARCHITECTURE.md", Budget: 10500},
	{Label: "docs/ARCHITECTURE.md §1-§5, §19", File: "docs/ARCHITECTURE.md", Sections: []int{1, 2, 3, 4, 5, 19}, Budget: 2000},
}

// docCount is the result of counting one budget.
type docCount struct {
	Label  string `json:"label"`
	Words  int    `json:"words"`
	Budget int    `json:"budget"`
	Err    string `json:"error,omitempty"`
}

// wordCount counts words like `wc -w`.
func wordCount(s string) int { return len(strings.Fields(s)) }

// sectionWords counts the words of the numbered sections ("## 5. Layers")
// of a document, each running up to the next "## " heading.
func sectionWords(text string, nums []int) int {
	want := map[int]bool{}
	for _, n := range nums {
		want[n] = true
	}
	words, in := 0, false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = false
			if m := sectionHeading.FindStringSubmatch(line); m != nil {
				n, _ := strconv.Atoi(m[1])
				in = want[n]
			}
		}
		if in {
			words += len(strings.Fields(line))
		}
	}
	return words
}

// checkDocs counts every budget of the table under root.
func checkDocs(root string, table []docBudget) []docCount {
	out := make([]docCount, 0, len(table))
	for _, b := range table {
		c := docCount{Label: b.Label, Budget: b.Budget}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(b.File)))
		switch {
		case err != nil:
			c.Err = err.Error()
		case len(b.Sections) > 0:
			c.Words = sectionWords(string(data), b.Sections)
		default:
			c.Words = wordCount(string(data))
		}
		out = append(out, c)
	}
	return out
}
