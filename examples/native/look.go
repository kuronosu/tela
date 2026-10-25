package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// A Look is one platform's way of drawing the explorer. The component tree
// never asks which platform it is: everything that differs is here, in
// three layers (ADR-129, K17):
//
//   - Theme holds the tokens: colors with a light and a dark value (so no
//     component writes a dark: class), the base radius and the font.
//   - Style is the kit's recipes for this look: what a button, an input, a
//     table, a menu or a dialog is made of. kit.StyleProvider applies it to
//     the window, so switching look swaps these two and nothing else.
//   - Frame is the classes of the regions of the window that the explorer
//     itself lays out (the toolbar row, the sidebar, the details pane, the
//     status bar): there is no kit component for those, so the example
//     names them with tokens.
//
// The rest is data: which columns show and in what order, how dates and
// sizes read, what the status bar says, which sections the sidebar has.
type Look struct {
	ID    string
	Title string // window title
	Name  string // the platform, as in "Windows 11"

	Theme tela.Theme
	Style *kit.Style
	Frame Frame

	// Columns are the visible columns, in order; Labels their headers,
	// Widths their grid tracks and Aligned the ones that are right aligned.
	Columns []Column
	Labels  map[Column]string
	Widths  map[Column]string
	Right   map[Column]bool
	// Types are the type names by kind; %s is the upper case extension.
	// ExtTypes override them for an extension.
	Types    [numKinds]string
	ExtTypes map[string]string

	// Root is the name of the home folder in the path bar.
	Root string
	// Chevrons draws an arrow between two crumbs (a vertical line otherwise).
	Chevrons bool
	// SideGroups are the sidebar's sections.
	SideGroups []SideGroup
	// SearchHint is the placeholder of the search field, given the name of
	// the current folder.
	SearchHint func(folder string) string
	// IconPx is the size of the icons in the list at the default zoom.
	IconPx float32

	// The parts of the window a platform has or lacks.
	CommandBar    bool // Windows' command bar under the address row
	HeaderBar     bool // GNOME's header bar: sidebar and menu buttons, one row
	StatusViews   bool // the view switcher is in the status bar (Windows)
	ToolbarViews  bool // the view switcher is in the toolbar (GNOME, KDE)
	Zoom          bool // a zoom slider in the status bar (KDE)
	StatusAtEnd   bool // the status bar has a border and a surface of its own
	SideGroupHead bool // sections of the sidebar have a heading (KDE)

	// Date, Size and FolderSize format the cells of the list.
	Date       func(time.Time) string
	Size       func(int64) string
	FolderSize func(items int) string
	// Status words the status bar from the counts and the selection.
	Status func(files, folders int, size int64, sel *Entry) string
}

// Frame is the classes of the explorer's own regions. They name tokens
// (bg-background, border-border, text-muted-foreground), so one string
// serves light and dark.
type Frame struct {
	// Window is the root: text size and leading (the font is the theme's).
	Window string
	// ToolbarRow is the row of the navigation buttons, the path bar and the
	// search field; ToolbarLead the box over the sidebar that GNOME's
	// header bar has; ToolbarEnd the buttons after the search field.
	ToolbarRow, ToolbarLead, ToolbarEnd string
	// Nav wraps the back, forward and up buttons.
	Nav string
	// PathBar is the breadcrumb bar, PathCur is added to the last crumb and
	// PathSep is what is between two crumbs.
	PathBar, PathCur, PathSep string
	// Search wraps the search field and its magnifier; SearchIcon is the
	// magnifier.
	Search, SearchIcon string
	// CmdBar is the row under the address bar, with CmdSep between groups.
	CmdBar, CmdSep string
	// Sidebar is the places pane, SideHead a section heading, SideSep the
	// line between sections ("" for none), SideGroup is added to the list of
	// places of a section and SideMark is the bar of the selected place
	// ("" for none).
	Sidebar, SideHead, SideSep, SideGroup, SideMark string
	// Views is added to the view switcher.
	Views string
	// Content is the box around the file list and the icon view.
	Content string
	// CellDimOn is added to the secondary cells of a list row, to say what
	// color they take when the row is selected.
	CellDimOn string
	// IconOn is added to the icon of a selected row, TileIconOn to the icon
	// of a selected tile ("" keeps the icon's colors).
	IconOn, TileIconOn string
	// Details is the preview pane and its parts.
	Details, DetailsPreview, DetailsTitle, DetailsKey, DetailsVal string
	// Status is the bar at the bottom.
	Status string
}

// SideGroup is a section of the sidebar.
type SideGroup struct {
	Title string // the heading, "" for none
	Label string // the group's accessible name when it has no heading
	Items []Place
}

// Place is a sidebar entry.
type Place struct {
	Name string
	Path Path
	Icon tela.Icon
}

// Looks are the platform looks by id.
var Looks = map[string]*Look{
	"win11":   win11(),
	"adwaita": adwaita(),
	"breeze":  breeze(),
}

// LookIDs lists the looks in a fixed order.
var LookIDs = []string{"win11", "adwaita", "breeze"}

// Label returns the header of a column.
func (l *Look) Label(c Column) string { return l.Labels[c] }

// TypeName returns the type of an entry as the platform words it.
func (l *Look) TypeName(e *Entry) string {
	s := l.Types[e.Kind]
	if x, ok := l.ExtTypes[e.Ext()]; ok && !e.IsFolder() {
		return x
	}
	if strings.Contains(s, "%s") {
		ext := strings.ToUpper(e.Ext())
		if ext == "" {
			ext = "FILE"
		}
		return fmt.Sprintf(s, ext)
	}
	return s
}

// SizeCell is the text of the Size column for an entry.
func (l *Look) SizeCell(e *Entry) string {
	if e.IsFolder() {
		return l.FolderSize(len(e.Visible()))
	}
	return l.Size(e.Size)
}

// now is the clock of the fake desktop: dates are relative to it, so the
// screens never change.
var now = time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC)

// commas writes n with thousands separators.
func commas(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// sizeSI formats with powers of 1000 and one decimal (GNOME).
func sizeSI(n int64) string {
	units := []string{"bytes", "kB", "MB", "GB", "TB"}
	if n < 1000 {
		return fmt.Sprintf("%d bytes", n)
	}
	f, i := float64(n), 0
	for f >= 1000 && i < len(units)-1 {
		f /= 1000
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

// sizeIEC formats with powers of 1024 and one decimal (KDE).
func sizeIEC(n int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

// sizeKB formats in whole kilobytes, rounded up (Windows' details view).
func sizeKB(n int64) string {
	kb := (n + 1023) / 1024
	if kb == 0 {
		kb = 1
	}
	return commas(kb) + " KB"
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

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
