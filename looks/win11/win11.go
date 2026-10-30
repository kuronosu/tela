// Package win11 is the look of Windows 11 (Fluent): Mica gray around a white
// content pane with a rounded corner, a command bar, an address bar and a
// search box with Fluent's thin bottom edge, 4 px radii and the blue 0067C0
// accent (60CDFF in dark). The font is Segoe UI Variable Text; the embedded
// Inter is the fallback, and goldens always get Inter.
//
// The tokens beyond kit's are the ones of package looks: toolbar and sidebar
// follow the window, field is the text box, edge the line under it, and
// selection the light blue of a selected row.
package win11

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/internal/base"
)

// Look returns the look of Windows 11. Each call returns a new one: its Style
// may be edited.
func Look() looks.Look {
	style := Style()
	return looks.Look{
		Name:  "Windows 11",
		Theme: Theme(),
		Style: &style,
		Regions: looks.Regions{
			Window:            "text-[12px] leading-4",
			Toolbar:           "flex items-center gap-2 px-2 py-1",
			ToolbarGroup:      "flex items-center",
			ToolbarEnd:        "flex shrink-0 items-center gap-0.5",
			Location:          "flex h-8 min-w-0 flex-1 items-center overflow-hidden rounded-md border border-input border-b-edge bg-field px-1",
			LocationSeparator: "flex w-3 shrink-0 items-center justify-center text-muted-foreground",
			Search:            "relative w-[300px] shrink-0",
			SearchIcon:        "pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground",
			CommandBar:        "flex h-10 shrink-0 items-center gap-0.5 px-2",
			CommandSeparator:  "mx-1.5 h-5",
			Sidebar:           "flex w-[240px] shrink-0 flex-col gap-0.5 overflow-y-auto px-2 py-1",
			SidebarSeparator:  "mx-1 my-1.5",
			SidebarMark:       "absolute top-2 left-0 h-4 w-[3px] rounded-full bg-primary",
			SidebarGroup:      "gap-0.5",
			Content:           "flex min-w-0 flex-1 flex-col overflow-hidden rounded-tl-[8px] border-t border-l border-border bg-card",
			Details:           "hidden w-[300px] shrink-0 flex-col gap-5 overflow-y-auto border-l border-border bg-card p-5 lg:flex",
			DetailsTitle:      "text-[16px] leading-5 font-semibold break-words",
			DetailsLabel:      "text-muted-foreground",
			StatusBar:         "flex h-7 shrink-0 items-center justify-between gap-4 px-3",
		},
	}
}

// Theme is Fluent's palette: Mica for the window, white (a deep gray in dark)
// for the content pane, translucent black or white for hovers and hairlines,
// and one blue accent.
func Theme() tela.Theme {
	tok, black, white := base.Tok, base.Black, base.White
	return base.Theme(5, "Segoe UI Variable Text", map[string]tela.SchemeColor{
		"background":           tok(tela.Hex("#f3f3f3"), tela.Hex("#202020")),
		"foreground":           tok(tela.Hex("#1b1b1b"), tela.Hex("#ffffff")),
		"card":                 tok(tela.Hex("#ffffff"), tela.Hex("#191919")),
		"card-foreground":      tok(tela.Hex("#1b1b1b"), tela.Hex("#ffffff")),
		"popover":              tok(tela.Hex("#fbfbfb"), tela.Hex("#2c2c2c")),
		"popover-foreground":   tok(tela.Hex("#1b1b1b"), tela.Hex("#ffffff")),
		"primary":              tok(tela.Hex("#0067c0"), tela.Hex("#60cdff")),
		"primary-foreground":   tok(tela.Hex("#ffffff"), tela.Hex("#000000")),
		"secondary":            tok(tela.Hex("#f6f6f6"), tela.Hex("#2b2b2b")),
		"secondary-foreground": tok(tela.Hex("#1b1b1b"), tela.Hex("#ffffff")),
		"muted":                tok(black(0.06), white(0.08)),
		"muted-foreground":     tok(black(0.62), white(0.7)),
		"accent":               tok(black(0.05), white(0.06)),
		"accent-foreground":    tok(tela.Hex("#1b1b1b"), tela.Hex("#ffffff")),
		"destructive":          tok(tela.Hex("#c42b1c"), tela.Hex("#ff6b5e")),
		"border":               tok(black(0.1), white(0.09)),
		"input":                tok(black(0.08), white(0.08)),
		"ring":                 tok(tela.Hex("#0067c0"), tela.Hex("#60cdff")),

		"field":                tok(tela.Hex("#ffffff"), tela.Hex("#2b2b2b")),
		"edge":                 tok(black(0.35), white(0.5)),
		"selection":            tok(tela.Hex("#cce8ff"), tela.Hex("#2b3e52")),
		"selection-foreground": tok(tela.Hex("#1b1b1b"), tela.Hex("#ffffff")),
	})
}

// Style is what Fluent makes of kit's recipes: 4 px radii, ghost buttons that
// gray on hover, a field with a thin edge under it that turns into a blue line
// on focus, a list whose rows are rounded and hover gray, and a sidebar with a
// blue mark on the selected place.
func Style() kit.Style {
	const fs = "text-[12px]"
	s := base.Style(fs)

	// Search and rename fields: the Fluent text box.
	s.Input.Root = "h-8 w-full min-w-0 rounded-md border border-input border-b-edge bg-field pr-2 pl-8 " + fs + " outline-none " +
		"placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground " +
		"focus-visible:border-b-2 focus-visible:border-b-primary disabled:opacity-50"

	// The select takes the text box, and its list the flyout.
	s.Select.Root = "flex w-full items-center justify-between gap-2 rounded-md border border-input border-b-edge bg-field px-3 " + fs + " whitespace-nowrap outline-none " +
		"focus-visible:border-b-2 focus-visible:border-b-primary disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive"
	s.Select.Sizes[""] = "h-8"
	s.Select.Sizes[kit.Sm] = "h-6"
	base.Swap("rounded-md border", "rounded-lg border", &s.Select.Content)
	base.Swap("rounded-sm", "rounded-[4px]", &s.Select.Item)

	// The list: header cells with the sort button, rows 30 px high.
	t := &s.Table
	t.Root += " px-2 pb-2"
	t.Head += " h-8 font-normal text-foreground"
	t.SortButton = "-ml-2 h-6 px-2 font-normal"
	t.Cell += " h-[30px] first:rounded-l-md last:rounded-r-md"
	t.Empty += " text-[13px]"

	// The sidebar: full width entries with a gray selection.
	g := &s.Toggle
	g.Variants[looks.Sidebar] = "relative h-8 w-full justify-start gap-3 rounded-md px-3 text-left text-foreground hover:bg-accent active:bg-foreground/4 checked:bg-foreground/6 checked:hover:bg-foreground/6"
	g.Sizes[looks.SidebarSize] = ""
	// The view switcher of the status bar.
	g.Variants[looks.Toolbar] = "rounded-[3px] text-foreground hover:bg-foreground/6 checked:bg-foreground/10"
	g.Sizes[looks.ToolbarSize] = "size-6"

	// Menus: Fluent flyouts have round corners and a subtle item hover.
	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		base.Swap("rounded-md border", "rounded-lg border", &m.Content)
		base.Swap("rounded-sm px-2", "rounded-[4px] px-2.5", &m.Item, &m.CheckboxItem, &m.RadioItem)
	}
	return s
}
