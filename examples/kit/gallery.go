package main

import (
	"cmp"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
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
	// Bare is a page that draws its own heading instead of the title, the
	// summary and the code badge above its body: the Introduction.
	Bare bool
}

// home is the page the gallery opens on.
const home = "Introduction"

// find returns the page called name, or the home page.
func find(name string) page {
	all := sidebar()
	for _, p := range all {
		if p.Name == name {
			return p
		}
	}
	if name != home {
		return find(home)
	}
	return all[0]
}

// navigate hands the pages the setter of the selected page, so that a page
// can send the user to another one (the Introduction's list of components).
// The zero Setter, outside a Gallery or a Canvas, goes nowhere.
var navigate = tela.CreateContext(tela.Setter[string]{})

// Shell is the box that holds a screen of the gallery and sets its
// direction from the language of the window: right to left in Arabic, so
// the header, the sidebar and the page mirror.
type Shell struct {
	Class    string
	Children tela.Node
}

func (s Shell) Build() tela.Node {
	return Div(Dir(i18n.UseFormatter().Dir().String()), Class(s.Class), s.Children)
}

// Gallery is the whole window: a header with the Style picker, the dark
// switch and the Language menu, a sidebar with a tab for each page and the
// selected one. Page, Style and Locale seed its state (a preview starts on
// the page it names, in the style and language it names); they are the names
// the sidebar, the picker and the menu use ("rounded", "es").
//
// The language is the gallery's own: it is held in a state and given to
// i18n.Provider, not to i18n.SetLocale, so it never depends on the system's
// language and a test cannot disturb another. The window is laid out in the
// direction of that language.
//
// The sidebar is a vertical kit.Tabs: one tab stop, the arrow keys move
// through the components and select them, Home and End jump, and Tab goes on
// to the page. The style is provided to everything below the header by
// kit.StyleProvider, the scheme is the app's (tela.SetColorScheme), and the
// Toaster around the page is the one the Toast page's buttons show toasts in.
type Gallery struct {
	Page   string
	Style  string
	Locale string
}

func (g Gallery) Build() tela.Node {
	current, setCurrent := tela.UseState(find(g.Page).Name)
	styleName, setStyle := tela.UseState(cmp.Or(g.Style, "default"))
	lang, setLang := tela.UseState(cmp.Or(g.Locale, "en"))
	dark := tela.UseColorScheme() == tela.Dark

	all := sidebar()
	tabs := make([]kit.Tab[string], len(all))
	for i, p := range all {
		tabs[i] = kit.Tab[string]{Value: p.Name, Label: p.Label, Content: PageView{Name: p.Name}}
	}

	return i18n.Provider(lang, navigate.Provider(setCurrent, kit.StyleProvider{
		Style: styles[styleName],
		Children: kit.Toaster{Children: Shell{Class: "flex h-screen flex-col overflow-hidden bg-background text-foreground", Children: Fragment(
			Header{Style: styleName, SetStyle: setStyle.Set, Dark: dark, Lang: lang, SetLang: setLang.Set},
			kit.Tabs[string]{
				Tabs:          tabs,
				Value:         &current,
				OnValueChange: setCurrent.Set,
				Orientation:   kit.Vertical,
				AriaLabel:     "Pages",
				Class:         "min-h-0 flex-1 gap-0",
				ListClass: "h-full w-56 shrink-0 items-stretch justify-start gap-0.5 overflow-y-auto rounded-none border-e border-border " +
					"bg-transparent p-3",
				TriggerClass: "h-8 flex-none px-3",
				ContentClass: "min-w-0 overflow-y-auto",
			},
		)}},
	}))
}

// Header is the top bar: the name, the Style picker, the dark switch and the
// Language menu. Its own words stay English; the menu names each language in
// itself.
type Header struct {
	Style    string
	SetStyle func(string)
	Dark     bool
	Lang     string
	SetLang  func(string)
}

func (h Header) Build() tela.Node {
	return Fragment(Div(Role("banner"), Class("flex h-14 shrink-0 items-center gap-3 border-b border-border px-4"),
		Div(Class("flex items-center gap-2.5 pr-2"),
			Div(Class("flex size-8 items-center justify-center rounded-lg bg-primary text-primary-foreground"),
				ic(icons.LayoutGrid, "size-4")),
			Span(Class("text-sm font-semibold tracking-tight"), Text("Tela kit")),
			kit.Badge{Variant: kit.Secondary, Children: Span(Dir("ltr"), Textf("%d components", len(pages)))},
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
		kit.Separator{Orientation: kit.Vertical, Class: "mx-1 h-5"},
		Div(Class("flex items-center gap-2"),
			ic(icons.Languages, "size-4 text-muted-foreground"),
			languageSelect(h.Lang, h.SetLang),
		),
	))
}

// languageSelect is the Language menu: each option is named in its own
// language, and the trigger is named by the current one followed by the
// catalog's word for "Language".
func languageSelect(current string, set func(string)) tela.Component {
	items := make([]kit.SelectItem[string], len(languages))
	for i, tag := range languages {
		items[i] = kit.SelectItem[string]{Value: tag, Label: i18n.For(tag).T("language")}
	}
	return kit.Select[string]{
		Items: items, Value: &current, OnValueChange: set,
		AriaLabel: i18n.For(current).T("languageLabel"), Class: "w-36", Props: TestID("language"),
	}
}

// PageView is the page of a component: its name, what it is, and the demos.
type PageView struct{ Name string }

func (v PageView) Build() tela.Node {
	p := find(v.Name)
	if p.Bare {
		return Fragment(Div(Key(p.Name), Class("mx-auto flex w-full max-w-6xl flex-col px-8 py-8"), p.Body))
	}
	return Fragment(Div(Key(p.Name), Class("mx-auto flex w-full max-w-6xl flex-col gap-8 px-8 py-8"),
		Div(Dir("ltr"), Class("flex flex-col gap-2"),
			H1(Class("text-3xl font-bold tracking-tight"), Text(p.Label)),
			P(Class("max-w-2xl text-base text-muted-foreground"), Text(p.Summary)),
			Div(Class("flex"), kit.Badge{Variant: kit.Outline, Class: "font-mono", Children: Text(p.Code)}),
		),
		p.Body,
	))
}

// Canvas is a page on the window's background and under the Toaster,
// without the sidebar: what a preview shows. Locale fixes its language
// (English when empty) and so its direction. A page that sends the user to
// another one (the Introduction's list) changes the Canvas to it.
type Canvas struct {
	Page   string
	Style  string
	Locale string
}

func (c Canvas) Build() tela.Node {
	current, setCurrent := tela.UseState(find(c.Page).Name)
	return i18n.Provider(cmp.Or(c.Locale, "en"), navigate.Provider(setCurrent, kit.StyleProvider{
		Style: styles[cmp.Or(c.Style, "default")],
		Children: kit.Toaster{Children: Shell{Class: "h-screen overflow-y-auto bg-background text-foreground",
			Children: Fragment(PageView{Name: current})}},
	}))
}
