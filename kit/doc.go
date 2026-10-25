// Package kit provides styled components for Tela apps: a button, a card, a
// text field, in the look of shadcn/ui (style "new-york-v4", at the
// registry commit [ShadcnCommit]), built on the behavior hooks of package
// aria and the classes of package h. It is the layer an app is written
// with; reach for aria and h when a component of the kit does not fit.
//
// # The shape of a component
//
// Every component is a struct component (ARCHITECTURE ADR-088): its props
// are the fields of the struct, and Build returns its element.
//
//	kit.Button{Variant: kit.Outline, Size: kit.Sm, OnClick: save, Children: h.Text("Save")}
//
// Every component has the same few fields, with the same meaning:
//
//   - Class is added to the classes of the root element after the
//     component's own, and Tela's class order is last-wins, so Class
//     overrides the look with no merging: Class: "w-full" widens a Card,
//     Class: "rounded-full" rounds a Button.
//   - Props is a tela.Child with more props for the root element, such as
//     h.Key, h.TestID or h.OnMouseEnter(...).
//   - Children is a tela.Component, the content of the component. A
//     component with several parts has a slot for each as a tela.Component
//     field (kit.Card has Title, Description, Action, Children and Footer);
//     nil leaves a slot out. A slot that is only text for assistive
//     technologies, such as a Field's Label, Description and Error, is a
//     string. The Class of a part is named after it: HeaderClass,
//     FooterClass.
//   - Behavior fields are named after the HTML or React prop they stand for:
//     Disabled, OnClick, Type, Placeholder, Name, ID, ReadOnly, AutoFocus,
//     MaxLength. Variant and Size choose among the looks of the recipe.
//   - Icons are never built in: every icon a component draws is a
//     tela.Icon field (Button.Icon and IconEnd, Badge.Icon), so an app uses
//     the icon set it likes, such as icons/lucide.
//
// Because a struct component compares its fields to skip a build, a
// component given a func (OnClick) or a slice builds on every parent build;
// that is cheap, and tela.Callback makes it skippable when it matters.
//
// # Controlled and uncontrolled
//
// A component that has a value follows one rule (ADR-101), whatever the
// value is. Give a pointer to your state to control it, a default to leave
// it to the component, and a callback to be told of changes:
//
//	name, setName := tela.UseState("")
//	kit.Input{Value: &name, OnValueChange: setName.Set} // controlled
//	kit.Input{DefaultValue: "Ana"}                      // uncontrolled
//
// The same names recur: Value/DefaultValue/OnValueChange for a string, Checked
// (*bool)/DefaultChecked/OnCheckedChange for an on/off control, Open
// (*bool)/DefaultOpen/OnOpenChange for what opens and closes. A control that
// holds a choice among keys takes Value *K, DefaultValue K and OnValueChange
// func(K).
//
// # Customization, in three levels
//
// 1. Theme tokens. [Theme] is shadcn's neutral theme with a 10 px radius;
// change its colors and every component follows (bg-primary,
// text-muted-foreground and the other classes of the recipes are tokens):
//
//	theme := kit.Theme()
//	theme.Colors["primary"] = tela.SchemeColor{Light: tela.Hex("#4f46e5"), Dark: tela.Hex("#818cf8")}
//	tela.Main(tela.App{Title: "Notes", Root: Notes{}, Theme: &theme,
//	    RootClass: "bg-background text-foreground"})
//
// 2. Recipes. The whole look of a component is data: a [Style] has one
// struct of class strings per component (ButtonStyle, CardStyle...), with a
// Root for the root element and a field for each other part, variant and
// size. [DefaultStyle] returns a copy of the defaults to edit and
// [StyleProvider] applies the edited one to a subtree, as h.Scheme does for
// the color scheme:
//
//	s := kit.DefaultStyle()
//	s.Card.Root += " backdrop-blur-xl"
//	s.Button.Variants["glass"] = "bg-white/10 text-white hover:bg-white/20"
//	return kit.StyleProvider{Style: &s, Children: Dashboard{}}
//
// A Variant or Size is valid when the active recipe defines it, so "glass"
// above is a variant of every Button under the provider. One the recipe
// does not define panics in dev mode, naming the ones it does and the
// closest, and uses the default look otherwise. Components of your own read
// the active style with [UseStyle]. Build a style once and do not edit it
// after giving it to a provider.
//
// 3. Per instance. Class, the Class of a part, Props and the icon slots
// change one component:
//
//	kit.Card{Class: "w-full max-w-sm", HeaderClass: "pb-6 border-b", Title: h.Text("Billing")}
//
// # What is there
//
// Button, Badge, Card, Avatar, Label, Input, Textarea, Separator, Skeleton,
// Field and Table; and the controls that choose: Checkbox, Switch,
// RadioGroup, Toggle, ToggleGroup (the segmented control), Slider and
// Progress. A [Table] shows rows of any type through columns, sorts
// by a header (aria-sort, Enter and Space) and keeps its header and leading
// columns in place when it scrolls:
//
//	kit.Table[Invoice]{Rows: invoices, RowKey: func(i Invoice) any { return i.ID },
//	    Columns: []kit.Column[Invoice]{
//	        {Header: "Invoice", Cell: func(i Invoice) tela.Node { return h.Text(i.ID) }, Sticky: true, Width: "120px",
//	            Sort: func(a, b Invoice) int { return strings.Compare(a.ID, b.ID) }},
//	        {Header: "Amount", Align: kit.AlignRight, Cell: showAmount, Sort: byAmount},
//	    }}
//
// A [Field] is the usual way to put a label, a control, help text
// and an error together: it ties them with an id, so the label names the
// control and clicking it focuses the control, and the control is described
// by the error or the help text.
//
//	kit.Field{Label: "Email", Description: "We never share it.", Error: errs.Email,
//	    Children: kit.Input{Type: "email", Value: &email, OnValueChange: setEmail.Set}}
//
// # The look and its tests
//
// The class strings are shadcn/ui's own, from the pinned commit
// [ShadcnCommit], with the translations Tela needs: data-[state=...]: is
// checked: or an aria-*: variant, [&_svg]: classes are on the icon slot, a
// has-[>svg]: padding is a Go condition, and a border with no color (shadcn
// sets the border color of every element in its base layer) is
// border-border. Every component has a golden image in light and dark at
// scales 1 and 2 in testdata, and a page in testdata/chromium written with
// shadcn's markup, rendered by Chromium with Tailwind 4.3.3, which the
// tests compare with Tela's rendering: boxes to a few hundredths of a pixel
// and the pixels to the anti-aliasing of text.
package kit
