package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// The pages of Select, Slider and Tabs (Progress and Table have files of their own).

func selectPage() tela.Node {
	fruit, setFruit := tela.UseState("")
	theme, setTheme := tela.UseState("system")
	return grid(
		demo("Basic", "Press, Enter, Space or the arrows open the list; typing jumps to an option; on the closed trigger typing chooses.", col,
			kit.Select[string]{Placeholder: "Select a fruit", Class: "w-48", Value: &fruit, OnValueChange: setFruit.Set, AriaLabel: "Fruit",
				Items: []kit.SelectItem[string]{
					{Value: "apple", Label: "Apple"}, {Value: "banana", Label: "Banana"}, {Value: "blueberry", Label: "Blueberry"},
					{Value: "grapes", Label: "Grapes", Disabled: true}, {Value: "pineapple", Label: "Pineapple"},
				}},
			readout(map[bool]string{true: "Nothing chosen yet.", false: "Chosen: " + fruit}[fruit == ""])),
		demo("Groups", "Consecutive items with the same Group are drawn under its label, with a separator between groups.", "",
			kit.Select[string]{Placeholder: "Select a timezone", Class: "w-64", AriaLabel: "Timezone", Items: []kit.SelectItem[string]{
				{Group: "North America", Value: "est", Label: "Eastern Standard Time (EST)"},
				{Group: "North America", Value: "cst", Label: "Central Standard Time (CST)"},
				{Group: "North America", Value: "pst", Label: "Pacific Standard Time (PST)"},
				{Group: "Europe & Africa", Value: "gmt", Label: "Greenwich Mean Time (GMT)"},
				{Group: "Europe & Africa", Value: "cet", Label: "Central European Time (CET)"},
				{Group: "Asia", Value: "jst", Label: "Japan Standard Time (JST)"},
			}}),
		demo("Icons", "An Icon is drawn in the list and in the trigger.", col,
			kit.Select[string]{Class: "w-40", Value: &theme, OnValueChange: setTheme.Set, AriaLabel: "Theme", Items: []kit.SelectItem[string]{
				{Value: "light", Label: "Light", Icon: icons.Sun},
				{Value: "dark", Label: "Dark", Icon: icons.Moon},
				{Value: "system", Label: "System", Icon: icons.Settings},
			}}),
		demo("Sizes and states", "The default size (36 px) and Sm (32 px); disabled and invalid.", "",
			kit.Select[string]{Placeholder: "Default", Class: "w-36", AriaLabel: "Default", Items: someItems},
			kit.Select[string]{Placeholder: "Small", Size: kit.Sm, Class: "w-36", AriaLabel: "Small", Items: someItems},
			kit.Select[string]{Placeholder: "Disabled", Disabled: true, Class: "w-36", AriaLabel: "Disabled", Items: someItems},
			kit.Select[string]{Placeholder: "Invalid", Invalid: true, Class: "w-36", AriaLabel: "Invalid", Items: someItems}),
	)
}

var someItems = []kit.SelectItem[string]{{Value: "one", Label: "One"}, {Value: "two", Label: "Two"}}

func sliderPage() tela.Node {
	volume, setVolume := tela.UseState(40.0)
	price, setPrice := tela.UseState([]float64{200, 800})
	var bands []tela.Child
	for i, name := range []string{"Bass", "Mid", "Treble"} {
		bands = append(bands, Div(Key(name), Class("flex flex-col items-center gap-3"),
			kit.Slider{Orientation: kit.Vertical, Class: "h-44", DefaultValue: new(float64(30 + 25*i)), AriaLabel: name},
			Span(Class("text-xs text-muted-foreground"), Text(name))))
	}
	return grid(
		demo("Single value", "Dragging only repaints: the knob follows the pointer without the page being built on every move.", col,
			kit.Slider{Class: "max-w-sm", Value: &volume, OnValueChange: setVolume.Set, Step: 1, AriaLabel: "Volume"},
			readout(fmt.Sprintf("Volume: %.0f%%", volume))),
		demo("Range", "Values, with one per knob; the knobs never pass each other.", col,
			kit.Slider{Class: "max-w-sm", Values: &price, OnValuesChange: setPrice.Set, Min: 0, Max: 1000, Step: 10, MinStepsBetweenThumbs: 5,
				ThumbLabels: []string{"Minimum price", "Maximum price"}},
			readout(fmt.Sprintf("Price: $%.0f - $%.0f", price[0], price[1]))),
		demo("Steps and disabled", "Step 25 over 0 to 100; a disabled slider.", col,
			kit.Slider{Class: "max-w-sm", DefaultValue: new(50.0), Step: 25, AriaLabel: "In steps of 25"},
			kit.Slider{Class: "max-w-sm", DefaultValue: new(70.0), Disabled: true, AriaLabel: "Disabled"}),
		demo("Vertical", "A vertical slider has its Min at the bottom.", "flex flex-wrap items-end gap-10", Fragment(bands...)),
	)
}

func tabsPage() tela.Node {
	return grid(
		demo("Account", "A Tabs with a form in each panel, as in shadcn's example. Only the selected panel is built.", "",
			kit.Tabs[string]{DefaultValue: new("account"), Class: "w-full max-w-md", AriaLabel: "Settings", Tabs: []kit.Tab[string]{
				{Value: "account", Label: "Account", Content: kit.Card{
					TitleLevel: 2, Title: Text("Account"), Description: Text("Make changes to your account here."),
					Children: Div(Class("flex flex-col gap-4"),
						kit.Field{Label: "Name", Children: kit.Input{DefaultValue: "Mei Tanaka"}},
						kit.Field{Label: "Username", Children: kit.Input{DefaultValue: "@mei"}}),
					Footer: kit.Button{Children: Text("Save changes")}}},
				{Value: "password", Label: "Password", Content: kit.Card{
					TitleLevel: 2, Title: Text("Password"), Description: Text("Change your password here."),
					Children: Div(Class("flex flex-col gap-4"),
						kit.Field{Label: "Current password", Children: kit.Input{Type: "password"}},
						kit.Field{Label: "New password", Children: kit.Input{Type: "password"}}),
					Footer: kit.Button{Children: Text("Save password")}}},
			}}),
		demo("Line variant", "TabsLine draws no pill and underlines the selected tab; a tab can have an icon or be disabled.", col,
			kit.Tabs[string]{Variant: kit.TabsLine, DefaultValue: new("overview"), AriaLabel: "Project", Tabs: []kit.Tab[string]{
				{Value: "overview", Label: "Overview", Icon: icons.LayoutGrid, Content: muted("Everything about the project, at a glance.")},
				{Value: "activity", Label: "Activity", Icon: icons.Bell, Content: muted("Commits, comments and deployments.")},
				{Value: "settings", Label: "Settings", Icon: icons.Settings, Content: muted("Names, access and integrations.")},
				{Value: "billing", Label: "Billing", Icon: icons.CreditCard, Disabled: true, Content: muted("Not available on this plan.")},
			}}),
		demo("Vertical", "Orientation Vertical lays the list out beside the panel and moves with the up and down arrows.", "",
			kit.Tabs[string]{Orientation: kit.Vertical, DefaultValue: new("profile"), AriaLabel: "Preferences", Class: "w-full max-w-md", Tabs: []kit.Tab[string]{
				{Value: "profile", Label: "Profile", Content: muted("Your name, picture and bio.")},
				{Value: "security", Label: "Security", Content: muted("Password and two-factor authentication.")},
				{Value: "notifications", Label: "Notifications", Content: muted("What reaches your inbox.")},
			}}),
		demo("Manual activation", "ManualActivation makes the arrows move the focus only; Enter or Space selects.", col,
			kit.Tabs[string]{ManualActivation: true, DefaultValue: new("a"), AriaLabel: "Manual", Tabs: []kit.Tab[string]{
				{Value: "a", Label: "First", Content: muted("The first panel.")},
				{Value: "b", Label: "Second", Content: muted("The second panel.")},
				{Value: "c", Label: "Third", Content: muted("The third panel.")},
			}}),
	)
}
