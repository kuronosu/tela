// Portions of this file follow React Aria's useSelectableCollection,
// useSelectableItem, useSelectableList, ListKeyboardDelegate and
// useTypeSelect, and react-stately's SelectionManager and useListState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/selection,
// packages/@react-stately/selection and packages/@react-stately/list),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0.
// See THIRD_PARTY.md.

package aria

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// This file is the engine of the collection hooks (UseListBox, UseMenu
// and UseTabs): a keyed list of items with a focused key, a selection and
// the keyboard and pointer rules of React Aria's selectable collections.
// A hook can not be called once per item (a list changes length), so the
// state lives in one collState per hook call and the items get their
// props, and one ElementRef each, from ItemProps(key).

// The kinds of collection.
const (
	kindListBox = "listbox"
	kindMenu    = "menu"
	kindTabs    = "tabs"
)

// collConfig is what a collection hook gives the engine on every build.
type collConfig[K comparable] struct {
	kind string
	// keys, textValue and disabled describe the items.
	keys      []K
	textValue func(K) string
	disabled  []K
	// Selection.
	mode, behavior    string
	selected          Selection[K]
	defaultSelected   Selection[K]
	onSelectionChange func(Selection[K])
	disallowEmpty     bool
	disallowSelectAll bool
	// Keyboard.
	wrap         bool
	orientation  string
	rtl          bool
	disableType  bool
	escapeClears bool
	manual       bool // tabs: arrows move the focus without selecting
	virtualFocus bool // aria-activedescendant instead of moving the focus
	external     bool // the focus is on another element (a combobox's input)
	focusOnHover bool
	// Focus on mount.
	autoFocus bool
	strategy  string
	// Actions.
	onAction      func(K)
	onClose       func()
	closeOnSelect *bool
	// onChosen is called when the user chooses an item of a single
	// selection (a click, Enter, Space, a tap), after the selection was
	// updated, whether or not it changed: same says that the item already
	// was the selection. A select closes and a combobox commits with it.
	onChosen func(k K, same bool)
	// Element.
	ref        tela.ElementRef
	role       string
	label      string
	labelledBy tela.ElementRef
}

// collState is the state of one collection hook, on the UI thread. The
// handlers read and write it directly (a second event before the next
// build sees the first one's effect, as boolState does); bump asks for a
// build.
type collState[K comparable] struct {
	cfg  collConfig[K]
	bump tela.Setter[int]
	t    core.Timeout
	ref  tela.ElementRef

	started bool // the first build ran
	mounted bool // the mount effect ran

	keys     []K
	index    map[K]int
	disabled map[K]struct{}

	// Focus: the focused key is the item the keyboard acts on; with a
	// roving focus the element of that item has the focus of the window.
	focused    K
	hasFocused bool
	focusedIdx int
	// pending is the key whose element must take the focus as soon as it
	// exists (it is not rendered yet: a virtual list, a menu that mounts).
	pending       K
	hasPending    bool
	autoContainer bool
	leaving       bool // the mouse left an item of a menu: the menu keeps the focus with no item (onItemLeave)
	repair        bool // the focus was inside when its item vanished: give it to the key that took over
	within        bool // the focus is inside the collection
	visible       bool // and it shows as focus-visible

	// Selection.
	sel                   Selection[K]
	controlled            bool
	anchor, current       K
	hasAnchor, hasCurrent bool
	downKey               K
	hasDown               bool
	downKind              PointerType
	search                string
	refs                  map[K]tela.ElementRef
	sections              map[string]*sectionRefs
	panels                map[K]tela.ElementRef
}

type sectionRefs struct{ group, heading tela.ElementRef }

// useCollection holds the state of a collection and syncs it with the
// options of this build.
func useCollection[K comparable](cfg collConfig[K]) *collState[K] {
	_, bump := tela.UseState(0)
	own := tela.UseElementRef()
	timeout := core.UseTimeout()
	c := tela.UseOnce(func() *collState[K] {
		return &collState[K]{refs: map[K]tela.ElementRef{}, sections: map[string]*sectionRefs{}, panels: map[K]tela.ElementRef{}}
	})
	c.bump, c.t = bump, timeout
	c.ref = cfg.ref
	if c.ref == (tela.ElementRef{}) {
		c.ref = own
	}
	cfg.ref = c.ref
	c.sync(cfg)
	tela.UseMount(func() func() {
		c.mounted = true
		c.autoContainer = false
		if c.hasPending && !c.cfg.virtualFocus {
			c.scrollToKey(c.pending) // not rendered yet: bring it in
		}
		return nil
	})
	// After every build: give the focus to the key that took it when its
	// item vanished (a removed item was focused).
	tela.UseEffect(func() func() {
		if c.repair {
			c.repair = false
			if c.hasFocused && !c.cfg.virtualFocus {
				c.focusElement(c.focused)
			}
		}
		return nil
	})
	return c
}

// sprintKey is the text of an item that has no TextValue: its key as
// fmt.Sprint prints it.
func sprintKey[K comparable](k K) string { return fmt.Sprint(k) }

func (c *collState[K]) invalidate() {
	c.bump.Update(func(n int) int { return n + 1 })
}

// sync takes the options of a build.
func (c *collState[K]) sync(cfg collConfig[K]) {
	if cfg.textValue == nil {
		cfg.textValue = sprintKey[K]
	}
	c.cfg = cfg
	first := !c.started
	c.started = true
	if first {
		c.sel = cfg.defaultSelected
		if !c.sel.IsSet() {
			c.sel = SelectionOf[K]()
		}
	}
	if cfg.selected.IsSet() {
		c.controlled = true
		c.sel = cfg.selected
	} else {
		c.controlled = false
	}
	// Disabled keys: usually few. They come first: when the keys change,
	// the focus is repaired with this build's disabled set.
	if len(cfg.disabled) == 0 {
		c.disabled = nil
	} else {
		c.disabled = make(map[K]struct{}, len(cfg.disabled))
		for _, k := range cfg.disabled {
			c.disabled[k] = struct{}{}
		}
	}
	// Keys: the index is rebuilt only when they change.
	if first || !slices.Equal(c.keys, cfg.keys) {
		old := c.keys
		c.keys = slices.Clone(cfg.keys)
		c.index = make(map[K]int, len(c.keys))
		for i, k := range c.keys {
			c.index[k] = i
		}
		for k := range c.refs {
			if _, ok := c.index[k]; !ok {
				delete(c.refs, k)
			}
		}
		for k := range c.panels {
			if _, ok := c.index[k]; !ok {
				delete(c.panels, k)
			}
		}
		if !first {
			c.keysChanged(old)
		}
	}
	if first {
		if cfg.autoFocus || cfg.strategy != "" {
			c.autoFocusInit()
		}
	}
	if c.cfg.kind == kindTabs {
		c.fixTabSelection()
	}
}

// keysChanged keeps the focused key valid: when its item went, the focus
// moves to the item that took its place, else to the one before it, never
// to a disabled one (react-stately's useListState: the old index of the
// focused item, moved back by the number of items that went when more than
// one did, then the first enabled item from there on, else going back).
func (c *collState[K]) keysChanged(old []K) {
	if c.hasPending {
		if _, ok := c.index[c.pending]; !ok {
			c.hasPending = false
		}
	}
	if !c.hasFocused {
		return
	}
	if i, ok := c.index[c.focused]; ok {
		c.focusedIdx = i
		return
	}
	start := c.focusedIdx
	if i := slices.Index(old, c.focused); i >= 0 {
		start = i
	}
	i := start
	if diff := len(old) - len(c.keys); diff > 1 {
		i = max(start-diff+1, 0)
	}
	i = min(i, len(c.keys)-1)
	if i < 0 {
		c.hasFocused = false
		return
	}
	for j := i; j < len(c.keys); j++ {
		if !c.isDisabled(c.keys[j]) {
			c.focused, c.focusedIdx = c.keys[j], j
			c.repair = c.within
			return
		}
	}
	for j := i - 1; j >= 0; j-- {
		if !c.isDisabled(c.keys[j]) {
			c.focused, c.focusedIdx = c.keys[j], j
			c.repair = c.within
			return
		}
	}
	c.hasFocused = false
}

// autoFocusInit decides what takes the focus when the collection mounts:
// the first selected key, else the first or last key by the strategy, else
// the collection itself (React Aria's autoFocus of useSelectableCollection).
func (c *collState[K]) autoFocusInit() {
	var key K
	ok := false
	switch c.cfg.strategy {
	case FocusFirst:
		key, ok = c.firstKey()
	case FocusLast:
		key, ok = c.lastKey()
	}
	if k, found := c.firstSelected(); found {
		key, ok = k, true
	}
	c.within = true
	if ok {
		c.focused, c.hasFocused, c.focusedIdx = key, true, c.index[key] // a build: no state to set
		if !c.cfg.virtualFocus {
			c.pending, c.hasPending = key, true
			return
		}
	}
	// No item takes the focus: the collection does, except when the focus
	// lives on another element.
	c.autoContainer = !c.cfg.external
}

func (c *collState[K]) firstSelected() (K, bool) {
	var zero K
	if c.cfg.mode == "" || c.cfg.mode == "none" {
		return zero, false
	}
	for _, k := range c.keys {
		if c.isSelected(k) && c.canSelect(k) {
			return k, true
		}
	}
	return zero, false
}

// ---------------------------------------------------------------------
// Items.

func (c *collState[K]) isDisabled(k K) bool {
	_, d := c.disabled[k]
	return d
}

func (c *collState[K]) has(k K) bool {
	_, ok := c.index[k]
	return ok
}

// canSelect is React Aria's canSelectItem: not disabled, and the
// collection selects.
func (c *collState[K]) canSelect(k K) bool {
	return c.has(k) && !c.isDisabled(k) && c.selectable()
}

func (c *collState[K]) selectable() bool {
	return c.cfg.mode == "single" || c.cfg.mode == "multiple"
}

func (c *collState[K]) multiple() bool { return c.cfg.mode == "multiple" }

func (c *collState[K]) isSelected(k K) bool {
	if !c.selectable() {
		return false
	}
	if c.sel.IsAll() {
		return c.canSelect(k)
	}
	return c.sel.Has(k)
}

func (c *collState[K]) refFor(k K) tela.ElementRef {
	r, ok := c.refs[k]
	if !ok {
		r = core.NewElementRef()
		c.refs[k] = r
	}
	return r
}

// ---------------------------------------------------------------------
// Keyboard delegate (React Aria's ListKeyboardDelegate).

func (c *collState[K]) firstKey() (K, bool) {
	for _, k := range c.keys {
		if !c.isDisabled(k) {
			return k, true
		}
	}
	var zero K
	return zero, false
}

func (c *collState[K]) lastKey() (K, bool) {
	for i := len(c.keys) - 1; i >= 0; i-- {
		if !c.isDisabled(c.keys[i]) {
			return c.keys[i], true
		}
	}
	var zero K
	return zero, false
}

// keyBelow returns the next enabled key after k in the list.
func (c *collState[K]) keyBelow(k K) (K, bool) {
	i, ok := c.index[k]
	if !ok {
		return c.firstKey()
	}
	for j := i + 1; j < len(c.keys); j++ {
		if !c.isDisabled(c.keys[j]) {
			return c.keys[j], true
		}
	}
	var zero K
	return zero, false
}

func (c *collState[K]) keyAbove(k K) (K, bool) {
	i, ok := c.index[k]
	if !ok {
		return c.lastKey()
	}
	for j := i - 1; j >= 0; j-- {
		if !c.isDisabled(c.keys[j]) {
			return c.keys[j], true
		}
	}
	var zero K
	return zero, false
}

// stride returns the distance between two neighboring items, measured on
// the items that are rendered: the mean over the span of the mounted
// ones. It is 0 when fewer than two are laid out.
func (c *collState[K]) stride() float32 {
	lo, hi := -1, -1
	var loY, hiY float32
	for k, r := range c.refs {
		b := r.Bounds()
		if b.Height == 0 {
			continue
		}
		i := c.index[k]
		if lo < 0 || i < lo {
			lo, loY = i, b.Y
		}
		if hi < 0 || i > hi {
			hi, hiY = i, b.Y
		}
	}
	if hi <= lo {
		return 0
	}
	return (hiY - loY) / float32(hi-lo)
}

func (c *collState[K]) horizontal() bool { return c.cfg.orientation == "horizontal" }

// pageKey is React Aria's getKeyPageBelow and getKeyPageAbove: the item a
// viewport away from k (the first one that starts past the viewport's
// height), the last or first when none is. Items that are not rendered
// (a virtual list) count by the stride of those that are.
func (c *collState[K]) pageKey(k K, down bool) (K, bool) {
	i, ok := c.index[k]
	if !ok {
		return c.firstKey()
	}
	view := c.ref.Bounds().Height
	step := c.stride()
	var n int
	if view <= 0 || step <= 0 {
		n = len(c.keys) // not laid out, not scrollable: the end
	} else {
		n = int((view + step - 1) / step)
		n = max(n, 1)
	}
	var j int
	if down {
		j = min(i+n, len(c.keys)-1)
		for ; j < len(c.keys); j++ {
			if !c.isDisabled(c.keys[j]) {
				return c.keys[j], true
			}
		}
		return c.lastKey()
	}
	j = max(i-n, 0)
	for ; j >= 0; j-- {
		if !c.isDisabled(c.keys[j]) {
			return c.keys[j], true
		}
	}
	return c.firstKey()
}

// keyForSearch is getKeyForSearch: the first enabled item from from (it
// included) whose text starts with the search, ignoring case.
func (c *collState[K]) keyForSearch(search string, from K, hasFrom bool) (K, bool) {
	var zero K
	start := 0
	if hasFrom {
		if i, ok := c.index[from]; ok {
			start = i
		}
	}
	for i := start; i < len(c.keys); i++ {
		k := c.keys[i]
		if c.isDisabled(k) {
			continue
		}
		if hasPrefixFold(c.cfg.textValue(k), search) {
			return k, true
		}
	}
	return zero, false
}

// hasPrefixFold reports whether s starts with prefix, ignoring case (the
// base sensitivity of React Aria's collator, for the letters that have a
// simple case).
func hasPrefixFold(s, prefix string) bool {
	for _, p := range prefix {
		if s == "" {
			return false
		}
		r, n := utf8.DecodeRuneInString(s)
		if unicode.ToLower(r) != unicode.ToLower(p) {
			return false
		}
		s = s[n:]
	}
	return true
}

// ---------------------------------------------------------------------
// Focus.

// setFocusedState records k as the focused key.
func (c *collState[K]) setFocusedState(k K) {
	if c.hasFocused && c.focused == k {
		return
	}
	c.focused, c.hasFocused = k, true
	c.focusedIdx = c.index[k]
	c.invalidate()
}

// focusElement gives the DOM focus to the element of k; when it is not
// rendered (a virtual list) it is brought in and takes the focus when it
// mounts.
func (c *collState[K]) focusElement(k K) {
	r := c.refFor(k)
	r.Focus()
	if r.Focused() {
		c.hasPending = false
		return
	}
	c.pending, c.hasPending = k, true
	c.scrollToKey(k)
	c.invalidate()
}

// scrollToKey brings an item that is not rendered into view: a virtual
// list scrolls to its index.
func (c *collState[K]) scrollToKey(k K) {
	if i, ok := c.index[k]; ok {
		c.ref.ScrollToIndex(i, tela.AlignAuto)
	}
}

// moveFocus makes k the focused key and moves the focus to it: the
// element of its item (a roving focus) or the highlight (a virtual one,
// which scrolls the item into view).
func (c *collState[K]) moveFocus(k K) {
	c.setFocusedState(k)
	if c.cfg.virtualFocus {
		r := c.refFor(k)
		r.ScrollIntoView()
		if r.Bounds().Height == 0 {
			c.scrollToKey(k)
		}
		return
	}
	c.focusElement(k)
}

// clearFocus forgets the focused key (the highlight of a combobox goes
// when it closes).
func (c *collState[K]) clearFocus() {
	if !c.hasFocused && !c.hasPending {
		return
	}
	c.hasFocused, c.hasPending = false, false
	c.invalidate()
}

// syncFocus reads the engine for the focus inside the collection (the
// focus moves without events when a key follows a click).
func (c *collState[K]) syncFocus() {
	within := core.ContainsFocus(c.ref)
	visible := within && core.FocusVisibleWithin(c.ref)
	if within != c.within || visible != c.visible {
		c.within, c.visible = within, visible
		c.invalidate()
	}
}

// ---------------------------------------------------------------------
// Selection (react-stately's SelectionManager).

// setSelection takes a new selection: an uncontrolled collection keeps
// it, a controlled one only reports it. An equal selection is not
// reported.
func (c *collState[K]) setSelection(next Selection[K]) {
	if c.sel.Equal(next) {
		return
	}
	if !c.controlled {
		c.sel = next
		c.invalidate()
	}
	if f := c.cfg.onSelectionChange; f != nil {
		f(next)
	}
}

func (c *collState[K]) setAnchor(k K) {
	c.anchor, c.hasAnchor = k, true
	c.current, c.hasCurrent = k, true
}

func (c *collState[K]) replaceSelection(k K) {
	if !c.canSelect(k) {
		return
	}
	c.setAnchor(k)
	c.setSelection(SelectionOf(k))
}

func (c *collState[K]) toggleSelection(k K) {
	if !c.selectable() {
		return
	}
	if c.cfg.mode == "single" && !c.isSelected(k) {
		c.replaceSelection(k)
		return
	}
	cur := c.sel
	if cur.IsAll() {
		cur = SelectionOf(c.selectableKeys()...)
	}
	if cur.Has(k) {
		cur = cur.Without(k)
	} else if c.canSelect(k) {
		cur = cur.With(k)
		c.setAnchor(k)
	} else {
		return
	}
	if c.cfg.disallowEmpty && cur.IsEmpty() {
		return
	}
	c.setSelection(cur)
}

func (c *collState[K]) selectableKeys() []K {
	var out []K
	for _, k := range c.keys {
		if c.canSelect(k) {
			out = append(out, k)
		}
	}
	return out
}

// range returns the keys from a to b inclusive, in list order whichever
// comes first.
func (c *collState[K]) keyRange(a, b K) []K {
	i, ok1 := c.index[a]
	j, ok2 := c.index[b]
	if !ok1 || !ok2 {
		return nil
	}
	if i > j {
		i, j = j, i
	}
	return c.keys[i : j+1]
}

// extendSelection selects the range from the anchor to k, replacing the
// range from the anchor to the previous end (Shift+Arrow, Shift+click).
func (c *collState[K]) extendSelection(k K) {
	if !c.selectable() {
		return
	}
	if c.cfg.mode == "single" {
		c.replaceSelection(k)
		return
	}
	if c.sel.IsAll() {
		c.setAnchor(k)
		c.setSelection(SelectionOf(k))
		return
	}
	anchor := k
	if c.hasAnchor {
		anchor = c.anchor
	}
	end := k
	if c.hasCurrent {
		end = c.current
	}
	next := c.sel
	for _, key := range c.keyRange(anchor, end) {
		next = next.Without(key)
	}
	for _, key := range c.keyRange(k, anchor) {
		if c.canSelect(key) {
			next = next.With(key)
		}
	}
	c.anchor, c.hasAnchor = anchor, true
	c.current, c.hasCurrent = k, true
	c.setSelection(next)
}

func (c *collState[K]) selectAll() {
	if c.multiple() && !c.sel.IsAll() && !c.cfg.disallowSelectAll {
		c.setSelection(All[K]())
	}
}

func (c *collState[K]) clearSelection() {
	if c.cfg.disallowEmpty || c.sel.IsEmpty() {
		return
	}
	c.setSelection(SelectionOf[K]())
}

// ctrlKey is React Aria's isCtrlKeyPressed: Command on a Mac, Control
// elsewhere.
func ctrlKey(ctrl, meta bool) bool {
	if runtime.GOOS == "darwin" {
		return meta
	}
	return ctrl
}

// selectOnFocus is whether moving the focus selects (selection behavior
// "replace", and tabs with automatic activation).
func (c *collState[K]) selectOnFocus() bool {
	if c.cfg.kind == kindTabs {
		return !c.cfg.manual
	}
	return c.cfg.behavior == "replace"
}

// ---------------------------------------------------------------------
// Actions and selecting with a pointer or the keyboard (useSelectableItem).

// hasAction is React Aria's hasPrimaryAction and hasSecondaryAction for
// k. A menu runs its action on every item instead (see activate).
func (c *collState[K]) actions(k K) (primary, secondary bool) {
	allowsSelection := !c.isDisabled(k) && c.selectable()
	allowsActions := c.cfg.onAction != nil && !c.isDisabled(k)
	if c.cfg.behavior == "replace" {
		primary = allowsActions && !allowsSelection
	} else {
		primary = allowsActions && c.sel.IsEmpty()
	}
	secondary = allowsActions && allowsSelection && c.cfg.behavior == "replace"
	return
}

type activation struct {
	kind                   PointerType
	shift, ctrl, alt, meta bool
}

// onSelect is React Aria's onSelect of useSelectableItem.
func (c *collState[K]) onSelect(k K, a activation) {
	if a.kind == PointerKeyboard && ctrlKey(a.ctrl, a.meta) {
		c.toggleSelection(k)
		return
	}
	switch {
	case !c.selectable():
	case c.cfg.mode == "single":
		was := c.isSelected(k)
		if was && !c.cfg.disallowEmpty {
			c.toggleSelection(k)
		} else {
			c.replaceSelection(k)
		}
		if f := c.cfg.onChosen; f != nil && c.canSelect(k) {
			f(k, was)
		}
	case a.shift:
		c.extendSelection(k)
	case c.cfg.behavior != "replace" || ctrlKey(a.ctrl, a.meta) || a.kind == PointerTouch || a.kind == PointerVirtual:
		// A touch or a virtual press has no modifier keys: it toggles,
		// or multiple selection would be impossible.
		c.toggleSelection(k)
	default:
		c.replaceSelection(k)
	}
}

func (c *collState[K]) performAction(k K) {
	if f := c.cfg.onAction; f != nil {
		f(k)
	}
}

// activate is what choosing an item does in a menu (and in tabs): select
// it, run the action, and close the menu unless the selection is
// multiple.
func (c *collState[K]) activate(k K, a activation) {
	if c.isDisabled(k) {
		return
	}
	c.onSelect(k, a)
	c.performAction(k)
	if f := c.cfg.onClose; f != nil {
		if cs := c.cfg.closeOnSelect; (cs != nil && *cs) || (cs == nil && !c.multiple()) {
			f()
		}
	}
}

// press is a click, a tap, a virtual click or Enter and Space.
func (c *collState[K]) press(k K, a activation) {
	if c.isDisabled(k) {
		return
	}
	c.setFocusedState(k)
	if c.cfg.kind != kindListBox {
		c.activate(k, a)
		return
	}
	primary, secondary := c.actions(k)
	switch {
	case a.kind == PointerKeyboard:
		return // Enter and Space have their own rules (onKey)
	case primary || (secondary && a.kind != PointerMouse):
		c.performAction(k)
	case c.selectable():
		c.onSelect(k, a)
	}
}

// ---------------------------------------------------------------------
// Props.

func (c *collState[K]) itemRole() string {
	switch c.cfg.kind {
	case kindMenu:
		switch c.cfg.mode {
		case "single":
			return "menuitemradio"
		case "multiple":
			return "menuitemcheckbox"
		}
		return "menuitem"
	case kindTabs:
		return "tab"
	}
	return "option"
}

// itemProps returns the props of the item with key k.
func (c *collState[K]) itemProps(k K) tela.Prop {
	disabled := c.isDisabled(k)
	ps := []tela.Prop{h.Ref(c.refFor(k)), h.Role(c.itemRole())}
	selected := c.isSelected(k)
	switch c.cfg.kind {
	case kindListBox:
		if c.selectable() {
			ps = append(ps, h.AriaSelected(selected))
		}
	case kindMenu:
		if c.selectable() {
			ps = append(ps, h.AriaChecked(selected))
		}
	case kindTabs:
		ps = append(ps, h.AriaSelected(selected))
		if selected {
			ps = append(ps, h.AriaControls(c.panelRef(k)))
		}
	}
	if disabled {
		ps = append(ps, h.Disabled(true), h.AriaDisabled(true))
		if c.cfg.focusOnHover && c.cfg.kind == kindMenu {
			// The mouse over a disabled item clears the highlight, as
			// Radix's MenuItem calls onItemLeave for it.
			ps = append(ps, h.OnPointerMove(func(e tela.PointerEvent) { c.onItemLeave(e) }))
		}
		return core.MergeProps(ps...)
	}
	if !c.cfg.virtualFocus {
		stop := c.hasFocused && c.focused == k
		if c.cfg.kind == kindTabs {
			stop = selected
		}
		if stop {
			ps = append(ps, h.TabIndex(0))
		} else {
			ps = append(ps, h.TabIndex(-1))
		}
		if c.hasPending && c.pending == k {
			ps = append(ps, h.AutoFocus(true))
		}
	}
	ps = append(ps,
		h.OnPointerDown(func(e tela.PointerEvent) { c.onItemPointerDown(k, e) }),
		h.OnClick(func(e tela.MouseEvent) { c.onItemClick(k, e) }),
		h.OnFocus(func() { c.onItemFocus(k) }),
	)
	if c.cfg.kind == kindListBox {
		ps = append(ps, h.OnDoubleClick(func(e tela.MouseEvent) { c.onItemDoubleClick(k, e) }))
	}
	if c.cfg.focusOnHover {
		ps = append(ps, h.OnPointerMove(func(e tela.PointerEvent) { c.onItemHover(k, e) }))
		if c.cfg.kind == kindMenu {
			ps = append(ps, h.OnPointerLeave(func(e tela.PointerEvent) { c.onItemLeave(e) }))
		}
	}
	return core.MergeProps(ps...)
}

func (c *collState[K]) onItemPointerDown(k K, e tela.PointerEvent) {
	if e.Button != tela.MouseButtonLeft {
		return
	}
	c.downKey, c.hasDown, c.downKind = k, true, pointerTypeOf(e.Kind)
	if c.cfg.virtualFocus {
		// Keep the focus where it is (the combobox's input, or the list
		// itself) and highlight the item.
		if c.cfg.external {
			e.PreventDefault()
		}
		if c.cfg.kind != kindMenu {
			c.setFocusedState(k)
		}
	}
}

func (c *collState[K]) onItemClick(k K, e tela.MouseEvent) {
	kind := PointerVirtual
	if c.hasDown && c.downKey == k {
		kind = c.downKind
	}
	c.hasDown = false
	c.press(k, activation{kind: kind, shift: e.Shift, ctrl: e.Ctrl, alt: e.Alt, meta: e.Meta})
}

func (c *collState[K]) onItemDoubleClick(k K, e tela.MouseEvent) {
	if _, secondary := c.actions(k); secondary && c.downKind == PointerMouse {
		e.StopPropagation()
		e.PreventDefault()
		c.performAction(k)
	}
}

func (c *collState[K]) onItemFocus(k K) {
	if !c.refFor(k).Focused() {
		return // a descendant of the item took the focus
	}
	c.hasPending = false
	c.setFocusedState(k)
}

func (c *collState[K]) onItemHover(k K, e tela.PointerEvent) {
	if e.Kind == tela.PointerTouch || (c.hasFocused && c.focused == k && c.refFor(k).Focused()) {
		return
	}
	c.within = true
	c.moveFocus(k)
}

// onItemLeave is Radix's onItemLeave of a menu: when the mouse leaves an
// item (for the gap between items, a separator, a disabled item or the
// outside of the menu) nothing is highlighted, and the focus stays on the
// menu itself, so the keys still work: ArrowDown goes to the first item,
// ArrowUp to the last, as on a menu that opened under the mouse. Only the
// mouse's own movement counts: the leave that a new hit test fires when
// the menu scrolls under a resting pointer (the keys moved the highlight
// out of view) is not one, or ArrowDown would start over from the first
// item; in Radix scrolling fires no pointer events either.
func (c *collState[K]) onItemLeave(e tela.PointerEvent) {
	if e.Kind == tela.PointerTouch || !c.within || core.Rehovered(e) {
		return
	}
	c.clearFocus()
	c.leaving = true
	c.ref.Focus()
	c.leaving = false
	c.within = true
}

// containerProps are the props of the element that holds the items.
func (c *collState[K]) containerProps() tela.Prop {
	ps := []tela.Prop{h.Ref(c.ref), h.Role(c.cfg.role)}
	if c.cfg.kind == kindListBox && c.multiple() {
		ps = append(ps, h.AriaMultiSelectable(true))
	}
	if c.cfg.orientation != "" && c.cfg.kind != kindMenu {
		ps = append(ps, h.AriaOrientation(c.cfg.orientation))
	}
	if c.cfg.label != "" {
		ps = append(ps, h.AriaLabel(c.cfg.label))
	}
	if c.cfg.labelledBy != (tela.ElementRef{}) {
		ps = append(ps, h.AriaLabelledBy(c.cfg.labelledBy))
	}
	if c.cfg.external {
		return core.MergeProps(ps...)
	}
	if c.cfg.kind != kindTabs {
		switch {
		case c.cfg.virtualFocus:
			ps = append(ps, h.TabIndex(0))
			if c.hasFocused && c.has(c.focused) {
				ps = append(ps, h.AriaActiveDescendant(c.refFor(c.focused)))
			}
		case !c.hasFocused:
			ps = append(ps, h.TabIndex(0))
		default:
			ps = append(ps, h.TabIndex(-1))
		}
		if c.autoContainer {
			ps = append(ps, h.AutoFocus(true))
		}
	}
	ps = append(ps,
		h.OnKeyDown(c.onKeyDown),
		h.OnFocus(c.onFocus),
		h.OnBlur(func() {
			c.within, c.visible = false, false
			c.invalidate()
		}),
		h.OnPointerDown(c.syncFocus),
	)
	return core.MergeProps(ps...)
}

// onFocus runs when the focus enters the collection or moves inside it.
func (c *collState[K]) onFocus() {
	was := c.within
	c.syncFocus()
	if !c.ref.Focused() || c.cfg.kind == kindTabs {
		return
	}
	// The collection itself took the focus (Tab, or a press on it or on a
	// disabled item). With a roving focus it goes to the focused item, else
	// to the first selected one or the first (React Aria's onFocus).
	switch {
	case c.leaving:
		// The menu keeps the focus with no item highlighted (onItemLeave).
	case c.hasFocused && !c.cfg.virtualFocus:
		c.focusElement(c.focused)
	case !was && !c.hasFocused:
		if k, ok := c.firstSelected(); ok {
			c.moveFocus(k)
		} else if k, ok := c.firstKey(); ok {
			c.moveFocus(k)
		}
	}
}

// onKeyDown is useSelectableCollection's key handler with useTypeSelect.
func (c *collState[K]) onKeyDown(e tela.KeyboardEvent) {
	c.syncFocus()
	if !c.cfg.external && !c.ref.Focused() && !(c.hasFocused && c.refFor(c.focused).Focused()) {
		return // a key typed in a field inside an item is the field's
	}
	c.handleKey(e)
}

func (c *collState[K]) handleKey(e tela.KeyboardEvent) {
	if c.typeSelect(e) {
		return
	}
	ctrl := ctrlKey(e.Ctrl, e.Meta)
	vertical := !c.horizontal()
	next := func(k K, ok bool) {
		if ok {
			c.navigate(k, e)
		}
	}
	forward := func(wrapTo func() (K, bool)) {
		var (
			k  K
			ok bool
		)
		if c.hasFocused {
			k, ok = c.keyBelow(c.focused)
		} else {
			k, ok = c.firstKey()
		}
		if !ok && c.cfg.wrap {
			k, ok = wrapTo()
		}
		next(k, ok)
	}
	backward := func(wrapTo func() (K, bool)) {
		var (
			k  K
			ok bool
		)
		if c.hasFocused {
			k, ok = c.keyAbove(c.focused)
		} else {
			k, ok = c.lastKey()
		}
		if !ok && c.cfg.wrap {
			k, ok = wrapTo()
		}
		next(k, ok)
	}
	switch e.Key {
	case tela.KeyArrowDown:
		if vertical {
			e.PreventDefault()
			forward(c.firstKey)
		}
	case tela.KeyArrowUp:
		if vertical {
			e.PreventDefault()
			backward(c.lastKey)
		}
	case tela.KeyArrowRight, tela.KeyArrowLeft:
		if !vertical {
			e.PreventDefault()
			if (e.Key == tela.KeyArrowRight) != c.cfg.rtl {
				forward(c.firstKey)
			} else {
				backward(c.lastKey)
			}
		}
	case tela.KeyHome:
		if c.cfg.external {
			return // the text field's cursor
		}
		e.PreventDefault()
		if k, ok := c.firstKey(); ok {
			c.moveFocus(k)
			switch {
			case ctrl && e.Shift && c.multiple():
				c.extendSelection(k)
			case c.selectOnFocus():
				c.replaceSelection(k)
			}
		}
	case tela.KeyEnd:
		if c.cfg.external {
			return
		}
		e.PreventDefault()
		if k, ok := c.lastKey(); ok {
			c.moveFocus(k)
			switch {
			case ctrl && e.Shift && c.multiple():
				c.extendSelection(k)
			case c.selectOnFocus():
				c.replaceSelection(k)
			}
		}
	case tela.KeyPageDown:
		if c.hasFocused && vertical {
			e.PreventDefault()
			next(c.pageKey(c.focused, true))
		}
	case tela.KeyPageUp:
		if c.hasFocused && vertical {
			e.PreventDefault()
			next(c.pageKey(c.focused, false))
		}
	case tela.KeyEscape:
		if c.cfg.escapeClears && !c.cfg.external && !c.cfg.disallowEmpty && !c.sel.IsEmpty() {
			e.StopPropagation()
			e.PreventDefault()
			c.clearSelection()
		}
	case tela.KeyEnter, " ":
		if e.Key == " " && c.cfg.external {
			return // a space typed in the text field
		}
		if c.hasFocused && c.has(c.focused) && !e.Repeat {
			e.PreventDefault()
			e.StopPropagation()
			c.onKey(c.focused, e)
		}
	default:
		if (e.Key == "a" || e.Key == "A") && ctrl && c.multiple() && !c.cfg.disallowSelectAll && !c.cfg.external {
			e.PreventDefault()
			c.selectAll()
		}
	}
}

// navigate moves the focus to k, and the selection with it as the
// modifiers and the selection behavior say.
func (c *collState[K]) navigate(k K, e tela.KeyboardEvent) {
	c.moveFocus(k)
	switch {
	case e.Shift && c.multiple():
		c.extendSelection(k)
	case c.selectOnFocus() && !ctrlKey(e.Ctrl, e.Meta):
		c.replaceSelection(k)
	}
}

// onKey is Enter or Space on the focused item: the keyboard press of
// useSelectableItem.
func (c *collState[K]) onKey(k K, e tela.KeyboardEvent) {
	if c.isDisabled(k) {
		return
	}
	a := activation{kind: PointerKeyboard, shift: e.Shift, ctrl: e.Ctrl, alt: e.Alt, meta: e.Meta}
	if c.cfg.kind != kindListBox {
		c.activate(k, a)
		return
	}
	primary, secondary := c.actions(k)
	hasAction := primary || secondary
	switch {
	case e.Key == tela.KeyEnter && hasAction:
		c.performAction(k)
	case !hasAction || e.Key == " ":
		if c.selectable() {
			c.onSelect(k, a)
		}
	}
}

// typeAhead is useTypeSelect's buffer: printable keys build a search
// string that expires after a second without keys. It returns the first
// enabled item from from (when hasFrom, else from the top) whose text
// starts with the search, else the first of the whole list. took reports
// that the key was a space inside a search, which is a character and not a
// selection (it is stopped and prevented). With typeahead disabled
// (DisallowTypeAhead) or for a key that is not a character it does nothing.
func (c *collState[K]) typeAhead(e tela.KeyboardEvent, from K, hasFrom bool) (k K, found, took bool) {
	if c.cfg.disableType || e.Ctrl || e.Meta {
		return
	}
	s := string(e.Key)
	if utf8.RuneCountInString(s) != 1 {
		return
	}
	if r, _ := utf8.DecodeRuneInString(s); !unicode.IsPrint(r) {
		return
	}
	if c.search == "" && s == " " {
		return
	}
	took = s == " " && strings.TrimSpace(c.search) != ""
	if took {
		e.PreventDefault()
		e.StopPropagation()
	}
	c.search += s
	k, found = c.keyForSearch(c.search, from, hasFrom)
	if !found {
		k, found = c.keyForSearch(c.search, from, false)
	}
	c.t.Start(time.Second, func() { c.search = "" })
	return
}

// typeSelect is useTypeSelect: the focus goes to the item typeAhead finds,
// starting at the focused one. It reports whether it took the key.
func (c *collState[K]) typeSelect(e tela.KeyboardEvent) bool {
	k, ok, took := c.typeAhead(e, c.focused, c.hasFocused)
	if ok {
		c.moveFocus(k)
	}
	return took
}

// focusOnOpen decides what has the focus when a collection that is
// rendered only while it is open opens (a select's list): the first
// selected item, else the first or last by the strategy ([FocusFirst],
// [FocusLast]), else the collection itself. It runs in the build that shows
// the collection, before the items are rendered, so the key waits for its
// element (pending) and the container asks for the focus when it mounts.
func (c *collState[K]) focusOnOpen(strategy string) {
	c.hasPending, c.autoContainer = false, false
	c.within = true
	var key K
	ok := false
	switch strategy {
	case FocusFirst:
		key, ok = c.firstKey()
	case FocusLast:
		key, ok = c.lastKey()
	}
	if k, found := c.firstSelected(); found {
		key, ok = k, true
	}
	if !ok {
		c.hasFocused = false
		c.autoContainer = !c.cfg.external
		return
	}
	c.focused, c.hasFocused, c.focusedIdx = key, true, c.index[key]
	if !c.cfg.virtualFocus {
		c.pending, c.hasPending = key, true
	}
}

// closed forgets the focus of a collection that closed: its elements are
// gone, and the next opening decides again.
func (c *collState[K]) closed() {
	c.hasPending, c.hasFocused, c.autoContainer = false, false, false
	c.within, c.visible = false, false
	c.search = ""
}

// ---------------------------------------------------------------------
// Tabs.

func (c *collState[K]) panelRef(k K) tela.ElementRef {
	r, ok := c.panels[k]
	if !ok {
		r = core.NewElementRef()
		c.panels[k] = r
	}
	return r
}

// fixTabSelection keeps one enabled tab selected (useTabListState): the
// first one when nothing, or something that is gone or disabled, is.
func (c *collState[K]) fixTabSelection() {
	if k, ok := c.sel.First(); ok && c.has(k) && !c.isDisabled(k) {
		return
	}
	k, ok := c.firstKey()
	if !ok {
		return
	}
	if c.controlled {
		c.sel = SelectionOf(k) // shown; the caller's state is not touched
		return
	}
	c.sel = SelectionOf(k)
	c.setAnchor(k)
}
