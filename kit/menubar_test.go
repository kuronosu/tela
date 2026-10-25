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

// shadcn/ui's Menubar and MenubarTrigger (new-york-v4/ui/menubar.tsx at
// kit.ShadcnCommit), verbatim.
const (
	shadcnMenubar        = "flex h-9 items-center gap-1 rounded-md border bg-background p-1 shadow-xs"
	shadcnMenubarTrigger = "flex items-center rounded-sm px-2 py-1 text-sm font-medium outline-hidden select-none focus:bg-accent focus:text-accent-foreground data-[state=open]:bg-accent data-[state=open]:text-accent-foreground"
)

// fileMenu, editMenu and viewMenu are shadcn's menubar demo.
func fileMenu() kit.MenubarMenu[string] {
	return kit.MenubarMenu[string]{Key: "file", Label: "File", Items: []kit.MenuItem[string]{
		{Key: "new-tab", Label: "New Tab", Shortcut: "⌘T"},
		{Key: "new-window", Label: "New Window", Shortcut: "⌘N"},
		{Key: "incognito", Label: "New Incognito Window", Disabled: true},
		{Kind: kit.MenuSeparator},
		{Key: "print", Label: "Print...", Shortcut: "⌘P"},
	}}
}

func editMenu() kit.MenubarMenu[string] {
	return kit.MenubarMenu[string]{Key: "edit", Label: "Edit", Items: []kit.MenuItem[string]{
		{Key: "undo", Label: "Undo", Shortcut: "⌘Z"},
		{Key: "redo", Label: "Redo", Shortcut: "⇧⌘Z"},
		{Kind: kit.MenuSeparator},
		{Key: "cut", Label: "Cut"},
		{Key: "copy", Label: "Copy"},
		{Key: "paste", Label: "Paste"},
	}}
}

func viewMenu() kit.MenubarMenu[string] {
	return kit.MenubarMenu[string]{Key: "view", Label: "View", Items: []kit.MenuItem[string]{
		{Kind: kit.MenuCheckbox, Key: "bookmarks", Label: "Always Show Bookmarks Bar"},
		{Kind: kit.MenuCheckbox, Key: "urls", Label: "Always Show Full URLs", DefaultChecked: true},
		{Kind: kit.MenuSeparator},
		{Key: "reload", Label: "Reload", Shortcut: "⌘R", Inset: true},
		{Key: "force", Label: "Force Reload", Shortcut: "⇧⌘R", Inset: true, Disabled: true},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuLabel, Label: "Profile", Inset: true},
		{Kind: kit.MenuRadioGroup, Key: "profile", DefaultValue: new("benoit"), Items: []kit.MenuItem[string]{
			{Key: "andy", Label: "Andy"}, {Key: "benoit", Label: "Benoit"}, {Key: "luis", Label: "Luis"}}},
	}}
}

func appBar(edit func(*kit.Menubar[string])) kit.Menubar[string] {
	m := kit.Menubar[string]{AriaLabel: "Application", Menus: []kit.MenubarMenu[string]{fileMenu(), editMenu(), viewMenu()}}
	if edit != nil {
		edit(&m)
	}
	return m
}

// barHTML is the markup of shadcn's Menubar with the given triggers.
func barHTML(probe string, labels ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, `<div role="menubar" data-probe=%q data-slot="menubar" class=%q>`, probe, shadcnMenubar)
	for _, l := range labels {
		fmt.Fprintf(&sb, `<button type="button" role="menuitem" aria-haspopup="menu" aria-expanded="false" data-state="closed" data-slot="menubar-trigger" class=%q>%s</button>`, shadcnMenubarTrigger, l)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

func menubarSheet() *sheet {
	bar := func(name string, m kit.Menubar[string], labels ...string) cell {
		m.Props = h.TestID(name + "-bar")
		return cell{name: name, probes: []string{name + "-bar"}, tela: m, html: barHTML(name+"-bar", labels...)}
	}
	panel := func(name string, m kit.MenubarMenu[string]) cell {
		mb := appBar(nil)
		return cell{
			name: name, width: 300, probes: []string{name + "-panel"},
			tela: kit.StaticMenubarMenu(mb, m.Key, h.TestID(name+"-panel")),
			html: menuHTML(menubarFlavor, m.Items, "", name+"-panel"),
		}
	}
	return &sheet{name: "menubar", width: 760, cells: []cell{
		bar("bar", appBar(nil), "File", "Edit", "View"),
		bar("single", kit.Menubar[string]{Menus: []kit.MenubarMenu[string]{fileMenu()}}, "File"),
		panel("file", fileMenu()),
		panel("edit", editMenu()),
		panel("view", viewMenu()),
	}}
}

func TestMenubarGolden(t *testing.T)   { menubarSheet().golden(t) }
func TestChromiumMenubar(t *testing.T) { menubarSheet().chromium(t) }

func barItem(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

// menuNamed is the open menu of the trigger with the name.
func menuNamed(name string) telatest.Locator { return telatest.ByRole("menu", name) }

func mountBar(t *testing.T, m kit.Menubar[string], opts ...telatest.Option) *telatest.App {
	t.Helper()
	page := func() tela.Node {
		return h.Div(h.Class("flex flex-col items-start gap-4 p-6"), h.Button(h.Class("h-9"), h.Text("Before")), m,
			h.Button(h.Class("h-9"), h.Text("After")))
	}
	return telatest.MountFunc(t, page, append([]telatest.Option{telatest.Theme(kit.Theme())}, opts...)...)
}

func TestMenubarOpensAndCloses(t *testing.T) {
	app := mountBar(t, appBar(nil))
	app.ExpectAria(`
        - button "Before"
        - menubar "Application":
          - menuitem "File"
          - menuitem "Edit"
          - menuitem "View"
        - button "After"
    `)
	app.Click(barItem("File"))
	app.Expect(barItem("File"), telatest.Expanded(true), telatest.HasPopup("menu"))
	app.ExpectCount(menuNamed("File"), 1)
	// Not modal: the page stays in the accessibility tree.
	app.ExpectCount(telatest.ByRole("button", "After"), 1)
	app.Press("Escape")
	app.ExpectCount(menuNamed("File"), 0)
	app.ExpectFocused(barItem("File"))
	app.Expect(barItem("File"), telatest.Expanded(false))

	// A press on the open menu's trigger closes it.
	app.Click(barItem("File"))
	app.ExpectCount(menuNamed("File"), 1)
	app.Click(barItem("File"))
	app.ExpectCount(menuNamed("File"), 0)

	// A press outside closes it.
	app.Click(barItem("Edit"))
	app.ExpectCount(menuNamed("Edit"), 1)
	app.Click(telatest.ByRole("button", "After"))
	app.ExpectCount(menuNamed("Edit"), 0)
}

func TestMenubarKeyboard(t *testing.T) {
	var log []string
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) {
		m.OnSelect = func(menu, item string) { log = append(log, menu+"/"+item) }
		m.OnOpenChange = func(menu string, open bool) { log = append(log, fmt.Sprintf("%s:%t", menu, open)) }
	}))
	app.Focus(barItem("File"))
	app.Press("ArrowRight")
	app.ExpectFocused(barItem("Edit"))
	app.Press("End")
	app.ExpectFocused(barItem("View"))
	app.Press("ArrowRight") // wraps
	app.ExpectFocused(barItem("File"))
	app.Press("ArrowLeft")
	app.ExpectFocused(barItem("View"))
	app.Press("Home")
	app.ExpectFocused(barItem("File"))

	// ArrowDown opens on the first enabled item, ArrowUp on the last.
	app.Press("ArrowDown")
	app.ExpectFocused(telatest.ByRole("menuitem", "New Tab"))
	app.Press("ArrowDown")
	app.Press("ArrowDown") // New Incognito Window is disabled
	app.ExpectFocused(telatest.ByRole("menuitem", "Print..."))
	// ArrowRight in the menu opens the neighbor's menu on its first item.
	app.Press("ArrowRight")
	app.ExpectCount(menuNamed("File"), 0)
	app.ExpectCount(menuNamed("Edit"), 1)
	app.ExpectFocused(telatest.ByRole("menuitem", "Undo"))
	app.Press("ArrowLeft")
	app.ExpectCount(menuNamed("File"), 1)
	app.Press("Escape")
	app.ExpectFocused(barItem("File"))

	app.Press("ArrowUp")
	app.ExpectFocused(telatest.ByRole("menuitem", "Print..."))
	app.Press("Enter")
	app.ExpectCount(menuNamed("File"), 0)
	app.ExpectFocused(barItem("File"))
	// Moving from a menu to another is one call; choosing an item is
	// reported before the menu closes.
	if got := fmt.Sprint(log); got != "[file:true edit:true file:true file:false file:true file/print file:false]" {
		t.Errorf("events %s", got)
	}
}

func TestMenubarHoverSwitches(t *testing.T) {
	app := mountBar(t, appBar(nil))
	// With no menu open, hovering opens nothing.
	app.Hover(barItem("Edit"))
	app.ExpectCount(menuNamed("Edit"), 0)
	app.Click(barItem("File"))
	app.ExpectCount(menuNamed("File"), 1)
	app.Hover(barItem("Edit"))
	app.ExpectCount(menuNamed("Edit"), 1)
	app.ExpectCount(menuNamed("File"), 0)
	app.Hover(barItem("View"))
	app.ExpectCount(menuNamed("View"), 1)
	app.ExpectCount(menuNamed("Edit"), 0)
}

func TestMenubarTabLeaves(t *testing.T) {
	app := mountBar(t, appBar(nil))
	app.Focus(barItem("File"))
	app.Press("Enter")
	app.ExpectCount(menuNamed("File"), 1)
	app.Press("Tab")
	app.ExpectCount(menuNamed("File"), 0)
	app.ExpectFocused(telatest.ByRole("button", "After"))
}

func TestMenubarChoosingAnItem(t *testing.T) {
	var log []string
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) {
		m.OnSelect = func(menu, item string) { log = append(log, menu+"/"+item) }
		m.Menus[1].Items[0].OnSelect = func() { log = append(log, "undo") }
	}))
	app.Click(barItem("Edit"))
	// A mouse leaves the focus to the menu; hovering focuses an item.
	app.Hover(telatest.ByRole("menuitem", "Redo"))
	app.ExpectFocused(telatest.ByRole("menuitem", "Redo"))
	app.Click(telatest.ByRole("menuitem", "Undo"))
	app.ExpectCount(menuNamed("Edit"), 0)
	app.ExpectFocused(barItem("Edit"))
	if got := fmt.Sprint(log); got != "[undo edit/undo]" {
		t.Errorf("chosen %s, want [undo edit/undo]", got)
	}
}

func TestMenubarCheckboxAndRadioState(t *testing.T) {
	app := mountBar(t, appBar(nil))
	app.Click(barItem("View"))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Always Show Bookmarks Bar"), telatest.Checked(false))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Always Show Full URLs"), telatest.Checked(true))
	app.Expect(telatest.ByRole("menuitemradio", "Benoit"), telatest.Checked(true))
	app.Click(telatest.ByRole("menuitemcheckbox", "Always Show Bookmarks Bar"))
	app.ExpectCount(menuNamed("View"), 0)
	app.Click(barItem("View"))
	app.Click(telatest.ByRole("menuitemradio", "Luis"))
	// The state survives: the menus are rebuilt when they open.
	app.Click(barItem("Edit"))
	app.Hover(barItem("View"))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Always Show Bookmarks Bar"), telatest.Checked(true))
	app.Expect(telatest.ByRole("menuitemradio", "Luis"), telatest.Checked(true))
	app.Expect(telatest.ByRole("menuitemradio", "Benoit"), telatest.Checked(false))
}

func TestMenubarDisabledMenu(t *testing.T) {
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) { m.Menus[1].Disabled = true }))
	app.Expect(barItem("Edit"), telatest.Disabled(true))
	app.Focus(barItem("File"))
	app.Press("ArrowRight")
	app.ExpectFocused(barItem("View"))
}

func TestMenubarControlledOpen(t *testing.T) {
	var log []string
	open := "edit"
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) {
		m.Open = &open
		m.OnOpenChange = func(menu string, o bool) { log = append(log, fmt.Sprintf("%s:%t", menu, o)) }
	}))
	app.ExpectCount(menuNamed("Edit"), 1)
	app.Press("Escape")
	app.ExpectCount(menuNamed("Edit"), 1) // the parent did not close it
	if got := fmt.Sprint(log); got != "[edit:false]" {
		t.Errorf("OnOpenChange got %v, want [edit:false]", got)
	}
}

func TestMenubarDefaultOpen(t *testing.T) {
	file := "file"
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) { m.DefaultOpen = &file }))
	app.ExpectCount(menuNamed("File"), 1)
}

func TestMenubarMenuPlacement(t *testing.T) {
	app := mountBar(t, appBar(nil), telatest.Size(600, 400))
	tr := app.Bounds(barItem("Edit"))
	app.Click(barItem("Edit"))
	m := app.Bounds(menuNamed("Edit"))
	// shadcn: sideOffset 8, alignOffset -4, align start.
	if !near(m.Y, tr.Y+tr.Height+8) || !near(m.X, tr.X-4) {
		t.Errorf("trigger %v menu %v: want it 8 px below and 4 px before the trigger's start", tr, m)
	}
	if m.Width < 192 {
		t.Errorf("menu is %v px wide, want at least min-w-48 (192)", m.Width)
	}
}

func TestMenubarDuplicateItemKeyPanics(t *testing.T) {
	file := "file"
	app := appBar(func(m *kit.Menubar[string]) {
		m.DefaultOpen = &file
		m.Menus[0].Items = []kit.MenuItem[string]{{Key: "a", Label: "One"}, {Key: "a", Label: "Two"}}
	})
	_, err := telatest.AriaSnapshot(tela.C(func() tela.Node { return h.Div(app) }))
	if err == nil || !strings.Contains(err.Error(), "Menubar has two items with the key a") {
		t.Fatalf("error %v, want the duplicate key reported", err)
	}
}

func TestMenubarStyleProvider(t *testing.T) {
	s := kit.DefaultStyle()
	s.Menubar.Root += " ring-4"
	s.Menubar.Menu.Content += " ring-2"
	file := "file"
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: appBar(func(m *kit.Menubar[string]) { m.DefaultOpen = &file })})
	}, telatest.Theme(kit.Theme()))
	if n := app.Count(telatest.ByClass("ring-4")); n != 1 {
		t.Errorf("the bar has the provider's classes %d times, want 1", n)
	}
	if n := app.Count(telatest.ByClass("ring-2")); n != 1 {
		t.Errorf("the menu has the provider's classes %d times, want 1", n)
	}
}

func TestMenubarOpenGolden(t *testing.T) {
	page := func() tela.Node {
		return h.Div(h.Class("flex flex-col items-start gap-4 p-6"), appBar(func(m *kit.Menubar[string]) {
			open := "view"
			m.DefaultOpen = &open
		}))
	}
	openGolden(t, "menubar", 520, 420, page, func(a *telatest.App) {
		a.Press("ArrowDown")
	})
}

func TestMenubarIdle(t *testing.T) {
	file := "file"
	expectIdle(t, func() tela.Node {
		return h.Div(h.Class("p-6"), appBar(func(m *kit.Menubar[string]) { m.DefaultOpen = &file }))
	}, nil)
}
