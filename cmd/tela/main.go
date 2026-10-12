// Command tela is the Tela command line tool.
//
// Commands:
//
//	tela snapshot [--size WxH] [--scale N] [-o out.png] <pkg>
//
// More commands (inspect, lint, dev, add, mcp) arrive in later phases; see
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

	snapshot    render a package's scene headlessly to a PNG
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
	case "help", "-h", "-help", "--help":
		if len(args) > 1 {
			switch args[1] {
			case "snapshot":
				fmt.Fprint(stdout, snapshotUsage)
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
