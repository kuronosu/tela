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
	"github.com/kuronosu/tela/internal/semantics"
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

// A control named by several elements: the name is their texts joined by a
// space, in the order given.
func TestAriaLabelledByMany(t *testing.T) {
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: 600, H: 400})
	tr.Mount(tela.C(func() tela.Node {
		value, label := tela.UseElementRef(), tela.UseElementRef()
		return Div(
			Span(Ref(label), Text("Favorite fruit")),
			Button(AriaLabelledBy(value, label), Span(Ref(value), Text("Banana"))),
			Button(AriaLabelledBy(label), Text("Other")),
		)
	}))
	tr.Frame()
	want := strings.Join([]string{
		`- text: Favorite fruit`,
		`- button "Banana Favorite fruit": Banana`,
		`- button "Favorite fruit": Other`,
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
	if msg := recovered(func() { AriaLabelledBy() }); !strings.Contains(msg, "no references") {
		t.Errorf("AriaLabelledBy(): %q", msg)
	}
	core.NewTree(core.Config{Dev: true}) // dev mode on in this process
	if msg := recovered(func() { Role("swich") }); !strings.Contains(msg, `did you mean "switch"?`) {
		t.Errorf("Role(\"swich\") in dev mode: %q", msg)
	}
}

// TestAccessibilityProps: the props of the OS accessibility bridges
// (ADR-246) reach the semantics tree through the public API.
func TestAccessibilityProps(t *testing.T) {
	var n *tela.Value[int]
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: 600, H: 400})
	tr.Mount(tela.C(func() tela.Node {
		n = tela.UseValue(3)
		return Div(
			Div(AriaLive("polite"), AriaAtomic(true), AriaLabelFn(func() string { return fmt.Sprintf("%d results", n.Get()) })),
			Button(AriaKeyShortcuts("Control+S"), Text("Save")),
			Div(Role("listbox"), AriaLabel("L"), AriaSetSize(50),
				Div(Role("option"), AriaPosInSet(7), Text("Seven"))),
		)
	}))
	tr.Frame()
	var region, save, seven bool
	n.Set(4)
	tr.ApplyUpdates()
	tr.Semantics().Walk(func(x *semantics.Node) bool {
		switch {
		case x.Name == "4 results":
			region = x.Live == "polite" && x.Atomic && x.LiveRoot == x.ID
		case x.Name == "Save":
			save = x.KeyShortcuts == "Control+S"
		case x.Name == "Seven":
			seven = x.PosInSet == 7 && x.SetSize == 50
		}
		return true
	})
	if !region || !save || !seven {
		t.Errorf("region %v, shortcut %v, set position %v:\n%s", region, save, seven, tr.AriaSnapshot())
	}
}
