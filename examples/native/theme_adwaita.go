package main

import "time"

// gnomeDate words a date the way Files does: the time today, "Yesterday", the
// weekday within a week, the day this year, the full date before.
func gnomeDate(t time.Time) string {
	switch {
	case sameDay(t, now):
		return t.Format("15:04")
	case sameDay(t, now.AddDate(0, 0, -1)):
		return "Yesterday"
	case t.Before(now) && now.Sub(t) < 7*24*time.Hour:
		return t.Format("Mon")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}

// adwaita is Files (Nautilus) in libadwaita: a header bar that holds the
// navigation, the path bar and the search entry, a sidebar of the same gray,
// rounded rows with a tinted selection, 6 and 8 px radii and the blue
// 3584E4 accent. The header bar is drawn inside the client area.
func adwaita() *Theme {
	return &Theme{
		ID:    "adwaita",
		Title: "Files",
		Base: Look{
			Window:         "font-['Adwaita_Sans',Cantarell,Inter,sans-serif] text-[15px] leading-5",
			Toolbar:        "flex shrink-0 flex-col",
			ToolbarRow:     "flex h-[47px] items-center gap-1.5 border-b pr-1.5",
			ToolbarLead:    "flex h-full w-[240px] shrink-0 items-center justify-between border-r px-1.5",
			ToolbarEnd:     "flex shrink-0 items-center gap-0.5",
			NavGroup:       "flex items-center gap-px overflow-hidden rounded-[8px]",
			NavBtn:         "flex h-[34px] w-[36px] shrink-0 items-center justify-center rounded-[8px] transition-colors",
			PathBar:        "flex h-[34px] min-w-0 flex-1 items-center gap-0.5 overflow-hidden rounded-[8px] p-[3px]",
			PathSeg:        "flex h-7 shrink-0 items-center gap-2 rounded-[6px] px-3 whitespace-nowrap transition-colors",
			PathSegCur:     "font-bold",
			PathSep:        "h-4 w-px shrink-0",
			Search:         "flex h-[34px] w-[250px] shrink-0 items-center gap-2 rounded-[8px] px-3",
			SearchIcon:     "flex shrink-0",
			SearchInput:    "min-w-0 flex-1 outline-none",
			CmdBar:         "hidden",
			Sidebar:        "flex w-[240px] shrink-0 flex-col gap-px overflow-y-auto border-r p-1.5",
			SideHead:       "hidden",
			SideSep:        "mx-1 my-1.5 h-px shrink-0",
			SideItem:       "relative flex h-9 shrink-0 items-center gap-3 rounded-[8px] px-3 text-left whitespace-nowrap transition-colors",
			SideMark:       "hidden",
			Content:        "flex min-w-0 flex-1 flex-col overflow-hidden",
			ColHeader:      "flex h-9 shrink-0 items-stretch px-[14px]",
			ColCell:        "relative flex items-center gap-1.5 text-left text-[13px] font-bold",
			ColSep:         "hidden",
			List:           "flex min-h-0 flex-1 flex-col gap-px overflow-y-auto p-1.5 pt-0 outline-none",
			Row:            "flex h-10 shrink-0 items-center rounded-[8px] px-2 transition-colors",
			Empty:          "flex flex-1 items-center justify-center pb-16 text-[17px] font-bold",
			WName:          "min-w-0 flex-1 px-2",
			WModified:      "w-[150px] shrink-0 px-2",
			WType:          "w-[170px] shrink-0 px-2",
			WSize:          "w-[110px] shrink-0 px-2",
			Details:        "hidden w-[300px] shrink-0 flex-col gap-5 overflow-y-auto border-l p-5 lg:flex",
			DetailsPreview: "flex h-[150px] shrink-0 items-center justify-center rounded-[12px]",
			DetailsTitle:   "text-[17px] leading-6 font-bold break-words",
			DetailsKey:     "text-[13px]",
			DetailsVal:     "text-[14px]",
			Status:         "flex h-9 shrink-0 items-center justify-between gap-4 border-t px-4 text-[13px]",
			StatusToggle:   "hidden",
			Zoom:           "hidden",
		},
		Light: Look{
			Window:         "bg-[#fafafa] text-[#323232]",
			ToolbarRow:     "border-black/12 bg-[#ebebeb]",
			ToolbarLead:    "border-black/12",
			NavGroup:       "bg-black/7",
			NavBtn:         "text-[#323232] enabled:hover:bg-black/7 enabled:active:bg-black/14 disabled:opacity-40",
			PathBar:        "bg-black/7",
			PathSeg:        "hover:bg-black/8",
			PathSegCur:     "bg-black/8",
			PathSep:        "bg-black/12",
			Search:         "bg-black/7 focus-within:ring-2 focus-within:ring-[#3584e4]",
			SearchIcon:     "text-black/60",
			SearchInput:    "placeholder:text-black/50 selection:bg-[#3584e4]",
			Sidebar:        "border-black/12 bg-[#ebebeb] text-[#323232]",
			SideSep:        "bg-black/12",
			SideItem:       "hover:bg-black/6",
			SideItemOn:     "bg-black/10 hover:bg-black/10",
			Content:        "bg-white",
			ColHeader:      "text-black/60",
			ColCell:        "hover:text-black/80",
			Row:            "text-[#323232]",
			RowIdle:        "hover:bg-black/5",
			RowOn:          "bg-[#3584e4]/16",
			CellDim:        "text-black/60",
			CellDimOn:      "text-black/60",
			Empty:          "text-black/45",
			Details:        "border-black/12 bg-[#fafafa]",
			DetailsPreview: "bg-black/5",
			DetailsKey:     "text-black/55",
			DetailsVal:     "text-[#323232]",
			Status:         "border-black/12 bg-[#fafafa] text-black/60",
			FolderBack:     "bg-[#3a7bd5]",
			FolderFront:    "bg-linear-to-b from-[#aacdf7] to-[#7db0f0]",
			PageBody:       "bg-white",
			PageEdge:       "border-[#b5b5b8]",
			PageFlap:       "bg-[#b5b5b8]",
			KindText:       "bg-[#77767b]",
			KindDoc:        "bg-[#3584e4]",
			KindSheet:      "bg-[#26a269]",
			KindPDF:        "bg-[#e01b24]",
			KindImage:      "bg-[#9141ac]",
			KindAudio:      "bg-[#c061cb]",
			KindVideo:      "bg-[#e66100]",
			KindArchive:    "bg-[#986a44]",
			KindCode:       "bg-[#613583]",
		},
		Dark: Look{
			Window:         "bg-[#242424] text-white",
			ToolbarRow:     "border-white/10 bg-[#303030]",
			ToolbarLead:    "border-white/10",
			NavGroup:       "bg-white/10",
			NavBtn:         "text-white enabled:hover:bg-white/10 enabled:active:bg-white/18 disabled:opacity-40",
			PathBar:        "bg-white/10",
			PathSeg:        "hover:bg-white/10",
			PathSegCur:     "bg-white/10",
			PathSep:        "bg-white/15",
			Search:         "bg-white/10 focus-within:ring-2 focus-within:ring-[#3584e4]",
			SearchIcon:     "text-white/70",
			SearchInput:    "placeholder:text-white/50 selection:bg-[#3584e4]",
			Sidebar:        "border-white/10 bg-[#2b2b2b] text-white",
			SideSep:        "bg-white/12",
			SideItem:       "hover:bg-white/8",
			SideItemOn:     "bg-white/14 hover:bg-white/14",
			Content:        "bg-[#1e1e1e]",
			ColHeader:      "text-white/60",
			ColCell:        "hover:text-white",
			Row:            "text-white",
			RowIdle:        "hover:bg-white/6",
			RowOn:          "bg-[#3584e4]/30",
			CellDim:        "text-white/60",
			CellDimOn:      "text-white/70",
			Empty:          "text-white/45",
			Details:        "border-white/10 bg-[#242424]",
			DetailsPreview: "bg-white/5",
			DetailsKey:     "text-white/55",
			DetailsVal:     "text-white",
			Status:         "border-white/10 bg-[#242424] text-white/60",
			FolderBack:     "bg-[#3a73c2]",
			FolderFront:    "bg-linear-to-b from-[#8db8ee] to-[#639fe6]",
			PageBody:       "bg-[#f4f4f4]",
			PageEdge:       "border-[#8d8d91]",
			PageFlap:       "bg-[#8d8d91]",
			KindText:       "bg-[#77767b]",
			KindDoc:        "bg-[#3584e4]",
			KindSheet:      "bg-[#26a269]",
			KindPDF:        "bg-[#e01b24]",
			KindImage:      "bg-[#9141ac]",
			KindAudio:      "bg-[#c061cb]",
			KindVideo:      "bg-[#e66100]",
			KindArchive:    "bg-[#986a44]",
			KindCode:       "bg-[#613583]",
		},
		Columns: []Column{ColName, ColSize, ColType, ColModified},
		Labels:  map[Column]string{ColName: "Name", ColModified: "Modified", ColType: "Type", ColSize: "Size"},
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
			{Items: []Place{{"Home", "", "home"}}},
			{Items: []Place{
				{"Desktop", "Desktop", "desktop"}, {"Documents", "Documents", "documents"}, {"Downloads", "Downloads", "downloads"},
				{"Music", "Music", "music"}, {"Pictures", "Pictures", "pictures"}, {"Videos", "Videos", "videos"},
			}},
			{Items: []Place{{"Data", "Data", "drive"}, {"Trash", "Trash", "trash"}}},
		},
		SearchHint: func(string) string { return "Search" },
		IconPx:     22, SideIconPx: 16,
		Date:       gnomeDate,
		Size:       sizeSI,
		FolderSize: func(n int) string { return plural(n, "item", "items") },
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
