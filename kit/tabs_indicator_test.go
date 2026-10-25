package kit_test

import (
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// An Indicator slot is mounted inside the selected tab only, so it follows
// the selection (and a LayoutID on it slides).
func TestTabsIndicatorSlot(t *testing.T) {
	underline := h.Div(h.Class("absolute inset-x-0 -bottom-px h-0.5 bg-primary"), h.LayoutID("underline"))
	app := telatest.Mount(t, kit.Tabs[string]{Tabs: demoTabs(), Variant: kit.TabsLine, Indicator: underline})
	line := telatest.ByClass("h-0.5", "bg-primary")
	app.ExpectCount(line, 1)
	app.ExpectCount(telatest.ByClass("bg-foreground"), 0) // replaces the recipe's underline
	inside := func(tab string) {
		t.Helper()
		l, r := app.Bounds(line), app.Bounds(telatest.ByRole("tab", tab))
		if l.X < r.X || l.Right() > r.Right() {
			t.Errorf("the indicator spans %v..%v, outside the %s tab (%v..%v)", l.X, l.Right(), tab, r.X, r.Right())
		}
	}
	inside("Account")
	app.Click(telatest.ByRole("tab", "Password"))
	app.ExpectCount(line, 1)
	inside("Password")
	// On any variant, not only the line one.
	plain := telatest.Mount(t, kit.Tabs[string]{Tabs: demoTabs(), Indicator: underline})
	plain.ExpectCount(line, 1)
}
