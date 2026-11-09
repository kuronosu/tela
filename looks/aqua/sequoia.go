package aqua

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/internal/base"
)

// ctl is the hairline shadow under a control of Sequoia.
const ctl = "shadow-[0_0.5px_1px_var(--color-control-shadow)]"

// raisedSegment is the shadow of the white segment of a segmented control.
const raisedSegment = "shadow-[0_0.5px_1.5px_rgba(0,0,0,0.3)]"

// sequoiaTheme is the palette of macOS 11 to 15: a light gray window (a very
// dark one in dark), white content, and the accent 007AFF.
func sequoiaTheme() tela.Theme {
	tok, black, white := base.Tok, base.Black, base.White
	blue, blueDark := tela.Hex("#007aff"), tela.Hex("#0a84ff")
	return base.Theme(6, "Inter", merge(map[string]tela.SchemeColor{
		"background":           tok(tela.Hex("#ececec"), tela.Hex("#1e1e1e")),
		"foreground":           tok(black(0.85), white(0.88)),
		"card":                 tok(tela.Hex("#ffffff"), tela.Hex("#2a2a2a")),
		"card-foreground":      tok(black(0.85), white(0.88)),
		"popover":              tok(tela.Hex("#f6f6f6"), tela.Hex("#323232")),
		"popover-foreground":   tok(black(0.85), white(0.88)),
		"primary":              tok(blue, blueDark),
		"primary-foreground":   tok(tela.Hex("#ffffff"), tela.Hex("#ffffff")),
		"secondary":            tok(tela.Hex("#ffffff"), white(0.14)),
		"secondary-foreground": tok(black(0.85), white(0.9)),
		"muted":                tok(black(0.06), white(0.08)),
		"muted-foreground":     tok(black(0.5), white(0.55)),
		"accent":               tok(black(0.06), white(0.08)),
		"accent-foreground":    tok(black(0.85), white(0.9)),
		"destructive":          tok(tela.Hex("#ff3b30"), tela.Hex("#ff453a")),
		"destructive-text":     tok(tela.Hex("#c4001a"), tela.Hex("#ff7a7d")),
		"border":               tok(black(0.1), white(0.1)),
		"input":                tok(black(0.2), white(0.2)),
		"ring":                 tok(blue, blueDark),

		"toolbar":              tok(tela.Hex("#f4f4f4"), tela.Hex("#2c2c2c")),
		"sidebar":              tok(tela.Hex("#e3e3e5"), tela.Hex("#272727")),
		"field":                tok(tela.Hex("#ffffff"), white(0.1)),
		"edge":                 tok(black(0.1), white(0.1)),
		"selection":            tok(blue, blueDark),
		"selection-foreground": tok(tela.Hex("#ffffff"), tela.Hex("#ffffff")),
	}, extraTokens(tela.RGBA(0, 136, 255, 0.4), tela.RGBA(10, 145, 255, 0.45))))
}

// Sequoia's search field is borderless gray, with its magnifier close to the
// edge (the input has 28 px at its start).
const (
	sequoiaSearchIcon  = "pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground"
	sequoiaSearchField = "border-transparent bg-foreground/8 shadow-none focus-visible:bg-field"
)

// sequoiaRegions is Sequoia's window: a flat toolbar unified with the title
// bar, over a sidebar that runs the whole height (its head is the toolbar's
// lead, where the traffic lights are), and white content.
func sequoiaRegions() looks.Regions {
	return looks.Regions{
		Window:            "bg-background text-[13px] leading-4",
		Toolbar:           "flex h-[52px] shrink-0 items-center gap-2 bg-toolbar pr-3",
		ToolbarLead:       "flex h-full w-[216px] shrink-0 items-center justify-end gap-1 border-r border-border bg-sidebar px-2",
		ToolbarGroup:      "flex items-center gap-0.5",
		ToolbarEnd:        "flex shrink-0 items-center gap-1.5",
		ToolbarSegments:   "gap-0 rounded-[6px] bg-foreground/8 p-0.5",
		Location:          "flex h-7 min-w-0 flex-1 items-center overflow-hidden rounded-[6px] bg-foreground/6 px-0.5",
		LocationCurrent:   "font-semibold",
		LocationSeparator: "flex w-3 shrink-0 items-center justify-center text-muted-foreground",
		Search:            "relative w-[190px] shrink-0",
		SearchIcon:        sequoiaSearchIcon,
		SearchField:       sequoiaSearchField,
		Sidebar:           "flex w-[216px] shrink-0 flex-col gap-px overflow-y-auto border-r border-border bg-sidebar p-2.5",
		SidebarHeading:    "px-2 pt-3 pb-1 text-[11px] font-semibold text-muted-foreground first:pt-1",
		SidebarGroup:      "gap-px",
		Content:           "flex min-w-0 flex-1 flex-col overflow-hidden border-t border-border bg-card",
		Details:           "hidden w-[260px] shrink-0 flex-col gap-3 overflow-y-auto border-t border-l border-border bg-background p-3.5 lg:flex",
		DetailsTitle:      "text-[14px] leading-5 font-semibold break-words",
		DetailsLabel:      "text-[11px] font-semibold text-muted-foreground",
		DetailsValue:      "text-[13px]",
		StatusBar:         "flex h-6 shrink-0 items-center justify-between gap-4 border-t border-border bg-toolbar px-3 text-[11px] text-muted-foreground",
	}
}

// sequoiaFrame is the toggles of the sidebar's entries and the toolbar's view
// switcher.
func sequoiaFrame(s *kit.Style) {
	g := &s.Toggle
	g.Variants[looks.Sidebar] = "relative h-7 w-full justify-start gap-2 rounded-[6px] px-2 text-left font-normal text-foreground hover:bg-foreground/5 checked:bg-foreground/10 checked:hover:bg-foreground/10"
	g.Sizes[looks.SidebarSize] = ""
	g.Variants[looks.Toolbar] = "rounded-[5px] text-foreground/70 hover:text-foreground checked:bg-segment checked:text-foreground checked:" + raisedSegment
	g.Sizes[looks.ToolbarSize] = "h-5 w-8"
	g.Icon = "pointer-events-none size-[15px] shrink-0"
	// The glyphs of a source list are tinted with the accent, selected or not.
	g.IconVariants[looks.Sidebar] = "text-primary"
}

// sequoiaControls is the 6 px controls: white push buttons with a hairline
// shadow, small checkboxes and radios, 38x22 switches, a slim slider and the
// segmented control.
func sequoiaControls(s *kit.Style) {
	const fs = fontSize
	b := &s.Button
	b.Root = "inline-flex shrink-0 items-center justify-center gap-1.5 rounded-[6px] " + fs + " whitespace-nowrap transition-colors outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:pointer-events-none disabled:opacity-40"
	b.Variants[kit.Default] = "bg-primary text-primary-foreground shadow-[inset_0_0.5px_0_rgba(255,255,255,0.3),0_0.5px_1px_var(--color-control-shadow)] hover:bg-primary/92 active:bg-primary/80"
	b.Variants[kit.Secondary] = "border border-border bg-secondary text-secondary-foreground " + ctl + " hover:bg-secondary/80 active:bg-foreground/8"
	b.Variants[kit.Outline] = b.Variants[kit.Secondary]
	b.Variants[kit.Destructive] = "border border-border bg-secondary text-destructive-text " + ctl + " hover:bg-secondary/80 active:bg-foreground/8 " +
		"dark:bg-black/50 dark:hover:bg-black/40 dark:active:bg-black/60"
	b.Variants[kit.Ghost] = "text-foreground/80 hover:bg-foreground/7 active:bg-foreground/12"
	b.Variants[kit.Link] = "text-primary hover:underline"
	b.Sizes[""] = "h-6 px-3"
	b.Sizes[kit.Sm] = "h-5 gap-1 rounded-[5px] px-2 text-[11px]"
	b.Sizes[kit.Icon] = "h-6 w-7"
	b.Sizes[kit.IconSm] = "size-6"
	b.HasIcon[""] = "px-2.5"
	b.HasIcon[kit.Sm] = "px-1.5"

	s.Checkbox.Root = "peer inline-flex size-[14px] shrink-0 items-center justify-center rounded-[4px] border border-input bg-field " + ctl +
		" outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-40 " +
		"checked:border-primary checked:bg-primary checked:text-primary-foreground"
	s.Checkbox.Icon = "size-2.5 stroke-[3.5]"
	r := &s.RadioGroup
	r.Root = "grid gap-1 self-start"
	r.Option = "flex h-[22px] items-center gap-2"
	r.Item = "peer inline-flex aspect-square size-[14px] shrink-0 items-center justify-center rounded-full border border-input bg-field " + ctl +
		" outline-none focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-40 aria-checked:border-primary aria-checked:bg-primary"
	r.Icon = "absolute top-1/2 left-1/2 size-[6px] -translate-x-1/2 -translate-y-1/2 fill-white stroke-white"
	r.Label = "leading-4"
	r.Description = "text-[11px] text-muted-foreground"

	// 38x22 with an 18 px white knob.
	sw := &s.Switch
	sw.Root = "peer group inline-flex shrink-0 items-center rounded-full bg-foreground/16 px-0.5 outline-none transition-colors " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-40 checked:bg-primary"
	sw.Sizes[""] = "h-[22px] w-[38px]"
	sw.Sizes[kit.Sm] = "h-4 w-7"
	sw.Thumb = "pointer-events-none block rounded-full bg-white shadow-[0_1px_2.5px_rgba(0,0,0,0.35)] transition-transform translate-x-0 group-checked:translate-x-[calc(100%-2px)]"
	sw.ThumbSizes[""] = "size-[18px]"
	sw.ThumbSizes[kit.Sm] = "size-3"

	sl := &s.Slider
	sl.Track = "relative grow overflow-hidden rounded-full bg-foreground/14"
	sl.TrackOrientations[kit.Horizontal] = "h-1 w-full"
	sl.Thumb = "block size-4 shrink-0 rounded-full border border-black/10 bg-white shadow-[0_1px_3px_rgba(0,0,0,0.35)] ring-ring/30 transition-shadow " +
		"hover:ring-4 focus-visible:ring-4 focus-visible:outline-hidden"
	s.Progress.Root = "relative h-1.5 w-full overflow-hidden rounded-full bg-foreground/14"

	// The text field and the pop-up button (its double chevron sits in an
	// accent tile).
	s.Input.Root = "h-6 w-full min-w-0 rounded-[5px] border border-input bg-field px-2 pl-7 " + fs + " " + ctl + " outline-none " +
		"placeholder:text-muted-foreground selection:bg-primary selection:text-primary-foreground " +
		"focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:opacity-50"
	sel := &s.Select
	sel.Root = "flex w-full items-center justify-between gap-2 rounded-[6px] border border-border bg-secondary pl-2.5 pr-[3px] " + fs + " whitespace-nowrap " + ctl + " outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:cursor-not-allowed disabled:opacity-40"
	sel.Sizes[""] = "h-6"
	sel.Sizes[kit.Sm] = "h-5"
	sel.Icon = "pointer-events-none size-[18px] shrink-0 rounded-[4px] bg-primary p-[3px] text-primary-foreground opacity-100"
	sel.DefaultIcon = chevronsUpDown
	base.Swap("rounded-[6px] border", "rounded-[10px] border", &sel.Content)
	sel.Content += " shadow-xl"
	sel.Item = "relative flex w-full cursor-default items-center gap-2 rounded-[5px] py-1 pe-8 ps-2 " + fs + " outline-hidden select-none focus:bg-primary focus:text-primary-foreground aria-disabled:opacity-50"

	// Toggles: white push buttons that turn accent when on.
	g := &s.Toggle
	g.Variants[kit.Default] = "hover:bg-foreground/7 aria-pressed:bg-foreground/10 checked:bg-foreground/10"
	g.Variants[kit.Outline] = "border border-border bg-secondary " + ctl + " hover:bg-secondary/80 aria-pressed:bg-primary aria-pressed:text-primary-foreground checked:bg-primary checked:text-primary-foreground"
	g.Root = "inline-flex items-center justify-center gap-1.5 rounded-[6px] " + fs + " whitespace-nowrap transition-colors outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/45 disabled:pointer-events-none disabled:opacity-40"
	g.Sizes[""] = "h-6 min-w-6 px-3"
	g.Sizes[kit.Sm] = "h-5 min-w-5 px-2 text-[11px]"
	g.Sizes[kit.Lg] = "h-7 min-w-7 px-4"

	// The segmented control.
	tb := &s.Tabs
	tb.List = "inline-flex w-fit items-center justify-center rounded-[6px] p-0.5 text-foreground"
	tb.ListVariants[kit.Default] = "bg-foreground/8"
	tb.ListOrientations[kit.Horizontal] = "h-6"
	tb.Trigger = "group relative inline-flex h-full flex-1 items-center justify-center rounded-[5px] px-3 " + fs + " whitespace-nowrap text-foreground outline-none transition-colors " +
		"hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/50 disabled:opacity-40 " +
		"aria-selected:bg-segment aria-selected:" + raisedSegment
	tb.TriggerVariants[kit.Default] = ""
	tb.Content = "flex-1 outline-none"
}

// sequoiaSurfaces is what Sequoia makes of the tables, menus, dialogs,
// tooltips and cards: small gray headers, 24 px rows, 10 px menus and group
// boxes with a hairline.
func sequoiaSurfaces(s *kit.Style) {
	t := &s.Table
	t.Root += " px-1.5 pb-1.5"
	t.Head += " h-6 border-b border-border text-[11px] font-semibold text-muted-foreground"
	t.HeadSticky = "sticky top-0 z-20 bg-card"
	t.SortButton = "-ml-2 h-5 px-2 text-[11px] font-semibold text-muted-foreground hover:bg-transparent hover:text-foreground active:bg-transparent"
	t.Cell += " h-6 px-2 first:rounded-l-[5px] last:rounded-r-[5px]"
	t.Empty += " text-[13px]"

	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		base.Swap("rounded-[6px] border", "rounded-[10px] border", &m.Content)
		base.Swap("rounded-sm px-2", "rounded-[5px] px-2.5", &m.Item, &m.CheckboxItem, &m.RadioItem)
		m.Content += " shadow-xl"
	}
	base.Swap("rounded-lg border", "rounded-[12px] border", &s.Dialog.Root, &s.AlertDialog.Root)

	// A group box of a settings window: white, 10 px, hairline.
	c := &s.Card
	c.Root = "flex flex-col gap-3 rounded-[10px] border border-border bg-card py-3.5 text-card-foreground " + ctl
	c.Header = "grid auto-rows-min grid-rows-[auto_auto] items-start gap-0.5 px-4"
	c.Title = "leading-none font-semibold"
	c.Description = "text-[12px] text-muted-foreground"
	c.Content = "px-4"
	c.Footer = "flex items-center px-4"
}
