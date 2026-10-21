package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const snapshotUsage = `Usage:

	tela snapshot [--size WxH] [--scale N] [--dark] [--preview NAME] [-o out.png] <pkg>

Renders a package's screen headlessly, with Tela's own renderer and the
embedded fonts, and writes it as a PNG.

<pkg> is a package path or a relative directory, such as ./examples/counter.
The package must define one of

	func Preview() tela.Node

		a single screen (it may return a tela.Component, such as a struct
		component, instead), or

	var Previews = tela.Previews{
	    "Login":    {Root: tela.C(Login)},
	    "Settings": {Root: Settings{Tab: "account"}},
	}

		several named screens; --preview picks one, and without it the first
		name in byte order is rendered. If a package defines both, Previews
		wins. A Preview may also give a Wrap function that returns its root
		inside the providers of the contexts it reads.

or a var App = tela.App{..., Root: Screen}, whose Root is the screen.

What a preview returns is mounted as is, so a function component needs
tela.C (a preview that calls hooks directly is a mistake dev mode reports).
The screen is drawn on a white canvas: give it its own background
(min-h-screen bg-...) or the App a RootClass.

A package that also defines a package-level

	var App = tela.App{Width: 1440, Height: 900, RootClass: "bg-background",
	    ColorScheme: tela.Dark, Theme: &theme, ClassAliases: aliases, ...}

(the value its main passes to tela.Main) is rendered in that window: the
image has the App's Width and Height (unless --size is given), RootClass,
ColorScheme, Theme, ClassAliases, ClassicScrollbars, InstantScroll and Dev
apply, so the PNG matches what the window shows. Title and AppID are
ignored, and so is Root when the package has previews. The App's
ColorScheme overrides --dark, as it overrides the system in a window.
Dev warnings and class errors of the screen are printed on standard error.

For the packages of phase 1 (such as examples/text) a package may
instead define

	func Scene(w, h float32) *gfx.DisplayList

which builds a scene for a window of w x h logical pixels.

Flags:

	--size WxH       logical size of the window (default: the App's Width and
	                 Height when the package defines them, else 960x640)
	--scale N        display scale; fractional scales such as 1.25 are allowed
	                 (default 1). The PNG is (W*N)x(H*N) pixels, rounded like
	                 the window backends do
	--dark           render in dark mode: dark: variants apply (not available
	                 for Scene packages)
	--preview NAME   the named screen of a Previews variable
	-o file          output file (default snapshot.png)
`

// snapshotOptions are the parsed arguments of `tela snapshot`.
type snapshotOptions struct {
	renderOptions
	Out string
}

// parseSnapshotArgs parses the arguments of `tela snapshot`.
func parseSnapshotArgs(args []string) (snapshotOptions, error) {
	opts := snapshotOptions{Out: "snapshot.png"}
	var size string
	pkg, err := parseFlags("snapshot", args, func(fs *flag.FlagSet) {
		opts.register(fs, &size, true)
		fs.StringVar(&opts.Out, "o", opts.Out, "")
	})
	if err != nil {
		return opts, err
	}
	opts.Pkg = pkg
	if err := opts.validate(size); err != nil {
		return opts, err
	}
	if opts.Out == "" {
		return opts, errors.New("empty -o: give an output file such as -o snapshot.png")
	}
	return opts, nil
}

// runSnapshot implements `tela snapshot`.
func runSnapshot(args []string, stdout, stderr io.Writer) int {
	opts, err := parseSnapshotArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, snapshotUsage)
			return 0
		}
		fmt.Fprintf(stderr, "tela snapshot: %v\n\n%s", err, snapshotUsage)
		return 2
	}
	if err := snapshot(opts, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "tela snapshot: %v\n", err)
		return 1
	}
	return 0
}

// snapshot renders opts.Pkg to opts.Out.
func snapshot(opts snapshotOptions, stdout, stderr io.Writer) error {
	out, err := filepath.Abs(opts.Out)
	if err != nil {
		return err
	}
	if st, err := os.Stat(out); err == nil && st.IsDir() {
		return fmt.Errorf("%s is a directory; give a file name such as %s", out, filepath.Join(out, "snapshot.png"))
	}
	res, err := runInPackage(modeSnapshot, opts.renderOptions, "png", stderr)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(out, res.Output); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s%s\n", out, res.describe())
	return nil
}

// describe returns ` (preview "Login"; others: Settings)` for a Previews
// package, and "" otherwise.
func (r *runResult) describe() string {
	if r.Info.Preview == "" {
		return ""
	}
	s := fmt.Sprintf(" (preview %q", r.Info.Preview)
	if others := without(r.Info.Names, r.Info.Preview); len(others) > 0 {
		s += "; others: " + strings.Join(others, ", ")
	}
	return s + ")"
}
