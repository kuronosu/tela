// Package base is what the looks share: the helpers that build a theme, and
// the starting point of their kit recipes.
package base

import (
	"reflect"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// Tok is a token with a light and a dark value.
func Tok(light, dark tela.Color) tela.SchemeColor {
	return tela.SchemeColor{Light: light, Dark: dark}
}

// Black is black with an opacity.
func Black(a float32) tela.Color { return tela.RGBA(0, 0, 0, a) }

// White is white with an opacity.
func White(a float32) tela.Color { return tela.RGBA(255, 255, 255, a) }

// Theme is kit's theme with the tokens of a look, its base radius and its
// font.
func Theme(radius float32, font string, colors map[string]tela.SchemeColor) tela.Theme {
	t := kit.Theme()
	t.Radius = radius
	t.FontSans = font
	for name, c := range colors {
		t.Colors[name] = c
	}
	// Every look defines the same tokens, so that the classes of any recipe
	// resolve under any theme (a preview rendered by the tools has one theme
	// for all looks): the ones a look does not need follow its surfaces.
	// Written out, not in a loop over the names, so that tela lint sees them.
	if _, ok := colors["toolbar"]; !ok {
		t.Colors["toolbar"] = t.Colors["background"]
	}
	if _, ok := colors["sidebar"]; !ok {
		t.Colors["sidebar"] = t.Colors["background"]
	}
	if _, ok := colors["field"]; !ok {
		t.Colors["field"] = t.Colors["card"]
	}
	if _, ok := colors["edge"]; !ok {
		t.Colors["edge"] = t.Colors["border"]
	}
	// The tokens of Aqua's glass and controls (package looks lists them):
	// the other looks have them follow their surfaces.
	if _, ok := colors["glass"]; !ok {
		t.Colors["glass"] = t.Colors["card"]
	}
	if _, ok := colors["glass-strong"]; !ok {
		t.Colors["glass-strong"] = t.Colors["card"]
	}
	if _, ok := colors["glass-selected"]; !ok {
		t.Colors["glass-selected"] = t.Colors["accent"]
	}
	if _, ok := colors["glass-edge"]; !ok {
		t.Colors["glass-edge"] = t.Colors["border"]
	}
	if _, ok := colors["glass-highlight"]; !ok {
		t.Colors["glass-highlight"] = t.Colors["border"]
	}
	if _, ok := colors["glass-shadow"]; !ok {
		t.Colors["glass-shadow"] = Tok(Black(0.15), Black(0.4))
	}
	if _, ok := colors["primary-glow"]; !ok {
		t.Colors["primary-glow"] = t.Colors["ring"]
	}
	if _, ok := colors["control-shadow"]; !ok {
		t.Colors["control-shadow"] = Tok(Black(0.15), Black(0.4))
	}
	if _, ok := colors["segment"]; !ok {
		t.Colors["segment"] = t.Colors["accent"]
	}
	return t
}

// Swap replaces old by new in every class string given.
func Swap(old, new string, classes ...*string) {
	for _, c := range classes {
		*c = strings.ReplaceAll(*c, old, new)
	}
}

// Style is the starting point of the three recipes: kit's defaults with the
// edits every look needs because a window of an app is not a shadcn page. Each
// look then edits what is its own. Colors never appear in a recipe unless a
// token cannot say them: the classes name tokens (bg-accent,
// text-muted-foreground, bg-selection), and each look's tokens give them
// their values in light and in dark.
//
// fs is the look's font size class: portals (menus, dialogs, tooltips) do not
// inherit it from the window, so the recipes of what they hold say it
// themselves.
func Style(fs string) kit.Style {
	s := kit.DefaultStyle()

	// Buttons: flat, token colored; the sizes are the ones a desktop app has
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

	// Tables: lists of rows. Rows have no box of their own, so the hover
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

	// Menus (a dropdown, a context menu): sized for the look.
	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		Swap("text-sm", fs, &m.Item, &m.CheckboxItem, &m.RadioItem, &m.Label)
		m.Content += " " + fs
		m.Content = strings.ReplaceAll(m.Content, "min-w-32", "min-w-48")
		m.Separator = "-mx-1 my-1 h-px bg-border"
	}

	// Dialogs sit on the popover surface.
	for _, d := range []*string{&s.Dialog.Root, &s.AlertDialog.Root} {
		Swap("bg-background", "bg-popover", d)
		*d += " text-popover-foreground " + fs
	}
	s.Dialog.Root = strings.ReplaceAll(s.Dialog.Root, "max-w-lg", "max-w-[420px]")
	s.AlertDialog.Sizes[""] = "max-w-[420px]"
	Swap("text-lg", "text-[16px]", &s.Dialog.Title, &s.AlertDialog.Title)
	Swap("text-sm", fs, &s.Dialog.Description, &s.AlertDialog.Description)
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

	// A toggle that is on shows it in every variant (the group's On is
	// empty, see above); the outline one has the look's border.
	s.Toggle.Variants[kit.Default] = "hover:bg-accent aria-pressed:bg-accent checked:bg-accent"
	s.Toggle.Variants[kit.Outline] = "border border-input bg-card hover:bg-accent aria-pressed:bg-accent checked:bg-accent"

	// Cards: a flat surface with a hairline, closer to a group of a settings
	// window than to a shadcn card.
	c := &s.Card
	c.Root = "flex flex-col gap-3 rounded-lg border border-border bg-card py-4 text-card-foreground"
	c.Header = "grid auto-rows-min grid-rows-[auto_auto] items-start gap-1 px-4"
	c.Content = "px-4"
	c.Footer = "flex items-center px-4"

	// Labels are plain text on a desktop.
	s.Label.Root = strings.ReplaceAll(s.Label.Root, "font-medium", "font-normal")

	// The components that no recipe above sized read the window's text size
	// too.
	retext(reflect.ValueOf(&s).Elem(), fs)
	return s
}

// retext replaces the text-sm class by fs in every string of v: the recipes
// of the components a look does not size itself (selects, tabs, cards...) are
// written for shadcn's 14 px.
func retext(v reflect.Value, fs string) {
	switch v.Kind() {
	case reflect.String:
		v.SetString(strings.ReplaceAll(v.String(), "text-sm", fs))
	case reflect.Struct:
		for i := range v.NumField() {
			retext(v.Field(i), fs)
		}
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			e := reflect.New(v.Type().Elem()).Elem()
			e.Set(it.Value())
			retext(e, fs)
			v.SetMapIndex(it.Key(), e)
		}
	}
}
