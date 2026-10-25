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
//   - Icons are slots: every icon a component draws is a tela.Icon field
//     (Button.Icon and IconEnd, Badge.Icon), so an app uses the icon set it
//     likes, such as icons/lucide. Where shadcn draws an icon itself, the
//     field has Lucide's as its default (the X of Dialog.CloseIcon).
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
// Field, Select, Tabs, Table and Toaster; the overlays Dialog, AlertDialog,
// Popover and Tooltip; and the controls that choose: Checkbox, Switch,
// RadioGroup, Toggle, ToggleGroup (the segmented control), Slider and
// Progress. A [ColorPicker] chooses a [tela.Color] with an area for the
// saturation and brightness, a hue slider, an optional alpha slider, a hex
// field and optional swatches; a [ColorInput] is the same behind a button
// that shows the color (the look of both is the kit's own, shadcn/ui has
// no color picker):
//
//	kit.ColorPicker{Value: &color, OnValueChange: setColor.Set, Alpha: true,
//	    Swatches: []tela.Color{tela.Hex("#ef4444"), tela.Hex("#22c55e")}}
//	kit.ColorInput{DefaultValue: tela.Hex("#6366f1"), OnValueCommit: save}
//
// A [Table] shows rows of any type through columns, sorts
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
// # Overlays and triggers
//
// A [Dialog], [AlertDialog], [Popover] and [Tooltip] draw in a Portal over
// the window, exist only while they are open (so what is inside them starts
// afresh each time) and enter and leave with a transition. Each has a
// Trigger slot, the element that opens it, and the Open (*bool),
// DefaultOpen and OnOpenChange of the controlled and uncontrolled rule.
//
//	kit.Dialog{
//	    Trigger:     kit.Button{Variant: kit.Outline, Children: h.Text("Edit profile")},
//	    Title:       h.Text("Edit profile"),
//	    Description: h.Text("Make changes to your profile here."),
//	    Children:    form,
//	    Footer:      kit.Button{Children: h.Text("Save changes")},
//	    ShowClose:   true,
//	}
//
// One rule makes every trigger slot of the kit work, Radix's asChild: the
// overlay needs to put props on the element that opens it (the press, the
// aria-expanded and aria-controls, the ref it is placed against, the hover
// of a tooltip), and it does not wrap the trigger in an element of its own.
// A trigger slot is a [Trigger], a component with one method,
// WithTriggerProps(tela.Prop) tela.Component, that merges the props into its
// root element. [Button] is a Trigger, as are the kit components that say
// so; a component of your own becomes one by adding the method; and any
// other element is made one with [TriggerFunc], which hands you the props to
// put where you like:
//
//	kit.Tooltip{
//	    Trigger: kit.TriggerFunc(func(p tela.Prop) tela.Node {
//	        return h.Button(p, h.Class("rounded-full"), h.Text("Docs"))
//	    }),
//	    Content: h.Text("Open the manual"),
//	}
//
// A dialog is modal: the page is inert, Tab stays inside, Escape and a press
// on the scrim close it, and the focus returns to the trigger. An alert
// dialog is the same for a question that needs an answer, and is not
// closed by a press outside. A popover is not modal and closes when the
// focus or a press goes elsewhere. A tooltip opens on hover or keyboard
// focus and describes its trigger; the tooltips of a window are a group,
// so after one opened the next opens at once.
//
// A [Select] and a [Tabs] take their items as slices of structs, generic
// over the key type, and follow the value rule:
//
//	kit.Select[string]{Placeholder: "Select a fruit", Items: []kit.SelectItem[string]{
//	    {Value: "apple", Label: "Apple"}, {Value: "banana", Label: "Banana"}}}
//	kit.Tabs[string]{Tabs: []kit.Tab[string]{
//	    {Value: "account", Label: "Account", Content: AccountForm{}},
//	    {Value: "password", Label: "Password", Content: PasswordForm{}}}}
//
// Toasts have no component to place: a [Toaster] goes once around the app,
// and any component below it gets a handle with [UseToast] and shows toasts
// with it, from a handler or from any goroutine (the handle is comparable and
// every method is safe to call from any goroutine; there is no global):
//
//	tela.Main(tela.App{Title: "Notes", Root: kit.Toaster{Children: Notes{}}})
//
//	toast := kit.UseToast()
//	id := toast.Loading("Uploading…")
//	go func() { upload(); toast.Success("Uploaded", kit.ToastReplace(id)) }()
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
// and the pixels to the anti-aliasing of text. The color picker has no
// shadcn original: its gradients are compared with Chromium's CSS gradients
// of the same colors, and the rest is checked by goldens and behavior tests.
package kit
