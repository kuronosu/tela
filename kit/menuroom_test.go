package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/testutil/rawwin"
	"github.com/kuronosu/tela/kit"
)

// A menu scrolls only when the window has no room for it (shadcn's
// max-h-(--radix-context-menu-content-available-height)): the height it is
// limited to must agree with the Flip that decides which side it goes on,
// or a menu that fits is limited anyway (ADR-163). These tests open the
// menu of the gallery's "Right click" demo (a context menu, because it is
// opened at a point and the window's height is what the test varies) in
// windows that leave it a few pixels more than it needs, at the display
// scales 1, 1.5 and 2.

// roomItems are the entries of the gallery's "Right click" demo: three
// items, two separators, two checkboxes, a label and a radio group.
func roomItems() []kit.MenuItem[string] {
	return []kit.MenuItem[string]{
		{Key: "back", Label: "Back", Shortcut: "Cmd+["},
		{Key: "forward", Label: "Forward", Shortcut: "Cmd+]", Disabled: true},
		{Key: "reload", Label: "Reload", Shortcut: "Cmd+R"},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuCheckbox, Key: "bookmarks", Label: "Show Bookmarks", DefaultChecked: true},
		{Kind: kit.MenuCheckbox, Key: "urls", Label: "Show Full URLs"},
		{Kind: kit.MenuSeparator},
		{Kind: kit.MenuLabel, Label: "People"},
		{Kind: kit.MenuRadioGroup, Key: "person", DefaultValue: new("pedro"), Items: []kit.MenuItem[string]{
			{Key: "pedro", Label: "Pedro Duarte"}, {Key: "colm", Label: "Colm Tuite"}}},
	}
}

// roomPage is a context menu whose area is the whole window.
func roomPage() tela.Node {
	return h.Div(h.Class("relative h-full w-full"), kit.ContextMenu[string]{
		AriaLabel: "Actions", Class: "absolute inset-0", Children: h.Text("Right click"), Items: roomItems(),
	})
}

// menuNode returns the open menu of the window, failing the test when there
// is none.
func (a *menuRaw) menuNode() *semantics.Node {
	a.t.Helper()
	n := rawwin.FindNode(a.tree.Semantics(), "menu", "Actions")
	if n == nil {
		a.t.Fatalf("no menu is open:\n%s", a.snapshot())
	}
	return n
}

// scrolls reports whether the menu's content can be scrolled.
func scrolls(n *semantics.Node) bool { return n.States&semantics.StateScrollable != 0 }

var roomScales = []float64{1, 1.5, 2}

// naturalMenuHeight is the height of the menu with all its entries, opened
// in a window that has room for it.
func naturalMenuHeight(t *testing.T, scale float64) float32 {
	t.Helper()
	app := mountMenuRawScale(t, roomPage, 600, 900, scale)
	app.rightClick(100, 300)
	n := app.menuNode()
	if scrolls(n) {
		t.Fatalf("scale %v: the menu scrolls in a 900 px window: %+v", scale, n.Bounds)
	}
	return n.Bounds.Y1 - n.Bounds.Y0
}

// TestContextMenuDoesNotScrollWhenItFits opens the menu where the window
// leaves it between 1 and 40 px more room than its height (the band that a
// 10 px margin on the limit alone used to take from it) and expects its
// whole height, at the pointer, with nothing to scroll.
func TestContextMenuDoesNotScrollWhenItFits(t *testing.T) {
	for _, scale := range roomScales {
		natural := naturalMenuHeight(t, scale)
		for _, extra := range []float32{1, 4, 7, 9.5, 12, 40} {
			t.Run(fmt.Sprintf("scale %v extra %v", scale, extra), func(t *testing.T) {
				const y = 300
				win := y + natural + extra
				app := mountMenuRawScale(t, roomPage, 600, win, scale)
				app.rightClick(100, y)
				n := app.menuNode()
				if scrolls(n) {
					t.Errorf("the menu scrolls in a %v px window, with %v px more room than its %v px: %+v", win, extra, natural, n.Bounds)
				}
				if got := n.Bounds.Y1 - n.Bounds.Y0; got < natural-0.5 || got > natural+0.5 {
					t.Errorf("the menu is %v px tall in a %v px window, want its own %v px", got, win, natural)
				}
				if n.Bounds.Y0 < y-0.5 || n.Bounds.Y0 > y+0.5 {
					t.Errorf("the menu starts at y=%v, want it at the pointer (y=%d)", n.Bounds.Y0, y)
				}
				if n.Bounds.Y1 > win+0.01 {
					t.Errorf("the menu ends at y=%v, past the %v px window", n.Bounds.Y1, win)
				}
			})
		}
	}
}

// TestContextMenuFlipsInsteadOfScrolling opens the menu where there is no
// room below it but plenty above: it goes above the pointer, whole.
func TestContextMenuFlipsInsteadOfScrolling(t *testing.T) {
	for _, scale := range roomScales {
		natural := naturalMenuHeight(t, scale)
		for _, short := range []float32{1, 5, 9.5, 30} {
			t.Run(fmt.Sprintf("scale %v short %v", scale, short), func(t *testing.T) {
				const y = 400
				win := y + natural - short
				app := mountMenuRawScale(t, roomPage, 600, win, scale)
				app.rightClick(100, y)
				n := app.menuNode()
				if scrolls(n) {
					t.Errorf("the menu scrolls in a %v px window instead of going above the pointer: %+v", win, n.Bounds)
				}
				if got := n.Bounds.Y1 - n.Bounds.Y0; got < natural-0.5 || got > natural+0.5 {
					t.Errorf("the menu is %v px tall, want its own %v px", got, natural)
				}
				if n.Bounds.Y1 > y+0.5 {
					t.Errorf("the menu ends at y=%v, want it above the pointer (y=%d)", n.Bounds.Y1, y)
				}
			})
		}
	}
}

// TestContextMenuInALargeWindowNeverScrolls opens it at a spread of points
// of a 1280x800 window, the size of the gallery's.
func TestContextMenuInALargeWindowNeverScrolls(t *testing.T) {
	for _, scale := range roomScales {
		app := mountMenuRawScale(t, roomPage, 1280, 800, scale)
		for _, p := range [][2]float32{{5, 5}, {300, 333}, {420.4, 410.3}, {550, 480}, {300, 506}, {300, 512.5}, {1200, 300}, {640, 790}, {900, 760.25}} {
			app.rightClick(p[0], p[1])
			n := app.menuNode()
			if scrolls(n) {
				t.Errorf("scale %v: the menu opened at %v scrolls: %+v", scale, p, n.Bounds)
			}
			if n.Bounds.Y0 < -0.01 || n.Bounds.Y1 > 800.01 {
				t.Errorf("scale %v: the menu opened at %v is outside the window: %+v", scale, p, n.Bounds)
			}
		}
	}
}

// TestContextMenuScrollsInAShortWindow keeps the other half of the rule: a
// window with no room for the menu on either side limits it to the room
// and the menu scrolls.
func TestContextMenuScrollsInAShortWindow(t *testing.T) {
	for _, scale := range roomScales {
		natural := naturalMenuHeight(t, scale)
		t.Run(fmt.Sprintf("scale %v", scale), func(t *testing.T) {
			win := natural - 60
			app := mountMenuRawScale(t, roomPage, 600, win, scale)
			app.rightClick(100, win/2)
			n := app.menuNode()
			if !scrolls(n) {
				t.Errorf("the menu does not scroll in a %v px window, shorter than its %v px: %+v", win, natural, n.Bounds)
			}
			if n.Bounds.Y0 < -0.01 || n.Bounds.Y1 > win+0.01 {
				t.Errorf("the menu %+v is outside the %v px window", n.Bounds, win)
			}
			if got := n.Bounds.Y1 - n.Bounds.Y0; got >= natural-1 {
				t.Errorf("the menu is %v px tall in a %v px window, want it limited to the room", got, win)
			}
		})
	}
}
