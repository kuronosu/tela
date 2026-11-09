// Portions of this file follow React Aria's useSubmenuTrigger and
// useSafelyMouseToSubmenu, and react-stately's useSubmenuTriggerState
// (https://github.com/adobe/react-spectrum, packages/@react-aria/menu and
// packages/@react-stately/menu), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. The behavior also follows Radix UI's
// MenuSub, MenuSubTrigger and MenuSubContent (MIT; no code copied). See
// THIRD_PARTY.md.

package aria

import (
	"math"
	"time"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// SubmenuDelay is how long the pointer rests on a submenu trigger before
// its submenu opens (React Aria's default delay of useSubmenuTrigger).
const SubmenuDelay = 200 * time.Millisecond

// The pointer grace of a submenu (React Aria's useSafelyMouseToSubmenu):
// while the mouse moves from a submenu's trigger toward the submenu, the
// items of the parent menu it crosses are not hovered, so a diagonal move
// does not close the submenu.
const (
	// graceMoves is how many moves toward the submenu in a row start the
	// grace (and how many moves away end it).
	graceMoves = 2
	// graceThrottle is the time between two moves the grace looks at.
	graceThrottle = 50 * time.Millisecond
	// graceTimeout ends the grace when the mouse rests that long.
	graceTimeout = time.Second
	// graceAngle widens the cone toward the submenu's corners by 15°.
	graceAngle = math.Pi / 12
)

// SubmenuTriggerResult is the state of one submenu of a menu, returned by
// [MenuResult.SubmenuTrigger] for an item of [MenuOptions].SubmenuKeys: what
// the component that renders the submenu while it is open needs (React
// Aria's useSubmenuTrigger and useSubmenuTriggerState; Radix's MenuSub).
//
// The trigger item's own props (aria-haspopup, aria-expanded,
// aria-controls, the keys and the hover that open the submenu) are in
// [Collection.ItemProps] already.
type SubmenuTriggerResult struct {
	// MenuProps go to the submenu's element, with [UsePopover]'s props
	// (PopoverOptions) and the submenu's own [UseMenu] props: the ref,
	// aria-labelledby the trigger item, and the keys that close the
	// submenu and give the focus back to its trigger (ArrowLeft, or
	// ArrowRight under "rtl", and Escape).
	MenuProps tela.Prop
	// Open is whether the submenu is open.
	Open bool
	// FocusStrategy is where the focus goes when the submenu opens:
	// [FocusFirst] when the keyboard (ArrowRight, Enter, Space) or an
	// assistive technology opened it, "" when the pointer did (the focus
	// stays on the trigger until the pointer moves into the submenu). The
	// submenu's [UseMenu] reads it through its Submenu option.
	FocusStrategy string
	// Close closes this submenu (and the ones it opened); CloseAll closes
	// the whole menu, through the root menu's [MenuOptions].OnClose.
	Close, CloseAll func()
	// Ref is the trigger item's ref and MenuRef the submenu's.
	Ref, MenuRef tela.ElementRef
	// Direction is the parent menu's ("ltr" or "rtl"): the submenu opens on
	// the right of its trigger, or on its left under "rtl".
	Direction string

	lvl submenuLevel // the menu the trigger is in; nil for the zero value
}

// PopoverOptions returns the options of [UsePopover] for the submenu:
// beside its trigger item ("right-start", "left-start" under "rtl"),
// flipped and shifted to stay in the window, not modal. A submenu that
// fits on neither side of its parent goes on the side that overflows
// least and is then shifted across into the window, over its parent, as
// the native menus of Windows, GTK and macOS do (Floating UI's shift with
// crossAxis, ADR-260; Floating's first option). Escape and the key
// back close this submenu only, a press outside every menu of the chain
// closes the whole menu, and a press inside one of the parent menus is
// left to that menu (it moves the focus, which closes the submenu, or
// chooses an item). Change what you need in the value returned (Offset;
// append to Floating, for a cross-axis offset that lines the first item
// up with the trigger).
func (r SubmenuTriggerResult) PopoverOptions() PopoverOptions {
	placement := "right-start"
	if r.Direction == "rtl" {
		placement = "left-start"
	}
	o := PopoverOptions{
		Trigger: r.Ref, Ref: r.MenuRef, Open: r.Open, Placement: placement,
		OnClose: r.Close,
		// After the popover's flip and shift (which slides along the
		// trigger only): across, flush with the window's edge.
		Floating: []FloatingOption{Shift(0, ShiftOptions{NoMainAxis: true, CrossAxis: true})},
	}
	lvl := r.lvl
	if lvl == nil {
		return o
	}
	o.ShouldCloseOnInteractOutside = func(e tela.PointerEvent) bool {
		for _, m := range lvl.chain() {
			if m.Bounds().Contains(e.X, e.Y) {
				return false
			}
		}
		// A press the root menu's popover lets through (a menu bar's other
		// buttons) reaches its target: it switches menus, which unmounts
		// this one with its root.
		if f := lvl.rootOutside(); f != nil && !f(e) {
			return false
		}
		*lvl.outside() = true
		return true
	}
	o.OnClose = func() {
		all := lvl.outside()
		if *all {
			*all = false
			r.CloseAll()
			return
		}
		r.Close()
	}
	return o
}

// submenuLevel is the menu a submenu trigger is in, as its submenus see
// it.
type submenuLevel interface {
	// chain is the menus from the root to this one.
	chain() []tela.ElementRef
	// rootOutside is the root menu's ShouldCloseOnInteractOutside, nil
	// when it has none.
	rootOutside() func(tela.PointerEvent) bool
	// closeAllMenus closes the whole menu.
	closeAllMenus()
	// outside is set by a press outside the chain, so that the submenu's
	// close is the whole menu's.
	outside() *bool
	// setChild records the open submenu's collection, and dropChild
	// forgets it when it unmounts.
	setChild(c subChild)
	dropChild(c subChild)
}

// subChild is the collection of an open submenu, as its parent sees it.
type subChild interface {
	// focusEdge focuses the first or last item ([FocusFirst],
	// [FocusLast]).
	focusEdge(strategy string)
}

// subOptions is what a collection keeps of the submenu it is (the
// Submenu option of UseMenu), and of its own submenus.
type subOptions[K comparable] struct {
	keys     map[K]bool
	menuRefs map[K]tela.ElementRef

	// The open submenu of this level: one at a time.
	open     bool
	key      K
	strategy string
	child    subChild
	// out is set by a press outside the chain (PopoverOptions).
	out bool

	// The open delay and the pointer grace toward the open submenu.
	openT, graceT core.Timeout
	grace         bool
	toward        int
	hasPrev       bool
	prevX, prevY  float32
	last          time.Duration
	// pending is the item the pointer moved over during the grace,
	// hovered when the grace ends.
	pending    K
	hasPending bool
}

// isSub reports whether k opens a submenu.
func (c *collState[K]) isSub(k K) bool { return c.cfg.kind == kindMenu && c.sub.keys[k] }

// subMenuRef returns the ref of the submenu that the item k opens.
func (c *collState[K]) subMenuRef(k K) tela.ElementRef {
	r, ok := c.sub.menuRefs[k]
	if !ok {
		r = core.NewElementRef()
		c.sub.menuRefs[k] = r
	}
	return r
}

// syncSub takes the submenu keys of a build.
func (c *collState[K]) syncSub(keys []K) {
	s := &c.sub
	clear(s.keys)
	for _, k := range keys {
		if c.has(k) {
			s.keys[k] = true
		}
	}
	for k := range s.menuRefs {
		if !s.keys[k] {
			delete(s.menuRefs, k)
		}
	}
	if s.open && !s.keys[s.key] {
		c.closeSub()
	}
	if p := c.cfg.parent; p.lvl != nil {
		p.lvl.setChild(c)
	}
}

// openSub opens the submenu of k with a focus strategy. An open one only
// moves the focus into it when the strategy says so.
func (c *collState[K]) openSub(k K, strategy string) {
	s := &c.sub
	s.openT.Stop()
	if c.isDisabled(k) || !c.isSub(k) {
		return
	}
	if s.open && s.key == k {
		if strategy != "" && s.child != nil {
			s.child.focusEdge(strategy)
		}
		return
	}
	s.open, s.key, s.strategy, s.out = true, k, strategy, false
	c.endGrace(false)
	c.invalidate()
}

// closeSub closes this level's open submenu.
func (c *collState[K]) closeSub() {
	s := &c.sub
	s.openT.Stop()
	if !s.open {
		return
	}
	s.open, s.child, s.out = false, nil, false
	c.endGrace(false)
	c.invalidate()
}

// focusChanged closes the open submenu when the focus moves to another
// item of this menu (react-stately closes the submenus of a level when
// another of its items takes the focus).
func (c *collState[K]) focusChanged(k K) {
	s := &c.sub
	if s.openT.Pending() && s.key != k {
		s.openT.Stop()
	}
	if s.open && s.key != k {
		c.closeSub()
	}
}

// hoverSub starts the open delay of the submenu of k, which the pointer
// is over.
func (c *collState[K]) hoverSub(k K) {
	s := &c.sub
	if !c.isSub(k) || (s.open && s.key == k) || s.openT.Pending() {
		return
	}
	s.key = k
	s.openT.Start(SubmenuDelay, func() { c.openSub(k, "") })
}

func (c *collState[K]) chain() []tela.ElementRef {
	var up []tela.ElementRef
	if p := c.cfg.parent; p.lvl != nil {
		up = p.lvl.chain()
	}
	return append(up[:len(up):len(up)], c.ref)
}

func (c *collState[K]) rootOutside() func(tela.PointerEvent) bool {
	if p := c.cfg.parent; p.lvl != nil {
		return p.lvl.rootOutside()
	}
	return c.cfg.outside
}

func (c *collState[K]) closeAllMenus() {
	if p := c.cfg.parent; p.lvl != nil {
		p.CloseAll()
		return
	}
	if f := c.cfg.onClose; f != nil {
		f()
	}
}

func (c *collState[K]) outside() *bool { return &c.sub.out }

func (c *collState[K]) setChild(ch subChild) { c.sub.child = ch }

func (c *collState[K]) dropChild(ch subChild) {
	if c.sub.child == ch {
		c.sub.child = nil
	}
}

func (c *collState[K]) focusEdge(strategy string) {
	var (
		k  K
		ok bool
	)
	if strategy == FocusLast {
		k, ok = c.lastKey()
	} else {
		k, ok = c.firstKey()
	}
	if ok {
		c.within = true
		c.moveFocus(k)
	}
}

// submenuTrigger returns the state of the submenu that the item k opens.
func (c *collState[K]) submenuTrigger(k K) SubmenuTriggerResult {
	s := &c.sub
	if !c.isSub(k) {
		return SubmenuTriggerResult{}
	}
	open := s.open && s.key == k
	trig, menu := c.refFor(k), c.subMenuRef(k)
	dir := "ltr"
	back := tela.KeyArrowLeft
	if c.cfg.rtl {
		dir, back = "rtl", tela.KeyArrowRight
	}
	r := SubmenuTriggerResult{
		Open: open, Ref: trig, MenuRef: menu, Direction: dir, lvl: c,
		Close: func() {
			if s.open && s.key == k {
				c.closeSub()
			}
		},
		CloseAll: c.closeAllMenus,
	}
	if open {
		r.FocusStrategy = s.strategy
	}
	r.MenuProps = core.MergeProps(h.Ref(menu), h.AriaLabelledBy(trig),
		h.OnKeyDown(func(e tela.KeyboardEvent) {
			if e.Ctrl || e.Meta || e.Alt || (e.Key != back && e.Key != tela.KeyEscape) {
				return
			}
			// The key closes one level and gives the focus back to the
			// trigger (React Aria's useSubmenuTrigger), and goes no
			// further: a menu bar does not move to another menu, and the
			// parent menu stays open.
			e.PreventDefault()
			e.StopPropagation()
			if e.Repeat && e.Key == tela.KeyEscape {
				return
			}
			c.within = true
			c.focusElement(k)
			c.closeSub()
		}),
		// A press that started outside and ended inside closes nothing.
		h.OnPointerUp(func() { s.out = false }))
	return r
}

// subItemProps are the props of an item that opens a submenu.
func (c *collState[K]) subItemProps(k K) []tela.Prop {
	s := &c.sub
	open := s.open && s.key == k
	ps := []tela.Prop{h.AriaHasPopup("menu"), h.AriaExpanded(open)}
	if open {
		ps = append(ps, h.AriaControls(c.subMenuRef(k)))
	}
	fwd := tela.KeyArrowRight
	if c.cfg.rtl {
		fwd = tela.KeyArrowLeft
	}
	// On the item, so that it runs before the handlers of the menu's own
	// element (a menu bar's ArrowRight moves to the next menu).
	ps = append(ps, h.OnKeyDown(func(e tela.KeyboardEvent) {
		if e.Key != fwd || e.Ctrl || e.Meta || e.Alt || c.isDisabled(k) || !c.refFor(k).Focused() {
			return
		}
		e.PreventDefault()
		e.StopPropagation()
		c.openSub(k, FocusFirst)
	}))
	return ps
}

// menuPointerMove is React Aria's useSafelyMouseToSubmenu, on the parent
// menu's element: while the mouse moves toward the open submenu, the items
// it crosses are not hovered (onItemHover asks graceActive).
func (c *collState[K]) menuPointerMove(e tela.PointerEvent) {
	s := &c.sub
	if !s.open || e.Kind == tela.PointerTouch {
		return
	}
	sub := c.subMenuRef(s.key).Bounds()
	if sub.Width == 0 || sub.Contains(e.X, e.Y) || !c.ref.Bounds().Contains(e.X, e.Y) {
		// Not laid out yet, or the move bubbled from inside the submenu
		// (a Portal is inside its parent in the element tree): the grace
		// is over, the pointer got there.
		if sub.Width != 0 && sub.Contains(e.X, e.Y) {
			c.endGrace(false)
		}
		return
	}
	now := core.Clock(c.ref)
	if s.hasPrev && now-s.last < graceThrottle {
		return
	}
	s.graceT.Stop()
	if !s.hasPrev {
		s.prevX, s.prevY, s.hasPrev, s.last = e.X, e.Y, true, now
		return
	}
	menu := c.ref.Bounds()
	right := sub.X+sub.Width/2 >= menu.X+menu.Width/2
	px, py := float64(s.prevX), float64(s.prevY)
	toSub := float64(sub.X) - px
	dx := float64(e.X - s.prevX)
	if !right {
		toSub = px - float64(sub.X+sub.Width)
		dx = -dx
	}
	top := math.Atan2(py-float64(sub.Y), toSub) + graceAngle
	bottom := math.Atan2(py-float64(sub.Y+sub.Height), toSub) - graceAngle
	ptr := math.Atan2(py-float64(e.Y), dx)
	if ptr < top && ptr > bottom {
		s.toward = min(s.toward+1, graceMoves)
	} else {
		s.toward = max(s.toward-1, 0)
	}
	s.last, s.prevX, s.prevY = now, e.X, e.Y
	if s.toward >= graceMoves {
		s.grace = true
		// A mouse that rests ends the grace, and the item under it is
		// hovered then.
		s.graceT.Start(graceTimeout, func() { c.endGrace(true) })
		return
	}
	if s.grace {
		c.endGrace(true)
	}
}

// graceActive reports whether the pointer is moving toward the open
// submenu.
func (c *collState[K]) graceActive() bool { return c.sub.open && c.sub.grace }

// endGrace ends the pointer grace; with hover, the item the pointer moved
// over during it is hovered now (React Aria lets the pointer events reach
// the parent menu again, and the browser hovers what is under the
// pointer). Without hover the moves are forgotten too (the submenu opened
// or closed, or the pointer reached it).
func (c *collState[K]) endGrace(hover bool) {
	s := &c.sub
	was := s.grace
	s.grace = false
	s.graceT.Stop()
	if !hover {
		s.hasPrev, s.toward = false, 0
	}
	k, ok := s.pending, s.hasPending
	s.hasPending = false
	if hover && was && ok && c.has(k) && !c.isDisabled(k) && !(s.open && s.key == k) {
		s.hasPrev, s.toward = false, 0
		c.within = true
		c.moveFocus(k)
		c.hoverSub(k)
	}
}
