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

func dropdownSheet() *sheet {
	d := func(items []kit.MenuItem[string], class string) kit.DropdownMenu[string] {
		return kit.DropdownMenu[string]{Items: items, Class: class}
	}
	cellOf := func(name string, items []kit.MenuItem[string], class string) cell {
		dd := d(items, class)
		dd.Props = h.TestID(name + "-panel")
		return menuCell(name, dropdownFlavor, kit.StaticDropdownMenu(dd), items, class)
	}
	return &sheet{name: "dropdownmenu", width: 760, cells: []cell{
		cellOf("account", accountItems(), ""),
		cellOf("kinds", kindsItems(), ""),
		cellOf("icons", iconItems(), ""),
		cellOf("narrow", []kit.MenuItem[string]{{Key: "a", Label: "Cut"}, {Key: "b", Label: "Copy"}}, ""),
		cellOf("class", []kit.MenuItem[string]{{Key: "a", Label: "Cut"}, {Key: "b", Label: "Copy"}}, "w-56 shadow-lg"),
	}}
}

func TestDropdownMenuGolden(t *testing.T)   { dropdownSheet().golden(t) }
func TestChromiumDropdownMenu(t *testing.T) { dropdownSheet().chromium(t) }

var (
	ddTrigger = telatest.ByRole("button", "Actions")
	ddMenu    = telatest.ByRole("menu", "Actions")
)

func ddItem(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

// actionsMenu is a dropdown menu of the behavior tests; log records what
// was chosen.
func actionsMenu(log *[]string, edit func(*kit.DropdownMenu[string])) kit.DropdownMenu[string] {
	rec := func(s string) func() { return func() { *log = append(*log, s) } }
	d := kit.DropdownMenu[string]{
		Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Actions")},
		Items: []kit.MenuItem[string]{
			{Key: "new", Label: "New file", Shortcut: "⌘N", OnSelect: rec("new")},
			{Key: "open", Label: "Open", OnSelect: rec("open")},
			{Key: "export", Label: "Export", Disabled: true, OnSelect: rec("export")},
			{Kind: kit.MenuSeparator},
			{Key: "delete", Label: "Delete", Variant: kit.Destructive, OnSelect: rec("delete")},
		},
		OnSelect: func(k string) { *log = append(*log, "menu:"+k) },
	}
	if edit != nil {
		edit(&d)
	}
	return d
}

func mountDropdown(t *testing.T, d kit.DropdownMenu[string], opts ...telatest.Option) *telatest.App {
	t.Helper()
	return telatest.MountFunc(t, pageUnder(d), append([]telatest.Option{telatest.Theme(kit.Theme())}, opts...)...)
}

func TestDropdownMenuOpensAndCloses(t *testing.T) {
	var log []string
	app := mountDropdown(t, actionsMenu(&log, nil))
	app.ExpectCount(ddMenu, 0)
	app.Expect(ddTrigger, telatest.HasPopup("menu"), telatest.Expanded(false))

	app.Click(ddTrigger)
	app.ExpectCount(ddMenu, 1)
	// Modal: the page is out of the accessibility tree.
	app.ExpectCount(telatest.ByRole("heading", "Project settings"), 0)
	app.ExpectAria(`
        - menu "Actions":
          - menuitem "New file"
          - menuitem "Open"
          - menuitem "Export" [disabled]
          - separator
          - menuitem "Delete"
    `)

	app.Press("Escape")
	app.ExpectCount(ddMenu, 0)
	app.ExpectFocused(ddTrigger)

	// The trigger toggles it, and so does the keyboard.
	app.Click(ddTrigger)
	app.ExpectCount(ddMenu, 1)
	app.Press("Escape")
	app.Focus(ddTrigger)
	app.Press("Enter")
	app.ExpectCount(ddMenu, 1)
	app.Press("Escape")
	app.ExpectCount(ddMenu, 0)
	if len(log) != 0 {
		t.Errorf("nothing was chosen, but %v was", log)
	}
}

func TestDropdownMenuKeyboard(t *testing.T) {
	var log []string
	app := mountDropdown(t, actionsMenu(&log, nil))
	app.Focus(ddTrigger)
	app.Press("ArrowDown")
	app.ExpectFocused(ddItem("New file"))
	app.Press("ArrowDown")
	app.ExpectFocused(ddItem("Open"))
	app.Press("ArrowDown") // the disabled item is skipped
	app.ExpectFocused(ddItem("Delete"))
	app.Press("Home")
	app.ExpectFocused(ddItem("New file"))
	app.Press("End")
	app.ExpectFocused(ddItem("Delete"))
	app.Press("ArrowUp")
	app.ExpectFocused(ddItem("Open"))
	// Typeahead.
	app.Type("d")
	app.ExpectFocused(ddItem("Delete"))
	app.Press("Escape")
	app.ExpectFocused(ddTrigger)

	// ArrowUp opens it on the last item.
	app.Press("ArrowUp")
	app.ExpectFocused(ddItem("Delete"))
	app.Press("Escape")

	// Enter chooses the focused item, closes the menu and returns the focus.
	app.Press("ArrowDown")
	app.Press("ArrowDown")
	app.Press("Enter")
	app.ExpectCount(ddMenu, 0)
	app.ExpectFocused(ddTrigger)
	if got := fmt.Sprint(log); got != "[open menu:open]" {
		t.Errorf("chosen %s, want the item's OnSelect and then the menu's", got)
	}

	// Space chooses too.
	log = nil
	app.Press("ArrowDown")
	app.Press("Space")
	app.ExpectCount(ddMenu, 0)
	if got := fmt.Sprint(log); got != "[new menu:new]" {
		t.Errorf("chosen %s, want [new menu:new]", got)
	}
}

func TestDropdownMenuPointer(t *testing.T) {
	var log []string
	app := mountDropdown(t, actionsMenu(&log, nil))
	app.Click(ddTrigger)
	// A mouse leaves the focus to the menu itself, and hovering moves it.
	app.Hover(ddItem("Open"))
	app.ExpectFocused(ddItem("Open"))
	app.Click(ddItem("Delete"))
	app.ExpectCount(ddMenu, 0)
	app.ExpectFocused(ddTrigger)
	if got := fmt.Sprint(log); got != "[delete menu:delete]" {
		t.Errorf("chosen %s, want [delete menu:delete]", got)
	}

	// A disabled item does nothing and keeps the menu open.
	log = nil
	app.Click(ddTrigger)
	ex := app.Bounds(ddItem("Export"))
	app.ClickAt(ex.X+ex.Width/2, ex.Y+ex.Height/2)
	app.ExpectCount(ddMenu, 1)
	if len(log) != 0 {
		t.Errorf("a disabled item chose %v", log)
	}
	// A press outside closes it without choosing and is not delivered.
	app.ClickAt(700, 500)
	app.ExpectCount(ddMenu, 0)
	if len(log) != 0 {
		t.Errorf("a press outside chose %v", log)
	}
}

func TestDropdownMenuDisabledItemsAreMarked(t *testing.T) {
	var log []string
	app := mountDropdown(t, actionsMenu(&log, nil))
	app.Click(ddTrigger)
	app.Expect(ddItem("Export"), telatest.Disabled(true))
	app.Expect(ddItem("Open"), telatest.Disabled(false))
	// The recipe takes it out of the pointer's reach (pointer-events-none),
	// so hovering it does not move the focus to it.
	app.Hover(ddItem("Open"))
	ex := app.Bounds(ddItem("Export"))
	app.MouseMove(ex.X+ex.Width/2, ex.Y+ex.Height/2)
	app.ExpectFocused(ddItem("Open"))
}

func TestDropdownMenuShortcutIsShownNotNamed(t *testing.T) {
	var log []string
	app := mountDropdown(t, actionsMenu(&log, nil))
	app.Click(ddTrigger)
	// The shortcut does not become part of the item's name, and is out of
	// the accessibility tree: it is only drawn (the goldens and the Chromium
	// sheet show it).
	app.ExpectCount(ddItem("New file"), 1)
	app.ExpectNoText("⌘N")
	if n := strings.Count(app.AriaSnapshot(), "⌘N"); n != 0 {
		t.Errorf("the shortcut is in the aria snapshot:\n%s", app.AriaSnapshot())
	}
}

func TestDropdownMenuDestructive(t *testing.T) {
	var log []string
	app := mountDropdown(t, actionsMenu(&log, nil), telatest.Dark())
	app.Click(ddTrigger)
	app.Expect(ddItem("Delete"), telatest.Disabled(false))
	if n := app.Count(telatest.ByClass("text-destructive")); n != 1 {
		t.Errorf("%d destructive items, want 1", n)
	}
	// A variant the recipe does not define panics in dev mode, with the
	// closest one.
	d := actionsMenu(&log, func(d *kit.DropdownMenu[string]) {
		d.Items[0].Variant = "destuctive"
		d.DefaultOpen = true
	})
	_, err := telatest.AriaSnapshot(tela.C(pageUnder(d)))
	if err == nil || !strings.Contains(err.Error(), `DropdownMenu item Variant "destuctive"`) || !strings.Contains(err.Error(), `did you mean "destructive"`) {
		t.Fatalf("error %v, want the variant reported with a suggestion", err)
	}
}

func TestDropdownMenuCheckboxItems(t *testing.T) {
	var changes []bool
	var log []string
	app := mountDropdown(t, actionsMenu(&log, func(d *kit.DropdownMenu[string]) {
		d.Items = []kit.MenuItem[string]{
			{Kind: kit.MenuCheckbox, Key: "bars", Label: "Status bar", DefaultChecked: true, OnCheckedChange: func(c bool) { changes = append(changes, c) }},
			{Kind: kit.MenuCheckbox, Key: "panel", Label: "Panel"},
			{Kind: kit.MenuCheckbox, Key: "keep", Label: "Keep open", KeepOpen: true},
		}
	}))
	check := func(name string) telatest.Locator { return telatest.ByRole("menuitemcheckbox", name) }
	app.Click(ddTrigger)
	app.Expect(check("Status bar"), telatest.Checked(true))
	app.Expect(check("Panel"), telatest.Checked(false))

	// Choosing toggles and closes the menu; the state is kept by the
	// component, so it is there when the menu opens again.
	app.Click(check("Status bar"))
	app.ExpectCount(ddMenu, 0)
	app.Click(ddTrigger)
	app.Expect(check("Status bar"), telatest.Checked(false))
	app.Click(check("Panel"))
	app.Click(ddTrigger)
	app.Expect(check("Panel"), telatest.Checked(true))
	if got := fmt.Sprint(changes); got != "[false]" {
		t.Errorf("OnCheckedChange of the first item got %v, want [false]", got)
	}

	// KeepOpen leaves the menu open, and the item shows its new state.
	app.Click(check("Keep open"))
	app.ExpectCount(ddMenu, 1)
	app.Expect(check("Keep open"), telatest.Checked(true))
	app.Focus(check("Keep open"))
	app.Press("Space")
	app.Expect(check("Keep open"), telatest.Checked(false))
	app.ExpectCount(ddMenu, 1)
}

func TestDropdownMenuControlledCheckbox(t *testing.T) {
	var asked []bool
	page := func() tela.Node {
		on := true
		return pageUnder(kit.DropdownMenu[string]{
			Trigger: kit.Button{Children: h.Text("View")},
			Items: []kit.MenuItem[string]{
				{Kind: kit.MenuCheckbox, Key: "bars", Label: "Status bar", Checked: &on, OnCheckedChange: func(c bool) { asked = append(asked, c) }},
			},
		})()
	}
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "View"))
	app.Click(telatest.ByRole("menuitemcheckbox", "Status bar"))
	if got := fmt.Sprint(asked); got != "[false]" {
		t.Errorf("OnCheckedChange got %v, want [false]", got)
	}
	// The parent did not change its state, so the item is still on.
	app.Click(telatest.ByRole("button", "View"))
	app.Expect(telatest.ByRole("menuitemcheckbox", "Status bar"), telatest.Checked(true))
}

func TestDropdownMenuRadioGroup(t *testing.T) {
	var chosen []string
	app := mountDropdown(t, actionsMenu(nil, func(d *kit.DropdownMenu[string]) {
		d.Items = []kit.MenuItem[string]{
			{Kind: kit.MenuRadioGroup, Key: "theme", DefaultValue: new("light"), OnValueChange: func(v string) { chosen = append(chosen, v) },
				Items: []kit.MenuItem[string]{
					{Key: "light", Label: "Light"}, {Key: "dark", Label: "Dark"}, {Key: "system", Label: "System", Disabled: true}}},
		}
	}))
	radio := func(name string) telatest.Locator { return telatest.ByRole("menuitemradio", name) }
	app.Click(ddTrigger)
	app.Expect(radio("Light"), telatest.Checked(true))
	app.Expect(radio("Dark"), telatest.Checked(false))
	// Choosing the chosen one is no change; a disabled one cannot be chosen.
	app.Click(radio("Light"))
	app.ExpectCount(ddMenu, 0)
	app.Click(ddTrigger)
	sys := app.Bounds(radio("System"))
	app.ClickAt(sys.X+sys.Width/2, sys.Y+sys.Height/2)
	app.ExpectCount(ddMenu, 1)
	app.Click(radio("Dark"))
	app.ExpectCount(ddMenu, 0)
	if got := fmt.Sprint(chosen); got != "[dark]" {
		t.Errorf("OnValueChange got %v, want [dark]", got)
	}
	app.Click(ddTrigger)
	app.Expect(radio("Dark"), telatest.Checked(true))
	app.Expect(radio("Light"), telatest.Checked(false))
}

func TestDropdownMenuControlledRadioGroup(t *testing.T) {
	theme := "light"
	var asked []string
	page := func() tela.Node {
		return pageUnder(kit.DropdownMenu[string]{
			Trigger: kit.Button{Children: h.Text("Theme")},
			Items: []kit.MenuItem[string]{
				{Kind: kit.MenuRadioGroup, Key: "theme", Value: &theme, OnValueChange: func(v string) { asked = append(asked, v); theme = v },
					Items: []kit.MenuItem[string]{{Key: "light", Label: "Light"}, {Key: "dark", Label: "Dark"}}},
			},
		})()
	}
	app := telatest.MountFunc(t, page, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Theme"))
	app.Click(telatest.ByRole("menuitemradio", "Dark"))
	app.Click(telatest.ByRole("button", "Theme"))
	app.Expect(telatest.ByRole("menuitemradio", "Dark"), telatest.Checked(true))
	if got := fmt.Sprint(asked); got != "[dark]" {
		t.Errorf("OnValueChange got %v, want [dark]", got)
	}
}

func TestDropdownMenuGroupsAndLabels(t *testing.T) {
	app := mountDropdown(t, actionsMenu(nil, func(d *kit.DropdownMenu[string]) {
		d.DefaultOpen = true
		d.Items = []kit.MenuItem[string]{
			{Kind: kit.MenuLabel, Label: "Account"},
			{Kind: kit.MenuGroup, Label: "Profile actions", Items: []kit.MenuItem[string]{{Key: "a", Label: "Edit"}, {Key: "b", Label: "Share"}}},
			{Kind: kit.MenuSeparator},
			{Key: "c", Label: "Sign out"},
		}
	}))
	app.ExpectAria(`
        - menu "Actions":
          - text: Account
          - group "Profile actions":
            - menuitem "Edit"
            - menuitem "Share"
          - separator
          - menuitem "Sign out"
    `)
}

func TestDropdownMenuDuplicateKeyPanics(t *testing.T) {
	d := actionsMenu(nil, func(d *kit.DropdownMenu[string]) {
		d.DefaultOpen = true
		d.Items = []kit.MenuItem[string]{{Key: "a", Label: "One"}, {Key: "a", Label: "Two"}}
	})
	_, err := telatest.AriaSnapshot(tela.C(pageUnder(d)))
	if err == nil || !strings.Contains(err.Error(), "two items with the key a") {
		t.Fatalf("error %v, want the duplicate key reported", err)
	}
}

func TestDropdownMenuControlledOpen(t *testing.T) {
	var log []bool
	open := true
	d := actionsMenu(nil, func(d *kit.DropdownMenu[string]) {
		d.Open = &open
		d.OnOpenChange = func(o bool) { log = append(log, o) }
	})
	app := mountDropdown(t, d)
	app.ExpectCount(ddMenu, 1)
	app.Press("Escape")
	app.ExpectCount(ddMenu, 1) // the parent did not close it
	if fmt.Sprint(log) != "[false]" {
		t.Errorf("OnOpenChange got %v, want [false]", log)
	}
}

func TestDropdownMenuDisabledTrigger(t *testing.T) {
	app := mountDropdown(t, actionsMenu(nil, func(d *kit.DropdownMenu[string]) { d.Disabled = true }))
	app.Expect(ddTrigger, telatest.Disabled(true))
	tr := app.Bounds(ddTrigger)
	app.ClickAt(tr.X+tr.Width/2, tr.Y+tr.Height/2)
	app.ExpectCount(ddMenu, 0)
}

func TestDropdownMenuTriggerFunc(t *testing.T) {
	d := actionsMenu(nil, func(d *kit.DropdownMenu[string]) {
		d.Trigger = kit.TriggerFunc(func(p tela.Prop) tela.Node {
			return h.Button(p, h.AriaLabel("Actions"), h.Class("rounded-full"), kit.Avatar{Fallback: "AB"})
		})
	})
	app := mountDropdown(t, d)
	app.Click(ddTrigger)
	app.ExpectCount(ddMenu, 1)
}

func TestDropdownMenuPlacement(t *testing.T) {
	const w, hgt = 800, 600
	run := func(name string, x, y float32, edit func(*kit.DropdownMenu[string]), check func(trigger, panel tela.Rect)) {
		t.Run(name, func(t *testing.T) {
			var log []string
			d := actionsMenu(&log, edit)
			page := func() tela.Node {
				return h.Div(h.Class("relative h-full w-full"),
					h.Div(h.Style(tela.Left(x), tela.Top(y)), h.Class("absolute"), d))
			}
			app := telatest.MountFunc(t, page, telatest.Size(w, hgt), telatest.Theme(kit.Theme()))
			tr := app.Bounds(ddTrigger)
			app.Click(ddTrigger)
			check(tr, app.Bounds(ddMenu))
		})
	}
	run("below, centered, 4 px away", 300, 200, nil, func(tr, p tela.Rect) {
		if !near(p.Y, tr.Y+tr.Height+4) || !near(p.X+p.Width/2, tr.X+tr.Width/2) {
			t.Errorf("trigger %v menu %v: want it 4 px below, centered", tr, p)
		}
	})
	run("start", 300, 200, func(d *kit.DropdownMenu[string]) { d.Align = "start" }, func(tr, p tela.Rect) {
		if !near(p.X, tr.X) {
			t.Errorf("trigger %v menu %v: want it at the trigger's left edge", tr, p)
		}
	})
	run("end", 300, 200, func(d *kit.DropdownMenu[string]) { d.Align = "end" }, func(tr, p tela.Rect) {
		if !near(p.X+p.Width, tr.X+tr.Width) {
			t.Errorf("trigger %v menu %v: want it at the trigger's right edge", tr, p)
		}
	})
	run("right", 300, 200, func(d *kit.DropdownMenu[string]) { d.Placement, d.Align = "right", "start" }, func(tr, p tela.Rect) {
		if !near(p.X, tr.X+tr.Width+4) || !near(p.Y, tr.Y) {
			t.Errorf("trigger %v menu %v: want it 4 px to the right", tr, p)
		}
	})
	run("no offset", 300, 200, func(d *kit.DropdownMenu[string]) { d.Offset = kit.NoOffset }, func(tr, p tela.Rect) {
		if !near(p.Y, tr.Y+tr.Height) {
			t.Errorf("trigger %v menu %v: want it against the trigger", tr, p)
		}
	})
	run("flips above", 300, 560, nil, func(tr, p tela.Rect) {
		if !near(p.Y+p.Height+4, tr.Y) {
			t.Errorf("trigger %v menu %v: want it flipped above", tr, p)
		}
	})
}

func TestDropdownMenuLongMenuScrolls(t *testing.T) {
	items := make([]kit.MenuItem[string], 40)
	for i := range items {
		items[i] = kit.MenuItem[string]{Key: fmt.Sprint(i), Label: fmt.Sprintf("Item %d", i)}
	}
	d := actionsMenu(nil, func(d *kit.DropdownMenu[string]) { d.Items = items })
	app := mountDropdown(t, d, telatest.Size(400, 300))
	app.Click(ddTrigger)
	m := app.Bounds(ddMenu)
	if m.Y < 0 || m.Y+m.Height > 300.01 { // the limit is the room to the window's edge, with no margin (ADR-163)
		t.Errorf("a long menu at %v does not fit the 300 px window", m)
	}
	// The keyboard scrolls the focused item into view.
	app.Press("End")
	last := app.Bounds(ddItem("Item 39"))
	if last.Y+last.Height > m.Y+m.Height+0.5 {
		t.Errorf("the last item %v is outside the menu %v", last, m)
	}
}

func TestDropdownMenuStyleProvider(t *testing.T) {
	s := kit.DefaultStyle()
	s.DropdownMenu.Item += " ring-4"
	s.DropdownMenu.ItemVariants["quiet"] = "text-muted-foreground"
	var log []string
	d := actionsMenu(&log, func(d *kit.DropdownMenu[string]) {
		d.DefaultOpen = true
		d.Items[1].Variant = "quiet"
	})
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Fragment(kit.StyleProvider{Style: &s, Children: tela.C(pageUnder(d))})
	}, telatest.Theme(kit.Theme()))
	// "New file", "Open", "Export" and "Delete" are plain items.
	if n := app.Count(telatest.ByClass("ring-4")); n != 4 {
		t.Errorf("%d items have the provider's classes, want 4", n)
	}
	if n := app.Count(telatest.ByClass("text-muted-foreground")); n < 1 {
		t.Errorf("the provider's variant is not applied")
	}
}

func TestDropdownMenuOpenGolden(t *testing.T) {
	var log []string
	d := actionsMenu(&log, func(d *kit.DropdownMenu[string]) {
		d.Align = "start"
		d.Items = []kit.MenuItem[string]{
			{Kind: kit.MenuLabel, Label: "My Account"},
			{Kind: kit.MenuSeparator},
			{Key: "profile", Label: "Profile", Icon: iconMail, Shortcut: "⇧⌘P"},
			{Key: "billing", Label: "Billing", Shortcut: "⌘B"},
			{Kind: kit.MenuCheckbox, Key: "bars", Label: "Status Bar", DefaultChecked: true},
			{Kind: kit.MenuRadioGroup, Key: "theme", DefaultValue: new("dark"), Items: []kit.MenuItem[string]{
				{Key: "light", Label: "Light"}, {Key: "dark", Label: "Dark"}}},
			{Kind: kit.MenuSeparator},
			{Key: "api", Label: "API", Disabled: true},
			{Key: "logout", Label: "Log out", Variant: kit.Destructive, Shortcut: "⇧⌘Q"},
		}
	})
	openGolden(t, "dropdownmenu", 480, 560, pageUnder(d), func(a *telatest.App) {
		a.Focus(ddTrigger)
		a.Press("ArrowDown")
		a.Press("ArrowDown")
		a.Press("ArrowDown")
	})
}

func TestDropdownMenuIdle(t *testing.T) {
	var log []string
	expectIdle(t, pageUnder(actionsMenu(&log, func(d *kit.DropdownMenu[string]) { d.DefaultOpen = true })), nil)
}
