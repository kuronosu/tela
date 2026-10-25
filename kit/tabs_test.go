package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

// shadcn/ui's Tabs, TabsList, TabsTrigger and TabsContent (new-york-v4/ui/tabs.tsx
// at kit.ShadcnCommit), verbatim.
const (
	shadcnTabs = "group/tabs flex gap-2 data-[orientation=horizontal]:flex-col"

	shadcnTabsList        = "group/tabs-list inline-flex w-fit items-center justify-center rounded-lg p-[3px] text-muted-foreground group-data-[orientation=horizontal]/tabs:h-9 group-data-[orientation=vertical]/tabs:h-fit group-data-[orientation=vertical]/tabs:flex-col data-[variant=line]:rounded-none"
	shadcnTabsListDefault = "bg-muted"
	shadcnTabsListLine    = "gap-1 bg-transparent"

	shadcnTabsTrigger = "relative inline-flex h-[calc(100%-1px)] flex-1 items-center justify-center gap-1.5 rounded-md border border-transparent px-2 py-1 text-sm font-medium whitespace-nowrap text-foreground/60 transition-all group-data-[orientation=vertical]/tabs:w-full group-data-[orientation=vertical]/tabs:justify-start hover:text-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-1 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-50 group-data-[variant=default]/tabs-list:data-[state=active]:shadow-sm group-data-[variant=line]/tabs-list:data-[state=active]:shadow-none dark:text-muted-foreground dark:hover:text-foreground [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4" +
		" group-data-[variant=line]/tabs-list:bg-transparent group-data-[variant=line]/tabs-list:data-[state=active]:bg-transparent dark:group-data-[variant=line]/tabs-list:data-[state=active]:border-transparent dark:group-data-[variant=line]/tabs-list:data-[state=active]:bg-transparent" +
		" data-[state=active]:bg-background data-[state=active]:text-foreground dark:data-[state=active]:border-input dark:data-[state=active]:bg-input/30 dark:data-[state=active]:text-foreground" +
		" after:absolute after:bg-foreground after:opacity-0 after:transition-opacity group-data-[orientation=horizontal]/tabs:after:inset-x-0 group-data-[orientation=horizontal]/tabs:after:bottom-[-5px] group-data-[orientation=horizontal]/tabs:after:h-0.5 group-data-[orientation=vertical]/tabs:after:inset-y-0 group-data-[orientation=vertical]/tabs:after:-right-1 group-data-[orientation=vertical]/tabs:after:w-0.5 group-data-[variant=line]/tabs-list:data-[state=active]:after:opacity-100"

	shadcnTabsContent = "flex-1 outline-none"
)

// tabsHTML is the markup shadcn's Tabs render for tabs (icons are Lucide
// SVGs), with the tab at index selected and panel as its content.
func tabsHTML(tabs []kit.Tab[string], selected int, orient kit.Orientation, variant kit.Variant, panel string) string {
	o := "horizontal"
	if orient == kit.Vertical {
		o = "vertical"
	}
	v := "default"
	list := shadcnTabsListDefault
	if variant == kit.TabsLine {
		v, list = "line", shadcnTabsListLine
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, `<div data-orientation=%q data-slot="tabs" class=%q>`, o, shadcnTabs)
	fmt.Fprintf(&sb, `<div role="tablist" aria-orientation=%q data-slot="tabs-list" data-variant=%q class=%q>`, o, v, cn(shadcnTabsList, list))
	for i, t := range tabs {
		state := "inactive"
		if i == selected {
			state = "active"
		}
		extra := ""
		if t.Disabled {
			extra = ` disabled data-disabled=""`
		}
		inner := t.Label
		if t.Icon != (tela.Icon{}) {
			inner = svgHTML(t.Icon) + t.Label
		}
		fmt.Fprintf(&sb, `<button type="button" role="tab" aria-selected=%t data-state=%q data-slot="tabs-trigger"%s class=%q>%s</button>`,
			i == selected, state, extra, cn(shadcnTabsTrigger), inner)
	}
	sb.WriteString(`</div>`)
	if panel != "" {
		fmt.Fprintf(&sb, `<div data-state="active" role="tabpanel" data-slot="tabs-content" tabindex="0" class=%q>%s</div>`, shadcnTabsContent, panel)
	}
	sb.WriteString(`</div>`)
	return sb.String()
}

const (
	accountText  = "Make changes to your account here."
	passwordText = "Change your password here."
)

func demoTabs() []kit.Tab[string] {
	return []kit.Tab[string]{
		{Value: "account", Label: "Account", Content: h.P(h.Class("text-sm text-muted-foreground"), h.Text(accountText))},
		{Value: "password", Label: "Password", Content: h.P(h.Class("text-sm text-muted-foreground"), h.Text(passwordText))},
		{Value: "team", Label: "Team", Content: h.P(h.Class("text-sm text-muted-foreground"), h.Text("Invite people."))},
	}
}

const tabsPanelHTML = `<p class="text-sm text-muted-foreground">%s</p>`

func tabsCell(name string, t kit.Tabs[string], selected int, width int) cell {
	t.Props = h.TestID(name + "-tabs")
	if selected >= 0 && t.Value == nil && t.DefaultValue == nil {
		t.DefaultValue = new(t.Tabs[selected].Value)
	}
	panel := ""
	if selected >= 0 && t.Tabs[selected].Content != nil {
		panel = fmt.Sprintf(tabsPanelHTML, map[string]string{"account": accountText, "password": passwordText, "team": "Invite people."}[t.Tabs[selected].Value])
	}
	html := strings.Replace(tabsHTML(t.Tabs, selected, t.Orientation, t.Variant, panel), `data-slot="tabs"`, `data-probe="`+name+`-tabs" data-slot="tabs"`, 1)
	return cell{name: name, width: width, probes: []string{name + "-tabs"}, tela: t, html: html}
}

func tabsSheet() *sheet {
	with := func(f func([]kit.Tab[string])) []kit.Tab[string] {
		ts := demoTabs()
		f(ts)
		return ts
	}
	withIcons := with(func(ts []kit.Tab[string]) { ts[0].Icon, ts[1].Icon = iconMail, iconCheck })
	disabled := with(func(ts []kit.Tab[string]) { ts[1].Disabled = true })
	return &sheet{name: "tabs", cells: []cell{
		tabsCell("default", kit.Tabs[string]{Tabs: demoTabs()}, 0, 320),
		tabsCell("second", kit.Tabs[string]{Tabs: demoTabs()}, 1, 320),
		tabsCell("line", kit.Tabs[string]{Tabs: demoTabs(), Variant: kit.TabsLine}, 0, 320),
		tabsCell("icons", kit.Tabs[string]{Tabs: withIcons}, 1, 320),
		tabsCell("line-icons", kit.Tabs[string]{Tabs: withIcons, Variant: kit.TabsLine}, 0, 320),
		tabsCell("disabled", kit.Tabs[string]{Tabs: disabled}, 0, 320),
		tabsCell("vertical", kit.Tabs[string]{Tabs: demoTabs(), Orientation: kit.Vertical}, 0, 400),
		tabsCell("vertical-line", kit.Tabs[string]{Tabs: demoTabs(), Orientation: kit.Vertical, Variant: kit.TabsLine}, 1, 400),
		tabsCell("no-panel", kit.Tabs[string]{Tabs: with(func(ts []kit.Tab[string]) {
			for i := range ts {
				ts[i].Content = nil
			}
		})}, 0, 320),
	}}
}

func TestTabsGolden(t *testing.T)   { tabsSheet().golden(t) }
func TestChromiumTabs(t *testing.T) { tabsSheet().chromium(t) }

// tabsApp mounts tabs under a Page and returns the app.
func selectedTab(app *telatest.App, name string) {
	app.Expect(telatest.ByRole("tab", name), telatest.Selected(true))
}

func TestTabsKeyboard(t *testing.T) {
	ts := demoTabs()
	ts[1].Disabled = true
	app := telatest.Mount(t, kit.Tabs[string]{Tabs: ts, AriaLabel: "Settings"})
	selectedTab(app, "Account")
	app.ExpectText(accountText)
	app.Focus(telatest.ByRole("tab", "Account"))
	// The arrow skips the disabled tab, and the focused tab is selected.
	app.Press("ArrowRight")
	selectedTab(app, "Team")
	app.ExpectText("Invite people.")
	app.ExpectNoText(accountText)
	app.Press("ArrowRight") // wraps
	selectedTab(app, "Account")
	app.Press("End")
	selectedTab(app, "Team")
	app.Press("Home")
	selectedTab(app, "Account")
	app.Expect(telatest.ByRole("tab", "Password"), telatest.Disabled(true))
	// Tab goes from the selected tab to its panel.
	app.Press("Tab")
	app.Expect(telatest.ByRole("tabpanel", "Account"), telatest.Focused(true))
}

func TestTabsManualActivation(t *testing.T) {
	app := telatest.Mount(t, kit.Tabs[string]{Tabs: demoTabs(), ManualActivation: true})
	app.Focus(telatest.ByRole("tab", "Account"))
	app.Press("ArrowRight")
	selectedTab(app, "Account")
	app.Press("Enter")
	selectedTab(app, "Password")
	app.ExpectText(passwordText)
}

func TestTabsPointerAndUncontrolled(t *testing.T) {
	var seen []string
	app := telatest.Mount(t, kit.Tabs[string]{Tabs: demoTabs(), DefaultValue: new("password"), OnValueChange: func(k string) { seen = append(seen, k) }})
	selectedTab(app, "Password")
	app.ExpectText(passwordText)
	app.Click(telatest.ByRole("tab", "Team"))
	selectedTab(app, "Team")
	if len(seen) != 1 || seen[0] != "team" {
		t.Errorf("OnValueChange saw %v, want [team]", seen)
	}
}

// controlledTabs keeps the selected tab in its own state and refuses "team".
type controlledTabs struct{ log *[]string }

func (c controlledTabs) Build() tela.Node {
	v, setV := tela.UseState("account")
	return h.Div(
		kit.Tabs[string]{Tabs: demoTabs(), Value: &v, OnValueChange: func(k string) {
			*c.log = append(*c.log, k)
			if k != "team" {
				setV.Set(k)
			}
		}},
		kit.Button{Variant: kit.Outline, OnClick: func() { setV.Set("password") }, Children: h.Text("Go to password")},
	)
}

func TestTabsControlled(t *testing.T) {
	var log []string
	app := telatest.Mount(t, controlledTabs{&log})
	selectedTab(app, "Account")
	app.Click(telatest.ByRole("tab", "Team"))
	// The request was refused: the tab stays where the owner's state is.
	selectedTab(app, "Account")
	app.Click(telatest.ByRole("tab", "Password"))
	selectedTab(app, "Password")
	if strings.Join(log, ",") != "team,password" {
		t.Errorf("OnValueChange saw %v, want team then password", log)
	}
	// The state changes from outside.
	app.Click(telatest.ByRole("button", "Go to password"))
	selectedTab(app, "Password")
	app.Click(telatest.ByRole("tab", "Account"))
	app.Click(telatest.ByRole("button", "Go to password"))
	selectedTab(app, "Password")
}

func TestTabsVerticalUsesUpAndDown(t *testing.T) {
	app := telatest.Mount(t, kit.Tabs[string]{Tabs: demoTabs(), Orientation: kit.Vertical})
	app.Focus(telatest.ByRole("tab", "Account"))
	app.Press("ArrowDown")
	selectedTab(app, "Password")
	app.Press("ArrowRight") // not an arrow of a vertical list
	selectedTab(app, "Password")
	app.Press("ArrowUp")
	selectedTab(app, "Account")
}

func TestTabsLineDrawsAnIndicator(t *testing.T) {
	app := telatest.Mount(t, kit.Tabs[string]{Tabs: demoTabs(), Variant: kit.TabsLine})
	app.ExpectCount(telatest.ByClass("bg-foreground"), 3)
	plain := telatest.Mount(t, kit.Tabs[string]{Tabs: demoTabs()})
	plain.ExpectCount(telatest.ByClass("bg-foreground"), 0)
}

func TestTabsAriaSnapshot(t *testing.T) {
	snap, err := telatest.AriaSnapshot(kit.Tabs[string]{Tabs: demoTabs(), AriaLabel: "Settings", DefaultValue: new("password")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`tablist "Settings"`, `tab "Account"`, `tab "Password" [selected]`, `tabpanel "Password"`, passwordText} {
		if !strings.Contains(snap, want) {
			t.Errorf("the aria snapshot lacks %q:\n%s", want, snap)
		}
	}
}

func TestTabsStyleAndClass(t *testing.T) {
	s := kit.DefaultStyle()
	s.Tabs.ListVariants["pill"] = "bg-primary"
	s.Tabs.TriggerVariants["pill"] = "rounded-full"
	app := telatest.Mount(t, kit.StyleProvider{Style: &s, Children: kit.Tabs[string]{Tabs: demoTabs(), Variant: "pill", Class: "w-96", ContentClass: "p-4"}})
	app.ExpectCount(telatest.ByClass("bg-primary", "w-fit"), 1)
	app.ExpectCount(telatest.ByClass("w-96"), 1)
	app.ExpectCount(telatest.ByClass("p-4", "flex-1"), 1)
	if _, err := telatest.Render(kit.Tabs[string]{Tabs: demoTabs(), Variant: "pill"}); err == nil || !strings.Contains(err.Error(), `Tabs.Variant "pill" is not defined`) {
		t.Errorf("error %v, want the undefined variant reported", err)
	}
}

var _ tela.Component = kit.Tabs[string]{}

func ExampleTabs() {
	t := &exampleT{}
	defer t.close()
	page := kit.Tabs[string]{
		DefaultValue: new("account"),
		Tabs: []kit.Tab[string]{
			{Value: "account", Label: "Account", Content: h.Text("Make changes to your account.")},
			{Value: "password", Label: "Password", Content: h.Text("Change your password.")},
		},
	}
	app := telatest.Mount(t, page)
	app.Click(telatest.ByRole("tab", "Password"))
	app.ExpectText("Change your password.")
	fmt.Println("password tab shown")
	// Output: password tab shown
}
