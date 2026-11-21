package aria_test

import (
	"testing"

	"github.com/kuronosu/tela/aria"
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
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			primary, other := primaryOf(goos)
			o := aria.ListBoxOptions[string]{SelectionMode: "multiple", SelectionBehavior: "replace"}
			app := telatest.MountFunc(t, selPage(o, nil), telatest.Size(300, 400), telatest.PlatformKeys(goos))
			click := func(i int, mods ...string) { app.ClickAt(20, float32(i)*32+16, telatest.Modifiers(mods...)) }
			click(1)
			app.ExpectText("selected: banana")
			click(3, primary)
			app.ExpectText("selected: banana,cherry")
			click(1, primary)
			app.ExpectText("selected: cherry")
			click(4, other)
			if goos == "darwin" {
				// Control+click is a context click on a Mac: no click, but
				// the item it is on was not selected, so it becomes the
				// selection, as Finder does (ADR-247).
				app.ExpectText("selected: date")
				click(1, primary)
				click(4, other) // on the selection: it stays
				app.ExpectText("selected: banana,date")
			} else {
				app.ExpectText("selected: date")
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
	for _, goos := range []string{"darwin", "linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			var ev events
			app := telatest.MountFunc(t, ctxPage(aria.ContextMenuTriggerOptions{}, &ev), telatest.Size(400, 300), telatest.PlatformKeys(goos))
			app.ClickAt(90, 60, telatest.Modifiers("Control"))
			got := ev.take()
			if goos != "darwin" {
				if n := app.Count(ctxMenuLoc); n != 0 || got != "inner" {
					t.Errorf("Control+click: %d menus, events %q; want a click and no menu", n, got)
				}
				return
			}
			if app.Count(ctxMenuLoc) != 1 || got != "" {
				t.Fatalf("Control+click: %d menus, events %q; want the menu and no click", app.Count(ctxMenuLoc), got)
			}
			if m := app.Bounds(ctxMenuLoc); m.X != 90 || m.Y != 60 {
				t.Errorf("menu at (%v, %v), want (90, 60)", m.X, m.Y)
			}
			// A Control+click elsewhere on the area moves it there, as a
			// right click does.
			app.ClickAt(40, 40, telatest.Modifiers("Control"))
			if m := app.Bounds(ctxMenuLoc); m.X != 40 || m.Y != 40 {
				t.Errorf("second Control+click: menu at (%v, %v), want (40, 40)", m.X, m.Y)
			}
		})
	}
}
