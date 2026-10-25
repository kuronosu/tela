// Package aria provides unstyled behavior hooks for building components:
// the interaction and accessibility that a button, a checkbox or a dialog
// needs, without a look. It follows React Aria (Adobe's behavior hooks,
// Apache-2.0): the same names, the same rules for pointers, keys and
// focus, translated to Tela's hooks and props. The styled components of
// package kit are built on it; use it directly to build your own.
//
// # The pattern: options in, Props out
//
// A hook takes an options struct and returns a struct. Its Props field is
// one tela.Prop that carries everything the hook needs on the element:
// event handlers, the ref, the role and the aria states. Give it to the
// element, and style the element from the other fields of the result:
//
//	press := aria.UsePress(aria.PressOptions{
//	    OnPress:  func(e aria.PressEvent) { save() },
//	    Disabled: busy,
//	})
//	return Button(press.Props, Class("rounded-md px-3 py-1.5"),
//	    If(press.Pressed, Class("scale-95")), Text("Save"))
//
// Props merges several props into one (core's MergeProps): the handlers of
// one event all run, in the order given, and the others (a class, a role)
// follow the same rules as props given one after the other. So hook props
// combine with your own props and with each other:
//
//	ref := tela.UseElementRef() // see "One element, several hooks" below
//	btn := aria.UseButton(aria.ButtonOptions{Ref: ref})
//	hover := aria.UseHover(aria.HoverOptions{})
//	ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
//	return Button(btn.Props, hover.Props, ring.Props,
//	    Class("rounded-md px-3 py-1.5 transition"),
//	    If(hover.Hovered, Class("bg-accent")),
//	    If(ring.FocusVisible, Class("ring-2 ring-primary")), ...)
//
// The hooks are ordinary custom hooks (ADR-085): call them at the top
// level of a component, in the same order on every build.
//
// # One element, several hooks: the ref
//
// A few hooks need the element itself (press measures it to know when the
// pointer leaves, the focus ring asks the engine who has the focus) and
// attach a ref to it. An element has one ref, so when several hooks serve
// one element they must share it: make it once with tela.UseElementRef and
// give it to each hook as its Ref option. The hooks that need no ref
// (UseHover, UseOverlay) take none.
//
//	ref := tela.UseElementRef()
//	btn := aria.UseButton(aria.ButtonOptions{Ref: ref, OnPress: func(aria.PressEvent) { save() }})
//	ring := aria.UseFocusRing(aria.FocusRingOptions{Ref: ref})
//	return Button(btn.Props, ring.Props, ...)
//
// Two different refs on one element are a mistake that dev mode reports
// with a panic that names both call sites and this fix (outside dev mode
// the last one wins). That includes h.Ref(other) next to the props of a
// hook: use the Ref field of the hook's result instead, or pass the ref to
// the hook. The same ref twice is fine.
//
// # Controlled and uncontrolled state
//
// A hook that holds state keeps it itself by default (uncontrolled), and
// lets the caller keep it (controlled) when the caller passes a pointer to
// the value, next to the callback that asks for changes. The callback is
// also called for an uncontrolled control, to follow it:
//
//	// Uncontrolled: DefaultSelected is the initial state.
//	box := aria.UseCheckbox(aria.ToggleOptions{DefaultSelected: true})
//	// Uncontrolled and notified: read the result's Selected, or listen.
//	box = aria.UseCheckbox(aria.ToggleOptions{OnChange: func(on bool) { save(on) }})
//	// Controlled: the caller keeps the state.
//	on, setOn := tela.UseState(false)
//	sw := aria.UseSwitch(aria.ToggleOptions{Selected: &on, OnChange: setOn.Set})
//
// A controlled control shows what the caller passes: a change that the
// caller does not store is refused, as with any controlled field. The open
// state of the overlay hooks (OverlayTriggerOptions, MenuTriggerOptions and
// TooltipTriggerOptions) follows the same rule with Open, DefaultOpen and
// OnOpenChange.
//
// # Hooks
//
// Interactions: [UsePress] (mouse, touch, pen and keyboard as one press),
// [UseHover] (mouse and pen) and [UseFocusRing] (the focus, and whether to
// show it). Focus: [UseFocusScope] (contain, restore and auto-focus the
// focus, and a [FocusManager] that moves it). Controls: [UseButton]
// (which can submit or reset its Form), [UseToggleButton], [UseCheckbox]
// and [UseSwitch], which add the role, the aria state, the tab stop and
// the toggle state to a press. Choices and ranges: [UseRadioGroup] (one
// checked radio, which the arrow keys move and check), [UseToggleGroup] (a
// segmented control or a toolbar of toggle buttons), [UseSlider] (one or
// more thumbs on a track, dragged by repainting), [UseProgressBar] and
// [UseMeter]. Dates: [UseCalendar] and [UseRangeCalendar], with the [Date]
// type. Colors: [UseColorState] (the color that a picker shares, as an
// [HSB] that keeps the hue), [UseColorArea], [UseColorSlider] and
// [UseColorField], with the [HSB] and [HSL] types, [ToHSB], [ToHSL],
// [ParseHexColor] and [FormatHexColor]. Positioning: [UseFloating].
//
// Overlays: [UseOverlay] (Escape and a press outside close the top
// overlay; a modal one makes the rest of the window inert), [UseDialog]
// (role, name and focus), [UsePopover] (a floating overlay next to its
// trigger), [UseOverlayTrigger] (the open state and the trigger's props),
// [UseMenuTrigger] (a trigger that opens with press, long press or keys)
// and [UseTooltipTrigger] (hover and focus with React Aria's delays and
// warm-up).
//
// Collections, keyed lists of items with a focused key and a selection:
// [UseListBox], [UseMenu] (the menu of [UseMenuTrigger]), [UseTabs],
// [UseSelect] (a button that opens a list box) and [UseCombobox] (a text
// field that filters one, with the [Contains] and [StartsWith] filters),
// with the [Selection] type for what is selected.
//
// # Overlays
//
// An overlay is rendered by its owner, in a Portal and only while it is
// open; the hooks give the props. The trigger hook holds the state, the
// popover positions and dismisses, the dialog names and focuses, and they
// share the overlay element's ref:
//
//	trig := aria.UseOverlayTrigger(aria.OverlayTriggerOptions{})
//	pop := aria.UsePopover(aria.PopoverOptions{
//	    Trigger: trig.Ref, Ref: trig.OverlayRef, Open: trig.Open, OnClose: trig.Close,
//	    Placement: "bottom-start", Offset: 4,
//	})
//	dlg := aria.UseDialog(aria.DialogOptions{Ref: trig.OverlayRef, AriaLabel: "Filters"})
//	return Fragment(
//	    Button(trig.TriggerProps, Text("Filters")),
//	    Portal(Presence(If(trig.Open,
//	        Div(pop.Props, dlg.Props, Exit("opacity-0"),
//	            Class("rounded-md border bg-popover p-4 transition starting:opacity-0"), ...)))))
//
// To let an overlay fade out, render it inside a Presence with If and give
// it Exit classes and a transition (and the starting: variant for the
// entrance), as above. An exiting overlay is inert and off the overlay
// stack at once, so the page below works while it fades, a second Escape
// reaches the next overlay, and the hooks' Open value is already false.
//
// Presses and Escape reach only the top overlay. What is opened from an
// overlay is inside it (a popover in a dialog, a tooltip on a button in a
// popover), in a Portal or not.
//
// # Collections
//
// A list box, a menu and a tab list are collections: the caller gives the
// keys of the items (any comparable type) on every build, and renders one
// element per key with the props of ItemProps(key). The hook keeps the
// focused key and the selection, moves the focus with the arrow keys,
// Home, End, PageUp, PageDown and typeahead, and selects with the pointer
// and the keyboard as React Aria does.
//
//	lb := aria.UseListBox(aria.ListBoxOptions[string]{Keys: names, SelectionMode: "multiple"})
//	return Div(lb.Props, Range(len(names), func(i int) tela.Node {
//	    return Div(lb.ItemProps(names[i]), Key(names[i]),
//	        Class("aria-selected:bg-accent focus-visible:ring-2"), Text(names[i]))
//	}))
//
// The selection is a [Selection], uncontrolled by default and controlled
// when the options hold a non-zero one:
//
//	sel, setSel := tela.UseState(aria.SelectionOf("b"))
//	aria.ListBoxOptions[string]{Keys: names, SelectionMode: "multiple",
//	    Selected: sel, OnSelectionChange: setSel.Set}
//
// The focus is roving (the focused item's element has it) unless
// ShouldUseVirtualFocus keeps it on the collection and marks the focused
// item with aria-activedescendant. Give the items of a long list to a
// h.VirtualList: keys that are not rendered can still be focused.
//
// A select and a combobox put a list box in a popover next to a trigger or
// an input. They return the props of each of their elements; the popover
// goes in a Portal, only while Open, and the items are rendered from
// ListBox.ItemProps as in a list box:
//
//	sel := aria.UseSelect(aria.SelectOptions[string]{Keys: fruit, Placeholder: "Pick one"})
//	cb := aria.UseCombobox(aria.ComboboxOptions[string]{Items: fruit, AriaLabel: "Fruit"})
//
// # Radio groups, toggle groups and sliders
//
// A radio group keeps one checked key, uncontrolled by default and
// controlled when Value points to the caller's state; the caller renders a
// Button per key with RadioProps(key), and a Label(For(RadioID(key))) names
// and checks it:
//
//	g := aria.UseRadioGroup(aria.RadioGroupOptions[string]{Keys: plans, AriaLabel: "Plan"})
//	return Div(g.Props, Range(len(plans), func(i int) tela.Node {
//	    return Button(g.RadioProps(plans[i]), Key(plans[i]), Class("checked:bg-primary"), Text(plans[i]))
//	}))
//
// A toggle group is the same with a [Selection]: SelectionMode "single" (a
// radio group of toggle buttons) or "multiple" (a toolbar of aria-pressed
// buttons). A slider has one thumb per value; its ThumbPercent is read in a
// paint-phase closure, so that dragging repaints and does not build:
//
//	s := aria.UseSlider(aria.SliderOptions{DefaultValues: []float64{20, 80}, AriaLabel: "Price"})
//	track := tela.UseSize(s.TrackRef)
//	return Div(s.Props, Div(s.TrackProps, Class("relative h-2 w-64 bg-muted"),
//	    Range(2, func(i int) tela.Node {
//	        return Div(s.ThumbProps(i), Key(i), Class("absolute size-5 -ml-2.5 rounded-full bg-primary"),
//	            Style(tela.TranslateFn(func() (x, y float32) {
//	                return float32(s.ThumbPercent(i)) / 100 * track.Get().Width, 0
//	            })))
//	    })))
//
// # Calendars
//
// A calendar selects a date (or, with [UseRangeCalendar], a range of
// dates) in a month grid, with React Aria's keys and rules, in the
// Gregorian calendar and the locale of package i18n (month names, weekday
// names, the first day of the week). A day is a [Date], a comparable value
// with no time zone; the zero Date is none. The caller renders the parts
// from the methods of the result: the heading and month buttons, the
// weekday headers, and one grid of Weeks per visible month with a
// CellProps per day, keyed by its date, and the grid keyed by its month:
//
//	cal := aria.UseCalendar(aria.CalendarOptions{AriaLabel: "Birthday", Min: aria.Today()})
//	return Div(cal.Props,
//	    Button(cal.PrevProps(), Text("<")), H2(cal.HeadingProps(0), Text(cal.Heading(0))), Button(cal.NextProps(), Text(">")),
//	    Div(cal.GridProps(0), Key(cal.Month(0).String()),
//	        Map(cal.Weeks(0), func(week []aria.Date) tela.Node {
//	            return Div(cal.RowProps(), Key(week[0].String()), Class("flex"),
//	                Map(week, func(day aria.Date) tela.Node {
//	                    return Button(cal.CellProps(0, day), Key(day.String()), Text(cal.DayText(day)))
//	                }))
//	        })))
//
// The selected date is uncontrolled by default and controlled when Value
// points to the caller's state, as everywhere in this package; so is the
// focused date, which decides the month on display.
//
// # Colors
//
// A color picker is a [UseColorState] that its parts share: an area
// ([UseColorArea]: saturation along one axis, brightness along the other),
// sliders for the hue and the opacity ([UseColorSlider], whose
// TrackColors are the stops of the gradient behind the thumb) and a hex
// field ([UseColorField]). The state is the color the picker shows,
// uncontrolled by default and controlled when Value points to the caller's
// tela.Color; it keeps the color as an [HSB], so that dragging through gray
// or black does not lose the hue. All the parts are made in one component,
// and dragging moves the thumbs by repainting, as a slider does:
//
//	st := aria.UseColorState(aria.ColorStateOptions{DefaultValue: tela.Hex("#6366f1")})
//	area := aria.UseColorArea(aria.ColorAreaOptions{State: st, AriaLabel: "Color"})
//	hue := aria.UseColorSlider(aria.ColorSliderOptions{State: st, Channel: aria.ColorHue})
//	hex := aria.UseColorField(aria.ColorFieldOptions{State: st, AriaLabel: "Hex"})
//
// # Testing and agents
//
// The roles and states the hooks set are the ones the aria snapshot shows:
// a checkbox made with [UseCheckbox] prints as `checkbox "Name" [checked]`,
// and telatest finds it with ByRole("checkbox", "Name").
package aria
