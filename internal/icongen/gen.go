// Package icongen turns a directory of SVG files into a Go file with one
// tela.Icon variable per file; it is the engine of "tela icons gen"
// (ADR-102).
//
// Each file is validated with the SVG parser Tela draws with, minified
// (Minify) with a check that the minified document draws the same display
// lists as the original (Equivalent), named after its file (VarName) and
// written as
//
//	var Check = tela.Icon{SVG: `<svg ...>...</svg>`}
//
// in one gofmt'd file, sorted by name, so the output is the same on every
// run. There is no table of icons by name: a variable nothing refers to is
// left out of the binary by the linker, a table would keep all of them.
package icongen

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kuronosu/tela/internal/svg"
)

// Options configure Generate.
type Options struct {
	// Package is the name of the generated package (required).
	Package string
	// Recursive reads the subdirectories too; the variable names then
	// include the directories ("solid/arrow-up.svg" is SolidArrowUp).
	Recursive bool
	// Prefix and Suffix are added around every variable name. A Prefix
	// must start with a capital letter.
	Prefix, Suffix string
	// SkipInvalid skips the files that cannot be parsed (or minified
	// equivalently) and reports them in Result.Skipped; otherwise they are
	// an error.
	SkipInvalid bool
	// Minify rewrites each document without what does not draw (Minify)
	// and checks that it draws the same.
	Minify bool
	// Source describes where the icons come from ("Lucide 1.61.0,
	// https://lucide.dev"), License names their license ("ISC"), and
	// LicenseText is the text of the license file, copied into the
	// generated file. All three are optional.
	Source, License, LicenseText string
	// Deprecated marks icons as deprecated in favor of others: each key is
	// the name of an icon (its file name without the extension, slash
	// separated) and the value the name of the icon that replaces it. The
	// variable of a deprecated icon gets a "Deprecated: use X." paragraph in
	// its documentation, so gopls and staticcheck flag its uses. A key or a
	// value that is not an icon of the directory, and a replacement that is
	// itself deprecated, are errors.
	Deprecated map[string]string
}

// Icon is one generated icon.
type Icon struct {
	File string // the path of the SVG file, relative to the directory
	Name string // the file name without the extension, slash separated
	Var  string // the Go variable
	Use  string // the variable that replaces a deprecated icon, or ""
	SVG  string // the document that went into the file
	Size int    // the size of the file in bytes before minifying
}

// Problem is a file that was not turned into an icon.
type Problem struct {
	File string
	Err  error
}

func (p Problem) String() string { return fmt.Sprintf("%s: %v", p.File, p.Err) }

// Result is what Generate produced.
type Result struct {
	// Code is the generated Go file, formatted.
	Code  []byte
	Icons []Icon
	// Skipped are the invalid files left out because of SkipInvalid.
	Skipped []Problem
	// BytesIn and BytesOut add up the size of the SVG files and of the
	// documents in the generated file.
	BytesIn, BytesOut int
}

// Error lists the files that made Generate fail.
type Error struct{ Problems []Problem }

func (e *Error) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d problems:", len(e.Problems))
	for _, p := range e.Problems {
		sb.WriteString("\n\t")
		sb.WriteString(p.String())
	}
	return sb.String()
}

// maxPrinted bounds how many problems an Error lists.
const maxPrinted = 50

// limitedError makes the error of a list of problems, listing at most
// maxPrinted of them, followed by hint when it is not empty.
func limitedError(problems []Problem, hint string) error {
	if len(problems) > maxPrinted {
		problems = append(problems[:maxPrinted:maxPrinted], Problem{File: "...", Err: fmt.Errorf("and %d more", len(problems)-maxPrinted)})
	}
	if hint == "" {
		return &Error{Problems: problems}
	}
	return fmt.Errorf("%w\n%s", &Error{Problems: problems}, hint)
}

// Generate reads the SVG files of fsys (every *.svg, case-insensitive,
// except those whose name starts with a dot) and returns the Go file.
func Generate(fsys fs.FS, opt Options) (*Result, error) {
	if opt.Package == "" {
		return nil, errors.New("missing the package name: pass -pkg, for example -pkg icons")
	}
	if !token.IsIdentifier(opt.Package) || opt.Package == "_" {
		return nil, fmt.Errorf("invalid package name %q: use a Go identifier such as icons", opt.Package)
	}
	if err := checkAffix(opt.Prefix, opt.Suffix); err != nil {
		return nil, err
	}

	var files []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != "." && (!opt.Recursive || strings.HasPrefix(d.Name(), ".")) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !strings.EqualFold(path.Ext(d.Name()), ".svg") {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		hint := ""
		if !opt.Recursive {
			hint = " (use -r to read the subdirectories too)"
		}
		return nil, fmt.Errorf("no .svg files found%s", hint)
	}
	sort.Strings(files)

	res := &Result{}
	var problems, collisions []Problem
	byVar := map[string]string{}
	for _, f := range files {
		ic, err := convert(fsys, f, opt)
		if err == nil {
			ic.Var, err = VarName(f, opt.Prefix, opt.Suffix)
		}
		if err == nil {
			if other, dup := byVar[ic.Var]; dup {
				// A collision is never skipped: leaving one of the two out
				// would hide an icon.
				collisions = append(collisions, Problem{File: f, Err: fmt.Errorf("the name %s is already made from %s; rename one of the two files", ic.Var, other)})
				continue
			}
		}
		if err != nil {
			p := Problem{File: f, Err: err}
			if opt.SkipInvalid {
				res.Skipped = append(res.Skipped, p)
			} else {
				problems = append(problems, p)
			}
			continue
		}
		byVar[ic.Var] = f
		res.Icons = append(res.Icons, ic)
		res.BytesIn += ic.Size
		res.BytesOut += len(ic.SVG)
	}
	if len(collisions) > 0 {
		return nil, limitedError(collisions, "")
	}
	if len(problems) > 0 {
		return nil, limitedError(problems, "fix the files or pass -skip-invalid to leave them out")
	}
	if len(res.Icons) == 0 {
		return nil, errors.New("every .svg file was skipped; nothing to generate")
	}
	if err := markDeprecated(res.Icons, opt.Deprecated); err != nil {
		return nil, err
	}
	sort.Slice(res.Icons, func(i, j int) bool { return res.Icons[i].Var < res.Icons[j].Var })

	code, err := render(opt, res.Icons)
	if err != nil {
		return nil, err
	}
	res.Code = code
	return res, nil
}

// markDeprecated sets Icon.Use for the icons named by deprecated.
func markDeprecated(icons []Icon, deprecated map[string]string) error {
	if len(deprecated) == 0 {
		return nil
	}
	byName := make(map[string]int, len(icons))
	for i, ic := range icons {
		byName[ic.Name] = i
	}
	names := make([]string, 0, len(deprecated))
	for name := range deprecated {
		names = append(names, name)
	}
	sort.Strings(names)
	var problems []Problem
	for _, name := range names {
		use := deprecated[name]
		i, ok := byName[name]
		if !ok {
			problems = append(problems, Problem{File: name, Err: fmt.Errorf("deprecated icon %q is not an icon of the directory (a name is a file name without .svg, such as \"alert-circle\"; was the file skipped?)", name)})
			continue
		}
		j, ok := byName[use]
		switch {
		case !ok:
			problems = append(problems, Problem{File: name, Err: fmt.Errorf("deprecated in favor of %q, which is not an icon of the directory", use)})
		case use == name:
			problems = append(problems, Problem{File: name, Err: errors.New("deprecated in favor of itself")})
		default:
			if _, again := deprecated[use]; again {
				problems = append(problems, Problem{File: name, Err: fmt.Errorf("deprecated in favor of %q, which is deprecated too; name the final icon", use)})
				continue
			}
			icons[i].Use = icons[j].Var
		}
	}
	if len(problems) > 0 {
		return limitedError(problems, "fix the deprecated list (-deprecated)")
	}
	return nil
}

// convert reads, validates and minifies one file.
func convert(fsys fs.FS, name string, opt Options) (Icon, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return Icon{}, err
	}
	// The same parser tela.ParseSVG uses, so an icon that passes here
	// passes at run time.
	if _, err := svg.Parse(data); err != nil {
		return Icon{}, fmt.Errorf("the SVG cannot be parsed: %w", err)
	}
	doc := bytes.TrimSpace(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	if opt.Minify {
		min, err := Minify(data)
		switch {
		case errors.Is(err, ErrNotMinifiable):
			// Kept as written.
		case err != nil:
			return Icon{}, fmt.Errorf("cannot minify: %w (pass -minify=false to keep the files as they are)", err)
		default:
			if err := Equivalent(data, min); err != nil {
				return Icon{}, fmt.Errorf("the minified document does not draw like the original: %w (pass -minify=false to keep the files as they are)", err)
			}
			doc = min
		}
	}
	if !utf8.Valid(doc) {
		return Icon{}, errors.New("the file is not valid UTF-8")
	}
	return Icon{
		File: name,
		Name: strings.TrimSuffix(name, path.Ext(name)),
		SVG:  string(doc),
		Size: len(data),
	}, nil
}

// render writes the Go file.
func render(opt Options, icons []Icon) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by tela icons gen; DO NOT EDIT.\n")
	if opt.Source != "" {
		comment(&b, "Source: "+opt.Source)
	}
	if opt.License != "" {
		comment(&b, "License: "+opt.License)
	}
	b.WriteString("\n")
	if t := strings.TrimSpace(opt.LicenseText); t != "" {
		comment(&b, t)
		b.WriteString("\n")
	}
	noun := "icons"
	if len(icons) == 1 {
		noun = "icon"
	}
	fmt.Fprintf(&b, "// Package %s holds %d SVG %s as tela.Icon variables, generated by\n", opt.Package, len(icons), noun)
	b.WriteString("// \"tela icons gen\"")
	if opt.Source != "" {
		b.WriteString(" from " + strings.Join(strings.Fields(opt.Source), " "))
	}
	b.WriteString(".")
	if opt.License != "" {
		b.WriteString(" License: " + strings.Join(strings.Fields(opt.License), " ") + ".")
	}
	b.WriteString("\n//\n// Show one with h.Icon. It is 1em wide and high unless a class sets its size,\n")
	b.WriteString("// and it is drawn in the text color of its element:\n//\n")
	fmt.Fprintf(&b, "//\th.Icon(%s.%s, Class(\"size-4 text-muted-foreground\"))\n//\n", opt.Package, icons[0].Var)
	b.WriteString("// Every icon is a variable of static data and there is no table of them by\n")
	b.WriteString("// name, so the linker leaves out the icons a program does not use.\n")
	fmt.Fprintf(&b, "package %s\n\nimport \"github.com/kuronosu/tela\"\n", opt.Package)
	for _, ic := range icons {
		fmt.Fprintf(&b, "\n// %s is the icon %s.\n", ic.Var, strconv.Quote(ic.Name))
		if ic.Use != "" {
			fmt.Fprintf(&b, "//\n// Deprecated: use %s.\n", ic.Use)
		}
		fmt.Fprintf(&b, "var %s = tela.Icon{SVG: %s}\n", ic.Var, literal(ic.SVG))
	}
	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("the generated file does not format (a bug in tela icons gen): %w", err)
	}
	return out, nil
}

// comment writes text as line comments.
func comment(b *bytes.Buffer, text string) {
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			b.WriteString("//\n")
		} else {
			b.WriteString("// " + line + "\n")
		}
	}
}

// literal returns s as a Go string literal: raw when it can be, so that the
// markup stays readable.
func literal(s string) string {
	if strings.ContainsAny(s, "`\r\x00\uFEFF") {
		return strconv.Quote(s)
	}
	return "`" + s + "`"
}
