package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	"github.com/kuronosu/tela/h"
)

// TabsLine is the Variant of a [Tabs] list that draws no background and
// underlines the selected tab (shadcn's tabsListVariants "line"). The other
// variant, Default, is the muted pill.
const TabsLine Variant = "line"

// TabsStyle is the recipe of [Tabs] (shadcn/ui's Tabs, TabsList,
// TabsTrigger and TabsContent).
type TabsStyle struct {
	// Root is the arrangement of the list and the panel.
	Root string
	// Orientations are the root's classes per orientation: Horizontal puts
	// the list above the panel, Vertical beside it.
	Orientations map[Orientation]string
	// List is the strip of triggers.
	List string
	// ListVariants are the looks of the list: Default (a muted pill) and
	// TabsLine (no background).
	ListVariants map[Variant]string
	// ListOrientations are the list's dimensions per orientation.
	ListOrientations map[Orientation]string
	// Trigger is what every tab has; its selected look is the aria-selected:
	// classes, and TriggerVariants (the list's variant) go after them.
	Trigger string
	// TriggerVariants are the classes a tab adds per variant of its list,
	// after Trigger (shadcn's group-data-[variant=…]/tabs-list:).
	TriggerVariants map[Variant]string
	// TriggerOrientations are the tab's classes per orientation (the
	// vertical list's tabs are full-width and start-aligned).
	TriggerOrientations map[Orientation]string
	// Icon is what a tab's icon has.
	Icon string
	// Indicator is the underline of the TabsLine list's selected tab (the
	// pseudo-element shadcn draws with after:, here a real element);
	// IndicatorOrientations place it per orientation.
	Indicator             string
	IndicatorOrientations map[Orientation]string
	// Content is the panel.
	Content string
}

func init() {
	defaultStyle.Tabs = TabsStyle{
		Root: "flex gap-2",
		Orientations: map[Orientation]string{
			Horizontal: "flex-col",
			Vertical:   "",
		},
		List: "inline-flex w-fit items-center justify-center rounded-lg p-[3px] text-muted-foreground",
		ListVariants: map[Variant]string{
			Default:  "bg-muted",
			TabsLine: "gap-1 rounded-none bg-transparent",
		},
		ListOrientations: map[Orientation]string{
			Horizontal: "h-9",
			Vertical:   "h-fit flex-col",
		},
		Trigger: "group relative inline-flex h-[calc(100%-1px)] flex-1 items-center justify-center gap-1.5 rounded-md border border-transparent px-2 py-1 text-sm font-medium whitespace-nowrap text-foreground/60 " +
			"transition-all outline-none hover:text-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-1 focus-visible:outline-ring " +
			"disabled:pointer-events-none disabled:opacity-50 dark:text-muted-foreground dark:hover:text-foreground " +
			"aria-selected:bg-background aria-selected:text-foreground dark:aria-selected:border-input dark:aria-selected:bg-input/30 dark:aria-selected:text-foreground",
		TriggerVariants: map[Variant]string{
			Default:  "aria-selected:shadow-sm",
			TabsLine: "bg-transparent aria-selected:bg-transparent aria-selected:shadow-none dark:aria-selected:border-transparent dark:aria-selected:bg-transparent",
		},
		TriggerOrientations: map[Orientation]string{
			Horizontal: "",
			Vertical:   "w-full justify-start",
		},
		Icon:      "pointer-events-none size-4 shrink-0",
		Indicator: "absolute bg-foreground opacity-0 transition-opacity group-aria-selected:opacity-100",
		IndicatorOrientations: map[Orientation]string{
			Horizontal: "inset-x-0 bottom-[-5px] h-0.5",
			Vertical:   "inset-y-0 -right-1 w-0.5",
		},
		Content: "flex-1 outline-none",
	}
}

// Tab is one tab of a [Tabs]: its key, its label in the list and the
// content of its panel.
type Tab[K comparable] struct {
	// Value is the tab's key: what Tabs.Value holds and OnValueChange
	// reports.
	Value K
	// Label is the text of the tab; it names the tab for assistive
	// technologies and its panel.
	Label string
	// Icon is drawn before the label; the zero Icon draws nothing.
	Icon tela.Icon
	// Disabled makes the tab inert; the arrow keys skip it.
	Disabled bool
	// Content is the panel shown while the tab is selected. Only the
	// selected tab's panel is built.
	Content tela.Component
}

// Tabs is a tab list with its panels, shadcn/ui's Tabs: a strip of
// triggers and the content of the selected one. It is built on
// aria.UseTabs, so the arrow keys move between the tabs and select them,
// Home and End jump to the first and last, Tab moves from the selected tab
// to its panel, and disabled tabs are skipped.
//
//	kit.Tabs[string]{
//	    DefaultValue: "account",
//	    Tabs: []kit.Tab[string]{
//	        {Value: "account", Label: "Account", Content: AccountForm{}},
//	        {Value: "password", Label: "Password", Content: PasswordForm{}},
//	    },
//	}
//
// The selected tab follows the kit's value rule: Value (a pointer to your
// state) with OnValueChange controls it, DefaultValue starts an
// uncontrolled one. There is always one selected tab: the first enabled
// one when no value is given, or the given one is missing or disabled.
//
// Orientation Vertical lays the list out as a column beside the panel and
// moves with the up and down arrows. Variant TabsLine draws no pill and
// underlines the selected tab.
type Tabs[K comparable] struct {
	// Tabs are the tabs, in order.
	Tabs []Tab[K]
	// Value points to the selected tab of a controlled Tabs; nil leaves
	// the selection to the component, which starts at DefaultValue.
	Value *K
	// DefaultValue is the tab an uncontrolled Tabs starts on; the first
	// enabled tab when it is not one of Tabs.
	DefaultValue K
	// OnValueChange is called with the key of the tab the user selected. For
	// a controlled Tabs it is how the change is asked for.
	OnValueChange func(K)
	// Orientation is Horizontal (the zero value) or Vertical.
	Orientation Orientation
	// Variant is Default (the muted pill) or TabsLine.
	Variant Variant
	// ManualActivation makes the arrow keys only move the focus; Enter or
	// Space selects the focused tab. By default the focused tab is selected.
	ManualActivation bool
	// AriaLabel names the tab list.
	AriaLabel string

	// Class is added after the recipe's Root classes; the other Class
	// fields do the same for their part.
	Class        string
	ListClass    string
	TriggerClass string
	ContentClass string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the tabs element: the list of triggers and the selected
// panel.
func (t Tabs[K]) Build() tela.Node {
	s := &UseStyle().Tabs
	orient := orientationOr(t.Orientation, Horizontal)
	rootClass := pick("Tabs", "Orientation", s.Orientations, orient)

	keys := make([]K, len(t.Tabs))
	var disabled []K
	for i, tab := range t.Tabs {
		keys[i] = tab.Value
		if tab.Disabled {
			disabled = append(disabled, tab.Value)
		}
	}
	o := aria.TabsOptions[K]{
		Keys: keys, DisabledKeys: disabled, Orientation: string(orient), AriaLabel: t.AriaLabel,
		DefaultSelected: aria.SelectionOf(t.DefaultValue),
	}
	if t.ManualActivation {
		o.KeyboardActivation = "manual"
	}
	if t.Value != nil {
		o.Selected = aria.SelectionOf(*t.Value)
	}
	if on := t.OnValueChange; on != nil {
		o.OnSelectionChange = func(sel aria.Selection[K]) {
			if k, ok := sel.First(); ok {
				on(k)
			}
		}
	}
	tabs := aria.UseTabs(o)
	current, _ := tabs.Selected.First()

	listClass := joinClass(s.List, pick("Tabs", "Variant", s.ListVariants, t.Variant),
		pick("Tabs", "Orientation", s.ListOrientations, orient), t.ListClass)
	triggerClass := joinClass(s.Trigger, pick("Tabs", "Variant", s.TriggerVariants, t.Variant),
		pick("Tabs", "Orientation", s.TriggerOrientations, orient), t.TriggerClass)

	triggers := make([]tela.Child, 0, len(t.Tabs)+1)
	triggers = append(triggers, tabs.Props, h.Class(listClass))
	var panel tela.Child
	for _, tab := range t.Tabs {
		var indicator tela.Child
		if t.Variant == TabsLine {
			indicator = h.Span(h.Class(s.Indicator, s.IndicatorOrientations[orient]))
		}
		triggers = append(triggers, h.Div(tabs.TabProps(tab.Value), h.Key(tab.Value), h.Class(triggerClass),
			icon(tab.Icon, s.Icon), h.Text(tab.Label), indicator))
		if tab.Value == current && tab.Content != nil {
			panel = h.Div(tabs.PanelProps(tab.Value), h.Class(s.Content, t.ContentClass), tab.Content)
		}
	}
	return h.Div(h.Class(s.Root, rootClass, t.Class), t.Props, h.Div(triggers...), panel)
}
