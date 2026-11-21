package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// genFeatures says what the generated code of tela snapshot, inspect and
// mcp may use. The package is compiled against the tela its own module
// requires, which may be older than this binary (ADR-194, ADR-300), so
// the generated source asks first.
type genFeatures struct {
	// Settle: telatest mounts the app as a test does and settles it
	// (Mount with AllowDev and NoSettle, and App.LayoutJSON), so that
	// tela snapshot and inspect render the screen at rest. An older tela
	// renders one frame with telatest.Render, LayoutJSON and AriaSnapshot
	// (ADR-059: the commands work on any module that requires tela).
	Settle bool
	// DevTools: telatest has Instrument, AriaSnapshotSources, Components,
	// ComponentOf and Frames (ADR-299), and so the source lines of
	// elements, the state and perf tools and the preference tool.
	DevTools bool
	// Film: telatest has App.Film, so tela snapshot --film and the film
	// tool of tela mcp can capture frames.
	Film bool
	// I18N: the package links tela/i18n, so a locale preference can act.
	I18N bool
	// PendingKinds: App.Pending gives telatest.Pending items with a kind
	// (ADR-308), so tela mcp tells quiet work (a caret blink) from work
	// that can change the screen without reading their text.
	PendingKinds bool
	// Scrollables: telatest has App.Scrollables, so tela mcp's scroll
	// without an element scrolls the largest scroll container.
	Scrollables bool
}

// detectFeatures asks the go command about the package in dir: the
// packages it links (tela/i18n) and the telatest of its module.
func detectFeatures(goBin, dir string) genFeatures {
	cmd := exec.Command(goBin, "list", "-e", "-deps", "-f", "{{.ImportPath}}|{{.Dir}}", ".", "github.com/kuronosu/tela/telatest")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return genFeatures{}
	}
	var f genFeatures
	for _, line := range strings.Split(out.String(), "\n") {
		path, pkgDir, ok := strings.Cut(strings.TrimSpace(line), "|")
		switch {
		case !ok:
		case path == "github.com/kuronosu/tela/i18n":
			f.I18N = true
		case path == "github.com/kuronosu/tela/telatest":
			f.Settle = hasFunc(pkgDir, "func NoSettle() Option") && hasFunc(pkgDir, "func (a *App) LayoutJSON(") && hasFunc(pkgDir, "func AllowDev(")
			f.DevTools = hasFunc(pkgDir, "func Instrument() Option")
			f.Film = hasFunc(pkgDir, "func (a *App) Film(")
			f.PendingKinds = hasFunc(pkgDir, "func (a *App) Pending() []Pending")
			f.Scrollables = hasFunc(pkgDir, "func (a *App) Scrollables() []uint64")
		}
	}
	return f
}

// hasFunc reports whether a non-test Go file of dir contains decl.
func hasFunc(dir, decl string) bool {
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if b, err := os.ReadFile(name); err == nil && bytes.Contains(b, []byte(decl)) {
			return true
		}
	}
	return false
}
