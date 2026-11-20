package devtools

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// Theme is the look of the DevTools: kit's theme with a graphite palette, a
// calm one that is dense and readable in the dark and has the same shape
// in the light. No gradients, no glass: flat surfaces told apart by a
// step of lightness and a hairline.
func Theme() tela.Theme {
	t := kit.Theme()
	t.Radius = 8
	set := func(name, light, dark string) {
		t.Colors[name] = tela.SchemeColor{Light: tela.Hex(light), Dark: tela.Hex(dark)}
	}
	// background is the work area, card the bars and side panes around it,
	// muted the wells inside them, accent the hover and selection.
	set("background", "#fbfbfc", "#14161a")
	set("foreground", "#1c2026", "#d6dae0")
	set("card", "#f2f4f7", "#1a1d22")
	set("card-foreground", "#1c2026", "#d6dae0")
	set("popover", "#ffffff", "#1f2227")
	set("popover-foreground", "#1c2026", "#d6dae0")
	set("muted", "#eaedf1", "#21252b")
	set("muted-foreground", "#667085", "#8a919e")
	set("accent", "#e1e6ed", "#2a2f37")
	set("accent-foreground", "#1c2026", "#e6e9ee")
	set("secondary", "#e6e9ee", "#262a31")
	set("secondary-foreground", "#1c2026", "#d6dae0")
	set("primary", "#2563c9", "#6aa9ff")
	set("primary-foreground", "#ffffff", "#0a1220")
	set("destructive", "#cf3a35", "#f0625d")
	set("border", "#d8dce3", "#2a2e35")
	set("input", "#c8ced8", "#363b44")
	set("ring", "#7da4de", "#4d77b3")
	return t
}

// KitStyle is kit's recipes made denser: the DevTools show a lot at once.
func KitStyle() kit.Style {
	s := kit.DefaultStyle()
	// A tab strip as a bar: flat tabs, an underline in the accent color.
	s.Tabs.Root = "flex"
	s.Tabs.List = "inline-flex w-full items-end gap-1 border-b border-border bg-card px-3 text-muted-foreground"
	s.Tabs.ListVariants[kit.TabsLine] = ""
	s.Tabs.ListOrientations[kit.Horizontal] = "h-9"
	s.Tabs.Trigger = "group relative inline-flex h-9 items-center gap-1.5 px-2.5 text-[13px] font-medium whitespace-nowrap text-muted-foreground " +
		"transition-colors outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60 aria-selected:text-foreground"
	s.Tabs.TriggerVariants[kit.TabsLine] = ""
	s.Tabs.Indicator = "absolute inset-x-1.5 -bottom-px h-0.5 rounded-full bg-primary opacity-0 group-aria-selected:opacity-100"
	s.Tabs.IndicatorOrientations[kit.Horizontal] = ""
	s.Tabs.Content = "min-h-0 flex-1 outline-none"
	return s
}
