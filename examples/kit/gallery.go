package main

import (
	"cmp"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// page is one component's page of the gallery.
type page struct {
	// Name is the key of the page: the tab's value and the name of its
	// preview ("DatePicker").
	Name string
	// Label is the sidebar's text ("Date Picker").
	Label string
	// Code is the kit API the page is about, shown under the title.
	Code string
	// Summary is one sentence about the component.
	Summary string
	// Body is the demos.
	Body tela.Component
}

// home is the page the gallery opens on.
const home = "Button"

// find returns the page called name, or the home page.
func find(name string) page {
	for _, p := range pages {
		if p.Name == name {
			return p
		}
	}
	if name != home {
		return find(home)
	}
	return pages[0]
}

// Gallery is the whole window: a header with the Style picker and the dark
// switch, a sidebar with a tab for each component and the page of the
// selected one. Page and Style seed its state (a preview starts on the
// page it names); both are the names the sidebar and the picker use.
//
// The sidebar is a vertical kit.Tabs: one tab stop, the arrow keys move
// through the components and select them, Home and End jump, and Tab goes on
// to the page. The style is provided to everything below the header by
// kit.StyleProvider, the scheme is the app's (tela.SetColorScheme), and the
// Toaster around the page is the one the Toast page's buttons show toasts in.
type Gallery struct {
	Page  string
	Style string
}

func (g Gallery) Build() tela.Node {
	current, setCurrent := tela.UseState(find(g.Page).Name)
	styleName, setStyle := tela.UseState(cmp.Or(g.Style, "default"))
	dark := tela.UseColorScheme() == tela.Dark

	tabs := make([]kit.Tab[string], len(pages))
	for i, p := range pages {
		tabs[i] = kit.Tab[string]{Value: p.Name, Label: p.Label, Content: PageView{Name: p.Name}}
	}

	return Fragment(kit.StyleProvider{
		Style: styles[styleName],
		Children: kit.Toaster{Children: Div(Class("flex h-screen flex-col overflow-hidden bg-background text-foreground"),
			Header{Style: styleName, SetStyle: setStyle.Set, Dark: dark},
			kit.Tabs[string]{
				Tabs:          tabs,
				Value:         &current,
				OnValueChange: setCurrent.Set,
				Orientation:   kit.Vertical,
				AriaLabel:     "Components",
				Class:         "min-h-0 flex-1 gap-0",
				ListClass: "h-full w-56 shrink-0 items-stretch justify-start gap-0.5 overflow-y-auto rounded-none border-r border-border " +
					"bg-transparent p-3",
				TriggerClass: "h-8 flex-none px-3",
				ContentClass: "min-w-0 overflow-y-auto",
			},
		)},
	})
}

// Header is the top bar: the name, the Style picker and the dark switch.
type Header struct {
	Style    string
	SetStyle func(string)
	Dark     bool
}

func (h Header) Build() tela.Node {
	return Fragment(Div(Role("banner"), Class("flex h-14 shrink-0 items-center gap-3 border-b border-border px-4"),
		Div(Class("flex items-center gap-2.5 pr-2"),
			Div(Class("flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"),
				ic(icons.LayoutGrid, "size-4")),
			Span(Class("text-sm font-semibold tracking-tight"), Text("Tela kit")),
			kit.Badge{Variant: kit.Secondary, Children: Textf("%d components", len(pages))},
		),
		Div(Class("flex-1")),
		Div(Class("flex items-center gap-2"),
			Span(Class("text-sm text-muted-foreground"), Text("Style")),
			kit.ToggleGroup[string]{
				AriaLabel: "Style", Variant: kit.Outline, Size: kit.Sm, DisallowEmptySelection: true,
				Value: &h.Style, OnValueChange: h.SetStyle,
				Items: []kit.ToggleGroupItem[string]{
					{Value: "default", Label: "Default"},
					{Value: "rounded", Label: "Rounded"},
				},
			},
		),
		kit.Separator{Orientation: kit.Vertical, Class: "mx-1 h-5"},
		Div(Class("flex items-center gap-2"),
			ic(icons.Sun, "size-4 text-muted-foreground"),
			kit.Switch{
				ID: "gallery-dark", AriaLabel: "Dark mode", Checked: &h.Dark,
				OnCheckedChange: func(on bool) {
					if on {
						tela.SetColorScheme(tela.Dark)
					} else {
						tela.SetColorScheme(tela.Light)
					}
				},
			},
			ic(icons.Moon, "size-4 text-muted-foreground"),
		),
	))
}

// PageView is the page of a component: its name, what it is, and the demos.
type PageView struct{ Name string }

func (v PageView) Build() tela.Node {
	p := find(v.Name)
	return Fragment(Div(Key(p.Name), Class("mx-auto flex w-full max-w-6xl flex-col gap-8 px-8 py-8"),
		Div(Class("flex flex-col gap-2"),
			H1(Class("text-3xl font-bold tracking-tight"), Text(p.Label)),
			P(Class("max-w-2xl text-base text-muted-foreground"), Text(p.Summary)),
			Div(Class("flex"), kit.Badge{Variant: kit.Outline, Class: "font-mono", Children: Text(p.Code)}),
		),
		p.Body,
	))
}

// Canvas is the page of a component on the window's background and under
// the Toaster, without the sidebar: what a preview shows.
type Canvas struct {
	Page  string
	Style string
}

func (c Canvas) Build() tela.Node {
	return Fragment(kit.StyleProvider{
		Style: styles[cmp.Or(c.Style, "default")],
		Children: kit.Toaster{Children: Div(Class("h-screen overflow-y-auto bg-background text-foreground"),
			PageView{Name: find(c.Page).Name})},
	})
}
