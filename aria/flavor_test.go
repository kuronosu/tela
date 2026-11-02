package aria_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/telatest"
)

// The collections' primary modifier (React Aria's isCtrlKeyPressed) is the
// event's CmdOrCtrl: Command under the Mac flavor, Control elsewhere
// (ADR-221), so each platform's behavior is tested on any OS.

// primary and other are the modifier names Press uses for the primary
// modifier of goos and for the one that is not.
func primaryOf(goos string) (primary, other string) {
	if goos == "darwin" {
		return "Meta", "Control"
	}
	return "Control", "Meta"
}

func TestListBoxSelectAllPerPlatform(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			primary, other := primaryOf(goos)
			o := aria.ListBoxOptions[string]{SelectionMode: "multiple"}
			app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400), telatest.PlatformKeys(goos))
			app.Focus(option("Banana"))
			// The other modifier does nothing special.
			app.Press(other + "+a")
			app.ExpectText("selected:")
			app.Expect(option("Apple"), telatest.Selected(false))
			app.Press(primary + "+a")
			app.Expect(option("Apple"), telatest.Selected(true))
			app.Expect(option("Fig"), telatest.Selected(true))
			// ControlOrMeta is the primary modifier.
			app.Press("Escape")
			app.Expect(option("Apple"), telatest.Selected(false))
			app.Press("ControlOrMeta+a")
			app.Expect(option("Cherry"), telatest.Selected(true))
		})
	}
}

// With the "replace" behavior the primary modifier moves the focus without
// selecting and toggles with Space; the other modifier moves and replaces
// like a plain key.
func TestListBoxReplaceKeysPerPlatform(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			primary, other := primaryOf(goos)
			o := aria.ListBoxOptions[string]{SelectionMode: "multiple", SelectionBehavior: "replace"}
			app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400), telatest.PlatformKeys(goos))
			app.Click(option("Apple"))
			app.ExpectText("selected: apple")
			app.Press(primary + "+ArrowDown")
			app.Press(primary + "+ArrowDown")
			app.ExpectFocused(option("Blueberry"))
			app.ExpectText("selected: apple")
			app.Press(primary + "+Space")
			app.ExpectText("selected: apple,blueberry")
			app.Press(other + "+ArrowDown")
			app.ExpectFocused(option("Cherry"))
			app.ExpectText("selected: cherry")
		})
	}
}

// A click with the primary modifier toggles an item in a "replace" list;
// with the other modifier it replaces the selection, except on a Mac,
// where Control+click opens a context menu instead of clicking.
func TestListBoxModifierClickPerPlatform(t *testing.T) {
	for _, c := range []struct {
		fl             platform.Flavor
		primary, other platform.Modifiers
	}{
		{platform.FlavorMac, platform.ModSuper, platform.ModCtrl},
		{platform.FlavorLinux, platform.ModCtrl, platform.ModSuper},
		{platform.FlavorWindows, platform.ModCtrl, platform.ModSuper},
	} {
		t.Run(c.fl.String(), func(t *testing.T) {
			o := aria.ListBoxOptions[string]{SelectionMode: "multiple", SelectionBehavior: "replace"}
			b, host, win := rawwin.Mount(t, tela.C(selPage(o, nil)), 300, 400, 0, core.Config{Flavor: c.fl})
			app := &rawApp{t: t, b: b, host: host, win: win, tree: host.Tree()}
			click := func(i int, mods platform.Modifiers) { app.clickWith(20, float32(i)*32+16, mods) }
			want := func(what, sel string) {
				t.Helper()
				if s := app.tree.AriaSnapshot(); !strings.Contains(s, `"selected: `+sel+`"`) {
					t.Errorf("%s:\n%s", what, s)
				}
			}
			click(1, 0)
			want("click", "banana")
			click(3, c.primary)
			want("primary+click", "banana,cherry")
			click(1, c.primary)
			want("primary+click again", "cherry")
			click(4, c.other)
			if c.fl == platform.FlavorMac {
				// Control+click is a context click on a Mac: no click.
				want("other+click", "cherry")
			} else {
				want("other+click", "date")
			}
		})
	}
}

// Parallel apps with different flavors do not interfere.
func TestListBoxFlavorsInParallel(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "windows", "darwin", "linux"} {
		t.Run(goos, func(t *testing.T) {
			t.Parallel()
			primary, other := primaryOf(goos)
			o := aria.ListBoxOptions[string]{SelectionMode: "multiple"}
			app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400), telatest.PlatformKeys(goos))
			for i := 0; i < 3; i++ {
				app.Focus(option("Banana"))
				app.Press(other + "+a")
				app.Expect(option("Apple"), telatest.Selected(false))
				app.Press(primary + "+a")
				app.Expect(option("Apple"), telatest.Selected(true))
				app.Press("Escape")
				app.Expect(option("Apple"), telatest.Selected(false))
			}
		})
	}
}

// A Control+click opens a context menu under the Mac flavor (one-button
// trackpads), without clicking what it is on; elsewhere it is a click.
func TestContextMenuControlClickPerFlavor(t *testing.T) {
	for _, fl := range []platform.Flavor{platform.FlavorMac, platform.FlavorLinux, platform.FlavorWindows} {
		t.Run(fl.String(), func(t *testing.T) {
			var ev events
			b, host, win := rawwin.Mount(t, tela.C(ctxPage(aria.ContextMenuTriggerOptions{}, &ev)), 400, 300, 0, core.Config{Flavor: fl})
			app := &rawApp{t: t, b: b, host: host, win: win, tree: host.Tree()}
			app.clickWith(90, 60, platform.ModCtrl)
			got := ev.take()
			if fl != platform.FlavorMac {
				if hasMenu(app) || got != "inner" {
					t.Errorf("Control+click: menu %v, events %q; want a click and no menu", hasMenu(app), got)
				}
				return
			}
			if !hasMenu(app) || got != "" {
				t.Fatalf("Control+click: menu %v, events %q; want the menu and no click", hasMenu(app), got)
			}
			if x, y, _, _ := menuBounds(t, app); x != 90 || y != 60 {
				t.Errorf("menu at (%v, %v), want (90, 60)", x, y)
			}
			// A Control+click elsewhere on the area moves it there, as a
			// right click does.
			app.clickWith(40, 40, platform.ModCtrl)
			if x, y, _, _ := menuBounds(t, app); x != 40 || y != 40 {
				t.Errorf("second Control+click: menu at (%v, %v), want (40, 40)", x, y)
			}
		})
	}
}
