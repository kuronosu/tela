package icongen

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/kuronosu/tela/internal/testutil/golden"
)

func TestMinify(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{
			name: "lucide",
			in: `<!-- @license lucide-static v1.61.0 - ISC -->
<svg
  class="lucide lucide-check"
  xmlns="http://www.w3.org/2000/svg"
  width="24"
  viewBox="0 0 24 24"
>
  <path d="M20 6 9 17l-5-5" />
</svg>
`,
			want: `<svg xmlns="http://www.w3.org/2000/svg" width="24" viewBox="0 0 24 24"><path d="M20 6 9 17l-5-5"/></svg>`,
		},
		{
			name: "declaration and doctype",
			in: `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "http://www.w3.org/Graphics/SVG/1.1/DTD/svg11.dtd">
<svg xmlns="http://www.w3.org/2000/svg" version="1.1" baseProfile="full" viewBox="0 0 2 2"><rect width="1" height="1"/></svg>`,
			want: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><rect width="1" height="1"/></svg>`,
		},
		{
			name: "editor namespaces and metadata",
			in: `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" xmlns:rdf="r" viewBox="0 0 2 2" inkscape:version="1">
  <title>Hi</title><desc>there</desc><metadata><rdf:RDF/></metadata>
  <inkscape:foo><path d="M0 0"/></inkscape:foo>
  <defs/>
  <path d="M0 0h1"/>
</svg>`,
			want: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><path d="M0 0h1"/></svg>`,
		},
		{
			name: "path whitespace",
			in:   "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0\n0  2   2\"><path d=\"M 0,0\n\t L 2,2\n\"/></svg>",
			want: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><path d="M 0,0 L 2,2"/></svg>`,
		},
		{
			name: "ids: kept when referenced",
			in: `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" id="root" viewBox="0 0 4 4">
<defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient><clipPath id="c"><rect width="2" height="2"/></clipPath></defs>
<rect id="r" width="2" height="2" fill="url(#g)" style="clip-path:url(#c)"/><use xlink:href="#r" x="2"/><rect id="gg" width="1" height="1"/></svg>`,
			want: `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 4 4"><defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient><clipPath id="c"><rect width="2" height="2"/></clipPath></defs><rect id="r" width="2" height="2" fill="url(#g)" style="clip-path:url(#c)"/><use xlink:href="#r" x="2"/><rect width="1" height="1"/></svg>`,
		},
		{
			name: "unused xlink declaration",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 2 2"><path d="M0 0"/></svg>`,
			want: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><path d="M0 0"/></svg>`,
		},
		{
			name: "style sheet keeps classes and ids",
			in: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2">
  <style>
    .a { fill: red }
    #b { fill: blue }
  </style>
  <rect class="a" width="1" height="1"/>
  <rect id="b" width="1" height="1" x="1"/>
</svg>`,
			want: "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 2 2\"><style>\n    .a { fill: red }\n    #b { fill: blue }\n  </style><rect class=\"a\" width=\"1\" height=\"1\"/><rect id=\"b\" width=\"1\" height=\"1\" x=\"1\"/></svg>",
		},
		{
			name: "text keeps its space, entities are escaped",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><text x="1" y="9">a  <tspan> b </tspan> &amp; &lt;c&gt;</text><path d="M0 0" data-x="1"/></svg>`,
			want: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20"><text x="1" y="9">a  <tspan> b </tspan> &amp; &lt;c&gt;</text><path d="M0 0"/></svg>`,
		},
		{
			name: "attribute escapes",
			in:   `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><g style="font-family: 'A &amp; B', &quot;C&quot;"><path d="M0 0"/></g></svg>`,
			want: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><g style="font-family: 'A &amp; B', &quot;C&quot;"><path d="M0 0"/></g></svg>`,
		},
		{
			name: "prefixed svg elements",
			in:   `<svg:svg xmlns:svg="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><svg:path d="M0 0"/></svg:svg>`,
			want: `<svg:svg xmlns:svg="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><svg:path d="M0 0"/></svg:svg>`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Minify([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got\n%s\nwant\n%s", got, tc.want)
			}
			if err := Equivalent([]byte(tc.in), got); err != nil {
				t.Errorf("not equivalent: %v", err)
			}
			again, err := Minify(got)
			if err != nil || string(again) != string(got) {
				t.Errorf("not idempotent: %s (%v)", again, err)
			}
		})
	}
}

func TestMinifyErrors(t *testing.T) {
	if _, err := Minify([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path></svg>`)); err == nil {
		t.Error("malformed XML: no error")
	}
	if _, err := Minify([]byte(`<html></html>`)); err == nil || !strings.Contains(err.Error(), "not <svg>") {
		t.Errorf("root html: %v", err)
	}
	entities := `<!DOCTYPE svg [ <!ENTITY a "M0 0"> ]><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2 2"><path d="&a;"/></svg>`
	if _, err := Minify([]byte(entities)); !errors.Is(err, ErrNotMinifiable) {
		t.Errorf("entities: %v, want ErrNotMinifiable", err)
	}
}

// Every SVG file of the SVG package's Chromium galleries (gradients, clip
// paths, masks, use and symbol, style sheets, switch, entities...) draws
// the same after Minify.
func TestMinifyEquivalentOnSVGCells(t *testing.T) {
	var files []string
	for _, pat := range []string{"../svg/testdata/chromium/cells/*.svg", "../svg/testdata/chromium-sheets/cells/*.svg", "testdata/icons/*.svg"} {
		m, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) < 30 {
		t.Fatalf("found %d files, want more than 30", len(files))
	}
	smaller := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		min, err := Minify(data)
		if errors.Is(err, ErrNotMinifiable) {
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if err := Equivalent(data, min); err != nil {
			t.Errorf("%s: %v", f, err)
		}
		if len(min) < len(data) {
			smaller++
		}
	}
	if smaller < len(files)/2 {
		t.Errorf("only %d of %d files got smaller", smaller, len(files))
	}
}

// Equivalent notices a change that draws differently.
func TestEquivalentDetectsDifferences(t *testing.T) {
	const a = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" stroke="currentColor" fill="none"><path d="M0 0L24 24"/></svg>`
	for name, b := range map[string]string{
		"path":      strings.Replace(a, "L24 24", "L24 23", 1),
		"viewBox":   strings.Replace(a, "0 0 24 24", "0 0 25 24", 1),
		"stroke":    strings.Replace(a, `stroke="currentColor"`, `stroke="red"`, 1),
		"dropped":   strings.Replace(a, `<path d="M0 0L24 24"/>`, ``, 1),
		"intrinsic": strings.Replace(a, `viewBox=`, `width="10" height="10" viewBox=`, 1),
	} {
		if err := Equivalent([]byte(a), []byte(b)); err == nil {
			t.Errorf("%s: no difference found", name)
		}
	}
	if err := Equivalent([]byte(a), []byte(a)); err != nil {
		t.Errorf("same document: %v", err)
	}
	if err := Equivalent([]byte("nope"), []byte(a)); err == nil {
		t.Error("invalid document: no error")
	}
}

func TestVarName(t *testing.T) {
	for _, tc := range []struct {
		file, prefix, suffix, want string
	}{
		{"check.svg", "", "", "Check"},
		{"arrow-up.svg", "", "", "ArrowUp"},
		{"arrow_up-right.SVG", "", "", "ArrowUpRight"},
		{"a-arrow-down.svg", "", "", "AArrowDown"},
		{"volume-2.svg", "", "", "Volume2"},
		{"arrow-down-0-1.svg", "", "", "ArrowDown0_1"},
		{"arrow-down-01.svg", "", "", "ArrowDown01"},
		{"box-1-a-2.svg", "", "", "Box1A2"},
		{"v1.2.svg", "", "", "V1_2"},
		{"3d-rotate.svg", "", "", "Icon3dRotate"},
		{"123.svg", "", "", "Icon123"},
		{"solid/arrow-up.svg", "", "", "SolidArrowUp"},
		{"arrowUp.svg", "", "", "ArrowUp"},
		{"my icon (2).svg", "", "", "MyIcon2"},
		{"héllo-wörld.svg", "", "", "HélloWörld"},
		{"日本.svg", "", "", "Icon日本"},
		{"type.svg", "", "", "Type"},
		{"func.svg", "", "", "Func"},
		{"range.svg", "", "", "Range"},
		{"check.svg", "Icon", "", "IconCheck"},
		{"3d.svg", "Icon", "", "Icon3d"},
		{"check.svg", "", "Icon", "CheckIcon"},
		{"check.v2.svg", "", "", "CheckV2"},
	} {
		got, err := VarName(tc.file, tc.prefix, tc.suffix)
		if err != nil || got != tc.want {
			t.Errorf("VarName(%q, %q, %q) = %q, %v; want %q", tc.file, tc.prefix, tc.suffix, got, err, tc.want)
		}
	}
	for _, file := range []string{"---.svg", ".svg", "(+).svg"} {
		if got, err := VarName(file, "", ""); err == nil {
			t.Errorf("VarName(%q) = %q, want an error", file, got)
		}
	}
}

func TestOptionErrors(t *testing.T) {
	fsys := fstest.MapFS{"a.svg": {Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`)}}
	for _, tc := range []struct {
		opt  Options
		want string
	}{
		{Options{}, "missing the package name"},
		{Options{Package: "1x"}, "invalid package name"},
		{Options{Package: "func"}, "invalid package name"},
		{Options{Package: "icons", Prefix: "icon"}, "invalid -prefix"},
		{Options{Package: "icons", Prefix: "I-"}, "invalid -prefix"},
		{Options{Package: "icons", Suffix: "a b"}, "invalid -suffix"},
	} {
		_, err := Generate(fsys, tc.opt)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%+v: %v, want %q", tc.opt, err, tc.want)
		}
	}
	if _, err := Generate(fstest.MapFS{"readme.txt": {}}, Options{Package: "icons"}); err == nil || !strings.Contains(err.Error(), "no .svg files") {
		t.Errorf("empty directory: %v", err)
	}
	if _, err := Generate(fstest.MapFS{"sub/a.svg": fsys["a.svg"]}, Options{Package: "icons"}); err == nil || !strings.Contains(err.Error(), "-r") {
		t.Errorf("files only in a subdirectory: %v", err)
	}
}

func TestGenerateProblems(t *testing.T) {
	good := &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="M0 0h1"/></svg>`)}
	fsys := fstest.MapFS{
		"good.svg":   good,
		"broken.svg": {Data: []byte(`<svg><path></svg>`)},
		"html.svg":   {Data: []byte(`<html/>`)},
		"empty.svg":  {Data: nil},
	}
	_, err := Generate(fsys, Options{Package: "icons", Minify: true})
	var ge *Error
	if !errors.As(err, &ge) || len(ge.Problems) != 3 {
		t.Fatalf("got %v, want an Error with 3 problems", err)
	}
	for _, want := range []string{"broken.svg:", "html.svg:", "empty.svg:", "-skip-invalid"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %q:\n%v", want, err)
		}
	}
	res, err := Generate(fsys, Options{Package: "icons", Minify: true, SkipInvalid: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Icons) != 1 || res.Icons[0].Var != "Good" || len(res.Skipped) != 3 {
		t.Errorf("icons %v, skipped %v", res.Icons, res.Skipped)
	}
	if _, err := Generate(fstest.MapFS{"bad.svg": fsys["broken.svg"]}, Options{Package: "icons", SkipInvalid: true}); err == nil || !strings.Contains(err.Error(), "nothing to generate") {
		t.Errorf("all skipped: %v", err)
	}
}

func TestGenerateCollisions(t *testing.T) {
	doc := &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`)}
	fsys := fstest.MapFS{"arrow-up.svg": doc, "arrow_up.svg": doc, "Arrow.Up.svg": doc, "ok.svg": doc}
	// SkipInvalid does not hide a collision.
	_, err := Generate(fsys, Options{Package: "icons", SkipInvalid: true})
	var ge *Error
	if !errors.As(err, &ge) || len(ge.Problems) != 2 {
		t.Fatalf("got %v, want 2 collisions", err)
	}
	if !strings.Contains(err.Error(), "ArrowUp is already made from") || !strings.Contains(err.Error(), "rename") {
		t.Errorf("error: %v", err)
	}
}

func TestGenerateDeterministicAndSorted(t *testing.T) {
	read := func(r bool) *Result {
		res, err := Generate(os.DirFS("testdata/icons"), Options{Package: "demo", Recursive: r, Minify: true,
			Source: "Demo icons 1.0 (https://example.com)", License: "ISC", LicenseText: "ISC License\n\nCopyright (c) Demo\n"})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	a, b := read(true), read(true)
	if string(a.Code) != string(b.Code) {
		t.Error("two runs differ")
	}
	if n := len(a.Icons); n != 6 {
		t.Errorf("%d icons, want 6 (recursive)", n)
	}
	if n := len(read(false).Icons); n != 5 {
		t.Errorf("%d icons, want 5 (not recursive)", n)
	}
	prev := ""
	for _, ic := range a.Icons {
		if ic.Var <= prev {
			t.Errorf("%s after %s: not sorted", ic.Var, prev)
		}
		prev = ic.Var
	}
	if a.BytesOut >= a.BytesIn {
		t.Errorf("minified size %d, source %d", a.BytesOut, a.BytesIn)
	}
	golden.Text(t, "demo_icons.go.txt", string(a.Code))
}

func TestGenerateNotMinified(t *testing.T) {
	src := "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 1 1\">\n  <path d=\"M0 0h1\"/>\n</svg>\n"
	res, err := Generate(fstest.MapFS{"a.svg": {Data: []byte(src)}}, Options{Package: "icons"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Icons[0].SVG != strings.TrimSpace(src) {
		t.Errorf("got %q", res.Icons[0].SVG)
	}
}

func TestGenerateEntitiesKept(t *testing.T) {
	src := `<?xml version="1.0"?><!DOCTYPE svg [ <!ENTITY d "M0 0h1"> ]>` + "\n" + `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><path d="&d;" stroke="red"/></svg>`
	res, err := Generate(fstest.MapFS{"a.svg": {Data: []byte(src)}}, Options{Package: "icons", Minify: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Icons[0].SVG != src {
		t.Errorf("got %q, want the file as written", res.Icons[0].SVG)
	}
}

func TestGenerateQuoting(t *testing.T) {
	// A backtick or a carriage return needs an interpreted string.
	doc := "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 1 1\"><style>/* `tick` */ rect { fill: red }</style><rect width=\"1\" height=\"1\"/></svg>"
	res, err := Generate(fstest.MapFS{"a.svg": {Data: []byte(doc)}}, Options{Package: "icons", Minify: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Code), `SVG: "<svg`) {
		t.Errorf("the backtick was not quoted:\n%s", res.Code)
	}
}

func TestGenerateIgnoresOtherFiles(t *testing.T) {
	doc := &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`)}
	res, err := Generate(fstest.MapFS{
		"a.svg": doc, "b.SVG": doc, ".hidden.svg": doc, "c.png": doc, "d.svg.bak": doc, ".git/e.svg": doc, "dir/f.svg": doc,
	}, Options{Package: "icons"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, ic := range res.Icons {
		got = append(got, ic.Var)
	}
	if strings.Join(got, ",") != "A,B" {
		t.Errorf("got %v, want A,B", got)
	}
}

func TestGenerateDeprecated(t *testing.T) {
	doc := &fstest.MapFile{Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"/>`)}
	fsys := fstest.MapFS{"circle-alert.svg": doc, "alert-circle.svg": doc, "old/arrow.svg": doc, "ok.svg": doc}
	res, err := Generate(fsys, Options{Package: "icons", Recursive: true, Deprecated: map[string]string{
		"alert-circle": "circle-alert",
		"old/arrow":    "ok",
	}})
	if err != nil {
		t.Fatal(err)
	}
	code := string(res.Code)
	for _, want := range []string{
		"// AlertCircle is the icon \"alert-circle\".\n//\n// Deprecated: use CircleAlert.\nvar AlertCircle =",
		"// OldArrow is the icon \"old/arrow\".\n//\n// Deprecated: use Ok.\nvar OldArrow =",
		"// CircleAlert is the icon \"circle-alert\".\nvar CircleAlert =",
	} {
		if !strings.Contains(code, want) {
			t.Errorf("the file lacks %q:\n%s", want, code)
		}
	}
	if strings.Count(code, "Deprecated:") != 2 {
		t.Errorf("want 2 deprecation paragraphs:\n%s", code)
	}

	for _, tc := range []struct {
		name string
		dep  map[string]string
		want string
	}{
		{"missing icon", map[string]string{"nope": "ok"}, `deprecated icon "nope" is not an icon`},
		{"missing replacement", map[string]string{"ok": "nope"}, `in favor of "nope", which is not an icon`},
		{"itself", map[string]string{"ok": "ok"}, "in favor of itself"},
		{"chain", map[string]string{"alert-circle": "ok", "ok": "circle-alert"}, `"ok", which is deprecated too`},
	} {
		_, err := Generate(fsys, Options{Package: "icons", Recursive: true, Deprecated: tc.dep})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", tc.name, err, tc.want)
		}
	}
}
