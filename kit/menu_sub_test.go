package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// subItems is a menu with a submenu, which has a submenu of its own
// (shadcn's DropdownMenuSub demo).
func subItems(log *[]string) []kit.MenuItem[string] {
	return []kit.MenuItem[string]{
		{Key: "team", Label: "Team"},
		{Kind: kit.MenuSub, Key: "invite", Label: "Invite users", Items: []kit.MenuItem[string]{
			{Key: "email", Label: "Email", OnSelect: func() { *log = append(*log, "email") }},
			{Key: "message", Label: "Message"},
			{Kind: kit.MenuSeparator},
			{Kind: kit.MenuSub, Key: "more", Label: "More...", Items: []kit.MenuItem[string]{
				{Key: "link", Label: "Copy link"},
				{Key: "qr", Label: "QR code"},
			}},
		}},
		{Key: "new-team", Label: "New Team", Shortcut: "⌘T"},
	}
}

func subDropdown(log *[]string, dir string) func() tela.Node {
	return func() tela.Node {
		return h.Div(h.Class("p-6"), h.Dir(dir), kit.DropdownMenu[string]{
			Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Open")}, Align: "start",
			Items: subItems(log), OnSelect: func(k string) { *log = append(*log, "select:"+k) },
		})
	}
}

func TestDropdownSubmenuKeyboard(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, subDropdown(&log, ""), telatest.Size(640, 480), telatest.Theme(kit.Theme()))
	trigger := telatest.ByRole("button", "Open")
	invite := cmItem("Invite users")
	app.Focus(trigger)
	app.Press("Enter")
	app.Press("ArrowDown")
	app.ExpectFocused(invite)
	app.Expect(invite, telatest.Expanded(false))
	app.Press("ArrowRight")
	app.ExpectFocused(cmItem("Email"))
	app.Expect(invite, telatest.Expanded(true))
	// The submenu is beside its trigger, its first item level with it.
	if s, i := app.Bounds(menuNamed("Invite users")), app.Bounds(invite); s.X < i.X+i.Width-1 || s.Y > i.Y {
		t.Errorf("submenu %v, trigger %v: want it on the right", s, i)
	}
	app.Press("ArrowLeft")
	app.ExpectFocused(invite)
	app.ExpectCount(cmItem("Email"), 0)
	// A nested submenu; Escape closes one level at a time.
	app.Press("Space")
	app.Press("End")
	app.ExpectFocused(cmItem("More..."))
	app.Press("Enter")
	app.ExpectFocused(cmItem("Copy link"))
	app.Press("Escape")
	app.ExpectFocused(cmItem("More..."))
	app.Press("Escape")
	app.ExpectFocused(invite)
	// Choosing an item of a submenu closes the whole menu: the item's
	// OnSelect, then the menu's with the item's key.
	app.Press("ArrowRight")
	app.Press("Enter")
	if got := fmt.Sprint(log); got != "[email select:email]" {
		t.Errorf("log %s", got)
	}
	app.ExpectCount(cmItem("Team"), 0)
	app.ExpectFocused(trigger)
}

func TestDropdownSubmenuRTL(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, subDropdown(&log, "rtl"), telatest.Size(640, 480), telatest.Theme(kit.Theme()))
	invite := cmItem("Invite users")
	app.Focus(telatest.ByRole("button", "Open"))
	app.Press("Enter")
	app.Press("ArrowDown")
	app.Press("ArrowRight")
	app.ExpectFocused(invite)
	app.Press("ArrowLeft")
	app.ExpectFocused(cmItem("Email"))
	if s, i := app.Bounds(menuNamed("Invite users")), app.Bounds(invite); s.X+s.Width > i.X+1 {
		t.Errorf("submenu %v, trigger %v: want it on the left", s, i)
	}
	app.Press("ArrowRight")
	app.ExpectFocused(invite)
}

func TestDropdownSubmenuPointer(t *testing.T) {
	var log []string
	app := telatest.MountFunc(t, subDropdown(&log, ""), telatest.Size(640, 480), telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open"))
	invite := cmItem("Invite users")
	app.Hover(invite)
	app.Advance(2 * aria.SubmenuDelay)
	app.ExpectCount(cmItem("Email"), 1)
	app.Hover(cmItem("Message"))
	app.Click(cmItem("Message"))
	if got := fmt.Sprint(log); got != "[select:message]" {
		t.Errorf("log %s", got)
	}
	app.ExpectCount(cmItem("Team"), 0)
	// A press outside closes every level.
	app.Click(telatest.ByRole("button", "Open"))
	app.Click(invite)
	app.ExpectCount(cmItem("Email"), 1)
	app.ClickAt(600, 450)
	app.ExpectCount(cmItem("Email"), 0)
	app.ExpectCount(cmItem("Team"), 0)
}

func TestContextMenuSubmenu(t *testing.T) {
	var log []string
	page := contextPage(kit.ContextMenu[string]{Items: subItems(&log), OnSelect: func(k string) { log = append(log, "select:"+k) }})
	app := mountContext(t, page, 640, 480)
	app.ClickAt(100, 80, telatest.Right())
	app.Click(cmItem("Invite users"))
	app.Hover(cmItem("More..."))
	app.Advance(2 * aria.SubmenuDelay)
	app.Click(cmItem("QR code"))
	if got := fmt.Sprint(log); got != "[select:qr]" {
		t.Errorf("log %s", got)
	}
	app.ExpectCount(cmMenu, 0)
}

// In a menu bar, ArrowRight on a submenu trigger opens its submenu (it
// does not move to the next menu), ArrowLeft in the submenu closes it
// (it does not move to the previous menu), and ArrowRight on another item
// of the submenu moves to the next menu.
func TestMenubarSubmenuArrows(t *testing.T) {
	var log []string
	file := fileMenu()
	file.Items = append(file.Items, kit.MenuItem[string]{Kind: kit.MenuSub, Key: "share", Label: "Share", Items: []kit.MenuItem[string]{
		{Key: "email", Label: "Email link"}, {Key: "messages", Label: "Messages"}}})
	app := mountBar(t, appBar(func(m *kit.Menubar[string]) {
		m.Menus[0] = file
		m.OnSelect = func(menu, item string) { log = append(log, menu+"/"+item) }
	}))
	app.Focus(barItem("File"))
	app.Press("ArrowUp") // the last item: Share
	share := barItem("Share")
	app.ExpectFocused(share)
	app.Press("ArrowRight")
	app.ExpectFocused(barItem("Email link"))
	app.ExpectCount(menuNamed("File"), 1)
	app.Press("ArrowLeft")
	app.ExpectFocused(share)
	app.ExpectCount(menuNamed("File"), 1)
	app.Press("ArrowRight")
	app.Press("ArrowDown")
	app.Press("Enter")
	if got := fmt.Sprint(log); got != "[file/messages]" {
		t.Errorf("log %s", got)
	}
	app.Press("ArrowUp") // open File again on its last item, Share
	app.Press("ArrowRight")
	app.ExpectFocused(barItem("Email link"))
	app.Press("ArrowRight")
	app.ExpectCount(menuNamed("Edit"), 1)
	app.ExpectFocused(barItem("Undo"))
}

func TestNativeContextMenuSubmenu(t *testing.T) {
	var log []string
	page := contextPage(kit.ContextMenu[string]{Native: kit.NativeOn, Items: subItems(&log),
		OnSelect: func(k string) { log = append(log, "select:"+k) }})
	app := mountContext(t, page, 640, 480)
	app.ClickAt(100, 80, telatest.Right())
	ms := app.PopupMenus()
	if len(ms) != 1 || len(ms[0].Items) != 3 || len(ms[0].Items[1].Submenu) != 4 || len(ms[0].Items[1].Submenu[3].Submenu) != 2 {
		t.Fatalf("popup menus %+v", ms)
	}
	app.ChoosePopupItem("Invite users", "More...", "QR code")
	if got := fmt.Sprint(log); got != "[select:qr]" {
		t.Errorf("log %s", got)
	}
}

func TestMenuSubmenuDuplicateKeyPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(fmt.Sprint(r), `key team`) {
			t.Errorf("recover() = %v, want the duplicate key named", r)
		}
	}()
	items := []kit.MenuItem[string]{
		{Key: "team", Label: "Team"},
		{Kind: kit.MenuSub, Key: "invite", Label: "Invite", Items: []kit.MenuItem[string]{{Key: "team", Label: "Team again"}}},
	}
	app := telatest.MountFunc(t, func() tela.Node {
		return kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items}
	}, telatest.Theme(kit.Theme()))
	app.Click(telatest.ByRole("button", "Open"))
}

// The open submenu, left to right and right to left (the chevron turns,
// the submenu opens on the other side).
func TestDropdownSubmenuGolden(t *testing.T) {
	for _, dir := range []string{"ltr", "rtl"} {
		var log []string
		page := func() tela.Node {
			return h.Div(h.Class("flex justify-center p-6"), h.Dir(dir), kit.DropdownMenu[string]{
				Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Open")},
				Items:   subItems(&log),
			})
		}
		openGolden(t, "dropdown_submenu_"+dir, 520, 300, page, func(a *telatest.App) {
			a.Focus(telatest.ByRole("button", "Open"))
			a.Press("Enter")
			a.Press("ArrowDown")
			if dir == "rtl" {
				a.Press("ArrowLeft")
			} else {
				a.Press("ArrowRight")
			}
			a.Press("ArrowDown")
		})
	}
}

// A key is unique across the whole menu, every level: two submenus with
// the same key inside, or an item after a submenu with one of its keys,
// would make a native menu run the wrong handler (flattenDeep keeps one).
func TestMenuSubmenuDuplicateKeyAcrossLevelsPanics(t *testing.T) {
	sub := func(key string, items ...kit.MenuItem[string]) kit.MenuItem[string] {
		return kit.MenuItem[string]{Kind: kit.MenuSub, Key: key, Label: key, Items: items}
	}
	cases := map[string][]kit.MenuItem[string]{
		"sibling submenus": {
			sub("a", kit.MenuItem[string]{Key: "x", Label: "X in a"}),
			sub("b", kit.MenuItem[string]{Key: "x", Label: "X in b"}),
		},
		"item after the submenu": {
			sub("a", kit.MenuItem[string]{Key: "x", Label: "X in a"}),
			{Key: "x", Label: "X"},
		},
		"radio item after the submenu": {
			sub("a", kit.MenuItem[string]{Key: "x", Label: "X in a"}),
			{Kind: kit.MenuRadioGroup, Key: "g", Items: []kit.MenuItem[string]{{Key: "x", Label: "X"}}},
		},
		"nested submenus": {
			sub("a", sub("a1", kit.MenuItem[string]{Key: "x", Label: "X in a1"})),
			sub("b", sub("b1", kit.MenuItem[string]{Key: "x", Label: "X in b1"})),
		},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil || !strings.Contains(fmt.Sprint(r), `key x`) {
					t.Errorf("recover() = %v, want the duplicate key named", r)
				}
			}()
			app := telatest.MountFunc(t, func() tela.Node {
				return kit.DropdownMenu[string]{Trigger: kit.Button{Children: h.Text("Open")}, Items: items}
			}, telatest.Theme(kit.Theme()))
			app.Click(telatest.ByRole("button", "Open"))
		})
	}
}
