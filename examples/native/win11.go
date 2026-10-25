package main

import (
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/native/icons"
	"github.com/kuronosu/tela/kit"
)

// win11 is File Explorer in Windows 11 (Fluent): Mica gray around a white
// content pane with a rounded corner, a command bar, an address bar and a
// search box with Fluent's thin bottom edge, 4 px radii and the blue
// 0067C0 accent (60CDFF in dark).
func win11() *Look {
	style := win11Style()
	return &Look{
		ID:    "win11",
		Title: "File Explorer",
		Name:  "Windows 11",
		Theme: win11Theme(),
		Style: &style,
		Frame: Frame{
			Window:         "text-[12px] leading-4",
			ToolbarRow:     "flex items-center gap-2 px-2 py-1",
			Nav:            "flex items-center",
			PathBar:        "flex h-8 min-w-0 flex-1 items-center overflow-hidden rounded-md border border-input border-b-edge bg-field px-1",
			PathSep:        "flex w-3 shrink-0 items-center justify-center text-muted-foreground",
			Search:         "relative w-[300px] shrink-0",
			SearchIcon:     "pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground",
			CmdBar:         "flex h-10 shrink-0 items-center gap-0.5 px-2",
			CmdSep:         "mx-1.5 h-5",
			Sidebar:        "flex w-[240px] shrink-0 flex-col gap-0.5 overflow-y-auto px-2 py-1",
			SideSep:        "mx-1 my-1.5",
			SideMark:       "absolute top-2 left-0 h-4 w-[3px] rounded-full bg-primary",
			CellDimOn:      "group-aria-selected:text-muted-foreground",
			SideGroup:      "gap-0.5",
			Views:          "gap-1",
			Content:        "flex min-w-0 flex-1 flex-col overflow-hidden rounded-tl-[8px] border-t border-l border-border bg-card",
			Details:        "hidden w-[300px] shrink-0 flex-col gap-5 overflow-y-auto border-l border-border bg-card p-5 lg:flex",
			DetailsPreview: "flex h-[150px] shrink-0 items-center justify-center rounded-[8px] bg-background",
			DetailsTitle:   "text-[16px] leading-5 font-semibold break-words",
			DetailsKey:     "text-muted-foreground",
			Status:         "flex h-7 shrink-0 items-center justify-between gap-4 px-3",
		},
		Columns: []Column{ColName, ColModified, ColType, ColSize},
		Labels:  map[Column]string{ColName: "Name", ColModified: "Date modified", ColType: "Type", ColSize: "Size"},
		Widths:  map[Column]string{ColName: "minmax(0,1fr)", ColModified: "170px", ColType: "170px", ColSize: "90px"},
		Right:   map[Column]bool{ColSize: true},
		Types: [numKinds]string{
			KindFolder: "File folder", KindText: "%s File", KindDoc: "%s File", KindSheet: "%s File", KindPDF: "%s File",
			KindImage: "%s File", KindAudio: "%s File", KindVideo: "%s File", KindArchive: "%s File", KindCode: "%s File",
		},
		ExtTypes: map[string]string{
			"txt": "Text Document", "docx": "Microsoft Word Document", "xlsx": "Microsoft Excel Worksheet",
			"pdf": "Microsoft Edge PDF Document", "zip": "Compressed (zipped) Folder", "go": "Go Source File",
		},
		Root:     "Ana",
		Chevrons: true,
		SideGroups: []SideGroup{
			{Label: "Quick access", Items: []Place{
				{"Home", "", icons.House}, {"Desktop", "Desktop", icons.Monitor}, {"Downloads", "Downloads", icons.Download},
				{"Documents", "Documents", icons.FileText}, {"Pictures", "Pictures", icons.Image},
				{"Music", "Music", icons.Music}, {"Videos", "Videos", icons.Video},
			}},
			{Label: "Devices", Items: []Place{{"Data (D:)", "Data", icons.HardDrive}, {"Recycle Bin", "Trash", icons.Trash2}}},
		},
		SearchHint:  func(folder string) string { return "Search " + folder },
		IconPx:      20,
		CommandBar:  true,
		StatusViews: true,
		Date:        func(t time.Time) string { return t.Format("1/2/2006 3:04 PM") },
		Size:        sizeKB,
		FolderSize:  func(int) string { return "" },
		Status: func(files, folders int, size int64, sel *Entry) string {
			s := plural(files+folders, "item", "items")
			if sel != nil {
				s += "   |   1 item selected"
				if !sel.IsFolder() {
					s += "   " + sizeKB(sel.Size)
				}
			}
			return s
		},
	}
}

// win11Theme is Fluent's palette: Mica for the window, white (a deep gray
// in dark) for the content pane, translucent black or white for hovers and
// hairlines, and one blue accent.
func win11Theme() tela.Theme {
	return makeTheme(5, "Segoe UI Variable Text", map[string]tela.SchemeColor{
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

		"folder":       tok(tela.Hex("#e6a500"), tela.Hex("#eaaf1e")),
		"kind-text":    tok(tela.Hex("#8a8a8a"), tela.Hex("#9a9a9a")),
		"kind-doc":     tok(tela.Hex("#2b579a"), tela.Hex("#5b8bd6")),
		"kind-sheet":   tok(tela.Hex("#217346"), tela.Hex("#3fae72")),
		"kind-pdf":     tok(tela.Hex("#e5292b"), tela.Hex("#f0585a")),
		"kind-image":   tok(tela.Hex("#0f7dc2"), tela.Hex("#3aa1e3")),
		"kind-audio":   tok(tela.Hex("#8e44ad"), tela.Hex("#b36ad0")),
		"kind-video":   tok(tela.Hex("#e0522b"), tela.Hex("#f0764f")),
		"kind-archive": tok(tela.Hex("#c79a2a"), tela.Hex("#d9ad44")),
		"kind-code":    tok(tela.Hex("#4a4a4a"), tela.Hex("#a0a0a0")),
	})
}

// win11Style is what Fluent makes of the kit's recipes: 4 px radii, ghost
// buttons that gray on hover, a field with a thin edge under it that turns
// into a blue line on focus, a list whose rows are rounded and hover gray,
// and a sidebar with a blue mark on the selected place.
func win11Style() kit.Style {
	const fs = "text-[12px]"
	s := nativeStyle(fs)

	// Search and rename fields: the Fluent text box.
	s.Input.Root = "h-8 w-full min-w-0 rounded-md border border-input border-b-edge bg-field pr-2 pl-8 " + fs + " outline-none " +
		"placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground " +
		"focus-visible:border-b-2 focus-visible:border-b-primary disabled:opacity-50"

	// The list: header cells with the sort button, rows 30 px high.
	t := &s.Table
	t.Root += " px-2 pb-2"
	t.Head += " h-8 font-normal text-foreground"
	t.SortButton = "-ml-2 h-6 px-2 font-normal"
	t.Cell += " h-[30px] first:rounded-l-md last:rounded-r-md"
	t.Empty += " text-[13px]"

	// The sidebar: full width entries with a gray selection.
	g := &s.Toggle
	g.Variants[Sidebar] = "relative h-8 w-full justify-start gap-3 rounded-md px-3 text-left text-foreground hover:bg-accent active:bg-foreground/4 checked:bg-foreground/6 checked:hover:bg-foreground/6"
	g.Sizes[SideSize] = ""
	// The view switcher of the status bar.
	g.Variants[View] = "rounded-[3px] text-foreground hover:bg-foreground/6 checked:bg-foreground/10"
	g.Sizes[ViewSize] = "size-6"
	// The tiles of the icon view.
	g.Variants[Tile] = "h-auto w-[104px] flex-col gap-1.5 whitespace-normal rounded-md p-2 text-center text-foreground hover:bg-accent aria-pressed:bg-selection aria-pressed:hover:bg-selection"
	g.Sizes[TileSize] = ""

	// Menus: Fluent flyouts have round corners and a subtle item hover.
	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		swap("rounded-md border", "rounded-lg border", &m.Content)
		swap("rounded-sm px-2", "rounded-[4px] px-2.5", &m.Item, &m.CheckboxItem, &m.RadioItem)
	}
	return s
}
