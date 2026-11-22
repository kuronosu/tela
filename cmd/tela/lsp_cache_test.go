package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/lint"
)

// aliasModule writes a module outside the repository (it requires tela
// through a replace directive) whose package theme declares class aliases
// in two files, and returns its folder and the folder of its package ui,
// which has no file on disk: the test's buffer is its only one.
func aliasModule(t *testing.T) (root, ui string) {
	t.Helper()
	root = t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	files := map[string]string{
		"go.mod": fmt.Sprintf("module example.com/aliases\n\ngo 1.27\n\nrequire github.com/kuronosu/tela v0.0.0\n\nreplace github.com/kuronosu/tela => %s\n",
			filepath.ToSlash(repoRoot(t))),
		"theme/theme.go": themeFile("brand-btn"),
		"theme/more.go":  "package theme\n\nimport \"github.com/kuronosu/tela\"\n\nvar Extra = tela.App{ClassAliases: map[string]string{\"brand-card\": \"rounded-xl p-4\"}}\n",
		"theme/card.go":  "package theme\n\n// Card is the class of a card.\nconst Card = \"rounded-xl flex\"\n",
		"ui/doc.go":      "// Package ui is the test's buffer.\npackage ui\n",
	}
	for name, src := range files {
		writeTestFile(t, root, name, src)
	}
	tidy := exec.Command("go", "mod", "tidy")
	tidy.Dir = root
	tidy.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOSUMDB=off")
	if out, err := tidy.CombinedOutput(); err != nil {
		offlineUnresolved(t, "cannot resolve the dependencies of a module outside the repository offline (go mod tidy: %v):\n%s", err, out)
	}
	return root, filepath.Join(root, "ui")
}

func themeFile(alias string) string {
	return "// Package theme declares the aliases of the app.\npackage theme\n\nimport \"github.com/kuronosu/tela\"\n\nvar App = tela.App{ClassAliases: map[string]string{\"" + alias + "\": \"px-4 py-2\"}}\n"
}

func writeTestFile(t *testing.T, root, name, src string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLSPContextCacheSeesChanges edits, deletes and restores on disk the
// files of a package that declares aliases and that the buffer imports,
// which the server keeps checked between lints (internal/lint's Cache), and
// checks after each change that the diagnostics of the buffer are a fresh
// run's.
func TestLSPContextCacheSeesChanges(t *testing.T) {
	t.Parallel()
	root, ui := aliasModule(t)
	path := filepath.Join(ui, "ui.go")
	uri := fileURI(path)
	text := `package ui

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"

	"example.com/aliases/theme"
)

func Card() tela.Node { return Div(Class("brand-btn brand-card flex"), Div(Class(theme.Card))) }
`
	c := startLSP(t)
	c.initialize()
	version := 1
	// relint makes the server lint the buffer again and returns the messages
	// of its diagnostics once they are those of a fresh run.
	relint := func(what string, want []string) {
		t.Helper()
		fresh := freshMessages(t, ui, path, text)
		ok := len(fresh) == len(want)
		for i := range want {
			ok = ok && strings.Contains(fresh[i], want[i])
		}
		if !ok {
			t.Fatalf("%s: a fresh run finds %q, the test wants %q", what, fresh, want)
		}
		if version == 1 {
			c.open(uri, text)
		} else {
			c.change(uri, version, text)
		}
		version++
		c.diagnostics(uri, func(d []lspDiagnostic) bool {
			var got []string
			for _, x := range d {
				got = append(got, x.Message)
			}
			slices.Sort(got)
			return slices.Equal(got, fresh)
		})
	}
	unknown := func(class string) string {
		return fmt.Sprintf("unknown class %q", class)
	}
	relint("first lint", nil)

	// The buffer changes, the context does not: it is the cached one.
	orig := text
	text = strings.Replace(text, "flex", "flex bg-rde-500", 1)
	relint("buffer edited", []string{unknown("bg-rde-500")})
	text = orig

	writeTestFile(t, root, "theme/theme.go", themeFile("brand-button"))
	relint("alias renamed on disk", []string{unknown("brand-btn")})

	if err := os.Remove(filepath.Join(root, "theme", "more.go")); err != nil {
		t.Fatal(err)
	}
	relint("file of aliases deleted", []string{unknown("brand-btn"), unknown("brand-card")})

	writeTestFile(t, root, "theme/theme.go", themeFile("brand-btn"))
	writeTestFile(t, root, "theme/more.go", "package theme\n\nimport \"github.com/kuronosu/tela\"\n\nvar Extra = tela.App{ClassAliases: map[string]string{\"brand-card\": \"rounded-xl p-4\"}}\n")
	relint("both restored", nil)
}

// freshMessages lints the buffer as the server does, without a cache, and
// returns the messages of the findings in it, sorted.
func freshMessages(t *testing.T, dir, path, text string) []string {
	t.Helper()
	res, err := lint.Run(lint.Options{
		Dir: dir, Patterns: []string{"."}, Context: moduleContext(dir),
		Overlay: map[string][]byte{path: []byte(text)}, AbsPaths: true, Lenient: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range res.Diagnostics {
		if d.File == path { // what the server publishes for the buffer
			out = append(out, d.Message)
		}
	}
	slices.Sort(out)
	return out
}
