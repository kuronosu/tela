package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/telatest"
)

func menuItem(name string) telatest.Locator { return telatest.ByRole("menuitem", name) }

// contextMenu selects an entry and opens the context menu with the keyboard
// (the headless window has no right button): the menu of the files opens on
// Shift+F10.
func contextMenu(a *telatest.App, name string) {
	a.Click(cell(name))
	a.Press("Shift+F10")
	a.Advance(300e6)
}

// TestContextMenu: the menu of a file offers its actions, in every look.
func TestContextMenu(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		contextMenu(a, "Resume.pdf")
		for _, name := range []string{"Open", "Cut", "Copy", "Rename", "Delete", "New folder"} {
			a.ExpectCount(menuItem(name), 1)
		}
		a.Expect(menuItem("Paste"), telatest.Disabled(true)) // nothing was copied

		a.Click(menuItem("Copy"))
		a.ExpectText("Copied “Resume.pdf”")
		a.ExpectCount(menuItem("Copy"), 0) // the menu closed

		a.Press("Shift+F10")
		a.Advance(300e6)
		a.Expect(menuItem("Paste"), telatest.Disabled(false))
		a.Click(menuItem("Paste"))
		a.ExpectCount(cell("Resume (2).pdf"), 1)
		a.Expect(row("Resume (2).pdf"), telatest.Selected(true))
	})
}

// TestCutAndPaste moves an entry into another folder.
func TestCutAndPaste(t *testing.T) {
	a := start(t, "adwaita", tela.Light, "Documents")
	a.Click(cell("todo.md"))
	a.Press("Control+x")
	a.ExpectText("Cut “todo.md”")
	a.DoubleClick(cell("Reports"))
	a.Press("Control+v")
	a.ExpectCount(cell("todo.md"), 1)
	a.Click(btn("Go to Documents"))
	a.ExpectCount(cell("todo.md"), 0)
	a.ExpectCount(cell("Resume.pdf"), 1)
}

// TestRename: F2 opens the dialog with the name, Enter renames.
func TestRename(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		a.Press("F2") // nothing is selected: nothing happens
		a.ExpectCount(telatest.ByRole("dialog", "Rename"), 0)

		a.Click(cell("todo.md"))
		a.Press("F2")
		a.Advance(300e6)
		input := telatest.ByRole("textbox", "")
		a.ExpectCount(telatest.ByRole("dialog", "Rename"), 1)
		a.ExpectValue(input, "todo.md")
		a.Fill(input, "tasks.md")
		a.Press("Enter")
		a.Advance(300e6)
		a.ExpectCount(cell("tasks.md"), 1)
		a.ExpectCount(cell("todo.md"), 0)
		a.Expect(row("tasks.md"), telatest.Selected(true))
		a.ExpectCount(telatest.ByRole("dialog", "Rename"), 0)
		a.ExpectText("Renamed to “tasks.md”")
	})
}

// TestRenameFolderKeepsHistory: the history follows a renamed folder.
func TestRenameFolderKeepsHistory(t *testing.T) {
	a := start(t, "breeze", tela.Light, "Documents")
	a.DoubleClick(cell("Reports"))
	a.Click(btn("Up"))
	a.Click(cell("Reports"))
	a.Press("F2")
	a.Advance(300e6)
	a.Fill(telatest.ByRole("textbox", ""), "Archive")
	a.Press("Enter")
	a.Advance(300e6)
	a.ExpectCount(cell("Archive"), 1)
	a.Click(btn("Back")) // into the renamed folder
	a.ExpectCount(cell("Q3 summary.pdf"), 1)
	a.ExpectCount(btn("Go to Archive"), 1)
}

// TestDelete: the alert dialog asks, Cancel keeps the file and Delete
// removes it.
func TestDelete(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		a.Click(cell("todo.md"))
		a.Press("Delete")
		a.Advance(300e6)
		dlg := telatest.ByRole("alertdialog", "")
		a.ExpectCount(dlg, 1)
		a.ExpectText("Delete “todo.md”?")
		a.Click(btn("Cancel"))
		a.Advance(300e6)
		a.ExpectCount(cell("todo.md"), 1)

		a.Press("Delete")
		a.Advance(300e6)
		a.Click(telatest.ByRole("button", "Delete").Filter(telatest.HasText("Delete")).Nth(-1))
		a.Advance(300e6)
		a.ExpectCount(cell("todo.md"), 0)
		a.ExpectText("Deleted “todo.md”")
		a.ExpectCount(telatest.ByRole("alertdialog", ""), 0)
	})
}

// TestNewFolder from the menu.
func TestNewFolder(t *testing.T) {
	a := start(t, "adwaita", tela.Light, "Documents")
	a.Click(btn("More options"))
	a.Advance(300e6)
	a.Click(menuItem("New folder"))
	a.Advance(300e6)
	a.ExpectCount(cell("New folder"), 1)
	a.Expect(row("New folder"), telatest.Selected(true))
	a.Click(btn("More options"))
	a.Advance(300e6)
	a.Click(menuItem("New folder"))
	a.ExpectCount(cell("New folder (2)"), 1)
}

// TestViewSwitcher: the segmented control swaps the table for tiles and
// back, and a double click on a tile enters the folder.
func TestViewSwitcher(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := start(t, id, s, "Documents")
		a.Expect(telatest.ByRole("radio", "Details view"), telatest.Checked(true))
		a.Click(telatest.ByRole("radio", "Large icons view"))
		a.Expect(telatest.ByRole("radio", "Large icons view"), telatest.Checked(true))
		a.ExpectCount(telatest.ByRole("table", ""), 0)
		a.ExpectCount(btn("Projects"), 1)
		a.Click(btn("Resume.pdf"))
		a.Expect(btn("Resume.pdf"), telatest.Pressed(true))
		a.Expect(btn("Projects"), telatest.Pressed(false))
		a.DoubleClick(btn("Projects"))
		a.ExpectCount(btn("tela"), 1)
		a.Click(telatest.ByRole("radio", "Details view"))
		a.ExpectCount(cell("tela"), 1)
	})
}

// TestIconViewGolden is each look with the icon view and a selection.
func TestIconViewGolden(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		p := shown(id, s)
		p.View = "icons"
		mount(t, p).Golden(fmt.Sprintf("native_%s_%s_icons.png", id, schemeName(s)))
	})
}

// TestMenusGolden shows each look's context menu and, in the toolbar, its
// menu, over the list.
func TestMenusGolden(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := mount(t, shown(id, s))
		a.Press("Shift+F10")
		a.Advance(300e6)
		a.Golden(fmt.Sprintf("native_%s_%s_context.png", id, schemeName(s)))
	})
}

// TestDialogsGolden shows the rename dialog and the delete confirmation.
func TestDialogsGolden(t *testing.T) {
	eachLook(t, func(t *testing.T, id string, s tela.ColorScheme) {
		a := mount(t, shown(id, s))
		a.Click(cell("Contract draft.docx"))
		a.Press("F2")
		a.Advance(300e6)
		a.Golden(fmt.Sprintf("native_%s_%s_rename.png", id, schemeName(s)))
		a.Press("Escape")
		a.Advance(300e6)
		a.Press("Delete")
		a.Advance(300e6)
		a.Golden(fmt.Sprintf("native_%s_%s_delete.png", id, schemeName(s)))
	})
}

// TestDetailsPane: the menu hides and shows the pane, which the look lays
// out beside the list.
func TestDetailsPane(t *testing.T) {
	a := start(t, "win11", tela.Light, "Documents")
	complementary := telatest.ByRole("complementary", "Details")
	a.ExpectCount(complementary, 1)
	a.Click(btn("View"))
	a.Advance(300e6)
	a.Click(telatest.ByRole("menuitemcheckbox", "Details pane"))
	a.Advance(300e6)
	a.ExpectCount(complementary, 0)
	a.Click(btn("View"))
	a.Advance(300e6)
	a.Click(telatest.ByRole("menuitemcheckbox", "Details pane"))
	a.ExpectCount(complementary, 1)
}

// TestSortMenu: the Sort menu and the headers are two ways to one state.
func TestSortMenu(t *testing.T) {
	a := start(t, "win11", tela.Light, "Documents")
	a.Click(btn("Sort"))
	a.Advance(300e6)
	a.Click(telatest.ByRole("menuitemradio", "Size"))
	a.Advance(300e6)
	a.Expect(telatest.ByRole("columnheader", "Size"), telatest.Sort("ascending"))
	a.Click(btn("Sort"))
	a.Advance(300e6)
	a.Click(telatest.ByRole("menuitemradio", "Descending"))
	a.Advance(300e6)
	a.Expect(telatest.ByRole("columnheader", "Size"), telatest.Sort("descending"))
}

// TestToolbarButtons: the buttons of the command bar act on the selection.
func TestToolbarButtons(t *testing.T) {
	a := start(t, "win11", tela.Light, "Documents")
	a.Expect(btn("Delete"), telatest.Disabled(true))
	a.Expect(btn("Paste"), telatest.Disabled(true))
	a.Click(cell("Resume.pdf"))
	a.Expect(btn("Delete"), telatest.Disabled(false))
	a.Click(btn("Copy"))
	a.Expect(btn("Paste"), telatest.Disabled(false))
	a.Click(btn("Paste"))
	a.ExpectCount(cell("Resume (2).pdf"), 1)
	a.Click(btn("Rename"))
	a.Advance(300e6)
	a.ExpectCount(telatest.ByRole("dialog", "Rename"), 1)
}

// TestTooltips: an icon button of the toolbar says what it does when the
// pointer rests on it.
func TestTooltips(t *testing.T) {
	a := start(t, "adwaita", tela.Light, "Documents")
	a.Hover(btn("Up"))
	a.Advance(500e6)
	a.ExpectCount(telatest.ByRole("tooltip", "Up"), 1)
}

// TestZoom: KDE's slider scales the icons of the list.
func TestZoom(t *testing.T) {
	a := start(t, "breeze", tela.Light, "Documents")
	slider := telatest.ByRole("slider", "Zoom")
	a.Focus(slider)
	for range 5 {
		a.Press("PageUp")
	}
	shot := a.Screenshot()
	b := start(t, "breeze", tela.Light, "Documents").Screenshot()
	if same(shot, b) {
		t.Error("the zoom did not change the screen")
	}
}
