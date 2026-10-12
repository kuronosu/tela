package tela_test

import (
	"fmt"
	"go/build"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// div builds a Div without package h, which tela's tests must not import
// (h imports tela).
func div(c ...tela.Child) tela.Node { return core.Primitive(style.KindDiv, c...) }

func TestDoesNotImportH(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if imp == "github.com/kuronosu/tela/h" || strings.HasPrefix(imp, "github.com/kuronosu/tela/h/") {
			t.Errorf("package tela imports %s; h imports tela, not the other way round (ARCHITECTURE section 5)", imp)
		}
		if strings.HasPrefix(imp, "github.com/kuronosu/tela/cmd/") || imp == "github.com/kuronosu/tela/aria" || imp == "github.com/kuronosu/tela/kit" {
			t.Errorf("package tela imports %s, a higher layer", imp)
		}
	}
}

func TestHex(t *testing.T) {
	if got := tela.Hex("#1e1e2e"); got != tela.RGB(0x1e, 0x1e, 0x2e) {
		t.Errorf("Hex(#1e1e2e) = %v, want RGB(30, 30, 46)", got)
	}
	if got := tela.Hex("#fff8"); got.A < 0.53 || got.A > 0.54 || got.R != 1 {
		t.Errorf("Hex(#fff8) = %v", got)
	}
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, `"1e1e2e"`) || !strings.Contains(msg, "must start with '#'") {
			t.Errorf("panic message %q does not explain the problem", msg)
		}
	}()
	tela.Hex("1e1e2e")
}

func TestColorConstructors(t *testing.T) {
	if got := tela.RGBA(255, 0, 0, 0.5); got != (tela.Color{R: 1, A: 0.5}) {
		t.Errorf("RGBA = %v", got)
	}
	// red-500 of the Tailwind v4 palette is oklch(0.637 0.237 25.331),
	// #fb2c36 once clipped to sRGB.
	if got := tela.OKLCH(0.637, 0.237, 25.331).Clamp().NRGBA8(); got.R < 0xfa || got.R > 0xfc || got.G < 0x2b || got.G > 0x2d || got.A != 255 {
		t.Errorf("OKLCH(red-500) = %v, want about #fb2c36", got)
	}
	if tela.White.A != 1 || tela.Transparent.A != 0 || tela.Black.R != 0 {
		t.Error("color constants wrong")
	}
}

func TestPreviewsNames(t *testing.T) {
	p := tela.Previews{"b": nil, "A": nil, "a": nil, "1 first": nil}
	if got, want := p.Names(), []string{"1 first", "A", "a", "b"}; !slices.Equal(got, want) {
		t.Errorf("Names = %q, want %q", got, want)
	}
	if got := (tela.Previews(nil)).Names(); len(got) != 0 {
		t.Errorf("Names of nil = %q", got)
	}
}

func TestRunNeedsRoot(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "Root") {
			t.Errorf("panic message %q does not mention Root", msg)
		}
	}()
	tela.Run(tela.App{Title: "x"})
}

func TestUseStateRendersAndRebuilds(t *testing.T) {
	var set func(int)
	comp := func() tela.Node {
		n, s := tela.UseState(1)
		set = s
		return div(core.Text(fmt.Sprint("n=", n)))
	}
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: 100, H: 50})
	tr.Mount(core.Component(comp))
	tr.Frame()
	text := func() string {
		n := tr.Inspect()
		for len(n.Children) > 0 && n.Text == "" {
			n = n.Children[0]
		}
		return n.Text
	}
	if got := text(); got != "n=1" {
		t.Fatalf("first render %q", got)
	}
	set(2)
	tr.Frame()
	if got := text(); got != "n=2" {
		t.Errorf("after set(2): %q", got)
	}
}

func TestUseStateOutsideBuild(t *testing.T) {
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, "outside a component build") {
			t.Errorf("panic message %q", msg)
		}
	}()
	tela.UseState(0)
}

func TestLinearGradientNeedsStops(t *testing.T) {
	defer func() {
		if msg := fmt.Sprint(recover()); !strings.Contains(msg, "at least two stops") {
			t.Errorf("panic message %q", msg)
		}
	}()
	tela.LinearGradient(90, tela.Stop(tela.White))
}

func TestStopAt(t *testing.T) {
	g := tela.LinearGradient(90, tela.Stop(tela.Black), tela.StopAt(tela.White, 70))
	if !g.Stops[1].Pos.Set || g.Stops[1].Pos.Pct != 70 || g.Stops[0].Pos.Set {
		t.Errorf("stops %+v", g.Stops)
	}
}

func TestMouseButtonString(t *testing.T) {
	for b, want := range map[tela.MouseButton]string{tela.MouseButtonNone: "none", tela.MouseButtonLeft: "left", tela.MouseButtonForward: "forward"} {
		if b.String() != want {
			t.Errorf("%d.String() = %q, want %q", b, b.String(), want)
		}
	}
}
