package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// Limits that keep a typo from allocating gigabytes.
const (
	maxLogicalSide  = 32768
	maxPhysicalSide = 32768
	maxPhysicalPix  = 1 << 28 // 256 Mpx, 1 GiB of RGBA
)

// Defaults of the window a package is rendered in.
const (
	defaultWidth  = 960
	defaultHeight = 640
)

// renderOptions are the arguments `tela snapshot` and `tela inspect` share:
// which package and which screen of it, in what environment.
type renderOptions struct {
	Pkg     string
	W, H    float64 // logical size
	Scale   float64
	Dark    bool
	Preview string // name in the package's Previews; empty means the first
}

// parseFlags parses args with flags defined by setup, and returns the single
// positional argument, the package. Flags and the package may come in any
// order. The flag errors are decorated with a pointer to the command's help.
func parseFlags(command string, args []string, setup func(fs *flag.FlagSet)) (pkg string, err error) {
	fs := flag.NewFlagSet("tela "+command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	setup(fs)

	var pkgs []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return "", err
			}
			return "", fmt.Errorf("%v (see \"tela help %s\")", err, command)
		}
		args = fs.Args()
		if len(args) > 0 {
			pkgs = append(pkgs, args[0])
			args = args[1:]
		}
	}
	switch len(pkgs) {
	case 0:
		return "", fmt.Errorf("missing <pkg>: say which package to render, for example \"tela %s ./examples/counter\"", command)
	case 1:
		return pkgs[0], nil
	}
	return "", fmt.Errorf("expected one <pkg>, got %d (%s)", len(pkgs), strings.Join(pkgs, ", "))
}

// renderFlags registers the flags of renderOptions on fs. size is where the
// raw --size is stored until validate parses it. scale reports whether the
// command takes --scale.
func (o *renderOptions) register(fs *flag.FlagSet, size *string, scale bool) {
	o.W, o.H, o.Scale = defaultWidth, defaultHeight, 1
	fs.StringVar(size, "size", "", "")
	if scale {
		fs.Float64Var(&o.Scale, "scale", o.Scale, "")
	}
	fs.BoolVar(&o.Dark, "dark", false, "")
	fs.StringVar(&o.Preview, "preview", "", "")
}

// validate checks the options once the flags are parsed.
func (o *renderOptions) validate(size string) error {
	if size != "" {
		w, h, err := parseSize(size)
		if err != nil {
			return err
		}
		o.W, o.H = w, h
	}
	if !(o.Scale > 0) || math.IsInf(o.Scale, 0) || o.Scale > 64 {
		return fmt.Errorf("invalid --scale %v: use a number > 0 and <= 64, for example 1, 2 or 1.25", o.Scale)
	}
	px, py := physicalSide(o.W, o.Scale), physicalSide(o.H, o.Scale)
	if px > maxPhysicalSide || py > maxPhysicalSide || px*py > maxPhysicalPix {
		return fmt.Errorf("a %gx%g window at scale %g is %dx%d pixels, which is too large; lower --size or --scale", o.W, o.H, o.Scale, px, py)
	}
	return nil
}

// physicalSide mirrors platform.PhysicalSize for one axis, only to bound the
// image size before running anything (the generated test uses the real one).
func physicalSide(logical, scale float64) int { return int(math.Round(logical * scale)) }

// parseSize parses "WxH" with positive numbers.
func parseSize(s string) (w, h float64, err error) {
	bad := fmt.Errorf("invalid --size %q: use WxH with positive numbers, for example 960x640", s)
	ws, hs, ok := strings.Cut(strings.ToLower(s), "x")
	if !ok {
		return 0, 0, bad
	}
	w, err1 := strconv.ParseFloat(ws, 64)
	h, err2 := strconv.ParseFloat(hs, 64)
	if err1 != nil || err2 != nil || !(w > 0) || !(h > 0) || w > maxLogicalSide || h > maxLogicalSide {
		return 0, 0, bad
	}
	return w, h, nil
}
