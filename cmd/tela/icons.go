package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuronosu/tela/internal/icongen"
)

const iconsUsage = `Usage:

	tela icons gen [flags] <dir>

Commands:

	gen     generate a Go package of tela.Icon variables from a directory of
	        SVG files

Run "tela help icons gen" for the flags.
`

const iconsGenUsage = `Usage:

	tela icons gen -pkg <name> [-o outdir] [flags] <dir>

Reads the SVG files of <dir> and writes icons_gen.go, a Go file with one
exported tela.Icon variable per file:

	// ArrowUp is the icon "arrow-up".
	var ArrowUp = tela.Icon{SVG: ` + "`<svg ...>...</svg>`" + `}

Show one with h.Icon(icons.ArrowUp, Class("size-4")). There is no table of
icons by name on purpose: the linker leaves out the variables a program does
not use, and a table would keep all of them.

Every file is checked with the SVG parser Tela draws with (a file that fails
is reported with its path, and stops the command unless -skip-invalid is
given) and minified: the XML declaration, comments, DOCTYPE, metadata, title
and desc, editor namespaces (Inkscape, Illustrator), class attributes and
unreferenced ids (when the file has no style element), and white space between
elements go; the command checks that the minified document draws the same as
the original, so a file that would draw differently is an error. Files that
declare entities are kept as written. The output is the same on every run.

Variable names come from the file names, split at every character that is not
a letter or a digit: arrow-up.svg is ArrowUp, volume_2.svg is Volume2,
3d-rotate.svg is Icon3dRotate (a name must start with a capital letter), and
with -r solid/arrow-up.svg is SolidArrowUp. Digits that were apart stay apart
with an underscore: arrow-down-0-1.svg is ArrowDown0_1 and arrow-down-01.svg is
ArrowDown01. Two files that make the same name are an error. Files whose name
starts with a dot are ignored.

Flags:

	-pkg name        package name of the generated file (required)
	-o dir           output directory (default .); the file is dir/icons_gen.go
	-r               read the subdirectories of <dir> too
	-prefix text     put text before every variable name (it must start with a
	                 capital letter, for example -prefix Icon)
	-suffix text     put text after every variable name
	-skip-invalid    leave out the files that cannot be parsed, with a warning
	-minify=false    keep the SVG files as they are (only trimmed)
	-source text     where the icons come from, for the file header and the
	                 package documentation ("Lucide 1.61.0, https://lucide.dev")
	-license name    the license of the icons, for the same places ("ISC")
	-license-file f  copy the text of this license file into the header
	-deprecated f    a JSON file that marks icons as deprecated: an object from
	                 the name of an icon (its file name without .svg) to the
	                 name of the icon that replaces it, such as
	                 {"alert-circle": "circle-alert"}. The variable of
	                 alert-circle gets the paragraph "Deprecated: use
	                 CircleAlert." in its documentation, so gopls and
	                 staticcheck flag its uses. A name that is not a file of
	                 <dir>, or a replacement that is deprecated too, is an
	                 error.

Example, the way the Lucide module of the Tela repository is generated:

	tela icons gen -pkg lucide -source "Lucide 1.61.0" -license ISC \
	    -license-file LICENSE -deprecated deprecated.json ./lucide-static/icons
`

// iconsGenFile is the name of the generated file.
const iconsGenFile = "icons_gen.go"

// iconsGenOptions are the parsed arguments of `tela icons gen`.
type iconsGenOptions struct {
	icongen.Options
	Dir         string
	Out         string
	LicenseFile string
	// DeprecatedFile is the -deprecated JSON file.
	DeprecatedFile string
}

// parseIconsGenArgs parses the arguments of `tela icons gen`.
func parseIconsGenArgs(args []string) (iconsGenOptions, error) {
	opts := iconsGenOptions{Out: "."}
	opts.Minify = true
	fs := flag.NewFlagSet("tela icons gen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.Package, "pkg", "", "")
	fs.StringVar(&opts.Out, "o", opts.Out, "")
	fs.BoolVar(&opts.Recursive, "r", false, "")
	fs.StringVar(&opts.Prefix, "prefix", "", "")
	fs.StringVar(&opts.Suffix, "suffix", "", "")
	fs.BoolVar(&opts.SkipInvalid, "skip-invalid", false, "")
	fs.BoolVar(&opts.Minify, "minify", true, "")
	fs.StringVar(&opts.Source, "source", "", "")
	fs.StringVar(&opts.License, "license", "", "")
	fs.StringVar(&opts.LicenseFile, "license-file", "", "")
	fs.StringVar(&opts.DeprecatedFile, "deprecated", "", "")

	var dirs []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return opts, err
			}
			return opts, fmt.Errorf("%v (see \"tela help icons gen\")", err)
		}
		args = fs.Args()
		if len(args) > 0 {
			dirs = append(dirs, args[0])
			args = args[1:]
		}
	}
	switch len(dirs) {
	case 0:
		return opts, errors.New("missing <dir>: say where the SVG files are, for example \"tela icons gen -pkg icons ./svg\"")
	case 1:
		opts.Dir = dirs[0]
	default:
		return opts, fmt.Errorf("expected one <dir>, got %d (%s)", len(dirs), strings.Join(dirs, ", "))
	}
	if opts.Package == "" {
		return opts, errors.New("missing -pkg: name the Go package to generate, for example -pkg icons")
	}
	if opts.Out == "" {
		return opts, errors.New("empty -o: give an output directory such as -o ./icons, or leave it out for the current directory")
	}
	return opts, nil
}

// runIcons implements `tela icons`.
func runIcons(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, iconsUsage)
		return 2
	}
	switch args[0] {
	case "gen":
		return runIconsGen(args[1:], stdout, stderr)
	case "help", "-h", "-help", "--help":
		if len(args) > 1 && args[1] == "gen" {
			fmt.Fprint(stdout, iconsGenUsage)
			return 0
		}
		fmt.Fprint(stdout, iconsUsage)
		return 0
	}
	fmt.Fprintf(stderr, "tela icons: unknown command %q\n\n%s", args[0], iconsUsage)
	return 2
}

// runIconsGen implements `tela icons gen`.
func runIconsGen(args []string, stdout, stderr io.Writer) int {
	opts, err := parseIconsGenArgs(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, iconsGenUsage)
			return 0
		}
		fmt.Fprintf(stderr, "tela icons gen: %v\n\n%s", err, iconsGenUsage)
		return 2
	}
	if err := iconsGen(opts, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "tela icons gen: %v\n", err)
		return 1
	}
	return 0
}

func iconsGen(opts iconsGenOptions, stdout, stderr io.Writer) error {
	if fi, err := os.Stat(opts.Dir); err != nil {
		return fmt.Errorf("cannot read <dir>: %v", err)
	} else if !fi.IsDir() {
		return fmt.Errorf("<dir> %q is not a directory", opts.Dir)
	}
	if opts.LicenseFile != "" {
		text, err := os.ReadFile(opts.LicenseFile)
		if err != nil {
			return fmt.Errorf("cannot read -license-file: %v", err)
		}
		opts.LicenseText = string(text)
	}
	if opts.DeprecatedFile != "" {
		data, err := os.ReadFile(opts.DeprecatedFile)
		if err != nil {
			return fmt.Errorf("cannot read -deprecated: %v", err)
		}
		dec := json.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&opts.Deprecated); err != nil {
			return fmt.Errorf("cannot read -deprecated %s: %v (it is a JSON object from icon name to replacement name, such as {\"alert-circle\": \"circle-alert\"})", opts.DeprecatedFile, err)
		}
		if _, err := dec.Token(); err != io.EOF {
			return fmt.Errorf("cannot read -deprecated %s: unexpected data after the JSON object", opts.DeprecatedFile)
		}
	}
	res, err := icongen.Generate(os.DirFS(opts.Dir), opts.Options)
	if err != nil {
		return err
	}
	for _, p := range res.Skipped {
		fmt.Fprintf(stderr, "tela icons gen: skipped %s: %v\n", filepath.Join(opts.Dir, filepath.FromSlash(p.File)), p.Err)
	}
	if err := os.MkdirAll(opts.Out, 0o755); err != nil {
		return err
	}
	out := filepath.Join(opts.Out, iconsGenFile)
	if err := os.WriteFile(out, res.Code, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "tela icons gen: wrote %s: %d icons, %d skipped, SVG data %d KiB (%d KiB before minifying), file %d KiB\n",
		out, len(res.Icons), len(res.Skipped), (res.BytesOut+512)/1024, (res.BytesIn+512)/1024, (len(res.Code)+512)/1024)
	return nil
}
