package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const inspectUsage = `Usage:

	tela inspect [--layout] [--size WxH] [--dark] [--preview NAME] <pkg>

Prints the layout tree of a package's screen as JSON, after a headless
render: one object per box, with its border box in logical pixels (rect: x,
y, width, height from the top-left of the window, unrounded), its resolved
margin, border and padding (each left out when all four edges are zero), the text and line count of paragraphs, the
element's classes and a summary of its computed style (display, size, flex
settings, color, background, font, opacity) and the class errors of the
element. Children nest in tree order. The field order is stable.

<pkg> and the screen are chosen as in "tela snapshot": the package defines
func Preview() tela.Node or a Previews variable, and --preview picks a screen
(without it, the first name in byte order).

--layout is the only mode of inspect until the semantics tree exists
(phase 4), which adds --aria, a Playwright-style YAML snapshot and the
default mode. It is accepted now so that scripts written today keep working;
without any mode flag, inspect prints the layout tree.

Flags:

	--layout         print the layout tree (the default for now)
	--size WxH       logical size of the window (default 960x640)
	--dark           lay out in dark mode: dark: variants apply
	--preview NAME   the named screen of a Previews variable
`

// inspectOptions are the parsed arguments of `tela inspect`.
type inspectOptions struct {
	renderOptions
	Layout bool
}

// parseInspectArgs parses the arguments of `tela inspect`.
func parseInspectArgs(args []string) (inspectOptions, error) {
	var opts inspectOptions
	var size string
	var aria bool
	pkg, err := parseFlags("inspect", args, func(fs *flag.FlagSet) {
		opts.register(fs, &size, false)
		fs.BoolVar(&opts.Layout, "layout", false, "")
		fs.BoolVar(&aria, "aria", false, "")
	})
	if err != nil {
		return opts, err
	}
	opts.Pkg = pkg
	if aria {
		return opts, errors.New("--aria is not available yet: the semantics tree arrives in phase 4. Use --layout (the default) for the layout tree as JSON")
	}
	if err := opts.validate(size); err != nil {
		return opts, err
	}
	return opts, nil
}

// runInspect implements `tela inspect`.
func runInspect(args []string, stdout, stderr io.Writer) int {
	opts, err := parseInspectArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, inspectUsage)
			return 0
		}
		fmt.Fprintf(stderr, "tela inspect: %v\n\n%s", err, inspectUsage)
		return 2
	}
	res, err := runInPackage(modeInspect, opts.renderOptions, "json", stderr)
	if err != nil {
		fmt.Fprintf(stderr, "tela inspect: %v\n", err)
		return 1
	}
	if opts.Preview == "" && len(res.Info.Names) > 1 {
		fmt.Fprintf(stderr, "tela inspect: preview %q (the package also defines: %s); use --preview NAME for another\n",
			res.Info.Preview, strings.Join(without(res.Info.Names, res.Info.Preview), ", "))
	}
	if _, err := stdout.Write(res.Output); err != nil {
		fmt.Fprintf(stderr, "tela inspect: %v\n", err)
		return 1
	}
	return 0
}

// without returns names minus one.
func without(names []string, name string) []string {
	var out []string
	for _, n := range names {
		if n != name {
			out = append(out, n)
		}
	}
	return out
}
