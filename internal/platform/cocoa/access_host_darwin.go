//go:build darwin

package cocoa

import (
	"fmt"
	"slices"

	"github.com/kuronosu/tela/internal/darwin"
)

// The hosts of the elements: TelaView (the root) and TelaWindow, over
// both NSAccessibility APIs.
//
// A trusted AX client in another process never reached TelaView's
// accessibilityChildren (macOS 26): its queries of the window's children
// and of the focused element got AppKit's own answer (the title bar
// buttons and the title; the window focused) and the bridge never
// activated, while the same queries made in-process did reach the view.
// So the window and the view both answer the queries that reach the
// tree themselves, in both APIs: children (accessibilityChildren, the
// AXChildren attribute and the array attribute helpers), the focused
// element and hit testing. Each one activates the bridge. The window's
// children are the root's exposed elements followed by AppKit's own (the
// title bar). Every accessibility message the two classes receive here
// is traced (accessTrace), so a client test shows which path AppKit
// takes.

// axChildrenAttr and axFocusedAttr are the attributes the hosts answer.
const (
	axChildrenAttr = "AXChildren"
	axFocusedAttr  = "AXFocusedUIElement"
)

// rootElements returns the elements of the root's exposed children,
// activating the bridge.
func (w *Window) rootElements() []darwin.ID {
	w.activateAccess()
	v := w.acc.tree.Load()
	if v == nil {
		return nil
	}
	out := make([]darwin.ID, 0, len(v.kids(v.root())))
	for _, id := range v.kids(v.root()) {
		if e := w.element(id); e != 0 {
			out = append(out, e)
		}
	}
	return out
}

// focusedElement returns the element of the focused node, or 0 when no
// element has the focus (or the tree is empty), activating the bridge.
func (w *Window) focusedElement() darwin.ID {
	w.activateAccess()
	v := w.acc.tree.Load()
	if v == nil {
		return 0
	}
	if f := v.focus(); f != v.root() {
		return w.element(f)
	}
	return 0
}

// windowChildren merges the window's own children (AppKit's answer)
// with the root's elements: the elements first, then AppKit's other
// children (TelaAccessElements already there are not repeated).
func (w *Window) windowChildren(own []darwin.ID) []darwin.ID {
	out := w.rootElements()
	for _, o := range own {
		if !slices.Contains(out, o) && !isElement(o) {
			out = append(out, o)
		}
	}
	return out
}

// isElement reports whether o is a TelaAccessElement.
func isElement(o darwin.ID) bool {
	if o == 0 || axAccess.class == 0 {
		return false
	}
	return darwin.SendBool(o, asel.isKindOfClass, axAccess.class)
}

// hostWindow returns the window of a TelaWindow or TelaView self when the
// bridge can answer for it.
func hostWindow(self darwin.ID) *Window { return accessWindow(self) }

// htrace traces a message a host received, with the thread and whether
// the host's window was found.
func htrace(self darwin.ID, who, format string, args ...any) {
	if accessTrace == nil {
		return
	}
	w := windowFor(self)
	title := "<none>"
	if w != nil {
		title = w.opts.Title
	}
	atracef("%s %s (main thread %v, window %q, active %v)", who, fmt.Sprintf(format, args...), darwin.IsMainThread(),
		title, w != nil && w.acc.active.Load())
}

// hostMethods returns the accessibility methods of a host class whose
// superclass is super: children computes the host's children from
// AppKit's own answer (nil when super has none), focus the focused
// element (0: AppKit's answer), hit the element at a screen point (0:
// AppKit's answer).
func hostMethods(who string, super func() darwin.Class,
	children func(w *Window, own []darwin.ID) []darwin.ID) []darwin.Method {
	superArray := func(self darwin.ID, cmd darwin.SEL, args ...any) []darwin.ID {
		v, _ := superSend(self, super(), cmd, args...)
		return darwin.ArrayItems(v)
	}
	isChildren := func(attr darwin.ID) bool { return darwin.GoString(attr) == axChildrenAttr }
	return []darwin.Method{
		{Sel: darwin.Sel("accessibilityChildren"), Fn: func(self darwin.ID, cmd darwin.SEL) darwin.ID {
			htrace(self, who, "accessibilityChildren")
			if w := hostWindow(self); w != nil {
				return darwin.NSArray(children(w, superArray(self, cmd))...)
			}
			v, _ := superSend(self, super(), cmd)
			return v
		}},
		{Sel: darwin.Sel("accessibilityChildrenInNavigationOrder"), Fn: func(self darwin.ID, cmd darwin.SEL) darwin.ID {
			htrace(self, who, "accessibilityChildrenInNavigationOrder")
			if w := hostWindow(self); w != nil {
				return darwin.NSArray(children(w, superArray(self, cmd))...)
			}
			v, _ := superSend(self, super(), cmd)
			return v
		}},
		{Sel: darwin.Sel("accessibilityFocusedUIElement"), Fn: func(self darwin.ID, cmd darwin.SEL) darwin.ID {
			htrace(self, who, "accessibilityFocusedUIElement")
			if w := hostWindow(self); w != nil {
				if f := w.focusedElement(); f != 0 && w.hostsFocus() {
					return f
				}
			}
			v, _ := superSend(self, super(), cmd)
			return v
		}},
		{Sel: asel.hitTest, Fn: func(self darwin.ID, cmd darwin.SEL, p darwin.CGPoint) darwin.ID {
			htrace(self, who, "accessibilityHitTest: %v", p)
			if w := hostWindow(self); w != nil {
				w.activateAccess()
				if v := w.acc.tree.Load(); v != nil {
					x, y := w.viewPoint(p)
					if id := v.hitTest(v.root(), x, y); id != 0 {
						return w.element(id)
					}
				}
			}
			v, _ := superSend(self, super(), cmd, p)
			return v
		}},
		{Sel: darwin.Sel("accessibilityAttributeValue:"), Fn: func(self darwin.ID, cmd darwin.SEL, attr darwin.ID) darwin.ID {
			name := darwin.GoString(attr)
			htrace(self, who, "accessibilityAttributeValue: %s", name)
			if w := hostWindow(self); w != nil {
				switch name {
				case axChildrenAttr:
					return darwin.NSArray(children(w, superArray(self, cmd, attr))...)
				case axFocusedAttr:
					if f := w.focusedElement(); f != 0 && w.hostsFocus() {
						return f
					}
				}
			}
			v, _ := superSend(self, super(), cmd, attr)
			return v
		}},
		{Sel: darwin.Sel("accessibilityArrayAttributeCount:"), Fn: func(self darwin.ID, cmd darwin.SEL, attr darwin.ID) uint {
			htrace(self, who, "accessibilityArrayAttributeCount: %s", darwin.GoString(attr))
			if w := hostWindow(self); w != nil && isChildren(attr) {
				return uint(len(children(w, superArray(self, darwin.Sel("accessibilityAttributeValue:"), attr))))
			}
			v, _ := superSend(self, super(), cmd, attr)
			return uint(v)
		}},
		{Sel: darwin.Sel("accessibilityArrayAttributeValues:index:maxCount:"), Fn: func(self darwin.ID, cmd darwin.SEL, attr darwin.ID, index, maxCount uint) darwin.ID {
			htrace(self, who, "accessibilityArrayAttributeValues: %s %d %d", darwin.GoString(attr), index, maxCount)
			if w := hostWindow(self); w != nil && isChildren(attr) {
				items := children(w, superArray(self, darwin.Sel("accessibilityAttributeValue:"), attr))
				start := min(int(min(index, 1<<31)), len(items))
				end := min(start+int(min(maxCount, 1<<31)), len(items))
				return darwin.NSArray(items[start:end]...)
			}
			v, _ := superSend(self, super(), cmd, attr, index, maxCount)
			return v
		}},
		{Sel: darwin.Sel("accessibilityIndexOfChild:"), Fn: func(self darwin.ID, cmd darwin.SEL, child darwin.ID) uint {
			htrace(self, who, "accessibilityIndexOfChild:")
			if w := hostWindow(self); w != nil && isElement(child) {
				if i := slices.Index(children(w, superArray(self, darwin.Sel("accessibilityAttributeValue:"), darwin.AutoString(axChildrenAttr))), child); i >= 0 {
					return uint(i)
				}
			}
			v, ok := superSend(self, super(), cmd, child)
			if !ok {
				return uint(darwin.NSNotFound)
			}
			return uint(v)
		}},
		// Traced only.
		{Sel: darwin.Sel("accessibilityAttributeNames"), Fn: func(self darwin.ID, cmd darwin.SEL) darwin.ID {
			htrace(self, who, "accessibilityAttributeNames")
			v, _ := superSend(self, super(), cmd)
			return v
		}},
		{Sel: darwin.Sel("accessibilityIsIgnored"), Fn: func(self darwin.ID, cmd darwin.SEL) bool {
			v, ok := superSend(self, super(), cmd)
			htrace(self, who, "accessibilityIsIgnored: %v", ok && v&0xff != 0)
			return ok && v&0xff != 0
		}},
	}
}

// hostsFocus reports whether the window's focus is the view's: TelaView
// is the first responder (a field editor or another view would have it
// otherwise).
func (w *Window) hostsFocus() bool {
	return darwin.Send(w.ns, axSelOf("firstResponder")) == w.view
}

// accessViewMethods are TelaView's accessibility methods: the view
// stands for the root of the tree, so its children are the root's.
func accessViewMethods() []darwin.Method {
	return hostMethods("view", func() darwin.Class { return app.nsView },
		func(w *Window, _ []darwin.ID) []darwin.ID { return w.rootElements() })
}

// accessWindowMethods are TelaWindow's accessibility methods.
func accessWindowMethods() []darwin.Method {
	return hostMethods("window", func() darwin.Class { return app.windowSuper },
		func(w *Window, own []darwin.ID) []darwin.ID { return w.windowChildren(own) })
}
