// Command tela is the Tela command line tool.
//
// Commands:
//
//	tela snapshot [--size WxH] [--scale N] [--dark] [--preview NAME] [-o out.png] <pkg>
//	tela inspect  [--aria | --layout] [--size WxH] [--dark] [--preview NAME] <pkg>
//	tela lint     [-json] [-tests] [-fix] [packages]
//	tela check    [--preview NAME] [--size WxH] [--rules LIST] [--disable LIST] [-json] <pkg>
//	tela verify   [--preview NAME] [--size WxH] [--widths LIST] [--text-scale N] [--lang TAG] [-json] <pkg>
//	tela dev      [--size WxH] [--scale N] [--dark] [--preview NAME] [--no-state] [--devtools] [-v] [pkg]
//	tela devtools [-addr ADDRESS]
//	tela mcp      [--size WxH] [--scale N] [--dark] [--preview NAME] [-v] [pkg]
//	tela lsp      [-v]
//	tela build    [-o path] [-id ID] [-name NAME] [-version V] [-icon file] [-console] [-tags list] [-ldflags f] [-gcflags f] [-trimpath] [-v] [pkg]
//	tela run      [-id ID] [-name NAME] [-version V] [-tags list] [-ldflags f] [-gcflags f] [-trimpath] [-v] [pkg [args...]]
//	tela package  [-format list] [-os list] [-arch list] [-o dir] [-version V] [-icon file] [-sign] [-sign-cmd cmd] [build flags] [pkg]
//	tela icons gen -pkg <name> [-o outdir] [flags] <dir>
//	tela new      [-module path] [-skip-deps] <dir>
//	tela update   keygen|sign|manifest [flags]
//	tela version
//
// snapshot and inspect render a package headlessly: the package defines func Preview()
// tela.Node (built as a component of its own), or a Previews variable (a
// tela.Previews map of named tela.Preview screens, each with a Root and an
// optional Wrap);
// --preview NAME chooses one of them. A package-level App (a tela.App or a
// *tela.App; a variable App of another type is ignored) sets
// the size, root class, color scheme, theme and class aliases of the render,
// so the image matches the window; alone, its Root is the screen. The dev
// warnings of the screen go to standard error. snapshot writes a PNG; inspect prints
// the aria snapshot as YAML (--aria, the default) or the layout tree as
// JSON (--layout).
//
// The commands run the package's own code: they generate a test file that
// the go command compiles into the package (through an overlay, so the
// directory is not touched), which means they need the go command in PATH
// and a package in a module that requires Tela (the generated code uses
// the public telatest entry point, ADR-059).
//
// icons gen turns a directory of SVG files into a Go package with one
// tela.Icon variable per file (ADR-102); it does not run the package's code
// and needs neither the go command nor a module.
//
// check and verify mount a package's screens headlessly as snapshot does and
// run telatest's accessibility audit and layout rules on them (ADR-306,
// ADR-312): check the default preview in light and dark, verify every
// preview in a matrix of widths, large text, right-to-left and another
// language. Each finding names its rule, the element's aria line, its source
// line and the fix; the exit status is 1 when there is one.
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
// devtools opens the DevTools window of a running tela dev session: the
// element tree with a pick tool and why each element rendered, the
// accessibility tree, a frame profiler and the log (ADR-299); tela dev
// --devtools starts it beside the app.
//
// mcp is a Model Context Protocol server on standard input and output
// (ADR-174): an agent drives the package's screen, mounted headlessly with
// telatest in a process that stays up between calls, through the tools
// snapshot (the aria snapshot with [ref=eN] marks), screenshot, click,
// hover, fill, press, scroll, drag, resize, open, wait, inspect and lint.
// Before each call it rebuilds and remounts when the sources changed, and a
// compile error is that call's result while the last good build keeps
// answering.
//
// lsp is a Language Server Protocol server on standard input and output, for
// an editor to run beside gopls on the Go files of a Tela app: the findings
// of lint for the open buffers as diagnostics, and completion and hover for
// the Tailwind classes in class strings, with the CSS each one produces.
//
// build builds the program of a package main for the environment's GOOS and
// GOARCH. For darwin it writes a macOS application bundle, <Name>.app, whose
// Info.plist names the app's ID (read from the package's tela.App, or -id);
// for windows it writes the executable as a GUI program with the app's icon
// and version information (ADR-294); elsewhere it writes the plain
// executable. run builds the same bundle in
// the user cache folder, under a folder named by the app's ID, and runs the
// executable inside it with the terminal attached (ADR-224, ADR-227).
//
// package builds the program for each target (-os and -arch lists) and
// writes its packages from the app's declaration: on Linux an AppImage on
// Tela's own runtime, a deb and a flatpak manifest, with the same bytes for
// the same inputs (ADR-295).
//
// new creates an app in a new directory: a go.mod that requires Tela, a
// counter in main.go, a telatest test, a README and a .gitignore (ADR-128).
// It runs go get and go mod tidy unless -skip-deps is given.
//
// tela add is deferred (docs/ROADMAP.md, Deferred); see docs/ARCHITECTURE.md
// section 17.
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
	check       find cut-off text, overlaps, low contrast, small targets and focus problems on a package's screen
	verify      the same over every preview, in light and dark, at several widths, with large text, in RTL and in another language
	dev         run an app, rebuilding and reloading it as its sources change
	devtools    open the DevTools of a running tela dev session
	mcp         serve a package's screen to agents (Model Context Protocol)
	lsp         serve class diagnostics, completion and hover to editors (Language Server Protocol)
	build       build a program; on macOS, as an application bundle
	run         build a program and run it; on macOS, as a bundle
	package     build and package a program for each target (AppImage, deb, flatpak)
	new         create a new Tela app in a directory
	icons       generate a Go package of tela.Icon variables from SVG files
	update      make signing keys, update manifests and their signatures
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
	case "check":
		return runCheck(false, args[1:], stdout, stderr)
	case "verify":
		return runCheck(true, args[1:], stdout, stderr)
	case "dev":
		return runDev(args[1:], stdout, stderr)
	case "devtools":
		return runDevtools(args[1:], stdout, stderr)
	case "mcp":
		return runMCP(args[1:], stdout, stderr)
	case "lsp":
		return runLSP(args[1:], stdout, stderr)
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	case "package":
		return runPackage(args[1:], stdout, stderr)
	case "new":
		return runNew(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
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
			case "devtools":
				fmt.Fprint(stdout, devtoolsUsage)
				return 0
			case "mcp":
				fmt.Fprint(stdout, mcpUsage)
				return 0
			case "lint":
				fmt.Fprint(stdout, lintUsage)
				return 0
			case "check":
				fmt.Fprint(stdout, checkUsage)
				return 0
			case "verify":
				fmt.Fprint(stdout, verifyUsage)
				return 0
			case "lsp":
				fmt.Fprint(stdout, lspUsage)
				return 0
			case "icons":
				if len(args) > 2 && args[2] == "gen" {
					fmt.Fprint(stdout, iconsGenUsage)
					return 0
				}
				fmt.Fprint(stdout, iconsUsage)
				return 0
			case "build":
				fmt.Fprint(stdout, buildUsage)
				return 0
			case "run":
				fmt.Fprint(stdout, runUsage)
				return 0
			case "package":
				fmt.Fprint(stdout, packageUsage)
				return 0
			case "new":
				fmt.Fprint(stdout, newUsage)
				return 0
			case "version":
				fmt.Fprint(stdout, versionUsage)
				return 0
			case "update":
				return runUpdate(append([]string{"help"}, args[2:]...), stdout, stderr)
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
