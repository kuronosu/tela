package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/kuronosu/tela/telatest"
)

// tela check and tela verify (ADR-312): the audit of telatest (ADR-306)
// over the screens of a package, as a command. The generated test mounts
// each preview in each variant (check.gen.go) and returns the violations
// as JSON; this file builds the variants, merges what the variants found
// and prints it.

const checkUsage = `Usage:

	tela check [--preview NAME] [--size WxH] [--rules LIST] [--disable LIST] [-json] <pkg>

Looks for what a designer would call a bug in a package's screen: text
that is cut off or lies over other text, targets over targets, low
contrast, targets smaller than 24x24, controls a keyboard cannot operate,
focus that shows nothing, and missing names and states for screen readers.
It mounts the screen headlessly, as "tela snapshot" does, in light and in
dark mode, and runs every rule of telatest's accessibility audit
(telatest.Rules) on it; it is the fast pass, one screen. "tela verify"
runs the same rules over every preview and a matrix of widths, large text,
right-to-left and another language.

<pkg>, the screen, --preview and --size are as for "tela snapshot". The
screen is the package's Preview, its first preview in byte order or its
App's Root.

Each finding is printed with its source line, so that it can be opened:

	settings.go:42: color-contrast (serious): text is #9f9fa9 on #ffffff: contrast 2.63:1, text needs 4.5:1 (WCAG 2.2 AA 1.4.3)
	    element: text "Quarterly revenue" in p (class "text-sm text-zinc-400")
	    fix: raise the contrast to 4.5:1: ...
	    in: Overview, light, dark

The line is where the app made the element (a library component's is the
line that wrote it); a finding of a preview has the preview's name in
front of "in:". The same finding in several variants is printed once, with
the variants it appears in. -json prints the findings as one JSON object:
package, previews, variants, skipped (variants that do not apply, with
why) and findings (preview, rule, impact, message, fix, element, src, x, y,
width, height, variants).

Exit status: 0 when nothing is found, 1 when something is, 2 on a usage
error or when the package cannot be run.

Heuristics (ADR-312): a finding is a judgement of the laid-out screen, and
each rule says what it does not see:

	color-contrast   text over a plain color (the text's alpha, the opacity
	                 around it, translucent backgrounds composited); text
	                 over an image, a gradient, a blur or a filter, over an
	                 absolutely positioned box with no opaque background, or
	                 disabled, is not judged. Text only: not borders, icons
	                 or focus rings
	text-clipped     glyphs cut by overflow-hidden or by the window's edge;
	                 truncation on purpose (truncate, line-clamp), text in a
	                 scroll container, sr-only text and text under a
	                 transform are left alone
	text-overlap     two texts in normal flow whose glyphs lie over each
	                 other; overlays (absolute, fixed, sticky, portals) are
	                 not
	target-overlap   two targets in normal flow whose boxes share a quarter
	                 of the smaller one
	target-size      WCAG 2.5.8: 24x24, or no other target within a 24 px
	                 circle; inline links and disabled controls are exempt
	focus-visible    Tab is pressed through the screen (at most 100 stops);
	                 an element whose surroundings do not change when it is
	                 focused shows no indicator (text fields are not
	                 judged: their caret is the indicator)
	click-events-have-key-events
	                 an element with a click handler that cannot take the
	                 focus (not inside or around something that can)
	aria-*, role-img-alt, focus-order-semantics, set-position
	                 the names, states and references of the semantics tree

Flags:

	--preview NAME   the named screen of a Previews variable
	--size WxH       logical size of the window (default: the App's Width and
	                 Height when the package defines them, else 960x640)
	--rules LIST     only these rules, comma separated (default: all)
	--disable LIST   leave these rules out, comma separated
	-json            print the report as JSON
`

const verifyUsage = `Usage:

	tela verify [--preview NAME] [--size WxH] [--widths LIST] [--text-scale N] [--lang TAG] [--rules LIST] [--disable LIST] [-json] <pkg>

Runs the rules of "tela check" over every preview of the package (or the one
--preview names) in a matrix, one change from the baseline at a time:

	light      the baseline, at the App's size (or --size)
	dark       dark mode (not when the App's or the preview's ColorScheme fixes it)
	width N    one per --widths, at the baseline's height (default 360, 768 and 1280)
	text xN    the system's text scale, --text-scale (default 2, WCAG 1.4.4's 200%)
	rtl        the whole screen under h.Dir("rtl")
	lang TAG   the locale --lang, or, by default, the first other locale of a
	           package that links tela/i18n and has several; others skip

The focus rule (Tab through the screen) runs in light and dark only. A
variant that does not apply (a fixed color scheme, no other locale) is
listed under "skipped". Output, exit status and the heuristics are those of
"tela check" (see "tela help check").

Flags:

	--preview NAME   only the named screen of a Previews variable
	--size WxH       logical size of the baseline (default: the App's, else 960x640)
	--widths LIST    the widths to try, comma separated, in logical pixels
	--text-scale N   the text scale of the large text variant
	--lang TAG       the locale of the language variant, a BCP 47 tag
	--rules LIST     only these rules, comma separated (default: all)
	--disable LIST   leave these rules out, comma separated
	-json            print the report as JSON
`

// checkVariant is one way of mounting a screen. The generated test
// reads it from JSON (check.gen.go declares the same fields).
type checkVariant struct {
	Name      string  `json:"name"`
	Dark      bool    `json:"dark,omitempty"`
	Width     float64 `json:"width,omitempty"`     // 0: the baseline's
	TextScale float64 `json:"textScale,omitempty"` // 0: 1
	RTL       bool    `json:"rtl,omitempty"`
	Lang      string  `json:"lang,omitempty"` // "*": the first other locale
	Focus     bool    `json:"focus,omitempty"`
}

// checkSpec is what the generated test is asked to do.
type checkSpec struct {
	All      bool           `json:"all"` // every preview, not the one chosen
	Variants []checkVariant `json:"variants"`
	Rules    []string       `json:"rules,omitempty"`
	Disable  []string       `json:"disable,omitempty"`
}

// checkRaw is what the generated test writes.
type checkRaw struct {
	Names   []string `json:"names"`
	Results []struct {
		Preview    string               `json:"preview"`
		Variant    string               `json:"variant"`
		Skipped    string               `json:"skipped,omitempty"`
		Violations []telatest.Violation `json:"violations"`
	} `json:"results"`
}

// checkFinding is a violation with the previews and variants it was found in.
type checkFinding struct {
	Preview  string   `json:"preview,omitempty"`
	Rule     string   `json:"rule"`
	Impact   string   `json:"impact"`
	Message  string   `json:"message"`
	Fix      string   `json:"fix"`
	Element  string   `json:"element"`
	Src      string   `json:"src,omitempty"`
	X        float32  `json:"x"`
	Y        float32  `json:"y"`
	Width    float32  `json:"width"`
	Height   float32  `json:"height"`
	Variants []string `json:"variants"`
}

// checkReport is the output of the commands.
type checkReport struct {
	Package  string         `json:"package"`
	Previews []string       `json:"previews,omitempty"`
	Variants []string       `json:"variants"`
	Skipped  []string       `json:"skipped,omitempty"`
	Findings []checkFinding `json:"findings"`
}

// checkOptions are the parsed arguments of tela check and tela verify.
type checkOptions struct {
	renderOptions
	Verify         bool
	Widths         []float64
	TextScale      float64
	Lang           string
	Rules, Disable []string
	JSON           bool
}

// defaultWidths are the widths of tela verify: a phone, a tablet and a
// desktop window.
var defaultWidths = []float64{360, 768, 1280}

// parseCheckArgs parses the arguments of tela check (verify false) and
// tela verify.
func parseCheckArgs(verify bool, args []string) (checkOptions, error) {
	name := "check"
	if verify {
		name = "verify"
	}
	opts := checkOptions{Verify: verify, TextScale: 2}
	opts.W, opts.H, opts.Scale = defaultWidth, defaultHeight, 1
	var size, widths, rules, disable string
	pkg, err := parseFlags(name, args, func(fs *flag.FlagSet) {
		fs.StringVar(&size, "size", "", "")
		fs.StringVar(&opts.Preview, "preview", "", "")
		fs.StringVar(&rules, "rules", "", "")
		fs.StringVar(&disable, "disable", "", "")
		fs.BoolVar(&opts.JSON, "json", false, "")
		if verify {
			fs.StringVar(&widths, "widths", "", "")
			fs.Float64Var(&opts.TextScale, "text-scale", 2, "")
			fs.StringVar(&opts.Lang, "lang", "", "")
		}
	})
	if err != nil {
		return opts, err
	}
	opts.Pkg = pkg
	if err := opts.validate(size); err != nil {
		return opts, err
	}
	for _, w := range splitList(widths) {
		v, err := strconv.ParseFloat(w, 64)
		if err != nil || !(v >= 100) || v > maxLogicalSide {
			return opts, fmt.Errorf("invalid --widths entry %q: use logical widths of at least 100, such as --widths 360,768,1280", w)
		}
		opts.Widths = append(opts.Widths, v)
	}
	if verify && (!(opts.TextScale > 1) || opts.TextScale > 4) {
		return opts, fmt.Errorf("invalid --text-scale %g: use a number above 1 and at most 4, such as 2", opts.TextScale)
	}
	opts.Rules, opts.Disable = splitList(rules), splitList(disable)
	known := map[string]bool{}
	var ids []string
	for _, r := range telatest.Rules() {
		known[r.ID] = true
		ids = append(ids, r.ID)
	}
	for _, id := range slices.Concat(opts.Rules, opts.Disable) {
		if !known[id] {
			return opts, fmt.Errorf("unknown rule %q: the rules are %s", id, strings.Join(ids, ", "))
		}
	}
	return opts, nil
}

// checkVariants returns the variants an invocation runs.
func (o checkOptions) variants() []checkVariant {
	vs := []checkVariant{{Name: "light", Focus: true}, {Name: "dark", Dark: true, Focus: true}}
	if !o.Verify {
		return vs
	}
	widths := o.Widths
	if len(widths) == 0 {
		widths = defaultWidths
	}
	for _, w := range widths {
		vs = append(vs, checkVariant{Name: "width " + strconv.FormatFloat(w, 'g', -1, 64), Width: w})
	}
	vs = append(vs,
		checkVariant{Name: "text x" + strconv.FormatFloat(o.TextScale, 'g', -1, 64), TextScale: o.TextScale},
		checkVariant{Name: "rtl", RTL: true})
	lang := o.Lang
	if lang == "" {
		lang = "*"
	}
	return append(vs, checkVariant{Name: "lang", Lang: lang})
}

// runCheck implements `tela check` and `tela verify`.
func runCheck(verify bool, args []string, stdout, stderr io.Writer) int {
	name, usage := "check", checkUsage
	if verify {
		name, usage = "verify", verifyUsage
	}
	opts, err := parseCheckArgs(verify, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		fmt.Fprintf(stderr, "tela %s: %v\n\n%s", name, err, usage)
		return 2
	}
	rep, err := checkPackage(opts, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "tela %s: %v\n", name, err)
		return 2
	}
	if opts.JSON {
		b, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Fprintf(stdout, "%s\n", b)
	} else {
		writeCheckReport(stdout, name, rep)
	}
	if len(rep.Findings) > 0 {
		return 1
	}
	return 0
}

// checkPackage runs the audit of opts over its package and merges what
// the variants found.
func checkPackage(opts checkOptions, stderr io.Writer) (*checkReport, error) {
	spec := checkSpec{All: opts.Verify && opts.Preview == "", Variants: opts.variants(), Rules: opts.Rules, Disable: opts.Disable}
	js, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	ro := opts.renderOptions
	ro.Spec = string(js)
	res, err := runInPackage(modeCheck, ro, "json", stderr)
	if err != nil {
		return nil, err
	}
	var raw checkRaw
	if err := json.Unmarshal(res.Output, &raw); err != nil {
		return nil, fmt.Errorf("unreadable result of the run: %v", err)
	}
	if !spec.All && opts.Preview == "" && len(raw.Names) > 1 {
		cmd := "tela verify"
		fmt.Fprintf(stderr, "tela check: preview %q (the package also defines: %s); --preview NAME for another, %s for all of them\n",
			res.Info.Preview, strings.Join(without(raw.Names, res.Info.Preview), ", "), cmd)
	}
	return mergeCheck(opts.Pkg, &raw, spec), nil
}

// contrastColors finds the colors of a color-contrast message.
var contrastColors = regexp.MustCompile(`#[0-9a-f]{6} on #[0-9a-f]{6}`)

// mergeCheck merges the results of the variants: a finding that several
// variants share is one, listing them. Two violations are the same when
// preview, rule, element and source are; contrast findings also compare
// the colors, since light and dark differ in them (and not the ratio or
// the size class, which large text changes). A variant that does not apply
// to every preview for the same reason is listed once.
func mergeCheck(pkg string, raw *checkRaw, spec checkSpec) *checkReport {
	rep := &checkReport{Package: pkg, Findings: []checkFinding{}}
	for _, v := range spec.Variants {
		rep.Variants = append(rep.Variants, v.Name)
	}
	seenPreview := map[string]bool{}
	index := map[string]int{}
	type skip struct{ variant, why string }
	skipped := map[skip][]string{} // the previews each skip is of
	var skipOrder []skip
	for _, r := range raw.Results {
		if strings.HasPrefix(r.Variant, "lang ") {
			// The language variant says which locale it tried.
			for i, v := range rep.Variants {
				if v == "lang" {
					rep.Variants[i] = r.Variant
				}
			}
		}
		if !seenPreview[r.Preview] {
			seenPreview[r.Preview] = true
			if r.Preview != "" {
				rep.Previews = append(rep.Previews, r.Preview)
			}
		}
		if r.Skipped != "" {
			k := skip{r.Variant, r.Skipped}
			if _, ok := skipped[k]; !ok {
				skipOrder = append(skipOrder, k)
			}
			skipped[k] = append(skipped[k], r.Preview)
			continue
		}
		for _, v := range r.Violations {
			key := strings.Join([]string{r.Preview, v.Rule, v.Element, v.Source}, "\x00")
			if v.Rule == "color-contrast" {
				key += "\x00" + contrastColors.FindString(v.Message)
			}
			if i, ok := index[key]; ok {
				f := &rep.Findings[i]
				if !slices.Contains(f.Variants, r.Variant) {
					f.Variants = append(f.Variants, r.Variant)
				}
				continue
			}
			index[key] = len(rep.Findings)
			rep.Findings = append(rep.Findings, checkFinding{
				Preview: r.Preview, Rule: v.Rule, Impact: v.Impact, Message: v.Message, Fix: v.Fix,
				Element: v.Element, Src: v.Source,
				X: v.Bounds.X, Y: v.Bounds.Y, Width: v.Bounds.Width, Height: v.Bounds.Height,
				Variants: []string{r.Variant},
			})
		}
	}
	for _, k := range skipOrder {
		s := k.variant + ": " + k.why
		if ps := skipped[k]; len(ps) < len(seenPreview) {
			s = strings.Join(ps, ", ") + " / " + s
		}
		rep.Skipped = append(rep.Skipped, s)
	}
	sort.SliceStable(rep.Findings, func(i, j int) bool {
		a, b := rep.Findings[i], rep.Findings[j]
		if a.Preview != b.Preview {
			return a.Preview < b.Preview
		}
		af, al := srcPosition(a.Src)
		bf, bl := srcPosition(b.Src)
		if af != bf {
			return af < bf
		}
		return al < bl
	})
	return rep
}

// srcPosition splits "dir/file.go:42" into the file and the line.
func srcPosition(src string) (file string, line int) {
	i := strings.LastIndexByte(src, ':')
	if i < 0 {
		return src, 0
	}
	line, _ = strconv.Atoi(src[i+1:])
	return src[:i], line
}

// writeCheckReport prints the report for people and agents.
func writeCheckReport(w io.Writer, name string, rep *checkReport) {
	var b bytes.Buffer
	for _, f := range rep.Findings {
		where := f.Src
		if where == "" {
			where = rep.Package
			if f.Preview != "" {
				where += " [" + f.Preview + "]"
			}
		}
		fmt.Fprintf(&b, "%s: %s (%s): %s\n", where, f.Rule, f.Impact, f.Message)
		fmt.Fprintf(&b, "    element: %s\n", f.Element)
		fmt.Fprintf(&b, "    fix: %s\n", f.Fix)
		in := strings.Join(f.Variants, ", ")
		if f.Preview != "" {
			in = f.Preview + " / " + in
		}
		fmt.Fprintf(&b, "    in: %s\n", in)
	}
	for _, s := range rep.Skipped {
		fmt.Fprintf(&b, "skipped: %s\n", s)
	}
	switch n := len(rep.Findings); n {
	case 0:
		fmt.Fprintf(&b, "tela %s: nothing found in %s (%s)\n", name, rep.Package, strings.Join(rep.Variants, ", "))
	default:
		noun := "findings"
		if n == 1 {
			noun = "finding"
		}
		fmt.Fprintf(&b, "tela %s: %d %s in %s (%s)\n", name, n, noun, rep.Package, strings.Join(rep.Variants, ", "))
	}
	w.Write(b.Bytes())
}
