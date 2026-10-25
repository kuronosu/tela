package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// The menus of the kit are drawn in a Portal, so a Chromium page cannot
// hold them in a cell next to shadcn's markup. These functions, for the
// tests of package kit_test only, return the panel of a menu as it looks
// open, as an ordinary component in the page's flow: the same recipe, the
// same renderer and the same element tree as the real menu, without its
// positioning, hooks of the trigger and portal. (Like overlay_export_test.go
// for the overlays.)

// staticMenu is the panel of a menu with the given items.
func staticMenu(component string, style func(*Style) *MenuStyle, items []MenuItem[string], class string, props tela.Child) tela.Component {
	return tela.C(func() tela.Node {
		st := style(UseStyle())
		store := useMenuStore[string]()
		flat := flattenMenu(component, items)
		menu := aria.UseMenu(aria.MenuOptions[string]{
			Keys: flat.keys, DisabledKeys: flat.disabled, TextValue: func(k string) string { return flat.text[k] },
		})
		r := menuRenderer[string]{component: component, st: st, menu: menu, store: store}
		kids := []tela.Child{cls(st.Content, st.Sides["bottom"], class), props}
		return h.Div(append(kids, r.entries(items)...)...)
	})
}

// StaticDropdownMenu is the panel of d, as an open dropdown menu draws it.
func StaticDropdownMenu(d DropdownMenu[string]) tela.Component {
	return staticMenu("DropdownMenu", func(s *Style) *MenuStyle { return &s.DropdownMenu }, d.Items, d.Class, d.Props)
}

// StaticContextMenu is the panel of c, as an open context menu draws it,
// with props on the panel.
func StaticContextMenu(c ContextMenu[string], props tela.Child) tela.Component {
	return staticMenu("ContextMenu", func(s *Style) *MenuStyle { return &s.ContextMenu }, c.Items, c.ContentClass, props)
}

// StaticMenubarMenu is the panel of the menu with key k of m, as an open
// menu bar draws it, with props on the panel.
func StaticMenubarMenu(m Menubar[string], k string, props tela.Child) tela.Component {
	var items []MenuItem[string]
	for _, mm := range m.Menus {
		if mm.Key == k {
			items = mm.Items
		}
	}
	return staticMenu("Menubar", func(s *Style) *MenuStyle { return &s.Menubar.Menu }, items, m.ContentClass, props)
}
