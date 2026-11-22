package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const snapshotUsage = `Usage:

	tela snapshot [--size WxH] [--scale N] [--dark] [--preview NAME] [-o out.png] <pkg>
	tela snapshot --film SPAN [--every DUR] [--apng film-anim.png] [--click LOC | --hover LOC | --press KEY] [flags] <pkg>

Renders a package's screen headlessly, with Tela's own renderer and fonts
(the App's Fonts, then Tela's standard ones, never the system's), and writes
it as a PNG.

The screen is rendered at rest, as a window shows it after a moment: the
effects that run after the first frame have run, the state they set and the
toasts they show are built, and finite animations are finished. One that
never ends (a spinner, state set on every frame) is shown at its frame after
a few seconds of the app's time (state that is set on every frame also gets
a note on standard error).

<pkg> is a package path or a relative directory, such as ./examples/counter.
The package must define one of

	func Preview() tela.Node

		a single screen, built as a component of its own (it may use
		hooks, and return an element or a struct component), or

	var Previews = tela.Previews{
	    "Login":    {Root: tela.C(Login)},
	    "Settings": {Root: Settings{Tab: "account"}},
	}

		several named screens; --preview picks one, and without it the first
		name in byte order is rendered. If a package defines both, Previews
		wins. A Preview may also give a Wrap function that returns its root
		inside the providers of the contexts it reads.

or a var App = tela.App{..., Root: tela.C(Screen)}, whose Root is the screen.

What a preview returns is mounted as is, so a function component needs
tela.C (a preview that calls hooks directly is a mistake dev mode reports).
The screen is drawn on a white canvas: give it its own background
(min-h-screen bg-...) or the App a RootClass.

A package that also defines a package-level

	var App = tela.App{Width: 1440, Height: 900, RootClass: "bg-background",
	    ColorScheme: tela.Dark, Theme: &theme, ClassAliases: aliases, ...}

(the value its main passes to tela.Main) is rendered in that window: the
image has the App's Width and Height (unless --size is given), RootClass,
ColorScheme, Theme, ClassAliases, ClassicScrollbars, InstantScroll,
NativeMenus, IgnoreTextScale and Dev apply, so the PNG matches what the
window shows. Title and AppID are
ignored, and so is Root when the package has previews. The App's
ColorScheme overrides --dark, as it overrides the system in a window.
Dev warnings and class errors of the screen are printed on standard error.

Film. With --film the screen is not rendered at rest but while it moves: the
frames of the first SPAN of the app's clock after the mount (an entrance
that a passive effect starts, a transition, a spring, a spinner, a Presence
entering), one every --every, as a contact sheet: a grid of the frames, each
scaled down to keep the sheet within 1600 pixels and captioned with its time
("150 ms"). More than 24 frames are sampled evenly for the sheet. The clock
is the app's, not the wall's, so the film is deterministic. --apng also
writes every frame as an animated PNG (it plays in browsers and image
viewers; one frame is the first for programs that do not animate). A film is
at most 60 frames and 30 s.

With --click, --hover or --press the film starts with that action instead of
the mount: the screen is settled first, the action runs, and the first frame
is right after it (a dialog that opens, a hover transition, a menu a key
shows). An element is named as Playwright's selectors name it:
role=button[name="Open"] (the name may be left out), text=Open (any case,
contained) or text="Open" (exact), or testid=open (an h.TestID). For a
longer session use the film tool of tela mcp.

Flags:

	--size WxH       logical size of the window (default: the App's Width and
	                 Height when the package defines them, else 960x640)
	--scale N        display scale; fractional scales such as 1.25 are allowed
	                 (default 1). The PNG is (W*N)x(H*N) pixels, rounded like
	                 the window backends do
	--dark           render in dark mode: dark: variants apply
	--preview NAME   the named screen of a Previews variable
	-o file          output file (default snapshot.png, or film.png with
	                 --film: the contact sheet)
	--film SPAN      film the first SPAN (such as 600ms or 1.5s) of the
	                 app's clock instead of a still
	--every DUR      with --film: the time between frames (default 50ms;
	                 at least 1ms). Browsers play an animated PNG's frames
	                 shorter than 20 ms slower
	--apng file      with --film: also write the animated PNG
	--click LOC      with --film: film what a click on the element starts
	--hover LOC      with --film: film what moving the pointer over it starts
	--press KEY      with --film: film what a key starts, in Playwright's
	                 syntax (Escape, Enter, Control+k), on the focused element
`

// snapshotOptions are the parsed arguments of `tela snapshot`.
type snapshotOptions struct {
	renderOptions
	Out  string
	APNG string // with Film: the animated PNG to write
	// ActArg is the flag's value of the action, for the output.
	ActArg string
}

// parseSnapshotArgs parses the arguments of `tela snapshot`.
func parseSnapshotArgs(args []string) (snapshotOptions, error) {
	opts := snapshotOptions{Out: "snapshot.png"}
	var size, film, every, click, hover, press string
	outSet := false
	pkg, err := parseFlags("snapshot", args, func(fs *flag.FlagSet) {
		opts.register(fs, &size, true)
		fs.Func("o", "", func(v string) error { opts.Out, outSet = v, true; return nil })
		fs.StringVar(&film, "film", "", "")
		fs.StringVar(&every, "every", "", "")
		fs.StringVar(&opts.APNG, "apng", "", "")
		fs.StringVar(&click, "click", "", "")
		fs.StringVar(&hover, "hover", "", "")
		fs.StringVar(&press, "press", "", "")
	})
	if err != nil {
		return opts, err
	}
	opts.Pkg = pkg
	if err := opts.validate(size); err != nil {
		return opts, err
	}
	if outSet && opts.Out == "" {
		return opts, errors.New("empty -o: give an output file such as -o snapshot.png")
	}
	if film == "" {
		switch {
		case every != "":
			return opts, errors.New("--every needs --film: say how long to film, for example --film 600ms --every 50ms")
		case opts.APNG != "":
			return opts, errors.New("--apng needs --film: say how long to film, for example --film 600ms --apng motion.png")
		case click != "" || hover != "" || press != "":
			return opts, errors.New(`--click, --hover and --press need --film: say how long to film, for example --film 600ms --click 'role=button[name="Open"]'`)
		}
		return opts, nil
	}
	given := 0
	for _, v := range []string{click, hover, press} {
		if v != "" {
			given++
		}
	}
	switch {
	case given > 1:
		return opts, errors.New("a film starts with one action: give one of --click, --hover and --press")
	case click != "" || hover != "":
		op, v := "click", click
		if hover != "" {
			op, v = "hover", hover
		}
		loc, err := parseFilmLocator(op, v)
		if err != nil {
			return opts, err
		}
		opts.Act, opts.ActArg = &filmAct{Op: op, Loc: loc}, strings.TrimSpace(v)
	case press != "":
		opts.Act, opts.ActArg = &filmAct{Op: "press", Key: strings.TrimSpace(press)}, strings.TrimSpace(press)
	}
	if opts.Film, err = parseFilmDuration("film", film); err != nil {
		return opts, err
	}
	if every != "" {
		if opts.Every, err = parseFilmDuration("every", every); err != nil {
			return opts, err
		}
	}
	if opts.Every, _, err = checkFilm(opts.Film, opts.Every); err != nil {
		return opts, fmt.Errorf("--film: %v", err)
	}
	if !outSet {
		opts.Out = "film.png"
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
	if opts.Film > 0 {
		return snapshotFilm(opts, out, stdout, stderr)
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

// snapshotFilm renders the film of opts.Pkg: the contact sheet to out and,
// with --apng, the animated PNG.
func snapshotFilm(opts snapshotOptions, out string, stdout, stderr io.Writer) error {
	apng := ""
	if opts.APNG != "" {
		var err error
		if apng, err = filepath.Abs(opts.APNG); err != nil {
			return err
		}
		if apng == out {
			return fmt.Errorf("-o and --apng are the same file (%s): the contact sheet and the animation are two files", out)
		}
	}
	res, err := runInPackage(modeFilm, opts.renderOptions, "json", stderr)
	if err != nil {
		return err
	}
	var wire []struct {
		At  int64  `json:"at"`
		PNG []byte `json:"png"`
	}
	if err := json.Unmarshal(res.Output, &wire); err != nil {
		return fmt.Errorf("unreadable film: %v", err)
	}
	var frames []filmFrame
	for _, w := range wire {
		f, err := decodeFilmFrame(time.Duration(w.At)*time.Millisecond, w.PNG)
		if err != nil {
			return err
		}
		frames = append(frames, f)
	}
	sheet, err := encodePNG(contactSheet(frames))
	if err != nil {
		return err
	}
	if err := writeFileAtomic(out, sheet); err != nil {
		return err
	}
	after := ""
	if opts.Act != nil {
		after = ", " + opts.Act.describe(opts.ActArg)
	}
	fmt.Fprintf(stdout, "wrote %s (%d frames, 0 to %d ms every %d ms%s)%s\n", out, len(frames), opts.Film.Milliseconds(), opts.Every.Milliseconds(), after, res.describe())
	if apng != "" {
		data, err := encodeAPNG(frames)
		if err != nil {
			return err
		}
		if err := writeFileAtomic(apng, data); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "wrote %s (animated PNG, %d frames)\n", apng, len(frames))
	}
	return nil
}
