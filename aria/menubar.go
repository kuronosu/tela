// Portions of this file follow React Aria's useMenuTrigger and
// useToolbar, and react-stately's useMenuTriggerState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/menu,
// packages/@react-aria/toolbar and packages/@react-stately/menu),
// Copyright 2020 Adobe, licensed under the Apache License, Version 2.0, and
// the WAI-ARIA Authoring Practices "Menubar" pattern. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// MenuBarOptions configures [UseMenuBar]: a row of menu triggers, such as
// the File, Edit and View of an application. K is the type of the menus'
// keys; the triggers are keyed like the items of a list box (see
// [ListBoxOptions] for Keys and DisabledKeys).
//
// The open menu is a key, uncontrolled when Open is nil (the hook keeps it,
// starting at DefaultOpen) and controlled when Open points to it: the bar
// shows it and only asks OnOpenChange for the next one. A key that is not
// in Keys (the zero value of K, say) means that no menu is open.
type MenuBarOptions[K comparable] struct {
	// Keys are the keys of the menus, in the order their triggers are
	// shown. The list must be given on every build.
	Keys []K
	// DisabledKeys are the triggers that can not be focused or opened.
	DisabledKeys []K
	// Disabled disables every trigger.
	Disabled bool
	// Open points to the open menu of a controlled bar; nil leaves it to
	// the hook, which starts with DefaultOpen (nil: all closed).
	Open        *K
	DefaultOpen *K
	// OnOpenChange is called with the menu that opened (open true) or the
	// menu that closed (open false). Moving from one menu to another calls
	// it once, with the one that opens. It is also called for an
	// uncontrolled bar.
	OnOpenChange func(key K, open bool)
	// Direction is "ltr" (the default) or "rtl", which swaps ArrowLeft and
	// ArrowRight.
	Direction string
	// NoWrap keeps ArrowLeft and ArrowRight from going around the ends of
	// the bar (the pattern wraps).
	NoWrap bool
	// AriaLabel names the bar; else it is named by AriaLabelledBy.
	AriaLabel      string
	AriaLabelledBy tela.ElementRef
	// Ref is the bar element's ref, for a hook used together with others on
	// the same element; the zero value makes the hook create its own.
	Ref tela.ElementRef
}

// MenuBarResult is what [UseMenuBar] returns.
type MenuBarResult[K comparable] struct {
	// Props go to the bar's element: the "menubar" role, its orientation,
	// its name and the ref.
	Props tela.Prop
	// Open is whether a menu is open, and OpenKey which one (the zero value
	// of K when none is).
	Open    bool
	OpenKey K
	// FocusStrategy is where the focus goes when the open menu mounts:
	// [FocusFirst], [FocusLast], or "" for the menu itself (opened with a
	// pointer). Pass it to [MenuOptions].FocusStrategy.
	FocusStrategy string
	// Close closes the open menu, if any: pass it to [MenuOptions].OnClose
	// so that choosing an item closes the menu.
	Close func()
	// Ref is the ref attached to the bar by Props.
	Ref tela.ElementRef

	b *menuBar[K]
}

// menuBar is the state of a menu bar kept across builds. The handlers write
// it directly (a second event before the next build sees the first one's
// effect) and ask for a build with the rover's bump.
type menuBar[K comparable] struct {
	r   *rover[K]
	cfg MenuBarOptions[K]
	rtl bool

	cur        K
	has        bool
	controlled bool
	kept       K
	keptHas    bool
	started    bool
	strategy   string
	// switching is true while the bar moves the focus from the open menu to
	// a trigger: the blur of the old menu is not a reason to close it.
	switching bool

	menuRefs   map[K]tela.ElementRef
	press      map[K]*pressState
	setPressed tela.Setter[bool]
}

// UseMenuBar holds the state of a menu bar and returns the props of the bar
// and of its triggers and menus (the WAI-ARIA menubar pattern with React
// Aria's menu trigger behavior), on top of [UseMenu] and [UsePopover].
// Only one menu is open at a time, so the hook is called once, and the menu
// that is open is rendered by a component of its own, in a Portal:
//
//	bar := aria.UseMenuBar(aria.MenuBarOptions[string]{Keys: []string{"file", "edit"}, AriaLabel: "Application"})
//	return Fragment(
//	    Div(bar.Props, Class("flex gap-1"),
//	        h.Range(len(titles), func(i int) tela.Node {
//	            k := keys[i]
//	            return Button(bar.TriggerProps(k), Key(k), Class("rounded px-3 py-1 aria-expanded:bg-accent"), Text(titles[k]))
//	        })),
//	    Portal(If(bar.Open, tela.WithKey(appMenu{bar}, bar.OpenKey))))
//
//	type appMenu struct{ bar aria.MenuBarResult[string] }
//
//	func (m appMenu) Build() tela.Node {
//	    k := m.bar.OpenKey
//	    pop := aria.UsePopover(m.bar.PopoverOptions(k))
//	    menu := aria.UseMenu(aria.MenuOptions[string]{
//	        Keys: items[k], OnAction: run, OnClose: m.bar.Close,
//	        AutoFocus: true, FocusStrategy: m.bar.FocusStrategy, Ref: m.bar.MenuRef(k),
//	    })
//	    return Div(pop.Props, m.bar.MenuProps(k), menu.Props, Class("w-48 rounded-md border bg-popover p-1"), ...)
//	}
//
// The component has the open key as its key, so that each menu mounts anew
// and focuses its first item. The triggers are "menuitem"s with
// aria-haspopup and aria-expanded, and the bar is one tab stop that
// remembers its last trigger. ArrowLeft and ArrowRight (swapped by
// Direction "rtl") move the focus between the triggers, wrapping unless
// NoWrap, and Home and End jump; ArrowDown, Enter and Space open the menu
// of the focused trigger on its first item, ArrowUp on its last. While a
// menu is open ArrowLeft and ArrowRight close it and open the neighbor's,
// and hovering another trigger opens its menu: that is why the popover is
// not modal. A press on the open menu's trigger closes it, so do Escape,
// a press outside, and Tab, which also leaves the bar. Closing gives the
// focus back to the trigger.
//
// There are no submenus, no typeahead on the triggers and no mnemonics.
func UseMenuBar[K comparable](o MenuBarOptions[K]) MenuBarResult[K] {
	const hook = "UseMenuBar: MenuBarOptions."
	dir := checkChoice(hook, "Direction", o.Direction, "ltr", "ltr", "rtl")
	r := useRover(o.Keys, o.DisabledKeys, o.Disabled)
	_, setPressed := tela.UseState(false)
	b := tela.UseOnce(func() *menuBar[K] {
		return &menuBar[K]{r: r, menuRefs: map[K]tela.ElementRef{}, press: map[K]*pressState{}}
	})
	own := tela.UseElementRef()
	ref := o.Ref
	if ref == (tela.ElementRef{}) {
		ref = own
	}
	b.cfg, b.rtl, b.setPressed = o, dir == "rtl", setPressed
	if !b.started {
		b.started = true
		if o.DefaultOpen != nil {
			b.kept, b.keptHas = *o.DefaultOpen, true
		}
	}
	b.controlled = o.Open != nil
	if b.controlled {
		b.cur, b.has = *o.Open, true
	} else {
		b.cur, b.has = b.kept, b.keptHas
	}
	b.has = b.has && r.enabled(b.cur)
	if !b.has {
		var zero K
		b.cur = zero
		b.strategy = ""
	}
	// The refs of menus that left are dropped with those of the triggers.
	for k := range b.menuRefs {
		if !r.has(k) {
			delete(b.menuRefs, k)
			delete(b.press, k)
		}
	}

	ps := []tela.Prop{h.Ref(ref), h.Role("menubar"), h.AriaOrientation("horizontal")}
	switch {
	case o.AriaLabel != "":
		ps = append(ps, h.AriaLabel(o.AriaLabel))
	case o.AriaLabelledBy != (tela.ElementRef{}):
		ps = append(ps, h.AriaLabelledBy(o.AriaLabelledBy))
	}
	if o.Disabled {
		ps = append(ps, h.AriaDisabled(true))
	}
	return MenuBarResult[K]{
		Props: core.MergeProps(ps...),
		Open:  b.has, OpenKey: b.cur,
		FocusStrategy: b.strategy,
		Close:         b.close,
		Ref:           ref,
		b:             b,
	}
}

// IsOpen reports whether the menu with key k is open.
func (r MenuBarResult[K]) IsOpen(k K) bool { return r.b.has && r.b.cur == k }

// SetOpen opens the menu with key k (focusing the menu itself, as a pointer
// does), or closes it. It does nothing for a disabled or unknown key.
func (r MenuBarResult[K]) SetOpen(k K, open bool) {
	if open {
		r.b.open(k, "")
	} else {
		r.b.closeIf(k)
	}
}

// TriggerRef returns the ref that TriggerProps(k) attaches to the trigger
// with key k, to share with another hook on its element.
func (r MenuBarResult[K]) TriggerRef(k K) tela.ElementRef { return r.b.r.refFor(k) }

// MenuRef returns the ref of the menu with key k: pass it as
// [MenuOptions].Ref; [MenuBarResult.MenuProps] and PopoverOptions share it.
func (r MenuBarResult[K]) MenuRef(k K) tela.ElementRef { return r.b.menuRef(k) }

// TriggerProps returns the props of the trigger with key k: the "menuitem"
// role, aria-haspopup, aria-expanded and, while its menu is open,
// aria-controls; the ref, the disabled state, a tab stop on the trigger
// that had the focus last (else the first enabled one) and the handlers: a
// press opens the menu (a mouse or a pen on the press, a touch on the
// release) or closes it, hovering opens it while another menu is open, and
// the keys listed in [UseMenuBar] work.
//
// Render the trigger with a Button, or with any element that should behave
// as one.
func (r MenuBarResult[K]) TriggerProps(k K) tela.Prop {
	b := r.b
	ref := b.r.refFor(k)
	open := b.has && b.cur == k
	ps := []tela.Prop{
		h.Ref(ref), core.DefaultButtonType("button"),
		h.Role("menuitem"), h.AriaHasPopup("menu"), h.AriaExpanded(open),
	}
	if open {
		ps = append(ps, h.AriaControls(b.menuRef(k)))
	}
	if b.r.isDisabled(k) {
		return core.MergeProps(append(ps, h.Disabled(true), h.AriaDisabled(true))...)
	}
	if b.tabStop(k) {
		ps = append(ps, h.TabIndex(0))
	} else {
		ps = append(ps, h.TabIndex(-1))
	}
	st := b.press[k]
	if st == nil {
		st = &pressState{}
		b.press[k] = st
	}
	p := &presser{
		c: pressConfig{
			PressOptions: PressOptions{
				Ref: ref,
				OnPressStart: func(e PressEvent) {
					// A mouse or a pen opens the menu on the press, a touch
					// on the release; a key is the key handler's.
					if e.PointerType == PointerTouch || e.PointerType == PointerKeyboard {
						return
					}
					s := ""
					if e.PointerType == PointerVirtual {
						s = FocusFirst
					}
					if b.has && b.cur == k {
						b.close()
						return
					}
					if e.PointerType != PointerVirtual {
						// The press focuses the trigger after this handler;
						// the menu's focus scope remembers the focus when it
						// mounts, before that.
						b.focusTrigger(k)
					}
					b.open(k, s)
				},
				OnPress: func(e PressEvent) {
					if e.PointerType == PointerTouch {
						b.toggle(k, "")
					}
				},
			},
			enter: true,
		},
		st: st, ref: ref, setPressed: b.setPressed,
	}
	ps = append(ps,
		p.handlers(),
		h.OnKeyDown(func(e tela.KeyboardEvent) { b.onTriggerKey(k, e) }),
		h.OnFocus(func() { b.r.onFocus(k) }),
		h.OnPointerEnter(func(e tela.PointerEvent) {
			// Hovering the trigger of another menu while one is open opens
			// that one. A touch does not hover.
			if e.Kind == tela.PointerTouch || !b.has || b.cur == k {
				return
			}
			b.switchTo(k, "")
		}),
	)
	return core.MergeProps(ps...)
}

// MenuProps returns the props of the element of the menu with key k: its
// ref, aria-labelledby (the trigger), a focus scope that gives the focus
// back to the trigger when the menu closes, and the keys that move between
// the menus: ArrowLeft and ArrowRight open the neighbor's menu and Tab
// closes the menu and leaves the bar. Give them next to the popover's
// props and [UseMenu]'s.
func (r MenuBarResult[K]) MenuProps(k K) tela.Prop {
	b := r.b
	return core.MergeProps(
		h.Ref(b.menuRef(k)), h.AriaLabelledBy(b.r.refFor(k)),
		core.Scope(core.ScopeOptions{Restore: true}),
		h.OnKeyDown(func(e tela.KeyboardEvent) { b.onMenuKey(k, e) }),
	)
}

// PopoverOptions returns the options of [UsePopover] for the menu with key
// k: anchored below its trigger ("bottom-start"), open, closed through the
// bar, and not modal, so that the other triggers stay under the pointer:
// a press on any trigger is the trigger's own and not a press outside.
// Change what you need in the value returned (Offset, Floating).
func (r MenuBarResult[K]) PopoverOptions(k K) PopoverOptions {
	b := r.b
	return PopoverOptions{
		Trigger:   b.r.refFor(k),
		Ref:       b.menuRef(k),
		Open:      b.has && b.cur == k,
		OnClose:   func() { b.closeIf(k) },
		Placement: "bottom-start",
		ShouldCloseOnInteractOutside: func(e tela.PointerEvent) bool {
			for _, key := range b.r.keys {
				if b.r.refFor(key).Bounds().Contains(e.X, e.Y) {
					return false
				}
			}
			return true
		},
	}
}

func (b *menuBar[K]) menuRef(k K) tela.ElementRef {
	ref, ok := b.menuRefs[k]
	if !ok {
		ref = core.NewElementRef()
		b.menuRefs[k] = ref
	}
	return ref
}

// tabStop is whether the trigger with key k is the one Tab reaches.
func (b *menuBar[K]) tabStop(k K) bool {
	if b.r.hasFocused && b.r.enabled(b.r.focused) {
		return b.r.focused == k
	}
	first, ok := b.r.firstKey()
	return ok && first == k
}

// open opens the menu with key k, asking for the focus strategy s; it
// closes the one that was open.
func (b *menuBar[K]) open(k K, s string) {
	if !b.r.enabled(k) || (b.has && b.cur == k) {
		return
	}
	b.strategy = s
	if !b.controlled {
		b.cur, b.has, b.kept, b.keptHas = k, true, k, true
		b.r.invalidate()
	}
	if f := b.cfg.OnOpenChange; f != nil {
		f(k, true)
	}
}

// close closes the open menu.
func (b *menuBar[K]) close() {
	if !b.has {
		return
	}
	old := b.cur
	b.strategy = ""
	if !b.controlled {
		var zero K
		b.cur, b.has, b.kept, b.keptHas = zero, false, zero, false
		b.r.invalidate()
	}
	if f := b.cfg.OnOpenChange; f != nil {
		f(old, false)
	}
}

// closeIf closes the menu with key k if it is the open one: the popover of
// a menu that was replaced by another one closes late, and must not close
// the new one.
func (b *menuBar[K]) closeIf(k K) {
	if b.switching || !b.has || b.cur != k {
		return
	}
	b.close()
}

// toggle opens the menu with key k, or closes it when it is open.
func (b *menuBar[K]) toggle(k K, s string) {
	if b.has && b.cur == k {
		b.close()
		return
	}
	b.open(k, s)
}

// focusTrigger focuses the trigger with key k without closing the open menu
// for the blur it makes.
func (b *menuBar[K]) focusTrigger(k K) {
	was := b.switching
	b.switching = true
	b.r.refFor(k).Focus()
	b.switching = was
}

// switchTo moves to the trigger with key k and opens its menu.
func (b *menuBar[K]) switchTo(k K, s string) {
	if !b.r.enabled(k) {
		return
	}
	b.focusTrigger(k)
	b.open(k, s)
}

// onTriggerKey handles a key on the trigger with key k.
func (b *menuBar[K]) onTriggerKey(k K, e tela.KeyboardEvent) {
	// Only the trigger itself handles keys: a key typed in a field inside
	// it is the field's.
	if !b.r.refFor(k).Focused() || e.Ctrl || e.Meta || e.Alt {
		return
	}
	// moveTo goes to a trigger, opening its menu when one is open.
	moveTo := func(nk K, ok bool) {
		if !ok || nk == k {
			return
		}
		if b.has {
			b.switchTo(nk, FocusFirst)
		} else {
			b.r.refFor(nk).Focus()
		}
	}
	if d := arrowDirection(e.Key, true, b.rtl, false); d != 0 {
		e.PreventDefault()
		e.StopPropagation()
		moveTo(b.r.next(k, d > 0, !b.cfg.NoWrap))
		return
	}
	switch e.Key {
	case tela.KeyHome:
		e.PreventDefault()
		moveTo(b.r.firstKey())
	case tela.KeyEnd:
		e.PreventDefault()
		moveTo(b.r.lastKey())
	case tela.KeyEnter, " ", tela.KeyArrowDown:
		e.PreventDefault()
		e.StopPropagation()
		b.toggle(k, FocusFirst)
	case tela.KeyArrowUp:
		e.PreventDefault()
		e.StopPropagation()
		b.toggle(k, FocusLast)
	}
}

// onMenuKey handles a key that reaches the menu with key k, from one of its
// items.
func (b *menuBar[K]) onMenuKey(k K, e tela.KeyboardEvent) {
	if !b.has || b.cur != k || e.Ctrl || e.Meta || e.Alt {
		return
	}
	if d := arrowDirection(e.Key, true, b.rtl, false); d != 0 {
		e.PreventDefault()
		e.StopPropagation()
		if nk, ok := b.r.next(k, d > 0, !b.cfg.NoWrap); ok && nk != k {
			b.switchTo(nk, FocusFirst)
		}
		return
	}
	if e.Key == tela.KeyTab {
		// The focus goes back to the trigger, the only tab stop of the bar,
		// and Tab's own move then leaves the bar.
		b.focusTrigger(k)
		b.close()
	}
}
