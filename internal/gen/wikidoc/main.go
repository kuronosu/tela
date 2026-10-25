// Command wikidoc generates the API reference pages of the GitHub wiki
// (docs/wiki/API-Reference-<pkg>.md) from the doc comments of Tela's
// public packages, so the wiki never drifts from pkg.go.dev.
//
// For every exported constant, variable, type, function and method it
// writes the declaration in a Go code block followed by its doc comment
// converted to Markdown (headings, lists, code blocks, and [Symbol] doc
// links to anchors of the pages or to pkg.go.dev), grouped as "go doc"
// groups them, with an index at the top. Examples from the package's
// _test.go files are included.
//
// Types that the public package defines as aliases of types in the
// module's internal packages (type MouseEvent = core.MouseEvent) are
// expanded with the exported fields and methods of the aliased type,
// which pkg.go.dev cannot show.
//
// It uses only the standard library (go/parser, go/doc, go/doc/comment,
// go/printer), reads the sources with the linux/amd64 build context, and
// its output is deterministic. It is run by "go generate" in the module
// root (see events.go there).
//
// Flags:
//
//	-root dir     the module root (default ".")
//	-dir dir      generate one package only: its directory, relative to -root
//	-import path  with -dir: the import path of the package
//	-out file     with -dir: the file to write (default: standard output)
//
// Without -dir it writes the pages of tela, tela/h, tela/aria, tela/telatest
// and tela/kit into docs/wiki.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", ".", "the module root")
	dir := flag.String("dir", "", "generate one package only: its directory, relative to -root")
	imp := flag.String("import", "", "with -dir: the import path of the package")
	out := flag.String("out", "", "with -dir: the file to write (default: standard output)")
	flag.Parse()

	modPath, err := modulePath(*root)
	if err != nil {
		fatal(err)
	}
	specs := defaultSpecs(modPath)
	if *dir != "" {
		if *imp == "" {
			fatal(fmt.Errorf("-dir needs -import, the import path of the package"))
		}
		specs = []pkgSpec{{Dir: *dir, ImportPath: *imp, Out: *out, Page: pageName(*imp)}}
	}
	pages, err := generate(*root, modPath, specs)
	if err != nil {
		fatal(err)
	}
	for i, spec := range specs {
		if spec.Out == "" {
			os.Stdout.Write(pages[i])
			continue
		}
		path := spec.Out
		if !filepath.IsAbs(path) && *dir == "" {
			path = filepath.Join(*root, path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fatal(err)
		}
		if err := os.WriteFile(path, pages[i], 0o644); err != nil {
			fatal(err)
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "wikidoc:", err)
	os.Exit(1)
}
