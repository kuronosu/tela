// Portions of this file follow React Aria's useRadioGroup, useToolbar and
// createFocusManager (https://github.com/adobe/react-spectrum,
// packages/@react-aria/radio, packages/@react-aria/toolbar and
// packages/@react-aria/focus), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"slices"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// rover is the roving focus of a group of keyed items that are not a
// collection (a radio group, a toggle button group): the keys in order,
// the disabled ones, an ElementRef per key and the item that had the focus
// last. A hook cannot be called once per item, so the state lives in one
// rover per hook call, written directly by the handlers (a second event
// before the next build sees the first one's effect) and read by the
// props of the build.
type rover[K comparable] struct {
	bump tela.Setter[int]

	keys     []K
	index    map[K]int
	disabled map[K]struct{}
	all      bool // the whole group is disabled
	refs     map[K]tela.ElementRef

	// focused is the item that has the focus, or had it last: the tab stop
	// of a group that remembers it.
	focused    K
	hasFocused bool
}

// useRover holds the roving state of a group and syncs it with the keys
// of this build.
func useRover[K comparable](keys, disabled []K, all bool) *rover[K] {
	_, bump := tela.UseState(0)
	r := tela.UseOnce(func() *rover[K] { return &rover[K]{refs: map[K]tela.ElementRef{}} })
	r.bump = bump
	r.sync(keys, disabled, all)
	return r
}

func (r *rover[K]) invalidate() {
	r.bump.Update(func(n int) int { return n + 1 })
}

func (r *rover[K]) sync(keys, disabled []K, all bool) {
	r.all = all
	if len(disabled) == 0 {
		r.disabled = nil
	} else {
		r.disabled = make(map[K]struct{}, len(disabled))
		for _, k := range disabled {
			r.disabled[k] = struct{}{}
		}
	}
	if r.index != nil && slices.Equal(r.keys, keys) {
		return
	}
	r.keys = slices.Clone(keys)
	r.index = make(map[K]int, len(keys))
	for i, k := range keys {
		r.index[k] = i
	}
	for k := range r.refs {
		if _, ok := r.index[k]; !ok {
			delete(r.refs, k)
		}
	}
	if r.hasFocused {
		if _, ok := r.index[r.focused]; !ok {
			r.hasFocused = false
		}
	}
}

func (r *rover[K]) has(k K) bool {
	_, ok := r.index[k]
	return ok
}

// isDisabled reports whether the item is disabled, by its key or by the
// group.
func (r *rover[K]) isDisabled(k K) bool {
	if r.all {
		return true
	}
	_, d := r.disabled[k]
	return d
}

// enabled reports whether k is an item that can be focused and acted on.
func (r *rover[K]) enabled(k K) bool { return r.has(k) && !r.isDisabled(k) }

func (r *rover[K]) refFor(k K) tela.ElementRef {
	ref, ok := r.refs[k]
	if !ok {
		ref = core.NewElementRef()
		r.refs[k] = ref
	}
	return ref
}

func (r *rover[K]) first() (K, bool) {
	for _, k := range r.keys {
		if !r.isDisabled(k) {
			return k, true
		}
	}
	var zero K
	return zero, false
}

func (r *rover[K]) last() (K, bool) {
	for i := len(r.keys) - 1; i >= 0; i-- {
		if !r.isDisabled(r.keys[i]) {
			return r.keys[i], true
		}
	}
	var zero K
	return zero, false
}

// next returns the enabled item after from (before it when forward is
// false), going around the ends when wrap is set. It reports false when
// there is none other than from.
func (r *rover[K]) next(from K, forward, wrap bool) (K, bool) {
	var zero K
	i, ok := r.index[from]
	if !ok {
		if forward {
			return r.first()
		}
		return r.last()
	}
	n := len(r.keys)
	for step := 1; step < n; step++ {
		j := i + step
		if !forward {
			j = i - step
		}
		if j < 0 || j >= n {
			if !wrap {
				return zero, false
			}
			j = (j%n + n) % n
		}
		if !r.isDisabled(r.keys[j]) {
			return r.keys[j], true
		}
	}
	return zero, false
}

// onFocus records the item that took the focus.
func (r *rover[K]) onFocus(k K) {
	if !r.refFor(k).Focused() {
		return // a descendant of the item took the focus
	}
	if r.hasFocused && r.focused == k {
		return
	}
	r.focused, r.hasFocused = k, true
	r.invalidate()
}

// arrowDirection maps an arrow key to a step along a group's items: +1
// forward, -1 backward, 0 for a key that does not move the focus. All four
// arrows work whatever the orientation when both is set (a radio group);
// otherwise only the two of the orientation do (a toolbar). A right-to-left
// direction swaps the horizontal arrows of a horizontal group.
func arrowDirection(key tela.KeyName, horizontal, rtl, both bool) int {
	switch key {
	case tela.KeyArrowDown:
		if both || !horizontal {
			return 1
		}
	case tela.KeyArrowUp:
		if both || !horizontal {
			return -1
		}
	case tela.KeyArrowRight, tela.KeyArrowLeft:
		if both || horizontal {
			d := 1
			if key == tela.KeyArrowLeft {
				d = -1
			}
			if rtl && horizontal {
				d = -d
			}
			return d
		}
	}
	return 0
}
