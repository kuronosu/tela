// Package breeze is the look of KDE (Breeze, Plasma 6): a flat toolbar with
// highlight-framed hovers, a breadcrumb field, a Places panel with category
// headings, a solid 3DAEE9 selection with white text, 5 px radii, and a slim
// slider. The font is Noto Sans; the embedded Inter is the fallback, and
// goldens always get Inter.
//
// The tokens beyond kit's are the ones of package looks: field is the text
// field, and selection the solid highlight that Breeze gives every selected
// row, place and menu item.
package breeze

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/internal/base"
)

// Look returns the look of KDE. Each call returns a new one: its Style may be
// edited.
func Look() looks.Look {
	style := Style()
	return looks.Look{
		Name:  "KDE",
		Theme: Theme(),
		Style: &style,
		Regions: looks.Regions{
			Window:            "text-[13px] leading-[18px]",
			Toolbar:           "flex h-[46px] items-center gap-1.5 border-b border-border px-2",
			ToolbarGroup:      "flex items-center gap-0.5",
			ToolbarEnd:        "flex shrink-0 items-center gap-0.5",
			Location:          "flex h-[30px] min-w-0 flex-1 items-center overflow-hidden rounded-md border border-input bg-field px-1",
			LocationSeparator: "flex w-3 shrink-0 items-center justify-center text-muted-foreground",
			Search:            "relative w-[240px] shrink-0",
			SearchIcon:        "pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground",
			Sidebar:           "flex w-[210px] shrink-0 flex-col gap-px overflow-y-auto p-1.5",
			SidebarHeading:    "px-2 pt-3 pb-1 text-[12px] font-bold text-muted-foreground first:pt-1",
			SidebarGroup:      "gap-px",
			Content:           "flex min-w-0 flex-1 flex-col overflow-hidden border-l border-border bg-card",
			Details:           "hidden w-[260px] shrink-0 flex-col gap-3 overflow-y-auto border-l border-border bg-background p-3 lg:flex",
			DetailsTitle:      "text-[14px] leading-5 font-bold break-words",
			DetailsLabel:      "text-[12px] text-muted-foreground",
			DetailsValue:      "text-[13px]",
			StatusBar:         "flex h-8 shrink-0 items-center justify-between gap-4 border-t border-border bg-background px-3 text-[12px]",
		},
	}
}

// Theme is Breeze's palette: a blue-gray window, a near white (or near black)
// view, the 3DAEE9 highlight for the selection, hovers that are a tint of it
// and white text on it.
func Theme() tela.Theme {
	tok, black, white := base.Tok, base.Black, base.White
	blue := tela.Hex("#3daee9")
	return base.Theme(6.25, "Noto Sans", map[string]tela.SchemeColor{
		"background":           tok(tela.Hex("#eff0f1"), tela.Hex("#2a2e32")),
		"foreground":           tok(tela.Hex("#232627"), tela.Hex("#fcfcfc")),
		"card":                 tok(tela.Hex("#fcfcfc"), tela.Hex("#1b1e20")),
		"card-foreground":      tok(tela.Hex("#232627"), tela.Hex("#fcfcfc")),
		"popover":              tok(tela.Hex("#fcfcfc"), tela.Hex("#31363b")),
		"popover-foreground":   tok(tela.Hex("#232627"), tela.Hex("#fcfcfc")),
		"primary":              tok(blue, blue),
		"primary-foreground":   tok(tela.Hex("#ffffff"), tela.Hex("#ffffff")),
		"secondary":            tok(tela.Hex("#e3e5e6"), tela.Hex("#31363b")),
		"secondary-foreground": tok(tela.Hex("#232627"), tela.Hex("#fcfcfc")),
		"muted":                tok(black(0.15), white(0.2)),
		"muted-foreground":     tok(black(0.55), white(0.55)),
		"accent":               tok(tela.RGBA(61, 174, 233, 0.2), tela.RGBA(61, 174, 233, 0.22)),
		"accent-foreground":    tok(tela.Hex("#232627"), tela.Hex("#fcfcfc")),
		"destructive":          tok(tela.Hex("#da4453"), tela.Hex("#ed1515")),
		"border":               tok(black(0.1), white(0.1)),
		"input":                tok(tela.Hex("#bcc0c2"), white(0.2)),
		"ring":                 tok(blue, blue),

		"field":                tok(tela.Hex("#fcfcfc"), tela.Hex("#1b1e20")),
		"selection":            tok(blue, blue),
		"selection-foreground": tok(tela.Hex("#ffffff"), tela.Hex("#ffffff")),
	})
}

// Style is what Breeze makes of kit's recipes: toolbar buttons that frame
// themselves in the highlight color on hover, a field with a 1 px edge that
// becomes the highlight on focus, a solid highlight for the selection (in
// lists, places and menus) and a thin slider.
func Style() kit.Style {
	const fs = "text-[13px]"
	s := base.Style(fs)

	b := &s.Button
	b.Variants[kit.Ghost] = "border border-transparent text-foreground hover:border-primary hover:bg-accent active:bg-primary/35"
	b.Sizes[kit.Sm] = "h-6 gap-1.5 rounded-sm px-1.5"
	b.Variants[kit.Outline] = "border border-input bg-field text-foreground hover:border-primary hover:bg-accent"

	s.Input.Root = "h-[30px] w-full min-w-0 rounded-md border border-input bg-field pr-2 pl-7 " + fs + " outline-none " +
		"placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground " +
		"focus-visible:border-primary focus-visible:ring-1 focus-visible:ring-primary disabled:opacity-50"

	s.Select.Root = "flex w-full items-center justify-between gap-2 rounded-md border border-input bg-field px-2 " + fs + " whitespace-nowrap outline-none " +
		"focus-visible:border-primary focus-visible:ring-1 focus-visible:ring-primary disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive"
	s.Select.Sizes[""] = "h-[30px]"
	s.Select.Sizes[kit.Sm] = "h-6"
	base.Swap("focus:bg-accent focus:text-accent-foreground", "focus:bg-selection focus:text-selection-foreground", &s.Select.Item)

	t := &s.Table
	t.Root += " p-1"
	t.Head += " h-7 border-b border-border px-2.5 text-[12px]"
	t.HeadSticky += " "
	t.SortButton = "-ml-2 h-5 px-2 text-[12px] hover:bg-accent"
	t.Cell += " h-7 first:rounded-l-md last:rounded-r-md"
	t.Empty += " text-[15px]"

	g := &s.Toggle
	g.Variants[looks.Sidebar] = "relative h-7 w-full justify-start gap-2.5 rounded-md px-2 text-left text-foreground hover:bg-accent checked:bg-selection checked:text-selection-foreground checked:hover:bg-selection"
	g.Sizes[looks.SidebarSize] = ""
	g.Variants[looks.Toolbar] = "rounded-md border border-transparent text-foreground hover:border-primary hover:bg-accent checked:bg-primary/35"
	g.Sizes[looks.ToolbarSize] = "size-8"

	// Menus highlight with the solid selection, as every Breeze list does.
	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		base.Swap("focus:bg-accent focus:text-accent-foreground", "focus:bg-selection focus:text-selection-foreground", &m.Item, &m.CheckboxItem, &m.RadioItem)
		m.ItemIconVariants[""] = "text-current"
		m.Shortcut = "ml-auto text-xs tracking-widest opacity-60"
	}

	// The slider: a thin groove with the highlight to the left of a round
	// handle.
	z := &s.Slider
	z.Track = "relative grow overflow-hidden rounded-full bg-muted"
	z.TrackOrientations[kit.Horizontal] = "h-1 w-full"
	z.Range = "absolute bg-primary"
	z.Thumb = "block size-3.5 shrink-0 rounded-full border border-input bg-field transition-[color,box-shadow] hover:border-primary focus-visible:border-primary focus-visible:outline-hidden"
	return s
}
