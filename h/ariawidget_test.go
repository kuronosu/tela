package h_test

import (
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
)

// combobox uses the widget props and Inert through the public API.
func combobox() tela.Node {
	list, active := tela.UseElementRef(), tela.UseElementRef()
	return Div(
		Div(Role("combobox"), AriaLabel("Fruit"), TabIndex(0), AriaHasPopup("listbox"), AriaExpanded(true),
			AriaControls(list), AriaActiveDescendant(active), AriaAutoComplete("list")),
		Div(Ref(list), Role("listbox"), AriaMultiSelectable(true), AriaOrientation("vertical"),
			Div(Ref(active), Role("option"), Text("Apple")),
			Div(Role("option"), Text("Pear"))),
		Div(Role("dialog"), AriaModal(true), AriaLabel("Help")),
		Div(Inert(true), Button(Text("Behind"))),
	)
}

func TestWidgetProps(t *testing.T) {
	tr := core.NewTree(core.Config{})
	tr.SetViewport(gfx.Size{W: 600, H: 400})
	tr.Mount(tela.C(combobox))
	tr.Frame()
	if got, want := tr.AriaSnapshot(), "- combobox \"Fruit\" [expanded]\n- listbox:\n  - option \"Apple\"\n  - option \"Pear\"\n- dialog \"Help\""; got != want {
		t.Errorf("aria snapshot:\n%s\nwant:\n%s", got, want)
	}
	var cb, lb, apple, dlg *semantics.Node
	tr.Semantics().Walk(func(n *semantics.Node) bool {
		switch {
		case n.Role == "combobox":
			cb = n
		case n.Role == "listbox":
			lb = n
		case n.Role == "option" && n.Name == "Apple":
			apple = n
		case n.Role == "dialog":
			dlg = n
		}
		return true
	})
	if cb == nil || lb == nil || apple == nil || dlg == nil {
		t.Fatal("missing nodes")
	}
	if cb.HasPopup != "listbox" || cb.Controls != lb.ID || cb.ActiveDescendant != apple.ID || cb.AutoComplete != "list" {
		t.Errorf("combobox %+v", cb)
	}
	if !lb.MultiSelectable || lb.Orientation != "vertical" || !dlg.Modal {
		t.Errorf("listbox %+v, dialog %+v", lb, dlg)
	}
}
