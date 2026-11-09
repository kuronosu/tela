package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The demo window of the Title Bar and Menubar pages. The gallery's own
// window is framed, so it has no resize handles of its own to replace and
// no bar that is the window's: what h.WindowResize and an AppMenu bar do
// has to be tried in a window the app draws itself. The pages' button opens
// one (a tela.Window, ADR-183): frameless and transparent, a card with a
// shadow over the desktop (ADR-249), resizable at its edges and corners
// (h.WindowResize, ADR-257), with a kit.TitleBar and a drawn kit.Menubar
// that is the window's bar (AppMenu: F10 and a lone Alt reach it, ADR-255).

// DemoWindow is a button that opens the demo window while it is on, and the
// window. It is a component of its own so each page keeps its state.
type DemoWindow struct{}

func (DemoWindow) Build() tela.Node {
	tr := useTr()
	open, setOpen := tela.UseState(false)
	return Fragment(
		kit.Button{Variant: kit.Outline, Icon: icons.LayoutGrid, OnClick: func() { setOpen.Set(true) },
			Children: Text(tr("playground.open", "Open a frameless window"))},
		If(open, tela.Component(tela.Window{
			Title: tr("playground.title", "Frameless window"),
			Width: 560, Height: 420, MinWidth: 380, MinHeight: 300,
			Frameless: true, Transparent: true,
			OnClose: func() { setOpen.Set(false) },
			Root:    demoCard{},
		})),
	)
}

// demoCard is the content of the demo window: a card inside a margin that
// is transparent where the desktop can show it, so the card's shadow fits.
type demoCard struct{}

func (demoCard) Build() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	win := tela.UseWindow()
	last, setLast := tela.UseState("")
	showSize, setShowSize := tela.UseState(true)
	card := tela.UseElementRef()
	size := tela.UseSize(card).Get()

	// Where the OS cannot show a transparent window (X11 without a
	// compositing manager) the margin is opaque: give it a background.
	margin := "h-full p-6"
	if !win.State().Transparent {
		margin += " bg-background"
	}
	return Shell{Class: margin, Children: Div(Ref(card), WindowResize(),
		Class("flex h-full min-h-0 w-full flex-col overflow-hidden rounded-xl border border-border bg-card text-card-foreground shadow-2xl"),
		kit.TitleBar{
			Variant: kit.Plain, Class: "border-b border-border bg-muted/40",
			Leading: ic(icons.LayoutGrid, "size-4 text-primary"),
			Title:   Text(tr("playground.title", "Frameless window")),
		},
		Div(Class("border-b border-border px-2 py-1"),
			kit.Menubar[string]{
				AppMenu: true, Native: kit.NativeOff, Class: "border-0 bg-transparent shadow-none",
				AriaLabel: tr("playground.bar", "Window menu"),
				OnSelect: func(menu, item string) {
					if item == "close" {
						win.Close()
						return
					}
					setLast.Set(menu + " > " + item)
				},
				Menus: []kit.MenubarMenu[string]{
					{Key: "file", Label: tr("menubar.file", "File"), Items: []kit.MenuItem[string]{
						{Key: "new", Label: tr("menubar.newTab", "New Tab"), Shortcut: "Cmd+T"},
						{Key: "print", Label: tr("menubar.print", "Print..."), Shortcut: "Cmd+P"},
						{Kind: kit.MenuSeparator},
						{Key: "close", Label: tr("playground.close", "Close window"), Shortcut: "Cmd+W"},
					}},
					{Key: "view", Label: tr("menubar.view", "View"), Items: []kit.MenuItem[string]{
						{Kind: kit.MenuCheckbox, Key: "size", Label: tr("playground.showSize", "Show the size"), Checked: &showSize, OnCheckedChange: setShowSize.Set},
						{Key: "reload", Label: tr("menubar.reload", "Reload"), Shortcut: "Cmd+R"},
					}},
					{Key: "help", Label: tr("playground.help", "Help"), Items: []kit.MenuItem[string]{
						{Key: "about", Label: tr("playground.about", "About")},
					}},
				},
			}),
		Div(Class("flex min-h-0 flex-1 flex-col justify-center gap-3 p-6 text-sm"),
			P(Text(tr("playground.keys", "Press F10 or Alt: the menu bar takes the focus. F10 or Escape gives it back."))),
			P(Class("text-muted-foreground"), Text(tr("playground.edges", "Drag the title bar to move the window, and its edges or corners to resize it."))),
			If(showSize, P(Class("font-medium tabular-nums"), TestID("playground-size"),
				Text(tr("playground.size", "Size: {width} × {height}", "width", f.Number(float64(size.Width)), "height", f.Number(float64(size.Height)))))),
			chose(tr, last, tr("playground.pick", "Choose something from the menus.")),
		),
	)}
}
