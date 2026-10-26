// Command tela is the Tela command line tool.
//
// Commands:
//
//	tela snapshot [--size WxH] [--scale N] [--dark] [--preview NAME] [-o out.png] <pkg>
//	tela inspect  [--aria | --layout] [--size WxH] [--dark] [--preview NAME] <pkg>
//	tela lint     [-json] [-tests] [packages]
//	tela dev      [--size WxH] [--scale N] [--dark] [--preview NAME] [--no-state] [-v] [pkg]
//	tela icons gen -pkg <name> [-o outdir] [flags] <dir>
//	tela new      [-module path] [-skip-deps] <dir>
//	tela version
//
// Both render a package headlessly: the package defines func Preview()
// tela.Node (or tela.Component), or a Previews variable (a tela.Previews
// map of named tela.Preview screens, each with a Root and an optional Wrap);
// --preview NAME chooses one of them. A package-level App (a tela.App or a
// *tela.App; a variable App of another type is ignored) sets
// the size, root class, color scheme, theme and class aliases of the render,
// so the image matches the window; alone, its Root is the screen. The dev
// warnings of the screen go to standard error. snapshot writes a PNG; inspect prints
// the aria snapshot as YAML (--aria, the default) or the layout tree as
// JSON (--layout). The phase-1
// contract func Scene(w, h float32) *gfx.DisplayList still works for
// snapshot.
//
// The commands run the package's own code: they generate a test file that
// the go command compiles into the package (through an overlay, so the
// directory is not touched), which means they need the go command in PATH
// and a package in a module that requires Tela (the generated code uses
// the public telatest entry point, ADR-059; only the legacy Scene contract
// needs the Tela module itself).
//
// icons gen turns a directory of SVG files into a Go package with one
// tela.Icon variable per file (ADR-102); it does not run the package's code
// and needs neither the go command nor a module.
//
// lint checks the Go source of the packages (default ./...) for the mistakes
// that dev mode reports at run time: unknown classes, roles and aria tokens,
// duplicate or missing keys, misused hooks. It type-checks the source with
// the standard library and reuses the runtime's own checks (ADR-173); it
// does not run the code, and needs the go command in PATH. It prints
// file:line:col: rule: message and exits 1 when it reports something.
//
// dev runs a package's app as a child process under a window it owns,
// rebuilds it with -tags teladev when its sources change and swaps the
// running process for the new build, keeping the state of its components
// (ADR-111, ADR-115). Like snapshot, a package that is not a package main
// runs its Preview, Previews or App.
//
// new creates an app in a new directory: a go.mod that requires Tela, a
// counter in main.go, a telatest test, a README and a .gitignore (ADR-128).
// It runs go get and go mod tidy unless -skip-deps is given.
//
// More commands (add, mcp) arrive in later phases; see
// docs/ARCHITECTURE.md section 17.
package main

import (
	"fmt"
	"io"
	"os"
)

const usage = `tela is the Tela command line tool.

Usage:

	tela <command> [arguments]

Commands:

	snapshot    render a package's screen headlessly to a PNG
	inspect     print a package's aria snapshot (YAML) or layout tree (JSON)
	lint        check the source for the mistakes dev mode reports at run time
	dev         run an app, rebuilding and reloading it as its sources change
	new         create a new Tela app in a directory
	icons       generate a Go package of tela.Icon variables from SVG files
	version     print the tela version
	help        show this help

Run "tela help <command>" for details about a command.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the command line args (without the program name) and returns
// the process exit code: 0 on success, 1 on failure, 2 on a usage error.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "snapshot":
		return runSnapshot(args[1:], stdout, stderr)
	case "inspect":
		return runInspect(args[1:], stdout, stderr)
	case "icons":
		return runIcons(args[1:], stdout, stderr)
	case "lint":
		return runLint(args[1:], stdout, stderr)
	case "dev":
		return runDev(args[1:], stdout, stderr)
	case "new":
		return runNew(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		if len(args) > 1 {
			switch args[1] {
			case "snapshot":
				fmt.Fprint(stdout, snapshotUsage)
				return 0
			case "inspect":
				fmt.Fprint(stdout, inspectUsage)
				return 0
			case "dev":
				fmt.Fprint(stdout, devUsage)
				return 0
			case "lint":
				fmt.Fprint(stdout, lintUsage)
				return 0
			case "icons":
				if len(args) > 2 && args[2] == "gen" {
					fmt.Fprint(stdout, iconsGenUsage)
					return 0
				}
				fmt.Fprint(stdout, iconsUsage)
				return 0
			case "new":
				fmt.Fprint(stdout, newUsage)
				return 0
			case "version":
				fmt.Fprint(stdout, versionUsage)
				return 0
			default:
				fmt.Fprintf(stderr, "tela: unknown help topic %q; run \"tela help\" for the list of commands\n", args[1])
				return 2
			}
		}
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "tela: unknown command %q\n\n%s", args[0], usage)
	return 2
}
