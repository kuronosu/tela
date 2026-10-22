// Portions of this file follow React Aria's useComboBox and
// react-stately's useComboBoxState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/combobox
// and packages/@react-stately/combobox), Copyright 2020 Adobe, licensed
// under the Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// The ways a combobox's menu opens ([ComboboxOptions].MenuTrigger).
const (
	// MenuTriggerInput opens the list when the user types (the default).
	MenuTriggerInput = "input"
	// MenuTriggerFocus opens it when the input gets the focus too, with
	// every item.
	MenuTriggerFocus = "focus"
	// MenuTriggerManual opens it only with ArrowDown, ArrowUp and the
	// toggle button.
	MenuTriggerManual = "manual"
)

// ComboboxOptions configures [UseCombobox]. The items are keyed like those
// of a list box: see [ListBoxOptions] for TextValue, DisabledKeys, and the
// controlled and uncontrolled Selected, DefaultSelected and
// OnSelectionChange (a combobox holds at most one selected key).
type ComboboxOptions[K comparable] struct {
	// Keys are the items to show, filtered by the caller: the caller keeps
	// the text, filters its data as it likes (a server search, a fuzzy
	// match) and gives the result on every build. Or give Items.
	Keys []K
	// Items are all the items, in order, and the hook shows the ones that
	// match the text with Filter (React Aria's defaultItems with
	// defaultFilter). Give Keys or Items, not both; an empty, non-nil Items
	// is a combobox without items. ComboboxResult.Keys are the items to
	// render.
	Items []K
	// Filter decides whether the text of an item matches what is typed;
	// [Contains] (the default), [StartsWith] or your own function. It
	// applies to Items only.
	Filter Filter
	// TextValue is the text of an item: it goes to the input when the item
	// is chosen and Filter matches it. The default is fmt.Sprint of the
	// key.
	TextValue         func(K) string
	DisabledKeys      []K
	Selected          Selection[K]
	DefaultSelected   Selection[K]
	OnSelectionChange func(Selection[K])
	// InputValue points to the text of a controlled combobox, with
	// OnInputChange asking for the next one; nil leaves it to the hook,
	// which starts at DefaultInputValue (else the text of the selected
	// item). OnInputChange is also called for an uncontrolled combobox and
	// for the text that choosing an item or leaving the input puts back.
	InputValue        *string
	DefaultInputValue string
	OnInputChange     func(string)
	// Open points to the open state of a controlled list; nil leaves it to
	// the hook, starting at DefaultOpen. OnOpenChange is called with the
	// next state.
	Open         *bool
	DefaultOpen  bool
	OnOpenChange func(open bool)
	// MenuTrigger is how the list opens: [MenuTriggerInput] (the default),
	// [MenuTriggerFocus] or [MenuTriggerManual]. ArrowDown (first item)
	// and ArrowUp (last) and the toggle button open it in every mode, with
	// all the items.
	MenuTrigger string
	// AllowsCustomValue lets the text stay as typed when the input loses
	// the focus or Enter is pressed with no item chosen, clearing the
	// selection. Without it the text goes back to that of the selected item
	// (empty when none is).
	AllowsCustomValue bool
	// AllowsEmptyCollection keeps the list open when no item matches, to
	// show a "no results" message. By default it closes.
	AllowsEmptyCollection bool
	// Disabled makes the input and the button inert and disabled; ReadOnly
	// makes the input read-only and the list does not open.
	Disabled, ReadOnly bool
	// ShouldFocusWrap makes ArrowDown on the last item highlight the first,
	// and ArrowUp on the first the last.
	ShouldFocusWrap bool
	// Placement is where the popover goes around its anchor, one of
	// Floating UI's 12 placements; "bottom-start" by default. Offset is the
	// distance in pixels, and Floating are more options of [UseFloating].
	Placement string
	Offset    float32
	Floating  []FloatingOption
	// MatchWidth makes the popover as wide as the anchor.
	MatchWidth bool
	// AnchorRef is the element the popover is placed next to and
	// MatchWidth measures: the wrapper of the input and the button, say. The
	// zero value anchors it to the input.
	AnchorRef tela.ElementRef
	// AriaLabel names the input when there is no label element; with
	// LabelProps the label element names it.
	AriaLabel string
	// ButtonLabel is the accessible name of the toggle button that
	// ButtonProps go to (default "Show suggestions"). Set it to translate
	// the name.
	ButtonLabel string
	// Ref is the input's ref, for a hook used together with others on the
	// same element; the zero value makes the hook create its own.
	Ref tela.ElementRef
}

// ComboboxResult is what [UseCombobox] returns.
type ComboboxResult[K comparable] struct {
	// InputProps go to the Input: the "combobox" role, the text and its
	// change, the keys (arrows, Enter, Tab, Escape), aria-autocomplete,
	// aria-expanded, aria-controls while open and aria-activedescendant
	// for the highlighted item. The input keeps the focus: the items are
	// highlighted, never focused.
	InputProps tela.Prop
	// ButtonProps go to an optional toggle button: it shows or hides the
	// list with every item, and keeps the focus on the input. It is not a
	// tab stop and is labelled with ButtonLabel ("Show suggestions" by
	// default; an AriaLabel after the props also changes it).
	ButtonProps tela.Prop
	// LabelProps go to the label element: it names the input, and a click
	// on it focuses the input.
	LabelProps tela.Prop
	// PopoverProps go to the popover's element, which holds the list box:
	// it is fixed to the window and placed next to the anchor in the frame
	// that shows it, it is on the overlay stack (Escape closes it), and a
	// press in it keeps the focus on the input. With MatchWidth it is as
	// wide as the anchor. Render it in a Portal, only while Open.
	PopoverProps tela.Prop
	// ArrowProps go to an arrow element inside the popover.
	ArrowProps tela.Prop
	// ListBox is the list: its Props go to the element that holds the
	// items and ItemProps(k) to each item, as in [UseListBox]. The items
	// are rendered inside the popover, only while Open, from Keys.
	ListBox ListBoxResult[K]
	// Keys are the items to render: ComboboxOptions.Keys, or the Items that
	// match the text (all of them after the list was opened with the
	// arrows or the button, until the text changes).
	Keys []K
	// InputValue is the text of the input.
	InputValue string
	// ShowAll is true after the list was opened by hand (the button, the
	// arrows, or the focus with MenuTriggerFocus) and until the text
	// changes: the list shows every item whatever the text. A hook given
	// Items does it itself; a caller who filters Keys should too.
	ShowAll bool
	// Selected is the current selection: [Selection.First] is the selected
	// key.
	Selected Selection[K]
	// Open is whether the list is open; SetOpen asks for a state, Toggle for
	// the other one and Close for closed.
	Open    bool
	SetOpen func(open bool)
	Toggle  func()
	Close   func()
	// Pressed is true while the button is pressed.
	Pressed bool
	// Placement is the popover's placement after flipping (see
	// [PopoverResult]).
	Placement string
	// InputRef, ButtonRef and PopoverRef are the refs of those elements.
	InputRef, ButtonRef, PopoverRef tela.ElementRef
}

// comboState is the state of one combobox hook, on the UI thread (like
// collState, the handlers read and write it directly, so a second event
// before the next build sees the first one's effect).
type comboState[K comparable] struct {
	o    ComboboxOptions[K]
	c    *collState[K]
	ost  *boolState
	text func(K) string
	bump tela.Setter[int]

	started   bool
	itemsMode bool
	filter    Filter

	// The text: input is the current value, whether the hook keeps it or
	// the caller does.
	input           string
	inputControlled bool
	setInternal     tela.Setter[string]
	selControlled   bool
	// lastValue and lastKey are what the last sync saw, to tell what
	// changed (React Aria's lastValue and lastSelectedKey).
	lastValue string
	lastKey   K
	lastHas   bool

	focused      bool
	composing    bool
	showAll      bool
	openStrategy string

	// match is the result of matching for the text it was made for; it is
	// dropped when a build starts or the text changes.
	match      []K
	matchValid bool
}

// UseCombobox holds the state of a combobox and returns the props of its
// input, its toggle button, its popover and its list (React Aria's
// useComboBox with useComboBoxState): a text field that filters a list of
// options, and a list that opens next to it.
//
//	cb := aria.UseCombobox(aria.ComboboxOptions[string]{
//	    Items: fruit, TextValue: capital, AriaLabel: "Fruit", MatchWidth: true,
//	})
//	return Fragment(
//	    Div(Class("flex"),
//	        Input(cb.InputProps, Class("w-48 rounded-l-md border px-3 py-2")),
//	        Button(cb.ButtonProps, Class("rounded-r-md border px-2"), Icon(chevronDown))),
//	    Portal(If(cb.Open,
//	        Div(cb.PopoverProps, Class("rounded-md border bg-popover p-1 shadow-md"),
//	            Div(cb.ListBox.Props, Class("max-h-60 overflow-auto"),
//	                Range(len(cb.Keys), func(i int) tela.Node {
//	                    k := cb.Keys[i]
//	                    return Div(cb.ListBox.ItemProps(k), Key(k),
//	                        Class("rounded px-2 py-1.5 aria-selected:font-medium"),
//	                        If(cb.ListBox.IsFocused(k), Class("bg-accent")), Text(capital(k)))
//	                }))))))
//
// Typing filters the list and opens it (see MenuTrigger). The input keeps
// the focus: ArrowDown and ArrowUp highlight the next and previous item
// (the first and the last, opening the list when closed), Enter chooses
// the highlighted item, which sets the text to its TextValue and closes the
// list, Tab chooses it and moves on, and Escape closes the list and puts the
// text back to that of the selected item (cleared when there is none).
// When the input loses the focus the text is committed: reset to the
// selected item's, or kept as a custom value with AllowsCustomValue. Home,
// End, Space and the left and right arrows stay with the text field, and
// nothing moves while an IME composes text.
//
// Items are highlighted with [Collection.IsFocused] (there is no DOM focus
// on them); the input carries aria-activedescendant. In the aria snapshot
// the input is `combobox "Fruit" [expanded]: b`, the button `button "Show
// suggestions"` and the list a listbox named by the label.
func UseCombobox[K comparable](o ComboboxOptions[K]) ComboboxResult[K] {
	const hook = "UseCombobox: ComboboxOptions."
	menuTrigger := checkChoice(hook, "MenuTrigger", o.MenuTrigger, MenuTriggerInput, MenuTriggerInput, MenuTriggerFocus, MenuTriggerManual)
	itemsMode := o.Items != nil
	if core.DevNow() {
		if itemsMode && o.Keys != nil {
			panic("aria: " + hook + "Keys and Items: give Keys (items filtered by you) or Items (filtered by the hook with Filter), not both")
		}
		if !itemsMode && o.Filter != nil {
			panic("aria: " + hook + "Filter: it filters Items; with Keys you filter the items yourself, or give them as Items")
		}
	}
	text := o.TextValue
	if text == nil {
		text = func(k K) string { return fmt.Sprint(k) }
	}
	filter := o.Filter
	if filter == nil {
		filter = Contains
	}
	o.MenuTrigger = menuTrigger

	listRef, labelRef := tela.UseElementRef(), tela.UseElementRef()
	ownInput := tela.UseElementRef()
	inputRef := o.Ref
	if inputRef == (tela.ElementRef{}) {
		inputRef = ownInput
	}
	anchor := o.AnchorRef
	if anchor == (tela.ElementRef{}) {
		anchor = inputRef
	}
	_, bump := tela.UseState(0)
	open, ost := useBoolState(o.Open, o.DefaultOpen, o.OnOpenChange)

	// The text starts as DefaultInputValue, else that of the selected item.
	initial := o.DefaultInputValue
	if initial == "" {
		sel := o.DefaultSelected
		if o.Selected.IsSet() {
			sel = o.Selected
		}
		if k, ok := sel.First(); ok {
			initial = text(k)
		}
	}
	internal, setInternal := tela.UseState(initial)
	s := tela.UseOnce(func() *comboState[K] { return &comboState[K]{} })
	s.o, s.ost, s.text, s.bump, s.itemsMode, s.filter = o, ost, text, bump, itemsMode, filter
	s.input = internal
	if o.InputValue != nil {
		s.input = *o.InputValue
	}
	s.inputControlled, s.setInternal, s.selControlled = o.InputValue != nil, setInternal, o.Selected.IsSet()
	s.matchValid = false

	visible := o.Keys
	if itemsMode {
		visible = s.visibleItems()
	}
	c := useCollection(collConfig[K]{
		kind: kindListBox, role: "listbox",
		keys: visible, textValue: text, disabled: o.DisabledKeys,
		mode: "single", behavior: "toggle", disallowEmpty: true,
		selected: o.Selected, defaultSelected: o.DefaultSelected, onSelectionChange: o.OnSelectionChange,
		wrap: o.ShouldFocusWrap, orientation: "vertical", disableType: true, focusOnHover: true,
		virtualFocus: true, external: true,
		label: o.AriaLabel, labelledBy: labelRef, ref: listRef,
		onChosen: s.chosen,
	})
	s.c = c
	if !s.started {
		s.started = true
		s.lastValue = s.input
		s.lastKey, s.lastHas = c.sel.First()
	}

	// The highlight starts when the list opens by hand and goes when it
	// closes.
	was := &tela.UseRef(false).Current
	switch {
	case open && !*was:
		if s.openStrategy != "" {
			c.focusOnOpen(s.openStrategy)
		}
		s.openStrategy = ""
	case !open && *was:
		c.closed()
	}
	*was = open

	// After every build: what changed (React Aria's effect), and the
	// highlighted item in view when the list opens on one.
	tela.UseEffect(func() func() {
		s.sync()
		return nil
	})
	tela.UseEffect(func() func() {
		if open && c.hasFocused && c.has(c.focused) {
			c.refFor(c.focused).ScrollIntoView()
		}
		return nil
	}, open)

	size := tela.UseSize(anchor)
	pop := UsePopover(PopoverOptions{
		Trigger: anchor, Open: open, OnClose: s.closeMenu,
		Placement: orDefault(o.Placement, "bottom-start"), Offset: o.Offset, Floating: o.Floating,
		// The press that leaves the input blurs it, which commits the text
		// and closes the list; the press itself reaches its target.
		ShouldCloseOnInteractOutside: func(tela.PointerEvent) bool { return false },
	})
	popover := []tela.Prop{pop.Props, h.OnPointerDown(func(e tela.PointerEvent) { e.PreventDefault() })}
	if o.MatchWidth {
		popover = append(popover, h.Style(tela.WidthFn(tela.MapValue(size, func(z tela.Size) float32 { return z.Width }))))
	}

	inputProps := []tela.Prop{
		h.Ref(inputRef), h.Role("combobox"),
		h.Value(s.input), h.OnChange(s.onChange),
		h.OnKeyDown(s.onKeyDown), h.OnFocus(s.onFocus), h.OnBlur(s.onBlur),
		h.OnCompositionStart(func() { s.composing = true }),
		h.OnCompositionEnd(func() { s.composing = false }),
		h.AriaExpanded(open), h.AriaAutoComplete("list"),
		core.AriaLabelledBy(labelRef), Collection[K]{c}.ActiveDescendant(),
	}
	if open {
		inputProps = append(inputProps, h.AriaControls(listRef))
	}
	if o.AriaLabel != "" {
		inputProps = append(inputProps, h.AriaLabel(o.AriaLabel))
	}
	if o.Disabled {
		inputProps = append(inputProps, h.Disabled(true))
	}
	if o.ReadOnly {
		inputProps = append(inputProps, h.ReadOnly(true))
	}

	press, pressed, buttonRef := usePressCore(pressConfig{
		PressOptions: PressOptions{
			Disabled: o.Disabled, PreventFocusOnPress: true,
			OnPressStart: func(e PressEvent) {
				// A mouse, a pen or a virtual click toggles at the press, a
				// touch at the release.
				if e.PointerType == PointerTouch || e.PointerType == PointerKeyboard || o.Disabled {
					return
				}
				inputRef.Focus()
				if e.PointerType == PointerVirtual {
					s.toggle(FocusFirst, "manual")
				} else {
					s.toggle("", "manual")
				}
			},
			OnPress: func(e PressEvent) {
				if e.PointerType == PointerTouch && !o.Disabled {
					inputRef.Focus()
					s.toggle("", "manual")
				}
			},
		},
		enter: true,
	})
	buttonLabel := o.ButtonLabel
	if buttonLabel == "" {
		buttonLabel = "Show suggestions"
	}
	buttonProps := []tela.Prop{
		press, h.Role("button"), h.Disabled(o.Disabled), h.TabIndex(-1),
		h.AriaLabel(buttonLabel), h.AriaHasPopup("listbox"), h.AriaExpanded(open),
	}
	if open {
		buttonProps = append(buttonProps, h.AriaControls(listRef))
	}

	return ComboboxResult[K]{
		InputProps:  core.MergeProps(inputProps...),
		ButtonProps: core.MergeProps(buttonProps...),
		LabelProps: core.MergeProps(h.Ref(labelRef), h.OnClick(func() {
			if !o.Disabled {
				inputRef.Focus()
			}
		})),
		PopoverProps: core.MergeProps(popover...),
		ArrowProps:   pop.ArrowProps,
		ListBox:      ListBoxResult[K]{Collection: Collection[K]{c}, Props: c.containerProps(), Selected: c.sel, Ref: c.ref},
		Keys:         visible,
		InputValue:   s.input,
		ShowAll:      s.showAll,
		Selected:     c.sel,
		Open:         open,
		SetOpen:      func(v bool) { s.setOpen(v) },
		Toggle:       func() { s.toggle("", "manual") },
		Close:        s.closeMenu,
		Pressed:      pressed,
		Placement:    pop.Placement,
		InputRef:     inputRef,
		ButtonRef:    buttonRef,
		PopoverRef:   pop.Ref,
	}
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ---------------------------------------------------------------------
// The items.

// matching returns the items whose text matches the input.
func (s *comboState[K]) matching() []K {
	if !s.itemsMode {
		return s.o.Keys
	}
	if !s.matchValid {
		s.match = s.match[:0:0]
		for _, k := range s.o.Items {
			if s.filter(s.text(k), s.input) {
				s.match = append(s.match, k)
			}
		}
		s.matchValid = true
	}
	return s.match
}

// visibleItems are the items of the list: all of them after the list was
// opened by hand, else the ones that match.
func (s *comboState[K]) visibleItems() []K {
	if s.showAll {
		return s.o.Items
	}
	return s.matching()
}

// total is how many items there are before filtering.
func (s *comboState[K]) total() int {
	if s.itemsMode {
		return len(s.o.Items)
	}
	return len(s.o.Keys)
}

func (s *comboState[K]) isOpen() bool { return s.ost.cur }

// selected returns the selected key.
func (s *comboState[K]) selected() (K, bool) { return s.c.sel.First() }

// selectedText is the text of the selected item, empty when there is none.
func (s *comboState[K]) selectedText() string {
	if k, ok := s.selected(); ok {
		return s.text(k)
	}
	return ""
}

// ---------------------------------------------------------------------
// State changes (react-stately's useComboBoxState).

func (s *comboState[K]) setShowAll(v bool) {
	if s.showAll != v {
		s.showAll = v
		s.bump.Update(func(n int) int { return n + 1 })
	}
}

// setInput asks for the next text: a combobox that keeps it takes it, one
// that the caller keeps only reports it.
func (s *comboState[K]) setInput(v string) {
	if v == s.input {
		return
	}
	if !s.inputControlled {
		s.input = v
		s.matchValid = false
		s.setInternal.Set(v)
	}
	if f := s.o.OnInputChange; f != nil {
		f(v)
	}
}

// resetInput puts the text of the selected item back.
func (s *comboState[K]) resetInput() {
	t := s.selectedText()
	s.lastValue = t
	s.setInput(t)
}

func (s *comboState[K]) closeMenu() {
	if s.isOpen() {
		s.ost.request(false)
	}
	s.setShowAll(false)
	s.c.clearFocus()
}

func (s *comboState[K]) setOpen(v bool) {
	if v {
		s.openMenu("", "manual")
	} else {
		s.closeMenu()
	}
}

// canOpen is React Aria's rule for opening: something to show, or a list
// that the caller filters, or a list that may be empty.
func (s *comboState[K]) canOpen(displayAll bool) bool {
	return s.o.AllowsEmptyCollection || len(s.matching()) > 0 ||
		(displayAll && s.total() > 0) || !s.itemsMode
}

func (s *comboState[K]) showsAll(trigger string) bool {
	return trigger == "manual" || (trigger == "focus" && s.o.MenuTrigger == MenuTriggerFocus)
}

// openMenu opens the list: strategy is where the highlight goes ([FocusFirst],
// [FocusLast] or "" for none), trigger what asked ("input", "focus" or
// "manual"; the last two show every item).
func (s *comboState[K]) openMenu(strategy, trigger string) {
	displayAll := s.showsAll(trigger)
	if s.o.Disabled || s.o.ReadOnly || !s.canOpen(displayAll) {
		return
	}
	if displayAll && !s.isOpen() {
		s.setShowAll(true)
	}
	s.openStrategy = strategy
	s.ost.request(true)
}

// toggle opens the list when it is closed and closes it otherwise.
func (s *comboState[K]) toggle(strategy, trigger string) {
	displayAll := s.showsAll(trigger)
	if s.o.Disabled || s.o.ReadOnly || (!s.isOpen() && !s.canOpen(displayAll)) {
		return
	}
	if s.isOpen() {
		s.closeMenu()
		return
	}
	if displayAll {
		s.setShowAll(true)
	}
	s.openStrategy = strategy
	s.ost.request(true)
}

// commitSelection ends an edit on the selected item: the text goes back to
// its text and the list closes. When the caller keeps both the text and the
// selection, it is asked instead.
func (s *comboState[K]) commitSelection() {
	if s.inputControlled && s.selControlled {
		if f := s.o.OnSelectionChange; f != nil {
			f(s.c.sel)
		}
		s.lastValue = s.selectedText()
		s.closeMenu()
		return
	}
	s.resetInput()
	s.closeMenu()
}

// commitCustomValue keeps the text as typed and clears the selection.
func (s *comboState[K]) commitCustomValue() {
	s.lastKey, s.lastHas = *new(K), false
	s.c.setSelection(SelectionOf[K]())
	s.closeMenu()
}

func (s *comboState[K]) commitValue() {
	if s.o.AllowsCustomValue {
		if s.input == s.selectedText() {
			s.commitSelection()
		} else {
			s.commitCustomValue()
		}
		return
	}
	s.commitSelection()
}

// commit is Enter and Tab: the highlighted item is chosen, else the text is
// committed.
func (s *comboState[K]) commit() {
	c := s.c
	if s.isOpen() && c.hasFocused && c.has(c.focused) {
		if c.sel.Has(c.focused) {
			s.commitSelection()
		} else {
			c.onSelect(c.focused, activation{kind: PointerKeyboard})
		}
		return
	}
	s.commitValue()
}

// revert is Escape: the text and the selection agree again.
func (s *comboState[K]) revert() {
	if _, ok := s.selected(); s.o.AllowsCustomValue && !ok {
		s.commitCustomValue()
	} else {
		s.commitSelection()
	}
}

// chosen runs when an item was chosen with the pointer or the keyboard: a
// key that already was the selection puts its text back, a new one is
// followed by sync.
func (s *comboState[K]) chosen(_ K, same bool) {
	if same {
		s.commitSelection()
		return
	}
	s.sync()
}

// sync is React Aria's effect, which runs after every build: it opens the
// list when the text changed, closes it when no item is left or an item
// was chosen, forgets the highlight and clears the selection when the text
// changed, and keeps the text of the selected item.
func (s *comboState[K]) sync() {
	o := s.o
	key, has := s.selected()
	changed := has != s.lastHas || (has && key != s.lastKey)
	if s.focused && !s.isOpen() && s.input != s.lastValue && o.MenuTrigger != MenuTriggerManual && !o.ReadOnly &&
		(o.AllowsEmptyCollection || len(s.matching()) > 0) {
		s.openMenu("", "input")
	}
	if s.isOpen() && !s.showAll && !o.AllowsEmptyCollection && len(s.matching()) == 0 {
		s.closeMenu()
	}
	if has && changed {
		s.closeMenu()
	}
	if s.input != s.lastValue {
		s.c.clearFocus()
		s.setShowAll(false)
		if s.input == "" && (!s.inputControlled || !s.selControlled) {
			s.c.setSelection(SelectionOf[K]())
		}
	}
	if changed && (!s.inputControlled || !s.selControlled) {
		s.resetInput()
	} else {
		s.lastValue = s.input
	}
	s.lastKey, s.lastHas = key, has
}

// ---------------------------------------------------------------------
// Events.

func (s *comboState[K]) onChange(e tela.ChangeEvent) {
	s.composing = e.IsComposing
	if s.o.ReadOnly || s.o.Disabled {
		return
	}
	s.setInput(e.Value)
	if s.itemsMode {
		s.sync() // the matches are known now; a caller's Keys come with the next build
	}
}

func (s *comboState[K]) onFocus() {
	s.focused = true
	if s.o.MenuTrigger == MenuTriggerFocus && !s.o.ReadOnly && !s.o.Disabled {
		s.openMenu("", "focus")
	}
}

func (s *comboState[K]) onBlur() {
	s.focused, s.composing = false, false
	s.c.clearFocus()
	if !s.o.Disabled {
		s.commitValue()
	}
}

func (s *comboState[K]) onKeyDown(e tela.KeyboardEvent) {
	if s.composing || s.o.Disabled || s.o.ReadOnly {
		if s.composing && e.Key == tela.KeyEscape {
			e.PreventDefault() // the IME's: the list stays
		}
		return
	}
	c := s.c
	switch e.Key {
	case tela.KeyEnter, tela.KeyTab:
		if s.isOpen() && e.Key == tela.KeyEnter {
			e.PreventDefault() // no implicit submission: an item may be chosen
		}
		s.commit()
	case tela.KeyEscape:
		// Escape reverts the text; it reaches an enclosing dialog only when
		// there was nothing to revert (React Aria).
		_, hasSel := s.selected()
		if s.isOpen() || !(hasSel || s.input == "" || s.o.AllowsCustomValue) {
			e.PreventDefault()
		}
		s.revert()
	case tela.KeyArrowDown, tela.KeyArrowUp:
		e.PreventDefault()
		if s.isOpen() {
			Collection[K]{c}.HandleKeyDown(e)
		} else if e.Key == tela.KeyArrowDown {
			s.openMenu(FocusFirst, "manual")
		} else {
			s.openMenu(FocusLast, "manual")
		}
	case tela.KeyArrowLeft, tela.KeyArrowRight:
		c.clearFocus() // the caret moves: no item is highlighted
	default:
		if s.isOpen() {
			Collection[K]{c}.HandleKeyDown(e) // PageUp and PageDown
		}
	}
}
