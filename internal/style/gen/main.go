// Command gen generates internal/style/theme_gen.go, the default theme,
// from the Tailwind v4 theme source vendored in tailwind/theme.css
// (ADR-018, ADR-037).
//
// It is run by "go generate ./internal/style" (see generate.go there) from
// the internal/style directory. The output is deterministic and gofmt'ed.
// OKLCH colors are converted to sRGB with gfx.OKLCH, which maps colors
// outside the sRGB gamut with the CSS Color 4 algorithm.
//
// Flags:
//
//	-theme path   Tailwind theme.css (default gen/tailwind/theme.css)
//	-out path     output file (default theme_gen.go)
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	themePath := flag.String("theme", "gen/tailwind/theme.css", "Tailwind theme.css to read")
	outPath := flag.String("out", "theme_gen.go", "Go file to write")
	flag.Parse()

	src, err := os.ReadFile(*themePath)
	if err != nil {
		fatal(fmt.Errorf("reading the Tailwind theme: %v (run the generator from internal/style, e.g. with go generate)", err))
	}
	out, err := build(string(src))
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*outPath, out, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen:", err)
	os.Exit(1)
}
