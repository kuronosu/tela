package h_test

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"image"
	"image/color"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// inspect renders root headlessly and returns the box tree as parsed JSON
// of its serialized form, the way `tela inspect --layout` prints it.
func inspect(t *testing.T, root tela.Node, w, h float32) box {
	t.Helper()
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: w, H: h})
	tr.Mount(root)
	tr.Frame()
	js, err := json.Marshal(tr.Inspect())
	if err != nil {
		t.Fatal(err)
	}
	var b box
	if err := json.Unmarshal(js, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

type box struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	Rect struct {
		X, Y, Width, Height float32
	} `json:"rect"`
	Style struct {
		Background string `json:"background"`
		Color      string `json:"color"`
		Opacity    float32
		Classes    string
		Errors     []string
	} `json:"style"`
	Text     string `json:"text"`
	Children []box  `json:"children"`
}

func (b *box) find(key string) *box {
	if b.Key == key {
		return b
	}
	for i := range b.Children {
		if f := b.Children[i].find(key); f != nil {
			return f
		}
	}
	return nil
}

func TestClassesAndTypedProps(t *testing.T) {
	root := Div(Class("flex flex-col gap-2 p-4"),
		Div(Key("class"), Class("h-10 w-24 bg-red-500")),
		// A typed prop wins over the classes, whatever the order.
		Div(Key("typed"), Style(tela.Width(120), tela.Height(30), tela.Bg(tela.Hex("#336699"))), Class("w-4 h-4 bg-red-500")),
		Div(Key("padded"), Style(tela.PaddingXY(10, 5), tela.Radius(4), tela.TextColor(tela.RGB(255, 0, 0))), Span(Text("x"))),
	)
	b := inspect(t, root, 400, 300)
	class, typed, padded := b.find("class"), b.find("typed"), b.find("padded")
	if class == nil || typed == nil || padded == nil {
		t.Fatalf("missing boxes in %+v", b)
	}
	if class.Rect.Width != 96 || class.Rect.Height != 40 {
		t.Errorf("class box %vx%v, want 96x40", class.Rect.Width, class.Rect.Height)
	}
	if typed.Rect.Width != 120 || typed.Rect.Height != 30 {
		t.Errorf("typed box %vx%v, want 120x30 (typed props win over classes)", typed.Rect.Width, typed.Rect.Height)
	}
	if typed.Style.Background != "#336699" {
		t.Errorf("typed background %q, want #336699", typed.Style.Background)
	}
	if padded.Style.Color != "#ff0000" {
		t.Errorf("text color %q, want #ff0000", padded.Style.Color)
	}
	// 10px of padding on each side of the text, 5px above and below
	// (the line box of the 16px root text is 24px tall).
	if padded.Rect.Height != 34 {
		t.Errorf("padded height %v, want 34", padded.Rect.Height)
	}
}

func TestDeferredProps(t *testing.T) {
	x, w := float32(30), float32(80)
	root := Div(Class("relative"),
		Div(Key("a"), Class("h-4 w-4"), Style(tela.WidthFn(func() float32 { return w }), tela.HeightFn(func() float32 { return 12 }))),
		Div(Key("b"), Class("h-4 w-4"), Style(tela.TranslateFn(func() (float32, float32) { return x, 0 }), tela.OpacityFn(func() float32 { return 0.5 }))),
	)
	b := inspect(t, root, 200, 100)
	if a := b.find("a"); a == nil || a.Rect.Width != 80 || a.Rect.Height != 12 {
		t.Errorf("deferred size: %+v, want 80x12", a)
	}
	if bb := b.find("b"); bb == nil || bb.Rect.Width != 16 {
		t.Errorf("box with deferred translate and opacity: %+v", bb)
	}
}

func TestHelpers(t *testing.T) {
	items := []string{"a", "b", "c"}
	root := Div(Key("root"),
		If(true, Div(Key("shown"), Text("yes"))),
		If(false, Div(Key("hidden"), Text("no"))),
		Map(items, func(s string) tela.Node { return Div(Key(s), Text(s)) }),
		Fragment(Div(Key("f1")), Div(Key("f2"))),
		tela.WithKey(core.Component(func() tela.Node { return Div(Key("inner")) }), "comp"),
	)
	b := inspect(t, root, 200, 200)
	for _, k := range []string{"shown", "a", "b", "c", "f1", "f2", "inner"} {
		if b.find(k) == nil {
			t.Errorf("box %q missing", k)
		}
	}
	if b.find("hidden") != nil {
		t.Error("If(false) rendered its node")
	}
}

func TestElementKinds(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 4))
	root := Div(
		H1(), H2(), H3(), H4(), H5(), H6(),
		P(Strong(Text("s")), Em(Text("e")), Code(Text("c")), A(Href("https://example.com"), Text("a")), Span(Textf("%d", 1))),
		Button(OnClick(func(tela.MouseEvent) {}), OnMouseDown(func(tela.MouseEvent) {}), OnMouseUp(func(tela.MouseEvent) {}),
			OnMouseEnter(func(tela.MouseEvent) {}), OnMouseLeave(func(tela.MouseEvent) {}), Text("b")),
		Img(Key("img"), Src(src)),
	)
	b := inspect(t, root, 200, 200)
	if im := b.find("img"); im == nil || im.Kind != "img" || im.Rect.Width != 8 || im.Rect.Height != 4 {
		t.Errorf("img: %+v, want 8x4", im)
	}
}

func TestProgrammerErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		f    func()
		want string
	}{
		"OnClick nil":   {func() { OnClick[func()](nil) }, "OnClick(nil)"},
		"OnMouseUp nil": {func() { OnMouseUp[func()](nil) }, "OnMouseUp(nil)"},
		// Key(nil) and Key([]int{1}) do not compile (Key[K comparable]);
		// an interface holding nil or a slice still panics.
		"Key nil":       {func() { Key[any](nil) }, "non-nil comparable"},
		"Key slice":     {func() { Key[any]([]int{1}) }, "comparable"},
		"Src nil":       {func() { Src(nil) }, "Src(nil)"},
		"Href on Div":   {func() { Div(Href("x")) }, "only A"},
		"Src on Div":    {func() { Div(Src(image.NewRGBA(image.Rect(0, 0, 1, 1)))) }, "only Img"},
		"Fragment prop": {func() { Fragment(Class("p-4")) }, "no box"},
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("%s: no panic", name)
					return
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, tc.want) {
					t.Errorf("%s: panic %q does not contain %q", name, msg, tc.want)
				}
			}()
			tc.f()
		}()
	}
}

// TestExportedNames pins the dot-import surface: only elements, props and
// helpers (ARCHITECTURE section 19), so that importing h cannot collide
// with a user's names.
func TestExportedNames(t *testing.T) {
	want := []string{
		"A", "Alt", "AnimateLayout", "AriaBusy", "AriaChecked", "AriaDescription", "AriaDisabled", "AriaExpanded", "AriaHidden", "AriaLabel",
		"AriaLabelledBy", "AriaLevel", "AriaPressed", "AriaReadOnly", "AriaRequired", "AriaSelected", "AriaValueMax", "AriaValueMin",
		"AriaValueNow", "AriaValueText", "AutoComplete", "AutoFocus", "Bind", "BindValue", "Button", "Class",
		"Code", "DefaultValue", "Dir", "Disabled", "Div", "Em", "Exit", "For", "Form", "Fragment", "H1", "H2", "H3", "H4", "H5",
		"H6", "Href", "ID", "Icon", "If", "IfElse", "Img", "Input", "ItemHeight", "ItemKey", "Key", "Label", "LayoutID", "Map", "MapIndex", "MapKeyed", "MaxLength", "Name", "Nodes",
		"OnBeforeInput", "OnBlur", "OnChange", "OnClick", "OnCompositionEnd", "OnCompositionStart",
		"OnCompositionUpdate", "OnContextMenu", "OnDoubleClick", "OnDrag", "OnDragEnd", "OnDragStart", "OnFocus",
		"OnKeyDown", "OnKeyUp", "OnLongPress", "OnMouseDown", "OnMouseEnter", "OnMouseLeave", "OnMouseMove",
		"OnMouseUp", "OnPointerCancel", "OnPointerDown", "OnPointerEnter", "OnPointerLeave", "OnPointerMove",
		"OnPointerUp", "OnScroll", "OnSelect", "OnSubmit", "OnWheel", "Overscan", "P", "Placeholder", "Portal", "Presence", "Range", "ReadOnly",
		"Ref", "Role", "Rows", "Scale", "Scheme", "Span", "SpellCheck", "Src", "SrcBytes", "SrcFS", "SrcPath", "Strong", "Style", "TabIndex", "TestID", "Text", "TextFn",
		"Textarea", "Textf", "Tint", "Type", "Value",
		"VirtualList",
		// ADR-096: the widget ARIA props and Inert.
		"AriaActiveDescendant", "AriaAutoComplete", "AriaControls", "AriaHasPopup", "AriaModal", "AriaMultiSelectable",
		"AriaOrientation", "Inert",
		// ADR-101: the tooltip's description.
		"AriaDescribedBy",
		// ADR-131: aria-invalid and aria-sort.
		"AriaInvalid", "AriaSort",
		// ADR-171: aria-current.
		"AriaCurrent",
	}
	fset := token.NewFileSet()
	var got []string
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	for _, f := range files {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					got = append(got, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							got = append(got, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								got = append(got, n.Name)
							}
						}
					}
				}
			}
		}
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("package h exports\n  %v\nwant\n  %v\n(adding a name to h risks collisions in dot-importing packages: it needs a decision)", got, want)
	}
}

// profile is a small screen that exercises typed props, helpers and images
// through the public API only.
func profile() tela.Node {
	avatar := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			avatar.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 200, 255})
		}
	}
	people := []struct {
		ID   int
		Name string
		Tag  string
		On   bool
	}{{1, "Ada Lovelace", "math", true}, {2, "Grace Hopper", "compilers", false}, {3, "Linus", "kernel", true}}
	return Div(Class("flex flex-col gap-4 p-6 bg-slate-100 min-h-screen"),
		Div(Class("flex items-center gap-4 rounded-2xl p-5 text-white"),
			Style(
				tela.BackgroundGradient(tela.LinearGradient(135, tela.Stop(tela.Hex("#6366f1")), tela.Stop(tela.Hex("#d946ef")))),
				tela.BoxShadow(tela.Shadow{OffsetY: 10, Blur: 25, Spread: -3, Color: tela.Black.WithAlpha(0.25)}),
			),
			Img(Class("h-14 w-14 rounded-full"), Src(avatar)),
			Div(
				H2(Class("text-xl font-bold"), Text("Team")),
				P(Class("text-sm opacity-80"), Text("Typed "), Strong(Text("props")), Text(", classes and helpers.")),
			),
		),
		Map(people, func(p struct {
			ID   int
			Name string
			Tag  string
			On   bool
		}) tela.Node {
			return Div(Key(p.ID), Class("flex items-center justify-between rounded-xl bg-white px-4 py-3 shadow-sm"),
				Span(Class("font-medium text-slate-800"), Text(p.Name)),
				Div(Class("flex items-center gap-2"),
					Span(Class("text-xs text-slate-500"), Text(p.Tag)),
					If(p.On, Span(Class("rounded-full px-2 py-0.5 text-xs font-semibold"),
						Style(tela.Bg(tela.OKLCH(0.9, 0.1, 150)), tela.TextColor(tela.OKLCH(0.4, 0.1, 150))), Text("online"))),
				),
			)
		}),
	)
}

func TestProfileGolden(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			_, img := core.Render(core.Component(profile), core.RenderOptions{Width: 420, Height: 320, Scale: scale})
			golden.Image(t, fmt.Sprintf("profile_x%g", scale), img)
		})
	}
}
