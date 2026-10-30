package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

// mount opens a root with the settings of App (size, theme, root class).
// Dev mode is on, so any dev message (an unknown class, a hook misuse, a
// variant the active style does not define) fails the test.
func mount(t *testing.T, root tela.Component, opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.Mount(t, root, append([]telatest.Option{telatest.FromApp(App)}, opts...)...)
}

// schemes are the two looks every golden exists in.
var schemes = []struct {
	name string
	opts []telatest.Option
}{
	{"light", nil},
	{"dark", []telatest.Option{telatest.Dark()}},
}

// The elements the tests use, by role and accessible name, as a person with
// a screen reader finds them.
var (
	rounded    = telatest.ByRole("radio", "Rounded")
	darkSwitch = telatest.ByRole("switch", "Dark mode")
	// languageMenu is the header's Select, the trigger's name is the current
	// language, then the catalog's word for "Language".
	languageMenu = telatest.ByTestID("language")
)

func tabFor(label string) telatest.Locator { return telatest.ByRole("tab", label) }

func heading(text string) telatest.Locator { return telatest.ByRole("heading", text) }

// TestPreviewsRenderClean mounts every preview, in both schemes, with dev
// mode on: no unknown class, hook misuse or undefined variant on any page.
func TestPreviewsRenderClean(t *testing.T) {
	for _, name := range Previews.Names() {
		for _, s := range schemes {
			t.Run(name+"/"+s.name, func(t *testing.T) {
				a := mount(t, Previews[name], s.opts...)
				if got := a.Count(telatest.ByRole("heading", "")); got == 0 {
					t.Errorf("preview %q has no headings", name)
				}
			})
		}
	}
}

// TestEveryPageInBothStyles opens the whole gallery on each page in each
// style and scheme: the second style must define every variant and size
// the pages use. The Introduction draws its own heading, a headline.
func TestEveryPageInBothStyles(t *testing.T) {
	for _, p := range sidebar() {
		for _, style := range []string{"default", "rounded"} {
			t.Run(p.Name+"/"+style, func(t *testing.T) {
				a := mount(t, Gallery{Page: p.Name, Style: style}, telatest.Dark())
				a.Expect(tabFor(p.Label), telatest.Selected(true))
				if p.Bare {
					a.ExpectRole("heading", "Hello, kit.")
				} else {
					a.Expect(heading(p.Label), telatest.HasName(p.Label))
				}
			})
		}
	}
}

// TestEveryPageInEveryLanguage opens every page in Spanish and in Arabic
// (right to left) with dev mode on: nothing breaks when the language or the
// direction changes, whatever the page.
func TestEveryPageInEveryLanguage(t *testing.T) {
	for _, p := range sidebar() {
		for _, tag := range languages[1:] {
			t.Run(p.Name+"/"+tag, func(t *testing.T) {
				a := mount(t, Gallery{Page: p.Name, Locale: tag})
				a.Expect(tabFor(p.Label), telatest.Selected(true))
			})
		}
	}
}

// TestPreviewsMatchPages checks that Previews has one entry per page, the
// gallery's three and the Internationalization page in the other languages,
// that the home page is the first of the sidebar, and that the components are
// alphabetical.
func TestPreviewsMatchPages(t *testing.T) {
	want := len(sidebar()) + 3 + len(languages) - 1
	if got := len(Previews); got != want {
		t.Errorf("Previews has %d entries, want %d (one per page, Gallery, Gallery Rounded, Gallery Arabic and Internationalization in each other language)", got, want)
	}
	if sidebar()[0].Name != home {
		t.Errorf("the sidebar starts with %q, want the home page %q", sidebar()[0].Name, home)
	}
	for _, p := range sidebar() {
		if _, ok := Previews[p.Name]; !ok {
			t.Errorf("no preview named %q", p.Name)
		}
	}
	labels := make([]string, len(pages))
	for i, p := range pages {
		labels[i] = p.Label
	}
	if !sort.StringsAreSorted(labels) {
		t.Errorf("the sidebar is not alphabetical: %v", labels)
	}
	seen := map[string]bool{}
	for _, p := range pages {
		if seen[p.Name] {
			t.Errorf("two pages are named %q", p.Name)
		}
		seen[p.Name] = true
	}
}

// TestEveryKitComponentHasAPage reads the sources of package kit and checks
// that every exported component (a type with a Build method) is on a page:
// a component added to the kit without a page fails here.
func TestEveryKitComponentHasAPage(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir("../../kit")
	if err != nil {
		t.Fatal(err)
	}
	var components []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join("../../kit", e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Build" || !fn.Name.IsExported() {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			switch r := recv.(type) {
			case *ast.IndexExpr:
				recv = r.X
			case *ast.IndexListExpr:
				recv = r.X
			}
			if id, ok := recv.(*ast.Ident); ok && id.IsExported() {
				components = append(components, id.Name)
			}
		}
	}
	if len(components) < 30 {
		t.Fatalf("found only %d components in kit: %v", len(components), components)
	}
	// StyleProvider is not a widget: the gallery's header uses it.
	for _, name := range components {
		if name == "StyleProvider" {
			continue
		}
		found := false
		for _, p := range pages {
			for _, code := range strings.Split(p.Code, ",") {
				code = strings.TrimSpace(code)
				code = strings.TrimPrefix(code, "kit.")
				code = strings.TrimSuffix(code, "[K]")
				code = strings.TrimSuffix(code, "[T]")
				if code == name {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("kit.%s has no page in the gallery: add it to pages.go with a demo", name)
		}
	}
}
