package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// Frosted graphite is a cool graphite scale with one accent, amber, and
// glass in the manner of Apple's materials: panes of translucent graphite
// (or white, in the light scheme) over a wallpaper, blurred. It is the
// kit's three levels of customization (ADR-129):
//
//  1. Tokens. theme is kit.Theme() with the colors of the page, dark first.
//     Besides shadcn/ui's names it has pane and pane-border (the glass of
//     the three panes and of the palette), menu (the fill of the menus,
//     lists and toasts, nearly opaque, so they need no blur), accent-text
//     (amber that reads on both schemes) and negative.
//  2. Recipes. style edits kit.DefaultStyle(): hairline cards, an amber
//     primary button, a segmented control with a sliding pill, small
//     uppercase tabs, the glass dialog and the menus. kit.StyleProvider
//     applies it to the whole page.
//  3. Class on one component, where an instance differs.

func hex(s string) tela.Color                  { return tela.Hex(s) }
func white(a float32) tela.Color               { return tela.RGBA(255, 255, 255, a) }
func black(a float32) tela.Color               { return tela.RGBA(0, 0, 0, a) }
func rgba(r, g, b uint8, a float32) tela.Color { return tela.RGBA(r, g, b, a) }

var theme = graphiteTheme()

func graphiteTheme() tela.Theme {
	t := kit.Theme()
	t.Radius = 12
	set := func(name string, dark, light tela.Color) {
		t.Colors[name] = tela.SchemeColor{Light: light, Dark: dark}
	}
	set("background", hex("#0b0d11"), hex("#eceef2"))
	set("foreground", hex("#e9ebef"), hex("#14161a"))
	set("card", white(0.045), white(0.55))
	set("card-foreground", hex("#e9ebef"), hex("#14161a"))
	set("popover", rgba(18, 21, 27, 0.72), white(0.72))
	set("popover-foreground", hex("#e9ebef"), hex("#14161a"))
	set("primary", hex("#f6b94a"), hex("#f1a31c"))
	set("primary-foreground", hex("#1c1403"), hex("#1c1403"))
	set("secondary", white(0.06), black(0.05))
	set("secondary-foreground", hex("#e9ebef"), hex("#14161a"))
	set("muted", white(0.06), black(0.05))
	set("muted-foreground", hex("#8d95a3"), hex("#5a6270"))
	set("accent", white(0.09), black(0.06))
	set("accent-foreground", hex("#f4f5f7"), hex("#0b0c0e"))
	set("destructive", hex("#f0716b"), hex("#d23a35"))
	set("border", white(0.085), black(0.08))
	set("input", white(0.12), black(0.12))
	set("ring", rgba(246, 185, 74, 0.55), rgba(241, 163, 28, 0.6))
	// The page's own tokens are assigned with constant keys so that tela
	// lint learns them (it does not follow set).
	t.Colors["negative"] = tela.SchemeColor{Dark: hex("#f0716b"), Light: hex("#d23a35")}
	t.Colors["pane"] = tela.SchemeColor{Dark: rgba(14, 17, 23, 0.42), Light: white(0.42)}
	t.Colors["pane-border"] = tela.SchemeColor{Dark: white(0.10), Light: white(0.8)}
	t.Colors["menu"] = tela.SchemeColor{Dark: rgba(26, 29, 36, 0.97), Light: rgba(250, 250, 251, 0.97)}
	t.Colors["accent-text"] = tela.SchemeColor{Dark: hex("#f6b94a"), Light: hex("#9a5d00")}
	return t
}

// pane is a sheet of frosted glass: translucent graphite over the
// wallpaper, blurred, a hairline and a soft drop shadow. The three panes
// are fixed in the window and only their content scrolls, so what a pane
// blurs never moves (the glass budget in the README).
const pane = "rounded-2xl border border-pane-border bg-pane shadow-2xl shadow-black/25 backdrop-blur-2xl"

// eyebrow is the small uppercase caption over a section.
const eyebrow = "text-[11px] font-medium tracking-widest text-muted-foreground uppercase"

// iconBtn is a square button of the top bar.
const iconBtn = "flex size-8 items-center justify-center rounded-lg border border-border bg-white/[0.03] text-muted-foreground transition-colors outline-none " +
	"hover:bg-accent hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring"

// tipClass is the look of a tooltip: it is small and short-lived, so it is
// the opaque menu fill, without a blur.
const tipClass = "rounded-md border border-pane-border bg-menu px-2 py-1 text-xs font-medium text-popover-foreground shadow-lg"

// surface is the look of the floating lists (menus, the select's list, the
// notifications): nearly opaque, without a blur, since they animate in and
// out and a blur inside an animated layer stops the parallel rasterizer.
const surface = "rounded-xl border border-pane-border bg-menu text-popover-foreground shadow-xl shadow-black/30"

var style = graphiteStyle()

func graphiteStyle() kit.Style {
	s := kit.DefaultStyle()
	s.Card.Root = "flex flex-col gap-3 rounded-xl border border-border bg-card py-4 text-card-foreground"

	b := &s.Button
	b.Root = "inline-flex shrink-0 items-center justify-center gap-2 rounded-lg text-sm font-medium whitespace-nowrap transition duration-150 outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
	b.Variants[kit.Default] = "bg-primary text-primary-foreground hover:brightness-110"
	b.Variants[kit.Outline] = "border border-border bg-white/[0.03] text-foreground hover:bg-accent"
	b.Variants[kit.Ghost] = "text-muted-foreground hover:bg-accent hover:text-foreground"
	b.Variants[kit.Destructive] = "bg-destructive text-white hover:brightness-110"
	b.Variants[kit.Link] = "text-accent-text hover:underline"
	b.Sizes[""] = "h-9 px-3.5"
	b.Sizes[kit.Sm] = "h-8 px-3 text-xs"
	b.HasIcon[""] = "px-3"

	g := &s.Badge
	g.Root = "inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 text-[11px] font-medium whitespace-nowrap"
	g.Variants[kit.Outline] = "border border-border text-muted-foreground"
	g.Variants[kit.Secondary] = "bg-muted text-muted-foreground"

	a := &s.Avatar
	a.Root = "relative flex size-7 shrink-0 overflow-hidden rounded-full select-none"
	a.Fallback = "flex size-full items-center justify-center rounded-full bg-accent text-[10px] font-semibold text-foreground"

	field := "w-full min-w-0 rounded-lg border border-input bg-white/[0.04] px-3 text-sm text-foreground outline-none transition-[color,box-shadow] " +
		"placeholder:text-muted-foreground focus-visible:ring-[3px] focus-visible:ring-ring aria-invalid:border-destructive"
	s.Input.Root = "h-9 " + field
	s.Textarea.Root = "flex min-h-16 py-2 " + field
	s.Select.Root = "flex w-fit items-center justify-between gap-2 rounded-lg border border-input bg-white/[0.04] px-3 py-2 text-sm whitespace-nowrap outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
	s.Select.Content = "z-50 flex min-w-32 flex-col overflow-hidden " + surface + " transition duration-150 ease-out"
	s.Select.Item = "relative flex w-full cursor-default items-center gap-2 rounded-md py-1.5 pe-8 ps-2 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground aria-disabled:pointer-events-none aria-disabled:opacity-50"

	// A segmented control: a muted track with a pill under the selected
	// item; the pill is a child of the item (Segment), so it can slide.
	s.ToggleGroup.Root = "flex w-fit items-center rounded-lg bg-muted p-0.5"
	s.ToggleGroup.Item = "relative"
	s.ToggleGroup.ItemWithoutSpacing = ""
	s.Toggle.Root = "inline-flex items-center justify-center gap-1.5 rounded-md text-[13px] font-medium whitespace-nowrap text-muted-foreground transition-colors duration-200 outline-none " +
		"hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring"
	s.Toggle.Sizes[""] = "h-7 px-3"
	s.Toggle.Sizes[kit.Sm] = "h-6 px-2.5 text-xs"
	s.Toggle.On = "checked:text-foreground aria-pressed:text-foreground"

	// The headline's tabs are small uppercase labels with an amber line
	// that slides under the selected one (Tabs.Indicator).
	t := &s.Tabs
	t.List = "inline-flex w-fit items-center text-muted-foreground"
	t.ListVariants[kit.TabsLine] = "gap-4"
	t.ListOrientations[kit.Horizontal] = ""
	t.Trigger = "group relative inline-flex items-center pb-1.5 text-[11px] font-medium tracking-widest whitespace-nowrap text-muted-foreground uppercase transition-colors outline-none " +
		"hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring aria-selected:text-foreground"
	t.TriggerVariants[kit.TabsLine] = ""

	tb := &s.Table
	tb.Table = "grid w-max min-w-full text-sm"
	tb.Head = "flex h-9 items-center border-b border-border px-3 text-start text-[11px] font-medium tracking-widest whitespace-nowrap text-muted-foreground uppercase"
	// The head sticks over rows that scroll: an opaque fill, no blur.
	tb.HeadSticky = "sticky top-0 z-20 bg-menu"
	tb.SortButton = "-mx-2 h-7 gap-1 px-2 text-[11px] font-medium tracking-widest uppercase"
	tb.Cell = "flex items-center border-b border-border/70 px-3 py-2 whitespace-nowrap transition-colors group-hover:bg-white/[0.04]"

	s.Checkbox.Root = "peer inline-flex size-4 shrink-0 items-center justify-center rounded-[5px] border border-input bg-white/[0.04] text-transparent outline-none transition " +
		"hover:border-primary/70 focus-visible:ring-[3px] focus-visible:ring-ring checked:border-primary checked:bg-primary checked:text-primary-foreground"
	s.Checkbox.Icon = "size-3"

	s.Switch.Root = "peer group inline-flex shrink-0 items-center rounded-full border border-transparent bg-input transition-colors outline-none " +
		"focus-visible:ring-[3px] focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 checked:bg-primary"
	s.Switch.Thumb = "pointer-events-none block rounded-full bg-foreground ring-0 transition-transform translate-x-0 group-checked:translate-x-[calc(100%-2px)] group-checked:bg-primary-foreground"

	// The dialogs are the glass that does blur: the page is behind them and
	// does not move while they are open. The scrim appears at once (a fade
	// over the window repaints the whole page on every frame of it) and
	// only the panel animates.
	d := &s.Dialog
	d.Overlay = "absolute inset-0 bg-black/45"
	d.OverlayExit = ""
	d.Root = "relative grid w-full max-w-md gap-5 rounded-2xl border border-pane-border bg-popover p-6 text-popover-foreground shadow-2xl shadow-black/40 backdrop-blur-2xl outline-none " +
		"transition duration-200 ease-out starting:scale-95 starting:opacity-0"
	d.Exit = "scale-95 opacity-0"
	d.Header = "flex flex-col gap-1.5"
	d.Title = "text-base font-semibold tracking-tight"
	d.Footer = "flex justify-end gap-2 pt-1"
	d.Close = "absolute top-4 end-4 flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors outline-none hover:bg-accent hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring"

	ad := &s.AlertDialog
	ad.Overlay = "absolute inset-0 bg-black/45"
	ad.OverlayExit = ""
	ad.Root = "relative grid w-full gap-4 rounded-2xl border border-pane-border bg-popover p-6 text-popover-foreground shadow-2xl shadow-black/40 backdrop-blur-2xl " +
		"transition duration-200 ease-out starting:scale-95 starting:opacity-0"
	ad.Title = "text-base font-semibold tracking-tight"

	for _, m := range []*kit.MenuStyle{&s.DropdownMenu, &s.ContextMenu} {
		m.Content = "z-50 min-w-40 overflow-x-hidden overflow-y-auto p-1 outline-hidden " + surface + " transition duration-150 starting:scale-95 starting:opacity-0"
		m.Item = "relative flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-[13px] outline-hidden select-none focus:bg-accent focus:text-accent-foreground " +
			"aria-disabled:pointer-events-none aria-disabled:opacity-40"
		m.Label = "px-2 pt-1.5 pb-1 " + eyebrow
	}

	s.Popover.Root = "z-50 w-80 p-0 outline-hidden " + surface + " transition duration-150 starting:scale-95 starting:opacity-0"

	to := &s.Toast
	to.Item = "absolute start-0 box-border flex w-full items-center gap-1.5 rounded-xl border border-pane-border bg-menu p-4 text-[13px] text-popover-foreground " +
		"shadow-xl shadow-black/30 transition-[transform,opacity,height,box-shadow] duration-[400ms] outline-none focus-visible:ring-2 focus-visible:ring-ring"
	to.Icon = "relative -ms-[3px] me-1 flex size-4 shrink-0 items-center justify-start text-primary"
	to.Description = "leading-[1.4] font-normal text-muted-foreground"
	to.Close = "absolute top-0 start-0 z-10 flex size-5 -translate-x-[35%] -translate-y-[35%] cursor-pointer items-center justify-center rounded-full border border-pane-border " +
		"bg-menu p-0 text-muted-foreground outline-none transition-colors hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
	return s
}
