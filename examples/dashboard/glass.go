package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// The glass look, as the kit's three levels of customization (ADR-129,
// K17): tokens, recipes and per-instance classes.
//
//  1. theme is kit.Theme() with the colors of the page. The glass lives in
//     the tokens: border is a hairline of white (white/60, white/10 in
//     dark), card a translucent fill, popover the fill of the dialog and
//     the toasts, accent the lighter pill of a hover or a selection. A kit
//     class such as border-border or bg-card is then already glass, and one
//     class serves both schemes (no dark: pairs in the page's own markup
//     either, where a token fits).
//  2. glassStyle edits kit.DefaultStyle(): what a token cannot say (the
//     indigo to fuchsia gradient of the primary button, the sliding pill
//     of the segmented controls, the blur of the dialog and the toasts, the
//     rounded-2xl cards) is a class in a recipe or a new variant of one
//     ("glass", "tint", "nav", "fab" buttons; "live", "up", "down" badges).
//     kit.StyleProvider applies it to the whole page.
//  3. Class on one component, for the few instances that differ (the stat
//     cards lift when hovered, the sort chips are smaller).

// oklch is a palette color of Tailwind v4, which is written in OKLCH.
func oklch(l, c, h float32) tela.Color { return tela.OKLCH(l, c, h) }

var (
	slate50   = oklch(0.984, 0.003, 247.858)
	slate100  = oklch(0.968, 0.007, 247.896)
	slate400  = oklch(0.704, 0.04, 256.788)
	slate600  = oklch(0.446, 0.043, 257.281)
	slate900  = oklch(0.208, 0.042, 265.755)
	slate950  = oklch(0.129, 0.042, 264.695)
	indigo300 = oklch(0.785, 0.115, 274.713)
	indigo400 = oklch(0.673, 0.182, 276.935)
	indigo500 = oklch(0.585, 0.233, 277.117)
	indigo600 = oklch(0.511, 0.262, 276.966)
	rose500   = oklch(0.645, 0.246, 16.439)
)

func white(a float32) tela.Color { return tela.RGBA(255, 255, 255, a) }

// theme is the kit's theme with the colors of the page.
var theme = glassTheme()

func glassTheme() tela.Theme {
	t := kit.Theme()
	set := func(name string, light, dark tela.Color) {
		t.Colors[name] = tela.SchemeColor{Light: light, Dark: dark}
	}
	set("background", slate50, slate950)
	set("foreground", slate900, slate100)
	set("card", white(0.55), white(0.05))
	set("card-foreground", slate900, slate100)
	set("popover", white(0.65), slate900.WithAlpha(0.6))
	set("popover-foreground", slate900, slate100)
	set("primary", indigo600, indigo300)
	set("primary-foreground", white(1), slate950)
	set("secondary", white(0.5), white(0.05))
	set("secondary-foreground", slate900, slate100)
	set("muted", tela.RGBA(0, 0, 0, 0.05), white(0.1))
	set("muted-foreground", slate600, slate400)
	set("accent", white(0.8), white(0.1))
	set("accent-foreground", slate900, slate100)
	set("destructive", rose500, rose500)
	set("border", white(0.6), white(0.1))
	set("input", white(0.7), white(0.1))
	set("ring", indigo500, indigo400)
	return t
}

// glass is the style of the page: the default recipes with the glass look.
var glass = glassStyle()

func glassStyle() kit.Style {
	s := kit.DefaultStyle()

	// Cards are frosted panels. They carry no blur (the glass budget in
	// backdrop.go) and no shadow: only a fill and an edge, from the tokens.
	s.Card.Root = "flex flex-col gap-4 rounded-2xl border border-border bg-card py-6 text-card-foreground"
	s.Card.Header = "grid auto-rows-min grid-rows-[auto_auto] items-start gap-x-4 px-6"
	s.Card.Title = "text-lg leading-snug font-semibold tracking-tight text-foreground"
	s.Card.Description = "mt-1 text-sm text-muted-foreground"

	// Buttons. The default one is the page's gradient; "glass" is a pane
	// of it; "tint" sits on a gradient; "nav" is a sidebar entry and "fab"
	// the round button fixed to the corner. All of them press down.
	b := &s.Button
	b.Root = "inline-flex shrink-0 items-center justify-center gap-2 rounded-xl text-sm font-medium whitespace-nowrap transition duration-200 outline-none active:scale-95 " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
	b.Variants[kit.Default] = "bg-linear-to-r from-indigo-500 to-fuchsia-500 font-semibold text-white shadow-lg shadow-fuchsia-500/30 hover:-translate-y-0.5 hover:shadow-xl hover:shadow-fuchsia-500/40"
	b.Variants[kit.Link] = "text-primary transition-colors hover:text-primary/70"
	b.Variants["glass"] = "border border-border bg-secondary text-foreground hover:bg-accent"
	b.Variants["tint"] = "bg-white/20 text-white hover:bg-white/30"
	b.Variants["nav"] = "text-muted-foreground transition-colors hover:text-foreground"
	b.Variants["fab"] = "bg-linear-to-br from-indigo-500 to-fuchsia-500 text-lg font-semibold text-white shadow-xl shadow-fuchsia-500/30 ring-4 ring-white/40 ease-out hover:scale-110 hover:shadow-2xl dark:ring-white/10"
	b.Sizes[""] = "h-10 px-4"
	b.Sizes[kit.Sm] = "h-8 rounded-lg px-3 text-xs"
	b.Sizes[kit.Icon] = "size-10"
	b.Sizes["nav"] = "h-10 w-full justify-start gap-3 px-3"
	b.Sizes["fab"] = "size-12"
	b.HasIcon[""] = "px-4"
	b.IconSizes["fab"] = "size-6"

	// Badges.
	g := &s.Badge
	g.Variants[kit.Default] = "bg-primary text-primary-foreground"
	g.Variants["live"] = "gap-2 bg-emerald-400/20 px-2.5 text-emerald-700 dark:text-emerald-300"
	g.Variants["syncing"] = "gap-2 bg-amber-400/20 px-2.5 text-amber-700 dark:text-amber-300"
	g.Variants["up"] = "bg-emerald-500/15 font-semibold text-emerald-700 dark:text-emerald-300"
	g.Variants["down"] = "bg-rose-500/15 font-semibold text-rose-700 dark:text-rose-300"
	g.Variants["soft"] = "bg-muted text-muted-foreground"
	g.Variants["tint"] = "bg-white/20 font-semibold text-white"

	// Avatars are circles with a white ring and a gradient of their own
	// (FallbackClass).
	a := &s.Avatar
	a.Root = "relative flex size-9 shrink-0 overflow-hidden rounded-full ring-2 ring-white/70 select-none dark:ring-white/20"
	a.Fallback = "flex size-full items-center justify-center rounded-full bg-linear-to-br from-slate-400 to-slate-500 text-xs font-semibold text-white"
	a.Sizes[kit.Lg] = "size-10"

	// Inputs are panes too.
	s.Input.Root = "h-10 w-full min-w-0 rounded-xl border border-input bg-secondary px-3 text-sm text-foreground outline-none transition duration-200 " +
		"placeholder:text-muted-foreground focus:bg-accent focus-visible:ring-2 focus-visible:ring-ring/50 " +
		"disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive"

	// The switch is a gradient track with a white knob.
	w := &s.Switch
	w.Root = "peer group inline-flex shrink-0 items-center rounded-full border border-transparent bg-slate-300 shadow-inner transition-colors duration-200 outline-none dark:bg-slate-600 " +
		"focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 " +
		"checked:bg-linear-to-r checked:from-indigo-500 checked:to-fuchsia-500 dark:checked:bg-linear-to-r"
	w.Sizes[""] = "h-6 w-11"
	w.Thumb = "pointer-events-none ml-px block rounded-full bg-white shadow transition duration-200 ease-out translate-x-0 group-checked:translate-x-5"
	w.ThumbSizes[""] = "size-5"

	// The checkbox of the task list is a round one with a gradient.
	c := &s.Checkbox
	c.Root = "peer inline-flex size-6 shrink-0 items-center justify-center rounded-full border border-slate-300 bg-white/60 text-transparent outline-none transition duration-200 active:scale-90 " +
		"dark:border-slate-600 dark:bg-transparent hover:border-indigo-400 focus-visible:ring-[3px] focus-visible:ring-ring/50 " +
		"checked:border-transparent checked:bg-linear-to-br checked:from-indigo-500 checked:to-fuchsia-500 checked:text-white dark:checked:bg-linear-to-br"
	c.Icon = "size-3.5"

	// A segmented control: a pane with a pill under the selected item. The
	// pill is a child of the item (segment below), so that it can slide.
	s.ToggleGroup.Root = "flex w-fit items-center rounded-xl border border-border bg-secondary p-1"
	s.ToggleGroup.Item = "relative"
	s.ToggleGroup.ItemWithoutSpacing = ""
	s.Toggle.Root = "inline-flex items-center justify-center rounded-lg text-sm font-medium whitespace-nowrap text-muted-foreground transition-colors duration-200 outline-none " +
		"hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50"
	s.Toggle.Sizes[""] = "h-8 min-w-8 px-3.5"
	s.Toggle.Sizes[kit.Sm] = "h-6 min-w-6 rounded-md px-2.5 text-xs"
	s.Toggle.On = "aria-pressed:text-foreground checked:text-foreground"

	// Tabs with an underline that slides (Tabs.Indicator).
	t := &s.Tabs
	t.List = "inline-flex w-fit items-center justify-center text-muted-foreground"
	t.ListVariants[kit.TabsLine] = "gap-1 border-b border-black/10 dark:border-white/10"
	t.ListOrientations[kit.Horizontal] = ""
	t.Trigger = "group relative inline-flex items-center justify-center gap-1.5 px-3 pb-2.5 text-sm font-medium whitespace-nowrap text-muted-foreground transition-colors duration-200 outline-none " +
		"hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-selected:text-primary"
	t.TriggerVariants[kit.TabsLine] = ""

	// Tables are lists in a card: a hairline under each row, and the row
	// lights up under the pointer.
	tb := &s.Table
	tb.Table = "grid w-full text-sm"
	tb.Head = "flex h-9 items-center border-b border-border px-2 text-left text-xs font-medium tracking-wider whitespace-nowrap text-muted-foreground uppercase"
	tb.HeadSticky = "sticky top-0 z-20" // the glass card shows through: no opaque bg-background
	tb.Cell = "flex items-center border-b border-border px-2 py-2.5 whitespace-nowrap transition-colors duration-200 group-hover:bg-accent/60"

	// The dialog and the toasts are the frosted glass that does blur: the
	// whole page is behind them. The scrim appears at once and only the
	// panel animates (a fade over the window repaints the whole page on
	// every frame of it).
	d := &s.Dialog
	d.Overlay = "absolute inset-0 bg-slate-900/30 dark:bg-black/50"
	d.OverlayExit = ""
	d.Root = "relative grid w-full max-w-md gap-5 rounded-3xl border border-border bg-popover p-6 text-popover-foreground shadow-2xl shadow-indigo-950/20 backdrop-blur-xl outline-none " +
		"transition duration-200 ease-out starting:translate-y-4 starting:scale-95 starting:opacity-0"
	d.Exit = "translate-y-4 scale-95 opacity-0"
	d.Header = "flex flex-col gap-1"
	d.Title = "text-xl font-semibold tracking-tight"
	d.Description = "text-sm text-muted-foreground"
	d.Footer = "flex justify-end gap-3 pt-1"

	to := &s.Toast
	to.Positions[kit.ToastBottomCenter] = "bottom-8 left-1/2 -translate-x-1/2"
	to.Item = "absolute left-0 box-border flex w-full items-center gap-1.5 rounded-2xl border border-border bg-popover py-4 pr-12 pl-4 text-sm font-medium text-popover-foreground " +
		"shadow-2xl shadow-indigo-950/20 backdrop-blur-xl transition-[transform,opacity,height,box-shadow] duration-[400ms] outline-none"
	to.Icon = "relative -ml-[3px] mr-1 flex size-5 shrink-0 items-center justify-start text-emerald-500"
	to.IconSvg = "size-5 shrink-0"
	to.Close = "absolute top-1/2 right-3 flex size-7 -translate-y-1/2 cursor-pointer items-center justify-center rounded-lg p-0 text-muted-foreground transition-colors duration-200 outline-none " +
		"hover:bg-black/5 focus-visible:ring-2 focus-visible:ring-ring/50 dark:hover:bg-white/10"
	to.CloseIcon = "size-4"
	return s
}
