// Command tela is the Tela command line tool.
//
// Commands:
//
//	tela snapshot [--size WxH] [--scale N] [--dark] [--preview NAME] [-o out.png] <pkg>
//	tela inspect  [--aria | --layout] [--size WxH] [--dark] [--preview NAME] <pkg>
//	tela version
//
// Both render a package headlessly: the package defines func Preview()
// tela.Node, or a Previews variable (a tela.Previews map of named screens);
// --preview NAME chooses one of them. snapshot writes a PNG; inspect prints
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
// More commands (lint, dev, add, mcp) arrive in later phases; see
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
