package main

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
)

const (
	// tailwindVersion is the version of the vendored theme.css (ADR-037).
	tailwindVersion = "4.3.3"

	// rootFontSize is the pixel size of 1rem (ARCHITECTURE section 10).
	rootFontSize = 16
)

// fontStacks replaces the font stacks of theme.css. Tailwind's stacks list
// system fonts (-apple-system, Segoe UI, Roboto...) that differ per OS and
// are not installed in tests; Tela draws the same pixels everywhere, so
// the three families map to its embedded fonts (ADR-008, ADR-038): Inter
// for sans, JetBrains Mono for mono. Tela embeds no serif font; "serif"
// is the generic family, which a Collection resolves to a registered or
// system font. Emoji and script fallbacks come from the text layer's
// fallback order (ADR-031), so they are not repeated in the stacks.
//
// Every font family in theme.css must have an entry here: a new one is a
// decision to make, not a value to copy.
var fontStacks = map[string][]string{
	"sans":  {"Inter"},
	"mono":  {"JetBrains Mono"},
	"serif": {"serif"},
}

// field describes one Theme map field filled from a theme.css namespace.
type field struct {
	Name   string // Theme field name
	GoType string // element type of the map
	Prefix string // "--color-"
	// Value converts a CSS value to a Go expression of GoType.
	Value func(string) (string, error)
}

// fields lists the namespaces in the order they are emitted. Prefixes are
// matched longest first, so --text-shadow-* is not read as --text-*.
var fields = []*field{
	{Name: "Colors", GoType: "gfx.Color", Prefix: "--color-", Value: colorExpr},
	{Name: "Breakpoints", GoType: "Length", Prefix: "--breakpoint-", Value: lengthExpr},
	{Name: "Containers", GoType: "Length", Prefix: "--container-", Value: lengthExpr},
	{Name: "Text", GoType: "TextSize", Prefix: "--text-"}, // paired with --line-height, see build
	{Name: "FontWeights", GoType: "float32", Prefix: "--font-weight-", Value: numberExpr},
	{Name: "Tracking", GoType: "Length", Prefix: "--tracking-", Value: lengthExpr},
	{Name: "Leading", GoType: "float32", Prefix: "--leading-", Value: numberExpr},
	{Name: "Radius", GoType: "Length", Prefix: "--radius-", Value: lengthExpr},
	{Name: "Shadows", GoType: "[]Shadow", Prefix: "--shadow-", Value: shadowExpr},
	{Name: "InsetShadows", GoType: "[]Shadow", Prefix: "--inset-shadow-", Value: shadowExpr},
	{Name: "DropShadows", GoType: "[]Shadow", Prefix: "--drop-shadow-", Value: shadowExpr},
	{Name: "TextShadows", GoType: "[]Shadow", Prefix: "--text-shadow-", Value: shadowExpr},
	{Name: "Blur", GoType: "Length", Prefix: "--blur-", Value: lengthExpr},
	{Name: "Perspective", GoType: "Length", Prefix: "--perspective-", Value: lengthExpr},
	{Name: "Aspect", GoType: "float32", Prefix: "--aspect-", Value: numberExpr},
	{Name: "Ease", GoType: "CubicBezier", Prefix: "--ease-", Value: easeExpr},
	{Name: "Fonts", GoType: "[]string", Prefix: "--font-"}, // replaced by fontStacks, see build
	{Name: "Animate", GoType: "string", Prefix: "--animate-", Value: func(s string) (string, error) {
		return fmt.Sprintf("%q", s), nil
	}},
}

// deprecatedKeys maps the bare variables of Tailwind's deprecated block
// ("rounded", "shadow", "blur", "drop-shadow" utilities) to the field that
// holds them under the key "DEFAULT".
var deprecatedKeys = map[string]string{
	"--radius":      "Radius",
	"--shadow":      "Shadows",
	"--drop-shadow": "DropShadows",
	"--blur":        "Blur",
}

// skipped lists variables the generator reads but does not emit, with the
// reason, so that every variable of theme.css is accounted for.
var skipped = map[string]string{
	"--default-font-feature-settings":        "Tela themes carry no font feature settings; the text layer applies the font's defaults",
	"--default-font-variation-settings":      "Tela themes carry no font variation settings; weights and optical size come from the style",
	"--default-mono-font-feature-settings":   "same as --default-font-feature-settings",
	"--default-mono-font-variation-settings": "same as --default-font-variation-settings",
	"--max-width-prose":                      "ch units are approximate (ARCHITECTURE section 10) and max-w-prose is not a theme key; the class parser defines it",
}

// build parses theme.css and returns the formatted Go source of
// theme_gen.go.
func build(themeCSS string) ([]byte, error) {
	vars, err := parseThemeCSS(themeCSS)
	if err != nil {
		return nil, err
	}

	type entry struct{ Key, Expr string }
	entries := map[string][]entry{}
	seen := map[string]bool{}
	add := func(f, key, expr, name string) error {
		id := f + "/" + key
		if seen[id] {
			return fmt.Errorf("theme.css: %s declared twice (as %s %q)", name, f, key)
		}
		seen[id] = true
		entries[f] = append(entries[f], entry{key, expr})
		return nil
	}

	var (
		spacing, transDuration, timing string
		defaultFont, defaultMono       string
		textSize                       = map[string]string{}
		textLine                       = map[string]string{}
		textOrder                      []string
		fontFamilies                   []string
	)

	for _, v := range vars {
		name := v.Name
		if _, ok := skipped[name]; ok {
			continue
		}
		switch name {
		case "--spacing":
			if spacing, err = lengthExpr(v.Value); err != nil {
				return nil, fmt.Errorf("%s: %v", name, err)
			}
			continue
		case "--default-transition-duration":
			if transDuration, err = durationExpr(v.Value); err != nil {
				return nil, fmt.Errorf("%s: %v", name, err)
			}
			continue
		case "--default-transition-timing-function":
			b, err := cubicBezier(v.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", name, err)
			}
			timing = cubicBezierLit(b)
			continue
		case "--default-font-family", "--default-mono-font-family":
			// --theme(--font-sans, initial)
			ref, ok := strings.CutPrefix(v.Value, "--theme(--font-")
			key, _, ok2 := strings.Cut(ref, ",")
			if !ok || !ok2 {
				return nil, fmt.Errorf("%s: unsupported value %q; expected --theme(--font-<name>, initial)", name, v.Value)
			}
			if name == "--default-font-family" {
				defaultFont = key
			} else {
				defaultMono = key
			}
			continue
		}
		if fname, ok := deprecatedKeys[name]; ok {
			f := fieldByName(fname)
			expr, err := f.Value(v.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", name, err)
			}
			if err := add(fname, "DEFAULT", expr, name); err != nil {
				return nil, err
			}
			continue
		}

		f := matchField(name)
		if f == nil {
			return nil, fmt.Errorf("theme.css: unhandled variable %s: add its namespace to fields in internal/style/gen/generate.go, or list it in skipped with a reason", name)
		}
		key := strings.TrimPrefix(name, f.Prefix)
		switch f.Name {
		case "Text":
			if k, ok := strings.CutSuffix(key, "--line-height"); ok {
				ratioV, err := ratio(v.Value)
				if err != nil {
					return nil, fmt.Errorf("%s: %v", name, err)
				}
				textLine[k] = floatLit(ratioV)
				continue
			}
			if strings.Contains(key, "--") {
				return nil, fmt.Errorf("theme.css: unhandled variable %s: only --text-*--line-height is supported", name)
			}
			e, err := lengthExpr(v.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", name, err)
			}
			textSize[key] = e
			textOrder = append(textOrder, key)
		case "Fonts":
			if strings.Contains(key, "--") {
				return nil, fmt.Errorf("theme.css: unhandled variable %s: font feature and variation settings are not supported", name)
			}
			if _, ok := fontStacks[key]; !ok {
				return nil, fmt.Errorf("theme.css: font family %q has no entry in fontStacks; choose which embedded font it maps to", key)
			}
			fontFamilies = append(fontFamilies, key)
			if err := add("Fonts", key, stringsLit(fontStacks[key]), name); err != nil {
				return nil, err
			}
		default:
			expr, err := f.Value(v.Value)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", name, err)
			}
			if err := add(f.Name, key, expr, name); err != nil {
				return nil, err
			}
		}
	}

	for _, k := range textOrder {
		lh, ok := textLine[k]
		if !ok {
			return nil, fmt.Errorf("theme.css: --text-%s has no --text-%s--line-height", k, k)
		}
		if err := add("Text", k, fmt.Sprintf("{Size: %s, LineHeight: %s}", textSize[k], lh), "--text-"+k); err != nil {
			return nil, err
		}
		delete(textLine, k)
	}
	for k := range textLine {
		return nil, fmt.Errorf("theme.css: --text-%s--line-height has no --text-%s", k, k)
	}
	for k := range fontStacks {
		found := false
		for _, f := range fontFamilies {
			found = found || f == k
		}
		if !found {
			return nil, fmt.Errorf("fontStacks has %q but theme.css declares no --font-%s", k, k)
		}
	}
	for _, c := range []struct{ what, v string }{
		{"--spacing", spacing},
		{"--default-transition-duration", transDuration},
		{"--default-transition-timing-function", timing},
		{"--default-font-family", defaultFont},
		{"--default-mono-font-family", defaultMono},
	} {
		if c.v == "" {
			return nil, fmt.Errorf("theme.css: %s is missing", c.what)
		}
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by internal/style/gen from gen/tailwind/theme.css (Tailwind CSS v%s). DO NOT EDIT.\n", tailwindVersion)
	b.WriteString("// Run \"go generate ./internal/style\" to regenerate it.\n\n")
	b.WriteString("package style\n\n")
	b.WriteString("import (\n\t\"time\"\n\n\t\"github.com/kuronosu/tela/internal/gfx\"\n)\n\n")
	b.WriteString("// newDefaultTheme builds the Tailwind v4 default theme. It allocates a new\n")
	b.WriteString("// theme on every call, so the result can be modified freely.\n")
	b.WriteString("func newDefaultTheme() Theme {\n\treturn Theme{\n")
	fmt.Fprintf(&b, "\t\tRootFontSize: %d,\n", rootFontSize)
	fmt.Fprintf(&b, "\t\tSpacing: %s,\n", spacing)
	for _, f := range fields {
		es := entries[f.Name]
		if len(es) == 0 {
			return nil, fmt.Errorf("theme.css declares no %s variables", f.Prefix+"*")
		}
		fmt.Fprintf(&b, "\t\t%s: map[string]%s{\n", f.Name, f.GoType)
		for _, e := range es {
			expr := e.Expr
			if strings.HasPrefix(expr, f.GoType+"{") {
				expr = expr[len(f.GoType):] // elide the type inside the map literal
			}
			fmt.Fprintf(&b, "\t\t\t%q: %s,\n", e.Key, expr)
		}
		b.WriteString("\t\t},\n")
	}
	fmt.Fprintf(&b, "\t\tDefaultTransition: Transition{Duration: %s, Timing: %s},\n", transDuration, timing)
	fmt.Fprintf(&b, "\t\tDefaultFont: %q,\n", defaultFont)
	fmt.Fprintf(&b, "\t\tDefaultMonoFont: %q,\n", defaultMono)
	b.WriteString("\t}\n}\n")

	out, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated code: %v", err)
	}
	return out, nil
}

// matchField returns the field with the longest prefix of name.
func matchField(name string) *field {
	var best *field
	for _, f := range fields {
		if strings.HasPrefix(name, f.Prefix) && (best == nil || len(f.Prefix) > len(best.Prefix)) {
			best = f
		}
	}
	return best
}

func fieldByName(name string) *field {
	for _, f := range fields {
		if f.Name == name {
			return f
		}
	}
	panic("gen: no field " + name)
}
