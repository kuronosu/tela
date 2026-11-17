package main

import "strings"

// findFlake looks for a failing test in the roadmap, the file where known
// flaky tests are recorded. The test is searched by name in backticks:
// the full path of a subtest first, then each parent up to the top-level
// name ("TestDebounce/native/x", "TestDebounce/native", "TestDebounce").
// It returns the 1-based line of the first mention.
func findFlake(roadmap []string, test string) (line int, ok bool) {
	if test == "" {
		return 0, false
	}
	for cand := test; cand != ""; {
		needle := "`" + cand + "`"
		for i, l := range roadmap {
			if strings.Contains(l, needle) {
				return i + 1, true
			}
		}
		i := strings.LastIndexByte(cand, '/')
		if i < 0 {
			break
		}
		cand = cand[:i]
	}
	return 0, false
}
