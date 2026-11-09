package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/i18n"
	"github.com/kuronosu/tela/kit"
)

// The pages of Select, Slider and Tabs (Progress and Table have files of their own).
// The stages are written with tr: English here, Spanish and Arabic in the
// catalogs.

func selectPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	fruit, setFruit := tela.UseState("")
	theme, setTheme := tela.UseState("system")
	fruitName := map[string]string{
		"apple":     tr("select.apple", "Apple"),
		"banana":    tr("select.banana", "Banana"),
		"blueberry": tr("select.blueberry", "Blueberry"),
		"grapes":    tr("select.grapes", "Grapes"),
		"pineapple": tr("select.pineapple", "Pineapple"),
	}
	someItems := []kit.SelectItem[string]{{Value: "one", Label: tr("select.one", "One")}, {Value: "two", Label: tr("select.two", "Two")}}
	northAmerica, europe, asia := tr("select.northAmerica", "North America"), tr("select.europeAfrica", "Europe & Africa"), tr("select.asia", "Asia")
	return grid(
		demo("Basic", "Press, Enter, Space or the arrows open the list; typing jumps to an option; on the closed trigger typing chooses.", col,
			kit.Select[string]{Placeholder: tr("select.fruit", "Select a fruit"), Class: "w-48", Value: &fruit, OnValueChange: setFruit.Set, AriaLabel: tr("select.fruitLabel", "Fruit"),
				Items: []kit.SelectItem[string]{
					{Value: "apple", Label: fruitName["apple"]}, {Value: "banana", Label: fruitName["banana"]}, {Value: "blueberry", Label: fruitName["blueberry"]},
					{Value: "grapes", Label: fruitName["grapes"], Disabled: true}, {Value: "pineapple", Label: fruitName["pineapple"]},
				}},
			readout(map[bool]string{true: tr("select.nothing", "Nothing chosen yet."), false: tr("select.chosen", "Chosen: {fruit}", "fruit", fruitName[fruit])}[fruit == ""])),
		demo("Groups", "Consecutive items with the same Group are drawn under its label, with a separator between groups.", "",
			kit.Select[string]{Placeholder: tr("select.timezone", "Select a timezone"), Class: "w-64", AriaLabel: tr("select.timezoneLabel", "Timezone"), Items: []kit.SelectItem[string]{
				{Group: northAmerica, Value: "est", Label: tr("select.est", "Eastern Standard Time (EST)")},
				{Group: northAmerica, Value: "cst", Label: tr("select.cst", "Central Standard Time (CST)")},
				{Group: northAmerica, Value: "pst", Label: tr("select.pst", "Pacific Standard Time (PST)")},
				{Group: europe, Value: "gmt", Label: tr("select.gmt", "Greenwich Mean Time (GMT)")},
				{Group: europe, Value: "cet", Label: tr("select.cet", "Central European Time (CET)")},
				{Group: asia, Value: "jst", Label: tr("select.jst", "Japan Standard Time (JST)")},
			}}),
		demo("Icons", "An Icon is drawn in the list and in the trigger.", col,
			kit.Select[string]{Class: "w-40", Value: &theme, OnValueChange: setTheme.Set, AriaLabel: tr("select.theme", "Theme"), Items: []kit.SelectItem[string]{
				{Value: "light", Label: tr("select.light", "Light"), Icon: icons.Sun},
				{Value: "dark", Label: tr("select.dark", "Dark"), Icon: icons.Moon},
				{Value: "system", Label: tr("select.system", "System"), Icon: icons.Settings},
			}}),
		demo("Sizes and states", "The default size (36 px) and Sm (32 px); disabled and invalid.", "",
			kit.Select[string]{Placeholder: tr("select.default", "Default"), Class: "w-36", AriaLabel: tr("select.default", "Default"), Items: someItems},
			kit.Select[string]{Placeholder: tr("select.small", "Small"), Size: kit.Sm, Class: "w-36", AriaLabel: tr("select.small", "Small"), Items: someItems},
			kit.Select[string]{Placeholder: tr("select.disabled", "Disabled"), Disabled: true, Class: "w-36", AriaLabel: tr("select.disabled", "Disabled"), Items: someItems},
			kit.Select[string]{Placeholder: tr("select.invalid", "Invalid"), Invalid: true, Class: "w-36", AriaLabel: tr("select.invalid", "Invalid"), Items: someItems}),
	)
}

func sliderPage() tela.Node {
	f := i18n.UseFormatter()
	tr := trFor(f)
	volume, setVolume := tela.UseState(40.0)
	price, setPrice := tela.UseState([]float64{200, 800})
	var bands []tela.Child
	for i, band := range []struct{ key, name string }{
		{"Bass", tr("slider.bass", "Bass")}, {"Mid", tr("slider.mid", "Mid")}, {"Treble", tr("slider.treble", "Treble")},
	} {
		bands = append(bands, Div(Key(band.key), Class("flex flex-col items-center gap-3"),
			kit.Slider{Orientation: kit.Vertical, Class: "h-44", DefaultValue: new(float64(30 + 25*i)), AriaLabel: band.name},
			Span(Class("text-xs text-muted-foreground"), Text(band.name))))
	}
	return grid(
		demo("Single value", "Dragging only repaints: the knob follows the pointer without the page being built on every move.", col,
			kit.Slider{Class: "max-w-sm", Value: &volume, OnValueChange: setVolume.Set, Step: 1, AriaLabel: tr("slider.volume", "Volume")},
			readout(tr("slider.volumeIs", "Volume: {value}%", "value", fmt.Sprintf("%.0f", volume)))),
		demo("Range", "Values, with one per knob; the knobs never pass each other.", col,
			kit.Slider{Class: "max-w-sm", Values: &price, OnValuesChange: setPrice.Set, Min: 0, Max: 1000, Step: 10, MinStepsBetweenThumbs: 5,
				ThumbLabels: []string{tr("slider.minPrice", "Minimum price"), tr("slider.maxPrice", "Maximum price")}},
			readout(tr("slider.priceIs", "Price: ${min} - ${max}", "min", fmt.Sprintf("%.0f", price[0]), "max", fmt.Sprintf("%.0f", price[1])))),
		demo("Steps and disabled", "Step 25 over 0 to 100; a disabled slider.", col,
			kit.Slider{Class: "max-w-sm", DefaultValue: new(50.0), Step: 25, AriaLabel: tr("slider.steps", "In steps of 25")},
			kit.Slider{Class: "max-w-sm", DefaultValue: new(70.0), Disabled: true, AriaLabel: tr("slider.disabled", "Disabled")}),
		demo("Vertical", "A vertical slider has its Min at the bottom.", "flex flex-wrap items-end gap-10", Fragment(bands...)),
	)
}

func tabsPage() tela.Node {
	tr := trFor(i18n.UseFormatter())
	return grid(
		demo("Account", "A Tabs with a form in each panel, as in shadcn's example. Only the selected panel is built.", "",
			kit.Tabs[string]{DefaultValue: new("account"), Class: "w-full max-w-md", AriaLabel: tr("tabs.settings", "Settings"), Tabs: []kit.Tab[string]{
				{Value: "account", Label: tr("tabs.account", "Account"), Content: kit.Card{
					TitleLevel: 2, Title: Text(tr("tabs.account", "Account")), Description: Text(tr("tabs.accountText", "Make changes to your account here.")),
					Children: Div(Class("flex flex-col gap-4"),
						kit.Field{Label: tr("tabs.name", "Name"), Children: kit.Input{DefaultValue: "Mei Tanaka"}},
						kit.Field{Label: tr("tabs.username", "Username"), Children: kit.Input{DefaultValue: "@mei", Props: Dir("ltr")}}),
					Footer: kit.Button{Children: Text(tr("tabs.saveChanges", "Save changes"))}}},
				{Value: "password", Label: tr("tabs.password", "Password"), Content: kit.Card{
					TitleLevel: 2, Title: Text(tr("tabs.password", "Password")), Description: Text(tr("tabs.passwordText", "Change your password here.")),
					Children: Div(Class("flex flex-col gap-4"),
						kit.Field{Label: tr("tabs.currentPassword", "Current password"), Children: kit.Input{Type: "password"}},
						kit.Field{Label: tr("tabs.newPassword", "New password"), Children: kit.Input{Type: "password"}}),
					Footer: kit.Button{Children: Text(tr("tabs.savePassword", "Save password"))}}},
			}}),
		demo("Line variant", "TabsLine draws no pill and underlines the selected tab; a tab can have an icon or be disabled.", col,
			kit.Tabs[string]{Variant: kit.TabsLine, DefaultValue: new("overview"), AriaLabel: tr("tabs.project", "Project"), Tabs: []kit.Tab[string]{
				{Value: "overview", Label: tr("tabs.overview", "Overview"), Icon: icons.LayoutGrid, Content: muted(tr("tabs.overviewText", "Everything about the project, at a glance."))},
				{Value: "activity", Label: tr("tabs.activity", "Activity"), Icon: icons.Bell, Content: muted(tr("tabs.activityText", "Commits, comments and deployments."))},
				{Value: "settings", Label: tr("tabs.settings", "Settings"), Icon: icons.Settings, Content: muted(tr("tabs.settingsText", "Names, access and integrations."))},
				{Value: "billing", Label: tr("tabs.billing", "Billing"), Icon: icons.CreditCard, Disabled: true, Content: muted(tr("tabs.billingText", "Not available on this plan."))},
			}}),
		demo("Vertical", "Orientation Vertical lays the list out beside the panel and moves with the up and down arrows.", "",
			kit.Tabs[string]{Orientation: kit.Vertical, DefaultValue: new("profile"), AriaLabel: tr("tabs.preferences", "Preferences"), Class: "w-full max-w-md", Tabs: []kit.Tab[string]{
				{Value: "profile", Label: tr("tabs.profile", "Profile"), Content: muted(tr("tabs.profileText", "Your name, picture and bio."))},
				{Value: "security", Label: tr("tabs.security", "Security"), Content: muted(tr("tabs.securityText", "Password and two-factor authentication."))},
				{Value: "notifications", Label: tr("tabs.notifications", "Notifications"), Content: muted(tr("tabs.notificationsText", "What reaches your inbox."))},
			}}),
		demo("Manual activation", "ManualActivation makes the arrows move the focus only; Enter or Space selects.", col,
			kit.Tabs[string]{ManualActivation: true, DefaultValue: new("a"), AriaLabel: tr("tabs.manual", "Manual"), Tabs: []kit.Tab[string]{
				{Value: "a", Label: tr("tabs.first", "First"), Content: muted(tr("tabs.firstText", "The first panel."))},
				{Value: "b", Label: tr("tabs.second", "Second"), Content: muted(tr("tabs.secondText", "The second panel."))},
				{Value: "c", Label: tr("tabs.third", "Third"), Content: muted(tr("tabs.thirdText", "The third panel."))},
			}}),
	)
}
