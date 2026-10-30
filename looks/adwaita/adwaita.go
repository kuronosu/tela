// Package adwaita is the look of GNOME (libadwaita): a header bar that holds
// the navigation, the location and the search entry, a sidebar of the same
// gray, rounded rows with a tinted selection, 6 and 8 px radii and the blue
// 3584E4 accent. The font is Adwaita Sans; the embedded Inter is the
// fallback, and goldens always get Inter.
//
// The header bar is drawn inside the client area, with a lead over the sidebar
// (Regions.ToolbarLead) for the sidebar toggle and the main menu. The OS title
// bar stays: Tela does not draw window controls here.
//
// The tokens beyond kit's are the ones of package looks: toolbar and sidebar
// are the grays of the header bar and the sidebar, and selection the tinted
// blue of a selected row.
package adwaita

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/internal/base"
)

// Look returns the look of GNOME. Each call returns a new one: its Style may
// be edited.
func Look() looks.Look {
	style := Style()
	return looks.Look{
		Name:  "GNOME",
		Theme: Theme(),
		Style: &style,
		Regions: looks.Regions{
			Window:            "text-[15px] leading-5",
			Toolbar:           "flex h-[47px] items-center gap-1.5 border-b border-border bg-toolbar pr-1.5",
			ToolbarLead:       "flex h-full w-[240px] shrink-0 items-center justify-between border-r border-border px-1.5",
			ToolbarGroup:      "flex items-center gap-px overflow-hidden rounded-lg bg-foreground/7",
			ToolbarEnd:        "flex shrink-0 items-center gap-0.5",
			Location:          "flex h-[34px] min-w-0 flex-1 items-center gap-0.5 overflow-hidden rounded-lg bg-foreground/7 p-[3px]",
			LocationCurrent:   "bg-foreground/8 font-bold",
			LocationSeparator: "flex shrink-0 items-center",
			Search:            "relative w-[250px] shrink-0",
			SearchIcon:        "pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground",
			Sidebar:           "flex w-[240px] shrink-0 flex-col gap-px overflow-y-auto border-r border-border bg-sidebar p-1.5",
			SidebarSeparator:  "mx-1 my-1.5",
			SidebarGroup:      "gap-px",
			Content:           "flex min-w-0 flex-1 flex-col overflow-hidden bg-card",
			Details:           "hidden w-[300px] shrink-0 flex-col gap-5 overflow-y-auto border-l border-border bg-background p-5 lg:flex",
			DetailsTitle:      "text-[17px] leading-6 font-bold break-words",
			DetailsLabel:      "text-[13px] text-muted-foreground",
			DetailsValue:      "text-[14px]",
			StatusBar:         "flex h-9 shrink-0 items-center justify-between gap-4 border-t border-border bg-background px-4 text-[13px] text-muted-foreground",
		},
	}
}

// Theme is libadwaita's palette: a light gray window with a white content
// area (a near black one in dark), translucent black or white over it, and
// the blue accent.
func Theme() tela.Theme {
	tok, black, white := base.Tok, base.Black, base.White
	blue := tela.Hex("#3584e4")
	return base.Theme(10, "Adwaita Sans", map[string]tela.SchemeColor{
		"background":           tok(tela.Hex("#fafafa"), tela.Hex("#242424")),
		"foreground":           tok(tela.Hex("#323232"), tela.Hex("#ffffff")),
		"card":                 tok(tela.Hex("#ffffff"), tela.Hex("#1e1e1e")),
		"card-foreground":      tok(tela.Hex("#323232"), tela.Hex("#ffffff")),
		"popover":              tok(tela.Hex("#ffffff"), tela.Hex("#383838")),
		"popover-foreground":   tok(tela.Hex("#323232"), tela.Hex("#ffffff")),
		"primary":              tok(blue, blue),
		"primary-foreground":   tok(tela.Hex("#ffffff"), tela.Hex("#ffffff")),
		"secondary":            tok(black(0.07), white(0.1)),
		"secondary-foreground": tok(tela.Hex("#323232"), tela.Hex("#ffffff")),
		"muted":                tok(black(0.07), white(0.1)),
		"muted-foreground":     tok(black(0.6), white(0.6)),
		"accent":               tok(black(0.06), white(0.08)),
		"accent-foreground":    tok(tela.Hex("#323232"), tela.Hex("#ffffff")),
		"destructive":          tok(tela.Hex("#e01b24"), tela.Hex("#ff7b63")),
		"border":               tok(black(0.12), white(0.1)),
		"input":                tok(black(0.12), white(0.1)),
		"ring":                 tok(blue, blue),

		"toolbar":              tok(tela.Hex("#ebebeb"), tela.Hex("#303030")),
		"sidebar":              tok(tela.Hex("#ebebeb"), tela.Hex("#2b2b2b")),
		"selection":            tok(tela.RGBA(53, 132, 228, 0.16), tela.RGBA(53, 132, 228, 0.3)),
		"selection-foreground": tok(tela.Hex("#323232"), tela.Hex("#ffffff")),
	})
}

// Style is what libadwaita makes of kit's recipes: 8 px buttons that darken on
// hover, a flat gray entry with a blue ring on focus, a list with rounded rows
// and bold column titles, menus that are rounded popovers and a dark tooltip.
func Style() kit.Style {
	const fs = "text-[15px]"
	s := base.Style(fs)

	b := &s.Button
	b.Variants[kit.Ghost] = "text-foreground hover:bg-foreground/7 active:bg-foreground/14"
	b.Sizes[kit.Icon] = "h-[34px] w-9 rounded-lg"
	b.Sizes[kit.Sm] = "h-7 gap-2 rounded-sm px-3"
	b.Sizes[""] = "h-9 rounded-lg px-4"
	b.HasIcon[""] = "px-3"
	b.Variants[kit.Outline] = "bg-foreground/8 text-foreground hover:bg-foreground/12"

	s.Input.Root = "h-[34px] w-full min-w-0 rounded-lg bg-foreground/7 pr-3 pl-9 " + fs + " outline-none " +
		"placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground " +
		"focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-50"

	s.Select.Root = "flex w-full items-center justify-between gap-2 rounded-lg bg-foreground/7 px-3 " + fs + " whitespace-nowrap outline-none " +
		"focus-visible:ring-2 focus-visible:ring-primary disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:ring-2 aria-invalid:ring-destructive"
	s.Select.Sizes[""] = "h-[34px]"
	s.Select.Sizes[kit.Sm] = "h-7"
	base.Swap("rounded-md border", "rounded-xl border", &s.Select.Content)
	base.Swap("rounded-sm", "rounded-lg", &s.Select.Item)
	base.Swap("p-1 ", "p-1.5 ", &s.Select.Viewport)

	t := &s.Table
	t.Root += " px-1.5 pb-1.5"
	t.Table += " gap-y-px"
	t.Head += " h-9 px-[14px] text-[13px] font-bold text-muted-foreground"
	t.SortButton = "-ml-2 h-7 px-2 text-[13px] font-bold text-muted-foreground hover:bg-transparent hover:text-foreground active:bg-transparent"
	t.Cell += " h-10 first:rounded-l-lg last:rounded-r-lg"
	t.Empty += " text-[17px] font-bold"

	g := &s.Toggle
	g.Variants[looks.Sidebar] = "relative h-9 w-full justify-start gap-3 rounded-lg px-3 text-left text-foreground hover:bg-foreground/6 checked:bg-foreground/10 checked:hover:bg-foreground/10"
	g.Sizes[looks.SidebarSize] = ""
	g.Variants[looks.Toolbar] = "rounded-lg text-foreground hover:bg-foreground/7 checked:bg-foreground/14"
	g.Sizes[looks.ToolbarSize] = "h-[34px] w-9"

	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		base.Swap("rounded-md border", "rounded-xl border", &m.Content)
		base.Swap("rounded-sm px-2", "rounded-lg px-3", &m.Item, &m.CheckboxItem, &m.RadioItem)
		base.Swap("p-1 ", "p-1.5 ", &m.Content)
	}
	base.Swap("rounded-lg border", "rounded-xl border", &s.Dialog.Root, &s.AlertDialog.Root)

	// GNOME's tooltips are small, dark and rounded.
	s.Tooltip.Root = "z-50 w-fit rounded-lg bg-foreground px-3 py-1.5 text-[13px] text-background " +
		"transition duration-150 starting:scale-95 starting:opacity-0"
	return s
}
