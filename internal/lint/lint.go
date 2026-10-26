// Package lint is the engine of `tela lint` (ADR-173): it checks Go source
// statically for the mistakes that dev mode reports at run time, so that
// they fail in CI with dev mode off.
//
// Every rule calls the runtime's own check: the class parser and its
// suggestions (internal/style), the role list (internal/semantics) and the
// token checks of the props (internal/core), so the messages are the ones
// dev mode prints and there is one source of truth for what is valid.
// Packages are type-checked with the standard library (go/parser and
// go/types), found with `go list`; no dependency outside it is needed.
//
// A value is checked when it is a constant: a string literal, a const, a
// concatenation of constants. Anything computed is skipped, never guessed.
// A package that fails to type-check is reported and skipped.
//
// The rules (the names `//tela:ignore` takes):
//
//	class  a constant class string with an unknown, unsupported or invalid class
//	role   h.Role with an unknown ARIA role
//	aria   an Aria prop with a value outside its fixed set of tokens
//	prop   h.Dir, h.Type and tela.Ease with a value they do not take
//	key    duplicate keys among siblings; a Map without keys
//	hooks  React's rules of hooks, as dev mode applies them
//
// This package is used only by cmd/tela.
package lint

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// The rules.
const (
	RuleClass = "class"
	RuleRole  = "role"
	RuleAria  = "aria"
	RuleProp  = "prop"
	RuleKey   = "key"
	RuleHooks = "hooks"
)

// Rules lists the rule names.
var Rules = []string{RuleClass, RuleRole, RuleAria, RuleProp, RuleKey, RuleHooks}

// Severities of a Diagnostic.
const (
	// SeverityError is a mistake dev mode reports as an error or a panic.
	SeverityError = "error"
	// SeverityWarning is a likely mistake that dev mode only warns about.
	SeverityWarning = "warning"
)

// Diagnostic is one finding.
type Diagnostic struct {
	// File is the path of the file, relative to the working directory when
	// it is inside it.
	File string `json:"file"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
	Rule string `json:"rule"`
	// Severity is "error", or "warning" for what dev mode only warns about.
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message"`
	// Suggestion is the replacement the message proposes, when it does.
	Suggestion string `json:"suggestion,omitempty"`
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s", d.File, d.Line, d.Col, d.Rule, d.Message)
}

// Options say what to lint.
type Options struct {
	// Dir is the directory the patterns are relative to; empty is the
	// working directory.
	Dir string
	// Patterns are package patterns as `go list` takes them; empty is
	// "./...".
	Patterns []string
	// Tests also lints the _test.go files.
	Tests bool
}

// Result is the outcome of a Run.
type Result struct {
	// Diagnostics are the findings, sorted by position.
	Diagnostics []Diagnostic
	// LoadErrors are the packages that could not be loaded or
	// type-checked, one message each; they were skipped.
	LoadErrors []string
	// Packages is the number of packages linted.
	Packages int
}

// Run lints the packages the options name. It returns an error when no
// package can be listed (a bad pattern, no go command); problems in single
// packages are in Result.LoadErrors.
func Run(opts Options) (res *Result, err error) {
	defer func() {
		if v := recover(); v != nil {
			res, err = nil, fmt.Errorf("internal error: %v (please report it)", v)
		}
	}()
	dir := opts.Dir
	if dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		dir = wd
	}
	patterns := opts.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	r := &run{opts: opts, dir: dir}
	return r.run(patterns)
}

// WriteText prints the diagnostics one per line, as go vet does.
func WriteText(w io.Writer, diags []Diagnostic) {
	for _, d := range diags {
		fmt.Fprintln(w, d.String())
	}
}

// WriteJSON prints the diagnostics as a JSON array (an empty one is []).
func WriteJSON(w io.Writer, diags []Diagnostic) error {
	if diags == nil {
		diags = []Diagnostic{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(diags)
}

func sortDiagnostics(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		a, b := ds[i], ds[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
}

// suggestionOf returns the replacement a dev-mode message proposes
// ("...; did you mean \"x\"?"), or "".
func suggestionOf(msg string) string {
	const marker = `did you mean "`
	i := strings.Index(msg, marker)
	if i < 0 {
		return ""
	}
	rest := msg[i+len(marker):]
	if j := strings.Index(rest, `"?`); j >= 0 {
		return rest[:j]
	}
	return ""
}
