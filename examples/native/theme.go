package main

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Mode says which color scheme a theme draws.
type Mode uint8

const (
	// Auto follows the system: the dark values are written as dark: classes,
	// which apply when the window is told the system prefers dark.
	Auto Mode = iota
	// Light always draws the light values.
	Light
	// Dark always draws the dark values.
	Dark
)

func (m Mode) String() string {
	switch m {
	case Light:
		return "light"
	case Dark:
		return "dark"
	}
	return "auto"
}

// Look is the set of class strings the explorer is styled with. Every field
// is the class list of one part of the tree, so the tree stays the same
// while a theme changes everything about how it looks. A Theme holds three
// of them: Base (geometry, type, shared by both schemes) and the colors of
// Light and Dark, which are appended to Base.
type Look struct {
	// Window is the root: background, text color, font.
	Window string

	// Toolbar is the box around the toolbar row and the command bar.
	Toolbar, ToolbarRow, ToolbarLead, ToolbarEnd string
	// NavGroup wraps the back, forward and up buttons; NavBtn is one.
	NavGroup, NavBtn string
	// PathBar is the breadcrumb bar; PathSeg one crumb, PathSegCur the last
	// one, PathSep what is between two.
	PathBar, PathSeg, PathSegCur, PathSep string
	// Search is the search field box; SearchIcon its magnifier and
	// SearchInput the text field inside.
	Search, SearchIcon, SearchInput string
	// CmdBar is Windows' command bar, hidden elsewhere.
	CmdBar, CmdBtn, CmdSep string

	// Sidebar is the places pane.
	Sidebar, SideHead, SideSep, SideItem, SideItemOn, SideMark string

	// Content is the box around the column headers and the list.
	Content, ColHeader, ColCell, ColSep string
	// List is the scrolling box of rows; Row one row, RowIdle the extra
	// classes of a row that is not selected (hover) and RowOn those of the
	// selected one. CellDim and CellDimOn color the secondary cells.
	List, Row, RowIdle, RowOn, CellDim, CellDimOn, Empty string
	// WName to WSize give the width of the four columns.
	WName, WModified, WType, WSize string

	// Details is the preview pane on the right.
	Details, DetailsPreview, DetailsTitle, DetailsKey, DetailsVal string

	// Status is the bar at the bottom; StatusToggle a view button in it and
	// StatusToggleOn the selected one, Zoom the slider.
	Status, StatusToggle, StatusToggleOn, Zoom, ZoomTrack, ZoomKnob string

	// Icon colors, as background classes: the two parts of a folder, a page
	// and its edge, and the accent of each kind of file.
	FolderBack, FolderFront, PageBody, PageEdge, PageFlap                                         string
	KindText, KindDoc, KindSheet, KindPDF, KindImage, KindAudio, KindVideo, KindArchive, KindCode string
}

// Theme is one platform's look. All the platform differences that are not
// class strings live here too: which columns show in which order, how dates
// and sizes read, what the buttons are called.
type Theme struct {
	ID    string
	Title string // window title

	Base, Light, Dark Look

	// Columns are the visible columns, in order.
	Columns []Column
	// Labels are the column headers by column.
	Labels map[Column]string
	// Types are the type names by kind; %s is the upper case extension.
	// ExtTypes override them for an extension.
	Types    [numKinds]string
	ExtTypes map[string]string

	// Root is the name of the home folder in the path bar.
	Root string
	// Chevrons draws an arrow between two crumbs (a line otherwise).
	Chevrons bool
	// SideGroups are the sidebar's sections.
	SideGroups []SideGroup
	// SearchHint is the placeholder of the search field, given the name of
	// the current folder.
	SearchHint func(folder string) string
	// IconPx is the size of the icons in the list; SideIconPx in the sidebar
	// and the toolbar.
	IconPx, SideIconPx float32

	// Date, Size and FolderSize format the cells of the list.
	Date       func(time.Time) string
	Size       func(int64) string
	FolderSize func(items int) string
	// Status words the status bar from the counts and the selection.
	Status func(files, folders int, size int64, sel *Entry) string
}

// SideGroup is a section of the sidebar.
type SideGroup struct {
	Title string
	Items []Place
}

// Place is a sidebar entry.
type Place struct {
	Name string
	Path Path
	Icon string // glyph name, see icons.go
}

// Themes are the platform looks by id.
var Themes = map[string]*Theme{
	"win11":   win11(),
	"adwaita": adwaita(),
	"breeze":  breeze(),
}

// ThemeIDs lists the themes in a fixed order.
var ThemeIDs = []string{"win11", "adwaita", "breeze"}

var (
	lookMu    sync.Mutex
	lookCache = map[lookKey]*Look{}
)

type lookKey struct {
	theme string
	mode  Mode
}

// Resolve returns the look of a theme in a mode. Light and Dark pick one
// color set; Auto writes both, the dark one as dark: classes. The result is
// shared: treat it as read-only.
func (t *Theme) Resolve(m Mode) *Look {
	k := lookKey{t.ID, m}
	lookMu.Lock()
	defer lookMu.Unlock()
	if l, ok := lookCache[k]; ok {
		return l
	}
	out := &Look{}
	ov, bv, lv, dv := reflect.ValueOf(out).Elem(), reflect.ValueOf(t.Base), reflect.ValueOf(t.Light), reflect.ValueOf(t.Dark)
	for i := 0; i < ov.NumField(); i++ {
		parts := []string{bv.Field(i).String()}
		switch m {
		case Light:
			parts = append(parts, lv.Field(i).String())
		case Dark:
			parts = append(parts, dv.Field(i).String())
		default:
			parts = append(parts, lv.Field(i).String(), darkOnly(dv.Field(i).String()))
		}
		ov.Field(i).SetString(strings.Join(strings.Fields(strings.Join(parts, " ")), " "))
	}
	lookCache[k] = out
	return out
}

// darkOnly turns a class list into its dark: twin.
func darkOnly(classes string) string {
	f := strings.Fields(classes)
	for i, c := range f {
		f[i] = "dark:" + c
	}
	return strings.Join(f, " ")
}

// Label returns the header of a column.
func (t *Theme) Label(c Column) string { return t.Labels[c] }

// TypeName returns the type of an entry as the platform words it.
func (t *Theme) TypeName(e *Entry) string {
	s := t.Types[e.Kind]
	if x, ok := t.ExtTypes[e.Ext()]; ok && !e.IsFolder() {
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

// Name is the platform's name, as in "Windows 11".
func (t *Theme) Name() string { return themeNames[t.ID] }

var themeNames = map[string]string{"win11": "Windows 11", "adwaita": "GNOME", "breeze": "KDE"}
