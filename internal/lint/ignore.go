package lint

import (
	"os"
	"strings"
)

// Suppression: a comment `//tela:ignore rule[,rule...]` on the line of a
// finding, or alone on the line above it, silences the findings of those
// rules there. Text after the rules is a reason and is ignored; the name
// "all", or no name, covers every rule.
//
//	//tela:ignore class -- the test checks that this typo is reported
//	Div(Class("rounded-xxl"))
const ignoreDirective = "tela:ignore"

// ignoreSet holds the rules ignored on each line of a file.
type ignoreSet map[int]map[string]bool

// ignored reports whether a finding of rule at line of file is silenced.
func (r *run) ignored(file string, line int, rule string) bool {
	set, ok := r.ignores[file]
	if !ok {
		set = r.buildIgnores(file)
		if r.ignores == nil {
			r.ignores = map[string]ignoreSet{}
		}
		r.ignores[file] = set
	}
	rules := set[line]
	return rules != nil && (rules[rule] || rules["all"])
}

// buildIgnores scans the comments of a file for ignore directives.
func (r *run) buildIgnores(file string) ignoreSet {
	f := r.fileAST[file]
	if f == nil {
		return nil
	}
	var set ignoreSet
	var src []byte
	var srcRead bool
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			rules, ok := parseIgnore(c.Text)
			if !ok {
				continue
			}
			if !srcRead {
				src, _ = os.ReadFile(file)
				srcRead = true
			}
			pos := r.l.fset.Position(c.Pos())
			if set == nil {
				set = ignoreSet{}
			}
			add := func(line int) {
				if set[line] == nil {
					set[line] = map[string]bool{}
				}
				for _, rule := range rules {
					set[line][rule] = true
				}
			}
			add(pos.Line)
			if aloneOnLine(src, pos.Offset) {
				add(pos.Line + 1)
			}
		}
	}
	return set
}

// parseIgnore returns the rules of an ignore comment.
func parseIgnore(text string) ([]string, bool) {
	text, ok := strings.CutPrefix(text, "//")
	if !ok {
		return nil, false
	}
	text, ok = strings.CutPrefix(strings.TrimLeft(text, " \t"), ignoreDirective)
	if !ok || (text != "" && text[0] != ' ' && text[0] != '\t') {
		return nil, false
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return []string{"all"}, true
	}
	var rules []string
	for _, rule := range strings.Split(fields[0], ",") {
		if rule = strings.TrimSpace(rule); rule != "" {
			rules = append(rules, rule)
		}
	}
	if len(rules) == 0 {
		return []string{"all"}, true
	}
	return rules, true
}

// aloneOnLine reports whether only white space comes before offset on its
// line.
func aloneOnLine(src []byte, offset int) bool {
	if offset > len(src) {
		return false
	}
	for i := offset - 1; i >= 0 && src[i] != '\n'; i-- {
		if src[i] != ' ' && src[i] != '\t' {
			return false
		}
	}
	return true
}
