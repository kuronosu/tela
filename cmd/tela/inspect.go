package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

const inspectUsage = `Usage:

	tela inspect [--aria | --layout] [--size WxH] [--dark] [--preview NAME] <pkg>

Prints what a package's screen contains after a headless render.

--aria (the default) prints its aria snapshot: the semantics tree as the
YAML of Playwright's locator.ariaSnapshot(), one line per role with its
accessible name and states, and the text between them:

	- heading "Settings" [level=1]
	- switch "Email notifications" [checked]
	- button "Save changes" [disabled]

Elements without a role (Div, Span) are left out and their texts merged;
hidden elements are left out.

--layout prints the layout tree as JSON: one object per box, with its
border box in logical pixels (rect: x, y, width, height from the top-left of
the window, unrounded), its resolved margin, border and padding (each left
out when all four edges are zero), the text and line count of paragraphs,
the element's classes and a summary of its computed style (display, size,
flex settings, color, background, font, opacity) and the class errors of the
element. Children nest in tree order. The field order is stable.

rect is the box as layout placed it. When the box is painted elsewhere, its
object also has bounds, in the same shape as rect: the bounding box of the
border box in window coordinates after the scroll offsets of the containers
around it and the transforms of the box and its ancestors, which is what a
user sees and what ElementRef.Bounds returns (getBoundingClientRect). A
box that is not moved, scaled or rotated, nor inside scrolled content, has
no bounds.

A paragraph lists its inline elements (inline: "span", "code", "a"...) and
their fragments (fragments: one per line an element is on, and per bidi
run that separates its text, each with the element's index in inline, its
line and its border box rect, padding and borders included). Its atomic
inlines (inline-block, inline-flex, buttons in the text) are its children,
marked inlineBox: true.

<pkg> and the screen are chosen as in "tela snapshot": the package defines
func Preview() tela.Node (or tela.Component), a Previews variable or an App,
and --preview picks a screen (without it, the first name in byte order). A
package-level App also sets the size, root class, color scheme, theme and
class aliases of the render, as for "tela snapshot"; dev warnings and class
errors go to standard error, so standard output is the output only.

Flags:

	--aria           print the aria snapshot as YAML (the default)
	--layout         print the layout tree as JSON
	--size WxH       logical size of the window (default: the App's Width and
	                 Height when the package defines them, else 960x640)
	--dark           render in dark mode: dark: variants apply
	--preview NAME   the named screen of a Previews variable
`

// inspectOptions are the parsed arguments of `tela inspect`.
type inspectOptions struct {
	renderOptions
	// Layout selects the layout tree; otherwise the aria snapshot.
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
	if aria && opts.Layout {
		return opts, errors.New("--aria and --layout exclude each other: --aria prints the aria snapshot (the default), --layout the layout tree")
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
	m, ext := modeAria, "yml"
	if opts.Layout {
		m, ext = modeInspect, "json"
	}
	res, err := runInPackage(m, opts.renderOptions, ext, stderr)
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
