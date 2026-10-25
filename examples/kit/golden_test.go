package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// TestGolden renders the gallery and a few representative pages, light and
// dark at scale 1, bit by bit:
// `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGolden ./examples/kit`
// writes the pictures, which must be looked at before they are committed.
// The pages chosen are the ones with no animation running (a skeleton or an
// indeterminate bar would pulse), and between them they cover the kit's text
// fields, choices, dates, color, tables and tabs.
func TestGolden(t *testing.T) {
	for _, name := range []string{"Gallery", "Gallery Rounded", "Button", "Card", "Field", "Calendar", "Table", "Tabs", "ColorPicker", "Select", "ToggleGroup"} {
		for _, s := range schemes {
			t.Run(fmt.Sprintf("%s/%s", name, s.name), func(t *testing.T) {
				a := mount(t, Previews[name], s.opts...)
				a.GoldenScales(fmt.Sprintf("kit_%s_%s", slug(name), s.name), 1)
			})
		}
	}
}

// TestGoldenOpen are the overlays open: a modal dialog, a dropdown menu with
// its checkboxes and radios, a date picker with its calendar, a select with
// its groups, a toast and the focus ring of the sidebar reached with the
// keyboard.
func TestGoldenOpen(t *testing.T) {
	for _, s := range schemes {
		t.Run("dialog/"+s.name, func(t *testing.T) {
			a := mount(t, Canvas{Page: "Dialog"}, s.opts...)
			a.Click(telatest.ByRole("button", "Edit profile"))
			a.ExpectRole("dialog", "Edit profile")
			a.GoldenScales("kit_dialog_open_"+s.name, 1)
		})
		t.Run("dropdown/"+s.name, func(t *testing.T) {
			a := mount(t, Canvas{Page: "DropdownMenu"}, s.opts...)
			a.Click(telatest.ByRole("button", "View"))
			a.ExpectRole("menu", "View")
			a.GoldenScales("kit_dropdown_open_"+s.name, 1)
		})
		t.Run("datepicker/"+s.name, func(t *testing.T) {
			a := mount(t, Canvas{Page: "DatePicker"}, s.opts...)
			a.Click(telatest.ByRole("button", "Date"))
			a.ExpectRole("grid", "October 2026")
			a.GoldenScales("kit_datepicker_open_"+s.name, 1)
		})
		t.Run("select/"+s.name, func(t *testing.T) {
			a := mount(t, Canvas{Page: "Select"}, s.opts...)
			a.Click(telatest.ByRole("button", "Select a timezone Timezone"))
			a.ExpectRole("listbox", "Timezone")
			a.GoldenScales("kit_select_open_"+s.name, 1)
		})
		t.Run("toast/"+s.name, func(t *testing.T) {
			a := mount(t, Canvas{Page: "Toast"}, s.opts...)
			a.Click(telatest.ByRole("button", "Error"))
			a.ExpectText("The upload failed")
			a.GoldenScales("kit_toast_"+s.name, 1)
		})
	}
	t.Run("rounded dialog", func(t *testing.T) {
		a := mount(t, Gallery{Page: "Dialog", Style: "rounded"})
		a.Click(telatest.ByRole("button", "Edit profile"))
		a.ExpectRole("dialog", "Edit profile")
		a.GoldenScales("kit_dialog_open_rounded_light", 1)
	})
	t.Run("sidebar focus", func(t *testing.T) {
		a := mount(t, Gallery{Page: "Card"})
		a.Press("Tab")
		a.Press("Tab")
		a.Press("Tab")
		a.ExpectFocused(tabFor("Card"))
		a.GoldenScales("kit_sidebar_focus_light", 1)
	})
}

// slug turns a preview name such as "Gallery Rounded" into "gallery_rounded".
func slug(name string) string {
	out := []rune{}
	for _, r := range name {
		switch {
		case r == ' ':
			out = append(out, '_')
		case r >= 'A' && r <= 'Z':
			out = append(out, r+'a'-'A')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}
