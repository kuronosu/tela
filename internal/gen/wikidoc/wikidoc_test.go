package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureSpecs are the packages of testdata, a module of its own.
func fixtureSpecs() []pkgSpec {
	return []pkgSpec{
		{Dir: "fix", ImportPath: "example.com/fix/fix", Page: "API-Reference-fix"},
		{Dir: "other", ImportPath: "example.com/fix/other", Page: "API-Reference-other"},
	}
}

func generateFixture(t *testing.T) [][]byte {
	t.Helper()
	pages, err := generate("testdata", "example.com/fix", fixtureSpecs())
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

// TestGolden compares the pages of the fixture with testdata/*.golden.md.
// TELA_UPDATE=1 rewrites them (review the diff).
func TestGolden(t *testing.T) {
	pages := generateFixture(t)
	for i, spec := range fixtureSpecs() {
		path := filepath.Join("testdata", spec.Page+".golden.md")
		if os.Getenv("TELA_UPDATE") == "1" {
			if err := os.WriteFile(path, pages[i], 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (TELA_UPDATE=1 go test creates the golden)", err)
		}
		if !bytes.Equal(pages[i], want) {
			t.Errorf("%s differs from %s; run TELA_UPDATE=1 go test ./internal/gen/wikidoc and review the diff\ngot:\n%s", spec.Page, path, pages[i])
		}
	}
}

// TestDeterministic generates twice and expects identical bytes.
func TestDeterministic(t *testing.T) {
	a, b := generateFixture(t), generateFixture(t)
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			t.Errorf("page %d differs between two runs", i)
		}
	}
}

// TestRepositoryPagesAreUpToDate fails when docs/wiki holds a reference
// page that no longer matches the doc comments.
func TestRepositoryPagesAreUpToDate(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	modPath, err := modulePath(root)
	if err != nil {
		t.Fatal(err)
	}
	specs := defaultSpecs(modPath)
	pages, err := generate(root, modPath, specs)
	if err != nil {
		t.Fatal(err)
	}
	for i, spec := range specs {
		got, err := os.ReadFile(filepath.Join(root, spec.Out))
		if err != nil {
			t.Fatalf("%v: run \"go generate\" in the module root", err)
		}
		if !bytes.Equal(got, pages[i]) {
			t.Errorf("%s is stale: run \"go generate\" in the module root and commit the result", spec.Out)
		}
	}
}

func TestAnchorOf(t *testing.T) {
	for heading, want := range map[string]string{
		"func UseState":            "func-usestate",
		"func (*Setter) Set":       "func-setter-set",
		"type App":                 "type-app",
		"API reference: package h": "api-reference-package-h",
		"Actions and auto-wait":    "actions-and-auto-wait",
	} {
		if got := anchorOf(heading); got != want {
			t.Errorf("anchorOf(%q) = %q, want %q", heading, got, want)
		}
	}
}

func TestEscape(t *testing.T) {
	for in, want := range map[string]string{
		"TELA_UPDATE=1": "TELA_UPDATE=1",
		"_lead":         `\_lead`,
		"a <b> & *c*":   `a \<b\> \& \*c\*`,
		"[x]":           `\[x\]`,
	} {
		if got := escape(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
	if got := plainMarkdown("run `tela inspect` or `x"); got != "run `tela inspect` or \\`x" {
		t.Errorf("plainMarkdown = %q", got)
	}
}

func TestCodeLang(t *testing.T) {
	for in, want := range map[string]string{
		"w := fix.NewWidget()":        "go",
		"tela snapshot -o out.png .":  "sh",
		"TELA_UPDATE=1 go test ./...": "sh",
		"- button \"Save\"\n- link":   "yaml",
		"- heading (x)":               "go",
		"go func() {":                 "go",
		"go run ./cmd/tela":           "sh",
	} {
		if got := codeLang(in); got != want {
			t.Errorf("codeLang(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDeprecatedAnchorClash: a deprecated function whose name differs from
// another's only in case (UseId and UseID) gets a heading of its own, and
// links to it go there.
func TestDeprecatedAnchorClash(t *testing.T) {
	dir := t.TempDir()
	src := `package dep

// UseID is the new name.
func UseID() string { return "" }

// UseId is the old name.
//
// Deprecated: use [UseID].
func UseId() string { return "" }

// Make is deprecated, without a clash.
//
// Deprecated: use [UseID].
func Make() {}
`
	if err := os.WriteFile(filepath.Join(dir, "dep.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/dep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pages, err := generate(dir, "example.com/dep", []pkgSpec{{Dir: ".", ImportPath: "example.com/dep", Page: "API-Reference-dep"}})
	if err != nil {
		t.Fatal(err)
	}
	page := string(pages[0])
	for _, want := range []string{"### func UseID\n", "### func UseId (deprecated)\n", "### func Make\n", "[`UseId`](#func-useid-deprecated)"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page does not contain %q:\n%s", want, page)
		}
	}
}
