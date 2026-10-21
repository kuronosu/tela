package h_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// The width and aria variants of ADR-097, through the whole pipeline.

func boxWidth(app *telatest.App) float32 { return app.Bounds(telatest.ByTestID("box")).Width }

func TestWidthVariantsFollowTheWindow(t *testing.T) {
	page := func() tela.Node {
		return Div(Class("flex"),
			Div(TestID("box"), Class("h-4 w-10 max-sm:w-20 md:max-xl:w-30 min-[900px]:max-[1000px]:w-40 max-[20rem]:h-8")))
	}
	for _, tt := range []struct {
		width float32
		want  float32
	}{
		{300, 80}, {639, 80}, {640, 40}, {767, 40}, {768, 120}, {899, 120}, {900, 160}, {999, 160}, {1000, 120}, {1279, 120}, {1280, 40}, {2000, 40},
	} {
		app := telatest.MountFunc(t, page, telatest.Size(tt.width, 200))
		if got := boxWidth(app); got != tt.want {
			t.Errorf("window %gpx wide: the box is %gpx wide, want %g", tt.width, got, tt.want)
		}
		wantH := float32(16)
		if tt.width < 320 {
			wantH = 32
		}
		if got := app.Bounds(telatest.ByTestID("box")).Height; got != wantH {
			t.Errorf("window %gpx wide: the box is %gpx high, want %g (max-[20rem]:h-8)", tt.width, got, wantH)
		}
	}
}

func TestAriaVariantsMatchTheProps(t *testing.T) {
	type prop struct {
		name  string
		set   func(bool) tela.Prop
		class string
	}
	for _, p := range []prop{
		{"busy", AriaBusy, "aria-busy"},
		{"checked", AriaChecked, "aria-checked"},
		{"disabled", AriaDisabled, "aria-disabled"},
		{"expanded", AriaExpanded, "aria-expanded"},
		{"hidden", AriaHidden, "aria-hidden"},
		{"pressed", AriaPressed, "aria-pressed"},
		{"readonly", AriaReadOnly, "aria-readonly"},
		{"required", AriaRequired, "aria-required"},
		{"selected", AriaSelected, "aria-selected"},
	} {
		t.Run(p.name, func(t *testing.T) {
			page := func() tela.Node {
				on, setOn := tela.UseState(false)
				// The elements are measured through wrappers, since AriaHidden
				// takes its element out of the semantics tree that locators use.
				return Div(Class("flex flex-col"),
					Button(OnClick(tela.Toggle(setOn)), Text("Toggle")),
					Div(TestID("box"), Class("w-fit"),
						Div(p.set(on), Class("h-4 w-10 "+p.class+":w-20"))),
					Div(TestID("child"), Class("w-fit"),
						Div(Class("group"), p.set(on), Div(Class("size-4 group-"+p.class+":w-24")))),
					Div(TestID("sibling"), Class("flex w-fit"),
						Div(Class("peer"), p.set(on)),
						Div(Class("size-4 peer-"+p.class+":w-28"))),
				)
			}
			app := telatest.MountFunc(t, page, telatest.Size(300, 300))
			width := func(id string) float32 { return app.Bounds(telatest.ByTestID(id)).Width }
			if width("box") != 40 || width("child") != 16 || width("sibling") != 16 {
				t.Fatalf("before: box %g, child %g, sibling %g, want 40, 16, 16", width("box"), width("child"), width("sibling"))
			}
			app.Click(telatest.ByRole("button", "Toggle"))
			if width("box") != 80 || width("child") != 96 || width("sibling") != 112 {
				t.Errorf("true: box %g, child %g, sibling %g, want 80 (own), 96 (group), 112 (peer)", width("box"), width("child"), width("sibling"))
			}
			app.Click(telatest.ByRole("button", "Toggle"))
			if width("box") != 40 || width("child") != 16 || width("sibling") != 16 {
				t.Errorf("false again: box %g, child %g, sibling %g, want 40, 16, 16", width("box"), width("child"), width("sibling"))
			}
		})
	}
}

// The variants match the attribute selector [aria-x="true"]: not a state
// the element has implicitly, not false, not mixed.
func TestAriaVariantsMatchOnlyTrueProps(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(Class("flex flex-col"),
			// Disabled makes the button inert, but is not aria-disabled="true".
			Button(TestID("a"), Disabled(true), Class("h-4 w-10 aria-disabled:w-20"), Text("a")),
			Button(TestID("b"), AriaDisabled(true), Class("h-4 w-10 aria-disabled:w-20"), Text("b")),
			Div(TestID("c"), AriaChecked(false), Class("h-4 w-10 aria-checked:w-20")),
			Div(TestID("d"), Class("h-4 w-10 aria-checked:w-20")),
			Div(TestID("e"), Class("h-4 w-10 aria-[checked=true]:w-20 aria-[checked=true]:bg-red-500"), AriaChecked(true)),
		)
	}, telatest.Size(300, 300))
	for id, want := range map[string]float32{"a": 40, "b": 80, "c": 40, "d": 40, "e": 80} {
		if got := app.Bounds(telatest.ByTestID(id)).Width; got != want {
			t.Errorf("%s: width %g, want %g", id, got, want)
		}
	}
}

// A change of an aria prop restyles only the elements whose classes test
// it (ADR-055's rule), however many others the component renders.
func TestAriaVariantsRestyleOnlyTheirUsers(t *testing.T) {
	page := func() tela.Node {
		busy, setBusy := tela.UseState(false)
		return Div(Class("flex flex-col"),
			Button(OnClick(tela.Toggle(setBusy)), Text("Toggle")),
			Div(AriaBusy(busy), Class("h-4 w-10 aria-busy:w-20")),
			Div(AriaBusy(busy), Class("h-4 w-10")), // sets the prop, does not test it
			Div(Class("h-4 w-10 aria-expanded:w-20")),
			Div(Class("h-4 w-10")),
			Div(Class("h-4 w-10")),
			Div(Class("h-4 w-10")),
		)
	}
	app := telatest.MountFunc(t, page, telatest.Size(300, 300))
	st := app.Measure(func() { app.Click(telatest.ByRole("button", "Toggle")) })
	if st.Styles != 1 {
		t.Errorf("the click restyled %d elements, want 1 (only the one whose class tests aria-busy)", st.Styles)
	}
}

// Playwright's aria snapshot prints neither required, readonly nor busy
// (the semantics node holds them: internal/core's tests).
func TestNewAriaPropsAreNotInTheSnapshot(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return Div(
			Div(Role("textbox"), AriaLabel("Name"), AriaRequired(true), AriaReadOnly(true), AriaBusy(true)),
			Div(Role("textbox"), AriaLabel("Other"), AriaRequired(false)),
		)
	}, telatest.Size(300, 200))
	if snap := app.AriaSnapshot(); snap != "- textbox \"Name\"\n- textbox \"Other\"" {
		t.Errorf("snapshot %q", snap)
	}
}
