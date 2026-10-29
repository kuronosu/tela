// Portions of this file follow React Aria's useRadioGroup, useToolbar and
// createFocusManager (https://github.com/adobe/react-spectrum,
// packages/@react-aria/radio, packages/@react-aria/toolbar and
// packages/@react-aria/focus), Copyright 2020 Adobe, licensed under the
// Apache License, Version 2.0. See THIRD_PARTY.md.

package aria

import (
	"github.com/kuronosu/tela"
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

	keyList[K] // the keys and refs; all set means the whole group is disabled

	// focused is the item that has the focus, or had it last: the tab stop
	// of a group that remembers it.
	focused    K
	hasFocused bool
}

// useRover holds the roving state of a group and syncs it with the keys
// of this build.
func useRover[K comparable](keys, disabled []K, all bool) *rover[K] {
	_, bump := tela.UseState(0)
	r := tela.UseOnce(func() *rover[K] { return &rover[K]{keyList: keyList[K]{refs: map[K]tela.ElementRef{}}} })
	r.bump = bump
	r.sync(keys, disabled, all)
	return r
}

func (r *rover[K]) invalidate() {
	r.bump.Update(func(n int) int { return n + 1 })
}

func (r *rover[K]) sync(keys, disabled []K, all bool) {
	r.all = all
	r.setDisabled(disabled)
	if _, changed := r.setKeys(keys); !changed {
		return
	}
	if r.hasFocused {
		if _, ok := r.index[r.focused]; !ok {
			r.hasFocused = false
		}
	}
}

// enabled reports whether k is an item that can be focused and acted on.
func (r *rover[K]) enabled(k K) bool { return r.has(k) && !r.isDisabled(k) }

// next returns the enabled item after from (before it when forward is
// false), going around the ends when wrap is set. It reports false when
// there is none other than from.
func (r *rover[K]) next(from K, forward, wrap bool) (K, bool) {
	var zero K
	i, ok := r.index[from]
	if !ok {
		if forward {
			return r.firstKey()
		}
		return r.lastKey()
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
