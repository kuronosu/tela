package core

import (
	"fmt"
	"math"
	"strings"

	"github.com/kuronosu/tela/internal/semantics"
)

// The ARIA props the OS accessibility bridges read (ADR-246): live regions
// (aria-live, aria-atomic), key shortcuts (aria-keyshortcuts), positions in
// sets (aria-posinset, aria-setsize) and a deferred accessible name
// (AriaLabelFn). They change nothing on screen and Playwright's aria
// snapshot prints none of them.

// ariaA11y holds the props of this file (ariaProps.x), allocated by the
// first one applied.
type ariaA11y struct {
	live         string // "off", "polite", "assertive"; "" unset
	atomic       semantics.Toggle
	keyShortcuts string
	posInSet     int32 // 0 unset
	setSize      int32 // 0 unset, -1 unknown
}

func ariaA11yProp(name string, f func(x *ariaA11y)) Prop {
	return ariaProp(name, func(a *ariaProps) {
		if a.x == nil {
			a.x = &ariaA11y{}
		}
		f(a.x)
	})
}

var liveValues = []string{"off", "polite", "assertive"}

// AriaLive makes the element a live region (aria-live): assistive
// technologies announce changes of its content without the focus moving
// there. "polite" waits for the user to be idle, "assertive" interrupts,
// "off" (the default) announces nothing and ends a region inherited from
// an ancestor or implied by the role (alert is assertive, status and log
// polite). ASCII case-insensitive; unknown values panic in dev mode with
// the closest one and are ignored otherwise.
func AriaLive(politeness string) Prop {
	v, ok := ariaToken("AriaLive", politeness, liveValues)
	return ariaA11yProp("AriaLive", func(x *ariaA11y) {
		if ok {
			x.live = v
		}
	})
}

// AriaAtomic sets whether assistive technologies read the whole live
// region when part of it changes (aria-atomic), rather than the changed
// nodes only. alert and status are atomic by default.
func AriaAtomic(atomic bool) Prop {
	return ariaA11yProp("AriaAtomic", func(x *ariaA11y) { x.atomic = semantics.Of(atomic) })
}

// AriaKeyShortcuts names the keys that activate or focus the element
// (aria-keyshortcuts), in ARIA's syntax: space-separated alternatives, each
// a "+"-joined chord of UI Events key values with the modifiers first,
// such as "Control+S Meta+S" or "Alt+Shift+N". It only describes the
// shortcut; the app handles the keys. In dev mode an empty key ("Control+")
// or a common alias of a key value ("Ctrl", "Cmd", "Option", "Esc")
// panics with the right name (CheckKeyShortcuts).
func AriaKeyShortcuts(keys string) Prop {
	if devNow() {
		if msg := CheckKeyShortcuts(keys); msg != "" {
			panic(msg)
		}
	}
	keys = strings.Join(strings.Fields(keys), " ")
	return ariaA11yProp("AriaKeyShortcuts", func(x *ariaA11y) { x.keyShortcuts = keys })
}

// keyAliases are names people write for UI Events key values.
var keyAliases = map[string]string{
	"ctrl": "Control", "ctl": "Control", "cmd": "Meta", "command": "Meta", "win": "Meta", "windows": "Meta",
	"super": "Meta", "option": "Alt", "opt": "Alt", "esc": "Escape", "del": "Delete", "return": "Enter",
	"ins": "Insert", "up": "ArrowUp", "down": "ArrowDown", "left": "ArrowLeft", "right": "ArrowRight",
	"pgup": "PageUp", "pgdn": "PageDown", "space": "\" \" (a space is written Space in ARIA: \"Shift+Space\")",
}

// CheckKeyShortcuts returns the message of AriaKeyShortcuts(keys) for a
// value ARIA does not allow: a chord with an empty key ("Control+",
// "Control++"), or a key written with a common alias of its UI Events
// value ("Ctrl+S" is "Control+S"). It is a loose check: other key names
// are not looked up.
func CheckKeyShortcuts(keys string) string {
	for _, chord := range strings.Fields(keys) {
		for _, k := range strings.Split(chord, "+") {
			if k == "" {
				return fmt.Sprintf("tela: AriaKeyShortcuts(%q): the chord %q has an empty key; join keys with a single \"+\" (\"Control+S\") and write the + key as Plus (\"Control+Plus\")", keys, chord)
			}
			lk := strings.ToLower(k)
			if lk == "space" {
				continue // ARIA 1.2's own example: "Shift+Space"
			}
			if v, ok := keyAliases[lk]; ok {
				return fmt.Sprintf("tela: AriaKeyShortcuts(%q): %q is not a UI Events key value; did you mean %q?", keys, k, v)
			}
		}
	}
	return ""
}

// AriaPosInSet sets the position of an item in its set (aria-posinset),
// from 1, for a set that has only some of its items in the tree (a
// virtualized list, a paged listbox). Without it the position is computed
// from the items in the tree (semantics.FillSetPositions). Values below 1
// are ignored.
func AriaPosInSet(pos int) Prop {
	p := int32(max(min(pos, math.MaxInt32), 0))
	return ariaA11yProp("AriaPosInSet", func(x *ariaA11y) { x.posInSet = p })
}

// AriaSetSize sets the number of items of the set an item belongs to
// (aria-setsize), on the item or on its container (a listbox, menu, list,
// tree...), for a set that has only some of its items in the tree; -1 says
// the size is unknown. 0 is ignored.
func AriaSetSize(n int) Prop {
	c := int32(max(min(n, math.MaxInt32), -1))
	return ariaA11yProp("AriaSetSize", func(x *ariaA11y) { x.setSize = c })
}

// AriaLabelFn sets the accessible name of a primitive, like AriaLabel, to
// the result of fn, called when the semantics tree is built rather than
// when the component builds. Values that fn reads with Get are tracked: a
// change of one rebuilds nothing and needs no frame; it only makes the
// next Tree.Semantics build a fresh tree, and tells the semantics listener
// (Tree.OnSemanticsStale). fn runs on the UI thread and should only
// compute the string. AriaLabel and AriaLabelFn replace each other.
func AriaLabelFn(fn func() string) Prop {
	if fn == nil {
		panic("tela: AriaLabelFn(nil): pass a function that returns the name, such as AriaLabelFn(func() string { return fmt.Sprintf(\"%d%% uploaded\", progress.Get()) })")
	}
	return ariaProp("AriaLabelFn", func(a *ariaProps) { a.label, a.labelFn, a.hasLabel = "", fn, true })
}

// implicitLive returns the aria-live and aria-atomic values a role implies
// (ARIA 1.2): alert is assertive and atomic, status polite and atomic, log
// polite; timer and marquee are off, which ends a region around them as
// AriaLive("off") does.
func implicitLive(role string) (live string, atomic bool) {
	switch role {
	case "alert":
		return "assertive", true
	case "status":
		return "polite", true
	case "log":
		return "polite", false
	case "timer", "marquee":
		return "off", false
	}
	return "", false
}

// semLive sets the live region fields and the props of this file on node
// n of a primitive with ARIA props a (nil without) and role role, and
// returns the live region root of its subtree: n's ID for a region root,
// 0 for AriaLive("off"), inherited otherwise.
func semLive(a *ariaProps, n *semantics.Node, inherited semantics.NodeID) semantics.NodeID {
	live, atomic := implicitLive(n.Role)
	var x *ariaA11y
	if a != nil {
		x = a.x
	}
	if x != nil {
		if x.live != "" {
			live = x.live
		}
		if x.atomic != semantics.ToggleUnset {
			atomic = x.atomic == semantics.ToggleTrue
		}
		n.KeyShortcuts = x.keyShortcuts
		n.PosInSet, n.SetSize = int(x.posInSet), int(x.setSize)
	}
	root := inherited
	switch live {
	case "off":
		live, root = "", 0
	case "polite", "assertive":
		root = n.ID
	}
	n.Live, n.Atomic, n.LiveRoot = live, atomic, root
	return root
}
