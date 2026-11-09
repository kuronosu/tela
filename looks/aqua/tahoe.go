package aqua

import (
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/internal/base"
)

// The recipes of a piece of glass: a backdrop blur, a translucent fill, a
// light 1 px edge and an inset highlight along the top, with a soft shadow.
// Panel is a large piece (the sidebar), Bar a group of the toolbar, Small a
// control.
const (
	glassPanel = "bg-glass backdrop-blur-2xl backdrop-saturate-150 border border-glass-edge shadow-[inset_0_1px_0_var(--color-glass-highlight),0_8px_28px_var(--color-glass-shadow)]"
	glassBar   = "bg-glass backdrop-blur-2xl backdrop-saturate-150 border border-glass-edge shadow-[inset_0_1px_0_var(--color-glass-highlight),0_4px_16px_var(--color-glass-shadow)]"
	glassSmall = "bg-glass-strong backdrop-blur-xl backdrop-saturate-150 border border-glass-edge shadow-[inset_0_1px_0_var(--color-glass-highlight),0_2px_8px_var(--color-glass-shadow)]"
	// glassRaised is the selected piece inside a glass one.
	glassRaised = "shadow-[inset_0_1px_0_var(--color-glass-highlight),0_2px_6px_var(--color-glass-shadow)]"
)

// extraTokens are the tokens of the glass and of the controls that both
// designs define, so that any mix resolves: Tahoe's glass, Sequoia's hairline
// under a control and its raised segment.
func extraTokens(glow, glowDark tela.Color) map[string]tela.SchemeColor {
	tok, black, white := base.Tok, base.Black, base.White
	return map[string]tela.SchemeColor{
		"glass":           tok(white(0.46), white(0.07)),
		"glass-strong":    tok(white(0.62), white(0.12)),
		"glass-selected":  tok(white(0.7), white(0.17)),
		"glass-edge":      tok(white(0.7), white(0.16)),
		"glass-highlight": tok(white(0.95), white(0.32)),
		"glass-shadow":    tok(black(0.1), black(0.35)),
		"primary-glow":    tok(glow, glowDark),
		"control-shadow":  tok(black(0.18), black(0.5)),
		"segment":         tok(tela.Hex("#ffffff"), tela.Hex("#636366")),
	}
}

// merge adds the extra tokens to a design's.
func merge(colors, extra map[string]tela.SchemeColor) map[string]tela.SchemeColor {
	for k, v := range extra {
		colors[k] = v
	}
	return colors
}

// tahoeTheme is the palette of macOS 26: a cool light gray (a near black in
// dark) with white glass over it, and the accent 0088FF.
func tahoeTheme() tela.Theme {
	tok, black, white := base.Tok, base.Black, base.White
	blue, blueDark := tela.Hex("#0088ff"), tela.Hex("#0a91ff")
	return base.Theme(16, "Inter", merge(map[string]tela.SchemeColor{
		"background":           tok(tela.Hex("#f2f3f8"), tela.Hex("#17181c")),
		"foreground":           tok(black(0.85), white(0.9)),
		"card":                 tok(tela.Hex("#ffffff"), tela.Hex("#25262b")),
		"card-foreground":      tok(black(0.85), white(0.9)),
		"popover":              tok(white(0.8), tela.Hex("#2b2c31")),
		"popover-foreground":   tok(black(0.85), white(0.9)),
		"primary":              tok(blue, blueDark),
		"primary-foreground":   tok(tela.Hex("#ffffff"), tela.Hex("#ffffff")),
		"secondary":            tok(white(0.7), white(0.12)),
		"secondary-foreground": tok(black(0.85), white(0.92)),
		"muted":                tok(black(0.06), white(0.08)),
		"muted-foreground":     tok(black(0.5), white(0.58)),
		"accent":               tok(black(0.06), white(0.1)),
		"accent-foreground":    tok(black(0.85), white(0.92)),
		"destructive":          tok(tela.Hex("#ff383c"), tela.Hex("#ff4245")),
		"destructive-text":     tok(tela.Hex("#ff383c"), tela.Hex("#ff7a7d")),
		"border":               tok(black(0.08), white(0.1)),
		"input":                tok(black(0.18), white(0.22)),
		"ring":                 tok(blue, blueDark),

		"toolbar":              tok(white(0.5), white(0.08)),
		"sidebar":              tok(white(0.5), white(0.08)),
		"field":                tok(white(0.7), white(0.1)),
		"edge":                 tok(black(0.08), white(0.1)),
		"selection":            tok(tela.RGBA(0, 136, 255, 0.9), tela.RGBA(10, 145, 255, 0.85)),
		"selection-foreground": tok(tela.Hex("#ffffff"), tela.Hex("#ffffff")),
	}, extraTokens(tela.RGBA(0, 136, 255, 0.4), tela.RGBA(10, 145, 255, 0.45))))
}

// tahoeRegions is Tahoe's window: the toolbar's groups, the location bar and
// the sidebar are glass floating over the window's soft wash of the accent and
// the destructive color, and the content and the details pane sit inset
// beside them.
func tahoeRegions() looks.Regions {
	return looks.Regions{
		Window:            "bg-background bg-linear-to-br from-primary/15 to-destructive/10 text-[13px] leading-4",
		Toolbar:           "flex h-[60px] shrink-0 items-center gap-2.5 px-3",
		ToolbarGroup:      "flex items-center gap-0.5 rounded-full p-[3px] " + glassBar,
		ToolbarEnd:        "flex shrink-0 items-center gap-2.5",
		ToolbarSegments:   "gap-0.5 rounded-full p-[3px] " + glassBar,
		Location:          "flex h-10 min-w-0 flex-1 items-center overflow-hidden rounded-full px-1.5 " + glassBar,
		LocationCurrent:   "bg-foreground/8 font-semibold",
		LocationSeparator: "flex w-3 shrink-0 items-center justify-center text-muted-foreground",
		Search:            "relative w-[200px] shrink-0",
		SearchIcon:        "pointer-events-none absolute top-1/2 left-3 z-10 size-3.5 -translate-y-1/2 text-muted-foreground",
		Sidebar:           "mb-3 ml-3 flex w-[226px] shrink-0 flex-col gap-px overflow-y-auto rounded-[22px] p-2.5 " + glassPanel,
		SidebarHeading:    "px-2.5 pt-3 pb-1 text-[11px] font-semibold text-muted-foreground first:pt-1",
		SidebarGroup:      "gap-0.5",
		Content:           "mx-3 mb-3 flex min-w-0 flex-1 flex-col overflow-hidden rounded-[22px] border border-border bg-background shadow-[0_4px_16px_var(--color-glass-shadow)]",
		Details:           "mr-3 mb-3 hidden w-[260px] shrink-0 flex-col gap-4 overflow-y-auto rounded-[22px] p-4 lg:flex " + glassPanel,
		DetailsTitle:      "text-[15px] leading-5 font-semibold break-words",
		DetailsLabel:      "text-[11px] font-semibold text-muted-foreground",
		DetailsValue:      "text-[13px]",
		StatusBar:         "flex h-9 shrink-0 items-center justify-between gap-4 px-6 text-[12px] text-muted-foreground",
	}
}

// tahoeFrame is the toggles of the sidebar's entries and the toolbar's view
// switcher.
func tahoeFrame(s *kit.Style) {
	g := &s.Toggle
	g.Variants[looks.Sidebar] = "relative h-8 w-full justify-start gap-2.5 rounded-full px-3 text-left font-normal text-foreground hover:bg-foreground/6 " +
		"checked:bg-glass-selected checked:font-medium checked:" + glassRaised + " checked:hover:bg-glass-selected"
	g.Sizes[looks.SidebarSize] = ""
	g.Variants[looks.Toolbar] = "rounded-full text-foreground/75 hover:text-foreground checked:bg-glass-selected checked:text-foreground checked:" + glassRaised
	g.Sizes[looks.ToolbarSize] = "h-[30px] w-10"
	g.Icon = "pointer-events-none size-4 shrink-0"
	g.IconVariants[looks.Sidebar] = "text-primary"
}

// tahoeControls is the capsule controls: buttons, fields, selects, toggles,
// checkboxes, radios, switches, sliders and segmented controls.
func tahoeControls(s *kit.Style) {
	const fs = fontSize
	b := &s.Button
	b.Root = "inline-flex shrink-0 items-center justify-center gap-2 rounded-full " + fs + " font-medium whitespace-nowrap transition-colors outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:pointer-events-none disabled:opacity-40"
	// The default button is tinted glass: the accent at 90% over a highlight.
	b.Variants[kit.Default] = "bg-primary/90 text-primary-foreground border border-white/25 shadow-[inset_0_1px_0_rgba(255,255,255,0.5),0_4px_12px_var(--color-primary-glow)] hover:bg-primary active:bg-primary/80"
	b.Variants[kit.Secondary] = glassSmall + " text-secondary-foreground hover:bg-glass-selected active:bg-foreground/10"
	b.Variants[kit.Outline] = b.Variants[kit.Secondary]
	// Red text needs a darker pane in dark: over a saturated backdrop the
	// glass is a mid-tone, where no red passes AA, so the tint is black.
	b.Variants[kit.Destructive] = glassSmall + " text-destructive-text hover:bg-glass-selected active:bg-foreground/10 " +
		"dark:bg-black/50 dark:hover:bg-black/40 dark:active:bg-black/60"
	b.Variants[kit.Ghost] = "text-foreground/80 hover:bg-foreground/8 active:bg-foreground/14"
	b.Variants[kit.Link] = "text-primary hover:underline"
	b.Sizes[""] = "h-8 px-4"
	b.Sizes[kit.Sm] = "h-7 gap-1.5 px-3 text-[12px]"
	b.Sizes[kit.Icon] = "size-8"
	b.Sizes[kit.IconSm] = "size-7"
	b.HasIcon[""] = "px-3.5"
	b.HasIcon[kit.Sm] = "px-2.5"

	s.Checkbox.Root = "peer inline-flex size-[18px] shrink-0 items-center justify-center rounded-[6px] border border-input bg-glass-strong " +
		"shadow-[inset_0_1px_0_var(--color-glass-highlight)] outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-40 " +
		"checked:border-primary checked:bg-primary checked:text-primary-foreground checked:shadow-[inset_0_1px_0_rgba(255,255,255,0.4),0_2px_6px_var(--color-primary-glow)]"
	s.Checkbox.Icon = "size-3 stroke-[3.5]"
	r := &s.RadioGroup
	r.Root = "grid gap-1 self-start"
	r.Option = "flex h-6 items-center gap-2.5"
	r.Item = "peer inline-flex aspect-square size-[18px] shrink-0 items-center justify-center rounded-full border border-input bg-glass-strong " +
		"shadow-[inset_0_1px_0_var(--color-glass-highlight)] outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-40 " +
		"aria-checked:border-primary aria-checked:bg-primary aria-checked:shadow-[inset_0_1px_0_rgba(255,255,255,0.4),0_2px_6px_var(--color-primary-glow)]"
	r.Icon = "absolute top-1/2 left-1/2 size-2 -translate-x-1/2 -translate-y-1/2 fill-white stroke-white"
	r.Label = "leading-4"
	r.Description = "text-[12px] text-muted-foreground"

	// A wide white knob on a 48x28 track.
	sw := &s.Switch
	sw.Root = "peer group inline-flex shrink-0 items-center rounded-full bg-foreground/14 px-[3px] shadow-[inset_0_1px_2px_rgba(0,0,0,0.12)] outline-none transition-colors " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-40 checked:bg-primary"
	sw.Sizes[""] = "h-7 w-[48px]"
	sw.Sizes[kit.Sm] = "h-5 w-[38px]"
	sw.Thumb = "pointer-events-none block rounded-full bg-white shadow-[0_2px_6px_rgba(0,0,0,0.28),inset_0_1px_0_rgba(255,255,255,1)] transition-transform translate-x-0 group-checked:translate-x-[10px]"
	sw.ThumbSizes[""] = "h-[22px] w-[32px]"
	sw.ThumbSizes[kit.Sm] = "h-4 w-[22px]"

	sl := &s.Slider
	sl.Track = "relative grow overflow-hidden rounded-full bg-foreground/12"
	sl.TrackOrientations[kit.Horizontal] = "h-1.5 w-full"
	sl.Thumb = "block h-[18px] w-[30px] shrink-0 rounded-full border border-black/5 bg-white shadow-[0_2px_8px_rgba(0,0,0,0.25)] ring-ring/30 transition-shadow " +
		"hover:ring-4 focus-visible:ring-4 focus-visible:outline-hidden"
	s.Progress.Root = "relative h-2 w-full overflow-hidden rounded-full bg-foreground/12"

	s.Input.Root = "h-9 w-full min-w-0 rounded-full border border-glass-edge bg-glass-strong px-4 pl-9 " + fs + " shadow-[inset_0_1px_2px_rgba(0,0,0,0.08)] outline-none " +
		"placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground " +
		"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-50"
	sel := &s.Select
	sel.Root = "flex w-full items-center justify-between gap-2 rounded-full pl-4 pr-3 " + fs + " whitespace-nowrap text-foreground " + glassSmall + " outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:cursor-not-allowed disabled:opacity-40"
	sel.Sizes[""] = "h-9"
	sel.Sizes[kit.Sm] = "h-7"
	sel.Icon = "pointer-events-none size-4 shrink-0 text-muted-foreground opacity-80"
	base.Swap("rounded-md border", "rounded-[20px] border", &sel.Content)
	sel.Content += " backdrop-blur-2xl shadow-2xl"
	sel.Item = "relative flex w-full cursor-default items-center gap-2 rounded-full py-1.5 pe-8 ps-3 " + fs + " outline-hidden select-none focus:bg-primary focus:text-primary-foreground aria-disabled:opacity-50"
	sel.Viewport = "min-h-0 flex-auto overflow-x-hidden overflow-y-auto p-1.5 outline-none"

	// Toggles: a glass capsule that lights up when on.
	g := &s.Toggle
	g.Variants[kit.Default] = "hover:bg-foreground/8 aria-pressed:bg-glass-selected checked:bg-glass-selected"
	g.Variants[kit.Outline] = glassSmall + " hover:bg-glass-selected aria-pressed:bg-primary/90 aria-pressed:text-primary-foreground checked:bg-primary/90 checked:text-primary-foreground"
	g.Root = "inline-flex items-center justify-center gap-2 rounded-full " + fs + " font-medium whitespace-nowrap transition-colors outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:pointer-events-none disabled:opacity-40"
	g.Sizes[""] = "h-8 min-w-8 px-4"
	g.Sizes[kit.Sm] = "h-7 min-w-7 px-3"
	g.Sizes[kit.Lg] = "h-9 min-w-9 px-5"

	// A segmented control: a glass capsule with a lighter capsule inside.
	tb := &s.Tabs
	tb.List = "inline-flex w-fit items-center justify-center rounded-full p-[3px] text-foreground " + glassSmall
	tb.ListVariants[kit.Default] = ""
	tb.ListOrientations[kit.Horizontal] = "h-9"
	tb.Trigger = "group relative inline-flex h-full flex-1 items-center justify-center rounded-full px-4 " + fs + " whitespace-nowrap text-foreground/80 outline-none transition-colors " +
		"hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-40 " +
		"aria-selected:bg-glass-selected aria-selected:font-medium aria-selected:text-foreground aria-selected:" + glassRaised
	tb.TriggerVariants[kit.Default] = ""
}

// readableRed makes the destructive text of a recipe the token
// destructive-text: in dark, the red over glass (a dark gray, or the wash of
// the window behind it) reads at 3.6:1 and the lighter red passes AA.
func readableRed(classes ...*string) {
	for _, c := range classes {
		*c = strings.ReplaceAll(*c, "text-destructive", "text-destructive-text")
	}
}

// tahoeSurfaces is what Tahoe makes of the tables, menus, dialogs, tooltips
// and cards: large radii, glass menus, and opaque cards.
func tahoeSurfaces(s *kit.Style) {
	t := &s.Table
	t.Root += " px-1 pb-1"
	t.Head += " h-7 border-b border-border px-2.5 text-[12px] font-semibold text-muted-foreground"
	t.HeadSticky = "sticky top-0 z-20 bg-background"
	t.SortButton = "-ml-2 h-6 px-2 text-[12px] font-semibold text-muted-foreground hover:bg-transparent hover:text-foreground active:bg-transparent"
	t.Cell += " h-8 px-2.5 first:rounded-l-full last:rounded-r-full"
	t.Empty += " text-[13px]"

	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		base.Swap("rounded-md border", "rounded-[20px] border", &m.Content)
		base.Swap("rounded-sm px-2", "rounded-full px-3", &m.Item, &m.CheckboxItem, &m.RadioItem)
		m.Content += " bg-popover backdrop-blur-2xl shadow-2xl"
	}
	readableRed(&s.Field.Invalid, &s.Field.Error)
	for _, m := range []map[kit.Variant]string{s.DropdownMenu.ItemVariants, s.DropdownMenu.ItemIconVariants,
		s.ContextMenu.ItemVariants, s.ContextMenu.ItemIconVariants, s.Alert.Variants, s.Alert.DescriptionVariants} {
		for k, v := range m {
			readableRed(&v)
			m[k] = v
		}
	}
	base.Swap("rounded-lg border", "rounded-[28px] border", &s.Dialog.Root, &s.AlertDialog.Root)
	s.Dialog.Root += " backdrop-blur-2xl"
	s.AlertDialog.Root += " backdrop-blur-2xl"
	s.Tooltip.Root = "z-50 w-fit rounded-full border border-glass-edge bg-popover px-3 py-1 text-[11px] text-popover-foreground shadow-md backdrop-blur-xl " +
		"transition duration-150 starting:scale-95 starting:opacity-0"

	c := &s.Card
	c.Root = "flex flex-col gap-3 rounded-[24px] border border-border bg-card py-4 text-card-foreground shadow-[0_4px_16px_var(--color-glass-shadow)]"
	c.Header = "grid auto-rows-min grid-rows-[auto_auto] items-start gap-0.5 px-5"
	c.Title = "text-[14px] leading-none font-semibold"
	c.Description = "text-[12px] text-muted-foreground"
	c.Content = "px-5"
	c.Footer = "flex items-center px-5"
}

// glassCards makes the cards glass.
func glassCards(s *kit.Style) {
	s.Card.Root = "flex flex-col gap-3 rounded-[24px] border border-glass-edge bg-card/70 py-4 text-card-foreground backdrop-blur-xl " +
		"shadow-[inset_0_1px_0_var(--color-glass-highlight),0_8px_24px_var(--color-glass-shadow)]"
}
