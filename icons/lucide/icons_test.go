package lucide

import (
	"encoding/xml"
	"go/ast"
	"go/parser"
	"go/token"
	"image/color"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// entry is one icon of the generated file.
type entry struct {
	Name string // the Go variable
	SVG  string
}

var (
	entriesOnce sync.Once
	entriesAll  []entry
	entriesErr  error
)

// allIcons lists the icons of icons_gen.go by reading the file: the package
// has no table of icons (a table would link every icon into every program
// that imports the package), so the tests find them in the source.
func allIcons(t testing.TB) []entry {
	t.Helper()
	entriesOnce.Do(func() {
		f, err := parser.ParseFile(token.NewFileSet(), "icons_gen.go", nil, 0)
		if err != nil {
			entriesErr = err
			return
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				lit, ok := vs.Values[0].(*ast.CompositeLit)
				if !ok || len(lit.Elts) != 1 {
					entriesErr = errUnexpected(vs.Names[0].Name)
					return
				}
				kv, ok := lit.Elts[0].(*ast.KeyValueExpr)
				if !ok {
					entriesErr = errUnexpected(vs.Names[0].Name)
					return
				}
				bl, ok := kv.Value.(*ast.BasicLit)
				if !ok || bl.Kind != token.STRING {
					entriesErr = errUnexpected(vs.Names[0].Name)
					return
				}
				s, err := strconv.Unquote(bl.Value)
				if err != nil {
					entriesErr = err
					return
				}
				entriesAll = append(entriesAll, entry{vs.Names[0].Name, s})
			}
		}
	})
	if entriesErr != nil {
		t.Fatal(entriesErr)
	}
	return entriesAll
}

type errUnexpected string

func (e errUnexpected) Error() string { return "icons_gen.go: unexpected shape of var " + string(e) }

func TestGeneratedFileIsForThisVersion(t *testing.T) {
	// The header line "Source: Lucide X.Y.Z (...)" and Version agree.
	data, err := readFile("icons_gen.go", 400)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(data, "// Source: Lucide "+Version+" ") {
		t.Errorf("icons_gen.go was generated from another Lucide version than Version = %q; regenerate it (see README.md)\n%s", Version, data)
	}
}

func TestEveryIconIsValid(t *testing.T) {
	icons := allIcons(t)
	if len(icons) < 1500 {
		t.Fatalf("%d icons, want at least 1500", len(icons))
	}
	seen := map[string]bool{}
	for _, ic := range icons {
		if seen[ic.Name] {
			t.Errorf("%s is declared twice", ic.Name)
		}
		seen[ic.Name] = true
		if _, err := tela.ParseSVG([]byte(ic.SVG)); err != nil {
			t.Errorf("%s: %v", ic.Name, err)
		}
	}
	t.Logf("%d icons", len(icons))
}

// Lucide draws every icon on a 24x24 grid with a 2 px round stroke in
// currentColor and no fill; the generated icons keep that.
func TestLucideDefaults(t *testing.T) {
	want := map[string]string{
		"viewBox":         "0 0 24 24",
		"fill":            "none",
		"stroke":          "currentColor",
		"stroke-width":    "2",
		"stroke-linecap":  "round",
		"stroke-linejoin": "round",
		"width":           "24",
		"height":          "24",
		"xmlns":           "http://www.w3.org/2000/svg",
	}
	for _, ic := range allIcons(t) {
		root := rootAttrs(t, ic)
		for k, v := range want {
			if root[k] != v {
				t.Errorf("%s: %s = %q, want %q", ic.Name, k, root[k], v)
			}
		}
		if len(root) != len(want) {
			t.Errorf("%s: attributes %v, want only %d", ic.Name, root, len(want))
		}
	}
}

func rootAttrs(t testing.TB, ic entry) map[string]string {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(ic.SVG))
	for {
		tok, err := d.Token()
		if err != nil {
			t.Fatalf("%s: %v", ic.Name, err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			m := map[string]string{}
			for _, a := range se.Attr {
				m[a.Name.Local] = a.Value
			}
			return m
		}
	}
}

func TestKnownIcons(t *testing.T) {
	// Names the generator made from Lucide's file names (these also fail
	// to compile when a rename drops them).
	for name, ic := range map[string]tela.Icon{
		"Check": Check, "X": X, "Menu": Menu, "Search": Search, "ChevronDown": ChevronDown,
		"Settings": Settings, "ArrowRight": ArrowRight, "AArrowDown": AArrowDown,
		"ArrowDown0_1": ArrowDown0_1, "ArrowDown01": ArrowDown01, "Volume2": Volume2, "Type": Type,
	} {
		if !strings.HasPrefix(ic.SVG, "<svg ") {
			t.Errorf("%s = %q", name, ic.SVG)
		}
		if ic == (tela.Icon{}) {
			t.Errorf("%s is empty", name)
		}
	}
	if Check.SVG != `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>` {
		t.Errorf("Check = %s", Check.SVG)
	}
}

// Some icons through h.Icon: size, color, stroke width and accessibility.
func TestIconsRender(t *testing.T) {
	red := color.RGBA{R: 220, G: 38, B: 38, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	page := func() tela.Node {
		return h.Div(h.Class("flex items-center gap-4 p-4 text-[#dc2626]"),
			// Icons are decorative (aria-hidden), so the tests find them
			// through their wrappers.
			h.Div(h.TestID("default"), h.Icon(Check)),
			h.Div(h.TestID("sized"), h.Icon(Check, h.Class("size-12"))),
			h.Div(h.TestID("x"), h.Icon(X, h.Class("size-12 stroke-[4]"))),
			h.Icon(Search, h.Class("size-6"), h.AriaLabel("Search")),
		)
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(200, 80), telatest.RootClass("bg-white"))
	if b := app.Bounds(telatest.ByTestID("default")); b.Width != 16 || b.Height != 16 {
		t.Errorf("an icon without a size is %vx%v, want 1em = 16", b.Width, b.Height)
	}
	if b := app.Bounds(telatest.ByTestID("sized")); b.Width != 48 || b.Height != 48 {
		t.Errorf("size-12 icon is %vx%v", b.Width, b.Height)
	}
	// Check's long stroke runs from (9,17) to (20,6) in a 24 unit box shown
	// at 48 px: the middle of it is (14.5, 11.5) * 2.
	b := app.Bounds(telatest.ByTestID("sized"))
	if c := app.PixelAt(b.X+29, b.Y+23); c != red {
		t.Errorf("middle of the check is %v, want the text color %v", c, red)
	}
	if c := app.PixelAt(b.X+3, b.Y+3); c != white {
		t.Errorf("a corner of the check box is %v, want white", c)
	}
	// stroke-[4] on the X: 4 user units = 8 px, so a point 3 px off the
	// diagonal is still inside the stroke (the default would be 4 px wide).
	xb := app.Bounds(telatest.ByTestID("x"))
	// The X goes from (18,6) to (6,18): at its middle (12,12) = (24,24).
	if c := app.PixelAt(xb.X+24+3, xb.Y+24); c != red {
		t.Errorf("stroke-[4] did not widen the X: %v", c)
	}
	app.ExpectRole("img", "Search")
}
