package main

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// The recipes of the three looks share a starting point, nativeStyle: the
// kit's defaults with the edits every look needs because an explorer is not
// a shadcn page. Each look then edits what is its own (win11Style,
// adwaitaStyle and breezeStyle). Colors never appear in a recipe unless a
// token cannot say them: the classes name tokens (bg-accent,
// text-muted-foreground, bg-selection), and each look's tokens give them
// their values in light and in dark.

// The Variants and Sizes that the explorer adds to the kit's components.
// A recipe defines each of them for its look.
const (
	// Sidebar is an entry of the places pane (a Toggle of a ToggleGroup),
	// View an icon of the view switcher and Tile an entry of the icon view.
	Sidebar kit.Variant = "sidebar"
	View    kit.Variant = "view"
	Tile    kit.Variant = "tile"

	SideSize kit.Size = "sidebar"
	ViewSize kit.Size = "view"
	TileSize kit.Size = "tile"
)

// tok is a token with a light and a dark value.
func tok(light, dark tela.Color) tela.SchemeColor {
	return tela.SchemeColor{Light: light, Dark: dark}
}

func black(a float32) tela.Color { return tela.RGBA(0, 0, 0, a) }
func white(a float32) tela.Color { return tela.RGBA(255, 255, 255, a) }

// makeTheme is the kit's theme with the tokens of a look, its base radius
// and its font. The tokens that are not shadcn's (toolbar surfaces, the
// selection, the colors of the kinds of file) are named after what they
// paint.
func makeTheme(radius float32, font string, colors map[string]tela.SchemeColor) tela.Theme {
	t := kit.Theme()
	t.Radius = radius
	t.FontSans = font
	for name, c := range colors {
		t.Colors[name] = c
	}
	// Every look defines the same tokens, so that the classes of any recipe
	// resolve under any theme (a preview rendered by the tools has one theme
	// for all looks): the ones a look does not need follow its surfaces.
	for name, from := range map[string]string{"toolbar": "background", "sidebar": "background", "field": "card", "edge": "border"} {
		if _, ok := colors[name]; !ok {
			t.Colors[name] = t.Colors[from]
		}
	}
	return t
}

// swap replaces old by new in every class string given.
func swap(old, new string, classes ...*string) {
	for _, c := range classes {
		*c = strings.ReplaceAll(*c, old, new)
	}
}

// nativeStyle is the starting point of the three recipes. fs is the look's
// font size class: portals (menus, dialogs, tooltips) do not inherit it
// from the window, so the recipes of what they hold say it themselves.
func nativeStyle(fs string) kit.Style {
	s := kit.DefaultStyle()

	// Buttons: flat, token colored; the sizes are the ones an explorer has
	// (a toolbar icon, a crumb, a command, a dialog button).
	b := &s.Button
	b.Root = "inline-flex shrink-0 items-center justify-center gap-2 rounded-md " + fs + " whitespace-nowrap transition-colors outline-none " +
		"focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40"
	b.Variants[kit.Default] = "bg-primary text-primary-foreground hover:bg-primary/90"
	b.Variants[kit.Destructive] = "bg-destructive text-white hover:bg-destructive/90"
	b.Variants[kit.Outline] = "border border-border bg-card text-foreground hover:bg-accent"
	b.Variants[kit.Secondary] = "bg-secondary text-secondary-foreground hover:bg-secondary/80"
	b.Variants[kit.Ghost] = "text-foreground hover:bg-accent active:bg-foreground/10"
	b.Variants[kit.Link] = "text-primary hover:underline"
	b.Sizes[""] = "h-8 px-3"
	b.Sizes[kit.Sm] = "h-6 gap-1.5 px-2"
	b.Sizes[kit.Icon] = "size-8"
	b.Sizes[kit.IconSm] = "size-6"
	b.HasIcon[""] = "px-2.5"
	b.HasIcon[kit.Sm] = "px-2"

	// Tables: the file list. Rows have no box of their own, so the hover
	// and the selection are painted by their cells.
	t := &s.Table
	t.Root = "relative min-h-0 flex-1 overflow-auto outline-none"
	t.Table = "grid w-full " + fs
	t.Head = "flex items-center px-2 text-left whitespace-nowrap"
	t.HeadSticky = "sticky top-0 z-20 bg-card"
	t.Cell = "flex items-center overflow-hidden whitespace-nowrap transition-colors " +
		"group-hover:bg-accent group-aria-selected:bg-selection group-aria-selected:text-selection-foreground"
	t.CellWithLastRow = ""
	t.Empty = "col-span-full h-64 justify-center text-muted-foreground"

	// Toggle groups are the sidebar and the view switcher: a toggle is
	// what each look says it is by its variant, so the group adds nothing.
	// The selected look is in each variant (checked:), not in On.
	s.Toggle.On = ""
	s.ToggleGroup.Item = "shrink-0 focus:z-10"
	s.ToggleGroup.ItemWithoutSpacing = ""
	s.ToggleGroup.OutlineItemWithoutSpacing = ""
	s.Toggle.Root = "inline-flex items-center justify-center gap-2 whitespace-nowrap " + fs + " transition-colors outline-none " +
		"focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40"
	s.Toggle.Icon = "pointer-events-none size-4 shrink-0"

	// Menus (the "More" menu, the context menu): sized for the look.
	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		swap("text-sm", fs, &m.Item, &m.CheckboxItem, &m.RadioItem, &m.Label)
		m.Content += " " + fs
		m.Content = strings.ReplaceAll(m.Content, "min-w-32", "min-w-48")
		m.Separator = "-mx-1 my-1 h-px bg-border"
	}

	// Dialogs sit on the popover surface.
	for _, d := range []*string{&s.Dialog.Root, &s.AlertDialog.Root} {
		swap("bg-background", "bg-popover", d)
		*d += " text-popover-foreground " + fs
	}
	s.Dialog.Root = strings.ReplaceAll(s.Dialog.Root, "max-w-lg", "max-w-[420px]")
	s.AlertDialog.Sizes[""] = "max-w-[420px]"
	swap("text-lg", "text-[16px]", &s.Dialog.Title, &s.AlertDialog.Title)
	swap("text-sm", fs, &s.Dialog.Description, &s.AlertDialog.Description)
	s.AlertDialog.HeaderSizes[""] = "sm:place-items-start sm:text-left"
	s.Dialog.Overlay = "absolute inset-0 bg-black/30 transition-opacity duration-150 starting:opacity-0"
	s.AlertDialog.Overlay = s.Dialog.Overlay

	// Tooltips name the icon buttons: small, on the popover surface, with
	// no arrow.
	s.Tooltip.Root = "z-50 w-fit rounded-md border border-border bg-popover px-2 py-1 text-[11px] text-popover-foreground shadow-md " +
		"transition duration-150 starting:scale-95 starting:opacity-0"
	s.Tooltip.Arrow = "hidden"
	s.Tooltip.ArrowSides = nil

	// Separators between the groups of the toolbar and the sidebar.
	s.Separator.Root = "shrink-0 bg-border"
	return s
}
