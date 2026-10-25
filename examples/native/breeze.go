package main

import (
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/native/icons"
	"github.com/kuronosu/tela/kit"
)

// breeze is Dolphin with the Breeze style (Plasma 6): a flat toolbar with
// highlight-framed hovers, a breadcrumb field, a Places panel with category
// headings, a solid 3DAEE9 selection with white text, 5 px radii, and a
// status bar with a zoom slider.
func breeze() *Look {
	style := breezeStyle()
	return &Look{
		ID:    "breeze",
		Title: "Dolphin",
		Name:  "KDE",
		Theme: breezeTheme(),
		Style: &style,
		Frame: Frame{
			Window:         "text-[13px] leading-[18px]",
			ToolbarRow:     "flex h-[46px] items-center gap-1.5 border-b border-border px-2",
			ToolbarEnd:     "flex shrink-0 items-center gap-0.5",
			Nav:            "flex items-center gap-0.5",
			PathBar:        "flex h-[30px] min-w-0 flex-1 items-center overflow-hidden rounded-md border border-input bg-field px-1",
			PathSep:        "flex w-3 shrink-0 items-center justify-center text-muted-foreground",
			Search:         "relative w-[240px] shrink-0",
			SearchIcon:     "pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground",
			Sidebar:        "flex w-[210px] shrink-0 flex-col gap-px overflow-y-auto p-1.5",
			SideHead:       "px-2 pt-3 pb-1 text-[12px] font-bold text-muted-foreground first:pt-1",
			SideGroup:      "gap-px",
			Views:          "gap-0.5",
			Content:        "flex min-w-0 flex-1 flex-col overflow-hidden border-l border-border bg-card",
			CellDimOn:      "group-aria-selected:text-selection-foreground/90",
			IconOn:         "group-aria-selected:text-selection-foreground",
			TileIconOn:     "text-selection-foreground",
			Details:        "hidden w-[260px] shrink-0 flex-col gap-3 overflow-y-auto border-l border-border bg-background p-3 lg:flex",
			DetailsPreview: "flex h-[140px] shrink-0 items-center justify-center",
			DetailsTitle:   "text-center text-[14px] leading-5 font-bold break-words",
			DetailsKey:     "text-[12px] text-muted-foreground",
			DetailsVal:     "text-[13px]",
			Status:         "flex h-8 shrink-0 items-center justify-between gap-4 border-t border-border bg-background px-3 text-[12px]",
		},
		Columns: []Column{ColName, ColSize, ColModified, ColType},
		Labels:  map[Column]string{ColName: "Name", ColModified: "Date Modified", ColType: "Type", ColSize: "Size"},
		Widths:  map[Column]string{ColName: "minmax(0,1fr)", ColSize: "90px", ColModified: "150px", ColType: "130px"},
		Right:   map[Column]bool{ColSize: true},
		Types: [numKinds]string{
			KindFolder: "Folder", KindText: "Text document", KindDoc: "Document", KindSheet: "Spreadsheet", KindPDF: "PDF document",
			KindImage: "%s image", KindAudio: "%s audio", KindVideo: "%s video", KindArchive: "%s archive", KindCode: "Source code",
		},
		ExtTypes: map[string]string{
			"txt": "Plain Text Document", "md": "Markdown document", "docx": "Word document", "xlsx": "Excel spreadsheet",
			"go": "Go source code", "mod": "Go module file", "gz": "Gzip archive",
		},
		Root:     "Home",
		Chevrons: true,
		SideGroups: []SideGroup{
			{Title: "Places", Items: []Place{
				{"Home", "", icons.House}, {"Desktop", "Desktop", icons.Monitor}, {"Documents", "Documents", icons.FileText},
				{"Downloads", "Downloads", icons.Download}, {"Music", "Music", icons.Music}, {"Pictures", "Pictures", icons.Image},
				{"Videos", "Videos", icons.Video}, {"Trash", "Trash", icons.Trash2},
			}},
			{Title: "Devices", Items: []Place{{"Data", "Data", icons.HardDrive}}},
		},
		SearchHint:   func(string) string { return "Search…" },
		IconPx:       22,
		ToolbarViews: true,
		Zoom:         true,
		Date:         func(t time.Time) string { return t.Format("2006-01-02 15:04") },
		Size:         sizeIEC,
		FolderSize:   func(n int) string { return plural(n, "item", "items") },
		Status: func(files, folders int, size int64, sel *Entry) string {
			if sel != nil {
				if sel.IsFolder() {
					return fmt.Sprintf("%s (Folder, %s)", sel.Name, plural(len(sel.Visible()), "item", "items"))
				}
				return fmt.Sprintf("%s (%s, %s)", sel.Name, sel.Ext(), sizeIEC(sel.Size))
			}
			return fmt.Sprintf("%s, %s (%s)", plural(folders, "Folder", "Folders"), plural(files, "File", "Files"), sizeIEC(size))
		},
	}
}

// breezeTheme is Breeze's palette: a blue-gray window, a near white (or
// near black) view, the 3DAEE9 highlight for the selection, hovers that are
// a tint of it and white text on it.
func breezeTheme() tela.Theme {
	blue := tela.Hex("#3daee9")
	return makeTheme(6.25, "Noto Sans", map[string]tela.SchemeColor{
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

		"folder":       tok(tela.Hex("#1d99f3"), tela.Hex("#3daee9")),
		"kind-text":    tok(tela.Hex("#7f8c8d"), tela.Hex("#7f8c8d")),
		"kind-doc":     tok(tela.Hex("#2980b9"), tela.Hex("#3daee9")),
		"kind-sheet":   tok(tela.Hex("#27ae60"), tela.Hex("#2ecc71")),
		"kind-pdf":     tok(tela.Hex("#da4453"), tela.Hex("#ed6a76")),
		"kind-image":   tok(tela.Hex("#1d99f3"), tela.Hex("#3daee9")),
		"kind-audio":   tok(tela.Hex("#9b59b6"), tela.Hex("#b07cc6")),
		"kind-video":   tok(tela.Hex("#f67400"), tela.Hex("#f79a3c")),
		"kind-archive": tok(tela.Hex("#8e6c4a"), tela.Hex("#b08b66")),
		"kind-code":    tok(tela.Hex("#16a085"), tela.Hex("#1abc9c")),
	})
}

// breezeStyle is what Breeze makes of the kit's recipes: toolbar buttons
// that frame themselves in the highlight color on hover, a field with a
// 1 px edge that becomes the highlight on focus, a solid highlight for the
// selection (in the list, the places and the menus) and a thin slider.
func breezeStyle() kit.Style {
	const fs = "text-[13px]"
	s := nativeStyle(fs)

	b := &s.Button
	b.Variants[kit.Ghost] = "border border-transparent text-foreground hover:border-primary hover:bg-accent active:bg-primary/35"
	b.Sizes[kit.Sm] = "h-6 gap-1.5 rounded-sm px-1.5"
	b.Variants[kit.Outline] = "border border-input bg-field text-foreground hover:border-primary hover:bg-accent"

	s.Input.Root = "h-[30px] w-full min-w-0 rounded-md border border-input bg-field pr-2 pl-7 " + fs + " outline-none " +
		"placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground " +
		"focus-visible:border-primary focus-visible:ring-1 focus-visible:ring-primary disabled:opacity-50"

	t := &s.Table
	t.Root += " p-1"
	t.Head += " h-7 border-b border-border px-2.5 text-[12px]"
	t.HeadSticky += " "
	t.SortButton = "-ml-2 h-5 px-2 text-[12px] hover:bg-accent"
	t.Cell += " h-7 first:rounded-l-md last:rounded-r-md"
	t.Empty += " text-[15px]"

	g := &s.Toggle
	g.Variants[Sidebar] = "relative h-7 w-full justify-start gap-2.5 rounded-md px-2 text-left text-foreground hover:bg-accent checked:bg-selection checked:text-selection-foreground checked:hover:bg-selection"
	g.Sizes[SideSize] = ""
	g.Variants[View] = "rounded-md border border-transparent text-foreground hover:border-primary hover:bg-accent checked:bg-primary/35"
	g.Sizes[ViewSize] = "size-8"
	g.Variants[Tile] = "h-auto w-[104px] flex-col gap-1.5 whitespace-normal rounded-md p-2 text-center text-foreground hover:bg-accent aria-pressed:bg-selection aria-pressed:text-selection-foreground aria-pressed:hover:bg-selection"
	g.Sizes[TileSize] = ""

	// Menus highlight with the solid selection, as every Breeze list does.
	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		swap("focus:bg-accent focus:text-accent-foreground", "focus:bg-selection focus:text-selection-foreground", &m.Item, &m.CheckboxItem, &m.RadioItem)
		m.ItemIconVariants[""] = "text-current"
		m.Shortcut = "ml-auto text-xs tracking-widest opacity-60"
	}

	// The zoom slider: a thin groove with the highlight to the left of a
	// round handle.
	z := &s.Slider
	z.Track = "relative grow overflow-hidden rounded-full bg-muted"
	z.TrackOrientations[kit.Horizontal] = "h-1 w-full"
	z.Range = "absolute bg-primary"
	z.Thumb = "block size-3.5 shrink-0 rounded-full border border-input bg-field transition-[color,box-shadow] hover:border-primary focus-visible:border-primary focus-visible:outline-hidden"
	return s
}
