package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/native/icons"
	"github.com/kuronosu/tela/kit"
)

// adwaita is Files (Nautilus) in libadwaita: a header bar that holds the
// navigation, the path bar and the search entry, a sidebar of the same gray,
// rounded rows with a tinted selection, 6 and 8 px radii and the blue
// 3584E4 accent. The header bar is drawn inside the client area.
func adwaita() *Look {
	style := adwaitaStyle()
	return &Look{
		ID:    "adwaita",
		Title: "Files",
		Name:  "GNOME",
		Theme: adwaitaTheme(),
		Style: &style,
		Frame: Frame{
			Window:         "text-[15px] leading-5",
			ToolbarRow:     "flex h-[47px] items-center gap-1.5 border-b border-border bg-toolbar pr-1.5",
			ToolbarLead:    "flex h-full w-[240px] shrink-0 items-center justify-between border-r border-border px-1.5",
			ToolbarEnd:     "flex shrink-0 items-center gap-0.5",
			Nav:            "flex items-center gap-px overflow-hidden rounded-lg bg-foreground/7",
			PathBar:        "flex h-[34px] min-w-0 flex-1 items-center gap-0.5 overflow-hidden rounded-lg bg-foreground/7 p-[3px]",
			PathCur:        "bg-foreground/8 font-bold",
			PathSep:        "flex shrink-0 items-center",
			Search:         "relative w-[250px] shrink-0",
			SearchIcon:     "pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground",
			Sidebar:        "flex w-[240px] shrink-0 flex-col gap-px overflow-y-auto border-r border-border bg-sidebar p-1.5",
			SideSep:        "mx-1 my-1.5",
			SideGroup:      "gap-px",
			Views:          "gap-0.5",
			Content:        "flex min-w-0 flex-1 flex-col overflow-hidden bg-card",
			CellDimOn:      "group-aria-selected:text-muted-foreground",
			Details:        "hidden w-[300px] shrink-0 flex-col gap-5 overflow-y-auto border-l border-border bg-background p-5 lg:flex",
			DetailsPreview: "flex h-[150px] shrink-0 items-center justify-center rounded-[12px] bg-foreground/5",
			DetailsTitle:   "text-[17px] leading-6 font-bold break-words",
			DetailsKey:     "text-[13px] text-muted-foreground",
			DetailsVal:     "text-[14px]",
			Status:         "flex h-9 shrink-0 items-center justify-between gap-4 border-t border-border bg-background px-4 text-[13px] text-muted-foreground",
		},
		Columns: []Column{ColName, ColSize, ColType, ColModified},
		Labels:  map[Column]string{ColName: "Name", ColModified: "Modified", ColType: "Type", ColSize: "Size"},
		Widths:  map[Column]string{ColName: "minmax(0,1fr)", ColSize: "110px", ColType: "170px", ColModified: "150px"},
		Types: [numKinds]string{
			KindFolder: "Folder", KindText: "Text document", KindDoc: "Document", KindSheet: "Spreadsheet", KindPDF: "PDF document",
			KindImage: "%s image", KindAudio: "%s audio", KindVideo: "%s video", KindArchive: "%s archive", KindCode: "Source code",
		},
		ExtTypes: map[string]string{
			"txt": "Plain text document", "md": "Markdown document", "docx": "Word 2007 document", "xlsx": "Excel 2007 spreadsheet",
			"go": "Go source code", "mod": "Go module file", "gz": "Compressed archive",
		},
		Root:     "Home",
		Chevrons: false,
		SideGroups: []SideGroup{
			{Label: "Home", Items: []Place{{"Home", "", icons.House}}},
			{Label: "Places", Items: []Place{
				{"Desktop", "Desktop", icons.Monitor}, {"Documents", "Documents", icons.FileText}, {"Downloads", "Downloads", icons.Download},
				{"Music", "Music", icons.Music}, {"Pictures", "Pictures", icons.Image}, {"Videos", "Videos", icons.Video},
			}},
			{Label: "Devices", Items: []Place{{"Data", "Data", icons.HardDrive}, {"Trash", "Trash", icons.Trash2}}},
		},
		SearchHint:   func(string) string { return "Search" },
		IconPx:       22,
		HeaderBar:    true,
		ToolbarViews: true,
		Date:         gnomeDate,
		Size:         sizeSI,
		FolderSize:   func(n int) string { return plural(n, "item", "items") },
		Status: func(files, folders int, size int64, sel *Entry) string {
			if sel != nil {
				if sel.IsFolder() {
					return "“" + sel.Name + "” selected (" + plural(len(sel.Visible()), "item", "items") + ")"
				}
				return "“" + sel.Name + "” selected (" + sizeSI(sel.Size) + ")"
			}
			return plural(files+folders, "item", "items")
		},
	}
}

// adwaitaTheme is libadwaita's palette: a light gray window with a white
// content area (a near black one in dark), translucent black or white
// over it, and the blue accent.
func adwaitaTheme() tela.Theme {
	blue := tela.Hex("#3584e4")
	return makeTheme(10, "Adwaita Sans", map[string]tela.SchemeColor{
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

		"folder":       tok(tela.Hex("#3a7bd5"), tela.Hex("#639fe6")),
		"kind-text":    tok(tela.Hex("#77767b"), tela.Hex("#9a9996")),
		"kind-doc":     tok(tela.Hex("#3584e4"), tela.Hex("#62a0ea")),
		"kind-sheet":   tok(tela.Hex("#26a269"), tela.Hex("#57e389")),
		"kind-pdf":     tok(tela.Hex("#e01b24"), tela.Hex("#f66151")),
		"kind-image":   tok(tela.Hex("#9141ac"), tela.Hex("#c061cb")),
		"kind-audio":   tok(tela.Hex("#c061cb"), tela.Hex("#dc8add")),
		"kind-video":   tok(tela.Hex("#e66100"), tela.Hex("#ffa348")),
		"kind-archive": tok(tela.Hex("#986a44"), tela.Hex("#b5835a")),
		"kind-code":    tok(tela.Hex("#613583"), tela.Hex("#dc8add")),
	})
}

// adwaitaStyle is what libadwaita makes of the kit's recipes: 8 px buttons
// that darken on hover, a flat gray entry with a blue ring on focus, a list
// with rounded rows and bold column titles, menus that are rounded
// popovers and a dark tooltip.
func adwaitaStyle() kit.Style {
	const fs = "text-[15px]"
	s := nativeStyle(fs)

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

	t := &s.Table
	t.Root += " px-1.5 pb-1.5"
	t.Table += " gap-y-px"
	t.Head += " h-9 px-[14px] text-[13px] font-bold text-muted-foreground"
	t.SortButton = "-ml-2 h-7 px-2 text-[13px] font-bold text-muted-foreground hover:bg-transparent hover:text-foreground active:bg-transparent"
	t.Cell += " h-10 first:rounded-l-lg last:rounded-r-lg"
	t.Empty += " text-[17px] font-bold"

	g := &s.Toggle
	g.Variants[Sidebar] = "relative h-9 w-full justify-start gap-3 rounded-lg px-3 text-left text-foreground hover:bg-foreground/6 checked:bg-foreground/10 checked:hover:bg-foreground/10"
	g.Sizes[SideSize] = ""
	g.Variants[View] = "rounded-lg text-foreground hover:bg-foreground/7 checked:bg-foreground/14"
	g.Sizes[ViewSize] = "h-[34px] w-9"
	g.Variants[Tile] = "h-auto w-[116px] flex-col gap-1.5 whitespace-normal rounded-xl p-2.5 text-center text-foreground hover:bg-foreground/6 aria-pressed:bg-selection aria-pressed:hover:bg-selection"
	g.Sizes[TileSize] = ""

	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		swap("rounded-md border", "rounded-xl border", &m.Content)
		swap("rounded-sm px-2", "rounded-lg px-3", &m.Item, &m.CheckboxItem, &m.RadioItem)
		swap("p-1 ", "p-1.5 ", &m.Content)
	}
	swap("rounded-lg border", "rounded-xl border", &s.Dialog.Root, &s.AlertDialog.Root)

	// GNOME's tooltips are small, dark and rounded.
	s.Tooltip.Root = "z-50 w-fit rounded-lg bg-foreground px-3 py-1.5 text-[13px] text-background " +
		"transition duration-150 starting:scale-95 starting:opacity-0"
	return s
}
