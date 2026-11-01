package kit

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
)

// NativeMode says whether a menu is the OS's own or drawn by the kit
// (ADR-215). [ContextMenu] and [Menubar] have a Native field of this
// type; the zero value follows the app.
//
//	kit.ContextMenu[string]{Native: kit.NativeOn, ...}  // the OS's menu where there is one
//	kit.Menubar[string]{Native: kit.NativeOff, ...}     // drawn, whatever the app asks
type NativeMode uint8

const (
	// NativeAuto follows the app: native when tela.App.NativeMenus is set,
	// drawn otherwise (the default). The zero value.
	NativeAuto NativeMode = iota
	// NativeOn uses the OS's menu where the platform has one, whatever
	// the app asks. Where it has none (Linux, a menu bar on a frameless
	// Windows window) the menu is drawn.
	NativeOn
	// NativeOff always draws the menu in the kit's look, so a menu can
	// stay drawn while the app's others are native.
	NativeOff
)

func (n NativeMode) String() string {
	switch n {
	case NativeAuto:
		return "NativeAuto"
	case NativeOn:
		return "NativeOn"
	case NativeOff:
		return "NativeOff"
	}
	return fmt.Sprintf("NativeMode(%d)", uint8(n))
}

// useNative reports whether a menu with the setting n asks for the OS's
// menu: its own setting, else the app's. It is a hook. Whether the
// platform has one is only known when the menu is shown.
func useNative(component string, n NativeMode) bool {
	app := tela.UseNativeMenus()
	switch n {
	case NativeAuto:
		return app
	case NativeOn:
		return true
	case NativeOff:
		return false
	}
	if devMode() {
		panic(fmt.Sprintf("kit: %s has Native %v; use kit.NativeAuto (follow tela.App.NativeMenus), kit.NativeOn or kit.NativeOff", component, n))
	}
	return app
}

// nativeItems converts the items of a menu to the OS's: choose runs what
// choosing the item with the key does (menuPanel.choose). What the OS's
// menus cannot show is dropped:
//
//   - a plain item keeps its Label, Shortcut (as the accelerator, written
//     the usual way when [desktop.ParseKeys] reads it and as given
//     otherwise, such as "⌘T") and Disabled; Icon, Variant (a destructive
//     item looks like any other), Inset, KeepOpen (native menus always
//     close), Class are dropped;
//   - a checkbox item is a check item, checked as the menu's state says;
//   - the items of a radio group are check items too, only the chosen one
//     checked (desktop.MenuItem has no radio kind); a disabled group
//     disables them;
//   - a label becomes a disabled item with its text (the OS has no
//     headings); a separator stays one;
//   - a group is replaced by its items (its name is not shown), disabled
//     when the group is.
func nativeItems[K comparable](items []MenuItem[K], store *menuStore[K], choose func(K)) []desktop.MenuItem {
	var out []desktop.MenuItem
	var walk func(items []MenuItem[K], group *MenuItem[K], off bool)
	one := func(it MenuItem[K], group *MenuItem[K], off bool) desktop.MenuItem {
		k := it.Key
		n := desktop.MenuItem{
			Label:       it.Label,
			Accelerator: nativeAccelerator(it.Shortcut),
			Disabled:    it.Disabled || off,
			OnSelect:    func() { choose(k) },
		}
		switch {
		case group != nil:
			n.Checkbox, n.Checked = true, store.chosen(*group, k)
		case it.Kind == MenuCheckbox:
			n.Checkbox, n.Checked = true, store.isChecked(it)
		}
		return n
	}
	walk = func(items []MenuItem[K], group *MenuItem[K], off bool) {
		for i := range items {
			it := items[i]
			switch it.Kind {
			case MenuPlain, MenuCheckbox:
				out = append(out, one(it, nil, off))
			case MenuRadioGroup:
				g := it
				for _, ri := range it.Items {
					out = append(out, one(ri, &g, off || it.Disabled))
				}
			case MenuLabel:
				out = append(out, desktop.MenuItem{Label: it.Label, Disabled: true})
			case MenuSeparator:
				out = append(out, desktop.MenuItem{Separator: true})
			case MenuGroup:
				walk(it.Items, nil, off || it.Disabled)
			}
		}
	}
	walk(items, nil, false)
	return out
}

// nativeAccelerator returns the text of a shortcut for a native menu: the
// canonical form when desktop.ParseKeys reads it ("ctrl+c" is "Ctrl+C"),
// else the text as it is ("⌘T").
func nativeAccelerator(shortcut string) string {
	if shortcut == "" {
		return ""
	}
	if keys, err := desktop.ParseKeys(shortcut); err == nil {
		return keys
	}
	return shortcut
}
