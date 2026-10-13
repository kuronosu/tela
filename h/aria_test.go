package h_test

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
)

// toolbar uses every semantics prop through the public API.
func toolbar() tela.Node {
	title := tela.UseElementRef()
	icon := image.NewRGBA(image.Rect(0, 0, 8, 8))
	return Div(Class("flex flex-col gap-2 p-4"),
		Div(Role("toolbar"), AriaLabel("Format"), Class("flex gap-1"),
			Button(AriaPressed(true), Text("B")),
			Button(AriaPressed(false), AriaDescription("Italic text"), Text("I")),
			Button(AriaLabel("Close"), Img(Src(icon), Alt(""))),
			Button(AriaExpanded(true), Text("More"), Span(AriaHidden(true), Text("▾"))),
		),
		Div(Role("dialog"), AriaLabelledBy(title),
			H2(Ref(title), Text("Rename")),
			Div(Role("slider"), AriaLabel("Size"), AriaValueNow(3), AriaValueMin(1), AriaValueMax(5), AriaValueText("Medium")),
			Div(Role("tab"), AriaSelected(true), Text("General")),
			Div(Role("heading"), AriaLevel(4), Text("Advanced")),
			Div(Role("checkbox"), AriaChecked(true), AriaDisabled(true), Text("Keep extension")),
			Img(Src(icon), Alt("Preview")),
		),
	)
}

func TestSemanticsProps(t *testing.T) {
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: 600, H: 400})
	tr.Mount(tela.C(toolbar))
	tr.Frame()
	want := strings.Join([]string{
		`- toolbar "Format":`,
		`  - button "B" [pressed]`,
		`  - button "I"`,
		`  - button "Close"`,
		`  - button "More" [expanded]`,
		`- dialog "Rename":`,
		`  - heading "Rename" [level=2]`,
		`  - slider "Size"`,
		`  - tab "General" [selected]`,
		`  - heading "Advanced" [level=4]`,
		`  - checkbox "Keep extension" [checked] [disabled]`,
		`  - img "Preview"`,
	}, "\n")
	if got := tr.AriaSnapshot(); got != want {
		t.Errorf("aria snapshot:\n%s\nwant:\n%s", got, want)
	}
}

func TestSemanticsPropPanics(t *testing.T) {
	recovered := func(f func()) (msg string) {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
		}()
		f()
		return ""
	}
	if msg := recovered(func() { Div(Alt("x")) }); !strings.Contains(msg, "only Img") {
		t.Errorf("Alt on a Div: %q", msg)
	}
	if msg := recovered(func() { AriaLabelledBy(tela.ElementRef{}) }); !strings.Contains(msg, "zero ElementRef") {
		t.Errorf("AriaLabelledBy(zero): %q", msg)
	}
	core.NewTree(core.Config{Dev: true}) // dev mode on in this process
	if msg := recovered(func() { Role("swich") }); !strings.Contains(msg, `did you mean "switch"?`) {
		t.Errorf("Role(\"swich\") in dev mode: %q", msg)
	}
}
