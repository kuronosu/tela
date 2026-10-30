// Package looks holds the platform looks of Tela: Windows 11 (Fluent), GNOME
// (libadwaita) and KDE (Breeze) as values an app can adopt, each in light and
// dark. It is a separate module, github.com/kuronosu/tela/looks (ADR-197), so
// that package kit stays platform neutral and the looks change at their own
// pace.
//
// A look is the three levels of kit's customization (ADR-129):
//
//   - Theme holds the tokens: colors with a light and a dark value (so no
//     component writes a dark: class), the base radius and the font.
//   - Style is kit's recipes for the look: what a button, an input, a table, a
//     menu or a dialog is made of. [kit.StyleProvider] applies it to a subtree.
//   - Regions are the classes of the parts of a window that kit has no
//     component for: the toolbar, the sidebar, the content, the details pane
//     and the status bar. They name tokens, so one string serves light and
//     dark.
//
// Nothing in a component tree needs to ask which platform it draws. Adopting
// a look is giving the window its theme and the tree its style:
//
//	l := win11.Look() // or adwaita.Look(), breeze.Look(), detect.Look()
//	tela.Main(tela.App{
//		Title: "Files",
//		Theme: &l.Theme,
//		Root:  kit.StyleProvider{Style: l.Style, Children: Window{Regions: l.Regions}},
//	})
//
// and switching look while the app runs is [tela.SetTheme] with the new
// Theme and a new Style for the provider.
//
// # Packages
//
// Each look is a package that exports Look: [github.com/kuronosu/tela/looks/win11],
// [github.com/kuronosu/tela/looks/adwaita] and
// [github.com/kuronosu/tela/looks/breeze]. Package looks defines their types,
// so that they can all import it; for the same reason it cannot import them,
// and the choice of the look of the running desktop lives in
// [github.com/kuronosu/tela/looks/detect].
//
// The gallery, github.com/kuronosu/tela/looks/gallery, shows kit's controls
// in each look and switches between them while it runs.
//
// # Tokens
//
// Every look defines kit's theme tokens (background, foreground, card,
// popover, primary, secondary, muted, accent, destructive, border, input and
// ring, with their foregrounds) and the following, which its Regions and
// recipes use. A look that has no use for one gives it the value of the
// closest surface, so the classes of any look resolve under any look's theme
// (a preview rendered by the tools has one theme for all of them):
//
//   - toolbar and sidebar: the surfaces of the toolbar and of the sidebar.
//   - field: the background of a text field set in the toolbar.
//   - edge: the stronger line under a field (Windows 11's text box).
//   - selection and selection-foreground: a selected row or place.
//
// # Extra variants
//
// Besides kit's, the recipes of a look define two [kit.Toggle] variants and
// their sizes: [Sidebar] (an entry of a sidebar, full width, in a vertical
// [kit.ToggleGroup]) and [Toolbar] (an icon toggle of the toolbar).
package looks

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// Look is a platform's way of drawing an app. See the package documentation.
type Look struct {
	// Name is the platform, as in "Windows 11".
	Name string
	// Theme is the look's tokens: give it to tela.App.Theme, and to
	// tela.SetTheme to switch look at run time.
	Theme tela.Theme
	// Style is the look's kit recipes: give it to kit.StyleProvider. It
	// belongs to the caller of Look(), which returns a new one each time.
	Style *kit.Style
	// Regions are the classes of the window regions kit has no component for.
	Regions Regions
}

// Regions is the classes of the parts of a window that an app lays out
// itself because kit has no component for them. They name tokens
// (bg-background, border-border, text-muted-foreground, bg-selection), so
// one string serves light and dark. A field that is empty means the look has
// no such part, or adds nothing to it: the app draws it plain, or leaves it
// out where the field says so.
type Regions struct {
	// Window is the root of the window: the text size and leading. The font
	// is the theme's.
	Window string

	// Toolbar is the row on top of the window that holds the navigation,
	// the location and the search field (GNOME's header bar, Windows'
	// address row, Dolphin's toolbar).
	Toolbar string
	// ToolbarLead is a box at the start of the toolbar, as wide as the
	// sidebar, that holds the sidebar toggle and the main menu (GNOME's
	// header bar). Empty: the look has none, and the toolbar starts with
	// ToolbarGroup.
	ToolbarLead string
	// ToolbarGroup wraps a few related buttons (back, forward and up).
	ToolbarGroup string
	// ToolbarEnd wraps the buttons after the search field.
	ToolbarEnd string
	// Location is the bar that shows where the user is, with a breadcrumb
	// of small buttons; LocationCurrent is added to the last one, and
	// LocationSeparator wraps what is between two crumbs.
	Location, LocationCurrent, LocationSeparator string
	// Search wraps a search field (a [kit.Input] with room for an icon at
	// its start) and its magnifier; SearchIcon is the class of the
	// magnifier.
	Search, SearchIcon string

	// CommandBar is a second row of commands under the toolbar (Windows
	// 11's command bar), and CommandSeparator the class of the line between
	// two groups of commands. Empty: the look has none.
	CommandBar, CommandSeparator string

	// Sidebar is the pane of places. SidebarHeading is the heading of a
	// section (empty: sections have none), SidebarSeparator the line
	// between two sections (empty: none), SidebarGroup is added to the list
	// of places of a section and SidebarMark is the bar that marks the
	// selected place (empty: none).
	Sidebar, SidebarHeading, SidebarSeparator, SidebarGroup, SidebarMark string

	// Content is the box around the main area of the window.
	Content string

	// Details is the side pane that describes the selection. It is hidden
	// below the lg breakpoint. DetailsTitle is its heading, DetailsLabel the
	// name of a property and DetailsValue its value.
	Details, DetailsTitle, DetailsLabel, DetailsValue string

	// StatusBar is the bar at the bottom of the window.
	StatusBar string
}

// The Variants and Sizes that every look adds to [kit.Toggle].
const (
	// Sidebar is an entry of the sidebar: a Toggle of a vertical ToggleGroup
	// whose selected entry is marked as the look does.
	Sidebar kit.Variant = "sidebar"
	// Toolbar is an icon toggle of the toolbar (a view switcher).
	Toolbar kit.Variant = "toolbar"

	// SidebarSize and ToolbarSize are the sizes that go with them.
	SidebarSize kit.Size = "sidebar"
	ToolbarSize kit.Size = "toolbar"
)
