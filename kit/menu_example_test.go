package kit_test

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func ExampleDropdownMenu() {
	t := &exampleT{}
	defer t.close()
	dark := true
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.DropdownMenu[string]{
			Trigger: kit.Button{Variant: kit.Outline, Children: h.Text("Account")},
			Align:   "start",
			Items: []kit.MenuItem[string]{
				{Kind: kit.MenuLabel, Label: "My account"},
				{Kind: kit.MenuSeparator},
				{Key: "profile", Label: "Profile", Shortcut: "⇧⌘P"},
				{Kind: kit.MenuCheckbox, Key: "dark", Label: "Dark mode", Checked: &dark,
					OnCheckedChange: func(on bool) { dark = on }},
				{Kind: kit.MenuSeparator},
				{Key: "logout", Label: "Log out", Variant: kit.Destructive},
			},
			OnSelect: func(key string) { fmt.Println("chose", key) },
		})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("button", "Account"))
	app.Click(telatest.ByRole("menuitem", "Profile"))
	app.Click(telatest.ByRole("button", "Account"))
	app.Click(telatest.ByRole("menuitemcheckbox", "Dark mode"))
	fmt.Println("dark:", dark)
	// Output:
	// chose profile
	// dark: false
}

func ExampleMenubar() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.Menubar[string]{
			AriaLabel: "Application",
			Menus: []kit.MenubarMenu[string]{
				{Key: "file", Label: "File", Items: []kit.MenuItem[string]{
					{Key: "new", Label: "New Tab", Shortcut: "⌘T"},
					{Kind: kit.MenuSeparator},
					{Key: "print", Label: "Print...", Shortcut: "⌘P"},
				}},
				{Key: "edit", Label: "Edit", Items: []kit.MenuItem[string]{
					{Key: "undo", Label: "Undo", Shortcut: "⌘Z"},
				}},
			},
			OnSelect: func(menu, item string) { fmt.Println(menu, item) },
		})
	}
	app := telatest.MountFunc(t, page)
	app.Click(telatest.ByRole("menuitem", "File"))
	// With a menu open, hovering another button opens its menu.
	app.Hover(telatest.ByRole("menuitem", "Edit"))
	app.Click(telatest.ByRole("menuitem", "Undo"))
	// Output: edit undo
}

func ExampleContextMenu() {
	t := &exampleT{}
	defer t.close()
	page := func() tela.Node {
		return h.Div(h.Class("p-6"), kit.ContextMenu[string]{
			AriaLabel: "Actions",
			Class:     "flex h-40 w-72 items-center justify-center rounded-md border border-dashed text-sm",
			Children:  h.Button(h.Text("Right click here")),
			Items: []kit.MenuItem[string]{
				{Key: "back", Label: "Back", Shortcut: "⌘["},
				{Key: "reload", Label: "Reload", Shortcut: "⌘R"},
			},
			OnSelect: func(key string) { fmt.Println("chose", key) },
		})
	}
	app := telatest.MountFunc(t, page)
	// Shift+F10 opens the menu from the keyboard; a right click opens it at
	// the pointer.
	app.Focus(telatest.ByRole("button", "Right click here"))
	app.Press("Shift+F10")
	app.Press("ArrowDown")
	app.Press("Enter")
	// Output: chose reload
}
