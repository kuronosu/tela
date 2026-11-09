package kit_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// The end padding of an Input follows its EndContent, and an IconEnd and an
// EndContent can be combined (ADR-263). Tela's own design: shadcn's
// InputGroup has the addon in the flow, which this follows by measuring.

func endFitSheet() *sheet {
	c := func(name string, in kit.Input) cell {
		in.Props = h.TestID(name + "-input")
		return cell{name: name, width: 280, probes: []string{name + "-input"}, tela: in}
	}
	two := h.Div(h.Class("flex gap-1"),
		kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconEye, AriaLabel: "Show"},
		kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconX, AriaLabel: "Clear"})
	unit := h.Span(h.Class("px-1 text-sm"), h.Text("kg per month"))
	return &sheet{name: "input_endfit", width: 640, cells: []cell{
		c("two", kit.Input{DefaultValue: "a rather long text that runs under the buttons", EndContent: tela.C(func() tela.Node { return two })}),
		c("unit", kit.Input{DefaultValue: "a rather long text that runs under the unit", EndContent: tela.C(func() tela.Node { return unit })}),
		c("both", kit.Input{DefaultValue: "a rather long text that runs under the icon", IconEnd: iconX,
			EndContent: kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconEye, AriaLabel: "Show"}}),
		c("one", kit.Input{DefaultValue: "a rather long text that runs under the button", EndContent: kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconEye, IconClass: "size-4", AriaLabel: "Show"}}),
	}}
}

func TestInputEndFitGolden(t *testing.T) { endFitSheet().golden(t) }

func TestInputEndFitRTLGolden(t *testing.T) {
	s := endFitSheet()
	s.name, s.rtl = "input_endfit_rtl", true
	s.golden(t)
}

func TestInputEndPaddingFollowsTheContent(t *testing.T) {
	two := tela.C(func() tela.Node {
		return h.Div(h.Class("flex gap-1"),
			kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconEye, AriaLabel: "Show", Props: h.TestID("a")},
			kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconX, AriaLabel: "Clear", Props: h.TestID("b")})
	})
	app := telatest.Mount(t, h.Div(h.Class("w-72"), kit.Input{AriaLabel: "Wide", EndContent: two, Props: h.TestID("in")}))
	in := app.Bounds(telatest.ByTestID("in"))
	a, b := app.Bounds(telatest.ByTestID("a")), app.Bounds(telatest.ByTestID("b"))
	// 6 px from the edge, the two 24 px buttons and a 4 px gap: 58 px, and a
	// 6 px gap to the text: pe-[64px].
	if got := in.X + in.Width - (b.X + b.Width); got != 6 {
		t.Errorf("the last button is %v px from the end, want 6", got)
	}
	if got := b.X - (a.X + a.Width); got != 4 {
		t.Errorf("the buttons are %v px apart, want 4", got)
	}
	app.ExpectCount(telatest.ByClass("peer", "pe-[64px]"), 1)

	// One icon-sized button keeps the recipe's pe-9.
	one := telatest.Mount(t, h.Div(h.Class("w-72"), kit.Input{AriaLabel: "One", EndContent: kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconEye, AriaLabel: "Show"}}))
	one.ExpectCount(telatest.ByClass("peer", "pe-9"), 1)
	one.ExpectCount(telatest.ByClass("peer", "pe-[36px]"), 0)
}

func TestInputEndPaddingFollowsAChangingContent(t *testing.T) {
	var set tela.Setter[bool]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		wide, s := tela.UseState(false)
		set = s
		var content tela.Component = kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconEye, AriaLabel: "Show"}
		if wide {
			content = tela.C(func() tela.Node {
				return h.Span(h.Class("px-2 text-sm"), h.TestID("label"), h.Text("Forgot password?"))
			})
		}
		return h.Div(h.Class("w-72"), kit.Input{AriaLabel: "Password", EndContent: content})
	}))
	app.ExpectCount(telatest.ByClass("peer", "pe-9"), 1)
	set.Set(true)
	w := app.Bounds(telatest.ByTestID("label")).Width
	want := fmt.Sprintf("pe-[%gpx]", math.Ceil(float64(w)+12))
	app.ExpectCount(telatest.ByClass("peer", want), 1)
	set.Set(false)
	app.ExpectCount(telatest.ByClass("peer", "pe-9"), 1)
	if n := app.Count(telatest.ByClass("peer", want)); n != 0 {
		t.Errorf("the padding %s of the wide content stayed after it left", want)
	}
}

func TestInputIconEndWithEndContent(t *testing.T) {
	app := telatest.Mount(t, h.Div(h.Class("w-72"), kit.Input{AriaLabel: "Both", IconEnd: iconX, Props: h.TestID("in"),
		EndContent: kit.Button{Variant: kit.Ghost, Size: kit.IconXs, Icon: iconEye, AriaLabel: "Show", Props: h.TestID("btn")}}))
	in := app.Bounds(telatest.ByTestID("in"))
	btn := app.Bounds(telatest.ByTestID("btn"))
	icon := app.Bounds(telatest.ByClass("text-muted-foreground", "size-4", "end-3"))
	if got := in.X + in.Width - (icon.X + icon.Width); got != 12 {
		t.Errorf("the icon is %v px from the end, want 12 (end-3)", got)
	}
	if got := icon.X - (btn.X + btn.Width); got != 8 {
		t.Errorf("the button is %v px before the icon, want 8", got)
	}
	// 36 px of the icon's slot, the 24 px button and a 6 px gap.
	app.ExpectCount(telatest.ByClass("peer", "pe-[66px]"), 1)
}

// An IconXs button draws a 12 px icon; IconClass makes it 16 px, as the
// reveal button of shadcn's InputGroup.
func TestIconXsButtonIconSize(t *testing.T) {
	app := telatest.Mount(t, h.Div(h.Class("flex gap-2"),
		kit.Button{Size: kit.IconXs, Icon: iconEye, IconClass: "text-red-500", AriaLabel: "Small", Props: h.TestID("small")},
		kit.Button{Size: kit.IconXs, Icon: iconEye, IconClass: "text-blue-500 size-4", AriaLabel: "Large", Props: h.TestID("large")}))
	small := app.Bounds(telatest.ByClass("text-red-500"))
	large := app.Bounds(telatest.ByClass("text-blue-500"))
	if small.Width != 12 || large.Width != 16 {
		t.Errorf("icons %v and %v px, want 12 and 16", small.Width, large.Width)
	}
	if b := app.Bounds(telatest.ByTestID("large")); b.Width != 24 {
		t.Errorf("the button is %v px wide, want 24", b.Width)
	}
}
