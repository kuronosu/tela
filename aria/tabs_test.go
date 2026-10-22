package aria_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

var tabNames = []string{"general", "privacy", "billing", "advanced"}

// tabsPage is a tab list with a panel for the selected tab.
func tabsPage(o aria.TabsOptions[string]) func() tela.Node {
	return func() tela.Node {
		o.Keys, o.AriaLabel = tabNames, "Settings"
		tabs := aria.UseTabs(o)
		sel, _ := tabs.Selected.First()
		cls := "flex"
		if o.Orientation == "vertical" {
			cls = "flex flex-col"
		}
		return Div(Class("flex"),
			Div(tabs.Props, Class(cls),
				Range(len(tabNames), func(i int) tela.Node {
					k := tabNames[i]
					return Div(tabs.TabProps(k), Key(k), Class("h-8 w-24 aria-selected:border-b-2 aria-disabled:opacity-50"), Text(capital(k)))
				})),
			Div(tabs.PanelProps(sel), Key("panel"), Class("p-4"), Textf("%s panel", sel)))
	}
}

func tab(name string) telatest.Locator { return telatest.ByRole("tab", name) }

func TestTabsAutomaticActivation(t *testing.T) {
	app := telatest.MountFunc(t, tabsPage(aria.TabsOptions[string]{}), telatest.Size(500, 300))
	// The first tab is selected by default, and the only tab stop.
	app.Expect(tab("General"), telatest.Selected(true))
	app.ExpectText("general panel")
	app.Press("Tab")
	app.ExpectFocused(tab("General"))
	app.Press("ArrowRight")
	app.ExpectFocused(tab("Privacy"))
	app.Expect(tab("Privacy"), telatest.Selected(true))
	app.ExpectText("privacy panel")
	app.Press("End")
	app.ExpectFocused(tab("Advanced"))
	app.ExpectText("advanced panel")
	// The arrows go around the ends.
	app.Press("ArrowRight")
	app.ExpectFocused(tab("General"))
	app.Press("ArrowLeft")
	app.ExpectFocused(tab("Advanced"))
	app.Press("Home")
	app.ExpectText("general panel")
	// ArrowUp and ArrowDown do nothing in a horizontal list; Tab goes
	// from the selected tab to its panel.
	app.Press("ArrowDown")
	app.ExpectFocused(tab("General"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("tabpanel", "General"))
	// A click selects.
	app.Click(tab("Billing"))
	app.ExpectText("billing panel")
	app.Expect(tab("Billing"), telatest.Selected(true))
	app.Expect(tab("General"), telatest.Selected(false))
}

func TestTabsManualActivation(t *testing.T) {
	o := aria.TabsOptions[string]{KeyboardActivation: "manual"}
	app := telatest.MountFunc(t, tabsPage(o), telatest.Size(500, 300))
	app.Press("Tab")
	app.Press("ArrowRight")
	app.Press("ArrowRight")
	app.ExpectFocused(tab("Billing"))
	app.ExpectText("general panel") // the arrows only move the focus
	app.Expect(tab("General"), telatest.Selected(true))
	app.Press("Enter")
	app.ExpectText("billing panel")
	app.Expect(tab("Billing"), telatest.Selected(true))
	app.Press("ArrowLeft")
	app.Press("Space")
	app.ExpectText("privacy panel")
	// Only the selected tab is a tab stop: from a tab that has the focus
	// without being selected, Tab goes on to the panel and Shift+Tab back
	// to the selected tab.
	app.Press("ArrowRight")
	app.ExpectFocused(tab("Billing"))
	app.Press("Tab")
	app.ExpectFocused(telatest.ByRole("tabpanel", "Privacy"))
	app.Press("Shift+Tab")
	app.ExpectFocused(tab("Privacy"))
}

func TestTabsDisabled(t *testing.T) {
	o := aria.TabsOptions[string]{DisabledKeys: []string{"privacy", "advanced"}}
	app := telatest.MountFunc(t, tabsPage(o), telatest.Size(500, 300))
	app.Press("Tab")
	app.Press("ArrowRight")
	app.ExpectFocused(tab("Billing"))
	app.Press("ArrowRight") // advanced is disabled: around to the first
	app.ExpectFocused(tab("General"))
	app.Expect(tab("Privacy"), telatest.Disabled(true))
	app.ExpectAria(`
		- tablist "Settings":
		  - tab "General" [selected]
		  - tab "Privacy" [disabled]
		  - tab "Billing"
		  - tab "Advanced" [disabled]
		- tabpanel "General": general panel
	`)
}

// The first enabled tab is selected when the given one is disabled.
func TestTabsDefaultSkipsDisabled(t *testing.T) {
	o := aria.TabsOptions[string]{DisabledKeys: []string{"general"}}
	app := telatest.MountFunc(t, tabsPage(o), telatest.Size(500, 300))
	app.Expect(tab("Privacy"), telatest.Selected(true))
	app.ExpectText("privacy panel")
}

func TestTabsVertical(t *testing.T) {
	app := telatest.MountFunc(t, tabsPage(aria.TabsOptions[string]{Orientation: "vertical"}), telatest.Size(500, 300))
	app.Press("Tab")
	app.Press("ArrowRight") // does nothing
	app.ExpectFocused(tab("General"))
	app.Press("ArrowDown")
	app.ExpectFocused(tab("Privacy"))
	app.ExpectText("privacy panel")
	app.Press("ArrowUp")
	app.Press("ArrowUp") // wraps
	app.ExpectFocused(tab("Advanced"))
}

func TestTabsRTL(t *testing.T) {
	app := telatest.MountFunc(t, tabsPage(aria.TabsOptions[string]{Direction: "rtl"}), telatest.Size(500, 300))
	app.Press("Tab")
	app.Press("ArrowLeft") // forward in a right-to-left list
	app.ExpectFocused(tab("Privacy"))
	app.Press("ArrowRight")
	app.ExpectFocused(tab("General"))
}

func TestTabsPanelLinking(t *testing.T) {
	app := telatest.MountFunc(t, tabsPage(aria.TabsOptions[string]{DefaultSelected: aria.SelectionOf("billing")}), telatest.Size(500, 300))
	// The panel is named by its tab, and the tab controls the panel.
	app.ExpectAria(`
		- tablist "Settings":
		  - tab "General"
		  - tab "Privacy"
		  - tab "Billing" [selected]
		  - tab "Advanced"
		- tabpanel "Billing": billing panel
	`)
	n := app.Count(telatest.ByRole("tabpanel", "Billing"))
	if n != 1 {
		t.Errorf("tabpanel count %d", n)
	}
}

func TestTabsControlled(t *testing.T) {
	var ev events
	page := func() tela.Node {
		sel, setSel := tela.UseState(aria.SelectionOf("privacy"))
		tabs := aria.UseTabs(aria.TabsOptions[string]{
			Keys: tabNames, AriaLabel: "Settings", Selected: sel,
			OnSelectionChange: func(s aria.Selection[string]) {
				k, _ := s.First()
				ev.add(k)
				if k != "advanced" { // the app refuses this one
					setSel.Set(s)
				}
			}})
		k, _ := sel.First()
		return Div(
			Div(tabs.Props, Range(len(tabNames), func(i int) tela.Node {
				return Div(tabs.TabProps(tabNames[i]), Key(tabNames[i]), Class("h-8"), Text(capital(tabNames[i])))
			})),
			Textf("selected: %s", k))
	}
	app := telatest.MountFunc(t, page, telatest.Size(500, 300))
	app.ExpectText("selected: privacy")
	app.Click(tab("Billing"))
	app.ExpectText("selected: billing")
	app.Click(tab("Advanced"))
	app.ExpectText("selected: billing")
	app.Expect(tab("Advanced"), telatest.Selected(false))
	if got := ev.take(); got != "billing advanced" {
		t.Errorf("events %q", got)
	}
}

func ExampleUseTabs() {
	names := []string{"account", "password"}
	page := func() tela.Node {
		tabs := aria.UseTabs(aria.TabsOptions[string]{Keys: names, AriaLabel: "Settings"})
		sel, _ := tabs.Selected.First()
		return Div(
			Div(tabs.Props, Class("flex gap-1"),
				Range(len(names), func(i int) tela.Node {
					k := names[i]
					return Div(tabs.TabProps(k), Key(k), Class("px-3 py-2 aria-selected:border-b-2"), Text(k))
				})),
			Div(tabs.PanelProps(sel), Class("p-4"), Textf("%s settings", sel)))
	}
	app := telatest.MountFunc(&exampleTB{}, page)
	app.Press("Tab")
	app.Press("ArrowRight")
	fmt.Println(app.AriaSnapshot())
	// Output:
	// - tablist "Settings":
	//   - tab "account"
	//   - tab "password" [selected]
	// - tabpanel "password": password settings
}

// A tab list has no typeahead (React Aria's TabsKeyboardDelegate has no
// getKeyForSearch): typing a letter moves neither the focus nor the
// selection, so the panel never disagrees with the focused tab.
func TestTabsHaveNoTypeahead(t *testing.T) {
	for _, act := range []string{"automatic", "manual"} {
		t.Run(act, func(t *testing.T) {
			app := telatest.MountFunc(t, tabsPage(aria.TabsOptions[string]{KeyboardActivation: act}), telatest.Size(500, 300))
			app.Press("Tab")
			app.ExpectFocused(tab("General"))
			app.Press("p")
			app.Press("b")
			app.ExpectFocused(tab("General"))
			app.Expect(tab("General"), telatest.Selected(true))
			app.Expect(tab("Privacy"), telatest.Selected(false))
			app.ExpectText("general panel")
		})
	}
}
