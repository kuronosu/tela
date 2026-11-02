package aqua_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/aqua"
	"github.com/kuronosu/tela/telatest"
)

// sheet is the controls the gallery does not show (radios, a segmented
// control, switches of both sizes, a toggle of each variant) and a card,
// laid over a colorful backdrop so that glass has something to show.
type sheet struct {
	Look   looks.Look
	Scheme tela.ColorScheme
}

func (s sheet) Build() tela.Node {
	radio, setRadio := tela.UseState("list")
	volume, setVolume := tela.UseState(40.0)
	pressed, setPressed := tela.UseState(true)
	group := func(title string, kids ...tela.Child) tela.Node {
		all := []tela.Child{Class("flex flex-col gap-2.5"), Key(title),
			Span(Class("text-[11px] font-semibold text-muted-foreground"), Text(title))}
		return Div(append(all, kids...)...)
	}
	row := func(kids ...tela.Child) tela.Node {
		return Div(append([]tela.Child{Class("flex flex-wrap items-center gap-2.5")}, kids...)...)
	}
	left := Div(Class("flex min-w-0 flex-1 flex-col gap-5"), Key("left"),
		group("Buttons", row(
			kit.Button{Children: Text("Continue")},
			kit.Button{Variant: kit.Secondary, Children: Text("Cancel")},
			kit.Button{Variant: kit.Destructive, Children: Text("Delete")},
			kit.Button{Variant: kit.Ghost, Children: Text("Ghost")},
			kit.Button{Size: kit.Sm, Children: Text("Small")},
			kit.Button{Disabled: true, Children: Text("Disabled")},
		)),
		group("Radios and checkboxes", row(
			kit.RadioGroup[string]{AriaLabel: "View as", Value: &radio, OnValueChange: setRadio.Set,
				Items: []kit.RadioItem[string]{{Value: "icons", Label: "as Icons"}, {Value: "list", Label: "as List"}, {Value: "columns", Label: "as Columns"}}},
			Div(Class("flex flex-col gap-2"),
				Div(Class("flex items-center gap-2"), kit.Checkbox{DefaultChecked: true, AriaLabel: "On", ID: "c1"}, kit.Label{For: "c1", Children: Text("On")}),
				Div(Class("flex items-center gap-2"), kit.Checkbox{AriaLabel: "Off", ID: "c2"}, kit.Label{For: "c2", Children: Text("Off")}),
				Div(Class("flex items-center gap-2"), kit.Checkbox{Disabled: true, AriaLabel: "Disabled", ID: "c3"}, kit.Label{For: "c3", Children: Text("Disabled")}),
			),
		)),
		group("Switches", row(
			kit.Switch{DefaultChecked: true, AriaLabel: "On"},
			kit.Switch{AriaLabel: "Off"},
			kit.Switch{Size: kit.Sm, DefaultChecked: true, AriaLabel: "Small on"},
			kit.Switch{Size: kit.Sm, AriaLabel: "Small off"},
		)),
		group("Toggles", row(
			kit.Toggle{Pressed: &pressed, OnPressedChange: setPressed.Set, Children: Text("Pressed")},
			kit.Toggle{Variant: kit.Outline, DefaultPressed: true, Children: Text("Outline on")},
			kit.Toggle{Variant: kit.Outline, Children: Text("Outline")},
		)),
	)
	right := Div(Class("flex min-w-0 flex-1 flex-col gap-5"), Key("right"),
		group("Segmented control",
			kit.Tabs[string]{DefaultValue: new("overview"), AriaLabel: "Sections", Tabs: []kit.Tab[string]{
				{Value: "overview", Label: "Overview"}, {Value: "activity", Label: "Activity"}, {Value: "settings", Label: "Settings"},
			}}),
		group("Fields",
			kit.Input{Placeholder: "Your name", AriaLabel: "Name", Class: "pl-3"},
			kit.Select[string]{DefaultValue: new("apple"), AriaLabel: "Fruit", Class: "w-full", Items: []kit.SelectItem[string]{
				{Value: "apple", Label: "Apple"}, {Value: "banana", Label: "Banana"}}}),
		group("Range",
			kit.Slider{Value: &volume, OnValueChange: setVolume.Set, Max: 100, AriaLabel: "Volume"},
			kit.Progress{Value: volume, AriaLabel: "Volume level"}),
		kit.Card{TitleLevel: 2, Title: Text("Storage"), Description: Text("62% of 10 GB used"),
			Children: kit.Progress{Value: 62, AriaLabel: "Storage used"},
			Footer:   Div(Class("flex gap-2"), kit.Button{Size: kit.Sm, Children: Text("Upgrade")}, kit.Button{Variant: kit.Ghost, Size: kit.Sm, Children: Text("Manage")})},
	)
	// What the glass shows: a sky of the accent, a pink and an orange blob.
	backdrop := Div(Class("pointer-events-none absolute inset-0 overflow-hidden"),
		Div(Class("absolute inset-0 bg-linear-to-br from-[#5db4ff] via-[#a98bff] to-[#ff8fb8] opacity-80 dark:opacity-50")),
		Div(Class("absolute top-6 right-10 size-56 rounded-full bg-[#ffb454] opacity-70 blur-3xl dark:opacity-35")),
		Div(Class("absolute bottom-6 left-24 size-64 rounded-full bg-[#ff6bd6] opacity-45 blur-3xl dark:opacity-25")),
	)
	return Div(Class("relative h-full w-full overflow-hidden bg-background text-foreground select-none", s.Look.Regions.Window), Scheme(s.Scheme),
		kit.StyleProvider{Style: s.Look.Style, Children: Div(Class("relative flex h-full w-full"), backdrop,
			Div(Class("relative flex w-full gap-6 p-6"), left, right))})
}

func mountSheet(t *testing.T, l looks.Look, s tela.ColorScheme) *telatest.App {
	t.Helper()
	return telatest.Mount(t, sheet{Look: l, Scheme: s}, telatest.Size(900, 560), telatest.Theme(l.Theme))
}

// TestSheetGoldens is the controls the gallery does not show, in each design
// and scheme, a mix, and Tahoe's glass cards.
func TestSheetGoldens(t *testing.T) {
	for _, c := range []struct {
		name string
		look looks.Look
	}{
		{"tahoe", aqua.Look(aqua.Tahoe)},
		{"sequoia", aqua.Look(aqua.Sequoia)},
		{"tahoe_glass", aqua.Look(aqua.Tahoe, aqua.GlassContent())},
		{"sequoia_tahoe_controls", aqua.Look(aqua.Sequoia, aqua.Controls(aqua.Tahoe))},
	} {
		for _, s := range []tela.ColorScheme{tela.Light, tela.Dark} {
			scheme := "light"
			if s == tela.Dark {
				scheme = "dark"
			}
			t.Run(c.name+"/"+scheme, func(t *testing.T) {
				mountSheet(t, c.look, s).Golden(fmt.Sprintf("sheet_%s_%s.png", c.name, scheme))
			})
		}
	}
}

// TestRadioChoosesAndSwitchToggles: the recipes of the radio and the switch
// keep their roles and states.
func TestRadioChoosesAndSwitchToggles(t *testing.T) {
	for _, d := range []aqua.Design{aqua.Tahoe, aqua.Sequoia} {
		t.Run(d.String(), func(t *testing.T) {
			a := mountSheet(t, aqua.Look(d), tela.Light)
			a.Expect(telatest.ByRole("radio", "as List"), telatest.Checked(true))
			a.Click(telatest.ByRole("radio", "as Icons"))
			a.Expect(telatest.ByRole("radio", "as Icons"), telatest.Checked(true))
			a.Click(telatest.ByRole("switch", "Off"))
			a.Expect(telatest.ByRole("switch", "Off"), telatest.Checked(true))
		})
	}
}
