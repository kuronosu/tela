package platform

import (
	"errors"
	"testing"
)

func TestMenuRoles(t *testing.T) {
	names := map[MenuRole]string{
		RoleNone: "", RoleUndo: "undo", RoleRedo: "redo", RoleCut: "cut", RoleCopy: "copy", RolePaste: "paste",
		RoleSelectAll: "selectAll", RoleMinimize: "minimize", RoleZoom: "zoom", RoleClose: "close",
		RoleEditMenu: "editMenu", RoleWindowMenu: "windowMenu", RoleHelpMenu: "help",
	}
	for r, want := range names {
		if r.String() != want || !r.Valid() {
			t.Errorf("role %d: String %q Valid %v, want %q (Electron's name)", r, r.String(), r.Valid(), want)
		}
		if got := r >= RoleEditMenu; r.IsMenu() != got {
			t.Errorf("%v.IsMenu() = %v", r, r.IsMenu())
		}
	}
	if bad := RoleHelpMenu + 1; bad.Valid() || bad.String() != "MenuRole(13)" {
		t.Errorf("invalid role: Valid %v, String %q", bad.Valid(), bad.String())
	}
}

// TestCheckMenuRoles checks the role rules of CheckMenu: role items are
// activatable items with IDs, menu roles mark submenus, once each.
func TestCheckMenuRoles(t *testing.T) {
	bar := []MenuItem{
		{Label: "File", Submenu: []MenuItem{{ID: 1, Label: "Open"}}},
		{Label: "Edit", Role: RoleEditMenu, Submenu: []MenuItem{
			{ID: 2, Label: "Undo", Role: RoleUndo, Accelerator: "Cmd+Z"},
			{Separator: true, Role: RoleCopy}, // a separator's other fields are ignored
			{ID: 3, Label: "Copy", Role: RoleCopy},
			{ID: 4, Label: "Select All", Role: RoleSelectAll},
		}},
		{Label: "Window", Role: RoleWindowMenu, Submenu: []MenuItem{{ID: 5, Label: "Minimize", Role: RoleMinimize}, {ID: 6, Label: "Zoom", Role: RoleZoom}}},
		{Label: "Help", Role: RoleHelpMenu, Submenu: []MenuItem{{ID: 7, Label: "Tela Help"}}},
	}
	CheckMenu("bar", bar)
	CheckPopupMenu(Point{}, []MenuItem{{ID: 1, Label: "Cut", Role: RoleCut}, {ID: 2, Label: "Paste", Role: RolePaste}}, true)

	for _, c := range []struct {
		want string
		menu []MenuItem
	}{
		{"has ID 0", []MenuItem{{Label: "Copy", Role: RoleCopy}}},
		{"reuses ID 1", []MenuItem{{ID: 1, Label: "Open"}, {ID: 1, Label: "Close", Role: RoleClose}}},
		{"invalid Role 99", []MenuItem{{ID: 1, Role: 99}}},
		{"the role of an item", []MenuItem{{Label: "Edit", Role: RoleCopy, Submenu: []MenuItem{{ID: 1}}}}},
		{"the role of a submenu", []MenuItem{{ID: 1, Label: "Window", Role: RoleWindowMenu}}},
		{"m[1] (\"Window\") has Role windowMenu, which m[0] has already", []MenuItem{
			{Label: "Window", Role: RoleWindowMenu, Submenu: []MenuItem{{ID: 1}}},
			{Label: "Window", Role: RoleWindowMenu, Submenu: []MenuItem{{ID: 2}}},
		}},
		{"m[0].Submenu[0]", []MenuItem{{Label: "More", Submenu: []MenuItem{{Label: "Help", Role: RoleHelpMenu}}}}},
	} {
		expectPanic(t, c.want, func() { CheckMenu("m", c.menu) })
	}
}

func TestCheckAppMenu(t *testing.T) {
	menu := []MenuItem{{Label: "File", Submenu: []MenuItem{{ID: 1, Label: "New"}}}}
	items := []MenuItem{{ID: 2, Label: "Settings…", Accelerator: "Cmd+,"}, {Separator: true}, {ID: 3, Label: "Check for Updates"}}
	CheckAppMenu(menu, items)
	CheckAppMenu(nil, nil)
	expectPanic(t, "SetAppMenu appItems[0] (\"Again\") reuses ID 1", func() { CheckAppMenu(menu, []MenuItem{{ID: 1, Label: "Again"}}) })
	expectPanic(t, "among the app items", func() {
		CheckAppMenu(nil, []MenuItem{{Label: "Help", Role: RoleHelpMenu, Submenu: []MenuItem{{ID: 1}}}})
	})
	expectPanic(t, "SetAppMenu menu[0]", func() { CheckAppMenu([]MenuItem{{Label: "x"}}, nil) })

	d := UnsupportedDesktop{}
	if err := d.SetAppMenu(menu, items); !errors.Is(err, ErrUnsupported) {
		t.Errorf("UnsupportedDesktop.SetAppMenu = %v, want ErrUnsupported", err)
	}
	expectPanic(t, "has ID 0", func() { _ = d.SetAppMenu(nil, []MenuItem{{Label: "x"}}) })
	var _ Event = AppMenuEvent{}
	var _ Event = QuitRequestedEvent{}
}

func TestTitleBarOverlayOptions(t *testing.T) {
	o := WindowOptions{TitleBarOverlay: true}.Resolve()
	if !o.TitleBarOverlay || o.Frameless {
		t.Errorf("Resolve changed TitleBarOverlay: %+v", o)
	}
	if f := o.OverlayAsFrameless(); !f.Frameless || f.TitleBarOverlay {
		t.Errorf("OverlayAsFrameless = Frameless %v, TitleBarOverlay %v; want a Frameless window", f.Frameless, f.TitleBarOverlay)
	}
	for _, plain := range []WindowOptions{{}, {Frameless: true}, {Title: "x", FixedSize: true}} {
		if plain.OverlayAsFrameless() != plain {
			t.Errorf("OverlayAsFrameless changed %+v", plain)
		}
	}
	expectPanic(t, "both Frameless and TitleBarOverlay", func() { WindowOptions{Frameless: true, TitleBarOverlay: true}.Resolve() })

	st := WindowState{ClientSideDecorations: true, TitleBarControls: Rect{X: 8, Y: 6, W: 52, H: 16}}
	if got := st.String(); got != "{csd controls 8,6 52x16}" {
		t.Errorf("WindowState.String() = %q", got)
	}
}
