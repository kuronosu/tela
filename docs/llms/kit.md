# Recipe: kit components, forms and patterns

Questions: which `kit` component to use and its fields; forms with fields, errors and a submit; switch rows, radio groups and cards, select, segmented controls; dialogs, alert dialogs, popovers, tooltips, menus, tabs, tables, toasts, calendars, color pickers; how to customize kit's look.

`kit` has 39 components with shadcn/ui's ("new-york-v4") look, built on the `aria` behavior hooks: keyboard, focus and roles are done. Use them for every control; build layout and decoration with `h` elements and classes around them.

**Rules that hold for every component:**
- Each is a struct; its fields are its props. `Class` adds classes to the root after the component's own (last wins: `Class: "w-full"` overrides); parts have `...Class` fields (`HeaderClass`, `ContentClass`, `ItemClass`...). `Props` takes one more `h` prop for the root (`Props: TestID("save")`; several: `tela.MergeProps(TestID("x"), AutoComplete("email"))`).
- Content is `Children tela.Component`; multi-part components have slots (`kit.Card{Title, Description, Action, Children, Footer}`). Text-only slots for assistive technologies are strings (`kit.Field{Label, Description, Error}`).
- A kit component is a `tela.Node`: return it from a function or a `Build` as it is (`return kit.Button{...}`); `Fragment` is for several siblings.
- Values: controlled `Value *T` + `OnValueChange func(T)` (`Checked *bool` + `OnCheckedChange`, `Open *bool` + `OnOpenChange`, `Pressed`), or uncontrolled `DefaultValue`/`DefaultChecked`/`DefaultOpen`. Choices among keys are generic: `kit.Select[string]`, `Value *K`, `DefaultValue *K` (write `new("pro")`: `new` takes a value since Go 1.26).
- `Variant`: `kit.Default` (zero), `kit.Secondary`, `kit.Outline`, `kit.Ghost`, `kit.Destructive`, `kit.Link` (and `kit.TabsLine` for Tabs). `Size`: empty is the default size, then `kit.Xs`, `kit.Sm`, `kit.Lg`, and for icon-only buttons `kit.Icon`, `kit.IconXs`, `kit.IconSm`, `kit.IconLg`. `Orientation`: `kit.Horizontal`, `kit.Vertical`. An undefined variant or size panics in dev mode with the valid ones.
- Overlays (`Dialog`, `AlertDialog`, `Popover`, `Tooltip`, menus, pickers) take a `Trigger`: a `kit.Button{...}`, or any element through `kit.TriggerFunc(func(p tela.Prop) tela.Node { return Button(p, ...) })`. Overlays, `Toggle`, `Checkbox`, `Switch` and `Slider` are triggers too: `kit.Tooltip{Trigger: kit.DropdownMenu[K]{Trigger: btn}}`.
- Icons are `tela.Icon` fields (`Button.Icon`, `IconEnd`), defaulting to Lucide's where shadcn draws one.
- Customize: theme tokens first; then recipes: `s := kit.DefaultStyle()`, edit class strings (`s.Card.Root += " shadow-lg"`, `s.Button.Variants["glass"] = "bg-white/10 text-white hover:bg-white/20"`), apply with `kit.StyleProvider{Style: &s, Children: ...}`; then per instance `Class`.

**Catalog** (key fields; nearly every component also has `Class` and `Props`):

- `kit.Button{Variant, Size, Icon, IconEnd, Type, Disabled, Loading, OnClick, AriaLabel, Children}`: `Type: "submit"` in a `Form`; icon-only buttons need `Size: kit.Icon` and `AriaLabel`; `Loading`: spinner, `aria-busy`, presses ignored, width kept.
- `kit.Badge{Variant, Icon, Dot, DotClass, Children}`: a small label; tones `kit.Success`, `Warning`, `Info` (also for `Alert`).
- `kit.Alert{Variant, Icon, Title, Description, Action, Role}`: a callout; role `alert`, or `Role: "status"` for a notice; `Action` is a button at its end.
- `kit.Card{Title, Description, Action, Children, Footer, TitleLevel, HeaderClass, ContentClass, FooterClass}`: the surface; `TitleLevel: 2` makes the title an `h2`; `Action` sits at the header's end.
- `kit.Avatar{Src, Alt, Name, Fallback, Color, Size, FallbackClass}`: `Src: SrcPath("img/ada.png")`; `Fallback` initials (or from `Name`); `Color: kit.AvatarAuto` picks a background from the name.
- `kit.AvatarGroup{Avatars, Max, Total, Size, AriaLabel}`: overlapping avatars and a "+N".
- `kit.Separator{Orientation, Semantic, Caption}`: a 1 px line (`Semantic`: in the accessibility tree); `Caption: Text("or")` centers a label on it.
- `kit.Skeleton{Class}`: a pulsing placeholder (`Class: "h-4 w-40 rounded-md"`).
- `kit.Markdown{Source, OnLink, OnToggleTask}`: CommonMark + GFM; links call `OnLink` or `tela.OpenURL`; images from `tela.Assets` only.
- `kit.Label{For, Disabled, Children}`: a styled label for a control with an `ID`.
- `kit.Input{Value, DefaultValue, OnValueChange, Type, Placeholder, Name, ID, AriaLabel, Disabled, ReadOnly, Invalid, AutoFocus, MaxLength, Icon, IconEnd}`: with an icon `Class` is the wrapper's, `InputClass` the input's.
- `kit.Textarea{Value, DefaultValue, OnValueChange, Placeholder, Rows, MaxLength, AriaLabel, Invalid}`.
- `kit.Field{Label, Description, Error, Invalid, Orientation, ControlAfter, Children, LabelContent, DescriptionContent, LabelEnd, DescriptionEnd, LabelClass, ContentClass}`: label, control, help and error wired together (`Error` marks the control invalid). `LabelEnd` is a slot at the label line's end ("Forgot password?"); a `RadioGroup`/`ToggleGroup` inside is named by the label. One kit control per field, not a trigger (`Dialog`, `Popover`...; lint says so): a "label + button" row is plain elements, see below. By hand: `gap-3` between its parts; description `text-sm text-muted-foreground`.
- `kit.FieldSet{Legend, LegendVariant, Description, Children, LegendClass, ContentClass}`: a named group; names a `RadioGroup`/`ToggleGroup` inside. `LegendVariant: kit.LegendVariantLabel` is label-sized, the default `kit.LegendVariantLegend` a title.
- `kit.Checkbox{Checked, DefaultChecked, OnCheckedChange, Disabled, Required, Invalid, ID, AriaLabel}`.
- `kit.Switch{Checked, DefaultChecked, OnCheckedChange, Size, Disabled, ID, AriaLabel, ThumbClass}`.
- `kit.RadioGroup[K]{Items, Value, DefaultValue, OnValueChange, Orientation, Disabled, AriaLabel, Render, OptionClass}`: options are `kit.RadioItem[K]{Value, Label, Description, Disabled}`; `Render func(kit.RadioItem[K], kit.RadioState) tela.Node` draws an option yourself (cards).
- `kit.RadioState{Checked, Disabled, Invalid, ID, Props, Indicator}`: what `Render` gets: put `Props` on the one element that is the radio; `Indicator` is the default circle.
- `kit.Toggle{Pressed, DefaultPressed, OnPressedChange, Variant, Size, Icon, AriaLabel, Children}`: a button that stays pressed.
- `kit.ToggleGroup[K]{Items, Value, ValueFn, DefaultValue, OnValueChange, Multiple, Selected, SelectedFn, OnSelectionChange, DisallowEmptySelection, Indicator, Variant, Size, Spacing, AriaLabel, ItemClass}`: segmented control; `ValueFn` reads the choice from a `tela.Value` (the group builds, not its holder); `Indicator` is a pill drawn in each item that is on, which an `h.LayoutID` slides; items `kit.ToggleGroupItem[K]{Value, Label, Icon, AriaLabel, Disabled}`.
- `kit.Slider{Value, DefaultValue, OnValueChange, Values, OnValuesChange, Min, Max, Step, Orientation, Disabled, AriaLabel, FormatValue}`: `Value *float64`; `DefaultValue: new(50.0)`; `FormatValue func(float64) string` is the value as read aloud (aria-valuetext), not drawn: show the value yourself (`Textf("%g px", v)`).
- `kit.Progress{Value, ValueFn, Max, Indeterminate, FormatValue, AriaLabel}`: `Value` is a plain number (0 to `Max`, 100 by default); `ValueFn func() float64` reads a `tela.Value`, so a tween or a download moves the bar and its aria value without a build (the bar jumps: tween the value for a smooth one).
- `kit.Select[K]{Items, Value, DefaultValue, OnValueChange, Placeholder, Size, Disabled, Invalid, AriaLabel, ID}`: items `kit.SelectItem[K]{Value, Label, Icon, Disabled, Group}`; its trigger is a `button` named "chosen label + field label".
- `kit.Tabs[K]{Tabs, Value, DefaultValue, OnValueChange, Orientation, Variant, AriaLabel, ListClass, TriggerClass, ContentClass}`: `kit.Tab[K]{Value, Label, Icon, Disabled, Content}`; only the selected panel is built.
- `kit.Table[T]{Rows, RowKey, Columns, Sort, DefaultSort, OnSortChange, AlwaysSorted, HideUnsortedIcon, Selected, RowProps, Caption, AriaLabel, Empty, RowHeight}`: columns `kit.Column[T]{Header, Cell, Sort, Width, Align, Sticky, Footer}`; `Width string` is the column's grid track (`"200px"`, `"1fr"`, `"auto"`); `Align: kit.AlignRight`; a `Sort` func makes a header sortable. `RowHeight: 37` (px) makes the table virtual for thousands of rows (give it a height and px/fr/minmax widths, no Sticky).
- `kit.Dialog{Trigger, Title, Description, Children, Footer, Open, DefaultOpen, OnOpenChange, HideClose, AriaLabel}`: modal; Escape and the scrim close it.
- `kit.AlertDialog{Trigger, Title, Description, Cancel, Action, Destructive, OnAction, OnCancel, Open, OnOpenChange, Disabled}`: a confirmation. `Title`, `Description`, `Cancel` and `Action` are `tela.Component`s, not strings: `Title: Text("Delete?")`, `Cancel: Text("Keep editing")`, `Action: Text("Delete")` (defaults "Cancel" and "Continue").
- `kit.Popover{Trigger, Children, Placement, Align, Offset, Modal, AriaLabel, Open, OnOpenChange}`: non-modal panel; `Placement: "top"`, `Align: "start"`.
- `kit.TitleBar{Variant, Look, Title, Leading, Children, Trailing, ControlsSide, HideMinimize, HideMaximize, State, TitleClass}`: the title bar of a `Frameless` window (or one with `tela.UseWindow().State().ClientSideDecorations`): a drag area (`h.WindowDrag`) with a title, slots and the window controls at the platform's side; buttons and fields in the slots stay clickable. Looks `kit.Windows`, `kit.GNOME`, `kit.MacOS` (default by OS), and `kit.Plain` for an app with a design of its own: the platform's arrangement (`Look` picks another) with no background, border or outline (add `Class: "bg-sidebar"` for a surface) and ghost window controls in the theme's tokens; it dims when unfocused. Put it at the top of a flex column.
- `kit.WindowControls{Variant, Look, HideMinimize, HideMaximize, State, MinimizeLabel, MaximizeLabel, RestoreLabel, CloseLabel}`: minimize, maximize/restore and close as real buttons (named "Minimize", "Maximize"/"Restore", "Close") that act on click; `Variant: kit.Plain` draws them quietly (ghost buttons; `Look` picks the arrangement); `TitleBar` draws them, so use this alone only to place them yourself.
- `kit.Tooltip{Trigger, Content, Placement, Delay, Disabled}`: on hover or keyboard focus; describes its trigger.
- `kit.DropdownMenu[K]{Trigger, Items, OnSelect, Placement, Align, AriaLabel, Open, OnOpenChange}`: items `kit.MenuItem[K]{Kind, Key, Label, AccessKey, Icon, Shortcut, Role, Variant, Disabled, OnSelect, Checked, DefaultChecked, OnCheckedChange, Items}`; `Kind`: `kit.MenuPlain` (zero), `MenuCheckbox`, `MenuRadioGroup`, `MenuLabel`, `MenuSeparator`, `MenuGroup`, `MenuSub` (a submenu of `Items`). `Role: desktop.RoleCopy` is a standard item; `Shortcut: "CmdOrCtrl+N"` shows ⌘N on a Mac.
- `kit.ContextMenu[K]{Children, Items, OnSelect, AriaLabel, Disabled, Native}`: right click (or long press) on `Children`; no tab stop (`Props`: `TabIndex(0)`, role, name). `Native: kit.NativeOn` uses the OS's menu where there is one (recipe desktop-integration).
- `kit.Menubar[K]{Menus, OnSelect, AriaLabel, Native, AppMenu, ActivationKeys, AppItems}`: menus `kit.MenubarMenu[K]{Key, Label, AccessKey, Items, Disabled, Role}` (`Role: desktop.RoleEditMenu` with no Items: the OS's Edit menu in a native bar). `AppMenu` makes it the app's one bar (recipe desktop-integration, Native menus). Access keys: `kit.MenuItem.AccessKey` and `kit.MenubarMenu.AccessKey` (a rune, Windows and Linux; recipe accessibility).
- `kit.Toaster{Children, Position, Duration, VisibleToasts, CloseButton, AriaLabel, CloseLabel}`: put once around the app (`AriaLabel` and `CloseLabel` replace its built-in texts: recipe i18n); any component below calls `toast := kit.UseToast()` (a `kit.ToastHandle`: comparable, safe to keep or pass on) then `toast.Success(title, opts...)`, `Error`, `Info`, `Warning`, `Loading`, `Show`, `Dismiss(id)`; options `kit.ToastDescription(text)`, `kit.ToastAction(label, fn)`, `kit.ToastDuration(d)`, `kit.ToastReplace(id)`. Safe from any goroutine.
- `kit.Calendar{Value, DefaultValue, OnValueChange, Min, Max, IsDateUnavailable, VisibleMonths}`: one `aria.Date` (comparable day, zero = none; `aria.Today()`).
- `kit.RangeCalendar{Value, DefaultValue, OnValueChange, VisibleMonths}`: an `aria.DateRange`.
- `kit.DatePicker{Value, DefaultValue, OnValueChange, Placeholder, Min, Max}`: an outline button that opens a calendar.
- `kit.DateRangePicker{Value, DefaultValue, OnValueChange, Placeholder}`: the same for a range.
- `kit.ColorPicker{Value, DefaultValue, OnValueChange, OnValueCommit, Alpha, Swatches}`: a `tela.Color` (`DefaultValue: new(tela.Hex("#6366f1"))`).
- `kit.ColorInput{Value, DefaultValue, OnValueChange, Swatches}`: the picker behind a button.
- `kit.StyleProvider{Style, Children}`: applies an edited `kit.DefaultStyle()` to a subtree.

**Patterns.** A form card with fields, errors and a submit:

```go
func Profile() tela.Node { return tela.C(profile) }

func profile() tela.Node {
	name, setName := tela.UseState("Ada Lovelace")
	email, setEmail := tela.UseState("ada@example")
	bio, setBio := tela.UseState("")
	emailError := ""
	if !strings.Contains(email, ".") {
		emailError = "Enter a valid e-mail address."
	}
	return kit.Card{
		Class: "w-full max-w-md", TitleLevel: 2,
		Title:       Text("Profile"),
		Description: Text("This is how others see you."),
		Action:      kit.Badge{Variant: kit.Secondary, Children: Text("Public")},
		Children: Div(Class("flex flex-col gap-5"),
			kit.Field{Label: "Name", Children: kit.Input{Value: &name, OnValueChange: setName.Set}},
			kit.Field{Label: "E-mail", Description: "We never share it.", Error: emailError,
				Children: kit.Input{Type: "email", Value: &email, OnValueChange: setEmail.Set}},
			kit.Field{Label: "Bio", Children: kit.Textarea{Rows: 3, Placeholder: "A few words", Value: &bio, OnValueChange: setBio.Set}}),
		FooterClass: "justify-end gap-2",
		Footer: Fragment(
			kit.Button{Variant: kit.Outline, Children: Text("Cancel")},
			kit.Button{Disabled: emailError != "", Children: Text("Save")}),
	}
}
```

Switch rows, a radio group named by a legend, radio cards, a select, a segmented control:

```go
func Preferences() tela.Node { return tela.C(preferences) }

func preferences() tela.Node {
	digest, setDigest := tela.UseState(true)
	plan, setPlan := tela.UseState("pro")
	size, setSize := tela.UseState("md")
	view, setView := tela.UseState("list")
	plans := []kit.RadioItem[string]{
		{Value: "free", Label: "Free", Description: "Up to 3 projects."},
		{Value: "pro", Label: "Pro", Description: "Unlimited projects."},
	}
	return Div(Class("flex max-w-lg flex-col gap-6"),
		// A switch row: text first, the switch at the end.
		kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center gap-6",
			Label: "Weekly digest", Description: "A summary every Monday.",
			Children: kit.Switch{Checked: &digest, OnCheckedChange: setDigest.Set}},
		// The legend names the group: no AriaLabel needed.
		kit.FieldSet{Legend: "Plan", LegendVariant: kit.LegendVariantLabel,
			Children: kit.RadioGroup[string]{Items: plans, Value: &plan, OnValueChange: setPlan.Set}},
		kit.Field{Label: "Text size", Children: kit.Select[string]{Class: "w-40", Value: &size, OnValueChange: setSize.Set,
			Items: []kit.SelectItem[string]{{Value: "sm", Label: "Small"}, {Value: "md", Label: "Medium"}, {Value: "lg", Label: "Large"}}}},
		kit.FieldSet{Legend: "View", LegendVariant: kit.LegendVariantLabel,
			Children: kit.ToggleGroup[string]{Variant: kit.Outline, DisallowEmptySelection: true, Value: &view, OnValueChange: setView.Set,
				Items: []kit.ToggleGroupItem[string]{{Value: "list", Label: "List"}, {Value: "grid", Label: "Grid"}}}},
		// Radio cards: Render draws each option; st.Props makes the button the radio.
		kit.RadioGroup[string]{AriaLabel: "Plan cards", Items: plans, Value: &plan, OnValueChange: setPlan.Set, Class: "grid-cols-2",
			Render: func(o kit.RadioItem[string], st kit.RadioState) tela.Node {
				return Button(Key(o.Value), st.Props,
					Class("flex flex-col items-start gap-1 rounded-lg border border-border p-4 text-left transition-colors hover:bg-accent",
						"aria-checked:border-primary aria-checked:ring-1 aria-checked:ring-primary"),
					Div(Class("flex items-center gap-2 text-sm font-medium"), st.Indicator, Text(o.Label)),
					P(Class("text-sm text-muted-foreground"), Text(o.Description)))
			}},
	)
}
```

Tabs, buttons, an alert dialog, a toast, a tooltip, a dropdown menu, an avatar:

```go
func Account() tela.Node { return kit.Toaster{Children: tela.C(account)} }

func account() tela.Node {
	toast := kit.UseToast()
	menu := []kit.MenuItem[string]{
		{Kind: kit.MenuLabel, Label: "My account"},
		{Key: "edit", Label: "Edit", Icon: lucide.Pencil, Shortcut: "Ctrl+E"},
		{Kind: kit.MenuSeparator},
		{Key: "logout", Label: "Log out", Icon: lucide.LogOut, Variant: kit.Destructive},
	}
	return Div(Class("flex flex-col gap-6 p-8"),
		Div(Class("flex items-center gap-3"),
			kit.Avatar{Fallback: "AL", Size: kit.Lg},
			Div(Class("flex flex-1 flex-col"), Span(Class("font-medium"), Text("Ada Lovelace")),
				Span(Class("text-sm text-muted-foreground"), Text("ada@example.com"))),
			kit.Tooltip{Content: Text("Invite a teammate"),
				Trigger: kit.Button{Variant: kit.Outline, Size: kit.Icon, Icon: lucide.Plus, AriaLabel: "Invite"}},
			kit.DropdownMenu[string]{Align: "end", Items: menu, OnSelect: func(k string) { toast.Info("Chose " + k) },
				Trigger: kit.Button{Variant: kit.Ghost, Size: kit.Icon, Icon: lucide.Ellipsis, AriaLabel: "More"}}),
		kit.Separator{},
		kit.Tabs[string]{DefaultValue: new("general"), Tabs: []kit.Tab[string]{
			{Value: "general", Label: "General", Content: Div(Class("flex gap-2 pt-2"),
				kit.Button{Icon: lucide.Check, Children: Text("Save"),
					OnClick: func() { toast.Success("Saved", kit.ToastDescription("Your changes are live.")) }},
				kit.Button{Variant: kit.Secondary, Children: Text("Preview")},
				kit.Button{Variant: kit.Link, Children: Text("Learn more")})},
			{Value: "danger", Label: "Danger zone", Content: Div(Class("pt-2"),
				kit.AlertDialog{
					Trigger:     kit.Button{Variant: kit.Destructive, Icon: lucide.Trash2, Children: Text("Delete account")},
					Title:       Text("Delete your account?"),
					Description: Text("This cannot be undone."),
					Action:      Text("Delete"), Destructive: true,
					OnAction: func() { toast.Error("Account deleted") },
				})},
		}})
}
```

A table and a dialog with a form:

```go
type Invoice struct {
	ID, Status string
	Amount     float64
}

func Invoices(rows []Invoice) tela.Node {
	return kit.Table[Invoice]{
		Rows: rows, RowKey: func(i Invoice) any { return i.ID }, Caption: "Recent invoices",
		Columns: []kit.Column[Invoice]{
			{Header: "Invoice", Cell: func(i Invoice) tela.Node { return Span(Class("font-medium"), Text(i.ID)) },
				Sort: func(a, b Invoice) int { return strings.Compare(a.ID, b.ID) }},
			{Header: "Status", Cell: func(i Invoice) tela.Node { return kit.Badge{Variant: kit.Outline, Children: Text(i.Status)} }},
			{Header: "Amount", Align: kit.AlignRight, Cell: func(i Invoice) tela.Node { return Textf("$%.2f", i.Amount) },
				Sort: func(a, b Invoice) int { return cmp.Compare(a.Amount, b.Amount) }},
		},
	}
}

func Rename() tela.Node { return tela.C(rename) }

func rename() tela.Node {
	open, setOpen := tela.UseState(false)
	name, setName := tela.UseState("Roadmap")
	return kit.Dialog{
		Open: &open, OnOpenChange: setOpen.Set,
		Trigger:     kit.Button{Variant: kit.Outline, Icon: lucide.Pencil, Children: Text("Rename")},
		Title:       Text("Rename project"),
		Description: Text("Pick a short, memorable name."),
		Children:    kit.Field{Label: "Name", Children: kit.Input{Value: &name, OnValueChange: setName.Set, AutoFocus: true}},
		Footer:      kit.Button{Disabled: name == "", OnClick: func() { setOpen.Set(false) }, Children: Text("Save")},
	}
}
```

A row with a label and a button that opens an overlay (not a `kit.Field`, whose label would name nothing):

```go
func SignOutRow(onConfirm func()) tela.Node {
	return Div(Class("flex items-center justify-between gap-6"),
		Div(Class("flex flex-col gap-1.5"),
			Span(Class("text-sm leading-snug font-medium"), Text("Sign out everywhere")),
			Span(Class("text-sm text-muted-foreground"), Text("Other devices will need to sign in again."))),
		kit.AlertDialog{
			Trigger:     kit.Button{Variant: kit.Outline, Icon: lucide.LogOut, Children: Text("Sign out")},
			Title:       Text("Sign out of all devices?"),
			Description: Text("You stay signed in here."),
			Action:      Text("Sign out"), Destructive: true, OnAction: onConfirm,
		})
}
```

An alert, with a deferred value and a sliding segmented control (`ValueFn`, `Indicator`):

```go
func ErrorAlert(title, detail string, retry func()) tela.Node {
	return kit.Alert{Variant: kit.Destructive, Icon: lucide.CircleAlert,
		Title: Text(title), Description: Text(detail),
		Action: kit.Button{Variant: kit.Outline, Size: kit.Sm, Children: Text("Retry"), OnClick: retry}}
}

// Progress follows a tela.Value: a tween or a download moves the bar and its
// aria value without building the component.
func Download(progress *tela.Value[float64]) tela.Node {
	return kit.Progress{ValueFn: progress.Get, AriaLabel: "Download"}
}

// ToggleGroup with an Indicator: the pill slides to the item that is on
// (h.LayoutID), and the group reads the choice from a tela.Value.
func ViewSwitch() tela.Node { return tela.C(viewSwitch) }

func viewSwitch() tela.Node {
	view := tela.UseValue("list")
	return kit.ToggleGroup[string]{
		ValueFn: view.Get, OnValueChange: view.Set, DisallowEmptySelection: true, AriaLabel: "View",
		Indicator: Div(Class("absolute inset-0 -z-10 rounded-md bg-background shadow-sm"), LayoutID("view-pill")),
		Items:     []kit.ToggleGroupItem[string]{{Value: "list", Label: "List"}, {Value: "grid", Label: "Grid"}},
	}
}
```
