package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/kuronosu/tela/internal/lint"
)

const lintUsage = `Usage:

	tela lint [-json] [-tests] [-fix] [packages]

Checks the Go source of the packages (default ./...) for the mistakes that
dev mode reports at run time, so they fail in CI with dev mode off. Packages
are written as for "go build" and need the go command in PATH. Only
constant strings are checked (a literal, a const, a concatenation of
constants); anything computed is skipped, never guessed.

Rules, each with the message and the "did you mean" of dev mode:

	class   the classes of h.Class and h.Exit, of the Class fields of kit
	        components and of the recipe fields of kit.Style, of
	        tela.App.RootClass and telatest.RootClass, and the values of
	        ClassAliases: unknown, unsupported or invalid classes. The
	        aliases (tela.App.ClassAliases, telatest.ClassAliases) and the
	        theme colors (the constant keys of map[string]tela.SchemeColor
	        literals and assignments, such as tela.Theme.Colors, and the
	        constant arguments of calls to a function that stores a color
	        under a name parameter, set("gold", ...)) declared in the linted
	        packages are known
	role    h.Role with an unknown ARIA role
	aria    h.AriaCurrent, h.AriaSort, h.AriaHasPopup, h.AriaOrientation and
	        h.AriaAutoComplete with a value outside their tokens
	prop    h.Dir, h.Type (and the Type of kit.Button and kit.Input) and
	        tela.Ease with a value they do not take
	key     two siblings with the same constant h.Key; the items of an
	        h.Map or h.MapIndex without a key
	hooks   a hook called conditionally, in a loop, after an early return,
	        from a function that is neither a component nor a custom hook, or
	        in a function literal that is not a component; a component that
	        uses hooks called directly instead of through tela.C or tela.CP
	fragment   h.Fragment of a single child: a struct component is a
	        tela.Node, so write the node itself (ADR-205). Reported, with a
	        fix for -fix, only where removing the wrapper type-checks and
	        keeps the child's identity: the child is not a Prop, the place
	        takes a tela.Node or tela.Child (not a short variable
	        declaration, an interface{} or a generic parameter), no comment
	        is lost, the first argument of tela.WithKey is a struct literal,
	        and the function (or IfElse) has no Fragment of several
	        children as an alternative, which would give the child another
	        identity and reset its state
	prop-node  a Prop (h.Class, h.Key, h.OnClick, tela.Width...) where a
	        tela.Node is expected: the result of a function or of a Map
	        callback, an element of a []tela.Node, a Node field, an
	        argument of tela.WithKey. It compiles, because a Prop has a Build
	        method, and panics when the node is consumed; no fix

Each finding is printed as file:line:col: rule: message, sorted by position,
with paths relative to the current directory. Exit status: 0 when nothing is
reported, 1 when something is, 2 on a usage error or when a package cannot be
loaded (a package that does not type-check is reported and skipped).

A comment //tela:ignore rule on the line of a finding, or alone on the line
above, silences it; several rules are separated by commas (spaces allowed),
a reason may follow after --, and without rules it covers every rule:

	//tela:ignore class -- tests the dev-mode error
	Div(Class("rounded-xxl"))

Limits: files for other operating systems or build tags are not read (the
build constraints of the current platform apply); aliases and theme colors
are collected from the linted packages only, so lint ./... when you use them;
a theme color whose name is built at run time is unknown, and a class that
uses it is reported.

Flags:

	-json    print the findings as a JSON array of objects with file, line,
	         col, rule, severity ("error", or "warning" for what dev mode only
	         warns about), message and, when there is one, suggestion
	-tests   lint the _test.go files too
	-fix     apply the fixes of the findings that have one (the fragment
	         rule), rewriting the files (gofmt-formatted; a file whose fixed
	         text would not parse is left as it was), and report what is
	         left. Review the result with git diff; no other rule has a fix
`

// lintOptions are the parsed arguments of `tela lint`.
type lintOptions struct {
	JSON     bool
	Tests    bool
	Fix      bool
	Patterns []string
}

// parseLintArgs parses the arguments of `tela lint`. Flags and package
// patterns may come in any order.
func parseLintArgs(args []string) (lintOptions, error) {
	var opts lintOptions
	fs := flag.NewFlagSet("tela lint", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.JSON, "json", false, "")
	fs.BoolVar(&opts.Tests, "tests", false, "")
	fs.BoolVar(&opts.Fix, "fix", false, "")
	pats, err := parsePositionals("lint", fs, args)
	opts.Patterns = pats
	return opts, err
}

// runLint implements `tela lint`.
func runLint(args []string, stdout, stderr io.Writer) int {
	opts, err := parseLintArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, lintUsage)
			return 0
		}
		fmt.Fprintf(stderr, "tela lint: %v\n\n%s", err, lintUsage)
		return 2
	}
	res, err := lint.Run(lint.Options{Patterns: opts.Patterns, Tests: opts.Tests, Fix: opts.Fix})
	if err != nil {
		fmt.Fprintf(stderr, "tela lint: %v\n", err)
		return 2
	}
	if opts.JSON {
		if err := lint.WriteJSON(stdout, res.Diagnostics); err != nil {
			fmt.Fprintf(stderr, "tela lint: %v\n", err)
			return 2
		}
	} else {
		lint.WriteText(stdout, res.Diagnostics)
	}
	if opts.Fix {
		if n := len(res.Fixed); n > 0 {
			fmt.Fprintf(stderr, "tela lint: fixed %d findings in %d files\n", n, res.FixedFiles)
		}
		if res.FixError != nil {
			fmt.Fprintf(stderr, "tela lint: %v\n", res.FixError)
		}
	}
	for _, e := range res.LoadErrors {
		fmt.Fprintf(stderr, "tela lint: %s\n", strings.TrimSpace(e))
	}
	switch {
	case len(res.LoadErrors) > 0, res.FixError != nil:
		return 2
	case len(res.Diagnostics) > 0:
		return 1
	}
	return 0
}
