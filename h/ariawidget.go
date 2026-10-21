package h

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// The ARIA props of widget patterns (React's aria-* attributes): what a
// button opens, which element it controls, the active option of a
// listbox, a modal dialog. Package aria's hooks set them for you; they
// change nothing on screen and the aria snapshot does not print them
// (Playwright's does not either), but assistive technologies read them.

// AriaHasPopup sets the kind of popup an element opens: "menu",
// "listbox", "tree", "grid" or "dialog" ("true" means "menu", "false"
// none). An unknown value panics in dev mode with the closest valid one.
//
//	Button(AriaHasPopup("menu"), AriaExpanded(open), OnClick(toggle), Text("Options"))
func AriaHasPopup(kind string) tela.Prop { return core.AriaHasPopup(kind) }

// AriaControls names the element whose content or visibility this one
// controls, attached to ref with Ref: the menu of a menu button, the
// panel of a tab, the listbox of a combobox.
//
//	menu := tela.UseElementRef()
//	Button(AriaControls(menu), AriaExpanded(open), Text("File")),
//	If(open, Div(Ref(menu), Role("menu"), items))
func AriaControls(ref tela.ElementRef) tela.Prop { return core.AriaControls(ref) }

// AriaActiveDescendant names the active descendant of a composite widget
// that keeps the focus itself, attached to ref with Ref: the option a
// combobox's arrow keys moved to while the focus stays in its input.
//
//	Input(Role("combobox"), AriaActiveDescendant(active), AriaControls(list))
func AriaActiveDescendant(ref tela.ElementRef) tela.Prop { return core.AriaActiveDescendant(ref) }

// AriaModal marks a dialog as modal: the rest of the window is not
// available while it is open. It only describes the dialog; package
// aria's UseDialog also makes the outside inert.
//
//	Div(Role("dialog"), AriaModal(true), AriaLabelledBy(title), ...)
func AriaModal(modal bool) tela.Prop { return core.AriaModal(modal) }

// AriaMultiSelectable marks a listbox, grid, tablist or tree in which
// several items can be selected.
//
//	Div(Role("listbox"), AriaMultiSelectable(true), options)
func AriaMultiSelectable(multi bool) tela.Prop { return core.AriaMultiSelectable(multi) }

// AriaOrientation sets the orientation of a listbox, menu, tablist,
// toolbar, slider or separator: "horizontal", "vertical" or "undefined".
// An unknown value panics in dev mode.
//
//	Div(Role("tablist"), AriaOrientation("vertical"), tabs)
func AriaOrientation(orientation string) tela.Prop { return core.AriaOrientation(orientation) }

// AriaAutoComplete describes the completion a combobox or textbox offers:
// "inline" (the rest of the word is typed ahead), "list" (a list of
// suggestions), "both" or "none". An unknown value panics in dev mode.
//
//	Input(Role("combobox"), AriaAutoComplete("list"), AriaExpanded(open))
func AriaAutoComplete(mode string) tela.Prop { return core.AriaAutoComplete(mode) }
