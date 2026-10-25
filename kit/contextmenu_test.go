package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// browserItems is shadcn's context menu demo, without its submenu.
func browserItems() []kit.MenuItem[string] {
	return []kit.MenuItem[string]{
		{Key: "back", Label: "Back", Inset: true, Shortcut: "⌘["},
		{Key: "forward", Label: "Forward", Inset: true, Disabled: true, Shortcut: "⌘]"},
		{Key: "reload", Label: "Reload", Inset: true, Shortcut: "⌘R"},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuCheckbox, Key: "bookmarks", Label: "Show Bookmarks", DefaultChecked: true},
		{Kind: kit.MenuCheckbox, Key: "urls", Label: "Show Full URLs"},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuLabel, Label: "People", Inset: true},
		{Kind: kit.MenuRadioGroup, Key: "person", DefaultValue: "pedro", Items: []kit.MenuItem[string]{
			{Key: "pedro", Label: "Pedro Duarte"}, {Key: "colm", Label: "Colm Tuite"}}},
		{Kind: kit.MenuSeparator},
		{Key: "delete", Label: "Delete", Variant: kit.Destructive, Icon: iconCircleAlert},
	}
}

func contextSheet() *sheet {
	cm := func(name string, items []kit.MenuItem[string], class string) cell {
		return cell{
			name: name, width: 300, probes: []string{name + "-panel"},
			tela: kit.StaticContextMenu(kit.ContextMenu[string]{Items: items, ContentClass: class}, h.TestID(name+"-panel")),
			html: menuHTML(contextFlavor, items, class, name+"-panel"),
		}
	}
	return &sheet{name: "contextmenu", width: 760, cells: []cell{
		cm("browser", browserItems(), ""),
		cm("icons", iconItems(), ""),
		cm("class", []kit.MenuItem[string]{{Key: "a", Label: "Cut"}, {Key: "b", Label: "Copy"}}, "w-56"),
	}}
}

func TestContextMenuGolden(t *testing.T)   { contextSheet().golden(t) }
func TestChromiumContextMenu(t *testing.T) { contextSheet().chromium(t) }

var cmMenu = telatest.ByRole("menu", "Actions")

func cmItem(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

// contextPage is an area at (20, 20), 240x160, with a button inside, an
// outside button, and the context menu on the area.
func contextPage(m kit.ContextMenu[string]) func() tela.Node {
	if m.Items == nil {
		m.Items = []kit.MenuItem[string]{
			{Key: "copy", Label: "Copy"}, {Key: "paste", Label: "Paste", Disabled: true}, {Key: "delete", Label: "Delete", Variant: kit.Destructive},
		}
	}
	if m.AriaLabel == "" {
		m.AriaLabel = "Actions"
	}
	m.Class = "absolute top-5 left-5 h-40 w-60 rounded-md border border-dashed " + m.Class
	m.Children = h.Button(h.Class("m-4 h-8 w-20"), h.Text("Inner"))
	return func() tela.Node {
		return h.Div(h.Class("relative h-full w-full"), m,
			h.Button(h.Class("absolute top-5 left-[320px] h-8 w-20"), h.Text("Outside")))
	}
}

func TestContextMenuRightClick(t *testing.T) {
	var log []string
	app := mountMenuRaw(t, contextPage(kit.ContextMenu[string]{
		OnSelect: func(k string) { log = append(log, k) },
	}), 480, 320)
	if strings.Contains(app.snapshot(), "menu") {
		t.Fatalf("the menu is open at the start:\n%s", app.snapshot())
	}
	app.rightClick(100, 80)
	snap := app.snapshot()
	for _, want := range []string{`menu "Actions"`, `menuitem "Copy"`, `menuitem "Paste" [disabled]`, `menuitem "Delete"`} {
		if !strings.Contains(snap, want) {
			t.Errorf("the snapshot lacks %q:\n%s", want, snap)
		}
	}
	// A second right click on the area moves the menu; a press on an item
	// chooses it, closes the menu and calls OnSelect.
	app.rightClick(60, 50)
	app.leftClick(70, 60) // on the first item: the menu is at (60, 50) with 4 px of padding
	if got := fmt.Sprint(log); got != "[copy]" {
		t.Errorf("chosen %s, want [copy]", got)
	}
	if strings.Contains(app.snapshot(), "menu") {
		t.Errorf("the menu is still open after choosing an item:\n%s", app.snapshot())
	}

	// A press outside closes it without choosing.
	app.rightClick(100, 80)
	app.leftClick(400, 280)
	if strings.Contains(app.snapshot(), "menu") {
		t.Errorf("the menu is still open after a press outside:\n%s", app.snapshot())
	}
	if len(log) != 1 {
		t.Errorf("a press outside chose: %v", log)
	}
}

func TestContextMenuOpensAtThePointer(t *testing.T) {
	app := mountMenuRaw(t, contextPage(kit.ContextMenu[string]{}), 480, 320)
	app.rightClick(100, 80)
	n := app.find("menu", "Actions")
	if n == nil {
		t.Fatalf("no menu:\n%s", app.snapshot())
	}
	if n.Bounds.X0 != 100 || n.Bounds.Y0 != 80 {
		t.Errorf("menu at (%v, %v), want the pointer's (100, 80)", n.Bounds.X0, n.Bounds.Y0)
	}
	// Right clicking elsewhere on the area moves it.
	app.rightClick(40, 40)
	if n = app.find("menu", "Actions"); n == nil || n.Bounds.X0 != 40 || n.Bounds.Y0 != 40 {
		t.Errorf("after a second right click the menu is %+v, want it at (40, 40)", n)
	}
	// Near the window's edge it flips and shifts to stay in the window.
	app.leftClick(400, 300)
	app.rightClick(230, 170)
	n = app.find("menu", "Actions")
	if n == nil || n.Bounds.X1 > 480 || n.Bounds.Y1 > 320 {
		t.Errorf("menu %+v is out of the 480x320 window", n)
	}
}

func TestContextMenuKeyboard(t *testing.T) {
	var log []string
	page := contextPage(kit.ContextMenu[string]{OnSelect: func(k string) { log = append(log, k) }})
	app := telatest.MountFunc(t, page, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	inner := telatest.ByRole("button", "Inner")
	app.Focus(inner)
	app.Press("Shift+F10")
	app.ExpectCount(cmMenu, 1)
	// The menu opens at the area's corner, on its first item.
	app.ExpectFocused(cmItem("Copy"))
	if m := app.Bounds(cmMenu); !near(m.X, 20) || !near(m.Y, 20) {
		t.Errorf("menu at %v, want the area's corner (20, 20)", m)
	}
	app.Press("ArrowDown") // the disabled item is skipped
	app.ExpectFocused(cmItem("Delete"))
	app.Press("Escape")
	app.ExpectCount(cmMenu, 0)
	app.ExpectFocused(inner)

	// The ContextMenu key too; Enter chooses.
	app.Press("ContextMenu")
	app.ExpectCount(cmMenu, 1)
	app.Press("Enter")
	app.ExpectCount(cmMenu, 0)
	app.ExpectFocused(inner)
	if got := fmt.Sprint(log); got != "[copy]" {
		t.Errorf("chosen %s, want [copy]", got)
	}
}

func TestContextMenuItemKinds(t *testing.T) {
	var log []string
	page := contextPage(kit.ContextMenu[string]{Items: browserItems(), OnSelect: func(k string) { log = append(log, k) }})
	app := telatest.MountFunc(t, page, telatest.Size(480, 480), telatest.Theme(kit.Theme()))
	open := func() {
		app.Focus(telatest.ByRole("button", "Inner"))
		app.Press("Shift+F10")
		app.ExpectCount(cmMenu, 1)
	}
	open()
	app.ExpectAria(`
        - button "Inner"
        - button "Outside"
        - menu "Actions":
          - menuitem "Back"
          - menuitem "Forward" [disabled]
          - menuitem "Reload"
          - separator
          - menuitemcheckbox "Show Bookmarks" [checked]
          - menuitemcheckbox "Show Full URLs"
          - separator
          - text: People
          - group:
            - menuitemradio "Pedro Duarte" [checked]
            - menuitemradio "Colm Tuite"
          - separator
          - menuitem "Delete"
    `)
	app.Click(telatest.ByRole("menuitemcheckbox", "Show Full URLs"))
	app.ExpectCount(cmMenu, 0)
	open()
	app.Expect(telatest.ByRole("menuitemcheckbox", "Show Full URLs"), telatest.Checked(true))
	app.Click(telatest.ByRole("menuitemradio", "Colm Tuite"))
	open()
	app.Expect(telatest.ByRole("menuitemradio", "Colm Tuite"), telatest.Checked(true))
	app.Expect(telatest.ByRole("menuitemradio", "Pedro Duarte"), telatest.Checked(false))
	app.Click(cmItem("Delete"))
	if got := fmt.Sprint(log); got != "[delete]" {
		t.Errorf("OnSelect got %s: only plain items report it, want [delete]", got)
	}
}

func TestContextMenuDisabled(t *testing.T) {
	app := mountMenuRaw(t, contextPage(kit.ContextMenu[string]{Disabled: true}), 480, 320)
	app.rightClick(100, 80)
	if strings.Contains(app.snapshot(), `menu "Actions"`) {
		t.Errorf("a disabled area opened its menu:\n%s", app.snapshot())
	}
}

func TestContextMenuControlledOpen(t *testing.T) {
	var log []bool
	open := false
	app := mountMenuRaw(t, contextPage(kit.ContextMenu[string]{Open: &open, OnOpenChange: func(o bool) { log = append(log, o) }}), 480, 320)
	app.rightClick(100, 80)
	if strings.Contains(app.snapshot(), `menu "Actions"`) {
		t.Errorf("the parent did not open it, but it is open")
	}
	if fmt.Sprint(log) != "[true]" {
		t.Errorf("OnOpenChange got %v, want [true]", log)
	}
}

func TestContextMenuInnermostWins(t *testing.T) {
	var log []string
	inner := kit.ContextMenu[string]{
		AriaLabel: "Inner actions", Class: "m-4 h-24 w-40 border",
		Items: []kit.MenuItem[string]{{Key: "inner", Label: "Inner item", OnSelect: func() { log = append(log, "inner") }}},
	}
	outer := kit.ContextMenu[string]{
		AriaLabel: "Outer actions", Class: "absolute top-5 left-5 h-40 w-60 border", Children: inner,
		Items: []kit.MenuItem[string]{{Key: "outer", Label: "Outer item"}},
	}
	app := mountMenuRaw(t, func() tela.Node { return h.Div(h.Class("relative h-full w-full"), outer) }, 480, 320)
	app.rightClick(60, 60) // inside the inner area
	snap := app.snapshot()
	if !strings.Contains(snap, `menu "Inner actions"`) || strings.Contains(snap, "Outer actions") {
		t.Errorf("a right click in the inner area must open its menu only:\n%s", snap)
	}
}

func TestContextMenuStyleProvider(t *testing.T) {
	s := kit.DefaultStyle()
	s.ContextMenu.Content += " ring-2"
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: tela.C(contextPage(kit.ContextMenu[string]{}))})
	}, telatest.Size(480, 320), telatest.Theme(kit.Theme()))
	app.Focus(telatest.ByRole("button", "Inner"))
	app.Press("Shift+F10")
	if n := app.Count(telatest.ByClass("ring-2")); n != 1 {
		t.Errorf("the menu has the provider's classes %d times, want 1", n)
	}
}

func TestContextMenuOpenGolden(t *testing.T) {
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.ContextMenu[string]{
			AriaLabel: "Actions", Items: browserItems(), Class: "flex h-80 w-96 items-start justify-end rounded-md border border-dashed p-4 text-sm",
			Children: h.Button(h.Class("h-8"), h.Text("Right click here")),
		})
	}
	openGolden(t, "contextmenu", 520, 520, page, func(a *telatest.App) {
		a.Focus(telatest.ByRole("button", "Right click here"))
		a.Press("Shift+F10")
		a.Press("ArrowDown")
		a.Press("ArrowDown")
	})
}

func TestContextMenuIdle(t *testing.T) {
	expectIdle(t, contextPage(kit.ContextMenu[string]{}), func(a *telatest.App) {
		a.Focus(telatest.ByRole("button", "Inner"))
		a.Press("Shift+F10")
	})
}
