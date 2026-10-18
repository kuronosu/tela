package layout

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The expected layouts of the Taffy fixtures are measured with headless
// Chromium, as Taffy's own gentest does (ADR-039), and committed in
// testdata/taffy/chromium.json so that the conformance test runs without
// Chromium; Tela's own cases (testdata/cases, same format) likewise. To
// regenerate them:
//
//	TELA_CHROMIUM=1 go test -run 'TestTaffyFixtures|TestTelaCases' ./internal/layout/
//
// The generator writes one page (to a temporary directory) that links the
// vendored test_base_style.css, then, for each fixture in turn, puts the
// fixture's body into the page, waits for the Ahem font and records the
// border box of #test-root and of each of its descendants relative to its
// parent (getBoundingClientRect minus the parent's, Taffy's measure),
// plus scrollWidth and scrollHeight. Chromium runs with an 800x600 window,
// like Taffy's gentest; the size of its viewport (the initial containing
// block, smaller than the window in headless Chromium) is recorded with the
// results, and the document never shows scrollbars (html { overflow:
// hidden }). Only the "border-box ltr" variant of Taffy's four is measured:
// Tela is always border-box, and direction comes from the fixtures' own
// styles.

// chromiumBox is one measured box.
type chromiumBox struct {
	X, Y, W, H float64
	// SW and SH are scrollWidth and scrollHeight (integers in Chromium).
	SW, SH float64
	C      []*chromiumBox `json:",omitempty"`
}

// chromiumResults is the content of chromium.json.
type chromiumResults struct {
	Chromium string
	// ViewportW and ViewportH are the size of the initial containing block.
	ViewportW, ViewportH float64
	Fixtures             map[string]*chromiumBox
}

// fixtureSet is a directory of fixtures with their Chromium results in
// chromium.json.
type fixtureSet struct {
	dir string
	// categories are subdirectories; fixture names are "category/name".
	// Without categories, names are the base names of dir's files.
	categories []string
}

var (
	// taffyFixtures are Taffy's fixtures (vendored).
	taffyFixtures = fixtureSet{"testdata/taffy", []string{"block", "blockflex", "blockgrid", "flex", "grid", "gridflex", "leaf"}}
	// telaCases are Tela's own cases in the same format, for behaviour the
	// Taffy fixtures do not exercise.
	telaCases = fixtureSet{dir: "testdata/cases"}
)

// baseStyleCSS is the fixtures' stylesheet (with the Ahem font).
const baseStyleCSS = "testdata/taffy/gentest/test_base_style.css"

// files returns the fixture names in order.
func (fs fixtureSet) files(t testing.TB) []string {
	var names []string
	cats := fs.categories
	if len(cats) == 0 {
		cats = []string{""}
	}
	for _, cat := range cats {
		files, err := filepath.Glob(filepath.Join(fs.dir, cat, "*.html"))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			name := strings.TrimSuffix(filepath.Base(f), ".html")
			if cat != "" {
				name = cat + "/" + name
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// source returns the HTML of a fixture.
func (fs fixtureSet) source(t testing.TB, name string) string {
	src, err := os.ReadFile(filepath.Join(fs.dir, name+".html"))
	if err != nil {
		t.Fatal(err)
	}
	return string(src)
}

// fixtureBody returns the inner HTML and the style attribute of a
// fixture's body.
func fixtureBody(src string) (body, bodyStyle string, err error) {
	i := strings.Index(src, "<body")
	if i < 0 {
		return "", "", fmt.Errorf("no <body>")
	}
	j := strings.IndexByte(src[i:], '>')
	if j < 0 {
		return "", "", fmt.Errorf("malformed <body>")
	}
	// Some fixtures omit the end tags, which HTML allows.
	k := strings.Index(src, "</body>")
	if k < 0 {
		if k = strings.Index(src, "</html>"); k < 0 {
			k = len(src)
		}
	}
	tag := src[i : i+j+1]
	if s := strings.Index(tag, `style="`); s >= 0 {
		rest := tag[s+7:]
		bodyStyle = rest[:strings.IndexByte(rest, '"')]
	}
	return src[i+j+1 : k], bodyStyle, nil
}

// chromiumPage returns the page that measures the fixtures.
func (fs fixtureSet) chromiumPage(t *testing.T, names []string) string {
	css, err := filepath.Abs(baseStyleCSS)
	if err != nil {
		t.Fatal(err)
	}
	type fx struct {
		Name, Body, Style string
	}
	var list []fx
	for _, name := range names {
		body, style, err := fixtureBody(fs.source(t, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		list = append(list, fx{name, body, style})
	}
	js, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	// "</" cannot appear inside a script element.
	jsText := strings.ReplaceAll(string(js), "</", `<\/`)
	return `<!DOCTYPE html>
<html><head><meta charset="utf-8">
<link rel="stylesheet" type="text/css" href="file://` + css + `">
<style>html { overflow: hidden; }</style>
</head><body class="border-box ltr"><script>
const fixtures = ` + jsText + `;
function describe(e) {
  const r = e.getBoundingClientRect(), p = e.parentNode.getBoundingClientRect();
  const box = {X: r.x - p.x, Y: r.y - p.y, W: r.width, H: r.height, SW: e.scrollWidth, SH: e.scrollHeight};
  const c = Array.from(e.children).map(describe);
  if (c.length) box.C = c;
  return box;
}
document.fonts.load("10px ahem", "X").then(() => {
  const out = {Chromium: navigator.userAgent, ViewportW: document.documentElement.clientWidth,
    ViewportH: document.documentElement.clientHeight, Fixtures: {}};
  for (const f of fixtures) {
    if (f.Style) document.body.setAttribute("style", f.Style); else document.body.removeAttribute("style");
    document.body.innerHTML = f.Body;
    const root = document.getElementById("test-root");
    out.Fixtures[f.Name] = root ? describe(root) : null;
  }
  document.body.removeAttribute("style");
  const pre = document.createElement("pre");
  pre.textContent = "BEGIN" + JSON.stringify(out) + "END";
  document.body.replaceChildren(pre);
});
</script></body></html>
`
}

// regenerate measures every fixture of the set in Chromium and writes
// chromium.json.
func (fs fixtureSet) regenerate(t *testing.T) {
	names := fs.files(t)
	page := filepath.Join(t.TempDir(), "fixtures.html")
	if err := os.WriteFile(page, []byte(fs.chromiumPage(t, names)), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("chromium", "--headless", "--disable-gpu", "--force-device-scale-factor=1",
		"--allow-file-access-from-files", "--virtual-time-budget=60000", "--window-size=800,600",
		"--dump-dom", "file://"+page)
	outb, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium: %v", err)
	}
	s := string(outb)
	i, j := strings.Index(s, "BEGIN"), strings.Index(s, "END</pre>")
	if i < 0 || j < 0 {
		t.Fatalf("chromium output has no result:\n%s", s[:min(len(s), 2000)])
	}
	var res chromiumResults
	if err := json.Unmarshal([]byte(html.UnescapeString(s[i+5:j])), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Fixtures) != len(names) {
		t.Fatalf("chromium measured %d fixtures, want %d", len(res.Fixtures), len(names))
	}
	// One fixture per line: compact, but diffs show which fixtures changed.
	var b strings.Builder
	fmt.Fprintf(&b, "{\"Chromium\": %q,\n\"ViewportW\": %g, \"ViewportH\": %g,\n\"Fixtures\": {\n", res.Chromium, res.ViewportW, res.ViewportH)
	for k, name := range names {
		v, err := json.Marshal(res.Fixtures[name])
		if err != nil {
			t.Fatal(err)
		}
		sep := ","
		if k == len(names)-1 {
			sep = ""
		}
		fmt.Fprintf(&b, "%q: %s%s\n", name, v, sep)
	}
	b.WriteString("}}\n")
	if err := os.WriteFile(filepath.Join(fs.dir, "chromium.json"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("measured %d fixtures with %s", len(names), res.Chromium)
}

// load reads the set's chromium.json, regenerating it first when
// TELA_CHROMIUM=1.
func (fs fixtureSet) load(t *testing.T) *chromiumResults {
	if os.Getenv("TELA_CHROMIUM") == "1" {
		fs.regenerate(t)
	}
	data, err := os.ReadFile(filepath.Join(fs.dir, "chromium.json"))
	if err != nil {
		t.Fatalf("%v (regenerate with TELA_CHROMIUM=1)", err)
	}
	var res chromiumResults
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	return &res
}
